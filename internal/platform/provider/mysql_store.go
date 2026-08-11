package provider

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	mysqlDriver "github.com/go-sql-driver/mysql"

	"github.com/shikanon/cookies/internal/platform/contract"
)

var ErrIdempotencyConflict = errors.New("provider idempotency key was reused with a different request")

// MySQLStore persists only Provider-owned data. Assets records and object
// storage details never appear in this repository.
type MySQLStore struct {
	DB                *sql.DB
	AllowInsecureHTTP bool
}

func (s MySQLStore) Create(ctx context.Context, record JobRecord) (JobRecord, bool, error) {
	if s.DB == nil {
		return JobRecord{}, false, fmt.Errorf("MySQL database is required")
	}
	if err := validateRecord(record, s.AllowInsecureHTTP); err != nil {
		return JobRecord{}, false, err
	}
	payload, err := marshalProviderInput(record)
	if err != nil {
		return JobRecord{}, false, fmt.Errorf("encode provider input: %w", err)
	}
	job := record.Job
	routeSnapshot, err := marshalRouteSnapshot(record.Route, record.Operation, s.AllowInsecureHTTP)
	if err != nil {
		return JobRecord{}, false, err
	}
	submissionState := record.SubmissionState
	if submissionState == "" {
		submissionState = SubmissionNotStarted
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO provider_jobs (
		id, organization_id, project_id, principal_kind, principal_id, operation_name,
		idempotency_key, request_hash, kind, model_alias, route_snapshot, source_system, source_task_id,
		project_context_version, execution_status, provider_status, progress, provider_code, model_version, external_task_id, input_payload,
		submission_state, adapter_request_id, actual_provider, actual_model, execution_deadline_at, submitted_at, response_received_at,
		attempt_count, max_attempts, version, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?,
		?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.OrganizationID, job.ProjectID, record.Principal.Kind, record.Principal.ID, record.Operation,
		record.IdempotencyKey, record.RequestHash, job.Kind, record.ModelAlias, routeSnapshot, record.SourceSystem, record.SourceTaskID,
		record.ProjectContextVersion, job.ExecutionStatus, job.ProviderStatus, job.Progress,
		record.ProviderCode, record.ModelVersion, record.ExternalTaskID, payload,
		submissionState, record.AdapterRequestID, record.ActualProvider, record.ActualModel,
		record.ExecutionDeadlineAt, record.SubmittedAt, record.ResponseReceivedAt,
		job.AttemptCount, job.MaxAttempts, job.Version, job.CreatedAt, job.UpdatedAt)
	if err == nil {
		if observabilityErr := s.ensureInitialJobObservability(ctx, record); observabilityErr != nil {
			return JobRecord{}, false, observabilityErr
		}
		return record, false, nil
	}
	var mysqlError *mysqlDriver.MySQLError
	if !errors.As(err, &mysqlError) || mysqlError.Number != 1062 {
		return JobRecord{}, false, err
	}
	existing, getErr := s.getByIdempotency(ctx, record)
	if getErr != nil {
		return JobRecord{}, false, fmt.Errorf("load existing provider idempotency record: %w", getErr)
	}
	if existing.RequestHash != record.RequestHash {
		return JobRecord{}, false, ErrIdempotencyConflict
	}
	if observabilityErr := s.ensureInitialJobObservability(ctx, existing); observabilityErr != nil {
		return JobRecord{}, false, observabilityErr
	}
	return existing, true, nil
}

func (s MySQLStore) Get(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, jobID string) (JobRecord, error) {
	if s.DB == nil {
		return JobRecord{}, fmt.Errorf("MySQL database is required")
	}
	row := s.DB.QueryRowContext(ctx, `SELECT
		id, organization_id, project_id, principal_kind, principal_id, operation_name,
		idempotency_key, request_hash, kind, model_alias, route_snapshot, source_system, source_task_id,
		project_context_version, execution_status, provider_status, progress, provider_code, model_version, external_task_id, input_payload,
		submission_state, adapter_request_id, actual_provider, actual_model, execution_deadline_at, submitted_at, response_received_at,
		error_code, error_message, retryable, attempt_count, max_attempts, version, created_at, updated_at
		FROM provider_jobs WHERE id = ? AND organization_id = ? AND project_id = ?`, jobID, organizationID, projectID)
	record, err := scanRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return JobRecord{}, ErrJobNotFound
	}
	if err != nil {
		return JobRecord{}, err
	}
	outputs, err := s.loadOutputs(ctx, record.Job.ID, record.Job.ProjectID)
	if err != nil {
		return JobRecord{}, err
	}
	record.Outputs = outputs
	record.Job.ProjectAssetRefs = projectAssetRefs(outputs)
	if err = s.loadJobObservability(ctx, &record); err != nil {
		return JobRecord{}, err
	}
	return record, nil
}

func (s MySQLStore) ListJobs(ctx context.Context, filter JobQueryFilter) ([]JobRecord, bool, error) {
	if s.DB == nil {
		return nil, false, fmt.Errorf("MySQL database is required")
	}
	query := `SELECT
		id, organization_id, project_id, principal_kind, principal_id, operation_name,
		idempotency_key, request_hash, kind, model_alias, route_snapshot, source_system, source_task_id,
		project_context_version, execution_status, provider_status, progress, provider_code, model_version, external_task_id, input_payload,
		submission_state, adapter_request_id, actual_provider, actual_model, execution_deadline_at, submitted_at, response_received_at,
		error_code, error_message, retryable, attempt_count, max_attempts, version, created_at, updated_at
		FROM provider_jobs WHERE organization_id = ? AND project_id = ?`
	args := []any{filter.OrganizationID, filter.ProjectID}
	if filter.CreativeOnly {
		query += ` AND (source_system = 'creative' OR source_system LIKE 'creative.%')`
	}
	if len(filter.Statuses) > 0 {
		query += ` AND provider_status IN (` + strings.TrimSuffix(strings.Repeat("?,", len(filter.Statuses)), ",") + `)`
		for _, status := range filter.Statuses {
			args = append(args, status)
		}
	}
	switch filter.MediaKind {
	case "image":
		query += ` AND kind LIKE 'provider.image.%'`
	case "video":
		query += ` AND kind LIKE 'provider.video.%'`
	case "":
	default:
		query += ` AND 1=0`
	}
	if filter.SourceTaskID != "" {
		query += ` AND source_task_id = ?`
		args = append(args, filter.SourceTaskID)
	}
	if filter.CreatedAfter != nil {
		query += ` AND created_at >= ?`
		args = append(args, filter.CreatedAfter.UTC())
	}
	if filter.CreatedBefore != nil {
		query += ` AND created_at < ?`
		args = append(args, filter.CreatedBefore.UTC())
	}
	if value := strings.TrimSpace(filter.Query); value != "" {
		query += ` AND (id LIKE ? OR error_code = ?)`
		args = append(args, "%"+value+"%", value)
	}
	if filter.BeforeCreated != nil {
		query += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, filter.BeforeCreated.UTC(), filter.BeforeCreated.UTC(), filter.BeforeID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, filter.Limit+1)
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	records := make([]JobRecord, 0, filter.Limit+1)
	for rows.Next() {
		record, scanErr := scanRecord(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(records) > filter.Limit
	if hasMore {
		records = records[:filter.Limit]
	}
	for index := range records {
		outputs, loadErr := s.loadOutputs(ctx, records[index].Job.ID, records[index].Job.ProjectID)
		if loadErr != nil {
			return nil, false, loadErr
		}
		records[index].Outputs = outputs
		records[index].Job.ProjectAssetRefs = projectAssetRefs(outputs)
		if loadErr = s.loadJobObservability(ctx, &records[index]); loadErr != nil {
			return nil, false, loadErr
		}
	}
	return records, hasMore, nil
}

func (s MySQLStore) Update(ctx context.Context, record JobRecord) (JobRecord, error) {
	if s.DB == nil {
		return JobRecord{}, fmt.Errorf("MySQL database is required")
	}
	if err := validateRecord(record, s.AllowInsecureHTTP); err != nil {
		return JobRecord{}, err
	}
	payload, err := marshalProviderInput(record)
	if err != nil {
		return JobRecord{}, fmt.Errorf("encode provider input: %w", err)
	}
	var errorCode, errorMessage any
	var retryable any
	if record.Job.Error != nil {
		errorCode = record.Job.Error.Code
		errorMessage = record.Job.Error.Message
		retryable = record.Job.Error.Retryable
	}
	routeSnapshot, err := marshalRouteSnapshot(record.Route, record.Operation, s.AllowInsecureHTTP)
	if err != nil {
		return JobRecord{}, err
	}
	submissionState := record.SubmissionState
	if submissionState == "" {
		submissionState = SubmissionNotStarted
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return JobRecord{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE provider_jobs SET
		model_alias = ?, route_snapshot = ?, source_system = NULLIF(?, ''), source_task_id = NULLIF(?, ''),
		project_context_version = ?, execution_status = ?, provider_status = ?, progress = ?,
		provider_code = NULLIF(?, ''), model_version = NULLIF(?, ''), external_task_id = NULLIF(?, ''),
		input_payload = ?, submission_state = ?, adapter_request_id = NULLIF(?, ''),
		actual_provider = NULLIF(?, ''), actual_model = NULLIF(?, ''), execution_deadline_at = ?,
		submitted_at = ?, response_received_at = ?, error_code = ?, error_message = ?, retryable = ?, attempt_count = ?,
		max_attempts = ?, version = version + 1, updated_at = ?
		WHERE id = ? AND organization_id = ? AND project_id = ? AND version = ?`,
		record.ModelAlias, routeSnapshot, record.SourceSystem, record.SourceTaskID, record.ProjectContextVersion,
		record.Job.ExecutionStatus, record.Job.ProviderStatus, record.Job.Progress,
		record.ProviderCode, record.ModelVersion, record.ExternalTaskID, payload,
		submissionState, record.AdapterRequestID, record.ActualProvider, record.ActualModel,
		record.ExecutionDeadlineAt, record.SubmittedAt, record.ResponseReceivedAt,
		errorCode, errorMessage, retryable, record.Job.AttemptCount, record.Job.MaxAttempts, record.Job.UpdatedAt,
		record.Job.ID, record.Job.OrganizationID, record.Job.ProjectID, record.Job.Version)
	if err != nil {
		return JobRecord{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return JobRecord{}, err
	}
	if changed != 1 {
		return JobRecord{}, ErrVersionConflict
	}
	for _, output := range record.Outputs {
		if err := upsertOutput(ctx, tx, record.Job.ID, output); err != nil {
			return JobRecord{}, err
		}
	}
	if err := appendProviderJobEvent(ctx, tx, record); err != nil {
		return JobRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return JobRecord{}, err
	}
	record.Job.Version++
	return record, nil
}

func (s MySQLStore) getByIdempotency(ctx context.Context, record JobRecord) (JobRecord, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT
		id, organization_id, project_id, principal_kind, principal_id, operation_name,
		idempotency_key, request_hash, kind, model_alias, route_snapshot, source_system, source_task_id,
		project_context_version, execution_status, provider_status, progress, provider_code, model_version, external_task_id, input_payload,
		submission_state, adapter_request_id, actual_provider, actual_model, execution_deadline_at, submitted_at, response_received_at,
		error_code, error_message, retryable, attempt_count, max_attempts, version, created_at, updated_at
		FROM provider_jobs
		WHERE organization_id = ? AND project_id = ? AND principal_kind = ? AND principal_id = ?
			AND operation_name = ? AND idempotency_key = ?`,
		record.Job.OrganizationID, record.Job.ProjectID, record.Principal.Kind, record.Principal.ID,
		record.Operation, record.IdempotencyKey)
	existing, err := scanRecord(row)
	if err != nil {
		return JobRecord{}, err
	}
	outputs, err := s.loadOutputs(ctx, existing.Job.ID, existing.Job.ProjectID)
	if err != nil {
		return JobRecord{}, err
	}
	existing.Outputs = outputs
	existing.Job.ProjectAssetRefs = projectAssetRefs(outputs)
	if err = s.loadJobObservability(ctx, &existing); err != nil {
		return JobRecord{}, err
	}
	return existing, nil
}

func (s MySQLStore) ensureInitialJobObservability(ctx context.Context, record JobRecord) error {
	usage := requestedJobUsage(record)
	_, err := s.DB.ExecContext(ctx, `INSERT IGNORE INTO provider_job_usage
		(provider_job_id,organization_id,project_id,unit_kind,requested_units,billed_units,currency,actual_cost_minor,measured_at)
		VALUES (?,?,?,?,?,0,'CNY',NULL,?)`, record.Job.ID, record.Job.OrganizationID, record.Job.ProjectID, usage.UnitKind, usage.RequestedUnits, record.Job.CreatedAt)
	if err != nil {
		return err
	}
	event := sanitizeJobEvent(JobEvent{Ordinal: 1, Stage: "queued", SafeMessage: "Provider job persisted before scheduling.", OccurredAt: record.Job.CreatedAt})
	_, err = s.DB.ExecContext(ctx, `INSERT IGNORE INTO provider_job_events
		(provider_job_id,organization_id,project_id,ordinal,stage,safe_message,error_code,occurred_at)
		VALUES (?,?,?,?,?,?,NULL,?)`, record.Job.ID, record.Job.OrganizationID, record.Job.ProjectID, event.Ordinal, event.Stage, event.SafeMessage, event.OccurredAt)
	return err
}

func requestedJobUsage(record JobRecord) JobUsage {
	usage := JobUsage{UnitKind: UsageUnitImageCount, RequestedUnits: 1, Currency: "CNY", MeasuredAt: record.Job.CreatedAt}
	if record.Operation == videoGenerateOperation {
		usage.UnitKind = UsageUnitVideoSeconds
		usage.RequestedUnits = int64(record.VideoInput.DurationSeconds)
	}
	return usage
}

func appendProviderJobEvent(ctx context.Context, tx *sql.Tx, record JobRecord) error {
	stage := string(record.Job.ProviderStatus)
	message := providerStageMessage(record.Job.ProviderStatus)
	errorCode := sql.NullString{}
	if record.Job.Error != nil {
		errorCode = sql.NullString{String: boundedSafeToken(record.Job.Error.Code, 128, "PROVIDER_ERROR"), Valid: true}
	}
	event := sanitizeJobEvent(JobEvent{Stage: stage, SafeMessage: message, ErrorCode: errorCode.String, OccurredAt: record.Job.UpdatedAt})
	_, err := tx.ExecContext(ctx, `INSERT INTO provider_job_events
		(provider_job_id,organization_id,project_id,ordinal,stage,safe_message,error_code,occurred_at)
		SELECT ?,?,?,COALESCE(MAX(ordinal),0)+1,?,?,?,? FROM provider_job_events WHERE provider_job_id=?`,
		record.Job.ID, record.Job.OrganizationID, record.Job.ProjectID, event.Stage, event.SafeMessage,
		sql.NullString{String: event.ErrorCode, Valid: event.ErrorCode != ""}, event.OccurredAt, record.Job.ID)
	return err
}

func providerStageMessage(status contract.ProviderJobStatus) string {
	switch status {
	case contract.ProviderJobSubmitted:
		return "Provider job was submitted."
	case contract.ProviderJobRunning:
		return "Provider job is running."
	case contract.ProviderJobOutputsReady:
		return "Provider outputs are ready for stable asset intake."
	case contract.ProviderJobIngesting:
		return "Provider outputs are being ingested as stable assets."
	case contract.ProviderJobSucceeded:
		return "Provider job completed with stable asset outputs."
	case contract.ProviderJobPartiallySucceeded:
		return "Provider job completed with some failed outputs."
	case contract.ProviderJobFailed:
		return "Provider job failed."
	case contract.ProviderJobCancelled:
		return "Provider job was cancelled."
	case contract.ProviderJobExpired:
		return "Provider job expired."
	default:
		return "Provider state changed."
	}
}

func (s MySQLStore) loadJobObservability(ctx context.Context, record *JobRecord) error {
	var usage JobUsage
	var cost sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT unit_kind,requested_units,billed_units,currency,actual_cost_minor,measured_at
		FROM provider_job_usage WHERE provider_job_id=? AND organization_id=? AND project_id=?`,
		record.Job.ID, record.Job.OrganizationID, record.Job.ProjectID).Scan(&usage.UnitKind, &usage.RequestedUnits, &usage.BilledUnits, &usage.Currency, &cost, &usage.MeasuredAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		if cost.Valid {
			usage.ActualCostMinor = &cost.Int64
		}
		record.Usage = &usage
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT ordinal,stage,safe_message,COALESCE(error_code,''),occurred_at
		FROM provider_job_events WHERE provider_job_id=? AND organization_id=? AND project_id=? ORDER BY ordinal`,
		record.Job.ID, record.Job.OrganizationID, record.Job.ProjectID)
	if err != nil {
		return err
	}
	defer rows.Close()
	record.Events = []JobEvent{}
	for rows.Next() {
		var event JobEvent
		if err := rows.Scan(&event.Ordinal, &event.Stage, &event.SafeMessage, &event.ErrorCode, &event.OccurredAt); err != nil {
			return err
		}
		record.Events = append(record.Events, sanitizeJobEvent(event))
	}
	return rows.Err()
}

func (s MySQLStore) RecordJobUsage(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, jobID string, usage JobUsage) error {
	if err := usage.Validate(); err != nil {
		return err
	}
	result, err := s.DB.ExecContext(ctx, `UPDATE provider_job_usage SET unit_kind=?,requested_units=?,billed_units=?,currency=?,actual_cost_minor=?,measured_at=?
		WHERE provider_job_id=? AND organization_id=? AND project_id=?`, usage.UnitKind, usage.RequestedUnits, usage.BilledUnits, usage.Currency,
		usage.ActualCostMinor, usage.MeasuredAt, jobID, organizationID, projectID)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return ErrJobNotFound
	}
	return nil
}

func projectAssetRefs(outputs []OutputRecord) []contract.ProjectAssetRef {
	refs := make([]contract.ProjectAssetRef, 0, len(outputs))
	for _, output := range outputs {
		if output.Status == OutputSucceeded && output.ProjectAssetRef != nil {
			refs = append(refs, *output.ProjectAssetRef)
		}
	}
	return refs
}

func (s MySQLStore) loadOutputs(ctx context.Context, jobID string, projectID contract.ProjectID) ([]OutputRecord, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT output_id, provider_code, retrieval_expires_at, declared_mime_type,
		declared_size_bytes, declared_sha256, output_status, intake_id, asset_id, asset_version,
		error_code, error_message, retryable
		FROM provider_job_outputs WHERE provider_job_id = ? ORDER BY output_id`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	outputs := make([]OutputRecord, 0)
	for rows.Next() {
		var output OutputRecord
		var declaredSHA256, intakeID, assetID, errorCode, errorMessage sql.NullString
		var assetVersion sql.NullInt64
		var retryable sql.NullBool
		if err := rows.Scan(&output.Ref.OutputID, &output.Ref.ProviderCode, &output.Ref.RetrievalExpiresAt,
			&output.Ref.DeclaredMIMEType, &output.Ref.DeclaredSizeBytes, &declaredSHA256, &output.Status,
			&intakeID, &assetID, &assetVersion, &errorCode, &errorMessage, &retryable); err != nil {
			return nil, err
		}
		output.Ref.ProviderJobID = jobID
		if declaredSHA256.Valid {
			value := declaredSHA256.String
			output.Ref.DeclaredSHA256 = &value
		}
		output.IntakeID = intakeID.String
		if assetID.Valid && assetVersion.Valid {
			output.ProjectAssetRef = &contract.ProjectAssetRef{ProjectID: projectID, AssetVersion: contract.AssetVersionRef{AssetID: contract.AssetID(assetID.String), Version: assetVersion.Int64}}
		}
		if errorCode.Valid {
			output.Error = &contract.JobError{Code: errorCode.String, Message: errorMessage.String, Retryable: retryable.Bool}
		}
		outputs = append(outputs, output)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return outputs, nil
}

func upsertOutput(ctx context.Context, tx *sql.Tx, jobID string, output OutputRecord) error {
	if err := validateOutput(jobID, output); err != nil {
		return err
	}
	var declaredSHA256 any
	if output.Ref.DeclaredSHA256 != nil {
		declaredSHA256 = *output.Ref.DeclaredSHA256
	}
	var assetID, assetVersion any
	if output.ProjectAssetRef != nil {
		assetID = output.ProjectAssetRef.AssetVersion.AssetID
		assetVersion = output.ProjectAssetRef.AssetVersion.Version
	}
	var errorCode, errorMessage, retryable any
	if output.Error != nil {
		errorCode, errorMessage, retryable = output.Error.Code, output.Error.Message, output.Error.Retryable
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO provider_job_outputs (
		provider_job_id, output_id, provider_code, retrieval_expires_at, declared_mime_type,
		declared_size_bytes, declared_sha256, output_status, intake_id, asset_id, asset_version,
		error_code, error_message, retryable
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE provider_code = VALUES(provider_code), retrieval_expires_at = VALUES(retrieval_expires_at),
		declared_mime_type = VALUES(declared_mime_type), declared_size_bytes = VALUES(declared_size_bytes),
		declared_sha256 = VALUES(declared_sha256), output_status = VALUES(output_status), intake_id = VALUES(intake_id),
		asset_id = VALUES(asset_id), asset_version = VALUES(asset_version), error_code = VALUES(error_code),
		error_message = VALUES(error_message), retryable = VALUES(retryable)`,
		jobID, output.Ref.OutputID, output.Ref.ProviderCode, output.Ref.RetrievalExpiresAt,
		output.Ref.DeclaredMIMEType, output.Ref.DeclaredSizeBytes, declaredSHA256, output.Status, output.IntakeID,
		assetID, assetVersion, errorCode, errorMessage, retryable)
	return err
}

type rowScanner interface{ Scan(...any) error }

func scanRecord(row rowScanner) (JobRecord, error) {
	var record JobRecord
	var input json.RawMessage
	var sourceSystem, sourceTaskID, routeSnapshot, providerCode, modelVersion, externalTaskID, adapterRequestID, actualProvider, actualModel, errorCode, errorMessage sql.NullString
	var executionDeadlineAt, submittedAt, responseReceivedAt sql.NullTime
	var retryable sql.NullBool
	err := row.Scan(
		&record.Job.ID, &record.Job.OrganizationID, &record.Job.ProjectID, &record.Principal.Kind, &record.Principal.ID, &record.Operation,
		&record.IdempotencyKey, &record.RequestHash, &record.Job.Kind, &record.ModelAlias, &routeSnapshot, &sourceSystem, &sourceTaskID,
		&record.ProjectContextVersion, &record.Job.ExecutionStatus, &record.Job.ProviderStatus, &record.Job.Progress,
		&providerCode, &modelVersion, &externalTaskID, &input,
		&record.SubmissionState, &adapterRequestID, &actualProvider, &actualModel, &executionDeadlineAt, &submittedAt, &responseReceivedAt,
		&errorCode, &errorMessage, &retryable, &record.Job.AttemptCount, &record.Job.MaxAttempts, &record.Job.Version,
		&record.Job.CreatedAt, &record.Job.UpdatedAt,
	)
	if err != nil {
		return JobRecord{}, err
	}
	record.SourceSystem = sourceSystem.String
	record.SourceTaskID = sourceTaskID.String
	record.ProviderCode = providerCode.String
	record.ModelVersion = modelVersion.String
	record.ExternalTaskID = externalTaskID.String
	record.AdapterRequestID = adapterRequestID.String
	record.ActualProvider = actualProvider.String
	record.ActualModel = actualModel.String
	if executionDeadlineAt.Valid {
		value := executionDeadlineAt.Time
		record.ExecutionDeadlineAt = &value
	}
	if submittedAt.Valid {
		value := submittedAt.Time
		record.SubmittedAt = &value
	}
	if responseReceivedAt.Valid {
		value := responseReceivedAt.Time
		record.ResponseReceivedAt = &value
	}
	if routeSnapshot.Valid && routeSnapshot.String != "" && routeSnapshot.String != "null" {
		var snapshot ImageRouteSnapshot
		if err := json.Unmarshal([]byte(routeSnapshot.String), &snapshot); err != nil {
			return JobRecord{}, fmt.Errorf("decode provider route snapshot: %w", err)
		}
		record.Route = &snapshot
	}
	record.Job.ProjectAssetRefs = []contract.ProjectAssetRef{}
	if errorCode.Valid {
		record.Job.Error = &contract.JobError{Code: errorCode.String, Message: errorMessage.String, Retryable: retryable.Bool}
	}
	if err := unmarshalProviderInput(input, &record); err != nil {
		return JobRecord{}, err
	}
	return record, nil
}

func validateRecord(record JobRecord, allowInsecureHTTP bool) error {
	if err := record.Job.Validate(); err != nil {
		return err
	}
	if err := (contract.ActorContext{OrganizationID: record.Job.OrganizationID, Principal: record.Principal, Scopes: []contract.Scope{}}).Validate(); err != nil {
		return fmt.Errorf("invalid principal: %w", err)
	}
	if strings.TrimSpace(record.Operation) == "" {
		return fmt.Errorf("provider operation is required")
	}
	if err := record.IdempotencyKey.Validate(); err != nil {
		return err
	}
	if !validSHA256(record.RequestHash) {
		return fmt.Errorf("request hash must be a lowercase hexadecimal SHA-256 digest")
	}
	if record.ProjectContextVersion < 1 {
		return fmt.Errorf("project_context_version must be positive")
	}
	if strings.TrimSpace(record.ModelAlias) == "" {
		return fmt.Errorf("model alias is required")
	}
	switch record.Operation {
	case imageGenerateOperation, imageEditOperation:
		if err := record.Input.Validate(); err != nil {
			return err
		}
	case videoGenerateOperation:
		if err := record.VideoInput.Validate(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("provider operation is not supported")
	}
	if record.Route != nil {
		if err := validateRouteForOperation(*record.Route, record.Operation, allowInsecureHTTP); err != nil {
			return err
		}
		switch record.SubmissionState {
		case SubmissionNotStarted, SubmissionInFlight, SubmissionCompleted, SubmissionUnknown:
		default:
			return fmt.Errorf("provider submission state is invalid")
		}
	}
	for _, output := range record.Outputs {
		if err := validateOutput(record.Job.ID, output); err != nil {
			return err
		}
	}
	return nil
}

func marshalProviderInput(record JobRecord) ([]byte, error) {
	var input any
	switch record.Operation {
	case imageGenerateOperation, imageEditOperation:
		input = record.Input
	case videoGenerateOperation:
		input = record.VideoInput
	default:
		return nil, fmt.Errorf("provider operation is not supported")
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode provider input: %w", err)
	}
	return payload, nil
}

func unmarshalProviderInput(input json.RawMessage, record *JobRecord) error {
	if record == nil {
		return fmt.Errorf("provider job record is required")
	}
	var target any
	switch record.Operation {
	case imageGenerateOperation, imageEditOperation:
		target = &record.Input
	case videoGenerateOperation:
		target = &record.VideoInput
	default:
		return fmt.Errorf("provider operation is not supported")
	}
	if err := json.Unmarshal(input, target); err != nil {
		return fmt.Errorf("decode provider input: %w", err)
	}
	return nil
}

func marshalRouteSnapshot(snapshot *ImageRouteSnapshot, operation string, allowInsecureHTTP bool) (any, error) {
	if snapshot == nil {
		return nil, nil
	}
	if err := validateRouteForOperation(*snapshot, operation, allowInsecureHTTP); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return nil, fmt.Errorf("encode provider route snapshot: %w", err)
	}
	return encoded, nil
}

func validateRouteForOperation(snapshot GatewayRouteSnapshot, operation string, allowInsecureHTTP bool) error {
	if operation == videoGenerateOperation {
		return snapshot.ValidateVideoWithPolicy(allowInsecureHTTP)
	}
	return snapshot.ValidateWithPolicy(allowInsecureHTTP)
}

func validateOutput(jobID string, output OutputRecord) error {
	if err := output.Ref.Validate(); err != nil {
		return fmt.Errorf("invalid provider output: %w", err)
	}
	if output.Ref.ProviderJobID != jobID {
		return fmt.Errorf("provider output belongs to another job")
	}
	switch output.Status {
	case OutputReady, OutputIngesting:
		if output.ProjectAssetRef != nil || output.Error != nil {
			return fmt.Errorf("pending output cannot include an asset or error")
		}
	case OutputSucceeded:
		if output.ProjectAssetRef == nil || output.Error != nil {
			return fmt.Errorf("succeeded output requires one project asset and no error")
		}
		if err := output.ProjectAssetRef.Validate(); err != nil {
			return err
		}
	case OutputFailed:
		if output.ProjectAssetRef != nil || output.Error == nil || strings.TrimSpace(output.Error.Code) == "" {
			return fmt.Errorf("failed output requires one error and no project asset")
		}
	default:
		return fmt.Errorf("provider output status is invalid")
	}
	return nil
}
