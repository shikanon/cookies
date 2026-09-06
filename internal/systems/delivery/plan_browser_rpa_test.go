package delivery

import (
	"testing"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
)

func TestExecutionDriverPreservesHistoricalBindings(t *testing.T) {
	if got := executionDriverForBinding(ControlledAuthorityBinding{WorkflowID: "runner-v3-plan_1"}); got != browserautomation.ExecutionDriverPlaywrightEdgeV3 {
		t.Fatalf("historical driver=%s", got)
	}
	if got := executionDriverForBinding(ControlledAuthorityBinding{WorkflowID: "web-api-v1-plan_1"}); got != browserautomation.ExecutionDriverOceanEngineWebAPI {
		t.Fatalf("new driver=%s", got)
	}
	if got := executionDriverForBinding(ControlledAuthorityBinding{WorkflowID: "web-api-v1-plan_1", ExecutionDriver: browserautomation.ExecutionDriverPlaywrightEdgeV3}); got != browserautomation.ExecutionDriverPlaywrightEdgeV3 {
		t.Fatalf("explicit driver=%s", got)
	}
}

func TestPlanExecutionDriverSelectionIsValidatedAndDefaultsToWebAPI(t *testing.T) {
	if got, err := normalizePlanExecutionDriver(""); err != nil || got != browserautomation.ExecutionDriverOceanEngineWebAPI {
		t.Fatalf("default driver=%s err=%v", got, err)
	}
	for _, driver := range []browserautomation.ExecutionDriver{browserautomation.ExecutionDriverOceanEngineWebAPI, browserautomation.ExecutionDriverPlaywrightEdgeV3} {
		if got, err := normalizePlanExecutionDriver(driver); err != nil || got != driver {
			t.Fatalf("driver=%s got=%s err=%v", driver, got, err)
		}
	}
	if _, err := normalizePlanExecutionDriver("unknown/v1"); err != ErrInvalidRequest {
		t.Fatalf("invalid driver err=%v", err)
	}
}

func TestPlanExecutionDriverIsBoundIntoWorkflowAndObjectIdentity(t *testing.T) {
	webHash, err := planExecutionWorkflowHash(browserautomation.ExecutionDriverOceanEngineWebAPI, "plan", "configuration", "preflight", "key")
	if err != nil {
		t.Fatal(err)
	}
	playwrightHash, err := planExecutionWorkflowHash(browserautomation.ExecutionDriverPlaywrightEdgeV3, "plan", "configuration", "preflight", "key")
	if err != nil {
		t.Fatal(err)
	}
	if webHash == playwrightHash {
		t.Fatal("different drivers must produce different workflow hashes")
	}
	webFingerprint, err := planExecutionObjectFingerprint(browserautomation.ExecutionDriverOceanEngineWebAPI, "account", ControlledActionCreateProjectAndPromotions, "configuration")
	if err != nil {
		t.Fatal(err)
	}
	playwrightFingerprint, err := planExecutionObjectFingerprint(browserautomation.ExecutionDriverPlaywrightEdgeV3, "account", ControlledActionCreateProjectAndPromotions, "configuration")
	if err != nil {
		t.Fatal(err)
	}
	if webFingerprint == playwrightFingerprint {
		t.Fatal("different drivers must not reuse one ChangeSet fingerprint")
	}
	if workflowIDForExecutionDriver(browserautomation.ExecutionDriverOceanEngineWebAPI, "plan_1") != "web-api-v1-plan_1" || workflowIDForExecutionDriver(browserautomation.ExecutionDriverPlaywrightEdgeV3, "plan_1") != "playwright-v3-plan_1" {
		t.Fatal("workflow IDs must record the selected driver")
	}
}

func TestSamePlanExecutionTargetRecoversAfterClientReload(t *testing.T) {
	binding := ControlledAuthorityBinding{
		PlanID: "plan_1", PlanVersion: 12, PlanCanonicalHash: "plan_hash",
		ConfigurationCanonicalHash: "configuration_hash", AccountReferenceID: "account_1",
		ObjectFingerprint: "object_fingerprint", WorkflowCanonicalHash: "first_request_hash",
	}
	change := ControlledChangeSet{Action: ControlledActionCreateProjectAndPromotions, Status: ControlledChangeSetExecuting, Binding: binding}
	retryBinding := binding
	retryBinding.WorkflowCanonicalHash = "retry_after_reload_hash"
	if !samePlanExecutionTarget(change, retryBinding, ControlledActionCreateProjectAndPromotions) {
		t.Fatal("a reload must recover the execution for the same immutable plan target")
	}
	retryBinding.PlanVersion = 13
	retryBinding.PlanCanonicalHash = "same_configuration_new_plan_hash"
	if !samePlanExecutionTarget(change, retryBinding, ControlledActionCreateProjectAndPromotions) {
		t.Fatal("the same platform configuration must reuse its controlled target after a safe retry")
	}
	retryBinding.ConfigurationCanonicalHash = "different_configuration"
	if samePlanExecutionTarget(change, retryBinding, ControlledActionCreateProjectAndPromotions) {
		t.Fatal("a different configuration must not reuse the existing execution")
	}
	retryBinding = binding
	retryBinding.ExecutionDriver = browserautomation.ExecutionDriverPlaywrightEdgeV3
	change.Binding.ExecutionDriver = browserautomation.ExecutionDriverOceanEngineWebAPI
	if samePlanExecutionTarget(change, retryBinding, ControlledActionCreateProjectAndPromotions) {
		t.Fatal("a different driver must not reuse the existing execution")
	}
	change.Status = ControlledChangeSetInvalidated
	if samePlanExecutionTarget(change, binding, ControlledActionCreateProjectAndPromotions) {
		t.Fatal("an invalidated execution must not be recovered")
	}
}

func TestExistingBrowserRpaExecutionReturnsItsBoundRun(t *testing.T) {
	change := ControlledChangeSet{ID: "change_1", Status: ControlledChangeSetExecuting}
	execution := ControlledExecution{ID: "execution_1", ControlledChangeSetID: change.ID, BrowserRpaRunID: "run_1", Status: "running"}

	result, replayed, err := replayExistingBrowserRpaExecution(change, execution)
	if err != nil || !replayed || result.BrowserRpaRun.RunID != "run_1" {
		t.Fatalf("replayed=%v result=%#v err=%v", replayed, result, err)
	}
}

func TestExistingBrowserRpaExecutionOnlyContinuesAnUnboundPendingExecution(t *testing.T) {
	change := ControlledChangeSet{ID: "change_1", Status: ControlledChangeSetExecuting}
	pending := ControlledExecution{ID: "execution_1", ControlledChangeSetID: change.ID, Status: "pending"}
	if _, replayed, err := replayExistingBrowserRpaExecution(change, pending); err != nil || replayed {
		t.Fatalf("pending replayed=%v err=%v", replayed, err)
	}
	invalid := pending
	invalid.Status = "running"
	if _, _, err := replayExistingBrowserRpaExecution(change, invalid); err != ErrInvalidState {
		t.Fatalf("invalid execution error=%v", err)
	}
}

func TestSafeStagedPrepareRetryRequiresAnUnsubmittedLatestStage(t *testing.T) {
	if !safeStagedPrepareRetry("prepare_and_readback", "failed", string(browserautomation.BlockPageDrift)) {
		t.Fatal("a staged page-drift failure before submit must allow a new run")
	}
	if !safeStagedPrepareRetry("prepare_and_readback", "failed", string(browserautomation.BlockRunnerFailure)) {
		t.Fatal("a staged runner failure before submit must allow a new run")
	}
	for _, value := range [][3]string{
		{"result_observed", "failed", string(browserautomation.BlockPageDrift)},
		{"prepare_and_readback", "succeeded", ""},
		{"prepare_and_readback", "failed", string(browserautomation.BlockResultReconciliation)},
	} {
		if safeStagedPrepareRetry(value[0], value[1], value[2]) {
			t.Fatalf("unsafe staged retry accepted: %#v", value)
		}
	}
}
