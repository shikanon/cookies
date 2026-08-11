package delivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/shikanon/cookies/internal/platform/contract"
)

// CreateRecommendation persists a project-scoped recommendation derived from an immutable configuration snapshot.
func (r MySQLRepository) CreateRecommendation(ctx context.Context, v DeliveryRecommendation) (DeliveryRecommendation, error) {
	target, err := json.Marshal(recommendationTarget(v))
	if err != nil {
		return v, err
	}
	base, err := json.Marshal(recommendationBase(v))
	if err != nil {
		return v, err
	}
	evidence, _ := json.Marshal(v.Evidence)
	risks, _ := json.Marshal(v.Risks)
	_, err = r.DB.ExecContext(ctx, `INSERT INTO delivery_recommendations (id,organization_id,project_id,plan_id,plan_version,simulation_run_id,fingerprint,base_snapshot_hash,base_snapshot,target_snapshot,target_snapshot_hash,evidence_json,action_text,impact_text,risks_json,observation_text,cooldown_until,provenance,status,version,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, v.ID, v.OrganizationID, v.ProjectID, v.PlanID, v.PlanVersion, nullableString(v.SimulationRunID), v.Fingerprint, v.BaseSnapshotHash, base, target, v.TargetSnapshotHash, evidence, v.Action, v.Impact, risks, v.Observation, v.CooldownUntil, v.Provenance, v.Status, v.Version, v.CreatedBy, v.CreatedAt, v.UpdatedAt)
	if err != nil {
		var mysqlError *mysqlDriver.MySQLError
		if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
			existing, getErr := scanRecommendation(r.DB.QueryRowContext(ctx, recommendationSelect+` WHERE organization_id=? AND project_id=? AND fingerprint=?`, v.OrganizationID, v.ProjectID, v.Fingerprint))
			if getErr == nil && existing.BaseSnapshotHash == v.BaseSnapshotHash && existing.TargetSnapshotHash == v.TargetSnapshotHash {
				return existing, nil
			}
			if getErr == nil {
				return DeliveryRecommendation{}, ErrIdempotencyConflict
			}
		}
		return DeliveryRecommendation{}, err
	}
	return v, nil
}
func (r MySQLRepository) ListRecommendations(ctx context.Context, o contract.OrganizationID, p contract.ProjectID, limit int) ([]DeliveryRecommendation, error) {
	rows, err := r.DB.QueryContext(ctx, recommendationSelect+` WHERE organization_id=? AND project_id=? ORDER BY created_at DESC,id DESC LIMIT ?`, o, p, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeliveryRecommendation{}
	for rows.Next() {
		v, e := scanRecommendation(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r MySQLRepository) GetRecommendation(ctx context.Context, o contract.OrganizationID, p contract.ProjectID, id string) (DeliveryRecommendation, error) {
	v, err := scanRecommendation(r.DB.QueryRowContext(ctx, recommendationSelect+` WHERE organization_id=? AND project_id=? AND id=?`, o, p, id))
	if errors.Is(err, sql.ErrNoRows) {
		return DeliveryRecommendation{}, ErrNotFound
	}
	return v, err
}
func (r MySQLRepository) AcceptRecommendation(ctx context.Context, v DeliveryRecommendation, key, requestHash string, cs ChangeSet) (RecommendationAcceptance, bool, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return RecommendationAcceptance{}, false, err
	}
	defer tx.Rollback()
	stored, err := scanRecommendation(tx.QueryRowContext(ctx, recommendationSelect+` WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, v.OrganizationID, v.ProjectID, v.ID))
	if errors.Is(err, sql.ErrNoRows) {
		return RecommendationAcceptance{}, false, ErrNotFound
	}
	if err != nil {
		return RecommendationAcceptance{}, false, err
	}
	if stored.Status == RecommendationAccepted {
		if stored.IdempotencyKey == key && stored.RequestHash == requestHash {
			got, err := scanChangeSet(tx.QueryRowContext(ctx, changeSetSelect+` WHERE organization_id=? AND project_id=? AND id=?`, v.OrganizationID, v.ProjectID, stored.AcceptedChangeSetID))
			return RecommendationAcceptance{Recommendation: stored, ChangeSet: got}, true, err
		}
		return RecommendationAcceptance{}, false, ErrIdempotencyConflict
	}
	if stored.Status != RecommendationProposed || stored.Version != v.Version {
		return RecommendationAcceptance{}, false, ErrVersionConflict
	}
	// Serialize draft creation per Plan even when two different recommendations
	// are accepted concurrently. MySQL has no portable partial unique index for
	// status='draft', so the parent Plan row is the transaction lock.
	var lockedPlanID string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM delivery_plans WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, v.OrganizationID, v.ProjectID, v.PlanID).Scan(&lockedPlanID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RecommendationAcceptance{}, false, ErrNotFound
		}
		return RecommendationAcceptance{}, false, err
	}
	var draftCount int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM delivery_change_sets WHERE organization_id=? AND project_id=? AND plan_id=? AND status='draft'`, v.OrganizationID, v.ProjectID, v.PlanID).Scan(&draftCount)
	if err != nil {
		return RecommendationAcceptance{}, false, err
	}
	if draftCount > 0 {
		return RecommendationAcceptance{}, false, ErrInvalidState
	}
	notes, _ := json.Marshal(cs.PreflightNotes)
	target := changeSetSnapshotJSON(cs)
	_, err = tx.ExecContext(ctx, `INSERT INTO delivery_change_sets (id,organization_id,project_id,plan_id,plan_version,status,risk_level,preflight_notes,target_snapshot,target_snapshot_hash,target_snapshot_schema_version,recommendation_id,approved_by,approved_at,version,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,NULL,NULL,?,?,?,?)`, cs.ID, cs.OrganizationID, cs.ProjectID, cs.PlanID, cs.PlanVersion, cs.Status, cs.RiskLevel, notes, target, cs.TargetSnapshotHash, nullableString(changeSetSnapshotSchema(cs)), cs.RecommendationID, cs.Version, cs.CreatedBy, cs.CreatedAt, cs.UpdatedAt)
	if err != nil {
		return RecommendationAcceptance{}, false, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE delivery_recommendations SET status=?,version=version+1,idempotency_key=?,request_hash=?,accepted_change_set_id=?,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND status=? AND version=?`, RecommendationAccepted, key, requestHash, cs.ID, cs.UpdatedAt, v.OrganizationID, v.ProjectID, v.ID, RecommendationProposed, v.Version)
	if err != nil {
		return RecommendationAcceptance{}, false, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return RecommendationAcceptance{}, false, ErrVersionConflict
	}
	if err = tx.Commit(); err != nil {
		return RecommendationAcceptance{}, false, err
	}
	stored.Status = RecommendationAccepted
	stored.Version++
	stored.IdempotencyKey = key
	stored.RequestHash = requestHash
	stored.AcceptedChangeSetID = cs.ID
	stored.UpdatedAt = cs.UpdatedAt
	return RecommendationAcceptance{Recommendation: stored, ChangeSet: cs}, false, nil
}
func (r MySQLRepository) RejectRecommendation(ctx context.Context, o contract.OrganizationID, p contract.ProjectID, id string, expected int64, actor string, now time.Time) (DeliveryRecommendation, error) {
	res, err := r.DB.ExecContext(ctx, `UPDATE delivery_recommendations SET status=?,version=version+1,updated_at=? WHERE organization_id=? AND project_id=? AND id=? AND status=? AND version=?`, RecommendationRejected, now, o, p, id, RecommendationProposed, expected)
	if err != nil {
		return DeliveryRecommendation{}, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return DeliveryRecommendation{}, ErrVersionConflict
	}
	return r.GetRecommendation(ctx, o, p, id)
}
func (r MySQLRepository) CreateOrGetManualActionPackage(ctx context.Context, v ManualActionPackage) (ManualActionPackage, bool, error) {
	existing, err := r.GetManualActionPackage(ctx, v.OrganizationID, v.ProjectID, v.ChangeSetID)
	if err == nil {
		if existing.TargetSnapshotHash == v.TargetSnapshotHash {
			return existing, true, nil
		}
		return ManualActionPackage{}, false, ErrInvalidState
	}
	if !errors.Is(err, ErrNotFound) {
		return ManualActionPackage{}, false, err
	}
	payload, err := json.Marshal(v)
	if err != nil {
		return ManualActionPackage{}, false, err
	}
	_, err = r.DB.ExecContext(ctx, `INSERT INTO delivery_manual_action_packages (id,organization_id,project_id,change_set_id,target_snapshot_hash,content_hash,package_json,created_at) VALUES (?,?,?,?,?,?,?,?)`, v.ID, v.OrganizationID, v.ProjectID, v.ChangeSetID, v.TargetSnapshotHash, v.ContentHash, payload, v.CreatedAt)
	if err != nil {
		var mysqlError *mysqlDriver.MySQLError
		if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
			existing, getErr := r.GetManualActionPackage(ctx, v.OrganizationID, v.ProjectID, v.ChangeSetID)
			if getErr == nil && existing.TargetSnapshotHash == v.TargetSnapshotHash && existing.ContentHash == v.ContentHash {
				return existing, true, nil
			}
			if getErr == nil {
				return ManualActionPackage{}, false, ErrIdempotencyConflict
			}
		}
		return ManualActionPackage{}, false, err
	}
	return v, false, nil
}
func (r MySQLRepository) GetManualActionPackage(ctx context.Context, o contract.OrganizationID, p contract.ProjectID, cs string) (ManualActionPackage, error) {
	var payload []byte
	err := r.DB.QueryRowContext(ctx, `SELECT package_json FROM delivery_manual_action_packages WHERE organization_id=? AND project_id=? AND change_set_id=? ORDER BY created_at DESC LIMIT 1`, o, p, cs).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return ManualActionPackage{}, ErrNotFound
	}
	if err != nil {
		return ManualActionPackage{}, err
	}
	var v ManualActionPackage
	if err = json.Unmarshal(payload, &v); err != nil {
		return ManualActionPackage{}, fmt.Errorf("decode manual action package: %w", err)
	}
	return v, nil
}

const recommendationSelect = `SELECT id,organization_id,project_id,plan_id,plan_version,simulation_run_id,fingerprint,base_snapshot_hash,base_snapshot,target_snapshot,target_snapshot_hash,evidence_json,action_text,impact_text,risks_json,observation_text,cooldown_until,provenance,status,version,idempotency_key,request_hash,accepted_change_set_id,created_by,created_at,updated_at FROM delivery_recommendations`

func scanRecommendation(row rowScanner) (DeliveryRecommendation, error) {
	var v DeliveryRecommendation
	var base, target, evidence, risks []byte
	var cooldown sql.NullTime
	var simulationRunID, key, hash, cs sql.NullString
	err := row.Scan(&v.ID, &v.OrganizationID, &v.ProjectID, &v.PlanID, &v.PlanVersion, &simulationRunID, &v.Fingerprint, &v.BaseSnapshotHash, &base, &target, &v.TargetSnapshotHash, &evidence, &v.Action, &v.Impact, &risks, &v.Observation, &cooldown, &v.Provenance, &v.Status, &v.Version, &key, &hash, &cs, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return v, err
	}
	v.SimulationRunID = simulationRunID.String
	if err = decodeRecommendationSnapshot(&v, base, true); err != nil {
		return v, err
	}
	if err = decodeRecommendationSnapshot(&v, target, false); err != nil {
		return v, err
	}
	_ = json.Unmarshal(evidence, &v.Evidence)
	_ = json.Unmarshal(risks, &v.Risks)
	if cooldown.Valid {
		v.CooldownUntil = &cooldown.Time
	}
	if key.Valid {
		v.IdempotencyKey = key.String
	}
	if hash.Valid {
		v.RequestHash = hash.String
	}
	if cs.Valid {
		v.AcceptedChangeSetID = cs.String
	}
	return v, nil
}

func recommendationBase(value DeliveryRecommendation) any {
	if value.BaseConfiguration != nil {
		return value.BaseConfiguration
	}
	return value.BaseSnapshot
}

func recommendationTarget(value DeliveryRecommendation) any {
	if value.TargetConfiguration != nil {
		return value.TargetConfiguration
	}
	return value.TargetSnapshot
}

func decodeRecommendationSnapshot(value *DeliveryRecommendation, payload []byte, base bool) error {
	if len(payload) == 0 || string(payload) == "null" {
		return nil
	}
	var descriptor struct {
		SchemaVersion string `json:"schema_version"`
		Schema        string `json:"schema"`
	}
	if err := json.Unmarshal(payload, &descriptor); err != nil {
		return err
	}
	if descriptor.SchemaVersion == PlatformConfigurationSchemaV2 {
		var configuration PlatformConfiguration
		if err := json.Unmarshal(payload, &configuration); err != nil {
			return err
		}
		if base {
			value.BaseConfiguration = &configuration
		} else {
			value.TargetConfiguration = &configuration
		}
		return nil
	}
	if descriptor.Schema == ThreeTierSchema {
		var snapshot ThreeTierConfiguration
		if err := json.Unmarshal(payload, &snapshot); err != nil {
			return err
		}
		if base {
			value.BaseSnapshot = &snapshot
		} else {
			value.TargetSnapshot = &snapshot
		}
		return nil
	}
	return contractFailure(ContractErrorUnknownSchemaVersion, "recommendation_snapshot", "unknown recommendation snapshot schema")
}
