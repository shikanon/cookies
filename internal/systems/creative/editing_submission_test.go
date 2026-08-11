package creative

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

func TestC6SuccessfulExportBecomesCheckableApprovableCreativeVersion(t *testing.T) {
	now := time.Date(2026, 8, 11, 13, 0, 0, 0, time.UTC)
	asset := contract.AssetVersionRef{AssetID: "source-video", Version: 3}
	timeline := EditingTimelineV1{SchemaVersion: EditingTimelineSchemaV1, OutputProfile: EditingMVPVerticalOutputProfile, DurationMS: 6000, Tracks: []EditingTimelineTrack{{ID: "video", Role: EditingTrackPrimaryVideo, Clips: []EditingTimelineClip{{ID: "clip", AssetRef: &asset, TimelineEndMS: 6000, SourceOutMS: 6000}}}}}
	edits := &memoryEditTaskRepository{}
	versions := &memoryRepository{intakes: map[string]CreativeIntake{}, tasks: map[string]TaskDetail{}, renders: map[string]RenderJob{}, versions: map[string]CreativeVersion{}, packages: map[string]CreativePackage{}}
	renders := &editingRenderMemoryRepository{}
	sequence := 0
	service := Service{Projects: testProjects{}, EditTasks: edits, EditingRenders: renders, Repository: versions, Assets: testAssetReader{snapshots: map[contract.AssetID]CreativeAssetSnapshot{
		asset.AssetID:   {Ref: asset, Kind: contract.AssetVideo, Ready: true, DurationMS: 6000},
		"edited-output": {Ref: contract.AssetVersionRef{AssetID: "edited-output", Version: 1}, Kind: contract.AssetVideo, MIMEType: "video/mp4", Ready: true, WidthPixels: 720, HeightPixels: 1280, DurationMS: 6000, FrameRate: "30/1", VideoCodec: "h264", AudioCodec: "aac"},
	}}, NewID: func(prefix string) (string, error) { sequence++; return fmt.Sprintf("%s_%d", prefix, sequence), nil }, Now: func() time.Time { return now }}
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "reviewer_1"}, Scopes: []contract.Scope{ScopeRead, ScopeWrite}}
	task, err := service.CreateEditTask(context.Background(), actor, "project_1", CreateEditTaskRequest{DisplayName: "C6 成片", Timeline: &timeline})
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := editingRenderFingerprint(*task.CurrentTimeline, EditingRenderExport)
	if err != nil {
		t.Fatal(err)
	}
	output := contract.ProjectAssetRef{ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: "edited-output", Version: 1}}
	renders.job = EditingRenderJob{ID: "export_1", OrganizationID: "org_1", ProjectID: "project_1", EditTaskID: task.ID, Timeline: *task.CurrentTimeline, Kind: EditingRenderExport, RendererFingerprint: fingerprint, Status: EditingRenderSucceeded, ProgressPercent: 100, OutputAsset: &output, CreatedBy: actor.Principal, CreatedAt: now, UpdatedAt: now}
	rc := contract.RequestContext{RequestID: "request_1", TraceID: "trace_1", Actor: actor}
	version, duplicate, err := service.SubmitEditingVersion(context.Background(), rc, "project_1", task.ID, SubmitEditingVersionRequest{RenderJobID: renders.job.ID, ExpectedTimelineVersion: 1}, "submit-edit-1")
	if err != nil || duplicate {
		t.Fatalf("submit version=%#v duplicate=%v err=%v", version, duplicate, err)
	}
	if version.EditTaskID != task.ID || version.VideoSnapshot == nil || version.VideoSnapshot.Editing == nil || version.VideoSnapshot.Editing.RendererFingerprint != fingerprint || len(version.VideoSnapshot.Editing.InputAssets) != 1 {
		t.Fatalf("editing lineage was not frozen: %#v", version)
	}
	checked, err := service.CheckVersion(context.Background(), actor, "project_1", version.ID)
	if err != nil || checked.Check == nil || !checked.Check.Passed {
		t.Fatalf("checked=%#v err=%v", checked, err)
	}
	approved, err := service.ApproveVersion(context.Background(), actor, "project_1", version.ID)
	if err != nil || approved.Status != CreativeVersionApproved {
		t.Fatalf("approved=%#v err=%v", approved, err)
	}
	packaged, err := service.DeliverVersion(context.Background(), actor, "project_1", version.ID)
	if err != nil || packaged.EditTaskID != task.ID || packaged.VideoSnapshot == nil || packaged.VideoSnapshot.Editing == nil {
		t.Fatalf("package=%#v err=%v", packaged, err)
	}
}

func TestC6RejectsPreviewOrStaleExportSubmission(t *testing.T) {
	request := SubmitEditingVersionRequest{RenderJobID: "preview_1", ExpectedTimelineVersion: 2}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	preview := EditingRenderJob{Kind: EditingRenderPreview, Status: EditingRenderSucceeded}
	if editingRenderMaySubmit(preview, "edit_1", 2) == nil {
		t.Fatal("preview must never become a CreativeVersion")
	}
	export := EditingRenderJob{EditTaskID: "edit_1", Kind: EditingRenderExport, Status: EditingRenderSucceeded, Timeline: TimelineVersion{Version: 1}}
	if editingRenderMaySubmit(export, "edit_1", 2) == nil {
		t.Fatal("stale export must never become a CreativeVersion")
	}
}
