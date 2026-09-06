package oceanengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type ListRequest struct {
	Start string
	End   string
	Page  int
	Limit int
}

type StatQueryRequest struct {
	DatasetKey string
	Dimensions []string
	Metrics    []string
	StartTime  string
	EndTime    string
	Offset     int
	Limit      int
	Host       string
	Extra      map[string]any
}

type AssetPageRequest struct {
	Page  int
	Limit int
}

const (
	OrangeLandingPageConsultType            = 172
	OrangeLandingPageTrafficEcommerceType   = 174
	OrangeLandingPageDeepTargetFirstBypass  = 607
	OrangeLandingPageDeepTargetSecondBypass = 608
)

// OrangeLandingPageFilter contains the business filters used by the live
// Orange landing-page picker. Zero values omit optional query parameters.
type OrangeLandingPageFilter struct {
	Search                    string
	OrderMode                 int
	SearchMode                int
	NeedUBA                   bool
	AuditStatuses             []int
	MultiAssetTypes           []int
	ExternalAction            int
	DeepExternalAction        int
	Statuses                  []int
	FilterDPA                 bool
	TypeList                  int
	FilterTypeList            int
	MicroAppInstanceID        string
	EBPGroupIDs               []string
	CheckConversionTarget     bool
	ConvertTargetForCheck     int
	DeepConvertTargetForCheck int
}

// OptimizationTargetContext is the complete parent form state used by
// get_optimization_goal_v2. The same account can return different goals for
// different values in this context.
type OptimizationTargetContext struct {
	CampaignType       int    `json:"campaign_type"`
	LandingType        int    `json:"landing_type"`
	AssetType          int    `json:"asset_type"`
	MicroAppID         string `json:"micro_app_id"`
	CDPMarketingGoal   int    `json:"cdp_marketing_goal"`
	DPAAdType          int    `json:"dpa_ad_type"`
	MicroPromotionType int    `json:"micro_promotion_type"`
	MicroAppInstanceID string `json:"micro_app_instance_id"`
	MultiAssetTypes    []int  `json:"multi_asset_types,omitempty"`
	NeedAssets         bool   `json:"need_assets"`
}

type ReferenceReadError struct {
	Stage string
	Err   error
}

func (e ReferenceReadError) Error() string     { return "Ocean Engine reference read failed" }
func (e ReferenceReadError) Unwrap() error     { return e.Err }
func (e ReferenceReadError) ReadStage() string { return e.Stage }

type Reader interface {
	ListPage(context.Context, ListRequest) (map[string]any, error)
	PromotionConfiguration(context.Context, string) (map[string]any, error)
	PromotionMaterials(context.Context, string, bool) (map[string]any, error)
	Attributes(context.Context, []string, string) (map[string]any, error)
	StatQueryPage(context.Context, StatQueryRequest) (map[string]any, error)
	AccountInfo(context.Context) (map[string]any, error)
}

func (c *Client) ListPage(ctx context.Context, request ListRequest) (map[string]any, error) {
	body := map[string]any{
		"st": request.Start, "et": request.End, "page": request.Page, "limit": request.Limit,
		"sort_stat": "create_time", "project_status": []int{-1}, "promotion_status": []int{-1},
		"sort_order": 1, "campaign_type": []int{1}, "fields": []string{"stat_cost", "show_cnt", "click_cnt", "convert_cnt"},
		"isSophonx": 1, "project_ids": []string{}, "cascade_fields": []string{"disable_by_cpl2"}, "metrics_range_filter": []any{},
	}
	return c.postJSON(ctx, "/ad/api/promotion/ads/list", body)
}

// ProjectListContract and PromotionListContract are exact readback paths for
// Web API write reconciliation. Callers must query by the unique object name.
func (c *Client) ProjectListContract(ctx context.Context, request any) (map[string]any, error) {
	return c.postJSON(ctx, ProjectListPath, request)
}

func (c *Client) PromotionListContract(ctx context.Context, request any) (map[string]any, error) {
	return c.postJSON(ctx, PromotionListPath, request)
}

func (c *Client) PromotionConfiguration(ctx context.Context, promotionID string) (map[string]any, error) {
	return c.getJSON(ctx, "/ad/api/promotion/ads/get_promotion_detail?promotion_ids="+promotionID)
}

func (c *Client) PromotionMaterials(ctx context.Context, promotionID string, needGroup bool) (map[string]any, error) {
	path := "/superior/api/ad/promotion/detail?promotion_ids=" + promotionID + "&need_invisible_material=false&need_material_group=" + fmt.Sprintf("%t", needGroup)
	return c.getJSON(ctx, path)
}

func (c *Client) Attributes(ctx context.Context, promotionIDs []string, requestID string) (map[string]any, error) {
	body := map[string]any{"promotion_ids": promotionIDs, "cascade_fields": []string{"diagnosis", "diagnosis_interfere_status", "compensate_status"}, "need_trans_toLocal": true, "ad_list_request_id": requestID}
	return c.postJSON(ctx, "/ad/api/promotion/ads/attribute/list", body)
}

func (c *Client) StatQueryPage(ctx context.Context, request StatQueryRequest) (map[string]any, error) {
	path := "/report/api/tool/agw/statistics_sophonx/statQuery"
	if request.Host == "ad" {
		path = "/ad/api/agw/statistics_sophonx/statQuery"
	}
	body := map[string]any{
		"DataSetKey": request.DatasetKey, "Dimensions": request.Dimensions, "StartTime": request.StartTime, "EndTime": request.EndTime,
		"Filters":    map[string]any{"ConditionRelationshipType": 1, "Conditions": []any{map[string]any{"Field": "advertiser_id", "Operator": 7, "Values": []string{c.AdvertiserID}}}},
		"IsDownload": false, "Metrics": request.Metrics, "PageParams": map[string]int{"Limit": request.Limit, "Offset": request.Offset},
	}
	if len(request.Dimensions) > 0 {
		body["OrderBy"] = []any{map[string]any{"Field": request.Dimensions[0], "Type": 2}}
	}
	if request.Extra != nil {
		body["Extra"] = request.Extra
	}
	return c.postJSON(ctx, path, body)
}

func (c *Client) AccountInfo(ctx context.Context) (map[string]any, error) {
	paths := []string{"/ad/api/account/info", "/superior/api/v2/account/info", "/ad/api/account/conf"}
	var value map[string]any
	var err error
	for index, path := range paths {
		value, err = c.getJSON(ctx, path)
		if err == nil || index == len(paths)-1 || !accountInfoFallbackAllowed(err) {
			return value, err
		}
	}
	return value, err
}

// AccountConfiguration reads the account-scoped frontend capability catalog.
// It contains dictionaries, feature gates, quotas, and component rules. It
// does not replace the branch-specific optimization capability endpoint.
func (c *Client) AccountConfiguration(ctx context.Context) (map[string]any, error) {
	return c.getJSON(ctx, "/superior/api/v2/account/conf")
}

// ImageMaterialsPage reads one image-library page. The endpoint is read-only
// and was verified by the Connector prototype against the live asset picker.
func (c *Client) ImageMaterialsPage(ctx context.Context, request AssetPageRequest) (map[string]any, error) {
	page, limit := normalizeAssetPage(request)
	body := map[string]any{
		"sort_by": "create_time", "sort_type": "desc",
		"metric_names": []string{"stat_cost", "ctr"},
		"image_modes":  []int{3, 16, 2}, "use_pre_audit_result": true,
		"is_need_stats_cost": true, "limit": limit, "page": page,
	}
	return c.postJSON(ctx, "/superior/api/v2/ad/getImageList", body)
}

// ProductImagesPage reads one page from the product-image "My Images" picker.
// Ocean Engine separates this catalog with image_mode 649502.
func (c *Client) ProductImagesPage(ctx context.Context, request AssetPageRequest) (map[string]any, error) {
	page, limit := normalizeAssetPage(request)
	if limit > 30 {
		limit = 30
	}
	body := map[string]any{"page": page, "limit": limit, "image_mode": 649502}
	return c.postJSON(ctx, "/superior/api/v2/ad/getImageList", body)
}

// VideoMaterialsPage reads one video-library page.
func (c *Client) VideoMaterialsPage(ctx context.Context, request AssetPageRequest) (map[string]any, error) {
	page, limit := normalizeAssetPage(request)
	query := url.Values{
		"image_mode": {"5,15"}, "ad_id": {"0"}, "sort_type": {"desc"},
		"metric_names": {"create_time,stat_cost,ctr"}, "landing_type": {"17"},
		"external_action": {"20"}, "page": {strconv.Itoa(page)},
		"limit": {strconv.Itoa(limit)}, "version": {"v2"}, "operation_platform": {"1"},
	}
	return c.getJSON(ctx, "/superior/api/v2/video/list?"+query.Encode())
}

// AwemePhotoMaterialsPage reads one Douyin image-text material page.
func (c *Client) AwemePhotoMaterialsPage(ctx context.Context, request AssetPageRequest) (map[string]any, error) {
	page, limit := normalizeAssetPage(request)
	query := url.Values{
		"sort_by": {"1"}, "sort_type": {"desc"}, "tab": {"myAwemePhoto"},
		"fields[]": {"stat_cost", "ctr"}, "page": {strconv.Itoa(page)}, "limit": {strconv.Itoa(limit)},
	}
	return c.getJSON(ctx, "/superior/api/v2/creative/material/aweme_photo_list?"+query.Encode())
}

// MarketingProductsPage reads one account product-library page.
func (c *Client) MarketingProductsPage(ctx context.Context, request AssetPageRequest) (map[string]any, error) {
	page, _ := normalizeAssetPage(request)
	body := map[string]any{"keywords": "", "category_id": "", "page": page, "ebp_asset_scope": 3}
	return c.postJSON(ctx, "/superior/api/v2/ad/product/clue_product_list", body)
}

// OrangeLandingPagesPage reads one Orange third-party landing-page page.
func (c *Client) OrangeLandingPagesPage(ctx context.Context, request AssetPageRequest) (map[string]any, error) {
	page, limit := normalizeAssetPage(request)
	query := url.Values{
		"page": {strconv.Itoa(page)}, "size": {strconv.Itoa(limit)},
		"order_mode": {"1"}, "search_mode": {"3"}, "need_uba": {"false"},
		"audit_status_list": {"0, 9, 1, 10"},
	}
	return c.getJSON(ctx, "/platform/api/v1/orange/third_part_list?"+query.Encode())
}

// FilteredOrangeLandingPagesPage reads one Orange landing-page picker branch.
// The query rules match the currently observed Superior business bundle.
func (c *Client) FilteredOrangeLandingPagesPage(ctx context.Context, request AssetPageRequest, filter OrangeLandingPageFilter) (map[string]any, error) {
	page, limit := normalizeAssetPage(request)
	orderMode := filter.OrderMode
	if orderMode <= 0 {
		orderMode = 1
	}
	searchMode := filter.SearchMode
	if searchMode <= 0 {
		searchMode = 3
	}
	auditStatuses := filter.AuditStatuses
	if len(auditStatuses) == 0 {
		auditStatuses = []int{0, 9, 1, 10}
	}
	statuses := filter.Statuses
	if len(statuses) == 0 {
		statuses = []int{0, 5, 8}
	}
	query := url.Values{
		"page":              {strconv.Itoa(page)},
		"size":              {strconv.Itoa(limit)},
		"search":            {filter.Search},
		"order_mode":        {strconv.Itoa(orderMode)},
		"search_mode":       {strconv.Itoa(searchMode)},
		"need_uba":          {strconv.FormatBool(filter.NeedUBA)},
		"audit_status_list": {joinInts(auditStatuses, ", ")},
		"status":            {joinInts(statuses, ",")},
	}
	if filter.FilterDPA {
		query.Set("filter_dpa", "1")
	}
	setIntQuery(query, "type_list", filter.TypeList)
	setIntQuery(query, "filter_type_list", filter.FilterTypeList)
	setStringQuery(query, "instance_id", filter.MicroAppInstanceID)
	setIntListQuery(query, "multi_asset_types", filter.MultiAssetTypes)
	setStringListQuery(query, "ebp_group_ids", filter.EBPGroupIDs)

	deepTarget := filter.DeepExternalAction
	if deepTarget == 0 {
		deepTarget = filter.DeepConvertTargetForCheck
	}
	bypassConversionFilter := deepTarget == OrangeLandingPageDeepTargetFirstBypass || deepTarget == OrangeLandingPageDeepTargetSecondBypass
	if !bypassConversionFilter {
		setIntQuery(query, "external_action", filter.ExternalAction)
		setIntQuery(query, "deep_external_action", filter.DeepExternalAction)
		if filter.CheckConversionTarget {
			setIntQuery(query, "convert_target_for_check", filter.ConvertTargetForCheck)
			setIntQuery(query, "deep_convert_target_for_check", filter.DeepConvertTargetForCheck)
		}
	}
	return c.getJSON(ctx, "/superior/api/v2/ad/get_orange_landing_page?"+query.Encode())
}

// MultiConversionOrangeLandingPagesPage reads pages that the live picker
// permits for external_action=100 and the multi-lead-component branch.
func (c *Client) MultiConversionOrangeLandingPagesPage(ctx context.Context, request AssetPageRequest) (map[string]any, error) {
	return c.FilteredOrangeLandingPagesPage(ctx, request, OrangeLandingPageFilter{
		MultiAssetTypes:       []int{2},
		ExternalAction:        100,
		FilterDPA:             true,
		CheckConversionTarget: true,
		ConvertTargetForCheck: 100,
	})
}

func joinInts(values []int, separator string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, separator)
}

func setIntQuery(query url.Values, key string, value int) {
	if value != 0 {
		query.Set(key, strconv.Itoa(value))
	}
}

func setStringQuery(query url.Values, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		query.Set(key, value)
	}
}

func setIntListQuery(query url.Values, key string, values []int) {
	if len(values) > 0 {
		query.Set(key, joinInts(values, ","))
	}
}

func setStringListQuery(query url.Values, key string, values []string) {
	clean := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			clean = append(clean, value)
		}
	}
	if len(clean) > 0 {
		query.Set(key, strings.Join(clean, ","))
	}
}

// OptimizationTargets reads the project optimization targets for one landing-page carrier.
// assetType 2 is Orange and assetType 3 is an advertiser-owned landing page.
func (c *Client) OptimizationTargets(ctx context.Context, assetType int, needAssets bool) (map[string]any, error) {
	if (assetType != 2 && assetType != 3) || needAssets != (assetType == 3) {
		return nil, fmt.Errorf("invalid optimization target context")
	}
	body := map[string]any{
		"campaign_type": 1, "landing_type": 17, "asset_type": assetType,
		"micro_app_id": "", "cdp_marketing_goal": 1, "dpa_ad_type": 0,
		"micro_promotion_type": 2, "micro_app_instance_id": "", "need_assets": needAssets,
	}
	return c.adSurface().postJSON(ctx, "/superior/api/v2/project/get_optimization_goal_v2", body)
}

// OptimizationTargetCapabilities reads the account capability for one exact
// project branch. It does not create or change a platform object.
func (c *Client) OptimizationTargetCapabilities(ctx context.Context, request OptimizationTargetContext) (map[string]any, error) {
	if request.CampaignType <= 0 || request.LandingType <= 0 || request.AssetType <= 0 || request.CDPMarketingGoal <= 0 || request.MicroPromotionType <= 0 {
		return nil, fmt.Errorf("invalid optimization target context")
	}
	if len(request.MultiAssetTypes) > 8 {
		return nil, fmt.Errorf("invalid optimization target context")
	}
	return c.adSurface().postJSON(ctx, "/superior/api/v2/project/get_optimization_goal_v2", request)
}

// BrandIndustries reads the account product-category tree.
func (c *Client) BrandIndustries(ctx context.Context) (map[string]any, error) {
	organizationClient := *c
	organizationClient.AdvertiserID = ""
	global, err := organizationClient.GlobalInfo(ctx)
	customerID := ""
	if err == nil {
		data, _ := global["data"].(map[string]any)
		advertiser, _ := data["advertiser"].(map[string]any)
		customerID = firstScalarString(advertiser["customer_id"])
		if customerID == "" {
			customerID = findScalarByKey(global, "customer_id", 0)
		}
	}
	if customerID == "" {
		account, accountErr := c.AccountInfo(ctx)
		if accountErr == nil {
			customerID = findScalarByKey(account, "customer_id", 0)
		} else if err != nil {
			return nil, ReferenceReadError{Stage: "customer_context_read", Err: err}
		}
	}
	if customerID == "" {
		return nil, ReferenceReadError{Stage: "customer_context_missing", Err: fmt.Errorf("Ocean Engine global info has no customer context")}
	}
	query := url.Values{"customer_id": {customerID}}
	value, err := c.adSurface().getJSON(ctx, "/nbs/api/ads/brand/yuntu/query_brand_industry?"+query.Encode())
	if err != nil {
		return nil, ReferenceReadError{Stage: "category_request", Err: err}
	}
	return value, nil
}

// Brands reads the account brand list.
func (c *Client) Brands(ctx context.Context) (map[string]any, error) {
	return c.adSurface().getJSON(ctx, "/superior/api/v2/agw/ad/brand")
}

// AuthorizedIdentitiesPage reads one authorized delivery-identity page.
func (c *Client) AuthorizedIdentitiesPage(ctx context.Context, request AssetPageRequest) (map[string]any, error) {
	page, limit := normalizeAssetPage(request)
	body := map[string]any{
		"page_index": page, "page_size": limit, "need_limits_info": true,
		"need_limit_scenes": []int{4}, "level": []int{1, 4, 5, 7},
		"need_auth_extra_info": true, "dpa_id": "", "order_by": 1,
	}
	return c.adSurface().postJSON(ctx, "/superior/api/v2/ad/authorize/list", body)
}

func (c *Client) adSurface() *Client {
	if c.BaseURL == nil || !strings.HasSuffix(strings.ToLower(c.BaseURL.Hostname()), ".oceanengine.com") {
		return c
	}
	clone := *c
	base := *c.BaseURL
	base.Scheme = "https"
	base.Host = "ad.oceanengine.com"
	clone.BaseURL = &base
	return &clone
}

// SignPictureURIs gets new short-lived preview URLs for existing image URIs.
// The operation does not upload or change platform material.
func (c *Client) SignPictureURIs(ctx context.Context, uris []string) (map[string]any, error) {
	return c.postJSON(ctx, "/superior/api/v2/creative/material/picture/sign", map[string]any{"uris": uris})
}

func normalizeAssetPage(request AssetPageRequest) (int, int) {
	page, limit := request.Page, request.Limit
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 32
	}
	return page, limit
}

func accountInfoFallbackAllowed(err error) bool {
	var statusErr HTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode == http.StatusNotFound || statusErr.StatusCode == http.StatusMethodNotAllowed
	}
	var redirectErr RedirectBlockedError
	return errors.As(err, &redirectErr)
}

func (c *Client) GlobalInfo(ctx context.Context) (map[string]any, error) {
	return c.getJSON(ctx, "/api/ebp/ebp_info/get_global_info")
}

func (c *Client) getJSON(ctx context.Context, path string) (map[string]any, error) {
	return c.do(ctx, http.MethodGet, path, nil, "")
}
func (c *Client) postJSON(ctx context.Context, path string, value any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return c.do(ctx, http.MethodPost, path, bytes.NewReader(encoded), "application/json;charset=UTF-8")
}

func firstScalarString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		if typed >= 0 && typed == math.Trunc(typed) {
			return strconv.FormatInt(int64(typed), 10)
		}
	case json.Number:
		return string(typed)
	}
	return ""
}

func findScalarByKey(value any, key string, depth int) string {
	if depth > 8 {
		return ""
	}
	switch typed := value.(type) {
	case map[string]any:
		if found := firstScalarString(typed[key]); found != "" {
			return found
		}
		for _, nested := range typed {
			if found := findScalarByKey(nested, key, depth+1); found != "" {
				return found
			}
		}
	case []any:
		for _, nested := range typed {
			if found := findScalarByKey(nested, key, depth+1); found != "" {
				return found
			}
		}
	}
	return ""
}

func FlattenRows(rows any) []map[string]any {
	var leaves []map[string]any
	walkRows(rows, &leaves)
	return leaves
}
func walkRows(value any, leaves *[]map[string]any) {
	items, ok := value.([]any)
	if !ok {
		return
	}
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if nested, ok := row["Rows"].([]any); ok && len(nested) > 0 {
			walkRows(nested, leaves)
			continue
		}
		if _, ok := row["Metrics"]; ok {
			*leaves = append(*leaves, row)
		}
	}
}

// FlattenNamedRows collects every object carrying a non-empty value under the
// given name key. The aggregate platform views name rows project_name and
// promotion_name instead of name.
func FlattenNamedRows(value any, nameKey string) []map[string]any {
	var rows []map[string]any
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			if name, ok := typed[nameKey].(string); ok && name != "" {
				rows = append(rows, typed)
				return
			}
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return rows
}
