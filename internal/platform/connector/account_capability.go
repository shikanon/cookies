package connector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
)

const OceanEngineAccountCapabilitySchemaV1 = "oceanengine-account-capability/v1"

type AccountCapabilityRequest struct {
	OrganizationID string
	ProjectID      string
	AccountRef     string
}

type AccountCapabilityValue struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
	Value       string `json:"value"`
	Step        string `json:"step,omitempty"`
	Default     bool   `json:"default,omitempty"`
}

type CreativeComponentEligibility struct {
	ComponentTypeIDs []string `json:"component_type_ids"`
	AccessFlags      []string `json:"access_flags,omitempty"`
	LandingTypes     []string `json:"landing_types,omitempty"`
	CampaignTypes    []string `json:"campaign_types,omitempty"`
	InventoryTypes   []string `json:"inventory_types,omitempty"`
	InventoryCatalog []string `json:"inventory_catalogs,omitempty"`
	ImageModes       []string `json:"image_modes,omitempty"`
	ContentTypes     []string `json:"content_types,omitempty"`
}

type AccountInterfaceCapability struct {
	Method      string   `json:"method,omitempty"`
	Path        string   `json:"path"`
	Description string   `json:"description,omitempty"`
	EmptyEvents []string `json:"empty_events,omitempty"`
}

type OceanEngineAccountCapabilitySnapshot struct {
	SchemaVersion       string                         `json:"schema_version"`
	SnapshotID          string                         `json:"snapshot_id"`
	AccountID           string                         `json:"account_id"`
	ExternalActions     []AccountCapabilityValue       `json:"external_actions"`
	DeepExternalActions []AccountCapabilityValue       `json:"deep_external_actions"`
	CreativeComponents  []CreativeComponentEligibility `json:"creative_components"`
	BudgetRules         map[string]any                 `json:"budget_rules,omitempty"`
	BidConstraints      map[string]any                 `json:"bid_constraints,omitempty"`
	Quotas              map[string]any                 `json:"quotas,omitempty"`
	FeatureRules        map[string]any                 `json:"feature_rules,omitempty"`
	OrangeSiteDomains   []string                       `json:"orange_site_domains,omitempty"`
	Interfaces          []AccountInterfaceCapability   `json:"interfaces,omitempty"`
	ObservedAt          time.Time                      `json:"observed_at"`
}

type accountCapabilityReader interface {
	AccountConfiguration(context.Context) (map[string]any, error)
}

func (s Synchronizer) ReadAccountCapabilities(ctx context.Context, request AccountCapabilityRequest) (OceanEngineAccountCapabilitySnapshot, error) {
	if s.Readers == nil || request.OrganizationID == "" || request.ProjectID == "" || request.AccountRef == "" {
		return OceanEngineAccountCapabilitySnapshot{}, ErrInvalidFact
	}
	reader, closeReader, err := s.Readers.Open(ctx, SyncRequest{OrganizationID: request.OrganizationID, ProjectID: request.ProjectID, AccountRef: request.AccountRef})
	if err != nil {
		return OceanEngineAccountCapabilitySnapshot{}, err
	}
	defer closeReader()
	capabilityReader, ok := reader.(accountCapabilityReader)
	if !ok {
		return OceanEngineAccountCapabilitySnapshot{}, errors.New("account capability reader unavailable")
	}
	payload, err := capabilityReader.AccountConfiguration(ctx)
	if err != nil {
		return OceanEngineAccountCapabilitySnapshot{}, err
	}
	snapshot := parseAccountCapabilities(payload)
	snapshot.SchemaVersion = OceanEngineAccountCapabilitySchemaV1
	snapshot.AccountID = request.AccountRef
	snapshot.ObservedAt = time.Now().UTC()
	if s.Now != nil {
		snapshot.ObservedAt = s.Now().UTC()
	}
	hashInput := snapshot
	hashInput.SnapshotID = ""
	hashInput.ObservedAt = time.Time{}
	encoded, _ := json.Marshal(hashInput)
	digest := sha256.Sum256(encoded)
	snapshot.SnapshotID = "oeaccountcap_" + hex.EncodeToString(digest[:])
	return snapshot, nil
}

func parseAccountCapabilities(payload map[string]any) OceanEngineAccountCapabilitySnapshot {
	data, _ := payload["data"].(map[string]any)
	front, _ := data["superior_front_config"].(map[string]any)
	creative, _ := data["creative_component_v2"].(map[string]any)
	componentTypes, _ := creative["componentTypes"].(map[string]any)
	return OceanEngineAccountCapabilitySnapshot{
		ExternalActions:     parseCapabilityValues(data["external_action"]),
		DeepExternalActions: parseCapabilityValues(data["deep_external_action"]),
		CreativeComponents:  parseCreativeComponentEligibility(componentTypes),
		BudgetRules:         mapValue(data["new_custom_budget"]),
		BidConstraints:      mapValue(data["bid_constraint_config"]),
		Quotas:              mapValue(front["quota"]),
		FeatureRules:        mapValue(front["rule"]),
		OrangeSiteDomains:   sortedStrings(data["orange_site_domains"]),
		Interfaces:          parseAccountInterfaces(data),
	}
}

func parseCapabilityValues(value any) []AccountCapabilityValue {
	source, _ := value.(map[string]any)
	result := make([]AccountCapabilityValue, 0, len(source))
	for key, raw := range source {
		item, _ := raw.(map[string]any)
		value := firstString(item, "value")
		name := firstString(item, "text")
		if value == "" || name == "" {
			continue
		}
		result = append(result, AccountCapabilityValue{Key: key, DisplayName: name, Value: value, Step: firstString(item, "step"), Default: boolValue(item["default"])})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Value == result[j].Value {
			return result[i].Key < result[j].Key
		}
		return result[i].Value < result[j].Value
	})
	return result
}

var componentRuleDimensions = []struct {
	pattern *regexp.Regexp
	assign  func(*CreativeComponentEligibility, []string)
}{
	{regexp.MustCompile(`access_list\.([A-Za-z0-9_]+)`), func(v *CreativeComponentEligibility, values []string) { v.AccessFlags = values }},
	{regexp.MustCompile(`landing_type\??\s*===\s*(\d+)`), func(v *CreativeComponentEligibility, values []string) { v.LandingTypes = values }},
	{regexp.MustCompile(`campaign_type\??\s*===\s*(\d+)`), func(v *CreativeComponentEligibility, values []string) { v.CampaignTypes = values }},
	{regexp.MustCompile(`inventory_types\??\.includes\((\d+)\)`), func(v *CreativeComponentEligibility, values []string) { v.InventoryTypes = values }},
	{regexp.MustCompile(`inventory_catalog\s*===\s*(\d+)`), func(v *CreativeComponentEligibility, values []string) { v.InventoryCatalog = values }},
	{regexp.MustCompile(`imageMode\.includes\((\d+)\)`), func(v *CreativeComponentEligibility, values []string) { v.ImageModes = values }},
	{regexp.MustCompile(`content_type\??\s*===\s*(\d+)`), func(v *CreativeComponentEligibility, values []string) { v.ContentTypes = values }},
}

func parseCreativeComponentEligibility(source map[string]any) []CreativeComponentEligibility {
	result := make([]CreativeComponentEligibility, 0, len(source))
	for rawIDs, rule := range source {
		encoded, _ := json.Marshal(rule)
		item := CreativeComponentEligibility{ComponentTypeIDs: splitComponentIDs(rawIDs)}
		for _, dimension := range componentRuleDimensions {
			matches := dimension.pattern.FindAllStringSubmatch(string(encoded), -1)
			values := make([]string, 0, len(matches))
			for _, match := range matches {
				if len(match) > 1 {
					values = append(values, match[1])
				}
			}
			dimension.assign(&item, uniqueSortedCapabilityStrings(values))
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.Join(result[i].ComponentTypeIDs, ",") < strings.Join(result[j].ComponentTypeIDs, ",")
	})
	return result
}

func splitComponentIDs(value string) []string {
	value = strings.Trim(value, "[] ")
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return uniqueSortedCapabilityStrings(parts)
}

func parseAccountInterfaces(data map[string]any) []AccountInterfaceCapability {
	values := map[string]AccountInterfaceCapability{}
	for _, raw := range mapItems(data["p0_interface_info"]) {
		path := strings.TrimSpace(firstString(raw, "url"))
		if path != "" {
			values[path] = AccountInterfaceCapability{Path: path, Description: firstString(raw, "description")}
		}
	}
	for _, raw := range mapItems(data["core_interface"]) {
		path := strings.TrimSpace(firstString(raw, "api"))
		if path == "" {
			continue
		}
		item := values[path]
		item.Path = path
		item.Method = firstString(raw, "method")
		for _, check := range mapItems(raw["checks"]) {
			item.EmptyEvents = append(item.EmptyEvents, firstString(check, "event"))
		}
		item.EmptyEvents = uniqueSortedCapabilityStrings(item.EmptyEvents)
		values[path] = item
	}
	result := make([]AccountInterfaceCapability, 0, len(values))
	for _, item := range values {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}

func mapValue(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func sortedStrings(value any) []string {
	items, _ := value.([]any)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			result = append(result, strings.TrimSpace(text))
		}
	}
	return uniqueSortedCapabilityStrings(result)
}

func uniqueSortedCapabilityStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
