package delivery

import (
	"context"
	"errors"
	"strings"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/platform/contract"
)

func (s Service) StartObjectExecution(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, changeID string, request StartBrowserRpaExecutionRequest) (StartBrowserRpaExecutionResult, error) {
	if err := s.ready(actor, projectID, ScopeExecute); err != nil {
		return StartBrowserRpaExecutionResult{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return StartBrowserRpaExecutionResult{}, err
	}
	if s.BrowserRpaLauncher == nil || request.ExpectedVersion < 1 || strings.TrimSpace(request.IdempotencyKey) == "" || len(request.IdempotencyKey) > 160 {
		return StartBrowserRpaExecutionResult{}, ErrInvalidRequest
	}
	repo, ok := s.Repository.(controlledAuthorityRepository)
	if !ok {
		return StartBrowserRpaExecutionResult{}, ErrUnsupportedConfigurationWorkflow
	}
	change, err := repo.GetControlledChangeSet(ctx, actor.OrganizationID, projectID, changeID)
	if err != nil {
		return StartBrowserRpaExecutionResult{}, err
	}
	if change.Action != ControlledActionUpdatePromotionBudget || change.CreatedBy != actor.Principal.ID || change.Binding.OperatorPrincipalID != actor.Principal.ID || change.Binding.ExecutionDriver != browserautomation.ExecutionDriverPlaywrightEdgeV3 {
		return StartBrowserRpaExecutionResult{}, ErrApprovalContentMismatch
	}
	if change.Status == ControlledChangeSetReady {
		if change.Version != request.ExpectedVersion {
			return StartBrowserRpaExecutionResult{}, ErrVersionConflict
		}
		change, _, err = s.ApproveControlledChangeSet(ctx, actor, projectID, change.ID, ApproveControlledChangeSetRequest{ExpectedVersion: request.ExpectedVersion})
		if err != nil {
			return StartBrowserRpaExecutionResult{}, err
		}
	}
	var execution ControlledExecution
	if existing, supported := s.Repository.(controlledExecutionByChangeSetRepository); supported {
		execution, err = existing.GetControlledExecutionByChangeSet(ctx, actor.OrganizationID, projectID, change.ID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return StartBrowserRpaExecutionResult{}, err
		}
		if execution.BrowserRpaRunID != "" {
			return StartBrowserRpaExecutionResult{ControlledChangeSet: change, ControlledExecution: execution, BrowserRpaRun: BrowserRpaLaunchResult{RunID: execution.BrowserRpaRunID}}, nil
		}
	}
	if execution.ID == "" {
		execution, err = s.CreateControlledExecution(ctx, actor, projectID, change.ID)
		if err != nil {
			return StartBrowserRpaExecutionResult{}, err
		}
	}
	run, err := s.BrowserRpaLauncher.LaunchBrowserRpaRun(ctx, BrowserRpaLaunchRequest{OrganizationID: actor.OrganizationID, ProjectID: projectID, AccountID: change.Binding.AccountReferenceID, ExecutionDriver: browserautomation.ExecutionDriverPlaywrightEdgeV3, BusinessExecutionID: execution.ID, Action: change.Action, ParentProjectID: change.Binding.ParentPlatformProjectID, IdempotencyKey: request.IdempotencyKey, CreatedBy: actor.Principal.ID})
	if err != nil {
		return StartBrowserRpaExecutionResult{}, err
	}
	return StartBrowserRpaExecutionResult{ControlledChangeSet: change, ControlledExecution: execution, BrowserRpaRun: run}, nil
}
