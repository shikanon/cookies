package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/integrations/oceanengine"
	"github.com/shikanon/cookies/internal/platform/connector"
	"github.com/shikanon/cookies/internal/platform/contract"
)

type readerStub struct {
	query          connector.Query
	objectQuery    connector.PlatformObjectQuery
	objects        []connector.PlatformObject
	preview        connector.PlatformObjectPreview
	previewQuery   connector.PlatformObjectPreviewQuery
	previewContent connector.PlatformObjectPreviewContent
	previews       []connector.PlatformObjectPreview
	previewCalls   int
}

func (r *readerStub) ReadPlatformObjectPreview(_ context.Context, _ connector.PlatformObjectPreviewQuery) (connector.PlatformObjectPreviewContent, error) {
	return r.previewContent, nil
}

func (r *readerStub) GetPlatformObjectPreview(_ context.Context, query connector.PlatformObjectPreviewQuery) (connector.PlatformObjectPreview, error) {
	r.previewQuery = query
	if len(r.previews) > 0 {
		index := r.previewCalls
		if index >= len(r.previews) {
			index = len(r.previews) - 1
		}
		r.previewCalls++
		return r.previews[index], nil
	}
	return r.preview, nil
}

func (r *readerStub) Snapshot(_ context.Context, q connector.Query) (connector.CanonicalSnapshot, error) {
	r.query = q
	return connector.CanonicalSnapshot{DatasetVersion: connector.DatasetVersion, PredictionCutoff: q.PredictionCutoff}, nil
}

func (r *readerStub) ReconcilePlatformObjects(context.Context, string, string, string, string, connector.PlatformObjectKind, time.Time, []connector.PlatformObjectCandidate) (connector.PlatformObjectSyncStats, error) {
	return connector.PlatformObjectSyncStats{}, nil
}

func (r *readerStub) ListPlatformObjects(_ context.Context, query connector.PlatformObjectQuery) ([]connector.PlatformObject, error) {
	r.objectQuery = query
	return r.objects, nil
}

type syncerStub struct {
	mu                       sync.Mutex
	request                  connector.SyncRequest
	refreshQuery             connector.PlatformObjectPreviewQuery
	refreshedPreview         connector.PlatformObjectPreview
	capabilityRequest        connector.OptimizationTargetCapabilityRequest
	accountCapabilityRequest connector.AccountCapabilityRequest
}
type authorizerStub struct{ err error }
type sessionManagerStub struct{ plaintext string }
type accountManagerStub struct {
	verifyErr error
	accounts  []connector.PlatformAccount
}

func (s accountManagerStub) Register(context.Context, connector.RegisterAccountRequest) (connector.PlatformAccount, error) {
	return connector.PlatformAccount{}, nil
}
func (s accountManagerStub) List(context.Context, string, string) ([]connector.PlatformAccount, error) {
	return s.accounts, nil
}
func (s accountManagerStub) Claim(context.Context, string, string, string) (connector.PlatformAccount, error) {
	return connector.PlatformAccount{}, nil
}
func (s accountManagerStub) Verify(context.Context, string, string, string) (connector.PlatformAccount, error) {
	return connector.PlatformAccount{}, s.verifyErr
}
func (s accountManagerStub) Revoke(context.Context, string, string, string) (connector.PlatformAccount, error) {
	return connector.PlatformAccount{}, nil
}

func (s *sessionManagerStub) Get(context.Context, string, string) (connector.OceanEngineAccountSession, error) {
	return connector.OceanEngineAccountSession{ID: "session_safe", OrganizationID: "org_1", AccountID: "oeacct_safe", Status: connector.AccountSessionUnverified, CredentialRefPresent: true, Version: 1}, nil
}
func (s *sessionManagerStub) Update(_ context.Context, _, _ string, plaintext []byte, _ int64) (connector.OceanEngineAccountSession, error) {
	s.plaintext = string(plaintext)
	return connector.OceanEngineAccountSession{ID: "session_safe", OrganizationID: "org_1", AccountID: "oeacct_safe", Status: connector.AccountSessionUnverified, CredentialRefPresent: true, Version: 1}, nil
}

func (a authorizerStub) AuthorizeProject(context.Context, contract.ActorContext, contract.ProjectID) error {
	return a.err
}

func (s *syncerStub) Sync(_ context.Context, r connector.SyncRequest) (connector.SyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.request = r
	return connector.SyncResult{RunID: "sync_opaque"}, nil
}

func (s *syncerStub) ReadOptimizationTargetCapabilities(_ context.Context, request connector.OptimizationTargetCapabilityRequest) (connector.OptimizationTargetCapabilitySnapshot, error) {
	s.capabilityRequest = request
	return connector.OptimizationTargetCapabilitySnapshot{SchemaVersion: connector.OptimizationTargetCapabilitySchemaV1, SnapshotID: "oecap_safe", AccountID: request.AccountRef, Context: request.Context, Options: []connector.OptimizationTargetCapability{{ExternalAction: "2", SemanticKey: "form", DisplayName: "表单提交"}}}, nil
}

func (s *syncerStub) ReadAccountCapabilities(_ context.Context, request connector.AccountCapabilityRequest) (connector.OceanEngineAccountCapabilitySnapshot, error) {
	s.accountCapabilityRequest = request
	return connector.OceanEngineAccountCapabilitySnapshot{SchemaVersion: connector.OceanEngineAccountCapabilitySchemaV1, SnapshotID: "oeaccountcap_safe", AccountID: request.AccountRef, ExternalActions: []connector.AccountCapabilityValue{{Key: "form", DisplayName: "表单提交", Value: "2"}}}, nil
}

func (s *syncerStub) RefreshPlatformObjectPreview(_ context.Context, query connector.PlatformObjectPreviewQuery) (connector.PlatformObjectPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshQuery = query
	return s.refreshedPreview, nil
}

func (s *syncerStub) lastRequest() connector.SyncRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.request
}
func request(method, path, body, scope string) *http.Request {
	value := httptest.NewRequest(method, path, strings.NewReader(body))
	value = value.WithContext(contract.WithRequestContext(value.Context(), contract.RequestContext{RequestID: "request_1", TraceID: "trace_1", Actor: contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"}, Scopes: []contract.Scope{contract.Scope(scope)}}}))
	return value
}

func TestCanonicalSnapshotRequiresCutoffAndScopesAccount(t *testing.T) {
	reader := &readerStub{}
	server := New(reader, nil, authorizerStub{}, nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request(http.MethodGet, "/api/connector/v1/projects/project_1/accounts/raw-account/canonical-snapshots?prediction_cutoff=2026-08-20T08:00:00Z", "", connector.ScopeRead))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if reader.query.ProjectID != "project_1" || reader.query.SourceRef == "raw-account" || reader.query.SourceRef == "" {
		t.Fatalf("query=%#v", reader.query)
	}
	if strings.Contains(response.Body.String(), "raw-account") || strings.Contains(strings.ToLower(response.Body.String()), "cookie") {
		t.Fatalf("sensitive response=%s", response.Body.String())
	}
}

func TestOptimizationTargetCapabilitiesUseExactProjectAccountBranch(t *testing.T) {
	syncer := &syncerStub{}
	accounts := accountManagerStub{accounts: []connector.PlatformAccount{{ID: "oeacct_safe", ProjectID: "project_1", Status: "verified"}}}
	server := New(nil, syncer, authorizerStub{}, accounts)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request(http.MethodPost, "/api/connector/v1/projects/project_1/accounts/oeacct_safe/optimization-target-capabilities", `{"context":{"campaign_type":1,"landing_type":1,"asset_type":2,"micro_app_id":"","cdp_marketing_goal":1,"dpa_ad_type":0,"micro_promotion_type":2,"micro_app_instance_id":"","multi_asset_types":[2,1002],"need_assets":false}}`, connector.ScopeRead))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "表单提交") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	want := oceanengine.OptimizationTargetContext{CampaignType: 1, LandingType: 1, AssetType: 2, CDPMarketingGoal: 1, MicroPromotionType: 2, MultiAssetTypes: []int{2, 1002}}
	if syncer.capabilityRequest.OrganizationID != "org_1" || syncer.capabilityRequest.ProjectID != "project_1" || syncer.capabilityRequest.AccountRef != "oeacct_safe" || syncer.capabilityRequest.Context.AssetType != want.AssetType || len(syncer.capabilityRequest.Context.MultiAssetTypes) != 2 {
		t.Fatalf("request=%#v", syncer.capabilityRequest)
	}
}

func TestAccountCapabilitiesUseExactProjectAccountSession(t *testing.T) {
	syncer := &syncerStub{}
	accounts := accountManagerStub{accounts: []connector.PlatformAccount{{ID: "oeacct_safe", ProjectID: "project_1", Status: "verified"}}}
	server := New(nil, syncer, authorizerStub{}, accounts)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request(http.MethodGet, "/api/connector/v1/projects/project_1/accounts/oeacct_safe/capabilities", "", connector.ScopeRead))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "表单提交") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if syncer.accountCapabilityRequest.OrganizationID != "org_1" || syncer.accountCapabilityRequest.ProjectID != "project_1" || syncer.accountCapabilityRequest.AccountRef != "oeacct_safe" {
		t.Fatalf("request=%#v", syncer.accountCapabilityRequest)
	}
}
func TestOrganizationSnapshotRequiresNoBusinessProject(t *testing.T) {
	reader := &readerStub{}
	server := New(reader, nil, authorizerStub{err: context.Canceled}, nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request(http.MethodGet, "/api/connector/v1/accounts/oeacct_safe/canonical-snapshots?prediction_cutoff=2026-08-20T08:00:00Z", "", connector.ScopeRead))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if reader.query.ProjectID != "" || reader.query.OrganizationID != "org_1" || reader.query.SourceRef == "" {
		t.Fatalf("query=%#v", reader.query)
	}
}
func TestCanonicalSnapshotRejectsMissingCutoffAndScope(t *testing.T) {
	server := New(&readerStub{}, nil, authorizerStub{}, nil)
	for _, test := range []struct {
		path, scope string
		status      int
	}{{"/api/connector/v1/projects/project_1/accounts/a/canonical-snapshots", connector.ScopeRead, http.StatusBadRequest}, {"/api/connector/v1/projects/project_1/accounts/a/canonical-snapshots?prediction_cutoff=2026-08-20T08:00:00Z", "insights.read", http.StatusForbidden}} {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request(http.MethodGet, test.path, "", test.scope))
		if response.Code != test.status {
			t.Fatalf("path=%s status=%d body=%s", test.path, response.Code, response.Body.String())
		}
	}
}

func TestPlatformObjectListScopesProjectAccountAndFilters(t *testing.T) {
	reader := &readerStub{objects: []connector.PlatformObject{{
		ID: "oeobj_safe", AccountID: "oeacct_safe", Kind: connector.PlatformObjectVideoMaterial,
		PlatformObjectID: "123456789", DisplayName: "video-a", Status: "active", PreviewAvailable: true, PreviewKind: "video_poster",
		Performance: &connector.PlatformObjectPerformance{Available: true, SpendMinor: 1234, Impressions: 100, Clicks: 5, Conversions: 2, CTR: 0.05},
	}}}
	accounts := accountManagerStub{accounts: []connector.PlatformAccount{{ID: "oeacct_safe", ProjectID: "project_1", Status: "verified"}}}
	server := New(reader, nil, authorizerStub{}, accounts)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request(http.MethodGet, "/api/connector/v1/projects/project_1/accounts/oeacct_safe/platform-objects?object_kind=video_material&status=active&q=video&limit=1&sort_by=ctr&sort_order=desc&cursor=offset%3A40", "", connector.ScopeRead))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	query := reader.objectQuery
	if query.OrganizationID != "org_1" || query.ProjectID != "project_1" || query.AccountID != "oeacct_safe" || query.Kind != connector.PlatformObjectVideoMaterial || query.Status != "active" || query.Search != "video" || query.Limit != 1 || query.SortBy != "ctr" || query.SortOrder != "desc" || query.Offset != 40 {
		t.Fatalf("query=%#v", query)
	}
	if !strings.Contains(response.Body.String(), "123456789") {
		t.Fatalf("object missing from response: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "/platform-objects/oeobj_safe/preview") || strings.Contains(response.Body.String(), "example.invalid") {
		t.Fatalf("safe preview URL missing: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"spend_minor":1234`) || !strings.Contains(response.Body.String(), `"next_cursor":"offset:41"`) {
		t.Fatalf("performance or sorted cursor missing: %s", response.Body.String())
	}
}

func TestPlatformObjectListRejectsInvalidSort(t *testing.T) {
	accounts := accountManagerStub{accounts: []connector.PlatformAccount{{ID: "oeacct_safe", ProjectID: "project_1", Status: "verified"}}}
	server := New(&readerStub{}, nil, authorizerStub{}, accounts)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request(http.MethodGet, "/api/connector/v1/projects/project_1/accounts/oeacct_safe/platform-objects?sort_by=spend", "", connector.ScopeRead))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestPlatformObjectPreviewRedirectRequiresProjectGrant(t *testing.T) {
	reader := &readerStub{preview: connector.PlatformObjectPreview{URL: "https://example.invalid/signed-preview", Kind: "landing_page"}}
	accounts := accountManagerStub{accounts: []connector.PlatformAccount{{ID: "oeacct_safe", ProjectID: "project_1", Status: "verified"}}}
	server := New(reader, nil, authorizerStub{}, accounts)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request(http.MethodGet, "/api/connector/v1/projects/project_1/accounts/oeacct_safe/platform-objects/oeobj_safe/preview", "", connector.ScopeRead))
	if response.Code != http.StatusTemporaryRedirect || response.Header().Get("Location") != "https://example.invalid/signed-preview" {
		t.Fatalf("status=%d location=%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	if strings.Contains(response.Body.String(), "signed-preview") {
		t.Fatalf("redirect target leaked in body: %s", response.Body.String())
	}
	if reader.previewQuery.OrganizationID != "org_1" || reader.previewQuery.ProjectID != "project_1" || reader.previewQuery.AccountID != "oeacct_safe" || reader.previewQuery.ObjectID != "oeobj_safe" {
		t.Fatalf("query=%#v", reader.previewQuery)
	}
}

func TestPlatformObjectMediaPreviewReturnsSameOriginContent(t *testing.T) {
	reader := &readerStub{preview: connector.PlatformObjectPreview{URL: "https://example.invalid/signed-preview", Kind: "image"}, previewContent: connector.PlatformObjectPreviewContent{ContentType: "image/jpeg", Data: []byte("safe-image")}}
	accounts := accountManagerStub{accounts: []connector.PlatformAccount{{ID: "oeacct_safe", ProjectID: "project_1", Status: "verified"}}}
	server := New(reader, nil, authorizerStub{}, accounts)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request(http.MethodGet, "/api/connector/v1/projects/project_1/accounts/oeacct_safe/platform-objects/oeobj_safe/preview", "", connector.ScopeRead))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/jpeg" || response.Body.String() != "safe-image" {
		t.Fatalf("status=%d content-type=%q body=%q", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
}

func TestExpiredPlatformObjectPreviewRefreshesTargetCatalog(t *testing.T) {
	expired := time.Now().Add(-time.Hour)
	fresh := time.Now().Add(12 * time.Hour)
	reader := &readerStub{
		previews: []connector.PlatformObjectPreview{
			{URL: "https://example.invalid/expired", Kind: "video_poster", ObjectKind: connector.PlatformObjectVideoMaterial, ExpiresAt: &expired},
			{URL: "https://example.invalid/fresh", Kind: "video_poster", ObjectKind: connector.PlatformObjectVideoMaterial, ExpiresAt: &fresh},
		},
		previewContent: connector.PlatformObjectPreviewContent{ContentType: "image/jpeg", Data: []byte("fresh-image")},
	}
	syncer := &syncerStub{refreshedPreview: reader.previews[1]}
	accounts := accountManagerStub{accounts: []connector.PlatformAccount{{ID: "oeacct_safe", ProjectID: "project_1", Status: "verified"}}}
	server := New(reader, syncer, authorizerStub{}, accounts)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request(http.MethodGet, "/api/connector/v1/projects/project_1/accounts/oeacct_safe/platform-objects/oeobj_safe/preview", "", connector.ScopeRead))
	if response.Code != http.StatusOK || response.Body.String() != "fresh-image" {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if syncer.refreshQuery.ProjectID != "project_1" || syncer.refreshQuery.AccountID != "oeacct_safe" || syncer.refreshQuery.ObjectID != "oeobj_safe" {
		t.Fatalf("refresh query=%#v", syncer.refreshQuery)
	}
	if reader.previewCalls != 1 {
		t.Fatalf("preview reads=%d", reader.previewCalls)
	}
}
func TestSyncRequiresIdempotencyAndPassesNoCredentialMaterial(t *testing.T) {
	syncer := &syncerStub{}
	server := New(nil, syncer, authorizerStub{}, nil)
	value := request(http.MethodPost, "/api/connector/v1/projects/project_1/accounts/account-1/syncs", `{"start":"2026-08-19T00:00:00Z","end":"2026-08-20T00:00:00Z","time_zone":"Asia/Shanghai","currency":"CNY"}`, connector.ScopeSync)
	value.Header.Set("Idempotency-Key", "sync-request-1")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, value)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	deadline := time.Now().Add(time.Second)
	var syncedRequest connector.SyncRequest
	for time.Now().Before(deadline) {
		syncedRequest = syncer.lastRequest()
		if syncedRequest.IdempotencyKey != "" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if syncedRequest.IdempotencyKey != "sync-request-1" || syncedRequest.AccountRef != "account-1" || !syncedRequest.WindowEnd.After(syncedRequest.WindowStart) {
		t.Fatalf("request=%#v", syncedRequest)
	}
	if strings.Contains(strings.ToLower(response.Body.String()), "token") || strings.Contains(strings.ToLower(response.Body.String()), "cookie") {
		t.Fatalf("sensitive response=%s", response.Body.String())
	}
}

func TestOrganizationSyncAndSessionDoNotRequireProjectOrReturnCredential(t *testing.T) {
	syncer := &syncerStub{}
	sessions := &sessionManagerStub{}
	server := New(nil, syncer, authorizerStub{err: context.Canceled}, nil, sessions)
	sessionRequest := request(http.MethodPut, "/api/connector/v1/accounts/oeacct_safe/session", `{"session":"synthetic-cookie","expected_version":0}`, connector.ScopeSync)
	sessionResponse := httptest.NewRecorder()
	server.ServeHTTP(sessionResponse, sessionRequest)
	if sessionResponse.Code != http.StatusOK || sessions.plaintext != "synthetic-cookie" {
		t.Fatalf("status=%d plaintext=%q body=%s", sessionResponse.Code, sessions.plaintext, sessionResponse.Body.String())
	}
	if strings.Contains(sessionResponse.Body.String(), "synthetic-cookie") {
		t.Fatalf("credential returned in response: %s", sessionResponse.Body.String())
	}
	syncRequest := request(http.MethodPost, "/api/connector/v1/accounts/oeacct_safe/syncs", `{"start":"2026-08-19T00:00:00Z","end":"2026-08-20T00:00:00Z"}`, connector.ScopeSync)
	syncRequest.Header.Set("Idempotency-Key", "org-sync-1")
	syncResponse := httptest.NewRecorder()
	server.ServeHTTP(syncResponse, syncRequest)
	var syncedRequest connector.SyncRequest
	for attempt := 0; attempt < 100; attempt++ {
		syncedRequest = syncer.lastRequest()
		if syncedRequest.OrganizationID != "" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if syncResponse.Code != http.StatusAccepted || syncedRequest.ProjectID != "" || syncedRequest.OrganizationID != "org_1" {
		t.Fatalf("status=%d request=%#v body=%s", syncResponse.Code, syncedRequest, syncResponse.Body.String())
	}
}

func TestOrganizationVerifyDoesNotReportEveryPlatformFailureAsVersionConflict(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{connector.ErrImmutableConflict, http.StatusConflict, "VERSION_CONFLICT"},
		{connector.ErrAccountSessionInvalid, http.StatusUnprocessableEntity, "CONNECTOR_ACCOUNT_SESSION_INVALID"},
		{connector.ErrAccountVerificationUnavailable, http.StatusBadGateway, "CONNECTOR_ACCOUNT_VERIFY_UNAVAILABLE"},
	}
	for _, test := range tests {
		server := New(nil, nil, authorizerStub{}, accountManagerStub{verifyErr: test.err})
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request(http.MethodPost, "/api/connector/v1/accounts/oeacct_safe/verify", "", connector.ScopeSync))
		if response.Code != test.status || !strings.Contains(response.Body.String(), test.code) {
			t.Fatalf("error=%v status=%d body=%s", test.err, response.Code, response.Body.String())
		}
	}
}

func TestProjectVerifyReportsSpecificFailure(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{connector.ErrImmutableConflict, http.StatusConflict, "VERSION_CONFLICT"},
		{connector.ErrAccountSessionInvalid, http.StatusUnprocessableEntity, "CONNECTOR_ACCOUNT_SESSION_INVALID"},
		{connector.ErrAccountVerificationUnavailable, http.StatusBadGateway, "CONNECTOR_ACCOUNT_VERIFY_UNAVAILABLE"},
	}
	for _, test := range tests {
		server := New(nil, nil, authorizerStub{}, accountManagerStub{verifyErr: test.err})
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request(http.MethodPost, "/api/connector/v1/projects/project_1/accounts/oeacct_safe/verify", "", connector.ScopeSync))
		if response.Code != test.status || !strings.Contains(response.Body.String(), test.code) {
			t.Fatalf("error=%v status=%d body=%s", test.err, response.Code, response.Body.String())
		}
	}
}
