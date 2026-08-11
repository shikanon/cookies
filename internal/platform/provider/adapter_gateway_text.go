package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type AdapterGatewayTextAdapter struct {
	routes            TextRouteResolver
	credentials       GatewayCredentialResolver
	client            *http.Client
	allowInsecureHTTP bool
}

func NewAdapterGatewayTextAdapter(routes TextRouteResolver, credentials GatewayCredentialResolver, allowInsecureHTTP bool) (*AdapterGatewayTextAdapter, error) {
	if routes == nil || credentials == nil {
		return nil, fmt.Errorf("text route and credential resolvers are required")
	}
	return &AdapterGatewayTextAdapter{routes: routes, credentials: credentials, client: &http.Client{}, allowInsecureHTTP: allowInsecureHTTP}, nil
}

func (a *AdapterGatewayTextAdapter) GenerateText(ctx context.Context, request TextAdapterRequest) (SynchronousResult, error) {
	if request.OrganizationID == "" || strings.TrimSpace(request.ModelAlias) == "" || len(request.Messages) == 0 {
		return SynchronousResult{}, fmt.Errorf("adapter gateway text request is invalid")
	}
	route, err := a.routes.ResolveTextRoute(ctx, request.OrganizationID, request.ModelAlias)
	if err != nil {
		return SynchronousResult{}, err
	}
	if err := route.ValidateTextWithPolicy(a.allowInsecureHTTP); err != nil {
		return SynchronousResult{}, err
	}
	token, err := a.credentials.ResolveGatewayCredential(ctx, route.CredentialID, route.CredentialVersion)
	if err != nil {
		return SynchronousResult{}, gatewayExecutionError("MODEL_AUTH_UNAVAILABLE", "Adapter gateway credential could not be resolved")
	}
	messages := make([]map[string]string, 0, len(request.Messages)+1)
	for _, message := range request.Messages {
		if err := message.Validate(); err != nil {
			return SynchronousResult{}, err
		}
		messages = append(messages, map[string]string{"role": string(message.Role), "content": message.Content})
	}
	if route.TextAPIMode == TextAPIResponses {
		return a.generateResponses(ctx, request, route, token, messages)
	}
	body := map[string]any{"model": route.UpstreamModel, "messages": messages}
	if len(request.OutputJSONSchema) > 0 {
		var schema any
		if err := json.Unmarshal(request.OutputJSONSchema, &schema); err != nil {
			return SynchronousResult{}, err
		}
		switch route.TextResponseMode {
		case TextResponseJSONSchema:
			body["response_format"] = map[string]any{
				"type":        "json_schema",
				"json_schema": map[string]any{"name": "cookies_strategy_output", "strict": true, "schema": schema},
			}
		case TextResponseJSONObject:
			body["response_format"] = map[string]any{"type": "json_object"}
			messages = prependSchemaInstruction(messages, request.OutputJSONSchema)
			body["messages"] = messages
		case TextResponsePromptJSON:
			messages = prependSchemaInstruction(messages, request.OutputJSONSchema)
			body["messages"] = messages
		default:
			return SynchronousResult{}, fmt.Errorf("adapter gateway text response mode is invalid")
		}
	}
	if route.MaxOutputTokens > 0 {
		switch route.OutputTokenParameter {
		case "", TextOutputTokenParameterMaxTokens:
			body["max_tokens"] = route.MaxOutputTokens
		case TextOutputTokenParameterMaxCompletionTokens:
			body["max_completion_tokens"] = route.MaxOutputTokens
		default:
			return SynchronousResult{}, fmt.Errorf("adapter gateway output token parameter is invalid")
		}
	}
	if route.TemperatureSet {
		body["temperature"] = route.Temperature
	}
	if route.ThinkingMode != "" {
		body["thinking"] = map[string]string{"type": route.ThinkingMode}
	}
	if route.ReasoningSplit {
		body["reasoning_split"] = true
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
	httpRequest.Header.Set("Accept", "application/json")
	if request.InvocationKey != "" {
		httpRequest.Header.Set("Idempotency-Key", string(request.InvocationKey))
	}
	response, err := a.client.Do(httpRequest)
	if err != nil {
		if requestCtx.Err() == context.DeadlineExceeded {
			return SynchronousResult{}, ExecutionError{JobError: contract.JobError{
				Code: "MODEL_TIMEOUT", Message: "Adapter gateway text request timed out", Retryable: true,
			}}
		}
		return SynchronousResult{}, gatewayExecutionError("PROVIDER_UNAVAILABLE", "Adapter gateway text request failed")
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, route.MaxResponseBytes+1))
	if err != nil || int64(len(responseBody)) > route.MaxResponseBytes {
		return SynchronousResult{}, gatewayExecutionError("MODEL_RESPONSE_INVALID", "Adapter gateway response exceeded the safety limit")
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
	if err := json.Unmarshal(responseBody, &decoded); err != nil || len(decoded.Choices) != 1 {
		return SynchronousResult{}, gatewayExecutionError("MODEL_RESPONSE_INVALID", "Adapter gateway returned an invalid text response")
	}
	if strings.TrimSpace(decoded.Choices[0].Message.Refusal) != "" {
		return SynchronousResult{}, gatewayExecutionError("MODEL_REFUSED", "The model refused the Strategy request")
	}
	if strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return SynchronousResult{}, gatewayExecutionError("MODEL_RESPONSE_INVALID", "Adapter gateway returned an invalid text response")
	}
	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	result := SynchronousResult{
		ProviderCode: adapterGatewayProviderCode, ModelVersion: strings.TrimSpace(decoded.Model),
		Text: content, RouteSnapshot: &route,
	}
	if result.ModelVersion == "" {
		result.ModelVersion = route.UpstreamModel
	}
	if len(request.OutputJSONSchema) > 0 {
		if !json.Valid([]byte(content)) {
			if route.TextResponseMode != TextResponsePromptJSON {
				return SynchronousResult{}, gatewayExecutionError("MODEL_OUTPUT_INVALID", "Structured text output is not valid JSON")
			}
		} else {
			result.StructuredOutput = json.RawMessage(content)
		}
	}
	if decoded.Usage.TotalTokens > 0 {
		result.Usage = &TokenUsage{
			InputTokens: decoded.Usage.PromptTokens, OutputTokens: decoded.Usage.CompletionTokens,
			TotalTokens: decoded.Usage.TotalTokens,
		}
	}
	return result, nil
}

func (a *AdapterGatewayTextAdapter) InspectTextRoute(ctx context.Context, organizationID contract.OrganizationID, modelAlias string) (TextRouteInspection, error) {
	route, err := a.routes.ResolveTextRoute(ctx, organizationID, modelAlias)
	if err != nil {
		return TextRouteInspection{}, err
	}
	if err := route.ValidateTextWithPolicy(a.allowInsecureHTTP); err != nil {
		return TextRouteInspection{}, err
	}
	if _, err := a.credentials.ResolveGatewayCredential(ctx, route.CredentialID, route.CredentialVersion); err != nil {
		return TextRouteInspection{}, gatewayExecutionError("MODEL_AUTH_UNAVAILABLE", "Adapter gateway credential could not be resolved")
	}
	return TextRouteInspection{
		ModelAlias: modelAlias, UpstreamModel: route.UpstreamModel,
		RouteRevisionID: route.RouteRevisionID, ResponseMode: route.TextResponseMode,
		APIMode: route.TextAPIMode, Background: route.Background, Ready: true,
	}, nil
}

func prependSchemaInstruction(messages []map[string]string, schema json.RawMessage) []map[string]string {
	instruction := map[string]string{
		"role":    "system",
		"content": "Return exactly one JSON object matching this JSON Schema. Do not use Markdown fences or add commentary.\n" + string(schema),
	}
	return append([]map[string]string{instruction}, messages...)
}
