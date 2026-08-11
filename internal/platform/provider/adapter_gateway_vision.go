package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type AdapterGatewayVisionAdapter struct {
	routes            VisionRouteResolver
	credentials       GatewayCredentialResolver
	client            *http.Client
	allowInsecureHTTP bool
}

func NewAdapterGatewayVisionAdapter(routes VisionRouteResolver, credentials GatewayCredentialResolver, allowInsecureHTTP bool) (*AdapterGatewayVisionAdapter, error) {
	if routes == nil || credentials == nil {
		return nil, fmt.Errorf("vision route and credential resolvers are required")
	}
	return &AdapterGatewayVisionAdapter{routes: routes, credentials: credentials, client: &http.Client{}, allowInsecureHTTP: allowInsecureHTTP}, nil
}

func (a *AdapterGatewayVisionAdapter) UnderstandVision(ctx context.Context, request VisionAdapterRequest) (SynchronousResult, error) {
	if strings.TrimSpace(request.ModelAlias) == "" || strings.TrimSpace(request.Input.Instruction) == "" || len(request.Sources) == 0 || len(request.Sources) > 8 {
		return SynchronousResult{}, fmt.Errorf("adapter gateway vision request is invalid")
	}
	if request.OrganizationID == "" {
		return SynchronousResult{}, fmt.Errorf("adapter gateway vision organization context is missing")
	}
	for _, source := range request.Sources {
		if source.Reference.ProjectID == "" {
			return SynchronousResult{}, fmt.Errorf("adapter gateway vision source is invalid")
		}
	}
	route, err := a.routes.ResolveVisionRoute(ctx, request.OrganizationID, request.ModelAlias)
	if err != nil {
		return SynchronousResult{}, err
	}
	if err := route.ValidateTextWithPolicy(a.allowInsecureHTTP); err != nil {
		return SynchronousResult{}, err
	}
	if route.TextAPIMode == TextAPIResponses {
		return SynchronousResult{}, gatewayExecutionError("MODEL_INPUT_UNSUPPORTED", "Vision understanding currently requires a chat-completions route")
	}
	token, err := a.credentials.ResolveGatewayCredential(ctx, route.CredentialID, route.CredentialVersion)
	if err != nil {
		return SynchronousResult{}, gatewayExecutionError("MODEL_AUTH_UNAVAILABLE", "Adapter gateway credential could not be resolved")
	}
	content := []any{map[string]any{"type": "text", "text": request.Input.Instruction}}
	totalBytes := int64(0)
	for _, source := range request.Sources {
		if source.MIMEType != "image/png" && source.MIMEType != "image/jpeg" {
			return SynchronousResult{}, gatewayExecutionError("MODEL_INPUT_UNSUPPORTED", "Vision understanding accepts PNG or JPEG sources")
		}
		data, readErr := io.ReadAll(io.LimitReader(source.Content, (20<<20)+1))
		if readErr != nil || len(data) == 0 || len(data) > 20<<20 {
			return SynchronousResult{}, gatewayExecutionError("MODEL_INPUT_UNSUPPORTED", "Vision source exceeded the safety limit")
		}
		totalBytes += int64(len(data))
		if totalBytes > 40<<20 {
			return SynchronousResult{}, gatewayExecutionError("MODEL_INPUT_UNSUPPORTED", "Vision sources exceeded the total safety limit")
		}
		content = append(content, map[string]any{
			"type": "image_url", "image_url": map[string]string{"url": "data:" + source.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(data)},
		})
	}
	messages := []any{map[string]any{"role": "user", "content": content}}
	body := map[string]any{"model": route.UpstreamModel, "messages": messages}
	if len(request.Input.OutputJSONSchema) > 0 {
		var schema any
		if err := json.Unmarshal(request.Input.OutputJSONSchema, &schema); err != nil {
			return SynchronousResult{}, err
		}
		switch route.TextResponseMode {
		case TextResponseJSONSchema:
			body["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "cookies_media_understanding", "strict": true, "schema": schema}}
		case TextResponseJSONObject:
			body["response_format"] = map[string]any{"type": "json_object"}
			messages = append([]any{map[string]any{"role": "system", "content": "Return exactly one JSON object matching this JSON Schema.\n" + string(request.Input.OutputJSONSchema)}}, messages...)
			body["messages"] = messages
		case TextResponsePromptJSON:
			messages = append([]any{map[string]any{"role": "system", "content": "Return exactly one JSON object matching this JSON Schema.\n" + string(request.Input.OutputJSONSchema)}}, messages...)
			body["messages"] = messages
		default:
			return SynchronousResult{}, fmt.Errorf("adapter gateway vision response mode is invalid")
		}
	}
	if route.MaxOutputTokens > 0 {
		if route.OutputTokenParameter == "" || route.OutputTokenParameter == TextOutputTokenParameterMaxTokens {
			body["max_tokens"] = route.MaxOutputTokens
		} else if route.OutputTokenParameter == TextOutputTokenParameterMaxCompletionTokens {
			body["max_completion_tokens"] = route.MaxOutputTokens
		}
	}
	if route.TemperatureSet {
		body["temperature"] = route.Temperature
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return SynchronousResult{}, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(route.TimeoutSeconds)*time.Second)
	defer cancel()
	endpoint := route.ChatCompletionsEndpoint()
	httpRequest, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return SynchronousResult{}, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+token)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(httpRequest)
	if err != nil {
		if requestCtx.Err() == context.DeadlineExceeded {
			return SynchronousResult{}, ExecutionError{JobError: contract.JobError{Code: "MODEL_TIMEOUT", Message: "Adapter gateway vision request timed out", Retryable: true}}
		}
		return SynchronousResult{}, gatewayExecutionError("PROVIDER_UNAVAILABLE", "Adapter gateway vision request failed")
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, route.MaxResponseBytes+1))
	if err != nil || int64(len(responseBody)) > route.MaxResponseBytes {
		return SynchronousResult{}, gatewayExecutionError("MODEL_RESPONSE_INVALID", "Adapter gateway vision response exceeded the safety limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return SynchronousResult{}, mapGatewayTextHTTPError(response.StatusCode, responseBody)
	}
	var decoded struct {
		Model string `json:"model"`
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			TotalTokens      int64 `json:"total_tokens"`
		} `json:"usage"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
				Refusal string `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(responseBody, &decoded) != nil || len(decoded.Choices) != 1 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return SynchronousResult{}, gatewayExecutionError("MODEL_RESPONSE_INVALID", "Adapter gateway returned an invalid vision response")
	}
	if strings.TrimSpace(decoded.Choices[0].Message.Refusal) != "" {
		return SynchronousResult{}, gatewayExecutionError("MODEL_REFUSED", "The model refused the media understanding request")
	}
	result := SynchronousResult{ProviderCode: adapterGatewayProviderCode, ModelVersion: strings.TrimSpace(decoded.Model), Text: strings.TrimSpace(decoded.Choices[0].Message.Content), RouteSnapshot: &route}
	if result.ModelVersion == "" {
		result.ModelVersion = route.UpstreamModel
	}
	if len(request.Input.OutputJSONSchema) > 0 && json.Valid([]byte(result.Text)) {
		result.StructuredOutput = json.RawMessage(result.Text)
	}
	if decoded.Usage.TotalTokens > 0 {
		result.Usage = &TokenUsage{InputTokens: decoded.Usage.PromptTokens, OutputTokens: decoded.Usage.CompletionTokens, TotalTokens: decoded.Usage.TotalTokens}
	}
	return result, nil
}
