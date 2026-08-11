package creative

import (
	"fmt"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

const (
	ManualShortDramaPrerollV2RouteID   = "route_manual_short_drama_preroll_v2"
	ShortDramaPrerollV2ContractVersion = "creative-short-drama-preroll-workspace/v2"
)

type ShortDramaV2Stage string

const (
	ShortDramaV2StageSourceReady      ShortDramaV2Stage = "source_ready"
	ShortDramaV2StageAnalyzing        ShortDramaV2Stage = "analyzing"
	ShortDramaV2StageAnalysisReady    ShortDramaV2Stage = "analysis_ready"
	ShortDramaV2StageDirectionsReady  ShortDramaV2Stage = "directions_ready"
	ShortDramaV2StagePromptsReady     ShortDramaV2Stage = "prompts_ready"
	ShortDramaV2StageFramesGenerating ShortDramaV2Stage = "first_frames_generating"
	ShortDramaV2StageFramesReady      ShortDramaV2Stage = "first_frames_ready"
	ShortDramaV2StageFrameSelected    ShortDramaV2Stage = "first_frame_selected"
	ShortDramaV2StageVideoGenerating  ShortDramaV2Stage = "video_generating"
	ShortDramaV2StageNormalizing      ShortDramaV2Stage = "normalizing_output"
	ShortDramaV2StageCompleted        ShortDramaV2Stage = "completed"
)

type ShortDramaV2ResourceStatus string

const (
	ShortDramaV2ResourceIdle      ShortDramaV2ResourceStatus = "idle"
	ShortDramaV2ResourceQueued    ShortDramaV2ResourceStatus = "queued"
	ShortDramaV2ResourceRunning   ShortDramaV2ResourceStatus = "running"
	ShortDramaV2ResourceReady     ShortDramaV2ResourceStatus = "ready"
	ShortDramaV2ResourcePartial   ShortDramaV2ResourceStatus = "partial"
	ShortDramaV2ResourceFailed    ShortDramaV2ResourceStatus = "failed"
	ShortDramaV2ResourceCancelled ShortDramaV2ResourceStatus = "cancelled"
)

type ManualShortDramaPrerollV2Input struct {
	SourceVideo       contract.AssetVersionRef `json:"source_video"`
	SourceVideoRights RightsStatus             `json:"source_video_rights"`
}

func (i ManualShortDramaPrerollV2Input) Validate() error {
	if err := i.SourceVideo.Validate(); err != nil {
		return fmt.Errorf("source_video: %w", err)
	}
	if i.SourceVideoRights != RightsConfirmed {
		return fmt.Errorf("source_video_rights must be confirmed")
	}
	return nil
}

type ShortDramaV2AsyncResource struct {
	Status       ShortDramaV2ResourceStatus `json:"status"`
	AttemptID    string                     `json:"attempt_id,omitempty"`
	ErrorCode    string                     `json:"error_code,omitempty"`
	ErrorMessage string                     `json:"error_message,omitempty"`
}

type ShortDramaV2Character struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Relationship string `json:"relationship,omitempty"`
}

type ShortDramaV2Evidence struct {
	ID           string `json:"id"`
	TimestampMS  int64  `json:"timestamp_ms"`
	Transcript   string `json:"transcript,omitempty"`
	FrameAssetID string `json:"frame_asset_id,omitempty"`
}

type ShortDramaV2AnalysisContent struct {
	Title          string                  `json:"title"`
	Episode        string                  `json:"episode,omitempty"`
	Synopsis       string                  `json:"synopsis"`
	OpeningBeat    string                  `json:"opening_beat"`
	CoreConflict   string                  `json:"core_conflict"`
	UnresolvedHook string                  `json:"unresolved_hook"`
	Tone           string                  `json:"tone"`
	Characters     []ShortDramaV2Character `json:"characters"`
	VisualKeywords []string                `json:"visual_keywords"`
	Evidence       []ShortDramaV2Evidence  `json:"evidence"`
}

type ShortDramaV2Analysis struct {
	ShortDramaV2AsyncResource
	Revision      int64                       `json:"revision"`
	InputHash     string                      `json:"input_hash,omitempty"`
	PromptVersion string                      `json:"prompt_version,omitempty"`
	Content       ShortDramaV2AnalysisContent `json:"content"`
}

type ShortDramaV2HookDirection struct {
	ID                   string   `json:"id"`
	Category             string   `json:"category"`
	Title                string   `json:"title"`
	HookCopy             string   `json:"hook_copy"`
	Description          string   `json:"description"`
	Rationale            string   `json:"rationale"`
	VisualIntent         string   `json:"visual_intent"`
	GroundingEvidenceIDs []string `json:"grounding_evidence_ids"`
}

type ShortDramaV2DirectionBatch struct {
	ShortDramaV2AsyncResource
	ID                  string                      `json:"id,omitempty"`
	Revision            int64                       `json:"revision"`
	AnalysisRevision    int64                       `json:"analysis_revision"`
	PlannerVersion      string                      `json:"planner_version,omitempty"`
	Items               []ShortDramaV2HookDirection `json:"items"`
	SelectedDirectionID string                      `json:"selected_direction_id,omitempty"`
}

type ShortDramaV2PromptDraft struct {
	Revision           int64  `json:"revision"`
	DirectionID        string `json:"direction_id"`
	DurationSeconds    int    `json:"duration_seconds"`
	ImagePrompt        string `json:"image_prompt"`
	VideoDescription   string `json:"video_description"`
	VideoPrompt        string `json:"video_prompt"`
	BaseVideoPrompt    string `json:"base_video_prompt,omitempty"`
	SelectedVariantKey string `json:"selected_variant_key,omitempty"`
	CompilerVersion    string `json:"compiler_version"`
	ContentHash        string `json:"content_hash"`
}

type ShortDramaV2FirstFrameCandidate struct {
	ID                string                     `json:"id"`
	VariantIndex      int                        `json:"variant_index"`
	ProviderJobID     string                     `json:"provider_job_id,omitempty"`
	Status            ShortDramaV2ResourceStatus `json:"status"`
	Asset             *contract.ProjectAssetRef  `json:"asset,omitempty"`
	ModelCanvasAsset  *contract.ProjectAssetRef  `json:"model_canvas_asset,omitempty"`
	OutputCanvasAsset *contract.ProjectAssetRef  `json:"output_canvas_asset,omitempty"`
	VariantKey        string                     `json:"variant_key,omitempty"`
	VisualMechanism   string                     `json:"visual_mechanism,omitempty"`
	StyleProfile      string                     `json:"style_profile,omitempty"`
	ErrorCode         string                     `json:"error_code,omitempty"`
	ErrorMessage      string                     `json:"error_message,omitempty"`
}

type ShortDramaV2FirstFrameBatch struct {
	ShortDramaV2AsyncResource
	ID                  string                            `json:"id,omitempty"`
	Revision            int64                             `json:"revision"`
	PromptRevision      int64                             `json:"prompt_revision"`
	Candidates          []ShortDramaV2FirstFrameCandidate `json:"candidates"`
	SelectedAsset       *contract.ProjectAssetRef         `json:"selected_asset,omitempty"`
	SelectedOutputAsset *contract.ProjectAssetRef         `json:"selected_output_asset,omitempty"`
}

type ShortDramaV2SourceOpeningFrame struct {
	Status            ShortDramaV2ResourceStatus `json:"status"`
	Asset             *contract.ProjectAssetRef  `json:"asset,omitempty"`
	SourceVideo       contract.ProjectAssetRef   `json:"source_video"`
	TimestampMS       int64                      `json:"timestamp_ms"`
	DerivationID      string                     `json:"derivation_id,omitempty"`
	ExtractionVersion string                     `json:"extraction_version,omitempty"`
}

// ShortDramaV2TrustedMaterialBinding records user-confirmed Ark trusted-library
// assets corresponding to the selected local first and last frame. Enrollment
// and portrait consent happen in Ark; Cookies only stores the returned IDs.
type ShortDramaV2TrustedMaterialBinding struct {
	ProviderCode      string `json:"provider_code"`
	FirstFrameAssetID string `json:"first_frame_asset_id"`
	LastFrameAssetID  string `json:"last_frame_asset_id"`
}

type ShortDramaV2GenerationSpec struct {
	ContractVersion  string                              `json:"contract_version"`
	DraftRevision    int64                               `json:"draft_revision"`
	PromptRevision   int64                               `json:"prompt_revision"`
	DurationSeconds  int                                 `json:"duration_seconds"`
	AspectRatio      string                              `json:"aspect_ratio"`
	Resolution       string                              `json:"resolution"`
	AudioPolicy      string                              `json:"audio_policy"`
	InputMode        string                              `json:"input_mode"`
	FirstFrameAsset  contract.ProjectAssetRef            `json:"first_frame_asset"`
	LastFrameAsset   *contract.ProjectAssetRef           `json:"last_frame_asset,omitempty"`
	TrustedMaterials *ShortDramaV2TrustedMaterialBinding `json:"trusted_materials,omitempty"`
	SourceCanvas     *ShortDramaSourceCanvas             `json:"source_canvas,omitempty"`
	ModelCanvas      *ShortDramaModelCanvas              `json:"model_canvas,omitempty"`
	OutputCanvas     *ShortDramaOutputCanvas             `json:"output_canvas,omitempty"`
	CompiledPrompt   string                              `json:"compiled_prompt,omitempty"`
	PromptHash       string                              `json:"prompt_hash"`
	SpecHash         string                              `json:"spec_hash"`
}

type ShortDramaPrerollV2Workspace struct {
	ContractVersion      string                              `json:"contract_version"`
	TaskID               string                              `json:"task_id"`
	Revision             int64                               `json:"revision"`
	ActiveStage          ShortDramaV2Stage                   `json:"active_stage"`
	SourceVideo          contract.ProjectAssetRef            `json:"source_video"`
	SourceMetadata       CreativeAssetSnapshot               `json:"source_metadata"`
	SourceCanvas         *ShortDramaSourceCanvas             `json:"source_canvas,omitempty"`
	ModelCanvas          *ShortDramaModelCanvas              `json:"model_canvas,omitempty"`
	OutputCanvas         *ShortDramaOutputCanvas             `json:"output_canvas,omitempty"`
	Analysis             ShortDramaV2Analysis                `json:"analysis"`
	DirectionBatch       *ShortDramaV2DirectionBatch         `json:"direction_batch,omitempty"`
	PromptDraft          *ShortDramaV2PromptDraft            `json:"prompt_draft,omitempty"`
	FirstFrameBatch      *ShortDramaV2FirstFrameBatch        `json:"first_frame_batch,omitempty"`
	SourceOpeningFrame   *ShortDramaV2SourceOpeningFrame     `json:"source_opening_frame,omitempty"`
	TrustedMaterials     *ShortDramaV2TrustedMaterialBinding `json:"trusted_materials,omitempty"`
	GenerationSpec       *ShortDramaV2GenerationSpec         `json:"generation_spec,omitempty"`
	LatestVideoAttemptID string                              `json:"latest_video_attempt_id,omitempty"`
	VideoError           *contract.JobError                  `json:"video_error,omitempty"`
	RawOutputAsset       *contract.ProjectAssetRef           `json:"raw_output_asset,omitempty"`
	OutputNormalization  *ShortDramaV2AsyncResource          `json:"output_normalization,omitempty"`
	OutputAsset          *contract.ProjectAssetRef           `json:"output_asset,omitempty"`
	CreatedAt            time.Time                           `json:"created_at"`
	UpdatedAt            time.Time                           `json:"updated_at"`
}

func (w ShortDramaPrerollV2Workspace) Validate() error {
	if (w.ContractVersion != ShortDramaPrerollV2ContractVersion && w.ContractVersion != ShortDramaPrerollV3ContractVersion) || strings.TrimSpace(w.TaskID) == "" ||
		w.Revision < 1 || w.ActiveStage == "" || w.SourceVideo.Validate() != nil ||
		w.CreatedAt.IsZero() || w.UpdatedAt.IsZero() {
		return fmt.Errorf("creative short drama preroll V2 workspace is incomplete")
	}
	if w.Analysis.Status == "" {
		return fmt.Errorf("creative short drama preroll V2 analysis state is required")
	}
	return nil
}
