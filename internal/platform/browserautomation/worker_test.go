package browserautomation

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type submitGateAdapter struct {
	DeterministicFakeAdapter
	err error
}

func (a submitGateAdapter) CheckSubmit(BrowserRpaRun) error { return a.err }

type stagedRecorderProvider struct {
	recorded []PreparedPage
	pairs    [][2]string
	complete bool
}

func (*stagedRecorderProvider) ResolveAuthority(context.Context, contract.OrganizationID, contract.ProjectID, string, time.Time) (AuthorityResolution, error) {
	return AuthorityResolution{}, nil
}
func (*stagedRecorderProvider) BindRun(context.Context, AuthorityBinding, string, time.Time) error {
	return nil
}
func (*stagedRecorderProvider) VerifyAuthority(context.Context, AuthorityBinding, string, time.Time) error {
	return nil
}
func (p *stagedRecorderProvider) RecordCreatedObject(_ context.Context, _ AuthorityBinding, _ string, page PreparedPage, resultID, listID string, _ time.Time) (bool, error) {
	p.recorded = append(p.recorded, page)
	p.pairs = append(p.pairs, [2]string{resultID, listID})
	if p.complete {
		return true, nil
	}
	return len(p.recorded) == 2, nil
}

func TestWorkerReconcilesUnknownResultWithoutAnotherControlledAction(t *testing.T) {
	now := time.Date(2026, 8, 30, 20, 45, 0, 0, time.UTC)
	repo := NewMemoryRepository()
	provider := &stagedRecorderProvider{complete: true}
	counter := 0
	service := Service{Repository: repo, AuthorityProvider: provider, Now: func() time.Time { return now }, NewID: func(prefix string) (string, error) {
		counter++
		return fmt.Sprintf("%s_%d", prefix, counter), nil
	}}
	run := validRun(now)
	run.State = RunResultUnknown
	if _, _, err := repo.CreateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	unknownStep := RunStep{ID: "submit_unknown", RunID: run.ID, Sequence: 1, WorkflowStepID: run.Authority.WorkflowStepID, Action: string(TakeoverResultUnknown), Status: StepResultUnknown, Attempt: 1, Version: 1}
	if err := repo.PutStep(context.Background(), run.OrganizationID, run.ProjectID, unknownStep); err != nil {
		t.Fatal(err)
	}
	if err := repo.AppendEvidence(context.Background(), Evidence{SchemaVersion: EvidenceSchemaV1, ID: "evidence_unknown", OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, RunID: run.ID, StepID: unknownStep.ID, FieldReadback: map[string]string{"final_click_performed": "true"}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}

	worker := Worker{Service: service}
	result, err := worker.ReconcileResultUnknown(context.Background(), run.OrganizationID, run.ProjectID, run.ID, PreparedPage{
		InternalObjectKind: "promotion",
		InternalObjectID:   "promotion-draft-1",
		Readback: map[string]string{
			"platform_object_id":          "7679817347264446507",
			"platform_status":             "pending_review",
			"reconciliation":              "matched",
			"field_reconciliation_status": "matched",
			"final_click_performed":       "false",
			"recovery_mode":               "query_only",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != RunSucceeded {
		t.Fatalf("state=%s want=%s", result.State, RunSucceeded)
	}
	if len(provider.recorded) != 1 || provider.recorded[0].Readback["recovery_mode"] != "query_only" {
		t.Fatalf("recorded=%#v", provider.recorded)
	}
	if len(repo.attempts) != 0 {
		t.Fatalf("controlled actions=%d want=0", len(repo.attempts))
	}
	evidence, err := repo.ListEvidence(context.Background(), run.OrganizationID, run.ProjectID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 3 {
		t.Fatalf("evidence=%d want=3", len(evidence))
	}
}

func TestWorkerRejectsUnknownResultReconciliationWithoutFinalClickEvidence(t *testing.T) {
	now := time.Date(2026, 8, 30, 20, 45, 0, 0, time.UTC)
	repo := NewMemoryRepository()
	provider := &stagedRecorderProvider{complete: true}
	service := Service{Repository: repo, AuthorityProvider: provider, Now: func() time.Time { return now }}
	run := validRun(now)
	run.State = RunResultUnknown
	if _, _, err := repo.CreateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if err := repo.PutStep(context.Background(), run.OrganizationID, run.ProjectID, RunStep{ID: "submit_unknown", RunID: run.ID, Sequence: 1, Status: StepResultUnknown}); err != nil {
		t.Fatal(err)
	}

	_, err := (Worker{Service: service}).ReconcileResultUnknown(context.Background(), run.OrganizationID, run.ProjectID, run.ID, PreparedPage{
		InternalObjectKind: "promotion",
		InternalObjectID:   "promotion-draft-1",
		Readback: map[string]string{
			"platform_object_id":          "7679817347264446507",
			"reconciliation":              "matched",
			"field_reconciliation_status": "matched",
		},
	})
	if err != ErrInvalidTransition {
		t.Fatalf("err=%v want=%v", err, ErrInvalidTransition)
	}
	if len(provider.recorded) != 0 || len(repo.attempts) != 0 {
		t.Fatalf("recorded=%d controlled actions=%d", len(provider.recorded), len(repo.attempts))
	}
}

func TestWorkerRecordsReadOnlyNoEffectAndMakesRunSafelyFailed(t *testing.T) {
	now := time.Date(2026, 8, 30, 20, 45, 0, 0, time.UTC)
	repo := NewMemoryRepository()
	service := Service{Repository: repo, AuthorityProvider: &stagedRecorderProvider{complete: true}, Now: func() time.Time { return now }}
	run := validRun(now)
	run.State = RunResultUnknown
	if _, _, err := repo.CreateRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if err := repo.PutStep(context.Background(), run.OrganizationID, run.ProjectID, RunStep{ID: "submit_unknown", RunID: run.ID, Sequence: 1, Status: StepResultUnknown}); err != nil {
		t.Fatal(err)
	}

	result, err := (Worker{Service: service}).ReconcileResultUnknown(context.Background(), run.OrganizationID, run.ProjectID, run.ID, PreparedPage{
		InternalObjectKind: "promotion",
		InternalObjectID:   "promotion-draft-1",
		Readback: map[string]string{
			"reconciliation":           "not_found",
			"read_only_reconciliation": "true",
			"platform_write_performed": "false",
			"exact_name_matches":       "0",
			"query_attempts":           "3",
			"final_click_performed":    "false",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != RunFailed || result.BlockingReason != BlockTargetEffectNotObserved {
		t.Fatalf("run=%#v", result)
	}
}

type stagedWorkerAdapter struct{ stage int }

func (a *stagedWorkerAdapter) Prepare(context.Context, BrowserRpaRun) (PreparedPage, error) {
	return PreparedPage{Readback: map[string]string{}, DiffKeys: []string{}, InternalObjectKind: []string{"project", "promotion"}[a.stage], InternalObjectID: []string{"project-draft-1", "promotion-draft-1"}[a.stage]}, nil
}

type driftedStagedWorkerAdapter struct{}

type unavailableWorkerAdapter struct{}

type probeCountingWorkerAdapter struct {
	probeCalls   int
	prepareCalls int
}

type driverRecordingAdapter struct {
	prepareCalls int
}

func (a *driverRecordingAdapter) Prepare(_ context.Context, _ BrowserRpaRun) (PreparedPage, error) {
	a.prepareCalls++
	return PreparedPage{Readback: map[string]string{}}, nil
}

func (*driverRecordingAdapter) Submit(context.Context, BrowserRpaRun, ControlledActionAttempt, string) (WorkerOutcome, PreparedPage, error) {
	return WorkerSuccess, PreparedPage{}, nil
}

func (a *probeCountingWorkerAdapter) CheckSession(context.Context, BrowserRpaRun) (EdgeSessionProbe, error) {
	a.probeCalls++
	return EdgeSessionProbe{SchemaVersion: EdgeSessionProbeSchemaV1, Status: "ready", Reason: "session_ready", CDPAvailable: true, OceanEnginePageAvailable: true, LoggedIn: true, AccountMatched: true}, nil
}

func (a *probeCountingWorkerAdapter) Prepare(context.Context, BrowserRpaRun) (PreparedPage, error) {
	a.prepareCalls++
	return PreparedPage{Readback: map[string]string{}}, nil
}

func (*probeCountingWorkerAdapter) Submit(context.Context, BrowserRpaRun, ControlledActionAttempt, string) (WorkerOutcome, PreparedPage, error) {
	return WorkerSuccess, PreparedPage{}, nil
}

func (unavailableWorkerAdapter) Prepare(context.Context, BrowserRpaRun) (PreparedPage, error) {
	return PreparedPage{}, ErrEnvironmentUnavailable
}

func (unavailableWorkerAdapter) Submit(context.Context, BrowserRpaRun, ControlledActionAttempt, string) (WorkerOutcome, PreparedPage, error) {
	return WorkerFailed, PreparedPage{}, ErrEnvironmentUnavailable
}

func (driftedStagedWorkerAdapter) Prepare(context.Context, BrowserRpaRun) (PreparedPage, error) {
	return PreparedPage{Readback: map[string]string{}, DiffKeys: []string{}, InternalObjectKind: "promotion", InternalObjectID: "promotion-draft-1"}, nil
}
func (driftedStagedWorkerAdapter) Submit(context.Context, BrowserRpaRun, ControlledActionAttempt, string) (WorkerOutcome, PreparedPage, error) {
	return WorkerPartial, PreparedPage{Readback: map[string]string{"platform_object_id": "7683558668450021382", "reconciliation": "matched", "field_reconciliation_status": "drifted"}, DiffKeys: []string{"promotion.call_to_action"}, InternalObjectKind: "promotion", InternalObjectID: "promotion-draft-1"}, nil
}
func (a *stagedWorkerAdapter) Submit(context.Context, BrowserRpaRun, ControlledActionAttempt, string) (WorkerOutcome, PreparedPage, error) {
	stage := a.stage
	a.stage++
	return WorkerSuccess, PreparedPage{Readback: map[string]string{"platform_object_id": []string{"7677595885572784182", "7683558668450021382"}[stage], "reconciliation": "matched"}, DiffKeys: []string{}, InternalObjectKind: []string{"project", "promotion"}[stage], InternalObjectID: []string{"project-draft-1", "promotion-draft-1"}[stage]}, nil
}

func TestDeterministicFakeWorkerTerminalOutcomes(t *testing.T) {
	for _, test := range []struct {
		outcome WorkerOutcome
		want    RunState
	}{{WorkerSuccess, RunSucceeded}, {WorkerFailed, RunFailed}, {WorkerPartial, RunPartial}, {WorkerResultUnknown, RunResultUnknown}} {
		t.Run(string(test.outcome), func(t *testing.T) {
			worker, service, repo, run, now := fakeWorkerFixture(test.outcome)
			prepared, err := worker.Prepare(context.Background(), run.OrganizationID, run.ProjectID, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			issued, err := service.IssueFinalConfirmation(context.Background(), run.OrganizationID, run.ProjectID, run.ID, prepared.Version, prepared.Authority.ApprovalActionHash, "operator")
			if err != nil {
				t.Fatal(err)
			}
			result, err := worker.Submit(context.Background(), WorkerSubmitRequest{Authorize: AuthorizeActionRequest{OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, RunID: run.ID, StepID: "submit_1", ConfirmationID: issued.Confirmation.ID, Token: issued.Token, LeaseID: "lease_1", FencingToken: 1, IdempotencyKey: "attempt_1"}})
			if err != nil {
				t.Fatal(err)
			}
			if result.State != test.want {
				t.Fatalf("state=%s want=%s", result.State, test.want)
			}
			if test.want == RunResultUnknown {
				_, err = worker.Submit(context.Background(), WorkerSubmitRequest{Authorize: AuthorizeActionRequest{OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, RunID: run.ID, StepID: "submit_2", ConfirmationID: issued.Confirmation.ID, Token: issued.Token, LeaseID: "lease_1", FencingToken: 1, IdempotencyKey: "attempt_2"}})
				if err != ErrConfirmationInvalid && err != ErrInvalidTransition {
					t.Fatalf("unknown result retried: %v", err)
				}
			}
			if len(repo.evidence) != 2 {
				t.Fatalf("evidence=%d", len(repo.evidence))
			}
			_ = now
		})
	}
}

func TestWorkerDispatchesTheRunOnlyToItsFrozenExecutionDriver(t *testing.T) {
	worker, _, repo, run, _ := fakeWorkerFixture(WorkerSuccess)
	playwright := &driverRecordingAdapter{}
	webAPI := &driverRecordingAdapter{}
	run.ExecutionDriver = ExecutionDriverPlaywrightEdgeV3
	run.Authority.ExecutionDriver = ExecutionDriverPlaywrightEdgeV3
	repo.runs[scopeKey(run.OrganizationID, run.ProjectID, run.ID)] = run
	worker.Adapter = nil
	worker.DriverAdapters = map[ExecutionDriver]WorkerAdapter{
		ExecutionDriverPlaywrightEdgeV3:  playwright,
		ExecutionDriverOceanEngineWebAPI: webAPI,
	}

	if _, err := worker.Prepare(context.Background(), run.OrganizationID, run.ProjectID, run.ID); err != nil {
		t.Fatal(err)
	}
	if playwright.prepareCalls != 1 || webAPI.prepareCalls != 0 {
		t.Fatalf("playwright calls=%d web API calls=%d", playwright.prepareCalls, webAPI.prepareCalls)
	}
}

func TestWorkerReleasesProfileLeaseAtTerminalState(t *testing.T) {
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	repo := NewMemoryRepository()
	counter := 0
	service := Service{Repository: repo, Now: func() time.Time { return now }, NewID: func(prefix string) (string, error) {
		counter++
		return fmt.Sprintf("%s_%d", prefix, counter), nil
	}}
	run := validRun(now)
	if _, _, err := service.CreateRun(context.Background(), CreateRunRequest{Run: run}); err != nil {
		t.Fatal(err)
	}
	acquired, err := service.AcquireRunLease(context.Background(), run.OrganizationID, run.ProjectID, run.ID, run.Version, "worker")
	if err != nil {
		t.Fatal(err)
	}
	worker := Worker{Service: service, Adapter: DeterministicFakeAdapter{Outcome: WorkerFailed, AccountID: run.AccountID}}
	prepared, err := worker.Prepare(context.Background(), run.OrganizationID, run.ProjectID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := service.IssueFinalConfirmation(context.Background(), run.OrganizationID, run.ProjectID, run.ID, prepared.Version, prepared.Authority.ApprovalActionHash, "operator")
	if err != nil {
		t.Fatal(err)
	}
	result, err := worker.Submit(context.Background(), WorkerSubmitRequest{Authorize: AuthorizeActionRequest{OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, RunID: run.ID, StepID: "submit_terminal", ConfirmationID: issued.Confirmation.ID, Token: issued.Token, LeaseID: acquired.Lease.ID, FencingToken: acquired.Lease.FencingToken, IdempotencyKey: "attempt_terminal"}})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := repo.GetLease(context.Background(), run.OrganizationID, run.ProjectID, acquired.Lease.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != RunFailed || result.LeaseID != "" || lease.ReleasedAt == nil {
		t.Fatalf("result=%#v lease=%#v", result, lease)
	}
}

func TestWorkerSubmitGateDoesNotConsumeConfirmation(t *testing.T) {
	worker, service, _, run, _ := fakeWorkerFixture(WorkerSuccess)
	prepared, err := worker.Prepare(context.Background(), run.OrganizationID, run.ProjectID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := service.IssueFinalConfirmation(context.Background(), run.OrganizationID, run.ProjectID, run.ID, prepared.Version, prepared.Authority.ApprovalActionHash, "operator")
	if err != nil {
		t.Fatal(err)
	}
	gateErr := errors.New("write gate closed")
	worker.Adapter = submitGateAdapter{DeterministicFakeAdapter: DeterministicFakeAdapter{Outcome: WorkerSuccess, AccountID: run.AccountID}, err: gateErr}
	request := AuthorizeActionRequest{OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, RunID: run.ID, StepID: "submit_gate", ConfirmationID: issued.Confirmation.ID, Token: issued.Token, LeaseID: "lease_1", FencingToken: 1, IdempotencyKey: "attempt_gate"}
	if _, err := worker.Submit(context.Background(), WorkerSubmitRequest{Authorize: request}); !errors.Is(err, gateErr) {
		t.Fatalf("gate error=%v", err)
	}
	if _, err := service.AuthorizeAction(context.Background(), request); err != nil {
		t.Fatalf("confirmation was consumed by the gate: %v", err)
	}
}

func TestWorkerAdvancesStagedCreatesAndUsesFreshConfirmationPerObject(t *testing.T) {
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	repo := NewMemoryRepository()
	counter := 0
	provider := &stagedRecorderProvider{}
	service := Service{Repository: repo, AuthorityProvider: provider, Now: func() time.Time { return now }, NewID: func(prefix string) (string, error) {
		counter++
		return fmt.Sprintf("%s_%d", prefix, counter), nil
	}}
	run := validRun(now)
	run.Authority.Action = "create_project_and_promotions"
	created, _, err := service.CreateRun(context.Background(), CreateRunRequest{Run: run})
	if err != nil {
		t.Fatal(err)
	}
	worker := Worker{Service: service, Adapter: &stagedWorkerAdapter{}}
	current := created
	for stage := 0; stage < 2; stage++ {
		acquired, err := service.AcquireRunLease(context.Background(), current.OrganizationID, current.ProjectID, current.ID, current.Version, fmt.Sprintf("worker_%d", stage))
		if err != nil {
			t.Fatal(err)
		}
		current = acquired.Run
		prepared, err := worker.Prepare(context.Background(), current.OrganizationID, current.ProjectID, current.ID)
		if err != nil {
			t.Fatal(err)
		}
		issued, err := service.IssueFinalConfirmation(context.Background(), current.OrganizationID, current.ProjectID, current.ID, prepared.Version, prepared.Authority.ApprovalActionHash, "operator")
		if err != nil {
			t.Fatal(err)
		}
		current, err = worker.Submit(context.Background(), WorkerSubmitRequest{Authorize: AuthorizeActionRequest{OrganizationID: current.OrganizationID, ProjectID: current.ProjectID, RunID: current.ID, StepID: fmt.Sprintf("submit_%d", stage), ConfirmationID: issued.Confirmation.ID, Token: issued.Token, LeaseID: acquired.Lease.ID, FencingToken: acquired.Lease.FencingToken, IdempotencyKey: fmt.Sprintf("attempt_%d", stage)}})
		if err != nil {
			t.Fatal(err)
		}
		want := RunEnvironmentCheck
		if stage == 1 {
			want = RunSucceeded
		}
		if current.State != want {
			t.Fatalf("stage %d state=%s want=%s", stage, current.State, want)
		}
		lease, leaseErr := repo.GetLease(context.Background(), current.OrganizationID, current.ProjectID, acquired.Lease.ID)
		if leaseErr != nil || current.LeaseID != "" || lease.ReleasedAt == nil {
			t.Fatalf("stage %d run=%#v lease=%#v err=%v", stage, current, lease, leaseErr)
		}
	}
	if len(provider.recorded) != 2 || provider.recorded[0].InternalObjectKind != "project" || provider.recorded[1].InternalObjectKind != "promotion" {
		t.Fatalf("recorded=%#v", provider.recorded)
	}
	for _, pair := range provider.pairs {
		if pair[0] == "" || pair[1] == "" || pair[0] == pair[1] {
			t.Fatalf("evidence pair=%#v", pair)
		}
	}
}

func TestWorkerWritesStagedObjectIDAndStopsOnFieldDrift(t *testing.T) {
	worker, service, _, run, _ := fakeWorkerFixture(WorkerSuccess)
	provider := &stagedRecorderProvider{}
	service.AuthorityProvider = provider
	worker.Service = service
	worker.Adapter = driftedStagedWorkerAdapter{}
	run.Authority.Action = "create_project_and_promotions"
	repo := service.Repository.(*MemoryRepository)
	repo.runs[scopeKey(run.OrganizationID, run.ProjectID, run.ID)] = run
	prepared, err := worker.Prepare(context.Background(), run.OrganizationID, run.ProjectID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := service.IssueFinalConfirmation(context.Background(), run.OrganizationID, run.ProjectID, run.ID, prepared.Version, prepared.Authority.ApprovalActionHash, "operator")
	if err != nil {
		t.Fatal(err)
	}
	result, err := worker.Submit(context.Background(), WorkerSubmitRequest{Authorize: AuthorizeActionRequest{OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, RunID: run.ID, StepID: "submit_drift", ConfirmationID: issued.Confirmation.ID, Token: issued.Token, LeaseID: "lease_1", FencingToken: 1, IdempotencyKey: "attempt_drift"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != RunPartial || len(provider.recorded) != 1 || provider.recorded[0].Readback["platform_object_id"] == "" {
		t.Fatalf("result=%#v recorded=%#v", result, provider.recorded)
	}
}

func TestWorkerControlsPauseTakeoverResumeAndCancel(t *testing.T) {
	worker, service, _, run, _ := fakeWorkerFixture(WorkerSuccess)
	paused, err := service.ControlRun(context.Background(), run.OrganizationID, run.ProjectID, run.ID, 1, ControlPause)
	if err != nil || !paused.Paused {
		t.Fatalf("pause=%#v err=%v", paused, err)
	}
	if _, err = worker.Prepare(context.Background(), run.OrganizationID, run.ProjectID, run.ID); err != ErrInvalidTransition {
		t.Fatalf("paused worker err=%v", err)
	}
	resumed, err := service.ControlRun(context.Background(), run.OrganizationID, run.ProjectID, run.ID, paused.Version, ControlResume)
	if err != nil || resumed.State != RunEnvironmentCheck || resumed.Paused {
		t.Fatalf("resume=%#v err=%v", resumed, err)
	}
	taken, err := service.ControlRun(context.Background(), run.OrganizationID, run.ProjectID, run.ID, resumed.Version, ControlTakeover)
	if err != nil || !taken.TakeoverActive || taken.State != RunAwaitingTakeover {
		t.Fatalf("takeover=%#v err=%v", taken, err)
	}
	released, err := service.ControlRun(context.Background(), run.OrganizationID, run.ProjectID, run.ID, taken.Version, ControlReleaseTakeover)
	if err != nil || released.TakeoverActive {
		t.Fatalf("release=%#v err=%v", released, err)
	}
	cancelled, err := service.ControlRun(context.Background(), run.OrganizationID, run.ProjectID, run.ID, released.Version, ControlCancel)
	if err != nil || cancelled.State != RunCancelled {
		t.Fatalf("cancel=%#v err=%v", cancelled, err)
	}
}

func TestFakeWorkerRejectsAccountDriftBeforeConfirmation(t *testing.T) {
	worker, _, _, run, _ := fakeWorkerFixture(WorkerSuccess)
	worker.Adapter = DeterministicFakeAdapter{Outcome: WorkerSuccess, AccountID: "wrong-account"}
	result, err := worker.Prepare(context.Background(), run.OrganizationID, run.ProjectID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != RunFailed || result.BlockingReason != BlockAccountMismatch {
		t.Fatalf("result=%#v", result)
	}
}

func TestWorkerDoesNotReportRunnerInfrastructureFailureAsPageDrift(t *testing.T) {
	worker, _, _, run, _ := fakeWorkerFixture(WorkerSuccess)
	worker.Adapter = unavailableWorkerAdapter{}
	result, err := worker.Prepare(context.Background(), run.OrganizationID, run.ProjectID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != RunFailed || result.BlockingReason != BlockRunnerFailure {
		t.Fatalf("result=%#v", result)
	}
}

func TestWorkerPrepareDoesNotOpenASecondSessionProbeConnection(t *testing.T) {
	worker, _, _, run, _ := fakeWorkerFixture(WorkerSuccess)
	adapter := &probeCountingWorkerAdapter{}
	worker.Adapter = adapter
	result, err := worker.Prepare(context.Background(), run.OrganizationID, run.ProjectID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != RunAwaitingConfirmation || adapter.probeCalls != 0 || adapter.prepareCalls != 1 {
		t.Fatalf("result=%#v probe_calls=%d prepare_calls=%d", result, adapter.probeCalls, adapter.prepareCalls)
	}
}

func TestDeterministicFakeAdapterProjectsExactPromotionMutationReadback(t *testing.T) {
	now := time.Date(2026, 8, 14, 5, 0, 0, 0, time.UTC)
	run := validRun(now)
	currentHash, _ := contract.CanonicalJSONHash(struct {
		DailyBudgetMinor int64 `json:"daily_budget_minor"`
	}{30000})
	targetHash, _ := contract.CanonicalJSONHash(struct {
		DailyBudgetMinor int64 `json:"daily_budget_minor"`
	}{36000})
	run.Authority.Action = "update_promotion_budget"
	run.Authority.ParentPlatformProjectID = "project_test"
	run.Authority.TargetMappingID = "mapping_test"
	run.Authority.TargetMappingVersion = 2
	run.Authority.TargetPlatformObjectID = "promotion_test"
	run.Authority.TargetPlatformObjectKind = "promotion"
	run.Authority.OperatorPrincipalID = "operator_1"
	run.Authority.PromotionBudgetLimitMinor = 36000
	run.Authority.BudgetLimitMinor = 36000
	run.Authority.PromotionMutation = &PromotionMutationBinding{CurrentDailyBudgetMinor: 30000, TargetDailyBudgetMinor: 36000, CurrentStateHash: currentHash, TargetStateHash: targetHash}
	adapter := DeterministicFakeAdapter{Outcome: WorkerSuccess, AccountID: run.AccountID}
	prepared, err := adapter.Prepare(context.Background(), run)
	if err != nil || prepared.Readback["platform_object_id"] != "promotion_test" || prepared.Readback["current_state_hash"] != currentHash || prepared.Readback["target_state_hash"] != targetHash || len(prepared.DiffKeys) != 0 {
		t.Fatalf("prepared=%#v err=%v", prepared, err)
	}
	_, outcome, err := adapter.Submit(context.Background(), run, ControlledActionAttempt{}, "")
	if err != nil || outcome.Readback["platform_object_id"] != "promotion_test" || outcome.Readback["target_state_hash"] != targetHash {
		t.Fatalf("outcome=%#v err=%v", outcome, err)
	}
}

func TestDeterministicFakeAdapterProjectsEmergencyPauseReadback(t *testing.T) {
	now := time.Date(2026, 8, 14, 6, 30, 0, 0, time.UTC)
	run := validRun(now)
	currentHash, _ := contract.CanonicalJSONHash(struct {
		DailyBudgetMinor int64  `json:"daily_budget_minor"`
		PlatformStatus   string `json:"platform_status"`
	}{30000, "delivering"})
	targetHash, _ := contract.CanonicalJSONHash(struct {
		DailyBudgetMinor int64  `json:"daily_budget_minor"`
		PlatformStatus   string `json:"platform_status"`
	}{30000, "paused"})
	run.Authority.Action = "pause_promotion"
	run.Authority.ParentPlatformProjectID = "project_test"
	run.Authority.TargetMappingID = "mapping_test"
	run.Authority.TargetMappingVersion = 2
	run.Authority.TargetPlatformObjectID = "promotion_test"
	run.Authority.TargetPlatformObjectKind = "promotion"
	run.Authority.OperatorPrincipalID = "operator_1"
	run.Authority.PromotionBudgetLimitMinor = 30000
	run.Authority.BudgetLimitMinor = 30000
	run.Authority.PromotionControl = &PromotionControlBinding{CurrentDailyBudgetMinor: 30000, CurrentPlatformStatus: "delivering", TargetPlatformStatus: "paused", CurrentStateHash: currentHash, TargetStateHash: targetHash}
	adapter := DeterministicFakeAdapter{Outcome: WorkerSuccess, AccountID: run.AccountID}
	prepared, err := adapter.Prepare(context.Background(), run)
	if err != nil || prepared.Readback["platform_object_id"] != "promotion_test" || prepared.Readback["current_state_hash"] != currentHash || prepared.Readback["target_state_hash"] != targetHash {
		t.Fatalf("prepared=%#v err=%v", prepared, err)
	}
	_, outcome, err := adapter.Submit(context.Background(), run, ControlledActionAttempt{}, "")
	if err != nil || outcome.Readback["platform_object_id"] != "promotion_test" || outcome.Readback["platform_status"] != "paused" || outcome.Readback["target_state_hash"] != targetHash {
		t.Fatalf("outcome=%#v err=%v", outcome, err)
	}
}

func TestDeterministicFakeAdapterProjectsControlledRestartRechecks(t *testing.T) {
	now := time.Date(2026, 8, 14, 7, 30, 0, 0, time.UTC)
	run := validRun(now)
	restart := PromotionRestartBinding{
		CurrentDailyBudgetMinor:  30000,
		ApprovedDailyBudgetMinor: 30000,
		CurrentPlatformStatus:    "paused",
		TargetPlatformStatus:     "delivering",
		Schedule:                 PromotionScheduleWindow{StartAt: now.Add(-time.Hour), EndAt: now.Add(24 * time.Hour), Timezone: "Asia/Shanghai"},
		Materials:                []PromotionMaterialReference{{ReferenceID: "asset_test", AuthorizationEvidenceID: "material_evidence_test"}},
		LandingPage:              PromotionLandingPageReference{ReferenceID: "landing_test", AuthorizationEvidenceID: "landing_evidence_test"},
	}
	current, _ := restart.statePayload(false)
	target, _ := restart.statePayload(true)
	restart.CurrentStateHash, _ = contract.CanonicalJSONHash(current)
	restart.TargetStateHash, _ = contract.CanonicalJSONHash(target)
	run.Authority.Action = "resume_promotion"
	run.Authority.ParentPlatformProjectID = "project_test"
	run.Authority.TargetMappingID = "mapping_test"
	run.Authority.TargetMappingVersion = 3
	run.Authority.TargetPlatformObjectID = "promotion_test"
	run.Authority.TargetPlatformObjectKind = "promotion"
	run.Authority.OperatorPrincipalID = "operator_1"
	run.Authority.PromotionBudgetLimitMinor = 30000
	run.Authority.BudgetLimitMinor = 30000
	run.Authority.PromotionRestart = &restart
	adapter := DeterministicFakeAdapter{Outcome: WorkerSuccess, AccountID: run.AccountID}
	prepared, err := adapter.Prepare(context.Background(), run)
	if err != nil || run.Authority.validatePreSubmitReadback(prepared.Readback, now) != nil || prepared.Readback["platform_status"] != "paused" || prepared.Readback["materials_available"] != "true" || prepared.Readback["landing_page_available"] != "true" {
		t.Fatalf("prepared=%#v err=%v", prepared, err)
	}
	_, outcome, err := adapter.Submit(context.Background(), run, ControlledActionAttempt{}, "")
	if err != nil || outcome.Readback["platform_object_id"] != "promotion_test" || outcome.Readback["platform_status"] != "delivering" || outcome.Readback["target_state_hash"] != restart.TargetStateHash {
		t.Fatalf("outcome=%#v err=%v", outcome, err)
	}
}

func fakeWorkerFixture(outcome WorkerOutcome) (Worker, Service, *MemoryRepository, BrowserRpaRun, time.Time) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	repo := NewMemoryRepository()
	counter := 0
	service := Service{Repository: repo, Now: func() time.Time { return now }, NewID: func(prefix string) (string, error) { counter++; return prefix + "_test", nil }}
	run := validRun(now)
	_, _, _ = service.CreateRun(context.Background(), CreateRunRequest{Run: run})
	_, _ = repo.AcquireLease(context.Background(), validLease(now))
	return Worker{Service: service, Adapter: DeterministicFakeAdapter{Outcome: outcome, AccountID: run.AccountID}}, service, repo, run, now
}
