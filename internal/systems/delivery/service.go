// Package delivery owns versioned advertising plans and their controlled,
// auditable execution. The MVP executor is deliberately a local simulation.
package delivery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/connector"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/ids"
)

const (
	ScopeRead    contract.Scope = "delivery.read"
	ScopeWrite   contract.Scope = "delivery.write"
	ScopeApprove contract.Scope = "delivery.approve"
	ScopeExecute contract.Scope = "delivery.execute"
)

var (
	ErrNotFound                          = errors.New("delivery resource not found")
	ErrInvalidRequest                    = errors.New("delivery request is invalid")
	ErrInvalidState                      = errors.New("delivery resource is not in a state that allows this action")
	ErrVersionConflict                   = errors.New("delivery resource version conflict")
	ErrPlanVersionConflict               = errors.New("delivery plan version conflict")
	ErrStalePlanVersion                  = errors.New("delivery change set references a stale plan version")
	ErrApprovalRequired                  = errors.New("delivery approval is required")
	ErrApprovalExpired                   = errors.New("delivery approval has expired")
	ErrApprovalContentMismatch           = errors.New("delivery approval content does not match")
	ErrApprovalScopeExceeded             = errors.New("delivery approval scope or budget was exceeded")
	ErrIdempotencyConflict               = errors.New("delivery idempotency key was reused with a different request")
	ErrUnsupportedConfigurationWorkflow  = errors.New("delivery repository does not support the configuration workflow")
	ErrUnsupportedTour                   = errors.New("delivery repository does not support delivery tours")
	ErrTourOwnerMismatch                 = errors.New("delivery tour belongs to another owner")
	ErrLegacyConfigurationUnsupported    = errors.New("legacy delivery configuration is read-only and unsupported by this operation")
	ErrImmutableContractIdentityConflict = errors.New("delivery immutable contract identity conflict")
)

// ValidationFailedError carries structured preflight checks that blocked a
// save. The HTTP layer maps it to a 422 with per-field violations so the
// frontend can show inline errors.
type ValidationFailedError struct {
	Checks []PreflightCheck
}

func (e *ValidationFailedError) Error() string {
	return "投放计划校验未通过，请检查表单字段"
}

type DeliveryPlanStatus string

const DeliveryPlanDraft DeliveryPlanStatus = "draft"

type ChangeSetStatus string

const (
	ChangeSetDraft           ChangeSetStatus = "draft"
	ChangeSetPreflightPassed ChangeSetStatus = "preflight_passed"
	ChangeSetPreflightFailed ChangeSetStatus = "preflight_failed"
	ChangeSetApproved        ChangeSetStatus = "approved"
	ChangeSetRejected        ChangeSetStatus = "rejected"
	ChangeSetExecuted        ChangeSetStatus = "executed"
	ChangeSetRolledBack      ChangeSetStatus = "rolled_back"
)

const ExecutionModeLocalSimulation = "local_simulation"

const (
	DemoMetricDatasetVersion = "post-launch-simulator/v1"
	MetricSourceDemoFixture  = "post_launch_simulator"
)

// CreatePlanRequest only accepts the authoritative immutable intent and
// platform-specific configuration envelopes.
type CreatePlanRequest struct {
	Intent                *DeliveryIntent        `json:"intent,omitempty"`
	PlatformConfiguration *PlatformConfiguration `json:"platform_configuration,omitempty"`
}

func (r CreatePlanRequest) usesPlatformRuntime() bool {
	return r.Intent != nil && r.PlatformConfiguration != nil
}

func (r CreatePlanRequest) UsesPlatformRuntime() bool { return r.usesPlatformRuntime() }

func (r CreatePlanRequest) Validate() error {
	if !r.usesPlatformRuntime() {
		return ErrInvalidRequest
	}
	return nil
}

// DeliveryPlan remains the #21 current projection and also exposes the
// immutable lifecycle snapshots needed by the plan editor.
type DeliveryPlan struct {
	ID                   string                  `json:"id"`
	OrganizationID       contract.OrganizationID `json:"organization_id"`
	ProjectID            contract.ProjectID      `json:"project_id"`
	CreativePackageID    string                  `json:"creative_package_id"`
	CreativePackageHash  string                  `json:"creative_package_hash"`
	CreativeVersionID    string                  `json:"creative_version_id"`
	Name                 string                  `json:"name"`
	Objective            string                  `json:"objective"`
	BudgetCents          int64                   `json:"budget_cents"`
	StartAt              time.Time               `json:"start_at"`
	EndAt                time.Time               `json:"end_at"`
	Status               DeliveryPlanStatus      `json:"status"`
	Version              int64                   `json:"version"`
	Platform             string                  `json:"platform"`
	Source               Source                  `json:"source"`
	Scenario             Scenario                `json:"scenario"`
	TourRunID            string                  `json:"tour_run_id,omitempty"`
	TourOwnerID          string                  `json:"tour_owner_id,omitempty"`
	TourCase             string                  `json:"tour_case,omitempty"`
	CurrentVersionNumber int                     `json:"current_version_number"`
	CurrentVersion       DeliveryPlanVersion     `json:"current_version"`
	Versions             []DeliveryPlanVersion   `json:"versions"`
	CreatedBy            string                  `json:"created_by"`
	CreatedAt            time.Time               `json:"created_at"`
	UpdatedAt            time.Time               `json:"updated_at"`
}

type ChangeSet struct {
	ID                   string                  `json:"id"`
	OrganizationID       contract.OrganizationID `json:"organization_id"`
	ProjectID            contract.ProjectID      `json:"project_id"`
	PlanID               string                  `json:"plan_id"`
	PlanName             string                  `json:"plan_name"`
	PlanVersion          int64                   `json:"plan_version"`
	PlanCanonicalHash    string                  `json:"plan_canonical_hash"`
	TargetSnapshot       *PlatformConfiguration  `json:"target_snapshot,omitempty"`
	LegacyTargetSnapshot *ThreeTierConfiguration `json:"legacy_target_snapshot,omitempty"`
	TargetSnapshotHash   string                  `json:"target_snapshot_hash,omitempty"`
	RuntimeStatus        string                  `json:"runtime_status,omitempty"`
	ReadOnly             bool                    `json:"read_only,omitempty"`
	RecommendationID     string                  `json:"recommendation_id,omitempty"`
	BudgetLimit          Budget                  `json:"budget_limit"`
	Status               ChangeSetStatus         `json:"status"`
	RiskLevel            string                  `json:"risk_level"`
	PreflightNotes       []string                `json:"preflight_notes"`
	ApprovedBy           string                  `json:"approved_by,omitempty"`
	ApprovedAt           *time.Time              `json:"approved_at,omitempty"`
	RejectedBy           string                  `json:"rejected_by,omitempty"`
	RejectedAt           *time.Time              `json:"rejected_at,omitempty"`
	RejectionReason      string                  `json:"rejection_reason,omitempty"`
	Approval             *ApprovalView           `json:"approval,omitempty"`
	Source               Source                  `json:"source"`
	Scenario             Scenario                `json:"scenario"`
	Version              int64                   `json:"version"`
	CreatedBy            string                  `json:"created_by"`
	CreatedAt            time.Time               `json:"created_at"`
	UpdatedAt            time.Time               `json:"updated_at"`
}

type RejectChangeSetRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}

func (r RejectChangeSetRequest) Validate() error {
	if r.ExpectedVersion < 1 || len(strings.TrimSpace(r.Reason)) < 3 || len(strings.TrimSpace(r.Reason)) > 1000 {
		return ErrInvalidRequest
	}
	return nil
}

type changeSetRejectionRepository interface {
	RejectChangeSet(context.Context, contract.OrganizationID, contract.ProjectID, string, int64, string, string, time.Time) (ChangeSet, error)
}

type Execution struct {
	ID                     string                  `json:"id"`
	OrganizationID         contract.OrganizationID `json:"organization_id"`
	ProjectID              contract.ProjectID      `json:"project_id"`
	ChangeSetID            string                  `json:"change_set_id"`
	ApprovalID             string                  `json:"approval_id"`
	Status                 ExecutionStatus         `json:"status"`
	Version                int64                   `json:"version"`
	Mode                   string                  `json:"mode"`
	Adapter                string                  `json:"adapter"`
	Source                 Source                  `json:"source"`
	Scenario               ExecutionScenario       `json:"scenario"`
	IdempotencyKey         string                  `json:"idempotency_key"`
	RequestHash            string                  `json:"request_hash"`
	ExecutedBy             string                  `json:"executed_by"`
	StartedAt              time.Time               `json:"started_at"`
	CompletedAt            *time.Time              `json:"completed_at"`
	RetryAllowed           bool                    `json:"retry_allowed"`
	RecoveryAction         string                  `json:"recovery_action"`
	RecoveryReason         string                  `json:"recovery_reason"`
	CompensationCandidates []string                `json:"compensation_candidates"`
	Steps                  []ExecutionStep         `json:"steps"`
}

type ExecutionStatus string

const (
	ExecutionQueued             ExecutionStatus = "queued"
	ExecutionValidatingApproval ExecutionStatus = "validating_approval"
	ExecutionExecuting          ExecutionStatus = "executing"
	ExecutionVerifying          ExecutionStatus = "verifying"
	ExecutionSucceeded          ExecutionStatus = "succeeded"
	ExecutionFailed             ExecutionStatus = "failed"
	ExecutionPartial            ExecutionStatus = "partial"
	ExecutionResultUnknown      ExecutionStatus = "result_unknown"
	ExecutionCancelled          ExecutionStatus = "cancelled"
)

type StepStatus string

const (
	StepPending       StepStatus = "pending"
	StepRunning       StepStatus = "running"
	StepSucceeded     StepStatus = "succeeded"
	StepFailed        StepStatus = "failed"
	StepResultUnknown StepStatus = "result_unknown"
	StepSkipped       StepStatus = "skipped"
)

type ExecutionScenario string

const (
	ExecutionScenarioSuccess       ExecutionScenario = "success"
	ExecutionScenarioFailed        ExecutionScenario = "failed"
	ExecutionScenarioPartial       ExecutionScenario = "partial"
	ExecutionScenarioResultUnknown ExecutionScenario = "result_unknown"
)

type ExecutionStep struct {
	ID             string     `json:"id"`
	Sequence       int        `json:"sequence"`
	Action         string     `json:"action"`
	Status         StepStatus `json:"status"`
	Attempt        int        `json:"attempt"`
	Effect         string     `json:"effect"`
	OutcomeSummary string     `json:"outcome_summary"`
	EvidenceRef    string     `json:"evidence_ref"`
	StartedAt      *time.Time `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at"`
	Version        int64      `json:"version"`
}

type ExecuteRequest struct {
	ExpectedVersion int64             `json:"expected_version"`
	Scenario        ExecutionScenario `json:"scenario"`
}

func (r ExecuteRequest) Validate() error {
	if r.ExpectedVersion < 1 {
		return ErrInvalidRequest
	}
	switch r.Scenario {
	case ExecutionScenarioSuccess, ExecutionScenarioFailed, ExecutionScenarioPartial, ExecutionScenarioResultUnknown:
		return nil
	default:
		return ErrInvalidRequest
	}
}

type Evidence struct {
	ID             string                  `json:"id"`
	OrganizationID contract.OrganizationID `json:"organization_id"`
	ProjectID      contract.ProjectID      `json:"project_id"`
	ExecutionID    string                  `json:"execution_id"`
	Summary        string                  `json:"summary"`
	Mode           string                  `json:"mode"`
	Reversible     bool                    `json:"reversible"`
	Source         Source                  `json:"source"`
	Scenario       ExecutionScenario       `json:"scenario"`
	References     []string                `json:"references"`
	CreatedAt      time.Time               `json:"created_at"`
}

type ExecutionResult struct {
	ChangeSet ChangeSet `json:"change_set"`
	Execution Execution `json:"execution"`
	Evidence  Evidence  `json:"evidence"`
}

type RawMetrics struct {
	Impressions  int64 `json:"impressions"`
	Clicks       int64 `json:"clicks"`
	Conversions  int64 `json:"conversions"`
	SpendCents   int64 `json:"spend_cents"`
	RevenueCents int64 `json:"revenue_cents,omitempty"`
}

type DeliveryMetricSnapshot struct {
	ID                string                  `json:"id"`
	OrganizationID    contract.OrganizationID `json:"organization_id"`
	ProjectID         contract.ProjectID      `json:"project_id"`
	ExecutionID       string                  `json:"execution_id"`
	SimulationRunID   string                  `json:"simulation_run_id,omitempty"`
	PlanID            string                  `json:"plan_id"`
	CreativePackageID string                  `json:"creative_package_id"`
	Source            string                  `json:"source"`
	IsSimulated       bool                    `json:"is_simulated"`
	DatasetVersion    string                  `json:"dataset_version"`
	FixtureVersion    string                  `json:"fixture_version"`
	WindowSequence    int                     `json:"window_sequence"`
	DataThrough       time.Time               `json:"data_through"`
	Currency          string                  `json:"currency"`
	WindowStart       time.Time               `json:"window_start"`
	WindowEnd         time.Time               `json:"window_end"`
	RawMetrics        RawMetrics              `json:"raw_metrics"`
	CalculationBasis  MetricCalculationBasis  `json:"calculation_basis"`
	CreatedBy         string                  `json:"created_by"`
	CreatedAt         time.Time               `json:"created_at"`
}

type CreateMetricSnapshotRequest struct {
	DatasetVersion string `json:"dataset_version"`
}

func (r CreateMetricSnapshotRequest) Validate() error {
	if strings.TrimSpace(r.DatasetVersion) != DemoMetricDatasetVersion {
		return ErrInvalidRequest
	}
	return nil
}

type PlanDetail struct {
	Plan       DeliveryPlan      `json:"plan"`
	ChangeSets []ChangeSet       `json:"change_sets"`
	Executions []ExecutionResult `json:"executions"`
}

type ActiveProjectResolver interface {
	RequireActiveContext(context.Context, contract.ActorContext, contract.ProjectID) (contract.ProjectContext, error)
}

type ConnectorAccountReader interface {
	ListAccounts(context.Context, string, string) ([]connector.PlatformAccount, error)
}

type ExternalAccountIDResolver interface {
	ResolveExternalAccountID(context.Context, string, string, string) (string, error)
}

type BrowserRpaRunLauncher interface {
	LaunchBrowserRpaRun(context.Context, BrowserRpaLaunchRequest) (BrowserRpaLaunchResult, error)
}

type Repository interface {
	CreatePlan(context.Context, DeliveryPlan, DeliveryPlanVersion) (DeliveryPlan, error)
	UpdatePlan(context.Context, contract.OrganizationID, contract.ProjectID, string, int, DeliveryPlanVersion) (DeliveryPlan, error)
	ListPlans(context.Context, contract.OrganizationID, contract.ProjectID, int) ([]DeliveryPlan, error)
	GetPlan(context.Context, contract.OrganizationID, contract.ProjectID, string) (DeliveryPlan, error)
	ListPlanVersions(context.Context, contract.OrganizationID, contract.ProjectID, string) ([]DeliveryPlanVersion, error)
	GetPlanVersion(context.Context, contract.OrganizationID, contract.ProjectID, string, int) (DeliveryPlanVersion, error)
	CreateChangeSet(context.Context, ChangeSet) (ChangeSet, error)
	ListChangeSets(context.Context, contract.OrganizationID, contract.ProjectID, int) ([]ChangeSet, error)
	GetChangeSet(context.Context, contract.OrganizationID, contract.ProjectID, string) (ChangeSet, error)
	TransitionChangeSet(context.Context, contract.OrganizationID, contract.ProjectID, string, int64, ChangeSetStatus, string, time.Time) (ChangeSet, error)
	ApproveChangeSet(context.Context, ChangeSet, DeliveryApproval) (ChangeSet, error)
	GetApproval(context.Context, contract.OrganizationID, contract.ProjectID, string) (DeliveryApproval, error)
	CreateOrReplayExecution(context.Context, ChangeSet, DeliveryApproval, Execution, Evidence) (ExecutionResult, bool, error)
	FindExecutionByIdempotency(context.Context, contract.OrganizationID, contract.ProjectID, string) (ExecutionResult, bool, error)
	RecordDirectExecution(context.Context, ChangeSet, Execution, Evidence) (ExecutionResult, error)
	AdvanceExecution(context.Context, Execution, ExecutionStatus, *time.Time, string, string, []string) (ExecutionResult, error)
	AdvanceStep(context.Context, Execution, ExecutionStep, ExecutionStep) (ExecutionStep, error)
	ListExecutions(context.Context, contract.OrganizationID, contract.ProjectID, int) ([]ExecutionResult, error)
	GetExecution(context.Context, contract.OrganizationID, contract.ProjectID, string) (ExecutionResult, error)
	GetExecutionByChangeSet(context.Context, contract.OrganizationID, contract.ProjectID, string) (ExecutionResult, error)
	CreateMetricSnapshot(context.Context, DeliveryMetricSnapshot) (DeliveryMetricSnapshot, bool, error)
	ListMetricSnapshots(context.Context, contract.OrganizationID, contract.ProjectID, string, int) ([]DeliveryMetricSnapshot, error)
	ListProjectMetricSnapshots(context.Context, contract.OrganizationID, contract.ProjectID, int) ([]DeliveryMetricSnapshot, error)
	UpsertAlert(context.Context, DeliveryAlert) (DeliveryAlert, error)
	ListAlerts(context.Context, contract.OrganizationID, contract.ProjectID, AlertFilter) ([]DeliveryAlert, error)
	UpdateAlert(context.Context, contract.OrganizationID, contract.ProjectID, string, AlertAction, int64, string, time.Time) (DeliveryAlert, error)
}

type Service struct {
	Repository              Repository
	Projects                ActiveProjectResolver
	Adapter                 PlatformAdapter
	Insights                InsightsConsumer
	ConnectorSnapshots      ConnectorSnapshotReader
	ConnectorAccounts       ConnectorAccountReader
	ExternalAccountIDs      ExternalAccountIDResolver
	BrowserRpaLauncher      BrowserRpaRunLauncher
	LaunchBatchCalibrations ConnectorLaunchBatchCalibrationReader
	NewID                   ids.Generator
	Now                     func() time.Time
}

func (s Service) CreatePlan(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, request CreatePlanRequest) (DeliveryPlan, error) {
	return s.createPlan(ctx, actor, projectID, request, "", "")
}

func (s Service) createPlan(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, request CreatePlanRequest, tourRunID, tourCase string) (DeliveryPlan, error) {
	if err := s.ready(actor, projectID, ScopeWrite); err != nil {
		return DeliveryPlan{}, err
	}
	if err := request.Validate(); err != nil {
		return DeliveryPlan{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return DeliveryPlan{}, err
	}
	if tourRunID == "" {
		if err := s.validateProjectAccount(ctx, actor, projectID, request.PlatformConfiguration); err != nil {
			return DeliveryPlan{}, err
		}
	}
	tourOwnerID := ""
	if tourRunID == "" && tourCase != "" || tourRunID != "" && tourCase == "" {
		return DeliveryPlan{}, ErrInvalidRequest
	}
	if tourRunID != "" {
		tourOwnerID = actor.Principal.ID
	}
	id, err := s.idGenerator()("deliveryplan")
	if err != nil {
		return DeliveryPlan{}, err
	}
	now := s.now()
	version, err := newPlatformPlanVersion(id, actor, projectID, 1, *request.Intent, *request.PlatformConfiguration, now)
	if err != nil {
		return DeliveryPlan{}, err
	}
	plan := planProjectionFromPlatformVersion(id, actor, projectID, version, now)
	plan.TourRunID, plan.TourOwnerID, plan.TourCase = tourRunID, tourOwnerID, tourCase
	return s.Repository.CreatePlan(ctx, plan, version)
}

func (s Service) UpdatePlan(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, planID string, request UpdatePlanRequest) (DeliveryPlan, error) {
	if err := s.ready(actor, projectID, ScopeWrite); err != nil {
		return DeliveryPlan{}, err
	}
	if strings.TrimSpace(planID) == "" {
		return DeliveryPlan{}, ErrInvalidRequest
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return DeliveryPlan{}, err
	}
	plan, err := s.Repository.GetPlan(ctx, actor.OrganizationID, projectID, planID)
	if err != nil {
		return DeliveryPlan{}, err
	}
	if plan.Status != DeliveryPlanDraft {
		return DeliveryPlan{}, ErrInvalidState
	}
	if plan.CurrentVersion.ReadOnly || !plan.CurrentVersion.IsPlatformConfigurationV2() {
		return DeliveryPlan{}, ErrLegacyConfigurationUnsupported
	}
	if err := request.Validate(); err != nil {
		return DeliveryPlan{}, ErrInvalidRequest
	}
	if plan.TourRunID == "" {
		if err := s.validateProjectAccount(ctx, actor, projectID, request.PlatformConfiguration); err != nil {
			return DeliveryPlan{}, err
		}
	}
	version, err := newPlatformPlanVersion(plan.ID, actor, projectID, request.ExpectedVersion+1, *request.Intent, *request.PlatformConfiguration, s.now())
	if err != nil {
		return DeliveryPlan{}, err
	}
	return s.Repository.UpdatePlan(ctx, actor.OrganizationID, projectID, planID, request.ExpectedVersion, version)
}

func (s Service) validateProjectAccount(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, configuration *PlatformConfiguration) error {
	if s.ConnectorAccounts == nil {
		return nil
	}
	if configuration == nil || configuration.Payload.OceanEngine == nil || configuration.Payload.OceanEngine.Project == nil {
		return ErrInvalidRequest
	}
	accountID := strings.TrimSpace(configuration.Payload.OceanEngine.Project.AccountReference.ID)
	if !strings.HasPrefix(accountID, "oeacct_") {
		return fmt.Errorf("%w: select a verified Connector account from the current project", ErrInvalidRequest)
	}
	accounts, err := s.ConnectorAccounts.ListAccounts(ctx, string(actor.OrganizationID), string(projectID))
	if err != nil {
		return err
	}
	for _, account := range accounts {
		if account.ID == accountID && account.Status == "verified" && account.ProjectID == string(projectID) {
			return nil
		}
	}
	return fmt.Errorf("%w: the Connector account is not verified in the current project", ErrInvalidRequest)
}

// validateVersionBlocking runs the authoritative preflight rules against a
// plan version that is about to be executed. If any error-level check fails,
// it returns a ValidationFailedError with the full check list. This is
// distinct from save-time validation: save accepts incomplete drafts (missing
// references, capability_pending), while execute requires them resolved.
func validateVersionBlocking(version DeliveryPlanVersion) error {
	checks := RunPreflight(version)
	blocked := false
	for _, check := range checks {
		if !check.Passed && check.Severity == CheckSeverityError {
			blocked = true
			break
		}
	}
	if blocked {
		return &ValidationFailedError{Checks: checks}
	}
	return nil
}

func (s Service) ListPlans(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, limit int) ([]DeliveryPlan, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return nil, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	return s.Repository.ListPlans(ctx, actor.OrganizationID, projectID, normalizeLimit(limit))
}

func (s Service) GetPlan(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, planID string) (DeliveryPlan, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return DeliveryPlan{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return DeliveryPlan{}, err
	}
	return s.Repository.GetPlan(ctx, actor.OrganizationID, projectID, planID)
}

func (s Service) ListPlanVersions(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, planID string) ([]DeliveryPlanVersion, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return nil, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	return s.Repository.ListPlanVersions(ctx, actor.OrganizationID, projectID, planID)
}

func (s Service) GetPlanVersion(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, planID string, version int) (DeliveryPlanVersion, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return DeliveryPlanVersion{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return DeliveryPlanVersion{}, err
	}
	return s.Repository.GetPlanVersion(ctx, actor.OrganizationID, projectID, planID, version)
}

func (s Service) RunPlanPreflight(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, planID string) (PreflightResult, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return PreflightResult{}, err
	}
	plan, err := s.GetPlan(ctx, actor, projectID, planID)
	if err != nil {
		return PreflightResult{}, err
	}
	if plan.CurrentVersion.ReadOnly || !plan.CurrentVersion.IsPlatformConfigurationV2() {
		return PreflightResult{}, ErrLegacyConfigurationUnsupported
	}
	checks := RunPreflight(plan.CurrentVersion)
	return preflightResult(plan.ID, plan.CurrentVersion, checks, s.now()), nil
}

func preflightResult(planID string, version DeliveryPlanVersion, checks []PreflightCheck, checkedAt time.Time) PreflightResult {
	blocked := false
	for _, check := range checks {
		if !check.Passed && check.Severity == CheckSeverityError {
			blocked = true
			break
		}
	}
	return PreflightResult{
		PlanID: planID, PlanVersion: version.VersionNumber, Passed: !blocked, Blocked: blocked,
		Checks: checks, Source: SourceMock, Scenario: version.Scenario, CheckedAt: checkedAt,
	}
}

func (s Service) ListChangeSets(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, limit int) ([]ChangeSet, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return nil, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	values, err := s.Repository.ListChangeSets(ctx, actor.OrganizationID, projectID, normalizeLimit(limit))
	if err != nil {
		return nil, err
	}
	for index := range values {
		values[index], err = s.hydrateChangeSet(ctx, actor.OrganizationID, projectID, values[index])
		if err != nil {
			return nil, err
		}
	}
	return values, nil
}

func (s Service) GetChangeSet(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, changeSetID string) (ChangeSet, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return ChangeSet{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ChangeSet{}, err
	}
	value, err := s.Repository.GetChangeSet(ctx, actor.OrganizationID, projectID, changeSetID)
	if err != nil {
		return ChangeSet{}, err
	}
	return s.hydrateChangeSet(ctx, actor.OrganizationID, projectID, value)
}

func (s Service) hydrateChangeSet(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, value ChangeSet) (ChangeSet, error) {
	version, err := s.Repository.GetPlanVersion(ctx, organizationID, projectID, value.PlanID, int(value.PlanVersion))
	if err != nil {
		return ChangeSet{}, err
	}
	value.PlanName = version.Name
	value.Source, value.Scenario = version.Source, version.Scenario
	if value.TargetSnapshot != nil {
		value.Scenario = ScenarioPlatformConfiguration
		if value.TargetSnapshot.Platform == DeliveryPlatformMagneticEngine {
			value.Scenario = ScenarioCapabilityPending
		}
	} else if value.LegacyTargetSnapshot != nil {
		value.Source, value.Scenario = value.LegacyTargetSnapshot.Source, Scenario(value.LegacyTargetSnapshot.Scenario)
		value.RuntimeStatus, value.ReadOnly = PlanRuntimeLegacyUnsupported, true
	}
	if version.IsLegacy() {
		value.RuntimeStatus, value.ReadOnly = PlanRuntimeLegacyUnsupported, true
	} else if value.RuntimeStatus == "" {
		value.RuntimeStatus = version.RuntimeStatus
	}
	value.PlanName = versionName(version)
	value.PlanCanonicalHash = version.CanonicalHash
	value.BudgetLimit = versionBudget(version)
	approval, err := s.Repository.GetApproval(ctx, organizationID, projectID, value.ID)
	if errors.Is(err, ErrNotFound) {
		value.Approval = nil
		return value, nil
	}
	if err != nil {
		return ChangeSet{}, err
	}
	plan, err := s.Repository.GetPlan(ctx, organizationID, projectID, value.PlanID)
	if err != nil {
		return ChangeSet{}, err
	}
	view, err := s.approvalView(value, plan, version, approval)
	if err != nil {
		return ChangeSet{}, err
	}
	value.Approval = &view
	value.ApprovedBy = approval.ApprovedBy
	approvedAt := approval.ApprovedAt
	value.ApprovedAt = &approvedAt
	return value, nil
}

func (s Service) approvalView(changeSet ChangeSet, plan DeliveryPlan, version DeliveryPlanVersion, approval DeliveryApproval) (ApprovalView, error) {
	view := ApprovalView{
		DeliveryApproval: approval,
		RuntimeStatus:    version.RuntimeStatus,
		ReadOnly:         version.IsLegacy(),
		Valid:            true,
		HashSummary:      hashSummary(approval.PlanCanonicalHash),
		BudgetLimit:      Budget{TotalMinor: approval.BudgetLimitMinor, Currency: approval.Currency},
	}
	if view.ReadOnly {
		view.RuntimeStatus = PlanRuntimeLegacyUnsupported
	}
	if version.IsPlatformConfigurationV2() {
		view.BudgetLimit = versionBudget(version)
	}
	if !s.now().Before(approval.ExpiresAt) {
		view.Valid, view.InvalidReason = false, ApprovalInvalidExpired
		return view, nil
	}
	if plan.Version != approval.PlanVersion {
		view.Valid, view.InvalidReason = false, ApprovalInvalidStalePlan
		return view, nil
	}
	approvedChangeSetVersion, validLifecycleState := approvalVersionForChangeSetState(changeSet.Status, changeSet.Version)
	if approval.OrganizationID != changeSet.OrganizationID ||
		approval.ProjectID != changeSet.ProjectID ||
		approval.PlanID != changeSet.PlanID ||
		approval.PlanVersion != changeSet.PlanVersion ||
		approval.ChangeSetID != changeSet.ID ||
		!validLifecycleState ||
		approval.ChangeSetVersion != approvedChangeSetVersion ||
		approval.PlanCanonicalHash != version.CanonicalHash ||
		approval.TargetSnapshotHash != changeSet.TargetSnapshotHash ||
		approval.Source != SourceMock ||
		approval.Scenario != version.Scenario {
		view.Valid, view.InvalidReason = false, ApprovalInvalidContentMismatch
		return view, nil
	}
	if err := validatePlanCanonicalHash(version); err != nil {
		view.Valid, view.InvalidReason = false, ApprovalInvalidContentMismatch
		return view, nil
	}
	actionHash, err := ApprovalActionHash(approval)
	if err != nil {
		return ApprovalView{}, err
	}
	if actionHash != approval.ActionHash {
		view.Valid, view.InvalidReason = false, ApprovalInvalidContentMismatch
		return view, nil
	}
	if approval.Action != ApprovalActionExecute ||
		approval.Scope != ApprovalScopeExecuteMock ||
		versionBudget(version).TotalMinor > approval.BudgetLimitMinor ||
		versionBudget(version).Currency != approval.Currency {
		view.Valid, view.InvalidReason = false, ApprovalInvalidScopeExceeded
	}
	if version.IsPlatformConfigurationV2() {
		configuration, intent := version.PlatformConfiguration, version.DeliveryIntent
		if changeSet.TargetSnapshot != nil {
			configuration = changeSet.TargetSnapshot
		}
		if approval.ConfigurationSchemaVersion != configuration.SchemaVersion || approval.ConfigurationID != configuration.ConfigurationID ||
			approval.ConfigurationVersion != configuration.VersionNumber || approval.ConfigurationPlatform != configuration.Platform ||
			approval.ConfigurationProfileVersion != configuration.ProfileVersion || approval.ConfigurationCanonicalHash != configuration.CanonicalHash ||
			approval.IntentSchemaVersion != intent.SchemaVersion || approval.IntentID != intent.IntentID || approval.IntentVersion != intent.VersionNumber ||
			approval.IntentCanonicalHash != intent.CanonicalHash {
			view.Valid, view.InvalidReason = false, ApprovalInvalidContentMismatch
		}
	}
	return view, nil
}

func (s Service) GetPlanDetail(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, planID string) (PlanDetail, error) {
	plan, err := s.GetPlan(ctx, actor, projectID, planID)
	if err != nil {
		return PlanDetail{}, err
	}
	changeSets, err := s.ListChangeSets(ctx, actor, projectID, 100)
	if err != nil {
		return PlanDetail{}, err
	}
	filtered := make([]ChangeSet, 0)
	for _, value := range changeSets {
		if value.PlanID == planID {
			filtered = append(filtered, value)
		}
	}
	executions, err := s.ListExecutions(ctx, actor, projectID, 100)
	if err != nil {
		return PlanDetail{}, err
	}
	filteredExecutions := make([]ExecutionResult, 0)
	for _, value := range executions {
		for _, changeSet := range filtered {
			if value.Execution.ChangeSetID == changeSet.ID {
				filteredExecutions = append(filteredExecutions, value)
			}
		}
	}
	return PlanDetail{Plan: plan, ChangeSets: filtered, Executions: filteredExecutions}, nil
}

func (s Service) CreateChangeSet(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, planID string, expectedPlanVersion int64) (ChangeSet, error) {
	if err := s.ready(actor, projectID, ScopeWrite); err != nil {
		return ChangeSet{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ChangeSet{}, err
	}
	plan, err := s.Repository.GetPlan(ctx, actor.OrganizationID, projectID, planID)
	if err != nil {
		return ChangeSet{}, err
	}
	if plan.Version != expectedPlanVersion {
		return ChangeSet{}, ErrVersionConflict
	}
	if plan.CurrentVersion.ReadOnly {
		return ChangeSet{}, ErrLegacyConfigurationUnsupported
	}
	if err := validatePlanCanonicalHash(plan.CurrentVersion); err != nil {
		return ChangeSet{}, err
	}
	id, err := s.idGenerator()("deliverychangeset")
	if err != nil {
		return ChangeSet{}, err
	}
	now := s.now()
	changeSet := ChangeSet{
		ID: id, OrganizationID: actor.OrganizationID, ProjectID: projectID, PlanID: plan.ID,
		PlanName: versionName(plan.CurrentVersion), PlanVersion: plan.Version, PlanCanonicalHash: plan.CurrentVersion.CanonicalHash,
		BudgetLimit: versionBudget(plan.CurrentVersion), Status: ChangeSetDraft, RiskLevel: "low",
		PreflightNotes: []string{}, Source: plan.Source, Scenario: plan.Scenario,
		Version: 1, CreatedBy: actor.Principal.ID, CreatedAt: now, UpdatedAt: now,
	}
	changeSet.TargetSnapshot = cloneJSONPointer(plan.CurrentVersion.PlatformConfiguration)
	changeSet.TargetSnapshotHash = plan.CurrentVersion.PlatformConfiguration.CanonicalHash
	return s.Repository.CreateChangeSet(ctx, changeSet)
}

func (s Service) Preflight(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, changeSetID string, expectedVersion int64) (ChangeSet, error) {
	if err := s.ready(actor, projectID, ScopeWrite); err != nil {
		return ChangeSet{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ChangeSet{}, err
	}
	value, err := s.Repository.GetChangeSet(ctx, actor.OrganizationID, projectID, changeSetID)
	if err != nil {
		return ChangeSet{}, err
	}
	if value.Status != ChangeSetDraft {
		return ChangeSet{}, ErrInvalidState
	}
	if value.LegacyTargetSnapshot != nil || value.TargetSnapshot == nil {
		return ChangeSet{}, ErrLegacyConfigurationUnsupported
	}
	plan, err := s.Repository.GetPlan(ctx, actor.OrganizationID, projectID, value.PlanID)
	if err != nil {
		return ChangeSet{}, err
	}
	if plan.Version != value.PlanVersion {
		return ChangeSet{}, ErrStalePlanVersion
	}
	version, err := s.Repository.GetPlanVersion(ctx, actor.OrganizationID, projectID, value.PlanID, int(value.PlanVersion))
	if err != nil {
		return ChangeSet{}, err
	}
	if version.ReadOnly || !version.IsPlatformConfigurationV2() {
		return ChangeSet{}, ErrLegacyConfigurationUnsupported
	}
	preflightVersion, err := changeSetPreflightVersion(version, value)
	if err != nil {
		return ChangeSet{}, err
	}
	next := ChangeSetPreflightPassed
	for _, check := range RunPreflight(preflightVersion) {
		if !check.Passed && check.Severity == CheckSeverityError {
			next = ChangeSetPreflightFailed
			break
		}
	}
	transitioned, err := s.Repository.TransitionChangeSet(ctx, actor.OrganizationID, projectID, changeSetID, expectedVersion, next, actor.Principal.ID, s.now())
	if err != nil {
		return ChangeSet{}, err
	}
	return s.hydrateChangeSet(ctx, actor.OrganizationID, projectID, transitioned)
}

func (s Service) Approve(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, changeSetID string, expectedVersion int64) (ChangeSet, error) {
	if err := s.ready(actor, projectID, ScopeApprove); err != nil {
		return ChangeSet{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ChangeSet{}, err
	}
	value, err := s.Repository.GetChangeSet(ctx, actor.OrganizationID, projectID, changeSetID)
	if err != nil {
		return ChangeSet{}, err
	}
	if value.Status != ChangeSetPreflightPassed {
		return ChangeSet{}, ErrInvalidState
	}
	if value.ReadOnly || value.LegacyTargetSnapshot != nil || value.TargetSnapshot == nil {
		return ChangeSet{}, ErrLegacyConfigurationUnsupported
	}
	if value.Version != expectedVersion {
		return ChangeSet{}, ErrVersionConflict
	}
	plan, err := s.Repository.GetPlan(ctx, actor.OrganizationID, projectID, value.PlanID)
	if err != nil {
		return ChangeSet{}, err
	}
	if plan.Version != value.PlanVersion {
		return ChangeSet{}, ErrStalePlanVersion
	}
	version, err := s.Repository.GetPlanVersion(ctx, actor.OrganizationID, projectID, value.PlanID, int(value.PlanVersion))
	if err != nil {
		return ChangeSet{}, err
	}
	if version.ReadOnly || !version.IsPlatformConfigurationV2() {
		return ChangeSet{}, ErrLegacyConfigurationUnsupported
	}
	if err := validatePlanCanonicalHash(version); err != nil {
		return ChangeSet{}, err
	}
	preflightVersion, err := changeSetPreflightVersion(version, value)
	if err != nil {
		return ChangeSet{}, err
	}
	for _, check := range RunPreflight(preflightVersion) {
		if !check.Passed && check.Severity == CheckSeverityError {
			return ChangeSet{}, ErrInvalidState
		}
	}
	approvalID, err := s.idGenerator()("deliveryapproval")
	if err != nil {
		return ChangeSet{}, err
	}
	now := s.now()
	budget := versionBudget(version)
	approval := DeliveryApproval{
		ApprovalID: approvalID, OrganizationID: actor.OrganizationID, ProjectID: projectID,
		PlanID: value.PlanID, PlanVersion: value.PlanVersion,
		ChangeSetID: value.ID, ChangeSetVersion: value.Version + 1,
		PlanCanonicalHash:  version.CanonicalHash,
		TargetSnapshotHash: value.TargetSnapshotHash,
		Action:             ApprovalActionExecute, Scope: ApprovalScopeExecuteMock,
		BudgetLimitMinor: budget.TotalMinor, Currency: budget.Currency,
		ApprovedBy: actor.Principal.ID, ApprovedAt: now, ExpiresAt: now.Add(ApprovalTTL),
		Source: SourceMock, Scenario: preflightVersion.Scenario,
	}
	if preflightVersion.IsPlatformConfigurationV2() {
		configuration, intent := preflightVersion.PlatformConfiguration, preflightVersion.DeliveryIntent
		approval.ConfigurationSchemaVersion, approval.ConfigurationID = configuration.SchemaVersion, configuration.ConfigurationID
		approval.ConfigurationVersion, approval.ConfigurationPlatform = configuration.VersionNumber, configuration.Platform
		approval.ConfigurationProfileVersion, approval.ConfigurationCanonicalHash = configuration.ProfileVersion, configuration.CanonicalHash
		approval.IntentSchemaVersion, approval.IntentID = intent.SchemaVersion, intent.IntentID
		approval.IntentVersion, approval.IntentCanonicalHash = intent.VersionNumber, intent.CanonicalHash
	}
	approval.ActionHash, err = ApprovalActionHash(approval)
	if err != nil {
		return ChangeSet{}, err
	}
	approved, err := s.Repository.ApproveChangeSet(ctx, value, approval)
	if err != nil {
		return ChangeSet{}, err
	}
	return s.hydrateChangeSet(ctx, actor.OrganizationID, projectID, approved)
}

func (s Service) RejectChangeSet(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, changeSetID string, request RejectChangeSetRequest) (ChangeSet, error) {
	if err := s.ready(actor, projectID, ScopeApprove); err != nil {
		return ChangeSet{}, err
	}
	if err := request.Validate(); err != nil || strings.TrimSpace(changeSetID) == "" {
		return ChangeSet{}, ErrInvalidRequest
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ChangeSet{}, err
	}
	value, err := s.Repository.GetChangeSet(ctx, actor.OrganizationID, projectID, changeSetID)
	if err != nil {
		return ChangeSet{}, err
	}
	if value.Status != ChangeSetPreflightPassed {
		return ChangeSet{}, ErrInvalidState
	}
	repository, ok := s.Repository.(changeSetRejectionRepository)
	if !ok {
		transitioned, transitionErr := s.Repository.TransitionChangeSet(ctx, actor.OrganizationID, projectID, changeSetID, request.ExpectedVersion, ChangeSetRejected, actor.Principal.ID, s.now())
		if transitionErr != nil {
			return ChangeSet{}, transitionErr
		}
		transitioned.RejectedBy, transitioned.RejectionReason = actor.Principal.ID, strings.TrimSpace(request.Reason)
		rejectedAt := s.now()
		transitioned.RejectedAt = &rejectedAt
		return s.hydrateChangeSet(ctx, actor.OrganizationID, projectID, transitioned)
	}
	transitioned, err := repository.RejectChangeSet(ctx, actor.OrganizationID, projectID, changeSetID, request.ExpectedVersion, actor.Principal.ID, strings.TrimSpace(request.Reason), s.now())
	if err != nil {
		return ChangeSet{}, err
	}
	return s.hydrateChangeSet(ctx, actor.OrganizationID, projectID, transitioned)
}

func (s Service) Execute(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, changeSetID, idempotencyKey string, request ExecuteRequest) (ExecutionResult, bool, error) {
	if err := s.ready(actor, projectID, ScopeExecute); err != nil {
		return ExecutionResult{}, false, err
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if err := request.Validate(); err != nil || len(idempotencyKey) < 1 || len(idempotencyKey) > 255 {
		return ExecutionResult{}, false, ErrInvalidRequest
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ExecutionResult{}, false, err
	}
	value, err := s.Repository.GetChangeSet(ctx, actor.OrganizationID, projectID, changeSetID)
	if err != nil {
		return ExecutionResult{}, false, err
	}
	requestHash, err := contract.CanonicalJSONHash(struct {
		OrganizationID  contract.OrganizationID `json:"organization_id"`
		ProjectID       contract.ProjectID      `json:"project_id"`
		ChangeSetID     string                  `json:"change_set_id"`
		Operation       string                  `json:"operation"`
		ExpectedVersion int64                   `json:"expected_version"`
		Scenario        ExecutionScenario       `json:"scenario"`
	}{actor.OrganizationID, projectID, value.ID, "execute_mock", request.ExpectedVersion, request.Scenario})
	if err != nil {
		return ExecutionResult{}, false, err
	}
	if existing, found, findErr := s.Repository.FindExecutionByIdempotency(ctx, actor.OrganizationID, projectID, idempotencyKey); findErr != nil {
		return ExecutionResult{}, false, findErr
	} else if found {
		if existing.Execution.RequestHash != requestHash {
			return ExecutionResult{}, false, ErrIdempotencyConflict
		}
		existing, err = s.hydrateExecutionResult(ctx, actor.OrganizationID, projectID, existing)
		return existing, true, err
	}
	if value.Version != request.ExpectedVersion {
		return ExecutionResult{}, false, ErrVersionConflict
	}
	if value.Status != ChangeSetApproved {
		return ExecutionResult{}, false, ErrInvalidState
	}
	plan, err := s.Repository.GetPlan(ctx, actor.OrganizationID, projectID, value.PlanID)
	if err != nil {
		return ExecutionResult{}, false, err
	}
	if plan.Version != value.PlanVersion {
		return ExecutionResult{}, false, ErrStalePlanVersion
	}
	version, err := s.Repository.GetPlanVersion(ctx, actor.OrganizationID, projectID, value.PlanID, int(value.PlanVersion))
	if err != nil {
		return ExecutionResult{}, false, err
	}
	if version.ReadOnly || !version.IsPlatformConfigurationV2() || value.ReadOnly || value.LegacyTargetSnapshot != nil {
		return ExecutionResult{}, false, ErrLegacyConfigurationUnsupported
	}
	value.PlanName = version.Name
	value.Source, value.Scenario = version.Source, version.Scenario
	value.PlanCanonicalHash, value.BudgetLimit = version.CanonicalHash, version.Budget
	approval, err := s.Repository.GetApproval(ctx, actor.OrganizationID, projectID, value.ID)
	if errors.Is(err, ErrNotFound) {
		return ExecutionResult{}, false, ErrApprovalRequired
	}
	if err != nil {
		return ExecutionResult{}, false, err
	}
	view, err := s.approvalView(value, plan, version, approval)
	if err != nil {
		return ExecutionResult{}, false, err
	}
	if !view.Valid {
		switch view.InvalidReason {
		case ApprovalInvalidExpired:
			return ExecutionResult{}, false, ErrApprovalExpired
		case ApprovalInvalidStalePlan:
			return ExecutionResult{}, false, ErrStalePlanVersion
		case ApprovalInvalidScopeExceeded:
			return ExecutionResult{}, false, ErrApprovalScopeExceeded
		default:
			return ExecutionResult{}, false, ErrApprovalContentMismatch
		}
	}
	executionID, err := s.idGenerator()("deliveryexecution")
	if err != nil {
		return ExecutionResult{}, false, err
	}
	evidenceID, err := s.idGenerator()("deliveryevidence")
	if err != nil {
		return ExecutionResult{}, false, err
	}
	now := s.now()
	actions := []string{"create_platform_project", "create_promotion", "verify_platform_state"}
	steps := make([]ExecutionStep, len(actions))
	for index, action := range actions {
		steps[index].ID, err = s.idGenerator()("deliveryexecutionstep")
		if err != nil {
			return ExecutionResult{}, false, err
		}
		steps[index].Sequence = index + 1
		steps[index].Action = action
		steps[index].Status = StepPending
		steps[index].Effect = "none"
		steps[index].OutcomeSummary = "queued; no adapter call has occurred"
		steps[index].Version = 1
	}
	adapter := s.platformAdapter()
	if adapter.Source() != SourceMock {
		return ExecutionResult{}, false, fmt.Errorf("%w: A04 only permits the mock platform adapter", ErrInvalidRequest)
	}
	execution := Execution{
		ID: executionID, OrganizationID: actor.OrganizationID, ProjectID: projectID,
		ChangeSetID: value.ID, ApprovalID: approval.ApprovalID, Status: ExecutionQueued, Version: 1,
		Mode: ExecutionModeLocalSimulation, Adapter: MockOceanEngineAdapter, Source: SourceMock,
		Scenario: request.Scenario, IdempotencyKey: idempotencyKey, RequestHash: requestHash,
		ExecutedBy: actor.Principal.ID, StartedAt: now, RetryAllowed: false,
		RecoveryAction: "none", RecoveryReason: "", CompensationCandidates: []string{}, Steps: steps,
	}
	evidence := Evidence{
		ID: evidenceID, OrganizationID: actor.OrganizationID, ProjectID: projectID,
		ExecutionID: execution.ID, Summary: "本地模拟执行记录，无真实广告平台写入。",
		Mode: ExecutionModeLocalSimulation, Reversible: false,
		Source: SourceMock, Scenario: request.Scenario, References: []string{"mock://execution/" + string(request.Scenario)}, CreatedAt: now,
	}
	value.Approval = &view
	created, replay, err := s.Repository.CreateOrReplayExecution(ctx, value, approval, execution, evidence)
	if err != nil {
		return created, replay, err
	}
	if replay {
		created, err = s.hydrateExecutionResult(ctx, actor.OrganizationID, projectID, created)
		return created, true, err
	}
	for _, state := range []ExecutionStatus{ExecutionValidatingApproval, ExecutionExecuting} {
		created, err = s.Repository.AdvanceExecution(ctx, created.Execution, state, nil, "none", "", []string{})
		if err != nil {
			return ExecutionResult{}, false, err
		}
	}

	adapterUnknown := false
	for index := range created.Execution.Steps {
		current := created.Execution.Steps[index]
		if current.Action == "verify_platform_state" {
			created, err = s.Repository.AdvanceExecution(ctx, created.Execution, ExecutionVerifying, nil, "none", "", []string{})
			if err != nil {
				return ExecutionResult{}, false, err
			}
			current = created.Execution.Steps[index]
		}
		if (request.Scenario == ExecutionScenarioFailed && current.Sequence > 1) || adapterUnknown {
			skipped := current
			skipped.Status = StepSkipped
			skipped.Effect = "none"
			skipped.OutcomeSummary = "not run after a prior terminal step outcome"
			skipped.EvidenceRef = "mock://execution/skipped"
			completedAt := s.now()
			skipped.CompletedAt = &completedAt
			created.Execution.Steps[index], err = s.Repository.AdvanceStep(ctx, created.Execution, current, skipped)
			if err != nil {
				return ExecutionResult{}, false, err
			}
			continue
		}

		running := current
		running.Status = StepRunning
		running.Attempt++
		running.Effect = "none"
		running.OutcomeSummary = "adapter call in progress"
		startedAt := s.now()
		running.StartedAt = &startedAt
		running, err = s.Repository.AdvanceStep(ctx, created.Execution, current, running)
		if err != nil {
			return ExecutionResult{}, false, err
		}
		created.Execution.Steps[index] = running

		stepResult, adapterErr := adapter.ExecuteStep(ctx, PlatformStepRequest{
			ExecutionID: executionID, Scenario: request.Scenario, Action: running.Action, Sequence: running.Sequence,
		})
		completedAt := s.now()
		terminal := running
		terminal.CompletedAt = &completedAt
		if adapterErr != nil {
			adapterUnknown = true
			terminal.Status = StepResultUnknown
			terminal.Effect = "unknown"
			terminal.OutcomeSummary = "adapter returned an error after the step began; target effect is unknown"
			terminal.EvidenceRef = "mock://execution/adapter-error"
		} else {
			terminal.Status = stepResult.Status
			terminal.Effect = stepResult.Effect
			terminal.OutcomeSummary = stepResult.Summary
			terminal.EvidenceRef = stepResult.EvidenceRef
		}
		created.Execution.Steps[index], err = s.Repository.AdvanceStep(ctx, created.Execution, running, terminal)
		if err != nil {
			return ExecutionResult{}, false, err
		}
	}

	status, recoveryAction, recoveryReason, compensation := executionOutcome(request.Scenario)
	if adapterUnknown {
		status, recoveryAction = ExecutionResultUnknown, "query_and_reconcile"
		recoveryReason, compensation = "adapter result is unknown; blind retry is prohibited", []string{}
	}
	completedAt := s.now()
	created, err = s.Repository.AdvanceExecution(ctx, created.Execution, status, &completedAt, recoveryAction, recoveryReason, compensation)
	if err != nil {
		return ExecutionResult{}, false, err
	}
	created, err = s.hydrateExecutionResult(ctx, actor.OrganizationID, projectID, created)
	return created, false, err
}

func (s Service) Rollback(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, changeSetID string, expectedVersion int64) (ChangeSet, error) {
	if err := s.ready(actor, projectID, ScopeExecute); err != nil {
		return ChangeSet{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ChangeSet{}, err
	}
	value, err := s.Repository.GetChangeSet(ctx, actor.OrganizationID, projectID, changeSetID)
	if err != nil {
		return ChangeSet{}, err
	}
	if value.Status != ChangeSetExecuted {
		return ChangeSet{}, ErrInvalidState
	}
	execution, err := s.Repository.GetExecutionByChangeSet(ctx, actor.OrganizationID, projectID, changeSetID)
	if err != nil {
		return ChangeSet{}, err
	}
	if execution.Execution.Status != ExecutionSucceeded || execution.Execution.CompletedAt == nil {
		return ChangeSet{}, ErrInvalidState
	}
	transitioned, err := s.Repository.TransitionChangeSet(
		ctx, actor.OrganizationID, projectID, changeSetID, expectedVersion,
		ChangeSetRolledBack, actor.Principal.ID, s.now(),
	)
	if err != nil {
		return ChangeSet{}, err
	}
	return s.hydrateChangeSet(ctx, actor.OrganizationID, projectID, transitioned)
}

// ExecutePlanRequest is the input for the simplified "confirm launch" action
// that writes a plan directly to the platform without ChangeSet/Approval.
type ExecutePlanRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}

func (r ExecutePlanRequest) Validate() error {
	if r.ExpectedVersion < 1 {
		return ErrInvalidRequest
	}
	return nil
}

// ExecutePlan writes the current plan version directly to the platform
// adapter. This is the simplified "confirm launch" action that replaces the
// CreateChangeSet → Preflight → Approve → Execute chain for daily operations.
func (s Service) ExecutePlan(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, planID, idempotencyKey string, request ExecutePlanRequest) (ExecutionResult, bool, error) {
	if err := s.ready(actor, projectID, ScopeExecute); err != nil {
		return ExecutionResult{}, false, err
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if err := request.Validate(); err != nil || len(idempotencyKey) < 1 || len(idempotencyKey) > 255 {
		return ExecutionResult{}, false, ErrInvalidRequest
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ExecutionResult{}, false, err
	}
	plan, err := s.Repository.GetPlan(ctx, actor.OrganizationID, projectID, planID)
	if err != nil {
		return ExecutionResult{}, false, err
	}
	if plan.Version != request.ExpectedVersion {
		return ExecutionResult{}, false, ErrVersionConflict
	}
	version := plan.CurrentVersion
	if version.ReadOnly || !version.IsPlatformConfigurationV2() {
		return ExecutionResult{}, false, ErrLegacyConfigurationUnsupported
	}
	// Run preflight inline before execution.
	if err := validateVersionBlocking(version); err != nil {
		return ExecutionResult{}, false, err
	}
	requestHash, err := contract.CanonicalJSONHash(struct {
		OrganizationID  contract.OrganizationID `json:"organization_id"`
		ProjectID       contract.ProjectID      `json:"project_id"`
		PlanID          string                  `json:"plan_id"`
		PlanVersion     int64                   `json:"plan_version"`
		Operation       string                  `json:"operation"`
		ExpectedVersion int64                   `json:"expected_version"`
	}{actor.OrganizationID, projectID, plan.ID, plan.Version, "execute_plan", request.ExpectedVersion})
	if err != nil {
		return ExecutionResult{}, false, err
	}
	if existing, found, findErr := s.Repository.FindExecutionByIdempotency(ctx, actor.OrganizationID, projectID, idempotencyKey); findErr != nil {
		return ExecutionResult{}, false, findErr
	} else if found {
		if existing.Execution.RequestHash != requestHash {
			return ExecutionResult{}, false, ErrIdempotencyConflict
		}
		existing, err = s.hydrateExecutionResult(ctx, actor.OrganizationID, projectID, existing)
		return existing, true, err
	}
	executionID, err := s.idGenerator()("deliveryexecution")
	if err != nil {
		return ExecutionResult{}, false, err
	}
	evidenceID, err := s.idGenerator()("deliveryevidence")
	if err != nil {
		return ExecutionResult{}, false, err
	}
	now := s.now()
	actions := []string{"create_platform_project", "create_promotion", "verify_platform_state"}
	steps := make([]ExecutionStep, len(actions))
	for index, action := range actions {
		steps[index].ID, err = s.idGenerator()("deliveryexecutionstep")
		if err != nil {
			return ExecutionResult{}, false, err
		}
		steps[index].Sequence = index + 1
		steps[index].Action = action
		steps[index].Status = StepPending
		steps[index].Effect = "none"
		steps[index].OutcomeSummary = "queued; no adapter call has occurred"
		steps[index].Version = 1
	}
	adapter := s.platformAdapter()
	if adapter.Source() != SourceMock {
		return ExecutionResult{}, false, fmt.Errorf("%w: only the mock platform adapter is permitted", ErrInvalidRequest)
	}
	// Build an audit ChangeSet bound to this direct write. The ChangeSet is an
	// operatation log only; daily direct writes do not create an Approval.
	changeSetID, err := s.idGenerator()("deliverychangeset")
	if err != nil {
		return ExecutionResult{}, false, err
	}
	changeSet := ChangeSet{
		ID: changeSetID, OrganizationID: actor.OrganizationID, ProjectID: projectID,
		PlanID: plan.ID, PlanName: versionName(version), PlanVersion: plan.Version,
		PlanCanonicalHash: version.CanonicalHash, BudgetLimit: versionBudget(version),
		Status: ChangeSetExecuted, RiskLevel: "low", PreflightNotes: []string{},
		Source: version.Source, Scenario: version.Scenario,
		Version: 1, CreatedBy: actor.Principal.ID, CreatedAt: now, UpdatedAt: now,
	}
	changeSet.TargetSnapshot = cloneJSONPointer(version.PlatformConfiguration)
	changeSet.TargetSnapshotHash = version.PlatformConfiguration.CanonicalHash
	execution := Execution{
		ID: executionID, OrganizationID: actor.OrganizationID, ProjectID: projectID,
		ChangeSetID: changeSet.ID, Status: ExecutionQueued, Version: 1,
		Mode: ExecutionModeLocalSimulation, Adapter: MockOceanEngineAdapter, Source: SourceMock,
		Scenario: ExecutionScenarioSuccess, IdempotencyKey: idempotencyKey, RequestHash: requestHash,
		ExecutedBy: actor.Principal.ID, StartedAt: now, RetryAllowed: false,
		RecoveryAction: "none", RecoveryReason: "", CompensationCandidates: []string{}, Steps: steps,
	}
	evidence := Evidence{
		ID: evidenceID, OrganizationID: actor.OrganizationID, ProjectID: projectID,
		ExecutionID: execution.ID, Summary: "本地模拟执行记录，无真实广告平台写入。",
		Mode: ExecutionModeLocalSimulation, Reversible: false,
		Source: SourceMock, Scenario: ExecutionScenarioSuccess, References: []string{"mock://execution/" + string(ExecutionScenarioSuccess)}, CreatedAt: now,
	}
	created, err := s.Repository.RecordDirectExecution(ctx, changeSet, execution, evidence)
	if err != nil {
		return ExecutionResult{}, false, err
	}
	for _, state := range []ExecutionStatus{ExecutionValidatingApproval, ExecutionExecuting} {
		created, err = s.Repository.AdvanceExecution(ctx, created.Execution, state, nil, "none", "", []string{})
		if err != nil {
			return ExecutionResult{}, false, err
		}
	}
	adapterUnknown := false
	for index := range created.Execution.Steps {
		current := created.Execution.Steps[index]
		if current.Action == "verify_platform_state" {
			created, err = s.Repository.AdvanceExecution(ctx, created.Execution, ExecutionVerifying, nil, "none", "", []string{})
			if err != nil {
				return ExecutionResult{}, false, err
			}
			current = created.Execution.Steps[index]
		}
		if adapterUnknown {
			skipped := current
			skipped.Status = StepSkipped
			skipped.Effect = "none"
			skipped.OutcomeSummary = "not run after a prior terminal step outcome"
			skipped.EvidenceRef = "mock://execution/skipped"
			completedAt := s.now()
			skipped.CompletedAt = &completedAt
			created.Execution.Steps[index], err = s.Repository.AdvanceStep(ctx, created.Execution, current, skipped)
			if err != nil {
				return ExecutionResult{}, false, err
			}
			continue
		}
		running := current
		running.Status = StepRunning
		running.Attempt++
		running.Effect = "none"
		running.OutcomeSummary = "adapter call in progress"
		startedAt := s.now()
		running.StartedAt = &startedAt
		running, err = s.Repository.AdvanceStep(ctx, created.Execution, current, running)
		if err != nil {
			return ExecutionResult{}, false, err
		}
		created.Execution.Steps[index] = running
		stepResult, adapterErr := adapter.ExecuteStep(ctx, PlatformStepRequest{
			ExecutionID: executionID, Scenario: ExecutionScenarioSuccess, Action: running.Action, Sequence: running.Sequence,
		})
		completedAt := s.now()
		terminal := running
		terminal.CompletedAt = &completedAt
		if adapterErr != nil {
			adapterUnknown = true
			terminal.Status = StepResultUnknown
			terminal.Effect = "unknown"
			terminal.OutcomeSummary = "adapter returned an error after the step began; target effect is unknown"
			terminal.EvidenceRef = "mock://execution/adapter-error"
		} else {
			terminal.Status = stepResult.Status
			terminal.Effect = stepResult.Effect
			terminal.OutcomeSummary = stepResult.Summary
			terminal.EvidenceRef = stepResult.EvidenceRef
		}
		created.Execution.Steps[index], err = s.Repository.AdvanceStep(ctx, created.Execution, running, terminal)
		if err != nil {
			return ExecutionResult{}, false, err
		}
	}
	status, recoveryAction, recoveryReason, compensation := executionOutcome(ExecutionScenarioSuccess)
	if adapterUnknown {
		status, recoveryAction = ExecutionResultUnknown, "query_and_reconcile"
		recoveryReason, compensation = "adapter result is unknown; blind retry is prohibited", []string{}
	}
	completedAt := s.now()
	created, err = s.Repository.AdvanceExecution(ctx, created.Execution, status, &completedAt, recoveryAction, recoveryReason, compensation)
	if err != nil {
		return ExecutionResult{}, false, err
	}
	created, err = s.hydrateExecutionResult(ctx, actor.OrganizationID, projectID, created)
	return created, false, err
}

func (s Service) ListExecutions(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, limit int) ([]ExecutionResult, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return nil, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	values, err := s.Repository.ListExecutions(ctx, actor.OrganizationID, projectID, normalizeLimit(limit))
	if err != nil {
		return nil, err
	}
	for index := range values {
		values[index], err = s.hydrateExecutionResult(ctx, actor.OrganizationID, projectID, values[index])
		if err != nil {
			return nil, err
		}
	}
	return values, nil
}

func (s Service) GetExecution(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, executionID string) (ExecutionResult, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return ExecutionResult{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ExecutionResult{}, err
	}
	if strings.TrimSpace(executionID) == "" {
		return ExecutionResult{}, ErrInvalidRequest
	}
	value, err := s.Repository.GetExecution(ctx, actor.OrganizationID, projectID, executionID)
	if err != nil {
		return ExecutionResult{}, err
	}
	return s.hydrateExecutionResult(ctx, actor.OrganizationID, projectID, value)
}

func (s Service) hydrateExecutionResult(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, value ExecutionResult) (ExecutionResult, error) {
	changeSet, err := s.hydrateChangeSet(ctx, organizationID, projectID, value.ChangeSet)
	if err != nil {
		return ExecutionResult{}, err
	}
	value.ChangeSet = changeSet
	if value.Execution.CompensationCandidates == nil {
		value.Execution.CompensationCandidates = []string{}
	}
	if value.Evidence.References == nil {
		value.Evidence.References = []string{}
	}
	if value.Execution.Steps == nil {
		value.Execution.Steps = []ExecutionStep{}
	}
	return value, nil
}

func (s Service) CreateDemoMetricSnapshot(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, executionID string, request CreateMetricSnapshotRequest) (DeliveryMetricSnapshot, error) {
	if err := s.ready(actor, projectID, ScopeWrite); err != nil {
		return DeliveryMetricSnapshot{}, err
	}
	if strings.TrimSpace(executionID) == "" {
		return DeliveryMetricSnapshot{}, ErrInvalidRequest
	}
	if err := request.Validate(); err != nil {
		return DeliveryMetricSnapshot{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return DeliveryMetricSnapshot{}, err
	}
	execution, err := s.findExecution(ctx, actor.OrganizationID, projectID, executionID)
	if err != nil {
		return DeliveryMetricSnapshot{}, err
	}
	if execution.Execution.Mode != ExecutionModeLocalSimulation || execution.Execution.Status != "succeeded" {
		return DeliveryMetricSnapshot{}, ErrInvalidState
	}
	result, err := s.CreateOutcomeSimulation(ctx, actor, projectID, execution.Execution.ID, CreateOutcomeSimulationRequest{Scenario: OutcomeScenarioCostPressure})
	if err != nil {
		return DeliveryMetricSnapshot{}, err
	}
	return result.MetricSnapshots[len(result.MetricSnapshots)-1], nil
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func (s Service) ListMetricSnapshots(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, executionID string, limit int) ([]DeliveryMetricSnapshot, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return nil, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(executionID) == "" {
		return nil, ErrInvalidRequest
	}
	return s.Repository.ListMetricSnapshots(ctx, actor.OrganizationID, projectID, executionID, normalizeLimit(limit))
}

func (s Service) ListExecutionEvidence(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, limit int) ([]ExecutionResult, error) {
	if s.Repository == nil || s.Projects == nil {
		return nil, fmt.Errorf("delivery evidence dependencies are incomplete")
	}
	if actor.OrganizationID == "" || projectID == "" {
		return nil, fmt.Errorf("organization and project are required")
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	return s.Repository.ListExecutions(ctx, actor.OrganizationID, projectID, normalizeLimit(limit))
}

func (s Service) ReadExecutionEvidence(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, executionID string) (ExecutionResult, *DeliveryMetricSnapshot, DeliveryPlan, error) {
	if s.Repository == nil || s.Projects == nil {
		return ExecutionResult{}, nil, DeliveryPlan{}, fmt.Errorf("delivery evidence dependencies are incomplete")
	}
	if actor.OrganizationID == "" || projectID == "" || strings.TrimSpace(executionID) == "" {
		return ExecutionResult{}, nil, DeliveryPlan{}, ErrInvalidRequest
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ExecutionResult{}, nil, DeliveryPlan{}, err
	}
	execution, err := s.findExecution(ctx, actor.OrganizationID, projectID, executionID)
	if err != nil {
		return ExecutionResult{}, nil, DeliveryPlan{}, err
	}
	plan, err := s.Repository.GetPlan(ctx, actor.OrganizationID, projectID, execution.ChangeSet.PlanID)
	if err != nil {
		return ExecutionResult{}, nil, DeliveryPlan{}, err
	}
	values, err := s.Repository.ListMetricSnapshots(ctx, actor.OrganizationID, projectID, executionID, 1)
	if err != nil {
		return ExecutionResult{}, nil, DeliveryPlan{}, err
	}
	if len(values) == 0 {
		return execution, nil, plan, nil
	}
	return execution, &values[0], plan, nil
}

func (s Service) findExecution(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, executionID string) (ExecutionResult, error) {
	values, err := s.Repository.ListExecutions(ctx, organizationID, projectID, 100)
	if err != nil {
		return ExecutionResult{}, err
	}
	for _, value := range values {
		if value.Execution.ID == executionID {
			value.ChangeSet, err = s.hydrateChangeSet(ctx, organizationID, projectID, value.ChangeSet)
			if err != nil {
				return ExecutionResult{}, err
			}
			return value, nil
		}
	}
	return ExecutionResult{}, ErrNotFound
}

func (s Service) ready(actor contract.ActorContext, projectID contract.ProjectID, scope contract.Scope) error {
	if s.Repository == nil || s.Projects == nil {
		return fmt.Errorf("delivery dependencies are incomplete")
	}
	if actor.OrganizationID == "" || projectID == "" || !actor.HasScope(scope) {
		return fmt.Errorf("%s scope is required", scope)
	}
	return nil
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s Service) idGenerator() ids.Generator {
	if s.NewID != nil {
		return s.NewID
	}
	return ids.New
}

func normalizeLimit(value int) int {
	if value < 1 || value > 100 {
		return 50
	}
	return value
}
