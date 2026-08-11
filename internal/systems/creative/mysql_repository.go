package creative

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

// MySQLRepository persists only Creative-owned objects. It never joins
// Strategy, Provider, or Assets tables; those systems expose their own seams.
type MySQLRepository struct{ DB *sql.DB }

func (r MySQLRepository) CreateIntake(ctx context.Context, intake CreativeIntake) (CreativeIntake, bool, error) {
	if r.DB == nil {
		return CreativeIntake{}, false, fmt.Errorf("creative MySQL database is required")
	}
	request, err := json.Marshal(intake.Request)
	if err != nil {
		return CreativeIntake{}, false, fmt.Errorf("encode creative intake request: %w", err)
	}
	missing, err := json.Marshal(intake.MissingFields)
	if err != nil {
		return CreativeIntake{}, false, err
	}
	warnings, err := json.Marshal(intake.Warnings)
	if err != nil {
		return CreativeIntake{}, false, err
	}
	strategyPackageID := ""
	strategyPackageVersion := int64(0)
	strategyPackageHash := ""
	if intake.Request.StrategyPackage != nil {
		strategyPackageID = intake.Request.StrategyPackage.PackageID
		strategyPackageVersion = intake.Request.StrategyPackage.PackageVersion
		strategyPackageHash = intake.Request.StrategyPackage.ExpectedContentHash
	}
	intakeContractVersion := intake.ContractVersion
	if intakeContractVersion == "" {
		intakeContractVersion = "creative-intake/v1"
	}
	selectedRouteID := intake.Request.SelectedRouteID
	handoffContractVersion := ""
	handoffContentHash := ""
	if intake.Request.StrategyPackage != nil {
		handoffContractVersion = intake.Request.StrategyPackage.HandoffContractVersion
		handoffContentHash = intake.Request.StrategyPackage.ExpectedHandoffHash
	}
	taskOverlayID := ""
	taskOverlayHash := ""
	if intake.Request.TaskOverlay != nil {
		taskOverlayID = intake.Request.TaskOverlay.OverlayID
		taskOverlayHash = intake.Request.TaskOverlay.ExpectedContentHash
	}
	taskStrategyPlanID := ""
	taskStrategyVersion := int64(0)
	taskStrategyHash := ""
	if intake.Request.TaskStrategy != nil {
		taskStrategyPlanID = intake.Request.TaskStrategy.PlanID
		taskStrategyVersion = intake.Request.TaskStrategy.StrategyVersion
		taskStrategyHash = intake.Request.TaskStrategy.ExpectedContentHash
	}
	requirementBriefID := ""
	requirementBriefVersion := int64(0)
	requirementContentHash := ""
	if intake.Request.RequirementSnapshotRef != nil {
		requirementBriefID = intake.Request.RequirementSnapshotRef.BriefID
		requirementBriefVersion = intake.Request.RequirementSnapshotRef.BriefVersion
		requirementContentHash = intake.Request.RequirementSnapshotRef.ContentHash
	}
	businessCode := ""
	businessVersion := ""
	businessContentHash := ""
	if intake.Request.BusinessCapabilityRef != nil {
		businessCode = intake.Request.BusinessCapabilityRef.BusinessCode
		businessVersion = intake.Request.BusinessCapabilityRef.Version
		businessContentHash = intake.Request.BusinessCapabilityRef.ContentHash
	}
	_, err = r.DB.ExecContext(ctx, `INSERT INTO creative_intakes (
		id, organization_id, project_id, principal_kind, principal_id, source_type, status,
		request_payload, missing_fields, warnings, confirmed_by, idempotency_key, request_hash,
		strategy_package_id, strategy_package_version, strategy_package_content_hash,
		task_strategy_plan_id, task_strategy_version, task_strategy_content_hash,
		requirement_brief_id, requirement_brief_version, requirement_content_hash,
		business_code, business_version, business_content_hash,
		contract_version, selected_route_id, handoff_contract_version,
		handoff_content_hash, task_overlay_id, task_overlay_content_hash,
		task_overlay_identity, input_identity_hash,
		version, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?,
		NULLIF(?, ''), NULLIF(?, 0), NULLIF(?, ''),
		NULLIF(?, ''), NULLIF(?, 0), NULLIF(?, ''),
		NULLIF(?, ''), NULLIF(?, 0), NULLIF(?, ''),
		NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''),
		?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''),
		NULLIF(?, ''), NULLIF(?, ''), ?, NULLIF(?, ''),
		?, ?, ?)`,
		intake.ID, intake.OrganizationID, intake.ProjectID, intake.Principal.Kind, intake.Principal.ID, intake.Source, intake.Status,
		request, missing, warnings, intake.ConfirmedBy, intake.IdempotencyKey, intake.RequestHash,
		strategyPackageID, strategyPackageVersion, strategyPackageHash,
		taskStrategyPlanID, taskStrategyVersion, taskStrategyHash,
		requirementBriefID, requirementBriefVersion, requirementContentHash,
		businessCode, businessVersion, businessContentHash,
		intakeContractVersion, selectedRouteID, handoffContractVersion,
		handoffContentHash, taskOverlayID, taskOverlayHash, taskOverlayID,
		intake.InputIdentityHash,
		intake.Version, intake.CreatedAt, intake.UpdatedAt)
	if err == nil {
		return intake, false, nil
	}
	var mysqlError *mysqlDriver.MySQLError
	if !errors.As(err, &mysqlError) || mysqlError.Number != 1062 {
		return CreativeIntake{}, false, err
	}
	existing, getErr := r.getIntakeByIdempotency(ctx, intake)
	if getErr == nil {
		if existing.RequestHash != intake.RequestHash {
			return CreativeIntake{}, false, ErrIdempotencyConflict
		}
		return existing, true, nil
	}
	if intake.InputIdentityHash != "" {
		existing, identityErr := r.getIntakeByInputIdentity(ctx, intake.OrganizationID, intake.ProjectID, intake.Source, intake.InputIdentityHash)
		if identityErr == nil {
			return existing, true, nil
		}
		if !errors.Is(identityErr, sql.ErrNoRows) {
			return CreativeIntake{}, false, identityErr
		}
	}
	if intake.Source == IntakeSourceStrategyPackage && intake.Request.StrategyPackage != nil {
		existing, packageErr := r.getIntakeByStrategyPackage(ctx, intake.OrganizationID, intake.ProjectID, intake.Request)
		if packageErr == nil {
			return existing, true, nil
		}
		if !errors.Is(packageErr, sql.ErrNoRows) {
			return CreativeIntake{}, false, packageErr
		}
	}
	if intake.Source == IntakeSourceTaskStrategy && intake.Request.TaskStrategy != nil {
		existing, strategyErr := r.getIntakeByTaskStrategy(ctx, intake.OrganizationID, intake.ProjectID, *intake.Request.TaskStrategy)
		if strategyErr == nil {
			return existing, true, nil
		}
		if !errors.Is(strategyErr, sql.ErrNoRows) {
			return CreativeIntake{}, false, strategyErr
		}
	}
	return CreativeIntake{}, false, getErr
}

func (r MySQLRepository) ListIntakes(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, limit int) ([]CreativeIntake, error) {
	if r.DB == nil {
		return nil, fmt.Errorf("creative MySQL database is required")
	}
	rows, err := r.DB.QueryContext(ctx, creativeIntakeSelect+` WHERE organization_id = ? AND project_id = ? ORDER BY created_at DESC LIMIT ?`, organizationID, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]CreativeIntake, 0)
	for rows.Next() {
		value, scanErr := scanIntake(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r MySQLRepository) GetIntake(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, intakeID string) (CreativeIntake, error) {
	if r.DB == nil {
		return CreativeIntake{}, fmt.Errorf("creative MySQL database is required")
	}
	value, err := scanIntake(r.DB.QueryRowContext(ctx, creativeIntakeSelect+` WHERE organization_id = ? AND project_id = ? AND id = ?`, organizationID, projectID, intakeID))
	if errors.Is(err, sql.ErrNoRows) {
		return CreativeIntake{}, ErrNotFound
	}
	return value, err
}

func (r MySQLRepository) UpdateIntakeReadiness(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	intakeID string,
	expectedVersion int64,
	status IntakeStatus,
	missingFields []string,
	confirmedBy string,
	updatedAt time.Time,
) (CreativeIntake, error) {
	if r.DB == nil {
		return CreativeIntake{}, fmt.Errorf("creative MySQL database is required")
	}
	missing, err := json.Marshal(missingFields)
	if err != nil {
		return CreativeIntake{}, err
	}
	result, err := r.DB.ExecContext(ctx, `UPDATE creative_intakes
		SET status = ?, missing_fields = ?, confirmed_by = NULLIF(?, ''),
			version = version + 1, updated_at = ?
		WHERE organization_id = ? AND project_id = ? AND id = ? AND version = ?`,
		status, missing, confirmedBy, updatedAt,
		organizationID, projectID, intakeID, expectedVersion,
	)
	if err != nil {
		return CreativeIntake{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return CreativeIntake{}, err
	}
	if affected != 1 {
		return CreativeIntake{}, ErrVersionConflict
	}
	return r.GetIntake(ctx, organizationID, projectID, intakeID)
}

func (r MySQLRepository) CreateTask(ctx context.Context, task CreativeTask, draft ImageTextDraft) (CreativeTask, error) {
	if r.DB == nil {
		return CreativeTask{}, fmt.Errorf("creative MySQL database is required")
	}
	direction, err := json.Marshal(task.Direction)
	if err != nil {
		return CreativeTask{}, err
	}
	content, err := json.Marshal(draft)
	if err != nil {
		return CreativeTask{}, err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return CreativeTask{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO creative_tasks (
		id, display_name, organization_id, project_id, intake_id, creative_format, channel, lineage_key, status, direction_payload, version, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)`,
		task.ID, task.DisplayName, task.OrganizationID, task.ProjectID, task.IntakeID, task.Format, task.Channel, task.LineageKey, task.Status, direction, task.Version, task.CreatedAt, task.UpdatedAt)
	if err != nil {
		return CreativeTask{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO creative_image_text_drafts (organization_id, task_id, version, status, content_payload, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		task.OrganizationID, task.ID, draft.Version, draft.Status, content, draft.CreatedAt)
	if err != nil {
		return CreativeTask{}, err
	}
	if err := tx.Commit(); err != nil {
		return CreativeTask{}, err
	}
	return task, nil
}

func (r MySQLRepository) CreateVideoTask(ctx context.Context, task CreativeTask, draft VideoDraft) (CreativeTask, error) {
	if r.DB == nil {
		return CreativeTask{}, fmt.Errorf("creative MySQL database is required")
	}
	if task.Format != FormatVideo || draft.TaskID != task.ID {
		return CreativeTask{}, fmt.Errorf("creative video task and draft do not match")
	}
	direction, err := json.Marshal(task.Direction)
	if err != nil {
		return CreativeTask{}, err
	}
	content, err := json.Marshal(draft)
	if err != nil {
		return CreativeTask{}, err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return CreativeTask{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO creative_tasks (
		id, display_name, organization_id, project_id, intake_id, creative_format, channel, video_purpose, performance_mode,
		lineage_key, status, direction_payload, version, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)`,
		task.ID, task.DisplayName, task.OrganizationID, task.ProjectID, task.IntakeID, task.Format, task.Channel,
		task.VideoPurpose, task.PerformanceMode, task.LineageKey, task.Status, direction, task.Version, task.CreatedAt, task.UpdatedAt)
	if err != nil {
		var mysqlError *mysqlDriver.MySQLError
		if task.LineageKey != "" && errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
			_ = tx.Rollback()
			return scanTask(r.DB.QueryRowContext(ctx, creativeTaskSelect+` WHERE organization_id = ? AND project_id = ? AND lineage_key = ?`, task.OrganizationID, task.ProjectID, task.LineageKey))
		}
		return CreativeTask{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO creative_video_drafts
		(organization_id, task_id, revision, content_payload, created_at) VALUES (?, ?, ?, ?, ?)`,
		task.OrganizationID, task.ID, draft.Revision, content, draft.CreatedAt)
	if err != nil {
		return CreativeTask{}, err
	}
	if err := tx.Commit(); err != nil {
		return CreativeTask{}, err
	}
	return task, nil
}

func (r MySQLRepository) ReviseVideoDraft(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, taskID string, expectedRevision int64, draft VideoDraft, status TaskStatus) (VideoDraft, error) {
	if r.DB == nil {
		return VideoDraft{}, fmt.Errorf("creative MySQL database is required")
	}
	if draft.TaskID != taskID || draft.Revision != expectedRevision+1 {
		return VideoDraft{}, fmt.Errorf("next creative video draft revision is invalid")
	}
	if err := draft.Validate(); err != nil {
		return VideoDraft{}, fmt.Errorf("next creative video draft revision is invalid: %w", err)
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return VideoDraft{}, err
	}
	defer tx.Rollback()
	var current int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision), 0) FROM creative_video_drafts WHERE organization_id = ? AND task_id = ? FOR UPDATE`, organizationID, taskID).Scan(&current); err != nil {
		return VideoDraft{}, err
	}
	if current != expectedRevision {
		return VideoDraft{}, ErrVersionConflict
	}
	content, err := json.Marshal(draft)
	if err != nil {
		return VideoDraft{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO creative_video_drafts
		(organization_id, task_id, revision, content_payload, created_at) VALUES (?, ?, ?, ?, ?)`,
		organizationID, taskID, draft.Revision, content, draft.CreatedAt); err != nil {
		return VideoDraft{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE creative_tasks SET status = ?, version = version + 1, updated_at = ?
		WHERE organization_id = ? AND project_id = ? AND id = ?`,
		status, draft.CreatedAt, organizationID, projectID, taskID)
	if err != nil {
		return VideoDraft{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return VideoDraft{}, err
	}
	if affected != 1 {
		return VideoDraft{}, ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return VideoDraft{}, err
	}
	return draft, nil
}

func (r MySQLRepository) ListTasks(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, limit int) ([]CreativeTask, error) {
	if r.DB == nil {
		return nil, fmt.Errorf("creative MySQL database is required")
	}
	rows, err := r.DB.QueryContext(ctx, creativeTaskSelect+` WHERE organization_id = ? AND project_id = ? AND status <> ? ORDER BY created_at DESC LIMIT ?`, organizationID, projectID, TaskArchived, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]CreativeTask, 0)
	for rows.Next() {
		value, scanErr := scanTask(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r MySQLRepository) ArchiveTask(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, taskID string, now time.Time) error {
	if r.DB == nil {
		return fmt.Errorf("creative MySQL database is required")
	}
	result, err := r.DB.ExecContext(ctx, `UPDATE creative_tasks SET status = ?, updated_at = ? WHERE organization_id = ? AND project_id = ? AND id = ? AND status <> ?`, TaskArchived, now, organizationID, projectID, taskID, TaskArchived)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r MySQLRepository) RenameTask(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, taskID string, expectedVersion int64, displayName string, now time.Time) (CreativeTask, error) {
	if r.DB == nil {
		return CreativeTask{}, fmt.Errorf("creative MySQL database is required")
	}
	result, err := r.DB.ExecContext(ctx, `UPDATE creative_tasks SET display_name = ?, version = version + 1, updated_at = ?
		WHERE organization_id = ? AND project_id = ? AND id = ? AND version = ? AND status <> ?`,
		displayName, now, organizationID, projectID, taskID, expectedVersion, TaskArchived)
	if err != nil {
		return CreativeTask{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return CreativeTask{}, err
	}
	if affected != 1 {
		var exists int
		if scanErr := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM creative_tasks WHERE organization_id = ? AND project_id = ? AND id = ?`, organizationID, projectID, taskID).Scan(&exists); scanErr != nil {
			return CreativeTask{}, scanErr
		}
		if exists == 0 {
			return CreativeTask{}, ErrNotFound
		}
		return CreativeTask{}, ErrVersionConflict
	}
	return scanTask(r.DB.QueryRowContext(ctx, creativeTaskSelect+` WHERE organization_id = ? AND project_id = ? AND id = ?`, organizationID, projectID, taskID))
}

func (r MySQLRepository) GetTaskDetail(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, taskID string) (TaskDetail, error) {
	if r.DB == nil {
		return TaskDetail{}, fmt.Errorf("creative MySQL database is required")
	}
	task, err := scanTask(r.DB.QueryRowContext(ctx, creativeTaskSelect+` WHERE organization_id = ? AND project_id = ? AND id = ?`, organizationID, projectID, taskID))
	if errors.Is(err, sql.ErrNoRows) {
		return TaskDetail{}, ErrNotFound
	}
	if err != nil {
		return TaskDetail{}, err
	}
	intake, err := r.GetIntake(ctx, organizationID, projectID, task.IntakeID)
	if err != nil {
		return TaskDetail{}, err
	}
	var draft ImageTextDraft
	var videoDraft *VideoDraft
	var aiNativeWorkspaceID string
	if task.Format == FormatVideo && task.PerformanceMode == PerformanceModeAINativeAd {
		if err = r.DB.QueryRowContext(ctx, `SELECT workspace_id FROM creative_ai_native_requirement_workspaces
			WHERE organization_id=? AND project_id=? AND creative_task_id=?`, organizationID, projectID, taskID).Scan(&aiNativeWorkspaceID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return TaskDetail{}, ErrNotFound
			}
			return TaskDetail{}, err
		}
	} else if task.Format == FormatVideo {
		value, videoErr := r.getLatestVideoDraft(ctx, organizationID, taskID)
		if videoErr != nil {
			return TaskDetail{}, videoErr
		}
		videoDraft = &value
	} else {
		draft, err = r.getLatestDraft(ctx, organizationID, taskID)
		if err != nil {
			return TaskDetail{}, err
		}
	}
	jobs, err := r.productionJobs(ctx, organizationID, projectID, taskID)
	if err != nil {
		return TaskDetail{}, err
	}
	attempts, err := r.shortDramaGenerationAttempts(ctx, organizationID, projectID, taskID)
	if err != nil {
		return TaskDetail{}, err
	}
	gameAttempts, err := r.gamePrerollGenerationAttempts(ctx, organizationID, projectID, taskID)
	if err != nil {
		return TaskDetail{}, err
	}
	return TaskDetail{
		Task: task, Intake: intake, Draft: draft, VideoDraft: videoDraft, AINativeWorkspaceID: aiNativeWorkspaceID,
		ProductionJobs: jobs, ShortDramaGenerationAttempts: attempts,
		GamePrerollGenerationAttempts: gameAttempts,
	}, nil
}

func (r MySQLRepository) ReviseDraft(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, taskID string, expectedVersion int64, draft ImageTextDraft) (ImageTextDraft, error) {
	if r.DB == nil {
		return ImageTextDraft{}, fmt.Errorf("creative MySQL database is required")
	}
	payload, err := json.Marshal(draft)
	if err != nil {
		return ImageTextDraft{}, fmt.Errorf("encode creative draft revision: %w", err)
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return ImageTextDraft{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE creative_tasks SET version = ?, status = ?, updated_at = ?
		WHERE organization_id = ? AND project_id = ? AND id = ? AND version = ?`,
		draft.Version, TaskDraft, draft.CreatedAt, organizationID, projectID, taskID, expectedVersion)
	if err != nil {
		return ImageTextDraft{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ImageTextDraft{}, err
	}
	if affected != 1 {
		var exists int
		err = tx.QueryRowContext(ctx, `SELECT 1 FROM creative_tasks WHERE organization_id = ? AND project_id = ? AND id = ?`, organizationID, projectID, taskID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return ImageTextDraft{}, ErrNotFound
		}
		if err != nil {
			return ImageTextDraft{}, err
		}
		return ImageTextDraft{}, ErrVersionConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO creative_image_text_drafts (organization_id, task_id, version, status, content_payload, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, organizationID, taskID, draft.Version, draft.Status, payload, draft.CreatedAt)
	if err != nil {
		return ImageTextDraft{}, err
	}
	if err := tx.Commit(); err != nil {
		return ImageTextDraft{}, err
	}
	return draft, nil
}

func (r MySQLRepository) CreateRenderJob(ctx context.Context, value RenderJob) (RenderJob, bool, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return RenderJob{}, false, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO creative_render_jobs
		(id, organization_id, project_id, task_id, status, pre_roll_asset_id, pre_roll_asset_version,
		 main_asset_id, main_asset_version, created_by_kind, created_by_id, idempotency_key, request_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID, value.OrganizationID, value.ProjectID, value.TaskID, value.Status,
		value.PreRollVideo.AssetID, value.PreRollVideo.Version, value.MainVideo.AssetID, value.MainVideo.Version,
		value.CreatedBy.Kind, value.CreatedBy.ID, value.IdempotencyKey, value.RequestHash, value.CreatedAt, value.UpdatedAt)
	if err == nil {
		if err = ensureInitialRenderObservability(ctx, tx, ProductionSourceCreativeRender, value.OrganizationID, value.ProjectID, value.ID, value.CreatedAt); err != nil {
			return RenderJob{}, false, err
		}
		if err = tx.Commit(); err != nil {
			return RenderJob{}, false, err
		}
		return value, false, nil
	}
	_ = tx.Rollback()
	var mysqlError *mysqlDriver.MySQLError
	if !errors.As(err, &mysqlError) || mysqlError.Number != 1062 {
		return RenderJob{}, false, err
	}
	existing, getErr := scanRenderJob(r.DB.QueryRowContext(ctx, creativeRenderSelect+`
		WHERE organization_id=? AND project_id=? AND created_by_kind=? AND created_by_id=? AND idempotency_key=?`,
		value.OrganizationID, value.ProjectID, value.CreatedBy.Kind, value.CreatedBy.ID, value.IdempotencyKey))
	if getErr != nil {
		return RenderJob{}, false, getErr
	}
	if existing.RequestHash != value.RequestHash {
		return RenderJob{}, false, ErrIdempotencyConflict
	}
	if err = ensureInitialRenderObservability(ctx, r.DB, ProductionSourceCreativeRender, existing.OrganizationID, existing.ProjectID, existing.ID, existing.CreatedAt); err != nil {
		return RenderJob{}, false, err
	}
	existing.ProductionUsage, existing.ProductionEvents, err = r.loadRenderObservability(ctx, ProductionSourceCreativeRender, existing.OrganizationID, existing.ProjectID, existing.ID)
	if err != nil {
		return RenderJob{}, false, err
	}
	return existing, true, nil
}

func (r MySQLRepository) GetRenderJob(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, id string) (RenderJob, error) {
	value, err := scanRenderJob(r.DB.QueryRowContext(ctx, creativeRenderSelect+` WHERE organization_id=? AND project_id=? AND id=?`, organizationID, projectID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return RenderJob{}, ErrNotFound
	}
	if err != nil {
		return RenderJob{}, err
	}
	value.ProductionUsage, value.ProductionEvents, err = r.loadRenderObservability(ctx, ProductionSourceCreativeRender, organizationID, projectID, id)
	return value, err
}

func (r MySQLRepository) MarkRenderRunning(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, id string, now time.Time) (RenderJob, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return RenderJob{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE creative_render_jobs SET status='running', error_code=NULL, error_message=NULL, updated_at=?
		WHERE organization_id=? AND project_id=? AND id=? AND status IN ('queued','running')`, now, organizationID, projectID, id)
	if err != nil {
		return RenderJob{}, err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return RenderJob{}, ErrInvalidState
	}
	if err = appendRenderLifecycleEvent(ctx, tx, ProductionSourceCreativeRender, organizationID, projectID, id, string(RenderRunning), "", now); err != nil {
		return RenderJob{}, err
	}
	if err = tx.Commit(); err != nil {
		return RenderJob{}, err
	}
	return r.GetRenderJob(ctx, organizationID, projectID, id)
}

func (r MySQLRepository) CompleteRenderJob(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, id string, ref contract.ProjectAssetRef, now time.Time) error {
	if ref.ProjectID != projectID || ref.Validate() != nil {
		return ErrInvalidState
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE creative_render_jobs
		SET status='succeeded', output_asset_id=?, output_asset_version=?, error_code=NULL, error_message=NULL, updated_at=?
		WHERE organization_id=? AND project_id=? AND id=? AND status='running'`,
		ref.AssetVersion.AssetID, ref.AssetVersion.Version, now, organizationID, projectID, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return ErrInvalidState
	}
	if err = appendRenderLifecycleEvent(ctx, tx, ProductionSourceCreativeRender, organizationID, projectID, id, string(RenderSucceeded), "", now); err != nil {
		return err
	}
	return tx.Commit()
}

func (r MySQLRepository) FailRenderJob(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, id, code, message string, now time.Time) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE creative_render_jobs SET status='failed', error_code=?, error_message=?, updated_at=?
		WHERE organization_id=? AND project_id=? AND id=? AND status IN ('queued','running')`,
		code, message, now, organizationID, projectID, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return ErrInvalidState
	}
	if err = appendRenderLifecycleEvent(ctx, tx, ProductionSourceCreativeRender, organizationID, projectID, id, string(RenderFailed), code, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (r MySQLRepository) RegisterProductionJob(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, taskID string, job ProductionJob) error {
	if r.DB == nil {
		return fmt.Errorf("creative MySQL database is required")
	}
	_, err := r.DB.ExecContext(ctx, `INSERT INTO creative_production_jobs (organization_id, project_id, task_id, job_kind, provider_job_id, created_at) VALUES (?, ?, ?, ?, ?, ?)`, organizationID, projectID, taskID, job.Kind, job.ProviderJobID, job.CreatedAt)
	if err == nil {
		return nil
	}
	var mysqlError *mysqlDriver.MySQLError
	if !errors.As(err, &mysqlError) || mysqlError.Number != 1062 {
		return err
	}
	var existing string
	readErr := r.DB.QueryRowContext(ctx, `SELECT provider_job_id FROM creative_production_jobs WHERE organization_id = ? AND task_id = ? AND job_kind = ?`, organizationID, taskID, job.Kind).Scan(&existing)
	if readErr != nil {
		return readErr
	}
	if existing != job.ProviderJobID {
		return ErrProviderJobConflict
	}
	return nil
}

func (r MySQLRepository) CreateShortDramaGenerationAttempt(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	attempt ShortDramaGenerationAttempt,
) (ShortDramaGenerationAttempt, error) {
	if r.DB == nil {
		return ShortDramaGenerationAttempt{}, fmt.Errorf("creative MySQL database is required")
	}
	_, err := r.DB.ExecContext(ctx, `INSERT INTO creative_short_drama_generation_attempts (
		id, organization_id, project_id, task_id, draft_revision, candidate_batch_id, candidate_id,
		prompt_package_hash, generation_spec_hash, provider_job_id, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		attempt.ID, organizationID, projectID, attempt.TaskID, attempt.DraftRevision,
		attempt.CandidateBatchID, attempt.CandidateID, attempt.PromptPackageHash,
		attempt.GenerationSpecHash, attempt.ProviderJobID, attempt.CreatedAt,
	)
	if err == nil {
		return attempt, nil
	}
	var mysqlError *mysqlDriver.MySQLError
	if !errors.As(err, &mysqlError) || mysqlError.Number != 1062 {
		return ShortDramaGenerationAttempt{}, err
	}
	return r.shortDramaGenerationAttemptByProviderJob(ctx, organizationID, projectID, attempt.ProviderJobID)
}

func (r MySQLRepository) CreateGamePrerollGenerationAttempt(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	attempt GamePrerollGenerationAttempt,
) (GamePrerollGenerationAttempt, error) {
	if r.DB == nil {
		return GamePrerollGenerationAttempt{}, fmt.Errorf("creative MySQL database is required")
	}
	_, err := r.DB.ExecContext(ctx, `INSERT INTO creative_game_preroll_generation_attempts (
		id, organization_id, project_id, task_id, draft_revision, candidate_batch_id, candidate_id,
		prompt_package_hash, generation_spec_hash, provider_job_id, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		attempt.ID, organizationID, projectID, attempt.TaskID, attempt.DraftRevision,
		attempt.CandidateBatchID, attempt.CandidateID, attempt.PromptPackageHash,
		attempt.GenerationSpecHash, attempt.ProviderJobID, attempt.CreatedAt,
	)
	if err == nil {
		return attempt, nil
	}
	var mysqlError *mysqlDriver.MySQLError
	if !errors.As(err, &mysqlError) || mysqlError.Number != 1062 {
		return GamePrerollGenerationAttempt{}, err
	}
	return r.gamePrerollGenerationAttemptByProviderJob(ctx, organizationID, projectID, attempt.ProviderJobID)
}

func (r MySQLRepository) CreateVersion(ctx context.Context, value CreativeVersion) (CreativeVersion, bool, error) {
	if r.DB == nil {
		return CreativeVersion{}, false, fmt.Errorf("creative MySQL database is required")
	}
	snapshot, err := json.Marshal(value.Snapshot)
	if err != nil {
		return CreativeVersion{}, false, fmt.Errorf("encode creative version snapshot: %w", err)
	}
	var videoSnapshot any
	if value.VideoSnapshot != nil {
		videoSnapshot, err = json.Marshal(value.VideoSnapshot)
		if err != nil {
			return CreativeVersion{}, false, fmt.Errorf("encode creative video version snapshot: %w", err)
		}
	}
	_, err = r.DB.ExecContext(ctx, `INSERT INTO creative_versions (
		id, organization_id, project_id, task_id, edit_task_id, version, draft_version, creative_format, status,
		snapshot_payload, video_snapshot_payload, content_hash, created_by, idempotency_key, request_hash, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID, value.OrganizationID, value.ProjectID, sql.NullString{String: value.TaskID, Valid: value.TaskID != ""}, sql.NullString{String: value.EditTaskID, Valid: value.EditTaskID != ""}, value.Version, value.DraftVersion,
		value.Format, value.Status, snapshot, videoSnapshot, value.ContentHash, value.CreatedBy, value.IdempotencyKey, value.RequestHash, value.CreatedAt)
	if err == nil {
		return value, false, nil
	}
	var mysqlError *mysqlDriver.MySQLError
	if !errors.As(err, &mysqlError) || mysqlError.Number != 1062 {
		return CreativeVersion{}, false, err
	}
	replayed, readErr := r.getVersionByIdempotency(ctx, value)
	if readErr == nil {
		if replayed.RequestHash != value.RequestHash {
			return CreativeVersion{}, false, ErrIdempotencyConflict
		}
		return replayed, true, nil
	}
	var existing CreativeVersion
	if value.EditTaskID != "" {
		existing, readErr = r.getVersionByEditTimeline(ctx, value.OrganizationID, value.ProjectID, value.EditTaskID, value.DraftVersion)
	} else {
		existing, readErr = r.getVersionByTaskDraft(ctx, value.OrganizationID, value.ProjectID, value.TaskID, value.DraftVersion)
	}
	if readErr != nil {
		return CreativeVersion{}, false, readErr
	}
	if !existing.ContentHash.Equal(value.ContentHash) {
		return CreativeVersion{}, false, ErrVersionConflict
	}
	return existing, false, nil
}

func (r MySQLRepository) GetVersion(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, versionID string) (CreativeVersion, error) {
	if r.DB == nil {
		return CreativeVersion{}, fmt.Errorf("creative MySQL database is required")
	}
	value, err := scanCreativeVersion(r.DB.QueryRowContext(ctx, creativeVersionSelect+` WHERE organization_id = ? AND project_id = ? AND id = ?`, organizationID, projectID, versionID))
	if errors.Is(err, sql.ErrNoRows) {
		return CreativeVersion{}, ErrNotFound
	}
	return value, err
}

func (r MySQLRepository) ListVersions(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, taskID string, limit int) ([]CreativeVersion, error) {
	query := creativeVersionSelect + ` WHERE organization_id = ? AND project_id = ?`
	args := []any{organizationID, projectID}
	if taskID != "" {
		query += ` AND (task_id = ? OR edit_task_id = ?)`
		args = append(args, taskID, taskID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]CreativeVersion, 0)
	for rows.Next() {
		value, scanErr := scanCreativeVersion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r MySQLRepository) RecordVersionCheck(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, versionID string, check CreativeCheck) (CreativeVersion, error) {
	payload, err := json.Marshal(check)
	if err != nil {
		return CreativeVersion{}, err
	}
	result, err := r.DB.ExecContext(ctx, `UPDATE creative_versions SET status = ?, check_payload = ? WHERE organization_id = ? AND project_id = ? AND id = ? AND status IN (?, ?)`, CreativeVersionChecked, payload, organizationID, projectID, versionID, CreativeVersionCreated, CreativeVersionChecked)
	if err != nil {
		return CreativeVersion{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return CreativeVersion{}, err
	}
	if affected == 0 {
		if _, getErr := r.GetVersion(ctx, organizationID, projectID, versionID); getErr != nil {
			return CreativeVersion{}, getErr
		}
		return CreativeVersion{}, ErrInvalidState
	}
	return r.GetVersion(ctx, organizationID, projectID, versionID)
}

func (r MySQLRepository) ApproveVersion(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, versionID string, approval CreativeApproval) (CreativeVersion, error) {
	payload, err := json.Marshal(approval)
	if err != nil {
		return CreativeVersion{}, err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return CreativeVersion{}, err
	}
	defer tx.Rollback()
	var version CreativeVersion
	version, err = scanCreativeVersion(tx.QueryRowContext(ctx, creativeVersionSelect+` WHERE organization_id = ? AND project_id = ? AND id = ? FOR UPDATE`, organizationID, projectID, versionID))
	if errors.Is(err, sql.ErrNoRows) {
		return CreativeVersion{}, ErrNotFound
	}
	if err != nil {
		return CreativeVersion{}, err
	}
	if version.Status != CreativeVersionChecked || version.Check == nil || !version.Check.Passed {
		return CreativeVersion{}, ErrInvalidState
	}
	_, err = tx.ExecContext(ctx, `UPDATE creative_versions SET status = ?, approval_payload = ? WHERE organization_id = ? AND project_id = ? AND id = ?`, CreativeVersionApproved, payload, organizationID, projectID, versionID)
	if err != nil {
		return CreativeVersion{}, err
	}
	eventPayload, err := json.Marshal(map[string]any{"creative_version_id": versionID, "content_hash": version.ContentHash})
	if err != nil {
		return CreativeVersion{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO event_outbox (event_id, organization_id, project_id, event_type, subject_type, subject_id, subject_version, payload, available_at, created_at)
		VALUES (?, ?, ?, 'creative.approved.v1', 'creative_version', ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE event_id = event_id`, "creative-approved-"+versionID, organizationID, projectID, versionID, version.Version, eventPayload, approval.ApprovedAt, approval.ApprovedAt)
	if err != nil {
		return CreativeVersion{}, err
	}
	if err := tx.Commit(); err != nil {
		return CreativeVersion{}, err
	}
	return r.GetVersion(ctx, organizationID, projectID, versionID)
}

func (r MySQLRepository) CreatePackage(ctx context.Context, value CreativePackage) (CreativePackage, error) {
	snapshot, err := json.Marshal(value.Snapshot)
	if err != nil {
		return CreativePackage{}, err
	}
	var videoSnapshot any
	if value.VideoSnapshot != nil {
		videoSnapshot, err = json.Marshal(value.VideoSnapshot)
		if err != nil {
			return CreativePackage{}, err
		}
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return CreativePackage{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO creative_packages (id, organization_id, project_id, creative_version_id, edit_task_id, creative_format, content_hash, snapshot_payload, video_snapshot_payload, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, value.OrganizationID, value.ProjectID, value.CreativeVersionID, sql.NullString{String: value.EditTaskID, Valid: value.EditTaskID != ""}, value.Format, value.ContentHash, snapshot, videoSnapshot, value.CreatedBy, value.CreatedAt)
	if err != nil {
		var mysqlError *mysqlDriver.MySQLError
		if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
			packageValue, readErr := r.getPackageByVersion(ctx, value.OrganizationID, value.ProjectID, value.CreativeVersionID)
			if readErr != nil {
				return CreativePackage{}, readErr
			}
			return packageValue, nil
		}
		return CreativePackage{}, err
	}
	eventPayload, err := json.Marshal(map[string]any{"creative_package_id": value.ID, "creative_version_id": value.CreativeVersionID, "content_hash": value.ContentHash})
	if err != nil {
		return CreativePackage{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO event_outbox (event_id, organization_id, project_id, event_type, subject_type, subject_id, subject_version, payload, available_at, created_at)
		VALUES (?, ?, ?, 'creative.delivered.v1', 'creative_package', ?, 1, ?, ?, ?)
		ON DUPLICATE KEY UPDATE event_id = event_id`, "creative-delivered-"+value.ID, value.OrganizationID, value.ProjectID, value.ID, eventPayload, value.CreatedAt, value.CreatedAt)
	if err != nil {
		return CreativePackage{}, err
	}
	if err := tx.Commit(); err != nil {
		return CreativePackage{}, err
	}
	return value, nil
}

func (r MySQLRepository) ListPackages(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, limit int) ([]CreativePackage, error) {
	rows, err := r.DB.QueryContext(ctx, creativePackageSelect+` WHERE organization_id = ? AND project_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`, organizationID, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]CreativePackage, 0)
	for rows.Next() {
		value, scanErr := scanCreativePackage(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

const creativeIntakeSelect = `SELECT id, organization_id, project_id, principal_kind, principal_id, source_type, status,
	request_payload, missing_fields, warnings, confirmed_by, idempotency_key, request_hash,
	contract_version, COALESCE(input_identity_hash, ''), version, created_at, updated_at FROM creative_intakes`
const creativeTaskSelect = `SELECT id, display_name, organization_id, project_id, intake_id, creative_format, channel, COALESCE(video_purpose, ''), COALESCE(performance_mode, ''), COALESCE(lineage_key, ''), status, direction_payload, version, created_at, updated_at FROM creative_tasks`
const creativeVersionSelect = `SELECT id, organization_id, project_id, task_id, edit_task_id, version, draft_version, status,
	creative_format, snapshot_payload, video_snapshot_payload, content_hash, created_by, idempotency_key, request_hash, created_at, check_payload, approval_payload FROM creative_versions`
const creativePackageSelect = `SELECT id, organization_id, project_id, creative_version_id, edit_task_id, creative_format, content_hash, snapshot_payload, video_snapshot_payload, created_by, created_at FROM creative_packages`
const creativeRenderSelect = `SELECT id, organization_id, project_id, task_id, status,
	pre_roll_asset_id, pre_roll_asset_version, main_asset_id, main_asset_version,
	output_asset_id, output_asset_version, error_code, error_message,
	created_by_kind, created_by_id, idempotency_key, request_hash, created_at, updated_at
	FROM creative_render_jobs`

type rowScanner interface{ Scan(...any) error }

func scanIntake(row rowScanner) (CreativeIntake, error) {
	var value CreativeIntake
	var request, missing, warnings []byte
	var confirmed sql.NullString
	err := row.Scan(&value.ID, &value.OrganizationID, &value.ProjectID, &value.Principal.Kind, &value.Principal.ID, &value.Source, &value.Status,
		&request, &missing, &warnings, &confirmed, &value.IdempotencyKey, &value.RequestHash,
		&value.ContractVersion, &value.InputIdentityHash, &value.Version, &value.CreatedAt, &value.UpdatedAt)
	if err != nil {
		return CreativeIntake{}, err
	}
	if err := json.Unmarshal(request, &value.Request); err != nil {
		return CreativeIntake{}, fmt.Errorf("decode creative intake request: %w", err)
	}
	if err := json.Unmarshal(missing, &value.MissingFields); err != nil {
		return CreativeIntake{}, fmt.Errorf("decode creative intake missing fields: %w", err)
	}
	if err := json.Unmarshal(warnings, &value.Warnings); err != nil {
		return CreativeIntake{}, fmt.Errorf("decode creative intake warnings: %w", err)
	}
	value.ConfirmedBy = confirmed.String
	return value, nil
}

func scanCreativePackage(row rowScanner) (CreativePackage, error) {
	var value CreativePackage
	var editTaskID sql.NullString
	var snapshot []byte
	var videoSnapshot []byte
	if err := row.Scan(&value.ID, &value.OrganizationID, &value.ProjectID, &value.CreativeVersionID, &editTaskID, &value.Format, &value.ContentHash, &snapshot, &videoSnapshot, &value.CreatedBy, &value.CreatedAt); err != nil {
		return CreativePackage{}, err
	}
	value.EditTaskID = editTaskID.String
	if err := json.Unmarshal(snapshot, &value.Snapshot); err != nil {
		return CreativePackage{}, fmt.Errorf("decode creative package snapshot: %w", err)
	}
	if len(videoSnapshot) > 0 {
		value.VideoSnapshot = &VideoVersionSnapshot{}
		if err := json.Unmarshal(videoSnapshot, value.VideoSnapshot); err != nil {
			return CreativePackage{}, fmt.Errorf("decode creative package video snapshot: %w", err)
		}
	}
	return value, nil
}

func scanTask(row rowScanner) (CreativeTask, error) {
	var value CreativeTask
	var direction []byte
	err := row.Scan(&value.ID, &value.DisplayName, &value.OrganizationID, &value.ProjectID, &value.IntakeID, &value.Format, &value.Channel, &value.VideoPurpose, &value.PerformanceMode, &value.LineageKey, &value.Status, &direction, &value.Version, &value.CreatedAt, &value.UpdatedAt)
	if err != nil {
		return CreativeTask{}, err
	}
	if err := json.Unmarshal(direction, &value.Direction); err != nil {
		return CreativeTask{}, fmt.Errorf("decode creative task direction: %w", err)
	}
	return value, nil
}

func scanRenderJob(row rowScanner) (RenderJob, error) {
	var value RenderJob
	var outputAsset, errorCode, errorMessage sql.NullString
	var outputVersion sql.NullInt64
	err := row.Scan(
		&value.ID, &value.OrganizationID, &value.ProjectID, &value.TaskID, &value.Status,
		&value.PreRollVideo.AssetID, &value.PreRollVideo.Version, &value.MainVideo.AssetID, &value.MainVideo.Version,
		&outputAsset, &outputVersion, &errorCode, &errorMessage,
		&value.CreatedBy.Kind, &value.CreatedBy.ID, &value.IdempotencyKey, &value.RequestHash,
		&value.CreatedAt, &value.UpdatedAt,
	)
	if err != nil {
		return RenderJob{}, err
	}
	value.ErrorCode, value.ErrorMessage = errorCode.String, errorMessage.String
	if outputAsset.Valid && outputVersion.Valid {
		ref := contract.ProjectAssetRef{
			ProjectID:    value.ProjectID,
			AssetVersion: contract.AssetVersionRef{AssetID: contract.AssetID(outputAsset.String), Version: outputVersion.Int64},
		}
		value.OutputAsset = &ref
	}
	return value, nil
}

func scanCreativeVersion(row rowScanner) (CreativeVersion, error) {
	var value CreativeVersion
	var taskID, editTaskID sql.NullString
	var snapshot, videoSnapshot, check, approval []byte
	err := row.Scan(&value.ID, &value.OrganizationID, &value.ProjectID, &taskID, &editTaskID, &value.Version, &value.DraftVersion,
		&value.Status, &value.Format, &snapshot, &videoSnapshot, &value.ContentHash, &value.CreatedBy, &value.IdempotencyKey, &value.RequestHash, &value.CreatedAt, &check, &approval)
	if err != nil {
		return CreativeVersion{}, err
	}
	value.TaskID, value.EditTaskID = taskID.String, editTaskID.String
	if err := json.Unmarshal(snapshot, &value.Snapshot); err != nil {
		return CreativeVersion{}, fmt.Errorf("decode creative version snapshot: %w", err)
	}
	if len(videoSnapshot) > 0 {
		value.VideoSnapshot = &VideoVersionSnapshot{}
		if err := json.Unmarshal(videoSnapshot, value.VideoSnapshot); err != nil {
			return CreativeVersion{}, fmt.Errorf("decode creative video version snapshot: %w", err)
		}
	}
	if len(check) > 0 {
		value.Check = &CreativeCheck{}
		if err := json.Unmarshal(check, value.Check); err != nil {
			return CreativeVersion{}, fmt.Errorf("decode creative version check: %w", err)
		}
	}
	if len(approval) > 0 {
		value.Approval = &CreativeApproval{}
		if err := json.Unmarshal(approval, value.Approval); err != nil {
			return CreativeVersion{}, fmt.Errorf("decode creative version approval: %w", err)
		}
	}
	return value, nil
}

func (r MySQLRepository) getPackageByVersion(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, creativeVersionID string) (CreativePackage, error) {
	value, err := scanCreativePackage(r.DB.QueryRowContext(ctx, creativePackageSelect+` WHERE organization_id = ? AND project_id = ? AND creative_version_id = ?`, organizationID, projectID, creativeVersionID))
	if errors.Is(err, sql.ErrNoRows) {
		return CreativePackage{}, ErrNotFound
	}
	return value, err
}

func (r MySQLRepository) getIntakeByIdempotency(ctx context.Context, intake CreativeIntake) (CreativeIntake, error) {
	return scanIntake(r.DB.QueryRowContext(ctx, creativeIntakeSelect+` WHERE organization_id = ? AND project_id = ? AND principal_kind = ? AND principal_id = ? AND idempotency_key = ?`, intake.OrganizationID, intake.ProjectID, intake.Principal.Kind, intake.Principal.ID, intake.IdempotencyKey))
}

func (r MySQLRepository) getIntakeByInputIdentity(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, source IntakeSource, inputIdentityHash string) (CreativeIntake, error) {
	return scanIntake(r.DB.QueryRowContext(ctx, creativeIntakeSelect+` WHERE organization_id = ? AND project_id = ? AND source_type = ? AND input_identity_hash = ?`, organizationID, projectID, source, inputIdentityHash))
}

func (r MySQLRepository) getIntakeByStrategyPackage(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, request CreateIntakeRequest) (CreativeIntake, error) {
	reference := request.StrategyPackage
	return scanIntake(r.DB.QueryRowContext(ctx, creativeIntakeSelect+`
		WHERE organization_id = ? AND project_id = ? AND source_type = ?
		AND strategy_package_id = ? AND strategy_package_version = ?
		AND strategy_package_content_hash = ?
		AND selected_route_id <=> NULLIF(?, '')
		AND handoff_content_hash <=> NULLIF(?, '')
		AND task_overlay_identity = ?`,
		organizationID, projectID, IntakeSourceStrategyPackage,
		reference.PackageID, reference.PackageVersion, reference.ExpectedContentHash,
		request.SelectedRouteID, reference.ExpectedHandoffHash, func() string {
			if request.TaskOverlay == nil {
				return ""
			}
			return request.TaskOverlay.OverlayID
		}()))
}

func (r MySQLRepository) getIntakeByTaskStrategy(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, reference TaskStrategyReference) (CreativeIntake, error) {
	return scanIntake(r.DB.QueryRowContext(ctx, creativeIntakeSelect+` WHERE organization_id = ? AND project_id = ? AND source_type = ? AND task_strategy_plan_id = ? AND task_strategy_version = ? AND task_strategy_content_hash = ?`, organizationID, projectID, IntakeSourceTaskStrategy, reference.PlanID, reference.StrategyVersion, reference.ExpectedContentHash))
}

func (r MySQLRepository) getVersionByIdempotency(ctx context.Context, value CreativeVersion) (CreativeVersion, error) {
	version, err := scanCreativeVersion(r.DB.QueryRowContext(ctx, creativeVersionSelect+` WHERE organization_id = ? AND project_id = ? AND created_by = ? AND idempotency_key = ?`, value.OrganizationID, value.ProjectID, value.CreatedBy, value.IdempotencyKey))
	if errors.Is(err, sql.ErrNoRows) {
		return CreativeVersion{}, ErrNotFound
	}
	return version, err
}

func (r MySQLRepository) getVersionByTaskDraft(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, taskID string, draftVersion int64) (CreativeVersion, error) {
	version, err := scanCreativeVersion(r.DB.QueryRowContext(ctx, creativeVersionSelect+` WHERE organization_id = ? AND project_id = ? AND task_id = ? AND draft_version = ?`, organizationID, projectID, taskID, draftVersion))
	if errors.Is(err, sql.ErrNoRows) {
		return CreativeVersion{}, ErrNotFound
	}
	return version, err
}

func (r MySQLRepository) getVersionByEditTimeline(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, editTaskID string, timelineVersion int64) (CreativeVersion, error) {
	version, err := scanCreativeVersion(r.DB.QueryRowContext(ctx, creativeVersionSelect+` WHERE organization_id = ? AND project_id = ? AND edit_task_id = ? AND draft_version = ?`, organizationID, projectID, editTaskID, timelineVersion))
	if errors.Is(err, sql.ErrNoRows) {
		return CreativeVersion{}, ErrNotFound
	}
	return version, err
}

func (r MySQLRepository) getLatestDraft(ctx context.Context, organizationID contract.OrganizationID, taskID string) (ImageTextDraft, error) {
	var payload []byte
	var value ImageTextDraft
	err := r.DB.QueryRowContext(ctx, `SELECT content_payload FROM creative_image_text_drafts WHERE organization_id = ? AND task_id = ? ORDER BY version DESC LIMIT 1`, organizationID, taskID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return ImageTextDraft{}, ErrNotFound
	}
	if err != nil {
		return ImageTextDraft{}, err
	}
	if err := json.Unmarshal(payload, &value); err != nil {
		return ImageTextDraft{}, fmt.Errorf("decode creative draft: %w", err)
	}
	return value, nil
}

func (r MySQLRepository) getLatestVideoDraft(ctx context.Context, organizationID contract.OrganizationID, taskID string) (VideoDraft, error) {
	var payload []byte
	var value VideoDraft
	err := r.DB.QueryRowContext(ctx, `SELECT content_payload FROM creative_video_drafts WHERE organization_id = ? AND task_id = ? ORDER BY revision DESC LIMIT 1`, organizationID, taskID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return VideoDraft{}, ErrNotFound
	}
	if err != nil {
		return VideoDraft{}, err
	}
	if err := json.Unmarshal(payload, &value); err != nil {
		return VideoDraft{}, fmt.Errorf("decode creative video draft: %w", err)
	}
	return value, nil
}

func (r MySQLRepository) getTaskByIntake(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, intakeID string) (CreativeTask, error) {
	value, err := scanTask(r.DB.QueryRowContext(ctx, creativeTaskSelect+` WHERE organization_id = ? AND project_id = ? AND intake_id = ?`, organizationID, projectID, intakeID))
	if errors.Is(err, sql.ErrNoRows) {
		return CreativeTask{}, ErrNotFound
	}
	return value, err
}

func (r MySQLRepository) productionJobs(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, taskID string) ([]ProductionJob, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT task_id, job_kind, provider_job_id, created_at FROM creative_production_jobs WHERE organization_id = ? AND project_id = ? AND task_id = ? ORDER BY created_at`, organizationID, projectID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]ProductionJob, 0)
	for rows.Next() {
		var job ProductionJob
		if err := rows.Scan(&job.TaskID, &job.Kind, &job.ProviderJobID, &job.CreatedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (r MySQLRepository) shortDramaGenerationAttempts(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	taskID string,
) ([]ShortDramaGenerationAttempt, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT
		id, task_id, draft_revision, candidate_batch_id, candidate_id, prompt_package_hash,
		generation_spec_hash, provider_job_id, output_asset_id, output_asset_version, created_at
		FROM creative_short_drama_generation_attempts
		WHERE organization_id = ? AND project_id = ? AND task_id = ?
		ORDER BY created_at, id`, organizationID, projectID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	attempts := make([]ShortDramaGenerationAttempt, 0)
	for rows.Next() {
		attempt, scanErr := scanShortDramaGenerationAttempt(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

func (r MySQLRepository) shortDramaGenerationAttemptByProviderJob(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	providerJobID string,
) (ShortDramaGenerationAttempt, error) {
	return scanShortDramaGenerationAttempt(r.DB.QueryRowContext(ctx, `SELECT
		id, task_id, draft_revision, candidate_batch_id, candidate_id, prompt_package_hash,
		generation_spec_hash, provider_job_id, output_asset_id, output_asset_version, created_at
		FROM creative_short_drama_generation_attempts
		WHERE organization_id = ? AND project_id = ? AND provider_job_id = ?`,
		organizationID, projectID, providerJobID))
}

type shortDramaGenerationAttemptScanner interface {
	Scan(...any) error
}

func scanShortDramaGenerationAttempt(scanner shortDramaGenerationAttemptScanner) (ShortDramaGenerationAttempt, error) {
	var attempt ShortDramaGenerationAttempt
	var outputAssetID sql.NullString
	var outputAssetVersion sql.NullInt64
	if err := scanner.Scan(
		&attempt.ID, &attempt.TaskID, &attempt.DraftRevision, &attempt.CandidateBatchID,
		&attempt.CandidateID, &attempt.PromptPackageHash, &attempt.GenerationSpecHash,
		&attempt.ProviderJobID, &outputAssetID, &outputAssetVersion, &attempt.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ShortDramaGenerationAttempt{}, ErrNotFound
		}
		return ShortDramaGenerationAttempt{}, err
	}
	if outputAssetID.Valid && outputAssetVersion.Valid {
		ref := contract.AssetVersionRef{
			AssetID: contract.AssetID(outputAssetID.String),
			Version: outputAssetVersion.Int64,
		}
		attempt.OutputAssetVersion = &ref
	}
	return attempt, nil
}

func (r MySQLRepository) gamePrerollGenerationAttempts(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	taskID string,
) ([]GamePrerollGenerationAttempt, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT
		id, task_id, draft_revision, candidate_batch_id, candidate_id, prompt_package_hash,
		generation_spec_hash, provider_job_id, output_asset_id, output_asset_version, created_at
		FROM creative_game_preroll_generation_attempts
		WHERE organization_id = ? AND project_id = ? AND task_id = ?
		ORDER BY created_at, id`, organizationID, projectID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	attempts := make([]GamePrerollGenerationAttempt, 0)
	for rows.Next() {
		attempt, scanErr := scanGamePrerollGenerationAttempt(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

func (r MySQLRepository) gamePrerollGenerationAttemptByProviderJob(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	providerJobID string,
) (GamePrerollGenerationAttempt, error) {
	return scanGamePrerollGenerationAttempt(r.DB.QueryRowContext(ctx, `SELECT
		id, task_id, draft_revision, candidate_batch_id, candidate_id, prompt_package_hash,
		generation_spec_hash, provider_job_id, output_asset_id, output_asset_version, created_at
		FROM creative_game_preroll_generation_attempts
		WHERE organization_id = ? AND project_id = ? AND provider_job_id = ?`,
		organizationID, projectID, providerJobID))
}

type gamePrerollGenerationAttemptScanner interface {
	Scan(...any) error
}

func scanGamePrerollGenerationAttempt(scanner gamePrerollGenerationAttemptScanner) (GamePrerollGenerationAttempt, error) {
	var attempt GamePrerollGenerationAttempt
	var outputAssetID sql.NullString
	var outputAssetVersion sql.NullInt64
	if err := scanner.Scan(
		&attempt.ID, &attempt.TaskID, &attempt.DraftRevision, &attempt.CandidateBatchID,
		&attempt.CandidateID, &attempt.PromptPackageHash, &attempt.GenerationSpecHash,
		&attempt.ProviderJobID, &outputAssetID, &outputAssetVersion, &attempt.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GamePrerollGenerationAttempt{}, ErrNotFound
		}
		return GamePrerollGenerationAttempt{}, err
	}
	if outputAssetID.Valid && outputAssetVersion.Valid {
		ref := contract.AssetVersionRef{
			AssetID: contract.AssetID(outputAssetID.String),
			Version: outputAssetVersion.Int64,
		}
		attempt.OutputAssetVersion = &ref
	}
	return attempt, nil
}
