package creative

import (
	"context"
	"fmt"
	"strings"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/media"
	"github.com/shikanon/cookies/internal/platform/provider"
)

type ShortDramaV2FirstFrameJobRequest struct {
	TaskID       string
	BatchID      string
	CandidateID  string
	VariantIndex int
	Prompt       string
	PromptHash   string
	Width        int
	Height       int
}

type ShortDramaV2ImageJobCreator interface {
	CreateFirstFrameJob(context.Context, contract.ActorContext, contract.ProjectContext, ShortDramaV2FirstFrameJobRequest) (contract.ProviderJob, error)
}

type PrepareShortDramaV2OpeningFrameRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

type GenerateShortDramaV2FirstFramesRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

type ReconcileShortDramaV2FirstFrameRequest struct {
	ExpectedRevision int64                `json:"expected_revision"`
	CandidateID      string               `json:"candidate_id"`
	Job              contract.ProviderJob `json:"job"`
}

type SelectShortDramaV2FirstFrameRequest struct {
	ExpectedRevision int64  `json:"expected_revision"`
	BatchID          string `json:"batch_id"`
	CandidateID      string `json:"candidate_id"`
}

type BindShortDramaV2TrustedMaterialsRequest struct {
	ExpectedRevision  int64  `json:"expected_revision"`
	FirstFrameAssetID string `json:"first_frame_asset_id"`
	LastFrameAssetID  string `json:"last_frame_asset_id"`
}

func (s Service) PrepareShortDramaV2OpeningFrame(ctx context.Context, requestContext contract.RequestContext, projectID contract.ProjectID, taskID string, request PrepareShortDramaV2OpeningFrameRequest) (TaskDetail, error) {
	detail, err := s.requireShortDramaV2Workspace(ctx, requestContext.Actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	if request.ExpectedRevision != detail.VideoDraft.Revision {
		return TaskDetail{}, ErrVersionConflict
	}
	if s.GameEvidenceFrames == nil || s.DerivedAssets == nil {
		return TaskDetail{}, fmt.Errorf("short drama V2 frame extraction capability is unavailable")
	}
	workspace := detail.VideoDraft.ShortDramaPrerollV2
	extracted, err := s.GameEvidenceFrames.ExtractFrame(ctx, media.FrameExtractionRequest{
		OrganizationID: requestContext.Actor.OrganizationID, ProjectID: projectID,
		SourceVideo: workspace.SourceVideo.AssetVersion, TimestampMS: 0,
	})
	if err != nil {
		return TaskDetail{}, fmt.Errorf("extract short drama opening frame: %w", err)
	}
	derivationHash, err := contract.CanonicalJSONHash(struct {
		Source    contract.AssetVersionRef `json:"source"`
		Timestamp int64                    `json:"timestamp_ms"`
		Version   string                   `json:"extractor_version"`
	}{workspace.SourceVideo.AssetVersion, 0, extracted.Version})
	if err != nil {
		_ = extracted.Content.Close()
		return TaskDetail{}, err
	}
	derivationID := "short-drama-opening-frame-" + derivationHash
	asset, writeErr := s.DerivedAssets.IngestDerivedImage(ctx, requestContext, projectID, derivationID, workspace.SourceVideo.AssetVersion, extracted.Content, extracted.SizeBytes, extracted.MIMEType)
	closeErr := extracted.Content.Close()
	if writeErr != nil {
		return TaskDetail{}, writeErr
	}
	if closeErr != nil {
		return TaskDetail{}, closeErr
	}
	now := s.now()
	next := *detail.VideoDraft
	next.Revision++
	next.CreatedAt = now
	updated := *workspace
	updated.Revision = next.Revision
	updated.SourceOpeningFrame = &ShortDramaV2SourceOpeningFrame{
		Status: ShortDramaV2ResourceReady, Asset: &asset, SourceVideo: workspace.SourceVideo,
		TimestampMS: 0, DerivationID: derivationID, ExtractionVersion: extracted.Version,
	}
	updated.GenerationSpec = nil
	updated.LatestVideoAttemptID, updated.VideoError = "", nil
	updated.RawOutputAsset = nil
	updated.OutputAsset = nil
	updated.OutputNormalization = nil
	updated.UpdatedAt = now
	next.ShortDramaPrerollV2 = &updated
	if _, err := s.ViralRemakes.ReviseVideoDraft(ctx, requestContext.Actor.OrganizationID, projectID, taskID, detail.VideoDraft.Revision, next, TaskInProgress); err != nil {
		return TaskDetail{}, err
	}
	return s.Repository.GetTaskDetail(ctx, requestContext.Actor.OrganizationID, projectID, taskID)
}

func (s Service) GenerateShortDramaV2FirstFrames(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request GenerateShortDramaV2FirstFramesRequest) (TaskDetail, error) {
	detail, err := s.requireShortDramaV2Workspace(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	if request.ExpectedRevision != detail.VideoDraft.Revision {
		return TaskDetail{}, ErrVersionConflict
	}
	workspace := detail.VideoDraft.ShortDramaPrerollV2
	canGenerate := workspace.ActiveStage == ShortDramaV2StagePromptsReady ||
		workspace.ActiveStage == ShortDramaV2StageFramesReady ||
		workspace.ActiveStage == ShortDramaV2StageFrameSelected ||
		workspace.ActiveStage == ShortDramaV2StageCompleted
	if workspace.PromptDraft == nil || !canGenerate {
		return TaskDetail{}, ErrInvalidState
	}
	if s.ShortDramaV2Images == nil {
		return TaskDetail{}, fmt.Errorf("short drama V2 image generation capability is unavailable")
	}
	project, err := s.Projects.RequireActiveContext(ctx, actor, projectID)
	if err != nil {
		return TaskDetail{}, err
	}
	batchID := fmt.Sprintf("%s_first_frame_batch_%d", taskID, detail.VideoDraft.Revision+1)
	candidates := make([]ShortDramaV2FirstFrameCandidate, 0, 3)
	var sourceCanvas ShortDramaSourceCanvas
	var modelCanvas ShortDramaModelCanvas
	var outputCanvas ShortDramaOutputCanvas
	if workspace.SourceCanvas != nil && workspace.ModelCanvas != nil && workspace.OutputCanvas != nil {
		sourceCanvas, modelCanvas, outputCanvas = *workspace.SourceCanvas, *workspace.ModelCanvas, *workspace.OutputCanvas
	} else {
		sourceCanvas, modelCanvas, outputCanvas, err = deriveShortDramaCanvases(workspace.SourceMetadata)
		if err != nil {
			return TaskDetail{}, fmt.Errorf("derive short drama V3 canvases: %w", err)
		}
	}
	variants := []struct {
		key, mechanism, style, instruction string
	}{
		{"anime_cinematic", "强姿态与强情绪瞬间", "二维动漫电影风", "明显二维动漫电影风；用强姿态和强情绪瞬间建立猎奇注意力，原创角色设计"},
		{"guoman_semireal", "环境悬念与空间压迫", "原创国漫半写实", "原创国漫半写实风；通过环境信息缺口和空间压迫建立悬念，人物与场景关系清楚"},
		{"cinematic_realistic", "人物反应与冲突临界点", "照片级电影写实", "照片级电影写实风；捕捉人物反应和冲突临界点，不模仿真实演员或已知角色"},
	}
	for index, variant := range variants {
		candidateID := fmt.Sprintf("%s_candidate_%d", batchID, index+1)
		job, err := s.ShortDramaV2Images.CreateFirstFrameJob(ctx, actor, project, ShortDramaV2FirstFrameJobRequest{
			TaskID: taskID, BatchID: batchID, CandidateID: candidateID, VariantIndex: index + 1,
			Prompt: workspace.PromptDraft.ImagePrompt + "\n候选画风与机制：" + variant.instruction +
				fmt.Sprintf("\n画幅安全区：最终画面为 %d:%d，主体、脸和关键道具必须位于中央安全区，不得贴边，不生成文字、水印或 Logo。", outputCanvas.AspectNum, outputCanvas.AspectDen),
			PromptHash: workspace.PromptDraft.ContentHash,
			Width:      modelCanvas.ImageWidth, Height: modelCanvas.ImageHeight,
		})
		if err != nil {
			return TaskDetail{}, fmt.Errorf("create first frame job %d: %w", index+1, err)
		}
		candidates = append(candidates, ShortDramaV2FirstFrameCandidate{
			ID: candidateID, VariantIndex: index + 1, ProviderJobID: job.ID, Status: ShortDramaV2ResourceQueued,
			VariantKey: variant.key, VisualMechanism: variant.mechanism, StyleProfile: variant.style,
		})
	}
	now := s.now()
	next := *detail.VideoDraft
	next.Revision++
	next.CreatedAt = now
	updated := *workspace
	updated.ContractVersion = ShortDramaPrerollV3ContractVersion
	updated.Revision = next.Revision
	updated.SourceCanvas = &sourceCanvas
	updated.ModelCanvas = &modelCanvas
	updated.OutputCanvas = &outputCanvas
	updated.FirstFrameBatch = &ShortDramaV2FirstFrameBatch{
		ShortDramaV2AsyncResource: ShortDramaV2AsyncResource{Status: ShortDramaV2ResourceQueued},
		ID:                        batchID, Revision: next.Revision, PromptRevision: workspace.PromptDraft.Revision, Candidates: candidates,
	}
	updated.ActiveStage = ShortDramaV2StageFramesGenerating
	updated.TrustedMaterials = nil
	updated.GenerationSpec = nil
	updated.LatestVideoAttemptID, updated.VideoError = "", nil
	updated.RawOutputAsset = nil
	updated.OutputAsset = nil
	updated.OutputNormalization = nil
	updated.UpdatedAt = now
	next.ShortDramaPrerollV2 = &updated
	if _, err := s.ViralRemakes.ReviseVideoDraft(ctx, actor.OrganizationID, projectID, taskID, detail.VideoDraft.Revision, next, TaskInProgress); err != nil {
		return TaskDetail{}, err
	}
	return s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
}

func (s Service) ReconcileShortDramaV2FirstFrame(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request ReconcileShortDramaV2FirstFrameRequest) (TaskDetail, error) {
	detail, err := s.requireShortDramaV2Workspace(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	if request.ExpectedRevision != detail.VideoDraft.Revision {
		return TaskDetail{}, ErrVersionConflict
	}
	workspace := detail.VideoDraft.ShortDramaPrerollV2
	if workspace.FirstFrameBatch == nil || request.Job.ProjectID != projectID {
		return TaskDetail{}, ErrInvalidState
	}
	batch := *workspace.FirstFrameBatch
	index := -1
	for i := range batch.Candidates {
		if batch.Candidates[i].ID == request.CandidateID && batch.Candidates[i].ProviderJobID == request.Job.ID {
			index = i
			break
		}
	}
	if index < 0 {
		return TaskDetail{}, fmt.Errorf("short drama V2 first frame job does not match the active batch")
	}
	candidate := batch.Candidates[index]
	switch request.Job.ProviderStatus {
	case contract.ProviderJobSucceeded, contract.ProviderJobPartiallySucceeded:
		if len(request.Job.ProjectAssetRefs) == 0 || request.Job.ProjectAssetRefs[0].ProjectID != projectID {
			return TaskDetail{}, fmt.Errorf("short drama V2 image job completed without a durable project asset")
		}
		candidate.Status = ShortDramaV2ResourceReady
		asset := request.Job.ProjectAssetRefs[0]
		candidate.Asset = &asset
		if workspace.ModelCanvas == nil || workspace.OutputCanvas == nil {
			return TaskDetail{}, ErrInvalidState
		}
		candidate, err = s.normalizeShortDramaFirstFrameCandidate(ctx, actor, projectID, taskID, candidate, *workspace.ModelCanvas, *workspace.OutputCanvas)
		if err != nil {
			return TaskDetail{}, fmt.Errorf("normalize short drama first frame: %w", err)
		}
		candidate.ErrorCode, candidate.ErrorMessage = "", ""
	case contract.ProviderJobFailed, contract.ProviderJobCancelled, contract.ProviderJobExpired:
		candidate.Status = ShortDramaV2ResourceFailed
		candidate.ErrorCode = "IMAGE_GENERATION_FAILED"
		if request.Job.Error != nil {
			candidate.ErrorMessage = request.Job.Error.Message
		}
	default:
		return detail, nil
	}
	batch.Candidates[index] = candidate
	ready, failed := 0, 0
	for _, item := range batch.Candidates {
		switch item.Status {
		case ShortDramaV2ResourceReady:
			ready++
		case ShortDramaV2ResourceFailed, ShortDramaV2ResourceCancelled:
			failed++
		}
	}
	switch {
	case ready == len(batch.Candidates):
		batch.Status = ShortDramaV2ResourceReady
	case ready > 0 && ready+failed == len(batch.Candidates):
		batch.Status = ShortDramaV2ResourcePartial
	case failed == len(batch.Candidates):
		batch.Status = ShortDramaV2ResourceFailed
	default:
		batch.Status = ShortDramaV2ResourceRunning
	}
	now := s.now()
	next := *detail.VideoDraft
	next.Revision++
	next.CreatedAt = now
	updated := *workspace
	updated.Revision = next.Revision
	updated.FirstFrameBatch = &batch
	if ready > 0 && ready+failed == len(batch.Candidates) {
		updated.ActiveStage = ShortDramaV2StageFramesReady
	}
	updated.UpdatedAt = now
	next.ShortDramaPrerollV2 = &updated
	if _, err := s.ViralRemakes.ReviseVideoDraft(ctx, actor.OrganizationID, projectID, taskID, detail.VideoDraft.Revision, next, TaskInProgress); err != nil {
		return TaskDetail{}, err
	}
	return s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
}

func (s Service) SelectShortDramaV2FirstFrame(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request SelectShortDramaV2FirstFrameRequest) (TaskDetail, error) {
	detail, err := s.requireShortDramaV2Workspace(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	if request.ExpectedRevision != detail.VideoDraft.Revision {
		return TaskDetail{}, ErrVersionConflict
	}
	workspace := detail.VideoDraft.ShortDramaPrerollV2
	batch := workspace.FirstFrameBatch
	if batch == nil || batch.ID != request.BatchID || workspace.PromptDraft == nil {
		return TaskDetail{}, ErrInvalidState
	}
	batchCopy := *batch
	batchCopy.Candidates = append([]ShortDramaV2FirstFrameCandidate(nil), batch.Candidates...)
	sourceCanvas, modelCanvas, outputCanvas := workspace.SourceCanvas, workspace.ModelCanvas, workspace.OutputCanvas
	var selected, selectedOutput *contract.ProjectAssetRef
	var selectedCandidate *ShortDramaV2FirstFrameCandidate
	for index := range batchCopy.Candidates {
		candidate := batchCopy.Candidates[index]
		if candidate.ID == request.CandidateID && candidate.Status == ShortDramaV2ResourceReady {
			if candidate.ModelCanvasAsset == nil || candidate.OutputCanvasAsset == nil {
				if candidate.Asset == nil {
					return TaskDetail{}, fmt.Errorf("short drama V2 ready first frame has no source asset")
				}
				if sourceCanvas == nil || modelCanvas == nil || outputCanvas == nil {
					derivedSource, derivedModel, derivedOutput, err := deriveShortDramaCanvases(workspace.SourceMetadata)
					if err != nil {
						return TaskDetail{}, fmt.Errorf("derive canvases for legacy first frame: %w", err)
					}
					sourceCanvas, modelCanvas, outputCanvas = &derivedSource, &derivedModel, &derivedOutput
				}
				repaired, err := s.normalizeShortDramaFirstFrameCandidate(ctx, actor, projectID, taskID, candidate, *modelCanvas, *outputCanvas)
				if err != nil {
					return TaskDetail{}, fmt.Errorf("repair legacy short drama first frame: %w", err)
				}
				candidate = repaired
				batchCopy.Candidates[index] = repaired
			}
			if candidate.ModelCanvasAsset == nil || candidate.OutputCanvasAsset == nil {
				return TaskDetail{}, fmt.Errorf("short drama V2 first frame canvas assets are incomplete")
			}
			asset := *candidate.ModelCanvasAsset
			preview := *candidate.OutputCanvasAsset
			selected = &asset
			selectedOutput = &preview
			candidateCopy := candidate
			selectedCandidate = &candidateCopy
			break
		}
	}
	if selected == nil || selectedOutput == nil || selectedCandidate == nil {
		return TaskDetail{}, fmt.Errorf("short drama V2 first frame does not belong to the active ready batch")
	}
	now := s.now()
	next := *detail.VideoDraft
	next.Revision++
	next.CreatedAt = now
	updated := *workspace
	updated.Revision = next.Revision
	batchCopy.SelectedAsset = selected
	batchCopy.SelectedOutputAsset = selectedOutput
	updated.FirstFrameBatch = &batchCopy
	updated.SourceCanvas = sourceCanvas
	updated.ModelCanvas = modelCanvas
	updated.OutputCanvas = outputCanvas
	updated.TrustedMaterials = nil
	updated.ActiveStage = ShortDramaV2StageFrameSelected
	prompt := *updated.PromptDraft
	if strings.TrimSpace(prompt.BaseVideoPrompt) == "" {
		prompt.BaseVideoPrompt = prompt.VideoPrompt
	}
	prompt.Revision++
	prompt.SelectedVariantKey = selectedCandidate.VariantKey
	prompt.VideoPrompt = compileShortDramaSelectedFramePrompt(prompt.BaseVideoPrompt, prompt.DurationSeconds, *selectedCandidate, updated.OutputCanvas)
	promptHash, err := shortDramaV2PromptHash(prompt)
	if err != nil {
		return TaskDetail{}, err
	}
	prompt.ContentHash = promptHash
	updated.PromptDraft = &prompt
	spec, err := compileShortDramaV2GenerationSpec(updated, projectID, next.Revision)
	if err != nil {
		return TaskDetail{}, err
	}
	updated.GenerationSpec = spec
	updated.LatestVideoAttemptID, updated.VideoError = "", nil
	updated.RawOutputAsset = nil
	updated.OutputAsset = nil
	updated.OutputNormalization = nil
	updated.UpdatedAt = now
	next.ShortDramaPrerollV2 = &updated
	if _, err := s.ViralRemakes.ReviseVideoDraft(ctx, actor.OrganizationID, projectID, taskID, detail.VideoDraft.Revision, next, TaskReady); err != nil {
		return TaskDetail{}, err
	}
	return s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
}

func (s Service) BindShortDramaV2TrustedMaterials(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request BindShortDramaV2TrustedMaterialsRequest) (TaskDetail, error) {
	detail, err := s.requireShortDramaV2Workspace(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	if request.ExpectedRevision != detail.VideoDraft.Revision {
		return TaskDetail{}, ErrVersionConflict
	}
	workspace := detail.VideoDraft.ShortDramaPrerollV2
	if workspace.PromptDraft == nil || workspace.FirstFrameBatch == nil || workspace.FirstFrameBatch.SelectedAsset == nil ||
		workspace.SourceOpeningFrame == nil || workspace.SourceOpeningFrame.Asset == nil {
		return TaskDetail{}, ErrInvalidState
	}
	binding := ShortDramaV2TrustedMaterialBinding{
		ProviderCode: "ark-video", FirstFrameAssetID: strings.TrimSpace(request.FirstFrameAssetID), LastFrameAssetID: strings.TrimSpace(request.LastFrameAssetID),
	}
	for _, assetID := range []string{binding.FirstFrameAssetID, binding.LastFrameAssetID} {
		if err := (provider.VideoAuthorizedAssetReference{ProviderCode: binding.ProviderCode, AssetID: assetID}).Validate(); err != nil {
			return TaskDetail{}, err
		}
	}
	now := s.now()
	next := *detail.VideoDraft
	next.Revision++
	next.CreatedAt = now
	updated := *workspace
	updated.Revision = next.Revision
	updated.ActiveStage = ShortDramaV2StageFrameSelected
	updated.TrustedMaterials = &binding
	updated.LatestVideoAttemptID, updated.VideoError = "", nil
	updated.RawOutputAsset = nil
	updated.OutputAsset = nil
	updated.OutputNormalization = nil
	updated.UpdatedAt = now
	spec, err := compileShortDramaV2GenerationSpec(updated, projectID, next.Revision)
	if err != nil {
		return TaskDetail{}, err
	}
	updated.GenerationSpec = spec
	next.ShortDramaPrerollV2 = &updated
	if _, err := s.ViralRemakes.ReviseVideoDraft(ctx, actor.OrganizationID, projectID, taskID, detail.VideoDraft.Revision, next, TaskReady); err != nil {
		return TaskDetail{}, err
	}
	return s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
}

func compileShortDramaV2GenerationSpec(workspace ShortDramaPrerollV2Workspace, projectID contract.ProjectID, revision int64) (*ShortDramaV2GenerationSpec, error) {
	if workspace.PromptDraft == nil || workspace.FirstFrameBatch == nil || workspace.FirstFrameBatch.SelectedAsset == nil ||
		workspace.SourceCanvas == nil || workspace.ModelCanvas == nil || workspace.OutputCanvas == nil {
		return nil, ErrInvalidState
	}
	first := *workspace.FirstFrameBatch.SelectedAsset
	if first.ProjectID != projectID {
		return nil, ErrInvalidState
	}
	spec := ShortDramaV2GenerationSpec{
		ContractVersion: ShortDramaGenerationSpecV3, DraftRevision: revision,
		PromptRevision: workspace.PromptDraft.Revision, DurationSeconds: workspace.PromptDraft.DurationSeconds,
		AspectRatio: workspace.ModelCanvas.Ratio, Resolution: workspace.ModelCanvas.Resolution, AudioPolicy: string(provider.VideoAudioGenerated),
		InputMode: string(provider.VideoInputReferenceImage), FirstFrameAsset: first,
		SourceCanvas: workspace.SourceCanvas, ModelCanvas: workspace.ModelCanvas, OutputCanvas: workspace.OutputCanvas,
		CompiledPrompt: workspace.PromptDraft.VideoPrompt, PromptHash: workspace.PromptDraft.ContentHash,
	}
	hash, err := contract.CanonicalJSONHash(spec)
	if err != nil {
		return nil, err
	}
	spec.SpecHash = "sha256:" + hash
	return &spec, nil
}

func compileShortDramaSelectedFramePrompt(base string, duration int, candidate ShortDramaV2FirstFrameCandidate, output *ShortDramaOutputCanvas) string {
	canvasInstruction := "保持主体位于画面中央安全区"
	if output != nil {
		canvasInstruction = fmt.Sprintf("最终画面将确定性裁切为 %d:%d（%d×%d），人物面部、关键动作和字幕必须始终位于中央安全区", output.AspectNum, output.AspectDen, output.Width, output.Height)
	}
	return strings.TrimSpace(base) + fmt.Sprintf(
		"\n\n已选首帧方案：%s；视觉机制：%s；画风：%s。以所选参考图为视频起始视觉依据，不新增已知人物或额外角色。总时长严格为 %d 秒，最后 0.5 秒保持稳定构图。%s。不得生成水印、Logo、乱码文字或额外肢体。",
		candidate.VariantKey, candidate.VisualMechanism, candidate.StyleProfile, duration, canvasInstruction,
	)
}

func (s Service) ShortDramaV2ProviderInput(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string) (provider.VideoGenerationInput, string, string, error) {
	detail, err := s.requireShortDramaV2Workspace(ctx, actor, projectID, taskID, false)
	if err != nil {
		return provider.VideoGenerationInput{}, "", "", err
	}
	workspace := detail.VideoDraft.ShortDramaPrerollV2
	if workspace.GenerationSpec == nil || workspace.PromptDraft == nil || workspace.ActiveStage != ShortDramaV2StageFrameSelected {
		return provider.VideoGenerationInput{}, "", "", ErrInvalidState
	}
	spec := workspace.GenerationSpec
	if spec.PromptHash != workspace.PromptDraft.ContentHash || strings.TrimSpace(spec.SpecHash) == "" {
		return provider.VideoGenerationInput{}, "", "", ErrInvalidState
	}
	input := provider.VideoGenerationInput{
		Prompt: spec.CompiledPrompt, DurationSeconds: spec.DurationSeconds,
		AspectRatio: spec.AspectRatio, Resolution: spec.Resolution,
		AudioPolicy: provider.VideoAudioPolicy(spec.AudioPolicy), InputMode: provider.VideoInputMode(spec.InputMode),
		ConditioningAssets: []provider.VideoConditioningAsset{{Role: provider.VideoConditioningReferenceImage, Reference: spec.FirstFrameAsset}},
	}
	if err := input.Validate(); err != nil {
		return provider.VideoGenerationInput{}, "", "", err
	}
	return input, spec.PromptHash, spec.SpecHash, nil
}

func (s Service) RegisterShortDramaV2VideoJob(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID, providerJobID string) (TaskDetail, error) {
	detail, err := s.requireShortDramaV2Workspace(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	if strings.TrimSpace(providerJobID) == "" || detail.VideoDraft.ShortDramaPrerollV2.GenerationSpec == nil {
		return TaskDetail{}, ErrInvalidState
	}
	now := s.now()
	next := *detail.VideoDraft
	next.Revision++
	next.CreatedAt = now
	updated := *detail.VideoDraft.ShortDramaPrerollV2
	updated.Revision, updated.ActiveStage, updated.LatestVideoAttemptID = next.Revision, ShortDramaV2StageVideoGenerating, providerJobID
	updated.VideoError, updated.OutputAsset, updated.UpdatedAt = nil, nil, now
	next.ShortDramaPrerollV2 = &updated
	if _, err := s.ViralRemakes.ReviseVideoDraft(ctx, actor.OrganizationID, projectID, taskID, detail.VideoDraft.Revision, next, TaskGenerating); err != nil {
		return TaskDetail{}, err
	}
	return s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
}

type ReconcileShortDramaV2VideoRequest struct {
	ExpectedRevision int64                `json:"expected_revision"`
	Job              contract.ProviderJob `json:"job"`
}

func (s Service) ReconcileShortDramaV2Video(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request ReconcileShortDramaV2VideoRequest) (TaskDetail, error) {
	detail, err := s.requireShortDramaV2Workspace(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	workspace := detail.VideoDraft.ShortDramaPrerollV2
	if request.ExpectedRevision != detail.VideoDraft.Revision {
		return TaskDetail{}, ErrVersionConflict
	}
	if request.Job.ID != workspace.LatestVideoAttemptID || request.Job.ProjectID != projectID {
		return TaskDetail{}, ErrInvalidState
	}
	if request.Job.ProviderStatus == contract.ProviderJobFailed || request.Job.ProviderStatus == contract.ProviderJobCancelled || request.Job.ProviderStatus == contract.ProviderJobExpired {
		jobError := request.Job.Error
		if jobError == nil {
			jobError = &contract.JobError{Code: "VIDEO_GENERATION_FAILED", Message: "视频生成任务未完成，请重试。", Retryable: request.Job.ProviderStatus != contract.ProviderJobCancelled}
		}
		now := s.now()
		next := *detail.VideoDraft
		next.Revision++
		next.CreatedAt = now
		updated := *workspace
		updated.Revision, updated.ActiveStage, updated.VideoError, updated.UpdatedAt = next.Revision, ShortDramaV2StageFrameSelected, jobError, now
		next.ShortDramaPrerollV2 = &updated
		if _, err := s.ViralRemakes.ReviseVideoDraft(ctx, actor.OrganizationID, projectID, taskID, detail.VideoDraft.Revision, next, TaskInProgress); err != nil {
			return TaskDetail{}, err
		}
		return s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
	}
	if request.Job.ProviderStatus != contract.ProviderJobSucceeded && request.Job.ProviderStatus != contract.ProviderJobPartiallySucceeded {
		return detail, nil
	}
	if len(request.Job.ProjectAssetRefs) == 0 || request.Job.ProjectAssetRefs[0].ProjectID != projectID {
		return TaskDetail{}, fmt.Errorf("short drama V2 video completed without a durable project asset")
	}
	rawAsset := request.Job.ProjectAssetRefs[0]
	asset := rawAsset
	normalization := &ShortDramaV2AsyncResource{Status: ShortDramaV2ResourceReady}
	spec := workspace.GenerationSpec
	if spec != nil && spec.ModelCanvas != nil && spec.OutputCanvas != nil &&
		(spec.ModelCanvas.Width != spec.OutputCanvas.Width || spec.ModelCanvas.Height != spec.OutputCanvas.Height) {
		if s.ShortDramaV2OutputNormalizer == nil || s.RenderedAssets == nil {
			return TaskDetail{}, fmt.Errorf("short drama output normalization capability is unavailable")
		}
		output, normalizeErr := s.ShortDramaV2OutputNormalizer.NormalizeVideo(ctx, media.VideoNormalizationRequest{
			OrganizationID: actor.OrganizationID, ProjectID: projectID, SourceVideo: rawAsset.AssetVersion,
			Width: spec.OutputCanvas.Width, Height: spec.OutputCanvas.Height, FrameRate: spec.OutputCanvas.FrameRate,
		})
		if normalizeErr != nil {
			return TaskDetail{}, fmt.Errorf("normalize short drama output: %w", normalizeErr)
		}
		hash := strings.TrimPrefix(spec.SpecHash, "sha256:")
		if len(hash) > 24 {
			hash = hash[:24]
		}
		requestContext := contract.RequestContext{RequestID: "short-drama-normalize-" + request.Job.ID, TraceID: taskID, Actor: actor}
		normalized, ingestErr := s.RenderedAssets.IngestRenderedVideo(ctx, requestContext, projectID, "short-drama-normalized-"+hash, output.Content, output.SizeBytes)
		closeErr := output.Content.Close()
		if ingestErr != nil {
			return TaskDetail{}, ingestErr
		}
		if closeErr != nil {
			return TaskDetail{}, closeErr
		}
		asset = normalized
	}
	now := s.now()
	next := *detail.VideoDraft
	next.Revision++
	next.CreatedAt = now
	updated := *workspace
	updated.Revision, updated.ActiveStage, updated.VideoError, updated.OutputAsset, updated.UpdatedAt = next.Revision, ShortDramaV2StageCompleted, nil, &asset, now
	updated.RawOutputAsset, updated.OutputNormalization = &rawAsset, normalization
	next.ShortDramaPrerollV2 = &updated
	if _, err := s.ViralRemakes.ReviseVideoDraft(ctx, actor.OrganizationID, projectID, taskID, detail.VideoDraft.Revision, next, TaskGenerated); err != nil {
		return TaskDetail{}, err
	}
	return s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
}
