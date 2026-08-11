package creative

import (
	"context"
	"fmt"
	"strings"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type ShortDramaV2AnalysisResult struct {
	InputHash     string                      `json:"input_hash"`
	PromptVersion string                      `json:"prompt_version"`
	Content       ShortDramaV2AnalysisContent `json:"content"`
}

type ShortDramaV2Analyzer interface {
	Analyze(context.Context, contract.ActorContext, contract.ProjectContext, contract.ProjectAssetRef) (ShortDramaV2AnalysisResult, error)
}

type ShortDramaV2Planner interface {
	PlanDirections(context.Context, contract.ActorContext, contract.ProjectContext, ShortDramaV2Analysis) ([]ShortDramaV2HookDirection, string, error)
	CompilePrompts(context.Context, contract.ActorContext, contract.ProjectContext, ShortDramaV2Analysis, ShortDramaV2HookDirection, int) (ShortDramaV2PromptDraft, error)
}

type AnalyzeShortDramaV2SourceRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

type GenerateShortDramaV2DirectionsRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

type SelectShortDramaV2DirectionRequest struct {
	ExpectedRevision int64  `json:"expected_revision"`
	DirectionBatchID string `json:"direction_batch_id"`
	DirectionID      string `json:"direction_id"`
	DurationSeconds  int    `json:"duration_seconds"`
}

type UpdateShortDramaV2AnalysisRequest struct {
	ExpectedRevision int64                       `json:"expected_revision"`
	Content          ShortDramaV2AnalysisContent `json:"content"`
}

type UpdateShortDramaV2PromptsRequest struct {
	ExpectedRevision int64  `json:"expected_revision"`
	ImagePrompt      string `json:"image_prompt"`
	VideoDescription string `json:"video_description"`
	VideoPrompt      string `json:"video_prompt"`
}

func (s Service) AnalyzeShortDramaV2Source(
	ctx context.Context,
	actor contract.ActorContext,
	projectID contract.ProjectID,
	taskID string,
	request AnalyzeShortDramaV2SourceRequest,
) (TaskDetail, error) {
	detail, err := s.requireShortDramaV2Workspace(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	if request.ExpectedRevision != detail.VideoDraft.Revision {
		return TaskDetail{}, ErrVersionConflict
	}
	if s.ShortDramaV2Analyzer == nil {
		return TaskDetail{}, fmt.Errorf("short drama V2 analysis capability is unavailable")
	}
	project, err := s.Projects.RequireActiveContext(ctx, actor, projectID)
	if err != nil {
		return TaskDetail{}, err
	}
	result, err := s.ShortDramaV2Analyzer.Analyze(ctx, actor, project, detail.VideoDraft.ShortDramaPrerollV2.SourceVideo)
	if err != nil {
		return TaskDetail{}, err
	}
	if err := validateShortDramaV2AnalysisResult(result); err != nil {
		return TaskDetail{}, err
	}
	now := s.now()
	next := *detail.VideoDraft
	next.Revision++
	next.CreatedAt = now
	workspace := *detail.VideoDraft.ShortDramaPrerollV2
	workspace.Revision = next.Revision
	workspace.ActiveStage = ShortDramaV2StageAnalysisReady
	workspace.Analysis = ShortDramaV2Analysis{
		ShortDramaV2AsyncResource: ShortDramaV2AsyncResource{Status: ShortDramaV2ResourceReady},
		Revision:                  1, InputHash: result.InputHash, PromptVersion: result.PromptVersion, Content: result.Content,
	}
	workspace.DirectionBatch = nil
	workspace.PromptDraft = nil
	workspace.FirstFrameBatch = nil
	workspace.TrustedMaterials = nil
	workspace.GenerationSpec = nil
	workspace.LatestVideoAttemptID, workspace.VideoError = "", nil
	workspace.RawOutputAsset = nil
	workspace.OutputAsset = nil
	workspace.OutputNormalization = nil
	workspace.UpdatedAt = now
	next.ShortDramaPrerollV2 = &workspace
	next.Prompt = "视频理解已完成，等待生成前贴方向"
	if _, err := s.ViralRemakes.ReviseVideoDraft(ctx, actor.OrganizationID, projectID, taskID, detail.VideoDraft.Revision, next, TaskInProgress); err != nil {
		return TaskDetail{}, err
	}
	return s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
}

func (s Service) GenerateShortDramaV2Directions(
	ctx context.Context,
	actor contract.ActorContext,
	projectID contract.ProjectID,
	taskID string,
	request GenerateShortDramaV2DirectionsRequest,
) (TaskDetail, error) {
	detail, err := s.requireShortDramaV2Workspace(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	if request.ExpectedRevision != detail.VideoDraft.Revision {
		return TaskDetail{}, ErrVersionConflict
	}
	workspace := detail.VideoDraft.ShortDramaPrerollV2
	if workspace.Analysis.Status != ShortDramaV2ResourceReady || workspace.Analysis.Revision < 1 {
		return TaskDetail{}, ErrInvalidState
	}
	if s.ShortDramaV2Planner == nil {
		return TaskDetail{}, fmt.Errorf("short drama V2 direction planner is unavailable")
	}
	project, err := s.Projects.RequireActiveContext(ctx, actor, projectID)
	if err != nil {
		return TaskDetail{}, err
	}
	directions, plannerVersion, err := s.ShortDramaV2Planner.PlanDirections(ctx, actor, project, workspace.Analysis)
	if err != nil {
		return TaskDetail{}, err
	}
	if err := validateShortDramaV2Directions(workspace.Analysis, directions); err != nil {
		return TaskDetail{}, err
	}
	now := s.now()
	next := *detail.VideoDraft
	next.Revision++
	next.CreatedAt = now
	updated := *workspace
	updated.Revision = next.Revision
	updated.ActiveStage = ShortDramaV2StageDirectionsReady
	updated.DirectionBatch = &ShortDramaV2DirectionBatch{
		ShortDramaV2AsyncResource: ShortDramaV2AsyncResource{Status: ShortDramaV2ResourceReady},
		ID:                        fmt.Sprintf("%s_direction_batch_%d", taskID, next.Revision), Revision: next.Revision,
		AnalysisRevision: workspace.Analysis.Revision, PlannerVersion: strings.TrimSpace(plannerVersion),
		Items: append([]ShortDramaV2HookDirection{}, directions...),
	}
	updated.PromptDraft = nil
	updated.FirstFrameBatch = nil
	updated.TrustedMaterials = nil
	updated.GenerationSpec = nil
	updated.LatestVideoAttemptID, updated.VideoError = "", nil
	updated.RawOutputAsset = nil
	updated.OutputAsset = nil
	updated.OutputNormalization = nil
	updated.UpdatedAt = now
	next.ShortDramaPrerollV2 = &updated
	next.Prompt = "已生成 4 个前贴方向，等待人工选择"
	if _, err := s.ViralRemakes.ReviseVideoDraft(ctx, actor.OrganizationID, projectID, taskID, detail.VideoDraft.Revision, next, TaskInProgress); err != nil {
		return TaskDetail{}, err
	}
	return s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
}

func (s Service) SelectShortDramaV2Direction(
	ctx context.Context,
	actor contract.ActorContext,
	projectID contract.ProjectID,
	taskID string,
	request SelectShortDramaV2DirectionRequest,
) (TaskDetail, error) {
	detail, err := s.requireShortDramaV2Workspace(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	if request.ExpectedRevision != detail.VideoDraft.Revision {
		return TaskDetail{}, ErrVersionConflict
	}
	if err := validateShortDramaV2Duration(request.DurationSeconds); err != nil {
		return TaskDetail{}, err
	}
	workspace := detail.VideoDraft.ShortDramaPrerollV2
	batch := workspace.DirectionBatch
	if batch == nil || batch.Status != ShortDramaV2ResourceReady || batch.ID != request.DirectionBatchID {
		return TaskDetail{}, ErrInvalidState
	}
	var selected *ShortDramaV2HookDirection
	for index := range batch.Items {
		if batch.Items[index].ID == request.DirectionID {
			selected = &batch.Items[index]
			break
		}
	}
	if selected == nil {
		return TaskDetail{}, fmt.Errorf("short drama V2 direction does not belong to the active batch")
	}
	if s.ShortDramaV2Planner == nil {
		return TaskDetail{}, fmt.Errorf("short drama V2 prompt planner is unavailable")
	}
	project, err := s.Projects.RequireActiveContext(ctx, actor, projectID)
	if err != nil {
		return TaskDetail{}, err
	}
	prompt, err := s.ShortDramaV2Planner.CompilePrompts(ctx, actor, project, workspace.Analysis, *selected, request.DurationSeconds)
	if err != nil {
		return TaskDetail{}, err
	}
	prompt.DirectionID = selected.ID
	prompt.DurationSeconds = request.DurationSeconds
	prompt.Revision = 1
	prompt.BaseVideoPrompt = prompt.VideoPrompt
	if err := validateShortDramaV2PromptDraft(prompt); err != nil {
		return TaskDetail{}, err
	}
	hash, err := shortDramaV2PromptHash(prompt)
	if err != nil {
		return TaskDetail{}, err
	}
	prompt.ContentHash = hash
	now := s.now()
	next := *detail.VideoDraft
	next.Revision++
	next.CreatedAt = now
	updated := *workspace
	updated.Revision = next.Revision
	updated.ActiveStage = ShortDramaV2StagePromptsReady
	batchCopy := *batch
	batchCopy.SelectedDirectionID = selected.ID
	updated.DirectionBatch = &batchCopy
	updated.PromptDraft = &prompt
	updated.FirstFrameBatch = nil
	updated.TrustedMaterials = nil
	updated.GenerationSpec = nil
	updated.LatestVideoAttemptID, updated.VideoError = "", nil
	updated.RawOutputAsset = nil
	updated.OutputAsset = nil
	updated.OutputNormalization = nil
	updated.UpdatedAt = now
	next.ShortDramaPrerollV2 = &updated
	next.Prompt = prompt.VideoPrompt
	next.DurationSeconds = request.DurationSeconds
	if _, err := s.ViralRemakes.ReviseVideoDraft(ctx, actor.OrganizationID, projectID, taskID, detail.VideoDraft.Revision, next, TaskInProgress); err != nil {
		return TaskDetail{}, err
	}
	return s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
}

func (s Service) UpdateShortDramaV2Analysis(
	ctx context.Context,
	actor contract.ActorContext,
	projectID contract.ProjectID,
	taskID string,
	request UpdateShortDramaV2AnalysisRequest,
) (TaskDetail, error) {
	detail, err := s.requireShortDramaV2Workspace(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	if request.ExpectedRevision != detail.VideoDraft.Revision {
		return TaskDetail{}, ErrVersionConflict
	}
	workspace := detail.VideoDraft.ShortDramaPrerollV2
	result := ShortDramaV2AnalysisResult{
		InputHash: workspace.Analysis.InputHash, PromptVersion: workspace.Analysis.PromptVersion, Content: request.Content,
	}
	if err := validateShortDramaV2AnalysisResult(result); err != nil {
		return TaskDetail{}, err
	}
	now := s.now()
	next := *detail.VideoDraft
	next.Revision++
	next.CreatedAt = now
	updated := *workspace
	updated.Revision = next.Revision
	updated.ActiveStage = ShortDramaV2StageAnalysisReady
	updated.Analysis = workspace.Analysis
	updated.Analysis.Revision++
	updated.Analysis.Content = request.Content
	updated.DirectionBatch = nil
	updated.PromptDraft = nil
	updated.FirstFrameBatch = nil
	updated.TrustedMaterials = nil
	updated.GenerationSpec = nil
	updated.LatestVideoAttemptID, updated.VideoError = "", nil
	updated.RawOutputAsset = nil
	updated.OutputAsset = nil
	updated.OutputNormalization = nil
	updated.UpdatedAt = now
	next.ShortDramaPrerollV2 = &updated
	next.Prompt = "视频理解已人工修订，等待重新生成前贴方向"
	if _, err := s.ViralRemakes.ReviseVideoDraft(ctx, actor.OrganizationID, projectID, taskID, detail.VideoDraft.Revision, next, TaskInProgress); err != nil {
		return TaskDetail{}, err
	}
	return s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
}

func (s Service) UpdateShortDramaV2Prompts(
	ctx context.Context,
	actor contract.ActorContext,
	projectID contract.ProjectID,
	taskID string,
	request UpdateShortDramaV2PromptsRequest,
) (TaskDetail, error) {
	detail, err := s.requireShortDramaV2Workspace(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	if request.ExpectedRevision != detail.VideoDraft.Revision {
		return TaskDetail{}, ErrVersionConflict
	}
	workspace := detail.VideoDraft.ShortDramaPrerollV2
	if workspace.PromptDraft == nil || workspace.DirectionBatch == nil || workspace.DirectionBatch.SelectedDirectionID == "" {
		return TaskDetail{}, ErrInvalidState
	}
	prompt := *workspace.PromptDraft
	imagePromptChanged := strings.TrimSpace(request.ImagePrompt) != prompt.ImagePrompt
	prompt.Revision++
	prompt.ImagePrompt = strings.TrimSpace(request.ImagePrompt)
	prompt.VideoDescription = strings.TrimSpace(request.VideoDescription)
	prompt.VideoPrompt = strings.TrimSpace(request.VideoPrompt)
	if strings.TrimSpace(prompt.SelectedVariantKey) == "" {
		prompt.BaseVideoPrompt = prompt.VideoPrompt
	}
	if err := validateShortDramaV2PromptDraft(prompt); err != nil {
		return TaskDetail{}, err
	}
	hash, err := shortDramaV2PromptHash(prompt)
	if err != nil {
		return TaskDetail{}, err
	}
	prompt.ContentHash = hash
	now := s.now()
	next := *detail.VideoDraft
	next.Revision++
	next.CreatedAt = now
	updated := *workspace
	updated.Revision = next.Revision
	updated.PromptDraft = &prompt
	updated.ActiveStage = ShortDramaV2StagePromptsReady
	if imagePromptChanged {
		updated.FirstFrameBatch = nil
		updated.TrustedMaterials = nil
		updated.GenerationSpec = nil
	} else if updated.FirstFrameBatch != nil && updated.FirstFrameBatch.SelectedAsset != nil {
		updated.ActiveStage = ShortDramaV2StageFrameSelected
		spec, compileErr := compileShortDramaV2GenerationSpec(updated, projectID, next.Revision)
		if compileErr != nil {
			return TaskDetail{}, compileErr
		}
		updated.GenerationSpec = spec
	} else if updated.FirstFrameBatch != nil && updated.FirstFrameBatch.Status == ShortDramaV2ResourceReady {
		updated.ActiveStage = ShortDramaV2StageFramesReady
		updated.GenerationSpec = nil
	} else {
		updated.GenerationSpec = nil
	}
	updated.LatestVideoAttemptID, updated.VideoError = "", nil
	updated.RawOutputAsset = nil
	updated.OutputAsset = nil
	updated.OutputNormalization = nil
	updated.UpdatedAt = now
	next.ShortDramaPrerollV2 = &updated
	next.Prompt = prompt.VideoPrompt
	if _, err := s.ViralRemakes.ReviseVideoDraft(ctx, actor.OrganizationID, projectID, taskID, detail.VideoDraft.Revision, next, TaskInProgress); err != nil {
		return TaskDetail{}, err
	}
	return s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
}

func shortDramaV2PromptHash(prompt ShortDramaV2PromptDraft) (string, error) {
	hash, err := contract.CanonicalJSONHash(struct {
		DirectionID        string `json:"direction_id"`
		DurationSeconds    int    `json:"duration_seconds"`
		ImagePrompt        string `json:"image_prompt"`
		VideoDescription   string `json:"video_description"`
		VideoPrompt        string `json:"video_prompt"`
		BaseVideoPrompt    string `json:"base_video_prompt,omitempty"`
		SelectedVariantKey string `json:"selected_variant_key,omitempty"`
		CompilerVersion    string `json:"compiler_version"`
	}{prompt.DirectionID, prompt.DurationSeconds, prompt.ImagePrompt, prompt.VideoDescription, prompt.VideoPrompt, prompt.BaseVideoPrompt, prompt.SelectedVariantKey, prompt.CompilerVersion})
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

func (s Service) requireShortDramaV2Workspace(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, write bool) (TaskDetail, error) {
	if s.Repository == nil || s.ViralRemakes == nil || s.Projects == nil {
		return TaskDetail{}, fmt.Errorf("short drama preroll V2 dependencies are incomplete")
	}
	if write && !actor.HasScope(ScopeWrite) {
		return TaskDetail{}, fmt.Errorf("%s scope is required", ScopeWrite)
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return TaskDetail{}, err
	}
	detail, err := s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
	if err != nil {
		return TaskDetail{}, err
	}
	if detail.Task.Format != FormatVideo || detail.Task.PerformanceMode != PerformanceModeShortDramaPreroll ||
		detail.VideoDraft == nil || detail.VideoDraft.ShortDramaPrerollV2 == nil || detail.Task.Status == TaskArchived {
		return TaskDetail{}, ErrInvalidState
	}
	return detail, nil
}

func validateShortDramaV2AnalysisResult(result ShortDramaV2AnalysisResult) error {
	content := result.Content
	if strings.TrimSpace(result.InputHash) == "" || strings.TrimSpace(result.PromptVersion) == "" ||
		strings.TrimSpace(content.Title) == "" || len([]rune(strings.TrimSpace(content.Synopsis))) < 20 ||
		strings.TrimSpace(content.OpeningBeat) == "" || strings.TrimSpace(content.CoreConflict) == "" ||
		strings.TrimSpace(content.UnresolvedHook) == "" || len(content.Evidence) == 0 {
		return fmt.Errorf("short drama V2 analysis result is incomplete")
	}
	seen := map[string]struct{}{}
	for _, evidence := range content.Evidence {
		id := strings.TrimSpace(evidence.ID)
		if id == "" {
			return fmt.Errorf("short drama V2 analysis evidence id is required")
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("short drama V2 analysis evidence id is duplicated")
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validateShortDramaV2Directions(analysis ShortDramaV2Analysis, directions []ShortDramaV2HookDirection) error {
	if len(directions) != 4 {
		return fmt.Errorf("short drama V2 direction batch must contain exactly four items")
	}
	evidence := map[string]struct{}{}
	for _, item := range analysis.Content.Evidence {
		evidence[item.ID] = struct{}{}
	}
	categories := map[string]int{}
	ids := map[string]struct{}{}
	for _, direction := range directions {
		if direction.Category != "curiosity" && direction.Category != "summary" {
			return fmt.Errorf("short drama V2 direction category is unsupported")
		}
		categories[direction.Category]++
		if strings.TrimSpace(direction.ID) == "" || strings.TrimSpace(direction.Title) == "" ||
			strings.TrimSpace(direction.HookCopy) == "" || strings.TrimSpace(direction.Description) == "" ||
			strings.TrimSpace(direction.VisualIntent) == "" || len(direction.GroundingEvidenceIDs) == 0 {
			return fmt.Errorf("short drama V2 direction is incomplete")
		}
		if _, exists := ids[direction.ID]; exists {
			return fmt.Errorf("short drama V2 direction id is duplicated")
		}
		ids[direction.ID] = struct{}{}
		for _, evidenceID := range direction.GroundingEvidenceIDs {
			if _, exists := evidence[evidenceID]; !exists {
				return fmt.Errorf("short drama V2 direction is not grounded in the active analysis")
			}
		}
	}
	if categories["curiosity"] != 2 || categories["summary"] != 2 {
		return fmt.Errorf("short drama V2 direction batch must contain two curiosity and two summary items")
	}
	return nil
}

func validateShortDramaV2Duration(duration int) error {
	switch duration {
	case 5, 6, 10, 12, 15:
		return nil
	default:
		return fmt.Errorf("short drama V2 duration must be one of 5, 6, 10, 12, or 15 seconds")
	}
}

func validateShortDramaV2PromptDraft(prompt ShortDramaV2PromptDraft) error {
	if err := validateShortDramaV2Duration(prompt.DurationSeconds); err != nil {
		return err
	}
	if strings.TrimSpace(prompt.DirectionID) == "" || strings.TrimSpace(prompt.ImagePrompt) == "" ||
		strings.TrimSpace(prompt.VideoDescription) == "" || strings.TrimSpace(prompt.VideoPrompt) == "" ||
		strings.TrimSpace(prompt.CompilerVersion) == "" || len(prompt.ImagePrompt) > 8000 || len(prompt.VideoPrompt) > 12000 {
		return fmt.Errorf("short drama V2 prompt draft is incomplete")
	}
	return nil
}
