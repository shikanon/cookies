package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/shikanon/cookies/internal/platform/contract"
)

func TestAdapterGatewayTextProducesStructuredOutput(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("unexpected request path=%q auth=%q", request.URL.Path, request.Header.Get("Authorization"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"model":"text-v2","usage":{"prompt_tokens":12,"completion_tokens":4,"total_tokens":16},"choices":[{"message":{"content":"{\"operations\":[]}"}}]}`))
	}))
	defer server.Close()
	adapter, err := NewAdapterGatewayTextAdapter(
		textRouteStub{snapshot: textRouteSnapshot(server.URL)},
		credentialStub("test-token"),
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	adapter.client = server.Client()
	result, err := adapter.GenerateText(context.Background(), TextAdapterRequest{
		OrganizationID: "org_1", ModelAlias: "cookies.text.standard",
		Messages:         []TextMessage{{Role: TextRoleUser, Content: "extract"}},
		OutputJSONSchema: []byte(`{"type":"object"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ModelVersion != "text-v2" || string(result.StructuredOutput) != `{"operations":[]}` || result.Usage == nil || result.Usage.TotalTokens != 16 {
		t.Fatalf("result = %#v", result)
	}
}

func TestAdapterGatewayTextMapsRateLimitAsRetryableTextError(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()
	adapter, err := NewAdapterGatewayTextAdapter(
		textRouteStub{snapshot: textRouteSnapshot(server.URL)},
		credentialStub("test-token"),
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	adapter.client = server.Client()
	_, err = adapter.GenerateText(context.Background(), TextAdapterRequest{
		OrganizationID: "org_1",
		ModelAlias:     "cookies.text.standard",
		Messages:       []TextMessage{{Role: TextRoleUser, Content: "generate"}},
	})
	var execution ExecutionError
	if !errors.As(err, &execution) {
		t.Fatalf("error = %T %v, want ExecutionError", err, err)
	}
	if execution.JobError.Code != "MODEL_RATE_LIMITED" || !execution.JobError.Retryable ||
		execution.JobError.Message != "Adapter gateway rate limited the text request" {
		t.Fatalf("job error = %#v", execution.JobError)
	}
}

func TestAdapterGatewayTextDoesNotRetryInvalidParameterDisguisedAsRateLimit(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"error":{"code":"rate_limited","message":"Unsupported thinking type for the current model: auto","provider":"xinyuan","provider_code":"InvalidParameter"}}`))
	}))
	defer server.Close()
	adapter, err := NewAdapterGatewayTextAdapter(
		textRouteStub{snapshot: textRouteSnapshot(server.URL)},
		credentialStub("test-token"),
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	adapter.client = server.Client()
	_, err = adapter.GenerateText(context.Background(), TextAdapterRequest{
		OrganizationID: "org_1",
		ModelAlias:     "cookies.text.standard",
		Messages:       []TextMessage{{Role: TextRoleUser, Content: "generate"}},
	})
	var execution ExecutionError
	if !errors.As(err, &execution) {
		t.Fatalf("error = %T %v, want ExecutionError", err, err)
	}
	if execution.JobError.Code != "MODEL_REQUEST_REJECTED" || execution.JobError.Retryable ||
		execution.JobError.Message != "Text model rejected the configured request parameters" {
		t.Fatalf("job error = %#v", execution.JobError)
	}
}

func TestAdapterGatewayTextRejectsOversizedResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(strings.Repeat("x", 40)))
	}))
	defer server.Close()
	snapshot := textRouteSnapshot(server.URL)
	snapshot.MaxResponseBytes = 16
	adapter, _ := NewAdapterGatewayTextAdapter(textRouteStub{snapshot: snapshot}, credentialStub("token"), false)
	adapter.client = server.Client()
	_, err := adapter.GenerateText(context.Background(), TextAdapterRequest{
		OrganizationID: "org_1", ModelAlias: "model",
		Messages: []TextMessage{{Role: TextRoleUser, Content: "test"}},
	})
	if err == nil {
		t.Fatal("expected oversized response rejection")
	}
}

func TestAdapterGatewayTextForwardsInvocationKey(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Idempotency-Key"); got != "agent-task-brief" {
			t.Fatalf("Idempotency-Key = %q", got)
		}
		_, _ = writer.Write([]byte(`{"model":"text-v2","choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()
	adapter, _ := NewAdapterGatewayTextAdapter(textRouteStub{snapshot: textRouteSnapshot(server.URL)}, credentialStub("token"), false)
	adapter.client = server.Client()
	_, err := adapter.GenerateText(context.Background(), TextAdapterRequest{
		OrganizationID: "org_1", ModelAlias: "model", InvocationKey: "agent-task-brief",
		Messages: []TextMessage{{Role: TextRoleUser, Content: "test"}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAdapterGatewayTextReturnsStableRefusalCode(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"model":"text-v2","choices":[{"message":{"refusal":"policy"}}]}`))
	}))
	defer server.Close()
	adapter, _ := NewAdapterGatewayTextAdapter(textRouteStub{snapshot: textRouteSnapshot(server.URL)}, credentialStub("token"), false)
	adapter.client = server.Client()
	_, err := adapter.GenerateText(context.Background(), TextAdapterRequest{
		OrganizationID: "org_1", ModelAlias: "model",
		Messages: []TextMessage{{Role: TextRoleUser, Content: "test"}},
	})
	var executionError ExecutionError
	if !errors.As(err, &executionError) || executionError.JobError.Code != "MODEL_REFUSED" {
		t.Fatalf("error = %#v", err)
	}
}

func TestAdapterGatewayTextUsesPromptJSONForRoutesWithoutStrictSchema(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			ResponseFormat any `json:"response_format"`
			Messages       []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.ResponseFormat != nil {
			t.Fatalf("prompt_json request sent response_format: %#v", body.ResponseFormat)
		}
		if len(body.Messages) < 2 || body.Messages[0].Role != "system" ||
			!strings.Contains(body.Messages[0].Content, "JSON Schema") {
			t.Fatalf("messages = %#v", body.Messages)
		}
		_, _ = writer.Write([]byte(`{"model":"minimax","choices":[{"message":{"content":"not-json"}}]}`))
	}))
	defer server.Close()
	snapshot := textRouteSnapshot(server.URL)
	snapshot.TextResponseMode = TextResponsePromptJSON
	adapter, _ := NewAdapterGatewayTextAdapter(textRouteStub{snapshot: snapshot}, credentialStub("token"), false)
	adapter.client = server.Client()
	result, err := adapter.GenerateText(context.Background(), TextAdapterRequest{
		OrganizationID: "org_1", ModelAlias: "model",
		Messages:         []TextMessage{{Role: TextRoleUser, Content: "generate"}},
		OutputJSONSchema: []byte(`{"type":"object"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "not-json" || len(result.StructuredOutput) != 0 ||
		result.RouteSnapshot == nil || result.RouteSnapshot.TextResponseMode != TextResponsePromptJSON {
		t.Fatalf("result = %#v", result)
	}
}

func TestAdapterGatewayTextUsesJSONObjectMode(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		format, _ := body["response_format"].(map[string]any)
		if format["type"] != "json_object" {
			t.Fatalf("response_format = %#v", body["response_format"])
		}
		if temperature, ok := body["temperature"].(float64); !ok || temperature != 0 {
			t.Fatalf("temperature = %#v", body["temperature"])
		}
		_, _ = writer.Write([]byte(`{"model":"text","choices":[{"message":{"content":"{\"ok\":true}"}}]}`))
	}))
	defer server.Close()
	snapshot := textRouteSnapshot(server.URL)
	snapshot.TextResponseMode = TextResponseJSONObject
	snapshot.TemperatureSet = true
	adapter, _ := NewAdapterGatewayTextAdapter(textRouteStub{snapshot: snapshot}, credentialStub("token"), false)
	adapter.client = server.Client()
	result, err := adapter.GenerateText(context.Background(), TextAdapterRequest{
		OrganizationID: "org_1", ModelAlias: "model",
		Messages:         []TextMessage{{Role: TextRoleUser, Content: "generate"}},
		OutputJSONSchema: []byte(`{"type":"object"}`),
	})
	if err != nil || string(result.StructuredOutput) != `{"ok":true}` {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestAdapterGatewayTextForwardsThinkingMode(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		thinking, _ := body["thinking"].(map[string]any)
		if thinking["type"] != "disabled" {
			t.Fatalf("thinking = %#v", body["thinking"])
		}
		_, _ = writer.Write([]byte(`{"model":"text","choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()
	snapshot := textRouteSnapshot(server.URL)
	snapshot.ThinkingMode = "disabled"
	adapter, _ := NewAdapterGatewayTextAdapter(textRouteStub{snapshot: snapshot}, credentialStub("token"), false)
	adapter.client = server.Client()
	if _, err := adapter.GenerateText(context.Background(), TextAdapterRequest{
		OrganizationID: "org_1", ModelAlias: "model",
		Messages: []TextMessage{{Role: TextRoleUser, Content: "generate"}},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAdapterGatewayTextUsesMiniMaxResponseControls(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["max_completion_tokens"] != float64(2048) || body["max_tokens"] != nil || body["reasoning_split"] != true {
			t.Fatalf("MiniMax request body = %#v", body)
		}
		_, _ = writer.Write([]byte(`{"model":"MiniMax-M2.7","choices":[{"message":{"content":"{\"ok\":true}"}}]}`))
	}))
	defer server.Close()
	snapshot := textRouteSnapshot(server.URL)
	snapshot.TextResponseMode = TextResponsePromptJSON
	snapshot.MaxOutputTokens = 2048
	snapshot.OutputTokenParameter = TextOutputTokenParameterMaxCompletionTokens
	snapshot.ReasoningSplit = true
	adapter, _ := NewAdapterGatewayTextAdapter(textRouteStub{snapshot: snapshot}, credentialStub("token"), false)
	adapter.client = server.Client()
	if _, err := adapter.GenerateText(context.Background(), TextAdapterRequest{
		OrganizationID: "org_1", ModelAlias: "cookies.text.standard",
		Messages: []TextMessage{{Role: TextRoleUser, Content: "generate"}},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAdapterGatewayTextInspectionDoesNotInvokeModel(t *testing.T) {
	t.Parallel()
	snapshot := textRouteSnapshot("https://gateway.example")
	adapter, _ := NewAdapterGatewayTextAdapter(textRouteStub{snapshot: snapshot}, credentialStub("token"), false)
	inspection, err := adapter.InspectTextRoute(context.Background(), "org_1", "cookies.text.standard")
	if err != nil {
		t.Fatal(err)
	}
	if !inspection.Ready || inspection.RouteRevisionID != snapshot.RouteRevisionID ||
		inspection.ResponseMode != TextResponseJSONSchema {
		t.Fatalf("inspection = %#v", inspection)
	}
}

func TestAdapterGatewayTextUsesBackgroundResponsesLifecycle(t *testing.T) {
	t.Parallel()
	var polls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPost && request.URL.Path == "/v1/responses" {
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			text, _ := body["text"].(map[string]any)
			format, _ := text["format"].(map[string]any)
			reasoning, _ := body["reasoning"].(map[string]any)
			if body["background"] != true || format["type"] != "json_schema" || reasoning["effort"] != "high" {
				t.Fatalf("Responses body = %#v", body)
			}
			_, _ = writer.Write([]byte(`{"id":"resp_1","model":"gpt-5.5-pro","status":"in_progress"}`))
			return
		}
		if request.Method == http.MethodGet && request.URL.Path == "/v1/responses/resp_1" {
			polls.Add(1)
			_, _ = writer.Write([]byte(`{"id":"resp_1","model":"gpt-5.5-pro","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"{\"summary\":\"ready\"}"}]}],"usage":{"input_tokens":20,"output_tokens":8,"total_tokens":28}}`))
			return
		}
		t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
	}))
	defer server.Close()
	snapshot := textRouteSnapshot(server.URL)
	snapshot.TextAPIMode = TextAPIResponses
	snapshot.Background = true
	snapshot.ReasoningEffort = "high"
	snapshot.MaxOutputTokens = 4096
	snapshot.OutputTokenParameter = TextOutputTokenParameterMaxOutputTokens
	snapshot.PollIntervalMS = 100
	adapter, _ := NewAdapterGatewayTextAdapter(textRouteStub{snapshot: snapshot}, credentialStub("token"), false)
	adapter.client = server.Client()
	result, err := adapter.GenerateText(context.Background(), TextAdapterRequest{
		OrganizationID: "org_1", ModelAlias: "cookies.text.deep_review", InvocationKey: "deep-review-1",
		Messages:         []TextMessage{{Role: TextRoleSystem, Content: "Review."}, {Role: TextRoleUser, Content: "candidate"}},
		OutputJSONSchema: []byte(`{"type":"object"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if polls.Load() != 1 || string(result.StructuredOutput) != `{"summary":"ready"}` ||
		result.Usage == nil || result.Usage.TotalTokens != 28 {
		t.Fatalf("result=%#v polls=%d", result, polls.Load())
	}
}

func TestResponsesEndpointPreservesVersionedProviderBasePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{name: "unversioned", baseURL: "https://api.openai.com", want: "https://api.openai.com/v1/responses"},
		{name: "v1", baseURL: "https://api.openai.com/v1", want: "https://api.openai.com/v1/responses"},
		{name: "nested v3", baseURL: "https://ark.cn-beijing.volces.com/api/v3", want: "https://ark.cn-beijing.volces.com/api/v3/responses"},
		{name: "trailing slash", baseURL: "https://ark.cn-beijing.volces.com/api/v3/", want: "https://ark.cn-beijing.volces.com/api/v3/responses"},
		{name: "version-like host", baseURL: "https://v3", want: "https://v3/v1/responses"},
		{name: "non-numeric version", baseURL: "https://gateway.example/api/v3beta", want: "https://gateway.example/api/v3beta/v1/responses"},
		{name: "signed version", baseURL: "https://gateway.example/api/v+3", want: "https://gateway.example/api/v+3/v1/responses"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := responsesEndpoint(test.baseURL); got != test.want {
				t.Fatalf("responsesEndpoint(%q) = %q, want %q", test.baseURL, got, test.want)
			}
		})
	}
}

type textRouteStub struct {
	snapshot ImageRouteSnapshot
	err      error
}

func (s textRouteStub) ResolveTextRoute(context.Context, contract.OrganizationID, string) (ImageRouteSnapshot, error) {
	return s.snapshot, s.err
}

type credentialStub string

func (s credentialStub) ResolveGatewayCredential(context.Context, string, int64) (string, error) {
	if s == "" {
		return "", fmt.Errorf("missing")
	}
	return string(s), nil
}

func textRouteSnapshot(baseURL string) ImageRouteSnapshot {
	return ImageRouteSnapshot{
		RouteID: "route_1", RouteRevisionID: "routerev_1", ConnectionID: "connection_1",
		ConnectionRevisionID: "connectionrev_1", BaseURL: baseURL, UpstreamModel: "text-v1",
		CredentialID: "credential_1", CredentialVersion: 1, TimeoutSeconds: 5, MaxResponseBytes: 1 << 20,
		TextResponseMode: TextResponseJSONSchema,
	}
}

func TestGatewayTextImageContentInBothAPIModes(t *testing.T) {
	for _, mode := range []TextAPIMode{TextAPIChatCompletions, TextAPIResponses} {
		t.Run(string(mode), func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				key := "messages"
				if mode == TextAPIResponses {
					key = "input"
				}
				if !strings.Contains(string(body[key]), "data:image/jpeg;base64,aW1hZ2U=") || !strings.Contains(string(body[key]), "candidate:7") {
					t.Fatalf("image lost: %s", body[key])
				}
				w.Header().Set("Content-Type", "application/json")
				if mode == TextAPIResponses {
					fmt.Fprint(w, `{"id":"1","model":"test","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`)
				} else {
					fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
				}
			}))
			defer server.Close()
			route := textRouteSnapshot(server.URL)
			route.TextAPIMode = mode
			if mode == TextAPIResponses {
				route.OutputTokenParameter = TextOutputTokenParameterMaxOutputTokens
			}
			adapter, _ := NewAdapterGatewayTextAdapter(textRouteStub{snapshot: route}, credentialStub("token"), false)
			adapter.client = server.Client()
			_, err := adapter.GenerateText(context.Background(), TextAdapterRequest{OrganizationID: "org_1", ModelAlias: "text", Messages: []TextMessage{{Role: TextRoleUser, Content: "candidate:7", Images: []TextImage{{MIMEType: "image/jpeg", Data: []byte("image")}}}}})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
