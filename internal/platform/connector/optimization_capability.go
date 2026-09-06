package connector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/integrations/oceanengine"
)

const OptimizationTargetCapabilitySchemaV1 = "oceanengine-optimization-target-capability/v1"

type OptimizationTargetCapabilityRequest struct {
	OrganizationID string
	ProjectID      string
	AccountRef     string
	Context        oceanengine.OptimizationTargetContext
}

type OptimizationTargetLimit struct {
	DeliveryModes   []int `json:"delivery_modes,omitempty"`
	AutoAdTypes     []int `json:"auto_ad_types,omitempty"`
	DeliveryPackage []int `json:"delivery_packages,omitempty"`
}

type OptimizationEventAssetCapability struct {
	AssetID   string `json:"asset_id"`
	AssetName string `json:"asset_name,omitempty"`
	Role      string `json:"role,omitempty"`
}

type OptimizationTargetCapability struct {
	ExternalAction        string                             `json:"external_action"`
	SemanticKey           string                             `json:"semantic_key"`
	DisplayName           string                             `json:"display_name"`
	OptimizationEventType string                             `json:"optimization_event_type,omitempty"`
	AssetTypes            []string                           `json:"asset_types,omitempty"`
	TrackTypes            []string                           `json:"track_types,omitempty"`
	IsGray                bool                               `json:"is_gray"`
	DeepGoalRequired      bool                               `json:"deep_goal_required"`
	NeedAssets            bool                               `json:"need_assets"`
	Limits                OptimizationTargetLimit            `json:"limits"`
	EventAssets           []OptimizationEventAssetCapability `json:"event_assets,omitempty"`
}

type OptimizationTargetCapabilitySnapshot struct {
	SchemaVersion string                                `json:"schema_version"`
	SnapshotID    string                                `json:"snapshot_id"`
	AccountID     string                                `json:"account_id"`
	Context       oceanengine.OptimizationTargetContext `json:"context"`
	ContextHash   string                                `json:"context_hash"`
	Options       []OptimizationTargetCapability        `json:"options"`
	AssetIDs      []string                              `json:"asset_ids,omitempty"`
	ShowOther     bool                                  `json:"show_other"`
	ObservedAt    time.Time                             `json:"observed_at"`
}

type optimizationTargetCapabilityReader interface {
	OptimizationTargetCapabilities(context.Context, oceanengine.OptimizationTargetContext) (map[string]any, error)
}

func (s Synchronizer) ReadOptimizationTargetCapabilities(ctx context.Context, request OptimizationTargetCapabilityRequest) (OptimizationTargetCapabilitySnapshot, error) {
	if s.Readers == nil || request.OrganizationID == "" || request.ProjectID == "" || request.AccountRef == "" {
		return OptimizationTargetCapabilitySnapshot{}, ErrInvalidFact
	}
	reader, closeReader, err := s.Readers.Open(ctx, SyncRequest{OrganizationID: request.OrganizationID, ProjectID: request.ProjectID, AccountRef: request.AccountRef})
	if err != nil {
		return OptimizationTargetCapabilitySnapshot{}, err
	}
	defer closeReader()
	capabilityReader, ok := reader.(optimizationTargetCapabilityReader)
	if !ok {
		return OptimizationTargetCapabilitySnapshot{}, errors.New("optimization target capability reader unavailable")
	}
	payload, err := capabilityReader.OptimizationTargetCapabilities(ctx, request.Context)
	if err != nil {
		return OptimizationTargetCapabilitySnapshot{}, err
	}
	contextHash := hashOptimizationCapabilityValue(request.Context)
	options, assetIDs, showOther := parseOptimizationTargetCapabilities(payload, request.Context.NeedAssets)
	observedAt := time.Now().UTC()
	if s.Now != nil {
		observedAt = s.Now().UTC()
	}
	snapshotHash := hashOptimizationCapabilityValue(struct {
		AccountID   string                         `json:"account_id"`
		ContextHash string                         `json:"context_hash"`
		Options     []OptimizationTargetCapability `json:"options"`
		AssetIDs    []string                       `json:"asset_ids"`
		ShowOther   bool                           `json:"show_other"`
	}{request.AccountRef, contextHash, options, assetIDs, showOther})
	return OptimizationTargetCapabilitySnapshot{
		SchemaVersion: OptimizationTargetCapabilitySchemaV1,
		SnapshotID:    "oecap_" + snapshotHash,
		AccountID:     request.AccountRef,
		Context:       request.Context,
		ContextHash:   contextHash,
		Options:       options,
		AssetIDs:      assetIDs,
		ShowOther:     showOther,
		ObservedAt:    observedAt,
	}, nil
}

func parseOptimizationTargetCapabilities(payload map[string]any, needAssets bool) ([]OptimizationTargetCapability, []string, bool) {
	data, _ := payload["data"].(map[string]any)
	goals := mapItems(data["goals"])
	options := make([]OptimizationTargetCapability, 0, len(goals))
	for _, goal := range goals {
		externalAction := firstString(goal, "external_action")
		displayName := firstString(goal, "optimization_name")
		if externalAction == "" || displayName == "" {
			continue
		}
		eventType := firstString(goal, "optimization_event_type")
		semanticKey := strings.TrimSpace(eventType)
		if semanticKey == "" {
			semanticKey = "external_action:" + externalAction
		}
		limit, _ := goal["limit"].(map[string]any)
		assets := make([]OptimizationEventAssetCapability, 0)
		for _, asset := range objectItems(goal["asset_info"]) {
			assetID := firstString(asset, "asset_id")
			if assetID == "" {
				continue
			}
			assets = append(assets, OptimizationEventAssetCapability{AssetID: assetID, AssetName: firstString(asset, "asset_name"), Role: firstString(asset, "role")})
		}
		options = append(options, OptimizationTargetCapability{
			ExternalAction: externalAction, SemanticKey: semanticKey, DisplayName: displayName,
			OptimizationEventType: eventType, AssetTypes: stringSlice(goal["asset_types"]), TrackTypes: stringSlice(goal["track_type"]),
			IsGray: boolValue(goal["is_gray"]), DeepGoalRequired: boolValue(goal["deep_goal_required"]), NeedAssets: needAssets || len(assets) > 0,
			Limits:      OptimizationTargetLimit{DeliveryModes: intSlice(limit["delivery_mode"]), AutoAdTypes: intSlice(limit["auto_ad_type"]), DeliveryPackage: intSlice(limit["delivery_package"])},
			EventAssets: assets,
		})
	}
	sort.Slice(options, func(i, j int) bool { return options[i].ExternalAction < options[j].ExternalAction })
	assetIDs := stringSlice(data["asset_ids"])
	sort.Strings(assetIDs)
	return options, assetIDs, boolValue(data["show_other"])
}

func stringSlice(value any) []string {
	items, _ := value.([]any)
	result := make([]string, 0, len(items))
	for _, item := range items {
		value := firstString(map[string]any{"value": item}, "value")
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func intSlice(value any) []int {
	items, _ := value.([]any)
	result := make([]int, 0, len(items))
	for _, item := range items {
		result = append(result, int(numberValue(item)))
	}
	return result
}

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
}

func hashOptimizationCapabilityValue(value any) string {
	payload, _ := json.Marshal(value)
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}
