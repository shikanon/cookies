// Package creative owns the advertising creative vertical's business state.
// It consumes only stable project context and Provider capability seams.
package creative

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

const (
	ScopeRead  contract.Scope = "creative.read"
	ScopeWrite contract.Scope = "creative.write"
)

type IntakeSource string

const (
	IntakeSourceManual           IntakeSource = "manual"
	IntakeSourceStrategyPackage  IntakeSource = "strategy_package"
	IntakeSourceTaskStrategy     IntakeSource = "task_strategy"
	IntakeSourceUploadedDocument IntakeSource = "uploaded_document"
	IntakeSourceConversation     IntakeSource = "conversation"
	IntakeSourceRequirement      IntakeSource = "requirement_snapshot"
)

type IntakeStatus string

const (
	IntakeDraft              IntakeStatus = "draft"
	IntakeNeedsClarification IntakeStatus = "needs_clarification"
	IntakeReady              IntakeStatus = "ready"
	IntakeSuperseded         IntakeStatus = "superseded"
)

type CreativeFormat string

const (
	FormatImageText CreativeFormat = "image_text"
	FormatVideo     CreativeFormat = "video"
)

type CreativeChannel string

const (
	ChannelXiaohongshu CreativeChannel = "xiaohongshu"
	ChannelDouyin      CreativeChannel = "douyin"
	ChannelKuaishou    CreativeChannel = "kuaishou"
)

type TaskStatus string

const (
	TaskDraft      TaskStatus = "draft"
	TaskInProgress TaskStatus = "in_progress"
	TaskReady      TaskStatus = "ready_for_review"
	TaskGenerating TaskStatus = "generating"
	TaskGenerated  TaskStatus = "generated"
	TaskRendering  TaskStatus = "rendering"
	TaskApproved   TaskStatus = "approved"
	TaskDelivered  TaskStatus = "delivered"
	// TaskArchived is a reversible-looking UI state backed by a retained record.
	// It deliberately does not delete drafts, Provider lineage, or frozen versions.
	TaskArchived TaskStatus = "archived"
)

type CreateIntakeRequest struct {
	ContractVersion          string                        `json:"contract_version,omitempty"`
	Source                   IntakeSource                  `json:"source"`
	RequirementSnapshotRef   *RequirementSnapshotReference `json:"requirement_snapshot_ref,omitempty"`
	BusinessCapabilityRef    *BusinessCapabilityReference  `json:"business_capability_ref,omitempty"`
	RequirementSnapshotInput *RequirementSnapshotInput     `json:"requirement_snapshot_input,omitempty"`
	// ParentIntakeID links a production-specific manual intake back to a
	// task-strategy handoff without allowing the manual flow to rewrite it.
	ParentIntakeID string `json:"parent_intake_id,omitempty"`
	// StrategyPackage is supplied only for the explicit, user-triggered handoff
	// from an immutable Strategy package. The server reads and validates that
	// package; callers never submit its content as trusted Creative input.
	StrategyPackage    *StrategyPackageReference         `json:"strategy_package,omitempty"`
	StrategyPackageRef *StrategyPackageContractReference `json:"strategy_package_ref,omitempty"`
	// SelectedRouteID is required by the strict v3 Strategy handoff. Creative
	// resolves it from the immutable Handoff snapshot; callers cannot submit a
	// route body or override the route after confirmation.
	SelectedRouteID      string                        `json:"selected_route_id,omitempty"`
	TaskOverlay          *TaskOverlayReference         `json:"task_overlay,omitempty"`
	TaskOverlayRef       *TaskOverlayContractReference `json:"task_overlay_ref,omitempty"`
	TaskOverlayInput     *TaskOverlayInput             `json:"task_overlay_input,omitempty"`
	StrategyHandoffInput json.RawMessage               `json:"strategy_handoff_input,omitempty"`
	// TaskStrategy is the immutable Strategy-side version selected by the
	// user. TaskStrategyInput is populated only by the server-side adapter.
	TaskStrategy              *TaskStrategyReference          `json:"task_strategy,omitempty"`
	TaskStrategyInput         *TaskStrategyInput              `json:"task_strategy_input,omitempty"`
	CreativeRoutes            []CreativeRouteSnapshot         `json:"creative_routes,omitempty"`
	Format                    CreativeFormat                  `json:"format,omitempty"`
	PerformanceMode           string                          `json:"performance_mode,omitempty"`
	ManualViralRemake         *ManualViralRemakeInput         `json:"manual_viral_remake,omitempty"`
	ManualShortDramaPreroll   *ManualShortDramaPrerollInput   `json:"manual_short_drama_preroll,omitempty"`
	ManualShortDramaPrerollV2 *ManualShortDramaPrerollV2Input `json:"manual_short_drama_preroll_v2,omitempty"`
	ManualGamePreroll         *ManualGamePrerollInput         `json:"manual_game_preroll,omitempty"`
	ManualCommercePreroll     *ManualCommercePrerollInput     `json:"manual_commerce_preroll,omitempty"`
	ManualCommercePrerollV2   *ManualCommercePrerollV2Input   `json:"manual_commerce_preroll_v2,omitempty"`
	ManualBrandFilm           *ManualBrandFilmInput           `json:"manual_brand_film,omitempty"`
	Channel                   CreativeChannel                 `json:"channel"`
	Objective                 string                          `json:"objective"`
	Audience                  string                          `json:"audience"`
	CoreMessage               string                          `json:"core_message"`
	CallToAction              string                          `json:"call_to_action"`
	Concept                   string                          `json:"concept"`
	Tone                      []string                        `json:"tone"`
	VisualKeywords            []string                        `json:"visual_keywords"`
	Mandatory                 []string                        `json:"mandatory_elements"`
	Prohibited                []string                        `json:"prohibited_claims"`
}

type CreativeRouteSnapshot struct {
	RouteID                   string                     `json:"route_id,omitempty"`
	RouteType                 string                     `json:"route_type"`
	VideoPurpose              string                     `json:"video_purpose"`
	Channels                  []string                   `json:"channels"`
	Reason                    string                     `json:"reason"`
	TargetDurationSeconds     int                        `json:"target_duration_seconds"`
	AspectRatio               string                     `json:"aspect_ratio"`
	Resolution                string                     `json:"resolution,omitempty"`
	SourceAssetRefs           []contract.AssetVersionRef `json:"source_asset_refs"`
	EvidenceRefs              []string                   `json:"evidence_refs"`
	RequiresHumanConfirmation bool                       `json:"requires_human_confirmation"`
	ReadinessStatus           string                     `json:"readiness_status,omitempty"`
}

func (r CreativeRouteSnapshot) Validate() error {
	if r.RouteType != CreativeRouteImageText && r.RouteType != CreativeRouteBrandVideo && r.RouteType != "pre_roll" && r.RouteType != PerformanceModeViralRemake &&
		r.RouteType != PerformanceModeShortDramaPreroll && r.RouteType != PerformanceModeGamePreroll &&
		r.RouteType != PerformanceModeCommercePreroll && r.RouteType != PerformanceModeBrandFilm {
		return fmt.Errorf("creative route type %q is unsupported", r.RouteType)
	}
	if r.RouteType == CreativeRouteImageText {
		if len(r.Channels) != 1 || r.Channels[0] != string(ChannelXiaohongshu) ||
			r.AspectRatio != "3:4" || strings.TrimSpace(r.RouteID) == "" ||
			strings.TrimSpace(r.Reason) == "" || r.VideoPurpose != "" ||
			r.TargetDurationSeconds != 0 {
			return fmt.Errorf("creative image-text route is incomplete")
		}
		return nil
	}
	if r.RouteType == CreativeRouteBrandVideo {
		if r.VideoPurpose != "brand" || len(r.Channels) == 0 ||
			r.TargetDurationSeconds < 1 || strings.TrimSpace(r.AspectRatio) == "" ||
			strings.TrimSpace(r.RouteID) == "" || strings.TrimSpace(r.Reason) == "" ||
			!r.RequiresHumanConfirmation {
			return fmt.Errorf("creative brand-video route is incomplete")
		}
		for _, channel := range r.Channels {
			if channel != "xiaohongshu" && channel != "wechat_official_account" &&
				channel != "douyin" && channel != "kuaishou" {
				return fmt.Errorf("creative brand-video route channel %q is unsupported", channel)
			}
		}
		for _, ref := range r.SourceAssetRefs {
			if err := ref.Validate(); err != nil {
				return fmt.Errorf("creative brand-video route source asset: %w", err)
			}
		}
		return nil
	}
	if r.RouteType == PerformanceModeViralRemake && r.RouteID != ManualViralRemakeRouteID {
		return fmt.Errorf("viral remake route_id must be %q", ManualViralRemakeRouteID)
	}
	if r.RouteType == PerformanceModeShortDramaPreroll &&
		r.RouteID != ManualShortDramaPrerollRouteID && r.RouteID != ManualShortDramaPrerollV2RouteID {
		return fmt.Errorf("short drama preroll route_id is unsupported")
	}
	if r.RouteType == PerformanceModeGamePreroll && r.RouteID != ManualGamePrerollRouteID {
		return fmt.Errorf("game preroll route_id must be %q", ManualGamePrerollRouteID)
	}
	if r.RouteType == PerformanceModeCommercePreroll && r.RouteID != ManualCommercePrerollRouteID && r.RouteID != ManualCommercePrerollV2RouteID {
		return fmt.Errorf("commerce preroll route_id is unsupported")
	}
	if r.RouteType == PerformanceModeBrandFilm && r.RouteID != ManualBrandFilmRouteID {
		return fmt.Errorf("brand film route_id must be %q", ManualBrandFilmRouteID)
	}
	if r.RouteType == "pre_roll" && r.TargetDurationSeconds != 5 {
		return fmt.Errorf("creative pre-roll route duration must be 5 seconds")
	}
	if r.RouteType == PerformanceModeShortDramaPreroll && r.TargetDurationSeconds != 6 {
		return fmt.Errorf("short drama preroll route duration must be 6 seconds")
	}
	if r.RouteType == PerformanceModeGamePreroll && r.TargetDurationSeconds != 6 {
		return fmt.Errorf("game preroll route duration must be 6 seconds")
	}
	if r.RouteType == PerformanceModeCommercePreroll && r.RouteID == ManualCommercePrerollRouteID && r.TargetDurationSeconds != 6 {
		return fmt.Errorf("legacy commerce preroll route duration must be 6 seconds")
	}
	if r.RouteType == PerformanceModeCommercePreroll && r.RouteID == ManualCommercePrerollV2RouteID &&
		(r.TargetDurationSeconds < 6 || r.TargetDurationSeconds > 10) {
		return fmt.Errorf("commerce preroll V2 route duration must be between 6 and 10 seconds")
	}
	if r.RouteType == PerformanceModeBrandFilm && r.TargetDurationSeconds != 15 {
		return fmt.Errorf("brand film route duration must be 15 seconds")
	}
	expectedPurpose := "performance"
	if r.RouteType == PerformanceModeBrandFilm {
		expectedPurpose = "brand"
	}
	if r.VideoPurpose != expectedPurpose || len(r.Channels) == 0 ||
		r.TargetDurationSeconds < 4 || r.TargetDurationSeconds > 60 || r.AspectRatio != "9:16" || strings.TrimSpace(r.Reason) == "" ||
		!r.RequiresHumanConfirmation {
		return fmt.Errorf("creative video route is incomplete")
	}
	for _, channel := range r.Channels {
		if channel != "douyin" && channel != "kuaishou" {
			return fmt.Errorf("creative route channel %q is unsupported", channel)
		}
	}
	for _, ref := range r.SourceAssetRefs {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("creative route source asset: %w", err)
		}
	}
	return nil
}

type StrategyPackageReference struct {
	PackageID              string `json:"package_id"`
	PackageVersion         int64  `json:"package_version"`
	ExpectedContentHash    string `json:"expected_content_hash"`
	HandoffContractVersion string `json:"handoff_contract_version,omitempty"`
	ExpectedHandoffHash    string `json:"expected_handoff_hash,omitempty"`
}

type StrategyPackageContractReference struct {
	PackageID              string `json:"package_id"`
	PackageVersion         int64  `json:"package_version"`
	PackageContentHash     string `json:"package_content_hash"`
	HandoffContractVersion string `json:"handoff_contract_version"`
	HandoffContentHash     string `json:"handoff_content_hash"`
}

func (r StrategyPackageReference) Validate() error {
	if strings.TrimSpace(r.PackageID) == "" || r.PackageVersion < 1 || strings.TrimSpace(r.ExpectedContentHash) == "" {
		return fmt.Errorf("strategy_package package_id, package_version, and expected_content_hash are required")
	}
	return nil
}

func (r CreateIntakeRequest) Validate() error {
	if r.Source == "" {
		return fmt.Errorf("source is required")
	}
	if r.Source != IntakeSourceRequirement && (r.RequirementSnapshotRef != nil || r.BusinessCapabilityRef != nil || r.RequirementSnapshotInput != nil) {
		return fmt.Errorf("requirement snapshot fields are only valid for requirement_snapshot intake")
	}
	switch r.Source {
	case IntakeSourceManual:
		if r.StrategyPackage != nil || r.TaskStrategy != nil || r.TaskStrategyInput != nil ||
			r.TaskOverlay != nil || r.TaskOverlayInput != nil {
			return fmt.Errorf("manual intake must not include a Strategy reference")
		}
		if r.ManualViralRemake != nil {
			return r.validateManualViralRemake()
		}
		if r.ManualShortDramaPreroll != nil {
			return r.validateManualShortDramaPreroll()
		}
		if r.ManualShortDramaPrerollV2 != nil {
			return r.validateManualShortDramaPrerollV2()
		}
		if r.ManualGamePreroll != nil {
			return r.validateManualGamePreroll()
		}
		if r.ManualCommercePreroll != nil {
			return r.validateManualCommercePreroll()
		}
		if r.ManualCommercePrerollV2 != nil {
			return r.validateManualCommercePrerollV2()
		}
		if r.ManualBrandFilm != nil {
			return r.validateManualBrandFilm()
		}
		if len(r.CreativeRoutes) != 0 || r.SelectedRouteID != "" || r.Format == FormatVideo || r.PerformanceMode != "" {
			return fmt.Errorf("manual image intake must not include video routing")
		}
	case IntakeSourceStrategyPackage:
		if r.StrategyPackage == nil || r.TaskStrategy != nil || r.TaskStrategyInput != nil || r.ParentIntakeID != "" {
			return fmt.Errorf("strategy_package is required for a strategy intake")
		}
		if r.TaskOverlayInput != nil {
			return fmt.Errorf("task overlay content is resolved by Creative and must not be submitted by a caller")
		}
		if r.TaskOverlay != nil && r.ContractVersion != CreativeIntakeCreateV3ContractVersion {
			return fmt.Errorf("task_overlay requires creative-intake-create/v3")
		}
		if len(r.CreativeRoutes) != 0 {
			return fmt.Errorf("creative_routes are resolved from Strategy and must not be submitted by a caller")
		}
		if err := r.StrategyPackage.Validate(); err != nil {
			return err
		}
		if r.ContractVersion == CreativeIntakeCreateV3ContractVersion {
			if r.StrategyPackage.HandoffContractVersion != "strategy-creative-handoff/v1" ||
				strings.TrimSpace(r.StrategyPackage.ExpectedHandoffHash) == "" ||
				strings.TrimSpace(r.SelectedRouteID) == "" {
				return fmt.Errorf("v3 strategy intake requires the frozen handoff version/hash and selected_route_id")
			}
			if r.TaskOverlay != nil {
				if err := r.TaskOverlay.Validate(); err != nil {
					return err
				}
			}
		}
		return nil
	case IntakeSourceTaskStrategy:
		if r.TaskStrategy == nil {
			return fmt.Errorf("task_strategy is required for a task strategy intake")
		}
		if r.StrategyPackage != nil || r.TaskStrategyInput != nil || r.ParentIntakeID != "" ||
			r.TaskOverlay != nil || r.TaskOverlayInput != nil {
			return fmt.Errorf("task strategy content is resolved by Creative and must not be submitted by a caller")
		}
		if len(r.CreativeRoutes) != 0 || r.Format != "" || r.PerformanceMode != "" ||
			r.Channel != "" || strings.TrimSpace(r.Objective) != "" || strings.TrimSpace(r.Audience) != "" ||
			strings.TrimSpace(r.CoreMessage) != "" {
			return fmt.Errorf("task strategy mapped fields are resolved by Creative and must not be submitted by a caller")
		}
		return r.TaskStrategy.Validate()
	case IntakeSourceRequirement:
		return r.validateRequirementSnapshotV4()
	default:
		return fmt.Errorf("unsupported Creative intake source %q", r.Source)
	}
	return r.validateContent()
}

func (r CreateIntakeRequest) validateManualShortDramaPreroll() error {
	if r.Format != FormatVideo || r.PerformanceMode != PerformanceModeShortDramaPreroll {
		return fmt.Errorf("manual short drama preroll intake requires format=video and performance_mode=short_drama_preroll")
	}
	if r.Channel != ChannelDouyin && r.Channel != ChannelKuaishou {
		return fmt.Errorf("manual short drama preroll supports douyin or kuaishou")
	}
	if len(r.CreativeRoutes) != 1 || r.CreativeRoutes[0].RouteID != ManualShortDramaPrerollRouteID {
		return fmt.Errorf("manual short drama preroll requires exactly one stable route")
	}
	if err := r.CreativeRoutes[0].Validate(); err != nil {
		return err
	}
	if err := r.ManualShortDramaPreroll.Validate(); err != nil {
		return err
	}
	return r.validateVideoContent()
}

func (r CreateIntakeRequest) validateManualShortDramaPrerollV2() error {
	if r.Format != FormatVideo || r.PerformanceMode != PerformanceModeShortDramaPreroll {
		return fmt.Errorf("manual short drama preroll V2 intake requires format=video and performance_mode=short_drama_preroll")
	}
	if r.Channel != ChannelDouyin && r.Channel != ChannelKuaishou {
		return fmt.Errorf("manual short drama preroll V2 supports douyin or kuaishou")
	}
	if len(r.CreativeRoutes) != 1 || r.CreativeRoutes[0].RouteID != ManualShortDramaPrerollV2RouteID {
		return fmt.Errorf("manual short drama preroll V2 requires exactly one stable route")
	}
	if err := r.CreativeRoutes[0].Validate(); err != nil {
		return err
	}
	if err := r.ManualShortDramaPrerollV2.Validate(); err != nil {
		return err
	}
	if len(r.CreativeRoutes[0].SourceAssetRefs) != 1 ||
		r.CreativeRoutes[0].SourceAssetRefs[0] != r.ManualShortDramaPrerollV2.SourceVideo {
		return fmt.Errorf("short drama preroll V2 route must freeze the licensed source video")
	}
	return r.validateVideoContent()
}

func (r CreateIntakeRequest) validateManualGamePreroll() error {
	if r.Format != FormatVideo || r.PerformanceMode != PerformanceModeGamePreroll {
		return fmt.Errorf("manual game preroll intake requires format=video and performance_mode=game_preroll")
	}
	if r.Channel != ChannelDouyin && r.Channel != ChannelKuaishou {
		return fmt.Errorf("manual game preroll supports douyin or kuaishou")
	}
	if len(r.CreativeRoutes) != 1 || r.CreativeRoutes[0].RouteID != ManualGamePrerollRouteID {
		return fmt.Errorf("manual game preroll requires exactly one stable route")
	}
	if err := r.CreativeRoutes[0].Validate(); err != nil {
		return err
	}
	if err := r.ManualGamePreroll.Validate(); err != nil {
		return err
	}
	if len(r.CreativeRoutes[0].SourceAssetRefs) != 1 ||
		r.CreativeRoutes[0].SourceAssetRefs[0] != r.ManualGamePreroll.SourceVideo {
		return fmt.Errorf("game preroll route must freeze the licensed source video")
	}
	return r.validateVideoContent()
}

func (r CreateIntakeRequest) validateManualCommercePreroll() error {
	if r.Format != FormatVideo || r.PerformanceMode != PerformanceModeCommercePreroll {
		return fmt.Errorf("manual commerce preroll intake requires format=video and performance_mode=commerce_preroll")
	}
	if r.Channel != ChannelDouyin {
		return fmt.Errorf("manual commerce preroll intake requires channel=douyin")
	}
	if len(r.CreativeRoutes) != 1 || r.CreativeRoutes[0].RouteID != ManualCommercePrerollRouteID {
		return fmt.Errorf("manual commerce preroll requires exactly one stable route")
	}
	if err := r.CreativeRoutes[0].Validate(); err != nil {
		return err
	}
	if err := r.ManualCommercePreroll.Validate(); err != nil {
		return err
	}
	return r.validateVideoContent()
}

func (r CreateIntakeRequest) validateManualCommercePrerollV2() error {
	if r.Format != FormatVideo || r.PerformanceMode != PerformanceModeCommercePreroll {
		return fmt.Errorf("manual commerce preroll V2 intake requires format=video and performance_mode=commerce_preroll")
	}
	if r.Channel != ChannelDouyin && r.Channel != ChannelKuaishou {
		return fmt.Errorf("manual commerce preroll V2 supports douyin or kuaishou")
	}
	if len(r.CreativeRoutes) != 1 || r.CreativeRoutes[0].RouteID != ManualCommercePrerollV2RouteID {
		return fmt.Errorf("manual commerce preroll V2 requires exactly one stable route")
	}
	if err := r.CreativeRoutes[0].Validate(); err != nil {
		return err
	}
	if err := r.ManualCommercePrerollV2.Validate(); err != nil {
		return err
	}
	if len(r.CreativeRoutes[0].SourceAssetRefs) != 1 ||
		r.CreativeRoutes[0].SourceAssetRefs[0] != r.ManualCommercePrerollV2.SourceVideo {
		return fmt.Errorf("commerce preroll V2 route must freeze the licensed source video")
	}
	return r.validateVideoContent()
}

func (r CreateIntakeRequest) validateManualBrandFilm() error {
	if r.Format != FormatVideo || r.PerformanceMode != PerformanceModeBrandFilm {
		return fmt.Errorf("manual brand film intake requires format=video and performance_mode=brand_video")
	}
	if r.Channel != ChannelDouyin {
		return fmt.Errorf("manual brand film fixture requires channel=douyin")
	}
	if len(r.CreativeRoutes) != 1 || r.CreativeRoutes[0].RouteID != ManualBrandFilmRouteID {
		return fmt.Errorf("manual brand film requires exactly one stable route")
	}
	if err := r.CreativeRoutes[0].Validate(); err != nil {
		return err
	}
	if err := r.ManualBrandFilm.Validate(); err != nil {
		return err
	}
	return r.validateVideoContent()
}

func (r CreateIntakeRequest) validateContent() error {
	if r.Channel != ChannelXiaohongshu {
		return fmt.Errorf("Creative M1 supports the xiaohongshu channel")
	}
	if len(r.Objective) > 500 || len(r.Audience) > 500 || len(r.CoreMessage) > 1000 || len(r.CallToAction) > 300 || len(r.Concept) > 500 {
		return fmt.Errorf("creative input exceeds its maximum length")
	}
	if err := validateStringList("tone", r.Tone, 12, 80); err != nil {
		return err
	}
	if err := validateStringList("visual_keywords", r.VisualKeywords, 16, 120); err != nil {
		return err
	}
	if err := validateStringList("mandatory_elements", r.Mandatory, 20, 200); err != nil {
		return err
	}
	return validateStringList("prohibited_claims", r.Prohibited, 20, 200)
}

func (r CreateIntakeRequest) validateManualViralRemake() error {
	if r.Format != FormatVideo || r.PerformanceMode != PerformanceModeViralRemake {
		return fmt.Errorf("manual viral remake intake requires format=video and performance_mode=viral_remake")
	}
	if r.Channel != ChannelDouyin && r.Channel != ChannelKuaishou {
		return fmt.Errorf("manual viral remake supports douyin or kuaishou")
	}
	if len(r.CreativeRoutes) != 1 {
		return fmt.Errorf("manual viral remake requires exactly one stable route")
	}
	if r.CreativeRoutes[0].RouteID != ManualViralRemakeRouteID {
		return fmt.Errorf("manual viral remake route_id must be %q", ManualViralRemakeRouteID)
	}
	if err := r.CreativeRoutes[0].Validate(); err != nil {
		return err
	}
	if err := r.ManualViralRemake.Validate(); err != nil {
		return err
	}
	return r.validateVideoContent()
}

func (r CreateIntakeRequest) validateVideoContent() error {
	if len(r.Objective) > 500 || len(r.Audience) > 500 || len(r.CoreMessage) > 1000 || len(r.CallToAction) > 300 || len(r.Concept) > 500 {
		return fmt.Errorf("creative input exceeds its maximum length")
	}
	if err := validateStringList("tone", r.Tone, 12, 80); err != nil {
		return err
	}
	if err := validateStringList("visual_keywords", r.VisualKeywords, 16, 120); err != nil {
		return err
	}
	if err := validateStringList("mandatory_elements", r.Mandatory, 20, 200); err != nil {
		return err
	}
	return validateStringList("prohibited_claims", r.Prohibited, 20, 200)
}

func (r CreateIntakeRequest) missingFields() []string {
	missing := make([]string, 0, 3)
	if strings.TrimSpace(r.Objective) == "" {
		missing = append(missing, "objective")
	}
	if strings.TrimSpace(r.Audience) == "" {
		missing = append(missing, "audience")
	}
	if strings.TrimSpace(r.CoreMessage) == "" {
		missing = append(missing, "core_message")
	}
	return missing
}

type CreativeIntake struct {
	ContractVersion   string                  `json:"contract_version,omitempty"`
	ID                string                  `json:"id"`
	OrganizationID    contract.OrganizationID `json:"organization_id"`
	ProjectID         contract.ProjectID      `json:"project_id"`
	Source            IntakeSource            `json:"source"`
	Status            IntakeStatus            `json:"status"`
	Request           CreateIntakeRequest     `json:"request"`
	MissingFields     []string                `json:"missing_fields"`
	Warnings          []string                `json:"warnings"`
	ConfirmedBy       string                  `json:"confirmed_by,omitempty"`
	Principal         contract.Principal      `json:"-"`
	IdempotencyKey    contract.IdempotencyKey `json:"-"`
	RequestHash       string                  `json:"-"`
	InputIdentityHash string                  `json:"input_identity_hash,omitempty"`
	Version           int64                   `json:"version"`
	CreatedAt         time.Time               `json:"created_at"`
	UpdatedAt         time.Time               `json:"updated_at"`
}

type CreativeTask struct {
	ID              string                  `json:"id"`
	DisplayName     string                  `json:"display_name"`
	OrganizationID  contract.OrganizationID `json:"organization_id"`
	ProjectID       contract.ProjectID      `json:"project_id"`
	IntakeID        string                  `json:"intake_id"`
	Format          CreativeFormat          `json:"format"`
	Channel         CreativeChannel         `json:"channel"`
	VideoPurpose    string                  `json:"video_purpose,omitempty"`
	PerformanceMode string                  `json:"performance_mode,omitempty"`
	LineageKey      string                  `json:"-"`
	Status          TaskStatus              `json:"status"`
	Direction       CreativeDirection       `json:"direction"`
	Version         int64                   `json:"version"`
	CreatedAt       time.Time               `json:"created_at"`
	UpdatedAt       time.Time               `json:"updated_at"`
}

type RenameTaskRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	DisplayName     string `json:"display_name"`
}

func (r RenameTaskRequest) Validate() error {
	name := strings.TrimSpace(r.DisplayName)
	if r.ExpectedVersion < 1 || name == "" || len([]rune(name)) > 80 {
		return fmt.Errorf("expected_version and a display_name of at most 80 characters are required")
	}
	return nil
}

type CreateVideoTaskRequest struct {
	SelectedRouteID string                   `json:"selected_route_id,omitempty"`
	DirectionID     string                   `json:"direction_id,omitempty"`
	RouteIndex      int                      `json:"route_index"`
	Channel         CreativeChannel          `json:"channel"`
	SourceVideo     contract.AssetVersionRef `json:"source_video,omitempty"`
	Concept         string                   `json:"concept"`
	Prompt          string                   `json:"prompt"`
	CallToAction    string                   `json:"call_to_action"`
	Mandatory       []string                 `json:"mandatory_elements"`
	Prohibited      []string                 `json:"prohibited_claims"`
	ConfirmRoute    bool                     `json:"confirm_route"`
}

func (r CreateVideoTaskRequest) Validate() error {
	if (strings.TrimSpace(r.SelectedRouteID) == "" && r.RouteIndex < 0) ||
		(r.Channel != ChannelXiaohongshu && r.Channel != ChannelDouyin && r.Channel != ChannelKuaishou) || !r.ConfirmRoute {
		return fmt.Errorf("selected_route_id (or legacy route_index), supported video channel, and explicit route confirmation are required")
	}
	if len(r.DirectionID) > 96 || len(r.Concept) > 500 || len(r.Prompt) > 4000 || len(r.CallToAction) > 300 {
		return fmt.Errorf("video direction, concept, prompt, or call_to_action exceeds its maximum length")
	}
	if err := validateStringList("mandatory_elements", r.Mandatory, 20, 200); err != nil {
		return err
	}
	return validateStringList("prohibited_claims", r.Prohibited, 20, 200)
}

type VideoDraft struct {
	ContractVersion     string                        `json:"contract_version"`
	TaskID              string                        `json:"task_id"`
	Revision            int64                         `json:"revision"`
	Concept             string                        `json:"concept"`
	Prompt              string                        `json:"prompt"`
	DurationSeconds     int                           `json:"duration_seconds"`
	AspectRatio         string                        `json:"aspect_ratio"`
	Resolution          string                        `json:"resolution"`
	VideoPurpose        string                        `json:"video_purpose,omitempty"`
	SourceVideo         contract.AssetVersionRef      `json:"source_video,omitempty"`
	Mandatory           []string                      `json:"mandatory_elements"`
	Prohibited          []string                      `json:"prohibited_claims"`
	CallToAction        string                        `json:"cta"`
	ViralRemake         *ViralRemakeDraft             `json:"viral_remake,omitempty"`
	ShortDramaPreroll   *ShortDramaPrerollDraft       `json:"short_drama_preroll,omitempty"`
	ShortDramaPrerollV2 *ShortDramaPrerollV2Workspace `json:"short_drama_preroll_v2,omitempty"`
	GamePreroll         *GamePrerollDraft             `json:"game_preroll,omitempty"`
	CommercePreroll     *CommercePrerollDraft         `json:"commerce_preroll,omitempty"`
	CommercePrerollV2   *CommercePrerollV2Workspace   `json:"commerce_preroll_v2,omitempty"`
	BrandFilm           *BrandFilmDraft               `json:"brand_film,omitempty"`
	CreatedAt           time.Time                     `json:"created_at"`
}

func (d VideoDraft) Validate() error {
	if d.ContractVersion != "creative-video-draft/v1" || strings.TrimSpace(d.TaskID) == "" || d.Revision < 1 ||
		strings.TrimSpace(d.Concept) == "" || strings.TrimSpace(d.Prompt) == "" || d.DurationSeconds < 4 || d.DurationSeconds > 60 ||
		strings.TrimSpace(d.AspectRatio) == "" || strings.TrimSpace(d.Resolution) == "" || d.CreatedAt.IsZero() {
		return fmt.Errorf("creative video draft is incomplete")
	}
	if d.VideoPurpose != "brand" && d.ShortDramaPreroll == nil && d.CommercePreroll == nil && d.CommercePrerollV2 == nil && d.BrandFilm == nil && d.SourceVideo.Validate() != nil {
		return fmt.Errorf("creative video draft is incomplete")
	}
	if d.ViralRemake != nil && d.ViralRemake.Validate() != nil {
		return fmt.Errorf("creative viral remake draft is incomplete")
	}
	if d.ShortDramaPreroll != nil && d.ShortDramaPreroll.Validate() != nil {
		return fmt.Errorf("creative short drama preroll draft is incomplete")
	}
	if d.ShortDramaPrerollV2 != nil && d.ShortDramaPrerollV2.Validate() != nil {
		return fmt.Errorf("creative short drama preroll V2 draft is incomplete")
	}
	if d.GamePreroll != nil && d.GamePreroll.Validate() != nil {
		return fmt.Errorf("creative game preroll draft is incomplete")
	}
	if d.CommercePreroll != nil && d.CommercePreroll.Validate() != nil {
		return fmt.Errorf("creative commerce preroll draft is incomplete")
	}
	if d.CommercePrerollV2 != nil && d.CommercePrerollV2.Validate() != nil {
		return fmt.Errorf("creative commerce preroll V2 draft is incomplete")
	}
	if d.BrandFilm != nil && d.BrandFilm.Validate() != nil {
		return fmt.Errorf("creative brand film draft is incomplete")
	}
	return nil
}

type CreativeContentType string

const (
	ContentTypeLifestyle             CreativeContentType = "lifestyle"
	ContentTypeIngredientExplanation CreativeContentType = "ingredient_explanation"
	ContentTypeUsageScenario         CreativeContentType = "usage_scenario"
	ContentTypeListGuide             CreativeContentType = "list_guide"
	ContentTypeComparison            CreativeContentType = "comparison"
	ContentTypeCustom                CreativeContentType = "custom"
)

// CreateTaskRequest is the explicit second-stage brief that differentiates
// several Creative tasks produced from one approved Strategy package.
type CreateTaskRequest struct {
	DirectionID  string              `json:"direction_id,omitempty"`
	ContentType  CreativeContentType `json:"content_type"`
	Focus        string              `json:"focus"`
	Audience     string              `json:"audience,omitempty"`
	CoreMessage  string              `json:"core_message,omitempty"`
	CallToAction string              `json:"call_to_action,omitempty"`
}

func (r CreateTaskRequest) Validate() error {
	switch r.ContentType {
	case ContentTypeLifestyle, ContentTypeIngredientExplanation, ContentTypeUsageScenario, ContentTypeListGuide, ContentTypeComparison, ContentTypeCustom:
	default:
		return fmt.Errorf("unsupported Creative content_type %q", r.ContentType)
	}
	if (len(strings.TrimSpace(r.Focus)) == 0 && len(strings.TrimSpace(r.DirectionID)) == 0) ||
		len(r.Focus) > 300 || len(r.DirectionID) > 96 || len(r.Audience) > 500 ||
		len(r.CoreMessage) > 1000 || len(r.CallToAction) > 300 {
		return fmt.Errorf("creative task focus is required or task input exceeds its maximum length")
	}
	return nil
}

type CreativeDirection struct {
	DirectionVersionID string              `json:"direction_version_id,omitempty"`
	InputIdentityHash  string              `json:"input_identity_hash,omitempty"`
	ContentType        CreativeContentType `json:"content_type"`
	Focus              string              `json:"focus"`
	Audience           string              `json:"audience"`
	CoreMessage        string              `json:"core_message"`
	CallToAction       string              `json:"call_to_action"`
	Concept            string              `json:"concept"`
	Tone               []string            `json:"tone"`
	VisualKeywords     []string            `json:"visual_keywords"`
}

type ImageTextDraft struct {
	ContractVersion         string                 `json:"contract_version,omitempty"`
	TaskID                  string                 `json:"task_id"`
	Version                 int64                  `json:"version"`
	GenerationSourceVersion *int64                 `json:"generation_source_version,omitempty"`
	DirectionRef            *ImageTextDirectionRef `json:"direction_ref,omitempty"`
	InputIdentityHash       string                 `json:"input_identity_hash,omitempty"`
	Status                  string                 `json:"status"`
	TitleCandidates         []string               `json:"title_candidates"`
	SelectedTitle           string                 `json:"selected_title,omitempty"`
	Body                    string                 `json:"body"`
	Topics                  []string               `json:"topics"`
	CoverCopy               string                 `json:"cover_copy"`
	ImagePlan               []ImagePlanItem        `json:"image_plan"`
	CreatedAt               time.Time              `json:"created_at"`
}

// ReviseDraftRequest replaces the editable content of the current draft. The
// expected version is an optimistic-lock boundary: a stale browser must reload
// rather than silently overwriting someone else's Creative revision.
type ReviseDraftRequest struct {
	ExpectedVersion int64           `json:"expected_version"`
	TitleCandidates []string        `json:"title_candidates"`
	Body            string          `json:"body"`
	Topics          []string        `json:"topics"`
	CoverCopy       string          `json:"cover_copy"`
	ImagePlan       []ImagePlanItem `json:"image_plan"`
}

// BindImageAssetRequest binds an already-ready project asset to one planned
// image. The Asset context owns the asset itself; Creative retains only the
// immutable asset-version reference in its next Draft revision.
type BindImageAssetRequest struct {
	ExpectedDraftVersion int64                    `json:"expected_draft_version"`
	ImagePlanOrder       int                      `json:"image_plan_order"`
	AssetRef             contract.AssetVersionRef `json:"asset_ref"`
}

func (r BindImageAssetRequest) Validate() error {
	if r.ExpectedDraftVersion < 1 || r.ImagePlanOrder < 1 || r.ImagePlanOrder > 12 {
		return fmt.Errorf("expected_draft_version and image_plan_order are invalid")
	}
	return r.AssetRef.Validate()
}

// CreateImageJobRequest is deliberately scoped to an image-plan position.
// A retry for one failed image never recreates the other images in the group.
type CreateImageJobRequest struct {
	ImagePlanOrder int    `json:"image_plan_order"`
	ModelAlias     string `json:"model_alias"`
}

type CreateVideoJobRequest struct {
	ModelAlias     string                       `json:"model_alias"`
	Prompt         *CreativeVideoPrompt         `json:"prompt,omitempty"`
	GenerationSpec *CreativeVideoGenerationSpec `json:"generation_spec,omitempty"`
	Approval       *VideoGenerationApproval     `json:"approval,omitempty"`
}

func (r CreateVideoJobRequest) Validate() error {
	if len(strings.TrimSpace(r.ModelAlias)) > 128 {
		return fmt.Errorf("model_alias is too long")
	}
	approvedFields := 0
	if r.Prompt != nil {
		approvedFields++
	}
	if r.GenerationSpec != nil {
		approvedFields++
	}
	if r.Approval != nil {
		approvedFields++
	}
	if approvedFields != 0 && approvedFields != 3 {
		return fmt.Errorf("prompt, generation_spec, and approval must be supplied together")
	}
	return nil
}

func (r CreateImageJobRequest) Validate() error {
	if r.ImagePlanOrder < 1 || r.ImagePlanOrder > 12 {
		return fmt.Errorf("image_plan_order must be between 1 and 12")
	}
	if len(r.ModelAlias) > 128 {
		return fmt.Errorf("model_alias is too long")
	}
	return nil
}

func (r ReviseDraftRequest) Validate() error {
	if r.ExpectedVersion < 1 {
		return fmt.Errorf("expected_version must be positive")
	}
	if len(r.TitleCandidates) < 3 || len(r.TitleCandidates) > 8 {
		return fmt.Errorf("title_candidates must contain between 3 and 8 candidates")
	}
	if len(strings.TrimSpace(r.Body)) == 0 || len(r.Body) > 5000 {
		return fmt.Errorf("body is required and must not exceed 5000 characters")
	}
	if len(strings.TrimSpace(r.CoverCopy)) == 0 || len([]rune(r.CoverCopy)) > 30 {
		return fmt.Errorf("cover_copy is required and must not exceed 30 characters")
	}
	if len(r.Topics) > 12 || len(r.ImagePlan) < 1 || len(r.ImagePlan) > 12 {
		return fmt.Errorf("topics or image_plan is outside the supported range")
	}
	for _, title := range r.TitleCandidates {
		if len(strings.TrimSpace(title)) == 0 || len([]rune(title)) > 80 {
			return fmt.Errorf("title_candidates contains an invalid value")
		}
	}
	for _, topic := range r.Topics {
		if len(strings.TrimSpace(topic)) == 0 || len([]rune(topic)) > 80 {
			return fmt.Errorf("topics contains an invalid value")
		}
	}
	for index, item := range r.ImagePlan {
		if item.Order != index+1 || strings.TrimSpace(item.Purpose) == "" || strings.TrimSpace(item.VisualBrief) == "" || strings.TrimSpace(item.Caption) == "" {
			return fmt.Errorf("image_plan must have ordered, complete items")
		}
		if item.AssetRef != nil {
			if err := item.AssetRef.Validate(); err != nil {
				return fmt.Errorf("image_plan asset_ref: %w", err)
			}
		}
	}
	return nil
}

func (r ReviseDraftRequest) Draft(taskID string, version int64, now time.Time) ImageTextDraft {
	return ImageTextDraft{TaskID: taskID, Version: version, Status: "draft", TitleCandidates: append([]string{}, r.TitleCandidates...), Body: r.Body,
		Topics: append([]string{}, r.Topics...), CoverCopy: r.CoverCopy, ImagePlan: append([]ImagePlanItem{}, r.ImagePlan...), CreatedAt: now}
}

// CreativeVersion is the immutable Creative-owned snapshot that downstream
// systems may reference. A draft remains editable; a CreativeVersion never is.
type CreativeVersionStatus string

const (
	CreativeVersionCreated    CreativeVersionStatus = "created"
	CreativeVersionChecked    CreativeVersionStatus = "checked"
	CreativeVersionApproved   CreativeVersionStatus = "approved"
	CreativeVersionSuperseded CreativeVersionStatus = "superseded"
)

type FreezeVersionRequest struct {
	DraftVersion int64  `json:"draft_version"`
	RenderJobID  string `json:"render_job_id,omitempty"`
}

func (r FreezeVersionRequest) Validate() error {
	if r.DraftVersion < 1 {
		return fmt.Errorf("draft_version must be positive")
	}
	return nil
}

type CreativeVersion struct {
	ID             string                  `json:"id"`
	OrganizationID contract.OrganizationID `json:"organization_id"`
	ProjectID      contract.ProjectID      `json:"project_id"`
	TaskID         string                  `json:"creative_task_id,omitempty"`
	EditTaskID     string                  `json:"edit_task_id,omitempty"`
	Format         CreativeFormat          `json:"format"`
	Version        int64                   `json:"version"`
	DraftVersion   int64                   `json:"draft_version"`
	Status         CreativeVersionStatus   `json:"status"`
	Snapshot       ImageTextDraft          `json:"snapshot"`
	VideoSnapshot  *VideoVersionSnapshot   `json:"video_snapshot,omitempty"`
	ContentHash    contract.ContentHash    `json:"content_hash"`
	CreatedBy      string                  `json:"created_by"`
	CreatedAt      time.Time               `json:"created_at"`
	Check          *CreativeCheck          `json:"check,omitempty"`
	Approval       *CreativeApproval       `json:"approval,omitempty"`
	IdempotencyKey contract.IdempotencyKey `json:"-"`
	RequestHash    string                  `json:"-"`
}

type VideoVersionSnapshot struct {
	ContractVersion   string                            `json:"contract_version"`
	Format            CreativeFormat                    `json:"format"`
	Channel           CreativeChannel                   `json:"channel"`
	VideoPurpose      string                            `json:"video_purpose"`
	PerformanceMode   string                            `json:"performance_mode"`
	StrategyPackage   *StrategyPackageReference         `json:"strategy_package_ref,omitempty"`
	DraftRevision     int64                             `json:"draft_revision"`
	SourceVideo       contract.AssetVersionRef          `json:"source_video"`
	GeneratedPreRoll  contract.AssetVersionRef          `json:"generated_preroll"`
	FinalVideo        contract.AssetVersionRef          `json:"final_video"`
	ProviderJobID     string                            `json:"provider_job_id"`
	RenderJobID       string                            `json:"render_job_id"`
	BrandFilm         *BrandFilmVersionSnapshot         `json:"brand_film,omitempty"`
	Editing           *EditingVersionSnapshot           `json:"editing,omitempty"`
	CommercePrerollV2 *CommercePrerollV2VersionSnapshot `json:"commerce_preroll_v2,omitempty"`
}

type CommercePrerollV2VersionSnapshot struct {
	ContractVersion string                     `json:"contract_version"`
	Workspace       CommercePrerollV2Workspace `json:"workspace"`
}

// EditingVersionSnapshot is the immutable bridge from the generic editor into
// Creative's existing check, review and delivery lifecycle.
type EditingVersionSnapshot struct {
	ContractVersion     string                     `json:"contract_version"`
	EditTaskID          string                     `json:"edit_task_id"`
	TimelineVersion     int64                      `json:"timeline_version"`
	TimelineSchema      string                     `json:"timeline_schema"`
	TimelineHash        string                     `json:"timeline_hash"`
	CompilerVersion     string                     `json:"compiler_version"`
	RendererFingerprint string                     `json:"renderer_fingerprint"`
	RenderJobID         string                     `json:"render_job_id"`
	OutputAsset         contract.AssetVersionRef   `json:"output_asset"`
	InputAssets         []contract.AssetVersionRef `json:"input_assets"`
	Width               int                        `json:"width"`
	Height              int                        `json:"height"`
	FrameRate           int                        `json:"frame_rate"`
	SampleRate          int                        `json:"sample_rate"`
	DurationMS          int                        `json:"duration_ms"`
	VideoCodec          string                     `json:"video_codec"`
	AudioCodec          string                     `json:"audio_codec"`
	TargetLUFS          float64                    `json:"target_lufs"`
}

func (v EditingVersionSnapshot) Validate() error {
	if v.ContractVersion != "creative-editing-version/v1" || strings.TrimSpace(v.EditTaskID) == "" || v.TimelineVersion < 1 ||
		(v.TimelineSchema != EditingTimelineSchemaV1 && v.TimelineSchema != EditingTimelineSchemaV2) || !strings.HasPrefix(v.TimelineHash, "sha256:") ||
		strings.TrimSpace(v.CompilerVersion) == "" || !strings.HasPrefix(v.RendererFingerprint, "sha256:") || strings.TrimSpace(v.RenderJobID) == "" ||
		v.OutputAsset.Validate() != nil || len(v.InputAssets) == 0 || v.Width < 1 || v.Height < 1 || v.FrameRate != 30 || v.SampleRate != 48000 ||
		v.DurationMS < 1 || v.VideoCodec != "h264" || v.AudioCodec != "aac" || v.TargetLUFS != -16 {
		return fmt.Errorf("creative editing version snapshot is incomplete")
	}
	for _, ref := range v.InputAssets {
		if ref.Validate() != nil {
			return fmt.Errorf("creative editing input asset is invalid")
		}
	}
	return nil
}

type BrandFilmVersionSnapshot struct {
	ContractVersion string                    `json:"contract_version"`
	PlanRevision    int64                     `json:"plan_revision"`
	QualityRunID    string                    `json:"quality_run_id"`
	Lineage         *BrandFilmLineageSnapshot `json:"lineage,omitempty"`
	ReferenceAsset  contract.AssetVersionRef  `json:"reference_asset"`
	FinalVideo      contract.AssetVersionRef  `json:"final_video"`
	UnitCount       int                       `json:"unit_count"`
	AttemptCount    int                       `json:"attempt_count"`
	ConfirmedBy     string                    `json:"confirmed_by"`
	ConfirmedAt     time.Time                 `json:"confirmed_at"`
}

// BrandFilmLineageSnapshot keeps the immutable cross-system proof required to
// trace a delivered brand film back to the exact Strategy handoff, confirmed
// Brand Brief, and CreativeDirection that authored it. It deliberately omits
// the duplicated Brief body stored in the working draft.
type BrandFilmLineageSnapshot struct {
	SourceType             string `json:"source_type"`
	IntakeID               string `json:"intake_id"`
	InputIdentityHash      string `json:"input_identity_hash"`
	StrategyPackageID      string `json:"strategy_package_id"`
	StrategyPackageVersion int64  `json:"strategy_package_version"`
	StrategyPackageHash    string `json:"strategy_package_hash"`
	HandoffContractVersion string `json:"handoff_contract_version"`
	HandoffContentHash     string `json:"handoff_content_hash"`
	BrandBriefRevision     int64  `json:"brand_brief_revision"`
	BrandBriefContentHash  string `json:"brand_brief_content_hash"`
	DirectionBatchID       string `json:"direction_batch_id"`
	DirectionID            string `json:"direction_id"`
	DirectionVersion       int64  `json:"direction_version"`
	DirectionContentHash   string `json:"direction_content_hash"`
	RouteID                string `json:"route_id"`
}

func (v BrandFilmLineageSnapshot) Validate() error {
	if v.SourceType != strategyBrandFilmSourceType || strings.TrimSpace(v.IntakeID) == "" ||
		!validSHA256Ref(v.InputIdentityHash) || strings.TrimSpace(v.StrategyPackageID) == "" ||
		v.StrategyPackageVersion < 1 || !validSHA256Ref(v.StrategyPackageHash) ||
		strings.TrimSpace(v.HandoffContractVersion) == "" || !validSHA256Ref(v.HandoffContentHash) ||
		v.BrandBriefRevision < 1 || !validSHA256Ref(v.BrandBriefContentHash) ||
		strings.TrimSpace(v.DirectionBatchID) == "" || strings.TrimSpace(v.DirectionID) == "" ||
		v.DirectionVersion < 1 || !validSHA256Ref(v.DirectionContentHash) || strings.TrimSpace(v.RouteID) == "" {
		return fmt.Errorf("creative brand film lineage snapshot is incomplete")
	}
	return nil
}

func (v BrandFilmVersionSnapshot) Validate() error {
	if v.ContractVersion != "creative-brand-film-version/v1" || v.PlanRevision < 1 ||
		strings.TrimSpace(v.QualityRunID) == "" || v.ReferenceAsset.Validate() != nil || v.FinalVideo.Validate() != nil ||
		v.UnitCount < 1 || v.AttemptCount < v.UnitCount || strings.TrimSpace(v.ConfirmedBy) == "" || v.ConfirmedAt.IsZero() {
		return fmt.Errorf("creative brand film version snapshot is incomplete")
	}
	if v.Lineage != nil && v.Lineage.Validate() != nil {
		return fmt.Errorf("creative brand film version lineage is incomplete")
	}
	return nil
}

func (v VideoVersionSnapshot) Validate() error {
	if v.Editing != nil {
		if v.ContractVersion != "creative-video-version/v1" || v.Format != FormatVideo || v.VideoPurpose != "editing" || v.PerformanceMode != "material_edit" ||
			v.DraftRevision != v.Editing.TimelineVersion || v.FinalVideo != v.Editing.OutputAsset || strings.TrimSpace(v.RenderJobID) == "" || v.RenderJobID != v.Editing.RenderJobID ||
			v.BrandFilm != nil || v.StrategyPackage != nil || v.Editing.Validate() != nil {
			return fmt.Errorf("creative editing video version snapshot is incomplete")
		}
		return nil
	}
	if v.CommercePrerollV2 != nil {
		if v.ContractVersion != "creative-video-version/v1" || v.Format != FormatVideo || v.VideoPurpose != "performance" || v.PerformanceMode != PerformanceModeCommercePreroll || v.DraftRevision != v.CommercePrerollV2.Workspace.Revision || v.CommercePrerollV2.ContractVersion != "creative-commerce-preroll-version/v1" || v.CommercePrerollV2.Workspace.Validate() != nil || v.BrandFilm != nil || v.Editing != nil {
			return fmt.Errorf("creative commerce preroll version snapshot is incomplete")
		}
		return nil
	}
	if v.ContractVersion != "creative-video-version/v1" || v.Format != FormatVideo ||
		(v.Channel != ChannelDouyin && v.Channel != ChannelKuaishou) ||
		v.DraftRevision < 1 {
		return fmt.Errorf("creative video version snapshot is incomplete")
	}
	if v.PerformanceMode == PerformanceModeBrandFilm {
		if v.VideoPurpose != "brand" || v.BrandFilm == nil || v.BrandFilm.Validate() != nil || v.FinalVideo != v.BrandFilm.FinalVideo ||
			v.StrategyPackage != nil || strings.TrimSpace(v.ProviderJobID) != "" || strings.TrimSpace(v.RenderJobID) != "" {
			return fmt.Errorf("creative brand film version snapshot is incomplete")
		}
		return nil
	}
	if v.VideoPurpose != "performance" || v.PerformanceMode != "pre_roll" || v.BrandFilm != nil ||
		v.SourceVideo.Validate() != nil || v.GeneratedPreRoll.Validate() != nil || v.FinalVideo.Validate() != nil ||
		strings.TrimSpace(v.ProviderJobID) == "" || strings.TrimSpace(v.RenderJobID) == "" {
		return fmt.Errorf("creative video version snapshot is incomplete")
	}
	if v.StrategyPackage == nil || v.StrategyPackage.Validate() != nil {
		return fmt.Errorf("creative video version requires immutable Strategy package lineage")
	}
	return nil
}

// CreativeCheck is an auditable, deterministic Phase-1 gate. It records why
// a frozen snapshot cannot proceed rather than silently changing its content.
type CreativeCheck struct {
	Passed    bool      `json:"passed"`
	Blockers  []string  `json:"blockers"`
	Warnings  []string  `json:"warnings"`
	CheckedBy string    `json:"checked_by"`
	CheckedAt time.Time `json:"checked_at"`
}

type CreativeApproval struct {
	ApprovedBy string    `json:"approved_by"`
	ApprovedAt time.Time `json:"approved_at"`
}

// CreativePackage is the stable output consumed by Delivery and Insights. It
// references one approved immutable CreativeVersion and never a mutable task.
type CreativePackage struct {
	ID                string                  `json:"id"`
	OrganizationID    contract.OrganizationID `json:"organization_id"`
	ProjectID         contract.ProjectID      `json:"project_id"`
	CreativeVersionID string                  `json:"creative_version_id"`
	EditTaskID        string                  `json:"edit_task_id,omitempty"`
	Format            CreativeFormat          `json:"format"`
	ContentHash       contract.ContentHash    `json:"content_hash"`
	Snapshot          ImageTextDraft          `json:"snapshot"`
	VideoSnapshot     *VideoVersionSnapshot   `json:"video_snapshot,omitempty"`
	CreatedBy         string                  `json:"created_by"`
	CreatedAt         time.Time               `json:"created_at"`
}

func (v CreativeVersion) Validate() error {
	if strings.TrimSpace(v.ID) == "" || (strings.TrimSpace(v.TaskID) == "") == (strings.TrimSpace(v.EditTaskID) == "") || v.Version < 1 || v.DraftVersion < 1 ||
		v.OrganizationID == "" || v.ProjectID == "" || v.ContentHash.Validate() != nil || v.CreatedAt.IsZero() {
		return fmt.Errorf("creative version is incomplete")
	}
	if !validCreativeVersionStatus(v.Status) {
		return fmt.Errorf("creative version status is invalid")
	}
	switch v.Format {
	case "", FormatImageText:
		if v.Snapshot.TaskID != v.TaskID || v.Snapshot.Version != v.DraftVersion || v.VideoSnapshot != nil {
			return fmt.Errorf("creative version snapshot does not match its image-text draft reference")
		}
	case FormatVideo:
		if v.VideoSnapshot == nil || v.VideoSnapshot.DraftRevision != v.DraftVersion || v.VideoSnapshot.Validate() != nil {
			return fmt.Errorf("creative version snapshot does not match its video draft reference")
		}
		if v.EditTaskID != "" && (v.VideoSnapshot.Editing == nil || v.VideoSnapshot.Editing.EditTaskID != v.EditTaskID) {
			return fmt.Errorf("creative editing version identity is invalid")
		}
	default:
		return fmt.Errorf("creative version format is invalid")
	}
	return nil
}

type ImagePlanItem struct {
	Order        int                       `json:"order"`
	Role         string                    `json:"role,omitempty"`
	Purpose      string                    `json:"purpose"`
	VisualBrief  string                    `json:"visual_brief"`
	Caption      string                    `json:"caption"`
	OverlayCopy  string                    `json:"overlay_copy,omitempty"`
	LayoutPreset string                    `json:"layout_preset,omitempty"`
	AssetRef     *contract.AssetVersionRef `json:"asset_ref,omitempty"`
}

type ImageTextDirectionRef struct {
	DirectionID string `json:"direction_id"`
	ContentHash string `json:"content_hash"`
}

type ProductionJob struct {
	TaskID        string    `json:"task_id"`
	Kind          string    `json:"kind"`
	ProviderJobID string    `json:"provider_job_id"`
	CreatedAt     time.Time `json:"created_at"`
}

type ShortDramaGenerationAttempt struct {
	ID                 string                    `json:"id"`
	TaskID             string                    `json:"task_id"`
	DraftRevision      int64                     `json:"draft_revision"`
	CandidateBatchID   string                    `json:"candidate_batch_id"`
	CandidateID        string                    `json:"candidate_id"`
	PromptPackageHash  string                    `json:"prompt_package_hash"`
	GenerationSpecHash string                    `json:"generation_spec_hash"`
	ProviderJobID      string                    `json:"provider_job_id"`
	OutputAssetVersion *contract.AssetVersionRef `json:"output_asset_version,omitempty"`
	CreatedAt          time.Time                 `json:"created_at"`
}

type GamePrerollGenerationAttempt struct {
	ID                 string                    `json:"id"`
	TaskID             string                    `json:"task_id"`
	DraftRevision      int64                     `json:"draft_revision"`
	CandidateBatchID   string                    `json:"candidate_batch_id"`
	CandidateID        string                    `json:"candidate_id"`
	PromptPackageHash  string                    `json:"prompt_package_hash"`
	GenerationSpecHash string                    `json:"generation_spec_hash"`
	ProviderJobID      string                    `json:"provider_job_id"`
	OutputAssetVersion *contract.AssetVersionRef `json:"output_asset_version,omitempty"`
	CreatedAt          time.Time                 `json:"created_at"`
}

type TaskDetail struct {
	Task                          CreativeTask                   `json:"task"`
	Intake                        CreativeIntake                 `json:"intake"`
	AINativeWorkspaceID           string                         `json:"ai_native_workspace_id,omitempty"`
	Draft                         ImageTextDraft                 `json:"draft"`
	VideoDraft                    *VideoDraft                    `json:"video_draft,omitempty"`
	ProductionJobs                []ProductionJob                `json:"production_jobs"`
	ShortDramaGenerationAttempts  []ShortDramaGenerationAttempt  `json:"short_drama_generation_attempts,omitempty"`
	GamePrerollGenerationAttempts []GamePrerollGenerationAttempt `json:"game_preroll_generation_attempts,omitempty"`
	CommerceGenerationAttempts    []CommerceGenerationAttempt    `json:"commerce_preroll_generation_attempts,omitempty"`
}

func validateStringList(name string, values []string, maxItems, maxLength int) error {
	if values == nil {
		return fmt.Errorf("%s must be an array", name)
	}
	if len(values) > maxItems {
		return fmt.Errorf("%s has too many values", name)
	}
	for _, value := range values {
		if len(strings.TrimSpace(value)) == 0 || len(value) > maxLength {
			return fmt.Errorf("%s contains an invalid value", name)
		}
	}
	return nil
}
