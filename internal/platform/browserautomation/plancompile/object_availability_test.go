package plancompile

import (
	"strings"
	"testing"

	"github.com/shikanon/cookies/internal/systems/delivery"
)

func TestPlatformReferenceIDUsesBoundOceanEngineID(t *testing.T) {
	ref := delivery.StableReference{
		Namespace: "cookies", ObjectKind: "product", ID: "product_internal_1", State: delivery.ReferenceResolved,
		AuditAttributes: map[string]string{"ocean_engine_product_id": "7665932008710946858"},
	}
	if got := platformReferenceID(ref); got != "7665932008710946858" {
		t.Fatalf("platform reference ID = %q", got)
	}
	spec, err := stableReferenceSpec(ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	if spec["object_id"] != "7665932008710946858" {
		t.Fatalf("selection spec = %#v", spec)
	}
}

func TestPlatformReferenceIDRequiresUniqueProductIDForConnectorProduct(t *testing.T) {
	legacy := delivery.StableReference{
		Namespace: "oceanengine", ObjectKind: "product", ID: "1784863906740671489", State: delivery.ReferenceResolved,
		AuditAttributes: map[string]string{"platform_object_id": "1784863906740671489", "ocean_engine_product_id": "1784863906740671489"},
	}
	if got := platformReferenceID(legacy); got != "" {
		t.Fatalf("legacy product_id must not be executable: %q", got)
	}
	configuration := delivery.OceanEngineConfiguration{Project: &delivery.OceanEngineProjectDraft{MarketingPurpose: "ecommerce", MarketingProductReference: &legacy}}
	availability := configurationObjectAvailability(configuration)
	if len(availability) != 1 || availability[0].Available || availability[0].Reason != "当前商品绑定的是 product_id。请同步巨量对象目录后重新选择商品" {
		t.Fatalf("legacy availability = %#v", availability)
	}
	legacy.AuditAttributes["unique_product_id"] = "7665932008710946858"
	if got := platformReferenceID(legacy); got != "7665932008710946858" {
		t.Fatalf("unique product ID = %q", got)
	}
}

func TestPlatformReferenceIDRejectsUnboundCookiesObject(t *testing.T) {
	ref := delivery.StableReference{Namespace: "cookies", ObjectKind: "material", ID: "asset_internal_1", State: delivery.ReferenceResolved}
	if got := platformReferenceID(ref); got != "" {
		t.Fatalf("platform reference ID = %q", got)
	}
	if _, err := stableReferenceSpec(ref, nil); err == nil {
		t.Fatal("expected unbound reference to fail")
	}
}

func TestConfigurationObjectAvailabilityAcceptsManualDirectLink(t *testing.T) {
	link := "tbopen://m.taobao.com/tbopen/index.html?action=ali.open.nav&module=h5"
	configuration := delivery.OceanEngineConfiguration{
		Project: &delivery.OceanEngineProjectDraft{AccountReference: delivery.StableReference{ID: "account"}},
		Promotions: []delivery.OceanEnginePromotionDraft{{
			DirectLinkReference: &delivery.StableReference{
				Namespace: "cookies", ObjectKind: "direct_link", ID: link, State: delivery.ReferenceResolved,
			},
		}},
	}
	items := configurationObjectAvailability(configuration)
	if len(items) != 1 || !items[0].Available || items[0].PlatformObjectID != "" || items[0].Reason != "手动填写链接，无需绑定平台 ID" {
		t.Fatalf("manual direct-link availability = %#v", items)
	}
	if validManualDirectLink("javascript:alert(1)") {
		t.Fatal("unsafe direct-link scheme must be rejected")
	}
}

func TestConfigurationObjectAvailabilityRejectsImageMaterialAsProductImage(t *testing.T) {
	configuration := delivery.OceanEngineConfiguration{
		Project: &delivery.OceanEngineProjectDraft{AccountReference: delivery.StableReference{ID: "account"}},
		Promotions: []delivery.OceanEnginePromotionDraft{{
			ProductImageReferences: []delivery.StableReference{{
				Namespace: "oceanengine", ObjectKind: "image_material", ID: "7649703629105889290", State: delivery.ReferenceResolved,
			}},
		}},
	}
	items := configurationObjectAvailability(configuration)
	if len(items) != 1 {
		t.Fatalf("availability count = %d", len(items))
	}
	if items[0].Available || items[0].Reason != "产品主图必须来自巨量“我的图片”，不能使用图片素材" {
		t.Fatalf("availability = %#v", items[0])
	}
}

func TestConfigurationObjectAvailabilityRejectsUnqualifiedMultiConversionLandingPage(t *testing.T) {
	landing := delivery.StableReference{Namespace: "oceanengine", ObjectKind: "orange_landing_page", ID: "7047763949009535007", State: delivery.ReferenceResolved}
	configuration := delivery.OceanEngineConfiguration{
		Project: &delivery.OceanEngineProjectDraft{
			MarketingPurpose:            "lead_generation",
			LeadCaptureMode:             "smart_lead",
			Carrier:                     "orange_landing_page",
			OptimizationTargetReference: &delivery.StableReference{ID: "100", State: delivery.ReferenceResolved},
		},
		Promotions: []delivery.OceanEnginePromotionDraft{{LandingPageReference: &landing}},
	}
	items := configurationObjectAvailability(configuration)
	if len(items) != 1 || items[0].Available || !strings.Contains(items[0].Reason, "优化目标 100") {
		t.Fatalf("availability = %#v", items)
	}
	landing.AuditAttributes = map[string]string{"multi_conversion_eligible": "true"}
	items = configurationObjectAvailability(configuration)
	if len(items) != 1 || !items[0].Available {
		t.Fatalf("qualified availability = %#v", items)
	}
}

func TestConfigurationObjectAvailabilityChecksFormSubmissionLandingQualification(t *testing.T) {
	landing := delivery.StableReference{
		Namespace: "oceanengine", ObjectKind: "orange_landing_page", ID: "7047763949009535007", State: delivery.ReferenceResolved,
		AuditAttributes: map[string]string{"multi_lead_external_actions": "100"},
	}
	configuration := delivery.OceanEngineConfiguration{
		Project: &delivery.OceanEngineProjectDraft{
			MarketingPurpose:            "lead_generation",
			LeadCaptureMode:             "smart_lead",
			Carrier:                     "orange_landing_page",
			OptimizationTargetReference: &delivery.StableReference{ID: "2", State: delivery.ReferenceResolved},
		},
		Promotions: []delivery.OceanEnginePromotionDraft{{LandingPageReference: &landing}},
	}
	items := configurationObjectAvailability(configuration)
	if len(items) != 1 || items[0].Available || !strings.Contains(items[0].Reason, "优化目标 2") {
		t.Fatalf("unqualified form-submission availability = %#v", items)
	}
	landing.AuditAttributes["multi_lead_external_actions"] = "2,100"
	items = configurationObjectAvailability(configuration)
	if len(items) != 1 || !items[0].Available {
		t.Fatalf("qualified form-submission availability = %#v", items)
	}
}
