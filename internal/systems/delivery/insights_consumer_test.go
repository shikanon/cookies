package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestMockInsightsReaderFixturesAreVersionedAndProjectScoped(t *testing.T) {
	reader := MockInsightsReader{}
	query := InsightsQuery{OrganizationID: "org_a", ProjectID: "project_a", Platform: "ocean_engine", Fixture: InsightsFixtureUsable, WindowStart: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), WindowEnd: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), Granularity: "day"}
	first, err := reader.Read(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	second, err := reader.Read(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same fixture/window was not stable:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if !first.Usable() || first.Source != InsightsSourceMock || first.FixtureVersion == "" || first.SchemaVersion != InsightsSchemaVersion {
		t.Fatalf("usable fixture lost its provenance or quality: %#v", first)
	}
	if len(first.Objects) != 4 || len(first.Metrics) != 4 {
		t.Fatalf("expected account/project/promotion/material and four base metrics, got %#v", first)
	}
	for _, object := range first.Objects {
		if object.ID == "" || object.Platform != "ocean_engine" || (object.Kind != InsightsObjectAccount && object.ParentID == "") {
			t.Fatalf("object mapping is incomplete: %#v", object)
		}
	}
	for _, fact := range first.Metrics {
		if fact.OrganizationID != "org_a" || fact.ProjectID != "project_a" || fact.Source != InsightsSourceMock || fact.Quality != InsightsQualityUsable || len(fact.EvidenceRefs) == 0 {
			t.Fatalf("metric provenance is incomplete: %#v", fact)
		}
	}

	other, err := reader.Read(context.Background(), InsightsQuery{OrganizationID: "org_a", ProjectID: "project_b", Platform: "ocean_engine", Fixture: InsightsFixtureUsable})
	if err != nil {
		t.Fatal(err)
	}
	if first.Objects[0].ID == other.Objects[0].ID || first.Metrics[0].ObjectID == other.Metrics[0].ObjectID {
		t.Fatalf("project-scoped identities leaked across projects: %q vs %q", first.Objects[0].ID, other.Objects[0].ID)
	}
	for _, object := range other.Objects {
		if object.ID == first.Objects[0].ID {
			t.Fatalf("project b can observe project a object identity: %#v", object)
		}
	}
}

func TestUsableRejectsMalformedFactsAndWindows(t *testing.T) {
	base, err := (MockInsightsReader{}).Read(context.Background(), InsightsQuery{OrganizationID: "org_a", ProjectID: "project_a", Fixture: InsightsFixtureUsable})
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*DeliveryInsightsSnapshot){
		"orphan object":          func(value *DeliveryInsightsSnapshot) { value.Metrics[0].ObjectID = "project_a/orphan" },
		"organization mismatch":  func(value *DeliveryInsightsSnapshot) { value.Metrics[0].OrganizationID = "org_other" },
		"project mismatch":       func(value *DeliveryInsightsSnapshot) { value.Metrics[0].ProjectID = "project_other" },
		"platform mismatch":      func(value *DeliveryInsightsSnapshot) { value.Metrics[0].Platform = "other_platform" },
		"split base metrics":     func(value *DeliveryInsightsSnapshot) { value.Metrics[1].ObjectID = "project_a/other" },
		"invalid window":         func(value *DeliveryInsightsSnapshot) { value.Metrics[0].WindowEnd = value.Metrics[0].WindowStart },
		"missing observed time":  func(value *DeliveryInsightsSnapshot) { value.Metrics[0].ObservedAt = time.Time{} },
		"schema mismatch":        func(value *DeliveryInsightsSnapshot) { value.Metrics[0].SchemaVersion = "delivery-insights/v0" },
		"definition mismatch":    func(value *DeliveryInsightsSnapshot) { value.Metrics[0].DefinitionVersion = "delivery-base-metrics/v0" },
		"invalid spend unit":     func(value *DeliveryInsightsSnapshot) { value.Metrics[0].Unit = "count" },
		"invalid spend currency": func(value *DeliveryInsightsSnapshot) { value.Metrics[0].Currency = "USD" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			value := cloneInsightsSnapshot(base)
			mutate(&value)
			if value.Usable() {
				t.Fatalf("malformed usable snapshot was accepted: %#v", value)
			}
		})
	}
}

func TestInsightsFixturesStayAlignedWithEmbeddedJSONSchema(t *testing.T) {
	var schema map[string]any
	data, err := insightsFixtureFS.ReadFile("fixtures/insights-consumer-fixture-v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	required := stringList(schema["required"])
	properties := objectMap(schema["properties"])
	objectSchema := objectMap(objectMap(properties["objects"])["items"])
	metricSchema := objectMap(objectMap(properties["metrics"])["items"])
	objectProperties := objectMap(objectSchema["properties"])
	metricProperties := objectMap(metricSchema["properties"])
	objectRequired := stringList(objectSchema["required"])
	metricRequired := stringList(metricSchema["required"])
	qualityEnum := stringSet(objectMap(properties["quality"])["enum"])
	metricEnum := stringSet(objectMap(metricProperties["metric"])["enum"])
	for _, fixture := range []string{"usable", "empty", "stale", "incomplete", "schema-mismatch", "unavailable"} {
		t.Run(fixture, func(t *testing.T) {
			fixtureData, readErr := insightsFixtureFS.ReadFile("fixtures/insights-" + fixture + ".json")
			if readErr != nil {
				t.Fatal(readErr)
			}
			var value map[string]any
			if err := json.Unmarshal(fixtureData, &value); err != nil {
				t.Fatal(err)
			}
			assertRequiredKeys(t, value, required, "fixture")
			assertAllowedKeys(t, value, properties, "fixture")
			quality, _ := value["quality"].(string)
			if !qualityEnum[quality] {
				t.Fatalf("fixture quality %q is not declared by schema", quality)
			}
			if value["source"] != "mock" {
				t.Fatalf("fixture source must remain explicit mock, got %#v", value["source"])
			}
			for index, raw := range value["objects"].([]any) {
				object := raw.(map[string]any)
				assertRequiredKeys(t, object, objectRequired, fmt.Sprintf("object[%d]", index))
				assertAllowedKeys(t, object, objectProperties, fmt.Sprintf("object[%d]", index))
			}
			for index, raw := range value["metrics"].([]any) {
				metric := raw.(map[string]any)
				assertRequiredKeys(t, metric, metricRequired, fmt.Sprintf("metric[%d]", index))
				assertAllowedKeys(t, metric, metricProperties, fmt.Sprintf("metric[%d]", index))
				name, _ := metric["metric"].(string)
				if !metricEnum[name] {
					t.Fatalf("metric[%d] name %q is not declared by schema", index, name)
				}
			}
		})
	}
}

func cloneInsightsSnapshot(value DeliveryInsightsSnapshot) DeliveryInsightsSnapshot {
	value.Objects = append([]DeliveryInsightsObject(nil), value.Objects...)
	value.Metrics = append([]DeliveryMetricFact(nil), value.Metrics...)
	value.EvidenceRefs = append([]string(nil), value.EvidenceRefs...)
	return value
}

func objectMap(value any) map[string]any {
	result, ok := value.(map[string]any)
	if !ok {
		panic(fmt.Sprintf("expected object, got %T", value))
	}
	return result
}

func stringList(value any) []string {
	items, ok := value.([]any)
	if !ok {
		panic(fmt.Sprintf("expected string list, got %T", value))
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.(string))
	}
	return result
}

func stringSet(value any) map[string]bool {
	result := map[string]bool{}
	for _, item := range stringList(value) {
		result[item] = true
	}
	return result
}

func assertRequiredKeys(t *testing.T, value map[string]any, required []string, label string) {
	t.Helper()
	for _, key := range required {
		if _, ok := value[key]; !ok {
			t.Fatalf("%s is missing schema-required key %q", label, key)
		}
	}
}

func assertAllowedKeys(t *testing.T, value map[string]any, properties map[string]any, label string) {
	t.Helper()
	for key := range value {
		if _, ok := properties[key]; !ok {
			t.Fatalf("%s contains schema-unknown key %q", label, key)
		}
	}
}

func TestMockInsightsReaderQualityFixturesGateConclusions(t *testing.T) {
	reader := MockInsightsReader{}
	fixtures := map[string]InsightsQualityStatus{
		InsightsFixtureEmpty:          InsightsQualityEmpty,
		InsightsFixtureStale:          InsightsQualityStale,
		InsightsFixtureIncomplete:     InsightsQualityIncomplete,
		InsightsFixtureSchemaMismatch: InsightsQualitySchemaMismatch,
		InsightsFixtureUnavailable:    InsightsQualityUnavailable,
	}
	for fixture, expected := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			snapshot, err := reader.Read(context.Background(), InsightsQuery{OrganizationID: "org_a", ProjectID: "project_a", Fixture: fixture})
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Quality != expected || snapshot.Usable() || snapshot.QualityReason == "" || len(snapshot.EvidenceRefs) == 0 {
				t.Fatalf("quality fixture was not preserved: %#v", snapshot)
			}
		})
	}
}

func TestDeriveRatioUsesVersionedFormulaAndUnavailableZeroDenominator(t *testing.T) {
	numerator := DeliveryMetricFact{OrganizationID: "org_a", ProjectID: "project_a", ObjectKind: InsightsObjectProject, ObjectID: "project_a/project-1", Metric: InsightsMetricClicks, WindowStart: time.Unix(1, 0), WindowEnd: time.Unix(2, 0), Quality: InsightsQualityUsable, EvidenceRefs: []string{"e/clicks"}, Value: 25}
	denominator := numerator
	denominator.Metric = InsightsMetricImpressions
	denominator.Value = 100
	result := DeriveRatio("ctr", numerator, denominator)
	if result.Status != InsightsQualityUsable || result.Value == nil || *result.Value != 0.25 || result.FormulaVersion != InsightsDerivedFormulaV1 {
		t.Fatalf("unexpected derived metric: %#v", result)
	}
	denominator.Value = 0
	result = DeriveRatio("ctr", numerator, denominator)
	if result.Status != InsightsQualityUnavailable || result.Value != nil || result.Reason != "denominator is zero" {
		t.Fatalf("zero denominator must be unavailable: %#v", result)
	}
	numerator.ObjectKind, denominator.ObjectKind = InsightsObjectUnit, InsightsObjectUnit
	denominator.Value = 100
	result = DeriveRatio("ctr", numerator, denominator)
	if result.Status != InsightsQualityUnavailable || result.Reason == "" {
		t.Fatalf("derived metrics must stay at the explicit project level: %#v", result)
	}
}

func TestReplayInsightsReaderPreservesFactsAndRejectsCrossProjectReads(t *testing.T) {
	snapshot, err := (MockInsightsReader{}).Read(context.Background(), InsightsQuery{OrganizationID: "org_a", ProjectID: "project_a", Fixture: InsightsFixtureUsable})
	if err != nil {
		t.Fatal(err)
	}
	reader := NewReplayInsightsReader(snapshot)
	first, err := reader.Read(context.Background(), InsightsQuery{OrganizationID: "org_a", ProjectID: "project_a"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := reader.Read(context.Background(), InsightsQuery{OrganizationID: "org_a", ProjectID: "project_a"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.Source != InsightsSourceReplay || first.Metrics[0].Source != InsightsSourceReplay {
		t.Fatalf("replay was not stable or explicitly marked: %#v", first)
	}
	if _, err := reader.Read(context.Background(), InsightsQuery{OrganizationID: "org_a", ProjectID: "project_b"}); !errors.Is(err, ErrInsightsScope) {
		t.Fatalf("cross-project replay read should be rejected, got %v", err)
	}
}

func TestSimulationInsightsReaderSelectsRequestedExecutionBeforeChoosingSeed(t *testing.T) {
	service, actor := newTestService()
	repository := service.Repository.(*memoryRepository)
	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	for _, execution := range []struct {
		id    string
		spend int64
	}{
		{id: "execution_old", spend: 100},
		{id: "execution_new", spend: 900},
	} {
		for window := 0; window < 2; window++ {
			start := base.Add(time.Duration(window) * 24 * time.Hour)
			repository.metrics = append(repository.metrics, DeliveryMetricSnapshot{
				ID:             execution.id + fmt.Sprintf("_window_%d", window),
				OrganizationID: actor.OrganizationID,
				ProjectID:      "project_a",
				ExecutionID:    execution.id,
				PlanID:         "plan_a",
				FixtureVersion: "execution-regression/v1",
				WindowSequence: window,
				WindowStart:    start,
				WindowEnd:      start.Add(24 * time.Hour),
				DataThrough:    start.Add(24 * time.Hour),
				RawMetrics: RawMetrics{
					SpendCents:  execution.spend + int64(window),
					Impressions: 1000 + int64(window),
					Clicks:      100 + int64(window),
					Conversions: 10 + int64(window),
				},
				CreatedAt: start.Add(25 * time.Hour),
			})
		}
	}

	snapshot, err := (SimulationInsightsReader{Repository: repository}).Read(context.Background(), InsightsQuery{
		OrganizationID: actor.OrganizationID,
		ProjectID:      "project_a",
		ExecutionID:    "execution_old",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Usable() {
		t.Fatalf("requested execution should produce usable facts: %#v", snapshot)
	}
	windows := snapshot.metricWindows("execution_old")
	if len(windows) != 2 {
		t.Fatalf("expected two windows for the requested execution, got %#v", windows)
	}
	for _, fact := range snapshot.Metrics {
		if fact.ExecutionID != "execution_old" {
			t.Fatalf("snapshot leaked another execution: %#v", fact)
		}
	}
	if windows[0].Values[InsightsMetricSpend] != 100 || windows[1].Values[InsightsMetricSpend] != 101 {
		t.Fatalf("selected latest execution instead of requested earlier execution: %#v", windows)
	}
}
