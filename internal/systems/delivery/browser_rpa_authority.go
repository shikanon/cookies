package delivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/platform/contract"
)

const browserRpaRemoteWriteStepID = "submit-platform-configuration"

type browserRpaAuthorityRepository interface {
	GetControlledExecution(context.Context, contract.OrganizationID, contract.ProjectID, string) (ControlledExecution, error)
	GetControlledChangeSet(context.Context, contract.OrganizationID, contract.ProjectID, string) (ControlledChangeSet, error)
	GetRemoteWriteApproval(context.Context, contract.OrganizationID, contract.ProjectID, string) (RemoteWriteApproval, error)
	AttachBrowserRpaRun(context.Context, contract.OrganizationID, contract.ProjectID, string, int64, string, time.Time) (ControlledExecution, error)
}

type browserRpaStagedCreateRepository interface {
	GetPlanVersion(context.Context, contract.OrganizationID, contract.ProjectID, string, int) (DeliveryPlanVersion, error)
	CreatePlatformEntityMapping(context.Context, PlatformEntityMapping) (PlatformEntityMapping, error)
	GetPlatformEntityMappingByInternalObject(context.Context, contract.OrganizationID, contract.ProjectID, string, string, string) (PlatformEntityMapping, error)
	ConfirmPlatformEntityMapping(context.Context, contract.OrganizationID, contract.ProjectID, string, int64, string, string) (PlatformEntityMapping, error)
}

type rebindPendingPlatformEntityMappingRequest struct {
	OrganizationID      contract.OrganizationID
	ProjectID           contract.ProjectID
	MappingID           string
	ExpectedVersion     int64
	ConfigurationID     string
	BusinessExecutionID string
	BrowserRpaRunID     string
	Now                 time.Time
}

type browserRpaStagedMappingRecoveryRepository interface {
	RebindSafePendingPlatformEntityMapping(context.Context, rebindPendingPlatformEntityMappingRequest) (PlatformEntityMapping, error)
}

// BrowserRpaAuthorityProvider projects the immutable Delivery authority into
// the shared Computer Use control plane. The browser client supplies only the
// business execution ID; it cannot construct or widen the authority binding.
type BrowserRpaAuthorityProvider struct {
	Repository browserRpaAuthorityRepository
}

func (p BrowserRpaAuthorityProvider) ResolveAuthority(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, executionID string, now time.Time) (browserautomation.AuthorityResolution, error) {
	execution, change, approval, err := p.load(ctx, organizationID, projectID, executionID, now)
	if err != nil {
		return browserautomation.AuthorityResolution{}, err
	}
	if execution.Status != "pending" && execution.Status != "running" {
		return browserautomation.AuthorityResolution{}, browserautomation.ErrInvalidContract
	}
	authority, err := p.authorityFromLoaded(execution, change, approval)
	if err != nil {
		return browserautomation.AuthorityResolution{}, err
	}
	return browserautomation.AuthorityResolution{Binding: authority, BoundRunID: execution.BrowserRpaRunID}, nil
}

func (p BrowserRpaAuthorityProvider) BindRun(ctx context.Context, authority browserautomation.AuthorityBinding, runID string, now time.Time) error {
	execution, err := p.Repository.GetControlledExecution(ctx, authority.OrganizationID, authority.ProjectID, authority.BusinessExecutionID)
	if err != nil {
		return mapBrowserRpaAuthorityError(err)
	}
	if execution.BrowserRpaRunID == runID && execution.Status == "running" {
		return p.initializeStagedMappings(ctx, authority, runID, now)
	}
	if execution.BrowserRpaRunID != "" || execution.Status != "pending" {
		return browserautomation.ErrInvalidContract
	}
	_, err = p.Repository.AttachBrowserRpaRun(ctx, authority.OrganizationID, authority.ProjectID, execution.ID, execution.Version, runID, now)
	if err != nil {
		return mapBrowserRpaAuthorityError(err)
	}
	return p.initializeStagedMappings(ctx, authority, runID, now)
}

func (p BrowserRpaAuthorityProvider) initializeStagedMappings(ctx context.Context, authority browserautomation.AuthorityBinding, runID string, now time.Time) error {
	if authority.Action != string(ControlledActionCreateProjectAndPromotions) && authority.Action != string(ControlledActionCreatePromotionsInExistingProject) {
		return nil
	}
	repo, ok := p.Repository.(browserRpaStagedCreateRepository)
	if !ok {
		// Keep in-memory authority fixtures compatible. Production repositories
		// implement the staged mapping interface.
		return nil
	}
	version, err := repo.GetPlanVersion(ctx, authority.OrganizationID, authority.ProjectID, authority.PlanID, authority.PlanVersion)
	if errors.Is(err, ErrNotFound) {
		// Historical authority fixtures can predate immutable plan storage. The
		// Runner compiler still fails closed if such a run requests automation.
		return nil
	}
	if err != nil || version.PlatformConfiguration == nil || version.PlatformConfiguration.Payload.OceanEngine == nil || version.PlatformConfiguration.Payload.OceanEngine.Project == nil {
		return browserautomation.ErrInvalidContract
	}
	ocean := version.PlatformConfiguration.Payload.OceanEngine
	type target struct{ kind, id string }
	targets := make([]target, 0, len(ocean.Promotions)+1)
	if authority.Action == string(ControlledActionCreateProjectAndPromotions) {
		targets = append(targets, target{"project", ocean.Project.ProjectDraftID})
	}
	for _, promotion := range ocean.Promotions {
		targets = append(targets, target{"promotion", promotion.PromotionDraftID})
	}
	for _, item := range targets {
		existing, getErr := repo.GetPlatformEntityMappingByInternalObject(ctx, authority.OrganizationID, authority.ProjectID, authority.AccountReferenceID, item.kind, item.id)
		if getErr == nil {
			currentConfiguration := existing.PlanID == authority.PlanID && existing.ConfigurationID == version.PlatformConfiguration.ConfigurationID
			if existing.Status == PlatformEntityMappingConfirmed {
				if !currentConfiguration {
					return browserautomation.ErrInvalidContract
				}
				continue
			}
			if existing.Status == PlatformEntityMappingPending && currentConfiguration && existing.BusinessExecutionID == authority.BusinessExecutionID && existing.BrowserRpaRunID == runID {
				continue
			}
			if existing.Status == PlatformEntityMappingPending {
				recovery, supported := p.Repository.(browserRpaStagedMappingRecoveryRepository)
				if !supported {
					return browserautomation.ErrInvalidContract
				}
				rebound, recoveryErr := recovery.RebindSafePendingPlatformEntityMapping(ctx, rebindPendingPlatformEntityMappingRequest{
					OrganizationID: authority.OrganizationID, ProjectID: authority.ProjectID,
					MappingID: existing.ID, ExpectedVersion: existing.Version,
					ConfigurationID:     version.PlatformConfiguration.ConfigurationID,
					BusinessExecutionID: authority.BusinessExecutionID, BrowserRpaRunID: runID, Now: now,
				})
				if recoveryErr != nil {
					return mapBrowserRpaAuthorityError(recoveryErr)
				}
				if rebound.BusinessExecutionID == authority.BusinessExecutionID && rebound.BrowserRpaRunID == runID {
					continue
				}
			}
			return browserautomation.ErrInvalidContract
		}
		if !errors.Is(getErr, ErrNotFound) {
			return mapBrowserRpaAuthorityError(getErr)
		}
		mapping := PlatformEntityMapping{
			SchemaVersion: PlatformEntityMappingV1, ID: stagedMappingID(authority.BusinessExecutionID, item.kind, item.id),
			OrganizationID: authority.OrganizationID, ProjectID: authority.ProjectID, AccountReferenceID: authority.AccountReferenceID,
			PlanID: authority.PlanID, ConfigurationID: version.PlatformConfiguration.ConfigurationID,
			BusinessExecutionID: authority.BusinessExecutionID, BrowserRpaRunID: runID,
			InternalObjectKind: item.kind, InternalObjectID: item.id, PlatformObjectKind: item.kind,
			Status: PlatformEntityMappingPending, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		if _, createErr := repo.CreatePlatformEntityMapping(ctx, mapping); createErr != nil {
			return mapBrowserRpaAuthorityError(createErr)
		}
	}
	return nil
}

func stagedMappingID(executionID, kind, internalID string) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{executionID, kind, internalID}, "\x00")))
	return "platformmapping_" + hex.EncodeToString(digest[:16])
}

func (p BrowserRpaAuthorityProvider) RecordCreatedObject(ctx context.Context, authority browserautomation.AuthorityBinding, runID string, page browserautomation.PreparedPage, resultEvidenceID, listEvidenceID string, _ time.Time) (bool, error) {
	repo, ok := p.Repository.(browserRpaStagedCreateRepository)
	if !ok || page.InternalObjectID == "" || (page.InternalObjectKind != "project" && page.InternalObjectKind != "promotion") || !numericPlatformID(page.Readback["platform_object_id"]) || page.Readback["reconciliation"] != "matched" || page.Readback["field_reconciliation_status"] == "not_checked" {
		return false, browserautomation.ErrInvalidContract
	}
	mapping, err := repo.GetPlatformEntityMappingByInternalObject(ctx, authority.OrganizationID, authority.ProjectID, authority.AccountReferenceID, page.InternalObjectKind, page.InternalObjectID)
	if err != nil || mapping.BusinessExecutionID != authority.BusinessExecutionID || mapping.BrowserRpaRunID != runID {
		return false, browserautomation.ErrInvalidContract
	}
	if _, err = repo.ConfirmPlatformEntityMapping(ctx, authority.OrganizationID, authority.ProjectID, mapping.ID, mapping.Version, resultEvidenceID, listEvidenceID); err != nil {
		return false, mapBrowserRpaAuthorityError(err)
	}
	execution, err := p.Repository.GetControlledExecution(ctx, authority.OrganizationID, authority.ProjectID, authority.BusinessExecutionID)
	if err != nil {
		return false, mapBrowserRpaAuthorityError(err)
	}
	switch execution.Status {
	case "running":
		return false, nil
	case "succeeded":
		return true, nil
	default:
		return false, fmt.Errorf("%w: staged execution status %s", browserautomation.ErrInvalidContract, execution.Status)
	}
}

func numericPlatformID(value string) bool {
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

func (p BrowserRpaAuthorityProvider) VerifyAuthority(ctx context.Context, authority browserautomation.AuthorityBinding, runID string, now time.Time) error {
	execution, change, approval, err := p.load(ctx, authority.OrganizationID, authority.ProjectID, authority.BusinessExecutionID, now)
	if err != nil {
		return err
	}
	if execution.BrowserRpaRunID != runID || execution.Status != "running" {
		return browserautomation.ErrInvalidContract
	}
	expected, err := p.authorityFromLoaded(execution, change, approval)
	if err != nil || !reflect.DeepEqual(expected, authority) {
		return browserautomation.ErrInvalidContract
	}
	return nil
}

func (p BrowserRpaAuthorityProvider) load(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, executionID string, now time.Time) (ControlledExecution, ControlledChangeSet, RemoteWriteApproval, error) {
	if p.Repository == nil || organizationID == "" || projectID == "" || executionID == "" {
		return ControlledExecution{}, ControlledChangeSet{}, RemoteWriteApproval{}, browserautomation.ErrInvalidContract
	}
	execution, err := p.Repository.GetControlledExecution(ctx, organizationID, projectID, executionID)
	if err != nil {
		return ControlledExecution{}, ControlledChangeSet{}, RemoteWriteApproval{}, mapBrowserRpaAuthorityError(err)
	}
	change, err := p.Repository.GetControlledChangeSet(ctx, organizationID, projectID, execution.ControlledChangeSetID)
	if err != nil {
		return ControlledExecution{}, ControlledChangeSet{}, RemoteWriteApproval{}, mapBrowserRpaAuthorityError(err)
	}
	approval, err := p.Repository.GetRemoteWriteApproval(ctx, organizationID, projectID, change.ID)
	if err != nil {
		return ControlledExecution{}, ControlledChangeSet{}, RemoteWriteApproval{}, mapBrowserRpaAuthorityError(err)
	}
	if change.Status != ControlledChangeSetExecuting || execution.RemoteWriteApprovalID != approval.ID || approval.ControlledChangeSetID != change.ID || approval.ControlledChangeSetHash != change.CanonicalHash || !reflect.DeepEqual(approval.Binding, change.Binding) {
		return ControlledExecution{}, ControlledChangeSet{}, RemoteWriteApproval{}, browserautomation.ErrInvalidContract
	}
	if err := change.Validate(); err != nil {
		return ControlledExecution{}, ControlledChangeSet{}, RemoteWriteApproval{}, browserautomation.ErrInvalidContract
	}
	approvalValidationTime := now
	if change.Binding.AuthorityOrigin == "plan_execution" &&
		(change.Action == ControlledActionCreateProjectAndPromotions || change.Action == ControlledActionCreatePromotionsInExistingProject) {
		// A safe Prepare retry can occur after the server-created approval TTL.
		// The immutable plan still binds the action. Submit also requires a new
		// five-minute final confirmation, so this does not extend click authority.
		approvalValidationTime = approval.ApprovedAt
	}
	if err := approval.Validate(approvalValidationTime); err != nil {
		return ControlledExecution{}, ControlledChangeSet{}, RemoteWriteApproval{}, browserautomation.ErrInvalidContract
	}
	return execution, change, approval, nil
}

func (p BrowserRpaAuthorityProvider) authorityFromLoaded(execution ControlledExecution, change ControlledChangeSet, approval RemoteWriteApproval) (browserautomation.AuthorityBinding, error) {
	binding := approval.Binding
	value := browserautomation.AuthorityBinding{SchemaVersion: browserautomation.AuthoritySchemaV1, AuthorityOrigin: binding.AuthorityOrigin, PreflightCanonicalHash: binding.PreflightCanonicalHash, OrganizationID: execution.OrganizationID, ProjectID: execution.ProjectID, BusinessExecutionID: execution.ID, ChangeSetID: change.ID, ApprovalID: approval.ID, ApprovalActionHash: approval.ActionHash, AccountReferenceID: binding.AccountReferenceID, ParentPlatformProjectID: binding.ParentPlatformProjectID, TargetMappingID: binding.TargetMappingID, TargetMappingVersion: binding.TargetMappingVersion, TargetPlatformObjectID: binding.TargetPlatformObjectID, TargetPlatformObjectKind: binding.TargetPlatformObjectKind, OperatorPrincipalID: binding.OperatorPrincipalID, SupersedesControlledChangeSetID: binding.SupersedesControlledChangeSetID, ObjectFingerprint: binding.ObjectFingerprint, Action: string(approval.Action), PlanID: binding.PlanID, PlanVersion: binding.PlanVersion, ProjectBudgetMode: binding.ProjectBudgetMode, ProjectBudgetLimitMinor: binding.ProjectBudgetLimitMinor, PromotionBudgetLimitMinor: binding.PromotionBudgetLimitMinor, BudgetLimitMinor: approval.BudgetLimitMinor, Currency: approval.Currency, PlanCanonicalHash: binding.PlanCanonicalHash, IntentCanonicalHash: binding.IntentCanonicalHash, FeedbackCanonicalHash: binding.OperatorFeedbackCanonicalHash, DecisionCanonicalHash: binding.DecisionCanonicalHash, ConfigurationCanonicalHash: binding.ConfigurationCanonicalHash, WorkflowID: binding.WorkflowID, WorkflowCanonicalHash: binding.WorkflowCanonicalHash, WorkflowStepID: browserRpaRemoteWriteStepID, SkillID: binding.SkillID, SkillVersion: binding.SkillVersion, ExecutionDriver: executionDriverForBinding(binding)}
	if binding.PromotionMutation != nil {
		value.PromotionMutation = toBrowserRpaPromotionMutation(*binding.PromotionMutation)
	}
	if binding.PromotionControl != nil {
		value.PromotionControl = &browserautomation.PromotionControlBinding{CurrentDailyBudgetMinor: binding.PromotionControl.CurrentDailyBudgetMinor, CurrentPlatformStatus: binding.PromotionControl.CurrentPlatformStatus, TargetPlatformStatus: binding.PromotionControl.TargetPlatformStatus, CurrentStateHash: binding.PromotionControl.CurrentStateHash, TargetStateHash: binding.PromotionControl.TargetStateHash}
	}
	if binding.PromotionRestart != nil {
		value.PromotionRestart = toBrowserRpaPromotionRestart(*binding.PromotionRestart)
	}
	return value, value.Validate()
}

func toBrowserRpaPromotionRestart(value ControlledPromotionRestart) *browserautomation.PromotionRestartBinding {
	converted := &browserautomation.PromotionRestartBinding{
		CurrentDailyBudgetMinor:  value.CurrentDailyBudgetMinor,
		ApprovedDailyBudgetMinor: value.ApprovedDailyBudgetMinor,
		CurrentPlatformStatus:    value.CurrentPlatformStatus,
		TargetPlatformStatus:     value.TargetPlatformStatus,
		Schedule:                 browserautomation.PromotionScheduleWindow{StartAt: value.Schedule.StartAt, EndAt: value.Schedule.EndAt, Timezone: value.Schedule.Timezone},
		LandingPage:              browserautomation.PromotionLandingPageReference{ReferenceID: value.LandingPage.ReferenceID, AuthorizationEvidenceID: value.LandingPage.AuthorizationEvidenceID},
		CurrentStateHash:         value.CurrentStateHash,
		TargetStateHash:          value.TargetStateHash,
	}
	for _, reference := range value.Materials {
		converted.Materials = append(converted.Materials, browserautomation.PromotionMaterialReference{ReferenceID: reference.ReferenceID, AuthorizationEvidenceID: reference.AuthorizationEvidenceID})
	}
	return converted
}

func toBrowserRpaPromotionMutation(value ControlledPromotionMutation) *browserautomation.PromotionMutationBinding {
	converted := &browserautomation.PromotionMutationBinding{CurrentDailyBudgetMinor: value.CurrentDailyBudgetMinor, TargetDailyBudgetMinor: value.TargetDailyBudgetMinor, CurrentStateHash: value.CurrentStateHash, TargetStateHash: value.TargetStateHash}
	for _, reference := range value.CurrentMaterials {
		converted.CurrentMaterials = append(converted.CurrentMaterials, browserautomation.PromotionMaterialReference{ReferenceID: reference.ReferenceID, AuthorizationEvidenceID: reference.AuthorizationEvidenceID})
	}
	for _, reference := range value.TargetMaterials {
		converted.TargetMaterials = append(converted.TargetMaterials, browserautomation.PromotionMaterialReference{ReferenceID: reference.ReferenceID, AuthorizationEvidenceID: reference.AuthorizationEvidenceID})
	}
	return converted
}

func mapBrowserRpaAuthorityError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) {
		return browserautomation.ErrNotFound
	}
	if errors.Is(err, ErrVersionConflict) {
		return browserautomation.ErrVersionConflict
	}
	return err
}
