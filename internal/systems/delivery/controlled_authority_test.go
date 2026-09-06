package delivery

import (
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

func TestControlledAuthorityRejectsHistoricalOrRejectedFeedback(t *testing.T) {
	binding := validControlledBinding()
	binding.OperatorFeedbackDisposition = ObservatoryFeedbackRejected
	if err := binding.Validate(); err != ErrInvalidState {
		t.Fatalf("expected rejected feedback to be ineligible, got %v", err)
	}
}

func TestControlledAuthorityAcceptsPlanExecutionPreflightWithoutReplayFeedback(t *testing.T) {
	binding := ControlledAuthorityBinding{
		AuthorityOrigin: "plan_execution", PreflightCanonicalHash: testHash("a"),
		PlanID: "plan_1", PlanVersion: 2, PlanCanonicalHash: testHash("b"),
		IntentID: "intent_1", IntentVersion: 2, IntentCanonicalHash: testHash("c"),
		ConfigurationID: "configuration_1", ConfigurationVersion: 2, ConfigurationCanonicalHash: testHash("d"),
		WorkflowID: "runner-v3-plan_1", WorkflowCanonicalHash: testHash("e"), AccountReferenceID: "1855554434276391",
		ObjectFingerprint: testHash("f"), ProjectBudgetMode: OceanEngineBudgetModeDaily, ProjectBudgetLimitMinor: 30000,
	}
	if err := binding.Validate(); err != nil {
		t.Fatalf("plan execution authority failed: %v", err)
	}
	binding.PreflightCanonicalHash = ""
	if err := binding.Validate(); err != ErrInvalidRequest {
		t.Fatalf("missing preflight hash error = %v", err)
	}
}

func TestRemoteWriteApprovalHashBindsEveryAuthorityIdentity(t *testing.T) {
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	approval := RemoteWriteApproval{SchemaVersion: RemoteWriteApprovalSchemaV1, ID: "approval_1", OrganizationID: "org_1", ProjectID: "project_1", ControlledChangeSetID: "change_1", ControlledChangeSetHash: testHash("b"), Binding: validControlledBinding(), Action: ControlledActionCreateProjectAndPromotions, Scope: "controlled_remote_write", BudgetLimitMinor: 30000, Currency: "CNY", ApprovedBy: "user_1", ApprovedAt: now, ExpiresAt: now.Add(RemoteWriteApprovalTTL)}
	var err error
	approval.ActionHash, err = approval.ComputeActionHash()
	if err != nil {
		t.Fatal(err)
	}
	if err := approval.Validate(now); err != nil {
		t.Fatalf("valid approval failed: %v", err)
	}
	approval.Binding.WorkflowCanonicalHash = testHash("c")
	if err := approval.Validate(now); err != ErrApprovalContentMismatch {
		t.Fatalf("workflow drift should invalidate action hash, got %v", err)
	}
}

func TestConfirmedPlatformMappingRequiresTwoEvidenceReads(t *testing.T) {
	createdAt := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	mapping := PlatformEntityMapping{SchemaVersion: PlatformEntityMappingV1, ID: "mapping_1", OrganizationID: "org_1", ProjectID: "project_1", AccountReferenceID: "account_1", PlanID: "plan_1", ConfigurationID: "config_1", BusinessExecutionID: "execution_1", BrowserRpaRunID: "run_1", InternalObjectKind: "project", InternalObjectID: "draft_1", PlatformObjectKind: "project", PlatformObjectID: "platform_1", PlatformStatus: "pending_review", ResultEvidenceID: "evidence_result", Status: PlatformEntityMappingConfirmed, Version: 1, CreatedAt: createdAt, UpdatedAt: createdAt}
	if err := mapping.Validate(); err != ErrInvalidState {
		t.Fatalf("confirmed mapping without list evidence was accepted: %v", err)
	}
	mapping.ListEvidenceID = "evidence_list"
	if err := mapping.Validate(); err != nil {
		t.Fatalf("two-read mapping should validate: %v", err)
	}
}

func validControlledBinding() ControlledAuthorityBinding {
	return ControlledAuthorityBinding{SelectionID: "selection_1", ObservatoryRunID: "run_1", ObservatoryRunCanonicalHash: testHash("a"), OperatorFeedbackID: "feedback_1", OperatorFeedbackCanonicalHash: testHash("b"), OperatorFeedbackDisposition: ObservatoryFeedbackAccepted, PlanID: "plan_1", PlanVersion: 2, PlanCanonicalHash: testHash("c"), IntentID: "intent_1", IntentVersion: 1, IntentCanonicalHash: testHash("d"), DecisionID: "decision_1", DecisionCanonicalHash: testHash("e"), ConfigurationID: "configuration_1", ConfigurationVersion: 3, ConfigurationCanonicalHash: testHash("f"), WorkflowID: "workflow_1", WorkflowCanonicalHash: testHash("1"), AccountReferenceID: "account_1", ObjectFingerprint: "fingerprint_1", SkillID: "oceanengine-ecommerce-manual", SkillVersion: "v0.1-calibration"}
}

func TestExistingProjectControlledActionRequiresBoundParentAndPromotionBudget(t *testing.T) {
	now := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	binding := validControlledBinding()
	binding.ProjectBudgetMode = OceanEngineBudgetModeUnlimited
	binding.ParentPlatformProjectID = "platform-project-1"
	binding.PromotionBudgetLimitMinor = 30000
	change := ControlledChangeSet{SchemaVersion: ControlledChangeSetSchemaV1, ID: "change-existing", OrganizationID: "org_1", ProjectID: "project_1", Binding: binding, Action: ControlledActionCreatePromotionsInExistingProject, BudgetLimitMinor: 30000, Currency: "CNY", Status: ControlledChangeSetReady, Version: 1, CreatedBy: "operator", CreatedAt: now, UpdatedAt: now}
	change.CanonicalHash, _ = change.ComputeCanonicalHash()
	if err := change.Validate(); err != nil {
		t.Fatal(err)
	}
	change.Binding.ParentPlatformProjectID = ""
	change.CanonicalHash, _ = change.ComputeCanonicalHash()
	if err := change.Validate(); err != ErrApprovalContentMismatch {
		t.Fatalf("missing parent project err=%v", err)
	}
	change.Binding.ParentPlatformProjectID = "platform-project-1"
	change.Binding.PromotionBudgetLimitMinor = 0
	change.CanonicalHash, _ = change.ComputeCanonicalHash()
	if err := change.Validate(); err != ErrApprovalContentMismatch {
		t.Fatalf("missing promotion budget err=%v", err)
	}
	change.Binding.PromotionBudgetLimitMinor = 30000
	change.Binding.OperatorPrincipalID = "operator"
	change.CanonicalHash, _ = change.ComputeCanonicalHash()
	if err := change.Validate(); err != ErrApprovalContentMismatch {
		t.Fatalf("create action accepted a post-launch operator binding: %v", err)
	}
}

func TestFinitePromotionMutationContractsBindBudgetAndAuthorizedMaterials(t *testing.T) {
	budget, err := (CompileMappedControlledChangeSetRequest{
		Action:                  ControlledActionUpdatePromotionBudget,
		CurrentDailyBudgetMinor: 30000,
		TargetDailyBudgetMinor:  36000,
	}).mutation()
	if err != nil || budget.CurrentStateHash == budget.TargetStateHash {
		t.Fatalf("budget mutation=%#v err=%v", budget, err)
	}

	if _, err := (CompileMappedControlledChangeSetRequest{
		Action:                  ControlledAction("update_promotion_schedule"),
		CurrentDailyBudgetMinor: 30000,
		TargetDailyBudgetMinor:  30000,
	}).mutation(); err != ErrInvalidRequest {
		t.Fatalf("project-owned schedule was accepted as a promotion mutation: %v", err)
	}

	materials, err := (CompileMappedControlledChangeSetRequest{
		Action:                  ControlledActionUpdatePromotionMaterials,
		CurrentDailyBudgetMinor: 30000,
		TargetDailyBudgetMinor:  30000,
		CurrentMaterials:        []ControlledMaterialReference{{ReferenceID: "asset_a", AuthorizationEvidenceID: "evidence_a"}},
		TargetMaterials:         []ControlledMaterialReference{{ReferenceID: "asset_a", AuthorizationEvidenceID: "evidence_a"}, {ReferenceID: "asset_b", AuthorizationEvidenceID: "evidence_b"}},
	}).mutation()
	if err != nil || materials.CurrentStateHash == materials.TargetStateHash {
		t.Fatalf("materials mutation=%#v err=%v", materials, err)
	}

	_, err = (CompileMappedControlledChangeSetRequest{
		Action:                  ControlledActionUpdatePromotionMaterials,
		CurrentDailyBudgetMinor: 30000,
		TargetDailyBudgetMinor:  30000,
		CurrentMaterials:        []ControlledMaterialReference{{ReferenceID: "asset_a", AuthorizationEvidenceID: "evidence_a"}},
		TargetMaterials:         []ControlledMaterialReference{{ReferenceID: "asset_b"}},
	}).mutation()
	if err != ErrInvalidRequest {
		t.Fatalf("material without authorization evidence err=%v", err)
	}
}

func TestControlledRestartBindsBudgetScheduleReferencesAndActiveWindow(t *testing.T) {
	now := time.Date(2026, 8, 14, 14, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	restart := ControlledPromotionRestart{
		CurrentDailyBudgetMinor:  36000,
		ApprovedDailyBudgetMinor: 36000,
		CurrentPlatformStatus:    "paused",
		TargetPlatformStatus:     "delivering",
		Schedule:                 ControlledScheduleWindow{StartAt: now.Add(-time.Hour), EndAt: now.Add(24 * time.Hour), Timezone: "Asia/Shanghai"},
		Materials:                []ControlledMaterialReference{{ReferenceID: "asset_a", AuthorizationEvidenceID: "material_evidence_a"}},
		LandingPage:              ControlledLandingPageReference{ReferenceID: "landing_a", AuthorizationEvidenceID: "landing_evidence_a"},
	}
	current, err := restart.statePayload(false)
	if err != nil {
		t.Fatal(err)
	}
	target, err := restart.statePayload(true)
	if err != nil {
		t.Fatal(err)
	}
	restart.CurrentStateHash, _ = contract.CanonicalJSONHash(current)
	restart.TargetStateHash, _ = contract.CanonicalJSONHash(target)
	if err := restart.ValidateAt(ControlledActionResumePromotion, now); err != nil {
		t.Fatalf("valid restart err=%v", err)
	}
	if err := restart.ValidateAt(ControlledActionResumePromotion, restart.Schedule.EndAt); err != ErrInvalidState {
		t.Fatalf("expired schedule err=%v", err)
	}
	restart.ApprovedDailyBudgetMinor++
	if err := restart.Validate(ControlledActionResumePromotion); err != ErrApprovalContentMismatch {
		t.Fatalf("budget drift err=%v", err)
	}
}

func testHash(character string) string {
	value := ""
	for len(value) < 64 {
		value += character
	}
	return value
}

var _ contract.OrganizationID = "org_1"
