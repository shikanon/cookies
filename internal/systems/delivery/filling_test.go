package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/provider"
)

type fillingTextStub struct {
	output   string
	code     string
	calls    int
	err      error
	messages []provider.TextMessage
}

func (s *fillingTextStub) GenerateText(_ context.Context, request provider.TextAdapterRequest) (provider.SynchronousResult, error) {
	s.calls++
	s.messages = request.Messages
	return provider.SynchronousResult{ProviderCode: s.code, ModelVersion: "test-model", Text: s.output}, s.err
}

func TestFillingPreservesModelTimeout(t *testing.T) {
	s, actor, request, text := fillingFixture(t)
	text.err = context.DeadlineExceeded
	_, err := s.SuggestFilling(context.Background(), actor, "project_a", request)
	if !errors.Is(err, ErrFillingUnavailable) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("model timeout lost: %v", err)
	}
}

func fillingFixture(t *testing.T) (Service, contract.ActorContext, FillingRequest, *fillingTextStub) {
	t.Helper()
	service, actor := newTestService()
	text := &fillingTextStub{code: "ark_text", output: `{"suggestions":[{"field":"category","candidate_ids":["category1"],"text":"","reason":"匹配当前项目产品"}]}`}
	service.FillingText = &provider.Service{TextAdapter: text}
	service.FillingModelAlias = "cookies.text.standard"
	brand := contract.BrandID("brand_a")
	service.LoadFillingContext = func(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, request FillingRequest) (FillingContext, error) {
		if request.Strategy != nil && request.Strategy.ContentHash != "approved" {
			return FillingContext{}, ErrInvalidRequest
		}
		return FillingContext{Project: contract.ProjectContext{OrganizationID: actor.OrganizationID, ProjectID: "project_a", BrandID: &brand, ProductIDs: []contract.ProductID{}, ProjectContextVersion: 1}, Facts: json.RawMessage(`{"project":"fact"}`), Strategy: request.Strategy, Sources: []string{"项目"}, ProhibitedClaims: []string{"保证收益"}, Choices: map[string][]FillingChoice{"category": {
			{ID: "connector:1", Label: "产品", Value: json.RawMessage(`{"namespace":"oceanengine","object_kind":"category","scope":"account:a","id":"same","version":"7","content_hash":"hash","state":"resolved","audit_attributes":{"connector_platform_object_id":"1"}}`), Source: "目录"},
			{ID: "cookies:1", Label: "产品", Value: json.RawMessage(`{"namespace":"cookies","object_kind":"category","scope":"current_project","id":"same","state":"resolved"}`), Source: "项目"},
		}}}, nil
	}
	ocean := *testPlatformCreateRequest().PlatformConfiguration.Payload.OceanEngine
	ocean.Promotions[0].Settings.CategoryReference = nil
	ocean.Promotions[0].CopyItems = nil
	return service, actor, FillingRequest{Page: "configuration", TargetID: ocean.Promotions[0].PromotionDraftID, Fields: []string{"category"}, Current: map[string]json.RawMessage{}, Ocean: ocean}, text
}

func TestFillingResolvesOnlyAuthoritativeCandidateIdentity(t *testing.T) {
	s, actor, request, text := fillingFixture(t)
	result, err := s.SuggestFilling(context.Background(), actor, "project_a", request)
	if err != nil {
		t.Fatal(err)
	}
	var ref StableReference
	if len(result.Suggestions) != 1 {
		t.Fatal(result)
	}
	_ = json.Unmarshal(result.Suggestions[0].Value, &ref)
	if ref.Namespace != "oceanengine" || ref.Version != "7" || ref.ContentHash != "hash" || ref.AuditAttributes["connector_platform_object_id"] != "1" || text.calls != 1 {
		t.Fatalf("reference lost: %+v", ref)
	}
}

func TestFillingRejectsInvalidOutputAndFakeProvider(t *testing.T) {
	for _, output := range []string{
		`{}`, `{"suggestions":null}`, `{"suggestions":[]} {}`,
		`{"suggestions":[{"field":"arbitrary.path","candidate_ids":[],"text":"x","reason":"x"}]}`,
		`{"suggestions":[{"field":"category","candidate_ids":["missing"],"text":"","reason":"x"}]}`,
		`{"suggestions":[{"field":"category","candidate_ids":["category1"],"text":"invented","reason":"x"}]}`,
	} {
		t.Run(output, func(t *testing.T) {
			s, a, r, text := fillingFixture(t)
			text.output = output
			if _, err := s.SuggestFilling(context.Background(), a, "project_a", r); !errors.Is(err, ErrFillingInvalid) {
				t.Fatalf("got %v", err)
			}
		})
	}
	s, a, r, text := fillingFixture(t)
	text.code = "fake"
	if _, err := s.SuggestFilling(context.Background(), a, "project_a", r); !errors.Is(err, ErrFillingUnavailable) {
		t.Fatal(err)
	}
}

func TestFillingRejectsChangedCatalogAndUnapprovedStrategy(t *testing.T) {
	s, a, r, _ := fillingFixture(t)
	load := s.LoadFillingContext
	reads := 0
	s.LoadFillingContext = func(ctx context.Context, actor contract.ActorContext, p contract.ProjectID, request FillingRequest) (FillingContext, error) {
		v, e := load(ctx, actor, p, request)
		reads++
		if reads > 1 {
			v.Choices["category"] = nil
		}
		return v, e
	}
	if _, err := s.SuggestFilling(context.Background(), a, "project_a", r); !errors.Is(err, ErrFillingInvalid) {
		t.Fatal(err)
	}
	s, a, r, _ = fillingFixture(t)
	r.Strategy = &FillingStrategy{PackageID: "p", Version: 1, ContentHash: "stale"}
	if _, err := s.SuggestFilling(context.Background(), a, "project_a", r); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal(err)
	}
	r.Strategy.ContentHash = "approved"
	result, err := s.SuggestFilling(context.Background(), a, "project_a", r)
	if err != nil || result.Strategy == nil || result.Strategy.ContentHash != "approved" {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestFillingPreservesEditingAndAuthorizationBoundaries(t *testing.T) {
	s, a, r, text := fillingFixture(t)
	r.Fields = []string{"materials"}
	r.Page = "plan"
	if _, err := s.SuggestFilling(context.Background(), a, "project_a", r); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal(err)
	}
	r.Page = "configuration"
	r.Fields = []string{"copy"}
	text.output = `{"suggestions":[{"field":"copy","candidate_ids":[],"text":"保证收益","reason":"x"}]}`
	if _, err := s.SuggestFilling(context.Background(), a, "project_a", r); !errors.Is(err, ErrFillingInvalid) {
		t.Fatal(err)
	}
	a.Scopes = nil
	text.calls = 0
	if _, err := s.SuggestFilling(context.Background(), a, "project_a", r); err == nil || text.calls != 0 {
		t.Fatalf("authorization failed: %v", err)
	}
}

func TestFillingDoesNotTreatProjectBudgetAsLongTermDailyBudget(t *testing.T) {
	s, a, r, text := fillingFixture(t)
	r.Ocean.Project.Schedule.Mode = "long_term"
	r.Fields = []string{"total_budget"}
	if _, err := s.SuggestFilling(context.Background(), a, "project_a", r); !errors.Is(err, ErrInvalidRequest) || text.calls != 0 {
		t.Fatalf("total budget offered as daily amount: %v", err)
	}
}

func TestFillingMaterialBranchesAndLimits(t *testing.T) {
	p := &OceanEngineProjectDraft{MarketingPurpose: "content_marketing", Carrier: "douyin_account"}
	video := json.RawMessage(`{"namespace":"oceanengine","object_kind":"douyin_video","id":"1","state":"resolved"}`)
	image := json.RawMessage(`{"namespace":"oceanengine","object_kind":"image_material","id":"2","state":"resolved"}`)
	if !validFillingMaterials([]json.RawMessage{video}, p) || validFillingMaterials([]json.RawMessage{image}, p) || validFillingMaterials([]json.RawMessage{video, video}, p) {
		t.Fatal("native video boundary lost")
	}
	p.MarketingPurpose = "ecommerce"
	p.Carrier = "owned_landing_page"
	if validFillingMaterials([]json.RawMessage{video}, p) || !validFillingMaterials([]json.RawMessage{image}, p) {
		t.Fatal("regular material branch lost")
	}
}

func TestFillingRejectsBusinessDecisionsAndSkipsFilledFields(t *testing.T) {
	for _, field := range []string{"account", "daily_budget", "bid", "schedule", "product", "optimization_target"} {
		s, a, r, text := fillingFixture(t)
		r.Fields = []string{field}
		if _, err := s.SuggestFilling(context.Background(), a, "project_a", r); !errors.Is(err, ErrInvalidRequest) || text.calls != 0 {
			t.Fatalf("business field %s: %v", field, err)
		}
	}
	s, a, r, text := fillingFixture(t)
	r.Fields = []string{"name"}
	r.Current = map[string]json.RawMessage{"name": json.RawMessage(`""`)}
	r.Ocean.Promotions[0].PromotionName = "operator value"
	result, err := s.SuggestFilling(context.Background(), a, "project_a", r)
	if err != nil || len(result.Suggestions) != 0 || text.calls != 0 {
		t.Fatalf("filled field was suggested: %+v %v", result, err)
	}
}

func TestFillingRequiresProductEvidenceForTextAndSellingPoints(t *testing.T) {
	s, a, r, text := fillingFixture(t)
	r.Fields = []string{"copy"}
	original := s.LoadFillingContext
	s.LoadFillingContext = func(ctx context.Context, a contract.ActorContext, p contract.ProjectID, r FillingRequest) (FillingContext, error) {
		data, err := original(ctx, a, p, r)
		data.Sources = nil
		return data, err
	}
	text.output = `{"suggestions":[{"field":"copy","candidate_ids":[],"text":"invented","reason":"x"}]}`
	result, err := s.SuggestFilling(context.Background(), a, "project_a", r)
	if err != nil || len(result.Suggestions) != 0 || len(result.Warnings) == 0 {
		t.Fatalf("ungrounded copy: %+v %v", result, err)
	}
	s.LoadFillingContext = original
	r.Fields = []string{"selling_points"}
	r.Ocean.Promotions[0].ProductSellingPoints = nil
	text.output = `{"suggestions":[{"field":"selling_points","candidate_ids":[],"text":"invented","reason":"x"}]}`
	if _, err := s.SuggestFilling(context.Background(), a, "project_a", r); !errors.Is(err, ErrFillingInvalid) {
		t.Fatalf("missing description accepted: %v", err)
	}
}

func TestFillingContentCountsInstructionsAndImages(t *testing.T) {
	s, a, r, text := fillingFixture(t)
	r.Fields = []string{"copy", "selling_points", "search_keywords"}
	r.CopyCount = 2
	r.Instructions = "只介绍选购入口"
	r.Ocean.Promotions[0].CopyItems = []OceanEngineCopyItem{{Text: "旧文案"}}
	loads := 0
	s.LoadFillingImages = func(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, _ FillingRequest, data *FillingContext) ([]provider.TextMessage, error) {
		loads++
		return []provider.TextMessage{{Role: provider.TextRoleUser, Content: "素材首帧", Images: []provider.TextImage{{MIMEType: "image/jpeg", Data: []byte("image")}}}}, nil
	}
	text.output = `{"suggestions":[{"field":"copy","candidate_ids":[],"text":"浏览产品\n查看详情","reason":"资料"},{"field":"selling_points","candidate_ids":[],"text":"便捷选购","reason":"说明"},{"field":"search_keywords","candidate_ids":[],"text":"淘宝\n商品选购","reason":"说明"}]}`
	result, err := s.SuggestFilling(context.Background(), a, "project_a", r)
	if err != nil || len(result.Suggestions) != 3 || loads != 1 || len(text.messages) != 3 || len(text.messages[2].Images) != 1 {
		t.Fatalf("content/image filling: %+v %v", result, err)
	}
	r.CopyCount = 1
	if _, err = s.SuggestFilling(context.Background(), a, "project_a", r); !errors.Is(err, ErrFillingInvalid) {
		t.Fatalf("excess copy accepted: %v", err)
	}
	r.MaterialCount = 11
	if _, err = s.SuggestFilling(context.Background(), a, "project_a", r); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("excess material count accepted: %v", err)
	}
}

func TestFillingTargetingAndBrandResolveCatalogValues(t *testing.T) {
	s, a, r, text := fillingFixture(t)
	r.Fields = []string{"regions", "source_label"}
	r.Instructions = "地域限定上海市"
	r.Ocean.Promotions[0].Settings.SourceLabel = "活动名"
	load := s.LoadFillingContext
	s.LoadFillingContext = func(ctx context.Context, a contract.ActorContext, p contract.ProjectID, r FillingRequest) (FillingContext, error) {
		data, err := load(ctx, a, p, r)
		data.Choices["regions"] = []FillingChoice{{ID: "region:all", Value: json.RawMessage(`"all"`)}, {ID: "region:shanghai", Value: json.RawMessage(`"上海市"`)}}
		data.Choices["source_label"] = []FillingChoice{{ID: "brand", Value: json.RawMessage(`"淘宝"`), Source: "brand_name"}}
		return data, err
	}
	text.output = `{"suggestions":[{"field":"regions","candidate_ids":["regions2"],"text":"","reason":"用户要求"},{"field":"source_label","candidate_ids":["source_label1"],"text":"","reason":"brand_name"}]}`
	result, err := s.SuggestFilling(context.Background(), a, "project_a", r)
	if err != nil || len(result.Suggestions) != 2 || string(result.Suggestions[1].Value) != `"淘宝"` {
		t.Fatalf("brand/targeting: %+v %v", result, err)
	}
	text.output = `{"suggestions":[{"field":"regions","candidate_ids":["regions1","regions2"],"text":"","reason":"conflict"}]}`
	if _, err = s.SuggestFilling(context.Background(), a, "project_a", r); !errors.Is(err, ErrFillingInvalid) {
		t.Fatalf("conflicting regions accepted: %v", err)
	}
	text.output = `{"suggestions":[{"field":"source_label","candidate_ids":[],"text":"活动名","reason":"x"}]}`
	if _, err = s.SuggestFilling(context.Background(), a, "project_a", r); !errors.Is(err, ErrFillingInvalid) {
		t.Fatalf("invented brand accepted: %v", err)
	}
}

func TestFillingShortlistResolvesFullCatalogAndRejectsInventedIDs(t *testing.T) {
	s, a, r, text := fillingFixture(t)
	s.NewID = nil
	data, err := s.LoadFillingContext(context.Background(), a, "project_a", r)
	if err != nil {
		t.Fatal(err)
	}
	data.Choices["materials"] = []FillingChoice{{ID: "later-page", Label: "later page candidate", Value: json.RawMessage(`"identity"`)}, {ID: "first-page", Value: json.RawMessage(`"other"`)}}
	text.output = `{"materials":["later-page"],"product_images":[]}`
	if err = s.shortlistFillingImages(context.Background(), a, r, &data); err != nil || len(data.Choices["materials"]) != 1 || data.Choices["materials"][0].ID != "later-page" {
		t.Fatalf("shortlist: %+v %v", data, err)
	}
	text.output = `{"materials":["invented"],"product_images":[]}`
	if err = s.shortlistFillingImages(context.Background(), a, r, &data); !errors.Is(err, ErrFillingInvalid) {
		t.Fatalf("invented ID: %v", err)
	}
}

func TestFillingAliasesDoNotConfuseChangedCatalogOrder(t *testing.T) {
	s, a, r, _ := fillingFixture(t)
	load := s.LoadFillingContext
	reads := 0
	s.LoadFillingContext = func(ctx context.Context, a contract.ActorContext, p contract.ProjectID, r FillingRequest) (FillingContext, error) {
		data, err := load(ctx, a, p, r)
		reads++
		if reads > 1 {
			choices := data.Choices["category"]
			data.Choices["category"] = []FillingChoice{choices[1], choices[0]}
		}
		return data, err
	}
	result, err := s.SuggestFilling(context.Background(), a, "project_a", r)
	if err != nil || len(result.Suggestions) != 1 {
		t.Fatalf("reordered catalog: %+v %v", result, err)
	}
	var ref StableReference
	if err := json.Unmarshal(result.Suggestions[0].Value, &ref); err != nil {
		t.Fatal(err)
	}
	if ref.Namespace != "oceanengine" || ref.Version != "7" {
		t.Fatalf("identity changed: %+v", ref)
	}
}

func TestFillingSearchKeywordsAcceptFormSeparatorsWithPerKeywordLimits(t *testing.T) {
	s, a, r, text := fillingFixture(t)
	r.Fields = []string{"search_keywords"}
	text.output = `{"suggestions":[{"field":"search_keywords","candidate_ids":[],"text":"淘宝限时福利活动, 平台限时好物选购，淘宝平台活动入口、今日好物浏览;平台产品信息；线上选购入口","reason":"产品资料"}]}`
	result, err := s.SuggestFilling(context.Background(), a, "project_a", r)
	if err != nil || len(result.Suggestions) != 1 {
		t.Fatalf("keyword separators: %+v %v", result, err)
	}
	var words []string
	if err := json.Unmarshal(result.Suggestions[0].Value, &words); err != nil {
		t.Fatal(err)
	}
	if len(words) != 6 || words[1] != "平台限时好物选购" {
		t.Fatalf("incorrect split: %+v", words)
	}
	text.output = `{"suggestions":[{"field":"search_keywords","candidate_ids":[],"text":"1234567890123456789012345678901","reason":"资料"}]}`
	if _, err = s.SuggestFilling(context.Background(), a, "project_a", r); !errors.Is(err, ErrFillingInvalid) {
		t.Fatalf("overlong single keyword: %v", err)
	}
}

func TestFillingDoesNotInventTargetingWithoutInstructionsOrStrategy(t *testing.T) {
	s, a, r, text := fillingFixture(t)
	r.Fields = []string{"regions", "gender", "smart_expansion", "search_expansion"}
	result, err := s.SuggestFilling(context.Background(), a, "project_a", r)
	if err != nil || text.calls != 0 || len(result.Suggestions) != 0 {
		t.Fatalf("ungrounded targeting: %+v %v", result, err)
	}
}

func TestFillingRejectsCommercialClaimsFromUnconfirmedVisuals(t *testing.T) {
	s, a, r, text := fillingFixture(t)
	r.Fields = []string{"copy"}
	for _, copy := range []string{"奶茶低至1.9元起", "新用户专享福利", "官方活动靠谱有保障"} {
		encoded, _ := json.Marshal(map[string]any{"suggestions": []map[string]any{{"field": "copy", "text": copy, "candidate_ids": []string{}, "reason": "封面"}}})
		text.output = string(encoded)
		result, err := s.SuggestFilling(context.Background(), a, "project_a", r)
		if err != nil || len(result.Suggestions) != 0 || len(result.Warnings) == 0 {
			t.Fatalf("unconfirmed commercial copy: %s %+v %v", copy, result, err)
		}
	}
	r.Instructions = "已确认：奶茶低至1.9元起"
	text.output = `{"suggestions":[{"field":"copy","candidate_ids":[],"text":"奶茶低至1.9元起，来看看","reason":"用户说明"}]}`
	if _, err := s.SuggestFilling(context.Background(), a, "project_a", r); err != nil {
		t.Fatal(err)
	}
}

func TestFillingKeepsGroundedCopyWhenAnotherLineHasUnconfirmedClaim(t *testing.T) {
	s, a, r, text := fillingFixture(t)
	r.Fields = []string{"copy"}
	r.CopyCount = 2
	text.output = `{"suggestions":[{"field":"copy","candidate_ids":[],"text":"浏览淘宝好物，看看活动详情\n官方保障，下单即可领35元","reason":"商品与封面"}]}`
	result, err := s.SuggestFilling(context.Background(), a, "project_a", r)
	if err != nil || len(result.Suggestions) != 1 || string(result.Suggestions[0].Value) != `"浏览淘宝好物，看看活动详情"` || len(result.Warnings) == 0 {
		t.Fatalf("grounded copy: %+v %v", result, err)
	}
}

func TestFillingRejectsDuplicateFieldAfterDroppingUnconfirmedText(t *testing.T) {
	s, a, r, text := fillingFixture(t)
	r.Fields = []string{"copy"}
	text.output = `{"suggestions":[{"field":"copy","candidate_ids":[],"text":"官方保障","reason":"x"},{"field":"copy","candidate_ids":[],"text":"查看详情","reason":"x"}]}`
	if _, err := s.SuggestFilling(context.Background(), a, "project_a", r); !errors.Is(err, ErrFillingInvalid) {
		t.Fatalf("duplicate field: %v", err)
	}
}
