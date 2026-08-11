package creative

import (
	"context"
	"fmt"
	"strings"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type GenerateCommercePrerollV2HooksRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

type SelectCommercePrerollV2HookRequest struct {
	ExpectedRevision int64  `json:"expected_revision"`
	HookID           string `json:"hook_id"`
	DurationSeconds  int    `json:"duration_seconds"`
	ExtraInstruction string `json:"extra_instruction,omitempty"`
}

func (s Service) GenerateCommercePrerollV2Hooks(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request GenerateCommercePrerollV2HooksRequest) (TaskDetail, error) {
	return s.reviseCommercePrerollV2(ctx, actor, projectID, taskID, request.ExpectedRevision,
		func(draft *VideoDraft, workspace *CommercePrerollV2Workspace) error {
			if workspace.ActiveStage != CommercePrerollV2StageUnderstandingConfirmed {
				return ErrInvalidState
			}
			workspace.HookBatch = &CommercePrerollV2HookBatch{
				CommercePrerollV2AsyncResource: CommercePrerollV2AsyncResource{Status: CommercePrerollV2ResourceReady, Progress: 100},
				ID:                             fmt.Sprintf("%s_hooks_%d", taskID, workspace.Revision), Revision: workspace.Revision,
				Items: buildCommercePrerollV2Hooks(workspace.Analysis.Content),
			}
			workspace.PromptDraft = nil
			workspace.ActiveStage = CommercePrerollV2StageHooksReady
			draft.Prompt = "已生成 5 个前贴钩子，等待人工选择"
			return nil
		}, TaskInProgress)
}

func (s Service) SelectCommercePrerollV2Hook(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request SelectCommercePrerollV2HookRequest) (TaskDetail, error) {
	if request.DurationSeconds < 6 || request.DurationSeconds > 10 || strings.TrimSpace(request.HookID) == "" || len(request.ExtraInstruction) > 1000 {
		return TaskDetail{}, fmt.Errorf("hook_id and duration_seconds between 6 and 10 are required")
	}
	return s.reviseCommercePrerollV2(ctx, actor, projectID, taskID, request.ExpectedRevision,
		func(draft *VideoDraft, workspace *CommercePrerollV2Workspace) error {
			if workspace.HookBatch == nil || workspace.HookBatch.Status != CommercePrerollV2ResourceReady {
				return ErrInvalidState
			}
			var selected *CommercePrerollV2HookRecipe
			for index := range workspace.HookBatch.Items {
				if workspace.HookBatch.Items[index].ID == request.HookID {
					selected = &workspace.HookBatch.Items[index]
					break
				}
			}
			if selected == nil {
				return fmt.Errorf("selected commerce preroll hook does not exist")
			}
			prompt, err := compileCommercePrerollV2Prompt(workspace, *selected, request.DurationSeconds, request.ExtraInstruction)
			if err != nil {
				return err
			}
			workspace.HookBatch.SelectedHookID = selected.ID
			workspace.PromptDraft = &prompt
			workspace.ActiveStage = CommercePrerollV2StageHookSelected
			draft.DurationSeconds = request.DurationSeconds
			draft.Prompt = prompt.CompiledPrompt
			return nil
		}, TaskInProgress)
}

func buildCommercePrerollV2Hooks(understanding CommercePrerollV2SourceUnderstanding) []CommercePrerollV2HookRecipe {
	product := understanding.Product
	sellingPoint := strings.Join(product.SellingPoints, "、")
	guardrails := append(append([]string{}, product.AppearanceGuardrails...), product.LogoGuardrails...)
	baseNegative := []string{"不得虚构原视频未表达的卖点或功效", "不得改变商品瓶型、标签、Logo、主色和文字布局", "不得出现多余商品或不可辨认文字"}
	items := []CommercePrerollV2HookRecipe{
		{ID: "product-cut", Name: "商品切割", Mechanism: "瞬间冲击", Concept: fmt.Sprintf("用利落切割动作揭示%s的核心卖点", product.Name), Rationale: "动作信息密度高，适合快速建立注意力", SellingPoint: sellingPoint, PrimaryAction: "单次切割或剖开展示，商品本体保持完整", CameraRules: []string{"近景定机位", "动作结束后回到商品正面"}, ProductGuardrails: guardrails, NegativeConstraints: baseNegative},
		{ID: "frosted-reveal", Name: "雾面橱窗揭幕", Mechanism: "从不可见到清晰可见", Concept: fmt.Sprintf("从雾面遮挡中一次擦拭揭示%s", product.Name), Rationale: "信息缺口明确，尾帧易与原片开场衔接", SellingPoint: sellingPoint, PrimaryAction: "单次擦拭或揭幕，雾面由模糊变清晰", CameraRules: []string{"稳定正面构图", "不切换机位"}, ProductGuardrails: guardrails, NegativeConstraints: baseNegative},
		{ID: "one-tap-pick", Name: "一键取物", Mechanism: "动作魔法", Concept: fmt.Sprintf("一次点击让%s从场景中准确出现", product.Name), Rationale: "交互感强，适合电商点击心智", SellingPoint: sellingPoint, PrimaryAction: "单次点击或拿取，商品沿明确路径出现", CameraRules: []string{"手部不得遮挡标签", "保持商品朝向"}, ProductGuardrails: guardrails, NegativeConstraints: baseNegative},
		{ID: "mini-benefit", Name: "微缩功效剧场", Mechanism: "功效可视化", Concept: fmt.Sprintf("以微缩场景隐喻%s的已确认卖点", product.Name), Rationale: "用视觉隐喻解释卖点，但不新增功效结论", SellingPoint: sellingPoint, PrimaryAction: "一个微缩动作完成卖点隐喻", CameraRules: []string{"微距质感", "商品始终是视觉中心"}, ProductGuardrails: guardrails, NegativeConstraints: baseNegative},
		{ID: "device-recall", Name: "3C 设备召回", Mechanism: "场景追踪", Concept: fmt.Sprintf("让场景元素沿秩序路径汇聚并召回%s", product.Name), Rationale: "适合强调秩序、效率或精准使用场景", SellingPoint: sellingPoint, PrimaryAction: "一次汇聚或召回动作", CameraRules: []string{"轨迹清晰", "尾帧静止"}, ProductGuardrails: guardrails, NegativeConstraints: baseNegative},
	}
	contextText := strings.ToLower(strings.Join([]string{product.Category, product.Description, sellingPoint, understanding.VisualStyle, understanding.OpeningShot}, " "))
	bestIndex := 0
	bestScore := 0.0
	for index := range items {
		item := &items[index]
		item.RecipeVersion = "commerce-hook-recipe/v3"
		item.MatchScore = 0.68
		switch item.ID {
		case "frosted-reveal":
			if strings.Contains(contextText, "护肤") || strings.Contains(contextText, "美妆") || strings.Contains(contextText, "香水") || strings.Contains(contextText, "精华") {
				item.MatchScore += 0.23
			}
		case "device-recall":
			if strings.Contains(contextText, "3c") || strings.Contains(contextText, "设备") || strings.Contains(contextText, "数码") || strings.Contains(contextText, "家电") {
				item.MatchScore += 0.25
			}
		case "mini-benefit":
			if strings.Contains(contextText, "功效") || strings.Contains(contextText, "修护") || strings.Contains(contextText, "补水") || strings.Contains(contextText, "清洁") {
				item.MatchScore += 0.18
			}
		case "one-tap-pick":
			if strings.Contains(contextText, "轻便") || strings.Contains(contextText, "快速") || strings.Contains(contextText, "便携") {
				item.MatchScore += 0.17
			}
		case "product-cut":
			if strings.Contains(contextText, "食品") || strings.Contains(contextText, "玩具") || strings.Contains(contextText, "新奇") {
				item.MatchScore += 0.19
			}
		}
		item.RecommendationLevel = "alternative"
		item.SuitableFor = []string{product.Category, "强调" + sellingPoint}
		item.WhyForSource = []string{
			"原片视觉风格：" + understanding.VisualStyle,
			"原片开场特征：" + understanding.OpeningShot,
		}
		item.OpeningState = "先建立清晰的信息缺口，商品尚未完整显现"
		item.ResultState = "商品正面、标签和 Logo 清晰稳定，动作完全停止"
		item.ContinuityPlan = "尾段回到原片的构图、色温、光线方向和运动趋势"
		item.VisualSignature = item.Mechanism + "、单一主动作、商品正面定格"
		item.RiskNotes = []string{"动作不得遮挡标签", "不得改变商品瓶型、包装文字或 Logo"}
		if item.MatchScore > bestScore {
			bestIndex, bestScore = index, item.MatchScore
		}
	}
	items[bestIndex].RecommendationLevel = "primary"
	return items
}

func compileCommercePrerollV2Prompt(workspace *CommercePrerollV2Workspace, hook CommercePrerollV2HookRecipe, duration int, extra string) (CommercePrerollV2PromptDraft, error) {
	hookEnd := int64(duration * 250)
	changeEnd := int64((duration - 2) * 1000)
	beats := []CommercePrerollV2Beat{
		{ID: "hook", Label: "建立钩子", StartMS: 0, EndMS: hookEnd, Detail: hook.Concept, VisualDescription: hook.OpeningState, SubjectAction: "建立未完成状态", Camera: strings.Join(hook.CameraRules, "；"), SceneAndLighting: workspace.Analysis.Content.VisualStyle, ProductState: "商品轮廓或局部可辨", TransitionOut: "进入唯一主动作", AudioInstruction: workspace.Analysis.Content.AudioMood},
		{ID: "change", Label: "完成变化", StartMS: hookEnd, EndMS: changeEnd, Detail: hook.PrimaryAction, VisualDescription: hook.Concept, SubjectAction: hook.PrimaryAction, Camera: strings.Join(hook.CameraRules, "；"), SceneAndLighting: workspace.Analysis.Content.VisualStyle, ProductState: "商品外观与 Logo 全程保真", TransitionIn: "承接信息缺口", TransitionOut: "动作减速并准备定格", AudioInstruction: "声音只辅助主动作"},
		{ID: "lockup", Label: "商品定格", StartMS: changeEnd, EndMS: int64(duration * 1000), Detail: "商品正面清晰稳定，并向原视频开场锚点的构图、光线和运动趋势靠拢", VisualDescription: hook.ResultState, SubjectAction: "停止主动作并稳定停留", Camera: "机位稳定，不再切换", SceneAndLighting: workspace.Analysis.Content.VisualStyle, ProductState: hook.ResultState, TransitionIn: "动作自然减速", TransitionOut: hook.ContinuityPlan, AudioInstruction: "尾段声音平稳，为后续原片留出连接点"},
	}
	product := workspace.Analysis.Content.Product
	parts := []string{
		fmt.Sprintf("生成一条 %d 秒、9:16 的独立电商前贴视频。", duration),
		fmt.Sprintf("商品：%s；品类：%s；客观描述：%s。", product.Name, product.Category, product.Description),
		"仅使用已确认卖点：" + strings.Join(product.SellingPoints, "、") + "。",
		fmt.Sprintf("钩子机制：%s。唯一主动作：%s。", hook.Mechanism, hook.PrimaryAction),
		fmt.Sprintf("原视频视觉连续性：%s；原片开场：%s；音频气质：%s。", workspace.Analysis.Content.VisualStyle, workspace.Analysis.Content.OpeningShot, workspace.Analysis.Content.AudioMood),
		"商品保真：" + strings.Join(hook.ProductGuardrails, "；") + "。",
		"禁止项：" + strings.Join(hook.NegativeConstraints, "；") + "。",
		"0 至尾帧只执行一个主动作；最后 2 秒完成商品正面定格，并自然靠近原片开场锚点。",
	}
	if strings.TrimSpace(extra) != "" {
		parts = append(parts, "用户补充要求："+strings.TrimSpace(extra)+"。")
	}
	compiled := strings.Join(parts, "\n")
	hash, err := contract.CanonicalJSONHash(struct {
		Prompt string                  `json:"prompt"`
		Beats  []CommercePrerollV2Beat `json:"beats"`
	}{compiled, beats})
	if err != nil {
		return CommercePrerollV2PromptDraft{}, err
	}
	return CommercePrerollV2PromptDraft{
		Revision: workspace.Revision, HookID: hook.ID, DurationSeconds: duration,
		ExtraInstruction: strings.TrimSpace(extra), Beats: beats, PromptSummary: hook.Concept,
		CreativePrompt: compiled, LockedConstraints: append(append([]string{}, hook.ProductGuardrails...), hook.NegativeConstraints...),
		EditMode: "storyboard_compiled", CompiledPrompt: compiled, CompilerVersion: "commerce-preroll-prompt/v3", ContentHash: "sha256:" + hash,
	}, nil
}

type UpdateCommercePrerollV2StoryboardRequest struct {
	ExpectedRevision int64                   `json:"expected_revision"`
	Beats            []CommercePrerollV2Beat `json:"beats"`
}

type UpdateCommercePrerollV2PromptRequest struct {
	ExpectedRevision int64  `json:"expected_revision"`
	CreativePrompt   string `json:"creative_prompt"`
}

func (s Service) UpdateCommercePrerollV2Storyboard(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request UpdateCommercePrerollV2StoryboardRequest) (TaskDetail, error) {
	return s.reviseCommercePrerollV2(ctx, actor, projectID, taskID, request.ExpectedRevision, func(draft *VideoDraft, workspace *CommercePrerollV2Workspace) error {
		if workspace.PromptDraft == nil || len(request.Beats) != 3 {
			return ErrInvalidState
		}
		total := int64(workspace.PromptDraft.DurationSeconds * 1000)
		for index, beat := range request.Beats {
			if beat.StartMS < 0 || beat.EndMS <= beat.StartMS || (index > 0 && beat.StartMS != request.Beats[index-1].EndMS) || strings.TrimSpace(beat.VisualDescription) == "" || strings.TrimSpace(beat.SubjectAction) == "" {
				return fmt.Errorf("commerce storyboard shots must be contiguous and complete")
			}
		}
		if request.Beats[0].StartMS != 0 || request.Beats[len(request.Beats)-1].EndMS != total || strings.TrimSpace(request.Beats[2].ProductState) == "" {
			return fmt.Errorf("commerce storyboard must cover the selected duration and end with a product lockup")
		}
		prompt := *workspace.PromptDraft
		prompt.Beats = append([]CommercePrerollV2Beat{}, request.Beats...)
		lines := make([]string, 0, len(prompt.Beats))
		for _, beat := range prompt.Beats {
			lines = append(lines, fmt.Sprintf("%0.1f-%0.1f秒 %s：%s；主体动作：%s；镜头：%s；场景与光线：%s；商品状态：%s；转入：%s；转出：%s；字幕：%s；音频：%s", float64(beat.StartMS)/1000, float64(beat.EndMS)/1000, beat.Label, beat.VisualDescription, beat.SubjectAction, beat.Camera, beat.SceneAndLighting, beat.ProductState, beat.TransitionIn, beat.TransitionOut, beat.OnScreenText, beat.AudioInstruction))
		}
		prompt.CreativePrompt = strings.Join(lines, "\n")
		prompt.EditMode = "storyboard_compiled"
		if err := sealCommercePrerollV2Prompt(&prompt); err != nil {
			return err
		}
		workspace.PromptDraft = &prompt
		workspace.FirstFrameBatch, workspace.GenerationSpec, workspace.OutputAsset, workspace.AdoptedAsset = nil, nil, nil, nil
		workspace.ActiveStage = CommercePrerollV2StageHookSelected
		draft.Prompt = prompt.CompiledPrompt
		return nil
	}, TaskInProgress)
}

func (s Service) UpdateCommercePrerollV2Prompt(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request UpdateCommercePrerollV2PromptRequest) (TaskDetail, error) {
	if strings.TrimSpace(request.CreativePrompt) == "" || len([]rune(request.CreativePrompt)) > 8000 {
		return TaskDetail{}, fmt.Errorf("creative_prompt is required and must not exceed 8000 characters")
	}
	return s.reviseCommercePrerollV2(ctx, actor, projectID, taskID, request.ExpectedRevision, func(draft *VideoDraft, workspace *CommercePrerollV2Workspace) error {
		if workspace.PromptDraft == nil {
			return ErrInvalidState
		}
		prompt := *workspace.PromptDraft
		prompt.CreativePrompt = strings.TrimSpace(request.CreativePrompt)
		prompt.EditMode = "manual_creative_override"
		if err := sealCommercePrerollV2Prompt(&prompt); err != nil {
			return err
		}
		workspace.PromptDraft = &prompt
		workspace.FirstFrameBatch, workspace.GenerationSpec, workspace.OutputAsset, workspace.AdoptedAsset = nil, nil, nil, nil
		workspace.ActiveStage = CommercePrerollV2StageHookSelected
		draft.Prompt = prompt.CompiledPrompt
		return nil
	}, TaskInProgress)
}

func sealCommercePrerollV2Prompt(prompt *CommercePrerollV2PromptDraft) error {
	prompt.CompiledPrompt = prompt.CreativePrompt + "\n系统锁定约束：" + strings.Join(prompt.LockedConstraints, "；")
	hash, err := contract.CanonicalJSONHash(struct {
		Prompt string                  `json:"prompt"`
		Beats  []CommercePrerollV2Beat `json:"beats"`
	}{prompt.CompiledPrompt, prompt.Beats})
	if err != nil {
		return err
	}
	prompt.ContentHash = "sha256:" + hash
	prompt.CompilerVersion = "commerce-preroll-prompt/v3"
	return nil
}
