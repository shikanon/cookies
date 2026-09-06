package delivery

import "testing"

func TestPlatformPreflightBlocksMarketingProductOutsideIntent(t *testing.T) {
	intent := validDeliveryIntent(t)
	intent.Payload.ProductReferences = nil
	intent.CanonicalHash = ""
	var err error
	intent, err = FinalizeDeliveryIntent(intent)
	if err != nil {
		t.Fatal(err)
	}
	configuration := validOceanEnginePlatformConfiguration(t, intent, 0)
	version := DeliveryPlanVersion{
		SchemaVersion: DeliveryPlanVersionSchemaV2, CanonicalHash: configuration.CanonicalHash,
		DeliveryIntent: &intent, PlatformConfiguration: &configuration,
	}

	checks := RunPreflight(version)
	for _, item := range checks {
		if item.Code == "marketing_product_outside_intent" {
			if item.Passed || item.Repair == nil || item.Repair.Field != "intent.product_references" {
				t.Fatalf("marketing product check = %#v", item)
			}
			return
		}
	}
	t.Fatalf("marketing product check is missing: %#v", checks)
}

func TestPlatformPreflightBlocksConfiguredReferencesOutsideIntent(t *testing.T) {
	intent := validDeliveryIntent(t)
	configuration := validOceanEnginePlatformConfiguration(t, intent, 1)
	promotion := &configuration.Payload.OceanEngine.Promotions[0]
	promotion.BaseMaterialReferences = []StableReference{resolvedReference("oceanengine", "video_material", "account:6391", "video-1")}
	promotion.ProductImageReferences = []StableReference{resolvedReference("oceanengine", "product_image", "account:6391", "image-1")}
	promotion.LandingPageReference = referencePointer(resolvedReference("oceanengine", "orange_landing_page", "account:6391", "landing-1"))
	configuration.CanonicalHash = ""
	configuration.Intent.CanonicalHash = intent.CanonicalHash
	var err error
	configuration, err = FinalizePlatformConfiguration(configuration)
	if err != nil {
		t.Fatal(err)
	}
	version := DeliveryPlanVersion{
		SchemaVersion: DeliveryPlanVersionSchemaV2, CanonicalHash: configuration.CanonicalHash,
		DeliveryIntent: &intent, PlatformConfiguration: &configuration,
	}

	want := map[string]string{
		"base_material_outside_intent": "intent.material_references",
		"product_image_outside_intent": "intent.material_references",
		"landing_page_outside_intent":  "intent.landing_page_references",
	}
	for _, item := range RunPreflight(version) {
		field, ok := want[item.Code]
		if !ok {
			continue
		}
		if item.Passed || item.Repair == nil || item.Repair.Field != field {
			t.Fatalf("%s check = %#v", item.Code, item)
		}
		delete(want, item.Code)
	}
	if len(want) != 0 {
		t.Fatalf("reference checks are missing: %#v", want)
	}
}
