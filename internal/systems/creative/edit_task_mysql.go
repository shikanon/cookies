package creative

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

func (r MySQLRepository) CreateEditTask(ctx context.Context, task EditTask, timeline *TimelineVersion) (EditTask, error) {
	if r.DB == nil {
		return EditTask{}, fmt.Errorf("creative repository database is required")
	}
	if err := task.Validate(); err != nil {
		return EditTask{}, err
	}
	if (task.CurrentTimeline == nil) != (timeline == nil) {
		return EditTask{}, fmt.Errorf("edit task current timeline must match the initial timeline version")
	}
	var payload []byte
	if timeline != nil {
		if task.CurrentTimeline.Version != timeline.Version || task.CurrentTimeline.ContentHash != timeline.ContentHash {
			return EditTask{}, fmt.Errorf("edit task current timeline must match the initial timeline version")
		}
		var err error
		payload, err = marshalTimelinePayload(*timeline)
		if err != nil {
			return EditTask{}, err
		}
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return EditTask{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO creative_edit_tasks
		(organization_id, project_id, edit_task_id, display_name, status, entry_source, source_creative_task_id,
		 current_timeline_version, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?)`,
		task.OrganizationID, task.ProjectID, task.ID, task.DisplayName, task.Status, task.EntrySource, task.SourceCreativeTaskID,
		nullableTimelineVersion(timeline), task.CreatedBy, task.CreatedAt, task.UpdatedAt)
	if err != nil {
		return EditTask{}, err
	}
	if timeline != nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO creative_edit_timeline_versions
			(organization_id, project_id, edit_task_id, version, schema_version, parent_version, content_payload, content_hash, change_summary, operation_batch_id, compiler_compatibility, created_by, created_at)
			VALUES (?, ?, ?, ?, ?, NULLIF(?, 0), ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?)`, task.OrganizationID, task.ProjectID, task.ID, timeline.Version, timeline.Schema(), timeline.ParentVersion, payload,
			timeline.ContentHash, timeline.ChangeSummary, timeline.OperationBatchID, timeline.CompilerCompatibility, timeline.CreatedBy, timeline.CreatedAt)
		if err != nil {
			return EditTask{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return EditTask{}, err
	}
	return task, nil
}

func (r MySQLRepository) GetEditTask(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, taskID string) (EditTask, error) {
	if r.DB == nil {
		return EditTask{}, fmt.Errorf("creative repository database is required")
	}
	return scanEditTask(r.DB.QueryRowContext(ctx, `SELECT
		t.edit_task_id, t.organization_id, t.project_id, t.display_name, t.status, t.entry_source,
		COALESCE(t.source_creative_task_id, ''), t.created_by, t.created_at, t.updated_at,
		v.version, v.schema_version, v.parent_version, v.content_payload, v.content_hash, v.change_summary, v.operation_batch_id, v.compiler_compatibility, v.created_by, v.created_at
		FROM creative_edit_tasks t
		LEFT JOIN creative_edit_timeline_versions v
		  ON v.organization_id=t.organization_id AND v.project_id=t.project_id
		 AND v.edit_task_id=t.edit_task_id AND v.version=t.current_timeline_version
		WHERE t.organization_id=? AND t.project_id=? AND t.edit_task_id=?`, organizationID, projectID, taskID))
}

func (r MySQLRepository) FindEditTaskBySource(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, source EditTaskEntrySource, sourceCreativeTaskID string) (EditTask, error) {
	if r.DB == nil {
		return EditTask{}, fmt.Errorf("creative repository database is required")
	}
	var id string
	err := r.DB.QueryRowContext(ctx, `SELECT edit_task_id FROM creative_edit_tasks
		WHERE organization_id=? AND project_id=? AND entry_source=? AND source_creative_task_id=?
		ORDER BY updated_at DESC, edit_task_id DESC LIMIT 1`, organizationID, projectID, source, sourceCreativeTaskID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return EditTask{}, ErrNotFound
	}
	if err != nil {
		return EditTask{}, err
	}
	return r.GetEditTask(ctx, organizationID, projectID, id)
}

func (r MySQLRepository) ListEditTasks(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, status EditTaskStatus, limit int) ([]EditTask, error) {
	if r.DB == nil {
		return nil, fmt.Errorf("creative repository database is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := `SELECT
		t.edit_task_id, t.organization_id, t.project_id, t.display_name, t.status, t.entry_source,
		COALESCE(t.source_creative_task_id, ''), t.created_by, t.created_at, t.updated_at,
		v.version, v.schema_version, v.parent_version, v.content_payload, v.content_hash, v.change_summary, v.operation_batch_id, v.compiler_compatibility, v.created_by, v.created_at
		FROM creative_edit_tasks t
		LEFT JOIN creative_edit_timeline_versions v
		  ON v.organization_id=t.organization_id AND v.project_id=t.project_id
		 AND v.edit_task_id=t.edit_task_id AND v.version=t.current_timeline_version
		WHERE t.organization_id=? AND t.project_id=?`
	args := []any{organizationID, projectID}
	if status != "" {
		query += ` AND t.status=?`
		args = append(args, status)
	}
	query += ` ORDER BY t.updated_at DESC, t.edit_task_id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]EditTask, 0)
	for rows.Next() {
		item, scanErr := scanEditTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r MySQLRepository) AppendEditTimeline(ctx context.Context, task EditTask, expectedVersion int64, timeline TimelineVersion) (EditTask, error) {
	if r.DB == nil {
		return EditTask{}, fmt.Errorf("creative repository database is required")
	}
	if err := timeline.Validate(); err != nil {
		return EditTask{}, err
	}
	if timeline.Version != expectedVersion+1 {
		return EditTask{}, ErrVersionConflict
	}
	payload, err := marshalTimelinePayload(timeline)
	if err != nil {
		return EditTask{}, err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return EditTask{}, err
	}
	defer tx.Rollback()
	var current sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT current_timeline_version FROM creative_edit_tasks
		WHERE organization_id=? AND project_id=? AND edit_task_id=? FOR UPDATE`, task.OrganizationID, task.ProjectID, task.ID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return EditTask{}, ErrNotFound
	}
	if err != nil {
		return EditTask{}, err
	}
	currentVersion := int64(0)
	if current.Valid {
		currentVersion = current.Int64
	}
	if currentVersion != expectedVersion {
		return EditTask{}, ErrVersionConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO creative_edit_timeline_versions
		(organization_id, project_id, edit_task_id, version, schema_version, parent_version, content_payload, content_hash, change_summary, operation_batch_id, compiler_compatibility, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, NULLIF(?, 0), ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?)`, task.OrganizationID, task.ProjectID, task.ID, timeline.Version, timeline.Schema(), timeline.ParentVersion, payload,
		timeline.ContentHash, timeline.ChangeSummary, timeline.OperationBatchID, timeline.CompilerCompatibility, timeline.CreatedBy, timeline.CreatedAt)
	if err != nil {
		return EditTask{}, err
	}
	query := `UPDATE creative_edit_tasks SET current_timeline_version=?, updated_at=?
		WHERE organization_id=? AND project_id=? AND edit_task_id=? AND current_timeline_version=?`
	args := []any{timeline.Version, timeline.CreatedAt, task.OrganizationID, task.ProjectID, task.ID, expectedVersion}
	if expectedVersion == 0 {
		query = `UPDATE creative_edit_tasks SET current_timeline_version=?, updated_at=?
			WHERE organization_id=? AND project_id=? AND edit_task_id=? AND current_timeline_version IS NULL`
		args = []any{timeline.Version, timeline.CreatedAt, task.OrganizationID, task.ProjectID, task.ID}
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return EditTask{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return EditTask{}, err
	}
	if affected != 1 {
		return EditTask{}, ErrVersionConflict
	}
	if err := tx.Commit(); err != nil {
		return EditTask{}, err
	}
	return r.GetEditTask(ctx, task.OrganizationID, task.ProjectID, task.ID)
}

func (r MySQLRepository) ListEditTimelineVersions(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, taskID string, limit int) ([]TimelineVersion, error) {
	if r.DB == nil {
		return nil, fmt.Errorf("creative repository database is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT version,schema_version,parent_version,content_payload,content_hash,change_summary,operation_batch_id,compiler_compatibility,created_by,created_at FROM creative_edit_timeline_versions WHERE organization_id=? AND project_id=? AND edit_task_id=? ORDER BY version DESC LIMIT ?`, org, project, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]TimelineVersion, 0)
	for rows.Next() {
		value, scanErr := scanTimelineVersion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, value)
	}
	return items, rows.Err()
}

func (r MySQLRepository) UpdateEditTaskStatus(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, taskID string, status EditTaskStatus, now time.Time) error {
	if !validEditTaskStatus(status) {
		return fmt.Errorf("edit task status is invalid")
	}
	result, err := r.DB.ExecContext(ctx, `UPDATE creative_edit_tasks SET status=?,updated_at=? WHERE organization_id=? AND project_id=? AND edit_task_id=? AND status<>'archived'`, status, now, org, project, taskID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrInvalidState
	}
	return nil
}

type editTaskScanner interface {
	Scan(...any) error
}

func scanTimelineVersion(row editTaskScanner) (TimelineVersion, error) {
	var value TimelineVersion
	var payload []byte
	var parent sql.NullInt64
	var summary, batch sql.NullString
	if err := row.Scan(&value.Version, &value.SchemaVersion, &parent, &payload, &value.ContentHash, &summary, &batch, &value.CompilerCompatibility, &value.CreatedBy, &value.CreatedAt); err != nil {
		return value, err
	}
	value.ParentVersion, value.ChangeSummary, value.OperationBatchID = parent.Int64, summary.String, batch.String
	if value.Schema() == EditingTimelineSchemaV2 {
		var v2 EditingTimelineV2
		if err := json.Unmarshal(payload, &v2); err != nil {
			return value, err
		}
		value.TimelineV2 = &v2
	} else if err := json.Unmarshal(payload, &value.Timeline); err != nil {
		return value, err
	}
	return value, value.Validate()
}

func scanEditTask(row editTaskScanner) (EditTask, error) {
	var task EditTask
	var timeline TimelineVersion
	var version sql.NullInt64
	var schemaVersion, changeSummary, operationBatchID, compilerCompatibility sql.NullString
	var parentVersion sql.NullInt64
	var payload []byte
	var contentHash, timelineCreatedBy sql.NullString
	var timelineCreatedAt sql.NullTime
	err := row.Scan(&task.ID, &task.OrganizationID, &task.ProjectID, &task.DisplayName, &task.Status, &task.EntrySource,
		&task.SourceCreativeTaskID, &task.CreatedBy, &task.CreatedAt, &task.UpdatedAt,
		&version, &schemaVersion, &parentVersion, &payload, &contentHash, &changeSummary, &operationBatchID, &compilerCompatibility, &timelineCreatedBy, &timelineCreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return EditTask{}, ErrNotFound
	}
	if err != nil {
		return EditTask{}, err
	}
	if version.Valid {
		timeline.SchemaVersion = schemaVersion.String
		if timeline.Schema() == EditingTimelineSchemaV2 {
			var v2 EditingTimelineV2
			if err := json.Unmarshal(payload, &v2); err != nil {
				return EditTask{}, err
			}
			timeline.TimelineV2 = &v2
		} else if err := json.Unmarshal(payload, &timeline.Timeline); err != nil {
			return EditTask{}, err
		}
		timeline.Version, timeline.ContentHash, timeline.CreatedBy, timeline.CreatedAt = version.Int64, contentHash.String, timelineCreatedBy.String, timelineCreatedAt.Time
		timeline.ParentVersion, timeline.ChangeSummary, timeline.OperationBatchID, timeline.CompilerCompatibility = parentVersion.Int64, changeSummary.String, operationBatchID.String, compilerCompatibility.String
		task.CurrentTimeline = &timeline
	}
	task.ContractVersion = EditTaskContractVersion
	if err := task.Validate(); err != nil {
		return EditTask{}, err
	}
	return task, nil
}

func marshalTimelinePayload(timeline TimelineVersion) ([]byte, error) {
	if timeline.Schema() == EditingTimelineSchemaV2 {
		return json.Marshal(timeline.TimelineV2)
	}
	return json.Marshal(timeline.Timeline)
}

func nullableTimelineVersion(timeline *TimelineVersion) any {
	if timeline == nil {
		return nil
	}
	return timeline.Version
}
