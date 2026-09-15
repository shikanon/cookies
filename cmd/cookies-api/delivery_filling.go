package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/connector"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/media"
	"github.com/shikanon/cookies/internal/platform/project"
	"github.com/shikanon/cookies/internal/systems/delivery"
	"github.com/shikanon/cookies/internal/systems/strategy"
)

// Wiring lives at the composition root; Delivery does not depend on Strategy's storage.
type deliveryFillingReader struct {
	projects   *project.Service
	assets     *assets.UploadService
	catalog    connector.MySQLRepository
	strategies *strategy.Service
	frames     media.FrameExtractor
	previews   connector.PlatformObjectPreviewRefresher
}

func (r *deliveryFillingReader) read(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, request delivery.FillingRequest) (delivery.FillingContext, error) {
	result := delivery.FillingContext{Choices: map[string][]delivery.FillingChoice{}, Sources: []string{}, Warnings: []string{"文案与自然语言策略限制仍需人工核对。"}}
	projectContext, err := r.projects.RequireActiveContext(ctx, actor, projectID)
	if err != nil {
		return result, err
	}
	result.Project = projectContext
	workbench, err := r.projects.GetWorkbench(ctx, actor, projectID)
	if err != nil {
		return result, err
	}
	detail, err := r.projects.GetDetail(ctx, actor, projectID)
	if err != nil {
		return result, err
	}
	var strategyProduct *strategy.BriefProduct
	strategySource := ""
	facts := map[string]any{"timezone": detail.Runtime.Timezone}
	add := func(field, id, label string, value any, source string) {
		raw, _ := json.Marshal(value)
		result.Choices[field] = append(result.Choices[field], delivery.FillingChoice{ID: id, Label: label, Value: raw, Source: source})
	}
	if request.Strategy != nil {
		if r.strategies == nil {
			return result, delivery.ErrFillingUnavailable
		}
		pkg, readErr := r.strategies.GetPackage(ctx, actor, projectID, request.Strategy.PackageID, request.Strategy.Version)
		if readErr != nil {
			return result, delivery.ErrInvalidRequest
		}
		if pkg.Status != "published" || pkg.Snapshot.Approval.ApprovedBy == "" || string(pkg.ContentHash) != request.Strategy.ContentHash {
			return result, delivery.ErrInvalidRequest
		}
		result.Strategy = &delivery.FillingStrategy{PackageID: pkg.PackageID, Version: pkg.Version, ContentHash: string(pkg.ContentHash)}
		facts["approved_strategy"] = pkg.Snapshot
		source := fmt.Sprintf("已批准策略 %s V%d", pkg.PackageID, pkg.Version)
		// Strategy evidence is attached only after the selected product is matched.
		brief := pkg.Snapshot.Brief.Snapshot
		result.ProhibitedClaims = brief.Creative.ProhibitedClaims
		result.Warnings = append(result.Warnings, brief.Constraints...)
		result.Warnings = append(result.Warnings, pkg.Snapshot.Strategy.Constraints...)
		strategyProduct = &brief.Product
		strategySource = source
	} else {
		result.Warnings = append(result.Warnings, "未选择已批准策略，本次依据商品资料、素材和附加说明填写。")
	}
	selected := request.Ocean.Project.MarketingProductReference
	for _, product := range workbench.Products {
		if selected == nil || selected.Namespace != "cookies" || selected.ID != string(product.ID) || (selected.Scope != "current_project" && selected.Scope != "project:"+string(projectID)) {
			continue
		}
		product, readErr := r.projects.GetProduct(ctx, actor, product.ID)
		if readErr != nil {
			return result, readErr
		}
		if product.Status != "active" {
			continue
		}
		facts["selected_product"] = product
		result.ConfirmedCopyFacts = append(result.ConfirmedCopyFacts, product.Name, product.BrandName, product.Description)
		result.CanGenerateSellingPoints = strings.TrimSpace(product.Description) != ""
		result.Sources = append(result.Sources, "已选项目产品 "+product.Name)
		add("product_name", "cookies:name:"+string(product.ID), product.Name, product.Name, "已选项目产品")
		if brand := strings.TrimSpace(product.BrandName); brand != "" {
			add("source_label", "cookies:source:"+string(product.ID), brand, brand, "已选项目产品品牌")
		}
	}
	accounts, err := r.catalog.ListAccounts(ctx, string(actor.OrganizationID), string(projectID))
	if err != nil {
		return result, err
	}
	accountID := request.Ocean.Project.AccountReference.ID
	foundAccount := accountID == ""
	for _, account := range accounts {
		if account.Status != "verified" || account.ProjectID != string(projectID) {
			continue
		}
		foundAccount = foundAccount || account.ID == accountID
	}
	if !foundAccount {
		return result, delivery.ErrInvalidRequest
	}
	p := request.Ocean.Project
	content := p.MarketingPurpose == "content_marketing"
	native := content && p.Carrier == "douyin_account"
	if !native && slices.Contains(request.Fields, "materials") {
		for _, asset := range workbench.AssetVersionPointers {
			if asset.HumanConfirmedVersion == nil || (request.Search != "" && !strings.Contains(strings.ToLower(asset.AssetID+" "+asset.OceanEngineMaterialID), strings.ToLower(request.Search))) {
				continue
			}
			for _, version := range asset.Versions {
				if version.Version != *asset.HumanConfirmedVersion {
					continue
				}
				media, readErr := r.assets.Get(ctx, actor, projectID, contract.AssetVersionRef{AssetID: contract.AssetID(asset.AssetID), Version: int64(version.Version)})
				if readErr != nil {
					return result, readErr
				}
				if media.Version.Status != assets.AssetReady || (media.Asset.Kind != "image" && media.Asset.Kind != "video") {
					continue
				}
				if !asset.Authorization.ExpiresAt.IsZero() && asset.Authorization.ExpiresAt.Before(time.Now()) {
					continue
				}
				hash := media.Version.SHA256
				ref := delivery.StableReference{Namespace: "cookies", ObjectKind: "asset_version", Scope: "project:" + string(projectID), ID: asset.AssetID, Version: strconv.Itoa(version.Version), ContentHash: string(hash), State: "resolved", DisplayNameSnapshot: asset.AssetID, AuditAttributes: map[string]string{"ocean_engine_material_id": media.Version.OceanEngineMaterialID, "media_kind": string(media.Asset.Kind)}}
				add("materials", "cookies:asset:"+asset.AssetID+"@"+ref.Version, asset.AssetID, ref, "当前项目人工确认的素材版本")
				result.Choices["materials"][len(result.Choices["materials"])-1].Metadata = map[string]any{"source_label": version.SourceLabel, "change_summary": version.ChangeSummary, "media_kind": media.Asset.Kind, "width": media.Version.WidthPixels, "height": media.Version.HeightPixels, "duration_ms": media.Version.DurationMS}
			}
		}
	}
	if accountID == "" {
		result.Warnings = append(result.Warnings, "请先选择账户，再填写账户内素材。")
	} else {
		kinds := map[connector.PlatformObjectKind]string{connector.PlatformObjectMarketingProduct: "product", connector.PlatformObjectAuthorizedIdentity: "identity", connector.PlatformObjectProductImage: "product_images", connector.PlatformObjectIndustryCategory: "category"}
		if native {
			delete(kinds, connector.PlatformObjectAuthorizedIdentity)
			kinds[connector.PlatformObjectDouyinVideo] = "materials"
		} else {
			kinds[connector.PlatformObjectVideoMaterial] = "materials"
			kinds[connector.PlatformObjectImageMaterial] = "materials"
			kinds[connector.PlatformObjectAwemePhotoMaterial] = "materials"
		}
		if p.Carrier == "orange_landing_page" || p.Carrier == "orange_landing_page_and_im" {
			kinds[connector.PlatformObjectOrangeLandingPage] = "landing_page"
		}
		orderedKinds := make([]connector.PlatformObjectKind, 0, len(kinds))
		for kind := range kinds {
			orderedKinds = append(orderedKinds, kind)
		}
		sort.Slice(orderedKinds, func(i, j int) bool {
			if orderedKinds[i] == connector.PlatformObjectMarketingProduct {
				return true
			}
			if orderedKinds[j] == connector.PlatformObjectMarketingProduct {
				return false
			}
			return orderedKinds[i] < orderedKinds[j]
		})
		for _, kind := range orderedKinds {
			field := kinds[kind]
			if !slices.Contains(request.Fields, field) && field != "product" {
				continue
			}
			cursor := ""
			search := ""
			if field == "materials" {
				search = request.Search
				if search == "" {
					if brands := result.Choices["source_label"]; len(brands) > 0 {
						search = brands[0].Label
					}
				}
			}
			if field == "product" {
				if selected == nil || selected.Namespace != "oceanengine" {
					continue
				}
				search = selected.ID
			}
			for count := 0; ; {
				items, readErr := r.catalog.ListPlatformObjects(ctx, connector.PlatformObjectQuery{OrganizationID: string(actor.OrganizationID), ProjectID: string(projectID), AccountID: accountID, Kind: kind, Status: "active", Search: search, Cursor: cursor, Limit: 100})
				if readErr != nil {
					return result, delivery.ErrFillingUnavailable
				}
				if len(items) == 0 {
					break
				}
				count += len(items)
				if count > 1000 {
					return result, delivery.ErrFillingCatalogTooLarge
				}
				for _, item := range items {
					if item.Status != "active" || item.AccountID != accountID {
						continue
					}
					ref := fillingObjectReference(item)
					if item.PreviewAvailable {
						ref.AuditAttributes["preview_url"] = "/api/connector/v1/projects/" + url.PathEscape(string(projectID)) + "/accounts/" + url.PathEscape(accountID) + "/platform-objects/" + url.PathEscape(item.ID) + "/preview"
					}
					if field == "landing_page" && !fillingLandingEligible(*p, item.Metadata) {
						continue
					}
					if field == "product" {
						if selected == nil || selected.ID != ref.ID || selected.Scope != ref.Scope || (selected.Version != "" && selected.Version != ref.Version) {
							continue
						}
						facts["selected_product"] = map[string]any{"name": item.DisplayName, "reference": ref, "brand_name": item.Metadata["brand_name"], "category_name": item.Metadata["category_name"]}
						result.Sources = append(result.Sources, "已选 Connector 产品 "+item.DisplayName+" V"+ref.Version)
						result.ConfirmedCopyFacts = append(result.ConfirmedCopyFacts, item.DisplayName, fillingMetadataString(item.Metadata["brand_name"]))
						add("product_name", "connector:name:"+item.ID, item.DisplayName, item.DisplayName, "已选 Connector 产品")
						if brand := fillingMetadataString(item.Metadata["brand_name"]); brand != "" {
							add("source_label", "connector:source:"+item.ID, brand, brand, "已选 Connector 产品 brand_name")
						}
						continue
					}
					add(field, "connector:"+item.ID, item.DisplayName, ref, "当前账户 Connector 目录")
					result.Choices[field][len(result.Choices[field])-1].Metadata = item.Metadata
					if result.Choices[field][len(result.Choices[field])-1].Metadata == nil {
						result.Choices[field][len(result.Choices[field])-1].Metadata = map[string]any{}
					}
					result.Choices[field][len(result.Choices[field])-1].Metadata["object_kind"] = string(item.Kind)
				}
				if len(items) < 100 {
					break
				}
				next := items[len(items)-1].ID
				if next == cursor {
					return result, delivery.ErrFillingUnavailable
				}
				cursor = next
			}
		}
	}

	if request.Search == "" {
		if brands := result.Choices["source_label"]; len(brands) > 0 && slices.Contains(request.Fields, "materials") {
			result.Warnings = append(result.Warnings, "已按品牌“"+brands[0].Label+"”检索平台素材，可调整素材筛选词。")
		}
	}
	if strategyProduct != nil {
		matched := false
		for _, choice := range result.Choices["product_name"] {
			if choice.Label == strategyProduct.Name {
				matched = true
			}
		}
		if matched {
			result.Sources = append(result.Sources, strategySource)
			result.ConfirmedCopyFacts = append(result.ConfirmedCopyFacts, strategyProduct.SellingPoints...)
			for i, point := range strategyProduct.SellingPoints {
				add("selling_points", fmt.Sprintf("strategy:point:%d", i), point, point, strategySource)
			}
		} else {
			delete(facts, "approved_strategy")
			result.Warnings = append(result.Warnings, "策略中的产品与当前已选产品不匹配，未将策略内容作为产品事实。")
		}
	}
	if facts["selected_product"] == nil {
		result.Warnings = append(result.Warnings, "未找到当前产品的有效资料，请先确认产品引用与目录版本。")
	} else {
		add("call_to_action", "cta:details", "查看详情", "查看详情", "不承诺优惠或效果的通用行动号召；请核对落地页")
	}
	for _, region := range []string{"all", "北京市", "上海市", "广东省", "浙江省", "江苏省", "四川省"} {
		label := region
		if region == "all" {
			label = "不限"
		}
		add("regions", "region:"+region, label, region, "平台配置地域选项")
	}
	for _, age := range []string{"all", "18-23", "24-30", "31-40", "41-49", "50+"} {
		label := age
		if age == "all" {
			label = "不限"
		}
		add("age_ranges", "age:"+age, label, age, "平台配置年龄选项")
	}
	for _, gender := range []struct{ value, label string }{{"", "不限"}, {"male", "男"}, {"female", "女"}} {
		add("gender", "gender:"+gender.value, gender.label, gender.value, "平台配置性别选项")
	}
	add("smart_expansion", "expansion:true", "开启", true, "平台配置智能定向扩展")
	add("smart_expansion", "expansion:false", "关闭", false, "平台配置智能定向扩展")
	add("search_expansion", "search_expansion:true", "开启", true, "平台配置搜索定向扩展")
	add("search_expansion", "search_expansion:false", "关闭", false, "平台配置搜索定向扩展")
	result.Facts, err = json.Marshal(facts)
	return result, err
}

func fillingObjectReference(item connector.PlatformObject) delivery.StableReference {
	ref := delivery.StableReference{Namespace: "oceanengine", ObjectKind: string(item.Kind), Scope: "account:" + item.AccountID, ID: item.PlatformObjectID, Version: strconv.FormatInt(item.Version, 10), State: "resolved", DisplayNameSnapshot: item.DisplayName, AuditAttributes: map[string]string{"connector_platform_object_id": item.ID, "preview_url": item.PreviewURL, "ocean_engine_material_id": item.PlatformObjectID}}
	if item.Kind == connector.PlatformObjectMarketingProduct {
		ref.ObjectKind = "product"
		if unique := fillingMetadataString(item.Metadata["unique_product_id"]); unique != "" {
			ref.ID = unique
		}
		ref.AuditAttributes["ocean_engine_product_id"] = ref.ID
		ref.AuditAttributes["unique_product_id"] = ref.ID
		ref.AuditAttributes["platform_object_id"] = ref.ID
		ref.AuditAttributes["product_id"] = fillingMetadataString(item.Metadata["product_id"])
	}
	if item.Kind == connector.PlatformObjectDouyinVideo {
		ref.AuditAttributes["ies_core_user_id"] = fillingMetadataString(item.Metadata["ies_core_user_id"])
		ref.AuditAttributes["video_id"] = fillingMetadataString(item.Metadata["video_id"])
	}
	ref.AuditAttributes["platform_object_id"] = ref.ID
	return ref
}

func fillingMetadataString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		return string(v)
	}
	return ""
}
func fillingMetadataContains(value any, expected string) bool {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if fillingMetadataString(item) == expected {
				return true
			}
		}
	case []string:
		for _, item := range v {
			if item == expected {
				return true
			}
		}
	case string:
		for _, item := range strings.Split(v, ",") {
			if strings.TrimSpace(item) == expected {
				return true
			}
		}
	}
	return false
}
func fillingLandingEligible(p delivery.OceanEngineProjectDraft, metadata map[string]any) bool {
	if p.OptimizationTargetReference == nil {
		return false
	}
	action := p.OptimizationTargetReference.ID
	if p.MarketingPurpose == "ecommerce" {
		if p.OptimizationTargetReference.SemanticKey == "in_app_order" {
			action = "20"
		}
		return fillingMetadataContains(metadata["ecommerce_external_actions"], action)
	}
	if p.MarketingPurpose == "lead_generation" && p.LeadCaptureMode != "custom_lead" {
		return fillingMetadataContains(metadata["multi_lead_external_actions"], action) || (action == "100" && metadata["multi_conversion_eligible"] == true)
	}
	return true
}
