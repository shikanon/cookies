package creative

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/media"
	"github.com/shikanon/cookies/internal/platform/provider"
)

func TestManualBrandFilmDocumentInputValidation(t *testing.T) {
	input := ManualBrandFilmInput{
		DocumentID: "knowledgedoc_1", FixtureHash: "sha256:" + strings.Repeat("a", 64),
		BriefName: "brand-brief.pdf", BriefText: "品牌目标与受众", ProductName: "新品品牌广告",
	}
	if err := input.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	input.BriefText = ""
	if err := input.Validate(); err == nil {
		t.Fatal("Validate() expected incomplete document error")
	}
}

func TestBrandFilmFixtureCompletesPersistentPhaseZeroToTwo(t *testing.T) {
	t.Parallel()
	service := testService()
	repository := service.Repository.(*memoryRepository)
	service.ViralRemakes = repository
	service.BrandFilmPlanner = DeterministicBrandFilmPlanner{}
	ctx := context.Background()
	rc := testRequestContext()

	workspace, err := service.EnsureBrandFilmFixtureWorkspace(ctx, rc, "project_1", "brand-film-fixture-1")
	if err != nil {
		t.Fatal(err)
	}
	if workspace.VideoDraft == nil || workspace.VideoDraft.BrandFilm == nil {
		t.Fatal("brand film draft was not created")
	}
	taskID := workspace.Task.ID
	if workspace.VideoDraft.BrandFilm.PromptSeam.ContractVersion != "creative-brand-generation-seam/v1" {
		t.Fatalf("generation seam = %#v", workspace.VideoDraft.BrandFilm.PromptSeam)
	}

	workspace, err = service.AnalyzeBrandFilmBrief(ctx, rc.Actor, "project_1", taskID, BrandFilmRevisionRequest{ExpectedRevision: workspace.VideoDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	analysis := *workspace.VideoDraft.BrandFilm.CurrentAnalysis()
	if analysis.ModelAlias != "fixture.deterministic" || len(analysis.AssetCandidates) == 0 {
		t.Fatalf("analysis = %#v", analysis)
	}
	for index := range analysis.AssetCandidates {
		if analysis.AssetCandidates[index].Role == "product_front" || analysis.AssetCandidates[index].Role == "logo" {
			analysis.AssetCandidates[index].UserConfirmed = true
			analysis.AssetCandidates[index].RightsStatus = "user_confirmed"
		}
	}
	workspace, err = service.UpdateBrandFilmBrief(ctx, rc.Actor, "project_1", taskID, UpdateBrandBriefAnalysisRequest{
		ExpectedRevision: workspace.VideoDraft.Revision,
		Analysis:         analysis,
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err = service.ConfirmBrandFilmBrief(ctx, rc.Actor, "project_1", taskID, BrandFilmRevisionRequest{ExpectedRevision: workspace.VideoDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	conceptBaseRevision := workspace.VideoDraft.Revision
	workspace, err = service.GenerateBrandFilmConcepts(ctx, rc.Actor, "project_1", taskID, BrandFilmRevisionRequest{ExpectedRevision: conceptBaseRevision})
	if err != nil {
		t.Fatal(err)
	}
	if _, staleErr := service.GenerateBrandFilmConcepts(ctx, rc.Actor, "project_1", taskID, BrandFilmRevisionRequest{ExpectedRevision: conceptBaseRevision}); !errors.Is(staleErr, ErrVersionConflict) {
		t.Fatalf("stale concept generation error = %v, want ErrVersionConflict", staleErr)
	}
	concepts := workspace.VideoDraft.BrandFilm.CurrentConceptSet()
	if concepts == nil || len(concepts.Candidates) != 3 || workspace.VideoDraft.BrandFilm.SelectedConceptID != "" {
		t.Fatalf("concepts = %#v", concepts)
	}
	workspace, err = service.SelectBrandFilmConcept(ctx, rc.Actor, "project_1", taskID, SelectBrandConceptRequest{
		ExpectedRevision: workspace.VideoDraft.Revision,
		ConceptID:        concepts.Candidates[0].ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err = service.GenerateBrandFilmPlan(ctx, rc.Actor, "project_1", taskID, BrandFilmRevisionRequest{ExpectedRevision: workspace.VideoDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	plan := workspace.VideoDraft.BrandFilm.CurrentPlan()
	if plan == nil || len(plan.Shots) != 3 || plan.Shots[len(plan.Shots)-1].EndSecond != 15 {
		t.Fatalf("plan = %#v", plan)
	}
	workspace, err = service.ConfirmBrandFilmPlan(ctx, rc.Actor, "project_1", taskID, BrandFilmRevisionRequest{ExpectedRevision: workspace.VideoDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	brand := workspace.VideoDraft.BrandFilm
	if brand.Stage != BrandFilmPlanConfirmed || !brand.CurrentPlan().Confirmed || brand.Readiness.GenerationReady {
		t.Fatalf("completed workspace = %#v", brand)
	}
	if len(workspace.ProductionJobs) != 0 {
		t.Fatalf("Phase 0-2 must not create provider jobs: %#v", workspace.ProductionJobs)
	}

	restored, err := service.GetLatestBrandFilmWorkspace(ctx, rc.Actor, "project_1")
	if err != nil {
		t.Fatal(err)
	}
	if restored.VideoDraft.Revision != workspace.VideoDraft.Revision || restored.VideoDraft.BrandFilm.Stage != BrandFilmPlanConfirmed {
		t.Fatalf("restored workspace = %#v", restored.VideoDraft.BrandFilm)
	}
}

func TestBrandFilmConfirmedBriefCanBeRevisedAndInvalidatesDownstreamWork(t *testing.T) {
	t.Parallel()
	service := testService()
	repository := service.Repository.(*memoryRepository)
	service.ViralRemakes = repository
	service.BrandFilmPlanner = DeterministicBrandFilmPlanner{}
	ctx, rc := context.Background(), testRequestContext()
	workspace := completeBrandFilmPlanForTest(t, service, ctx, rc)

	analysis := *workspace.VideoDraft.BrandFilm.CurrentAnalysis()
	analysis.Summary += " 用户修订。"
	for index := range analysis.AssetCandidates {
		if analysis.AssetCandidates[index].Role == "logo" {
			analysis.AssetCandidates[index].UserConfirmed = false
			analysis.AssetCandidates[index].RightsStatus = "needs_confirmation"
		}
	}
	workspace, err := service.UpdateBrandFilmBrief(ctx, rc.Actor, "project_1", workspace.Task.ID, UpdateBrandBriefAnalysisRequest{
		ExpectedRevision: workspace.VideoDraft.Revision,
		Analysis:         analysis,
	})
	if err != nil {
		t.Fatal(err)
	}
	brand := workspace.VideoDraft.BrandFilm
	if brand.Stage != BrandFilmBriefDraft || brand.CurrentAnalysis().Confirmed || len(brand.ConceptSets) != 0 ||
		brand.SelectedConceptID != "" || len(brand.FilmPlans) != 0 || brand.Generation != nil || len(brand.QualityRuns) != 0 || brand.Delivery != nil {
		t.Fatalf("revised Brief retained stale downstream state: %#v", brand)
	}
	if _, err = service.ConfirmBrandFilmBrief(ctx, rc.Actor, "project_1", workspace.Task.ID, BrandFilmRevisionRequest{ExpectedRevision: workspace.VideoDraft.Revision}); err == nil {
		t.Fatal("Brief confirmation succeeded without an explicit logo confirmation")
	}
}

func TestBrandFilmConfirmedConceptsAndPlanCanBeRevised(t *testing.T) {
	t.Parallel()
	service := testService()
	repository := service.Repository.(*memoryRepository)
	service.ViralRemakes = repository
	service.BrandFilmPlanner = DeterministicBrandFilmPlanner{}
	ctx, rc := context.Background(), testRequestContext()
	workspace := completeBrandFilmPlanForTest(t, service, ctx, rc)

	candidates := append([]BrandCreativeConcept{}, workspace.VideoDraft.BrandFilm.CurrentConceptSet().Candidates...)
	candidates[0].Title = "人工修订创意方向"
	workspace, err := service.UpdateBrandFilmConcepts(ctx, rc.Actor, "project_1", workspace.Task.ID, UpdateBrandConceptsRequest{
		ExpectedRevision: workspace.VideoDraft.Revision,
		Candidates:       candidates,
	})
	if err != nil {
		t.Fatal(err)
	}
	brand := workspace.VideoDraft.BrandFilm
	if brand.Stage != BrandFilmConceptSelection || brand.SelectedConceptID != "" || len(brand.FilmPlans) != 0 || brand.CurrentConceptSet().Candidates[0].Title != "人工修订创意方向" {
		t.Fatalf("revised concepts retained stale selection or plan: %#v", brand)
	}

	workspace, err = service.SelectBrandFilmConcept(ctx, rc.Actor, "project_1", workspace.Task.ID, SelectBrandConceptRequest{ExpectedRevision: workspace.VideoDraft.Revision, ConceptID: candidates[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err = service.GenerateBrandFilmPlan(ctx, rc.Actor, "project_1", workspace.Task.ID, BrandFilmRevisionRequest{ExpectedRevision: workspace.VideoDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err = service.ConfirmBrandFilmPlan(ctx, rc.Actor, "project_1", workspace.Task.ID, BrandFilmRevisionRequest{ExpectedRevision: workspace.VideoDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err = service.PrepareBrandFilmGeneration(ctx, rc.Actor, "project_1", workspace.Task.ID, PrepareBrandFilmGenerationRequest{ExpectedRevision: workspace.VideoDraft.Revision, ReferenceAsset: contract.AssetVersionRef{AssetID: "asset_reference", Version: 1}})
	if err != nil {
		t.Fatal(err)
	}
	plan := *workspace.VideoDraft.BrandFilm.CurrentPlan()
	plan.StorySummary = "人工修订后的故事概要。"
	workspace, err = service.UpdateBrandFilmPlan(ctx, rc.Actor, "project_1", workspace.Task.ID, UpdateBrandFilmPlanRequest{ExpectedRevision: workspace.VideoDraft.Revision, Plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	brand = workspace.VideoDraft.BrandFilm
	if brand.Stage != BrandFilmPlanDraft || brand.CurrentPlan().Confirmed || brand.Generation != nil || brand.CurrentPlan().StorySummary != "人工修订后的故事概要。" {
		t.Fatalf("revised plan retained stale generation state: %#v", brand)
	}
}

func TestBrandFilmPhaseThreePersistsGenerationAttemptsFeedbackAndLocks(t *testing.T) {
	t.Parallel()
	service := testService()
	repository := service.Repository.(*memoryRepository)
	service.ViralRemakes = repository
	service.BrandFilmPlanner = DeterministicBrandFilmPlanner{}
	ctx, rc := context.Background(), testRequestContext()
	workspace := completeBrandFilmPlanForTest(t, service, ctx, rc)

	workspace, err := service.PrepareBrandFilmGeneration(ctx, rc.Actor, "project_1", workspace.Task.ID, PrepareBrandFilmGenerationRequest{
		ExpectedRevision: workspace.VideoDraft.Revision,
		ReferenceAsset:   contract.AssetVersionRef{AssetID: "asset_guerlain_product", Version: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	generation := workspace.VideoDraft.BrandFilm.Generation
	if generation == nil || len(generation.Units) != 3 || !workspace.VideoDraft.BrandFilm.Readiness.GenerationReady {
		t.Fatalf("generation = %#v", generation)
	}
	for index, duration := range []int{5, 5, 5} {
		unit := generation.Units[index]
		if unit.EndSecond-unit.StartSecond != duration || len(unit.PromptPackages) != 1 || unit.PromptPackages[0].ContentHash == "" {
			t.Fatalf("unit %d = %#v", index, unit)
		}
	}

	unitID := generation.Units[0].ID
	input, promptHash, err := service.BrandFilmProviderInput(ctx, rc.Actor, "project_1", workspace.Task.ID, unitID)
	if err != nil {
		t.Fatal(err)
	}
	if input.InputMode != "reference_image" || input.AudioPolicy != provider.VideoAudioSilent || input.DurationSeconds != 5 ||
		input.AspectRatio != workspace.VideoDraft.BrandFilm.SourceSnapshot.AspectRatio || input.Resolution != "720p" ||
		promptHash != generation.Units[0].PromptPackages[0].ContentHash {
		t.Fatalf("provider input = %#v hash=%s", input, promptHash)
	}
	workspace, err = service.RegisterBrandFilmGenerationAttempt(ctx, rc.Actor, "project_1", workspace.Task.ID, unitID, "provider_job_1")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err = service.ReconcileBrandFilmGenerationAttempt(ctx, rc.Actor, "project_1", workspace.Task.ID, unitID, contract.ProviderJob{
		ID: "provider_job_1", ProjectID: "project_1", ProviderStatus: contract.ProviderJobSucceeded,
		ProjectAssetRefs: []contract.ProjectAssetRef{{ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: "asset_segment_1", Version: 1}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	attempt := workspace.VideoDraft.BrandFilm.Generation.Units[0].Attempts[0]
	if attempt.OutputAssetRef == nil || attempt.Status != string(contract.ProviderJobSucceeded) {
		t.Fatalf("attempt = %#v", attempt)
	}
	workspace, err = service.RegenerateBrandFilmUnit(ctx, rc.Actor, "project_1", workspace.Task.ID, RegenerateBrandFilmUnitRequest{
		ExpectedRevision: workspace.VideoDraft.Revision, UnitID: unitID, Feedback: "瓶身标签更稳定，减少镜头环绕",
	})
	if err != nil {
		t.Fatal(err)
	}
	packages := workspace.VideoDraft.BrandFilm.Generation.Units[0].PromptPackages
	if len(packages) != 2 || packages[1].ContentHash == packages[0].ContentHash || packages[1].Feedback == "" {
		t.Fatalf("packages = %#v", packages)
	}

	workspace, err = service.LockBrandFilmGenerationUnit(ctx, rc.Actor, "project_1", workspace.Task.ID, LockBrandFilmUnitRequest{
		ExpectedRevision: workspace.VideoDraft.Revision, UnitID: unitID, AttemptID: attempt.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if workspace.VideoDraft.BrandFilm.Generation.Units[0].LockedAttemptID != attempt.ID {
		t.Fatalf("lock was not persisted")
	}
	if _, err := service.RegenerateBrandFilmUnit(ctx, rc.Actor, "project_1", workspace.Task.ID, RegenerateBrandFilmUnitRequest{ExpectedRevision: workspace.VideoDraft.Revision, UnitID: unitID, Feedback: "should fail"}); err == nil {
		t.Fatal("locked unit accepted regeneration")
	}
}

func TestNormalizeBrandFilmProviderResolution(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"1080x1920": "720p",
		"1920×1080": "720p",
		"1080p":     "720p",
		"720x1280":  "720p",
		"480p":      "480p",
		"":          "720p",
		"original":  "720p",
	}
	for input, want := range tests {
		if got := normalizeBrandFilmProviderResolution(input); got != want {
			t.Errorf("normalizeBrandFilmProviderResolution(%q) = %q, want %q", input, got, want)
		}
	}
}

func completeBrandFilmPlanForTest(t *testing.T, service Service, ctx context.Context, rc contract.RequestContext) TaskDetail {
	t.Helper()
	workspace, err := service.EnsureBrandFilmFixtureWorkspace(ctx, rc, "project_1", "brand-film-phase3-fixture")
	if err != nil {
		t.Fatal(err)
	}
	workspace, err = service.AnalyzeBrandFilmBrief(ctx, rc.Actor, "project_1", workspace.Task.ID, BrandFilmRevisionRequest{ExpectedRevision: workspace.VideoDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	analysis := *workspace.VideoDraft.BrandFilm.CurrentAnalysis()
	for index := range analysis.AssetCandidates {
		if analysis.AssetCandidates[index].Role == "product_front" || analysis.AssetCandidates[index].Role == "logo" {
			analysis.AssetCandidates[index].UserConfirmed = true
			analysis.AssetCandidates[index].RightsStatus = "user_confirmed"
		}
	}
	workspace, err = service.UpdateBrandFilmBrief(ctx, rc.Actor, "project_1", workspace.Task.ID, UpdateBrandBriefAnalysisRequest{ExpectedRevision: workspace.VideoDraft.Revision, Analysis: analysis})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err = service.ConfirmBrandFilmBrief(ctx, rc.Actor, "project_1", workspace.Task.ID, BrandFilmRevisionRequest{ExpectedRevision: workspace.VideoDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err = service.GenerateBrandFilmConcepts(ctx, rc.Actor, "project_1", workspace.Task.ID, BrandFilmRevisionRequest{ExpectedRevision: workspace.VideoDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err = service.SelectBrandFilmConcept(ctx, rc.Actor, "project_1", workspace.Task.ID, SelectBrandConceptRequest{ExpectedRevision: workspace.VideoDraft.Revision, ConceptID: workspace.VideoDraft.BrandFilm.CurrentConceptSet().Candidates[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err = service.GenerateBrandFilmPlan(ctx, rc.Actor, "project_1", workspace.Task.ID, BrandFilmRevisionRequest{ExpectedRevision: workspace.VideoDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err = service.ConfirmBrandFilmPlan(ctx, rc.Actor, "project_1", workspace.Task.ID, BrandFilmRevisionRequest{ExpectedRevision: workspace.VideoDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	return workspace
}

func TestBrandFilmPhaseThreeComposesOnlyLockedOutputs(t *testing.T) {
	t.Parallel()
	service := testService()
	repository := service.Repository.(*memoryRepository)
	service.ViralRemakes = repository
	service.BrandFilmPlanner = DeterministicBrandFilmPlanner{}
	composer := &brandSegmentComposer{}
	service.BrandFilmComposer = composer
	service.RenderedAssets = &testRenderedAssetWriter{ref: contract.ProjectAssetRef{ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: "asset_brand_preview", Version: 1}}}
	ctx, rc := context.Background(), testRequestContext()
	workspace := completeBrandFilmPlanForTest(t, service, ctx, rc)
	workspace, err := service.PrepareBrandFilmGeneration(ctx, rc.Actor, "project_1", workspace.Task.ID, PrepareBrandFilmGenerationRequest{ExpectedRevision: workspace.VideoDraft.Revision, ReferenceAsset: contract.AssetVersionRef{AssetID: "asset_reference", Version: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ComposeBrandFilmPreview(ctx, rc, "project_1", workspace.Task.ID, ComposeBrandFilmPreviewRequest{ExpectedRevision: workspace.VideoDraft.Revision}); err == nil {
		t.Fatal("unlocked generation composed a preview")
	}
	for unitIndex := range workspace.VideoDraft.BrandFilm.Generation.Units {
		unitID := workspace.VideoDraft.BrandFilm.Generation.Units[unitIndex].ID
		jobID := "provider_job_" + unitID
		workspace, err = service.RegisterBrandFilmGenerationAttempt(ctx, rc.Actor, "project_1", workspace.Task.ID, unitID, jobID)
		if err != nil {
			t.Fatal(err)
		}
		workspace, err = service.ReconcileBrandFilmGenerationAttempt(ctx, rc.Actor, "project_1", workspace.Task.ID, unitID, contract.ProviderJob{ID: jobID, ProjectID: "project_1", ProviderStatus: contract.ProviderJobSucceeded, ProjectAssetRefs: []contract.ProjectAssetRef{{ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: contract.AssetID("asset_" + unitID), Version: 1}}}})
		if err != nil {
			t.Fatal(err)
		}
		attempt := workspace.VideoDraft.BrandFilm.Generation.Units[unitIndex].Attempts[0]
		workspace, err = service.LockBrandFilmGenerationUnit(ctx, rc.Actor, "project_1", workspace.Task.ID, LockBrandFilmUnitRequest{ExpectedRevision: workspace.VideoDraft.Revision, UnitID: unitID, AttemptID: attempt.ID})
		if err != nil {
			t.Fatal(err)
		}
	}
	workspace, err = service.ComposeBrandFilmPreview(ctx, rc, "project_1", workspace.Task.ID, ComposeBrandFilmPreviewRequest{ExpectedRevision: workspace.VideoDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if workspace.VideoDraft.BrandFilm.Generation.PreviewAsset == nil || workspace.VideoDraft.BrandFilm.Readiness.ProductionReady || len(composer.request.Segments) != 3 {
		t.Fatalf("composed workspace = %#v request=%#v", workspace.VideoDraft.BrandFilm, composer.request)
	}
	if got := workspace.VideoDraft.BrandFilm.Readiness.Blockers; len(got) != 2 || got[0] != "automatic_quality_check" {
		t.Fatalf("Phase 4 gates = %#v", got)
	}
}

func TestBrandFilmPhaseFourRequiresAutomaticAndHumanQualityBeforeDelivery(t *testing.T) {
	t.Parallel()
	service := testService()
	repository := service.Repository.(*memoryRepository)
	service.ViralRemakes = repository
	service.BrandFilmPlanner = DeterministicBrandFilmPlanner{}
	service.BrandFilmComposer = &brandSegmentComposer{}
	reference := contract.AssetVersionRef{AssetID: "asset_reference", Version: 1}
	preview := contract.AssetVersionRef{AssetID: "asset_brand_preview", Version: 1}
	service.Assets = testAssetReader{snapshots: map[contract.AssetID]CreativeAssetSnapshot{
		reference.AssetID: {Ref: reference, Kind: contract.AssetImage, MIMEType: "image/jpeg", Ready: true},
		preview.AssetID: {
			Ref: preview, Kind: contract.AssetVideo, MIMEType: "video/mp4", Ready: true,
			WidthPixels: 720, HeightPixels: 1280, DurationMS: 15021, FrameRate: "24/1", VideoCodec: "h264", AudioCodec: "aac",
		},
	}}
	service.RenderedAssets = &testRenderedAssetWriter{ref: contract.ProjectAssetRef{ProjectID: "project_1", AssetVersion: preview}}
	ctx, rc := context.Background(), testRequestContext()
	workspace := completeBrandFilmPlanForTest(t, service, ctx, rc)
	workspace, err := service.PrepareBrandFilmGeneration(ctx, rc.Actor, "project_1", workspace.Task.ID, PrepareBrandFilmGenerationRequest{ExpectedRevision: workspace.VideoDraft.Revision, ReferenceAsset: reference})
	if err != nil {
		t.Fatal(err)
	}
	for unitIndex := range workspace.VideoDraft.BrandFilm.Generation.Units {
		unitID := workspace.VideoDraft.BrandFilm.Generation.Units[unitIndex].ID
		jobID := "provider_job_" + unitID
		workspace, err = service.RegisterBrandFilmGenerationAttempt(ctx, rc.Actor, "project_1", workspace.Task.ID, unitID, jobID)
		if err != nil {
			t.Fatal(err)
		}
		workspace, err = service.ReconcileBrandFilmGenerationAttempt(ctx, rc.Actor, "project_1", workspace.Task.ID, unitID, contract.ProviderJob{ID: jobID, ProjectID: "project_1", ProviderStatus: contract.ProviderJobSucceeded, ProjectAssetRefs: []contract.ProjectAssetRef{{ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: contract.AssetID("asset_" + unitID), Version: 1}}}})
		if err != nil {
			t.Fatal(err)
		}
		attempt := workspace.VideoDraft.BrandFilm.Generation.Units[unitIndex].Attempts[0]
		workspace, err = service.LockBrandFilmGenerationUnit(ctx, rc.Actor, "project_1", workspace.Task.ID, LockBrandFilmUnitRequest{ExpectedRevision: workspace.VideoDraft.Revision, UnitID: unitID, AttemptID: attempt.ID})
		if err != nil {
			t.Fatal(err)
		}
	}
	workspace, err = service.ComposeBrandFilmPreview(ctx, rc, "project_1", workspace.Task.ID, ComposeBrandFilmPreviewRequest{ExpectedRevision: workspace.VideoDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err = service.RunBrandFilmQuality(ctx, rc.Actor, "project_1", workspace.Task.ID, RunBrandFilmQualityRequest{ExpectedRevision: workspace.VideoDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	run := workspace.VideoDraft.BrandFilm.QualityRuns[0]
	if !run.AutomaticPassed || run.Status != "awaiting_human" || run.Metrics.UnitCount != 3 || run.Metrics.SuccessRate != 1 {
		t.Fatalf("quality run = %#v", run)
	}
	if _, err := service.FinalizeBrandFilmVersion(ctx, rc, "project_1", workspace.Task.ID, BrandFilmVersionRequest{ExpectedRevision: workspace.VideoDraft.Revision}, "brand-film-version-before-human"); err == nil {
		t.Fatal("version frozen before human confirmation")
	}
	manual := make([]BrandFilmManualCheck, 0, len(requiredBrandFilmManualChecks))
	for _, code := range requiredBrandFilmManualChecks {
		manual = append(manual, BrandFilmManualCheck{Code: code, Passed: true, Note: "人工审核通过"})
	}
	workspace, err = service.ConfirmBrandFilmQuality(ctx, rc.Actor, "project_1", workspace.Task.ID, ConfirmBrandFilmQualityRequest{ExpectedRevision: workspace.VideoDraft.Revision, ManualChecks: manual})
	if err != nil {
		t.Fatal(err)
	}
	if workspace.VideoDraft.BrandFilm.Stage != BrandFilmReadyForReview || !workspace.VideoDraft.BrandFilm.Readiness.ProductionReady {
		t.Fatalf("human-confirmed workspace = %#v", workspace.VideoDraft.BrandFilm)
	}
	finalized, err := service.FinalizeBrandFilmVersion(ctx, rc, "project_1", workspace.Task.ID, BrandFilmVersionRequest{ExpectedRevision: workspace.VideoDraft.Revision}, "brand-film-version-1")
	if err != nil {
		t.Fatal(err)
	}
	if finalized.Version.Status != CreativeVersionChecked || finalized.Version.VideoSnapshot == nil || finalized.Version.VideoSnapshot.BrandFilm == nil {
		t.Fatalf("finalized version = %#v", finalized.Version)
	}
	approved, err := service.ApproveBrandFilmVersion(ctx, rc.Actor, "project_1", workspace.Task.ID, BrandFilmVersionRequest{ExpectedRevision: finalized.Workspace.VideoDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if approved.Version.Status != CreativeVersionApproved || approved.Workspace.VideoDraft.BrandFilm.Stage != BrandFilmApproved {
		t.Fatalf("approved result = %#v", approved)
	}
	delivered, err := service.DeliverBrandFilmVersion(ctx, rc.Actor, "project_1", workspace.Task.ID, BrandFilmVersionRequest{ExpectedRevision: approved.Workspace.VideoDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if delivered.Package.ID == "" || delivered.Package.VideoSnapshot == nil || delivered.Workspace.VideoDraft.BrandFilm.Stage != BrandFilmDelivered {
		t.Fatalf("delivered result = %#v", delivered)
	}
}

type brandSegmentComposer struct {
	request media.SegmentCompositionRequest
}

func (c *brandSegmentComposer) ComposeSegments(_ context.Context, request media.SegmentCompositionRequest) (media.CompositionOutput, error) {
	c.request = request
	return media.CompositionOutput{Content: io.NopCloser(bytes.NewReader([]byte("preview"))), SizeBytes: 7}, nil
}
