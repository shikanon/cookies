package delivery

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestMissingExecutionAdapterDoesNotWriteOrChangeApproval(t *testing.T) {
	for _, direct := range []bool{false, true} {
		t.Run(map[bool]string{false: "approved_change_set", true: "plan"}[direct], func(t *testing.T) {
			service, actor := newTestService()
			approved := approveGoldenChangeSet(t, &service, actor)
			repository := service.Repository.(*memoryRepository)
			before := cloneJSONPointer(&approved)
			service.Adapter = nil
			service.NewID = func(string) (string, error) { t.Fatal("unavailable execution must not allocate IDs"); return "", nil }
			var err error
			if direct {
				_, _, err = service.ExecutePlan(context.Background(), actor, "project_a", approved.PlanID, "unavailable-plan", ExecutePlanRequest{ExpectedVersion: approved.PlanVersion})
			} else {
				_, _, err = service.Execute(context.Background(), actor, "project_a", approved.ID, "unavailable-change", ExecuteRequest{ExpectedVersion: approved.Version, Scenario: ExecutionScenarioSuccess})
			}
			if !errors.Is(err, ErrExecutionUnavailable) {
				t.Fatalf("expected unavailable execution, got %v", err)
			}
			if len(repository.executions) != 0 || len(repository.metrics) != 0 || len(repository.changeSets) != 1 {
				t.Fatal("unavailable execution produced records")
			}
			after, err := service.GetChangeSet(context.Background(), actor, "project_a", approved.ID)
			if err != nil || !reflect.DeepEqual(*before, after) {
				t.Fatalf("unavailable execution changed the approved snapshot: %v", err)
			}
		})
	}
}

func TestHistoricalExecutionRemainsReadableWithoutAdapter(t *testing.T) {
	service, actor := newTestService()
	approved := approveGoldenChangeSet(t, &service, actor)
	executed, _, err := service.Execute(context.Background(), actor, "project_a", approved.ID, "historical", ExecuteRequest{ExpectedVersion: approved.Version, Scenario: ExecutionScenarioResultUnknown})
	if err != nil {
		t.Fatal(err)
	}
	service.Adapter = nil
	restored, err := service.GetExecution(context.Background(), actor, "project_a", executed.Execution.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Execution.Source != SourceMock || restored.Execution.Status != ExecutionResultUnknown || restored.Execution.RetryAllowed {
		t.Fatalf("historical provenance or recovery state changed: %#v", restored.Execution)
	}
	if !reflect.DeepEqual(restored.Evidence, executed.Evidence) {
		t.Fatal("historical evidence changed")
	}
	if _, err := service.GetExecution(context.Background(), actor, "project_b", executed.Execution.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-project history read must fail: %v", err)
	}
}

func TestHistoricalOutcomeMetricsKeepTheirSourceAndHashes(t *testing.T) {
	service, actor := newTestService()
	service.Adapter = nil
	repository := service.Repository.(*memoryRepository)
	run := OutcomeSimulationRun{ID: "historical-simulation", OrganizationID: actor.OrganizationID, ProjectID: "project_a", ExecutionID: "historical-execution", PlanHash: "stored-plan-hash", InputHash: "stored-input-hash"}
	metric := DeliveryMetricSnapshot{ID: "historical-metric", OrganizationID: actor.OrganizationID, ProjectID: "project_a", ExecutionID: run.ExecutionID, SimulationRunID: run.ID, Source: MetricSourceDemoFixture, IsSimulated: true}
	repository.simulations = []OutcomeSimulationRun{run}
	repository.metrics = []DeliveryMetricSnapshot{metric}
	result, err := service.GetLatestOutcomeSimulation(context.Background(), actor, "project_a", run.ExecutionID)
	if err != nil || !reflect.DeepEqual(result.Run, run) || !reflect.DeepEqual(result.MetricSnapshots, []DeliveryMetricSnapshot{metric}) {
		t.Fatalf("historical outcome changed: %#v, %v", result, err)
	}
	if _, err := service.GetLatestOutcomeSimulation(context.Background(), actor, "project_b", run.ExecutionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-project history read must fail: %v", err)
	}
}
