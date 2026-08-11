package delivery

import (
	"fmt"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type ApprovalAction string
type ApprovalScope string

const (
	ApprovalActionExecute    ApprovalAction = "execute"
	ApprovalScopeExecuteMock ApprovalScope  = "execute_mock"
	ApprovalTTL                             = 24 * time.Hour
)

const (
	ApprovalInvalidExpired         = "APPROVAL_EXPIRED"
	ApprovalInvalidContentMismatch = "APPROVAL_CONTENT_MISMATCH"
	ApprovalInvalidScopeExceeded   = "APPROVAL_SCOPE_EXCEEDED"
	ApprovalInvalidStalePlan       = "STALE_PLAN_VERSION"
)

type DeliveryApproval struct {
	ApprovalID                  string                  `json:"approval_id"`
	OrganizationID              contract.OrganizationID `json:"organization_id"`
	ProjectID                   contract.ProjectID      `json:"project_id"`
	PlanID                      string                  `json:"plan_id"`
	PlanVersion                 int64                   `json:"plan_version"`
	ChangeSetID                 string                  `json:"change_set_id"`
	ChangeSetVersion            int64                   `json:"change_set_version"`
	PlanCanonicalHash           string                  `json:"plan_canonical_hash"`
	TargetSnapshotHash          string                  `json:"target_snapshot_hash,omitempty"`
	ConfigurationSchemaVersion  string                  `json:"configuration_schema_version,omitempty"`
	ConfigurationID             string                  `json:"configuration_id,omitempty"`
	ConfigurationVersion        int                     `json:"configuration_version,omitempty"`
	ConfigurationPlatform       DeliveryPlatform        `json:"configuration_platform,omitempty"`
	ConfigurationProfileVersion string                  `json:"configuration_profile_version,omitempty"`
	ConfigurationCanonicalHash  string                  `json:"configuration_canonical_hash,omitempty"`
	IntentSchemaVersion         string                  `json:"intent_schema_version,omitempty"`
	IntentID                    string                  `json:"intent_id,omitempty"`
	IntentVersion               int                     `json:"intent_version,omitempty"`
	IntentCanonicalHash         string                  `json:"intent_canonical_hash,omitempty"`
	ActionHash                  string                  `json:"action_hash"`
	Action                      ApprovalAction          `json:"action"`
	Scope                       ApprovalScope           `json:"scope"`
	BudgetLimitMinor            int64                   `json:"budget_limit_minor"`
	Currency                    string                  `json:"currency"`
	ApprovedBy                  string                  `json:"approved_by"`
	ApprovedAt                  time.Time               `json:"approved_at"`
	ExpiresAt                   time.Time               `json:"expires_at"`
	Source                      Source                  `json:"source"`
	Scenario                    Scenario                `json:"scenario"`
}

type ApprovalView struct {
	DeliveryApproval
	Valid         bool   `json:"valid"`
	InvalidReason string `json:"invalid_reason,omitempty"`
	HashSummary   string `json:"hash_summary"`
	BudgetLimit   Budget `json:"budget_limit"`
}

type planCanonicalPayload struct {
	Name                   string                  `json:"name"`
	Objective              string                  `json:"objective"`
	Advertiser             AdvertiserInput         `json:"advertiser"`
	Budget                 Budget                  `json:"budget"`
	Schedule               Schedule                `json:"schedule"`
	Tracking               Tracking                `json:"tracking"`
	CreativeReferences     []CreativeReference     `json:"creative_references"`
	SourceStrategyVersion  string                  `json:"source_strategy_version"`
	Source                 Source                  `json:"source"`
	Platform               string                  `json:"platform"`
	ThreeTierConfiguration *ThreeTierConfiguration `json:"three_tier_configuration,omitempty"`
}

type approvalActionPayload struct {
	OrganizationID              contract.OrganizationID `json:"organization_id"`
	ProjectID                   contract.ProjectID      `json:"project_id"`
	PlanID                      string                  `json:"plan_id"`
	PlanVersion                 int64                   `json:"plan_version"`
	ChangeSetID                 string                  `json:"change_set_id"`
	ChangeSetVersion            int64                   `json:"change_set_version"`
	PlanCanonicalHash           string                  `json:"plan_canonical_hash"`
	Action                      ApprovalAction          `json:"action"`
	Scope                       ApprovalScope           `json:"scope"`
	BudgetLimit                 int64                   `json:"budget_limit"`
	Currency                    string                  `json:"currency"`
	TargetSnapshotHash          string                  `json:"target_snapshot_hash,omitempty"`
	ConfigurationSchemaVersion  string                  `json:"configuration_schema_version,omitempty"`
	ConfigurationID             string                  `json:"configuration_id,omitempty"`
	ConfigurationVersion        int                     `json:"configuration_version,omitempty"`
	ConfigurationPlatform       DeliveryPlatform        `json:"configuration_platform,omitempty"`
	ConfigurationProfileVersion string                  `json:"configuration_profile_version,omitempty"`
	ConfigurationCanonicalHash  string                  `json:"configuration_canonical_hash,omitempty"`
	IntentSchemaVersion         string                  `json:"intent_schema_version,omitempty"`
	IntentID                    string                  `json:"intent_id,omitempty"`
	IntentVersion               int                     `json:"intent_version,omitempty"`
	IntentCanonicalHash         string                  `json:"intent_canonical_hash,omitempty"`
}

func PlanCanonicalHash(version DeliveryPlanVersion) (string, error) {
	if version.IsPlatformConfigurationV2() {
		if err := version.DeliveryIntent.Validate(); err != nil {
			return "", err
		}
		configuration := version.PlatformConfiguration
		if configuration.Intent.SchemaVersion != version.DeliveryIntent.SchemaVersion ||
			configuration.Intent.IntentID != version.DeliveryIntent.IntentID ||
			configuration.Intent.VersionNumber != version.DeliveryIntent.VersionNumber ||
			configuration.Intent.CanonicalHash != version.DeliveryIntent.CanonicalHash {
			return "", contractFailure(ContractErrorInvalidIntent, "platform_configuration.intent", "configuration intent binding does not match the embedded delivery intent")
		}
		if err := configuration.validateStructure(); err != nil {
			return "", err
		}
		return configuration.ComputeCanonicalHash()
	}
	platform := version.Platform
	if platform == "" {
		platform = version.Advertiser.Platform
	}
	return contract.CanonicalJSONHash(planCanonicalPayload{
		Name:      version.Name,
		Objective: version.Objective,
		Advertiser: AdvertiserInput{
			ID:       version.Advertiser.ID,
			Name:     version.Advertiser.Name,
			Platform: version.Advertiser.Platform,
		},
		Budget:                 version.Budget,
		Schedule:               version.Schedule,
		Tracking:               version.Tracking,
		CreativeReferences:     append([]CreativeReference(nil), version.CreativeReferences...),
		SourceStrategyVersion:  version.SourceStrategyVersion,
		Source:                 version.Source,
		Platform:               platform,
		ThreeTierConfiguration: cloneThreeTierConfiguration(version.ThreeTierConfiguration),
	})
}

func ApprovalActionHash(approval DeliveryApproval) (string, error) {
	return contract.CanonicalJSONHash(approvalActionPayload{
		OrganizationID:              approval.OrganizationID,
		ProjectID:                   approval.ProjectID,
		PlanID:                      approval.PlanID,
		PlanVersion:                 approval.PlanVersion,
		ChangeSetID:                 approval.ChangeSetID,
		ChangeSetVersion:            approval.ChangeSetVersion,
		PlanCanonicalHash:           approval.PlanCanonicalHash,
		Action:                      approval.Action,
		Scope:                       approval.Scope,
		BudgetLimit:                 approval.BudgetLimitMinor,
		Currency:                    approval.Currency,
		TargetSnapshotHash:          approval.TargetSnapshotHash,
		ConfigurationSchemaVersion:  approval.ConfigurationSchemaVersion,
		ConfigurationID:             approval.ConfigurationID,
		ConfigurationVersion:        approval.ConfigurationVersion,
		ConfigurationPlatform:       approval.ConfigurationPlatform,
		ConfigurationProfileVersion: approval.ConfigurationProfileVersion,
		ConfigurationCanonicalHash:  approval.ConfigurationCanonicalHash,
		IntentSchemaVersion:         approval.IntentSchemaVersion,
		IntentID:                    approval.IntentID,
		IntentVersion:               approval.IntentVersion,
		IntentCanonicalHash:         approval.IntentCanonicalHash,
	})
}

func validatePlanCanonicalHash(version DeliveryPlanVersion) error {
	hash, err := PlanCanonicalHash(version)
	if err != nil {
		return err
	}
	if version.CanonicalHash == "" || hash != version.CanonicalHash {
		return fmt.Errorf("%w: plan canonical hash does not match the immutable version", ErrApprovalContentMismatch)
	}
	return nil
}

func hashSummary(value string) string {
	const summaryLength = 12
	if len(value) <= summaryLength {
		return value
	}
	return value[:summaryLength]
}

// approvalVersionForChangeSetState maps the current lifecycle version back to
// the immutable ChangeSet version that was approved. Execute and rollback only
// advance lifecycle state; they do not alter the approved action payload.
func approvalVersionForChangeSetState(status ChangeSetStatus, currentVersion int64) (int64, bool) {
	approvalVersion := currentVersion
	switch status {
	case ChangeSetApproved:
	case ChangeSetExecuted:
		approvalVersion--
	case ChangeSetRolledBack:
		approvalVersion -= 2
	default:
		return 0, false
	}
	if approvalVersion < 1 {
		return 0, false
	}
	return approvalVersion, true
}
