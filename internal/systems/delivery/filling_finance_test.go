package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/connector"
	"github.com/shikanon/cookies/internal/platform/contract"
)

type fillingHistoryReader struct {
	snapshot connector.CanonicalSnapshot
	err      error
	query    connector.Query
}

func (r *fillingHistoryReader) Snapshot(_ context.Context, q connector.Query) (connector.CanonicalSnapshot, error) {
	r.query = q
	return r.snapshot, r.err
}

func TestFillingFinanceOptInAndAllocation(t *testing.T) {
	s, actor, request, _ := fillingFixture(t)
	request.Ocean.Project.BudgetAndBidding = OceanEngineBudgetAndBidding{Currency: "CNY", BudgetMode: "daily", DailyBudgetMinor: 100001, BiddingStrategy: "stable_cost", ChargingMode: "CPC"}
	unit := request.Ocean.Promotions[0]
	unit.PromotionDraftID = "reserved"
	unit.BudgetAndBidding = &OceanEngineBudgetAndBidding{Currency: "CNY", DailyBudgetMinor: 20000}
	request.Ocean.Promotions = append(request.Ocean.Promotions, unit)
	unit.PromotionDraftID = "unassigned"
	unit.BudgetAndBidding = nil
	request.Ocean.Promotions = append(request.Ocean.Promotions, unit)
	before, _ := json.Marshal(request.Ocean)
	result, err := s.SuggestFilling(context.Background(), actor, "project_a", request)
	if err != nil || result.Finance != nil {
		t.Fatalf("finance must be opt-in: %+v %v", result, err)
	}
	request.Finance = &FillingFinanceOptions{UnitWeight: 2}
	result, err = s.SuggestFilling(context.Background(), actor, "project_a", request)
	if err != nil || result.Finance == nil || len(result.Finance.Entries) != 1 || result.Finance.Entries[0].AmountMinor != 53334 {
		t.Fatalf("allocation: %+v %v", result.Finance, err)
	}
	after, _ := json.Marshal(request.Ocean)
	if string(before) != string(after) {
		t.Fatal("mutated caller configuration")
	}
	request.Ocean.Promotions[1].BudgetAndBidding.DailyBudgetMinor = 100001
	if amount, _ := allocateFillingBudget(request); amount != 0 {
		t.Fatal("oversubscribed project allocated budget")
	}
	request.Ocean.Promotions[1].BudgetAndBidding.BudgetMode = "unlimited"
	if amount, _ := allocateFillingBudget(request); amount != 0 {
		t.Fatal("unlimited sibling allocated budget")
	}
	request.Finance.UnitWeight = 0
	if _, err = s.SuggestFilling(context.Background(), actor, "project_a", request); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal(err)
	}
}

func TestFillingFinanceRejectsModelAmounts(t *testing.T) {
	s, actor, request, text := fillingFixture(t)
	request.Finance = &FillingFinanceOptions{UnitWeight: 1}
	text.output = `{"suggestions":[{"field":"project_bid","candidate_ids":[],"text":"100","reason":"guess"}]}`
	if _, err := s.SuggestFilling(context.Background(), actor, "project_a", request); !errors.Is(err, ErrFillingInvalid) {
		t.Fatal(err)
	}
}

func TestFillingHistoricalBidsRequireComparableRealEvidence(t *testing.T) {
	s, actor, request, _ := fillingFixture(t)
	now := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	p := request.Ocean.Project
	p.MarketingPurpose, p.Carrier, p.DeliveryMode = "ecommerce", "orange_landing_page", "manual"
	p.MarketingProductReference = &StableReference{Namespace: "oceanengine", ID: "product", Scope: "account:" + p.AccountReference.ID, State: "resolved"}
	p.OptimizationTargetReference = &StableReference{ID: "19", Scope: "account:" + p.AccountReference.ID, State: "resolved"}
	p.BudgetAndBidding = OceanEngineBudgetAndBidding{Currency: "CNY", BudgetMode: "daily", DailyBudgetMinor: 100000, BiddingStrategy: "stable_cost", ChargingMode: "CPC"}
	request.Ocean.Promotions[0].BudgetAndBidding = &OceanEngineBudgetAndBidding{Currency: "CNY", BiddingStrategy: "stable_cost", ChargingMode: "CPC"}
	request.Finance = &FillingFinanceOptions{UnitWeight: 1}
	snapshot := connector.CanonicalSnapshot{}
	for i, id := range []string{"a", "b", "c"} {
		h := connector.FactHeader{OrganizationID: string(actor.OrganizationID), ProjectID: "project_a", SourceRef: connector.AnonymizeRef(p.AccountReference.ID), SourceSystem: connector.SourceSystem, QualityStatus: connector.QualityAccept, EvidenceRef: id, PayloadHash: "hash-" + id, AvailableAt: now.Add(-time.Hour), ValidFrom: now.Add(-72 * time.Hour), DataThrough: now.Add(-24 * time.Hour)}
		snapshot.Objects = append(snapshot.Objects, connector.ObjectSnapshot{FactHeader: h, ObjectKind: "promotion", ObjectRef: id, ParentRef: "parent-" + id, State: map[string]any{"product_ref": connector.AnonymizeRef("product")}})
		snapshot.Configurations = append(snapshot.Configurations, connector.ConfigurationSnapshot{FactHeader: h, ObjectRef: id, Values: map[string]any{"project_bid_minor": 100 + i*10, "unit_bid_minor": 50 + i*10, "currency": "CNY", "charging_mode": "CPC", "external_action": "19", "delivery_mode": "manual", "bidding_strategy": "stable_cost", "marketing_purpose": p.MarketingPurpose, "carrier": p.Carrier}})
		snapshot.Metrics = append(snapshot.Metrics, connector.MetricWindow{FactHeader: h, ObjectRef: id, WindowStart: now.Add(-48 * time.Hour), WindowEnd: now.Add(-24 * time.Hour), Currency: "CNY", AmountUnit: "fen", Metrics: map[string]int64{"spend": 1000}})
	}
	reader := &fillingHistoryReader{snapshot: snapshot}
	s.ConnectorSnapshots = reader
	result := s.fillFinance(context.Background(), actor, "project_a", request, false)
	if len(result.Entries) != 3 || result.Entries[1].AmountMinor != 110 || result.Entries[2].AmountMinor != 60 {
		t.Fatalf("historical median: %+v", result)
	}
	if reader.query.SourceRef != connector.AnonymizeRef(p.AccountReference.ID) || reader.query.ProjectID != "project_a" {
		t.Fatal("unscoped history query")
	}
	additional := snapshot.Objects[2]
	additional.ObjectRef, additional.ParentRef = "d", "parent-d"
	additionalConfig := snapshot.Configurations[2]
	additionalConfig.ObjectRef = "d"
	additionalMetric := snapshot.Metrics[2]
	additionalMetric.ObjectRef = "d"
	reader.snapshot.Objects = append(append([]connector.ObjectSnapshot{}, snapshot.Objects...), additional)
	reader.snapshot.Configurations = append(append([]connector.ConfigurationSnapshot{}, snapshot.Configurations...), additionalConfig)
	reader.snapshot.Metrics = append(append([]connector.MetricWindow{}, snapshot.Metrics...), additionalMetric)
	even := s.fillFinance(context.Background(), actor, "project_a", request, false)
	if even.Entries[1].AmountMinor != 115 || even.Entries[2].AmountMinor != 65 {
		t.Fatalf("even sample median: %+v", even)
	}
	reader.snapshot = snapshot
	locked := s.fillFinance(context.Background(), actor, "project_a", request, true)
	for _, entry := range locked.Entries {
		if entry.Field == "project_bid" {
			t.Fatal("edited locked project")
		}
	}
	for _, mutation := range []func(*connector.CanonicalSnapshot){
		func(v *connector.CanonicalSnapshot) { v.Metrics[0].SourceSystem = "simulation" },
		func(v *connector.CanonicalSnapshot) { v.Metrics[0].QualityStatus = connector.QualityQuarantine },
		func(v *connector.CanonicalSnapshot) { v.Metrics[0].SourceRef = "other-account" },
		func(v *connector.CanonicalSnapshot) { v.Metrics[0].WindowEnd = now.Add(-8 * 24 * time.Hour) },
		func(v *connector.CanonicalSnapshot) { v.Metrics[0].DataThrough = now.Add(-72 * time.Hour) },
		func(v *connector.CanonicalSnapshot) { v.Configurations[0].Values["external_action"] = "20" },
		func(v *connector.CanonicalSnapshot) {
			v.Configurations[0].Values["bidding_strategy"] = "maximum_conversion"
		},
		func(v *connector.CanonicalSnapshot) { v.Configurations[0].Values["currency"] = "USD" },
		func(v *connector.CanonicalSnapshot) { v.Objects[0].State["product_ref"] = "other-product" },
		func(v *connector.CanonicalSnapshot) { v.Configurations[0].ValidFrom = now.Add(-36 * time.Hour) },
	} {
		raw, _ := json.Marshal(snapshot)
		var changed connector.CanonicalSnapshot
		_ = json.Unmarshal(raw, &changed)
		mutation(&changed)
		if samples := comparableFillingBids(changed, actor, contract.ProjectID("project_a"), p, p.BudgetAndBidding, "bid", now); len(samples) != 2 {
			t.Fatalf("invalid sample included: %+v", samples)
		}
	}
	reader.err = errors.New("unavailable")
	failed := s.fillFinance(context.Background(), actor, "project_a", request, false)
	if len(failed.Entries) != 1 || !strings.Contains(strings.Join(failed.Warnings, ""), "读取失败") {
		t.Fatalf("read error: %+v", failed)
	}
	reader.err = nil
	p.MarketingPurpose, p.DeliveryMode = "content_marketing", "ubmax"
	mode := s.fillFinance(context.Background(), actor, "project_a", request, false)
	for _, entry := range mode.Entries {
		if entry.Field == "daily_budget" || entry.Field == "bid" {
			t.Fatal("hidden unit financial fields modified")
		}
	}
	if reflect.DeepEqual(result, mode) {
		t.Fatal("mode did not change eligibility")
	}
}

func TestFillingBudgetCannotInvalidateExistingBid(t *testing.T) {
	bid := int64(50000)
	unit := &OceanEnginePromotionDraft{BudgetAndBidding: &OceanEngineBudgetAndBidding{Currency: "CNY", ChargingMode: "OCPM", DailyBudgetMinor: 60000, BidMinor: &bid}}
	result := &FillingFinanceResult{}
	result.add("daily_budget", 30000, 30000, 30000, "分配", []string{"项目预算"})
	result.add("project_bid", 100, 100, 100, "历史", []string{"历史"})
	result.validateUnitAmounts(unit, OceanEngineBudgetAndBidding{})
	if len(result.Entries) != 1 || result.Entries[0].Field != "project_bid" || len(result.Suggestions) != 1 || len(result.Warnings) != 1 {
		t.Fatalf("conflicting allocation applied: %+v", result)
	}
	if unit.BudgetAndBidding.DailyBudgetMinor != 60000 || *unit.BudgetAndBidding.BidMinor != 50000 {
		t.Fatal("mutated existing amounts")
	}
}
