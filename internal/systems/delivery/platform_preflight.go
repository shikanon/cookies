package delivery

import (
	"errors"
	"fmt"
)

func intentContainsReference(references []StableReference, selected StableReference) bool {
	for _, reference := range references {
		if reference.State == ReferenceResolved && reference.Namespace == selected.Namespace && reference.ObjectKind == selected.ObjectKind && reference.ID == selected.ID {
			return true
		}
	}
	return false
}

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
	if configuration := version.PlatformConfiguration.Payload.OceanEngine; configuration != nil && configuration.Project != nil && configuration.Project.MarketingProductReference != nil {
		product := *configuration.Project.MarketingProductReference
		allowed := intentContainsReference(version.DeliveryIntent.Payload.ProductReferences, product)
		checks = append(checks, check(
			"marketing_product_outside_intent", CheckSeverityError, allowed,
			"营销商品已加入投放意图。", "营销商品未加入投放意图。请保存当前平台配置后再创建执行。",
			RepairTarget{Field: "intent.product_references", Section: "平台配置", Label: "保存营销商品引用"},
		))
	}
	if configuration := version.PlatformConfiguration.Payload.OceanEngine; configuration != nil && configuration.Project != nil {
		for promotionIndex, promotion := range configuration.Promotions {
			for _, reference := range promotion.BaseMaterialReferences {
				if reference.State != ReferenceResolved || reference.ID == "" {
					continue
				}
				checks = append(checks, check("base_material_outside_intent", CheckSeverityError, intentContainsReference(version.DeliveryIntent.Payload.MaterialReferences, reference), "基础素材已加入投放意图。", fmt.Sprintf("推广单元 %d 的基础素材未加入投放意图。请保存当前平台配置后再创建执行。", promotionIndex+1), RepairTarget{Field: "intent.material_references", Section: "平台配置", Label: "保存基础素材引用"}))
			}
			for _, reference := range promotion.ProductImageReferences {
				if reference.State != ReferenceResolved || reference.ID == "" {
					continue
				}
				checks = append(checks, check("product_image_outside_intent", CheckSeverityError, intentContainsReference(version.DeliveryIntent.Payload.MaterialReferences, reference), "产品主图已加入投放意图。", fmt.Sprintf("推广单元 %d 的产品主图未加入投放意图。请保存当前平台配置后再创建执行。", promotionIndex+1), RepairTarget{Field: "intent.material_references", Section: "平台配置", Label: "保存产品主图引用"}))
			}
			landingPage := promotion.LandingPageReference
			ownedLandingPage := configuration.Project.Carrier == "owned_landing_page" && landingPage != nil && landingPage.ObjectKind == "owned_landing_page"
			if landingPage != nil && !ownedLandingPage && landingPage.State == ReferenceResolved && landingPage.ID != "" {
				checks = append(checks, check("landing_page_outside_intent", CheckSeverityError, intentContainsReference(version.DeliveryIntent.Payload.LandingPageReferences, *landingPage), "落地页已加入投放意图。", fmt.Sprintf("推广单元 %d 的落地页未加入投放意图。请保存当前平台配置后再创建执行。", promotionIndex+1), RepairTarget{Field: "intent.landing_page_references", Section: "平台配置", Label: "保存落地页引用"}))
			}
		}
	}
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
	if configuration.Project.MarketingProductReference != nil {
		result["platform_configuration.project.marketing_product_reference"] = *configuration.Project.MarketingProductReference
	}
	for index, promotion := range configuration.Promotions {
		for materialIndex, reference := range promotion.BaseMaterialReferences {
			result[fmt.Sprintf("platform_configuration.promotions.%d.base_material_references.%d", index, materialIndex)] = reference
		}
		for imageIndex, reference := range promotion.ProductImageReferences {
			result[fmt.Sprintf("platform_configuration.promotions.%d.product_image_references.%d", index, imageIndex)] = reference
		}
		if promotion.LandingPageReference != nil {
			result[fmt.Sprintf("platform_configuration.promotions.%d.landing_page_reference", index)] = *promotion.LandingPageReference
		}
		if promotion.DeliveryIdentity.Mode == "douyin_account" && promotion.DeliveryIdentity.AuthorizedIdentity != nil {
			result[fmt.Sprintf("platform_configuration.promotions.%d.delivery_identity.authorized_identity", index)] = *promotion.DeliveryIdentity.AuthorizedIdentity
		}
	}
	return result
}
