package delivery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type AlertType string
type AlertStatus string
type AlertAction string
type AlertEvaluationScenario string

const (
	AlertReviewRejected              AlertType               = "review_rejected"
	AlertSpendSpike                  AlertType               = "spend_spike"
	AlertZeroConversion              AlertType               = "zero_conversion"
	AlertCostWorsening               AlertType               = "cost_worsening"
	AlertUnderDelivery               AlertType               = "under_delivery"
	AlertCreativeFatigue             AlertType               = "creative_fatigue"
	AlertTrackingAnomaly             AlertType               = "tracking_anomaly"
	AlertOpen                        AlertStatus             = "open"
	AlertAcknowledged                AlertStatus             = "acknowledged"
	AlertDismissed                   AlertStatus             = "dismissed"
	AlertAcknowledge                 AlertAction             = "acknowledge"
	AlertDismiss                     AlertAction             = "dismiss"
	AlertScenarioNormalDay           AlertEvaluationScenario = "normal_day"
	AlertScenarioAnomalyDay          AlertEvaluationScenario = "anomaly_day"
	AlertScenarioStaleData           AlertEvaluationScenario = "stale_data"
	AlertScenarioInsufficientData    AlertEvaluationScenario = "insufficient_data"
	AlertScenarioConnectorInspection AlertEvaluationScenario = "connector_inspection"
)

type DeliveryAlert struct {
	ID               string                  `json:"id"`
	OrganizationID   contract.OrganizationID `json:"organization_id"`
	ProjectID        contract.ProjectID      `json:"project_id"`
	PlanID           string                  `json:"plan_id"`
	ExecutionID      string                  `json:"execution_id"`
	SimulationRunID  string                  `json:"simulation_run_id,omitempty"`
	MonitoredEntity  AlertMonitoredEntity    `json:"monitored_entity"`
	Type             AlertType               `json:"type"`
	RuleID           string                  `json:"rule_id"`
	RuleVersion      string                  `json:"rule_version"`
	Status           AlertStatus             `json:"status"`
	Fingerprint      string                  `json:"fingerprint"`
	Title            string                  `json:"title"`
	Detail           string                  `json:"detail"`
	Severity         string                  `json:"severity"`
	Window           AlertWindow             `json:"window"`
	MetricDefinition AlertMetricDefinition   `json:"metric_definition"`
	Owner            AlertOwner              `json:"owner"`
	EvidenceRefs     []string                `json:"evidence_refs"`
	Source           string                  `json:"source"`
	IsSimulated      bool                    `json:"is_simulated"`
	Scenario         AlertEvaluationScenario `json:"scenario"`
	DatasetVersion   string                  `json:"dataset_version"`
	FixtureVersion   string                  `json:"fixture_version"`
	Freshness        AlertFreshness          `json:"freshness"`
	Version          int64                   `json:"version"`
	CreatedAt        time.Time               `json:"created_at"`
	UpdatedAt        time.Time               `json:"updated_at"`
	CreatedBy        string                  `json:"created_by"`
	AcknowledgedAt   *time.Time              `json:"acknowledged_at"`
	DismissedAt      *time.Time              `json:"dismissed_at"`
	ResolvedBy       string                  `json:"-"`
}
type AlertFreshness struct {
	Status         string    `json:"status"`
	AsOf           time.Time `json:"as_of"`
	EvaluatedAt    time.Time `json:"evaluated_at"`
	AgeSeconds     int64     `json:"age_seconds"`
	MaxAgeSeconds  int64     `json:"max_age_seconds"`
	MissingMetrics []string  `json:"missing_metrics,omitempty"`
}
type AlertMonitoredEntity struct {
	Type         string `json:"type"`
	ID           string `json:"id"`
	AdvertiserID string `json:"advertiser_id"`
}
type AlertWindow struct {
	Start         time.Time  `json:"start"`
	End           time.Time  `json:"end"`
	Timezone      string     `json:"timezone"`
	DataThrough   time.Time  `json:"data_through"`
	BaselineStart *time.Time `json:"baseline_start,omitempty"`
	BaselineEnd   *time.Time `json:"baseline_end,omitempty"`
}
type AlertMetricDefinition struct {
	Name          string   `json:"name"`
	Unit          string   `json:"unit"`
	Numerator     *float64 `json:"numerator,omitempty"`
	Denominator   *float64 `json:"denominator,omitempty"`
	ObservedValue *float64 `json:"observed_value,omitempty"`
	BaselineValue *float64 `json:"baseline_value,omitempty"`
	Threshold     *float64 `json:"threshold,omitempty"`
}
type AlertOwner struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Source      string `json:"source"`
}
type AlertList struct {
	Items       []DeliveryAlert `json:"items"`
	NextCursor  string          `json:"next_cursor,omitempty"`
	Source      string          `json:"source"`
	IsSimulated bool            `json:"is_simulated"`
}
type AlertFilter struct {
	PlanID      string
	ExecutionID string
	Status      AlertStatus
	Type        AlertType
	Severity    string
	Fixture     AlertEvaluationScenario
	Cursor      string
	Limit       int
}
type UpdateAlertRequest struct {
	Action          AlertAction `json:"action"`
	ExpectedVersion int64       `json:"expected_version"`
}

func (r UpdateAlertRequest) Validate() error {
	if r.ExpectedVersion < 1 || (r.Action != AlertAcknowledge && r.Action != AlertDismiss) {
		return ErrInvalidRequest
	}
	return nil
}

func alertTitle(kind AlertType) string {
	return map[AlertType]string{AlertReviewRejected: "平台审核被拒", AlertSpendSpike: "消耗较基准明显上升", AlertZeroConversion: "有点击但没有转化", AlertCostWorsening: "转化成本较基准恶化", AlertUnderDelivery: "跑量不足", AlertCreativeFatigue: "素材疲劳", AlertTrackingAnomaly: "追踪异常"}[kind]
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
func alertSeverity(kind AlertType) string {
	switch kind {
	case AlertReviewRejected, AlertTrackingAnomaly:
		return "critical"
	case AlertSpendSpike, AlertUnderDelivery:
		return "medium"
	default:
		return "high"
	}
}
func (s Service) ListAlerts(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, filter AlertFilter) ([]DeliveryAlert, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return nil, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 50
	}
	return s.Repository.ListAlerts(ctx, actor.OrganizationID, projectID, filter)
}
func (s Service) UpdateAlert(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, id string, request UpdateAlertRequest) (DeliveryAlert, error) {
	if err := s.ready(actor, projectID, ScopeWrite); err != nil {
		return DeliveryAlert{}, err
	}
	if strings.TrimSpace(id) == "" {
		return DeliveryAlert{}, ErrInvalidRequest
	}
	if err := request.Validate(); err != nil {
		return DeliveryAlert{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return DeliveryAlert{}, err
	}
	return s.Repository.UpdateAlert(ctx, actor.OrganizationID, projectID, id, request.Action, request.ExpectedVersion, actor.Principal.ID, s.now())
}
func alertStatus(action AlertAction) (AlertStatus, error) {
	if action == AlertAcknowledge {
		return AlertAcknowledged, nil
	}
	if action == AlertDismiss {
		return AlertDismissed, nil
	}
	return "", fmt.Errorf("%w: alert action", ErrInvalidRequest)
}
