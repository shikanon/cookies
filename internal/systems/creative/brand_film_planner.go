package creative

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/provider"
)

const (
	brandBriefPromptVersion   = "brand-brief-analysis/v1"
	brandConceptPromptVersion = "brand-concept-set/v1"
	brandFilmPromptVersion    = "brand-film-plan/v1"
)

type BrandFilmTextGenerator interface {
	GenerateText(context.Context, provider.TextGenerateRequest) (provider.SynchronousResponse, error)
}

var brandFilmHashtagProductPattern = regexp.MustCompile(`#\s*([^#\r\n]{2,48}?)\s*#`)

func reconcileBriefProductAssets(briefText string, candidates []BrandBriefAssetCandidate) []BrandBriefAssetCandidate {
	productNames := briefProductNames(briefText)
	if len(productNames) == 0 {
		return candidates
	}
	products := make([]BrandBriefAssetCandidate, 0, len(productNames))
	nonProducts := make([]BrandBriefAssetCandidate, 0, len(candidates))
	available := make([]BrandBriefAssetCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Role == "product_front" {
			available = append(available, candidate)
		} else {
			nonProducts = append(nonProducts, candidate)
		}
	}
	used := make([]bool, len(available))
	for index, name := range productNames {
		matched := -1
		for candidateIndex, candidate := range available {
			if used[candidateIndex] || isGenericBrandProductLabel(candidate.Label) {
				continue
			}
			label := strings.TrimSpace(candidate.Label)
			if strings.Contains(label, name) || strings.Contains(name, label) {
				matched = candidateIndex
				break
			}
		}
		if matched < 0 {
			for candidateIndex, candidate := range available {
				if !used[candidateIndex] && isGenericBrandProductLabel(candidate.Label) {
					matched = candidateIndex
					break
				}
			}
		}
		candidate := BrandBriefAssetCandidate{
			ID: fmt.Sprintf("asset_product_front_%02d", index+1), Role: "product_front", Label: name,
			SourceLocator: fmt.Sprintf("brief://products/%02d", index+1), RightsStatus: "needs_confirmation",
		}
		if matched >= 0 {
			candidate = available[matched]
			used[matched] = true
			candidate.Label = name
			if candidate.ID == "" || (isGenericBrandProductLabel(available[matched].Label) && index > 0) {
				candidate.ID = fmt.Sprintf("asset_product_front_%02d", index+1)
			}
			if index > 0 && isGenericBrandProductLabel(available[matched].Label) {
				candidate.AssetRef, candidate.FixtureURI = nil, ""
				candidate.UserConfirmed, candidate.RightsStatus = false, "needs_confirmation"
			}
		}
		products = append(products, candidate)
	}
	return append(products, nonProducts...)
}

func briefProductNames(briefText string) []string {
	names := make([]string, 0, 4)
	add := func(value string) {
		value = strings.TrimSpace(strings.Trim(value, "#：:，,。.;；【】[]（）()"))
		if !looksLikeBrandProductName(value) {
			return
		}
		identity := normalizeBrandProductIdentity(value)
		for _, current := range names {
			if normalizeBrandProductIdentity(current) == identity {
				return
			}
		}
		names = append(names, value)
	}
	for _, line := range strings.Split(strings.ReplaceAll(briefText, "\r\n", "\n"), "\n") {
		matches := brandFilmHashtagProductPattern.FindAllStringSubmatch(line, -1)
		if len(matches) == 0 {
			if strings.Contains(line, "娇兰") {
				add(line)
			}
			continue
		}
		for _, match := range matches {
			add(match[1])
		}
	}
	return names
}

func looksLikeBrandProductName(value string) bool {
	if value == "" || len([]rune(value)) > 20 || strings.ContainsAny(value, " \t，,。.;；：:、“”'‘’/") {
		return false
	}
	for _, keyword := range []string{"蜜", "水", "精华", "面霜", "乳霜", "粉底", "口红", "唇膏", "香水", "眼霜", "面膜", "套组"} {
		if strings.Contains(value, keyword) {
			return true
		}
	}
	return false
}

func normalizeBrandProductIdentity(value string) string {
	value = strings.ReplaceAll(strings.ReplaceAll(value, " ", ""), "\t", "")
	value = strings.TrimPrefix(value, "法国娇兰")
	return strings.TrimPrefix(value, "娇兰")
}

func isGenericBrandProductLabel(value string) bool {
	value = strings.TrimSpace(value)
	return value == "" || value == "商品正面图" || value == "产品正面图" || value == "商品参考图"
}

type BrandFilmPlanner interface {
	AnalyzeBrief(context.Context, contract.ActorContext, contract.ProjectContext, BrandFilmSourceSnapshot, int64, time.Time) (BrandBriefAnalysisVersion, error)
	GenerateConcepts(context.Context, contract.ActorContext, contract.ProjectContext, BrandFilmSourceSnapshot, BrandBriefAnalysisVersion, int64, time.Time) (BrandCreativeConceptSet, error)
	GenerateFilmPlan(context.Context, contract.ActorContext, contract.ProjectContext, BrandFilmSourceSnapshot, BrandBriefAnalysisVersion, BrandCreativeConcept, int64, time.Time) (BrandFilmPlanVersion, error)
}

type DeterministicBrandFilmPlanner struct{}

func (DeterministicBrandFilmPlanner) AnalyzeBrief(_ context.Context, _ contract.ActorContext, _ contract.ProjectContext, source BrandFilmSourceSnapshot, revision int64, now time.Time) (BrandBriefAnalysisVersion, error) {
	if source.SourceKind != "" && source.SourceKind != "fixture" {
		mandatory := append([]string{}, source.Mandatory...)
		if len(mandatory) == 0 {
			mandatory = []string{"商品与品牌识别必须与已确认素材一致"}
		}
		prohibited := append([]string{}, source.Prohibited...)
		if len(prohibited) == 0 {
			prohibited = []string{"不得编造 Brief 未支持的功效、价格或促销信息"}
		}
		message := strings.TrimSpace(source.CoreMessage)
		if message == "" {
			message = strings.TrimSpace(source.ProductName)
		}
		return BrandBriefAnalysisVersion{
			Revision: revision, Summary: fmt.Sprintf("围绕%s建立可追溯的品牌影片表达，所有事实和资产仍需人工确认。", source.ProductName),
			Audience: source.Audience, CoreMessage: message,
			SellingPoints: []BrandBriefFact{{Text: message, Locator: "creative-intake://" + source.IntakeID + "#core_message", Confidence: 1, Status: "brief_fact"}},
			Mandatory:     mandatory, Prohibited: prohibited,
			ImageRequirements: []string{"使用已确认的商品正面图与品牌 Logo", "保持商品包装、文字和比例真实"},
			VideoRequirements: []string{fmt.Sprintf("%s · %d 秒 · %s", source.Channel, source.Duration, source.AspectRatio), "品牌定格需保留清晰识别时间"},
			VoiceDirection:    "克制、可信、清晰，品牌名与产品名按确认读法口播。",
			AssetCandidates:   append([]BrandBriefAssetCandidate{}, source.AssetCandidates...),
			Uncertainties:     []string{"商品正面图、Logo 与声音权利需在生成前确认"},
			ModelAlias:        "fixture.deterministic", ModelVersion: "generic-brand-film-v1", PromptVersion: brandBriefPromptVersion, CreatedAt: now,
		}, nil
	}
	return BrandBriefAnalysisVersion{
		Revision:    revision,
		Summary:     "娇兰 25X 蜂皇水面向关注补水修护与轻盈肤感的人群，以水感、蜂巢与暖金光影建立高端品牌记忆。",
		Audience:    "关注补水、修护、屏障护理与轻盈肤感的都市护肤人群。",
		CoreMessage: "轻盈如水的使用体验，承载娇兰黑蜂修护科技与高端品牌质感。",
		SellingPoints: []BrandBriefFact{
			{Text: "精华水质地轻盈不黏腻", Locator: "fixture://briefs/guerlain-25x-bee-water-v1#pages=8-12", Confidence: 0.96, Status: "brief_fact"},
			{Text: "强调补水、修护与屏障护理", Locator: "fixture://briefs/guerlain-25x-bee-water-v1#pages=8-12", Confidence: 0.94, Status: "needs_confirmation"},
			{Text: "含微囊蜂王浆与黑蜂修护科技", Locator: "fixture://briefs/guerlain-25x-bee-water-v1#pages=8-12", Confidence: 0.92, Status: "brief_fact"},
			{Text: "适合湿敷与日常全脸护理", Locator: "fixture://briefs/guerlain-25x-bee-water-v1#pages=8-12", Confidence: 0.9, Status: "brief_fact"},
		},
		Mandatory:         []string{"商品瓶型、颜色、标签与比例保持真实", "书面使用“25X蜂皇水”，口播使用“二十五倍蜂皇水”", "包含湿敷或全脸使用方式", "功效表达绑定 Brief 事实"},
		Prohibited:        []string{"不得编造医学或绝对化功效", "不得生成错误 Logo、包装文字、价格或促销信息", "不得混入其他娇兰产品卖点"},
		ImageRequirements: []string{"优先使用无水印商品正面图", "保留琥珀金瓶身、黑色瓶盖和原始标签比例", "Logo 与包装文字进入生产前必须人工确认"},
		VideoRequirements: []string{"抖音 9:16，15 秒", "使用水感微距、暖金蜂巢与克制运镜", "结尾至少保留 2 秒稳定商品定格"},
		VoiceDirection:    "温柔、克制的年轻成熟女声，中低语速，品牌名与“二十五倍蜂皇水”咬字清晰。",
		AssetCandidates: []BrandBriefAssetCandidate{
			{ID: "asset_product_front", Role: "product_front", Label: "25X 蜂皇水正面图", SourceLocator: "fixture://briefs/guerlain-25x-bee-water-v1#page=9&image=IM135", FixtureURI: "/assets/guerlain-25x-bee-water-product-front.png", RightsStatus: "needs_confirmation"},
			{ID: "asset_brand_logo", Role: "logo", Label: "娇兰 Logo", SourceLocator: "fixture://briefs/guerlain-25x-bee-water-v1#page=1&image=IM17", FixtureURI: "/assets/guerlain-logo.png", RightsStatus: "needs_confirmation"},
		},
		Uncertainties: []string{"98% 天然来源成分的传播脚注与适用范围需要人工确认", "统一口播音色的 Voice ID 与授权尚未提供"},
		ModelAlias:    "fixture.deterministic", ModelVersion: "guerlain-brand-v1", PromptVersion: brandBriefPromptVersion, CreatedAt: now,
	}, nil
}

func (DeterministicBrandFilmPlanner) GenerateConcepts(_ context.Context, _ contract.ActorContext, _ contract.ProjectContext, source BrandFilmSourceSnapshot, analysis BrandBriefAnalysisVersion, revision int64, now time.Time) (BrandCreativeConceptSet, error) {
	if source.SourceKind != "" && source.SourceKind != "fixture" {
		product := source.ProductName
		return BrandCreativeConceptSet{
			Revision: revision, AnalysisRevision: analysis.Revision,
			Candidates: []BrandCreativeConcept{
				{ID: "concept_truth_in_detail", Title: "真实细节", OneLiner: "让真实细节成为品牌价值最有力的证明。", StoryMechanism: "从一个可验证细节逐步展开到完整品牌主张。", BrandEntrance: "产品作为证据主体自然进入画面。", VisualLanguage: []string{"真实材质", "克制微距", "精确构图"}, SoundIdea: "低饱和环境声与简洁节拍", BriefRationale: analysis.CoreMessage, Risk: "不得把视觉演绎误写成未经证实的产品事实。"},
				{ID: "concept_human_resonance", Title: "人的感受", OneLiner: "从真实人物感受出发，让品牌主张被看见和相信。", StoryMechanism: "人物情绪转变承接产品价值，不使用夸张前后对比。", BrandEntrance: product + "进入人物真实使用场景。", VisualLanguage: []string{"自然人物", "柔和光线", "留白叙事"}, SoundIdea: "呼吸感音乐与克制旁白", BriefRationale: analysis.CoreMessage, Risk: "人物表达不能替代事实证据。"},
				{ID: "concept_symbolic_world", Title: "品牌意象", OneLiner: "将核心主张转译为一个可持续记忆的视觉意象。", StoryMechanism: "视觉意象贯穿开场、产品进入与品牌定格。", BrandEntrance: "产品由意象变化自然显形。", VisualLanguage: []string{"抽象意象", "品牌色控制", "英雄定格"}, SoundIdea: "标志性声音动机逐步建立", BriefRationale: analysis.CoreMessage, Risk: "意象不能遮挡商品与 Logo 识别。"},
			},
			ModelAlias: "fixture.deterministic", ModelVersion: "generic-brand-film-v1", PromptVersion: brandConceptPromptVersion, CreatedAt: now,
		}, nil
	}
	return BrandCreativeConceptSet{
		Revision: revision, AnalysisRevision: analysis.Revision,
		Candidates: []BrandCreativeConcept{
			{ID: "concept_hive_awaken", Title: "蜂巢苏醒", OneLiner: "一滴水唤醒金色蜂巢，让修护能量汇入肌肤。", StoryMechanism: "从微观蜂巢能量进入产品，再落到真实湿敷动作。", BrandEntrance: "第 5 秒由蜂巢光影折射出产品正面。", VisualLanguage: []string{"暖金蜂巢", "水滴微距", "琥珀瓶身"}, SoundIdea: "细微水滴与低频弦乐渐起", BriefRationale: "对应黑蜂科技、水感质地与湿敷使用方式。", Risk: "蜂巢元素不可盖过商品识别。"},
			{ID: "concept_light_on_skin", Title: "晨光入肤", OneLiner: "清晨的一层轻盈水光，让肌肤与一天同时苏醒。", StoryMechanism: "以都市清晨的时间推进展示轻盈、湿敷与稳定光泽。", BrandEntrance: "产品从晨光中的梳妆台自然进入人物动作。", VisualLanguage: []string{"柔和晨光", "肌肤水光", "奢华留白"}, SoundIdea: "呼吸感钢琴与轻柔环境声", BriefRationale: "服务轻盈肤感和日常护理场景。", Risk: "人物肌肤效果不得被表达成即时医学功效。"},
			{ID: "concept_water_sculpture", Title: "水之雕刻", OneLiner: "流动的水被金色光线雕刻成娇兰瓶身。", StoryMechanism: "用抽象水体逐步显形为产品，最后回到真实使用。", BrandEntrance: "产品是视觉变化的结果而非后置贴片。", VisualLanguage: []string{"水体雕塑", "金色折射", "黑色高光"}, SoundIdea: "水流声与克制电子脉冲", BriefRationale: "把精华水的轻盈质感转译为高级视觉记忆。", Risk: "生成阶段需严控瓶身变形与标签错误。"},
		},
		ModelAlias: "fixture.deterministic", ModelVersion: "guerlain-brand-v1", PromptVersion: brandConceptPromptVersion, CreatedAt: now,
	}, nil
}

func (DeterministicBrandFilmPlanner) GenerateFilmPlan(_ context.Context, _ contract.ActorContext, _ contract.ProjectContext, source BrandFilmSourceSnapshot, analysis BrandBriefAnalysisVersion, concept BrandCreativeConcept, revision int64, now time.Time) (BrandFilmPlanVersion, error) {
	if source.SourceKind != "" && source.SourceKind != "fixture" {
		profile, err := ResolveBrandFilmDurationProfile(source.Duration)
		if err != nil {
			return BrandFilmPlanVersion{}, err
		}
		shots := make([]BrandFilmShot, 0, profile.ShotCount)
		base, remainder, start := source.Duration/profile.ShotCount, source.Duration%profile.ShotCount, 0
		for index := 0; index < profile.ShotCount; index++ {
			duration := base
			if index < remainder {
				duration++
			}
			end := start + duration
			purpose := "展开品牌主张"
			referenceRole := "composition"
			if index == 0 {
				purpose, referenceRole = "建立品牌世界与注意力", "style"
			}
			if index == profile.ShotCount-1 {
				purpose, referenceRole = "商品与品牌定格", "required_identity"
			}
			shots = append(shots, BrandFilmShot{
				ID: fmt.Sprintf("shot_%02d", index+1), Order: index + 1, StartSecond: start, EndSecond: end,
				Purpose: purpose, Visual: fmt.Sprintf("以%s的视觉语言呈现%s，第 %d 段保持商品与品牌事实准确。", concept.Title, source.ProductName, index+1),
				Action: "使用克制且连续的主体动作推进叙事。", Camera: "稳定构图配合缓慢推进", Lighting: "遵循品牌色与真实材质的受控光线",
				Voiceover: analysis.CoreMessage, OnScreenText: source.ProductName, ReferenceRole: referenceRole,
				ContinuityNotes: "商品、Logo、人物与场景连续性必须遵循已确认 Brief。",
			})
			start = end
		}
		return BrandFilmPlanVersion{
			Revision: revision, MasterDurationMS: source.Duration * 1000, ConceptID: concept.ID, Title: "《" + concept.Title + "》", StorySummary: concept.OneLiner,
			VoiceDirection: analysis.VoiceDirection, MusicDirection: concept.SoundIdea, Shots: shots,
			ModelAlias: "fixture.deterministic", ModelVersion: "generic-brand-film-v1", PromptVersion: brandFilmPromptVersion, CreatedAt: now,
		}, nil
	}
	shots := []BrandFilmShot{
		{ID: "shot_01", Order: 1, StartSecond: 0, EndSecond: 5, Purpose: "建立品牌世界并完成产品进入", Visual: "暗金背景中一滴清水落入蜂巢纹理，光线沿六边形苏醒并汇聚出娇兰 25X 蜂皇水正面瓶身。", Action: "水滴扩散为金色波纹，瓶身从水雾中稳定显形。", Camera: "微距推进后转为中近景轻微环绕", Lighting: "低照度暖金轮廓光与瓶身侧光", Voiceover: "当自然的修护能量被轻盈唤醒，娇兰二十五倍蜂皇水。", OnScreenText: "娇兰 25X蜂皇水", ReferenceRole: "required_identity", ContinuityNotes: "瓶型、黑色瓶盖、标签比例必须保持真实。"},
		{ID: "shot_02", Order: 2, StartSecond: 5, EndSecond: 10, Purpose: "展示质地与湿敷体验", Visual: "轻盈水感掠过肌肤，人物将浸润化妆棉贴于面颊，金色波纹在肌肤边缘自然呼应。", Action: "完成一次自然湿敷并轻微转头，不做前后对比。", Camera: "水感微距切至稳定面部近景", Lighting: "柔和晨光与克制金色反射", Voiceover: "轻盈不黏腻，温柔补水修护，让护理回到柔润与从容。", ReferenceRole: "composition", ContinuityNotes: "人物、肤色与产品方向连续，不表现夸张即时功效。"},
		{ID: "shot_03", Order: 3, StartSecond: 10, EndSecond: 15, Purpose: "品牌定格与行动收束", Visual: "产品正面在暖金蜂巢背景前稳定定格，品牌与产品信息留白清晰。", Action: "仅保留细微水光和金色光线流动。", Camera: "固定商品英雄镜头", Lighting: "高端暖金主光", Voiceover: "法国娇兰，二十五倍蜂皇水。", OnScreenText: "法国娇兰 · 25X蜂皇水", ReferenceRole: "required_identity", ContinuityNotes: "不新增价格、促销或错误文字。"},
	}
	return BrandFilmPlanVersion{
		Revision: revision, MasterDurationMS: 15000, ConceptID: concept.ID, Title: "《" + concept.Title + "》", StorySummary: concept.OneLiner,
		VoiceDirection: "温柔、克制的年轻成熟女声，中低语速。", MusicDirection: concept.SoundIdea, Shots: shots,
		ModelAlias: "fixture.deterministic", ModelVersion: "guerlain-brand-v1", PromptVersion: brandFilmPromptVersion, CreatedAt: now,
	}, nil
}

type ModelBrandFilmPlanner struct {
	Text       BrandFilmTextGenerator
	ModelAlias string
}

type FallbackBrandFilmPlanner struct {
	Primary          BrandFilmPlanner
	Fallback         BrandFilmPlanner
	OnPrimaryFailure func(error)
}

func (p FallbackBrandFilmPlanner) AnalyzeBrief(ctx context.Context, actor contract.ActorContext, project contract.ProjectContext, source BrandFilmSourceSnapshot, revision int64, now time.Time) (BrandBriefAnalysisVersion, error) {
	value, err := p.Primary.AnalyzeBrief(ctx, actor, project, source, revision, now)
	if err == nil {
		return value, nil
	}
	if p.OnPrimaryFailure != nil {
		p.OnPrimaryFailure(err)
	}
	return p.Fallback.AnalyzeBrief(ctx, actor, project, source, revision, now)
}

func (p FallbackBrandFilmPlanner) GenerateConcepts(ctx context.Context, actor contract.ActorContext, project contract.ProjectContext, source BrandFilmSourceSnapshot, analysis BrandBriefAnalysisVersion, revision int64, now time.Time) (BrandCreativeConceptSet, error) {
	value, err := p.Primary.GenerateConcepts(ctx, actor, project, source, analysis, revision, now)
	if err == nil {
		return value, nil
	}
	if p.OnPrimaryFailure != nil {
		p.OnPrimaryFailure(err)
	}
	return p.Fallback.GenerateConcepts(ctx, actor, project, source, analysis, revision, now)
}

func (p FallbackBrandFilmPlanner) GenerateFilmPlan(ctx context.Context, actor contract.ActorContext, project contract.ProjectContext, source BrandFilmSourceSnapshot, analysis BrandBriefAnalysisVersion, concept BrandCreativeConcept, revision int64, now time.Time) (BrandFilmPlanVersion, error) {
	value, err := p.Primary.GenerateFilmPlan(ctx, actor, project, source, analysis, concept, revision, now)
	if err == nil {
		return value, nil
	}
	if p.OnPrimaryFailure != nil {
		p.OnPrimaryFailure(err)
	}
	return p.Fallback.GenerateFilmPlan(ctx, actor, project, source, analysis, concept, revision, now)
}

func brandPlannerActor(actor contract.ActorContext) contract.ActorContext {
	return contract.ActorContext{OrganizationID: actor.OrganizationID, Principal: actor.Principal, Scopes: []contract.Scope{provider.ScopeTextGenerate}}
}

func brandFilmSourceInvocationToken(source BrandFilmSourceSnapshot) string {
	for _, value := range []string{
		source.FixtureHash,
		source.DirectionContentHash,
		source.BrandBriefContentHash,
		source.InputIdentityHash,
		source.StrategyPackageHash,
	} {
		token := strings.TrimPrefix(strings.TrimSpace(value), "sha256:")
		if len(token) >= 12 {
			return token[:12]
		}
	}
	payload, _ := json.Marshal(source)
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("%x", digest[:6])
}

func (p ModelBrandFilmPlanner) AnalyzeBrief(ctx context.Context, actor contract.ActorContext, project contract.ProjectContext, source BrandFilmSourceSnapshot, revision int64, now time.Time) (BrandBriefAnalysisVersion, error) {
	if p.Text == nil || strings.TrimSpace(p.ModelAlias) == "" {
		return BrandBriefAnalysisVersion{}, fmt.Errorf("brand film model planner is not configured")
	}
	raw, err := json.Marshal(source)
	if err != nil {
		return BrandBriefAnalysisVersion{}, err
	}
	response, err := p.Text.GenerateText(ctx, provider.TextGenerateRequest{
		Actor: brandPlannerActor(actor), Project: project, ModelAlias: p.ModelAlias,
		InvocationKey: contract.IdempotencyKey(fmt.Sprintf("brand-brief-%s-%d", brandFilmSourceInvocationToken(source), revision)),
		Messages: []provider.TextMessage{
			{Role: provider.TextRoleSystem, Content: "你是品牌广告 Brief 分析师。只分析输入中明确出现的品牌与产品，不混入其他项目。只输出 JSON。事实必须保留 locator；不确定功效标记 needs_confirmation。每个明确出现的商品必须建立一条独立 product_front 素材候选，label 使用完整商品名；品牌 Logo 单独建立一条 logo 候选。不得把多个商品合并成‘商品正面图’。"},
			{Role: provider.TextRoleUser, Content: "请提炼摘要、受众、核心信息、卖点、必须项、禁用项、图片/视频要求、统一口播方向和不确定项，并逐个列出 Brief 中出现的商品素材候选与品牌 Logo。INPUT=" + string(raw)},
		}, OutputJSONSchema: brandBriefAnalysisSchema,
	})
	if err != nil {
		return BrandBriefAnalysisVersion{}, err
	}
	var value BrandBriefAnalysisVersion
	if err := decodeBrandStructured(response, &value); err != nil {
		return BrandBriefAnalysisVersion{}, err
	}
	value.Revision, value.Confirmed, value.ConfirmedAt = revision, false, nil
	value.ModelAlias, value.ModelVersion, value.RouteRevisionID = p.ModelAlias, response.ModelVersion, response.RouteRevisionID
	value.PromptVersion, value.CreatedAt = brandBriefPromptVersion, now
	if len(value.AssetCandidates) > 0 {
		value.AssetCandidates = mergeKnownBrandAssets(value.AssetCandidates, source.AssetCandidates)
	} else if len(source.AssetCandidates) > 0 {
		value.AssetCandidates = append([]BrandBriefAssetCandidate{}, source.AssetCandidates...)
	} else {
		fallback, _ := (DeterministicBrandFilmPlanner{}).AnalyzeBrief(ctx, actor, project, source, revision, now)
		value.AssetCandidates = fallback.AssetCandidates
	}
	return value, value.Validate()
}

func mergeKnownBrandAssets(generated, known []BrandBriefAssetCandidate) []BrandBriefAssetCandidate {
	merged := append([]BrandBriefAssetCandidate{}, generated...)
	for _, asset := range known {
		if asset.AssetRef == nil && strings.TrimSpace(asset.FixtureURI) == "" {
			continue
		}
		matched := -1
		for index := range merged {
			if merged[index].Role == asset.Role && merged[index].AssetRef == nil && strings.TrimSpace(merged[index].FixtureURI) == "" {
				matched = index
				break
			}
		}
		if matched < 0 {
			merged = append(merged, asset)
			continue
		}
		label := strings.TrimSpace(merged[matched].Label)
		merged[matched] = asset
		if label != "" {
			merged[matched].Label = label
		}
	}
	return merged
}

func (p ModelBrandFilmPlanner) GenerateConcepts(ctx context.Context, actor contract.ActorContext, project contract.ProjectContext, source BrandFilmSourceSnapshot, analysis BrandBriefAnalysisVersion, revision int64, now time.Time) (BrandCreativeConceptSet, error) {
	input, _ := json.Marshal(struct {
		Source   BrandFilmSourceSnapshot   `json:"source"`
		Analysis BrandBriefAnalysisVersion `json:"analysis"`
	}{source, analysis})
	response, err := p.Text.GenerateText(ctx, provider.TextGenerateRequest{
		Actor: brandPlannerActor(actor), Project: project, ModelAlias: p.ModelAlias,
		InvocationKey: contract.IdempotencyKey(fmt.Sprintf("brand-concepts-%s-%d", brandFilmSourceInvocationToken(source), revision)),
		Messages: []provider.TextMessage{
			{Role: provider.TextRoleSystem, Content: fmt.Sprintf("你是品牌广告创意总监。输出 3 个叙事机制明显不同的 %d 秒 %s 方向，不生成镜头表，不编造 Brief 事实，只输出 JSON。", source.Duration, source.AspectRatio)},
			{Role: provider.TextRoleUser, Content: "根据已确认 Brief 生成创意方向。INPUT=" + string(input)},
		}, OutputJSONSchema: brandConceptSetSchema,
	})
	if err != nil {
		return BrandCreativeConceptSet{}, err
	}
	var value BrandCreativeConceptSet
	if err := decodeBrandStructured(response, &value); err != nil {
		return BrandCreativeConceptSet{}, err
	}
	value.Revision, value.AnalysisRevision = revision, analysis.Revision
	for index := range value.Candidates {
		value.Candidates[index].ID = fmt.Sprintf("concept_%02d", index+1)
		value.Candidates[index].Selected, value.Candidates[index].Confirmed = false, false
	}
	value.ModelAlias, value.ModelVersion, value.RouteRevisionID = p.ModelAlias, response.ModelVersion, response.RouteRevisionID
	value.PromptVersion, value.CreatedAt = brandConceptPromptVersion, now
	return value, value.Validate()
}

func (p ModelBrandFilmPlanner) GenerateFilmPlan(ctx context.Context, actor contract.ActorContext, project contract.ProjectContext, source BrandFilmSourceSnapshot, analysis BrandBriefAnalysisVersion, concept BrandCreativeConcept, revision int64, now time.Time) (BrandFilmPlanVersion, error) {
	input, _ := json.Marshal(struct {
		Source   BrandFilmSourceSnapshot   `json:"source"`
		Analysis BrandBriefAnalysisVersion `json:"analysis"`
		Concept  BrandCreativeConcept      `json:"concept"`
	}{source, analysis, concept})
	response, err := p.Text.GenerateText(ctx, provider.TextGenerateRequest{
		Actor: brandPlannerActor(actor), Project: project, ModelAlias: p.ModelAlias,
		InvocationKey: contract.IdempotencyKey(fmt.Sprintf("brand-film-plan-%s-%d", brandFilmSourceInvocationToken(source), revision)),
		Messages: []provider.TextMessage{
			{Role: provider.TextRoleSystem, Content: fmt.Sprintf("你是品牌广告导演。输出可编辑的 %d 秒、9:16 剧本分镜。镜头必须从 0 秒连续覆盖到 %d 秒且固定为 %d 个，每个镜头 4～15 秒；用户编辑镜头表而不是模型 Prompt。只输出 JSON。", source.Duration, source.Duration, (source.Duration+4)/5)},
			{Role: provider.TextRoleUser, Content: "基于已确认 Brief 与创意方向生成剧本、旁白和镜头表。INPUT=" + string(input)},
		}, OutputJSONSchema: brandFilmPlanOutputSchema(source.Duration),
	})
	if err != nil {
		return BrandFilmPlanVersion{}, err
	}
	var value BrandFilmPlanVersion
	if err := decodeBrandStructured(response, &value); err != nil {
		return BrandFilmPlanVersion{}, err
	}
	value.Revision, value.MasterDurationMS, value.ConceptID, value.Confirmed, value.ConfirmedAt = revision, source.Duration*1000, concept.ID, false, nil
	for index := range value.Shots {
		value.Shots[index].ID, value.Shots[index].Order = fmt.Sprintf("shot_%02d", index+1), index+1
	}
	value.ModelAlias, value.ModelVersion, value.RouteRevisionID = p.ModelAlias, response.ModelVersion, response.RouteRevisionID
	value.PromptVersion, value.CreatedAt = brandFilmPromptVersion, now
	return value, value.Validate()
}

func decodeBrandStructured(response provider.SynchronousResponse, target any) error {
	raw := response.StructuredOutput
	if len(raw) == 0 {
		raw = json.RawMessage(response.Text)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode brand film planner output: %w", err)
	}
	return nil
}

var brandBriefAnalysisSchema = json.RawMessage(`{
  "type":"object","additionalProperties":false,
  "required":["summary","audience","core_message","selling_points","mandatory_elements","prohibited_claims","image_requirements","video_requirements","voice_direction","asset_candidates","uncertainties"],
  "properties":{
    "summary":{"type":"string"},"audience":{"type":"string"},"core_message":{"type":"string"},
    "selling_points":{"type":"array","minItems":3,"items":{"type":"object","additionalProperties":false,"required":["text","locator","confidence","status"],"properties":{"text":{"type":"string"},"locator":{"type":"string"},"confidence":{"type":"number","minimum":0,"maximum":1},"status":{"enum":["brief_fact","needs_confirmation"]}}}},
    "mandatory_elements":{"type":"array","minItems":1,"items":{"type":"string"}},"prohibited_claims":{"type":"array","minItems":1,"items":{"type":"string"}},
    "image_requirements":{"type":"array","items":{"type":"string"}},"video_requirements":{"type":"array","items":{"type":"string"}},"voice_direction":{"type":"string"},
    "asset_candidates":{"type":"array","minItems":2,"items":{"type":"object","additionalProperties":false,"required":["id","role","label","source_locator","rights_status","user_confirmed"],"properties":{"id":{"type":"string"},"role":{"enum":["product_front","logo"]},"label":{"type":"string"},"source_locator":{"type":"string"},"rights_status":{"const":"needs_confirmation"},"user_confirmed":{"const":false}}}},
    "uncertainties":{"type":"array","items":{"type":"string"}}
  }
}`)

var brandConceptSetSchema = json.RawMessage(`{
  "type":"object","additionalProperties":false,"required":["candidates"],
  "properties":{"candidates":{"type":"array","minItems":3,"maxItems":3,"items":{"type":"object","additionalProperties":false,"required":["title","one_liner","story_mechanism","brand_entrance","visual_language","sound_idea","brief_rationale","risk"],"properties":{"title":{"type":"string"},"one_liner":{"type":"string"},"story_mechanism":{"type":"string"},"brand_entrance":{"type":"string"},"visual_language":{"type":"array","items":{"type":"string"}},"sound_idea":{"type":"string"},"brief_rationale":{"type":"string"},"risk":{"type":"string"}}}}}
}`)

var brandFilmPlanSchema = json.RawMessage(`{
  "type":"object","additionalProperties":false,"required":["title","story_summary","voice_direction","music_direction","shots"],
  "properties":{"title":{"type":"string"},"story_summary":{"type":"string"},"voice_direction":{"type":"string"},"music_direction":{"type":"string"},"shots":{"type":"array","minItems":3,"items":{"type":"object","additionalProperties":false,"required":["start_second","end_second","purpose","visual","action","camera","lighting","voiceover","on_screen_text","reference_role","continuity_notes"],"properties":{"start_second":{"type":"integer","minimum":0},"end_second":{"type":"integer","minimum":1,"maximum":15},"purpose":{"type":"string"},"visual":{"type":"string"},"action":{"type":"string"},"camera":{"type":"string"},"lighting":{"type":"string"},"voiceover":{"type":"string"},"on_screen_text":{"type":"string"},"reference_role":{"type":"string"},"continuity_notes":{"type":"string"}}}}}
}`)

func brandFilmPlanOutputSchema(durationSeconds int) json.RawMessage {
	if durationSeconds < 1 {
		durationSeconds = 15
	}
	return json.RawMessage(strings.Replace(
		string(brandFilmPlanSchema),
		`"end_second":{"type":"integer","minimum":1,"maximum":15}`,
		fmt.Sprintf(`"end_second":{"type":"integer","minimum":1,"maximum":%d}`, durationSeconds),
		1,
	))
}
