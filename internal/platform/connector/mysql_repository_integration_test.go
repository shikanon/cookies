package connector

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func TestMySQLRepositoryAppendAndPointInTimeRead(t *testing.T) {
	dsn := os.Getenv("COOKIES_CONNECTOR_MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("COOKIES_CONNECTOR_MYSQL_TEST_DSN is not set")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	repository := MySQLRepository{DB: db}
	suffix := time.Now().UTC().Format("150405.000000000")
	org, project, run := "org_"+suffix, "project_"+suffix, "run_"+suffix
	cleanup := []string{"connector_launch_batch_calibrations", "connector_conversion_revisions", "connector_platform_diagnosis_snapshots", "connector_platform_status_events", "connector_material_metric_windows", "connector_material_bindings", "connector_metric_windows", "connector_configuration_change_events", "connector_configuration_snapshots", "connector_object_snapshots", "connector_raw_snapshots", "connector_sync_runs", "connector_ocean_engine_account_sessions", "platform_account_connections", "platform_accounts"}
	defer func() {
		for _, table := range cleanup {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM `+table+` WHERE organization_id=?`, org)
		}
	}()
	now := time.Now().UTC().Truncate(time.Microsecond)
	registered, err := repository.RegisterAccount(ctx, RegisterAccountRequest{OrganizationID: org, ProjectID: project, ExternalID: "raw-platform-account", CredentialRef: "insights-session://compat"})
	if err != nil {
		t.Fatal(err)
	}
	accountID := registered.ID
	if _, err = repository.MarkAccountVerified(ctx, org, project, accountID, now); err != nil {
		t.Fatal(err)
	}
	externalID, err := repository.ResolveExternalAccountID(ctx, org, project, accountID)
	if err != nil || externalID != "raw-platform-account" {
		t.Fatalf("external account=%s error=%v", externalID, err)
	}
	if created, err := repository.StartSync(ctx, SyncRun{ID: run, OrganizationID: org, ProjectID: project, AccountRef: "account", StartedAt: now, Attempt: 1}); err != nil || !created {
		t.Fatalf("start=%v %v", created, err)
	}
	header := FactHeader{OrganizationID: org, ProjectID: project, SourceSystem: SourceSystem, SourceRef: "ref_account", IngestRunID: run, SchemaVersion: DatasetVersion, PayloadHash: canonicalHash("raw"), CollectedAt: now, AvailableAt: now, DataThrough: now, ValidFrom: now, QualityStatus: QualityAccept, EvidenceRef: "raw_" + suffix}
	raw := RawSnapshot{Header: header, ID: header.EvidenceRef, Endpoint: "account_info", RequestHash: canonicalHash("request"), EncryptedEvidence: []byte{1, 2, 3}, KeyVersion: "test-v1"}
	if created, err := repository.AppendRaw(ctx, raw); err != nil || !created {
		t.Fatalf("raw=%v %v", created, err)
	}
	object := ObjectSnapshot{FactHeader: header, ID: "object_" + suffix, ObjectKind: "promotion", ObjectRef: "ref_promotion", State: map[string]any{"status": "active"}}
	object.PayloadHash = canonicalHash(object.State)
	if created, err := repository.AppendObject(ctx, object); err != nil || !created {
		t.Fatalf("object=%v %v", created, err)
	}
	config := ConfigurationSnapshot{FactHeader: header, ID: "config_" + suffix, ObjectRef: "ref_promotion", Values: map[string]any{"budget": 100.0}}
	config.PayloadHash = canonicalHash(config.Values)
	if created, err := repository.AppendConfiguration(ctx, config); err != nil || !created {
		t.Fatalf("config=%v %v", created, err)
	}
	after := config
	after.ID = "config_after_" + suffix
	after.Values = map[string]any{"budget": 200.0}
	after.PayloadHash = canonicalHash(after.Values)
	after.CollectedAt = now.Add(time.Microsecond)
	after.AvailableAt = after.CollectedAt
	changes := diffConfigurations(config, after)
	if created, err := repository.AppendConfiguration(ctx, after); err != nil || !created {
		t.Fatalf("config after=%v %v", created, err)
	}
	if len(changes) != 1 {
		t.Fatalf("changes=%#v", changes)
	}
	if created, err := repository.AppendChange(ctx, changes[0]); err != nil || !created {
		t.Fatalf("change=%v %v", created, err)
	}
	metric := MetricWindow{FactHeader: header, ID: "metric_" + suffix, ObjectRef: "ref_promotion", WindowStart: now.Add(-time.Hour), WindowEnd: now, Granularity: "hour", TimeZone: "UTC", AttributionWindow: "7d", MetricDefinitionVersion: "v1", Currency: "CNY", AmountUnit: "fen", Metrics: map[string]int64{"spend": 100}}
	metric.PayloadHash = canonicalHash(metric.Metrics)
	if created, err := repository.AppendMetric(ctx, metric); err != nil || !created {
		t.Fatalf("metric=%v %v", created, err)
	}
	materialMetric := MaterialMetricWindow{MetricWindow: metric, MaterialRef: "ref_material", PromotionRef: "ref_promotion"}
	materialMetric.ID = "material_metric_" + suffix
	materialMetric.PayloadHash = canonicalHash(materialMetric.ID)
	if created, err := repository.AppendMaterialMetric(ctx, materialMetric); err != nil || !created {
		t.Fatalf("material metric=%v %v", created, err)
	}
	conversionMetric := metric
	conversionMetric.ID = "conversion_" + suffix
	conversionMetric.PayloadHash = canonicalHash(conversionMetric.ID)
	conversionMetric.RevisionOf = metric.ID
	conversion := ConversionRevision{MetricWindow: conversionMetric, OriginalWindowID: metric.ID, RevisionNumber: 1}
	if created, err := repository.AppendConversionRevision(ctx, conversion); err != nil || !created {
		t.Fatalf("conversion=%v %v", created, err)
	}
	binding := MaterialBinding{FactHeader: header, ID: "binding_" + suffix, MaterialRef: "ref_material", PromotionRef: "ref_promotion"}
	binding.PayloadHash = canonicalHash(binding.ID)
	if created, err := repository.AppendBinding(ctx, binding); err != nil || !created {
		t.Fatalf("binding=%v %v", created, err)
	}
	status := PlatformStatusEvent{FactHeader: header, ID: "status_" + suffix, ObjectRef: "ref_promotion", Status: "active"}
	status.PayloadHash = canonicalHash(status.ID)
	if created, err := repository.AppendStatus(ctx, status); err != nil || !created {
		t.Fatalf("status=%v %v", created, err)
	}
	diagnosis := PlatformDiagnosisSnapshot{FactHeader: header, ID: "diagnosis_" + suffix, ObjectRef: "ref_promotion", Diagnosis: map[string]any{"level": "normal"}}
	diagnosis.PayloadHash = canonicalHash(diagnosis.ID)
	if created, err := repository.AppendDiagnosis(ctx, diagnosis); err != nil || !created {
		t.Fatalf("diagnosis=%v %v", created, err)
	}
	snapshot, err := repository.Snapshot(ctx, Query{OrganizationID: org, ProjectID: project, SourceRef: "ref_account", PredictionCutoff: now.Add(time.Second), IncludeDiagnosis: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Objects) != 1 || len(snapshot.Configurations) != 2 || len(snapshot.Changes) != 1 || len(snapshot.Metrics) != 1 || len(snapshot.MaterialMetrics) != 1 || len(snapshot.ConversionRevisions) != 1 || len(snapshot.Bindings) != 1 || len(snapshot.Statuses) != 1 || len(snapshot.Diagnoses) != 1 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	if err := repository.CompleteSync(ctx, run, "complete", "completed", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
}

func TestMySQLRepositoryListPlatformObjectsSupportsUnicodeSearch(t *testing.T) {
	dsn := os.Getenv("COOKIES_CONNECTOR_MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("COOKIES_CONNECTOR_MYSQL_TEST_DSN is not set")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	repository := MySQLRepository{DB: db}
	suffix := time.Now().UTC().Format("150405.000000000")
	organizationID := "org_unicode_search_" + suffix
	projectID := "project_unicode_search_" + suffix
	defer func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM connector_platform_object_project_grants WHERE organization_id=?`, organizationID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM connector_platform_objects WHERE organization_id=?`, organizationID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM platform_account_connections WHERE organization_id=?`, organizationID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM platform_accounts WHERE organization_id=?`, organizationID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM projects WHERE organization_id=?`, organizationID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM organizations WHERE id=?`, organizationID)
	}()

	observedAt := time.Now().UTC().Truncate(time.Microsecond)
	if _, err = db.ExecContext(ctx, `INSERT INTO organizations (id,name,status) VALUES (?,?,'active')`, organizationID, "Unicode search test"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO projects (id,organization_id,name,status) VALUES (?,?,?,'draft')`, projectID, organizationID, "Unicode search test"); err != nil {
		t.Fatal(err)
	}
	registered, err := repository.RegisterAccount(ctx, RegisterAccountRequest{OrganizationID: organizationID, ProjectID: projectID, ExternalID: "unicode-search-account", CredentialRef: "insights-session://unicode-search"})
	if err != nil {
		t.Fatal(err)
	}
	accountID := registered.ID
	_, err = repository.ReconcilePlatformObjects(ctx, organizationID, projectID, accountID, "sync_unicode_search_"+suffix, PlatformObjectMarketingProduct, observedAt, []PlatformObjectCandidate{{
		Kind: PlatformObjectMarketingProduct, PlatformObjectID: "123456789", DisplayName: "菜鸟物流产品", Metadata: map[string]any{"create_time": "2026-08-27 12:00:00"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	values, err := repository.ListPlatformObjects(ctx, PlatformObjectQuery{
		OrganizationID: organizationID, ProjectID: projectID, AccountID: accountID,
		Kind: PlatformObjectMarketingProduct, Status: "active", Search: "菜鸟",
		SortBy: "created_at", SortOrder: "desc", Limit: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].DisplayName != "菜鸟物流产品" {
		t.Fatalf("values=%#v", values)
	}
	title := strings.Repeat("原", 512)
	_, err = repository.ReconcilePlatformObjects(ctx, organizationID, projectID, accountID, "sync_native_"+suffix, PlatformObjectDouyinVideo, observedAt, []PlatformObjectCandidate{{
		Kind: PlatformObjectDouyinVideo, PlatformObjectID: "7681605279024303402", DisplayName: title,
		Metadata: map[string]any{"ies_core_user_id": "7500877386264609852", "aweme_nickname": "测试作者"},
	}})
	if err != nil {
		t.Fatalf("512-character native title: %v", err)
	}
	for _, authorID := range []string{"7500877386264609852", "999"} {
		videos, err := repository.ListPlatformObjects(ctx, PlatformObjectQuery{OrganizationID: organizationID, ProjectID: projectID, AccountID: accountID, Kind: PlatformObjectDouyinVideo, IESCoreUserID: authorID, Search: "测试作者"})
		if err != nil || (len(videos) == 1) != (authorID == "7500877386264609852") {
			t.Fatalf("author=%s videos=%v err=%v", authorID, videos, err)
		}
		if len(videos) == 1 && videos[0].DisplayName != title {
			t.Fatal("native title was truncated")
		}
	}
	_, err = repository.ReconcilePlatformObjects(ctx, organizationID, projectID, accountID, "sync_native_long_"+suffix, PlatformObjectDouyinVideo, observedAt, []PlatformObjectCandidate{{Kind: PlatformObjectDouyinVideo, PlatformObjectID: "7681605279024303402", DisplayName: title + "长"}})
	if err == nil {
		t.Fatal("513-character title must still be rejected")
	}
}
