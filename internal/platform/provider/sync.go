package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/shikanon/cookies/internal/platform/contract"
)

const ScopeTextGenerate contract.Scope = "provider.text.generate"
const ScopeVisionUnderstand contract.Scope = "provider.vision.understand"

type TextRole string

const (
	TextRoleSystem    TextRole = "system"
	TextRoleUser      TextRole = "user"
	TextRoleAssistant TextRole = "assistant"
)

type TextMessage struct {
	Role    TextRole    `json:"role"`
	Content string      `json:"content"`
	Images  []TextImage `json:"-"`
}

type TextImage struct {
	MIMEType string
	Data     []byte
}

func (m TextMessage) chatContent() any {
	if len(m.Images) == 0 {
		return m.Content
	}
	parts := []map[string]any{{"type": "text", "text": m.Content}}
	for _, img := range m.Images {
		parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": img.dataURL()}})
	}
	return parts
}
func (img TextImage) dataURL() string {
	return "data:" + img.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(img.Data)
}
func (m TextMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"role": m.Role, "content": m.chatContent()})
}

func (m TextMessage) Validate() error {
	if m.Role != TextRoleSystem && m.Role != TextRoleUser && m.Role != TextRoleAssistant {
		return fmt.Errorf("text message role is invalid")
	}
	if len(m.Images) > 24 || (len(m.Images) > 0 && m.Role != TextRoleUser) {
		return fmt.Errorf("invalid text image inputs")
	}
	for _, img := range m.Images {
		if img.MIMEType != "image/jpeg" && img.MIMEType != "image/png" || len(img.Data) == 0 || len(img.Data) > 2<<20 {
			return fmt.Errorf("invalid text image")
		}
	}
	if strings.TrimSpace(m.Content) == "" {
		return fmt.Errorf("text message content is required")
	}
	return nil
}

// TextGenerateRequest is a synchronous capability request. It deliberately
// has no vendor model ID or credential field.
type TextGenerateRequest struct {
	Actor            contract.ActorContext
	Project          contract.ProjectContext
	ModelAlias       string
	InvocationKey    contract.IdempotencyKey
	Messages         []TextMessage
	OutputJSONSchema json.RawMessage
}

func (r TextGenerateRequest) Validate() error {
	if err := validateSynchronousScope(r.Actor, r.Project, ScopeTextGenerate); err != nil {
		return err
	}
	if strings.TrimSpace(r.ModelAlias) == "" || len(r.Messages) == 0 {
		return fmt.Errorf("model alias and one or more text messages are required")
	}
	if r.InvocationKey != "" {
		if err := r.InvocationKey.Validate(); err != nil {
			return fmt.Errorf("invalid text invocation key: %w", err)
		}
	}
	for index, message := range r.Messages {
		if err := message.Validate(); err != nil {
			return fmt.Errorf("invalid text message at index %d: %w", index, err)
		}
	}
	return validateOptionalJSONObject(r.OutputJSONSchema)
}

type TextAdapterRequest struct {
	OrganizationID   contract.OrganizationID
	ModelAlias       string
	InvocationKey    contract.IdempotencyKey
	Messages         []TextMessage
	OutputJSONSchema json.RawMessage
}

type TextProviderAdapter interface {
	GenerateText(context.Context, TextAdapterRequest) (SynchronousResult, error)
}

// VisionUnderstandingInput references only Assets-owned immutable versions.
// A real Adapter must obtain media bytes through a separately authorized
// Assets reader; this shape is intentionally not a storage URL contract.
type VisionUnderstandingInput struct {
	Instruction      string                     `json:"instruction"`
	SourceAssets     []contract.ProjectAssetRef `json:"source_assets"`
	OutputJSONSchema json.RawMessage            `json:"output_json_schema,omitempty"`
}

type VisionUnderstandRequest struct {
	Actor      contract.ActorContext
	Project    contract.ProjectContext
	ModelAlias string
	Input      VisionUnderstandingInput
}

func (r VisionUnderstandRequest) Validate() error {
	if err := validateSynchronousScope(r.Actor, r.Project, ScopeVisionUnderstand); err != nil {
		return err
	}
	if strings.TrimSpace(r.ModelAlias) == "" || strings.TrimSpace(r.Input.Instruction) == "" || len(r.Input.SourceAssets) == 0 {
		return fmt.Errorf("model alias, instruction, and one or more source assets are required")
	}
	seen := make(map[string]struct{}, len(r.Input.SourceAssets))
	for index, ref := range r.Input.SourceAssets {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("invalid vision source asset at index %d: %w", index, err)
		}
		if ref.ProjectID != r.Project.ProjectID {
			return fmt.Errorf("vision source asset at index %d belongs to another project", index)
		}
		key := string(ref.AssetVersion.AssetID) + ":" + fmt.Sprint(ref.AssetVersion.Version)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("vision source asset at index %d is duplicated", index)
		}
		seen[key] = struct{}{}
	}
	return validateOptionalJSONObject(r.Input.OutputJSONSchema)
}

type VisionAdapterRequest struct {
	OrganizationID contract.OrganizationID
	ModelAlias     string
	Input          VisionUnderstandingInput
	Sources        []VisionSource
}

type VisionProviderAdapter interface {
	UnderstandVision(context.Context, VisionAdapterRequest) (SynchronousResult, error)
}

// VisionSourceResolver is implemented at the Assets composition boundary.
// It authorizes every ProjectAssetRef and returns a short-lived content stream
// to Provider without revealing storage URLs, bucket keys, or credentials.
type VisionSourceResolver interface {
	ResolveVisionSources(context.Context, contract.ActorContext, contract.ProjectContext, []contract.ProjectAssetRef) ([]VisionSource, error)
}

type VisionSource struct {
	Reference contract.ProjectAssetRef
	MIMEType  string
	Content   io.ReadCloser
}

// SynchronousResult contains only normalized, provider-agnostic facts. No
// raw vendor response, URL, token, or request body becomes part of it.
type SynchronousResult struct {
	ProviderCode     string
	ModelVersion     string
	Text             string
	StructuredOutput json.RawMessage
	Usage            *TokenUsage
	RouteSnapshot    *GatewayRouteSnapshot
}

type TokenUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

func (u TokenUsage) Validate() error {
	if u.InputTokens < 0 || u.OutputTokens < 0 || u.TotalTokens < 0 ||
		u.TotalTokens < u.InputTokens+u.OutputTokens {
		return fmt.Errorf("provider token usage is invalid")
	}
	return nil
}

func (r SynchronousResult) Validate() error {
	if strings.TrimSpace(r.ProviderCode) == "" || strings.TrimSpace(r.ModelVersion) == "" {
		return fmt.Errorf("provider code and model version are required")
	}
	if strings.TrimSpace(r.Text) == "" && len(r.StructuredOutput) == 0 {
		return fmt.Errorf("text or structured output is required")
	}
	if len(r.StructuredOutput) > 0 && !json.Valid(r.StructuredOutput) {
		return fmt.Errorf("structured output must be valid JSON")
	}
	if r.Usage != nil {
		if err := r.Usage.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type SynchronousResponse struct {
	ProviderCode     string           `json:"provider_code"`
	ModelAlias       string           `json:"model_alias"`
	ModelVersion     string           `json:"model_version"`
	Text             string           `json:"text"`
	StructuredOutput json.RawMessage  `json:"structured_output,omitempty"`
	Usage            *TokenUsage      `json:"usage,omitempty"`
	RouteRevisionID  string           `json:"route_revision_id,omitempty"`
	ResponseMode     TextResponseMode `json:"response_mode,omitempty"`
	APIMode          TextAPIMode      `json:"api_mode,omitempty"`
	Background       bool             `json:"background,omitempty"`
}

type TextRouteInspection struct {
	ModelAlias      string           `json:"model_alias"`
	UpstreamModel   string           `json:"upstream_model"`
	RouteRevisionID string           `json:"route_revision_id"`
	ResponseMode    TextResponseMode `json:"response_mode"`
	APIMode         TextAPIMode      `json:"api_mode"`
	Background      bool             `json:"background"`
	Ready           bool             `json:"ready"`
}

type TextRouteInspector interface {
	InspectTextRoute(context.Context, contract.OrganizationID, string) (TextRouteInspection, error)
}

type CapabilityRouteInspection struct {
	ModelAlias      string `json:"model_alias"`
	UpstreamModel   string `json:"upstream_model"`
	RouteRevisionID string `json:"route_revision_id"`
	Ready           bool   `json:"ready"`
}

type VisionRouteInspector interface {
	InspectVisionRoute(context.Context, contract.OrganizationID, string) (CapabilityRouteInspection, error)
}

func (s Service) InspectTextRoute(ctx context.Context, organizationID contract.OrganizationID, modelAlias string) (TextRouteInspection, error) {
	inspector, ok := s.TextAdapter.(TextRouteInspector)
	if !ok {
		return TextRouteInspection{}, fmt.Errorf("text provider does not expose route inspection")
	}
	return inspector.InspectTextRoute(ctx, organizationID, modelAlias)
}

func (s Service) InspectVisionRoute(ctx context.Context, organizationID contract.OrganizationID, modelAlias string) (CapabilityRouteInspection, error) {
	inspector, ok := s.VisionAdapter.(VisionRouteInspector)
	if !ok {
		return CapabilityRouteInspection{}, fmt.Errorf("vision provider does not expose route inspection")
	}
	return inspector.InspectVisionRoute(ctx, organizationID, modelAlias)
}

func (s Service) GenerateText(ctx context.Context, request TextGenerateRequest) (SynchronousResponse, error) {
	if s.TextAdapter == nil {
		return SynchronousResponse{}, fmt.Errorf("text provider adapter is required")
	}
	if err := request.Validate(); err != nil {
		return SynchronousResponse{}, err
	}
	result, err := s.TextAdapter.GenerateText(ctx, TextAdapterRequest{OrganizationID: request.Actor.OrganizationID, ModelAlias: request.ModelAlias, InvocationKey: request.InvocationKey, Messages: request.Messages, OutputJSONSchema: request.OutputJSONSchema})
	if err != nil {
		return SynchronousResponse{}, err
	}
	if err := result.Validate(); err != nil {
		return SynchronousResponse{}, fmt.Errorf("text provider response: %w", err)
	}
	response := SynchronousResponse{ProviderCode: result.ProviderCode, ModelAlias: request.ModelAlias, ModelVersion: result.ModelVersion, Text: result.Text, StructuredOutput: result.StructuredOutput, Usage: result.Usage}
	if result.RouteSnapshot != nil {
		response.RouteRevisionID = result.RouteSnapshot.RouteRevisionID
		response.ResponseMode = result.RouteSnapshot.TextResponseMode
		response.APIMode = result.RouteSnapshot.TextAPIMode
		response.Background = result.RouteSnapshot.Background
	}
	return response, nil
}

func (s Service) UnderstandVision(ctx context.Context, request VisionUnderstandRequest) (SynchronousResponse, error) {
	if s.VisionAdapter == nil {
		return SynchronousResponse{}, fmt.Errorf("vision provider adapter is required")
	}
	if s.VisionSources == nil {
		return SynchronousResponse{}, fmt.Errorf("vision source resolver is required")
	}
	if err := request.Validate(); err != nil {
		return SynchronousResponse{}, err
	}
	sources, err := s.VisionSources.ResolveVisionSources(ctx, request.Actor, request.Project, request.Input.SourceAssets)
	if err != nil {
		return SynchronousResponse{}, err
	}
	if err := validateVisionSources(request.Input.SourceAssets, sources); err != nil {
		return SynchronousResponse{}, err
	}
	for _, source := range sources {
		defer source.Content.Close()
	}
	result, err := s.VisionAdapter.UnderstandVision(ctx, VisionAdapterRequest{OrganizationID: request.Actor.OrganizationID, ModelAlias: request.ModelAlias, Input: request.Input, Sources: sources})
	if err != nil {
		return SynchronousResponse{}, err
	}
	if err := result.Validate(); err != nil {
		return SynchronousResponse{}, fmt.Errorf("vision provider response: %w", err)
	}
	response := SynchronousResponse{ProviderCode: result.ProviderCode, ModelAlias: request.ModelAlias, ModelVersion: result.ModelVersion, Text: result.Text, StructuredOutput: result.StructuredOutput, Usage: result.Usage}
	if result.RouteSnapshot != nil {
		response.RouteRevisionID = result.RouteSnapshot.RouteRevisionID
		response.ResponseMode = result.RouteSnapshot.TextResponseMode
		response.APIMode = result.RouteSnapshot.TextAPIMode
		response.Background = result.RouteSnapshot.Background
	}
	return response, nil
}

func validateVisionSources(requested []contract.ProjectAssetRef, sources []VisionSource) error {
	if len(requested) != len(sources) {
		return fmt.Errorf("vision source resolver returned an unexpected source count")
	}
	for index, source := range sources {
		if source.Reference != requested[index] || strings.TrimSpace(source.MIMEType) == "" || source.Content == nil {
			return fmt.Errorf("vision source resolver returned an invalid source at index %d", index)
		}
	}
	return nil
}

func validateSynchronousScope(actor contract.ActorContext, project contract.ProjectContext, requiredScope contract.Scope) error {
	if err := actor.Validate(); err != nil {
		return fmt.Errorf("invalid actor: %w", err)
	}
	if !actor.HasScope(requiredScope) {
		return fmt.Errorf("%s scope is required", requiredScope)
	}
	if err := project.ValidateBrandBound(); err != nil {
		return fmt.Errorf("invalid project for synchronous model invocation: %w", err)
	}
	if project.OrganizationID != actor.OrganizationID {
		return fmt.Errorf("project organization does not match actor organization")
	}
	return nil
}

func validateOptionalJSONObject(value json.RawMessage) error {
	if len(value) == 0 {
		return nil
	}
	if !json.Valid(value) || strings.TrimSpace(string(value))[0] != '{' {
		return fmt.Errorf("output JSON schema must be a JSON object")
	}
	return nil
}
