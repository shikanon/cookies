package delivery

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/platform/contract"
)

type controlledMemoryRepository struct {
	*memoryRepository
	changes    map[string]ControlledChangeSet
	approvals  map[string]RemoteWriteApproval
	executions map[string]ControlledExecution
	mappings   map[string]PlatformEntityMapping
	revisions  map[string]PlatformEntityMappingRevision
	evidence   map[string]platformMappingEvidence
}

func newControlledMemoryRepository() *controlledMemoryRepository {
	return &controlledMemoryRepository{memoryRepository: newMemoryRepository(), changes: map[string]ControlledChangeSet{}, approvals: map[string]RemoteWriteApproval{}, executions: map[string]ControlledExecution{}, mappings: map[string]PlatformEntityMapping{}, revisions: map[string]PlatformEntityMappingRevision{}, evidence: map[string]platformMappingEvidence{}}
}
func (r *controlledMemoryRepository) CreateControlledChangeSet(_ context.Context, v ControlledChangeSet) (ControlledChangeSet, bool, error) {
	for _, existing := range r.changes {
		if existing.CanonicalHash == v.CanonicalHash {
			return existing, true, nil
		}
	}
	r.changes[repositoryKey(v.OrganizationID, v.ProjectID, v.ID)] = v
	return v, false, nil
}
func (r *controlledMemoryRepository) GetControlledChangeSet(_ context.Context, org contract.OrganizationID, project contract.ProjectID, id string) (ControlledChangeSet, error) {
	v, ok := r.changes[repositoryKey(org, project, id)]
	if !ok {
		return ControlledChangeSet{}, ErrNotFound
	}
	return v, nil
}
func (r *controlledMemoryRepository) ApproveControlledChangeSet(_ context.Context, c ControlledChangeSet, a RemoteWriteApproval) (ControlledChangeSet, RemoteWriteApproval, error) {
	c.Status = ControlledChangeSetApproved
	c.Version++
	c.UpdatedAt = a.ApprovedAt
	r.changes[repositoryKey(c.OrganizationID, c.ProjectID, c.ID)] = c
	r.approvals[repositoryKey(c.OrganizationID, c.ProjectID, c.ID)] = a
	return c, a, nil
}
func (r *controlledMemoryRepository) GetRemoteWriteApproval(_ context.Context, org contract.OrganizationID, project contract.ProjectID, changeID string) (RemoteWriteApproval, error) {
	v, ok := r.approvals[repositoryKey(org, project, changeID)]
	if !ok {
		return RemoteWriteApproval{}, ErrNotFound
	}
	return v, nil
}
func (r *controlledMemoryRepository) CreateControlledExecution(_ context.Context, v ControlledExecution) (ControlledExecution, error) {
	r.executions[repositoryKey(v.OrganizationID, v.ProjectID, v.ID)] = v
	changeKey := repositoryKey(v.OrganizationID, v.ProjectID, v.ControlledChangeSetID)
	change := r.changes[changeKey]
	change.Status = ControlledChangeSetExecuting
	change.Version++
	change.UpdatedAt = v.CreatedAt
	r.changes[changeKey] = change
	return v, nil
}
func (r *controlledMemoryRepository) GetControlledExecution(_ context.Context, org contract.OrganizationID, project contract.ProjectID, id string) (ControlledExecution, error) {
	v, ok := r.executions[repositoryKey(org, project, id)]
	if !ok {
		return ControlledExecution{}, ErrNotFound
	}
	return v, nil
}
func (r *controlledMemoryRepository) AttachBrowserRpaRun(_ context.Context, org contract.OrganizationID, project contract.ProjectID, id string, expectedVersion int64, runID string, now time.Time) (ControlledExecution, error) {
	key := repositoryKey(org, project, id)
	v, ok := r.executions[key]
	if !ok {
		return ControlledExecution{}, ErrNotFound
	}
	if v.Version != expectedVersion || v.Status != "pending" || v.BrowserRpaRunID != "" {
		return ControlledExecution{}, ErrVersionConflict
	}
	v.BrowserRpaRunID = runID
	v.Status = "running"
	v.Version++
	v.UpdatedAt = now
	r.executions[key] = v
	return v, nil
}
func (r *controlledMemoryRepository) InvalidateCalibratedControlledChangeSet(_ context.Context, org contract.OrganizationID, project contract.ProjectID, id string, expectedVersion int64, now time.Time) (ControlledChangeSet, ControlledExecution, error) {
	changeKey := repositoryKey(org, project, id)
	change, ok := r.changes[changeKey]
	if !ok {
		return ControlledChangeSet{}, ControlledExecution{}, ErrNotFound
	}
	if change.Status != ControlledChangeSetExecuting || change.Version != expectedVersion {
		return ControlledChangeSet{}, ControlledExecution{}, ErrInvalidState
	}
	var executionKey string
	var execution ControlledExecution
	for key, candidate := range r.executions {
		if candidate.OrganizationID == org && candidate.ProjectID == project && candidate.ControlledChangeSetID == id {
			executionKey, execution = key, candidate
			break
		}
	}
	if executionKey == "" || execution.Status != "running" {
		return ControlledChangeSet{}, ControlledExecution{}, ErrInvalidState
	}
	change.Status = ControlledChangeSetInvalidated
	change.Version++
	change.UpdatedAt = now
	execution.Status = "cancelled"
	execution.Version++
	execution.UpdatedAt = now
	r.changes[changeKey] = change
	r.executions[executionKey] = execution
	return change, execution, nil
}
func (r *controlledMemoryRepository) CreatePlatformEntityMapping(_ context.Context, v PlatformEntityMapping) (PlatformEntityMapping, error) {
	r.mappings[repositoryKey(v.OrganizationID, v.ProjectID, v.ID)] = v
	return v, nil
}

func (r *controlledMemoryRepository) ListPlatformEntityMappings(_ context.Context, org contract.OrganizationID, project contract.ProjectID, account string) ([]PlatformEntityMapping, error) {
	values := make([]PlatformEntityMapping, 0)
	for _, value := range r.mappings {
		if value.OrganizationID == org && value.ProjectID == project && value.AccountReferenceID == account {
			values = append(values, value)
		}
	}
	return values, nil
}
func (r *controlledMemoryRepository) GetPlatformEntityMapping(_ context.Context, org contract.OrganizationID, project contract.ProjectID, id string) (PlatformEntityMapping, error) {
	v, ok := r.mappings[repositoryKey(org, project, id)]
	if !ok {
		return PlatformEntityMapping{}, ErrNotFound
	}
	return v, nil
}
func (r *controlledMemoryRepository) GetPlatformEntityMappingByInternalObject(_ context.Context, org contract.OrganizationID, project contract.ProjectID, account, kind, internalID string) (PlatformEntityMapping, error) {
	for _, value := range r.mappings {
		if value.OrganizationID == org && value.ProjectID == project && value.AccountReferenceID == account && value.InternalObjectKind == kind && value.InternalObjectID == internalID {
			return value, nil
		}
	}
	return PlatformEntityMapping{}, ErrNotFound
}
func (r *controlledMemoryRepository) RebindSafePendingPlatformEntityMapping(_ context.Context, request rebindPendingPlatformEntityMappingRequest) (PlatformEntityMapping, error) {
	key := repositoryKey(request.OrganizationID, request.ProjectID, request.MappingID)
	value, ok := r.mappings[key]
	if !ok {
		return PlatformEntityMapping{}, ErrNotFound
	}
	if value.Status != PlatformEntityMappingPending || value.Version != request.ExpectedVersion || value.PlatformObjectID != "" || value.ResultEvidenceID != "" || value.ListEvidenceID != "" {
		return PlatformEntityMapping{}, ErrInvalidState
	}
	value.ConfigurationID = request.ConfigurationID
	value.BusinessExecutionID = request.BusinessExecutionID
	value.BrowserRpaRunID = request.BrowserRpaRunID
	value.Version++
	value.UpdatedAt = request.Now
	r.mappings[key] = value
	return value, nil
}
func (r *controlledMemoryRepository) ValidateControlledMaterialReferences(_ context.Context, org contract.OrganizationID, project contract.ProjectID, _ string, references []ControlledMaterialReference) error {
	for _, reference := range references {
		evidence, ok := r.evidence[reference.AuthorizationEvidenceID]
		if !ok {
			return ErrNotFound
		}
		if evidence.Evidence.OrganizationID != org || evidence.Evidence.ProjectID != project || evidence.Evidence.FieldReadback["authorized_material_reference_id"] != reference.ReferenceID {
			return ErrApprovalContentMismatch
		}
	}
	return nil
}
func (r *controlledMemoryRepository) ValidateControlledRestartReferences(_ context.Context, org contract.OrganizationID, project contract.ProjectID, _ string, materials []ControlledMaterialReference, landingPage ControlledLandingPageReference) error {
	for _, reference := range materials {
		evidence, ok := r.evidence[reference.AuthorizationEvidenceID]
		if !ok {
			return ErrNotFound
		}
		if evidence.Evidence.OrganizationID != org || evidence.Evidence.ProjectID != project || evidence.Evidence.FieldReadback["authorized_material_reference_id"] != reference.ReferenceID || evidence.Evidence.FieldReadback["material_available"] != "true" {
			return ErrApprovalContentMismatch
		}
	}
	landingEvidence, ok := r.evidence[landingPage.AuthorizationEvidenceID]
	if !ok {
		return ErrNotFound
	}
	if landingEvidence.Evidence.OrganizationID != org || landingEvidence.Evidence.ProjectID != project || landingEvidence.Evidence.FieldReadback["authorized_landing_page_reference_id"] != landingPage.ReferenceID || landingEvidence.Evidence.FieldReadback["landing_page_available"] != "true" {
		return ErrApprovalContentMismatch
	}
	return nil
}
func (r *controlledMemoryRepository) ConfirmPlatformEntityMapping(_ context.Context, org contract.OrganizationID, project contract.ProjectID, id string, expectedVersion int64, resultEvidenceID, listEvidenceID string) (PlatformEntityMapping, error) {
	key := repositoryKey(org, project, id)
	current, ok := r.mappings[key]
	if !ok {
		return PlatformEntityMapping{}, ErrNotFound
	}
	if current.Version != expectedVersion || current.Status != PlatformEntityMappingPending {
		return PlatformEntityMapping{}, ErrVersionConflict
	}
	result, resultOK := r.evidence[resultEvidenceID]
	list, listOK := r.evidence[listEvidenceID]
	if !resultOK || !listOK {
		return PlatformEntityMapping{}, ErrNotFound
	}
	objectID, status, err := validatePlatformMappingEvidence(current, result, list)
	if err != nil {
		return PlatformEntityMapping{}, err
	}
	current.PlatformObjectID, current.PlatformStatus = objectID, status
	current.ResultEvidenceID, current.ListEvidenceID = resultEvidenceID, listEvidenceID
	current.Status, current.Version = PlatformEntityMappingConfirmed, current.Version+1
	current.UpdatedAt = list.Evidence.CreatedAt
	r.mappings[key] = current
	pending := 0
	for _, mapping := range r.mappings {
		if mapping.OrganizationID == org && mapping.ProjectID == project && mapping.BusinessExecutionID == current.BusinessExecutionID && mapping.Status == PlatformEntityMappingPending {
			pending++
		}
	}
	fieldDrifted := result.Evidence.FieldReadback["field_reconciliation_status"] == "drifted" || list.Evidence.FieldReadback["field_reconciliation_status"] == "drifted"
	if execution, exists := r.executions[repositoryKey(org, project, current.BusinessExecutionID)]; exists && pending == 0 && !fieldDrifted {
		execution.Status = "succeeded"
		execution.Version++
		r.executions[repositoryKey(org, project, execution.ID)] = execution
		change := r.changes[repositoryKey(org, project, execution.ControlledChangeSetID)]
		change.Status = ControlledChangeSetExecuted
		change.Version++
		r.changes[repositoryKey(org, project, change.ID)] = change
	}
	return current, nil
}

func (r *controlledMemoryRepository) ConfirmPlatformEntityMappingMutation(_ context.Context, org contract.OrganizationID, project contract.ProjectID, id string, expectedVersion int64, businessExecutionID, resultEvidenceID, listEvidenceID string) (PlatformEntityMapping, PlatformEntityMappingRevision, error) {
	key := repositoryKey(org, project, id)
	mapping, ok := r.mappings[key]
	if !ok {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrNotFound
	}
	if mapping.Version != expectedVersion || mapping.Status != PlatformEntityMappingConfirmed {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrVersionConflict
	}
	execution, ok := r.executions[repositoryKey(org, project, businessExecutionID)]
	if !ok || execution.Status != "running" {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrInvalidState
	}
	change := r.changes[repositoryKey(org, project, execution.ControlledChangeSetID)]
	currentStateHash, targetStateHash, stateErr := change.Binding.existingPromotionStateHashes(change.Action)
	if stateErr != nil || !change.Action.ChangesExistingPromotion() || change.Binding.TargetMappingID != mapping.ID || change.Binding.TargetMappingVersion != mapping.Version || change.Binding.TargetPlatformObjectID != mapping.PlatformObjectID || (mapping.CurrentStateAction == change.Action && mapping.CurrentStateHash != currentStateHash) {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrApprovalContentMismatch
	}
	result, resultOK := r.evidence[resultEvidenceID]
	list, listOK := r.evidence[listEvidenceID]
	if !resultOK || !listOK {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrNotFound
	}
	evidenceScope := mapping
	evidenceScope.BrowserRpaRunID = execution.BrowserRpaRunID
	evidenceScope.InternalObjectID = change.Binding.ObjectFingerprint
	objectID, status, err := validatePlatformMappingEvidence(evidenceScope, result, list)
	targetStatus := change.Binding.existingPromotionTargetStatus()
	if err != nil || objectID != mapping.PlatformObjectID || result.Evidence.FieldReadback["target_state_hash"] != targetStateHash || list.Evidence.FieldReadback["target_state_hash"] != targetStateHash || (targetStatus != "" && status != targetStatus) {
		return PlatformEntityMapping{}, PlatformEntityMappingRevision{}, ErrApprovalContentMismatch
	}
	revision := PlatformEntityMappingRevision{MappingID: mapping.ID, OrganizationID: org, ProjectID: project, Version: mapping.Version + 1, Action: change.Action, BusinessExecutionID: execution.ID, BrowserRpaRunID: execution.BrowserRpaRunID, PlatformObjectID: objectID, PlatformStatus: status, PreviousStateAction: mapping.CurrentStateAction, PreviousStateHash: mapping.CurrentStateHash, CurrentStateAction: change.Action, CurrentStateHash: targetStateHash, ResultEvidenceID: resultEvidenceID, ListEvidenceID: listEvidenceID, CreatedAt: list.Evidence.CreatedAt}
	mapping.PlatformStatus = status
	mapping.BusinessExecutionID = execution.ID
	mapping.BrowserRpaRunID = execution.BrowserRpaRunID
	mapping.CurrentStateAction = change.Action
	mapping.CurrentStateHash = targetStateHash
	mapping.ResultEvidenceID = resultEvidenceID
	mapping.ListEvidenceID = listEvidenceID
	mapping.Version++
	mapping.UpdatedAt = list.Evidence.CreatedAt
	r.mappings[key] = mapping
	r.revisions[repositoryKey(org, project, id+"-"+strconv.FormatInt(mapping.Version, 10))] = revision
	execution.Status = "succeeded"
	execution.Version++
	r.executions[repositoryKey(org, project, execution.ID)] = execution
	change.Status = ControlledChangeSetExecuted
	change.Version++
	r.changes[repositoryKey(org, project, change.ID)] = change
	return mapping, revision, nil
}

func TestControlledAuthorityCompilesLatestReviewedStateAndApprovesExactHash(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	repo := newControlledMemoryRepository()
	counter := 0
	service := Service{Repository: repo, Projects: testProjects{}, Now: func() time.Time { return now }, NewID: func(prefix string) (string, error) { counter++; return prefix + "_" + strconv.Itoa(counter), nil }}
	actor := contract.ActorContext{OrganizationID: "org_a", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "operator_1"}, Scopes: contract.ScopesFromStrings([]string{string(ScopeWrite), string(ScopeApprove), string(ScopeExecute)})}
	selection := validObservatorySelection(t)
	selection.OrganizationID, selection.ProjectID = actor.OrganizationID, "project_a"
	selection.Workflow.OrganizationID, selection.Workflow.ProjectID = actor.OrganizationID, "project_a"
	repo.selections[repositoryKey(actor.OrganizationID, "project_a", selection.ID)] = selection
	decision := DeliveryDecision{ID: selection.DecisionID, OrganizationID: actor.OrganizationID, ProjectID: "project_a", CanonicalHash: selection.DecisionCanonicalHash, Inputs: DecisionInputBindings{PlanID: "plan_1", PlanVersion: 2, PlanCanonicalHash: selection.FinalApprovalBinding.PlanCanonicalHash, IntentID: "intent_1", IntentVersion: 1, IntentCanonicalHash: selection.FinalApprovalBinding.IntentCanonicalHash}}
	repo.decisions[repositoryKey(actor.OrganizationID, "project_a", decision.ID)] = decision
	run, err := BuildObservatoryRun(selection, validObservatoryRequest(selection, ObservatoryModePrepareNew), actor.Principal.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	repo.observatoryRuns[repositoryKey(actor.OrganizationID, "project_a", run.ID)] = run
	liveConfiguration := selection.Configuration
	liveConfiguration.Payload.OceanEngine.Project.ProjectDraftID = "platform-project-1"
	liveConfiguration.Payload.OceanEngine.Project.BudgetAndBidding.BudgetMode = OceanEngineBudgetModeUnlimited
	liveConfiguration.Payload.OceanEngine.Project.BudgetAndBidding.DailyBudgetMinor = 0
	promotionBid := int64(1)
	liveConfiguration.Payload.OceanEngine.Promotions[0].BudgetAndBidding = &OceanEngineBudgetAndBidding{BudgetMode: OceanEngineBudgetModeDaily, Currency: "CNY", DailyBudgetMinor: 30000, BiddingStrategy: "stable_cost", ChargingMode: "oCPM", BidMinor: &promotionBid}
	liveConfiguration, err = FinalizePlatformConfiguration(liveConfiguration)
	if err != nil {
		t.Fatal(err)
	}
	feedback := DeliveryObservatoryFeedback{SchemaVersion: ObservatoryFeedbackSchemaV1, ID: "feedback_1", OrganizationID: actor.OrganizationID, ProjectID: "project_a", RunID: run.ID, RunCanonicalHash: run.CanonicalHash, RunOutcome: run.Outcome, Disposition: ObservatoryFeedbackModified, Reason: "reviewed", DiffKeys: []string{"project.budget_and_bidding", "promotions.0.budget_and_bidding"}, FinalConfiguration: &liveConfiguration, FinalConfigurationCanonicalHash: liveConfiguration.CanonicalHash, CreatedBy: actor.Principal.ID, CreatedAt: now}
	feedback.CanonicalHash, _ = feedback.ComputeCanonicalHash()
	repo.observatoryFeedback[repositoryKey(actor.OrganizationID, "project_a", feedback.ID)] = feedback
	if _, _, err := service.CompileControlledChangeSet(context.Background(), actor, "project_a", CompileControlledChangeSetRequest{ObservatoryRunID: run.ID, Action: ControlledActionCreatePromotionsInExistingProject}); err != ErrInvalidRequest {
		t.Fatalf("existing-project action without parent id err=%v", err)
	}
	change, replay, err := service.CompileControlledChangeSet(context.Background(), actor, "project_a", CompileControlledChangeSetRequest{ObservatoryRunID: run.ID, Action: ControlledActionCreatePromotionsInExistingProject, ParentPlatformProjectID: "platform-project-1"})
	if err != nil || replay {
		t.Fatalf("compile replay=%t err=%v", replay, err)
	}
	if change.Binding.OperatorFeedbackCanonicalHash != feedback.CanonicalHash || change.Binding.AccountReferenceID != selection.Workflow.AccountReference.ID {
		t.Fatalf("binding=%#v", change.Binding)
	}
	if change.Binding.SkillID != "oceanengine-ecommerce-manual" || change.Binding.SkillVersion != "v0.1-calibration" {
		t.Fatalf("stage B Platform Skill calibration was not bound: %#v", change.Binding)
	}
	if change.Action != ControlledActionCreatePromotionsInExistingProject || change.Binding.ParentPlatformProjectID != "platform-project-1" || change.Binding.ProjectBudgetMode != OceanEngineBudgetModeUnlimited || change.Binding.ProjectBudgetLimitMinor != 0 || change.Binding.PromotionBudgetLimitMinor != 30000 || change.BudgetLimitMinor != 30000 {
		t.Fatalf("existing-project authority boundary=%#v change=%#v", change.Binding, change)
	}
	approved, approval, err := service.ApproveControlledChangeSet(context.Background(), actor, "project_a", change.ID, ApproveControlledChangeSetRequest{ExpectedVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != ControlledChangeSetApproved || approval.ControlledChangeSetHash != change.CanonicalHash {
		t.Fatalf("approval=%#v", approval)
	}
	execution, err := service.CreateControlledExecution(context.Background(), actor, "project_a", change.ID)
	if err != nil {
		t.Fatal(err)
	}
	if execution.RemoteWriteApprovalID != approval.ID || execution.Status != "pending" {
		t.Fatalf("execution=%#v", execution)
	}
	execution, err = service.AttachBrowserRpaRun(context.Background(), actor, "project_a", execution.ID, execution.Version, "run_1")
	if err != nil || execution.Status != "running" || execution.BrowserRpaRunID != "run_1" || execution.Version != 2 {
		t.Fatalf("attached execution=%#v err=%v", execution, err)
	}
	mapping, err := service.CreatePendingPlatformEntityMapping(context.Background(), actor, PlatformEntityMapping{ID: "mapping_1", ProjectID: "project_a", AccountReferenceID: change.Binding.AccountReferenceID, PlanID: change.Binding.PlanID, ConfigurationID: change.Binding.ConfigurationID, BusinessExecutionID: execution.ID, BrowserRpaRunID: execution.BrowserRpaRunID, InternalObjectKind: "promotion", InternalObjectID: change.Binding.ObjectFingerprint, PlatformObjectKind: "promotion"})
	if err != nil || mapping.Status != PlatformEntityMappingPending || mapping.PlatformObjectID != "" || mapping.PlatformStatus != "" || mapping.ResultEvidenceID != "" || mapping.ListEvidenceID != "" {
		t.Fatalf("pending mapping=%#v err=%v", mapping, err)
	}
	repo.evidence["evidence_result"] = validMappingEvidence(mapping, "evidence_result", "step_result", 2, browserautomation.TakeoverResultObserved, "platform_1", "pending_review")
	repo.evidence["evidence_list"] = validMappingEvidence(mapping, "evidence_list", "step_list", 3, browserautomation.TakeoverListConfirmed, "platform_1", "pending_review")
	mapping, err = service.ConfirmPlatformEntityMapping(context.Background(), actor, "project_a", mapping.ID, ConfirmPlatformEntityMappingRequest{ExpectedVersion: mapping.Version, ResultEvidenceID: "evidence_result", ListEvidenceID: "evidence_list"})
	if err != nil || mapping.Status != PlatformEntityMappingConfirmed || mapping.Version != 2 {
		t.Fatalf("confirmed mapping=%#v err=%v", mapping, err)
	}
	calibrationRequest := CompileMappedControlledChangeSetRequest{ExpectedMappingVersion: mapping.Version, Action: ControlledActionUpdatePromotionBudget, CurrentDailyBudgetMinor: 30000, TargetDailyBudgetMinor: 31000}
	calibrationChange, replay, err := service.CompileMappedControlledChangeSet(context.Background(), actor, "project_a", mapping.ID, calibrationRequest)
	if err != nil || replay {
		t.Fatalf("compile calibration replay=%t err=%v", replay, err)
	}
	approvedCalibration, _, err := service.ApproveControlledChangeSet(context.Background(), actor, "project_a", calibrationChange.ID, ApproveControlledChangeSetRequest{ExpectedVersion: calibrationChange.Version})
	if err != nil {
		t.Fatal(err)
	}
	calibrationExecution, err := service.CreateControlledExecution(context.Background(), actor, "project_a", approvedCalibration.ID)
	if err != nil {
		t.Fatal(err)
	}
	calibrationExecution, err = service.AttachBrowserRpaRun(context.Background(), actor, "project_a", calibrationExecution.ID, calibrationExecution.Version, "run_calibration_1")
	if err != nil {
		t.Fatal(err)
	}
	executingCalibration, err := repo.GetControlledChangeSet(context.Background(), actor.OrganizationID, "project_a", approvedCalibration.ID)
	if err != nil {
		t.Fatal(err)
	}
	invalidatedCalibration, cancelledCalibrationExecution, err := service.InvalidateCalibratedControlledChangeSet(context.Background(), actor, "project_a", approvedCalibration.ID, InvalidateCalibratedControlledChangeSetRequest{ExpectedVersion: executingCalibration.Version})
	if err != nil || invalidatedCalibration.Status != ControlledChangeSetInvalidated || cancelledCalibrationExecution.Status != "cancelled" || cancelledCalibrationExecution.ID != calibrationExecution.ID {
		t.Fatalf("invalidated calibration=%#v execution=%#v err=%v", invalidatedCalibration, cancelledCalibrationExecution, err)
	}
	replayedCalibration, replay, err := service.CompileMappedControlledChangeSet(context.Background(), actor, "project_a", mapping.ID, calibrationRequest)
	if err != nil || !replay || replayedCalibration.Status != ControlledChangeSetInvalidated {
		t.Fatalf("immutable calibration replay=%#v replay=%t err=%v", replayedCalibration, replay, err)
	}
	calibrationRequest.SupersedesControlledChangeSetID = invalidatedCalibration.ID
	supersedingCalibration, replay, err := service.CompileMappedControlledChangeSet(context.Background(), actor, "project_a", mapping.ID, calibrationRequest)
	if err != nil || replay || supersedingCalibration.ID == invalidatedCalibration.ID || supersedingCalibration.Binding.SupersedesControlledChangeSetID != invalidatedCalibration.ID {
		t.Fatalf("superseding calibration=%#v replay=%t err=%v", supersedingCalibration, replay, err)
	}

	mutationChange, mutationReplay, err := service.CompileMappedControlledChangeSet(context.Background(), actor, "project_a", mapping.ID, CompileMappedControlledChangeSetRequest{ExpectedMappingVersion: mapping.Version, Action: ControlledActionUpdatePromotionBudget, CurrentDailyBudgetMinor: 30000, TargetDailyBudgetMinor: 36000})
	if err != nil || mutationReplay {
		t.Fatalf("compile mutation replay=%t err=%v", mutationReplay, err)
	}
	mutation := mutationChange.Binding.PromotionMutation
	if mutation == nil || mutationChange.Action != ControlledActionUpdatePromotionBudget || mutationChange.Binding.TargetMappingID != mapping.ID || mutationChange.Binding.TargetMappingVersion != mapping.Version || mutationChange.Binding.TargetPlatformObjectID != mapping.PlatformObjectID || mutation.CurrentDailyBudgetMinor != 30000 || mutation.TargetDailyBudgetMinor != 36000 || mutationChange.BudgetLimitMinor != 36000 {
		t.Fatalf("mutation change=%#v", mutationChange)
	}
	if mutationChange.Binding.ObjectFingerprint == change.Binding.ObjectFingerprint {
		t.Fatal("mutation reused the creation object fingerprint")
	}
	approvedMutation, mutationApproval, err := service.ApproveControlledChangeSet(context.Background(), actor, "project_a", mutationChange.ID, ApproveControlledChangeSetRequest{ExpectedVersion: mutationChange.Version})
	if err != nil || mutationApproval.ID == approval.ID || mutationApproval.ControlledChangeSetID != mutationChange.ID {
		t.Fatalf("mutation approval=%#v err=%v", mutationApproval, err)
	}
	mutationExecution, err := service.CreateControlledExecution(context.Background(), actor, "project_a", approvedMutation.ID)
	if err != nil {
		t.Fatal(err)
	}
	mutationExecution, err = service.AttachBrowserRpaRun(context.Background(), actor, "project_a", mutationExecution.ID, mutationExecution.Version, "run_mutation_1")
	if err != nil {
		t.Fatal(err)
	}
	evidenceScope := mapping
	evidenceScope.BrowserRpaRunID = mutationExecution.BrowserRpaRunID
	evidenceScope.InternalObjectID = mutationChange.Binding.ObjectFingerprint
	resultMutationEvidence := validMappingEvidence(evidenceScope, "evidence_mutation_result", "step_mutation_result", 2, browserautomation.TakeoverResultObserved, mapping.PlatformObjectID, "pending_review")
	resultMutationEvidence.Evidence.FieldReadback["target_state_hash"] = mutation.TargetStateHash
	listMutationEvidence := validMappingEvidence(evidenceScope, "evidence_mutation_list", "step_mutation_list", 3, browserautomation.TakeoverListConfirmed, mapping.PlatformObjectID, "pending_review")
	listMutationEvidence.Evidence.FieldReadback["target_state_hash"] = mutation.TargetStateHash
	repo.evidence[resultMutationEvidence.Evidence.ID] = resultMutationEvidence
	repo.evidence[listMutationEvidence.Evidence.ID] = listMutationEvidence
	updatedMapping, revision, err := service.ConfirmPlatformEntityMappingMutation(context.Background(), actor, "project_a", mapping.ID, ConfirmPlatformEntityMappingMutationRequest{ExpectedVersion: mapping.Version, BusinessExecutionID: mutationExecution.ID, ResultEvidenceID: resultMutationEvidence.Evidence.ID, ListEvidenceID: listMutationEvidence.Evidence.ID})
	if err != nil || updatedMapping.Version != mapping.Version+1 || updatedMapping.BusinessExecutionID != mutationExecution.ID || updatedMapping.BrowserRpaRunID != mutationExecution.BrowserRpaRunID || updatedMapping.CurrentStateAction != ControlledActionUpdatePromotionBudget || updatedMapping.CurrentStateHash != mutation.TargetStateHash || revision.PreviousStateHash != "" || revision.CurrentStateAction != ControlledActionUpdatePromotionBudget || revision.CurrentStateHash != mutation.TargetStateHash || revision.Action != ControlledActionUpdatePromotionBudget {
		t.Fatalf("updated mapping=%#v revision=%#v err=%v", updatedMapping, revision, err)
	}
	if _, _, err := service.CompileMappedControlledChangeSet(context.Background(), actor, "project_a", updatedMapping.ID, CompileMappedControlledChangeSetRequest{ExpectedMappingVersion: updatedMapping.Version, Action: ControlledAction("update_promotion_schedule"), CurrentDailyBudgetMinor: 36000, TargetDailyBudgetMinor: 36000}); err != ErrInvalidRequest {
		t.Fatalf("project-owned schedule was accepted through a promotion mapping: %v", err)
	}
	for _, material := range []struct{ reference, evidence string }{{"asset_a", "material_evidence_a"}, {"asset_b", "material_evidence_b"}} {
		repo.evidence[material.evidence] = platformMappingEvidence{Evidence: browserautomation.Evidence{ID: material.evidence, OrganizationID: actor.OrganizationID, ProjectID: "project_a", FieldReadback: map[string]string{"authorized_material_reference_id": material.reference, "material_available": "true"}}}
	}
	materialChange, _, err := service.CompileMappedControlledChangeSet(context.Background(), actor, "project_a", updatedMapping.ID, CompileMappedControlledChangeSetRequest{ExpectedMappingVersion: updatedMapping.Version, Action: ControlledActionUpdatePromotionMaterials, CurrentDailyBudgetMinor: 36000, TargetDailyBudgetMinor: 36000, CurrentMaterials: []ControlledMaterialReference{{ReferenceID: "asset_a", AuthorizationEvidenceID: "material_evidence_a"}}, TargetMaterials: []ControlledMaterialReference{{ReferenceID: "asset_a", AuthorizationEvidenceID: "material_evidence_a"}, {ReferenceID: "asset_b", AuthorizationEvidenceID: "material_evidence_b"}}})
	if err != nil || materialChange.Binding.PromotionMutation == nil || len(materialChange.Binding.PromotionMutation.TargetMaterials) != 2 {
		t.Fatalf("material change=%#v err=%v", materialChange, err)
	}
	if _, _, err := service.CompileEmergencyPauseChangeSet(context.Background(), actor, "project_a", updatedMapping.ID, CompileEmergencyPauseChangeSetRequest{ExpectedMappingVersion: updatedMapping.Version, CurrentDailyBudgetMinor: 36000, CurrentPlatformStatus: "delivering"}); err != ErrInvalidState {
		t.Fatalf("non-delivering promotion pause err=%v", err)
	}
	updatedMapping.PlatformStatus = "delivering"
	repo.mappings[repositoryKey(actor.OrganizationID, "project_a", updatedMapping.ID)] = updatedMapping
	pauseChange, pauseReplay, err := service.CompileEmergencyPauseChangeSet(context.Background(), actor, "project_a", updatedMapping.ID, CompileEmergencyPauseChangeSetRequest{ExpectedMappingVersion: updatedMapping.Version, CurrentDailyBudgetMinor: 36000, CurrentPlatformStatus: "delivering"})
	if err != nil || pauseReplay || pauseChange.Action != ControlledActionPausePromotion || pauseChange.Binding.PromotionControl == nil || pauseChange.Binding.PromotionMutation != nil || pauseChange.Binding.OperatorPrincipalID != actor.Principal.ID || pauseChange.BudgetLimitMinor != 36000 {
		t.Fatalf("pause change=%#v replay=%t err=%v", pauseChange, pauseReplay, err)
	}
	approvedPause, pauseApproval, err := service.ApproveControlledChangeSet(context.Background(), actor, "project_a", pauseChange.ID, ApproveControlledChangeSetRequest{ExpectedVersion: pauseChange.Version})
	if err != nil || pauseApproval.ID == mutationApproval.ID || pauseApproval.ControlledChangeSetID != pauseChange.ID {
		t.Fatalf("pause approval=%#v err=%v", pauseApproval, err)
	}
	otherOperator := actor
	otherOperator.Principal.ID = "operator_2"
	if _, err := service.CreateControlledExecution(context.Background(), otherOperator, "project_a", approvedPause.ID); err != ErrApprovalContentMismatch {
		t.Fatalf("different pause operator execution err=%v", err)
	}
	pauseExecution, err := service.CreateControlledExecution(context.Background(), actor, "project_a", approvedPause.ID)
	if err != nil {
		t.Fatal(err)
	}
	pauseExecution, err = service.AttachBrowserRpaRun(context.Background(), actor, "project_a", pauseExecution.ID, pauseExecution.Version, "run_pause_1")
	if err != nil {
		t.Fatal(err)
	}
	pauseEvidenceScope := updatedMapping
	pauseEvidenceScope.BrowserRpaRunID = pauseExecution.BrowserRpaRunID
	pauseEvidenceScope.InternalObjectID = pauseChange.Binding.ObjectFingerprint
	pauseResult := validMappingEvidence(pauseEvidenceScope, "evidence_pause_result", "step_pause_result", 2, browserautomation.TakeoverResultObserved, updatedMapping.PlatformObjectID, "paused")
	pauseResult.Evidence.FieldReadback["target_state_hash"] = pauseChange.Binding.PromotionControl.TargetStateHash
	pauseList := validMappingEvidence(pauseEvidenceScope, "evidence_pause_list", "step_pause_list", 3, browserautomation.TakeoverListConfirmed, updatedMapping.PlatformObjectID, "paused")
	pauseList.Evidence.FieldReadback["target_state_hash"] = pauseChange.Binding.PromotionControl.TargetStateHash
	repo.evidence[pauseResult.Evidence.ID] = pauseResult
	repo.evidence[pauseList.Evidence.ID] = pauseList
	pausedMapping, pauseRevision, err := service.ConfirmPlatformEntityMappingChange(context.Background(), actor, "project_a", updatedMapping.ID, ConfirmPlatformEntityMappingChangeRequest{ExpectedVersion: updatedMapping.Version, BusinessExecutionID: pauseExecution.ID, ResultEvidenceID: pauseResult.Evidence.ID, ListEvidenceID: pauseList.Evidence.ID})
	if err != nil || pausedMapping.PlatformStatus != "paused" || pausedMapping.Version != updatedMapping.Version+1 || pausedMapping.CurrentStateAction != ControlledActionPausePromotion || pausedMapping.CurrentStateHash != pauseChange.Binding.PromotionControl.TargetStateHash || pauseRevision.PreviousStateAction != ControlledActionUpdatePromotionBudget || pauseRevision.PreviousStateHash != mutation.TargetStateHash || pauseRevision.CurrentStateAction != ControlledActionPausePromotion {
		t.Fatalf("paused mapping=%#v revision=%#v err=%v", pausedMapping, pauseRevision, err)
	}

	landingEvidenceID := "landing_evidence_a"
	repo.evidence[landingEvidenceID] = platformMappingEvidence{Evidence: browserautomation.Evidence{ID: landingEvidenceID, OrganizationID: actor.OrganizationID, ProjectID: "project_a", FieldReadback: map[string]string{"authorized_landing_page_reference_id": "landing_a", "landing_page_available": "true"}}}
	restartSchedule := ControlledScheduleWindow{StartAt: now.Add(-time.Hour), EndAt: now.Add(24 * time.Hour), Timezone: "Asia/Shanghai"}
	restartRequest := CompileControlledRestartChangeSetRequest{
		ExpectedMappingVersion:   pausedMapping.Version,
		CurrentDailyBudgetMinor:  36000,
		ApprovedDailyBudgetMinor: 36000,
		CurrentPlatformStatus:    "paused",
		Schedule:                 restartSchedule,
		Materials:                []ControlledMaterialReference{{ReferenceID: "asset_a", AuthorizationEvidenceID: "material_evidence_a"}},
		LandingPage:              ControlledLandingPageReference{ReferenceID: "landing_a", AuthorizationEvidenceID: landingEvidenceID},
	}
	budgetDrift := restartRequest
	budgetDrift.ApprovedDailyBudgetMinor = 37000
	if _, _, err := service.CompileControlledRestartChangeSet(context.Background(), actor, "project_a", pausedMapping.ID, budgetDrift); err != ErrApprovalContentMismatch {
		t.Fatalf("restart budget drift err=%v", err)
	}
	expiredSchedule := restartRequest
	expiredSchedule.Schedule = ControlledScheduleWindow{StartAt: now.Add(-2 * time.Hour), EndAt: now.Add(-time.Hour), Timezone: "Asia/Shanghai"}
	if _, _, err := service.CompileControlledRestartChangeSet(context.Background(), actor, "project_a", pausedMapping.ID, expiredSchedule); err != ErrInvalidState {
		t.Fatalf("restart expired schedule err=%v", err)
	}
	restartChange, restartReplay, err := service.CompileControlledRestartChangeSet(context.Background(), actor, "project_a", pausedMapping.ID, restartRequest)
	if err != nil || restartReplay || restartChange.Action != ControlledActionResumePromotion || restartChange.Binding.PromotionRestart == nil || restartChange.Binding.PromotionMutation != nil || restartChange.Binding.PromotionControl != nil || restartChange.Binding.OperatorPrincipalID != actor.Principal.ID || restartChange.BudgetLimitMinor != 36000 {
		t.Fatalf("restart change=%#v replay=%t err=%v", restartChange, restartReplay, err)
	}
	approvedRestart, restartApproval, err := service.ApproveControlledChangeSet(context.Background(), actor, "project_a", restartChange.ID, ApproveControlledChangeSetRequest{ExpectedVersion: restartChange.Version})
	if err != nil || restartApproval.ControlledChangeSetID != restartChange.ID || restartApproval.ID == pauseApproval.ID {
		t.Fatalf("restart approval=%#v err=%v", restartApproval, err)
	}
	if err := restartApproval.Validate(restartSchedule.EndAt); err != ErrInvalidState {
		t.Fatalf("expired restart approval err=%v", err)
	}
	restartExecution, err := service.CreateControlledExecution(context.Background(), actor, "project_a", approvedRestart.ID)
	if err != nil {
		t.Fatal(err)
	}
	restartExecution, err = service.AttachBrowserRpaRun(context.Background(), actor, "project_a", restartExecution.ID, restartExecution.Version, "run_restart_1")
	if err != nil {
		t.Fatal(err)
	}
	restartEvidenceScope := pausedMapping
	restartEvidenceScope.BrowserRpaRunID = restartExecution.BrowserRpaRunID
	restartEvidenceScope.InternalObjectID = restartChange.Binding.ObjectFingerprint
	restartResult := validMappingEvidence(restartEvidenceScope, "evidence_restart_result", "step_restart_result", 2, browserautomation.TakeoverResultObserved, pausedMapping.PlatformObjectID, "delivering")
	restartResult.Evidence.FieldReadback["target_state_hash"] = restartChange.Binding.PromotionRestart.TargetStateHash
	restartList := validMappingEvidence(restartEvidenceScope, "evidence_restart_list", "step_restart_list", 3, browserautomation.TakeoverListConfirmed, pausedMapping.PlatformObjectID, "delivering")
	restartList.Evidence.FieldReadback["target_state_hash"] = restartChange.Binding.PromotionRestart.TargetStateHash
	repo.evidence[restartResult.Evidence.ID] = restartResult
	repo.evidence[restartList.Evidence.ID] = restartList
	resumedMapping, restartRevision, err := service.ConfirmPlatformEntityMappingChange(context.Background(), actor, "project_a", pausedMapping.ID, ConfirmPlatformEntityMappingChangeRequest{ExpectedVersion: pausedMapping.Version, BusinessExecutionID: restartExecution.ID, ResultEvidenceID: restartResult.Evidence.ID, ListEvidenceID: restartList.Evidence.ID})
	if err != nil || resumedMapping.PlatformStatus != "delivering" || resumedMapping.Version != pausedMapping.Version+1 || resumedMapping.CurrentStateAction != ControlledActionResumePromotion || resumedMapping.CurrentStateHash != restartChange.Binding.PromotionRestart.TargetStateHash || restartRevision.PreviousStateAction != ControlledActionPausePromotion || restartRevision.PreviousStateHash != pauseChange.Binding.PromotionControl.TargetStateHash || restartRevision.CurrentStateAction != ControlledActionResumePromotion {
		t.Fatalf("resumed mapping=%#v revision=%#v err=%v", resumedMapping, restartRevision, err)
	}
}

func validMappingEvidence(mapping PlatformEntityMapping, evidenceID, stepID string, sequence int, action browserautomation.TakeoverWriteOutcome, objectID, status string) platformMappingEvidence {
	return platformMappingEvidence{
		Evidence: browserautomation.Evidence{SchemaVersion: browserautomation.EvidenceSchemaV1, ID: evidenceID, OrganizationID: mapping.OrganizationID, ProjectID: mapping.ProjectID, RunID: mapping.BrowserRpaRunID, StepID: stepID, ObjectFingerprint: mapping.InternalObjectID, FieldReadback: map[string]string{"platform_object_id": objectID, "platform_status": status}, CreatedAt: time.Date(2026, 8, 13, 13, 0, sequence, 0, time.UTC)},
		Step:     browserautomation.RunStep{ID: stepID, RunID: mapping.BrowserRpaRunID, Sequence: sequence, Action: string(action), Status: browserautomation.StepSucceeded},
	}
}

func TestPlatformEntityMappingConfirmationRejectsUntrustedEvidence(t *testing.T) {
	now := time.Date(2026, 8, 13, 13, 0, 0, 0, time.UTC)
	actor := contract.ActorContext{OrganizationID: "org_a", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "operator"}, Scopes: contract.ScopesFromStrings([]string{string(ScopeExecute)})}
	base := PlatformEntityMapping{SchemaVersion: PlatformEntityMappingV1, ID: "mapping_1", OrganizationID: actor.OrganizationID, ProjectID: "project_a", AccountReferenceID: "account_1", PlanID: "plan_1", ConfigurationID: "configuration_1", BusinessExecutionID: "execution_1", BrowserRpaRunID: "run_1", InternalObjectKind: "project", InternalObjectID: "fingerprint_1", PlatformObjectKind: "project", Status: PlatformEntityMappingPending, Version: 1, CreatedAt: now}
	tests := []struct {
		name   string
		mutate func(*controlledMemoryRepository)
		result string
		list   string
		want   error
	}{
		{name: "evidence does not exist", result: "forged_result", list: "forged_list", want: ErrNotFound},
		{name: "cross run evidence", result: "result", list: "list", want: ErrApprovalContentMismatch, mutate: func(repo *controlledMemoryRepository) {
			result := validMappingEvidence(base, "result", "step_result", 2, browserautomation.TakeoverResultObserved, "platform_1", "pending_review")
			result.Evidence.RunID = "run_other"
			repo.evidence["result"] = result
			repo.evidence["list"] = validMappingEvidence(base, "list", "step_list", 3, browserautomation.TakeoverListConfirmed, "platform_1", "pending_review")
		}},
		{name: "wrong step action", result: "result", list: "list", want: ErrApprovalContentMismatch, mutate: func(repo *controlledMemoryRepository) {
			repo.evidence["result"] = validMappingEvidence(base, "result", "step_result", 2, browserautomation.TakeoverListConfirmed, "platform_1", "pending_review")
			repo.evidence["list"] = validMappingEvidence(base, "list", "step_list", 3, browserautomation.TakeoverListConfirmed, "platform_1", "pending_review")
		}},
		{name: "forged object value", result: "result", list: "list", want: ErrApprovalContentMismatch, mutate: func(repo *controlledMemoryRepository) {
			repo.evidence["result"] = validMappingEvidence(base, "result", "step_result", 2, browserautomation.TakeoverResultObserved, "platform_1", "pending_review")
			repo.evidence["list"] = validMappingEvidence(base, "list", "step_list", 3, browserautomation.TakeoverListConfirmed, "platform_forged", "pending_review")
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			repo := newControlledMemoryRepository()
			repo.mappings[repositoryKey(base.OrganizationID, base.ProjectID, base.ID)] = base
			if testCase.mutate != nil {
				testCase.mutate(repo)
			}
			service := Service{Repository: repo, Projects: testProjects{}, Now: func() time.Time { return now }}
			_, err := service.ConfirmPlatformEntityMapping(context.Background(), actor, base.ProjectID, base.ID, ConfirmPlatformEntityMappingRequest{ExpectedVersion: 1, ResultEvidenceID: testCase.result, ListEvidenceID: testCase.list})
			if err != testCase.want {
				t.Fatalf("err=%v want=%v", err, testCase.want)
			}
			if repo.mappings[repositoryKey(base.OrganizationID, base.ProjectID, base.ID)].Status != PlatformEntityMappingPending {
				t.Fatal("mapping was confirmed from untrusted evidence")
			}
		})
	}
}
