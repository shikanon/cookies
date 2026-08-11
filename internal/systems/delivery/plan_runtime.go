package delivery

import (
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

func finalizeIntentInput(value DeliveryIntent, actor contract.Principal, now time.Time) (DeliveryIntent, error) {
	declaredHash := strings.TrimSpace(value.CanonicalHash)
	if value.HashAlgorithm == "" {
		value.HashAlgorithm = CanonicalPayloadHashAlgorithm
	}
	value.Audit = ContractAuditMetadata{CreatedBy: actor.ID, CreatedAt: now}
	computed, err := value.ComputeCanonicalHash()
	if err != nil {
		return DeliveryIntent{}, err
	}
	if declaredHash != "" && declaredHash != computed {
		return DeliveryIntent{}, contractFailure(ContractErrorCanonicalHashMismatch, "intent.canonical_hash", "declared delivery intent hash does not match the server canonical payload")
	}
	value.CanonicalHash = computed
	if err := value.Validate(); err != nil {
		return DeliveryIntent{}, err
	}
	return value, nil
}

func finalizeConfigurationInput(value PlatformConfiguration, intent DeliveryIntent, actor contract.Principal, now time.Time) (PlatformConfiguration, error) {
	binding := value.Intent
	if binding.SchemaVersion != "" && binding.SchemaVersion != intent.SchemaVersion ||
		binding.IntentID != "" && binding.IntentID != intent.IntentID ||
		binding.VersionNumber != 0 && binding.VersionNumber != intent.VersionNumber ||
		binding.CanonicalHash != "" && binding.CanonicalHash != intent.CanonicalHash {
		return PlatformConfiguration{}, contractFailure(ContractErrorInvalidIntent, "platform_configuration.intent", "declared configuration binding does not match the delivery intent")
	}
	value.Intent = IntentBinding{SchemaVersion: intent.SchemaVersion, IntentID: intent.IntentID, VersionNumber: intent.VersionNumber, CanonicalHash: intent.CanonicalHash}
	declaredHash := strings.TrimSpace(value.CanonicalHash)
	if value.HashAlgorithm == "" {
		value.HashAlgorithm = CanonicalPayloadHashAlgorithm
	}
	value.Audit = ContractAuditMetadata{CreatedBy: actor.ID, CreatedAt: now}
	computed, err := value.ComputeCanonicalHash()
	if err != nil {
		return PlatformConfiguration{}, err
	}
	if declaredHash != "" && declaredHash != computed {
		return PlatformConfiguration{}, contractFailure(ContractErrorCanonicalHashMismatch, "platform_configuration.canonical_hash", "declared platform configuration hash does not match the server canonical payload")
	}
	value.CanonicalHash = computed
	if err := value.validateStructure(); err != nil {
		return PlatformConfiguration{}, err
	}
	return value, nil
}

func newPlatformPlanVersion(planID string, actor contract.ActorContext, projectID contract.ProjectID, versionNumber int, intent DeliveryIntent, configuration PlatformConfiguration, now time.Time) (DeliveryPlanVersion, error) {
	finalIntent, err := finalizeIntentInput(intent, actor.Principal, now)
	if err != nil {
		return DeliveryPlanVersion{}, err
	}
	finalConfiguration, err := finalizeConfigurationInput(configuration, finalIntent, actor.Principal, now)
	if err != nil {
		return DeliveryPlanVersion{}, err
	}
	value := DeliveryPlanVersion{
		SchemaVersion: DeliveryPlanVersionSchemaV2, RuntimeStatus: PlanRuntimeActive,
		PlanID: planID, OrganizationID: actor.OrganizationID, ProjectID: projectID,
		VersionNumber: versionNumber, CanonicalHash: finalConfiguration.CanonicalHash,
		DeliveryIntent: &finalIntent, PlatformConfiguration: &finalConfiguration,
		Platform: string(finalConfiguration.Platform), Source: SourceMock,
		Scenario: ScenarioPlatformConfiguration, CreatedBy: actor.Principal, CreatedAt: now,
	}
	if finalConfiguration.Platform == DeliveryPlatformMagneticEngine {
		value.RuntimeStatus = PlanRuntimeCapabilityPending
		value.Scenario = ScenarioCapabilityPending
	}
	return value, nil
}

func planProjectionFromPlatformVersion(id string, actor contract.ActorContext, projectID contract.ProjectID, version DeliveryPlanVersion, now time.Time) DeliveryPlan {
	intent := version.DeliveryIntent
	configuration := version.PlatformConfiguration
	name := "平台投放配置"
	if configuration.Payload.OceanEngine != nil && configuration.Payload.OceanEngine.Project != nil {
		name = configuration.Payload.OceanEngine.Project.ProjectName
	} else if configuration.Platform == DeliveryPlatformMagneticEngine {
		name = "磁力引擎能力待补"
	}
	creativeID, creativeVersion, creativeHash := "unresolved-material", "1", ""
	if len(intent.Payload.MaterialReferences) > 0 {
		ref := intent.Payload.MaterialReferences[0]
		if ref.ID != "" {
			creativeID = ref.ID
		}
		if ref.Version != "" {
			creativeVersion = ref.Version
		}
		creativeHash = ref.ContentHash
	}
	return DeliveryPlan{
		ID: id, OrganizationID: actor.OrganizationID, ProjectID: projectID,
		CreativePackageID: creativeID, CreativePackageHash: creativeHash, CreativeVersionID: creativeVersion,
		Name: name, Objective: intent.Payload.MarketingObjective,
		BudgetCents: intent.Payload.BudgetBoundary.MaximumTotalMinor,
		StartAt:     intent.Payload.ScheduleBoundary.EarliestStart, EndAt: intent.Payload.ScheduleBoundary.LatestEnd,
		Status: DeliveryPlanDraft, Version: int64(version.VersionNumber), Platform: string(configuration.Platform), Source: SourceMock,
		Scenario: version.Scenario, CurrentVersionNumber: version.VersionNumber,
		CreatedBy: actor.Principal.ID, CreatedAt: now, UpdatedAt: now,
	}
}

func versionBudget(value DeliveryPlanVersion) Budget {
	if value.IsPlatformConfigurationV2() {
		return Budget{TotalMinor: value.DeliveryIntent.Payload.BudgetBoundary.MaximumTotalMinor, Currency: value.DeliveryIntent.Payload.BudgetBoundary.Currency}
	}
	return value.Budget
}

func versionName(value DeliveryPlanVersion) string {
	if value.IsPlatformConfigurationV2() && value.PlatformConfiguration.Payload.OceanEngine != nil && value.PlatformConfiguration.Payload.OceanEngine.Project != nil {
		return value.PlatformConfiguration.Payload.OceanEngine.Project.ProjectName
	}
	return value.Name
}

func versionObjective(value DeliveryPlanVersion) string {
	if value.IsPlatformConfigurationV2() {
		return value.DeliveryIntent.Payload.MarketingObjective
	}
	return value.Objective
}

func versionSchedule(value DeliveryPlanVersion) (time.Time, time.Time) {
	if value.IsPlatformConfigurationV2() {
		return value.DeliveryIntent.Payload.ScheduleBoundary.EarliestStart, value.DeliveryIntent.Payload.ScheduleBoundary.LatestEnd
	}
	return value.Schedule.StartAt, value.Schedule.EndAt
}
