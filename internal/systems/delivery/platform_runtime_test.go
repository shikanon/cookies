package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

func TestPlatformRuntimeCreateUpdateAndApprovalBindings(t *testing.T) {
	service, actor := newTestService()
	intent, configuration := readyOceanRuntimeInputs(t, 0)

	plan, err := service.CreatePlan(context.Background(), actor, "project_a", CreatePlanRequest{
		Intent: &intent, PlatformConfiguration: &configuration,
	})
	if err != nil {
		t.Fatalf("create typed plan: %v", err)
	}
	if !plan.CurrentVersion.IsPlatformConfigurationV2() || plan.CurrentVersion.ReadOnly {
		t.Fatalf("typed plan runtime flags = %#v", plan.CurrentVersion)
	}
	if plan.CurrentVersion.CanonicalHash != plan.CurrentVersion.PlatformConfiguration.CanonicalHash {
		t.Fatalf("plan hash %q does not equal configuration hash %q", plan.CurrentVersion.CanonicalHash, plan.CurrentVersion.PlatformConfiguration.CanonicalHash)
	}
	if got := len(plan.CurrentVersion.PlatformConfiguration.Payload.OceanEngine.Promotions); got != 0 {
		t.Fatalf("zero-promotion configuration changed to %d promotions", got)
	}

	changeSet, err := service.CreateChangeSet(context.Background(), actor, "project_a", plan.ID, plan.Version)
	if err != nil {
		t.Fatalf("create change set: %v", err)
	}
	if changeSet.TargetSnapshot == nil || changeSet.TargetSnapshotHash != plan.CurrentVersion.CanonicalHash {
		t.Fatalf("change set did not freeze typed target: %#v", changeSet)
	}
	changeSet, err = service.Preflight(context.Background(), actor, "project_a", changeSet.ID, changeSet.Version)
	if err != nil {
		t.Fatalf("preflight typed change set: %v", err)
	}
	if changeSet.Status != ChangeSetPreflightPassed {
		t.Fatalf("preflight status = %q", changeSet.Status)
	}
	changeSet, err = service.Approve(context.Background(), actor, "project_a", changeSet.ID, changeSet.Version)
	if err != nil {
		t.Fatalf("approve typed change set: %v", err)
	}
	if changeSet.Approval == nil || changeSet.Approval.ConfigurationCanonicalHash != plan.CurrentVersion.CanonicalHash || changeSet.Approval.IntentCanonicalHash != plan.CurrentVersion.DeliveryIntent.CanonicalHash {
		t.Fatalf("approval omitted immutable typed bindings: %#v", changeSet.Approval)
	}

	updatedIntent := intent
	updatedIntent.VersionNumber = 2
	updatedIntent.CanonicalHash = ""
	updatedConfiguration := configuration
	updatedConfiguration.VersionNumber = 2
	updatedConfiguration.ConfigurationID = "ocean-config-2"
	updatedConfiguration.Intent = IntentBinding{}
	updatedConfiguration.CanonicalHash = ""
	updated, err := service.UpdatePlan(context.Background(), actor, "project_a", plan.ID, UpdatePlanRequest{
		ExpectedVersion: plan.CurrentVersionNumber, Intent: &updatedIntent, PlatformConfiguration: &updatedConfiguration,
	})
	if err != nil {
		t.Fatalf("update typed plan: %v", err)
	}
	if updated.CurrentVersionNumber != 2 || updated.CurrentVersion.DeliveryIntent.VersionNumber != 2 || len(updated.Versions) != 2 {
		t.Fatalf("typed immutable history = %#v", updated)
	}
	_, err = service.UpdatePlan(context.Background(), actor, "project_a", plan.ID, UpdatePlanRequest{
		ExpectedVersion: 1, Intent: &updatedIntent, PlatformConfiguration: &updatedConfiguration,
	})
	if !errors.Is(err, ErrPlanVersionConflict) {
		t.Fatalf("stale typed update error = %v", err)
	}
}

func TestPlatformRuntimeMigrationIsAdditiveAndDoesNotRewriteLegacyPayloads(t *testing.T) {
	payload, err := os.ReadFile("../../../migrations/delivery/20260810120000_delivery_platform_configuration_runtime.up.sql")
	if err != nil {
		t.Fatalf("read runtime migration: %v", err)
	}
	sql := strings.ToUpper(string(payload))
	for _, required := range []string{"CREATE TABLE DELIVERY_INTENTS", "CREATE TABLE DELIVERY_PLATFORM_CONFIGURATIONS", "PAYLOAD_SCHEMA_VERSION", "TARGET_SNAPSHOT_SCHEMA_VERSION", "INTENT_CANONICAL_HASH"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration omitted %s", required)
		}
	}
	if strings.Count(sql, "HASH_ALGORITHM VARCHAR(64)") != 2 {
		t.Fatal("runtime migration cannot persist the frozen canonical hash algorithm identifier")
	}
	if strings.Contains(sql, "UNIQUE KEY UQ_DELIVERY_INTENTS_HASH") || strings.Contains(sql, "UNIQUE KEY UQ_DELIVERY_PLATFORM_CONFIGURATIONS_HASH") {
		t.Fatal("canonical payload hashes are not identity keys and must allow equal immutable payloads")
	}
	if !strings.Contains(sql, "MODIFY DATASET_VERSION VARCHAR(160)") || !strings.Contains(sql, "MODIFY FIXTURE_VERSION VARCHAR(160)") {
		t.Fatal("runtime migration must preserve complete outcome fixture lineage in alerts")
	}
	if strings.Contains(sql, "UPDATE DELIVERY_PLAN_VERSIONS") || strings.Contains(sql, "UPDATE DELIVERY_CHANGE_SETS") || strings.Contains(sql, "UPDATE DELIVERY_APPROVALS") {
		t.Fatal("runtime migration rewrites legacy rows")
	}
}

func TestMagneticEngineRuntimeIsCapabilityPendingAndBlocked(t *testing.T) {
	service, actor := newTestService()
	intent := validDeliveryIntent(t)
	configuration := PlatformConfiguration{
		SchemaVersion: PlatformConfigurationSchemaV2, ConfigurationID: "magnetic-config-1", VersionNumber: 1,
		Platform: DeliveryPlatformMagneticEngine, ProfileVersion: MagneticEngineConfigurationProfileV1,
		Intent:        IntentBinding{SchemaVersion: intent.SchemaVersion, IntentID: intent.IntentID, VersionNumber: intent.VersionNumber, CanonicalHash: intent.CanonicalHash},
		HashAlgorithm: CanonicalPayloadHashAlgorithm,
		Payload: PlatformConfigurationPayload{
			Profile:        DeliveryPlatformMagneticEngine,
			MagneticEngine: &MagneticEngineConfiguration{Profile: DeliveryPlatformMagneticEngine, Status: "capability_pending", ReasonCode: ContractErrorCapabilityPending, Reason: "no verified Magnetic Engine field evidence is available"},
		},
		ConfigurationProvenance: ConfigurationProvenance{Kind: ConfigurationGeneratedManually, GeneratorRef: "operator-brief"},
		FactProvenance:          FactProvenance{Source: FactSourceReplay, SnapshotRef: "replay://magnetic-config-1", EvidenceRefs: []string{"evidence://magnetic-config-1"}},
	}
	configuration, err := FinalizePlatformConfiguration(configuration)
	if err != nil {
		t.Fatalf("finalize Magnetic Engine configuration: %v", err)
	}
	plan, err := service.CreatePlan(context.Background(), actor, "project_a", CreatePlanRequest{Intent: &intent, PlatformConfiguration: &configuration})
	if err != nil {
		t.Fatalf("create Magnetic Engine plan: %v", err)
	}
	if plan.CurrentVersion.RuntimeStatus != PlanRuntimeCapabilityPending || plan.CurrentVersion.Scenario != ScenarioCapabilityPending {
		t.Fatalf("Magnetic Engine runtime markers = %#v", plan.CurrentVersion)
	}
	result, err := service.RunPlanPreflight(context.Background(), actor, "project_a", plan.ID)
	if err != nil {
		t.Fatalf("run Magnetic Engine preflight: %v", err)
	}
	if result.Passed || !result.Blocked || len(result.Checks) != 1 || result.Checks[0].Code != ContractErrorCapabilityPending {
		t.Fatalf("Magnetic Engine preflight = %#v", result)
	}
}

func TestDecodedLegacyPlanIsReadOnlyAndKeepsCanonicalHash(t *testing.T) {
	legacy, err := versionFromDraft(
		DeliveryPlan{ID: "legacy-plan", OrganizationID: "org_a", ProjectID: "project_a"},
		1,
		goldenDraft(),
		contract.Principal{Kind: contract.PrincipalUser, ID: "user_a"},
		time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("build legacy version: %v", err)
	}
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodePlanVersion(payload, legacy.CanonicalHash)
	if err != nil {
		t.Fatalf("decode legacy version: %v", err)
	}
	if !decoded.ReadOnly || decoded.RuntimeStatus != PlanRuntimeLegacyUnsupported || decoded.CanonicalHash != legacy.CanonicalHash {
		t.Fatalf("legacy compatibility flags/hash = %#v", decoded)
	}
}

func TestDecodePlanVersionAcceptsStoredCarrierLandingPageConflict(t *testing.T) {
	intent := validDeliveryIntent(t)
	configuration := validOceanEnginePlatformConfiguration(t, intent, 1)
	configuration.Payload.OceanEngine.Project.Carrier = "im"
	hash, err := configuration.ComputeCanonicalHash()
	if err != nil {
		t.Fatal(err)
	}
	configuration.CanonicalHash = hash
	version := DeliveryPlanVersion{
		SchemaVersion:         DeliveryPlanVersionSchemaV2,
		PlanID:                "stored-plan",
		VersionNumber:         1,
		CanonicalHash:         hash,
		DeliveryIntent:        &intent,
		PlatformConfiguration: &configuration,
	}
	payload, err := json.Marshal(version)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodePlanVersion(payload, hash)
	if err != nil {
		t.Fatalf("decode stored version: %v", err)
	}
	if decoded.PlatformConfiguration.Payload.OceanEngine.Project.Carrier != "im" {
		t.Fatalf("stored carrier changed during read: %#v", decoded.PlatformConfiguration.Payload.OceanEngine.Project)
	}
	if err := decoded.PlatformConfiguration.validateStructure(); err == nil {
		t.Fatal("strict validation accepted the stored carrier and landing-page conflict")
	}
}

func readyOceanRuntimeInputs(t *testing.T, promotionCount int) (DeliveryIntent, PlatformConfiguration) {
	t.Helper()
	intent := validDeliveryIntent(t)
	landingPage := resolvedReference("oceanengine", "landing_page", "account:6391", "landing-page-1")
	intent.Payload.LandingPageReferences = []StableReference{landingPage}
	intent.CanonicalHash = ""
	var err error
	intent, err = FinalizeDeliveryIntent(intent)
	if err != nil {
		t.Fatalf("finalize ready OceanEngine intent: %v", err)
	}
	configuration := validOceanEnginePlatformConfiguration(t, intent, promotionCount)
	configuration.Payload.OceanEngine.Project.OptimizationTargetReference = referencePointer(resolvedReference("oceanengine", "optimization_target", "account:6391", "optimization-target-1"))
	for index := range configuration.Payload.OceanEngine.Promotions {
		configuration.Payload.OceanEngine.Promotions[index].LandingPageReference = referencePointer(landingPage)
	}
	configuration.CompilationMetadata.FieldEvidence = []PlatformFieldEvidence{{Field: "project.carrier", State: PlatformEvidenceObserved}}
	configuration.CanonicalHash = ""
	finalized, err := FinalizePlatformConfiguration(configuration)
	if err != nil {
		t.Fatalf("finalize ready OceanEngine configuration: %v", err)
	}
	return intent, finalized
}
