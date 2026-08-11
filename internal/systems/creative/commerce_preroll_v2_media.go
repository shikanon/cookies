package creative

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/media"
	"github.com/shikanon/cookies/internal/platform/provider"
)

type PrepareCommercePrerollV2ReferencesRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

type CommercePrerollV2FirstFrameJobRequest struct {
	TaskID         string
	BatchID        string
	CandidateID    string
	VariantIndex   int
	Prompt         string
	PromptHash     string
	Width          int
	Height         int
	ReferenceAsset contract.ProjectAssetRef
}

type CommercePrerollV2ImageJobCreator interface {
	CreateCommerceFirstFrameJob(context.Context, contract.ActorContext, contract.ProjectContext, CommercePrerollV2FirstFrameJobRequest) (contract.ProviderJob, error)
}

type GenerateCommercePrerollV2FirstFramesRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

type ReconcileCommercePrerollV2FirstFrameRequest struct {
	ExpectedRevision int64                `json:"expected_revision"`
	CandidateID      string               `json:"candidate_id"`
	Job              contract.ProviderJob `json:"job"`
}

type SelectCommercePrerollV2FirstFrameRequest struct {
	ExpectedRevision int64  `json:"expected_revision"`
	BatchID          string `json:"batch_id"`
	CandidateID      string `json:"candidate_id"`
}

type BindCommercePrerollV2ProductReferenceRequest struct {
	ExpectedRevision int64                    `json:"expected_revision"`
	Asset            contract.AssetVersionRef `json:"asset"`
}

type BindCommercePrerollV2CustomFirstFrameRequest struct {
	ExpectedRevision int64                    `json:"expected_revision"`
	Asset            contract.AssetVersionRef `json:"asset"`
	Title            string                   `json:"title,omitempty"`
}

func (s Service) BindCommercePrerollV2ProductReference(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request BindCommercePrerollV2ProductReferenceRequest) (TaskDetail, error) {
	if s.Assets == nil || request.Asset.Validate() != nil {
		return TaskDetail{}, fmt.Errorf("a valid product reference asset is required")
	}
	asset, err := s.Assets.ReadForCreative(ctx, actor, projectID, request.Asset)
	if err != nil {
		return TaskDetail{}, err
	}
	if !asset.Ready || asset.Kind != contract.AssetImage || asset.Ref != request.Asset {
		return TaskDetail{}, fmt.Errorf("product reference must be a ready image in the same project")
	}
	return s.reviseCommercePrerollV2(ctx, actor, projectID, taskID, request.ExpectedRevision,
		func(_ *VideoDraft, workspace *CommercePrerollV2Workspace) error {
			ref := contract.ProjectAssetRef{ProjectID: projectID, AssetVersion: request.Asset}
			frame := CommercePrerollV2DerivedFrame{Status: CommercePrerollV2ResourceReady, Asset: &ref, TimestampMS: -1, DerivationID: "user-upload"}
			workspace.ProductReference = &frame
			batch := workspace.ProductReferenceBatch
			if batch == nil {
				batch = &CommercePrerollV2ProductReferenceBatch{ID: taskID + "_product_references", Revision: workspace.Revision}
			}
			id := fmt.Sprintf("user_upload_%s_%d", ref.AssetVersion.AssetID, ref.AssetVersion.Version)
			batch.Candidates = append(batch.Candidates, CommercePrerollV2ProductReferenceCandidate{ID: id, SourceKind: "user_upload", Label: "用户上传商品图", Frame: frame, Scores: CommercePrerollV2ProductFrameCandidate{ID: id, TimestampMS: -1, Overall: 1}})
			batch.SelectedID = id
			workspace.ProductReferenceBatch = batch
			workspace.FirstFrameBatch, workspace.GenerationSpec, workspace.OutputAsset, workspace.AdoptedAsset = nil, nil, nil, nil
			return nil
		}, TaskInProgress)
}

func (s Service) BindCommercePrerollV2CustomFirstFrame(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request BindCommercePrerollV2CustomFirstFrameRequest) (TaskDetail, error) {
	if s.Assets == nil || request.Asset.Validate() != nil {
		return TaskDetail{}, fmt.Errorf("a valid custom first-frame asset is required")
	}
	asset, err := s.Assets.ReadForCreative(ctx, actor, projectID, request.Asset)
	if err != nil {
		return TaskDetail{}, err
	}
	if !asset.Ready || asset.Kind != contract.AssetImage || asset.Ref != request.Asset {
		return TaskDetail{}, fmt.Errorf("custom first frame must be a ready image in the same project")
	}
	return s.reviseCommercePrerollV2(ctx, actor, projectID, taskID, request.ExpectedRevision,
		func(_ *VideoDraft, workspace *CommercePrerollV2Workspace) error {
			if workspace.PromptDraft == nil {
				return ErrInvalidState
			}
			batchID := fmt.Sprintf("%s_custom_frames_%d", taskID, workspace.Revision)
			batch := workspace.FirstFrameBatch
			if batch == nil {
				batch = &CommercePrerollV2FirstFrameBatch{ID: batchID, Revision: workspace.Revision, PromptRevision: workspace.PromptDraft.Revision}
			} else {
				copy := *batch
				copy.Candidates = append([]CommercePrerollV2FirstFrameCandidate{}, batch.Candidates...)
				batch = &copy
				batchID = batch.ID
			}
			ref := contract.ProjectAssetRef{ProjectID: projectID, AssetVersion: request.Asset}
			candidateID := fmt.Sprintf("%s_custom_%d", batchID, len(batch.Candidates)+1)
			batch.Candidates = append(batch.Candidates, CommercePrerollV2FirstFrameCandidate{ID: candidateID, VariantIndex: len(batch.Candidates) + 1, Status: CommercePrerollV2ResourceReady, Asset: &ref, VariantKey: "user-upload", Title: strings.TrimSpace(request.Title), Description: "用户上传并确认的自定义首帧"})
			batch.Status = CommercePrerollV2ResourcePartial
			workspace.FirstFrameBatch = batch
			workspace.ActiveStage = CommercePrerollV2StageFrameReady
			workspace.GenerationSpec, workspace.OutputAsset, workspace.AdoptedAsset = nil, nil, nil
			return nil
		}, TaskInProgress)
}

func (s Service) PrepareCommercePrerollV2References(ctx context.Context, rc contract.RequestContext, projectID contract.ProjectID, taskID string, request PrepareCommercePrerollV2ReferencesRequest) (TaskDetail, error) {
	if s.GameEvidenceFrames == nil || s.DerivedAssets == nil {
		return TaskDetail{}, fmt.Errorf("commerce preroll frame extraction capability is unavailable")
	}
	detail, err := s.GetCommercePrerollV2Workspace(ctx, rc.Actor, projectID, taskID)
	if err != nil {
		return TaskDetail{}, err
	}
	if detail.VideoDraft.Revision != request.ExpectedRevision || detail.VideoDraft.CommercePrerollV2.Analysis.Status != CommercePrerollV2ResourceReady {
		return TaskDetail{}, ErrVersionConflict
	}
	workspace := detail.VideoDraft.CommercePrerollV2
	productFrames := append([]CommercePrerollV2ProductFrameCandidate{}, workspace.Analysis.Content.ProductFrameCandidates...)
	if len(productFrames) == 0 {
		productFrames = []CommercePrerollV2ProductFrameCandidate{{ID: "product_candidate_1", TimestampMS: workspace.Analysis.Content.ProductFrameMS, Overall: 1}}
	}
	sort.SliceStable(productFrames, func(i, j int) bool {
		if productFrames[i].Overall == productFrames[j].Overall {
			return productFrames[i].TimestampMS < productFrames[j].TimestampMS
		}
		return productFrames[i].Overall > productFrames[j].Overall
	})
	if len(productFrames) > 8 {
		productFrames = productFrames[:8]
	}
	productCandidates := make([]CommercePrerollV2ProductReferenceCandidate, 0, len(productFrames))
	for index, candidate := range productFrames {
		if strings.TrimSpace(candidate.ID) == "" {
			candidate.ID = fmt.Sprintf("product_candidate_%d", index+1)
		}
		frame, extractErr := s.extractCommercePrerollFrame(ctx, rc, projectID, taskID+"-product-reference-"+candidate.ID, workspace.SourceVideo.AssetVersion, candidate.TimestampMS)
		if extractErr != nil {
			return TaskDetail{}, extractErr
		}
		productCandidates = append(productCandidates, CommercePrerollV2ProductReferenceCandidate{ID: candidate.ID, SourceKind: "video_extract", Label: fmt.Sprintf("原视频 %.1f 秒商品画面", float64(candidate.TimestampMS)/1000), Frame: frame, Scores: candidate})
	}
	product := productCandidates[0].Frame
	anchor, err := s.extractCommercePrerollFrame(ctx, rc, projectID, taskID+"-opening-anchor", workspace.SourceVideo.AssetVersion, workspace.Analysis.Content.OpeningAnchorFrameMS)
	if err != nil {
		return TaskDetail{}, err
	}
	return s.reviseCommercePrerollV2(ctx, rc.Actor, projectID, taskID, request.ExpectedRevision,
		func(_ *VideoDraft, updated *CommercePrerollV2Workspace) error {
			updated.ProductReference = &product
			updated.ProductReferenceBatch = &CommercePrerollV2ProductReferenceBatch{ID: taskID + "_product_references", Revision: updated.Revision, Candidates: productCandidates, SelectedID: productCandidates[0].ID}
			updated.OpeningAnchor = &anchor
			return nil
		}, TaskInProgress)
}

type SelectCommercePrerollV2ProductReferenceRequest struct {
	ExpectedRevision int64  `json:"expected_revision"`
	CandidateID      string `json:"candidate_id"`
}

func (s Service) SelectCommercePrerollV2ProductReference(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request SelectCommercePrerollV2ProductReferenceRequest) (TaskDetail, error) {
	return s.reviseCommercePrerollV2(ctx, actor, projectID, taskID, request.ExpectedRevision, func(_ *VideoDraft, workspace *CommercePrerollV2Workspace) error {
		if workspace.ProductReferenceBatch == nil {
			return ErrInvalidState
		}
		batch := *workspace.ProductReferenceBatch
		for _, candidate := range batch.Candidates {
			if candidate.ID == request.CandidateID && candidate.Frame.Asset != nil {
				frame := candidate.Frame
				workspace.ProductReference = &frame
				batch.SelectedID = candidate.ID
				workspace.ProductReferenceBatch = &batch
				workspace.FirstFrameBatch, workspace.GenerationSpec, workspace.OutputAsset, workspace.AdoptedAsset = nil, nil, nil, nil
				return nil
			}
		}
		return ErrNotFound
	}, TaskInProgress)
}

func (s Service) extractCommercePrerollFrame(ctx context.Context, rc contract.RequestContext, projectID contract.ProjectID, label string, source contract.AssetVersionRef, timestampMS int64) (CommercePrerollV2DerivedFrame, error) {
	extracted, err := s.GameEvidenceFrames.ExtractFrame(ctx, media.FrameExtractionRequest{
		OrganizationID: rc.Actor.OrganizationID, ProjectID: projectID, SourceVideo: source, TimestampMS: timestampMS,
	})
	if err != nil {
		return CommercePrerollV2DerivedFrame{}, err
	}
	defer extracted.Content.Close()
	hash, err := contract.CanonicalJSONHash(struct {
		Source      contract.AssetVersionRef `json:"source"`
		TimestampMS int64                    `json:"timestamp_ms"`
		Version     string                   `json:"version"`
	}{source, timestampMS, extracted.Version})
	if err != nil {
		return CommercePrerollV2DerivedFrame{}, err
	}
	derivationID := commercePrerollFrameDerivationID(label, hash)
	asset, err := s.DerivedAssets.IngestDerivedImage(ctx, rc, projectID, derivationID, source, extracted.Content, extracted.SizeBytes, extracted.MIMEType)
	if err != nil {
		return CommercePrerollV2DerivedFrame{}, err
	}
	return CommercePrerollV2DerivedFrame{
		Status: CommercePrerollV2ResourceReady, Asset: &asset, TimestampMS: timestampMS,
		DerivationID: derivationID, ExtractionVersion: extracted.Version,
	}, nil
}

func commercePrerollFrameDerivationID(_ string, canonicalHash string) string {
	return "commerce-frame-" + canonicalHash
}

func (s Service) GenerateCommercePrerollV2FirstFrames(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request GenerateCommercePrerollV2FirstFramesRequest) (TaskDetail, error) {
	if s.CommercePrerollV2Images == nil {
		return TaskDetail{}, fmt.Errorf("commerce preroll first-frame generation capability is unavailable")
	}
	detail, err := s.GetCommercePrerollV2Workspace(ctx, actor, projectID, taskID)
	if err != nil {
		return TaskDetail{}, err
	}
	workspace := detail.VideoDraft.CommercePrerollV2
	if detail.VideoDraft.Revision != request.ExpectedRevision || workspace.PromptDraft == nil || workspace.ProductReference == nil || workspace.ProductReference.Asset == nil {
		return TaskDetail{}, ErrInvalidState
	}
	project, err := s.Projects.RequireActiveContext(ctx, actor, projectID)
	if err != nil {
		return TaskDetail{}, err
	}
	_, modelCanvas, _, err := deriveShortDramaCanvases(workspace.SourceMetadata)
	if err != nil {
		return TaskDetail{}, fmt.Errorf("derive commerce canvases: %w", err)
	}
	if workspace.OpeningAnchor == nil {
		return TaskDetail{}, ErrInvalidState
	}
	openingAnchor := *workspace.OpeningAnchor
	if openingAnchor.ModelCanvasAsset == nil {
		openingAnchor, err = s.normalizeCommerceOpeningAnchor(ctx, actor, projectID, taskID, openingAnchor, modelCanvas)
		if err != nil {
			return TaskDetail{}, err
		}
	}
	batchID := fmt.Sprintf("%s_commerce_frames_%d", taskID, request.ExpectedRevision+1)
	variants := []struct{ key, title, description, instruction string }{
		{"restrained", "克制清晰", "商品识别最稳定，动作最克制", "克制商业摄影，稳定正面构图，柔和受控光线"},
		{"dramatic", "强钩子", "明暗和材质变化更明显", "高反差商业摄影，钩子变化明显但商品外观完全保真"},
		{"native", "原片同调", "最大程度贴近原视频画面气质", "严格继承原视频的色调、构图、光线方向和材质质感"},
	}
	candidates := make([]CommercePrerollV2FirstFrameCandidate, 0, 3)
	for index, variant := range variants {
		candidateID := fmt.Sprintf("%s_candidate_%d", batchID, index+1)
		sourceAspect := reducedAspectRatio(workspace.SourceMetadata.WidthPixels, workspace.SourceMetadata.HeightPixels)
		job, createErr := s.CommercePrerollV2Images.CreateCommerceFirstFrameJob(ctx, actor, project, CommercePrerollV2FirstFrameJobRequest{
			TaskID: taskID, BatchID: batchID, CandidateID: candidateID, VariantIndex: index + 1,
			Prompt: workspace.PromptDraft.CompiledPrompt + "\n首帧变体：" + variant.instruction +
				"。输出为方形工作画布，但构图必须集中在中央 9:16 模型区；商品、Logo、动作主体和所有关键视觉必须进一步保持在该模型区的中央 " + sourceAspect + " 成片安全区内。两个安全区之外只允许可裁切背景。不要生成字幕、水印或额外 Logo。",
			PromptHash: workspace.PromptDraft.ContentHash, Width: 1024, Height: 1536, ReferenceAsset: *workspace.ProductReference.Asset,
		})
		if createErr != nil {
			return TaskDetail{}, createErr
		}
		candidates = append(candidates, CommercePrerollV2FirstFrameCandidate{
			ID: candidateID, VariantIndex: index + 1, ProviderJobID: job.ID, Status: CommercePrerollV2ResourceQueued,
			VariantKey: variant.key, Title: variant.title, Description: variant.description,
		})
	}
	return s.reviseCommercePrerollV2(ctx, actor, projectID, taskID, request.ExpectedRevision,
		func(_ *VideoDraft, updated *CommercePrerollV2Workspace) error {
			updated.OpeningAnchor = &openingAnchor
			updated.FirstFrameBatch = &CommercePrerollV2FirstFrameBatch{
				CommercePrerollV2AsyncResource: CommercePrerollV2AsyncResource{Status: CommercePrerollV2ResourceQueued},
				ID:                             batchID, Revision: updated.Revision, PromptRevision: workspace.PromptDraft.Revision, Candidates: candidates,
			}
			updated.GenerationSpec, updated.OutputAsset, updated.AdoptedAsset = nil, nil, nil
			updated.ActiveStage = CommercePrerollV2StageFramesGenerating
			return nil
		}, TaskInProgress)
}

func reducedAspectRatio(width, height int) string {
	if width < 1 || height < 1 {
		return "9:16"
	}
	a, b := width, height
	for b != 0 {
		a, b = b, a%b
	}
	return fmt.Sprintf("%d:%d", width/a, height/a)
}

func (s Service) ReconcileCommercePrerollV2FirstFrame(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request ReconcileCommercePrerollV2FirstFrameRequest) (TaskDetail, error) {
	if !commercePrerollProviderJobTerminal(request.Job.ProviderStatus) {
		detail, err := s.GetCommercePrerollV2Workspace(ctx, actor, projectID, taskID)
		if err != nil {
			return TaskDetail{}, err
		}
		workspace := detail.VideoDraft.CommercePrerollV2
		if detail.VideoDraft.Revision != request.ExpectedRevision || request.Job.ProjectID != projectID || workspace.FirstFrameBatch == nil {
			return TaskDetail{}, ErrInvalidState
		}
		for _, candidate := range workspace.FirstFrameBatch.Candidates {
			if candidate.ID == request.CandidateID && candidate.ProviderJobID == request.Job.ID {
				return detail, nil
			}
		}
		return TaskDetail{}, ErrInvalidState
	}
	return s.reviseCommercePrerollV2(ctx, actor, projectID, taskID, request.ExpectedRevision,
		func(_ *VideoDraft, workspace *CommercePrerollV2Workspace) error {
			if workspace.FirstFrameBatch == nil || request.Job.ProjectID != projectID {
				return ErrInvalidState
			}
			batch := *workspace.FirstFrameBatch
			batch.Candidates = append([]CommercePrerollV2FirstFrameCandidate{}, workspace.FirstFrameBatch.Candidates...)
			index := -1
			for i := range batch.Candidates {
				if batch.Candidates[i].ID == request.CandidateID && batch.Candidates[i].ProviderJobID == request.Job.ID {
					index = i
					break
				}
			}
			if index < 0 {
				return ErrInvalidState
			}
			candidate := batch.Candidates[index]
			switch request.Job.ProviderStatus {
			case contract.ProviderJobSucceeded, contract.ProviderJobPartiallySucceeded:
				if len(request.Job.ProjectAssetRefs) == 0 {
					return fmt.Errorf("commerce first-frame job completed without a durable asset")
				}
				asset := request.Job.ProjectAssetRefs[0]
				candidate.Status, candidate.Asset = CommercePrerollV2ResourceReady, &asset
				_, modelCanvas, outputCanvas, canvasErr := deriveShortDramaCanvases(workspace.SourceMetadata)
				if canvasErr != nil {
					return canvasErr
				}
				candidate, canvasErr = s.normalizeCommerceFirstFrameCandidate(ctx, actor, projectID, taskID, candidate, modelCanvas, outputCanvas)
				if canvasErr != nil {
					return canvasErr
				}
			case contract.ProviderJobFailed, contract.ProviderJobCancelled, contract.ProviderJobExpired:
				candidate.Status, candidate.ErrorCode = CommercePrerollV2ResourceFailed, "IMAGE_GENERATION_FAILED"
				if request.Job.Error != nil {
					candidate.ErrorMessage = request.Job.Error.Message
				}
			default:
				return nil
			}
			batch.Candidates[index] = candidate
			ready, finished := 0, 0
			for _, item := range batch.Candidates {
				if item.Status == CommercePrerollV2ResourceReady {
					ready++
					finished++
				}
				if item.Status == CommercePrerollV2ResourceFailed || item.Status == CommercePrerollV2ResourceCancelled {
					finished++
				}
			}
			if finished == len(batch.Candidates) {
				if ready == len(batch.Candidates) {
					batch.Status = CommercePrerollV2ResourceReady
				} else if ready > 0 {
					batch.Status = CommercePrerollV2ResourcePartial
				} else {
					batch.Status = CommercePrerollV2ResourceFailed
				}
				if ready > 0 {
					workspace.ActiveStage = CommercePrerollV2StageFrameReady
				}
			} else {
				batch.Status = CommercePrerollV2ResourceRunning
			}
			workspace.FirstFrameBatch = &batch
			return nil
		}, TaskInProgress)
}

func (s Service) SelectCommercePrerollV2FirstFrame(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request SelectCommercePrerollV2FirstFrameRequest) (TaskDetail, error) {
	return s.reviseCommercePrerollV2(ctx, actor, projectID, taskID, request.ExpectedRevision,
		func(_ *VideoDraft, workspace *CommercePrerollV2Workspace) error {
			if workspace.FirstFrameBatch == nil || workspace.FirstFrameBatch.ID != request.BatchID || workspace.PromptDraft == nil {
				return ErrInvalidState
			}
			batch := *workspace.FirstFrameBatch
			batch.Candidates = append([]CommercePrerollV2FirstFrameCandidate{}, workspace.FirstFrameBatch.Candidates...)
			var selected *contract.ProjectAssetRef
			for _, candidate := range batch.Candidates {
				if candidate.ID == request.CandidateID && candidate.Status == CommercePrerollV2ResourceReady && candidate.ModelCanvasAsset != nil && candidate.OutputCanvasAsset != nil {
					asset := *candidate.ModelCanvasAsset
					selected = &asset
					break
				}
			}
			if selected == nil {
				return ErrInvalidState
			}
			batch.SelectedID, batch.SelectedAsset = request.CandidateID, selected
			workspace.FirstFrameBatch = &batch
			sourceCanvas, modelCanvas, outputCanvas, canvasErr := deriveShortDramaCanvases(workspace.SourceMetadata)
			if canvasErr != nil {
				return canvasErr
			}
			spec := CommercePrerollV2GenerationSpec{
				ContractVersion: "creative-commerce-preroll-generation/v2", DraftRevision: workspace.Revision,
				DurationSeconds: workspace.PromptDraft.DurationSeconds, AspectRatio: modelCanvas.Ratio, Resolution: modelCanvas.Resolution,
				AudioPolicy: string(provider.VideoAudioGenerated), InputMode: string(provider.VideoInputReferenceImage),
				FirstFrameAsset: *selected,
				SourceCanvas:    &sourceCanvas, ModelCanvas: &modelCanvas, OutputCanvas: &outputCanvas,
				CompiledPrompt: workspace.PromptDraft.CompiledPrompt, PromptHash: workspace.PromptDraft.ContentHash,
			}
			hash, err := contract.CanonicalJSONHash(spec)
			if err != nil {
				return err
			}
			spec.SpecHash = "sha256:" + hash
			workspace.GenerationSpec = &spec
			workspace.ActiveStage = CommercePrerollV2StageFrameReady
			workspace.LatestVideoAttemptID, workspace.VideoError = "", nil
			workspace.OutputAsset, workspace.AdoptedAsset = nil, nil
			return nil
		}, TaskReady)
}

func (s Service) CommercePrerollV2ProviderInput(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string) (provider.VideoGenerationInput, string, string, error) {
	detail, err := s.GetCommercePrerollV2Workspace(ctx, actor, projectID, taskID)
	if err != nil {
		return provider.VideoGenerationInput{}, "", "", err
	}
	workspace := detail.VideoDraft.CommercePrerollV2
	if workspace.GenerationSpec == nil || workspace.PromptDraft == nil || workspace.FirstFrameBatch == nil || workspace.FirstFrameBatch.SelectedAsset == nil {
		return provider.VideoGenerationInput{}, "", "", ErrInvalidState
	}
	spec := workspace.GenerationSpec
	if spec.PromptHash != workspace.PromptDraft.ContentHash || strings.TrimSpace(spec.SpecHash) == "" {
		return provider.VideoGenerationInput{}, "", "", ErrInvalidState
	}
	input := provider.VideoGenerationInput{
		Prompt: spec.CompiledPrompt, DurationSeconds: spec.DurationSeconds, AspectRatio: spec.AspectRatio, Resolution: spec.Resolution,
		AudioPolicy: provider.VideoAudioPolicy(spec.AudioPolicy), InputMode: provider.VideoInputMode(spec.InputMode),
		ConditioningAssets: []provider.VideoConditioningAsset{{Role: provider.VideoConditioningReferenceImage, Reference: spec.FirstFrameAsset}},
	}
	if err := input.Validate(); err != nil {
		return provider.VideoGenerationInput{}, "", "", err
	}
	return input, spec.PromptHash, spec.SpecHash, nil
}

func (s Service) ReserveCommercePrerollV2VideoOperation(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, expectedRevision int64, operationID string) (TaskDetail, error) {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return TaskDetail{}, ErrInvalidState
	}
	return s.reviseCommercePrerollV2(ctx, actor, projectID, taskID, expectedRevision,
		func(_ *VideoDraft, workspace *CommercePrerollV2Workspace) error {
			if workspace.GenerationSpec == nil || workspace.GenerationSpec.SpecHash == "" {
				return ErrInvalidState
			}
			workspace.LatestVideoAttemptID = "pending:" + operationID
			workspace.VideoError, workspace.OutputAsset, workspace.AdoptedAsset = nil, nil, nil
			workspace.ActiveStage = CommercePrerollV2StageVideoGenerating
			return nil
		}, TaskGenerating)
}

func (s Service) RegisterCommercePrerollV2VideoJob(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, expectedRevision int64, operationID, providerJobID string) (TaskDetail, error) {
	operationID, providerJobID = strings.TrimSpace(operationID), strings.TrimSpace(providerJobID)
	if operationID == "" || providerJobID == "" {
		return TaskDetail{}, ErrInvalidState
	}
	return s.reviseCommercePrerollV2(ctx, actor, projectID, taskID, expectedRevision,
		func(_ *VideoDraft, workspace *CommercePrerollV2Workspace) error {
			if workspace.GenerationSpec == nil || workspace.LatestVideoAttemptID != "pending:"+operationID {
				return ErrInvalidState
			}
			workspace.LatestVideoAttemptID, workspace.VideoError, workspace.OutputAsset = providerJobID, nil, nil
			workspace.ActiveStage = CommercePrerollV2StageVideoGenerating
			return nil
		}, TaskGenerating)
}

func (s Service) FailCommercePrerollV2VideoOperation(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, expectedRevision int64, operationID string, jobError contract.JobError) (TaskDetail, error) {
	operationID = strings.TrimSpace(operationID)
	return s.reviseCommercePrerollV2(ctx, actor, projectID, taskID, expectedRevision,
		func(_ *VideoDraft, workspace *CommercePrerollV2Workspace) error {
			if operationID == "" || workspace.LatestVideoAttemptID != "pending:"+operationID {
				return ErrInvalidState
			}
			workspace.VideoError = &jobError
			workspace.ActiveStage = CommercePrerollV2StageFrameReady
			return nil
		}, TaskReady)
}

type ReconcileCommercePrerollV2VideoRequest struct {
	ExpectedRevision int64                `json:"expected_revision"`
	Job              contract.ProviderJob `json:"job"`
}

func (s Service) ReconcileCommercePrerollV2Video(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request ReconcileCommercePrerollV2VideoRequest) (TaskDetail, error) {
	if !commercePrerollProviderJobTerminal(request.Job.ProviderStatus) {
		detail, err := s.GetCommercePrerollV2Workspace(ctx, actor, projectID, taskID)
		if err != nil {
			return TaskDetail{}, err
		}
		workspace := detail.VideoDraft.CommercePrerollV2
		if detail.VideoDraft.Revision != request.ExpectedRevision || request.Job.ProjectID != projectID || request.Job.ID != workspace.LatestVideoAttemptID {
			return TaskDetail{}, ErrInvalidState
		}
		return detail, nil
	}
	return s.reviseCommercePrerollV2(ctx, actor, projectID, taskID, request.ExpectedRevision,
		func(_ *VideoDraft, workspace *CommercePrerollV2Workspace) error {
			if request.Job.ID != workspace.LatestVideoAttemptID || request.Job.ProjectID != projectID {
				return ErrInvalidState
			}
			switch request.Job.ProviderStatus {
			case contract.ProviderJobSucceeded, contract.ProviderJobPartiallySucceeded:
				if len(request.Job.ProjectAssetRefs) == 0 {
					return fmt.Errorf("commerce video completed without a durable project asset")
				}
				rawAsset := request.Job.ProjectAssetRefs[0]
				asset := rawAsset
				normalization := &CommercePrerollV2AsyncResource{Status: CommercePrerollV2ResourceReady, Progress: 100}
				spec := workspace.GenerationSpec
				if spec != nil && spec.ModelCanvas != nil && spec.OutputCanvas != nil &&
					(spec.ModelCanvas.Width != spec.OutputCanvas.Width || spec.ModelCanvas.Height != spec.OutputCanvas.Height) {
					if s.CommercePrerollV2OutputNormalizer == nil || s.RenderedAssets == nil {
						return fmt.Errorf("commerce preroll output normalization capability is unavailable")
					}
					output, normalizeErr := s.CommercePrerollV2OutputNormalizer.NormalizeVideo(ctx, media.VideoNormalizationRequest{
						OrganizationID: actor.OrganizationID, ProjectID: projectID, SourceVideo: rawAsset.AssetVersion,
						Width: spec.OutputCanvas.Width, Height: spec.OutputCanvas.Height, FrameRate: spec.OutputCanvas.FrameRate,
					})
					if normalizeErr != nil {
						return fmt.Errorf("normalize commerce preroll output: %w", normalizeErr)
					}
					hash := strings.TrimPrefix(spec.SpecHash, "sha256:")
					if len(hash) > 24 {
						hash = hash[:24]
					}
					requestContext := contract.RequestContext{RequestID: "commerce-preroll-normalize-" + request.Job.ID, TraceID: taskID, Actor: actor}
					normalized, ingestErr := s.RenderedAssets.IngestRenderedVideo(ctx, requestContext, projectID, "commerce-preroll-normalized-"+hash, output.Content, output.SizeBytes)
					closeErr := output.Content.Close()
					if ingestErr != nil {
						return ingestErr
					}
					if closeErr != nil {
						return closeErr
					}
					asset = normalized
				}
				workspace.RawOutputAsset, workspace.OutputNormalization = &rawAsset, normalization
				workspace.OutputAsset, workspace.VideoError = &asset, nil
				workspace.ActiveStage = CommercePrerollV2StageVideoReady
			case contract.ProviderJobFailed, contract.ProviderJobCancelled, contract.ProviderJobExpired:
				workspace.VideoError = request.Job.Error
				if workspace.VideoError == nil {
					workspace.VideoError = &contract.JobError{Code: "VIDEO_GENERATION_FAILED", Message: "视频生成任务未完成", Retryable: true}
				}
				workspace.ActiveStage = CommercePrerollV2StageFrameReady
			default:
				return nil
			}
			return nil
		}, TaskGenerated)
}

func commercePrerollProviderJobTerminal(status contract.ProviderJobStatus) bool {
	switch status {
	case contract.ProviderJobSucceeded, contract.ProviderJobPartiallySucceeded, contract.ProviderJobFailed, contract.ProviderJobCancelled, contract.ProviderJobExpired:
		return true
	default:
		return false
	}
}

type AdoptCommercePrerollV2OutputRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

func (s Service) AdoptCommercePrerollV2Output(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request AdoptCommercePrerollV2OutputRequest) (TaskDetail, error) {
	return s.reviseCommercePrerollV2(ctx, actor, projectID, taskID, request.ExpectedRevision,
		func(_ *VideoDraft, workspace *CommercePrerollV2Workspace) error {
			if workspace.OutputAsset == nil {
				return ErrInvalidState
			}
			asset := *workspace.OutputAsset
			workspace.AdoptedAsset = &asset
			workspace.ActiveStage = CommercePrerollV2StageOutputAdopted
			return nil
		}, TaskGenerated)
}
