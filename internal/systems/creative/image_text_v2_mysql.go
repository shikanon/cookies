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

func (r MySQLRepository) SaveImageTextDraft(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	taskID string,
	expectedTaskVersion int64,
	expectedDraftRevision int64,
	draft ImageTextDraft,
	targetStatus TaskStatus,
	now time.Time,
) (CreativeTask, ImageTextDraft, error) {
	if r.DB == nil {
		return CreativeTask{}, ImageTextDraft{}, fmt.Errorf("creative MySQL database is required")
	}
	if draft.ContractVersion != ImageTextDraftV2Contract || draft.TaskID != taskID ||
		draft.Version != expectedDraftRevision+1 || !validCreativeTaskStatus(targetStatus) {
		return CreativeTask{}, ImageTextDraft{}, fmt.Errorf("image-text v2 draft revision is invalid")
	}
	payload, err := json.Marshal(draft)
	if err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	defer tx.Rollback()
	var currentVersion int64
	var currentStatus TaskStatus
	var format CreativeFormat
	if err := tx.QueryRowContext(ctx, `SELECT version, status, creative_format FROM creative_tasks
		WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`,
		organizationID, projectID, taskID).Scan(&currentVersion, &currentStatus, &format); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CreativeTask{}, ImageTextDraft{}, ErrNotFound
		}
		return CreativeTask{}, ImageTextDraft{}, err
	}
	if currentVersion != expectedTaskVersion || format != FormatImageText ||
		!CanTransitionCreativeTaskStatus(currentStatus, targetStatus) {
		return CreativeTask{}, ImageTextDraft{}, ErrVersionConflict
	}
	var latestDraft int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM creative_image_text_drafts
		WHERE organization_id=? AND task_id=?`, organizationID, taskID).Scan(&latestDraft); err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	if latestDraft != expectedDraftRevision {
		return CreativeTask{}, ImageTextDraft{}, ErrVersionConflict
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO creative_image_text_drafts
		(organization_id, task_id, version, status, content_payload, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		organizationID, taskID, draft.Version, draft.Status, payload, draft.CreatedAt); err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE creative_tasks
		SET version=version+1, status=?, updated_at=?
		WHERE organization_id=? AND project_id=? AND id=?`,
		targetStatus, now, organizationID, projectID, taskID); err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	task, err := scanTask(tx.QueryRowContext(ctx, creativeTaskSelect+`
		WHERE organization_id=? AND project_id=? AND id=?`, organizationID, projectID, taskID))
	if err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	if err := tx.Commit(); err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	return task, draft, nil
}

func (r MySQLRepository) CreateImagePromptPackage(ctx context.Context, value ImagePromptPackage) (ImagePromptPackage, bool, error) {
	if r.DB == nil {
		return ImagePromptPackage{}, false, fmt.Errorf("creative MySQL database is required")
	}
	if err := value.Validate(); err != nil {
		return ImagePromptPackage{}, false, err
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return ImagePromptPackage{}, false, err
	}
	_, err = r.DB.ExecContext(ctx, `INSERT INTO creative_image_prompt_packages
		(organization_id, project_id, id, task_id, draft_revision, image_plan_order,
		 direction_id, direction_content_hash, input_identity_hash, compiler_version,
		 prompt_payload, content_hash, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.OrganizationID, value.ProjectID, value.ID, value.TaskID, value.DraftRevision,
		value.ImagePlanOrder, value.DirectionID, value.DirectionContentHash,
		value.InputIdentityHash, value.CompilerVersion, payload, value.ContentHash,
		value.CreatedBy, value.CreatedAt)
	if err == nil {
		return value, false, nil
	}
	var mysqlError *mysqlDriver.MySQLError
	if !errors.As(err, &mysqlError) || mysqlError.Number != 1062 {
		return ImagePromptPackage{}, false, err
	}
	existing, getErr := r.getImagePromptPackageByContent(
		ctx, value.OrganizationID, value.ProjectID, value.TaskID, value.DraftRevision,
		value.ImagePlanOrder, value.ContentHash,
	)
	if getErr != nil {
		return ImagePromptPackage{}, false, getErr
	}
	return existing, true, nil
}

func (r MySQLRepository) CreateImageGenerationAttempt(ctx context.Context, value ImageGenerationAttempt) (ImageGenerationAttempt, bool, error) {
	if r.DB == nil {
		return ImageGenerationAttempt{}, false, fmt.Errorf("creative MySQL database is required")
	}
	specPayload, err := json.Marshal(value.GenerationSpec)
	if err != nil {
		return ImageGenerationAttempt{}, false, err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return ImageGenerationAttempt{}, false, err
	}
	defer tx.Rollback()
	var taskVersion int64
	var taskStatus TaskStatus
	if err := tx.QueryRowContext(ctx, `SELECT version, status FROM creative_tasks
		WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`,
		value.OrganizationID, value.ProjectID, value.TaskID).Scan(&taskVersion, &taskStatus); err != nil {
		return ImageGenerationAttempt{}, false, err
	}
	existing, existingErr := scanImageGenerationAttempt(tx.QueryRowContext(ctx, imageAttemptSelect+`
		WHERE organization_id=? AND project_id=? AND idempotency_key=? FOR UPDATE`,
		value.OrganizationID, value.ProjectID, value.IdempotencyKey))
	switch {
	case existingErr == nil:
		if existing.RequestHash != value.RequestHash {
			return ImageGenerationAttempt{}, false, ErrIdempotencyConflict
		}
		return existing, true, nil
	case !errors.Is(existingErr, sql.ErrNoRows):
		return ImageGenerationAttempt{}, false, existingErr
	}
	if taskVersion != value.ExpectedTaskVersion ||
		!oneOfTaskStatus(taskStatus, TaskDraft, TaskInProgress, TaskGenerated) {
		return ImageGenerationAttempt{}, false, ErrVersionConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO creative_image_generation_attempts
		(organization_id, project_id, id, task_id, draft_revision, image_plan_order,
		 attempt_no, prompt_package_id, generation_spec_payload, generation_spec_hash,
		 provider_job_id, render_job_id, status, base_asset_id, base_asset_version,
		 final_asset_id, final_asset_version, reused_from_attempt_id, stale_reason,
		 error_code, error_message, idempotency_key, request_hash, created_by_kind, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?, NULL, NULL, NULL, NULL, NULL, '', '', '', ?, ?, ?, ?, ?, ?)`,
		value.OrganizationID, value.ProjectID, value.ID, value.TaskID, value.DraftRevision,
		value.ImagePlanOrder, value.AttemptNo, value.PromptPackageID, specPayload,
		value.GenerationSpecHash, value.Status, value.IdempotencyKey, value.RequestHash,
		value.CreatedByKind, value.CreatedBy, value.CreatedAt, value.UpdatedAt)
	if err != nil {
		var mysqlError *mysqlDriver.MySQLError
		if !errors.As(err, &mysqlError) || mysqlError.Number != 1062 {
			return ImageGenerationAttempt{}, false, err
		}
		existing, getErr := r.getImageAttemptByIdempotency(
			ctx, value.OrganizationID, value.ProjectID, value.IdempotencyKey,
		)
		if getErr != nil {
			return ImageGenerationAttempt{}, false, getErr
		}
		if existing.RequestHash != value.RequestHash {
			return ImageGenerationAttempt{}, false, ErrIdempotencyConflict
		}
		return existing, true, nil
	}
	target := TaskGenerating
	if taskStatus == TaskGenerated {
		target = TaskInProgress
	}
	if !CanTransitionCreativeTaskStatus(taskStatus, target) {
		return ImageGenerationAttempt{}, false, ErrInvalidState
	}
	if _, err := tx.ExecContext(ctx, `UPDATE creative_tasks SET status=?, version=version+1, updated_at=?
		WHERE organization_id=? AND project_id=? AND id=?`,
		target, value.UpdatedAt, value.OrganizationID, value.ProjectID, value.TaskID); err != nil {
		return ImageGenerationAttempt{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return ImageGenerationAttempt{}, false, err
	}
	return value, false, nil
}

func (r MySQLRepository) AttachImageProviderJob(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	attemptID string,
	providerJobID string,
	now time.Time,
) (ImageGenerationAttempt, error) {
	result, err := r.DB.ExecContext(ctx, `UPDATE creative_image_generation_attempts
		SET provider_job_id=?, status=?, updated_at=?
		WHERE organization_id=? AND project_id=? AND id=? AND status=? AND provider_job_id IS NULL`,
		providerJobID, ImageAttemptRunning, now, organizationID, projectID, attemptID, ImageAttemptQueued)
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	if affected != 1 {
		existing, getErr := r.GetImageGenerationAttempt(ctx, organizationID, projectID, attemptID)
		if getErr != nil {
			return ImageGenerationAttempt{}, getErr
		}
		if existing.ProviderJobID != providerJobID {
			return ImageGenerationAttempt{}, ErrProviderJobConflict
		}
		return existing, nil
	}
	return r.GetImageGenerationAttempt(ctx, organizationID, projectID, attemptID)
}

func (r MySQLRepository) ListImageGenerationAttempts(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	taskID string,
	draftRevision int64,
) ([]ImageGenerationAttempt, error) {
	rows, err := r.DB.QueryContext(ctx, imageAttemptSelect+`
		WHERE organization_id=? AND project_id=? AND task_id=? AND draft_revision=?
		ORDER BY image_plan_order, attempt_no`,
		organizationID, projectID, taskID, draftRevision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ImageGenerationAttempt, 0)
	for rows.Next() {
		value, scanErr := scanImageGenerationAttempt(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (r MySQLRepository) ListActiveImageGenerationAttempts(
	ctx context.Context,
	limit int,
) ([]ImageGenerationAttempt, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.DB.QueryContext(ctx, imageAttemptSelect+`
		WHERE status IN (?, ?, ?, ?)
		   OR (
		     status=?
		     AND (
		       NOT EXISTS (
		         SELECT 1 FROM creative_image_slot_selections slot_selection
		         WHERE slot_selection.organization_id=creative_image_generation_attempts.organization_id
		           AND slot_selection.task_id=creative_image_generation_attempts.task_id
		           AND slot_selection.draft_revision=creative_image_generation_attempts.draft_revision
		           AND slot_selection.image_plan_order=creative_image_generation_attempts.image_plan_order
		       )
		       OR (
		         (SELECT COUNT(*) FROM creative_image_slot_selections selection
		          WHERE selection.organization_id=creative_image_generation_attempts.organization_id
		            AND selection.task_id=creative_image_generation_attempts.task_id
		            AND selection.draft_revision=creative_image_generation_attempts.draft_revision)=3
		         AND NOT EXISTS (
		           SELECT 1 FROM creative_image_text_drafts draft
		           WHERE draft.organization_id=creative_image_generation_attempts.organization_id
		             AND draft.task_id=creative_image_generation_attempts.task_id
		             AND JSON_UNQUOTE(JSON_EXTRACT(draft.content_payload,'$.generation_source_version'))=
		                 CAST(creative_image_generation_attempts.draft_revision AS CHAR)
		         )
		       )
		     )
		   )
		ORDER BY updated_at
		LIMIT ?`,
		ImageAttemptQueued, ImageAttemptRunning, ImageAttemptBaseAssetReady, ImageAttemptRendering,
		ImageAttemptSucceeded, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ImageGenerationAttempt, 0)
	for rows.Next() {
		value, scanErr := scanImageGenerationAttempt(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (r MySQLRepository) GetImageGenerationAttempt(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	attemptID string,
) (ImageGenerationAttempt, error) {
	value, err := scanImageGenerationAttempt(r.DB.QueryRowContext(ctx, imageAttemptSelect+`
		WHERE organization_id=? AND project_id=? AND id=?`,
		organizationID, projectID, attemptID))
	if errors.Is(err, sql.ErrNoRows) {
		return ImageGenerationAttempt{}, ErrNotFound
	}
	return value, err
}

func (r MySQLRepository) FindImageGenerationAttemptByProviderJob(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	providerJobID string,
) (ImageGenerationAttempt, error) {
	return scanImageGenerationAttempt(r.DB.QueryRowContext(ctx, imageAttemptSelect+`
		WHERE organization_id=? AND project_id=? AND provider_job_id=?`,
		organizationID, projectID, providerJobID))
}

func (r MySQLRepository) MarkImageAttemptBaseReady(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	attemptID string,
	ref contract.AssetVersionRef,
	now time.Time,
) (ImageGenerationAttempt, error) {
	if err := ref.Validate(); err != nil {
		return ImageGenerationAttempt{}, err
	}
	return r.updateImageAttemptAsset(ctx, organizationID, projectID, attemptID, ref, true, now)
}

func (r MySQLRepository) MarkImageAttemptFinalReady(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	attemptID string,
	renderJobID string,
	ref contract.AssetVersionRef,
	now time.Time,
) (ImageGenerationAttempt, error) {
	if err := ref.Validate(); err != nil {
		return ImageGenerationAttempt{}, err
	}
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	defer tx.Rollback()
	taskID, err := lockImageAttemptTask(ctx, tx, organizationID, projectID, attemptID)
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE creative_image_generation_attempts
		SET render_job_id=?, final_asset_id=?, final_asset_version=?, status=?, updated_at=?
		WHERE organization_id=? AND project_id=? AND id=? AND status IN (?, ?)`,
		renderJobID, ref.AssetID, ref.Version, ImageAttemptSucceeded, now,
		organizationID, projectID, attemptID, ImageAttemptBaseAssetReady, ImageAttemptRendering)
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	if affected != 1 {
		existing, getErr := scanImageGenerationAttempt(tx.QueryRowContext(ctx, imageAttemptSelect+`
			WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`,
			organizationID, projectID, attemptID))
		if getErr != nil {
			return ImageGenerationAttempt{}, getErr
		}
		if existing.Status != ImageAttemptSucceeded || existing.FinalAssetRef == nil || *existing.FinalAssetRef != ref {
			return ImageGenerationAttempt{}, ErrInvalidState
		}
		if err := tx.Commit(); err != nil {
			return ImageGenerationAttempt{}, err
		}
		return existing, nil
	}
	if err := r.settleImageAttemptTask(ctx, tx, organizationID, projectID, taskID, true, now); err != nil {
		return ImageGenerationAttempt{}, err
	}
	value, err := scanImageGenerationAttempt(tx.QueryRowContext(ctx, imageAttemptSelect+`
		WHERE organization_id=? AND project_id=? AND id=?`,
		organizationID, projectID, attemptID))
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	if err := tx.Commit(); err != nil {
		return ImageGenerationAttempt{}, err
	}
	return value, nil
}

func (r MySQLRepository) MarkImageAttemptFailed(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	attemptID string,
	code string,
	message string,
	now time.Time,
) (ImageGenerationAttempt, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	defer tx.Rollback()
	taskID, err := lockImageAttemptTask(ctx, tx, organizationID, projectID, attemptID)
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE creative_image_generation_attempts
		SET status=?, error_code=?, error_message=?, updated_at=?
		WHERE organization_id=? AND project_id=? AND id=?
		  AND status IN (?, ?, ?, ?)`,
		ImageAttemptFailed, code, message, now, organizationID, projectID, attemptID,
		ImageAttemptQueued, ImageAttemptRunning, ImageAttemptBaseAssetReady, ImageAttemptRendering)
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	if affected == 1 {
		if err := r.settleImageAttemptTask(ctx, tx, organizationID, projectID, taskID, false, now); err != nil {
			return ImageGenerationAttempt{}, err
		}
	}
	value, err := scanImageGenerationAttempt(tx.QueryRowContext(ctx, imageAttemptSelect+`
		WHERE organization_id=? AND project_id=? AND id=?`,
		organizationID, projectID, attemptID))
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	if err := tx.Commit(); err != nil {
		return ImageGenerationAttempt{}, err
	}
	return value, nil
}

func (r MySQLRepository) MarkImageAttemptStale(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	attemptID string,
	reason string,
	now time.Time,
) (ImageGenerationAttempt, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	defer tx.Rollback()
	taskID, err := lockImageAttemptTask(ctx, tx, organizationID, projectID, attemptID)
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE creative_image_generation_attempts
		SET status=?, stale_reason=?, updated_at=?
		WHERE organization_id=? AND project_id=? AND id=?
		  AND status IN (?, ?, ?, ?)`,
		ImageAttemptStale, reason, now, organizationID, projectID, attemptID,
		ImageAttemptQueued, ImageAttemptRunning, ImageAttemptBaseAssetReady, ImageAttemptRendering)
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	if affected == 1 {
		if err := r.settleImageAttemptTask(ctx, tx, organizationID, projectID, taskID, false, now); err != nil {
			return ImageGenerationAttempt{}, err
		}
	}
	value, err := scanImageGenerationAttempt(tx.QueryRowContext(ctx, imageAttemptSelect+`
		WHERE organization_id=? AND project_id=? AND id=?`,
		organizationID, projectID, attemptID))
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	if err := tx.Commit(); err != nil {
		return ImageGenerationAttempt{}, err
	}
	return value, nil
}

func (r MySQLRepository) settleImageAttemptTask(
	ctx context.Context,
	tx *sql.Tx,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	taskID string,
	succeeded bool,
	now time.Time,
) error {
	var activeCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM creative_image_generation_attempts
		WHERE organization_id=? AND project_id=? AND task_id=? AND status IN (?, ?, ?, ?)`,
		organizationID, projectID, taskID,
		ImageAttemptQueued, ImageAttemptRunning, ImageAttemptBaseAssetReady, ImageAttemptRendering,
	).Scan(&activeCount); err != nil {
		return err
	}
	if activeCount != 0 {
		return nil
	}
	target := TaskInProgress
	if succeeded {
		target = TaskGenerated
	}
	_, err := tx.ExecContext(ctx, `UPDATE creative_tasks SET status=?, version=version+1, updated_at=?
		WHERE organization_id=? AND project_id=? AND id=? AND status=?`,
		target, now, organizationID, projectID, taskID, TaskGenerating)
	return err
}

func lockImageAttemptTask(
	ctx context.Context,
	tx *sql.Tx,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	attemptID string,
) (string, error) {
	var taskID string
	if err := tx.QueryRowContext(ctx, `SELECT task_id FROM creative_image_generation_attempts
		WHERE organization_id=? AND project_id=? AND id=?`,
		organizationID, projectID, attemptID).Scan(&taskID); err != nil {
		return "", err
	}
	var lockedTaskID string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM creative_tasks
		WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`,
		organizationID, projectID, taskID).Scan(&lockedTaskID); err != nil {
		return "", err
	}
	return taskID, nil
}

func (r MySQLRepository) AdoptImageGenerationAttempt(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	taskID string,
	expectedTaskVersion int64,
	order int64,
	attemptID string,
	expectedSelectionVersion int64,
	adoptedBy string,
	now time.Time,
) (ImageSlotSelection, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return ImageSlotSelection{}, err
	}
	defer tx.Rollback()
	var taskVersion int64
	if err := tx.QueryRowContext(ctx, `SELECT version FROM creative_tasks
		WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`,
		organizationID, projectID, taskID).Scan(&taskVersion); err != nil {
		return ImageSlotSelection{}, err
	}
	if taskVersion != expectedTaskVersion {
		return ImageSlotSelection{}, ErrVersionConflict
	}
	var revision int64
	var attemptOrder int
	var status ImageGenerationAttemptStatus
	if err := tx.QueryRowContext(ctx, `SELECT draft_revision, image_plan_order, status
		FROM creative_image_generation_attempts
		WHERE organization_id=? AND project_id=? AND task_id=? AND id=?`,
		organizationID, projectID, taskID, attemptID).Scan(&revision, &attemptOrder, &status); err != nil {
		return ImageSlotSelection{}, err
	}
	if int64(attemptOrder) != order || status != ImageAttemptSucceeded {
		return ImageSlotSelection{}, ErrInvalidState
	}
	var currentVersion int64
	err = tx.QueryRowContext(ctx, `SELECT version FROM creative_image_slot_selections
		WHERE organization_id=? AND task_id=? AND draft_revision=? AND image_plan_order=? FOR UPDATE`,
		organizationID, taskID, revision, order).Scan(&currentVersion)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if expectedSelectionVersion != 0 {
			return ImageSlotSelection{}, ErrVersionConflict
		}
		currentVersion = 0
	case err != nil:
		return ImageSlotSelection{}, err
	case currentVersion != expectedSelectionVersion:
		return ImageSlotSelection{}, ErrVersionConflict
	}
	nextVersion := currentVersion + 1
	if _, err := tx.ExecContext(ctx, `INSERT INTO creative_image_slot_selections
		(organization_id, project_id, task_id, draft_revision, image_plan_order,
		 adopted_attempt_id, version, adopted_by, adopted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE adopted_attempt_id=VALUES(adopted_attempt_id),
		 version=VALUES(version), adopted_by=VALUES(adopted_by), adopted_at=VALUES(adopted_at)`,
		organizationID, projectID, taskID, revision, order, attemptID,
		nextVersion, adoptedBy, now); err != nil {
		return ImageSlotSelection{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE creative_tasks SET version=version+1, updated_at=?
		WHERE organization_id=? AND project_id=? AND id=?`,
		now, organizationID, projectID, taskID); err != nil {
		return ImageSlotSelection{}, err
	}
	if err := tx.Commit(); err != nil {
		return ImageSlotSelection{}, err
	}
	return ImageSlotSelection{
		ContractVersion: ImageSlotSelectionV1Contract, OrganizationID: organizationID,
		ProjectID: projectID, TaskID: taskID, DraftRevision: revision,
		ImagePlanOrder: int(order), AdoptedAttemptID: attemptID, Version: nextVersion,
		AdoptedBy: adoptedBy, AdoptedAt: now,
	}, nil
}

func (r MySQLRepository) ListImageSlotSelections(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	taskID string,
	draftRevision int64,
) ([]ImageSlotSelection, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT organization_id, project_id, task_id,
		draft_revision, image_plan_order, adopted_attempt_id, version, adopted_by, adopted_at
		FROM creative_image_slot_selections
		WHERE organization_id=? AND project_id=? AND task_id=? AND draft_revision=?
		ORDER BY image_plan_order`, organizationID, projectID, taskID, draftRevision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ImageSlotSelection{}
	for rows.Next() {
		value := ImageSlotSelection{ContractVersion: ImageSlotSelectionV1Contract}
		if err := rows.Scan(
			&value.OrganizationID, &value.ProjectID, &value.TaskID, &value.DraftRevision,
			&value.ImagePlanOrder, &value.AdoptedAttemptID, &value.Version,
			&value.AdoptedBy, &value.AdoptedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (r MySQLRepository) FinalizeImageTextDraftAssets(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	taskID string,
	expectedTaskVersion int64,
	authoringRevision int64,
	createdBy string,
	now time.Time,
) (CreativeTask, ImageTextDraft, error) {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	defer tx.Rollback()
	var taskVersion int64
	var taskStatus TaskStatus
	if err := tx.QueryRowContext(ctx, `SELECT version, status FROM creative_tasks
		WHERE organization_id=? AND project_id=? AND id=? FOR UPDATE`,
		organizationID, projectID, taskID).Scan(&taskVersion, &taskStatus); err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	var latestPayload []byte
	if err := tx.QueryRowContext(ctx, `SELECT content_payload FROM creative_image_text_drafts
		WHERE organization_id=? AND task_id=? ORDER BY version DESC LIMIT 1`,
		organizationID, taskID).Scan(&latestPayload); err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	var latestDraft ImageTextDraft
	if err := json.Unmarshal(latestPayload, &latestDraft); err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	if latestDraft.GenerationSourceVersion != nil &&
		*latestDraft.GenerationSourceVersion == authoringRevision {
		task, scanErr := scanTask(tx.QueryRowContext(ctx, creativeTaskSelect+`
			WHERE organization_id=? AND project_id=? AND id=?`,
			organizationID, projectID, taskID))
		if scanErr != nil {
			return CreativeTask{}, ImageTextDraft{}, scanErr
		}
		if err := tx.Commit(); err != nil {
			return CreativeTask{}, ImageTextDraft{}, err
		}
		return task, latestDraft, nil
	}
	if taskVersion != expectedTaskVersion ||
		!oneOfTaskStatus(taskStatus, TaskGenerating, TaskGenerated, TaskRendering, TaskInProgress) {
		return CreativeTask{}, ImageTextDraft{}, ErrVersionConflict
	}
	var payload []byte
	if err := tx.QueryRowContext(ctx, `SELECT content_payload FROM creative_image_text_drafts
		WHERE organization_id=? AND task_id=? AND version=? FOR UPDATE`,
		organizationID, taskID, authoringRevision).Scan(&payload); err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	var draft ImageTextDraft
	if err := json.Unmarshal(payload, &draft); err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	if draft.ContractVersion != ImageTextDraftV2Contract || len(draft.ImagePlan) != 3 {
		return CreativeTask{}, ImageTextDraft{}, ErrInvalidState
	}
	rows, err := tx.QueryContext(ctx, `SELECT s.image_plan_order, a.final_asset_id, a.final_asset_version
		FROM creative_image_slot_selections s
		JOIN creative_image_generation_attempts a
		  ON a.organization_id=s.organization_id AND a.id=s.adopted_attempt_id
		WHERE s.organization_id=? AND s.project_id=? AND s.task_id=? AND s.draft_revision=?
		  AND a.status=? ORDER BY s.image_plan_order FOR UPDATE`,
		organizationID, projectID, taskID, authoringRevision, ImageAttemptSucceeded)
	if err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	defer rows.Close()
	refs := map[int]contract.AssetVersionRef{}
	for rows.Next() {
		var order int
		var assetID contract.AssetID
		var version int64
		if err := rows.Scan(&order, &assetID, &version); err != nil {
			return CreativeTask{}, ImageTextDraft{}, err
		}
		refs[order] = contract.AssetVersionRef{AssetID: assetID, Version: version}
	}
	if err := rows.Err(); err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	if err := rows.Close(); err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	if len(refs) != 3 {
		return CreativeTask{}, ImageTextDraft{}, ErrInvalidState
	}
	source := authoringRevision
	draft.Version = authoringRevision + 1
	draft.GenerationSourceVersion = &source
	draft.Status = "ready_for_review"
	draft.CreatedAt = now
	for index := range draft.ImagePlan {
		ref, ok := refs[index+1]
		if !ok || ref.Validate() != nil {
			return CreativeTask{}, ImageTextDraft{}, ErrInvalidState
		}
		draft.ImagePlan[index].AssetRef = &ref
	}
	materialized, err := json.Marshal(draft)
	if err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO creative_image_text_drafts
		(organization_id, task_id, version, status, content_payload, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		organizationID, taskID, draft.Version, draft.Status, materialized, now); err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	currentStatus := taskStatus
	for _, next := range []TaskStatus{TaskGenerated, TaskRendering, TaskReady} {
		if currentStatus == next {
			continue
		}
		if !CanTransitionCreativeTaskStatus(currentStatus, next) {
			if next == TaskGenerated || next == TaskRendering {
				continue
			}
			return CreativeTask{}, ImageTextDraft{}, ErrInvalidState
		}
		if _, err := tx.ExecContext(ctx, `UPDATE creative_tasks SET status=?, version=version+1, updated_at=?
			WHERE organization_id=? AND project_id=? AND id=?`,
			next, now, organizationID, projectID, taskID); err != nil {
			return CreativeTask{}, ImageTextDraft{}, err
		}
		currentStatus = next
	}
	eventPayload, err := json.Marshal(map[string]any{
		"contract_version":          ImageTextDraftV2Contract,
		"task_id":                   taskID,
		"authoring_revision":        authoringRevision,
		"materialized_revision":     draft.Version,
		"generation_source_version": source,
		"created_by":                createdBy,
	})
	if err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	eventDigest, err := contract.CanonicalJSONHash(map[string]any{
		"organization_id":    organizationID,
		"project_id":         projectID,
		"task_id":            taskID,
		"authoring_revision": authoringRevision,
	})
	if err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO event_outbox
		(event_id, organization_id, project_id, event_type, subject_type, subject_id,
		 subject_version, payload, available_at, created_at)
		VALUES (?, ?, ?, 'creative.image_text.materialized.v1',
		 'creative_image_text_draft', ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE event_id=event_id`,
		"creative-image-text-"+eventDigest, organizationID, projectID, taskID,
		draft.Version, eventPayload, now, now); err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	task, err := scanTask(tx.QueryRowContext(ctx, creativeTaskSelect+`
		WHERE organization_id=? AND project_id=? AND id=?`, organizationID, projectID, taskID))
	if err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	if err := tx.Commit(); err != nil {
		return CreativeTask{}, ImageTextDraft{}, err
	}
	return task, draft, nil
}

func (r MySQLRepository) updateImageAttemptAsset(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	attemptID string,
	ref contract.AssetVersionRef,
	base bool,
	now time.Time,
) (ImageGenerationAttempt, error) {
	if !base {
		return ImageGenerationAttempt{}, fmt.Errorf("unsupported image attempt asset update")
	}
	result, err := r.DB.ExecContext(ctx, `UPDATE creative_image_generation_attempts
		SET base_asset_id=?, base_asset_version=?, status=?, updated_at=?
		WHERE organization_id=? AND project_id=? AND id=? AND status IN (?, ?)`,
		ref.AssetID, ref.Version, ImageAttemptBaseAssetReady, now,
		organizationID, projectID, attemptID, ImageAttemptQueued, ImageAttemptRunning)
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	if affected != 1 {
		existing, getErr := r.GetImageGenerationAttempt(ctx, organizationID, projectID, attemptID)
		if getErr != nil {
			return ImageGenerationAttempt{}, getErr
		}
		if existing.BaseAssetRef == nil || *existing.BaseAssetRef != ref {
			return ImageGenerationAttempt{}, ErrInvalidState
		}
		return existing, nil
	}
	return r.GetImageGenerationAttempt(ctx, organizationID, projectID, attemptID)
}

func (r MySQLRepository) getImagePromptPackageByContent(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	taskID string,
	draftRevision int64,
	order int,
	contentHash string,
) (ImagePromptPackage, error) {
	var payload []byte
	err := r.DB.QueryRowContext(ctx, `SELECT prompt_payload FROM creative_image_prompt_packages
		WHERE organization_id=? AND project_id=? AND task_id=? AND draft_revision=?
		  AND image_plan_order=? AND content_hash=?`,
		organizationID, projectID, taskID, draftRevision, order, contentHash).Scan(&payload)
	if err != nil {
		return ImagePromptPackage{}, err
	}
	var value ImagePromptPackage
	if err := json.Unmarshal(payload, &value); err != nil {
		return ImagePromptPackage{}, err
	}
	return value, nil
}

func (r MySQLRepository) getImageAttemptByIdempotency(
	ctx context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	key contract.IdempotencyKey,
) (ImageGenerationAttempt, error) {
	return scanImageGenerationAttempt(r.DB.QueryRowContext(ctx, imageAttemptSelect+`
		WHERE organization_id=? AND project_id=? AND idempotency_key=?`,
		organizationID, projectID, key))
}

const imageAttemptSelect = `SELECT organization_id, project_id, id, task_id,
	draft_revision, image_plan_order, attempt_no, prompt_package_id,
	generation_spec_payload, generation_spec_hash, COALESCE(provider_job_id,''),
	COALESCE(render_job_id,''), status, base_asset_id, base_asset_version,
	final_asset_id, final_asset_version, COALESCE(reused_from_attempt_id,''),
	stale_reason, error_code, error_message, idempotency_key, request_hash,
	created_by_kind, created_by, created_at, updated_at
	FROM creative_image_generation_attempts`

func scanImageGenerationAttempt(row rowScanner) (ImageGenerationAttempt, error) {
	var value ImageGenerationAttempt
	var spec []byte
	var baseID, finalID sql.NullString
	var baseVersion, finalVersion sql.NullInt64
	err := row.Scan(
		&value.OrganizationID, &value.ProjectID, &value.ID, &value.TaskID,
		&value.DraftRevision, &value.ImagePlanOrder, &value.AttemptNo,
		&value.PromptPackageID, &spec, &value.GenerationSpecHash,
		&value.ProviderJobID, &value.RenderJobID, &value.Status,
		&baseID, &baseVersion, &finalID, &finalVersion, &value.ReusedFromAttemptID,
		&value.StaleReason, &value.ErrorCode, &value.ErrorMessage,
		&value.IdempotencyKey, &value.RequestHash, &value.CreatedByKind, &value.CreatedBy,
		&value.CreatedAt, &value.UpdatedAt,
	)
	if err != nil {
		return ImageGenerationAttempt{}, err
	}
	value.ContractVersion = ImageGenerationAttemptV1
	if err := json.Unmarshal(spec, &value.GenerationSpec); err != nil {
		return ImageGenerationAttempt{}, err
	}
	if baseID.Valid && baseVersion.Valid {
		ref := contract.AssetVersionRef{AssetID: contract.AssetID(baseID.String), Version: baseVersion.Int64}
		value.BaseAssetRef = &ref
	}
	if finalID.Valid && finalVersion.Valid {
		ref := contract.AssetVersionRef{AssetID: contract.AssetID(finalID.String), Version: finalVersion.Int64}
		value.FinalAssetRef = &ref
	}
	return value, nil
}
