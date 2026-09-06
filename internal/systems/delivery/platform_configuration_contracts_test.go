package delivery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/systems/delivery/calibrationmanifest"
)

func TestOceanEnginePromotionSettingsReadsLegacyCallToAction(t *testing.T) {
	var legacy OceanEnginePromotionSettings
	if err := json.Unmarshal([]byte(`{"call_to_action":"查看详情","source_label":"ecommerce"}`), &legacy); err != nil {
		t.Fatalf("unmarshal legacy settings: %v", err)
	}
	if len(legacy.CallToAction) != 1 || legacy.CallToAction[0] != "查看详情" || legacy.SourceLabel != "ecommerce" {
		t.Fatalf("unexpected legacy settings: %#v", legacy)
	}

	var current OceanEnginePromotionSettings
	if err := json.Unmarshal([]byte(`{"call_to_action":["查看详情","立即预订"]}`), &current); err != nil {
		t.Fatalf("unmarshal current settings: %v", err)
	}
	if len(current.CallToAction) != 2 || current.CallToAction[1] != "立即预订" {
		t.Fatalf("unexpected current settings: %#v", current)
	}
}

func TestPlatformConfigurationAcceptsLegacySingleCallToActionHash(t *testing.T) {
	intent := validDeliveryIntent(t)
	configuration := validOceanEnginePlatformConfiguration(t, intent, 1)
	configuration.Payload.OceanEngine = cloneOceanConfigurationForTest(configuration.Payload.OceanEngine)
	configuration.Payload.OceanEngine.Promotions[0].Settings.CallToAction = []string{"查看详情"}
	legacyHash, compatible, err := configuration.computeLegacySingleCallToActionHash()
	if err != nil {
		t.Fatal(err)
	}
	if !compatible {
		t.Fatal("expected legacy hash compatibility")
	}
	configuration.CanonicalHash = legacyHash
	if err = configuration.Validate(); err != nil {
		t.Fatalf("validate legacy hash: %v", err)
	}
	version := DeliveryPlanVersion{
		SchemaVersion: DeliveryPlanVersionSchemaV2, CanonicalHash: legacyHash,
		DeliveryIntent: &intent, PlatformConfiguration: &configuration,
	}
	if err = validatePlanCanonicalHash(version); err != nil {
		t.Fatalf("validate plan legacy hash: %v", err)
	}
}

func TestDeliveryContractFixturesMatchGoDomainValidation(t *testing.T) {
	fixtureDirectory := filepath.Join("..", "..", "..", "docs", "delivery", "fixtures")

	var intent DeliveryIntent
	readContractFixture(t, filepath.Join(fixtureDirectory, "delivery-intent-v1-valid.json"), &intent)
	if err := intent.Validate(); err != nil {
		t.Fatalf("intent fixture: %v", err)
	}

	var ocean PlatformConfiguration
	readContractFixture(t, filepath.Join(fixtureDirectory, "delivery-platform-configuration-v2-oceanengine-valid.json"), &ocean)
	if err := ocean.Validate(); err != nil {
		t.Fatalf("OceanEngine fixture: %v", err)
	}
	if ocean.Payload.OceanEngine == nil || ocean.Payload.OceanEngine.Project == nil || len(ocean.Payload.OceanEngine.Promotions) != 2 {
		t.Fatal("OceanEngine fixture must contain one project and two promotions")
	}

	var magnetic PlatformConfiguration
	readContractFixture(t, filepath.Join(fixtureDirectory, "delivery-platform-configuration-v2-magnetic-pending.json"), &magnetic)
	if code := DeliveryContractErrorCode(magnetic.Validate()); code != ContractErrorCapabilityPending {
		t.Fatalf("Magnetic fixture result code = %q", code)
	}
}

func TestOceanEngineMarketingPurposeMustBeAllowedByTheFrozenManifest(t *testing.T) {
	intent := validDeliveryIntent(t)
	configuration := validOceanEnginePlatformConfiguration(t, intent, 1)
	configuration.Payload.OceanEngine.Project.MarketingPurpose = "free_text_business_goal"
	if code := DeliveryContractErrorCode(configuration.Validate()); code != ContractErrorInvalidConfiguration {
		t.Fatalf("marketing purpose code = %q", code)
	}
}

func TestManifestNonEvidenceMappingsReachTheirDeclaredDomainFields(t *testing.T) {
	manifest, err := calibrationmanifest.Current()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateManifestContractOwnership(manifest); err != nil {
		t.Fatal(err)
	}
	for index := range manifest.ConsumerMappings {
		if manifest.ConsumerMappings[index].Treatment == calibrationmanifest.EvidenceOnly {
			continue
		}
		manifest.ConsumerMappings[index].ContractPath += ".MissingField"
		if err := validateManifestContractOwnership(manifest); err == nil {
			t.Fatal("a non-evidence mapping to a missing domain field must fail closed")
		}
		return
	}
	t.Fatal("fixture must contain a non-evidence consumer mapping")
}

func TestCanonicalProjectionsContainEveryBusinessFieldAndEveryLeafIsHashSensitive(t *testing.T) {
	intent := validDeliveryIntent(t)
	intentExpected := jsonValue(t, intent.Payload)
	removeReferenceMetadata(intentExpected)
	intentActual := jsonValue(t, intent.CanonicalPayload())
	if !reflect.DeepEqual(intentActual, intentExpected) {
		t.Fatalf("intent canonical projection omitted or added fields\nactual: %#v\nexpected: %#v", intentActual, intentExpected)
	}
	assertEveryCanonicalLeafAffectsHash(t, intentActual)

	configuration := validOceanEnginePlatformConfiguration(t, intent, 2)
	payload := jsonValue(t, configuration.Payload).(map[string]any)
	removeReferenceMetadata(payload)
	configurationExpected := map[string]any{
		"schema_version":  configuration.SchemaVersion,
		"platform":        string(configuration.Platform),
		"profile_version": configuration.ProfileVersion,
		"intent":          jsonValue(t, configuration.Intent),
		"profile":         string(configuration.Payload.Profile),
		"ocean_engine":    payload["ocean_engine"],
	}
	configurationActual := jsonValue(t, configuration.CanonicalPayload())
	if !reflect.DeepEqual(configurationActual, configurationExpected) {
		t.Fatalf("platform canonical projection omitted or added fields\nactual: %#v\nexpected: %#v", configurationActual, configurationExpected)
	}
	assertEveryCanonicalLeafAffectsHash(t, configurationActual)
}

func readContractFixture(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}

func jsonValue(t *testing.T, value any) any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func removeReferenceMetadata(value any) {
	switch typed := value.(type) {
	case map[string]any:
		delete(typed, "display_name_snapshot")
		delete(typed, "evidence_version")
		for _, child := range typed {
			removeReferenceMetadata(child)
		}
	case []any:
		for _, child := range typed {
			removeReferenceMetadata(child)
		}
	}
}

type canonicalLeaf struct {
	path  []any
	value any
}

func canonicalLeaves(value any, path []any) []canonicalLeaf {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var leaves []canonicalLeaf
		for _, key := range keys {
			leaves = append(leaves, canonicalLeaves(typed[key], append(append([]any(nil), path...), key))...)
		}
		return leaves
	case []any:
		var leaves []canonicalLeaf
		for index, child := range typed {
			leaves = append(leaves, canonicalLeaves(child, append(append([]any(nil), path...), index))...)
		}
		return leaves
	case nil:
		return nil
	default:
		return []canonicalLeaf{{path: append([]any(nil), path...), value: typed}}
	}
}

func assertEveryCanonicalLeafAffectsHash(t *testing.T, projection any) {
	t.Helper()
	baseHash, err := contract.CanonicalJSONHash(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, leaf := range canonicalLeaves(projection, nil) {
		candidate := jsonValue(t, projection)
		mutateCanonicalLeaf(candidate, leaf.path, changedCanonicalLeafValue(leaf.value))
		changedHash, err := contract.CanonicalJSONHash(candidate)
		if err != nil {
			t.Fatal(err)
		}
		if changedHash == baseHash {
			t.Fatalf("canonical leaf change at %v did not affect hash", leaf.path)
		}
	}
}

func mutateCanonicalLeaf(root any, path []any, value any) {
	current := root
	for _, segment := range path[:len(path)-1] {
		switch key := segment.(type) {
		case string:
			current = current.(map[string]any)[key]
		case int:
			current = current.([]any)[key]
		}
	}
	switch key := path[len(path)-1].(type) {
	case string:
		current.(map[string]any)[key] = value
	case int:
		current.([]any)[key] = value
	}
}

func changedCanonicalLeafValue(value any) any {
	switch typed := value.(type) {
	case string:
		return typed + "#changed"
	case float64:
		return typed + 1
	case bool:
		return !typed
	default:
		panic("unsupported canonical leaf type")
	}
}

func TestDeliveryIntentContractValidationAndHashProjection(t *testing.T) {
	intent := validDeliveryIntent(t)
	if err := intent.Validate(); err != nil {
		t.Fatalf("valid intent: %v", err)
	}

	repeated, err := intent.ComputeCanonicalHash()
	if err != nil {
		t.Fatal(err)
	}
	if repeated != intent.CanonicalHash {
		t.Fatalf("non-deterministic intent hash: %q != %q", repeated, intent.CanonicalHash)
	}

	businessChange := intent
	businessChange.Payload.MarketingObjective = "maximize qualified orders"
	assertHashChanged(t, intent.CanonicalHash, businessChange.ComputeCanonicalHash)

	referenceChange := intent
	referenceChange.Payload.MaterialReferences = append([]StableReference(nil), intent.Payload.MaterialReferences...)
	referenceChange.Payload.MaterialReferences[0].ID = "asset-2:v1"
	assertHashChanged(t, intent.CanonicalHash, referenceChange.ComputeCanonicalHash)

	metadataChange := intent
	metadataChange.Audit = ContractAuditMetadata{CreatedBy: "another-user", CreatedAt: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
	metadataChange.ConfigurationProvenance = ConfigurationProvenance{Kind: ConfigurationGeneratedByImport, GeneratorRef: "import-2"}
	metadataChange.FactProvenance = FactProvenance{Source: FactSourceConnector, EvidenceRefs: []string{"evidence://changed"}, ObservedAt: time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC)}
	metadataChange.Payload.MaterialReferences = append([]StableReference(nil), intent.Payload.MaterialReferences...)
	metadataChange.Payload.MaterialReferences[0].DisplayNameSnapshot = "changed display label"
	metadataChange.Payload.MaterialReferences[0].EvidenceVersion = "changed-evidence"
	metadataHash, err := metadataChange.ComputeCanonicalHash()
	if err != nil {
		t.Fatal(err)
	}
	if metadataHash != intent.CanonicalHash {
		t.Fatalf("audit/evidence/display metadata changed intent hash: %q != %q", metadataHash, intent.CanonicalHash)
	}

	encoded, err := json.Marshal(intent.Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, platformField := range []string{"ocean_engine", "magnetic_engine", "carrier", "delivery_mode", "promotion"} {
		if strings.Contains(string(encoded), platformField) {
			t.Fatalf("platform-neutral intent contains %q: %s", platformField, encoded)
		}
	}

	invalid := intent
	invalid.Payload.MaterialReferences = append([]StableReference(nil), intent.Payload.MaterialReferences...)
	invalid.Payload.MaterialReferences[0].ID = ""
	invalid.CanonicalHash, _ = invalid.ComputeCanonicalHash()
	if code := DeliveryContractErrorCode(invalid.Validate()); code != ContractErrorInvalidReference {
		t.Fatalf("resolved reference without id code = %q", code)
	}

	unresolved := intent
	unresolved.Payload.LandingPageReferences = []StableReference{{Namespace: "oceanengine", ObjectKind: "landing_page", Scope: "account:6391", State: ReferenceUnresolved, Reason: "asset has not been selected"}}
	unresolved, err = FinalizeDeliveryIntent(unresolved)
	if err != nil {
		t.Fatalf("unresolved reference should be representable: %v", err)
	}
	blocked := intent
	blocked.Payload.ProductReferences = []StableReference{{Namespace: "oceanengine", ObjectKind: "product", Scope: "account:6391", State: ReferenceBlocked, Reason: "product permission is missing"}}
	if _, err = FinalizeDeliveryIntent(blocked); err != nil {
		t.Fatalf("blocked reference should be representable: %v", err)
	}
}

func TestOceanEngineConfigurationSupportsOneProjectAndMultiplePromotions(t *testing.T) {
	intent := validDeliveryIntent(t)
	configuration := validOceanEnginePlatformConfiguration(t, intent, 2)
	if err := configuration.Validate(); err != nil {
		t.Fatalf("valid OceanEngine configuration: %v", err)
	}
	if len(configuration.Payload.OceanEngine.Promotions) != 2 {
		t.Fatalf("promotion count = %d", len(configuration.Payload.OceanEngine.Promotions))
	}

	zeroPromotions := validOceanEnginePlatformConfiguration(t, intent, 0)
	if err := zeroPromotions.Validate(); err != nil {
		t.Fatalf("zero promotions should be valid: %v", err)
	}

	ownedLandingPage := configuration
	ownedLandingPage.Payload.OceanEngine = cloneOceanConfigurationForTest(configuration.Payload.OceanEngine)
	ownedLandingPage.Payload.OceanEngine.Project.Carrier = "owned_landing_page"
	ownedLandingPage.Payload.OceanEngine.Promotions[0].LandingPageReference = &StableReference{
		Namespace: "cookies", ObjectKind: "owned_landing_page", Scope: "current_project",
		ID: "https://example.test/landing", State: ReferenceResolved,
	}
	ownedLandingPage.CanonicalHash, _ = ownedLandingPage.ComputeCanonicalHash()
	if err := ownedLandingPage.Validate(); err != nil {
		t.Fatalf("owned landing-page configuration: %v", err)
	}

	repeated, err := configuration.ComputeCanonicalHash()
	if err != nil {
		t.Fatal(err)
	}
	if repeated != configuration.CanonicalHash {
		t.Fatalf("non-deterministic configuration hash: %q != %q", repeated, configuration.CanonicalHash)
	}

	businessChange := configuration
	businessChange.Payload.OceanEngine = cloneOceanConfigurationForTest(configuration.Payload.OceanEngine)
	businessChange.Payload.OceanEngine.Project.BudgetAndBidding.DailyBudgetMinor++
	assertHashChanged(t, configuration.CanonicalHash, businessChange.ComputeCanonicalHash)

	referenceChange := configuration
	referenceChange.Payload.OceanEngine = cloneOceanConfigurationForTest(configuration.Payload.OceanEngine)
	referenceChange.Payload.OceanEngine.Promotions[0].BaseMaterialReferences[0].ID = "asset-9:v1"
	assertHashChanged(t, configuration.CanonicalHash, referenceChange.ComputeCanonicalHash)

	intentChange := configuration
	intentChange.Intent.VersionNumber++
	assertHashChanged(t, configuration.CanonicalHash, intentChange.ComputeCanonicalHash)

	orderChange := configuration
	orderChange.Payload.OceanEngine = cloneOceanConfigurationForTest(configuration.Payload.OceanEngine)
	orderChange.Payload.OceanEngine.Promotions[0], orderChange.Payload.OceanEngine.Promotions[1] = orderChange.Payload.OceanEngine.Promotions[1], orderChange.Payload.OceanEngine.Promotions[0]
	assertHashChanged(t, configuration.CanonicalHash, orderChange.ComputeCanonicalHash)

	metadataChange := configuration
	metadataChange.Audit = ContractAuditMetadata{CreatedBy: "reviewer-2", CreatedAt: time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)}
	metadataChange.ConfigurationProvenance = ConfigurationProvenance{Kind: ConfigurationGeneratedByDecisionEngine, PolicyVersion: "policy/v2"}
	metadataChange.FactProvenance = FactProvenance{Source: FactSourceConnector, EvidenceRefs: []string{"evidence://new"}}
	metadataChange.CompilationMetadata = CompilationMetadata{FieldEvidence: []PlatformFieldEvidence{{Field: "project.carrier", State: PlatformEvidencePending, Reason: "new evidence"}}, Steps: []string{"changed-step"}}
	metadataChange.Payload.OceanEngine = cloneOceanConfigurationForTest(configuration.Payload.OceanEngine)
	metadataChange.Payload.OceanEngine.Project.AccountReference.DisplayNameSnapshot = "changed label"
	metadataChange.Payload.OceanEngine.Project.AccountReference.EvidenceVersion = "changed evidence"
	metadataHash, err := metadataChange.ComputeCanonicalHash()
	if err != nil {
		t.Fatal(err)
	}
	if metadataHash != configuration.CanonicalHash {
		t.Fatalf("audit/evidence/compilation/display metadata changed configuration hash: %q != %q", metadataHash, configuration.CanonicalHash)
	}
}

func TestPlatformConfigurationValidationErrorsAreStable(t *testing.T) {
	intent := validDeliveryIntent(t)
	base := validOceanEnginePlatformConfiguration(t, intent, 2)

	tests := []struct {
		name string
		code string
		edit func(*PlatformConfiguration)
	}{
		{name: "unknown schema", code: ContractErrorUnknownSchemaVersion, edit: func(value *PlatformConfiguration) { value.SchemaVersion = "delivery-platform-configuration/v999" }},
		{name: "invalid envelope", code: ContractErrorInvalidConfiguration, edit: func(value *PlatformConfiguration) { value.ConfigurationID = "" }},
		{name: "invalid intent hash", code: ContractErrorInvalidIntent, edit: func(value *PlatformConfiguration) { value.Intent.CanonicalHash = "not-a-sha256" }},
		{name: "mixed provenance vocabulary", code: ContractErrorInvalidConfiguration, edit: func(value *PlatformConfiguration) {
			value.FactProvenance.Source = FactSource(value.ConfigurationProvenance.Kind)
		}},
		{name: "unknown profile", code: ContractErrorUnknownProfileVersion, edit: func(value *PlatformConfiguration) { value.ProfileVersion = "oceanengine-configuration/v999" }},
		{name: "discriminator mismatch", code: ContractErrorPlatformProfileMismatch, edit: func(value *PlatformConfiguration) { value.Payload.Profile = DeliveryPlatformMagneticEngine }},
		{name: "missing project", code: ContractErrorProjectRequired, edit: func(value *PlatformConfiguration) {
			value.Payload.OceanEngine = cloneOceanConfigurationForTest(value.Payload.OceanEngine)
			value.Payload.OceanEngine.Project = nil
		}},
		{name: "duplicate promotion", code: ContractErrorInvalidPromotion, edit: func(value *PlatformConfiguration) {
			value.Payload.OceanEngine = cloneOceanConfigurationForTest(value.Payload.OceanEngine)
			value.Payload.OceanEngine.Promotions[1].PromotionDraftID = value.Payload.OceanEngine.Promotions[0].PromotionDraftID
		}},
		{name: "too many call-to-action values", code: ContractErrorInvalidPromotion, edit: func(value *PlatformConfiguration) {
			value.Payload.OceanEngine = cloneOceanConfigurationForTest(value.Payload.OceanEngine)
			value.Payload.OceanEngine.Promotions[0].Settings.CallToAction = []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"}
		}},
		{name: "duplicate call-to-action value", code: ContractErrorInvalidPromotion, edit: func(value *PlatformConfiguration) {
			value.Payload.OceanEngine = cloneOceanConfigurationForTest(value.Payload.OceanEngine)
			value.Payload.OceanEngine.Promotions[0].Settings.CallToAction = []string{"查看详情", "查看详情"}
		}},
		{name: "invalid promotion reference", code: ContractErrorInvalidReference, edit: func(value *PlatformConfiguration) {
			value.Payload.OceanEngine = cloneOceanConfigurationForTest(value.Payload.OceanEngine)
			value.Payload.OceanEngine.Promotions[0].BaseMaterialReferences[0].ID = ""
		}},
		{name: "owned carrier with Orange landing page", code: ContractErrorInvalidPromotion, edit: func(value *PlatformConfiguration) {
			value.Payload.OceanEngine = cloneOceanConfigurationForTest(value.Payload.OceanEngine)
			value.Payload.OceanEngine.Project.Carrier = "owned_landing_page"
			value.Payload.OceanEngine.Promotions[0].LandingPageReference.ObjectKind = "orange_landing_page"
		}},
		{name: "IM carrier with promotion landing page", code: ContractErrorInvalidPromotion, edit: func(value *PlatformConfiguration) {
			value.Payload.OceanEngine = cloneOceanConfigurationForTest(value.Payload.OceanEngine)
			value.Payload.OceanEngine.Project.Carrier = "im"
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			test.edit(&candidate)
			candidate.CanonicalHash, _ = candidate.ComputeCanonicalHash()
			if code := DeliveryContractErrorCode(candidate.Validate()); code != test.code {
				t.Fatalf("error code = %q, want %q", code, test.code)
			}
		})
	}
}

func TestMagneticEngineReturnsCapabilityPendingWithoutInventedConfiguration(t *testing.T) {
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
		ConfigurationProvenance: ConfigurationProvenance{Kind: ConfigurationGeneratedManually},
		FactProvenance:          FactProvenance{Source: FactSourcePageEvidence},
	}
	var err error
	configuration, err = FinalizePlatformConfiguration(configuration)
	if err != nil {
		t.Fatalf("finalize pending profile: %v", err)
	}
	if code := DeliveryContractErrorCode(configuration.Validate()); code != ContractErrorCapabilityPending {
		t.Fatalf("capability result code = %q", code)
	}

	encoded, err := json.Marshal(configuration.Payload.MagneticEngine)
	if err != nil {
		t.Fatal(err)
	}
	for _, invented := range []string{"project", "promotion", "selector"} {
		if strings.Contains(string(encoded), invented) {
			t.Fatalf("Magnetic profile invented %q fields: %s", invented, encoded)
		}
	}
}

func validDeliveryIntent(t *testing.T) DeliveryIntent {
	t.Helper()
	dailyMin, dailyMax := int64(30000), int64(300000)
	target := 120.0
	value := DeliveryIntent{
		SchemaVersion: DeliveryIntentSchemaV1, IntentID: "intent-1", VersionNumber: 1, HashAlgorithm: CanonicalPayloadHashAlgorithm,
		Payload: DeliveryIntentPayload{
			PayloadSchemaVersion:    DeliveryIntentSchemaV1,
			MarketingObjective:      "increase qualified ecommerce conversions",
			BudgetBoundary:          IntentBudgetBoundary{Currency: "CNY", MinimumTotalMinor: 30000, MaximumTotalMinor: 1000000, MinimumDailyMinor: &dailyMin, MaximumDailyMinor: &dailyMax},
			ScheduleBoundary:        IntentScheduleBoundary{EarliestStart: time.Date(2026, 8, 11, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60)), LatestEnd: time.Date(2026, 8, 31, 23, 59, 59, 0, time.FixedZone("CST", 8*60*60)), Timezone: "Asia/Shanghai"},
			OptimizationPreferences: []OptimizationPreference{{Metric: "cost_per_conversion", Direction: OptimizationMinimize, TargetValue: &target, Unit: "CNY"}},
			ProductReferences:       []StableReference{resolvedReference("oceanengine", "product", "account:6391", "product-1")},
			LandingPageReferences:   []StableReference{{Namespace: "oceanengine", ObjectKind: "landing_page", Scope: "account:6391", State: ReferenceUnresolved, Reason: "landing page selection is pending"}},
			MaterialReferences:      []StableReference{resolvedReference("cookies", "asset_version", "project:project-1", "asset-1:v1")},
			AudienceConstraints:     IntentAudienceConstraints{IncludeReferences: []StableReference{{Namespace: "oceanengine", ObjectKind: "audience_package", Scope: "account:6391", State: ReferenceUnresolved, Reason: "audience package is optional"}}, Constraints: []string{"exclude existing purchasers"}},
			StrategyReference:       resolvedReference("cookies", "strategy_version", "project:project-1", "strategy-task-1:v3"),
			CalibrationManifest:     CalibrationManifestBinding{SchemaVersion: OceanEngineCalibrationManifestV1, ManifestID: "oceanengine-calibration-current-test-account-2026-08-16"},
		},
		ConfigurationProvenance: ConfigurationProvenance{Kind: ConfigurationGeneratedManually, GeneratorRef: "operator-brief"},
		FactProvenance:          FactProvenance{Source: FactSourceReplay, SnapshotRef: "replay://intent-1", EvidenceRefs: []string{"evidence://intent-1"}},
		Audit:                   ContractAuditMetadata{CreatedBy: "user-1", CreatedAt: time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)},
	}
	finalized, err := FinalizeDeliveryIntent(value)
	if err != nil {
		t.Fatalf("finalize intent: %v", err)
	}
	return finalized
}

func validOceanEnginePlatformConfiguration(t *testing.T, intent DeliveryIntent, promotionCount int) PlatformConfiguration {
	t.Helper()
	bid := int64(100)
	comments := true
	promotions := make([]OceanEnginePromotionDraft, promotionCount)
	for index := range promotions {
		promotions[index] = OceanEnginePromotionDraft{
			DraftSchemaVersion:     OceanEngineConfigurationProfileV1,
			PromotionDraftID:       "promotion-" + string(rune('1'+index)),
			DeliveryIdentity:       OceanEngineDeliveryIdentity{Mode: "account_info"},
			BaseMaterialReferences: []StableReference{resolvedReference("cookies", "asset_version", "project:project-1", "asset-1:v1")},
			CopyItems:              []OceanEngineCopyItem{{Text: "verified ecommerce copy"}},
			LandingPageReference:   &StableReference{Namespace: "oceanengine", ObjectKind: "landing_page", Scope: "account:6391", State: ReferenceUnresolved, Reason: "page selection is pending"},
			ProductReference:       referencePointer(resolvedReference("oceanengine", "product", "account:6391", "product-1")),
			Settings:               OceanEnginePromotionSettings{CallToAction: []string{"platform_pending"}, SourceLabel: "ecommerce", CommentsEnabled: &comments},
			PromotionName:          "ecommerce-promotion-" + string(rune('1'+index)),
		}
	}
	value := PlatformConfiguration{
		SchemaVersion: PlatformConfigurationSchemaV2, ConfigurationID: "ocean-config-1", VersionNumber: 1,
		Platform: DeliveryPlatformOceanEngine, ProfileVersion: OceanEngineConfigurationProfileV1,
		Intent:        IntentBinding{SchemaVersion: intent.SchemaVersion, IntentID: intent.IntentID, VersionNumber: intent.VersionNumber, CanonicalHash: intent.CanonicalHash},
		HashAlgorithm: CanonicalPayloadHashAlgorithm,
		Payload: PlatformConfigurationPayload{
			Profile: DeliveryPlatformOceanEngine,
			OceanEngine: &OceanEngineConfiguration{
				Profile: DeliveryPlatformOceanEngine, CalibrationManifest: CalibrationManifestBinding{SchemaVersion: OceanEngineCalibrationManifestV1, ManifestID: "oceanengine-calibration-current-test-account-2026-08-16"},
				Project: &OceanEngineProjectDraft{
					DraftSchemaVersion: OceanEngineConfigurationProfileV1, ProjectDraftID: "project-draft-1",
					AccountReference: resolvedReference("oceanengine", "account", "account:6391", "account-6391"),
					MarketingPurpose: "ecommerce", MarketingScenario: "short_video_image_text", MarketingProductReference: referencePointer(resolvedReference("oceanengine", "product", "account:6391", "product-1")),
					Carrier: "orange_landing_page", OptimizationTargetReference: &StableReference{Namespace: "oceanengine", ObjectKind: "optimization_target", Scope: "account:6391", State: ReferenceUnresolved, Reason: "platform target id is pending"},
					DeliveryMode: "manual", Targeting: OceanEngineTargeting{Regions: []string{"CN"}, AgeRanges: []string{"18-60"}},
					Schedule:         OceanEngineSchedule{StartAt: time.Date(2026, 8, 11, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60)), EndAt: time.Date(2026, 8, 31, 23, 59, 59, 0, time.FixedZone("CST", 8*60*60)), Timezone: "Asia/Shanghai"},
					BudgetAndBidding: OceanEngineBudgetAndBidding{Currency: "CNY", DailyBudgetMinor: 30000, BiddingStrategy: "stable_cost", ChargingMode: "CPC", BidMinor: &bid},
					ProjectName:      "ecommerce-manual-project",
				},
				Promotions: promotions,
			},
		},
		ConfigurationProvenance: ConfigurationProvenance{Kind: ConfigurationGeneratedByRule, GeneratorRef: "delivery-policy", PolicyVersion: "v1"},
		FactProvenance:          FactProvenance{Source: FactSourceReplay, SnapshotRef: "replay://ocean-config-1", EvidenceRefs: []string{"evidence://ocean-config-1"}},
		Audit:                   ContractAuditMetadata{CreatedBy: "user-1", CreatedAt: time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)},
		CompilationMetadata:     CompilationMetadata{FieldEvidence: []PlatformFieldEvidence{{Field: "project.carrier", State: PlatformEvidenceObserved}, {Field: "project.optimization_target_reference", State: PlatformEvidencePending, Reason: "stable target id unavailable"}}, EvidenceRefs: []string{"page://oceanengine/ecommerce-manual"}},
	}
	finalized, err := FinalizePlatformConfiguration(value)
	if err != nil {
		t.Fatalf("finalize OceanEngine configuration: %v", err)
	}
	return finalized
}

func resolvedReference(namespace, kind, scope, id string) StableReference {
	return StableReference{Namespace: namespace, ObjectKind: kind, Scope: scope, ID: id, Version: "v1", ContentHash: strings.Repeat("a", 64), State: ReferenceResolved, DisplayNameSnapshot: "audit label", EvidenceVersion: "evidence/v1"}
}

func referencePointer(value StableReference) *StableReference {
	return &value
}

func cloneOceanConfigurationForTest(value *OceanEngineConfiguration) *OceanEngineConfiguration {
	encoded, _ := json.Marshal(value)
	var clone OceanEngineConfiguration
	_ = json.Unmarshal(encoded, &clone)
	return &clone
}

func assertHashChanged(t *testing.T, original string, compute func() (string, error)) {
	t.Helper()
	changed, err := compute()
	if err != nil {
		t.Fatal(err)
	}
	if changed == original {
		t.Fatalf("business change did not affect hash %q", original)
	}
}
