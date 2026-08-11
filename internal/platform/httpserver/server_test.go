package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/identity"
	"github.com/shikanon/cookies/internal/platform/project"
	"github.com/shikanon/cookies/internal/platform/provider"
	"github.com/shikanon/cookies/internal/platform/remix"
	"github.com/shikanon/cookies/internal/systems/creative"
)

func TestHealthDoesNotRequireIdentity(t *testing.T) {
	t.Parallel()
	server := New(identity.RejectingResolver{})
	response := httptest.NewRecorder()

	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if got, want := response.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected response request ID")
	}
}

func TestProjectActionClassifiesRoleSensitiveRoutes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		method string
		path   string
		want   string
	}{
		{http.MethodGet, "/platform/v1/projects/p1/members", "read"},
		{http.MethodPost, "/platform/v1/projects/p1/members", "manage"},
		{http.MethodGet, "/platform/v1/projects/p1/assets", "read"},
		{http.MethodPatch, "/platform/v1/projects/p1/tasks/t1", "write"},
		{http.MethodPost, "/platform/v1/projects/p1/change-sets/c1/approve", "approve"},
	}
	for _, test := range tests {
		if got := projectAction(httptest.NewRequest(test.method, test.path, nil)); got != test.want {
			t.Fatalf("%s %s action=%q, want %q", test.method, test.path, got, test.want)
		}
	}
}

func TestOrganizationMemberRoutesStayInCurrentOrganization(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"},
		Scopes:         []contract.Scope{"organization.members.read"},
	}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	accounts := &staticAccountManager{members: []identity.OrganizationMember{{
		User:       identity.User{ID: "user_1", DisplayName: "Owner"},
		Membership: identity.OrganizationMembership{OrganizationID: "org_1", UserID: "user_1", Role: "owner", Status: "active"},
	}}}
	server := NewWithDependencies(Dependencies{Resolver: resolver, Accounts: accounts})

	allowed := httptest.NewRecorder()
	server.ServeHTTP(allowed, httptest.NewRequest(http.MethodGet, "/platform/v1/organizations/org_1/members", nil))
	if allowed.Code != http.StatusOK || !strings.Contains(allowed.Body.String(), `"user_1"`) {
		t.Fatalf("allowed status=%d body=%s", allowed.Code, allowed.Body.String())
	}
	denied := httptest.NewRecorder()
	server.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, "/platform/v1/organizations/org_2/members", nil))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("cross-organization status=%d body=%s", denied.Code, denied.Body.String())
	}
	if accounts.listMemberCalls != 1 {
		t.Fatalf("ListOrganizationMembers calls=%d, want 1", accounts.listMemberCalls)
	}
}

func TestProjectMemberWriteRequiresManageScope(t *testing.T) {
	t.Parallel()
	baseActor := contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"},
		Scopes:         []contract.Scope{"project.members.read"},
	}
	resolver, err := identity.NewStaticResolver(baseActor)
	if err != nil {
		t.Fatal(err)
	}
	members := &staticProjectMembershipManager{}
	server := NewWithDependencies(Dependencies{
		Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"},
		ProjectMembers: members,
	})
	body := `{"principal_kind":"user","principal_id":"user_2","role":"viewer"}`
	denied := httptest.NewRecorder()
	server.ServeHTTP(denied, httptest.NewRequest(http.MethodPost, "/platform/v1/projects/project_1/members", strings.NewReader(body)))
	if denied.Code != http.StatusForbidden || members.addCalls != 0 {
		t.Fatalf("denied status=%d addCalls=%d body=%s", denied.Code, members.addCalls, denied.Body.String())
	}

	baseActor.Scopes = []contract.Scope{"project.members.manage"}
	resolver, _ = identity.NewStaticResolver(baseActor)
	server = NewWithDependencies(Dependencies{
		Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"},
		ProjectMembers: members,
	})
	allowed := httptest.NewRecorder()
	server.ServeHTTP(allowed, httptest.NewRequest(http.MethodPost, "/platform/v1/projects/project_1/members", strings.NewReader(body)))
	if allowed.Code != http.StatusCreated || members.addCalls != 1 {
		t.Fatalf("allowed status=%d addCalls=%d body=%s", allowed.Code, members.addCalls, allowed.Body.String())
	}
}

func TestWorkbenchReviewWritesRequireIdempotencyKey(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"},
		Scopes:         []contract.Scope{"assets.write"},
	}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	server := NewWithDependencies(Dependencies{
		Resolver:          resolver,
		ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"},
		Projects:          staticProjectManager{},
	})

	missingKey := httptest.NewRecorder()
	server.ServeHTTP(missingKey, httptest.NewRequest(
		http.MethodPost,
		"/platform/v1/projects/project_1/assets/asset_1/versions/1/quality-checks",
		bytes.NewBufferString(`{}`),
	))
	if missingKey.Code != http.StatusBadRequest {
		t.Fatalf("missing key status=%d body=%s", missingKey.Code, missingKey.Body.String())
	}

	request := httptest.NewRequest(
		http.MethodPost,
		"/platform/v1/projects/project_1/assets/asset_1/versions/1/quality-checks",
		bytes.NewBufferString(`{}`),
	)
	request.Header.Set("Idempotency-Key", "material-qc-project_1-asset_1-1")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestProviderCapabilitiesExposeRoutesWithoutCredentials(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"},
	}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	server := NewWithDependencies(Dependencies{
		Resolver: resolver,
		ProviderConfig: staticProviderConfigurationReader{items: []provider.CapabilityStatus{{
			Capability: "video.generate", ModelAlias: "cookies.video.standard",
			UpstreamModel: "doubao-seedance", Available: true, CredentialConfigured: true,
			UpdatedAt: time.Date(2026, 7, 29, 6, 0, 0, 0, time.UTC),
		}}},
	})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/platform/v1/provider/capabilities", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"cookies.video.standard"`) || !strings.Contains(body, `"masked_api_key":"encrypted"`) {
		t.Fatalf("capability response missing safe route status: %s", body)
	}
	if strings.Contains(body, "ark-secret") {
		t.Fatalf("capability response leaked a credential: %s", body)
	}
}

func TestCreativeDomainErrorsAreMappedToActionableHTTPProblems(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		err           error
		wantStatus    int
		wantCode      string
		wantRetryable bool
	}{
		{name: "invalid state", err: creative.ErrInvalidState, wantStatus: http.StatusConflict, wantCode: "INVALID_STATE"},
		{name: "edit timeline conflict", err: creative.ErrEditTimelineVersionConflict, wantStatus: http.StatusConflict, wantCode: "EDIT_TIMELINE_VERSION_CONFLICT"},
		{name: "edit operation conflict", err: creative.ErrOperationVersionConflict, wantStatus: http.StatusConflict, wantCode: "EDIT_OPERATION_VERSION_CONFLICT"},
		{name: "stale version", err: creative.ErrVersionConflict, wantStatus: http.StatusPreconditionFailed, wantCode: "CREATIVE_VERSION_CONFLICT"},
		{name: "viral source unavailable", err: creative.ErrViralAnalysisSourceUnavailable, wantStatus: http.StatusUnprocessableEntity, wantCode: "VIRAL_ANALYSIS_SOURCE_UNAVAILABLE"},
		{name: "viral provider unavailable", err: creative.ErrViralAnalysisProviderUnavailable, wantStatus: http.StatusServiceUnavailable, wantCode: "VIRAL_ANALYSIS_PROVIDER_UNAVAILABLE", wantRetryable: true},
		{name: "viral invalid response", err: creative.ErrViralAnalysisResponseInvalid, wantStatus: http.StatusBadGateway, wantCode: "VIRAL_ANALYSIS_RESPONSE_INVALID", wantRetryable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/creative/v1/test", nil)

			(&Server{}).writeServiceError(response, request, tt.err)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, tt.wantStatus)
			}
			var problem struct {
				Error contract.Error `json:"error"`
			}
			if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
				t.Fatalf("decode problem: %v", err)
			}
			if problem.Error.Code != tt.wantCode {
				t.Fatalf("code = %q, want %q", problem.Error.Code, tt.wantCode)
			}
			if problem.Error.Retryable != tt.wantRetryable {
				t.Fatalf("retryable = %t, want %t", problem.Error.Retryable, tt.wantRetryable)
			}
		})
	}
}

func TestAuthenticatedDomainMountReceivesTrustedRequestContext(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"},
		Scopes:         []contract.Scope{"strategy.read"},
	}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	mount := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestContext, ok := contract.RequestContextFrom(request.Context())
		if !ok || requestContext.Actor.OrganizationID != actor.OrganizationID || requestContext.RequestID == "" {
			t.Fatal("domain mount did not receive trusted request context")
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	server := NewWithDependencies(Dependencies{
		Resolver: resolver,
		AuthenticatedDomainMounts: []DomainMount{{
			Pattern: "/api/strategy/v1/",
			Handler: mount,
		}},
	})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/strategy/v1/workspaces/workspace_1", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	denied := NewWithDependencies(Dependencies{
		Resolver: identity.RejectingResolver{},
		AuthenticatedDomainMounts: []DomainMount{{
			Pattern: "/api/strategy/v1/",
			Handler: mount,
		}},
	})
	unauthenticated := httptest.NewRecorder()
	denied.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/strategy/v1/workspaces/workspace_1", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", unauthenticated.Code)
	}
}

func TestGeneratedIntakeRouteRequiresScopeAndReturnsLocation(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"}, Scopes: []contract.Scope{"assets.write"}}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	server := NewWithDependencies(Dependencies{Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"}, Intakes: fakeIntakeManager{}})
	now := time.Now().UTC()
	requestBody := assets.GeneratedAssetIntakeRequest{ProviderJobID: "job_1", Output: contract.ProviderOutputRef{ProviderCode: "fake", ProviderJobID: "job_1", OutputID: "out_1", RetrievalExpiresAt: now.Add(time.Hour), DeclaredMIMEType: "image/png", DeclaredSizeBytes: 100}, Provenance: assets.GenerationProvenance{Capability: "image.generate", ProviderCode: "fake", ModelAlias: "standard", ModelVersion: "v1", SourceAssetRefs: []contract.AssetVersionRef{}, ProjectContextVersion: 1, GeneratedAt: now}}
	body, _ := json.Marshal(requestBody)
	request := httptest.NewRequest(http.MethodPost, "/platform/v1/projects/project_1/assets/generated-intakes", bytes.NewReader(body))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Location") != "/platform/v1/projects/project_1/assets/generated-intakes/intake_1" {
		t.Fatalf("location=%q", response.Header().Get("Location"))
	}
	responseBody := response.Body.String()
	for _, forbidden := range []string{"provider_code", "retrieval_expires_at", "declared_mime_type", "declared_size_bytes", "bucket", "object_key", "vendor"} {
		if strings.Contains(responseBody, forbidden) {
			t.Fatalf("generated intake response leaked %q: %s", forbidden, responseBody)
		}
	}

	actor.Scopes = []contract.Scope{}
	resolver, _ = identity.NewStaticResolver(actor)
	deniedServer := NewWithDependencies(Dependencies{Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"}, Intakes: fakeIntakeManager{}})
	denied := httptest.NewRecorder()
	deniedServer.ServeHTTP(denied, httptest.NewRequest(http.MethodPost, "/platform/v1/projects/project_1/assets/generated-intakes", bytes.NewReader(body)))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("scope denied status=%d", denied.Code)
	}
}

func TestRemoveProjectAssetRequiresWriteScope(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"}, Scopes: []contract.Scope{"assets.write"}}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	uploads := &fakeUploadManager{}
	server := NewWithDependencies(Dependencies{Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"}, Uploads: uploads})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/platform/v1/projects/project_1/assets/asset_1/versions/3", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if uploads.removed.AssetID != "asset_1" || uploads.removed.Version != 3 {
		t.Fatalf("removed=%#v", uploads.removed)
	}

	actor.Scopes = nil
	resolver, _ = identity.NewStaticResolver(actor)
	deniedServer := NewWithDependencies(Dependencies{Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"}, Uploads: &fakeUploadManager{}})
	denied := httptest.NewRecorder()
	deniedServer.ServeHTTP(denied, httptest.NewRequest(http.MethodDelete, "/platform/v1/projects/project_1/assets/asset_1/versions/3", nil))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("scope denied status=%d", denied.Code)
	}
}

func TestLocalAssetPreviewReturnsProtectedContentURL(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"}, Scopes: []contract.Scope{"assets.read"}}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	uploads := &fakeUploadManager{content: []byte("png-bytes"), mime: "image/png"}
	server := NewWithDependencies(Dependencies{Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"}, Uploads: uploads})

	preview := httptest.NewRecorder()
	server.ServeHTTP(preview, httptest.NewRequest(http.MethodGet, "/platform/v1/projects/project_1/assets/asset_1/versions/2/preview", nil))
	if preview.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", preview.Code, preview.Body.String())
	}
	var signed assets.SignedRequest
	if err := json.NewDecoder(preview.Body).Decode(&signed); err != nil {
		t.Fatal(err)
	}
	wantURL := "/platform/v1/projects/project_1/assets/asset_1/versions/2/content"
	if signed.URL != wantURL || signed.Method != http.MethodGet {
		t.Fatalf("signed request=%#v, want URL %q", signed, wantURL)
	}

	content := httptest.NewRecorder()
	server.ServeHTTP(content, httptest.NewRequest(http.MethodGet, signed.URL, nil))
	if content.Code != http.StatusOK || content.Body.String() != "png-bytes" {
		t.Fatalf("content status=%d body=%q", content.Code, content.Body.String())
	}
	if content.Header().Get("Content-Type") != "image/png" || content.Header().Get("Cache-Control") != "private, no-store" || content.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("unexpected content headers: %#v", content.Header())
	}

	actor.Scopes = nil
	resolver, _ = identity.NewStaticResolver(actor)
	deniedServer := NewWithDependencies(Dependencies{Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"}, Uploads: uploads})
	denied := httptest.NewRecorder()
	deniedServer.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, wantURL, nil))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("scope denied status=%d", denied.Code)
	}
}

func TestListAssetsReturnsMediaMetadata(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"}, Scopes: []contract.Scope{"assets.read"}}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	uploads := &fakeUploadManager{items: []assets.ProjectAsset{{
		Ref: contract.ProjectAssetRef{ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: "asset_1", Version: 1}},
		Asset: assets.Asset{
			ID: "asset_1", OrganizationID: "org_1", Kind: contract.AssetVideo, Status: assets.AssetReady,
			OwnerSystem: "assets", LatestVersion: 1, CreatedAt: now, UpdatedAt: now,
		},
		Version: assets.AssetVersion{
			OrganizationID: "org_1", AssetID: "asset_1", Version: 1, Status: assets.AssetReady,
			SourceType: contract.AssetSourceUpload, MIMEType: "video/mp4", SizeBytes: 1024, SHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			Media:     assets.MediaMetadata{DurationSeconds: 9.6, FPS: 30, Codec: "h264", ProbeStatus: assets.MediaProbeSucceeded},
			CreatedAt: now,
		},
		CreatedAt: now,
	}}}
	server := NewWithDependencies(Dependencies{Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"}, Uploads: uploads})

	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/platform/v1/projects/project_1/assets", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Items []assets.ProjectAsset `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.Items[0].Version.Media.DurationSeconds != 9.6 || body.Items[0].Version.Media.ProbeStatus != assets.MediaProbeSucceeded {
		t.Fatalf("media metadata missing from API response: %#v", body.Items)
	}
}

func TestAssetFeatureRoutesReadWriteAndDegradeMissingFeature(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"}, Scopes: []contract.Scope{"assets.read", "assets.write"}}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	uploads := &fakeUploadManager{}
	server := NewWithDependencies(Dependencies{Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"}, Uploads: uploads})
	payload := `{"schema_version":"asset_feature_v1","hook_strength":0.86,"product_visibility":0.74,"scene_tags":["factory"],"product_tags":["cnc"],"person_tags":["engineer"],"action_tags":["cutting"],"emotion_tags":["trust"],"selling_points":["0.01mm precision"],"cta_presence":true,"similarity_group":"precision-demo-a","similarity_risk":"medium","evidence":["00:00-00:03 strong hook"]}`

	put := httptest.NewRecorder()
	server.ServeHTTP(put, httptest.NewRequest(http.MethodPut, "/platform/v1/projects/project_1/assets/asset_1/versions/2/features/vlm-2026-07-26", bytes.NewBufferString(payload)))
	if put.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", put.Code, put.Body.String())
	}
	if uploads.feature.AssetID != "asset_1" || uploads.feature.AssetVersion != 2 || uploads.feature.ProjectID != "project_1" || uploads.feature.FeatureVersion != "vlm-2026-07-26" {
		t.Fatalf("feature scope not set from URL: %#v", uploads.feature)
	}

	get := httptest.NewRecorder()
	server.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/platform/v1/projects/project_1/assets/asset_1/versions/2/features/vlm-2026-07-26", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", get.Code, get.Body.String())
	}
	var getBody struct {
		Feature *assets.AssetFeature `json:"feature"`
	}
	if err := json.NewDecoder(get.Body).Decode(&getBody); err != nil {
		t.Fatal(err)
	}
	if getBody.Feature == nil || getBody.Feature.HookStrength != 0.86 || getBody.Feature.SimilarityRisk != assets.AssetFeatureRiskMedium {
		t.Fatalf("unexpected feature body: %#v", getBody.Feature)
	}

	list := httptest.NewRecorder()
	server.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/platform/v1/projects/project_1/assets/features?limit=10", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var listBody struct {
		Items []assets.AssetFeature `json:"items"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listBody); err != nil {
		t.Fatal(err)
	}
	if len(listBody.Items) != 1 || listBody.Items[0].SellingPoints[0] != "0.01mm precision" {
		t.Fatalf("unexpected list body: %#v", listBody)
	}

	missing := httptest.NewRecorder()
	server.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/platform/v1/projects/project_1/assets/asset_2/versions/1/features/missing", nil))
	if missing.Code != http.StatusOK || !strings.Contains(missing.Body.String(), `"feature":null`) {
		t.Fatalf("missing status=%d body=%s", missing.Code, missing.Body.String())
	}

	actor.Scopes = []contract.Scope{"assets.read"}
	resolver, _ = identity.NewStaticResolver(actor)
	deniedServer := NewWithDependencies(Dependencies{Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"}, Uploads: &fakeUploadManager{}})
	denied := httptest.NewRecorder()
	deniedServer.ServeHTTP(denied, httptest.NewRequest(http.MethodPut, "/platform/v1/projects/project_1/assets/asset_1/versions/2/features/vlm-2026-07-26", bytes.NewBufferString(payload)))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("scope denied status=%d", denied.Code)
	}
}

type fakeUploadManager struct {
	removed contract.AssetVersionRef
	content []byte
	items   []assets.ProjectAsset
	mime    string
	feature assets.AssetFeature
}

func (*fakeUploadManager) Create(context.Context, contract.RequestContext, contract.ProjectID, contract.IdempotencyKey, assets.CreateUploadRequest) (assets.CreateUploadResponse, error) {
	return assets.CreateUploadResponse{}, nil
}
func (*fakeUploadManager) PutContent(context.Context, contract.ActorContext, contract.ProjectID, string, io.Reader, int64) error {
	return nil
}
func (*fakeUploadManager) Finalize(context.Context, contract.RequestContext, contract.ProjectID, string) (assets.UploadSession, error) {
	return assets.UploadSession{}, nil
}
func (f *fakeUploadManager) List(context.Context, contract.ActorContext, contract.ProjectID, int) ([]assets.ProjectAsset, error) {
	return f.items, nil
}
func (*fakeUploadManager) Preview(context.Context, contract.ActorContext, contract.ProjectID, contract.AssetVersionRef) (assets.SignedRequest, error) {
	return assets.SignedRequest{Method: http.MethodGet}, nil
}
func (f *fakeUploadManager) OpenPreview(context.Context, contract.ActorContext, contract.ProjectID, contract.AssetVersionRef) (io.ReadCloser, assets.ObjectInfo, error) {
	return io.NopCloser(bytes.NewReader(f.content)), assets.ObjectInfo{SizeBytes: int64(len(f.content)), MIMEType: f.mime}, nil
}
func (f *fakeUploadManager) Remove(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, ref contract.AssetVersionRef) error {
	f.removed = ref
	return nil
}
func (f *fakeUploadManager) UpsertFeature(_ context.Context, actor contract.ActorContext, projectID contract.ProjectID, feature assets.AssetFeature) (assets.AssetFeature, error) {
	feature.OrganizationID = actor.OrganizationID
	feature.ProjectID = projectID
	feature.CreatedAt = time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	feature.UpdatedAt = feature.CreatedAt
	f.feature = feature
	return feature, nil
}
func (f *fakeUploadManager) GetFeature(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, ref contract.AssetVersionRef, featureVersion string) (assets.AssetFeature, error) {
	if f.feature.AssetID != ref.AssetID || f.feature.AssetVersion != ref.Version || f.feature.FeatureVersion != featureVersion {
		return assets.AssetFeature{}, assets.ErrNotFound
	}
	return f.feature, nil
}
func (f *fakeUploadManager) ListFeatures(context.Context, contract.ActorContext, contract.ProjectID, int) ([]assets.AssetFeature, error) {
	if f.feature.AssetID == "" {
		return nil, nil
	}
	return []assets.AssetFeature{f.feature}, nil
}

type fakeIntakeManager struct{}

func (fakeIntakeManager) Create(_ context.Context, rc contract.RequestContext, projectID contract.ProjectID, key contract.IdempotencyKey, request assets.GeneratedAssetIntakeRequest) (assets.GeneratedIntake, error) {
	return assets.GeneratedIntake{ID: "intake_1", OrganizationID: rc.Actor.OrganizationID, ProjectID: projectID, ProviderJobID: request.ProviderJobID, OutputID: request.Output.OutputID, ProviderCode: request.Output.ProviderCode, Status: assets.GeneratedIntakeQueued, IdempotencyKey: key, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}
func (fakeIntakeManager) Get(context.Context, contract.ActorContext, contract.ProjectID, string) (assets.GeneratedIntake, error) {
	return assets.GeneratedIntake{}, assets.ErrNotFound
}

type fakeRemixPlanManager struct {
	plan       remix.Plan
	render     remix.RenderJob
	quality    remix.QualityReport
	analysis   remix.HitAnalysis
	mapping    remix.ProductMapping
	preroll    remix.Preroll
	renderKey  contract.IdempotencyKey
	renderHash string
}

func (f *fakeRemixPlanManager) Create(_ context.Context, actor contract.ActorContext, projectID contract.ProjectID, request remix.CreatePlanRequest) (remix.Plan, error) {
	f.plan = remix.Plan{
		ID:             "remixplan_1",
		OrganizationID: actor.OrganizationID,
		ProjectID:      projectID,
		CreatedBy:      actor.Principal,
		SchemaVersion:  request.SchemaVersion,
		ClientPlanID:   request.ClientPlanID,
		TargetSeconds:  request.TargetSeconds,
		ActualSeconds:  request.ActualSeconds,
		Pace:           request.Pace,
		Segments:       request.Segments,
		Warnings:       request.Warnings,
		Summary:        request.Summary,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	return f.plan, nil
}

func (f *fakeRemixPlanManager) Get(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, id string) (remix.Plan, error) {
	if id != f.plan.ID {
		return remix.Plan{}, remix.ErrNotFound
	}
	return f.plan, nil
}

func (f *fakeRemixPlanManager) List(context.Context, contract.ActorContext, contract.ProjectID, int) ([]remix.Plan, error) {
	if f.plan.ID == "" {
		return nil, nil
	}
	return []remix.Plan{f.plan}, nil
}

func (f *fakeRemixPlanManager) CreateRenderJob(_ context.Context, actor contract.ActorContext, projectID contract.ProjectID, key contract.IdempotencyKey, request remix.CreateRenderJobRequest) (remix.RenderJob, error) {
	if request.PlanID != f.plan.ID {
		return remix.RenderJob{}, remix.ErrNotFound
	}
	hash, _ := contract.CanonicalJSONHash(request)
	if f.render.ID != "" {
		if f.renderKey == key && f.renderHash == hash {
			return f.render, nil
		}
		if f.renderKey == key {
			return remix.RenderJob{}, remix.ErrIdempotencyConflict
		}
	}
	f.renderKey = key
	f.renderHash = hash
	f.render = remix.RenderJob{
		ID:             "remixrender_1",
		OrganizationID: actor.OrganizationID,
		ProjectID:      projectID,
		PlanID:         request.PlanID,
		Status:         remix.RenderQueued,
		Progress:       0,
		TargetFormat:   "mp4",
		TargetQuality:  request.TargetQuality,
		IdempotencyKey: key,
		RequestHash:    hash,
		InputSnapshot:  remix.RenderInputSnapshot{Plan: f.plan, Request: request},
		CreatedBy:      actor.Principal,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	return f.render, nil
}

func (f *fakeRemixPlanManager) GetRenderJob(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, id string) (remix.RenderJob, error) {
	if id != f.render.ID {
		return remix.RenderJob{}, remix.ErrNotFound
	}
	return f.render, nil
}

func (f *fakeRemixPlanManager) CreateQualityReport(_ context.Context, actor contract.ActorContext, projectID contract.ProjectID, request remix.CreateQualityReportRequest) (remix.QualityReport, error) {
	if request.RenderJobID != f.render.ID {
		return remix.QualityReport{}, remix.ErrNotFound
	}
	now := time.Now().UTC()
	f.quality = remix.QualityReport{
		ID:             "qualityreport_1",
		OrganizationID: actor.OrganizationID,
		ProjectID:      projectID,
		RenderJobID:    request.RenderJobID,
		OutputAsset:    request.OutputAsset,
		Verdict:        remix.QualityVerdictMajor,
		Score:          0.64,
		Dimensions: []remix.QualityDimension{
			{Name: "aesthetics", Score: 0.58, Verdict: string(remix.QualityVerdictMajor), Summary: "字幕遮挡主体"},
		},
		Issues: []remix.QualityIssue{{
			Code:             "LOW_READABILITY",
			Severity:         remix.QualityVerdictMajor,
			Dimension:        "aesthetics",
			StartSeconds:     5,
			EndSeconds:       6.5,
			Description:      "字幕和商品主体重叠，major 质检要求人工复核",
			RepairSuggestion: "调整字幕安全区",
		}},
		Evidence:          []remix.QualityEvidence{{Kind: "vlm_frame", TimestampSec: 5.8, Summary: "fake VLM 检出字幕遮挡主体"}},
		RepairSuggestions: []string{"调整字幕安全区"},
		CreatedBy:         actor.Principal,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	f.render.Status = remix.RenderRequiresReview
	f.render.RequiresReview = true
	f.render.QualityReportID = f.quality.ID
	return f.quality, nil
}

func (f *fakeRemixPlanManager) GetQualityReport(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, id string) (remix.QualityReport, error) {
	if id != f.quality.ID {
		return remix.QualityReport{}, remix.ErrNotFound
	}
	return f.quality, nil
}

func (f *fakeRemixPlanManager) GetQualityReportForRenderJob(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, id string) (remix.QualityReport, error) {
	if id != f.quality.RenderJobID {
		return remix.QualityReport{}, remix.ErrNotFound
	}
	return f.quality, nil
}

func (f *fakeRemixPlanManager) CreateHitAnalysis(_ context.Context, actor contract.ActorContext, projectID contract.ProjectID, request remix.CreateHitAnalysisRequest) (remix.HitAnalysis, error) {
	f.analysis = remix.HitAnalysis{
		ID:             "hitanalysis_1",
		OrganizationID: actor.OrganizationID,
		ProjectID:      projectID,
		SourceAsset:    request.SourceAsset,
		Title:          request.Title,
		VideoMeta:      remix.HitVideoMeta{DurationSeconds: request.DurationSeconds, Language: request.Language},
		Segments:       []remix.HitSegment{{ID: "seg_1", StartSeconds: 0, EndSeconds: request.DurationSeconds, Role: remix.HitRoleHook}},
		CreatedBy:      actor.Principal,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	return f.analysis, nil
}

func (f *fakeRemixPlanManager) GetHitAnalysis(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, id string) (remix.HitAnalysis, error) {
	if id != f.analysis.ID {
		return remix.HitAnalysis{}, remix.ErrNotFound
	}
	return f.analysis, nil
}

func (f *fakeRemixPlanManager) CreateProductMapping(_ context.Context, actor contract.ActorContext, projectID contract.ProjectID, request remix.CreateProductMappingRequest) (remix.ProductMapping, error) {
	f.mapping = remix.ProductMapping{
		ID:               "productmapping_1",
		OrganizationID:   actor.OrganizationID,
		ProjectID:        projectID,
		HitAnalysisID:    request.HitAnalysisID,
		TargetProduct:    request.TargetProduct,
		RequiredAssets:   request.RequiredAssets,
		ReplacementRules: request.ReplacementRules,
		Constraints:      request.Constraints,
		TargetSeconds:    request.TargetSeconds,
		Pace:             request.Pace,
		CreatedBy:        actor.Principal,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
	return f.mapping, nil
}

func (f *fakeRemixPlanManager) GetProductMapping(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, id string) (remix.ProductMapping, error) {
	if id != f.mapping.ID {
		return remix.ProductMapping{}, remix.ErrNotFound
	}
	return f.mapping, nil
}

func (f *fakeRemixPlanManager) GeneratePlanFromProductMapping(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, id string) (remix.Plan, error) {
	if id != f.mapping.ID {
		return remix.Plan{}, remix.ErrNotFound
	}
	return f.plan, nil
}

func (f *fakeRemixPlanManager) CreatePreroll(_ context.Context, actor contract.ActorContext, projectID contract.ProjectID, request remix.CreatePrerollRequest) (remix.Preroll, error) {
	if request.PlanID != f.plan.ID {
		return remix.Preroll{}, remix.ErrNotFound
	}
	now := time.Now().UTC()
	f.preroll = remix.Preroll{
		ID:               "preroll_1",
		OrganizationID:   actor.OrganizationID,
		ProjectID:        projectID,
		PlanID:           request.PlanID,
		HookType:         request.HookType,
		ReferenceAsset:   request.ReferenceAsset,
		StyleConstraints: request.StyleConstraints,
		DurationSeconds:  request.DurationSeconds,
		Mode:             request.Mode,
		PromptDraft:      "为 opening 段生成冲突钩子",
		QualityVerdict:   remix.QualityVerdictPass,
		Status:           remix.PrerollReady,
		OutputAsset:      &contract.ProjectAssetRef{ProjectID: projectID, AssetVersion: contract.AssetVersionRef{AssetID: "preroll_asset", Version: 1}},
		CreatedBy:        actor.Principal,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	return f.preroll, nil
}

func (f *fakeRemixPlanManager) GetPreroll(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, id string) (remix.Preroll, error) {
	if id != f.preroll.ID {
		return remix.Preroll{}, remix.ErrNotFound
	}
	return f.preroll, nil
}

func (f *fakeRemixPlanManager) ApplyPreroll(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, id string) (remix.Plan, error) {
	if id != f.preroll.ID {
		return remix.Plan{}, remix.ErrNotFound
	}
	if f.preroll.Status != remix.PrerollReady {
		return remix.Plan{}, remix.ErrPrerollNotReady
	}
	f.plan.Warnings = append(f.plan.Warnings, "ai_preroll_applied")
	f.plan.ActualSeconds += f.preroll.DurationSeconds
	f.preroll.Status = remix.PrerollApplied
	return f.plan, nil
}

func (f *fakeRemixPlanManager) CreateFeedbackEvent(_ context.Context, actor contract.ActorContext, projectID contract.ProjectID, request remix.CreateFeedbackEventRequest) (remix.FeedbackEvent, error) {
	return remix.FeedbackEvent{ID: "feedback_1", OrganizationID: actor.OrganizationID, ProjectID: projectID, EventType: request.EventType, TargetType: request.TargetType, TargetID: request.TargetID, AssetVersion: request.AssetVersion, Rating: request.Rating, Comment: request.Comment, CreatedBy: actor.Principal, CreatedAt: time.Now().UTC()}, nil
}

func (f *fakeRemixPlanManager) ListFeedbackEvents(context.Context, contract.ActorContext, contract.ProjectID, remix.FeedbackEventFilter) ([]remix.FeedbackEvent, error) {
	return nil, nil
}

func (f *fakeRemixPlanManager) GetAssetPerformanceSnapshot(context.Context, contract.ActorContext, contract.ProjectID) ([]remix.AssetPerformance, error) {
	return nil, nil
}

func (f *fakeRemixPlanManager) CreatePlannerWeightSnapshot(_ context.Context, actor contract.ActorContext, projectID contract.ProjectID) (remix.PlannerWeightSnapshot, error) {
	return remix.PlannerWeightSnapshot{ID: "weights_1", OrganizationID: actor.OrganizationID, ProjectID: projectID, CreatedBy: actor.Principal, CreatedAt: time.Now().UTC()}, nil
}

func httpRemixSegment(segment remix.Segment, label string, assetID contract.AssetID) remix.SegmentPlan {
	return remix.SegmentPlan{
		Segment:       segment,
		Label:         label,
		TargetSeconds: 10,
		ActualSeconds: 3.2,
		Shots: []remix.Shot{{
			ID:           string(segment) + "_shot_1",
			Segment:      segment,
			Source:       remix.ShotSourceExistingAsset,
			AssetVersion: contract.AssetVersionRef{AssetID: assetID, Version: 1},
			Timeline:     remix.ShotTimeline{StartSeconds: 0, DurationSeconds: 3.2, InPointSeconds: 0, OutPointSeconds: 3.2},
			Creative:     remix.ShotCreative{ShotType: "close_up", Transition: "cut"},
			Planning:     remix.ShotPlanning{Score: 0.8, ReasonCodes: []string{"test"}, Reason: "test", Evidence: []string{"fixture"}},
			Risks:        []string{},
		}},
	}
}

func httpProductMappingRequest(analysisID string) remix.CreateProductMappingRequest {
	return remix.CreateProductMappingRequest{
		HitAnalysisID: analysisID,
		TargetProduct: remix.ProductProfile{
			Name:          "白域精工新品",
			SellingPoints: []string{"±0.01mm 精度", "98% 准时交付"},
			CTA:           "预约获取打样方案",
		},
		RequiredAssets: []contract.AssetVersionRef{
			{AssetID: "target_hook", Version: 1},
			{AssetID: "target_proof", Version: 1},
			{AssetID: "target_cta", Version: 1},
		},
		ReplacementRules: []remix.ReplacementRule{
			{Role: remix.HitRoleHook, TargetAsset: contract.AssetVersionRef{AssetID: "target_hook", Version: 1}, Message: "先展示交期风险反差"},
			{Role: remix.HitRoleProof, TargetAsset: contract.AssetVersionRef{AssetID: "target_proof", Version: 1}, Message: "用精度和产线证据替换原证明段"},
			{Role: remix.HitRoleCTA, TargetAsset: contract.AssetVersionRef{AssetID: "target_cta", Version: 1}, Message: "引导预约打样方案"},
		},
		Constraints:   []string{"不得复用原视频二进制"},
		TargetSeconds: 30,
		Pace:          remix.PaceBalanced,
	}
}

func SchemaVersionV2ForHTTPTest() string {
	return remix.SchemaVersionV2
}

type workflowProjectManager struct {
	staticProjectManager
	projectValue project.Project
	runtime      project.ProjectRuntime
	task         project.BusinessTask
	operations   []project.OperationalRecord
	changeSet    project.ChangeSet
	auditEvents  []project.AuditEvent
}

func (m *workflowProjectManager) GetDetail(context.Context, contract.ActorContext, contract.ProjectID) (project.ProjectDetail, error) {
	runtime := m.runtime
	if runtime.Code == "" {
		runtime = project.ProjectRuntime{
			Code:      string(m.projectValue.ID),
			Stage:     string(m.projectValue.Status),
			Progress:  60,
			Status:    "active",
			Owner:     "user:usr_1",
			Budget:    0,
			Currency:  "CNY",
			Timezone:  "Asia/Shanghai",
			UpdatedAt: m.projectValue.UpdatedAt,
		}
	}
	return project.ProjectDetail{
		Project:    m.projectValue,
		Runtime:    runtime,
		Artifacts:  []project.ProjectArtifactSummary{},
		Tasks:      m.tasks(),
		Operations: m.operations,
		ChangeSets: m.changeSets(),
	}, nil
}

func (m *workflowProjectManager) UpdateProject(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, request project.UpdateProjectRequest) (project.Project, error) {
	if request.ExpectedContextVersion != nil && *request.ExpectedContextVersion != m.projectValue.ProjectContextVersion {
		return project.Project{}, project.ErrVersionConflict
	}
	if request.Name != nil {
		m.projectValue.Name = *request.Name
	}
	if request.Industry != nil {
		m.projectValue.Industry = *request.Industry
	}
	if request.Brand != nil {
		m.runtime.Brand = *request.Brand
	}
	if request.Goal != nil {
		m.runtime.Goal = *request.Goal
	}
	m.projectValue.ProjectContextVersion++
	return m.projectValue, nil
}

func (m *workflowProjectManager) GetWorkbench(_ context.Context, _ contract.ActorContext, projectID contract.ProjectID) (project.Workbench, error) {
	return project.Workbench{Project: project.WorkbenchProject{ProjectID: string(projectID)}}, nil
}

func (m *workflowProjectManager) CreateBusinessTask(_ context.Context, actor contract.ActorContext, projectID contract.ProjectID, request project.CreateBusinessTaskRequest) (project.BusinessTask, error) {
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	m.task = project.BusinessTask{
		ID:                "task_1",
		OrganizationID:    actor.OrganizationID,
		ProjectID:         projectID,
		Type:              request.Type,
		Name:              request.Name,
		Objective:         request.Objective,
		Status:            project.BusinessTaskDraft,
		SourceTaskIDs:     request.SourceTaskIDs,
		SourceArtifactIDs: request.SourceArtifactIDs,
		OutputArtifactIDs: []string{},
		Version:           1,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	return m.task, nil
}

func (m *workflowProjectManager) ListBusinessTasks(context.Context, contract.ActorContext, contract.ProjectID) ([]project.BusinessTask, error) {
	return m.tasks(), nil
}

func (m *workflowProjectManager) GetBusinessTask(context.Context, contract.ActorContext, contract.ProjectID, string) (project.BusinessTask, error) {
	if m.task.ID == "" {
		return project.BusinessTask{}, project.ErrNotFound
	}
	return m.task, nil
}

func (m *workflowProjectManager) UpdateBusinessTask(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, _ string, request project.UpdateBusinessTaskRequest) (project.BusinessTask, error) {
	if request.Status != nil {
		m.task.Status = *request.Status
	}
	if request.OutputArtifactIDs != nil {
		m.task.OutputArtifactIDs = request.OutputArtifactIDs
	}
	m.task.Version = 2
	return m.task, nil
}

func (m *workflowProjectManager) CreateOperationalRecord(_ context.Context, actor contract.ActorContext, projectID contract.ProjectID, request project.UpsertOperationalRecordRequest) (project.OperationalRecord, error) {
	record := workflowOperation(actor.OrganizationID, projectID, "operation_1", request)
	m.operations = append(m.operations, record)
	return record, nil
}

func (m *workflowProjectManager) ListOperationalRecords(context.Context, contract.ActorContext, contract.ProjectID) ([]project.OperationalRecord, error) {
	return m.operations, nil
}

func (m *workflowProjectManager) GetOperationalRecord(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, id string) (project.OperationalRecord, error) {
	for _, record := range m.operations {
		if record.ID == id {
			return record, nil
		}
	}
	return project.OperationalRecord{}, project.ErrNotFound
}

func (m *workflowProjectManager) UpsertOperationalRecord(_ context.Context, actor contract.ActorContext, projectID contract.ProjectID, id string, request project.UpsertOperationalRecordRequest) (project.OperationalRecord, error) {
	record := workflowOperation(actor.OrganizationID, projectID, id, request)
	m.operations = append(m.operations, record)
	return record, nil
}

func (m *workflowProjectManager) CreateChangeSet(_ context.Context, actor contract.ActorContext, projectID contract.ProjectID, request project.CreateChangeSetRequest) (project.ChangeSet, error) {
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	m.changeSet = project.ChangeSet{
		ID:             "changeset_1",
		OrganizationID: actor.OrganizationID,
		ProjectID:      projectID,
		Name:           request.Name,
		Status:         project.ChangeSetDraft,
		ArtifactRefs:   request.ArtifactRefs,
		BudgetLimit:    request.BudgetLimit,
		AuditEvents:    []project.AuditEvent{},
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	m.appendAudit("change_set.created")
	m.changeSet.AuditEvents = m.auditEvents
	return m.changeSet, nil
}

func (m *workflowProjectManager) ListChangeSets(context.Context, contract.ActorContext, contract.ProjectID) ([]project.ChangeSet, error) {
	return m.changeSets(), nil
}

func (m *workflowProjectManager) GetChangeSet(context.Context, contract.ActorContext, contract.ProjectID, string) (project.ChangeSet, error) {
	if m.changeSet.ID == "" {
		return project.ChangeSet{}, project.ErrNotFound
	}
	m.changeSet.AuditEvents = m.auditEvents
	return m.changeSet, nil
}

func (m *workflowProjectManager) PreflightChangeSet(context.Context, contract.ActorContext, contract.ProjectID, string) (project.ChangeSet, error) {
	m.changeSet.Status = project.ChangeSetPreflightPassed
	m.changeSet.Preflight = &project.ChangeSetPreflight{Passed: true, Checks: []project.PreflightCheck{{Code: "ready_creative", Passed: true, Message: "ready", Repair: ""}}, CheckedAt: time.Date(2026, 7, 28, 10, 1, 0, 0, time.UTC)}
	m.appendAudit("change_set.preflight")
	m.changeSet.AuditEvents = m.auditEvents
	return m.changeSet, nil
}

func (m *workflowProjectManager) ApproveChangeSet(context.Context, contract.ActorContext, contract.ProjectID, string, project.ChangeSetApprovalRequest) (project.ChangeSet, error) {
	m.changeSet.Status = project.ChangeSetApproved
	m.appendAudit("change_set.approved")
	m.changeSet.AuditEvents = m.auditEvents
	return m.changeSet, nil
}

func (m *workflowProjectManager) ExecuteChangeSet(context.Context, contract.ActorContext, contract.ProjectID, string) (project.ChangeSet, error) {
	now := time.Date(2026, 7, 28, 10, 2, 0, 0, time.UTC)
	m.changeSet.Status = project.ChangeSetExecuted
	m.changeSet.Execution = &project.ChangeSetExecution{Simulated: true, Evidence: []project.ChangeSetEvidence{{Step: "simulate", Status: "ok", Message: "done", RecordedAt: now}}, ExecutedAt: now}
	m.appendAudit("change_set.executed")
	m.changeSet.AuditEvents = m.auditEvents
	return m.changeSet, nil
}

func (m *workflowProjectManager) RollbackChangeSet(context.Context, contract.ActorContext, contract.ProjectID, string, project.RollbackChangeSetRequest) (project.ChangeSet, error) {
	m.changeSet.Status = project.ChangeSetRolledBack
	m.changeSet.Rollback = &project.ChangeSetRollback{Simulated: true, Reason: "演示回滚", RolledBackAt: time.Date(2026, 7, 28, 10, 3, 0, 0, time.UTC)}
	m.appendAudit("change_set.rolled_back")
	m.changeSet.AuditEvents = m.auditEvents
	return m.changeSet, nil
}

func (m *workflowProjectManager) ListAuditEvents(context.Context, contract.ActorContext, contract.ProjectID) ([]project.AuditEvent, error) {
	return m.auditEvents, nil
}

func (m *workflowProjectManager) tasks() []project.BusinessTask {
	if m.task.ID == "" {
		return []project.BusinessTask{}
	}
	return []project.BusinessTask{m.task}
}

func (m *workflowProjectManager) changeSets() []project.ChangeSet {
	if m.changeSet.ID == "" {
		return []project.ChangeSet{}
	}
	m.changeSet.AuditEvents = m.auditEvents
	return []project.ChangeSet{m.changeSet}
}

func (m *workflowProjectManager) appendAudit(action string) {
	m.auditEvents = append(m.auditEvents, project.AuditEvent{
		ID:             fmt.Sprintf("audit_%d", len(m.auditEvents)+1),
		OrganizationID: "org_1",
		ProjectID:      "project_1",
		Actor:          "user:usr_1",
		Action:         action,
		EntityType:     project.AuditEntityChangeSet,
		EntityID:       "changeset_1",
		Metadata:       map[string]any{"source": "handler-test"},
		CreatedAt:      time.Date(2026, 7, 28, 10, len(m.auditEvents), 0, 0, time.UTC),
	})
}

func workflowOperation(organizationID contract.OrganizationID, projectID contract.ProjectID, id string, request project.UpsertOperationalRecordRequest) project.OperationalRecord {
	now := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	return project.OperationalRecord{
		ID:             id,
		OrganizationID: organizationID,
		ProjectID:      projectID,
		Kind:           request.Kind,
		Title:          request.Title,
		Status:         request.Status,
		OccurredAt:     request.OccurredAt,
		Fields:         request.Fields,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func TestContextFailsClosedWithoutTrustedIdentity(t *testing.T) {
	t.Parallel()
	server := New(identity.RejectingResolver{})
	response := httptest.NewRecorder()

	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/platform/v1/context", nil))

	if got, want := response.Code, http.StatusUnauthorized; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	var body contract.Problem
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if body.Error.Code != "UNAUTHENTICATED" || body.Error.RequestID == "" {
		t.Fatalf("unexpected problem: %#v", body)
	}
	if body.Error.Details == nil {
		t.Fatal("problem details must serialize as an empty array")
	}
}

func TestProjectProbeUsesSharedAuthenticationAndAuthorization(t *testing.T) {
	t.Parallel()
	resolver, err := identity.NewStaticResolver(contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"}})
	if err != nil {
		t.Fatal(err)
	}
	server := NewWithDependencies(Dependencies{Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"}})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/platform/v1/projects/project_1/context", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}

	denied := httptest.NewRecorder()
	server.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, "/platform/v1/projects/project_2/context", nil))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("denied status = %d", denied.Code)
	}
}

func TestContextReturnsTrustedTenantAndTrace(t *testing.T) {
	t.Parallel()
	resolver, err := identity.NewStaticResolver(contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"},
		Scopes:         []contract.Scope{"strategy.brief.read"},
	})
	if err != nil {
		t.Fatalf("NewStaticResolver() error = %v", err)
	}
	server := New(resolver)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/platform/v1/context", nil)
	request.Header.Set("X-Request-ID", "req_from_client")
	request.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00")

	server.ServeHTTP(response, request)

	if got, want := response.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	var body contract.RequestContext
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode context: %v", err)
	}
	if body.RequestID != "req_from_client" || body.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" || body.Actor.OrganizationID != "org_1" {
		t.Fatalf("unexpected context: %#v", body)
	}
}

func TestInvalidClientRequestIDIsNotReflected(t *testing.T) {
	t.Parallel()
	server := New(identity.RejectingResolver{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("X-Request-ID", "bad\r\nvalue")

	server.ServeHTTP(response, request)

	if got := response.Header().Get("X-Request-ID"); got == "bad\r\nvalue" || got == "" {
		t.Fatalf("unexpected request ID response header: %q", got)
	}
}

func TestCreateImageJobUsesTrustedActorAndResolvedProjectContext(t *testing.T) {
	t.Parallel()
	resolver, err := identity.NewStaticResolver(contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"},
		Scopes:         []contract.Scope{provider.ScopeJobCreate},
	})
	if err != nil {
		t.Fatal(err)
	}
	brandID := contract.BrandID("brand_1")
	jobs := &providerJobStub{job: providerJobForHTTPTest()}
	server := NewWithDependencies(Dependencies{
		Resolver:          resolver,
		ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"},
		Projects: staticProjectManager{context: contract.ProjectContext{
			OrganizationID: "org_1", ProjectID: "project_1", BrandID: &brandID, ProductIDs: []contract.ProductID{}, ProjectContextVersion: 7,
		}},
		ProviderJobs: jobs,
	})
	request := httptest.NewRequest(http.MethodPost, "/platform/v1/projects/project_1/model/jobs", bytes.NewBufferString(`{
		"capability":"image.generate",
		"model_alias":"cookies.image.standard",
		"input":{"prompt":"launch poster","width":1024,"height":1024},
		"project_context_version":7
	}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "create-image-1")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	if jobs.request.Actor.OrganizationID != "org_1" || jobs.request.Project.ProjectContextVersion != 7 || jobs.request.Input.Prompt != "launch poster" {
		t.Fatalf("unexpected Provider request: %+v", jobs.request)
	}
	if jobs.request.RequestHash == "" || jobs.request.IdempotencyKey != "create-image-1" {
		t.Fatalf("request hash or idempotency key missing: %+v", jobs.request)
	}
}

func TestCreateImageJobRejectsStaleProjectContext(t *testing.T) {
	t.Parallel()
	resolver, err := identity.NewStaticResolver(contract.ActorContext{
		OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"}, Scopes: []contract.Scope{provider.ScopeJobCreate},
	})
	if err != nil {
		t.Fatal(err)
	}
	brandID := contract.BrandID("brand_1")
	jobs := &providerJobStub{job: providerJobForHTTPTest()}
	server := NewWithDependencies(Dependencies{
		Resolver:          resolver,
		ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"},
		Projects: staticProjectManager{context: contract.ProjectContext{
			OrganizationID: "org_1", ProjectID: "project_1", BrandID: &brandID, ProductIDs: []contract.ProductID{}, ProjectContextVersion: 7,
		}},
		ProviderJobs: jobs,
	})
	request := httptest.NewRequest(http.MethodPost, "/platform/v1/projects/project_1/model/jobs", bytes.NewBufferString(`{"capability":"image.generate","model_alias":"cookies.image.standard","input":{"prompt":"launch poster","width":1024,"height":1024},"project_context_version":6}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "create-image-1")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusConflict || jobs.createCalls != 0 {
		t.Fatalf("status = %d create_calls=%d body=%s", response.Code, jobs.createCalls, response.Body.String())
	}
}

func TestCreativeImageTextSlotGenerationUsesFrozenPromptAndPortraitProfile(t *testing.T) {
	t.Parallel()
	resolver, err := identity.NewStaticResolver(contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"},
		Scopes:         []contract.Scope{creative.ScopeWrite, provider.ScopeJobCreate},
	})
	if err != nil {
		t.Fatal(err)
	}
	brandID := contract.BrandID("brand_1")
	jobs := &providerJobStub{job: providerJobForHTTPTest()}
	manager := &creativeManagerStub{
		imagePrompt: creative.ImagePromptPackage{
			ID: "prompt_1", CompiledPrompt: "frozen portrait prompt",
			SourceAssetRefs: []contract.AssetVersionRef{{AssetID: "asset_product", Version: 2}},
		},
		imageAttempt: creative.ImageGenerationAttempt{
			ID: "attempt_1", TaskID: "task_1", DraftRevision: 4, RequestHash: "sha256:request",
			GenerationSpec: creative.DefaultImageGenerationSpec("cookies.image.standard"),
			Status:         creative.ImageAttemptQueued,
		},
	}
	server := NewWithDependencies(Dependencies{
		Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"},
		Projects: staticProjectManager{context: contract.ProjectContext{
			OrganizationID: "org_1", ProjectID: "project_1", BrandID: &brandID,
			ProductIDs: []contract.ProductID{}, ProjectContextVersion: 7,
		}},
		Creative: manager, ProviderJobs: jobs,
	})
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/creative/v1/projects/project_1/creative-tasks/task_1/image-slots/2:generate",
		bytes.NewBufferString(`{"expected_task_version":3,"draft_revision":4}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "image-slot-http-1")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	if manager.preparedImageOrder != 2 || manager.attachedImageProviderJobID != jobs.job.ID {
		t.Fatalf("Creative image calls were not linked: manager=%+v", manager)
	}
	if jobs.request.Input.Width != creative.ImageTextSourceWidth ||
		jobs.request.Input.Height != creative.ImageTextSourceHeight ||
		jobs.request.Operation != "image.edit" ||
		jobs.request.Input.Prompt != "frozen portrait prompt" ||
		len(jobs.request.Input.SourceAssets) != 1 ||
		jobs.request.Input.PromptRef == nil ||
		jobs.request.Input.PromptRef.ID != "prompt_1" {
		t.Fatalf("unexpected Provider image input: %+v", jobs.request.Input)
	}
}

func TestCreateVideoJobUsesProviderVideoSeam(t *testing.T) {
	t.Parallel()
	resolver, err := identity.NewStaticResolver(contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"},
		Scopes:         []contract.Scope{provider.ScopeJobCreate},
	})
	if err != nil {
		t.Fatal(err)
	}
	brandID := contract.BrandID("brand_1")
	jobs := &providerJobStub{job: providerJobForHTTPTest()}
	jobs.job.Kind = "provider.video.generate"
	server := NewWithDependencies(Dependencies{
		Resolver:          resolver,
		ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"},
		Projects: staticProjectManager{context: contract.ProjectContext{
			OrganizationID: "org_1", ProjectID: "project_1", BrandID: &brandID, ProductIDs: []contract.ProductID{}, ProjectContextVersion: 7,
		}},
		ProviderJobs: jobs,
	})
	request := httptest.NewRequest(http.MethodPost, "/platform/v1/projects/project_1/model/jobs", bytes.NewBufferString(`{
		"capability":"video.generate",
		"model_alias":"cookies.video.standard",
		"input":{"prompt":"five-second product pre-roll","duration_seconds":5,"aspect_ratio":"9:16","resolution":"720p"},
		"project_context_version":7,
		"source_system":"creative",
		"source_task_id":"creative_task_1"
	}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "create-video-1")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	if jobs.videoRequest.Input.DurationSeconds != 5 || jobs.videoRequest.Input.AspectRatio != "9:16" || jobs.videoRequest.SourceTaskID != "creative_task_1" {
		t.Fatalf("unexpected Provider video request: %+v", jobs.videoRequest)
	}
}

func TestGetViralRemakeWorkspaceRestoresPersistedDraft(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"},
		Scopes:         []contract.Scope{creative.ScopeRead},
	}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 28, 10, 0, 0, 0, time.UTC)
	manager := &creativeManagerStub{detail: creative.TaskDetail{
		Task: creative.CreativeTask{
			ID: "creative_task_viral", OrganizationID: "org_1", ProjectID: "project_1",
			Format: creative.FormatVideo, PerformanceMode: creative.PerformanceModeViralRemake,
		},
		VideoDraft: &creative.VideoDraft{
			ContractVersion: "creative-video-draft/v1", TaskID: "creative_task_viral", Revision: 1,
			Concept: "viral", Prompt: "pending analysis", DurationSeconds: 15, AspectRatio: "9:16",
			Resolution: "720p", SourceVideo: contract.AssetVersionRef{AssetID: "asset_video", Version: 1},
			Mandatory: []string{}, Prohibited: []string{}, CreatedAt: now,
			ViralRemake: &creative.ViralRemakeDraft{
				ContractVersion: "creative-viral-remake-draft/v1", TaskID: "creative_task_viral", Revision: 1,
				Status: "waiting_for_analysis", SelectedRouteID: creative.ManualViralRemakeRouteID,
				InputSnapshot: creative.ViralRemakeInputSnapshot{
					Source: creative.IntakeSourceManual, SelectedRouteID: creative.ManualViralRemakeRouteID,
					ReferenceVideo: contract.AssetVersionRef{AssetID: "asset_video", Version: 1},
				},
				InputHash: "sha256:test", Readiness: creative.CreativeReadiness{
					PlanningReady: true, MissingFields: []string{}, Blockers: []string{"analysis_snapshot"},
				},
				CreatedAt: now, UpdatedAt: now,
			},
		},
	}}
	server := NewWithDependencies(Dependencies{
		Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"},
		Creative: manager,
	})
	request := httptest.NewRequest(http.MethodGet, "/api/creative/v1/projects/project_1/creative-tasks/creative_task_viral/viral-remake", nil)
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"selected_route_id":"route_manual_viral_remake_v1"`) {
		t.Fatalf("workspace body = %s", response.Body.String())
	}
}

func TestRegenerateShortDramaCandidatesForwardsVersionedConfig(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"},
		Scopes:         []contract.Scope{creative.ScopeRead, creative.ScopeWrite},
	}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	manager := &creativeManagerStub{detail: creative.TaskDetail{
		Task: creative.CreativeTask{
			ID: "creative_task_short_drama", OrganizationID: "org_1", ProjectID: "project_1",
			Format: creative.FormatVideo, PerformanceMode: creative.PerformanceModeShortDramaPreroll,
		},
		VideoDraft: &creative.VideoDraft{
			ContractVersion: "creative-video-draft/v1", TaskID: "creative_task_short_drama", Revision: 3,
			Concept: "独立六秒短剧引流前贴", Prompt: "候选", DurationSeconds: 6, AspectRatio: "9:16", Resolution: "720p",
			ShortDramaPreroll: &creative.ShortDramaPrerollDraft{
				ContractVersion: "creative-short-drama-preroll-draft/v1", TaskID: "creative_task_short_drama", Revision: 3,
			},
		},
	}}
	server := NewWithDependencies(Dependencies{
		Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"},
		Creative: manager,
	})
	body := bytes.NewBufferString(`{
		"expected_revision": 2,
		"generation_config": {
			"subtitle_style": "brand_minimal",
			"hook_strength": 5,
			"pace_profile": "auto"
		},
		"variation_intent": "more_visual"
	}`)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/creative/v1/projects/project_1/creative-tasks/creative_task_short_drama/short-drama-preroll:regenerate-candidates",
		body,
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "short-drama-regenerate-1")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	if manager.regenerateTaskID != "creative_task_short_drama" ||
		manager.regenerateRequest.ExpectedRevision != 2 ||
		manager.regenerateRequest.GenerationConfig.SubtitleStyle != "brand_minimal" ||
		manager.regenerateRequest.GenerationConfig.HookStrength != 5 {
		t.Fatalf("regenerate request was not forwarded: task=%q request=%+v", manager.regenerateTaskID, manager.regenerateRequest)
	}
}

func TestRegenerateGamePrerollCandidatesForwardsVersionedConfig(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"},
		Scopes:         []contract.Scope{creative.ScopeRead, creative.ScopeWrite},
	}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	manager := &creativeManagerStub{detail: creative.TaskDetail{
		Task: creative.CreativeTask{
			ID: "creative_task_game", OrganizationID: "org_1", ProjectID: "project_1",
			Format: creative.FormatVideo, PerformanceMode: creative.PerformanceModeGamePreroll,
		},
	}}
	server := NewWithDependencies(Dependencies{
		Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"},
		Creative: manager,
	})
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/creative/v1/projects/project_1/creative-tasks/creative_task_game/game-preroll:regenerate-candidates",
		bytes.NewBufferString(`{
			"expected_revision": 3,
			"generation_config": {
				"subtitle_style": "high_contrast_dynamic",
				"hook_strength": 4,
				"pace_profile": "punchy"
			}
		}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "game-preroll-regenerate-1")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	if manager.regenerateGameTaskID != "creative_task_game" ||
		manager.regenerateGameRequest.ExpectedRevision != 3 ||
		manager.regenerateGameRequest.GenerationConfig.HookStrength != 4 {
		t.Fatalf("game regenerate request was not forwarded: task=%q request=%+v", manager.regenerateGameTaskID, manager.regenerateGameRequest)
	}
}

func TestCreateShortDramaIntakeAcceptsInitialPaceProfile(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"},
		Scopes:         []contract.Scope{creative.ScopeRead, creative.ScopeWrite},
	}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	manager := &creativeManagerStub{}
	server := NewWithDependencies(Dependencies{
		Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"},
		Creative: manager,
	})
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/creative/v1/projects/project_1/creative-intakes",
		bytes.NewBufferString(`{
			"source":"manual",
			"format":"video",
			"performance_mode":"short_drama_preroll",
			"channel":"douyin",
			"objective":"引导观看短剧",
			"audience":"悬疑短剧观众",
			"core_message":"关键证词与事故记录矛盾",
			"call_to_action":"点击揭开真相",
			"concept":"独立六秒短剧引流前贴",
			"tone":["悬念"],
			"visual_keywords":["信息缺口"],
			"mandatory_elements":[],
			"prohibited_claims":[],
			"creative_routes":[],
			"manual_short_drama_preroll":{
				"brief_id":"brief_suspense",
				"brief_version":1,
				"brief_name":"悬疑真相",
				"story_title":"消失的第七份证词",
				"synopsis":"林夏整理父亲遗物时发现一份未出现在案件卷宗里的录音，录音时间与六年前事故记录完全矛盾，真正隐瞒秘密的人似乎已经找到她。",
				"reviewed_selling_points":["关键录音出现"],
				"hook_strategy":"suspense_reveal",
				"subtitle_style":"high_contrast_dynamic",
				"transition":"hard_cut",
				"hook_strength":4,
				"pace_profile":"suspense_hold",
				"character_references":[]
			}
		}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "short-drama-initial-pace-1")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	if manager.createdIntakeRequest.ManualShortDramaPreroll == nil ||
		manager.createdIntakeRequest.ManualShortDramaPreroll.PaceProfile != "suspense_hold" {
		t.Fatalf("pace profile was not decoded: %#v", manager.createdIntakeRequest.ManualShortDramaPreroll)
	}
}

func TestGetLatestShortDramaWorkspaceRestoresThePersistedTask(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"},
		Scopes:         []contract.Scope{creative.ScopeRead},
	}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	manager := &creativeManagerStub{detail: creative.TaskDetail{
		Task: creative.CreativeTask{
			ID: "creative_task_short_drama", OrganizationID: "org_1", ProjectID: "project_1",
			Format: creative.FormatVideo, PerformanceMode: creative.PerformanceModeShortDramaPreroll,
		},
		VideoDraft: &creative.VideoDraft{
			ContractVersion: "creative-video-draft/v1", TaskID: "creative_task_short_drama", Revision: 4,
			Concept: "独立六秒短剧引流前贴", Prompt: "候选", DurationSeconds: 6, AspectRatio: "9:16", Resolution: "720p",
			ShortDramaPreroll: &creative.ShortDramaPrerollDraft{
				ContractVersion: "creative-short-drama-preroll-draft/v1", TaskID: "creative_task_short_drama", Revision: 4,
				SelectedCandidateID: "candidate_3",
			},
		},
	}}
	server := NewWithDependencies(Dependencies{
		Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"},
		Creative: manager,
	})
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/creative/v1/projects/project_1/creative-workspaces/short-drama-preroll",
		nil,
	)
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"id":"creative_task_short_drama"`) ||
		!strings.Contains(response.Body.String(), `"selected_candidate_id":"candidate_3"`) {
		t.Fatalf("workspace body = %s", response.Body.String())
	}
	if manager.latestShortDramaProjectID != "project_1" {
		t.Fatalf("latest short drama workspace project = %q", manager.latestShortDramaProjectID)
	}
}

func TestGetLatestAINativeWorkspaceRestoresThePersistedStage(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"},
		Scopes:         []contract.Scope{creative.ScopeRead},
	}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	manager := &creativeManagerStub{latestAINativeWorkspace: creative.AINativeRequirementWorkspace{
		WorkspaceID: "ainativeworkspace_1", OrganizationID: "org_1", ProjectID: "project_1", CurrentStage: creative.AINativeStageStoryboard,
	}}
	server := NewWithDependencies(Dependencies{
		Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"},
		Creative: manager,
	})
	request := httptest.NewRequest(http.MethodGet, "/api/creative/v1/projects/project_1/ai-native-ads:latest", nil)
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"workspace_id":"ainativeworkspace_1"`) ||
		!strings.Contains(response.Body.String(), `"current_stage":"storyboard"`) {
		t.Fatalf("workspace body = %s", response.Body.String())
	}
	if manager.latestAINativeProjectID != "project_1" {
		t.Fatalf("latest AI native workspace project = %q", manager.latestAINativeProjectID)
	}
}

func TestGamePrerollWorkspaceRestoresAndForwardsHumanSelection(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"},
		Scopes:         []contract.Scope{creative.ScopeRead, creative.ScopeWrite},
	}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 30, 16, 0, 0, 0, time.UTC)
	manager := &creativeManagerStub{detail: creative.TaskDetail{
		Task: creative.CreativeTask{
			ID: "creative_task_game", OrganizationID: "org_1", ProjectID: "project_1",
			Format: creative.FormatVideo, PerformanceMode: creative.PerformanceModeGamePreroll,
		},
		VideoDraft: &creative.VideoDraft{
			ContractVersion: "creative-video-draft/v1", TaskID: "creative_task_game", Revision: 2,
			Concept: "保卫向日葵技能选择挑战", Prompt: "已编译 Prompt", DurationSeconds: 6,
			AspectRatio: "9:16", Resolution: "720p",
			SourceVideo: contract.AssetVersionRef{AssetID: "asset_gameplay", Version: 1},
			Mandatory:   []string{}, Prohibited: []string{}, CreatedAt: now,
			GamePreroll: &creative.GamePrerollDraft{
				ContractVersion: "creative-game-preroll-draft/v1", TaskID: "creative_task_game",
				Revision: 2, SelectedRouteID: creative.ManualGamePrerollRouteID,
				InputHash: "sha256:test", Candidates: []creative.GamePrerollCandidate{
					{ID: "game_candidate_1"},
					{ID: "game_candidate_2"},
					{ID: "game_candidate_3"},
				},
				SelectedCandidateID: "game_candidate_1",
				CreatedAt:           now, UpdatedAt: now,
			},
		},
	}}
	server := NewWithDependencies(Dependencies{
		Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"},
		Creative: manager,
	})

	getRequest := httptest.NewRequest(
		http.MethodGet,
		"/api/creative/v1/projects/project_1/creative-workspaces/game-preroll",
		nil,
	)
	getResponse := httptest.NewRecorder()
	server.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("get status = %d, body=%s", getResponse.Code, getResponse.Body.String())
	}
	if !strings.Contains(getResponse.Body.String(), `"selected_candidate_id":"game_candidate_1"`) ||
		manager.latestGamePrerollProjectID != "project_1" {
		t.Fatalf("restored game workspace = %s, project=%q", getResponse.Body.String(), manager.latestGamePrerollProjectID)
	}

	selectRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/creative/v1/projects/project_1/creative-tasks/creative_task_game/game-preroll:select-candidate",
		bytes.NewBufferString(`{"expected_revision":2,"candidate_id":"game_candidate_2"}`),
	)
	selectRequest.Header.Set("Content-Type", "application/json")
	selectRequest.Header.Set("Idempotency-Key", "game-select-1")
	selectResponse := httptest.NewRecorder()
	server.ServeHTTP(selectResponse, selectRequest)
	if selectResponse.Code != http.StatusOK {
		t.Fatalf("select status = %d, body=%s", selectResponse.Code, selectResponse.Body.String())
	}
	if manager.selectedGameTaskID != "creative_task_game" ||
		manager.selectedGameRequest.ExpectedRevision != 2 ||
		manager.selectedGameRequest.CandidateID != "game_candidate_2" {
		t.Fatalf("game selection was not forwarded: task=%q request=%+v", manager.selectedGameTaskID, manager.selectedGameRequest)
	}
}

func TestShortDramaVideoJobRegistersAVersionedGenerationAttempt(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"},
		Scopes:         []contract.Scope{creative.ScopeRead, creative.ScopeWrite, provider.ScopeJobCreate},
	}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	manager := &creativeManagerStub{detail: creative.TaskDetail{
		Task: creative.CreativeTask{
			ID: "creative_task_short_drama", OrganizationID: "org_1", ProjectID: "project_1",
			Format: creative.FormatVideo, PerformanceMode: creative.PerformanceModeShortDramaPreroll,
		},
		VideoDraft: &creative.VideoDraft{
			ContractVersion: "creative-video-draft/v1", TaskID: "creative_task_short_drama", Revision: 5,
			Concept: "独立六秒短剧引流前贴", Prompt: "已批准候选", DurationSeconds: 6, AspectRatio: "9:16", Resolution: "720p",
			ShortDramaPreroll: &creative.ShortDramaPrerollDraft{
				ContractVersion: "creative-short-drama-preroll-draft/v1", TaskID: "creative_task_short_drama", Revision: 5,
				SelectedCandidateID: "candidate_3",
			},
		},
	}}
	jobs := &providerJobStub{job: providerJobForHTTPTest()}
	brandID := contract.BrandID("brand_1")
	server := NewWithDependencies(Dependencies{
		Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"},
		Creative: manager, ProviderJobs: jobs,
		Projects: staticProjectManager{context: contract.ProjectContext{
			OrganizationID: "org_1", ProjectID: "project_1", BrandID: &brandID,
			ProductIDs: []contract.ProductID{}, ProjectContextVersion: 7,
		}},
	})
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/creative/v1/projects/project_1/creative-tasks/creative_task_short_drama:video-job",
		bytes.NewBufferString(`{}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "short-drama-video-1")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	if manager.registeredShortDramaProviderJobID != jobs.job.ID || manager.registeredProviderJobID != "" {
		t.Fatalf(
			"short drama job registered through wrong lineage seam: short=%q generic=%q",
			manager.registeredShortDramaProviderJobID, manager.registeredProviderJobID,
		)
	}
}

func TestGamePrerollVideoJobUsesSelectedPromptAndRegistersGenerationAttempt(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"},
		Scopes:         []contract.Scope{creative.ScopeRead, creative.ScopeWrite, provider.ScopeJobCreate},
	}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	manager := &creativeManagerStub{detail: creative.TaskDetail{
		Task: creative.CreativeTask{
			ID: "creative_task_game", OrganizationID: "org_1", ProjectID: "project_1",
			Format: creative.FormatVideo, PerformanceMode: creative.PerformanceModeGamePreroll,
		},
		VideoDraft: &creative.VideoDraft{
			ContractVersion: "creative-video-draft/v1", TaskID: "creative_task_game", Revision: 3,
			Concept: "保卫向日葵", Prompt: "server compiled", DurationSeconds: 6,
			AspectRatio: "9:16", Resolution: "720p",
			SourceVideo: contract.AssetVersionRef{AssetID: "asset_gameplay", Version: 1},
			GamePreroll: &creative.GamePrerollDraft{
				ContractVersion: "creative-game-preroll-draft/v1", TaskID: "creative_task_game",
				Revision: 3, SelectedCandidateID: "game_candidate_2",
			},
		},
	}}
	jobs := &providerJobStub{job: providerJobForHTTPTest()}
	brandID := contract.BrandID("brand_1")
	server := NewWithDependencies(Dependencies{
		Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"},
		Creative: manager, ProviderJobs: jobs,
		Projects: staticProjectManager{context: contract.ProjectContext{
			OrganizationID: "org_1", ProjectID: "project_1", BrandID: &brandID,
			ProductIDs: []contract.ProductID{}, ProjectContextVersion: 7,
		}},
	})
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/creative/v1/projects/project_1/creative-tasks/creative_task_game:video-job",
		bytes.NewBufferString(`{"model_alias":"cookies.video.standard"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "game-video-1")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	if jobs.videoRequest.ModelAlias != "cookies.video.standard" ||
		manager.registeredGamePrerollProviderJobID != jobs.job.ID ||
		manager.registeredProviderJobID != "" {
		t.Fatalf(
			"game job route mismatch: model=%q game=%q generic=%q",
			jobs.videoRequest.ModelAlias,
			manager.registeredGamePrerollProviderJobID,
			manager.registeredProviderJobID,
		)
	}
}

func TestCreativeTaskStrategyReadEndpointsRestoreHandoffAndCapabilities(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"},
		Scopes:         []contract.Scope{creative.ScopeRead},
	}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	manager := &creativeManagerStub{intake: creative.CreativeIntake{
		ID: "creativeintake_task_strategy_1", OrganizationID: "org_1", ProjectID: "project_1",
		Source: creative.IntakeSourceTaskStrategy, Status: creative.IntakeReady,
		Request: creative.CreateIntakeRequest{
			Source: creative.IntakeSourceTaskStrategy,
			TaskStrategy: &creative.TaskStrategyReference{
				PlanID: "creativeplan_1", StrategyVersion: 2, ExpectedContentHash: "sha256:task",
			},
			TaskStrategyInput: &creative.TaskStrategyInput{
				ContractVersion: creative.TaskStrategyContractVersion,
				BusinessCode:    creative.BusinessCommercePreroll,
			},
		},
	}}
	server := NewWithDependencies(Dependencies{
		Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"},
		Creative: manager,
	})

	capabilities := httptest.NewRecorder()
	server.ServeHTTP(capabilities, httptest.NewRequest(
		http.MethodGet, "/api/creative/v1/projects/project_1/business-capabilities", nil,
	))
	if capabilities.Code != http.StatusOK ||
		!strings.Contains(capabilities.Body.String(), `"business_code":"commerce_preroll"`) ||
		!strings.Contains(capabilities.Body.String(), `"status":"available"`) {
		t.Fatalf("capabilities status=%d body=%s", capabilities.Code, capabilities.Body.String())
	}

	intake := httptest.NewRecorder()
	server.ServeHTTP(intake, httptest.NewRequest(
		http.MethodGet,
		"/api/creative/v1/projects/project_1/creative-intakes/creativeintake_task_strategy_1",
		nil,
	))
	if intake.Code != http.StatusOK ||
		!strings.Contains(intake.Body.String(), `"source":"task_strategy"`) ||
		!strings.Contains(intake.Body.String(), `"strategy_version":2`) {
		t.Fatalf("intake status=%d body=%s", intake.Code, intake.Body.String())
	}
}

func TestCreativeVideoJobRequiresAndMapsApprovedFirstLastFrameSpec(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{
		OrganizationID: "org_1",
		Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"},
		Scopes:         []contract.Scope{creative.ScopeRead, creative.ScopeWrite, provider.ScopeJobCreate},
	}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := (creative.CommercePrerollPlanner{}).Plan(creative.CommercePrerollPlanningInput{
		TaskID: "creative_task_1", IntakeVersion: 1,
		TemplateID: creative.CommerceWindowRevealTemplateID, TemplateVersion: 1,
		BrandName: "Guerlain", ProductName: "Abeille Royale",
		ProductAsset:    contract.AssetVersionRef{AssetID: "asset_product", Version: 1},
		DurationSeconds: 6, AspectRatio: "9:16", Resolution: "720p",
		AudioPolicy: creative.VideoAudioSilent,
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	spec, err := plan.BindFrames(creative.ConditionedFrames{
		StartFrame: contract.AssetVersionRef{AssetID: "asset_first", Version: 1},
		TailFrame:  contract.AssetVersionRef{AssetID: "asset_last", Version: 1},
	})
	if err != nil {
		t.Fatalf("BindFrames() error = %v", err)
	}
	approval, err := creative.ApproveVideoGeneration(spec, actor.Principal.ID, time.Date(2026, time.July, 28, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ApproveVideoGeneration() error = %v", err)
	}
	body, err := json.Marshal(creative.CreateVideoJobRequest{
		ModelAlias:     "cookies.video.standard",
		Prompt:         &plan.Prompt,
		GenerationSpec: &spec,
		Approval:       &approval,
	})
	if err != nil {
		t.Fatal(err)
	}
	brandID := contract.BrandID("brand_1")
	jobs := &providerJobStub{job: providerJobForHTTPTest()}
	jobs.job.Kind = "provider.video.generate"
	creativeManager := &creativeManagerStub{detail: creative.TaskDetail{
		Task: creative.CreativeTask{ID: "creative_task_1", OrganizationID: "org_1", ProjectID: "project_1", Format: creative.FormatVideo},
		VideoDraft: &creative.VideoDraft{
			TaskID: "creative_task_1", Prompt: "legacy prompt", DurationSeconds: 5, AspectRatio: "9:16", Resolution: "720p",
		},
	}}
	server := NewWithDependencies(Dependencies{
		Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"},
		Creative: creativeManager, ProviderJobs: jobs,
		Projects: staticProjectManager{context: contract.ProjectContext{
			OrganizationID: "org_1", ProjectID: "project_1", BrandID: &brandID,
			ProductIDs: []contract.ProductID{}, ProjectContextVersion: 7,
		}},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/creative/v1/projects/project_1/creative-tasks/creative_task_1:video-job", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "creative-video-approved-1")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	input := jobs.videoRequest.Input
	if input.Prompt != plan.Prompt.CompiledPrompt ||
		input.InputMode != provider.VideoInputFirstLastFrame ||
		input.AudioPolicy != provider.VideoAudioSilent ||
		len(input.ConditioningAssets) != 2 {
		t.Fatalf("approved provider video input = %+v", input)
	}
	if creativeManager.registeredProviderJobID != jobs.job.ID {
		t.Fatalf("registered provider job = %q", creativeManager.registeredProviderJobID)
	}
}

func TestCreativeCoverJobKeepsCreativeTaskLineage(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"}, Scopes: []contract.Scope{creative.ScopeRead, creative.ScopeWrite, provider.ScopeJobCreate}}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	brandID := contract.BrandID("brand_1")
	jobs := &providerJobStub{job: providerJobForHTTPTest()}
	creativeManager := &creativeManagerStub{detail: creative.TaskDetail{
		Task:  creative.CreativeTask{ID: "creative_task_1", OrganizationID: "org_1", ProjectID: "project_1", Direction: creative.CreativeDirection{Tone: []string{"克制"}}},
		Draft: creative.ImageTextDraft{CoverCopy: "从容开始", ImagePlan: []creative.ImagePlanItem{{Order: 1, VisualBrief: "晨光中的咖啡桌"}}},
	}}
	server := NewWithDependencies(Dependencies{
		Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"}, Creative: creativeManager, ProviderJobs: jobs,
		Projects: staticProjectManager{context: contract.ProjectContext{OrganizationID: "org_1", ProjectID: "project_1", BrandID: &brandID, ProductIDs: []contract.ProductID{}, ProjectContextVersion: 7}},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/creative/v1/projects/project_1/creative-tasks/creative_task_1:cover-image-job", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "creative-cover-1")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if jobs.request.SourceSystem != "creative" || jobs.request.SourceTaskID != "creative_task_1" {
		t.Fatalf("Provider source=%q task=%q", jobs.request.SourceSystem, jobs.request.SourceTaskID)
	}
	if creativeManager.registeredProviderJobID != "provider_job_1" {
		t.Fatalf("registered provider job=%q", creativeManager.registeredProviderJobID)
	}
}

func TestFreezeCreativeVersionUsesIdempotencyKey(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"}, Scopes: []contract.Scope{creative.ScopeRead, creative.ScopeWrite}}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	creativeManager := &creativeManagerStub{frozenVersion: creative.CreativeVersion{ID: "creative_version_1", OrganizationID: "org_1", ProjectID: "project_1", TaskID: "creative_task_1", Version: 1, DraftVersion: 1, Status: creative.CreativeVersionCreated}}
	server := NewWithDependencies(Dependencies{Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"}, Creative: creativeManager})
	request := httptest.NewRequest(http.MethodPost, "/api/creative/v1/projects/project_1/creative-tasks/creative_task_1:freeze-version", bytes.NewBufferString(`{"draft_version":1}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "creative-freeze-http-1")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if creativeManager.freezeKey != "creative-freeze-http-1" || creativeManager.freezeTaskID != "creative_task_1" {
		t.Fatalf("freeze request was not forwarded: key=%q task=%q", creativeManager.freezeKey, creativeManager.freezeTaskID)
	}
}

func TestReviseCreativeDraftUsesTaskActionAndExpectedVersion(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"}, Scopes: []contract.Scope{creative.ScopeRead, creative.ScopeWrite}}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	creativeManager := &creativeManagerStub{revisedDraft: creative.ImageTextDraft{TaskID: "creative_task_1", Version: 2}}
	server := NewWithDependencies(Dependencies{Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"}, Creative: creativeManager})
	request := httptest.NewRequest(http.MethodPatch, "/api/creative/v1/projects/project_1/creative-tasks/creative_task_1:draft", bytes.NewBufferString(`{"expected_version":1,"title_candidates":["标题一","标题二","标题三"],"body":"正文","topics":[],"cover_copy":"封面标题","image_plan":[{"order":1,"purpose":"封面","visual_brief":"干净的产品图","caption":"封面标题"}]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if creativeManager.reviseTaskID != "creative_task_1" || creativeManager.reviseRequest.ExpectedVersion != 1 {
		t.Fatalf("revision was not forwarded: task=%q request=%+v", creativeManager.reviseTaskID, creativeManager.reviseRequest)
	}
}

func TestCreativeHistoryReadEndpointsSurviveRefresh(t *testing.T) {
	t.Parallel()
	actor := contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"}, Scopes: []contract.Scope{creative.ScopeRead}}
	resolver, err := identity.NewStaticResolver(actor)
	if err != nil {
		t.Fatal(err)
	}
	manager := &creativeManagerStub{
		versions: []creative.CreativeVersion{{ID: "creativeversion_1", TaskID: "creativetask_1"}},
		packages: []creative.CreativePackage{{ID: "creativepackage_1", CreativeVersionID: "creativeversion_1"}},
	}
	server := NewWithDependencies(Dependencies{Resolver: resolver, ProjectAuthorizer: identity.StaticProjectAuthorizer{ProjectID: "project_1"}, Creative: manager})

	for _, target := range []struct {
		path string
		want string
	}{
		{"/api/creative/v1/projects/project_1/creative-versions?task_id=creativetask_1", "creativeversion_1"},
		{"/api/creative/v1/projects/project_1/creative-packages", "creativepackage_1"},
	} {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target.path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), target.want) {
			t.Fatalf("%s status=%d body=%s", target.path, response.Code, response.Body.String())
		}
	}
}

type staticProjectManager struct{ context contract.ProjectContext }

type staticAccountManager struct {
	members         []identity.OrganizationMember
	listMemberCalls int
}

func (s *staticAccountManager) ListOrganizations(context.Context, contract.ActorContext) ([]identity.OrganizationAccess, error) {
	return nil, nil
}
func (s *staticAccountManager) UpdateCurrentUser(context.Context, contract.ActorContext, string) (identity.User, error) {
	return identity.User{}, nil
}
func (s *staticAccountManager) ListOrganizationMembers(context.Context, contract.ActorContext) ([]identity.OrganizationMember, error) {
	s.listMemberCalls++
	return s.members, nil
}
func (s *staticAccountManager) AddOrganizationMember(context.Context, contract.ActorContext, string, string) (identity.OrganizationMember, error) {
	return identity.OrganizationMember{}, nil
}
func (s *staticAccountManager) UpdateOrganizationMember(context.Context, contract.ActorContext, string, identity.UpdateOrganizationMembershipRequest) (identity.OrganizationMember, error) {
	return identity.OrganizationMember{}, nil
}

type staticProjectMembershipManager struct {
	addCalls int
}

func (s *staticProjectMembershipManager) ListProjectMembers(context.Context, contract.ActorContext, contract.ProjectID) ([]project.ProjectMembership, error) {
	return nil, nil
}
func (s *staticProjectMembershipManager) AddProjectMember(_ context.Context, actor contract.ActorContext, projectID contract.ProjectID, principal contract.Principal, role string) (project.ProjectMembership, error) {
	s.addCalls++
	return project.ProjectMembership{
		OrganizationID: actor.OrganizationID, ProjectID: projectID,
		PrincipalKind: principal.Kind, PrincipalID: principal.ID, Role: role, Status: "active",
	}, nil
}
func (s *staticProjectMembershipManager) UpdateProjectMember(context.Context, contract.ActorContext, contract.ProjectID, contract.Principal, project.UpdateProjectMembershipRequest) (project.ProjectMembership, error) {
	return project.ProjectMembership{}, nil
}

type staticProviderConfigurationReader struct {
	items []provider.CapabilityStatus
}

func (s staticProviderConfigurationReader) ListCapabilities(context.Context, contract.OrganizationID) ([]provider.CapabilityStatus, error) {
	return s.items, nil
}

func (s staticProjectManager) GetContext(context.Context, contract.ActorContext, contract.ProjectID) (contract.ProjectContext, error) {
	return s.context, nil
}

func (staticProjectManager) CreateBrand(context.Context, contract.ActorContext, string) (project.Brand, error) {
	return project.Brand{}, nil
}

func (staticProjectManager) CreateProject(context.Context, contract.ActorContext, project.CreateProjectRequest) (project.Project, error) {
	return project.Project{}, nil
}
func (staticProjectManager) UpdateProject(context.Context, contract.ActorContext, contract.ProjectID, project.UpdateProjectRequest) (project.Project, error) {
	return project.Project{}, nil
}

func (staticProjectManager) ListProjects(context.Context, contract.ActorContext) ([]project.Project, error) {
	return nil, nil
}

func (staticProjectManager) GetDetail(context.Context, contract.ActorContext, contract.ProjectID) (project.ProjectDetail, error) {
	return project.ProjectDetail{}, nil
}
func (staticProjectManager) CreateProjectArtifact(context.Context, contract.ActorContext, contract.ProjectID, project.CreateProjectArtifactRequest) (project.ProjectArtifact, error) {
	return project.ProjectArtifact{}, nil
}
func (staticProjectManager) ListProjectArtifacts(context.Context, contract.ActorContext, contract.ProjectID) ([]project.ProjectArtifact, error) {
	return nil, nil
}
func (staticProjectManager) GetProjectArtifact(context.Context, contract.ActorContext, contract.ProjectID, string) (project.ProjectArtifact, error) {
	return project.ProjectArtifact{}, nil
}
func (staticProjectManager) UpdateProjectArtifact(context.Context, contract.ActorContext, contract.ProjectID, string, project.UpdateProjectArtifactRequest) (project.ProjectArtifact, error) {
	return project.ProjectArtifact{}, nil
}
func (staticProjectManager) GetWorkbench(context.Context, contract.ActorContext, contract.ProjectID) (project.Workbench, error) {
	return project.Workbench{}, nil
}
func (staticProjectManager) RunWorkbenchQualityCheck(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, request project.RunWorkbenchQualityCheckRequest) (project.WorkbenchQualityCheckRun, error) {
	return project.WorkbenchQualityCheckRun{ID: "qualitycheck_1", AssetID: request.AssetID, AssetVersion: request.AssetVersion, Status: "passed"}, nil
}
func (staticProjectManager) RecordWorkbenchMaterialConfirmation(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, request project.RecordWorkbenchMaterialConfirmationRequest) (project.WorkbenchMaterialConfirmation, error) {
	return project.WorkbenchMaterialConfirmation{ID: "confirmation_1", AssetID: request.AssetID, AssetVersion: request.AssetVersion, Status: request.Status}, nil
}
func (staticProjectManager) UpdateWorkbenchAssetPointer(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, request project.UpdateWorkbenchAssetPointerRequest) (project.WorkbenchAssetVersionPointer, error) {
	return project.WorkbenchAssetVersionPointer{ID: request.AssetID, AssetID: request.AssetID, DeliveryVersion: request.DeliveryVersion}, nil
}

func (staticProjectManager) CreateBusinessTask(context.Context, contract.ActorContext, contract.ProjectID, project.CreateBusinessTaskRequest) (project.BusinessTask, error) {
	return project.BusinessTask{}, nil
}

func (staticProjectManager) ListBusinessTasks(context.Context, contract.ActorContext, contract.ProjectID) ([]project.BusinessTask, error) {
	return nil, nil
}

func (staticProjectManager) GetBusinessTask(context.Context, contract.ActorContext, contract.ProjectID, string) (project.BusinessTask, error) {
	return project.BusinessTask{}, nil
}

func (staticProjectManager) UpdateBusinessTask(context.Context, contract.ActorContext, contract.ProjectID, string, project.UpdateBusinessTaskRequest) (project.BusinessTask, error) {
	return project.BusinessTask{}, nil
}

func (staticProjectManager) CreateOperationalRecord(context.Context, contract.ActorContext, contract.ProjectID, project.UpsertOperationalRecordRequest) (project.OperationalRecord, error) {
	return project.OperationalRecord{}, nil
}

func (staticProjectManager) ListOperationalRecords(context.Context, contract.ActorContext, contract.ProjectID) ([]project.OperationalRecord, error) {
	return nil, nil
}

func (staticProjectManager) GetOperationalRecord(context.Context, contract.ActorContext, contract.ProjectID, string) (project.OperationalRecord, error) {
	return project.OperationalRecord{}, nil
}

func (staticProjectManager) UpsertOperationalRecord(context.Context, contract.ActorContext, contract.ProjectID, string, project.UpsertOperationalRecordRequest) (project.OperationalRecord, error) {
	return project.OperationalRecord{}, nil
}

func (staticProjectManager) CreateChangeSet(context.Context, contract.ActorContext, contract.ProjectID, project.CreateChangeSetRequest) (project.ChangeSet, error) {
	return project.ChangeSet{}, nil
}

func (staticProjectManager) ListChangeSets(context.Context, contract.ActorContext, contract.ProjectID) ([]project.ChangeSet, error) {
	return nil, nil
}

func (staticProjectManager) GetChangeSet(context.Context, contract.ActorContext, contract.ProjectID, string) (project.ChangeSet, error) {
	return project.ChangeSet{}, nil
}

func (staticProjectManager) PreflightChangeSet(context.Context, contract.ActorContext, contract.ProjectID, string) (project.ChangeSet, error) {
	return project.ChangeSet{}, nil
}

func (staticProjectManager) ApproveChangeSet(context.Context, contract.ActorContext, contract.ProjectID, string, project.ChangeSetApprovalRequest) (project.ChangeSet, error) {
	return project.ChangeSet{}, nil
}

func (staticProjectManager) ExecuteChangeSet(context.Context, contract.ActorContext, contract.ProjectID, string) (project.ChangeSet, error) {
	return project.ChangeSet{}, nil
}

func (staticProjectManager) RollbackChangeSet(context.Context, contract.ActorContext, contract.ProjectID, string, project.RollbackChangeSetRequest) (project.ChangeSet, error) {
	return project.ChangeSet{}, nil
}

func (staticProjectManager) ListAuditEvents(context.Context, contract.ActorContext, contract.ProjectID) ([]project.AuditEvent, error) {
	return nil, nil
}

type providerJobStub struct {
	job          contract.ProviderJob
	request      provider.CreateImageJobRequest
	videoRequest provider.CreateVideoJobRequest
	createCalls  int
}

type creativeManagerStub struct {
	detail                             creative.TaskDetail
	intake                             creative.CreativeIntake
	commerceSources                    []creative.CreativeSourceOption
	preparedCommerce                   creative.PreparedCommercePreroll
	registeredProviderJobID            string
	registeredShortDramaProviderJobID  string
	registeredGamePrerollProviderJobID string
	frozenVersion                      creative.CreativeVersion
	freezeKey                          contract.IdempotencyKey
	freezeTaskID                       string
	revisedDraft                       creative.ImageTextDraft
	reviseTaskID                       string
	reviseRequest                      creative.ReviseDraftRequest
	versions                           []creative.CreativeVersion
	packages                           []creative.CreativePackage
	regenerateTaskID                   string
	regenerateRequest                  creative.RegenerateShortDramaCandidatesRequest
	regenerateGameTaskID               string
	regenerateGameRequest              creative.RegenerateGamePrerollCandidatesRequest
	latestShortDramaProjectID          contract.ProjectID
	latestAINativeWorkspace            creative.AINativeRequirementWorkspace
	latestAINativeProjectID            contract.ProjectID
	latestGamePrerollProjectID         contract.ProjectID
	selectedGameTaskID                 string
	selectedGameRequest                creative.SelectGamePrerollCandidateRequest
	createdIntakeRequest               creative.CreateIntakeRequest
	imageWorkspace                     creative.ImageTextWorkspace
	imagePrompt                        creative.ImagePromptPackage
	imageAttempt                       creative.ImageGenerationAttempt
	preparedImageOrder                 int
	attachedImageProviderJobID         string
	failedImageAttemptID               string
}

func (s *creativeManagerStub) GetLatestAINativeRequirementWorkspace(_ context.Context, _ contract.ActorContext, projectID contract.ProjectID) (creative.AINativeRequirementWorkspace, error) {
	s.latestAINativeProjectID = projectID
	return s.latestAINativeWorkspace, nil
}

func (s *creativeManagerStub) ListCommercePrerollSources(context.Context, contract.ActorContext, contract.ProjectID) ([]creative.CreativeSourceOption, error) {
	return s.commerceSources, nil
}
func (s *creativeManagerStub) PrepareCommercePreroll(context.Context, contract.ActorContext, contract.ProjectID, creative.PrepareCommercePrerollRequest) (creative.PreparedCommercePreroll, error) {
	return s.preparedCommerce, nil
}
func (s *creativeManagerStub) EnsureCommerceFixtureWorkspace(context.Context, contract.RequestContext, contract.ProjectID, contract.IdempotencyKey, creative.EnsureCommerceFixtureWorkspaceRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) GetLatestCommerceWorkspace(context.Context, contract.ActorContext, contract.ProjectID) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) GetCommerceWorkspace(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) UpdateCommercePrerollDraft(context.Context, contract.ActorContext, contract.ProjectID, string, creative.UpdateCommercePrerollDraftRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) ConfirmCommerceGeneration(context.Context, contract.ActorContext, contract.ProjectID, string, creative.ConfirmCommerceGenerationRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) CommerceProviderInput(context.Context, contract.ActorContext, contract.ProjectID, string) (provider.VideoGenerationInput, string, error) {
	return provider.VideoGenerationInput{}, "", nil
}
func (s *creativeManagerStub) EnsureBrandFilmFixtureWorkspace(context.Context, contract.RequestContext, contract.ProjectID, contract.IdempotencyKey) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) GetLatestBrandFilmWorkspace(context.Context, contract.ActorContext, contract.ProjectID) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) GetBrandFilmWorkspace(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) InitializeStrategyBrandFilmWorkspace(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) AnalyzeBrandFilmBrief(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BrandFilmRevisionRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) UpdateBrandFilmBrief(context.Context, contract.ActorContext, contract.ProjectID, string, creative.UpdateBrandBriefAnalysisRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) ConfirmBrandFilmBrief(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BrandFilmRevisionRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) GenerateBrandFilmConcepts(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BrandFilmRevisionRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) UpdateBrandFilmConcepts(context.Context, contract.ActorContext, contract.ProjectID, string, creative.UpdateBrandConceptsRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) SelectBrandFilmConcept(context.Context, contract.ActorContext, contract.ProjectID, string, creative.SelectBrandConceptRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) GenerateBrandFilmPlan(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BrandFilmRevisionRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) UpdateBrandFilmPlan(context.Context, contract.ActorContext, contract.ProjectID, string, creative.UpdateBrandFilmPlanRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) ConfirmBrandFilmPlan(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BrandFilmRevisionRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) PrepareBrandFilmGeneration(context.Context, contract.ActorContext, contract.ProjectID, string, creative.PrepareBrandFilmGenerationRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) RegenerateBrandFilmUnit(context.Context, contract.ActorContext, contract.ProjectID, string, creative.RegenerateBrandFilmUnitRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) BrandFilmProviderInput(context.Context, contract.ActorContext, contract.ProjectID, string, string) (provider.VideoGenerationInput, string, error) {
	return provider.VideoGenerationInput{}, "", nil
}
func (s *creativeManagerStub) RegisterBrandFilmGenerationAttempt(context.Context, contract.ActorContext, contract.ProjectID, string, string, string) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) ReconcileBrandFilmGenerationAttempt(context.Context, contract.ActorContext, contract.ProjectID, string, string, contract.ProviderJob) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) LockBrandFilmGenerationUnit(context.Context, contract.ActorContext, contract.ProjectID, string, creative.LockBrandFilmUnitRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) ComposeBrandFilmPreview(context.Context, contract.RequestContext, contract.ProjectID, string, creative.ComposeBrandFilmPreviewRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) PrepareBrandFilmAudio(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BrandFilmRevisionRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) MaterializeBrandFilmAudioAssets(context.Context, contract.RequestContext, contract.ProjectID, string, creative.BrandFilmRevisionRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) UpdateBrandFilmAudioMix(context.Context, contract.ActorContext, contract.ProjectID, string, creative.UpdateBrandAudioMixRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) SelectBrandFilmAudioVariant(context.Context, contract.ActorContext, contract.ProjectID, string, creative.SelectBrandAudioVariantRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) RenderBrandFilmAudioPreview(context.Context, contract.RequestContext, contract.ProjectID, string, creative.BrandFilmRevisionRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) GenerateBrandFilmVoiceClip(context.Context, contract.RequestContext, contract.ProjectID, string, creative.GenerateBrandVoiceClipRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) ProbeBrandFilmSpeech(context.Context, contract.ActorContext, contract.ProjectID) (provider.SpeechCapability, error) {
	return provider.SpeechCapability{Provider: "fixture", Available: false, VoiceAliases: []string{}}, nil
}
func (s *creativeManagerStub) RunBrandFilmQuality(context.Context, contract.ActorContext, contract.ProjectID, string, creative.RunBrandFilmQualityRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) ConfirmBrandFilmQuality(context.Context, contract.ActorContext, contract.ProjectID, string, creative.ConfirmBrandFilmQualityRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) FinalizeBrandFilmVersion(context.Context, contract.RequestContext, contract.ProjectID, string, creative.BrandFilmVersionRequest, contract.IdempotencyKey) (creative.BrandFilmVersionResult, error) {
	return creative.BrandFilmVersionResult{Workspace: s.detail}, nil
}
func (s *creativeManagerStub) ApproveBrandFilmVersion(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BrandFilmVersionRequest) (creative.BrandFilmVersionResult, error) {
	return creative.BrandFilmVersionResult{Workspace: s.detail}, nil
}
func (s *creativeManagerStub) DeliverBrandFilmVersion(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BrandFilmVersionRequest) (creative.BrandFilmDeliveryResult, error) {
	return creative.BrandFilmDeliveryResult{Workspace: s.detail}, nil
}
func (s *creativeManagerStub) CreateIntake(_ context.Context, _ contract.RequestContext, _ contract.ProjectID, _ contract.IdempotencyKey, request creative.CreateIntakeRequest) (creative.CreativeIntake, error) {
	s.createdIntakeRequest = request
	return creative.CreativeIntake{}, nil
}
func (s *creativeManagerStub) ListIntakes(context.Context, contract.ActorContext, contract.ProjectID, int) ([]creative.CreativeIntake, error) {
	return nil, nil
}
func (s *creativeManagerStub) GetIntake(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.CreativeIntake, error) {
	return s.intake, nil
}
func (s *creativeManagerStub) ListBusinessCapabilities(context.Context, contract.ActorContext, contract.ProjectID) ([]creative.CreativeBusinessCapability, error) {
	return creative.CreativeBusinessCapabilities(), nil
}
func (s *creativeManagerStub) CreateTask(context.Context, contract.ActorContext, contract.ProjectID, string, creative.CreateTaskRequest) (creative.CreativeTask, error) {
	return creative.CreativeTask{}, nil
}
func (s *creativeManagerStub) CreateVideoTask(context.Context, contract.ActorContext, contract.ProjectID, string, creative.CreateVideoTaskRequest) (creative.CreativeTask, error) {
	return creative.CreativeTask{}, nil
}
func (s *creativeManagerStub) SelectShortDramaCandidate(context.Context, contract.ActorContext, contract.ProjectID, string, creative.SelectShortDramaCandidateRequest) (creative.TaskDetail, error) {
	return creative.TaskDetail{}, nil
}
func (s *creativeManagerStub) RegenerateShortDramaCandidates(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, taskID string, request creative.RegenerateShortDramaCandidatesRequest) (creative.TaskDetail, error) {
	s.regenerateTaskID = taskID
	s.regenerateRequest = request
	return s.detail, nil
}
func (s *creativeManagerStub) GetLatestShortDramaWorkspace(_ context.Context, _ contract.ActorContext, projectID contract.ProjectID) (creative.TaskDetail, error) {
	s.latestShortDramaProjectID = projectID
	return s.detail, nil
}
func (s *creativeManagerStub) ShortDramaProviderInput(context.Context, contract.ActorContext, contract.ProjectID, string) (provider.VideoGenerationInput, string, error) {
	return provider.VideoGenerationInput{}, "", nil
}
func (s *creativeManagerStub) GetLatestGamePrerollWorkspace(_ context.Context, _ contract.ActorContext, projectID contract.ProjectID) (creative.TaskDetail, error) {
	s.latestGamePrerollProjectID = projectID
	return s.detail, nil
}
func (s *creativeManagerStub) PrepareGamePrerollEvidence(_ context.Context, _ contract.RequestContext, _ contract.ProjectID, _ string, _ creative.PrepareGamePrerollEvidenceRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) SelectGamePrerollCandidate(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, taskID string, request creative.SelectGamePrerollCandidateRequest) (creative.TaskDetail, error) {
	s.selectedGameTaskID = taskID
	s.selectedGameRequest = request
	return s.detail, nil
}
func (s *creativeManagerStub) RegenerateGamePrerollCandidates(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, taskID string, request creative.RegenerateGamePrerollCandidatesRequest) (creative.TaskDetail, error) {
	s.regenerateGameTaskID = taskID
	s.regenerateGameRequest = request
	return s.detail, nil
}
func (s *creativeManagerStub) GamePrerollProviderInput(context.Context, contract.ActorContext, contract.ProjectID, string) (provider.VideoGenerationInput, string, error) {
	return provider.VideoGenerationInput{}, "", nil
}
func (s *creativeManagerStub) ListTasks(context.Context, contract.ActorContext, contract.ProjectID, int) ([]creative.CreativeTask, error) {
	return nil, nil
}
func (s *creativeManagerStub) RenameTask(context.Context, contract.ActorContext, contract.ProjectID, string, creative.RenameTaskRequest) (creative.CreativeTask, error) {
	return creative.CreativeTask{}, nil
}
func (s *creativeManagerStub) GetTaskDetail(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) AnalyzeViralRemake(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) UpdateViralPrompt(context.Context, contract.ActorContext, contract.ProjectID, string, creative.UpdateViralPromptRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) ConfirmViralGeneration(context.Context, contract.ActorContext, contract.ProjectID, string, creative.ConfirmViralGenerationRequest) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) ViralProviderInput(context.Context, contract.ActorContext, contract.ProjectID, string) (provider.VideoGenerationInput, string, error) {
	return provider.VideoGenerationInput{
		Prompt: "viral", DurationSeconds: 5, AspectRatio: "9:16", Resolution: "720p",
		InputMode: provider.VideoInputTextOnly, ConditioningAssets: []provider.VideoConditioningAsset{},
	}, "sha256:viral", nil
}
func (s *creativeManagerStub) RegisterViralCandidateJob(context.Context, contract.ActorContext, contract.ProjectID, string, string) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) ReconcileViralCandidate(context.Context, contract.ActorContext, contract.ProjectID, string, contract.ProviderJob) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) SubmitViralCandidateReview(context.Context, contract.ActorContext, contract.ProjectID, string, string) (creative.TaskDetail, error) {
	return s.detail, nil
}
func (s *creativeManagerStub) ArchiveTask(context.Context, contract.ActorContext, contract.ProjectID, string) error {
	return nil
}
func (s *creativeManagerStub) RegisterCoverImageJob(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, _ string, providerJobID string) error {
	s.registeredProviderJobID = providerJobID
	return nil
}
func (s *creativeManagerStub) RegisterImagePlanJob(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, _ string, _ int, providerJobID string) error {
	s.registeredProviderJobID = providerJobID
	return nil
}
func (s *creativeManagerStub) RegisterVideoJob(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, _ string, providerJobID string) error {
	s.registeredProviderJobID = providerJobID
	return nil
}
func (s *creativeManagerStub) RegisterShortDramaGenerationAttempt(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, _ string, providerJobID string) (creative.ShortDramaGenerationAttempt, error) {
	s.registeredShortDramaProviderJobID = providerJobID
	return creative.ShortDramaGenerationAttempt{ProviderJobID: providerJobID}, nil
}
func (s *creativeManagerStub) RegisterGamePrerollGenerationAttempt(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, _ string, providerJobID string) (creative.GamePrerollGenerationAttempt, error) {
	s.registeredGamePrerollProviderJobID = providerJobID
	return creative.GamePrerollGenerationAttempt{ProviderJobID: providerJobID}, nil
}
func (s *creativeManagerStub) RegisterCommerceGenerationAttempt(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, _ string, providerJobID string) (creative.CommerceGenerationAttempt, error) {
	return creative.CommerceGenerationAttempt{ProviderJobID: providerJobID}, nil
}
func (s *creativeManagerStub) CreateRenderJob(context.Context, contract.RequestContext, contract.ProjectID, string, creative.CreateRenderJobRequest, contract.IdempotencyKey) (creative.RenderJob, bool, error) {
	return creative.RenderJob{}, false, nil
}
func (s *creativeManagerStub) GetRenderJob(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.RenderJob, error) {
	return creative.RenderJob{}, nil
}
func (s *creativeManagerStub) FreezeVersion(_ context.Context, _ contract.RequestContext, _ contract.ProjectID, taskID string, _ creative.FreezeVersionRequest, key contract.IdempotencyKey) (creative.CreativeVersion, bool, error) {
	s.freezeKey = key
	s.freezeTaskID = taskID
	return s.frozenVersion, false, nil
}
func (s *creativeManagerStub) ListVersions(context.Context, contract.ActorContext, contract.ProjectID, string, int) ([]creative.CreativeVersion, error) {
	return s.versions, nil
}
func (s *creativeManagerStub) ReviseDraft(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, taskID string, request creative.ReviseDraftRequest) (creative.ImageTextDraft, error) {
	s.reviseTaskID = taskID
	s.reviseRequest = request
	return s.revisedDraft, nil
}
func (s *creativeManagerStub) BindImageAsset(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BindImageAssetRequest) (creative.ImageTextDraft, error) {
	return s.revisedDraft, nil
}
func (s *creativeManagerStub) CheckVersion(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.CreativeVersion, error) {
	return s.frozenVersion, nil
}
func (s *creativeManagerStub) ApproveVersion(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.CreativeVersion, error) {
	return s.frozenVersion, nil
}
func (s *creativeManagerStub) DeliverVersion(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.CreativePackage, error) {
	return creative.CreativePackage{}, nil
}
func (s *creativeManagerStub) ListPackages(context.Context, contract.ActorContext, contract.ProjectID, int) ([]creative.CreativePackage, error) {
	return s.packages, nil
}

func (s *creativeManagerStub) GetImageTextWorkspace(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.ImageTextWorkspace, error) {
	return s.imageWorkspace, nil
}

func (s *creativeManagerStub) GenerateImageTextDraft(context.Context, contract.ActorContext, contract.ProjectID, string, creative.GenerateImageTextDraftRequest) (creative.ImageTextDraft, error) {
	return s.imageWorkspace.Draft, nil
}

func (s *creativeManagerStub) UpdateImageTextDraft(context.Context, contract.ActorContext, contract.ProjectID, string, creative.UpdateImageTextDraftRequest) (creative.ImageTextDraft, error) {
	return s.imageWorkspace.Draft, nil
}

func (s *creativeManagerStub) PrepareImageSlotGeneration(
	_ context.Context,
	_ contract.RequestContext,
	_ contract.ProjectID,
	_ string,
	order int,
	_ creative.PrepareImageSlotRequest,
	_ contract.IdempotencyKey,
) (creative.ImagePromptPackage, creative.ImageGenerationAttempt, bool, error) {
	s.preparedImageOrder = order
	return s.imagePrompt, s.imageAttempt, false, nil
}

func (s *creativeManagerStub) AttachImageProviderJob(
	_ context.Context,
	_ contract.ActorContext,
	_ contract.ProjectID,
	_ string,
	providerJobID string,
) (creative.ImageGenerationAttempt, error) {
	s.attachedImageProviderJobID = providerJobID
	s.imageAttempt.ProviderJobID = providerJobID
	return s.imageAttempt, nil
}

func (s *creativeManagerStub) FailImageGenerationAttempt(
	_ context.Context,
	_ contract.ActorContext,
	_ contract.ProjectID,
	attemptID string,
	_ string,
	_ string,
) (creative.ImageGenerationAttempt, error) {
	s.failedImageAttemptID = attemptID
	s.imageAttempt.Status = creative.ImageAttemptFailed
	return s.imageAttempt, nil
}

func (s *creativeManagerStub) AdoptImageGenerationAttempt(context.Context, contract.ActorContext, contract.ProjectID, string, int, string, creative.AdoptImageAttemptRequest) (creative.ImageSlotSelection, error) {
	return creative.ImageSlotSelection{}, nil
}

func (s *providerJobStub) CreateImageJob(_ context.Context, request provider.CreateImageJobRequest) (contract.ProviderJob, bool, error) {
	s.createCalls++
	s.request = request
	return s.job, false, nil
}

func (s *providerJobStub) CreateVideoJob(_ context.Context, request provider.CreateVideoJobRequest) (contract.ProviderJob, bool, error) {
	s.createCalls++
	s.videoRequest = request
	return s.job, false, nil
}

func (s *providerJobStub) GetJob(context.Context, contract.OrganizationID, contract.ProjectID, string) (contract.ProviderJob, error) {
	return s.job, nil
}

func providerJobForHTTPTest() contract.ProviderJob {
	now := time.Date(2026, time.July, 22, 5, 0, 0, 0, time.UTC)
	return contract.ProviderJob{
		ID: "provider_job_1", Kind: "provider.image.generate", OrganizationID: "org_1", ProjectID: "project_1",
		ExecutionStatus: contract.JobQueued, ProviderStatus: contract.ProviderJobSubmitted, ProjectAssetRefs: []contract.ProjectAssetRef{},
		MaxAttempts: 3, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
}
