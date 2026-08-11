package creativeprovider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/provider"
	"github.com/shikanon/cookies/internal/systems/creative"
)

const viralAnalysisSystemPrompt = `你是广告视频结构分析器。根据抽帧、音频转写和用户输入，只提炼可复用的抽象结构，不复制人物、商标、字幕、音乐、逐字台词或受保护画面。
只返回一个 JSON 对象，包含 dimensions（恰好五项）、preserve_rules、replace_rules、confidence。
dimensions 的 id 必须依次为 task_goal_type、quality_style_lighting、environment_atmosphere、camera_content、music_sound；每项包含 prompt、evidence_refs、confidence。`

type ViralAssetOpener interface {
	OpenPreview(context.Context, contract.ActorContext, contract.ProjectID, contract.AssetVersionRef) (io.ReadCloser, assets.ObjectInfo, error)
}

type ASRConfig struct {
	Endpoint    string
	AuthMode    string
	AppID       string
	AccessToken string
	APIKey      string
	ResourceID  string
	Model       string
}

type ViralAnalyzerConfig struct {
	Assets            ViralAssetOpener
	Routes            provider.TextRouteResolver
	Credentials       provider.GatewayCredentialResolver
	FFmpegPath        string
	WorkRoot          string
	ModelAlias        string
	PromptVersion     string
	AllowInsecureHTTP bool
	ASR               ASRConfig
	Client            *http.Client
}

type ViralAnalyzer struct{ config ViralAnalyzerConfig }

func NewViralAnalyzer(config ViralAnalyzerConfig) (*ViralAnalyzer, error) {
	if config.Assets == nil || config.Routes == nil || config.Credentials == nil ||
		strings.TrimSpace(config.FFmpegPath) == "" || strings.TrimSpace(config.ModelAlias) == "" ||
		strings.TrimSpace(config.PromptVersion) == "" || strings.TrimSpace(config.ASR.Endpoint) == "" {
		return nil, fmt.Errorf("viral analyzer dependencies are incomplete")
	}
	if config.Client == nil {
		config.Client = &http.Client{}
	}
	if strings.TrimSpace(config.WorkRoot) == "" {
		config.WorkRoot = ".data/video-work"
	}
	return &ViralAnalyzer{config: config}, nil
}

func (a *ViralAnalyzer) Analyze(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, request creative.ViralAnalysisRequest) (creative.ViralAnalysisResult, error) {
	if strings.TrimSpace(request.TaskID) == "" || request.InputSnapshot.ReferenceVideo.Validate() != nil {
		return creative.ViralAnalysisResult{}, viralAnalysisFailure(creative.ErrViralAnalysisSourceUnavailable, "reference video is invalid")
	}
	video, _, err := a.config.Assets.OpenPreview(ctx, actor, projectID, request.InputSnapshot.ReferenceVideo)
	if err != nil {
		return creative.ViralAnalysisResult{}, viralAnalysisFailure(creative.ErrViralAnalysisSourceUnavailable, "reference video cannot be opened")
	}
	defer video.Close()
	if err := os.MkdirAll(a.config.WorkRoot, 0o750); err != nil {
		return creative.ViralAnalysisResult{}, viralAnalysisFailure(creative.ErrViralAnalysisPreparationFailed, "temporary workspace cannot be prepared")
	}
	workDir, err := os.MkdirTemp(a.config.WorkRoot, "viral-analysis-*")
	if err != nil {
		return creative.ViralAnalysisResult{}, viralAnalysisFailure(creative.ErrViralAnalysisPreparationFailed, "temporary workspace cannot be created")
	}
	defer os.RemoveAll(workDir)
	videoPath := filepath.Join(workDir, "source.mp4")
	file, err := os.Create(videoPath)
	if err != nil {
		return creative.ViralAnalysisResult{}, viralAnalysisFailure(creative.ErrViralAnalysisPreparationFailed, "reference video cannot be staged")
	}
	if _, err = io.Copy(file, video); err != nil {
		file.Close()
		return creative.ViralAnalysisResult{}, viralAnalysisFailure(creative.ErrViralAnalysisPreparationFailed, "reference video cannot be staged")
	}
	if err := file.Close(); err != nil {
		return creative.ViralAnalysisResult{}, viralAnalysisFailure(creative.ErrViralAnalysisPreparationFailed, "reference video cannot be staged")
	}
	transcript, asrEvidence := a.extractTranscript(ctx, videoPath, workDir)
	frames, frameEvidence, err := a.extractFrames(ctx, videoPath, workDir)
	if err != nil {
		return creative.ViralAnalysisResult{}, viralAnalysisFailure(creative.ErrViralAnalysisPreparationFailed, "reference video frames cannot be extracted")
	}
	if len(frames) == 0 {
		return creative.ViralAnalysisResult{}, viralAnalysisFailure(creative.ErrViralAnalysisPreparationFailed, "reference video has no analyzable frames")
	}
	route, err := a.config.Routes.ResolveTextRoute(ctx, actor.OrganizationID, a.config.ModelAlias)
	if err != nil {
		return creative.ViralAnalysisResult{}, viralAnalysisFailure(creative.ErrViralAnalysisProviderUnavailable, "model route is unavailable")
	}
	if err := route.ValidateTextWithPolicy(a.config.AllowInsecureHTTP); err != nil {
		return creative.ViralAnalysisResult{}, viralAnalysisFailure(creative.ErrViralAnalysisProviderUnavailable, "model route is invalid")
	}
	token, err := a.config.Credentials.ResolveGatewayCredential(ctx, route.CredentialID, route.CredentialVersion)
	if err != nil {
		return creative.ViralAnalysisResult{}, viralAnalysisFailure(creative.ErrViralAnalysisProviderUnavailable, "model credential is unavailable")
	}
	result, err := a.callVisionModel(ctx, route, token, request, transcript, frames)
	if err != nil {
		return creative.ViralAnalysisResult{}, err
	}
	evidence := append([]string{}, frameEvidence...)
	evidence = append(evidence, asrEvidence...)
	result.Transcript = transcript
	result.EvidenceRefs = evidence
	result.RouteRevisionID = route.RouteRevisionID
	result.PromptVersion = a.config.PromptVersion
	for index := range result.Dimensions {
		result.Dimensions[index].Source = "ai_extracted"
		if len(result.Dimensions[index].EvidenceRefs) == 0 {
			result.Dimensions[index].EvidenceRefs = append([]string{}, evidence...)
		}
	}
	return result, nil
}

func (a *ViralAnalyzer) extractTranscript(ctx context.Context, videoPath, workDir string) (string, []string) {
	audioPath := filepath.Join(workDir, "audio.wav")
	command := exec.CommandContext(ctx, a.config.FFmpegPath, "-hide_banner", "-loglevel", "error", "-y",
		"-i", videoPath, "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", audioPath)
	if err := command.Run(); err != nil {
		return "", []string{"asr:no_audio_track"}
	}
	audio, err := os.ReadFile(audioPath)
	if err != nil || len(audio) == 0 {
		return "", []string{"asr:no_audio_track"}
	}
	transcript, err := a.transcribe(ctx, audio)
	if err != nil {
		return "", []string{"asr:unavailable"}
	}
	return transcript, []string{"asr:transcript"}
}

func (a *ViralAnalyzer) extractFrames(ctx context.Context, videoPath, workDir string) ([][]byte, []string, error) {
	pattern := filepath.Join(workDir, "frame-%02d.jpg")
	command := exec.CommandContext(ctx, a.config.FFmpegPath, "-hide_banner", "-loglevel", "error", "-y",
		"-i", videoPath, "-vf", "fps=1/3,scale=512:-2", "-q:v", "4", "-frames:v", "5", pattern)
	if output, err := command.CombinedOutput(); err != nil {
		return nil, nil, fmt.Errorf("extract viral reference frames: %w: %s", err, strings.TrimSpace(string(output)))
	}
	paths, err := filepath.Glob(filepath.Join(workDir, "frame-*.jpg"))
	if err != nil {
		return nil, nil, err
	}
	frames := make([][]byte, 0, len(paths))
	evidence := make([]string, 0, len(paths))
	for index, path := range paths {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, nil, readErr
		}
		frames = append(frames, content)
		evidence = append(evidence, fmt.Sprintf("frame:%d", index+1))
	}
	return frames, evidence, nil
}

func (a *ViralAnalyzer) transcribe(ctx context.Context, audio []byte) (string, error) {
	headers := map[string]string{
		"X-Api-Resource-Id": a.config.ASR.ResourceID,
		"X-Api-Request-Id":  fmt.Sprintf("cookies-%d", time.Now().UnixNano()),
		"X-Api-Sequence":    "-1",
	}
	uid := "cookies-viral-analysis"
	switch a.config.ASR.AuthMode {
	case "legacy":
		headers["X-Api-App-Key"] = a.config.ASR.AppID
		headers["X-Api-Access-Key"] = a.config.ASR.AccessToken
		uid = a.config.ASR.AppID
	case "api_key":
		headers["X-Api-Key"] = a.config.ASR.APIKey
	default:
		return "", fmt.Errorf("unsupported ASR auth mode")
	}
	body, err := json.Marshal(map[string]any{
		"user":    map[string]string{"uid": uid},
		"audio":   map[string]string{"data": base64.StdEncoding.EncodeToString(audio)},
		"request": map[string]string{"model_name": a.config.ASR.Model},
	})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.ASR.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := a.config.Client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	status := response.Header.Get("X-Api-Status-Code")
	if status == "20000003" {
		return "", nil
	}
	if status != "20000000" {
		return "", fmt.Errorf("ASR returned status %s", status)
	}
	var decoded struct {
		Result struct {
			Text string `json:"text"`
		} `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&decoded); err != nil {
		return "", err
	}
	return strings.TrimSpace(decoded.Result.Text), nil
}

func (a *ViralAnalyzer) callVisionModel(ctx context.Context, route provider.GatewayRouteSnapshot, token string, request creative.ViralAnalysisRequest, transcript string, frames [][]byte) (creative.ViralAnalysisResult, error) {
	content := []any{map[string]any{
		"type": "text",
		"text": fmt.Sprintf("任务输入：%s\n产品：%s\n卖点：%s\nCTA：%s\nASR 转写：%s",
			request.InputSnapshot.UserInstruction, request.InputSnapshot.ProductName,
			strings.Join(request.InputSnapshot.SellingPoints, "；"), request.InputSnapshot.CallToAction, transcript),
	}}
	for _, frame := range frames {
		content = append(content, map[string]any{
			"type":      "image_url",
			"image_url": map[string]string{"url": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(frame)},
		})
	}
	payload := map[string]any{
		"model": route.UpstreamModel,
		"messages": []any{
			map[string]any{"role": "system", "content": viralAnalysisSystemPrompt},
			map[string]any{"role": "user", "content": content},
		},
		"temperature": 0.2,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return creative.ViralAnalysisResult{}, err
	}
	endpoint := route.ChatCompletionsEndpoint()
	timeout, cancel := context.WithTimeout(ctx, time.Duration(route.TimeoutSeconds)*time.Second)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(timeout, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return creative.ViralAnalysisResult{}, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+token)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := a.config.Client.Do(httpRequest)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return creative.ViralAnalysisResult{}, viralAnalysisFailure(creative.ErrViralAnalysisProviderUnavailable, "model request timed out")
		}
		return creative.ViralAnalysisResult{}, viralAnalysisFailure(creative.ErrViralAnalysisProviderUnavailable, "model gateway request failed")
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, route.MaxResponseBytes+1))
	if err != nil || int64(len(responseBody)) > route.MaxResponseBytes {
		return creative.ViralAnalysisResult{}, viralAnalysisFailure(creative.ErrViralAnalysisResponseInvalid, "model response exceeded the safety limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return creative.ViralAnalysisResult{}, viralAnalysisHTTPFailure(response.StatusCode)
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil || len(envelope.Choices) == 0 {
		return creative.ViralAnalysisResult{}, viralAnalysisFailure(creative.ErrViralAnalysisResponseInvalid, "model response envelope is invalid")
	}
	result, err := decodeViralAnalysis(envelope.Choices[0].Message.Content)
	if err != nil {
		return creative.ViralAnalysisResult{}, viralAnalysisFailure(creative.ErrViralAnalysisResponseInvalid, "model response does not contain five-dimensional JSON")
	}
	return result, nil
}

func viralAnalysisHTTPFailure(status int) error {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return viralAnalysisFailure(creative.ErrViralAnalysisProviderRejected, "model gateway rejected the credential")
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		return viralAnalysisFailure(creative.ErrViralAnalysisProviderRejected, "model gateway rejected the visual analysis request")
	case status == http.StatusTooManyRequests || status >= http.StatusInternalServerError:
		return viralAnalysisFailure(creative.ErrViralAnalysisProviderUnavailable, "model gateway is temporarily unavailable")
	default:
		return viralAnalysisFailure(creative.ErrViralAnalysisProviderRejected, "model gateway rejected the request")
	}
}

func viralAnalysisFailure(category error, detail string) error {
	return fmt.Errorf("%w: %s", category, detail)
}

func decodeViralAnalysis(content string) (creative.ViralAnalysisResult, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	var decoded struct {
		Dimensions []struct {
			ID           creative.ViralPromptDimensionID `json:"id"`
			Prompt       string                          `json:"prompt"`
			EvidenceRefs []string                        `json:"evidence_refs"`
			Confidence   float64                         `json:"confidence"`
		} `json:"dimensions"`
		PreserveRules []string `json:"preserve_rules"`
		ReplaceRules  []string `json:"replace_rules"`
		Confidence    float64  `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &decoded); err != nil {
		return creative.ViralAnalysisResult{}, fmt.Errorf("Seed-2-pro did not return the required JSON object")
	}
	result := creative.ViralAnalysisResult{
		PreserveRules: decoded.PreserveRules, ReplaceRules: decoded.ReplaceRules, Confidence: decoded.Confidence,
	}
	for _, dimension := range decoded.Dimensions {
		result.Dimensions = append(result.Dimensions, creative.ViralAnalysisDimension{
			ID: dimension.ID, Prompt: strings.TrimSpace(dimension.Prompt),
			EvidenceRefs: dimension.EvidenceRefs, Confidence: dimension.Confidence, Source: "ai_extracted",
		})
	}
	return result, nil
}
