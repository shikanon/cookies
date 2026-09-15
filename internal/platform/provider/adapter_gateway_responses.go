package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type gatewayResponsesPayload struct {
	ID     string `json:"id"`
	Model  string `json:"model"`
	Status string `json:"status"`
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Refusal string `json:"refusal"`
		} `json:"content"`
	} `json:"output"`
	Usage struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
		TotalTokens  int64 `json:"total_tokens"`
	} `json:"usage"`
}

func (a *AdapterGatewayTextAdapter) generateResponses(
	ctx context.Context,
	request TextAdapterRequest,
	route GatewayRouteSnapshot,
	token string,
) (SynchronousResult, error) {
	input := make([]map[string]any, 0, len(request.Messages))
	instructions := []string{}
	for _, message := range request.Messages {
		if message.Role == TextRoleSystem {
			instructions = append(instructions, message.Content)
			continue
		}
		var content any = message.Content
		if len(message.Images) > 0 {
			parts := []map[string]any{{"type": "input_text", "text": message.Content}}
			for _, img := range message.Images {
				parts = append(parts, map[string]any{"type": "input_image", "image_url": img.dataURL()})
			}
			content = parts
		}
		input = append(input, map[string]any{"role": message.Role, "content": content})
	}
	body := map[string]any{"model": route.UpstreamModel, "input": input}
	if len(instructions) > 0 {
		body["instructions"] = strings.Join(instructions, "\n\n")
	}
	if route.Background {
		body["background"] = true
	}
	if route.ReasoningEffort != "" {
		body["reasoning"] = map[string]any{"effort": route.ReasoningEffort}
	}
	if route.MaxOutputTokens > 0 {
		body["max_output_tokens"] = route.MaxOutputTokens
	}
	if route.TemperatureSet {
		body["temperature"] = route.Temperature
	}
	if len(request.OutputJSONSchema) > 0 {
		var schema any
		if err := json.Unmarshal(request.OutputJSONSchema, &schema); err != nil {
			return SynchronousResult{}, err
		}
		switch route.TextResponseMode {
		case TextResponseJSONSchema:
			body["text"] = map[string]any{"format": map[string]any{
				"type": "json_schema", "name": "cookies_strategy_output", "strict": true, "schema": schema,
			}}
		case TextResponseJSONObject:
			body["text"] = map[string]any{"format": map[string]any{"type": "json_object"}}
			body["instructions"] = appendResponseInstruction(body["instructions"], request.OutputJSONSchema)
		case TextResponsePromptJSON:
			body["instructions"] = appendResponseInstruction(body["instructions"], request.OutputJSONSchema)
		default:
			return SynchronousResult{}, fmt.Errorf("adapter gateway text response mode is invalid")
		}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return SynchronousResult{}, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(route.TimeoutSeconds)*time.Second)
	defer cancel()
	endpoint := responsesEndpoint(route.BaseURL)
	payload, err := a.doResponsesRequest(
		requestCtx, http.MethodPost, endpoint, encoded, token, request.InvocationKey, route.MaxResponseBytes,
	)
	if err != nil {
		return SynchronousResult{}, err
	}
	if route.Background {
		payload, err = a.pollResponse(requestCtx, endpoint, payload, token, request.InvocationKey, route)
		if err != nil {
			return SynchronousResult{}, err
		}
	}
	return normalizeResponsesPayload(payload, request, route)
}

func (a *AdapterGatewayTextAdapter) pollResponse(
	ctx context.Context,
	endpoint string,
	payload gatewayResponsesPayload,
	token string,
	invocationKey contract.IdempotencyKey,
	route GatewayRouteSnapshot,
) (gatewayResponsesPayload, error) {
	interval := time.Second
	if route.PollIntervalMS > 0 {
		interval = time.Duration(route.PollIntervalMS) * time.Millisecond
	}
	for {
		switch payload.Status {
		case "completed":
			return payload, nil
		case "failed", "cancelled", "incomplete":
			return gatewayResponsesPayload{}, responsesStatusError(payload.Status)
		}
		if strings.TrimSpace(payload.ID) == "" {
			return gatewayResponsesPayload{}, gatewayExecutionError(
				"MODEL_RESPONSE_INVALID", "Adapter gateway returned a background response without an ID",
			)
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			a.cancelBackgroundResponse(endpoint, payload.ID, token)
			if ctx.Err() == context.DeadlineExceeded {
				return gatewayResponsesPayload{}, ExecutionError{JobError: contract.JobError{
					Code: "MODEL_TIMEOUT", Message: "Adapter gateway background response timed out", Retryable: true,
				}}
			}
			return gatewayResponsesPayload{}, ctx.Err()
		case <-timer.C:
		}
		var err error
		payload, err = a.doResponsesRequest(
			ctx, http.MethodGet, endpoint+"/"+url.PathEscape(payload.ID), nil, token, invocationKey, route.MaxResponseBytes,
		)
		if err != nil {
			return gatewayResponsesPayload{}, err
		}
	}
}

func (a *AdapterGatewayTextAdapter) doResponsesRequest(
	ctx context.Context,
	method string,
	endpoint string,
	body []byte,
	token string,
	invocationKey contract.IdempotencyKey,
	maxResponseBytes int64,
) (gatewayResponsesPayload, error) {
	httpRequest, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return gatewayResponsesPayload{}, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+token)
	httpRequest.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	if invocationKey != "" && method == http.MethodPost {
		httpRequest.Header.Set("Idempotency-Key", string(invocationKey))
	}
	response, err := a.client.Do(httpRequest)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return gatewayResponsesPayload{}, ExecutionError{JobError: contract.JobError{
				Code: "MODEL_TIMEOUT", Message: "Adapter gateway Responses request timed out", Retryable: true,
			}}
		}
		return gatewayResponsesPayload{}, gatewayExecutionError("PROVIDER_UNAVAILABLE", "Adapter gateway Responses request failed")
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || int64(len(responseBody)) > maxResponseBytes {
		return gatewayResponsesPayload{}, gatewayExecutionError(
			"MODEL_RESPONSE_INVALID", "Adapter gateway response exceeded the safety limit",
		)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return gatewayResponsesPayload{}, mapGatewayTextHTTPError(response.StatusCode, responseBody)
	}
	var payload gatewayResponsesPayload
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return gatewayResponsesPayload{}, gatewayExecutionError(
			"MODEL_RESPONSE_INVALID", "Adapter gateway returned an invalid Responses payload",
		)
	}
	return payload, nil
}

func (a *AdapterGatewayTextAdapter) cancelBackgroundResponse(endpoint, responseID, token string) {
	cancelCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = a.doResponsesRequest(
		cancelCtx, http.MethodPost, endpoint+"/"+url.PathEscape(responseID)+"/cancel",
		[]byte("{}"), token, "", 1<<20,
	)
}

func normalizeResponsesPayload(
	payload gatewayResponsesPayload,
	request TextAdapterRequest,
	route GatewayRouteSnapshot,
) (SynchronousResult, error) {
	if payload.Status != "completed" {
		return SynchronousResult{}, responsesStatusError(payload.Status)
	}
	var content string
	for _, output := range payload.Output {
		if output.Type != "message" {
			continue
		}
		for _, part := range output.Content {
			if strings.TrimSpace(part.Refusal) != "" || part.Type == "refusal" {
				return SynchronousResult{}, gatewayExecutionError("MODEL_REFUSED", "The model refused the Strategy request")
			}
			if part.Type == "output_text" {
				content += part.Text
			}
		}
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return SynchronousResult{}, gatewayExecutionError(
			"MODEL_RESPONSE_INVALID", "Adapter gateway returned an empty Responses output",
		)
	}
	result := SynchronousResult{
		ProviderCode:  adapterGatewayProviderCode,
		ModelVersion:  strings.TrimSpace(payload.Model),
		Text:          content,
		RouteSnapshot: &route,
	}
	if result.ModelVersion == "" {
		result.ModelVersion = route.UpstreamModel
	}
	if len(request.OutputJSONSchema) > 0 {
		if !json.Valid([]byte(content)) {
			if route.TextResponseMode != TextResponsePromptJSON {
				return SynchronousResult{}, gatewayExecutionError(
					"MODEL_OUTPUT_INVALID", "Structured Responses output is not valid JSON",
				)
			}
		} else {
			result.StructuredOutput = json.RawMessage(content)
		}
	}
	if payload.Usage.TotalTokens > 0 {
		result.Usage = &TokenUsage{
			InputTokens: payload.Usage.InputTokens, OutputTokens: payload.Usage.OutputTokens,
			TotalTokens: payload.Usage.TotalTokens,
		}
	}
	return result, nil
}

func responsesStatusError(status string) error {
	switch status {
	case "cancelled":
		return gatewayExecutionError("MODEL_CANCELLED", "Adapter gateway background response was cancelled")
	case "incomplete":
		return gatewayExecutionError("MODEL_RESPONSE_INCOMPLETE", "Adapter gateway returned an incomplete response")
	default:
		return gatewayExecutionError("MODEL_RESPONSE_FAILED", "Adapter gateway Responses request failed")
	}
}

func responsesEndpoint(baseURL string) string {
	endpoint := strings.TrimRight(baseURL, "/")
	parsed, err := url.Parse(endpoint)
	if err == nil && hasVersionedAPIPath(parsed.Path) {
		return endpoint + "/responses"
	}
	return endpoint + "/v1/responses"
}

func hasVersionedAPIPath(path string) bool {
	lastSegment := path[strings.LastIndexByte(path, '/')+1:]
	if len(lastSegment) < 2 || lastSegment[0] != 'v' {
		return false
	}
	for _, digit := range lastSegment[1:] {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func appendResponseInstruction(current any, schema json.RawMessage) string {
	instruction := "Return exactly one JSON object matching this JSON Schema. Do not use Markdown fences or add commentary.\n" + string(schema)
	if value, ok := current.(string); ok && strings.TrimSpace(value) != "" {
		return value + "\n\n" + instruction
	}
	return instruction
}
