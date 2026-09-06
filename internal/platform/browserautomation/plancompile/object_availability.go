package plancompile

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/shikanon/cookies/internal/systems/delivery"
)

const unavailablePlatformObjectsReason = "PLATFORM_OBJECTS_UNAVAILABLE"

// V3ObjectAvailability reports whether a Cookies reference can be used by an
// OceanEngine form. It does not query or change the remote platform.
type V3ObjectAvailability struct {
	FieldKey         string `json:"field_key"`
	ObjectKind       string `json:"object_kind"`
	InternalObjectID string `json:"internal_object_id"`
	DisplayName      string `json:"display_name,omitempty"`
	PlatformObjectID string `json:"platform_object_id,omitempty"`
	Available        bool   `json:"available"`
	Reason           string `json:"reason,omitempty"`
}

func configurationObjectAvailability(configuration delivery.OceanEngineConfiguration) []V3ObjectAvailability {
	values := make([]V3ObjectAvailability, 0)
	appendReference := func(field string, ref *delivery.StableReference) {
		if ref == nil {
			return
		}
		platformID := platformReferenceID(*ref)
		if ref.ObjectKind == "owned_landing_page" && ref.State == delivery.ReferenceResolved {
			platformID = strings.TrimSpace(ref.ID)
		}
		manualDirectLink := ref.ObjectKind == "direct_link" && ref.State == delivery.ReferenceResolved && validManualDirectLink(ref.ID)
		item := V3ObjectAvailability{
			FieldKey: field, ObjectKind: ref.ObjectKind, InternalObjectID: ref.ID,
			DisplayName: ref.DisplayNameSnapshot, PlatformObjectID: platformID,
			Available: platformID != "" || manualDirectLink,
		}
		if manualDirectLink {
			item.PlatformObjectID = ""
			item.Reason = "手动填写链接，无需绑定平台 ID"
		}
		if strings.Contains(field, ".product_image_references.") && ref.ObjectKind != "product_image" {
			item.PlatformObjectID = ""
			item.Available = false
			item.Reason = "产品主图必须来自巨量“我的图片”，不能使用图片素材"
		}
		if field == "project.marketing_product_reference" && ref.Namespace == "oceanengine" && strings.TrimSpace(ref.AuditAttributes["unique_product_id"]) == "" {
			item.PlatformObjectID = ""
			item.Available = false
			item.Reason = "当前商品绑定的是 product_id。请同步巨量对象目录后重新选择商品"
		}
		if !item.Available {
			if item.Reason == "" {
				item.Reason = "未绑定巨量平台 ID"
			}
		}
		values = append(values, item)
	}
	appendReferences := func(field string, refs []delivery.StableReference) {
		for index := range refs {
			appendReference(fmt.Sprintf("%s.%d", field, index), &refs[index])
		}
	}

	project := configuration.Project
	if project == nil {
		return values
	}
	switch project.MarketingPurpose {
	case "ecommerce", "lead_generation":
		appendReference("project.marketing_product_reference", project.MarketingProductReference)
	case "application":
		appendReference("project.application_reference", project.ApplicationReference)
	case "product_catalog":
		appendReference("project.product_catalog_reference", project.ProductCatalogReference)
	}
	for promotionIndex := range configuration.Promotions {
		promotion := &configuration.Promotions[promotionIndex]
		prefix := fmt.Sprintf("promotions.%d", promotionIndex)
		if promotion.DeliveryIdentity.Mode != "account_info" {
			appendReference(prefix+".delivery_identity.authorized_identity", promotion.DeliveryIdentity.AuthorizedIdentity)
		}
		appendReferences(prefix+".base_material_references", promotion.BaseMaterialReferences)
		appendReferences(prefix+".product_image_references", promotion.ProductImageReferences)
		appendReference(prefix+".native_anchor_reference", promotion.NativeAnchorReference)
		appendReference(prefix+".landing_page_reference", promotion.LandingPageReference)
		if requiredAction := requiredMultiLeadLandingAction(*project); requiredAction != "" && promotion.LandingPageReference != nil {
			item := &values[len(values)-1]
			if item.FieldKey == prefix+".landing_page_reference" && !landingSupportsMultiLeadAction(*promotion.LandingPageReference, requiredAction) {
				item.Available = false
				item.Reason = fmt.Sprintf("当前橙子落地页不支持优化目标 %s 的多留资组件分支。请同步巨量对象后重新选择。当前账户可能没有可用落地页", requiredAction)
			}
		}
		appendReference(prefix+".direct_link_reference", promotion.DirectLinkReference)
		appendReference(prefix+".product_reference", promotion.ProductReference)
		appendReferences(prefix+".creative_component_references", promotion.CreativeComponentReferences)
		appendReference(prefix+".settings.category_reference", promotion.Settings.CategoryReference)
		appendReference(prefix+".settings.brand_reference", promotion.Settings.BrandReference)
	}
	return values
}

func requiredMultiLeadLandingAction(project delivery.OceanEngineProjectDraft) string {
	if project.MarketingPurpose != "lead_generation" || project.OptimizationTargetReference == nil {
		return ""
	}
	leadCaptureMode := strings.TrimSpace(project.LeadCaptureMode)
	if leadCaptureMode == "" {
		if project.Carrier == "owned_landing_page" || project.Carrier == "im" {
			leadCaptureMode = "custom_lead"
		} else {
			leadCaptureMode = "smart_lead"
		}
	}
	if leadCaptureMode != "smart_lead" || (project.Carrier != "orange_landing_page" && project.Carrier != "orange_landing_page_and_im") {
		return ""
	}
	return strings.TrimSpace(project.OptimizationTargetReference.ID)
}

func landingSupportsMultiLeadAction(reference delivery.StableReference, action string) bool {
	for _, value := range strings.Split(reference.AuditAttributes["multi_lead_external_actions"], ",") {
		if strings.TrimSpace(value) == action {
			return true
		}
	}
	return action == "100" && strings.TrimSpace(reference.AuditAttributes["multi_conversion_eligible"]) == "true"
}

func validManualDirectLink(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "tbopen":
		return true
	default:
		return false
	}
}

func platformReferenceID(ref delivery.StableReference) string {
	if ref.State != delivery.ReferenceResolved {
		return ""
	}
	keys := []string{"platform_object_id"}
	switch ref.ObjectKind {
	case "product":
		if ref.Namespace == "oceanengine" {
			// The project picker searches unique_product_id. product_id is a
			// source identity and cannot select a product in this form.
			value := strings.TrimSpace(ref.AuditAttributes["unique_product_id"])
			if validPlatformReferenceID(ref.ObjectKind, value) {
				return value
			}
			return ""
		}
		keys = []string{"unique_product_id", "platform_object_id", "ocean_engine_product_id"}
	case "product_catalog":
		keys = append(keys, "ocean_engine_product_id")
	case "material", "product_image":
		keys = append(keys, "ocean_engine_material_id")
	case "landing_page":
		keys = append(keys, "ocean_engine_landing_page_id")
	case "delivery_identity":
		keys = append(keys, "ocean_engine_identity_id")
	case "application":
		keys = append(keys, "ocean_engine_application_id")
	case "native_anchor":
		keys = append(keys, "ocean_engine_anchor_id")
	case "direct_link":
		keys = append(keys, "ocean_engine_direct_link_id")
	case "creative_component":
		keys = append(keys, "ocean_engine_component_id")
	case "category":
		keys = append(keys, "ocean_engine_category_id")
	case "brand":
		keys = append(keys, "ocean_engine_brand_id")
	}
	for _, key := range keys {
		if value := strings.TrimSpace(ref.AuditAttributes[key]); validPlatformReferenceID(ref.ObjectKind, value) {
			return value
		}
	}
	if validPlatformReferenceID(ref.ObjectKind, strings.TrimSpace(ref.ID)) {
		return strings.TrimSpace(ref.ID)
	}
	return ""
}

func validPlatformReferenceID(objectKind, value string) bool {
	// OceanEngine uses -1 as the real platform ID for the built-in "其他"
	// brand option. No other object kind can use a negative sentinel here.
	return numericReference(value) || objectKind == "brand" && value == "-1"
}
