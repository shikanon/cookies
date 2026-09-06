package plancompile

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/browserautomation/rparunner"
	"github.com/shikanon/cookies/internal/platform/oceanengineconstraints"
	"github.com/shikanon/cookies/internal/systems/delivery"
)

const configurationPlanSetV1 = "oceanengine-configuration-plan-set/v1"

// V3ObjectBindings bind Cookies draft IDs to confirmed OceanEngine IDs.
// A missing binding selects a create form. A present binding selects an edit form.
type V3ObjectBindings struct {
	ProjectPlatformID    string            `json:"project_platform_id,omitempty"`
	PromotionPlatformIDs map[string]string `json:"promotion_platform_ids,omitempty"`
}

type V3FieldDifference struct {
	FieldKey  string `json:"field_key"`
	Operation string `json:"operation"`
	Target    any    `json:"target"`
}

type V3PlannedForm struct {
	InternalObjectKind string              `json:"internal_object_kind"`
	InternalObjectID   string              `json:"internal_object_id"`
	PlatformObjectID   string              `json:"platform_object_id,omitempty"`
	DependsOn          string              `json:"depends_on,omitempty"`
	Plan               json.RawMessage     `json:"plan"`
	Diff               []V3FieldDifference `json:"diff"`
}

type V3ConfigurationPlanSet struct {
	SchemaVersion     string          `json:"schema_version"`
	ConfigurationID   string          `json:"configuration_id"`
	ConfigurationHash string          `json:"configuration_hash"`
	AccountReference  string          `json:"account_reference"`
	Forms             []V3PlannedForm `json:"forms"`
}

// V3BindingsFromMappings selects confirmed mappings for the configured
// objects and account. It ignores mappings for other plans in the project.
// It never treats an internal draft ID as a platform object ID.
func V3BindingsFromMappings(configuration delivery.PlatformConfiguration, account string, mappings []delivery.PlatformEntityMapping) (V3ObjectBindings, error) {
	bindings := V3ObjectBindings{PromotionPlatformIDs: map[string]string{}}
	ocean := configuration.Payload.OceanEngine
	if ocean == nil || ocean.Project == nil {
		return V3ObjectBindings{}, fmt.Errorf("configuration has no OceanEngine project")
	}
	for _, mapping := range mappings {
		if mapping.ConfigurationID != configuration.ConfigurationID {
			continue
		}
		isProject := mapping.InternalObjectKind == "project" && mapping.InternalObjectID == ocean.Project.ProjectDraftID
		isPromotion := mapping.InternalObjectKind == "promotion" && slices.ContainsFunc(ocean.Promotions, func(value delivery.OceanEnginePromotionDraft) bool {
			return value.PromotionDraftID == mapping.InternalObjectID
		})
		if !isProject && !isPromotion {
			continue
		}
		if mapping.Status == delivery.PlatformEntityMappingPending {
			continue
		}
		if mapping.Status != delivery.PlatformEntityMappingConfirmed || mapping.AccountReferenceID != account || !numericReference(mapping.PlatformObjectID) {
			return V3ObjectBindings{}, fmt.Errorf("platform mapping %s is not a confirmed binding for this configuration", mapping.ID)
		}
		switch mapping.InternalObjectKind {
		case "project":
			if mapping.PlatformObjectKind != "project" || bindings.ProjectPlatformID != "" {
				return V3ObjectBindings{}, fmt.Errorf("platform mapping %s does not bind the configured project", mapping.ID)
			}
			bindings.ProjectPlatformID = mapping.PlatformObjectID
		case "promotion":
			if mapping.PlatformObjectKind != "promotion" {
				return V3ObjectBindings{}, fmt.Errorf("platform mapping %s does not bind a configured promotion", mapping.ID)
			}
			if bindings.PromotionPlatformIDs[mapping.InternalObjectID] != "" {
				return V3ObjectBindings{}, fmt.Errorf("promotion %s has duplicate platform bindings", mapping.InternalObjectID)
			}
			bindings.PromotionPlatformIDs[mapping.InternalObjectID] = mapping.PlatformObjectID
		}
	}
	return bindings, nil
}

type fieldSpec struct {
	Key, Operation, Scope, Target string
	Required                      bool
}

var projectSpecs = map[string]fieldSpec{
	"project.marketing_purpose":             {"project.marketing_purpose", "choose_exact_visible_option", "营销目的", "电商", true},
	"project.marketing_scenario":            {"project.marketing_scenario", "choose_exact_visible_option", "营销场景", "短视频+图文", true},
	"project.marketing_product_reference":   {"project.marketing_product_reference", "open_reference_picker", "营销产品", "更换", true},
	"project.lead_capture_mode":             {"project.lead_capture_mode", "choose_exact_visible_option", "获取线索方式", "智能优选", true},
	"project.application_reference":         {"project.application_reference", "open_reference_picker", "应用", "请输入应用下载链接或选择已有应用", true},
	"project.application_scenario":          {"project.application_scenario", "choose_exact_visible_option", "营销目的", "应用下载", true},
	"project.operating_system":              {"project.operating_system", "choose_exact_visible_option", "操作系统", "安卓", true},
	"project.application_download_mode":     {"project.application_download_mode", "choose_exact_visible_option", "下载方式", "直接下载", true},
	"project.application_launch_mode":       {"project.application_launch_mode", "choose_exact_visible_option", "调起方式", "直接调起", true},
	"project.product_catalog_reference":     {"project.product_catalog_reference", "open_reference_picker", "商品目录", "请选择或输入搜索商品目录", true},
	"project.product_targeting":             {"project.product_targeting", "configure_object", "商品投放条件", "RTA重定向", false},
	"project.carrier":                       {"project.carrier", "choose_exact_visible_option", "投放载体", "橙子落地页", true},
	"project.optimization_target_reference": {"project.optimization_target_reference", "choose_exact_visible_option", "优化目标", "请选择", true},
	"project.deep_optimization_mode":        {"project.deep_optimization_mode", "choose_exact_visible_option", "深度优化方式", "不启用", true},
	"project.delivery_mode":                 {"project.delivery_mode", "choose_exact_visible_option", "投放模式", "自动投放(UBMax)", true},
	"project.aigc_dynamic_creative":         {"project.aigc_dynamic_creative", "toggle", "素材补充方式", "AIGC动态创意", false},
	"project.placement_strategy":            {"project.placement_strategy", "choose_exact_visible_option", "投放位置", "通投智选", true},
	"project.placement_media":               {"project.placement_media", "configure_object", "媒体选择", "全选", true},
	"project.schedule":                      {"project.schedule", "configure_object", "投放时间", "设置开始和结束日期", true},
	"project.budget_mode":                   {"project.budget_mode", "choose_exact_visible_option", "项目日预算", "不限", true},
	"project.daily_budget":                  {"project.daily_budget", "fill_money", "日预算", "spinbutton", true},
	"project.bid":                           {"project.bid", "fill_money", "出价", "spinbutton", true},
	"project.roi_coefficient":               {"project.roi_coefficient", "fill_decimal", "净成交ROI系数", "spinbutton", true},
	"project.search_bid_coefficient":        {"project.search_bid_coefficient", "fill_decimal", "出价系数", "请输入", true},
	"project.search_targeting_expansion":    {"project.search_targeting_expansion", "choose_exact_visible_option", "定向拓展", "启用", false},
	"project.project_name":                  {"project.project_name", "fill_text", "项目名称", "请输入项目名称", true},
}

var promotionSpecs = map[string]fieldSpec{
	"promotion.delivery_identity":        {"promotion.delivery_identity", "open_reference_picker", "投放身份", "请选择投放抖音号", true},
	"promotion.base_materials":           {"promotion.base_materials", "open_reference_picker", "基础素材", "添加素材", true},
	"promotion.copy_materials":           {"promotion.copy_materials", "configure_object", "文案素材", "请输入5-55个字的标题或输入关键词后选择推荐标题", true},
	"promotion.direct_link_mode":         {"promotion.direct_link_mode", "choose_exact_visible_option", "直达链接生成方式", "自动生成", false},
	"promotion.direct_link_reference":    {"promotion.direct_link_reference", "open_reference_picker", "直达链接内容", "请填写Schema直达链接，保证可跳转并打开APP", false},
	"promotion.product_name":             {"promotion.product_name", "fill_text", "产品信息", "请输入", false},
	"promotion.product_image_references": {"promotion.product_image_references", "open_reference_picker", "产品主图", "产品主图", true},
	"promotion.product_selling_points":   {"promotion.product_selling_points", "configure_object", "产品卖点", "最多10个产品卖点，每个6-9个字，可空格分隔，回车(Enter)提交", true},
	"promotion.native_anchor_reference":  {"promotion.native_anchor_reference", "open_reference_picker", "原生锚点", "原生锚点", false},
	"promotion.landing_page_reference":   {"promotion.landing_page_reference", "open_reference_picker", "落地页", "请选择橙子落地页链接", true},
	"promotion.call_to_action":           {"promotion.call_to_action", "configure_object", "行动号召", "行动号召", true},
	"promotion.source_label":             {"promotion.source_label", "fill_text", "来源", "请输入来源", true},
	"promotion.comments_enabled":         {"promotion.comments_enabled", "choose_exact_visible_option", "单元评论", "不启用", true},
	"promotion.smart_generation_enabled": {"promotion.smart_generation_enabled", "toggle", "行动号召", "开启智能生成", false},
	"promotion.category":                 {"promotion.category", "choose_exact_visible_option", "所属类别", "请选择", true},
	"promotion.brand_reference":          {"promotion.brand_reference", "open_reference_picker", "品牌名称", "选择或手动输入品牌", true},
	"promotion.daily_budget":             {"promotion.daily_budget", "fill_money", "单元预算", "spinbutton", true},
	"promotion.bid":                      {"promotion.bid", "fill_money", "单元出价", "spinbutton", true},
	"promotion.roi_coefficient":          {"promotion.roi_coefficient", "fill_decimal", "ROI系数", "spinbutton", true},
	"promotion.pangle_bid_coefficient":   {"promotion.pangle_bid_coefficient", "fill_decimal", "穿山甲系数", "spinbutton", true},
	"promotion.promotion_name":           {"promotion.promotion_name", "fill_text", "单元名称", "请输入", true},
}

// CompileConfigurationV3 validates one immutable configuration and produces
// one Runner v3 form for each configured project and promotion.
func CompileConfigurationV3(configuration delivery.PlatformConfiguration, intent *delivery.DeliveryIntent, account string, bindings V3ObjectBindings, now time.Time) (V3ConfigurationPlanSet, error) {
	ocean := configuration.Payload.OceanEngine
	if configuration.Platform != delivery.DeliveryPlatformOceanEngine || ocean == nil || ocean.Project == nil {
		return V3ConfigurationPlanSet{}, fmt.Errorf("unsupported platform configuration")
	}
	project := *ocean.Project
	optimizationTarget, optimizationDisplayName := "", ""
	if project.OptimizationTargetReference != nil {
		optimizationTarget = referenceKey(project.OptimizationTargetReference)
		optimizationDisplayName = project.OptimizationTargetReference.DisplayNameSnapshot
	}
	project.BudgetAndBidding.ChargingMode = oceanengineconstraints.ResolveChargingModeForTarget(optimizationTarget, optimizationDisplayName, project.BudgetAndBidding.ChargingMode)
	if err := validateAccountPath(project, account); err != nil {
		return V3ConfigurationPlanSet{}, err
	}
	if err := validateConfigurationLimits(project, ocean.Promotions, intent, now); err != nil {
		return V3ConfigurationPlanSet{}, err
	}
	parent, err := parentContext(project)
	if err != nil {
		return V3ConfigurationPlanSet{}, err
	}
	projectValues, err := projectPlanValues(project, intent)
	if err != nil {
		return V3ConfigurationPlanSet{}, err
	}
	projectKind := "project_create"
	if bindings.ProjectPlatformID != "" {
		if !numericReference(bindings.ProjectPlatformID) {
			return V3ConfigurationPlanSet{}, fmt.Errorf("project platform binding is not numeric")
		}
		projectKind = "project_edit"
	}
	projectPlan := formPlan(projectKind, account, bindings.ProjectPlatformID, "", parent, orderedProjectFields(project, parent), projectValues)
	attachProjectBidConstraint(&projectPlan, project)
	projectRaw, _ := json.Marshal(projectPlan)
	forms := []V3PlannedForm{{InternalObjectKind: "project", InternalObjectID: project.ProjectDraftID, PlatformObjectID: bindings.ProjectPlatformID, Plan: projectRaw, Diff: planDiff(projectPlan)}}

	parentID := bindings.ProjectPlatformID
	if parentID == "" {
		parentID = "binding:" + project.ProjectDraftID
	}
	for _, promotion := range ocean.Promotions {
		values, valueErr := promotionPlanValues(promotion, project, intent)
		if valueErr != nil {
			return V3ConfigurationPlanSet{}, fmt.Errorf("promotion %s: %w", promotion.PromotionDraftID, valueErr)
		}
		objectID := bindings.PromotionPlatformIDs[promotion.PromotionDraftID]
		kind := "promotion_create"
		if objectID != "" {
			if !numericReference(objectID) {
				return V3ConfigurationPlanSet{}, fmt.Errorf("promotion %s platform binding is not numeric", promotion.PromotionDraftID)
			}
			kind = "promotion_edit"
		}
		plan := formPlan(kind, account, objectID, parentID, parent, orderedPromotionFields(project, promotion), values)
		raw, _ := json.Marshal(plan)
		depends := ""
		if bindings.ProjectPlatformID == "" {
			depends = project.ProjectDraftID
		}
		forms = append(forms, V3PlannedForm{InternalObjectKind: "promotion", InternalObjectID: promotion.PromotionDraftID, PlatformObjectID: objectID, DependsOn: depends, Plan: raw, Diff: planDiff(plan)})
	}
	return V3ConfigurationPlanSet{SchemaVersion: configurationPlanSetV1, ConfigurationID: configuration.ConfigurationID, ConfigurationHash: configuration.CanonicalHash, AccountReference: account, Forms: forms}, nil
}

func validateAccountPath(project delivery.OceanEngineProjectDraft, account string) error {
	if !numericReference(account) || project.AccountReference.State != delivery.ReferenceResolved || project.AccountReference.ID != account {
		return fmt.Errorf("unsupported account path: exact numeric OceanEngine account binding is required")
	}
	if !slices.Contains([]string{"ecommerce", "lead_generation", "application", "product_catalog", "content_marketing"}, project.MarketingPurpose) {
		return fmt.Errorf("unsupported account path: marketing purpose %s is not calibrated", project.MarketingPurpose)
	}
	if project.MarketingPurpose != "product_catalog" && !slices.Contains([]string{"short_video_image_text", "manual_delivery"}, project.MarketingScenario) {
		return fmt.Errorf("unsupported account path: marketing scenario %s is not calibrated", project.MarketingScenario)
	}
	return nil
}

func validateConfigurationLimits(project delivery.OceanEngineProjectDraft, promotions []delivery.OceanEnginePromotionDraft, intent *delivery.DeliveryIntent, now time.Time) error {
	budget := project.BudgetAndBidding
	if project.MarketingPurpose == "lead_generation" && budget.BudgetMode == delivery.OceanEngineBudgetModeUnlimited {
		return fmt.Errorf("sales-lead projects require a daily budget of at least CNY 300")
	}
	if budget.Currency != "CNY" || (budget.BudgetMode != delivery.OceanEngineBudgetModeUnlimited && budget.DailyBudgetMinor < 30000) {
		return fmt.Errorf("project daily budget must be unlimited or at least CNY 300")
	}
	projectBudget := budget
	if !projectBidRequired(project) {
		projectBudget.BidMinor = nil
	}
	if err := validateProjectBid(projectBudget); err != nil {
		return fmt.Errorf("project: %w", err)
	}
	if project.Schedule.Timezone != "Asia/Shanghai" || !project.Schedule.EndAt.After(project.Schedule.StartAt) {
		return fmt.Errorf("project schedule must use Asia/Shanghai and have an ordered range")
	}
	shanghai := time.FixedZone("Asia/Shanghai", 8*60*60)
	tomorrow := time.Date(now.In(shanghai).Year(), now.In(shanghai).Month(), now.In(shanghai).Day()+1, 0, 0, 0, 0, shanghai)
	projectStart := project.Schedule.StartAt.In(shanghai)
	if projectStart.Before(tomorrow) {
		return fmt.Errorf("project start date %s must be no earlier than %s", projectStart.Format(time.DateOnly), tomorrow.Format(time.DateOnly))
	}
	if intent != nil {
		boundary := intent.Payload.BudgetBoundary
		if boundary.Currency != "CNY" {
			return fmt.Errorf("intent budget currency is not CNY")
		}
		if budget.BudgetMode != delivery.OceanEngineBudgetModeUnlimited {
			if boundary.MinimumDailyMinor != nil && budget.DailyBudgetMinor < *boundary.MinimumDailyMinor {
				return fmt.Errorf("project daily budget is below intent limit")
			}
			if boundary.MaximumDailyMinor != nil && budget.DailyBudgetMinor > *boundary.MaximumDailyMinor {
				return fmt.Errorf("project daily budget exceeds intent limit")
			}
		}
		schedule := intent.Payload.ScheduleBoundary
		if project.Schedule.StartAt.Before(schedule.EarliestStart) || project.Schedule.EndAt.After(schedule.LatestEnd) {
			return fmt.Errorf("project schedule is outside intent limits")
		}
	}
	for _, promotion := range promotions {
		if err := validatePromotionLandingPageCarrier(project.Carrier, promotion.LandingPageReference); err != nil {
			return fmt.Errorf("promotion %s: %w", promotion.PromotionDraftID, err)
		}
		if len(promotion.CopyItems) == 0 || strings.TrimSpace(promotion.PromotionName) == "" || strings.TrimSpace(promotion.Settings.SourceLabel) == "" {
			return fmt.Errorf("promotion %s requires copy, source, and name", promotion.PromotionDraftID)
		}
		if len(promotion.Settings.CallToAction) < 1 || len(promotion.Settings.CallToAction) > 10 || duplicateOrBlank(promotion.Settings.CallToAction) {
			return fmt.Errorf("promotion %s call to action needs 1 to 10 unique values", promotion.PromotionDraftID)
		}
		if promotion.BudgetAndBidding == nil {
			continue
		}
		value := *promotion.BudgetAndBidding
		if value.Currency != "CNY" || value.DailyBudgetMinor < 30000 {
			return fmt.Errorf("promotion %s daily budget must be at least CNY 300", promotion.PromotionDraftID)
		}
		if err := validateBid(value); err != nil {
			return fmt.Errorf("promotion %s: %w", promotion.PromotionDraftID, err)
		}
	}
	return nil
}

func validatePromotionLandingPageCarrier(carrier string, reference *delivery.StableReference) error {
	if reference == nil {
		return nil
	}
	switch carrier {
	case "orange_landing_page", "orange_landing_page_and_im":
		if reference.ObjectKind == "owned_landing_page" {
			return fmt.Errorf("Orange landing-page carrier cannot use an owned landing page")
		}
	case "owned_landing_page":
		if reference.ObjectKind == "orange_landing_page" {
			return fmt.Errorf("owned landing-page carrier cannot use an Orange landing page")
		}
	default:
		return fmt.Errorf("carrier %s does not use a promotion landing page", carrier)
	}
	return nil
}

func duplicateOrBlank(values []string) bool {
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return true
		}
		if _, ok := seen[value]; ok {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func validateBid(value delivery.OceanEngineBudgetAndBidding) error {
	if value.BidMinor != nil {
		if *value.BidMinor < 1 || (value.DailyBudgetMinor > 0 && *value.BidMinor > value.DailyBudgetMinor) {
			return fmt.Errorf("bid is outside the calibrated limit")
		}
	}
	if value.ROICoefficient != nil && *value.ROICoefficient <= 0 {
		return fmt.Errorf("ROI coefficient must be positive")
	}
	return nil
}

func validateProjectBid(value delivery.OceanEngineBudgetAndBidding) error {
	if value.BidMinor == nil {
		return validateBid(value)
	}
	constraint, err := oceanengineconstraints.Resolve(value.ChargingMode, value.DailyBudgetMinor)
	if err != nil {
		return err
	}
	if err := oceanengineconstraints.ValidateBid(*value.BidMinor, constraint); err != nil {
		return err
	}
	if value.ROICoefficient != nil && *value.ROICoefficient <= 0 {
		return fmt.Errorf("ROI coefficient must be positive")
	}
	return nil
}

func attachProjectBidConstraint(plan *v3Plan, project delivery.OceanEngineProjectDraft) {
	constraint, err := oceanengineconstraints.Resolve(project.BudgetAndBidding.ChargingMode, project.BudgetAndBidding.DailyBudgetMinor)
	if err != nil {
		return
	}
	for index := range plan.Steps {
		if plan.Steps[index].FieldKey == "project.bid" {
			plan.Steps[index].MoneyConstraint = &constraint
			return
		}
	}
}

func projectPlanValues(project delivery.OceanEngineProjectDraft, intent *delivery.DeliveryIntent) (map[string]any, error) {
	shanghai := time.FixedZone("Asia/Shanghai", 8*60*60)
	values := map[string]any{
		"project.marketing_purpose": marketingPurposeLabel(project.MarketingPurpose),
		"project.carrier":           carrierLabel(project.Carrier), "project.delivery_mode": deliveryModeLabel(project.DeliveryMode),
		"project.schedule":     map[string]string{"start": project.Schedule.StartAt.In(shanghai).Format(time.DateOnly), "end": project.Schedule.EndAt.In(shanghai).Format(time.DateOnly)},
		"project.budget_mode":  budgetModeLabel(project.BudgetAndBidding.BudgetMode),
		"project.project_name": project.ProjectName,
	}
	if project.MarketingPurpose != "product_catalog" {
		values["project.marketing_scenario"] = marketingScenarioLabel(project.MarketingScenario)
	}
	if project.BudgetAndBidding.BudgetMode != delivery.OceanEngineBudgetModeUnlimited {
		values["project.daily_budget"] = money(project.BudgetAndBidding.DailyBudgetMinor)
	}
	if slices.Contains([]string{"ecommerce", "lead_generation"}, project.MarketingPurpose) && project.MarketingProductReference != nil {
		spec, err := stableReferenceSpec(*project.MarketingProductReference, intentRefs(intent, "product"))
		if err != nil {
			return nil, fmt.Errorf("marketing product: %w", err)
		}
		values["project.marketing_product_reference"] = spec
	}
	if project.MarketingPurpose == "lead_generation" {
		leadCaptureMode := project.LeadCaptureMode
		if leadCaptureMode == "" {
			leadCaptureMode = "smart_lead"
			if project.Carrier == "owned_landing_page" || project.Carrier == "im" {
				leadCaptureMode = "custom_lead"
			}
		}
		values["project.lead_capture_mode"] = leadCaptureModeLabel(leadCaptureMode)
	}
	if project.MarketingPurpose == "application" {
		if project.ApplicationReference == nil {
			return nil, fmt.Errorf("application marketing requires an application reference")
		}
		spec, err := stableReferenceSpec(*project.ApplicationReference, nil)
		if err != nil {
			return nil, fmt.Errorf("application: %w", err)
		}
		values["project.application_reference"] = spec
		values["project.application_scenario"] = applicationScenarioLabel(project.ApplicationScenario)
		values["project.operating_system"] = operatingSystemLabel(project.OperatingSystem)
		if project.ApplicationScenario == "app_download" {
			values["project.application_download_mode"] = applicationDownloadModeLabel(project.ApplicationDownloadMode)
		}
		if project.ApplicationScenario == "app_launch" {
			values["project.application_launch_mode"] = applicationLaunchModeLabel(project.ApplicationLaunchMode)
		}
	}
	if project.MarketingPurpose == "product_catalog" {
		if project.ProductCatalogReference == nil {
			return nil, fmt.Errorf("product-catalog marketing requires a product catalog reference")
		}
		spec, err := stableReferenceSpec(*project.ProductCatalogReference, nil)
		if err != nil {
			return nil, fmt.Errorf("product catalog: %w", err)
		}
		values["project.product_catalog_reference"] = spec
		if project.ProductTargeting != nil {
			values["project.product_targeting"] = project.ProductTargeting
		}
	}
	optimization := referenceKey(project.OptimizationTargetReference)
	optimizationName := optimizationLabel(optimization)
	if optimizationName == "" && project.OptimizationTargetReference != nil {
		optimizationName = project.OptimizationTargetReference.DisplayNameSnapshot
	}
	values["project.optimization_target_reference"] = optimizationName
	deepOptimizationMode := strings.TrimSpace(project.DeepOptimizationMode)
	if deepOptimizationMode == "" {
		deepOptimizationMode = "disabled"
	}
	values["project.deep_optimization_mode"] = deepOptimizationLabel(deepOptimizationMode)
	if project.AIGCDynamicCreative != nil {
		values["project.aigc_dynamic_creative"] = *project.AIGCDynamicCreative
	}
	placementStrategy := strings.TrimSpace(project.PlacementStrategy)
	if placementStrategy == "" {
		placementStrategy = "automatic"
	}
	values["project.placement_strategy"] = placementLabel(placementStrategy)
	if len(project.PlacementMedia) > 0 {
		values["project.placement_media"] = placementMediaLabels(project.PlacementMedia)
	}
	if projectBidRequired(project) && project.BudgetAndBidding.BidMinor != nil {
		values["project.bid"] = money(*project.BudgetAndBidding.BidMinor)
	}
	if project.BudgetAndBidding.ROICoefficient != nil {
		values["project.roi_coefficient"] = decimal(*project.BudgetAndBidding.ROICoefficient)
	}
	if project.SearchBoost != nil {
		if project.SearchBoost.BidCoefficient != nil {
			values["project.search_bid_coefficient"] = decimal(*project.SearchBoost.BidCoefficient)
		}
		if project.SearchBoost.TargetingExpansion != nil {
			if *project.SearchBoost.TargetingExpansion {
				values["project.search_targeting_expansion"] = "启用"
			} else {
				values["project.search_targeting_expansion"] = "不启用"
			}
		}
	}
	return values, nil
}

func promotionPlanValues(p delivery.OceanEnginePromotionDraft, project delivery.OceanEngineProjectDraft, intent *delivery.DeliveryIntent) (map[string]any, error) {
	values := map[string]any{"promotion.copy_materials": copyTexts(p.CopyItems), "promotion.product_selling_points": p.ProductSellingPoints, "promotion.call_to_action": p.Settings.CallToAction, "promotion.source_label": p.Settings.SourceLabel, "promotion.promotion_name": p.PromotionName}
	if promotionDirectLinkApplicable(project) {
		directLinkMode := strings.TrimSpace(p.Settings.DirectLinkMode)
		if directLinkMode == "" {
			directLinkMode = "automatic"
		}
		switch directLinkMode {
		case "automatic":
			values["promotion.direct_link_mode"] = "自动生成"
		case "manual":
			values["promotion.direct_link_mode"] = "手动填写"
			if p.DirectLinkReference == nil {
				return nil, fmt.Errorf("manual direct link requires a link or an OceanEngine object")
			}
			if validManualDirectLink(p.DirectLinkReference.ID) {
				values["promotion.direct_link_reference"] = strings.TrimSpace(p.DirectLinkReference.ID)
			} else {
				spec, err := stableReferenceSpec(*p.DirectLinkReference, nil)
				if err != nil {
					return nil, fmt.Errorf("direct link: %w", err)
				}
				values["promotion.direct_link_reference"] = spec
			}
		default:
			return nil, fmt.Errorf("direct link mode must be automatic or manual")
		}
	}
	productName := strings.TrimSpace(p.ProductName)
	if productName == "" && project.MarketingProductReference != nil {
		productName = strings.TrimSpace(project.MarketingProductReference.DisplayNameSnapshot)
	}
	if productName != "" {
		values["promotion.product_name"] = productName
	}
	if p.DeliveryIdentity.Mode == "account_info" {
		values["promotion.delivery_identity"] = "账户信息"
	} else if p.DeliveryIdentity.AuthorizedIdentity != nil {
		spec, err := stableReferenceSpec(*p.DeliveryIdentity.AuthorizedIdentity, nil)
		if err != nil {
			return nil, fmt.Errorf("delivery identity: %w", err)
		}
		values["promotion.delivery_identity"] = spec
	} else {
		return nil, fmt.Errorf("delivery identity is unresolved")
	}
	materials := make([]any, 0, len(p.BaseMaterialReferences))
	for _, ref := range p.BaseMaterialReferences {
		spec, err := stableReferenceSpec(ref, intentRefs(intent, "material"))
		if err != nil {
			return nil, fmt.Errorf("base material: %w", err)
		}
		materials = append(materials, spec)
	}
	if len(materials) != 1 {
		return nil, fmt.Errorf("Runner v3 supports exactly one bound base material per form")
	}
	values["promotion.base_materials"] = materials[0]
	if p.NativeAnchorReference != nil {
		spec, err := stableReferenceSpec(*p.NativeAnchorReference, nil)
		if err != nil {
			return nil, fmt.Errorf("native anchor: %w", err)
		}
		values["promotion.native_anchor_reference"] = spec
	}
	if len(p.ProductImageReferences) > 0 {
		if len(p.ProductImageReferences) != 1 {
			return nil, fmt.Errorf("Runner v3 supports exactly one product image")
		}
		spec, err := stableReferenceSpec(p.ProductImageReferences[0], intentRefs(intent, "material"))
		if err != nil {
			return nil, fmt.Errorf("product image: %w", err)
		}
		if !validImageSourceIdentity(p.ProductImageReferences[0].AuditAttributes["image_src_identity"]) {
			return nil, fmt.Errorf("product image requires a stable image_src_identity")
		}
		spec["selection_kind"] = "image_card"
		values["promotion.product_image_references"] = spec
	}
	if p.LandingPageReference != nil {
		if project.Carrier == "owned_landing_page" && p.LandingPageReference.ObjectKind == "owned_landing_page" {
			if p.LandingPageReference.State != delivery.ReferenceResolved || strings.TrimSpace(p.LandingPageReference.ID) == "" {
				return nil, fmt.Errorf("landing page: owned landing-page reference is not resolved")
			}
			values["promotion.landing_page_reference"] = strings.TrimSpace(p.LandingPageReference.ID)
		} else {
			spec, err := stableReferenceSpec(*p.LandingPageReference, intentRefs(intent, "landing_page"))
			if err != nil {
				return nil, fmt.Errorf("landing page: %w", err)
			}
			values["promotion.landing_page_reference"] = spec
		}
	}
	if p.Settings.CategoryReference != nil {
		label, err := resolvedLabel(*p.Settings.CategoryReference)
		if err != nil {
			return nil, fmt.Errorf("category: %w", err)
		}
		values["promotion.category"] = label
	}
	if p.Settings.BrandReference != nil {
		spec, err := stableReferenceSpec(*p.Settings.BrandReference, nil)
		if err != nil {
			return nil, fmt.Errorf("brand: %w", err)
		}
		spec["selection_kind"] = "text_option"
		values["promotion.brand_reference"] = spec
	}
	if p.Settings.CommentsEnabled != nil {
		if *p.Settings.CommentsEnabled {
			values["promotion.comments_enabled"] = "启用"
		} else {
			values["promotion.comments_enabled"] = "不启用"
		}
	}
	if p.Settings.SmartGenerationEnabled != nil {
		values["promotion.smart_generation_enabled"] = *p.Settings.SmartGenerationEnabled
	}
	if p.BudgetAndBidding != nil {
		values["promotion.daily_budget"] = money(p.BudgetAndBidding.DailyBudgetMinor)
		if p.BudgetAndBidding.BidMinor != nil {
			values["promotion.bid"] = money(*p.BudgetAndBidding.BidMinor)
		}
		if p.BudgetAndBidding.ROICoefficient != nil {
			values["promotion.roi_coefficient"] = decimal(*p.BudgetAndBidding.ROICoefficient)
		}
	}
	_ = project
	return values, nil
}

func stableReferenceSpec(ref delivery.StableReference, allowed []delivery.StableReference) (map[string]any, error) {
	if ref.State != delivery.ReferenceResolved || strings.TrimSpace(ref.ID) == "" {
		return nil, fmt.Errorf("reference %s is not resolved", ref.ObjectKind)
	}
	if allowed != nil && !slices.ContainsFunc(allowed, func(candidate delivery.StableReference) bool {
		return candidate.State == delivery.ReferenceResolved && candidate.Namespace == ref.Namespace && candidate.ObjectKind == ref.ObjectKind && candidate.ID == ref.ID
	}) {
		return nil, fmt.Errorf("reference %s is outside the delivery intent", ref.ID)
	}
	platformID := platformReferenceID(ref)
	if platformID == "" {
		return nil, fmt.Errorf("reference %s has no OceanEngine platform ID", ref.ID)
	}
	value := map[string]any{"selection_kind": "async_row", "object_id": platformID, "confirm_button": "确定"}
	if ref.DisplayNameSnapshot != "" {
		value["label"] = ref.DisplayNameSnapshot
	}
	for _, key := range []string{"selection_kind", "confirm_button", "image_src_identity"} {
		if ref.AuditAttributes[key] != "" {
			value[key] = ref.AuditAttributes[key]
		}
	}
	for _, key := range []string{"expected_total", "index", "minimum_visible"} {
		if raw := ref.AuditAttributes[key]; raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil {
				return nil, fmt.Errorf("reference %s has invalid %s", ref.ID, key)
			}
			value[key] = parsed
		}
	}
	return value, nil
}

func validImageSourceIdentity(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && strings.Contains(value, "/") && !strings.Contains(value, "://") && !strings.ContainsAny(value, "?#") && !strings.HasPrefix(value, "api/")
}

func resolvedLabel(ref delivery.StableReference) (string, error) {
	if ref.State != delivery.ReferenceResolved {
		return "", fmt.Errorf("reference is not resolved")
	}
	if ref.DisplayNameSnapshot != "" {
		return ref.DisplayNameSnapshot, nil
	}
	if ref.SemanticKey != "" {
		return ref.SemanticKey, nil
	}
	if ref.ID != "" {
		return ref.ID, nil
	}
	return "", fmt.Errorf("reference has no label")
}
func intentRefs(intent *delivery.DeliveryIntent, kind string) []delivery.StableReference {
	if intent == nil {
		return nil
	}
	var values []delivery.StableReference
	switch kind {
	case "product":
		values = intent.Payload.ProductReferences
	case "landing_page":
		values = intent.Payload.LandingPageReferences
	case "material":
		values = intent.Payload.MaterialReferences
	}
	if values == nil {
		return []delivery.StableReference{}
	}
	return values
}
func referenceKey(ref *delivery.StableReference) string {
	if ref == nil {
		return ""
	}
	if ref.SemanticKey != "" {
		return ref.SemanticKey
	}
	return ref.ID
}
func copyTexts(items []delivery.OceanEngineCopyItem) []string {
	out := make([]string, len(items))
	for i := range items {
		out[i] = items[i].Text
	}
	return out
}
func money(minor int64) string     { return strconv.FormatFloat(float64(minor)/100, 'f', 2, 64) }
func decimal(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) }
func carrierLabel(value string) string {
	return map[string]string{"orange_landing_page": "橙子落地页", "owned_landing_page": "自研落地页", "byte_miniapp": "字节小程序", "wechat_miniapp": "微信小程序"}[value]
}
func marketingPurposeLabel(value string) string {
	return map[string]string{"ecommerce": "电商", "lead_generation": "销售线索", "application": "应用", "product_catalog": "商品", "content_marketing": "内容营销"}[value]
}
func marketingScenarioLabel(value string) string {
	return map[string]string{"short_video_image_text": "短视频+图文", "manual_delivery": "短视频+图文"}[value]
}
func leadCaptureModeLabel(value string) string {
	return map[string]string{"smart_lead": "智能优选", "custom_lead": "自定义"}[value]
}
func applicationScenarioLabel(value string) string {
	return map[string]string{"app_download": "应用下载", "app_launch": "应用调起", "app_appointment_download": "预约下载"}[value]
}
func operatingSystemLabel(value string) string {
	return map[string]string{"android": "安卓", "ios": "iOS", "harmony": "鸿蒙", "harmonyos": "鸿蒙"}[value]
}
func applicationDownloadModeLabel(value string) string {
	return map[string]string{"direct_download": "直接下载", "landing_page_download": "落地页下载"}[value]
}
func applicationLaunchModeLabel(value string) string {
	return map[string]string{"direct_launch": "直接调起", "landing_page_launch": "落地页调起"}[value]
}
func projectBidRequired(project delivery.OceanEngineProjectDraft) bool {
	return slices.Contains([]string{"stable_cost", "cost_cap"}, project.BudgetAndBidding.BiddingStrategy) && !slices.Contains([]string{"conversion_roi", "net_roi"}, project.DeepOptimizationMode)
}
func budgetModeLabel(value string) string {
	if value == delivery.OceanEngineBudgetModeUnlimited {
		return "不限"
	}
	return "设置预算"
}
func optimizationLabel(value string) string {
	value = normalizedOptimizationTarget(value)
	return map[string]string{"button_jump": "按钮跳转", "in_app_order": "app内下单", "click": "点击量", "impression": "展示量", "store_call": "门店电话拨打", "store_stay": "门店停留"}[value]
}
func deepOptimizationLabel(value string) string {
	return map[string]string{"disabled": "不启用", "conversion_roi": "成交ROI", "net_order": "净成交下单", "net_roi": "净成交ROI"}[value]
}
func deliveryModeLabel(value string) string {
	if value == "automatic" || value == "ubmax" {
		return "自动投放(UBMax)"
	}
	return "手动投放"
}
func placementLabel(value string) string {
	if value == "preferred_media" {
		return "首选媒体"
	}
	return "通投智选"
}

func orderedProjectFields(project delivery.OceanEngineProjectDraft, parent v3ParentContext) []fieldSpec {
	keys := []string{"project.marketing_purpose"}
	if project.MarketingPurpose != "product_catalog" {
		keys = append(keys, "project.marketing_scenario")
	}
	switch project.MarketingPurpose {
	case "ecommerce":
		keys = append(keys, "project.marketing_product_reference")
	case "lead_generation":
		keys = append(keys, "project.marketing_product_reference", "project.lead_capture_mode")
	case "application":
		keys = append(keys, "project.application_reference", "project.application_scenario", "project.operating_system")
		if project.ApplicationScenario == "app_download" {
			keys = append(keys, "project.application_download_mode")
		}
		if project.ApplicationScenario == "app_launch" {
			keys = append(keys, "project.application_launch_mode")
		}
	case "product_catalog":
		keys = append(keys, "project.product_catalog_reference", "project.product_targeting")
	}
	if slices.Contains([]string{"ecommerce", "lead_generation", "content_marketing"}, project.MarketingPurpose) {
		keys = append(keys, "project.carrier")
	}
	keys = append(keys, "project.optimization_target_reference", "project.deep_optimization_mode", "project.delivery_mode", "project.schedule", "project.budget_mode", "project.daily_budget", "project.project_name")
	if project.MarketingPurpose == "lead_generation" {
		keys = removeKey(keys, "project.delivery_mode")
	}
	if parent.DeliveryMode == "ubmax" {
		if slices.Contains([]string{"ecommerce", "lead_generation"}, project.MarketingPurpose) {
			keys = append(keys, "project.aigc_dynamic_creative")
		}
	} else if project.MarketingPurpose == "ecommerce" {
		keys = append(keys, "project.placement_strategy", "project.search_bid_coefficient", "project.search_targeting_expansion")
	} else if project.MarketingPurpose == "product_catalog" {
		keys = append(keys, "project.placement_strategy")
	}
	if slices.Contains([]string{"ecommerce", "product_catalog"}, project.MarketingPurpose) && parent.PlacementMode == "preferred_media" {
		keys = append(keys, "project.placement_media")
	}
	if project.BudgetAndBidding.BudgetMode == delivery.OceanEngineBudgetModeUnlimited {
		keys = removeKey(keys, "project.daily_budget")
	}
	if project.MarketingPurpose == "lead_generation" {
		// The current sales-lead form has one required numeric daily-budget
		// input. It has no ecommerce-style budget-mode control.
		keys = removeKey(keys, "project.budget_mode")
	}
	if projectBidRequired(project) {
		keys = append(keys, "project.bid")
	}
	if parent.DeepOptimization == "conversion_roi" {
		keys = removeKey(keys, "project.bid")
	}
	if parent.DeepOptimization == "net_roi" {
		keys = removeKey(keys, "project.bid")
		keys = append(keys, "project.roi_coefficient")
	}
	if !slices.Contains([]string{"in_app_order"}, parent.OptimizationTarget) {
		keys = removeKey(keys, "project.deep_optimization_mode")
	}
	return specs(keys, projectSpecs)
}
func orderedPromotionFields(project delivery.OceanEngineProjectDraft, promotion delivery.OceanEnginePromotionDraft) []fieldSpec {
	parent, _ := parentContext(project)
	keys := []string{"promotion.delivery_identity", "promotion.base_materials", "promotion.copy_materials", "promotion.native_anchor_reference", "promotion.landing_page_reference", "promotion.direct_link_mode", "promotion.direct_link_reference", "promotion.product_name", "promotion.product_image_references", "promotion.product_selling_points", "promotion.call_to_action", "promotion.smart_generation_enabled", "promotion.source_label", "promotion.comments_enabled", "promotion.category", "promotion.brand_reference", "promotion.promotion_name"}
	if slices.Contains([]string{"click", "impression"}, parent.OptimizationTarget) {
		keys = removeKey(keys, "promotion.native_anchor_reference")
	}
	if !promotionDirectLinkApplicable(project) {
		keys = removeKey(keys, "promotion.direct_link_mode")
		keys = removeKey(keys, "promotion.direct_link_reference")
	}
	if !slices.Contains([]string{"orange_landing_page", "orange_landing_page_and_im", "owned_landing_page"}, project.Carrier) {
		keys = removeKey(keys, "promotion.landing_page_reference")
	}
	if parent.DeliveryMode == "manual" {
		keys = append(keys, "promotion.daily_budget", "promotion.bid")
	}
	if parent.DeepOptimization == "conversion_roi" {
		keys = removeKey(keys, "promotion.bid")
		keys = append(keys, "promotion.roi_coefficient")
	}
	if parent.PlacementMode == "preferred_media" && placementMediaSelected(project.PlacementMedia, "pangolin", "穿山甲") {
		keys = append(keys, "promotion.pangle_bid_coefficient")
	}
	fields := specs(keys, promotionSpecs)
	if promotion.Settings.DirectLinkMode != "manual" {
		fields = slices.DeleteFunc(fields, func(field fieldSpec) bool { return field.Key == "promotion.direct_link_reference" })
	} else if promotion.DirectLinkReference != nil && validManualDirectLink(promotion.DirectLinkReference.ID) {
		for index := range fields {
			if fields[index].Key == "promotion.direct_link_reference" {
				fields[index].Operation = "fill_text"
			}
		}
	}
	if project.Carrier == "owned_landing_page" {
		for index := range fields {
			if fields[index].Key != "promotion.landing_page_reference" {
				continue
			}
			fields[index].Target = "请选择或填写自研落地页链接"
			if promotion.LandingPageReference != nil && promotion.LandingPageReference.ObjectKind == "owned_landing_page" {
				fields[index].Operation = "fill_text"
			}
		}
	}
	return fields
}

func promotionDirectLinkApplicable(project delivery.OceanEngineProjectDraft) bool {
	parent, _ := parentContext(project)
	return parent.OptimizationTarget != "impression"
}

func placementMediaLabels(values []string) []string {
	labels := map[string]string{
		"toutiao":  "今日头条",
		"xigua":    "西瓜视频",
		"douyin":   "抖音",
		"fanqie":   "番茄系媒体",
		"pangolin": "穿山甲",
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if label := labels[value]; label != "" {
			result = append(result, label)
			continue
		}
		result = append(result, value)
	}
	return result
}

func placementMediaSelected(values []string, keys ...string) bool {
	return slices.ContainsFunc(values, func(value string) bool {
		return slices.Contains(keys, strings.TrimSpace(value))
	})
}
func specs(keys []string, source map[string]fieldSpec) []fieldSpec {
	out := make([]fieldSpec, 0, len(keys))
	for _, key := range keys {
		if value, ok := source[key]; ok {
			out = append(out, value)
		}
	}
	return out
}
func removeKey(values []string, key string) []string {
	return slices.DeleteFunc(values, func(value string) bool { return value == key })
}

func formPlan(kind, account, object, parentProject string, parent v3ParentContext, fields []fieldSpec, values map[string]any) v3Plan {
	plan := v3Plan{SchemaVersion: rparunner.PlanSchemaV3, PlanKind: kind, Browser: "msedge", Mode: "prepare", Status: "ready", AccountReference: account, ObjectReference: object, ParentProjectReference: parentProject, ParentConditionManifestID: v3ParentManifestID, ParentContext: parent, BlockedReasons: []string{}, AllowRemoteWrite: false, MaximumFinalClicks: 0}
	plan.Steps = append(plan.Steps, v3Step{ID: "001-identify-page", Kind: "identify_page", PageKind: kind})
	for _, field := range fields {
		value, ok := values[field.Key]
		stepRequired := field.Required
		state := "missing"
		if ok {
			state = "provided"
		}
		plan.Steps = append(plan.Steps, v3Step{ID: fmt.Sprintf("%03d-%s", len(plan.Steps)+1, field.Key), Kind: "field_action", PageKind: kind, FieldKey: field.Key, Operation: field.Operation, Scope: field.Scope, Target: field.Target, Value: value, ValueState: state, Required: &stepRequired})
		if field.Required && !ok {
			plan.Status = "blocked"
			plan.BlockedReasons = append(plan.BlockedReasons, "missing_required_value:"+field.Key)
		}
	}
	plan.Steps = append(plan.Steps, v3Step{ID: fmt.Sprintf("%03d-readback", len(plan.Steps)+1), Kind: "readback", PageKind: kind})
	target := "保存并关闭"
	if strings.HasPrefix(kind, "project_") {
		target = "保存并新建单元"
	}
	plan.Steps = append(plan.Steps, v3Step{ID: fmt.Sprintf("%03d-final-click-boundary", len(plan.Steps)+1), Kind: "final_click_boundary", PageKind: kind, Target: target, RemoteWrite: true, Blocked: true, BlockReason: "PREPARE_PLAN_REMOTE_WRITE_PROHIBITED"})
	return plan
}
func planDiff(plan v3Plan) []V3FieldDifference {
	out := []V3FieldDifference{}
	for _, step := range plan.Steps {
		if step.Kind == "field_action" && step.ValueState == "provided" {
			out = append(out, V3FieldDifference{FieldKey: step.FieldKey, Operation: step.Operation, Target: step.Value})
		}
	}
	return out
}
