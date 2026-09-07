package delivery

import (
	"context"
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
	GetLatestOutcomeSimulation(context.Context, contract.OrganizationID, contract.ProjectID, string) (OutcomeSimulationRun, []DeliveryMetricSnapshot, error)
}

func (s Service) outcomeSimulations() (outcomeSimulationRepository, error) {
	r, ok := s.Repository.(outcomeSimulationRepository)
	if !ok {
		return nil, ErrUnsupportedConfigurationWorkflow
	}
	return r, nil
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

func clampInt64(value, minimum, maximum int64) int64 {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
