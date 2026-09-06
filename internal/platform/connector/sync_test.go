package connector

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/integrations/oceanengine"
)

func TestSyncErrorCategoryDoesNotExposeResponseData(t *testing.T) {
	if got := SyncErrorCategory(fmt.Errorf("read metrics: %w", oceanengine.BusinessCodeError{Code: 401})); got != "business_401" {
		t.Fatalf("unexpected business category %q", got)
	}
	if got := SyncErrorCategory(oceanengine.HTTPStatusError{StatusCode: 503}); got != "http_503" {
		t.Fatalf("unexpected HTTP category %q", got)
	}
}

func TestSyncErrorStageOnlyExposesCatalogKind(t *testing.T) {
	err := platformObjectReadError{stage: "industry_category:customer_context_read", err: fmt.Errorf("private upstream detail")}
	if got := SyncErrorStage(err); got != "industry_category:customer_context_read" {
		t.Fatalf("stage=%q", got)
	}
}

type testCipher struct{}

func (testCipher) Encrypt(value []byte) ([]byte, string, error) {
	result := make([]byte, len(value))
	for i := range value {
		result[i] = value[i] ^ 0xff
	}
	return result, "test-v1", nil
}

type testFactory struct{ reader oceanengine.Reader }

func (f testFactory) Open(context.Context, SyncRequest) (oceanengine.Reader, func(), error) {
	return f.reader, func() {}, nil
}

type testReader struct {
	statRequests *[]oceanengine.StatQueryRequest
}

type metricsOnlyReader struct{ testReader }

type inventoryOnlyReader struct{ testReader }

type productImageFailureReader struct{ testReader }

func (productImageFailureReader) ProductImagesPage(context.Context, oceanengine.AssetPageRequest) (map[string]any, error) {
	return nil, fmt.Errorf("product image upstream failed")
}

func (inventoryOnlyReader) PromotionConfiguration(context.Context, string) (map[string]any, error) {
	return nil, fmt.Errorf("configuration read is not permitted")
}
func (inventoryOnlyReader) PromotionMaterials(context.Context, string, bool) (map[string]any, error) {
	return nil, fmt.Errorf("material read is not permitted")
}
func (inventoryOnlyReader) Attributes(context.Context, []string, string) (map[string]any, error) {
	return nil, fmt.Errorf("attribute read is not permitted")
}
func (inventoryOnlyReader) StatQueryPage(context.Context, oceanengine.StatQueryRequest) (map[string]any, error) {
	return nil, fmt.Errorf("metric read is not permitted")
}

func (metricsOnlyReader) AccountInfo(context.Context) (map[string]any, error) {
	return nil, fmt.Errorf("inventory read is not permitted")
}
func (metricsOnlyReader) ListPage(context.Context, oceanengine.ListRequest) (map[string]any, error) {
	return nil, fmt.Errorf("inventory read is not permitted")
}
func (metricsOnlyReader) PromotionConfiguration(context.Context, string) (map[string]any, error) {
	return nil, fmt.Errorf("inventory read is not permitted")
}
func (metricsOnlyReader) PromotionMaterials(context.Context, string, bool) (map[string]any, error) {
	return nil, fmt.Errorf("inventory read is not permitted")
}
func (metricsOnlyReader) Attributes(context.Context, []string, string) (map[string]any, error) {
	return nil, fmt.Errorf("inventory read is not permitted")
}

func (testReader) AccountInfo(context.Context) (map[string]any, error) {
	return map[string]any{"advertiser_id": "raw-account-1", "name": "demo"}, nil
}
func (testReader) ImageMaterialsPage(context.Context, oceanengine.AssetPageRequest) (map[string]any, error) {
	return map[string]any{"data": map[string]any{"images": []any{map[string]any{"material_id": "1001", "file_name": "image"}}, "pagination": map[string]any{"total_page": 1.0}}}, nil
}
func (testReader) ProductImagesPage(context.Context, oceanengine.AssetPageRequest) (map[string]any, error) {
	return map[string]any{"data": map[string]any{"images": []any{map[string]any{"material_id": "1501", "file_name": "product-image", "image_mode": 649502.0}}, "pagination": map[string]any{"total_page": 1.0}}}, nil
}
func (testReader) VideoMaterialsPage(context.Context, oceanengine.AssetPageRequest) (map[string]any, error) {
	return map[string]any{"data": map[string]any{"videos": []any{map[string]any{"material_id": "2001", "video_name": "video"}}, "pagination": map[string]any{"total_page": 1.0}}}, nil
}
func (testReader) AwemePhotoMaterialsPage(context.Context, oceanengine.AssetPageRequest) (map[string]any, error) {
	return map[string]any{"data": map[string]any{"list": []any{map[string]any{"material_id": "2501", "file_name": "photo"}}, "pagination": map[string]any{"total_page": 1.0}}}, nil
}
func (testReader) MarketingProductsPage(context.Context, oceanengine.AssetPageRequest) (map[string]any, error) {
	return map[string]any{"data": map[string]any{"list": []any{map[string]any{"unique_product_id": "2601", "product_id": "1601", "name": "product"}}, "pagination": map[string]any{"total_page": 1.0}}}, nil
}
func (testReader) OrangeLandingPagesPage(context.Context, oceanengine.AssetPageRequest) (map[string]any, error) {
	return map[string]any{"data": map[string]any{"data": []any{map[string]any{"site_id": "3001", "name": "landing"}}, "pagination": map[string]any{"page": 1.0, "size": 30.0, "total": 1.0}}}, nil
}
func (testReader) FilteredOrangeLandingPagesPage(_ context.Context, _ oceanengine.AssetPageRequest, filter oceanengine.OrangeLandingPageFilter) (map[string]any, error) {
	items := []any{}
	if filter.ExternalAction == 100 {
		items = append(items, map[string]any{"site_id": "3001", "name": "landing"})
	}
	return map[string]any{"data": map[string]any{"data": items, "pagination": map[string]any{"page": 1.0, "size": 30.0, "total": float64(len(items))}}}, nil
}
func (testReader) OptimizationTargets(_ context.Context, assetType int, _ bool) (map[string]any, error) {
	goal := map[string]any{"optimization_name": "conversion", "external_action": 20.0}
	if assetType == 3 {
		goal["asset_info"] = []any{map[string]any{"asset_id": 3101.0, "asset_name": "event"}}
	}
	return map[string]any{"data": map[string]any{"goals": []any{goal}}}, nil
}
func (testReader) BrandIndustries(context.Context) (map[string]any, error) {
	return map[string]any{"data": []any{map[string]any{"id": 3201.0, "label": "category"}}}, nil
}
func (testReader) Brands(context.Context) (map[string]any, error) {
	return map[string]any{"data": map[string]any{"data": []any{map[string]any{"cdp_brand_id": 3301.0, "brand_name": "brand"}}}}, nil
}
func (testReader) AuthorizedIdentitiesPage(context.Context, oceanengine.AssetPageRequest) (map[string]any, error) {
	return map[string]any{"data": []any{map[string]any{"ies_core_id": 3401.0, "ies_user_name": "identity"}}, "extra": map[string]any{"hasMore": false}}, nil
}
func (testReader) ListPage(_ context.Context, r oceanengine.ListRequest) (map[string]any, error) {
	if r.Page > 1 {
		return map[string]any{"data": map[string]any{"ads": []any{}, "pagination": map[string]any{"total_page": 1.0}}}, nil
	}
	return map[string]any{"data": map[string]any{"ads": []any{map[string]any{"promotion_id": "raw-promotion-1", "project_id": "raw-project-1", "promotion_create_time": "2026-08-01 10:30:00", "promotion_object": map[string]any{"product_id": "raw-product-1"}, "status": "active"}}, "pagination": map[string]any{"total_page": 1.0}}}, nil
}
func (testReader) PromotionConfiguration(context.Context, string) (map[string]any, error) {
	return map[string]any{"budget": 100.0, "landing_url": "https://secret.test/?token=x"}, nil
}
func (testReader) PromotionMaterials(context.Context, string, bool) (map[string]any, error) {
	return map[string]any{"data": map[string]any{"material_ids": []any{"raw-material-1"}}}, nil
}
func (testReader) Attributes(context.Context, []string, string) (map[string]any, error) {
	return map[string]any{"diagnosis": "stable"}, nil
}
func (r testReader) StatQueryPage(_ context.Context, request oceanengine.StatQueryRequest) (map[string]any, error) {
	if r.statRequests != nil {
		*r.statRequests = append(*r.statRequests, request)
	}
	dimensions := map[string]any{"cdp_promotion_id": map[string]any{"Value": "raw-promotion-1"}, "stat_time_day": map[string]any{"ValueStr": "2026-08-19"}}
	if request.DatasetKey == "ad_material_data" {
		dimensions = map[string]any{"material_id": map[string]any{"Value": "report-material-1"}, "stat_time_day": map[string]any{"ValueStr": "2026-08-19"}, "image_mode": map[string]any{"Value": "video"}}
	}
	metrics := map[string]any{"stat_cost": map[string]any{"Value": 100.0}, "show_cnt": map[string]any{"Value": 1000.0}, "click_cnt": map[string]any{"Value": 10.0}, "convert_cnt": map[string]any{"Value": 2.0}}
	return map[string]any{"data": map[string]any{"StatsData": map[string]any{"TotalCount": "1", "Rows": []any{map[string]any{"Dimensions": dimensions, "Metrics": metrics, "Rows": nil}}}}}, nil
}

func TestMetricTotalCountAcceptsPlatformStringValue(t *testing.T) {
	payload := map[string]any{"data": map[string]any{"StatsData": map[string]any{"TotalCount": "12"}}}
	if got := metricTotalCount(payload); got != 12 {
		t.Fatalf("metric total count = %d, want 12", got)
	}
}

type testWriter struct {
	started         bool
	completed       string
	completedCursor string
	raw             []RawSnapshot
	objects         []ObjectSnapshot
	configs         []ConfigurationSnapshot
	metrics         []MetricWindow
	materialMetrics []MaterialMetricWindow
	bindings        []MaterialBinding
	statuses        []PlatformStatusEvent
	diagnoses       []PlatformDiagnosisSnapshot
	platformObjects map[PlatformObjectKind][]PlatformObjectCandidate
}

func (w *testWriter) ReconcilePlatformObjects(_ context.Context, _, _, _, _ string, kind PlatformObjectKind, _ time.Time, candidates []PlatformObjectCandidate) (PlatformObjectSyncStats, error) {
	if w.platformObjects == nil {
		w.platformObjects = map[PlatformObjectKind][]PlatformObjectCandidate{}
	}
	w.platformObjects[kind] = append([]PlatformObjectCandidate(nil), candidates...)
	return PlatformObjectSyncStats{Created: len(candidates)}, nil
}
func (w *testWriter) ListPlatformObjects(context.Context, PlatformObjectQuery) ([]PlatformObject, error) {
	return nil, nil
}

func (w *testWriter) StartSync(context.Context, SyncRun) (bool, error) {
	if w.started {
		return false, nil
	}
	w.started = true
	return true, nil
}
func (w *testWriter) UpdateSyncCursor(_ context.Context, _, cursor string) error {
	w.completed = cursor
	return nil
}
func (w *testWriter) CompleteSync(_ context.Context, _, cursor, status string, _ time.Time) error {
	w.completedCursor = cursor
	w.completed = status
	return nil
}
func (w *testWriter) AppendRaw(_ context.Context, v RawSnapshot) (bool, error) {
	w.raw = append(w.raw, v)
	return true, nil
}
func (w *testWriter) AppendObject(_ context.Context, v ObjectSnapshot) (bool, error) {
	w.objects = append(w.objects, v)
	return true, nil
}
func (w *testWriter) AppendConfiguration(_ context.Context, v ConfigurationSnapshot) (bool, error) {
	w.configs = append(w.configs, v)
	return true, nil
}
func (w *testWriter) AppendChange(context.Context, ConfigurationChangeEvent) (bool, error) {
	return true, nil
}
func (w *testWriter) AppendMetric(_ context.Context, v MetricWindow) (bool, error) {
	w.metrics = append(w.metrics, v)
	return true, nil
}
func (w *testWriter) AppendMaterialMetric(_ context.Context, value MaterialMetricWindow) (bool, error) {
	w.materialMetrics = append(w.materialMetrics, value)
	return true, nil
}
func (w *testWriter) AppendConversionRevision(context.Context, ConversionRevision) (bool, error) {
	return true, nil
}
func (w *testWriter) AppendBinding(_ context.Context, v MaterialBinding) (bool, error) {
	w.bindings = append(w.bindings, v)
	return true, nil
}
func (w *testWriter) AppendStatus(_ context.Context, v PlatformStatusEvent) (bool, error) {
	w.statuses = append(w.statuses, v)
	return true, nil
}
func (w *testWriter) AppendDiagnosis(_ context.Context, v PlatformDiagnosisSnapshot) (bool, error) {
	w.diagnoses = append(w.diagnoses, v)
	return true, nil
}
func (w *testWriter) LatestConfiguration(context.Context, string, string, string, string, time.Time) (ConfigurationSnapshot, bool, error) {
	return ConfigurationSnapshot{}, false, nil
}
func (w *testWriter) LatestMetric(context.Context, string, string, string, string, time.Time, time.Time, string, string, time.Time) (MetricWindow, int, bool, error) {
	return MetricWindow{}, 0, false, nil
}

func TestSynchronizerBuildsEncryptedImmutableLedgerSlice(t *testing.T) {
	writer := &testWriter{}
	now := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	statRequests := []oceanengine.StatQueryRequest{}
	syncer := Synchronizer{Writer: writer, Readers: testFactory{reader: testReader{statRequests: &statRequests}}, Cipher: testCipher{}, Now: func() time.Time { return now }}
	result, err := syncer.Sync(context.Background(), SyncRequest{OrganizationID: "org_1", ProjectID: "project_1", AccountRef: "raw-account-1", IdempotencyKey: "request-1", WindowStart: now.AddDate(0, 0, -1), WindowEnd: now, TimeZone: "Asia/Shanghai", Currency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ObjectCount != 5 || result.MetricCount != 2 || writer.completed != "completed" {
		t.Fatalf("result=%#v completed=%s", result, writer.completed)
	}
	if len(writer.raw) != 20 || len(writer.configs) != 1 || len(writer.bindings) != 1 || len(writer.diagnoses) != 1 || len(writer.statuses) != 1 {
		t.Fatalf("raw=%d config=%d binding=%d diagnosis=%d status=%d", len(writer.raw), len(writer.configs), len(writer.bindings), len(writer.diagnoses), len(writer.statuses))
	}
	if len(writer.platformObjects) != 11 || result.PlatformObjects[PlatformObjectProductImage].Created != 1 || result.PlatformObjects[PlatformObjectVideoMaterial].Created != 1 || result.PlatformObjects[PlatformObjectAwemePhotoMaterial].Created != 1 || result.PlatformObjects[PlatformObjectMarketingProduct].Created != 1 || result.PlatformObjects[PlatformObjectConversionAsset].Created != 1 || result.PlatformObjects[PlatformObjectAuthorizedIdentity].Created != 1 {
		t.Fatalf("platform objects=%#v result=%#v", writer.platformObjects, result.PlatformObjects)
	}
	landing := writer.platformObjects[PlatformObjectOrangeLandingPage][0]
	if eligible, ok := landing.Metadata["multi_conversion_eligible"].(bool); !ok || !eligible {
		t.Fatalf("legacy multi-conversion landing eligibility = %#v", landing.Metadata)
	}
	actions, ok := landing.Metadata["multi_lead_external_actions"].([]string)
	if !ok || len(actions) != 1 || actions[0] != "100" {
		t.Fatalf("multi-lead landing actions = %#v", landing.Metadata)
	}
	if writer.objects[0].ObjectRef == "raw-account-1" || writer.objects[1].ObjectRef == "raw-promotion-1" || writer.bindings[0].MaterialRef == "raw-material-1" {
		t.Fatal("raw platform identity leaked")
	}
	if writer.bindings[0].ValidTo != nil || !writer.bindings[0].ValidFrom.Equal(now) {
		t.Fatalf("observed active binding has an invalid interval: %#v", writer.bindings[0])
	}
	if _, ok := writer.objects[0].State["advertiser_id"]; ok {
		t.Fatal("raw account identity leaked in canonical state")
	}
	if writer.objects[1].State["product_ref"] == "raw-product-1" || writer.objects[1].State["product_ref"] == "" {
		t.Fatal("promotion did not retain an opaque product cohort reference")
	}
	if _, ok := writer.configs[0].Values["landing_url"]; ok {
		t.Fatal("tracking URL leaked into canonical configuration")
	}
	if writer.diagnoses[0].EligibleAsPrelaunchFeature {
		t.Fatal("diagnosis became prelaunch eligible")
	}
	if writer.metrics[0].QualityStatus != QualityQuarantine || len(writer.metrics[0].QualityIssues) == 0 {
		t.Fatalf("metric quality=%s issues=%#v", writer.metrics[0].QualityStatus, writer.metrics[0].QualityIssues)
	}
	if writer.metrics[0].Metrics["spend"] != 10000 || writer.metrics[0].AmountUnit != "fen" {
		t.Fatalf("spend was not normalized from yuan to fen: %#v", writer.metrics[0])
	}
	wantWindowStart := time.Date(2026, 8, 18, 16, 0, 0, 0, time.UTC)
	if !writer.metrics[0].WindowStart.Equal(wantWindowStart) || !writer.metrics[0].DataThrough.Equal(writer.metrics[0].WindowEnd) {
		t.Fatalf("platform day was not normalized to UTC: %#v", writer.metrics[0])
	}
	if len(writer.materialMetrics) != 1 || writer.materialMetrics[0].PromotionRef != "" || !hasQualityIssue(writer.materialMetrics[0].QualityIssues, "material_binding_unresolved") {
		t.Fatalf("unbound material metric was not quarantined: %#v", writer.materialMetrics)
	}
	if writer.configs[0].Values["currency"] != "CNY" {
		t.Fatalf("configuration currency was not retained: %#v", writer.configs[0].Values)
	}
	if len(statRequests) != 2 || statRequests[0].DatasetKey != "basic_ad_data" || statRequests[0].Host != "" || statRequests[0].StartTime != "2026-08-19 00:00:00" || statRequests[0].EndTime != "2026-08-19 23:59:59" {
		t.Fatalf("promotion metric request=%#v", statRequests)
	}
	if statRequests[1].DatasetKey != "ad_material_data" || statRequests[1].Dimensions[1] != "material_id" {
		t.Fatalf("material metric request=%#v", statRequests[1])
	}
	if string(writer.raw[0].EncryptedEvidence) == "" || writer.raw[0].KeyVersion != "test-v1" {
		t.Fatal("raw evidence was not encrypted")
	}
}

func TestSynchronizerPersistsPlatformObjectFailureStage(t *testing.T) {
	writer := &testWriter{}
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	syncer := Synchronizer{Writer: writer, Readers: testFactory{reader: productImageFailureReader{}}, Cipher: testCipher{}, Now: func() time.Time { return now }}
	_, err := syncer.Sync(context.Background(), SyncRequest{
		OrganizationID: "org_1", ProjectID: "project_1", AccountRef: "account_1", IdempotencyKey: "product-image-failure",
		WindowStart: now.AddDate(0, 0, -1), WindowEnd: now, TimeZone: "Asia/Shanghai", Currency: "CNY", Mode: SyncModeInventoryOnly,
	})
	if err == nil {
		t.Fatal("expected product-image read failure")
	}
	if writer.completed != "failed" || writer.completedCursor != string(PlatformObjectProductImage) {
		t.Fatalf("status=%q cursor=%q", writer.completed, writer.completedCursor)
	}
}

func TestSynchronizerReplaysIdempotencyKeyWithoutRemoteRead(t *testing.T) {
	writer := &testWriter{started: true}
	syncer := Synchronizer{Writer: writer, Readers: testFactory{reader: testReader{}}, Cipher: testCipher{}}
	result, err := syncer.Sync(context.Background(), SyncRequest{OrganizationID: "org_1", ProjectID: "project_1", AccountRef: "account", IdempotencyKey: "same", WindowStart: baseTime.Add(-time.Hour), WindowEnd: baseTime})
	if err != nil || !result.Replayed || len(writer.raw) != 0 {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}

func TestSynchronizerMetricsOnlySkipsInventory(t *testing.T) {
	writer := &testWriter{}
	now := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	statRequests := []oceanengine.StatQueryRequest{}
	syncer := Synchronizer{Writer: writer, Readers: testFactory{reader: metricsOnlyReader{testReader{statRequests: &statRequests}}}, Cipher: testCipher{}, Now: func() time.Time { return now }}
	result, err := syncer.Sync(context.Background(), SyncRequest{OrganizationID: "org_1", AccountRef: "account", IdempotencyKey: "metrics", WindowStart: now.AddDate(0, 0, -14), WindowEnd: now, Mode: SyncModeMetricsOnly})
	if err != nil {
		t.Fatal(err)
	}
	if result.ObjectCount != 0 || result.MetricCount == 0 || len(writer.objects) != 0 || len(writer.configs) != 0 {
		t.Fatalf("metrics-only result=%#v objects=%d configs=%d", result, len(writer.objects), len(writer.configs))
	}
	if len(statRequests) != 15 || statRequests[0].StartTime != statRequests[0].EndTime[:10]+" 00:00:00" || statRequests[0].EndTime[11:] != "23:59:59" {
		t.Fatalf("metrics-only requests do not use 14 stable daily windows: %#v", statRequests)
	}
}

func TestSynchronizerInventoryOnlyStopsAfterObjectIndex(t *testing.T) {
	writer := &testWriter{}
	now := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	syncer := Synchronizer{Writer: writer, Readers: testFactory{reader: inventoryOnlyReader{testReader{}}}, Cipher: testCipher{}, Now: func() time.Time { return now }}
	result, err := syncer.Sync(context.Background(), SyncRequest{OrganizationID: "org_1", AccountRef: "account", IdempotencyKey: "inventory", WindowStart: now.AddDate(0, 0, -180), WindowEnd: now, Mode: SyncModeInventoryOnly})
	if err != nil {
		t.Fatal(err)
	}
	if result.ObjectCount != 4 || result.MetricCount != 0 || writer.completed != "completed" {
		t.Fatalf("inventory-only result=%#v completed=%s", result, writer.completed)
	}
	if len(writer.raw) != 2 || len(writer.configs) != 0 || len(writer.metrics) != 0 || len(writer.materialMetrics) != 0 || len(writer.bindings) != 0 || len(writer.diagnoses) != 0 {
		t.Fatalf("raw=%d config=%d metrics=%d material_metrics=%d bindings=%d diagnoses=%d", len(writer.raw), len(writer.configs), len(writer.metrics), len(writer.materialMetrics), len(writer.bindings), len(writer.diagnoses))
	}
	if got := writer.objects[1].State["promotion_create_time"]; got != "2026-08-01 10:30:00" {
		t.Fatalf("promotion create time=%v", got)
	}
}

func TestSyncRunIDIncludesMode(t *testing.T) {
	request := SyncRequest{OrganizationID: "org_1", AccountRef: "account", IdempotencyKey: "same"}
	full := SyncRunID(request)
	request.Mode = SyncModeMetricsOnly
	if full == SyncRunID(request) {
		t.Fatal("sync mode did not change the run ID")
	}
}

func TestSynchronizerRejectsUnknownMode(t *testing.T) {
	syncer := Synchronizer{Writer: &testWriter{}, Readers: testFactory{reader: testReader{}}, Cipher: testCipher{}}
	_, err := syncer.Sync(context.Background(), SyncRequest{OrganizationID: "org_1", AccountRef: "account", IdempotencyKey: "bad", WindowStart: baseTime.Add(-time.Hour), WindowEnd: baseTime, Mode: "unknown"})
	if err != ErrInvalidFact {
		t.Fatalf("error=%v", err)
	}
}
