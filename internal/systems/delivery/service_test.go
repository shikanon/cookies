package delivery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

func TestPlanLifecyclePersistsVersionsAndRejectsStaleUpdate(t *testing.T) {
	service, actor := newTestService()
	draft := goldenDraft()

	created, err := service.CreatePlan(context.Background(), actor, "project_a", CreatePlanRequest{PlanDraft: draft})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if created.Source != SourceMock || created.Scenario != ScenarioGoldenPath || created.Version != 1 {
		t.Fatalf("unexpected created plan: %#v", created)
	}

	draft.Budget.TotalMinor = 880_000
	updated, err := service.UpdatePlan(context.Background(), actor, "project_a", created.ID, UpdatePlanRequest{
		ExpectedVersion: 1, PlanDraft: draft,
	})
	if err != nil {
		t.Fatalf("update plan: %v", err)
	}
	if updated.Version != 2 || len(updated.Versions) != 2 || updated.Versions[0].Budget.TotalMinor == updated.Versions[1].Budget.TotalMinor {
		t.Fatalf("immutable version history was not retained: %#v", updated)
	}
	_, err = service.UpdatePlan(context.Background(), actor, "project_a", created.ID, UpdatePlanRequest{
		ExpectedVersion: 1, PlanDraft: draft,
	})
	if !errors.Is(err, ErrPlanVersionConflict) {
		t.Fatalf("expected plan version conflict, got %v", err)
	}
}

func TestPlanIsolationAndAuthoritativePreflightScenarios(t *testing.T) {
	service, actor := newTestService()
	cases := []struct {
		name           string
		mutate         func(*PlanDraft)
		scenario       Scenario
		blocked        bool
		failedCode     string
		failedSeverity CheckSeverity
	}{
		{name: "golden", mutate: func(*PlanDraft) {}, scenario: ScenarioGoldenPath},
		{name: "budget zero", mutate: func(value *PlanDraft) { value.Budget.TotalMinor = 0 }, scenario: ScenarioBudgetZero, blocked: true, failedCode: "budget_positive", failedSeverity: CheckSeverityError},
		{name: "creative warning", mutate: func(value *PlanDraft) { value.CreativeReferences[0].Confirmed = false }, scenario: ScenarioCreativeUnconfirmed, failedCode: "creative_confirmed", failedSeverity: CheckSeverityWarning},
		{name: "tracking missing", mutate: func(value *PlanDraft) { value.Tracking.PixelID = "" }, scenario: ScenarioTrackingMissing, blocked: true, failedCode: "tracking_complete", failedSeverity: CheckSeverityError},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			draft := goldenDraft()
			testCase.mutate(&draft)
			plan, err := service.CreatePlan(context.Background(), actor, "project_a", CreatePlanRequest{PlanDraft: draft})
			if err != nil {
				t.Fatalf("create plan: %v", err)
			}
			result, err := service.RunPlanPreflight(context.Background(), actor, "project_a", plan.ID)
			if err != nil {
				t.Fatalf("run preflight: %v", err)
			}
			if result.Source != SourceMock || result.Scenario != testCase.scenario || result.Blocked != testCase.blocked {
				t.Fatalf("unexpected result: %#v", result)
			}
			if testCase.failedCode != "" {
				found := false
				for _, check := range result.Checks {
					if check.Code == testCase.failedCode && !check.Passed && check.Severity == testCase.failedSeverity && check.Repair != nil {
						found = true
					}
				}
				if !found {
					t.Fatalf("missing failed check %s: %#v", testCase.failedCode, result.Checks)
				}
			}
		})
	}

	plan, err := service.CreatePlan(context.Background(), actor, "project_a", CreatePlanRequest{PlanDraft: goldenDraft()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetPlan(context.Background(), actor, "project_b", plan.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-project read should be hidden, got %v", err)
	}
}

func TestChangeSetFreezesVersionAndRejectsStalePlan(t *testing.T) {
	service, actor := newTestService()
	plan, err := service.CreatePlan(context.Background(), actor, "project_a", CreatePlanRequest{PlanDraft: goldenDraft()})
	if err != nil {
		t.Fatal(err)
	}
	changeSet, err := service.CreateChangeSet(context.Background(), actor, "project_a", plan.ID, plan.Version)
	if err != nil {
		t.Fatal(err)
	}
	draft := goldenDraft()
	draft.Budget.TotalMinor++
	if _, err := service.UpdatePlan(context.Background(), actor, "project_a", plan.ID, UpdatePlanRequest{ExpectedVersion: 1, PlanDraft: draft}); err != nil {
		t.Fatal(err)
	}
	frozen, err := service.GetChangeSet(context.Background(), actor, "project_a", changeSet.ID)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.PlanName != plan.CurrentVersion.Name {
		t.Fatalf("ChangeSet plan name = %q, want immutable V1 name %q", frozen.PlanName, plan.CurrentVersion.Name)
	}
	if _, err := service.Preflight(context.Background(), actor, "project_a", changeSet.ID, changeSet.Version); !errors.Is(err, ErrStalePlanVersion) {
		t.Fatalf("expected stale frozen version rejection, got %v", err)
	}
}

func TestChangeSetGoldenFlowAndMetricProvenance(t *testing.T) {
	service, actor := newTestService()
	plan, err := service.CreatePlan(context.Background(), actor, "project_a", CreatePlanRequest{PlanDraft: goldenDraft()})
	if err != nil {
		t.Fatal(err)
	}
	changeSet, err := service.CreateChangeSet(context.Background(), actor, "project_a", plan.ID, plan.Version)
	if err != nil {
		t.Fatal(err)
	}
	if changeSet.PlanName != plan.CurrentVersion.Name {
		t.Fatalf("ChangeSet plan name = %q, want %q", changeSet.PlanName, plan.CurrentVersion.Name)
	}
	changeSet, err = service.Preflight(context.Background(), actor, "project_a", changeSet.ID, changeSet.Version)
	if err != nil || changeSet.Status != ChangeSetPreflightPassed {
		t.Fatalf("preflight: %#v %v", changeSet, err)
	}
	changeSet, err = service.Approve(context.Background(), actor, "project_a", changeSet.ID, changeSet.Version)
	if err != nil {
		t.Fatal(err)
	}
	executed, _, err := service.Execute(context.Background(), actor, "project_a", changeSet.ID, "key-golden", ExecuteRequest{ExpectedVersion: changeSet.Version, Scenario: ExecutionScenarioSuccess})
	if err != nil {
		t.Fatal(err)
	}
	if executed.Execution.Mode != ExecutionModeLocalSimulation {
		t.Fatalf("unexpected mode %q", executed.Execution.Mode)
	}
	metric, err := service.CreateDemoMetricSnapshot(context.Background(), actor, "project_a", executed.Execution.ID, CreateMetricSnapshotRequest{DatasetVersion: DemoMetricDatasetVersion})
	if err != nil {
		t.Fatal(err)
	}
	if metric.Source != MetricSourceDemoFixture || !metric.IsSimulated {
		t.Fatalf("unexpected metric provenance: %#v", metric)
	}
}

func TestAlertsAreDeterministicAndUseCAS(t *testing.T) {
	service, actor := newTestService()
	plan, err := service.CreatePlan(context.Background(), actor, "project_a", CreatePlanRequest{PlanDraft: goldenDraft()})
	if err != nil {
		t.Fatal(err)
	}
	repo := service.Repository.(*memoryRepository)
	repo.simulations = append(repo.simulations, OutcomeSimulationRun{ID: "simulation_1", OrganizationID: actor.OrganizationID, ProjectID: "project_a", ExecutionID: "execution_1", Scenario: OutcomeScenarioReviewRejected, Events: []OutcomeSimulationEvent{{Type: "review_rejected"}}})
	repo.metrics = append(repo.metrics,
		DeliveryMetricSnapshot{ID: "metric_1", OrganizationID: actor.OrganizationID, ProjectID: "project_a", ExecutionID: "execution_1", SimulationRunID: "simulation_1", PlanID: plan.ID, Source: MetricSourceDemoFixture, DatasetVersion: DemoMetricDatasetVersion, FixtureVersion: "deterministic-v1/test", WindowSequence: 1, WindowStart: plan.StartAt, WindowEnd: plan.StartAt.Add(24 * time.Hour), DataThrough: plan.StartAt.Add(24 * time.Hour), RawMetrics: RawMetrics{Impressions: 10000, Clicks: 500, Conversions: 20, SpendCents: 20000}},
		DeliveryMetricSnapshot{ID: "metric_2", OrganizationID: actor.OrganizationID, ProjectID: "project_a", ExecutionID: "execution_1", SimulationRunID: "simulation_1", PlanID: plan.ID, Source: MetricSourceDemoFixture, DatasetVersion: DemoMetricDatasetVersion, FixtureVersion: "deterministic-v1/test", WindowSequence: 2, WindowStart: plan.StartAt.Add(24 * time.Hour), WindowEnd: plan.StartAt.Add(48 * time.Hour), DataThrough: plan.StartAt.Add(48 * time.Hour), RawMetrics: RawMetrics{Impressions: 10000, Clicks: 400, Conversions: 0, SpendCents: 60000}},
	)
	response, err := service.EvaluateAlerts(context.Background(), actor, "project_a", EvaluateAlertsRequest{Fixture: AlertScenarioAnomalyDay})
	if err != nil || response.CreatedCount != 4 || len(response.Items) != 4 {
		t.Fatalf("evaluate=%#v err=%v", response, err)
	}
	byType := map[AlertType]DeliveryAlert{}
	for _, alert := range response.Items {
		byType[alert.Type] = alert
	}
	if got := *byType[AlertSpendSpike].MetricDefinition.ObservedValue; got != 60000 {
		t.Fatalf("spend spike must evaluate the anomaly window, got %v", got)
	}
	if got := *byType[AlertZeroConversion].MetricDefinition.ObservedValue; got != 0 {
		t.Fatalf("zero conversion must evaluate the anomaly window, got %v", got)
	}
	if got := *byType[AlertCostWorsening].MetricDefinition.ObservedValue; got != 60000 {
		t.Fatalf("cost worsening must use its safe zero-conversion denominator, got %v", got)
	}
	for _, alert := range response.Items {
		if len(alert.EvidenceRefs) < 3 || alert.ExecutionID != "execution_1" {
			t.Fatalf("alert must retain the exact execution and metric chain: %#v", alert)
		}
	}
	replayed, err := service.EvaluateAlerts(context.Background(), actor, "project_a", EvaluateAlertsRequest{Fixture: AlertScenarioAnomalyDay})
	if err != nil || replayed.ReusedCount != 4 {
		t.Fatalf("replay=%#v err=%v", replayed, err)
	}
	alert := response.Items[0]
	updated, err := service.UpdateAlert(context.Background(), actor, "project_a", alert.ID, UpdateAlertRequest{Action: AlertAcknowledge, ExpectedVersion: alert.Version})
	if err != nil || updated.Status != AlertAcknowledged || updated.Version != 2 {
		t.Fatalf("update=%#v err=%v", updated, err)
	}
	if _, err := service.UpdateAlert(context.Background(), actor, "project_a", alert.ID, UpdateAlertRequest{Action: AlertDismiss, ExpectedVersion: 1}); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale terminal action error=%v", err)
	}
	if _, err := service.EvaluateAlerts(context.Background(), actor, "project_a", EvaluateAlertsRequest{Fixture: AlertScenarioStaleData}); err != nil {
		t.Fatal(err)
	}
}

func TestApprovalRemainsValidAfterExecutionAndRollbackLifecycleTransitions(t *testing.T) {
	service, actor, _ := newTestServiceClock()
	approved := approveGoldenChangeSet(t, &service, actor)
	approvalID := approved.Approval.ApprovalID
	approvalVersion := approved.Approval.ChangeSetVersion

	executed, _, err := service.Execute(context.Background(), actor, "project_a", approved.ID, "key-approval", ExecuteRequest{ExpectedVersion: approved.Version, Scenario: ExecutionScenarioSuccess})
	if err != nil {
		t.Fatal(err)
	}
	if executed.ChangeSet.Status != ChangeSetExecuted || executed.ChangeSet.Version != approvalVersion+1 {
		t.Fatalf("unexpected executed lifecycle state: %#v", executed.ChangeSet)
	}
	refreshed, err := service.GetChangeSet(context.Background(), actor, "project_a", approved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Approval == nil || !refreshed.Approval.Valid ||
		refreshed.Approval.ApprovalID != approvalID ||
		refreshed.Approval.ChangeSetVersion != approvalVersion {
		t.Fatalf("execution invalidated the immutable approval: %#v", refreshed.Approval)
	}

	rolledBack, err := service.Rollback(context.Background(), actor, "project_a", approved.ID, refreshed.Version)
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.Status != ChangeSetRolledBack || rolledBack.Version != approvalVersion+2 {
		t.Fatalf("unexpected rolled-back lifecycle state: %#v", rolledBack)
	}
	if rolledBack.Approval == nil || !rolledBack.Approval.Valid ||
		rolledBack.Approval.ApprovalID != approvalID ||
		rolledBack.Approval.ChangeSetVersion != approvalVersion {
		t.Fatalf("rollback invalidated the immutable approval: %#v", rolledBack.Approval)
	}
}

func TestRollbackRejectsNonSuccessfulExecutionOutcomes(t *testing.T) {
	for _, scenario := range []ExecutionScenario{
		ExecutionScenarioFailed,
		ExecutionScenarioPartial,
		ExecutionScenarioResultUnknown,
	} {
		t.Run(string(scenario), func(t *testing.T) {
			service, actor, _ := newTestServiceClock()
			approved := approveGoldenChangeSet(t, &service, actor)
			executed, _, err := service.Execute(context.Background(), actor, "project_a", approved.ID, "rollback-"+string(scenario), ExecuteRequest{
				ExpectedVersion: approved.Version,
				Scenario:        scenario,
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.Rollback(context.Background(), actor, "project_a", approved.ID, executed.ChangeSet.Version); !errors.Is(err, ErrInvalidState) {
				t.Fatalf("rollback %s error=%v, want invalid state", scenario, err)
			}
		})
	}
}

func TestRollbackRejectsInFlightExecution(t *testing.T) {
	service, actor, _ := newTestServiceClock()
	approved := approveGoldenChangeSet(t, &service, actor)
	executed, _, err := service.Execute(context.Background(), actor, "project_a", approved.ID, "rollback-in-flight", ExecuteRequest{
		ExpectedVersion: approved.Version,
		Scenario:        ExecutionScenarioSuccess,
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := service.Repository.(*memoryRepository)
	for index := range repository.executions {
		if repository.executions[index].Execution.ID == executed.Execution.ID {
			repository.executions[index].Execution.Status = ExecutionExecuting
			repository.executions[index].Execution.CompletedAt = nil
		}
	}
	if _, err := service.Rollback(context.Background(), actor, "project_a", approved.ID, executed.ChangeSet.Version); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("rollback in-flight error=%v, want invalid state", err)
	}
}

func TestApprovalIsValidFor24HoursThenExpires(t *testing.T) {
	service, actor, setNow := newTestServiceClock()
	changeSet := approveGoldenChangeSet(t, &service, actor)
	if changeSet.Approval == nil || !changeSet.Approval.Valid {
		t.Fatalf("approval should initially be valid: %#v", changeSet.Approval)
	}
	if got := changeSet.Approval.ExpiresAt.Sub(changeSet.Approval.ApprovedAt); got != ApprovalTTL {
		t.Fatalf("approval TTL = %s, want %s", got, ApprovalTTL)
	}

	setNow(changeSet.Approval.ExpiresAt.Add(-time.Nanosecond))
	beforeExpiry, err := service.GetChangeSet(context.Background(), actor, "project_a", changeSet.ID)
	if err != nil {
		t.Fatal(err)
	}
	if beforeExpiry.Approval == nil || !beforeExpiry.Approval.Valid {
		t.Fatalf("approval should remain valid immediately before expiry: %#v", beforeExpiry.Approval)
	}

	setNow(changeSet.Approval.ExpiresAt)
	expired, err := service.GetChangeSet(context.Background(), actor, "project_a", changeSet.ID)
	if err != nil {
		t.Fatal(err)
	}
	if expired.Approval == nil || expired.Approval.Valid || expired.Approval.InvalidReason != ApprovalInvalidExpired {
		t.Fatalf("unexpected expired approval view: %#v", expired.Approval)
	}
	if _, _, err := service.Execute(context.Background(), actor, "project_a", changeSet.ID, "key-expired", ExecuteRequest{ExpectedVersion: changeSet.Version, Scenario: ExecutionScenarioSuccess}); !errors.Is(err, ErrApprovalExpired) {
		t.Fatalf("execute after expiry error = %v, want APPROVAL_EXPIRED", err)
	}
}

func TestPlanVersionChangePermanentlyInvalidatesApprovalEvenAfterContentReverts(t *testing.T) {
	service, actor, _ := newTestServiceClock()
	changeSet := approveGoldenChangeSet(t, &service, actor)
	plan, err := service.GetPlan(context.Background(), actor, "project_a", changeSet.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	changed := goldenDraft()
	changed.Budget.TotalMinor++
	if _, err := service.UpdatePlan(context.Background(), actor, "project_a", plan.ID, UpdatePlanRequest{
		ExpectedVersion: 1, PlanDraft: changed,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdatePlan(context.Background(), actor, "project_a", plan.ID, UpdatePlanRequest{
		ExpectedVersion: 2, PlanDraft: goldenDraft(),
	}); err != nil {
		t.Fatal(err)
	}
	stale, err := service.GetChangeSet(context.Background(), actor, "project_a", changeSet.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Approval == nil || stale.Approval.Valid || stale.Approval.InvalidReason != ApprovalInvalidStalePlan {
		t.Fatalf("reverted content reactivated an old approval: %#v", stale.Approval)
	}
	if _, _, err := service.Execute(context.Background(), actor, "project_a", changeSet.ID, "key-stale", ExecuteRequest{ExpectedVersion: changeSet.Version, Scenario: ExecutionScenarioSuccess}); !errors.Is(err, ErrStalePlanVersion) {
		t.Fatalf("execute stale approval error = %v", err)
	}
	repository := service.Repository.(*memoryRepository)
	if got := len(repository.approvals[repositoryKey(actor.OrganizationID, "project_a", changeSet.ID)]); got != 1 {
		t.Fatalf("old approval audit count = %d, want 1", got)
	}
}

func TestExecuteRejectsApprovalContentAndChangeSetVersionMismatch(t *testing.T) {
	t.Run("action hash", func(t *testing.T) {
		service, actor, _ := newTestServiceClock()
		changeSet := approveGoldenChangeSet(t, &service, actor)
		repository := service.Repository.(*memoryRepository)
		key := repositoryKey(actor.OrganizationID, "project_a", changeSet.ID)
		repository.approvals[key][0].ActionHash = "tampered"
		if _, _, err := service.Execute(context.Background(), actor, "project_a", changeSet.ID, "key-hash", ExecuteRequest{ExpectedVersion: changeSet.Version, Scenario: ExecutionScenarioSuccess}); !errors.Is(err, ErrApprovalContentMismatch) {
			t.Fatalf("execute tampered action hash error = %v", err)
		}
	})

	t.Run("change set version", func(t *testing.T) {
		service, actor, _ := newTestServiceClock()
		changeSet := approveGoldenChangeSet(t, &service, actor)
		repository := service.Repository.(*memoryRepository)
		key := repositoryKey(actor.OrganizationID, "project_a", changeSet.ID)
		stored := repository.changeSets[key]
		stored.Version++
		repository.changeSets[key] = stored
		if _, _, err := service.Execute(context.Background(), actor, "project_a", changeSet.ID, "key-version", ExecuteRequest{ExpectedVersion: stored.Version, Scenario: ExecutionScenarioSuccess}); !errors.Is(err, ErrApprovalContentMismatch) {
			t.Fatalf("execute mismatched ChangeSetVersion error = %v", err)
		}
	})
}

func TestExecuteRejectsApprovalScopeAndBudgetExceeded(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*DeliveryApproval)
	}{
		{name: "scope", mutate: func(value *DeliveryApproval) { value.Scope = "execute_real" }},
		{name: "budget", mutate: func(value *DeliveryApproval) { value.BudgetLimitMinor-- }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service, actor, _ := newTestServiceClock()
			changeSet := approveGoldenChangeSet(t, &service, actor)
			repository := service.Repository.(*memoryRepository)
			key := repositoryKey(actor.OrganizationID, "project_a", changeSet.ID)
			approval := repository.approvals[key][0]
			testCase.mutate(&approval)
			var err error
			approval.ActionHash, err = ApprovalActionHash(approval)
			if err != nil {
				t.Fatal(err)
			}
			repository.approvals[key][0] = approval
			if _, _, err := service.Execute(context.Background(), actor, "project_a", changeSet.ID, "key-scope", ExecuteRequest{ExpectedVersion: changeSet.Version, Scenario: ExecutionScenarioSuccess}); !errors.Is(err, ErrApprovalScopeExceeded) {
				t.Fatalf("execute exceeded approval error = %v", err)
			}
		})
	}
}

func TestApprovalRequiresTrustedScopeAndProjectAndCannotBeOverwritten(t *testing.T) {
	service, actor, _ := newTestServiceClock()
	plan, err := service.CreatePlan(context.Background(), actor, "project_a", CreatePlanRequest{PlanDraft: goldenDraft()})
	if err != nil {
		t.Fatal(err)
	}
	changeSet, err := service.CreateChangeSet(context.Background(), actor, "project_a", plan.ID, plan.Version)
	if err != nil {
		t.Fatal(err)
	}
	changeSet, err = service.Preflight(context.Background(), actor, "project_a", changeSet.ID, changeSet.Version)
	if err != nil {
		t.Fatal(err)
	}

	withoutApprove := actor
	withoutApprove.Scopes = contract.ScopesFromStrings([]string{
		string(ScopeRead), string(ScopeWrite), string(ScopeExecute),
	})
	if _, err := service.Approve(context.Background(), withoutApprove, "project_a", changeSet.ID, changeSet.Version); err == nil || !strings.Contains(err.Error(), string(ScopeApprove)) {
		t.Fatalf("approve without trusted scope error = %v", err)
	}
	if _, err := service.Approve(context.Background(), actor, "project_b", changeSet.ID, changeSet.Version); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-project approve error = %v, want hidden not found", err)
	}
	otherOrganization := actor
	otherOrganization.OrganizationID = "org_b"
	if _, err := service.GetChangeSet(context.Background(), otherOrganization, "project_a", changeSet.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-organization read error = %v, want hidden not found", err)
	}
	if _, err := service.Approve(context.Background(), otherOrganization, "project_a", changeSet.ID, changeSet.Version); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-organization approve error = %v, want hidden not found", err)
	}
	if _, err := service.Approve(context.Background(), actor, "project_a", changeSet.ID, changeSet.Version-1); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale approve expected_version error = %v", err)
	}

	approved, err := service.Approve(context.Background(), actor, "project_a", changeSet.ID, changeSet.Version)
	if err != nil {
		t.Fatal(err)
	}
	if approved.Approval == nil ||
		approved.Approval.Source != SourceMock ||
		approved.Approval.Scenario != ScenarioGoldenPath ||
		approved.Approval.ApprovedBy != actor.Principal.ID ||
		approved.Approval.Scope != ApprovalScopeExecuteMock {
		t.Fatalf("unexpected approval projection: %#v", approved.Approval)
	}
	if _, err := service.Approve(context.Background(), actor, "project_a", changeSet.ID, approved.Version); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("second approval error = %v", err)
	}
	repository := service.Repository.(*memoryRepository)
	if got := len(repository.approvals[repositoryKey(actor.OrganizationID, "project_a", changeSet.ID)]); got != 1 {
		t.Fatalf("approval count = %d, want immutable singleton", got)
	}
}

func TestRejectChangeSetPersistsReasonAndPreventsApproval(t *testing.T) {
	service, actor, _ := newTestServiceClock()
	plan, err := service.CreatePlan(context.Background(), actor, "project_a", CreatePlanRequest{PlanDraft: goldenDraft()})
	if err != nil {
		t.Fatal(err)
	}
	changeSet, err := service.CreateChangeSet(context.Background(), actor, "project_a", plan.ID, plan.Version)
	if err != nil {
		t.Fatal(err)
	}
	changeSet, err = service.Preflight(context.Background(), actor, "project_a", changeSet.ID, changeSet.Version)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.RejectChangeSet(context.Background(), actor, "project_a", changeSet.ID, RejectChangeSetRequest{
		ExpectedVersion: changeSet.Version - 1,
		Reason:          "需要补充素材授权",
	}); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale rejection error = %v", err)
	}
	rejected, err := service.RejectChangeSet(context.Background(), actor, "project_a", changeSet.ID, RejectChangeSetRequest{
		ExpectedVersion: changeSet.Version,
		Reason:          "  需要补充素材授权  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != ChangeSetRejected || rejected.RejectionReason != "需要补充素材授权" || rejected.RejectedBy != actor.Principal.ID || rejected.RejectedAt == nil {
		t.Fatalf("unexpected rejected change request: %#v", rejected)
	}
	stored, err := service.GetChangeSet(context.Background(), actor, "project_a", changeSet.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.RejectionReason != rejected.RejectionReason || stored.RejectedAt == nil {
		t.Fatalf("rejection was not durable: %#v", stored)
	}
	if _, err := service.Approve(context.Background(), actor, "project_a", changeSet.ID, rejected.Version); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("approval after rejection error = %v", err)
	}
}

func TestExecuteRequiresAuthoritativeApprovalRecord(t *testing.T) {
	service, actor, _ := newTestServiceClock()
	plan, err := service.CreatePlan(context.Background(), actor, "project_a", CreatePlanRequest{PlanDraft: goldenDraft()})
	if err != nil {
		t.Fatal(err)
	}
	changeSet, err := service.CreateChangeSet(context.Background(), actor, "project_a", plan.ID, plan.Version)
	if err != nil {
		t.Fatal(err)
	}
	changeSet, err = service.Preflight(context.Background(), actor, "project_a", changeSet.ID, changeSet.Version)
	if err != nil {
		t.Fatal(err)
	}
	repository := service.Repository.(*memoryRepository)
	key := repositoryKey(actor.OrganizationID, "project_a", changeSet.ID)
	changeSet.Status, changeSet.Version = ChangeSetApproved, changeSet.Version+1
	repository.changeSets[key] = changeSet
	if _, _, err := service.Execute(context.Background(), actor, "project_a", changeSet.ID, "key-required", ExecuteRequest{ExpectedVersion: changeSet.Version, Scenario: ExecutionScenarioSuccess}); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("execute without authoritative approval error = %v", err)
	}
}

func TestExecutionFixturesPersistRecoveryAndIdempotency(t *testing.T) {
	cases := []struct {
		scenario ExecutionScenario
		status   ExecutionStatus
		retry    bool
		recovery string
	}{
		{ExecutionScenarioSuccess, ExecutionSucceeded, false, "none"},
		{ExecutionScenarioFailed, ExecutionFailed, false, "create_new_change_set"},
		{ExecutionScenarioPartial, ExecutionPartial, false, "review_and_compensate"},
		{ExecutionScenarioResultUnknown, ExecutionResultUnknown, false, "query_and_reconcile"},
	}
	for _, testCase := range cases {
		t.Run(string(testCase.scenario), func(t *testing.T) {
			service, actor, _ := newTestServiceClock()
			changeSet := approveGoldenChangeSet(t, &service, actor)
			request := ExecuteRequest{ExpectedVersion: changeSet.Version, Scenario: testCase.scenario}
			created, replay, err := service.Execute(context.Background(), actor, "project_a", changeSet.ID, "fixture-key", request)
			if err != nil || replay {
				t.Fatalf("create err=%v replay=%v", err, replay)
			}
			if created.Execution.Status != testCase.status || created.Execution.RetryAllowed != testCase.retry || created.Execution.RecoveryAction != testCase.recovery || len(created.Execution.Steps) != 3 {
				t.Fatalf("unexpected execution: %#v", created.Execution)
			}
			expected := map[ExecutionScenario][]struct {
				status StepStatus
				effect string
			}{
				ExecutionScenarioSuccess:       {{StepSucceeded, "confirmed_applied"}, {StepSucceeded, "confirmed_applied"}, {StepSucceeded, "confirmed_applied"}},
				ExecutionScenarioFailed:        {{StepFailed, "confirmed_not_applied"}, {StepSkipped, "none"}, {StepSkipped, "none"}},
				ExecutionScenarioPartial:       {{StepSucceeded, "confirmed_applied"}, {StepFailed, "confirmed_not_applied"}, {StepSucceeded, "confirmed_applied"}},
				ExecutionScenarioResultUnknown: {{StepSucceeded, "confirmed_applied"}, {StepResultUnknown, "unknown"}, {StepResultUnknown, "unknown"}},
			}[testCase.scenario]
			for index, step := range created.Execution.Steps {
				if step.Status != expected[index].status || step.Effect != expected[index].effect {
					t.Fatalf("scenario %s step %d=%#v want status=%s effect=%s", testCase.scenario, index+1, step, expected[index].status, expected[index].effect)
				}
			}
			if testCase.scenario == ExecutionScenarioPartial && len(created.Execution.CompensationCandidates) == 0 {
				t.Fatal("partial fixture lacks controlled compensation candidates")
			}
			loaded, err := service.GetExecution(context.Background(), actor, "project_a", created.Execution.ID)
			if err != nil || loaded.Execution.RequestHash == "" || len(loaded.Execution.Steps) != 3 {
				t.Fatalf("durable read err=%v value=%#v", err, loaded)
			}
			again, replay, err := service.Execute(context.Background(), actor, "project_a", changeSet.ID, "fixture-key", request)
			if err != nil || !replay || again.Execution.ID != created.Execution.ID {
				t.Fatalf("replay err=%v replay=%v value=%#v", err, replay, again)
			}
			_, _, err = service.Execute(context.Background(), actor, "project_a", changeSet.ID, "fixture-key", ExecuteRequest{ExpectedVersion: changeSet.Version + 1, Scenario: testCase.scenario})
			if !errors.Is(err, ErrIdempotencyConflict) {
				t.Fatalf("conflict error=%v", err)
			}
			second := approveGoldenChangeSet(t, &service, actor)
			_, _, err = service.Execute(context.Background(), actor, "project_a", second.ID, "fixture-key", ExecuteRequest{ExpectedVersion: second.Version, Scenario: testCase.scenario})
			if !errors.Is(err, ErrIdempotencyConflict) {
				t.Fatalf("cross-change-set conflict error=%v", err)
			}
			otherProject := approveGoldenChangeSetForProject(t, &service, actor, "project_b")
			if _, replay, err := service.Execute(context.Background(), actor, "project_b", otherProject.ID, "fixture-key", ExecuteRequest{ExpectedVersion: otherProject.Version, Scenario: testCase.scenario}); err != nil || replay {
				t.Fatalf("project-scoped key was not reusable in another Project: err=%v replay=%v", err, replay)
			}
			if _, err := service.GetExecution(context.Background(), actor, "project_b", created.Execution.ID); !errors.Is(err, ErrNotFound) {
				t.Fatalf("cross-project execution read=%v", err)
			}
		})
	}
}

func TestExecutionInvokesAdapterOnlyAfterDurableRunningState(t *testing.T) {
	service, actor, _ := newTestServiceClock()
	repository := service.Repository.(*memoryRepository)
	adapter := &observingAdapter{repository: repository}
	service.Adapter = adapter
	changeSet := approveGoldenChangeSet(t, &service, actor)
	request := ExecuteRequest{ExpectedVersion: changeSet.Version, Scenario: ExecutionScenarioSuccess}

	created, replay, err := service.Execute(context.Background(), actor, "project_a", changeSet.ID, "step-order-key", request)
	if err != nil || replay {
		t.Fatalf("execute err=%v replay=%v", err, replay)
	}
	if got, want := strings.Join(adapter.calls, ","), "create_platform_project,create_promotion,verify_platform_state"; got != want {
		t.Fatalf("adapter calls=%q want=%q", got, want)
	}
	if got, want := strings.Join(adapter.executionStates, ","), "executing,executing,verifying"; got != want {
		t.Fatalf("execution states at adapter boundary=%q want=%q", got, want)
	}
	for _, step := range created.Execution.Steps {
		if step.Status != StepSucceeded || step.Version != 3 || step.Attempt != 1 || step.StartedAt == nil || step.CompletedAt == nil {
			t.Fatalf("step was not advanced pending→running→succeeded: %#v", step)
		}
	}

	if _, replay, err := service.Execute(context.Background(), actor, "project_a", changeSet.ID, "step-order-key", request); err != nil || !replay {
		t.Fatalf("replay err=%v replay=%v", err, replay)
	}
	if len(adapter.calls) != 3 {
		t.Fatalf("successful terminal steps were re-executed: calls=%v", adapter.calls)
	}
}

func TestExecutionAdapterErrorBecomesUnknownWithoutBlindContinuation(t *testing.T) {
	service, actor, _ := newTestServiceClock()
	repository := service.Repository.(*memoryRepository)
	adapter := &observingAdapter{repository: repository, failAction: "create_promotion"}
	service.Adapter = adapter
	changeSet := approveGoldenChangeSet(t, &service, actor)

	created, _, err := service.Execute(context.Background(), actor, "project_a", changeSet.ID, "adapter-error-key", ExecuteRequest{
		ExpectedVersion: changeSet.Version,
		Scenario:        ExecutionScenarioSuccess,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Execution.Status != ExecutionResultUnknown || created.Execution.RecoveryAction != "query_and_reconcile" || created.Execution.RetryAllowed {
		t.Fatalf("adapter error did not produce a non-retryable unknown result: %#v", created.Execution)
	}
	if got, want := strings.Join(adapter.calls, ","), "create_platform_project,create_promotion"; got != want {
		t.Fatalf("adapter continued after unknown result: calls=%q want=%q", got, want)
	}
	if created.Execution.Steps[0].Status != StepSucceeded || created.Execution.Steps[1].Status != StepResultUnknown || created.Execution.Steps[2].Status != StepSkipped {
		t.Fatalf("unexpected interruption-safe steps: %#v", created.Execution.Steps)
	}
	if created.Execution.Steps[1].Effect != "unknown" || created.Execution.Steps[2].Attempt != 0 {
		t.Fatalf("unknown/skip effects are not safe: %#v", created.Execution.Steps)
	}
}

func TestFailedFixtureDoesNotInvokeSkippedSteps(t *testing.T) {
	service, actor, _ := newTestServiceClock()
	repository := service.Repository.(*memoryRepository)
	adapter := &observingAdapter{repository: repository}
	service.Adapter = adapter
	changeSet := approveGoldenChangeSet(t, &service, actor)
	created, _, err := service.Execute(context.Background(), actor, "project_a", changeSet.ID, "failed-call-key", ExecuteRequest{
		ExpectedVersion: changeSet.Version,
		Scenario:        ExecutionScenarioFailed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(adapter.calls, ","), "create_platform_project"; got != want {
		t.Fatalf("failed fixture adapter calls=%q want=%q", got, want)
	}
	if created.Execution.Steps[0].Effect != "confirmed_not_applied" || created.Execution.Steps[1].Status != StepSkipped || created.Execution.Steps[2].Status != StepSkipped {
		t.Fatalf("failed fixture did not prove zero target effect: %#v", created.Execution.Steps)
	}
}

func TestExecutionTransitionsUseCASAndRejectStaleWorkers(t *testing.T) {
	if !validExecutionTransition(ExecutionQueued, ExecutionValidatingApproval) || validExecutionTransition(ExecutionSucceeded, ExecutionExecuting) {
		t.Fatal("unexpected transition table")
	}
	repo := newMemoryRepository()
	now := time.Now().UTC()
	result := ExecutionResult{Execution: Execution{ID: "execution_1", OrganizationID: "org_a", ProjectID: "project_a", Status: ExecutionQueued, Version: 1, Steps: []ExecutionStep{{ID: "step_1", Status: StepPending, Version: 1}}}}
	repo.executions = []ExecutionResult{result}
	advanced, err := repo.AdvanceExecution(context.Background(), result.Execution, ExecutionValidatingApproval, nil, "none", "", nil)
	if err != nil || advanced.Execution.Version != 2 {
		t.Fatalf("advance=%#v err=%v", advanced, err)
	}
	if _, err := repo.AdvanceExecution(context.Background(), result.Execution, ExecutionExecuting, &now, "none", "", nil); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale worker error=%v", err)
	}
	if !validStepTransition(StepPending, StepRunning) || validStepTransition(StepSucceeded, StepRunning) {
		t.Fatal("unexpected step transition table")
	}
	running := advanced.Execution.Steps[0]
	running.Status = StepRunning
	step, err := repo.AdvanceStep(context.Background(), advanced.Execution, advanced.Execution.Steps[0], running)
	if err != nil || step.Version != 2 {
		t.Fatalf("step advance=%#v err=%v", step, err)
	}
	staleStep := advanced.Execution.Steps[0]
	staleStep.Status, staleStep.Version = StepPending, 1
	if _, err := repo.AdvanceStep(context.Background(), advanced.Execution, staleStep, running); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale step error=%v", err)
	}
}

func TestExactExecutionAndStepTransitionTables(t *testing.T) {
	executionAllowed := map[[2]ExecutionStatus]bool{
		{ExecutionQueued, ExecutionValidatingApproval}:    true,
		{ExecutionQueued, ExecutionCancelled}:             true,
		{ExecutionValidatingApproval, ExecutionExecuting}: true,
		{ExecutionValidatingApproval, ExecutionCancelled}: true,
		{ExecutionExecuting, ExecutionVerifying}:          true,
		{ExecutionExecuting, ExecutionResultUnknown}:      true,
		{ExecutionVerifying, ExecutionSucceeded}:          true,
		{ExecutionVerifying, ExecutionFailed}:             true,
		{ExecutionVerifying, ExecutionPartial}:            true,
		{ExecutionVerifying, ExecutionResultUnknown}:      true,
	}
	executionStates := []ExecutionStatus{ExecutionQueued, ExecutionValidatingApproval, ExecutionExecuting, ExecutionVerifying, ExecutionSucceeded, ExecutionFailed, ExecutionPartial, ExecutionResultUnknown, ExecutionCancelled}
	for _, from := range executionStates {
		for _, to := range executionStates {
			if got, want := validExecutionTransition(from, to), executionAllowed[[2]ExecutionStatus{from, to}]; got != want {
				t.Fatalf("execution transition %s→%s=%v want=%v", from, to, got, want)
			}
		}
	}

	stepAllowed := map[[2]StepStatus]bool{
		{StepPending, StepRunning}:       true,
		{StepPending, StepSkipped}:       true,
		{StepRunning, StepSucceeded}:     true,
		{StepRunning, StepFailed}:        true,
		{StepRunning, StepResultUnknown}: true,
	}
	stepStates := []StepStatus{StepPending, StepRunning, StepSucceeded, StepFailed, StepResultUnknown, StepSkipped}
	for _, from := range stepStates {
		for _, to := range stepStates {
			if got, want := validStepTransition(from, to), stepAllowed[[2]StepStatus{from, to}]; got != want {
				t.Fatalf("step transition %s→%s=%v want=%v", from, to, got, want)
			}
		}
	}
}

func approveGoldenChangeSet(t *testing.T, service *Service, actor contract.ActorContext) ChangeSet {
	return approveGoldenChangeSetForProject(t, service, actor, "project_a")
}

func approveGoldenChangeSetForProject(t *testing.T, service *Service, actor contract.ActorContext, projectID contract.ProjectID) ChangeSet {
	t.Helper()
	plan, err := service.CreatePlan(context.Background(), actor, projectID, CreatePlanRequest{PlanDraft: goldenDraft()})
	if err != nil {
		t.Fatal(err)
	}
	changeSet, err := service.CreateChangeSet(context.Background(), actor, projectID, plan.ID, plan.Version)
	if err != nil {
		t.Fatal(err)
	}
	changeSet, err = service.Preflight(context.Background(), actor, projectID, changeSet.ID, changeSet.Version)
	if err != nil {
		t.Fatal(err)
	}
	changeSet, err = service.Approve(context.Background(), actor, projectID, changeSet.ID, changeSet.Version)
	if err != nil {
		t.Fatal(err)
	}
	return changeSet
}

func goldenDraft() PlanDraft {
	return PlanDraft{
		Name: "Mock 投放计划", Objective: "获取销售线索",
		Advertiser: AdvertiserInput{ID: "mock-advertiser-001", Name: "Cookies Mock 广告主", Platform: "ocean_engine"},
		Budget:     Budget{TotalMinor: 300_000, Currency: "CNY"},
		Schedule: Schedule{
			StartAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
			EndAt:   time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), Timezone: "Asia/Shanghai",
		},
		Tracking:              Tracking{LandingPage: "https://demo.cookies.local", PixelID: "PX-001", ConversionEvent: "lead_submit"},
		CreativeReferences:    []CreativeReference{{AssetID: "asset_mock_001", Version: 1, Confirmed: true}},
		SourceStrategyVersion: "strategy-v1",
	}
}

func newTestService() (Service, contract.ActorContext) {
	service, actor, _ := newTestServiceClock()
	return service, actor
}

func newTestServiceClock() (Service, contract.ActorContext, func(time.Time)) {
	repository := newMemoryRepository()
	actor := contract.ActorContext{
		OrganizationID: "org_a",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "user_a"},
		Scopes: contract.ScopesFromStrings([]string{
			string(ScopeRead), string(ScopeWrite), string(ScopeApprove), string(ScopeExecute),
		}),
	}
	counter := 0
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	service := Service{
		Repository: repository,
		Projects:   testProjects{},
		Packages:   testPackages{},
		NewID: func(prefix string) (string, error) {
			counter++
			return fmt.Sprintf("%s_%d", prefix, counter), nil
		},
		Now: func() time.Time { return now },
	}
	return service, actor, func(value time.Time) { now = value }
}

type testProjects struct{}

func (testProjects) RequireActiveContext(_ context.Context, actor contract.ActorContext, projectID contract.ProjectID) (contract.ProjectContext, error) {
	if projectID != "project_a" && projectID != "project_b" {
		return contract.ProjectContext{}, ErrNotFound
	}
	return contract.ProjectContext{OrganizationID: actor.OrganizationID, ProjectID: projectID}, nil
}

type testPackages struct{}

func (testPackages) ReadCreativePackage(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, id string) (CreativePackageSnapshot, error) {
	return CreativePackageSnapshot{ID: id, CreativeVersionID: "creative_v1", ContentHash: "sha256:mock"}, nil
}

type observingAdapter struct {
	repository      *memoryRepository
	calls           []string
	executionStates []string
	failAction      string
}

func (*observingAdapter) Source() Source { return SourceMock }

func (a *observingAdapter) ExecuteStep(ctx context.Context, request PlatformStepRequest) (PlatformStepResult, error) {
	value, err := a.repository.GetExecution(ctx, "org_a", "project_a", request.ExecutionID)
	if err != nil {
		return PlatformStepResult{}, fmt.Errorf("execution was not durable before adapter call: %w", err)
	}
	var durable *ExecutionStep
	for index := range value.Execution.Steps {
		if value.Execution.Steps[index].Action == request.Action {
			durable = &value.Execution.Steps[index]
			break
		}
	}
	if durable == nil || durable.Status != StepRunning || durable.Attempt != 1 || durable.StartedAt == nil {
		return PlatformStepResult{}, fmt.Errorf("step was not durably running before adapter call: %#v", durable)
	}
	a.calls = append(a.calls, request.Action)
	a.executionStates = append(a.executionStates, string(value.Execution.Status))
	if request.Action == a.failAction {
		return PlatformStepResult{}, errors.New("controlled adapter interruption")
	}
	return (DeterministicMockAdapter{}).ExecuteStep(ctx, request)
}

type memoryRepository struct {
	plans       map[string]DeliveryPlan
	changeSets  map[string]ChangeSet
	approvals   map[string][]DeliveryApproval
	executions  []ExecutionResult
	metrics     []DeliveryMetricSnapshot
	simulations []OutcomeSimulationRun
	alerts      map[string]DeliveryAlert
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		plans: map[string]DeliveryPlan{}, changeSets: map[string]ChangeSet{},
		approvals: map[string][]DeliveryApproval{},
	}
}

func repositoryKey(organizationID contract.OrganizationID, projectID contract.ProjectID, id string) string {
	return string(organizationID) + "/" + string(projectID) + "/" + id
}

func (r *memoryRepository) CreatePlan(_ context.Context, plan DeliveryPlan, version DeliveryPlanVersion) (DeliveryPlan, error) {
	key := repositoryKey(plan.OrganizationID, plan.ProjectID, plan.ID)
	plan.CurrentVersion = cloneVersion(version)
	plan.Versions = []DeliveryPlanVersion{cloneVersion(version)}
	r.plans[key] = plan
	return plan, nil
}

func (r *memoryRepository) UpdatePlan(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, id string, expectedVersion int, version DeliveryPlanVersion) (DeliveryPlan, error) {
	key := repositoryKey(organizationID, projectID, id)
	plan, ok := r.plans[key]
	if !ok {
		return DeliveryPlan{}, ErrNotFound
	}
	if plan.CurrentVersionNumber != expectedVersion {
		return DeliveryPlan{}, ErrPlanVersionConflict
	}
	plan.Version = int64(version.VersionNumber)
	plan.CurrentVersionNumber = version.VersionNumber
	plan.CurrentVersion = cloneVersion(version)
	plan.Versions = append(plan.Versions, cloneVersion(version))
	plan.Name, plan.Objective = versionName(version), versionObjective(version)
	plan.BudgetCents = versionBudget(version).TotalMinor
	plan.StartAt, plan.EndAt = versionSchedule(version)
	plan.Scenario, plan.UpdatedAt = version.Scenario, version.CreatedAt
	r.plans[key] = plan
	return plan, nil
}

func (r *memoryRepository) ListPlans(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, _ int) ([]DeliveryPlan, error) {
	values := make([]DeliveryPlan, 0)
	for _, plan := range r.plans {
		if plan.OrganizationID == organizationID && plan.ProjectID == projectID {
			values = append(values, plan)
		}
	}
	return values, nil
}

func (r *memoryRepository) GetPlan(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, id string) (DeliveryPlan, error) {
	value, ok := r.plans[repositoryKey(organizationID, projectID, id)]
	if !ok {
		return DeliveryPlan{}, ErrNotFound
	}
	return value, nil
}

func (r *memoryRepository) ListPlanVersions(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, id string) ([]DeliveryPlanVersion, error) {
	plan, err := r.GetPlan(ctx, organizationID, projectID, id)
	return plan.Versions, err
}

func (r *memoryRepository) GetPlanVersion(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, id string, version int) (DeliveryPlanVersion, error) {
	values, err := r.ListPlanVersions(ctx, organizationID, projectID, id)
	if err != nil {
		return DeliveryPlanVersion{}, err
	}
	for _, value := range values {
		if value.VersionNumber == version {
			return value, nil
		}
	}
	return DeliveryPlanVersion{}, ErrNotFound
}

func (r *memoryRepository) CreateChangeSet(_ context.Context, value ChangeSet) (ChangeSet, error) {
	r.changeSets[repositoryKey(value.OrganizationID, value.ProjectID, value.ID)] = value
	return value, nil
}

func (r *memoryRepository) ListChangeSets(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, _ int) ([]ChangeSet, error) {
	values := make([]ChangeSet, 0)
	for _, value := range r.changeSets {
		if value.OrganizationID == organizationID && value.ProjectID == projectID {
			values = append(values, value)
		}
	}
	return values, nil
}

func (r *memoryRepository) GetChangeSet(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, id string) (ChangeSet, error) {
	value, ok := r.changeSets[repositoryKey(organizationID, projectID, id)]
	if !ok {
		return ChangeSet{}, ErrNotFound
	}
	return value, nil
}

func (r *memoryRepository) TransitionChangeSet(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, id string, expectedVersion int64, next ChangeSetStatus, actorID string, now time.Time) (ChangeSet, error) {
	value, err := r.GetChangeSet(ctx, organizationID, projectID, id)
	if err != nil {
		return ChangeSet{}, err
	}
	if value.Version != expectedVersion {
		return ChangeSet{}, ErrVersionConflict
	}
	value.Status, value.Version, value.UpdatedAt = next, value.Version+1, now
	if next == ChangeSetApproved {
		value.ApprovedBy, value.ApprovedAt = actorID, &now
	}
	r.changeSets[repositoryKey(organizationID, projectID, id)] = value
	return value, nil
}

func (r *memoryRepository) ApproveChangeSet(ctx context.Context, changeSet ChangeSet, approval DeliveryApproval) (ChangeSet, error) {
	plan, err := r.GetPlan(ctx, changeSet.OrganizationID, changeSet.ProjectID, changeSet.PlanID)
	if err != nil {
		return ChangeSet{}, err
	}
	if plan.Version != changeSet.PlanVersion {
		return ChangeSet{}, ErrStalePlanVersion
	}
	stored, err := r.GetChangeSet(ctx, changeSet.OrganizationID, changeSet.ProjectID, changeSet.ID)
	if err != nil {
		return ChangeSet{}, err
	}
	if stored.Status != ChangeSetPreflightPassed {
		return ChangeSet{}, ErrInvalidState
	}
	if stored.Version != changeSet.Version || approval.ChangeSetVersion != stored.Version+1 {
		return ChangeSet{}, ErrVersionConflict
	}
	key := repositoryKey(changeSet.OrganizationID, changeSet.ProjectID, changeSet.ID)
	if len(r.approvals[key]) != 0 {
		return ChangeSet{}, ErrInvalidState
	}
	r.approvals[key] = append(r.approvals[key], approval)
	stored.Status, stored.Version, stored.UpdatedAt = ChangeSetApproved, approval.ChangeSetVersion, approval.ApprovedAt
	stored.ApprovedBy = approval.ApprovedBy
	approvedAt := approval.ApprovedAt
	stored.ApprovedAt = &approvedAt
	r.changeSets[key] = stored
	return stored, nil
}

func (r *memoryRepository) RejectChangeSet(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, id string, expectedVersion int64, actorID, reason string, now time.Time) (ChangeSet, error) {
	stored, err := r.GetChangeSet(ctx, organizationID, projectID, id)
	if err != nil {
		return ChangeSet{}, err
	}
	if stored.Status != ChangeSetPreflightPassed {
		return ChangeSet{}, ErrInvalidState
	}
	if stored.Version != expectedVersion {
		return ChangeSet{}, ErrVersionConflict
	}
	stored.Status, stored.Version, stored.UpdatedAt = ChangeSetRejected, stored.Version+1, now
	stored.RejectedBy, stored.RejectionReason = actorID, reason
	rejectedAt := now
	stored.RejectedAt = &rejectedAt
	r.changeSets[repositoryKey(organizationID, projectID, id)] = stored
	return stored, nil
}

func (r *memoryRepository) GetApproval(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, changeSetID string) (DeliveryApproval, error) {
	values := r.approvals[repositoryKey(organizationID, projectID, changeSetID)]
	if len(values) == 0 {
		return DeliveryApproval{}, ErrNotFound
	}
	return values[len(values)-1], nil
}

func (r *memoryRepository) RecordExecution(ctx context.Context, changeSet ChangeSet, approval DeliveryApproval, execution Execution, evidence Evidence) (ExecutionResult, error) {
	plan, err := r.GetPlan(ctx, changeSet.OrganizationID, changeSet.ProjectID, changeSet.PlanID)
	if err != nil {
		return ExecutionResult{}, err
	}
	if plan.Version != changeSet.PlanVersion {
		return ExecutionResult{}, ErrStalePlanVersion
	}
	storedApproval, err := r.GetApproval(ctx, changeSet.OrganizationID, changeSet.ProjectID, changeSet.ID)
	if errors.Is(err, ErrNotFound) {
		return ExecutionResult{}, ErrApprovalRequired
	}
	if err != nil {
		return ExecutionResult{}, err
	}
	if !execution.StartedAt.Before(storedApproval.ExpiresAt) {
		return ExecutionResult{}, ErrApprovalExpired
	}
	if !sameApproval(storedApproval, approval) {
		return ExecutionResult{}, ErrApprovalContentMismatch
	}
	changeSet.Status, changeSet.Version = ChangeSetExecuted, changeSet.Version+1
	r.changeSets[repositoryKey(changeSet.OrganizationID, changeSet.ProjectID, changeSet.ID)] = changeSet
	value := ExecutionResult{ChangeSet: changeSet, Execution: execution, Evidence: evidence}
	r.executions = append(r.executions, value)
	return value, nil
}

func (r *memoryRepository) CreateOrReplayExecution(ctx context.Context, changeSet ChangeSet, approval DeliveryApproval, execution Execution, evidence Evidence) (ExecutionResult, bool, error) {
	for _, existing := range r.executions {
		if existing.Execution.OrganizationID == execution.OrganizationID && existing.Execution.ProjectID == execution.ProjectID && existing.Execution.IdempotencyKey == execution.IdempotencyKey {
			if existing.Execution.RequestHash != execution.RequestHash {
				return ExecutionResult{}, false, ErrIdempotencyConflict
			}
			return existing, true, nil
		}
	}
	value, err := r.RecordExecution(ctx, changeSet, approval, execution, evidence)
	return value, false, err
}

func (r *memoryRepository) FindExecutionByIdempotency(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, key string) (ExecutionResult, bool, error) {
	for _, value := range r.executions {
		if value.Execution.OrganizationID == organizationID && value.Execution.ProjectID == projectID && value.Execution.IdempotencyKey == key {
			return value, true, nil
		}
	}
	return ExecutionResult{}, false, nil
}

func (r *memoryRepository) AdvanceExecution(_ context.Context, execution Execution, next ExecutionStatus, completed *time.Time, action, reason string, compensation []string) (ExecutionResult, error) {
	for index := range r.executions {
		value := &r.executions[index]
		if value.Execution.ID != execution.ID || value.Execution.OrganizationID != execution.OrganizationID || value.Execution.ProjectID != execution.ProjectID {
			continue
		}
		if value.Execution.Version != execution.Version {
			return ExecutionResult{}, ErrVersionConflict
		}
		if !validExecutionTransition(value.Execution.Status, next) {
			return ExecutionResult{}, ErrInvalidState
		}
		value.Execution.Status, value.Execution.Version = next, value.Execution.Version+1
		value.Execution.CompletedAt, value.Execution.RecoveryAction, value.Execution.RecoveryReason, value.Execution.CompensationCandidates = completed, action, reason, compensation
		return *value, nil
	}
	return ExecutionResult{}, ErrNotFound
}

func (r *memoryRepository) AdvanceStep(_ context.Context, execution Execution, step ExecutionStep, next ExecutionStep) (ExecutionStep, error) {
	if step.ID != next.ID || step.Sequence != next.Sequence || step.Action != next.Action || !validStepTransition(step.Status, next.Status) {
		return ExecutionStep{}, ErrInvalidState
	}
	for i := range r.executions {
		if r.executions[i].Execution.ID == execution.ID && r.executions[i].Execution.Version == execution.Version {
			for j := range r.executions[i].Execution.Steps {
				current := &r.executions[i].Execution.Steps[j]
				if current.ID == step.ID {
					if current.Version != step.Version {
						return ExecutionStep{}, ErrVersionConflict
					}
					next.Version = current.Version + 1
					*current = next
					return *current, nil
				}
			}
			return ExecutionStep{}, ErrNotFound
		}
	}
	return ExecutionStep{}, ErrVersionConflict
}

func (r *memoryRepository) ListExecutions(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, _ int) ([]ExecutionResult, error) {
	values := make([]ExecutionResult, 0)
	for _, value := range r.executions {
		if value.Execution.OrganizationID == organizationID && value.Execution.ProjectID == projectID {
			values = append(values, value)
		}
	}
	return values, nil
}

func (r *memoryRepository) GetExecution(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, id string) (ExecutionResult, error) {
	for _, value := range r.executions {
		if value.Execution.OrganizationID == organizationID && value.Execution.ProjectID == projectID && value.Execution.ID == id {
			return value, nil
		}
	}
	return ExecutionResult{}, ErrNotFound
}

func (r *memoryRepository) GetExecutionByChangeSet(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, changeSetID string) (ExecutionResult, error) {
	for _, value := range r.executions {
		if value.Execution.OrganizationID == organizationID && value.Execution.ProjectID == projectID && value.Execution.ChangeSetID == changeSetID {
			return value, nil
		}
	}
	return ExecutionResult{}, ErrNotFound
}

func (r *memoryRepository) CreateMetricSnapshot(_ context.Context, value DeliveryMetricSnapshot) (DeliveryMetricSnapshot, bool, error) {
	for _, existing := range r.metrics {
		if existing.OrganizationID == value.OrganizationID && existing.ExecutionID == value.ExecutionID && existing.DatasetVersion == value.DatasetVersion && existing.FixtureVersion == value.FixtureVersion && existing.WindowSequence == value.WindowSequence {
			return existing, false, nil
		}
	}
	r.metrics = append(r.metrics, value)
	return value, true, nil
}

func (r *memoryRepository) ListMetricSnapshots(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, executionID string, _ int) ([]DeliveryMetricSnapshot, error) {
	values := make([]DeliveryMetricSnapshot, 0)
	for _, value := range r.metrics {
		if value.OrganizationID == organizationID && value.ProjectID == projectID && value.ExecutionID == executionID {
			values = append(values, value)
		}
	}
	return values, nil
}

func (r *memoryRepository) ListProjectMetricSnapshots(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, _ int) ([]DeliveryMetricSnapshot, error) {
	values := make([]DeliveryMetricSnapshot, 0)
	for _, v := range r.metrics {
		if v.OrganizationID == organizationID && v.ProjectID == projectID {
			values = append(values, v)
		}
	}
	return values, nil
}
func (r *memoryRepository) CreateOrGetOutcomeSimulation(_ context.Context, run OutcomeSimulationRun, metrics []DeliveryMetricSnapshot) (OutcomeSimulationRun, []DeliveryMetricSnapshot, bool, error) {
	for _, existing := range r.simulations {
		if existing.OrganizationID == run.OrganizationID && existing.ProjectID == run.ProjectID && existing.Fingerprint == run.Fingerprint {
			stored := make([]DeliveryMetricSnapshot, 0)
			for _, metric := range r.metrics {
				if metric.SimulationRunID == existing.ID {
					stored = append(stored, metric)
				}
			}
			return existing, stored, true, nil
		}
	}
	r.simulations = append(r.simulations, run)
	r.metrics = append(r.metrics, metrics...)
	return run, metrics, false, nil
}
func (r *memoryRepository) GetLatestOutcomeSimulation(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, executionID string) (OutcomeSimulationRun, []DeliveryMetricSnapshot, error) {
	for index := len(r.simulations) - 1; index >= 0; index-- {
		run := r.simulations[index]
		if run.OrganizationID == organizationID && run.ProjectID == projectID && run.ExecutionID == executionID {
			metrics := make([]DeliveryMetricSnapshot, 0)
			for _, metric := range r.metrics {
				if metric.SimulationRunID == run.ID {
					metrics = append(metrics, metric)
				}
			}
			return run, metrics, nil
		}
	}
	return OutcomeSimulationRun{}, nil, ErrNotFound
}
func (r *memoryRepository) UpsertAlert(_ context.Context, value DeliveryAlert) (DeliveryAlert, error) {
	if r.alerts == nil {
		r.alerts = map[string]DeliveryAlert{}
	}
	for _, existing := range r.alerts {
		if existing.OrganizationID == value.OrganizationID && existing.ProjectID == value.ProjectID && existing.Fingerprint == value.Fingerprint {
			return existing, nil
		}
	}
	r.alerts[repositoryKey(value.OrganizationID, value.ProjectID, value.ID)] = value
	return value, nil
}
func (r *memoryRepository) ListAlerts(_ context.Context, org contract.OrganizationID, project contract.ProjectID, f AlertFilter) ([]DeliveryAlert, error) {
	values := make([]DeliveryAlert, 0)
	for _, v := range r.alerts {
		if v.OrganizationID == org && v.ProjectID == project && (f.PlanID == "" || v.PlanID == f.PlanID) && (f.ExecutionID == "" || v.ExecutionID == f.ExecutionID) && (f.Status == "" || v.Status == f.Status) && (f.Type == "" || v.Type == f.Type) && (f.Severity == "" || v.Severity == f.Severity) && (f.Fixture == "" || v.Scenario == f.Fixture) {
			values = append(values, v)
		}
	}
	return values, nil
}
func (r *memoryRepository) UpdateAlert(_ context.Context, org contract.OrganizationID, project contract.ProjectID, id string, action AlertAction, expected int64, actor string, now time.Time) (DeliveryAlert, error) {
	key := repositoryKey(org, project, id)
	v, ok := r.alerts[key]
	if !ok {
		return DeliveryAlert{}, ErrNotFound
	}
	next, _ := alertStatus(action)
	if v.Status == next {
		return v, nil
	}
	if v.Version != expected {
		return DeliveryAlert{}, ErrVersionConflict
	}
	if v.Status != AlertOpen {
		return DeliveryAlert{}, ErrInvalidState
	}
	v.Status, v.Version, v.ResolvedBy, v.UpdatedAt = next, v.Version+1, actor, now
	if next == AlertAcknowledged {
		v.AcknowledgedAt = &now
	} else {
		v.DismissedAt = &now
	}
	r.alerts[key] = v
	return v, nil
}

var _ Repository = (*memoryRepository)(nil)
