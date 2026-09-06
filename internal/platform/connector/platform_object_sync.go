package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/integrations/oceanengine"
)

type platformObjectReader interface {
	ImageMaterialsPage(context.Context, oceanengine.AssetPageRequest) (map[string]any, error)
	ProductImagesPage(context.Context, oceanengine.AssetPageRequest) (map[string]any, error)
	VideoMaterialsPage(context.Context, oceanengine.AssetPageRequest) (map[string]any, error)
	AwemePhotoMaterialsPage(context.Context, oceanengine.AssetPageRequest) (map[string]any, error)
	MarketingProductsPage(context.Context, oceanengine.AssetPageRequest) (map[string]any, error)
	OrangeLandingPagesPage(context.Context, oceanengine.AssetPageRequest) (map[string]any, error)
	FilteredOrangeLandingPagesPage(context.Context, oceanengine.AssetPageRequest, oceanengine.OrangeLandingPageFilter) (map[string]any, error)
	OptimizationTargets(context.Context, int, bool) (map[string]any, error)
	BrandIndustries(context.Context) (map[string]any, error)
	Brands(context.Context) (map[string]any, error)
	AuthorizedIdentitiesPage(context.Context, oceanengine.AssetPageRequest) (map[string]any, error)
}

type platformObjectPage struct {
	Items      []map[string]any
	TotalPages int
}

type platformObjectReadError struct {
	stage string
	err   error
}

func (e platformObjectReadError) Error() string     { return "platform object read failed" }
func (e platformObjectReadError) Unwrap() error     { return e.err }
func (e platformObjectReadError) SyncStage() string { return e.stage }

func (s Synchronizer) syncPlatformObjectCatalog(ctx context.Context, request SyncRequest, runID string, reader oceanengine.Reader, limit, maxPages int) (map[PlatformObjectKind]PlatformObjectSyncStats, error) {
	if request.ProjectID == "" {
		return nil, nil
	}
	objectReader, ok := reader.(platformObjectReader)
	if !ok {
		return nil, fmt.Errorf("%w: Ocean Engine reader has no platform object catalog", ErrInvalidFact)
	}
	catalog, ok := s.Writer.(PlatformObjectCatalog)
	if !ok {
		return nil, fmt.Errorf("%w: Connector writer has no platform object catalog", ErrInvalidFact)
	}
	type source struct {
		kind     PlatformObjectKind
		endpoint string
		fetch    func(context.Context, oceanengine.AssetPageRequest) (map[string]any, error)
		parse    func(map[string]any) platformObjectPage
		convert  func(map[string]any) (PlatformObjectCandidate, bool)
	}
	multiLeadLandingActions, err := s.readMultiLeadOrangeLandingPageActions(ctx, request, runID, objectReader, limit, maxPages)
	if err != nil {
		return nil, err
	}
	landingCandidate := func(item map[string]any) (PlatformObjectCandidate, bool) {
		candidate, valid := orangeLandingCandidate(item)
		if !valid {
			return candidate, false
		}
		actions := multiLeadLandingActions[candidate.PlatformObjectID]
		candidate.Metadata["multi_lead_external_actions"] = actions
		candidate.Metadata["multi_conversion_eligible"] = containsString(actions, "100")
		return candidate, true
	}
	sources := []source{
		{PlatformObjectIndustryCategory, "industry_category_list", func(ctx context.Context, _ oceanengine.AssetPageRequest) (map[string]any, error) {
			return objectReader.BrandIndustries(ctx)
		}, industryCategoryPage, industryCategoryCandidate},
		{PlatformObjectBrand, "brand_list", func(ctx context.Context, _ oceanengine.AssetPageRequest) (map[string]any, error) {
			return objectReader.Brands(ctx)
		}, brandPage, brandCandidate},
		{PlatformObjectAuthorizedIdentity, "authorized_identity_list", objectReader.AuthorizedIdentitiesPage, authorizedIdentityPage, authorizedIdentityCandidate},
		// Qualify landing pages before large material catalogs. A later material
		// read failure must not leave stale landing-page eligibility in Cookies.
		{PlatformObjectOrangeLandingPage, "orange_landing_page_list", objectReader.OrangeLandingPagesPage, orangeLandingPage, landingCandidate},
		{PlatformObjectImageMaterial, "image_material_list", objectReader.ImageMaterialsPage, imageMaterialPage, imageMaterialCandidate},
		{PlatformObjectProductImage, "product_image_list", objectReader.ProductImagesPage, imageMaterialPage, productImageCandidate},
		{PlatformObjectVideoMaterial, "video_material_list", objectReader.VideoMaterialsPage, videoMaterialPage, videoMaterialCandidate},
		{PlatformObjectAwemePhotoMaterial, "aweme_photo_material_list", objectReader.AwemePhotoMaterialsPage, awemePhotoMaterialPage, awemePhotoMaterialCandidate},
		{PlatformObjectMarketingProduct, "marketing_product_list", objectReader.MarketingProductsPage, marketingProductPage, marketingProductCandidate},
	}
	result := make(map[PlatformObjectKind]PlatformObjectSyncStats, len(sources)+2)
	optimizationStats, err := s.syncOptimizationObjects(ctx, request, runID, objectReader, catalog)
	if err != nil {
		return result, err
	}
	for kind, stats := range optimizationStats {
		result[kind] = stats
	}
	for _, current := range sources {
		candidates := []PlatformObjectCandidate{}
		observedAt := time.Time{}
		for page := 1; page <= maxPages; page++ {
			cursor := fmt.Sprintf("platform_objects:%s:page:%d", current.kind, page)
			if err := s.Writer.UpdateSyncCursor(ctx, runID, cursor); err != nil {
				return result, err
			}
			payload, err := current.fetch(ctx, oceanengine.AssetPageRequest{Page: page, Limit: limit})
			if err != nil {
				stage := string(current.kind)
				var staged interface{ ReadStage() string }
				if errors.As(err, &staged) {
					stage += ":" + staged.ReadStage()
				}
				return result, platformObjectReadError{stage: stage, err: err}
			}
			_, collectedAt, err := s.storeRaw(ctx, request, runID, current.endpoint, map[string]any{"page": page, "limit": limit}, payload)
			if err != nil {
				return result, err
			}
			observedAt = collectedAt
			parsed := current.parse(payload)
			for _, item := range parsed.Items {
				if candidate, valid := current.convert(item); valid {
					candidates = append(candidates, candidate)
				}
			}
			complete := parsed.TotalPages > 0 && page >= parsed.TotalPages
			if parsed.TotalPages == 0 && len(parsed.Items) < limit {
				complete = true
			}
			if complete {
				break
			}
			if page == maxPages {
				return result, fmt.Errorf("%w: %s exceeds page limit", ErrInvalidFact, current.kind)
			}
		}
		if observedAt.IsZero() {
			return result, fmt.Errorf("%w: %s produced no page", ErrInvalidFact, current.kind)
		}
		stats, err := catalog.ReconcilePlatformObjects(ctx, request.OrganizationID, request.ProjectID, request.AccountRef, runID, current.kind, observedAt, candidates)
		if err != nil {
			return result, err
		}
		result[current.kind] = stats
	}
	return result, nil
}

func (s Synchronizer) readMultiLeadOrangeLandingPageActions(ctx context.Context, request SyncRequest, runID string, reader platformObjectReader, limit, maxPages int) (map[string][]string, error) {
	result := map[string][]string{}
	for _, externalAction := range []int{2, 100} {
		for page := 1; page <= maxPages; page++ {
			if err := s.Writer.UpdateSyncCursor(ctx, runID, fmt.Sprintf("platform_objects:orange_landing_page:multi_lead:action:%d:page:%d", externalAction, page)); err != nil {
				return nil, err
			}
			filter := oceanengine.OrangeLandingPageFilter{
				MultiAssetTypes:       []int{2},
				ExternalAction:        externalAction,
				FilterDPA:             true,
				CheckConversionTarget: true,
				ConvertTargetForCheck: externalAction,
			}
			payload, err := reader.FilteredOrangeLandingPagesPage(ctx, oceanengine.AssetPageRequest{Page: page, Limit: limit}, filter)
			if err != nil {
				return nil, platformObjectReadError{stage: fmt.Sprintf("orange_landing_page:multi_lead:%d", externalAction), err: err}
			}
			queryEvidence := map[string]any{
				"page": page, "limit": limit, "search": "", "order_mode": 1, "search_mode": 3,
				"need_uba": false, "audit_status_list": []int{0, 9, 1, 10}, "multi_asset_types": []int{2},
				"external_action": externalAction, "status": []int{0, 5, 8}, "filter_dpa": 1, "convert_target_for_check": externalAction,
			}
			if _, _, err = s.storeRaw(ctx, request, runID, "orange_landing_page_multi_lead", queryEvidence, payload); err != nil {
				return nil, err
			}
			parsed := orangeLandingPage(payload)
			for _, item := range parsed.Items {
				if candidate, valid := orangeLandingCandidate(item); valid {
					result[candidate.PlatformObjectID] = appendUniqueString(result[candidate.PlatformObjectID], strconv.Itoa(externalAction))
				}
			}
			if (parsed.TotalPages > 0 && page >= parsed.TotalPages) || (parsed.TotalPages == 0 && len(parsed.Items) < limit) {
				break
			}
			if page == maxPages {
				return nil, fmt.Errorf("%w: orange_landing_page multi-lead action %d exceeds page limit", ErrInvalidFact, externalAction)
			}
		}
	}
	for id := range result {
		sort.Strings(result[id])
	}
	return result, nil
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func (s Synchronizer) syncOptimizationObjects(ctx context.Context, request SyncRequest, runID string, reader platformObjectReader, catalog PlatformObjectCatalog) (map[PlatformObjectKind]PlatformObjectSyncStats, error) {
	type optimizationContext struct {
		name       string
		assetType  int
		needAssets bool
	}
	contexts := []optimizationContext{{"orange_landing_page", 2, false}, {"owned_landing_page", 3, true}}
	targets := map[string]*PlatformObjectCandidate{}
	assets := map[string]*PlatformObjectCandidate{}
	observedAt := time.Time{}
	for _, current := range contexts {
		if err := s.Writer.UpdateSyncCursor(ctx, runID, "platform_objects:optimization_target:"+current.name); err != nil {
			return nil, err
		}
		payload, err := reader.OptimizationTargets(ctx, current.assetType, current.needAssets)
		if err != nil {
			return nil, platformObjectReadError{stage: "optimization_target:" + current.name, err: err}
		}
		_, collectedAt, err := s.storeRaw(ctx, request, runID, "optimization_target_"+current.name, map[string]any{"asset_type": current.assetType, "need_assets": current.needAssets}, payload)
		if err != nil {
			return nil, err
		}
		observedAt = collectedAt
		mergeOptimizationPayload(payload, current.name, targets, assets)
	}
	result := make(map[PlatformObjectKind]PlatformObjectSyncStats, 2)
	for _, kind := range []PlatformObjectKind{PlatformObjectOptimizationTarget, PlatformObjectConversionAsset} {
		source := targets
		if kind == PlatformObjectConversionAsset {
			source = assets
		}
		ids := make([]string, 0, len(source))
		for id := range source {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		candidates := make([]PlatformObjectCandidate, 0, len(ids))
		for _, id := range ids {
			candidates = append(candidates, *source[id])
		}
		stats, err := catalog.ReconcilePlatformObjects(ctx, request.OrganizationID, request.ProjectID, request.AccountRef, runID, kind, observedAt, candidates)
		if err != nil {
			return result, err
		}
		result[kind] = stats
	}
	return result, nil
}

func imageMaterialPage(payload map[string]any) platformObjectPage {
	data, _ := payload["data"].(map[string]any)
	return platformObjectPage{Items: mapItems(data["images"]), TotalPages: totalPages(data, 0)}
}

func videoMaterialPage(payload map[string]any) platformObjectPage {
	data, _ := payload["data"].(map[string]any)
	return platformObjectPage{Items: mapItems(data["videos"]), TotalPages: totalPages(data, 0)}
}

func awemePhotoMaterialPage(payload map[string]any) platformObjectPage {
	data, _ := payload["data"].(map[string]any)
	return platformObjectPage{Items: mapItems(data["list"]), TotalPages: totalPages(data, 0)}
}

func marketingProductPage(payload map[string]any) platformObjectPage {
	data, _ := payload["data"].(map[string]any)
	return platformObjectPage{Items: mapItems(data["list"]), TotalPages: totalPages(data, 32)}
}

func orangeLandingPage(payload map[string]any) platformObjectPage {
	data, _ := payload["data"].(map[string]any)
	return platformObjectPage{Items: mapItems(data["data"]), TotalPages: totalPages(data, 30)}
}

func industryCategoryPage(payload map[string]any) platformObjectPage {
	items := []map[string]any{}
	flattenIndustryCategories(payload["data"], nil, nil, &items)
	return platformObjectPage{Items: items, TotalPages: 1}
}

func flattenIndustryCategories(value any, parentIDs, parentLabels []string, result *[]map[string]any) {
	for _, item := range mapItems(value) {
		id := firstString(item, "id", "value")
		label := firstString(item, "label")
		ids := append(append([]string{}, parentIDs...), id)
		labels := append(append([]string{}, parentLabels...), label)
		copyItem := make(map[string]any, len(item)+2)
		for key, field := range item {
			if key != "children" {
				copyItem[key] = field
			}
		}
		copyItem["category_id_path"] = ids
		copyItem["category_path"] = labels
		*result = append(*result, copyItem)
		flattenIndustryCategories(item["children"], ids, labels, result)
	}
}

func brandPage(payload map[string]any) platformObjectPage {
	data, _ := payload["data"].(map[string]any)
	return platformObjectPage{Items: mapItems(data["data"]), TotalPages: 1}
}

func authorizedIdentityPage(payload map[string]any) platformObjectPage {
	extra, _ := payload["extra"].(map[string]any)
	items := mapItems(payload["data"])
	hasMore, _ := extra["hasMore"].(bool)
	if !hasMore {
		return platformObjectPage{Items: items, TotalPages: 1}
	}
	total := int(numberValue(extra["total"]))
	pages := 2
	if total > 0 && len(items) > 0 {
		pages = int(math.Ceil(float64(total) / float64(len(items))))
	}
	return platformObjectPage{Items: items, TotalPages: pages}
}

func mergeOptimizationPayload(payload map[string]any, contextName string, targets, assets map[string]*PlatformObjectCandidate) {
	data, _ := payload["data"].(map[string]any)
	for _, goal := range mapItems(data["goals"]) {
		id := firstString(goal, "external_action")
		if !validPlatformObjectID(PlatformObjectOptimizationTarget, id) {
			continue
		}
		candidate := targets[id]
		if candidate == nil {
			value := PlatformObjectCandidate{
				Kind: PlatformObjectOptimizationTarget, PlatformObjectID: id,
				DisplayName: firstString(goal, "optimization_name"),
				Metadata:    structuredMetadata(goal, "optimization_event_type", "external_action", "is_gray", "history_back", "twenty_four_hour_back", "asset_types", "current_account_back", "other_account_back", "track_type_match", "track_type", "value_type", "deep_goal_required", "limit"),
			}
			candidate = &value
			targets[id] = candidate
		}
		candidate.Metadata["contexts"] = appendUniqueString(candidate.Metadata["contexts"], contextName)
		for _, asset := range objectItems(goal["asset_info"]) {
			assetID := firstString(asset, "asset_id")
			if !validPlatformObjectID(PlatformObjectConversionAsset, assetID) {
				continue
			}
			assetCandidate := assets[assetID]
			if assetCandidate == nil {
				value := PlatformObjectCandidate{
					Kind: PlatformObjectConversionAsset, PlatformObjectID: assetID,
					DisplayName: firstString(asset, "asset_name"),
					Metadata:    structuredMetadata(asset, "limit", "role"),
				}
				assetCandidate = &value
				assets[assetID] = assetCandidate
			}
			assetCandidate.Metadata["optimization_target_ids"] = appendUniqueString(assetCandidate.Metadata["optimization_target_ids"], id)
			assetCandidate.Metadata["contexts"] = appendUniqueString(assetCandidate.Metadata["contexts"], contextName)
			candidate.Metadata["conversion_event_asset_ids"] = appendUniqueString(candidate.Metadata["conversion_event_asset_ids"], assetID)
		}
	}
}

func appendUniqueString(value any, next string) []string {
	items, _ := value.([]string)
	for _, item := range items {
		if item == next {
			return items
		}
	}
	return append(items, next)
}

func objectItems(value any) []map[string]any {
	if item, ok := value.(map[string]any); ok {
		return []map[string]any{item}
	}
	return mapItems(value)
}

func totalPages(data map[string]any, fallbackPageSize int) int {
	pagination, _ := data["pagination"].(map[string]any)
	if pages := int(numberValue(pagination["total_page"])); pages > 0 {
		return pages
	}
	total := int(numberValue(pagination["total"]))
	if total < 1 {
		total = int(numberValue(pagination["total_count"]))
	}
	pageSize := int(numberValue(pagination["size"]))
	if pageSize < 1 {
		pageSize = int(numberValue(pagination["limit"]))
	}
	if pageSize < 1 {
		pageSize = fallbackPageSize
	}
	if total > 0 && pageSize > 0 {
		return int(math.Ceil(float64(total) / float64(pageSize)))
	}
	return 0
}

func mapItems(value any) []map[string]any {
	items, _ := value.([]any)
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if mapped, ok := item.(map[string]any); ok {
			result = append(result, mapped)
		}
	}
	return result
}

func imageMaterialCandidate(item map[string]any) (PlatformObjectCandidate, bool) {
	id := firstString(item, "material_id")
	if !numericPlatformObjectID(id) {
		return PlatformObjectCandidate{}, false
	}
	previewURL, expiresAt := platformPreview(firstString(item, "sign_url"))
	return PlatformObjectCandidate{
		Kind: PlatformObjectImageMaterial, PlatformObjectID: id,
		DisplayName: firstString(item, "file_name"),
		Metadata:    scalarMetadata(item, "width", "height", "size", "image_mode", "ratio", "create_time", "web_uri"),
		PreviewURL:  previewURL, PreviewKind: previewKind(previewURL, "image"), PreviewExpiresAt: expiresAt,
	}, true
}

func productImageCandidate(item map[string]any) (PlatformObjectCandidate, bool) {
	candidate, valid := imageMaterialCandidate(item)
	if valid {
		candidate.Kind = PlatformObjectProductImage
	}
	return candidate, valid
}

func videoMaterialCandidate(item map[string]any) (PlatformObjectCandidate, bool) {
	id := firstString(item, "material_id", "video_id")
	if !numericPlatformObjectID(id) {
		return PlatformObjectCandidate{}, false
	}
	previewURL, expiresAt := firstPlatformPreview(item, "sign_url", "video_poster")
	return PlatformObjectCandidate{
		Kind: PlatformObjectVideoMaterial, PlatformObjectID: id,
		DisplayName: firstString(item, "video_name"),
		Metadata:    scalarMetadata(item, "video_filmLength", "image_mode", "is_low_quality", "similar_material_status", "related_creative_count", "create_time"),
		PreviewURL:  previewURL, PreviewKind: previewKind(previewURL, "video_poster"), PreviewExpiresAt: expiresAt,
	}, true
}

func awemePhotoMaterialCandidate(item map[string]any) (PlatformObjectCandidate, bool) {
	id := firstString(item, "material_id")
	if !numericPlatformObjectID(id) {
		return PlatformObjectCandidate{}, false
	}
	previewURL, expiresAt := nestedPlatformPreview(item, "image_info", "sign_url")
	metadata := scalarMetadata(item, "carousel_type", "origin_source", "source", "image_mode", "music_id", "create_time", "update_time")
	metadata["image_count"] = float64(len(mapItems(item["image_info"])))
	return PlatformObjectCandidate{
		Kind: PlatformObjectAwemePhotoMaterial, PlatformObjectID: id,
		DisplayName: firstString(item, "file_name"), Metadata: metadata,
		PreviewURL: previewURL, PreviewKind: previewKind(previewURL, "image"), PreviewExpiresAt: expiresAt,
	}, true
}

func marketingProductCandidate(item map[string]any) (PlatformObjectCandidate, bool) {
	id := firstString(item, "unique_product_id")
	if !numericPlatformObjectID(id) {
		return PlatformObjectCandidate{}, false
	}
	metadata := scalarMetadata(item, "unique_product_id", "product_id", "platform_product_id", "category_id", "brand_name", "audit_status", "online_status", "status", "type", "create_time", "modify_time", "online_time")
	if category, ok := item["clue_product_category"].(map[string]any); ok {
		if name := firstString(category, "category_name"); name != "" {
			metadata["category_name"] = name
		}
	}
	return PlatformObjectCandidate{
		Kind: PlatformObjectMarketingProduct, PlatformObjectID: id,
		DisplayName: firstString(item, "name", "title"), Metadata: metadata,
	}, true
}

func orangeLandingCandidate(item map[string]any) (PlatformObjectCandidate, bool) {
	id := firstString(item, "site_id")
	if !numericPlatformObjectID(id) {
		return PlatformObjectCandidate{}, false
	}
	previewURL, expiresAt := firstPlatformPreview(item, "preview_url", "url")
	return PlatformObjectCandidate{
		Kind: PlatformObjectOrangeLandingPage, PlatformObjectID: id,
		DisplayName: firstString(item, "name"),
		Metadata:    scalarMetadata(item, "audit_status", "status", "share_mode", "create_time"),
		PreviewURL:  previewURL, PreviewKind: previewKind(previewURL, "landing_page"), PreviewExpiresAt: expiresAt,
	}, true
}

func industryCategoryCandidate(item map[string]any) (PlatformObjectCandidate, bool) {
	id := firstString(item, "id", "value")
	if !validPlatformObjectID(PlatformObjectIndustryCategory, id) {
		return PlatformObjectCandidate{}, false
	}
	metadata := scalarMetadata(item, "value")
	if path, ok := item["category_path"].([]string); ok {
		metadata["category_path"] = path
	}
	if path, ok := item["category_id_path"].([]string); ok {
		metadata["category_id_path"] = path
	}
	return PlatformObjectCandidate{Kind: PlatformObjectIndustryCategory, PlatformObjectID: id, DisplayName: firstString(item, "label"), Metadata: metadata}, true
}

func brandCandidate(item map[string]any) (PlatformObjectCandidate, bool) {
	id := firstString(item, "cdp_brand_id")
	if !validPlatformObjectID(PlatformObjectBrand, id) {
		return PlatformObjectCandidate{}, false
	}
	return PlatformObjectCandidate{
		Kind: PlatformObjectBrand, PlatformObjectID: id,
		DisplayName: firstString(item, "brand_name"),
		Metadata:    structuredMetadata(item, "ecom_brand_id", "yuntu_brand_id", "sub_brand_map", "ecom_brand_name"),
	}, true
}

func authorizedIdentityCandidate(item map[string]any) (PlatformObjectCandidate, bool) {
	id := firstString(item, "ies_core_id")
	if !validPlatformObjectID(PlatformObjectAuthorizedIdentity, id) {
		return PlatformObjectCandidate{}, false
	}
	previewURL, expiresAt := platformPreview(firstString(item, "ies_avatar_url"))
	return PlatformObjectCandidate{
		Kind: PlatformObjectAuthorizedIdentity, PlatformObjectID: id,
		DisplayName: firstString(item, "ies_user_name", "ies_id"),
		Metadata:    structuredMetadata(item, "ies_id", "auth_level", "ies_bind_limits", "ies_avatar_uri", "aweme_user_type", "is_business_account", "ban_push_item"),
		PreviewURL:  previewURL, PreviewKind: previewKind(previewURL, "image"), PreviewExpiresAt: expiresAt,
	}, true
}

func firstPlatformPreview(item map[string]any, keys ...string) (string, *time.Time) {
	for _, key := range keys {
		if raw, ok := item[key].(string); ok {
			if value, expiresAt := platformPreview(raw); value != "" {
				return value, expiresAt
			}
		}
	}
	return "", nil
}

func nestedPlatformPreview(item map[string]any, listKey, urlKey string) (string, *time.Time) {
	for _, nested := range mapItems(item[listKey]) {
		if value, expiresAt := platformPreview(firstString(nested, urlKey)); value != "" {
			return value, expiresAt
		}
	}
	return "", nil
}

func platformPreview(raw string) (string, *time.Time) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 8192 {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", nil
	}
	for _, key := range []string{"x-orig-expires", "expires", "x-expires"} {
		seconds, err := strconv.ParseInt(parsed.Query().Get(key), 10, 64)
		if err == nil && seconds > 0 {
			value := time.Unix(seconds, 0).UTC()
			return raw, &value
		}
	}
	return raw, nil
}

func previewKind(previewURL, kind string) string {
	if previewURL == "" {
		return ""
	}
	return kind
}

func scalarMetadata(item map[string]any, keys ...string) map[string]any {
	result := map[string]any{}
	for _, key := range keys {
		switch value := item[key].(type) {
		case string:
			value = strings.TrimSpace(value)
			if value != "" && !strings.HasPrefix(strings.ToLower(value), "http://") && !strings.HasPrefix(strings.ToLower(value), "https://") {
				result[key] = value
			}
		case bool, float64, json.Number:
			result[key] = value
		}
	}
	return result
}

func structuredMetadata(item map[string]any, keys ...string) map[string]any {
	result := map[string]any{}
	for _, key := range keys {
		if value, ok := safeMetadataValue(item[key]); ok {
			result[key] = value
		}
	}
	return result
}

func safeMetadataValue(value any) (any, bool) {
	switch typed := value.(type) {
	case string:
		typed = strings.TrimSpace(typed)
		if typed == "" || strings.HasPrefix(strings.ToLower(typed), "http://") || strings.HasPrefix(strings.ToLower(typed), "https://") {
			return nil, false
		}
		return typed, true
	case bool, float64, json.Number:
		return typed, true
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			if safe, ok := safeMetadataValue(item); ok {
				result = append(result, safe)
			}
		}
		return result, true
	case map[string]any:
		result := map[string]any{}
		for key, item := range typed {
			if safe, ok := safeMetadataValue(item); ok {
				result[key] = safe
			}
		}
		return result, true
	default:
		return nil, false
	}
}
