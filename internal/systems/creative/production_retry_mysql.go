package creative

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (r MySQLRepository) Claim(ctx context.Context, claim ProductionRetryClaim) (*ProductionRunRef, error) {
	if r.DB == nil {
		return nil, fmt.Errorf("creative repository database is required")
	}
	result, err := r.DB.ExecContext(ctx, `INSERT IGNORE INTO creative_production_retry_commands
		(organization_id,project_id,idempotency_key,request_hash,previous_source,previous_run_id,status,actor_id,actor_kind,created_at,updated_at)
		VALUES (?,?,?,?,?,?,'pending',?,?,?,?)`, claim.OrganizationID, claim.ProjectID, claim.IdempotencyKey, claim.RequestHash,
		claim.PreviousRun.Source, claim.PreviousRun.ID, claim.Actor.ID, claim.Actor.Kind, claim.CreatedAt, claim.CreatedAt)
	if err != nil {
		return nil, err
	}
	inserted, _ := result.RowsAffected()
	if inserted == 1 {
		return nil, nil
	}
	var requestHash, status, source, runID string
	err = r.DB.QueryRowContext(ctx, `SELECT request_hash,status,COALESCE(new_source,''),COALESCE(new_run_id,'')
		FROM creative_production_retry_commands WHERE organization_id=? AND project_id=? AND idempotency_key=?`,
		claim.OrganizationID, claim.ProjectID, claim.IdempotencyKey).Scan(&requestHash, &status, &source, &runID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("production retry claim disappeared")
	}
	if err != nil {
		return nil, err
	}
	if requestHash != claim.RequestHash {
		return nil, ErrProductionIdempotencyConflict
	}
	if status == "completed" && source != "" && runID != "" {
		ref := ProductionRunRef{Source: ProductionRunSourceKind(source), ID: runID}
		return &ref, nil
	}
	return nil, nil
}

func (r MySQLRepository) Complete(ctx context.Context, claim ProductionRetryClaim, result ProductionRunRef) error {
	now := time.Now().UTC()
	changed, err := r.DB.ExecContext(ctx, `UPDATE creative_production_retry_commands
		SET status='completed',new_source=?,new_run_id=?,completed_at=?,updated_at=?
		WHERE organization_id=? AND project_id=? AND idempotency_key=? AND request_hash=? AND status='pending'`,
		result.Source, result.ID, now, now, claim.OrganizationID, claim.ProjectID, claim.IdempotencyKey, claim.RequestHash)
	if err != nil {
		return err
	}
	rows, _ := changed.RowsAffected()
	if rows != 1 {
		return ErrProductionIdempotencyConflict
	}
	return nil
}
