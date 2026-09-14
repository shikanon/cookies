package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/integrations/oceanengine"
	"github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/connector"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/project"
	"github.com/shikanon/cookies/internal/systems/delivery"
	"github.com/shikanon/cookies/internal/systems/strategy"
)

// Wiring lives at the composition root; Delivery does not depend on Strategy's storage.
type deliveryFillingReader struct {
	projects     *project.Service
	assets       *assets.UploadService
	catalog      connector.MySQLRepository
	capabilities *connector.Synchronizer
	strategies   *strategy.Service
}

func (r *deliveryFillingReader) read(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, request delivery.FillingRequest) (delivery.FillingContext, error) {
	result := delivery.FillingContext{Choices: map[string][]delivery.FillingChoice{}, Sources: []string{"当前项目"}, Warnings: []string{"预算、出价和排期须有可核实的依据；缺少依据的字段请手工补充。", "文案与自然语言策略限制仍需人工核对。"}}
	projectContext, err := r.projects.RequireActiveContext(ctx, actor, projectID)
	if err != nil {
		return result, err
	}
	result.Project = projectContext
	business, err := r.projects.GetBusinessContext(ctx, actor, projectID)
	if err != nil {
		return result, err
	}
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
	facts := map[string]any{"project": business, "business_goal": detail.Runtime.Goal, "project_budget_cny": detail.Runtime.Budget, "timezone": detail.Runtime.Timezone}
	add := func(field, id, label string, value any, source string) {
		raw, _ := json.Marshal(value)
		result.Choices[field] = append(result.Choices[field], delivery.FillingChoice{ID: id, Label: label, Value: raw, Source: source})
	}
	if detail.Runtime.Currency == "CNY" && detail.Runtime.Budget > 0 && detail.Runtime.Budget < 1e10 {
		add("total_budget", "project:budget", "项目预算上限（须确认本计划分配）", int64(math.Round(detail.Runtime.Budget*100)), "项目总预算；须确认分配给本计划的额度")
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
		result.Sources = append(result.Sources, source)
		brief := pkg.Snapshot.Brief.Snapshot
		result.ProhibitedClaims = brief.Creative.ProhibitedClaims
		result.Warnings = append(result.Warnings, brief.Constraints...)
		result.Warnings = append(result.Warnings, pkg.Snapshot.Strategy.Constraints...)
		strategyProduct = &brief.Product
		strategySource = source
	} else {
		result.Warnings = append(result.Warnings, "未选择已批准策略；本次建议仅依据项目事实和有效目录。")
	}
	for _, product := range workbench.Products {
		ref := delivery.StableReference{Namespace: "cookies", ObjectKind: "product", Scope: "current_project", ID: string(product.ID), State: "resolved", DisplayNameSnapshot: product.Name, AuditAttributes: map[string]string{"ocean_engine_product_id": product.OceanEngineProductID}}
		if request.Search == "" || strings.Contains(strings.ToLower(product.Name+" "+string(product.ID)), strings.ToLower(request.Search)) {
			add("product", "cookies:product:"+string(product.ID), product.Name, ref, "当前项目产品")
		}
		// Names and selling points must belong to the selected product, not another candidate.
		if selected := request.Ocean.Project.MarketingProductReference; selected != nil && selected.Namespace == "cookies" && selected.ID == string(product.ID) {
			add("product_name", "cookies:name:"+string(product.ID), product.Name, product.Name, "已选项目产品")
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
		add("account", "account:"+account.ID, account.DisplayLabel, delivery.StableReference{Namespace: "oceanengine", ObjectKind: "advertiser_account", Scope: "project:" + string(projectID), ID: account.ID, State: "resolved", DisplayNameSnapshot: account.DisplayLabel}, "当前项目已验证账户")
		foundAccount = foundAccount || account.ID == accountID
	}
	if !foundAccount {
		return result, delivery.ErrInvalidRequest
	}
	for _, item := range []struct{ ID, Label string }{{"ecommerce", "电商"}, {"lead_generation", "销售线索"}, {"product_catalog", "商品"}, {"content_marketing", "内容营销"}} {
		add("marketing_purpose", item.ID, item.Label, item.ID, "平台配置支持的营销目的")
	}
	p := request.Ocean.Project
	content := p.MarketingPurpose == "content_marketing"
	native := content && p.Carrier == "douyin_account"
	carriers := []string{"orange_landing_page"}
	custom := p.LeadCaptureMode == "custom_lead" || (p.LeadCaptureMode == "" && (p.Carrier == "owned_landing_page" || p.Carrier == "im"))
	if content {
		carriers = append(carriers, "douyin_account")
	}
	if p.MarketingPurpose == "lead_generation" && !custom {
		carriers = append(carriers, "orange_landing_page_and_im")
	} else {
		carriers = append(carriers, "owned_landing_page")
		if !content {
			carriers = append(carriers, "im")
		}
	}
	for _, carrier := range carriers {
		add("carrier", carrier, carrier, carrier, "当前业务分支")
	}
	if !native {
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
			}
		}
	}
	if accountID == "" {
		result.Warnings = append(result.Warnings, "请先接受账户建议，再生成账户内产品、素材和优化目标建议。")
	} else {
		kinds := map[connector.PlatformObjectKind]string{connector.PlatformObjectMarketingProduct: "product", connector.PlatformObjectAuthorizedIdentity: "identity"}
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
		if p.Carrier != "" && p.MarketingPurpose != "" && !content && p.MarketingPurpose != "lead_generation" {
			kinds[connector.PlatformObjectOptimizationTarget] = "optimization_target"
		}
		for kind, field := range kinds {
			cursor := ""
			search := ""
			if field == "product" || field == "materials" {
				search = request.Search
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
					if field == "optimization_target" {
						// Match the regular selector's carrier qualification.
						required := "orange_landing_page"
						if p.Carrier == "owned_landing_page" {
							required = p.Carrier
						}
						if !fillingMetadataContains(item.Metadata["contexts"], required) {
							continue
						}
					}
					if field == "landing_page" && !fillingLandingEligible(*p, item.Metadata) {
						continue
					}
					add(field, "connector:"+item.ID, item.DisplayName, ref, "当前账户 Connector 目录")
					if field == "product" && p.MarketingProductReference != nil && p.MarketingProductReference.ID == ref.ID && p.MarketingProductReference.Scope == ref.Scope {
						add("product_name", "connector:name:"+item.ID, item.DisplayName, item.DisplayName, "已选 Connector 产品")
					}
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
		if p.Carrier != "" && (content || p.MarketingPurpose == "lead_generation") {
			if r.capabilities == nil {
				return result, delivery.ErrFillingUnavailable
			}
			branch := oceanengine.OptimizationTargetContext{CampaignType: 1, LandingType: 1, CDPMarketingGoal: 1, MicroPromotionType: 2, AssetType: 2}
			switch {
			case content:
				branch.LandingType = 7
				branch.DeliveryProduct = 5001
			case p.Carrier == "orange_landing_page_and_im":
				branch.MultiAssetTypes = []int{2, 1002}
			case p.Carrier == "orange_landing_page" && !custom:
				branch.MultiAssetTypes = []int{2}
			case p.Carrier == "owned_landing_page":
				branch.AssetType = 3
				branch.NeedAssets = true
			case p.Carrier == "im":
				branch.AssetType = 1002
			}
			snapshot, readErr := r.capabilities.ReadOptimizationTargetCapabilities(ctx, connector.OptimizationTargetCapabilityRequest{OrganizationID: string(actor.OrganizationID), ProjectID: string(projectID), AccountRef: accountID, Context: branch})
			if readErr != nil {
				return result, delivery.ErrFillingUnavailable
			}
			for _, option := range snapshot.Options {
				ref := delivery.StableReference{Namespace: "oceanengine_capability", ObjectKind: "optimization_target", Scope: "account:" + accountID, ID: option.ExternalAction, SemanticKey: option.SemanticKey, State: "resolved", DisplayNameSnapshot: option.DisplayName, AuditAttributes: map[string]string{"selection_kind": "account_capability", "external_action": option.ExternalAction, "optimization_event_type": option.OptimizationEventType, "capability_snapshot_id": snapshot.SnapshotID, "capability_context_hash": snapshot.ContextHash, "capability_observed_at": snapshot.ObservedAt.Format(time.RFC3339Nano)}}
				add("optimization_target", "capability:"+option.ExternalAction, option.DisplayName, ref, "当前账户和业务分支能力快照 "+snapshot.SnapshotID)
			}
		}
	}
	if strategyProduct != nil && p.MarketingProductReference != nil {
		for _, choice := range result.Choices["product"] {
			var ref delivery.StableReference
			if json.Unmarshal(choice.Value, &ref) == nil && ref.Namespace == p.MarketingProductReference.Namespace && ref.Scope == p.MarketingProductReference.Scope && ref.ID == p.MarketingProductReference.ID && choice.Label == strategyProduct.Name {
				add("product_name", "strategy:product_name", strategyProduct.Name, strategyProduct.Name, strategySource)
				for i, point := range strategyProduct.SellingPoints {
					add("selling_points", fmt.Sprintf("strategy:point:%d", i), point, point, strategySource)
				}
			}
		}
	}
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
