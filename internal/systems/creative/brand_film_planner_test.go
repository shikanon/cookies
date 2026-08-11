package creative

import (
	"strings"
	"testing"
)

func TestBrandFilmSourceInvocationTokenSupportsStrategyHandoff(t *testing.T) {
	source := BrandFilmSourceSnapshot{
		SourceType:           strategyBrandFilmSourceType,
		DirectionContentHash: "sha256:9e96b18466abeb0842f745b72f40c54095bb897ba2284f05d646e6688fa780fc",
	}
	token := brandFilmSourceInvocationToken(source)
	if token != "9e96b18466ab" {
		t.Fatalf("unexpected Strategy source token: %s", token)
	}
}

func TestBrandFilmPlanOutputSchemaUsesRequestedDuration(t *testing.T) {
	schema := string(brandFilmPlanOutputSchema(30))
	if !strings.Contains(schema, `"end_second":{"type":"integer","minimum":1,"maximum":30}`) ||
		strings.Contains(schema, `"end_second":{"type":"integer","minimum":1,"maximum":15}`) {
		t.Fatalf("30-second plan schema still carries a 15-second ceiling: %s", schema)
	}
}

func TestBrandBriefSchemaRequiresOneCandidatePerProduct(t *testing.T) {
	schema := string(brandBriefAnalysisSchema)
	for _, expected := range []string{`"asset_candidates"`, `"product_front"`, `"label"`, `"source_locator"`} {
		if !strings.Contains(schema, expected) {
			t.Fatalf("brand Brief schema does not require %s: %s", expected, schema)
		}
	}
}

func TestMergeKnownBrandAssetsKeepsGeneratedProductNameAndKnownAsset(t *testing.T) {
	generated := []BrandBriefAssetCandidate{
		{ID: "product_golden_repair", Role: "product_front", Label: "娇兰第三代黄金复原蜜", SourceLocator: "brief://products/golden-repair", RightsStatus: "needs_confirmation"},
		{ID: "product_bee_water", Role: "product_front", Label: "娇兰 25X 蜂皇水", SourceLocator: "brief://products/bee-water", RightsStatus: "needs_confirmation"},
	}
	known := []BrandBriefAssetCandidate{
		{ID: "asset_product_front", Role: "product_front", Label: "商品正面图", SourceLocator: "fixture://brief#image=1", FixtureURI: "/assets/product.png", RightsStatus: "needs_confirmation"},
	}
	merged := mergeKnownBrandAssets(generated, known)
	if len(merged) != 2 || merged[0].Label != "娇兰第三代黄金复原蜜" || merged[0].FixtureURI != "/assets/product.png" || merged[1].Label != "娇兰 25X 蜂皇水" {
		t.Fatalf("unexpected merged candidates: %#v", merged)
	}
}

func TestReconcileBriefProductAssetsKeepsEveryNamedProduct(t *testing.T) {
	briefText := `
#娇兰第三代黄金复原蜜#
法国娇兰对蜂蜜及其修护力逾10年专注研究的心血结晶
产品词：娇兰第三代黄金复原蜜 or 娇兰第三代复原蜜；
#25X蜂皇水#
娇兰25X蜂皇水
娇兰帝皇蜂姿面霜套组
每天晚上，在娇兰黄金复原蜜之后
#娇兰金钻修颜粉底液#
`
	generated := []BrandBriefAssetCandidate{
		{ID: "asset_product_front", Role: "product_front", Label: "商品正面图", SourceLocator: "knowledge://documents/doc_1#product-image", RightsStatus: "needs_confirmation"},
		{ID: "asset_brand_logo", Role: "logo", Label: "娇兰 Logo", SourceLocator: "knowledge://documents/doc_1#brand-logo", RightsStatus: "needs_confirmation"},
	}

	reconciled := reconcileBriefProductAssets(briefText, generated)
	labels := make([]string, 0, len(reconciled))
	for _, candidate := range reconciled {
		if candidate.Role == "product_front" {
			labels = append(labels, candidate.Label)
		}
	}
	want := []string{"娇兰第三代黄金复原蜜", "25X蜂皇水", "娇兰帝皇蜂姿面霜套组", "娇兰金钻修颜粉底液"}
	if len(labels) != len(want) {
		t.Fatalf("expected one product candidate per named product, got %#v", labels)
	}
	for index := range want {
		if labels[index] != want[index] {
			t.Fatalf("unexpected product labels: %#v", labels)
		}
	}
}
