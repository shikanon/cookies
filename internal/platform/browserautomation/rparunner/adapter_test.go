package rparunner

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/platform/contract"
)

type stubV3Compiler struct{}

func (stubV3Compiler) CompilePrepareV3(context.Context, browserautomation.BrowserRpaRun, browserautomation.SitePolicy) (json.RawMessage, error) {
	return json.RawMessage(`{"schema_version":"oceanengine-playwright-rpa-plan/v3","plan_kind":"promotion_edit","mode":"prepare","steps":[{"id":"identify"},{"id":"budget","kind":"field_action","field_key":"promotion.daily_budget","operation":"fill_money","value":"300.00","value_state":"provided"}]}`), nil
}

func (stubV3Compiler) CompileSubmitV3(context.Context, browserautomation.BrowserRpaRun, browserautomation.ControlledActionAttempt, browserautomation.SitePolicy, string) (json.RawMessage, error) {
	return json.RawMessage(`{"schema_version":"oceanengine-playwright-rpa-plan/v3","plan_kind":"promotion_edit","mode":"submit","steps":[{"id":"identify"}]}`), nil
}

type stubCompiler struct {
	mode string
}

func (s stubCompiler) CompilePrepare(run browserautomation.BrowserRpaRun, _ browserautomation.SitePolicy) (RpaPlan, error) {
	s.mode = "prepare"
	return minimalPlan("prepare", run), nil
}

func (s stubCompiler) CompileSubmit(run browserautomation.BrowserRpaRun, _ browserautomation.ControlledActionAttempt, _ browserautomation.SitePolicy) (RpaPlan, error) {
	s.mode = "submit"
	return minimalPlan("submit", run), nil
}

func minimalPlan(mode string, run browserautomation.BrowserRpaRun) RpaPlan {
	return RpaPlan{
		SchemaVersion: PlanSchemaV2,
		Browser:       "msedge",
		Mode:          mode,
		AccountID:     run.AccountID,
		Steps:         []RpaStep{{ID: "identify_account_and_object", Kind: "identify_page", PageKind: "promotion_list"}},
	}
}

func testAdapter(t *testing.T, mode string, env browserautomation.ExecutionEnvironment, policy browserautomation.SitePolicy) PlaywrightRPAAdapter {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "fake-runner.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	repo := browserautomation.NewMemoryRepository()
	if _, err := repo.CreateEnvironment(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateSitePolicy(context.Background(), policy); err != nil {
		t.Fatal(err)
	}
	return PlaywrightRPAAdapter{
		Runner: Runner{
			Command:        []string{"node", abs},
			ScriptPath:     mode,
			PrepareTimeout: 30 * time.Second,
			SubmitTimeout:  30 * time.Second,
		},
		Compiler: stubCompiler{},
		Store:    repo,
	}
}

func adapterFixture() (browserautomation.BrowserRpaRun, browserautomation.ExecutionEnvironment, browserautomation.SitePolicy) {
	run := browserautomation.BrowserRpaRun{
		ID:             "run_test",
		OrganizationID: contract.OrganizationID("org_test"),
		ProjectID:      contract.ProjectID("project_test"),
		AccountID:      "account_test",
		EnvironmentID:  "env_test",
		PolicyID:       "policy_test",
		Authority: browserautomation.AuthorityBinding{
			SchemaVersion:          browserautomation.AuthoritySchemaV1,
			Action:                 "update_promotion_budget",
			AccountReferenceID:     "account_test",
			TargetPlatformObjectID: "promotion_test",
			PromotionMutation: &browserautomation.PromotionMutationBinding{
				CurrentDailyBudgetMinor: 30000,
				TargetDailyBudgetMinor:  50000,
				CurrentStateHash:        strings.Repeat("a", 64),
				TargetStateHash:         strings.Repeat("b", 64),
			},
		},
	}
	env := browserautomation.ExecutionEnvironment{
		ID: "env_test", OrganizationID: run.OrganizationID, ProjectID: run.ProjectID,
		Platform: browserautomation.PlatformOceanEngine, AccountID: "account_test",
		Mode: "local_visible", BrowserVersion: "edge-test", Region: "local",
		Healthy: true, CDPEndpoint: "success", Version: 1,
	}
	policy := browserautomation.SitePolicy{
		ID: "policy_test", OrganizationID: run.OrganizationID, ProjectID: run.ProjectID,
		Platform: browserautomation.PlatformOceanEngine, AccountID: "account_test",
		AllowedProtocols: []string{"https"}, AllowedHosts: []string{"ad.oceanengine.com"},
		AllowedPageKinds: []string{"promotion_list"}, AllowedPlatformProjects: []string{"platform_project_test"},
		Version: 1,
	}
	return run, env, policy
}

func TestAdapterPrepareInjectsServerOwnedReadback(t *testing.T) {
	run, env, policy := adapterFixture()
	adapter := testAdapter(t, "success", env, policy)
	page, err := adapter.Prepare(context.Background(), run)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if page.Readback["platform_object_id"] != "promotion_test" {
		t.Fatalf("runner object identity was not promoted: %+v", page.Readback)
	}
	if page.Readback["current_state_hash"] != strings.Repeat("a", 64) || page.Readback["target_state_hash"] != strings.Repeat("b", 64) {
		t.Fatalf("server-owned state hashes were not injected: %+v", page.Readback)
	}
	if page.SelectorVersion != SelectorVersion || page.ActionVersion != ActionVersion {
		t.Fatalf("evidence provenance must identify the playwright adapter: %+v", page)
	}
}

func TestAdapterPlanReturnsPrepareOnlyV3PlanWithoutRunningBrowser(t *testing.T) {
	run, env, policy := adapterFixture()
	adapter := testAdapter(t, "runner-must-not-start", env, policy)
	adapter.Protocol = ProtocolV3
	adapter.V3Compiler = stubV3Compiler{}
	plan, err := adapter.Plan(context.Background(), run)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plan), `"mode":"prepare"`) || strings.Contains(string(plan), `"allow_remote_write":true`) {
		t.Fatalf("unexpected preview: %s", plan)
	}
}

func TestAdapterPrepareRejectsAccountMismatchingEnvironment(t *testing.T) {
	run, env, policy := adapterFixture()
	env.AccountID = "another_account"
	adapter := testAdapter(t, "success", env, policy)
	_, err := adapter.Prepare(context.Background(), run)
	if !errors.Is(err, browserautomation.ErrAccountMismatch) {
		t.Fatalf("expected account mismatch, got %v", err)
	}
}

func TestAdapterPrepareRequiresACDPEndpoint(t *testing.T) {
	run, env, policy := adapterFixture()
	env.CDPEndpoint = ""
	adapter := testAdapter(t, "success", env, policy)
	_, err := adapter.Prepare(context.Background(), run)
	if !errors.Is(err, browserautomation.ErrEnvironmentUnavailable) {
		t.Fatalf("expected environment unavailable, got %v", err)
	}
}

func TestAdapterPrepareClassifiesRunnerPageDrift(t *testing.T) {
	run, env, policy := adapterFixture()
	adapter := testAdapter(t, "page-drift", env, policy)
	_, err := adapter.Prepare(context.Background(), run)
	if !errors.Is(err, browserautomation.ErrPageDrift) {
		t.Fatalf("expected page drift, got %v", err)
	}
}

func TestAdapterSubmitInfrastructureFailureIsResultUnknown(t *testing.T) {
	run, env, policy := adapterFixture()
	env.CDPEndpoint = "garbage"
	adapter := testAdapter(t, "garbage", env, policy)
	outcome, _, err := adapter.Submit(context.Background(), run, browserautomation.ControlledActionAttempt{}, "")
	if err != nil {
		t.Fatalf("submit must not surface infrastructure errors as authorization failures: %v", err)
	}
	if outcome != browserautomation.WorkerResultUnknown {
		t.Fatalf("infrastructure failure after authorization must be result_unknown, got %s", outcome)
	}
}

func TestAdapterSubmitAcceptsConfirmedNoEffectAsFailed(t *testing.T) {
	run, env, policy := adapterFixture()
	adapter := testAdapter(t, "v3-no-effect", env, policy)
	adapter.Protocol = ProtocolV3
	adapter.V3Compiler = stubV3Compiler{}
	outcome, page, err := adapter.Submit(context.Background(), run, browserautomation.ControlledActionAttempt{}, "token")
	if err != nil || outcome != browserautomation.WorkerFailed {
		t.Fatalf("confirmed no effect must be a failed action: outcome=%s err=%v", outcome, err)
	}
	if page.Readback["reconciliation"] != "not_found" || page.Readback["platform_write_request_observed"] != "false" {
		t.Fatalf("confirmed no-effect proof was not preserved: %#v", page.Readback)
	}
}

func TestAdapterSubmitRejectsSuccessWithoutClick(t *testing.T) {
	run, env, policy := adapterFixture()
	adapter := testAdapter(t, "success", env, policy)
	outcome, _, err := adapter.Submit(context.Background(), run, browserautomation.ControlledActionAttempt{}, "")
	if err == nil || outcome != browserautomation.WorkerFailed {
		t.Fatalf("success without the final click must fail, got outcome=%s err=%v", outcome, err)
	}
}

func TestAdapterRunsV3AndProjectsObjectAndFieldReconciliation(t *testing.T) {
	run, env, policy := adapterFixture()
	run.Authority.TargetPlatformObjectID = "promotion_v3_test"
	adapter := testAdapter(t, "success", env, policy)
	adapter.Protocol = ProtocolV3
	adapter.V3Compiler = stubV3Compiler{}

	page, err := adapter.Prepare(context.Background(), run)
	if err != nil {
		t.Fatalf("prepare v3: %v", err)
	}
	if page.Readback["platform_object_id"] != "promotion_v3_test" || page.Readback["field_reconciliation_status"] != "matched" {
		t.Fatalf("v3 prepare readback = %#v", page.Readback)
	}
	if len(page.DiffKeys) != 1 || page.DiffKeys[0] != "promotion.daily_budget" || page.Readback["plan_diff.promotion.daily_budget.target"] != "300.00" {
		t.Fatalf("v3 prepare plan diff = %#v / %#v", page.DiffKeys, page.Readback)
	}
	outcome, page, err := adapter.Submit(context.Background(), run, browserautomation.ControlledActionAttempt{}, "token")
	if err != nil || outcome != browserautomation.WorkerSuccess || page.Readback["platform_object_id"] != "promotion_v3_test" {
		t.Fatalf("v3 submit outcome=%s page=%#v err=%v", outcome, page, err)
	}
}

func TestAdapterRejectsSuccessfulV3ResultWithUncheckedRequiredFields(t *testing.T) {
	run, env, policy := adapterFixture()
	adapter := testAdapter(t, "v3-not-checked", env, policy)
	adapter.Protocol = ProtocolV3
	adapter.V3Compiler = stubV3Compiler{}
	outcome, page, err := adapter.Submit(context.Background(), run, browserautomation.ControlledActionAttempt{}, "token")
	if err != nil || outcome != browserautomation.WorkerResultUnknown || page.Readback["field_reconciliation_status"] != "not_checked" {
		t.Fatalf("outcome=%s page=%#v err=%v", outcome, page, err)
	}
}

func TestV3DriftReadbackKeepsObjectAndStructuredFields(t *testing.T) {
	page := preparedPageFromResult(RpaResult{
		SchemaVersion: ResultSchemaV2, Outcome: OutcomeSuccessWithDrift, CreatedObjectID: "promotion_1", Reconciliation: "matched",
		FieldReconciliation: &FieldReconciliation{Status: "drifted", Fields: []ReconciledField{{
			FieldKey: "promotion.call_to_action", Expected: []any{"查看详情", "立即预订"}, Observed: []any{"立即预订"}, Status: "drifted",
		}}},
	})
	if page.Readback["platform_object_id"] != "promotion_1" || page.Readback["field.promotion.call_to_action.observed"] != `["立即预订"]` || len(page.DiffKeys) != 1 || page.DiffKeys[0] != "promotion.call_to_action" {
		t.Fatalf("drift readback = %#v", page)
	}
}

func TestClassifyAuthorityFailureAsFinalConfirmationInvalid(t *testing.T) {
	err := classifyResult(RpaResult{ErrorCode: "authority_schedule_mismatch", ErrorMessage: "start date mismatch"})
	if !errors.Is(err, browserautomation.ErrFinalConfirmationInvalid) {
		t.Fatalf("expected final confirmation invalid, got %v", err)
	}
}

func TestPreparedPageRecordsFinalClickBoundary(t *testing.T) {
	page := preparedPageFromResult(RpaResult{FinalClickPerformed: true})
	if page.Readback["final_click_performed"] != "true" {
		t.Fatalf("final click readback = %#v", page.Readback)
	}
}
