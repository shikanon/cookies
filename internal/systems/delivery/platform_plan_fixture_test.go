package delivery

import (
	"fmt"
	"strings"
	"time"
)

func platformPlanRequest(runID string, now time.Time) CreatePlanRequest {
	start := now.UTC().Truncate(24 * time.Hour).Add(7 * 24 * time.Hour)
	identity := runID
	ref := func(kind, id string) StableReference {
		return StableReference{Namespace: "cookies", ObjectKind: kind, Scope: "test:" + runID, ID: id, Version: "v1", ContentHash: strings.Repeat("a", 64), State: ReferenceResolved}
	}
	material := ref("asset_version", "asset_test_creative_video")
	intent, _ := FinalizeDeliveryIntent(DeliveryIntent{
		SchemaVersion: DeliveryIntentSchemaV1, IntentID: "intent-" + identity, VersionNumber: 1, HashAlgorithm: CanonicalPayloadHashAlgorithm,
		Payload:                 DeliveryIntentPayload{PayloadSchemaVersion: DeliveryIntentSchemaV1, MarketingObjective: "验证计划来源、审批、平台写入、上线后指标与优化证据链。", BudgetBoundary: IntentBudgetBoundary{Currency: "CNY", MinimumTotalMinor: 0, MaximumTotalMinor: 3000000}, ScheduleBoundary: IntentScheduleBoundary{EarliestStart: start, LatestEnd: start.Add(14 * 24 * time.Hour), Timezone: "Asia/Shanghai"}, OptimizationPreferences: []OptimizationPreference{}, MaterialReferences: []StableReference{material}, AudienceConstraints: IntentAudienceConstraints{}, StrategyReference: ref("strategy_version", "task_test_precision_strategy"), CalibrationManifest: CalibrationManifestBinding{SchemaVersion: OceanEngineCalibrationManifestV1, ManifestID: "oceanengine-calibration-current-test-account-2026-08-16"}},
		ConfigurationProvenance: ConfigurationProvenance{Kind: ConfigurationGeneratedManually, GeneratorRef: "delivery-test"}, FactProvenance: FactProvenance{Source: FactSourceMock, SnapshotRef: "mock://test/" + identity}, Audit: ContractAuditMetadata{CreatedBy: "delivery-test", CreatedAt: now},
	})
	configuration, _ := FinalizePlatformConfiguration(PlatformConfiguration{
		SchemaVersion: PlatformConfigurationSchemaV2, ConfigurationID: "configuration-" + identity, VersionNumber: 1, Platform: DeliveryPlatformOceanEngine, ProfileVersion: OceanEngineConfigurationProfileV1,
		Intent:                  IntentBinding{SchemaVersion: intent.SchemaVersion, IntentID: intent.IntentID, VersionNumber: intent.VersionNumber, CanonicalHash: intent.CanonicalHash},
		HashAlgorithm:           CanonicalPayloadHashAlgorithm,
		Payload:                 PlatformConfigurationPayload{Profile: DeliveryPlatformOceanEngine, OceanEngine: &OceanEngineConfiguration{Profile: DeliveryPlatformOceanEngine, CalibrationManifest: CalibrationManifestBinding{SchemaVersion: OceanEngineCalibrationManifestV1, ManifestID: "oceanengine-calibration-current-test-account-2026-08-16"}, Project: &OceanEngineProjectDraft{DraftSchemaVersion: OceanEngineConfigurationProfileV1, ProjectDraftID: "project-" + identity, AccountReference: ref("advertiser_account", "mock-test-advertiser"), MarketingPurpose: "lead_generation", MarketingScenario: "manual_delivery", Carrier: "landing_page", DeliveryMode: "manual", Targeting: OceanEngineTargeting{SmartExpansion: false}, Schedule: OceanEngineSchedule{StartAt: start, EndAt: start.Add(14 * 24 * time.Hour), Timezone: "Asia/Shanghai"}, BudgetAndBidding: OceanEngineBudgetAndBidding{Currency: "CNY", DailyBudgetMinor: 200000, BiddingStrategy: "manual_bid", ChargingMode: "CPC", BidMinor: int64Pointer(100)}, ProjectName: fmt.Sprintf("上线后优化闭环 · %s", runID)}, Promotions: []OceanEnginePromotionDraft{{DraftSchemaVersion: OceanEngineConfigurationProfileV1, PromotionDraftID: "promotion-" + identity, DeliveryIdentity: OceanEngineDeliveryIdentity{Mode: "account_info"}, BaseMaterialReferences: []StableReference{material}, CopyItems: []OceanEngineCopyItem{{Text: "test copy"}}, PromotionName: "Test promotion"}}}},
		ConfigurationProvenance: ConfigurationProvenance{Kind: ConfigurationGeneratedManually, GeneratorRef: "delivery-test"}, FactProvenance: FactProvenance{Source: FactSourceMock, SnapshotRef: "mock://test/" + identity}, Audit: ContractAuditMetadata{CreatedBy: "delivery-test", CreatedAt: now}, CompilationMetadata: CompilationMetadata{FieldEvidence: []PlatformFieldEvidence{{Field: "project", State: PlatformEvidenceOperatorReviewed}}, EvidenceRefs: []string{"mock://test/" + identity}},
	})
	return CreatePlanRequest{Intent: &intent, PlatformConfiguration: &configuration}
}

func int64Pointer(value int64) *int64 { return &value }
