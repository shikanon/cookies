package delivery

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

//go:embed fixtures/insights-*.json
var insightsFixtureFS embed.FS

const (
	InsightsSchemaVersion         = "delivery-insights/v1"
	InsightsMetricDefinitionV1    = "delivery-base-metrics/v1"
	InsightsDerivedFormulaV1      = "delivery-derived-metrics/v1"
	InsightsFixtureUsable         = "usable"
	InsightsFixtureEmpty          = "empty"
	InsightsFixtureStale          = "stale"
	InsightsFixtureIncomplete     = "incomplete"
	InsightsFixtureSchemaMismatch = "schema_mismatch"
	InsightsFixtureUnavailable    = "unavailable"
)

var (
	ErrInsightsScope   = errors.New("delivery insights consumer scope mismatch")
	ErrInsightsFixture = errors.New("delivery insights fixture is invalid")
)

type InsightsSource string

const (
	InsightsSourceMock      InsightsSource = "mock"
	InsightsSourceReplay    InsightsSource = "replay"
	InsightsSourceConnector InsightsSource = "connector"
)

type InsightsQualityStatus string

const (
	InsightsQualityUsable         InsightsQualityStatus = "usable"
	InsightsQualityEmpty          InsightsQualityStatus = "empty"
	InsightsQualityStale          InsightsQualityStatus = "stale"
	InsightsQualityIncomplete     InsightsQualityStatus = "incomplete"
	InsightsQualitySchemaMismatch InsightsQualityStatus = "schema_mismatch"
	InsightsQualityUnavailable    InsightsQualityStatus = "unavailable"
)

type InsightsFreshnessStatus string

const (
	InsightsFreshnessFresh   InsightsFreshnessStatus = "fresh"
	InsightsFreshnessUnknown InsightsFreshnessStatus = "unknown"
)

type InsightsObjectKind string

const (
	InsightsObjectAccount   InsightsObjectKind = "account"
	InsightsObjectProject   InsightsObjectKind = "project"
	InsightsObjectUnit      InsightsObjectKind = "unit"
	InsightsObjectPromotion InsightsObjectKind = "promotion"
	InsightsObjectMaterial  InsightsObjectKind = "material"
)

type InsightsMetricName string

const (
	InsightsMetricSpend           InsightsMetricName = "spend"
	InsightsMetricImpressions     InsightsMetricName = "impressions"
	InsightsMetricClicks          InsightsMetricName = "clicks"
	InsightsMetricConversions     InsightsMetricName = "conversions"
	InsightsMetricDeepConversions InsightsMetricName = "deep_conversions"
)

// InsightsQuery is the only input a Delivery consumer needs. It intentionally
// contains no connector table, credential, collector, or implementation type.
type InsightsQuery struct {
	OrganizationID contract.OrganizationID `json:"organization_id"`
	ProjectID      contract.ProjectID      `json:"project_id"`
	ExecutionID    string                  `json:"execution_id,omitempty"`
	Platform       string                  `json:"platform"`
	Fixture        string                  `json:"fixture"`
	WindowStart    time.Time               `json:"window_start"`
	WindowEnd      time.Time               `json:"window_end"`
	Granularity    string                  `json:"granularity"`
}

type DeliveryInsightsObject struct {
	Kind     InsightsObjectKind `json:"kind"`
	ID       string             `json:"id"`
	ParentID string             `json:"parent_id,omitempty"`
	Version  int                `json:"version"`
	Name     string             `json:"name"`
	Platform string             `json:"platform"`
}

// DeliveryMetricFact is an immutable, project-scoped base fact. Derived
// metrics are intentionally represented separately and never overwrite it.
type DeliveryMetricFact struct {
	ID                string                  `json:"id,omitempty"`
	OrganizationID    contract.OrganizationID `json:"organization_id"`
	ProjectID         contract.ProjectID      `json:"project_id"`
	PlanID            string                  `json:"plan_id,omitempty"`
	ExecutionID       string                  `json:"execution_id,omitempty"`
	SimulationRunID   string                  `json:"simulation_run_id,omitempty"`
	Platform          string                  `json:"platform"`
	ObjectKind        InsightsObjectKind      `json:"object_kind"`
	ObjectID          string                  `json:"object_id"`
	Metric            InsightsMetricName      `json:"metric"`
	WindowStart       time.Time               `json:"window_start"`
	WindowEnd         time.Time               `json:"window_end"`
	Granularity       string                  `json:"granularity"`
	Timezone          string                  `json:"timezone"`
	Value             float64                 `json:"value"`
	Unit              string                  `json:"unit"`
	Currency          string                  `json:"currency,omitempty"`
	SchemaVersion     string                  `json:"schema_version"`
	DefinitionVersion string                  `json:"definition_version"`
	Source            InsightsSource          `json:"source"`
	ObservedAt        time.Time               `json:"observed_at"`
	DataThrough       time.Time               `json:"data_through"`
	Freshness         InsightsFreshnessStatus `json:"freshness"`
	Quality           InsightsQualityStatus   `json:"quality"`
	EvidenceRefs      []string                `json:"evidence_refs"`
}

type DeliveryInsightsSnapshot struct {
	OrganizationID contract.OrganizationID  `json:"organization_id"`
	ProjectID      contract.ProjectID       `json:"project_id"`
	Platform       string                   `json:"platform"`
	SchemaVersion  string                   `json:"schema_version"`
	FixtureVersion string                   `json:"fixture_version"`
	Source         InsightsSource           `json:"source"`
	Quality        InsightsQualityStatus    `json:"quality"`
	QualityReason  string                   `json:"quality_reason"`
	ObservedAt     time.Time                `json:"observed_at"`
	DataThrough    time.Time                `json:"data_through"`
	Objects        []DeliveryInsightsObject `json:"objects"`
	Metrics        []DeliveryMetricFact     `json:"metrics"`
	EvidenceRefs   []string                 `json:"evidence_refs"`
}

func (s DeliveryInsightsSnapshot) Usable() bool {
	if s.Quality != InsightsQualityUsable || s.SchemaVersion != InsightsSchemaVersion || len(s.Metrics) == 0 {
		return false
	}
	kinds := map[InsightsObjectKind]bool{}
	objects := map[string]DeliveryInsightsObject{}
	for _, object := range s.Objects {
		kinds[object.Kind] = true
		objects[object.ID] = object
	}
	if !kinds[InsightsObjectAccount] || !kinds[InsightsObjectProject] || (!kinds[InsightsObjectUnit] && !kinds[InsightsObjectPromotion]) || !kinds[InsightsObjectMaterial] {
		return false
	}
	for _, object := range s.Objects {
		if object.Kind != InsightsObjectAccount && (object.ParentID == "" || objects[object.ParentID].ID == "") {
			return false
		}
	}
	baseMetrics := map[InsightsMetricName]bool{}
	metricGroups := map[string]map[InsightsMetricName]bool{}
	for _, fact := range s.Metrics {
		if fact.OrganizationID != s.OrganizationID || fact.ProjectID != s.ProjectID || fact.Platform != s.Platform || fact.Quality != InsightsQualityUsable || fact.Freshness != InsightsFreshnessFresh || fact.Source == "" || len(fact.EvidenceRefs) == 0 || fact.SchemaVersion != InsightsSchemaVersion || fact.DefinitionVersion != InsightsMetricDefinitionV1 || !validMetricWindow(fact.WindowStart, fact.WindowEnd) || fact.Granularity == "" || fact.Timezone == "" || fact.ObservedAt.IsZero() || fact.DataThrough.IsZero() || fact.Value < 0 {
			return false
		}
		if fact.ObjectID == "" || objects[fact.ObjectID].ID == "" {
			return false
		}
		if fact.Metric == InsightsMetricSpend && (fact.Unit != "CNY_minor" || fact.Currency != "CNY") {
			return false
		}
		if fact.Metric != InsightsMetricSpend && fact.Unit != "count" {
			return false
		}
		if fact.Metric != InsightsMetricSpend && fact.Currency != "" {
			return false
		}
		switch fact.Metric {
		case InsightsMetricSpend, InsightsMetricImpressions, InsightsMetricClicks, InsightsMetricConversions:
			baseMetrics[fact.Metric] = true
			key := fmt.Sprintf("%s|%s|%s|%s", fact.ObjectKind, fact.ObjectID, fact.WindowStart.UTC().Format(time.RFC3339Nano), fact.WindowEnd.UTC().Format(time.RFC3339Nano))
			if metricGroups[key] == nil {
				metricGroups[key] = map[InsightsMetricName]bool{}
			}
			metricGroups[key][fact.Metric] = true
		}
	}
	for _, group := range metricGroups {
		if len(group) != 4 {
			return false
		}
	}
	return baseMetrics[InsightsMetricSpend] && baseMetrics[InsightsMetricImpressions] && baseMetrics[InsightsMetricClicks] && baseMetrics[InsightsMetricConversions]
}

func validMetricWindow(start, end time.Time) bool {
	return !start.IsZero() && !end.IsZero() && end.After(start)
}

type insightsMetricWindow struct {
	OrganizationID  contract.OrganizationID
	ProjectID       contract.ProjectID
	Platform        string
	PlanID          string
	ExecutionID     string
	SimulationRunID string
	ObjectKind      InsightsObjectKind
	ObjectID        string
	WindowStart     time.Time
	WindowEnd       time.Time
	DataThrough     time.Time
	Timezone        string
	FixtureVersion  string
	Source          InsightsSource
	EvidenceRefs    []string
	Values          map[InsightsMetricName]float64
}

func (s DeliveryInsightsSnapshot) metricWindows(executionID string) []insightsMetricWindow {
	groups := map[string]*insightsMetricWindow{}
	for _, fact := range s.Metrics {
		if executionID != "" && fact.ExecutionID != executionID {
			continue
		}
		key := fmt.Sprintf("%s|%s|%s|%s|%s", fact.ObjectKind, fact.ObjectID, fact.WindowStart.UTC().Format(time.RFC3339Nano), fact.WindowEnd.UTC().Format(time.RFC3339Nano), fact.ExecutionID)
		window := groups[key]
		if window == nil {
			window = &insightsMetricWindow{OrganizationID: fact.OrganizationID, ProjectID: fact.ProjectID, Platform: fact.Platform, PlanID: fact.PlanID, ExecutionID: fact.ExecutionID, SimulationRunID: fact.SimulationRunID, ObjectKind: fact.ObjectKind, ObjectID: fact.ObjectID, WindowStart: fact.WindowStart, WindowEnd: fact.WindowEnd, DataThrough: fact.DataThrough, Timezone: fact.Timezone, FixtureVersion: s.FixtureVersion, Source: fact.Source, Values: map[InsightsMetricName]float64{}}
			groups[key] = window
		}
		window.Values[fact.Metric] = fact.Value
		if fact.DataThrough.After(window.DataThrough) {
			window.DataThrough = fact.DataThrough
		}
		window.EvidenceRefs = mergeEvidence(window.EvidenceRefs, fact.EvidenceRefs)
	}
	values := make([]insightsMetricWindow, 0, len(groups))
	for _, window := range groups {
		if window.Values[InsightsMetricSpend] == 0 && !hasMetric(window.Values, InsightsMetricSpend) || !hasMetric(window.Values, InsightsMetricImpressions) || !hasMetric(window.Values, InsightsMetricClicks) || !hasMetric(window.Values, InsightsMetricConversions) {
			continue
		}
		values = append(values, *window)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].WindowStart.Before(values[j].WindowStart) })
	return values
}

func hasMetric(values map[InsightsMetricName]float64, name InsightsMetricName) bool {
	_, ok := values[name]
	return ok
}

type DerivedMetric struct {
	Name           string                `json:"name"`
	Value          *float64              `json:"value,omitempty"`
	Unit           string                `json:"unit"`
	Formula        string                `json:"formula"`
	FormulaVersion string                `json:"formula_version"`
	Status         InsightsQualityStatus `json:"status"`
	Reason         string                `json:"reason,omitempty"`
	EvidenceRefs   []string              `json:"evidence_refs"`
}

// DeriveRatio is the sole v1 derived-metric operation. A zero denominator is
// unavailable, never a fabricated zero.
func DeriveRatio(name string, numerator, denominator DeliveryMetricFact) DerivedMetric {
	result := DerivedMetric{Name: name, Unit: "ratio", Formula: "numerator / denominator", FormulaVersion: InsightsDerivedFormulaV1, Status: InsightsQualityUnavailable, EvidenceRefs: mergeEvidence(numerator.EvidenceRefs, denominator.EvidenceRefs)}
	if numerator.ObjectKind != InsightsObjectProject || denominator.ObjectKind != InsightsObjectProject {
		result.Reason = "derived metrics are project-level in delivery-derived-metrics/v1"
		return result
	}
	if numerator.OrganizationID != denominator.OrganizationID || numerator.ProjectID != denominator.ProjectID || numerator.ObjectKind != denominator.ObjectKind || numerator.ObjectID != denominator.ObjectID || !numerator.WindowStart.Equal(denominator.WindowStart) || !numerator.WindowEnd.Equal(denominator.WindowEnd) {
		result.Reason = "numerator and denominator are outside the same project/object/window"
		return result
	}
	if numerator.Quality != InsightsQualityUsable || denominator.Quality != InsightsQualityUsable {
		result.Reason = "input metric quality is not usable"
		return result
	}
	if denominator.Value == 0 {
		result.Reason = "denominator is zero"
		return result
	}
	value := numerator.Value / denominator.Value
	result.Value = &value
	result.Status = InsightsQualityUsable
	return result
}

type InsightsConsumer interface {
	Read(context.Context, InsightsQuery) (DeliveryInsightsSnapshot, error)
}

type fixtureEnvelope struct {
	FixtureVersion string                   `json:"fixture_version"`
	SchemaVersion  string                   `json:"schema_version"`
	Source         InsightsSource           `json:"source"`
	Quality        InsightsQualityStatus    `json:"quality"`
	QualityReason  string                   `json:"quality_reason"`
	ObservedAt     time.Time                `json:"observed_at"`
	DataThrough    time.Time                `json:"data_through"`
	Objects        []DeliveryInsightsObject `json:"objects"`
	Metrics        []DeliveryMetricFact     `json:"metrics"`
	EvidenceRefs   []string                 `json:"evidence_refs"`
}

type MockInsightsReader struct{}

func (MockInsightsReader) Read(ctx context.Context, query InsightsQuery) (DeliveryInsightsSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return DeliveryInsightsSnapshot{}, err
	}
	if query.OrganizationID == "" || query.ProjectID == "" {
		return DeliveryInsightsSnapshot{}, ErrInsightsScope
	}
	fixture := normalizeInsightsFixture(query.Fixture)
	path := "fixtures/insights-" + fixture + ".json"
	data, err := insightsFixtureFS.ReadFile(path)
	if err != nil {
		return DeliveryInsightsSnapshot{}, fmt.Errorf("%w: %s", ErrInsightsFixture, path)
	}
	var envelope fixtureEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return DeliveryInsightsSnapshot{}, fmt.Errorf("%w: %s: %v", ErrInsightsFixture, path, err)
	}
	if envelope.Source != InsightsSourceMock {
		return DeliveryInsightsSnapshot{}, fmt.Errorf("%w: source must be %q", ErrInsightsFixture, InsightsSourceMock)
	}
	platform := strings.TrimSpace(query.Platform)
	if platform == "" {
		platform = "ocean_engine"
	}
	snapshot := DeliveryInsightsSnapshot{OrganizationID: query.OrganizationID, ProjectID: query.ProjectID, Platform: platform, SchemaVersion: envelope.SchemaVersion, FixtureVersion: envelope.FixtureVersion, Source: InsightsSourceMock, Quality: envelope.Quality, QualityReason: envelope.QualityReason, ObservedAt: envelope.ObservedAt.UTC(), DataThrough: envelope.DataThrough.UTC(), Objects: append([]DeliveryInsightsObject(nil), envelope.Objects...), Metrics: append([]DeliveryMetricFact(nil), envelope.Metrics...), EvidenceRefs: append([]string(nil), envelope.EvidenceRefs...)}
	if !query.WindowStart.IsZero() && !query.WindowEnd.IsZero() && query.WindowEnd.After(query.WindowStart) {
		snapshot.DataThrough = query.WindowEnd.UTC()
	}
	for index := range snapshot.Objects {
		snapshot.Objects[index].ID = scopedInsightsID(query.ProjectID, snapshot.Objects[index].ID)
		if snapshot.Objects[index].ParentID != "" {
			snapshot.Objects[index].ParentID = scopedInsightsID(query.ProjectID, snapshot.Objects[index].ParentID)
		}
		snapshot.Objects[index].Platform = platform
	}
	for index := range snapshot.Metrics {
		fact := &snapshot.Metrics[index]
		fact.OrganizationID, fact.ProjectID, fact.Platform = query.OrganizationID, query.ProjectID, platform
		fact.ObjectID = scopedInsightsID(query.ProjectID, fact.ObjectID)
		fact.Source = InsightsSourceMock
		fact.ObservedAt, fact.DataThrough = snapshot.ObservedAt, snapshot.DataThrough
		if !query.WindowStart.IsZero() && !query.WindowEnd.IsZero() && query.WindowEnd.After(query.WindowStart) {
			fact.WindowStart, fact.WindowEnd = query.WindowStart.UTC(), query.WindowEnd.UTC()
		}
		fact.EvidenceRefs = scopeEvidence(query.ProjectID, fact.EvidenceRefs)
	}
	snapshot.EvidenceRefs = scopeEvidence(query.ProjectID, snapshot.EvidenceRefs)
	return snapshot, nil
}

// SimulationInsightsReader is the explicit mock/replay bridge for the current
// OutcomeSimulation records. It normalizes those records into the same
// DeliveryMetricFact contract consumed by alert rules; the rules do not read
// the repository metrics directly.
type SimulationInsightsReader struct {
	Repository Repository
}

func (r SimulationInsightsReader) Read(ctx context.Context, query InsightsQuery) (DeliveryInsightsSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return DeliveryInsightsSnapshot{}, err
	}
	if r.Repository == nil || query.OrganizationID == "" || query.ProjectID == "" {
		return DeliveryInsightsSnapshot{}, ErrInsightsScope
	}
	values, err := r.Repository.ListProjectMetricSnapshots(ctx, query.OrganizationID, query.ProjectID, 100)
	if err != nil {
		return DeliveryInsightsSnapshot{}, err
	}
	if query.ExecutionID != "" {
		filtered := make([]DeliveryMetricSnapshot, 0, len(values))
		for _, value := range values {
			if value.ExecutionID == query.ExecutionID {
				filtered = append(filtered, value)
			}
		}
		values = filtered
	}
	if len(values) == 0 {
		reason := "simulation has no metric windows"
		evidence := "simulation://project/empty"
		if query.ExecutionID != "" {
			reason = "requested execution has no metric windows"
			evidence = "simulation://execution/" + query.ExecutionID + "/empty"
		}
		return DeliveryInsightsSnapshot{OrganizationID: query.OrganizationID, ProjectID: query.ProjectID, Platform: query.Platform, SchemaVersion: InsightsSchemaVersion, FixtureVersion: "post-launch-simulator/v1/empty", Source: InsightsSourceMock, Quality: InsightsQualityEmpty, QualityReason: reason, EvidenceRefs: []string{evidence}}, nil
	}
	seed := values[0]
	for _, value := range values {
		if value.ExecutionID != "" {
			seed = value
			break
		}
	}
	selected := make([]DeliveryMetricSnapshot, 0, len(values))
	for _, value := range values {
		if seed.ExecutionID != "" && value.ExecutionID != seed.ExecutionID {
			continue
		}
		if seed.SimulationRunID != "" && value.SimulationRunID != seed.SimulationRunID {
			continue
		}
		selected = append(selected, value)
	}
	platform := strings.TrimSpace(query.Platform)
	if platform == "" {
		platform = "ocean_engine"
	}
	timezone := "UTC"
	if plan, planErr := r.Repository.GetPlan(ctx, query.OrganizationID, query.ProjectID, seed.PlanID); planErr == nil && plan.CurrentVersion.Schedule.Timezone != "" {
		timezone = plan.CurrentVersion.Schedule.Timezone
	}
	objectID := scopedInsightsID(query.ProjectID, seed.PlanID)
	snapshot := DeliveryInsightsSnapshot{OrganizationID: query.OrganizationID, ProjectID: query.ProjectID, Platform: platform, SchemaVersion: InsightsSchemaVersion, FixtureVersion: "post-launch-simulator/v1/" + seed.FixtureVersion, Source: InsightsSourceMock, Quality: InsightsQualityUsable, QualityReason: "deterministic simulation facts normalized through the consumer port", ObservedAt: seed.CreatedAt.UTC(), DataThrough: seed.DataThrough.UTC(), Objects: []DeliveryInsightsObject{{Kind: InsightsObjectAccount, ID: scopedInsightsID(query.ProjectID, "simulation-account"), Version: 1, Name: "Simulation account", Platform: platform}, {Kind: InsightsObjectProject, ID: objectID, ParentID: scopedInsightsID(query.ProjectID, "simulation-account"), Version: 1, Name: "Simulation project", Platform: platform}, {Kind: InsightsObjectUnit, ID: scopedInsightsID(query.ProjectID, seed.PlanID+"/unit"), ParentID: objectID, Version: 1, Name: "Simulation unit", Platform: platform}, {Kind: InsightsObjectMaterial, ID: scopedInsightsID(query.ProjectID, seed.PlanID+"/material"), ParentID: scopedInsightsID(query.ProjectID, seed.PlanID+"/unit"), Version: 1, Name: "Simulation material", Platform: platform}}, EvidenceRefs: []string{"simulation://project/" + seed.PlanID}}
	for _, value := range selected {
		observedAt := value.CreatedAt.UTC()
		if observedAt.IsZero() {
			observedAt = value.DataThrough.UTC()
		}
		appendSimulationFacts(&snapshot, value, timezone, objectID, observedAt)
	}
	if len(snapshot.Metrics) == 0 {
		snapshot.Quality, snapshot.QualityReason = InsightsQualityIncomplete, "simulation windows did not contain base metrics"
	}
	return snapshot, nil
}

func appendSimulationFacts(snapshot *DeliveryInsightsSnapshot, value DeliveryMetricSnapshot, timezone, objectID string, observedAt time.Time) {
	base := func(name InsightsMetricName, amount float64, unit, currency string) {
		snapshot.Metrics = append(snapshot.Metrics, DeliveryMetricFact{ID: value.ID + "/" + string(name), OrganizationID: value.OrganizationID, ProjectID: value.ProjectID, PlanID: value.PlanID, ExecutionID: value.ExecutionID, SimulationRunID: value.SimulationRunID, Platform: snapshot.Platform, ObjectKind: InsightsObjectProject, ObjectID: objectID, Metric: name, WindowStart: value.WindowStart, WindowEnd: value.WindowEnd, Granularity: "window", Timezone: timezone, Value: amount, Unit: unit, Currency: currency, SchemaVersion: InsightsSchemaVersion, DefinitionVersion: InsightsMetricDefinitionV1, Source: InsightsSourceMock, ObservedAt: observedAt, DataThrough: value.DataThrough, Freshness: InsightsFreshnessFresh, Quality: InsightsQualityUsable, EvidenceRefs: []string{"simulation://metric/" + value.ID}})
	}
	base(InsightsMetricSpend, float64(value.RawMetrics.SpendCents), "CNY_minor", "CNY")
	base(InsightsMetricImpressions, float64(value.RawMetrics.Impressions), "count", "")
	base(InsightsMetricClicks, float64(value.RawMetrics.Clicks), "count", "")
	base(InsightsMetricConversions, float64(value.RawMetrics.Conversions), "count", "")
	snapshot.EvidenceRefs = mergeEvidence(snapshot.EvidenceRefs, []string{"simulation://metric/" + value.ID})
}

type ReplayInsightsReader struct {
	Snapshot DeliveryInsightsSnapshot
}

func NewReplayInsightsReader(snapshot DeliveryInsightsSnapshot) ReplayInsightsReader {
	return ReplayInsightsReader{Snapshot: snapshot}
}

func (r ReplayInsightsReader) Read(ctx context.Context, query InsightsQuery) (DeliveryInsightsSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return DeliveryInsightsSnapshot{}, err
	}
	if query.OrganizationID == "" || query.ProjectID == "" || query.OrganizationID != r.Snapshot.OrganizationID || query.ProjectID != r.Snapshot.ProjectID {
		return DeliveryInsightsSnapshot{}, ErrInsightsScope
	}
	snapshot := r.Snapshot
	snapshot.Source = InsightsSourceReplay
	snapshot.Objects = append([]DeliveryInsightsObject(nil), r.Snapshot.Objects...)
	snapshot.Metrics = append([]DeliveryMetricFact(nil), r.Snapshot.Metrics...)
	snapshot.EvidenceRefs = append([]string(nil), r.Snapshot.EvidenceRefs...)
	for index := range snapshot.Metrics {
		snapshot.Metrics[index].Source = InsightsSourceReplay
		snapshot.Metrics[index].EvidenceRefs = append([]string(nil), snapshot.Metrics[index].EvidenceRefs...)
	}
	return snapshot, nil
}

func normalizeInsightsFixture(value string) string {
	switch strings.TrimSpace(value) {
	case InsightsFixtureEmpty:
		return InsightsFixtureEmpty
	case InsightsFixtureStale, "stale_data":
		return InsightsFixtureStale
	case InsightsFixtureIncomplete, "insufficient_data":
		return InsightsFixtureIncomplete
	case InsightsFixtureSchemaMismatch:
		return "schema-mismatch"
	case InsightsFixtureUnavailable:
		return InsightsFixtureUnavailable
	default:
		return InsightsFixtureUsable
	}
}

func scopedInsightsID(projectID contract.ProjectID, id string) string {
	return string(projectID) + "/" + id
}

func scopeEvidence(projectID contract.ProjectID, refs []string) []string {
	values := make([]string, 0, len(refs))
	for _, ref := range refs {
		values = append(values, fmt.Sprintf("%s?project_id=%s", ref, projectID))
	}
	return values
}

func mergeEvidence(left, right []string) []string {
	values := append([]string(nil), left...)
	for _, ref := range right {
		for _, existing := range values {
			if existing == ref {
				ref = ""
				break
			}
		}
		if ref != "" {
			values = append(values, ref)
		}
	}
	return values
}
