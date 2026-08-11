package delivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type OutcomeSimulationScenario string

const (
	OutcomeSimulationModelVersion = "delivery-outcome-scenario/v1"

	OutcomeScenarioSteady          OutcomeSimulationScenario = "steady"
	OutcomeScenarioCostPressure    OutcomeSimulationScenario = "cost_pressure"
	OutcomeScenarioUnderDelivery   OutcomeSimulationScenario = "under_delivery"
	OutcomeScenarioCreativeFatigue OutcomeSimulationScenario = "creative_fatigue"
	OutcomeScenarioTrackingAnomaly OutcomeSimulationScenario = "tracking_anomaly"
	OutcomeScenarioReviewRejected  OutcomeSimulationScenario = "review_rejected"
)

type CreateOutcomeSimulationRequest struct {
	Scenario   OutcomeSimulationScenario `json:"scenario"`
	StableSeed string                    `json:"stable_seed"`
}

func (r CreateOutcomeSimulationRequest) Validate() error {
	if len(strings.TrimSpace(r.StableSeed)) > 128 {
		return ErrInvalidRequest
	}
	switch r.Scenario {
	case OutcomeScenarioSteady, OutcomeScenarioCostPressure, OutcomeScenarioUnderDelivery,
		OutcomeScenarioCreativeFatigue, OutcomeScenarioTrackingAnomaly, OutcomeScenarioReviewRejected:
		return nil
	default:
		return ErrInvalidRequest
	}
}

type OutcomeCreativeFeature struct {
	AssetID     string `json:"asset_id"`
	Version     int    `json:"version"`
	ContentHash string `json:"content_hash"`
	QualityBP   int64  `json:"quality_bp"`
}

type OutcomeSimulationInput struct {
	PlanID                 string                   `json:"plan_id"`
	PlanVersion            int                      `json:"plan_version"`
	PlanCanonicalHash      string                   `json:"plan_canonical_hash"`
	Budget                 Budget                   `json:"budget"`
	Schedule               Schedule                 `json:"schedule"`
	Objective              string                   `json:"objective"`
	OptimizationGoal       string                   `json:"optimization_goal"`
	BidMinor               int64                    `json:"bid_minor"`
	Audience               string                   `json:"audience"`
	StrategyReference      StrategyReference        `json:"strategy_reference"`
	CreativeFeatures       []OutcomeCreativeFeature `json:"creative_features"`
	ConfigurationHash      string                   `json:"configuration_hash"`
	PlatformExecutionID    string                   `json:"platform_execution_id"`
	PlatformExecutionMode  string                   `json:"platform_execution_mode"`
	PlatformExecutionProof string                   `json:"platform_execution_proof"`
}

type OutcomeSimulationFactor struct {
	Key         string   `json:"key"`
	ValueBP     int64    `json:"value_bp"`
	Explanation string   `json:"explanation"`
	Evidence    []string `json:"evidence"`
}

type OutcomeSimulationParameters struct {
	BaseCPMMinor         int64                     `json:"base_cpm_minor"`
	BaseCTRBP            int64                     `json:"base_ctr_bp"`
	BaseCVRBP            int64                     `json:"base_cvr_bp"`
	RevenuePerConversion int64                     `json:"revenue_per_conversion_minor"`
	DailyBudgetMinor     int64                     `json:"daily_budget_minor"`
	Factors              []OutcomeSimulationFactor `json:"factors"`
}

type OutcomeSimulationEvent struct {
	Type           string   `json:"type"`
	Severity       string   `json:"severity"`
	WindowSequence int      `json:"window_sequence"`
	Explanation    string   `json:"explanation"`
	Evidence       []string `json:"evidence"`
}

type MetricCalculationBasis struct {
	Formula          string                    `json:"formula"`
	SpendMultiplier  int64                     `json:"spend_multiplier_bp"`
	ReachMultiplier  int64                     `json:"reach_multiplier_bp"`
	CTRMultiplier    int64                     `json:"ctr_multiplier_bp"`
	CVRMultiplier    int64                     `json:"cvr_multiplier_bp"`
	TrackingRate     int64                     `json:"tracking_rate_bp"`
	AppliedFactors   []OutcomeSimulationFactor `json:"applied_factors"`
	ScenarioEvidence []string                  `json:"scenario_evidence"`
}

type OutcomeSimulationRun struct {
	ID             string                      `json:"id"`
	OrganizationID contract.OrganizationID     `json:"organization_id"`
	ProjectID      contract.ProjectID          `json:"project_id"`
	ExecutionID    string                      `json:"execution_id"`
	PlanID         string                      `json:"plan_id"`
	PlanVersion    int                         `json:"plan_version"`
	PlanHash       string                      `json:"plan_hash"`
	ModelVersion   string                      `json:"model_version"`
	Scenario       OutcomeSimulationScenario   `json:"scenario"`
	StableSeed     string                      `json:"stable_seed"`
	InputHash      string                      `json:"input_hash"`
	Fingerprint    string                      `json:"fingerprint"`
	Input          OutcomeSimulationInput      `json:"input"`
	Parameters     OutcomeSimulationParameters `json:"parameters"`
	Events         []OutcomeSimulationEvent    `json:"events"`
	Evidence       []string                    `json:"evidence"`
	Status         string                      `json:"status"`
	CreatedBy      string                      `json:"created_by"`
	CreatedAt      time.Time                   `json:"created_at"`
	CompletedAt    time.Time                   `json:"completed_at"`
}

type OutcomeSimulationResult struct {
	Run             OutcomeSimulationRun     `json:"run"`
	MetricSnapshots []DeliveryMetricSnapshot `json:"metric_snapshots"`
	Replay          bool                     `json:"replay"`
}

type outcomeSimulationRepository interface {
	CreateOrGetOutcomeSimulation(context.Context, OutcomeSimulationRun, []DeliveryMetricSnapshot) (OutcomeSimulationRun, []DeliveryMetricSnapshot, bool, error)
	GetLatestOutcomeSimulation(context.Context, contract.OrganizationID, contract.ProjectID, string) (OutcomeSimulationRun, []DeliveryMetricSnapshot, error)
}

func (s Service) outcomeSimulations() (outcomeSimulationRepository, error) {
	r, ok := s.Repository.(outcomeSimulationRepository)
	if !ok {
		return nil, ErrUnsupportedConfigurationWorkflow
	}
	return r, nil
}

func (s Service) CreateOutcomeSimulation(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, executionID string, request CreateOutcomeSimulationRequest) (OutcomeSimulationResult, error) {
	if err := s.ready(actor, projectID, ScopeWrite); err != nil {
		return OutcomeSimulationResult{}, err
	}
	if strings.TrimSpace(executionID) == "" || request.Validate() != nil {
		return OutcomeSimulationResult{}, ErrInvalidRequest
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return OutcomeSimulationResult{}, err
	}
	execution, err := s.findExecution(ctx, actor.OrganizationID, projectID, executionID)
	if err != nil {
		return OutcomeSimulationResult{}, err
	}
	if execution.Execution.Mode != ExecutionModeLocalSimulation || execution.Execution.Status != ExecutionSucceeded {
		return OutcomeSimulationResult{}, ErrInvalidState
	}
	version, err := s.Repository.GetPlanVersion(ctx, actor.OrganizationID, projectID, execution.ChangeSet.PlanID, int(execution.ChangeSet.PlanVersion))
	if err != nil {
		return OutcomeSimulationResult{}, err
	}
	input, err := outcomeInput(version, execution)
	if err != nil {
		return OutcomeSimulationResult{}, err
	}
	if strings.TrimSpace(request.StableSeed) == "" {
		request.StableSeed = version.CanonicalHash[:minInt(16, len(version.CanonicalHash))]
	}
	inputHash, err := contract.CanonicalJSONHash(input)
	if err != nil {
		return OutcomeSimulationResult{}, err
	}
	fingerprint, err := contract.CanonicalJSONHash(struct {
		OrganizationID contract.OrganizationID   `json:"organization_id"`
		ProjectID      contract.ProjectID        `json:"project_id"`
		ExecutionID    string                    `json:"execution_id"`
		InputHash      string                    `json:"input_hash"`
		ModelVersion   string                    `json:"model_version"`
		Scenario       OutcomeSimulationScenario `json:"scenario"`
		StableSeed     string                    `json:"stable_seed"`
	}{actor.OrganizationID, projectID, executionID, inputHash, OutcomeSimulationModelVersion, request.Scenario, request.StableSeed})
	if err != nil {
		return OutcomeSimulationResult{}, err
	}
	runID, err := s.idGenerator()("deliverysimulationrun")
	if err != nil {
		return OutcomeSimulationResult{}, err
	}
	completedAt := execution.Execution.StartedAt
	if execution.Execution.CompletedAt != nil {
		completedAt = *execution.Execution.CompletedAt
	}
	runAt := s.now()
	parameters, metrics, events := simulateOutcome(input, request, fingerprint, completedAt)
	run := OutcomeSimulationRun{
		ID: runID, OrganizationID: actor.OrganizationID, ProjectID: projectID, ExecutionID: executionID,
		PlanID: version.PlanID, PlanVersion: version.VersionNumber, PlanHash: version.CanonicalHash,
		ModelVersion: OutcomeSimulationModelVersion, Scenario: request.Scenario, StableSeed: request.StableSeed,
		InputHash: inputHash, Fingerprint: fingerprint, Input: input, Parameters: parameters, Events: events,
		Evidence: []string{"simulation://execution/" + executionID, "plan-version://" + version.PlanID + "/" + strconv.Itoa(version.VersionNumber), "simulation-model://" + OutcomeSimulationModelVersion},
		Status:   "completed", CreatedBy: actor.Principal.ID, CreatedAt: runAt, CompletedAt: runAt,
	}
	for index := range metrics {
		metricID, idErr := s.idGenerator()("deliverymetric")
		if idErr != nil {
			return OutcomeSimulationResult{}, idErr
		}
		metrics[index].ID = metricID
		metrics[index].SimulationRunID = runID
		metrics[index].OrganizationID = actor.OrganizationID
		metrics[index].ProjectID = projectID
		metrics[index].ExecutionID = executionID
		metrics[index].PlanID = version.PlanID
		metrics[index].CreativePackageID = firstCreativeAsset(version)
		metrics[index].CreatedBy = actor.Principal.ID
		metrics[index].CreatedAt = runAt
	}
	r, err := s.outcomeSimulations()
	if err != nil {
		return OutcomeSimulationResult{}, err
	}
	storedRun, storedMetrics, replay, err := r.CreateOrGetOutcomeSimulation(ctx, run, metrics)
	if err != nil {
		return OutcomeSimulationResult{}, err
	}
	return OutcomeSimulationResult{Run: storedRun, MetricSnapshots: storedMetrics, Replay: replay}, nil
}

func (s Service) GetLatestOutcomeSimulation(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, executionID string) (OutcomeSimulationResult, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return OutcomeSimulationResult{}, err
	}
	if strings.TrimSpace(executionID) == "" {
		return OutcomeSimulationResult{}, ErrInvalidRequest
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return OutcomeSimulationResult{}, err
	}
	r, err := s.outcomeSimulations()
	if err != nil {
		return OutcomeSimulationResult{}, err
	}
	run, metrics, err := r.GetLatestOutcomeSimulation(ctx, actor.OrganizationID, projectID, executionID)
	return OutcomeSimulationResult{Run: run, MetricSnapshots: metrics, Replay: true}, err
}

func outcomeInput(version DeliveryPlanVersion, execution ExecutionResult) (OutcomeSimulationInput, error) {
	if version.IsPlatformConfigurationV2() {
		return platformOutcomeInput(version, execution)
	}
	configuration := version.ThreeTierConfiguration
	if execution.ChangeSet.LegacyTargetSnapshot != nil {
		configuration = execution.ChangeSet.LegacyTargetSnapshot
	}
	configurationHash, err := snapshotHash(configuration)
	if err != nil {
		return OutcomeSimulationInput{}, err
	}
	optimization, audience := version.Objective, "all"
	budget, bid := version.Budget.TotalMinor, maxInt64(1, version.Budget.TotalMinor/100)
	if field := findThreeTierField(configuration, "optimization"); field != nil {
		optimization = fmt.Sprint(field.Effective.Value)
	}
	if field := findThreeTierField(configuration, "audience"); field != nil {
		audience = fmt.Sprint(field.Effective.Value)
	}
	if field := findThreeTierField(configuration, "budget"); field != nil {
		budget = numericMinor(field.Effective.Value, budget)
	}
	if field := findThreeTierField(configuration, "bid"); field != nil {
		bid = numericMinor(field.Effective.Value, bid)
	}
	features := make([]OutcomeCreativeFeature, 0, len(version.CreativeReferences))
	for _, creative := range version.CreativeReferences {
		hash := creative.ContentHash
		if hash == "" {
			hash, _ = contract.CanonicalJSONHash(creative)
		}
		features = append(features, OutcomeCreativeFeature{AssetID: creative.AssetID, Version: creative.Version, ContentHash: hash, QualityBP: stableRange(hash, 8500, 11500)})
	}
	if len(features) == 0 {
		features = append(features, OutcomeCreativeFeature{AssetID: "mock-creative", Version: 1, ContentHash: configurationHash, QualityBP: stableRange(configurationHash, 8500, 11500)})
	}
	version.Budget.TotalMinor = budget
	return OutcomeSimulationInput{
		PlanID: version.PlanID, PlanVersion: version.VersionNumber, PlanCanonicalHash: version.CanonicalHash,
		Budget: version.Budget, Schedule: version.Schedule, Objective: version.Objective, OptimizationGoal: optimization,
		BidMinor: bid, Audience: audience, StrategyReference: version.StrategyReference, CreativeFeatures: features,
		ConfigurationHash: configurationHash, PlatformExecutionID: execution.Execution.ID, PlatformExecutionMode: execution.Execution.Mode,
		PlatformExecutionProof: execution.Evidence.ID,
	}, nil
}

func platformOutcomeInput(version DeliveryPlanVersion, execution ExecutionResult) (OutcomeSimulationInput, error) {
	configuration := version.PlatformConfiguration
	if execution.ChangeSet.TargetSnapshot != nil {
		configuration = execution.ChangeSet.TargetSnapshot
	}
	if configuration == nil || configuration.CanonicalHash == "" {
		return OutcomeSimulationInput{}, ErrInvalidState
	}
	intent := version.DeliveryIntent
	budget := Budget{TotalMinor: intent.Payload.BudgetBoundary.MaximumTotalMinor, Currency: intent.Payload.BudgetBoundary.Currency}
	schedule := Schedule{StartAt: intent.Payload.ScheduleBoundary.EarliestStart, EndAt: intent.Payload.ScheduleBoundary.LatestEnd, Timezone: intent.Payload.ScheduleBoundary.Timezone}
	optimization := intent.Payload.MarketingObjective
	if len(intent.Payload.OptimizationPreferences) > 0 {
		optimization = intent.Payload.OptimizationPreferences[0].Metric
	}
	audience := strings.Join(intent.Payload.AudienceConstraints.Constraints, ",")
	if audience == "" {
		audience = "all"
	}
	bid := maxInt64(1, budget.TotalMinor/100)
	if ocean := configuration.Payload.OceanEngine; ocean != nil && ocean.Project != nil {
		if ocean.Project.BudgetAndBidding.BidMinor != nil {
			bid = *ocean.Project.BudgetAndBidding.BidMinor
		}
	}
	strategyVersion := 1
	if parsed, err := strconv.Atoi(strings.TrimPrefix(intent.Payload.StrategyReference.Version, "v")); err == nil && parsed > 0 {
		strategyVersion = parsed
	}
	strategy := StrategyReference{TaskID: intent.Payload.StrategyReference.ID, Version: int64(strategyVersion), ContentHash: intent.Payload.StrategyReference.ContentHash}
	features := make([]OutcomeCreativeFeature, 0, len(intent.Payload.MaterialReferences))
	for _, reference := range intent.Payload.MaterialReferences {
		versionNumber := 1
		if parsed, err := strconv.Atoi(strings.TrimPrefix(reference.Version, "v")); err == nil && parsed > 0 {
			versionNumber = parsed
		}
		hash := reference.ContentHash
		if hash == "" {
			hash, _ = contract.CanonicalJSONHash(reference)
		}
		features = append(features, OutcomeCreativeFeature{AssetID: reference.ID, Version: versionNumber, ContentHash: hash, QualityBP: stableRange(hash, 8500, 11500)})
	}
	if len(features) == 0 {
		features = append(features, OutcomeCreativeFeature{AssetID: "mock-creative", Version: 1, ContentHash: configuration.CanonicalHash, QualityBP: stableRange(configuration.CanonicalHash, 8500, 11500)})
	}
	return OutcomeSimulationInput{
		PlanID: version.PlanID, PlanVersion: version.VersionNumber, PlanCanonicalHash: version.CanonicalHash,
		Budget: budget, Schedule: schedule, Objective: intent.Payload.MarketingObjective, OptimizationGoal: optimization,
		BidMinor: bid, Audience: audience, StrategyReference: strategy, CreativeFeatures: features,
		ConfigurationHash: configuration.CanonicalHash, PlatformExecutionID: execution.Execution.ID, PlatformExecutionMode: execution.Execution.Mode,
		PlatformExecutionProof: execution.Evidence.ID,
	}, nil
}

func simulateOutcome(input OutcomeSimulationInput, request CreateOutcomeSimulationRequest, fingerprint string, completedAt time.Time) (OutcomeSimulationParameters, []DeliveryMetricSnapshot, []OutcomeSimulationEvent) {
	days := int64(input.Schedule.EndAt.Sub(input.Schedule.StartAt).Hours() / 24)
	if days < 1 {
		days = 1
	}
	dailyBudget := maxInt64(100, input.Budget.TotalMinor/minInt64(days, 30))
	creativeTotal := int64(0)
	for _, feature := range input.CreativeFeatures {
		creativeTotal += feature.QualityBP
	}
	creativeBP := creativeTotal / int64(len(input.CreativeFeatures))
	audienceBP := stableRange(input.Audience+request.StableSeed, 8500, 11500)
	strategyBP := stableRange(input.StrategyReference.ContentHash+input.StrategyReference.TaskID, 9000, 11000)
	expectedBid := maxInt64(1, input.Budget.TotalMinor/100)
	bidBP := clampInt64(input.BidMinor*10000/expectedBid, 7000, 13000)
	seedBP := stableRange(fingerprint, 9500, 10500)
	factors := []OutcomeSimulationFactor{
		{Key: "budget", ValueBP: 10000, Explanation: "预算决定每日最大可消耗规模，调整预算会近似同比改变消耗与可购买曝光。", Evidence: []string{"plan://budget"}},
		{Key: "bid", ValueBP: bidBP, Explanation: "出价相对基准影响竞价胜率与千次曝光成本。", Evidence: []string{"configuration://field/bid"}},
		{Key: "audience", ValueBP: audienceBP, Explanation: "定向边界的稳定特征影响可触达规模与点击倾向。", Evidence: []string{"configuration://field/audience"}},
		{Key: "creative", ValueBP: creativeBP, Explanation: "素材版本与内容哈希形成稳定的 Mock 质量因子。", Evidence: []string{"plan://creative-references"}},
		{Key: "strategy", ValueBP: strategyBP, Explanation: "策略来源版本形成稳定的目标匹配因子。", Evidence: []string{"plan://strategy-reference"}},
		{Key: "seed", ValueBP: seedBP, Explanation: "稳定 seed 仅提供有界扰动，相同输入必然得到相同结果。", Evidence: []string{"simulation://stable-seed"}},
	}
	parameters := OutcomeSimulationParameters{BaseCPMMinor: 2800, BaseCTRBP: 480, BaseCVRBP: 520, RevenuePerConversion: 18000, DailyBudgetMinor: dailyBudget, Factors: factors}
	type multipliers struct{ spend, reach, ctr, cvr, tracking int64 }
	windows := []multipliers{{7000, 10000, 10000, 10000, 10000}, {8500, 10200, 9800, 9500, 10000}, {9000, 10400, 9700, 9200, 10000}}
	scenarioEvidence := []string{"scenario://" + string(request.Scenario)}
	switch request.Scenario {
	case OutcomeScenarioCostPressure:
		windows[1], windows[2] = multipliers{9800, 9800, 8500, 6500, 10000}, multipliers{12500, 9200, 7800, 3800, 10000}
	case OutcomeScenarioUnderDelivery:
		windows[1], windows[2] = multipliers{5200, 7000, 9600, 9200, 10000}, multipliers{3000, 4200, 9300, 8500, 10000}
	case OutcomeScenarioCreativeFatigue:
		windows[1], windows[2] = multipliers{8500, 10000, 8000, 8000, 10000}, multipliers{8500, 9800, 5200, 5500, 10000}
	case OutcomeScenarioTrackingAnomaly:
		windows[1], windows[2] = multipliers{8500, 10000, 9800, 9500, 4500}, multipliers{9000, 10200, 9700, 9000, 0}
	case OutcomeScenarioReviewRejected:
		windows[1], windows[2] = multipliers{0, 0, 0, 0, 10000}, multipliers{0, 0, 0, 0, 10000}
	}
	metrics := make([]DeliveryMetricSnapshot, 0, len(windows))
	fixtureVersion := OutcomeSimulationModelVersion + "/" + fingerprint[:12]
	for index, multiplier := range windows {
		spend := dailyBudget * multiplier.spend / 10000
		cpm := parameters.BaseCPMMinor * bidBP / 10000
		impressions := spend * 1000 * audienceBP * multiplier.reach / maxInt64(1, cpm) / 10000 / 10000
		ctrBP := parameters.BaseCTRBP * creativeBP * seedBP * multiplier.ctr / 10000 / 10000 / 10000
		clicks := impressions * ctrBP / 10000
		cvrBP := parameters.BaseCVRBP * strategyBP * multiplier.cvr / 10000 / 10000
		trackedConversions := clicks * cvrBP * multiplier.tracking / 10000 / 10000
		if clicks > 0 && trackedConversions == 0 && multiplier.tracking > 0 {
			trackedConversions = 1
		}
		start := completedAt.Add(time.Duration(index-3) * 24 * time.Hour)
		end := start.Add(24 * time.Hour)
		metrics = append(metrics, DeliveryMetricSnapshot{
			Source: MetricSourceDemoFixture, IsSimulated: true, DatasetVersion: OutcomeSimulationModelVersion,
			FixtureVersion: fixtureVersion, WindowSequence: index + 1, DataThrough: end, Currency: "CNY", WindowStart: start, WindowEnd: end,
			RawMetrics:       RawMetrics{Impressions: impressions, Clicks: clicks, Conversions: trackedConversions, SpendCents: spend, RevenueCents: trackedConversions * parameters.RevenuePerConversion},
			CalculationBasis: MetricCalculationBasis{Formula: "spend → impressions(CPM,bid,audience) → clicks(CTR,creative) → conversions(CVR,strategy,tracking)", SpendMultiplier: multiplier.spend, ReachMultiplier: multiplier.reach, CTRMultiplier: multiplier.ctr, CVRMultiplier: multiplier.cvr, TrackingRate: multiplier.tracking, AppliedFactors: factors, ScenarioEvidence: scenarioEvidence},
		})
	}
	events := outcomeEvents(request.Scenario, metrics)
	return parameters, metrics, events
}

func outcomeEvents(scenario OutcomeSimulationScenario, metrics []DeliveryMetricSnapshot) []OutcomeSimulationEvent {
	if len(metrics) < 2 {
		return []OutcomeSimulationEvent{}
	}
	evidence := []string{"simulation://metric-window/1", "simulation://metric-window/3"}
	last := metrics[len(metrics)-1]
	switch scenario {
	case OutcomeScenarioCostPressure:
		return []OutcomeSimulationEvent{{Type: "cost_worsening", Severity: "high", WindowSequence: 3, Explanation: "竞价成本上升且转化效率下降，当前 CPA 相对基准显著恶化。", Evidence: evidence}}
	case OutcomeScenarioUnderDelivery:
		return []OutcomeSimulationEvent{{Type: "under_delivery", Severity: "medium", WindowSequence: 3, Explanation: "当前窗口消耗和曝光低于预算可支持规模。", Evidence: evidence}}
	case OutcomeScenarioCreativeFatigue:
		return []OutcomeSimulationEvent{{Type: "creative_fatigue", Severity: "high", WindowSequence: 3, Explanation: "素材点击率与转化率连续衰减。", Evidence: evidence}}
	case OutcomeScenarioTrackingAnomaly:
		return []OutcomeSimulationEvent{{Type: "tracking_anomaly", Severity: "critical", WindowSequence: 3, Explanation: "存在点击但追踪到的转化为零，模拟追踪链路中断。", Evidence: evidence}}
	case OutcomeScenarioReviewRejected:
		return []OutcomeSimulationEvent{{Type: "review_rejected", Severity: "critical", WindowSequence: 2, Explanation: "模拟平台审核拒绝，后续窗口不产生投放数据。", Evidence: []string{"simulation://platform-event/review-rejected"}}}
	default:
		if last.RawMetrics.Conversions > 0 {
			return []OutcomeSimulationEvent{}
		}
	}
	return []OutcomeSimulationEvent{}
}

func numericMinor(value any, fallback int64) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	default:
		return fallback
	}
}

func stableRange(value string, minimum, maximum int64) int64 {
	digest := sha256.Sum256([]byte(value))
	hash := hex.EncodeToString(digest[:])
	number, _ := strconv.ParseUint(hash[:12], 16, 64)
	return minimum + int64(number%uint64(maximum-minimum+1))
}

func clampInt64(value, minimum, maximum int64) int64 {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func minInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func firstCreativeAsset(version DeliveryPlanVersion) string {
	if version.IsPlatformConfigurationV2() && len(version.DeliveryIntent.Payload.MaterialReferences) > 0 {
		return version.DeliveryIntent.Payload.MaterialReferences[0].ID
	}
	if len(version.CreativeReferences) > 0 {
		return version.CreativeReferences[0].AssetID
	}
	return "mock-creative"
}
