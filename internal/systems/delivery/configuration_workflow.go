package delivery

import (
	"context"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type RecommendationStatus string

const (
	RecommendationProposed RecommendationStatus = "proposed"
	RecommendationAccepted RecommendationStatus = "accepted"
	RecommendationRejected RecommendationStatus = "rejected"
)

// DeliveryRecommendation preserves both historical snapshot formats for reads.
type DeliveryRecommendation struct {
	ID                  string                  `json:"id"`
	OrganizationID      contract.OrganizationID `json:"organization_id"`
	ProjectID           contract.ProjectID      `json:"project_id"`
	PlanID              string                  `json:"plan_id"`
	PlanVersion         int                     `json:"plan_version"`
	SimulationRunID     string                  `json:"simulation_run_id"`
	Fingerprint         string                  `json:"fingerprint"`
	BaseSnapshotHash    string                  `json:"base_snapshot_hash"`
	BaseSnapshot        *ThreeTierConfiguration `json:"base_snapshot,omitempty"`
	TargetSnapshot      *ThreeTierConfiguration `json:"target_snapshot,omitempty"`
	BaseConfiguration   *PlatformConfiguration  `json:"base_configuration,omitempty"`
	TargetConfiguration *PlatformConfiguration  `json:"target_configuration,omitempty"`
	TargetSnapshotHash  string                  `json:"target_snapshot_hash"`
	RuntimeStatus       string                  `json:"runtime_status,omitempty"`
	ReadOnly            bool                    `json:"read_only,omitempty"`
	Evidence            []string                `json:"evidence"`
	Action              string                  `json:"action"`
	Impact              string                  `json:"impact"`
	Risks               []string                `json:"risks"`
	Observation         string                  `json:"observation"`
	CooldownUntil       *time.Time              `json:"cooldown_until,omitempty"`
	Provenance          string                  `json:"provenance"`
	Status              RecommendationStatus    `json:"status"`
	Version             int64                   `json:"version"`
	IdempotencyKey      string                  `json:"-"`
	RequestHash         string                  `json:"-"`
	AcceptedChangeSetID string                  `json:"accepted_change_set_id,omitempty"`
	CreatedBy           string                  `json:"created_by"`
	CreatedAt           time.Time               `json:"created_at"`
	UpdatedAt           time.Time               `json:"updated_at"`
}

// ManualActionPackage and ManualActionInstruction are frozen audit DTOs. No
// runtime path creates or materializes them.
type ManualActionPackage struct {
	ID                          string                    `json:"id"`
	OrganizationID              contract.OrganizationID   `json:"organization_id"`
	ProjectID                   contract.ProjectID        `json:"project_id"`
	ChangeSetID                 string                    `json:"change_set_id"`
	TargetSnapshotHash          string                    `json:"target_snapshot_hash"`
	ConfigurationSchemaVersion  string                    `json:"configuration_schema_version,omitempty"`
	ConfigurationID             string                    `json:"configuration_id,omitempty"`
	ConfigurationVersion        int                       `json:"configuration_version,omitempty"`
	ConfigurationPlatform       DeliveryPlatform          `json:"configuration_platform,omitempty"`
	ConfigurationProfileVersion string                    `json:"configuration_profile_version,omitempty"`
	ConfigurationCanonicalHash  string                    `json:"configuration_canonical_hash,omitempty"`
	IntentSchemaVersion         string                    `json:"intent_schema_version,omitempty"`
	IntentID                    string                    `json:"intent_id,omitempty"`
	IntentVersion               int                       `json:"intent_version,omitempty"`
	IntentCanonicalHash         string                    `json:"intent_canonical_hash,omitempty"`
	ContentHash                 string                    `json:"content_hash"`
	RuntimeStatus               string                    `json:"runtime_status,omitempty"`
	ReadOnly                    bool                      `json:"read_only,omitempty"`
	Instructions                []ManualActionInstruction `json:"instructions"`
	ForbiddenActions            []string                  `json:"forbidden_actions"`
	Evidence                    []string                  `json:"evidence"`
	Provenance                  string                    `json:"provenance"`
	OptimizedPlanVersion        int                       `json:"optimized_plan_version"`
	OptimizedPlanHash           string                    `json:"optimized_plan_hash"`
	Source                      Source                    `json:"source"`
	Scenario                    string                    `json:"scenario"`
	CreatedAt                   time.Time                 `json:"created_at"`
}

type ManualActionInstruction struct {
	Layer                string         `json:"layer"`
	GroupID              string         `json:"group_id"`
	PlanID               string         `json:"plan_id"`
	CreativeID           string         `json:"creative_id"`
	FieldKey             string         `json:"field_key"`
	Effective            ThreeTierValue `json:"effective"`
	Source               string         `json:"source"`
	ConfirmationRequired bool           `json:"confirmation_required"`
	ExpectedResult       string         `json:"expected_result"`
	EvidenceRefs         []string       `json:"evidence_refs"`
}

type configurationWorkflowRepository interface {
	ListRecommendations(context.Context, contract.OrganizationID, contract.ProjectID, int) ([]DeliveryRecommendation, error)
	GetRecommendation(context.Context, contract.OrganizationID, contract.ProjectID, string) (DeliveryRecommendation, error)
}

type legacyManualActionPackageRepository interface {
	GetManualActionPackage(context.Context, contract.OrganizationID, contract.ProjectID, string) (ManualActionPackage, error)
}

func (s Service) configurationWorkflow() (configurationWorkflowRepository, error) {
	repository, ok := s.Repository.(configurationWorkflowRepository)
	if !ok {
		return nil, ErrUnsupportedConfigurationWorkflow
	}
	return repository, nil
}

// legacyThreeTierSnapshotHash is frozen for verifying historical snapshots.
// It must never be used to create a runtime object.
func legacyThreeTierSnapshotHash(configuration *ThreeTierConfiguration) (string, error) {
	if configuration == nil {
		return "", nil
	}
	return contract.CanonicalJSONHash(configuration)
}

func changeSetPreflightVersion(base DeliveryPlanVersion, changeSet ChangeSet) (DeliveryPlanVersion, error) {
	if !base.IsPlatformConfigurationV2() || changeSet.TargetSnapshot == nil || changeSet.LegacyTargetSnapshot != nil {
		return DeliveryPlanVersion{}, ErrLegacyConfigurationUnsupported
	}
	if err := changeSet.TargetSnapshot.validateStructure(); err != nil {
		return DeliveryPlanVersion{}, err
	}
	if changeSet.TargetSnapshot.CanonicalHash != changeSet.TargetSnapshotHash {
		return DeliveryPlanVersion{}, ErrApprovalContentMismatch
	}
	version := cloneVersion(base)
	version.PlatformConfiguration = cloneJSONPointer(changeSet.TargetSnapshot)
	version.CanonicalHash = changeSet.TargetSnapshotHash
	return version, nil
}

func (s Service) ListRecommendations(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, limit int) ([]DeliveryRecommendation, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return nil, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	repository, err := s.configurationWorkflow()
	if err != nil {
		return nil, err
	}
	return repository.ListRecommendations(ctx, actor.OrganizationID, projectID, normalizeLimit(limit))
}

func (s Service) GetRecommendation(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, id string) (DeliveryRecommendation, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return DeliveryRecommendation{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return DeliveryRecommendation{}, err
	}
	repository, err := s.configurationWorkflow()
	if err != nil {
		return DeliveryRecommendation{}, err
	}
	return repository.GetRecommendation(ctx, actor.OrganizationID, projectID, id)
}

func (s Service) GetManualActionPackage(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, changeSetID string) (ManualActionPackage, error) {
	if err := s.ready(actor, projectID, ScopeRead); err != nil {
		return ManualActionPackage{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ManualActionPackage{}, err
	}
	repository, ok := s.Repository.(legacyManualActionPackageRepository)
	if !ok {
		return ManualActionPackage{}, ErrUnsupportedConfigurationWorkflow
	}
	value, err := repository.GetManualActionPackage(ctx, actor.OrganizationID, projectID, changeSetID)
	if err != nil {
		return ManualActionPackage{}, err
	}
	value.RuntimeStatus, value.ReadOnly = PlanRuntimeLegacyUnsupported, true
	return value, nil
}
