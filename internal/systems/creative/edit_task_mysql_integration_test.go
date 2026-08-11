package creative

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/shikanon/cookies/internal/platform/contract"
)

func TestEditTaskMySQLPersistsFirstTimelineForEmptyDraft(t *testing.T) {
	dsn := os.Getenv("COOKIES_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("COOKIES_TEST_MYSQL_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	organizationID := contract.OrganizationID("org_edit_it_" + suffix)
	projectID := contract.ProjectID("project_edit_it_" + suffix)
	taskID := "edit_it_" + suffix
	now := time.Now().UTC().Truncate(time.Microsecond)
	repository := MySQLRepository{DB: db}
	task := EditTask{
		ContractVersion: EditTaskContractVersion,
		ID:              taskID, OrganizationID: organizationID, ProjectID: projectID,
		DisplayName: "empty draft", Status: EditTaskDraft, EntrySource: EditTaskEntryManual,
		CreatedBy: "user_edit_it", CreatedAt: now, UpdatedAt: now,
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM creative_edit_timeline_versions WHERE organization_id=? AND project_id=? AND edit_task_id=?`, organizationID, projectID, taskID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM creative_edit_tasks WHERE organization_id=? AND project_id=? AND edit_task_id=?`, organizationID, projectID, taskID)
	})
	if _, err := repository.CreateEditTask(ctx, task, nil); err != nil {
		t.Fatalf("create empty task: %v", err)
	}
	ref := contract.AssetVersionRef{AssetID: "asset_edit_it", Version: 1}
	timeline := EditingTimelineV1{
		SchemaVersion: EditingTimelineSchemaV1,
		OutputProfile: EditingMVPVerticalOutputProfile,
		DurationMS:    1000,
		Tracks:        []EditingTimelineTrack{{ID: "video", Role: EditingTrackPrimaryVideo, Clips: []EditingTimelineClip{{ID: "clip", AssetRef: &ref, TimelineEndMS: 1000, SourceOutMS: 1000}}}},
	}
	version, err := newTimelineVersion(timeline, 1, "user_edit_it", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	saved, err := repository.AppendEditTimeline(ctx, task, 0, version)
	if err != nil {
		t.Fatalf("append first timeline: %v", err)
	}
	if saved.CurrentTimeline == nil || saved.CurrentTimeline.Version != 1 {
		t.Fatalf("expected persisted timeline v1, got %#v", saved.CurrentTimeline)
	}
}
