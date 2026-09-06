package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/shikanon/cookies/internal/integrations/oceanengine"
	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/platform/contract"
)

type payloadSourceStub struct {
	object  CompiledObject
	pending bool
}

func (s payloadSourceStub) CompileNext(context.Context, browserautomation.BrowserRpaRun) (CompiledObject, bool, error) {
	return s.object, s.pending, nil
}

type templateStub struct{ templates CreateTemplates }

func (s templateStub) Load(context.Context) (CreateTemplates, error) { return s.templates, nil }

type writeSessionStub struct {
	server *httptest.Server
	heads  *atomic.Int32
	posts  *atomic.Int32
}

func (s writeSessionStub) OpenSession(_ context.Context, run browserautomation.BrowserRpaRun) (WriteSession, error) {
	client, err := oceanengine.NewWriteClient(s.server.URL, run.AccountID, 1, oceanengine.Session{Cookies: "session=secret"}, s.server.Client(), nil)
	if err != nil {
		return WriteSession{}, err
	}
	client.AllowSDKDowngrade = true
	reader, err := oceanengine.NewClient(s.server.URL, run.AccountID, oceanengine.Session{Cookies: "session=secret"}, s.server.Client())
	if err != nil {
		return WriteSession{}, err
	}
	reader.Delay = 0
	reader.MaxAttempts = 1
	return WriteSession{Writer: client, Reader: reader, Close: func() { client.Close() }}, nil
}

func submitTestRun() browserautomation.BrowserRpaRun {
	return browserautomation.BrowserRpaRun{
		ID:             "run_1",
		OrganizationID: contract.OrganizationID("org_1"),
		ProjectID:      contract.ProjectID("project_1"),
		AccountID:      "1855554434276391",
	}
}

func submitTestTemplates() CreateTemplates {
	return CreateTemplates{
		Project: map[string]any{
			"name": "template", "start_time": "1788192000", "end_time": "1788364799", "bid": 0.01,
			"product_id": "1784863906740671489", "landing_type": 17, "campaign_type": 1,
		},
		Promotion: map[string]any{
			"name": "template", "project_id": "template-project", "check_hash": "0",
			"material_group": map[string]any{
				"video_material_info": []any{map[string]any{
					"video_info": map[string]any{"video_id": "v03033g10000d8kj9q2ljht6dsph9pm0"},
				}},
			},
		},
	}
}

func submitReadyAdapter(source PayloadSource, factory WriteSessionFactory) Adapter {
	return Adapter{
		WriteEnabled: true, AccountAllowlist: []string{"1855554434276391"},
		PayloadSource: source, Templates: templateStub{templates: submitTestTemplates()}, SessionFactory: factory,
	}
}

func TestWebAPISubmitCreatesProjectAndReconcilesByID(t *testing.T) {
	var heads, posts, reads atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
			heads.Add(1)
		case r.Method == http.MethodGet && r.URL.Path == oceanengine.CheckProjectNamePath:
			_, _ = w.Write([]byte(`{"code":0}`))
		case r.Method == http.MethodPost && r.URL.Path == oceanengine.ProjectCreatePath:
			posts.Add(1)
			if r.Header.Get("x-secsdk-csrf-token") != "DOWNGRADE" {
				t.Errorf("csrf header=%q", r.Header.Get("x-secsdk-csrf-token"))
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["external_action"] != "2" {
				t.Errorf("project payload=%#v err=%v", body, err)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"project_id":"7680332723195904041"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/superior/api/v2/project/detail":
			reads.Add(1)
			if r.URL.Query().Get("need_ea_conversion_status") != "true" {
				t.Errorf("project detail query=%s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"projects":[{"project_id":"7680332723195904041","project_name":"probe-project","external_action":"2","start_time":"2026-09-01 00:00:00","end_time":"2026-09-02 23:59:59","project_bid":"0.01"}]}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	schedule := int64(1788192000)
	end := int64(1788364799)
	bid := 0.01
	source := payloadSourceStub{object: CompiledObject{
		Kind: "project", InternalID: "draft_1", Name: "probe-project",
		StartUnix: schedule, EndUnix: end, BidYuan: &bid, ExternalAction: "2", ProductReferenceID: "1784863906740671489",
	}, pending: true}
	adapter := submitReadyAdapter(source, writeSessionStub{server: server})
	attempt := browserautomation.ControlledActionAttempt{ID: "attempt_1"}
	outcome, page, err := adapter.Submit(context.Background(), submitTestRun(), attempt, "token")
	if err != nil {
		t.Fatal(err)
	}
	if outcome != browserautomation.WorkerSuccess {
		t.Fatalf("outcome=%s", outcome)
	}
	if heads.Load() != 1 || posts.Load() != 1 || reads.Load() != 1 {
		t.Fatalf("heads=%d posts=%d reads=%d", heads.Load(), posts.Load(), reads.Load())
	}
	if page.InternalObjectKind != "project" || page.InternalObjectID != "draft_1" {
		t.Fatalf("staged identity=%s/%s", page.InternalObjectKind, page.InternalObjectID)
	}
	if page.Readback["platform_object_id"] != "7680332723195904041" || page.Readback["reconciliation"] != "matched" || page.Readback["field_reconciliation_status"] != "matched" || page.Readback["external_action"] != "2" {
		t.Fatalf("readback=%#v", page.Readback)
	}
}

func TestProjectReadbackRequiresMatchingExternalAction(t *testing.T) {
	object := CompiledObject{Kind: "project", ExternalAction: "2"}
	if status := reconcileScheduleAndBid(object, map[string]any{}, map[string]any{"external_action": "100"}); status != "mismatched" {
		t.Fatalf("mismatched action status=%s", status)
	}
	if status := reconcileScheduleAndBid(object, map[string]any{}, map[string]any{}); status != "not_checked" {
		t.Fatalf("missing action status=%s", status)
	}
	if status := reconcileScheduleAndBid(object, map[string]any{}, map[string]any{"marketing_info": map[string]any{"external_action": 2.0}}); status != "matched" {
		t.Fatalf("numeric action status=%s", status)
	}
}

func TestWebAPISubmitCreatesPromotionWithProjectBinding(t *testing.T) {
	var bodies atomic.Value
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead:
		case r.Method == http.MethodGet && r.URL.Path == oceanengine.CheckPromotionNamePath:
			_, _ = w.Write([]byte(`{"code":0}`))
		case r.Method == http.MethodPost && r.URL.Path == oceanengine.PromotionCreatePath:
			body := make([]byte, 4096)
			n, _ := r.Body.Read(body)
			bodies.Store(string(body[:n]))
			_, _ = w.Write([]byte(`{"code":0,"data":{"promotion_id":"7680399999999999999"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/superior/api/ad/promotion/detail":
			_, _ = w.Write([]byte(`{"code":0,"data":{"promotions":[{"promotion_id":"7680399999999999999","promotion_name":"probe-promotion"}]}}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	source := payloadSourceStub{object: CompiledObject{
		Kind: "promotion", InternalID: "draft_2", Name: "probe-promotion",
		DependsOnPlatformID: "7680332723195904041",
	}, pending: true}
	adapter := submitReadyAdapter(source, writeSessionStub{server: server})
	outcome, page, err := adapter.Submit(context.Background(), submitTestRun(), browserautomation.ControlledActionAttempt{ID: "attempt_2"}, "token")
	if err != nil {
		t.Fatal(err)
	}
	if outcome != browserautomation.WorkerSuccess || page.Readback["platform_object_id"] != "7680399999999999999" {
		t.Fatalf("outcome=%s page=%#v", outcome, page.Readback)
	}
	payload := bodies.Load().(string)
	for _, want := range []string{`"project_id":"7680332723195904041"`, `"check_hash":"`, `"name":"probe-promotion"`} {
		if !contains(payload, want) {
			t.Fatalf("payload missing %s: %s", want, payload)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || len(needle) == 0 || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func TestWebAPISubmitStopsBeforeWriteOnTemplateMismatch(t *testing.T) {
	var posts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
		}
	}))
	defer server.Close()
	source := payloadSourceStub{object: CompiledObject{
		Kind: "promotion", InternalID: "draft_2", Name: "probe-promotion",
		MaterialReferenceIDs: []string{"other-video"}, DependsOnPlatformID: "1",
	}, pending: true}
	adapter := submitReadyAdapter(source, writeSessionStub{server: server})
	_, _, err := adapter.Submit(context.Background(), submitTestRun(), browserautomation.ControlledActionAttempt{ID: "attempt_3"}, "token")
	if !errors.Is(err, ErrTemplateMismatch) {
		t.Fatalf("err=%v", err)
	}
	if posts.Load() != 0 {
		t.Fatal("a write escaped a template mismatch")
	}
}

func TestWebAPISubmitReportsUnknownOnTransportFailure(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()
	source := payloadSourceStub{object: CompiledObject{Kind: "project", InternalID: "draft_1", Name: "probe-project", ExternalAction: "2"}, pending: true}
	adapter := submitReadyAdapter(source, writeSessionStub{server: server})
	outcome, _, err := adapter.Submit(context.Background(), submitTestRun(), browserautomation.ControlledActionAttempt{ID: "attempt_4"}, "token")
	if err != nil {
		t.Fatal(err)
	}
	if outcome != browserautomation.WorkerResultUnknown {
		t.Fatalf("outcome=%s", outcome)
	}
}

func TestWebAPISubmitFailsClosedOnBusinessRejection(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write([]byte(`{"code":50100,"name":"INTERNAL_SERVICE_ERROR"}`))
	}))
	defer server.Close()
	source := payloadSourceStub{object: CompiledObject{Kind: "project", InternalID: "draft_1", Name: "probe-project", ExternalAction: "2"}, pending: true}
	adapter := submitReadyAdapter(source, writeSessionStub{server: server})
	outcome, _, err := adapter.Submit(context.Background(), submitTestRun(), browserautomation.ControlledActionAttempt{ID: "attempt_5"}, "token")
	if err == nil || outcome != browserautomation.WorkerFailed {
		t.Fatalf("outcome=%s err=%v", outcome, err)
	}
}

func TestWebAPISubmitGateStaysClosedWithoutContractPlumbing(t *testing.T) {
	adapter := Adapter{WriteEnabled: true, AccountAllowlist: []string{"1855554434276391"}}
	if err := adapter.CheckSubmit(submitTestRun()); !errors.Is(err, ErrContractNotCaptured) {
		t.Fatalf("err=%v", err)
	}
}
