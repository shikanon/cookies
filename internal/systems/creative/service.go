package creative

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	platformassets "github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/ids"
	"github.com/shikanon/cookies/internal/platform/media"
	"github.com/shikanon/cookies/internal/platform/provider"
)

type ActiveProjectResolver interface {
	RequireActiveContext(context.Context, contract.ActorContext, contract.ProjectID) (contract.ProjectContext, error)
}

// StrategyPackageReader is Creative's sole dependency on Strategy. Its
// implementation is composed at process startup and must return an immutable,
// authorization-checked package snapshot rather than exposing Strategy tables.
type StrategyPackageReader interface {
	ReadForCreative(context.Context, contract.ActorContext, contract.ProjectID, StrategyPackageReference) (StrategyPackageSnapshot, error)
}

// TaskStrategyReader is the explicit boundary for consuming a frozen
// CreativeTaskStrategyVersion. Implementations must authorize the read and
// verify project and content hash before returning a Creative-owned snapshot.
type TaskStrategyReader interface {
	ReadTaskStrategyForCreative(context.Context, contract.ActorContext, contract.ProjectID, TaskStrategyReference) (TaskStrategySnapshot, error)
}

type TaskOverlayReader interface {
	ReadTaskOverlayForCreative(context.Context, contract.ActorContext, contract.ProjectID, TaskOverlayReference) (TaskOverlaySnapshot, error)
}

type RequirementSnapshotReader interface {
	ReadRequirementForCreative(context.Context, contract.ActorContext, contract.ProjectID, RequirementSnapshotReference, BusinessCapabilityReference) (RequirementSnapshot, error)
}

type AssetReader interface {
	ReadForCreative(context.Context, contract.ActorContext, contract.ProjectID, contract.AssetVersionRef) (CreativeAssetSnapshot, error)
}

type DerivedImageWriter interface {
	IngestDerivedImage(context.Context, contract.RequestContext, contract.ProjectID, string, contract.AssetVersionRef, io.Reader, int64, string) (contract.ProjectAssetRef, error)
}

type AudioAssetWriter interface {
	IngestDerivedAudio(context.Context, contract.RequestContext, contract.ProjectID, string, io.Reader, int64, string, []contract.ResourceRef) (contract.ProjectAssetRef, error)
}

type CreativeAssetSnapshot struct {
	Ref          contract.AssetVersionRef
	Kind         contract.AssetKind
	MIMEType     string
	Ready        bool
	WidthPixels  int
	HeightPixels int
	DurationMS   int64
	FrameRate    string
	VideoCodec   string
	AudioCodec   string
}

type StrategyPackageSnapshot struct {
	PackageID              string
	PackageVersion         int64
	ContentHash            string
	HandoffContractVersion string
	HandoffContentHash     string
	CreativeReady          bool
	Objective              string
	Audience               string
	CoreMessage            string
	CallToAction           string
	BrandName              string
	ProductName            string
	SellingPoints          []string
	ProofPoints            []string
	UsageScenarios         []string
	// Deprecated compatibility fields. Strict Handoff readers leave these
	// empty because concepts and visual choices belong to CreativeDirection.
	Concept         string
	Tone            []string
	VisualKeywords  []string
	Mandatory       []string
	Prohibited      []string
	CreativeRoutes  []CreativeRouteSnapshot
	HandoffSnapshot json.RawMessage
}

type Service struct {
	Repository                          Repository
	ViralRemakes                        ViralRemakeRepository
	ViralAnalyzer                       ViralReferenceAnalyzer
	Projects                            ActiveProjectResolver
	StrategyPackages                    StrategyPackageReader
	TaskStrategies                      TaskStrategyReader
	TaskOverlays                        TaskOverlayReader
	Requirements                        RequirementSnapshotReader
	Sources                             CreativeSourceReader
	Assets                              AssetReader
	AssetUses                           platformassets.AssetUseAuthorizer
	GameEvidenceFrames                  media.FrameExtractor
	DerivedAssets                       DerivedImageWriter
	AudioAssets                         AudioAssetWriter
	AudioMixRenderer                    media.AudioMixRenderer
	AudioMixScheduler                   AudioMixRenderScheduler
	BrandFilmSpeech                     provider.SpeechSynthesizer
	Composer                            media.VideoComposer
	BrandFilmComposer                   media.SegmentComposer
	RenderedAssets                      RenderedAssetWriter
	RenderScheduler                     RenderScheduler
	ShortDramaPrerollPlanner            ShortDramaPrerollPlanner
	ShortDramaV2Analyzer                ShortDramaV2Analyzer
	CommercePrerollV2Analyzer           CommercePrerollV2Analyzer
	CommercePrerollV2Images             CommercePrerollV2ImageJobCreator
	ShortDramaV2Planner                 ShortDramaV2Planner
	ShortDramaV2Images                  ShortDramaV2ImageJobCreator
	ShortDramaV2OutputNormalizer        media.VideoNormalizer
	CommercePrerollV2OutputNormalizer   media.VideoNormalizer
	GamePrerollPlanner                  GamePrerollPlanner
	CommerceWorkspaces                  CommerceWorkspaceRepository
	BrandFilmPlanner                    BrandFilmPlanner
	BrandBriefs                         BrandBriefRepository
	DirectionPlanner                    CreativeDirectionPlanner
	Directions                          DirectionRepository
	DirectionScheduler                  DirectionGenerationScheduler
	ImageTextDraftPlanner               ImageTextDraftPlanner
	ImageRenderer                       *ImageTextRenderer
	ImageBaseAssets                     ImageBaseAssetReader
	RenderedImages                      RenderedImageWriter
	AINativeProducts                    AINativeProductResolver
	AINativeRequirementPlanner          AINativeRequirementPlanner
	AINativeRequirements                AINativeRequirementRepository
	AINativeProductMediaImporter        AINativeProductMediaImporter
	AINativeOperationCanceller          AINativeOperationCanceller
	AINativeScripts                     AINativeScriptRepository
	AINativeScriptPlanner               AINativeScriptPlanner
	AINativeScriptProfiles              ChannelCreativeProfileRegistry
	AINativeScriptScheduler             AINativeScriptScheduler
	AINativeStoryboards                 AINativeStoryboardRepository
	AINativeStoryboardPlanner           AINativeStoryboardPlanner
	AINativeVoiceoverFitter             AINativeVoiceoverFitter
	AINativeStoryboardAssetPreparer     AINativeStoryboardAssetPreparer
	AINativeStoryboardScheduler         AINativeStoryboardScheduler
	AINativeProductions                 AINativeProductionRepository
	AINativeProductionScheduler         AINativeProductionScheduler
	AINativeVideoJobs                   AINativeVideoJobManager
	AINativeSpeech                      provider.SpeechSynthesizer
	AINativeTimelineRenderer            media.TimelineRenderer
	EditTasks                           EditTaskRepository
	EditingRenders                      EditingRenderRepository
	EditingRenderScheduler              EditingRenderScheduler
	AINativeMaxActiveUnits              int
	AllowLegacyTaskStrategyIntakeWrites bool
	NewID                               ids.Generator
	Now                                 func() time.Time
}

func (s Service) CreateVideoTask(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, intakeID string, request CreateVideoTaskRequest) (CreativeTask, error) {
	if s.Repository == nil || s.Projects == nil {
		return CreativeTask{}, fmt.Errorf("creative video dependencies are incomplete")
	}
	if !actor.HasScope(ScopeWrite) {
		return CreativeTask{}, fmt.Errorf("%s scope is required", ScopeWrite)
	}
	if err := request.Validate(); err != nil {
		return CreativeTask{}, err
	}
	projectContext, err := s.Projects.RequireActiveContext(ctx, actor, projectID)
	if err != nil {
		return CreativeTask{}, err
	}
	intake, err := s.Repository.GetIntake(ctx, actor.OrganizationID, projectID, intakeID)
	if err != nil {
		return CreativeTask{}, err
	}
	if intake.Status != IntakeReady {
		return CreativeTask{}, ErrIntakeNotReady
	}
	route, err := selectedVideoRoute(intake, request)
	if err != nil {
		return CreativeTask{}, err
	}
	if err := route.Validate(); err != nil {
		return CreativeTask{}, err
	}
	channelAllowed := false
	for _, channel := range route.Channels {
		if channel == string(request.Channel) {
			channelAllowed = true
			break
		}
	}
	if !channelAllowed {
		return CreativeTask{}, fmt.Errorf("selected channel is not approved by the Strategy route")
	}
	isBrandFilm := route.RouteType == CreativeRouteBrandVideo || route.RouteType == PerformanceModeBrandFilm
	isManualBrandFilm := intake.Source == IntakeSourceManual && route.RouteID == ManualBrandFilmRouteID
	if isBrandFilm && strings.TrimSpace(request.DirectionID) == "" {
		if existing, existingErr := s.taskForIntake(ctx, actor, projectID, intake.ID); existingErr == nil {
			return existing.Task, nil
		} else if existingErr != ErrNotFound {
			return CreativeTask{}, existingErr
		}
	}
	isDirectViralRemake := (intake.Source == IntakeSourceManual || intake.Source == IntakeSourceRequirement) && route.RouteType == PerformanceModeViralRemake
	var confirmedDirection *CreativeDirectionVersion
	var confirmedDirectionBatch *CreativeDirectionBatch
	var confirmedBrief *BrandBriefReview
	if isBrandFilm && strings.TrimSpace(request.DirectionID) != "" {
		if s.Directions == nil {
			return CreativeTask{}, fmt.Errorf("creative direction repository is required")
		}
		direction, directionErr := s.Directions.GetDirection(ctx, actor.OrganizationID, projectID, request.DirectionID)
		if directionErr != nil {
			return CreativeTask{}, directionErr
		}
		if direction.Status != DirectionStatusConfirmed || direction.IntakeID != intake.ID ||
			direction.InputIdentityHash != intake.InputIdentityHash || direction.RouteID != route.RouteID {
			return CreativeTask{}, fmt.Errorf("brand-video direction does not match the confirmed intake lineage")
		}
		if !isManualBrandFilm {
			if s.BrandBriefs == nil {
				return CreativeTask{}, fmt.Errorf("confirmed brand Brief lineage is required for a brand-video task")
			}
			brief, briefErr := s.BrandBriefs.GetBrandBrief(ctx, actor.OrganizationID, projectID, intake.ID)
			if briefErr != nil || brief.Status != BrandBriefConfirmed || brief.InputIdentityHash != intake.InputIdentityHash ||
				!brandBriefReferencesEqual(direction.BrandBriefRef, &BrandBriefReference{Revision: brief.Revision, ContentHash: brief.ContentHash}) {
				return CreativeTask{}, fmt.Errorf("brand-video direction does not match the confirmed brand Brief lineage")
			}
			batch, batchErr := s.Directions.GetDirectionBatch(ctx, actor.OrganizationID, projectID, direction.BatchID)
			if batchErr != nil || batch.Status != DirectionBatchReady || batch.IntakeID != intake.ID ||
				batch.InputIdentityHash != intake.InputIdentityHash ||
				!brandBriefReferencesEqual(batch.BrandBriefRef, &BrandBriefReference{Revision: brief.Revision, ContentHash: brief.ContentHash}) {
				return CreativeTask{}, fmt.Errorf("brand-video direction batch does not match the confirmed brand Brief lineage")
			}
			confirmedDirectionBatch = &batch
			confirmedBrief = &brief
		}
		if !isManualBrandFilm && strings.TrimSpace(request.CallToAction) != "" {
			return CreativeTask{}, fmt.Errorf("brand-video task cannot introduce a performance CTA")
		}
		confirmedDirection = &direction
		request.Concept = direction.Concept
		request.Prompt = brandVideoOutlineFromDirection(direction)
	} else if isBrandFilm {
		request.Concept = strings.TrimSpace(intake.Request.Concept)
		if request.Concept == "" {
			request.Concept = strings.TrimSpace(intake.Request.CoreMessage)
		}
		if request.Concept == "" {
			request.Concept = "等待 Brief 确认的品牌广告"
		}
		request.Prompt = "等待 Brief 确认后生成创意方向、剧本与分镜"
		if !isManualBrandFilm && strings.TrimSpace(request.CallToAction) != "" {
			return CreativeTask{}, fmt.Errorf("brand-video task cannot introduce a performance CTA")
		}
	} else if strings.TrimSpace(request.Concept) == "" || strings.TrimSpace(request.Prompt) == "" {
		return CreativeTask{}, fmt.Errorf("video concept and prompt are required")
	}
	hasSourceVideo := strings.TrimSpace(string(request.SourceVideo.AssetID)) != "" || request.SourceVideo.Version != 0
	isShortDramaV2 := intake.Source == IntakeSourceManual && route.RouteID == ManualShortDramaPrerollV2RouteID
	isCommerceV2 := intake.Source == IntakeSourceManual && route.RouteID == ManualCommercePrerollV2RouteID
	needsSourceVideo := (route.RouteType != PerformanceModeShortDramaPreroll || isShortDramaV2) &&
		!isManualBrandFilm &&
		(route.RouteType != CreativeRouteBrandVideo || hasSourceVideo)
	if needsSourceVideo {
		if err := request.SourceVideo.Validate(); err != nil {
			return CreativeTask{}, fmt.Errorf("source_video: %w", err)
		}
		if s.Assets == nil {
			return CreativeTask{}, fmt.Errorf("creative video dependencies are incomplete")
		}
		source, readErr := s.Assets.ReadForCreative(ctx, actor, projectID, request.SourceVideo)
		if readErr != nil {
			return CreativeTask{}, readErr
		}
		if !source.Ready || source.Kind != contract.AssetVideo || source.MIMEType != "video/mp4" || source.Ref != request.SourceVideo {
			return CreativeTask{}, fmt.Errorf("source_video must be a ready MP4 in the same project")
		}
	}
	var viralDraft *ViralRemakeDraft
	var shortDramaDraft *ShortDramaPrerollDraft
	var shortDramaDraftV2 *ShortDramaPrerollV2Workspace
	var gamePrerollDraft *GamePrerollDraft
	var brandFilmDraft *BrandFilmDraft
	if isDirectViralRemake {
		if intake.Request.ManualViralRemake == nil || intake.Request.ManualViralRemake.ReferenceVideo != request.SourceVideo {
			return CreativeTask{}, fmt.Errorf("source_video must match the immutable manual intake snapshot")
		}
		if referenceImage := intake.Request.ManualViralRemake.ReferenceImage; referenceImage != nil {
			image, readErr := s.Assets.ReadForCreative(ctx, actor, projectID, *referenceImage)
			if readErr != nil {
				return CreativeTask{}, readErr
			}
			if !image.Ready || image.Kind != contract.AssetImage || image.Ref != *referenceImage {
				return CreativeTask{}, fmt.Errorf("reference_image must be a ready image in the same project")
			}
		}
	}
	if intake.Source == IntakeSourceManual && route.RouteType == PerformanceModeGamePreroll {
		if intake.Request.ManualGamePreroll == nil ||
			intake.Request.ManualGamePreroll.SourceVideo != request.SourceVideo {
			return CreativeTask{}, fmt.Errorf("source_video must match the immutable game preroll intake snapshot")
		}
	}
	if isShortDramaV2 {
		if intake.Request.ManualShortDramaPrerollV2 == nil ||
			intake.Request.ManualShortDramaPrerollV2.SourceVideo != request.SourceVideo {
			return CreativeTask{}, fmt.Errorf("source_video must match the immutable short drama V2 intake snapshot")
		}
	}
	if isCommerceV2 {
		if intake.Request.ManualCommercePrerollV2 == nil ||
			intake.Request.ManualCommercePrerollV2.SourceVideo != request.SourceVideo {
			return CreativeTask{}, fmt.Errorf("source_video must match the immutable commerce preroll V2 intake snapshot")
		}
	}
	lineageKey := ""
	if confirmedDirection != nil {
		lineageKey, err = creativeTaskLineageKey(intake, route, request.Channel, *confirmedDirection)
		if err != nil {
			return CreativeTask{}, err
		}
		existing, listErr := s.Repository.ListTasks(ctx, actor.OrganizationID, projectID, 100)
		if listErr != nil {
			return CreativeTask{}, listErr
		}
		for _, candidate := range existing {
			if candidate.LineageKey == lineageKey {
				return candidate, nil
			}
		}
	}
	id, err := s.idGenerator()("creativetask")
	if err != nil {
		return CreativeTask{}, err
	}
	now := s.now()
	displayName := strings.TrimSpace(request.Concept)
	if intake.Request.ManualBrandFilm != nil && strings.TrimSpace(intake.Request.ManualBrandFilm.ProductName) != "" {
		displayName = strings.TrimSpace(intake.Request.ManualBrandFilm.ProductName)
	}
	if displayName == "" {
		displayName = "未命名视频创作"
	}
	task := CreativeTask{
		ID: id, DisplayName: displayName, OrganizationID: actor.OrganizationID, ProjectID: projectID, IntakeID: intake.ID,
		Format: FormatVideo, Channel: request.Channel, VideoPurpose: route.VideoPurpose, PerformanceMode: route.RouteType,
		LineageKey: lineageKey,
		Status:     TaskDraft, Direction: CreativeDirection{
			Focus: request.Concept, Audience: intake.Request.Audience, CoreMessage: intake.Request.CoreMessage,
			CallToAction: request.CallToAction, Concept: request.Concept,
			Tone: append([]string{}, intake.Request.Tone...), VisualKeywords: append([]string{}, intake.Request.VisualKeywords...),
		},
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if confirmedDirection != nil {
		task.Direction.DirectionVersionID = confirmedDirection.ID
		task.Direction.InputIdentityHash = confirmedDirection.InputIdentityHash
	} else if isBrandFilm {
		task.Direction.InputIdentityHash = intake.InputIdentityHash
	}
	resolution := route.Resolution
	if strings.TrimSpace(resolution) == "" {
		resolution = "720p"
	}
	draft := VideoDraft{
		ContractVersion: "creative-video-draft/v1", TaskID: task.ID, Revision: 1,
		Concept: request.Concept, Prompt: request.Prompt, DurationSeconds: route.TargetDurationSeconds,
		AspectRatio: route.AspectRatio, Resolution: resolution, VideoPurpose: route.VideoPurpose, SourceVideo: request.SourceVideo,
		Mandatory: append([]string{}, request.Mandatory...), Prohibited: append([]string{}, request.Prohibited...),
		CallToAction: request.CallToAction, CreatedAt: now,
	}
	if confirmedDirection != nil && confirmedBrief != nil && confirmedDirectionBatch != nil {
		brandFilmDraft, err = buildStrategyBrandFilmDraft(task.ID, intake, route, *confirmedBrief, *confirmedDirectionBatch, *confirmedDirection, now)
		if err != nil {
			return CreativeTask{}, err
		}
		draft.BrandFilm = brandFilmDraft
	} else if isBrandFilm {
		brandFilmDraft, err = newBrandFilmDraft(task, intake, route, now)
		if err != nil {
			return CreativeTask{}, err
		}
		draft.BrandFilm = brandFilmDraft
	}
	if isDirectViralRemake {
		manual := intake.Request.ManualViralRemake
		snapshot := ViralRemakeInputSnapshot{
			Source: intake.Source, SelectedRouteID: route.RouteID,
			ReferenceVideo: request.SourceVideo, ReferenceImage: manual.ReferenceImage,
			ProductName: manual.ProductName, SellingPoints: append([]string{}, manual.SellingPoints...),
			CallToAction: request.CallToAction, UserInstruction: manual.UserInstruction,
			MandatoryElements:    append([]string{}, request.Mandatory...),
			ProhibitedClaims:     append([]string{}, request.Prohibited...),
			ReferenceVideoRights: manual.ReferenceVideoRights, ReferenceImageRights: manual.ReferenceImageRights,
		}
		inputHash, hashErr := contract.CanonicalJSONHash(snapshot)
		if hashErr != nil {
			return CreativeTask{}, fmt.Errorf("canonicalize viral remake input: %w", hashErr)
		}
		blockers := []string{"analysis_snapshot", "confirmed_prompt_package", "provider_video_route"}
		if manual.ReferenceVideoRights != RightsConfirmed {
			blockers = append(blockers, "reference_video_rights")
		}
		if manual.ReferenceImage != nil && manual.ReferenceImageRights != RightsConfirmed {
			blockers = append(blockers, "reference_image_rights")
		}
		viralDraft = &ViralRemakeDraft{
			ContractVersion: "creative-viral-remake-draft/v1", TaskID: task.ID, Revision: 1,
			Status: ViralWaitingForAnalysis, SelectedRouteID: route.RouteID,
			InputSnapshot: snapshot, InputHash: inputHash,
			Readiness: CreativeReadiness{
				PlanningReady: true, GenerationReady: false, ProductionReady: false,
				MissingFields: []string{}, Blockers: blockers,
			},
			Candidates: []ViralCandidate{}, CreatedAt: now, UpdatedAt: now,
		}
		draft.ViralRemake = viralDraft
	}
	if intake.Source == IntakeSourceManual && route.RouteType == PerformanceModeShortDramaPreroll && !isShortDramaV2 {
		manual := intake.Request.ManualShortDramaPreroll
		if manual == nil {
			return CreativeTask{}, fmt.Errorf("manual short drama preroll input is required")
		}
		snapshot := ShortDramaPrerollInputSnapshot{
			Source: intake.Source, SelectedRouteID: route.RouteID,
			BriefID: manual.BriefID, BriefVersion: manual.BriefVersion, BriefName: manual.BriefName,
			StoryTitle: manual.StoryTitle, Synopsis: manual.Synopsis,
			ReviewedSellingPoints: append([]string{}, manual.ReviewedSellingPoints...), OpeningLine: manual.OpeningLine,
			HookStrategy: manual.HookStrategy, SubtitleStyle: manual.SubtitleStyle, Transition: manual.Transition,
			HookStrength: manual.HookStrength, PaceProfile: normalizeShortDramaPaceProfile(manual.PaceProfile),
			CallToAction:        request.CallToAction,
			CharacterReferences: append([]contract.AssetVersionRef{}, manual.CharacterReferences...),
		}
		inputHash, hashErr := contract.CanonicalJSONHash(snapshot)
		if hashErr != nil {
			return CreativeTask{}, fmt.Errorf("canonicalize short drama input: %w", hashErr)
		}
		batchID := fmt.Sprintf("%s_batch_%d", task.ID, 1)
		planner := s.ShortDramaPrerollPlanner
		if planner == nil {
			planner = DeterministicShortDramaPrerollPlanner{}
		}
		batch, planErr := planner.Plan(
			ctx,
			actor,
			projectContext,
			snapshot,
			"sha256:"+inputHash,
			batchID,
			1,
			shortDramaGenerationConfig(snapshot),
			"balanced",
			now,
		)
		if planErr != nil {
			return CreativeTask{}, planErr
		}
		shortDramaDraft = &ShortDramaPrerollDraft{
			ContractVersion: "creative-short-drama-preroll-draft/v1", TaskID: task.ID, Revision: 1,
			SelectedRouteID: route.RouteID, InputSnapshot: snapshot, InputHash: "sha256:" + inputHash,
			Readiness: CreativeReadiness{
				PlanningReady: true, GenerationReady: false, ProductionReady: false,
				MissingFields: []string{}, Blockers: []string{"selected_candidate", "evidence_assets"},
			},
			ActiveCandidateBatch: &batch, Candidates: batch.Candidates, CreatedAt: now, UpdatedAt: now,
		}
		draft.ShortDramaPreroll = shortDramaDraft
		draft.Prompt = batch.Candidates[0].PromptPackage.CompiledPrompt
	}
	if isShortDramaV2 {
		source, readErr := s.Assets.ReadForCreative(ctx, actor, projectID, request.SourceVideo)
		if readErr != nil {
			return CreativeTask{}, readErr
		}
		sourceCanvas, modelCanvas, outputCanvas, canvasErr := deriveShortDramaCanvases(source)
		if canvasErr != nil {
			return CreativeTask{}, canvasErr
		}
		shortDramaDraftV2 = &ShortDramaPrerollV2Workspace{
			ContractVersion: ShortDramaPrerollV3ContractVersion, TaskID: task.ID, Revision: 1,
			ActiveStage:    ShortDramaV2StageSourceReady,
			SourceVideo:    contract.ProjectAssetRef{ProjectID: projectID, AssetVersion: request.SourceVideo},
			SourceMetadata: source,
			SourceCanvas:   &sourceCanvas,
			ModelCanvas:    &modelCanvas,
			OutputCanvas:   &outputCanvas,
			Analysis:       ShortDramaV2Analysis{ShortDramaV2AsyncResource: ShortDramaV2AsyncResource{Status: ShortDramaV2ResourceIdle}},
			CreatedAt:      now, UpdatedAt: now,
		}
		draft.ShortDramaPrerollV2 = shortDramaDraftV2
		draft.Prompt = "等待视频理解"
	}
	if isCommerceV2 {
		source, readErr := s.Assets.ReadForCreative(ctx, actor, projectID, request.SourceVideo)
		if readErr != nil {
			return CreativeTask{}, readErr
		}
		draft.CommercePrerollV2 = &CommercePrerollV2Workspace{
			ContractVersion: CommercePrerollV2ContractVersion,
			TaskID:          task.ID,
			Revision:        1,
			ActiveStage:     CommercePrerollV2StageSourceReady,
			SourceVideo: contract.ProjectAssetRef{
				ProjectID: projectID, AssetVersion: request.SourceVideo,
			},
			SourceMetadata: source,
			SourceVideoRights: RightsConfirmation{
				Status: RightsConfirmed, ConfirmedBy: actor.Principal.ID, ConfirmedAt: now,
			},
			Analysis: CommercePrerollV2Analysis{
				CommercePrerollV2AsyncResource: CommercePrerollV2AsyncResource{Status: CommercePrerollV2ResourceIdle},
			},
			CreatedAt: now,
			UpdatedAt: now,
		}
		draft.Prompt = "等待原视频理解"
	}
	if intake.Source == IntakeSourceManual && route.RouteType == PerformanceModeGamePreroll {
		manual := intake.Request.ManualGamePreroll
		if manual == nil {
			return CreativeTask{}, fmt.Errorf("manual game preroll input is required")
		}
		snapshot := GamePrerollInputSnapshot{
			Source: intake.Source, SelectedRouteID: route.RouteID,
			BriefID: manual.BriefID, BriefVersion: manual.BriefVersion, BriefName: manual.BriefName,
			GameName: manual.GameName, GameplaySummary: manual.GameplaySummary,
			SourceVideo: request.SourceVideo, SourceVideoRights: manual.SourceVideoRights,
			CallToAction:         request.CallToAction,
			EvidenceMoments:      append([]GameEvidenceMoment{}, manual.EvidenceMoments...),
			AllowedMechanisms:    append([]GameHookMechanism{}, manual.AllowedMechanisms...),
			ProhibitedMechanisms: append([]GameHookMechanism{}, manual.ProhibitedMechanisms...),
		}
		inputHash, hashErr := contract.CanonicalJSONHash(snapshot)
		if hashErr != nil {
			return CreativeTask{}, fmt.Errorf("canonicalize game preroll input: %w", hashErr)
		}
		planner := s.GamePrerollPlanner
		if planner == nil {
			planner = DeterministicGamePrerollPlanner{}
		}
		batch, planErr := planner.Plan(
			ctx,
			actor,
			projectContext,
			snapshot,
			"sha256:"+inputHash,
			fmt.Sprintf("%s_batch_%d", task.ID, 1),
			1,
			GamePrerollGenerationConfig{
				SubtitleStyle: manual.SubtitleStyle,
				HookStrength:  manual.HookStrength,
				PaceProfile:   manual.PaceProfile,
			},
			now,
		)
		if planErr != nil {
			return CreativeTask{}, planErr
		}
		gamePrerollDraft = &GamePrerollDraft{
			ContractVersion: "creative-game-preroll-draft/v1", TaskID: task.ID, Revision: 1,
			SelectedRouteID: route.RouteID, InputSnapshot: snapshot, InputHash: "sha256:" + inputHash,
			Readiness: CreativeReadiness{
				PlanningReady: true, GenerationReady: false, ProductionReady: false,
				MissingFields: []string{}, Blockers: []string{"selected_candidate"},
			},
			ActiveCandidateBatch: &batch, Candidates: batch.Candidates,
			CreatedAt: now, UpdatedAt: now,
		}
		draft.GamePreroll = gamePrerollDraft
		draft.Prompt = batch.Candidates[0].PromptPackage.CompiledPrompt
	}
	if err := draft.Validate(); err != nil {
		return CreativeTask{}, err
	}
	return s.Repository.CreateVideoTask(ctx, task, draft)
}

func brandVideoOutlineFromDirection(direction CreativeDirectionVersion) string {
	parts := []string{
		"品牌概念：" + direction.Concept,
		"创意依据：" + direction.CreativeRationale,
		"情绪转变：" + direction.EmotionalArc,
		"影像语法：" + direction.VisualGrammar,
		"品牌记忆装置：" + direction.BrandMemoryDevice,
		"人物瞬间：" + direction.HumanMoment,
		"信息顺序：" + strings.Join(direction.MessagePlan, "；"),
		"执行轮廓：" + strings.Join(direction.ExecutionOutline, "；"),
	}
	return strings.Join(parts, "\n")
}

func creativeTaskLineageKey(intake CreativeIntake, route CreativeRouteSnapshot, channel CreativeChannel, direction CreativeDirectionVersion) (string, error) {
	value := struct {
		IntakeID             string          `json:"intake_id"`
		InputIdentityHash    string          `json:"input_identity_hash"`
		RouteID              string          `json:"route_id"`
		Channel              CreativeChannel `json:"channel"`
		DirectionID          string          `json:"direction_id"`
		DirectionVersion     int64           `json:"direction_version"`
		DirectionContentHash string          `json:"direction_content_hash"`
	}{
		IntakeID: intake.ID, InputIdentityHash: intake.InputIdentityHash, RouteID: route.RouteID,
		Channel: channel, DirectionID: direction.ID, DirectionVersion: direction.Version,
		DirectionContentHash: direction.ContentHash,
	}
	hash, err := contract.CanonicalJSONHash(value)
	if err != nil {
		return "", fmt.Errorf("canonicalize creative task lineage: %w", err)
	}
	return hash, nil
}

func selectedVideoRoute(intake CreativeIntake, request CreateVideoTaskRequest) (CreativeRouteSnapshot, error) {
	if request.SelectedRouteID != "" {
		for _, route := range intake.Request.CreativeRoutes {
			if route.RouteID == request.SelectedRouteID {
				return route, nil
			}
		}
		return CreativeRouteSnapshot{}, fmt.Errorf("selected_route_id is not present in the Creative intake")
	}
	if intake.Source != IntakeSourceStrategyPackage || request.RouteIndex < 0 || request.RouteIndex >= len(intake.Request.CreativeRoutes) {
		return CreativeRouteSnapshot{}, ErrIntakeNotReady
	}
	return intake.Request.CreativeRoutes[request.RouteIndex], nil
}

func (s Service) CreateIntake(ctx context.Context, requestContext contract.RequestContext, projectID contract.ProjectID, key contract.IdempotencyKey, request CreateIntakeRequest) (CreativeIntake, error) {
	if s.Repository == nil || s.Projects == nil {
		return CreativeIntake{}, fmt.Errorf("creative dependencies are incomplete")
	}
	if err := requestContext.Validate(); err != nil {
		return CreativeIntake{}, err
	}
	if !requestContext.Actor.HasScope(ScopeWrite) {
		return CreativeIntake{}, fmt.Errorf("%s scope is required", ScopeWrite)
	}
	if err := key.Validate(); err != nil {
		return CreativeIntake{}, err
	}
	if request.ContractVersion == CreativeIntakeCreateV3ContractVersion {
		if request.StrategyPackageRef != nil {
			if request.StrategyPackage != nil {
				return CreativeIntake{}, fmt.Errorf("submit strategy_package_ref only once")
			}
			request.StrategyPackage = &StrategyPackageReference{
				PackageID:              request.StrategyPackageRef.PackageID,
				PackageVersion:         request.StrategyPackageRef.PackageVersion,
				ExpectedContentHash:    request.StrategyPackageRef.PackageContentHash,
				HandoffContractVersion: request.StrategyPackageRef.HandoffContractVersion,
				ExpectedHandoffHash:    request.StrategyPackageRef.HandoffContentHash,
			}
			request.StrategyPackageRef = nil
		}
		if request.TaskOverlayRef != nil {
			if request.TaskOverlay != nil {
				return CreativeIntake{}, fmt.Errorf("submit task_overlay_ref only once")
			}
			request.TaskOverlay = &TaskOverlayReference{
				OverlayID:           request.TaskOverlayRef.OverlayID,
				ExpectedContentHash: request.TaskOverlayRef.ContentHash,
			}
			request.TaskOverlayRef = nil
		}
	}
	if err := request.Validate(); err != nil {
		return CreativeIntake{}, err
	}
	if request.Source == IntakeSourceTaskStrategy && !s.AllowLegacyTaskStrategyIntakeWrites {
		return CreativeIntake{}, fmt.Errorf("legacy task_strategy intake creation is read-only; use a v3 strategy_package intake with an optional task_overlay_ref")
	}
	if request.Source == IntakeSourceManual && request.ContractVersion == CreativeIntakeCreateV3ContractVersion &&
		request.ManualViralRemake == nil && request.ManualShortDramaPreroll == nil &&
		request.ManualShortDramaPrerollV2 == nil && request.ManualGamePreroll == nil &&
		request.ManualCommercePreroll == nil && request.ManualCommercePrerollV2 == nil && request.ManualBrandFilm == nil {
		request.Format = FormatImageText
		request.SelectedRouteID = ManualImageTextRouteID
		request.CreativeRoutes = []CreativeRouteSnapshot{{
			RouteID: ManualImageTextRouteID, RouteType: CreativeRouteImageText,
			Channels:    []string{string(ChannelXiaohongshu)},
			Reason:      "用户在图文创作工作区直接提交创作需求",
			AspectRatio: "3:4", RequiresHumanConfirmation: true, ReadinessStatus: "ready",
		}}
	}
	project, err := s.Projects.RequireActiveContext(ctx, requestContext.Actor, projectID)
	if err != nil {
		return CreativeIntake{}, err
	}
	if project.OrganizationID != requestContext.Actor.OrganizationID || project.ProjectID != projectID {
		return CreativeIntake{}, fmt.Errorf("resolved project context does not match request scope")
	}
	if request.Source == IntakeSourceManual && strings.TrimSpace(request.ParentIntakeID) != "" {
		parent, parentErr := s.Repository.GetIntake(
			ctx, requestContext.Actor.OrganizationID, projectID, request.ParentIntakeID,
		)
		if parentErr != nil {
			return CreativeIntake{}, parentErr
		}
		if parent.Source != IntakeSourceTaskStrategy || parent.Request.TaskStrategyInput == nil ||
			parent.Status != IntakeReady ||
			!taskStrategyParentSupports(parent.Request.TaskStrategyInput.BusinessCode, request.PerformanceMode) {
			return CreativeIntake{}, fmt.Errorf("parent_intake_id is not a compatible ready task strategy handoff")
		}
	}
	requirementMissing := []string{}
	requirementWarnings := []string{}
	if request.Source == IntakeSourceRequirement {
		request, requirementMissing, requirementWarnings, err = s.resolveRequirementIntake(
			ctx, requestContext.Actor, projectID, request,
		)
		if err != nil {
			return CreativeIntake{}, err
		}
	}
	strategyReady := true
	if request.Source == IntakeSourceStrategyPackage {
		requestedContractVersion := request.ContractVersion
		if s.StrategyPackages == nil {
			return CreativeIntake{}, fmt.Errorf("strategy package intake is unavailable")
		}
		selectedRouteID := strings.TrimSpace(request.SelectedRouteID)
		taskOverlayRef := request.TaskOverlay
		snapshot, readErr := s.StrategyPackages.ReadForCreative(ctx, requestContext.Actor, projectID, *request.StrategyPackage)
		if readErr != nil {
			return CreativeIntake{}, readErr
		}
		request = resolvedStrategyPackageRequest(request.StrategyPackage, snapshot)
		// The strict path never lets legacy resolver defaults become approved
		// strategy facts. CreativeDirection is the first stage allowed to make
		// these creative choices.
		request.ContractVersion = requestedContractVersion
		request.SelectedRouteID = selectedRouteID
		request.TaskOverlay = taskOverlayRef
		request.StrategyHandoffInput = append(json.RawMessage(nil), snapshot.HandoffSnapshot...)
		if taskOverlayRef != nil {
			if s.TaskOverlays == nil {
				return CreativeIntake{}, fmt.Errorf("task strategy overlay intake is unavailable")
			}
			overlay, overlayErr := s.TaskOverlays.ReadTaskOverlayForCreative(
				ctx, requestContext.Actor, projectID, *taskOverlayRef,
			)
			if overlayErr != nil {
				return CreativeIntake{}, overlayErr
			}
			if overlay.PackageRef.PackageID != request.StrategyPackage.PackageID ||
				overlay.PackageRef.PackageVersion != request.StrategyPackage.PackageVersion ||
				!strings.EqualFold(overlay.PackageRef.ExpectedContentHash, request.StrategyPackage.ExpectedContentHash) ||
				!strings.EqualFold(overlay.PackageRef.ExpectedHandoffHash, request.StrategyPackage.ExpectedHandoffHash) ||
				overlay.SelectedRouteID != request.SelectedRouteID {
				return CreativeIntake{}, fmt.Errorf("task strategy overlay lineage does not match the selected Strategy handoff and route")
			}
			request.TaskOverlayInput = &overlay
		}
		selectedRouteFound := selectedRouteID == ""
		selectedRouteReady := false
		var selectedRoute CreativeRouteSnapshot
		for _, route := range request.CreativeRoutes {
			if selectedRouteID != "" && route.RouteID != selectedRouteID {
				continue
			}
			selectedRouteFound = true
			if err := route.Validate(); err != nil {
				return CreativeIntake{}, err
			}
			if selectedRouteID != "" {
				selectedRoute = route
				selectedRouteReady = route.ReadinessStatus == "ready"
			}
		}
		if !selectedRouteFound {
			return CreativeIntake{}, fmt.Errorf("selected_route_id is not present in the Strategy handoff")
		}
		if selectedRouteID != "" {
			request, err = projectStrategyPackageIntake(request, selectedRoute)
			if err != nil {
				return CreativeIntake{}, err
			}
		} else {
			// Legacy package intakes predate frozen route selection. Keep their
			// image-text channel compatibility without restoring fabricated CTA,
			// concept, or visual defaults.
			request.Channel = ChannelXiaohongshu
			if err := request.validateContent(); err != nil {
				return CreativeIntake{}, err
			}
		}
		// A frozen package can be blocked by optional market/language context while
		// its explicitly selected route is already executable. Route readiness is
		// authoritative for this handoff; required creative facts are still checked
		// by request.missingFields below.
		strategyReady = snapshot.CreativeReady || selectedRouteReady
	}
	if request.Source == IntakeSourceTaskStrategy {
		if s.TaskStrategies == nil {
			return CreativeIntake{}, fmt.Errorf("task strategy intake is unavailable")
		}
		reference := *request.TaskStrategy
		snapshot, readErr := s.TaskStrategies.ReadTaskStrategyForCreative(
			ctx, requestContext.Actor, projectID, reference,
		)
		if readErr != nil {
			return CreativeIntake{}, readErr
		}
		request, err = resolvedTaskStrategyRequest(&reference, snapshot)
		if err != nil {
			return CreativeIntake{}, err
		}
	}
	hash, err := contract.CanonicalJSONHash(request)
	if err != nil {
		return CreativeIntake{}, fmt.Errorf("canonicalize creative intake: %w", err)
	}
	inputIdentityHash := ""
	intakeContractVersion := ""
	if request.Source == IntakeSourceStrategyPackage && request.StrategyPackage != nil {
		identityHash, identityErr := contract.NewContentHash(struct {
			PackageID              string `json:"package_id"`
			PackageVersion         int64  `json:"package_version"`
			PackageContentHash     string `json:"package_content_hash"`
			HandoffContractVersion string `json:"handoff_contract_version"`
			HandoffContentHash     string `json:"handoff_content_hash"`
			SelectedRouteID        string `json:"selected_route_id"`
			TaskOverlayHash        string `json:"task_overlay_hash,omitempty"`
		}{
			PackageID:              request.StrategyPackage.PackageID,
			PackageVersion:         request.StrategyPackage.PackageVersion,
			PackageContentHash:     request.StrategyPackage.ExpectedContentHash,
			HandoffContractVersion: request.StrategyPackage.HandoffContractVersion,
			HandoffContentHash:     request.StrategyPackage.ExpectedHandoffHash,
			SelectedRouteID:        request.SelectedRouteID,
			TaskOverlayHash: func() string {
				if request.TaskOverlay == nil {
					return ""
				}
				return request.TaskOverlay.ExpectedContentHash
			}(),
		})
		if identityErr != nil {
			return CreativeIntake{}, fmt.Errorf("hash creative planning input identity: %w", identityErr)
		}
		inputIdentityHash = string(identityHash)
		if request.ContractVersion == CreativeIntakeCreateV3ContractVersion {
			intakeContractVersion = CreativeIntakeV3ContractVersion
		}
	}
	if request.Source == IntakeSourceManual && request.ContractVersion == CreativeIntakeCreateV3ContractVersion {
		identityHash, identityErr := contract.NewContentHash(request)
		if identityErr != nil {
			return CreativeIntake{}, fmt.Errorf("hash manual creative planning input identity: %w", identityErr)
		}
		inputIdentityHash = string(identityHash)
		intakeContractVersion = CreativeIntakeV3ContractVersion
	}
	if request.Source == IntakeSourceRequirement && request.ContractVersion == CreativeIntakeCreateV4ContractVersion {
		identityHash, identityErr := contract.NewContentHash(struct {
			Requirement *RequirementSnapshotReference `json:"requirement_snapshot_ref"`
			Capability  *BusinessCapabilityReference  `json:"business_capability_ref"`
			RouteID     string                        `json:"selected_route_id"`
		}{request.RequirementSnapshotRef, request.BusinessCapabilityRef, request.SelectedRouteID})
		if identityErr != nil {
			return CreativeIntake{}, fmt.Errorf("hash requirement intake identity: %w", identityErr)
		}
		inputIdentityHash = string(identityHash)
		intakeContractVersion = CreativeIntakeV4ContractVersion
	}
	if request.Source == IntakeSourceTaskStrategy && request.TaskStrategy != nil && request.SelectedRouteID != "" {
		identityHash, identityErr := contract.NewContentHash(struct {
			TaskStrategy *TaskStrategyReference `json:"task_strategy"`
			RouteID      string                 `json:"selected_route_id"`
		}{request.TaskStrategy, request.SelectedRouteID})
		if identityErr != nil {
			return CreativeIntake{}, fmt.Errorf("hash task strategy intake identity: %w", identityErr)
		}
		inputIdentityHash = string(identityHash)
		intakeContractVersion = CreativeIntakeV3ContractVersion
	}
	intakeID, err := s.idGenerator()("creativeintake")
	if err != nil {
		return CreativeIntake{}, err
	}
	now := s.now()
	missing := request.missingFields()
	if request.Source == IntakeSourceRequirement {
		missing = requirementMissing
	}
	status := IntakeReady
	confirmedBy := requestContext.Actor.Principal.ID
	if len(missing) > 0 {
		status, confirmedBy = IntakeNeedsClarification, ""
	}
	if !strategyReady {
		status, confirmedBy = IntakeNeedsClarification, ""
		missing = append(missing, "strategy_package.creative_ready")
	}
	value := CreativeIntake{
		ContractVersion: intakeContractVersion,
		ID:              intakeID, OrganizationID: requestContext.Actor.OrganizationID, ProjectID: projectID, Source: request.Source, Status: status,
		Request: request, MissingFields: missing, Warnings: requirementWarnings, ConfirmedBy: confirmedBy, Principal: requestContext.Actor.Principal,
		IdempotencyKey: key, RequestHash: hash, InputIdentityHash: inputIdentityHash,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	stored, existed, err := s.Repository.CreateIntake(ctx, value)
	if err != nil {
		return CreativeIntake{}, err
	}
	if existed && value.Status == IntakeReady && stored.Status == IntakeNeedsClarification &&
		value.InputIdentityHash != "" && value.InputIdentityHash == stored.InputIdentityHash {
		return s.Repository.UpdateIntakeReadiness(
			ctx, value.OrganizationID, value.ProjectID, stored.ID, stored.Version,
			IntakeReady, nil, value.ConfirmedBy, now,
		)
	}
	return stored, nil
}

func taskStrategyParentSupports(businessCode, performanceMode string) bool {
	switch businessCode {
	case BusinessShortDramaPreroll:
		return performanceMode == PerformanceModeShortDramaPreroll
	case BusinessViralRemake:
		return performanceMode == PerformanceModeViralRemake
	default:
		return false
	}
}

func resolvedStrategyPackageRequest(reference *StrategyPackageReference, snapshot StrategyPackageSnapshot) CreateIntakeRequest {
	return CreateIntakeRequest{
		Source: IntakeSourceStrategyPackage, StrategyPackage: reference,
		Objective: snapshot.Objective, Audience: snapshot.Audience, CoreMessage: snapshot.CoreMessage,
		CallToAction: snapshot.CallToAction, Concept: "",
		Tone: append([]string{}, snapshot.Tone...), VisualKeywords: []string{},
		Mandatory: append([]string{}, snapshot.Mandatory...), Prohibited: append([]string{}, snapshot.Prohibited...),
		CreativeRoutes: append([]CreativeRouteSnapshot{}, snapshot.CreativeRoutes...),
	}
}

// projectStrategyPackageIntake derives only production routing fields from the
// user-confirmed frozen route. Creative concepts, prompts, storyboards, and
// visual language are intentionally absent until CreativeDirection.
func projectStrategyPackageIntake(request CreateIntakeRequest, route CreativeRouteSnapshot) (CreateIntakeRequest, error) {
	request.SelectedRouteID = route.RouteID
	switch route.RouteType {
	case CreativeRouteImageText:
		request.Format = FormatImageText
		request.Channel = ChannelXiaohongshu
		if err := request.validateContent(); err != nil {
			return CreateIntakeRequest{}, err
		}
		return request, nil
	case CreativeRouteBrandVideo:
		return projectBrandVideoIntake(request, route)
	default:
		request.Format = FormatVideo
		request.PerformanceMode = route.RouteType
		if len(route.Channels) == 1 {
			request.Channel = supportedCreativeVideoChannel(route.Channels[0])
		}
		if err := request.validateVideoContent(); err != nil {
			return CreateIntakeRequest{}, err
		}
		return request, nil
	}
}

func projectBrandVideoIntake(request CreateIntakeRequest, route CreativeRouteSnapshot) (CreateIntakeRequest, error) {
	request.Format = FormatVideo
	request.PerformanceMode = CreativeRouteBrandVideo
	request.CallToAction = ""
	request.Concept = ""
	request.VisualKeywords = []string{}
	request.Channel = ""

	producibleChannels := 0
	for _, channel := range route.Channels {
		if supportedCreativeVideoChannel(channel) != "" {
			producibleChannels++
		}
	}
	if producibleChannels == 0 {
		return CreateIntakeRequest{}, fmt.Errorf("selected brand-video route has no Creative-supported production channel")
	}
	// A single frozen channel is deterministic. Multi-channel routes deliberately
	// remain unselected so the Creative UI must ask the user before task creation.
	if len(route.Channels) == 1 {
		request.Channel = supportedCreativeVideoChannel(route.Channels[0])
	}
	if err := request.validateVideoContent(); err != nil {
		return CreateIntakeRequest{}, err
	}
	return request, nil
}

func supportedCreativeVideoChannel(channel string) CreativeChannel {
	switch CreativeChannel(channel) {
	case ChannelXiaohongshu, ChannelDouyin, ChannelKuaishou:
		return CreativeChannel(channel)
	default:
		return ""
	}
}

func (s Service) ListIntakes(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, limit int) ([]CreativeIntake, error) {
	if s.Repository == nil || s.Projects == nil {
		return nil, fmt.Errorf("creative dependencies are incomplete")
	}
	if !actor.HasScope(ScopeRead) {
		return nil, fmt.Errorf("%s scope is required", ScopeRead)
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	return s.Repository.ListIntakes(ctx, actor.OrganizationID, projectID, normalizedLimit(limit))
}

func (s Service) GetIntake(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, intakeID string) (CreativeIntake, error) {
	if s.Repository == nil || s.Projects == nil {
		return CreativeIntake{}, fmt.Errorf("creative dependencies are incomplete")
	}
	if !actor.HasScope(ScopeRead) {
		return CreativeIntake{}, fmt.Errorf("%s scope is required", ScopeRead)
	}
	if strings.TrimSpace(intakeID) == "" {
		return CreativeIntake{}, fmt.Errorf("creative intake_id is required")
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return CreativeIntake{}, err
	}
	return s.Repository.GetIntake(ctx, actor.OrganizationID, projectID, intakeID)
}

func (s Service) ListBusinessCapabilities(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID) ([]CreativeBusinessCapability, error) {
	if s.Projects == nil {
		return nil, fmt.Errorf("creative dependencies are incomplete")
	}
	if !actor.HasScope(ScopeRead) {
		return nil, fmt.Errorf("%s scope is required", ScopeRead)
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	return CreativeBusinessCapabilities(), nil
}

func (s Service) CreateTask(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, intakeID string, request CreateTaskRequest) (CreativeTask, error) {
	if s.Repository == nil || s.Projects == nil {
		return CreativeTask{}, fmt.Errorf("creative dependencies are incomplete")
	}
	if !actor.HasScope(ScopeWrite) {
		return CreativeTask{}, fmt.Errorf("%s scope is required", ScopeWrite)
	}
	if err := request.Validate(); err != nil {
		return CreativeTask{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return CreativeTask{}, err
	}
	intake, err := s.Repository.GetIntake(ctx, actor.OrganizationID, projectID, intakeID)
	if err != nil {
		return CreativeTask{}, err
	}
	if intake.Status != IntakeReady {
		return CreativeTask{}, ErrIntakeNotReady
	}
	id, err := s.idGenerator()("creativetask")
	if err != nil {
		return CreativeTask{}, err
	}
	now := s.now()
	direction := CreativeDirection{
		ContentType: request.ContentType, Focus: strings.TrimSpace(request.Focus),
		Audience:     firstNonEmpty(request.Audience, intake.Request.Audience),
		CoreMessage:  firstNonEmpty(request.CoreMessage, intake.Request.CoreMessage),
		CallToAction: firstNonEmpty(request.CallToAction, intake.Request.CallToAction),
		Concept:      strings.TrimSpace(request.Focus), Tone: append([]string{}, intake.Request.Tone...),
		VisualKeywords: append([]string{}, intake.Request.VisualKeywords...),
	}
	if intake.ContractVersion == CreativeIntakeV3ContractVersion {
		if s.Directions == nil || strings.TrimSpace(request.DirectionID) == "" {
			return CreativeTask{}, fmt.Errorf("a confirmed CreativeDirection is required for a v3 intake")
		}
		confirmed, directionErr := s.Directions.GetDirection(
			ctx, actor.OrganizationID, projectID, request.DirectionID,
		)
		if directionErr != nil {
			return CreativeTask{}, directionErr
		}
		if confirmed.Status != "confirmed" || confirmed.IntakeID != intake.ID ||
			confirmed.InputIdentityHash != intake.InputIdentityHash ||
			confirmed.RouteID != intake.Request.SelectedRouteID {
			return CreativeTask{}, fmt.Errorf("confirmed CreativeDirection lineage does not match the v3 intake")
		}
		direction.Focus = confirmed.Concept
		direction.Concept = confirmed.Concept
		direction.DirectionVersionID = confirmed.ID
		direction.InputIdentityHash = confirmed.InputIdentityHash
		direction.Audience = intake.Request.Audience
		direction.CoreMessage = intake.Request.CoreMessage
		direction.CallToAction = intake.Request.CallToAction
		direction.VisualKeywords = append([]string{}, confirmed.ExecutionOutline...)
	}
	task := CreativeTask{ID: id, OrganizationID: actor.OrganizationID, ProjectID: projectID, IntakeID: intake.ID, Format: FormatImageText, Channel: intake.Request.Channel, Status: TaskDraft, Direction: direction, Version: 1, CreatedAt: now, UpdatedAt: now}
	draft := composeXiaohongshuDraft(task.ID, intake, direction, now)
	stored, err := s.Repository.CreateTask(ctx, task, draft)
	if err != nil {
		return CreativeTask{}, err
	}
	return stored, nil
}

func firstNonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(fallback)
}

func (s Service) ListTasks(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, limit int) ([]CreativeTask, error) {
	if s.Repository == nil || s.Projects == nil {
		return nil, fmt.Errorf("creative dependencies are incomplete")
	}
	if !actor.HasScope(ScopeRead) {
		return nil, fmt.Errorf("%s scope is required", ScopeRead)
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	return s.Repository.ListTasks(ctx, actor.OrganizationID, projectID, normalizedLimit(limit))
}

func (s Service) RenameTask(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request RenameTaskRequest) (CreativeTask, error) {
	metadata, ok := s.Repository.(TaskMetadataRepository)
	if !ok || s.Projects == nil {
		return CreativeTask{}, fmt.Errorf("creative task metadata dependencies are incomplete")
	}
	if !actor.HasScope(ScopeWrite) {
		return CreativeTask{}, fmt.Errorf("%s scope is required", ScopeWrite)
	}
	if err := request.Validate(); err != nil {
		return CreativeTask{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return CreativeTask{}, err
	}
	return metadata.RenameTask(ctx, actor.OrganizationID, projectID, taskID, request.ExpectedVersion, strings.TrimSpace(request.DisplayName), s.now())
}

func (s Service) GetTaskDetail(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string) (TaskDetail, error) {
	if s.Repository == nil || s.Projects == nil {
		return TaskDetail{}, fmt.Errorf("creative dependencies are incomplete")
	}
	if !actor.HasScope(ScopeRead) {
		return TaskDetail{}, fmt.Errorf("%s scope is required", ScopeRead)
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return TaskDetail{}, err
	}
	return s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
}

// ArchiveTask removes a task from the active Creative queue without deleting
// its drafts, frozen versions, Provider jobs, or Asset lineage. Those records
// are evidence used by downstream systems and must remain traceable.
func (s Service) ArchiveTask(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string) error {
	if s.Repository == nil || s.Projects == nil {
		return fmt.Errorf("creative dependencies are incomplete")
	}
	if !actor.HasScope(ScopeWrite) {
		return fmt.Errorf("%s scope is required", ScopeWrite)
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return err
	}
	detail, err := s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
	if err != nil {
		return err
	}
	if !CanTransitionCreativeTaskStatus(detail.Task.Status, TaskArchived) {
		return ErrInvalidState
	}
	return s.Repository.ArchiveTask(ctx, actor.OrganizationID, projectID, taskID, s.now())
}

// ReviseDraft creates the next editable revision. It does not mutate an older
// revision, so a previously frozen CreativeVersion continues to point at the
// exact content that was reviewed.
func (s Service) ReviseDraft(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request ReviseDraftRequest) (ImageTextDraft, error) {
	if s.Repository == nil || s.Projects == nil {
		return ImageTextDraft{}, fmt.Errorf("creative dependencies are incomplete")
	}
	if !actor.HasScope(ScopeWrite) {
		return ImageTextDraft{}, fmt.Errorf("%s scope is required", ScopeWrite)
	}
	if err := request.Validate(); err != nil {
		return ImageTextDraft{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ImageTextDraft{}, err
	}
	draft := request.Draft(taskID, request.ExpectedVersion+1, s.now())
	return s.Repository.ReviseDraft(ctx, actor.OrganizationID, projectID, taskID, request.ExpectedVersion, draft)
}

// BindImageAsset advances the editable draft after the HTTP composition layer
// has verified that ref is a ready asset in the same project. Creative stores
// only the reference so Assets remains the source of truth for media bytes.
func (s Service) BindImageAsset(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request BindImageAssetRequest) (ImageTextDraft, error) {
	if s.Repository == nil || s.Projects == nil {
		return ImageTextDraft{}, fmt.Errorf("creative dependencies are incomplete")
	}
	if !actor.HasScope(ScopeWrite) {
		return ImageTextDraft{}, fmt.Errorf("%s scope is required", ScopeWrite)
	}
	if err := request.Validate(); err != nil {
		return ImageTextDraft{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ImageTextDraft{}, err
	}
	detail, err := s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
	if err != nil {
		return ImageTextDraft{}, err
	}
	if detail.Task.Status == TaskArchived || detail.Draft.Version != request.ExpectedDraftVersion {
		if detail.Task.Status == TaskArchived {
			return ImageTextDraft{}, ErrInvalidState
		}
		return ImageTextDraft{}, ErrVersionConflict
	}
	if request.ImagePlanOrder > len(detail.Draft.ImagePlan) {
		return ImageTextDraft{}, fmt.Errorf("image_plan_order does not exist in this draft")
	}
	updated := detail.Draft
	updated.Version++
	updated.CreatedAt = s.now()
	ref := request.AssetRef
	updated.ImagePlan[request.ImagePlanOrder-1].AssetRef = &ref
	return s.Repository.ReviseDraft(ctx, actor.OrganizationID, projectID, taskID, request.ExpectedDraftVersion, updated)
}

func (s Service) RegisterCoverImageJob(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID, providerJobID string) error {
	return s.RegisterImagePlanJob(ctx, actor, projectID, taskID, 1, providerJobID)
}

func (s Service) RegisterVideoJob(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID, providerJobID string) error {
	if s.Repository == nil || s.Projects == nil {
		return fmt.Errorf("creative dependencies are incomplete")
	}
	if !actor.HasScope(ScopeWrite) {
		return fmt.Errorf("%s scope is required", ScopeWrite)
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return err
	}
	if strings.TrimSpace(providerJobID) == "" {
		return fmt.Errorf("provider job ID is required")
	}
	detail, err := s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
	if err != nil {
		return err
	}
	if detail.Task.Format != FormatVideo || detail.VideoDraft == nil || detail.Task.Status == TaskArchived {
		return ErrInvalidState
	}
	return s.Repository.RegisterProductionJob(ctx, actor.OrganizationID, projectID, taskID, ProductionJob{
		TaskID: taskID, Kind: "video_generate", ProviderJobID: providerJobID, CreatedAt: s.now(),
	})
}

func (s Service) RegisterImagePlanJob(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, imagePlanOrder int, providerJobID string) error {
	if s.Repository == nil || s.Projects == nil {
		return fmt.Errorf("creative dependencies are incomplete")
	}
	if !actor.HasScope(ScopeWrite) {
		return fmt.Errorf("%s scope is required", ScopeWrite)
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return err
	}
	if imagePlanOrder < 1 || imagePlanOrder > 12 || strings.TrimSpace(providerJobID) == "" {
		return fmt.Errorf("provider job ID is required")
	}
	detail, err := s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
	if err != nil {
		return err
	}
	if detail.Task.Status == TaskArchived || imagePlanOrder > len(detail.Draft.ImagePlan) {
		return ErrInvalidState
	}
	return s.Repository.RegisterProductionJob(ctx, actor.OrganizationID, projectID, taskID, ProductionJob{TaskID: taskID, Kind: imagePlanJobKind(imagePlanOrder, providerJobID), ProviderJobID: providerJobID, CreatedAt: s.now()})
}

// FreezeVersion creates the stable Creative-owned artifact consumed by later
// Delivery and Insights modules. It deliberately snapshots the current draft
// instead of exposing a mutable task or a Provider job as a cross-system ref.
func (s Service) FreezeVersion(ctx context.Context, requestContext contract.RequestContext, projectID contract.ProjectID, taskID string, request FreezeVersionRequest, key contract.IdempotencyKey) (CreativeVersion, bool, error) {
	if s.Repository == nil || s.Projects == nil {
		return CreativeVersion{}, false, fmt.Errorf("creative dependencies are incomplete")
	}
	if err := requestContext.Validate(); err != nil {
		return CreativeVersion{}, false, err
	}
	if !requestContext.Actor.HasScope(ScopeWrite) {
		return CreativeVersion{}, false, fmt.Errorf("%s scope is required", ScopeWrite)
	}
	if err := key.Validate(); err != nil {
		return CreativeVersion{}, false, err
	}
	if err := request.Validate(); err != nil {
		return CreativeVersion{}, false, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, requestContext.Actor, projectID); err != nil {
		return CreativeVersion{}, false, err
	}
	detail, err := s.Repository.GetTaskDetail(ctx, requestContext.Actor.OrganizationID, projectID, taskID)
	if err != nil {
		return CreativeVersion{}, false, err
	}
	var imageSnapshot ImageTextDraft
	var videoSnapshot *VideoVersionSnapshot
	draftVersion := request.DraftVersion
	if detail.Task.Format == FormatVideo {
		if detail.VideoDraft == nil || detail.VideoDraft.Revision != request.DraftVersion {
			return CreativeVersion{}, false, ErrVersionConflict
		}
		if detail.Task.PerformanceMode == PerformanceModeBrandFilm {
			brand := detail.VideoDraft.BrandFilm
			if brand == nil || brand.Generation == nil || brand.Generation.PreviewAsset == nil || !brandFilmQualityConfirmed(*brand) {
				return CreativeVersion{}, false, ErrInvalidState
			}
			run := brand.QualityRuns[len(brand.QualityRuns)-1]
			plan := brand.CurrentPlan()
			if plan == nil || run.HumanConfirmedAt == nil {
				return CreativeVersion{}, false, ErrInvalidState
			}
			metrics := brandFilmRunMetrics(*brand.Generation)
			brandSnapshot := &BrandFilmVersionSnapshot{
				ContractVersion: "creative-brand-film-version/v1", PlanRevision: plan.Revision, QualityRunID: run.ID,
				ReferenceAsset: brand.Generation.ReferenceAsset, FinalVideo: *brand.Generation.PreviewAsset,
				UnitCount: len(brand.Generation.Units), AttemptCount: metrics.AttemptCount,
				ConfirmedBy: run.HumanConfirmedBy, ConfirmedAt: *run.HumanConfirmedAt,
			}
			if brand.SourceSnapshot.SourceType == strategyBrandFilmSourceType {
				source := brand.SourceSnapshot
				brandSnapshot.Lineage = &BrandFilmLineageSnapshot{
					SourceType: source.SourceType, IntakeID: source.IntakeID, InputIdentityHash: source.InputIdentityHash,
					StrategyPackageID: source.StrategyPackageID, StrategyPackageVersion: source.StrategyPackageVersion,
					StrategyPackageHash: source.StrategyPackageHash, HandoffContractVersion: source.HandoffContractVersion,
					HandoffContentHash: source.HandoffContentHash, BrandBriefRevision: source.BrandBriefRevision,
					BrandBriefContentHash: source.BrandBriefContentHash, DirectionBatchID: source.DirectionBatchID,
					DirectionID: source.DirectionID, DirectionVersion: source.DirectionVersion,
					DirectionContentHash: source.DirectionContentHash, RouteID: source.RouteID,
				}
			}
			videoSnapshot = &VideoVersionSnapshot{
				ContractVersion: "creative-video-version/v1", Format: FormatVideo, Channel: detail.Task.Channel,
				VideoPurpose: detail.Task.VideoPurpose, PerformanceMode: detail.Task.PerformanceMode,
				DraftRevision: detail.VideoDraft.Revision, FinalVideo: *brand.Generation.PreviewAsset, BrandFilm: brandSnapshot,
			}
			if err := videoSnapshot.Validate(); err != nil {
				return CreativeVersion{}, false, err
			}
		} else if strings.TrimSpace(request.RenderJobID) == "" {
			return CreativeVersion{}, false, fmt.Errorf("render_job_id is required for a video version")
		} else {
			render, renderErr := s.Repository.GetRenderJob(ctx, requestContext.Actor.OrganizationID, projectID, request.RenderJobID)
			if renderErr != nil {
				return CreativeVersion{}, false, renderErr
			}
			if render.TaskID != taskID || render.Status != RenderSucceeded || render.OutputAsset == nil {
				return CreativeVersion{}, false, ErrInvalidState
			}
			providerJobID := ""
			for _, job := range detail.ProductionJobs {
				if job.Kind == "video_generate" {
					providerJobID = job.ProviderJobID
				}
			}
			videoSnapshot = &VideoVersionSnapshot{
				ContractVersion: "creative-video-version/v1", Format: FormatVideo, Channel: detail.Task.Channel,
				VideoPurpose: detail.Task.VideoPurpose, PerformanceMode: detail.Task.PerformanceMode,
				StrategyPackage: detail.Intake.Request.StrategyPackage, DraftRevision: detail.VideoDraft.Revision,
				SourceVideo: detail.VideoDraft.SourceVideo, GeneratedPreRoll: render.PreRollVideo,
				FinalVideo: render.OutputAsset.AssetVersion, ProviderJobID: providerJobID, RenderJobID: render.ID,
			}
			if err := videoSnapshot.Validate(); err != nil {
				return CreativeVersion{}, false, err
			}
		}
	} else {
		if detail.Draft.Version != request.DraftVersion {
			return CreativeVersion{}, false, ErrVersionConflict
		}
		imageSnapshot = detail.Draft
	}
	hashInput := struct {
		TaskID       string                `json:"creative_task_id"`
		DraftVersion int64                 `json:"draft_version"`
		Format       CreativeFormat        `json:"format"`
		Channel      CreativeChannel       `json:"channel"`
		Image        ImageTextDraft        `json:"image_text_snapshot,omitempty"`
		Video        *VideoVersionSnapshot `json:"video_snapshot,omitempty"`
	}{TaskID: detail.Task.ID, DraftVersion: draftVersion, Format: detail.Task.Format, Channel: detail.Task.Channel, Image: imageSnapshot, Video: videoSnapshot}
	contentHash, err := contract.NewContentHash(hashInput)
	if err != nil {
		return CreativeVersion{}, false, err
	}
	requestHash, err := contract.CanonicalJSONHash(request)
	if err != nil {
		return CreativeVersion{}, false, err
	}
	id, err := s.idGenerator()("creativeversion")
	if err != nil {
		return CreativeVersion{}, false, err
	}
	value := CreativeVersion{
		ID: id, OrganizationID: requestContext.Actor.OrganizationID, ProjectID: projectID, TaskID: taskID,
		Format: detail.Task.Format, Version: draftVersion, DraftVersion: draftVersion, Status: CreativeVersionCreated,
		Snapshot: imageSnapshot, VideoSnapshot: videoSnapshot, ContentHash: contentHash, CreatedBy: requestContext.Actor.Principal.ID,
		CreatedAt: s.now(), IdempotencyKey: key, RequestHash: requestHash,
	}
	if err := value.Validate(); err != nil {
		return CreativeVersion{}, false, err
	}
	return s.Repository.CreateVersion(ctx, value)
}

// CheckVersion validates a frozen version without changing it. The Phase-1
// rules intentionally cover only delivery blockers that are deterministic:
// every planned image must have a project Asset reference, prohibited claims
// must not appear in copy, and every mandatory element must be represented.
func (s Service) CheckVersion(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, versionID string) (CreativeVersion, error) {
	if s.Repository == nil || s.Projects == nil {
		return CreativeVersion{}, fmt.Errorf("creative dependencies are incomplete")
	}
	if !actor.HasScope(ScopeWrite) {
		return CreativeVersion{}, fmt.Errorf("%s scope is required", ScopeWrite)
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return CreativeVersion{}, err
	}
	version, err := s.Repository.GetVersion(ctx, actor.OrganizationID, projectID, versionID)
	if err != nil {
		return CreativeVersion{}, err
	}
	if !CanTransitionCreativeVersionStatus(version.Status, CreativeVersionChecked) {
		return CreativeVersion{}, ErrInvalidState
	}
	var check CreativeCheck
	if version.EditTaskID != "" {
		check = evaluateEditingVersion(version, actor.Principal.ID, s.now())
	} else {
		detail, detailErr := s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, version.TaskID)
		if detailErr != nil {
			return CreativeVersion{}, detailErr
		}
		check = evaluateVersion(version, detail.Intake, actor.Principal.ID, s.now())
	}
	return s.Repository.RecordVersionCheck(ctx, actor.OrganizationID, projectID, versionID, check)
}

// ListVersions restores immutable Creative history after a browser refresh.
// taskID is optional so the Project stage summary can use the same read model.
func (s Service) ListVersions(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, limit int) ([]CreativeVersion, error) {
	if s.Repository == nil || s.Projects == nil {
		return nil, fmt.Errorf("creative dependencies are incomplete")
	}
	if !actor.HasScope(ScopeRead) {
		return nil, fmt.Errorf("%s scope is required", ScopeRead)
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	return s.Repository.ListVersions(ctx, actor.OrganizationID, projectID, strings.TrimSpace(taskID), limit)
}

// ListPackages exposes only Creative's immutable handoff objects. Delivery
// never needs to inspect Creative drafts or tables.
func (s Service) ListPackages(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, limit int) ([]CreativePackage, error) {
	if s.Repository == nil || s.Projects == nil {
		return nil, fmt.Errorf("creative dependencies are incomplete")
	}
	if !actor.HasScope(ScopeRead) {
		return nil, fmt.Errorf("%s scope is required", ScopeRead)
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	return s.Repository.ListPackages(ctx, actor.OrganizationID, projectID, limit)
}

func (s Service) ApproveVersion(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, versionID string) (CreativeVersion, error) {
	if s.Repository == nil || s.Projects == nil {
		return CreativeVersion{}, fmt.Errorf("creative dependencies are incomplete")
	}
	if !actor.HasScope(ScopeWrite) {
		return CreativeVersion{}, fmt.Errorf("%s scope is required", ScopeWrite)
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return CreativeVersion{}, err
	}
	version, err := s.Repository.GetVersion(ctx, actor.OrganizationID, projectID, versionID)
	if err != nil {
		return CreativeVersion{}, err
	}
	if !CanTransitionCreativeVersionStatus(version.Status, CreativeVersionApproved) ||
		version.Check == nil || !version.Check.Passed {
		return CreativeVersion{}, ErrInvalidState
	}
	approved, err := s.Repository.ApproveVersion(ctx, actor.OrganizationID, projectID, versionID, CreativeApproval{ApprovedBy: actor.Principal.ID, ApprovedAt: s.now()})
	if err != nil {
		return CreativeVersion{}, err
	}
	if approved.EditTaskID != "" && s.EditTasks != nil {
		if err = s.EditTasks.UpdateEditTaskStatus(ctx, actor.OrganizationID, projectID, approved.EditTaskID, EditTaskCompleted, s.now()); err != nil {
			return CreativeVersion{}, err
		}
	}
	return approved, nil
}

func (s Service) DeliverVersion(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, versionID string) (CreativePackage, error) {
	if s.Repository == nil || s.Projects == nil {
		return CreativePackage{}, fmt.Errorf("creative dependencies are incomplete")
	}
	if !actor.HasScope(ScopeWrite) {
		return CreativePackage{}, fmt.Errorf("%s scope is required", ScopeWrite)
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return CreativePackage{}, err
	}
	version, err := s.Repository.GetVersion(ctx, actor.OrganizationID, projectID, versionID)
	if err != nil {
		return CreativePackage{}, err
	}
	if version.Status != CreativeVersionApproved {
		return CreativePackage{}, ErrInvalidState
	}
	id, err := s.idGenerator()("creativepackage")
	if err != nil {
		return CreativePackage{}, err
	}
	value := CreativePackage{ID: id, OrganizationID: actor.OrganizationID, ProjectID: projectID, CreativeVersionID: version.ID, EditTaskID: version.EditTaskID, Format: version.Format, ContentHash: version.ContentHash, Snapshot: version.Snapshot, VideoSnapshot: version.VideoSnapshot, CreatedBy: actor.Principal.ID, CreatedAt: s.now()}
	return s.Repository.CreatePackage(ctx, value)
}

// Provider jobs are immutable attempts. Including the Provider job identity
// permits retrying only a failed image-plan position while preserving every
// prior attempt for audit and avoiding a shared mutable "latest job" field.
func imagePlanJobKind(order int, providerJobID string) string {
	return fmt.Sprintf("image_plan_%d_job_%s", order, providerJobID)
}

func evaluateVersion(version CreativeVersion, intake CreativeIntake, actorID string, now time.Time) CreativeCheck {
	blockers := make([]string, 0)
	warnings := make([]string, 0)
	if version.Format == FormatVideo {
		if version.VideoSnapshot == nil || version.VideoSnapshot.Validate() != nil {
			blockers = append(blockers, "video production lineage is incomplete")
		}
		return CreativeCheck{Passed: len(blockers) == 0, Blockers: blockers, Warnings: warnings, CheckedBy: actorID, CheckedAt: now}
	}
	for _, item := range version.Snapshot.ImagePlan {
		if item.AssetRef == nil {
			blockers = append(blockers, fmt.Sprintf("image_plan[%d] has no bound project asset", item.Order))
		}
	}
	copyText := strings.ToLower(strings.Join(append(append([]string{}, version.Snapshot.TitleCandidates...), version.Snapshot.Body, version.Snapshot.CoverCopy, strings.Join(version.Snapshot.Topics, " ")), " "))
	for _, prohibited := range intake.Request.Prohibited {
		if needle := strings.ToLower(strings.TrimSpace(prohibited)); needle != "" && strings.Contains(copyText, needle) {
			blockers = append(blockers, fmt.Sprintf("prohibited claim appears in copy: %s", prohibited))
		}
	}
	for _, mandatory := range intake.Request.Mandatory {
		if !mandatoryElementSatisfied(mandatory, copyText) {
			warnings = append(warnings, fmt.Sprintf("mandatory element is not found in text: %s", mandatory))
		}
	}
	return CreativeCheck{Passed: len(blockers) == 0, Blockers: blockers, Warnings: warnings, CheckedBy: actorID, CheckedAt: now}
}

func mandatoryElementSatisfied(requirement string, copyText string) bool {
	requirement = strings.ToLower(strings.TrimSpace(requirement))
	if requirement == "" || strings.Contains(copyText, requirement) {
		return true
	}

	hasScene := containsAny(copyText, "新产品", "打样", "替换供应商", "供应商", "项目")
	hasEvidence := containsAny(copyText, "可核验", "核验", "判断依据", "工程评估", "资质", "文件")
	hasConsultation := containsAny(copyText, "私信", "咨询", "评论", "联系", "沟通")

	// Strategy hand-offs describe semantic requirements, not copy that must be
	// repeated verbatim. Recognize the three cross-route requirement classes so
	// a valid draft does not receive a warning simply for using natural wording.
	if strings.Contains(requirement, "触发场景") && strings.Contains(requirement, "证据") && strings.Contains(requirement, "咨询") {
		return hasScene && hasEvidence && hasConsultation
	}
	if strings.Contains(requirement, "触发场景") {
		return hasScene
	}
	if strings.Contains(requirement, "证据位") || strings.Contains(requirement, "可核验") || strings.Contains(requirement, "待核验") {
		return hasEvidence
	}
	if strings.Contains(requirement, "低摩擦") || strings.Contains(requirement, "咨询动作") {
		return hasConsultation
	}

	// Policy directives constrain how copy is written; they are not visible-copy
	// requirements. Prohibited claims are checked separately above, while these
	// directives remain part of the immutable intake for audit and human review.
	return strings.HasPrefix(requirement, "不得") ||
		strings.HasPrefix(requirement, "禁止") ||
		strings.HasPrefix(requirement, "避免") ||
		strings.HasPrefix(requirement, "仅使用") ||
		strings.HasPrefix(requirement, "所有内容需")
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s Service) idGenerator() ids.Generator {
	if s.NewID != nil {
		return s.NewID
	}
	return ids.New
}
func normalizedLimit(limit int) int {
	if limit < 1 {
		return 50
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func composeXiaohongshuDraft(taskID string, intake CreativeIntake, direction CreativeDirection, now time.Time) ImageTextDraft {
	r := intake.Request
	message := strings.TrimSpace(direction.CoreMessage)
	concept := strings.TrimSpace(direction.Concept)
	if concept == "" {
		concept = "把产品价值放进真实使用场景"
	}
	titles := []string{message + "：这次想认真说清楚", "不只是一句卖点，" + message, "给" + strings.TrimSpace(r.Audience) + "的一份实用说明"}
	cover := message
	if len([]rune(cover)) > 18 {
		cover = string([]rune(cover)[:18])
	}
	body := fmt.Sprintf("最近在为%s整理一个更清楚的表达：%s。\n\n%s\n\n先从真实场景出发，再把值得被看见的细节说具体。%s", strings.TrimSpace(r.Audience), message, concept, ctaSentence(r.CallToAction))
	topics := []string{"#品牌内容", "#创意灵感", "#好内容值得被看见"}
	return ImageTextDraft{TaskID: taskID, Version: 1, Status: "draft", TitleCandidates: titles, Body: body, Topics: topics, CoverCopy: cover, ImagePlan: []ImagePlanItem{
		{Order: 1, Purpose: "封面", VisualBrief: concept + "，突出核心信息：" + message, Caption: cover},
		{Order: 2, Purpose: "场景", VisualBrief: "目标人群的真实使用场景，画面自然可信", Caption: "从一个具体场景开始"},
		{Order: 3, Purpose: "价值", VisualBrief: "以细节或产品体验证明核心信息", Caption: message},
		{Order: 4, Purpose: "行动", VisualBrief: "干净的品牌收束画面，留出行动引导区域", Caption: strings.TrimSpace(r.CallToAction)},
	}, CreatedAt: now}
}

func ctaSentence(value string) string {
	if strings.TrimSpace(value) == "" {
		return "欢迎把你的想法留在评论区。"
	}
	return "如果你也在关注这件事，" + strings.TrimSpace(value) + "。"
}
