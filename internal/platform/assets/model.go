package assets

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

const MaxImageBytes int64 = 20 * 1024 * 1024
const MaxVideoBytes int64 = 200 * 1024 * 1024
const MaxAudioBytes int64 = 50 * 1024 * 1024
const MaxImageDimension = 16384
const MaxImagePixels int64 = 100_000_000

type AssetStatus string

const (
	AssetProcessing  AssetStatus = "processing"
	AssetReady       AssetStatus = "ready"
	AssetQuarantined AssetStatus = "quarantined"
	AssetFailed      AssetStatus = "failed"
	AssetArchived    AssetStatus = "archived"
)

type Asset struct {
	ID             contract.AssetID        `json:"id"`
	OrganizationID contract.OrganizationID `json:"organization_id"`
	Kind           contract.AssetKind      `json:"asset_kind"`
	Status         AssetStatus             `json:"status"`
	OwnerSystem    string                  `json:"owner_system"`
	LatestVersion  int64                   `json:"latest_version"`
	CreatedAt      time.Time               `json:"created_at"`
	UpdatedAt      time.Time               `json:"updated_at"`
}

func validSHA256(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

type AssetVersion struct {
	OrganizationID        contract.OrganizationID  `json:"organization_id"`
	AssetID               contract.AssetID         `json:"asset_id"`
	Version               int64                    `json:"version"`
	Status                AssetStatus              `json:"status"`
	SourceType            contract.AssetSourceType `json:"source_type"`
	MIMEType              string                   `json:"mime_type"`
	SizeBytes             int64                    `json:"size_bytes"`
	SHA256                string                   `json:"sha256"`
	WidthPixels           int                      `json:"width_pixels,omitempty"`
	HeightPixels          int                      `json:"height_pixels,omitempty"`
	Media                 MediaMetadata            `json:"media"`
	DurationMS            int64                    `json:"duration_ms,omitempty"`
	FrameRate             string                   `json:"frame_rate,omitempty"`
	VideoCodec            string                   `json:"video_codec,omitempty"`
	AudioCodec            string                   `json:"audio_codec,omitempty"`
	RenderJobID           string                   `json:"render_job_id,omitempty"`
	DerivationID          string                   `json:"derivation_id,omitempty"`
	ProviderJobID         string                   `json:"provider_job_id,omitempty"`
	ProviderOutputID      string                   `json:"provider_output_id,omitempty"`
	ProjectContextVersion int64                    `json:"project_context_version,omitempty"`
	CreatedAt             time.Time                `json:"created_at"`
	Blob                  ObjectLocation           `json:"-"`
}

func (v AssetVersion) Ref() contract.AssetVersionRef {
	return contract.AssetVersionRef{AssetID: v.AssetID, Version: v.Version}
}

type AssetRelationType string

const (
	AssetRelationGeneratedFrom AssetRelationType = "generated_from"
	AssetRelationDerivedFrom   AssetRelationType = "derived_from"
)

type AssetRelation struct {
	OrganizationID contract.OrganizationID  `json:"organization_id"`
	ProjectID      contract.ProjectID       `json:"project_id"`
	OutputAsset    contract.AssetVersionRef `json:"output_asset"`
	RelationType   AssetRelationType        `json:"relation_type"`
	Source         contract.ResourceRef     `json:"source"`
	CreatedAt      time.Time                `json:"created_at"`
}

func (r AssetRelation) Validate() error {
	if strings.TrimSpace(string(r.OrganizationID)) == "" || strings.TrimSpace(string(r.ProjectID)) == "" {
		return fmt.Errorf("asset relation scope is required")
	}
	if err := r.OutputAsset.Validate(); err != nil {
		return fmt.Errorf("output asset: %w", err)
	}
	if r.RelationType != AssetRelationGeneratedFrom && r.RelationType != AssetRelationDerivedFrom {
		return fmt.Errorf("asset relation type is invalid")
	}
	if err := r.Source.Validate(); err != nil {
		return fmt.Errorf("source: %w", err)
	}
	if r.Source.Type == "asset_version" && r.Source.Version == nil {
		return fmt.Errorf("asset_version source requires version")
	}
	return nil
}

type ProjectAsset struct {
	Ref         contract.ProjectAssetRef `json:"ref"`
	Asset       Asset                    `json:"asset"`
	Version     AssetVersion             `json:"version"`
	UseDecision *AssetUseDecision        `json:"use_decision,omitempty"`
	CreatedAt   time.Time                `json:"created_at"`
}

const AssetFeatureSchemaV1 = "asset_feature_v1"

type AssetFeatureSimilarityRisk string

const (
	AssetFeatureRiskLow    AssetFeatureSimilarityRisk = "low"
	AssetFeatureRiskMedium AssetFeatureSimilarityRisk = "medium"
	AssetFeatureRiskHigh   AssetFeatureSimilarityRisk = "high"
)

type AssetFeature struct {
	OrganizationID    contract.OrganizationID    `json:"organization_id"`
	ProjectID         contract.ProjectID         `json:"project_id"`
	AssetID           contract.AssetID           `json:"asset_id"`
	AssetVersion      int64                      `json:"asset_version"`
	SchemaVersion     string                     `json:"schema_version"`
	FeatureVersion    string                     `json:"feature_version"`
	HookStrength      float64                    `json:"hook_strength"`
	ProductVisibility float64                    `json:"product_visibility"`
	SceneTags         []string                   `json:"scene_tags"`
	ProductTags       []string                   `json:"product_tags"`
	PersonTags        []string                   `json:"person_tags"`
	ActionTags        []string                   `json:"action_tags"`
	EmotionTags       []string                   `json:"emotion_tags"`
	SellingPoints     []string                   `json:"selling_points"`
	CTAPresence       bool                       `json:"cta_presence"`
	SimilarityGroup   string                     `json:"similarity_group,omitempty"`
	SimilarityRisk    AssetFeatureSimilarityRisk `json:"similarity_risk"`
	Evidence          []string                   `json:"evidence"`
	CreatedAt         time.Time                  `json:"created_at"`
	UpdatedAt         time.Time                  `json:"updated_at"`
}

func (f AssetFeature) Ref() contract.AssetVersionRef {
	return contract.AssetVersionRef{AssetID: f.AssetID, Version: f.AssetVersion}
}

func (f AssetFeature) Validate() error {
	if f.OrganizationID == "" || f.ProjectID == "" || f.AssetID == "" || f.AssetVersion < 1 {
		return fmt.Errorf("asset feature scope is required")
	}
	if f.SchemaVersion != AssetFeatureSchemaV1 {
		return fmt.Errorf("asset feature schema_version must be %s", AssetFeatureSchemaV1)
	}
	if strings.TrimSpace(f.FeatureVersion) == "" || len(f.FeatureVersion) > 96 {
		return fmt.Errorf("asset feature feature_version is required")
	}
	if !validUnitScore(f.HookStrength) || !validUnitScore(f.ProductVisibility) {
		return fmt.Errorf("asset feature scores must be between 0 and 1")
	}
	if f.SimilarityRisk != AssetFeatureRiskLow && f.SimilarityRisk != AssetFeatureRiskMedium && f.SimilarityRisk != AssetFeatureRiskHigh {
		return fmt.Errorf("asset feature similarity_risk is invalid")
	}
	return nil
}

func validUnitScore(value float64) bool {
	return value >= 0 && value <= 1
}

type MediaProbeStatus string

const (
	MediaProbeNotRequired MediaProbeStatus = "not_required"
	MediaProbeSucceeded   MediaProbeStatus = "succeeded"
	MediaProbeFailed      MediaProbeStatus = "failed"
)

type MediaMetadata struct {
	DurationSeconds float64          `json:"duration_seconds,omitempty"`
	FPS             float64          `json:"fps,omitempty"`
	Codec           string           `json:"codec,omitempty"`
	BitrateBPS      int64            `json:"bitrate_bps,omitempty"`
	AudioCodec      string           `json:"audio_codec,omitempty"`
	AudioChannels   int              `json:"audio_channels,omitempty"`
	AudioSampleRate int              `json:"audio_sample_rate,omitempty"`
	PosterFrameRef  string           `json:"poster_frame_ref,omitempty"`
	ProbeStatus     MediaProbeStatus `json:"probe_status"`
	ProbeError      string           `json:"probe_error,omitempty"`
}

type UploadStatus string

const (
	UploadCreated    UploadStatus = "created"
	UploadUploaded   UploadStatus = "uploaded"
	UploadProcessing UploadStatus = "processing"
	UploadSucceeded  UploadStatus = "succeeded"
	UploadFailed     UploadStatus = "failed"
	UploadExpired    UploadStatus = "expired"
)

type CreateUploadRequest struct {
	Filename          string  `json:"filename"`
	DeclaredMIMEType  string  `json:"declared_mime_type"`
	DeclaredSizeBytes int64   `json:"declared_size_bytes"`
	DeclaredSHA256    *string `json:"declared_sha256"`
}

func (r CreateUploadRequest) Validate() error {
	if strings.TrimSpace(r.Filename) == "" || len(r.Filename) > 512 {
		return fmt.Errorf("filename must be between 1 and 512 characters")
	}
	_, maxBytes, supported := generatedAssetPolicy(r.DeclaredMIMEType)
	if !supported {
		return fmt.Errorf("declared_mime_type must be a supported image, video, or audio MIME type")
	}
	if r.DeclaredSizeBytes < 1 || r.DeclaredSizeBytes > maxBytes {
		return fmt.Errorf("declared_size_bytes must be between 1 and %d", maxBytes)
	}
	if r.DeclaredSHA256 != nil && !validSHA256(*r.DeclaredSHA256) {
		return fmt.Errorf("declared_sha256 must be a lowercase hexadecimal SHA-256 digest")
	}
	return nil
}

type UploadSession struct {
	ID                    string                    `json:"id"`
	OrganizationID        contract.OrganizationID   `json:"organization_id"`
	ProjectID             contract.ProjectID        `json:"project_id"`
	Principal             contract.Principal        `json:"principal"`
	Status                UploadStatus              `json:"status"`
	Filename              string                    `json:"filename"`
	DeclaredMIMEType      string                    `json:"declared_mime_type"`
	DeclaredSizeBytes     int64                     `json:"declared_size_bytes"`
	DeclaredSHA256        *string                   `json:"declared_sha256"`
	Quarantine            ObjectLocation            `json:"-"`
	IdempotencyKey        contract.IdempotencyKey   `json:"-"`
	RequestHash           string                    `json:"-"`
	ProjectContextVersion int64                     `json:"project_context_version"`
	TargetAssetID         contract.AssetID          `json:"-"`
	TargetBlobID          string                    `json:"-"`
	RequestID             string                    `json:"-"`
	TraceID               string                    `json:"-"`
	ProjectAssetRef       *contract.ProjectAssetRef `json:"project_asset_ref"`
	ErrorCode             string                    `json:"error_code,omitempty"`
	ExpiresAt             time.Time                 `json:"expires_at"`
	CreatedAt             time.Time                 `json:"created_at"`
	UpdatedAt             time.Time                 `json:"updated_at"`
}

type CreateUploadResponse struct {
	Session UploadSession  `json:"session"`
	Upload  *SignedRequest `json:"upload"`
}

type AssetCommit struct {
	BlobID                string
	OrganizationID        contract.OrganizationID
	ProjectID             contract.ProjectID
	AssetID               contract.AssetID
	Version               int64
	Kind                  contract.AssetKind
	SourceType            contract.AssetSourceType
	OwnerSystem           string
	MIMEType              string
	SizeBytes             int64
	SHA256                string
	WidthPixels           int
	HeightPixels          int
	Media                 MediaMetadata
	DurationMS            int64
	FrameRate             string
	VideoCodec            string
	AudioCodec            string
	RenderJobID           string
	DerivationID          string
	ProviderJobID         string
	ProviderOutputID      string
	ProjectContextVersion int64
	Location              ObjectLocation
	Event                 contract.EventEnvelope
	Relations             []AssetRelation
}

func allowedDeclaredImageMIME(value string) bool {
	return value == "image/jpeg" || value == "image/png"
}

func allowedDeclaredVideoMIME(value string) bool {
	return value == "video/mp4"
}

func allowedDeclaredAudioMIME(value string) bool {
	return value == "audio/wav" || value == "audio/mpeg" || value == "audio/aac"
}

func generatedAssetPolicy(mimeType string) (contract.AssetKind, int64, bool) {
	if allowedDeclaredImageMIME(mimeType) {
		return contract.AssetImage, MaxImageBytes, true
	}
	if allowedDeclaredVideoMIME(mimeType) {
		return contract.AssetVideo, MaxVideoBytes, true
	}
	if allowedDeclaredAudioMIME(mimeType) {
		return contract.AssetAudio, MaxAudioBytes, true
	}
	return "", 0, false
}
