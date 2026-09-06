package oceanengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWriteClientUsesProtectedPostPathForHEADAndCachesToken(t *testing.T) {
	var heads, posts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "session=secret; csrftoken=cookie-csrf" {
			t.Error("session cookie missing")
		}
		if r.Header.Get("x-sessionid") != "" {
			t.Error("unverified browser session header must be omitted")
		}
		if _, present := r.URL.Query()["_signature"]; present {
			t.Error("unverified browser signature must be omitted")
		}
		switch r.Method {
		case http.MethodHead:
			heads.Add(1)
			if r.URL.Path != ProjectCreatePath || r.URL.RawQuery != "" || r.Header.Get("x-csrftoken") != "" || r.Header.Get("x-secsdk-csrf-request") != "1" || r.Header.Get("x-secsdk-csrf-version") != SecSDKVersion {
				t.Errorf("invalid HEAD contract: %s %#v", r.URL.Path, r.Header)
			}
			w.Header().Set("x-ware-csrf-token", "0,token-value,60000,ok,session")
		case http.MethodPost:
			posts.Add(1)
			if r.URL.Path != ProjectCreatePath || r.URL.Query().Get("aadvid") != "10001" || r.Header.Get("x-csrftoken") != "cookie-csrf" || r.Header.Get("x-secsdk-csrf-token") != "token-value" {
				t.Errorf("invalid POST contract: %s %#v", r.URL.Path, r.Header)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":"123"}}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()
	client, err := NewWriteClient(server.URL, "10001", 7, Session{Cookies: "session=secret; csrftoken=cookie-csrf"}, server.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err = client.SubmitJSON(context.Background(), ProjectCreatePath, map[string]any{"name": "redacted"}); err != nil {
			t.Fatal(err)
		}
	}
	if heads.Load() != 1 || posts.Load() != 2 {
		t.Fatalf("HEAD=%d POST=%d", heads.Load(), posts.Load())
	}
}

func TestWriteClientRejectsInvalidAndDowngradeToken(t *testing.T) {
	for _, header := range []string{"", "1,token,60000,error", "0,,60000,ok", "0,DOWNGRADE,60000,ok", "0,token,0,ok"} {
		t.Run(header, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Header().Set("x-ware-csrf-token", header) }))
			defer server.Close()
			client, _ := NewWriteClient(server.URL, "10001", 1, Session{Cookies: "session=secret"}, server.Client(), nil)
			_, err := client.SubmitJSON(context.Background(), ProjectCreatePath, map[string]any{})
			if !errors.Is(err, ErrCSRFTokenInvalid) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestWriteClientDiagnosesMissingCSRFHeaderWithoutAWrite(t *testing.T) {
	var heads, posts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			heads.Add(1)
			return
		}
		posts.Add(1)
	}))
	defer server.Close()
	client, err := NewWriteClient(server.URL, "10001", 1, Session{Cookies: "session=secret"}, server.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	diagnostic, err := client.DiagnoseCSRF(context.Background(), ProjectCreatePath)
	if !errors.Is(err, ErrCSRFTokenInvalid) || diagnostic.HTTPStatus != http.StatusOK || diagnostic.HeaderPresent || heads.Load() != 1 || posts.Load() != 0 {
		t.Fatalf("diagnostic=%#v error=%v heads=%d posts=%d", diagnostic, err, heads.Load(), posts.Load())
	}
}

func TestWriteClientAllowsObservedSDKDowngradeOnlyWhenExplicit(t *testing.T) {
	var heads, posts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			heads.Add(1)
		case http.MethodPost:
			posts.Add(1)
			if r.Header.Get("x-secsdk-csrf-token") != "DOWNGRADE" {
				t.Errorf("csrf header=%q", r.Header.Get("x-secsdk-csrf-token"))
			}
			_, _ = w.Write([]byte(`{"code":0}`))
		}
	}))
	defer server.Close()
	client, err := NewWriteClient(server.URL, "10001", 1, Session{Cookies: "session=secret"}, server.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	client.AllowSDKDowngrade = true
	if _, err = client.SubmitJSON(context.Background(), ProjectCreatePath, map[string]any{"name": "redacted"}); err != nil {
		t.Fatal(err)
	}
	if heads.Load() != 1 || posts.Load() != 1 {
		t.Fatalf("heads=%d posts=%d", heads.Load(), posts.Load())
	}
}

func TestWriteClientAddsProbeSessionIDOnlyWhenConfigured(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
		case http.MethodPost:
			if got := r.Header.Get("x-sessionid"); got != "00000000-0000-4000-8000-000000000000" {
				t.Errorf("session header=%q", got)
			}
			_, _ = w.Write([]byte(`{"code":0}`))
		}
	}))
	defer server.Close()
	client, err := NewWriteClient(server.URL, "10001", 1, Session{Cookies: "session=secret"}, server.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	client.AllowSDKDowngrade = true
	client.ProbeSessionID = "00000000-0000-4000-8000-000000000000"
	if _, err = client.SubmitJSON(context.Background(), ProjectCreatePath, map[string]any{"name": "redacted"}); err != nil {
		t.Fatal(err)
	}
	client.Close()
	if client.ProbeSessionID != "" {
		t.Fatal("probe session ID was not cleared")
	}
}

func TestWriteClientAddsProbeSignatureWithoutLoggingItsValue(t *testing.T) {
	const signature = "sensitive-signature"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
		case http.MethodPost:
			if got := r.URL.Query().Get("_signature"); got != signature {
				t.Errorf("signature mismatch")
			}
			_, _ = w.Write([]byte(`{"code":0}`))
		}
	}))
	defer server.Close()
	client, err := NewWriteClient(server.URL, "10001", 1, Session{Cookies: "session=secret"}, server.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	client.AllowSDKDowngrade = true
	client.ProbeSigner = func(_ context.Context, target *url.URL, body []byte) (string, error) {
		if target.Query().Has("aadvid") || target.Path != ProjectCreatePath || !bytes.Contains(body, []byte(`"name":"redacted"`)) {
			t.Fatal("signer did not receive the exact URL and body")
		}
		return signature, nil
	}
	if _, err = client.SubmitJSON(context.Background(), ProjectCreatePath, map[string]any{"name": "redacted"}); err != nil {
		t.Fatal(err)
	}
	client.Close()
	if client.ProbeSigner != nil {
		t.Fatal("probe signer was not cleared")
	}
}

func TestWriteClientAddsBrowserHeadersOnlyWhenConfigured(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
		case http.MethodPost:
			for _, name := range []string{"Sec-Fetch-Site", "Sec-Fetch-Mode", "Sec-Fetch-Dest", "Sec-Ch-Ua", "Priority"} {
				if r.Header.Get(name) == "" {
					t.Errorf("missing browser header %s", name)
				}
			}
			if got := r.Header.Get("User-Agent"); !strings.Contains(got, "Edg/153") {
				t.Errorf("user agent override=%q", got)
			}
			if got := r.Header.Get("Referer"); !strings.Contains(got, "create-project?aadvid=10001") {
				t.Errorf("referer override=%q", got)
			}
			_, _ = w.Write([]byte(`{"code":0}`))
		}
	}))
	defer server.Close()
	client, err := NewWriteClient(server.URL, "10001", 1, Session{Cookies: "session=secret"}, server.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	client.AllowSDKDowngrade = true
	client.ProbeBrowserHeaders = true
	client.ProbeUserAgent = browserUserAgent
	client.ProbeReferer = server.URL + "/superior/create-project?aadvid=10001&fromPage=newProject"
	if _, err = client.SubmitJSON(context.Background(), ProjectCreatePath, map[string]any{"name": "redacted"}); err != nil {
		t.Fatal(err)
	}
	client.Close()
	if client.ProbeBrowserHeaders || client.ProbeReferer != "" || client.ProbeUserAgent != "" {
		t.Fatal("browser header probe fields were not cleared")
	}
}

func TestWriteClientOmitsBrowserHeadersByDefault(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
		case http.MethodPost:
			if r.Header.Get("Sec-Fetch-Site") != "" || r.Header.Get("Priority") != "" {
				t.Fatal("browser headers leaked into the default client")
			}
			_, _ = w.Write([]byte(`{"code":0}`))
		}
	}))
	defer server.Close()
	client, err := NewWriteClient(server.URL, "10001", 1, Session{Cookies: "session=secret"}, server.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	client.AllowSDKDowngrade = true
	if _, err = client.SubmitJSON(context.Background(), ProjectCreatePath, map[string]any{"name": "redacted"}); err != nil {
		t.Fatal(err)
	}
}

func TestWriteClientStopsBeforePostWhenProbeSignerFails(t *testing.T) {
	var posts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
		}
	}))
	defer server.Close()
	client, err := NewWriteClient(server.URL, "10001", 1, Session{Cookies: "session=secret"}, server.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	client.AllowSDKDowngrade = true
	client.ProbeSigner = func(context.Context, *url.URL, []byte) (string, error) {
		return "", errors.New("sensitive signer detail")
	}
	_, err = client.SubmitJSON(context.Background(), ProjectCreatePath, map[string]any{"name": "redacted"})
	if err == nil || strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("unsafe signer error=%v", err)
	}
	if posts.Load() != 0 {
		t.Fatalf("posts=%d", posts.Load())
	}
}

func TestWriteClientDoesNotRetryUnknownPost(t *testing.T) {
	var posts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("x-ware-csrf-token", "0,token,60000,ok")
			return
		}
		posts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client, _ := NewWriteClient(server.URL, "10001", 1, Session{Cookies: "session=secret"}, server.Client(), nil)
	_, err := client.SubmitJSON(context.Background(), PromotionCreatePath, json.RawMessage(`{"name":"redacted"}`))
	if !errors.Is(err, ErrResultUnknown) || posts.Load() != 1 {
		t.Fatalf("error=%v posts=%d", err, posts.Load())
	}
}

func TestWriteClientCacheKeyIncludesAccountAndSessionVersion(t *testing.T) {
	var heads atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			heads.Add(1)
			w.Header().Set("x-ware-csrf-token", "0,token,60000,ok")
			return
		}
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()
	cache := NewCSRFTokenCache()
	now := time.Now
	for _, item := range []struct {
		account string
		version int64
	}{{"1", 1}, {"2", 1}, {"1", 2}} {
		client, _ := NewWriteClient(server.URL, item.account, item.version, Session{Cookies: "secret"}, server.Client(), cache)
		client.Now = now
		if _, err := client.SubmitJSON(context.Background(), ProjectCreatePath, map[string]any{}); err != nil {
			t.Fatal(err)
		}
	}
	if heads.Load() != 3 {
		t.Fatalf("HEAD=%d", heads.Load())
	}
}

func TestWriteClientRefreshesExpiredToken(t *testing.T) {
	var heads atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			heads.Add(1)
			w.Header().Set("x-ware-csrf-token", "0,token,1000,ok")
			return
		}
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()
	client, _ := NewWriteClient(server.URL, "1", 1, Session{Cookies: "secret"}, server.Client(), nil)
	now := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	client.Now = func() time.Time { return now }
	if _, err := client.SubmitJSON(context.Background(), ProjectCreatePath, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	if _, err := client.SubmitJSON(context.Background(), ProjectCreatePath, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if heads.Load() != 2 {
		t.Fatalf("HEAD=%d", heads.Load())
	}
}

func TestWriteClientMarksAuthenticationRequired(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer server.Close()
	client, _ := NewWriteClient(server.URL, "1", 1, Session{Cookies: "secret"}, server.Client(), nil)
	_, err := client.SubmitJSON(context.Background(), ProjectCreatePath, map[string]any{})
	if !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("error=%v", err)
	}
}

func TestWriteClientErrorsDoNotExposeSessionOrAccount(t *testing.T) {
	client, err := NewWriteClient("https://127.0.0.1:1", "sensitive-account", 1, Session{Cookies: "sensitive-cookie"}, &http.Client{Timeout: 50 * time.Millisecond}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SubmitJSON(context.Background(), ProjectCreatePath, map[string]any{})
	message := err.Error()
	if strings.Contains(message, "sensitive-account") || strings.Contains(message, "sensitive-cookie") {
		t.Fatalf("secret escaped through error: %s", message)
	}
}
