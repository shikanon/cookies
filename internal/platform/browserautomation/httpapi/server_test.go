package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/platform/contract"
)

type projectAuthorizer struct{}

type planAdapter struct {
	browserautomation.DeterministicFakeAdapter
}

func (planAdapter) Plan(context.Context, browserautomation.BrowserRpaRun) (json.RawMessage, error) {
	return json.RawMessage(`{"schema_version":"oceanengine-playwright-rpa-plan/v3","mode":"prepare","allow_remote_write":false}`), nil
}

type authorityProvider struct {
	binding    browserautomation.AuthorityBinding
	boundRunID string
}

func (p *authorityProvider) ResolveAuthority(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, executionID string, _ time.Time) (browserautomation.AuthorityResolution, error) {
	if executionID != p.binding.BusinessExecutionID {
		return browserautomation.AuthorityResolution{}, browserautomation.ErrNotFound
	}
	return browserautomation.AuthorityResolution{Binding: p.binding, BoundRunID: p.boundRunID}, nil
}
func (p *authorityProvider) BindRun(_ context.Context, _ browserautomation.AuthorityBinding, runID string, _ time.Time) error {
	if p.boundRunID != "" && p.boundRunID != runID {
		return browserautomation.ErrIdempotencyConflict
	}
	p.boundRunID = runID
	return nil
}
func (p *authorityProvider) VerifyAuthority(_ context.Context, binding browserautomation.AuthorityBinding, runID string, _ time.Time) error {
	if binding != p.binding || runID != p.boundRunID {
		return browserautomation.ErrInvalidContract
	}
	return nil
}

func (projectAuthorizer) AuthorizeProject(_ context.Context, actor contract.ActorContext, project contract.ProjectID) error {
	if actor.OrganizationID != "org_1" || project != "project_1" {
		return browserautomation.ErrNotFound
	}
	return nil
}
func (projectAuthorizer) AuthorizeProjectAction(ctx context.Context, actor contract.ActorContext, project contract.ProjectID, _ string) error {
	return projectAuthorizer{}.AuthorizeProject(ctx, actor, project)
}

func TestRunEndpointsRequireScopeAndProjectIsolation(t *testing.T) {
	repo := browserautomation.NewMemoryRepository()
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	run := validHTTPRun(now)
	_, _, _ = repo.CreateRun(context.Background(), run)
	service := browserautomation.Service{Repository: repo, Now: func() time.Time { return now }}
	server := New(service, browserautomation.Worker{Service: service, Adapter: browserautomation.DeterministicFakeAdapter{}}, projectAuthorizer{})
	request := httptest.NewRequest(http.MethodGet, "/api/platform/v1/browser-rpa/projects/project_1/runs/run_1", nil)
	request = request.WithContext(contract.WithRequestContext(request.Context(), contract.RequestContext{RequestID: "req", TraceID: "trace", Actor: contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user"}, Scopes: []contract.Scope{"delivery.read"}}}))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/platform/v1/browser-rpa/projects/project_1/runs", nil)
	request = request.WithContext(contract.WithRequestContext(request.Context(), contract.RequestContext{RequestID: "req", TraceID: "trace", Actor: contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user"}, Scopes: []contract.Scope{"delivery.read"}}}))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"run_1"`) {
		t.Fatalf("list status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/platform/v1/browser-rpa/projects/project_1/runs/run_1", nil)
	request = request.WithContext(contract.WithRequestContext(request.Context(), contract.RequestContext{RequestID: "req", TraceID: "trace", Actor: contract.ActorContext{OrganizationID: "org_2", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user"}, Scopes: []contract.Scope{"delivery.read"}}}))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-org status=%d", response.Code)
	}
}

func TestLegacyComputerUsePrefixRemainsMountedAsTransitionalAlias(t *testing.T) {
	repo := browserautomation.NewMemoryRepository()
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	run := validHTTPRun(now)
	_, _, _ = repo.CreateRun(context.Background(), run)
	service := browserautomation.Service{Repository: repo, Now: func() time.Time { return now }}
	server := NewTakeoverOnly(service, projectAuthorizer{})
	server.MountLegacyAlias()
	request := httptest.NewRequest(http.MethodGet, "/api/platform/v1/computer-use/projects/project_1/runs/run_1", nil)
	request = request.WithContext(contract.WithRequestContext(request.Context(), contract.RequestContext{RequestID: "req", TraceID: "trace", Actor: contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user"}, Scopes: []contract.Scope{"delivery.read"}}}))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("legacy prefix status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestTakeoverOnlyServerRegistersScopedResourcesWithoutMountingFakeWorker(t *testing.T) {
	repo := browserautomation.NewMemoryRepository()
	service := browserautomation.Service{Repository: repo}
	server := NewTakeoverOnly(service, projectAuthorizer{})
	call := func(method, path, body string, scopes ...contract.Scope) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request = request.WithContext(contract.WithRequestContext(request.Context(), contract.RequestContext{RequestID: "req", TraceID: "trace", Actor: contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user"}, Scopes: scopes}}))
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		return response
	}
	environment := call(http.MethodPost, "/api/platform/v1/browser-rpa/projects/project_1/environments", `{"id":"env","platform":"ocean_engine","account_id":"account","mode":"local_visible","browser_version":"edge-test","region":"local","healthy":true}`, "delivery.execute")
	if environment.Code != http.StatusCreated {
		t.Fatalf("environment status=%d body=%s", environment.Code, environment.Body.String())
	}
	profile := call(http.MethodPost, "/api/platform/v1/browser-rpa/projects/project_1/browser-profiles", `{"id":"profile","environment_id":"env","platform":"ocean_engine","account_id":"account","state":"ready"}`, "delivery.execute")
	if profile.Code != http.StatusCreated {
		t.Fatalf("profile status=%d body=%s", profile.Code, profile.Body.String())
	}
	policy := call(http.MethodPost, "/api/platform/v1/browser-rpa/projects/project_1/site-policies", `{"id":"policy","platform":"ocean_engine","account_id":"account","allowed_protocols":["https"],"allowed_hosts":["ad.oceanengine.com"],"allowed_page_kinds":["project_create"],"allowed_platform_project_ids":["test_project"]}`, "delivery.execute")
	if policy.Code != http.StatusCreated {
		t.Fatalf("policy status=%d body=%s", policy.Code, policy.Body.String())
	}
	read := call(http.MethodGet, "/api/platform/v1/browser-rpa/projects/project_1/site-policies/policy", "", "delivery.read")
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"ad.oceanengine.com"`) {
		t.Fatalf("read status=%d body=%s", read.Code, read.Body.String())
	}
	run := validHTTPRun(time.Now().UTC())
	_, _, _ = repo.CreateRun(context.Background(), run)
	for _, action := range []string{"plan", "prepare", "submit"} {
		response := call(http.MethodPost, "/api/platform/v1/browser-rpa/projects/project_1/runs/run_1:"+action, `{}`, "delivery.execute")
		if response.Code != http.StatusNotFound {
			t.Fatalf("fake worker %s unexpectedly mounted: status=%d body=%s", action, response.Code, response.Body.String())
		}
	}
	authorization := call(http.MethodPost, "/api/platform/v1/browser-rpa/projects/project_1/runs/run_1/takeover-action-attempts", `{}`, "delivery.execute")
	if authorization.Code != http.StatusBadRequest || strings.Contains(authorization.Body.String(), "automated worker") {
		t.Fatalf("manual takeover authorization port status=%d body=%s", authorization.Code, authorization.Body.String())
	}
}

func TestPlanPreviewAndLeaseReadSupportTheFrontendExecutionFlow(t *testing.T) {
	repo := browserautomation.NewMemoryRepository()
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	run := validHTTPRun(now)
	_, _, _ = repo.CreateRun(context.Background(), run)
	service := browserautomation.Service{Repository: repo, Now: func() time.Time { return now }}
	acquired, err := service.AcquireRunLease(context.Background(), run.OrganizationID, run.ProjectID, run.ID, run.Version, "operator")
	if err != nil {
		t.Fatal(err)
	}
	server := New(service, browserautomation.Worker{Service: service, Adapter: planAdapter{}}, projectAuthorizer{})
	call := func(method, path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, nil)
		scope := contract.Scope("delivery.read")
		if strings.Contains(path, "/leases/") {
			scope = "delivery.execute"
		}
		request = request.WithContext(contract.WithRequestContext(request.Context(), contract.RequestContext{RequestID: "req", TraceID: "trace", Actor: contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user"}, Scopes: []contract.Scope{scope}}}))
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		return response
	}
	plan := call(http.MethodPost, "/api/platform/v1/browser-rpa/projects/project_1/runs/run_1:plan")
	if plan.Code != http.StatusOK || !strings.Contains(plan.Body.String(), `"allow_remote_write":false`) {
		t.Fatalf("plan status=%d body=%s", plan.Code, plan.Body.String())
	}
	environmentChecked, err := service.TransitionRun(context.Background(), run.OrganizationID, run.ProjectID, run.ID, acquired.Run.Version, browserautomation.RunEnvironmentCheck, "")
	if err != nil {
		t.Fatal(err)
	}
	preparing, err := service.TransitionRun(context.Background(), run.OrganizationID, run.ProjectID, run.ID, environmentChecked.Version, browserautomation.RunPreparing, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.TransitionRun(context.Background(), run.OrganizationID, run.ProjectID, run.ID, preparing.Version, browserautomation.RunFailed, browserautomation.BlockPageDrift); err != nil {
		t.Fatal(err)
	}
	terminalPlan := call(http.MethodPost, "/api/platform/v1/browser-rpa/projects/project_1/runs/run_1:plan")
	if terminalPlan.Code != http.StatusOK || !strings.Contains(terminalPlan.Body.String(), `"allow_remote_write":false`) {
		t.Fatalf("terminal plan status=%d body=%s", terminalPlan.Code, terminalPlan.Body.String())
	}
	lease := call(http.MethodGet, fmt.Sprintf("/api/platform/v1/browser-rpa/projects/project_1/runs/run_1/leases/%s", acquired.Lease.ID))
	if lease.Code != http.StatusOK || !strings.Contains(lease.Body.String(), `"fencing_token":1`) {
		t.Fatalf("lease status=%d body=%s", lease.Code, lease.Body.String())
	}
}

func TestSessionCheckUsesTheAutomatedWorkerAndReturnsSafeFacts(t *testing.T) {
	repo := browserautomation.NewMemoryRepository()
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	run := validHTTPRun(now)
	_, _, _ = repo.CreateRun(context.Background(), run)
	service := browserautomation.Service{Repository: repo, Now: func() time.Time { return now }}
	server := New(service, browserautomation.Worker{Service: service, Adapter: browserautomation.DeterministicFakeAdapter{}}, projectAuthorizer{})
	request := httptest.NewRequest(http.MethodPost, "/api/platform/v1/browser-rpa/projects/project_1/runs/run_1:check-session", nil)
	request = request.WithContext(contract.WithRequestContext(request.Context(), contract.RequestContext{RequestID: "req", TraceID: "trace", Actor: contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user"}, Scopes: []contract.Scope{"delivery.execute"}}}))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"account_matched":true`) || strings.Contains(response.Body.String(), "cdp_endpoint") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestPrepareContinuesAfterTheClientRequestIsCancelled(t *testing.T) {
	repo := browserautomation.NewMemoryRepository()
	now := time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
	run := validHTTPRun(now)
	_, _, _ = repo.CreateRun(context.Background(), run)
	service := browserautomation.Service{Repository: repo, Now: func() time.Time { return now }}
	server := New(service, browserautomation.Worker{Service: service, Adapter: browserautomation.DeterministicFakeAdapter{}}, projectAuthorizer{})

	request := httptest.NewRequest(http.MethodPost, "/api/platform/v1/browser-rpa/projects/project_1/runs/run_1:prepare", nil)
	request = request.WithContext(contract.WithRequestContext(request.Context(), contract.RequestContext{RequestID: "req", TraceID: "trace", Actor: contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user"}, Scopes: []contract.Scope{"delivery.execute"}}}))
	cancelled, cancel := context.WithCancel(request.Context())
	cancel()
	request = request.WithContext(cancelled)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"state":"awaiting_confirmation"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	stepsRequest := httptest.NewRequest(http.MethodGet, "/api/platform/v1/browser-rpa/projects/project_1/runs/run_1/steps", nil)
	stepsRequest = stepsRequest.WithContext(contract.WithRequestContext(stepsRequest.Context(), contract.RequestContext{RequestID: "req-steps", TraceID: "trace", Actor: contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user"}, Scopes: []contract.Scope{"delivery.read"}}}))
	stepsResponse := httptest.NewRecorder()
	server.ServeHTTP(stepsResponse, stepsRequest)
	if stepsResponse.Code != http.StatusOK || !strings.Contains(stepsResponse.Body.String(), `"action":"prepare_and_readback"`) || !strings.Contains(stepsResponse.Body.String(), `"status":"succeeded"`) {
		t.Fatalf("steps status=%d body=%s", stepsResponse.Code, stepsResponse.Body.String())
	}
}

func TestKillSwitchAdministrationRequiresServicePrincipalAndRemainsReadable(t *testing.T) {
	repo := browserautomation.NewMemoryRepository()
	service := browserautomation.Service{Repository: repo, Now: func() time.Time { return time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC) }}
	server := NewTakeoverOnly(service, projectAuthorizer{})
	call := func(path, body string, actor contract.ActorContext) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
		request = request.WithContext(contract.WithRequestContext(request.Context(), contract.RequestContext{RequestID: "req", TraceID: "trace", Actor: actor}))
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		return response
	}
	user := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user"}, Scopes: []contract.Scope{"platform.browser-rpa.admin"}}
	if response := call("/api/platform/v1/browser-rpa/kill-switches/organization", `{"active":true,"reason":"incident","expected_version":0}`, user); response.Code != http.StatusBadRequest {
		t.Fatalf("user admin status=%d body=%s", response.Code, response.Body.String())
	}
	admin := user
	admin.Principal = contract.Principal{Kind: contract.PrincipalService, ID: "safety-controller"}
	if response := call("/api/platform/v1/browser-rpa/kill-switches/organization", `{"active":true,"reason":"incident","expected_version":0}`, admin); response.Code != http.StatusOK {
		t.Fatalf("service admin status=%d body=%s", response.Code, response.Body.String())
	}
	request := httptest.NewRequest(http.MethodGet, "/api/platform/v1/browser-rpa/projects/project_1/kill-switches/active?platform=ocean_engine", nil)
	request = request.WithContext(contract.WithRequestContext(request.Context(), contract.RequestContext{RequestID: "req", TraceID: "trace", Actor: contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user"}, Scopes: []contract.Scope{"delivery.read"}}}))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"active":true`) {
		t.Fatalf("active status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestTakeoverEvidenceEndpointRecordsOnlyFencedReadback(t *testing.T) {
	repo := browserautomation.NewMemoryRepository()
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	run := validHTTPRun(now)
	run.State, run.Paused, run.TakeoverActive = browserautomation.RunAwaitingTakeover, true, true
	_, _, _ = repo.CreateRun(context.Background(), run)
	repo.PutSitePolicy(browserautomation.SitePolicy{ID: run.PolicyID, OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, Platform: run.Platform, AccountID: run.AccountID, AllowedProtocols: []string{"https"}, AllowedHosts: []string{"ad.oceanengine.com"}, AllowedPageKinds: []string{"project_create"}, AllowedPlatformProjects: []string{"test_project"}, Version: 1})
	idSequence := 0
	service := browserautomation.Service{Repository: repo, Now: func() time.Time { return now }, NewID: func(prefix string) (string, error) {
		idSequence++
		return fmt.Sprintf("%s_http_%d", prefix, idSequence), nil
	}}
	acquired, err := service.AcquireRunLease(context.Background(), run.OrganizationID, run.ProjectID, run.ID, run.Version, "agent")
	if err != nil {
		t.Fatal(err)
	}
	server := New(service, browserautomation.Worker{Service: service, Adapter: browserautomation.DeterministicFakeAdapter{}}, projectAuthorizer{})
	body := fmt.Sprintf(`{"expected_version":%d,"lease_id":%q,"fencing_token":%d,"step_id":"step_1","sequence":1,"action":"field_readback","status":"succeeded","page_kind":"project_create","platform_project_id":"test_project","before_page_facts":{"page_kind":"project_create"},"after_page_facts":{"page_kind":"project_create"},"field_readback":{"daily_budget":"300"},"diff_keys":[],"page_reference":"https://ad.oceanengine.com/superior/create-project?aadvid=secret","selector_version":"oceanengine-live-locators/v0.1","action_version":"takeover-readback/v1"}`, acquired.Run.Version, acquired.Lease.ID, acquired.Lease.FencingToken)
	request := httptest.NewRequest(http.MethodPost, "/api/platform/v1/browser-rpa/projects/project_1/runs/run_1/takeover-evidence", strings.NewReader(body))
	request = request.WithContext(contract.WithRequestContext(request.Context(), contract.RequestContext{RequestID: "req", TraceID: "trace", Actor: contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user"}, Scopes: []contract.Scope{"delivery.execute"}}}))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "aadvid") || !strings.Contains(response.Body.String(), `"daily_budget":"300"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRunLeaseHeartbeatAndReleaseAreScopedAndDetachTheRun(t *testing.T) {
	repo := browserautomation.NewMemoryRepository()
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	run := validHTTPRun(now)
	_, _, _ = repo.CreateRun(context.Background(), run)
	service := browserautomation.Service{Repository: repo, Now: func() time.Time { return now }}
	acquired, err := service.AcquireRunLease(context.Background(), run.OrganizationID, run.ProjectID, run.ID, run.Version, "agent")
	if err != nil {
		t.Fatal(err)
	}
	server := New(service, browserautomation.Worker{Service: service, Adapter: browserautomation.DeterministicFakeAdapter{}}, projectAuthorizer{})
	call := func(action, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/platform/v1/browser-rpa/projects/project_1/runs/run_1/leases/%s:%s", acquired.Lease.ID, action), strings.NewReader(body))
		request = request.WithContext(contract.WithRequestContext(request.Context(), contract.RequestContext{RequestID: "req", TraceID: "trace", Actor: contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user"}, Scopes: []contract.Scope{"delivery.execute"}}}))
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		return response
	}
	heartbeat := call("heartbeat", fmt.Sprintf(`{"expected_version":%d,"fencing_token":%d}`, acquired.Lease.Version, acquired.Lease.FencingToken))
	if heartbeat.Code != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", heartbeat.Code, heartbeat.Body.String())
	}
	var renewed browserautomation.SessionLease
	if err := json.Unmarshal(heartbeat.Body.Bytes(), &renewed); err != nil {
		t.Fatal(err)
	}
	release := call("release", fmt.Sprintf(`{"expected_run_version":%d,"expected_lease_version":%d,"fencing_token":%d}`, acquired.Run.Version, renewed.Version, renewed.FencingToken))
	if release.Code != http.StatusOK || !strings.Contains(release.Body.String(), `"lease_id":""`) || !strings.Contains(release.Body.String(), `"released_at"`) {
		t.Fatalf("release status=%d body=%s", release.Code, release.Body.String())
	}
	latest, err := repo.GetRun(context.Background(), run.OrganizationID, run.ProjectID, run.ID)
	if err != nil || latest.LeaseID != "" || latest.Version != acquired.Run.Version+1 {
		t.Fatalf("detached run=%+v err=%v", latest, err)
	}
	reacquired, err := service.AcquireRunLease(context.Background(), run.OrganizationID, run.ProjectID, run.ID, latest.Version, "agent_2")
	if err != nil || reacquired.Lease.FencingToken <= renewed.FencingToken {
		t.Fatalf("reacquired=%+v err=%v", reacquired, err)
	}
}

func TestCreateRunEndpointIsProjectScopedAndIdempotent(t *testing.T) {
	repo := browserautomation.NewMemoryRepository()
	now := time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	template := validHTTPRun(now)
	repo.PutEnvironment(browserautomation.ExecutionEnvironment{ID: template.EnvironmentID, OrganizationID: template.OrganizationID, ProjectID: template.ProjectID, Platform: template.Platform, AccountID: template.AccountID, Mode: "local_visible", BrowserVersion: "test", Region: "local", Healthy: true, Version: 1})
	repo.PutBrowserProfile(browserautomation.BrowserProfile{ID: template.ProfileID, OrganizationID: template.OrganizationID, ProjectID: template.ProjectID, EnvironmentID: template.EnvironmentID, Platform: template.Platform, AccountID: template.AccountID, State: "ready", Version: 1})
	repo.PutSitePolicy(browserautomation.SitePolicy{ID: template.PolicyID, OrganizationID: template.OrganizationID, ProjectID: template.ProjectID, Platform: template.Platform, AccountID: template.AccountID, AllowedProtocols: []string{"https"}, AllowedHosts: []string{"ad.oceanengine.com"}, AllowedPageKinds: []string{"project_create"}, AllowedPlatformProjects: []string{"test_project"}, Version: 1})
	repo.PutSitePolicy(browserautomation.SitePolicy{ID: "policy_drift", OrganizationID: template.OrganizationID, ProjectID: template.ProjectID, Platform: template.Platform, AccountID: template.AccountID, AllowedProtocols: []string{"https"}, AllowedHosts: []string{"ad.oceanengine.com"}, AllowedPageKinds: []string{"project_create"}, AllowedPlatformProjects: []string{"test_project"}, Version: 1})
	idSequence := 0
	provider := &authorityProvider{binding: template.Authority}
	service := browserautomation.Service{Repository: repo, AuthorityProvider: provider, Now: func() time.Time { return now }, NewID: func(prefix string) (string, error) {
		idSequence++
		return fmt.Sprintf("%s_create_%d", prefix, idSequence), nil
	}}
	server := New(service, browserautomation.Worker{Service: service, Adapter: browserautomation.DeterministicFakeAdapter{}}, projectAuthorizer{})
	body := `{"project_id":"project_1","platform":"ocean_engine","account_id":"account","business_execution_id":"exec","environment_id":"env","profile_id":"profile","policy_id":"policy"}`
	call := func(payload, key string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/platform/v1/browser-rpa/projects/project_1/runs", strings.NewReader(payload))
		request.Header.Set("Idempotency-Key", key)
		request = request.WithContext(contract.WithRequestContext(request.Context(), contract.RequestContext{RequestID: "req", TraceID: "trace", Actor: contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user"}, Scopes: []contract.Scope{"delivery.execute"}}}))
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		return response
	}
	created := call(body, "run-key")
	if created.Code != http.StatusCreated {
		t.Fatalf("created status=%d body=%s", created.Code, created.Body.String())
	}
	replayed := call(body, "run-key")
	if replayed.Code != http.StatusOK || replayed.Body.String() != created.Body.String() {
		t.Fatalf("replayed status=%d body=%s", replayed.Code, replayed.Body.String())
	}
	drifted := strings.Replace(body, `"policy_id":"policy"`, `"policy_id":"policy_drift"`, 1)
	conflict := call(drifted, "run-key")
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict status=%d body=%s", conflict.Code, conflict.Body.String())
	}
	secondKey := call(body, "different-run-key")
	if secondKey.Code != http.StatusConflict {
		t.Fatalf("same execution with a second key status=%d body=%s", secondKey.Code, secondKey.Body.String())
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}

func validHTTPRun(now time.Time) browserautomation.BrowserRpaRun {
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	authority := browserautomation.AuthorityBinding{SchemaVersion: browserautomation.AuthoritySchemaV1, OrganizationID: "org_1", ProjectID: "project_1", BusinessExecutionID: "exec", ChangeSetID: "change", ApprovalID: "approval", ApprovalActionHash: hash, AccountReferenceID: "account", ObjectFingerprint: hash, Action: "create_project_and_promotions", Currency: "CNY", PlanCanonicalHash: hash, IntentCanonicalHash: hash, FeedbackCanonicalHash: hash, DecisionCanonicalHash: hash, ConfigurationCanonicalHash: hash, WorkflowID: "workflow", WorkflowCanonicalHash: hash, WorkflowStepID: "submit", SkillID: "skill", SkillVersion: "v1"}
	return browserautomation.BrowserRpaRun{SchemaVersion: browserautomation.RunSchemaV1, ID: "run_1", OrganizationID: "org_1", ProjectID: "project_1", Platform: browserautomation.PlatformOceanEngine, AccountID: "account", Authority: authority, EnvironmentID: "env", ProfileID: "profile", PolicyID: "policy", State: browserautomation.RunQueued, Version: 1, IdempotencyKey: "key", RequestHash: hash, CreatedBy: "user", CreatedAt: now, UpdatedAt: now}
}
