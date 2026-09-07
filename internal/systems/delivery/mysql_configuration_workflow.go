package delivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/shikanon/cookies/internal/platform/contract"
)

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
	v.RuntimeStatus, v.ReadOnly = PlanRuntimeLegacyUnsupported, true
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
		value.RuntimeStatus, value.ReadOnly = PlanRuntimeLegacyUnsupported, true
		return nil
	}
	return contractFailure(ContractErrorUnknownSchemaVersion, "recommendation_snapshot", "unknown recommendation snapshot schema")
}
