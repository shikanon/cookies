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
	"github.com/shikanon/cookies/internal/platform/provider"
)

func TestCreateShortDramaPrerollV2WorkspaceFreezesSourceVideo(t *testing.T) {
	t.Parallel()

	service := testService()
	service.ViralRemakes = service.Repository.(*memoryRepository)
	source := contract.AssetVersionRef{AssetID: "asset_wuzetian", Version: 1}
	service.Assets = testAssetReader{snapshot: CreativeAssetSnapshot{
		Ref: source, Kind: contract.AssetVideo, MIMEType: "video/mp4", Ready: true,
		WidthPixels: 1920, HeightPixels: 818, DurationMS: 182417,
		FrameRate: "30/1", VideoCodec: "h264", AudioCodec: "aac",
	}}
	rc := testRequestContext()

	intake, err := service.CreateIntake(context.Background(), rc, "project_1", "short-drama-v2-intake", CreateIntakeRequest{
		Source: IntakeSourceManual, Format: FormatVideo, PerformanceMode: PerformanceModeShortDramaPreroll,
		Channel: ChannelDouyin, Objective: "用独立前贴吸引用户观看短剧", Audience: "竖屏短剧观众",
		CoreMessage: "基于短剧真实剧情生成钩子", CallToAction: "点击观看正片",
		Concept: "短剧视频理解驱动的前贴", Tone: []string{"紧凑"}, VisualKeywords: []string{"人物连续"},
		Mandatory: []string{}, Prohibited: []string{"不得虚构剧情"},
		CreativeRoutes: []CreativeRouteSnapshot{{
			RouteID: ManualShortDramaPrerollV2RouteID, RouteType: PerformanceModeShortDramaPreroll,
			VideoPurpose: "performance", Channels: []string{"douyin"}, Reason: "用户选择短剧前贴 V2",
			TargetDurationSeconds: 6, AspectRatio: "9:16", Resolution: "720p",
			SourceAssetRefs: []contract.AssetVersionRef{source}, RequiresHumanConfirmation: true,
		}},
		ManualShortDramaPrerollV2: &ManualShortDramaPrerollV2Input{
			SourceVideo: source, SourceVideoRights: RightsConfirmed,
		},
	})
	if err != nil {
		t.Fatalf("create V2 intake: %v", err)
	}

	task, err := service.CreateVideoTask(context.Background(), rc.Actor, "project_1", intake.ID, CreateVideoTaskRequest{
		SelectedRouteID: ManualShortDramaPrerollV2RouteID, Channel: ChannelDouyin,
		Concept: "短剧视频理解驱动的前贴", Prompt: "等待视频理解", SourceVideo: source,
		CallToAction: "点击观看正片", Mandatory: []string{}, Prohibited: []string{"不得虚构剧情"}, ConfirmRoute: true,
	})
	if err != nil {
		t.Fatalf("create V2 task: %v", err)
	}

	detail, err := service.GetTaskDetail(context.Background(), rc.Actor, "project_1", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	workspace := detail.VideoDraft.ShortDramaPrerollV2
	if workspace == nil {
		t.Fatal("short drama V2 workspace is nil")
	}
	if workspace.ContractVersion != ShortDramaPrerollV3ContractVersion || workspace.SourceCanvas == nil || workspace.ModelCanvas == nil || workspace.OutputCanvas == nil ||
		workspace.ActiveStage != ShortDramaV2StageSourceReady ||
		workspace.SourceVideo.ProjectID != "project_1" ||
		workspace.SourceVideo.AssetVersion != source ||
		workspace.Analysis.Status != ShortDramaV2ResourceIdle {
		t.Fatalf("unexpected initial V2 workspace: %#v", workspace)
	}
	if detail.VideoDraft.ShortDramaPreroll != nil {
		t.Fatal("V2 task must not create a legacy short drama candidate draft")
	}
	if err := detail.VideoDraft.Validate(); err != nil {
		t.Fatalf("persisted V2 draft is invalid: %v", err)
	}
}

func TestCreateShortDramaPrerollV2WorkspaceDoesNotRewriteLegacyV3ManualRoute(t *testing.T) {
	t.Parallel()

	service := testService()
	service.ViralRemakes = service.Repository.(*memoryRepository)
	source := contract.AssetVersionRef{AssetID: "asset_wuzetian_v3", Version: 1}
	service.Assets = testAssetReader{snapshot: CreativeAssetSnapshot{
		Ref: source, Kind: contract.AssetVideo, MIMEType: "video/mp4", Ready: true,
		WidthPixels: 1920, HeightPixels: 818, DurationMS: 182417,
	}}
	rc := testRequestContext()

	intake, err := service.CreateIntake(context.Background(), rc, "project_1", "short-drama-v2-v3-intake", CreateIntakeRequest{
		ContractVersion: CreativeIntakeCreateV3ContractVersion,
		Source:          IntakeSourceManual, Format: FormatVideo, PerformanceMode: PerformanceModeShortDramaPreroll,
		Channel: ChannelDouyin, Objective: "用独立前贴吸引用户观看短剧正片", Audience: "竖屏短剧观众",
		CoreMessage: "基于上传短剧的真实剧情生成钩子", CallToAction: "点击观看正片",
		Concept: "短剧前贴 V2", Tone: []string{"紧凑"}, VisualKeywords: []string{"人物连续"},
		Mandatory: []string{}, Prohibited: []string{"不得虚构剧情"},
		CreativeRoutes: []CreativeRouteSnapshot{{
			RouteID: ManualShortDramaPrerollV2RouteID, RouteType: PerformanceModeShortDramaPreroll,
			VideoPurpose: "performance", Channels: []string{"douyin"}, Reason: "用户选择短剧前贴 V2",
			TargetDurationSeconds: 6, AspectRatio: "9:16", Resolution: "720p",
			SourceAssetRefs: []contract.AssetVersionRef{source}, RequiresHumanConfirmation: true,
		}},
		ManualShortDramaPrerollV2: &ManualShortDramaPrerollV2Input{
			SourceVideo: source, SourceVideoRights: RightsConfirmed,
		},
	})
	if err != nil {
		t.Fatalf("create V2 intake: %v", err)
	}
	if len(intake.Request.CreativeRoutes) != 1 || intake.Request.CreativeRoutes[0].RouteID != ManualShortDramaPrerollV2RouteID ||
		intake.Request.ManualShortDramaPrerollV2 == nil || intake.Request.Format != FormatVideo {
		t.Fatalf("V2 manual route was rewritten: %#v", intake.Request)
	}
}

func TestShortDramaPrerollV3BuildsThreeFirstFramesAndSingleReferenceVideoInput(t *testing.T) {
	t.Parallel()

	service, taskID, rc := createShortDramaV2TestWorkspace(t)
	prompts := advanceShortDramaV2ToPrompts(t, &service, taskID, rc)
	service.ShortDramaV2Images = &shortDramaV2ImageJobsStub{}

	generated, err := service.GenerateShortDramaV2FirstFrames(context.Background(), rc.Actor, "project_1", taskID, GenerateShortDramaV2FirstFramesRequest{ExpectedRevision: prompts.VideoDraft.Revision})
	if err != nil {
		t.Fatalf("generate first frames: %v", err)
	}
	batch := generated.VideoDraft.ShortDramaPrerollV2.FirstFrameBatch
	if batch == nil || len(batch.Candidates) != 3 || batch.Status != ShortDramaV2ResourceQueued || batch.Candidates[0].VariantKey == batch.Candidates[1].VariantKey {
		t.Fatalf("first frame batch = %#v", batch)
	}

	current := generated
	for index, candidate := range batch.Candidates {
		asset := contract.ProjectAssetRef{ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: contract.AssetID(fmt.Sprintf("generated_frame_%d", index+1)), Version: 1}}
		job := contract.ProviderJob{ID: candidate.ProviderJobID, ProjectID: "project_1", ProviderStatus: contract.ProviderJobSucceeded, ProjectAssetRefs: []contract.ProjectAssetRef{asset}}
		current, err = service.ReconcileShortDramaV2FirstFrame(context.Background(), rc.Actor, "project_1", taskID, ReconcileShortDramaV2FirstFrameRequest{ExpectedRevision: current.VideoDraft.Revision, CandidateID: candidate.ID, Job: job})
		if err != nil {
			t.Fatalf("reconcile first frame %d: %v", index, err)
		}
	}
	readyBatch := current.VideoDraft.ShortDramaPrerollV2.FirstFrameBatch
	if readyBatch.Status != ShortDramaV2ResourceReady {
		t.Fatalf("reconciled first frame batch = %#v", readyBatch)
	}
	legacyDraft := *current.VideoDraft
	legacyWorkspace := *legacyDraft.ShortDramaPrerollV2
	legacyBatch := *legacyWorkspace.FirstFrameBatch
	legacyBatch.Candidates = append([]ShortDramaV2FirstFrameCandidate(nil), legacyBatch.Candidates...)
	legacyBatch.Candidates[0].ModelCanvasAsset = nil
	legacyBatch.Candidates[0].OutputCanvasAsset = nil
	legacyWorkspace.FirstFrameBatch = &legacyBatch
	legacyDraft.Revision++
	legacyWorkspace.Revision = legacyDraft.Revision
	legacyDraft.ShortDramaPrerollV2 = &legacyWorkspace
	if _, err := service.ViralRemakes.ReviseVideoDraft(context.Background(), rc.Actor.OrganizationID, "project_1", taskID, current.VideoDraft.Revision, legacyDraft, TaskInProgress); err != nil {
		t.Fatalf("persist legacy first frame batch: %v", err)
	}
	current, err = service.Repository.GetTaskDetail(context.Background(), rc.Actor.OrganizationID, "project_1", taskID)
	if err != nil {
		t.Fatalf("restore legacy first frame batch: %v", err)
	}
	readyBatch = current.VideoDraft.ShortDramaPrerollV2.FirstFrameBatch

	selected, err := service.SelectShortDramaV2FirstFrame(context.Background(), rc.Actor, "project_1", taskID, SelectShortDramaV2FirstFrameRequest{ExpectedRevision: current.VideoDraft.Revision, BatchID: readyBatch.ID, CandidateID: readyBatch.Candidates[0].ID})
	if err != nil {
		t.Fatalf("select first frame: %v", err)
	}
	if selected.VideoDraft.ShortDramaPrerollV2.FirstFrameBatch.Candidates[0].ModelCanvasAsset == nil || selected.VideoDraft.ShortDramaPrerollV2.FirstFrameBatch.Candidates[0].OutputCanvasAsset == nil {
		t.Fatalf("legacy candidate canvas assets were not repaired: %#v", selected.VideoDraft.ShortDramaPrerollV2.FirstFrameBatch.Candidates[0])
	}
	prompt := selected.VideoDraft.ShortDramaPrerollV2.PromptDraft
	selected, err = service.UpdateShortDramaV2Prompts(context.Background(), rc.Actor, "project_1", taskID, UpdateShortDramaV2PromptsRequest{
		ExpectedRevision: selected.VideoDraft.Revision,
		ImagePrompt:      prompt.ImagePrompt,
		VideoDescription: prompt.VideoDescription + " refined",
		VideoPrompt:      prompt.VideoPrompt + " refined motion",
	})
	if err != nil {
		t.Fatalf("update video-only prompts: %v", err)
	}
	if selected.VideoDraft.ShortDramaPrerollV2.ActiveStage != ShortDramaV2StageFrameSelected ||
		selected.VideoDraft.ShortDramaPrerollV2.FirstFrameBatch == nil || selected.VideoDraft.ShortDramaPrerollV2.GenerationSpec == nil {
		t.Fatalf("video-only prompt edit discarded selected first frame: %#v", selected.VideoDraft.ShortDramaPrerollV2)
	}
	input, promptHash, specHash, err := service.ShortDramaV2ProviderInput(context.Background(), rc.Actor, "project_1", taskID)
	if err != nil {
		t.Fatalf("compile provider input: %v", err)
	}
	if input.InputMode != provider.VideoInputReferenceImage || input.DurationSeconds != 6 || input.AspectRatio != "16:9" ||
		len(input.ConditioningAssets) != 1 || input.ConditioningAssets[0].Role != provider.VideoConditioningReferenceImage ||
		input.ConditioningAssets[0].AuthorizedAsset != nil || promptHash == "" || specHash == "" ||
		selected.VideoDraft.ShortDramaPrerollV2.GenerationSpec == nil {
		t.Fatalf("provider input=%#v promptHash=%q specHash=%q", input, promptHash, specHash)
	}

	generating, err := service.RegisterShortDramaV2VideoJob(context.Background(), rc.Actor, "project_1", taskID, "video_job_1")
	if err != nil {
		t.Fatalf("register video job: %v", err)
	}
	failed, err := service.ReconcileShortDramaV2Video(context.Background(), rc.Actor, "project_1", taskID, ReconcileShortDramaV2VideoRequest{
		ExpectedRevision: generating.VideoDraft.Revision,
		Job: contract.ProviderJob{ID: "video_job_1", ProjectID: "project_1", ProviderStatus: contract.ProviderJobFailed,
			Error: &contract.JobError{Code: "INPUT_IMAGE_REJECTED", Message: "reference image rejected", Retryable: false}},
	})
	if err != nil {
		t.Fatalf("reconcile failed video: %v", err)
	}
	failedWorkspace := failed.VideoDraft.ShortDramaPrerollV2
	if failedWorkspace.ActiveStage != ShortDramaV2StageFrameSelected || failedWorkspace.VideoError == nil ||
		failedWorkspace.VideoError.Code != "INPUT_IMAGE_REJECTED" || failed.Task.Status != TaskInProgress {
		t.Fatalf("failed video workspace = %#v task status=%q", failedWorkspace, failed.Task.Status)
	}
	generating, err = service.RegisterShortDramaV2VideoJob(context.Background(), rc.Actor, "project_1", taskID, "video_job_2")
	if err != nil {
		t.Fatalf("register retry video job: %v", err)
	}
	if generating.VideoDraft.ShortDramaPrerollV2.VideoError != nil {
		t.Fatalf("video retry retained stale failure: %#v", generating.VideoDraft.ShortDramaPrerollV2.VideoError)
	}
	videoAsset := contract.ProjectAssetRef{ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: "generated_preroll", Version: 1}}
	completed, err := service.ReconcileShortDramaV2Video(context.Background(), rc.Actor, "project_1", taskID, ReconcileShortDramaV2VideoRequest{
		ExpectedRevision: generating.VideoDraft.Revision,
		Job:              contract.ProviderJob{ID: "video_job_2", ProjectID: "project_1", ProviderStatus: contract.ProviderJobSucceeded, ProjectAssetRefs: []contract.ProjectAssetRef{videoAsset}},
	})
	if err != nil {
		t.Fatalf("reconcile video: %v", err)
	}
	completedWorkspace := completed.VideoDraft.ShortDramaPrerollV2
	if completedWorkspace.ActiveStage != ShortDramaV2StageCompleted || completedWorkspace.RawOutputAsset == nil ||
		completedWorkspace.RawOutputAsset.AssetVersion.AssetID != "generated_preroll" || completedWorkspace.OutputAsset == nil ||
		completedWorkspace.OutputAsset.AssetVersion.AssetID != "generated_preroll_normalized" || completedWorkspace.SourceOpeningFrame != nil ||
		completed.Task.Status != TaskGenerated {
		t.Fatalf("completed workspace = %#v task status=%q", completed.VideoDraft.ShortDramaPrerollV2, completed.Task.Status)
	}
	regenerated, err := service.GenerateShortDramaV2FirstFrames(context.Background(), rc.Actor, "project_1", taskID, GenerateShortDramaV2FirstFramesRequest{ExpectedRevision: completed.VideoDraft.Revision})
	if err != nil {
		t.Fatalf("regenerate first frames after completed video: %v", err)
	}
	regeneratedWorkspace := regenerated.VideoDraft.ShortDramaPrerollV2
	if regeneratedWorkspace.FirstFrameBatch == nil || regeneratedWorkspace.FirstFrameBatch.ID == readyBatch.ID ||
		len(regeneratedWorkspace.FirstFrameBatch.Candidates) != 3 || regeneratedWorkspace.OutputAsset != nil ||
		regeneratedWorkspace.RawOutputAsset != nil || regeneratedWorkspace.ActiveStage != ShortDramaV2StageFramesGenerating {
		t.Fatalf("regenerated workspace = %#v", regeneratedWorkspace)
	}
}

func TestShortDramaPrerollV2AnalyzesVideoAndCompilesFourGroundedDirections(t *testing.T) {
	t.Parallel()

	service, taskID, rc := createShortDramaV2TestWorkspace(t)
	service.ShortDramaV2Analyzer = shortDramaV2AnalyzerStub{result: ShortDramaV2AnalysisResult{
		InputHash: "sha256:analysis-input", PromptVersion: "short-drama-analysis/v1",
		Content: ShortDramaV2AnalysisContent{
			Title: "武则天权力之路", Synopsis: "武则天从宫廷边缘逐步进入权力中心，并在多次政治冲突中改变自己的处境。",
			OpeningBeat: "宫廷中的人物正面临一次权力选择", CoreConflict: "身份限制与权力意志的冲突",
			UnresolvedHook: "她将如何扭转局势", Tone: "历史人物爽剧",
			Characters:     []ShortDramaV2Character{{Name: "武则天", Description: "处于权力变化中心的女性"}},
			VisualKeywords: []string{"宫廷", "红色官服", "权力对峙"},
			Evidence:       []ShortDramaV2Evidence{{ID: "evidence_1", TimestampMS: 0, Transcript: "宫廷局势发生变化"}},
		},
	}}
	service.ShortDramaV2Planner = shortDramaV2PlannerStub{
		directions: testShortDramaV2Directions(),
		prompt: ShortDramaV2PromptDraft{
			ImagePrompt:      "宫廷长廊中，一名女性回头直视镜头，红金色调，电影质感。",
			VideoDescription: "从压迫感宫廷画面进入人物掌握主动权的瞬间。",
			VideoPrompt:      "0-2秒宫廷压迫，2-5秒人物抬眼，5-6秒画面趋近输入视频开场。",
			CompilerVersion:  "short-drama-prompt/v1",
		},
	}

	initial, err := service.GetTaskDetail(context.Background(), rc.Actor, "project_1", taskID)
	if err != nil {
		t.Fatal(err)
	}
	analyzed, err := service.AnalyzeShortDramaV2Source(context.Background(), rc.Actor, "project_1", taskID, AnalyzeShortDramaV2SourceRequest{ExpectedRevision: initial.VideoDraft.Revision})
	if err != nil {
		t.Fatalf("analyze source: %v", err)
	}
	if analyzed.VideoDraft.ShortDramaPrerollV2.Analysis.Status != ShortDramaV2ResourceReady ||
		analyzed.VideoDraft.ShortDramaPrerollV2.ActiveStage != ShortDramaV2StageAnalysisReady {
		t.Fatalf("analysis was not persisted: %#v", analyzed.VideoDraft.ShortDramaPrerollV2)
	}

	planned, err := service.GenerateShortDramaV2Directions(context.Background(), rc.Actor, "project_1", taskID, GenerateShortDramaV2DirectionsRequest{ExpectedRevision: analyzed.VideoDraft.Revision})
	if err != nil {
		t.Fatalf("generate directions: %v", err)
	}
	batch := planned.VideoDraft.ShortDramaPrerollV2.DirectionBatch
	if batch == nil || len(batch.Items) != 4 || batch.Status != ShortDramaV2ResourceReady ||
		planned.VideoDraft.ShortDramaPrerollV2.ActiveStage != ShortDramaV2StageDirectionsReady {
		t.Fatalf("direction batch = %#v", batch)
	}

	selected, err := service.SelectShortDramaV2Direction(context.Background(), rc.Actor, "project_1", taskID, SelectShortDramaV2DirectionRequest{
		ExpectedRevision: planned.VideoDraft.Revision, DirectionBatchID: batch.ID,
		DirectionID: batch.Items[0].ID, DurationSeconds: 6,
	})
	if err != nil {
		t.Fatalf("select direction: %v", err)
	}
	prompt := selected.VideoDraft.ShortDramaPrerollV2.PromptDraft
	if prompt == nil || prompt.DurationSeconds != 6 || prompt.DirectionID != batch.Items[0].ID ||
		prompt.ContentHash == "" || selected.VideoDraft.ShortDramaPrerollV2.ActiveStage != ShortDramaV2StagePromptsReady {
		t.Fatalf("compiled prompt draft = %#v", prompt)
	}
}

func TestShortDramaPrerollV2RejectsDirectionBatchWithoutTwoByTwoCategories(t *testing.T) {
	t.Parallel()

	service, taskID, rc := createShortDramaV2TestWorkspace(t)
	service.ShortDramaV2Analyzer = shortDramaV2AnalyzerStub{result: testShortDramaV2AnalysisResult()}
	invalid := testShortDramaV2Directions()[:3]
	service.ShortDramaV2Planner = shortDramaV2PlannerStub{directions: invalid}
	initial, _ := service.GetTaskDetail(context.Background(), rc.Actor, "project_1", taskID)
	analyzed, err := service.AnalyzeShortDramaV2Source(context.Background(), rc.Actor, "project_1", taskID, AnalyzeShortDramaV2SourceRequest{ExpectedRevision: initial.VideoDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GenerateShortDramaV2Directions(context.Background(), rc.Actor, "project_1", taskID, GenerateShortDramaV2DirectionsRequest{ExpectedRevision: analyzed.VideoDraft.Revision}); err == nil {
		t.Fatal("expected invalid 2+2 direction batch to be rejected")
	}
}

func TestShortDramaPrerollV2EditingAnalysisInvalidatesDerivedWork(t *testing.T) {
	t.Parallel()

	service, taskID, rc := createShortDramaV2TestWorkspace(t)
	selected := advanceShortDramaV2ToPrompts(t, &service, taskID, rc)
	workspace := selected.VideoDraft.ShortDramaPrerollV2
	content := workspace.Analysis.Content
	content.Synopsis = "An edited synopsis grounded in the uploaded episode, long enough for validation."

	updated, err := service.UpdateShortDramaV2Analysis(context.Background(), rc.Actor, "project_1", taskID, UpdateShortDramaV2AnalysisRequest{
		ExpectedRevision: selected.VideoDraft.Revision,
		Content:          content,
	})
	if err != nil {
		t.Fatalf("update analysis: %v", err)
	}
	actual := updated.VideoDraft.ShortDramaPrerollV2
	if actual.Analysis.Revision != workspace.Analysis.Revision+1 || actual.ActiveStage != ShortDramaV2StageAnalysisReady ||
		actual.DirectionBatch != nil || actual.PromptDraft != nil || actual.FirstFrameBatch != nil || actual.GenerationSpec != nil {
		t.Fatalf("edited analysis did not invalidate downstream state: %#v", actual)
	}
}

func TestShortDramaPrerollV2EditingPromptsInvalidatesGeneratedMedia(t *testing.T) {
	t.Parallel()

	service, taskID, rc := createShortDramaV2TestWorkspace(t)
	selected := advanceShortDramaV2ToPrompts(t, &service, taskID, rc)
	prompt := selected.VideoDraft.ShortDramaPrerollV2.PromptDraft

	updated, err := service.UpdateShortDramaV2Prompts(context.Background(), rc.Actor, "project_1", taskID, UpdateShortDramaV2PromptsRequest{
		ExpectedRevision: selected.VideoDraft.Revision,
		ImagePrompt:      prompt.ImagePrompt + " alternate composition",
		VideoDescription: prompt.VideoDescription + " edited",
		VideoPrompt:      prompt.VideoPrompt + " edited motion",
	})
	if err != nil {
		t.Fatalf("update prompts: %v", err)
	}
	actual := updated.VideoDraft.ShortDramaPrerollV2
	if actual.PromptDraft == nil || actual.PromptDraft.Revision != prompt.Revision+1 || actual.PromptDraft.ContentHash == prompt.ContentHash ||
		actual.ActiveStage != ShortDramaV2StagePromptsReady || actual.FirstFrameBatch != nil || actual.GenerationSpec != nil || actual.OutputAsset != nil {
		t.Fatalf("edited prompts did not invalidate media state: %#v", actual)
	}
}

type shortDramaV2AnalyzerStub struct{ result ShortDramaV2AnalysisResult }

func (s shortDramaV2AnalyzerStub) Analyze(_ context.Context, _ contract.ActorContext, _ contract.ProjectContext, _ contract.ProjectAssetRef) (ShortDramaV2AnalysisResult, error) {
	return s.result, nil
}

type shortDramaV2PlannerStub struct {
	directions []ShortDramaV2HookDirection
	prompt     ShortDramaV2PromptDraft
}

type shortDramaV2ImageJobsStub struct{ calls int }

type shortDramaV2ImageReaderStub struct{}

func (shortDramaV2ImageReaderStub) OpenImage(context.Context, contract.ActorContext, contract.ProjectID, contract.AssetVersionRef) (io.ReadCloser, error) {
	canvas := image.NewRGBA(image.Rect(0, 0, 1536, 1024))
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1536; x++ {
			canvas.SetRGBA(x, y, color.RGBA{R: 90, G: uint8(x % 255), B: uint8(y % 255), A: 255})
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(output.Bytes())), nil
}

type shortDramaV2RenderedImageWriterStub struct{}

func (shortDramaV2RenderedImageWriterStub) IngestRenderedImage(_ context.Context, _ contract.RequestContext, projectID contract.ProjectID, renderJobID string, _ io.Reader, _ int64, _, _ int, _ []contract.AssetVersionRef, _ []contract.ResourceRef) (contract.ProjectAssetRef, error) {
	return contract.ProjectAssetRef{ProjectID: projectID, AssetVersion: contract.AssetVersionRef{AssetID: contract.AssetID("asset_" + renderJobID), Version: 1}}, nil
}

type shortDramaV2VideoNormalizerStub struct{}

func (shortDramaV2VideoNormalizerStub) NormalizeVideo(_ context.Context, request media.VideoNormalizationRequest) (media.CompositionOutput, error) {
	content := "normalized-video"
	return media.CompositionOutput{
		Content: io.NopCloser(strings.NewReader(content)), SizeBytes: int64(len(content)),
		Metadata: assets.VideoMetadata{DurationMS: 6000, WidthPixels: request.Width, HeightPixels: request.Height, FrameRate: "25/1", VideoCodec: "h264", AudioCodec: "aac"},
	}, nil
}

func (s *shortDramaV2ImageJobsStub) CreateFirstFrameJob(_ context.Context, _ contract.ActorContext, project contract.ProjectContext, request ShortDramaV2FirstFrameJobRequest) (contract.ProviderJob, error) {
	s.calls++
	return contract.ProviderJob{ID: fmt.Sprintf("image_job_%d", s.calls), ProjectID: project.ProjectID, ProviderStatus: contract.ProviderJobSubmitted}, nil
}

func (s shortDramaV2PlannerStub) PlanDirections(_ context.Context, _ contract.ActorContext, _ contract.ProjectContext, _ ShortDramaV2Analysis) ([]ShortDramaV2HookDirection, string, error) {
	return s.directions, "short-drama-directions/test", nil
}

func (s shortDramaV2PlannerStub) CompilePrompts(_ context.Context, _ contract.ActorContext, _ contract.ProjectContext, _ ShortDramaV2Analysis, _ ShortDramaV2HookDirection, _ int) (ShortDramaV2PromptDraft, error) {
	return s.prompt, nil
}

func testShortDramaV2AnalysisResult() ShortDramaV2AnalysisResult {
	return ShortDramaV2AnalysisResult{
		InputHash: "sha256:analysis-input", PromptVersion: "short-drama-analysis/v1",
		Content: ShortDramaV2AnalysisContent{
			Title: "武则天权力之路", Synopsis: "武则天从宫廷边缘逐步进入权力中心，并在政治冲突中改变自己的处境。",
			OpeningBeat: "宫廷权力发生变化", CoreConflict: "身份限制与权力意志的冲突",
			UnresolvedHook: "她将如何扭转局势", Tone: "历史人物爽剧",
			Characters:     []ShortDramaV2Character{{Name: "武则天", Description: "权力变化中心人物"}},
			VisualKeywords: []string{"宫廷", "权力对峙"},
			Evidence:       []ShortDramaV2Evidence{{ID: "evidence_1", TimestampMS: 0, Transcript: "宫廷局势发生变化"}},
		},
	}
}

func testShortDramaV2Directions() []ShortDramaV2HookDirection {
	return []ShortDramaV2HookDirection{
		{ID: "direction_1", Category: "curiosity", Title: "她为何突然回头", HookCopy: "所有人都低估了她接下来的选择。", Description: "用动作信息缺口吸睛", Rationale: "对应权力选择", VisualIntent: "宫廷回头", GroundingEvidenceIDs: []string{"evidence_1"}},
		{ID: "direction_2", Category: "curiosity", Title: "这一句话改变局势", HookCopy: "一句话之后，宫廷里的态度全变了。", Description: "用结果未知制造悬念", Rationale: "对应局势变化", VisualIntent: "群臣反应", GroundingEvidenceIDs: []string{"evidence_1"}},
		{ID: "direction_3", Category: "summary", Title: "从边缘到权力中心", HookCopy: "她从被动承受到主动改变局势。", Description: "快速总结人物权力变化", Rationale: "对应核心冲突", VisualIntent: "身份递进", GroundingEvidenceIDs: []string{"evidence_1"}},
		{ID: "direction_4", Category: "summary", Title: "她的权力之路", HookCopy: "每次选择，都让她离权力中心更近一步。", Description: "用人物成长概括剧情", Rationale: "对应人物主线", VisualIntent: "宫廷纵深", GroundingEvidenceIDs: []string{"evidence_1"}},
	}
}

func createShortDramaV2TestWorkspace(t *testing.T) (Service, string, contract.RequestContext) {
	t.Helper()
	service := testService()
	service.ViralRemakes = service.Repository.(*memoryRepository)
	service.Now = func() time.Time { return time.Date(2026, time.August, 6, 8, 0, 0, 0, time.UTC) }
	source := contract.AssetVersionRef{AssetID: "asset_wuzetian", Version: 1}
	service.Assets = testAssetReader{snapshot: CreativeAssetSnapshot{Ref: source, Kind: contract.AssetVideo, MIMEType: "video/mp4", Ready: true, WidthPixels: 1920, HeightPixels: 818, DurationMS: 182417, FrameRate: "30/1", VideoCodec: "h264", AudioCodec: "aac"}}
	service.ImageBaseAssets = shortDramaV2ImageReaderStub{}
	service.RenderedImages = shortDramaV2RenderedImageWriterStub{}
	service.ShortDramaV2OutputNormalizer = shortDramaV2VideoNormalizerStub{}
	service.RenderedAssets = &testRenderedAssetWriter{ref: contract.ProjectAssetRef{ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: "generated_preroll_normalized", Version: 1}}}
	rc := testRequestContext()
	intake, err := service.CreateIntake(context.Background(), rc, "project_1", "short-drama-v2-intake-helper", CreateIntakeRequest{
		Source: IntakeSourceManual, Format: FormatVideo, PerformanceMode: PerformanceModeShortDramaPreroll,
		Channel: ChannelDouyin, Objective: "用独立前贴吸引用户观看短剧", Audience: "竖屏短剧观众", CoreMessage: "基于真实剧情生成钩子", CallToAction: "点击观看正片", Concept: "短剧前贴 V2",
		Tone: []string{"紧凑"}, VisualKeywords: []string{"人物连续"}, Mandatory: []string{}, Prohibited: []string{"不得虚构剧情"},
		CreativeRoutes:            []CreativeRouteSnapshot{{RouteID: ManualShortDramaPrerollV2RouteID, RouteType: PerformanceModeShortDramaPreroll, VideoPurpose: "performance", Channels: []string{"douyin"}, Reason: "V2", TargetDurationSeconds: 6, AspectRatio: "9:16", Resolution: "720p", SourceAssetRefs: []contract.AssetVersionRef{source}, RequiresHumanConfirmation: true}},
		ManualShortDramaPrerollV2: &ManualShortDramaPrerollV2Input{SourceVideo: source, SourceVideoRights: RightsConfirmed},
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.CreateVideoTask(context.Background(), rc.Actor, "project_1", intake.ID, CreateVideoTaskRequest{SelectedRouteID: ManualShortDramaPrerollV2RouteID, Channel: ChannelDouyin, Concept: "短剧前贴 V2", Prompt: "等待视频理解", SourceVideo: source, CallToAction: "点击观看正片", Mandatory: []string{}, Prohibited: []string{"不得虚构剧情"}, ConfirmRoute: true})
	if err != nil {
		t.Fatal(err)
	}
	return service, task.ID, rc
}

func advanceShortDramaV2ToPrompts(t *testing.T, service *Service, taskID string, rc contract.RequestContext) TaskDetail {
	t.Helper()
	service.ShortDramaV2Analyzer = shortDramaV2AnalyzerStub{result: testShortDramaV2AnalysisResult()}
	service.ShortDramaV2Planner = shortDramaV2PlannerStub{directions: testShortDramaV2Directions(), prompt: ShortDramaV2PromptDraft{
		ImagePrompt: "宫廷长廊中的女性回头，红金色调，电影质感。", VideoDescription: "人物从受压迫转为主动。",
		VideoPrompt: "0-2秒宫廷压迫，2-5秒人物抬眼，5-6秒衔接开场。", CompilerVersion: "short-drama-prompt/v1",
	}}
	initial, err := service.GetTaskDetail(context.Background(), rc.Actor, "project_1", taskID)
	if err != nil {
		t.Fatal(err)
	}
	analyzed, err := service.AnalyzeShortDramaV2Source(context.Background(), rc.Actor, "project_1", taskID, AnalyzeShortDramaV2SourceRequest{ExpectedRevision: initial.VideoDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	planned, err := service.GenerateShortDramaV2Directions(context.Background(), rc.Actor, "project_1", taskID, GenerateShortDramaV2DirectionsRequest{ExpectedRevision: analyzed.VideoDraft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	batch := planned.VideoDraft.ShortDramaPrerollV2.DirectionBatch
	selected, err := service.SelectShortDramaV2Direction(context.Background(), rc.Actor, "project_1", taskID, SelectShortDramaV2DirectionRequest{ExpectedRevision: planned.VideoDraft.Revision, DirectionBatchID: batch.ID, DirectionID: batch.Items[0].ID, DurationSeconds: 6})
	if err != nil {
		t.Fatal(err)
	}
	return selected
}
