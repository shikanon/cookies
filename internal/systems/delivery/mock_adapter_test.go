package delivery

import (
	"context"
	"fmt"
)

type DeterministicMockAdapter struct{}

func (DeterministicMockAdapter) Source() Source { return SourceMock }
func (DeterministicMockAdapter) ExecuteStep(_ context.Context, request PlatformStepRequest) (PlatformStepResult, error) {
	result := PlatformStepResult{Status: StepSucceeded, Effect: "confirmed_applied"}
	switch request.Action {
	case "create_platform_project":
		result.Summary, result.EvidenceRef = "approval and payload verified", "mock://execution/project"
		if request.Scenario == ExecutionScenarioFailed {
			result.Status, result.Effect = StepFailed, "confirmed_not_applied"
			result.Summary, result.EvidenceRef = "fixture rejected before any target effect", "mock://execution/rejected"
		}
	case "create_promotion":
		result.Summary, result.EvidenceRef = "fixture delivery created", "mock://execution/promotion"
		if request.Scenario == ExecutionScenarioPartial {
			result.Status, result.Effect = StepFailed, "confirmed_not_applied"
			result.Summary, result.EvidenceRef = "remaining fixture operation did not complete", "mock://execution/partial"
		}
		if request.Scenario == ExecutionScenarioResultUnknown {
			result.Status, result.Effect = StepResultUnknown, "unknown"
			result.Summary, result.EvidenceRef = "fixture response was interrupted; effect cannot be verified", "mock://execution/unknown"
		}
	case "verify_platform_state":
		result.Summary, result.EvidenceRef = "fixture result verified", "mock://execution/verification"
		if request.Scenario == ExecutionScenarioPartial {
			result.Summary = "completed scope verified"
		}
		if request.Scenario == ExecutionScenarioResultUnknown {
			result.Status, result.Effect = StepResultUnknown, "unknown"
			result.Summary, result.EvidenceRef = "query or reconcile before any retry", "mock://execution/reconcile"
		}
	default:
		return PlatformStepResult{}, fmt.Errorf("%w: unknown step %q", ErrInvalidRequest, request.Action)
	}
	return result, nil
}
