package delivery

import (
	"context"
	"fmt"
	"github.com/shikanon/cookies/internal/platform/contract"
	"sort"
	"strings"
	"time"
)

type ThreeTierFixture = Scenario

const (
	ThreeTierFixtureGoldenPath            ThreeTierFixture = "golden_path"
	ThreeTierFixtureMissingRequiredField  ThreeTierFixture = "missing_required_field"
	ThreeTierFixtureOrphanDependency      ThreeTierFixture = "orphan_dependency"
	ThreeTierFixtureMissingConfirmation   ThreeTierFixture = "missing_confirmation"
	ThreeTierFixturePlatformFieldsPending ThreeTierFixture = "platform_fields_pending"
)

type CompileThreeTierRequest struct {
	ExpectedVersion int              `json:"expected_version"`
	Fixture         ThreeTierFixture `json:"fixture"`
}
type ThreeTierOverrideRequest struct {
	ExpectedVersion int            `json:"expected_version"`
	GroupID         string         `json:"group_id"`
	PlanID          string         `json:"plan_id"`
	CreativeID      string         `json:"creative_id"`
	FieldKey        string         `json:"field_key"`
	Value           ThreeTierValue `json:"value"`
	Confirmed       bool           `json:"confirmed"`
}
type RecommendationStatus string

const (
	RecommendationProposed RecommendationStatus = "proposed"
	RecommendationAccepted RecommendationStatus = "accepted"
	RecommendationRejected RecommendationStatus = "rejected"
)

type DeliveryRecommendation struct {
	ID                  string                  `json:"id"`
	OrganizationID      contract.OrganizationID `json:"organization_id"`
	ProjectID           contract.ProjectID      `json:"project_id"`
	PlanID              string                  `json:"plan_id"`
	PlanVersion         int                     `json:"plan_version"`
	SimulationRunID     string                  `json:"simulation_run_id"`
	Fingerprint         string                  `json:"fingerprint"`
	BaseSnapshotHash    string                  `json:"base_snapshot_hash"`
	BaseSnapshot        *ThreeTierConfiguration `json:"base_snapshot,omitempty"`
	TargetSnapshot      *ThreeTierConfiguration `json:"target_snapshot,omitempty"`
	BaseConfiguration   *PlatformConfiguration  `json:"base_configuration,omitempty"`
	TargetConfiguration *PlatformConfiguration  `json:"target_configuration,omitempty"`
	TargetSnapshotHash  string                  `json:"target_snapshot_hash"`
	Evidence            []string                `json:"evidence"`
	Action              string                  `json:"action"`
	Impact              string                  `json:"impact"`
	Risks               []string                `json:"risks"`
	Observation         string                  `json:"observation"`
	CooldownUntil       *time.Time              `json:"cooldown_until,omitempty"`
	Provenance          string                  `json:"provenance"`
	Status              RecommendationStatus    `json:"status"`
	Version             int64                   `json:"version"`
	IdempotencyKey      string                  `json:"-"`
	RequestHash         string                  `json:"-"`
	AcceptedChangeSetID string                  `json:"accepted_change_set_id,omitempty"`
	CreatedBy           string                  `json:"created_by"`
	CreatedAt           time.Time               `json:"created_at"`
	UpdatedAt           time.Time               `json:"updated_at"`
}
type RecommendationAcceptance struct {
	Recommendation DeliveryRecommendation `json:"recommendation"`
	ChangeSet      ChangeSet              `json:"change_set"`
}
type ManualActionPackage struct {
	ID                          string                    `json:"id"`
	OrganizationID              contract.OrganizationID   `json:"organization_id"`
	ProjectID                   contract.ProjectID        `json:"project_id"`
	ChangeSetID                 string                    `json:"change_set_id"`
	TargetSnapshotHash          string                    `json:"target_snapshot_hash"`
	ConfigurationSchemaVersion  string                    `json:"configuration_schema_version,omitempty"`
	ConfigurationID             string                    `json:"configuration_id,omitempty"`
	ConfigurationVersion        int                       `json:"configuration_version,omitempty"`
	ConfigurationPlatform       DeliveryPlatform          `json:"configuration_platform,omitempty"`
	ConfigurationProfileVersion string                    `json:"configuration_profile_version,omitempty"`
	ConfigurationCanonicalHash  string                    `json:"configuration_canonical_hash,omitempty"`
	IntentSchemaVersion         string                    `json:"intent_schema_version,omitempty"`
	IntentID                    string                    `json:"intent_id,omitempty"`
	IntentVersion               int                       `json:"intent_version,omitempty"`
	IntentCanonicalHash         string                    `json:"intent_canonical_hash,omitempty"`
	ContentHash                 string                    `json:"content_hash"`
	Instructions                []ManualActionInstruction `json:"instructions"`
	ForbiddenActions            []string                  `json:"forbidden_actions"`
	Evidence                    []string                  `json:"evidence"`
	Provenance                  string                    `json:"provenance"`
	OptimizedPlanVersion        int                       `json:"optimized_plan_version"`
	OptimizedPlanHash           string                    `json:"optimized_plan_hash"`
	Source                      Source                    `json:"source"`
	Scenario                    string                    `json:"scenario"`
	CreatedAt                   time.Time                 `json:"created_at"`
}
type ManualActionInstruction struct {
	Layer                string         `json:"layer"`
	GroupID              string         `json:"group_id"`
	PlanID               string         `json:"plan_id"`
	CreativeID           string         `json:"creative_id"`
	FieldKey             string         `json:"field_key"`
	Effective            ThreeTierValue `json:"effective"`
	Source               string         `json:"source"`
	ConfirmationRequired bool           `json:"confirmation_required"`
	ExpectedResult       string         `json:"expected_result"`
	EvidenceRefs         []string       `json:"evidence_refs"`
}

// configurationWorkflowRepository is purposefully optional so existing Repository implementations
// remain source compatible while migrations roll out independently.
type configurationWorkflowRepository interface {
	CreateRecommendation(context.Context, DeliveryRecommendation) (DeliveryRecommendation, error)
	ListRecommendations(context.Context, contract.OrganizationID, contract.ProjectID, int) ([]DeliveryRecommendation, error)
	GetRecommendation(context.Context, contract.OrganizationID, contract.ProjectID, string) (DeliveryRecommendation, error)
	AcceptRecommendation(context.Context, DeliveryRecommendation, string, string, ChangeSet) (RecommendationAcceptance, bool, error)
	RejectRecommendation(context.Context, contract.OrganizationID, contract.ProjectID, string, int64, string, time.Time) (DeliveryRecommendation, error)
	CreateOrGetManualActionPackage(context.Context, ManualActionPackage) (ManualActionPackage, bool, error)
	GetManualActionPackage(context.Context, contract.OrganizationID, contract.ProjectID, string) (ManualActionPackage, error)
}

func (s Service) configurationWorkflow() (configurationWorkflowRepository, error) {
	r, ok := s.Repository.(configurationWorkflowRepository)
	if !ok {
		return nil, ErrUnsupportedConfigurationWorkflow
	}
	return r, nil
}
func (s Service) CompileThreeTierConfiguration(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, planID string, request CompileThreeTierRequest) (DeliveryPlan, error) {
	if err := s.ready(actor, projectID, ScopeWrite); err != nil {
		return DeliveryPlan{}, err
	}
	if request.ExpectedVersion < 1 {
		return DeliveryPlan{}, ErrInvalidRequest
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return DeliveryPlan{}, err
	}
	p, err := s.Repository.GetPlan(ctx, actor.OrganizationID, projectID, planID)
	if err != nil {
		return DeliveryPlan{}, err
	}
	if p.CurrentVersion.IsPlatformConfigurationV2() || p.CurrentVersion.ReadOnly {
		return DeliveryPlan{}, ErrLegacyConfigurationUnsupported
	}
	c, err := compileThreeTierFixture(p.CurrentVersion, request.Fixture)
	if err != nil {
		return DeliveryPlan{}, err
	}
	c.GeneratedAt = s.now()
	d := draftFromVersion(p.CurrentVersion)
	v, err := versionFromDraft(p, request.ExpectedVersion+1, d, actor.Principal, s.now())
	if err != nil {
		return DeliveryPlan{}, err
	}
	v.ThreeTierConfiguration = c
	v.Scenario, v.Advertiser.Scenario = Scenario(c.Scenario), Scenario(c.Scenario)
	v.CanonicalHash, err = PlanCanonicalHash(v)
	if err != nil {
		return DeliveryPlan{}, err
	}
	return s.Repository.UpdatePlan(ctx, actor.OrganizationID, projectID, planID, request.ExpectedVersion, v)
}
func (s Service) OverrideThreeTierField(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, planID string, request ThreeTierOverrideRequest) (DeliveryPlan, error) {
	if err := s.ready(actor, projectID, ScopeWrite); err != nil {
		return DeliveryPlan{}, err
	}
	if request.ExpectedVersion < 1 || request.Value.Type == "" || !request.Confirmed {
		return DeliveryPlan{}, ErrInvalidRequest
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return DeliveryPlan{}, err
	}
	p, err := s.Repository.GetPlan(ctx, actor.OrganizationID, projectID, planID)
	if err != nil {
		return DeliveryPlan{}, err
	}
	if p.CurrentVersion.IsPlatformConfigurationV2() || p.CurrentVersion.ReadOnly {
		return DeliveryPlan{}, ErrLegacyConfigurationUnsupported
	}
	if p.Version != int64(request.ExpectedVersion) || p.CurrentVersion.ThreeTierConfiguration == nil {
		return DeliveryPlan{}, ErrVersionConflict
	}
	c := cloneThreeTierConfiguration(p.CurrentVersion.ThreeTierConfiguration)
	c.GeneratedAt = s.now()
	found := false
	for gi := range c.Groups {
		g := &c.Groups[gi]
		if g.ID != request.GroupID {
			continue
		}
		for pi := range g.Plans {
			pl := &g.Plans[pi]
			if pl.ID != request.PlanID {
				continue
			}
			for ci := range pl.Creatives {
				cr := &pl.Creatives[ci]
				if cr.ID != request.CreativeID {
					continue
				}
				for fi := range cr.Fields {
					f := &cr.Fields[fi]
					if f.Key == request.FieldKey {
						if !f.Editable || request.Value.Type != f.Effective.Type || request.Value.Value == nil {
							return DeliveryPlan{}, ErrInvalidRequest
						}
						x := request.Value
						f.Manual = &x
						f.Effective = x
						f.EffectiveSource = "manual_override"
						f.Confirmed = true
						found = true
					}
				}
			}
		}
	}
	if !found {
		return DeliveryPlan{}, ErrNotFound
	}
	d := draftFromVersion(p.CurrentVersion)
	v, err := versionFromDraft(p, request.ExpectedVersion+1, d, actor.Principal, s.now())
	if err != nil {
		return DeliveryPlan{}, err
	}
	v.ThreeTierConfiguration = c
	v.CanonicalHash, err = PlanCanonicalHash(v)
	if err != nil {
		return DeliveryPlan{}, err
	}
	return s.Repository.UpdatePlan(ctx, actor.OrganizationID, projectID, planID, request.ExpectedVersion, v)
}
func snapshotHash(c *ThreeTierConfiguration) (string, error) {
	if c == nil {
		return "", nil
	}
	return contract.CanonicalJSONHash(c)
}
func changeSetPreflightVersion(base DeliveryPlanVersion, changeSet ChangeSet) (DeliveryPlanVersion, error) {
	if changeSet.TargetSnapshot != nil {
		if !base.IsPlatformConfigurationV2() {
			return DeliveryPlanVersion{}, ErrLegacyConfigurationUnsupported
		}
		if err := changeSet.TargetSnapshot.validateStructure(); err != nil {
			return DeliveryPlanVersion{}, err
		}
		if changeSet.TargetSnapshot.CanonicalHash != changeSet.TargetSnapshotHash {
			return DeliveryPlanVersion{}, ErrApprovalContentMismatch
		}
		v := cloneVersion(base)
		v.PlatformConfiguration = cloneJSONPointer(changeSet.TargetSnapshot)
		v.CanonicalHash = changeSet.TargetSnapshotHash
		return v, nil
	}
	if changeSet.LegacyTargetSnapshot == nil {
		return base, nil
	}
	hash, err := snapshotHash(changeSet.LegacyTargetSnapshot)
	if err != nil {
		return DeliveryPlanVersion{}, err
	}
	if hash == "" || hash != changeSet.TargetSnapshotHash {
		return DeliveryPlanVersion{}, ErrApprovalContentMismatch
	}
	v := cloneVersion(base)
	v.ThreeTierConfiguration = cloneThreeTierConfiguration(changeSet.LegacyTargetSnapshot)
	v.Scenario = Scenario(changeSet.LegacyTargetSnapshot.Scenario)
	return v, nil
}
func (s Service) GenerateRecommendation(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, planID string, expectedVersion int) (DeliveryRecommendation, error) {
	if err := s.ready(actor, projectID, ScopeWrite); err != nil {
		return DeliveryRecommendation{}, err
	}
	r, err := s.configurationWorkflow()
	if err != nil {
		return DeliveryRecommendation{}, err
	}
	p, err := s.Repository.GetPlan(ctx, actor.OrganizationID, projectID, planID)
	if err != nil {
		return DeliveryRecommendation{}, err
	}
	if p.Version != int64(expectedVersion) || (p.CurrentVersion.ThreeTierConfiguration == nil && p.CurrentVersion.PlatformConfiguration == nil) {
		return DeliveryRecommendation{}, ErrVersionConflict
	}
	executions, err := s.Repository.ListExecutions(ctx, actor.OrganizationID, projectID, 100)
	if err != nil {
		return DeliveryRecommendation{}, err
	}
	var sourceExecution *ExecutionResult
	for index := range executions {
		candidate := &executions[index]
		if candidate.ChangeSet.PlanID == p.ID && candidate.Execution.Status == ExecutionSucceeded {
			sourceExecution = candidate
			break
		}
	}
	if sourceExecution == nil {
		return DeliveryRecommendation{}, ErrInvalidState
	}
	simulationRepository, err := s.outcomeSimulations()
	if err != nil {
		return DeliveryRecommendation{}, err
	}
	simulationRun, _, err := simulationRepository.GetLatestOutcomeSimulation(ctx, actor.OrganizationID, projectID, sourceExecution.Execution.ID)
	if err != nil {
		return DeliveryRecommendation{}, ErrInvalidState
	}
	metrics, err := s.Repository.ListMetricSnapshots(ctx, actor.OrganizationID, projectID, sourceExecution.Execution.ID, 100)
	if err != nil {
		return DeliveryRecommendation{}, err
	}
	simulationMetrics := make([]DeliveryMetricSnapshot, 0, len(metrics))
	for _, metric := range metrics {
		if metric.SimulationRunID == simulationRun.ID {
			simulationMetrics = append(simulationMetrics, metric)
		}
	}
	metrics = simulationMetrics
	if len(metrics) < 2 {
		return DeliveryRecommendation{}, ErrInvalidState
	}
	sort.Slice(metrics, func(i, j int) bool { return metrics[i].WindowSequence < metrics[j].WindowSequence })
	baseline, current := metrics[0], metrics[len(metrics)-1]
	alerts, err := s.Repository.ListAlerts(ctx, actor.OrganizationID, projectID, AlertFilter{PlanID: p.ID, ExecutionID: sourceExecution.Execution.ID, Limit: 100})
	if err != nil {
		return DeliveryRecommendation{}, err
	}
	relevantAlerts := make([]DeliveryAlert, 0, len(alerts))
	for _, alert := range alerts {
		if alert.PlanID == p.ID && alert.ExecutionID == sourceExecution.Execution.ID && alert.SimulationRunID == simulationRun.ID {
			relevantAlerts = append(relevantAlerts, alert)
		}
	}
	if len(relevantAlerts) == 0 {
		return DeliveryRecommendation{}, ErrInvalidState
	}
	baselineCPA := baseline.RawMetrics.SpendCents / maxInt64(1, baseline.RawMetrics.Conversions)
	currentCPA := current.RawMetrics.SpendCents / maxInt64(1, current.RawMetrics.Conversions)
	cpaRatioBP := currentCPA * 10000 / maxInt64(1, baselineCPA)
	reductionPercent := clampInt64((cpaRatioBP-10000)/1000, 5, 20)
	// Deterministic and bounded: the recommendation magnitude is derived from
	// the same simulation run's measured CPA degradation.
	evidence := []string{"simulation://execution/" + sourceExecution.Execution.ID, "simulation://run/" + simulationRun.ID, "simulation://metric/" + baseline.ID, "simulation://metric/" + current.ID}
	for _, alert := range relevantAlerts {
		evidence = append(evidence, "simulation://alert/"+alert.ID)
	}
	var baseSnapshot, targetSnapshot *ThreeTierConfiguration
	var baseConfiguration, targetConfiguration *PlatformConfiguration
	var baseHash, h string
	if p.CurrentVersion.IsPlatformConfigurationV2() {
		baseConfiguration = cloneJSONPointer(p.CurrentVersion.PlatformConfiguration)
		targetConfiguration = cloneJSONPointer(p.CurrentVersion.PlatformConfiguration)
		targetConfiguration.VersionNumber++
		ocean := targetConfiguration.Payload.OceanEngine
		if ocean == nil || ocean.Project == nil {
			return DeliveryRecommendation{}, ErrInvalidState
		}
		ocean.Project.BudgetAndBidding.DailyBudgetMinor = ocean.Project.BudgetAndBidding.DailyBudgetMinor * (100 - reductionPercent) / 100
		targetConfiguration.CanonicalHash = ""
		targetConfiguration, err = finalizeRecommendationConfiguration(targetConfiguration)
		if err != nil {
			return DeliveryRecommendation{}, err
		}
		baseHash, h = baseConfiguration.CanonicalHash, targetConfiguration.CanonicalHash
	} else {
		targetSnapshot = cloneThreeTierConfiguration(p.CurrentVersion.ThreeTierConfiguration)
		budget := findThreeTierField(targetSnapshot, "budget")
		if budget == nil {
			return DeliveryRecommendation{}, ErrInvalidState
		}
		switch amount := budget.Effective.Value.(type) {
		case int64:
			budget.Effective.Value = amount * (100 - reductionPercent) / 100
		case float64:
			budget.Effective.Value = amount * float64(100-reductionPercent) / 100
		case int:
			budget.Effective.Value = int(int64(amount) * (100 - reductionPercent) / 100)
		}
		budget.EffectiveSource, budget.Risk = "recommendation", "bounded_budget_reduction"
		budget.RiskRefs = append(budget.RiskRefs, evidence...)
		baseSnapshot = cloneThreeTierConfiguration(p.CurrentVersion.ThreeTierConfiguration)
		baseHash, err = snapshotHash(baseSnapshot)
		if err != nil {
			return DeliveryRecommendation{}, err
		}
		h, err = snapshotHash(targetSnapshot)
		if err != nil {
			return DeliveryRecommendation{}, err
		}
	}
	fp, err := contract.CanonicalJSONHash(struct {
		Plan     string   `json:"plan"`
		Version  int      `json:"version"`
		Hash     string   `json:"hash"`
		Evidence []string `json:"evidence"`
	}{p.ID, expectedVersion, h, evidence})
	if err != nil {
		return DeliveryRecommendation{}, err
	}
	id, err := s.idGenerator()("deliveryrecommendation")
	if err != nil {
		return DeliveryRecommendation{}, err
	}
	now := s.now()
	cooldown := now.Add(24 * time.Hour)
	return r.CreateRecommendation(ctx, DeliveryRecommendation{ID: id, OrganizationID: actor.OrganizationID, ProjectID: projectID, PlanID: p.ID, PlanVersion: expectedVersion, SimulationRunID: simulationRun.ID, Fingerprint: fp, BaseSnapshotHash: baseHash, BaseSnapshot: baseSnapshot, TargetSnapshot: targetSnapshot, BaseConfiguration: baseConfiguration, TargetConfiguration: targetConfiguration, TargetSnapshotHash: h, Evidence: evidence, Action: fmt.Sprintf("reduce_budget_%d_percent", reductionPercent), Impact: fmt.Sprintf("第二次人工批准后，将计划预算下调 %d%%", reductionPercent), Risks: []string{"需验证下调后转化量是否恢复", "不得自动应用"}, Observation: fmt.Sprintf("投后情景模拟的 CPA 从 %d 分上升到 %d 分；调整幅度由 %.2f 倍恶化程度推导，建议观察下一完整窗口", baselineCPA, currentCPA, float64(cpaRatioBP)/10000), CooldownUntil: &cooldown, Provenance: OutcomeSimulationModelVersion, Status: RecommendationProposed, Version: 1, CreatedBy: actor.Principal.ID, CreatedAt: now, UpdatedAt: now})
}

func finalizeRecommendationConfiguration(value *PlatformConfiguration) (*PlatformConfiguration, error) {
	finalized, err := FinalizePlatformConfiguration(*value)
	if err != nil {
		return nil, err
	}
	return &finalized, nil
}
func (s Service) ListRecommendations(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, limit int) ([]DeliveryRecommendation, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return nil, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	r, err := s.configurationWorkflow()
	if err != nil {
		return nil, err
	}
	return r.ListRecommendations(ctx, actor.OrganizationID, projectID, normalizeLimit(limit))
}
func (s Service) GetRecommendation(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, id string) (DeliveryRecommendation, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return DeliveryRecommendation{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return DeliveryRecommendation{}, err
	}
	r, err := s.configurationWorkflow()
	if err != nil {
		return DeliveryRecommendation{}, err
	}
	return r.GetRecommendation(ctx, actor.OrganizationID, projectID, id)
}
func (s Service) AcceptRecommendation(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, id, key string, expected int64) (RecommendationAcceptance, bool, error) {
	if err := s.ready(actor, projectID, ScopeWrite); err != nil {
		return RecommendationAcceptance{}, false, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return RecommendationAcceptance{}, false, err
	}
	r, err := s.configurationWorkflow()
	if err != nil {
		return RecommendationAcceptance{}, false, err
	}
	if strings.TrimSpace(key) == "" || expected < 1 {
		return RecommendationAcceptance{}, false, ErrInvalidRequest
	}
	rec, err := r.GetRecommendation(ctx, actor.OrganizationID, projectID, id)
	if err != nil {
		return RecommendationAcceptance{}, false, err
	}
	if rec.Status != RecommendationProposed && rec.Status != RecommendationAccepted {
		return RecommendationAcceptance{}, false, ErrVersionConflict
	}
	if rec.Status == RecommendationProposed && rec.Version != expected {
		return RecommendationAcceptance{}, false, ErrVersionConflict
	}
	if rec.Status == RecommendationProposed {
		plan, planErr := s.Repository.GetPlan(ctx, actor.OrganizationID, projectID, rec.PlanID)
		if planErr != nil {
			return RecommendationAcceptance{}, false, planErr
		}
		if !recommendationMatchesCurrentPlan(rec, plan) {
			return RecommendationAcceptance{}, false, ErrVersionConflict
		}
	}
	if rec.BaseConfiguration != nil || rec.TargetConfiguration != nil {
		if rec.BaseConfiguration == nil || rec.TargetConfiguration == nil || rec.BaseConfiguration.CanonicalHash != rec.BaseSnapshotHash || rec.TargetConfiguration.CanonicalHash != rec.TargetSnapshotHash {
			return RecommendationAcceptance{}, false, ErrApprovalContentMismatch
		}
		if err := rec.BaseConfiguration.validateStructure(); err != nil {
			return RecommendationAcceptance{}, false, ErrApprovalContentMismatch
		}
		if err := rec.TargetConfiguration.validateStructure(); err != nil {
			return RecommendationAcceptance{}, false, ErrApprovalContentMismatch
		}
	} else {
		baseHash, hashErr := snapshotHash(rec.BaseSnapshot)
		if hashErr != nil || baseHash != rec.BaseSnapshotHash {
			return RecommendationAcceptance{}, false, ErrApprovalContentMismatch
		}
		targetHash, hashErr := snapshotHash(rec.TargetSnapshot)
		if hashErr != nil || targetHash != rec.TargetSnapshotHash {
			return RecommendationAcceptance{}, false, ErrApprovalContentMismatch
		}
	}
	csid, err := s.idGenerator()("deliverychangeset")
	if err != nil {
		return RecommendationAcceptance{}, false, err
	}
	now := s.now()
	reqHash, err := contract.CanonicalJSONHash(struct {
		ID      string `json:"id"`
		Version int64  `json:"version"`
	}{id, expected})
	if err != nil {
		return RecommendationAcceptance{}, false, err
	}
	cs := ChangeSet{ID: csid, OrganizationID: actor.OrganizationID, ProjectID: projectID, PlanID: rec.PlanID, PlanVersion: int64(rec.PlanVersion), Status: ChangeSetDraft, RiskLevel: "low", PreflightNotes: []string{}, TargetSnapshot: cloneJSONPointer(rec.TargetConfiguration), LegacyTargetSnapshot: cloneThreeTierConfiguration(rec.TargetSnapshot), TargetSnapshotHash: rec.TargetSnapshotHash, RecommendationID: rec.ID, Source: SourceMock, Scenario: ScenarioPlatformConfiguration, Version: 1, CreatedBy: actor.Principal.ID, CreatedAt: now, UpdatedAt: now}
	if rec.TargetSnapshot != nil {
		cs.Scenario = Scenario(rec.TargetSnapshot.Scenario)
	}
	accepted, replay, err := r.AcceptRecommendation(ctx, rec, key, reqHash, cs)
	if err != nil {
		return RecommendationAcceptance{}, false, err
	}
	accepted.ChangeSet, err = s.hydrateChangeSet(ctx, actor.OrganizationID, projectID, accepted.ChangeSet)
	if err != nil {
		return RecommendationAcceptance{}, false, err
	}
	return accepted, replay, nil
}

func recommendationMatchesCurrentPlan(recommendation DeliveryRecommendation, plan DeliveryPlan) bool {
	return recommendation.PlanID == plan.ID && recommendation.PlanVersion == plan.CurrentVersionNumber
}
func (s Service) RejectRecommendation(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, id string, expected int64) (DeliveryRecommendation, error) {
	if err := s.ready(actor, projectID, ScopeWrite); err != nil {
		return DeliveryRecommendation{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return DeliveryRecommendation{}, err
	}
	r, err := s.configurationWorkflow()
	if err != nil {
		return DeliveryRecommendation{}, err
	}
	return r.RejectRecommendation(ctx, actor.OrganizationID, projectID, id, expected, actor.Principal.ID, s.now())
}
func (s Service) CompileManualActionPackage(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, changeSetID string, expectedVersion int64) (ManualActionPackage, bool, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return ManualActionPackage{}, false, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ManualActionPackage{}, false, err
	}
	r, err := s.configurationWorkflow()
	if err != nil {
		return ManualActionPackage{}, false, err
	}
	cs, err := s.Repository.GetChangeSet(ctx, actor.OrganizationID, projectID, changeSetID)
	if err != nil {
		return ManualActionPackage{}, false, err
	}
	if cs.Status != ChangeSetApproved || (cs.LegacyTargetSnapshot == nil && cs.TargetSnapshot == nil) {
		return ManualActionPackage{}, false, ErrInvalidState
	}
	if cs.Version != expectedVersion {
		return ManualActionPackage{}, false, ErrVersionConflict
	}
	plan, err := s.Repository.GetPlan(ctx, actor.OrganizationID, projectID, cs.PlanID)
	if err != nil {
		return ManualActionPackage{}, false, err
	}
	optimizedVersion, alreadyMaterialized, err := optimizedVersionFromChangeSet(plan, cs, actor.Principal, s.now())
	if err != nil {
		return ManualActionPackage{}, false, err
	}
	instructions := manualInstructionsForChangeSet(cs)
	forbiddenActions := []string{"submit", "enable", "budget_expansion", "credentials", "unknown_pages", "platform_api_call", "automatic_execution"}
	evidence := []string{}
	if cs.LegacyTargetSnapshot != nil {
		evidence = append(evidence, cs.LegacyTargetSnapshot.Evidence...)
	} else {
		evidence = append(evidence, cs.TargetSnapshot.CompilationMetadata.EvidenceRefs...)
	}
	payload := struct {
		TargetSnapshotHash   string                    `json:"target_snapshot_hash"`
		ConfigurationID      string                    `json:"configuration_id,omitempty"`
		ConfigurationVersion int                       `json:"configuration_version,omitempty"`
		IntentID             string                    `json:"intent_id,omitempty"`
		IntentVersion        int                       `json:"intent_version,omitempty"`
		IntentCanonicalHash  string                    `json:"intent_canonical_hash,omitempty"`
		Instructions         []ManualActionInstruction `json:"instructions"`
		ForbiddenActions     []string                  `json:"forbidden_actions"`
		Evidence             []string                  `json:"evidence"`
		Provenance           string                    `json:"provenance"`
		OptimizedPlanVersion int                       `json:"optimized_plan_version"`
		OptimizedPlanHash    string                    `json:"optimized_plan_hash"`
		Source               Source                    `json:"source"`
		Scenario             string                    `json:"scenario"`
	}{TargetSnapshotHash: cs.TargetSnapshotHash, Instructions: instructions, ForbiddenActions: forbiddenActions, Evidence: evidence, Provenance: "approved_change_set", OptimizedPlanVersion: optimizedVersion.VersionNumber, OptimizedPlanHash: optimizedVersion.CanonicalHash, Source: SourceMock, Scenario: "manual_action_package"}
	if cs.TargetSnapshot != nil {
		payload.ConfigurationID, payload.ConfigurationVersion = cs.TargetSnapshot.ConfigurationID, cs.TargetSnapshot.VersionNumber
		payload.IntentID, payload.IntentVersion, payload.IntentCanonicalHash = plan.CurrentVersion.DeliveryIntent.IntentID, plan.CurrentVersion.DeliveryIntent.VersionNumber, plan.CurrentVersion.DeliveryIntent.CanonicalHash
	}
	hash, err := contract.CanonicalJSONHash(payload)
	if err != nil {
		return ManualActionPackage{}, false, err
	}
	id, err := s.idGenerator()("manualactionpackage")
	if err != nil {
		return ManualActionPackage{}, false, err
	}
	packageInput := ManualActionPackage{ID: id, OrganizationID: actor.OrganizationID, ProjectID: projectID, ChangeSetID: cs.ID, TargetSnapshotHash: cs.TargetSnapshotHash, ContentHash: hash, Instructions: instructions, ForbiddenActions: forbiddenActions, Evidence: evidence, Provenance: "approved_change_set", OptimizedPlanVersion: optimizedVersion.VersionNumber, OptimizedPlanHash: optimizedVersion.CanonicalHash, Source: SourceMock, Scenario: "manual_action_package", CreatedAt: s.now()}
	if cs.TargetSnapshot != nil {
		configuration, intent := cs.TargetSnapshot, plan.CurrentVersion.DeliveryIntent
		packageInput.ConfigurationSchemaVersion, packageInput.ConfigurationID, packageInput.ConfigurationVersion = configuration.SchemaVersion, configuration.ConfigurationID, configuration.VersionNumber
		packageInput.ConfigurationPlatform, packageInput.ConfigurationProfileVersion, packageInput.ConfigurationCanonicalHash = configuration.Platform, configuration.ProfileVersion, configuration.CanonicalHash
		packageInput.IntentSchemaVersion, packageInput.IntentID, packageInput.IntentVersion, packageInput.IntentCanonicalHash = intent.SchemaVersion, intent.IntentID, intent.VersionNumber, intent.CanonicalHash
	}
	packageValue, replay, err := r.CreateOrGetManualActionPackage(ctx, packageInput)
	if err != nil {
		return ManualActionPackage{}, false, err
	}
	if !alreadyMaterialized {
		if _, err = s.Repository.UpdatePlan(ctx, actor.OrganizationID, projectID, plan.ID, plan.CurrentVersionNumber, optimizedVersion); err != nil {
			return ManualActionPackage{}, false, err
		}
	}
	packageValue.OptimizedPlanVersion = optimizedVersion.VersionNumber
	packageValue.OptimizedPlanHash = optimizedVersion.CanonicalHash
	return packageValue, replay, nil
}

func optimizedVersionFromChangeSet(plan DeliveryPlan, changeSet ChangeSet, actor contract.Principal, now time.Time) (DeliveryPlanVersion, bool, error) {
	if changeSet.TargetSnapshot != nil {
		if plan.CurrentVersion.PlatformConfiguration != nil && plan.CurrentVersion.PlatformConfiguration.CanonicalHash == changeSet.TargetSnapshotHash {
			return plan.CurrentVersion, true, nil
		}
		if int64(plan.CurrentVersionNumber) != changeSet.PlanVersion || !plan.CurrentVersion.IsPlatformConfigurationV2() {
			return DeliveryPlanVersion{}, false, ErrStalePlanVersion
		}
		version := cloneVersion(plan.CurrentVersion)
		version.VersionNumber = plan.CurrentVersionNumber + 1
		version.PlatformConfiguration = cloneJSONPointer(changeSet.TargetSnapshot)
		version.CanonicalHash = changeSet.TargetSnapshotHash
		version.CreatedBy, version.CreatedAt = actor, now
		return version, false, nil
	}
	currentHash, err := snapshotHash(plan.CurrentVersion.ThreeTierConfiguration)
	if err != nil {
		return DeliveryPlanVersion{}, false, err
	}
	if currentHash == changeSet.TargetSnapshotHash {
		return plan.CurrentVersion, true, nil
	}
	if int64(plan.CurrentVersionNumber) != changeSet.PlanVersion {
		return DeliveryPlanVersion{}, false, ErrStalePlanVersion
	}
	version := cloneVersion(plan.CurrentVersion)
	version.VersionNumber = plan.CurrentVersionNumber + 1
	version.ThreeTierConfiguration = cloneThreeTierConfiguration(changeSet.LegacyTargetSnapshot)
	version.Scenario = Scenario(changeSet.LegacyTargetSnapshot.Scenario)
	version.CreatedBy = actor
	version.CreatedAt = now
	if budget := findThreeTierField(version.ThreeTierConfiguration, "budget"); budget != nil {
		version.Budget.TotalMinor = numericMinor(budget.Effective.Value, version.Budget.TotalMinor)
	}
	version.CanonicalHash, err = PlanCanonicalHash(version)
	if err != nil {
		return DeliveryPlanVersion{}, false, err
	}
	return version, false, nil
}
func (s Service) GetManualActionPackage(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, changeSetID string) (ManualActionPackage, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return ManualActionPackage{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ManualActionPackage{}, err
	}
	r, err := s.configurationWorkflow()
	if err != nil {
		return ManualActionPackage{}, err
	}
	value, err := r.GetManualActionPackage(ctx, actor.OrganizationID, projectID, changeSetID)
	if err != nil || value.OptimizedPlanVersion > 0 {
		return value, err
	}
	changeSet, err := s.Repository.GetChangeSet(ctx, actor.OrganizationID, projectID, changeSetID)
	if err != nil {
		return ManualActionPackage{}, err
	}
	plan, err := s.Repository.GetPlan(ctx, actor.OrganizationID, projectID, changeSet.PlanID)
	if err != nil {
		return ManualActionPackage{}, err
	}
	if plan.CurrentVersion.PlatformConfiguration != nil && plan.CurrentVersion.PlatformConfiguration.CanonicalHash == changeSet.TargetSnapshotHash {
		value.OptimizedPlanVersion = plan.CurrentVersionNumber
		value.OptimizedPlanHash = plan.CurrentVersion.CanonicalHash
	} else if currentHash, hashErr := snapshotHash(plan.CurrentVersion.ThreeTierConfiguration); hashErr == nil && currentHash == changeSet.TargetSnapshotHash {
		value.OptimizedPlanVersion = plan.CurrentVersionNumber
		value.OptimizedPlanHash = plan.CurrentVersion.CanonicalHash
	} else if optimized, _, versionErr := optimizedVersionFromChangeSet(plan, changeSet, actor.Principal, s.now()); versionErr == nil {
		value.OptimizedPlanVersion = optimized.VersionNumber
		value.OptimizedPlanHash = optimized.CanonicalHash
	}
	return value, nil
}
func manualInstructions(c *ThreeTierConfiguration) []ManualActionInstruction {
	out := []ManualActionInstruction{}
	for _, g := range c.Groups {
		for _, f := range g.Fields {
			out = append(out, manualInstruction("group", g.ID, "", "", f))
		}
		for _, p := range g.Plans {
			for _, f := range p.Fields {
				out = append(out, manualInstruction("plan", g.ID, p.ID, "", f))
			}
			for _, cr := range p.Creatives {
				for _, f := range cr.Fields {
					out = append(out, manualInstruction("creative", g.ID, p.ID, cr.ID, f))
				}
			}
		}
	}
	return out
}

func manualInstructionsForChangeSet(changeSet ChangeSet) []ManualActionInstruction {
	if changeSet.TargetSnapshot != nil {
		return platformManualInstructions(changeSet.TargetSnapshot)
	}
	if changeSet.LegacyTargetSnapshot != nil {
		return manualInstructions(changeSet.LegacyTargetSnapshot)
	}
	return []ManualActionInstruction{}
}

func platformManualInstructions(configuration *PlatformConfiguration) []ManualActionInstruction {
	if configuration == nil || configuration.Payload.OceanEngine == nil || configuration.Payload.OceanEngine.Project == nil {
		return []ManualActionInstruction{}
	}
	project := configuration.Payload.OceanEngine.Project
	return []ManualActionInstruction{{
		Layer: "project", PlanID: project.ProjectDraftID, FieldKey: "daily_budget_minor",
		Effective: ThreeTierValue{Type: "integer", Value: project.BudgetAndBidding.DailyBudgetMinor},
		Source:    "recommendation", ConfirmationRequired: true,
		ExpectedResult: "在不提交、不启用投放的前提下，按人工复核值填写",
		EvidenceRefs:   append([]string(nil), configuration.CompilationMetadata.EvidenceRefs...),
	}}
}
func manualInstruction(layer, groupID, planID, creativeID string, f ThreeTierField) ManualActionInstruction {
	return ManualActionInstruction{Layer: layer, GroupID: groupID, PlanID: planID, CreativeID: creativeID, FieldKey: f.Key, Effective: f.Effective, Source: f.Source, ConfirmationRequired: !f.Confirmed, ExpectedResult: "在不提交、不启用投放的前提下，按人工复核值填写", EvidenceRefs: append([]string(nil), f.EvidenceRefs...)}
}
func compileThreeTierFixture(version DeliveryPlanVersion, fixture ThreeTierFixture) (*ThreeTierConfiguration, error) {
	if fixture == "" {
		fixture = ThreeTierFixtureGoldenPath
	}
	switch fixture {
	case ThreeTierFixtureGoldenPath, ThreeTierFixtureMissingRequiredField, ThreeTierFixtureOrphanDependency, ThreeTierFixtureMissingConfirmation, ThreeTierFixturePlatformFieldsPending:
	default:
		return nil, ErrInvalidRequest
	}
	groupFields := []ThreeTierField{
		mockThreeTierField("group_name", "广告组名称", "string", version.Name, false, false),
		mockThreeTierField("group_objective", "营销目标", "string", version.Objective, false, false),
		mockThreeTierField("advertiser", "广告主", "string", version.Advertiser.Name, false, false),
		mockThreeTierField("business_asset_boundary", "业务资产边界", "string", "current_project_only", false, true),
	}
	planFields := func(name string, budget int64) []ThreeTierField {
		return []ThreeTierField{
			mockThreeTierField("plan_name", "广告计划名称", "string", name, false, false, "field:group_name"),
			mockThreeTierField("placement", "版位", "string", "platform_pending", false, true, "field:group_objective"),
			mockThreeTierField("optimization", "优化目标", "string", "platform_pending", false, true, "field:group_objective"),
			mockThreeTierField("audience", "受众", "string", "project_mock_audience", false, true, "field:group_objective"),
			mockThreeTierField("budget", "计划预算", "integer", budget, false, false, "field:group_objective"),
			mockThreeTierField("bid", "出价", "integer", max(budget/100, 1), false, true, "field:budget"),
			mockThreeTierField("schedule", "投放排期", "string", version.Schedule.Timezone, false, false, "field:budget"),
			mockThreeTierField("conversion", "转化目标", "string", version.Tracking.ConversionEvent, false, true, "field:optimization"),
			mockThreeTierField("tracking", "追踪标识", "string", version.Tracking.PixelID, false, true, "field:conversion"),
		}
	}
	creativeFields := func(name, assetID, title string) []ThreeTierField {
		return []ThreeTierField{
			mockThreeTierField("creative_name", "创意名称", "string", name, false, false, "field:plan_name"),
			mockThreeTierField("asset_version", "素材版本", "string", assetID, false, false, "field:creative_name"),
			mockThreeTierField("title", "标题", "string", title, true, false, "field:asset_version"),
			mockThreeTierField("format", "创意格式", "string", "mock_image_text", false, true, "field:asset_version"),
			mockThreeTierField("landing_page", "落地页", "string", version.Tracking.LandingPage, true, true, "field:tracking"),
			mockThreeTierField("call_to_action", "行动按钮", "string", "platform_pending", true, true, "field:landing_page"),
			mockThreeTierField("review_status", "审核状态", "string", "not_submitted", false, true, "field:format"),
			mockThreeTierField("disclosure", "披露信息", "string", "manual_review_required", true, true, "field:review_status"),
		}
	}
	asset1 := "mock-asset-1@v1"
	if len(version.CreativeReferences) > 0 {
		asset1 = fmt.Sprintf("%s@v%d", version.CreativeReferences[0].AssetID, version.CreativeReferences[0].Version)
	}
	c := &ThreeTierConfiguration{
		Schema: "delivery-three-tier/v1", Source: SourceMock, Scenario: fixture, FixtureScenario: fixture,
		Evidence: []string{"mock://delivery-three-tier/v1", "mock://current-project/plan-version"},
		Groups: []ThreeTierGroup{{
			ID: "group_1", Name: "项目广告组", Fields: groupFields,
			Plans: []ThreeTierPlan{
				{ID: "plan_1", Name: "获客计划", Fields: planFields("获客计划", version.Budget.TotalMinor), Creatives: []ThreeTierCreative{
					{ID: "creative_1", Name: "主创意", Fields: creativeFields("主创意", asset1, "项目主标题")},
					{ID: "creative_2", Name: "备选创意", Fields: creativeFields("备选创意", "mock-asset-2@v1", "项目备选标题")},
				}},
				{ID: "plan_2", Name: "再营销计划", Fields: planFields("再营销计划", max(version.Budget.TotalMinor/2, 1)), Creatives: []ThreeTierCreative{
					{ID: "creative_3", Name: "再营销创意", Fields: creativeFields("再营销创意", "mock-asset-3@v1", "项目再营销标题")},
				}},
			},
		}},
	}
	f := &c.Groups[0].Plans[0].Creatives[0].Fields[0]
	switch fixture {
	case ThreeTierFixtureMissingRequiredField:
		f.Key = ""
	case ThreeTierFixtureOrphanDependency:
		f.Dependency = "field:missing"
	case ThreeTierFixtureMissingConfirmation:
		f.Confirmed = false
	case ThreeTierFixturePlatformFieldsPending:
		f.PlatformRequired = true
		f.PlatformStatus = "pending"
	}
	return c, nil
}

func mockThreeTierField(key, label, valueType string, value any, editable, platformPending bool, dependencies ...string) ThreeTierField {
	status := "not_requested"
	if platformPending {
		status = "pending"
	}
	return ThreeTierField{
		Key: key, Label: label,
		Recommended: ThreeTierValue{Type: valueType, Value: value},
		Effective:   ThreeTierValue{Type: valueType, Value: value},
		Source:      "mock_fixture", SourceRefs: []string{"mock://delivery-three-tier/v1/" + key}, EffectiveSource: "recommended",
		DependencyRefs: append([]string(nil), dependencies...),
		Risk:           "manual_platform_review", RiskRefs: []string{"risk://manual-platform-review"},
		EvidenceRefs: []string{"mock://evidence/" + key}, MockRequired: true,
		PlatformRequired: platformPending, PlatformStatus: status, Editable: editable, Confirmed: true,
	}
}

func findThreeTierField(configuration *ThreeTierConfiguration, key string) *ThreeTierField {
	if configuration == nil {
		return nil
	}
	for groupIndex := range configuration.Groups {
		group := &configuration.Groups[groupIndex]
		for fieldIndex := range group.Fields {
			if group.Fields[fieldIndex].Key == key {
				return &group.Fields[fieldIndex]
			}
		}
		for planIndex := range group.Plans {
			plan := &group.Plans[planIndex]
			for fieldIndex := range plan.Fields {
				if plan.Fields[fieldIndex].Key == key {
					return &plan.Fields[fieldIndex]
				}
			}
			for creativeIndex := range plan.Creatives {
				creative := &plan.Creatives[creativeIndex]
				for fieldIndex := range creative.Fields {
					if creative.Fields[fieldIndex].Key == key {
						return &creative.Fields[fieldIndex]
					}
				}
			}
		}
	}
	return nil
}
func (r ThreeTierOverrideRequest) String() string {
	return fmt.Sprintf("%s/%s/%s/%s", r.GroupID, r.PlanID, r.CreativeID, r.FieldKey)
}
