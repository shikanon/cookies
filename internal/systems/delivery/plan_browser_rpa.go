package delivery

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/systems/delivery/platformskills"
)

type StartBrowserRpaExecutionRequest struct {
	ExpectedVersion int64                             `json:"expected_version"`
	ExecutionDriver browserautomation.ExecutionDriver `json:"execution_driver,omitempty"`
	IdempotencyKey  string                            `json:"-"`
}

type BrowserRpaLaunchRequest struct {
	OrganizationID      contract.OrganizationID
	ProjectID           contract.ProjectID
	AccountID           string
	ExecutionDriver     browserautomation.ExecutionDriver
	BusinessExecutionID string
	Action              ControlledAction
	ParentProjectID     string
	IdempotencyKey      string
	CreatedBy           string
}

type BrowserRpaLaunchResult struct {
	RunID string `json:"run_id"`
}

type StartBrowserRpaExecutionResult struct {
	ControlledChangeSet ControlledChangeSet    `json:"controlled_change_set"`
	ControlledExecution ControlledExecution    `json:"controlled_execution"`
	BrowserRpaRun       BrowserRpaLaunchResult `json:"browser_rpa_run"`
}

type controlledExecutionByChangeSetRepository interface {
	GetControlledExecutionByChangeSet(context.Context, contract.OrganizationID, contract.ProjectID, string) (ControlledExecution, error)
}

type controlledChangeSetByObjectFingerprintRepository interface {
	GetControlledChangeSetByObjectFingerprint(context.Context, contract.OrganizationID, contract.ProjectID, string) (ControlledChangeSet, error)
}

type safeFailedBrowserRpaRetryRepository interface {
	ResetSafeFailedBrowserRpaExecution(context.Context, contract.OrganizationID, contract.ProjectID, string, int64, string) (ControlledExecution, error)
}

type browserRpaRunReconciler interface {
	ReconcileBrowserRpaRun(context.Context, BrowserRpaLaunchRequest, string) error
}

func (s Service) StartBrowserRpaExecution(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, planID string, request StartBrowserRpaExecutionRequest) (StartBrowserRpaExecutionResult, error) {
	if err := s.ready(actor, projectID, ScopeExecute); err != nil {
		return StartBrowserRpaExecutionResult{}, err
	}
	if request.ExpectedVersion < 1 || strings.TrimSpace(request.IdempotencyKey) == "" || len(request.IdempotencyKey) > 160 || s.ExternalAccountIDs == nil || s.BrowserRpaLauncher == nil {
		return StartBrowserRpaExecutionResult{}, ErrUnsupportedConfigurationWorkflow
	}
	executionDriver, err := normalizePlanExecutionDriver(request.ExecutionDriver)
	if err != nil {
		return StartBrowserRpaExecutionResult{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return StartBrowserRpaExecutionResult{}, err
	}
	plan, err := s.Repository.GetPlan(ctx, actor.OrganizationID, projectID, planID)
	if err != nil {
		return StartBrowserRpaExecutionResult{}, err
	}
	if plan.Version != request.ExpectedVersion {
		return StartBrowserRpaExecutionResult{}, ErrVersionConflict
	}
	version := plan.CurrentVersion
	if version.ReadOnly || !version.IsPlatformConfigurationV2() || version.PlatformConfiguration == nil || version.DeliveryIntent == nil {
		return StartBrowserRpaExecutionResult{}, ErrLegacyConfigurationUnsupported
	}
	if err := validateVersionBlocking(version); err != nil {
		return StartBrowserRpaExecutionResult{}, err
	}
	configuration := version.PlatformConfiguration.Payload.OceanEngine
	if configuration == nil || configuration.Project == nil {
		return StartBrowserRpaExecutionResult{}, ErrInvalidState
	}
	stableAccountID := strings.TrimSpace(configuration.Project.AccountReference.ID)
	externalAccountID, err := s.ExternalAccountIDs.ResolveExternalAccountID(ctx, string(actor.OrganizationID), string(projectID), stableAccountID)
	if err != nil || strings.TrimSpace(externalAccountID) == "" {
		return StartBrowserRpaExecutionResult{}, fmt.Errorf("%w: resolve OceanEngine account", ErrInvalidState)
	}
	checks := RunPreflight(version)
	preflightHash, err := contract.CanonicalJSONHash(struct {
		PlanID      string           `json:"plan_id"`
		PlanVersion int64            `json:"plan_version"`
		Checks      []PreflightCheck `json:"checks"`
	}{plan.ID, plan.Version, checks})
	if err != nil {
		return StartBrowserRpaExecutionResult{}, err
	}
	projectBudgetMode := effectiveOceanEngineBudgetMode(configuration.Project.BudgetAndBidding)
	projectBudgetLimitMinor := configuration.Project.BudgetAndBidding.DailyBudgetMinor
	promotionBudgetLimitMinor := int64(0)
	for _, promotion := range configuration.Promotions {
		if promotion.BudgetAndBidding != nil {
			promotionBudgetLimitMinor += promotion.BudgetAndBidding.DailyBudgetMinor
		}
	}
	action := ControlledActionCreateProjectAndPromotions
	workflowHash, err := planExecutionWorkflowHash(executionDriver, version.CanonicalHash, version.PlatformConfiguration.CanonicalHash, preflightHash, request.IdempotencyKey)
	if err != nil {
		return StartBrowserRpaExecutionResult{}, err
	}
	fingerprint, err := planExecutionObjectFingerprint(executionDriver, externalAccountID, action, version.PlatformConfiguration.CanonicalHash)
	if err != nil {
		return StartBrowserRpaExecutionResult{}, err
	}
	skill, err := platformskills.Get(platformskills.OceanEngineEcommerceManualID, platformskills.OceanEngineEcommerceManualVersion)
	if err != nil {
		return StartBrowserRpaExecutionResult{}, err
	}
	binding := ControlledAuthorityBinding{
		AuthorityOrigin: "plan_execution", PreflightCanonicalHash: preflightHash,
		PlanID: plan.ID, PlanVersion: int(plan.Version), PlanCanonicalHash: version.CanonicalHash,
		IntentID: version.DeliveryIntent.IntentID, IntentVersion: version.DeliveryIntent.VersionNumber, IntentCanonicalHash: version.DeliveryIntent.CanonicalHash,
		ConfigurationID: version.PlatformConfiguration.ConfigurationID, ConfigurationVersion: version.PlatformConfiguration.VersionNumber, ConfigurationCanonicalHash: version.PlatformConfiguration.CanonicalHash,
		WorkflowID: workflowIDForExecutionDriver(executionDriver, plan.ID), WorkflowCanonicalHash: workflowHash, ExecutionDriver: executionDriver,
		AccountReferenceID: externalAccountID, ProjectBudgetMode: projectBudgetMode, ProjectBudgetLimitMinor: projectBudgetLimitMinor,
		PromotionBudgetLimitMinor: promotionBudgetLimitMinor, ObjectFingerprint: fingerprint, SkillID: skill.ID, SkillVersion: skill.Version,
	}
	repo, ok := s.Repository.(controlledAuthorityRepository)
	if !ok {
		return StartBrowserRpaExecutionResult{}, ErrUnsupportedConfigurationWorkflow
	}
	now := s.now()
	var change ControlledChangeSet
	if fingerprintRepo, supported := s.Repository.(controlledChangeSetByObjectFingerprintRepository); supported {
		change, err = fingerprintRepo.GetControlledChangeSetByObjectFingerprint(ctx, actor.OrganizationID, projectID, fingerprint)
		if err == nil && !samePlanExecutionTarget(change, binding, action) {
			return StartBrowserRpaExecutionResult{}, ErrInvalidState
		}
		if err != nil && !errors.Is(err, ErrNotFound) {
			return StartBrowserRpaExecutionResult{}, err
		}
	}
	if change.ID == "" {
		changeID, idErr := s.idGenerator()("controlledchangeset")
		if idErr != nil {
			return StartBrowserRpaExecutionResult{}, idErr
		}
		change = ControlledChangeSet{SchemaVersion: ControlledChangeSetSchemaV1, ID: changeID, OrganizationID: actor.OrganizationID, ProjectID: projectID, Binding: binding, Action: action, BudgetLimitMinor: projectBudgetLimitMinor, Currency: "CNY", Status: ControlledChangeSetReady, Version: 1, CreatedBy: actor.Principal.ID, CreatedAt: now, UpdatedAt: now}
		change.CanonicalHash, err = change.ComputeCanonicalHash()
		if err != nil || change.Validate() != nil {
			return StartBrowserRpaExecutionResult{}, ErrInvalidRequest
		}
		change, _, err = repo.CreateControlledChangeSet(ctx, change)
		if err != nil {
			return StartBrowserRpaExecutionResult{}, err
		}
	}
	if change.Status == ControlledChangeSetReady {
		change, _, err = s.ApproveControlledChangeSet(ctx, actor, projectID, change.ID, ApproveControlledChangeSetRequest{ExpectedVersion: change.Version})
		if err != nil {
			return StartBrowserRpaExecutionResult{}, err
		}
	}
	var execution ControlledExecution
	if executionRepo, supported := s.Repository.(controlledExecutionByChangeSetRepository); supported {
		execution, err = executionRepo.GetControlledExecutionByChangeSet(ctx, actor.OrganizationID, projectID, change.ID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return StartBrowserRpaExecutionResult{}, err
		}
		if err == nil && execution.ID != "" {
			if execution.BrowserRpaRunID != "" {
				if retryRepo, supported := s.Repository.(safeFailedBrowserRpaRetryRepository); supported {
					retried, retryErr := retryRepo.ResetSafeFailedBrowserRpaExecution(ctx, actor.OrganizationID, projectID, execution.ID, execution.Version, execution.BrowserRpaRunID)
					if retryErr == nil {
						execution = retried
					} else if !errors.Is(retryErr, ErrInvalidState) {
						return StartBrowserRpaExecutionResult{}, retryErr
					}
				}
			}
			if result, replayed, replayErr := replayExistingBrowserRpaExecution(change, execution); replayErr != nil {
				return StartBrowserRpaExecutionResult{}, replayErr
			} else if replayed && execution.BrowserRpaRunID != "" {
				if reconciler, supported := s.BrowserRpaLauncher.(browserRpaRunReconciler); supported {
					reconcileRequest := BrowserRpaLaunchRequest{OrganizationID: actor.OrganizationID, ProjectID: projectID, AccountID: externalAccountID, ExecutionDriver: executionDriverForBinding(change.Binding), BusinessExecutionID: execution.ID, Action: action, IdempotencyKey: request.IdempotencyKey, CreatedBy: actor.Principal.ID}
					if reconcileErr := reconciler.ReconcileBrowserRpaRun(ctx, reconcileRequest, execution.BrowserRpaRunID); reconcileErr != nil {
						return StartBrowserRpaExecutionResult{}, reconcileErr
					}
				}
				return result, nil
			} else if replayed && execution.BrowserRpaRunID == "" {
				return result, nil
			}
		}
	}
	if execution.ID == "" {
		execution, err = s.CreateControlledExecution(ctx, actor, projectID, change.ID)
		if err != nil {
			return StartBrowserRpaExecutionResult{}, err
		}
	}
	run, err := s.BrowserRpaLauncher.LaunchBrowserRpaRun(ctx, BrowserRpaLaunchRequest{OrganizationID: actor.OrganizationID, ProjectID: projectID, AccountID: externalAccountID, ExecutionDriver: executionDriverForBinding(change.Binding), BusinessExecutionID: execution.ID, Action: action, IdempotencyKey: request.IdempotencyKey, CreatedBy: actor.Principal.ID})
	if err != nil {
		return StartBrowserRpaExecutionResult{}, err
	}
	return StartBrowserRpaExecutionResult{ControlledChangeSet: change, ControlledExecution: execution, BrowserRpaRun: run}, nil
}

func planExecutionWorkflowHash(driver browserautomation.ExecutionDriver, planHash, configurationHash, preflightHash, idempotencyKey string) (string, error) {
	return contract.CanonicalJSONHash(struct {
		Driver            string `json:"driver"`
		PlanCanonicalHash string `json:"plan_canonical_hash"`
		ConfigurationHash string `json:"configuration_hash"`
		PreflightHash     string `json:"preflight_hash"`
		IdempotencyKey    string `json:"idempotency_key"`
	}{string(driver), planHash, configurationHash, preflightHash, idempotencyKey})
}

func planExecutionObjectFingerprint(driver browserautomation.ExecutionDriver, accountID string, action ControlledAction, configurationHash string) (string, error) {
	return contract.CanonicalJSONHash(struct {
		AccountID         string           `json:"account_id"`
		Action            ControlledAction `json:"action"`
		ConfigurationHash string           `json:"configuration_hash"`
		ExecutionDriver   string           `json:"execution_driver"`
	}{accountID, action, configurationHash, string(driver)})
}

func executionDriverForBinding(binding ControlledAuthorityBinding) browserautomation.ExecutionDriver {
	if binding.ExecutionDriver != "" {
		return binding.ExecutionDriver
	}
	if strings.HasPrefix(binding.WorkflowID, "web-api-v1-") {
		return browserautomation.ExecutionDriverOceanEngineWebAPI
	}
	return browserautomation.ExecutionDriverPlaywrightEdgeV3
}

func normalizePlanExecutionDriver(driver browserautomation.ExecutionDriver) (browserautomation.ExecutionDriver, error) {
	if driver == "" {
		return browserautomation.ExecutionDriverOceanEngineWebAPI, nil
	}
	if driver != browserautomation.ExecutionDriverOceanEngineWebAPI && driver != browserautomation.ExecutionDriverPlaywrightEdgeV3 {
		return "", ErrInvalidRequest
	}
	return driver, nil
}

func workflowIDForExecutionDriver(driver browserautomation.ExecutionDriver, planID string) string {
	if driver == browserautomation.ExecutionDriverPlaywrightEdgeV3 {
		return "playwright-v3-" + planID
	}
	return "web-api-v1-" + planID
}

func replayExistingBrowserRpaExecution(change ControlledChangeSet, execution ControlledExecution) (StartBrowserRpaExecutionResult, bool, error) {
	if execution.ID == "" {
		return StartBrowserRpaExecutionResult{}, false, nil
	}
	if execution.ControlledChangeSetID != change.ID {
		return StartBrowserRpaExecutionResult{}, false, ErrInvalidState
	}
	if execution.BrowserRpaRunID != "" {
		return StartBrowserRpaExecutionResult{ControlledChangeSet: change, ControlledExecution: execution, BrowserRpaRun: BrowserRpaLaunchResult{RunID: execution.BrowserRpaRunID}}, true, nil
	}
	if execution.Status != "pending" {
		return StartBrowserRpaExecutionResult{}, false, ErrInvalidState
	}
	return StartBrowserRpaExecutionResult{}, false, nil
}

func samePlanExecutionTarget(change ControlledChangeSet, binding ControlledAuthorityBinding, action ControlledAction) bool {
	return change.Action == action &&
		change.Status != ControlledChangeSetRejected &&
		change.Status != ControlledChangeSetInvalidated &&
		change.Binding.PlanID == binding.PlanID &&
		change.Binding.ConfigurationCanonicalHash == binding.ConfigurationCanonicalHash &&
		change.Binding.AccountReferenceID == binding.AccountReferenceID &&
		executionDriverForBinding(change.Binding) == executionDriverForBinding(binding) &&
		change.Binding.ObjectFingerprint == binding.ObjectFingerprint
}
