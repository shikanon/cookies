package creative

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

func (r MySQLRepository) AppendEditOperations(ctx context.Context, task EditTask, expectedVersion int64, version TimelineVersion, audit EditOperationAudit) (EditTask, error) {
	if r.DB == nil {
		return EditTask{}, fmt.Errorf("creative repository database is required")
	}
	if err := version.Validate(); err != nil {
		return EditTask{}, err
	}
	if version.Schema() != EditingTimelineSchemaV2 || version.Version != expectedVersion+1 || version.OperationBatchID != audit.Batch.BatchID {
		return EditTask{}, fmt.Errorf("operation timeline version is inconsistent")
	}
	payload, err := marshalTimelinePayload(version)
	if err != nil {
		return EditTask{}, err
	}
	operations, err := json.Marshal(audit.Batch.Operations)
	if err != nil {
		return EditTask{}, err
	}
	inverses, err := json.Marshal(audit.InverseOperations)
	if err != nil {
		return EditTask{}, err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return EditTask{}, err
	}
	defer tx.Rollback()
	var current sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT current_timeline_version FROM creative_edit_tasks WHERE organization_id=? AND project_id=? AND edit_task_id=? FOR UPDATE`, task.OrganizationID, task.ProjectID, task.ID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return EditTask{}, ErrNotFound
	}
	if err != nil {
		return EditTask{}, err
	}
	if !current.Valid || current.Int64 != expectedVersion {
		return EditTask{}, ErrVersionConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO creative_edit_timeline_versions (organization_id,project_id,edit_task_id,version,schema_version,parent_version,content_payload,content_hash,change_summary,operation_batch_id,compiler_compatibility,created_by,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, task.OrganizationID, task.ProjectID, task.ID, version.Version, version.Schema(), version.ParentVersion, payload, version.ContentHash, version.ChangeSummary, version.OperationBatchID, version.CompilerCompatibility, version.CreatedBy, version.CreatedAt)
	if err != nil {
		return EditTask{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO creative_edit_operation_batches (organization_id,project_id,edit_task_id,batch_id,base_timeline_version,result_timeline_version,actor,operations_payload,inverse_payload,result_content_hash,change_summary,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, audit.OrganizationID, audit.ProjectID, audit.EditTaskID, audit.Batch.BatchID, audit.Batch.BaseTimelineVersion, audit.ResultTimelineVersion, audit.Batch.Actor, operations, inverses, audit.ResultContentHash, audit.ChangeSummary, audit.CreatedAt)
	if err != nil {
		return EditTask{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE creative_edit_tasks SET current_timeline_version=?,updated_at=? WHERE organization_id=? AND project_id=? AND edit_task_id=? AND current_timeline_version=?`, version.Version, version.CreatedAt, task.OrganizationID, task.ProjectID, task.ID, expectedVersion)
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
	if err = tx.Commit(); err != nil {
		return EditTask{}, err
	}
	return r.GetEditTask(ctx, task.OrganizationID, task.ProjectID, task.ID)
}
