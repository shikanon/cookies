package delivery

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/connector"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/oceanengineconstraints"
)

type FillingFinanceOptions struct {
	UnitWeight int `json:"unit_weight"`
}

type FillingFinanceEntry struct {
	Field        string   `json:"field"`
	AmountMinor  int64    `json:"amount_minor"`
	MinimumMinor int64    `json:"minimum_minor"`
	MaximumMinor int64    `json:"maximum_minor"`
	Basis        string   `json:"basis"`
	Sources      []string `json:"sources"`
}

type FillingFinanceResult struct {
	RuleVersion string                `json:"rule_version"`
	Entries     []FillingFinanceEntry `json:"entries"`
	Warnings    []string              `json:"warnings"`
	Suggestions []FillingSuggestion   `json:"-"`
}

func (r *FillingFinanceResult) add(field string, amount, low, high int64, basis string, sources []string) {
	r.Entries = append(r.Entries, FillingFinanceEntry{field, amount, low, high, basis, sources})
	raw, _ := json.Marshal(amount)
	r.Suggestions = append(r.Suggestions, FillingSuggestion{Field: field, Value: raw, Reason: basis, Sources: sources})
}

func (s Service) fillFinance(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, request FillingRequest, projectLocked bool) *FillingFinanceResult {
	result := &FillingFinanceResult{RuleVersion: "delivery-filling-finance/v1", Entries: []FillingFinanceEntry{}, Warnings: []string{}}
	p := request.Ocean.Project
	var unit *OceanEnginePromotionDraft
	for i := range request.Ocean.Promotions {
		if request.Ocean.Promotions[i].PromotionDraftID == request.TargetID {
			unit = &request.Ocean.Promotions[i]
			break
		}
	}
	if unit == nil {
		return result
	}
	defer result.validateUnitAmounts(unit, p.BudgetAndBidding)
	unitEditable := p.MarketingPurpose != "content_marketing" || p.DeliveryMode == "manual"
	budget := p.BudgetAndBidding
	if budget.Currency != "CNY" || budget.DailyBudgetMinor <= 0 || budget.DailyBudgetMinor > 1_000_000_000_000 || budget.BudgetMode == OceanEngineBudgetModeUnlimited {
		result.Warnings = append(result.Warnings, "请先设置有效的项目日预算；本次未填写预算或出价。")
		return result
	}
	minimumBudget := int64(30000)
	if p.MarketingPurpose == "content_marketing" && budget.ChargingMode == "CPC" {
		minimumBudget = 10000
	}
	if budget.DailyBudgetMinor < minimumBudget {
		result.Warnings = append(result.Warnings, fmt.Sprintf("当前分支项目日预算至少 %.2f 元，请先调整项目预算。", float64(minimumBudget)/100))
		return result
	}
	unitBudget := int64(0)
	if unit.BudgetAndBidding != nil {
		unitBudget = unit.BudgetAndBidding.DailyBudgetMinor
	}
	if unitEditable {
		amount, basis := allocateFillingBudget(request)
		if amount > 0 {
			unitBudget = amount
			result.add("daily_budget", amount, amount, amount, basis, []string{"当前草稿项目日预算", "弹窗确认的分配权重"})
		} else {
			result.Warnings = append(result.Warnings, basis)
		}
	} else {
		result.Warnings = append(result.Warnings, "当前模式由项目控制预算，不填写单元预算或单元出价。")
	}
	if projectLocked {
		result.Warnings = append(result.Warnings, "项目已绑定平台对象，保留项目出价。")
	}
	if p.DeepOptimizationMode == "conversion_roi" || p.DeepOptimizationMode == "net_roi" || budget.ROICoefficient != nil {
		result.Warnings = append(result.Warnings, "当前使用 ROI 约束，保留出价及 ROI 系数。")
		return result
	}
	projectBid := !projectLocked && !(p.MarketingPurpose == "content_marketing" && p.DeliveryMode == "manual") && fillingCostBid(budget)
	unitBid := unitEditable && unit.BudgetAndBidding != nil && fillingCostBid(*unit.BudgetAndBidding)
	if !projectBid && !unitBid {
		return result
	}
	if s.ConnectorSnapshots == nil {
		result.Warnings = append(result.Warnings, "真实历史数据未配置，保留现有出价。")
		return result
	}
	now := s.now()
	account := connector.AnonymizeRef(p.AccountReference.ID)
	snapshot, err := s.ConnectorSnapshots.Snapshot(ctx, connector.Query{OrganizationID: string(actor.OrganizationID), ProjectID: string(projectID), SourceRef: account, PredictionCutoff: now, WindowStart: now.AddDate(0, 0, -30), WindowEnd: now})
	if err != nil {
		result.Warnings = append(result.Warnings, "历史出价读取失败，已保留现有出价；可重试。")
		return result
	}
	for _, field := range []string{"project_bid", "bid"} {
		if field == "project_bid" && !projectBid || field == "bid" && !unitBid {
			continue
		}
		bidding, cap := budget, budget.DailyBudgetMinor
		if field == "bid" {
			bidding, cap = *unit.BudgetAndBidding, unitBudget
		}
		samples := comparableFillingBids(snapshot, actor, projectID, p, bidding, field, now)
		label := "项目出价"
		if field == "bid" {
			label = "单元出价"
		}
		if len(samples) < 3 {
			result.Warnings = append(result.Warnings, label+"缺少至少 3 个近期、条件一致且有真实消耗的独立对象记录，保留原值。")
			continue
		}
		sort.Slice(samples, func(i, j int) bool { return samples[i].amount < samples[j].amount })
		amount := samples[len(samples)/2].amount
		if len(samples)%2 == 0 {
			lower := samples[len(samples)/2-1].amount
			amount = lower + (amount-lower+1)/2
		}
		constraint, constraintErr := oceanengineconstraints.Resolve(bidding.ChargingMode, cap)
		if constraintErr != nil || oceanengineconstraints.ValidateBid(amount, constraint) != nil {
			result.Warnings = append(result.Warnings, label+"的历史中位数不符合当前出价限制，保留原值。")
			continue
		}
		sources := []string{}
		for _, sample := range samples {
			sources = append(sources, sample.evidence...)
		}
		basis := fmt.Sprintf("近 30 天同账户、产品、目标和投放模式的 %d 个独立对象，采用历史出价中位数；历史区间不代表效果保证。", len(samples))
		result.add(field, amount, samples[0].amount, samples[len(samples)-1].amount, basis, sources)
	}
	return result
}

// A budget change must not make an unchanged bid exceed its new limit.
func (r *FillingFinanceResult) validateUnitAmounts(unit *OceanEnginePromotionDraft, fallback OceanEngineBudgetAndBidding) {
	bidding := fallback
	if unit.BudgetAndBidding != nil {
		bidding = *unit.BudgetAndBidding
	} else {
		bidding.BidMinor = nil
	}
	changed := false
	for _, entry := range r.Entries {
		if entry.Field == "daily_budget" {
			bidding.DailyBudgetMinor = entry.AmountMinor
			changed = true
		}
		if entry.Field == "bid" {
			amount := entry.AmountMinor
			bidding.BidMinor = &amount
			changed = true
		}
	}
	if !changed || bidding.BidMinor == nil || *bidding.BidMinor == 0 {
		return
	}
	constraint, err := oceanengineconstraints.Resolve(bidding.ChargingMode, bidding.DailyBudgetMinor)
	if err == nil && oceanengineconstraints.ValidateBid(*bidding.BidMinor, constraint) == nil {
		return
	}
	entries := r.Entries[:0]
	for _, entry := range r.Entries {
		if entry.Field != "daily_budget" && entry.Field != "bid" {
			entries = append(entries, entry)
		}
	}
	r.Entries = entries
	suggestions := r.Suggestions[:0]
	for _, suggestion := range r.Suggestions {
		if suggestion.Field != "daily_budget" && suggestion.Field != "bid" {
			suggestions = append(suggestions, suggestion)
		}
	}
	r.Suggestions = suggestions
	r.Warnings = append(r.Warnings, "分配后的单元预算与出价不符合当前限制，已保留原单元预算和出价。")
}

func fillingCostBid(value OceanEngineBudgetAndBidding) bool {
	return value.ROICoefficient == nil && (value.BiddingStrategy == "stable_cost" || value.BiddingStrategy == "cost_cap")
}

func allocateFillingBudget(request FillingRequest) (int64, string) {
	p := request.Ocean.Project
	remaining := p.BudgetAndBidding.DailyBudgetMinor
	weight := int64(request.Finance.UnitWeight)
	denominator := weight
	reserved := int64(0)
	seen := map[string]bool{}
	for _, unit := range request.Ocean.Promotions {
		if unit.PromotionDraftID == "" || seen[unit.PromotionDraftID] {
			return 0, "单元标识无效，未分配预算。"
		}
		seen[unit.PromotionDraftID] = true
		b := unit.BudgetAndBidding
		if b != nil && (b.Currency != "CNY" || b.BudgetMode == OceanEngineBudgetModeUnlimited || b.DailyBudgetMinor < 0) {
			return 0, "单元包含不限预算或不同币种，请先明确预算，再进行分配。"
		}
		if unit.PromotionDraftID == request.TargetID {
			continue
		}
		if b != nil && b.DailyBudgetMinor > 0 {
			if b.DailyBudgetMinor >= remaining {
				return 0, "其他单元已设预算达到或超过项目日预算，保留当前单元预算。"
			}
			remaining -= b.DailyBudgetMinor
			reserved += b.DailyBudgetMinor
		} else {
			denominator++
		}
	}
	amount := remaining * weight / denominator
	if amount < 1 {
		return 0, "剩余预算不足，未分配单元预算。"
	}
	return amount, fmt.Sprintf("项目日预算 %.2f 元，保留其他单元 %.2f 元；剩余 %.2f 元按当前单元权重 %d、其他未设预算单元各 1 分配，仅填写当前单元。", float64(p.BudgetAndBidding.DailyBudgetMinor)/100, float64(reserved)/100, float64(remaining)/100, weight)
}

type fillingBidSample struct {
	amount   int64
	at       time.Time
	evidence []string
}

func comparableFillingBids(snapshot connector.CanonicalSnapshot, actor contract.ActorContext, projectID contract.ProjectID, p *OceanEngineProjectDraft, bidding OceanEngineBudgetAndBidding, field string, now time.Time) []fillingBidSample {
	product, target := p.MarketingProductReference, p.OptimizationTargetReference
	accountScope := "account:" + p.AccountReference.ID
	if product == nil || product.Namespace != "oceanengine" || product.Scope != accountScope || product.ID == "" || product.State != "resolved" || target == nil || target.Scope != accountScope || target.ID == "" || target.State != "resolved" {
		return nil
	}
	account := connector.AnonymizeRef(p.AccountReference.ID)
	valid := func(h connector.FactHeader) bool {
		return h.OrganizationID == string(actor.OrganizationID) && h.ProjectID == string(projectID) && h.SourceRef == account && h.SourceSystem == connector.SourceSystem && h.QualityStatus == connector.QualityAccept && h.EvidenceRef != "" && h.PayloadHash != "" && !h.ValidFrom.IsZero() && !h.AvailableAt.IsZero() && !h.AvailableAt.After(now)
	}
	objects := map[string]connector.ObjectSnapshot{}
	for _, object := range snapshot.Objects {
		if valid(object.FactHeader) && object.ObjectKind == "promotion" {
			if old, ok := objects[object.ObjectRef]; !ok || old.ValidFrom.Before(object.ValidFrom) {
				objects[object.ObjectRef] = object
			}
		}
	}
	samples := map[string]fillingBidSample{}
	for _, metric := range snapshot.Metrics {
		if !valid(metric.FactHeader) || metric.Currency != "CNY" || metric.AmountUnit != "fen" || metric.Metrics["spend"] <= 0 || metric.WindowEnd.Before(now.AddDate(0, 0, -7)) || metric.WindowEnd.After(now) || !metric.WindowStart.Before(metric.WindowEnd) || metric.DataThrough.Before(metric.WindowEnd) || len(metric.QualityIssues) > 0 {
			continue
		}
		object, ok := objects[metric.ObjectRef]
		if !ok || object.State["product_ref"] != connector.AnonymizeRef(product.ID) || object.ValidFrom.After(metric.WindowStart) || object.ValidTo != nil && object.ValidTo.Before(metric.WindowEnd) {
			continue
		}
		var chosen *connector.ConfigurationSnapshot
		changed := false
		for i := range snapshot.Configurations {
			config := &snapshot.Configurations[i]
			if config.ObjectRef != metric.ObjectRef || !valid(config.FactHeader) {
				continue
			}
			if config.ValidFrom.After(metric.WindowStart) && config.ValidFrom.Before(metric.WindowEnd) {
				changed = true
			}
			if config.ValidFrom.After(metric.WindowStart) || config.ValidTo != nil && config.ValidTo.Before(metric.WindowEnd) {
				continue
			}
			if chosen == nil || chosen.ValidFrom.Before(config.ValidFrom) {
				chosen = config
			}
		}
		if changed || chosen == nil {
			continue
		}
		h := connector.ReadBiddingHistory(chosen.Values)
		if h.Currency != "CNY" || !strings.EqualFold(h.ChargingMode, bidding.ChargingMode) || h.OptimizationTarget != target.ID || h.DeliveryMode != p.DeliveryMode || h.BiddingStrategy != bidding.BiddingStrategy || h.MarketingPurpose != p.MarketingPurpose || h.Carrier != p.Carrier {
			continue
		}
		amount, key := h.UnitBidMinor, metric.ObjectRef
		if field == "project_bid" {
			amount, key = h.ProjectBidMinor, object.ParentRef
		}
		if amount == nil || *amount <= 0 || *amount > math.MaxInt32 || key == "" {
			continue
		}
		if old, ok := samples[key]; !ok || old.at.Before(metric.WindowEnd) {
			samples[key] = fillingBidSample{*amount, metric.WindowEnd, []string{chosen.EvidenceRef, chosen.PayloadHash, metric.EvidenceRef, metric.PayloadHash}}
		}
	}
	result := []fillingBidSample{}
	for _, sample := range samples {
		result = append(result, sample)
	}
	return result
}
