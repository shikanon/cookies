package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/identity"
	"github.com/shikanon/cookies/internal/systems/creative"
)

type productionCenterQueryStub struct {
	request      creative.ListProductionRunsRequest
	assetRequest creative.ListProductionAssetsRequest
}

type productionRetryCommandStub struct {
	ref creative.ProductionRunRef
	key contract.IdempotencyKey
	err error
}

func (s *productionRetryCommandStub) Retry(_ context.Context, _ contract.RequestContext, _ contract.ProjectID, ref creative.ProductionRunRef, key contract.IdempotencyKey) (creative.ProductionRetryResult, error) {
	s.ref, s.key = ref, key
	if s.err != nil {
		return creative.ProductionRetryResult{}, s.err
	}
	return creative.ProductionRetryResult{ContractVersion: "creative-production-retry/v1", Status: creative.ProductionRetryAccepted, PreviousRun: ref, NewRun: creative.ProductionRunRef{Source: ref.Source, ID: "render-new"}, SourceTask: &creative.ProductionSourceTask{System: "creative", ObjectType: "edit_task", ObjectID: "edit-1"}}, nil
}

func TestRetryProductionRunReturnsFrozenSourceWorkflowProblem(t *testing.T) {
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, Scopes: []contract.Scope{creative.ScopeWrite}}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	sourceTask := &creative.ProductionSourceTask{System: "creative.ai-native-ad", ObjectType: "production_unit", ObjectID: "unit-1"}
	command := &productionRetryCommandStub{err: creative.ProductionRetryRequiresSourceWorkflowError{SourceTask: sourceTask}}
	server := NewWithDependencies(Dependencies{Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"}, ProductionRetry: command})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/creative/v1/projects/project_1/production-runs/provider/job-old:retry", nil)
	request.Header.Set("Idempotency-Key", "retry_key_2")

	server.ServeHTTP(response, request)

	body := response.Body.String()
	if response.Code != http.StatusConflict || !strings.Contains(body, `"contract_version":"creative-production-problem/v1"`) || !strings.Contains(body, `"code":"PRODUCTION_RETRY_REQUIRES_SOURCE_WORKFLOW"`) || !strings.Contains(body, `"object_id":"unit-1"`) || strings.Contains(body, `"error":`) {
		t.Fatalf("status=%d body=%s", response.Code, body)
	}
}

func (s *productionCenterQueryStub) ListAssets(_ context.Context, _ contract.ActorContext, projectID contract.ProjectID, request creative.ListProductionAssetsRequest) (creative.ProductionAssetPage, error) {
	s.assetRequest = request
	return creative.ProductionAssetPage{ContractVersion: "creative-production-asset-page/v1", ProjectID: projectID, Items: []creative.ProductionAssetListItem{}, SourceHealth: []creative.ProductionSourceHealth{}}, nil
}

func (s *productionCenterQueryStub) ListRuns(_ context.Context, _ contract.ActorContext, projectID contract.ProjectID, request creative.ListProductionRunsRequest) (creative.ProductionRunPage, error) {
	s.request = request
	return creative.ProductionRunPage{ContractVersion: "creative-production-run-page/v1", ProjectID: projectID, Items: []creative.ProductionRunSummary{}, SourceHealth: []creative.ProductionSourceHealth{}}, nil
}
func (s *productionCenterQueryStub) GetRun(context.Context, contract.ActorContext, contract.ProjectID, creative.ProductionRunRef) (creative.ProductionRunDetail, error) {
	return creative.ProductionRunDetail{}, nil
}

func TestListProductionRunsUsesFrozenQueryContract(t *testing.T) {
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, Scopes: []contract.Scope{creative.ScopeRead}}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	query := &productionCenterQueryStub{}
	server := NewWithDependencies(Dependencies{
		Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"}, ProductionCenter: query,
	})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/creative/v1/projects/project_1/production-runs?media_kind=video&status=running,failed&limit=25&q=JOB", nil)

	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"contract_version":"creative-production-run-page/v1"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if query.request.MediaKind != creative.ProductionMediaVideo || query.request.Limit != 25 || len(query.request.Statuses) != 2 || query.request.Query != "JOB" {
		t.Fatalf("query contract was not mapped: %#v", query.request)
	}
}

func TestListProductionAssetsUsesFrozenLineageQueryContract(t *testing.T) {
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, Scopes: []contract.Scope{creative.ScopeRead}}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	query := &productionCenterQueryStub{}
	server := NewWithDependencies(Dependencies{
		Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"}, ProductionCenter: query, ProductionAssets: query,
	})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/creative/v1/projects/project_1/production-assets?role=output&media_kind=video&run_source=provider&limit=25", nil)

	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"contract_version":"creative-production-asset-page/v1"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if query.assetRequest.Role != "output" || query.assetRequest.MediaKind != creative.ProductionMediaVideo || query.assetRequest.RunSource != creative.ProductionSourceProvider || query.assetRequest.Limit != 25 {
		t.Fatalf("asset query contract was not mapped: %#v", query.assetRequest)
	}
}

func TestRetryProductionRunUsesFrozenCommandContract(t *testing.T) {
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, Scopes: []contract.Scope{creative.ScopeWrite}}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	command := &productionRetryCommandStub{}
	server := NewWithDependencies(Dependencies{
		Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"}, ProductionRetry: command,
	})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/creative/v1/projects/project_1/production-runs/editing_render/render-old:retry", nil)
	request.Header.Set("Idempotency-Key", "retry_key_1")

	server.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"new_run":{"source":"editing_render","id":"render-new"}`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if command.ref.ID != "render-old" || command.ref.Source != creative.ProductionSourceEditingRender || command.key != "retry_key_1" {
		t.Fatalf("retry command was not mapped: ref=%#v key=%q", command.ref, command.key)
	}
}
