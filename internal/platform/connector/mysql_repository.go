package connector

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type MySQLRepository struct {
	DB     *sql.DB
	Cipher AccountSessionCipher
}

func (r MySQLRepository) ResolveExternalAccountID(ctx context.Context, organizationID, projectID, accountID string) (string, error) {
	db, err := r.db()
	if err != nil {
		return "", err
	}
	var externalID string
	err = db.QueryRowContext(ctx, `SELECT a.external_id FROM platform_accounts a JOIN platform_account_connections c ON c.organization_id=a.organization_id AND c.account_id=a.id WHERE a.organization_id=? AND c.project_id=? AND a.id=? AND a.platform='ocean_engine' AND a.status='verified' AND c.status='verified'`, organizationID, projectID, accountID).Scan(&externalID)
	return externalID, err
}

func (r MySQLRepository) ResolveAccountIDByExternalID(ctx context.Context, organizationID, projectID, externalID string) (string, error) {
	db, err := r.db()
	if err != nil {
		return "", err
	}
	var accountID string
	err = db.QueryRowContext(ctx, `SELECT a.id FROM platform_accounts a JOIN platform_account_connections c ON c.organization_id=a.organization_id AND c.account_id=a.id WHERE a.organization_id=? AND c.project_id=? AND a.external_id=? AND a.platform='ocean_engine' AND a.status='verified' AND c.status='verified'`, organizationID, projectID, externalID).Scan(&accountID)
	return accountID, err
}

func (r MySQLRepository) db() (*sql.DB, error) {
	if r.DB == nil {
		return nil, fmt.Errorf("connector database is not configured")
	}
	return r.DB, nil
}

func (r MySQLRepository) StartSync(ctx context.Context, value SyncRun) (bool, error) {
	if value.ID == "" || value.OrganizationID == "" || value.AccountRef == "" || value.StartedAt.IsZero() || value.Attempt < 1 {
		return false, ErrInvalidFact
	}
	db, err := r.db()
	if err != nil {
		return false, err
	}
	result, err := db.ExecContext(ctx, `INSERT IGNORE INTO connector_sync_runs (id,organization_id,project_id,account_ref,schema_version,status,cursor_ref,attempt,started_at,completed_at) VALUES (?,?,?,?,?,'running',?,?,?,NULL)`, value.ID, value.OrganizationID, value.ProjectID, value.AccountRef, DatasetVersion, value.Cursor, value.Attempt, value.StartedAt)
	if err != nil {
		return false, fmt.Errorf("start connector sync: %w", err)
	}
	return inserted(result)
}

func (r MySQLRepository) CompleteSync(ctx context.Context, id, cursor, status string, completedAt time.Time) error {
	if id == "" || completedAt.IsZero() || (status != "completed" && status != "failed") {
		return ErrInvalidFact
	}
	db, err := r.db()
	if err != nil {
		return err
	}
	result, err := db.ExecContext(ctx, `UPDATE connector_sync_runs SET status=?,cursor_ref=?,completed_at=? WHERE id=? AND completed_at IS NULL`, status, cursor, completedAt, id)
	if err != nil {
		return fmt.Errorf("complete connector sync: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 1 {
		return nil
	}
	var existingStatus, existingCursor string
	var existingAt time.Time
	if err := db.QueryRowContext(ctx, `SELECT status,cursor_ref,completed_at FROM connector_sync_runs WHERE id=?`, id).Scan(&existingStatus, &existingCursor, &existingAt); err != nil {
		return err
	}
	if existingStatus == status && existingCursor == cursor && existingAt.Equal(completedAt) {
		return nil
	}
	return ErrImmutableConflict
}

func (r MySQLRepository) UpdateSyncCursor(ctx context.Context, id, cursor string) error {
	if id == "" || cursor == "" {
		return ErrInvalidFact
	}
	db, err := r.db()
	if err != nil {
		return err
	}
	result, err := db.ExecContext(ctx, `UPDATE connector_sync_runs SET cursor_ref=? WHERE id=? AND status='running' AND completed_at IS NULL`, cursor, id)
	if err != nil {
		return fmt.Errorf("update connector sync cursor: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrImmutableConflict
	}
	return nil
}

func (r MySQLRepository) GetSync(ctx context.Context, organizationID, projectID, accountRef, id string) (SyncRun, error) {
	db, err := r.db()
	if err != nil {
		return SyncRun{}, err
	}
	var value SyncRun
	value.OrganizationID, value.ProjectID, value.AccountRef = organizationID, projectID, accountRef
	var completed sql.NullTime
	err = db.QueryRowContext(ctx, `SELECT id,status,cursor_ref,attempt,started_at,completed_at FROM connector_sync_runs WHERE organization_id=? AND project_id=? AND account_ref=? AND id=?`, organizationID, projectID, accountRef, id).Scan(&value.ID, &value.Status, &value.Cursor, &value.Attempt, &value.StartedAt, &completed)
	if err != nil {
		return SyncRun{}, err
	}
	if completed.Valid {
		value.CompletedAt = completed.Time
	}
	return value, nil
}

func (r MySQLRepository) AppendRaw(ctx context.Context, value RawSnapshot) (bool, error) {
	if err := value.Header.validate(); err != nil {
		return false, err
	}
	if value.ID == "" || value.Endpoint == "" || value.RequestHash == "" || len(value.EncryptedEvidence) == 0 || value.KeyVersion == "" {
		return false, ErrInvalidFact
	}
	db, err := r.db()
	if err != nil {
		return false, err
	}
	result, err := db.ExecContext(ctx, `INSERT IGNORE INTO connector_raw_snapshots (id,organization_id,project_id,source_system,source_ref,ingest_run_id,schema_version,endpoint_key,request_hash,payload_hash,encrypted_evidence,key_version,collected_at,available_at,data_through,valid_from,valid_to,quality_status) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.Header.OrganizationID, value.Header.ProjectID, value.Header.SourceSystem, value.Header.SourceRef, value.Header.IngestRunID, value.Header.SchemaVersion, value.Endpoint, value.RequestHash, value.Header.PayloadHash, value.EncryptedEvidence, value.KeyVersion, value.Header.CollectedAt, value.Header.AvailableAt, value.Header.DataThrough, value.Header.ValidFrom, value.Header.ValidTo, value.Header.QualityStatus)
	if err != nil {
		return false, fmt.Errorf("append raw snapshot: %w", err)
	}
	return r.verifyInsert(ctx, result, "connector_raw_snapshots", value.ID, value.Header.PayloadHash)
}

func (r MySQLRepository) AppendObject(ctx context.Context, value ObjectSnapshot) (bool, error) {
	if err := value.FactHeader.validate(); err != nil {
		return false, err
	}
	if value.ID == "" || value.ObjectKind == "" || value.ObjectRef == "" || containsSensitiveValue(value.State) {
		return false, ErrInvalidFact
	}
	payload, err := json.Marshal(value.State)
	if err != nil {
		return false, err
	}
	return r.appendObjectLike(ctx, "connector_object_snapshots", value.ID, value.FactHeader, value.ObjectKind, value.ObjectRef, value.ParentRef, payload)
}

func (r MySQLRepository) AppendConfiguration(ctx context.Context, value ConfigurationSnapshot) (bool, error) {
	if err := value.FactHeader.validate(); err != nil {
		return false, err
	}
	if value.ID == "" || value.ObjectRef == "" || containsSensitiveValue(value.Values) {
		return false, ErrInvalidFact
	}
	payload, err := json.Marshal(value.Values)
	if err != nil {
		return false, err
	}
	return r.appendObjectLike(ctx, "connector_configuration_snapshots", value.ID, value.FactHeader, "configuration", value.ObjectRef, "", payload)
}

func (r MySQLRepository) appendObjectLike(ctx context.Context, table, id string, h FactHeader, kind, objectRef, parentRef string, payload []byte) (bool, error) {
	db, err := r.db()
	if err != nil {
		return false, err
	}
	query := `INSERT IGNORE INTO ` + table + ` (id,organization_id,project_id,source_system,source_ref,object_kind,object_ref,parent_ref,ingest_run_id,raw_snapshot_id,schema_version,payload_hash,state_json,collected_at,available_at,data_through,valid_from,valid_to,quality_status) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	result, err := db.ExecContext(ctx, query, id, h.OrganizationID, h.ProjectID, h.SourceSystem, h.SourceRef, kind, objectRef, parentRef, h.IngestRunID, h.EvidenceRef, h.SchemaVersion, h.PayloadHash, payload, h.CollectedAt, h.AvailableAt, h.DataThrough, h.ValidFrom, h.ValidTo, h.QualityStatus)
	if err != nil {
		return false, fmt.Errorf("append %s: %w", table, err)
	}
	return r.verifyInsert(ctx, result, table, id, h.PayloadHash)
}

func (r MySQLRepository) AppendChange(ctx context.Context, value ConfigurationChangeEvent) (bool, error) {
	if err := value.FactHeader.validate(); err != nil {
		return false, err
	}
	if value.ID == "" || value.ObjectRef == "" || value.FieldPath == "" || value.BeforeSnapshotID == "" || value.AfterSnapshotID == "" || value.ObservedAt.IsZero() {
		return false, ErrInvalidFact
	}
	db, err := r.db()
	if err != nil {
		return false, err
	}
	oldValue, err := json.Marshal(value.OldValue)
	if err != nil {
		return false, err
	}
	newValue, err := json.Marshal(value.NewValue)
	if err != nil {
		return false, err
	}
	result, err := db.ExecContext(ctx, `INSERT IGNORE INTO connector_configuration_change_events (id,organization_id,project_id,source_system,source_ref,object_ref,ingest_run_id,schema_version,payload_hash,field_path,old_value_hash,new_value_hash,old_value_json,new_value_json,before_snapshot_id,after_snapshot_id,observed_at,collected_at,available_at,data_through,valid_from,valid_to,quality_status) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.OrganizationID, value.ProjectID, value.SourceSystem, value.SourceRef, value.ObjectRef, value.IngestRunID, value.SchemaVersion, value.PayloadHash, value.FieldPath, value.OldValueHash, value.NewValueHash, oldValue, newValue, value.BeforeSnapshotID, value.AfterSnapshotID, value.ObservedAt, value.CollectedAt, value.AvailableAt, value.DataThrough, value.ValidFrom, value.ValidTo, value.QualityStatus)
	if err != nil {
		return false, fmt.Errorf("append configuration change: %w", err)
	}
	return r.verifyInsert(ctx, result, "connector_configuration_change_events", value.ID, value.PayloadHash)
}

func (r MySQLRepository) AppendMetric(ctx context.Context, value MetricWindow) (bool, error) {
	return r.appendMetricLike(ctx, "connector_metric_windows", value)
}

func (r MySQLRepository) AppendMaterialMetric(ctx context.Context, value MaterialMetricWindow) (bool, error) {
	if value.MaterialRef == "" || value.PromotionRef == "" && !hasQualityIssue(value.QualityIssues, "material_binding_unresolved") {
		return false, ErrInvalidFact
	}
	return r.appendExtendedMetric(ctx, "connector_material_metric_windows", value.MetricWindow, []string{"material_ref", "promotion_ref"}, []any{value.MaterialRef, value.PromotionRef})
}

func (r MySQLRepository) AppendConversionRevision(ctx context.Context, value ConversionRevision) (bool, error) {
	if value.OriginalWindowID == "" || value.RevisionNumber < 1 || value.RevisionOf == "" {
		return false, ErrInvalidFact
	}
	return r.appendExtendedMetric(ctx, "connector_conversion_revisions", value.MetricWindow, []string{"original_window_id", "revision_number"}, []any{value.OriginalWindowID, value.RevisionNumber})
}

func (r MySQLRepository) appendExtendedMetric(ctx context.Context, table string, value MetricWindow, columns []string, extra []any) (bool, error) {
	if err := validateMetric(value); err != nil {
		return false, err
	}
	payload, err := json.Marshal(value.Metrics)
	if err != nil {
		return false, err
	}
	qualityIssues, err := json.Marshal(value.QualityIssues)
	if err != nil {
		return false, err
	}
	db, err := r.db()
	if err != nil {
		return false, err
	}
	query := `INSERT IGNORE INTO ` + table + ` (id,organization_id,project_id,source_system,source_ref,object_ref,ingest_run_id,raw_snapshot_id,schema_version,payload_hash,window_start,window_end,granularity,platform_timezone,attribution_window,metric_definition_version,currency,amount_unit,metrics_json,quality_issues_json,revision_of,collected_at,available_at,data_through,valid_from,valid_to,quality_status,` + strings.Join(columns, ",") + `) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?` + strings.Repeat(",?", len(extra)) + `)`
	args := []any{value.ID, value.OrganizationID, value.ProjectID, value.SourceSystem, value.SourceRef, value.ObjectRef, value.IngestRunID, value.EvidenceRef, value.SchemaVersion, value.PayloadHash, value.WindowStart, value.WindowEnd, value.Granularity, value.TimeZone, value.AttributionWindow, value.MetricDefinitionVersion, value.Currency, value.AmountUnit, payload, qualityIssues, nullable(value.RevisionOf), value.CollectedAt, value.AvailableAt, value.DataThrough, value.ValidFrom, value.ValidTo, value.QualityStatus}
	args = append(args, extra...)
	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("append %s: %w", table, err)
	}
	return r.verifyInsert(ctx, result, table, value.ID, value.PayloadHash)
}

func (r MySQLRepository) appendMetricLike(ctx context.Context, table string, value MetricWindow) (bool, error) {
	if err := validateMetric(value); err != nil {
		return false, err
	}
	payload, err := json.Marshal(value.Metrics)
	if err != nil {
		return false, err
	}
	qualityIssues, err := json.Marshal(value.QualityIssues)
	if err != nil {
		return false, err
	}
	db, err := r.db()
	if err != nil {
		return false, err
	}
	query := `INSERT IGNORE INTO ` + table + ` (id,organization_id,project_id,source_system,source_ref,object_ref,ingest_run_id,raw_snapshot_id,schema_version,payload_hash,window_start,window_end,granularity,platform_timezone,attribution_window,metric_definition_version,currency,amount_unit,metrics_json,quality_issues_json,revision_of,collected_at,available_at,data_through,valid_from,valid_to,quality_status) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	result, err := db.ExecContext(ctx, query, value.ID, value.OrganizationID, value.ProjectID, value.SourceSystem, value.SourceRef, value.ObjectRef, value.IngestRunID, value.EvidenceRef, value.SchemaVersion, value.PayloadHash, value.WindowStart, value.WindowEnd, value.Granularity, value.TimeZone, value.AttributionWindow, value.MetricDefinitionVersion, value.Currency, value.AmountUnit, payload, qualityIssues, nullable(value.RevisionOf), value.CollectedAt, value.AvailableAt, value.DataThrough, value.ValidFrom, value.ValidTo, value.QualityStatus)
	if err != nil {
		return false, fmt.Errorf("append %s: %w", table, err)
	}
	return r.verifyInsert(ctx, result, table, value.ID, value.PayloadHash)
}

func (r MySQLRepository) AppendBinding(ctx context.Context, value MaterialBinding) (bool, error) {
	if err := value.FactHeader.validate(); err != nil {
		return false, err
	}
	if value.ID == "" || value.MaterialRef == "" || value.PromotionRef == "" {
		return false, ErrInvalidFact
	}
	state, _ := json.Marshal(map[string]string{"material_ref": value.MaterialRef, "promotion_ref": value.PromotionRef})
	db, err := r.db()
	if err != nil {
		return false, err
	}
	result, err := db.ExecContext(ctx, `INSERT IGNORE INTO connector_material_bindings (id,organization_id,project_id,source_system,source_ref,object_kind,object_ref,parent_ref,ingest_run_id,raw_snapshot_id,schema_version,payload_hash,state_json,collected_at,available_at,data_through,valid_from,valid_to,quality_status,material_ref,promotion_ref) VALUES (?,?,?,?,?,'material_binding',?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.OrganizationID, value.ProjectID, value.SourceSystem, value.SourceRef, value.MaterialRef, value.PromotionRef, value.IngestRunID, value.EvidenceRef, value.SchemaVersion, value.PayloadHash, state, value.CollectedAt, value.AvailableAt, value.DataThrough, value.ValidFrom, value.ValidTo, value.QualityStatus, value.MaterialRef, value.PromotionRef)
	if err != nil {
		return false, fmt.Errorf("append material binding: %w", err)
	}
	return r.verifyInsert(ctx, result, "connector_material_bindings", value.ID, value.PayloadHash)
}

func (r MySQLRepository) AppendStatus(ctx context.Context, value PlatformStatusEvent) (bool, error) {
	if err := value.FactHeader.validate(); err != nil {
		return false, err
	}
	if value.ID == "" || value.ObjectRef == "" || value.Status == "" {
		return false, ErrInvalidFact
	}
	state, _ := json.Marshal(map[string]string{"status": value.Status, "reason": value.Reason})
	db, err := r.db()
	if err != nil {
		return false, err
	}
	result, err := db.ExecContext(ctx, `INSERT IGNORE INTO connector_platform_status_events (id,organization_id,project_id,source_system,source_ref,object_kind,object_ref,parent_ref,ingest_run_id,raw_snapshot_id,schema_version,payload_hash,state_json,collected_at,available_at,data_through,valid_from,valid_to,quality_status,platform_status,status_reason) VALUES (?,?,?,?,?,'platform_status',?,'',?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.OrganizationID, value.ProjectID, value.SourceSystem, value.SourceRef, value.ObjectRef, value.IngestRunID, value.EvidenceRef, value.SchemaVersion, value.PayloadHash, state, value.CollectedAt, value.AvailableAt, value.DataThrough, value.ValidFrom, value.ValidTo, value.QualityStatus, value.Status, value.Reason)
	if err != nil {
		return false, fmt.Errorf("append platform status: %w", err)
	}
	return r.verifyInsert(ctx, result, "connector_platform_status_events", value.ID, value.PayloadHash)
}

func (r MySQLRepository) AppendDiagnosis(ctx context.Context, value PlatformDiagnosisSnapshot) (bool, error) {
	if value.EligibleAsPrelaunchFeature || containsSensitiveValue(value.Diagnosis) {
		return false, ErrInvalidFact
	}
	state, err := json.Marshal(value.Diagnosis)
	if err != nil {
		return false, err
	}
	if err := value.FactHeader.validate(); err != nil {
		return false, err
	}
	if value.ID == "" || value.ObjectRef == "" {
		return false, ErrInvalidFact
	}
	db, err := r.db()
	if err != nil {
		return false, err
	}
	result, err := db.ExecContext(ctx, `INSERT IGNORE INTO connector_platform_diagnosis_snapshots (id,organization_id,project_id,source_system,source_ref,object_kind,object_ref,parent_ref,ingest_run_id,raw_snapshot_id,schema_version,payload_hash,state_json,collected_at,available_at,data_through,valid_from,valid_to,quality_status,eligible_as_prelaunch_feature) VALUES (?,?,?,?,?,'platform_diagnosis',?,'',?,?,?,?,?,?,?,?,?,?,?,FALSE)`, value.ID, value.OrganizationID, value.ProjectID, value.SourceSystem, value.SourceRef, value.ObjectRef, value.IngestRunID, value.EvidenceRef, value.SchemaVersion, value.PayloadHash, state, value.CollectedAt, value.AvailableAt, value.DataThrough, value.ValidFrom, value.ValidTo, value.QualityStatus)
	if err != nil {
		return false, fmt.Errorf("append platform diagnosis: %w", err)
	}
	return r.verifyInsert(ctx, result, "connector_platform_diagnosis_snapshots", value.ID, value.PayloadHash)
}

func inserted(result sql.Result) (bool, error) {
	count, err := result.RowsAffected()
	return count == 1, err
}

func (r MySQLRepository) verifyInsert(ctx context.Context, result sql.Result, table, id, payloadHash string) (bool, error) {
	created, err := inserted(result)
	if err != nil || created {
		return created, err
	}
	if !safeLedgerTable(table) {
		return false, ErrInvalidFact
	}
	db, err := r.db()
	if err != nil {
		return false, err
	}
	var current string
	if err := db.QueryRowContext(ctx, `SELECT payload_hash FROM `+table+` WHERE id=?`, id).Scan(&current); err != nil {
		return false, err
	}
	if current != payloadHash {
		return false, ErrImmutableConflict
	}
	return false, nil
}

func safeLedgerTable(value string) bool {
	switch value {
	case "connector_raw_snapshots", "connector_object_snapshots", "connector_configuration_snapshots", "connector_configuration_change_events", "connector_metric_windows", "connector_material_bindings", "connector_material_metric_windows", "connector_platform_status_events", "connector_platform_diagnosis_snapshots", "connector_conversion_revisions":
		return true
	default:
		return false
	}
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func (r MySQLRepository) Snapshot(ctx context.Context, q Query) (CanonicalSnapshot, error) {
	if q.OrganizationID == "" || q.PredictionCutoff.IsZero() {
		return CanonicalSnapshot{}, ErrInvalidFact
	}
	result := CanonicalSnapshot{DatasetVersion: DatasetVersion, PredictionCutoff: q.PredictionCutoff}
	var err error
	result.Objects, err = r.readObjects(ctx, "connector_object_snapshots", q)
	if err != nil {
		return CanonicalSnapshot{}, err
	}
	result.Configurations, err = r.readConfigurations(ctx, q)
	if err != nil {
		return CanonicalSnapshot{}, err
	}
	result.Metrics, err = r.readMetrics(ctx, "connector_metric_windows", q)
	if err != nil {
		return CanonicalSnapshot{}, err
	}
	result.Changes, err = r.readChanges(ctx, q)
	if err != nil {
		return CanonicalSnapshot{}, err
	}
	result.Bindings, err = r.readBindings(ctx, q)
	if err != nil {
		return CanonicalSnapshot{}, err
	}
	result.Statuses, err = r.readStatuses(ctx, q)
	if err != nil {
		return CanonicalSnapshot{}, err
	}
	if q.IncludeDiagnosis {
		result.Diagnoses, err = r.readDiagnoses(ctx, q)
		if err != nil {
			return CanonicalSnapshot{}, err
		}
	}
	result.MaterialMetrics, err = r.readMaterialMetrics(ctx, q)
	if err != nil {
		return CanonicalSnapshot{}, err
	}
	result.ConversionRevisions, err = r.readConversions(ctx, q)
	if err != nil {
		return CanonicalSnapshot{}, err
	}
	return result, nil
}

func (r MySQLRepository) readChanges(ctx context.Context, q Query) ([]ConfigurationChangeEvent, error) {
	db, err := r.db()
	if err != nil {
		return nil, err
	}
	query := `SELECT id,source_system,source_ref,object_ref,ingest_run_id,schema_version,payload_hash,field_path,old_value_hash,new_value_hash,old_value_json,new_value_json,before_snapshot_id,after_snapshot_id,observed_at,collected_at,available_at,data_through,valid_from,valid_to,quality_status FROM connector_configuration_change_events WHERE organization_id=? AND project_id=? AND available_at<=? AND quality_status<>'reject'`
	args := []any{q.OrganizationID, q.ProjectID, q.PredictionCutoff}
	if q.SourceRef != "" {
		query += ` AND source_ref=?`
		args = append(args, q.SourceRef)
	}
	if q.ObjectRef != "" {
		query += ` AND object_ref=?`
		args = append(args, q.ObjectRef)
	}
	query += ` ORDER BY available_at,id`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ConfigurationChangeEvent{}
	for rows.Next() {
		var v ConfigurationChangeEvent
		var validTo sql.NullTime
		var oldValue, newValue []byte
		v.OrganizationID, v.ProjectID = q.OrganizationID, q.ProjectID
		if err := rows.Scan(&v.ID, &v.SourceSystem, &v.SourceRef, &v.ObjectRef, &v.IngestRunID, &v.SchemaVersion, &v.PayloadHash, &v.FieldPath, &v.OldValueHash, &v.NewValueHash, &oldValue, &newValue, &v.BeforeSnapshotID, &v.AfterSnapshotID, &v.ObservedAt, &v.CollectedAt, &v.AvailableAt, &v.DataThrough, &v.ValidFrom, &validTo, &v.QualityStatus); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(oldValue, &v.OldValue); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(newValue, &v.NewValue); err != nil {
			return nil, err
		}
		if validTo.Valid {
			v.ValidTo = &validTo.Time
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (r MySQLRepository) readBindings(ctx context.Context, q Query) ([]MaterialBinding, error) {
	objects, err := r.readObjects(ctx, "connector_material_bindings", q)
	if err != nil {
		return nil, err
	}
	result := make([]MaterialBinding, 0, len(objects))
	for _, v := range objects {
		result = append(result, MaterialBinding{FactHeader: v.FactHeader, ID: v.ID, MaterialRef: v.ObjectRef, PromotionRef: v.ParentRef})
	}
	return result, nil
}
func (r MySQLRepository) readStatuses(ctx context.Context, q Query) ([]PlatformStatusEvent, error) {
	objects, err := r.readObjects(ctx, "connector_platform_status_events", q)
	if err != nil {
		return nil, err
	}
	result := make([]PlatformStatusEvent, 0, len(objects))
	for _, v := range objects {
		result = append(result, PlatformStatusEvent{FactHeader: v.FactHeader, ID: v.ID, ObjectRef: v.ObjectRef, Status: stringValue(v.State, "status"), Reason: stringValue(v.State, "reason")})
	}
	return result, nil
}
func (r MySQLRepository) readDiagnoses(ctx context.Context, q Query) ([]PlatformDiagnosisSnapshot, error) {
	objects, err := r.readObjects(ctx, "connector_platform_diagnosis_snapshots", q)
	if err != nil {
		return nil, err
	}
	result := make([]PlatformDiagnosisSnapshot, 0, len(objects))
	for _, v := range objects {
		result = append(result, PlatformDiagnosisSnapshot{FactHeader: v.FactHeader, ID: v.ID, ObjectRef: v.ObjectRef, EligibleAsPrelaunchFeature: false, Diagnosis: v.State})
	}
	return result, nil
}
func stringValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func (r MySQLRepository) readMaterialMetrics(ctx context.Context, q Query) ([]MaterialMetricWindow, error) {
	base, err := r.readMetrics(ctx, "connector_material_metric_windows", q)
	if err != nil {
		return nil, err
	}
	db, _ := r.db()
	result := make([]MaterialMetricWindow, 0, len(base))
	for _, value := range base {
		var materialRef, promotionRef string
		if err := db.QueryRowContext(ctx, `SELECT material_ref,promotion_ref FROM connector_material_metric_windows WHERE id=?`, value.ID).Scan(&materialRef, &promotionRef); err != nil {
			return nil, err
		}
		result = append(result, MaterialMetricWindow{MetricWindow: value, MaterialRef: materialRef, PromotionRef: promotionRef})
	}
	return result, nil
}

func (r MySQLRepository) readConversions(ctx context.Context, q Query) ([]ConversionRevision, error) {
	base, err := r.readMetrics(ctx, "connector_conversion_revisions", q)
	if err != nil {
		return nil, err
	}
	db, _ := r.db()
	result := make([]ConversionRevision, 0, len(base))
	for _, value := range base {
		var original string
		var revision int
		if err := db.QueryRowContext(ctx, `SELECT original_window_id,revision_number FROM connector_conversion_revisions WHERE id=?`, value.ID).Scan(&original, &revision); err != nil {
			return nil, err
		}
		result = append(result, ConversionRevision{MetricWindow: value, OriginalWindowID: original, RevisionNumber: revision})
	}
	return result, nil
}

func (r MySQLRepository) readObjects(ctx context.Context, table string, q Query) ([]ObjectSnapshot, error) {
	db, err := r.db()
	if err != nil {
		return nil, err
	}
	query := `SELECT id,source_system,source_ref,object_kind,object_ref,parent_ref,ingest_run_id,raw_snapshot_id,schema_version,payload_hash,state_json,collected_at,available_at,data_through,valid_from,valid_to,quality_status FROM ` + table + ` WHERE organization_id=? AND project_id=? AND available_at<=? AND quality_status<>'reject'`
	args := []any{q.OrganizationID, q.ProjectID, q.PredictionCutoff}
	if q.SourceRef != "" {
		query += ` AND source_ref=?`
		args = append(args, q.SourceRef)
	}
	if q.ObjectRef != "" {
		query += ` AND object_ref=?`
		args = append(args, q.ObjectRef)
	}
	query += ` ORDER BY available_at,id`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ObjectSnapshot{}
	for rows.Next() {
		var v ObjectSnapshot
		var raw []byte
		var validTo sql.NullTime
		v.OrganizationID, v.ProjectID = q.OrganizationID, q.ProjectID
		if err := rows.Scan(&v.ID, &v.SourceSystem, &v.SourceRef, &v.ObjectKind, &v.ObjectRef, &v.ParentRef, &v.IngestRunID, &v.EvidenceRef, &v.SchemaVersion, &v.PayloadHash, &raw, &v.CollectedAt, &v.AvailableAt, &v.DataThrough, &v.ValidFrom, &validTo, &v.QualityStatus); err != nil {
			return nil, err
		}
		if validTo.Valid {
			v.ValidTo = &validTo.Time
		}
		if err := json.Unmarshal(raw, &v.State); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (r MySQLRepository) readConfigurations(ctx context.Context, q Query) ([]ConfigurationSnapshot, error) {
	objects, err := r.readObjects(ctx, "connector_configuration_snapshots", q)
	if err != nil {
		return nil, err
	}
	result := make([]ConfigurationSnapshot, 0, len(objects))
	for _, object := range objects {
		result = append(result, ConfigurationSnapshot{FactHeader: object.FactHeader, ID: object.ID, ObjectRef: object.ObjectRef, Values: object.State})
	}
	return result, nil
}

func (r MySQLRepository) LatestConfiguration(ctx context.Context, organizationID, projectID, sourceRef, objectRef string, before time.Time) (ConfigurationSnapshot, bool, error) {
	values, err := r.readConfigurations(ctx, Query{OrganizationID: organizationID, ProjectID: projectID, SourceRef: sourceRef, ObjectRef: objectRef, PredictionCutoff: before})
	if err != nil {
		return ConfigurationSnapshot{}, false, err
	}
	if len(values) == 0 {
		return ConfigurationSnapshot{}, false, nil
	}
	return values[len(values)-1], true, nil
}

func (r MySQLRepository) LatestMetric(ctx context.Context, organizationID, projectID, sourceRef, objectRef string, windowStart, windowEnd time.Time, attributionWindow, definition string, before time.Time) (MetricWindow, int, bool, error) {
	values, err := r.readMetrics(ctx, "connector_metric_windows", Query{OrganizationID: organizationID, ProjectID: projectID, SourceRef: sourceRef, ObjectRef: objectRef, WindowStart: windowStart, WindowEnd: windowEnd, PredictionCutoff: before})
	if err != nil {
		return MetricWindow{}, 0, false, err
	}
	filtered := values[:0]
	for _, value := range values {
		if value.WindowStart.Equal(windowStart) && value.WindowEnd.Equal(windowEnd) && value.AttributionWindow == attributionWindow && value.MetricDefinitionVersion == definition {
			filtered = append(filtered, value)
		}
	}
	if len(filtered) == 0 {
		return MetricWindow{}, 0, false, nil
	}
	return filtered[len(filtered)-1], len(filtered), true, nil
}

func (r MySQLRepository) readMetrics(ctx context.Context, table string, q Query) ([]MetricWindow, error) {
	db, err := r.db()
	if err != nil {
		return nil, err
	}
	query := `SELECT id,source_system,source_ref,object_ref,ingest_run_id,raw_snapshot_id,schema_version,payload_hash,window_start,window_end,granularity,platform_timezone,attribution_window,metric_definition_version,currency,amount_unit,metrics_json,quality_issues_json,revision_of,collected_at,available_at,data_through,valid_from,valid_to,quality_status FROM ` + table + ` WHERE organization_id=? AND project_id=? AND available_at<=? AND quality_status<>'reject'`
	args := []any{q.OrganizationID, q.ProjectID, q.PredictionCutoff}
	if q.SourceRef != "" {
		query += ` AND source_ref=?`
		args = append(args, q.SourceRef)
	}
	if q.ObjectRef != "" {
		query += ` AND object_ref=?`
		args = append(args, q.ObjectRef)
	}
	if !q.WindowStart.IsZero() {
		query += ` AND window_end>?`
		args = append(args, q.WindowStart)
	}
	if !q.WindowEnd.IsZero() {
		query += ` AND window_start<?`
		args = append(args, q.WindowEnd)
	}
	query += ` ORDER BY available_at,id`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []MetricWindow{}
	for rows.Next() {
		var v MetricWindow
		var raw []byte
		var qualityIssues []byte
		var revision sql.NullString
		var validTo sql.NullTime
		v.OrganizationID, v.ProjectID = q.OrganizationID, q.ProjectID
		if err := rows.Scan(&v.ID, &v.SourceSystem, &v.SourceRef, &v.ObjectRef, &v.IngestRunID, &v.EvidenceRef, &v.SchemaVersion, &v.PayloadHash, &v.WindowStart, &v.WindowEnd, &v.Granularity, &v.TimeZone, &v.AttributionWindow, &v.MetricDefinitionVersion, &v.Currency, &v.AmountUnit, &raw, &qualityIssues, &revision, &v.CollectedAt, &v.AvailableAt, &v.DataThrough, &v.ValidFrom, &validTo, &v.QualityStatus); err != nil {
			return nil, err
		}
		if revision.Valid {
			v.RevisionOf = revision.String
		}
		if validTo.Valid {
			v.ValidTo = &validTo.Time
		}
		if err := json.Unmarshal(raw, &v.Metrics); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(qualityIssues, &v.QualityIssues); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
