package creative

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	platformassets "github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/jobruntime"
)

type revocableEditingAssetUse struct {
	revoked bool
	calls   []platformassets.AssetUsePurpose
}

func (a *revocableEditingAssetUse) Authorize(_ context.Context, request platformassets.AssetUseRequest) (platformassets.AssetUseDecision, error) {
	a.calls = append(a.calls, request.Purpose)
	if a.revoked {
		return platformassets.AssetUseDecision{Allowed: false, RightsStatus: platformassets.AssetRightsRevoked, Code: platformassets.AssetRightsRevokedCode}, platformassets.AssetUseDeniedError{Code: platformassets.AssetRightsRevokedCode}
	}
	return platformassets.AssetUseDecision{Allowed: true, RightsStatus: platformassets.AssetRightsActive}, nil
}

type editingLineageWriterStub struct {
	productionRenderedWriterStub
	sources []contract.ResourceRef
}

func (w *editingLineageWriterStub) IngestRenderedVideoWithSources(_ context.Context, _ contract.RequestContext, projectID contract.ProjectID, _ string, sources []contract.ResourceRef, content io.Reader, _ int64) (contract.ProjectAssetRef, error) {
	w.calls++
	w.sources = append([]contract.ResourceRef(nil), sources...)
	_, _ = io.Copy(io.Discard, content)
	return contract.ProjectAssetRef{ProjectID: projectID, AssetVersion: contract.AssetVersionRef{AssetID: "final-video", Version: 1}}, nil
}

type editingRenderMemoryRepository struct {
	job      EditingRenderJob
	progress []int
}

func (r *editingRenderMemoryRepository) CreateEditingRender(_ context.Context, job EditingRenderJob) (EditingRenderJob, error) {
	r.job = job
	return job, nil
}
func (r *editingRenderMemoryRepository) GetEditingRender(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, id string) (EditingRenderJob, error) {
	if r.job.ID != id {
		return EditingRenderJob{}, ErrNotFound
	}
	return r.job, nil
}
func (r *editingRenderMemoryRepository) FindReusableEditingRender(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, fingerprint string, kind EditingRenderKind) (EditingRenderJob, error) {
	if r.job.Status == EditingRenderSucceeded && r.job.RendererFingerprint == fingerprint && r.job.Kind == kind && r.job.OutputAsset != nil {
		return r.job, nil
	}
	return EditingRenderJob{}, ErrNotFound
}
func (r *editingRenderMemoryRepository) MarkEditingRenderRunning(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, id string, now time.Time) (EditingRenderJob, error) {
	if r.job.ID != id || r.job.Status != EditingRenderQueued {
		return EditingRenderJob{}, ErrInvalidState
	}
	r.job.Status = EditingRenderRunning
	r.job.UpdatedAt = now
	return r.job, nil
}
func (r *editingRenderMemoryRepository) UpdateEditingRenderProgress(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, _ string, progress int, _ time.Time) error {
	r.progress = append(r.progress, progress)
	r.job.ProgressPercent = progress
	return nil
}
func (r *editingRenderMemoryRepository) CompleteEditingRender(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, _ string, ref contract.ProjectAssetRef, now time.Time) error {
	r.job.Status = EditingRenderSucceeded
	r.job.ProgressPercent = 100
	r.job.OutputAsset = &ref
	r.job.UpdatedAt = now
	return nil
}
func (r *editingRenderMemoryRepository) FailEditingRender(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, _, code, message string, now time.Time) error {
	r.job.Status = EditingRenderFailed
	r.job.ErrorCode = code
	r.job.ErrorMessage = message
	r.job.UpdatedAt = now
	return nil
}
func (r *editingRenderMemoryRepository) CancelEditingRender(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, _ string, now time.Time) (EditingRenderJob, error) {
	r.job.Status = EditingRenderCancelled
	r.job.UpdatedAt = now
	return r.job, nil
}

type editingRenderSchedulerStub struct {
	scheduled EditingRenderJob
	err       error
}

func (s *editingRenderSchedulerStub) ScheduleEditingRender(_ context.Context, job EditingRenderJob) error {
	s.scheduled = job
	return s.err
}

func TestEditingRenderSchedulerFailureMakesCreatedJobTerminal(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	asset := contract.AssetVersionRef{AssetID: "source", Version: 1}
	timeline := EditingTimelineV1{SchemaVersion: EditingTimelineSchemaV1, OutputProfile: EditingMVPVerticalOutputProfile, DurationMS: 6000, Tracks: []EditingTimelineTrack{{ID: "video", Role: EditingTrackPrimaryVideo, Clips: []EditingTimelineClip{{ID: "clip", AssetRef: &asset, TimelineEndMS: 6000, SourceOutMS: 6000}}}}}
	edits, renders := &memoryEditTaskRepository{}, &editingRenderMemoryRepository{}
	scheduler := &editingRenderSchedulerStub{err: errors.New("queue unavailable")}
	service := Service{Projects: testProjects{}, EditTasks: edits, EditingRenders: renders, EditingRenderScheduler: scheduler, Assets: testAssetReader{snapshots: map[contract.AssetID]CreativeAssetSnapshot{asset.AssetID: {Ref: asset, Kind: contract.AssetVideo, Ready: true, DurationMS: 6000}}}, AINativeTimelineRenderer: &productionTimelineRendererStub{}, RenderedAssets: &productionRenderedWriterStub{}, NewID: func(prefix string) (string, error) { return prefix + "_1", nil }, Now: func() time.Time { return now }}
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, Scopes: []contract.Scope{ScopeRead, ScopeWrite}}
	task, err := service.CreateEditTask(context.Background(), actor, "project_1", CreateEditTaskRequest{DisplayName: "剪辑", Timeline: &timeline})
	if err != nil {
		t.Fatal(err)
	}
	rc := contract.RequestContext{RequestID: "request_1", TraceID: "trace_1", Actor: actor}
	_, err = service.CreateEditingRender(context.Background(), rc, "project_1", task.ID, CreateEditingRenderRequest{Kind: EditingRenderPreview})
	if err == nil {
		t.Fatal("scheduler failure must be returned")
	}
	if renders.job.Status != EditingRenderFailed || renders.job.ErrorCode != "SCHEDULER_ENQUEUE_FAILED" {
		t.Fatalf("render job remained non-terminal: %#v", renders.job)
	}
}

func TestEditingRenderFreezesTimelineAndReturnsOutputAsset(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	asset := contract.AssetVersionRef{AssetID: "source", Version: 1}
	timeline := EditingTimelineV1{SchemaVersion: EditingTimelineSchemaV1, OutputProfile: EditingMVPVerticalOutputProfile, DurationMS: 6000, Tracks: []EditingTimelineTrack{{ID: "video", Role: EditingTrackPrimaryVideo, Clips: []EditingTimelineClip{{ID: "clip", AssetRef: &asset, TimelineEndMS: 6000, SourceOutMS: 6000}}}}}
	edits := &memoryEditTaskRepository{}
	renders := &editingRenderMemoryRepository{}
	scheduler := &editingRenderSchedulerStub{}
	renderer := &productionTimelineRendererStub{}
	writer := &editingLineageWriterStub{}
	sequence := 0
	service := Service{Projects: testProjects{}, EditTasks: edits, EditingRenders: renders, EditingRenderScheduler: scheduler, Assets: testAssetReader{snapshots: map[contract.AssetID]CreativeAssetSnapshot{asset.AssetID: {Ref: asset, Kind: contract.AssetVideo, Ready: true, DurationMS: 6000}}}, AINativeTimelineRenderer: renderer, RenderedAssets: writer, NewID: func(prefix string) (string, error) { sequence++; return prefix + "_" + string(rune('0'+sequence)), nil }, Now: func() time.Time { return now }}
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, Scopes: []contract.Scope{ScopeRead, ScopeWrite}}
	task, err := service.CreateEditTask(context.Background(), actor, "project_1", CreateEditTaskRequest{DisplayName: "剪辑", Timeline: &timeline})
	if err != nil {
		t.Fatal(err)
	}
	rc := contract.RequestContext{RequestID: "request_1", TraceID: "trace_1", Actor: actor}
	job, err := service.CreateEditingRender(context.Background(), rc, "project_1", task.ID, CreateEditingRenderRequest{Kind: EditingRenderPreview})
	if err != nil {
		t.Fatal(err)
	}
	if job.Timeline.Version != 1 || scheduler.scheduled.ID != job.ID {
		t.Fatalf("job was not frozen and queued: %#v %#v", job, scheduler.scheduled)
	}
	if job.RendererFingerprint == "" || scheduler.scheduled.RendererFingerprint != job.RendererFingerprint {
		t.Fatalf("render identity was not frozen: %#v", job)
	}
	if err := service.ExecuteEditingRender(context.Background(), "org_1", "project_1", job.ID); err != nil {
		t.Fatal(err)
	}
	if renders.job.Status != EditingRenderSucceeded || renders.job.ProgressPercent != 100 || renders.job.OutputAsset == nil || renderer.calls != 1 || writer.calls != 1 {
		t.Fatalf("render result=%#v renderer=%d writer=%d", renders.job, renderer.calls, writer.calls)
	}
	if len(writer.sources) != 2 || writer.sources[0].Type != "asset_version" || writer.sources[0].ID != "source" || writer.sources[1].Type != "edit_timeline_version" || writer.sources[1].ID != task.ID {
		t.Fatalf("rendered asset lineage=%#v", writer.sources)
	}
	reused, err := service.CreateEditingRender(context.Background(), rc, "project_1", task.ID, CreateEditingRenderRequest{Kind: EditingRenderPreview})
	if err != nil {
		t.Fatal(err)
	}
	if reused.ID != job.ID || reused.OutputAsset == nil || renderer.calls != 1 || writer.calls != 1 {
		t.Fatalf("proxy render was not reused: %#v", reused)
	}
}

func TestC6PreviewAndExportHaveDistinctAuthoritativeFingerprints(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	asset := contract.AssetVersionRef{AssetID: "source", Version: 1}
	timeline := EditingTimelineV1{SchemaVersion: EditingTimelineSchemaV1, OutputProfile: EditingMVPVerticalOutputProfile, DurationMS: 6000, Tracks: []EditingTimelineTrack{{ID: "video", Role: EditingTrackPrimaryVideo, Clips: []EditingTimelineClip{{ID: "clip", AssetRef: &asset, TimelineEndMS: 6000, SourceOutMS: 6000}}}}}
	edits, renders, scheduler := &memoryEditTaskRepository{}, &editingRenderMemoryRepository{}, &editingRenderSchedulerStub{}
	sequence := 0
	service := Service{Projects: testProjects{}, EditTasks: edits, EditingRenders: renders, EditingRenderScheduler: scheduler, Assets: testAssetReader{snapshots: map[contract.AssetID]CreativeAssetSnapshot{asset.AssetID: {Ref: asset, Kind: contract.AssetVideo, Ready: true, DurationMS: 6000}}}, AINativeTimelineRenderer: &productionTimelineRendererStub{}, RenderedAssets: &productionRenderedWriterStub{}, NewID: func(prefix string) (string, error) { sequence++; return fmt.Sprintf("%s_%d", prefix, sequence), nil }, Now: func() time.Time { return now }}
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, Scopes: []contract.Scope{ScopeRead, ScopeWrite}}
	task, err := service.CreateEditTask(context.Background(), actor, "project_1", CreateEditTaskRequest{DisplayName: "剪辑", Timeline: &timeline})
	if err != nil {
		t.Fatal(err)
	}
	rc := contract.RequestContext{RequestID: "request_1", TraceID: "trace_1", Actor: actor}
	preview, err := service.CreateEditingRender(context.Background(), rc, "project_1", task.ID, CreateEditingRenderRequest{Kind: EditingRenderPreview})
	if err != nil {
		t.Fatal(err)
	}
	renders.job = EditingRenderJob{}
	exported, err := service.CreateEditingRender(context.Background(), rc, "project_1", task.ID, CreateEditingRenderRequest{Kind: EditingRenderExport})
	if err != nil {
		t.Fatal(err)
	}
	if preview.RendererFingerprint == "" || exported.RendererFingerprint == "" || preview.RendererFingerprint == exported.RendererFingerprint {
		t.Fatalf("preview=%q export=%q", preview.RendererFingerprint, exported.RendererFingerprint)
	}
}

func TestC6V2RenderedAssetProvenanceIncludesTimelineVersion(t *testing.T) {
	asset := contract.AssetVersionRef{AssetID: "video", Version: 2}
	timeline := TimelineVersion{Version: 4, SchemaVersion: EditingTimelineSchemaV2, TimelineV2: &EditingTimelineV2{SchemaVersion: EditingTimelineSchemaV2, Timebase: EditingTimebaseV2{FrameRateNum: 30, FrameRateDen: 1}, Canvas: EditingCanvasV2{ProfileID: "vertical-720p-v1", Width: 720, Height: 1280, SampleRate: 48000, Background: EditingBackgroundV2{Type: "color", Value: "#000000"}}, DurationFrames: 30, Tracks: []EditingTrackV2{{ID: "visual-primary", Kind: "visual", Role: "primary", Clips: []EditingClipV2{{ID: "clip", Kind: "video", AssetRef: &asset, Timeline: EditingTimelineRangeV2{DurationFrames: 30}, Source: &EditingSourceRangeV2{OutUS: 1000000}, Transform: ptrEditingTransform(defaultTransformV2())}}}}}, CompilerCompatibility: "editing-v2-audio-c5", ContentHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatedBy: "user_1", CreatedAt: time.Now()}
	sources := editingRenderSources(EditingRenderJob{EditTaskID: "edit_1", Timeline: timeline})
	if len(sources) != 2 || sources[1].Type != "edit_timeline_version" || sources[1].ID != "edit_1" || sources[1].Version == nil || *sources[1].Version != 4 {
		t.Fatalf("provenance=%#v", sources)
	}
}

func ptrEditingTransform(value EditingVisualTransformV2) *EditingVisualTransformV2 { return &value }

func TestEditingRenderCancelsQueuedWorkAndRetriesFailureFromFrozenTimeline(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	asset := contract.AssetVersionRef{AssetID: "source", Version: 1}
	timeline := EditingTimelineV1{SchemaVersion: EditingTimelineSchemaV1, OutputProfile: EditingMVPVerticalOutputProfile, DurationMS: 6000, Tracks: []EditingTimelineTrack{{ID: "video", Role: EditingTrackPrimaryVideo, Clips: []EditingTimelineClip{{ID: "clip", AssetRef: &asset, TimelineEndMS: 6000, SourceOutMS: 6000}}}}}
	edits, renders, scheduler := &memoryEditTaskRepository{}, &editingRenderMemoryRepository{}, &editingRenderSchedulerStub{}
	sequence := 0
	service := Service{Projects: testProjects{}, EditTasks: edits, EditingRenders: renders, EditingRenderScheduler: scheduler, Assets: testAssetReader{snapshots: map[contract.AssetID]CreativeAssetSnapshot{asset.AssetID: {Ref: asset, Kind: contract.AssetVideo, Ready: true, DurationMS: 6000}}}, AINativeTimelineRenderer: &productionTimelineRendererStub{}, RenderedAssets: &productionRenderedWriterStub{}, NewID: func(prefix string) (string, error) { sequence++; return prefix + string(rune('0'+sequence)), nil }, Now: func() time.Time { return now }}
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, Scopes: []contract.Scope{ScopeRead, ScopeWrite}}
	task, err := service.CreateEditTask(context.Background(), actor, "project_1", CreateEditTaskRequest{DisplayName: "剪辑", Timeline: &timeline})
	if err != nil {
		t.Fatal(err)
	}
	rc := contract.RequestContext{RequestID: "request_1", TraceID: "trace_1", Actor: actor}
	job, err := service.CreateEditingRender(context.Background(), rc, "project_1", task.ID, CreateEditingRenderRequest{Kind: EditingRenderExport})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := service.CancelEditingRender(context.Background(), actor, "project_1", job.ID)
	if err != nil || cancelled.Status != EditingRenderCancelled {
		t.Fatalf("cancelled=%#v err=%v", cancelled, err)
	}
	renders.job.Status, renders.job.ErrorCode = EditingRenderFailed, "TIMELINE_RENDER_FAILED"
	retry, err := service.RetryEditingRender(context.Background(), rc, "project_1", job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retry.RetryOf != job.ID || retry.Timeline.ContentHash != job.Timeline.ContentHash || retry.Status != EditingRenderQueued || scheduler.scheduled.ID != retry.ID {
		t.Fatalf("retry=%#v scheduled=%#v", retry, scheduler.scheduled)
	}
}

func TestEditingRenderRuntimeFailureMakesPublicJobTerminal(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	renders := &editingRenderMemoryRepository{job: EditingRenderJob{
		ID: "render_1", OrganizationID: "org_1", ProjectID: "project_1",
		Status: EditingRenderQueued, CreatedAt: now, UpdatedAt: now,
	}}
	handler := EditingRenderRuntimeHandler(Service{EditingRenders: renders, Now: func() time.Time { return now }})
	_, err := handler(context.Background(), jobruntime.Claim{
		Job:     contract.Job{Kind: editingRenderExecutionKind, OrganizationID: "org_1", ProjectID: "project_1"},
		Payload: []byte(`{"render_job_id":"render_1"}`),
	})
	if err == nil {
		t.Fatal("expected runtime failure")
	}
	if renders.job.Status != EditingRenderFailed || renders.job.ErrorCode != "EDITING_RENDER_FAILED" {
		t.Fatalf("public render job remained non-terminal: %#v", renders.job)
	}
}

func TestC7WorkerReauthorizesFrozenAssetsAfterRightsRevocation(t *testing.T) {
	now := time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC)
	asset := contract.AssetVersionRef{AssetID: "source", Version: 1}
	timeline := EditingTimelineV1{SchemaVersion: EditingTimelineSchemaV1, OutputProfile: EditingMVPVerticalOutputProfile, DurationMS: 6000, Tracks: []EditingTimelineTrack{{ID: "video", Role: EditingTrackPrimaryVideo, Clips: []EditingTimelineClip{{ID: "clip", AssetRef: &asset, TimelineEndMS: 6000, SourceOutMS: 6000}}}}}
	rights := &revocableEditingAssetUse{}
	renderer := &productionTimelineRendererStub{}
	edits, renders, scheduler := &memoryEditTaskRepository{}, &editingRenderMemoryRepository{}, &editingRenderSchedulerStub{}
	service := Service{Projects: testProjects{}, EditTasks: edits, EditingRenders: renders, EditingRenderScheduler: scheduler, AssetUses: rights, Assets: testAssetReader{snapshots: map[contract.AssetID]CreativeAssetSnapshot{asset.AssetID: {Ref: asset, Kind: contract.AssetVideo, Ready: true, DurationMS: 6000}}}, AINativeTimelineRenderer: renderer, RenderedAssets: &productionRenderedWriterStub{}, NewID: func(prefix string) (string, error) { return prefix + "_c7", nil }, Now: func() time.Time { return now }}
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, Scopes: []contract.Scope{ScopeRead, ScopeWrite}}
	task, err := service.CreateEditTask(context.Background(), actor, "project_1", CreateEditTaskRequest{DisplayName: "C7 TOCTOU", Timeline: &timeline})
	if err != nil {
		t.Fatal(err)
	}
	rc := contract.RequestContext{RequestID: "request_c7", TraceID: "trace_c7", Actor: actor}
	job, err := service.CreateEditingRender(context.Background(), rc, "project_1", task.ID, CreateEditingRenderRequest{Kind: EditingRenderExport})
	if err != nil {
		t.Fatal(err)
	}
	rights.revoked = true
	err = service.ExecuteEditingRender(context.Background(), "org_1", "project_1", job.ID)
	if !errors.Is(err, platformassets.ErrAssetUseDenied) {
		t.Fatalf("worker error=%v, want asset use denied", err)
	}
	if renders.job.Status != EditingRenderFailed || renders.job.ErrorCode != "ASSET_USE_REVOKED" || renderer.calls != 0 {
		t.Fatalf("render=%#v renderer_calls=%d", renders.job, renderer.calls)
	}
	if len(rights.calls) < 3 || rights.calls[len(rights.calls)-1] != platformassets.AssetUseRenderExport {
		t.Fatalf("authorization purposes=%#v", rights.calls)
	}
}
