package main

import (
	"github.com/shikanon/cookies/internal/platform/connector"
	"github.com/shikanon/cookies/internal/systems/delivery"
	"testing"
)

func TestFillingObjectReferencesPreservePlatformIdentity(t *testing.T) {
	product := fillingObjectReference(connector.PlatformObject{ID: "internal", AccountID: "account", Kind: connector.PlatformObjectMarketingProduct, PlatformObjectID: "fallback", Version: 7, Metadata: map[string]any{"unique_product_id": float64(123), "product_id": "456"}})
	if product.ID != "123" || product.Version != "7" || product.ObjectKind != "product" || product.AuditAttributes["product_id"] != "456" || product.Scope != "account:account" {
		t.Fatalf("%+v", product)
	}
	video := fillingObjectReference(connector.PlatformObject{ID: "video", AccountID: "account", Kind: connector.PlatformObjectDouyinVideo, PlatformObjectID: "aweme", Version: 2, Metadata: map[string]any{"ies_core_user_id": "100", "video_id": "video-id"}})
	if video.AuditAttributes["ies_core_user_id"] != "100" || video.AuditAttributes["video_id"] != "video-id" {
		t.Fatal(video)
	}
}

func TestFillingLandingPagesRequireCurrentOptimizationQualification(t *testing.T) {
	p := delivery.OceanEngineProjectDraft{MarketingPurpose: "lead_generation", Carrier: "orange_landing_page", OptimizationTargetReference: &delivery.StableReference{ID: "19"}}
	if fillingLandingEligible(p, map[string]any{"multi_conversion_eligible": true}) {
		t.Fatal("unqualified landing page accepted")
	}
	if !fillingLandingEligible(p, map[string]any{"multi_lead_external_actions": []any{"19"}}) {
		t.Fatal("qualified landing page rejected")
	}
	p.MarketingPurpose = "ecommerce"
	p.OptimizationTargetReference.SemanticKey = "in_app_order"
	if !fillingLandingEligible(p, map[string]any{"ecommerce_external_actions": []any{float64(20)}}) {
		t.Fatal("order semantic mapping lost")
	}
	if fillingMetadataContains(nil, "orange_landing_page") {
		t.Fatal("missing optimization context accepted")
	}
}
