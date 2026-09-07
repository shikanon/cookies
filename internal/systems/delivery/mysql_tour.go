package delivery

import (
	"context"
	"database/sql"
	"errors"

	"github.com/shikanon/cookies/internal/platform/contract"
)

func (r MySQLRepository) GetTourRun(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, runID string) (DeliveryTourRun, error) {
	var value DeliveryTourRun
	var preparedAt, resetAt sql.NullTime
	err := r.DB.QueryRowContext(ctx, `SELECT id, organization_id, project_id, owner_id, status, source, scenario,
		prepared_at, reset_at, created_at, updated_at FROM delivery_tour_runs
		WHERE organization_id=? AND project_id=? AND id=?`, organizationID, projectID, runID).Scan(
		&value.ID, &value.OrganizationID, &value.ProjectID, &value.OwnerID, &value.Status, &value.Source, &value.Scenario,
		&preparedAt, &resetAt, &value.CreatedAt, &value.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return DeliveryTourRun{}, ErrNotFound
	}
	if preparedAt.Valid {
		value.PreparedAt = &preparedAt.Time
	}
	if resetAt.Valid {
		value.ResetAt = &resetAt.Time
	}
	return value, err
}

func (r MySQLRepository) ListTourPlans(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, runID, ownerID string) ([]DeliveryPlan, error) {
	rows, err := r.DB.QueryContext(ctx, deliveryPlanSelect+` WHERE organization_id=? AND project_id=? AND tour_run_id=? AND tour_owner_id=? ORDER BY tour_case`, organizationID, projectID, runID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]DeliveryPlan, 0, 7)
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
	return sortedTourPlans(values), rows.Err()
}

func (r MySQLRepository) ListTourPlanChangeSets(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, planID string) ([]ChangeSet, error) {
	rows, err := r.DB.QueryContext(ctx, changeSetSelect+` WHERE organization_id=? AND project_id=? AND plan_id=? ORDER BY updated_at DESC, id DESC`, organizationID, projectID, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]ChangeSet, 0, 2)
	for rows.Next() {
		value, scanErr := scanChangeSet(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r MySQLRepository) ListTourPlanExecutions(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, planID string) ([]ExecutionResult, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT x.id FROM delivery_executions x
		JOIN delivery_change_sets c ON c.organization_id=x.organization_id AND c.project_id=x.project_id AND c.id=x.change_set_id
		WHERE x.organization_id=? AND x.project_id=? AND c.plan_id=? ORDER BY x.started_at DESC, x.id DESC`, organizationID, projectID, planID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, 2)
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		ids = append(ids, id)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		rows.Close()
		return nil, rowsErr
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	values := make([]ExecutionResult, 0, len(ids))
	for _, id := range ids {
		value, getErr := r.GetExecution(ctx, organizationID, projectID, id)
		if getErr != nil {
			return nil, getErr
		}
		values = append(values, value)
	}
	return values, nil
}

func (r MySQLRepository) ListTourPlanAlerts(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, planID string) ([]DeliveryAlert, error) {
	rows, err := r.DB.QueryContext(ctx, alertSelect+` WHERE organization_id=? AND project_id=? AND plan_id=? ORDER BY id DESC`, organizationID, projectID, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]DeliveryAlert, 0, 4)
	for rows.Next() {
		value, scanErr := scanAlert(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r MySQLRepository) ListTourPlanRecommendations(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, planID string) ([]DeliveryRecommendation, error) {
	rows, err := r.DB.QueryContext(ctx, recommendationSelect+` WHERE organization_id=? AND project_id=? AND plan_id=? ORDER BY created_at DESC, id DESC`, organizationID, projectID, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]DeliveryRecommendation, 0, 2)
	for rows.Next() {
		value, scanErr := scanRecommendation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}
