package creative

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

func TestSaveEditTimelineRequestDecodesV2WithoutBreakingV1Callers(t *testing.T) {
	document := operationTestDocument()
	payload, err := json.Marshal(struct {
		ExpectedVersion int64             `json:"expected_version"`
		Timeline        EditingTimelineV2 `json:"timeline"`
	}{ExpectedVersion: 3, Timeline: *document.V2})
	if err != nil {
		t.Fatal(err)
	}
	var request SaveEditTimelineRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatal(err)
	}
	if request.ExpectedVersion != 3 || request.TimelineV2 == nil || request.TimelineV2.SchemaVersion != EditingTimelineSchemaV2 {
		t.Fatalf("v2 save envelope was not decoded: %#v", request)
	}
}

type memoryEditTaskRepository struct {
	stored EditTask
	tasks  []EditTask
	audits []EditOperationAudit
}

func (r *memoryEditTaskRepository) AppendEditOperations(_ context.Context, task EditTask, expectedVersion int64, version TimelineVersion, audit EditOperationAudit) (EditTask, error) {
	if r.stored.ID != task.ID || r.stored.CurrentTimeline == nil || r.stored.CurrentTimeline.Version != expectedVersion {
		return EditTask{}, ErrVersionConflict
	}
	r.stored.CurrentTimeline = &version
	r.audits = append(r.audits, audit)
	return r.stored, nil
}

type projectBoundEditingAssetReader struct {
	project  contract.ProjectID
	snapshot CreativeAssetSnapshot
}

func (r projectBoundEditingAssetReader) ReadForCreative(_ context.Context, _ contract.ActorContext, project contract.ProjectID, ref contract.AssetVersionRef) (CreativeAssetSnapshot, error) {
	if project != r.project || ref != r.snapshot.Ref {
		return CreativeAssetSnapshot{}, ErrNotFound
	}
	return r.snapshot, nil
}

func (r *memoryEditTaskRepository) CreateEditTask(_ context.Context, task EditTask, timeline *TimelineVersion) (EditTask, error) {
	r.stored = task
	r.stored.CurrentTimeline = timeline
	r.tasks = append(r.tasks, r.stored)
	return r.stored, nil
}

func (r *memoryEditTaskRepository) GetEditTask(_ context.Context, org contract.OrganizationID, project contract.ProjectID, id string) (EditTask, error) {
	if r.stored.ID != id || r.stored.OrganizationID != org || r.stored.ProjectID != project {
		return EditTask{}, ErrNotFound
	}
	return r.stored, nil
}

func (r *memoryEditTaskRepository) FindEditTaskBySource(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, _ EditTaskEntrySource, _ string) (EditTask, error) {
	return EditTask{}, ErrNotFound
}

func (r *memoryEditTaskRepository) ListEditTasks(_ context.Context, org contract.OrganizationID, project contract.ProjectID, status EditTaskStatus, limit int) ([]EditTask, error) {
	items := make([]EditTask, 0, min(limit, len(r.tasks)))
	for _, task := range r.tasks {
		if task.OrganizationID == org && task.ProjectID == project && (status == "" || task.Status == status) {
			items = append(items, task)
			if len(items) == limit {
				break
			}
		}
	}
	return items, nil
}

func (r *memoryEditTaskRepository) AppendEditTimeline(_ context.Context, task EditTask, expectedVersion int64, version TimelineVersion) (EditTask, error) {
	if r.stored.ID != task.ID || (r.stored.CurrentTimeline == nil && expectedVersion != 0) || (r.stored.CurrentTimeline != nil && r.stored.CurrentTimeline.Version != expectedVersion) {
		return EditTask{}, ErrVersionConflict
	}
	r.stored.CurrentTimeline = &version
	return r.stored, nil
}
func (r *memoryEditTaskRepository) ListEditTimelineVersions(_ context.Context, org contract.OrganizationID, project contract.ProjectID, id string, limit int) ([]TimelineVersion, error) {
	if r.stored.ID != id || r.stored.OrganizationID != org || r.stored.ProjectID != project {
		return nil, ErrNotFound
	}
	if r.stored.CurrentTimeline == nil {
		return []TimelineVersion{}, nil
	}
	return []TimelineVersion{*r.stored.CurrentTimeline}, nil
}

func (r *memoryEditTaskRepository) UpdateEditTaskStatus(_ context.Context, org contract.OrganizationID, project contract.ProjectID, id string, status EditTaskStatus, now time.Time) error {
	task, err := r.GetEditTask(context.Background(), org, project, id)
	if err != nil {
		return err
	}
	task.Status, task.UpdatedAt = status, now
	r.stored = task
	return nil
}

func TestCreateShortDramaV2EditTaskPrefillsPrerollThenSourceVideo(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	repository := &memoryEditTaskRepository{}
	preroll := contract.AssetVersionRef{AssetID: "asset_preroll", Version: 1}
	source := contract.AssetVersionRef{AssetID: "asset_source", Version: 3}
	service := Service{
		Projects:  testProjects{},
		EditTasks: repository,
		Assets: testAssetReader{snapshots: map[contract.AssetID]CreativeAssetSnapshot{
			preroll.AssetID: {Ref: preroll, Kind: contract.AssetVideo, MIMEType: "video/mp4", Ready: true, DurationMS: 6000},
			source.AssetID:  {Ref: source, Kind: contract.AssetVideo, MIMEType: "video/mp4", Ready: true, DurationMS: 15000},
		}},
		NewID: func(prefix string) (string, error) { return prefix + "_1", nil },
		Now:   func() time.Time { return now },
	}
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, Scopes: []contract.Scope{ScopeWrite}}

	created, err := service.CreateShortDramaV2EditTask(context.Background(), actor, "project_1", CreateShortDramaV2EditTaskRequest{
		SourceCreativeTaskID: "creative_task_1", PrerollAsset: preroll, SourceAsset: source,
	})
	if err != nil {
		t.Fatalf("CreateShortDramaV2EditTask() error = %v", err)
	}
	if created.EntrySource != EditTaskEntryShortDramaV2 || created.SourceCreativeTaskID != "creative_task_1" || created.CurrentTimeline.Version != 1 {
		t.Fatalf("created edit task = %#v", created)
	}
	clips := created.CurrentTimeline.Timeline.Tracks[0].Clips
	if len(clips) != 2 || clips[0].AssetRef == nil || *clips[0].AssetRef != preroll || clips[0].TimelineStartMS != 0 || clips[0].TimelineEndMS != 6000 ||
		clips[1].AssetRef == nil || *clips[1].AssetRef != source || clips[1].TimelineStartMS != 6000 || clips[1].TimelineEndMS != 21000 {
		t.Fatalf("prefilled primary timeline clips = %#v", clips)
	}
}

func TestManualEditTaskCanStartEmptyAndFirstSaveCreatesTimelineVersionOne(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	repository := &memoryEditTaskRepository{}
	asset := contract.AssetVersionRef{AssetID: "asset_source", Version: 1}
	service := Service{Projects: testProjects{}, EditTasks: repository,
		Assets: testAssetReader{snapshots: map[contract.AssetID]CreativeAssetSnapshot{
			asset.AssetID: {Ref: asset, Kind: contract.AssetVideo, MIMEType: "video/mp4", Ready: true, DurationMS: 6000},
		}}, NewID: func(prefix string) (string, error) { return prefix + "_1", nil }, Now: func() time.Time { return now }}
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, Scopes: []contract.Scope{ScopeWrite}}

	created, err := service.CreateEditTask(context.Background(), actor, "project_1", CreateEditTaskRequest{DisplayName: "空白剪辑"})
	if err != nil {
		t.Fatalf("CreateEditTask() error = %v", err)
	}
	if created.CurrentTimeline != nil {
		t.Fatalf("empty task timeline = %#v", created.CurrentTimeline)
	}
	timeline := EditingTimelineV1{SchemaVersion: EditingTimelineSchemaV1, OutputProfile: EditingMVPVerticalOutputProfile, DurationMS: 6000,
		Tracks: []EditingTimelineTrack{{ID: "video-primary", Role: EditingTrackPrimaryVideo, Clips: []EditingTimelineClip{{ID: "clip-source", AssetRef: &asset, TimelineEndMS: 6000, SourceOutMS: 6000}}}}}
	updated, err := service.SaveEditTimeline(context.Background(), actor, "project_1", created.ID, SaveEditTimelineRequest{ExpectedVersion: 0, Timeline: timeline})
	if err != nil {
		t.Fatalf("SaveEditTimeline() error = %v", err)
	}
	if updated.CurrentTimeline == nil || updated.CurrentTimeline.Version != 1 {
		t.Fatalf("first timeline version = %#v", updated.CurrentTimeline)
	}
}

func TestSaveEditTimelineRejectsSourceRangePastAssetDuration(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	repository := &memoryEditTaskRepository{}
	asset := contract.AssetVersionRef{AssetID: "asset_source", Version: 1}
	service := Service{Projects: testProjects{}, EditTasks: repository,
		Assets: testAssetReader{snapshots: map[contract.AssetID]CreativeAssetSnapshot{
			asset.AssetID: {Ref: asset, Kind: contract.AssetVideo, MIMEType: "video/mp4", Ready: true, DurationMS: 5000},
		}}, NewID: func(prefix string) (string, error) { return prefix + "_1", nil }, Now: func() time.Time { return now }}
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, Scopes: []contract.Scope{ScopeWrite}}
	timeline := EditingTimelineV1{SchemaVersion: EditingTimelineSchemaV1, OutputProfile: EditingMVPVerticalOutputProfile, DurationMS: 6000,
		Tracks: []EditingTimelineTrack{{ID: "video-primary", Role: EditingTrackPrimaryVideo, Clips: []EditingTimelineClip{{ID: "clip-source", AssetRef: &asset, TimelineEndMS: 6000, SourceOutMS: 6000}}}}}

	_, err := service.CreateEditTask(context.Background(), actor, "project_1", CreateEditTaskRequest{DisplayName: "越界剪辑", Timeline: &timeline})
	if err == nil {
		t.Fatal("source range past the probed asset duration must be rejected")
	}
}

func TestSaveEditTimelineAppendsAnImmutableVersion(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	repository := &memoryEditTaskRepository{}
	asset := contract.AssetVersionRef{AssetID: "asset_source", Version: 1}
	timeline := EditingTimelineV1{SchemaVersion: EditingTimelineSchemaV1, OutputProfile: EditingMVPVerticalOutputProfile, DurationMS: 6000,
		Tracks: []EditingTimelineTrack{{ID: "video-primary", Role: EditingTrackPrimaryVideo, Clips: []EditingTimelineClip{{
			ID: "clip-source", AssetRef: &asset, TimelineEndMS: 6000, SourceOutMS: 6000,
		}}}}}
	service := Service{Projects: testProjects{}, EditTasks: repository,
		Assets: testAssetReader{snapshots: map[contract.AssetID]CreativeAssetSnapshot{
			asset.AssetID: {Ref: asset, Kind: contract.AssetVideo, MIMEType: "video/mp4", Ready: true, DurationMS: 6000},
		}}, NewID: func(prefix string) (string, error) { return prefix + "_1", nil }, Now: func() time.Time { return now }}
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, Scopes: []contract.Scope{ScopeWrite}}
	created, err := service.CreateEditTask(context.Background(), actor, "project_1", CreateEditTaskRequest{DisplayName: "手动剪辑", Timeline: &timeline})
	if err != nil {
		t.Fatalf("CreateEditTask() error = %v", err)
	}
	updatedTimeline := timeline
	updatedTimeline.Tracks = append(updatedTimeline.Tracks, EditingTimelineTrack{ID: "caption", Role: EditingTrackCaption, Clips: []EditingTimelineClip{{
		ID: "caption-1", TimelineEndMS: 6000, Text: "前贴结束，正片开始",
	}}})
	updated, err := service.SaveEditTimeline(context.Background(), actor, "project_1", created.ID, SaveEditTimelineRequest{ExpectedVersion: 1, Timeline: updatedTimeline})
	if err != nil {
		t.Fatalf("SaveEditTimeline() error = %v", err)
	}
	if updated.CurrentTimeline.Version != 2 || updated.CurrentTimeline.ContentHash == created.CurrentTimeline.ContentHash {
		t.Fatalf("updated edit task = %#v", updated)
	}
}

func TestCreativeVersionCanEnterEditorButCannotCrossProject(t *testing.T) {
	now := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	ref := contract.AssetVersionRef{AssetID: "final_brand_video", Version: 2}
	repository := &memoryEditTaskRepository{}
	service := Service{Projects: testProjects{}, EditTasks: repository, Assets: projectBoundEditingAssetReader{project: "project_1", snapshot: CreativeAssetSnapshot{Ref: ref, Kind: contract.AssetVideo, Ready: true, DurationMS: 15000}}, NewID: func(prefix string) (string, error) { return prefix + "_1", nil }, Now: func() time.Time { return now }}
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, Scopes: []contract.Scope{ScopeWrite}}
	created, err := service.CreateCreativeVersionEditTask(context.Background(), actor, "project_1", CreateCreativeVersionEditTaskRequest{SourceCreativeTaskID: "brand_task_1", FinalVideo: ref})
	if err != nil {
		t.Fatal(err)
	}
	if created.EntrySource != EditTaskEntryCreativeVersion || created.CurrentTimeline.Timeline.Tracks[0].Clips[0].AssetRef == nil || *created.CurrentTimeline.Timeline.Tracks[0].Clips[0].AssetRef != ref {
		t.Fatalf("created=%#v", created)
	}
	_, err = service.CreateCreativeVersionEditTask(context.Background(), actor, "project_2", CreateCreativeVersionEditTaskRequest{SourceCreativeTaskID: "brand_task_2", FinalVideo: ref})
	if err == nil {
		t.Fatal("cross-project final video must be rejected")
	}
}

func TestEditTaskReloadsLatestConfirmedTimelineAndRejectsStaleSave(t *testing.T) {
	now := time.Date(2026, 8, 6, 10, 0, 0, 0, time.UTC)
	ref := contract.AssetVersionRef{AssetID: "source", Version: 1}
	repository := &memoryEditTaskRepository{}
	timeline := EditingTimelineV1{SchemaVersion: EditingTimelineSchemaV1, OutputProfile: EditingMVPVerticalOutputProfile, DurationMS: 6000, Tracks: []EditingTimelineTrack{{ID: "video", Role: EditingTrackPrimaryVideo, Clips: []EditingTimelineClip{{ID: "clip", AssetRef: &ref, TimelineEndMS: 6000, SourceOutMS: 6000}}}}}
	service := Service{Projects: testProjects{}, EditTasks: repository, Assets: testAssetReader{snapshots: map[contract.AssetID]CreativeAssetSnapshot{ref.AssetID: {Ref: ref, Kind: contract.AssetVideo, Ready: true, DurationMS: 6000}}}, NewID: func(prefix string) (string, error) { return prefix + "_1", nil }, Now: func() time.Time { return now }}
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, Scopes: []contract.Scope{ScopeRead, ScopeWrite}}
	created, err := service.CreateEditTask(context.Background(), actor, "project_1", CreateEditTaskRequest{DisplayName: "可恢复剪辑", Timeline: &timeline})
	if err != nil {
		t.Fatal(err)
	}
	updated := timeline
	updated.Tracks = append(updated.Tracks, EditingTimelineTrack{ID: "caption", Role: EditingTrackCaption, Clips: []EditingTimelineClip{{ID: "caption-1", TimelineEndMS: 6000, Text: "已确认字幕"}}})
	if _, err = service.SaveEditTimeline(context.Background(), actor, "project_1", created.ID, SaveEditTimelineRequest{ExpectedVersion: 1, Timeline: updated}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.SaveEditTimeline(context.Background(), actor, "project_1", created.ID, SaveEditTimelineRequest{ExpectedVersion: 1, Timeline: timeline}); err != ErrEditTimelineVersionConflict {
		t.Fatalf("stale save error=%v", err)
	}
	reloaded, err := service.GetEditTask(context.Background(), actor, "project_1", created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.CurrentTimeline.Version != 2 || len(reloaded.CurrentTimeline.Timeline.Tracks) != 2 {
		t.Fatalf("reloaded=%#v", reloaded.CurrentTimeline)
	}
}

func TestUserCanListProjectEditTasksByStatusWithoutCrossProjectResults(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	repository := &memoryEditTaskRepository{tasks: []EditTask{
		{ContractVersion: EditTaskContractVersion, ID: "edit_recent", OrganizationID: "org_1", ProjectID: "project_1", DisplayName: "最近草稿", Status: EditTaskDraft, EntrySource: EditTaskEntryManual, CreatedBy: "user_1", CreatedAt: now.Add(-time.Hour), UpdatedAt: now},
		{ContractVersion: EditTaskContractVersion, ID: "edit_failed", OrganizationID: "org_1", ProjectID: "project_1", DisplayName: "失败任务", Status: EditTaskFailed, EntrySource: EditTaskEntryManual, CreatedBy: "user_1", CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-time.Minute)},
		{ContractVersion: EditTaskContractVersion, ID: "edit_other", OrganizationID: "org_1", ProjectID: "project_2", DisplayName: "其他项目", Status: EditTaskDraft, EntrySource: EditTaskEntryManual, CreatedBy: "user_1", CreatedAt: now, UpdatedAt: now},
	}}
	service := Service{Projects: testProjects{}, EditTasks: repository}
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, Scopes: []contract.Scope{ScopeRead}}

	items, err := service.ListEditTasks(context.Background(), actor, "project_1", ListEditTasksRequest{Status: EditTaskDraft, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "edit_recent" {
		t.Fatalf("draft project tasks = %#v", items)
	}
}

func TestTwoClientsApplyingOperationsToTheSameVersionGetOneSuccessAndOneConflict(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	repo := &memoryEditTaskRepository{}
	firstRef := contract.AssetVersionRef{AssetID: "asset_1", Version: 1}
	secondRef := contract.AssetVersionRef{AssetID: "asset_2", Version: 1}
	v1 := EditingTimelineV1{SchemaVersion: EditingTimelineSchemaV1, OutputProfile: EditingMVPVerticalOutputProfile, DurationMS: 3000, Tracks: []EditingTimelineTrack{{ID: "video-primary", Role: EditingTrackPrimaryVideo, Clips: []EditingTimelineClip{{ID: "clip-1", AssetRef: &firstRef, TimelineEndMS: 3000, SourceOutMS: 3000}}}}}
	version, _ := newTimelineVersion(v1, 1, "user_1", now)
	repo.stored = EditTask{ContractVersion: EditTaskContractVersion, ID: "edit_1", OrganizationID: "org_1", ProjectID: "project_1", DisplayName: "test", Status: EditTaskDraft, EntrySource: EditTaskEntryManual, CurrentTimeline: &version, CreatedBy: "user_1", CreatedAt: now, UpdatedAt: now}
	service := Service{Projects: testProjects{}, EditTasks: repo, Assets: testAssetReader{snapshots: map[contract.AssetID]CreativeAssetSnapshot{firstRef.AssetID: {Ref: firstRef, Kind: contract.AssetVideo, Ready: true, DurationMS: 3000}, secondRef.AssetID: {Ref: secondRef, Kind: contract.AssetVideo, Ready: true, DurationMS: 1000}}}, Now: func() time.Time { return now }}
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, Scopes: []contract.Scope{ScopeRead, ScopeWrite}}
	request := ApplyEditOperationsRequest{Batch: EditOperationBatch{BatchID: "batch_1", BaseTimelineVersion: 1, Actor: "user_1", Operations: []EditOperation{{OperationID: "op_1", Type: OperationInsertAsset, BaseTimelineVersion: 1, Actor: "user_1", TrackID: "video-primary", ClipID: "clip-2", AssetRef: &secondRef, AtFrame: 90, DurationFrames: 30, Source: &EditingSourceRangeV2{OutUS: 1_000_000}}}}}
	updated, err := service.ApplyEditOperations(context.Background(), actor, "project_1", "edit_1", request)
	if err != nil {
		t.Fatal(err)
	}
	if updated.CurrentTimeline.Schema() != EditingTimelineSchemaV2 || updated.CurrentTimeline.Version != 2 || len(repo.audits) != 1 {
		t.Fatalf("unexpected result %#v", updated.CurrentTimeline)
	}
	request.Batch.BatchID = "batch_2"
	request.Batch.Operations[0].OperationID = "op_2"
	if _, err := service.ApplyEditOperations(context.Background(), actor, "project_1", "edit_1", request); err != ErrOperationVersionConflict {
		t.Fatalf("expected stable conflict, got %v", err)
	}
}
