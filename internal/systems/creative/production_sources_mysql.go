package creative

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/shikanon/cookies/internal/platform/contract"
)

func (r MySQLRepository) ListProductionRenderJobs(ctx context.Context, scope ProductionSourceScope) ([]RenderJob, bool, error) {
	if r.DB == nil {
		return nil, false, fmt.Errorf("creative repository database is required")
	}
	if scope.MediaKind != "" && scope.MediaKind != ProductionMediaRender {
		return []RenderJob{}, false, nil
	}
	query := creativeRenderSelect + ` WHERE organization_id=? AND project_id=?`
	args := []any{scope.OrganizationID, scope.ProjectID}
	query, args = appendRenderProductionFilters(query, args, scope, "task_id", "id", ProductionSourceCreativeRender)
	query += ` ORDER BY created_at DESC,id DESC LIMIT ?`
	args = append(args, scope.Limit+1)
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]RenderJob, 0, scope.Limit+1)
	for rows.Next() {
		job, scanErr := scanRenderJob(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		items = append(items, job)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if err := rows.Close(); err != nil {
		return nil, false, err
	}
	more := len(items) > scope.Limit
	if more {
		items = items[:scope.Limit]
	}
	for index := range items {
		items[index].ProductionUsage, items[index].ProductionEvents, err = r.loadRenderObservability(ctx, ProductionSourceCreativeRender, items[index].OrganizationID, items[index].ProjectID, items[index].ID)
		if err != nil {
			return nil, false, err
		}
	}
	return items, more, nil
}

func (r MySQLRepository) ListProductionEditingRenderJobs(ctx context.Context, scope ProductionSourceScope) ([]EditingRenderJob, bool, error) {
	if r.DB == nil {
		return nil, false, fmt.Errorf("creative repository database is required")
	}
	if scope.MediaKind != "" && scope.MediaKind != ProductionMediaRender {
		return []EditingRenderJob{}, false, nil
	}
	query := editingRenderSelect + ` WHERE organization_id=? AND project_id=?`
	args := []any{scope.OrganizationID, scope.ProjectID}
	query, args = appendRenderProductionFilters(query, args, scope, "edit_task_id", "edit_render_job_id", ProductionSourceEditingRender)
	query += ` ORDER BY created_at DESC,edit_render_job_id DESC LIMIT ?`
	args = append(args, scope.Limit+1)
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]EditingRenderJob, 0, scope.Limit+1)
	for rows.Next() {
		job, scanErr := scanProductionEditingRender(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		items = append(items, job)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if err := rows.Close(); err != nil {
		return nil, false, err
	}
	more := len(items) > scope.Limit
	if more {
		items = items[:scope.Limit]
	}
	for index := range items {
		items[index].ProductionUsage, items[index].ProductionEvents, err = r.loadRenderObservability(ctx, ProductionSourceEditingRender, items[index].OrganizationID, items[index].ProjectID, items[index].ID)
		if err != nil {
			return nil, false, err
		}
	}
	return items, more, nil
}

func appendRenderProductionFilters(query string, args []any, scope ProductionSourceScope, taskColumn, idColumn string, source ProductionRunSourceKind) (string, []any) {
	if len(scope.Statuses) > 0 {
		values := renderStatusesForProduction(scope.Statuses)
		if len(values) == 0 {
			query += ` AND 1=0`
		} else {
			query += ` AND status IN (` + strings.TrimSuffix(strings.Repeat("?,", len(values)), ",") + `)`
			for _, value := range values {
				args = append(args, value)
			}
		}
	}
	if scope.SourceTaskID != "" {
		query += ` AND ` + taskColumn + `=?`
		args = append(args, scope.SourceTaskID)
	}
	if scope.CreatedAfter != nil {
		query += ` AND created_at>=?`
		args = append(args, scope.CreatedAfter.UTC())
	}
	if scope.CreatedBefore != nil {
		query += ` AND created_at<?`
		args = append(args, scope.CreatedBefore.UTC())
	}
	if scope.BeforeCreated != nil {
		query += ` AND (created_at<? OR (created_at=? AND ` + idColumn + `<?))`
		args = append(args, scope.BeforeCreated.UTC(), scope.BeforeCreated.UTC(), nativeIDFromWatermark(scope.BeforeKey, source))
	}
	if value := strings.TrimSpace(scope.Query); value != "" {
		query += ` AND (` + idColumn + ` LIKE ? OR error_code=?)`
		args = append(args, "%"+value+"%", value)
	}
	return query, args
}

func renderStatusesForProduction(statuses []ProductionStatus) []string {
	result := []string{}
	for _, status := range statuses {
		switch status {
		case ProductionQueued, ProductionRunning, ProductionSucceeded, ProductionFailed, ProductionCancelled:
			result = append(result, string(status))
		}
	}
	return result
}

func scanProductionEditingRender(row rowScanner) (EditingRenderJob, error) {
	var job EditingRenderJob
	var payload []byte
	var assetID string
	var assetVersion int64
	err := row.Scan(&job.ID, &job.OrganizationID, &job.ProjectID, &job.EditTaskID, &job.Timeline.Version, &job.Timeline.SchemaVersion, &payload, &job.Timeline.ContentHash, &job.Timeline.CompilerCompatibility, &job.RendererFingerprint, &job.Timeline.CreatedBy, &job.Timeline.CreatedAt, &job.Kind, &job.Status, &job.ProgressPercent, &assetID, &assetVersion, &job.ErrorCode, &job.ErrorMessage, &job.RetryOf, &job.RetryIdempotencyKey, &job.RetryRequestHash, &job.CreatedBy.ID, &job.CreatedBy.Kind, &job.CreatedAt, &job.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return EditingRenderJob{}, ErrNotFound
	}
	if err != nil {
		return EditingRenderJob{}, err
	}
	if job.Timeline.Schema() == EditingTimelineSchemaV2 {
		var timeline EditingTimelineV2
		if err := json.Unmarshal(payload, &timeline); err != nil {
			return EditingRenderJob{}, err
		}
		job.Timeline.TimelineV2 = &timeline
	} else if err := json.Unmarshal(payload, &job.Timeline.Timeline); err != nil {
		return EditingRenderJob{}, err
	}
	if assetID != "" {
		job.OutputAsset = &contract.ProjectAssetRef{ProjectID: job.ProjectID, AssetVersion: contract.AssetVersionRef{AssetID: contract.AssetID(assetID), Version: assetVersion}}
	}
	return job, nil
}
