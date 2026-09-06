package oceanengine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestAssetLibraryReadersUseApprovedReadOnlyEndpoints(t *testing.T) {
	requests := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/superior/api/v2/ad/getImageList"},
		{http.MethodGet, "/superior/api/v2/video/list"},
		{http.MethodGet, "/superior/api/v2/creative/material/aweme_photo_list"},
		{http.MethodPost, "/superior/api/v2/ad/product/clue_product_list"},
		{http.MethodGet, "/platform/api/v1/orange/third_part_list"},
		{http.MethodGet, "/superior/api/v2/ad/get_orange_landing_page"},
	}
	index := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if index >= len(requests) {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		want := requests[index]
		index++
		if r.Method != want.method || r.URL.Path != want.path || r.URL.Query().Get("aadvid") != "123" {
			t.Fatalf("request=%s %s", r.Method, r.URL.RequestURI())
		}
		if r.Method == http.MethodPost && r.URL.Path == "/superior/api/v2/ad/getImageList" {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["page"] != float64(2) || body["limit"] != float64(20) {
				t.Fatalf("body=%#v err=%v", body, err)
			}
		} else if r.Method == http.MethodPost {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["page"] != float64(2) || body["ebp_asset_scope"] != float64(3) {
				t.Fatalf("body=%#v err=%v", body, err)
			}
		} else if r.URL.Query().Get("page") != "2" {
			t.Fatalf("query=%s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "123", Session{Cookies: "session=x"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.Delay = 0
	ctx := context.Background()
	request := AssetPageRequest{Page: 2, Limit: 20}
	if _, err = client.ImageMaterialsPage(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err = client.VideoMaterialsPage(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err = client.AwemePhotoMaterialsPage(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err = client.MarketingProductsPage(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err = client.OrangeLandingPagesPage(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err = client.MultiConversionOrangeLandingPagesPage(ctx, request); err != nil {
		t.Fatal(err)
	}
	if index != len(requests) {
		t.Fatalf("requests=%d", index)
	}
}

func TestMultiConversionOrangeLandingPagesUseExactLivePickerFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/superior/api/v2/ad/get_orange_landing_page" ||
			query.Get("aadvid") != "123" || query.Get("multi_asset_types") != "2" ||
			query.Get("external_action") != "100" || query.Get("convert_target_for_check") != "100" ||
			query.Get("filter_dpa") != "1" || query.Get("status") != "0,5,8" {
			t.Fatalf("request=%s %s", r.Method, r.URL.RequestURI())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "123", Session{Cookies: "session=x"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.Delay = 0
	if _, err = client.MultiConversionOrangeLandingPagesPage(context.Background(), AssetPageRequest{Page: 1, Limit: 30}); err != nil {
		t.Fatal(err)
	}
}

func TestAccountConfigurationAndProjectDetailsUseObservedReadContracts(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("aadvid") != "123" {
			t.Fatalf("account query=%s", r.URL.RawQuery)
		}
		switch requests {
		case 1:
			if r.Method != http.MethodGet || r.URL.Path != "/superior/api/v2/account/conf" {
				t.Fatalf("account capability request=%s %s", r.Method, r.URL.RequestURI())
			}
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/superior/api/v2/project/detail" || r.URL.Query().Get("project_ids") != "7681603698619908102" || r.URL.Query().Get("need_ea_conversion_status") != "true" || r.URL.Query().Get("need_product_recognition") != "true" {
				t.Fatalf("project detail request=%s %s", r.Method, r.URL.RequestURI())
			}
		default:
			t.Fatalf("unexpected request=%s", r.URL.RequestURI())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "123", Session{Cookies: "session=x"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.Delay = 0
	if _, err = client.AccountConfiguration(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = client.ProjectDetails(context.Background(), "7681603698619908102"); err != nil {
		t.Fatal(err)
	}
}

func TestFilteredOrangeLandingPagesIncludesObservedOptionalFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		checks := map[string]string{
			"aadvid":                        "123",
			"search":                        "site",
			"order_mode":                    "2",
			"search_mode":                   "4",
			"need_uba":                      "true",
			"audit_status_list":             "0, 1",
			"status":                        "0,8",
			"multi_asset_types":             "2,1002",
			"external_action":               "100",
			"deep_external_action":          "101",
			"convert_target_for_check":      "100",
			"deep_convert_target_for_check": "101",
			"filter_dpa":                    "1",
			"type_list":                     "172",
			"filter_type_list":              "174",
			"instance_id":                   "instance_1",
			"ebp_group_ids":                 "group_1,group_2",
		}
		if r.Method != http.MethodGet || r.URL.Path != "/superior/api/v2/ad/get_orange_landing_page" {
			t.Fatalf("request=%s %s", r.Method, r.URL.RequestURI())
		}
		for key, want := range checks {
			if got := query.Get(key); got != want {
				t.Errorf("%s=%q want %q", key, got, want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "123", Session{Cookies: "session=x"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.Delay = 0
	_, err = client.FilteredOrangeLandingPagesPage(context.Background(), AssetPageRequest{Page: 1, Limit: 30}, OrangeLandingPageFilter{
		Search: "site", OrderMode: 2, SearchMode: 4, NeedUBA: true,
		AuditStatuses: []int{0, 1}, MultiAssetTypes: []int{2, 1002},
		ExternalAction: 100, DeepExternalAction: 101, Statuses: []int{0, 8}, FilterDPA: true,
		TypeList: OrangeLandingPageConsultType, FilterTypeList: OrangeLandingPageTrafficEcommerceType,
		MicroAppInstanceID: "instance_1", EBPGroupIDs: []string{"group_1", "group_2"},
		CheckConversionTarget: true, ConvertTargetForCheck: 100, DeepConvertTargetForCheck: 101,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFilteredOrangeLandingPagesOmitsConversionFiltersForBypassDeepTargets(t *testing.T) {
	for _, deepTarget := range []int{OrangeLandingPageDeepTargetFirstBypass, OrangeLandingPageDeepTargetSecondBypass} {
		t.Run(strconv.Itoa(deepTarget), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				query := r.URL.Query()
				for _, key := range []string{"external_action", "deep_external_action", "convert_target_for_check", "deep_convert_target_for_check"} {
					if query.Has(key) {
						t.Errorf("unexpected %s=%q", key, query.Get(key))
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "123", Session{Cookies: "session=x"}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			client.Delay = 0
			_, err = client.FilteredOrangeLandingPagesPage(context.Background(), AssetPageRequest{Page: 1, Limit: 30}, OrangeLandingPageFilter{
				ExternalAction: 100, DeepExternalAction: deepTarget,
				CheckConversionTarget: true, ConvertTargetForCheck: 100, DeepConvertTargetForCheck: deepTarget,
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSignPictureURIsUsesReadOnlySigner(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/superior/api/v2/creative/material/picture/sign" || r.URL.Query().Get("aadvid") != "123" {
			t.Fatalf("request=%s %s", r.Method, r.URL.RequestURI())
		}
		var body struct {
			URIs []string `json:"uris"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.URIs) != 1 || body.URIs[0] != "tos-cn/image" {
			t.Fatalf("body=%#v err=%v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"list":{"tos-cn/image":{"main_url":"https://example.invalid/signed"}}}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "123", Session{Cookies: "session=x"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.Delay = 0
	if _, err := client.SignPictureURIs(context.Background(), []string{"tos-cn/image"}); err != nil {
		t.Fatal(err)
	}
}

func TestProductImagesPageUsesMyImagesMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/superior/api/v2/ad/getImageList" || r.URL.Query().Get("aadvid") != "123" {
			t.Fatalf("request=%s %s", r.Method, r.URL.RequestURI())
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["page"] != float64(1) || body["limit"] != float64(30) || body["image_mode"] != float64(649502) {
			t.Fatalf("body=%#v err=%v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"images":[]}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "123", Session{Cookies: "session=x"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.Delay = 0
	if _, err := client.ProductImagesPage(context.Background(), AssetPageRequest{Page: 1, Limit: 50}); err != nil {
		t.Fatal(err)
	}
}

func TestReferenceObjectReadersUseObservedReadOnlyRequests(t *testing.T) {
	requestIndex := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestIndex++
		if requestIndex != 3 && r.URL.Query().Get("aadvid") != "123" {
			t.Fatalf("missing advertiser context: %s", r.URL.RequestURI())
		}
		w.Header().Set("Content-Type", "application/json")
		switch requestIndex {
		case 1, 2:
			if r.Method != http.MethodPost || r.URL.Path != "/superior/api/v2/project/get_optimization_goal_v2" {
				t.Fatalf("request=%s %s", r.Method, r.URL.RequestURI())
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["asset_type"] != float64(requestIndex+1) || body["need_assets"] != (requestIndex == 2) || body["landing_type"] != float64(17) {
				t.Fatalf("optimization body=%#v err=%v", body, err)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"goals":[]}}`))
		case 3:
			if r.Method != http.MethodGet || r.URL.Path != "/api/ebp/ebp_info/get_global_info" || r.URL.Query().Get("aadvid") != "" {
				t.Fatalf("request=%s %s", r.Method, r.URL.RequestURI())
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"advertiser":{"customer_id":"456"}}}`))
		case 4:
			if r.Method != http.MethodGet || r.URL.Path != "/nbs/api/ads/brand/yuntu/query_brand_industry" || r.URL.Query().Get("customer_id") != "456" {
				t.Fatalf("request=%s %s", r.Method, r.URL.RequestURI())
			}
			_, _ = w.Write([]byte(`{"code":0,"data":[]}`))
		case 5:
			if r.Method != http.MethodGet || r.URL.Path != "/superior/api/v2/agw/ad/brand" {
				t.Fatalf("request=%s %s", r.Method, r.URL.RequestURI())
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"data":[]}}`))
		case 6:
			if r.Method != http.MethodPost || r.URL.Path != "/superior/api/v2/ad/authorize/list" {
				t.Fatalf("request=%s %s", r.Method, r.URL.RequestURI())
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["page_index"] != float64(2) || body["page_size"] != float64(10) || body["need_auth_extra_info"] != true {
				t.Fatalf("identity body=%#v err=%v", body, err)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":[],"extra":{"hasMore":false}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "123", Session{Cookies: "session=x"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.Delay = 0
	ctx := context.Background()
	if _, err = client.OptimizationTargets(ctx, 2, false); err != nil {
		t.Fatal(err)
	}
	if _, err = client.OptimizationTargets(ctx, 3, true); err != nil {
		t.Fatal(err)
	}
	if _, err = client.BrandIndustries(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Brands(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = client.AuthorizedIdentitiesPage(ctx, AssetPageRequest{Page: 2, Limit: 10}); err != nil {
		t.Fatal(err)
	}
	if requestIndex != 6 {
		t.Fatalf("requests=%d", requestIndex)
	}
}

func TestOptimizationTargetCapabilitiesPreserveCompleteLeadBranch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/superior/api/v2/project/get_optimization_goal_v2" || r.URL.Query().Get("aadvid") != "123" {
			t.Fatalf("request=%s %s", r.Method, r.URL.RequestURI())
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		multi, _ := body["multi_asset_types"].([]any)
		if body["campaign_type"] != float64(1) || body["landing_type"] != float64(1) || body["asset_type"] != float64(2) || body["need_assets"] != false || len(multi) != 2 || multi[0] != float64(2) || multi[1] != float64(1002) {
			t.Fatalf("body=%#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"goals":[]}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "123", Session{Cookies: "session=x"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.Delay = 0
	_, err = client.OptimizationTargetCapabilities(context.Background(), OptimizationTargetContext{
		CampaignType: 1, LandingType: 1, AssetType: 2, CDPMarketingGoal: 1,
		MicroPromotionType: 2, MultiAssetTypes: []int{2, 1002}, NeedAssets: false,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGlobalInfoUsesEnterpriseReadOnlyEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/ebp/ebp_info/get_global_info" || r.URL.RawQuery != "" {
			t.Errorf("unexpected enterprise request: %s %s", r.Method, r.URL.RequestURI())
		}
		if r.Header.Get("Cookie") != "session=x; csrftoken=csrf" || r.Header.Get("x-csrftoken") != "csrf" {
			t.Errorf("session headers not forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	}))
	defer server.Close()
	client, err := NewSessionClient(server.URL, Session{Cookies: "session=x; csrftoken=csrf"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.Delay = 0
	if _, err := client.GlobalInfo(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAccountInfoFallsBackToSuperiorReadOnlyEndpoint(t *testing.T) {
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/ad/api/account/info" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path != "/superior/api/v2/account/info" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "123", Session{Cookies: "session=x"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.Delay = 0
	if _, err = client.AccountInfo(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "/ad/api/account/info" || paths[1] != "/superior/api/v2/account/info" {
		t.Fatalf("paths=%v", paths)
	}
}

func TestAccountInfoUsesApprovedConfigurationEndpointAfterBlockedRedirects(t *testing.T) {
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path != "/ad/api/account/conf" {
			http.Redirect(w, r, "/unexpected-application-route", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"read_only":true}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "123", Session{Cookies: "session=x"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.Delay = 0
	if _, err = client.AccountInfo(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"/ad/api/account/info", "/superior/api/v2/account/info", "/ad/api/account/conf"}
	if len(paths) != len(want) {
		t.Fatalf("paths=%v", paths)
	}
	for index := range want {
		if paths[index] != want[index] {
			t.Fatalf("paths=%v", paths)
		}
	}
}

func TestReaderMethodsAndFlattenRows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ad/api/agw/statistics_sophonx/statQuery" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":{"StatsData":{"Rows":[{"Rows":[{"Metrics":{"stat_cost":{"Value":1}},"Rows":null},{"Metrics":{"stat_cost":{"Value":2}},"Rows":[]}]}]}}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"ok":true}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "123", Session{Cookies: "session=x"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.Delay = 0
	ctx := context.Background()
	if _, err := client.ListPage(ctx, ListRequest{Start: "2026-08-20", End: "2026-08-20", Page: 1, Limit: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PromotionConfiguration(ctx, "9000000000000000000000000001"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PromotionMaterials(ctx, "9000000000000000000000000001", true); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Attributes(ctx, []string{"9000000000000000000000000001"}, "request-1"); err != nil {
		t.Fatal(err)
	}
	response, err := client.StatQueryPage(ctx, StatQueryRequest{Host: "ad", DatasetKey: "basic_ad_data", Dimensions: []string{"stat_time_hour"}, Metrics: []string{"stat_cost"}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	stats := response["data"].(map[string]any)["StatsData"].(map[string]any)
	if got := len(FlattenRows(stats["Rows"])); got != 2 {
		t.Fatalf("flattened rows = %d, want 2", got)
	}
	if _, err := client.AccountInfo(ctx); err != nil {
		t.Fatal(err)
	}
}
