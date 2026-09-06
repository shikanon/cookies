package browserautomation

import (
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

// Legacy schema versions recorded by the historical computer-use control
// plane. Stored rows are immutable, so these values must keep validating.
const (
	LegacyRunSchemaV1          = "computer-use-run/v1"
	LegacyAuthoritySchemaV1    = "computer-use-authority/v1"
	LegacyEvidenceSchemaV1     = "computer-use-evidence/v1"
	LegacyConfirmationSchemaV1 = "computer-use-final-confirmation/v1"
)

const (
	RunSchemaV1          = "browser-rpa-run/v1"
	AuthoritySchemaV1    = "browser-rpa-authority/v1"
	EvidenceSchemaV1     = "browser-rpa-evidence/v1"
	ConfirmationSchemaV1 = "browser-rpa-final-confirmation/v1"
)

type ExecutionDriver string

const (
	ExecutionDriverOceanEngineWebAPI ExecutionDriver = "oceanengine-web-api/session/v1"
	ExecutionDriverPlaywrightEdgeV3  ExecutionDriver = "playwright-rpa/edge/v3"
)

func acceptedRunSchemaVersion(value string) bool {
	return value == RunSchemaV1 || value == LegacyRunSchemaV1
}

func acceptedAuthoritySchemaVersion(value string) bool {
	return value == AuthoritySchemaV1 || value == LegacyAuthoritySchemaV1
}

func acceptedEvidenceSchemaVersion(value string) bool {
	return value == EvidenceSchemaV1 || value == LegacyEvidenceSchemaV1
}

func acceptedConfirmationSchemaVersion(value string) bool {
	return value == ConfirmationSchemaV1 || value == LegacyConfirmationSchemaV1
}

var (
	ErrInvalidContract   = errors.New("invalid browser-rpa contract")
	ErrInvalidTransition = errors.New("invalid browser-rpa run transition")
)

type RunState string

const (
	RunQueued               RunState = "queued"
	RunEnvironmentCheck     RunState = "environment_check"
	RunAwaitingTakeover     RunState = "awaiting_takeover"
	RunPreparing            RunState = "preparing"
	RunAwaitingConfirmation RunState = "awaiting_confirmation"
	RunSubmitting           RunState = "submitting"
	RunVerifying            RunState = "verifying"
	RunSucceeded            RunState = "succeeded"
	RunFailed               RunState = "failed"
	RunPartial              RunState = "partial"
	RunResultUnknown        RunState = "result_unknown"
	RunCancelled            RunState = "cancelled"
)

type BlockingReason string

const (
	BlockFinalConfirmationRequired BlockingReason = "FINAL_CONFIRMATION_REQUIRED"
	BlockFinalConfirmationInvalid  BlockingReason = "FINAL_CONFIRMATION_INVALID"
	BlockApprovalInvalid           BlockingReason = "APPROVAL_INVALID"
	BlockLeaseInvalid              BlockingReason = "LEASE_INVALID"
	BlockKillSwitchActive          BlockingReason = "KILL_SWITCH_ACTIVE"
	BlockAccountMismatch           BlockingReason = "ACCOUNT_MISMATCH"
	BlockProjectNotAllowed         BlockingReason = "PROJECT_NOT_ALLOWED"
	BlockSiteNotAllowed            BlockingReason = "SITE_NOT_ALLOWED"
	BlockPageDrift                 BlockingReason = "PAGE_DRIFT"
	BlockRunnerFailure             BlockingReason = "RUNNER_FAILURE"
	BlockWorkflowDrift             BlockingReason = "WORKFLOW_DRIFT"
	BlockSkillDrift                BlockingReason = "SKILL_DRIFT"
	BlockResultReconciliation      BlockingReason = "RESULT_RECONCILIATION_REQUIRED"
	BlockTargetEffectNotObserved   BlockingReason = "TARGET_EFFECT_NOT_OBSERVED"
)

type Platform string

const PlatformOceanEngine Platform = "ocean_engine"

type PromotionScheduleWindow struct {
	StartAt  time.Time `json:"start_at"`
	EndAt    time.Time `json:"end_at"`
	Timezone string    `json:"timezone"`
}

type PromotionMaterialReference struct {
	ReferenceID             string `json:"reference_id"`
	AuthorizationEvidenceID string `json:"authorization_evidence_id"`
}

type PromotionLandingPageReference struct {
	ReferenceID             string `json:"reference_id"`
	AuthorizationEvidenceID string `json:"authorization_evidence_id"`
}

type PromotionMutationBinding struct {
	CurrentDailyBudgetMinor int64                        `json:"current_daily_budget_minor"`
	TargetDailyBudgetMinor  int64                        `json:"target_daily_budget_minor"`
	CurrentMaterials        []PromotionMaterialReference `json:"current_materials,omitempty"`
	TargetMaterials         []PromotionMaterialReference `json:"target_materials,omitempty"`
	CurrentStateHash        string                       `json:"current_state_hash"`
	TargetStateHash         string                       `json:"target_state_hash"`
}

type PromotionControlBinding struct {
	CurrentDailyBudgetMinor int64  `json:"current_daily_budget_minor"`
	CurrentPlatformStatus   string `json:"current_platform_status"`
	TargetPlatformStatus    string `json:"target_platform_status"`
	CurrentStateHash        string `json:"current_state_hash"`
	TargetStateHash         string `json:"target_state_hash"`
}

func (c PromotionControlBinding) Validate(action string) error {
	if action != "pause_promotion" || c.CurrentDailyBudgetMinor < 30000 || c.CurrentPlatformStatus != "delivering" || c.TargetPlatformStatus != "paused" || !isSHA256(c.CurrentStateHash) || !isSHA256(c.TargetStateHash) || c.CurrentStateHash == c.TargetStateHash {
		return ErrInvalidContract
	}
	current := struct {
		DailyBudgetMinor int64  `json:"daily_budget_minor"`
		PlatformStatus   string `json:"platform_status"`
	}{c.CurrentDailyBudgetMinor, c.CurrentPlatformStatus}
	target := struct {
		DailyBudgetMinor int64  `json:"daily_budget_minor"`
		PlatformStatus   string `json:"platform_status"`
	}{c.CurrentDailyBudgetMinor, c.TargetPlatformStatus}
	currentHash, currentErr := contract.CanonicalJSONHash(current)
	targetHash, targetErr := contract.CanonicalJSONHash(target)
	if currentErr != nil || targetErr != nil || currentHash != c.CurrentStateHash || targetHash != c.TargetStateHash {
		return ErrInvalidContract
	}
	return nil
}

type PromotionRestartBinding struct {
	CurrentDailyBudgetMinor  int64                         `json:"current_daily_budget_minor"`
	ApprovedDailyBudgetMinor int64                         `json:"approved_daily_budget_minor"`
	CurrentPlatformStatus    string                        `json:"current_platform_status"`
	TargetPlatformStatus     string                        `json:"target_platform_status"`
	Schedule                 PromotionScheduleWindow       `json:"schedule"`
	Materials                []PromotionMaterialReference  `json:"materials"`
	LandingPage              PromotionLandingPageReference `json:"landing_page"`
	CurrentStateHash         string                        `json:"current_state_hash"`
	TargetStateHash          string                        `json:"target_state_hash"`
}

func (r PromotionRestartBinding) statePayload(target bool) (any, error) {
	status := r.CurrentPlatformStatus
	if target {
		status = r.TargetPlatformStatus
	}
	if r.CurrentDailyBudgetMinor < 30000 || r.CurrentDailyBudgetMinor != r.ApprovedDailyBudgetMinor || r.CurrentPlatformStatus != "paused" || r.TargetPlatformStatus != "delivering" || !validPromotionSchedule(r.Schedule) || r.Schedule.Timezone != "Asia/Shanghai" || len(r.Materials) == 0 || !validPromotionMaterials(r.Materials) || strings.TrimSpace(r.LandingPage.ReferenceID) == "" || strings.TrimSpace(r.LandingPage.AuthorizationEvidenceID) == "" {
		return nil, ErrInvalidContract
	}
	return struct {
		DailyBudgetMinor int64                         `json:"daily_budget_minor"`
		PlatformStatus   string                        `json:"platform_status"`
		Schedule         PromotionScheduleWindow       `json:"schedule"`
		Materials        []PromotionMaterialReference  `json:"materials"`
		LandingPage      PromotionLandingPageReference `json:"landing_page"`
	}{r.CurrentDailyBudgetMinor, status, r.Schedule, r.Materials, r.LandingPage}, nil
}

func (r PromotionRestartBinding) Validate(action string) error {
	if action != "resume_promotion" || !isSHA256(r.CurrentStateHash) || !isSHA256(r.TargetStateHash) || r.CurrentStateHash == r.TargetStateHash {
		return ErrInvalidContract
	}
	current, currentErr := r.statePayload(false)
	target, targetErr := r.statePayload(true)
	currentHash, currentHashErr := contract.CanonicalJSONHash(current)
	targetHash, targetHashErr := contract.CanonicalJSONHash(target)
	if currentErr != nil || targetErr != nil || currentHashErr != nil || targetHashErr != nil || currentHash != r.CurrentStateHash || targetHash != r.TargetStateHash {
		return ErrInvalidContract
	}
	return nil
}

func (r PromotionRestartBinding) ValidateAt(action string, now time.Time) error {
	if err := r.Validate(action); err != nil || now.Before(r.Schedule.StartAt) || !now.Before(r.Schedule.EndAt) {
		return ErrInvalidContract
	}
	return nil
}

func (r PromotionRestartBinding) readbackHashes() (string, string, error) {
	scheduleHash, scheduleErr := contract.CanonicalJSONHash(r.Schedule)
	materialsHash, materialsErr := contract.CanonicalJSONHash(r.Materials)
	if scheduleErr != nil || materialsErr != nil {
		return "", "", ErrInvalidContract
	}
	return scheduleHash, materialsHash, nil
}

func (m PromotionMutationBinding) Validate(action string) error {
	if !modifiesExistingPromotionAction(action) || m.CurrentDailyBudgetMinor < 30000 || m.TargetDailyBudgetMinor < 30000 || !isSHA256(m.CurrentStateHash) || !isSHA256(m.TargetStateHash) || m.CurrentStateHash == m.TargetStateHash {
		return ErrInvalidContract
	}
	var current, target any
	switch action {
	case "update_promotion_budget":
		if m.CurrentDailyBudgetMinor == m.TargetDailyBudgetMinor || len(m.CurrentMaterials) != 0 || len(m.TargetMaterials) != 0 {
			return ErrInvalidContract
		}
		current = struct {
			DailyBudgetMinor int64 `json:"daily_budget_minor"`
		}{m.CurrentDailyBudgetMinor}
		target = struct {
			DailyBudgetMinor int64 `json:"daily_budget_minor"`
		}{m.TargetDailyBudgetMinor}
	case "update_promotion_materials":
		if m.CurrentDailyBudgetMinor != m.TargetDailyBudgetMinor || len(m.TargetMaterials) == 0 || !validPromotionMaterials(m.CurrentMaterials) || !validPromotionMaterials(m.TargetMaterials) {
			return ErrInvalidContract
		}
		current = struct {
			DailyBudgetMinor int64                        `json:"daily_budget_minor"`
			Materials        []PromotionMaterialReference `json:"materials"`
		}{m.CurrentDailyBudgetMinor, m.CurrentMaterials}
		target = struct {
			DailyBudgetMinor int64                        `json:"daily_budget_minor"`
			Materials        []PromotionMaterialReference `json:"materials"`
		}{m.TargetDailyBudgetMinor, m.TargetMaterials}
	}
	currentHash, currentErr := contract.CanonicalJSONHash(current)
	targetHash, targetErr := contract.CanonicalJSONHash(target)
	if currentErr != nil || targetErr != nil || currentHash != m.CurrentStateHash || targetHash != m.TargetStateHash {
		return ErrInvalidContract
	}
	return nil
}

func validPromotionSchedule(value PromotionScheduleWindow) bool {
	return !value.StartAt.IsZero() && value.EndAt.After(value.StartAt) && strings.TrimSpace(value.Timezone) != ""
}

func validPromotionMaterials(values []PromotionMaterialReference) bool {
	previous := ""
	for _, value := range values {
		if strings.TrimSpace(value.ReferenceID) == "" || strings.TrimSpace(value.AuthorizationEvidenceID) == "" || value.ReferenceID <= previous {
			return false
		}
		previous = value.ReferenceID
	}
	return true
}

type AuthorityBinding struct {
	SchemaVersion                   string                    `json:"schema_version"`
	AuthorityOrigin                 string                    `json:"authority_origin,omitempty"`
	PreflightCanonicalHash          string                    `json:"preflight_canonical_hash,omitempty"`
	OrganizationID                  contract.OrganizationID   `json:"organization_id"`
	ProjectID                       contract.ProjectID        `json:"project_id"`
	BusinessExecutionID             string                    `json:"business_execution_id"`
	ChangeSetID                     string                    `json:"change_set_id"`
	ApprovalID                      string                    `json:"approval_id"`
	ApprovalActionHash              string                    `json:"approval_action_hash"`
	AccountReferenceID              string                    `json:"account_reference_id"`
	ParentPlatformProjectID         string                    `json:"parent_platform_project_id,omitempty"`
	TargetMappingID                 string                    `json:"target_mapping_id,omitempty"`
	TargetMappingVersion            int64                     `json:"target_mapping_version,omitempty"`
	TargetPlatformObjectID          string                    `json:"target_platform_object_id,omitempty"`
	TargetPlatformObjectKind        string                    `json:"target_platform_object_kind,omitempty"`
	OperatorPrincipalID             string                    `json:"operator_principal_id,omitempty"`
	SupersedesControlledChangeSetID string                    `json:"supersedes_controlled_change_set_id,omitempty"`
	PromotionMutation               *PromotionMutationBinding `json:"promotion_mutation,omitempty"`
	PromotionControl                *PromotionControlBinding  `json:"promotion_control,omitempty"`
	PromotionRestart                *PromotionRestartBinding  `json:"promotion_restart,omitempty"`
	ObjectFingerprint               string                    `json:"object_fingerprint"`
	Action                          string                    `json:"action"`
	PlanID                          string                    `json:"plan_id,omitempty"`
	PlanVersion                     int                       `json:"plan_version,omitempty"`
	ProjectBudgetMode               string                    `json:"project_budget_mode,omitempty"`
	ProjectBudgetLimitMinor         int64                     `json:"project_budget_limit_minor"`
	PromotionBudgetLimitMinor       int64                     `json:"promotion_budget_limit_minor"`
	BudgetLimitMinor                int64                     `json:"budget_limit_minor"`
	Currency                        string                    `json:"currency"`
	PlanCanonicalHash               string                    `json:"plan_canonical_hash"`
	IntentCanonicalHash             string                    `json:"intent_canonical_hash"`
	FeedbackCanonicalHash           string                    `json:"feedback_canonical_hash"`
	DecisionCanonicalHash           string                    `json:"decision_canonical_hash"`
	ConfigurationCanonicalHash      string                    `json:"configuration_canonical_hash"`
	WorkflowID                      string                    `json:"workflow_id"`
	WorkflowCanonicalHash           string                    `json:"workflow_canonical_hash"`
	ExecutionDriver                 ExecutionDriver           `json:"execution_driver,omitempty"`
	WorkflowStepID                  string                    `json:"workflow_step_id"`
	SkillID                         string                    `json:"skill_id,omitempty"`
	SkillVersion                    string                    `json:"skill_version,omitempty"`
}

func (b AuthorityBinding) Validate() error {
	if !acceptedAuthoritySchemaVersion(b.SchemaVersion) || b.OrganizationID == "" || b.ProjectID == "" ||
		b.BusinessExecutionID == "" || b.ChangeSetID == "" || b.ApprovalID == "" || b.AccountReferenceID == "" ||
		b.ObjectFingerprint == "" || !validAuthorityAction(b.Action) || b.ProjectBudgetLimitMinor < 0 || b.PromotionBudgetLimitMinor < 0 || b.BudgetLimitMinor < 0 || b.Currency != "CNY" ||
		b.WorkflowID == "" || b.WorkflowStepID == "" || (b.SkillID == "") != (b.SkillVersion == "") {
		return ErrInvalidContract
	}
	if b.AuthorityOrigin != "" && b.AuthorityOrigin != "plan_execution" {
		return ErrInvalidContract
	}
	if b.AuthorityOrigin == "plan_execution" && !isSHA256(b.PreflightCanonicalHash) {
		return ErrInvalidContract
	}
	if b.Action == "create_promotions_in_existing_project" && (strings.TrimSpace(b.ParentPlatformProjectID) == "" || b.PromotionBudgetLimitMinor < 1 || b.BudgetLimitMinor != b.PromotionBudgetLimitMinor) {
		return ErrInvalidContract
	}
	if modifiesExistingPromotionAction(b.Action) {
		if strings.TrimSpace(b.ParentPlatformProjectID) == "" || strings.TrimSpace(b.OperatorPrincipalID) == "" || b.TargetMappingID == "" || b.TargetMappingVersion < 2 || b.TargetPlatformObjectID == "" || b.TargetPlatformObjectKind != "promotion" || b.PromotionMutation == nil || b.PromotionControl != nil || b.PromotionRestart != nil || b.PromotionBudgetLimitMinor < 30000 || b.BudgetLimitMinor != b.PromotionBudgetLimitMinor || b.PromotionBudgetLimitMinor != b.PromotionMutation.TargetDailyBudgetMinor || b.PromotionMutation.Validate(b.Action) != nil {
			return ErrInvalidContract
		}
	} else if b.Action == "pause_promotion" {
		if strings.TrimSpace(b.ParentPlatformProjectID) == "" || strings.TrimSpace(b.OperatorPrincipalID) == "" || b.TargetMappingID == "" || b.TargetMappingVersion < 2 || b.TargetPlatformObjectID == "" || b.TargetPlatformObjectKind != "promotion" || b.PromotionMutation != nil || b.PromotionControl == nil || b.PromotionRestart != nil || b.PromotionBudgetLimitMinor < 30000 || b.BudgetLimitMinor != b.PromotionBudgetLimitMinor || b.PromotionBudgetLimitMinor != b.PromotionControl.CurrentDailyBudgetMinor || b.PromotionControl.Validate(b.Action) != nil {
			return ErrInvalidContract
		}
	} else if b.Action == "resume_promotion" {
		if strings.TrimSpace(b.ParentPlatformProjectID) == "" || strings.TrimSpace(b.OperatorPrincipalID) == "" || b.TargetMappingID == "" || b.TargetMappingVersion < 2 || b.TargetPlatformObjectID == "" || b.TargetPlatformObjectKind != "promotion" || b.PromotionMutation != nil || b.PromotionControl != nil || b.PromotionRestart == nil || b.PromotionBudgetLimitMinor < 30000 || b.BudgetLimitMinor != b.PromotionBudgetLimitMinor || b.PromotionBudgetLimitMinor != b.PromotionRestart.ApprovedDailyBudgetMinor || b.PromotionRestart.Validate(b.Action) != nil {
			return ErrInvalidContract
		}
	} else if b.TargetMappingID != "" || b.TargetMappingVersion != 0 || b.TargetPlatformObjectID != "" || b.TargetPlatformObjectKind != "" || b.OperatorPrincipalID != "" || b.SupersedesControlledChangeSetID != "" || b.PromotionMutation != nil || b.PromotionControl != nil || b.PromotionRestart != nil {
		return ErrInvalidContract
	}
	if b.SupersedesControlledChangeSetID != "" && strings.TrimSpace(b.SupersedesControlledChangeSetID) != b.SupersedesControlledChangeSetID {
		return ErrInvalidContract
	}
	hashes := []string{b.ApprovalActionHash, b.PlanCanonicalHash, b.IntentCanonicalHash, b.ConfigurationCanonicalHash, b.WorkflowCanonicalHash}
	if b.AuthorityOrigin != "plan_execution" {
		hashes = append(hashes, b.FeedbackCanonicalHash, b.DecisionCanonicalHash)
	}
	for _, hash := range hashes {
		if !isSHA256(hash) {
			return ErrInvalidContract
		}
	}
	if b.ExecutionDriver != "" && b.ExecutionDriver != ExecutionDriverPlaywrightEdgeV3 && b.ExecutionDriver != ExecutionDriverOceanEngineWebAPI {
		return ErrInvalidContract
	}
	return nil
}

func validAuthorityAction(action string) bool {
	return slices.Contains([]string{
		"create_project_and_promotions",
		"create_promotions_in_existing_project",
		"update_promotion_budget",
		"update_promotion_materials",
		"pause_promotion",
		"resume_promotion",
	}, action)
}

func modifiesExistingPromotionAction(action string) bool {
	return slices.Contains([]string{"update_promotion_budget", "update_promotion_materials"}, action)
}

func changesExistingPromotionAction(action string) bool {
	return modifiesExistingPromotionAction(action) || action == "pause_promotion" || action == "resume_promotion"
}

func (b AuthorityBinding) existingPromotionStateHashes() (string, string, error) {
	switch {
	case modifiesExistingPromotionAction(b.Action) && b.PromotionMutation != nil && b.PromotionControl == nil:
		return b.PromotionMutation.CurrentStateHash, b.PromotionMutation.TargetStateHash, nil
	case b.Action == "pause_promotion" && b.PromotionMutation == nil && b.PromotionControl != nil:
		return b.PromotionControl.CurrentStateHash, b.PromotionControl.TargetStateHash, nil
	case b.Action == "resume_promotion" && b.PromotionMutation == nil && b.PromotionControl == nil && b.PromotionRestart != nil:
		return b.PromotionRestart.CurrentStateHash, b.PromotionRestart.TargetStateHash, nil
	default:
		return "", "", ErrInvalidContract
	}
}

func (b AuthorityBinding) existingPromotionTargetStatus() string {
	if b.PromotionControl != nil {
		return b.PromotionControl.TargetPlatformStatus
	}
	if b.PromotionRestart != nil {
		return b.PromotionRestart.TargetPlatformStatus
	}
	return ""
}

// ExistingPromotionStateHashes exposes the server-owned current/target state
// hashes bound into the authority, for adapters that assemble pre-submit
// readback evidence.
func (b AuthorityBinding) ExistingPromotionStateHashes() (string, string, error) {
	return b.existingPromotionStateHashes()
}

func (b AuthorityBinding) validatePreSubmitReadback(readback map[string]string, now time.Time) error {
	if !changesExistingPromotionAction(b.Action) {
		return nil
	}
	currentStateHash, targetStateHash, stateErr := b.existingPromotionStateHashes()
	if stateErr != nil || readback["platform_object_id"] != b.TargetPlatformObjectID || readback["current_state_hash"] != currentStateHash || readback["target_state_hash"] != targetStateHash {
		return ErrInvalidContract
	}
	if b.Action != "resume_promotion" {
		return nil
	}
	if b.PromotionRestart == nil || b.PromotionRestart.ValidateAt(b.Action, now) != nil {
		return ErrInvalidContract
	}
	scheduleHash, materialsHash, err := b.PromotionRestart.readbackHashes()
	if err != nil || readback["account_id"] != b.AccountReferenceID || readback["platform_project_id"] != b.ParentPlatformProjectID || readback["platform_status"] != b.PromotionRestart.CurrentPlatformStatus || readback["daily_budget_minor"] != strconv.FormatInt(b.PromotionRestart.ApprovedDailyBudgetMinor, 10) || readback["schedule_hash"] != scheduleHash || readback["material_references_hash"] != materialsHash || readback["landing_page_reference_id"] != b.PromotionRestart.LandingPage.ReferenceID || readback["materials_available"] != "true" || readback["landing_page_available"] != "true" {
		return ErrInvalidContract
	}
	return nil
}

func actionRequiresBoundPlatformProject(action string) bool {
	return action == "create_promotions_in_existing_project" || changesExistingPromotionAction(action)
}

type ExecutionEnvironment struct {
	ID             string                  `json:"id"`
	OrganizationID contract.OrganizationID `json:"organization_id"`
	ProjectID      contract.ProjectID      `json:"project_id"`
	Platform       Platform                `json:"platform"`
	AccountID      string                  `json:"account_id"`
	Mode           string                  `json:"mode"`
	BrowserVersion string                  `json:"browser_version"`
	Region         string                  `json:"region"`
	Healthy        bool                    `json:"healthy"`
	// CDPEndpoint is the Chrome DevTools Protocol endpoint of the externally
	// authenticated browser session this environment drives.
	CDPEndpoint string `json:"cdp_endpoint,omitempty"`
	Version     int64  `json:"version"`
}

func (e ExecutionEnvironment) Validate() error {
	if e.ID == "" || e.OrganizationID == "" || e.ProjectID == "" || e.Platform != PlatformOceanEngine || strings.TrimSpace(e.AccountID) == "" || e.Mode != "local_visible" || strings.TrimSpace(e.BrowserVersion) == "" || strings.TrimSpace(e.Region) == "" || e.Version < 1 {
		return ErrInvalidContract
	}
	if e.CDPEndpoint != "" && !strings.HasPrefix(e.CDPEndpoint, "http://") && !strings.HasPrefix(e.CDPEndpoint, "https://") && !strings.HasPrefix(e.CDPEndpoint, "ws://") && !strings.HasPrefix(e.CDPEndpoint, "wss://") {
		return ErrInvalidContract
	}
	return nil
}

type BrowserProfile struct {
	ID             string                  `json:"id"`
	OrganizationID contract.OrganizationID `json:"organization_id"`
	ProjectID      contract.ProjectID      `json:"project_id"`
	EnvironmentID  string                  `json:"environment_id"`
	Platform       Platform                `json:"platform"`
	AccountID      string                  `json:"account_id"`
	State          string                  `json:"state"`
	Version        int64                   `json:"version"`
}

func (p BrowserProfile) Validate() error {
	if p.ID == "" || p.OrganizationID == "" || p.ProjectID == "" || p.EnvironmentID == "" || p.Platform != PlatformOceanEngine || strings.TrimSpace(p.AccountID) == "" || p.Version < 1 {
		return ErrInvalidContract
	}
	if p.State != "ready" && p.State != "takeover_required" && p.State != "disabled" {
		return ErrInvalidContract
	}
	return nil
}

type SessionLease struct {
	ID                string                  `json:"id"`
	OrganizationID    contract.OrganizationID `json:"organization_id"`
	ProjectID         contract.ProjectID      `json:"project_id"`
	RunID             string                  `json:"run_id"`
	EnvironmentID     string                  `json:"environment_id"`
	ProfileID         string                  `json:"profile_id"`
	Platform          Platform                `json:"platform"`
	AccountID         string                  `json:"account_id"`
	Holder            string                  `json:"holder"`
	FencingToken      int64                   `json:"fencing_token"`
	Version           int64                   `json:"version"`
	ExpiresAt         time.Time               `json:"expires_at"`
	HeartbeatDeadline time.Time               `json:"heartbeat_deadline"`
	ReleasedAt        *time.Time              `json:"released_at,omitempty"`
}

func (l SessionLease) ValidAt(now time.Time) bool {
	return l.ID != "" && l.RunID != "" && l.FencingToken > 0 && l.Version > 0 && l.ReleasedAt == nil && now.Before(l.ExpiresAt) && now.Before(l.HeartbeatDeadline)
}

type SitePolicy struct {
	ID                      string                  `json:"id"`
	OrganizationID          contract.OrganizationID `json:"organization_id"`
	ProjectID               contract.ProjectID      `json:"project_id"`
	Platform                Platform                `json:"platform"`
	AccountID               string                  `json:"account_id"`
	AllowedProtocols        []string                `json:"allowed_protocols"`
	AllowedHosts            []string                `json:"allowed_hosts"`
	AllowedPageKinds        []string                `json:"allowed_page_kinds"`
	AllowedPlatformProjects []string                `json:"allowed_platform_project_ids"`
	Version                 int64                   `json:"version"`
}

func (p SitePolicy) Validate() error {
	if p.ID == "" || p.OrganizationID == "" || p.ProjectID == "" || p.Platform != PlatformOceanEngine || strings.TrimSpace(p.AccountID) == "" || p.Version < 1 || len(p.AllowedProtocols) == 0 || len(p.AllowedHosts) == 0 || len(p.AllowedPageKinds) == 0 || len(p.AllowedPlatformProjects) == 0 {
		return ErrInvalidContract
	}
	for _, protocol := range p.AllowedProtocols {
		if protocol != "https" {
			return ErrInvalidContract
		}
	}
	for _, host := range p.AllowedHosts {
		if host == "" || host != strings.ToLower(host) || strings.ContainsAny(host, "*/:@") {
			return ErrInvalidContract
		}
	}
	for _, values := range [][]string{p.AllowedPageKinds, p.AllowedPlatformProjects} {
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				return ErrInvalidContract
			}
		}
	}
	return nil
}

func (p SitePolicy) Allows(rawURL, pageKind, platformProjectID string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.User != nil || parsed.Hostname() == "" {
		return false
	}
	return slices.Contains(p.AllowedProtocols, parsed.Scheme) && slices.Contains(p.AllowedHosts, strings.ToLower(parsed.Hostname())) && slices.Contains(p.AllowedPageKinds, pageKind) && slices.Contains(p.AllowedPlatformProjects, platformProjectID)
}

type KillSwitchScope string

const (
	KillSwitchGlobal       KillSwitchScope = "global"
	KillSwitchPlatform     KillSwitchScope = "platform"
	KillSwitchOrganization KillSwitchScope = "organization"
)

type KillSwitch struct {
	ID             string                  `json:"id"`
	Scope          KillSwitchScope         `json:"scope"`
	OrganizationID contract.OrganizationID `json:"organization_id,omitempty"`
	Platform       Platform                `json:"platform,omitempty"`
	Active         bool                    `json:"active"`
	Reason         string                  `json:"reason"`
	Version        int64                   `json:"version"`
	UpdatedBy      string                  `json:"updated_by"`
	UpdatedAt      time.Time               `json:"updated_at"`
}

type BrowserRpaRun struct {
	SchemaVersion   string                  `json:"schema_version"`
	ID              string                  `json:"id"`
	OrganizationID  contract.OrganizationID `json:"organization_id"`
	ProjectID       contract.ProjectID      `json:"project_id"`
	Platform        Platform                `json:"platform"`
	AccountID       string                  `json:"account_id"`
	ExecutionDriver ExecutionDriver         `json:"execution_driver"`
	Authority       AuthorityBinding        `json:"authority"`
	EnvironmentID   string                  `json:"environment_id"`
	ProfileID       string                  `json:"profile_id"`
	LeaseID         string                  `json:"lease_id"`
	PolicyID        string                  `json:"policy_id"`
	State           RunState                `json:"state"`
	BlockingReason  BlockingReason          `json:"blocking_reason,omitempty"`
	Paused          bool                    `json:"paused"`
	TakeoverActive  bool                    `json:"takeover_active"`
	Version         int64                   `json:"version"`
	IdempotencyKey  string                  `json:"idempotency_key"`
	RequestHash     string                  `json:"request_hash"`
	CreatedBy       string                  `json:"created_by"`
	CreatedAt       time.Time               `json:"created_at"`
	UpdatedAt       time.Time               `json:"updated_at"`
}

func (r BrowserRpaRun) authorizesPlatformProject(platformProjectID string) bool {
	if actionRequiresBoundPlatformProject(r.Authority.Action) {
		return r.Authority.ParentPlatformProjectID != "" && platformProjectID == r.Authority.ParentPlatformProjectID
	}
	return true
}

func (r BrowserRpaRun) Validate() error {
	if !acceptedRunSchemaVersion(r.SchemaVersion) || r.ID == "" || r.OrganizationID == "" || r.ProjectID == "" || r.Platform != PlatformOceanEngine || r.AccountID == "" || r.Version < 1 || r.IdempotencyKey == "" || !isSHA256(r.RequestHash) {
		return ErrInvalidContract
	}
	if r.EffectiveExecutionDriver() != ExecutionDriverPlaywrightEdgeV3 && r.EffectiveExecutionDriver() != ExecutionDriverOceanEngineWebAPI {
		return ErrInvalidContract
	}
	if err := r.Authority.Validate(); err != nil || r.Authority.OrganizationID != r.OrganizationID || r.Authority.ProjectID != r.ProjectID || r.Authority.AccountReferenceID != r.AccountID {
		return ErrInvalidContract
	}
	if _, ok := runTransitions[r.State]; !ok {
		return ErrInvalidContract
	}
	return nil
}

// EffectiveExecutionDriver preserves the driver for rows created before the
// execution_driver column existed.
func (r BrowserRpaRun) EffectiveExecutionDriver() ExecutionDriver {
	if r.ExecutionDriver == "" {
		return ExecutionDriverPlaywrightEdgeV3
	}
	return r.ExecutionDriver
}

var runTransitions = map[RunState][]RunState{
	RunQueued:               {RunEnvironmentCheck, RunCancelled},
	RunEnvironmentCheck:     {RunAwaitingTakeover, RunPreparing, RunFailed, RunCancelled},
	RunAwaitingTakeover:     {RunEnvironmentCheck, RunPreparing, RunCancelled},
	RunPreparing:            {RunAwaitingTakeover, RunAwaitingConfirmation, RunFailed, RunPartial, RunResultUnknown, RunCancelled},
	RunAwaitingConfirmation: {RunAwaitingTakeover, RunPreparing, RunSubmitting, RunFailed, RunCancelled},
	RunSubmitting:           {RunVerifying, RunFailed, RunPartial, RunResultUnknown},
	RunVerifying:            {RunEnvironmentCheck, RunSucceeded, RunFailed, RunPartial, RunResultUnknown},
	RunSucceeded:            {}, RunFailed: {}, RunPartial: {},
	RunResultUnknown: {RunEnvironmentCheck, RunSucceeded, RunFailed},
	RunCancelled:     {},
}

func CanTransition(from, to RunState) bool { return slices.Contains(runTransitions[from], to) }

type StepStatus string

const (
	StepPending       StepStatus = "pending"
	StepRunning       StepStatus = "running"
	StepSucceeded     StepStatus = "succeeded"
	StepFailed        StepStatus = "failed"
	StepResultUnknown StepStatus = "result_unknown"
	StepSkipped       StepStatus = "skipped"
)

type RunStep struct {
	ID             string         `json:"id"`
	RunID          string         `json:"run_id"`
	Sequence       int            `json:"sequence"`
	WorkflowStepID string         `json:"workflow_step_id"`
	Action         string         `json:"action"`
	Status         StepStatus     `json:"status"`
	BlockingReason BlockingReason `json:"blocking_reason,omitempty"`
	Attempt        int            `json:"attempt"`
	Version        int64          `json:"version"`
}

type RunEvent struct {
	ID             string                  `json:"id"`
	OrganizationID contract.OrganizationID `json:"organization_id"`
	ProjectID      contract.ProjectID      `json:"project_id"`
	RunID          string                  `json:"run_id"`
	Sequence       int64                   `json:"sequence"`
	Kind           string                  `json:"kind"`
	Summary        string                  `json:"summary"`
	Actor          string                  `json:"actor"`
	CreatedAt      time.Time               `json:"created_at"`
}

type Evidence struct {
	SchemaVersion       string                  `json:"schema_version"`
	ID                  string                  `json:"id"`
	OrganizationID      contract.OrganizationID `json:"organization_id"`
	ProjectID           contract.ProjectID      `json:"project_id"`
	RunID               string                  `json:"run_id"`
	StepID              string                  `json:"step_id"`
	BeforePageFacts     map[string]string       `json:"before_page_facts"`
	AfterPageFacts      map[string]string       `json:"after_page_facts"`
	FieldReadback       map[string]string       `json:"field_readback"`
	DiffKeys            []string                `json:"diff_keys"`
	PageReference       string                  `json:"page_reference"`
	ScreenshotReference string                  `json:"screenshot_reference,omitempty"`
	ObjectFingerprint   string                  `json:"object_fingerprint"`
	SkillVersion        string                  `json:"skill_version,omitempty"`
	SelectorVersion     string                  `json:"selector_version"`
	ActionVersion       string                  `json:"action_version"`
	RedactionVersion    string                  `json:"redaction_version"`
	CreatedAt           time.Time               `json:"created_at"`
}

type TakeoverEvidenceAction string

const (
	TakeoverObservePage   TakeoverEvidenceAction = "observe_page"
	TakeoverBeginFormFill TakeoverEvidenceAction = "begin_form_fill"
	TakeoverFieldReadback TakeoverEvidenceAction = "field_readback"
	TakeoverDiscardDraft  TakeoverEvidenceAction = "discard_draft"
	TakeoverVerifyNoWrite TakeoverEvidenceAction = "verify_no_write"
)

func (a TakeoverEvidenceAction) Valid() bool {
	return slices.Contains([]TakeoverEvidenceAction{TakeoverObservePage, TakeoverBeginFormFill, TakeoverFieldReadback, TakeoverDiscardDraft, TakeoverVerifyNoWrite}, a)
}

type TakeoverWriteOutcome string

const (
	TakeoverResultObserved TakeoverWriteOutcome = "result_observed"
	TakeoverListConfirmed  TakeoverWriteOutcome = "list_confirmed"
	TakeoverWriteRejected  TakeoverWriteOutcome = "rejected_or_error"
	TakeoverResultUnknown  TakeoverWriteOutcome = "result_unknown"
)

func (o TakeoverWriteOutcome) Valid() bool {
	return slices.Contains([]TakeoverWriteOutcome{TakeoverResultObserved, TakeoverListConfirmed, TakeoverWriteRejected, TakeoverResultUnknown}, o)
}

type FinalConfirmation struct {
	SchemaVersion  string                  `json:"schema_version"`
	ID             string                  `json:"id"`
	OrganizationID contract.OrganizationID `json:"organization_id"`
	ProjectID      contract.ProjectID      `json:"project_id"`
	RunID          string                  `json:"run_id"`
	BindingHash    string                  `json:"binding_hash"`
	TokenDigest    string                  `json:"-"`
	IssuedBy       string                  `json:"issued_by"`
	IssuedAt       time.Time               `json:"issued_at"`
	ExpiresAt      time.Time               `json:"expires_at"`
	ConsumedAt     *time.Time              `json:"consumed_at,omitempty"`
	RejectedAt     *time.Time              `json:"rejected_at,omitempty"`
	InvalidatedAt  *time.Time              `json:"invalidated_at,omitempty"`
	Version        int64                   `json:"version"`
}

func (c FinalConfirmation) UsableAt(now time.Time) bool {
	return acceptedConfirmationSchemaVersion(c.SchemaVersion) && c.ID != "" && c.RunID != "" && isSHA256(c.BindingHash) && isSHA256(c.TokenDigest) && c.Version > 0 && c.ConsumedAt == nil && c.RejectedAt == nil && c.InvalidatedAt == nil && now.Before(c.ExpiresAt)
}

type ControlledActionAttempt struct {
	ID             string                  `json:"id"`
	OrganizationID contract.OrganizationID `json:"organization_id"`
	ProjectID      contract.ProjectID      `json:"project_id"`
	RunID          string                  `json:"run_id"`
	StepID         string                  `json:"step_id"`
	ConfirmationID string                  `json:"confirmation_id"`
	ApprovalID     string                  `json:"approval_id"`
	LeaseID        string                  `json:"lease_id"`
	FencingToken   int64                   `json:"fencing_token"`
	ActionHash     string                  `json:"action_hash"`
	IdempotencyKey string                  `json:"idempotency_key"`
	Status         string                  `json:"status"`
	CreatedAt      time.Time               `json:"created_at"`
}

const (
	ControlledActionAuthorized    = "authorized"
	ControlledActionVerified      = "verified"
	ControlledActionFailed        = "failed"
	ControlledActionResultUnknown = "result_unknown"
)

func isSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}
