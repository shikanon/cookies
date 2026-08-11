package creative

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type renderObservabilityExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func ensureInitialRenderObservability(ctx context.Context, executor renderObservabilityExecutor, source ProductionRunSourceKind, organizationID contract.OrganizationID, projectID contract.ProjectID, jobID string, createdAt time.Time) error {
	usageTable, eventTable, err := renderObservabilityTables(source)
	if err != nil {
		return err
	}
	reason := renderOwnerLabel(source) + " actual cost is not metered."
	if _, err = executor.ExecContext(ctx, fmt.Sprintf(`INSERT IGNORE INTO %s
		(render_job_id,organization_id,project_id,currency,actual_cost_minor,unavailable_reason,measured_at)
		VALUES (?,?,?,'CNY',NULL,?,?)`, usageTable), jobID, organizationID, projectID, reason, createdAt); err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, fmt.Sprintf(`INSERT IGNORE INTO %s
		(render_job_id,organization_id,project_id,ordinal,stage,safe_message,error_code,occurred_at)
		VALUES (?,?,?,1,'queued',?,NULL,?)`, eventTable), jobID, organizationID, projectID, renderOwnerLabel(source)+" queued.", createdAt)
	return err
}

func appendRenderLifecycleEvent(ctx context.Context, executor renderObservabilityExecutor, source ProductionRunSourceKind, organizationID contract.OrganizationID, projectID contract.ProjectID, jobID, status, errorCode string, occurredAt time.Time) error {
	_, eventTable, err := renderObservabilityTables(source)
	if err != nil {
		return err
	}
	stage := safeRenderToken(status, 64)
	if stage == "" {
		return fmt.Errorf("creative render lifecycle stage is invalid")
	}
	code := safeRenderToken(errorCode, 128)
	_, err = executor.ExecContext(ctx, fmt.Sprintf(`INSERT IGNORE INTO %s
		(render_job_id,organization_id,project_id,ordinal,stage,safe_message,error_code,occurred_at)
		SELECT ?,?,?,COALESCE(MAX(ordinal),0)+1,?,?,?,? FROM %s WHERE render_job_id=? AND organization_id=? AND project_id=?`, eventTable, eventTable),
		jobID, organizationID, projectID, stage, renderLifecycleMessage(source, stage), sql.NullString{String: code, Valid: code != ""}, occurredAt,
		jobID, organizationID, projectID)
	return err
}

func (r MySQLRepository) loadRenderObservability(ctx context.Context, source ProductionRunSourceKind, organizationID contract.OrganizationID, projectID contract.ProjectID, jobID string) (*RenderUsage, []RenderEvent, error) {
	usageTable, eventTable, err := renderObservabilityTables(source)
	if err != nil {
		return nil, nil, err
	}
	var usage RenderUsage
	var amount sql.NullInt64
	var reason sql.NullString
	err = r.DB.QueryRowContext(ctx, fmt.Sprintf(`SELECT currency,actual_cost_minor,unavailable_reason,measured_at
		FROM %s WHERE render_job_id=? AND organization_id=? AND project_id=?`, usageTable), jobID, organizationID, projectID).
		Scan(&usage.Currency, &amount, &reason, &usage.MeasuredAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, err
	}
	var usageResult *RenderUsage
	if err == nil {
		if amount.Valid {
			usage.ActualCostMinor = &amount.Int64
		}
		if reason.Valid {
			usage.UnavailableReason = &reason.String
		}
		usageResult = &usage
	}
	rows, err := r.DB.QueryContext(ctx, fmt.Sprintf(`SELECT ordinal,stage,safe_message,COALESCE(error_code,''),occurred_at
		FROM %s WHERE render_job_id=? AND organization_id=? AND project_id=? ORDER BY ordinal`, eventTable), jobID, organizationID, projectID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	events := []RenderEvent{}
	for rows.Next() {
		var event RenderEvent
		if err := rows.Scan(&event.Ordinal, &event.Stage, &event.SafeMessage, &event.ErrorCode, &event.OccurredAt); err != nil {
			return nil, nil, err
		}
		events = append(events, event)
	}
	return usageResult, events, rows.Err()
}

// RecordRenderUsage stores an owner-reported actual cost or an explicit
// unavailable fact. Production Center only reads this record and never writes it.
func (r MySQLRepository) RecordRenderUsage(ctx context.Context, source ProductionRunSourceKind, organizationID contract.OrganizationID, projectID contract.ProjectID, jobID string, usage RenderUsage) error {
	if err := usage.Validate(); err != nil {
		return err
	}
	usageTable, _, err := renderObservabilityTables(source)
	if err != nil {
		return err
	}
	result, err := r.DB.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET currency=?,actual_cost_minor=?,unavailable_reason=?,measured_at=?
		WHERE render_job_id=? AND organization_id=? AND project_id=?`, usageTable), usage.Currency,
		sql.NullInt64{Int64: valueOrZero(usage.ActualCostMinor), Valid: usage.ActualCostMinor != nil},
		sql.NullString{String: stringOrEmpty(usage.UnavailableReason), Valid: usage.UnavailableReason != nil}, usage.MeasuredAt,
		jobID, organizationID, projectID)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return ErrNotFound
	}
	return nil
}

func renderObservabilityTables(source ProductionRunSourceKind) (string, string, error) {
	switch source {
	case ProductionSourceCreativeRender:
		return "creative_render_job_usage", "creative_render_job_events", nil
	case ProductionSourceEditingRender:
		return "creative_edit_render_job_usage", "creative_edit_render_job_events", nil
	default:
		return "", "", fmt.Errorf("unsupported creative render owner %q", source)
	}
}

func renderOwnerLabel(source ProductionRunSourceKind) string {
	if source == ProductionSourceEditingRender {
		return "Editing render"
	}
	return "Creative render"
}

func renderLifecycleMessage(source ProductionRunSourceKind, stage string) string {
	owner := renderOwnerLabel(source)
	switch stage {
	case "running":
		return owner + " started."
	case "succeeded":
		return owner + " completed."
	case "failed":
		return owner + " failed."
	case "cancelled":
		return owner + " was cancelled."
	default:
		return owner + " state changed."
	}
}

func valueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
