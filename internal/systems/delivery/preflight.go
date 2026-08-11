package delivery

import "strings"

// RunPreflight is the single authoritative rule engine used by both the Plan
// preview endpoint and the ChangeSet execution gate.
func RunPreflight(version DeliveryPlanVersion) []PreflightCheck {
	if version.IsPlatformConfigurationV2() {
		return runPlatformConfigurationPreflight(version)
	}
	trackingPresent := strings.TrimSpace(version.Tracking.LandingPage) != "" &&
		strings.TrimSpace(version.Tracking.PixelID) != "" &&
		strings.TrimSpace(version.Tracking.ConversionEvent) != ""
	advertiserPresent := strings.TrimSpace(version.Advertiser.ID) != "" &&
		strings.TrimSpace(version.Advertiser.Name) != ""
	creativePresent := len(version.CreativeReferences) > 0
	creativeConfirmed := creativePresent
	creativeResolvable := creativePresent
	for _, reference := range version.CreativeReferences {
		if !reference.Confirmed {
			creativeConfirmed = false
		}
		if strings.TrimSpace(reference.ContentHash) == "" || strings.TrimSpace(reference.Route) == "" {
			creativeResolvable = false
		}
	}
	strategyResolvable := strings.TrimSpace(version.StrategyReference.TaskID) != "" && version.StrategyReference.Version > 0 &&
		strings.TrimSpace(version.StrategyReference.ContentHash) != "" && strings.TrimSpace(version.StrategyReference.Route) != ""
	checks := []PreflightCheck{
		check(
			"advertiser_available",
			CheckSeverityError,
			advertiserPresent,
			"Mock 广告主已选择，可用于投前验证。",
			"缺少可用的 mock 广告主。",
			RepairTarget{Field: "advertiser_id", Section: "目标与账户", Label: "选择 mock 广告主"},
		),
		check(
			"budget_positive",
			CheckSeverityError,
			version.Budget.TotalMinor > 0,
			"预算大于 0，可以进入投前验证。",
			"总预算必须大于 0。",
			RepairTarget{Field: "budget_total", Section: "预算与排期", Label: "修复总预算"},
		),
		check(
			"schedule_valid",
			CheckSeverityError,
			!version.Schedule.StartAt.IsZero() && version.Schedule.EndAt.After(version.Schedule.StartAt),
			"排期起止时间有效。",
			"排期结束时间必须晚于开始时间。",
			RepairTarget{Field: "schedule_start", Section: "预算与排期", Label: "修复投放排期"},
		),
		check(
			"creative_present",
			CheckSeverityError,
			creativePresent,
			"已引用至少一个素材版本。",
			"至少需要一个素材版本引用。",
			RepairTarget{Field: "creative_asset_id", Section: "素材引用", Label: "添加素材引用"},
		),
		check(
			"creative_confirmed",
			CheckSeverityWarning,
			creativeConfirmed,
			"所有素材版本均已人工确认。",
			"存在未人工确认的素材版本；当前为警告，执行前仍需确认。",
			RepairTarget{Field: "creative_confirmed", Section: "素材引用", Label: "确认素材版本"},
		),
		check(
			"tracking_complete",
			CheckSeverityError,
			trackingPresent,
			"落地页、像素和转化事件追踪完整。",
			"追踪缺失：请补齐落地页、像素和转化事件。",
			RepairTarget{Field: "tracking_pixel_id", Section: "追踪", Label: "修复追踪配置"},
		),
	}
	// Structured references are additive for legacy plans. Once a plan adopts
	// them, both strategy and creative references become a hard execution gate.
	if strings.TrimSpace(version.StrategyReference.TaskID) != "" {
		checks = append(checks, check(
			"upstream_references_resolved",
			CheckSeverityError,
			strategyResolvable && creativeResolvable,
			"策略任务与素材版本均已解析到不可变来源。",
			"策略任务或素材版本缺少可验证的内容哈希与返回入口。",
			RepairTarget{Field: "strategy_reference", Section: "素材引用", Label: "重新选择上游策略与已确认素材"},
		))
	}
	// Three-tier configuration is strictly additive: plans without a snapshot receive the
	// exact legacy check set and therefore retain old behavior and hashes.
	if version.ThreeTierConfiguration == nil {
		return checks
	}
	config := version.ThreeTierConfiguration
	structureValid := config.Validate() == nil
	missing, orphan, unconfirmed, platformPending := false, false, false, false
	known := map[string]bool{}
	fields := threeTierFields(config)
	for _, f := range fields {
		if f.Key == "" {
			missing = true
			continue
		}
		known[f.Key] = true
		if f.MockRequired && (f.Effective.Type == "" || f.Effective.Value == nil) {
			missing = true
		}
		if !f.Confirmed {
			unconfirmed = true
		}
		if f.PlatformRequired && f.PlatformStatus == "pending" {
			platformPending = true
		}
	}
	for _, f := range fields {
		dependencies := append([]string(nil), f.DependencyRefs...)
		if f.Dependency != "" {
			dependencies = append(dependencies, f.Dependency)
		}
		for _, dependency := range dependencies {
			if !known[strings.TrimPrefix(dependency, "field:")] {
				orphan = true
			}
		}
	}
	checks = append(checks,
		check("three_tier_structure", CheckSeverityError, structureValid, "Three-tier snapshot structure is valid", "Three-tier snapshot structure is invalid", RepairTarget{Field: "three_tier_configuration", Section: "configuration", Label: "recompile the three-tier fixture"}),
		check("three_tier_required_fields", CheckSeverityError, !missing, "Three-tier fields complete", "Three-tier snapshot has a missing required field", RepairTarget{Field: "three_tier_configuration", Section: "configuration", Label: "compile a complete fixture"}),
		check("three_tier_dependencies", CheckSeverityError, !orphan, "Three-tier dependencies resolve", "Three-tier snapshot has an orphan dependency", RepairTarget{Field: "dependency", Section: "configuration", Label: "repair dependency"}),
		check("three_tier_confirmation", CheckSeverityError, !unconfirmed, "Three-tier fields confirmed", "Three-tier snapshot requires confirmation", RepairTarget{Field: "confirmed", Section: "configuration", Label: "confirm field"}),
		check("three_tier_platform_pending", CheckSeverityWarning, !platformPending, "No platform values pending", "Real-platform values remain pending", RepairTarget{Field: "platform_status", Section: "configuration", Label: "platform values are not executable"}),
	)
	return checks
}

func threeTierFields(config *ThreeTierConfiguration) []ThreeTierField {
	fields := make([]ThreeTierField, 0)
	for _, group := range config.Groups {
		fields = append(fields, group.Fields...)
		for _, plan := range group.Plans {
			fields = append(fields, plan.Fields...)
			for _, creative := range plan.Creatives {
				fields = append(fields, creative.Fields...)
			}
		}
	}
	return fields
}

func check(code string, severity CheckSeverity, passed bool, successMessage, failureMessage string, repair RepairTarget) PreflightCheck {
	if passed {
		return PreflightCheck{Code: code, Severity: severity, Passed: true, Message: successMessage}
	}
	return PreflightCheck{Code: code, Severity: severity, Passed: false, Message: failureMessage, Repair: &repair}
}

func scenarioFor(draft PlanDraft) Scenario {
	if draft.Budget.TotalMinor == 0 {
		return ScenarioBudgetZero
	}
	if strings.TrimSpace(draft.Tracking.LandingPage) == "" ||
		strings.TrimSpace(draft.Tracking.PixelID) == "" ||
		strings.TrimSpace(draft.Tracking.ConversionEvent) == "" {
		return ScenarioTrackingMissing
	}
	for _, reference := range draft.CreativeReferences {
		if !reference.Confirmed {
			return ScenarioCreativeUnconfirmed
		}
	}
	if strings.TrimSpace(draft.Advertiser.ID) == "" || len(draft.CreativeReferences) == 0 {
		return ScenarioIncompleteDraft
	}
	return ScenarioGoldenPath
}
