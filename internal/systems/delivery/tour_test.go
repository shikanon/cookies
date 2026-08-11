package delivery

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type tourMemoryRepository struct {
	*memoryRepository
	runs map[string]DeliveryTourRun
}

func newTourTestService() (Service, contract.ActorContext, *tourMemoryRepository) {
	service, actor := newTestService()
	repository := &tourMemoryRepository{memoryRepository: service.Repository.(*memoryRepository), runs: map[string]DeliveryTourRun{}}
	service.Repository = repository
	return service, actor, repository
}

func tourRunKey(organizationID contract.OrganizationID, projectID contract.ProjectID, runID string) string {
	return repositoryKey(organizationID, projectID, runID)
}

func (r *tourMemoryRepository) CreateOrGetTourRun(_ context.Context, value DeliveryTourRun) (DeliveryTourRun, bool, error) {
	key := tourRunKey(value.OrganizationID, value.ProjectID, value.ID)
	if stored, ok := r.runs[key]; ok {
		return stored, true, nil
	}
	r.runs[key] = value
	return value, false, nil
}

func (r *tourMemoryRepository) GetTourRun(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, runID string) (DeliveryTourRun, error) {
	value, ok := r.runs[tourRunKey(organizationID, projectID, runID)]
	if !ok {
		return DeliveryTourRun{}, ErrNotFound
	}
	return value, nil
}

func (r *tourMemoryRepository) SetTourRunStatus(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, runID, ownerID string, status TourRunStatus, now time.Time) (DeliveryTourRun, error) {
	key := tourRunKey(organizationID, projectID, runID)
	value, ok := r.runs[key]
	if !ok {
		return DeliveryTourRun{}, ErrNotFound
	}
	if value.OwnerID != ownerID {
		return DeliveryTourRun{}, ErrTourOwnerMismatch
	}
	value.Status, value.UpdatedAt = status, now
	if status == TourRunPrepared {
		value.PreparedAt = &now
	}
	if status == TourRunReset {
		value.ResetAt = &now
	}
	r.runs[key] = value
	return value, nil
}

func (r *tourMemoryRepository) ListTourPlans(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, runID, ownerID string) ([]DeliveryPlan, error) {
	values := make([]DeliveryPlan, 0, 7)
	for _, plan := range r.plans {
		if plan.OrganizationID == organizationID && plan.ProjectID == projectID && plan.TourRunID == runID && plan.TourOwnerID == ownerID {
			values = append(values, plan)
		}
	}
	return sortedTourPlans(values), nil
}

func (r *tourMemoryRepository) ListTourPlanChangeSets(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, planID string) ([]ChangeSet, error) {
	values := make([]ChangeSet, 0, 2)
	for _, changeSet := range r.changeSets {
		if changeSet.OrganizationID == organizationID && changeSet.ProjectID == projectID && changeSet.PlanID == planID {
			values = append(values, changeSet)
		}
	}
	return values, nil
}

func (r *tourMemoryRepository) ListTourPlanExecutions(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, planID string) ([]ExecutionResult, error) {
	changeSetIDs := map[string]bool{}
	for _, changeSet := range r.changeSets {
		if changeSet.OrganizationID == organizationID && changeSet.ProjectID == projectID && changeSet.PlanID == planID {
			changeSetIDs[changeSet.ID] = true
		}
	}
	values := make([]ExecutionResult, 0, 2)
	for _, execution := range r.executions {
		if changeSetIDs[execution.Execution.ChangeSetID] {
			values = append(values, execution)
		}
	}
	return values, nil
}

func (r *tourMemoryRepository) ListTourPlanAlerts(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, planID string) ([]DeliveryAlert, error) {
	values := make([]DeliveryAlert, 0, 4)
	for _, alert := range r.alerts {
		if alert.OrganizationID == organizationID && alert.ProjectID == projectID && alert.PlanID == planID {
			values = append(values, alert)
		}
	}
	return values, nil
}

func (r *tourMemoryRepository) ListTourPlanRecommendations(context.Context, contract.OrganizationID, contract.ProjectID, string) ([]DeliveryRecommendation, error) {
	return []DeliveryRecommendation{}, nil
}

func (r *tourMemoryRepository) ResetTourRun(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, runID, ownerID string, now time.Time) (map[string]int64, DeliveryTourRun, error) {
	run, err := r.GetTourRun(context.Background(), organizationID, projectID, runID)
	if err != nil {
		return nil, DeliveryTourRun{}, err
	}
	if run.OwnerID != ownerID {
		return nil, DeliveryTourRun{}, ErrTourOwnerMismatch
	}
	planIDs := map[string]bool{}
	for key, plan := range r.plans {
		if plan.OrganizationID == organizationID && plan.ProjectID == projectID && plan.TourRunID == runID && plan.TourOwnerID == ownerID {
			planIDs[plan.ID] = true
			delete(r.plans, key)
		}
	}
	changeSetIDs := map[string]bool{}
	for key, changeSet := range r.changeSets {
		if planIDs[changeSet.PlanID] {
			changeSetIDs[changeSet.ID] = true
			delete(r.changeSets, key)
			delete(r.approvals, key)
		}
	}
	executionIDs := map[string]bool{}
	keptExecutions := r.executions[:0]
	for _, result := range r.executions {
		if changeSetIDs[result.Execution.ChangeSetID] {
			executionIDs[result.Execution.ID] = true
			continue
		}
		keptExecutions = append(keptExecutions, result)
	}
	r.executions = keptExecutions
	keptMetrics := r.metrics[:0]
	for _, metric := range r.metrics {
		if planIDs[metric.PlanID] || executionIDs[metric.ExecutionID] {
			continue
		}
		keptMetrics = append(keptMetrics, metric)
	}
	r.metrics = keptMetrics
	keptSimulations := r.simulations[:0]
	for _, simulation := range r.simulations {
		if planIDs[simulation.PlanID] || executionIDs[simulation.ExecutionID] {
			continue
		}
		keptSimulations = append(keptSimulations, simulation)
	}
	r.simulations = keptSimulations
	for key, alert := range r.alerts {
		if planIDs[alert.PlanID] {
			delete(r.alerts, key)
		}
	}
	run.Status, run.UpdatedAt, run.ResetAt = TourRunReset, now, &now
	r.runs[tourRunKey(organizationID, projectID, runID)] = run
	return map[string]int64{"delivery_plans": int64(len(planIDs)), "delivery_change_sets": int64(len(changeSetIDs)), "delivery_executions": int64(len(executionIDs))}, run, nil
}

func TestDeliveryTourPrepareReplayOwnershipAndIsolatedReset(t *testing.T) {
	ctx := context.Background()
	service, actor, repository := newTourTestService()
	sentinel, err := service.CreatePlan(ctx, actor, "project_a", CreatePlanRequest{PlanDraft: goldenDraft()})
	if err != nil {
		t.Fatal(err)
	}
	repository.metrics = append(repository.metrics, DeliveryMetricSnapshot{ID: "metric_unrelated", OrganizationID: actor.OrganizationID, ProjectID: "project_a", ExecutionID: "execution_unrelated", PlanID: sentinel.ID, WindowStart: sentinel.StartAt, WindowEnd: sentinel.EndAt, DataThrough: sentinel.EndAt})
	run, replay, err := service.PrepareTourRun(ctx, actor, "project_a", "investor-tour-01")
	if err != nil {
		t.Fatal(err)
	}
	if replay || run.Status != TourRunPrepared || run.OwnerID != actor.Principal.ID || run.Source != SourceMock {
		t.Fatalf("unexpected prepared run: replay=%t run=%#v", replay, run)
	}
	if len(run.Cases) != 7 || len(run.Steps) != 9 || run.CurrentStep != "first_approval" {
		t.Fatalf("tour contract is incomplete: cases=%d steps=%d current=%q", len(run.Cases), len(run.Steps), run.CurrentStep)
	}
	if !run.Steps[0].Complete || !strings.Contains(run.Steps[0].Explanation, "准备后此步默认完成") || !strings.Contains(strings.Join(run.Steps[0].Evidence, " "), "run="+run.ID) {
		t.Fatalf("step zero must prove the prepared plan belongs to this run: %#v", run.Steps[0])
	}
	for _, step := range run.Steps[5:7] {
		if !strings.Contains(step.URL, "/delivery/optimization") {
			t.Fatalf("recommendation decisions must route through Optimization Center: %#v", step)
		}
	}
	firstPlanIDs := map[string]string{}
	for _, tourCase := range run.Cases {
		firstPlanIDs[tourCase.Key] = tourCase.PlanID
		if tourCase.Source != SourceMock || tourCase.Scenario != tourCase.Key || tourCase.PlanID == "" || len(tourCase.Evidence) == 0 {
			t.Fatalf("case lacks provenance or evidence: %#v", tourCase)
		}
		if tourCase.Key != TourCaseGoldenPath && tourCase.Key != TourCaseReviewRejected && tourCase.Status != "observed" {
			t.Fatalf("exception was not independently observed: %#v", tourCase)
		}
		if tourCase.Key == TourCaseReviewRejected && tourCase.Status != "prepared" {
			t.Fatalf("review-rejection alert must be lazy, got %#v", tourCase)
		}
	}

	replayed, replay, err := service.PrepareTourRun(ctx, actor, "project_a", "investor-tour-01")
	if err != nil || !replay {
		t.Fatalf("prepare replay failed: replay=%t err=%v", replay, err)
	}
	for _, tourCase := range replayed.Cases {
		if firstPlanIDs[tourCase.Key] != tourCase.PlanID {
			t.Fatalf("replay duplicated %s: %s != %s", tourCase.Key, firstPlanIDs[tourCase.Key], tourCase.PlanID)
		}
	}

	otherActor := actor
	otherActor.Principal.ID = "user_b"
	if _, err := service.GetTourRun(ctx, otherActor, "project_a", run.ID); !errors.Is(err, ErrTourOwnerMismatch) {
		t.Fatalf("another owner read the tour: %v", err)
	}
	if _, err := service.ResetTourRun(ctx, otherActor, "project_a", run.ID); !errors.Is(err, ErrTourOwnerMismatch) {
		t.Fatalf("another owner reset the tour: %v", err)
	}

	reset, err := service.ResetTourRun(ctx, actor, "project_a", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reset.Run.Status != TourRunReset || reset.Deleted["delivery_plans"] != 7 {
		t.Fatalf("unexpected reset result: %#v", reset)
	}
	if _, err := repository.GetPlan(ctx, actor.OrganizationID, "project_a", sentinel.ID); err != nil {
		t.Fatalf("reset deleted a non-tour plan: %v", err)
	}
	remainingTourPlans, err := repository.ListTourPlans(ctx, actor.OrganizationID, "project_a", run.ID, actor.Principal.ID)
	if err != nil || len(remainingTourPlans) != 0 {
		t.Fatalf("tour plans survived reset: count=%d err=%v", len(remainingTourPlans), err)
	}
}

func TestDeliveryTourRejectsUnstableRunID(t *testing.T) {
	service, actor, _ := newTourTestService()
	for _, runID := range []string{"ab", "Has Spaces", "../unsafe", "UPPERCASE"} {
		if _, _, err := service.PrepareTourRun(context.Background(), actor, "project_a", runID); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("run id %q was accepted: %v", runID, err)
		}
		if _, err := service.GetTourRun(context.Background(), actor, "project_a", runID); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("run id %q was readable: %v", runID, err)
		}
		if _, err := service.ResetTourRun(context.Background(), actor, "project_a", runID); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("run id %q was resettable: %v", runID, err)
		}
	}
}

func TestDeliveryTourRoutesSubmittedChangeSetToApprovalCenter(t *testing.T) {
	ctx := context.Background()
	service, actor, _ := newTourTestService()
	run, _, err := service.PrepareTourRun(ctx, actor, "project_a", "investor-tour-routing")
	if err != nil {
		t.Fatal(err)
	}
	var goldenPlan DeliveryPlan
	for _, tourCase := range run.Cases {
		if tourCase.Key == TourCaseGoldenPath {
			goldenPlan, err = service.GetPlan(ctx, actor, "project_a", tourCase.PlanID)
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if goldenPlan.ID == "" {
		t.Fatal("golden tour plan is missing")
	}
	changeSet, err := service.CreateChangeSet(ctx, actor, "project_a", goldenPlan.ID, int64(goldenPlan.CurrentVersionNumber))
	if err != nil {
		t.Fatal(err)
	}
	changeSet, err = service.Preflight(ctx, actor, "project_a", changeSet.ID, changeSet.Version)
	if err != nil {
		t.Fatal(err)
	}
	if changeSet.Status != ChangeSetPreflightPassed {
		t.Fatalf("change set status = %s", changeSet.Status)
	}

	refreshed, err := service.GetTourRun(ctx, actor, "project_a", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantURL := "/projects/project_a/delivery/approvals/" + changeSet.ID
	if refreshed.CurrentStep != "first_approval" || !strings.HasPrefix(refreshed.SuggestedNextURL, wantURL) {
		t.Fatalf("submitted change set did not route to approval center: step=%s url=%s", refreshed.CurrentStep, refreshed.SuggestedNextURL)
	}
	if !strings.Contains(refreshed.SuggestedNextURL, "tour_run_id=investor-tour-routing") || !strings.Contains(refreshed.SuggestedNextURL, "view=%E5%BE%85%E6%88%91%E5%AE%A1%E6%89%B9") {
		t.Fatalf("approval route lost tour context: %s", refreshed.SuggestedNextURL)
	}
}
