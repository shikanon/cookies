// cookies-api starts the shared platform HTTP surface.
//
// It deliberately exposes only operational endpoints and a request-context
// probe while the platform modules are being built. Business systems own their
// own HTTP surfaces under /api/{system}/v1.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/shikanon/cookies/internal/integrations/crawler"
	"github.com/shikanon/cookies/internal/integrations/creativeprovider"
	"github.com/shikanon/cookies/internal/integrations/deliveryinsights"
	"github.com/shikanon/cookies/internal/integrations/gotenberg"
	"github.com/shikanon/cookies/internal/integrations/lasdocument"
	"github.com/shikanon/cookies/internal/integrations/oceanengine"
	"github.com/shikanon/cookies/internal/integrations/productsource"
	"github.com/shikanon/cookies/internal/integrations/seedresearch"
	"github.com/shikanon/cookies/internal/integrations/strategycreative"
	"github.com/shikanon/cookies/internal/platform/agent"
	"github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/browserautomation"
	browserautomationhttp "github.com/shikanon/cookies/internal/platform/browserautomation/httpapi"
	"github.com/shikanon/cookies/internal/platform/browserautomation/plancompile"
	"github.com/shikanon/cookies/internal/platform/browserautomation/rparunner"
	webapiadapter "github.com/shikanon/cookies/internal/platform/browserautomation/webapi"
	"github.com/shikanon/cookies/internal/platform/config"
	"github.com/shikanon/cookies/internal/platform/connector"
	connectorhttp "github.com/shikanon/cookies/internal/platform/connector/httpapi"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/database"
	"github.com/shikanon/cookies/internal/platform/httpserver"
	"github.com/shikanon/cookies/internal/platform/identity"
	"github.com/shikanon/cookies/internal/platform/ids"
	"github.com/shikanon/cookies/internal/platform/jobruntime"
	"github.com/shikanon/cookies/internal/platform/knowledge"
	"github.com/shikanon/cookies/internal/platform/media"
	"github.com/shikanon/cookies/internal/platform/mediaunderstanding"
	mediaunderstandinghttp "github.com/shikanon/cookies/internal/platform/mediaunderstanding/httpapi"
	"github.com/shikanon/cookies/internal/platform/project"
	"github.com/shikanon/cookies/internal/platform/provider"
	"github.com/shikanon/cookies/internal/platform/remix"
	"github.com/shikanon/cookies/internal/systems/creative"
	"github.com/shikanon/cookies/internal/systems/delivery"
	"github.com/shikanon/cookies/internal/systems/delivery/calibrationmanifest"
	deliveryhttp "github.com/shikanon/cookies/internal/systems/delivery/httpapi"
	"github.com/shikanon/cookies/internal/systems/insights"
	insightshttp "github.com/shikanon/cookies/internal/systems/insights/httpapi"
	strategysystem "github.com/shikanon/cookies/internal/systems/strategy"
	strategyhttp "github.com/shikanon/cookies/internal/systems/strategy/httpapi"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}
	ffmpegPath := localExecutablePath(cfg.Environment, cfg.Media.FFmpegPath, "ffmpeg")
	ffprobePath := localExecutablePath(cfg.Environment, cfg.Media.FFprobePath, "ffprobe")
	if cfg.MediaUnderstanding.ASREnabled && ffmpegPath == "" {
		log.Fatal("invalid configuration: COOKIES_MEDIA_UNDERSTANDING_ASR_ENABLED requires an available ffmpeg executable")
	}

	db, err := database.Open(context.Background(), cfg.MySQL)
	if err != nil {
		log.Fatalf("open MySQL: %v", err)
	}
	defer db.Close()
	identityStore := identity.MySQLStore{DB: db}
	projectStore := project.MySQLStore{DB: db}
	resolver, actor, err := buildIdentityResolver(cfg, identityStore)
	if err != nil {
		log.Fatalf("invalid identity configuration: %v", err)
	}
	seedProjectID := contract.ProjectID("")
	if cfg.LocalIdentity != nil {
		seedProjectID = contract.ProjectID(cfg.LocalIdentity.ProjectID)
	}
	if cfg.Auth.PasswordEnabled && actor == nil {
		adminActor := contract.ActorContext{
			OrganizationID: "org_admin",
			Principal:      contract.Principal{Kind: contract.PrincipalUser, ID: "user_admin"},
			Scopes: contract.ScopesFromStrings([]string{
				"project.read", "project.write", "assets.read", "assets.write",
				"provider.read", "provider.generate", "provider.job.create", "provider.text.generate",
				"strategy.read", "strategy.write", "strategy.confirm", "strategy.review",
				"strategy.approve", "strategy.package.read", "creative.read", "creative.write",
				"delivery.read", "delivery.write", "delivery.approve", "delivery.execute",
				"insights.read", "insights.write", "insights.confirm",
				connector.ScopeRead, connector.ScopeSync,
			}),
		}
		actor = &adminActor
		seedProjectID = "project_admin"
	}
	if actor != nil {
		if err := identityStore.EnsureLocalActor(context.Background(), *actor); err != nil {
			log.Fatalf("seed local identity: %v", err)
		}
		if err := projectStore.EnsureLocalProject(context.Background(), *actor, seedProjectID); err != nil {
			log.Fatalf("seed local project: %v", err)
		}
	}
	var sessionService *identity.PasswordSessionService
	if cfg.Auth.PasswordEnabled {
		if actor == nil {
			log.Fatal("password authentication requires a bootstrap actor")
		}
		value := &identity.PasswordSessionService{
			DB: db, Validator: identityStore,
			UserScopes: identityStore,
			SessionTTL: time.Duration(cfg.Auth.SessionHours) * time.Hour,
			Secure:     cfg.Environment != config.EnvironmentLocal && cfg.Environment != config.EnvironmentTest,
		}
		if err := value.EnsureBootstrapCredential(context.Background(), *actor, cfg.Auth.Username, cfg.Auth.Password); err != nil {
			log.Fatalf("seed password credential: %v", err)
		}
		sessionService = value
		resolver = value
	}
	blobs, err := buildBlobStore(cfg)
	if err != nil {
		log.Fatalf("configure object storage: %v", err)
	}
	scanner := buildScanner(cfg)
	projectService := &project.Service{Store: projectStore, Authorizer: projectStore}
	assetRepository := assets.MySQLRepository{DB: db}
	uploadService := &assets.UploadService{Repository: assetRepository, Projects: projectService, Blobs: blobs, Scanner: scanner, QuarantineBucket: cfg.ObjectStorage.QuarantineBucket, AssetsBucket: cfg.ObjectStorage.AssetsBucket, UsePolicy: assets.AssetUsePolicy{Rights: assetRepository}}
	if ffprobePath != "" {
		uploadService.VideoProbe = assets.FFprobeVideoProbe{Path: ffprobePath, WorkRoot: cfg.Media.VideoWorkRoot}
		uploadService.AudioProbe = assets.FFprobeAudioProbe{Path: ffprobePath, WorkRoot: cfg.Media.VideoWorkRoot}
	}
	intakeService := &assets.GeneratedIntakeService{Repository: assetRepository, Projects: projectService}
	creativeRepository := creative.MySQLRepository{DB: db}
	productionCenter := &creative.ProductionCenterService{
		Projects: projectService,
		Sources: []creative.ProductionRunSource{
			creative.CreativeRenderRunAdapter{Jobs: creativeRepository},
			creative.EditingRenderRunAdapter{Jobs: creativeRepository},
		},
		Assets: creative.AssetReadAdapter{Assets: uploadService},
	}
	creativeService := &creative.Service{
		Repository: creativeRepository, ViralRemakes: creativeRepository, EditTasks: creativeRepository, EditingRenders: creativeRepository,
		Projects: projectService, Assets: creativeAssetReader{uploads: uploadService}, AssetUses: assets.AssetUsePolicy{Rights: assetRepository},
		AudioAssets:        creativeAudioAssetWriter{uploads: uploadService},
		CommerceWorkspaces: creativeRepository, BrandBriefs: creativeRepository, Directions: creativeRepository,
		AINativeProducts:       creativeProductResolver{resolver: productsource.NewResolver()},
		AINativeRequirements:   creativeRepository,
		AINativeScripts:        creativeRepository,
		AINativeScriptProfiles: creative.NewChannelCreativeProfileRegistry(),
		AINativeOutputPresets: func() *creative.OutputPresetRegistry {
			value := creative.NewOutputPresetRegistry(creative.NewChannelCreativeProfileRegistry())
			return &value
		}(),
		AINativeProductMediaImporter: creativeProductMediaImporter{uploads: uploadService},
	}
	productionRetryAdapters := []creative.ProductionRetryAdapter{
		creative.EditingRenderProductionRetryAdapter{Renders: creativeService},
	}
	productionCenter.RetryAdapters = productionRetryAdapters
	productionRetry := &creative.ProductionRetryService{
		Projects: projectService, Sources: productionCenter.Sources, Adapters: productionRetryAdapters,
		Ledger: creativeRepository, Audit: productionRetryAuditAdapter{store: projectStore},
	}
	if cfg.Creative.DirectionPlanningEnabled {
		textAdapter, textAdapterErr := buildTextAdapter(cfg, db)
		if textAdapterErr != nil {
			log.Fatalf("configure CreativeDirection planner: %v", textAdapterErr)
		}
		creativeService.DirectionPlanner = creative.ModelCreativeDirectionPlanner{
			Text:       &provider.Service{TextAdapter: textAdapter},
			ModelAlias: cfg.Creative.DirectionPlannerModelAlias,
		}
		creativeService.ImageTextDraftPlanner = creative.ModelImageTextDraftPlanner{
			Text:       &provider.Service{TextAdapter: textAdapter},
			ModelAlias: cfg.Creative.DirectionPlannerModelAlias,
		}
		log.Printf(
			"CreativeDirection planning configured: model_alias=%s routes=xiaohongshu_image_text,brand_video",
			cfg.Creative.DirectionPlannerModelAlias,
		)
	}
	creativeService.ImageBaseAssets = creativeImageAssetIO{uploads: uploadService}
	creativeService.RenderedImages = creativeRenderedImageWriter{uploads: uploadService}
	if fontPath := cfg.Creative.ImageFontPath; fontPath != "" {
		fontBytes, fontErr := os.ReadFile(fontPath)
		if fontErr != nil {
			log.Fatalf("read Creative image renderer font: %v", fontErr)
		}
		fontChecksum := cfg.Creative.ImageFontSHA256
		renderer := &creative.ImageTextRenderer{
			FontBytes: fontBytes, FontRef: fontPath + "@sha256:" + fontChecksum,
			ExpectedSHA256: fontChecksum,
		}
		if readyErr := renderer.Ready(); readyErr != nil {
			log.Fatalf("configure Creative image renderer: %v", readyErr)
		}
		creativeService.ImageRenderer = renderer
		log.Printf("Creative image renderer configured: font_ref=%s renderer=%s", fontPath, creative.ImageRendererV2)
	}
	if ffmpegPath != "" {
		creativeService.GameEvidenceFrames = media.FFmpegFrameExtractor{
			FFmpegPath: ffmpegPath, WorkRoot: cfg.Media.VideoWorkRoot,
			Sources: creativeMediaSource{repository: assetRepository, blobs: blobs},
		}
		creativeService.DerivedAssets = uploadService
	}
	if cfg.Creative.ShortDramaModelPlannerEnabled {
		textAdapter, textAdapterErr := buildTextAdapter(cfg, db)
		if textAdapterErr != nil {
			log.Fatalf("configure Creative short-drama planner: %v", textAdapterErr)
		}
		creativeService.ShortDramaPrerollPlanner = creative.FallbackShortDramaPrerollPlanner{
			Primary: creative.ModelShortDramaPrerollPlanner{
				Text:       &provider.Service{TextAdapter: textAdapter},
				ModelAlias: cfg.Creative.ShortDramaPlannerModelAlias,
			},
			Fallback: creative.DeterministicShortDramaPrerollPlanner{},
			OnPrimaryFailure: func(err error) {
				log.Printf("Creative short-drama model planning fell back to deterministic planning: %v", err)
			},
		}
		creativeService.ShortDramaV2Planner = creative.FallbackShortDramaV2Planner{
			Primary: creative.ModelShortDramaV2Planner{
				Text: &provider.Service{TextAdapter: textAdapter}, ModelAlias: cfg.Creative.ShortDramaPlannerModelAlias,
			},
			Fallback: creative.DeterministicShortDramaV2Planner{},
			OnPrimaryFailure: func(err error) {
				log.Printf("Creative short-drama V2 model planning fell back to deterministic planning: %v", err)
			},
		}
		log.Printf(
			"Creative short-drama planning configured: model_alias=%s fallback=deterministic",
			cfg.Creative.ShortDramaPlannerModelAlias,
		)
	} else {
		creativeService.ShortDramaPrerollPlanner = creative.DeterministicShortDramaPrerollPlanner{}
		creativeService.ShortDramaV2Planner = creative.DeterministicShortDramaV2Planner{}
	}
	if cfg.Creative.GamePrerollModelPlannerEnabled {
		textAdapter, textAdapterErr := buildTextAdapter(cfg, db)
		if textAdapterErr != nil {
			log.Fatalf("configure Creative game-preroll planner: %v", textAdapterErr)
		}
		creativeService.GamePrerollPlanner = creative.FallbackGamePrerollPlanner{
			Primary: creative.ModelGamePrerollPlanner{
				Text:       &provider.Service{TextAdapter: textAdapter},
				ModelAlias: cfg.Creative.GamePrerollPlannerModelAlias,
			},
			Fallback: creative.GenericGamePrerollPlanner{},
			OnPrimaryFailure: func(err error) {
				log.Printf("Creative game-preroll model planning fell back to deterministic planning: %v", err)
			},
		}
		log.Printf(
			"Creative game-preroll planning configured: model_alias=%s fallback=deterministic",
			cfg.Creative.GamePrerollPlannerModelAlias,
		)
	} else {
		creativeService.GamePrerollPlanner = creative.GenericGamePrerollPlanner{}
	}
	if cfg.Creative.BrandFilmModelPlannerEnabled {
		textAdapter, textAdapterErr := buildTextAdapter(cfg, db)
		if textAdapterErr != nil {
			log.Fatalf("configure Creative brand-film planner: %v", textAdapterErr)
		}
		creativeService.BrandFilmPlanner = creative.FallbackBrandFilmPlanner{
			Primary: creative.ModelBrandFilmPlanner{
				Text:       &provider.Service{TextAdapter: textAdapter},
				ModelAlias: cfg.Creative.BrandFilmPlannerModelAlias,
			},
			Fallback: creative.DeterministicBrandFilmPlanner{},
			OnPrimaryFailure: func(err error) {
				log.Printf("Creative brand-film model planning fell back to deterministic planning: %v", err)
			},
		}
		log.Printf(
			"Creative brand-film planning configured: model_alias=%s fallback=deterministic",
			cfg.Creative.BrandFilmPlannerModelAlias,
		)
	} else {
		creativeService.BrandFilmPlanner = creative.DeterministicBrandFilmPlanner{}
	}
	if cfg.Provider.AudioAdapter == "volcengine_asr" && cfg.Provider.TextAdapter == "adapter_gateway" {
		cipher, cipherErr := provider.NewAESGCMCredentialCipher(cfg.Provider.MasterKey, cfg.Provider.MasterKeyVersion)
		if cipherErr != nil {
			log.Fatalf("configure viral analysis credential encryption: %v", cipherErr)
		}
		gatewayConfig := provider.MySQLGatewayConfigStore{
			DB: db, Cipher: cipher, AllowInsecureHTTP: cfg.Provider.AllowInsecureHTTP,
		}
		analysisConfig := creativeprovider.ViralAnalyzerConfig{
			Assets: uploadService, Routes: gatewayConfig, Credentials: gatewayConfig,
			FFmpegPath: ffmpegPath, WorkRoot: cfg.Media.VideoWorkRoot,
			ModelAlias: "cookies.text.standard", PromptVersion: "viral.analyze.v1",
			AllowInsecureHTTP: cfg.Provider.AllowInsecureHTTP,
			ASR: creativeprovider.ASRConfig{
				Endpoint: cfg.Provider.VolcengineASR.Endpoint, AuthMode: cfg.Provider.VolcengineASR.AuthMode,
				AppID: cfg.Provider.VolcengineASR.AppID, AccessToken: cfg.Provider.VolcengineASR.AccessToken,
				APIKey: cfg.Provider.VolcengineASR.APIKey, ResourceID: cfg.Provider.VolcengineASR.ResourceID,
				Model: cfg.Provider.VolcengineASR.Model,
			},
		}
		analyzer, analyzerErr := creativeprovider.NewViralAnalyzer(analysisConfig)
		if analyzerErr != nil {
			log.Fatalf("configure viral reference analyzer: %v", analyzerErr)
		}
		creativeService.ViralAnalyzer = analyzer
		shortDramaAnalyzer, shortDramaAnalyzerErr := creativeprovider.NewShortDramaV2Analyzer(analysisConfig)
		if shortDramaAnalyzerErr != nil {
			log.Fatalf("configure short drama V2 analyzer: %v", shortDramaAnalyzerErr)
		}
		creativeService.ShortDramaV2Analyzer = shortDramaAnalyzer
		commerceAnalyzer, commerceAnalyzerErr := creativeprovider.NewCommercePrerollV2Analyzer(analysisConfig)
		if commerceAnalyzerErr != nil {
			log.Fatalf("configure commerce preroll V2 analyzer: %v", commerceAnalyzerErr)
		}
		creativeService.CommercePrerollV2Analyzer = commerceAnalyzer
		gameAnalyzer, gameAnalyzerErr := creativeprovider.NewGamePrerollV2Analyzer(analysisConfig)
		if gameAnalyzerErr != nil {
			log.Fatalf("configure game preroll V2 analyzer: %v", gameAnalyzerErr)
		}
		creativeService.GamePrerollV2Analyzer = gameAnalyzer
		log.Printf("Creative viral analysis configured: model_alias=%s prompt_version=%s asr=%s", "cookies.text.standard", "viral.analyze.v1", cfg.Provider.VolcengineASR.ResourceID)
	}
	runtimeStore := jobruntime.MySQLStore{DB: db}
	creativeService.DirectionScheduler = creative.JobRuntimeDirectionGenerationScheduler{Store: runtimeStore}
	creativeService.AINativeOperationCanceller = creativeAINativeOperationCanceller{store: runtimeStore}
	var researchRunner knowledge.ExternalResearchRunner
	var researchRouteInspector strategysystem.ResearchRouteInspector
	if cfg.Research.SeedEnabled {
		cipher, cipherErr := provider.NewAESGCMCredentialCipher(
			cfg.Provider.MasterKey, cfg.Provider.MasterKeyVersion,
		)
		if cipherErr != nil {
			log.Fatalf("configure Seed web research credential encryption: %v", cipherErr)
		}
		gatewayConfig := provider.MySQLGatewayConfigStore{
			DB: db, Cipher: cipher, AllowInsecureHTTP: cfg.Provider.AllowInsecureHTTP,
		}
		seedResearchClient := &seedresearch.Client{
			Routes: gatewayConfig, Credentials: gatewayConfig,
			ModelAlias: cfg.Research.SeedModelAlias, MaxConcurrent: cfg.Research.MaxConcurrent,
			AllowInsecureHTTP: cfg.Provider.AllowInsecureHTTP,
		}
		researchRunner = seedResearchClient
		researchRouteInspector = seedResearchClient
		log.Printf("Knowledge research configured: transport=ark_responses tool=web_search model_alias=%s",
			cfg.Research.SeedModelAlias)
	}
	knowledgeService := &knowledge.Service{
		DB: db, Projects: projectService, Blobs: blobs, Scanner: scanner,
		AssetsBucket: cfg.ObjectStorage.AssetsBucket, Runner: researchRunner,
		JobProgress: runtimeStore, JobCanceller: runtimeStore,
	}
	if researchRunner != nil {
		knowledgeService.SourceVerifier = knowledge.SafeHTTPResearchSourceVerifier{Timeout: 8 * time.Second}
	}
	if cfg.Research.TikaEnabled {
		knowledgeService.DocumentParser = knowledge.TikaParser{
			BaseURL: cfg.Research.TikaBaseURL, Version: cfg.Research.TikaVersion,
			Timeout:        time.Duration(cfg.Research.TikaTimeoutSeconds) * time.Second,
			MaxOutputBytes: int64(cfg.Research.TikaMaxOutputBytes),
		}
		knowledgeService.DocumentScheduler = knowledge.JobRuntimeDocumentParseScheduler{
			Store: runtimeStore, NewID: func() (string, error) { return ids.New("documentparsejob") },
		}
		log.Printf("Knowledge document parsing configured: parser=tika version=%s", cfg.Research.TikaVersion)
	}
	if cfg.Research.DocumentVisionEnabled {
		cipher, cipherErr := provider.NewAESGCMCredentialCipher(cfg.Provider.MasterKey, cfg.Provider.MasterKeyVersion)
		if cipherErr != nil {
			log.Fatalf("configure LAS document vision credential encryption: %v", cipherErr)
		}
		gatewayConfig := provider.MySQLGatewayConfigStore{
			DB: db, Cipher: cipher, AllowInsecureHTTP: cfg.Provider.AllowInsecureHTTP,
		}
		knowledgeService.DocumentVision = &lasdocument.Client{
			Routes: gatewayConfig, Credentials: gatewayConfig,
			SourceURLs:   blobs,
			OutputBucket: cfg.ObjectStorage.AssetsBucket, OutputPrefix: "provider-output/document-vision",
			AllowInsecureHTTP: cfg.Provider.AllowInsecureHTTP,
		}
		knowledgeService.VisionModelAlias = cfg.Research.DocumentVisionModelAlias
		knowledgeService.VisionScheduler = knowledge.JobRuntimeDocumentVisionFallbackScheduler{
			Store: runtimeStore, NewID: func() (string, error) { return ids.New("documentvisionjob") },
		}
		log.Printf("Knowledge document vision configured: provider=volcengine_las model_alias=%s input=tos output=tos",
			cfg.Research.DocumentVisionModelAlias)
	}
	if cfg.Research.DocumentConverterEnabled {
		knowledgeService.DocumentConverter = &gotenberg.Client{
			BaseURL: cfg.Research.DocumentConverterBaseURL, Version: cfg.Research.DocumentConverterVersion,
			Timeout:           time.Duration(cfg.Research.DocumentConverterTimeout) * time.Second,
			MaxPDFBytes:       int64(cfg.Research.DocumentConverterMaxPDFBytes),
			AllowInsecureHTTP: cfg.Research.DocumentConverterAllowHTTP,
		}
		log.Printf("Knowledge presentation conversion configured: converter=gotenberg_libreoffice version=%s",
			cfg.Research.DocumentConverterVersion)
	}
	if researchRunner != nil {
		knowledgeService.Scheduler = knowledge.JobRuntimeResearchScheduler{
			Store: runtimeStore, NewID: func() (string, error) { return ids.New("researchjob") },
		}
	}
	visionAdapter, err := buildVisionAdapter(cfg, db)
	if err != nil {
		log.Fatalf("configure Provider vision adapter: %v", err)
	}
	var visionProvider *provider.Service
	if visionAdapter != nil {
		visionProvider = &provider.Service{VisionAdapter: visionAdapter, VisionSources: assetVisionSourceResolver{uploads: uploadService}}
	}
	log.Printf("Media understanding configured: real_provider=%t vision_model_alias=%s asr=%t", cfg.MediaUnderstanding.RealProviderEnabled, cfg.MediaUnderstanding.VisionModelAlias, cfg.MediaUnderstanding.ASREnabled)
	mediaUnderstandingService := &mediaunderstanding.Service{
		Store: mediaunderstanding.MySQLStore{DB: db}, Projects: projectService, Assets: uploadService,
		DerivedImages: uploadService, Vision: visionProvider, RealVision: cfg.MediaUnderstanding.RealProviderEnabled,
		ModelAlias: cfg.MediaUnderstanding.VisionModelAlias,
		Scheduler: mediaunderstanding.JobRuntimeScheduler{
			Store: runtimeStore, NewID: func() (string, error) { return ids.New("mediaunderstandingjob") },
		},
	}
	if ffmpegPath != "" {
		mediaUnderstandingService.Frames = media.FFmpegFrameExtractor{
			FFmpegPath: ffmpegPath, WorkRoot: cfg.Media.VideoWorkRoot,
			Sources: creativeMediaSource{repository: assetRepository, blobs: blobs},
		}
	}
	if cfg.MediaUnderstanding.ASREnabled && ffmpegPath != "" {
		mediaUnderstandingService.Transcriber = creativeprovider.AssetTranscriber{
			Assets: uploadService, FFmpegPath: ffmpegPath, WorkRoot: cfg.Media.VideoWorkRoot,
			ASR: creativeprovider.VolcengineASR{Config: creativeprovider.ASRConfig{
				Endpoint: cfg.Provider.VolcengineASR.Endpoint, AuthMode: cfg.Provider.VolcengineASR.AuthMode,
				AppID: cfg.Provider.VolcengineASR.AppID, AccessToken: cfg.Provider.VolcengineASR.AccessToken,
				APIKey: cfg.Provider.VolcengineASR.APIKey, ResourceID: cfg.Provider.VolcengineASR.ResourceID,
				Model: cfg.Provider.VolcengineASR.Model,
			}},
		}
		log.Printf("Media understanding ASR configured: adapter=volcengine_asr model=%s", cfg.Provider.VolcengineASR.Model)
	}
	remixService := remix.NewMemoryService(func() (string, error) { return ids.New("remixplan") })
	agentService := agent.NewMemoryService(remixService, func(prefix string) (string, error) { return ids.New(prefix) })
	dependencies := httpserver.Dependencies{
		Resolver:          resolver,
		ProjectAuthorizer: projectStore,
		Readiness:         database.Readiness{DB: db},
		Identities:        identityStore, Accounts: identityStore, Projects: projectService, ProjectMembers: projectStore,
		Uploads: uploadService, Blobs: blobs, Intakes: intakeService, Creative: creativeService, ProductionCenter: productionCenter, ProductionAssets: productionCenter, ProductionRetry: productionRetry,
		Sessions: sessionService, Knowledge: knowledgeService,
		RemixPlans: remixService, Evals: remixService, AgentRuns: agentService,
		ProviderConfig: provider.MySQLGatewayConfigStore{DB: db},
	}
	dependencies.AuthenticatedDomainMounts = append(dependencies.AuthenticatedDomainMounts,
		httpserver.DomainMount{Pattern: "/api/media/v1/", Handler: mediaunderstandinghttp.New(*mediaUnderstandingService)})
	connectorRepository := connector.MySQLRepository{DB: db}
	deliveryService := &delivery.Service{
		Repository:              delivery.MySQLRepository{DB: db},
		Projects:                projectService,
		ConnectorSnapshots:      connectorRepository,
		ConnectorAccounts:       connectorRepository,
		ExternalAccountIDs:      connectorRepository,
		LaunchBatchCalibrations: connectorRepository,
		// The Connector is not configured in this environment. Normalize the
		// deterministic OutcomeSimulation records through the Delivery consumer
		// port until its future adapter publishes a stable contract.
		Insights: delivery.SimulationInsightsReader{Repository: delivery.MySQLRepository{DB: db}},
	}
	dependencies.AuthenticatedDomainMounts = append(dependencies.AuthenticatedDomainMounts,
		httpserver.DomainMount{Pattern: "/api/delivery/v1/", Handler: deliveryhttp.New(deliveryService)})
	browserRpaRepository := browserautomation.MySQLRepository{DB: db}
	deliveryAuthorityProvider := delivery.BrowserRpaAuthorityProvider{Repository: delivery.MySQLRepository{DB: db}}
	browserRpaService := browserautomation.Service{
		Repository:        browserRpaRepository,
		AuthorityProvider: deliveryAuthorityProvider,
		NewID:             func(prefix string) (string, error) { return ids.New(prefix) },
	}
	deliveryService.BrowserRpaLauncher = deliveryBrowserRpaLauncher{service: browserRpaService, executionDriver: browserautomation.ExecutionDriverOceanEngineWebAPI}
	browserRpaServer := browserautomationhttp.NewTakeoverOnly(browserRpaService, projectStore)
	v3Compiler := plancompile.V3Compiler{Source: delivery.MySQLRepository{DB: db}, AccountResolver: connectorRepository, PlatformObjects: connectorRepository}
	var oceanEngineWriterFactory oceanEngineConnectorWriterFactory
	if cfg.OceanEngine.Enabled {
		oceanEngineCipher, cipherErr := insights.NewAESGCMSecretCipher(cfg.OceanEngine.MasterKey, cfg.OceanEngine.MasterKeyVersion)
		if cipherErr != nil {
			log.Fatalf("configure Ocean Engine session cipher: %v", cipherErr)
		}
		oceanEngineWriterFactory = oceanEngineConnectorWriterFactory{
			accountSessions: connectorRepository, accounts: connectorRepository, cipher: oceanEngineCipher,
			baseURL: cfg.OceanEngine.BaseURL, client: &http.Client{Timeout: 30 * time.Second},
			tokenCache: oceanengine.NewCSRFTokenCache(),
		}
	}
	driverAdapters := map[browserautomation.ExecutionDriver]browserautomation.WorkerAdapter{
		browserautomation.ExecutionDriverOceanEngineWebAPI: webapiadapter.Adapter{
			Compiler: v3Compiler, Policies: browserRpaRepository,
			Sessions:     oceanEngineWebAPISessionChecker{accountSessions: connectorRepository, accounts: connectorRepository},
			WriteEnabled: cfg.OceanEngine.WebAPIWriteEnabled, AccountAllowlist: cfg.OceanEngine.WebAPIWriteAccounts,
			PayloadSource:  deliveryAuthorityProvider,
			Templates:      fileTemplateSource{Path: cfg.OceanEngine.WebAPITemplateFile},
			SessionFactory: oceanEngineWriterFactory,
		},
	}
	var playwrightAdapter browserautomation.WorkerAdapter
	if cfg.BrowserRPA.Enabled {
		manifest, err := calibrationmanifest.Current()
		if err != nil {
			log.Fatalf("load OceanEngine calibration manifest: %v", err)
		}
		adapter := rparunner.NewPlaywrightRPAAdapter(rparunner.AdapterConfig{
			Protocol:            cfg.BrowserRPA.RunnerProtocol,
			Command:             strings.Fields(cfg.BrowserRPA.Command),
			ScriptPath:          cfg.BrowserRPA.ScriptPath,
			WorkDir:             ".",
			EvidenceRoot:        cfg.BrowserRPA.EvidenceRoot,
			EdgeSessionFile:     cfg.BrowserRPA.EdgeSessionFile,
			SessionProbeScript:  cfg.BrowserRPA.SessionProbeScript,
			AuthorityStateRoot:  cfg.BrowserRPA.AuthorityStateRoot,
			V3Compiler:          v3Compiler,
			PrepareTimeout:      time.Duration(cfg.BrowserRPA.PrepareTimeoutSeconds) * time.Second,
			SubmitTimeout:       time.Duration(cfg.BrowserRPA.SubmitTimeoutSeconds) * time.Second,
			FallbackCDPEndpoint: cfg.BrowserRPA.CDPEndpointFallback,
		}, browserRpaRepository, browserRpaService, plancompile.Compiler{Manifest: manifest})
		playwrightAdapter = adapter
		driverAdapters[browserautomation.ExecutionDriverPlaywrightEdgeV3] = adapter
		log.Printf("browser_rpa_automated_worker=true runner_protocol=%s", cfg.BrowserRPA.RunnerProtocol)
	}
	if cfg.OceanEngine.Enabled || cfg.BrowserRPA.Enabled {
		browserRpaServer = browserautomationhttp.New(browserRpaService, browserautomation.Worker{Service: browserRpaService, Adapter: playwrightAdapter, DriverAdapters: driverAdapters}, projectStore)
	}
	browserRpaServer.MountLegacyAlias()
	dependencies.AuthenticatedDomainMounts = append(dependencies.AuthenticatedDomainMounts,
		httpserver.DomainMount{Pattern: "/api/platform/v1/browser-rpa/", Handler: browserRpaServer},
		httpserver.DomainMount{Pattern: "/api/platform/v1/computer-use/", Handler: browserRpaServer})
	// 文本模型出口。Strategy 和 Insights 共用同一个网关适配器和同一个能力别名——
	// 它们要的是同一件事：调一次文本模型。**目前也共用同一个开关**
	// （COOKIES_STRATEGY_REAL_PROVIDER_ENABLED），这是个遗留：
	// 想单独关掉素材洞察的提取而留着策略生成，现在做不到。
	var textProvider *provider.Service
	if cfg.Strategy.RealProviderEnabled {
		textAdapter, err := buildTextAdapter(cfg, db)
		if err != nil {
			log.Fatalf("configure Provider text adapter: %v", err)
		}
		textProvider = &provider.Service{TextAdapter: textAdapter}
	}
	if textProvider != nil {
		creativeService.AINativeRequirementPlanner = creative.ModelAINativeRequirementPlanner{Text: textProvider, ModelAlias: cfg.Strategy.TextModelAlias}
		creativeService.AINativeScriptPlanner = creative.ModelAINativeScriptPlanner{Text: textProvider, ModelAlias: cfg.Strategy.TextModelAlias}
		creativeService.AINativeStoryboardPlanner = creative.ModelAINativeStoryboardPlanner{Text: textProvider, ModelAlias: cfg.Strategy.TextModelAlias}
		creativeService.AINativeVoiceoverFitter = creative.ModelAINativeVoiceoverFitter{Text: textProvider, ModelAlias: cfg.Strategy.TextModelAlias}
	}
	var miyunCipher insights.MiyunSecretCipher
	var sessionCipher insights.SecretCipher
	var miyunPages insights.MiyunPageClient
	var oceanEngineVerifier insights.OceanEngineSessionVerifier
	if cfg.OceanEngine.Enabled {
		cipher, cipherErr := insights.NewAESGCMSecretCipher(cfg.OceanEngine.MasterKey, cfg.OceanEngine.MasterKeyVersion)
		if cipherErr != nil {
			log.Fatalf("configure Ocean Engine session encryption: %v", cipherErr)
		}
		sessionCipher = cipher
		oceanEngineVerifier = oceanEngineSessionVerifier{baseURL: cfg.OceanEngine.BusinessBaseURL, client: &http.Client{Timeout: 30 * time.Second}}
		log.Printf("Ocean Engine read-only session verification configured: base_url=%s", cfg.OceanEngine.BusinessBaseURL)
	}
	var miyunVerifier insights.MiyunConnectionVerifier
	var miyunImports insights.MiyunAuthorizedImporter
	var miyunPreviews insights.MiyunAuthorizedPreviewer
	if cfg.Miyun.Enabled {
		cipher, cipherErr := insights.NewAESGCMMiyunSecretCipher(cfg.Miyun.MasterKey, cfg.Miyun.MasterKeyVersion)
		if cipherErr != nil {
			log.Fatalf("configure Miyun secret encryption: %v", cipherErr)
		}
		if uploadService.VideoProbe == nil && uploadService.MediaProbe == nil {
			log.Fatalf("configure Miyun external import: ffprobe or media probe is required")
		}
		gate := &crawler.YouShuGate{
			MaxConcurrent: cfg.Miyun.MaxConcurrent, RequestsPerSecond: cfg.Miyun.RequestsPerSecond,
			Cooldown: time.Duration(cfg.Miyun.CooldownSeconds) * time.Second,
		}
		protocol := miyunProtocolAdapter{endpoint: cfg.Miyun.Endpoint, cipher: cipher, client: &http.Client{Timeout: 30 * time.Second}, gate: gate}
		externalImports := assets.ExternalImportService{
			Repository: assetRepository, Projects: projectService, Upload: *uploadService,
			QuarantineBucket: cfg.ObjectStorage.QuarantineBucket,
		}
		miyunCipher, miyunPages, miyunVerifier = cipher, protocol, protocol
		miyunImports = miyunAuthorizedImportAdapter{
			downloader: &crawler.YouShuDownloader{HTTPClient: &http.Client{Timeout: 2 * time.Minute}, AllowedHosts: cfg.Miyun.DownloadAllowedHosts},
			assets:     externalImports, ledger: assetRepository, workRoot: miyunWorkRoot(cfg.Media.VideoWorkRoot),
		}
		miyunPreviews = miyunAuthorizedPreviewAdapter{
			downloader: &crawler.YouShuDownloader{HTTPClient: &http.Client{Timeout: 2 * time.Minute}, AllowedHosts: cfg.Miyun.DownloadAllowedHosts},
			workRoot:   miyunWorkRoot(cfg.Media.VideoWorkRoot),
		}
		log.Printf("Miyun collection configured: real_calls=true concurrency=%d rate=%d cooldown_seconds=%d download_hosts=%d",
			cfg.Miyun.MaxConcurrent, cfg.Miyun.RequestsPerSecond, cfg.Miyun.CooldownSeconds, len(cfg.Miyun.DownloadAllowedHosts))
	}
	insightsService := &insights.Service{
		Repository:     insights.MySQLRepository{DB: db},
		Assets:         insights.MySQLRepository{DB: db},
		ExternalAssets: insights.MySQLRepository{DB: db},
		Connectors:     insights.MySQLRepository{DB: db},
		Runs:           insights.MySQLRepository{DB: db},
		Experiments:    insights.MySQLRepository{DB: db},
		Thresholds:     insights.MySQLRepository{DB: db},
		Projects:       projectService,
		Delivery:       deliveryinsights.Reader{Service: deliveryService},
		Media:          insightMediaReader{uploads: uploadService},
		// 视频类提取走多模态。为 nil 时视频类退回人填画面描述那条路——
		// 这里永远不为 nil（媒体理解服务总是构造出来的），但它内部的视觉链路
		// 可能没接通，那种情况由 UnderstandMedia 自己识别并回落。
		Understanding: insightMediaUnderstander{service: mediaUnderstandingService},

		// 米云素材（来自上游 shikanon/cookies）。
		Miyun:               insights.MySQLRepository{DB: db},
		MiyunProjects:       miyunProjectSourceAdapter{projects: projectService},
		MiyunAssets:         miyunAssetSourceAdapter{uploads: uploadService},
		MiyunKnowledge:      miyunKnowledgeSourceAdapter{knowledge: knowledgeService},
		MiyunHandoffContent: miyunHandoffContentAdapter{uploads: uploadService, knowledge: knowledgeService},
		MiyunMedia:          miyunMediaEvidenceAdapter{media: mediaUnderstandingService},
		MiyunCrawl:          insights.MySQLRepository{DB: db},
		MiyunJobs:           runtimeStore,
		MiyunPages:          miyunPages,
		MiyunImports:        miyunImports,
		MiyunReturns:        miyunReturnImportAdapter{imports: assets.ExternalImportService{Repository: assetRepository, Projects: projectService, Upload: *uploadService, QuarantineBucket: cfg.ObjectStorage.QuarantineBucket}, uploads: *uploadService},
		MiyunPreviews:       miyunPreviews,
		MiyunSecrets:        miyunCipher,
		SessionSecrets:      sessionCipher,
		OceanEngineSessions: insights.MySQLRepository{DB: db},
		OceanEngineVerifier: oceanEngineVerifier,
		MiyunVerifier:       miyunVerifier,
		MiyunCooldown:       time.Duration(cfg.Miyun.CooldownSeconds) * time.Second,
	}
	// Text 为 nil 时提取会直接失败，不会退化成模板产出——
	// 库里一条编造的特征，代价远大于一次失败的提取。
	if textProvider != nil {
		insightsService.Text = textProvider
		insightsService.TextModelAlias = cfg.Strategy.TextModelAlias
	}
	dependencies.AuthenticatedDomainMounts = append(dependencies.AuthenticatedDomainMounts,
		httpserver.DomainMount{Pattern: "/api/insights/v1/", Handler: insightshttp.New(insightsService)})
	var connectorPatrol *connector.PatrolRunner
	if cfg.OceanEngine.Enabled && sessionCipher != nil {
		connectorRepository.Cipher = sessionCipher
		connectorSync := connector.Synchronizer{
			Writer: connectorRepository,
			Readers: oceanEngineConnectorReaderFactory{
				sessions:        insightsService.OceanEngineSessions,
				accountSessions: connectorRepository,
				cipher:          sessionCipher,
				baseURL:         cfg.OceanEngine.BaseURL,
				client:          &http.Client{Timeout: 30 * time.Second},
				accounts:        connectorRepository,
			},
			Cipher: sessionCipher,
		}
		connectorAccountSessions := connector.AccountSessionService{Store: connectorRepository, Cipher: sessionCipher}
		connectorAccounts := connector.AccountService{Store: connectorRepository, Sessions: connectorRepository, Probe: oceanEngineAccountProbe{accountSessions: connectorRepository, cipher: sessionCipher, baseURL: cfg.OceanEngine.BaseURL, client: &http.Client{Timeout: 30 * time.Second}}}
		if cfg.OceanEngine.PatrolEnabled {
			connectorPatrol = &connector.PatrolRunner{Sessions: connectorRepository, Syncer: connectorSync, LookbackDays: cfg.OceanEngine.PatrolLookbackDays, Timeout: 15 * time.Minute}
		}
		dependencies.AuthenticatedDomainMounts = append(dependencies.AuthenticatedDomainMounts,
			httpserver.DomainMount{Pattern: "/api/connector/v1/", Handler: connectorhttp.New(connectorRepository, connectorSync, projectStore, connectorAccounts, connectorAccountSessions)})
	}
	workerContext, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	if connectorPatrol != nil {
		startPeriodicWorker(workerContext, "ocean-engine-metric-patrol", time.Duration(cfg.OceanEngine.PatrolIntervalHours)*time.Hour, func(ctx context.Context) error {
			result, err := connectorPatrol.RunOnce(ctx)
			if err == nil {
				log.Printf("ocean-engine-metric-patrol completed: accounts=%d completed=%d", result.AccountCount, result.Completed)
			}
			return err
		})
	}
	if knowledgeService.DocumentScheduler != nil || knowledgeService.Scheduler != nil || knowledgeService.VisionScheduler != nil {
		knowledgeReconciler := knowledge.JobStateReconciler{Service: knowledgeService, Limit: 20}
		startWorker(workerContext, "knowledge-job-reconcile", knowledgeReconciler.RunOnce)
	}
	agentStore := agent.MySQLStore{DB: db}
	runtimeHandlers := map[string]jobruntime.Handler{}
	if cfg.Miyun.Enabled {
		runtimeHandlers[insights.MiyunCrawlJobKind] = insightsService.HandleMiyunCrawlJob
		runtimeHandlers[insights.MiyunMaterialImportJobKind] = insightsService.HandleMiyunMaterialImportJob
	}
	runtimeHandlers[creative.DirectionGenerationJobKind] = creativeService.HandleDirectionGenerationJob
	creativeService.AINativeScriptScheduler = creative.JobRuntimeAINativeScriptScheduler{
		Store: runtimeStore,
	}
	runtimeHandlers[creative.AINativeScriptJobKind] = creativeService.HandleAINativeScriptJob
	runtimeHandlers[mediaunderstanding.JobKind] = mediaUnderstandingService.HandleJob
	if researchRunner != nil {
		runtimeHandlers[knowledge.ResearchJobKind] = knowledgeService.HandleResearchJob
	}
	if knowledgeService.DocumentParser != nil {
		runtimeHandlers[knowledge.DocumentParseJobKind] = knowledgeService.HandleDocumentParseJob
	}
	if knowledgeService.DocumentVision != nil {
		runtimeHandlers[knowledge.DocumentVisionFallbackJobKind] = knowledgeService.HandleDocumentVisionFallbackJob
	}
	creativeService.RenderScheduler = creative.JobRuntimeRenderScheduler{
		Store: runtimeStore, NewID: func() (string, error) { return ids.New("creativerenderexec") },
	}
	creativeService.EditingRenderScheduler = creative.JobRuntimeEditingRenderScheduler{
		Store: runtimeStore, NewID: func() (string, error) { return ids.New("editingrenderexec") },
	}
	creativeService.AudioMixScheduler = creative.JobRuntimeAudioMixRenderScheduler{
		Store: runtimeStore, NewID: func() (string, error) { return ids.New("audiomixrenderexec") },
	}
	if ffmpegPath != "" && ffprobePath != "" {
		probe := assets.FFprobeVideoProbe{Path: ffprobePath, WorkRoot: cfg.Media.VideoWorkRoot}
		composer := media.FFmpegComposer{
			FFmpegPath: ffmpegPath, WorkRoot: cfg.Media.VideoWorkRoot,
			Sources: creativeMediaSource{repository: assetRepository, blobs: blobs}, Probe: probe,
		}
		creativeService.ShortDramaV2OutputNormalizer = composer
		creativeService.CommercePrerollV2OutputNormalizer = composer
		creativeService.Composer = composer
		creativeService.BrandFilmComposer = composer
		creativeService.RenderedAssets = creativeRenderedAssetWriter{uploads: uploadService}
		creativeService.AudioMixRenderer = media.FFmpegAudioMixRenderer{
			FFmpegPath: ffmpegPath, WorkRoot: cfg.Media.VideoWorkRoot,
			Videos: creativeMediaSource{repository: assetRepository, blobs: blobs},
			Audio:  creativeMediaSource{repository: assetRepository, blobs: blobs}, Probe: probe,
		}
		creativeService.AINativeTimelineRenderer = media.FFmpegTimelineRenderer{
			FFmpegPath: ffmpegPath, WorkRoot: cfg.Media.VideoWorkRoot,
			Videos:  creativeMediaSource{repository: assetRepository, blobs: blobs},
			Visuals: creativeMediaSource{repository: assetRepository, blobs: blobs},
			Audio:   creativeMediaSource{repository: assetRepository, blobs: blobs}, Probe: probe,
		}
		runtimeHandlers[creative.AudioMixRenderJobKind] = creative.AudioMixRenderRuntimeHandler(*creativeService)
		for kind, handler := range creative.NewRenderRuntimeWorker(runtimeStore, *creativeService).Handlers {
			runtimeHandlers[kind] = handler
		}
		runtimeHandlers["creative.editing.render"] = creative.EditingRenderRuntimeHandler(*creativeService)
	}
	if cfg.Strategy.Enabled {
		productEventWriter := strategysystem.MySQLProductEventWriter{DB: db}
		strategyService := strategysystem.Service{
			DB: db, Projects: projectService, Knowledge: knowledgeService, ConversationKnowledge: knowledgeService,
			ConversationResearch: knowledgeService, ResearchRoutes: researchRouteInspector,
			DocumentVisionRoutes: knowledgeService.DocumentVision,
			ConversationMedia:    mediaUnderstandingService,
			CreativeAssets:       uploadService, Agents: agentStore,
			ProductEvents: productEventWriter, Text: textProvider,
			TextModelAlias: cfg.Strategy.TextModelAlias, LiteTextModelAlias: cfg.Strategy.LiteTextModelAlias,
			DeepReviewModelAlias: cfg.Strategy.DeepReviewModelAlias,
			ResearchModelAlias:   cfg.Research.SeedModelAlias, DocumentVisionModelAlias: cfg.Research.DocumentVisionModelAlias,
			PromptVersion:             cfg.Strategy.PromptVersion,
			ConversationPromptVersion: cfg.Strategy.ConversationPromptVersion,
			RevisePromptVersion:       cfg.Strategy.RevisePromptVersion,
			ReviewPromptVersion:       cfg.Strategy.ReviewPromptVersion,
			RepairPromptVersion:       cfg.Strategy.RepairPromptVersion,
			CreativeTaskPromptVersion: cfg.Strategy.CreativeTaskPromptVersion,
			CriticEnabled:             cfg.Strategy.CriticEnabled, V2Enabled: cfg.Strategy.V2Enabled,
			CreativeTaskPlanningEnabled:  cfg.Strategy.CreativeTaskPlanningEnabled,
			QuickViralRemakeEnabled:      cfg.Strategy.QuickViralRemakeEnabled,
			ConversationWebSearchEnabled: researchRunner != nil,
			ContextSelectionEnabled:      cfg.Strategy.ContextSelectionEnabled,
			DisableApproval:              !cfg.Strategy.ApproveEnabled,
			AllowedOrganizations:         strategyOrganizationAllowlist(cfg.Strategy.OrganizationAllowlist),
		}
		knowledgeService.DocumentEvents = strategysystem.KnowledgeDocumentProductEventSink{
			Writer: productEventWriter, NewID: func() (string, error) { return ids.New("strategyproductevent") },
		}
		knowledgeService.ResearchCompletion = strategyService
		if err := strategyService.EnsureCreativeBusinessCatalog(context.Background()); err != nil {
			log.Fatalf("seed Strategy creative business catalog: %v", err)
		}
		generationMode := "deterministic"
		if cfg.Strategy.RealProviderEnabled {
			generationMode = "provider"
		}
		log.Printf(
			"Strategy generation configured: mode=%s model_alias=%s prompt_version=%s critic_enabled=%t context_selection_enabled=%t",
			generationMode, cfg.Strategy.TextModelAlias, cfg.Strategy.PromptVersion, cfg.Strategy.CriticEnabled,
			cfg.Strategy.ContextSelectionEnabled,
		)
		// This adapter is the only Strategy-to-Creative connection. It reads an
		// immutable, authorized Strategy package and leaves Creative to persist
		// its own Intake only after a user explicitly invokes the endpoint.
		strategyCreativeReader := strategycreative.Reader{Service: strategyService}
		creativeService.Sources = strategyCreativeReader
		creativeService.Requirements = strategyCreativeReader
		if cfg.Strategy.CreativeTaskPlanningEnabled {
			creativeService.TaskStrategies = strategyCreativeReader
			creativeService.TaskOverlays = strategyCreativeReader
		}
		if cfg.Strategy.PackageToCreativeEnabled {
			creativeService.StrategyPackages = strategyCreativeReader
		}
		strategyAPI := strategyhttp.New(strategyService, agentStore, runtimeStore)
		dependencies.AuthenticatedDomainMounts = append(dependencies.AuthenticatedDomainMounts,
			httpserver.DomainMount{Pattern: "/api/strategy/v1/", Handler: strategyAPI})
		strategyHandler := agent.RuntimeHandlerWithFinalFailure(
			agentStore, strategyService.HandleAgentTask, strategyService.HandleAgentTaskFinalFailure, runtimeStore,
		)
		runtimeHandlers[strategysystem.AgentKindBriefExtract] = strategyHandler
		runtimeHandlers[strategysystem.AgentKindDraftGenerate] = strategyHandler
		runtimeHandlers[strategysystem.AgentKindDraftRevise] = strategyHandler
		runtimeHandlers[strategysystem.AgentKindReviewDeep] = strategyHandler
		runtimeHandlers[strategysystem.AgentKindCreativeTaskGenerate] = strategyHandler
		agentDispatcher := agent.Dispatcher{DB: db, Jobs: runtimeStore}
		startWorker(workerContext, "agent-dispatch", agentDispatcher.RunOnce)
	}
	if cfg.Environment == config.EnvironmentLocal || cfg.Provider.ImageAdapter == "adapter_gateway" || cfg.Provider.VideoAdapter == "adapter_gateway" {
		adapter, outputHandles, err := buildImageAdapter(cfg, db, blobs)
		if err != nil {
			log.Fatalf("configure Provider image adapter: %v", err)
		}
		videoAdapter, err := buildVideoAdapter(cfg, db, outputHandles)
		if err != nil {
			log.Fatalf("configure Provider video adapter: %v", err)
		}
		providerStore := provider.MySQLStore{DB: db, AllowInsecureHTTP: cfg.Provider.AllowInsecureHTTP}
		providerService := provider.Service{
			Store:         providerStore,
			JobQueryStore: providerStore,
			Scheduler:     provider.JobRuntimeScheduler{Store: runtimeStore, NewID: func() (string, error) { return ids.New("providerexec") }},
			ImageAdapter:  adapter,
			VideoAdapter:  videoAdapter,
			VisionSources: assetVisionSourceResolver{uploads: uploadService},
			Intake:        provider.AssetsIntakeClient{API: intakeService},
			OutputHandles: outputHandles,
			NewID:         func() (string, error) { return ids.New("providerjob") },
		}
		if cfg.Provider.ImageAdapter == "adapter_gateway" {
			cipher, cipherErr := provider.NewAESGCMCredentialCipher(cfg.Provider.MasterKey, cfg.Provider.MasterKeyVersion)
			if cipherErr != nil {
				log.Fatalf("configure Provider credential encryption: %v", cipherErr)
			}
			providerService.Routes = provider.MySQLGatewayConfigStore{DB: db, Cipher: cipher, AllowInsecureHTTP: cfg.Provider.AllowInsecureHTTP}
		}
		if cfg.Provider.VideoAdapter == "adapter_gateway" {
			cipher, cipherErr := provider.NewAESGCMCredentialCipher(cfg.Provider.MasterKey, cfg.Provider.MasterKeyVersion)
			if cipherErr != nil {
				log.Fatalf("configure Provider video credential encryption: %v", cipherErr)
			}
			videoRoutes := provider.MySQLGatewayConfigStore{
				DB: db, Cipher: cipher, AllowInsecureHTTP: cfg.Provider.AllowInsecureHTTP,
				VideoConnectionType: "adapter_gateway",
			}
			resolved, resolveErr := videoRoutes.ResolveVideoRoute(context.Background(), "", "cookies.video.standard")
			if resolveErr != nil {
				log.Fatalf("resolve Adapter-only cookies.video.standard route: %v", resolveErr)
			}
			if resolved.ConnectionType != "adapter_gateway" {
				log.Fatalf("cookies.video.standard resolved forbidden video connection type %q", resolved.ConnectionType)
			}
			providerService.VideoRoutes = videoRoutes
		}
		dependencies.ProviderJobs = providerService
		productionCenter.Sources = append(productionCenter.Sources, creative.ProviderRunAdapter{Jobs: &providerService})
		imageRetryAdapter := creativeprovider.ImageSlotProductionRetryAdapter{Creative: creativeService, Attempts: creativeRepository, Provider: &providerService, Projects: projectService}
		productionRetryAdapters = append(productionRetryAdapters, imageRetryAdapter)
		productionCenter.RetryAdapters = productionRetryAdapters
		productionRetry.Adapters = productionRetryAdapters
		productionRetry.Sources = productionCenter.Sources
		creativeService.ShortDramaV2Images = creativeShortDramaV2ImageJobs{provider: &providerService}
		creativeService.CommercePrerollV2Images = creativeCommercePrerollV2ImageJobs{provider: &providerService}
		creativeService.AINativeStoryboards = creativeRepository
		creativeService.AINativeStoryboardAssetPreparer = creativeAINativeStoryboardAssetPreparer{provider: &providerService}
		creativeService.AINativeStoryboardScheduler = creative.JobRuntimeAINativeStoryboardScheduler{Store: runtimeStore}
		runtimeHandlers[creative.AINativeStoryboardJobKind] = creativeService.HandleAINativeStoryboardJob
		var speechSynthesizer provider.SpeechSynthesizer = provider.FakeSpeechAdapter{}
		if cfg.Provider.SpeechAdapter == "volcengine_speech" {
			speechSynthesizer, err = provider.NewVolcengineSpeechAdapter(provider.VolcengineSpeechConfig{
				Endpoint: cfg.Provider.VolcengineSpeech.Endpoint, APIKey: cfg.Provider.VolcengineSpeech.APIKey,
				ResourceID: cfg.Provider.VolcengineSpeech.ResourceID, DefaultVoice: cfg.Provider.VolcengineSpeech.DefaultVoice,
			})
			if err != nil {
				log.Fatalf("configure Volcengine speech adapter: %v", err)
			}
		} else if cfg.Provider.SpeechAdapter == "minimax_speech" {
			cipher, cipherErr := provider.NewAESGCMCredentialCipher(cfg.Provider.MasterKey, cfg.Provider.MasterKeyVersion)
			if cipherErr != nil {
				log.Fatalf("configure MiniMax speech credential encryption: %v", cipherErr)
			}
			routes := provider.MySQLGatewayConfigStore{DB: db, Cipher: cipher, AllowInsecureHTTP: cfg.Provider.AllowInsecureHTTP}
			speechSynthesizer = provider.MiniMaxSpeechAdapter{Routes: routes, Credentials: routes, ModelAlias: provider.DefaultMiniMaxSpeechModelAlias, DefaultVoiceAlias: "cookies.voice.brand.warm_female"}
			creativeService.BrandFilmSpeech = speechSynthesizer
		}
		creativeService.AINativeProductions = creativeRepository
		creativeService.AINativeProductionScheduler = creative.JobRuntimeAINativeProductionScheduler{Store: runtimeStore}
		creativeService.AINativeVideoJobs = &providerService
		creativeService.AINativeSpeech = speechSynthesizer
		creativeService.AINativeMaxActiveUnits = cfg.Creative.AINativeMaxActiveUnits
		runtimeHandlers[creative.AINativeProductionJobKind] = creativeService.HandleAINativeProductionJob
		imageTextReconciler := creative.ImageTextReconciler{
			Service: creativeService, Repository: creativeRepository,
			Provider: providerService, Limit: 100,
		}
		startWorker(workerContext, "creative-image-text-reconcile", imageTextReconciler.ProcessOnce)
		for kind, handler := range provider.NewRuntimeWorker(runtimeStore, providerService).Handlers {
			runtimeHandlers[kind] = handler
		}
		if actor != nil {
			imageFetcher, imageOK := adapter.(assets.GeneratedOutputFetcher)
			videoFetcher, videoOK := videoAdapter.(assets.GeneratedOutputFetcher)
			if !imageOK || !videoOK {
				log.Fatalf("configured Provider adapters must implement generated output fetching")
			}
			fetcher, routeErr := provider.NewOutputFetcherRouter(imageFetcher, videoFetcher)
			if routeErr != nil {
				log.Fatalf("configure Provider output routing: %v", routeErr)
			}
			intakeWorker := assets.GeneratedIntakeWorker{Repository: assetRepository, Projects: projectService, Fetcher: fetcher, Upload: *uploadService, Actor: *actor}
			startWorker(workerContext, "generated-intake", func(ctx context.Context) (bool, error) {
				return intakeWorker.ProcessOnce(ctx, "generated-intake")
			})
		}
	}
	sharedWorker := jobruntime.Worker{
		Store: runtimeStore, Handlers: runtimeHandlers, LeaseRenewer: runtimeStore,
		Canceller: runtimeStore, HeartbeatInterval: 15 * time.Second,
	}
	for index := 1; index <= 3; index++ {
		workerID := fmt.Sprintf("shared-runtime-%d", index)
		runtimeRunner := &jobruntime.RecoveryRunner{
			Worker: sharedWorker, Recoverer: runtimeStore, WorkerID: workerID,
			LeaseDuration: time.Minute, RecoveryInterval: 30 * time.Second,
		}
		startWorker(workerContext, workerID, runtimeRunner.RunOnce)
	}

	server := newHTTPServer(cfg.HTTPAddr, httpserver.NewWithDependencies(dependencies))

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-stop
		stopWorkers()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
	}()

	log.Printf("cookies platform API listening on %s (%s)", cfg.HTTPAddr, cfg.Environment)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server stopped unexpectedly: %v", err)
	}
}

const modelAwareWriteTimeout = 11 * time.Minute

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		// Text model routes may legitimately wait for an upstream provider for
		// as long as ten minutes. Keep the server response window slightly
		// larger so net/http does not cut off a successful model response.
		WriteTimeout: modelAwareWriteTimeout,
		IdleTimeout:  60 * time.Second,
	}
}

func buildVideoAdapter(cfg config.Config, db *sql.DB, handles provider.OutputHandleStore) (provider.VideoProviderAdapter, error) {
	switch cfg.Provider.VideoAdapter {
	case "fake":
		return provider.NewFakeVideoAdapter(nil), nil
	case "adapter_gateway":
		cipher, err := provider.NewAESGCMCredentialCipher(cfg.Provider.MasterKey, cfg.Provider.MasterKeyVersion)
		if err != nil {
			return nil, err
		}
		store := provider.MySQLGatewayConfigStore{DB: db, Cipher: cipher, AllowInsecureHTTP: cfg.Provider.AllowInsecureHTTP, VideoConnectionType: "adapter_gateway"}
		return provider.NewAdapterGatewayVideoAdapter(store, handles, cfg.Provider.AllowInsecureHTTP)
	default:
		return nil, fmt.Errorf("unsupported Provider video adapter %q", cfg.Provider.VideoAdapter)
	}
}

func buildImageAdapter(cfg config.Config, db *sql.DB, blobs assets.BlobStore) (provider.ImageProviderAdapter, provider.OutputHandleStore, error) {
	var handles provider.OutputHandleStore = provider.MySQLOutputHandleStore{DB: db}
	if cfg.Provider.ImageAdapter == "adapter_gateway" && cfg.Environment != config.EnvironmentLocal {
		handles = provider.ObjectOutputHandleStore{DB: db, Blobs: blobs, Bucket: cfg.Provider.OutputBucket}
	}
	switch cfg.Provider.ImageAdapter {
	case "fake":
		return provider.NewFakeImageAdapter(nil), handles, nil
	case "ark_image":
		adapter, err := provider.NewArkImageAdapter(provider.ArkImageConfig{APIKey: cfg.Provider.ArkImage.APIKey, Model: cfg.Provider.ArkImage.Model, BaseURL: cfg.Provider.ArkImage.BaseURL}, handles)
		return adapter, handles, err
	case "openai_image":
		adapter, err := provider.NewOpenAIImageAdapter(provider.OpenAIImageConfig{APIKey: cfg.Provider.OpenAIImage.APIKey, Model: cfg.Provider.OpenAIImage.Model, BaseURL: cfg.Provider.OpenAIImage.BaseURL}, handles)
		return adapter, handles, err
	case "adapter_gateway":
		cipher, err := provider.NewAESGCMCredentialCipher(cfg.Provider.MasterKey, cfg.Provider.MasterKeyVersion)
		if err != nil {
			return nil, nil, err
		}
		configStore := provider.MySQLGatewayConfigStore{DB: db, Cipher: cipher, AllowInsecureHTTP: cfg.Provider.AllowInsecureHTTP}
		adapter, err := provider.NewAdapterGatewayImageAdapterWithPolicy(configStore, handles, cfg.Provider.AllowInsecureHTTP)
		return adapter, handles, err
	default:
		return nil, nil, fmt.Errorf("unsupported Provider image adapter %q", cfg.Provider.ImageAdapter)
	}
}

func buildTextAdapter(cfg config.Config, db *sql.DB) (provider.TextProviderAdapter, error) {
	switch cfg.Provider.TextAdapter {
	case "fake":
		return provider.FakeSyncAdapter{}, nil
	case "adapter_gateway":
		cipher, err := provider.NewAESGCMCredentialCipher(cfg.Provider.MasterKey, cfg.Provider.MasterKeyVersion)
		if err != nil {
			return nil, err
		}
		store := provider.MySQLGatewayConfigStore{DB: db, Cipher: cipher, AllowInsecureHTTP: cfg.Provider.AllowInsecureHTTP}
		return provider.NewAdapterGatewayTextAdapter(store, store, cfg.Provider.AllowInsecureHTTP)
	case "ark_text":
		return provider.NewArkTextAdapter(provider.ArkTextConfig{
			APIKey:  cfg.Provider.ArkText.APIKey,
			Model:   cfg.Provider.ArkText.Model,
			BaseURL: cfg.Provider.ArkText.BaseURL,
		})
	default:
		return nil, fmt.Errorf("unsupported Provider text adapter %q", cfg.Provider.TextAdapter)
	}
}

func buildVisionAdapter(cfg config.Config, db *sql.DB) (provider.VisionProviderAdapter, error) {
	if !cfg.MediaUnderstanding.RealProviderEnabled {
		return provider.FakeSyncAdapter{}, nil
	}
	switch cfg.Provider.TextAdapter {
	case "fake":
		return provider.FakeSyncAdapter{}, nil
	case "adapter_gateway":
		cipher, err := provider.NewAESGCMCredentialCipher(cfg.Provider.MasterKey, cfg.Provider.MasterKeyVersion)
		if err != nil {
			return nil, err
		}
		store := provider.MySQLGatewayConfigStore{DB: db, Cipher: cipher, AllowInsecureHTTP: cfg.Provider.AllowInsecureHTTP}
		return provider.NewAdapterGatewayVisionAdapter(store, store, cfg.Provider.AllowInsecureHTTP)
	case "ark_text":
		// The direct Ark text adapter has no reviewed multimodal transport.
		// Media artifacts still expose verified metadata and an explicit partial
		// status instead of pretending that semantic vision ran.
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported Provider vision adapter derived from text adapter %q", cfg.Provider.TextAdapter)
	}
}

func localExecutablePath(environment config.Environment, configured, name string) string {
	if configured != "" {
		return configured
	}
	if environment != config.EnvironmentLocal && environment != config.EnvironmentTest {
		return ""
	}
	value, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return value
}

func strategyOrganizationAllowlist(values []string) map[contract.OrganizationID]struct{} {
	if len(values) == 0 {
		return nil
	}
	result := make(map[contract.OrganizationID]struct{}, len(values))
	for _, value := range values {
		result[contract.OrganizationID(value)] = struct{}{}
	}
	return result
}

type assetVisionSourceResolver struct{ uploads *assets.UploadService }

type miyunProjectSourceAdapter struct{ projects *project.Service }
type miyunAssetSourceAdapter struct{ uploads *assets.UploadService }
type miyunKnowledgeSourceAdapter struct{ knowledge *knowledge.Service }
type miyunHandoffContentAdapter struct {
	uploads   *assets.UploadService
	knowledge *knowledge.Service
}
type miyunMediaEvidenceAdapter struct{ media *mediaunderstanding.Service }

func (a miyunProjectSourceAdapter) ReadMiyunProjectSource(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID) (insights.MiyunProjectSource, error) {
	projectContext, err := a.projects.GetContext(ctx, actor, projectID)
	if err != nil {
		return insights.MiyunProjectSource{}, err
	}
	businessContext, err := a.projects.GetBusinessContext(ctx, actor, projectID)
	if err != nil {
		return insights.MiyunProjectSource{}, err
	}
	workbench, err := a.projects.GetWorkbench(ctx, actor, projectID)
	if err != nil {
		return insights.MiyunProjectSource{}, err
	}
	if businessContext.ProjectID != projectID || workbench.Project.ProjectID != string(projectID) ||
		workbench.Project.OrganizationID != string(actor.OrganizationID) {
		return insights.MiyunProjectSource{}, fmt.Errorf("%w: Miyun Project projections are inconsistent", insights.ErrInvalidState)
	}
	products := make([]insights.MiyunProjectProduct, 0, len(businessContext.Products))
	for _, product := range businessContext.Products {
		products = append(products, insights.MiyunProjectProduct{ID: product.ID, Name: product.Name})
	}
	return insights.MiyunProjectSource{
		Context: projectContext, ProjectName: businessContext.ProjectName,
		BrandName: businessContext.BrandName, CategoryName: workbench.Brand.Category,
		Products: products,
	}, nil
}

func (a miyunAssetSourceAdapter) ReadMiyunAssetSource(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, ref contract.AssetVersionRef) (insights.MiyunAssetSource, error) {
	value, err := a.uploads.Get(ctx, actor, projectID, ref)
	if err != nil {
		return insights.MiyunAssetSource{}, err
	}
	return insights.MiyunAssetSource{
		Ref: value.Version.Ref(), Kind: value.Asset.Kind, MIMEType: value.Version.MIMEType,
		SHA256: value.Version.SHA256,
		Ready:  value.Asset.Status == assets.AssetReady && value.Version.Status == assets.AssetReady,
	}, nil
}

func (a miyunKnowledgeSourceAdapter) ReadMiyunKnowledgeSource(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, documentID string) (insights.MiyunKnowledgeSource, error) {
	value, err := a.knowledge.GetDocument(ctx, actor, projectID, documentID)
	if err != nil {
		return insights.MiyunKnowledgeSource{}, err
	}
	return insights.MiyunKnowledgeSource{
		ID: value.ID, Filename: value.Filename, MIMEType: value.MIMEType,
		Status: value.Status, Text: value.ExtractedText, TextSHA256: value.TextSHA256, ContentSHA256: value.ContentSHA256,
	}, nil
}

func (a miyunHandoffContentAdapter) OpenMiyunHandoffAsset(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, ref contract.AssetVersionRef) (io.ReadCloser, error) {
	if a.uploads == nil {
		return nil, fmt.Errorf("Miyun asset content reader is unavailable")
	}
	stream, _, err := a.uploads.OpenPreview(ctx, actor, projectID, ref)
	return stream, err
}
func (a miyunHandoffContentAdapter) OpenMiyunHandoffDocument(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, id string) (io.ReadCloser, error) {
	if a.knowledge == nil {
		return nil, fmt.Errorf("Miyun knowledge content reader is unavailable")
	}
	stream, _, err := a.knowledge.OpenDocumentOriginalStream(ctx, actor, projectID, id)
	return stream, err
}

func (a miyunMediaEvidenceAdapter) ReadLatestMiyunMediaEvidence(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, ref contract.AssetVersionRef) (insights.MiyunMediaEvidence, bool, error) {
	value, err := a.media.GetLatestForAsset(ctx, actor, projectID, ref)
	if errors.Is(err, mediaunderstanding.ErrNotFound) {
		return insights.MiyunMediaEvidence{}, false, nil
	}
	if err != nil {
		return insights.MiyunMediaEvidence{}, false, err
	}
	evidence := make([]string, 0, 1+len(value.VisibleText)+len(value.Observations)+len(value.Inferences)+len(value.Risks)+len(value.Unknowns)+len(value.Transcript))
	if value.Summary != "" {
		evidence = append(evidence, value.Summary)
	}
	for _, group := range [][]mediaunderstanding.Evidence{value.VisibleText, value.Observations, value.Inferences, value.Risks, value.Unknowns, value.Transcript} {
		for _, item := range group {
			evidence = append(evidence, item.Text)
		}
	}
	asrProviderCode, asrModelVersion := "", ""
	if value.TranscriptionLineage != nil {
		asrProviderCode, asrModelVersion = value.TranscriptionLineage.ProviderCode, value.TranscriptionLineage.ModelVersion
	}
	return insights.MiyunMediaEvidence{
		ArtifactID: value.ID, Status: string(value.Status), ContentHash: value.ContentHash, Evidence: evidence,
		MediaFormatCode:        value.Classifications.MediaFormat.Code,
		ContentStyleCode:       value.Classifications.ContentStyle.Code,
		ContentStyleConfidence: value.Classifications.ContentStyle.Confidence,
		VisionProviderCode:     value.Lineage.ProviderCode,
		VisionModelVersion:     value.Lineage.ModelVersion,
		ASRProviderCode:        asrProviderCode,
		ASRModelVersion:        asrModelVersion,
	}, true, nil
}

type creativeAssetReader struct{ uploads *assets.UploadService }

type creativeProductResolver struct{ resolver productsource.Resolver }

func (r creativeProductResolver) Resolve(ctx context.Context, input string) (creative.AINativeProductSnapshot, error) {
	value, err := r.resolver.Resolve(ctx, input)
	if err != nil {
		switch {
		case errors.Is(err, productsource.ErrIncompleteLink):
			return creative.AINativeProductSnapshot{}, fmt.Errorf("%w: %v", creative.ErrAINativeProductLinkIncomplete, err)
		case errors.Is(err, productsource.ErrUnsupportedLink):
			return creative.AINativeProductSnapshot{}, fmt.Errorf("%w: %v", creative.ErrAINativeProductLinkUnsupported, err)
		case errors.Is(err, productsource.ErrProductMissing):
			return creative.AINativeProductSnapshot{}, fmt.Errorf("%w: %v", creative.ErrAINativeProductDetailMissing, err)
		}
		return creative.AINativeProductSnapshot{}, err
	}
	images := make([]creative.AINativeProductImage, 0, len(value.Images))
	for _, image := range value.Images {
		images = append(images, creative.AINativeProductImage{URL: image.URL, Role: image.Role})
	}
	return creative.AINativeProductSnapshot{
		Source: value.Source, ProductID: value.ProductID, Name: value.Name, Description: value.Description,
		Images: images,
		Price: creative.AINativeProductPrice{
			MinRaw: value.Price.MinRaw, MaxRaw: value.Price.MaxRaw, Currency: value.Price.Currency,
			DisplayUnconfirmed: value.Price.DisplayUnconfirmed,
		},
		Sales: value.Sales, SourceURL: value.SourceURL, ResolutionStatus: value.ResolutionStatus,
		ResourceType: value.ResourceType, MissingFields: append([]string{}, value.MissingFields...),
	}, nil
}

type creativeMediaSource struct {
	repository assets.Repository
	blobs      assets.BlobStore
}

func (s creativeMediaSource) OpenVideo(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, ref contract.AssetVersionRef) (assets.AssetVersion, io.ReadCloser, error) {
	if s.repository == nil || s.blobs == nil {
		return assets.AssetVersion{}, nil, fmt.Errorf("creative media source is unavailable")
	}
	value, err := s.repository.GetProjectAsset(ctx, organizationID, projectID, ref)
	if err != nil {
		return assets.AssetVersion{}, nil, err
	}
	if value.Asset.Status != assets.AssetReady || value.Version.Status != assets.AssetReady || value.Asset.Kind != contract.AssetVideo || value.Version.MIMEType != "video/mp4" {
		return assets.AssetVersion{}, nil, fmt.Errorf("creative media source is not a ready MP4")
	}
	reader, info, err := s.blobs.Open(ctx, value.Version.Blob)
	if err != nil {
		return assets.AssetVersion{}, nil, err
	}
	if info.SizeBytes != value.Version.SizeBytes {
		reader.Close()
		return assets.AssetVersion{}, nil, assets.ErrOutputMetadataMismatch
	}
	return value.Version, reader, nil
}

func (s creativeMediaSource) OpenVisual(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, ref contract.AssetVersionRef) (assets.AssetVersion, io.ReadCloser, error) {
	if s.repository == nil || s.blobs == nil {
		return assets.AssetVersion{}, nil, fmt.Errorf("creative visual source is unavailable")
	}
	value, err := s.repository.GetProjectAsset(ctx, organizationID, projectID, ref)
	if err != nil {
		return assets.AssetVersion{}, nil, err
	}
	readyVideo := value.Asset.Kind == contract.AssetVideo && value.Version.MIMEType == "video/mp4"
	readyImage := value.Asset.Kind == contract.AssetImage && (value.Version.MIMEType == "image/jpeg" || value.Version.MIMEType == "image/png" || value.Version.MIMEType == "image/webp")
	if value.Asset.Status != assets.AssetReady || value.Version.Status != assets.AssetReady || !readyVideo && !readyImage {
		return assets.AssetVersion{}, nil, fmt.Errorf("creative visual source is not a ready supported video or image")
	}
	reader, info, err := s.blobs.Open(ctx, value.Version.Blob)
	if err != nil {
		return assets.AssetVersion{}, nil, err
	}
	if info.SizeBytes != value.Version.SizeBytes {
		reader.Close()
		return assets.AssetVersion{}, nil, assets.ErrOutputMetadataMismatch
	}
	return value.Version, reader, nil
}

func (s creativeMediaSource) OpenAudio(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, ref contract.AssetVersionRef) (assets.AssetVersion, io.ReadCloser, error) {
	if s.repository == nil || s.blobs == nil {
		return assets.AssetVersion{}, nil, fmt.Errorf("creative media source is unavailable")
	}
	value, err := s.repository.GetProjectAsset(ctx, organizationID, projectID, ref)
	if err != nil {
		return assets.AssetVersion{}, nil, err
	}
	if value.Asset.Status != assets.AssetReady || value.Version.Status != assets.AssetReady || value.Asset.Kind != contract.AssetAudio || (value.Version.MIMEType != "audio/wav" && value.Version.MIMEType != "audio/mpeg" && value.Version.MIMEType != "audio/aac") {
		return assets.AssetVersion{}, nil, fmt.Errorf("creative media source is not ready supported audio")
	}
	reader, info, err := s.blobs.Open(ctx, value.Version.Blob)
	if err != nil {
		return assets.AssetVersion{}, nil, err
	}
	if info.SizeBytes != value.Version.SizeBytes {
		reader.Close()
		return assets.AssetVersion{}, nil, assets.ErrOutputMetadataMismatch
	}
	return value.Version, reader, nil
}

type creativeRenderedAssetWriter struct{ uploads *assets.UploadService }

type creativeAudioAssetWriter struct{ uploads *assets.UploadService }

type creativeImageAssetIO struct{ uploads *assets.UploadService }

type creativeRenderedImageWriter struct{ uploads *assets.UploadService }

func (w creativeRenderedAssetWriter) IngestRenderedVideo(ctx context.Context, requestContext contract.RequestContext, projectID contract.ProjectID, renderJobID string, content io.Reader, sizeBytes int64) (contract.ProjectAssetRef, error) {
	if w.uploads == nil {
		return contract.ProjectAssetRef{}, fmt.Errorf("rendered asset intake is unavailable")
	}
	return w.uploads.IngestRenderedVideo(ctx, requestContext, projectID, renderJobID, content, sizeBytes)
}

func (w creativeAudioAssetWriter) IngestDerivedAudio(ctx context.Context, requestContext contract.RequestContext, projectID contract.ProjectID, derivationID string, content io.Reader, sizeBytes int64, mimeType string, sourceResources []contract.ResourceRef) (contract.ProjectAssetRef, error) {
	if w.uploads == nil {
		return contract.ProjectAssetRef{}, fmt.Errorf("audio asset intake is unavailable")
	}
	return w.uploads.IngestDerivedAudio(ctx, requestContext, projectID, derivationID, content, sizeBytes, mimeType, sourceResources)
}

func (r creativeImageAssetIO) OpenImage(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, ref contract.AssetVersionRef) (io.ReadCloser, error) {
	if r.uploads == nil {
		return nil, fmt.Errorf("image asset reader is unavailable")
	}
	reader, info, err := r.uploads.OpenPreview(ctx, actor, projectID, ref)
	if err != nil {
		return nil, err
	}
	if info.MIMEType != "image/png" && info.MIMEType != "image/jpeg" {
		reader.Close()
		return nil, fmt.Errorf("image asset is not a supported image")
	}
	return reader, nil
}

func (w creativeRenderedImageWriter) IngestRenderedImage(
	ctx context.Context,
	requestContext contract.RequestContext,
	projectID contract.ProjectID,
	renderJobID string,
	content io.Reader,
	sizeBytes int64,
	expectedWidth int,
	expectedHeight int,
	sourceAssets []contract.AssetVersionRef,
	sourceResources []contract.ResourceRef,
) (contract.ProjectAssetRef, error) {
	if w.uploads == nil {
		return contract.ProjectAssetRef{}, fmt.Errorf("rendered image intake is unavailable")
	}
	return w.uploads.IngestRenderedImage(
		ctx, requestContext, projectID, renderJobID, content, sizeBytes,
		expectedWidth, expectedHeight,
		sourceAssets, sourceResources,
	)
}

func (r creativeAssetReader) ReadForCreative(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, ref contract.AssetVersionRef) (creative.CreativeAssetSnapshot, error) {
	if r.uploads == nil {
		return creative.CreativeAssetSnapshot{}, fmt.Errorf("asset upload service is required")
	}
	value, err := r.uploads.Get(ctx, actor, projectID, ref)
	if err != nil {
		return creative.CreativeAssetSnapshot{}, err
	}
	return creative.CreativeAssetSnapshot{
		Ref: value.Ref.AssetVersion, Kind: value.Asset.Kind, MIMEType: value.Version.MIMEType,
		Ready:       value.Asset.Status == assets.AssetReady && value.Version.Status == assets.AssetReady,
		WidthPixels: value.Version.WidthPixels, HeightPixels: value.Version.HeightPixels,
		DurationMS: value.Version.DurationMS, FrameRate: value.Version.FrameRate,
		VideoCodec: value.Version.VideoCodec, AudioCodec: value.Version.AudioCodec,
	}, nil
}

// insightMediaReader 把素材库上传时探测到的元数据递给洞察的客观可测层。
//
// 只读，而且只读已经落库的探测结果——洞察不再跑一遍 ffprobe。两处各自量出的时长
// 对不上的时候，没人说得清该信谁，而这一层的全部价值就在于「同一个文件量两遍
// 结果一样」。
type insightMediaReader struct{ uploads *assets.UploadService }

func (r insightMediaReader) ReadMediaFacts(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID,
	platformAssetID string, platformAssetVersion int64) (insights.MediaFacts, error) {
	if r.uploads == nil {
		return insights.MediaFacts{}, fmt.Errorf("asset upload service is required")
	}
	value, err := r.uploads.Get(ctx, actor, projectID, contract.AssetVersionRef{
		AssetID: contract.AssetID(platformAssetID), Version: platformAssetVersion,
	})
	if err != nil {
		return insights.MediaFacts{}, err
	}
	// 探测没成功就如实说没有。这几个字段在失败时是零值，当成「时长 0 秒」写进
	// 客观可测层，就是把一个探测故障伪装成一条测量结论。
	if value.Version.Media.ProbeStatus != assets.MediaProbeSucceeded {
		reason := "素材库还没探测过这个文件"
		switch value.Version.Media.ProbeStatus {
		case assets.MediaProbeFailed:
			reason = "素材库探测这个文件失败了"
		case assets.MediaProbeNotRequired:
			reason = "这个文件不是音视频，没有可探测的时长"
		}
		return insights.MediaFacts{Unavailable: reason}, nil
	}
	return insights.MediaFacts{
		Measured:        true,
		DurationSeconds: value.Version.Media.DurationSeconds,
		WidthPixels:     value.Version.WidthPixels,
		HeightPixels:    value.Version.HeightPixels,
	}, nil
}

// insightMediaUnderstander 把平台的「媒体理解」接到洞察的视频语义提取上。
//
// 一次 Request 同时管排队和读结果：媒体理解按 (文件 SHA256, profile, prompt 版本,
// 模型别名) 算输入指纹去重，同一条视频重复请求拿回的是同一份产出，不会重复排队，
// 也不会重复花钱。所以洞察那边只有一个方法，不用自己判断该 Request 还是该 Get。
type insightMediaUnderstander struct{ service *mediaunderstanding.Service }

func (u insightMediaUnderstander) UnderstandMedia(ctx context.Context, actor contract.ActorContext,
	projectID contract.ProjectID, platformAssetID string, platformAssetVersion int64) (insights.MediaUnderstanding, error) {
	if u.service == nil {
		return insights.MediaUnderstanding{Unavailable: "这个环境没接多模态"}, nil
	}
	artifact, _, err := u.service.Request(ctx, actor, projectID, mediaunderstanding.CreateRequest{
		AssetID: platformAssetID, Version: platformAssetVersion,
	})
	if errors.Is(err, mediaunderstanding.ErrUnsupportedProfile) {
		return insights.MediaUnderstanding{
			Unavailable: "这条视频不在多模态能看的范围内（只看 15–90 秒的 mp4）",
		}, nil
	}
	if err != nil {
		return insights.MediaUnderstanding{}, err
	}

	switch artifact.Status {
	case mediaunderstanding.StatusRunning:
		return insights.MediaUnderstanding{Pending: true, ArtifactID: artifact.ID}, nil
	case mediaunderstanding.StatusFailed:
		return insights.MediaUnderstanding{
			ArtifactID: artifact.ID, Unavailable: understandingFailureReason(artifact),
		}, nil
	}
	// 视觉链路没配好时，媒体理解仍然会落一条 partial，里面只有一句技术校验
	// （applyTechnicalFallback）。那句话喂给特征模型只会让它照着编，所以这里当成
	// 「没看成」，让洞察回落到人填正文——它自己在 Warnings 里说了这件事。
	for _, warning := range artifact.Warnings {
		switch warning {
		case "vision_provider_unavailable", "vision_route_unavailable", "fake_vision_no_semantic_claims":
			return insights.MediaUnderstanding{
				ArtifactID: artifact.ID, Unavailable: "这个环境的视觉模型没接通，模型没真看画面",
			}, nil
		}
	}
	return insights.MediaUnderstanding{
		Ready: true, ArtifactID: artifact.ID, Summary: artifact.Summary,
		Observations: evidenceTexts(artifact.Observations), Inferences: evidenceTexts(artifact.Inferences),
		VisibleText: evidenceTexts(artifact.VisibleText), Transcript: evidenceTexts(artifact.Transcript),
		KeyframeCount: len(artifact.Keyframes), ProviderCode: artifact.Lineage.ProviderCode,
		ModelAlias: artifact.Lineage.ModelAlias, ModelVersion: artifact.Lineage.ModelVersion,
		ContentHash: artifact.ContentHash,
	}, nil
}

func understandingFailureReason(artifact mediaunderstanding.Artifact) string {
	if message := strings.TrimSpace(artifact.ErrorMessage); message != "" {
		return message
	}
	return "多模态没能看完这条视频"
}

// evidenceTexts 只取正文，丢掉帧号和置信度。
//
// 丢掉是有意的：这段字是要发给特征模型的，带上「frame_index: 2, confidence: 0.7」
// 只会让它把这些数字也当成待提取的内容。帧号和置信度仍然完整留在媒体理解的产出上，
// 想看证据链去那边看——那边有帧图，这边只有文字。
func evidenceTexts(items []mediaunderstanding.Evidence) []string {
	texts := make([]string, 0, len(items))
	for _, item := range items {
		if value := strings.TrimSpace(item.Text); value != "" {
			texts = append(texts, value)
		}
	}
	return texts
}

func (r assetVisionSourceResolver) ResolveVisionSources(ctx context.Context, actor contract.ActorContext, projectContext contract.ProjectContext, refs []contract.ProjectAssetRef) ([]provider.VisionSource, error) {
	if r.uploads == nil {
		return nil, fmt.Errorf("asset upload service is required")
	}
	sources := make([]provider.VisionSource, 0, len(refs))
	for _, ref := range refs {
		reader, info, err := r.uploads.OpenPreview(ctx, actor, projectContext.ProjectID, ref.AssetVersion)
		if err != nil {
			for _, source := range sources {
				source.Content.Close()
			}
			return nil, err
		}
		sources = append(sources, provider.VisionSource{Reference: ref, MIMEType: info.MIMEType, Content: reader})
	}
	return sources, nil
}

func startWorker(ctx context.Context, name string, runOnce func(context.Context) (bool, error)) {
	go func() {
		for {
			if err := ctx.Err(); err != nil {
				return
			}
			processed, err := runOnce(ctx)
			if err != nil {
				log.Printf("%s worker error: %v", name, err)
			}
			if processed && err == nil {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(250 * time.Millisecond):
			}
		}
	}()
}

func startPeriodicWorker(ctx context.Context, name string, interval time.Duration, runOnce func(context.Context) error) {
	if interval <= 0 {
		return
	}
	go func() {
		run := func() {
			if err := runOnce(ctx); err != nil && ctx.Err() == nil {
				log.Printf("%s worker error: %v", name, err)
			}
		}
		run()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

func buildIdentityResolver(cfg config.Config, validator identity.ActorValidator) (identity.Resolver, *contract.ActorContext, error) {
	if cfg.LocalIdentity == nil {
		return identity.RejectingResolver{}, nil, nil
	}

	principalKind := contract.PrincipalKind(cfg.LocalIdentity.PrincipalKind)
	actor := contract.ActorContext{
		OrganizationID: contract.OrganizationID(cfg.LocalIdentity.OrganizationID),
		Principal: contract.Principal{
			Kind: principalKind,
			ID:   cfg.LocalIdentity.PrincipalID,
		},
		Scopes: contract.ScopesFromStrings(cfg.LocalIdentity.Scopes),
	}
	static, err := identity.NewStaticResolver(actor)
	if err != nil {
		return nil, nil, err
	}
	return identity.ValidatingResolver{Delegate: static, Validator: validator}, &actor, nil
}

func buildBlobStore(cfg config.Config) (assets.BlobStore, error) {
	switch cfg.ObjectStorage.Provider {
	case "memory":
		return assets.NewMemoryBlobStore(), nil
	case "filesystem":
		return assets.NewFilesystemBlobStore(cfg.ObjectStorage.FilesystemRoot)
	case "tos":
		return assets.NewTOSBlobStore(assets.TOSConfig{Endpoint: cfg.ObjectStorage.Endpoint, Region: cfg.ObjectStorage.Region, AccessKey: cfg.ObjectStorage.AccessKey, SecretKey: cfg.ObjectStorage.SecretKey, SecurityToken: cfg.ObjectStorage.SecurityToken})
	default:
		return nil, errors.New("unsupported object storage provider")
	}
}

func buildScanner(cfg config.Config) assets.ContentScanner {
	if cfg.Scanner.Mode == "clamav" {
		return assets.ClamAVScanner{Address: cfg.Scanner.Address}
	}
	return assets.NoopScanner{}
}
