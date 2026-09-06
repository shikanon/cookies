package browserautomation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type MySQLRepository struct{ DB *sql.DB }

func (r MySQLRepository) CreateRun(ctx context.Context, value BrowserRpaRun) (BrowserRpaRun, bool, error) {
	authority, err := json.Marshal(value.Authority)
	if err != nil {
		return BrowserRpaRun{}, false, err
	}
	_, err = r.DB.ExecContext(ctx, `INSERT INTO browser_rpa_runs
		(id,organization_id,project_id,platform,account_id,execution_driver,authority_json,environment_id,profile_id,lease_id,policy_id,state,blocking_reason,paused,takeover_active,version,idempotency_key,request_hash,created_by,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.OrganizationID, value.ProjectID, value.Platform, value.AccountID, value.EffectiveExecutionDriver(), authority, value.EnvironmentID, value.ProfileID, nullableString(value.LeaseID), value.PolicyID, value.State, nullableString(string(value.BlockingReason)), value.Paused, value.TakeoverActive, value.Version, value.IdempotencyKey, value.RequestHash, value.CreatedBy, value.CreatedAt, value.UpdatedAt)
	if err == nil {
		return value, false, nil
	}
	existingByID, idErr := r.GetRun(ctx, value.OrganizationID, value.ProjectID, value.ID)
	if idErr == nil {
		if existingByID.IdempotencyKey != value.IdempotencyKey || existingByID.RequestHash != value.RequestHash {
			return BrowserRpaRun{}, false, ErrIdempotencyConflict
		}
		return existingByID, true, nil
	}
	existing, getErr := r.getRunByIdempotency(ctx, value.OrganizationID, value.ProjectID, value.IdempotencyKey)
	if getErr != nil {
		return BrowserRpaRun{}, false, err
	}
	if existing.RequestHash != value.RequestHash {
		return BrowserRpaRun{}, false, ErrIdempotencyConflict
	}
	return existing, true, nil
}

func (r MySQLRepository) GetRun(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string) (BrowserRpaRun, error) {
	return scanRun(r.DB.QueryRowContext(ctx, runSelect+` WHERE organization_id=? AND project_id=? AND id=?`, org, project, id))
}

func (r MySQLRepository) ListRuns(ctx context.Context, org contract.OrganizationID, project contract.ProjectID) ([]BrowserRpaRun, error) {
	rows, err := r.DB.QueryContext(ctx, runSelect+` WHERE organization_id=? AND project_id=? ORDER BY updated_at DESC LIMIT 100`, org, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]BrowserRpaRun, 0)
	for rows.Next() {
		value, scanErr := scanRun(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r MySQLRepository) CreateEnvironment(ctx context.Context, value ExecutionEnvironment) (ExecutionEnvironment, error) {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO browser_rpa_environments (id,organization_id,project_id,platform,account_id,mode,browser_version,region,healthy,cdp_endpoint,version) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.OrganizationID, value.ProjectID, value.Platform, value.AccountID, value.Mode, value.BrowserVersion, value.Region, value.Healthy, value.CDPEndpoint, value.Version)
	if err == nil {
		return value, nil
	}
	existing, getErr := r.GetEnvironment(ctx, value.OrganizationID, value.ProjectID, value.ID)
	if getErr != nil {
		return ExecutionEnvironment{}, err
	}
	if !reflect.DeepEqual(existing, value) {
		return ExecutionEnvironment{}, ErrIdempotencyConflict
	}
	return existing, nil
}

func (r MySQLRepository) GetEnvironment(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string) (ExecutionEnvironment, error) {
	var value ExecutionEnvironment
	err := r.DB.QueryRowContext(ctx, `SELECT id,organization_id,project_id,platform,account_id,mode,browser_version,region,healthy,COALESCE(cdp_endpoint,''),version FROM browser_rpa_environments WHERE organization_id=? AND project_id=? AND id=?`, org, project, id).Scan(&value.ID, &value.OrganizationID, &value.ProjectID, &value.Platform, &value.AccountID, &value.Mode, &value.BrowserVersion, &value.Region, &value.Healthy, &value.CDPEndpoint, &value.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return ExecutionEnvironment{}, ErrNotFound
	}
	return value, err
}

func (r MySQLRepository) CreateBrowserProfile(ctx context.Context, value BrowserProfile) (BrowserProfile, error) {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO browser_rpa_browser_profiles (id,organization_id,project_id,environment_id,platform,account_id,state,version) VALUES (?,?,?,?,?,?,?,?)`, value.ID, value.OrganizationID, value.ProjectID, value.EnvironmentID, value.Platform, value.AccountID, value.State, value.Version)
	if err == nil {
		return value, nil
	}
	existing, getErr := r.GetBrowserProfile(ctx, value.OrganizationID, value.ProjectID, value.ID)
	if getErr != nil {
		return BrowserProfile{}, err
	}
	if !reflect.DeepEqual(existing, value) {
		return BrowserProfile{}, ErrIdempotencyConflict
	}
	return existing, nil
}

func (r MySQLRepository) GetBrowserProfile(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string) (BrowserProfile, error) {
	var value BrowserProfile
	err := r.DB.QueryRowContext(ctx, `SELECT id,organization_id,project_id,environment_id,platform,account_id,state,version FROM browser_rpa_browser_profiles WHERE organization_id=? AND project_id=? AND id=?`, org, project, id).Scan(&value.ID, &value.OrganizationID, &value.ProjectID, &value.EnvironmentID, &value.Platform, &value.AccountID, &value.State, &value.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return BrowserProfile{}, ErrNotFound
	}
	return value, err
}

func (r MySQLRepository) CreateSitePolicy(ctx context.Context, value SitePolicy) (SitePolicy, error) {
	protocols, err := json.Marshal(value.AllowedProtocols)
	if err != nil {
		return SitePolicy{}, err
	}
	hosts, err := json.Marshal(value.AllowedHosts)
	if err != nil {
		return SitePolicy{}, err
	}
	pageKinds, err := json.Marshal(value.AllowedPageKinds)
	if err != nil {
		return SitePolicy{}, err
	}
	platformProjects, err := json.Marshal(value.AllowedPlatformProjects)
	if err != nil {
		return SitePolicy{}, err
	}
	_, err = r.DB.ExecContext(ctx, `INSERT INTO browser_rpa_site_policies (id,organization_id,project_id,platform,account_id,allowed_protocols,allowed_hosts,allowed_page_kinds,allowed_platform_project_ids,version) VALUES (?,?,?,?,?,?,?,?,?,?)`, value.ID, value.OrganizationID, value.ProjectID, value.Platform, value.AccountID, protocols, hosts, pageKinds, platformProjects, value.Version)
	if err == nil {
		return value, nil
	}
	existing, getErr := r.GetSitePolicy(ctx, value.OrganizationID, value.ProjectID, value.ID)
	if getErr != nil {
		return SitePolicy{}, err
	}
	if !reflect.DeepEqual(existing, value) {
		return SitePolicy{}, ErrIdempotencyConflict
	}
	return existing, nil
}

func (r MySQLRepository) GetSitePolicy(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string) (SitePolicy, error) {
	var value SitePolicy
	var protocols, hosts, pageKinds, platformProjects []byte
	err := r.DB.QueryRowContext(ctx, `SELECT id,organization_id,project_id,platform,account_id,allowed_protocols,allowed_hosts,allowed_page_kinds,allowed_platform_project_ids,version FROM browser_rpa_site_policies WHERE organization_id=? AND project_id=? AND id=?`, org, project, id).Scan(&value.ID, &value.OrganizationID, &value.ProjectID, &value.Platform, &value.AccountID, &protocols, &hosts, &pageKinds, &platformProjects, &value.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return SitePolicy{}, ErrNotFound
	}
	if err != nil {
		return SitePolicy{}, err
	}
	for payload, target := range map[*[]byte]*[]string{&protocols: &value.AllowedProtocols, &hosts: &value.AllowedHosts, &pageKinds: &value.AllowedPageKinds, &platformProjects: &value.AllowedPlatformProjects} {
		if err := json.Unmarshal(*payload, target); err != nil {
			return SitePolicy{}, fmt.Errorf("decode browser-rpa site policy: %w", err)
		}
	}
	return value, nil
}

func (r MySQLRepository) getRunByIdempotency(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, key string) (BrowserRpaRun, error) {
	return scanRun(r.DB.QueryRowContext(ctx, runSelect+` WHERE organization_id=? AND project_id=? AND idempotency_key=?`, org, project, key))
}

func (r MySQLRepository) TransitionRun(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string, expected int64, state RunState, reason BlockingReason, now time.Time) (BrowserRpaRun, error) {
	result, err := r.DB.ExecContext(ctx, `UPDATE browser_rpa_runs SET state=?,blocking_reason=?,version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND version=?`, state, nullableString(string(reason)), now, org, project, id, expected)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return BrowserRpaRun{}, err
	}
	if affected != 1 {
		return BrowserRpaRun{}, ErrVersionConflict
	}
	return r.GetRun(ctx, org, project, id)
}

func (r MySQLRepository) SetRunControl(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string, expected int64, state RunState, paused, takeover bool, reason BlockingReason, now time.Time) (BrowserRpaRun, error) {
	result, err := r.DB.ExecContext(ctx, `UPDATE browser_rpa_runs SET state=?,paused=?,takeover_active=?,blocking_reason=?,version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND version=?`, state, paused, takeover, nullableString(string(reason)), now, org, project, id, expected)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return BrowserRpaRun{}, ErrVersionConflict
	}
	return r.GetRun(ctx, org, project, id)
}

func (r MySQLRepository) PutStep(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, value RunStep) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO browser_rpa_run_steps (id,organization_id,project_id,run_id,sequence_number,workflow_step_id,action,status,blocking_reason,attempt,version) VALUES (?,?,?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE status=VALUES(status),blocking_reason=VALUES(blocking_reason),attempt=VALUES(attempt),version=VALUES(version)`, value.ID, org, project, value.RunID, value.Sequence, value.WorkflowStepID, value.Action, value.Status, nullableString(string(value.BlockingReason)), value.Attempt, value.Version)
	return err
}
func (r MySQLRepository) ListSteps(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, runID string) ([]RunStep, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id,run_id,sequence_number,workflow_step_id,action,status,COALESCE(blocking_reason,''),attempt,version FROM browser_rpa_run_steps WHERE organization_id=? AND project_id=? AND run_id=? ORDER BY sequence_number`, org, project, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []RunStep{}
	for rows.Next() {
		var v RunStep
		if err := rows.Scan(&v.ID, &v.RunID, &v.Sequence, &v.WorkflowStepID, &v.Action, &v.Status, &v.BlockingReason, &v.Attempt, &v.Version); err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, rows.Err()
}

func (r MySQLRepository) AcquireLease(ctx context.Context, value SessionLease) (SessionLease, error) {
	activeKey := string(value.OrganizationID) + ":" + string(value.ProjectID) + ":" + string(value.Platform) + ":" + value.AccountID + ":" + value.ProfileID
	_, err := r.DB.ExecContext(ctx, `INSERT INTO browser_rpa_session_leases (id,organization_id,project_id,run_id,environment_id,profile_id,platform,account_id,holder,active_lock_key,fencing_token,version,expires_at,heartbeat_deadline,released_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.OrganizationID, value.ProjectID, value.RunID, value.EnvironmentID, value.ProfileID, value.Platform, value.AccountID, value.Holder, activeKey, value.FencingToken, value.Version, value.ExpiresAt, value.HeartbeatDeadline, value.ReleasedAt)
	if err != nil {
		return SessionLease{}, ErrLeaseUnavailable
	}
	return value, nil
}

func (r MySQLRepository) AcquireRunLease(ctx context.Context, run BrowserRpaRun, expected int64, lease SessionLease, now time.Time) (BrowserRpaRun, SessionLease, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return BrowserRpaRun{}, SessionLease{}, err
	}
	defer tx.Rollback()
	activeKey := string(lease.OrganizationID) + ":" + string(lease.ProjectID) + ":" + string(lease.Platform) + ":" + lease.AccountID + ":" + lease.ProfileID
	var lastFencingToken int64
	err = tx.QueryRowContext(ctx, `SELECT fencing_token FROM browser_rpa_session_leases WHERE organization_id=? AND project_id=? AND profile_id=? ORDER BY fencing_token DESC LIMIT 1 FOR UPDATE`, lease.OrganizationID, lease.ProjectID, lease.ProfileID).Scan(&lastFencingToken)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return BrowserRpaRun{}, SessionLease{}, err
	}
	lease.FencingToken = lastFencingToken + 1
	_, err = tx.ExecContext(ctx, `UPDATE browser_rpa_session_leases SET active_lock_key=NULL,released_at=?,version=version+1 WHERE organization_id=? AND project_id=? AND active_lock_key=? AND released_at IS NULL AND (expires_at<=? OR heartbeat_deadline<=?)`, now, lease.OrganizationID, lease.ProjectID, activeKey, now, now)
	if err != nil {
		return BrowserRpaRun{}, SessionLease{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO browser_rpa_session_leases (id,organization_id,project_id,run_id,environment_id,profile_id,platform,account_id,holder,active_lock_key,fencing_token,version,expires_at,heartbeat_deadline,released_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, lease.ID, lease.OrganizationID, lease.ProjectID, lease.RunID, lease.EnvironmentID, lease.ProfileID, lease.Platform, lease.AccountID, lease.Holder, activeKey, lease.FencingToken, lease.Version, lease.ExpiresAt, lease.HeartbeatDeadline, lease.ReleasedAt)
	if err != nil {
		return BrowserRpaRun{}, SessionLease{}, ErrLeaseUnavailable
	}
	result, err := tx.ExecContext(ctx, `UPDATE browser_rpa_runs SET lease_id=?,version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND version=? AND lease_id IS NULL`, lease.ID, now, run.OrganizationID, run.ProjectID, run.ID, expected)
	if err != nil {
		return BrowserRpaRun{}, SessionLease{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return BrowserRpaRun{}, SessionLease{}, err
	}
	if affected != 1 {
		return BrowserRpaRun{}, SessionLease{}, ErrVersionConflict
	}
	if err := tx.Commit(); err != nil {
		return BrowserRpaRun{}, SessionLease{}, err
	}
	updated, err := r.GetRun(ctx, run.OrganizationID, run.ProjectID, run.ID)
	return updated, lease, err
}

func (r MySQLRepository) GetLease(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string) (SessionLease, error) {
	return scanLease(r.DB.QueryRowContext(ctx, leaseSelect+` WHERE organization_id=? AND project_id=? AND id=?`, org, project, id))
}

func (r MySQLRepository) HeartbeatLease(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string, expectedVersion, fencingToken int64, now, expiresAt, heartbeatDeadline time.Time) (SessionLease, error) {
	result, err := r.DB.ExecContext(ctx, `UPDATE browser_rpa_session_leases SET expires_at=?,heartbeat_deadline=?,version=version+1 WHERE organization_id=? AND project_id=? AND id=? AND version=? AND fencing_token=? AND released_at IS NULL AND expires_at>? AND heartbeat_deadline>?`, expiresAt, heartbeatDeadline, org, project, id, expectedVersion, fencingToken, now, now)
	if err != nil {
		return SessionLease{}, err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return SessionLease{}, ErrVersionConflict
	}
	return r.GetLease(ctx, org, project, id)
}

func (r MySQLRepository) ReleaseLease(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string, expectedVersion, fencingToken int64, now time.Time) (SessionLease, error) {
	result, err := r.DB.ExecContext(ctx, `UPDATE browser_rpa_session_leases SET active_lock_key=NULL,released_at=?,version=version+1 WHERE organization_id=? AND project_id=? AND id=? AND version=? AND fencing_token=? AND released_at IS NULL`, now, org, project, id, expectedVersion, fencingToken)
	if err != nil {
		return SessionLease{}, err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return SessionLease{}, ErrVersionConflict
	}
	return r.GetLease(ctx, org, project, id)
}

func (r MySQLRepository) ReleaseRunLease(ctx context.Context, run BrowserRpaRun, expectedRunVersion int64, lease SessionLease, expectedLeaseVersion, fencingToken int64, now time.Time) (BrowserRpaRun, SessionLease, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return BrowserRpaRun{}, SessionLease{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE browser_rpa_session_leases SET active_lock_key=NULL,released_at=?,version=version+1 WHERE organization_id=? AND project_id=? AND id=? AND run_id=? AND version=? AND fencing_token=? AND released_at IS NULL`, now, lease.OrganizationID, lease.ProjectID, lease.ID, run.ID, expectedLeaseVersion, fencingToken)
	if err != nil {
		return BrowserRpaRun{}, SessionLease{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return BrowserRpaRun{}, SessionLease{}, err
	}
	if affected != 1 {
		return BrowserRpaRun{}, SessionLease{}, ErrVersionConflict
	}
	result, err = tx.ExecContext(ctx, `UPDATE browser_rpa_runs SET lease_id=NULL,version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND version=? AND lease_id=?`, now, run.OrganizationID, run.ProjectID, run.ID, expectedRunVersion, lease.ID)
	if err != nil {
		return BrowserRpaRun{}, SessionLease{}, err
	}
	affected, err = result.RowsAffected()
	if err != nil {
		return BrowserRpaRun{}, SessionLease{}, err
	}
	if affected != 1 {
		return BrowserRpaRun{}, SessionLease{}, ErrVersionConflict
	}
	if err := tx.Commit(); err != nil {
		return BrowserRpaRun{}, SessionLease{}, err
	}
	updatedRun, err := r.GetRun(ctx, run.OrganizationID, run.ProjectID, run.ID)
	if err != nil {
		return BrowserRpaRun{}, SessionLease{}, err
	}
	updatedLease, err := r.GetLease(ctx, lease.OrganizationID, lease.ProjectID, lease.ID)
	return updatedRun, updatedLease, err
}

func (r MySQLRepository) PutKillSwitch(ctx context.Context, value KillSwitch, expected int64) (KillSwitch, error) {
	scopeKey := "*"
	if value.Scope == KillSwitchPlatform {
		scopeKey = string(value.Platform)
	}
	if value.Scope == KillSwitchOrganization {
		scopeKey = string(value.OrganizationID)
	}
	if expected == 0 {
		_, err := r.DB.ExecContext(ctx, `INSERT INTO browser_rpa_kill_switches (id,scope,scope_key,organization_id,platform,active,reason,version,updated_by,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?)`, value.ID, value.Scope, scopeKey, nullableString(string(value.OrganizationID)), nullableString(string(value.Platform)), value.Active, value.Reason, value.Version, value.UpdatedBy, value.UpdatedAt)
		if err != nil {
			return KillSwitch{}, ErrVersionConflict
		}
		return value, nil
	}
	result, err := r.DB.ExecContext(ctx, `UPDATE browser_rpa_kill_switches SET active=?,reason=?,version=version+1,updated_by=?,updated_at=? WHERE scope=? AND scope_key=? AND version=?`, value.Active, value.Reason, value.UpdatedBy, value.UpdatedAt, value.Scope, scopeKey, expected)
	if err != nil {
		return KillSwitch{}, err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return KillSwitch{}, ErrVersionConflict
	}
	value.Version = expected + 1
	return value, nil
}

func (r MySQLRepository) ActiveKillSwitch(ctx context.Context, org contract.OrganizationID, platform Platform) (KillSwitch, bool, error) {
	row := r.DB.QueryRowContext(ctx, killSelect+` WHERE active=TRUE AND ((scope='global' AND scope_key='*') OR (scope='platform' AND scope_key=?) OR (scope='organization' AND scope_key=?)) ORDER BY FIELD(scope,'global','platform','organization') LIMIT 1`, platform, org)
	value, err := scanKill(row)
	if errors.Is(err, ErrNotFound) {
		return KillSwitch{}, false, nil
	}
	return value, err == nil, err
}

func (r MySQLRepository) IssueConfirmation(ctx context.Context, value FinalConfirmation) (FinalConfirmation, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return FinalConfirmation{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `UPDATE browser_rpa_final_confirmations SET invalidated_at=?,version=version+1 WHERE organization_id=? AND project_id=? AND run_id=? AND consumed_at IS NULL AND rejected_at IS NULL AND invalidated_at IS NULL`, value.IssuedAt, value.OrganizationID, value.ProjectID, value.RunID)
	if err != nil {
		return FinalConfirmation{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO browser_rpa_final_confirmations (id,organization_id,project_id,run_id,binding_hash,token_digest,issued_by,issued_at,expires_at,consumed_at,rejected_at,invalidated_at,version) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.OrganizationID, value.ProjectID, value.RunID, value.BindingHash, value.TokenDigest, value.IssuedBy, value.IssuedAt, value.ExpiresAt, value.ConsumedAt, value.RejectedAt, value.InvalidatedAt, value.Version)
	if err != nil {
		return FinalConfirmation{}, err
	}
	if err := tx.Commit(); err != nil {
		return FinalConfirmation{}, err
	}
	return value, nil
}

func (r MySQLRepository) AuthorizeControlledAction(ctx context.Context, identity FinalConfirmation, digest string, lease SessionLease, attempt ControlledActionAttempt, now time.Time) (ControlledActionAttempt, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return ControlledActionAttempt{}, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id FROM browser_rpa_kill_switches WHERE active=TRUE AND ((scope='global' AND scope_key='*') OR (scope='platform' AND scope_key=?) OR (scope='organization' AND scope_key=?)) FOR UPDATE`, lease.Platform, identity.OrganizationID)
	if err != nil {
		return ControlledActionAttempt{}, err
	}
	active := rows.Next()
	rows.Close()
	if active {
		return ControlledActionAttempt{}, ErrKillSwitchActive
	}
	storedLease, err := scanLease(tx.QueryRowContext(ctx, leaseSelect+` WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, identity.OrganizationID, identity.ProjectID, lease.ID))
	if err != nil || storedLease.RunID != identity.RunID || storedLease.FencingToken != lease.FencingToken || !storedLease.ValidAt(now) {
		return ControlledActionAttempt{}, ErrLeaseUnavailable
	}
	var confirmation FinalConfirmation
	var consumed, rejected, invalidated sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT id,organization_id,project_id,run_id,binding_hash,token_digest,issued_by,issued_at,expires_at,consumed_at,rejected_at,invalidated_at,version FROM browser_rpa_final_confirmations WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, identity.OrganizationID, identity.ProjectID, identity.ID).Scan(&confirmation.ID, &confirmation.OrganizationID, &confirmation.ProjectID, &confirmation.RunID, &confirmation.BindingHash, &confirmation.TokenDigest, &confirmation.IssuedBy, &confirmation.IssuedAt, &confirmation.ExpiresAt, &consumed, &rejected, &invalidated, &confirmation.Version)
	if err != nil {
		return ControlledActionAttempt{}, ErrConfirmationInvalid
	}
	confirmation.SchemaVersion = ConfirmationSchemaV1
	confirmation.ConsumedAt = timePtr(consumed)
	confirmation.RejectedAt = timePtr(rejected)
	confirmation.InvalidatedAt = timePtr(invalidated)
	if confirmation.RunID != identity.RunID || confirmation.BindingHash != identity.BindingHash || confirmation.TokenDigest != digest || !confirmation.UsableAt(now) {
		return ControlledActionAttempt{}, ErrConfirmationInvalid
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO browser_rpa_controlled_action_attempts (id,organization_id,project_id,run_id,step_id,confirmation_id,approval_id,lease_id,fencing_token,action_hash,idempotency_key,status,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, attempt.ID, attempt.OrganizationID, attempt.ProjectID, attempt.RunID, attempt.StepID, attempt.ConfirmationID, attempt.ApprovalID, attempt.LeaseID, attempt.FencingToken, attempt.ActionHash, attempt.IdempotencyKey, attempt.Status, attempt.CreatedAt)
	if err != nil {
		var existingHash string
		scanErr := tx.QueryRowContext(ctx, `SELECT action_hash FROM browser_rpa_controlled_action_attempts WHERE organization_id=? AND project_id=? AND idempotency_key=?`, attempt.OrganizationID, attempt.ProjectID, attempt.IdempotencyKey).Scan(&existingHash)
		if scanErr == nil && existingHash != attempt.ActionHash {
			return ControlledActionAttempt{}, ErrIdempotencyConflict
		}
		if scanErr == nil {
			return ControlledActionAttempt{}, ErrConfirmationInvalid
		}
		return ControlledActionAttempt{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE browser_rpa_final_confirmations SET consumed_at=?,version=version+1 WHERE organization_id=? AND project_id=? AND id=? AND version=? AND consumed_at IS NULL AND rejected_at IS NULL AND invalidated_at IS NULL`, now, identity.OrganizationID, identity.ProjectID, identity.ID, confirmation.Version)
	if err != nil {
		return ControlledActionAttempt{}, err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return ControlledActionAttempt{}, ErrConfirmationInvalid
	}
	if err := tx.Commit(); err != nil {
		return ControlledActionAttempt{}, err
	}
	return attempt, nil
}

func (r MySQLRepository) CompleteControlledAction(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, attemptID, status string) error {
	result, err := r.DB.ExecContext(ctx, `UPDATE browser_rpa_controlled_action_attempts SET status=? WHERE organization_id=? AND project_id=? AND id=? AND status=?`, status, org, project, attemptID, ControlledActionAuthorized)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrVersionConflict
	}
	return nil
}

func (r MySQLRepository) AuthorizeTakeoverAction(ctx context.Context, run BrowserRpaRun, expected int64, identity FinalConfirmation, digest string, lease SessionLease, attempt ControlledActionAttempt, step RunStep, evidence Evidence, event RunEvent, now time.Time) (BrowserRpaRun, ControlledActionAttempt, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return BrowserRpaRun{}, ControlledActionAttempt{}, err
	}
	defer tx.Rollback()
	var state RunState
	var paused, takeover bool
	var version int64
	var leaseID string
	err = tx.QueryRowContext(ctx, `SELECT state,paused,takeover_active,version,COALESCE(lease_id,'') FROM browser_rpa_runs WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, run.OrganizationID, run.ProjectID, run.ID).Scan(&state, &paused, &takeover, &version, &leaseID)
	if err != nil {
		return BrowserRpaRun{}, ControlledActionAttempt{}, ErrNotFound
	}
	if version != expected || state != RunAwaitingTakeover || !paused || !takeover || leaseID != lease.ID {
		return BrowserRpaRun{}, ControlledActionAttempt{}, ErrVersionConflict
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM browser_rpa_kill_switches WHERE active=TRUE AND ((scope='global' AND scope_key='*') OR (scope='platform' AND scope_key=?) OR (scope='organization' AND scope_key=?)) FOR UPDATE`, lease.Platform, identity.OrganizationID)
	if err != nil {
		return BrowserRpaRun{}, ControlledActionAttempt{}, err
	}
	active := rows.Next()
	rows.Close()
	if active {
		return BrowserRpaRun{}, ControlledActionAttempt{}, ErrKillSwitchActive
	}
	storedLease, err := scanLease(tx.QueryRowContext(ctx, leaseSelect+` WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, identity.OrganizationID, identity.ProjectID, lease.ID))
	if err != nil || storedLease.RunID != identity.RunID || storedLease.FencingToken != lease.FencingToken || !storedLease.ValidAt(now) {
		return BrowserRpaRun{}, ControlledActionAttempt{}, ErrLeaseUnavailable
	}
	var confirmation FinalConfirmation
	var consumed, rejected, invalidated sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT id,organization_id,project_id,run_id,binding_hash,token_digest,issued_by,issued_at,expires_at,consumed_at,rejected_at,invalidated_at,version FROM browser_rpa_final_confirmations WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, identity.OrganizationID, identity.ProjectID, identity.ID).Scan(&confirmation.ID, &confirmation.OrganizationID, &confirmation.ProjectID, &confirmation.RunID, &confirmation.BindingHash, &confirmation.TokenDigest, &confirmation.IssuedBy, &confirmation.IssuedAt, &confirmation.ExpiresAt, &consumed, &rejected, &invalidated, &confirmation.Version)
	if err != nil {
		return BrowserRpaRun{}, ControlledActionAttempt{}, ErrConfirmationInvalid
	}
	confirmation.SchemaVersion, confirmation.ConsumedAt, confirmation.RejectedAt, confirmation.InvalidatedAt = ConfirmationSchemaV1, timePtr(consumed), timePtr(rejected), timePtr(invalidated)
	if confirmation.RunID != identity.RunID || confirmation.BindingHash != identity.BindingHash || confirmation.TokenDigest != digest || !confirmation.UsableAt(now) {
		return BrowserRpaRun{}, ControlledActionAttempt{}, ErrConfirmationInvalid
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO browser_rpa_run_steps (id,organization_id,project_id,run_id,sequence_number,workflow_step_id,action,status,blocking_reason,attempt,version) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, step.ID, run.OrganizationID, run.ProjectID, run.ID, step.Sequence, step.WorkflowStepID, step.Action, step.Status, nil, step.Attempt, step.Version); err != nil {
		return BrowserRpaRun{}, ControlledActionAttempt{}, ErrIdempotencyConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO browser_rpa_controlled_action_attempts (id,organization_id,project_id,run_id,step_id,confirmation_id,approval_id,lease_id,fencing_token,action_hash,idempotency_key,status,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, attempt.ID, attempt.OrganizationID, attempt.ProjectID, attempt.RunID, attempt.StepID, attempt.ConfirmationID, attempt.ApprovalID, attempt.LeaseID, attempt.FencingToken, attempt.ActionHash, attempt.IdempotencyKey, attempt.Status, attempt.CreatedAt)
	if err != nil {
		return BrowserRpaRun{}, ControlledActionAttempt{}, ErrIdempotencyConflict
	}
	result, err := tx.ExecContext(ctx, `UPDATE browser_rpa_final_confirmations SET consumed_at=?,version=version+1 WHERE organization_id=? AND project_id=? AND id=? AND version=? AND consumed_at IS NULL AND rejected_at IS NULL AND invalidated_at IS NULL`, now, identity.OrganizationID, identity.ProjectID, identity.ID, confirmation.Version)
	if err != nil {
		return BrowserRpaRun{}, ControlledActionAttempt{}, err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return BrowserRpaRun{}, ControlledActionAttempt{}, ErrConfirmationInvalid
	}
	result, err = tx.ExecContext(ctx, `UPDATE browser_rpa_runs SET state='submitting',blocking_reason=NULL,version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND version=? AND state='awaiting_takeover' AND paused=TRUE AND takeover_active=TRUE`, now, run.OrganizationID, run.ProjectID, run.ID, expected)
	if err != nil {
		return BrowserRpaRun{}, ControlledActionAttempt{}, err
	}
	affected, _ = result.RowsAffected()
	if affected != 1 {
		return BrowserRpaRun{}, ControlledActionAttempt{}, ErrVersionConflict
	}
	payload, err := json.Marshal(evidence)
	if err != nil {
		return BrowserRpaRun{}, ControlledActionAttempt{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO browser_rpa_evidence (id,organization_id,project_id,run_id,step_id,evidence_json,object_fingerprint,skill_version,selector_version,action_version,redaction_version,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, evidence.ID, evidence.OrganizationID, evidence.ProjectID, evidence.RunID, evidence.StepID, payload, evidence.ObjectFingerprint, evidence.SkillVersion, evidence.SelectorVersion, evidence.ActionVersion, evidence.RedactionVersion, evidence.CreatedAt); err != nil {
		return BrowserRpaRun{}, ControlledActionAttempt{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO browser_rpa_events (id,organization_id,project_id,run_id,sequence_number,kind,summary,actor,created_at) VALUES (?,?,?,?,?,?,?,?,?)`, event.ID, event.OrganizationID, event.ProjectID, event.RunID, event.Sequence, event.Kind, event.Summary, event.Actor, event.CreatedAt); err != nil {
		return BrowserRpaRun{}, ControlledActionAttempt{}, err
	}
	if err := tx.Commit(); err != nil {
		return BrowserRpaRun{}, ControlledActionAttempt{}, err
	}
	updated, err := r.GetRun(ctx, run.OrganizationID, run.ProjectID, run.ID)
	return updated, attempt, err
}

func (r MySQLRepository) AppendEvent(ctx context.Context, value RunEvent) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO browser_rpa_events (id,organization_id,project_id,run_id,sequence_number,kind,summary,actor,created_at) VALUES (?,?,?,?,?,?,?,?,?)`, value.ID, value.OrganizationID, value.ProjectID, value.RunID, value.Sequence, value.Kind, value.Summary, value.Actor, value.CreatedAt)
	return err
}
func (r MySQLRepository) AppendEvidence(ctx context.Context, value Evidence) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = r.DB.ExecContext(ctx, `INSERT INTO browser_rpa_evidence (id,organization_id,project_id,run_id,step_id,evidence_json,object_fingerprint,skill_version,selector_version,action_version,redaction_version,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.OrganizationID, value.ProjectID, value.RunID, value.StepID, payload, value.ObjectFingerprint, value.SkillVersion, value.SelectorVersion, value.ActionVersion, value.RedactionVersion, value.CreatedAt)
	return err
}

func (r MySQLRepository) RecordTakeoverEvidence(ctx context.Context, run BrowserRpaRun, expected int64, step RunStep, evidence Evidence, event RunEvent, now time.Time) (BrowserRpaRun, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE browser_rpa_runs SET version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND version=? AND state='awaiting_takeover' AND paused=TRUE AND takeover_active=TRUE`, now, run.OrganizationID, run.ProjectID, run.ID, expected)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return BrowserRpaRun{}, err
	}
	if affected != 1 {
		return BrowserRpaRun{}, ErrVersionConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO browser_rpa_run_steps (id,organization_id,project_id,run_id,sequence_number,workflow_step_id,action,status,blocking_reason,attempt,version) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, step.ID, run.OrganizationID, run.ProjectID, run.ID, step.Sequence, step.WorkflowStepID, step.Action, step.Status, nullableString(string(step.BlockingReason)), step.Attempt, step.Version)
	if err != nil {
		return BrowserRpaRun{}, ErrIdempotencyConflict
	}
	payload, err := json.Marshal(evidence)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO browser_rpa_evidence (id,organization_id,project_id,run_id,step_id,evidence_json,object_fingerprint,skill_version,selector_version,action_version,redaction_version,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, evidence.ID, evidence.OrganizationID, evidence.ProjectID, evidence.RunID, evidence.StepID, payload, evidence.ObjectFingerprint, evidence.SkillVersion, evidence.SelectorVersion, evidence.ActionVersion, evidence.RedactionVersion, evidence.CreatedAt)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO browser_rpa_events (id,organization_id,project_id,run_id,sequence_number,kind,summary,actor,created_at) VALUES (?,?,?,?,?,?,?,?,?)`, event.ID, event.OrganizationID, event.ProjectID, event.RunID, event.Sequence, event.Kind, event.Summary, event.Actor, event.CreatedAt)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	if err := tx.Commit(); err != nil {
		return BrowserRpaRun{}, err
	}
	return r.GetRun(ctx, run.OrganizationID, run.ProjectID, run.ID)
}

func (r MySQLRepository) RecordTakeoverOutcome(ctx context.Context, run BrowserRpaRun, expected int64, attemptID, attemptStatus string, next RunState, reason BlockingReason, step RunStep, evidence Evidence, event RunEvent, now time.Time) (BrowserRpaRun, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	defer tx.Rollback()
	var storedAttemptID, storedLeaseID, storedAttemptStatus string
	var storedFencingToken int64
	if err := tx.QueryRowContext(ctx, `SELECT id,lease_id,fencing_token,status FROM browser_rpa_controlled_action_attempts WHERE organization_id=? AND project_id=? AND id=? AND run_id=? FOR UPDATE`, run.OrganizationID, run.ProjectID, attemptID, run.ID).Scan(&storedAttemptID, &storedLeaseID, &storedFencingToken, &storedAttemptStatus); err != nil {
		return BrowserRpaRun{}, ErrNotFound
	}
	if (storedAttemptStatus != ControlledActionAuthorized && storedAttemptStatus != ControlledActionVerified) || (attemptStatus != ControlledActionVerified && attemptStatus != ControlledActionFailed && attemptStatus != ControlledActionResultUnknown) {
		return BrowserRpaRun{}, ErrInvalidTransition
	}
	currentLease, err := scanLease(tx.QueryRowContext(ctx, leaseSelect+` WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, run.OrganizationID, run.ProjectID, run.LeaseID))
	if err != nil || currentLease.RunID != run.ID || !currentLease.ValidAt(now) {
		return BrowserRpaRun{}, ErrLeaseUnavailable
	}
	if storedAttemptStatus != attemptStatus {
		result, err := tx.ExecContext(ctx, `UPDATE browser_rpa_controlled_action_attempts SET status=? WHERE organization_id=? AND project_id=? AND id=? AND status=?`, attemptStatus, run.OrganizationID, run.ProjectID, attemptID, storedAttemptStatus)
		if err != nil {
			return BrowserRpaRun{}, err
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			return BrowserRpaRun{}, ErrVersionConflict
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE browser_rpa_runs SET state=?,blocking_reason=?,version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND version=? AND state=? AND paused=TRUE AND takeover_active=TRUE`, next, nullableString(string(reason)), now, run.OrganizationID, run.ProjectID, run.ID, expected, run.State)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return BrowserRpaRun{}, ErrVersionConflict
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO browser_rpa_run_steps (id,organization_id,project_id,run_id,sequence_number,workflow_step_id,action,status,blocking_reason,attempt,version) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, step.ID, run.OrganizationID, run.ProjectID, run.ID, step.Sequence, step.WorkflowStepID, step.Action, step.Status, nullableString(string(step.BlockingReason)), step.Attempt, step.Version); err != nil {
		return BrowserRpaRun{}, ErrIdempotencyConflict
	}
	payload, err := json.Marshal(evidence)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO browser_rpa_evidence (id,organization_id,project_id,run_id,step_id,evidence_json,object_fingerprint,skill_version,selector_version,action_version,redaction_version,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, evidence.ID, evidence.OrganizationID, evidence.ProjectID, evidence.RunID, evidence.StepID, payload, evidence.ObjectFingerprint, evidence.SkillVersion, evidence.SelectorVersion, evidence.ActionVersion, evidence.RedactionVersion, evidence.CreatedAt); err != nil {
		return BrowserRpaRun{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO browser_rpa_events (id,organization_id,project_id,run_id,sequence_number,kind,summary,actor,created_at) VALUES (?,?,?,?,?,?,?,?,?)`, event.ID, event.OrganizationID, event.ProjectID, event.RunID, event.Sequence, event.Kind, event.Summary, event.Actor, event.CreatedAt); err != nil {
		return BrowserRpaRun{}, err
	}
	if err := tx.Commit(); err != nil {
		return BrowserRpaRun{}, err
	}
	return r.GetRun(ctx, run.OrganizationID, run.ProjectID, run.ID)
}
func (r MySQLRepository) ListEvents(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, runID string) ([]RunEvent, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT id,organization_id,project_id,run_id,sequence_number,kind,summary,actor,created_at FROM browser_rpa_events WHERE organization_id=? AND project_id=? AND run_id=? ORDER BY sequence_number`, org, project, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []RunEvent{}
	for rows.Next() {
		var v RunEvent
		if err := rows.Scan(&v.ID, &v.OrganizationID, &v.ProjectID, &v.RunID, &v.Sequence, &v.Kind, &v.Summary, &v.Actor, &v.CreatedAt); err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, rows.Err()
}
func (r MySQLRepository) ListEvidence(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, runID string) ([]Evidence, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT evidence_json FROM browser_rpa_evidence WHERE organization_id=? AND project_id=? AND run_id=? ORDER BY created_at,id`, org, project, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []Evidence{}
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var v Evidence
		if err := json.Unmarshal(payload, &v); err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, rows.Err()
}

const runSelect = `SELECT id,organization_id,project_id,platform,account_id,execution_driver,authority_json,environment_id,profile_id,COALESCE(lease_id,''),policy_id,state,COALESCE(blocking_reason,''),paused,takeover_active,version,idempotency_key,request_hash,created_by,created_at,updated_at FROM browser_rpa_runs`

func scanRun(row interface{ Scan(...any) error }) (BrowserRpaRun, error) {
	var v BrowserRpaRun
	var authority []byte
	err := row.Scan(&v.ID, &v.OrganizationID, &v.ProjectID, &v.Platform, &v.AccountID, &v.ExecutionDriver, &authority, &v.EnvironmentID, &v.ProfileID, &v.LeaseID, &v.PolicyID, &v.State, &v.BlockingReason, &v.Paused, &v.TakeoverActive, &v.Version, &v.IdempotencyKey, &v.RequestHash, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BrowserRpaRun{}, ErrNotFound
	}
	if err != nil {
		return BrowserRpaRun{}, err
	}
	if err := json.Unmarshal(authority, &v.Authority); err != nil {
		return BrowserRpaRun{}, fmt.Errorf("decode browser-rpa authority: %w", err)
	}
	return v, nil
}

const leaseSelect = `SELECT id,organization_id,project_id,run_id,environment_id,profile_id,platform,account_id,holder,fencing_token,version,expires_at,heartbeat_deadline,released_at FROM browser_rpa_session_leases`

func scanLease(row interface{ Scan(...any) error }) (SessionLease, error) {
	var v SessionLease
	var released sql.NullTime
	err := row.Scan(&v.ID, &v.OrganizationID, &v.ProjectID, &v.RunID, &v.EnvironmentID, &v.ProfileID, &v.Platform, &v.AccountID, &v.Holder, &v.FencingToken, &v.Version, &v.ExpiresAt, &v.HeartbeatDeadline, &released)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionLease{}, ErrNotFound
	}
	if err != nil {
		return SessionLease{}, err
	}
	v.ReleasedAt = timePtr(released)
	return v, nil
}

const killSelect = `SELECT id,scope,organization_id,platform,active,reason,version,updated_by,updated_at FROM browser_rpa_kill_switches`

func scanKill(row interface{ Scan(...any) error }) (KillSwitch, error) {
	var v KillSwitch
	var org, platform sql.NullString
	err := row.Scan(&v.ID, &v.Scope, &org, &platform, &v.Active, &v.Reason, &v.Version, &v.UpdatedBy, &v.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return KillSwitch{}, ErrNotFound
	}
	if err != nil {
		return KillSwitch{}, err
	}
	v.OrganizationID = contract.OrganizationID(org.String)
	v.Platform = Platform(platform.String)
	return v, nil
}
func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func timePtr(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	value := v.Time
	return &value
}
