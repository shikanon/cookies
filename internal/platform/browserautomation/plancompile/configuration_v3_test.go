package plancompile

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/systems/delivery"
)

func TestCompileConfigurationV3CreatesAndEditsBoundObjects(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)

	created, err := CompileConfigurationV3(configuration, &intent, "1855554434276391", V3ObjectBindings{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Forms) != 2 || created.Forms[0].DependsOn != "" || created.Forms[1].DependsOn != "project-draft-1" {
		t.Fatalf("create forms = %#v", created.Forms)
	}
	assertPlanKind(t, created.Forms[0].Plan, "project_create", "")
	assertPlanKind(t, created.Forms[1].Plan, "promotion_create", "")
	if len(created.Forms[0].Diff) == 0 || len(created.Forms[1].Diff) == 0 {
		t.Fatal("generated plans must show field differences")
	}

	edited, err := CompileConfigurationV3(configuration, &intent, "1855554434276391", V3ObjectBindings{
		ProjectPlatformID: "7677595885572784182", PromotionPlatformIDs: map[string]string{"promotion-draft-1": "7683558668450021382"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	assertPlanKind(t, edited.Forms[0].Plan, "project_edit", "7677595885572784182")
	assertPlanKind(t, edited.Forms[1].Plan, "promotion_edit", "7683558668450021382")
	if edited.Forms[1].DependsOn != "" {
		t.Fatalf("bound promotion dependency = %q", edited.Forms[1].DependsOn)
	}
}

func TestCompileConfigurationV3UsesCalibratedProjectDefaults(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)
	configuration.Payload.OceanEngine.Project.DeepOptimizationMode = ""
	configuration.Payload.OceanEngine.Project.PlacementStrategy = ""

	compiled, err := CompileConfigurationV3(configuration, &intent, "1855554434276391", V3ObjectBindings{}, now)
	if err != nil {
		t.Fatal(err)
	}
	var plan v3Plan
	if err := json.Unmarshal(compiled.Forms[0].Plan, &plan); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"project.deep_optimization_mode": "不启用",
		"project.placement_strategy":     "通投智选",
	}
	for _, step := range plan.Steps {
		expected, ok := want[step.FieldKey]
		if !ok {
			continue
		}
		if step.ValueState != "provided" || step.Value != expected {
			t.Fatalf("%s step = %#v", step.FieldKey, step)
		}
		delete(want, step.FieldKey)
	}
	if len(want) != 0 {
		t.Fatalf("default project steps are missing: %#v", want)
	}
}

func TestCompileConfigurationV3RequiresPangleCoefficientOnlyForPangle(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)
	project := configuration.Payload.OceanEngine.Project
	project.PlacementStrategy = "preferred_media"
	project.PlacementMedia = []string{"douyin"}

	compiled, err := CompileConfigurationV3(configuration, &intent, "1855554434276391", V3ObjectBindings{}, now)
	if err != nil {
		t.Fatal(err)
	}
	var projectPlan, promotionPlan v3Plan
	if err := json.Unmarshal(compiled.Forms[0].Plan, &projectPlan); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(compiled.Forms[1].Plan, &promotionPlan); err != nil {
		t.Fatal(err)
	}
	for _, step := range projectPlan.Steps {
		if step.FieldKey == "project.placement_media" {
			media, ok := step.Value.([]any)
			if !ok || len(media) != 1 || media[0] != "抖音" {
				t.Fatalf("placement media = %#v", step.Value)
			}
		}
	}
	for _, step := range promotionPlan.Steps {
		if step.FieldKey == "promotion.pangle_bid_coefficient" {
			t.Fatalf("Douyin-only plan contains Pangle coefficient: %#v", promotionPlan.Steps)
		}
	}
}

func TestCompileConfigurationV3NormalizesButtonRedirectOptimizationTarget(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)
	configuration.Payload.OceanEngine.Project.OptimizationTargetReference.SemanticKey = "button_redirect"

	compiled, err := CompileConfigurationV3(configuration, &intent, "1855554434276391", V3ObjectBindings{}, now)
	if err != nil {
		t.Fatal(err)
	}
	var projectPlan v3Plan
	if err := json.Unmarshal(compiled.Forms[0].Plan, &projectPlan); err != nil {
		t.Fatal(err)
	}
	if projectPlan.ParentContext.OptimizationTarget != "button_jump" {
		t.Fatalf("optimization target = %q", projectPlan.ParentContext.OptimizationTarget)
	}
}

func TestCompileConfigurationV3UsesEnumeratedLeadGenerationPath(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)
	project := configuration.Payload.OceanEngine.Project
	project.MarketingPurpose = "lead_generation"
	project.MarketingScenario = "short_video_image_text"
	project.LeadCaptureMode = "custom_lead"
	project.OptimizationTargetReference.SemanticKey = "button_redirect"
	project.OptimizationTargetReference.AuditAttributes = map[string]string{}
	project.OptimizationTargetReference.AuditAttributes["capability_snapshot_id"] = "oecap_lead"
	project.OptimizationTargetReference.AuditAttributes["capability_context_hash"] = strings.Repeat("a", 64)
	project.BudgetAndBidding.BudgetMode = delivery.OceanEngineBudgetModeDaily
	project.BudgetAndBidding.DailyBudgetMinor = 30000
	project.BudgetAndBidding.BiddingStrategy = "cost_cap"
	minimumBid := int64(1)
	project.BudgetAndBidding.BidMinor = &minimumBid

	compiled, err := CompileConfigurationV3(configuration, &intent, "1855554434276391", V3ObjectBindings{}, now)
	if err != nil {
		t.Fatal(err)
	}
	var projectPlan v3Plan
	if err := json.Unmarshal(compiled.Forms[0].Plan, &projectPlan); err != nil {
		t.Fatal(err)
	}
	provided := map[string]any{}
	for _, step := range projectPlan.Steps {
		if step.ValueState == "provided" {
			provided[step.FieldKey] = step.Value
		}
	}
	if provided["project.marketing_purpose"] != "销售线索" || provided["project.lead_capture_mode"] != "自定义" {
		t.Fatalf("lead-generation values = %#v", provided)
	}
	if provided["project.daily_budget"] != "300.00" || provided["project.bid"] != "0.01" {
		t.Fatalf("lead-generation budget values = %#v", provided)
	}
	for _, step := range projectPlan.Steps {
		if step.FieldKey == "project.bid" {
			if step.MoneyConstraint == nil || step.MoneyConstraint.ChargingMode != "OCPM" || step.MoneyConstraint.MinimumMinor != 1 || step.MoneyConstraint.MaximumMinor != 30000 {
				t.Fatalf("lead-generation bid constraint = %#v", step.MoneyConstraint)
			}
		}
	}
	if _, ok := provided["project.marketing_product_reference"]; !ok {
		t.Fatalf("lead-generation plan has no marketing product: %#v", provided)
	}
	if projectPlan.ParentContext.DeliveryMode != "ubmax" {
		t.Fatalf("lead-generation delivery mode = %q", projectPlan.ParentContext.DeliveryMode)
	}
	for _, excluded := range []string{"project.delivery_mode", "project.placement_strategy", "project.search_bid_coefficient", "project.budget_mode"} {
		if _, ok := provided[excluded]; ok {
			t.Fatalf("lead-generation plan contains %s: %#v", excluded, provided)
		}
	}
	availability := configurationObjectAvailability(*configuration.Payload.OceanEngine)
	productAvailable := false
	for _, item := range availability {
		if item.FieldKey == "project.marketing_product_reference" {
			productAvailable = item.Available && item.PlatformObjectID == "1001"
		}
	}
	if !productAvailable {
		t.Fatalf("lead-generation product availability = %#v", availability)
	}
}

func TestCompileConfigurationV3RejectsUnlimitedSalesLeadBudget(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)
	project := configuration.Payload.OceanEngine.Project
	project.MarketingPurpose = "lead_generation"
	project.BudgetAndBidding.BudgetMode = delivery.OceanEngineBudgetModeUnlimited
	project.BudgetAndBidding.DailyBudgetMinor = 0

	_, err := CompileConfigurationV3(configuration, &intent, "1855554434276391", V3ObjectBindings{}, now)
	if err == nil || !strings.Contains(err.Error(), "sales-lead projects require a daily budget") {
		t.Fatalf("error = %v", err)
	}
}

func TestCompileConfigurationV3InfersLegacyLeadCaptureModeFromCarrier(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)
	project := configuration.Payload.OceanEngine.Project
	project.MarketingPurpose = "lead_generation"
	project.MarketingScenario = "short_video_image_text"
	project.LeadCaptureMode = ""
	project.Carrier = "owned_landing_page"
	project.OptimizationTargetReference.SemanticKey = "click"
	project.OptimizationTargetReference.AuditAttributes = map[string]string{}
	project.OptimizationTargetReference.AuditAttributes["capability_snapshot_id"] = "oecap_lead"
	project.OptimizationTargetReference.AuditAttributes["capability_context_hash"] = strings.Repeat("a", 64)
	project.DeliveryMode = "ubmax"

	compiled, err := CompileConfigurationV3(configuration, &intent, "1855554434276391", V3ObjectBindings{}, now)
	if err != nil {
		t.Fatal(err)
	}
	var projectPlan v3Plan
	if err := json.Unmarshal(compiled.Forms[0].Plan, &projectPlan); err != nil {
		t.Fatal(err)
	}
	for _, step := range projectPlan.Steps {
		if step.FieldKey == "project.lead_capture_mode" && step.Value == "自定义" {
			return
		}
	}
	t.Fatalf("legacy lead capture mode was not inferred: %#v", projectPlan.Steps)
}

func TestCompileConfigurationV3UsesAccountCapabilityForOwnedLandingPageLeadPath(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)
	project := configuration.Payload.OceanEngine.Project
	project.MarketingPurpose = "lead_generation"
	project.MarketingScenario = "short_video_image_text"
	project.LeadCaptureMode = "custom_lead"
	project.Carrier = "owned_landing_page"
	project.OptimizationTargetReference.SemanticKey = "form"
	project.OptimizationTargetReference.DisplayNameSnapshot = "表单提交"
	project.OptimizationTargetReference.AuditAttributes = map[string]string{}
	project.OptimizationTargetReference.AuditAttributes["capability_snapshot_id"] = "oecap_lead"
	project.OptimizationTargetReference.AuditAttributes["capability_context_hash"] = strings.Repeat("a", 64)
	project.DeliveryMode = "manual"

	compiled, err := CompileConfigurationV3(configuration, &intent, "1855554434276391", V3ObjectBindings{}, now)
	if err != nil {
		t.Fatal(err)
	}
	var projectPlan v3Plan
	if err := json.Unmarshal(compiled.Forms[0].Plan, &projectPlan); err != nil {
		t.Fatal(err)
	}
	if projectPlan.ParentContext.OptimizationTarget != "form" {
		t.Fatalf("parent context = %#v", projectPlan.ParentContext)
	}
}

func TestCompileConfigurationV3FillsOwnedLandingPageLink(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)
	configuration.Payload.OceanEngine.Project.Carrier = "owned_landing_page"
	configuration.Payload.OceanEngine.Promotions[0].LandingPageReference = &delivery.StableReference{
		Namespace: "cookies", ObjectKind: "owned_landing_page", Scope: "current_project",
		ID: "https://example.test/landing", State: delivery.ReferenceResolved,
	}

	compiled, err := CompileConfigurationV3(configuration, &intent, "1855554434276391", V3ObjectBindings{}, now)
	if err != nil {
		t.Fatal(err)
	}
	var promotionPlan v3Plan
	if err := json.Unmarshal(compiled.Forms[1].Plan, &promotionPlan); err != nil {
		t.Fatal(err)
	}
	for _, step := range promotionPlan.Steps {
		if step.FieldKey == "promotion.landing_page_reference" {
			if step.Operation != "fill_text" || step.Target != "请选择或填写自研落地页链接" || step.Value != "https://example.test/landing" {
				t.Fatalf("owned landing-page step = %#v", step)
			}
			availability := configurationObjectAvailability(*configuration.Payload.OceanEngine)
			ownedAvailable := false
			for _, item := range availability {
				if item.FieldKey == "promotions.0.landing_page_reference" && item.Available {
					ownedAvailable = true
				}
			}
			if !ownedAvailable {
				t.Fatalf("owned landing-page availability = %#v", availability)
			}
			return
		}
	}
	t.Fatal("owned landing-page step is missing")
}

func TestCompileConfigurationV3FillsManualDirectLink(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)
	link := "tbopen://m.taobao.com/tbopen/index.html?action=ali.open.nav&module=h5"
	promotion := &configuration.Payload.OceanEngine.Promotions[0]
	promotion.Settings.DirectLinkMode = "manual"
	promotion.DirectLinkReference = &delivery.StableReference{
		Namespace: "cookies", ObjectKind: "direct_link", Scope: "current_project",
		ID: link, State: delivery.ReferenceResolved,
	}

	compiled, err := CompileConfigurationV3(configuration, &intent, "1855554434276391", V3ObjectBindings{}, now)
	if err != nil {
		t.Fatal(err)
	}
	var promotionPlan v3Plan
	if err := json.Unmarshal(compiled.Forms[1].Plan, &promotionPlan); err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		operation string
		value     string
	}{
		"promotion.direct_link_mode":      {operation: "choose_exact_visible_option", value: "手动填写"},
		"promotion.direct_link_reference": {operation: "fill_text", value: link},
	}
	for _, step := range promotionPlan.Steps {
		expected, ok := want[step.FieldKey]
		if !ok {
			continue
		}
		if step.Operation != expected.operation || step.Value != expected.value || step.ValueState != "provided" {
			t.Fatalf("%s step = %#v", step.FieldKey, step)
		}
		delete(want, step.FieldKey)
	}
	if len(want) != 0 {
		t.Fatalf("manual direct-link steps are missing: %#v", want)
	}
}

func TestCompileConfigurationV3AcceptsOtherBrandSentinel(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)
	configuration.Payload.OceanEngine.Promotions[0].Settings.BrandReference = &delivery.StableReference{
		Namespace: "oceanengine", ObjectKind: "brand", Scope: "account:oeacct_safe",
		ID: "-1", State: delivery.ReferenceResolved, DisplayNameSnapshot: "其他",
		AuditAttributes: map[string]string{"platform_object_id": "-1"},
	}

	availability := configurationObjectAvailability(*configuration.Payload.OceanEngine)
	for _, item := range availability {
		if item.FieldKey == "promotions.0.settings.brand_reference" {
			if !item.Available || item.PlatformObjectID != "-1" {
				t.Fatalf("other brand availability = %#v", item)
			}
			if _, err := CompileConfigurationV3(configuration, &intent, "1855554434276391", V3ObjectBindings{}, now); err != nil {
				t.Fatalf("compile other brand: %v", err)
			}
			return
		}
	}
	t.Fatalf("other brand availability is missing: %#v", availability)
}

func TestCompileConfigurationV3KeepsPromotionMaterialFieldStructure(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)
	configuration.Payload.OceanEngine.Promotions[0].NativeAnchorReference = &delivery.StableReference{
		Namespace: "oceanengine", ObjectKind: "native_anchor", Scope: "account:1855554434276391",
		ID: "6001", State: delivery.ReferenceResolved, DisplayNameSnapshot: "测试原生锚点",
	}

	compiled, err := CompileConfigurationV3(configuration, &intent, "1855554434276391", V3ObjectBindings{}, now)
	if err != nil {
		t.Fatal(err)
	}
	var plan v3Plan
	if err := json.Unmarshal(compiled.Forms[1].Plan, &plan); err != nil {
		t.Fatal(err)
	}
	positions := map[string]int{}
	for index, step := range plan.Steps {
		positions[step.FieldKey] = index
		if step.FieldKey == "promotion.product_name" && step.Value != "测试商品" {
			t.Fatalf("inherited product name = %#v", step.Value)
		}
		if step.FieldKey == "promotion.product_image_references" {
			value, ok := step.Value.(map[string]any)
			if !ok || value["image_src_identity"] != "tos-cn-i-example/stable-image" || value["index"] != nil {
				t.Fatalf("product image identity = %#v", step.Value)
			}
		}
	}
	ordered := []string{"promotion.base_materials", "promotion.copy_materials", "promotion.native_anchor_reference", "promotion.landing_page_reference", "promotion.product_name", "promotion.product_image_references", "promotion.product_selling_points", "promotion.call_to_action", "promotion.source_label", "promotion.comments_enabled", "promotion.category", "promotion.brand_reference"}
	for index := 1; index < len(ordered); index++ {
		if positions[ordered[index-1]] >= positions[ordered[index]] {
			t.Fatalf("promotion field order = %#v", positions)
		}
	}
}

func TestImpressionPromotionOmitsUnavailableDirectLinkControls(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)
	project := configuration.Payload.OceanEngine.Project
	promotion := configuration.Payload.OceanEngine.Promotions[0]
	project.OptimizationTargetReference.SemanticKey = "impression"
	promotion.Settings.DirectLinkMode = "manual"
	promotion.DirectLinkReference = nil

	values, err := promotionPlanValues(promotion, *project, &intent)
	if err != nil {
		t.Fatal(err)
	}
	if values["promotion.direct_link_mode"] != nil || values["promotion.direct_link_reference"] != nil {
		t.Fatalf("impression promotion direct-link values = %#v", values)
	}
	for _, field := range orderedPromotionFields(*project, promotion) {
		if field.Key == "promotion.direct_link_mode" || field.Key == "promotion.direct_link_reference" {
			t.Fatalf("impression promotion contains unavailable field %q", field.Key)
		}
	}
}

func TestV3BindingsFromMappingsUsesConfirmedObjectsAndSkipsPendingStages(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, _ := executableConfigurationFixture(now)
	mappings := []delivery.PlatformEntityMapping{
		{ID: "mapping-stale-project", ConfigurationID: "configuration-previous", AccountReferenceID: "1855554434276391", InternalObjectKind: "project", InternalObjectID: "project-draft-1", PlatformObjectKind: "project", PlatformObjectID: "7677595885572784999", Status: delivery.PlatformEntityMappingConfirmed},
		{ID: "mapping-project", ConfigurationID: configuration.ConfigurationID, AccountReferenceID: "1855554434276391", InternalObjectKind: "project", InternalObjectID: "project-draft-1", PlatformObjectKind: "project", PlatformObjectID: "7677595885572784182", Status: delivery.PlatformEntityMappingConfirmed},
		{ID: "mapping-promotion", ConfigurationID: configuration.ConfigurationID, AccountReferenceID: "1855554434276391", InternalObjectKind: "promotion", InternalObjectID: "promotion-draft-1", PlatformObjectKind: "promotion", PlatformObjectID: "7683558668450021382", Status: delivery.PlatformEntityMappingConfirmed},
	}
	staleOnly, err := V3BindingsFromMappings(configuration, "1855554434276391", mappings[:1])
	if err != nil || staleOnly.ProjectPlatformID != "" || len(staleOnly.PromotionPlatformIDs) != 0 {
		t.Fatalf("stale bindings=%#v err=%v", staleOnly, err)
	}
	bindings, err := V3BindingsFromMappings(configuration, "1855554434276391", mappings)
	if err != nil || bindings.ProjectPlatformID != "7677595885572784182" || bindings.PromotionPlatformIDs["promotion-draft-1"] != "7683558668450021382" {
		t.Fatalf("bindings=%#v err=%v", bindings, err)
	}
	mappings[2].Status = delivery.PlatformEntityMappingPending
	pending, err := V3BindingsFromMappings(configuration, "1855554434276391", mappings)
	if err != nil || pending.ProjectPlatformID == "" || pending.PromotionPlatformIDs["promotion-draft-1"] != "" {
		t.Fatalf("pending bindings=%#v err=%v", pending, err)
	}
	mappings = append(mappings, delivery.PlatformEntityMapping{ID: "other-plan", AccountReferenceID: "1855554434276391", InternalObjectKind: "promotion", InternalObjectID: "other-draft", PlatformObjectKind: "promotion", PlatformObjectID: "7683558668450021999", Status: delivery.PlatformEntityMappingConfirmed})
	if _, err := V3BindingsFromMappings(configuration, "1855554434276391", mappings); err != nil {
		t.Fatalf("unrelated mapping blocked current configuration: %v", err)
	}
}

func TestCompileConfigurationV3RejectsLimitsReferencesAndAccountPaths(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		mutate  func(*delivery.PlatformConfiguration, *delivery.DeliveryIntent)
		account string
		want    string
	}{
		{"account", func(*delivery.PlatformConfiguration, *delivery.DeliveryIntent) {}, "1855554434276392", "unsupported account path"},
		{"budget", func(c *delivery.PlatformConfiguration, _ *delivery.DeliveryIntent) {
			c.Payload.OceanEngine.Project.BudgetAndBidding.DailyBudgetMinor = 29999
		}, "1855554434276391", "at least CNY 300"},
		{"bid", func(c *delivery.PlatformConfiguration, _ *delivery.DeliveryIntent) {
			value := int64(0)
			c.Payload.OceanEngine.Promotions[0].BudgetAndBidding.BidMinor = &value
		}, "1855554434276391", "bid is outside"},
		{"cost cap project bid", func(c *delivery.PlatformConfiguration, _ *delivery.DeliveryIntent) {
			value := int64(30001)
			c.Payload.OceanEngine.Project.BudgetAndBidding.BiddingStrategy = "cost_cap"
			c.Payload.OceanEngine.Project.BudgetAndBidding.ChargingMode = "CPM"
			c.Payload.OceanEngine.Project.BudgetAndBidding.BidMinor = &value
		}, "1855554434276391", "project: bid is outside"},
		{"impression project bid", func(c *delivery.PlatformConfiguration, _ *delivery.DeliveryIntent) {
			value := int64(1)
			c.Payload.OceanEngine.Project.OptimizationTargetReference.SemanticKey = ""
			c.Payload.OceanEngine.Project.OptimizationTargetReference.DisplayNameSnapshot = "展示量"
			c.Payload.OceanEngine.Project.BudgetAndBidding.BiddingStrategy = "cost_cap"
			c.Payload.OceanEngine.Project.BudgetAndBidding.BidMinor = &value
		}, "1855554434276391", "expected CNY 4.00 to 100.00"},
		{"date", func(c *delivery.PlatformConfiguration, _ *delivery.DeliveryIntent) {
			c.Payload.OceanEngine.Project.Schedule.StartAt = now
		}, "1855554434276391", "must be no earlier than"},
		{"material", func(c *delivery.PlatformConfiguration, _ *delivery.DeliveryIntent) {
			c.Payload.OceanEngine.Promotions[0].BaseMaterialReferences[0].State = delivery.ReferenceBlocked
		}, "1855554434276391", "not resolved"},
		{"product image identity", func(c *delivery.PlatformConfiguration, _ *delivery.DeliveryIntent) {
			delete(c.Payload.OceanEngine.Promotions[0].ProductImageReferences[0].AuditAttributes, "image_src_identity")
		}, "1855554434276391", "stable image_src_identity"},
		{"landing intent", func(c *delivery.PlatformConfiguration, i *delivery.DeliveryIntent) {
			i.Payload.LandingPageReferences = nil
		}, "1855554434276391", "outside the delivery intent"},
		{"manual direct link", func(c *delivery.PlatformConfiguration, _ *delivery.DeliveryIntent) {
			c.Payload.OceanEngine.Promotions[0].Settings.DirectLinkMode = "manual"
			c.Payload.OceanEngine.Promotions[0].DirectLinkReference = &delivery.StableReference{
				Namespace: "cookies", ObjectKind: "direct_link", ID: "javascript:alert(1)", State: delivery.ReferenceResolved,
			}
		}, "1855554434276391", "has no OceanEngine platform ID"},
		{"owned carrier with Orange landing page", func(c *delivery.PlatformConfiguration, _ *delivery.DeliveryIntent) {
			c.Payload.OceanEngine.Project.Carrier = "owned_landing_page"
			c.Payload.OceanEngine.Promotions[0].LandingPageReference.ObjectKind = "orange_landing_page"
		}, "1855554434276391", "cannot use an Orange landing page"},
		{"brand", func(c *delivery.PlatformConfiguration, _ *delivery.DeliveryIntent) {
			c.Payload.OceanEngine.Promotions[0].Settings.BrandReference.State = delivery.ReferenceUnresolved
		}, "1855554434276391", "not resolved"},
		{"category", func(c *delivery.PlatformConfiguration, _ *delivery.DeliveryIntent) {
			c.Payload.OceanEngine.Promotions[0].Settings.CategoryReference.State = delivery.ReferenceUnresolved
		}, "1855554434276391", "not resolved"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configuration, intent := executableConfigurationFixture(now)
			test.mutate(&configuration, &intent)
			_, err := CompileConfigurationV3(configuration, &intent, test.account, V3ObjectBindings{}, now)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCompileConfigurationV3AcceptsNextShanghaiDayBoundary(t *testing.T) {
	now := time.Date(2026, 8, 29, 13, 58, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)
	shanghai := time.FixedZone("Asia/Shanghai", 8*60*60)
	configuration.Payload.OceanEngine.Project.Schedule.StartAt = time.Date(2026, 8, 30, 0, 0, 0, 0, shanghai)
	configuration.Payload.OceanEngine.Project.Schedule.EndAt = time.Date(2026, 8, 31, 23, 59, 59, 0, shanghai)
	intent.Payload.ScheduleBoundary.EarliestStart = configuration.Payload.OceanEngine.Project.Schedule.StartAt
	intent.Payload.ScheduleBoundary.LatestEnd = configuration.Payload.OceanEngine.Project.Schedule.EndAt

	compiled, err := CompileConfigurationV3(configuration, &intent, "1855554434276391", V3ObjectBindings{}, now)
	if err != nil {
		t.Fatal(err)
	}
	var projectPlan v3Plan
	if err := json.Unmarshal(compiled.Forms[0].Plan, &projectPlan); err != nil {
		t.Fatal(err)
	}
	for _, step := range projectPlan.Steps {
		if step.FieldKey == "project.schedule" {
			value, ok := step.Value.(map[string]any)
			if !ok || value["start"] != "2026-08-30" || value["end"] != "2026-08-31" {
				t.Fatalf("project schedule = %#v", step.Value)
			}
			return
		}
	}
	t.Fatal("project schedule step is missing")
}

func executableConfigurationFixture(now time.Time) (delivery.PlatformConfiguration, delivery.DeliveryIntent) {
	ref := func(kind, id, label string) delivery.StableReference {
		return delivery.StableReference{Namespace: "oceanengine", ObjectKind: kind, Scope: "account:1855554434276391", ID: id, State: delivery.ReferenceResolved, DisplayNameSnapshot: label}
	}
	product := ref("product", "1001", "测试商品")
	product.AuditAttributes = map[string]string{"unique_product_id": "1001", "product_id": "9001"}
	material := ref("material", "2001", "测试视频")
	image := ref("product_image", "2002", "测试图片")
	image.AuditAttributes = map[string]string{"selection_kind": "image_card", "image_src_identity": "tos-cn-i-example/stable-image", "minimum_visible": "1"}
	landing := ref("landing_page", "3001", "我的落地页")
	brand := ref("brand", "4001", "测试品牌")
	category := ref("category", "5001", "零售/综合类2C电商")
	optimization := ref("optimization_target", "9001", "app内下单")
	optimization.SemanticKey = "in_app_order"
	start := now.Add(48 * time.Hour)
	end := start.Add(7 * 24 * time.Hour)
	minimum, maximum := int64(30000), int64(1000000)
	bid := int64(1)
	searchCoefficient := 1.0
	searchExpansion := true
	comments := false
	configuration := delivery.PlatformConfiguration{ConfigurationID: "configuration-1", CanonicalHash: strings.Repeat("c", 64), Platform: delivery.DeliveryPlatformOceanEngine, Payload: delivery.PlatformConfigurationPayload{OceanEngine: &delivery.OceanEngineConfiguration{Project: &delivery.OceanEngineProjectDraft{
		ProjectDraftID: "project-draft-1", AccountReference: ref("account", "1855554434276391", "测试账户"), MarketingPurpose: "ecommerce", MarketingScenario: "short_video_image_text", MarketingProductReference: &product, Carrier: "orange_landing_page", OptimizationTargetReference: &optimization, DeepOptimizationMode: "disabled", DeliveryMode: "manual", PlacementStrategy: "automatic", Schedule: delivery.OceanEngineSchedule{StartAt: start, EndAt: end, Timezone: "Asia/Shanghai"}, BudgetAndBidding: delivery.OceanEngineBudgetAndBidding{BudgetMode: delivery.OceanEngineBudgetModeDaily, Currency: "CNY", DailyBudgetMinor: 30000, ChargingMode: "CPC", BidMinor: &bid}, SearchBoost: &delivery.OceanEngineSearchBoost{BidCoefficient: &searchCoefficient, TargetingExpansion: &searchExpansion}, ProjectName: "一次性测试项目",
	}, Promotions: []delivery.OceanEnginePromotionDraft{{PromotionDraftID: "promotion-draft-1", DeliveryIdentity: delivery.OceanEngineDeliveryIdentity{Mode: "account_info"}, BaseMaterialReferences: []delivery.StableReference{material}, CopyItems: []delivery.OceanEngineCopyItem{{Text: "测试文案"}}, ProductImageReferences: []delivery.StableReference{image}, ProductSellingPoints: []string{"限时好物"}, LandingPageReference: &landing, Settings: delivery.OceanEnginePromotionSettings{CallToAction: []string{"立即预订"}, SourceLabel: "测试来源", CommentsEnabled: &comments, CategoryReference: &category, BrandReference: &brand}, BudgetAndBidding: &delivery.OceanEngineBudgetAndBidding{Currency: "CNY", DailyBudgetMinor: 30000, ChargingMode: "CPC", BidMinor: &bid}, PromotionName: "一次性测试单元"}}}}}
	intent := delivery.DeliveryIntent{Payload: delivery.DeliveryIntentPayload{BudgetBoundary: delivery.IntentBudgetBoundary{Currency: "CNY", MinimumDailyMinor: &minimum, MaximumDailyMinor: &maximum}, ScheduleBoundary: delivery.IntentScheduleBoundary{EarliestStart: start.Add(-time.Hour), LatestEnd: end.Add(time.Hour), Timezone: "Asia/Shanghai"}, ProductReferences: []delivery.StableReference{product}, LandingPageReferences: []delivery.StableReference{landing}, MaterialReferences: []delivery.StableReference{material, image}}}
	return configuration, intent
}

func assertPlanKind(t *testing.T, raw json.RawMessage, kind, objectID string) {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if value["plan_kind"] != kind {
		t.Fatalf("plan kind = %v", value["plan_kind"])
	}
	if objectID != "" && value["object_reference"] != objectID {
		t.Fatalf("object reference = %v", value["object_reference"])
	}
}
