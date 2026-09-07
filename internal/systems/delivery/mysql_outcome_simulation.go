package delivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/shikanon/cookies/internal/platform/contract"
)

func (r MySQLRepository) GetLatestOutcomeSimulation(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, executionID string) (OutcomeSimulationRun, []DeliveryMetricSnapshot, error) {
	run, err := scanOutcomeSimulation(r.DB.QueryRowContext(ctx, outcomeSimulationSelect+` WHERE organization_id=? AND project_id=? AND execution_id=? ORDER BY created_at DESC,id DESC LIMIT 1`, organizationID, projectID, executionID))
	if errors.Is(err, sql.ErrNoRows) {
		return OutcomeSimulationRun{}, nil, ErrNotFound
	}
	if err != nil {
		return OutcomeSimulationRun{}, nil, err
	}
	metrics, err := r.listSimulationMetrics(ctx, organizationID, projectID, run.ID)
	return run, metrics, err
}

func (r MySQLRepository) listSimulationMetrics(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, runID string) ([]DeliveryMetricSnapshot, error) {
	rows, err := r.DB.QueryContext(ctx, metricSnapshotSelect+` WHERE organization_id=? AND project_id=? AND simulation_run_id=? ORDER BY window_sequence ASC`, organizationID, projectID, runID)
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

const outcomeSimulationSelect = `SELECT id,organization_id,project_id,execution_id,plan_id,plan_version,plan_hash,model_version,scenario,stable_seed,input_hash,fingerprint,input_json,parameters_json,events_json,evidence_json,status,created_by,created_at,completed_at FROM delivery_simulation_runs`

func scanOutcomeSimulation(row rowScanner) (OutcomeSimulationRun, error) {
	var run OutcomeSimulationRun
	var input, parameters, events, evidence []byte
	err := row.Scan(&run.ID, &run.OrganizationID, &run.ProjectID, &run.ExecutionID, &run.PlanID, &run.PlanVersion, &run.PlanHash,
		&run.ModelVersion, &run.Scenario, &run.StableSeed, &run.InputHash, &run.Fingerprint, &input, &parameters, &events, &evidence,
		&run.Status, &run.CreatedBy, &run.CreatedAt, &run.CompletedAt)
	if err != nil {
		return run, err
	}
	if err = json.Unmarshal(input, &run.Input); err != nil {
		return run, fmt.Errorf("decode simulation input: %w", err)
	}
	if err = json.Unmarshal(parameters, &run.Parameters); err != nil {
		return run, fmt.Errorf("decode simulation parameters: %w", err)
	}
	if err = json.Unmarshal(events, &run.Events); err != nil {
		return run, fmt.Errorf("decode simulation events: %w", err)
	}
	if err = json.Unmarshal(evidence, &run.Evidence); err != nil {
		return run, fmt.Errorf("decode simulation evidence: %w", err)
	}
	return run, nil
}
