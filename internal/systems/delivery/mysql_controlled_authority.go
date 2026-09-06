package delivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/platform/contract"
)

func (r MySQLRepository) CreateControlledChangeSet(ctx context.Context, value ControlledChangeSet) (ControlledChangeSet, bool, error) {
	binding, err := json.Marshal(value.Binding)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	_, err = r.DB.ExecContext(ctx, `INSERT INTO delivery_controlled_change_sets (id,organization_id,project_id,binding_json,action,budget_limit_minor,currency,status,canonical_hash,version,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.OrganizationID, value.ProjectID, binding, value.Action, value.BudgetLimitMinor, value.Currency, value.Status, value.CanonicalHash, value.Version, value.CreatedBy, value.CreatedAt, value.UpdatedAt)
	if err == nil {
		return value, false, nil
	}
	existing, getErr := r.getControlledChangeSetByHash(ctx, value.OrganizationID, value.ProjectID, value.CanonicalHash)
	if getErr != nil {
		return ControlledChangeSet{}, false, err
	}
	return existing, true, nil
}

func (r MySQLRepository) GetControlledChangeSet(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string) (ControlledChangeSet, error) {
	return scanControlledChangeSet(r.DB.QueryRowContext(ctx, controlledChangeSetSelect+` WHERE organization_id=? AND project_id=? AND id=?`, org, project, id))
}

func (r MySQLRepository) GetControlledChangeSetByObjectFingerprint(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, fingerprint string) (ControlledChangeSet, error) {
	value, err := scanControlledChangeSet(r.DB.QueryRowContext(ctx, controlledChangeSetSelect+` WHERE organization_id=? AND project_id=? AND object_fingerprint=?`, org, project, fingerprint))
	if errors.Is(err, sql.ErrNoRows) {
		return ControlledChangeSet{}, ErrNotFound
	}
	return value, err
}

func (r MySQLRepository) getControlledChangeSetByHash(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, hash string) (ControlledChangeSet, error) {
	return scanControlledChangeSet(r.DB.QueryRowContext(ctx, controlledChangeSetSelect+` WHERE organization_id=? AND project_id=? AND canonical_hash=?`, org, project, hash))
}

func (r MySQLRepository) ApproveControlledChangeSet(ctx context.Context, change ControlledChangeSet, approval RemoteWriteApproval) (ControlledChangeSet, RemoteWriteApproval, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return ControlledChangeSet{}, RemoteWriteApproval{}, err
	}
	defer tx.Rollback()
	var status string
	var version int64
	var hash string
	err = tx.QueryRowContext(ctx, `SELECT status,version,canonical_hash FROM delivery_controlled_change_sets WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, change.OrganizationID, change.ProjectID, change.ID).Scan(&status, &version, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return ControlledChangeSet{}, RemoteWriteApproval{}, ErrNotFound
	}
	if err != nil {
		return ControlledChangeSet{}, RemoteWriteApproval{}, err
	}
	if status != string(ControlledChangeSetReady) {
		return ControlledChangeSet{}, RemoteWriteApproval{}, ErrInvalidState
	}
	if version != change.Version {
		return ControlledChangeSet{}, RemoteWriteApproval{}, ErrVersionConflict
	}
	if hash != approval.ControlledChangeSetHash {
		return ControlledChangeSet{}, RemoteWriteApproval{}, ErrApprovalContentMismatch
	}
	binding, err := json.Marshal(approval.Binding)
	if err != nil {
		return ControlledChangeSet{}, RemoteWriteApproval{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO delivery_remote_write_approvals (id,organization_id,project_id,controlled_change_set_id,controlled_change_set_hash,binding_json,action,scope,budget_limit_minor,currency,action_hash,approved_by,approved_at,expires_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, approval.ID, approval.OrganizationID, approval.ProjectID, approval.ControlledChangeSetID, approval.ControlledChangeSetHash, binding, approval.Action, approval.Scope, approval.BudgetLimitMinor, approval.Currency, approval.ActionHash, approval.ApprovedBy, approval.ApprovedAt, approval.ExpiresAt)
	if err != nil {
		return ControlledChangeSet{}, RemoteWriteApproval{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE delivery_controlled_change_sets SET status='approved',version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND version=?`, approval.ApprovedAt, change.OrganizationID, change.ProjectID, change.ID, change.Version)
	if err != nil {
		return ControlledChangeSet{}, RemoteWriteApproval{}, err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return ControlledChangeSet{}, RemoteWriteApproval{}, ErrVersionConflict
	}
	if err := tx.Commit(); err != nil {
		return ControlledChangeSet{}, RemoteWriteApproval{}, err
	}
	change.Status = ControlledChangeSetApproved
	change.Version++
	change.UpdatedAt = approval.ApprovedAt
	return change, approval, nil
}

func (r MySQLRepository) GetRemoteWriteApproval(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, changeSetID string) (RemoteWriteApproval, error) {
	var v RemoteWriteApproval
	var binding []byte
	err := r.DB.QueryRowContext(ctx, `SELECT id,organization_id,project_id,controlled_change_set_id,controlled_change_set_hash,binding_json,action,scope,budget_limit_minor,currency,action_hash,approved_by,approved_at,expires_at FROM delivery_remote_write_approvals WHERE organization_id=? AND project_id=? AND controlled_change_set_id=?`, org, project, changeSetID).Scan(&v.ID, &v.OrganizationID, &v.ProjectID, &v.ControlledChangeSetID, &v.ControlledChangeSetHash, &binding, &v.Action, &v.Scope, &v.BudgetLimitMinor, &v.Currency, &v.ActionHash, &v.ApprovedBy, &v.ApprovedAt, &v.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return RemoteWriteApproval{}, ErrNotFound
	}
	if err != nil {
		return RemoteWriteApproval{}, err
	}
	if err := json.Unmarshal(binding, &v.Binding); err != nil {
		return RemoteWriteApproval{}, fmt.Errorf("decode controlled approval binding: %w", err)
	}
	v.SchemaVersion = RemoteWriteApprovalSchemaV1
	return v, nil
}

func (r MySQLRepository) CreateControlledExecution(ctx context.Context, value ControlledExecution) (ControlledExecution, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return ControlledExecution{}, err
	}
	defer tx.Rollback()
	var status, approvalID string
	var bindingJSON []byte
	var expiresAt sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT c.status,c.binding_json,a.id,a.expires_at FROM delivery_controlled_change_sets c JOIN delivery_remote_write_approvals a ON a.organization_id=c.organization_id AND a.project_id=c.project_id AND a.controlled_change_set_id=c.id WHERE c.organization_id=? AND c.project_id=? AND c.id=? FOR UPDATE`, value.OrganizationID, value.ProjectID, value.ControlledChangeSetID).Scan(&status, &bindingJSON, &approvalID, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ControlledExecution{}, ErrNotFound
	}
	if err != nil {
		return ControlledExecution{}, err
	}
	if status != string(ControlledChangeSetApproved) || approvalID != value.RemoteWriteApprovalID || !expiresAt.Valid || !value.CreatedAt.Before(expiresAt.Time) {
		return ControlledExecution{}, ErrApprovalExpired
	}
	var binding ControlledAuthorityBinding
	if err := json.Unmarshal(bindingJSON, &binding); err != nil {
		return ControlledExecution{}, fmt.Errorf("decode controlled execution binding: %w", err)
	}
	if binding.OperatorPrincipalID != "" && binding.OperatorPrincipalID != value.CreatedBy {
		return ControlledExecution{}, ErrApprovalContentMismatch
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO delivery_controlled_executions (id,organization_id,project_id,controlled_change_set_id,remote_write_approval_id,browser_rpa_run_id,status,version,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.OrganizationID, value.ProjectID, value.ControlledChangeSetID, value.RemoteWriteApprovalID, nullableString(value.BrowserRpaRunID), value.Status, value.Version, value.CreatedBy, value.CreatedAt, value.UpdatedAt)
	if err != nil {
		return ControlledExecution{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE delivery_controlled_change_sets SET status='executing',version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND status='approved'`, value.CreatedAt, value.OrganizationID, value.ProjectID, value.ControlledChangeSetID)
	if err != nil {
		return ControlledExecution{}, err
	}
	if err := tx.Commit(); err != nil {
		return ControlledExecution{}, err
	}
	return value, nil
}

func (r MySQLRepository) GetControlledExecution(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string) (ControlledExecution, error) {
	var v ControlledExecution
	err := r.DB.QueryRowContext(ctx, `SELECT id,organization_id,project_id,controlled_change_set_id,remote_write_approval_id,COALESCE(browser_rpa_run_id,''),status,version,created_by,created_at,updated_at FROM delivery_controlled_executions WHERE organization_id=? AND project_id=? AND id=?`, org, project, id).Scan(&v.ID, &v.OrganizationID, &v.ProjectID, &v.ControlledChangeSetID, &v.RemoteWriteApprovalID, &v.BrowserRpaRunID, &v.Status, &v.Version, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ControlledExecution{}, ErrNotFound
	}
	return v, err
}

func (r MySQLRepository) GetControlledExecutionByChangeSet(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, changeSetID string) (ControlledExecution, error) {
	var value ControlledExecution
	err := r.DB.QueryRowContext(ctx, `SELECT id,organization_id,project_id,controlled_change_set_id,remote_write_approval_id,COALESCE(browser_rpa_run_id,''),status,version,created_by,created_at,updated_at FROM delivery_controlled_executions WHERE organization_id=? AND project_id=? AND controlled_change_set_id=?`, org, project, changeSetID).Scan(&value.ID, &value.OrganizationID, &value.ProjectID, &value.ControlledChangeSetID, &value.RemoteWriteApprovalID, &value.BrowserRpaRunID, &value.Status, &value.Version, &value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ControlledExecution{}, ErrNotFound
	}
	return value, err
}

func (r MySQLRepository) AttachBrowserRpaRun(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string, expectedVersion int64, runID string, now time.Time) (ControlledExecution, error) {
	result, err := r.DB.ExecContext(ctx, `UPDATE delivery_controlled_executions SET browser_rpa_run_id=?,status='running',version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND version=? AND browser_rpa_run_id IS NULL AND status='pending'`, runID, now, org, project, id, expectedVersion)
	if err != nil {
		return ControlledExecution{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ControlledExecution{}, err
	}
	if affected != 1 {
		return ControlledExecution{}, ErrVersionConflict
	}
	return r.GetControlledExecution(ctx, org, project, id)
}

// ResetSafeFailedBrowserRpaExecution releases only a failed run whose evidence
// proves that the Runner did not cross the final-click boundary. The failed
// run and its evidence remain available for audit.
func (r MySQLRepository) ResetSafeFailedBrowserRpaExecution(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, executionID string, expectedVersion int64, runID string) (ControlledExecution, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return ControlledExecution{}, err
	}
	defer tx.Rollback()
	var executionStatus, executionRunID string
	var executionVersion int64
	if err := tx.QueryRowContext(ctx, `SELECT status,version,COALESCE(browser_rpa_run_id,'') FROM delivery_controlled_executions WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, org, project, executionID).Scan(&executionStatus, &executionVersion, &executionRunID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ControlledExecution{}, ErrNotFound
		}
		return ControlledExecution{}, err
	}
	if executionStatus != "running" || executionVersion != expectedVersion || executionRunID != runID {
		return ControlledExecution{}, ErrInvalidState
	}
	var runState, leaseID string
	var takeoverActive bool
	if err := tx.QueryRowContext(ctx, `SELECT state,COALESCE(lease_id,''),takeover_active FROM browser_rpa_runs WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, org, project, runID).Scan(&runState, &leaseID, &takeoverActive); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ControlledExecution{}, ErrNotFound
		}
		return ControlledExecution{}, err
	}
	if runState != "failed" || takeoverActive {
		return ControlledExecution{}, ErrInvalidState
	}
	if leaseID != "" {
		var activeLeaseCount int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM browser_rpa_session_leases WHERE organization_id=? AND project_id=? AND id=? AND run_id=? AND released_at IS NULL AND expires_at>NOW(6) AND heartbeat_deadline>NOW(6)`, org, project, leaseID, runID).Scan(&activeLeaseCount); err != nil {
			return ControlledExecution{}, err
		}
		if activeLeaseCount != 0 {
			return ControlledExecution{}, ErrInvalidState
		}
		if _, err := tx.ExecContext(ctx, `UPDATE browser_rpa_session_leases SET active_lock_key=NULL,released_at=COALESCE(released_at,NOW(6)),version=version+1 WHERE organization_id=? AND project_id=? AND id=? AND run_id=?`, org, project, leaseID, runID); err != nil {
			return ControlledExecution{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE browser_rpa_runs SET lease_id=NULL,version=version+1,updated_at=NOW(6) WHERE organization_id=? AND project_id=? AND id=? AND lease_id=?`, org, project, runID, leaseID); err != nil {
			return ControlledExecution{}, err
		}
	}
	var attemptCount, failedAttemptCount, consumedConfirmationCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM browser_rpa_controlled_action_attempts WHERE organization_id=? AND project_id=? AND run_id=?`, org, project, runID).Scan(&attemptCount); err != nil {
		return ControlledExecution{}, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM browser_rpa_controlled_action_attempts WHERE organization_id=? AND project_id=? AND run_id=? AND status='failed'`, org, project, runID).Scan(&failedAttemptCount); err != nil {
		return ControlledExecution{}, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM browser_rpa_final_confirmations WHERE organization_id=? AND project_id=? AND run_id=? AND consumed_at IS NOT NULL`, org, project, runID).Scan(&consumedConfirmationCount); err != nil {
		return ControlledExecution{}, err
	}
	var latestStepAction, latestStepStatus, latestStepBlockingReason string
	if err := tx.QueryRowContext(ctx, `SELECT action,status,COALESCE(blocking_reason,'') FROM browser_rpa_run_steps WHERE organization_id=? AND project_id=? AND run_id=? ORDER BY sequence_number DESC LIMIT 1`, org, project, runID).Scan(&latestStepAction, &latestStepStatus, &latestStepBlockingReason); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ControlledExecution{}, ErrInvalidState
		}
		return ControlledExecution{}, err
	}
	stagedPrepareFailure := safeStagedPrepareRetry(latestStepAction, latestStepStatus, latestStepBlockingReason)
	var confirmedNoEffectEvidenceCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM browser_rpa_evidence WHERE organization_id=? AND project_id=? AND run_id=? AND JSON_UNQUOTE(JSON_EXTRACT(evidence_json,'$.field_readback.reconciliation'))='not_found' AND ((JSON_UNQUOTE(JSON_EXTRACT(evidence_json,'$.field_readback.read_only_reconciliation'))='true' AND JSON_UNQUOTE(JSON_EXTRACT(evidence_json,'$.field_readback.platform_write_performed'))='false' AND JSON_UNQUOTE(JSON_EXTRACT(evidence_json,'$.field_readback.exact_name_matches'))='0') OR (JSON_UNQUOTE(JSON_EXTRACT(evidence_json,'$.field_readback.final_click_performed'))='true' AND JSON_UNQUOTE(JSON_EXTRACT(evidence_json,'$.field_readback.platform_write_request_observed'))='false'))`, org, project, runID).Scan(&confirmedNoEffectEvidenceCount); err != nil {
		return ControlledExecution{}, err
	}
	confirmedNoEffect := confirmedNoEffectEvidenceCount > 0 && ((latestStepAction == string(browserautomation.TakeoverListConfirmed) && latestStepStatus == "succeeded") || latestStepStatus == "failed")
	if !stagedPrepareFailure && !confirmedNoEffect && (attemptCount != 0 || consumedConfirmationCount != 0) {
		var noClickEvidenceCount, clickEvidenceCount int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM browser_rpa_evidence WHERE organization_id=? AND project_id=? AND run_id=? AND JSON_UNQUOTE(JSON_EXTRACT(evidence_json,'$.field_readback.final_click_performed'))='false'`, org, project, runID).Scan(&noClickEvidenceCount); err != nil {
			return ControlledExecution{}, err
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM browser_rpa_evidence WHERE organization_id=? AND project_id=? AND run_id=? AND JSON_UNQUOTE(JSON_EXTRACT(evidence_json,'$.field_readback.final_click_performed'))='true'`, org, project, runID).Scan(&clickEvidenceCount); err != nil {
			return ControlledExecution{}, err
		}
		if attemptCount == 0 || failedAttemptCount != attemptCount || noClickEvidenceCount == 0 || clickEvidenceCount != 0 {
			return ControlledExecution{}, ErrInvalidState
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE browser_rpa_final_confirmations SET invalidated_at=COALESCE(invalidated_at,NOW(6)),version=version+1 WHERE organization_id=? AND project_id=? AND run_id=? AND consumed_at IS NULL AND rejected_at IS NULL AND invalidated_at IS NULL`, org, project, runID); err != nil {
		return ControlledExecution{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE delivery_controlled_executions SET browser_rpa_run_id=NULL,status='pending',version=version+1,updated_at=NOW(6) WHERE organization_id=? AND project_id=? AND id=? AND version=? AND status='running' AND browser_rpa_run_id=?`, org, project, executionID, expectedVersion, runID)
	if err != nil {
		return ControlledExecution{}, err
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected != 1 {
		if affectedErr != nil {
			return ControlledExecution{}, affectedErr
		}
		return ControlledExecution{}, ErrVersionConflict
	}
	if err := tx.Commit(); err != nil {
		return ControlledExecution{}, err
	}
	return r.GetControlledExecution(ctx, org, project, executionID)
}

func safeStagedPrepareRetry(action, status, blockingReason string) bool {
	if action != "prepare_and_readback" || status != "failed" {
		return false
	}
	return blockingReason == string(browserautomation.BlockPageDrift) || blockingReason == string(browserautomation.BlockRunnerFailure)
}

func (r MySQLRepository) InvalidateCalibratedControlledChangeSet(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string, expectedVersion int64, now time.Time) (ControlledChangeSet, ControlledExecution, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return ControlledChangeSet{}, ControlledExecution{}, err
	}
	defer tx.Rollback()
	var changeStatus string
	var changeVersion int64
	var execution ControlledExecution
	err = tx.QueryRowContext(ctx, `SELECT c.status,c.version,e.id,e.organization_id,e.project_id,e.controlled_change_set_id,e.remote_write_approval_id,COALESCE(e.browser_rpa_run_id,''),e.status,e.version,e.created_by,e.created_at,e.updated_at FROM delivery_controlled_change_sets c JOIN delivery_controlled_executions e ON e.organization_id=c.organization_id AND e.project_id=c.project_id AND e.controlled_change_set_id=c.id JOIN delivery_remote_write_approvals a ON a.organization_id=e.organization_id AND a.project_id=e.project_id AND a.id=e.remote_write_approval_id WHERE c.organization_id=? AND c.project_id=? AND c.id=? FOR UPDATE`, org, project, id).Scan(&changeStatus, &changeVersion, &execution.ID, &execution.OrganizationID, &execution.ProjectID, &execution.ControlledChangeSetID, &execution.RemoteWriteApprovalID, &execution.BrowserRpaRunID, &execution.Status, &execution.Version, &execution.CreatedBy, &execution.CreatedAt, &execution.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ControlledChangeSet{}, ControlledExecution{}, ErrNotFound
	}
	if err != nil {
		return ControlledChangeSet{}, ControlledExecution{}, err
	}
	if changeStatus != string(ControlledChangeSetExecuting) || changeVersion != expectedVersion || execution.Status != "running" || execution.BrowserRpaRunID == "" {
		return ControlledChangeSet{}, ControlledExecution{}, ErrInvalidState
	}
	var runState, leaseID string
	var takeoverActive bool
	if err := tx.QueryRowContext(ctx, `SELECT state,COALESCE(lease_id,''),takeover_active FROM browser_rpa_runs WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, org, project, execution.BrowserRpaRunID).Scan(&runState, &leaseID, &takeoverActive); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ControlledChangeSet{}, ControlledExecution{}, ErrNotFound
		}
		return ControlledChangeSet{}, ControlledExecution{}, err
	}
	if runState != "cancelled" || leaseID != "" || takeoverActive {
		return ControlledChangeSet{}, ControlledExecution{}, ErrInvalidState
	}
	var attemptCount, consumedConfirmationCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM browser_rpa_controlled_action_attempts WHERE organization_id=? AND project_id=? AND run_id=?`, org, project, execution.BrowserRpaRunID).Scan(&attemptCount); err != nil {
		return ControlledChangeSet{}, ControlledExecution{}, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM browser_rpa_final_confirmations WHERE organization_id=? AND project_id=? AND run_id=? AND consumed_at IS NOT NULL`, org, project, execution.BrowserRpaRunID).Scan(&consumedConfirmationCount); err != nil {
		return ControlledChangeSet{}, ControlledExecution{}, err
	}
	if attemptCount != 0 || consumedConfirmationCount != 0 {
		return ControlledChangeSet{}, ControlledExecution{}, ErrInvalidState
	}
	if _, err := tx.ExecContext(ctx, `UPDATE browser_rpa_final_confirmations SET invalidated_at=COALESCE(invalidated_at,?),version=version+1 WHERE organization_id=? AND project_id=? AND run_id=? AND consumed_at IS NULL AND rejected_at IS NULL AND invalidated_at IS NULL`, now, org, project, execution.BrowserRpaRunID); err != nil {
		return ControlledChangeSet{}, ControlledExecution{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE delivery_controlled_executions SET status='cancelled',version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND status='running' AND version=?`, now, org, project, execution.ID, execution.Version)
	if err != nil {
		return ControlledChangeSet{}, ControlledExecution{}, err
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected != 1 {
		if affectedErr != nil {
			return ControlledChangeSet{}, ControlledExecution{}, affectedErr
		}
		return ControlledChangeSet{}, ControlledExecution{}, ErrVersionConflict
	}
	result, err = tx.ExecContext(ctx, `UPDATE delivery_controlled_change_sets SET status='invalidated',version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND status='executing' AND version=?`, now, org, project, id, expectedVersion)
	if err != nil {
		return ControlledChangeSet{}, ControlledExecution{}, err
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected != 1 {
		if affectedErr != nil {
			return ControlledChangeSet{}, ControlledExecution{}, affectedErr
		}
		return ControlledChangeSet{}, ControlledExecution{}, ErrVersionConflict
	}
	if err := tx.Commit(); err != nil {
		return ControlledChangeSet{}, ControlledExecution{}, err
	}
	change, err := r.GetControlledChangeSet(ctx, org, project, id)
	if err != nil {
		return ControlledChangeSet{}, ControlledExecution{}, err
	}
	execution, err = r.GetControlledExecution(ctx, org, project, execution.ID)
	return change, execution, err
}

func (r MySQLRepository) CreatePlatformEntityMapping(ctx context.Context, value PlatformEntityMapping) (PlatformEntityMapping, error) {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO delivery_platform_entity_mappings (id,organization_id,project_id,account_reference_id,plan_id,configuration_id,business_execution_id,browser_rpa_run_id,internal_object_kind,internal_object_id,platform_object_kind,platform_object_id,platform_status,current_state_action,current_state_hash,result_evidence_id,list_evidence_id,status,version,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.OrganizationID, value.ProjectID, value.AccountReferenceID, value.PlanID, value.ConfigurationID, value.BusinessExecutionID, value.BrowserRpaRunID, value.InternalObjectKind, value.InternalObjectID, value.PlatformObjectKind, nullableString(value.PlatformObjectID), nullableString(value.PlatformStatus), nullableString(string(value.CurrentStateAction)), nullableString(value.CurrentStateHash), nullableString(value.ResultEvidenceID), nullableString(value.ListEvidenceID), value.Status, value.Version, value.CreatedAt, value.UpdatedAt)
	if err != nil {
		return PlatformEntityMapping{}, err
	}
	return value, nil
}

func (r MySQLRepository) GetPlatformEntityMapping(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string) (PlatformEntityMapping, error) {
	return scanPlatformEntityMapping(r.DB.QueryRowContext(ctx, platformEntityMappingSelect+` WHERE organization_id=? AND project_id=? AND id=?`, org, project, id))
}

func (r MySQLRepository) GetPlatformEntityMappingByInternalObject(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, account, kind, internalID string) (PlatformEntityMapping, error) {
	return scanPlatformEntityMapping(r.DB.QueryRowContext(ctx, platformEntityMappingSelect+` WHERE organization_id=? AND project_id=? AND account_reference_id=? AND internal_object_kind=? AND internal_object_id=?`, org, project, account, kind, internalID))
}

func (r MySQLRepository) RebindSafePendingPlatformEntityMapping(ctx context.Context, request rebindPendingPlatformEntityMappingRequest) (PlatformEntityMapping, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return PlatformEntityMapping{}, err
	}
	defer tx.Rollback()

	mapping, err := scanPlatformEntityMapping(tx.QueryRowContext(ctx, platformEntityMappingSelect+` WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, request.OrganizationID, request.ProjectID, request.MappingID))
	if err != nil {
		return PlatformEntityMapping{}, err
	}
	if mapping.Version != request.ExpectedVersion || mapping.Status != PlatformEntityMappingPending || mapping.PlatformObjectID != "" || mapping.PlatformStatus != "" || mapping.ResultEvidenceID != "" || mapping.ListEvidenceID != "" {
		return PlatformEntityMapping{}, ErrInvalidState
	}
	if mapping.BusinessExecutionID == request.BusinessExecutionID && mapping.BrowserRpaRunID == request.BrowserRpaRunID {
		return mapping, nil
	}

	var oldExecutionStatus, oldChangeSetID, oldChangeStatus, oldRunState, oldLeaseID string
	var oldTakeoverActive bool
	err = tx.QueryRowContext(ctx, `SELECT e.status,e.controlled_change_set_id,c.status,r.state,COALESCE(r.lease_id,''),r.takeover_active
		FROM delivery_controlled_executions e
		JOIN delivery_controlled_change_sets c ON c.organization_id=e.organization_id AND c.project_id=e.project_id AND c.id=e.controlled_change_set_id
		JOIN browser_rpa_runs r ON r.organization_id=e.organization_id AND r.project_id=e.project_id AND r.id=?
		WHERE e.organization_id=? AND e.project_id=? AND e.id=? FOR UPDATE`, mapping.BrowserRpaRunID, request.OrganizationID, request.ProjectID, mapping.BusinessExecutionID).
		Scan(&oldExecutionStatus, &oldChangeSetID, &oldChangeStatus, &oldRunState, &oldLeaseID, &oldTakeoverActive)
	if errors.Is(err, sql.ErrNoRows) {
		return PlatformEntityMapping{}, ErrNotFound
	}
	if err != nil {
		return PlatformEntityMapping{}, err
	}

	var newExecutionStatus, newRunState string
	err = tx.QueryRowContext(ctx, `SELECT e.status,r.state FROM delivery_controlled_executions e
		JOIN browser_rpa_runs r ON r.organization_id=e.organization_id AND r.project_id=e.project_id AND r.id=?
		WHERE e.organization_id=? AND e.project_id=? AND e.id=? FOR UPDATE`, request.BrowserRpaRunID, request.OrganizationID, request.ProjectID, request.BusinessExecutionID).
		Scan(&newExecutionStatus, &newRunState)
	if errors.Is(err, sql.ErrNoRows) {
		return PlatformEntityMapping{}, ErrNotFound
	}
	if err != nil {
		return PlatformEntityMapping{}, err
	}
	if newExecutionStatus != "running" || newRunState != "queued" {
		return PlatformEntityMapping{}, ErrInvalidState
	}

	var stepCount, evidenceCount, attemptCount, failedAttemptCount, confirmationCount int
	err = tx.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM browser_rpa_run_steps WHERE organization_id=? AND project_id=? AND run_id=?),
		(SELECT COUNT(*) FROM browser_rpa_evidence WHERE organization_id=? AND project_id=? AND run_id=?),
		(SELECT COUNT(*) FROM browser_rpa_controlled_action_attempts WHERE organization_id=? AND project_id=? AND run_id=?),
		(SELECT COUNT(*) FROM browser_rpa_controlled_action_attempts WHERE organization_id=? AND project_id=? AND run_id=? AND status='failed'),
		(SELECT COUNT(*) FROM browser_rpa_final_confirmations WHERE organization_id=? AND project_id=? AND run_id=?)`,
		request.OrganizationID, request.ProjectID, mapping.BrowserRpaRunID,
		request.OrganizationID, request.ProjectID, mapping.BrowserRpaRunID,
		request.OrganizationID, request.ProjectID, mapping.BrowserRpaRunID,
		request.OrganizationID, request.ProjectID, mapping.BrowserRpaRunID,
		request.OrganizationID, request.ProjectID, mapping.BrowserRpaRunID).
		Scan(&stepCount, &evidenceCount, &attemptCount, &failedAttemptCount, &confirmationCount)
	if err != nil {
		return PlatformEntityMapping{}, err
	}
	oldLeaseInactive := oldLeaseID == ""
	if oldLeaseID != "" && (oldRunState == "failed" || oldRunState == "cancelled" || oldRunState == "awaiting_confirmation") {
		var inactiveCount int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM browser_rpa_session_leases WHERE organization_id=? AND project_id=? AND id=? AND run_id=? AND (released_at IS NOT NULL OR expires_at<=? OR heartbeat_deadline<=?)`, request.OrganizationID, request.ProjectID, oldLeaseID, mapping.BrowserRpaRunID, request.Now, request.Now).Scan(&inactiveCount); err != nil {
			return PlatformEntityMapping{}, err
		}
		oldLeaseInactive = inactiveCount == 1
		if oldLeaseInactive {
			if _, err := tx.ExecContext(ctx, `UPDATE browser_rpa_session_leases SET active_lock_key=NULL,released_at=COALESCE(released_at,?),version=version+1 WHERE organization_id=? AND project_id=? AND id=? AND run_id=? AND released_at IS NULL`, request.Now, request.OrganizationID, request.ProjectID, oldLeaseID, mapping.BrowserRpaRunID); err != nil {
				return PlatformEntityMapping{}, err
			}
		}
	}
	controlledActionSafe := attemptCount == 0 && confirmationCount == 0
	if oldRunState == "awaiting_confirmation" {
		var noClickEvidenceCount, clickEvidenceCount int
		if err := tx.QueryRowContext(ctx, `SELECT
			(SELECT COUNT(*) FROM browser_rpa_evidence WHERE organization_id=? AND project_id=? AND run_id=? AND JSON_UNQUOTE(JSON_EXTRACT(evidence_json,'$.field_readback.final_click_performed'))='false'),
			(SELECT COUNT(*) FROM browser_rpa_evidence WHERE organization_id=? AND project_id=? AND run_id=? AND JSON_UNQUOTE(JSON_EXTRACT(evidence_json,'$.field_readback.final_click_performed'))='true')`,
			request.OrganizationID, request.ProjectID, mapping.BrowserRpaRunID,
			request.OrganizationID, request.ProjectID, mapping.BrowserRpaRunID).
			Scan(&noClickEvidenceCount, &clickEvidenceCount); err != nil {
			return PlatformEntityMapping{}, err
		}
		controlledActionSafe = preparedRunCanRebind(attemptCount, confirmationCount, noClickEvidenceCount, clickEvidenceCount)
	}
	if oldRunState == "failed" && attemptCount > 0 && failedAttemptCount == attemptCount {
		var noClickEvidenceCount, clickEvidenceCount int
		if err := tx.QueryRowContext(ctx, `SELECT
			(SELECT COUNT(*) FROM browser_rpa_evidence WHERE organization_id=? AND project_id=? AND run_id=? AND JSON_UNQUOTE(JSON_EXTRACT(evidence_json,'$.field_readback.final_click_performed'))='false'),
			(SELECT COUNT(*) FROM browser_rpa_evidence WHERE organization_id=? AND project_id=? AND run_id=? AND JSON_UNQUOTE(JSON_EXTRACT(evidence_json,'$.field_readback.final_click_performed'))='true')`,
			request.OrganizationID, request.ProjectID, mapping.BrowserRpaRunID,
			request.OrganizationID, request.ProjectID, mapping.BrowserRpaRunID).
			Scan(&noClickEvidenceCount, &clickEvidenceCount); err != nil {
			return PlatformEntityMapping{}, err
		}
		controlledActionSafe = noClickEvidenceCount > 0 && clickEvidenceCount == 0
	}
	if oldRunState == "failed" && !controlledActionSafe && attemptCount > 0 && confirmationCount == attemptCount {
		// One staged Run can create a project and then fail while it prepares a
		// promotion. A project submit must not block recovery of the untouched
		// promotion mapping. Require every controlled action to have final-click
		// evidence for another object, and require no final-click evidence for
		// the mapping that will move to the new Run.
		var targetClickEvidenceCount, otherObjectAttemptCount int
		if err := tx.QueryRowContext(ctx, `SELECT
			(SELECT COUNT(*) FROM browser_rpa_evidence WHERE organization_id=? AND project_id=? AND run_id=? AND object_fingerprint=? AND JSON_UNQUOTE(JSON_EXTRACT(evidence_json,'$.field_readback.final_click_performed'))='true'),
			(SELECT COUNT(DISTINCT a.id) FROM browser_rpa_controlled_action_attempts a
			 JOIN browser_rpa_evidence e ON e.organization_id=a.organization_id AND e.project_id=a.project_id AND e.run_id=a.run_id AND e.step_id=a.step_id
			 WHERE a.organization_id=? AND a.project_id=? AND a.run_id=? AND e.object_fingerprint<>?
			 AND JSON_UNQUOTE(JSON_EXTRACT(e.evidence_json,'$.field_readback.final_click_performed'))='true')`,
			request.OrganizationID, request.ProjectID, mapping.BrowserRpaRunID, mapping.InternalObjectID,
			request.OrganizationID, request.ProjectID, mapping.BrowserRpaRunID, mapping.InternalObjectID).
			Scan(&targetClickEvidenceCount, &otherObjectAttemptCount); err != nil {
			return PlatformEntityMapping{}, err
		}
		controlledActionSafe = controlledActionsBelongToOtherObjects(attemptCount, confirmationCount, targetClickEvidenceCount, otherObjectAttemptCount)
	}
	if !oldLeaseInactive || oldTakeoverActive || !controlledActionSafe {
		return PlatformEntityMapping{}, ErrInvalidState
	}
	// Prepare steps and readback evidence are safe to retain after a failed,
	// cancelled, or unsubmitted prepared run. They do not prove a remote write.
	// A queued run must still have no recorded page activity before reassignment.
	if oldRunState != "failed" && oldRunState != "cancelled" && oldRunState != "awaiting_confirmation" && (stepCount != 0 || evidenceCount != 0) {
		return PlatformEntityMapping{}, ErrInvalidState
	}

	activeOldRun := oldExecutionStatus == "running" && oldChangeStatus == string(ControlledChangeSetExecuting) && oldRunState == "queued"
	cancelledOldRun := oldExecutionStatus == "cancelled" && oldChangeStatus == string(ControlledChangeSetInvalidated) && oldRunState == "cancelled"
	orphanedCancelledRun := oldExecutionStatus == "running" && oldChangeStatus == string(ControlledChangeSetExecuting) && oldRunState == "cancelled"
	failedOldRun := oldExecutionStatus == "running" && oldChangeStatus == string(ControlledChangeSetExecuting) && oldRunState == "failed"
	preparedOldRun := oldExecutionStatus == "running" && oldChangeStatus == string(ControlledChangeSetExecuting) && oldRunState == "awaiting_confirmation"
	if !activeOldRun && !cancelledOldRun && !orphanedCancelledRun && !failedOldRun && !preparedOldRun {
		return PlatformEntityMapping{}, ErrInvalidState
	}
	if activeOldRun || orphanedCancelledRun || preparedOldRun {
		updates := []struct {
			query string
			args  []any
		}{
			{`UPDATE delivery_controlled_executions SET status='cancelled',version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND status='running'`, []any{request.Now, request.OrganizationID, request.ProjectID, mapping.BusinessExecutionID}},
			{`UPDATE delivery_controlled_change_sets SET status='invalidated',version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND status='executing'`, []any{request.Now, request.OrganizationID, request.ProjectID, oldChangeSetID}},
		}
		if activeOldRun {
			updates = append([]struct {
				query string
				args  []any
			}{{`UPDATE browser_rpa_runs SET state='cancelled',version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND state='queued' AND lease_id IS NULL AND takeover_active=FALSE`, []any{request.Now, request.OrganizationID, request.ProjectID, mapping.BrowserRpaRunID}}}, updates...)
		} else if preparedOldRun {
			updates = append([]struct {
				query string
				args  []any
			}{{`UPDATE browser_rpa_runs SET state='cancelled',blocking_reason=NULL,version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND state='awaiting_confirmation' AND takeover_active=FALSE`, []any{request.Now, request.OrganizationID, request.ProjectID, mapping.BrowserRpaRunID}}}, updates...)
		}
		for _, update := range updates {
			result, updateErr := tx.ExecContext(ctx, update.query, update.args...)
			if updateErr != nil {
				return PlatformEntityMapping{}, updateErr
			}
			if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected != 1 {
				if affectedErr != nil {
					return PlatformEntityMapping{}, affectedErr
				}
				return PlatformEntityMapping{}, ErrVersionConflict
			}
		}
	}

	result, err := tx.ExecContext(ctx, `UPDATE delivery_platform_entity_mappings
		SET configuration_id=?,business_execution_id=?,browser_rpa_run_id=?,version=version+1,updated_at=?
		WHERE organization_id=? AND project_id=? AND id=? AND version=? AND status='pending_verification'
		AND platform_object_id IS NULL AND platform_status IS NULL AND result_evidence_id IS NULL AND list_evidence_id IS NULL`,
		request.ConfigurationID, request.BusinessExecutionID, request.BrowserRpaRunID, request.Now,
		request.OrganizationID, request.ProjectID, request.MappingID, request.ExpectedVersion)
	if err != nil {
		return PlatformEntityMapping{}, err
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected != 1 {
		if affectedErr != nil {
			return PlatformEntityMapping{}, affectedErr
		}
		return PlatformEntityMapping{}, ErrVersionConflict
	}
	if err := tx.Commit(); err != nil {
		return PlatformEntityMapping{}, err
	}
	return r.GetPlatformEntityMapping(ctx, request.OrganizationID, request.ProjectID, request.MappingID)
}

func controlledActionsBelongToOtherObjects(attemptCount, confirmationCount, targetClickEvidenceCount, otherObjectAttemptCount int) bool {
	return attemptCount > 0 && confirmationCount == attemptCount && targetClickEvidenceCount == 0 && otherObjectAttemptCount == attemptCount
}

func preparedRunCanRebind(attemptCount, confirmationCount, noClickEvidenceCount, clickEvidenceCount int) bool {
	return attemptCount == 0 && confirmationCount == 0 && noClickEvidenceCount > 0 && clickEvidenceCount == 0
}

func (r MySQLRepository) ListPlatformEntityMappings(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, account string) ([]PlatformEntityMapping, error) {
	rows, err := r.DB.QueryContext(ctx, platformEntityMappingSelect+` WHERE organization_id=? AND project_id=? AND account_reference_id=? ORDER BY created_at,id`, org, project, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]PlatformEntityMapping, 0)
	for rows.Next() {
		value, scanErr := scanPlatformEntityMapping(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r MySQLRepository) ValidateControlledMaterialReferences(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, accountReferenceID string, references []ControlledMaterialReference) error {
	validated := map[string]struct{}{}
	for _, reference := range references {
		key := reference.ReferenceID + "\x00" + reference.AuthorizationEvidenceID
		if _, exists := validated[key]; exists {
			continue
		}
		var payload []byte
		err := r.DB.QueryRowContext(ctx, `SELECT e.evidence_json FROM browser_rpa_evidence e JOIN browser_rpa_runs r ON r.organization_id=e.organization_id AND r.project_id=e.project_id AND r.id=e.run_id WHERE e.organization_id=? AND e.project_id=? AND e.id=? AND r.account_id=?`, org, project, reference.AuthorizationEvidenceID, accountReferenceID).Scan(&payload)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		var evidence browserautomation.Evidence
		if err := json.Unmarshal(payload, &evidence); err != nil {
			return fmt.Errorf("decode material authorization evidence: %w", err)
		}
		if evidence.OrganizationID != org || evidence.ProjectID != project || evidence.FieldReadback["authorized_material_reference_id"] != reference.ReferenceID {
			return ErrApprovalContentMismatch
		}
		validated[key] = struct{}{}
	}
	return nil
}

func (r MySQLRepository) ValidateControlledRestartReferences(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, accountReferenceID string, materials []ControlledMaterialReference, landingPage ControlledLandingPageReference) error {
	for _, reference := range materials {
		evidence, err := r.loadControlledReferenceEvidence(ctx, org, project, accountReferenceID, reference.AuthorizationEvidenceID)
		if err != nil {
			return err
		}
		if evidence.FieldReadback["authorized_material_reference_id"] != reference.ReferenceID || evidence.FieldReadback["material_available"] != "true" {
			return ErrApprovalContentMismatch
		}
	}
	evidence, err := r.loadControlledReferenceEvidence(ctx, org, project, accountReferenceID, landingPage.AuthorizationEvidenceID)
	if err != nil {
		return err
	}
	if evidence.FieldReadback["authorized_landing_page_reference_id"] != landingPage.ReferenceID || evidence.FieldReadback["landing_page_available"] != "true" {
		return ErrApprovalContentMismatch
	}
	return nil
}

func (r MySQLRepository) loadControlledReferenceEvidence(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, accountReferenceID, evidenceID string) (browserautomation.Evidence, error) {
	var payload []byte
	err := r.DB.QueryRowContext(ctx, `SELECT e.evidence_json FROM browser_rpa_evidence e JOIN browser_rpa_runs r ON r.organization_id=e.organization_id AND r.project_id=e.project_id AND r.id=e.run_id WHERE e.organization_id=? AND e.project_id=? AND e.id=? AND r.account_id=?`, org, project, evidenceID, accountReferenceID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return browserautomation.Evidence{}, ErrNotFound
	}
	if err != nil {
		return browserautomation.Evidence{}, err
	}
	var evidence browserautomation.Evidence
	if err := json.Unmarshal(payload, &evidence); err != nil {
		return browserautomation.Evidence{}, fmt.Errorf("decode controlled reference evidence: %w", err)
	}
	if evidence.OrganizationID != org || evidence.ProjectID != project {
		return browserautomation.Evidence{}, ErrApprovalContentMismatch
	}
	return evidence, nil
}

func (r MySQLRepository) ConfirmPlatformEntityMapping(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string, expectedVersion int64, resultEvidenceID, listEvidenceID string) (PlatformEntityMapping, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return PlatformEntityMapping{}, err
	}
	defer tx.Rollback()
	value, err := scanPlatformEntityMapping(tx.QueryRowContext(ctx, platformEntityMappingSelect+` WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, org, project, id))
	if err != nil {
		return PlatformEntityMapping{}, err
	}
	if value.Version != expectedVersion {
		return PlatformEntityMapping{}, ErrVersionConflict
	}
	if value.Status != PlatformEntityMappingPending && value.Status != PlatformEntityMappingConfirmed {
		return PlatformEntityMapping{}, ErrInvalidState
	}
	resultEvidence, err := loadPlatformMappingEvidence(ctx, tx, value, resultEvidenceID)
	if err != nil {
		return PlatformEntityMapping{}, err
	}
	listEvidence, err := loadPlatformMappingEvidence(ctx, tx, value, listEvidenceID)
	if err != nil {
		return PlatformEntityMapping{}, err
	}
	platformObjectID, platformStatus, err := validatePlatformMappingEvidence(value, resultEvidence, listEvidence)
	if err != nil {
		return PlatformEntityMapping{}, err
	}
	confirmedNow := value.Status == PlatformEntityMappingPending
	if confirmedNow {
		result, err := tx.ExecContext(ctx, `UPDATE delivery_platform_entity_mappings SET platform_object_id=?,platform_status=?,result_evidence_id=?,list_evidence_id=?,status='confirmed',version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND status='pending_verification' AND version=?`, platformObjectID, platformStatus, resultEvidenceID, listEvidenceID, listEvidence.Evidence.CreatedAt, org, project, id, expectedVersion)
		if err != nil {
			return PlatformEntityMapping{}, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return PlatformEntityMapping{}, err
		}
		if affected != 1 {
			return PlatformEntityMapping{}, ErrVersionConflict
		}
	} else if value.PlatformObjectID != platformObjectID || value.PlatformStatus != platformStatus || value.ResultEvidenceID != resultEvidenceID || value.ListEvidenceID != listEvidenceID {
		return PlatformEntityMapping{}, ErrApprovalContentMismatch
	}

	var changeSetID, executionRunID, executionStatus string
	if err := tx.QueryRowContext(ctx, `SELECT controlled_change_set_id,COALESCE(browser_rpa_run_id,''),status FROM delivery_controlled_executions WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, org, project, value.BusinessExecutionID).Scan(&changeSetID, &executionRunID, &executionStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PlatformEntityMapping{}, ErrNotFound
		}
		return PlatformEntityMapping{}, err
	}
	if executionRunID != value.BrowserRpaRunID {
		return PlatformEntityMapping{}, ErrApprovalContentMismatch
	}
	if confirmedNow {
		var action ControlledAction
		if err := tx.QueryRowContext(ctx, `SELECT action FROM delivery_controlled_change_sets WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, org, project, changeSetID).Scan(&action); err != nil {
			return PlatformEntityMapping{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO delivery_platform_entity_mapping_revisions (organization_id,project_id,mapping_id,mapping_version,action,business_execution_id,browser_rpa_run_id,platform_object_id,platform_status,previous_state_action,previous_state_hash,current_state_action,current_state_hash,result_evidence_id,list_evidence_id,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, org, project, value.ID, expectedVersion+1, action, value.BusinessExecutionID, value.BrowserRpaRunID, platformObjectID, platformStatus, nil, nil, nil, nil, resultEvidenceID, listEvidenceID, listEvidence.Evidence.CreatedAt); err != nil {
			return PlatformEntityMapping{}, err
		}
	}
	var pendingMappings int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM delivery_platform_entity_mappings WHERE organization_id=? AND project_id=? AND business_execution_id=? AND status='pending_verification'`, org, project, value.BusinessExecutionID).Scan(&pendingMappings); err != nil {
		return PlatformEntityMapping{}, err
	}
	fieldDrifted := resultEvidence.Evidence.FieldReadback["field_reconciliation_status"] == "drifted" || listEvidence.Evidence.FieldReadback["field_reconciliation_status"] == "drifted"
	if pendingMappings > 0 || fieldDrifted {
		if err := tx.Commit(); err != nil {
			return PlatformEntityMapping{}, err
		}
		return r.GetPlatformEntityMapping(ctx, org, project, id)
	}
	completedAt := listEvidence.Evidence.CreatedAt
	if executionStatus == "running" {
		result, err := tx.ExecContext(ctx, `UPDATE delivery_controlled_executions SET status='succeeded',version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND status='running' AND browser_rpa_run_id=?`, completedAt, org, project, value.BusinessExecutionID, value.BrowserRpaRunID)
		if err != nil {
			return PlatformEntityMapping{}, err
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			return PlatformEntityMapping{}, ErrVersionConflict
		}
	} else if executionStatus != "succeeded" {
		return PlatformEntityMapping{}, ErrInvalidState
	}
	result, err := tx.ExecContext(ctx, `UPDATE delivery_controlled_change_sets SET status='executed',version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND status='executing'`, completedAt, org, project, changeSetID)
	if err != nil {
		return PlatformEntityMapping{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return PlatformEntityMapping{}, err
	}
	if affected == 0 {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM delivery_controlled_change_sets WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, org, project, changeSetID).Scan(&status); err != nil {
			return PlatformEntityMapping{}, err
		}
		if status != string(ControlledChangeSetExecuted) {
			return PlatformEntityMapping{}, ErrInvalidState
		}
	}
	if err := tx.Commit(); err != nil {
		return PlatformEntityMapping{}, err
	}
	return r.GetPlatformEntityMapping(ctx, org, project, id)
}

func (r MySQLRepository) ConfirmPlatformEntityMappingMutation(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string, expectedVersion int64, businessExecutionID, resultEvidenceID, listEvidenceID string) (PlatformEntityMapping, PlatformEntityMappingRevision, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, err
	}
	defer tx.Rollback()
	mapping, err := scanPlatformEntityMapping(tx.QueryRowContext(ctx, platformEntityMappingSelect+` WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, org, project, id))
	if err != nil {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, err
	}
	if mapping.Version == expectedVersion+1 {
		revision, revisionErr := scanPlatformEntityMappingRevision(tx.QueryRowContext(ctx, platformEntityMappingRevisionSelect+` WHERE organization_id=? AND project_id=? AND mapping_id=? AND mapping_version=?`, org, project, id, mapping.Version))
		if revisionErr == nil && revision.BusinessExecutionID == businessExecutionID && revision.ResultEvidenceID == resultEvidenceID && revision.ListEvidenceID == listEvidenceID {
			return mapping, revision, nil
		}
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrVersionConflict
	}
	if mapping.Version != expectedVersion {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrVersionConflict
	}
	if mapping.Status != PlatformEntityMappingConfirmed || mapping.PlatformObjectKind != "promotion" || mapping.PlatformObjectID == "" {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrInvalidState
	}
	var execution ControlledExecution
	if err := tx.QueryRowContext(ctx, `SELECT id,organization_id,project_id,controlled_change_set_id,remote_write_approval_id,COALESCE(browser_rpa_run_id,''),status,version,created_by,created_at,updated_at FROM delivery_controlled_executions WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, org, project, businessExecutionID).Scan(&execution.ID, &execution.OrganizationID, &execution.ProjectID, &execution.ControlledChangeSetID, &execution.RemoteWriteApprovalID, &execution.BrowserRpaRunID, &execution.Status, &execution.Version, &execution.CreatedBy, &execution.CreatedAt, &execution.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrNotFound
		}
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, err
	}
	if execution.Status != "running" || execution.BrowserRpaRunID == "" {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrInvalidState
	}
	change, err := scanControlledChangeSet(tx.QueryRowContext(ctx, controlledChangeSetSelect+` WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, org, project, execution.ControlledChangeSetID))
	if err != nil {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, err
	}
	currentStateHash, targetStateHash, stateErr := change.Binding.existingPromotionStateHashes(change.Action)
	if change.Status != ControlledChangeSetExecuting || !change.Action.ChangesExistingPromotion() || stateErr != nil || change.Binding.TargetMappingID != mapping.ID || change.Binding.TargetMappingVersion != expectedVersion || change.Binding.TargetPlatformObjectID != mapping.PlatformObjectID || change.Binding.TargetPlatformObjectKind != mapping.PlatformObjectKind || change.Binding.AccountReferenceID != mapping.AccountReferenceID || (mapping.CurrentStateAction == change.Action && mapping.CurrentStateHash != currentStateHash) {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrApprovalContentMismatch
	}
	var approvalChangeSetID, approvalActionHash string
	if err := tx.QueryRowContext(ctx, `SELECT controlled_change_set_id,action_hash FROM delivery_remote_write_approvals WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, org, project, execution.RemoteWriteApprovalID).Scan(&approvalChangeSetID, &approvalActionHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrNotFound
		}
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, err
	}
	if approvalChangeSetID != change.ID || approvalActionHash == "" {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrApprovalContentMismatch
	}
	evidenceScope := mapping
	evidenceScope.BrowserRpaRunID = execution.BrowserRpaRunID
	evidenceScope.InternalObjectID = change.Binding.ObjectFingerprint
	resultEvidence, err := loadPlatformMappingEvidence(ctx, tx, evidenceScope, resultEvidenceID)
	if err != nil {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, err
	}
	listEvidence, err := loadPlatformMappingEvidence(ctx, tx, evidenceScope, listEvidenceID)
	if err != nil {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, err
	}
	platformObjectID, platformStatus, err := validatePlatformMappingEvidence(evidenceScope, resultEvidence, listEvidence)
	if err != nil {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, err
	}
	targetStatus := change.Binding.existingPromotionTargetStatus()
	if platformObjectID != mapping.PlatformObjectID || resultEvidence.Evidence.FieldReadback["target_state_hash"] != targetStateHash || listEvidence.Evidence.FieldReadback["target_state_hash"] != targetStateHash || (targetStatus != "" && platformStatus != targetStatus) {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrApprovalContentMismatch
	}
	completedAt := listEvidence.Evidence.CreatedAt
	result, err := tx.ExecContext(ctx, `UPDATE delivery_platform_entity_mappings SET business_execution_id=?,browser_rpa_run_id=?,platform_status=?,current_state_action=?,current_state_hash=?,result_evidence_id=?,list_evidence_id=?,version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND status='confirmed' AND version=?`, execution.ID, execution.BrowserRpaRunID, platformStatus, change.Action, targetStateHash, resultEvidenceID, listEvidenceID, completedAt, org, project, id, expectedVersion)
	if err != nil {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, err
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected != 1 {
		if affectedErr != nil {
			return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, affectedErr
		}
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrVersionConflict
	}
	revision := PlatformEntityMappingRevision{MappingID: mapping.ID, OrganizationID: org, ProjectID: project, Version: expectedVersion + 1, Action: change.Action, BusinessExecutionID: execution.ID, BrowserRpaRunID: execution.BrowserRpaRunID, PlatformObjectID: platformObjectID, PlatformStatus: platformStatus, PreviousStateAction: mapping.CurrentStateAction, PreviousStateHash: mapping.CurrentStateHash, CurrentStateAction: change.Action, CurrentStateHash: targetStateHash, ResultEvidenceID: resultEvidenceID, ListEvidenceID: listEvidenceID, CreatedAt: completedAt}
	if _, err := tx.ExecContext(ctx, `INSERT INTO delivery_platform_entity_mapping_revisions (organization_id,project_id,mapping_id,mapping_version,action,business_execution_id,browser_rpa_run_id,platform_object_id,platform_status,previous_state_action,previous_state_hash,current_state_action,current_state_hash,result_evidence_id,list_evidence_id,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, revision.OrganizationID, revision.ProjectID, revision.MappingID, revision.Version, revision.Action, revision.BusinessExecutionID, revision.BrowserRpaRunID, revision.PlatformObjectID, revision.PlatformStatus, nullableString(string(revision.PreviousStateAction)), nullableString(revision.PreviousStateHash), revision.CurrentStateAction, revision.CurrentStateHash, revision.ResultEvidenceID, revision.ListEvidenceID, revision.CreatedAt); err != nil {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, err
	}
	result, err = tx.ExecContext(ctx, `UPDATE delivery_controlled_executions SET status='succeeded',version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND status='running' AND browser_rpa_run_id=?`, completedAt, org, project, execution.ID, execution.BrowserRpaRunID)
	if err != nil {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrVersionConflict
	}
	result, err = tx.ExecContext(ctx, `UPDATE delivery_controlled_change_sets SET status='executed',version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND status='executing'`, completedAt, org, project, change.ID)
	if err != nil {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrVersionConflict
	}
	if err := tx.Commit(); err != nil {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, err
	}
	updated, err := r.GetPlatformEntityMapping(ctx, org, project, id)
	if err != nil {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, err
	}
	return updated, revision, nil
}

type platformMappingEvidence struct {
	Evidence browserautomation.Evidence
	Step     browserautomation.RunStep
}

func loadPlatformMappingEvidence(ctx context.Context, tx *sql.Tx, mapping PlatformEntityMapping, evidenceID string) (platformMappingEvidence, error) {
	var payload []byte
	var value platformMappingEvidence
	err := tx.QueryRowContext(ctx, `SELECT e.evidence_json,s.id,s.run_id,s.sequence_number,s.action,s.status FROM browser_rpa_evidence e JOIN browser_rpa_run_steps s ON s.organization_id=e.organization_id AND s.project_id=e.project_id AND s.run_id=e.run_id AND s.id=e.step_id WHERE e.organization_id=? AND e.project_id=? AND e.run_id=? AND e.id=? FOR UPDATE`, mapping.OrganizationID, mapping.ProjectID, mapping.BrowserRpaRunID, evidenceID).Scan(&payload, &value.Step.ID, &value.Step.RunID, &value.Step.Sequence, &value.Step.Action, &value.Step.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return platformMappingEvidence{}, ErrNotFound
	}
	if err != nil {
		return platformMappingEvidence{}, err
	}
	if err := json.Unmarshal(payload, &value.Evidence); err != nil {
		return platformMappingEvidence{}, fmt.Errorf("decode computer-use evidence: %w", err)
	}
	return value, nil
}

func validatePlatformMappingEvidence(mapping PlatformEntityMapping, result, list platformMappingEvidence) (string, string, error) {
	if result.Evidence.ID == "" || list.Evidence.ID == "" || result.Evidence.ID == list.Evidence.ID ||
		result.Evidence.OrganizationID != mapping.OrganizationID || list.Evidence.OrganizationID != mapping.OrganizationID ||
		result.Evidence.ProjectID != mapping.ProjectID || list.Evidence.ProjectID != mapping.ProjectID ||
		result.Evidence.RunID != mapping.BrowserRpaRunID || list.Evidence.RunID != mapping.BrowserRpaRunID ||
		result.Evidence.StepID != result.Step.ID || list.Evidence.StepID != list.Step.ID ||
		result.Evidence.ObjectFingerprint != mapping.InternalObjectID || list.Evidence.ObjectFingerprint != mapping.InternalObjectID ||
		result.Step.RunID != mapping.BrowserRpaRunID || list.Step.RunID != mapping.BrowserRpaRunID ||
		result.Step.Action != string(browserautomation.TakeoverResultObserved) || list.Step.Action != string(browserautomation.TakeoverListConfirmed) ||
		result.Step.Status != browserautomation.StepSucceeded || list.Step.Status != browserautomation.StepSucceeded ||
		result.Step.Sequence >= list.Step.Sequence {
		return "", "", ErrApprovalContentMismatch
	}
	resultObjectID, resultStatus := result.Evidence.FieldReadback["platform_object_id"], result.Evidence.FieldReadback["platform_status"]
	listObjectID, listStatus := list.Evidence.FieldReadback["platform_object_id"], list.Evidence.FieldReadback["platform_status"]
	if resultObjectID == "" || resultStatus == "" || resultObjectID != listObjectID || resultStatus != listStatus {
		return "", "", ErrApprovalContentMismatch
	}
	return resultObjectID, resultStatus, nil
}

func scanPlatformEntityMapping(row rowScanner) (PlatformEntityMapping, error) {
	var value PlatformEntityMapping
	err := row.Scan(&value.ID, &value.OrganizationID, &value.ProjectID, &value.AccountReferenceID, &value.PlanID, &value.ConfigurationID, &value.BusinessExecutionID, &value.BrowserRpaRunID, &value.InternalObjectKind, &value.InternalObjectID, &value.PlatformObjectKind, &value.PlatformObjectID, &value.PlatformStatus, &value.CurrentStateAction, &value.CurrentStateHash, &value.ResultEvidenceID, &value.ListEvidenceID, &value.Status, &value.Version, &value.CreatedAt, &value.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return PlatformEntityMapping{}, ErrNotFound
	}
	if err != nil {
		return PlatformEntityMapping{}, err
	}
	value.SchemaVersion = PlatformEntityMappingV1
	return value, nil
}

func scanPlatformEntityMappingRevision(row rowScanner) (PlatformEntityMappingRevision, error) {
	var value PlatformEntityMappingRevision
	err := row.Scan(&value.OrganizationID, &value.ProjectID, &value.MappingID, &value.Version, &value.Action, &value.BusinessExecutionID, &value.BrowserRpaRunID, &value.PlatformObjectID, &value.PlatformStatus, &value.PreviousStateAction, &value.PreviousStateHash, &value.CurrentStateAction, &value.CurrentStateHash, &value.ResultEvidenceID, &value.ListEvidenceID, &value.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return PlatformEntityMappingRevision{}, ErrNotFound
	}
	return value, err
}

const controlledChangeSetSelect = `SELECT id,organization_id,project_id,binding_json,action,budget_limit_minor,currency,status,canonical_hash,version,created_by,created_at,updated_at FROM delivery_controlled_change_sets`
const platformEntityMappingSelect = `SELECT id,organization_id,project_id,account_reference_id,plan_id,configuration_id,business_execution_id,browser_rpa_run_id,internal_object_kind,internal_object_id,platform_object_kind,COALESCE(platform_object_id,''),COALESCE(platform_status,''),COALESCE(current_state_action,''),COALESCE(current_state_hash,''),COALESCE(result_evidence_id,''),COALESCE(list_evidence_id,''),status,version,created_at,updated_at FROM delivery_platform_entity_mappings`
const platformEntityMappingRevisionSelect = `SELECT organization_id,project_id,mapping_id,mapping_version,action,business_execution_id,browser_rpa_run_id,COALESCE(platform_object_id,''),COALESCE(platform_status,''),COALESCE(previous_state_action,''),COALESCE(previous_state_hash,''),COALESCE(current_state_action,''),COALESCE(current_state_hash,''),COALESCE(result_evidence_id,''),COALESCE(list_evidence_id,''),created_at FROM delivery_platform_entity_mapping_revisions`

func scanControlledChangeSet(row rowScanner) (ControlledChangeSet, error) {
	var v ControlledChangeSet
	var binding []byte
	err := row.Scan(&v.ID, &v.OrganizationID, &v.ProjectID, &binding, &v.Action, &v.BudgetLimitMinor, &v.Currency, &v.Status, &v.CanonicalHash, &v.Version, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ControlledChangeSet{}, ErrNotFound
	}
	if err != nil {
		return ControlledChangeSet{}, err
	}
	if err := json.Unmarshal(binding, &v.Binding); err != nil {
		return ControlledChangeSet{}, fmt.Errorf("decode controlled change-set binding: %w", err)
	}
	v.SchemaVersion = ControlledChangeSetSchemaV1
	return v, nil
}
