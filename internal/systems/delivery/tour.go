package delivery

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type TourRunStatus string

const (
	TourRunPreparing TourRunStatus = "preparing"
	TourRunPrepared  TourRunStatus = "prepared"
	TourRunReset     TourRunStatus = "reset"
)

const (
	TourCaseGoldenPath       = "golden_path"
	TourCasePreflightFailure = "preflight_failure"
	TourCaseApprovalExpired  = "approval_expired"
	TourCasePlanStale        = "plan_stale"
	TourCasePartialExecution = "partial_execution"
	TourCaseResultUnknown    = "result_unknown"
	TourCaseReviewRejected   = "review_rejected_alert"
)

var tourRunIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{2,63}$`)

type DeliveryTourRun struct {
	ID               string                  `json:"id"`
	OrganizationID   contract.OrganizationID `json:"organization_id"`
	ProjectID        contract.ProjectID      `json:"project_id"`
	OwnerID          string                  `json:"owner_id"`
	Status           TourRunStatus           `json:"status"`
	Source           Source                  `json:"source"`
	Scenario         string                  `json:"scenario"`
	PreparedAt       *time.Time              `json:"prepared_at"`
	ResetAt          *time.Time              `json:"reset_at"`
	CreatedAt        time.Time               `json:"created_at"`
	UpdatedAt        time.Time               `json:"updated_at"`
	Cases            []DeliveryTourCase      `json:"cases"`
	Steps            []DeliveryTourStep      `json:"steps"`
	CurrentStep      string                  `json:"current_step"`
	SuggestedNextURL string                  `json:"suggested_next_url"`
}

type DeliveryTourCase struct {
	Key             string    `json:"key"`
	Title           string    `json:"title"`
	PlanID          string    `json:"plan_id"`
	Status          string    `json:"status"`
	ExpectedOutcome string    `json:"expected_outcome"`
	StartURL        string    `json:"start_url"`
	Source          Source    `json:"source"`
	Scenario        string    `json:"scenario"`
	Evidence        []string  `json:"evidence"`
	ObservedAt      time.Time `json:"observed_at"`
}

type DeliveryTourStep struct {
	Key                 string   `json:"key"`
	Title               string   `json:"title"`
	CompletionCondition string   `json:"completion_condition"`
	Complete            bool     `json:"complete"`
	URL                 string   `json:"url"`
	Explanation         string   `json:"explanation"`
	Evidence            []string `json:"evidence"`
}

type DeliveryTourResetResult struct {
	Run          DeliveryTourRun  `json:"run"`
	Deleted      map[string]int64 `json:"deleted"`
	Source       Source           `json:"source"`
	Scenario     string           `json:"scenario"`
	ResetAt      time.Time        `json:"reset_at"`
	IsolationKey string           `json:"isolation_key"`
}

type deliveryTourRepository interface {
	CreateOrGetTourRun(context.Context, DeliveryTourRun) (DeliveryTourRun, bool, error)
	GetTourRun(context.Context, contract.OrganizationID, contract.ProjectID, string) (DeliveryTourRun, error)
	SetTourRunStatus(context.Context, contract.OrganizationID, contract.ProjectID, string, string, TourRunStatus, time.Time) (DeliveryTourRun, error)
	ListTourPlans(context.Context, contract.OrganizationID, contract.ProjectID, string, string) ([]DeliveryPlan, error)
	ListTourPlanChangeSets(context.Context, contract.OrganizationID, contract.ProjectID, string) ([]ChangeSet, error)
	ListTourPlanExecutions(context.Context, contract.OrganizationID, contract.ProjectID, string) ([]ExecutionResult, error)
	ListTourPlanAlerts(context.Context, contract.OrganizationID, contract.ProjectID, string) ([]DeliveryAlert, error)
	ListTourPlanRecommendations(context.Context, contract.OrganizationID, contract.ProjectID, string) ([]DeliveryRecommendation, error)
	ResetTourRun(context.Context, contract.OrganizationID, contract.ProjectID, string, string, time.Time) (map[string]int64, DeliveryTourRun, error)
}

func (s Service) tourRepository() (deliveryTourRepository, error) {
	r, ok := s.Repository.(deliveryTourRepository)
	if !ok {
		return nil, ErrUnsupportedTour
	}
	return r, nil
}

func (s Service) PrepareTourRun(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, runID string) (DeliveryTourRun, bool, error) {
	if err := validateTourAccess(s, actor, projectID); err != nil {
		return DeliveryTourRun{}, false, err
	}
	runID = strings.TrimSpace(runID)
	if !tourRunIDPattern.MatchString(runID) {
		return DeliveryTourRun{}, false, ErrInvalidRequest
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return DeliveryTourRun{}, false, err
	}
	r, err := s.tourRepository()
	if err != nil {
		return DeliveryTourRun{}, false, err
	}
	now := s.now()
	requested := DeliveryTourRun{ID: runID, OrganizationID: actor.OrganizationID, ProjectID: projectID, OwnerID: actor.Principal.ID, Status: TourRunPreparing, Source: SourceMock, Scenario: "delivery_tour", CreatedAt: now, UpdatedAt: now}
	stored, replay, err := r.CreateOrGetTourRun(ctx, requested)
	if err != nil {
		return DeliveryTourRun{}, false, err
	}
	if stored.OwnerID != actor.Principal.ID {
		return DeliveryTourRun{}, false, ErrTourOwnerMismatch
	}
	if stored.Status == TourRunPrepared {
		hydrated, hydrateErr := s.GetTourRun(ctx, actor, projectID, runID)
		return hydrated, true, hydrateErr
	}
	if replay && stored.Status == TourRunPreparing {
		if _, _, err := r.ResetTourRun(ctx, actor.OrganizationID, projectID, runID, actor.Principal.ID, now); err != nil {
			return DeliveryTourRun{}, false, err
		}
		if _, err := r.SetTourRunStatus(ctx, actor.OrganizationID, projectID, runID, actor.Principal.ID, TourRunPreparing, now); err != nil {
			return DeliveryTourRun{}, false, err
		}
	}

	if err := s.prepareTourCases(ctx, actor, projectID, runID, now); err != nil {
		return DeliveryTourRun{}, false, err
	}
	if _, err := r.SetTourRunStatus(ctx, actor.OrganizationID, projectID, runID, actor.Principal.ID, TourRunPrepared, s.now()); err != nil {
		return DeliveryTourRun{}, false, err
	}
	hydrated, err := s.GetTourRun(ctx, actor, projectID, runID)
	return hydrated, false, err
}

func validateTourAccess(s Service, actor contract.ActorContext, projectID contract.ProjectID) error {
	for _, scope := range []contract.Scope{ScopeRead, ScopeWrite, ScopeApprove, ScopeExecute} {
		if err := s.ready(actor, projectID, scope); err != nil {
			return err
		}
	}
	return nil
}

func (s Service) prepareTourCases(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, runID string, now time.Time) error {
	if _, err := s.createPlan(ctx, actor, projectID, tourPlanRequest(runID, TourCaseGoldenPath, now), runID, TourCaseGoldenPath); err != nil {
		return err
	}

	preflightPlan, err := s.createPlan(ctx, actor, projectID, tourPlanRequest(runID, TourCasePreflightFailure, now), runID, TourCasePreflightFailure)
	if err != nil {
		return err
	}
	preflightChangeSet, err := s.CreateChangeSet(ctx, actor, projectID, preflightPlan.ID, preflightPlan.Version)
	if err != nil {
		return err
	}
	if _, err = s.Preflight(ctx, actor, projectID, preflightChangeSet.ID, preflightChangeSet.Version); err != nil {
		return err
	}

	past := s
	past.Now = func() time.Time { return now.Add(-25 * time.Hour) }
	expiredPlan, err := past.createPlan(ctx, actor, projectID, tourPlanRequest(runID, TourCaseApprovalExpired, now), runID, TourCaseApprovalExpired)
	if err != nil {
		return err
	}
	if _, err = prepareApprovedTourPlan(ctx, past, actor, projectID, expiredPlan); err != nil {
		return err
	}

	stalePlan, err := s.createPlan(ctx, actor, projectID, tourPlanRequest(runID, TourCasePlanStale, now), runID, TourCasePlanStale)
	if err != nil {
		return err
	}
	if _, err = prepareApprovedTourPlan(ctx, s, actor, projectID, stalePlan); err != nil {
		return err
	}
	staleIntent := *cloneJSONPointer(stalePlan.CurrentVersion.DeliveryIntent)
	staleIntent.VersionNumber++
	staleIntent.Payload.MarketingObjective += "；当前版本已经更新，旧审批必须失效"
	staleIntent.CanonicalHash = ""
	staleConfiguration := *cloneJSONPointer(stalePlan.CurrentVersion.PlatformConfiguration)
	staleConfiguration.VersionNumber++
	staleConfiguration.Intent = IntentBinding{}
	staleConfiguration.CanonicalHash = ""
	if _, err = s.UpdatePlan(ctx, actor, projectID, stalePlan.ID, UpdatePlanRequest{ExpectedVersion: int(stalePlan.Version), Intent: &staleIntent, PlatformConfiguration: &staleConfiguration}); err != nil {
		return err
	}

	for _, executionCase := range []struct {
		key      string
		scenario ExecutionScenario
	}{{TourCasePartialExecution, ExecutionScenarioPartial}, {TourCaseResultUnknown, ExecutionScenarioResultUnknown}} {
		plan, createErr := s.createPlan(ctx, actor, projectID, tourPlanRequest(runID, executionCase.key, now), runID, executionCase.key)
		if createErr != nil {
			return createErr
		}
		approved, approveErr := prepareApprovedTourPlan(ctx, s, actor, projectID, plan)
		if approveErr != nil {
			return approveErr
		}
		if _, _, executeErr := s.Execute(ctx, actor, projectID, approved.ID, fmt.Sprintf("tour-%s-%s", runID, executionCase.key), ExecuteRequest{ExpectedVersion: approved.Version, Scenario: executionCase.scenario}); executeErr != nil {
			return executeErr
		}
	}

	// Prepare only the prerequisites for this isolated exception. Its rejection
	// event and alert are materialized lazily when the reviewer runs the scenario,
	// so neither this case nor the golden monitoring page starts with a warning.
	reviewPlan, err := s.createPlan(ctx, actor, projectID, tourPlanRequest(runID, TourCaseReviewRejected, now), runID, TourCaseReviewRejected)
	if err != nil {
		return err
	}
	reviewChangeSet, err := prepareApprovedTourPlan(ctx, s, actor, projectID, reviewPlan)
	if err != nil {
		return err
	}
	_, _, err = s.Execute(ctx, actor, projectID, reviewChangeSet.ID, fmt.Sprintf("tour-%s-%s", runID, TourCaseReviewRejected), ExecuteRequest{
		ExpectedVersion: reviewChangeSet.Version,
		Scenario:        ExecutionScenarioSuccess,
	})
	return err
}

func prepareApprovedTourPlan(ctx context.Context, s Service, actor contract.ActorContext, projectID contract.ProjectID, plan DeliveryPlan) (ChangeSet, error) {
	changeSet, err := s.CreateChangeSet(ctx, actor, projectID, plan.ID, plan.Version)
	if err != nil {
		return ChangeSet{}, err
	}
	changeSet, err = s.Preflight(ctx, actor, projectID, changeSet.ID, changeSet.Version)
	if err != nil {
		return ChangeSet{}, err
	}
	return s.Approve(ctx, actor, projectID, changeSet.ID, changeSet.Version)
}

func tourPlanRequest(runID, tourCase string, now time.Time) CreatePlanRequest {
	labels := map[string]string{
		TourCaseGoldenPath:       "黄金路径",
		TourCasePreflightFailure: "预检失败",
		TourCaseApprovalExpired:  "审批过期",
		TourCasePlanStale:        "计划版本过期",
		TourCasePartialExecution: "执行部分成功",
		TourCaseResultUnknown:    "执行结果未知",
		TourCaseReviewRejected:   "审核拒绝告警",
	}
	start := now.UTC().Truncate(24 * time.Hour).Add(7 * 24 * time.Hour)
	identity := runID + "-" + tourCase
	ref := func(kind, id string) StableReference {
		return StableReference{Namespace: "cookies", ObjectKind: kind, Scope: "tour:" + runID, ID: id, Version: "v1", ContentHash: strings.Repeat("a", 64), State: ReferenceResolved}
	}
	material := ref("asset_version", "asset_demo_investor_creative_video")
	intent, _ := FinalizeDeliveryIntent(DeliveryIntent{
		SchemaVersion: DeliveryIntentSchemaV1, IntentID: "intent-" + identity, VersionNumber: 1, HashAlgorithm: CanonicalPayloadHashAlgorithm,
		Payload:                 DeliveryIntentPayload{PayloadSchemaVersion: DeliveryIntentSchemaV1, MarketingObjective: "验证计划来源、审批、平台操作演练、上线后指标与优化证据链。", BudgetBoundary: IntentBudgetBoundary{Currency: "CNY", MinimumTotalMinor: 0, MaximumTotalMinor: 3000000}, ScheduleBoundary: IntentScheduleBoundary{EarliestStart: start, LatestEnd: start.Add(14 * 24 * time.Hour), Timezone: "Asia/Shanghai"}, OptimizationPreferences: []OptimizationPreference{}, MaterialReferences: []StableReference{material}, AudienceConstraints: IntentAudienceConstraints{}, StrategyReference: ref("strategy_version", "task_demo_precision_strategy")},
		ConfigurationProvenance: ConfigurationProvenance{Kind: ConfigurationGeneratedManually, GeneratorRef: "delivery-tour"}, FactProvenance: FactProvenance{Source: FactSourceMock, SnapshotRef: "mock://tour/" + identity}, Audit: ContractAuditMetadata{CreatedBy: "delivery-tour", CreatedAt: now},
	})
	fieldEvidence := []PlatformFieldEvidence{{Field: "project", State: PlatformEvidenceOperatorReviewed}}
	if tourCase == TourCasePreflightFailure {
		fieldEvidence = []PlatformFieldEvidence{{Field: "project.account_reference", State: PlatformEvidencePending, Reason: "tour preflight failure"}}
	}
	configuration, _ := FinalizePlatformConfiguration(PlatformConfiguration{
		SchemaVersion: PlatformConfigurationSchemaV2, ConfigurationID: "configuration-" + identity, VersionNumber: 1, Platform: DeliveryPlatformOceanEngine, ProfileVersion: OceanEngineConfigurationProfileV1,
		Intent: IntentBinding{SchemaVersion: intent.SchemaVersion, IntentID: intent.IntentID, VersionNumber: intent.VersionNumber, CanonicalHash: intent.CanonicalHash}, HashAlgorithm: CanonicalPayloadHashAlgorithm,
		Payload:                 PlatformConfigurationPayload{Profile: DeliveryPlatformOceanEngine, OceanEngine: &OceanEngineConfiguration{Profile: DeliveryPlatformOceanEngine, Project: &OceanEngineProjectDraft{DraftSchemaVersion: OceanEngineConfigurationProfileV1, ProjectDraftID: "project-" + identity, AccountReference: ref("advertiser_account", "mock-tour-advertiser"), MarketingPurpose: "lead_generation", MarketingScenario: "manual_delivery", Carrier: "landing_page", DeliveryMode: "manual", Targeting: OceanEngineTargeting{SmartExpansion: false}, Schedule: OceanEngineSchedule{StartAt: start, EndAt: start.Add(14 * 24 * time.Hour), Timezone: "Asia/Shanghai"}, BudgetAndBidding: OceanEngineBudgetAndBidding{Currency: "CNY", DailyBudgetMinor: 200000, BiddingStrategy: "manual_bid", ChargingMode: "CPC", BidMinor: int64Pointer(100)}, ProjectName: fmt.Sprintf("上线后优化闭环 · %s · %s", labels[tourCase], runID)}, Promotions: []OceanEnginePromotionDraft{{DraftSchemaVersion: OceanEngineConfigurationProfileV1, PromotionDraftID: "promotion-" + identity, DeliveryIdentity: OceanEngineDeliveryIdentity{Mode: "account_info"}, BaseMaterialReferences: []StableReference{material}, CopyItems: []OceanEngineCopyItem{{Text: "tour copy"}}, PromotionName: "Tour promotion"}}}},
		ConfigurationProvenance: ConfigurationProvenance{Kind: ConfigurationGeneratedManually, GeneratorRef: "delivery-tour"}, FactProvenance: FactProvenance{Source: FactSourceMock, SnapshotRef: "mock://tour/" + identity}, Audit: ContractAuditMetadata{CreatedBy: "delivery-tour", CreatedAt: now}, CompilationMetadata: CompilationMetadata{FieldEvidence: fieldEvidence, EvidenceRefs: []string{"mock://tour/" + identity}},
	})
	return CreatePlanRequest{Intent: &intent, PlatformConfiguration: &configuration}
}

func int64Pointer(value int64) *int64 { return &value }

func (s Service) GetTourRun(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, runID string) (DeliveryTourRun, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return DeliveryTourRun{}, err
	}
	runID = strings.TrimSpace(runID)
	if !tourRunIDPattern.MatchString(runID) {
		return DeliveryTourRun{}, ErrInvalidRequest
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return DeliveryTourRun{}, err
	}
	r, err := s.tourRepository()
	if err != nil {
		return DeliveryTourRun{}, err
	}
	run, err := r.GetTourRun(ctx, actor.OrganizationID, projectID, runID)
	if err != nil {
		return DeliveryTourRun{}, err
	}
	if run.OwnerID != actor.Principal.ID {
		return DeliveryTourRun{}, ErrTourOwnerMismatch
	}
	plans, err := r.ListTourPlans(ctx, actor.OrganizationID, projectID, run.ID, run.OwnerID)
	if err != nil {
		return DeliveryTourRun{}, err
	}
	return s.hydrateTourRun(ctx, actor, run, plans)
}

func (s Service) ResetTourRun(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, runID string) (DeliveryTourResetResult, error) {
	if err := validateTourAccess(s, actor, projectID); err != nil {
		return DeliveryTourResetResult{}, err
	}
	runID = strings.TrimSpace(runID)
	if !tourRunIDPattern.MatchString(runID) {
		return DeliveryTourResetResult{}, ErrInvalidRequest
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return DeliveryTourResetResult{}, err
	}
	r, err := s.tourRepository()
	if err != nil {
		return DeliveryTourResetResult{}, err
	}
	now := s.now()
	deleted, run, err := r.ResetTourRun(ctx, actor.OrganizationID, projectID, runID, actor.Principal.ID, now)
	if err != nil {
		return DeliveryTourResetResult{}, err
	}
	hydrated, err := s.hydrateTourRun(ctx, actor, run, nil)
	if err != nil {
		return DeliveryTourResetResult{}, err
	}
	return DeliveryTourResetResult{Run: hydrated, Deleted: deleted, Source: SourceMock, Scenario: "delivery_tour_reset", ResetAt: now, IsolationKey: fmt.Sprintf("%s/%s/%s/%s", actor.OrganizationID, projectID, run.ID, actor.Principal.ID)}, nil
}

func (s Service) hydrateTourRun(ctx context.Context, actor contract.ActorContext, run DeliveryTourRun, plans []DeliveryPlan) (DeliveryTourRun, error) {
	byCase := make(map[string]DeliveryPlan, len(plans))
	for _, plan := range plans {
		byCase[plan.TourCase] = plan
	}
	definitions := []struct{ key, title, expected, view string }{
		{TourCaseGoldenPath, "黄金路径", "完整走到既有人工操作包", "配置映射"},
		{TourCasePreflightFailure, "预检失败", "必填字段被服务端阻断并给出修复入口", "检查与提交"},
		{TourCaseApprovalExpired, "审批过期", "过期审批明确阻止执行", "检查与提交"},
		{TourCasePlanStale, "计划版本过期", "旧审批失效并要求重新预检和审批", "检查与提交"},
		{TourCasePartialExecution, "执行部分成功", "展示完成、未完成步骤和补偿候选", "检查与提交"},
		{TourCaseResultUnknown, "执行结果未知", "禁止盲目重试并要求查询与重新识别", "检查与提交"},
		{TourCaseReviewRejected, "审核拒绝告警", "展示窗口、证据和受控处置", "审核拒绝"},
	}
	run.Cases = make([]DeliveryTourCase, 0, len(definitions))
	for _, definition := range definitions {
		plan := byCase[definition.key]
		caseStatus, evidence := s.tourCaseEvidence(ctx, actor, run, definition.key, plan)
		nav := "configuration"
		if definition.key == TourCaseReviewRejected {
			nav = "monitoring"
		}
		startURL := tourPageURL(run.ProjectID, nav, run.ID, definition.key, plan.ID, definition.view)
		run.Cases = append(run.Cases, DeliveryTourCase{Key: definition.key, Title: definition.title, PlanID: plan.ID, Status: caseStatus, ExpectedOutcome: definition.expected, StartURL: startURL, Source: SourceMock, Scenario: definition.key, Evidence: evidence, ObservedAt: s.now()})
	}
	run.Steps = s.goldenTourSteps(ctx, actor, run, byCase[TourCaseGoldenPath])
	run.CurrentStep = "complete"
	for _, step := range run.Steps {
		if !step.Complete {
			run.CurrentStep, run.SuggestedNextURL = step.Key, step.URL
			break
		}
	}
	return run, nil
}

func (s Service) tourCaseEvidence(ctx context.Context, actor contract.ActorContext, run DeliveryTourRun, key string, plan DeliveryPlan) (string, []string) {
	if plan.ID == "" {
		return "missing", []string{}
	}
	switch key {
	case TourCasePreflightFailure:
		result, err := s.RunPlanPreflight(ctx, actor, run.ProjectID, plan.ID)
		if err == nil && result.Blocked {
			out := []string{fmt.Sprintf("checked_at=%s", result.CheckedAt.Format(time.RFC3339))}
			for _, check := range result.Checks {
				if !check.Passed {
					out = append(out, check.Code+":"+check.Message)
				}
			}
			return "observed", out
		}
	case TourCaseApprovalExpired, TourCasePlanStale:
		repository, repositoryErr := s.tourRepository()
		if repositoryErr != nil {
			break
		}
		changeSets, err := repository.ListTourPlanChangeSets(ctx, actor.OrganizationID, run.ProjectID, plan.ID)
		if err != nil {
			break
		}
		for _, changeSet := range changeSets {
			hydrated, hydrateErr := s.hydrateChangeSet(ctx, actor.OrganizationID, run.ProjectID, changeSet)
			if hydrateErr == nil && hydrated.Approval != nil && !hydrated.Approval.Valid {
				return "observed", []string{"approval=" + hydrated.Approval.InvalidReason, "change_set=" + hydrated.ID}
			}
		}
	case TourCasePartialExecution, TourCaseResultUnknown:
		repository, repositoryErr := s.tourRepository()
		if repositoryErr == nil {
			executions, err := repository.ListTourPlanExecutions(ctx, actor.OrganizationID, run.ProjectID, plan.ID)
			if err == nil && len(executions) > 0 {
				execution := executions[0].Execution
				return "observed", []string{"execution=" + execution.ID, "status=" + string(execution.Status), "recovery=" + execution.RecoveryAction}
			}
		}
	case TourCaseReviewRejected:
		repository, repositoryErr := s.tourRepository()
		if repositoryErr == nil {
			alerts, err := repository.ListTourPlanAlerts(ctx, actor.OrganizationID, run.ProjectID, plan.ID)
			if err != nil {
				break
			}
			for _, alert := range alerts {
				if alert.Type == AlertReviewRejected {
					return "observed", []string{"alert=" + alert.ID, "window=" + alert.Window.Start.Format(time.RFC3339) + "/" + alert.Window.End.Format(time.RFC3339), "evidence=" + strings.Join(alert.EvidenceRefs, ",")}
				}
			}
		}
	case TourCaseGoldenPath:
		return "ready", []string{"plan=" + plan.ID, "source=mock", "scenario=golden_path"}
	}
	return "prepared", []string{"plan=" + plan.ID}
}

func (s Service) goldenTourSteps(ctx context.Context, actor contract.ActorContext, run DeliveryTourRun, plan DeliveryPlan) []DeliveryTourStep {
	base := func(nav, view string) string {
		return tourPageURL(run.ProjectID, nav, run.ID, TourCaseGoldenPath, plan.ID, view)
	}
	steps := []DeliveryTourStep{
		{Key: "plan_creation", Title: "第 0 步：核对本次运行的投放计划", CompletionCondition: "计划已绑定当前运行，且策略任务与素材版本均可返回上游核对", URL: base("plans", "计划列表"), Explanation: "准备完整数据时，服务端会为当前运行创建并绑定计划，因此准备后此步默认完成；这里负责核对策略与素材来源。"},
		{Key: "configuration", Title: "核对平台配置并完成草稿检查", CompletionCondition: "当前不可变 PlanVersion 包含 DeliveryIntent 与 tagged PlatformConfiguration", URL: base("configuration", "配置映射"), Explanation: "草稿检查即时发现问题，不产生审批记录。"},
		{Key: "first_approval", Title: "提交首个变更申请并前往审批中心", CompletionCondition: "首个变更申请通过最终检查并在审批中心形成有效批准", URL: base("configuration", "检查与提交"), Explanation: "配置页面只负责提交；批准或打回统一在审批中心完成，本次批准只授权平台操作演练。"},
		{Key: "execution", Title: "运行平台操作演练", CompletionCondition: "成功 Execution、Step 和 Evidence 已持久化", URL: base("approvals", "待我审批"), Explanation: "验证平台操作链和审批边界，不预测真实投放效果。"},
		{Key: "monitoring", Title: "运行投放效果情景模拟并生成告警", CompletionCondition: "同一 SimulationRun 的三段指标窗口已产生可追溯告警", URL: base("monitoring", "全部告警"), Explanation: "显式选择情景与稳定 seed；规则模型先产出指标和事件，告警规则再读取同一运行的指标。"},
		{Key: "recommendation", Title: "在优化中心根据指标与告警生成建议", CompletionCondition: "建议明确引用 SimulationRun、Execution、指标窗口和告警", URL: base("optimization", "待处理建议"), Explanation: "没有同一投后演练的完整证据链就不能生成建议。"},
		{Key: "new_change_set", Title: "在优化中心采纳建议并生成优化草稿", CompletionCondition: "建议只关联一个新的 draft 变更申请", URL: base("optimization", "待处理建议"), Explanation: "采纳只起草修改；随后前往内部配置编排检查并提交，不自动应用。"},
		{Key: "second_approval", Title: "提交优化申请并前往审批中心", CompletionCondition: "优化变更申请在审批中心形成有效批准", URL: base("configuration", "检查与提交"), Explanation: "配置页面只负责提交；第二次批准针对新的优化写入，不是重复审批同一内容。"},
		{Key: "manual_action_package", Title: "生成既有人工操作包", CompletionCondition: "获批优化变更申请已编译不可变 ManualActionPackage", URL: base("configuration", "人工操作包"), Explanation: "操作包说明人工步骤，不代表平台已经执行。"},
	}
	if plan.ID == "" {
		return steps
	}
	strategyID, materialID := plan.CurrentVersion.StrategyReference.TaskID, ""
	if plan.CurrentVersion.IsPlatformConfigurationV2() {
		strategyID = plan.CurrentVersion.DeliveryIntent.Payload.StrategyReference.ID
		if len(plan.CurrentVersion.DeliveryIntent.Payload.MaterialReferences) > 0 {
			materialID = plan.CurrentVersion.DeliveryIntent.Payload.MaterialReferences[0].ID
		}
	} else if len(plan.CurrentVersion.CreativeReferences) > 0 {
		materialID = plan.CurrentVersion.CreativeReferences[0].AssetID
	}
	steps[0].Complete = plan.TourRunID == run.ID && plan.TourOwnerID == actor.Principal.ID && plan.TourCase == string(TourCaseGoldenPath) && strategyID != "" && materialID != ""
	if steps[0].Complete {
		steps[0].Evidence = []string{"run=" + run.ID, "plan=" + plan.ID, "strategy=" + strategyID, "creative=" + materialID}
	}
	steps[1].Complete = plan.CurrentVersion.PlatformConfiguration != nil || plan.CurrentVersion.ThreeTierConfiguration != nil
	if steps[1].Complete {
		steps[1].Evidence = []string{"plan_version=" + fmt.Sprint(plan.CurrentVersionNumber), "snapshot=" + plan.CurrentVersion.CanonicalHash}
	}
	repository, repositoryErr := s.tourRepository()
	if repositoryErr != nil {
		return steps
	}
	changeSets, err := repository.ListTourPlanChangeSets(ctx, actor.OrganizationID, run.ProjectID, plan.ID)
	if err != nil {
		return steps
	}
	for index := range changeSets {
		changeSets[index], err = s.hydrateChangeSet(ctx, actor.OrganizationID, run.ProjectID, changeSets[index])
		if err != nil {
			return steps
		}
	}
	var initial, recommendationChangeSet *ChangeSet
	for index := range changeSets {
		changeSet := &changeSets[index]
		if changeSet.RecommendationID == "" && initial == nil {
			initial = changeSet
		}
		if changeSet.RecommendationID != "" && recommendationChangeSet == nil {
			recommendationChangeSet = changeSet
		}
	}
	if initial != nil {
		if initial.Status != ChangeSetDraft && initial.Status != ChangeSetPreflightFailed {
			steps[2].URL = tourApprovalPageURL(run.ProjectID, run.ID, TourCaseGoldenPath, plan.ID, initial.ID)
		}
		steps[3].URL = tourApprovalPageURL(run.ProjectID, run.ID, TourCaseGoldenPath, plan.ID, initial.ID)
		steps[2].Complete = initial.Approval != nil && (initial.Approval.Valid || initial.Status == ChangeSetExecuted)
		steps[2].Evidence = []string{"change_set=" + initial.ID, "status=" + string(initial.Status)}
	}
	executions, _ := repository.ListTourPlanExecutions(ctx, actor.OrganizationID, run.ProjectID, plan.ID)
	for _, execution := range executions {
		if execution.Execution.Status == ExecutionSucceeded {
			steps[3].Complete = true
			steps[3].Evidence = []string{"execution=" + execution.Execution.ID, "evidence=" + execution.Evidence.ID}
		}
	}
	alerts, _ := repository.ListTourPlanAlerts(ctx, actor.OrganizationID, run.ProjectID, plan.ID)
	for _, alert := range alerts {
		if alert.PlanID == plan.ID && alert.SimulationRunID != "" {
			steps[4].Complete = true
			steps[4].Evidence = []string{"execution=" + alert.ExecutionID, "simulation_run=" + alert.SimulationRunID, "alert=" + alert.ID, "evaluated_at=" + alert.UpdatedAt.Format(time.RFC3339)}
			break
		}
	}
	workflow, workflowErr := s.configurationWorkflow()
	if workflowErr == nil {
		recommendations, _ := repository.ListTourPlanRecommendations(ctx, actor.OrganizationID, run.ProjectID, plan.ID)
		for _, recommendation := range recommendations {
			if recommendation.PlanID == plan.ID {
				steps[5].Complete = true
				steps[5].Evidence = append([]string{"recommendation=" + recommendation.ID, "status=" + string(recommendation.Status)}, recommendation.Evidence...)
				if recommendation.Status == RecommendationAccepted && recommendation.AcceptedChangeSetID != "" {
					steps[6].Complete = true
					steps[6].Evidence = []string{"change_set=" + recommendation.AcceptedChangeSetID}
				}
			}
		}
		if recommendationChangeSet != nil {
			if recommendationChangeSet.Status != ChangeSetDraft && recommendationChangeSet.Status != ChangeSetPreflightFailed {
				steps[7].URL = tourApprovalPageURL(run.ProjectID, run.ID, TourCaseGoldenPath, plan.ID, recommendationChangeSet.ID)
			}
			currentConfigurationHash := currentPlanConfigurationHash(plan.CurrentVersion)
			approvedTargetMaterialized := currentConfigurationHash != "" && currentConfigurationHash == recommendationChangeSet.TargetSnapshotHash
			steps[7].Complete = recommendationChangeSet.Approval != nil && (recommendationChangeSet.Approval.Valid || approvedTargetMaterialized)
			steps[7].Evidence = []string{"change_set=" + recommendationChangeSet.ID, "status=" + string(recommendationChangeSet.Status)}
			if _, packageErr := workflow.GetManualActionPackage(ctx, actor.OrganizationID, run.ProjectID, recommendationChangeSet.ID); packageErr == nil {
				steps[8].Complete = true
				steps[8].Evidence = []string{"change_set=" + recommendationChangeSet.ID, "package=present"}
			}
		}
	}
	return steps
}

func currentPlanConfigurationHash(version DeliveryPlanVersion) string {
	if version.IsPlatformConfigurationV2() {
		return version.PlatformConfiguration.CanonicalHash
	}
	hash, _ := snapshotHash(version.ThreeTierConfiguration)
	return hash
}

func tourPageURL(projectID contract.ProjectID, nav, runID, tourCase, planID, view string) string {
	query := url.Values{"tour_run_id": {runID}, "tour_case": {tourCase}}
	if planID != "" {
		query.Set("plan_id", planID)
	}
	if view != "" {
		query.Set("view", view)
	}
	return fmt.Sprintf("/projects/%s/delivery/%s?%s", url.PathEscape(string(projectID)), nav, query.Encode())
}

func tourApprovalPageURL(projectID contract.ProjectID, runID, tourCase, planID, changeSetID string) string {
	query := url.Values{
		"plan_id":     {planID},
		"tour_case":   {tourCase},
		"tour_run_id": {runID},
		"view":        {"待我审批"},
	}
	return fmt.Sprintf("/projects/%s/delivery/approvals/%s?%s", url.PathEscape(string(projectID)), url.PathEscape(changeSetID), query.Encode())
}

func sortedTourPlans(plans []DeliveryPlan) []DeliveryPlan {
	sort.Slice(plans, func(i, j int) bool { return plans[i].TourCase < plans[j].TourCase })
	return plans
}
