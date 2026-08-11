package provider

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

var ErrGatewayRouteNotFound = errors.New("adapter gateway route not found")

type TextResponseMode string

const (
	TextResponseJSONSchema TextResponseMode = "json_schema"
	TextResponseJSONObject TextResponseMode = "json_object"
	TextResponsePromptJSON TextResponseMode = "prompt_json"
)

type TextAPIMode string

const (
	TextAPIChatCompletions TextAPIMode = "chat_completions"
	TextAPIResponses       TextAPIMode = "responses"
)

// TextOutputTokenParameter records the upstream field used to cap output
// tokens. Most OpenAI-compatible endpoints use max_tokens, while some
// providers, including MiniMax, require max_completion_tokens.
type TextOutputTokenParameter string

const (
	TextOutputTokenParameterMaxTokens           TextOutputTokenParameter = "max_tokens"
	TextOutputTokenParameterMaxCompletionTokens TextOutputTokenParameter = "max_completion_tokens"
	TextOutputTokenParameterMaxOutputTokens     TextOutputTokenParameter = "max_output_tokens"
)

// GatewayRouteSnapshot is copied onto an invocation when it is created. Later route
// edits therefore cannot silently change the endpoint, model, or credential
// used by an already accepted image job or text skill run.
type GatewayRouteSnapshot struct {
	RouteID              string                   `json:"route_id"`
	RouteRevisionID      string                   `json:"route_revision_id"`
	ConnectionID         string                   `json:"connection_id"`
	ConnectionRevisionID string                   `json:"connection_revision_id"`
	ConnectionType       string                   `json:"connection_type,omitempty"`
	BaseURL              string                   `json:"base_url"`
	UpstreamModel        string                   `json:"upstream_model"`
	CredentialID         string                   `json:"credential_id"`
	CredentialVersion    int64                    `json:"credential_version"`
	TimeoutSeconds       int                      `json:"timeout_seconds"`
	MaxResponseBytes     int64                    `json:"max_response_bytes"`
	TextResponseMode     TextResponseMode         `json:"text_response_mode,omitempty"`
	TextAPIMode          TextAPIMode              `json:"text_api_mode,omitempty"`
	MaxOutputTokens      int                      `json:"max_output_tokens,omitempty"`
	OutputTokenParameter TextOutputTokenParameter `json:"output_token_parameter,omitempty"`
	Temperature          float64                  `json:"temperature,omitempty"`
	TemperatureSet       bool                     `json:"-"`
	ThinkingMode         string                   `json:"thinking_mode,omitempty"`
	ReasoningSplit       bool                     `json:"reasoning_split,omitempty"`
	ReasoningEffort      string                   `json:"reasoning_effort,omitempty"`
	Background           bool                     `json:"background,omitempty"`
	PollIntervalMS       int                      `json:"poll_interval_ms,omitempty"`
	VideoInputModes      []VideoInputMode         `json:"video_input_modes,omitempty"`
	VideoAudioPolicies   []VideoAudioPolicy       `json:"video_audio_policies,omitempty"`
	VideoSubmitPath      string                   `json:"video_submit_path,omitempty"`
	VideoPollPath        string                   `json:"video_poll_path,omitempty"`
	SpeechVoiceAliases   map[string]string        `json:"speech_voice_aliases,omitempty"`
}

// ChatCompletionsEndpoint resolves the provider-specific OpenAI-compatible
// path from an immutable route snapshot. Ark base URLs already include
// /api/v3, while legacy adapter gateways expose their API below /v1.
func (s GatewayRouteSnapshot) ChatCompletionsEndpoint() string {
	base := strings.TrimRight(s.BaseURL, "/")
	if s.ConnectionType == "ark" {
		return base + "/chat/completions"
	}
	if strings.HasSuffix(base, "/v1") {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}

func (s GatewayRouteSnapshot) Validate() error {
	return s.ValidateWithPolicy(false)
}

func (s GatewayRouteSnapshot) ValidateWithPolicy(allowInsecureHTTP bool) error {
	return s.validateWithLimits(allowInsecureHTTP, 600, 100<<20)
}

func (s GatewayRouteSnapshot) ValidateVideoWithPolicy(allowInsecureHTTP bool) error {
	if err := s.validateWithLimits(allowInsecureHTTP, 1800, 200<<20); err != nil {
		return err
	}
	if err := validateVideoInputModes(s.VideoInputModes); err != nil {
		return err
	}
	return validateVideoAudioPolicies(s.VideoAudioPolicies)
}

func (s GatewayRouteSnapshot) validateWithLimits(allowInsecureHTTP bool, maxTimeoutSeconds int, maxResponseBytes int64) error {
	if strings.TrimSpace(s.RouteID) == "" || strings.TrimSpace(s.RouteRevisionID) == "" ||
		strings.TrimSpace(s.ConnectionID) == "" || strings.TrimSpace(s.ConnectionRevisionID) == "" ||
		strings.TrimSpace(s.UpstreamModel) == "" || strings.TrimSpace(s.CredentialID) == "" ||
		s.CredentialVersion < 1 {
		return fmt.Errorf("adapter gateway route snapshot is incomplete")
	}
	parsed, err := url.Parse(s.BaseURL)
	validScheme := parsed.Scheme == "https" || (allowInsecureHTTP && parsed.Scheme == "http")
	if err != nil || !validScheme || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("adapter gateway base URL must use HTTPS (or explicitly allowed local HTTP) and contain no user info")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("adapter gateway base URL cannot contain a query or fragment")
	}
	if s.TimeoutSeconds < 1 || s.TimeoutSeconds > maxTimeoutSeconds {
		return fmt.Errorf("adapter gateway timeout must be between 1 and %d seconds", maxTimeoutSeconds)
	}
	if s.MaxResponseBytes < 1 || s.MaxResponseBytes > maxResponseBytes {
		return fmt.Errorf("adapter gateway response limit must be between 1 byte and %d bytes", maxResponseBytes)
	}
	return nil
}

func (s GatewayRouteSnapshot) ValidateTextWithPolicy(allowInsecureHTTP bool) error {
	if err := s.ValidateWithPolicy(allowInsecureHTTP); err != nil {
		return err
	}
	switch s.TextResponseMode {
	case TextResponseJSONSchema, TextResponseJSONObject, TextResponsePromptJSON:
	default:
		return fmt.Errorf("adapter gateway text response mode is invalid")
	}
	switch s.TextAPIMode {
	case "", TextAPIChatCompletions, TextAPIResponses:
	default:
		return fmt.Errorf("adapter gateway text API mode is invalid")
	}
	if s.MaxOutputTokens < 0 || s.MaxOutputTokens > 100_000 {
		return fmt.Errorf("adapter gateway max output tokens are invalid")
	}
	if s.MaxOutputTokens > 0 {
		switch s.OutputTokenParameter {
		case "", TextOutputTokenParameterMaxTokens, TextOutputTokenParameterMaxCompletionTokens, TextOutputTokenParameterMaxOutputTokens:
		default:
			return fmt.Errorf("adapter gateway output token parameter is invalid")
		}
	}
	if s.Temperature < 0 || s.Temperature > 2 {
		return fmt.Errorf("adapter gateway temperature is invalid")
	}
	switch s.ThinkingMode {
	case "", "auto", "enabled", "disabled":
	default:
		return fmt.Errorf("adapter gateway thinking mode is invalid")
	}
	switch s.ReasoningEffort {
	case "", "none", "minimal", "low", "medium", "high", "xhigh":
	default:
		return fmt.Errorf("adapter gateway reasoning effort is invalid")
	}
	if (s.TextAPIMode == "" || s.TextAPIMode == TextAPIChatCompletions) &&
		(s.Background || s.ReasoningEffort != "" || s.OutputTokenParameter == TextOutputTokenParameterMaxOutputTokens) {
		return fmt.Errorf("Responses-only text constraints require responses API mode")
	}
	if s.TextAPIMode == TextAPIResponses &&
		(s.ThinkingMode != "" || s.ReasoningSplit || s.OutputTokenParameter == TextOutputTokenParameterMaxCompletionTokens) {
		return fmt.Errorf("chat-completions-only text constraints cannot be used with responses API mode")
	}
	if s.PollIntervalMS < 0 || s.PollIntervalMS > 10_000 || (s.PollIntervalMS > 0 && s.PollIntervalMS < 100) {
		return fmt.Errorf("adapter gateway response polling interval is invalid")
	}
	return nil
}

type ImageRouteResolver interface {
	ResolveImageRoute(context.Context, contract.OrganizationID, string) (ImageRouteSnapshot, error)
}

type TextRouteResolver interface {
	ResolveTextRoute(context.Context, contract.OrganizationID, string) (GatewayRouteSnapshot, error)
}

type VisionRouteResolver interface {
	ResolveVisionRoute(context.Context, contract.OrganizationID, string) (GatewayRouteSnapshot, error)
}

type ResearchRouteResolver interface {
	ResolveResearchRoute(context.Context, contract.OrganizationID, string) (GatewayRouteSnapshot, error)
}

type VideoRouteResolver interface {
	ResolveVideoRoute(context.Context, contract.OrganizationID, string) (VideoRouteSnapshot, error)
}

type SpeechRouteResolver interface {
	ResolveSpeechRoute(context.Context, contract.OrganizationID, string) (GatewayRouteSnapshot, error)
}

// ImageRouteSnapshot is retained as a source-compatible alias for the
// existing durable ProviderJob JSON contract.
type ImageRouteSnapshot = GatewayRouteSnapshot

type GatewayCredentialResolver interface {
	ResolveGatewayCredential(context.Context, string, int64) (string, error)
}

// MySQLGatewayConfigStore resolves only enabled, immutable revisions. The
// active credential version is captured in the returned job snapshot.
type MySQLGatewayConfigStore struct {
	DB                  *sql.DB
	Cipher              CredentialCipher
	AllowInsecureHTTP   bool
	VideoConnectionType string
}

type CapabilityStatus struct {
	Capability           string    `json:"capability"`
	ModelAlias           string    `json:"model_alias"`
	UpstreamModel        string    `json:"upstream_model"`
	Available            bool      `json:"available"`
	CredentialConfigured bool      `json:"credential_configured"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// ListCapabilities is the read-only Provider configuration seam used by the
// Kanon settings surface. It reports only active route metadata and whether an
// encrypted credential exists; plaintext credentials never cross this seam.
func (s MySQLGatewayConfigStore) ListCapabilities(ctx context.Context, organizationID contract.OrganizationID) ([]CapabilityStatus, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("provider database is required")
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT r.capability, r.model_alias, rr.upstream_model,
		EXISTS(
			SELECT 1 FROM provider_credentials credential
			WHERE credential.connection_id = rr.connection_id
			  AND credential.status = 'active'
			  AND credential.active_from <= UTC_TIMESTAMP(6)
			  AND (credential.active_until IS NULL OR credential.active_until > UTC_TIMESTAMP(6))
		) AS credential_configured,
		r.updated_at
		FROM provider_model_routes r
		JOIN provider_model_route_revisions rr ON rr.id = r.current_revision_id
		JOIN provider_connections connection ON connection.id = rr.connection_id
		JOIN provider_connection_revisions connection_revision ON connection_revision.id = connection.current_revision_id
		WHERE r.status = 'enabled' AND connection.status = 'enabled'
		  AND (r.organization_id IS NULL OR r.organization_id = ?)
		ORDER BY r.capability, r.model_alias`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []CapabilityStatus{}
	for rows.Next() {
		var item CapabilityStatus
		if err := rows.Scan(&item.Capability, &item.ModelAlias, &item.UpstreamModel, &item.CredentialConfigured, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Available = item.CredentialConfigured
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s MySQLGatewayConfigStore) ResolveImageRoute(ctx context.Context, organizationID contract.OrganizationID, modelAlias string) (ImageRouteSnapshot, error) {
	return s.resolveRoute(ctx, organizationID, "image.generate", modelAlias, "adapter_gateway")
}

func (s MySQLGatewayConfigStore) ResolveTextRoute(ctx context.Context, organizationID contract.OrganizationID, modelAlias string) (GatewayRouteSnapshot, error) {
	snapshot, err := s.resolveRoute(ctx, organizationID, "text.generate", modelAlias, "adapter_gateway")
	if err == nil || !errors.Is(err, ErrGatewayRouteNotFound) {
		return snapshot, err
	}
	// Ark exposes an OpenAI-compatible chat-completions surface. Allow a text
	// route to reuse an encrypted Ark project credential when the legacy
	// adapter-gateway connection is unavailable or has been retired.
	return s.resolveRoute(ctx, organizationID, "text.generate", modelAlias, "ark")
}

func (s MySQLGatewayConfigStore) ResolveVisionRoute(ctx context.Context, organizationID contract.OrganizationID, modelAlias string) (GatewayRouteSnapshot, error) {
	return s.resolveRoute(ctx, organizationID, "vision.understand", modelAlias, "adapter_gateway")
}

func (s MySQLGatewayConfigStore) ResolveResearchRoute(ctx context.Context, organizationID contract.OrganizationID, modelAlias string) (GatewayRouteSnapshot, error) {
	return s.resolveRoute(ctx, organizationID, "research.web", modelAlias, "ark")
}

func (s MySQLGatewayConfigStore) ResolveVideoRoute(ctx context.Context, organizationID contract.OrganizationID, modelAlias string) (VideoRouteSnapshot, error) {
	connectionType := strings.TrimSpace(s.VideoConnectionType)
	if connectionType == "" {
		connectionType = "ark"
	}
	return s.resolveRoute(ctx, organizationID, "video.generate", modelAlias, connectionType)
}

func (s MySQLGatewayConfigStore) ResolveSpeechRoute(ctx context.Context, organizationID contract.OrganizationID, modelAlias string) (GatewayRouteSnapshot, error) {
	return s.resolveRoute(ctx, organizationID, "speech.synthesize", modelAlias, "minimax_speech")
}

func (s MySQLGatewayConfigStore) resolveRoute(ctx context.Context, organizationID contract.OrganizationID, capability, modelAlias, connectionType string) (ImageRouteSnapshot, error) {
	if s.DB == nil {
		return ImageRouteSnapshot{}, fmt.Errorf("MySQL database is required")
	}
	var snapshot ImageRouteSnapshot
	var constraintsJSON []byte
	err := s.DB.QueryRowContext(ctx, `SELECT
			r.id, rr.id, c.id, cr.id, c.connection_type, cr.base_url, rr.upstream_model,
			pc.id, pc.credential_version, cr.timeout_seconds, cr.max_response_bytes,
			COALESCE(rr.constraints_json, JSON_OBJECT())
		FROM provider_model_routes r
		JOIN provider_model_route_revisions rr ON rr.id = r.current_revision_id AND rr.route_id = r.id
		JOIN provider_connections c ON c.id = rr.connection_id AND c.status = 'enabled' AND c.connection_type = ?
		JOIN provider_connection_revisions cr ON cr.id = rr.connection_revision_id AND cr.connection_id = c.id
		JOIN provider_credentials pc ON pc.connection_id = c.id AND pc.status = 'active'
		WHERE r.capability = ? AND r.model_alias = ? AND r.status = 'enabled'
			AND (r.organization_id = ? OR r.organization_id IS NULL)
			AND pc.active_from <= UTC_TIMESTAMP(6)
			AND (pc.active_until IS NULL OR pc.active_until > UTC_TIMESTAMP(6))
		ORDER BY (r.organization_id IS NOT NULL) DESC, pc.credential_version DESC
		LIMIT 1`,
		connectionType, capability, modelAlias, organizationID,
	).Scan(
		&snapshot.RouteID, &snapshot.RouteRevisionID, &snapshot.ConnectionID, &snapshot.ConnectionRevisionID, &snapshot.ConnectionType,
		&snapshot.BaseURL, &snapshot.UpstreamModel, &snapshot.CredentialID, &snapshot.CredentialVersion,
		&snapshot.TimeoutSeconds, &snapshot.MaxResponseBytes, &constraintsJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ImageRouteSnapshot{}, fmt.Errorf("%w: no enabled %s %s route for model alias %q", ErrGatewayRouteNotFound, connectionType, capability, modelAlias)
	}
	if err != nil {
		return ImageRouteSnapshot{}, err
	}
	if capability == "text.generate" || capability == "vision.understand" {
		if err := applyTextRouteConstraints(&snapshot, constraintsJSON); err != nil {
			return ImageRouteSnapshot{}, fmt.Errorf("invalid adapter gateway route %q constraints: %w", modelAlias, err)
		}
		normalizeTextRouteTransportLimits(&snapshot)
	} else if capability == "video.generate" {
		if err := applyVideoRouteConstraints(&snapshot, constraintsJSON); err != nil {
			return ImageRouteSnapshot{}, fmt.Errorf("invalid adapter gateway route %q constraints: %w", modelAlias, err)
		}
	} else if capability == "speech.synthesize" {
		if err := applySpeechRouteConstraints(&snapshot, constraintsJSON); err != nil {
			return ImageRouteSnapshot{}, fmt.Errorf("invalid MiniMax speech route %q constraints: %w", modelAlias, err)
		}
	}
	validate := snapshot.ValidateWithPolicy
	if capability == "text.generate" || capability == "vision.understand" {
		validate = snapshot.ValidateTextWithPolicy
	} else if capability == "video.generate" {
		validate = snapshot.ValidateVideoWithPolicy
	}
	if err := validate(s.AllowInsecureHTTP); err != nil {
		return ImageRouteSnapshot{}, fmt.Errorf("invalid adapter gateway route %q: %w", modelAlias, err)
	}
	return snapshot, nil
}

// Text and multimodal understanding calls may share an Ark connection with
// long-running video generation. Keep the immutable connection revision as
// the provider's upper bound, then narrow the invocation snapshot to the
// safety envelope of the requested capability. This prevents a valid video
// timeout/response limit from making the same credential unusable for text.
func normalizeTextRouteTransportLimits(snapshot *GatewayRouteSnapshot) {
	if snapshot.TimeoutSeconds > 600 {
		snapshot.TimeoutSeconds = 600
	}
	if snapshot.MaxResponseBytes > 100<<20 {
		snapshot.MaxResponseBytes = 100 << 20
	}
}

func applySpeechRouteConstraints(snapshot *GatewayRouteSnapshot, raw json.RawMessage) error {
	if snapshot == nil {
		return fmt.Errorf("route snapshot is required")
	}
	var constraints struct {
		VoiceAliases map[string]string `json:"voice_aliases"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &constraints); err != nil {
			return err
		}
	}
	if len(constraints.VoiceAliases) == 0 {
		return fmt.Errorf("speech route requires at least one logical voice alias")
	}
	snapshot.SpeechVoiceAliases = make(map[string]string, len(constraints.VoiceAliases))
	for alias, voiceID := range constraints.VoiceAliases {
		alias, voiceID = strings.TrimSpace(alias), strings.TrimSpace(voiceID)
		if alias == "" || voiceID == "" {
			return fmt.Errorf("speech voice alias mapping is invalid")
		}
		snapshot.SpeechVoiceAliases[alias] = voiceID
	}
	return nil
}

func applyVideoRouteConstraints(snapshot *GatewayRouteSnapshot, raw json.RawMessage) error {
	if snapshot == nil {
		return fmt.Errorf("route snapshot is required")
	}
	var constraints struct {
		InputModes    []VideoInputMode   `json:"video_input_modes"`
		AudioPolicies []VideoAudioPolicy `json:"video_audio_policies"`
		SubmitPath    string             `json:"endpoint"`
		PollPath      string             `json:"poll_endpoint"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &constraints); err != nil {
			return err
		}
	}
	if err := validateVideoInputModes(constraints.InputModes); err != nil {
		return err
	}
	if err := validateVideoAudioPolicies(constraints.AudioPolicies); err != nil {
		return err
	}
	snapshot.VideoInputModes = append([]VideoInputMode(nil), constraints.InputModes...)
	snapshot.VideoAudioPolicies = append([]VideoAudioPolicy(nil), constraints.AudioPolicies...)
	snapshot.VideoSubmitPath = strings.TrimSpace(constraints.SubmitPath)
	snapshot.VideoPollPath = strings.TrimSpace(constraints.PollPath)
	if snapshot.VideoSubmitPath != "" && !validGatewayPath(snapshot.VideoSubmitPath, false) {
		return fmt.Errorf("video endpoint path is invalid")
	}
	if snapshot.VideoPollPath != "" && !validGatewayPath(snapshot.VideoPollPath, true) {
		return fmt.Errorf("video poll endpoint path is invalid")
	}
	return nil
}

func validGatewayPath(value string, taskTemplate bool) bool {
	if !strings.HasPrefix(value, "/") || strings.Contains(value, "..") || strings.ContainsAny(value, "?#") {
		return false
	}
	if taskTemplate {
		return strings.Count(value, "{task_id}") == 1
	}
	return !strings.Contains(value, "{") && !strings.Contains(value, "}")
}

func validateVideoInputModes(values []VideoInputMode) error {
	seen := make(map[VideoInputMode]struct{}, len(values))
	for _, value := range values {
		switch value {
		case VideoInputTextOnly, VideoInputReferenceImage, VideoInputFirstLastFrame:
		default:
			return fmt.Errorf("video input mode %q is invalid", value)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("video input mode %q is duplicated", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateVideoAudioPolicies(values []VideoAudioPolicy) error {
	seen := make(map[VideoAudioPolicy]struct{}, len(values))
	for _, value := range values {
		switch value {
		case VideoAudioSilent, VideoAudioGenerated:
		default:
			return fmt.Errorf("video audio policy %q is invalid", value)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("video audio policy %q is duplicated", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func applyTextRouteConstraints(snapshot *GatewayRouteSnapshot, raw json.RawMessage) error {
	if snapshot == nil {
		return fmt.Errorf("route snapshot is required")
	}
	var constraints struct {
		ResponseMode         TextResponseMode         `json:"text_response_mode"`
		APIMode              TextAPIMode              `json:"api_mode"`
		MaxOutputTokens      int                      `json:"max_output_tokens"`
		OutputTokenParameter TextOutputTokenParameter `json:"output_token_parameter"`
		Temperature          *float64                 `json:"temperature"`
		ThinkingMode         string                   `json:"thinking_mode"`
		ReasoningSplit       bool                     `json:"reasoning_split"`
		ReasoningEffort      string                   `json:"reasoning_effort"`
		Background           bool                     `json:"background"`
		PollIntervalMS       int                      `json:"poll_interval_ms"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &constraints); err != nil {
			return err
		}
	}
	// prompt_json is the safest backward-compatible mode for existing
	// OpenAI-compatible routes whose strict schema capability was never
	// recorded. A route must opt into stronger response modes explicitly.
	if constraints.ResponseMode == "" {
		constraints.ResponseMode = TextResponsePromptJSON
	}
	if constraints.APIMode == "" {
		constraints.APIMode = TextAPIChatCompletions
	}
	snapshot.TextResponseMode = constraints.ResponseMode
	snapshot.TextAPIMode = constraints.APIMode
	snapshot.MaxOutputTokens = constraints.MaxOutputTokens
	snapshot.OutputTokenParameter = constraints.OutputTokenParameter
	if constraints.Temperature != nil {
		snapshot.Temperature = *constraints.Temperature
		snapshot.TemperatureSet = true
	}
	snapshot.ThinkingMode = constraints.ThinkingMode
	snapshot.ReasoningSplit = constraints.ReasoningSplit
	snapshot.ReasoningEffort = constraints.ReasoningEffort
	snapshot.Background = constraints.Background
	snapshot.PollIntervalMS = constraints.PollIntervalMS
	return nil
}

func (s MySQLGatewayConfigStore) ResolveGatewayCredential(ctx context.Context, credentialID string, version int64) (string, error) {
	if s.DB == nil || s.Cipher == nil {
		return "", fmt.Errorf("MySQL database and credential cipher are required")
	}
	var ciphertext, nonce []byte
	var keyVersion string
	err := s.DB.QueryRowContext(ctx, `SELECT ciphertext, nonce, key_version
		FROM provider_credentials
		WHERE id = ? AND credential_version = ? AND status IN ('active', 'retired')`,
		credentialID, version,
	).Scan(&ciphertext, &nonce, &keyVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("adapter gateway credential version is unavailable")
	}
	if err != nil {
		return "", err
	}
	plaintext, err := s.Cipher.Decrypt(ciphertext, nonce, keyVersion)
	if err != nil {
		return "", fmt.Errorf("decrypt adapter gateway credential: %w", err)
	}
	token := strings.TrimSpace(string(plaintext))
	if token == "" {
		return "", fmt.Errorf("adapter gateway credential is empty")
	}
	return token, nil
}

type CredentialCipher interface {
	Encrypt([]byte) (ciphertext, nonce []byte, keyVersion string, err error)
	Decrypt(ciphertext, nonce []byte, keyVersion string) ([]byte, error)
}

// AESGCMCredentialCipher keeps the master key outside MySQL. The database
// contains only authenticated ciphertext, a random nonce, and a key version.
type AESGCMCredentialCipher struct {
	key        []byte
	keyVersion string
}

func NewAESGCMCredentialCipher(base64Key, keyVersion string) (*AESGCMCredentialCipher, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(base64Key))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("provider master key must be base64-encoded 32 bytes")
	}
	if strings.TrimSpace(keyVersion) == "" {
		return nil, fmt.Errorf("provider master key version is required")
	}
	return &AESGCMCredentialCipher{key: key, keyVersion: strings.TrimSpace(keyVersion)}, nil
}

func (c *AESGCMCredentialCipher) Encrypt(plaintext []byte) ([]byte, []byte, string, error) {
	if c == nil || len(c.key) != 32 || len(plaintext) == 0 {
		return nil, nil, "", fmt.Errorf("credential cipher and plaintext are required")
	}
	aead, err := newGCM(c.key)
	if err != nil {
		return nil, nil, "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, "", err
	}
	return aead.Seal(nil, nonce, plaintext, []byte(c.keyVersion)), nonce, c.keyVersion, nil
}

func (c *AESGCMCredentialCipher) Decrypt(ciphertext, nonce []byte, keyVersion string) ([]byte, error) {
	if c == nil || keyVersion != c.keyVersion {
		return nil, fmt.Errorf("provider master key version %q is unavailable", keyVersion)
	}
	aead, err := newGCM(c.key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != aead.NonceSize() {
		return nil, fmt.Errorf("credential nonce has an invalid length")
	}
	return aead.Open(nil, nonce, ciphertext, []byte(keyVersion))
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func routeDeadline(snapshot GatewayRouteSnapshot, now time.Time) time.Time {
	return now.Add(time.Duration(snapshot.TimeoutSeconds) * time.Second)
}
