// Package config owns process configuration validation. Configuration comes
// from the process environment; local development may use an ignored .env as
// a fallback, while deployed environments never read that file.
package config

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Environment string

const (
	EnvironmentLocal      Environment = "local"
	EnvironmentTest       Environment = "test"
	EnvironmentStaging    Environment = "staging"
	EnvironmentProduction Environment = "production"
)

type LocalIdentity struct {
	OrganizationID string
	PrincipalKind  string
	PrincipalID    string
	ProjectID      string
	Scopes         []string
}

type Config struct {
	Environment        Environment
	HTTPAddr           string
	MySQL              MySQL
	Auth               Auth
	ObjectStorage      ObjectStorage
	Scanner            Scanner
	Media              Media
	MediaUnderstanding MediaUnderstanding
	Provider           Provider
	Creative           Creative
	Strategy           Strategy
	Research           Research
	Miyun              Miyun
	OceanEngine        OceanEngine
	BrowserRPA         BrowserRPA
	LocalIdentity      *LocalIdentity
}

// BrowserRPA configures the Playwright-based browser automation executor that
// drives externally authenticated advertising-platform sessions for the
// delivery control plane. It is disabled by default; the takeover-only
// control plane remains the production baseline.
type BrowserRPA struct {
	Enabled               bool
	RunnerProtocol        string
	Command               string
	ScriptPath            string
	EvidenceRoot          string
	EdgeSessionFile       string
	SessionProbeScript    string
	AuthorityStateRoot    string
	PrepareTimeoutSeconds int
	SubmitTimeoutSeconds  int
	CDPEndpointFallback   string
}

type Auth struct {
	PasswordEnabled bool
	Username        string
	Password        string
	SessionHours    int
}

type ObjectStorage struct {
	Provider         string
	FilesystemRoot   string
	Endpoint         string
	Region           string
	AccessKey        string
	SecretKey        string
	SecurityToken    string
	QuarantineBucket string
	AssetsBucket     string
}

type Scanner struct {
	Mode    string
	Address string
}

// Media configures optional local media executables. Empty executable paths
// keep non-video capabilities available while video probing/rendering reports
// an explicit capability error at the operation boundary.
type Media struct {
	FFmpegPath    string
	FFprobePath   string
	VideoWorkRoot string
}

// MediaUnderstanding owns multimodal inference rollout independently from
// Strategy generation and other Creative model features.
type MediaUnderstanding struct {
	RealProviderEnabled bool
	ASREnabled          bool
	VisionModelAlias    string
}

// Strategy controls gradual rollout independently from the Creative system.
// Package-to-Creative permits only the explicit package-to-Intake handoff;
// approval never creates a Creative task implicitly.
type Strategy struct {
	Enabled                     bool
	V2Enabled                   bool
	RealProviderEnabled         bool
	ApproveEnabled              bool
	PackageToCreativeEnabled    bool
	CreativeTaskPlanningEnabled bool
	QuickViralRemakeEnabled     bool
	TextModelAlias              string
	LiteTextModelAlias          string
	DeepReviewModelAlias        string
	PromptVersion               string
	ConversationPromptVersion   string
	RevisePromptVersion         string
	ReviewPromptVersion         string
	RepairPromptVersion         string
	CreativeTaskPromptVersion   string
	CriticEnabled               bool
	ContextSelectionEnabled     bool
	OrganizationAllowlist       []string
}

type Creative struct {
	DirectionPlanningEnabled       bool
	DirectionPlannerModelAlias     string
	ImageFontPath                  string
	ImageFontSHA256                string
	ShortDramaModelPlannerEnabled  bool
	ShortDramaPlannerModelAlias    string
	GamePrerollModelPlannerEnabled bool
	GamePrerollPlannerModelAlias   string
	BrandFilmModelPlannerEnabled   bool
	BrandFilmPlannerModelAlias     string
	AINativeMaxActiveUnits         int
}

// Research configures backend-owned web research. MCP fields are retained only
// for decoding older local configuration and are not used by the API process.
type Research struct {
	SeedEnabled                  bool
	SeedModelAlias               string
	DocumentVisionEnabled        bool
	DocumentVisionModelAlias     string
	DocumentConverterEnabled     bool
	DocumentConverterBaseURL     string
	DocumentConverterVersion     string
	DocumentConverterTimeout     int
	DocumentConverterMaxPDFBytes int
	DocumentConverterAllowHTTP   bool
	MaxConcurrent                int
	TikaEnabled                  bool
	TikaBaseURL                  string
	TikaVersion                  string
	TikaTimeoutSeconds           int
	TikaMaxOutputBytes           int
	MCPStdioCommand              string
	MCPStdioArgs                 []string
	MCPToolName                  string
	MCPProtocolVersion           string
	MCPEnvAllowlist              []string
	TimeoutSeconds               int
	MaxOutputBytes               int
}

// Miyun controls the real third-party collection path. It is disabled by
// default; enabling it requires an application key and explicit CDN allowlist.
type Miyun struct {
	Enabled              bool
	Endpoint             string
	MasterKey            string
	MasterKeyVersion     string
	DownloadAllowedHosts []string
	MaxConcurrent        int
	RequestsPerSecond    int
	CooldownSeconds      int
}

// OceanEngine controls the read-only private web API session verifier.
type OceanEngine struct {
	Enabled             bool
	WebAPIWriteEnabled  bool
	WebAPIWriteAccounts []string
	WebAPITemplateFile  string
	BaseURL             string
	BusinessBaseURL     string
	MasterKey           string
	MasterKeyVersion    string
	PatrolEnabled       bool
	PatrolIntervalHours int
	PatrolLookbackDays  int
}

// Provider contains only local composition choices. Credentials are read from
// the process environment (or ignored local .env), never from project data.
type Provider struct {
	ImageAdapter      string
	VideoAdapter      string
	TextAdapter       string
	AudioAdapter      string
	SpeechAdapter     string
	MasterKey         string
	MasterKeyVersion  string
	OutputBucket      string
	AllowInsecureHTTP bool
	ArkImage          ArkImage
	ArkVideo          ArkVideo
	ArkText           ArkText
	OpenAIImage       OpenAIImage
	VolcengineASR     VolcengineASR
	VolcengineSpeech  VolcengineSpeech
}

type ArkImage struct {
	APIKey  string
	Model   string
	BaseURL string
}

type ArkText struct {
	APIKey  string
	Model   string
	BaseURL string
}

type ArkVideo struct {
	APIKey  string
	Model   string
	BaseURL string
}

type OpenAIImage struct {
	APIKey  string
	Model   string
	BaseURL string
}

// VolcengineASR configures the shared recording-file recognition adapter.
// Keeping it separate from Ark text/video prevents one credential from being
// used for the wrong API.
type VolcengineASR struct {
	Endpoint    string
	AuthMode    string
	AppID       string
	AccessToken string
	APIKey      string
	ResourceID  string
	Model       string
}

type VolcengineSpeech struct {
	Endpoint     string
	APIKey       string
	ResourceID   string
	DefaultVoice string
}

// MySQL contains only connection-pool configuration. No business module owns
// a second pool; modules receive this shared dependency instead.
type MySQL struct {
	DSN          string
	MaxOpenConns int
	MaxIdleConns int
}

func Load() (Config, error) {
	values, err := localDotEnvValues()
	if err != nil {
		return Config{}, err
	}
	return FromLookup(func(key string) (string, bool) {
		if value, ok := os.LookupEnv(key); ok {
			return value, true
		}
		value, ok := values[key]
		return value, ok
	})
}

// localDotEnvValues provides the local `Copy-Item .env.example .env` workflow
// without ever overriding process environment variables. A process configured
// as staging or production deliberately ignores the working-directory .env so
// deployed configuration remains environment/config-center only.
func localDotEnvValues() (map[string]string, error) {
	if environment, ok := os.LookupEnv("COOKIES_ENV"); ok && Environment(environment) != EnvironmentLocal {
		return map[string]string{}, nil
	}
	file, err := os.Open(filepath.Clean(".env"))
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open local .env: %w", err)
	}
	defer file.Close()
	values, err := parseDotEnv(file)
	if err != nil {
		return nil, fmt.Errorf("parse local .env: %w", err)
	}
	return values, nil
}

// parseDotEnv supports the constrained dotenv syntax used by .env.example:
// comments, optional `export`, KEY=VALUE, and single- or double-quoted
// values. It intentionally does not expand variables, preventing unexpected
// interpolation of credentials during local startup.
func parseDotEnv(reader io.Reader) (map[string]string, error) {
	values := make(map[string]string)
	scanner := bufio.NewScanner(reader)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if lineNumber == 1 {
			line = strings.TrimPrefix(line, "\uFEFF")
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, found := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return nil, fmt.Errorf("line %d must use KEY=VALUE", lineNumber)
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
			value = value[1 : len(value)-1]
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func FromLookup(lookup func(string) (string, bool)) (Config, error) {
	environment := Environment(valueOr(lookup, "COOKIES_ENV", string(EnvironmentLocal)))
	strategyEnabled, err := strictBoolValueOr(lookup, "COOKIES_STRATEGY_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	strategyRealProviderEnabled, err := strictBoolValueOr(lookup, "COOKIES_STRATEGY_REAL_PROVIDER_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	mediaUnderstandingRealProviderEnabled, err := strictBoolValueOr(lookup, "COOKIES_MEDIA_UNDERSTANDING_REAL_PROVIDER_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	mediaUnderstandingASREnabled, err := strictBoolValueOr(lookup, "COOKIES_MEDIA_UNDERSTANDING_ASR_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	strategyApproveEnabled, err := strictBoolValueOr(lookup, "COOKIES_STRATEGY_APPROVE_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	strategyPackageToCreativeEnabled, err := strictBoolValueOr(lookup, "COOKIES_STRATEGY_PACKAGE_TO_CREATIVE_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	strategyCreativeTaskPlanningEnabled, err := strictBoolValueOr(
		lookup,
		"COOKIES_STRATEGY_CREATIVE_TASK_PLANNING_ENABLED",
		environment == EnvironmentLocal || environment == EnvironmentTest,
	)
	if err != nil {
		return Config{}, err
	}
	strategyCriticEnabled, err := strictBoolValueOr(lookup, "COOKIES_STRATEGY_CRITIC_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	strategyContextSelectionEnabled, err := strictBoolValueOr(
		lookup,
		"COOKIES_STRATEGY_CONTEXT_SELECTION_ENABLED",
		environment == EnvironmentLocal || environment == EnvironmentTest,
	)
	if err != nil {
		return Config{}, err
	}
	passwordAuthEnabled, err := strictBoolValueOr(
		lookup,
		"COOKIES_PASSWORD_AUTH_ENABLED",
		environment == EnvironmentLocal || environment == EnvironmentTest,
	)
	if err != nil {
		return Config{}, err
	}
	strategyV2Enabled, err := strictBoolValueOr(lookup, "COOKIES_STRATEGY_V2_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	strategyQuickViralRemakeEnabled, err := strictBoolValueOr(
		lookup,
		"COOKIES_STRATEGY_QUICK_VIRAL_REMAKE_ENABLED",
		environment != EnvironmentProduction,
	)
	if err != nil {
		return Config{}, err
	}
	gamePrerollModelPlannerEnabled, err := strictBoolValueOr(
		lookup,
		"COOKIES_CREATIVE_GAME_PREROLL_MODEL_PLANNER_ENABLED",
		true,
	)
	if err != nil {
		return Config{}, err
	}
	shortDramaModelPlannerEnabled, err := strictBoolValueOr(
		lookup,
		"COOKIES_CREATIVE_SHORT_DRAMA_MODEL_PLANNER_ENABLED",
		true,
	)
	if err != nil {
		return Config{}, err
	}
	brandFilmModelPlannerEnabled, err := strictBoolValueOr(
		lookup,
		"COOKIES_CREATIVE_BRAND_FILM_MODEL_PLANNER_ENABLED",
		true,
	)
	if err != nil {
		return Config{}, err
	}
	directionPlanningEnabled, err := strictBoolValueOr(
		lookup,
		"COOKIES_CREATIVE_DIRECTION_PLANNING_ENABLED",
		false,
	)
	if err != nil {
		return Config{}, err
	}
	researchSeedEnabled, err := strictBoolValueOr(lookup, "COOKIES_RESEARCH_SEED_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	researchTikaEnabled, err := strictBoolValueOr(lookup, "COOKIES_RESEARCH_TIKA_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	documentVisionEnabled, err := strictBoolValueOr(lookup, "COOKIES_DOCUMENT_VISION_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	documentConverterEnabled, err := strictBoolValueOr(lookup, "COOKIES_DOCUMENT_CONVERTER_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	documentConverterAllowHTTP, err := strictBoolValueOr(lookup, "COOKIES_DOCUMENT_CONVERTER_ALLOW_INSECURE_HTTP", false)
	if err != nil {
		return Config{}, err
	}
	miyunEnabled, err := strictBoolValueOr(lookup, "COOKIES_MIYUN_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	oceanEngineEnabled, err := strictBoolValueOr(lookup, "COOKIES_OCEAN_ENGINE_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	oceanEnginePatrolEnabled, err := strictBoolValueOr(lookup, "COOKIES_OCEAN_ENGINE_PATROL_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	oceanEngineWebAPIWriteEnabled, err := strictBoolValueOr(lookup, "COOKIES_OCEAN_ENGINE_WEB_API_WRITE_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	generatePromptDefault := "strategy.generate.v2"
	conversationPromptDefault := "strategy.conversation.v3"
	revisePromptDefault := "strategy.revise.v2"
	reviewPromptDefault := "strategy.review.deep.v1"
	repairPromptDefault := "strategy.repair.v1"
	if environment == EnvironmentLocal || environment == EnvironmentTest {
		generatePromptDefault = "strategy.generate.v4"
		conversationPromptDefault = "strategy.conversation.v6"
		revisePromptDefault = "strategy.revise.v3"
		reviewPromptDefault = "strategy.review.deep.v2"
		repairPromptDefault = "strategy.repair.v2"
	}
	tosBucket := valueOrCompatibility(lookup, "COOKIES_TOS_BUCKET", "OBJECT_STORAGE_BUCKET_NAME", "")
	quarantineBucket := valueOrCompatibility(lookup, "COOKIES_TOS_QUARANTINE_BUCKET", "OBJECT_STORAGE_QUARANTINE_BUCKET", "cookies-quarantine")
	assetsBucket := valueOrCompatibility(lookup, "COOKIES_TOS_ASSETS_BUCKET", "OBJECT_STORAGE_ASSETS_BUCKET", "cookies-assets")
	providerOutputBucket := valueOr(lookup, "COOKIES_PROVIDER_OUTPUT_BUCKET", "cookies-provider-output")
	if tosBucket != "" {
		quarantineBucket = tosBucket
		assetsBucket = tosBucket
		providerOutputBucket = tosBucket
	}
	browserRpaEnabled, err := strictBoolValueOr(lookup, "COOKIES_BROWSER_RPA_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	browserRpaCDPFallback := valueOr(lookup, "COOKIES_BROWSER_RPA_CDP_ENDPOINT", "")
	if strings.TrimSpace(browserRpaCDPFallback) != "" && environment == EnvironmentProduction {
		return Config{}, fmt.Errorf("COOKIES_BROWSER_RPA_CDP_ENDPOINT fallback is only permitted outside production; register the endpoint on the execution environment instead")
	}
	browserRpaProtocol := valueOr(lookup, "COOKIES_BROWSER_RPA_RUNNER_PROTOCOL", "v3")
	if browserRpaProtocol != "v3" && browserRpaProtocol != "legacy" {
		return Config{}, fmt.Errorf("COOKIES_BROWSER_RPA_RUNNER_PROTOCOL must be v3 or legacy")
	}
	browserRpaScript := "scripts/browser-rpa-runner-v3.ts"
	if browserRpaProtocol == "legacy" {
		browserRpaScript = "scripts/browser-rpa-runner.ts"
	}
	config := Config{
		Environment: environment,
		HTTPAddr:    valueOr(lookup, "COOKIES_HTTP_ADDR", ":8080"),
		MySQL: MySQL{
			DSN:          valueOr(lookup, "COOKIES_MYSQL_DSN", "cookies:cookies_local_development_only@tcp(127.0.0.1:3307)/cookies?parseTime=true&multiStatements=true"),
			MaxOpenConns: intValueOr(lookup, "COOKIES_MYSQL_MAX_OPEN_CONNS", 10),
			MaxIdleConns: intValueOr(lookup, "COOKIES_MYSQL_MAX_IDLE_CONNS", 5),
		},
		Auth: Auth{
			PasswordEnabled: passwordAuthEnabled,
			Username:        valueOr(lookup, "COOKIES_ADMIN_USERNAME", "Admin"),
			Password:        valueOr(lookup, "COOKIES_ADMIN_PASSWORD", "123456"),
			SessionHours:    intValueOr(lookup, "COOKIES_SESSION_HOURS", 8),
		},
		ObjectStorage: ObjectStorage{
			Provider:         valueOrCompatibility(lookup, "COOKIES_BLOB_PROVIDER", "OBJECT_STORAGE_PROVIDER", "filesystem"),
			FilesystemRoot:   valueOr(lookup, "COOKIES_FILESYSTEM_BLOB_ROOT", ".data/blobs"),
			Endpoint:         valueOrCompatibility(lookup, "COOKIES_TOS_ENDPOINT", "OBJECT_STORAGE_ENDPOINT", ""),
			Region:           valueOrCompatibility(lookup, "COOKIES_TOS_REGION", "OBJECT_STORAGE_REGION", ""),
			AccessKey:        valueOrCompatibility(lookup, "COOKIES_TOS_ACCESS_KEY", "OBJECT_STORAGE_ACCESS_KEY", "OBJECT_STORAGE_ACCESS_KEY_ID", ""),
			SecretKey:        valueOrCompatibility(lookup, "COOKIES_TOS_SECRET_KEY", "OBJECT_STORAGE_SECRET_KEY", "OBJECT_STORAGE_ACCESS_KEY_SECRET", ""),
			SecurityToken:    valueOrCompatibility(lookup, "COOKIES_TOS_SECURITY_TOKEN", "OBJECT_STORAGE_SECURITY_TOKEN", ""),
			QuarantineBucket: quarantineBucket,
			AssetsBucket:     assetsBucket,
		},
		Scanner: Scanner{Mode: valueOr(lookup, "COOKIES_SCANNER_MODE", "noop"), Address: valueOr(lookup, "COOKIES_CLAMAV_ADDRESS", "")},
		Media: Media{
			FFmpegPath:    valueOr(lookup, "COOKIES_FFMPEG_PATH", ""),
			FFprobePath:   valueOr(lookup, "COOKIES_FFPROBE_PATH", ""),
			VideoWorkRoot: valueOr(lookup, "COOKIES_VIDEO_WORK_ROOT", ".data/video-work"),
		},
		MediaUnderstanding: MediaUnderstanding{
			RealProviderEnabled: mediaUnderstandingRealProviderEnabled,
			ASREnabled:          mediaUnderstandingASREnabled,
			VisionModelAlias:    valueOr(lookup, "COOKIES_MEDIA_UNDERSTANDING_VISION_MODEL_ALIAS", "cookies.vision.standard"),
		},
		Creative: Creative{
			DirectionPlanningEnabled:       directionPlanningEnabled,
			DirectionPlannerModelAlias:     valueOr(lookup, "COOKIES_CREATIVE_DIRECTION_PLANNER_MODEL_ALIAS", "cookies.text.standard"),
			ImageFontPath:                  valueOr(lookup, "COOKIES_CREATIVE_IMAGE_FONT_PATH", ""),
			ImageFontSHA256:                strings.ToLower(strings.TrimSpace(valueOr(lookup, "COOKIES_CREATIVE_IMAGE_FONT_SHA256", ""))),
			ShortDramaModelPlannerEnabled:  shortDramaModelPlannerEnabled,
			ShortDramaPlannerModelAlias:    valueOr(lookup, "COOKIES_CREATIVE_SHORT_DRAMA_PLANNER_MODEL_ALIAS", "cookies.text.standard"),
			GamePrerollModelPlannerEnabled: gamePrerollModelPlannerEnabled,
			GamePrerollPlannerModelAlias:   valueOr(lookup, "COOKIES_CREATIVE_GAME_PREROLL_PLANNER_MODEL_ALIAS", "cookies.text.standard"),
			BrandFilmModelPlannerEnabled:   brandFilmModelPlannerEnabled,
			BrandFilmPlannerModelAlias:     valueOr(lookup, "COOKIES_CREATIVE_BRAND_FILM_PLANNER_MODEL_ALIAS", "cookies.text.standard"),
			AINativeMaxActiveUnits:         intValueOr(lookup, "COOKIES_AI_AD_MAX_ACTIVE_UNITS", 2),
		},
		Strategy: Strategy{
			Enabled:                     strategyEnabled,
			V2Enabled:                   strategyV2Enabled,
			RealProviderEnabled:         strategyRealProviderEnabled,
			ApproveEnabled:              strategyApproveEnabled,
			PackageToCreativeEnabled:    strategyPackageToCreativeEnabled,
			CreativeTaskPlanningEnabled: strategyCreativeTaskPlanningEnabled,
			QuickViralRemakeEnabled:     strategyQuickViralRemakeEnabled,
			TextModelAlias:              valueOr(lookup, "COOKIES_STRATEGY_TEXT_MODEL_ALIAS", "cookies.text.standard"),
			LiteTextModelAlias:          valueOr(lookup, "COOKIES_STRATEGY_LITE_TEXT_MODEL_ALIAS", "cookies.text.lite"),
			DeepReviewModelAlias:        valueOr(lookup, "COOKIES_STRATEGY_DEEP_REVIEW_MODEL_ALIAS", "cookies.text.deep_review"),
			PromptVersion:               valueOr(lookup, "COOKIES_STRATEGY_PROMPT_VERSION", generatePromptDefault),
			ConversationPromptVersion:   valueOr(lookup, "COOKIES_STRATEGY_CONVERSATION_PROMPT_VERSION", conversationPromptDefault),
			RevisePromptVersion:         valueOr(lookup, "COOKIES_STRATEGY_REVISE_PROMPT_VERSION", revisePromptDefault),
			ReviewPromptVersion:         valueOr(lookup, "COOKIES_STRATEGY_REVIEW_PROMPT_VERSION", reviewPromptDefault),
			RepairPromptVersion:         valueOr(lookup, "COOKIES_STRATEGY_REPAIR_PROMPT_VERSION", repairPromptDefault),
			CreativeTaskPromptVersion:   valueOr(lookup, "COOKIES_STRATEGY_CREATIVE_TASK_PROMPT_VERSION", "strategy.creative_task.generate.v2"),
			CriticEnabled:               strategyCriticEnabled,
			ContextSelectionEnabled:     strategyContextSelectionEnabled,
			OrganizationAllowlist:       splitCSV(valueOr(lookup, "COOKIES_STRATEGY_ORGANIZATION_ALLOWLIST", "")),
		},
		Research: Research{
			SeedEnabled:                  researchSeedEnabled,
			SeedModelAlias:               valueOr(lookup, "COOKIES_RESEARCH_SEED_MODEL_ALIAS", "cookies.research.web.standard"),
			DocumentVisionEnabled:        documentVisionEnabled,
			DocumentVisionModelAlias:     valueOr(lookup, "COOKIES_DOCUMENT_VISION_MODEL_ALIAS", "cookies.document.vision.standard"),
			DocumentConverterEnabled:     documentConverterEnabled,
			DocumentConverterBaseURL:     valueOr(lookup, "COOKIES_DOCUMENT_CONVERTER_BASE_URL", "http://127.0.0.1:3000"),
			DocumentConverterVersion:     valueOr(lookup, "COOKIES_DOCUMENT_CONVERTER_VERSION", "gotenberg-8.34.0"),
			DocumentConverterTimeout:     intValueOr(lookup, "COOKIES_DOCUMENT_CONVERTER_TIMEOUT_SECONDS", 120),
			DocumentConverterMaxPDFBytes: intValueOr(lookup, "COOKIES_DOCUMENT_CONVERTER_MAX_PDF_BYTES", 32*1024*1024),
			DocumentConverterAllowHTTP:   documentConverterAllowHTTP,
			MaxConcurrent:                intValueOr(lookup, "COOKIES_RESEARCH_MAX_CONCURRENT", 3),
			TikaEnabled:                  researchTikaEnabled,
			TikaBaseURL:                  valueOr(lookup, "COOKIES_RESEARCH_TIKA_BASE_URL", "http://127.0.0.1:9998"),
			TikaVersion:                  valueOr(lookup, "COOKIES_RESEARCH_TIKA_VERSION", "3.2.3.0"),
			TikaTimeoutSeconds:           intValueOr(lookup, "COOKIES_RESEARCH_TIKA_TIMEOUT_SECONDS", 120),
			TikaMaxOutputBytes:           intValueOr(lookup, "COOKIES_RESEARCH_TIKA_MAX_OUTPUT_BYTES", 20*1024*1024),
			MCPStdioCommand:              strings.TrimSpace(valueOr(lookup, "COOKIES_RESEARCH_MCP_STDIO_COMMAND", "")),
			MCPToolName:                  valueOr(lookup, "COOKIES_RESEARCH_MCP_TOOL_NAME", "research"),
			MCPProtocolVersion:           valueOr(lookup, "COOKIES_RESEARCH_MCP_PROTOCOL_VERSION", "2025-11-25"),
			MCPEnvAllowlist:              splitCSV(valueOr(lookup, "COOKIES_RESEARCH_MCP_ENV_ALLOWLIST", "PATH,PATHEXT,SystemRoot,TEMP,TMP,ComSpec")),
			TimeoutSeconds:               intValueOr(lookup, "COOKIES_RESEARCH_TIMEOUT_SECONDS", 120),
			MaxOutputBytes:               intValueOr(lookup, "COOKIES_RESEARCH_MAX_OUTPUT_BYTES", 4*1024*1024),
		},
		Miyun: Miyun{
			Enabled: miyunEnabled, Endpoint: valueOr(lookup, "COOKIES_MIYUN_ENDPOINT", "https://api.youshu.youcloud.com/graphql"),
			MasterKey: valueOr(lookup, "COOKIES_MIYUN_MASTER_KEY", ""), MasterKeyVersion: valueOr(lookup, "COOKIES_MIYUN_MASTER_KEY_VERSION", "v1"),
			DownloadAllowedHosts: splitCSV(valueOr(lookup, "COOKIES_MIYUN_DOWNLOAD_ALLOWED_HOSTS", "")),
			MaxConcurrent:        intValueOr(lookup, "COOKIES_MIYUN_MAX_CONCURRENT", 1), RequestsPerSecond: intValueOr(lookup, "COOKIES_MIYUN_REQUESTS_PER_SECOND", 5),
			CooldownSeconds: intValueOr(lookup, "COOKIES_MIYUN_COOLDOWN_SECONDS", 300),
		},
		OceanEngine: OceanEngine{
			Enabled:             oceanEngineEnabled,
			WebAPIWriteEnabled:  oceanEngineWebAPIWriteEnabled,
			WebAPIWriteAccounts: splitCSV(valueOr(lookup, "COOKIES_OCEAN_ENGINE_WEB_API_WRITE_ACCOUNT_ALLOWLIST", "")),
			WebAPITemplateFile:  valueOr(lookup, "COOKIES_OCEAN_ENGINE_WEB_API_TEMPLATE_FILE", ""),
			BaseURL:             valueOr(lookup, "COOKIES_OCEAN_ENGINE_BASE_URL", "https://ad.oceanengine.com"),
			BusinessBaseURL:     valueOr(lookup, "COOKIES_OCEAN_ENGINE_BUSINESS_BASE_URL", "https://business.oceanengine.com"),
			MasterKey:           valueOr(lookup, "COOKIES_OCEAN_ENGINE_MASTER_KEY", ""),
			MasterKeyVersion:    valueOr(lookup, "COOKIES_OCEAN_ENGINE_MASTER_KEY_VERSION", "v1"),
			PatrolEnabled:       oceanEnginePatrolEnabled,
			PatrolIntervalHours: intValueOr(lookup, "COOKIES_OCEAN_ENGINE_PATROL_INTERVAL_HOURS", 24),
			PatrolLookbackDays:  intValueOr(lookup, "COOKIES_OCEAN_ENGINE_PATROL_LOOKBACK_DAYS", 14),
		},
		BrowserRPA: BrowserRPA{
			Enabled:               browserRpaEnabled,
			RunnerProtocol:        browserRpaProtocol,
			Command:               valueOr(lookup, "COOKIES_BROWSER_RPA_TSX_COMMAND", "node node_modules/tsx/dist/cli.mjs"),
			ScriptPath:            valueOr(lookup, "COOKIES_BROWSER_RPA_SCRIPT", browserRpaScript),
			EvidenceRoot:          valueOr(lookup, "COOKIES_BROWSER_RPA_EVIDENCE_ROOT", ".data/browser-rpa-evidence"),
			EdgeSessionFile:       valueOr(lookup, "COOKIES_BROWSER_RPA_EDGE_SESSION_FILE", filepath.Join(valueOr(lookup, "LOCALAPPDATA", ".data"), "cookies", "browser-rpa", "session.json")),
			SessionProbeScript:    valueOr(lookup, "COOKIES_BROWSER_RPA_SESSION_PROBE_SCRIPT", "scripts/browser-rpa-session-probe.ts"),
			AuthorityStateRoot:    valueOr(lookup, "COOKIES_BROWSER_RPA_AUTHORITY_STATE_ROOT", filepath.Join(valueOr(lookup, "LOCALAPPDATA", ".data"), "cookies", "browser-rpa", "authority-consumed")),
			PrepareTimeoutSeconds: intValueOr(lookup, "COOKIES_BROWSER_RPA_PREPARE_TIMEOUT_SECONDS", 180),
			SubmitTimeoutSeconds:  intValueOr(lookup, "COOKIES_BROWSER_RPA_SUBMIT_TIMEOUT_SECONDS", 300),
			CDPEndpointFallback:   browserRpaCDPFallback,
		},
		Provider: Provider{
			ImageAdapter:      valueOr(lookup, "COOKIES_PROVIDER_IMAGE_ADAPTER", "fake"),
			VideoAdapter:      valueOr(lookup, "COOKIES_PROVIDER_VIDEO_ADAPTER", "fake"),
			TextAdapter:       valueOr(lookup, "COOKIES_PROVIDER_TEXT_ADAPTER", "fake"),
			AudioAdapter:      valueOr(lookup, "COOKIES_PROVIDER_AUDIO_ADAPTER", "fake"),
			SpeechAdapter:     valueOr(lookup, "COOKIES_PROVIDER_SPEECH_ADAPTER", "fake"),
			MasterKey:         valueOr(lookup, "COOKIES_PROVIDER_MASTER_KEY", ""),
			MasterKeyVersion:  valueOr(lookup, "COOKIES_PROVIDER_MASTER_KEY_VERSION", "v1"),
			OutputBucket:      providerOutputBucket,
			AllowInsecureHTTP: boolValueOr(lookup, "COOKIES_PROVIDER_ALLOW_INSECURE_HTTP", false),
			ArkImage: ArkImage{
				APIKey:  valueOr(lookup, "COOKIES_ARK_IMAGE_API_KEY", ""),
				Model:   valueOr(lookup, "COOKIES_ARK_IMAGE_MODEL", ""),
				BaseURL: valueOr(lookup, "COOKIES_ARK_IMAGE_BASE_URL", ""),
			},
			ArkVideo: ArkVideo{
				APIKey:  valueOr(lookup, "COOKIES_ARK_VIDEO_API_KEY", ""),
				Model:   valueOr(lookup, "COOKIES_ARK_VIDEO_MODEL", ""),
				BaseURL: valueOr(lookup, "COOKIES_ARK_VIDEO_BASE_URL", ""),
			},
			ArkText: ArkText{
				APIKey:  valueOr(lookup, "COOKIES_ARK_TEXT_API_KEY", ""),
				Model:   valueOr(lookup, "COOKIES_ARK_TEXT_MODEL", ""),
				BaseURL: valueOr(lookup, "COOKIES_ARK_TEXT_BASE_URL", ""),
			},
			OpenAIImage: OpenAIImage{
				APIKey:  valueOr(lookup, "COOKIES_OPENAI_IMAGE_API_KEY", ""),
				Model:   valueOr(lookup, "COOKIES_OPENAI_IMAGE_MODEL", ""),
				BaseURL: valueOr(lookup, "COOKIES_OPENAI_IMAGE_BASE_URL", ""),
			},
			VolcengineASR: VolcengineASR{
				Endpoint:    valueOr(lookup, "COOKIES_VOLCENGINE_ASR_ENDPOINT", "https://openspeech.bytedance.com/api/v3/auc/bigmodel/recognize/flash"),
				AuthMode:    valueOr(lookup, "COOKIES_VOLCENGINE_ASR_AUTH_MODE", "legacy"),
				AppID:       valueOr(lookup, "COOKIES_VOLCENGINE_ASR_APP_ID", ""),
				AccessToken: valueOr(lookup, "COOKIES_VOLCENGINE_ASR_ACCESS_TOKEN", ""),
				APIKey:      valueOr(lookup, "COOKIES_VOLCENGINE_ASR_API_KEY", ""),
				ResourceID:  valueOr(lookup, "COOKIES_VOLCENGINE_ASR_RESOURCE_ID", "volc.bigasr.auc_turbo"),
				Model:       valueOr(lookup, "COOKIES_VOLCENGINE_ASR_MODEL", "bigmodel"),
			},
			VolcengineSpeech: VolcengineSpeech{
				Endpoint:     valueOr(lookup, "COOKIES_VOLCENGINE_SPEECH_ENDPOINT", "https://openspeech.bytedance.com/api/v3/tts/unidirectional"),
				APIKey:       valueOr(lookup, "COOKIES_VOLCENGINE_SPEECH_API_KEY", ""),
				ResourceID:   valueOr(lookup, "COOKIES_VOLCENGINE_SPEECH_RESOURCE_ID", "seed-tts-2.0"),
				DefaultVoice: valueOr(lookup, "COOKIES_VOLCENGINE_SPEECH_DEFAULT_VOICE", ""),
			},
		},
	}
	if raw := strings.TrimSpace(valueOr(lookup, "COOKIES_RESEARCH_MCP_STDIO_ARGS_JSON", "")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &config.Research.MCPStdioArgs); err != nil {
			return Config{}, fmt.Errorf("COOKIES_RESEARCH_MCP_STDIO_ARGS_JSON must be a JSON string array: %w", err)
		}
	}

	identityValues := map[string]string{
		"organization_id": valueOr(lookup, "COOKIES_LOCAL_ORGANIZATION_ID", ""),
		"principal_kind":  valueOr(lookup, "COOKIES_LOCAL_PRINCIPAL_KIND", ""),
		"principal_id":    valueOr(lookup, "COOKIES_LOCAL_PRINCIPAL_ID", ""),
		"project_id":      valueOr(lookup, "COOKIES_LOCAL_PROJECT_ID", ""),
		"scopes":          valueOr(lookup, "COOKIES_LOCAL_SCOPES", ""),
	}
	if anyValue(identityValues) {
		config.LocalIdentity = &LocalIdentity{
			OrganizationID: identityValues["organization_id"],
			PrincipalKind:  identityValues["principal_kind"],
			PrincipalID:    identityValues["principal_id"],
			ProjectID:      identityValues["project_id"],
			Scopes:         splitCSV(identityValues["scopes"]),
		}
	}

	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c Config) Validate() error {
	switch c.Environment {
	case EnvironmentLocal, EnvironmentTest, EnvironmentStaging, EnvironmentProduction:
	default:
		return fmt.Errorf("COOKIES_ENV must be one of local, test, staging, production")
	}
	if strings.TrimSpace(c.HTTPAddr) == "" {
		return fmt.Errorf("COOKIES_HTTP_ADDR must not be empty")
	}
	if strings.TrimSpace(c.MySQL.DSN) == "" {
		return fmt.Errorf("COOKIES_MYSQL_DSN must not be empty")
	}
	if c.MySQL.MaxOpenConns < 1 || c.MySQL.MaxIdleConns < 0 || c.MySQL.MaxIdleConns > c.MySQL.MaxOpenConns {
		return fmt.Errorf("MySQL connection pool limits are invalid")
	}
	if c.Auth.PasswordEnabled && (strings.TrimSpace(c.Auth.Username) == "" ||
		strings.TrimSpace(c.Auth.Password) == "" || c.Auth.SessionHours < 1 || c.Auth.SessionHours > 168) {
		return fmt.Errorf("password authentication requires username, password, and session hours between 1 and 168")
	}
	if c.Auth.PasswordEnabled && c.Environment != EnvironmentLocal && c.Environment != EnvironmentTest &&
		strings.EqualFold(strings.TrimSpace(c.Auth.Username), "Admin") && c.Auth.Password == "123456" {
		return fmt.Errorf("default local administrator credentials are forbidden outside local and test")
	}
	if c.ObjectStorage.Provider != "memory" && c.ObjectStorage.Provider != "filesystem" && c.ObjectStorage.Provider != "tos" {
		return fmt.Errorf("COOKIES_BLOB_PROVIDER must be memory, filesystem, or tos")
	}
	if strings.TrimSpace(c.ObjectStorage.QuarantineBucket) == "" || strings.TrimSpace(c.ObjectStorage.AssetsBucket) == "" {
		return fmt.Errorf("object storage requires quarantine and assets bucket names")
	}
	if c.ObjectStorage.Provider == "tos" && (c.ObjectStorage.Endpoint == "" || c.ObjectStorage.Region == "" || c.ObjectStorage.AccessKey == "" || c.ObjectStorage.SecretKey == "") {
		return fmt.Errorf("TOS storage requires endpoint, region, access key, and secret key")
	}
	if c.ObjectStorage.Provider == "filesystem" && strings.TrimSpace(c.ObjectStorage.FilesystemRoot) == "" {
		return fmt.Errorf("filesystem storage requires COOKIES_FILESYSTEM_BLOB_ROOT")
	}
	if c.Scanner.Mode != "noop" && c.Scanner.Mode != "clamav" {
		return fmt.Errorf("COOKIES_SCANNER_MODE must be noop or clamav")
	}
	if c.Scanner.Mode == "clamav" && c.Scanner.Address == "" {
		return fmt.Errorf("ClamAV scanner requires COOKIES_CLAMAV_ADDRESS")
	}
	if strings.TrimSpace(c.Media.VideoWorkRoot) == "" {
		return fmt.Errorf("COOKIES_VIDEO_WORK_ROOT must not be empty")
	}
	if strings.TrimSpace(c.Creative.ShortDramaPlannerModelAlias) == "" {
		return fmt.Errorf("COOKIES_CREATIVE_SHORT_DRAMA_PLANNER_MODEL_ALIAS must not be empty")
	}
	if strings.TrimSpace(c.Creative.GamePrerollPlannerModelAlias) == "" {
		return fmt.Errorf("COOKIES_CREATIVE_GAME_PREROLL_PLANNER_MODEL_ALIAS must not be empty")
	}
	if strings.TrimSpace(c.Creative.BrandFilmPlannerModelAlias) == "" {
		return fmt.Errorf("Creative brand film planner model alias is required")
	}
	if c.Creative.AINativeMaxActiveUnits < 1 || c.Creative.AINativeMaxActiveUnits > 10 {
		return fmt.Errorf("COOKIES_AI_AD_MAX_ACTIVE_UNITS must be between 1 and 10")
	}
	if c.Research.TimeoutSeconds < 1 || c.Research.TimeoutSeconds > 600 {
		return fmt.Errorf("COOKIES_RESEARCH_TIMEOUT_SECONDS must be between 1 and 600")
	}
	if c.Research.MaxOutputBytes < 1024 || c.Research.MaxOutputBytes > 16*1024*1024 {
		return fmt.Errorf("COOKIES_RESEARCH_MAX_OUTPUT_BYTES must be between 1024 and 16777216")
	}
	if strings.TrimSpace(c.Research.SeedModelAlias) == "" {
		return fmt.Errorf("COOKIES_RESEARCH_SEED_MODEL_ALIAS must not be empty")
	}
	if strings.TrimSpace(c.Research.DocumentVisionModelAlias) == "" {
		return fmt.Errorf("COOKIES_DOCUMENT_VISION_MODEL_ALIAS must not be empty")
	}
	if c.Research.DocumentVisionEnabled {
		if c.ObjectStorage.Provider != "tos" {
			return fmt.Errorf("COOKIES_DOCUMENT_VISION_ENABLED requires TOS object storage")
		}
		if c.ObjectStorage.AssetsBucket != c.ObjectStorage.QuarantineBucket ||
			c.ObjectStorage.AssetsBucket != c.Provider.OutputBucket {
			return fmt.Errorf("document vision requires one shared TOS bucket for quarantine, assets, and provider output")
		}
		if strings.TrimSpace(c.Provider.MasterKey) == "" {
			return fmt.Errorf("document vision requires COOKIES_PROVIDER_MASTER_KEY for encrypted LAS credentials")
		}
	}
	if c.Research.DocumentConverterEnabled {
		if !c.Research.DocumentVisionEnabled {
			return fmt.Errorf("COOKIES_DOCUMENT_CONVERTER_ENABLED requires COOKIES_DOCUMENT_VISION_ENABLED")
		}
		converterURL, err := url.Parse(strings.TrimSpace(c.Research.DocumentConverterBaseURL))
		if err != nil || converterURL.Host == "" || (converterURL.Scheme != "http" && converterURL.Scheme != "https") ||
			converterURL.User != nil || converterURL.RawQuery != "" || converterURL.Fragment != "" {
			return fmt.Errorf("COOKIES_DOCUMENT_CONVERTER_BASE_URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
		}
		if converterURL.Scheme == "http" && !c.Research.DocumentConverterAllowHTTP {
			return fmt.Errorf("HTTP document converter requires COOKIES_DOCUMENT_CONVERTER_ALLOW_INSECURE_HTTP=true")
		}
		if strings.TrimSpace(c.Research.DocumentConverterVersion) == "" {
			return fmt.Errorf("COOKIES_DOCUMENT_CONVERTER_VERSION must not be empty")
		}
		if c.Research.DocumentConverterTimeout < 10 || c.Research.DocumentConverterTimeout > 600 {
			return fmt.Errorf("COOKIES_DOCUMENT_CONVERTER_TIMEOUT_SECONDS must be between 10 and 600")
		}
		if c.Research.DocumentConverterMaxPDFBytes < 1024*1024 || c.Research.DocumentConverterMaxPDFBytes > 64*1024*1024 {
			return fmt.Errorf("COOKIES_DOCUMENT_CONVERTER_MAX_PDF_BYTES must be between 1048576 and 67108864")
		}
	}
	if c.Research.MaxConcurrent < 1 || c.Research.MaxConcurrent > 4 {
		return fmt.Errorf("COOKIES_RESEARCH_MAX_CONCURRENT must be between 1 and 4")
	}
	if c.Research.TikaTimeoutSeconds < 1 || c.Research.TikaTimeoutSeconds > 600 {
		return fmt.Errorf("COOKIES_RESEARCH_TIKA_TIMEOUT_SECONDS must be between 1 and 600")
	}
	if c.Research.TikaMaxOutputBytes < 1024 || c.Research.TikaMaxOutputBytes > 32*1024*1024 {
		return fmt.Errorf("COOKIES_RESEARCH_TIKA_MAX_OUTPUT_BYTES must be between 1024 and 33554432")
	}
	if c.Research.TikaEnabled {
		tikaURL, err := url.Parse(strings.TrimSpace(c.Research.TikaBaseURL))
		if err != nil || (tikaURL.Scheme != "http" && tikaURL.Scheme != "https") || tikaURL.Host == "" {
			return fmt.Errorf("COOKIES_RESEARCH_TIKA_BASE_URL must be an absolute HTTP(S) URL")
		}
		if strings.TrimSpace(c.Research.TikaVersion) == "" {
			return fmt.Errorf("COOKIES_RESEARCH_TIKA_VERSION must not be empty")
		}
	}
	if c.Research.MCPStdioCommand != "" &&
		(strings.TrimSpace(c.Research.MCPToolName) == "" || strings.TrimSpace(c.Research.MCPProtocolVersion) == "") {
		return fmt.Errorf("MCP stdio research requires a tool name and protocol version")
	}
	if c.Miyun.MaxConcurrent < 1 || c.Miyun.MaxConcurrent > 2 {
		return fmt.Errorf("COOKIES_MIYUN_MAX_CONCURRENT must be between 1 and 2")
	}
	if c.Miyun.RequestsPerSecond < 1 || c.Miyun.RequestsPerSecond > 8 {
		return fmt.Errorf("COOKIES_MIYUN_REQUESTS_PER_SECOND must be between 1 and 8")
	}
	if c.Miyun.CooldownSeconds < 60 || c.Miyun.CooldownSeconds > 3600 {
		return fmt.Errorf("COOKIES_MIYUN_COOLDOWN_SECONDS must be between 60 and 3600")
	}
	if c.Miyun.Enabled {
		endpoint, err := url.Parse(strings.TrimSpace(c.Miyun.Endpoint))
		if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil {
			return fmt.Errorf("COOKIES_MIYUN_ENDPOINT must be an absolute HTTPS URL")
		}
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(c.Miyun.MasterKey))
		if err != nil || len(key) != 32 || strings.TrimSpace(c.Miyun.MasterKeyVersion) == "" {
			return fmt.Errorf("COOKIES_MIYUN_MASTER_KEY must be a base64-encoded 32-byte key with a version")
		}
		if len(c.Miyun.DownloadAllowedHosts) == 0 {
			return fmt.Errorf("COOKIES_MIYUN_DOWNLOAD_ALLOWED_HOSTS is required when Miyun is enabled")
		}
		for _, host := range c.Miyun.DownloadAllowedHosts {
			if strings.TrimSpace(host) == "" || strings.ContainsAny(host, "/@?#") {
				return fmt.Errorf("COOKIES_MIYUN_DOWNLOAD_ALLOWED_HOSTS must contain hostnames only")
			}
		}
	}
	if c.OceanEngine.Enabled {
		endpoint, err := url.Parse(strings.TrimSpace(c.OceanEngine.BaseURL))
		if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil {
			return fmt.Errorf("COOKIES_OCEAN_ENGINE_BASE_URL must be an absolute HTTPS URL")
		}
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(c.OceanEngine.MasterKey))
		if err != nil || len(key) != 32 || strings.TrimSpace(c.OceanEngine.MasterKeyVersion) == "" {
			return fmt.Errorf("COOKIES_OCEAN_ENGINE_MASTER_KEY must be a base64-encoded 32-byte key with a version")
		}
		if c.OceanEngine.PatrolEnabled && (c.OceanEngine.PatrolIntervalHours < 1 || c.OceanEngine.PatrolIntervalHours > 168) {
			return fmt.Errorf("COOKIES_OCEAN_ENGINE_PATROL_INTERVAL_HOURS must be from 1 through 168")
		}
		if c.OceanEngine.PatrolEnabled && (c.OceanEngine.PatrolLookbackDays < 2 || c.OceanEngine.PatrolLookbackDays > 30) {
			return fmt.Errorf("COOKIES_OCEAN_ENGINE_PATROL_LOOKBACK_DAYS must be from 2 through 30")
		}
	}
	if c.OceanEngine.WebAPIWriteEnabled && (!c.OceanEngine.Enabled || len(c.OceanEngine.WebAPIWriteAccounts) == 0) {
		return fmt.Errorf("COOKIES_OCEAN_ENGINE_WEB_API_WRITE_ENABLED requires Ocean Engine and a non-empty account allowlist")
	}
	if c.Provider.ImageAdapter != "fake" && c.Provider.ImageAdapter != "ark_image" && c.Provider.ImageAdapter != "openai_image" && c.Provider.ImageAdapter != "adapter_gateway" {
		return fmt.Errorf("COOKIES_PROVIDER_IMAGE_ADAPTER must be fake, ark_image, openai_image, or adapter_gateway")
	}
	if c.Provider.TextAdapter != "fake" && c.Provider.TextAdapter != "adapter_gateway" && c.Provider.TextAdapter != "ark_text" {
		return fmt.Errorf("COOKIES_PROVIDER_TEXT_ADAPTER must be fake, adapter_gateway, or ark_text")
	}
	if c.Provider.VideoAdapter != "fake" && c.Provider.VideoAdapter != "adapter_gateway" {
		return fmt.Errorf("COOKIES_PROVIDER_VIDEO_ADAPTER must be fake or adapter_gateway; direct video providers are not allowed")
	}
	if c.Provider.AudioAdapter != "fake" && c.Provider.AudioAdapter != "volcengine_asr" {
		return fmt.Errorf("COOKIES_PROVIDER_AUDIO_ADAPTER must be fake or volcengine_asr")
	}
	if c.Provider.SpeechAdapter != "fake" && c.Provider.SpeechAdapter != "volcengine_speech" && c.Provider.SpeechAdapter != "minimax_speech" {
		return fmt.Errorf("COOKIES_PROVIDER_SPEECH_ADAPTER must be fake, volcengine_speech, or minimax_speech")
	}
	if c.Strategy.RealProviderEnabled && c.Provider.TextAdapter != "adapter_gateway" && c.Provider.TextAdapter != "ark_text" {
		return fmt.Errorf("COOKIES_STRATEGY_REAL_PROVIDER_ENABLED requires a real text adapter")
	}
	if strings.TrimSpace(c.MediaUnderstanding.VisionModelAlias) == "" {
		return fmt.Errorf("COOKIES_MEDIA_UNDERSTANDING_VISION_MODEL_ALIAS must not be empty")
	}
	if c.MediaUnderstanding.RealProviderEnabled && c.Provider.TextAdapter != "adapter_gateway" {
		return fmt.Errorf("COOKIES_MEDIA_UNDERSTANDING_REAL_PROVIDER_ENABLED requires COOKIES_PROVIDER_TEXT_ADAPTER=adapter_gateway")
	}
	if c.MediaUnderstanding.ASREnabled && c.Provider.AudioAdapter != "volcengine_asr" {
		return fmt.Errorf("COOKIES_MEDIA_UNDERSTANDING_ASR_ENABLED requires COOKIES_PROVIDER_AUDIO_ADAPTER=volcengine_asr")
	}
	if strings.TrimSpace(c.Strategy.TextModelAlias) == "" {
		return fmt.Errorf("COOKIES_STRATEGY_TEXT_MODEL_ALIAS must not be empty")
	}
	if strings.TrimSpace(c.Strategy.LiteTextModelAlias) == "" {
		return fmt.Errorf("COOKIES_STRATEGY_LITE_TEXT_MODEL_ALIAS must not be empty")
	}
	if strings.TrimSpace(c.Strategy.DeepReviewModelAlias) == "" {
		return fmt.Errorf("COOKIES_STRATEGY_DEEP_REVIEW_MODEL_ALIAS must not be empty")
	}
	if strings.TrimSpace(c.Strategy.PromptVersion) == "" {
		return fmt.Errorf("COOKIES_STRATEGY_PROMPT_VERSION must not be empty")
	}
	if !oneOf(c.Strategy.PromptVersion, "strategy.generate.v2", "strategy.generate.v3", "strategy.generate.v4") {
		return fmt.Errorf("COOKIES_STRATEGY_PROMPT_VERSION is unsupported")
	}
	if strings.TrimSpace(c.Strategy.ConversationPromptVersion) == "" {
		return fmt.Errorf("COOKIES_STRATEGY_CONVERSATION_PROMPT_VERSION must not be empty")
	}
	if !oneOf(c.Strategy.ConversationPromptVersion, "strategy.conversation.v3", "strategy.conversation.v4", "strategy.conversation.v5", "strategy.conversation.v6") {
		return fmt.Errorf("COOKIES_STRATEGY_CONVERSATION_PROMPT_VERSION is unsupported")
	}
	if strings.TrimSpace(c.Strategy.RevisePromptVersion) == "" {
		return fmt.Errorf("COOKIES_STRATEGY_REVISE_PROMPT_VERSION must not be empty")
	}
	if !oneOf(c.Strategy.RevisePromptVersion, "strategy.revise.v2", "strategy.revise.v3") {
		return fmt.Errorf("COOKIES_STRATEGY_REVISE_PROMPT_VERSION is unsupported")
	}
	if strings.TrimSpace(c.Strategy.ReviewPromptVersion) == "" {
		return fmt.Errorf("COOKIES_STRATEGY_REVIEW_PROMPT_VERSION must not be empty")
	}
	if !oneOf(c.Strategy.ReviewPromptVersion, "strategy.review.deep.v1", "strategy.review.deep.v2") {
		return fmt.Errorf("COOKIES_STRATEGY_REVIEW_PROMPT_VERSION is unsupported")
	}
	if strings.TrimSpace(c.Strategy.RepairPromptVersion) == "" {
		return fmt.Errorf("COOKIES_STRATEGY_REPAIR_PROMPT_VERSION must not be empty")
	}
	if !oneOf(c.Strategy.RepairPromptVersion, "strategy.repair.v1", "strategy.repair.v2") {
		return fmt.Errorf("COOKIES_STRATEGY_REPAIR_PROMPT_VERSION is unsupported")
	}
	if c.Strategy.CreativeTaskPromptVersion != "strategy.creative_task.generate.v2" {
		return fmt.Errorf("COOKIES_STRATEGY_CREATIVE_TASK_PROMPT_VERSION is unsupported")
	}
	if c.Strategy.CriticEnabled && !c.Strategy.RealProviderEnabled {
		return fmt.Errorf("COOKIES_STRATEGY_CRITIC_ENABLED requires COOKIES_STRATEGY_REAL_PROVIDER_ENABLED=true")
	}
	if c.Provider.ImageAdapter == "ark_image" && (c.Environment != EnvironmentLocal || strings.TrimSpace(c.Provider.ArkImage.APIKey) == "" || strings.TrimSpace(c.Provider.ArkImage.Model) == "") {
		return fmt.Errorf("ark_image is local-only and requires COOKIES_ARK_IMAGE_API_KEY and COOKIES_ARK_IMAGE_MODEL")
	}
	if c.Provider.ImageAdapter == "openai_image" && (c.Environment != EnvironmentLocal || strings.TrimSpace(c.Provider.OpenAIImage.APIKey) == "" || strings.TrimSpace(c.Provider.OpenAIImage.Model) == "" || strings.TrimSpace(c.Provider.OpenAIImage.BaseURL) == "") {
		return fmt.Errorf("openai_image is local-only and requires COOKIES_OPENAI_IMAGE_API_KEY, COOKIES_OPENAI_IMAGE_MODEL, and COOKIES_OPENAI_IMAGE_BASE_URL")
	}
	if c.Provider.TextAdapter == "ark_text" && (c.Environment != EnvironmentLocal || strings.TrimSpace(c.Provider.ArkText.APIKey) == "" || strings.TrimSpace(c.Provider.ArkText.Model) == "") {
		return fmt.Errorf("ark_text is local-only and requires COOKIES_ARK_TEXT_API_KEY and COOKIES_ARK_TEXT_MODEL")
	}
	if c.Provider.AudioAdapter == "volcengine_asr" {
		endpoint, err := url.Parse(c.Provider.VolcengineASR.Endpoint)
		if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" {
			return fmt.Errorf("COOKIES_VOLCENGINE_ASR_ENDPOINT must be an absolute HTTPS URL")
		}
		if strings.TrimSpace(c.Provider.VolcengineASR.ResourceID) == "" || strings.TrimSpace(c.Provider.VolcengineASR.Model) == "" {
			return fmt.Errorf("volcengine_asr requires resource ID and model")
		}
		switch c.Provider.VolcengineASR.AuthMode {
		case "legacy":
			if strings.TrimSpace(c.Provider.VolcengineASR.AppID) == "" || strings.TrimSpace(c.Provider.VolcengineASR.AccessToken) == "" {
				return fmt.Errorf("legacy volcengine_asr requires COOKIES_VOLCENGINE_ASR_APP_ID and COOKIES_VOLCENGINE_ASR_ACCESS_TOKEN")
			}
		case "api_key":
			if strings.TrimSpace(c.Provider.VolcengineASR.APIKey) == "" {
				return fmt.Errorf("api_key volcengine_asr requires COOKIES_VOLCENGINE_ASR_API_KEY")
			}
		default:
			return fmt.Errorf("COOKIES_VOLCENGINE_ASR_AUTH_MODE must be legacy or api_key")
		}
	}
	if c.Provider.SpeechAdapter == "volcengine_speech" {
		endpoint, err := url.Parse(c.Provider.VolcengineSpeech.Endpoint)
		if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" {
			return fmt.Errorf("COOKIES_VOLCENGINE_SPEECH_ENDPOINT must be an absolute HTTPS URL")
		}
		if strings.TrimSpace(c.Provider.VolcengineSpeech.APIKey) == "" || strings.TrimSpace(c.Provider.VolcengineSpeech.ResourceID) == "" || strings.TrimSpace(c.Provider.VolcengineSpeech.DefaultVoice) == "" {
			return fmt.Errorf("volcengine_speech requires API key, resource ID and default voice")
		}
	}
	usesGenerationBroker := c.Provider.ImageAdapter == "adapter_gateway" ||
		c.Provider.TextAdapter == "adapter_gateway" ||
		c.Provider.VideoAdapter == "adapter_gateway" ||
		c.Provider.SpeechAdapter == "minimax_speech"
	usesCredentialBroker := usesGenerationBroker || c.Research.SeedEnabled
	if usesCredentialBroker && (strings.TrimSpace(c.Provider.MasterKey) == "" || strings.TrimSpace(c.Provider.MasterKeyVersion) == "") {
		return fmt.Errorf("configured Provider adapter requires COOKIES_PROVIDER_MASTER_KEY and COOKIES_PROVIDER_MASTER_KEY_VERSION")
	}
	if usesCredentialBroker {
		key, err := base64.StdEncoding.DecodeString(c.Provider.MasterKey)
		if err != nil || len(key) != 32 {
			return fmt.Errorf("COOKIES_PROVIDER_MASTER_KEY must be base64-encoded 32 bytes")
		}
		if usesGenerationBroker && strings.TrimSpace(c.Provider.OutputBucket) == "" {
			return fmt.Errorf("adapter_gateway requires an output bucket")
		}
		if c.Provider.AllowInsecureHTTP && c.Environment != EnvironmentLocal {
			return fmt.Errorf("COOKIES_PROVIDER_ALLOW_INSECURE_HTTP is permitted only when COOKIES_ENV=local")
		}
	}
	if c.Environment == EnvironmentProduction {
		if c.ObjectStorage.Provider != "tos" {
			return fmt.Errorf("production requires TOS object storage")
		}
		if c.Scanner.Mode != "clamav" {
			return fmt.Errorf("production requires ClamAV content scanning")
		}
	}
	if c.LocalIdentity == nil {
		return nil
	}
	if c.Environment != EnvironmentLocal {
		return fmt.Errorf("local identity is only permitted when COOKIES_ENV=local")
	}
	if strings.TrimSpace(c.LocalIdentity.OrganizationID) == "" ||
		strings.TrimSpace(c.LocalIdentity.PrincipalKind) == "" ||
		strings.TrimSpace(c.LocalIdentity.PrincipalID) == "" || strings.TrimSpace(c.LocalIdentity.ProjectID) == "" {
		return fmt.Errorf("local identity requires organization, principal kind, principal ID, and project ID")
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if strings.TrimSpace(value) == candidate {
			return true
		}
	}
	return false
}

func valueOr(lookup func(string) (string, bool), key, fallback string) string {
	if value, ok := lookup(key); ok {
		return strings.TrimSpace(value)
	}
	return fallback
}

// valueOrCompatibility lets deployments migrate from generic object-storage
// names without changing the existing COOKIES_TOS_* configuration contract.
func valueOrCompatibility(lookup func(string) (string, bool), key string, compatibilityKeys ...string) string {
	if value, ok := lookup(key); ok {
		return strings.TrimSpace(value)
	}
	for _, compatibilityKey := range compatibilityKeys[:len(compatibilityKeys)-1] {
		if value, ok := lookup(compatibilityKey); ok {
			return strings.TrimSpace(value)
		}
	}
	return compatibilityKeys[len(compatibilityKeys)-1]
}

func anyValue(values map[string]string) bool {
	for _, value := range values {
		if value != "" {
			return true
		}
	}
	return false
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func intValueOr(lookup func(string) (string, bool), key string, fallback int) int {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	var result int
	if _, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &result); err != nil {
		return -1
	}
	return result
}

func boolValueOr(lookup func(string) (string, bool), key string, fallback bool) bool {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func strictBoolValueOr(lookup func(string) (string, bool), key string, fallback bool) (bool, error) {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return parsed, nil
}
