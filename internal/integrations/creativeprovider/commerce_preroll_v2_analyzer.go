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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/systems/creative"
)

const commercePrerollV2AnalysisPrompt = `你是电商广告原视频理解导演。输入是已经完成的电商正片的抽帧和 ASR。你的任务不是创作新卖点，而是提取可用于生成 6-10 秒独立前贴的事实与衔接约束。
只返回一个 JSON 对象，字段必须完整：
{
  "product": {
    "name": "画面、字幕或口播明确支持的商品名称；不确定则写通用品类名",
    "category": "商品品类",
    "description": "客观商品描述",
    "selling_points": ["原视频明确表达的卖点，不得扩写功效"],
    "appearance_guardrails": ["瓶型、材质、主色、标签布局等可观察外观约束"],
    "logo_guardrails": ["品牌字样或 Logo 的保真和禁止项"]
  },
  "visual_style": "原片色调、光线、构图、节奏和质感",
  "subtitle_summary": "字幕位置、字号、颜色和节奏；没有则写无字幕",
  "voice_summary": "口播人物、语气和关键信息；没有则写无口播",
  "audio_mood": "音乐、环境声和节奏；无法确认则如实说明",
  "opening_shot": "原视频开场镜头中可用于自然衔接的画面描述",
  "product_frame_ms": 0,
  "product_frame_candidates": [{"id":"product_candidate_1","timestamp_ms":0,"frontality":0.0,"sharpness":0.0,"completeness":0.0,"logo_readability":0.0,"occlusion":0.0,"overall":0.0}],
  "opening_anchor_frame_ms": 0,
  "evidence": [{"id":"frame_1","timestamp_ms":0,"source":"frame|subtitle|voice","excerpt":"证据摘要","confidence":0.0}],
  "risks": ["需要用户确认的功效、数字、权利或识别不确定项"]
}
product_frame_candidates 返回 3-8 个来自整条输入范围的真实候选并按 overall 降序，必须兼顾视频中部和尾部；product_frame_ms 等于第一候选时间。opening_anchor_frame_ms 选择原片开头 0-3 秒内稳定且适合提取连续性特征的一帧。所有时间必须来自输入帧时间。至少返回一条证据。不得输出 Markdown。`

type CommercePrerollV2Analyzer struct {
	helper *ShortDramaV2Analyzer
}

type commerceSampledFrame struct {
	TimestampMS int64
	Content     []byte
}

func NewCommercePrerollV2Analyzer(config ViralAnalyzerConfig) (*CommercePrerollV2Analyzer, error) {
	helper, err := NewShortDramaV2Analyzer(config)
	if err != nil {
		return nil, err
	}
	return &CommercePrerollV2Analyzer{helper: helper}, nil
}

func (a *CommercePrerollV2Analyzer) Analyze(ctx context.Context, actor contract.ActorContext, project contract.ProjectContext, source contract.ProjectAssetRef) (creative.CommercePrerollV2AnalysisResult, error) {
	if source.ProjectID != project.ProjectID || source.Validate() != nil {
		return creative.CommercePrerollV2AnalysisResult{}, fmt.Errorf("commerce source video reference is invalid")
	}
	video, _, err := a.helper.config.Assets.OpenPreview(ctx, actor, project.ProjectID, source.AssetVersion)
	if err != nil {
		return creative.CommercePrerollV2AnalysisResult{}, fmt.Errorf("commerce source video cannot be opened: %w", err)
	}
	defer video.Close()
	if err := os.MkdirAll(a.helper.config.WorkRoot, 0o750); err != nil {
		return creative.CommercePrerollV2AnalysisResult{}, fmt.Errorf("prepare commerce analysis workspace: %w", err)
	}
	workDir, err := os.MkdirTemp(a.helper.config.WorkRoot, "commerce-preroll-v2-*")
	if err != nil {
		return creative.CommercePrerollV2AnalysisResult{}, err
	}
	defer os.RemoveAll(workDir)
	videoPath := filepath.Join(workDir, "source.mp4")
	file, err := os.Create(videoPath)
	if err != nil {
		return creative.CommercePrerollV2AnalysisResult{}, err
	}
	if _, err = io.Copy(file, video); err != nil {
		_ = file.Close()
		return creative.CommercePrerollV2AnalysisResult{}, err
	}
	if err := file.Close(); err != nil {
		return creative.CommercePrerollV2AnalysisResult{}, err
	}
	transcript, _ := a.helper.helper.extractTranscript(ctx, videoPath, workDir)
	frames, err := a.extractFrames(ctx, videoPath, workDir)
	if err != nil || len(frames) == 0 {
		return creative.CommercePrerollV2AnalysisResult{}, fmt.Errorf("commerce source frames cannot be extracted: %w", err)
	}
	result, err := a.callModel(ctx, actor, source, transcript, frames)
	if err != nil {
		return creative.CommercePrerollV2AnalysisResult{}, err
	}
	hash, err := contract.CanonicalJSONHash(struct {
		Source contract.ProjectAssetRef `json:"source"`
		Prompt string                   `json:"prompt"`
	}{Source: source, Prompt: "commerce-preroll-analysis/v1"})
	if err != nil {
		return creative.CommercePrerollV2AnalysisResult{}, err
	}
	result.InputHash = "sha256:" + hash
	result.PromptVersion = "commerce-preroll-analysis/v1"
	return result, nil
}

func (a *CommercePrerollV2Analyzer) extractFrames(ctx context.Context, videoPath, workDir string) ([]commerceSampledFrame, error) {
	durationMS, err := probeCommerceDurationMS(ctx, a.helper.config.FFmpegPath, videoPath)
	if err != nil {
		return nil, err
	}
	sceneTimestamps := detectCommerceSceneTimestamps(ctx, a.helper.config.FFmpegPath, videoPath)
	timestamps := commerceFrameSamplePlan(durationMS, sceneTimestamps)
	frames := make([]commerceSampledFrame, 0, len(timestamps))
	var lastFailure error
	for index, timestampMS := range timestamps {
		path := filepath.Join(workDir, fmt.Sprintf("commerce-frame-%02d.jpg", index+1))
		command := exec.CommandContext(ctx, a.helper.config.FFmpegPath, "-hide_banner", "-loglevel", "error", "-y", "-ss", fmt.Sprintf("%.3f", float64(timestampMS)/1000), "-i", videoPath,
			"-frames:v", "1", "-vf", "scale=640:-2", "-q:v", "4", path)
		if output, commandErr := command.CombinedOutput(); commandErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastFailure = fmt.Errorf("ffmpeg frame at %dms: %w: %s", timestampMS, commandErr, strings.TrimSpace(string(output)))
			continue
		}
		frame, ok, readErr := readCommerceSampledFrame(path, timestampMS)
		if readErr != nil {
			lastFailure = readErr
			continue
		}
		if ok {
			frames = append(frames, frame)
		}
	}
	if len(frames) < 3 {
		if lastFailure != nil {
			return nil, fmt.Errorf("only %d commerce source frames were extracted: %w", len(frames), lastFailure)
		}
		return nil, fmt.Errorf("only %d commerce source frames were extracted", len(frames))
	}
	return frames, nil
}

func readCommerceSampledFrame(path string, timestampMS int64) (commerceSampledFrame, bool, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return commerceSampledFrame{}, false, nil
	}
	if err != nil {
		return commerceSampledFrame{}, false, err
	}
	if len(content) == 0 {
		return commerceSampledFrame{}, false, nil
	}
	return commerceSampledFrame{TimestampMS: timestampMS, Content: content}, true, nil
}

var commerceSceneTimestampPattern = regexp.MustCompile(`pts_time:([0-9]+(?:\.[0-9]+)?)`)

func detectCommerceSceneTimestamps(ctx context.Context, ffmpegPath, videoPath string) []int64 {
	command := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-loglevel", "info", "-i", videoPath, "-vf", "select=gt(scene\\,0.35),showinfo", "-an", "-f", "null", "-")
	output, err := command.CombinedOutput()
	if err != nil {
		return nil
	}
	matches := commerceSceneTimestampPattern.FindAllStringSubmatch(string(output), 24)
	result := make([]int64, 0, len(matches))
	for _, match := range matches {
		seconds, parseErr := strconv.ParseFloat(match[1], 64)
		if parseErr == nil {
			result = append(result, int64(seconds*1000))
		}
	}
	return result
}

func probeCommerceDurationMS(ctx context.Context, ffmpegPath, videoPath string) (int64, error) {
	extension := filepath.Ext(ffmpegPath)
	ffprobePath := filepath.Join(filepath.Dir(ffmpegPath), "ffprobe"+extension)
	command := exec.CommandContext(ctx, ffprobePath, "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", videoPath)
	output, err := command.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe duration: %w", err)
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("ffprobe returned an invalid commerce video duration")
	}
	return int64(seconds * 1000), nil
}

func commerceFrameSamplePlan(durationMS int64, sceneTimestamps []int64) []int64 {
	if durationMS <= 0 {
		return nil
	}
	last := durationMS - 250
	if last < 0 {
		last = 0
	}
	values := map[int64]struct{}{}
	add := func(value int64) {
		if value < 0 {
			value = 0
		}
		if value > last {
			value = last
		}
		values[value] = struct{}{}
	}
	for value := int64(0); value <= 3000; value += 500 {
		add(value)
	}
	const uniformCount = 16
	for index := 0; index < uniformCount; index++ {
		add(int64(float64(last) * float64(index) / float64(uniformCount-1)))
	}
	tailStart := durationMS - 5000
	for index := 0; index < 5; index++ {
		add(tailStart + int64(index)*1250)
	}
	for _, value := range sceneTimestamps {
		add(value)
	}
	result := make([]int64, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	if len(result) <= 32 {
		return result
	}
	trimmed := make([]int64, 0, 32)
	for index := 0; index < 32; index++ {
		trimmed = append(trimmed, result[index*(len(result)-1)/31])
	}
	return trimmed
}

func (a *CommercePrerollV2Analyzer) callModel(ctx context.Context, actor contract.ActorContext, source contract.ProjectAssetRef, transcript string, frames []commerceSampledFrame) (creative.CommercePrerollV2AnalysisResult, error) {
	route, err := a.helper.config.Routes.ResolveTextRoute(ctx, actor.OrganizationID, a.helper.config.ModelAlias)
	if err != nil {
		return creative.CommercePrerollV2AnalysisResult{}, fmt.Errorf("commerce analysis model route is unavailable: %w", err)
	}
	if err := route.ValidateTextWithPolicy(a.helper.config.AllowInsecureHTTP); err != nil {
		return creative.CommercePrerollV2AnalysisResult{}, err
	}
	token, err := a.helper.config.Credentials.ResolveGatewayCredential(ctx, route.CredentialID, route.CredentialVersion)
	if err != nil {
		return creative.CommercePrerollV2AnalysisResult{}, err
	}
	content := []any{map[string]any{"type": "text", "text": "evidence id=transcript_1，ASR 转写：" + transcript}}
	for index, frame := range frames {
		content = append(content,
			map[string]any{"type": "text", "text": fmt.Sprintf("下一张图 evidence id=frame_%d，timestamp_ms=%d", index+1, frame.TimestampMS)},
			map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(frame.Content)}},
		)
	}
	payload := map[string]any{"model": route.UpstreamModel, "messages": []any{
		map[string]any{"role": "system", "content": commercePrerollV2AnalysisPrompt},
		map[string]any{"role": "user", "content": content},
	}}
	if err := applyShortDramaTextRouteConstraints(payload, route); err != nil {
		return creative.CommercePrerollV2AnalysisResult{}, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return creative.CommercePrerollV2AnalysisResult{}, err
	}
	timeout, cancel := context.WithTimeout(ctx, time.Duration(route.TimeoutSeconds)*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(timeout, http.MethodPost, route.ChatCompletionsEndpoint(), bytes.NewReader(body))
	if err != nil {
		return creative.CommercePrerollV2AnalysisResult{}, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := a.helper.doModelRequestWithRetry(timeout, request, body)
	if err != nil {
		return creative.CommercePrerollV2AnalysisResult{}, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, route.MaxResponseBytes+1))
	if err != nil || int64(len(responseBody)) > route.MaxResponseBytes || response.StatusCode < 200 || response.StatusCode >= 300 {
		return creative.CommercePrerollV2AnalysisResult{}, fmt.Errorf("commerce analysis model response is unavailable")
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil || len(envelope.Choices) == 0 {
		return creative.CommercePrerollV2AnalysisResult{}, fmt.Errorf("commerce analysis model response envelope is invalid")
	}
	text := strings.TrimSpace(envelope.Choices[0].Message.Content)
	text = strings.TrimPrefix(strings.TrimPrefix(text, "```json"), "```")
	text = strings.TrimSuffix(text, "```")
	var result creative.CommercePrerollV2AnalysisResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &result.Content); err != nil {
		return creative.CommercePrerollV2AnalysisResult{}, fmt.Errorf("commerce analysis model response is invalid: %w", err)
	}
	return result, nil
}

var _ creative.CommercePrerollV2Analyzer = (*CommercePrerollV2Analyzer)(nil)
