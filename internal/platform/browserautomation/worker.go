package browserautomation

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type WorkerOutcome string

const (
	WorkerSuccess       WorkerOutcome = "success"
	WorkerFailed        WorkerOutcome = "failed"
	WorkerPartial       WorkerOutcome = "partial"
	WorkerResultUnknown WorkerOutcome = "result_unknown"
)

type PreparedPage struct {
	BeforeFacts map[string]string
	Readback    map[string]string
	DiffKeys    []string
	PageRef     string
	// These values identify the exact Cookies draft selected by a staged
	// create plan. The server compiler supplies them.
	InternalObjectKind string
	InternalObjectID   string
	// Evidence metadata supplied by the executing adapter. Empty values fall
	// back to the deterministic-fake provenance used by test adapters.
	ScreenshotRef   string
	SelectorVersion string
	ActionVersion   string
}

// Typed adapter failures. Worker.Prepare classifies them into stable blocking
// reasons; adapters must wrap these instead of returning free-form text.
var (
	ErrAccountMismatch          = errors.New("browser rpa account mismatch")
	ErrPageDrift                = errors.New("browser rpa page drift")
	ErrEnvironmentUnavailable   = errors.New("browser rpa environment unavailable")
	ErrFinalConfirmationInvalid = errors.New("browser rpa final confirmation invalid")
	ErrResultUnknown            = errors.New("browser rpa result unknown")
)

type WorkerAdapter interface {
	Prepare(context.Context, BrowserRpaRun) (PreparedPage, error)
	Submit(context.Context, BrowserRpaRun, ControlledActionAttempt, string) (WorkerOutcome, PreparedPage, error)
}

// WorkerSubmitGate checks deployment gates before a one-time confirmation is
// consumed. API adapters use it for feature flags and account allowlists.
type WorkerSubmitGate interface {
	CheckSubmit(BrowserRpaRun) error
}

// WorkerPlanAdapter is optional. Real Runner v3 adapters implement it to
// expose the exact prepare plan without opening a page or changing a run.
type WorkerPlanAdapter interface {
	Plan(context.Context, BrowserRpaRun) (json.RawMessage, error)
}

// WorkerResultReconciliationAdapter performs a read-only platform query. It
// must not authorize or perform a controlled action.
type WorkerResultReconciliationAdapter interface {
	ReconcileResultUnknown(context.Context, BrowserRpaRun) (PreparedPage, error)
}

const EdgeSessionProbeSchemaV1 = "browser-rpa-edge-session-probe/v1"

// EdgeSessionProbe contains only safe session facts. It never exposes a CDP
// endpoint, page URL, browser target, cookie, or account value from the page.
type EdgeSessionProbe struct {
	SchemaVersion            string    `json:"schema_version"`
	CheckedAt                time.Time `json:"checked_at"`
	Status                   string    `json:"status"`
	Reason                   string    `json:"reason"`
	CDPAvailable             bool      `json:"cdp_available"`
	OceanEnginePageAvailable bool      `json:"oceanengine_page_available"`
	LoggedIn                 bool      `json:"logged_in"`
	AccountMatched           bool      `json:"account_matched"`
}

func (p EdgeSessionProbe) Ready() bool {
	return p.Status == "ready" && p.CDPAvailable && p.OceanEnginePageAvailable && p.LoggedIn && p.AccountMatched
}

// WorkerSessionProbeAdapter is optional for legacy adapters. The production
// Runner v3 adapter implements it. Prepare performs its own page and account
// checks in the same CDP connection that applies the form plan.
type WorkerSessionProbeAdapter interface {
	CheckSession(context.Context, BrowserRpaRun) (EdgeSessionProbe, error)
}

// CreatedObjectRecorder saves a reconciled platform identity before the
// worker advances to the next staged form. A false complete value means that
// the same run has another approved form to execute.
type CreatedObjectRecorder interface {
	RecordCreatedObject(context.Context, AuthorityBinding, string, PreparedPage, string, string, time.Time) (complete bool, err error)
}

type DeterministicFakeAdapter struct {
	Outcome   WorkerOutcome
	AccountID string
}

func (a DeterministicFakeAdapter) CheckSession(_ context.Context, run BrowserRpaRun) (EdgeSessionProbe, error) {
	matched := a.AccountID == "" || a.AccountID == run.AccountID
	status, reason := "ready", "session_ready"
	if !matched {
		status, reason = "blocked", "account_mismatch"
	}
	return EdgeSessionProbe{SchemaVersion: EdgeSessionProbeSchemaV1, CheckedAt: time.Now().UTC(), Status: status, Reason: reason, CDPAvailable: true, OceanEnginePageAvailable: true, LoggedIn: true, AccountMatched: matched}, nil
}

func (a DeterministicFakeAdapter) Prepare(_ context.Context, run BrowserRpaRun) (PreparedPage, error) {
	if a.AccountID != "" && a.AccountID != run.AccountID {
		return PreparedPage{}, ErrAccountMismatch
	}
	readback := map[string]string{"account_id": run.AccountID, "object_fingerprint": run.Authority.ObjectFingerprint}
	if changesExistingPromotionAction(run.Authority.Action) {
		readback["platform_object_id"] = run.Authority.TargetPlatformObjectID
		if currentStateHash, targetStateHash, err := run.Authority.existingPromotionStateHashes(); err == nil {
			readback["current_state_hash"] = currentStateHash
			readback["target_state_hash"] = targetStateHash
		}
		if restart := run.Authority.PromotionRestart; restart != nil {
			scheduleHash, materialsHash, err := restart.readbackHashes()
			if err != nil {
				return PreparedPage{}, err
			}
			readback["platform_project_id"] = run.Authority.ParentPlatformProjectID
			readback["platform_status"] = restart.CurrentPlatformStatus
			readback["daily_budget_minor"] = strconv.FormatInt(restart.ApprovedDailyBudgetMinor, 10)
			readback["schedule_hash"] = scheduleHash
			readback["material_references_hash"] = materialsHash
			readback["landing_page_reference_id"] = restart.LandingPage.ReferenceID
			readback["materials_available"] = "true"
			readback["landing_page_available"] = "true"
		}
	}
	return PreparedPage{BeforeFacts: map[string]string{"account_id": run.AccountID, "page_kind": "review"}, Readback: readback, DiffKeys: []string{}, PageRef: "fake://oceanengine/review"}, nil
}
func (a DeterministicFakeAdapter) Submit(_ context.Context, run BrowserRpaRun, _ ControlledActionAttempt, _ string) (WorkerOutcome, PreparedPage, error) {
	outcome := a.Outcome
	if outcome == "" {
		outcome = WorkerSuccess
	}
	objectID := "fake-object-" + run.ID
	if changesExistingPromotionAction(run.Authority.Action) {
		objectID = run.Authority.TargetPlatformObjectID
	}
	targetStateHash := ""
	if _, stateHash, err := run.Authority.existingPromotionStateHashes(); err == nil {
		targetStateHash = stateHash
	}
	platformStatus := string(outcome)
	if targetStatus := run.Authority.existingPromotionTargetStatus(); targetStatus != "" && outcome == WorkerSuccess {
		platformStatus = targetStatus
	}
	page := PreparedPage{BeforeFacts: map[string]string{"object_fingerprint": run.Authority.ObjectFingerprint}, Readback: map[string]string{"platform_object_id": objectID, "platform_status": platformStatus, "target_state_hash": targetStateHash}, DiffKeys: []string{}, PageRef: "fake://oceanengine/result"}
	return outcome, page, nil
}

type Worker struct {
	Service        Service
	Adapter        WorkerAdapter
	DriverAdapters map[ExecutionDriver]WorkerAdapter
}

func (w Worker) adapterFor(run BrowserRpaRun) (WorkerAdapter, error) {
	if adapter := w.DriverAdapters[run.EffectiveExecutionDriver()]; adapter != nil {
		return adapter, nil
	}
	if run.EffectiveExecutionDriver() == ExecutionDriverPlaywrightEdgeV3 && w.Adapter != nil {
		return w.Adapter, nil
	}
	return nil, ErrEnvironmentUnavailable
}

func (w Worker) CheckSession(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, runID string) (EdgeSessionProbe, error) {
	run, err := w.Service.Repository.GetRun(ctx, org, project, runID)
	if err != nil {
		return EdgeSessionProbe{}, err
	}
	adapter, adapterErr := w.adapterFor(run)
	if adapterErr != nil {
		return EdgeSessionProbe{}, adapterErr
	}
	probe, ok := adapter.(WorkerSessionProbeAdapter)
	if !ok {
		return EdgeSessionProbe{}, ErrEnvironmentUnavailable
	}
	return probe.CheckSession(ctx, run)
}

func (w Worker) Plan(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, runID string) (json.RawMessage, error) {
	run, err := w.Service.Repository.GetRun(ctx, org, project, runID)
	if err != nil {
		return nil, err
	}
	// Plan generation is read-only. Keep it available for terminal Runs so an
	// operator can reproduce and diagnose the exact frozen Runner input.
	if run.TakeoverActive {
		return nil, ErrInvalidTransition
	}
	adapter, adapterErr := w.adapterFor(run)
	if adapterErr != nil {
		return nil, adapterErr
	}
	planner, ok := adapter.(WorkerPlanAdapter)
	if !ok {
		return nil, ErrEnvironmentUnavailable
	}
	return planner.Plan(ctx, run)
}

func (w Worker) Prepare(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, runID string) (BrowserRpaRun, error) {
	run, err := w.Service.Repository.GetRun(ctx, org, project, runID)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	if run.Paused || run.TakeoverActive {
		return BrowserRpaRun{}, ErrInvalidTransition
	}
	adapter, err := w.adapterFor(run)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	if run.State == RunQueued {
		run, err = w.Service.TransitionRun(ctx, org, project, runID, run.Version, RunEnvironmentCheck, "")
		if err != nil {
			return BrowserRpaRun{}, err
		}
	}
	if run.State != RunEnvironmentCheck {
		return BrowserRpaRun{}, ErrInvalidTransition
	}
	run, err = w.Service.TransitionRun(ctx, org, project, runID, run.Version, RunPreparing, "")
	if err != nil {
		return BrowserRpaRun{}, err
	}
	step := RunStep{ID: run.ID + "-prepare-v" + strconv.FormatInt(run.Version, 10), RunID: run.ID, Sequence: int(run.Version)*10 + 1, WorkflowStepID: run.Authority.WorkflowStepID, Action: "prepare_and_readback", Status: StepRunning, Attempt: 1, Version: 1}
	if err := w.Service.Repository.PutStep(ctx, org, project, step); err != nil {
		return BrowserRpaRun{}, err
	}
	failPrepare := func(reason BlockingReason) (BrowserRpaRun, error) {
		step.Status = StepFailed
		step.BlockingReason = reason
		step.Version++
		_ = w.Service.Repository.PutStep(ctx, org, project, step)
		return w.transitionTerminal(ctx, org, project, runID, run.Version, RunFailed, reason)
	}
	prepared, err := adapter.Prepare(ctx, run)
	if err != nil {
		reason := BlockPageDrift
		if errors.Is(err, ErrAccountMismatch) {
			reason = BlockAccountMismatch
		} else if errors.Is(err, ErrEnvironmentUnavailable) {
			reason = BlockRunnerFailure
		}
		return failPrepare(reason)
	}
	if changesExistingPromotionAction(run.Authority.Action) && run.Authority.validatePreSubmitReadback(prepared.Readback, w.Service.now()) != nil {
		return failPrepare(BlockPageDrift)
	}
	step.Status = StepSucceeded
	step.Version++
	if err := w.Service.Repository.PutStep(ctx, org, project, step); err != nil {
		return BrowserRpaRun{}, err
	}
	if err := w.appendEvidence(ctx, run, step, prepared); err != nil {
		return BrowserRpaRun{}, err
	}
	return w.Service.TransitionRun(ctx, org, project, runID, run.Version, RunAwaitingConfirmation, BlockFinalConfirmationRequired)
}

type WorkerSubmitRequest struct{ Authorize AuthorizeActionRequest }

func (w Worker) Submit(ctx context.Context, request WorkerSubmitRequest) (BrowserRpaRun, error) {
	run, err := w.Service.Repository.GetRun(ctx, request.Authorize.OrganizationID, request.Authorize.ProjectID, request.Authorize.RunID)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	adapter, err := w.adapterFor(run)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	if gate, ok := adapter.(WorkerSubmitGate); ok {
		if err := gate.CheckSubmit(run); err != nil {
			return BrowserRpaRun{}, err
		}
	}
	stepAction := "submit_platform_configuration"
	if stagedCreateAction(run.Authority.Action) {
		stepAction = string(TakeoverResultObserved)
	}
	step := RunStep{ID: request.Authorize.StepID, RunID: run.ID, Sequence: int(run.Version)*10 + 2, WorkflowStepID: run.Authority.WorkflowStepID, Action: stepAction, Status: StepPending, Attempt: 1, Version: 1}
	if err := w.Service.Repository.PutStep(ctx, run.OrganizationID, run.ProjectID, step); err != nil {
		return BrowserRpaRun{}, err
	}
	attempt, err := w.Service.AuthorizeAction(ctx, request.Authorize)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	run, err = w.Service.TransitionRun(ctx, run.OrganizationID, run.ProjectID, run.ID, run.Version, RunSubmitting, "")
	if err != nil {
		return BrowserRpaRun{}, err
	}
	step.Status = StepRunning
	step.Version++
	if err := w.Service.Repository.PutStep(ctx, run.OrganizationID, run.ProjectID, step); err != nil {
		return BrowserRpaRun{}, err
	}
	outcome, page, adapterErr := adapter.Submit(ctx, run, attempt, request.Authorize.Token)
	if adapterErr != nil {
		step.Status = StepFailed
		step.Version++
		_ = w.Service.Repository.PutStep(ctx, run.OrganizationID, run.ProjectID, step)
		_ = w.appendEvidence(ctx, run, step, page)
		if err := w.Service.Repository.CompleteControlledAction(ctx, run.OrganizationID, run.ProjectID, attempt.ID, ControlledActionFailed); err != nil {
			return BrowserRpaRun{}, err
		}
		reason := BlockPageDrift
		if errors.Is(adapterErr, ErrAccountMismatch) {
			reason = BlockAccountMismatch
		} else if errors.Is(adapterErr, ErrEnvironmentUnavailable) {
			reason = BlockResultReconciliation
		} else if errors.Is(adapterErr, ErrFinalConfirmationInvalid) {
			reason = BlockFinalConfirmationInvalid
		}
		return w.transitionTerminal(ctx, run.OrganizationID, run.ProjectID, run.ID, run.Version, RunFailed, reason)
	}
	if outcome == WorkerResultUnknown {
		step.Status = StepResultUnknown
		step.Version++
		_ = w.Service.Repository.PutStep(ctx, run.OrganizationID, run.ProjectID, step)
		_ = w.appendEvidence(ctx, run, step, page)
		if err := w.Service.Repository.CompleteControlledAction(ctx, run.OrganizationID, run.ProjectID, attempt.ID, ControlledActionResultUnknown); err != nil {
			return BrowserRpaRun{}, err
		}
		return w.transitionTerminal(ctx, run.OrganizationID, run.ProjectID, run.ID, run.Version, RunResultUnknown, BlockResultReconciliation)
	}
	attemptStatus := ControlledActionVerified
	if outcome == WorkerFailed {
		attemptStatus = ControlledActionFailed
	}
	if err := w.Service.Repository.CompleteControlledAction(ctx, run.OrganizationID, run.ProjectID, attempt.ID, attemptStatus); err != nil {
		return BrowserRpaRun{}, err
	}
	run, err = w.Service.TransitionRun(ctx, run.OrganizationID, run.ProjectID, run.ID, run.Version, RunVerifying, "")
	if err != nil {
		return BrowserRpaRun{}, err
	}
	step.Version++
	step.Status = StepSucceeded
	if outcome == WorkerFailed {
		step.Status = StepFailed
	}
	if err := w.Service.Repository.PutStep(ctx, run.OrganizationID, run.ProjectID, step); err != nil {
		return BrowserRpaRun{}, err
	}
	stagedObjectObserved := stagedCreateAction(run.Authority.Action) && (outcome == WorkerSuccess || outcome == WorkerPartial) && page.InternalObjectID != ""
	if stagedObjectObserved {
		if page.Readback == nil {
			page.Readback = map[string]string{}
		}
		if page.Readback["platform_status"] == "" {
			page.Readback["platform_status"] = "pending_review"
		}
	}
	resultEvidenceID, err := w.appendEvidenceWithID(ctx, run, step, page)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	if stagedObjectObserved {
		reconciliationStep := RunStep{ID: step.ID + "-reconcile", RunID: run.ID, Sequence: step.Sequence + 1, WorkflowStepID: run.Authority.WorkflowStepID, Action: string(TakeoverListConfirmed), Status: StepSucceeded, Attempt: 1, Version: 1}
		if err := w.Service.Repository.PutStep(ctx, run.OrganizationID, run.ProjectID, reconciliationStep); err != nil {
			return BrowserRpaRun{}, err
		}
		listEvidenceID, evidenceErr := w.appendEvidenceWithID(ctx, run, reconciliationStep, page)
		if evidenceErr != nil {
			return BrowserRpaRun{}, evidenceErr
		}
		recorder, ok := w.Service.AuthorityProvider.(CreatedObjectRecorder)
		if !ok {
			return w.transitionTerminal(ctx, run.OrganizationID, run.ProjectID, run.ID, run.Version, RunFailed, BlockResultReconciliation)
		}
		complete, recordErr := recorder.RecordCreatedObject(ctx, run.Authority, run.ID, page, resultEvidenceID, listEvidenceID, w.Service.now())
		if recordErr != nil {
			return w.transitionTerminal(ctx, run.OrganizationID, run.ProjectID, run.ID, run.Version, RunFailed, BlockResultReconciliation)
		}
		if outcome == WorkerSuccess && !complete {
			run, releaseErr := w.releaseRunLease(ctx, run)
			if releaseErr != nil {
				return run, releaseErr
			}
			return w.Service.TransitionRun(ctx, run.OrganizationID, run.ProjectID, run.ID, run.Version, RunEnvironmentCheck, "")
		}
	}
	terminal := RunSucceeded
	reason := BlockingReason("")
	if outcome == WorkerFailed {
		terminal = RunFailed
		if page.Readback["final_click_performed"] == "true" && page.Readback["reconciliation"] == "not_found" && page.Readback["platform_write_request_observed"] == "false" {
			reason = BlockTargetEffectNotObserved
		}
	}
	if outcome == WorkerPartial {
		terminal = RunPartial
		reason = BlockResultReconciliation
	}
	return w.transitionTerminal(ctx, run.OrganizationID, run.ProjectID, run.ID, run.Version, terminal, reason)
}

func (w Worker) releaseRunLease(ctx context.Context, run BrowserRpaRun) (BrowserRpaRun, error) {
	if run.LeaseID == "" {
		return run, nil
	}
	lease, err := w.Service.Repository.GetLease(ctx, run.OrganizationID, run.ProjectID, run.LeaseID)
	if err != nil {
		return run, err
	}
	if lease.ReleasedAt != nil {
		return run, ErrLeaseUnavailable
	}
	released, err := w.Service.ReleaseRunLease(ctx, run.OrganizationID, run.ProjectID, run.ID, lease.ID, run.Version, lease.Version, lease.FencingToken)
	if err != nil {
		return run, err
	}
	return released.Run, nil
}

func (w Worker) transitionTerminal(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, runID string, expected int64, state RunState, reason BlockingReason) (BrowserRpaRun, error) {
	run, err := w.Service.TransitionRun(ctx, org, project, runID, expected, state, reason)
	if err != nil || run.LeaseID == "" {
		return run, err
	}
	lease, leaseErr := w.Service.Repository.GetLease(ctx, org, project, run.LeaseID)
	if leaseErr != nil || lease.ReleasedAt != nil {
		return run, nil
	}
	released, releaseErr := w.Service.ReleaseRunLease(ctx, org, project, run.ID, lease.ID, run.Version, lease.Version, lease.FencingToken)
	if releaseErr != nil {
		// The run is already terminal. An expired lease can be reclaimed by the
		// next acquisition. Do not replace the terminal result with a 500.
		return run, nil
	}
	return released.Run, nil
}

func (w Worker) appendEvidence(ctx context.Context, run BrowserRpaRun, step RunStep, page PreparedPage) error {
	_, err := w.appendEvidenceWithID(ctx, run, step, page)
	return err
}

func (w Worker) appendEvidenceWithID(ctx context.Context, run BrowserRpaRun, step RunStep, page PreparedPage) (string, error) {
	now := w.Service.now()
	id, err := w.Service.newID(browserRpaEvidenceIDPrefix)
	if err != nil {
		return "", err
	}
	selectorVersion := page.SelectorVersion
	if selectorVersion == "" {
		selectorVersion = "deterministic-fake-selector/v1"
	}
	actionVersion := page.ActionVersion
	if actionVersion == "" {
		actionVersion = "deterministic-fake-action/v1"
	}
	fingerprint := run.Authority.ObjectFingerprint
	if page.InternalObjectID != "" {
		fingerprint = page.InternalObjectID
	}
	evidence := Evidence{SchemaVersion: EvidenceSchemaV1, ID: id, OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, RunID: run.ID, StepID: step.ID, BeforePageFacts: page.BeforeFacts, AfterPageFacts: page.Readback, FieldReadback: page.Readback, DiffKeys: page.DiffKeys, PageReference: page.PageRef, ScreenshotReference: page.ScreenshotRef, ObjectFingerprint: fingerprint, SelectorVersion: selectorVersion, ActionVersion: actionVersion, CreatedAt: now}
	if err := w.Service.Repository.AppendEvidence(ctx, RedactEvidence(evidence)); err != nil {
		return "", err
	}
	return id, nil
}

// ReconcileResultUnknown records a read-only platform query after one final
// click produced an unknown result. It never authorizes or performs another
// controlled action.
func (w Worker) ReconcileResultUnknown(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, runID string, page PreparedPage) (BrowserRpaRun, error) {
	run, err := w.Service.Repository.GetRun(ctx, org, project, runID)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	reconciliation := page.Readback["reconciliation"]
	matched := reconciliation == "matched"
	confirmedNoEffect := reconciliation == "not_found" && page.Readback["read_only_reconciliation"] == "true" && page.Readback["platform_write_performed"] == "false" && page.Readback["exact_name_matches"] == "0"
	if run.State != RunResultUnknown || run.LeaseID != "" || !stagedCreateAction(run.Authority.Action) || page.InternalObjectID == "" || (page.InternalObjectKind != "project" && page.InternalObjectKind != "promotion") || (!matched && !confirmedNoEffect) {
		return BrowserRpaRun{}, ErrInvalidTransition
	}
	if matched && (!numericReadbackID(page.Readback["platform_object_id"]) || page.Readback["field_reconciliation_status"] == "not_checked") {
		return BrowserRpaRun{}, ErrInvalidTransition
	}
	steps, err := w.Service.Repository.ListSteps(ctx, org, project, run.ID)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	evidence, err := w.Service.Repository.ListEvidence(ctx, org, project, run.ID)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	unknownStepIDs := map[string]struct{}{}
	for _, step := range steps {
		if step.Status == StepResultUnknown {
			unknownStepIDs[step.ID] = struct{}{}
		}
	}
	clicked := false
	for _, item := range evidence {
		if _, ok := unknownStepIDs[item.StepID]; ok && item.FieldReadback["final_click_performed"] == "true" {
			clicked = true
			break
		}
	}
	if matched && !clicked {
		return BrowserRpaRun{}, ErrInvalidTransition
	}
	resultStep := RunStep{ID: run.ID + "-reidentified-v" + strconv.FormatInt(run.Version, 10), RunID: run.ID, Sequence: int(run.Version)*10 + 1, WorkflowStepID: run.Authority.WorkflowStepID, Action: string(TakeoverResultObserved), Status: StepSucceeded, Attempt: 1, Version: 1}
	if err := w.Service.Repository.PutStep(ctx, org, project, resultStep); err != nil {
		return BrowserRpaRun{}, err
	}
	resultEvidenceID, err := w.appendEvidenceWithID(ctx, run, resultStep, page)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	listStep := RunStep{ID: resultStep.ID + "-list", RunID: run.ID, Sequence: resultStep.Sequence + 1, WorkflowStepID: run.Authority.WorkflowStepID, Action: string(TakeoverListConfirmed), Status: StepSucceeded, Attempt: 1, Version: 1}
	if err := w.Service.Repository.PutStep(ctx, org, project, listStep); err != nil {
		return BrowserRpaRun{}, err
	}
	listEvidenceID, err := w.appendEvidenceWithID(ctx, run, listStep, page)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	if confirmedNoEffect {
		return w.Service.TransitionRun(ctx, org, project, run.ID, run.Version, RunFailed, BlockTargetEffectNotObserved)
	}
	recorder, ok := w.Service.AuthorityProvider.(CreatedObjectRecorder)
	if !ok {
		return BrowserRpaRun{}, ErrInvalidContract
	}
	complete, err := recorder.RecordCreatedObject(ctx, run.Authority, run.ID, page, resultEvidenceID, listEvidenceID, w.Service.now())
	if err != nil {
		return BrowserRpaRun{}, err
	}
	next := RunEnvironmentCheck
	if complete {
		next = RunSucceeded
	}
	return w.Service.TransitionRun(ctx, org, project, run.ID, run.Version, next, "")
}

// ReconcileUnknownFromPlatform runs the adapter's query-only workflow and
// records its result. It never issues a final confirmation or a submit token.
func (w Worker) ReconcileUnknownFromPlatform(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, runID string) (BrowserRpaRun, error) {
	run, err := w.Service.Repository.GetRun(ctx, org, project, runID)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	if run.State != RunResultUnknown || run.LeaseID != "" {
		return BrowserRpaRun{}, ErrInvalidTransition
	}
	adapter, ok := w.Adapter.(WorkerResultReconciliationAdapter)
	if !ok {
		return BrowserRpaRun{}, ErrInvalidContract
	}
	page, err := adapter.ReconcileResultUnknown(ctx, run)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	return w.ReconcileResultUnknown(ctx, org, project, runID, page)
}

func numericReadbackID(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func stagedCreateAction(action string) bool {
	return action == "create_project_and_promotions" || action == "create_promotions_in_existing_project"
}

type ControlAction string

const (
	ControlPause           ControlAction = "pause"
	ControlResume          ControlAction = "resume"
	ControlCancel          ControlAction = "cancel"
	ControlTakeover        ControlAction = "takeover"
	ControlReleaseTakeover ControlAction = "release_takeover"
)

func (s Service) ControlRun(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, runID string, expected int64, action ControlAction) (BrowserRpaRun, error) {
	run, err := s.Repository.GetRun(ctx, org, project, runID)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	if run.Version != expected {
		return BrowserRpaRun{}, ErrVersionConflict
	}
	switch action {
	case ControlPause:
		if terminalState(run.State) {
			return BrowserRpaRun{}, ErrInvalidTransition
		}
		return s.setRunControl(ctx, run, expected, run.State, true, false, run.BlockingReason, action)
	case ControlTakeover:
		if terminalState(run.State) {
			return BrowserRpaRun{}, ErrInvalidTransition
		}
		return s.setRunControl(ctx, run, expected, RunAwaitingTakeover, true, true, run.BlockingReason, action)
	case ControlReleaseTakeover, ControlResume:
		if !run.Paused && !run.TakeoverActive {
			return BrowserRpaRun{}, ErrInvalidTransition
		}
		return s.setRunControl(ctx, run, expected, RunEnvironmentCheck, false, false, "", action)
	case ControlCancel:
		if terminalState(run.State) {
			return BrowserRpaRun{}, ErrInvalidTransition
		}
		return s.setRunControl(ctx, run, expected, RunCancelled, false, false, "", action)
	default:
		return BrowserRpaRun{}, ErrInvalidContract
	}
}
func (s Service) setRunControl(ctx context.Context, run BrowserRpaRun, expected int64, state RunState, paused, takeover bool, reason BlockingReason, action ControlAction) (BrowserRpaRun, error) {
	updated, err := s.Repository.SetRunControl(ctx, run.OrganizationID, run.ProjectID, run.ID, expected, state, paused, takeover, reason, s.now())
	if err != nil {
		return BrowserRpaRun{}, err
	}
	if err := s.recordEvent(ctx, updated, "control_"+string(action), string(action), run.CreatedBy); err != nil {
		return BrowserRpaRun{}, err
	}
	return updated, nil
}
func terminalState(state RunState) bool {
	switch state {
	case RunSucceeded, RunFailed, RunPartial, RunResultUnknown, RunCancelled:
		return true
	default:
		return false
	}
}
