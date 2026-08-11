package delivery

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

type MySQLRepository struct {
	DB *sql.DB
}

func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func nullablePositiveInt(v int) any {
	if v < 1 {
		return nil
	}
	return v
}
func nullableJSON(v any) any {
	if v == nil || (reflect.ValueOf(v).Kind() == reflect.Pointer && reflect.ValueOf(v).IsNil()) {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

func (r MySQLRepository) CreatePlan(ctx context.Context, value DeliveryPlan, version DeliveryPlanVersion) (DeliveryPlan, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return DeliveryPlan{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO delivery_plans (
		id, organization_id, project_id, creative_package_id, creative_package_hash, creative_version_id,
		name, objective, budget_cents, start_at, end_at, status, version, platform, source, scenario,
		tour_run_id, tour_owner_id, tour_case, current_version, created_by, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID, value.OrganizationID, value.ProjectID, value.CreativePackageID, value.CreativePackageHash,
		value.CreativeVersionID, value.Name, value.Objective, value.BudgetCents, value.StartAt, value.EndAt,
		value.Status, value.Version, value.Platform, value.Source, value.Scenario, nullableString(value.TourRunID), nullableString(value.TourOwnerID), nullableString(value.TourCase), value.CurrentVersionNumber,
		value.CreatedBy, value.CreatedAt, value.UpdatedAt)
	if err != nil {
		return DeliveryPlan{}, err
	}
	if err := insertPlanVersion(ctx, tx, version); err != nil {
		return DeliveryPlan{}, err
	}
	if err := tx.Commit(); err != nil {
		return DeliveryPlan{}, err
	}
	value.CurrentVersion = cloneVersion(version)
	value.Versions = []DeliveryPlanVersion{cloneVersion(version)}
	return value, nil
}

func (r MySQLRepository) ListPlans(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, limit int) ([]DeliveryPlan, error) {
	rows, err := r.DB.QueryContext(ctx, deliveryPlanSelect+` WHERE organization_id = ? AND project_id = ? ORDER BY updated_at DESC, id DESC LIMIT ?`, organizationID, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]DeliveryPlan, 0)
	for rows.Next() {
		value, scanErr := scanDeliveryPlan(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		if hydrateErr := r.hydratePlan(ctx, &value); hydrateErr != nil {
			return nil, hydrateErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r MySQLRepository) GetPlan(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, id string) (DeliveryPlan, error) {
	value, err := scanDeliveryPlan(r.DB.QueryRowContext(ctx, deliveryPlanSelect+` WHERE organization_id = ? AND project_id = ? AND id = ?`, organizationID, projectID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return DeliveryPlan{}, ErrNotFound
	}
	if err == nil {
		err = r.hydratePlan(ctx, &value)
	}
	return value, err
}

func (r MySQLRepository) UpdatePlan(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, id string, expectedVersion int, version DeliveryPlanVersion) (DeliveryPlan, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return DeliveryPlan{}, err
	}
	defer tx.Rollback()
	var current int
	err = tx.QueryRowContext(ctx, `SELECT current_version FROM delivery_plans
		WHERE organization_id = ? AND project_id = ? AND id = ? FOR UPDATE`,
		organizationID, projectID, id).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return DeliveryPlan{}, ErrNotFound
	}
	if err != nil {
		return DeliveryPlan{}, err
	}
	if current != expectedVersion {
		return DeliveryPlan{}, ErrPlanVersionConflict
	}
	if err := insertPlanVersion(ctx, tx, version); err != nil {
		return DeliveryPlan{}, err
	}
	startAt, endAt := versionSchedule(version)
	result, err := tx.ExecContext(ctx, `UPDATE delivery_plans SET
		creative_package_id = ?, creative_package_hash = ?, creative_version_id = ?,
		name = ?, objective = ?, budget_cents = ?, start_at = ?, end_at = ?,
		version = ?, source = ?, scenario = ?, current_version = ?, updated_at = ?
		WHERE organization_id = ? AND project_id = ? AND id = ? AND current_version = ?`,
		firstCreativeID(version), firstCreativeHash(version), firstCreativeVersion(version),
		versionName(version), versionObjective(version), versionBudget(version).TotalMinor, startAt, endAt,
		version.VersionNumber, version.Source, version.Scenario, version.VersionNumber, version.CreatedAt,
		organizationID, projectID, id, expectedVersion)
	if err != nil {
		return DeliveryPlan{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return DeliveryPlan{}, err
	}
	if affected != 1 {
		return DeliveryPlan{}, ErrPlanVersionConflict
	}
	if err := tx.Commit(); err != nil {
		return DeliveryPlan{}, err
	}
	return r.GetPlan(ctx, organizationID, projectID, id)
}

func (r MySQLRepository) ListPlanVersions(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, planID string) ([]DeliveryPlanVersion, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT config_json, canonical_hash FROM delivery_plan_versions
		WHERE organization_id = ? AND project_id = ? AND plan_id = ?
		ORDER BY version_number ASC`, organizationID, projectID, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]DeliveryPlanVersion, 0)
	for rows.Next() {
		var payload []byte
		var canonicalHash string
		if err := rows.Scan(&payload, &canonicalHash); err != nil {
			return nil, err
		}
		value, err := decodePlanVersion(payload, canonicalHash)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r MySQLRepository) GetPlanVersion(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, planID string, version int) (DeliveryPlanVersion, error) {
	var payload []byte
	var canonicalHash string
	err := r.DB.QueryRowContext(ctx, `SELECT config_json, canonical_hash FROM delivery_plan_versions
		WHERE organization_id = ? AND project_id = ? AND plan_id = ? AND version_number = ?`,
		organizationID, projectID, planID, version).Scan(&payload, &canonicalHash)
	if errors.Is(err, sql.ErrNoRows) {
		return DeliveryPlanVersion{}, ErrNotFound
	}
	if err != nil {
		return DeliveryPlanVersion{}, err
	}
	return decodePlanVersion(payload, canonicalHash)
}

func (r MySQLRepository) hydratePlan(ctx context.Context, value *DeliveryPlan) error {
	versions, err := r.ListPlanVersions(ctx, value.OrganizationID, value.ProjectID, value.ID)
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		return fmt.Errorf("delivery plan %s has no immutable version", value.ID)
	}
	value.Versions = versions
	value.CurrentVersion = cloneVersion(versions[len(versions)-1])
	value.CurrentVersionNumber = value.CurrentVersion.VersionNumber
	return nil
}

type planVersionExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func insertPlanVersion(ctx context.Context, executor planVersionExecutor, version DeliveryPlanVersion) error {
	if version.IsPlatformConfigurationV2() {
		intentJSON, err := json.Marshal(version.DeliveryIntent)
		if err != nil {
			return err
		}
		configurationJSON, err := json.Marshal(version.PlatformConfiguration)
		if err != nil {
			return err
		}
		intent := version.DeliveryIntent
		if _, err := executor.ExecContext(ctx, `INSERT INTO delivery_intents (
			organization_id, project_id, intent_id, version_number, schema_version, canonical_hash, hash_algorithm, intent_json, created_by, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE intent_id=VALUES(intent_id)`, version.OrganizationID, version.ProjectID, intent.IntentID, intent.VersionNumber, intent.SchemaVersion, intent.CanonicalHash, intent.HashAlgorithm, intentJSON, version.CreatedBy.ID, version.CreatedAt); err != nil {
			return err
		}
		var storedIntentSchema, storedIntentHash, storedIntentAlgorithm string
		var storedIntentJSON []byte
		if err := executor.QueryRowContext(ctx, `SELECT schema_version,canonical_hash,hash_algorithm,intent_json FROM delivery_intents WHERE organization_id=? AND project_id=? AND intent_id=? AND version_number=?`, version.OrganizationID, version.ProjectID, intent.IntentID, intent.VersionNumber).Scan(&storedIntentSchema, &storedIntentHash, &storedIntentAlgorithm, &storedIntentJSON); err != nil {
			return err
		}
		if storedIntentSchema != intent.SchemaVersion || storedIntentHash != intent.CanonicalHash || storedIntentAlgorithm != intent.HashAlgorithm || !equalJSONDocuments(storedIntentJSON, intentJSON) {
			return fmt.Errorf("delivery intent immutable identity conflict")
		}
		configuration := version.PlatformConfiguration
		if _, err := executor.ExecContext(ctx, `INSERT INTO delivery_platform_configurations (
			organization_id, project_id, configuration_id, version_number, schema_version, platform, profile_version,
			intent_id, intent_version, intent_canonical_hash, canonical_hash, hash_algorithm, configuration_json, created_by, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE configuration_id=VALUES(configuration_id)`, version.OrganizationID, version.ProjectID, configuration.ConfigurationID, configuration.VersionNumber, configuration.SchemaVersion, configuration.Platform, configuration.ProfileVersion, configuration.Intent.IntentID, configuration.Intent.VersionNumber, configuration.Intent.CanonicalHash, configuration.CanonicalHash, configuration.HashAlgorithm, configurationJSON, version.CreatedBy.ID, version.CreatedAt); err != nil {
			return err
		}
		var storedConfigurationSchema, storedPlatform, storedProfile, storedConfigurationIntentID, storedConfigurationIntentHash, storedConfigurationHash, storedConfigurationAlgorithm string
		var storedIntentVersion int
		var storedConfigurationJSON []byte
		if err := executor.QueryRowContext(ctx, `SELECT schema_version,platform,profile_version,intent_id,intent_version,intent_canonical_hash,canonical_hash,hash_algorithm,configuration_json FROM delivery_platform_configurations WHERE organization_id=? AND project_id=? AND configuration_id=? AND version_number=?`, version.OrganizationID, version.ProjectID, configuration.ConfigurationID, configuration.VersionNumber).Scan(&storedConfigurationSchema, &storedPlatform, &storedProfile, &storedConfigurationIntentID, &storedIntentVersion, &storedConfigurationIntentHash, &storedConfigurationHash, &storedConfigurationAlgorithm, &storedConfigurationJSON); err != nil {
			return err
		}
		if storedConfigurationSchema != configuration.SchemaVersion || storedPlatform != string(configuration.Platform) || storedProfile != configuration.ProfileVersion || storedConfigurationIntentID != configuration.Intent.IntentID || storedIntentVersion != configuration.Intent.VersionNumber || storedConfigurationIntentHash != configuration.Intent.CanonicalHash || storedConfigurationHash != configuration.CanonicalHash || storedConfigurationAlgorithm != configuration.HashAlgorithm || !equalJSONDocuments(storedConfigurationJSON, configurationJSON) {
			return fmt.Errorf("delivery platform configuration immutable identity conflict")
		}
	}
	payload, err := json.Marshal(version)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `INSERT INTO delivery_plan_versions (
		organization_id, project_id, plan_id, version_number, config_json, canonical_hash, payload_schema_version, source, scenario,
		created_by_kind, created_by_id, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		version.OrganizationID, version.ProjectID, version.PlanID, version.VersionNumber, payload,
		version.CanonicalHash, nullableString(version.SchemaVersion), version.Source, version.Scenario, version.CreatedBy.Kind, version.CreatedBy.ID, version.CreatedAt)
	return err
}

func equalJSONDocuments(left, right []byte) bool {
	var leftValue, rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func decodePlanVersion(payload []byte, canonicalHash string) (DeliveryPlanVersion, error) {
	var value DeliveryPlanVersion
	if err := json.Unmarshal(payload, &value); err != nil {
		return DeliveryPlanVersion{}, fmt.Errorf("decode delivery plan version: %w", err)
	}
	value.CanonicalHash = canonicalHash
	if value.IsLegacy() {
		value.RuntimeStatus = PlanRuntimeLegacyUnsupported
		value.ReadOnly = true
	}
	calculated, err := PlanCanonicalHash(value)
	if err != nil {
		return DeliveryPlanVersion{}, err
	}
	if calculated != canonicalHash {
		return DeliveryPlanVersion{}, fmt.Errorf("delivery plan version canonical hash mismatch")
	}
	return value, nil
}

func firstCreativeID(version DeliveryPlanVersion) string {
	if version.IsPlatformConfigurationV2() && len(version.DeliveryIntent.Payload.MaterialReferences) > 0 {
		if id := version.DeliveryIntent.Payload.MaterialReferences[0].ID; id != "" {
			return id
		}
	}
	if len(version.CreativeReferences) == 0 {
		return "mock-unset"
	}
	return version.CreativeReferences[0].AssetID
}

func firstCreativeVersion(version DeliveryPlanVersion) string {
	if version.IsPlatformConfigurationV2() && len(version.DeliveryIntent.Payload.MaterialReferences) > 0 {
		if value := version.DeliveryIntent.Payload.MaterialReferences[0].Version; value != "" {
			return value
		}
	}
	if len(version.CreativeReferences) == 0 {
		return "0"
	}
	return fmt.Sprint(version.CreativeReferences[0].Version)
}

func firstCreativeHash(version DeliveryPlanVersion) string {
	if version.IsPlatformConfigurationV2() && len(version.DeliveryIntent.Payload.MaterialReferences) > 0 {
		if value := version.DeliveryIntent.Payload.MaterialReferences[0].ContentHash; value != "" {
			return value
		}
	}
	if len(version.CreativeReferences) > 0 && version.CreativeReferences[0].ContentHash != "" {
		return version.CreativeReferences[0].ContentHash
	}
	return fmt.Sprintf("mock:%s@%s", firstCreativeID(version), firstCreativeVersion(version))
}

func (r MySQLRepository) CreateChangeSet(ctx context.Context, value ChangeSet) (ChangeSet, error) {
	notes, err := json.Marshal(value.PreflightNotes)
	if err != nil {
		return ChangeSet{}, err
	}
	_, err = r.DB.ExecContext(ctx, `INSERT INTO delivery_change_sets (
		id, organization_id, project_id, plan_id, plan_version, status, risk_level, preflight_notes, target_snapshot, target_snapshot_hash, target_snapshot_schema_version, recommendation_id,
		approved_by, approved_at, rejected_by, rejected_at, rejection_reason, version, created_by, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, NULL, ?, ?, ?, ?)`,
		value.ID, value.OrganizationID, value.ProjectID, value.PlanID, value.PlanVersion, value.Status,
		value.RiskLevel, notes, changeSetSnapshotJSON(value), nullableString(value.TargetSnapshotHash), nullableString(changeSetSnapshotSchema(value)), nullableString(value.RecommendationID), value.Version, value.CreatedBy, value.CreatedAt, value.UpdatedAt)
	return value, err
}

func (r MySQLRepository) RejectChangeSet(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, id string, expectedVersion int64, actorID, reason string, now time.Time) (ChangeSet, error) {
	result, err := r.DB.ExecContext(ctx, `UPDATE delivery_change_sets SET status = ?, rejected_by = ?, rejected_at = ?, rejection_reason = ?, version = version + 1, updated_at = ? WHERE organization_id = ? AND project_id = ? AND id = ? AND version = ? AND status = ?`,
		ChangeSetRejected, actorID, now, reason, now, organizationID, projectID, id, expectedVersion, ChangeSetPreflightPassed)
	if err != nil {
		return ChangeSet{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ChangeSet{}, err
	}
	if affected != 1 {
		if _, getErr := r.GetChangeSet(ctx, organizationID, projectID, id); getErr != nil {
			return ChangeSet{}, getErr
		}
		return ChangeSet{}, ErrVersionConflict
	}
	return r.GetChangeSet(ctx, organizationID, projectID, id)
}

func (r MySQLRepository) ListChangeSets(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, limit int) ([]ChangeSet, error) {
	rows, err := r.DB.QueryContext(ctx, changeSetSelect+` WHERE organization_id = ? AND project_id = ? ORDER BY updated_at DESC, id DESC LIMIT ?`, organizationID, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]ChangeSet, 0)
	for rows.Next() {
		value, scanErr := scanChangeSet(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r MySQLRepository) GetChangeSet(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, id string) (ChangeSet, error) {
	value, err := scanChangeSet(r.DB.QueryRowContext(ctx, changeSetSelect+` WHERE organization_id = ? AND project_id = ? AND id = ?`, organizationID, projectID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return ChangeSet{}, ErrNotFound
	}
	return value, err
}

func (r MySQLRepository) TransitionChangeSet(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, id string, expectedVersion int64, next ChangeSetStatus, actorID string, now time.Time) (ChangeSet, error) {
	var result sql.Result
	var err error
	if next == ChangeSetApproved {
		result, err = r.DB.ExecContext(ctx, `UPDATE delivery_change_sets SET status = ?, approved_by = ?, approved_at = ?, version = version + 1, updated_at = ? WHERE organization_id = ? AND project_id = ? AND id = ? AND version = ?`,
			next, actorID, now, now, organizationID, projectID, id, expectedVersion)
	} else {
		result, err = r.DB.ExecContext(ctx, `UPDATE delivery_change_sets SET status = ?, version = version + 1, updated_at = ? WHERE organization_id = ? AND project_id = ? AND id = ? AND version = ?`,
			next, now, organizationID, projectID, id, expectedVersion)
	}
	if err != nil {
		return ChangeSet{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ChangeSet{}, err
	}
	if affected == 0 {
		if _, getErr := r.GetChangeSet(ctx, organizationID, projectID, id); getErr != nil {
			return ChangeSet{}, getErr
		}
		return ChangeSet{}, ErrVersionConflict
	}
	return r.GetChangeSet(ctx, organizationID, projectID, id)
}

func (r MySQLRepository) ApproveChangeSet(ctx context.Context, changeSet ChangeSet, approval DeliveryApproval) (ChangeSet, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return ChangeSet{}, err
	}
	defer tx.Rollback()
	var currentPlanVersion int64
	err = tx.QueryRowContext(ctx, `SELECT current_version FROM delivery_plans
		WHERE organization_id = ? AND project_id = ? AND id = ? FOR UPDATE`,
		changeSet.OrganizationID, changeSet.ProjectID, changeSet.PlanID).Scan(&currentPlanVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return ChangeSet{}, ErrNotFound
	}
	if err != nil {
		return ChangeSet{}, err
	}
	if currentPlanVersion != changeSet.PlanVersion {
		return ChangeSet{}, ErrStalePlanVersion
	}
	var status ChangeSetStatus
	var version int64
	err = tx.QueryRowContext(ctx, `SELECT status, version FROM delivery_change_sets
		WHERE organization_id = ? AND project_id = ? AND id = ? FOR UPDATE`,
		changeSet.OrganizationID, changeSet.ProjectID, changeSet.ID).Scan(&status, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return ChangeSet{}, ErrNotFound
	}
	if err != nil {
		return ChangeSet{}, err
	}
	if status != ChangeSetPreflightPassed {
		return ChangeSet{}, ErrInvalidState
	}
	if version != changeSet.Version || approval.ChangeSetVersion != version+1 {
		return ChangeSet{}, ErrVersionConflict
	}
	if approval.OrganizationID != changeSet.OrganizationID ||
		approval.ProjectID != changeSet.ProjectID ||
		approval.PlanID != changeSet.PlanID ||
		approval.PlanVersion != changeSet.PlanVersion ||
		approval.ChangeSetID != changeSet.ID {
		return ChangeSet{}, ErrApprovalContentMismatch
	}
	actionHash, err := ApprovalActionHash(approval)
	if err != nil {
		return ChangeSet{}, err
	}
	if actionHash != approval.ActionHash {
		return ChangeSet{}, ErrApprovalContentMismatch
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO delivery_approvals (
		approval_id, organization_id, project_id, plan_id, plan_version,
		change_set_id, change_set_version, plan_canonical_hash, target_snapshot_hash, action_hash,
		configuration_schema_version, configuration_id, configuration_version, configuration_platform, configuration_profile_version, configuration_canonical_hash,
		intent_schema_version, intent_id, intent_version, intent_canonical_hash,
		action, scope, budget_limit_minor, currency, approved_by, approved_at,
		expires_at, source, scenario
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		approval.ApprovalID, approval.OrganizationID, approval.ProjectID, approval.PlanID,
		approval.PlanVersion, approval.ChangeSetID, approval.ChangeSetVersion,
		approval.PlanCanonicalHash, nullableString(approval.TargetSnapshotHash), approval.ActionHash,
		nullableString(approval.ConfigurationSchemaVersion), nullableString(approval.ConfigurationID), nullablePositiveInt(approval.ConfigurationVersion), nullableString(string(approval.ConfigurationPlatform)), nullableString(approval.ConfigurationProfileVersion), nullableString(approval.ConfigurationCanonicalHash),
		nullableString(approval.IntentSchemaVersion), nullableString(approval.IntentID), nullablePositiveInt(approval.IntentVersion), nullableString(approval.IntentCanonicalHash),
		approval.Action, approval.Scope,
		approval.BudgetLimitMinor, approval.Currency, approval.ApprovedBy, approval.ApprovedAt,
		approval.ExpiresAt, approval.Source, approval.Scenario)
	if err != nil {
		return ChangeSet{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE delivery_change_sets SET
		status = ?, approved_by = ?, approved_at = ?, version = ?, updated_at = ?
		WHERE organization_id = ? AND project_id = ? AND id = ? AND version = ? AND status = ?`,
		ChangeSetApproved, approval.ApprovedBy, approval.ApprovedAt, approval.ChangeSetVersion,
		approval.ApprovedAt, changeSet.OrganizationID, changeSet.ProjectID, changeSet.ID,
		changeSet.Version, ChangeSetPreflightPassed)
	if err != nil {
		return ChangeSet{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ChangeSet{}, err
	}
	if affected != 1 {
		return ChangeSet{}, ErrVersionConflict
	}
	if err := tx.Commit(); err != nil {
		return ChangeSet{}, err
	}
	changeSet.Status = ChangeSetApproved
	changeSet.ApprovedBy = approval.ApprovedBy
	approvedAt := approval.ApprovedAt
	changeSet.ApprovedAt = &approvedAt
	changeSet.Version = approval.ChangeSetVersion
	changeSet.UpdatedAt = approval.ApprovedAt
	return changeSet, nil
}

func (r MySQLRepository) GetApproval(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, changeSetID string) (DeliveryApproval, error) {
	value, err := scanApproval(r.DB.QueryRowContext(ctx, approvalSelect+`
		WHERE organization_id = ? AND project_id = ? AND change_set_id = ?`,
		organizationID, projectID, changeSetID))
	if errors.Is(err, sql.ErrNoRows) {
		return DeliveryApproval{}, ErrNotFound
	}
	return value, err
}

func (r MySQLRepository) RecordExecution(ctx context.Context, changeSet ChangeSet, approval DeliveryApproval, execution Execution, evidence Evidence) (ExecutionResult, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return ExecutionResult{}, err
	}
	defer tx.Rollback()
	var currentPlanVersion int64
	err = tx.QueryRowContext(ctx, `SELECT current_version FROM delivery_plans
		WHERE organization_id = ? AND project_id = ? AND id = ? FOR UPDATE`,
		changeSet.OrganizationID, changeSet.ProjectID, changeSet.PlanID).Scan(&currentPlanVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return ExecutionResult{}, ErrNotFound
	}
	if err != nil {
		return ExecutionResult{}, err
	}
	if currentPlanVersion != changeSet.PlanVersion {
		return ExecutionResult{}, ErrStalePlanVersion
	}
	var storedStatus ChangeSetStatus
	var storedVersion int64
	err = tx.QueryRowContext(ctx, `SELECT status, version FROM delivery_change_sets
		WHERE organization_id = ? AND project_id = ? AND id = ? FOR UPDATE`,
		changeSet.OrganizationID, changeSet.ProjectID, changeSet.ID).Scan(&storedStatus, &storedVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return ExecutionResult{}, ErrNotFound
	}
	if err != nil {
		return ExecutionResult{}, err
	}
	if storedStatus != ChangeSetApproved {
		return ExecutionResult{}, ErrInvalidState
	}
	if storedVersion != changeSet.Version {
		return ExecutionResult{}, ErrVersionConflict
	}
	storedApproval, err := scanApproval(tx.QueryRowContext(ctx, approvalSelect+`
		WHERE organization_id = ? AND project_id = ? AND change_set_id = ? FOR UPDATE`,
		changeSet.OrganizationID, changeSet.ProjectID, changeSet.ID))
	if errors.Is(err, sql.ErrNoRows) {
		return ExecutionResult{}, ErrApprovalRequired
	}
	if err != nil {
		return ExecutionResult{}, err
	}
	if !execution.StartedAt.Before(storedApproval.ExpiresAt) {
		return ExecutionResult{}, ErrApprovalExpired
	}
	if !sameApproval(storedApproval, approval) {
		return ExecutionResult{}, ErrApprovalContentMismatch
	}
	result, err := tx.ExecContext(ctx, `UPDATE delivery_change_sets SET status = ?, version = version + 1, updated_at = ? WHERE organization_id = ? AND project_id = ? AND id = ? AND version = ? AND status = ?`,
		ChangeSetExecuted, execution.StartedAt, changeSet.OrganizationID, changeSet.ProjectID, changeSet.ID, changeSet.Version, ChangeSetApproved)
	if err != nil {
		return ExecutionResult{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ExecutionResult{}, err
	}
	if affected == 0 {
		return ExecutionResult{}, ErrVersionConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO delivery_executions (id, organization_id, project_id, change_set_id, approval_id, status, version, execution_mode, adapter, source, scenario, idempotency_key, request_hash, executed_by, started_at, completed_at, retry_allowed, recovery_action, recovery_reason, compensation_candidates) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		execution.ID, execution.OrganizationID, execution.ProjectID, execution.ChangeSetID, execution.ApprovalID, execution.Status, execution.Version, execution.Mode, execution.Adapter, execution.Source, execution.Scenario, execution.IdempotencyKey, execution.RequestHash, execution.ExecutedBy, execution.StartedAt, execution.CompletedAt, execution.RetryAllowed, execution.RecoveryAction, execution.RecoveryReason, mustJSON(execution.CompensationCandidates))
	if err != nil {
		return ExecutionResult{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO delivery_evidence (id, organization_id, project_id, execution_id, summary, evidence_mode, reversible, source, scenario, references_json, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		evidence.ID, evidence.OrganizationID, evidence.ProjectID, evidence.ExecutionID, evidence.Summary, evidence.Mode, evidence.Reversible, evidence.Source, evidence.Scenario, mustJSON(evidence.References), evidence.CreatedAt)
	if err != nil {
		return ExecutionResult{}, err
	}
	for _, step := range execution.Steps {
		_, err = tx.ExecContext(ctx, `INSERT INTO delivery_execution_steps (id, organization_id, project_id, execution_id, sequence_number, action, status, attempt, effect, outcome_summary, evidence_ref, started_at, completed_at, version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			step.ID, execution.OrganizationID, execution.ProjectID, execution.ID, step.Sequence, step.Action, step.Status, step.Attempt, step.Effect, step.OutcomeSummary, step.EvidenceRef, step.StartedAt, step.CompletedAt, step.Version)
		if err != nil {
			return ExecutionResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ExecutionResult{}, err
	}
	changeSet.Status = ChangeSetExecuted
	changeSet.Version++
	changeSet.UpdatedAt = execution.StartedAt
	return ExecutionResult{ChangeSet: changeSet, Execution: execution, Evidence: evidence}, nil
}

// CreateOrReplayExecution preserves the immutable approval and makes a retry
// with the same idempotency key observationally identical to its first call.
func (r MySQLRepository) CreateOrReplayExecution(ctx context.Context, changeSet ChangeSet, approval DeliveryApproval, execution Execution, evidence Evidence) (ExecutionResult, bool, error) {
	var existingID, existingHash string
	err := r.DB.QueryRowContext(ctx, `SELECT id, request_hash FROM delivery_executions
		WHERE organization_id = ? AND project_id = ? AND idempotency_key = ?`,
		execution.OrganizationID, execution.ProjectID, execution.IdempotencyKey).Scan(&existingID, &existingHash)
	if err == nil {
		if existingHash != execution.RequestHash {
			return ExecutionResult{}, false, ErrIdempotencyConflict
		}
		value, getErr := r.GetExecution(ctx, execution.OrganizationID, execution.ProjectID, existingID)
		return value, true, getErr
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ExecutionResult{}, false, err
	}
	legacy, err := r.RecordExecution(ctx, changeSet, approval, execution, evidence)
	if err != nil {
		if replay, found, replayErr := r.FindExecutionByIdempotency(ctx, execution.OrganizationID, execution.ProjectID, execution.IdempotencyKey); replayErr == nil && found {
			if replay.Execution.RequestHash == execution.RequestHash {
				return replay, true, nil
			}
			return ExecutionResult{}, false, ErrIdempotencyConflict
		}
	}
	return legacy, false, err
}

func (r MySQLRepository) FindExecutionByIdempotency(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, key string) (ExecutionResult, bool, error) {
	var id string
	err := r.DB.QueryRowContext(ctx, `SELECT id FROM delivery_executions WHERE organization_id=? AND project_id=? AND idempotency_key=?`, organizationID, projectID, key).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ExecutionResult{}, false, nil
	}
	if err != nil {
		return ExecutionResult{}, false, err
	}
	value, err := r.GetExecution(ctx, organizationID, projectID, id)
	return value, err == nil, err
}

func (r MySQLRepository) AdvanceExecution(ctx context.Context, execution Execution, next ExecutionStatus, completed *time.Time, action, reason string, compensation []string) (ExecutionResult, error) {
	if !validExecutionTransition(execution.Status, next) {
		return ExecutionResult{}, ErrInvalidState
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return ExecutionResult{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE delivery_executions SET status=?, version=version+1, completed_at=?, recovery_action=?, recovery_reason=?, compensation_candidates=? WHERE organization_id=? AND project_id=? AND id=? AND version=? AND status=?`, next, completed, action, reason, mustJSON(compensation), execution.OrganizationID, execution.ProjectID, execution.ID, execution.Version, execution.Status)
	if err != nil {
		return ExecutionResult{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ExecutionResult{}, err
	}
	if affected != 1 {
		return ExecutionResult{}, ErrVersionConflict
	}
	if err = tx.Commit(); err != nil {
		return ExecutionResult{}, err
	}
	return r.GetExecution(ctx, execution.OrganizationID, execution.ProjectID, execution.ID)
}

func (r MySQLRepository) AdvanceStep(ctx context.Context, execution Execution, step ExecutionStep, next ExecutionStep) (ExecutionStep, error) {
	if step.ID != next.ID || step.Sequence != next.Sequence || step.Action != next.Action || !validStepTransition(step.Status, next.Status) {
		return ExecutionStep{}, ErrInvalidState
	}
	result, err := r.DB.ExecContext(ctx, `UPDATE delivery_execution_steps SET status=?, attempt=?, effect=?, outcome_summary=?, evidence_ref=?, started_at=?, completed_at=?, version=version+1 WHERE organization_id=? AND project_id=? AND execution_id=? AND id=? AND version=? AND status=?`, next.Status, next.Attempt, next.Effect, next.OutcomeSummary, next.EvidenceRef, next.StartedAt, next.CompletedAt, execution.OrganizationID, execution.ProjectID, execution.ID, step.ID, step.Version, step.Status)
	if err != nil {
		return ExecutionStep{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ExecutionStep{}, err
	}
	if affected != 1 {
		return ExecutionStep{}, ErrVersionConflict
	}
	next.Version = step.Version + 1
	return next, nil
}

func mustJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}

func (r MySQLRepository) GetExecution(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, id string) (ExecutionResult, error) {
	var value ExecutionResult
	var completedAt sql.NullTime
	var compensation, references []byte
	err := r.DB.QueryRowContext(ctx, `SELECT x.id, x.organization_id, x.project_id, x.change_set_id, x.approval_id, x.status, x.version, x.execution_mode, x.adapter, x.source, x.scenario, x.idempotency_key, x.request_hash, x.executed_by, x.started_at, x.completed_at, x.retry_allowed, x.recovery_action, x.recovery_reason, x.compensation_candidates, e.id, e.summary, e.evidence_mode, e.reversible, e.source, e.scenario, e.references_json, e.created_at FROM delivery_executions x JOIN delivery_evidence e ON e.organization_id=x.organization_id AND e.execution_id=x.id WHERE x.organization_id=? AND x.project_id=? AND x.id=?`, organizationID, projectID, id).Scan(
		&value.Execution.ID, &value.Execution.OrganizationID, &value.Execution.ProjectID, &value.Execution.ChangeSetID, &value.Execution.ApprovalID, &value.Execution.Status, &value.Execution.Version, &value.Execution.Mode, &value.Execution.Adapter, &value.Execution.Source, &value.Execution.Scenario, &value.Execution.IdempotencyKey, &value.Execution.RequestHash, &value.Execution.ExecutedBy, &value.Execution.StartedAt, &completedAt, &value.Execution.RetryAllowed, &value.Execution.RecoveryAction, &value.Execution.RecoveryReason, &compensation,
		&value.Evidence.ID, &value.Evidence.Summary, &value.Evidence.Mode, &value.Evidence.Reversible, &value.Evidence.Source, &value.Evidence.Scenario, &references, &value.Evidence.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ExecutionResult{}, ErrNotFound
	}
	if err != nil {
		return ExecutionResult{}, err
	}
	if completedAt.Valid {
		value.Execution.CompletedAt = &completedAt.Time
	}
	value.Evidence.OrganizationID, value.Evidence.ProjectID, value.Evidence.ExecutionID = organizationID, projectID, id
	_ = json.Unmarshal(compensation, &value.Execution.CompensationCandidates)
	_ = json.Unmarshal(references, &value.Evidence.References)
	rows, err := r.DB.QueryContext(ctx, `SELECT id, sequence_number, action, status, attempt, effect, outcome_summary, evidence_ref, started_at, completed_at, version FROM delivery_execution_steps WHERE organization_id=? AND project_id=? AND execution_id=? ORDER BY sequence_number`, organizationID, projectID, id)
	if err != nil {
		return ExecutionResult{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var step ExecutionStep
		var started, completed sql.NullTime
		if err := rows.Scan(&step.ID, &step.Sequence, &step.Action, &step.Status, &step.Attempt, &step.Effect, &step.OutcomeSummary, &step.EvidenceRef, &started, &completed, &step.Version); err != nil {
			return ExecutionResult{}, err
		}
		if started.Valid {
			step.StartedAt = &started.Time
		}
		if completed.Valid {
			step.CompletedAt = &completed.Time
		}
		value.Execution.Steps = append(value.Execution.Steps, step)
	}
	if err := rows.Err(); err != nil {
		return ExecutionResult{}, err
	}
	changeSet, err := r.GetChangeSet(ctx, organizationID, projectID, value.Execution.ChangeSetID)
	if err != nil {
		return ExecutionResult{}, err
	}
	value.ChangeSet = changeSet
	return value, nil
}

func (r MySQLRepository) GetExecutionByChangeSet(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, changeSetID string) (ExecutionResult, error) {
	var id string
	err := r.DB.QueryRowContext(ctx, `SELECT id FROM delivery_executions WHERE organization_id=? AND project_id=? AND change_set_id=?`, organizationID, projectID, changeSetID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ExecutionResult{}, ErrNotFound
	}
	if err != nil {
		return ExecutionResult{}, err
	}
	return r.GetExecution(ctx, organizationID, projectID, id)
}

func sameApproval(left, right DeliveryApproval) bool {
	return left.ApprovalID == right.ApprovalID &&
		left.OrganizationID == right.OrganizationID &&
		left.ProjectID == right.ProjectID &&
		left.PlanID == right.PlanID &&
		left.PlanVersion == right.PlanVersion &&
		left.ChangeSetID == right.ChangeSetID &&
		left.ChangeSetVersion == right.ChangeSetVersion &&
		left.PlanCanonicalHash == right.PlanCanonicalHash &&
		left.TargetSnapshotHash == right.TargetSnapshotHash &&
		left.ConfigurationSchemaVersion == right.ConfigurationSchemaVersion &&
		left.ConfigurationID == right.ConfigurationID && left.ConfigurationVersion == right.ConfigurationVersion &&
		left.ConfigurationPlatform == right.ConfigurationPlatform && left.ConfigurationProfileVersion == right.ConfigurationProfileVersion &&
		left.ConfigurationCanonicalHash == right.ConfigurationCanonicalHash && left.IntentSchemaVersion == right.IntentSchemaVersion &&
		left.IntentID == right.IntentID && left.IntentVersion == right.IntentVersion && left.IntentCanonicalHash == right.IntentCanonicalHash &&
		left.ActionHash == right.ActionHash &&
		left.Action == right.Action &&
		left.Scope == right.Scope &&
		left.BudgetLimitMinor == right.BudgetLimitMinor &&
		left.Currency == right.Currency &&
		left.ApprovedBy == right.ApprovedBy &&
		left.ApprovedAt.Equal(right.ApprovedAt) &&
		left.ExpiresAt.Equal(right.ExpiresAt) &&
		left.Source == right.Source &&
		left.Scenario == right.Scenario
}

func (r MySQLRepository) ListExecutions(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, limit int) ([]ExecutionResult, error) {
	ids, err := r.DB.QueryContext(ctx, `SELECT id FROM delivery_executions WHERE organization_id=? AND project_id=? ORDER BY started_at DESC, id DESC LIMIT ?`, organizationID, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer ids.Close()
	legacyValues := make([]ExecutionResult, 0)
	for ids.Next() {
		var id string
		if err := ids.Scan(&id); err != nil {
			return nil, err
		}
		value, err := r.GetExecution(ctx, organizationID, projectID, id)
		if err != nil {
			return nil, err
		}
		legacyValues = append(legacyValues, value)
	}
	if err := ids.Err(); err != nil {
		return nil, err
	}
	return legacyValues, nil
}

func (r MySQLRepository) CreateMetricSnapshot(ctx context.Context, value DeliveryMetricSnapshot) (DeliveryMetricSnapshot, bool, error) {
	basis, err := json.Marshal(value.CalculationBasis)
	if err != nil {
		return DeliveryMetricSnapshot{}, false, err
	}
	result, err := r.DB.ExecContext(ctx, `INSERT IGNORE INTO delivery_metric_snapshots (
		id, organization_id, project_id, execution_id, simulation_run_id, plan_id, creative_package_id,
		source, is_simulated, dataset_version, fixture_version, window_sequence, currency, window_start, window_end, data_through,
		impressions, clicks, conversions, spend_cents, revenue_cents, calculation_basis, created_by, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID, value.OrganizationID, value.ProjectID, value.ExecutionID, nullableString(value.SimulationRunID), value.PlanID, value.CreativePackageID,
		value.Source, value.IsSimulated, value.DatasetVersion, value.FixtureVersion, value.WindowSequence, value.Currency, value.WindowStart, value.WindowEnd, value.DataThrough,
		value.RawMetrics.Impressions, value.RawMetrics.Clicks, value.RawMetrics.Conversions, value.RawMetrics.SpendCents, value.RawMetrics.RevenueCents,
		basis, value.CreatedBy, value.CreatedAt)
	if err != nil {
		return DeliveryMetricSnapshot{}, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return DeliveryMetricSnapshot{}, false, err
	}
	if affected == 1 {
		return value, true, nil
	}
	values, err := r.ListMetricSnapshots(ctx, value.OrganizationID, value.ProjectID, value.ExecutionID, 100)
	if err != nil {
		return DeliveryMetricSnapshot{}, false, err
	}
	for _, existing := range values {
		if existing.DatasetVersion == value.DatasetVersion && existing.FixtureVersion == value.FixtureVersion && existing.WindowSequence == value.WindowSequence {
			return existing, false, nil
		}
	}
	return DeliveryMetricSnapshot{}, false, ErrNotFound
}

func (r MySQLRepository) ListMetricSnapshots(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, executionID string, limit int) ([]DeliveryMetricSnapshot, error) {
	rows, err := r.DB.QueryContext(ctx, metricSnapshotSelect+`
		WHERE organization_id = ? AND project_id = ? AND execution_id = ?
		ORDER BY created_at DESC, id DESC LIMIT ?`, organizationID, projectID, executionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]DeliveryMetricSnapshot, 0)
	for rows.Next() {
		value, scanErr := scanMetricSnapshot(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r MySQLRepository) ListProjectMetricSnapshots(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, limit int) ([]DeliveryMetricSnapshot, error) {
	rows, err := r.DB.QueryContext(ctx, metricSnapshotSelect+` WHERE organization_id=? AND project_id=? ORDER BY created_at DESC, id DESC LIMIT ?`, organizationID, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]DeliveryMetricSnapshot, 0)
	for rows.Next() {
		v, scanErr := scanMetricSnapshot(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, v)
	}
	return values, rows.Err()
}

const metricSnapshotSelect = `SELECT id,organization_id,project_id,execution_id,simulation_run_id,plan_id,creative_package_id,source,is_simulated,dataset_version,fixture_version,window_sequence,currency,window_start,window_end,data_through,impressions,clicks,conversions,spend_cents,revenue_cents,calculation_basis,created_by,created_at FROM delivery_metric_snapshots`

func scanMetricSnapshot(row rowScanner) (DeliveryMetricSnapshot, error) {
	var value DeliveryMetricSnapshot
	var simulationRunID sql.NullString
	var basis []byte
	err := row.Scan(&value.ID, &value.OrganizationID, &value.ProjectID, &value.ExecutionID, &simulationRunID, &value.PlanID,
		&value.CreativePackageID, &value.Source, &value.IsSimulated, &value.DatasetVersion, &value.FixtureVersion, &value.WindowSequence,
		&value.Currency, &value.WindowStart, &value.WindowEnd, &value.DataThrough, &value.RawMetrics.Impressions,
		&value.RawMetrics.Clicks, &value.RawMetrics.Conversions, &value.RawMetrics.SpendCents, &value.RawMetrics.RevenueCents,
		&basis, &value.CreatedBy, &value.CreatedAt)
	if err != nil {
		return value, err
	}
	value.SimulationRunID = simulationRunID.String
	if len(basis) > 0 {
		if err = json.Unmarshal(basis, &value.CalculationBasis); err != nil {
			return value, err
		}
	}
	return value, nil
}

func (r MySQLRepository) UpsertAlert(ctx context.Context, v DeliveryAlert) (DeliveryAlert, error) {
	entity, err := json.Marshal(v.MonitoredEntity)
	if err != nil {
		return DeliveryAlert{}, err
	}
	window, err := json.Marshal(v.Window)
	if err != nil {
		return DeliveryAlert{}, err
	}
	metric, err := json.Marshal(v.MetricDefinition)
	if err != nil {
		return DeliveryAlert{}, err
	}
	owner, err := json.Marshal(v.Owner)
	if err != nil {
		return DeliveryAlert{}, err
	}
	evidence, err := json.Marshal(v.EvidenceRefs)
	if err != nil {
		return DeliveryAlert{}, err
	}
	freshness, err := json.Marshal(v.Freshness)
	if err != nil {
		return DeliveryAlert{}, err
	}
	_, err = r.DB.ExecContext(ctx, `INSERT INTO delivery_alerts (id, organization_id, project_id, plan_id, execution_id, simulation_run_id, monitored_entity, alert_type, rule_id, rule_version, status, fingerprint, title, detail, severity, window_json, metric_definition, owner_json, evidence_refs, source, is_simulated, scenario, dataset_version, fixture_version, freshness, version, created_by, created_at, updated_at, acknowledged_at, dismissed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE updated_at=VALUES(updated_at)`, v.ID, v.OrganizationID, v.ProjectID, v.PlanID, v.ExecutionID, nullableString(v.SimulationRunID), entity, v.Type, v.RuleID, v.RuleVersion, v.Status, v.Fingerprint, v.Title, v.Detail, v.Severity, window, metric, owner, evidence, v.Source, v.IsSimulated, v.Scenario, v.DatasetVersion, v.FixtureVersion, freshness, v.Version, v.CreatedBy, v.CreatedAt, v.UpdatedAt, v.AcknowledgedAt, v.DismissedAt)
	if err != nil {
		return DeliveryAlert{}, err
	}
	return r.alertByFingerprint(ctx, v.OrganizationID, v.ProjectID, v.Fingerprint)
}
func (r MySQLRepository) alertByFingerprint(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, fingerprint string) (DeliveryAlert, error) {
	return scanAlert(r.DB.QueryRowContext(ctx, alertSelect+` WHERE organization_id=? AND project_id=? AND fingerprint=?`, org, project, fingerprint))
}
func (r MySQLRepository) ListAlerts(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, f AlertFilter) ([]DeliveryAlert, error) {
	q := alertSelect + ` WHERE organization_id=? AND project_id=?`
	args := []any{org, project}
	if f.PlanID != "" {
		q += ` AND plan_id=?`
		args = append(args, f.PlanID)
	}
	if f.ExecutionID != "" {
		q += ` AND execution_id=?`
		args = append(args, f.ExecutionID)
	}
	if f.Status != "" {
		q += ` AND status=?`
		args = append(args, f.Status)
	}
	if f.Type != "" {
		q += ` AND alert_type=?`
		args = append(args, f.Type)
	}
	if f.Severity != "" {
		q += ` AND severity=?`
		args = append(args, f.Severity)
	}
	if f.Fixture != "" {
		q += ` AND scenario=?`
		args = append(args, f.Fixture)
	}
	if f.Cursor != "" {
		q += ` AND id < ?`
		args = append(args, f.Cursor)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, f.Limit)
	rows, err := r.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]DeliveryAlert, 0)
	for rows.Next() {
		v, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, rows.Err()
}
func (r MySQLRepository) UpdateAlert(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string, action AlertAction, expected int64, actor string, now time.Time) (DeliveryAlert, error) {
	next, err := alertStatus(action)
	if err != nil {
		return DeliveryAlert{}, err
	}
	current, err := scanAlert(r.DB.QueryRowContext(ctx, alertSelect+` WHERE organization_id=? AND project_id=? AND id=?`, org, project, id))
	if errors.Is(err, sql.ErrNoRows) {
		return DeliveryAlert{}, ErrNotFound
	}
	if err != nil {
		return DeliveryAlert{}, err
	}
	if current.Status == next {
		return current, nil
	}
	if current.Version != expected {
		return DeliveryAlert{}, ErrVersionConflict
	}
	if current.Status != AlertOpen {
		return DeliveryAlert{}, ErrInvalidState
	}
	result, err := r.DB.ExecContext(ctx, `UPDATE delivery_alerts SET status=?, version=version+1, resolved_by=?, acknowledged_at=IF(?='acknowledged', ?, acknowledged_at), dismissed_at=IF(?='dismissed', ?, dismissed_at), updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND version=? AND status='open'`, next, actor, next, now, next, now, now, org, project, id, expected)
	if err != nil {
		return DeliveryAlert{}, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return DeliveryAlert{}, ErrVersionConflict
	}
	return scanAlert(r.DB.QueryRowContext(ctx, alertSelect+` WHERE organization_id=? AND project_id=? AND id=?`, org, project, id))
}

const alertSelect = `SELECT id, organization_id, project_id, plan_id, execution_id, simulation_run_id, monitored_entity, alert_type, rule_id, rule_version, status, fingerprint, title, detail, severity, window_json, metric_definition, owner_json, evidence_refs, source, is_simulated, scenario, dataset_version, fixture_version, freshness, version, created_by, created_at, updated_at, acknowledged_at, dismissed_at, resolved_by FROM delivery_alerts`

func scanAlert(row rowScanner) (DeliveryAlert, error) {
	var v DeliveryAlert
	var entity, window, metric, owner, evidence, freshness []byte
	var simulationRunID sql.NullString
	err := row.Scan(&v.ID, &v.OrganizationID, &v.ProjectID, &v.PlanID, &v.ExecutionID, &simulationRunID, &entity, &v.Type, &v.RuleID, &v.RuleVersion, &v.Status, &v.Fingerprint, &v.Title, &v.Detail, &v.Severity, &window, &metric, &owner, &evidence, &v.Source, &v.IsSimulated, &v.Scenario, &v.DatasetVersion, &v.FixtureVersion, &freshness, &v.Version, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt, &v.AcknowledgedAt, &v.DismissedAt, &v.ResolvedBy)
	if err != nil {
		return v, err
	}
	v.SimulationRunID = simulationRunID.String
	if err = json.Unmarshal(entity, &v.MonitoredEntity); err != nil {
		return v, err
	}
	if err = json.Unmarshal(window, &v.Window); err != nil {
		return v, err
	}
	if err = json.Unmarshal(metric, &v.MetricDefinition); err != nil {
		return v, err
	}
	if err = json.Unmarshal(owner, &v.Owner); err != nil {
		return v, err
	}
	err = json.Unmarshal(evidence, &v.EvidenceRefs)
	if err != nil {
		return v, err
	}
	err = json.Unmarshal(freshness, &v.Freshness)
	return v, err
}

const deliveryPlanSelect = `SELECT id, organization_id, project_id, creative_package_id, creative_package_hash, creative_version_id, name, objective, budget_cents, start_at, end_at, status, version, platform, source, scenario, tour_run_id, tour_owner_id, tour_case, current_version, created_by, created_at, updated_at FROM delivery_plans`
const changeSetSelect = `SELECT id, organization_id, project_id, plan_id, plan_version, status, risk_level, preflight_notes, target_snapshot, target_snapshot_hash, recommendation_id, approved_by, approved_at, rejected_by, rejected_at, rejection_reason, version, created_by, created_at, updated_at FROM delivery_change_sets`
const approvalSelect = `SELECT approval_id, organization_id, project_id, plan_id, plan_version, change_set_id, change_set_version, plan_canonical_hash, target_snapshot_hash, action_hash, configuration_schema_version, configuration_id, configuration_version, configuration_platform, configuration_profile_version, configuration_canonical_hash, intent_schema_version, intent_id, intent_version, intent_canonical_hash, action, scope, budget_limit_minor, currency, approved_by, approved_at, expires_at, source, scenario FROM delivery_approvals`

type rowScanner interface {
	Scan(...any) error
}

func scanDeliveryPlan(row rowScanner) (DeliveryPlan, error) {
	var value DeliveryPlan
	var tourRunID, tourOwnerID, tourCase sql.NullString
	err := row.Scan(&value.ID, &value.OrganizationID, &value.ProjectID, &value.CreativePackageID,
		&value.CreativePackageHash, &value.CreativeVersionID, &value.Name, &value.Objective,
		&value.BudgetCents, &value.StartAt, &value.EndAt, &value.Status, &value.Version,
		&value.Platform, &value.Source, &value.Scenario, &tourRunID, &tourOwnerID, &tourCase, &value.CurrentVersionNumber,
		&value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	value.TourRunID, value.TourOwnerID, value.TourCase = tourRunID.String, tourOwnerID.String, tourCase.String
	return value, err
}

func scanChangeSet(row rowScanner) (ChangeSet, error) {
	var value ChangeSet
	var notes []byte
	var target []byte
	var targetHash, recommendationID sql.NullString
	var approvedBy sql.NullString
	var approvedAt sql.NullTime
	var rejectedBy, rejectionReason sql.NullString
	var rejectedAt sql.NullTime
	err := row.Scan(&value.ID, &value.OrganizationID, &value.ProjectID, &value.PlanID,
		&value.PlanVersion, &value.Status, &value.RiskLevel, &notes, &target, &targetHash, &recommendationID, &approvedBy, &approvedAt,
		&rejectedBy, &rejectedAt, &rejectionReason,
		&value.Version, &value.CreatedBy, &value.CreatedAt, &value.UpdatedAt)
	if err != nil {
		return ChangeSet{}, err
	}
	if err := decodeChangeSetOptional(&value, notes, target, targetHash, recommendationID, approvedBy, approvedAt); err != nil {
		return ChangeSet{}, err
	}
	if rejectedBy.Valid {
		value.RejectedBy = rejectedBy.String
	}
	if rejectedAt.Valid {
		value.RejectedAt = &rejectedAt.Time
	}
	if rejectionReason.Valid {
		value.RejectionReason = rejectionReason.String
	}
	return value, nil
}

func decodeChangeSetOptional(value *ChangeSet, notes, target []byte, targetHash, recommendationID, approvedBy sql.NullString, approvedAt sql.NullTime) error {
	if err := json.Unmarshal(notes, &value.PreflightNotes); err != nil {
		return fmt.Errorf("decode delivery preflight notes: %w", err)
	}
	if len(target) > 0 {
		var descriptor struct {
			SchemaVersion string `json:"schema_version"`
			Schema        string `json:"schema"`
		}
		if err := json.Unmarshal(target, &descriptor); err != nil {
			return fmt.Errorf("decode delivery target discriminator: %w", err)
		}
		switch {
		case descriptor.SchemaVersion == PlatformConfigurationSchemaV2:
			var snapshot PlatformConfiguration
			if err := json.Unmarshal(target, &snapshot); err != nil {
				return fmt.Errorf("decode delivery target snapshot: %w", err)
			}
			value.TargetSnapshot = &snapshot
		case descriptor.Schema == ThreeTierSchema:
			var snapshot ThreeTierConfiguration
			if err := json.Unmarshal(target, &snapshot); err != nil {
				return fmt.Errorf("decode legacy delivery target snapshot: %w", err)
			}
			value.LegacyTargetSnapshot = &snapshot
		default:
			return contractFailure(ContractErrorUnknownSchemaVersion, "target_snapshot", "unknown delivery target snapshot schema")
		}
	}
	if targetHash.Valid {
		value.TargetSnapshotHash = targetHash.String
	}
	if recommendationID.Valid {
		value.RecommendationID = recommendationID.String
	}
	if approvedBy.Valid {
		value.ApprovedBy = approvedBy.String
	}
	if approvedAt.Valid {
		value.ApprovedAt = &approvedAt.Time
	}
	return nil
}

func changeSetSnapshotJSON(value ChangeSet) any {
	if value.TargetSnapshot != nil {
		return nullableJSON(value.TargetSnapshot)
	}
	return nullableJSON(value.LegacyTargetSnapshot)
}

func changeSetSnapshotSchema(value ChangeSet) string {
	if value.TargetSnapshot != nil {
		return PlatformConfigurationSchemaV2
	}
	return ""
}

func scanApproval(row rowScanner) (DeliveryApproval, error) {
	var value DeliveryApproval
	var targetSnapshotHash, configurationSchemaVersion, configurationID, configurationPlatform, configurationProfileVersion, configurationCanonicalHash sql.NullString
	var intentSchemaVersion, intentID, intentCanonicalHash sql.NullString
	var configurationVersion, intentVersion sql.NullInt64
	err := row.Scan(
		&value.ApprovalID, &value.OrganizationID, &value.ProjectID, &value.PlanID,
		&value.PlanVersion, &value.ChangeSetID, &value.ChangeSetVersion,
		&value.PlanCanonicalHash, &targetSnapshotHash, &value.ActionHash,
		&configurationSchemaVersion, &configurationID, &configurationVersion, &configurationPlatform, &configurationProfileVersion, &configurationCanonicalHash,
		&intentSchemaVersion, &intentID, &intentVersion, &intentCanonicalHash, &value.Action, &value.Scope,
		&value.BudgetLimitMinor, &value.Currency, &value.ApprovedBy, &value.ApprovedAt,
		&value.ExpiresAt, &value.Source, &value.Scenario,
	)
	if targetSnapshotHash.Valid {
		value.TargetSnapshotHash = targetSnapshotHash.String
	}
	value.ConfigurationSchemaVersion, value.ConfigurationID = configurationSchemaVersion.String, configurationID.String
	value.ConfigurationVersion, value.ConfigurationPlatform = int(configurationVersion.Int64), DeliveryPlatform(configurationPlatform.String)
	value.ConfigurationProfileVersion, value.ConfigurationCanonicalHash = configurationProfileVersion.String, configurationCanonicalHash.String
	value.IntentSchemaVersion, value.IntentID = intentSchemaVersion.String, intentID.String
	value.IntentVersion, value.IntentCanonicalHash = int(intentVersion.Int64), intentCanonicalHash.String
	return value, err
}
