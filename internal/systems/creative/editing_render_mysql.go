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

func (r MySQLRepository) CreateEditingRender(ctx context.Context, job EditingRenderJob) (EditingRenderJob, error) {
	if r.DB == nil {
		return EditingRenderJob{}, fmt.Errorf("creative repository database is required")
	}
	if err := job.Validate(); err != nil {
		return EditingRenderJob{}, err
	}
	payload, err := marshalTimelinePayload(job.Timeline)
	if err != nil {
		return EditingRenderJob{}, err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return EditingRenderJob{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO creative_edit_render_jobs (organization_id,project_id,edit_render_job_id,edit_task_id,timeline_version,timeline_schema_version,timeline_payload,timeline_hash,compiler_version,renderer_fingerprint,timeline_created_by,timeline_created_at,kind,status,progress_percent,retry_of,retry_idempotency_key,retry_request_hash,created_by_id,created_by_kind,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, job.OrganizationID, job.ProjectID, job.ID, job.EditTaskID, job.Timeline.Version, job.Timeline.Schema(), payload, job.Timeline.ContentHash, job.Timeline.CompilerCompatibility, job.RendererFingerprint, job.Timeline.CreatedBy, job.Timeline.CreatedAt, job.Kind, job.Status, job.ProgressPercent, sql.NullString{String: job.RetryOf, Valid: job.RetryOf != ""}, sql.NullString{String: string(job.RetryIdempotencyKey), Valid: job.RetryIdempotencyKey != ""}, sql.NullString{String: job.RetryRequestHash, Valid: job.RetryRequestHash != ""}, job.CreatedBy.ID, job.CreatedBy.Kind, job.CreatedAt, job.UpdatedAt)
	if err != nil {
		_ = tx.Rollback()
		if job.RetryIdempotencyKey != "" {
			existing, getErr := r.GetEditingRenderByRetryKey(ctx, job.OrganizationID, job.ProjectID, job.RetryIdempotencyKey)
			if getErr == nil && existing.RetryRequestHash == job.RetryRequestHash {
				if getErr = ensureInitialRenderObservability(ctx, r.DB, ProductionSourceEditingRender, existing.OrganizationID, existing.ProjectID, existing.ID, existing.CreatedAt); getErr != nil {
					return EditingRenderJob{}, getErr
				}
				existing.ProductionUsage, existing.ProductionEvents, getErr = r.loadRenderObservability(ctx, ProductionSourceEditingRender, existing.OrganizationID, existing.ProjectID, existing.ID)
				if getErr != nil {
					return EditingRenderJob{}, getErr
				}
				return existing, nil
			}
		}
		return EditingRenderJob{}, err
	}
	if err = ensureInitialRenderObservability(ctx, tx, ProductionSourceEditingRender, job.OrganizationID, job.ProjectID, job.ID, job.CreatedAt); err != nil {
		return EditingRenderJob{}, err
	}
	if err = tx.Commit(); err != nil {
		return EditingRenderJob{}, err
	}
	return job, nil
}

const editingRenderSelect = `SELECT edit_render_job_id,organization_id,project_id,edit_task_id,timeline_version,timeline_schema_version,timeline_payload,timeline_hash,compiler_version,renderer_fingerprint,timeline_created_by,timeline_created_at,kind,status,progress_percent,COALESCE(output_asset_id,''),COALESCE(output_asset_version,0),COALESCE(error_code,''),COALESCE(error_message,''),COALESCE(retry_of,''),COALESCE(retry_idempotency_key,''),COALESCE(retry_request_hash,''),created_by_id,created_by_kind,created_at,updated_at FROM creative_edit_render_jobs`

func (r MySQLRepository) GetEditingRender(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string) (EditingRenderJob, error) {
	job, err := scanEditingRender(r.DB.QueryRowContext(ctx, editingRenderSelect+` WHERE organization_id=? AND project_id=? AND edit_render_job_id=?`, org, project, id))
	return r.attachEditingRenderObservability(ctx, job, err)
}
func (r MySQLRepository) GetEditingRenderByRetryKey(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, key contract.IdempotencyKey) (EditingRenderJob, error) {
	job, err := scanEditingRender(r.DB.QueryRowContext(ctx, editingRenderSelect+` WHERE organization_id=? AND project_id=? AND retry_idempotency_key=?`, org, project, key))
	return r.attachEditingRenderObservability(ctx, job, err)
}
func (r MySQLRepository) FindReusableEditingRender(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, fingerprint string, kind EditingRenderKind) (EditingRenderJob, error) {
	job, err := scanEditingRender(r.DB.QueryRowContext(ctx, editingRenderSelect+` WHERE organization_id=? AND project_id=? AND renderer_fingerprint=? AND kind=? AND status='succeeded' AND output_asset_id IS NOT NULL ORDER BY updated_at DESC LIMIT 1`, org, project, fingerprint, kind))
	return r.attachEditingRenderObservability(ctx, job, err)
}
func (r MySQLRepository) MarkEditingRenderRunning(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string, now time.Time) (EditingRenderJob, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return EditingRenderJob{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE creative_edit_render_jobs SET status='running',error_code=NULL,error_message=NULL,updated_at=? WHERE organization_id=? AND project_id=? AND edit_render_job_id=? AND status='queued'`, now, org, project, id)
	if err != nil {
		return EditingRenderJob{}, err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return EditingRenderJob{}, ErrInvalidState
	}
	if err = appendRenderLifecycleEvent(ctx, tx, ProductionSourceEditingRender, org, project, id, string(EditingRenderRunning), "", now); err != nil {
		return EditingRenderJob{}, err
	}
	if err = tx.Commit(); err != nil {
		return EditingRenderJob{}, err
	}
	return r.GetEditingRender(ctx, org, project, id)
}
func (r MySQLRepository) UpdateEditingRenderProgress(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string, progress int, now time.Time) error {
	_, err := r.DB.ExecContext(ctx, `UPDATE creative_edit_render_jobs SET progress_percent=GREATEST(progress_percent,?),updated_at=? WHERE organization_id=? AND project_id=? AND edit_render_job_id=? AND status='running'`, progress, now, org, project, id)
	return err
}
func (r MySQLRepository) CompleteEditingRender(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string, ref contract.ProjectAssetRef, now time.Time) error {
	return r.updateEditingRenderLifecycle(ctx, org, project, id, string(EditingRenderSucceeded), "", now,
		`UPDATE creative_edit_render_jobs SET status='succeeded',progress_percent=100,output_asset_id=?,output_asset_version=?,updated_at=? WHERE organization_id=? AND project_id=? AND edit_render_job_id=? AND status='running'`,
		ref.AssetVersion.AssetID, ref.AssetVersion.Version, now, org, project, id)
}
func (r MySQLRepository) FailEditingRender(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id, code, message string, now time.Time) error {
	return r.updateEditingRenderLifecycle(ctx, org, project, id, string(EditingRenderFailed), code, now,
		`UPDATE creative_edit_render_jobs SET status='failed',error_code=?,error_message=?,updated_at=? WHERE organization_id=? AND project_id=? AND edit_render_job_id=? AND status IN ('queued','running')`,
		code, message, now, org, project, id)
}
func (r MySQLRepository) CancelEditingRender(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string, now time.Time) (EditingRenderJob, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return EditingRenderJob{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE creative_edit_render_jobs SET status='cancelled',updated_at=? WHERE organization_id=? AND project_id=? AND edit_render_job_id=? AND status IN ('queued','running')`, now, org, project, id)
	if err != nil {
		return EditingRenderJob{}, err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return EditingRenderJob{}, ErrInvalidState
	}
	if err = appendRenderLifecycleEvent(ctx, tx, ProductionSourceEditingRender, org, project, id, string(EditingRenderCancelled), "", now); err != nil {
		return EditingRenderJob{}, err
	}
	if err = tx.Commit(); err != nil {
		return EditingRenderJob{}, err
	}
	return r.GetEditingRender(ctx, org, project, id)
}

func (r MySQLRepository) attachEditingRenderObservability(ctx context.Context, job EditingRenderJob, err error) (EditingRenderJob, error) {
	if err != nil {
		return EditingRenderJob{}, err
	}
	job.ProductionUsage, job.ProductionEvents, err = r.loadRenderObservability(ctx, ProductionSourceEditingRender, job.OrganizationID, job.ProjectID, job.ID)
	return job, err
}

func (r MySQLRepository) updateEditingRenderLifecycle(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id, status, errorCode string, now time.Time, query string, args ...any) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return ErrInvalidState
	}
	if err = appendRenderLifecycleEvent(ctx, tx, ProductionSourceEditingRender, org, project, id, status, errorCode, now); err != nil {
		return err
	}
	return tx.Commit()
}
func scanEditingRender(row *sql.Row) (EditingRenderJob, error) {
	var j EditingRenderJob
	var payload []byte
	var assetID string
	var assetVersion int64
	err := row.Scan(&j.ID, &j.OrganizationID, &j.ProjectID, &j.EditTaskID, &j.Timeline.Version, &j.Timeline.SchemaVersion, &payload, &j.Timeline.ContentHash, &j.Timeline.CompilerCompatibility, &j.RendererFingerprint, &j.Timeline.CreatedBy, &j.Timeline.CreatedAt, &j.Kind, &j.Status, &j.ProgressPercent, &assetID, &assetVersion, &j.ErrorCode, &j.ErrorMessage, &j.RetryOf, &j.RetryIdempotencyKey, &j.RetryRequestHash, &j.CreatedBy.ID, &j.CreatedBy.Kind, &j.CreatedAt, &j.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return EditingRenderJob{}, ErrNotFound
	}
	if err != nil {
		return EditingRenderJob{}, err
	}
	if j.Timeline.Schema() == EditingTimelineSchemaV2 {
		var v2 EditingTimelineV2
		if err = json.Unmarshal(payload, &v2); err != nil {
			return EditingRenderJob{}, err
		}
		j.Timeline.TimelineV2 = &v2
	} else if err = json.Unmarshal(payload, &j.Timeline.Timeline); err != nil {
		return EditingRenderJob{}, err
	}
	if assetID != "" {
		j.OutputAsset = &contract.ProjectAssetRef{ProjectID: j.ProjectID, AssetVersion: contract.AssetVersionRef{AssetID: contract.AssetID(assetID), Version: assetVersion}}
	}
	if err = j.Validate(); err != nil {
		return EditingRenderJob{}, err
	}
	return j, nil
}
