package rparunner

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func testRunner(t *testing.T, mode string) Runner {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "fake-runner.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	return Runner{
		Command:        []string{"node", abs},
		ScriptPath:     mode,
		PrepareTimeout: 30 * time.Second,
		SubmitTimeout:  30 * time.Second,
	}
}

func basePlan(mode string) RpaPlan {
	return RpaPlan{
		SchemaVersion: PlanSchemaV2,
		Browser:       "msedge",
		Mode:          mode,
		AccountID:     "account_test",
		Steps: []RpaStep{{
			ID:       "identify_account_and_object",
			Kind:     "identify_page",
			PageKind: "promotion_list",
		}},
	}
}

func TestRunnerRoundTripsPlanOverStdinAndParsesResult(t *testing.T) {
	runner := testRunner(t, "success")
	result, err := runner.Run(context.Background(), basePlan("prepare"))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.SchemaVersion != ResultSchemaV1 || result.Outcome != OutcomeSuccess || result.ErrorCode != CodeOK {
		t.Fatalf("unexpected result %+v", result)
	}
	if len(result.Steps) != 1 || result.Steps[0].Readback["object_id"] != "promotion_test" {
		t.Fatalf("steps not forwarded: %+v", result.Steps)
	}
	if result.Steps[0].Readback["plan_mode"] != "prepare" {
		t.Fatalf("plan was not delivered over stdin: %+v", result.Steps[0].Readback)
	}
}

func TestRunnerReportsBusinessFailuresWithoutInfrastructureError(t *testing.T) {
	runner := testRunner(t, "page-drift")
	result, err := runner.Run(context.Background(), basePlan("prepare"))
	if err != nil {
		t.Fatalf("business failure must not be an infrastructure error: %v", err)
	}
	if result.Outcome != OutcomeFailed || result.ErrorCode != CodePageDrift {
		t.Fatalf("unexpected result %+v", result)
	}
}

func TestRunnerTreatsUnparseableOutputAsInfrastructureFailure(t *testing.T) {
	runner := testRunner(t, "garbage")
	_, err := runner.Run(context.Background(), basePlan("prepare"))
	if !errors.Is(err, ErrRunnerInfrastructure) {
		t.Fatalf("expected infrastructure failure, got %v", err)
	}
}

func TestParseResultAcceptsScalarRunnerV3Readback(t *testing.T) {
	result, err := parseResult([]byte(`{"schema_version":"oceanengine-playwright-rpa-result/v2","outcome":"success","error_code":"ok","final_click_performed":false,"steps":[{"id":"field","status":"succeeded","readback":"手动投放"},{"id":"aggregate","status":"succeeded","readback":{"project.delivery_mode":"手动投放"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Steps[0].Readback["value"] != "手动投放" || result.Steps[1].Readback["project.delivery_mode"] != "手动投放" {
		t.Fatalf("readback=%#v", result.Steps)
	}
}

func TestRunnerRejectsUnknownResultSchema(t *testing.T) {
	runner := testRunner(t, "wrong-schema")
	_, err := runner.Run(context.Background(), basePlan("prepare"))
	if !errors.Is(err, ErrRunnerInfrastructure) {
		t.Fatalf("expected infrastructure failure for unknown schema, got %v", err)
	}
}

func TestRunnerRecoversResultFromPollutedStdout(t *testing.T) {
	runner := testRunner(t, "noisy")
	result, err := runner.Run(context.Background(), basePlan("prepare"))
	if err != nil {
		t.Fatalf("polluted stdout must not fail the run: %v", err)
	}
	if result.Outcome != OutcomeSuccess {
		t.Fatalf("unexpected result %+v", result)
	}
}

func TestRunnerMissingCommandIsInfrastructureFailure(t *testing.T) {
	runner := Runner{ScriptPath: "x"}
	_, err := runner.Run(context.Background(), basePlan("prepare"))
	if !errors.Is(err, ErrRunnerInfrastructure) {
		t.Fatalf("expected infrastructure failure, got %v", err)
	}
}

func TestRunnerV3PassesRawPlanAndParsesResultV2(t *testing.T) {
	runner := testRunner(t, "success")
	plan := json.RawMessage(`{"schema_version":"oceanengine-playwright-rpa-plan/v3","mode":"submit","steps":[{"id":"identify"}]}`)
	result, err := runner.RunV3(context.Background(), plan, "one-time-token", t.TempDir())
	if err != nil {
		t.Fatalf("run v3: %v", err)
	}
	if result.SchemaVersion != ResultSchemaV2 || !result.FinalClickPerformed {
		t.Fatalf("unexpected v3 result %+v", result)
	}
	if result.CreatedObjectID != "promotion_v3_test" || result.Reconciliation != "matched" {
		t.Fatalf("v3 reconciliation not forwarded: %+v", result)
	}
	if result.FieldReconciliation == nil {
		t.Fatalf("v3 field reconciliation not forwarded: %+v", result)
	}
}

func TestRunnerV3RejectsWrongPlanSchema(t *testing.T) {
	runner := testRunner(t, "success")
	_, err := runner.RunV3(context.Background(), json.RawMessage(`{"schema_version":"wrong/v0","mode":"prepare"}`), "", "")
	if !errors.Is(err, ErrRunnerInfrastructure) {
		t.Fatalf("expected infrastructure failure, got %v", err)
	}
}

func TestRunnerV3PassesPersistentEdgeSessionFile(t *testing.T) {
	runner := testRunner(t, "success")
	runner.EdgeSessionFile = `C:\Users\test\AppData\Local\cookies\browser-rpa\session.json`
	plan := json.RawMessage(`{"schema_version":"oceanengine-playwright-rpa-plan/v3","mode":"prepare","steps":[{"id":"identify"}]}`)
	result, err := runner.RunV3(context.Background(), plan, "", "")
	if err != nil {
		t.Fatalf("run v3 with session file: %v", err)
	}
	arguments, ok := result.Steps[0].Readback["runner_args"].([]any)
	if !ok || len(arguments) < 4 || arguments[0] != "--session-file" || arguments[1] != runner.EdgeSessionFile || arguments[2] != "--result-file" {
		t.Fatalf("runner arguments = %#v", result.Steps[0].Readback)
	}
}
