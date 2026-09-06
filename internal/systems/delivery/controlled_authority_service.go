package delivery

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/systems/delivery/platformskills"
)

type controlledAuthorityRepository interface {
	CreateControlledChangeSet(context.Context, ControlledChangeSet) (ControlledChangeSet, bool, error)
	GetControlledChangeSet(context.Context, contract.OrganizationID, contract.ProjectID, string) (ControlledChangeSet, error)
	ApproveControlledChangeSet(context.Context, ControlledChangeSet, RemoteWriteApproval) (ControlledChangeSet, RemoteWriteApproval, error)
	CreateControlledExecution(context.Context, ControlledExecution) (ControlledExecution, error)
	GetControlledExecution(context.Context, contract.OrganizationID, contract.ProjectID, string) (ControlledExecution, error)
	AttachBrowserRpaRun(context.Context, contract.OrganizationID, contract.ProjectID, string, int64, string, time.Time) (ControlledExecution, error)
	CreatePlatformEntityMapping(context.Context, PlatformEntityMapping) (PlatformEntityMapping, error)
	GetPlatformEntityMapping(context.Context, contract.OrganizationID, contract.ProjectID, string) (PlatformEntityMapping, error)
	ListPlatformEntityMappings(context.Context, contract.OrganizationID, contract.ProjectID, string) ([]PlatformEntityMapping, error)
	ConfirmPlatformEntityMapping(context.Context, contract.OrganizationID, contract.ProjectID, string, int64, string, string) (PlatformEntityMapping, error)
	ConfirmPlatformEntityMappingMutation(context.Context, contract.OrganizationID, contract.ProjectID, string, int64, string, string, string) (PlatformEntityMapping, PlatformEntityMappingRevision, error)
	ValidateControlledMaterialReferences(context.Context, contract.OrganizationID, contract.ProjectID, string, []ControlledMaterialReference) error
	ValidateControlledRestartReferences(context.Context, contract.OrganizationID, contract.ProjectID, string, []ControlledMaterialReference, ControlledLandingPageReference) error
	InvalidateCalibratedControlledChangeSet(context.Context, contract.OrganizationID, contract.ProjectID, string, int64, time.Time) (ControlledChangeSet, ControlledExecution, error)
}

type ConfirmPlatformEntityMappingRequest struct {
	ExpectedVersion  int64  `json:"expected_version"`
	ResultEvidenceID string `json:"result_evidence_id"`
	ListEvidenceID   string `json:"list_evidence_id"`
}

type ConfirmPlatformEntityMappingMutationRequest struct {
	ExpectedVersion     int64  `json:"expected_version"`
	BusinessExecutionID string `json:"business_execution_id"`
	ResultEvidenceID    string `json:"result_evidence_id"`
	ListEvidenceID      string `json:"list_evidence_id"`
}

type ConfirmPlatformEntityMappingChangeRequest = ConfirmPlatformEntityMappingMutationRequest

func (s Service) AttachBrowserRpaRun(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, executionID string, expectedVersion int64, runID string) (ControlledExecution, error) {
	if err := s.ready(actor, projectID, ScopeExecute); err != nil {
		return ControlledExecution{}, err
	}
	if expectedVersion < 1 || strings.TrimSpace(runID) == "" {
		return ControlledExecution{}, ErrInvalidRequest
	}
	repo, ok := s.Repository.(controlledAuthorityRepository)
	if !ok {
		return ControlledExecution{}, ErrUnsupportedConfigurationWorkflow
	}
	return repo.AttachBrowserRpaRun(ctx, actor.OrganizationID, projectID, executionID, expectedVersion, runID, s.now())
}

func (s Service) CreatePendingPlatformEntityMapping(ctx context.Context, actor contract.ActorContext, value PlatformEntityMapping) (PlatformEntityMapping, error) {
	if err := s.ready(actor, value.ProjectID, ScopeExecute); err != nil {
		return PlatformEntityMapping{}, err
	}
	value.OrganizationID = actor.OrganizationID
	value.SchemaVersion = PlatformEntityMappingV1
	value.Status = PlatformEntityMappingPending
	value.PlatformObjectID = ""
	value.PlatformStatus = ""
	value.ResultEvidenceID = ""
	value.ListEvidenceID = ""
	value.Version = 1
	value.CreatedAt = s.now()
	value.UpdatedAt = value.CreatedAt
	if err := value.Validate(); err != nil {
		return PlatformEntityMapping{}, err
	}
	repo, ok := s.Repository.(controlledAuthorityRepository)
	if !ok {
		return PlatformEntityMapping{}, ErrUnsupportedConfigurationWorkflow
	}
	return repo.CreatePlatformEntityMapping(ctx, value)
}

func (s Service) ConfirmPlatformEntityMappingMutation(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, id string, request ConfirmPlatformEntityMappingMutationRequest) (PlatformEntityMapping, PlatformEntityMappingRevision, error) {
	if err := s.ready(actor, projectID, ScopeExecute); err != nil {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, err
	}
	if request.ExpectedVersion < 2 || strings.TrimSpace(request.BusinessExecutionID) == "" || strings.TrimSpace(request.ResultEvidenceID) == "" || strings.TrimSpace(request.ListEvidenceID) == "" || request.ResultEvidenceID == request.ListEvidenceID {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrApprovalContentMismatch
	}
	repo, ok := s.Repository.(controlledAuthorityRepository)
	if !ok {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrUnsupportedConfigurationWorkflow
	}
	return repo.ConfirmPlatformEntityMappingMutation(ctx, actor.OrganizationID, projectID, id, request.ExpectedVersion, request.BusinessExecutionID, request.ResultEvidenceID, request.ListEvidenceID)
}

func (s Service) ConfirmPlatformEntityMappingChange(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, id string, request ConfirmPlatformEntityMappingChangeRequest) (PlatformEntityMapping, PlatformEntityMappingRevision, error) {
	return s.ConfirmPlatformEntityMappingMutation(ctx, actor, projectID, id, request)
}

func (s Service) ConfirmPlatformEntityMapping(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, id string, request ConfirmPlatformEntityMappingRequest) (PlatformEntityMapping, error) {
	if err := s.ready(actor, projectID, ScopeExecute); err != nil {
		return PlatformEntityMapping{}, err
	}
	if request.ExpectedVersion < 1 || strings.TrimSpace(request.ResultEvidenceID) == "" || strings.TrimSpace(request.ListEvidenceID) == "" || request.ResultEvidenceID == request.ListEvidenceID {
		return PlatformEntityMapping{}, ErrApprovalContentMismatch
	}
	repo, ok := s.Repository.(controlledAuthorityRepository)
	if !ok {
		return PlatformEntityMapping{}, ErrUnsupportedConfigurationWorkflow
	}
	return repo.ConfirmPlatformEntityMapping(ctx, actor.OrganizationID, projectID, id, request.ExpectedVersion, request.ResultEvidenceID, request.ListEvidenceID)
}

func (s Service) GetPlatformEntityMapping(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, id string) (PlatformEntityMapping, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return PlatformEntityMapping{}, err
	}
	repo, ok := s.Repository.(controlledAuthorityRepository)
	if !ok {
		return PlatformEntityMapping{}, ErrUnsupportedConfigurationWorkflow
	}
	return repo.GetPlatformEntityMapping(ctx, actor.OrganizationID, projectID, id)
}

func (s Service) ListPlatformEntityMappings(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, accountReferenceID string) ([]PlatformEntityMapping, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return nil, err
	}
	if strings.TrimSpace(accountReferenceID) == "" {
		return nil, ErrInvalidRequest
	}
	resolvedAccountID := strings.TrimSpace(accountReferenceID)
	if s.ExternalAccountIDs != nil {
		value, err := s.ExternalAccountIDs.ResolveExternalAccountID(ctx, string(actor.OrganizationID), string(projectID), resolvedAccountID)
		if err != nil || strings.TrimSpace(value) == "" {
			return nil, ErrInvalidState
		}
		resolvedAccountID = strings.TrimSpace(value)
	}
	repo, ok := s.Repository.(controlledAuthorityRepository)
	if !ok {
		return nil, ErrUnsupportedConfigurationWorkflow
	}
	return repo.ListPlatformEntityMappings(ctx, actor.OrganizationID, projectID, resolvedAccountID)
}

func (s Service) GetControlledChangeSet(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, id string) (ControlledChangeSet, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return ControlledChangeSet{}, err
	}
	repo, ok := s.Repository.(controlledAuthorityRepository)
	if !ok {
		return ControlledChangeSet{}, ErrUnsupportedConfigurationWorkflow
	}
	return repo.GetControlledChangeSet(ctx, actor.OrganizationID, projectID, id)
}
func (s Service) GetControlledExecution(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, id string) (ControlledExecution, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return ControlledExecution{}, err
	}
	repo, ok := s.Repository.(controlledAuthorityRepository)
	if !ok {
		return ControlledExecution{}, ErrUnsupportedConfigurationWorkflow
	}
	return repo.GetControlledExecution(ctx, actor.OrganizationID, projectID, id)
}

type InvalidateCalibratedControlledChangeSetRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}

func (s Service) InvalidateCalibratedControlledChangeSet(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, id string, request InvalidateCalibratedControlledChangeSetRequest) (ControlledChangeSet, ControlledExecution, error) {
	if err := s.ready(actor, projectID, ScopeApprove); err != nil {
		return ControlledChangeSet{}, ControlledExecution{}, err
	}
	if strings.TrimSpace(id) == "" || request.ExpectedVersion < 1 {
		return ControlledChangeSet{}, ControlledExecution{}, ErrInvalidRequest
	}
	repo, ok := s.Repository.(controlledAuthorityRepository)
	if !ok {
		return ControlledChangeSet{}, ControlledExecution{}, ErrUnsupportedConfigurationWorkflow
	}
	change, err := repo.GetControlledChangeSet(ctx, actor.OrganizationID, projectID, id)
	if err != nil {
		return ControlledChangeSet{}, ControlledExecution{}, err
	}
	if change.Version != request.ExpectedVersion {
		return ControlledChangeSet{}, ControlledExecution{}, ErrVersionConflict
	}
	if change.Status != ControlledChangeSetExecuting || !change.Action.ChangesExistingPromotion() || change.Binding.OperatorPrincipalID != actor.Principal.ID {
		return ControlledChangeSet{}, ControlledExecution{}, ErrInvalidState
	}
	return repo.InvalidateCalibratedControlledChangeSet(ctx, actor.OrganizationID, projectID, id, request.ExpectedVersion, s.now())
}

type CompileControlledChangeSetRequest struct {
	ObservatoryRunID        string           `json:"observatory_run_id"`
	Action                  ControlledAction `json:"action,omitempty"`
	ParentPlatformProjectID string           `json:"parent_platform_project_id,omitempty"`
}

type CompileMappedControlledChangeSetRequest struct {
	ExpectedMappingVersion          int64                         `json:"expected_mapping_version"`
	Action                          ControlledAction              `json:"action"`
	CurrentDailyBudgetMinor         int64                         `json:"current_daily_budget_minor"`
	TargetDailyBudgetMinor          int64                         `json:"target_daily_budget_minor"`
	CurrentMaterials                []ControlledMaterialReference `json:"current_materials,omitempty"`
	TargetMaterials                 []ControlledMaterialReference `json:"target_materials,omitempty"`
	SupersedesControlledChangeSetID string                        `json:"supersedes_controlled_change_set_id,omitempty"`
}

type CompileEmergencyPauseChangeSetRequest struct {
	ExpectedMappingVersion          int64  `json:"expected_mapping_version"`
	CurrentDailyBudgetMinor         int64  `json:"current_daily_budget_minor"`
	CurrentPlatformStatus           string `json:"current_platform_status"`
	SupersedesControlledChangeSetID string `json:"supersedes_controlled_change_set_id,omitempty"`
}

type CompileControlledRestartChangeSetRequest struct {
	ExpectedMappingVersion          int64                          `json:"expected_mapping_version"`
	CurrentDailyBudgetMinor         int64                          `json:"current_daily_budget_minor"`
	ApprovedDailyBudgetMinor        int64                          `json:"approved_daily_budget_minor"`
	CurrentPlatformStatus           string                         `json:"current_platform_status"`
	Schedule                        ControlledScheduleWindow       `json:"schedule"`
	Materials                       []ControlledMaterialReference  `json:"materials"`
	LandingPage                     ControlledLandingPageReference `json:"landing_page"`
	SupersedesControlledChangeSetID string                         `json:"supersedes_controlled_change_set_id,omitempty"`
}

func (r CompileMappedControlledChangeSetRequest) mutation() (ControlledPromotionMutation, error) {
	mutation := ControlledPromotionMutation{
		CurrentDailyBudgetMinor: r.CurrentDailyBudgetMinor,
		TargetDailyBudgetMinor:  r.TargetDailyBudgetMinor,
		CurrentMaterials:        r.CurrentMaterials,
		TargetMaterials:         r.TargetMaterials,
	}
	current, err := mutation.statePayload(r.Action, false)
	if err != nil {
		return ControlledPromotionMutation{}, err
	}
	target, err := mutation.statePayload(r.Action, true)
	if err != nil {
		return ControlledPromotionMutation{}, err
	}
	mutation.CurrentStateHash, err = contract.CanonicalJSONHash(current)
	if err != nil {
		return ControlledPromotionMutation{}, err
	}
	mutation.TargetStateHash, err = contract.CanonicalJSONHash(target)
	if err != nil {
		return ControlledPromotionMutation{}, err
	}
	if err := mutation.Validate(r.Action); err != nil {
		return ControlledPromotionMutation{}, err
	}
	return mutation, nil
}

func validateControlledChangeSetSupersession(ctx context.Context, repo controlledAuthorityRepository, actor contract.ActorContext, projectID contract.ProjectID, supersededID string, action ControlledAction, mapping PlatformEntityMapping, currentStateHash, targetStateHash string) error {
	supersededID = strings.TrimSpace(supersededID)
	if supersededID == "" {
		return nil
	}
	previous, err := repo.GetControlledChangeSet(ctx, actor.OrganizationID, projectID, supersededID)
	if err != nil {
		return err
	}
	previousCurrentHash, previousTargetHash, err := previous.Binding.existingPromotionStateHashes(previous.Action)
	if err != nil {
		return ErrApprovalContentMismatch
	}
	if previous.Status != ControlledChangeSetInvalidated ||
		previous.Action != action ||
		previous.CreatedBy != actor.Principal.ID ||
		previous.Binding.OperatorPrincipalID != actor.Principal.ID ||
		previous.Binding.TargetMappingID != mapping.ID ||
		previous.Binding.TargetMappingVersion != mapping.Version ||
		previous.Binding.TargetPlatformObjectID != mapping.PlatformObjectID ||
		previous.Binding.TargetPlatformObjectKind != mapping.PlatformObjectKind ||
		previous.Binding.AccountReferenceID != mapping.AccountReferenceID ||
		previousCurrentHash != currentStateHash ||
		previousTargetHash != targetStateHash {
		return ErrApprovalContentMismatch
	}
	return nil
}

func (s Service) CompileMappedControlledChangeSet(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, mappingID string, request CompileMappedControlledChangeSetRequest) (ControlledChangeSet, bool, error) {
	if err := s.ready(actor, projectID, ScopeWrite); err != nil {
		return ControlledChangeSet{}, false, err
	}
	if strings.TrimSpace(mappingID) == "" || request.ExpectedMappingVersion < 2 || !request.Action.ModifiesExistingPromotion() {
		return ControlledChangeSet{}, false, ErrInvalidRequest
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ControlledChangeSet{}, false, err
	}
	repo, ok := s.Repository.(controlledAuthorityRepository)
	if !ok {
		return ControlledChangeSet{}, false, ErrUnsupportedConfigurationWorkflow
	}
	mapping, err := repo.GetPlatformEntityMapping(ctx, actor.OrganizationID, projectID, mappingID)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	if mapping.Status != PlatformEntityMappingConfirmed || mapping.Version != request.ExpectedMappingVersion || mapping.PlatformObjectKind != "promotion" || strings.TrimSpace(mapping.PlatformObjectID) == "" {
		return ControlledChangeSet{}, false, ErrVersionConflict
	}
	sourceExecution, err := repo.GetControlledExecution(ctx, actor.OrganizationID, projectID, mapping.BusinessExecutionID)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	sourceChange, err := repo.GetControlledChangeSet(ctx, actor.OrganizationID, projectID, sourceExecution.ControlledChangeSetID)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	if sourceChange.Status != ControlledChangeSetExecuted || sourceChange.Binding.AccountReferenceID != mapping.AccountReferenceID || sourceChange.Binding.PlanID != mapping.PlanID || sourceChange.Binding.ConfigurationID != mapping.ConfigurationID || sourceChange.Binding.ParentPlatformProjectID == "" {
		return ControlledChangeSet{}, false, ErrApprovalContentMismatch
	}
	mutation, err := request.mutation()
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	if mapping.CurrentStateAction == request.Action && mapping.CurrentStateHash != mutation.CurrentStateHash {
		return ControlledChangeSet{}, false, ErrApprovalContentMismatch
	}
	if request.Action == ControlledActionUpdatePromotionMaterials {
		references := append([]ControlledMaterialReference{}, mutation.CurrentMaterials...)
		references = append(references, mutation.TargetMaterials...)
		if err := repo.ValidateControlledMaterialReferences(ctx, actor.OrganizationID, projectID, mapping.AccountReferenceID, references); err != nil {
			return ControlledChangeSet{}, false, err
		}
	}
	if err := validateControlledChangeSetSupersession(ctx, repo, actor, projectID, request.SupersedesControlledChangeSetID, request.Action, mapping, mutation.CurrentStateHash, mutation.TargetStateHash); err != nil {
		return ControlledChangeSet{}, false, err
	}
	fingerprint, err := contract.CanonicalJSONHash(struct {
		AccountReferenceID              string           `json:"account_reference_id"`
		Action                          ControlledAction `json:"action"`
		MappingID                       string           `json:"mapping_id"`
		MappingVersion                  int64            `json:"mapping_version"`
		PlatformObjectID                string           `json:"platform_object_id"`
		OperatorPrincipal               string           `json:"operator_principal_id"`
		CurrentStateHash                string           `json:"current_state_hash"`
		TargetStateHash                 string           `json:"target_state_hash"`
		SupersedesControlledChangeSetID string           `json:"supersedes_controlled_change_set_id,omitempty"`
	}{
		AccountReferenceID:              mapping.AccountReferenceID,
		Action:                          request.Action,
		MappingID:                       mapping.ID,
		MappingVersion:                  mapping.Version,
		PlatformObjectID:                mapping.PlatformObjectID,
		OperatorPrincipal:               actor.Principal.ID,
		CurrentStateHash:                mutation.CurrentStateHash,
		TargetStateHash:                 mutation.TargetStateHash,
		SupersedesControlledChangeSetID: strings.TrimSpace(request.SupersedesControlledChangeSetID),
	})
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	binding := sourceChange.Binding
	binding.TargetMappingID = mapping.ID
	binding.TargetMappingVersion = mapping.Version
	binding.TargetPlatformObjectID = mapping.PlatformObjectID
	binding.TargetPlatformObjectKind = mapping.PlatformObjectKind
	binding.OperatorPrincipalID = actor.Principal.ID
	binding.SupersedesControlledChangeSetID = strings.TrimSpace(request.SupersedesControlledChangeSetID)
	binding.PromotionBudgetLimitMinor = mutation.TargetDailyBudgetMinor
	binding.ObjectFingerprint = fingerprint
	binding.PromotionMutation = &mutation
	binding.PromotionControl = nil
	binding.PromotionRestart = nil
	id, err := s.idGenerator()("controlledchangeset")
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	now := s.now()
	change := ControlledChangeSet{SchemaVersion: ControlledChangeSetSchemaV1, ID: id, OrganizationID: actor.OrganizationID, ProjectID: projectID, Binding: binding, Action: request.Action, BudgetLimitMinor: mutation.TargetDailyBudgetMinor, Currency: "CNY", Status: ControlledChangeSetReady, Version: 1, CreatedBy: actor.Principal.ID, CreatedAt: now, UpdatedAt: now}
	change.CanonicalHash, err = change.ComputeCanonicalHash()
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	if err := change.Validate(); err != nil {
		return ControlledChangeSet{}, false, err
	}
	return repo.CreateControlledChangeSet(ctx, change)
}

func (s Service) CompileEmergencyPauseChangeSet(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, mappingID string, request CompileEmergencyPauseChangeSetRequest) (ControlledChangeSet, bool, error) {
	if err := s.ready(actor, projectID, ScopeWrite); err != nil {
		return ControlledChangeSet{}, false, err
	}
	if strings.TrimSpace(mappingID) == "" || request.ExpectedMappingVersion < 2 || request.CurrentDailyBudgetMinor < 30000 || request.CurrentPlatformStatus != "delivering" {
		return ControlledChangeSet{}, false, ErrInvalidRequest
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ControlledChangeSet{}, false, err
	}
	repo, ok := s.Repository.(controlledAuthorityRepository)
	if !ok {
		return ControlledChangeSet{}, false, ErrUnsupportedConfigurationWorkflow
	}
	mapping, err := repo.GetPlatformEntityMapping(ctx, actor.OrganizationID, projectID, mappingID)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	if mapping.Status != PlatformEntityMappingConfirmed || mapping.Version != request.ExpectedMappingVersion || mapping.PlatformObjectKind != "promotion" || strings.TrimSpace(mapping.PlatformObjectID) == "" {
		return ControlledChangeSet{}, false, ErrVersionConflict
	}
	if mapping.PlatformStatus != request.CurrentPlatformStatus {
		return ControlledChangeSet{}, false, ErrInvalidState
	}
	sourceExecution, err := repo.GetControlledExecution(ctx, actor.OrganizationID, projectID, mapping.BusinessExecutionID)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	sourceChange, err := repo.GetControlledChangeSet(ctx, actor.OrganizationID, projectID, sourceExecution.ControlledChangeSetID)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	if sourceChange.Status != ControlledChangeSetExecuted || sourceChange.Binding.AccountReferenceID != mapping.AccountReferenceID || sourceChange.Binding.PlanID != mapping.PlanID || sourceChange.Binding.ConfigurationID != mapping.ConfigurationID || sourceChange.Binding.ParentPlatformProjectID == "" {
		return ControlledChangeSet{}, false, ErrApprovalContentMismatch
	}
	control := ControlledPromotionControl{
		CurrentDailyBudgetMinor: request.CurrentDailyBudgetMinor,
		CurrentPlatformStatus:   request.CurrentPlatformStatus,
		TargetPlatformStatus:    "paused",
	}
	currentState, err := control.statePayload(false)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	targetState, err := control.statePayload(true)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	control.CurrentStateHash, err = contract.CanonicalJSONHash(currentState)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	control.TargetStateHash, err = contract.CanonicalJSONHash(targetState)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	if err := control.Validate(ControlledActionPausePromotion); err != nil {
		return ControlledChangeSet{}, false, err
	}
	if mapping.CurrentStateAction == ControlledActionPausePromotion && mapping.CurrentStateHash != control.CurrentStateHash {
		return ControlledChangeSet{}, false, ErrApprovalContentMismatch
	}
	if err := validateControlledChangeSetSupersession(ctx, repo, actor, projectID, request.SupersedesControlledChangeSetID, ControlledActionPausePromotion, mapping, control.CurrentStateHash, control.TargetStateHash); err != nil {
		return ControlledChangeSet{}, false, err
	}
	fingerprint, err := contract.CanonicalJSONHash(struct {
		AccountReferenceID              string           `json:"account_reference_id"`
		Action                          ControlledAction `json:"action"`
		MappingID                       string           `json:"mapping_id"`
		MappingVersion                  int64            `json:"mapping_version"`
		PlatformObjectID                string           `json:"platform_object_id"`
		OperatorPrincipal               string           `json:"operator_principal_id"`
		CurrentStateHash                string           `json:"current_state_hash"`
		TargetStateHash                 string           `json:"target_state_hash"`
		SupersedesControlledChangeSetID string           `json:"supersedes_controlled_change_set_id,omitempty"`
	}{
		AccountReferenceID:              mapping.AccountReferenceID,
		Action:                          ControlledActionPausePromotion,
		MappingID:                       mapping.ID,
		MappingVersion:                  mapping.Version,
		PlatformObjectID:                mapping.PlatformObjectID,
		OperatorPrincipal:               actor.Principal.ID,
		CurrentStateHash:                control.CurrentStateHash,
		TargetStateHash:                 control.TargetStateHash,
		SupersedesControlledChangeSetID: strings.TrimSpace(request.SupersedesControlledChangeSetID),
	})
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	binding := sourceChange.Binding
	binding.TargetMappingID = mapping.ID
	binding.TargetMappingVersion = mapping.Version
	binding.TargetPlatformObjectID = mapping.PlatformObjectID
	binding.TargetPlatformObjectKind = mapping.PlatformObjectKind
	binding.OperatorPrincipalID = actor.Principal.ID
	binding.SupersedesControlledChangeSetID = strings.TrimSpace(request.SupersedesControlledChangeSetID)
	binding.PromotionBudgetLimitMinor = control.CurrentDailyBudgetMinor
	binding.ObjectFingerprint = fingerprint
	binding.PromotionMutation = nil
	binding.PromotionControl = &control
	binding.PromotionRestart = nil
	id, err := s.idGenerator()("controlledchangeset")
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	now := s.now()
	change := ControlledChangeSet{SchemaVersion: ControlledChangeSetSchemaV1, ID: id, OrganizationID: actor.OrganizationID, ProjectID: projectID, Binding: binding, Action: ControlledActionPausePromotion, BudgetLimitMinor: control.CurrentDailyBudgetMinor, Currency: "CNY", Status: ControlledChangeSetReady, Version: 1, CreatedBy: actor.Principal.ID, CreatedAt: now, UpdatedAt: now}
	change.CanonicalHash, err = change.ComputeCanonicalHash()
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	if err := change.Validate(); err != nil {
		return ControlledChangeSet{}, false, err
	}
	return repo.CreateControlledChangeSet(ctx, change)
}

func (s Service) CompileControlledRestartChangeSet(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, mappingID string, request CompileControlledRestartChangeSetRequest) (ControlledChangeSet, bool, error) {
	if err := s.ready(actor, projectID, ScopeWrite); err != nil {
		return ControlledChangeSet{}, false, err
	}
	if strings.TrimSpace(mappingID) == "" || request.ExpectedMappingVersion < 2 || request.CurrentDailyBudgetMinor < 30000 || request.ApprovedDailyBudgetMinor < 30000 || request.CurrentPlatformStatus != "paused" {
		return ControlledChangeSet{}, false, ErrInvalidRequest
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ControlledChangeSet{}, false, err
	}
	repo, ok := s.Repository.(controlledAuthorityRepository)
	if !ok {
		return ControlledChangeSet{}, false, ErrUnsupportedConfigurationWorkflow
	}
	mapping, err := repo.GetPlatformEntityMapping(ctx, actor.OrganizationID, projectID, mappingID)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	if mapping.Status != PlatformEntityMappingConfirmed || mapping.Version != request.ExpectedMappingVersion || mapping.PlatformObjectKind != "promotion" || strings.TrimSpace(mapping.PlatformObjectID) == "" {
		return ControlledChangeSet{}, false, ErrVersionConflict
	}
	if mapping.PlatformStatus != request.CurrentPlatformStatus || mapping.CurrentStateAction != ControlledActionPausePromotion {
		return ControlledChangeSet{}, false, ErrInvalidState
	}
	sourceExecution, err := repo.GetControlledExecution(ctx, actor.OrganizationID, projectID, mapping.BusinessExecutionID)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	sourceChange, err := repo.GetControlledChangeSet(ctx, actor.OrganizationID, projectID, sourceExecution.ControlledChangeSetID)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	pause := sourceChange.Binding.PromotionControl
	if sourceExecution.Status != "succeeded" || sourceExecution.BrowserRpaRunID != mapping.BrowserRpaRunID || sourceChange.Status != ControlledChangeSetExecuted || sourceChange.Action != ControlledActionPausePromotion || pause == nil || sourceChange.Binding.AccountReferenceID != mapping.AccountReferenceID || sourceChange.Binding.PlanID != mapping.PlanID || sourceChange.Binding.ConfigurationID != mapping.ConfigurationID || sourceChange.Binding.ParentPlatformProjectID == "" || sourceChange.Binding.TargetMappingID != mapping.ID || sourceChange.Binding.TargetMappingVersion+1 != mapping.Version || sourceChange.Binding.TargetPlatformObjectID != mapping.PlatformObjectID || sourceChange.Binding.TargetPlatformObjectKind != mapping.PlatformObjectKind || pause.TargetPlatformStatus != mapping.PlatformStatus || pause.TargetStateHash != mapping.CurrentStateHash || request.CurrentDailyBudgetMinor != pause.CurrentDailyBudgetMinor || request.ApprovedDailyBudgetMinor != pause.CurrentDailyBudgetMinor {
		return ControlledChangeSet{}, false, ErrApprovalContentMismatch
	}
	restart := ControlledPromotionRestart{
		CurrentDailyBudgetMinor:  request.CurrentDailyBudgetMinor,
		ApprovedDailyBudgetMinor: request.ApprovedDailyBudgetMinor,
		CurrentPlatformStatus:    request.CurrentPlatformStatus,
		TargetPlatformStatus:     "delivering",
		Schedule:                 request.Schedule,
		Materials:                append([]ControlledMaterialReference(nil), request.Materials...),
		LandingPage:              request.LandingPage,
	}
	currentState, err := restart.statePayload(false)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	targetState, err := restart.statePayload(true)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	restart.CurrentStateHash, err = contract.CanonicalJSONHash(currentState)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	restart.TargetStateHash, err = contract.CanonicalJSONHash(targetState)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	now := s.now()
	if err := restart.ValidateAt(ControlledActionResumePromotion, now); err != nil {
		return ControlledChangeSet{}, false, err
	}
	if err := repo.ValidateControlledRestartReferences(ctx, actor.OrganizationID, projectID, mapping.AccountReferenceID, restart.Materials, restart.LandingPage); err != nil {
		return ControlledChangeSet{}, false, err
	}
	if err := validateControlledChangeSetSupersession(ctx, repo, actor, projectID, request.SupersedesControlledChangeSetID, ControlledActionResumePromotion, mapping, restart.CurrentStateHash, restart.TargetStateHash); err != nil {
		return ControlledChangeSet{}, false, err
	}
	fingerprint, err := contract.CanonicalJSONHash(struct {
		AccountReferenceID              string           `json:"account_reference_id"`
		Action                          ControlledAction `json:"action"`
		MappingID                       string           `json:"mapping_id"`
		MappingVersion                  int64            `json:"mapping_version"`
		PlatformObjectID                string           `json:"platform_object_id"`
		OperatorPrincipal               string           `json:"operator_principal_id"`
		CurrentStateHash                string           `json:"current_state_hash"`
		TargetStateHash                 string           `json:"target_state_hash"`
		SupersedesControlledChangeSetID string           `json:"supersedes_controlled_change_set_id,omitempty"`
	}{
		AccountReferenceID:              mapping.AccountReferenceID,
		Action:                          ControlledActionResumePromotion,
		MappingID:                       mapping.ID,
		MappingVersion:                  mapping.Version,
		PlatformObjectID:                mapping.PlatformObjectID,
		OperatorPrincipal:               actor.Principal.ID,
		CurrentStateHash:                restart.CurrentStateHash,
		TargetStateHash:                 restart.TargetStateHash,
		SupersedesControlledChangeSetID: strings.TrimSpace(request.SupersedesControlledChangeSetID),
	})
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	binding := sourceChange.Binding
	binding.TargetMappingID = mapping.ID
	binding.TargetMappingVersion = mapping.Version
	binding.TargetPlatformObjectID = mapping.PlatformObjectID
	binding.TargetPlatformObjectKind = mapping.PlatformObjectKind
	binding.OperatorPrincipalID = actor.Principal.ID
	binding.SupersedesControlledChangeSetID = strings.TrimSpace(request.SupersedesControlledChangeSetID)
	binding.PromotionBudgetLimitMinor = restart.ApprovedDailyBudgetMinor
	binding.ObjectFingerprint = fingerprint
	binding.PromotionMutation = nil
	binding.PromotionControl = nil
	binding.PromotionRestart = &restart
	id, err := s.idGenerator()("controlledchangeset")
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	change := ControlledChangeSet{SchemaVersion: ControlledChangeSetSchemaV1, ID: id, OrganizationID: actor.OrganizationID, ProjectID: projectID, Binding: binding, Action: ControlledActionResumePromotion, BudgetLimitMinor: restart.ApprovedDailyBudgetMinor, Currency: "CNY", Status: ControlledChangeSetReady, Version: 1, CreatedBy: actor.Principal.ID, CreatedAt: now, UpdatedAt: now}
	change.CanonicalHash, err = change.ComputeCanonicalHash()
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	if err := change.Validate(); err != nil {
		return ControlledChangeSet{}, false, err
	}
	return repo.CreateControlledChangeSet(ctx, change)
}

func (s Service) CompileControlledChangeSet(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, request CompileControlledChangeSetRequest) (ControlledChangeSet, bool, error) {
	if err := s.ready(actor, projectID, ScopeWrite); err != nil {
		return ControlledChangeSet{}, false, err
	}
	if strings.TrimSpace(request.ObservatoryRunID) == "" {
		return ControlledChangeSet{}, false, ErrInvalidRequest
	}
	action := request.Action
	if action == "" {
		action = ControlledActionCreateProjectAndPromotions
	}
	parentPlatformProjectID := strings.TrimSpace(request.ParentPlatformProjectID)
	if !action.Valid() || (action == ControlledActionCreateProjectAndPromotions && parentPlatformProjectID != "") || (action == ControlledActionCreatePromotionsInExistingProject && parentPlatformProjectID == "") {
		return ControlledChangeSet{}, false, ErrInvalidRequest
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ControlledChangeSet{}, false, err
	}
	observatory, err := s.observatory()
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	decisions, err := s.decisionWorkflows()
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	repo, ok := s.Repository.(controlledAuthorityRepository)
	if !ok {
		return ControlledChangeSet{}, false, ErrUnsupportedConfigurationWorkflow
	}
	run, err := observatory.GetObservatoryRun(ctx, actor.OrganizationID, projectID, request.ObservatoryRunID)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	feedbacks, err := observatory.ListObservatoryFeedback(ctx, actor.OrganizationID, projectID, run.ID, 1)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	if len(feedbacks) != 1 || (feedbacks[0].Disposition != ObservatoryFeedbackAccepted && feedbacks[0].Disposition != ObservatoryFeedbackModified) {
		return ControlledChangeSet{}, false, ErrInvalidState
	}
	feedback := feedbacks[0]
	if feedback.RunCanonicalHash != run.CanonicalHash {
		return ControlledChangeSet{}, false, ErrApprovalContentMismatch
	}
	selection, err := decisions.GetDecisionSelection(ctx, actor.OrganizationID, projectID, run.Binding.SelectionID)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	decision, err := decisions.GetDecision(ctx, actor.OrganizationID, projectID, selection.DecisionID)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	configuration := selection.Configuration
	if feedback.FinalConfiguration != nil {
		configuration = *feedback.FinalConfiguration
	}
	if configuration.Platform != DeliveryPlatformOceanEngine || configuration.Payload.OceanEngine == nil || configuration.Payload.OceanEngine.Project == nil {
		return ControlledChangeSet{}, false, ErrInvalidState
	}
	accountID := selection.Workflow.AccountReference.ID
	if accountID == "" || configuration.Payload.OceanEngine.Project.AccountReference.ID != accountID {
		return ControlledChangeSet{}, false, ErrInvalidState
	}
	// ProjectDraftID is an internal Cookies identity. ParentPlatformProjectID is
	// the immutable OceanEngine object binding supplied by the controlled action.
	// Never compare or substitute these identifiers.
	projectBudgetMode := effectiveOceanEngineBudgetMode(configuration.Payload.OceanEngine.Project.BudgetAndBidding)
	projectBudgetLimitMinor := configuration.Payload.OceanEngine.Project.BudgetAndBidding.DailyBudgetMinor
	promotionBudgetLimitMinor := int64(0)
	for _, promotion := range configuration.Payload.OceanEngine.Promotions {
		if promotion.BudgetAndBidding == nil {
			if action == ControlledActionCreatePromotionsInExistingProject {
				return ControlledChangeSet{}, false, ErrInvalidState
			}
			continue
		}
		if promotion.BudgetAndBidding.Currency != "CNY" || promotion.BudgetAndBidding.DailyBudgetMinor > math.MaxInt64-promotionBudgetLimitMinor {
			return ControlledChangeSet{}, false, ErrInvalidState
		}
		promotionBudgetLimitMinor += promotion.BudgetAndBidding.DailyBudgetMinor
	}
	budgetLimitMinor := projectBudgetLimitMinor
	if action == ControlledActionCreatePromotionsInExistingProject {
		budgetLimitMinor = promotionBudgetLimitMinor
	}
	fingerprint, err := contract.CanonicalJSONHash(struct {
		AccountID         string           `json:"account_id"`
		Action            ControlledAction `json:"action"`
		ParentProjectID   string           `json:"parent_platform_project_id,omitempty"`
		ConfigurationHash string           `json:"configuration_hash"`
	}{accountID, action, parentPlatformProjectID, configuration.CanonicalHash})
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	skill, err := platformskills.Get(platformskills.OceanEngineEcommerceManualID, platformskills.OceanEngineEcommerceManualVersion)
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	// The immutable approval binds the stage B calibration definition. Its
	// definition remains non-executable until gate one revalidates live DOM
	// locators and a real Browser Driver is delivered.
	binding := ControlledAuthorityBinding{SelectionID: selection.ID, ObservatoryRunID: run.ID, ObservatoryRunCanonicalHash: run.CanonicalHash, OperatorFeedbackID: feedback.ID, OperatorFeedbackCanonicalHash: feedback.CanonicalHash, OperatorFeedbackDisposition: feedback.Disposition, PlanID: decision.Inputs.PlanID, PlanVersion: decision.Inputs.PlanVersion, PlanCanonicalHash: decision.Inputs.PlanCanonicalHash, IntentID: decision.Inputs.IntentID, IntentVersion: decision.Inputs.IntentVersion, IntentCanonicalHash: decision.Inputs.IntentCanonicalHash, DecisionID: decision.ID, DecisionCanonicalHash: decision.CanonicalHash, ConfigurationID: configuration.ConfigurationID, ConfigurationVersion: configuration.VersionNumber, ConfigurationCanonicalHash: configuration.CanonicalHash, WorkflowID: selection.Workflow.ID, WorkflowCanonicalHash: selection.Workflow.CanonicalHash, AccountReferenceID: accountID, ParentPlatformProjectID: parentPlatformProjectID, ProjectBudgetMode: projectBudgetMode, ProjectBudgetLimitMinor: projectBudgetLimitMinor, PromotionBudgetLimitMinor: promotionBudgetLimitMinor, ObjectFingerprint: fingerprint, SkillID: skill.ID, SkillVersion: skill.Version}
	id, err := s.idGenerator()("controlledchangeset")
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	now := s.now()
	change := ControlledChangeSet{SchemaVersion: ControlledChangeSetSchemaV1, ID: id, OrganizationID: actor.OrganizationID, ProjectID: projectID, Binding: binding, Action: action, BudgetLimitMinor: budgetLimitMinor, Currency: "CNY", Status: ControlledChangeSetReady, Version: 1, CreatedBy: actor.Principal.ID, CreatedAt: now, UpdatedAt: now}
	hash, err := change.ComputeCanonicalHash()
	if err != nil {
		return ControlledChangeSet{}, false, err
	}
	change.CanonicalHash = hash
	if err := change.Validate(); err != nil {
		return ControlledChangeSet{}, false, err
	}
	return repo.CreateControlledChangeSet(ctx, change)
}

type ApproveControlledChangeSetRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}

func (s Service) ApproveControlledChangeSet(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, id string, request ApproveControlledChangeSetRequest) (ControlledChangeSet, RemoteWriteApproval, error) {
	if err := s.ready(actor, projectID, ScopeApprove); err != nil {
		return ControlledChangeSet{}, RemoteWriteApproval{}, err
	}
	if request.ExpectedVersion < 1 {
		return ControlledChangeSet{}, RemoteWriteApproval{}, ErrInvalidRequest
	}
	repo, ok := s.Repository.(controlledAuthorityRepository)
	if !ok {
		return ControlledChangeSet{}, RemoteWriteApproval{}, ErrUnsupportedConfigurationWorkflow
	}
	change, err := repo.GetControlledChangeSet(ctx, actor.OrganizationID, projectID, id)
	if err != nil {
		return ControlledChangeSet{}, RemoteWriteApproval{}, err
	}
	if change.Version != request.ExpectedVersion {
		return ControlledChangeSet{}, RemoteWriteApproval{}, ErrVersionConflict
	}
	if change.Status != ControlledChangeSetReady {
		return ControlledChangeSet{}, RemoteWriteApproval{}, ErrInvalidState
	}
	approvalID, err := s.idGenerator()("remotewriteapproval")
	if err != nil {
		return ControlledChangeSet{}, RemoteWriteApproval{}, err
	}
	now := s.now()
	approval := RemoteWriteApproval{SchemaVersion: RemoteWriteApprovalSchemaV1, ID: approvalID, OrganizationID: actor.OrganizationID, ProjectID: projectID, ControlledChangeSetID: change.ID, ControlledChangeSetHash: change.CanonicalHash, Binding: change.Binding, Action: change.Action, Scope: "controlled_remote_write", BudgetLimitMinor: change.BudgetLimitMinor, Currency: change.Currency, ApprovedBy: actor.Principal.ID, ApprovedAt: now, ExpiresAt: now.Add(RemoteWriteApprovalTTL)}
	hash, err := approval.ComputeActionHash()
	if err != nil {
		return ControlledChangeSet{}, RemoteWriteApproval{}, err
	}
	approval.ActionHash = hash
	if err := approval.Validate(now); err != nil {
		return ControlledChangeSet{}, RemoteWriteApproval{}, err
	}
	return repo.ApproveControlledChangeSet(ctx, change, approval)
}

func (s Service) CreateControlledExecution(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, changeSetID string) (ControlledExecution, error) {
	if err := s.ready(actor, projectID, ScopeExecute); err != nil {
		return ControlledExecution{}, err
	}
	repo, ok := s.Repository.(controlledAuthorityRepository)
	if !ok {
		return ControlledExecution{}, ErrUnsupportedConfigurationWorkflow
	}
	change, err := repo.GetControlledChangeSet(ctx, actor.OrganizationID, projectID, changeSetID)
	if err != nil {
		return ControlledExecution{}, err
	}
	if change.Status != ControlledChangeSetApproved {
		return ControlledExecution{}, ErrApprovalRequired
	}
	if change.Binding.OperatorPrincipalID != "" && change.Binding.OperatorPrincipalID != actor.Principal.ID {
		return ControlledExecution{}, ErrApprovalContentMismatch
	}
	// The repository transaction verifies and links the immutable approval.
	approvalRepo, ok := s.Repository.(interface {
		GetRemoteWriteApproval(context.Context, contract.OrganizationID, contract.ProjectID, string) (RemoteWriteApproval, error)
	})
	if !ok {
		return ControlledExecution{}, ErrUnsupportedConfigurationWorkflow
	}
	approval, err := approvalRepo.GetRemoteWriteApproval(ctx, actor.OrganizationID, projectID, changeSetID)
	if err != nil {
		return ControlledExecution{}, err
	}
	if err := approval.Validate(s.now()); err != nil {
		return ControlledExecution{}, err
	}
	id, err := s.idGenerator()("controlledexecution")
	if err != nil {
		return ControlledExecution{}, err
	}
	now := s.now()
	value := ControlledExecution{ID: id, OrganizationID: actor.OrganizationID, ProjectID: projectID, ControlledChangeSetID: change.ID, RemoteWriteApprovalID: approval.ID, Status: "pending", Version: 1, CreatedBy: actor.Principal.ID, CreatedAt: now, UpdatedAt: now}
	return repo.CreateControlledExecution(ctx, value)
}
