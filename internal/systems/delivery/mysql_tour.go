package delivery

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

func (r MySQLRepository) CreateOrGetTourRun(ctx context.Context, value DeliveryTourRun) (DeliveryTourRun, bool, error) {
	result, err := r.DB.ExecContext(ctx, `INSERT IGNORE INTO delivery_tour_runs
		(id, organization_id, project_id, owner_id, status, source, scenario, prepared_at, reset_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?)`, value.ID, value.OrganizationID, value.ProjectID, value.OwnerID, value.Status, value.Source, value.Scenario, value.CreatedAt, value.UpdatedAt)
	if err != nil {
		return DeliveryTourRun{}, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return DeliveryTourRun{}, false, err
	}
	stored, err := r.GetTourRun(ctx, value.OrganizationID, value.ProjectID, value.ID)
	return stored, affected == 0, err
}

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

func (r MySQLRepository) SetTourRunStatus(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, runID, ownerID string, status TourRunStatus, now time.Time) (DeliveryTourRun, error) {
	preparedAt := any(nil)
	resetAt := any(nil)
	if status == TourRunPrepared {
		preparedAt = now
	}
	if status == TourRunReset {
		resetAt = now
	}
	result, err := r.DB.ExecContext(ctx, `UPDATE delivery_tour_runs SET status=?,
		prepared_at=COALESCE(?, prepared_at), reset_at=COALESCE(?, reset_at), updated_at=?
		WHERE organization_id=? AND project_id=? AND id=? AND owner_id=?`, status, preparedAt, resetAt, now, organizationID, projectID, runID, ownerID)
	if err != nil {
		return DeliveryTourRun{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return DeliveryTourRun{}, err
	}
	if affected == 0 {
		if value, getErr := r.GetTourRun(ctx, organizationID, projectID, runID); getErr == nil && value.OwnerID != ownerID {
			return DeliveryTourRun{}, ErrTourOwnerMismatch
		}
		return DeliveryTourRun{}, ErrNotFound
	}
	return r.GetTourRun(ctx, organizationID, projectID, runID)
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

func (r MySQLRepository) ResetTourRun(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, runID, ownerID string, now time.Time) (map[string]int64, DeliveryTourRun, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, DeliveryTourRun{}, err
	}
	defer tx.Rollback()
	var storedOwner string
	err = tx.QueryRowContext(ctx, `SELECT owner_id FROM delivery_tour_runs WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`, organizationID, projectID, runID).Scan(&storedOwner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, DeliveryTourRun{}, ErrNotFound
	}
	if err != nil {
		return nil, DeliveryTourRun{}, err
	}
	if storedOwner != ownerID {
		return nil, DeliveryTourRun{}, ErrTourOwnerMismatch
	}
	plans, err := selectIDs(ctx, tx, `SELECT id FROM delivery_plans WHERE organization_id=? AND project_id=? AND tour_run_id=? AND tour_owner_id=? FOR UPDATE`, organizationID, projectID, runID, ownerID)
	if err != nil {
		return nil, DeliveryTourRun{}, err
	}
	configurationIDs, intentIDs := []string{}, []string{}
	if len(plans) > 0 {
		configurationQuery, configurationArgs := scopedInQuery(`SELECT DISTINCT JSON_UNQUOTE(JSON_EXTRACT(config_json,'$.platform_configuration.configuration_id')) FROM delivery_plan_versions`, "plan_id", organizationID, projectID, plans)
		configurationIDs, err = selectIDs(ctx, tx, configurationQuery+` AND JSON_EXTRACT(config_json,'$.platform_configuration.configuration_id') IS NOT NULL FOR UPDATE`, configurationArgs...)
		if err != nil {
			return nil, DeliveryTourRun{}, err
		}
		intentQuery, intentArgs := scopedInQuery(`SELECT DISTINCT JSON_UNQUOTE(JSON_EXTRACT(config_json,'$.intent.intent_id')) FROM delivery_plan_versions`, "plan_id", organizationID, projectID, plans)
		intentIDs, err = selectIDs(ctx, tx, intentQuery+` AND JSON_EXTRACT(config_json,'$.intent.intent_id') IS NOT NULL FOR UPDATE`, intentArgs...)
		if err != nil {
			return nil, DeliveryTourRun{}, err
		}
	}
	changeSets, err := selectRelatedIDs(ctx, tx, "delivery_change_sets", "plan_id", organizationID, projectID, plans)
	if err != nil {
		return nil, DeliveryTourRun{}, err
	}
	executions, err := selectRelatedIDs(ctx, tx, "delivery_executions", "change_set_id", organizationID, projectID, changeSets)
	if err != nil {
		return nil, DeliveryTourRun{}, err
	}
	deleted := map[string]int64{}
	for _, operation := range []struct {
		table  string
		column string
		ids    []string
	}{
		{"delivery_manual_action_packages", "change_set_id", changeSets},
		{"delivery_recommendations", "plan_id", plans},
		{"delivery_alerts", "plan_id", plans},
		{"delivery_metric_snapshots", "plan_id", plans},
		{"delivery_simulation_runs", "plan_id", plans},
		{"delivery_execution_steps", "execution_id", executions},
		{"delivery_evidence", "execution_id", executions},
		{"delivery_executions", "id", executions},
		{"delivery_approvals", "change_set_id", changeSets},
		{"delivery_change_sets", "id", changeSets},
		{"delivery_plan_versions", "plan_id", plans},
		{"delivery_plans", "id", plans},
		{"delivery_platform_configurations", "configuration_id", configurationIDs},
		{"delivery_intents", "intent_id", intentIDs},
	} {
		count, deleteErr := deleteRelated(ctx, tx, operation.table, operation.column, organizationID, projectID, operation.ids)
		if deleteErr != nil {
			return nil, DeliveryTourRun{}, deleteErr
		}
		deleted[operation.table] = count
	}
	result, err := tx.ExecContext(ctx, `UPDATE delivery_tour_runs SET status=?, reset_at=?, updated_at=?
		WHERE organization_id=? AND project_id=? AND id=? AND owner_id=?`, TourRunReset, now, now, organizationID, projectID, runID, ownerID)
	if err != nil {
		return nil, DeliveryTourRun{}, err
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected != 1 {
		if affectedErr != nil {
			return nil, DeliveryTourRun{}, affectedErr
		}
		return nil, DeliveryTourRun{}, ErrVersionConflict
	}
	if err := tx.Commit(); err != nil {
		return nil, DeliveryTourRun{}, err
	}
	run, err := r.GetTourRun(ctx, organizationID, projectID, runID)
	return deleted, run, err
}

func selectIDs(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		values = append(values, id)
	}
	return values, rows.Err()
}

func selectRelatedIDs(ctx context.Context, tx *sql.Tx, table, column string, organizationID contract.OrganizationID, projectID contract.ProjectID, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}
	query, args := scopedInQuery("SELECT id FROM "+table, column, organizationID, projectID, ids)
	return selectIDs(ctx, tx, query+" FOR UPDATE", args...)
}

func deleteRelated(ctx context.Context, tx *sql.Tx, table, column string, organizationID contract.OrganizationID, projectID contract.ProjectID, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	query, args := scopedInQuery("DELETE FROM "+table, column, organizationID, projectID, ids)
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func scopedInQuery(prefix, column string, organizationID contract.OrganizationID, projectID contract.ProjectID, ids []string) (string, []any) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	query := fmt.Sprintf("%s WHERE organization_id=? AND project_id=? AND %s IN (%s)", prefix, column, placeholders)
	args := make([]any, 0, len(ids)+2)
	args = append(args, organizationID, projectID)
	for _, id := range ids {
		args = append(args, id)
	}
	return query, args
}
