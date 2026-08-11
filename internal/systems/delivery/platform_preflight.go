package delivery

import (
	"errors"
	"fmt"
)

func runPlatformConfigurationPreflight(version DeliveryPlanVersion) []PreflightCheck {
	checks := make([]PreflightCheck, 0, 8)
	appendFailure := func(err error, fallbackField string) {
		var contractErr *DeliveryContractError
		if errors.As(err, &contractErr) {
			field := contractErr.Field
			if field == "" {
				field = fallbackField
			}
			checks = append(checks, check(contractErr.Code, CheckSeverityError, false, "", contractErr.Message, RepairTarget{Field: field, Section: "平台配置", Label: "修复" + field}))
			return
		}
		checks = append(checks, check(ContractErrorInvalidConfiguration, CheckSeverityError, false, "", err.Error(), RepairTarget{Field: fallbackField, Section: "平台配置", Label: "修复平台配置"}))
	}
	if err := version.DeliveryIntent.Validate(); err != nil {
		appendFailure(err, "intent")
		return checks
	}
	if err := version.PlatformConfiguration.validateStructure(); err != nil {
		appendFailure(err, "platform_configuration")
		return checks
	}
	if version.CanonicalHash != version.PlatformConfiguration.CanonicalHash {
		appendFailure(contractFailure(ContractErrorCanonicalHashMismatch, "canonical_hash", "plan version hash must equal platform configuration hash"), "canonical_hash")
		return checks
	}
	if version.PlatformConfiguration.Platform == DeliveryPlatformMagneticEngine {
		appendFailure(contractFailure(ContractErrorCapabilityPending, "platform", version.PlatformConfiguration.Payload.MagneticEngine.Reason), "platform")
		return checks
	}
	checks = append(checks,
		check("delivery_intent_valid", CheckSeverityError, true, "业务意图结构和哈希有效。", "", RepairTarget{}),
		check("platform_configuration_valid", CheckSeverityError, true, "平台配置结构、判别器和哈希有效。", "", RepairTarget{}),
	)
	for field, reference := range executionReferences(version) {
		if reference.State != ReferenceResolved || reference.ID == "" {
			message := fmt.Sprintf("执行所需引用 %s 尚未解析到稳定 ID。", field)
			checks = append(checks, check(ContractErrorInvalidReference, CheckSeverityError, false, "", message, RepairTarget{Field: field, Section: "稳定引用", Label: "解析" + field}))
		}
	}
	for _, evidence := range version.PlatformConfiguration.CompilationMetadata.FieldEvidence {
		if evidence.State == PlatformEvidencePending || evidence.State == PlatformEvidenceBlockedByEventAsset {
			checks = append(checks, check(string(evidence.State), CheckSeverityError, false, "", evidence.Reason, RepairTarget{Field: evidence.Field, Section: "平台配置", Label: "补齐" + evidence.Field}))
		}
		if evidence.State == PlatformEvidenceWriteValidationPending {
			checks = append(checks, check(string(evidence.State), CheckSeverityWarning, false, "", "真实平台写入仍待验证，本地配置不会被描述为已写入。", RepairTarget{Field: evidence.Field, Section: "平台配置", Label: "保留写入边界"}))
		}
	}
	return checks
}

func executionReferences(version DeliveryPlanVersion) map[string]StableReference {
	result := map[string]StableReference{
		"intent.strategy_reference": version.DeliveryIntent.Payload.StrategyReference,
	}
	for index, reference := range version.DeliveryIntent.Payload.MaterialReferences {
		result[fmt.Sprintf("intent.material_references.%d", index)] = reference
	}
	configuration := version.PlatformConfiguration.Payload.OceanEngine
	if configuration == nil || configuration.Project == nil {
		return result
	}
	result["platform_configuration.project.account_reference"] = configuration.Project.AccountReference
	for index, promotion := range configuration.Promotions {
		for materialIndex, reference := range promotion.BaseMaterialReferences {
			result[fmt.Sprintf("platform_configuration.promotions.%d.base_material_references.%d", index, materialIndex)] = reference
		}
		if promotion.DeliveryIdentity.Mode == "douyin_account" && promotion.DeliveryIdentity.AuthorizedIdentity != nil {
			result[fmt.Sprintf("platform_configuration.promotions.%d.delivery_identity.authorized_identity", index)] = *promotion.DeliveryIdentity.AuthorizedIdentity
		}
	}
	return result
}
