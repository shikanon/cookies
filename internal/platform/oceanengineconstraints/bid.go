package oceanengineconstraints

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed oceanengine-bid-constraints-v1.json
var bidConstraintJSON []byte

type pricingRule struct {
	MinimumMinor  int64  `json:"minimum_minor"`
	MaximumMinor  int64  `json:"maximum_minor"`
	MaximumSource string `json:"maximum_source"`
}

type bidConstraintCatalog struct {
	SchemaVersion                  string                 `json:"schema_version"`
	DefaultTargetChargingMode      string                 `json:"default_target_charging_mode"`
	TargetChargingModes            map[string]string      `json:"target_charging_modes"`
	TargetDisplayNameChargingModes map[string]string      `json:"target_display_name_charging_modes"`
	PricingRules                   map[string]pricingRule `json:"pricing_rules"`
}

type BidConstraint struct {
	SchemaVersion string `json:"schema_version"`
	ChargingMode  string `json:"charging_mode"`
	MinimumMinor  int64  `json:"minimum_minor"`
	MaximumMinor  int64  `json:"maximum_minor"`
	MaximumSource string `json:"maximum_source"`
}

var catalog = mustLoadBidConstraintCatalog()

func mustLoadBidConstraintCatalog() bidConstraintCatalog {
	var value bidConstraintCatalog
	if err := json.Unmarshal(bidConstraintJSON, &value); err != nil {
		panic(fmt.Sprintf("parse OceanEngine bid constraints: %v", err))
	}
	return value
}

func NormalizeChargingMode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func ResolveChargingMode(optimizationTarget, fallback string) string {
	return ResolveChargingModeForTarget(optimizationTarget, "", fallback)
}

func ResolveChargingModeForTarget(optimizationTarget, displayName, fallback string) string {
	target := strings.TrimSpace(optimizationTarget)
	if value := catalog.TargetChargingModes[target]; value != "" {
		return value
	}
	name := strings.TrimSpace(displayName)
	if value := catalog.TargetDisplayNameChargingModes[name]; value != "" {
		return value
	}
	if target != "" || name != "" {
		fallbackMode := NormalizeChargingMode(fallback)
		if fallbackMode == "OCPC" || fallbackMode == "OCPM" {
			return fallbackMode
		}
		return catalog.DefaultTargetChargingMode
	}
	return NormalizeChargingMode(fallback)
}

func Resolve(chargingMode string, dailyBudgetMinor int64) (BidConstraint, error) {
	mode := NormalizeChargingMode(chargingMode)
	rule, ok := catalog.PricingRules[mode]
	if !ok {
		return BidConstraint{}, fmt.Errorf("unsupported OceanEngine charging mode %q", chargingMode)
	}
	maximum := rule.MaximumMinor
	maximumSource := rule.MaximumSource
	if maximumSource == "daily_budget" && dailyBudgetMinor > 0 && dailyBudgetMinor < maximum {
		maximum = dailyBudgetMinor
	}
	return BidConstraint{
		SchemaVersion: catalog.SchemaVersion,
		ChargingMode:  mode,
		MinimumMinor:  rule.MinimumMinor,
		MaximumMinor:  maximum,
		MaximumSource: maximumSource,
	}, nil
}

func ValidateBid(bidMinor int64, constraint BidConstraint) error {
	if bidMinor < constraint.MinimumMinor || bidMinor > constraint.MaximumMinor {
		return fmt.Errorf(
			"bid is outside the calibrated limit for %s: expected CNY %.2f to %.2f",
			constraint.ChargingMode,
			float64(constraint.MinimumMinor)/100,
			float64(constraint.MaximumMinor)/100,
		)
	}
	return nil
}
