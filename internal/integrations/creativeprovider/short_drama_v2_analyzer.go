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

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/provider"
	"github.com/shikanon/cookies/internal/systems/creative"
)

const shortDramaV2AnalysisPrompt = `你是短剧视频理解编导。根据全片间隔抽帧和 ASR 转写，提炼能够支撑前贴创作的真实剧情事实。不得猜测未出现的人物、关系、事件或结局。

只返回一个 JSON 对象，并严格包含以下字段：
{
  "title": "根据画面和转写识别的短剧标题；若原片未明确展示标题，则用真实剧情事实概括命名",
  "episode": "集数，无法确认时返回空字符串",
  "synopsis": "不少于 40 个汉字的全片剧情梗概",
  "opening_beat": "开场已经发生的动作或信息",
  "core_conflict": "全片真实呈现的核心冲突",
  "unresolved_hook": "适合引流且未剧透结局的信息缺口",
  "tone": "主要情绪与类型",
  "characters": [{"name":"人物称谓或姓名","description":"只写可观察事实","relationship":"可确认关系，无法确认时留空"}],
  "visual_keywords": ["可观察的场景、服饰、时代或动作关键词"],
  "evidence": [{"id":"frame_1","timestamp_ms":0,"transcript":"该证据实际支持的简短事实"}]
}

title、synopsis、opening_beat、core_conflict、unresolved_hook 和 evidence 不得为空。evidence.id 只能使用输入提供的 transcript_1、frame_1 至 frame_8；timestamp_ms 必须与输入给出的秒数对应。不要输出 Markdown。`

type ShortDramaV2Analyzer struct {
	config ViralAnalyzerConfig
	helper ViralAnalyzer
}

func NewShortDramaV2Analyzer(config ViralAnalyzerConfig) (*ShortDramaV2Analyzer, error) {
	helper, err := NewViralAnalyzer(config)
	if err != nil {
		return nil, err
	}
	return &ShortDramaV2Analyzer{config: helper.config, helper: *helper}, nil
}

func (a *ShortDramaV2Analyzer) Analyze(ctx context.Context, actor contract.ActorContext, project contract.ProjectContext, source contract.ProjectAssetRef) (creative.ShortDramaV2AnalysisResult, error) {
	if source.ProjectID != project.ProjectID || source.Validate() != nil {
		return creative.ShortDramaV2AnalysisResult{}, shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisSourceUnavailable, "source video reference is invalid")
	}
	video, _, err := a.config.Assets.OpenPreview(ctx, actor, project.ProjectID, source.AssetVersion)
	if err != nil {
		return creative.ShortDramaV2AnalysisResult{}, fmt.Errorf("%w: source video cannot be opened: %v", creative.ErrShortDramaAnalysisSourceUnavailable, err)
	}
	defer video.Close()
	if err := os.MkdirAll(a.config.WorkRoot, 0o750); err != nil {
		return creative.ShortDramaV2AnalysisResult{}, shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisPreparationFailed, "temporary workspace cannot be prepared")
	}
	workDir, err := os.MkdirTemp(a.config.WorkRoot, "short-drama-v2-*")
	if err != nil {
		return creative.ShortDramaV2AnalysisResult{}, shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisPreparationFailed, "temporary workspace cannot be created")
	}
	defer os.RemoveAll(workDir)
	videoPath := filepath.Join(workDir, "source.mp4")
	file, err := os.Create(videoPath)
	if err != nil {
		return creative.ShortDramaV2AnalysisResult{}, shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisPreparationFailed, "source video cannot be staged")
	}
	if _, err = io.Copy(file, video); err != nil {
		_ = file.Close()
		return creative.ShortDramaV2AnalysisResult{}, shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisPreparationFailed, "source video cannot be staged")
	}
	if err := file.Close(); err != nil {
		return creative.ShortDramaV2AnalysisResult{}, shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisPreparationFailed, "source video cannot be staged")
	}
	transcript, _ := a.helper.extractTranscript(ctx, videoPath, workDir)
	frames, err := a.extractWholeVideoFrames(ctx, videoPath, workDir)
	if err != nil || len(frames) == 0 {
		return creative.ShortDramaV2AnalysisResult{}, shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisPreparationFailed, "source video frames cannot be extracted")
	}
	result, err := a.callModel(ctx, actor, source, transcript, frames)
	if err != nil {
		return creative.ShortDramaV2AnalysisResult{}, err
	}
	hash, err := contract.CanonicalJSONHash(struct {
		Source        contract.ProjectAssetRef `json:"source"`
		PromptVersion string                   `json:"prompt_version"`
	}{source, "short-drama-analysis/v1"})
	if err != nil {
		return creative.ShortDramaV2AnalysisResult{}, err
	}
	result.InputHash = "sha256:" + hash
	result.PromptVersion = "short-drama-analysis/v1"
	return result, nil
}

func (a *ShortDramaV2Analyzer) extractWholeVideoFrames(ctx context.Context, videoPath, workDir string) ([][]byte, error) {
	pattern := filepath.Join(workDir, "story-frame-%02d.jpg")
	// One frame every 24 seconds covers the provided 182-second short drama with
	// eight representative observations instead of only sampling its opening.
	command := exec.CommandContext(ctx, a.config.FFmpegPath, "-hide_banner", "-loglevel", "error", "-y", "-i", videoPath,
		"-vf", "fps=1/24,scale=640:-2", "-q:v", "4", "-frames:v", "8", pattern)
	if output, err := command.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(string(output)))
	}
	paths, err := filepath.Glob(filepath.Join(workDir, "story-frame-*.jpg"))
	if err != nil {
		return nil, err
	}
	frames := make([][]byte, 0, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		frames = append(frames, content)
	}
	return frames, nil
}

func (a *ShortDramaV2Analyzer) callModel(ctx context.Context, actor contract.ActorContext, source contract.ProjectAssetRef, transcript string, frames [][]byte) (creative.ShortDramaV2AnalysisResult, error) {
	route, err := a.config.Routes.ResolveTextRoute(ctx, actor.OrganizationID, a.config.ModelAlias)
	if err != nil {
		return creative.ShortDramaV2AnalysisResult{}, shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisProviderUnavailable, "model route is unavailable")
	}
	if err := route.ValidateTextWithPolicy(a.config.AllowInsecureHTTP); err != nil {
		return creative.ShortDramaV2AnalysisResult{}, shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisProviderUnavailable, "model route is invalid")
	}
	token, err := a.config.Credentials.ResolveGatewayCredential(ctx, route.CredentialID, route.CredentialVersion)
	if err != nil {
		return creative.ShortDramaV2AnalysisResult{}, shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisProviderUnavailable, "model credential is unavailable")
	}
	content := []any{map[string]any{"type": "text", "text": "evidence id=transcript_1，ASR转写：" + transcript}}
	for index, frame := range frames {
		content = append(content,
			map[string]any{"type": "text", "text": fmt.Sprintf("下一张图的 evidence id=frame_%d，约位于全片第%d秒", index+1, index*24)},
			map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(frame)}},
		)
	}
	payload := map[string]any{"model": route.UpstreamModel, "messages": []any{
		map[string]any{"role": "system", "content": shortDramaV2AnalysisPrompt},
		map[string]any{"role": "user", "content": content},
	}}
	if err := applyShortDramaTextRouteConstraints(payload, route); err != nil {
		return creative.ShortDramaV2AnalysisResult{}, shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisProviderUnavailable, "model route constraints are invalid")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return creative.ShortDramaV2AnalysisResult{}, err
	}
	endpoint := route.ChatCompletionsEndpoint()
	timeout, cancel := context.WithTimeout(ctx, time.Duration(route.TimeoutSeconds)*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(timeout, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return creative.ShortDramaV2AnalysisResult{}, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := a.doModelRequestWithRetry(timeout, request, body)
	if err != nil {
		return creative.ShortDramaV2AnalysisResult{}, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, route.MaxResponseBytes+1))
	if err != nil || int64(len(responseBody)) > route.MaxResponseBytes {
		return creative.ShortDramaV2AnalysisResult{}, shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisResponseInvalid, "model response exceeded the safety limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return creative.ShortDramaV2AnalysisResult{}, shortDramaAnalysisHTTPFailure(response.StatusCode)
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil || len(envelope.Choices) == 0 {
		return creative.ShortDramaV2AnalysisResult{}, shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisResponseInvalid, "model response envelope is invalid")
	}
	text := strings.TrimSpace(envelope.Choices[0].Message.Content)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	var result creative.ShortDramaV2AnalysisResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &result.Content); err != nil {
		return creative.ShortDramaV2AnalysisResult{}, shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisResponseInvalid, "model response is not the required JSON object")
	}
	return result, nil
}

func applyShortDramaTextRouteConstraints(payload map[string]any, route provider.GatewayRouteSnapshot) error {
	switch route.TextResponseMode {
	case provider.TextResponseJSONSchema:
		// The analyzer validates the complete domain contract after decoding. A
		// JSON-object response keeps this multimodal request compatible with
		// gateways that do not accept an inline strict schema for image input.
		payload["response_format"] = map[string]any{"type": "json_object"}
	case provider.TextResponseJSONObject:
		payload["response_format"] = map[string]any{"type": "json_object"}
	case provider.TextResponsePromptJSON:
		// The system prompt already contains the exact JSON contract. Do not add
		// response_format: several OpenAI-compatible multimodal gateways reject it.
	case "":
		return fmt.Errorf("text response mode is required")
	default:
		return fmt.Errorf("unsupported text response mode %q", route.TextResponseMode)
	}
	if route.MaxOutputTokens > 0 {
		switch route.OutputTokenParameter {
		case "", provider.TextOutputTokenParameterMaxTokens:
			payload["max_tokens"] = route.MaxOutputTokens
		case provider.TextOutputTokenParameterMaxCompletionTokens:
			payload["max_completion_tokens"] = route.MaxOutputTokens
		default:
			return fmt.Errorf("unsupported output token parameter %q", route.OutputTokenParameter)
		}
	}
	if route.TemperatureSet {
		payload["temperature"] = route.Temperature
	}
	return nil
}

func (a *ShortDramaV2Analyzer) doModelRequestWithRetry(ctx context.Context, template *http.Request, body []byte) (*http.Response, error) {
	const maxAttempts = 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		request := template.Clone(ctx)
		request.Body = io.NopCloser(bytes.NewReader(body))
		response, err := a.config.Client.Do(request)
		if err == nil && response.StatusCode != http.StatusTooManyRequests && response.StatusCode < http.StatusInternalServerError {
			return response, nil
		}
		if err == nil && attempt == maxAttempts {
			return response, nil
		}
		if response != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
			_ = response.Body.Close()
		}
		if err != nil && attempt == maxAttempts {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisProviderUnavailable, "model request timed out")
			}
			return nil, shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisProviderUnavailable, "model gateway request failed")
		}
		delay := time.Duration(attempt*250) * time.Millisecond
		select {
		case <-ctx.Done():
			return nil, shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisProviderUnavailable, "model request timed out")
		case <-time.After(delay):
		}
	}
	return nil, shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisProviderUnavailable, "model gateway request failed")
}

func shortDramaAnalysisHTTPFailure(status int) error {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisProviderRejected, "model gateway rejected the credential")
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		return shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisProviderRejected, "model gateway rejected the multimodal request")
	case status == http.StatusTooManyRequests || status >= http.StatusInternalServerError:
		return shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisProviderUnavailable, "model gateway is temporarily unavailable")
	default:
		return shortDramaAnalysisFailure(creative.ErrShortDramaAnalysisProviderRejected, "model gateway rejected the request")
	}
}

func shortDramaAnalysisFailure(category error, detail string) error {
	return fmt.Errorf("%w: %s", category, detail)
}

var _ creative.ShortDramaV2Analyzer = (*ShortDramaV2Analyzer)(nil)
