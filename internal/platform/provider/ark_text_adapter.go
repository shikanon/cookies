package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	arkTextDefaultBaseURL = "https://ark.cn-beijing.volces.com/api/v3"
	arkTextMaxResponse    = 1 << 20
)

// ArkTextConfig holds server-only Ark credentials and model selection.
type ArkTextConfig struct {
	APIKey  string
	Model   string
	BaseURL string
}

// ArkTextAdapter implements Ark's OpenAI-compatible chat completions API.
// It returns only provider-neutral output and never exposes request secrets.
type ArkTextAdapter struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

func NewArkTextAdapter(config ArkTextConfig) (*ArkTextAdapter, error) {
	if strings.TrimSpace(config.APIKey) == "" || strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("Ark text API key and model are required")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = arkTextDefaultBaseURL
	}
	return &ArkTextAdapter{
		apiKey:  config.APIKey,
		model:   config.Model,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Minute},
	}, nil
}

func (a *ArkTextAdapter) GenerateText(ctx context.Context, request TextAdapterRequest) (SynchronousResult, error) {
	if strings.TrimSpace(request.ModelAlias) == "" || len(request.Messages) == 0 {
		return SynchronousResult{}, fmt.Errorf("Ark text request requires a model alias and messages")
	}
	for index, message := range request.Messages {
		if err := message.Validate(); err != nil {
			return SynchronousResult{}, fmt.Errorf("invalid Ark text message at index %d: %w", index, err)
		}
	}
	if err := validateOptionalJSONObject(request.OutputJSONSchema); err != nil {
		return SynchronousResult{}, err
	}

	payload := arkTextRequest{Model: a.model, Messages: request.Messages}
	if len(request.OutputJSONSchema) > 0 {
		payload.ResponseFormat = &arkTextResponseFormat{
			Type: "json_schema",
			JSONSchema: arkTextJSONSchema{
				Name:   "structured_response",
				Schema: request.OutputJSONSchema,
			},
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return SynchronousResult{}, fmt.Errorf("encode Ark text request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return SynchronousResult{}, fmt.Errorf("build Ark text request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+a.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")

	response, err := a.client.Do(httpRequest)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return SynchronousResult{}, fmt.Errorf("Ark text request timed out: %w", context.DeadlineExceeded)
		}
		if errors.Is(err, context.Canceled) {
			return SynchronousResult{}, context.Canceled
		}
		return SynchronousResult{}, fmt.Errorf("Ark text request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return SynchronousResult{}, fmt.Errorf("Ark text request returned HTTP %d", response.StatusCode)
	}
	var decoded arkTextResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, arkTextMaxResponse)).Decode(&decoded); err != nil {
		return SynchronousResult{}, fmt.Errorf("Ark text response could not be verified")
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return SynchronousResult{}, fmt.Errorf("Ark text response contained no output")
	}
	modelVersion := strings.TrimSpace(decoded.Model)
	if modelVersion == "" {
		modelVersion = a.model
	}
	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	result := SynchronousResult{ProviderCode: arkProviderCode, ModelVersion: modelVersion}
	if len(request.OutputJSONSchema) > 0 {
		if !json.Valid([]byte(content)) {
			return SynchronousResult{}, fmt.Errorf("Ark text structured response is invalid")
		}
		result.StructuredOutput = json.RawMessage(content)
	} else {
		result.Text = content
	}
	return result, nil
}

type arkTextRequest struct {
	Model          string                 `json:"model"`
	Messages       []TextMessage          `json:"messages"`
	ResponseFormat *arkTextResponseFormat `json:"response_format,omitempty"`
}

type arkTextResponseFormat struct {
	Type       string            `json:"type"`
	JSONSchema arkTextJSONSchema `json:"json_schema"`
}

type arkTextJSONSchema struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
}

type arkTextResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}
