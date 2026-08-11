package creative

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/media"
)

func TestCommercePrerollV2WorkspaceRequiresConfirmedImmutableSource(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	source := contract.ProjectAssetRef{
		ProjectID:    "project_1",
		AssetVersion: contract.AssetVersionRef{AssetID: "asset_source_ad", Version: 3},
	}
	workspace := CommercePrerollV2Workspace{
		ContractVersion: CommercePrerollV2ContractVersion,
		TaskID:          "creativetask_1",
		Revision:        1,
		ActiveStage:     CommercePrerollV2StageSourceReady,
		SourceVideo:     source,
		SourceVideoRights: RightsConfirmation{
			Status: RightsConfirmed, ConfirmedBy: "user_1", ConfirmedAt: now,
		},
		Analysis: CommercePrerollV2Analysis{
			CommercePrerollV2AsyncResource: CommercePrerollV2AsyncResource{Status: CommercePrerollV2ResourceIdle},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := workspace.Validate(); err != nil {
		t.Fatalf("valid source-ready workspace: %v", err)
	}

	workspace.SourceVideoRights.Status = RightsPending
	if err := workspace.Validate(); err == nil {
		t.Fatal("workspace accepted an unconfirmed source video")
	}
}

func TestUpgradeCommercePrerollV2WorkspaceBackfillsUserVersionFields(t *testing.T) {
	t.Parallel()

	result := testCommercePrerollV2AnalysisResult()
	asset := contract.ProjectAssetRef{ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: "asset_product", Version: 1}}
	workspace := CommercePrerollV2Workspace{
		ContractVersion: CommercePrerollV2ContractVersion,
		TaskID:          "creativetask_1",
		Revision:        5,
		Analysis:        CommercePrerollV2Analysis{Content: result.Content},
		ProductReference: &CommercePrerollV2DerivedFrame{
			Status: CommercePrerollV2ResourceReady, Asset: &asset, TimestampMS: 6800,
		},
		HookBatch:   &CommercePrerollV2HookBatch{Items: buildCommercePrerollV2Hooks(result.Content), SelectedHookID: "frosted-reveal"},
		PromptDraft: &CommercePrerollV2PromptDraft{HookID: "frosted-reveal", DurationSeconds: 8, Beats: []CommercePrerollV2Beat{{ID: "hook"}, {ID: "change"}, {ID: "lockup"}}, CompiledPrompt: "legacy prompt"},
	}
	for index := range workspace.HookBatch.Items {
		workspace.HookBatch.Items[index].RecipeVersion = ""
	}

	upgradeCommercePrerollV2Workspace(&workspace)

	if workspace.ProductReferenceBatch == nil || workspace.ProductReferenceBatch.SelectedID == "" || workspace.PromptDraft.CreativePrompt != "legacy prompt" || len(workspace.PromptDraft.LockedConstraints) == 0 || !strings.Contains(workspace.PromptDraft.CompiledPrompt, "系统锁定约束") || workspace.PromptDraft.Beats[0].SubjectAction == "" || workspace.HookBatch.Items[0].RecipeVersion != "commerce-hook-recipe/v3" {
		t.Fatalf("legacy commerce workspace was not upgraded: %#v", workspace)
	}
}

func TestCommercePrerollFrameDerivationIDFitsAssetContract(t *testing.T) {
	t.Parallel()

	value := commercePrerollFrameDerivationID(
		"creativetask_f50b4934f7a7d4266cb7627a54d4c6a6-product-reference-product_candidate_1",
		"f768df8ddc918a62c3bf500cd8ed33fbdf3ee5e2f2212447b914491ffe4b8341",
	)
	if len(value) > 128 {
		t.Fatalf("derivation id length = %d, want <= 128: %s", len(value), value)
	}
	if !strings.HasSuffix(value, "f768df8ddc918a62c3bf500cd8ed33fbdf3ee5e2f2212447b914491ffe4b8341") {
		t.Fatalf("derivation id must retain the canonical source hash: %s", value)
	}
}

func TestCommerceOpeningAnchorUsesBoundedStableRenderIdentity(t *testing.T) {
	t.Parallel()

	writer := &commercePrerollV2RenderedImageWriterStub{}
	service := Service{ImageBaseAssets: commercePrerollV2ImageReaderStub{}, RenderedImages: writer}
	frame := CommercePrerollV2DerivedFrame{
		Asset:        &contract.ProjectAssetRef{ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: "asset_anchor", Version: 1}},
		DerivationID: "creativetask_cf9132290199b5fdf813f5607580900a-opening-anchor-f768df8ddc918a62c3bf500cd8ed33fbdf3ee5e2f2212447b914491ffe4b8341",
	}
	result, err := service.normalizeCommerceOpeningAnchor(context.Background(), testRequestContext().Actor, "project_1", "creativetask_cf9132290199b5fdf813f5607580900a", frame,
		ShortDramaModelCanvas{Width: 720, Height: 1280})
	if err != nil {
		t.Fatalf("normalize opening anchor: %v", err)
	}
	if result.ModelCanvasAsset == nil || len(writer.renderJobID) > 128 || !strings.HasPrefix(writer.renderJobID, "commerce-anchor-") {
		t.Fatalf("unexpected bounded render identity %q and result %#v", writer.renderJobID, result)
	}
}

func TestCreateCommercePrerollV2TaskFreezesSourceAndRestoresWorkspace(t *testing.T) {
	t.Parallel()

	service := testService()
	source := contract.AssetVersionRef{AssetID: "asset_source_ad", Version: 3}
	service.Assets = testAssetReader{snapshot: CreativeAssetSnapshot{
		Ref: source, Kind: contract.AssetVideo, MIMEType: "video/mp4", Ready: true,
		WidthPixels: 720, HeightPixels: 960, DurationMS: 27100,
		FrameRate: "30/1", VideoCodec: "h264", AudioCodec: "aac",
	}}
	rc := testRequestContext()

	intake, err := service.CreateIntake(context.Background(), rc, "project_1", "commerce-v2-intake", CreateIntakeRequest{
		Source: IntakeSourceManual, Format: FormatVideo, PerformanceMode: PerformanceModeCommercePreroll,
		Channel: ChannelDouyin, Objective: "为已完成的电商正片生成独立前贴", Audience: "电商短视频观众",
		CoreMessage: "从原视频事实生成钩子", CallToAction: "继续观看",
		Concept: "原视频理解驱动的电商前贴", Tone: []string{"清晰"}, VisualKeywords: []string{"商品连续"},
		Mandatory: []string{}, Prohibited: []string{"不得虚构卖点"},
		CreativeRoutes: []CreativeRouteSnapshot{{
			RouteID: ManualCommercePrerollV2RouteID, RouteType: PerformanceModeCommercePreroll,
			VideoPurpose: "performance", Channels: []string{"douyin"}, Reason: "source-video-driven commerce preroll",
			TargetDurationSeconds: 8, AspectRatio: "9:16", Resolution: "720p",
			SourceAssetRefs: []contract.AssetVersionRef{source}, RequiresHumanConfirmation: true,
		}},
		ManualCommercePrerollV2: &ManualCommercePrerollV2Input{
			SourceVideo: source, SourceVideoRights: RightsConfirmed,
		},
	})
	if err != nil {
		t.Fatalf("create commerce V2 intake: %v", err)
	}

	task, err := service.CreateVideoTask(context.Background(), rc.Actor, "project_1", intake.ID, CreateVideoTaskRequest{
		SelectedRouteID: ManualCommercePrerollV2RouteID, Channel: ChannelDouyin,
		Concept: "原视频理解驱动的电商前贴", Prompt: "等待原视频理解", SourceVideo: source,
		CallToAction: "继续观看", Mandatory: []string{}, Prohibited: []string{"不得虚构卖点"}, ConfirmRoute: true,
	})
	if err != nil {
		t.Fatalf("create commerce V2 task: %v", err)
	}

	detail, err := service.GetTaskDetail(context.Background(), rc.Actor, "project_1", task.ID)
	if err != nil {
		t.Fatalf("restore commerce V2 task: %v", err)
	}
	workspace := detail.VideoDraft.CommercePrerollV2
	if workspace == nil {
		t.Fatal("commerce V2 workspace is nil")
	}
	if workspace.ActiveStage != CommercePrerollV2StageSourceReady ||
		workspace.SourceVideo.AssetVersion != source || workspace.SourceMetadata.DurationMS != 27100 ||
		workspace.SourceVideoRights.Status != RightsConfirmed || workspace.SourceVideoRights.ConfirmedBy != rc.Actor.Principal.ID {
		t.Fatalf("unexpected restored commerce V2 workspace: %#v", workspace)
	}
	if detail.VideoDraft.CommercePreroll != nil {
		t.Fatal("commerce V2 task must not create the legacy fixed-fixture workspace")
	}

	service.ViralRemakes = service.Repository.(*memoryRepository)
	analysisResult := testCommercePrerollV2AnalysisResult()
	analysisResult.Content.ProductFrameCandidates = []CommercePrerollV2ProductFrameCandidate{
		{ID: "middle", TimestampMS: 6800, Frontality: 0.76, Sharpness: 0.82, Completeness: 0.91, LogoReadability: 0.74, Occlusion: 0.18, Overall: 0.78},
		{ID: "tail", TimestampMS: 22100, Frontality: 0.97, Sharpness: 0.95, Completeness: 0.98, LogoReadability: 0.92, Occlusion: 0.02, Overall: 0.96},
	}
	service.CommercePrerollV2Analyzer = commercePrerollV2AnalyzerStub{result: analysisResult}
	analyzed, err := service.AnalyzeCommercePrerollV2Source(context.Background(), rc.Actor, "project_1", task.ID,
		AnalyzeCommercePrerollV2SourceRequest{ExpectedRevision: detail.VideoDraft.Revision})
	if err != nil {
		t.Fatalf("analyze commerce source: %v", err)
	}
	if analyzed.VideoDraft.CommercePrerollV2.ActiveStage != CommercePrerollV2StageUnderstandingReady ||
		analyzed.VideoDraft.CommercePrerollV2.Analysis.Content.Product.Name != "蜂皇水" ||
		analyzed.VideoDraft.CommercePrerollV2.Analysis.Content.OpeningAnchorFrameMS != 900 {
		t.Fatalf("unexpected source understanding: %#v", analyzed.VideoDraft.CommercePrerollV2.Analysis)
	}
	service.GameEvidenceFrames = gameFrameExtractor{}
	service.DerivedAssets = &gameDerivedWriter{}
	prepared, err := service.PrepareCommercePrerollV2References(context.Background(), rc, "project_1", task.ID,
		PrepareCommercePrerollV2ReferencesRequest{ExpectedRevision: analyzed.VideoDraft.Revision})
	if err != nil {
		t.Fatalf("prepare commerce product references: %v", err)
	}
	productBatch := prepared.VideoDraft.CommercePrerollV2.ProductReferenceBatch
	if productBatch == nil || len(productBatch.Candidates) != 2 || productBatch.Candidates[0].ID != "tail" || productBatch.SelectedID != "tail" || productBatch.Candidates[0].Scores.Overall != 0.96 {
		t.Fatalf("expected highest-quality full-video product frame to be selected first: %#v", productBatch)
	}

	product := prepared.VideoDraft.CommercePrerollV2.Analysis.Content.Product
	product.Description = "用户确认后的商品描述"
	confirmed, err := service.ConfirmCommercePrerollV2Understanding(context.Background(), rc.Actor, "project_1", task.ID,
		ConfirmCommercePrerollV2UnderstandingRequest{ExpectedRevision: prepared.VideoDraft.Revision, Product: product})
	if err != nil {
		t.Fatalf("confirm commerce understanding: %v", err)
	}
	if confirmed.VideoDraft.CommercePrerollV2.ActiveStage != CommercePrerollV2StageUnderstandingConfirmed ||
		confirmed.VideoDraft.CommercePrerollV2.Analysis.Content.Product.Description != "用户确认后的商品描述" {
		t.Fatalf("unexpected confirmed understanding: %#v", confirmed.VideoDraft.CommercePrerollV2)
	}

	hooks, err := service.GenerateCommercePrerollV2Hooks(context.Background(), rc.Actor, "project_1", task.ID,
		GenerateCommercePrerollV2HooksRequest{ExpectedRevision: confirmed.VideoDraft.Revision})
	if err != nil {
		t.Fatalf("generate commerce hooks: %v", err)
	}
	if hooks.VideoDraft.CommercePrerollV2.HookBatch == nil || len(hooks.VideoDraft.CommercePrerollV2.HookBatch.Items) != 5 {
		t.Fatalf("expected five hook recipes: %#v", hooks.VideoDraft.CommercePrerollV2.HookBatch)
	}
	if primary := hooks.VideoDraft.CommercePrerollV2.HookBatch.Items[1]; primary.ID != "frosted-reveal" || primary.RecommendationLevel != "primary" || primary.MatchScore <= 0.8 || primary.VisualSignature == "" || primary.ContinuityPlan == "" {
		t.Fatalf("expected enriched, source-matched frosted reveal recommendation: %#v", primary)
	}

	planned, err := service.SelectCommercePrerollV2Hook(context.Background(), rc.Actor, "project_1", task.ID,
		SelectCommercePrerollV2HookRequest{ExpectedRevision: hooks.VideoDraft.Revision, HookID: "frosted-reveal", DurationSeconds: 9})
	if err != nil {
		t.Fatalf("select commerce hook: %v", err)
	}
	prompt := planned.VideoDraft.CommercePrerollV2.PromptDraft
	if prompt == nil || prompt.DurationSeconds != 9 || len(prompt.Beats) != 3 ||
		!strings.Contains(prompt.CompiledPrompt, "蜂皇水") || !strings.Contains(prompt.CompiledPrompt, "暖金色高端护肤") {
		t.Fatalf("unexpected compiled commerce prompt: %#v", prompt)
	}
	editedBeats := append([]CommercePrerollV2Beat{}, prompt.Beats...)
	editedBeats[0].VisualDescription = "用户编辑的近景开场"
	storyboard, err := service.UpdateCommercePrerollV2Storyboard(context.Background(), rc.Actor, "project_1", task.ID,
		UpdateCommercePrerollV2StoryboardRequest{ExpectedRevision: planned.VideoDraft.Revision, Beats: editedBeats})
	if err != nil {
		t.Fatalf("update commerce storyboard: %v", err)
	}
	manualPrompt, err := service.UpdateCommercePrerollV2Prompt(context.Background(), rc.Actor, "project_1", task.ID,
		UpdateCommercePrerollV2PromptRequest{ExpectedRevision: storyboard.VideoDraft.Revision, CreativePrompt: "用户可编辑的三镜头创意描述"})
	if err != nil {
		t.Fatalf("update commerce prompt: %v", err)
	}
	sealedPrompt := manualPrompt.VideoDraft.CommercePrerollV2.PromptDraft
	if sealedPrompt.EditMode != "manual_creative_override" || !strings.Contains(sealedPrompt.CompiledPrompt, "用户可编辑") || !strings.Contains(sealedPrompt.CompiledPrompt, "系统锁定约束") || len(sealedPrompt.LockedConstraints) == 0 {
		t.Fatalf("manual prompt did not preserve server-owned constraints: %#v", sealedPrompt)
	}
	version, replayed, err := service.SaveCommercePrerollV2Version(context.Background(), rc, "project_1", task.ID,
		SaveCommercePrerollV2VersionRequest{ExpectedRevision: manualPrompt.VideoDraft.Revision, ExpectedTaskVersion: manualPrompt.Task.Version, DisplayName: "娇兰电商前贴草稿"}, "commerce-version-1")
	if err != nil || replayed || version.Version != 1 || version.VideoSnapshot == nil || version.VideoSnapshot.CommercePrerollV2 == nil {
		t.Fatalf("save user-visible commerce version: version=%#v replayed=%v err=%v", version, replayed, err)
	}
	versions, err := service.ListCommercePrerollV2Versions(context.Background(), rc.Actor, "project_1", task.ID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("list commerce versions: %#v err=%v", versions, err)
	}
	restored, err := service.RestoreCommercePrerollV2Version(context.Background(), rc.Actor, "project_1", task.ID,
		RestoreCommercePrerollV2VersionRequest{ExpectedRevision: manualPrompt.VideoDraft.Revision, VersionID: version.ID})
	if err != nil || restored.VideoDraft.CommercePrerollV2.PromptDraft.CreativePrompt != sealedPrompt.CreativePrompt {
		t.Fatalf("restore immutable commerce version: %#v err=%v", restored.VideoDraft.CommercePrerollV2, err)
	}
	planned = restored

	productReference := contract.ProjectAssetRef{ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: "asset_product_reference", Version: 1}}
	firstFrame := contract.ProjectAssetRef{ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: "asset_first_frame", Version: 1}}
	firstFrameModel := contract.ProjectAssetRef{ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: "asset_first_frame_model", Version: 1}}
	firstFrameOutput := contract.ProjectAssetRef{ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: "asset_first_frame_output", Version: 1}}
	openingAnchor := contract.ProjectAssetRef{ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: "asset_opening_anchor", Version: 1}}
	openingAnchorModel := contract.ProjectAssetRef{ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: "asset_opening_anchor_model", Version: 1}}
	next := *planned.VideoDraft
	workspaceCopy := *planned.VideoDraft.CommercePrerollV2
	next.Revision++
	workspaceCopy.Revision = next.Revision
	workspaceCopy.ProductReference = &CommercePrerollV2DerivedFrame{Status: CommercePrerollV2ResourceReady, Asset: &productReference, TimestampMS: 6800}
	workspaceCopy.OpeningAnchor = &CommercePrerollV2DerivedFrame{Status: CommercePrerollV2ResourceReady, Asset: &openingAnchor, ModelCanvasAsset: &openingAnchorModel, TimestampMS: 900}
	workspaceCopy.UpdatedAt = service.now()
	next.CommercePrerollV2 = &workspaceCopy
	if _, err := service.ViralRemakes.ReviseVideoDraft(context.Background(), rc.Actor.OrganizationID, "project_1", task.ID, planned.VideoDraft.Revision, next, TaskInProgress); err != nil {
		t.Fatalf("seed commerce frame references: %v", err)
	}
	imageJobs := &commercePrerollV2ImageJobsStub{}
	service.CommercePrerollV2Images = imageJobs
	generated, err := service.GenerateCommercePrerollV2FirstFrames(context.Background(), rc.Actor, "project_1", task.ID,
		GenerateCommercePrerollV2FirstFramesRequest{ExpectedRevision: next.Revision})
	if err != nil {
		t.Fatalf("generate commerce first frames: %v", err)
	}
	if len(imageJobs.requests) != 3 {
		t.Fatalf("first-frame requests = %d, want 3", len(imageJobs.requests))
	}
	for _, request := range imageJobs.requests {
		if request.Width != 1024 || request.Height != 1536 || !strings.Contains(request.Prompt, "中央 9:16 模型区") || !strings.Contains(request.Prompt, "中央 3:4 成片安全区") {
			t.Fatalf("unexpected model-safe first-frame request: %#v", request)
		}
	}
	generatedRevision := generated.VideoDraft.Revision
	pendingFrames, err := service.ReconcileCommercePrerollV2FirstFrame(context.Background(), rc.Actor, "project_1", task.ID,
		ReconcileCommercePrerollV2FirstFrameRequest{ExpectedRevision: generatedRevision, CandidateID: generated.VideoDraft.CommercePrerollV2.FirstFrameBatch.Candidates[0].ID, Job: contract.ProviderJob{
			ID: generated.VideoDraft.CommercePrerollV2.FirstFrameBatch.Candidates[0].ProviderJobID, ProjectID: "project_1", ProviderStatus: contract.ProviderJobSubmitted,
		}})
	if err != nil || pendingFrames.VideoDraft.Revision != generatedRevision {
		t.Fatalf("pending first-frame reconciliation changed the draft: revision=%d err=%v", pendingFrames.VideoDraft.Revision, err)
	}
	ready := *generated.VideoDraft
	readyWorkspace := *generated.VideoDraft.CommercePrerollV2
	ready.Revision++
	readyWorkspace.Revision = ready.Revision
	batch := *readyWorkspace.FirstFrameBatch
	batch.Candidates = append([]CommercePrerollV2FirstFrameCandidate{}, batch.Candidates...)
	batch.Status = CommercePrerollV2ResourceReady
	batch.Candidates[0].Status, batch.Candidates[0].Asset = CommercePrerollV2ResourceReady, &firstFrame
	batch.Candidates[0].ModelCanvasAsset, batch.Candidates[0].OutputCanvasAsset = &firstFrameModel, &firstFrameOutput
	readyWorkspace.FirstFrameBatch = &batch
	readyWorkspace.ActiveStage = CommercePrerollV2StageFrameReady
	readyWorkspace.UpdatedAt = service.now()
	ready.CommercePrerollV2 = &readyWorkspace
	if _, err := service.ViralRemakes.ReviseVideoDraft(context.Background(), rc.Actor.OrganizationID, "project_1", task.ID, generated.VideoDraft.Revision, ready, TaskInProgress); err != nil {
		t.Fatalf("seed ready first-frame candidate: %v", err)
	}
	selected, err := service.SelectCommercePrerollV2FirstFrame(context.Background(), rc.Actor, "project_1", task.ID,
		SelectCommercePrerollV2FirstFrameRequest{ExpectedRevision: ready.Revision, BatchID: batch.ID, CandidateID: batch.Candidates[0].ID})
	if err != nil {
		t.Fatalf("select first frame: %v", err)
	}
	providerInput, promptHash, specHash, err := service.CommercePrerollV2ProviderInput(context.Background(), rc.Actor, "project_1", task.ID)
	if err != nil {
		t.Fatalf("build provider input: %v", err)
	}
	if providerInput.InputMode != "reference_image" || len(providerInput.ConditioningAssets) != 1 ||
		providerInput.ConditioningAssets[0].Role != "reference_image" || providerInput.ConditioningAssets[0].Reference != firstFrameModel ||
		promptHash == "" || specHash == "" || selected.VideoDraft.CommercePrerollV2.GenerationSpec == nil {
		t.Fatalf("unexpected Seedance provider input: %#v", providerInput)
	}
	spec := selected.VideoDraft.CommercePrerollV2.GenerationSpec
	if spec.ModelCanvas == nil || spec.OutputCanvas == nil || spec.SourceCanvas == nil ||
		spec.ModelCanvas.Ratio != "9:16" || spec.ModelCanvas.Width != 720 || spec.ModelCanvas.Height != 1280 ||
		spec.OutputCanvas.Width != 720 || spec.OutputCanvas.Height != 960 || spec.OutputCanvas.AspectNum != 3 || spec.OutputCanvas.AspectDen != 4 ||
		spec.FirstFrameAsset != firstFrameModel || spec.LastFrameAsset.AssetVersion.AssetID != "" {
		t.Fatalf("unexpected 3:4 commerce canvas plan: %#v", spec)
	}

	reserved, err := service.ReserveCommercePrerollV2VideoOperation(context.Background(), rc.Actor, "project_1", task.ID, selected.VideoDraft.Revision, "operation_1")
	if err != nil {
		t.Fatalf("reserve video operation: %v", err)
	}
	if reserved.VideoDraft.CommercePrerollV2.LatestVideoAttemptID != "pending:operation_1" || reserved.VideoDraft.CommercePrerollV2.ActiveStage != CommercePrerollV2StageVideoGenerating {
		t.Fatalf("unexpected reserved video operation: %#v", reserved.VideoDraft.CommercePrerollV2)
	}
	if _, err := service.ReserveCommercePrerollV2VideoOperation(context.Background(), rc.Actor, "project_1", task.ID, selected.VideoDraft.Revision, "operation_2"); err != ErrVersionConflict {
		t.Fatalf("stale concurrent reservation error = %v, want %v", err, ErrVersionConflict)
	}
	registered, err := service.RegisterCommercePrerollV2VideoJob(context.Background(), rc.Actor, "project_1", task.ID, reserved.VideoDraft.Revision, "operation_1", "provider_job_1")
	if err != nil {
		t.Fatalf("register provider job: %v", err)
	}
	if registered.VideoDraft.CommercePrerollV2.LatestVideoAttemptID != "provider_job_1" {
		t.Fatalf("provider job was not attached: %#v", registered.VideoDraft.CommercePrerollV2)
	}
	registeredRevision := registered.VideoDraft.Revision
	pendingVideo, err := service.ReconcileCommercePrerollV2Video(context.Background(), rc.Actor, "project_1", task.ID,
		ReconcileCommercePrerollV2VideoRequest{ExpectedRevision: registeredRevision, Job: contract.ProviderJob{ID: "provider_job_1", ProjectID: "project_1", ProviderStatus: contract.ProviderJobSubmitted}})
	if err != nil || pendingVideo.VideoDraft.Revision != registeredRevision {
		t.Fatalf("pending video reconciliation changed the draft: revision=%d err=%v", pendingVideo.VideoDraft.Revision, err)
	}
	rawOutput := contract.ProjectAssetRef{ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: "asset_seedance_raw", Version: 1}}
	normalizedOutput := contract.ProjectAssetRef{ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: "asset_preroll_3x4", Version: 1}}
	normalizer := &commercePrerollV2VideoNormalizerStub{}
	rendered := &commercePrerollV2RenderedVideoWriterStub{ref: normalizedOutput}
	service.CommercePrerollV2OutputNormalizer = normalizer
	service.RenderedAssets = rendered
	reconciled, err := service.ReconcileCommercePrerollV2Video(context.Background(), rc.Actor, "project_1", task.ID,
		ReconcileCommercePrerollV2VideoRequest{ExpectedRevision: registered.VideoDraft.Revision, Job: contract.ProviderJob{
			ID: "provider_job_1", ProjectID: "project_1", ProviderStatus: contract.ProviderJobSucceeded, ProjectAssetRefs: []contract.ProjectAssetRef{rawOutput},
		}})
	if err != nil {
		t.Fatalf("reconcile normalized commerce output: %v", err)
	}
	result := reconciled.VideoDraft.CommercePrerollV2
	if result.RawOutputAsset == nil || *result.RawOutputAsset != rawOutput || result.OutputAsset == nil || *result.OutputAsset != normalizedOutput ||
		result.OutputNormalization == nil || result.OutputNormalization.Status != CommercePrerollV2ResourceReady {
		t.Fatalf("commerce output normalization was not persisted: %#v", result)
	}
	if normalizer.request.Width != 720 || normalizer.request.Height != 960 || normalizer.request.SourceVideo != rawOutput.AssetVersion {
		t.Fatalf("unexpected normalization request: %#v", normalizer.request)
	}
}

type commercePrerollV2AnalyzerStub struct {
	result CommercePrerollV2AnalysisResult
	err    error
}

type commercePrerollV2ImageJobsStub struct {
	requests []CommercePrerollV2FirstFrameJobRequest
}

type commercePrerollV2VideoNormalizerStub struct {
	request media.VideoNormalizationRequest
}

func (s *commercePrerollV2VideoNormalizerStub) NormalizeVideo(_ context.Context, request media.VideoNormalizationRequest) (media.CompositionOutput, error) {
	s.request = request
	content := "normalized-commerce-video"
	return media.CompositionOutput{
		Content: io.NopCloser(strings.NewReader(content)), SizeBytes: int64(len(content)),
		Metadata: assets.VideoMetadata{DurationMS: 8000, WidthPixels: request.Width, HeightPixels: request.Height, FrameRate: "30/1", VideoCodec: "h264", AudioCodec: "aac"},
	}, nil
}

type commercePrerollV2RenderedVideoWriterStub struct {
	ref contract.ProjectAssetRef
}

type commercePrerollV2ImageReaderStub struct{}

func (commercePrerollV2ImageReaderStub) OpenImage(context.Context, contract.ActorContext, contract.ProjectID, contract.AssetVersionRef) (io.ReadCloser, error) {
	canvas := image.NewRGBA(image.Rect(0, 0, 720, 960))
	for y := 0; y < 960; y++ {
		for x := 0; x < 720; x++ {
			canvas.SetRGBA(x, y, color.RGBA{R: 80, G: uint8(x % 255), B: uint8(y % 255), A: 255})
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(output.Bytes())), nil
}

type commercePrerollV2RenderedImageWriterStub struct {
	renderJobID string
}

func (s *commercePrerollV2RenderedImageWriterStub) IngestRenderedImage(_ context.Context, _ contract.RequestContext, projectID contract.ProjectID, renderJobID string, _ io.Reader, _ int64, _, _ int, _ []contract.AssetVersionRef, _ []contract.ResourceRef) (contract.ProjectAssetRef, error) {
	s.renderJobID = renderJobID
	if len(renderJobID) > 128 {
		return contract.ProjectAssetRef{}, fmt.Errorf("render job id exceeds 128 characters")
	}
	return contract.ProjectAssetRef{ProjectID: projectID, AssetVersion: contract.AssetVersionRef{AssetID: "asset_normalized_anchor", Version: 1}}, nil
}

func (s *commercePrerollV2RenderedVideoWriterStub) IngestRenderedVideo(_ context.Context, _ contract.RequestContext, _ contract.ProjectID, _ string, _ io.Reader, _ int64) (contract.ProjectAssetRef, error) {
	return s.ref, nil
}

func (s *commercePrerollV2ImageJobsStub) CreateCommerceFirstFrameJob(_ context.Context, _ contract.ActorContext, project contract.ProjectContext, request CommercePrerollV2FirstFrameJobRequest) (contract.ProviderJob, error) {
	s.requests = append(s.requests, request)
	return contract.ProviderJob{ID: fmt.Sprintf("provider_image_%d", len(s.requests)), ProjectID: project.ProjectID}, nil
}

func (s commercePrerollV2AnalyzerStub) Analyze(_ context.Context, _ contract.ActorContext, _ contract.ProjectContext, _ contract.ProjectAssetRef) (CommercePrerollV2AnalysisResult, error) {
	return s.result, s.err
}

func testCommercePrerollV2AnalysisResult() CommercePrerollV2AnalysisResult {
	return CommercePrerollV2AnalysisResult{
		InputHash: "sha256:analysis", PromptVersion: "commerce-preroll-analysis/v1",
		Content: CommercePrerollV2SourceUnderstanding{
			Product: CommercePrerollV2ProductFacts{
				Name: "蜂皇水", Category: "精华水", Description: "金色护肤精华水",
				SellingPoints: []string{"水油修护"}, AppearanceGuardrails: []string{"保持金色瓶身与标签"},
				LogoGuardrails: []string{"不得改写品牌字样"},
			},
			VisualStyle: "暖金色高端护肤", SubtitleSummary: "白色居中字幕", VoiceSummary: "女性口播",
			AudioMood: "克制舒缓", OpeningShot: "商品正面近景", ProductFrameMS: 6800, OpeningAnchorFrameMS: 900,
			Evidence: []CommercePrerollV2Evidence{{
				ID: "frame_1", TimestampMS: 900, Source: "frame", Excerpt: "商品正面出现", Confidence: 0.96,
			}}, Risks: []string{},
		},
	}
}

func TestCommercePrerollV2RouteAcceptsSixToTenSecondsWithoutRelaxingV1(t *testing.T) {
	t.Parallel()

	source := contract.AssetVersionRef{AssetID: "asset_source_ad", Version: 1}
	v2 := CreativeRouteSnapshot{
		RouteID: ManualCommercePrerollV2RouteID, RouteType: PerformanceModeCommercePreroll,
		VideoPurpose: "performance", Channels: []string{"douyin"}, Reason: "source-video-driven commerce preroll",
		TargetDurationSeconds: 10, AspectRatio: "9:16", Resolution: "720p",
		SourceAssetRefs: []contract.AssetVersionRef{source}, RequiresHumanConfirmation: true,
	}
	if err := v2.Validate(); err != nil {
		t.Fatalf("V2 route at 10 seconds: %v", err)
	}

	v1 := v2
	v1.RouteID = ManualCommercePrerollRouteID
	if err := v1.Validate(); err == nil {
		t.Fatal("legacy commerce preroll route unexpectedly accepted 10 seconds")
	}
}
