package delivery

import (
	"slices"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/platform/contract"
)

const (
	ControlledChangeSetSchemaV1 = "delivery-controlled-change-set/v1"
	RemoteWriteApprovalSchemaV1 = "delivery-remote-write-approval/v1"
	PlatformEntityMappingV1     = "delivery-platform-entity-mapping/v1"
	RemoteWriteApprovalTTL      = 30 * time.Minute
)

type ControlledChangeSetStatus string

const (
	ControlledChangeSetReady       ControlledChangeSetStatus = "ready_for_approval"
	ControlledChangeSetApproved    ControlledChangeSetStatus = "approved"
	ControlledChangeSetRejected    ControlledChangeSetStatus = "rejected"
	ControlledChangeSetExecuting   ControlledChangeSetStatus = "executing"
	ControlledChangeSetExecuted    ControlledChangeSetStatus = "executed"
	ControlledChangeSetInvalidated ControlledChangeSetStatus = "invalidated"
)

type ControlledAction string

const (
	ControlledActionCreateProjectAndPromotions        ControlledAction = "create_project_and_promotions"
	ControlledActionCreatePromotionsInExistingProject ControlledAction = "create_promotions_in_existing_project"
	ControlledActionUpdatePromotionBudget             ControlledAction = "update_promotion_budget"
	ControlledActionUpdatePromotionMaterials          ControlledAction = "update_promotion_materials"
	ControlledActionPausePromotion                    ControlledAction = "pause_promotion"
	ControlledActionResumePromotion                   ControlledAction = "resume_promotion"
)

func (a ControlledAction) Valid() bool {
	return slices.Contains([]ControlledAction{
		ControlledActionCreateProjectAndPromotions,
		ControlledActionCreatePromotionsInExistingProject,
		ControlledActionUpdatePromotionBudget,
		ControlledActionUpdatePromotionMaterials,
		ControlledActionPausePromotion,
		ControlledActionResumePromotion,
	}, a)
}

func (a ControlledAction) ModifiesExistingPromotion() bool {
	return slices.Contains([]ControlledAction{
		ControlledActionUpdatePromotionBudget,
		ControlledActionUpdatePromotionMaterials,
	}, a)
}

func (a ControlledAction) ChangesExistingPromotion() bool {
	return a.ModifiesExistingPromotion() || a == ControlledActionPausePromotion || a == ControlledActionResumePromotion
}

type ControlledScheduleWindow struct {
	StartAt  time.Time `json:"start_at"`
	EndAt    time.Time `json:"end_at"`
	Timezone string    `json:"timezone"`
}

func (s ControlledScheduleWindow) Validate() error {
	if s.StartAt.IsZero() || !s.EndAt.After(s.StartAt) || strings.TrimSpace(s.Timezone) == "" {
		return ErrInvalidRequest
	}
	return nil
}

type ControlledMaterialReference struct {
	ReferenceID             string `json:"reference_id"`
	AuthorizationEvidenceID string `json:"authorization_evidence_id"`
}

type ControlledLandingPageReference struct {
	ReferenceID             string `json:"reference_id"`
	AuthorizationEvidenceID string `json:"authorization_evidence_id"`
}

type ControlledPromotionMutation struct {
	CurrentDailyBudgetMinor int64                         `json:"current_daily_budget_minor,omitempty"`
	TargetDailyBudgetMinor  int64                         `json:"target_daily_budget_minor,omitempty"`
	CurrentMaterials        []ControlledMaterialReference `json:"current_materials,omitempty"`
	TargetMaterials         []ControlledMaterialReference `json:"target_materials,omitempty"`
	CurrentStateHash        string                        `json:"current_state_hash"`
	TargetStateHash         string                        `json:"target_state_hash"`
}

func (m ControlledPromotionMutation) statePayload(action ControlledAction, target bool) (any, error) {
	dailyBudgetMinor := m.CurrentDailyBudgetMinor
	if target {
		dailyBudgetMinor = m.TargetDailyBudgetMinor
	}
	if dailyBudgetMinor < 30000 {
		return nil, ErrApprovalScopeExceeded
	}
	switch action {
	case ControlledActionUpdatePromotionBudget:
		if len(m.CurrentMaterials) != 0 || len(m.TargetMaterials) != 0 || m.CurrentDailyBudgetMinor == m.TargetDailyBudgetMinor {
			return nil, ErrInvalidRequest
		}
		return struct {
			DailyBudgetMinor int64 `json:"daily_budget_minor"`
		}{dailyBudgetMinor}, nil
	case ControlledActionUpdatePromotionMaterials:
		if m.CurrentDailyBudgetMinor != m.TargetDailyBudgetMinor {
			return nil, ErrApprovalContentMismatch
		}
		value := m.CurrentMaterials
		if target {
			value = m.TargetMaterials
		}
		if target && len(value) == 0 {
			return nil, ErrInvalidRequest
		}
		previous := ""
		for _, reference := range value {
			if strings.TrimSpace(reference.ReferenceID) == "" || strings.TrimSpace(reference.AuthorizationEvidenceID) == "" || reference.ReferenceID <= previous {
				return nil, ErrInvalidRequest
			}
			previous = reference.ReferenceID
		}
		return struct {
			DailyBudgetMinor int64                         `json:"daily_budget_minor"`
			Materials        []ControlledMaterialReference `json:"materials"`
		}{dailyBudgetMinor, value}, nil
	default:
		return nil, ErrInvalidRequest
	}
}

func (m ControlledPromotionMutation) Validate(action ControlledAction) error {
	if !action.ModifiesExistingPromotion() || !isLowercaseSHA256(m.CurrentStateHash) || !isLowercaseSHA256(m.TargetStateHash) || m.CurrentStateHash == m.TargetStateHash {
		return ErrInvalidRequest
	}
	current, err := m.statePayload(action, false)
	if err != nil {
		return err
	}
	target, err := m.statePayload(action, true)
	if err != nil {
		return err
	}
	currentHash, err := contract.CanonicalJSONHash(current)
	if err != nil || currentHash != m.CurrentStateHash {
		return ErrApprovalContentMismatch
	}
	targetHash, err := contract.CanonicalJSONHash(target)
	if err != nil || targetHash != m.TargetStateHash {
		return ErrApprovalContentMismatch
	}
	return nil
}

type ControlledPromotionControl struct {
	CurrentDailyBudgetMinor int64  `json:"current_daily_budget_minor"`
	CurrentPlatformStatus   string `json:"current_platform_status"`
	TargetPlatformStatus    string `json:"target_platform_status"`
	CurrentStateHash        string `json:"current_state_hash"`
	TargetStateHash         string `json:"target_state_hash"`
}

func (c ControlledPromotionControl) statePayload(target bool) (any, error) {
	status := c.CurrentPlatformStatus
	if target {
		status = c.TargetPlatformStatus
	}
	if c.CurrentDailyBudgetMinor < 30000 || c.CurrentPlatformStatus != "delivering" || c.TargetPlatformStatus != "paused" {
		return nil, ErrApprovalScopeExceeded
	}
	return struct {
		DailyBudgetMinor int64  `json:"daily_budget_minor"`
		PlatformStatus   string `json:"platform_status"`
	}{c.CurrentDailyBudgetMinor, status}, nil
}

func (c ControlledPromotionControl) Validate(action ControlledAction) error {
	if action != ControlledActionPausePromotion || !isLowercaseSHA256(c.CurrentStateHash) || !isLowercaseSHA256(c.TargetStateHash) || c.CurrentStateHash == c.TargetStateHash {
		return ErrInvalidRequest
	}
	current, err := c.statePayload(false)
	if err != nil {
		return err
	}
	target, err := c.statePayload(true)
	if err != nil {
		return err
	}
	currentHash, err := contract.CanonicalJSONHash(current)
	if err != nil || currentHash != c.CurrentStateHash {
		return ErrApprovalContentMismatch
	}
	targetHash, err := contract.CanonicalJSONHash(target)
	if err != nil || targetHash != c.TargetStateHash {
		return ErrApprovalContentMismatch
	}
	return nil
}

type ControlledPromotionRestart struct {
	CurrentDailyBudgetMinor  int64                          `json:"current_daily_budget_minor"`
	ApprovedDailyBudgetMinor int64                          `json:"approved_daily_budget_minor"`
	CurrentPlatformStatus    string                         `json:"current_platform_status"`
	TargetPlatformStatus     string                         `json:"target_platform_status"`
	Schedule                 ControlledScheduleWindow       `json:"schedule"`
	Materials                []ControlledMaterialReference  `json:"materials"`
	LandingPage              ControlledLandingPageReference `json:"landing_page"`
	CurrentStateHash         string                         `json:"current_state_hash"`
	TargetStateHash          string                         `json:"target_state_hash"`
}

func (r ControlledPromotionRestart) statePayload(target bool) (any, error) {
	status := r.CurrentPlatformStatus
	if target {
		status = r.TargetPlatformStatus
	}
	if r.CurrentDailyBudgetMinor < 30000 || r.CurrentDailyBudgetMinor != r.ApprovedDailyBudgetMinor || r.CurrentPlatformStatus != "paused" || r.TargetPlatformStatus != "delivering" || r.Schedule.Validate() != nil || r.Schedule.Timezone != "Asia/Shanghai" || len(r.Materials) == 0 || strings.TrimSpace(r.LandingPage.ReferenceID) == "" || strings.TrimSpace(r.LandingPage.AuthorizationEvidenceID) == "" {
		return nil, ErrApprovalContentMismatch
	}
	previous := ""
	for _, reference := range r.Materials {
		if strings.TrimSpace(reference.ReferenceID) == "" || strings.TrimSpace(reference.AuthorizationEvidenceID) == "" || reference.ReferenceID <= previous {
			return nil, ErrInvalidRequest
		}
		previous = reference.ReferenceID
	}
	return struct {
		DailyBudgetMinor int64                          `json:"daily_budget_minor"`
		PlatformStatus   string                         `json:"platform_status"`
		Schedule         ControlledScheduleWindow       `json:"schedule"`
		Materials        []ControlledMaterialReference  `json:"materials"`
		LandingPage      ControlledLandingPageReference `json:"landing_page"`
	}{r.CurrentDailyBudgetMinor, status, r.Schedule, r.Materials, r.LandingPage}, nil
}

func (r ControlledPromotionRestart) Validate(action ControlledAction) error {
	if action != ControlledActionResumePromotion || !isLowercaseSHA256(r.CurrentStateHash) || !isLowercaseSHA256(r.TargetStateHash) || r.CurrentStateHash == r.TargetStateHash {
		return ErrInvalidRequest
	}
	current, err := r.statePayload(false)
	if err != nil {
		return err
	}
	target, err := r.statePayload(true)
	if err != nil {
		return err
	}
	currentHash, err := contract.CanonicalJSONHash(current)
	if err != nil || currentHash != r.CurrentStateHash {
		return ErrApprovalContentMismatch
	}
	targetHash, err := contract.CanonicalJSONHash(target)
	if err != nil || targetHash != r.TargetStateHash {
		return ErrApprovalContentMismatch
	}
	return nil
}

func (r ControlledPromotionRestart) ValidateAt(action ControlledAction, now time.Time) error {
	if err := r.Validate(action); err != nil {
		return err
	}
	if now.Before(r.Schedule.StartAt) || !now.Before(r.Schedule.EndAt) {
		return ErrInvalidState
	}
	return nil
}

type ControlledAuthorityBinding struct {
	AuthorityOrigin                 string                            `json:"authority_origin,omitempty"`
	PreflightCanonicalHash          string                            `json:"preflight_canonical_hash,omitempty"`
	SelectionID                     string                            `json:"selection_id"`
	ObservatoryRunID                string                            `json:"observatory_run_id"`
	ObservatoryRunCanonicalHash     string                            `json:"observatory_run_canonical_hash"`
	OperatorFeedbackID              string                            `json:"operator_feedback_id"`
	OperatorFeedbackCanonicalHash   string                            `json:"operator_feedback_canonical_hash"`
	OperatorFeedbackDisposition     ObservatoryFeedbackDisposition    `json:"operator_feedback_disposition"`
	PlanID                          string                            `json:"plan_id"`
	PlanVersion                     int                               `json:"plan_version"`
	PlanCanonicalHash               string                            `json:"plan_canonical_hash"`
	IntentID                        string                            `json:"intent_id"`
	IntentVersion                   int                               `json:"intent_version"`
	IntentCanonicalHash             string                            `json:"intent_canonical_hash"`
	DecisionID                      string                            `json:"decision_id"`
	DecisionCanonicalHash           string                            `json:"decision_canonical_hash"`
	ConfigurationID                 string                            `json:"configuration_id"`
	ConfigurationVersion            int                               `json:"configuration_version"`
	ConfigurationCanonicalHash      string                            `json:"configuration_canonical_hash"`
	WorkflowID                      string                            `json:"workflow_id"`
	WorkflowCanonicalHash           string                            `json:"workflow_canonical_hash"`
	ExecutionDriver                 browserautomation.ExecutionDriver `json:"execution_driver,omitempty"`
	AccountReferenceID              string                            `json:"account_reference_id"`
	OperatorPrincipalID             string                            `json:"operator_principal_id,omitempty"`
	ParentPlatformProjectID         string                            `json:"parent_platform_project_id,omitempty"`
	TargetMappingID                 string                            `json:"target_mapping_id,omitempty"`
	TargetMappingVersion            int64                             `json:"target_mapping_version,omitempty"`
	TargetPlatformObjectID          string                            `json:"target_platform_object_id,omitempty"`
	TargetPlatformObjectKind        string                            `json:"target_platform_object_kind,omitempty"`
	SupersedesControlledChangeSetID string                            `json:"supersedes_controlled_change_set_id,omitempty"`
	ProjectBudgetMode               string                            `json:"project_budget_mode,omitempty"`
	ProjectBudgetLimitMinor         int64                             `json:"project_budget_limit_minor"`
	PromotionBudgetLimitMinor       int64                             `json:"promotion_budget_limit_minor"`
	ObjectFingerprint               string                            `json:"object_fingerprint"`
	SkillID                         string                            `json:"skill_id,omitempty"`
	SkillVersion                    string                            `json:"skill_version,omitempty"`
	PromotionMutation               *ControlledPromotionMutation      `json:"promotion_mutation,omitempty"`
	PromotionControl                *ControlledPromotionControl       `json:"promotion_control,omitempty"`
	PromotionRestart                *ControlledPromotionRestart       `json:"promotion_restart,omitempty"`
}

func (b ControlledAuthorityBinding) Validate() error {
	if b.PlanID == "" || b.PlanVersion < 1 || b.IntentID == "" || b.IntentVersion < 1 || b.ConfigurationID == "" || b.ConfigurationVersion < 1 || b.WorkflowID == "" || b.AccountReferenceID == "" || b.ObjectFingerprint == "" || b.ProjectBudgetLimitMinor < 0 || b.PromotionBudgetLimitMinor < 0 || (b.SkillID == "") != (b.SkillVersion == "") {
		return ErrInvalidRequest
	}
	if b.AuthorityOrigin == "plan_execution" {
		if !isLowercaseSHA256(b.PreflightCanonicalHash) || b.SelectionID != "" || b.ObservatoryRunID != "" || b.OperatorFeedbackID != "" || b.DecisionID != "" {
			return ErrInvalidRequest
		}
	} else if b.AuthorityOrigin != "" {
		return ErrInvalidRequest
	} else if b.SelectionID == "" || b.ObservatoryRunID == "" || b.OperatorFeedbackID == "" || b.DecisionID == "" {
		return ErrInvalidRequest
	}
	if b.ExecutionDriver != "" && b.ExecutionDriver != browserautomation.ExecutionDriverOceanEngineWebAPI && b.ExecutionDriver != browserautomation.ExecutionDriverPlaywrightEdgeV3 {
		return ErrInvalidRequest
	}
	if b.ProjectBudgetMode != "" && b.ProjectBudgetMode != OceanEngineBudgetModeDaily && b.ProjectBudgetMode != OceanEngineBudgetModeUnlimited {
		return ErrInvalidRequest
	}
	if b.AuthorityOrigin != "plan_execution" && b.OperatorFeedbackDisposition != ObservatoryFeedbackAccepted && b.OperatorFeedbackDisposition != ObservatoryFeedbackModified {
		return ErrInvalidState
	}
	hashes := []string{b.PlanCanonicalHash, b.IntentCanonicalHash, b.ConfigurationCanonicalHash, b.WorkflowCanonicalHash}
	if b.AuthorityOrigin != "plan_execution" {
		hashes = append(hashes, b.ObservatoryRunCanonicalHash, b.OperatorFeedbackCanonicalHash, b.DecisionCanonicalHash)
	}
	for _, hash := range hashes {
		if !isLowercaseSHA256(hash) {
			return ErrApprovalContentMismatch
		}
	}
	if (b.PromotionMutation != nil || b.PromotionControl != nil || b.PromotionRestart != nil) && !b.HasMutationTarget() {
		return ErrInvalidRequest
	}
	if b.SupersedesControlledChangeSetID != "" && (!b.HasMutationTarget() || strings.TrimSpace(b.SupersedesControlledChangeSetID) != b.SupersedesControlledChangeSetID) {
		return ErrInvalidRequest
	}
	return nil
}

func (b ControlledAuthorityBinding) HasMutationTarget() bool {
	return b.TargetMappingID != "" && b.TargetMappingVersion > 0 && b.TargetPlatformObjectID != "" && b.TargetPlatformObjectKind == "promotion"
}

func (b ControlledAuthorityBinding) existingPromotionStateHashes(action ControlledAction) (string, string, error) {
	switch {
	case action.ModifiesExistingPromotion() && b.PromotionMutation != nil && b.PromotionControl == nil:
		return b.PromotionMutation.CurrentStateHash, b.PromotionMutation.TargetStateHash, nil
	case action == ControlledActionPausePromotion && b.PromotionMutation == nil && b.PromotionControl != nil:
		return b.PromotionControl.CurrentStateHash, b.PromotionControl.TargetStateHash, nil
	case action == ControlledActionResumePromotion && b.PromotionMutation == nil && b.PromotionControl == nil && b.PromotionRestart != nil:
		return b.PromotionRestart.CurrentStateHash, b.PromotionRestart.TargetStateHash, nil
	default:
		return "", "", ErrApprovalContentMismatch
	}
}

func (b ControlledAuthorityBinding) existingPromotionTargetStatus() string {
	if b.PromotionControl != nil {
		return b.PromotionControl.TargetPlatformStatus
	}
	if b.PromotionRestart != nil {
		return b.PromotionRestart.TargetPlatformStatus
	}
	return ""
}

type ControlledChangeSet struct {
	SchemaVersion    string                     `json:"schema_version"`
	ID               string                     `json:"id"`
	OrganizationID   contract.OrganizationID    `json:"organization_id"`
	ProjectID        contract.ProjectID         `json:"project_id"`
	Binding          ControlledAuthorityBinding `json:"binding"`
	Action           ControlledAction           `json:"action"`
	BudgetLimitMinor int64                      `json:"budget_limit_minor"`
	Currency         string                     `json:"currency"`
	Status           ControlledChangeSetStatus  `json:"status"`
	CanonicalHash    string                     `json:"canonical_hash"`
	Version          int64                      `json:"version"`
	CreatedBy        string                     `json:"created_by"`
	CreatedAt        time.Time                  `json:"created_at"`
	UpdatedAt        time.Time                  `json:"updated_at"`
}

func (c ControlledChangeSet) canonicalPayload() any {
	return struct {
		SchemaVersion    string                     `json:"schema_version"`
		OrganizationID   contract.OrganizationID    `json:"organization_id"`
		ProjectID        contract.ProjectID         `json:"project_id"`
		Binding          ControlledAuthorityBinding `json:"binding"`
		Action           ControlledAction           `json:"action"`
		BudgetLimitMinor int64                      `json:"budget_limit_minor"`
		Currency         string                     `json:"currency"`
	}{c.SchemaVersion, c.OrganizationID, c.ProjectID, c.Binding, c.Action, c.BudgetLimitMinor, c.Currency}
}

func (c ControlledChangeSet) ComputeCanonicalHash() (string, error) {
	return contract.CanonicalJSONHash(c.canonicalPayload())
}

func (c ControlledChangeSet) Validate() error {
	if c.SchemaVersion != ControlledChangeSetSchemaV1 || c.ID == "" || c.OrganizationID == "" || c.ProjectID == "" || !c.Action.Valid() || c.BudgetLimitMinor < 0 || c.Currency != "CNY" || c.Version < 1 || strings.TrimSpace(c.CreatedBy) == "" || c.CreatedAt.IsZero() || c.UpdatedAt.IsZero() {
		return ErrInvalidRequest
	}
	if err := c.Binding.Validate(); err != nil {
		return err
	}
	if err := validateControlledActionBinding(c.Action, c.Binding, c.BudgetLimitMinor); err != nil {
		return err
	}
	switch c.Status {
	case ControlledChangeSetReady, ControlledChangeSetApproved, ControlledChangeSetRejected, ControlledChangeSetExecuting, ControlledChangeSetExecuted, ControlledChangeSetInvalidated:
	default:
		return ErrInvalidState
	}
	hash, err := c.ComputeCanonicalHash()
	if err != nil || hash != c.CanonicalHash {
		return ErrApprovalContentMismatch
	}
	return nil
}

type RemoteWriteApproval struct {
	SchemaVersion           string                     `json:"schema_version"`
	ID                      string                     `json:"id"`
	OrganizationID          contract.OrganizationID    `json:"organization_id"`
	ProjectID               contract.ProjectID         `json:"project_id"`
	ControlledChangeSetID   string                     `json:"controlled_change_set_id"`
	ControlledChangeSetHash string                     `json:"controlled_change_set_hash"`
	Binding                 ControlledAuthorityBinding `json:"binding"`
	Action                  ControlledAction           `json:"action"`
	Scope                   string                     `json:"scope"`
	BudgetLimitMinor        int64                      `json:"budget_limit_minor"`
	Currency                string                     `json:"currency"`
	ActionHash              string                     `json:"action_hash"`
	ApprovedBy              string                     `json:"approved_by"`
	ApprovedAt              time.Time                  `json:"approved_at"`
	ExpiresAt               time.Time                  `json:"expires_at"`
}

func (a RemoteWriteApproval) actionPayload() any {
	return struct {
		SchemaVersion           string                     `json:"schema_version"`
		OrganizationID          contract.OrganizationID    `json:"organization_id"`
		ProjectID               contract.ProjectID         `json:"project_id"`
		ControlledChangeSetID   string                     `json:"controlled_change_set_id"`
		ControlledChangeSetHash string                     `json:"controlled_change_set_hash"`
		Binding                 ControlledAuthorityBinding `json:"binding"`
		Action                  ControlledAction           `json:"action"`
		Scope                   string                     `json:"scope"`
		BudgetLimitMinor        int64                      `json:"budget_limit_minor"`
		Currency                string                     `json:"currency"`
	}{a.SchemaVersion, a.OrganizationID, a.ProjectID, a.ControlledChangeSetID, a.ControlledChangeSetHash, a.Binding, a.Action, a.Scope, a.BudgetLimitMinor, a.Currency}
}

func (a RemoteWriteApproval) ComputeActionHash() (string, error) {
	return contract.CanonicalJSONHash(a.actionPayload())
}

func (a RemoteWriteApproval) Validate(now time.Time) error {
	if a.SchemaVersion != RemoteWriteApprovalSchemaV1 || a.ID == "" || a.OrganizationID == "" || a.ProjectID == "" || a.ControlledChangeSetID == "" || !isLowercaseSHA256(a.ControlledChangeSetHash) || !a.Action.Valid() || a.Scope != "controlled_remote_write" || a.BudgetLimitMinor < 0 || a.Currency != "CNY" || a.ApprovedBy == "" || a.ApprovedAt.IsZero() || !a.ExpiresAt.After(a.ApprovedAt) {
		return ErrInvalidRequest
	}
	if err := a.Binding.Validate(); err != nil {
		return err
	}
	if err := validateControlledActionBinding(a.Action, a.Binding, a.BudgetLimitMinor); err != nil {
		return err
	}
	if a.Action == ControlledActionResumePromotion {
		if a.Binding.PromotionRestart == nil {
			return ErrApprovalContentMismatch
		}
		if err := a.Binding.PromotionRestart.ValidateAt(a.Action, now); err != nil {
			return err
		}
	}
	hash, err := a.ComputeActionHash()
	if err != nil || hash != a.ActionHash {
		return ErrApprovalContentMismatch
	}
	if !now.Before(a.ExpiresAt) {
		return ErrApprovalExpired
	}
	return nil
}

func validateControlledActionBinding(action ControlledAction, binding ControlledAuthorityBinding, budgetLimitMinor int64) error {
	switch action {
	case ControlledActionCreateProjectAndPromotions:
		if binding.ParentPlatformProjectID != "" || binding.OperatorPrincipalID != "" || hasMutationTargetFields(binding) || binding.PromotionMutation != nil || binding.PromotionControl != nil || binding.PromotionRestart != nil || (binding.ProjectBudgetMode != "" && budgetLimitMinor != binding.ProjectBudgetLimitMinor) {
			return ErrApprovalContentMismatch
		}
	case ControlledActionCreatePromotionsInExistingProject:
		if strings.TrimSpace(binding.ParentPlatformProjectID) == "" || binding.OperatorPrincipalID != "" || hasMutationTargetFields(binding) || binding.PromotionMutation != nil || binding.PromotionControl != nil || binding.PromotionRestart != nil || binding.PromotionBudgetLimitMinor < 1 || budgetLimitMinor != binding.PromotionBudgetLimitMinor {
			return ErrApprovalContentMismatch
		}
	case ControlledActionUpdatePromotionBudget, ControlledActionUpdatePromotionMaterials:
		if strings.TrimSpace(binding.ParentPlatformProjectID) == "" || strings.TrimSpace(binding.OperatorPrincipalID) == "" || !binding.HasMutationTarget() || binding.PromotionMutation == nil || binding.PromotionControl != nil || binding.PromotionRestart != nil {
			return ErrApprovalContentMismatch
		}
		if err := binding.PromotionMutation.Validate(action); err != nil {
			return err
		}
		if budgetLimitMinor != binding.PromotionMutation.TargetDailyBudgetMinor || binding.PromotionBudgetLimitMinor != binding.PromotionMutation.TargetDailyBudgetMinor {
			return ErrApprovalScopeExceeded
		}
	case ControlledActionPausePromotion:
		if strings.TrimSpace(binding.ParentPlatformProjectID) == "" || strings.TrimSpace(binding.OperatorPrincipalID) == "" || !binding.HasMutationTarget() || binding.PromotionMutation != nil || binding.PromotionControl == nil || binding.PromotionRestart != nil {
			return ErrApprovalContentMismatch
		}
		if err := binding.PromotionControl.Validate(action); err != nil {
			return err
		}
		if budgetLimitMinor != binding.PromotionControl.CurrentDailyBudgetMinor || binding.PromotionBudgetLimitMinor != binding.PromotionControl.CurrentDailyBudgetMinor {
			return ErrApprovalScopeExceeded
		}
	case ControlledActionResumePromotion:
		if strings.TrimSpace(binding.ParentPlatformProjectID) == "" || strings.TrimSpace(binding.OperatorPrincipalID) == "" || !binding.HasMutationTarget() || binding.PromotionMutation != nil || binding.PromotionControl != nil || binding.PromotionRestart == nil {
			return ErrApprovalContentMismatch
		}
		if err := binding.PromotionRestart.Validate(action); err != nil {
			return err
		}
		if budgetLimitMinor != binding.PromotionRestart.ApprovedDailyBudgetMinor || binding.PromotionBudgetLimitMinor != binding.PromotionRestart.ApprovedDailyBudgetMinor {
			return ErrApprovalScopeExceeded
		}
	default:
		return ErrInvalidRequest
	}
	return nil
}

func hasMutationTargetFields(binding ControlledAuthorityBinding) bool {
	return binding.TargetMappingID != "" || binding.TargetMappingVersion != 0 || binding.TargetPlatformObjectID != "" || binding.TargetPlatformObjectKind != ""
}

type PlatformEntityMappingStatus string

const (
	PlatformEntityMappingPending   PlatformEntityMappingStatus = "pending_verification"
	PlatformEntityMappingConfirmed PlatformEntityMappingStatus = "confirmed"
)

type PlatformEntityMapping struct {
	SchemaVersion       string                      `json:"schema_version"`
	ID                  string                      `json:"id"`
	OrganizationID      contract.OrganizationID     `json:"organization_id"`
	ProjectID           contract.ProjectID          `json:"project_id"`
	AccountReferenceID  string                      `json:"account_reference_id"`
	PlanID              string                      `json:"plan_id"`
	ConfigurationID     string                      `json:"configuration_id"`
	BusinessExecutionID string                      `json:"business_execution_id"`
	BrowserRpaRunID     string                      `json:"browser_rpa_run_id"`
	InternalObjectKind  string                      `json:"internal_object_kind"`
	InternalObjectID    string                      `json:"internal_object_id"`
	PlatformObjectKind  string                      `json:"platform_object_kind"`
	PlatformObjectID    string                      `json:"platform_object_id"`
	PlatformStatus      string                      `json:"platform_status"`
	CurrentStateAction  ControlledAction            `json:"current_state_action,omitempty"`
	CurrentStateHash    string                      `json:"current_state_hash,omitempty"`
	ResultEvidenceID    string                      `json:"result_evidence_id"`
	ListEvidenceID      string                      `json:"list_evidence_id"`
	Status              PlatformEntityMappingStatus `json:"status"`
	Version             int64                       `json:"version"`
	CreatedAt           time.Time                   `json:"created_at"`
	UpdatedAt           time.Time                   `json:"updated_at"`
}

type PlatformEntityMappingRevision struct {
	MappingID           string                  `json:"mapping_id"`
	OrganizationID      contract.OrganizationID `json:"organization_id"`
	ProjectID           contract.ProjectID      `json:"project_id"`
	Version             int64                   `json:"version"`
	Action              ControlledAction        `json:"action"`
	BusinessExecutionID string                  `json:"business_execution_id"`
	BrowserRpaRunID     string                  `json:"browser_rpa_run_id"`
	PlatformObjectID    string                  `json:"platform_object_id"`
	PlatformStatus      string                  `json:"platform_status"`
	PreviousStateAction ControlledAction        `json:"previous_state_action,omitempty"`
	PreviousStateHash   string                  `json:"previous_state_hash,omitempty"`
	CurrentStateAction  ControlledAction        `json:"current_state_action,omitempty"`
	CurrentStateHash    string                  `json:"current_state_hash,omitempty"`
	ResultEvidenceID    string                  `json:"result_evidence_id"`
	ListEvidenceID      string                  `json:"list_evidence_id"`
	CreatedAt           time.Time               `json:"created_at"`
}

type ControlledExecution struct {
	ID                    string                  `json:"id"`
	OrganizationID        contract.OrganizationID `json:"organization_id"`
	ProjectID             contract.ProjectID      `json:"project_id"`
	ControlledChangeSetID string                  `json:"controlled_change_set_id"`
	RemoteWriteApprovalID string                  `json:"remote_write_approval_id"`
	BrowserRpaRunID       string                  `json:"browser_rpa_run_id,omitempty"`
	Status                string                  `json:"status"`
	Version               int64                   `json:"version"`
	CreatedBy             string                  `json:"created_by"`
	CreatedAt             time.Time               `json:"created_at"`
	UpdatedAt             time.Time               `json:"updated_at"`
}

func (e ControlledExecution) Validate() error {
	if e.ID == "" || e.OrganizationID == "" || e.ProjectID == "" || e.ControlledChangeSetID == "" || e.RemoteWriteApprovalID == "" || e.Version < 1 || e.CreatedBy == "" || e.CreatedAt.IsZero() || e.UpdatedAt.IsZero() {
		return ErrInvalidRequest
	}
	switch e.Status {
	case "pending", "running", "succeeded", "failed", "partial", "result_unknown", "cancelled":
		return nil
	default:
		return ErrInvalidState
	}
}

func (m PlatformEntityMapping) Validate() error {
	if m.SchemaVersion != PlatformEntityMappingV1 || m.ID == "" || m.OrganizationID == "" || m.ProjectID == "" || m.AccountReferenceID == "" || m.PlanID == "" || m.ConfigurationID == "" || m.BusinessExecutionID == "" || m.BrowserRpaRunID == "" || m.InternalObjectKind == "" || m.InternalObjectID == "" || m.PlatformObjectKind == "" || m.Version < 1 || m.CreatedAt.IsZero() || m.UpdatedAt.IsZero() || (!m.CreatedAt.Equal(m.UpdatedAt) && m.UpdatedAt.Before(m.CreatedAt)) || (m.CurrentStateHash == "") != (m.CurrentStateAction == "") || (m.CurrentStateHash != "" && (!isLowercaseSHA256(m.CurrentStateHash) || !m.CurrentStateAction.ChangesExistingPromotion())) {
		return ErrInvalidRequest
	}
	switch m.Status {
	case PlatformEntityMappingPending:
	case PlatformEntityMappingConfirmed:
		if m.PlatformObjectID == "" || m.PlatformStatus == "" || m.ResultEvidenceID == "" || m.ListEvidenceID == "" || m.ResultEvidenceID == m.ListEvidenceID {
			return ErrInvalidState
		}
	default:
		return ErrInvalidState
	}
	return nil
}
