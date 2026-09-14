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
	output string
	code   string
	calls  int
}

func (s *fillingTextStub) GenerateText(_ context.Context, request provider.TextAdapterRequest) (provider.SynchronousResult, error) {
	s.calls++
	return provider.SynchronousResult{ProviderCode: s.code, ModelVersion: "test-model", Text: s.output}, nil
}

func fillingFixture(t *testing.T) (Service, contract.ActorContext, FillingRequest, *fillingTextStub) {
	t.Helper()
	service, actor := newTestService()
	text := &fillingTextStub{code: "ark_text", output: `{"suggestions":[{"field":"product","candidate_ids":["connector:1"],"text":"","reason":"匹配当前项目产品"}]}`}
	service.FillingText = &provider.Service{TextAdapter: text}
	service.FillingModelAlias = "cookies.text.standard"
	brand := contract.BrandID("brand_a")
	service.LoadFillingContext = func(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, request FillingRequest) (FillingContext, error) {
		if request.Strategy != nil && request.Strategy.ContentHash != "approved" {
			return FillingContext{}, ErrInvalidRequest
		}
		return FillingContext{Project: contract.ProjectContext{OrganizationID: actor.OrganizationID, ProjectID: "project_a", BrandID: &brand, ProductIDs: []contract.ProductID{}, ProjectContextVersion: 1}, Facts: json.RawMessage(`{"project":"fact"}`), Strategy: request.Strategy, Sources: []string{"项目"}, ProhibitedClaims: []string{"保证收益"}, Choices: map[string][]FillingChoice{"product": {
			{ID: "connector:1", Label: "产品", Value: json.RawMessage(`{"namespace":"oceanengine","object_kind":"product","scope":"account:a","id":"same","version":"7","content_hash":"hash","state":"resolved","audit_attributes":{"connector_platform_object_id":"1"}}`), Source: "目录"},
			{ID: "cookies:1", Label: "产品", Value: json.RawMessage(`{"namespace":"cookies","object_kind":"product","scope":"current_project","id":"same","state":"resolved"}`), Source: "项目"},
		}}}, nil
	}
	return service, actor, FillingRequest{Page: "plan", Fields: []string{"product"}, Current: map[string]json.RawMessage{}, Ocean: *testPlatformCreateRequest().PlatformConfiguration.Payload.OceanEngine}, text
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
		`{"suggestions":[{"field":"product","candidate_ids":["missing"],"text":"","reason":"x"}]}`,
		`{"suggestions":[{"field":"product","candidate_ids":["connector:1"],"text":"invented","reason":"x"}]}`,
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
			v.Choices["product"] = nil
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
	r.PlanID = "saved"
	if _, err := s.SuggestFilling(context.Background(), a, "project_a", r); !errors.Is(err, ErrInvalidRequest) {
		t.Fatal(err)
	}
	r.PlanID = ""
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
