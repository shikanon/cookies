package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestArkTextAdapterAllowsLongResponsesAndPreservesTimeout(t *testing.T) {
	adapter, err := NewArkTextAdapter(ArkTextConfig{APIKey: "secret-key", Model: "doubao-test"})
	if err != nil {
		t.Fatal(err)
	}
	if adapter.client.Timeout < 10*time.Minute {
		t.Fatalf("model response timeout too short: %s", adapter.client.Timeout)
	}
	adapter.client.Transport = roundTripper(func(*http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	})
	_, err = adapter.GenerateText(context.Background(), TextAdapterRequest{
		ModelAlias: "cookies.text.standard", Messages: []TextMessage{{Role: TextRoleUser, Content: "Return JSON."}},
	})
	if !errors.Is(err, context.DeadlineExceeded) || bytes.Contains([]byte(err.Error()), []byte("secret-key")) {
		t.Fatalf("expected sanitized timeout, got %v", err)
	}
}

func TestArkTextAdapterSendsMessagesAndNormalizesTextResponse(t *testing.T) {
	t.Parallel()
	adapter, err := NewArkTextAdapter(ArkTextConfig{
		APIKey:  "test-key",
		Model:   "doubao-test",
		BaseURL: "https://ark.example.test/api/v3",
	})
	if err != nil {
		t.Fatalf("NewArkTextAdapter() error = %v", err)
	}
	adapter.client = &http.Client{Transport: roundTripper(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v3/chat/completions" {
			t.Fatalf("unexpected Ark request: %s %s", request.Method, request.URL)
		}
		if got, want := request.Header.Get("Authorization"), "Bearer test-key"; got != want {
			t.Fatalf("Authorization = %q, want %q", got, want)
		}
		var body struct {
			Model    string        `json:"model"`
			Messages []TextMessage `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Model != "doubao-test" || len(body.Messages) != 2 || body.Messages[0].Role != TextRoleSystem || body.Messages[1].Content != "Write a slogan." {
			t.Fatalf("unexpected request body: %#v", body)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString(`{"model":"doubao-test-202607","choices":[{"message":{"content":"Fresh choices, simply made."}}]}`)),
		}, nil
	})}

	result, err := adapter.GenerateText(context.Background(), TextAdapterRequest{
		ModelAlias: "cookies.text.standard",
		Messages: []TextMessage{
			{Role: TextRoleSystem, Content: "You write concise advertising copy."},
			{Role: TextRoleUser, Content: "Write a slogan."},
		},
	})
	if err != nil {
		t.Fatalf("GenerateText() error = %v", err)
	}
	if result.ProviderCode != arkProviderCode || result.ModelVersion != "doubao-test-202607" || result.Text != "Fresh choices, simply made." || len(result.StructuredOutput) != 0 {
		t.Fatalf("unexpected normalized result: %#v", result)
	}
}

func TestArkTextAdapterRequestsAndReturnsStructuredOutput(t *testing.T) {
	t.Parallel()
	adapter, err := NewArkTextAdapter(ArkTextConfig{APIKey: "test-key", Model: "doubao-test", BaseURL: "https://ark.example.test"})
	if err != nil {
		t.Fatalf("NewArkTextAdapter() error = %v", err)
	}
	adapter.client = &http.Client{Transport: roundTripper(func(request *http.Request) (*http.Response, error) {
		var body struct {
			ResponseFormat *struct {
				Type       string `json:"type"`
				JSONSchema struct {
					Schema json.RawMessage `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.ResponseFormat == nil || body.ResponseFormat.Type != "json_schema" || !json.Valid(body.ResponseFormat.JSONSchema.Schema) {
			t.Fatalf("unexpected response format: %#v", body.ResponseFormat)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString(`{"choices":[{"message":{"content":"{\"headline\":\"Fresh choices\"}"}}]}`)),
		}, nil
	})}

	result, err := adapter.GenerateText(context.Background(), TextAdapterRequest{
		ModelAlias:       "cookies.text.standard",
		Messages:         []TextMessage{{Role: TextRoleUser, Content: "Return JSON."}},
		OutputJSONSchema: json.RawMessage(`{"type":"object","properties":{"headline":{"type":"string"}},"required":["headline"]}`),
	})
	if err != nil {
		t.Fatalf("GenerateText() error = %v", err)
	}
	if got, want := string(result.StructuredOutput), `{"headline":"Fresh choices"}`; got != want || result.Text != "" || result.ModelVersion != "doubao-test" {
		t.Fatalf("unexpected structured result: %#v", result)
	}
}

func TestArkTextAdapterRejectsInvalidResponseWithoutExposingCredential(t *testing.T) {
	t.Parallel()
	adapter, err := NewArkTextAdapter(ArkTextConfig{APIKey: "secret-key", Model: "doubao-test", BaseURL: "https://ark.example.test"})
	if err != nil {
		t.Fatalf("NewArkTextAdapter() error = %v", err)
	}
	adapter.client = &http.Client{Transport: roundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString(`{"error":"invalid key"}`)),
		}, nil
	})}
	_, err = adapter.GenerateText(context.Background(), TextAdapterRequest{
		ModelAlias: "cookies.text.standard",
		Messages:   []TextMessage{{Role: TextRoleUser, Content: "Write a slogan."}},
	})
	if err == nil || bytes.Contains([]byte(err.Error()), []byte("secret-key")) {
		t.Fatalf("GenerateText() error = %v, want sanitized rejection", err)
	}
}

func TestArkTextSendsImageWithCandidateCaption(t *testing.T) {
	adapter, _ := NewArkTextAdapter(ArkTextConfig{APIKey: "test", Model: "vision"})
	adapter.client.Transport = roundTripper(func(request *http.Request) (*http.Response, error) {
		var body struct {
			Messages []struct {
				Content []struct {
					Type     string `json:"type"`
					Text     string `json:"text"`
					ImageURL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				} `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		parts := body.Messages[0].Content
		if len(parts) != 2 || parts[0].Text != "candidate:7" || parts[1].ImageURL.URL != "data:image/jpeg;base64,aW1hZ2U=" {
			t.Fatalf("image content lost: %+v", parts)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{"choices":[{"message":{"content":"ok"}}]}`)), Header: make(http.Header)}, nil
	})
	_, err := adapter.GenerateText(context.Background(), TextAdapterRequest{ModelAlias: "text", Messages: []TextMessage{{Role: TextRoleUser, Content: "candidate:7", Images: []TextImage{{MIMEType: "image/jpeg", Data: []byte("image")}}}}})
	if err != nil {
		t.Fatal(err)
	}
}
