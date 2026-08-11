package delivery

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

const (
	DeliveryIntentSchemaV1               = "delivery-intent/v1"
	PlatformConfigurationSchemaV2        = "delivery-platform-configuration/v2"
	OceanEngineConfigurationProfileV1    = "oceanengine-configuration/v1"
	MagneticEngineConfigurationProfileV1 = "magnetic-engine-configuration/v1"
	CanonicalPayloadHashAlgorithm        = "RFC8785-JCS-SHA256(canonical_payload)"
)

type DeliveryPlatform string

const (
	DeliveryPlatformOceanEngine    DeliveryPlatform = "ocean_engine"
	DeliveryPlatformMagneticEngine DeliveryPlatform = "magnetic_engine"
)

type ReferenceState string

const (
	ReferenceResolved   ReferenceState = "resolved"
	ReferenceUnresolved ReferenceState = "unresolved"
	ReferenceBlocked    ReferenceState = "blocked"
	ReferenceRedacted   ReferenceState = "redacted"
)

type ConfigurationGenerationKind string

const (
	ConfigurationGeneratedManually         ConfigurationGenerationKind = "manual"
	ConfigurationGeneratedByRule           ConfigurationGenerationKind = "rule"
	ConfigurationGeneratedByDecisionEngine ConfigurationGenerationKind = "decision_engine"
	ConfigurationGeneratedByImport         ConfigurationGenerationKind = "import"
)

type FactSource string

const (
	FactSourceMock         FactSource = "mock"
	FactSourceReplay       FactSource = "replay"
	FactSourceConnector    FactSource = "connector"
	FactSourcePageEvidence FactSource = "page_evidence"
)

type PlatformEvidenceState string

const (
	PlatformEvidenceObserved               PlatformEvidenceState = "observed"
	PlatformEvidenceSampleOnly             PlatformEvidenceState = "sample_only"
	PlatformEvidenceOperatorReviewed       PlatformEvidenceState = "operator_reviewed"
	PlatformEvidencePending                PlatformEvidenceState = "platform_pending"
	PlatformEvidenceBlockedByEventAsset    PlatformEvidenceState = "blocked_by_event_asset"
	PlatformEvidenceWriteValidationPending PlatformEvidenceState = "write_validation_pending"
)

const (
	ContractErrorUnknownSchemaVersion    = "UNKNOWN_SCHEMA_VERSION"
	ContractErrorUnknownProfileVersion   = "UNKNOWN_PROFILE_VERSION"
	ContractErrorPlatformProfileMismatch = "PLATFORM_PROFILE_MISMATCH"
	ContractErrorInvalidReference        = "INVALID_STABLE_REFERENCE"
	ContractErrorInvalidIntent           = "INVALID_DELIVERY_INTENT"
	ContractErrorInvalidConfiguration    = "INVALID_PLATFORM_CONFIGURATION"
	ContractErrorProjectRequired         = "OCEANENGINE_PROJECT_REQUIRED"
	ContractErrorInvalidPromotion        = "INVALID_OCEANENGINE_PROMOTION"
	ContractErrorCanonicalHashMismatch   = "CANONICAL_HASH_MISMATCH"
	ContractErrorCapabilityPending       = "CAPABILITY_PENDING"
)

type DeliveryContractError struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

func (e *DeliveryContractError) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s at %s: %s", e.Code, e.Field, e.Message)
}

func DeliveryContractErrorCode(err error) string {
	var contractErr *DeliveryContractError
	if errors.As(err, &contractErr) {
		return contractErr.Code
	}
	return ""
}

func contractFailure(code, field, message string) error {
	return &DeliveryContractError{Code: code, Field: field, Message: message}
}

func isLowercaseSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func isConfigurationGenerationKind(value ConfigurationGenerationKind) bool {
	switch value {
	case ConfigurationGeneratedManually, ConfigurationGeneratedByRule, ConfigurationGeneratedByDecisionEngine, ConfigurationGeneratedByImport:
		return true
	default:
		return false
	}
}

func isFactSource(value FactSource) bool {
	switch value {
	case FactSourceMock, FactSourceReplay, FactSourceConnector, FactSourcePageEvidence:
		return true
	default:
		return false
	}
}

func isPlatformEvidenceState(value PlatformEvidenceState) bool {
	switch value {
	case PlatformEvidenceObserved, PlatformEvidenceSampleOnly, PlatformEvidenceOperatorReviewed, PlatformEvidencePending, PlatformEvidenceBlockedByEventAsset, PlatformEvidenceWriteValidationPending:
		return true
	default:
		return false
	}
}

// StableReference identifies an upstream or platform-owned object without
// copying that object's business entity into Delivery. Display/evidence fields
// are audit metadata and are intentionally excluded from canonical payloads.
type StableReference struct {
	Namespace           string         `json:"namespace"`
	ObjectKind          string         `json:"object_kind"`
	Scope               string         `json:"scope"`
	ID                  string         `json:"id,omitempty"`
	Version             string         `json:"version,omitempty"`
	ContentHash         string         `json:"content_hash,omitempty"`
	State               ReferenceState `json:"state"`
	Reason              string         `json:"reason,omitempty"`
	DisplayNameSnapshot string         `json:"display_name_snapshot,omitempty"`
	EvidenceVersion     string         `json:"evidence_version,omitempty"`
}

type canonicalStableReference struct {
	Namespace   string         `json:"namespace"`
	ObjectKind  string         `json:"object_kind"`
	Scope       string         `json:"scope"`
	ID          string         `json:"id,omitempty"`
	Version     string         `json:"version,omitempty"`
	ContentHash string         `json:"content_hash,omitempty"`
	State       ReferenceState `json:"state"`
	Reason      string         `json:"reason,omitempty"`
}

func (r StableReference) canonical() canonicalStableReference {
	return canonicalStableReference{
		Namespace: strings.TrimSpace(r.Namespace), ObjectKind: strings.TrimSpace(r.ObjectKind), Scope: strings.TrimSpace(r.Scope),
		ID: strings.TrimSpace(r.ID), Version: strings.TrimSpace(r.Version), ContentHash: strings.TrimSpace(r.ContentHash),
		State: r.State, Reason: strings.TrimSpace(r.Reason),
	}
}

func (r StableReference) validate(field string) error {
	if strings.TrimSpace(r.Namespace) == "" || strings.TrimSpace(r.ObjectKind) == "" || strings.TrimSpace(r.Scope) == "" {
		return contractFailure(ContractErrorInvalidReference, field, "namespace, object_kind, and scope are required")
	}
	switch r.State {
	case ReferenceResolved:
		if strings.TrimSpace(r.ID) == "" {
			return contractFailure(ContractErrorInvalidReference, field+".id", "resolved references require a stable id")
		}
	case ReferenceUnresolved, ReferenceBlocked, ReferenceRedacted:
		if strings.TrimSpace(r.Reason) == "" {
			return contractFailure(ContractErrorInvalidReference, field+".reason", "non-resolved references require a reason")
		}
	default:
		return contractFailure(ContractErrorInvalidReference, field+".state", "unknown reference state")
	}
	return nil
}

type ConfigurationProvenance struct {
	Kind          ConfigurationGenerationKind `json:"kind"`
	GeneratorRef  string                      `json:"generator_ref,omitempty"`
	PolicyVersion string                      `json:"policy_version,omitempty"`
}

type FactProvenance struct {
	Source       FactSource `json:"source"`
	SnapshotRef  string     `json:"snapshot_ref,omitempty"`
	EvidenceRefs []string   `json:"evidence_refs,omitempty"`
	ObservedAt   time.Time  `json:"observed_at,omitempty"`
}

type ContractAuditMetadata struct {
	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

type PlatformFieldEvidence struct {
	Field  string                `json:"field"`
	State  PlatformEvidenceState `json:"state"`
	Reason string                `json:"reason,omitempty"`
}

type CompilationMetadata struct {
	FieldEvidence []PlatformFieldEvidence `json:"field_evidence,omitempty"`
	Steps         []string                `json:"steps,omitempty"`
	EvidenceRefs  []string                `json:"evidence_refs,omitempty"`
}

type IntentBudgetBoundary struct {
	Currency          string `json:"currency"`
	MinimumTotalMinor int64  `json:"minimum_total_minor"`
	MaximumTotalMinor int64  `json:"maximum_total_minor"`
	MinimumDailyMinor *int64 `json:"minimum_daily_minor,omitempty"`
	MaximumDailyMinor *int64 `json:"maximum_daily_minor,omitempty"`
}

type IntentScheduleBoundary struct {
	EarliestStart time.Time `json:"earliest_start"`
	LatestEnd     time.Time `json:"latest_end"`
	Timezone      string    `json:"timezone"`
}

type OptimizationDirection string

const (
	OptimizationMinimize OptimizationDirection = "minimize"
	OptimizationMaximize OptimizationDirection = "maximize"
)

type OptimizationPreference struct {
	Metric      string                `json:"metric"`
	Direction   OptimizationDirection `json:"direction"`
	TargetValue *float64              `json:"target_value,omitempty"`
	Unit        string                `json:"unit,omitempty"`
}

type IntentAudienceConstraints struct {
	IncludeReferences []StableReference `json:"include_references,omitempty"`
	ExcludeReferences []StableReference `json:"exclude_references,omitempty"`
	Constraints       []string          `json:"constraints,omitempty"`
}

type DeliveryIntentPayload struct {
	PayloadSchemaVersion    string                    `json:"payload_schema_version"`
	MarketingObjective      string                    `json:"marketing_objective"`
	BudgetBoundary          IntentBudgetBoundary      `json:"budget_boundary"`
	ScheduleBoundary        IntentScheduleBoundary    `json:"schedule_boundary"`
	OptimizationPreferences []OptimizationPreference  `json:"optimization_preferences"`
	ProductReferences       []StableReference         `json:"product_references,omitempty"`
	LandingPageReferences   []StableReference         `json:"landing_page_references,omitempty"`
	MaterialReferences      []StableReference         `json:"material_references"`
	AudienceConstraints     IntentAudienceConstraints `json:"audience_constraints"`
	StrategyReference       StableReference           `json:"strategy_reference"`
}

type DeliveryIntent struct {
	SchemaVersion           string                  `json:"schema_version"`
	IntentID                string                  `json:"intent_id"`
	VersionNumber           int                     `json:"version_number"`
	HashAlgorithm           string                  `json:"hash_algorithm"`
	CanonicalHash           string                  `json:"canonical_hash"`
	Payload                 DeliveryIntentPayload   `json:"payload"`
	ConfigurationProvenance ConfigurationProvenance `json:"configuration_provenance"`
	FactProvenance          FactProvenance          `json:"fact_provenance"`
	Audit                   ContractAuditMetadata   `json:"audit"`
}

type canonicalDeliveryIntentPayload struct {
	PayloadSchemaVersion    string                       `json:"payload_schema_version"`
	MarketingObjective      string                       `json:"marketing_objective"`
	BudgetBoundary          IntentBudgetBoundary         `json:"budget_boundary"`
	ScheduleBoundary        IntentScheduleBoundary       `json:"schedule_boundary"`
	OptimizationPreferences []OptimizationPreference     `json:"optimization_preferences"`
	ProductReferences       []canonicalStableReference   `json:"product_references,omitempty"`
	LandingPageReferences   []canonicalStableReference   `json:"landing_page_references,omitempty"`
	MaterialReferences      []canonicalStableReference   `json:"material_references"`
	AudienceConstraints     canonicalAudienceConstraints `json:"audience_constraints"`
	StrategyReference       canonicalStableReference     `json:"strategy_reference"`
}

type canonicalAudienceConstraints struct {
	IncludeReferences []canonicalStableReference `json:"include_references,omitempty"`
	ExcludeReferences []canonicalStableReference `json:"exclude_references,omitempty"`
	Constraints       []string                   `json:"constraints,omitempty"`
}

func canonicalReferences(values []StableReference) []canonicalStableReference {
	if values == nil {
		return nil
	}
	out := make([]canonicalStableReference, len(values))
	for i := range values {
		out[i] = values[i].canonical()
	}
	return out
}

func (i DeliveryIntent) CanonicalPayload() any {
	return canonicalDeliveryIntentPayload{
		PayloadSchemaVersion:    strings.TrimSpace(i.Payload.PayloadSchemaVersion),
		MarketingObjective:      strings.TrimSpace(i.Payload.MarketingObjective),
		BudgetBoundary:          i.Payload.BudgetBoundary,
		ScheduleBoundary:        i.Payload.ScheduleBoundary,
		OptimizationPreferences: append([]OptimizationPreference(nil), i.Payload.OptimizationPreferences...),
		ProductReferences:       canonicalReferences(i.Payload.ProductReferences),
		LandingPageReferences:   canonicalReferences(i.Payload.LandingPageReferences),
		MaterialReferences:      canonicalReferences(i.Payload.MaterialReferences),
		AudienceConstraints: canonicalAudienceConstraints{
			IncludeReferences: canonicalReferences(i.Payload.AudienceConstraints.IncludeReferences),
			ExcludeReferences: canonicalReferences(i.Payload.AudienceConstraints.ExcludeReferences),
			Constraints:       append([]string(nil), i.Payload.AudienceConstraints.Constraints...),
		},
		StrategyReference: i.Payload.StrategyReference.canonical(),
	}
}

func (i DeliveryIntent) ComputeCanonicalHash() (string, error) {
	return contract.CanonicalJSONHash(i.CanonicalPayload())
}

func FinalizeDeliveryIntent(value DeliveryIntent) (DeliveryIntent, error) {
	hash, err := value.ComputeCanonicalHash()
	if err != nil {
		return DeliveryIntent{}, err
	}
	value.CanonicalHash = hash
	if err := value.Validate(); err != nil {
		return DeliveryIntent{}, err
	}
	return value, nil
}

func (i DeliveryIntent) Validate() error {
	if i.SchemaVersion != DeliveryIntentSchemaV1 || i.Payload.PayloadSchemaVersion != DeliveryIntentSchemaV1 {
		return contractFailure(ContractErrorUnknownSchemaVersion, "schema_version", "delivery intent must use delivery-intent/v1")
	}
	if strings.TrimSpace(i.IntentID) == "" || i.VersionNumber < 1 || strings.TrimSpace(i.Payload.MarketingObjective) == "" {
		return contractFailure(ContractErrorInvalidIntent, "intent", "intent_id, positive version_number, and marketing_objective are required")
	}
	if i.HashAlgorithm != CanonicalPayloadHashAlgorithm {
		return contractFailure(ContractErrorInvalidIntent, "hash_algorithm", "unsupported canonical hash algorithm")
	}
	if !isConfigurationGenerationKind(i.ConfigurationProvenance.Kind) || !isFactSource(i.FactProvenance.Source) {
		return contractFailure(ContractErrorInvalidIntent, "provenance", "configuration generation kind and fact source must use their independent supported vocabularies")
	}
	budget := i.Payload.BudgetBoundary
	if strings.TrimSpace(budget.Currency) == "" || budget.MinimumTotalMinor < 0 || budget.MaximumTotalMinor < budget.MinimumTotalMinor {
		return contractFailure(ContractErrorInvalidIntent, "payload.budget_boundary", "currency and an ordered non-negative total budget boundary are required")
	}
	if (budget.MinimumDailyMinor == nil) != (budget.MaximumDailyMinor == nil) {
		return contractFailure(ContractErrorInvalidIntent, "payload.budget_boundary", "daily budget boundaries must be both present or both absent")
	}
	if budget.MinimumDailyMinor != nil && (*budget.MinimumDailyMinor < 0 || *budget.MaximumDailyMinor < *budget.MinimumDailyMinor) {
		return contractFailure(ContractErrorInvalidIntent, "payload.budget_boundary", "daily budget boundary is invalid")
	}
	schedule := i.Payload.ScheduleBoundary
	if schedule.EarliestStart.IsZero() || !schedule.LatestEnd.After(schedule.EarliestStart) || strings.TrimSpace(schedule.Timezone) == "" {
		return contractFailure(ContractErrorInvalidIntent, "payload.schedule_boundary", "schedule requires a timezone and latest_end after earliest_start")
	}
	for index, preference := range i.Payload.OptimizationPreferences {
		if strings.TrimSpace(preference.Metric) == "" || (preference.Direction != OptimizationMinimize && preference.Direction != OptimizationMaximize) {
			return contractFailure(ContractErrorInvalidIntent, fmt.Sprintf("payload.optimization_preferences.%d", index), "metric and direction are required")
		}
		if preference.TargetValue != nil && (math.IsInf(*preference.TargetValue, 0) || math.IsNaN(*preference.TargetValue)) {
			return contractFailure(ContractErrorInvalidIntent, fmt.Sprintf("payload.optimization_preferences.%d.target_value", index), "target value must be finite")
		}
	}
	if err := validateReferenceSlices(i.Payload.ProductReferences, "payload.product_references"); err != nil {
		return err
	}
	if err := validateReferenceSlices(i.Payload.LandingPageReferences, "payload.landing_page_references"); err != nil {
		return err
	}
	if len(i.Payload.MaterialReferences) == 0 {
		return contractFailure(ContractErrorInvalidIntent, "payload.material_references", "at least one material reference is required")
	}
	if err := validateReferenceSlices(i.Payload.MaterialReferences, "payload.material_references"); err != nil {
		return err
	}
	if err := validateReferenceSlices(i.Payload.AudienceConstraints.IncludeReferences, "payload.audience_constraints.include_references"); err != nil {
		return err
	}
	if err := validateReferenceSlices(i.Payload.AudienceConstraints.ExcludeReferences, "payload.audience_constraints.exclude_references"); err != nil {
		return err
	}
	if err := i.Payload.StrategyReference.validate("payload.strategy_reference"); err != nil {
		return err
	}
	hash, err := i.ComputeCanonicalHash()
	if err != nil {
		return err
	}
	if i.CanonicalHash == "" || i.CanonicalHash != hash {
		return contractFailure(ContractErrorCanonicalHashMismatch, "canonical_hash", "delivery intent hash does not match its canonical payload")
	}
	return nil
}

func validateReferenceSlices(values []StableReference, field string) error {
	for index := range values {
		if err := values[index].validate(fmt.Sprintf("%s.%d", field, index)); err != nil {
			return err
		}
	}
	return nil
}

type IntentBinding struct {
	SchemaVersion string `json:"schema_version"`
	IntentID      string `json:"intent_id"`
	VersionNumber int    `json:"version_number"`
	CanonicalHash string `json:"canonical_hash"`
}

type OceanEngineTargeting struct {
	AudiencePackageReference *StableReference `json:"audience_package_reference,omitempty"`
	Regions                  []string         `json:"regions,omitempty"`
	AgeRanges                []string         `json:"age_ranges,omitempty"`
	Gender                   string           `json:"gender,omitempty"`
	SmartExpansion           bool             `json:"smart_expansion"`
}

type OceanEngineSchedule struct {
	StartAt  time.Time `json:"start_at"`
	EndAt    time.Time `json:"end_at"`
	Timezone string    `json:"timezone"`
}

type OceanEngineBudgetAndBidding struct {
	Currency         string   `json:"currency"`
	DailyBudgetMinor int64    `json:"daily_budget_minor"`
	BiddingStrategy  string   `json:"bidding_strategy"`
	ChargingMode     string   `json:"charging_mode"`
	BidMinor         *int64   `json:"bid_minor,omitempty"`
	ROICoefficient   *float64 `json:"roi_coefficient,omitempty"`
}

type OceanEngineProjectDraft struct {
	DraftSchemaVersion          string                      `json:"draft_schema_version"`
	ProjectDraftID              string                      `json:"project_draft_id"`
	AccountReference            StableReference             `json:"account_reference"`
	MarketingPurpose            string                      `json:"marketing_purpose"`
	MarketingScenario           string                      `json:"marketing_scenario"`
	MarketingProductReference   *StableReference            `json:"marketing_product_reference,omitempty"`
	ApplicationReference        *StableReference            `json:"application_reference,omitempty"`
	Carrier                     string                      `json:"carrier"`
	OptimizationTargetReference *StableReference            `json:"optimization_target_reference,omitempty"`
	DeepOptimizationMode        string                      `json:"deep_optimization_mode,omitempty"`
	DeliveryMode                string                      `json:"delivery_mode"`
	Targeting                   OceanEngineTargeting        `json:"targeting"`
	Schedule                    OceanEngineSchedule         `json:"schedule"`
	BudgetAndBidding            OceanEngineBudgetAndBidding `json:"budget_and_bidding"`
	MonitoringReferences        []StableReference           `json:"monitoring_references,omitempty"`
	ProjectName                 string                      `json:"project_name"`
}

type OceanEngineDeliveryIdentity struct {
	Mode               string           `json:"mode"`
	AuthorizedIdentity *StableReference `json:"authorized_identity,omitempty"`
}

type OceanEngineCopyItem struct {
	Text string `json:"text"`
}

type OceanEnginePromotionSettings struct {
	CallToAction      string           `json:"call_to_action,omitempty"`
	SourceLabel       string           `json:"source_label,omitempty"`
	CommentsEnabled   *bool            `json:"comments_enabled,omitempty"`
	CategoryReference *StableReference `json:"category_reference,omitempty"`
	BrandReference    *StableReference `json:"brand_reference,omitempty"`
}

type OceanEnginePromotionDraft struct {
	DraftSchemaVersion          string                       `json:"draft_schema_version"`
	PromotionDraftID            string                       `json:"promotion_draft_id"`
	DeliveryIdentity            OceanEngineDeliveryIdentity  `json:"delivery_identity"`
	BaseMaterialReferences      []StableReference            `json:"base_material_references"`
	CopyItems                   []OceanEngineCopyItem        `json:"copy_items"`
	NativeAnchorReference       *StableReference             `json:"native_anchor_reference,omitempty"`
	LandingPageReference        *StableReference             `json:"landing_page_reference,omitempty"`
	DirectLinkReference         *StableReference             `json:"direct_link_reference,omitempty"`
	ProductReference            *StableReference             `json:"product_reference,omitempty"`
	CreativeComponentReferences []StableReference            `json:"creative_component_references,omitempty"`
	Settings                    OceanEnginePromotionSettings `json:"settings"`
	PromotionName               string                       `json:"promotion_name"`
}

type OceanEngineConfiguration struct {
	Profile    DeliveryPlatform            `json:"profile"`
	Project    *OceanEngineProjectDraft    `json:"project"`
	Promotions []OceanEnginePromotionDraft `json:"promotions"`
}

type MagneticEngineConfiguration struct {
	Profile    DeliveryPlatform `json:"profile"`
	Status     string           `json:"status"`
	ReasonCode string           `json:"reason_code"`
	Reason     string           `json:"reason"`
}

type PlatformConfigurationPayload struct {
	Profile        DeliveryPlatform             `json:"profile"`
	OceanEngine    *OceanEngineConfiguration    `json:"ocean_engine,omitempty"`
	MagneticEngine *MagneticEngineConfiguration `json:"magnetic_engine,omitempty"`
}

type PlatformConfiguration struct {
	SchemaVersion           string                       `json:"schema_version"`
	ConfigurationID         string                       `json:"configuration_id"`
	VersionNumber           int                          `json:"version_number"`
	Platform                DeliveryPlatform             `json:"platform"`
	ProfileVersion          string                       `json:"profile_version"`
	Intent                  IntentBinding                `json:"intent"`
	HashAlgorithm           string                       `json:"hash_algorithm"`
	CanonicalHash           string                       `json:"canonical_hash"`
	Payload                 PlatformConfigurationPayload `json:"payload"`
	ConfigurationProvenance ConfigurationProvenance      `json:"configuration_provenance"`
	FactProvenance          FactProvenance               `json:"fact_provenance"`
	Audit                   ContractAuditMetadata        `json:"audit"`
	CompilationMetadata     CompilationMetadata          `json:"compilation_metadata"`
}

type canonicalOceanEngineTargeting struct {
	AudiencePackageReference *canonicalStableReference `json:"audience_package_reference,omitempty"`
	Regions                  []string                  `json:"regions,omitempty"`
	AgeRanges                []string                  `json:"age_ranges,omitempty"`
	Gender                   string                    `json:"gender,omitempty"`
	SmartExpansion           bool                      `json:"smart_expansion"`
}

type canonicalOceanEngineProject struct {
	DraftSchemaVersion          string                        `json:"draft_schema_version"`
	ProjectDraftID              string                        `json:"project_draft_id"`
	AccountReference            canonicalStableReference      `json:"account_reference"`
	MarketingPurpose            string                        `json:"marketing_purpose"`
	MarketingScenario           string                        `json:"marketing_scenario"`
	MarketingProductReference   *canonicalStableReference     `json:"marketing_product_reference,omitempty"`
	ApplicationReference        *canonicalStableReference     `json:"application_reference,omitempty"`
	Carrier                     string                        `json:"carrier"`
	OptimizationTargetReference *canonicalStableReference     `json:"optimization_target_reference,omitempty"`
	DeepOptimizationMode        string                        `json:"deep_optimization_mode,omitempty"`
	DeliveryMode                string                        `json:"delivery_mode"`
	Targeting                   canonicalOceanEngineTargeting `json:"targeting"`
	Schedule                    OceanEngineSchedule           `json:"schedule"`
	BudgetAndBidding            OceanEngineBudgetAndBidding   `json:"budget_and_bidding"`
	MonitoringReferences        []canonicalStableReference    `json:"monitoring_references,omitempty"`
	ProjectName                 string                        `json:"project_name"`
}

type canonicalOceanEnginePromotionSettings struct {
	CallToAction      string                    `json:"call_to_action,omitempty"`
	SourceLabel       string                    `json:"source_label,omitempty"`
	CommentsEnabled   *bool                     `json:"comments_enabled,omitempty"`
	CategoryReference *canonicalStableReference `json:"category_reference,omitempty"`
	BrandReference    *canonicalStableReference `json:"brand_reference,omitempty"`
}

type canonicalOceanEnginePromotion struct {
	DraftSchemaVersion          string                                `json:"draft_schema_version"`
	PromotionDraftID            string                                `json:"promotion_draft_id"`
	DeliveryIdentity            canonicalOceanEngineDeliveryIdentity  `json:"delivery_identity"`
	BaseMaterialReferences      []canonicalStableReference            `json:"base_material_references"`
	CopyItems                   []OceanEngineCopyItem                 `json:"copy_items"`
	NativeAnchorReference       *canonicalStableReference             `json:"native_anchor_reference,omitempty"`
	LandingPageReference        *canonicalStableReference             `json:"landing_page_reference,omitempty"`
	DirectLinkReference         *canonicalStableReference             `json:"direct_link_reference,omitempty"`
	ProductReference            *canonicalStableReference             `json:"product_reference,omitempty"`
	CreativeComponentReferences []canonicalStableReference            `json:"creative_component_references,omitempty"`
	Settings                    canonicalOceanEnginePromotionSettings `json:"settings"`
	PromotionName               string                                `json:"promotion_name"`
}

type canonicalOceanEngineDeliveryIdentity struct {
	Mode               string                    `json:"mode"`
	AuthorizedIdentity *canonicalStableReference `json:"authorized_identity,omitempty"`
}

type canonicalOceanEngineConfiguration struct {
	Profile    DeliveryPlatform                `json:"profile"`
	Project    *canonicalOceanEngineProject    `json:"project"`
	Promotions []canonicalOceanEnginePromotion `json:"promotions"`
}

type canonicalPlatformConfigurationPayload struct {
	SchemaVersion  string                             `json:"schema_version"`
	Platform       DeliveryPlatform                   `json:"platform"`
	ProfileVersion string                             `json:"profile_version"`
	Intent         IntentBinding                      `json:"intent"`
	Profile        DeliveryPlatform                   `json:"profile"`
	OceanEngine    *canonicalOceanEngineConfiguration `json:"ocean_engine,omitempty"`
	MagneticEngine *MagneticEngineConfiguration       `json:"magnetic_engine,omitempty"`
}

func canonicalReferencePointer(value *StableReference) *canonicalStableReference {
	if value == nil {
		return nil
	}
	canonical := value.canonical()
	return &canonical
}

func canonicalOceanConfiguration(value *OceanEngineConfiguration) *canonicalOceanEngineConfiguration {
	if value == nil {
		return nil
	}
	out := &canonicalOceanEngineConfiguration{Profile: value.Profile, Promotions: make([]canonicalOceanEnginePromotion, len(value.Promotions))}
	if value.Project != nil {
		project := value.Project
		out.Project = &canonicalOceanEngineProject{
			DraftSchemaVersion: strings.TrimSpace(project.DraftSchemaVersion), ProjectDraftID: strings.TrimSpace(project.ProjectDraftID),
			AccountReference: project.AccountReference.canonical(), MarketingPurpose: strings.TrimSpace(project.MarketingPurpose),
			MarketingScenario: strings.TrimSpace(project.MarketingScenario), MarketingProductReference: canonicalReferencePointer(project.MarketingProductReference),
			ApplicationReference: canonicalReferencePointer(project.ApplicationReference), Carrier: strings.TrimSpace(project.Carrier),
			OptimizationTargetReference: canonicalReferencePointer(project.OptimizationTargetReference), DeepOptimizationMode: strings.TrimSpace(project.DeepOptimizationMode),
			DeliveryMode: strings.TrimSpace(project.DeliveryMode), Schedule: project.Schedule, BudgetAndBidding: project.BudgetAndBidding,
			MonitoringReferences: canonicalReferences(project.MonitoringReferences), ProjectName: strings.TrimSpace(project.ProjectName),
			Targeting: canonicalOceanEngineTargeting{
				AudiencePackageReference: canonicalReferencePointer(project.Targeting.AudiencePackageReference),
				Regions:                  append([]string(nil), project.Targeting.Regions...), AgeRanges: append([]string(nil), project.Targeting.AgeRanges...),
				Gender: strings.TrimSpace(project.Targeting.Gender), SmartExpansion: project.Targeting.SmartExpansion,
			},
		}
	}
	for index := range value.Promotions {
		promotion := value.Promotions[index]
		out.Promotions[index] = canonicalOceanEnginePromotion{
			DraftSchemaVersion: strings.TrimSpace(promotion.DraftSchemaVersion), PromotionDraftID: strings.TrimSpace(promotion.PromotionDraftID),
			DeliveryIdentity:       canonicalOceanEngineDeliveryIdentity{Mode: strings.TrimSpace(promotion.DeliveryIdentity.Mode), AuthorizedIdentity: canonicalReferencePointer(promotion.DeliveryIdentity.AuthorizedIdentity)},
			BaseMaterialReferences: canonicalReferences(promotion.BaseMaterialReferences), CopyItems: append([]OceanEngineCopyItem(nil), promotion.CopyItems...),
			NativeAnchorReference: canonicalReferencePointer(promotion.NativeAnchorReference), LandingPageReference: canonicalReferencePointer(promotion.LandingPageReference),
			DirectLinkReference: canonicalReferencePointer(promotion.DirectLinkReference), ProductReference: canonicalReferencePointer(promotion.ProductReference),
			CreativeComponentReferences: canonicalReferences(promotion.CreativeComponentReferences), PromotionName: strings.TrimSpace(promotion.PromotionName),
			Settings: canonicalOceanEnginePromotionSettings{
				CallToAction: strings.TrimSpace(promotion.Settings.CallToAction), SourceLabel: strings.TrimSpace(promotion.Settings.SourceLabel), CommentsEnabled: promotion.Settings.CommentsEnabled,
				CategoryReference: canonicalReferencePointer(promotion.Settings.CategoryReference), BrandReference: canonicalReferencePointer(promotion.Settings.BrandReference),
			},
		}
	}
	return out
}

func (c PlatformConfiguration) CanonicalPayload() any {
	return canonicalPlatformConfigurationPayload{
		SchemaVersion: strings.TrimSpace(c.SchemaVersion), Platform: c.Platform, ProfileVersion: strings.TrimSpace(c.ProfileVersion), Intent: c.Intent,
		Profile: c.Payload.Profile, OceanEngine: canonicalOceanConfiguration(c.Payload.OceanEngine), MagneticEngine: c.Payload.MagneticEngine,
	}
}

func (c PlatformConfiguration) ComputeCanonicalHash() (string, error) {
	return contract.CanonicalJSONHash(c.CanonicalPayload())
}

// FinalizePlatformConfiguration computes the immutable business hash and
// validates the tagged structure. A Magnetic Engine capability-pending profile
// can be finalized and stored, but Validate still returns CAPABILITY_PENDING to
// callers asking whether it is executable.
func FinalizePlatformConfiguration(value PlatformConfiguration) (PlatformConfiguration, error) {
	hash, err := value.ComputeCanonicalHash()
	if err != nil {
		return PlatformConfiguration{}, err
	}
	value.CanonicalHash = hash
	if err := value.validateStructure(); err != nil {
		return PlatformConfiguration{}, err
	}
	return value, nil
}

func (c PlatformConfiguration) validateStructure() error {
	if c.SchemaVersion != PlatformConfigurationSchemaV2 {
		return contractFailure(ContractErrorUnknownSchemaVersion, "schema_version", "platform configuration must use delivery-platform-configuration/v2")
	}
	if strings.TrimSpace(c.ConfigurationID) == "" || c.VersionNumber < 1 || c.HashAlgorithm != CanonicalPayloadHashAlgorithm {
		return contractFailure(ContractErrorInvalidConfiguration, "configuration", "configuration_id, positive version_number, and the canonical payload hash algorithm are required")
	}
	if !isConfigurationGenerationKind(c.ConfigurationProvenance.Kind) || !isFactSource(c.FactProvenance.Source) {
		return contractFailure(ContractErrorInvalidConfiguration, "provenance", "configuration generation kind and fact source must use their independent supported vocabularies")
	}
	for index, evidence := range c.CompilationMetadata.FieldEvidence {
		if strings.TrimSpace(evidence.Field) == "" || !isPlatformEvidenceState(evidence.State) {
			return contractFailure(ContractErrorInvalidConfiguration, fmt.Sprintf("compilation_metadata.field_evidence.%d", index), "field evidence requires a field and supported evidence state")
		}
	}
	if c.Intent.SchemaVersion != DeliveryIntentSchemaV1 || strings.TrimSpace(c.Intent.IntentID) == "" || c.Intent.VersionNumber < 1 || !isLowercaseSHA256(c.Intent.CanonicalHash) {
		return contractFailure(ContractErrorInvalidIntent, "intent", "configuration must bind a delivery-intent/v1 id, version, and canonical hash")
	}
	if c.Platform != c.Payload.Profile {
		return contractFailure(ContractErrorPlatformProfileMismatch, "payload.profile", "platform discriminator does not match payload profile")
	}
	switch c.Platform {
	case DeliveryPlatformOceanEngine:
		if c.ProfileVersion != OceanEngineConfigurationProfileV1 {
			return contractFailure(ContractErrorUnknownProfileVersion, "profile_version", "unknown OceanEngine profile version")
		}
		if c.Payload.OceanEngine == nil || c.Payload.MagneticEngine != nil || c.Payload.OceanEngine.Profile != DeliveryPlatformOceanEngine {
			return contractFailure(ContractErrorPlatformProfileMismatch, "payload.ocean_engine", "OceanEngine discriminator requires exactly one OceanEngine profile")
		}
		if err := validateOceanEngineConfiguration(*c.Payload.OceanEngine); err != nil {
			return err
		}
	case DeliveryPlatformMagneticEngine:
		if c.ProfileVersion != MagneticEngineConfigurationProfileV1 {
			return contractFailure(ContractErrorUnknownProfileVersion, "profile_version", "unknown Magnetic Engine profile version")
		}
		profile := c.Payload.MagneticEngine
		if profile == nil || c.Payload.OceanEngine != nil || profile.Profile != DeliveryPlatformMagneticEngine {
			return contractFailure(ContractErrorPlatformProfileMismatch, "payload.magnetic_engine", "Magnetic Engine discriminator requires exactly one capability profile")
		}
		if profile.Status != "capability_pending" || profile.ReasonCode != ContractErrorCapabilityPending || strings.TrimSpace(profile.Reason) == "" {
			return contractFailure(ContractErrorCapabilityPending, "payload.magnetic_engine", "Magnetic Engine must remain capability_pending until evidence is available")
		}
	default:
		return contractFailure(ContractErrorPlatformProfileMismatch, "platform", "unknown platform discriminator")
	}
	hash, err := c.ComputeCanonicalHash()
	if err != nil {
		return err
	}
	if c.CanonicalHash == "" || c.CanonicalHash != hash {
		return contractFailure(ContractErrorCanonicalHashMismatch, "canonical_hash", "platform configuration hash does not match its canonical payload")
	}
	return nil
}

func (c PlatformConfiguration) Validate() error {
	if err := c.validateStructure(); err != nil {
		return err
	}
	if c.Platform == DeliveryPlatformMagneticEngine {
		return contractFailure(ContractErrorCapabilityPending, "platform", c.Payload.MagneticEngine.Reason)
	}
	return nil
}

func validateOceanEngineConfiguration(configuration OceanEngineConfiguration) error {
	if configuration.Project == nil {
		return contractFailure(ContractErrorProjectRequired, "payload.ocean_engine.project", "one OceanEngine project is required")
	}
	project := configuration.Project
	if project.DraftSchemaVersion != OceanEngineConfigurationProfileV1 || strings.TrimSpace(project.ProjectDraftID) == "" || strings.TrimSpace(project.MarketingPurpose) == "" || strings.TrimSpace(project.MarketingScenario) == "" || strings.TrimSpace(project.Carrier) == "" || strings.TrimSpace(project.DeliveryMode) == "" || strings.TrimSpace(project.ProjectName) == "" {
		return contractFailure(ContractErrorProjectRequired, "payload.ocean_engine.project", "project profile version, id, marketing path, delivery mode, and name are required")
	}
	if err := project.AccountReference.validate("payload.ocean_engine.project.account_reference"); err != nil {
		return err
	}
	optionalProjectReferences := []struct {
		field     string
		reference *StableReference
	}{
		{"marketing_product_reference", project.MarketingProductReference},
		{"application_reference", project.ApplicationReference},
		{"optimization_target_reference", project.OptimizationTargetReference},
		{"targeting.audience_package_reference", project.Targeting.AudiencePackageReference},
	}
	for _, candidate := range optionalProjectReferences {
		if candidate.reference != nil {
			if err := candidate.reference.validate("payload.ocean_engine.project." + candidate.field); err != nil {
				return err
			}
		}
	}
	if err := validateReferenceSlices(project.MonitoringReferences, "payload.ocean_engine.project.monitoring_references"); err != nil {
		return err
	}
	if project.Schedule.StartAt.IsZero() || !project.Schedule.EndAt.After(project.Schedule.StartAt) || strings.TrimSpace(project.Schedule.Timezone) == "" {
		return contractFailure(ContractErrorProjectRequired, "payload.ocean_engine.project.schedule", "schedule requires timezone and end_at after start_at")
	}
	if strings.TrimSpace(project.BudgetAndBidding.Currency) == "" || project.BudgetAndBidding.DailyBudgetMinor < 0 || strings.TrimSpace(project.BudgetAndBidding.BiddingStrategy) == "" || strings.TrimSpace(project.BudgetAndBidding.ChargingMode) == "" {
		return contractFailure(ContractErrorProjectRequired, "payload.ocean_engine.project.budget_and_bidding", "currency, non-negative budget, bidding strategy, and charging mode are required")
	}
	if (project.BudgetAndBidding.BidMinor == nil) == (project.BudgetAndBidding.ROICoefficient == nil) {
		return contractFailure(ContractErrorProjectRequired, "payload.ocean_engine.project.budget_and_bidding", "exactly one of bid_minor or roi_coefficient is required")
	}
	if project.BudgetAndBidding.BidMinor != nil && *project.BudgetAndBidding.BidMinor < 0 {
		return contractFailure(ContractErrorProjectRequired, "payload.ocean_engine.project.budget_and_bidding.bid_minor", "bid must not be negative")
	}
	if project.BudgetAndBidding.ROICoefficient != nil && (*project.BudgetAndBidding.ROICoefficient < 0 || math.IsInf(*project.BudgetAndBidding.ROICoefficient, 0) || math.IsNaN(*project.BudgetAndBidding.ROICoefficient)) {
		return contractFailure(ContractErrorProjectRequired, "payload.ocean_engine.project.budget_and_bidding.roi_coefficient", "ROI coefficient must be finite and non-negative")
	}
	seen := map[string]bool{}
	for index := range configuration.Promotions {
		promotion := configuration.Promotions[index]
		field := fmt.Sprintf("payload.ocean_engine.promotions.%d", index)
		if promotion.DraftSchemaVersion != OceanEngineConfigurationProfileV1 || strings.TrimSpace(promotion.PromotionDraftID) == "" || seen[promotion.PromotionDraftID] || strings.TrimSpace(promotion.PromotionName) == "" {
			return contractFailure(ContractErrorInvalidPromotion, field, "promotion profile version, unique id, and name are required")
		}
		seen[promotion.PromotionDraftID] = true
		if promotion.DeliveryIdentity.Mode != "account_info" && promotion.DeliveryIdentity.Mode != "douyin_account" {
			return contractFailure(ContractErrorInvalidPromotion, field+".delivery_identity.mode", "identity must be account_info or douyin_account")
		}
		if promotion.DeliveryIdentity.Mode == "account_info" && promotion.DeliveryIdentity.AuthorizedIdentity != nil {
			return contractFailure(ContractErrorInvalidPromotion, field+".delivery_identity", "account_info must not carry an authorized identity")
		}
		if promotion.DeliveryIdentity.Mode == "douyin_account" {
			if promotion.DeliveryIdentity.AuthorizedIdentity == nil {
				return contractFailure(ContractErrorInvalidPromotion, field+".delivery_identity", "douyin_account requires an authorized identity reference")
			}
			if err := promotion.DeliveryIdentity.AuthorizedIdentity.validate(field + ".delivery_identity.authorized_identity"); err != nil {
				return err
			}
		}
		if err := validateReferenceSlices(promotion.BaseMaterialReferences, field+".base_material_references"); err != nil {
			return err
		}
		optionalPromotionReferences := []struct {
			field     string
			reference *StableReference
		}{
			{"native_anchor_reference", promotion.NativeAnchorReference},
			{"landing_page_reference", promotion.LandingPageReference},
			{"direct_link_reference", promotion.DirectLinkReference},
			{"product_reference", promotion.ProductReference},
			{"settings.category_reference", promotion.Settings.CategoryReference},
			{"settings.brand_reference", promotion.Settings.BrandReference},
		}
		for _, candidate := range optionalPromotionReferences {
			if candidate.reference != nil {
				if err := candidate.reference.validate(field + "." + candidate.field); err != nil {
					return err
				}
			}
		}
		if err := validateReferenceSlices(promotion.CreativeComponentReferences, field+".creative_component_references"); err != nil {
			return err
		}
	}
	return nil
}
