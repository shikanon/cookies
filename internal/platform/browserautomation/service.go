package browserautomation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

const FinalConfirmationTTL = 5 * time.Minute

const (
	SessionLeaseTTL     = time.Hour
	SessionHeartbeatTTL = time.Minute
)

type IDGenerator func(string) (string, error)

type Service struct {
	Repository        Repository
	AuthorityProvider AuthorityProvider
	NewID             IDGenerator
	Now               func() time.Time
}

type runListRepository interface {
	ListRuns(context.Context, contract.OrganizationID, contract.ProjectID) ([]BrowserRpaRun, error)
}

func (s Service) ListRuns(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID) ([]BrowserRpaRun, error) {
	if s.Repository == nil || organizationID == "" || projectID == "" {
		return nil, ErrInvalidContract
	}
	repository, ok := s.Repository.(runListRepository)
	if !ok {
		return nil, ErrInvalidContract
	}
	return repository.ListRuns(ctx, organizationID, projectID)
}

type AuthorityProvider interface {
	ResolveAuthority(context.Context, contract.OrganizationID, contract.ProjectID, string, time.Time) (AuthorityResolution, error)
	BindRun(context.Context, AuthorityBinding, string, time.Time) error
	VerifyAuthority(context.Context, AuthorityBinding, string, time.Time) error
}

type AuthorityResolution struct {
	Binding    AuthorityBinding
	BoundRunID string
}

type CreateRunRequest struct {
	Run BrowserRpaRun
}

func (s Service) RegisterEnvironment(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, value ExecutionEnvironment) (ExecutionEnvironment, error) {
	value.OrganizationID = organizationID
	value.ProjectID = projectID
	value.Version = 1
	if s.Repository == nil || value.Validate() != nil {
		return ExecutionEnvironment{}, ErrInvalidContract
	}
	return s.Repository.CreateEnvironment(ctx, value)
}

func (s Service) RegisterBrowserProfile(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, value BrowserProfile) (BrowserProfile, error) {
	value.OrganizationID = organizationID
	value.ProjectID = projectID
	value.Version = 1
	if s.Repository == nil || value.Validate() != nil {
		return BrowserProfile{}, ErrInvalidContract
	}
	environment, err := s.Repository.GetEnvironment(ctx, organizationID, projectID, value.EnvironmentID)
	if err != nil {
		return BrowserProfile{}, err
	}
	if environment.Platform != value.Platform || environment.AccountID != value.AccountID {
		return BrowserProfile{}, ErrInvalidContract
	}
	return s.Repository.CreateBrowserProfile(ctx, value)
}

func (s Service) RegisterSitePolicy(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, value SitePolicy) (SitePolicy, error) {
	value.OrganizationID = organizationID
	value.ProjectID = projectID
	value.Version = 1
	if s.Repository == nil || value.Validate() != nil {
		return SitePolicy{}, ErrInvalidContract
	}
	return s.Repository.CreateSitePolicy(ctx, value)
}

func (s Service) SetKillSwitch(ctx context.Context, actor contract.ActorContext, scope KillSwitchScope, platform Platform, active bool, reason string, expectedVersion int64) (KillSwitch, error) {
	if s.Repository == nil || actor.Principal.Kind != contract.PrincipalService || !actor.HasScope("platform.browser-rpa.admin") || strings.TrimSpace(reason) == "" || expectedVersion < 0 {
		return KillSwitch{}, ErrInvalidContract
	}
	value := KillSwitch{Scope: scope, Active: active, Reason: reason, UpdatedBy: actor.Principal.ID, UpdatedAt: s.now()}
	switch scope {
	case KillSwitchGlobal:
		if platform != "" {
			return KillSwitch{}, ErrInvalidContract
		}
		value.ID = "browser-rpa-kill-global"
	case KillSwitchPlatform:
		if platform != PlatformOceanEngine {
			return KillSwitch{}, ErrInvalidContract
		}
		value.ID = "browser-rpa-kill-platform-" + string(platform)
		value.Platform = platform
	case KillSwitchOrganization:
		if actor.OrganizationID == "" || platform != "" {
			return KillSwitch{}, ErrInvalidContract
		}
		value.ID = "browser-rpa-kill-organization-" + string(actor.OrganizationID)
		value.OrganizationID = actor.OrganizationID
	default:
		return KillSwitch{}, ErrInvalidContract
	}
	value.Version = expectedVersion + 1
	return s.Repository.PutKillSwitch(ctx, value, expectedVersion)
}

type CreateBoundRunRequest struct {
	OrganizationID  contract.OrganizationID
	ProjectID       contract.ProjectID
	Platform        Platform
	AccountID       string
	ExecutionDriver ExecutionDriver
	ExecutionID     string
	EnvironmentID   string
	ProfileID       string
	PolicyID        string
	IdempotencyKey  string
	CreatedBy       string
}

const (
	browserRpaLeaseIDPrefix        = "brpalease"
	browserRpaEvidenceIDPrefix     = "brpaevidence"
	browserRpaEventIDPrefix        = "brpaevent"
	browserRpaConfirmationIDPrefix = "brpaconfirmation"
	browserRpaAttemptIDPrefix      = "brpaattempt"
)

func (s Service) CreateBoundRun(ctx context.Context, request CreateBoundRunRequest) (BrowserRpaRun, bool, error) {
	if request.ExecutionDriver == "" {
		request.ExecutionDriver = ExecutionDriverPlaywrightEdgeV3
	}
	if s.Repository == nil || s.AuthorityProvider == nil || request.OrganizationID == "" || request.ProjectID == "" || request.Platform != PlatformOceanEngine || strings.TrimSpace(request.AccountID) == "" || strings.TrimSpace(request.ExecutionID) == "" || strings.TrimSpace(request.EnvironmentID) == "" || strings.TrimSpace(request.ProfileID) == "" || strings.TrimSpace(request.PolicyID) == "" || strings.TrimSpace(request.IdempotencyKey) == "" || len(request.IdempotencyKey) > 160 || strings.TrimSpace(request.CreatedBy) == "" || request.ExecutionDriver != ExecutionDriverPlaywrightEdgeV3 && request.ExecutionDriver != ExecutionDriverOceanEngineWebAPI {
		return BrowserRpaRun{}, false, ErrInvalidContract
	}
	now := s.now()
	resolution, err := s.AuthorityProvider.ResolveAuthority(ctx, request.OrganizationID, request.ProjectID, request.ExecutionID, now)
	if err != nil {
		return BrowserRpaRun{}, false, err
	}
	authority := resolution.Binding
	if err := authority.Validate(); err != nil || authority.OrganizationID != request.OrganizationID || authority.ProjectID != request.ProjectID || authority.BusinessExecutionID != request.ExecutionID || authority.AccountReferenceID != request.AccountID {
		return BrowserRpaRun{}, false, ErrInvalidContract
	}
	if authority.ExecutionDriver != "" && authority.ExecutionDriver != request.ExecutionDriver {
		return BrowserRpaRun{}, false, ErrInvalidContract
	}
	if authority.OperatorPrincipalID != "" && request.CreatedBy != authority.OperatorPrincipalID {
		return BrowserRpaRun{}, false, ErrInvalidContract
	}
	environment, err := s.Repository.GetEnvironment(ctx, request.OrganizationID, request.ProjectID, request.EnvironmentID)
	if err != nil {
		return BrowserRpaRun{}, false, err
	}
	profile, err := s.Repository.GetBrowserProfile(ctx, request.OrganizationID, request.ProjectID, request.ProfileID)
	if err != nil {
		return BrowserRpaRun{}, false, err
	}
	policy, err := s.Repository.GetSitePolicy(ctx, request.OrganizationID, request.ProjectID, request.PolicyID)
	if err != nil {
		return BrowserRpaRun{}, false, err
	}
	expectedMode := "local_visible"
	if request.ExecutionDriver == ExecutionDriverOceanEngineWebAPI {
		expectedMode = "remote_api"
	}
	if environment.Platform != request.Platform || environment.AccountID != request.AccountID || environment.Mode != expectedMode || !environment.Healthy || environment.Version < 1 || profile.EnvironmentID != environment.ID || profile.Platform != request.Platform || profile.AccountID != request.AccountID || profile.State != "ready" || profile.Version < 1 || policy.Platform != request.Platform || policy.AccountID != request.AccountID || policy.Version < 1 {
		return BrowserRpaRun{}, false, ErrInvalidContract
	}
	if actionRequiresBoundPlatformProject(authority.Action) && !slices.Contains(policy.AllowedPlatformProjects, authority.ParentPlatformProjectID) {
		return BrowserRpaRun{}, false, ErrInvalidContract
	}
	hashInput, err := json.Marshal(struct {
		OrganizationID  contract.OrganizationID `json:"organization_id"`
		ProjectID       contract.ProjectID      `json:"project_id"`
		Platform        Platform                `json:"platform"`
		AccountID       string                  `json:"account_id"`
		ExecutionDriver ExecutionDriver         `json:"execution_driver"`
		ExecutionID     string                  `json:"business_execution_id"`
		EnvironmentID   string                  `json:"environment_id"`
		ProfileID       string                  `json:"profile_id"`
		PolicyID        string                  `json:"policy_id"`
	}{OrganizationID: request.OrganizationID, ProjectID: request.ProjectID, Platform: request.Platform, AccountID: request.AccountID, ExecutionDriver: request.ExecutionDriver, ExecutionID: request.ExecutionID, EnvironmentID: request.EnvironmentID, ProfileID: request.ProfileID, PolicyID: request.PolicyID})
	if err != nil {
		return BrowserRpaRun{}, false, err
	}
	digest := sha256.Sum256(hashInput)
	requestHash := hex.EncodeToString(digest[:])
	if resolution.BoundRunID != "" {
		existing, err := s.Repository.GetRun(ctx, request.OrganizationID, request.ProjectID, resolution.BoundRunID)
		if err != nil {
			return BrowserRpaRun{}, false, err
		}
		allowReloadRecovery := authority.AuthorityOrigin == "plan_execution"
		if (!allowReloadRecovery && existing.IdempotencyKey != request.IdempotencyKey) || existing.RequestHash != requestHash || !reflect.DeepEqual(existing.Authority, authority) {
			return BrowserRpaRun{}, false, ErrIdempotencyConflict
		}
		// A prior request can persist and attach the run before staged object
		// mappings finish. Retry the server-owned binding work on replay.
		if err := s.AuthorityProvider.BindRun(ctx, existing.Authority, existing.ID, now); err != nil {
			return BrowserRpaRun{}, false, err
		}
		return existing, true, nil
	}
	id := boundRunID(request.OrganizationID, request.ProjectID, request.ExecutionID)
	if prior, priorErr := s.Repository.GetRun(ctx, request.OrganizationID, request.ProjectID, id); priorErr == nil && prior.State == RunFailed {
		// Delivery releases the execution binding only after it proves that the
		// failed run had no controlled action. Keep that run immutable and give
		// the retry a distinct audit identity.
		id = retryBoundRunID(request.OrganizationID, request.ProjectID, request.ExecutionID, request.IdempotencyKey)
	}
	run := BrowserRpaRun{SchemaVersion: RunSchemaV1, ID: id, OrganizationID: request.OrganizationID, ProjectID: request.ProjectID, Platform: request.Platform, AccountID: request.AccountID, ExecutionDriver: request.ExecutionDriver, Authority: authority, EnvironmentID: request.EnvironmentID, ProfileID: request.ProfileID, PolicyID: request.PolicyID, State: RunQueued, Version: 1, IdempotencyKey: request.IdempotencyKey, RequestHash: requestHash, CreatedBy: request.CreatedBy, CreatedAt: now, UpdatedAt: now}
	created, existingErr := s.Repository.GetRun(ctx, request.OrganizationID, request.ProjectID, id)
	replayed := existingErr == nil
	if replayed {
		if authority.AuthorityOrigin != "plan_execution" || created.RequestHash != requestHash || !reflect.DeepEqual(created.Authority, authority) {
			return BrowserRpaRun{}, false, ErrIdempotencyConflict
		}
	} else {
		if !errors.Is(existingErr, ErrNotFound) {
			return BrowserRpaRun{}, false, existingErr
		}
		created, replayed, err = s.CreateRun(ctx, CreateRunRequest{Run: run})
		if err != nil {
			return BrowserRpaRun{}, false, err
		}
	}
	if !replayed {
		if err := s.recordEvent(ctx, created, "run_created", "controlled visible-browser run created", request.CreatedBy); err != nil {
			return BrowserRpaRun{}, false, err
		}
	}
	if err := s.AuthorityProvider.BindRun(ctx, created.Authority, created.ID, now); err != nil {
		return BrowserRpaRun{}, false, err
	}
	return created, replayed, nil
}

func boundRunID(organizationID contract.OrganizationID, projectID contract.ProjectID, executionID string) string {
	digest := sha256.Sum256([]byte(string(organizationID) + "\x00" + string(projectID) + "\x00" + executionID))
	return "curun_" + hex.EncodeToString(digest[:])
}

func retryBoundRunID(organizationID contract.OrganizationID, projectID contract.ProjectID, executionID, idempotencyKey string) string {
	digest := sha256.Sum256([]byte(string(organizationID) + "\x00" + string(projectID) + "\x00" + executionID + "\x00retry\x00" + idempotencyKey))
	return "curun_" + hex.EncodeToString(digest[:])
}

type RecordTakeoverEvidenceRequest struct {
	OrganizationID    contract.OrganizationID
	ProjectID         contract.ProjectID
	RunID             string
	ExpectedVersion   int64
	LeaseID           string
	FencingToken      int64
	StepID            string
	Sequence          int
	Action            TakeoverEvidenceAction
	Status            StepStatus
	PageKind          string
	PlatformProjectID string
	BeforePageFacts   map[string]string
	AfterPageFacts    map[string]string
	FieldReadback     map[string]string
	DiffKeys          []string
	PageReference     string
	SelectorVersion   string
	ActionVersion     string
	Actor             string
}

type TakeoverEvidenceResult struct {
	Run      BrowserRpaRun `json:"run"`
	Step     RunStep       `json:"step"`
	Evidence Evidence      `json:"evidence"`
}

type AcquireRunLeaseResult struct {
	Run   BrowserRpaRun `json:"run"`
	Lease SessionLease  `json:"lease"`
}

func (s Service) AcquireRunLease(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, runID string, expectedVersion int64, holder string) (AcquireRunLeaseResult, error) {
	if s.Repository == nil || expectedVersion < 1 || strings.TrimSpace(holder) == "" {
		return AcquireRunLeaseResult{}, ErrInvalidContract
	}
	run, err := s.Repository.GetRun(ctx, organizationID, projectID, runID)
	if err != nil {
		return AcquireRunLeaseResult{}, err
	}
	if run.Version != expectedVersion {
		return AcquireRunLeaseResult{}, ErrVersionConflict
	}
	if terminalState(run.State) {
		return AcquireRunLeaseResult{}, ErrInvalidTransition
	}
	now := s.now()
	if run.LeaseID != "" {
		currentLease, leaseErr := s.Repository.GetLease(ctx, organizationID, projectID, run.LeaseID)
		if leaseErr != nil {
			return AcquireRunLeaseResult{}, leaseErr
		}
		if currentLease.ValidAt(now) {
			return AcquireRunLeaseResult{}, ErrInvalidTransition
		}
		released, _, releaseErr := s.Repository.ReleaseRunLease(ctx, run, expectedVersion, currentLease, currentLease.Version, currentLease.FencingToken, now)
		if releaseErr != nil {
			return AcquireRunLeaseResult{}, releaseErr
		}
		run = released
		expectedVersion = run.Version
	}
	id, err := s.newID(browserRpaLeaseIDPrefix)
	if err != nil {
		return AcquireRunLeaseResult{}, err
	}
	lease := SessionLease{ID: id, OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, RunID: run.ID, EnvironmentID: run.EnvironmentID, ProfileID: run.ProfileID, Platform: run.Platform, AccountID: run.AccountID, Holder: holder, FencingToken: 1, Version: 1, ExpiresAt: now.Add(SessionLeaseTTL), HeartbeatDeadline: now.Add(SessionHeartbeatTTL)}
	updated, lease, err := s.Repository.AcquireRunLease(ctx, run, expectedVersion, lease, now)
	if err != nil {
		return AcquireRunLeaseResult{}, err
	}
	if err := s.recordEvent(ctx, updated, "lease_acquired", "exclusive visible-browser lease acquired", holder); err != nil {
		return AcquireRunLeaseResult{}, err
	}
	return AcquireRunLeaseResult{Run: updated, Lease: lease}, nil
}

func (s Service) RecordTakeoverEvidence(ctx context.Context, request RecordTakeoverEvidenceRequest) (TakeoverEvidenceResult, error) {
	if s.Repository == nil || request.ExpectedVersion < 1 || request.LeaseID == "" || request.FencingToken < 1 || request.StepID == "" || request.Sequence < 1 || !request.Action.Valid() || strings.TrimSpace(request.Actor) == "" || strings.TrimSpace(request.PageKind) == "" || strings.TrimSpace(request.PlatformProjectID) == "" || strings.TrimSpace(request.PageReference) == "" || strings.TrimSpace(request.SelectorVersion) == "" || strings.TrimSpace(request.ActionVersion) == "" {
		return TakeoverEvidenceResult{}, ErrInvalidContract
	}
	if request.Status != StepRunning && request.Status != StepSucceeded && request.Status != StepFailed {
		return TakeoverEvidenceResult{}, ErrInvalidContract
	}
	run, err := s.Repository.GetRun(ctx, request.OrganizationID, request.ProjectID, request.RunID)
	if err != nil {
		return TakeoverEvidenceResult{}, err
	}
	if run.Version != request.ExpectedVersion {
		return TakeoverEvidenceResult{}, ErrVersionConflict
	}
	if run.State != RunAwaitingTakeover || !run.Paused || !run.TakeoverActive || run.LeaseID != request.LeaseID {
		return TakeoverEvidenceResult{}, ErrInvalidTransition
	}
	now := s.now()
	lease, err := s.Repository.GetLease(ctx, request.OrganizationID, request.ProjectID, request.LeaseID)
	if err != nil || lease.RunID != run.ID || lease.EnvironmentID != run.EnvironmentID || lease.ProfileID != run.ProfileID || lease.Platform != run.Platform || lease.AccountID != run.AccountID || lease.FencingToken != request.FencingToken || !lease.ValidAt(now) {
		return TakeoverEvidenceResult{}, ErrLeaseUnavailable
	}
	policy, err := s.Repository.GetSitePolicy(ctx, request.OrganizationID, request.ProjectID, run.PolicyID)
	if err != nil {
		return TakeoverEvidenceResult{}, err
	}
	if policy.Platform != run.Platform || policy.AccountID != run.AccountID || !run.authorizesPlatformProject(request.PlatformProjectID) || !policy.Allows(request.PageReference, request.PageKind, request.PlatformProjectID) {
		return TakeoverEvidenceResult{}, ErrInvalidContract
	}
	step := RunStep{ID: request.StepID, RunID: run.ID, Sequence: request.Sequence, WorkflowStepID: run.Authority.WorkflowStepID, Action: string(request.Action), Status: request.Status, Attempt: 1, Version: 1}
	evidenceID, err := s.newID(browserRpaEvidenceIDPrefix)
	if err != nil {
		return TakeoverEvidenceResult{}, err
	}
	evidence := RedactEvidence(Evidence{SchemaVersion: EvidenceSchemaV1, ID: evidenceID, OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, RunID: run.ID, StepID: step.ID, BeforePageFacts: request.BeforePageFacts, AfterPageFacts: request.AfterPageFacts, FieldReadback: request.FieldReadback, DiffKeys: request.DiffKeys, PageReference: request.PageReference, ObjectFingerprint: run.Authority.ObjectFingerprint, SkillVersion: run.Authority.SkillVersion, SelectorVersion: request.SelectorVersion, ActionVersion: request.ActionVersion, CreatedAt: now})
	eventID, err := s.newID(browserRpaEventIDPrefix)
	if err != nil {
		return TakeoverEvidenceResult{}, err
	}
	event := RunEvent{ID: eventID, OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, RunID: run.ID, Sequence: run.Version + 1, Kind: "takeover_evidence", Summary: string(request.Action) + ":" + string(request.Status), Actor: request.Actor, CreatedAt: now}
	updated, err := s.Repository.RecordTakeoverEvidence(ctx, run, request.ExpectedVersion, step, evidence, event, now)
	if err != nil {
		return TakeoverEvidenceResult{}, err
	}
	return TakeoverEvidenceResult{Run: updated, Step: step, Evidence: evidence}, nil
}

func (s Service) CreateRun(ctx context.Context, request CreateRunRequest) (BrowserRpaRun, bool, error) {
	if s.Repository == nil || request.Run.State != RunQueued || request.Run.BlockingReason != "" || request.Run.Paused || request.Run.TakeoverActive {
		return BrowserRpaRun{}, false, ErrInvalidContract
	}
	if err := request.Run.Validate(); err != nil {
		return BrowserRpaRun{}, false, err
	}
	if _, active, err := s.Repository.ActiveKillSwitch(ctx, request.Run.OrganizationID, request.Run.Platform); err != nil {
		return BrowserRpaRun{}, false, err
	} else if active {
		return BrowserRpaRun{}, false, ErrKillSwitchActive
	}
	return s.Repository.CreateRun(ctx, request.Run)
}

func (s Service) TransitionRun(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, runID string, expectedVersion int64, next RunState, reason BlockingReason) (BrowserRpaRun, error) {
	current, err := s.Repository.GetRun(ctx, organizationID, projectID, runID)
	if err != nil {
		return BrowserRpaRun{}, err
	}
	if current.Version != expectedVersion {
		return BrowserRpaRun{}, ErrVersionConflict
	}
	if !CanTransition(current.State, next) {
		return BrowserRpaRun{}, ErrInvalidTransition
	}
	if _, active, err := s.Repository.ActiveKillSwitch(ctx, organizationID, current.Platform); err != nil {
		return BrowserRpaRun{}, err
	} else if active && next != RunCancelled && next != RunFailed {
		return BrowserRpaRun{}, ErrKillSwitchActive
	}
	updated, err := s.Repository.TransitionRun(ctx, organizationID, projectID, runID, expectedVersion, next, reason, s.now())
	if err != nil {
		return BrowserRpaRun{}, err
	}
	if err := s.recordEvent(ctx, updated, "state_transition", string(current.State)+" -> "+string(next), updated.CreatedBy); err != nil {
		return BrowserRpaRun{}, err
	}
	return updated, nil
}

type IssuedConfirmation struct {
	Confirmation FinalConfirmation `json:"confirmation"`
	Token        string            `json:"token"`
}

func (s Service) IssueFinalConfirmation(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, runID string, expectedVersion int64, bindingHash, actor string) (IssuedConfirmation, error) {
	run, err := s.Repository.GetRun(ctx, organizationID, projectID, runID)
	if err != nil {
		return IssuedConfirmation{}, err
	}
	if expectedVersion < 1 || run.Version != expectedVersion {
		return IssuedConfirmation{}, ErrVersionConflict
	}
	now := s.now()
	confirmationReady := run.State == RunAwaitingConfirmation && !run.Paused && !run.TakeoverActive
	takeoverReady := run.State == RunAwaitingTakeover && run.Paused && run.TakeoverActive
	if (!confirmationReady && !takeoverReady) || bindingHash != run.Authority.ApprovalActionHash || strings.TrimSpace(actor) == "" || (run.Authority.OperatorPrincipalID != "" && actor != run.Authority.OperatorPrincipalID) {
		return IssuedConfirmation{}, ErrConfirmationInvalid
	}
	if run.Authority.Action == "resume_promotion" && (run.Authority.PromotionRestart == nil || run.Authority.PromotionRestart.ValidateAt(run.Authority.Action, now) != nil) {
		return IssuedConfirmation{}, ErrConfirmationInvalid
	}
	if s.AuthorityProvider != nil {
		if err := s.AuthorityProvider.VerifyAuthority(ctx, run.Authority, run.ID, now); err != nil {
			return IssuedConfirmation{}, ErrConfirmationInvalid
		}
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return IssuedConfirmation{}, err
	}
	token := hex.EncodeToString(tokenBytes)
	digest := sha256.Sum256([]byte(token))
	id, err := s.newID(browserRpaConfirmationIDPrefix)
	if err != nil {
		return IssuedConfirmation{}, err
	}
	confirmation := FinalConfirmation{SchemaVersion: ConfirmationSchemaV1, ID: id, OrganizationID: organizationID, ProjectID: projectID, RunID: runID, BindingHash: bindingHash, TokenDigest: hex.EncodeToString(digest[:]), IssuedBy: actor, IssuedAt: now, ExpiresAt: now.Add(FinalConfirmationTTL), Version: 1}
	confirmation, err = s.Repository.IssueConfirmation(ctx, confirmation)
	if err != nil {
		return IssuedConfirmation{}, err
	}
	return IssuedConfirmation{Confirmation: confirmation, Token: token}, nil
}

type AuthorizeTakeoverActionRequest struct {
	OrganizationID    contract.OrganizationID
	ProjectID         contract.ProjectID
	RunID             string
	ExpectedVersion   int64
	StepID            string
	Sequence          int
	ConfirmationID    string
	Token             string
	LeaseID           string
	FencingToken      int64
	IdempotencyKey    string
	PageKind          string
	PlatformProjectID string
	BeforePageFacts   map[string]string
	FieldReadback     map[string]string
	DiffKeys          []string
	PageReference     string
	SelectorVersion   string
	ActionVersion     string
	Actor             string
}

type TakeoverActionAuthorization struct {
	Run      BrowserRpaRun           `json:"run"`
	Attempt  ControlledActionAttempt `json:"attempt"`
	Step     RunStep                 `json:"step"`
	Evidence Evidence                `json:"evidence"`
}

// AuthorizeTakeoverAction is the production manual-click port. It consumes a
// one-time confirmation, fences the browser lease, persists the pre-click
// readback and advances to submitting in one repository transaction. It never
// performs the browser action itself.
func (s Service) AuthorizeTakeoverAction(ctx context.Context, request AuthorizeTakeoverActionRequest) (TakeoverActionAuthorization, error) {
	if s.Repository == nil || request.ExpectedVersion < 1 || request.StepID == "" || request.Sequence < 1 || request.ConfirmationID == "" || request.Token == "" || request.LeaseID == "" || request.FencingToken < 1 || strings.TrimSpace(request.IdempotencyKey) == "" || strings.TrimSpace(request.Actor) == "" || strings.TrimSpace(request.PageKind) == "" || strings.TrimSpace(request.PlatformProjectID) == "" || strings.TrimSpace(request.PageReference) == "" || strings.TrimSpace(request.SelectorVersion) == "" || strings.TrimSpace(request.ActionVersion) == "" || len(request.FieldReadback) == 0 || len(request.DiffKeys) != 0 {
		return TakeoverActionAuthorization{}, ErrInvalidContract
	}
	run, err := s.Repository.GetRun(ctx, request.OrganizationID, request.ProjectID, request.RunID)
	if err != nil {
		return TakeoverActionAuthorization{}, err
	}
	now := s.now()
	if run.Version != request.ExpectedVersion {
		return TakeoverActionAuthorization{}, ErrVersionConflict
	}
	if changesExistingPromotionAction(run.Authority.Action) {
		if request.Actor != run.Authority.OperatorPrincipalID || run.Authority.validatePreSubmitReadback(request.FieldReadback, now) != nil {
			return TakeoverActionAuthorization{}, ErrInvalidContract
		}
	}
	if run.State != RunAwaitingTakeover || !run.Paused || !run.TakeoverActive || run.LeaseID != request.LeaseID {
		return TakeoverActionAuthorization{}, ErrConfirmationInvalid
	}
	if s.AuthorityProvider != nil {
		if err := s.AuthorityProvider.VerifyAuthority(ctx, run.Authority, run.ID, now); err != nil {
			return TakeoverActionAuthorization{}, ErrConfirmationInvalid
		}
	}
	lease, err := s.Repository.GetLease(ctx, request.OrganizationID, request.ProjectID, request.LeaseID)
	if err != nil || lease.RunID != run.ID || lease.EnvironmentID != run.EnvironmentID || lease.ProfileID != run.ProfileID || lease.Platform != run.Platform || lease.AccountID != run.AccountID || lease.FencingToken != request.FencingToken || !lease.ValidAt(now) {
		return TakeoverActionAuthorization{}, ErrLeaseUnavailable
	}
	policy, err := s.Repository.GetSitePolicy(ctx, request.OrganizationID, request.ProjectID, run.PolicyID)
	if err != nil {
		return TakeoverActionAuthorization{}, err
	}
	if policy.Platform != run.Platform || policy.AccountID != run.AccountID || !run.authorizesPlatformProject(request.PlatformProjectID) || !policy.Allows(request.PageReference, request.PageKind, request.PlatformProjectID) {
		return TakeoverActionAuthorization{}, ErrInvalidContract
	}
	digest := sha256.Sum256([]byte(request.Token))
	attemptID, err := s.newID(browserRpaAttemptIDPrefix)
	if err != nil {
		return TakeoverActionAuthorization{}, err
	}
	evidenceID, err := s.newID(browserRpaEvidenceIDPrefix)
	if err != nil {
		return TakeoverActionAuthorization{}, err
	}
	eventID, err := s.newID(browserRpaEventIDPrefix)
	if err != nil {
		return TakeoverActionAuthorization{}, err
	}
	step := RunStep{ID: request.StepID, RunID: run.ID, Sequence: request.Sequence, WorkflowStepID: run.Authority.WorkflowStepID, Action: "submit_platform_configuration", Status: StepRunning, Attempt: 1, Version: 1}
	evidence := RedactEvidence(Evidence{SchemaVersion: EvidenceSchemaV1, ID: evidenceID, OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, RunID: run.ID, StepID: step.ID, BeforePageFacts: request.BeforePageFacts, FieldReadback: request.FieldReadback, DiffKeys: []string{}, PageReference: request.PageReference, ObjectFingerprint: run.Authority.ObjectFingerprint, SkillVersion: run.Authority.SkillVersion, SelectorVersion: request.SelectorVersion, ActionVersion: request.ActionVersion, CreatedAt: now})
	attempt := ControlledActionAttempt{ID: attemptID, OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, RunID: run.ID, StepID: step.ID, ConfirmationID: request.ConfirmationID, ApprovalID: run.Authority.ApprovalID, LeaseID: lease.ID, FencingToken: lease.FencingToken, ActionHash: run.Authority.ApprovalActionHash, IdempotencyKey: request.IdempotencyKey, Status: ControlledActionAuthorized, CreatedAt: now}
	event := RunEvent{ID: eventID, OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, RunID: run.ID, Sequence: run.Version + 1, Kind: "takeover_write_authorized", Summary: "single final click authorized; browser action not executed by control plane", Actor: request.Actor, CreatedAt: now}
	updated, attempt, err := s.Repository.AuthorizeTakeoverAction(ctx, run, request.ExpectedVersion, FinalConfirmation{ID: request.ConfirmationID, OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, RunID: run.ID, BindingHash: run.Authority.ApprovalActionHash}, hex.EncodeToString(digest[:]), lease, attempt, step, evidence, event, now)
	if err != nil {
		return TakeoverActionAuthorization{}, err
	}
	return TakeoverActionAuthorization{Run: updated, Attempt: attempt, Step: step, Evidence: evidence}, nil
}

type RecordTakeoverOutcomeRequest struct {
	OrganizationID    contract.OrganizationID
	ProjectID         contract.ProjectID
	RunID             string
	AttemptID         string
	ExpectedVersion   int64
	LeaseID           string
	FencingToken      int64
	StepID            string
	Sequence          int
	Outcome           TakeoverWriteOutcome
	PageKind          string
	PlatformProjectID string
	BeforePageFacts   map[string]string
	AfterPageFacts    map[string]string
	FieldReadback     map[string]string
	PageReference     string
	SelectorVersion   string
	ActionVersion     string
	Actor             string
}

func (s Service) RecordTakeoverOutcome(ctx context.Context, request RecordTakeoverOutcomeRequest) (TakeoverEvidenceResult, error) {
	if s.Repository == nil || request.AttemptID == "" || request.ExpectedVersion < 1 || request.LeaseID == "" || request.FencingToken < 1 || request.StepID == "" || request.Sequence < 1 || !request.Outcome.Valid() || strings.TrimSpace(request.Actor) == "" || strings.TrimSpace(request.PageKind) == "" || strings.TrimSpace(request.PlatformProjectID) == "" || strings.TrimSpace(request.PageReference) == "" || strings.TrimSpace(request.SelectorVersion) == "" || strings.TrimSpace(request.ActionVersion) == "" {
		return TakeoverEvidenceResult{}, ErrInvalidContract
	}
	run, err := s.Repository.GetRun(ctx, request.OrganizationID, request.ProjectID, request.RunID)
	if err != nil {
		return TakeoverEvidenceResult{}, err
	}
	if run.Version != request.ExpectedVersion {
		return TakeoverEvidenceResult{}, ErrVersionConflict
	}
	next, reason, status, attemptStatus := RunFailed, BlockingReason(""), StepFailed, ControlledActionFailed
	switch request.Outcome {
	case TakeoverResultObserved:
		if run.State != RunSubmitting || request.FieldReadback["platform_object_id"] == "" || request.FieldReadback["platform_status"] == "" {
			return TakeoverEvidenceResult{}, ErrInvalidTransition
		}
		if changesExistingPromotionAction(run.Authority.Action) {
			_, targetStateHash, stateErr := run.Authority.existingPromotionStateHashes()
			targetStatus := run.Authority.existingPromotionTargetStatus()
			if stateErr != nil || request.FieldReadback["platform_object_id"] != run.Authority.TargetPlatformObjectID || request.FieldReadback["target_state_hash"] != targetStateHash || (targetStatus != "" && request.FieldReadback["platform_status"] != targetStatus) {
				return TakeoverEvidenceResult{}, ErrInvalidContract
			}
		}
		next, status, attemptStatus = RunVerifying, StepSucceeded, ControlledActionVerified
	case TakeoverListConfirmed:
		if run.State != RunVerifying || request.FieldReadback["platform_object_id"] == "" || request.FieldReadback["platform_status"] == "" {
			return TakeoverEvidenceResult{}, ErrInvalidTransition
		}
		steps, err := s.Repository.ListSteps(ctx, request.OrganizationID, request.ProjectID, run.ID)
		if err != nil {
			return TakeoverEvidenceResult{}, err
		}
		resultStepID := ""
		for _, candidate := range steps {
			if candidate.Action == string(TakeoverResultObserved) && candidate.Status == StepSucceeded {
				if resultStepID != "" {
					return TakeoverEvidenceResult{}, ErrInvalidContract
				}
				resultStepID = candidate.ID
			}
		}
		evidence, err := s.Repository.ListEvidence(ctx, request.OrganizationID, request.ProjectID, run.ID)
		if err != nil {
			return TakeoverEvidenceResult{}, err
		}
		resultObjectID, resultStatus := "", ""
		for _, candidate := range evidence {
			if candidate.StepID == resultStepID {
				resultObjectID = candidate.FieldReadback["platform_object_id"]
				resultStatus = candidate.FieldReadback["platform_status"]
				break
			}
		}
		if resultStepID == "" || resultObjectID != request.FieldReadback["platform_object_id"] || resultStatus != request.FieldReadback["platform_status"] {
			return TakeoverEvidenceResult{}, ErrInvalidContract
		}
		if changesExistingPromotionAction(run.Authority.Action) {
			resultTargetHash := ""
			for _, candidate := range evidence {
				if candidate.StepID == resultStepID {
					resultTargetHash = candidate.FieldReadback["target_state_hash"]
					break
				}
			}
			_, targetStateHash, stateErr := run.Authority.existingPromotionStateHashes()
			targetStatus := run.Authority.existingPromotionTargetStatus()
			if stateErr != nil || resultObjectID != run.Authority.TargetPlatformObjectID || resultTargetHash != targetStateHash || request.FieldReadback["target_state_hash"] != targetStateHash || (targetStatus != "" && request.FieldReadback["platform_status"] != targetStatus) {
				return TakeoverEvidenceResult{}, ErrInvalidContract
			}
		}
		next, status, attemptStatus = RunSucceeded, StepSucceeded, ControlledActionVerified
	case TakeoverWriteRejected:
		if run.State != RunSubmitting && run.State != RunVerifying {
			return TakeoverEvidenceResult{}, ErrInvalidTransition
		}
		next, reason = RunFailed, BlockResultReconciliation
	case TakeoverResultUnknown:
		if run.State != RunSubmitting && run.State != RunVerifying {
			return TakeoverEvidenceResult{}, ErrInvalidTransition
		}
		next, reason, status, attemptStatus = RunResultUnknown, BlockResultReconciliation, StepResultUnknown, ControlledActionResultUnknown
	}
	now := s.now()
	lease, err := s.Repository.GetLease(ctx, request.OrganizationID, request.ProjectID, request.LeaseID)
	if err != nil || run.LeaseID != lease.ID || lease.RunID != run.ID || lease.FencingToken != request.FencingToken || !lease.ValidAt(now) {
		return TakeoverEvidenceResult{}, ErrLeaseUnavailable
	}
	policy, err := s.Repository.GetSitePolicy(ctx, request.OrganizationID, request.ProjectID, run.PolicyID)
	if err != nil || !run.authorizesPlatformProject(request.PlatformProjectID) || !policy.Allows(request.PageReference, request.PageKind, request.PlatformProjectID) {
		return TakeoverEvidenceResult{}, ErrInvalidContract
	}
	step := RunStep{ID: request.StepID, RunID: run.ID, Sequence: request.Sequence, WorkflowStepID: run.Authority.WorkflowStepID, Action: string(request.Outcome), Status: status, BlockingReason: reason, Attempt: 1, Version: 1}
	evidenceID, err := s.newID(browserRpaEvidenceIDPrefix)
	if err != nil {
		return TakeoverEvidenceResult{}, err
	}
	evidence := RedactEvidence(Evidence{SchemaVersion: EvidenceSchemaV1, ID: evidenceID, OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, RunID: run.ID, StepID: step.ID, BeforePageFacts: request.BeforePageFacts, AfterPageFacts: request.AfterPageFacts, FieldReadback: request.FieldReadback, PageReference: request.PageReference, ObjectFingerprint: run.Authority.ObjectFingerprint, SkillVersion: run.Authority.SkillVersion, SelectorVersion: request.SelectorVersion, ActionVersion: request.ActionVersion, CreatedAt: now})
	eventID, err := s.newID(browserRpaEventIDPrefix)
	if err != nil {
		return TakeoverEvidenceResult{}, err
	}
	event := RunEvent{ID: eventID, OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, RunID: run.ID, Sequence: run.Version + 1, Kind: "takeover_write_outcome", Summary: string(request.Outcome), Actor: request.Actor, CreatedAt: now}
	updated, err := s.Repository.RecordTakeoverOutcome(ctx, run, request.ExpectedVersion, request.AttemptID, attemptStatus, next, reason, step, evidence, event, now)
	if err != nil {
		return TakeoverEvidenceResult{}, err
	}
	return TakeoverEvidenceResult{Run: updated, Step: step, Evidence: evidence}, nil
}

type AuthorizeActionRequest struct {
	OrganizationID contract.OrganizationID
	ProjectID      contract.ProjectID
	RunID          string
	StepID         string
	ConfirmationID string
	Token          string
	LeaseID        string
	FencingToken   int64
	IdempotencyKey string
}

// AuthorizeAction performs the final fail-closed check. The repository must
// consume the one-time token and persist the attempt in the same transaction.
func (s Service) AuthorizeAction(ctx context.Context, request AuthorizeActionRequest) (ControlledActionAttempt, error) {
	run, err := s.Repository.GetRun(ctx, request.OrganizationID, request.ProjectID, request.RunID)
	if err != nil {
		return ControlledActionAttempt{}, err
	}
	now := s.now()
	if run.State != RunAwaitingConfirmation || run.Paused || run.TakeoverActive {
		return ControlledActionAttempt{}, ErrConfirmationInvalid
	}
	if run.Authority.Action == "resume_promotion" && (run.Authority.PromotionRestart == nil || run.Authority.PromotionRestart.ValidateAt(run.Authority.Action, now) != nil) {
		return ControlledActionAttempt{}, ErrConfirmationInvalid
	}
	if s.AuthorityProvider != nil {
		if err := s.AuthorityProvider.VerifyAuthority(ctx, run.Authority, run.ID, now); err != nil {
			return ControlledActionAttempt{}, ErrConfirmationInvalid
		}
	}
	if _, active, err := s.Repository.ActiveKillSwitch(ctx, request.OrganizationID, run.Platform); err != nil {
		return ControlledActionAttempt{}, err
	} else if active {
		return ControlledActionAttempt{}, ErrKillSwitchActive
	}
	lease, err := s.Repository.GetLease(ctx, request.OrganizationID, request.ProjectID, request.LeaseID)
	if err != nil || lease.RunID != run.ID || lease.FencingToken != request.FencingToken || !lease.ValidAt(now) {
		return ControlledActionAttempt{}, ErrLeaseUnavailable
	}
	digest := sha256.Sum256([]byte(request.Token))
	attemptID, err := s.newID(browserRpaAttemptIDPrefix)
	if err != nil {
		return ControlledActionAttempt{}, err
	}
	attempt := ControlledActionAttempt{ID: attemptID, OrganizationID: request.OrganizationID, ProjectID: request.ProjectID, RunID: run.ID, StepID: request.StepID, ConfirmationID: request.ConfirmationID, ApprovalID: run.Authority.ApprovalID, LeaseID: lease.ID, FencingToken: lease.FencingToken, ActionHash: run.Authority.ApprovalActionHash, IdempotencyKey: request.IdempotencyKey, Status: ControlledActionAuthorized, CreatedAt: now}
	return s.Repository.AuthorizeControlledAction(ctx, FinalConfirmation{ID: request.ConfirmationID, OrganizationID: request.OrganizationID, ProjectID: request.ProjectID, RunID: run.ID, BindingHash: run.Authority.ApprovalActionHash}, hex.EncodeToString(digest[:]), lease, attempt, now)
}

func (s Service) HeartbeatLease(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, leaseID string, expectedVersion, fencingToken int64) (SessionLease, error) {
	now := s.now()
	lease, err := s.Repository.GetLease(ctx, organizationID, projectID, leaseID)
	if err != nil {
		return SessionLease{}, err
	}
	if lease.Version != expectedVersion || lease.FencingToken != fencingToken || !lease.ValidAt(now) {
		return SessionLease{}, ErrLeaseUnavailable
	}
	return s.Repository.HeartbeatLease(ctx, organizationID, projectID, leaseID, expectedVersion, fencingToken, now, now.Add(SessionLeaseTTL), now.Add(SessionHeartbeatTTL))
}

func (s Service) HeartbeatRunLease(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, runID, leaseID string, expectedVersion, fencingToken int64) (SessionLease, error) {
	run, err := s.Repository.GetRun(ctx, organizationID, projectID, runID)
	if err != nil {
		return SessionLease{}, err
	}
	lease, err := s.Repository.GetLease(ctx, organizationID, projectID, leaseID)
	if err != nil {
		return SessionLease{}, err
	}
	if run.LeaseID != lease.ID || lease.RunID != run.ID {
		return SessionLease{}, ErrLeaseUnavailable
	}
	return s.HeartbeatLease(ctx, organizationID, projectID, leaseID, expectedVersion, fencingToken)
}

func (s Service) ReleaseLease(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, leaseID string, expectedVersion, fencingToken int64) (SessionLease, error) {
	lease, err := s.Repository.GetLease(ctx, organizationID, projectID, leaseID)
	if err != nil {
		return SessionLease{}, err
	}
	if lease.Version != expectedVersion || lease.FencingToken != fencingToken || lease.ReleasedAt != nil {
		return SessionLease{}, ErrLeaseUnavailable
	}
	return s.Repository.ReleaseLease(ctx, organizationID, projectID, leaseID, expectedVersion, fencingToken, s.now())
}

func (s Service) ReleaseRunLease(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, runID, leaseID string, expectedRunVersion, expectedLeaseVersion, fencingToken int64) (AcquireRunLeaseResult, error) {
	if expectedRunVersion < 1 || expectedLeaseVersion < 1 || fencingToken < 1 {
		return AcquireRunLeaseResult{}, ErrInvalidContract
	}
	run, err := s.Repository.GetRun(ctx, organizationID, projectID, runID)
	if err != nil {
		return AcquireRunLeaseResult{}, err
	}
	lease, err := s.Repository.GetLease(ctx, organizationID, projectID, leaseID)
	if err != nil {
		return AcquireRunLeaseResult{}, err
	}
	if run.Version != expectedRunVersion || run.LeaseID != lease.ID || lease.RunID != run.ID || lease.Version != expectedLeaseVersion || lease.FencingToken != fencingToken || lease.ReleasedAt != nil {
		return AcquireRunLeaseResult{}, ErrLeaseUnavailable
	}
	updatedRun, updatedLease, err := s.Repository.ReleaseRunLease(ctx, run, expectedRunVersion, lease, expectedLeaseVersion, fencingToken, s.now())
	if err != nil {
		return AcquireRunLeaseResult{}, err
	}
	return AcquireRunLeaseResult{Run: updatedRun, Lease: updatedLease}, nil
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s Service) newID(prefix string) (string, error) {
	if s.NewID != nil {
		return s.NewID(prefix)
	}
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(bytes), nil
}

func (s Service) recordEvent(ctx context.Context, run BrowserRpaRun, kind, summary, actor string) error {
	id, err := s.newID(browserRpaEventIDPrefix)
	if err != nil {
		return err
	}
	if actor == "" {
		actor = "browser-rpa-control-plane"
	}
	return s.Repository.AppendEvent(ctx, RunEvent{ID: id, OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, RunID: run.ID, Sequence: run.Version, Kind: kind, Summary: summary, Actor: actor, CreatedAt: s.now()})
}
