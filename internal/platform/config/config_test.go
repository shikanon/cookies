package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestStrategyRolloutDefaultsAreSafe(t *testing.T) {
	t.Parallel()
	value, err := FromLookup(mapLookup(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !value.Strategy.Enabled || value.Strategy.RealProviderEnabled || !value.Strategy.ApproveEnabled ||
		value.Strategy.PackageToCreativeEnabled || value.Strategy.CriticEnabled ||
		!value.Strategy.ContextSelectionEnabled ||
		value.Strategy.TextModelAlias != "cookies.text.standard" ||
		value.Strategy.LiteTextModelAlias != "cookies.text.lite" ||
		value.Strategy.DeepReviewModelAlias != "cookies.text.deep_review" ||
		value.Strategy.PromptVersion != "strategy.generate.v4" ||
		value.Strategy.ConversationPromptVersion != "strategy.conversation.v6" ||
		value.Strategy.RevisePromptVersion != "strategy.revise.v3" ||
		value.Strategy.ReviewPromptVersion != "strategy.review.deep.v2" ||
		value.Strategy.RepairPromptVersion != "strategy.repair.v2" ||
		!value.Strategy.CreativeTaskPlanningEnabled ||
		!value.Strategy.QuickViralRemakeEnabled ||
		value.Strategy.CreativeTaskPromptVersion != "strategy.creative_task.generate.v2" ||
		len(value.Strategy.OrganizationAllowlist) != 0 {
		t.Fatalf("unexpected Strategy defaults: %#v", value.Strategy)
	}
	if value.Research.SeedModelAlias != "cookies.research.web.standard" ||
		value.Research.DocumentVisionModelAlias != "cookies.document.vision.standard" {
		t.Fatalf("unexpected fixed research/document aliases: %#v", value.Research)
	}
	if !strings.Contains(value.MySQL.DSN, "127.0.0.1:3307") {
		t.Fatalf("default MySQL DSN does not use the isolated local port: %q", value.MySQL.DSN)
	}
	if value.Media.FFmpegPath != "" || value.Media.FFprobePath != "" || value.Media.VideoWorkRoot != ".data/video-work" {
		t.Fatalf("unexpected safe media defaults: %#v", value.Media)
	}
	if value.Provider.AudioAdapter != "fake" {
		t.Fatalf("unexpected audio adapter default: %q", value.Provider.AudioAdapter)
	}
	if value.Provider.SpeechAdapter != "fake" {
		t.Fatalf("unexpected speech adapter default: %q", value.Provider.SpeechAdapter)
	}
	if !value.Creative.ShortDramaModelPlannerEnabled ||
		value.Creative.ShortDramaPlannerModelAlias != "cookies.text.standard" ||
		!value.Creative.GamePrerollModelPlannerEnabled ||
		value.Creative.GamePrerollPlannerModelAlias != "cookies.text.standard" ||
		!value.Creative.BrandFilmModelPlannerEnabled ||
		value.Creative.BrandFilmPlannerModelAlias != "cookies.text.standard" {
		t.Fatalf("unexpected Creative planner defaults: %#v", value.Creative)
	}
	if value.BrowserRPA.RunnerProtocol != "v3" || value.BrowserRPA.ScriptPath != "scripts/browser-rpa-runner-v3.ts" || value.BrowserRPA.SessionProbeScript != "scripts/browser-rpa-session-probe.ts" || !strings.HasSuffix(strings.ReplaceAll(value.BrowserRPA.EdgeSessionFile, "\\", "/"), "cookies/browser-rpa/session.json") {
		t.Fatalf("unexpected Browser RPA defaults: %#v", value.BrowserRPA)
	}
}

func TestBrowserRPALegacyRunnerIsAnExplicitRollbackMode(t *testing.T) {
	value, err := FromLookup(mapLookup(map[string]string{"COOKIES_BROWSER_RPA_RUNNER_PROTOCOL": "legacy"}))
	if err != nil {
		t.Fatal(err)
	}
	if value.BrowserRPA.ScriptPath != "scripts/browser-rpa-runner.ts" {
		t.Fatalf("legacy script path = %q", value.BrowserRPA.ScriptPath)
	}
	if _, err := FromLookup(mapLookup(map[string]string{"COOKIES_BROWSER_RPA_RUNNER_PROTOCOL": "mixed"})); err == nil {
		t.Fatal("unknown Browser RPA protocol must fail")
	}
}

func TestOceanEngineWebAPIWriteDefaultsOff(t *testing.T) {
	value, err := FromLookup(mapLookup(nil))
	if err != nil {
		t.Fatal(err)
	}
	if value.OceanEngine.WebAPIWriteEnabled || len(value.OceanEngine.WebAPIWriteAccounts) != 0 {
		t.Fatalf("unexpected Ocean Engine Web API defaults: %#v", value.OceanEngine)
	}
}

func TestOceanEngineWebAPIWriteRequiresAllowlist(t *testing.T) {
	values := map[string]string{
		"COOKIES_OCEAN_ENGINE_WEB_API_WRITE_ENABLED": "true",
	}
	_, err := FromLookup(mapLookup(values))
	if err == nil || !strings.Contains(err.Error(), "account allowlist") {
		t.Fatalf("error=%v", err)
	}
}

func TestVolcengineSpeechConfigurationRequiresIndependentAPIKeyAndVoice(t *testing.T) {
	_, err := FromLookup(mapLookup(map[string]string{"COOKIES_PROVIDER_SPEECH_ADAPTER": "volcengine_speech"}))
	if err == nil {
		t.Fatal("speech adapter without speech credentials must be rejected")
	}
	value, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_PROVIDER_SPEECH_ADAPTER":         "volcengine_speech",
		"COOKIES_VOLCENGINE_SPEECH_API_KEY":       "speech-key",
		"COOKIES_VOLCENGINE_SPEECH_RESOURCE_ID":   "seed-tts-2.0",
		"COOKIES_VOLCENGINE_SPEECH_DEFAULT_VOICE": "zh_female_test",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if value.Provider.VolcengineSpeech.APIKey != "speech-key" || value.Provider.VolcengineSpeech.ResourceID != "seed-tts-2.0" {
		t.Fatalf("unexpected speech configuration: %#v", value.Provider.VolcengineSpeech)
	}
}

func TestStrategyRejectsUnknownPromptVersion(t *testing.T) {
	t.Parallel()
	_, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_STRATEGY_REVIEW_PROMPT_VERSION": "strategy.review.deep.unknown",
	}))
	if err == nil || !strings.Contains(err.Error(), "COOKIES_STRATEGY_REVIEW_PROMPT_VERSION") {
		t.Fatalf("err = %v", err)
	}
}

func TestProductionKeepsPreviousStrategyPromptDefaults(t *testing.T) {
	t.Parallel()
	values := secureProductionValues()
	config, err := FromLookup(mapLookup(values))
	if err != nil {
		t.Fatal(err)
	}
	if config.Strategy.PromptVersion != "strategy.generate.v2" ||
		config.Strategy.ConversationPromptVersion != "strategy.conversation.v3" ||
		config.Strategy.RevisePromptVersion != "strategy.revise.v2" ||
		config.Strategy.ReviewPromptVersion != "strategy.review.deep.v1" ||
		config.Strategy.RepairPromptVersion != "strategy.repair.v1" ||
		config.Strategy.ContextSelectionEnabled || config.Strategy.CreativeTaskPlanningEnabled {
		t.Fatalf("production prompt defaults = %#v", config.Strategy)
	}
	if config.Strategy.QuickViralRemakeEnabled {
		t.Fatalf("production quick viral remake must start disabled: %#v", config.Strategy)
	}
}

func TestStrategyContextSelectionRejectsInvalidBoolean(t *testing.T) {
	t.Parallel()
	_, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_STRATEGY_CONTEXT_SELECTION_ENABLED": "sometimes",
	}))
	if err == nil || !strings.Contains(err.Error(), "COOKIES_STRATEGY_CONTEXT_SELECTION_ENABLED") {
		t.Fatalf("err = %v", err)
	}
}

func TestStrategyQuickViralRemakeRejectsInvalidBoolean(t *testing.T) {
	t.Parallel()
	_, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_STRATEGY_QUICK_VIRAL_REMAKE_ENABLED": "sometimes",
	}))
	if err == nil || !strings.Contains(err.Error(), "COOKIES_STRATEGY_QUICK_VIRAL_REMAKE_ENABLED") {
		t.Fatalf("err = %v", err)
	}
}

func TestPasswordAuthenticationDefaultsToLocalOnly(t *testing.T) {
	t.Parallel()
	local, err := FromLookup(mapLookup(nil))
	if err != nil || !local.Auth.PasswordEnabled {
		t.Fatalf("local password authentication default = %#v, %v", local.Auth, err)
	}
	productionValues := secureProductionValues()
	production, err := FromLookup(mapLookup(productionValues))
	if err != nil || production.Auth.PasswordEnabled {
		t.Fatalf("production password authentication default = %#v, %v", production.Auth, err)
	}
	productionValues["COOKIES_PASSWORD_AUTH_ENABLED"] = "true"
	if _, err := FromLookup(mapLookup(productionValues)); err == nil {
		t.Fatal("production accepted the local default administrator password")
	}
}

func TestStrategyRolloutAllowsExplicitCreativeIntegration(t *testing.T) {
	t.Parallel()
	value, err := FromLookup(mapLookup(map[string]string{"COOKIES_STRATEGY_PACKAGE_TO_CREATIVE_ENABLED": "true"}))
	if err != nil || !value.Strategy.PackageToCreativeEnabled {
		t.Fatalf("expected explicit Strategy-to-Creative integration to be enabled: %#v, %v", value.Strategy, err)
	}
}

func TestCreativeImageFontUsesUnifiedConfigurationLookup(t *testing.T) {
	t.Parallel()
	value, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_CREATIVE_IMAGE_FONT_PATH":   "C:/fonts/chinese.ttf",
		"COOKIES_CREATIVE_IMAGE_FONT_SHA256": "ABCDEF",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if value.Creative.ImageFontPath != "C:/fonts/chinese.ttf" || value.Creative.ImageFontSHA256 != "abcdef" {
		t.Fatalf("unexpected Creative image font config: %#v", value.Creative)
	}
}

func TestStrategyRolloutRejectsInvalidBoolean(t *testing.T) {
	t.Parallel()
	_, err := FromLookup(mapLookup(map[string]string{"COOKIES_STRATEGY_APPROVE_ENABLED": "tru"}))
	if err == nil {
		t.Fatal("expected an invalid approval flag to fail closed")
	}
}

func TestStrategyCriticRequiresRealProvider(t *testing.T) {
	t.Parallel()
	_, err := FromLookup(mapLookup(map[string]string{"COOKIES_STRATEGY_CRITIC_ENABLED": "true"}))
	if err == nil {
		t.Fatal("expected Strategy critic without a real provider to be rejected")
	}
}

func TestAdapterGatewayRequiresExternalMasterKeyAndSupportsProduction(t *testing.T) {
	t.Parallel()
	_, err := FromLookup(mapLookup(map[string]string{"COOKIES_PROVIDER_IMAGE_ADAPTER": "adapter_gateway"}))
	if err == nil {
		t.Fatal("expected adapter gateway without a master key to be rejected")
	}
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	config, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_ENV": "production", "COOKIES_BLOB_PROVIDER": "tos",
		"COOKIES_TOS_ENDPOINT": "tos.example.com", "COOKIES_TOS_REGION": "cn-test",
		"COOKIES_TOS_ACCESS_KEY": "key", "COOKIES_TOS_SECRET_KEY": "secret",
		"COOKIES_SCANNER_MODE": "clamav", "COOKIES_CLAMAV_ADDRESS": "127.0.0.1:3310",
		"COOKIES_PROVIDER_IMAGE_ADAPTER": "adapter_gateway",
		"COOKIES_PROVIDER_MASTER_KEY":    key, "COOKIES_PROVIDER_MASTER_KEY_VERSION": "kms-v1",
	}))
	if err != nil || config.Provider.ImageAdapter != "adapter_gateway" {
		t.Fatalf("valid production adapter gateway config rejected: config=%#v err=%v", config.Provider, err)
	}
}

func TestAdapterGatewayAllowsInsecureHTTPOnlyForLocalIntegration(t *testing.T) {
	t.Parallel()
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	local, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_ENV": "local", "COOKIES_PROVIDER_IMAGE_ADAPTER": "adapter_gateway",
		"COOKIES_PROVIDER_MASTER_KEY":          key,
		"COOKIES_PROVIDER_ALLOW_INSECURE_HTTP": "true",
	}))
	if err != nil || !local.Provider.AllowInsecureHTTP {
		t.Fatalf("local insecure HTTP config rejected: config=%#v err=%v", local.Provider, err)
	}
	_, err = FromLookup(mapLookup(map[string]string{
		"COOKIES_ENV": "staging", "COOKIES_PROVIDER_IMAGE_ADAPTER": "adapter_gateway",
		"COOKIES_PROVIDER_MASTER_KEY":          key,
		"COOKIES_PROVIDER_ALLOW_INSECURE_HTTP": "true",
	}))
	if err == nil {
		t.Fatal("staging accepted local-only insecure HTTP setting")
	}
}

func TestParseDotEnvAcceptsLocalDevelopmentValues(t *testing.T) {
	t.Parallel()
	values, err := parseDotEnv(strings.NewReader("# local only\nCOOKIES_MYSQL_DSN='cookies:pass@tcp(127.0.0.1:3307)/cookies?parseTime=true'\nexport COOKIES_HTTP_ADDR=:8080\n"))
	if err != nil {
		t.Fatalf("parseDotEnv() error = %v", err)
	}
	if values["COOKIES_MYSQL_DSN"] != "cookies:pass@tcp(127.0.0.1:3307)/cookies?parseTime=true" || values["COOKIES_HTTP_ADDR"] != ":8080" {
		t.Fatalf("unexpected dotenv values: %#v", values)
	}
}

func TestParseDotEnvAcceptsUTF8BOM(t *testing.T) {
	t.Parallel()
	values, err := parseDotEnv(strings.NewReader("\uFEFF# local only\nCOOKIES_ENV=local\n"))
	if err != nil {
		t.Fatalf("parseDotEnv() error = %v", err)
	}
	if values["COOKIES_ENV"] != "local" {
		t.Fatalf("COOKIES_ENV = %q, want local", values["COOKIES_ENV"])
	}
}

func TestParseDotEnvRejectsMalformedLine(t *testing.T) {
	t.Parallel()
	if _, err := parseDotEnv(strings.NewReader("COOKIES_ENV\n")); err == nil {
		t.Fatal("parseDotEnv() error = nil, want malformed line rejection")
	}
}

func TestFromLookupRejectsLocalIdentityOutsideLocal(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"COOKIES_ENV":                   "production",
		"COOKIES_LOCAL_ORGANIZATION_ID": "org_1",
		"COOKIES_LOCAL_PRINCIPAL_KIND":  "user",
		"COOKIES_LOCAL_PRINCIPAL_ID":    "usr_1",
	}
	_, err := FromLookup(mapLookup(values))
	if err == nil {
		t.Fatal("expected production local identity to be rejected")
	}
}

func TestFromLookupBuildsExplicitLocalIdentity(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"COOKIES_ENV":                   "local",
		"COOKIES_HTTP_ADDR":             "127.0.0.1:8081",
		"COOKIES_LOCAL_ORGANIZATION_ID": "org_1",
		"COOKIES_LOCAL_PRINCIPAL_KIND":  "user",
		"COOKIES_LOCAL_PRINCIPAL_ID":    "usr_1",
		"COOKIES_LOCAL_PROJECT_ID":      "project_1",
		"COOKIES_LOCAL_SCOPES":          "project.read, project.write",
	}
	config, err := FromLookup(mapLookup(values))
	if err != nil {
		t.Fatalf("FromLookup() error = %v", err)
	}
	if config.HTTPAddr != "127.0.0.1:8081" || config.LocalIdentity == nil {
		t.Fatalf("unexpected config: %#v", config)
	}
	if config.ObjectStorage.Provider != "filesystem" || config.ObjectStorage.FilesystemRoot != ".data/blobs" {
		t.Fatalf("unexpected local object storage: %#v", config.ObjectStorage)
	}
	if got, want := len(config.LocalIdentity.Scopes), 2; got != want {
		t.Fatalf("scope count = %d, want %d", got, want)
	}
}

func TestFromLookupRejectsInvalidMySQLPool(t *testing.T) {
	t.Parallel()

	_, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_MYSQL_MAX_OPEN_CONNS": "0",
	}))
	if err == nil {
		t.Fatal("expected invalid MySQL connection pool to be rejected")
	}
}

func TestProductionRequiresTOSAndMalwareScanning(t *testing.T) {
	t.Parallel()
	_, err := FromLookup(mapLookup(map[string]string{"COOKIES_ENV": "production"}))
	if err == nil {
		t.Fatal("expected insecure production storage defaults to be rejected")
	}
	config, err := FromLookup(mapLookup(map[string]string{"COOKIES_ENV": "production", "COOKIES_BLOB_PROVIDER": "tos", "COOKIES_TOS_ENDPOINT": "tos.example.com", "COOKIES_TOS_REGION": "cn-test", "COOKIES_TOS_ACCESS_KEY": "key", "COOKIES_TOS_SECRET_KEY": "secret", "COOKIES_SCANNER_MODE": "clamav", "COOKIES_CLAMAV_ADDRESS": "127.0.0.1:3310"}))
	if err != nil {
		t.Fatalf("secure production config rejected: %v", err)
	}
	if config.ObjectStorage.Provider != "tos" || config.Scanner.Mode != "clamav" {
		t.Fatalf("unexpected config: %#v", config)
	}
}

func TestArkImageAdapterIsExplicitAndLocalOnly(t *testing.T) {
	t.Parallel()
	_, err := FromLookup(mapLookup(map[string]string{"COOKIES_PROVIDER_IMAGE_ADAPTER": "ark_image"}))
	if err == nil {
		t.Fatal("expected Ark configuration without credentials to be rejected")
	}
	config, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_ENV": "local", "COOKIES_PROVIDER_IMAGE_ADAPTER": "ark_image",
		"COOKIES_ARK_IMAGE_API_KEY": "test-key", "COOKIES_ARK_IMAGE_MODEL": "seedream-test",
	}))
	if err != nil || config.Provider.ImageAdapter != "ark_image" {
		t.Fatalf("valid local Ark configuration rejected: config=%#v err=%v", config.Provider, err)
	}
	_, err = FromLookup(mapLookup(map[string]string{
		"COOKIES_ENV": "staging", "COOKIES_BLOB_PROVIDER": "memory", "COOKIES_PROVIDER_IMAGE_ADAPTER": "ark_image",
		"COOKIES_ARK_IMAGE_API_KEY": "test-key", "COOKIES_ARK_IMAGE_MODEL": "seedream-test",
	}))
	if err == nil {
		t.Fatal("expected Ark adapter outside local to be rejected")
	}
}

func TestVideoGenerationRejectsDirectArkAdapter(t *testing.T) {
	t.Parallel()
	if _, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_ENV": "local", "COOKIES_PROVIDER_VIDEO_ADAPTER": "ark_video",
		"COOKIES_PROVIDER_MASTER_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32)),
	})); err == nil {
		t.Fatal("direct Ark video must be rejected even in local development")
	}
}

func TestVolcengineASRLegacyConfigurationIsExplicitAndReusable(t *testing.T) {
	t.Parallel()
	if _, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_PROVIDER_AUDIO_ADAPTER": "volcengine_asr",
	})); err == nil {
		t.Fatal("expected Volcengine ASR configuration without legacy credentials to be rejected")
	}
	config, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_ENV":                         "local",
		"COOKIES_PROVIDER_AUDIO_ADAPTER":      "volcengine_asr",
		"COOKIES_VOLCENGINE_ASR_AUTH_MODE":    "legacy",
		"COOKIES_VOLCENGINE_ASR_APP_ID":       "test-app",
		"COOKIES_VOLCENGINE_ASR_ACCESS_TOKEN": "test-token",
	}))
	if err != nil || config.Provider.AudioAdapter != "volcengine_asr" {
		t.Fatalf("valid local Volcengine ASR configuration rejected: config=%#v err=%v", config.Provider, err)
	}
	if config.Provider.VolcengineASR.ResourceID != "volc.bigasr.auc_turbo" ||
		config.Provider.VolcengineASR.Model != "bigmodel" {
		t.Fatalf("unexpected ASR defaults: %#v", config.Provider.VolcengineASR)
	}
	staging, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_ENV": "staging", "COOKIES_BLOB_PROVIDER": "memory",
		"COOKIES_PROVIDER_AUDIO_ADAPTER":          "volcengine_asr",
		"COOKIES_VOLCENGINE_ASR_APP_ID":           "test-app",
		"COOKIES_VOLCENGINE_ASR_ACCESS_TOKEN":     "test-token",
		"COOKIES_MEDIA_UNDERSTANDING_ASR_ENABLED": "true",
	}))
	if err != nil || !staging.MediaUnderstanding.ASREnabled {
		t.Fatalf("expected reusable Volcengine ASR configuration: %#v err=%v", staging.MediaUnderstanding, err)
	}
}

func TestMediaUnderstandingProviderRolloutIsIndependent(t *testing.T) {
	t.Parallel()
	if _, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_MEDIA_UNDERSTANDING_REAL_PROVIDER_ENABLED": "true",
	})); err == nil {
		t.Fatal("real media understanding must require the gateway adapter")
	}
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	value, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_PROVIDER_TEXT_ADAPTER":                     "adapter_gateway",
		"COOKIES_PROVIDER_MASTER_KEY":                       key,
		"COOKIES_MEDIA_UNDERSTANDING_REAL_PROVIDER_ENABLED": "true",
		"COOKIES_MEDIA_UNDERSTANDING_VISION_MODEL_ALIAS":    "cookies.vision.material.v1",
	}))
	if err != nil || !value.MediaUnderstanding.RealProviderEnabled || value.Strategy.RealProviderEnabled || value.MediaUnderstanding.VisionModelAlias != "cookies.vision.material.v1" {
		t.Fatalf("independent media understanding rollout=%#v strategy=%#v err=%v", value.MediaUnderstanding, value.Strategy, err)
	}
}

func TestVolcengineASRRejectsInsecureEndpointAndIncompleteAPIKeyAuth(t *testing.T) {
	t.Parallel()
	common := map[string]string{
		"COOKIES_PROVIDER_AUDIO_ADAPTER":      "volcengine_asr",
		"COOKIES_VOLCENGINE_ASR_APP_ID":       "test-app",
		"COOKIES_VOLCENGINE_ASR_ACCESS_TOKEN": "test-token",
	}
	common["COOKIES_VOLCENGINE_ASR_ENDPOINT"] = "http://openspeech.bytedance.com/recognize"
	if _, err := FromLookup(mapLookup(common)); err == nil {
		t.Fatal("expected an insecure ASR endpoint to be rejected")
	}
	delete(common, "COOKIES_VOLCENGINE_ASR_ENDPOINT")
	common["COOKIES_VOLCENGINE_ASR_AUTH_MODE"] = "api_key"
	if _, err := FromLookup(mapLookup(common)); err == nil {
		t.Fatal("expected api_key auth without COOKIES_VOLCENGINE_ASR_API_KEY to be rejected")
	}
}

func TestFromLookupUsesObjectStorageCompatibilityNamesForTOS(t *testing.T) {
	t.Parallel()
	config, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_BLOB_PROVIDER":            "tos",
		"OBJECT_STORAGE_ENDPOINT":          "tos.compat.example",
		"OBJECT_STORAGE_REGION":            "cn-test",
		"OBJECT_STORAGE_ACCESS_KEY":        "test-access-key",
		"OBJECT_STORAGE_SECRET_KEY":        "test-secret-key",
		"OBJECT_STORAGE_SECURITY_TOKEN":    "test-security-token",
		"OBJECT_STORAGE_QUARANTINE_BUCKET": "compat-quarantine",
		"OBJECT_STORAGE_ASSETS_BUCKET":     "compat-assets",
	}))
	if err != nil {
		t.Fatalf("FromLookup() error = %v", err)
	}
	if got, want := config.ObjectStorage.Endpoint, "tos.compat.example"; got != want {
		t.Fatalf("Endpoint = %q, want %q", got, want)
	}
	if got, want := config.ObjectStorage.Region, "cn-test"; got != want {
		t.Fatalf("Region = %q, want %q", got, want)
	}
	if got, want := config.ObjectStorage.AccessKey, "test-access-key"; got != want {
		t.Fatalf("AccessKey = %q, want %q", got, want)
	}
	if got, want := config.ObjectStorage.SecretKey, "test-secret-key"; got != want {
		t.Fatalf("SecretKey = %q, want %q", got, want)
	}
	if got, want := config.ObjectStorage.SecurityToken, "test-security-token"; got != want {
		t.Fatalf("SecurityToken = %q, want %q", got, want)
	}
	if got, want := config.ObjectStorage.QuarantineBucket, "compat-quarantine"; got != want {
		t.Fatalf("QuarantineBucket = %q, want %q", got, want)
	}
	if got, want := config.ObjectStorage.AssetsBucket, "compat-assets"; got != want {
		t.Fatalf("AssetsBucket = %q, want %q", got, want)
	}
}

func TestFromLookupUsesSingleTOSBucketForAllObjectClasses(t *testing.T) {
	t.Parallel()
	config, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_BLOB_PROVIDER":          "tos",
		"COOKIES_TOS_ENDPOINT":           "tos.example.com",
		"COOKIES_TOS_REGION":             "cn-test",
		"COOKIES_TOS_ACCESS_KEY":         "test-access-key",
		"COOKIES_TOS_SECRET_KEY":         "test-secret-key",
		"COOKIES_TOS_BUCKET":             "cookies-storage",
		"COOKIES_TOS_QUARANTINE_BUCKET":  "legacy-quarantine",
		"COOKIES_TOS_ASSETS_BUCKET":      "legacy-assets",
		"COOKIES_PROVIDER_OUTPUT_BUCKET": "legacy-provider-output",
	}))
	if err != nil {
		t.Fatalf("FromLookup() error = %v", err)
	}
	if got, want := config.ObjectStorage.QuarantineBucket, "cookies-storage"; got != want {
		t.Fatalf("QuarantineBucket = %q, want %q", got, want)
	}
	if got, want := config.ObjectStorage.AssetsBucket, "cookies-storage"; got != want {
		t.Fatalf("AssetsBucket = %q, want %q", got, want)
	}
	if got, want := config.Provider.OutputBucket, "cookies-storage"; got != want {
		t.Fatalf("OutputBucket = %q, want %q", got, want)
	}
}

func TestDocumentVisionRequiresEncryptedRouteAndOneSharedTOSBucket(t *testing.T) {
	t.Parallel()
	if _, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_DOCUMENT_VISION_ENABLED": "true",
	})); err == nil {
		t.Fatal("expected document vision with filesystem storage to be rejected")
	}
	base := map[string]string{
		"COOKIES_DOCUMENT_VISION_ENABLED": "true",
		"COOKIES_BLOB_PROVIDER":           "tos", "COOKIES_TOS_ENDPOINT": "tos.example.com",
		"COOKIES_TOS_REGION": "cn-test", "COOKIES_TOS_ACCESS_KEY": "key", "COOKIES_TOS_SECRET_KEY": "secret",
		"COOKIES_PROVIDER_MASTER_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32)),
	}
	base["COOKIES_TOS_ASSETS_BUCKET"] = "assets"
	base["COOKIES_TOS_QUARANTINE_BUCKET"] = "quarantine"
	base["COOKIES_PROVIDER_OUTPUT_BUCKET"] = "outputs"
	if _, err := FromLookup(mapLookup(base)); err == nil {
		t.Fatal("expected document vision with multiple buckets to be rejected")
	}
	base["COOKIES_TOS_BUCKET"] = "cookies-storage"
	config, err := FromLookup(mapLookup(base))
	if err != nil || !config.Research.DocumentVisionEnabled {
		t.Fatalf("valid document vision configuration rejected: config=%#v err=%v", config.Research, err)
	}
	base["COOKIES_DOCUMENT_CONVERTER_ENABLED"] = "true"
	base["COOKIES_DOCUMENT_CONVERTER_BASE_URL"] = "http://gotenberg:3000"
	if _, err := FromLookup(mapLookup(base)); err == nil {
		t.Fatal("expected insecure converter HTTP without explicit opt-in to be rejected")
	}
	base["COOKIES_DOCUMENT_CONVERTER_ALLOW_INSECURE_HTTP"] = "true"
	config, err = FromLookup(mapLookup(base))
	if err != nil || !config.Research.DocumentConverterEnabled || config.Research.DocumentConverterVersion == "" {
		t.Fatalf("valid document converter configuration rejected: config=%#v err=%v", config.Research, err)
	}
}

func TestDocumentConverterRequiresDocumentVision(t *testing.T) {
	t.Parallel()
	_, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_DOCUMENT_CONVERTER_ENABLED":             "true",
		"COOKIES_DOCUMENT_CONVERTER_ALLOW_INSECURE_HTTP": "true",
	}))
	if err == nil {
		t.Fatal("expected converter without document vision to be rejected")
	}
}

func secureProductionValues() map[string]string {
	return map[string]string{
		"COOKIES_ENV":            "production",
		"COOKIES_BLOB_PROVIDER":  "tos",
		"COOKIES_TOS_ENDPOINT":   "tos.example.com",
		"COOKIES_TOS_REGION":     "cn-test",
		"COOKIES_TOS_ACCESS_KEY": "key",
		"COOKIES_TOS_SECRET_KEY": "secret",
		"COOKIES_SCANNER_MODE":   "clamav",
		"COOKIES_CLAMAV_ADDRESS": "127.0.0.1:3310",
	}
}

func TestFromLookupUsesLegacyObjectStorageNamesForTOS(t *testing.T) {
	t.Parallel()
	config, err := FromLookup(mapLookup(map[string]string{
		"OBJECT_STORAGE_PROVIDER":          "tos",
		"OBJECT_STORAGE_ENDPOINT":          "tos.compat.example",
		"OBJECT_STORAGE_REGION":            "cn-test",
		"OBJECT_STORAGE_ACCESS_KEY_ID":     "test-access-key",
		"OBJECT_STORAGE_ACCESS_KEY_SECRET": "test-secret-key",
		"OBJECT_STORAGE_BUCKET_NAME":       "compat-assets",
	}))
	if err != nil {
		t.Fatalf("FromLookup() error = %v", err)
	}
	if got, want := config.ObjectStorage.Provider, "tos"; got != want {
		t.Fatalf("Provider = %q, want %q", got, want)
	}
	if got, want := config.ObjectStorage.AccessKey, "test-access-key"; got != want {
		t.Fatalf("AccessKey = %q, want %q", got, want)
	}
	if got, want := config.ObjectStorage.SecretKey, "test-secret-key"; got != want {
		t.Fatalf("SecretKey = %q, want %q", got, want)
	}
	if got, want := config.ObjectStorage.AssetsBucket, "compat-assets"; got != want {
		t.Fatalf("AssetsBucket = %q, want %q", got, want)
	}
}

func TestFromLookupPrefersCookiesTOSConfiguration(t *testing.T) {
	t.Parallel()
	config, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_BLOB_PROVIDER":            "tos",
		"COOKIES_TOS_ENDPOINT":             "tos.cookies.example",
		"COOKIES_TOS_REGION":               "cn-cookies",
		"COOKIES_TOS_ACCESS_KEY":           "cookies-access-key",
		"COOKIES_TOS_SECRET_KEY":           "cookies-secret-key",
		"COOKIES_TOS_QUARANTINE_BUCKET":    "cookies-quarantine",
		"COOKIES_TOS_ASSETS_BUCKET":        "cookies-assets",
		"OBJECT_STORAGE_ENDPOINT":          "tos.compat.example",
		"OBJECT_STORAGE_REGION":            "cn-compat",
		"OBJECT_STORAGE_ACCESS_KEY":        "compat-access-key",
		"OBJECT_STORAGE_SECRET_KEY":        "compat-secret-key",
		"OBJECT_STORAGE_QUARANTINE_BUCKET": "compat-quarantine",
		"OBJECT_STORAGE_ASSETS_BUCKET":     "compat-assets",
	}))
	if err != nil {
		t.Fatalf("FromLookup() error = %v", err)
	}
	if got, want := config.ObjectStorage.Endpoint, "tos.cookies.example"; got != want {
		t.Fatalf("Endpoint = %q, want %q", got, want)
	}
	if got, want := config.ObjectStorage.AccessKey, "cookies-access-key"; got != want {
		t.Fatalf("AccessKey = %q, want %q", got, want)
	}
	if got, want := config.ObjectStorage.AssetsBucket, "cookies-assets"; got != want {
		t.Fatalf("AssetsBucket = %q, want %q", got, want)
	}
}

func TestArkTextAdapterIsExplicitAndLocalOnly(t *testing.T) {
	t.Parallel()
	_, err := FromLookup(mapLookup(map[string]string{"COOKIES_PROVIDER_TEXT_ADAPTER": "ark_text"}))
	if err == nil {
		t.Fatal("expected Ark text configuration without credentials to be rejected")
	}
	config, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_ENV":                   "local",
		"COOKIES_PROVIDER_TEXT_ADAPTER": "ark_text",
		"COOKIES_ARK_TEXT_API_KEY":      "test-key",
		"COOKIES_ARK_TEXT_MODEL":        "doubao-test",
	}))
	if err != nil || config.Provider.TextAdapter != "ark_text" || config.Provider.ArkText.Model != "doubao-test" {
		t.Fatalf("valid local Ark text configuration rejected: config=%#v err=%v", config.Provider, err)
	}
	_, err = FromLookup(mapLookup(map[string]string{
		"COOKIES_ENV":                   "staging",
		"COOKIES_BLOB_PROVIDER":         "memory",
		"COOKIES_PROVIDER_TEXT_ADAPTER": "ark_text",
		"COOKIES_ARK_TEXT_API_KEY":      "test-key",
		"COOKIES_ARK_TEXT_MODEL":        "doubao-test",
	}))
	if err == nil {
		t.Fatal("expected Ark text adapter outside local to be rejected")
	}
}

func TestOpenAIImageAdapterRequiresCompleteLocalGatewayConfiguration(t *testing.T) {
	t.Parallel()
	_, err := FromLookup(mapLookup(map[string]string{"COOKIES_PROVIDER_IMAGE_ADAPTER": "openai_image"}))
	if err == nil {
		t.Fatal("expected incomplete OpenAI-compatible image configuration to be rejected")
	}
	config, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_PROVIDER_IMAGE_ADAPTER": "openai_image", "COOKIES_OPENAI_IMAGE_API_KEY": "test-key",
		"COOKIES_OPENAI_IMAGE_MODEL": "gpt-image-2", "COOKIES_OPENAI_IMAGE_BASE_URL": "http://gateway.example",
	}))
	if err != nil || config.Provider.ImageAdapter != "openai_image" {
		t.Fatalf("valid local OpenAI-compatible configuration rejected: config=%#v err=%v", config.Provider, err)
	}
}

func TestFromLookupParsesMCPStdioResearchConfiguration(t *testing.T) {
	t.Parallel()
	config, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_RESEARCH_MCP_STDIO_COMMAND":    "node",
		"COOKIES_RESEARCH_MCP_STDIO_ARGS_JSON":  `["server.js","--stdio"]`,
		"COOKIES_RESEARCH_MCP_TOOL_NAME":        "search_evidence",
		"COOKIES_RESEARCH_MCP_ENV_ALLOWLIST":    "PATH,SEARCH_API_KEY",
		"COOKIES_RESEARCH_TIMEOUT_SECONDS":      "90",
		"COOKIES_RESEARCH_MAX_OUTPUT_BYTES":     "2097152",
		"COOKIES_RESEARCH_MCP_PROTOCOL_VERSION": "2025-11-25",
	}))
	if err != nil {
		t.Fatalf("FromLookup() error = %v", err)
	}
	if config.Research.MCPStdioCommand != "node" ||
		len(config.Research.MCPStdioArgs) != 2 ||
		config.Research.MCPStdioArgs[1] != "--stdio" ||
		config.Research.MCPToolName != "search_evidence" ||
		config.Research.TimeoutSeconds != 90 {
		t.Fatalf("unexpected research config: %#v", config.Research)
	}
}

func TestFromLookupRejectsInvalidMCPArgumentsAndResearchBounds(t *testing.T) {
	t.Parallel()
	if _, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_RESEARCH_MCP_STDIO_ARGS_JSON": `["unterminated"`,
	})); err == nil {
		t.Fatal("expected invalid MCP args JSON to be rejected")
	}
	if _, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_RESEARCH_TIMEOUT_SECONDS": "0",
	})); err == nil {
		t.Fatal("expected invalid research timeout to be rejected")
	}
}

func TestFromLookupParsesAndValidatesTikaConfiguration(t *testing.T) {
	t.Parallel()
	config, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_RESEARCH_TIKA_ENABLED":          "true",
		"COOKIES_RESEARCH_TIKA_BASE_URL":         "http://127.0.0.1:9998",
		"COOKIES_RESEARCH_TIKA_VERSION":          "3.2.3.0",
		"COOKIES_RESEARCH_TIKA_TIMEOUT_SECONDS":  "90",
		"COOKIES_RESEARCH_TIKA_MAX_OUTPUT_BYTES": "1048576",
	}))
	if err != nil {
		t.Fatalf("FromLookup() error = %v", err)
	}
	if !config.Research.TikaEnabled || config.Research.TikaVersion != "3.2.3.0" ||
		config.Research.TikaTimeoutSeconds != 90 || config.Research.TikaMaxOutputBytes != 1048576 {
		t.Fatalf("unexpected Tika config: %#v", config.Research)
	}
	if _, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_RESEARCH_TIKA_ENABLED":  "true",
		"COOKIES_RESEARCH_TIKA_BASE_URL": "127.0.0.1:9998",
	})); err == nil {
		t.Fatal("expected relative Tika URL to be rejected")
	}
}

func TestSeedResearchRequiresCredentialEncryptionKey(t *testing.T) {
	t.Parallel()
	if _, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_RESEARCH_SEED_ENABLED": "true",
	})); err == nil {
		t.Fatal("expected Seed research without a credential encryption key to be rejected")
	}
	config, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_RESEARCH_SEED_ENABLED":       "true",
		"COOKIES_PROVIDER_MASTER_KEY":         base64.StdEncoding.EncodeToString(make([]byte, 32)),
		"COOKIES_PROVIDER_MASTER_KEY_VERSION": "v1",
	}))
	if err != nil || !config.Research.SeedEnabled {
		t.Fatalf("valid Seed research configuration rejected: config=%#v err=%v", config.Research, err)
	}
}

func TestMiyunConfigurationIsDisabledByDefaultAndStrictWhenEnabled(t *testing.T) {
	t.Parallel()
	defaults, err := FromLookup(mapLookup(nil))
	if err != nil {
		t.Fatal(err)
	}
	if defaults.Miyun.Enabled || defaults.Miyun.MaxConcurrent != 1 || defaults.Miyun.RequestsPerSecond != 5 || defaults.Miyun.CooldownSeconds != 300 {
		t.Fatalf("unsafe Miyun defaults: %#v", defaults.Miyun)
	}
	if _, err := FromLookup(mapLookup(map[string]string{"COOKIES_MIYUN_ENABLED": "true"})); err == nil {
		t.Fatal("enabled Miyun without key and host allowlist was accepted")
	}
	configured, err := FromLookup(mapLookup(map[string]string{
		"COOKIES_MIYUN_ENABLED":                "true",
		"COOKIES_MIYUN_MASTER_KEY":             base64.StdEncoding.EncodeToString(make([]byte, 32)),
		"COOKIES_MIYUN_MASTER_KEY_VERSION":     "key-v1",
		"COOKIES_MIYUN_DOWNLOAD_ALLOWED_HOSTS": "cdn.example.test,media.example.test",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !configured.Miyun.Enabled || len(configured.Miyun.DownloadAllowedHosts) != 2 {
		t.Fatalf("Miyun configuration = %#v", configured.Miyun)
	}
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
