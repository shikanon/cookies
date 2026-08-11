package creative

import (
	"context"
	"fmt"
	"strings"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type CreateCommercePrerollV2WorkspaceRequest struct {
	SourceVideo     contract.AssetVersionRef `json:"source_video"`
	RightsConfirmed bool                     `json:"rights_confirmed"`
	DurationSeconds int                      `json:"duration_seconds,omitempty"`
	Channel         CreativeChannel          `json:"channel,omitempty"`
}

func (s Service) CreateCommercePrerollV2Workspace(ctx context.Context, rc contract.RequestContext, projectID contract.ProjectID, key contract.IdempotencyKey, request CreateCommercePrerollV2WorkspaceRequest) (TaskDetail, error) {
	if request.SourceVideo.Validate() != nil || !request.RightsConfirmed {
		return TaskDetail{}, fmt.Errorf("a valid source_video and explicit rights confirmation are required")
	}
	if request.DurationSeconds == 0 {
		request.DurationSeconds = 8
	}
	if request.DurationSeconds < 6 || request.DurationSeconds > 10 {
		return TaskDetail{}, fmt.Errorf("duration_seconds must be between 6 and 10")
	}
	if request.Channel == "" {
		request.Channel = ChannelDouyin
	}
	if request.Channel != ChannelDouyin && request.Channel != ChannelKuaishou {
		return TaskDetail{}, fmt.Errorf("unsupported commerce preroll channel")
	}
	intake, err := s.CreateIntake(ctx, rc, projectID, key, CreateIntakeRequest{
		Source: IntakeSourceManual, Format: FormatVideo, PerformanceMode: PerformanceModeCommercePreroll,
		Channel: request.Channel, Objective: "为已完成的电商正片生成独立 6-10 秒前贴", Audience: "电商短视频观众",
		CoreMessage: "严格依据原视频商品事实与画面风格生成前贴", CallToAction: "继续观看",
		Concept: "原视频理解驱动的电商前贴", Tone: []string{"与原片连续"}, VisualKeywords: []string{"商品保真"},
		Mandatory: []string{}, Prohibited: []string{"不得虚构卖点", "不得改变商品外观与 Logo"},
		CreativeRoutes: []CreativeRouteSnapshot{{
			RouteID: ManualCommercePrerollV2RouteID, RouteType: PerformanceModeCommercePreroll, VideoPurpose: "performance",
			Channels: []string{string(request.Channel)}, Reason: "source-video-driven commerce preroll",
			TargetDurationSeconds: request.DurationSeconds, AspectRatio: "9:16", Resolution: "720p",
			SourceAssetRefs: []contract.AssetVersionRef{request.SourceVideo}, RequiresHumanConfirmation: true,
		}},
		ManualCommercePrerollV2: &ManualCommercePrerollV2Input{SourceVideo: request.SourceVideo, SourceVideoRights: RightsConfirmed},
	})
	if err != nil {
		return TaskDetail{}, err
	}
	if existing, existingErr := s.taskForIntake(ctx, rc.Actor, projectID, intake.ID); existingErr == nil {
		return existing, nil
	} else if existingErr != ErrNotFound {
		return TaskDetail{}, existingErr
	}
	task, err := s.CreateVideoTask(ctx, rc.Actor, projectID, intake.ID, CreateVideoTaskRequest{
		SelectedRouteID: ManualCommercePrerollV2RouteID, Channel: request.Channel, SourceVideo: request.SourceVideo,
		Concept: "原视频理解驱动的电商前贴", Prompt: "等待原视频理解", CallToAction: "继续观看",
		Mandatory: []string{}, Prohibited: []string{"不得虚构卖点", "不得改变商品外观与 Logo"}, ConfirmRoute: true,
	})
	if err != nil {
		return TaskDetail{}, err
	}
	return s.GetCommercePrerollV2Workspace(ctx, rc.Actor, projectID, task.ID)
}

func (s Service) GetCommercePrerollV2Workspace(
	ctx context.Context,
	actor contract.ActorContext,
	projectID contract.ProjectID,
	taskID string,
) (TaskDetail, error) {
	if s.Repository == nil || s.Projects == nil {
		return TaskDetail{}, fmt.Errorf("creative commerce preroll V2 dependencies are incomplete")
	}
	if !actor.HasScope(ScopeRead) {
		return TaskDetail{}, fmt.Errorf("%s scope is required", ScopeRead)
	}
	if strings.TrimSpace(taskID) == "" {
		return TaskDetail{}, fmt.Errorf("task_id is required")
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return TaskDetail{}, err
	}
	detail, err := s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
	if err != nil {
		return TaskDetail{}, err
	}
	if detail.Task.PerformanceMode != PerformanceModeCommercePreroll || detail.VideoDraft.CommercePrerollV2 == nil {
		return TaskDetail{}, ErrNotFound
	}
	draftCopy := *detail.VideoDraft
	workspaceCopy := *detail.VideoDraft.CommercePrerollV2
	upgradeCommercePrerollV2Workspace(&workspaceCopy)
	draftCopy.CommercePrerollV2 = &workspaceCopy
	detail.VideoDraft = &draftCopy
	return detail, nil
}

func upgradeCommercePrerollV2Workspace(workspace *CommercePrerollV2Workspace) {
	if workspace == nil {
		return
	}
	if workspace.ProductReferenceBatch == nil && workspace.ProductReference != nil && workspace.ProductReference.Asset != nil {
		candidate := CommercePrerollV2ProductReferenceCandidate{ID: "legacy_product_reference", SourceKind: "video_extract", Label: "原视频商品画面", Frame: *workspace.ProductReference, Scores: CommercePrerollV2ProductFrameCandidate{ID: "legacy_product_reference", TimestampMS: workspace.ProductReference.TimestampMS, Overall: 1}}
		workspace.ProductReferenceBatch = &CommercePrerollV2ProductReferenceBatch{ID: workspace.TaskID + "_product_references", Revision: workspace.Revision, Candidates: []CommercePrerollV2ProductReferenceCandidate{candidate}, SelectedID: candidate.ID}
	}
	if workspace.HookBatch != nil {
		enriched := buildCommercePrerollV2Hooks(workspace.Analysis.Content)
		byID := make(map[string]CommercePrerollV2HookRecipe, len(enriched))
		for _, item := range enriched {
			byID[item.ID] = item
		}
		batch := *workspace.HookBatch
		batch.Items = append([]CommercePrerollV2HookRecipe{}, workspace.HookBatch.Items...)
		for index := range batch.Items {
			fresh, ok := byID[batch.Items[index].ID]
			if !ok || batch.Items[index].RecipeVersion != "" {
				continue
			}
			fresh.Concept = batch.Items[index].Concept
			fresh.Rationale = batch.Items[index].Rationale
			fresh.PrimaryAction = batch.Items[index].PrimaryAction
			batch.Items[index] = fresh
		}
		workspace.HookBatch = &batch
	}
	if workspace.PromptDraft != nil {
		prompt := *workspace.PromptDraft
		prompt.Beats = append([]CommercePrerollV2Beat{}, workspace.PromptDraft.Beats...)
		if workspace.HookBatch != nil {
			for _, item := range workspace.HookBatch.Items {
				if item.ID != prompt.HookID {
					continue
				}
				template, err := compileCommercePrerollV2Prompt(workspace, item, prompt.DurationSeconds, prompt.ExtraInstruction)
				if err == nil && len(template.Beats) == len(prompt.Beats) {
					for index := range prompt.Beats {
						fillCommercePrerollV2Beat(&prompt.Beats[index], template.Beats[index])
					}
				}
				break
			}
		}
		if prompt.CreativePrompt == "" {
			prompt.CreativePrompt = prompt.CompiledPrompt
			prompt.EditMode = "storyboard_compiled"
		}
		if workspace.HookBatch != nil {
			for _, item := range workspace.HookBatch.Items {
				if item.ID == prompt.HookID {
					prompt.LockedConstraints = append(append([]string{}, item.ProductGuardrails...), item.NegativeConstraints...)
					break
				}
			}
		}
		if len(prompt.LockedConstraints) == 0 {
			prompt.LockedConstraints = append(append([]string{}, workspace.Analysis.Content.Product.AppearanceGuardrails...), workspace.Analysis.Content.Product.LogoGuardrails...)
		}
		if len(prompt.LockedConstraints) > 0 {
			_ = sealCommercePrerollV2Prompt(&prompt)
		}
		workspace.PromptDraft = &prompt
	}
}

func fillCommercePrerollV2Beat(target *CommercePrerollV2Beat, fallback CommercePrerollV2Beat) {
	if target.VisualDescription == "" {
		target.VisualDescription = fallback.VisualDescription
	}
	if target.SubjectAction == "" {
		target.SubjectAction = fallback.SubjectAction
	}
	if target.Camera == "" {
		target.Camera = fallback.Camera
	}
	if target.SceneAndLighting == "" {
		target.SceneAndLighting = fallback.SceneAndLighting
	}
	if target.ProductState == "" {
		target.ProductState = fallback.ProductState
	}
	if target.TransitionIn == "" {
		target.TransitionIn = fallback.TransitionIn
	}
	if target.TransitionOut == "" {
		target.TransitionOut = fallback.TransitionOut
	}
	if target.AudioInstruction == "" {
		target.AudioInstruction = fallback.AudioInstruction
	}
}

func (s Service) GetLatestCommercePrerollV2Workspace(
	ctx context.Context,
	actor contract.ActorContext,
	projectID contract.ProjectID,
) (TaskDetail, error) {
	if s.Repository == nil || s.Projects == nil {
		return TaskDetail{}, fmt.Errorf("creative commerce preroll V2 dependencies are incomplete")
	}
	if !actor.HasScope(ScopeRead) {
		return TaskDetail{}, fmt.Errorf("%s scope is required", ScopeRead)
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return TaskDetail{}, err
	}
	tasks, err := s.Repository.ListTasks(ctx, actor.OrganizationID, projectID, 100)
	if err != nil {
		return TaskDetail{}, err
	}
	for _, task := range tasks {
		if task.PerformanceMode != PerformanceModeCommercePreroll {
			continue
		}
		detail, detailErr := s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, task.ID)
		if detailErr != nil {
			return TaskDetail{}, detailErr
		}
		if detail.VideoDraft.CommercePrerollV2 != nil {
			return detail, nil
		}
	}
	return TaskDetail{}, ErrNotFound
}

func (s Service) reviseCommercePrerollV2(
	ctx context.Context,
	actor contract.ActorContext,
	projectID contract.ProjectID,
	taskID string,
	expectedRevision int64,
	mutate func(*VideoDraft, *CommercePrerollV2Workspace) error,
	status TaskStatus,
) (TaskDetail, error) {
	if s.ViralRemakes == nil {
		return TaskDetail{}, fmt.Errorf("creative commerce preroll V2 revision repository is unavailable")
	}
	detail, err := s.GetCommercePrerollV2Workspace(ctx, actor, projectID, taskID)
	if err != nil {
		return TaskDetail{}, err
	}
	if expectedRevision < 1 || detail.VideoDraft.Revision != expectedRevision {
		return TaskDetail{}, ErrVersionConflict
	}
	next := *detail.VideoDraft
	workspaceCopy := *detail.VideoDraft.CommercePrerollV2
	next.CommercePrerollV2 = &workspaceCopy
	next.Revision++
	next.CommercePrerollV2.Revision = next.Revision
	next.CommercePrerollV2.UpdatedAt = s.now()
	if err := mutate(&next, next.CommercePrerollV2); err != nil {
		return TaskDetail{}, err
	}
	if err := next.Validate(); err != nil {
		return TaskDetail{}, err
	}
	if _, err := s.ViralRemakes.ReviseVideoDraft(
		ctx, actor.OrganizationID, projectID, taskID, expectedRevision, next, status,
	); err != nil {
		return TaskDetail{}, err
	}
	return s.GetCommercePrerollV2Workspace(ctx, actor, projectID, taskID)
}
