package delivery

import (
	"context"
)

const MockOceanEngineAdapter = "mock_ocean_engine"

// PlatformAdapter is intentionally small so a real platform implementation can
// share the state model without changing the controlled execution contract.
type PlatformAdapter interface {
	Source() Source
	ExecuteStep(context.Context, PlatformStepRequest) (PlatformStepResult, error)
}

type PlatformStepRequest struct {
	ExecutionID string
	Scenario    ExecutionScenario
	Action      string
	Sequence    int
}
type PlatformStepResult struct {
	Status                       StepStatus
	Effect, Summary, EvidenceRef string
}

func executionOutcome(scenario ExecutionScenario) (ExecutionStatus, string, string, []string) {
	switch scenario {
	case ExecutionScenarioSuccess:
		return ExecutionSucceeded, "none", "", []string{}
	case ExecutionScenarioFailed:
		return ExecutionFailed, "create_new_change_set", "no target effect was produced", []string{}
	case ExecutionScenarioPartial:
		return ExecutionPartial, "review_and_compensate", "some target effects completed; compensation requires a new controlled action", []string{"remove_mock_delivery"}
	case ExecutionScenarioResultUnknown:
		return ExecutionResultUnknown, "query_and_reconcile", "result is unknown; blind retry is prohibited", []string{}
	default:
		return ExecutionResultUnknown, "query_and_reconcile", "adapter result is unknown; blind retry is prohibited", []string{}
	}
}

func validExecutionTransition(from, to ExecutionStatus) bool {
	switch from {
	case ExecutionQueued:
		return to == ExecutionValidatingApproval || to == ExecutionCancelled
	case ExecutionValidatingApproval:
		return to == ExecutionExecuting || to == ExecutionCancelled
	case ExecutionExecuting:
		return to == ExecutionVerifying || to == ExecutionResultUnknown
	case ExecutionVerifying:
		return to == ExecutionSucceeded || to == ExecutionFailed || to == ExecutionPartial || to == ExecutionResultUnknown
	default:
		return false
	}
}

func validStepTransition(from, to StepStatus) bool {
	return (from == StepPending && (to == StepRunning || to == StepSkipped)) || (from == StepRunning && (to == StepSucceeded || to == StepFailed || to == StepResultUnknown))
}
