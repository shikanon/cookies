package connector

import "testing"

func TestParseOptimizationTargetCapabilitiesPreservesDynamicOptions(t *testing.T) {
	payload := map[string]any{"data": map[string]any{
		"goals": []any{
			map[string]any{"optimization_name": "表单提交", "optimization_event_type": "form", "external_action": "2", "asset_types": []any{"2"}, "track_type": []any{"16", "17"}, "limit": map[string]any{"delivery_mode": []any{1.0, 3.0}}},
			map[string]any{"optimization_name": "私信留资", "optimization_event_type": "", "external_action": "192", "asset_types": []any{}},
		},
		"asset_ids": []any{}, "show_other": false,
	}}
	options, assetIDs, showOther := parseOptimizationTargetCapabilities(payload, false)
	if len(options) != 2 || len(assetIDs) != 0 || showOther {
		t.Fatalf("options=%#v assetIDs=%#v showOther=%v", options, assetIDs, showOther)
	}
	if options[0].ExternalAction != "192" || options[0].SemanticKey != "external_action:192" || options[0].DisplayName != "私信留资" {
		t.Fatalf("private message option=%#v", options[0])
	}
	if options[1].ExternalAction != "2" || options[1].SemanticKey != "form" || len(options[1].Limits.DeliveryModes) != 2 {
		t.Fatalf("form option=%#v", options[1])
	}
}
