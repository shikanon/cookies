package delivery

import (
	"reflect"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

func TestOutcomeSimulationIsDeterministicAndCausallySensitive(t *testing.T) {
	completedAt := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	input := OutcomeSimulationInput{
		PlanID: "plan_1", PlanVersion: 4, PlanCanonicalHash: "plan-hash", Budget: Budget{TotalMinor: 300000, Currency: "CNY"},
		Schedule:  Schedule{StartAt: completedAt, EndAt: completedAt.Add(10 * 24 * time.Hour), Timezone: "Asia/Shanghai"},
		Objective: "conversion", OptimizationGoal: "purchase", BidMinor: 3000, Audience: "high-intent",
		StrategyReference: StrategyReference{TaskID: "strategy_1", Version: 2, ContentHash: "strategy-hash"},
		CreativeFeatures:  []OutcomeCreativeFeature{{AssetID: "asset_1", Version: 3, ContentHash: "creative-hash", QualityBP: 10200}},
	}
	request := CreateOutcomeSimulationRequest{Scenario: OutcomeScenarioCostPressure, StableSeed: "stable-seed"}

	parametersA, metricsA, eventsA := simulateOutcome(input, request, "1234567890abcdef", completedAt)
	parametersB, metricsB, eventsB := simulateOutcome(input, request, "1234567890abcdef", completedAt)
	if !reflect.DeepEqual(parametersA, parametersB) || !reflect.DeepEqual(metricsA, metricsB) || !reflect.DeepEqual(eventsA, eventsB) {
		t.Fatal("same input and stable seed must produce the same parameters, windows and events")
	}
	if len(metricsA) != 3 || metricsA[2].RawMetrics.Conversions == 0 {
		t.Fatalf("cost-pressure scenario should produce three non-zero-conversion windows: %#v", metricsA)
	}

	higherBudget := input
	higherBudget.Budget.TotalMinor *= 2
	_, budgetMetrics, _ := simulateOutcome(higherBudget, request, "abcdef1234567890", completedAt)
	if budgetMetrics[0].RawMetrics.SpendCents <= metricsA[0].RawMetrics.SpendCents || budgetMetrics[0].RawMetrics.Impressions <= metricsA[0].RawMetrics.Impressions {
		t.Fatalf("higher budget must explainably increase spend and impressions: baseline=%#v higher=%#v", metricsA[0].RawMetrics, budgetMetrics[0].RawMetrics)
	}

	betterCreative := input
	betterCreative.CreativeFeatures = []OutcomeCreativeFeature{{AssetID: "asset_2", Version: 1, ContentHash: "better", QualityBP: 11400}}
	_, creativeMetrics, _ := simulateOutcome(betterCreative, request, "feedface12345678", completedAt)
	if creativeMetrics[0].RawMetrics.Clicks <= metricsA[0].RawMetrics.Clicks {
		t.Fatalf("higher creative feature must increase clicks: baseline=%d higher=%d", metricsA[0].RawMetrics.Clicks, creativeMetrics[0].RawMetrics.Clicks)
	}
}

func TestApprovedRecommendationTargetBecomesANewPlanVersion(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	version := DeliveryPlanVersion{
		PlanID: "plan_1", OrganizationID: "org_1", ProjectID: "project_1", VersionNumber: 3, Name: "Plan", Objective: "conversion",
		Advertiser: MockAdvertiser{ID: "advertiser_1", Name: "Advertiser", Platform: "ocean_engine"}, Budget: Budget{TotalMinor: 300000, Currency: "CNY"},
		Schedule: Schedule{StartAt: now, EndAt: now.Add(10 * 24 * time.Hour), Timezone: "Asia/Shanghai"}, Platform: "ocean_engine_mock", Source: SourceMock,
	}
	configuration, err := compileThreeTierFixture(version, ThreeTierFixtureGoldenPath)
	if err != nil {
		t.Fatal(err)
	}
	version.ThreeTierConfiguration = configuration
	version.CanonicalHash, err = PlanCanonicalHash(version)
	if err != nil {
		t.Fatal(err)
	}
	target := cloneThreeTierConfiguration(configuration)
	findThreeTierField(target, "budget").Effective.Value = int64(240000)
	targetHash, err := snapshotHash(target)
	if err != nil {
		t.Fatal(err)
	}
	plan := DeliveryPlan{ID: "plan_1", OrganizationID: "org_1", ProjectID: "project_1", CurrentVersionNumber: 3, Version: 3, CurrentVersion: version}
	changeSet := ChangeSet{PlanID: plan.ID, PlanVersion: 3, LegacyTargetSnapshot: target, TargetSnapshotHash: targetHash}

	optimized, replay, err := optimizedVersionFromChangeSet(plan, changeSet, contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, now)
	if err != nil || replay {
		t.Fatalf("optimized=%#v replay=%v err=%v", optimized, replay, err)
	}
	if optimized.VersionNumber != 4 || optimized.Budget.TotalMinor != 240000 || optimized.CanonicalHash == version.CanonicalHash {
		t.Fatalf("approved target must create a new internally materialized version: %#v", optimized)
	}
	plan.CurrentVersionNumber, plan.Version, plan.CurrentVersion = 4, 4, optimized
	got, replay, err := optimizedVersionFromChangeSet(plan, changeSet, contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, now.Add(time.Hour))
	if err != nil || !replay || got.VersionNumber != 4 {
		t.Fatalf("materialization must be idempotent: got=%#v replay=%v err=%v", got, replay, err)
	}
}

func TestApprovedPlatformConfigurationTargetBecomesANewPlanVersion(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	intent, configuration := readyOceanRuntimeInputs(t, 2)
	version, err := newPlatformPlanVersion("plan_1", contract.ActorContext{OrganizationID: "org_a", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}}, "project_a", 1, intent, configuration, now)
	if err != nil {
		t.Fatal(err)
	}
	target := cloneJSONPointer(version.PlatformConfiguration)
	target.VersionNumber++
	target.Payload.OceanEngine.Project.BudgetAndBidding.DailyBudgetMinor = 15000
	target.CanonicalHash = ""
	target, err = finalizeRecommendationConfiguration(target)
	if err != nil {
		t.Fatal(err)
	}
	plan := DeliveryPlan{ID: "plan_1", OrganizationID: "org_a", ProjectID: "project_a", CurrentVersionNumber: 1, Version: 1, CurrentVersion: version}
	changeSet := ChangeSet{PlanID: plan.ID, PlanVersion: 1, TargetSnapshot: target, TargetSnapshotHash: target.CanonicalHash}
	optimized, replay, err := optimizedVersionFromChangeSet(plan, changeSet, contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, now.Add(time.Hour))
	if err != nil || replay {
		t.Fatalf("optimized=%#v replay=%v err=%v", optimized, replay, err)
	}
	if optimized.VersionNumber != 2 || optimized.PlatformConfiguration.CanonicalHash != target.CanonicalHash || optimized.CanonicalHash != target.CanonicalHash {
		t.Fatalf("typed target was not materialized exactly: %#v", optimized)
	}
}

func TestOutcomeSimulationScenariosEmitMatchingEvents(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	input := OutcomeSimulationInput{
		Budget: Budget{TotalMinor: 300000, Currency: "CNY"}, Schedule: Schedule{StartAt: now, EndAt: now.Add(10 * 24 * time.Hour)},
		BidMinor: 3000, Audience: "audience", CreativeFeatures: []OutcomeCreativeFeature{{QualityBP: 10000}},
	}
	tests := []struct {
		scenario OutcomeSimulationScenario
		event    string
	}{
		{OutcomeScenarioCostPressure, "cost_worsening"},
		{OutcomeScenarioUnderDelivery, "under_delivery"},
		{OutcomeScenarioCreativeFatigue, "creative_fatigue"},
		{OutcomeScenarioTrackingAnomaly, "tracking_anomaly"},
		{OutcomeScenarioReviewRejected, "review_rejected"},
	}
	for _, test := range tests {
		t.Run(string(test.scenario), func(t *testing.T) {
			_, metrics, events := simulateOutcome(input, CreateOutcomeSimulationRequest{Scenario: test.scenario, StableSeed: "seed"}, "1234567890abcdef", now)
			if len(events) != 1 || events[0].Type != test.event {
				t.Fatalf("expected %q event, got %#v", test.event, events)
			}
			if test.scenario == OutcomeScenarioTrackingAnomaly && metrics[2].RawMetrics.Conversions != 0 {
				t.Fatalf("tracking anomaly should suppress tracked conversions, got %d", metrics[2].RawMetrics.Conversions)
			}
		})
	}
}

func TestRecommendationMustMatchCurrentPlanVersionBeforeAcceptance(t *testing.T) {
	plan := DeliveryPlan{ID: "plan_1", CurrentVersionNumber: 4}
	current := DeliveryRecommendation{PlanID: "plan_1", PlanVersion: 4}
	if !recommendationMatchesCurrentPlan(current, plan) {
		t.Fatal("recommendation from the current PlanVersion should remain decidable")
	}
	stale := current
	stale.PlanVersion = 3
	if recommendationMatchesCurrentPlan(stale, plan) {
		t.Fatal("recommendation from an older PlanVersion must not be accepted")
	}
}
