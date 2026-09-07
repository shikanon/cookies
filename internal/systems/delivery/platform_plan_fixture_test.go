package delivery

import (
	"fmt"
	"strings"
	"time"
)

func tourPlanRequest(runID, tourCase string, now time.Time) CreatePlanRequest {
	labels := map[string]string{
		TourCaseGoldenPath:       "黄金路径",
		TourCasePreflightFailure: "预检失败",
		TourCaseApprovalExpired:  "审批过期",
		TourCasePlanStale:        "计划版本过期",
		TourCasePartialExecution: "执行部分成功",
		TourCaseResultUnknown:    "执行结果未知",
		TourCaseReviewRejected:   "审核拒绝告警",
	}
	start := now.UTC().Truncate(24 * time.Hour).Add(7 * 24 * time.Hour)
	identity := runID + "-" + tourCase
	ref := func(kind, id string) StableReference {
		return StableReference{Namespace: "cookies", ObjectKind: kind, Scope: "tour:" + runID, ID: id, Version: "v1", ContentHash: strings.Repeat("a", 64), State: ReferenceResolved}
	}
	material := ref("asset_version", "asset_demo_investor_creative_video")
	intent, _ := FinalizeDeliveryIntent(DeliveryIntent{
		SchemaVersion: DeliveryIntentSchemaV1, IntentID: "intent-" + identity, VersionNumber: 1, HashAlgorithm: CanonicalPayloadHashAlgorithm,
		Payload:                 DeliveryIntentPayload{PayloadSchemaVersion: DeliveryIntentSchemaV1, MarketingObjective: "验证计划来源、审批、平台操作演练、上线后指标与优化证据链。", BudgetBoundary: IntentBudgetBoundary{Currency: "CNY", MinimumTotalMinor: 0, MaximumTotalMinor: 3000000}, ScheduleBoundary: IntentScheduleBoundary{EarliestStart: start, LatestEnd: start.Add(14 * 24 * time.Hour), Timezone: "Asia/Shanghai"}, OptimizationPreferences: []OptimizationPreference{}, MaterialReferences: []StableReference{material}, AudienceConstraints: IntentAudienceConstraints{}, StrategyReference: ref("strategy_version", "task_demo_precision_strategy"), CalibrationManifest: CalibrationManifestBinding{SchemaVersion: OceanEngineCalibrationManifestV1, ManifestID: "oceanengine-calibration-current-test-account-2026-08-16"}},
		ConfigurationProvenance: ConfigurationProvenance{Kind: ConfigurationGeneratedManually, GeneratorRef: "delivery-tour"}, FactProvenance: FactProvenance{Source: FactSourceMock, SnapshotRef: "mock://tour/" + identity}, Audit: ContractAuditMetadata{CreatedBy: "delivery-tour", CreatedAt: now},
	})
	fieldEvidence := []PlatformFieldEvidence{{Field: "project", State: PlatformEvidenceOperatorReviewed}}
	if tourCase == TourCasePreflightFailure {
		fieldEvidence = []PlatformFieldEvidence{{Field: "project.account_reference", State: PlatformEvidencePending, Reason: "tour preflight failure"}}
	}
	configuration, _ := FinalizePlatformConfiguration(PlatformConfiguration{
		SchemaVersion: PlatformConfigurationSchemaV2, ConfigurationID: "configuration-" + identity, VersionNumber: 1, Platform: DeliveryPlatformOceanEngine, ProfileVersion: OceanEngineConfigurationProfileV1,
		Intent: IntentBinding{SchemaVersion: intent.SchemaVersion, IntentID: intent.IntentID, VersionNumber: intent.VersionNumber, CanonicalHash: intent.CanonicalHash}, HashAlgorithm: CanonicalPayloadHashAlgorithm,
		Payload:                 PlatformConfigurationPayload{Profile: DeliveryPlatformOceanEngine, OceanEngine: &OceanEngineConfiguration{Profile: DeliveryPlatformOceanEngine, CalibrationManifest: CalibrationManifestBinding{SchemaVersion: OceanEngineCalibrationManifestV1, ManifestID: "oceanengine-calibration-current-test-account-2026-08-16"}, Project: &OceanEngineProjectDraft{DraftSchemaVersion: OceanEngineConfigurationProfileV1, ProjectDraftID: "project-" + identity, AccountReference: ref("advertiser_account", "mock-tour-advertiser"), MarketingPurpose: "lead_generation", MarketingScenario: "manual_delivery", Carrier: "landing_page", DeliveryMode: "manual", Targeting: OceanEngineTargeting{SmartExpansion: false}, Schedule: OceanEngineSchedule{StartAt: start, EndAt: start.Add(14 * 24 * time.Hour), Timezone: "Asia/Shanghai"}, BudgetAndBidding: OceanEngineBudgetAndBidding{Currency: "CNY", DailyBudgetMinor: 200000, BiddingStrategy: "manual_bid", ChargingMode: "CPC", BidMinor: int64Pointer(100)}, ProjectName: fmt.Sprintf("上线后优化闭环 · %s · %s", labels[tourCase], runID)}, Promotions: []OceanEnginePromotionDraft{{DraftSchemaVersion: OceanEngineConfigurationProfileV1, PromotionDraftID: "promotion-" + identity, DeliveryIdentity: OceanEngineDeliveryIdentity{Mode: "account_info"}, BaseMaterialReferences: []StableReference{material}, CopyItems: []OceanEngineCopyItem{{Text: "tour copy"}}, PromotionName: "Tour promotion"}}}},
		ConfigurationProvenance: ConfigurationProvenance{Kind: ConfigurationGeneratedManually, GeneratorRef: "delivery-tour"}, FactProvenance: FactProvenance{Source: FactSourceMock, SnapshotRef: "mock://tour/" + identity}, Audit: ContractAuditMetadata{CreatedBy: "delivery-tour", CreatedAt: now}, CompilationMetadata: CompilationMetadata{FieldEvidence: fieldEvidence, EvidenceRefs: []string{"mock://tour/" + identity}},
	})
	return CreatePlanRequest{Intent: &intent, PlatformConfiguration: &configuration}
}

func int64Pointer(value int64) *int64 { return &value }
