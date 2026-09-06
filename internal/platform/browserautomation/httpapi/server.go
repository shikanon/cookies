package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/identity"
)

const maxBody = 1 << 20

// APIDefaultPrefix is the current namespace. APILegacyPrefix is the
// historical computer-use namespace, kept as a transitional alias so
// existing runbooks and calibration records keep resolving for one
// release before removal.
const (
	APIDefaultPrefix = "/api/platform/v1/browser-rpa"
	APILegacyPrefix  = "/api/platform/v1/computer-use"
)

type Server struct {
	service         browserautomation.Service
	worker          browserautomation.Worker
	projects        identity.ProjectAuthorizer
	mux             *http.ServeMux
	automatedWorker bool
}

func New(service browserautomation.Service, worker browserautomation.Worker, projects identity.ProjectAuthorizer) *Server {
	return newServer(service, worker, projects, true)
}

// NewTakeoverOnly exposes the production-visible control plane without
// mounting a fake or unattended browser adapter. Real page actions advance
// only through the fenced takeover-evidence port.
func NewTakeoverOnly(service browserautomation.Service, projects identity.ProjectAuthorizer) *Server {
	return newServer(service, browserautomation.Worker{}, projects, false)
}

// MountLegacyAlias additionally registers the historical computer-use
// prefix on the same handlers as a transitional compatibility alias.
func (s *Server) MountLegacyAlias() { s.registerRoutes(APILegacyPrefix) }

func newServer(service browserautomation.Service, worker browserautomation.Worker, projects identity.ProjectAuthorizer, automatedWorker bool) *Server {
	s := &Server{service: service, worker: worker, projects: projects, mux: http.NewServeMux(), automatedWorker: automatedWorker}
	s.registerRoutes(APIDefaultPrefix)
	return s
}

func (s *Server) registerRoutes(prefix string) {
	s.mux.HandleFunc("PUT "+prefix+"/kill-switches/{scope}", s.setKillSwitch)
	s.mux.HandleFunc("POST "+prefix+"/projects/{project_id}/environments", s.registerEnvironment)
	s.mux.HandleFunc("GET "+prefix+"/projects/{project_id}/environments/{environment_id}", s.getEnvironment)
	s.mux.HandleFunc("POST "+prefix+"/projects/{project_id}/browser-profiles", s.registerBrowserProfile)
	s.mux.HandleFunc("GET "+prefix+"/projects/{project_id}/browser-profiles/{profile_id}", s.getBrowserProfile)
	s.mux.HandleFunc("POST "+prefix+"/projects/{project_id}/site-policies", s.registerSitePolicy)
	s.mux.HandleFunc("GET "+prefix+"/projects/{project_id}/site-policies/{policy_id}", s.getSitePolicy)
	s.mux.HandleFunc("GET "+prefix+"/projects/{project_id}/kill-switches/active", s.getActiveKillSwitch)
	s.mux.HandleFunc("POST "+prefix+"/projects/{project_id}/runs", s.createRun)
	s.mux.HandleFunc("GET "+prefix+"/projects/{project_id}/runs", s.listRuns)
	s.mux.HandleFunc("GET "+prefix+"/projects/{project_id}/runs/{run_id}", s.getRun)
	s.mux.HandleFunc("GET "+prefix+"/projects/{project_id}/runs/{run_id}/steps", s.listSteps)
	s.mux.HandleFunc("GET "+prefix+"/projects/{project_id}/runs/{run_id}/events", s.listEvents)
	s.mux.HandleFunc("GET "+prefix+"/projects/{project_id}/runs/{run_id}/evidence", s.listEvidence)
	s.mux.HandleFunc("POST "+prefix+"/projects/{project_id}/runs/{run_id}/takeover-evidence", s.recordTakeoverEvidence)
	s.mux.HandleFunc("POST "+prefix+"/projects/{project_id}/runs/{run_id}/takeover-action-attempts", s.authorizeTakeoverAction)
	s.mux.HandleFunc("POST "+prefix+"/projects/{project_id}/runs/{run_id}/takeover-action-attempts/{attempt_action}", s.takeoverActionAttemptCommand)
	s.mux.HandleFunc("POST "+prefix+"/projects/{project_id}/runs/{run_id}/leases", s.acquireRunLease)
	s.mux.HandleFunc("GET "+prefix+"/projects/{project_id}/runs/{run_id}/leases/{lease_id}", s.getRunLease)
	s.mux.HandleFunc("POST "+prefix+"/projects/{project_id}/runs/{run_id}/leases/{lease_action}", s.runLeaseCommand)
	s.mux.HandleFunc("POST "+prefix+"/projects/{project_id}/runs/{run_id}/confirmations", s.confirm)
	s.mux.HandleFunc("POST "+prefix+"/projects/{project_id}/runs/{run_action}", s.command)
}

func (s *Server) command(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(r.PathValue("run_action"), ":", 2)
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	r.SetPathValue("run_id", parts[0])
	r.SetPathValue("action", parts[1])
	switch parts[1] {
	case "check-session":
		if !s.automatedWorker {
			writeError(w, http.StatusNotFound, "automated worker is not mounted")
			return
		}
		s.checkSession(w, r)
	case "plan":
		if !s.automatedWorker {
			writeError(w, http.StatusNotFound, "automated worker is not mounted")
			return
		}
		s.plan(w, r)
	case "prepare":
		if !s.automatedWorker {
			writeError(w, http.StatusNotFound, "automated worker is not mounted")
			return
		}
		s.prepare(w, r)
	case "submit":
		if !s.automatedWorker {
			writeError(w, http.StatusNotFound, "automated worker is not mounted")
			return
		}
		s.submit(w, r)
	case "reconcile-result":
		if !s.automatedWorker {
			writeError(w, http.StatusNotFound, "automated worker is not mounted")
			return
		}
		s.reconcileResult(w, r)
	case "pause", "resume", "cancel", "takeover", "release_takeover":
		s.control(w, r)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (s *Server) checkSession(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.execute")
	if !ok {
		return
	}
	value, err := s.worker.CheckSession(r.Context(), actor.OrganizationID, project, r.PathValue("run_id"))
	writeResult(w, value, err)
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.read")
	if !ok {
		return
	}
	values, err := s.service.ListRuns(r.Context(), actor.OrganizationID, project)
	writeResult(w, map[string]any{"items": values}, err)
}

func (s *Server) listSteps(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.read")
	if !ok {
		return
	}
	values, err := s.service.Repository.ListSteps(r.Context(), actor.OrganizationID, project, r.PathValue("run_id"))
	writeResult(w, map[string]any{"items": values}, err)
}

func (s *Server) plan(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.read")
	if !ok {
		return
	}
	value, err := s.worker.Plan(r.Context(), actor.OrganizationID, project, r.PathValue("run_id"))
	writeResult(w, value, err)
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) setKillSwitch(w http.ResponseWriter, r *http.Request) {
	rc, ok := contract.RequestContextFrom(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var body struct {
		Platform        browserautomation.Platform `json:"platform"`
		Active          bool                       `json:"active"`
		Reason          string                     `json:"reason"`
		ExpectedVersion int64                      `json:"expected_version"`
	}
	if !decode(w, r, &body) {
		return
	}
	value, err := s.service.SetKillSwitch(r.Context(), rc.Actor, browserautomation.KillSwitchScope(r.PathValue("scope")), body.Platform, body.Active, body.Reason, body.ExpectedVersion)
	writeResult(w, value, err)
}

func (s *Server) getActiveKillSwitch(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := s.authorize(w, r, "delivery.read")
	if !ok {
		return
	}
	platform := browserautomation.Platform(r.URL.Query().Get("platform"))
	if platform != browserautomation.PlatformOceanEngine {
		writeError(w, http.StatusBadRequest, "invalid platform")
		return
	}
	value, active, err := s.service.Repository.ActiveKillSwitch(r.Context(), actor.OrganizationID, platform)
	var selected any
	if active {
		selected = value
	}
	writeResult(w, map[string]any{"active": active, "kill_switch": selected}, err)
}

func (s *Server) registerEnvironment(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.execute")
	if !ok {
		return
	}
	var body browserautomation.ExecutionEnvironment
	if !decode(w, r, &body) {
		return
	}
	if body.OrganizationID != "" || body.ProjectID != "" || body.Version != 0 {
		writeError(w, http.StatusBadRequest, "server-owned fields are not accepted")
		return
	}
	value, err := s.service.RegisterEnvironment(r.Context(), actor.OrganizationID, project, body)
	writeCreated(w, value, err)
}

func (s *Server) getEnvironment(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.read")
	if !ok {
		return
	}
	value, err := s.service.Repository.GetEnvironment(r.Context(), actor.OrganizationID, project, r.PathValue("environment_id"))
	writeResult(w, value, err)
}

func (s *Server) registerBrowserProfile(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.execute")
	if !ok {
		return
	}
	var body browserautomation.BrowserProfile
	if !decode(w, r, &body) {
		return
	}
	if body.OrganizationID != "" || body.ProjectID != "" || body.Version != 0 {
		writeError(w, http.StatusBadRequest, "server-owned fields are not accepted")
		return
	}
	value, err := s.service.RegisterBrowserProfile(r.Context(), actor.OrganizationID, project, body)
	writeCreated(w, value, err)
}

func (s *Server) getBrowserProfile(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.read")
	if !ok {
		return
	}
	value, err := s.service.Repository.GetBrowserProfile(r.Context(), actor.OrganizationID, project, r.PathValue("profile_id"))
	writeResult(w, value, err)
}

func (s *Server) registerSitePolicy(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.execute")
	if !ok {
		return
	}
	var body browserautomation.SitePolicy
	if !decode(w, r, &body) {
		return
	}
	if body.OrganizationID != "" || body.ProjectID != "" || body.Version != 0 {
		writeError(w, http.StatusBadRequest, "server-owned fields are not accepted")
		return
	}
	value, err := s.service.RegisterSitePolicy(r.Context(), actor.OrganizationID, project, body)
	writeCreated(w, value, err)
}

func (s *Server) getSitePolicy(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.read")
	if !ok {
		return
	}
	value, err := s.service.Repository.GetSitePolicy(r.Context(), actor.OrganizationID, project, r.PathValue("policy_id"))
	writeResult(w, value, err)
}

func (s *Server) createRun(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.execute")
	if !ok {
		return
	}
	var body struct {
		ProjectID     contract.ProjectID         `json:"project_id"`
		Platform      browserautomation.Platform `json:"platform"`
		AccountID     string                     `json:"account_id"`
		ExecutionID   string                     `json:"business_execution_id"`
		EnvironmentID string                     `json:"environment_id"`
		ProfileID     string                     `json:"profile_id"`
		PolicyID      string                     `json:"policy_id"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.ProjectID != project {
		writeError(w, http.StatusBadRequest, "project mismatch")
		return
	}
	value, replayed, err := s.service.CreateBoundRun(r.Context(), browserautomation.CreateBoundRunRequest{OrganizationID: actor.OrganizationID, ProjectID: project, Platform: body.Platform, AccountID: body.AccountID, ExecutionID: body.ExecutionID, EnvironmentID: body.EnvironmentID, ProfileID: body.ProfileID, PolicyID: body.PolicyID, IdempotencyKey: r.Header.Get("Idempotency-Key"), CreatedBy: actor.Principal.ID})
	if err != nil {
		writeResult(w, browserautomation.BrowserRpaRun{}, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if !replayed {
		w.WriteHeader(http.StatusCreated)
	}
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request, scope contract.Scope) (contract.ActorContext, contract.ProjectID, bool) {
	rc, ok := contract.RequestContextFrom(r.Context())
	if !ok || !rc.Actor.HasScope(scope) {
		writeError(w, http.StatusForbidden, "forbidden")
		return contract.ActorContext{}, "", false
	}
	project := contract.ProjectID(r.PathValue("project_id"))
	if s.projects == nil || s.projects.AuthorizeProject(r.Context(), rc.Actor, project) != nil {
		writeError(w, http.StatusForbidden, "project access denied")
		return contract.ActorContext{}, "", false
	}
	return rc.Actor, project, true
}
func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.read")
	if !ok {
		return
	}
	value, err := s.service.Repository.GetRun(r.Context(), actor.OrganizationID, project, r.PathValue("run_id"))
	writeResult(w, value, err)
}
func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.read")
	if !ok {
		return
	}
	values, err := s.service.Repository.ListEvents(r.Context(), actor.OrganizationID, project, r.PathValue("run_id"))
	writeResult(w, map[string]any{"items": values}, err)
}
func (s *Server) listEvidence(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.read")
	if !ok {
		return
	}
	values, err := s.service.Repository.ListEvidence(r.Context(), actor.OrganizationID, project, r.PathValue("run_id"))
	writeResult(w, map[string]any{"items": values}, err)
}
func (s *Server) recordTakeoverEvidence(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.execute")
	if !ok {
		return
	}
	var body struct {
		ExpectedVersion   int64                                    `json:"expected_version"`
		LeaseID           string                                   `json:"lease_id"`
		FencingToken      int64                                    `json:"fencing_token"`
		StepID            string                                   `json:"step_id"`
		Sequence          int                                      `json:"sequence"`
		Action            browserautomation.TakeoverEvidenceAction `json:"action"`
		Status            browserautomation.StepStatus             `json:"status"`
		PageKind          string                                   `json:"page_kind"`
		PlatformProjectID string                                   `json:"platform_project_id"`
		BeforePageFacts   map[string]string                        `json:"before_page_facts"`
		AfterPageFacts    map[string]string                        `json:"after_page_facts"`
		FieldReadback     map[string]string                        `json:"field_readback"`
		DiffKeys          []string                                 `json:"diff_keys"`
		PageReference     string                                   `json:"page_reference"`
		SelectorVersion   string                                   `json:"selector_version"`
		ActionVersion     string                                   `json:"action_version"`
	}
	if !decode(w, r, &body) {
		return
	}
	value, err := s.service.RecordTakeoverEvidence(r.Context(), browserautomation.RecordTakeoverEvidenceRequest{OrganizationID: actor.OrganizationID, ProjectID: project, RunID: r.PathValue("run_id"), ExpectedVersion: body.ExpectedVersion, LeaseID: body.LeaseID, FencingToken: body.FencingToken, StepID: body.StepID, Sequence: body.Sequence, Action: body.Action, Status: body.Status, PageKind: body.PageKind, PlatformProjectID: body.PlatformProjectID, BeforePageFacts: body.BeforePageFacts, AfterPageFacts: body.AfterPageFacts, FieldReadback: body.FieldReadback, DiffKeys: body.DiffKeys, PageReference: body.PageReference, SelectorVersion: body.SelectorVersion, ActionVersion: body.ActionVersion, Actor: actor.Principal.ID})
	writeResult(w, value, err)
}

func (s *Server) authorizeTakeoverAction(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.execute")
	if !ok {
		return
	}
	var body struct {
		ExpectedVersion   int64             `json:"expected_version"`
		StepID            string            `json:"step_id"`
		Sequence          int               `json:"sequence"`
		ConfirmationID    string            `json:"confirmation_id"`
		ConfirmationToken string            `json:"confirmation_token"`
		LeaseID           string            `json:"lease_id"`
		FencingToken      int64             `json:"fencing_token"`
		PageKind          string            `json:"page_kind"`
		PlatformProjectID string            `json:"platform_project_id"`
		BeforePageFacts   map[string]string `json:"before_page_facts"`
		FieldReadback     map[string]string `json:"field_readback"`
		DiffKeys          []string          `json:"diff_keys"`
		PageReference     string            `json:"page_reference"`
		SelectorVersion   string            `json:"selector_version"`
		ActionVersion     string            `json:"action_version"`
	}
	if !decode(w, r, &body) {
		return
	}
	value, err := s.service.AuthorizeTakeoverAction(r.Context(), browserautomation.AuthorizeTakeoverActionRequest{OrganizationID: actor.OrganizationID, ProjectID: project, RunID: r.PathValue("run_id"), ExpectedVersion: body.ExpectedVersion, StepID: body.StepID, Sequence: body.Sequence, ConfirmationID: body.ConfirmationID, Token: body.ConfirmationToken, LeaseID: body.LeaseID, FencingToken: body.FencingToken, IdempotencyKey: r.Header.Get("Idempotency-Key"), PageKind: body.PageKind, PlatformProjectID: body.PlatformProjectID, BeforePageFacts: body.BeforePageFacts, FieldReadback: body.FieldReadback, DiffKeys: body.DiffKeys, PageReference: body.PageReference, SelectorVersion: body.SelectorVersion, ActionVersion: body.ActionVersion, Actor: actor.Principal.ID})
	writeResult(w, value, err)
}

func (s *Server) takeoverActionAttemptCommand(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(r.PathValue("attempt_action"), ":", 2)
	if len(parts) != 2 || parts[1] != "outcome" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	r.SetPathValue("attempt_id", parts[0])
	actor, project, ok := s.authorize(w, r, "delivery.execute")
	if !ok {
		return
	}
	var body struct {
		ExpectedVersion   int64                                  `json:"expected_version"`
		LeaseID           string                                 `json:"lease_id"`
		FencingToken      int64                                  `json:"fencing_token"`
		StepID            string                                 `json:"step_id"`
		Sequence          int                                    `json:"sequence"`
		Outcome           browserautomation.TakeoverWriteOutcome `json:"outcome"`
		PageKind          string                                 `json:"page_kind"`
		PlatformProjectID string                                 `json:"platform_project_id"`
		BeforePageFacts   map[string]string                      `json:"before_page_facts"`
		AfterPageFacts    map[string]string                      `json:"after_page_facts"`
		FieldReadback     map[string]string                      `json:"field_readback"`
		PageReference     string                                 `json:"page_reference"`
		SelectorVersion   string                                 `json:"selector_version"`
		ActionVersion     string                                 `json:"action_version"`
	}
	if !decode(w, r, &body) {
		return
	}
	value, err := s.service.RecordTakeoverOutcome(r.Context(), browserautomation.RecordTakeoverOutcomeRequest{OrganizationID: actor.OrganizationID, ProjectID: project, RunID: r.PathValue("run_id"), AttemptID: r.PathValue("attempt_id"), ExpectedVersion: body.ExpectedVersion, LeaseID: body.LeaseID, FencingToken: body.FencingToken, StepID: body.StepID, Sequence: body.Sequence, Outcome: body.Outcome, PageKind: body.PageKind, PlatformProjectID: body.PlatformProjectID, BeforePageFacts: body.BeforePageFacts, AfterPageFacts: body.AfterPageFacts, FieldReadback: body.FieldReadback, PageReference: body.PageReference, SelectorVersion: body.SelectorVersion, ActionVersion: body.ActionVersion, Actor: actor.Principal.ID})
	writeResult(w, value, err)
}
func (s *Server) acquireRunLease(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.execute")
	if !ok {
		return
	}
	var body struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if !decode(w, r, &body) {
		return
	}
	value, err := s.service.AcquireRunLease(r.Context(), actor.OrganizationID, project, r.PathValue("run_id"), body.ExpectedVersion, actor.Principal.ID)
	writeResult(w, value, err)
}
func (s *Server) getRunLease(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.execute")
	if !ok {
		return
	}
	lease, err := s.service.Repository.GetLease(r.Context(), actor.OrganizationID, project, r.PathValue("lease_id"))
	if err == nil && lease.RunID != r.PathValue("run_id") {
		err = browserautomation.ErrNotFound
	}
	writeResult(w, lease, err)
}
func (s *Server) runLeaseCommand(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(r.PathValue("lease_action"), ":", 2)
	if len(parts) != 2 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	r.SetPathValue("lease_id", parts[0])
	switch parts[1] {
	case "heartbeat":
		s.heartbeatRunLease(w, r)
	case "release":
		s.releaseRunLease(w, r)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}
func (s *Server) heartbeatRunLease(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.execute")
	if !ok {
		return
	}
	var body struct {
		ExpectedVersion int64 `json:"expected_version"`
		FencingToken    int64 `json:"fencing_token"`
	}
	if !decode(w, r, &body) {
		return
	}
	value, err := s.service.HeartbeatRunLease(r.Context(), actor.OrganizationID, project, r.PathValue("run_id"), r.PathValue("lease_id"), body.ExpectedVersion, body.FencingToken)
	writeResult(w, value, err)
}
func (s *Server) releaseRunLease(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.execute")
	if !ok {
		return
	}
	var body struct {
		ExpectedRunVersion   int64 `json:"expected_run_version"`
		ExpectedLeaseVersion int64 `json:"expected_lease_version"`
		FencingToken         int64 `json:"fencing_token"`
	}
	if !decode(w, r, &body) {
		return
	}
	value, err := s.service.ReleaseRunLease(r.Context(), actor.OrganizationID, project, r.PathValue("run_id"), r.PathValue("lease_id"), body.ExpectedRunVersion, body.ExpectedLeaseVersion, body.FencingToken)
	writeResult(w, value, err)
}
func (s *Server) prepare(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.execute")
	if !ok {
		return
	}
	// A browser action belongs to the server-side Run. It must continue when
	// the operator leaves the page or the browser closes the HTTP request.
	value, err := s.worker.Prepare(context.WithoutCancel(r.Context()), actor.OrganizationID, project, r.PathValue("run_id"))
	writeResult(w, value, err)
}
func (s *Server) control(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.execute")
	if !ok {
		return
	}
	var body struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if !decode(w, r, &body) {
		return
	}
	action := browserautomation.ControlAction(r.PathValue("action"))
	value, err := s.service.ControlRun(r.Context(), actor.OrganizationID, project, r.PathValue("run_id"), body.ExpectedVersion, action)
	writeResult(w, value, err)
}
func (s *Server) confirm(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.execute")
	if !ok {
		return
	}
	var body struct {
		ExpectedVersion int64  `json:"expected_version"`
		BindingHash     string `json:"binding_hash"`
	}
	if !decode(w, r, &body) {
		return
	}
	value, err := s.service.IssueFinalConfirmation(r.Context(), actor.OrganizationID, project, r.PathValue("run_id"), body.ExpectedVersion, body.BindingHash, actor.Principal.ID)
	writeResult(w, value, err)
}
func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.execute")
	if !ok {
		return
	}
	var body struct {
		StepID         string `json:"step_id"`
		ConfirmationID string `json:"confirmation_id"`
		Token          string `json:"token"`
		LeaseID        string `json:"lease_id"`
		FencingToken   int64  `json:"fencing_token"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if !decode(w, r, &body) {
		return
	}
	value, err := s.worker.Submit(context.WithoutCancel(r.Context()), browserautomation.WorkerSubmitRequest{Authorize: browserautomation.AuthorizeActionRequest{OrganizationID: actor.OrganizationID, ProjectID: project, RunID: r.PathValue("run_id"), StepID: body.StepID, ConfirmationID: body.ConfirmationID, Token: body.Token, LeaseID: body.LeaseID, FencingToken: body.FencingToken, IdempotencyKey: body.IdempotencyKey}})
	writeResult(w, value, err)
}

func (s *Server) reconcileResult(w http.ResponseWriter, r *http.Request) {
	actor, project, ok := s.authorize(w, r, "delivery.execute")
	if !ok {
		return
	}
	value, err := s.worker.ReconcileUnknownFromPlatform(context.WithoutCancel(r.Context()), actor.OrganizationID, project, r.PathValue("run_id"))
	writeResult(w, value, err)
}

func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return false
	}
	return true
}
func writeResult(w http.ResponseWriter, value any, err error) {
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(value)
		return
	}
	status := http.StatusConflict
	if errors.Is(err, browserautomation.ErrNotFound) {
		status = http.StatusNotFound
	} else if errors.Is(err, browserautomation.ErrInvalidContract) {
		status = http.StatusBadRequest
	} else if errors.Is(err, browserautomation.ErrKillSwitchActive) {
		status = http.StatusLocked
	}
	writeError(w, status, err.Error())
}

func writeCreated(w http.ResponseWriter, value any, err error) {
	if err != nil {
		writeResult(w, value, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
