package connector

import "testing"

func TestParseAccountCapabilitiesExtractsExecutableFactsWithoutEvaluatingRules(t *testing.T) {
	payload := map[string]any{"data": map[string]any{
		"external_action": map[string]any{
			"form":     map[string]any{"text": "表单提交", "value": 2.0, "default": true},
			"multiple": map[string]any{"text": "多转化", "value": 100.0},
		},
		"deep_external_action": map[string]any{
			"customer_effective": map[string]any{"text": "有效获客", "value": 26.0},
		},
		"creative_component_v2": map[string]any{"componentTypes": map[string]any{
			"[3]": map[string]any{"rule": "!GLOBAL_VAR.access_list.promote_blacklist && GLOBAL_VAR.access_list.form_collection_v2", "next": map[string]any{"rule": "projectInfo.marketing_info.landing_type === 1", "next": map[string]any{"rule": "imageMode.includes(5)"}}},
		}},
		"new_custom_budget":     map[string]any{"2": map[string]any{"first": map[string]any{"others": 100.0}}},
		"bid_constraint_config": map[string]any{"8": map[string]any{"39": 0.05}},
		"superior_front_config": map[string]any{
			"quota": map[string]any{"oc_video_upload_bind_batch_limit": 120.0},
			"rule":  map[string]any{"material_api_migration": map[string]any{"enabled": true}},
		},
		"orange_site_domains": []any{"www.chengzijianzhan.com"},
		"p0_interface_info":   []any{map[string]any{"url": "/superior/api/v2/project/get_optimization_goal_v2", "description": "优化目标列表"}},
		"core_interface":      []any{map[string]any{"api": "/superior/api/v2/project/get_optimization_goal_v2", "method": "POST", "checks": []any{map[string]any{"event": "goals_empty"}}}},
	}}

	snapshot := parseAccountCapabilities(payload)
	if len(snapshot.ExternalActions) != 2 || snapshot.ExternalActions[0].Value != "100" || snapshot.ExternalActions[1].Value != "2" {
		t.Fatalf("external actions=%#v", snapshot.ExternalActions)
	}
	if len(snapshot.CreativeComponents) != 1 || snapshot.CreativeComponents[0].ComponentTypeIDs[0] != "3" || len(snapshot.CreativeComponents[0].AccessFlags) != 2 || snapshot.CreativeComponents[0].LandingTypes[0] != "1" || snapshot.CreativeComponents[0].ImageModes[0] != "5" {
		t.Fatalf("creative components=%#v", snapshot.CreativeComponents)
	}
	if len(snapshot.Interfaces) != 1 || snapshot.Interfaces[0].Method != "POST" || snapshot.Interfaces[0].EmptyEvents[0] != "goals_empty" {
		t.Fatalf("interfaces=%#v", snapshot.Interfaces)
	}
	if snapshot.Quotas["oc_video_upload_bind_batch_limit"] != 120.0 || snapshot.BudgetRules["2"] == nil {
		t.Fatalf("rules=%#v quotas=%#v", snapshot.BudgetRules, snapshot.Quotas)
	}
}
