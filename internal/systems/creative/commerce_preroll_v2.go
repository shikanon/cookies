package creative

import (
	"fmt"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

const (
	ManualCommercePrerollV2RouteID   = "route_manual_commerce_preroll_v2"
	CommercePrerollV2ContractVersion = "creative-commerce-preroll-workspace/v2"
)

type CommercePrerollV2Stage string

const (
	CommercePrerollV2StageSourceReady            CommercePrerollV2Stage = "source_ready"
	CommercePrerollV2StageAnalyzing              CommercePrerollV2Stage = "analyzing"
	CommercePrerollV2StageUnderstandingReady     CommercePrerollV2Stage = "understanding_ready"
	CommercePrerollV2StageUnderstandingConfirmed CommercePrerollV2Stage = "understanding_confirmed"
	CommercePrerollV2StageHooksReady             CommercePrerollV2Stage = "hooks_ready"
	CommercePrerollV2StageHookSelected           CommercePrerollV2Stage = "hook_selected"
	CommercePrerollV2StageFramesGenerating       CommercePrerollV2Stage = "first_frames_generating"
	CommercePrerollV2StageFrameReady             CommercePrerollV2Stage = "first_frame_ready"
	CommercePrerollV2StageVideoGenerating        CommercePrerollV2Stage = "video_generating"
	CommercePrerollV2StageVideoReady             CommercePrerollV2Stage = "video_ready"
	CommercePrerollV2StageOutputAdopted          CommercePrerollV2Stage = "output_adopted"
)

type CommercePrerollV2ResourceStatus string

const (
	CommercePrerollV2ResourceIdle      CommercePrerollV2ResourceStatus = "idle"
	CommercePrerollV2ResourceQueued    CommercePrerollV2ResourceStatus = "queued"
	CommercePrerollV2ResourceRunning   CommercePrerollV2ResourceStatus = "running"
	CommercePrerollV2ResourceReady     CommercePrerollV2ResourceStatus = "ready"
	CommercePrerollV2ResourcePartial   CommercePrerollV2ResourceStatus = "partial"
	CommercePrerollV2ResourceFailed    CommercePrerollV2ResourceStatus = "failed"
	CommercePrerollV2ResourceCancelled CommercePrerollV2ResourceStatus = "cancelled"
)

type RightsConfirmation struct {
	Status      RightsStatus `json:"status"`
	ConfirmedBy string       `json:"confirmed_by,omitempty"`
	ConfirmedAt time.Time    `json:"confirmed_at,omitempty"`
}

func (c RightsConfirmation) Validate() error {
	if c.Status != RightsConfirmed || strings.TrimSpace(c.ConfirmedBy) == "" || c.ConfirmedAt.IsZero() {
		return fmt.Errorf("source video rights must be explicitly confirmed")
	}
	return nil
}

type ManualCommercePrerollV2Input struct {
	SourceVideo       contract.AssetVersionRef `json:"source_video"`
	SourceVideoRights RightsStatus             `json:"source_video_rights"`
}

func (i ManualCommercePrerollV2Input) Validate() error {
	if err := i.SourceVideo.Validate(); err != nil {
		return fmt.Errorf("source_video: %w", err)
	}
	if i.SourceVideoRights != RightsConfirmed {
		return fmt.Errorf("source_video_rights must be confirmed")
	}
	return nil
}

type CommercePrerollV2AsyncResource struct {
	Status       CommercePrerollV2ResourceStatus `json:"status"`
	AttemptID    string                          `json:"attempt_id,omitempty"`
	Progress     int                             `json:"progress,omitempty"`
	ErrorCode    string                          `json:"error_code,omitempty"`
	ErrorMessage string                          `json:"error_message,omitempty"`
}

type CommercePrerollV2Evidence struct {
	ID          string  `json:"id"`
	TimestampMS int64   `json:"timestamp_ms"`
	Source      string  `json:"source"`
	Excerpt     string  `json:"excerpt"`
	Confidence  float64 `json:"confidence"`
}

type CommercePrerollV2ProductFacts struct {
	Name                 string   `json:"name"`
	Category             string   `json:"category"`
	Description          string   `json:"description"`
	SellingPoints        []string `json:"selling_points"`
	AppearanceGuardrails []string `json:"appearance_guardrails"`
	LogoGuardrails       []string `json:"logo_guardrails"`
}

type CommercePrerollV2SourceUnderstanding struct {
	Product                CommercePrerollV2ProductFacts            `json:"product"`
	VisualStyle            string                                   `json:"visual_style"`
	SubtitleSummary        string                                   `json:"subtitle_summary"`
	VoiceSummary           string                                   `json:"voice_summary"`
	AudioMood              string                                   `json:"audio_mood"`
	OpeningShot            string                                   `json:"opening_shot"`
	ProductFrameMS         int64                                    `json:"product_frame_ms"`
	ProductFrameCandidates []CommercePrerollV2ProductFrameCandidate `json:"product_frame_candidates,omitempty"`
	OpeningAnchorFrameMS   int64                                    `json:"opening_anchor_frame_ms"`
	Evidence               []CommercePrerollV2Evidence              `json:"evidence"`
	Risks                  []string                                 `json:"risks"`
}

type CommercePrerollV2Analysis struct {
	CommercePrerollV2AsyncResource
	Revision      int64                                `json:"revision"`
	InputHash     string                               `json:"input_hash,omitempty"`
	PromptVersion string                               `json:"prompt_version,omitempty"`
	Content       CommercePrerollV2SourceUnderstanding `json:"content"`
}

type CommercePrerollV2HookRecipe struct {
	ID                  string   `json:"id"`
	RecipeVersion       string   `json:"recipe_version"`
	Name                string   `json:"name"`
	Mechanism           string   `json:"mechanism"`
	Concept             string   `json:"concept"`
	Rationale           string   `json:"rationale"`
	SellingPoint        string   `json:"selling_point"`
	PrimaryAction       string   `json:"primary_action"`
	CameraRules         []string `json:"camera_rules"`
	ProductGuardrails   []string `json:"product_guardrails"`
	NegativeConstraints []string `json:"negative_constraints"`
	VisualSignature     string   `json:"visual_signature"`
	SuitableFor         []string `json:"suitable_for"`
	WhyForSource        []string `json:"why_for_this_source"`
	OpeningState        string   `json:"opening_state"`
	ResultState         string   `json:"result_state"`
	ContinuityPlan      string   `json:"continuity_plan"`
	RiskNotes           []string `json:"risk_notes"`
	MatchScore          float64  `json:"match_score"`
	RecommendationLevel string   `json:"recommendation_level"`
}

type CommercePrerollV2HookBatch struct {
	CommercePrerollV2AsyncResource
	ID             string                        `json:"id"`
	Revision       int64                         `json:"revision"`
	Items          []CommercePrerollV2HookRecipe `json:"items"`
	SelectedHookID string                        `json:"selected_hook_id,omitempty"`
}

type CommercePrerollV2Beat struct {
	ID                string `json:"id"`
	Label             string `json:"label"`
	StartMS           int64  `json:"start_ms"`
	EndMS             int64  `json:"end_ms"`
	Detail            string `json:"detail"`
	VisualDescription string `json:"visual_description,omitempty"`
	SubjectAction     string `json:"subject_action,omitempty"`
	Camera            string `json:"camera,omitempty"`
	SceneAndLighting  string `json:"scene_and_lighting,omitempty"`
	ProductState      string `json:"product_state,omitempty"`
	TransitionIn      string `json:"transition_in,omitempty"`
	TransitionOut     string `json:"transition_out,omitempty"`
	OnScreenText      string `json:"on_screen_text,omitempty"`
	AudioInstruction  string `json:"audio_instruction,omitempty"`
}

type CommercePrerollV2PromptDraft struct {
	Revision          int64                   `json:"revision"`
	HookID            string                  `json:"hook_id"`
	DurationSeconds   int                     `json:"duration_seconds"`
	ExtraInstruction  string                  `json:"extra_instruction,omitempty"`
	Beats             []CommercePrerollV2Beat `json:"beats"`
	PromptSummary     string                  `json:"prompt_summary"`
	CompiledPrompt    string                  `json:"compiled_prompt"`
	CreativePrompt    string                  `json:"creative_prompt"`
	LockedConstraints []string                `json:"locked_constraints"`
	EditMode          string                  `json:"edit_mode"`
	CompilerVersion   string                  `json:"compiler_version"`
	ContentHash       string                  `json:"content_hash"`
}

type CommercePrerollV2DerivedFrame struct {
	Status            CommercePrerollV2ResourceStatus `json:"status"`
	Asset             *contract.ProjectAssetRef       `json:"asset,omitempty"`
	ModelCanvasAsset  *contract.ProjectAssetRef       `json:"model_canvas_asset,omitempty"`
	TimestampMS       int64                           `json:"timestamp_ms"`
	DerivationID      string                          `json:"derivation_id,omitempty"`
	ExtractionVersion string                          `json:"extraction_version,omitempty"`
}

type CommercePrerollV2ProductFrameCandidate struct {
	ID              string  `json:"id"`
	TimestampMS     int64   `json:"timestamp_ms"`
	Frontality      float64 `json:"frontality"`
	Sharpness       float64 `json:"sharpness"`
	Completeness    float64 `json:"completeness"`
	LogoReadability float64 `json:"logo_readability"`
	Occlusion       float64 `json:"occlusion"`
	Overall         float64 `json:"overall"`
}

type CommercePrerollV2ProductReferenceCandidate struct {
	ID         string                                 `json:"id"`
	SourceKind string                                 `json:"source_kind"`
	Label      string                                 `json:"label"`
	Frame      CommercePrerollV2DerivedFrame          `json:"frame"`
	Scores     CommercePrerollV2ProductFrameCandidate `json:"scores"`
}

type CommercePrerollV2ProductReferenceBatch struct {
	ID         string                                       `json:"id"`
	Revision   int64                                        `json:"revision"`
	Candidates []CommercePrerollV2ProductReferenceCandidate `json:"candidates"`
	SelectedID string                                       `json:"selected_id,omitempty"`
}

func (b CommercePrerollV2ProductReferenceBatch) Validate() error {
	if strings.TrimSpace(b.ID) == "" || b.Revision < 1 || len(b.Candidates) == 0 || strings.TrimSpace(b.SelectedID) == "" {
		return fmt.Errorf("commerce product reference batch is incomplete")
	}
	selected := false
	seen := map[string]struct{}{}
	for _, candidate := range b.Candidates {
		if strings.TrimSpace(candidate.ID) == "" || strings.TrimSpace(candidate.SourceKind) == "" || candidate.Frame.Asset == nil || candidate.Frame.Asset.Validate() != nil {
			return fmt.Errorf("commerce product reference candidate is incomplete")
		}
		if _, exists := seen[candidate.ID]; exists {
			return fmt.Errorf("commerce product reference candidate ids must be unique")
		}
		seen[candidate.ID] = struct{}{}
		selected = selected || candidate.ID == b.SelectedID
	}
	if !selected {
		return fmt.Errorf("selected commerce product reference does not exist")
	}
	return nil
}

type CommercePrerollV2FirstFrameCandidate struct {
	ID                string                          `json:"id"`
	VariantIndex      int                             `json:"variant_index"`
	ProviderJobID     string                          `json:"provider_job_id,omitempty"`
	Status            CommercePrerollV2ResourceStatus `json:"status"`
	Asset             *contract.ProjectAssetRef       `json:"asset,omitempty"`
	ModelCanvasAsset  *contract.ProjectAssetRef       `json:"model_canvas_asset,omitempty"`
	OutputCanvasAsset *contract.ProjectAssetRef       `json:"output_canvas_asset,omitempty"`
	VariantKey        string                          `json:"variant_key"`
	Title             string                          `json:"title"`
	Description       string                          `json:"description"`
	ErrorCode         string                          `json:"error_code,omitempty"`
	ErrorMessage      string                          `json:"error_message,omitempty"`
}

type CommercePrerollV2FirstFrameBatch struct {
	CommercePrerollV2AsyncResource
	ID             string                                 `json:"id"`
	Revision       int64                                  `json:"revision"`
	PromptRevision int64                                  `json:"prompt_revision"`
	Candidates     []CommercePrerollV2FirstFrameCandidate `json:"candidates"`
	SelectedAsset  *contract.ProjectAssetRef              `json:"selected_asset,omitempty"`
	SelectedID     string                                 `json:"selected_id,omitempty"`
}

type CommercePrerollV2GenerationSpec struct {
	ContractVersion string                   `json:"contract_version"`
	DraftRevision   int64                    `json:"draft_revision"`
	DurationSeconds int                      `json:"duration_seconds"`
	AspectRatio     string                   `json:"aspect_ratio"`
	Resolution      string                   `json:"resolution"`
	AudioPolicy     string                   `json:"audio_policy"`
	InputMode       string                   `json:"input_mode"`
	FirstFrameAsset contract.ProjectAssetRef `json:"first_frame_asset"`
	LastFrameAsset  contract.ProjectAssetRef `json:"last_frame_asset"`
	SourceCanvas    *ShortDramaSourceCanvas  `json:"source_canvas,omitempty"`
	ModelCanvas     *ShortDramaModelCanvas   `json:"model_canvas,omitempty"`
	OutputCanvas    *ShortDramaOutputCanvas  `json:"output_canvas,omitempty"`
	CompiledPrompt  string                   `json:"compiled_prompt"`
	PromptHash      string                   `json:"prompt_hash"`
	SpecHash        string                   `json:"spec_hash"`
}

type CommercePrerollV2Workspace struct {
	ContractVersion       string                                  `json:"contract_version"`
	TaskID                string                                  `json:"task_id"`
	Revision              int64                                   `json:"revision"`
	ActiveStage           CommercePrerollV2Stage                  `json:"active_stage"`
	SourceVideo           contract.ProjectAssetRef                `json:"source_video"`
	SourceMetadata        CreativeAssetSnapshot                   `json:"source_metadata"`
	SourceVideoRights     RightsConfirmation                      `json:"source_video_rights"`
	Analysis              CommercePrerollV2Analysis               `json:"analysis"`
	HookBatch             *CommercePrerollV2HookBatch             `json:"hook_batch,omitempty"`
	PromptDraft           *CommercePrerollV2PromptDraft           `json:"prompt_draft,omitempty"`
	ProductReference      *CommercePrerollV2DerivedFrame          `json:"product_reference,omitempty"`
	ProductReferenceBatch *CommercePrerollV2ProductReferenceBatch `json:"product_reference_batch,omitempty"`
	OpeningAnchor         *CommercePrerollV2DerivedFrame          `json:"opening_anchor,omitempty"`
	FirstFrameBatch       *CommercePrerollV2FirstFrameBatch       `json:"first_frame_batch,omitempty"`
	GenerationSpec        *CommercePrerollV2GenerationSpec        `json:"generation_spec,omitempty"`
	LatestVideoAttemptID  string                                  `json:"latest_video_attempt_id,omitempty"`
	VideoError            *contract.JobError                      `json:"video_error,omitempty"`
	RawOutputAsset        *contract.ProjectAssetRef               `json:"raw_output_asset,omitempty"`
	OutputNormalization   *CommercePrerollV2AsyncResource         `json:"output_normalization,omitempty"`
	OutputAsset           *contract.ProjectAssetRef               `json:"output_asset,omitempty"`
	AdoptedAsset          *contract.ProjectAssetRef               `json:"adopted_asset,omitempty"`
	CreatedAt             time.Time                               `json:"created_at"`
	UpdatedAt             time.Time                               `json:"updated_at"`
}

func (w CommercePrerollV2Workspace) Validate() error {
	if w.ContractVersion != CommercePrerollV2ContractVersion || strings.TrimSpace(w.TaskID) == "" ||
		w.Revision < 1 || !validCommercePrerollV2Stage(w.ActiveStage) || w.SourceVideo.Validate() != nil ||
		w.CreatedAt.IsZero() || w.UpdatedAt.IsZero() || w.UpdatedAt.Before(w.CreatedAt) {
		return fmt.Errorf("creative commerce preroll V2 workspace is incomplete")
	}
	if err := w.SourceVideoRights.Validate(); err != nil {
		return err
	}
	if !validCommercePrerollV2ResourceStatus(w.Analysis.Status) {
		return fmt.Errorf("creative commerce preroll V2 analysis state is invalid")
	}
	if w.ProductReferenceBatch != nil && w.ProductReferenceBatch.Validate() != nil {
		return fmt.Errorf("creative commerce product references are invalid")
	}
	if w.PromptDraft != nil {
		if w.PromptDraft.DurationSeconds < 6 || w.PromptDraft.DurationSeconds > 10 || len(w.PromptDraft.Beats) != 3 || len(w.PromptDraft.LockedConstraints) == 0 || strings.TrimSpace(w.PromptDraft.CreativePrompt) == "" || strings.TrimSpace(w.PromptDraft.CompiledPrompt) == "" || !strings.HasPrefix(w.PromptDraft.ContentHash, "sha256:") {
			return fmt.Errorf("creative commerce prompt draft is invalid")
		}
	}
	return nil
}

func validCommercePrerollV2Stage(stage CommercePrerollV2Stage) bool {
	switch stage {
	case CommercePrerollV2StageSourceReady, CommercePrerollV2StageAnalyzing,
		CommercePrerollV2StageUnderstandingReady, CommercePrerollV2StageUnderstandingConfirmed,
		CommercePrerollV2StageHooksReady, CommercePrerollV2StageHookSelected,
		CommercePrerollV2StageFramesGenerating, CommercePrerollV2StageFrameReady,
		CommercePrerollV2StageVideoGenerating, CommercePrerollV2StageVideoReady,
		CommercePrerollV2StageOutputAdopted:
		return true
	default:
		return false
	}
}

func validCommercePrerollV2ResourceStatus(status CommercePrerollV2ResourceStatus) bool {
	switch status {
	case CommercePrerollV2ResourceIdle, CommercePrerollV2ResourceQueued,
		CommercePrerollV2ResourceRunning, CommercePrerollV2ResourceReady,
		CommercePrerollV2ResourcePartial, CommercePrerollV2ResourceFailed,
		CommercePrerollV2ResourceCancelled:
		return true
	default:
		return false
	}
}
