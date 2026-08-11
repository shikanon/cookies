package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/contract"
)

type goldenTimelineSource struct {
	files     map[contract.AssetID]string
	withAudio map[contract.AssetID]bool
}

type c0FixtureManifest struct {
	SchemaVersion string `json:"schema_version"`
	Assets        []struct {
		ID         string `json:"id"`
		File       string `json:"file"`
		SHA256     string `json:"sha256"`
		DurationMS int64  `json:"duration_ms"`
	} `json:"assets"`
	Golden struct {
		TimelineDurationMS     int    `json:"timeline_duration_ms"`
		Caption                string `json:"caption"`
		VideoFrameMD5SHA256    string `json:"video_framemd5_sha256"`
		AudioFrameMD5SHA256    string `json:"audio_framemd5_sha256"`
		C3VisualFrameMD5SHA256 string `json:"c3_visual_framemd5_sha256"`
		C7VideoFrameMD5SHA256  string `json:"c7_video_framemd5_sha256"`
		C7AudioFrameMD5SHA256  string `json:"c7_audio_framemd5_sha256"`
	} `json:"golden"`
}

func (s goldenTimelineSource) open(ref contract.AssetVersionRef) (assets.AssetVersion, io.ReadCloser, error) {
	contents, err := os.ReadFile(s.files[ref.AssetID])
	if err != nil {
		return assets.AssetVersion{}, nil, err
	}
	version := assets.AssetVersion{AssetID: ref.AssetID, Version: ref.Version, SizeBytes: int64(len(contents))}
	if s.withAudio[ref.AssetID] {
		version.AudioCodec = "aac"
	}
	return version, io.NopCloser(bytes.NewReader(contents)), nil
}
func (s goldenTimelineSource) OpenVideo(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, ref contract.AssetVersionRef) (assets.AssetVersion, io.ReadCloser, error) {
	return s.open(ref)
}
func (s goldenTimelineSource) OpenVisual(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, ref contract.AssetVersionRef) (assets.AssetVersion, io.ReadCloser, error) {
	return s.open(ref)
}
func (s goldenTimelineSource) OpenAudio(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, ref contract.AssetVersionRef) (assets.AssetVersion, io.ReadCloser, error) {
	return s.open(ref)
}

func TestFFmpegC3VisualLayersMatchFixedGolden(t *testing.T) {
	ffmpegPath, ffprobePath := os.Getenv("COOKIES_TEST_FFMPEG_PATH"), os.Getenv("COOKIES_TEST_FFPROBE_PATH")
	fixtureRoot := os.Getenv("COOKIES_C0_FIXTURE_ROOT")
	if ffmpegPath == "" || ffprobePath == "" || fixtureRoot == "" {
		if os.Getenv("COOKIES_REQUIRE_TIMELINE_GOLDEN") == "1" {
			t.Fatal("C3 visual golden is required but its tool or fixture paths are missing")
		}
		t.Skip("set C3 visual golden fixture environment")
	}
	manifest := loadC0FixtureManifest(t, fixtureRoot)
	source := goldenTimelineSource{files: map[contract.AssetID]string{
		"video-landscape": filepath.Join(fixtureRoot, "video-landscape.mp4"),
		"overlay-opaque":  filepath.Join(fixtureRoot, "overlay-opaque.png"),
		"overlay-alpha":   filepath.Join(fixtureRoot, "overlay-alpha.png"),
	}}
	probe := assets.FFprobeVideoProbe{Path: ffprobePath, WorkRoot: t.TempDir()}
	renderer := FFmpegTimelineRenderer{FFmpegPath: ffmpegPath, WorkRoot: t.TempDir(), Visuals: source, Audio: source, Probe: probe, Deterministic: true}
	request := TimelineRenderRequest{OrganizationID: "org_c3", ProjectID: "project_c3", DurationMS: 3000, Width: 720, Height: 1280, FrameRate: 30, SampleRate: 48000, Visual: []TimelineVisualClip{
		{ID: "primary", Kind: "video", Asset: contract.AssetVersionRef{AssetID: "video-landscape", Version: 1}, EndMS: 3000, Fit: "contain", PositionX: 0.5, PositionY: 0.5, Scale: 1, Opacity: 1, ZIndex: 0},
		{ID: "opaque", Kind: "image", Asset: contract.AssetVersionRef{AssetID: "overlay-opaque", Version: 1}, StartMS: 500, EndMS: 2500, Fit: "contain", PositionX: 0.1, PositionY: 0.8, Scale: 0.4, CropLeft: 0.05, CropRight: 0.05, Opacity: 0.8, ZIndex: 1},
		{ID: "alpha", Kind: "image", Asset: contract.AssetVersionRef{AssetID: "overlay-alpha", Version: 1}, StartMS: 1000, EndMS: 3000, Fit: "cover", PositionX: 0.8, PositionY: 0.1, Scale: 0.3, Opacity: 0.65, ZIndex: 2},
	}}
	output, err := renderer.Render(context.Background(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := io.ReadAll(output.Content)
	if err != nil {
		t.Fatal(err)
	}
	if err := output.Content.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "c3-visual.mp4")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := goldenDigest(t, ffmpegPath, path, "0:v:0")
	if digest != manifest.Golden.C3VisualFrameMD5SHA256 {
		t.Fatalf("C3 visual golden digest=%s, want %s", digest, manifest.Golden.C3VisualFrameMD5SHA256)
	}
}

func TestFFmpegTimelinePreviewAndExportMatchGoldenMedia(t *testing.T) {
	ffmpegPath, ffprobePath := os.Getenv("COOKIES_TEST_FFMPEG_PATH"), os.Getenv("COOKIES_TEST_FFPROBE_PATH")
	videoA, videoB, audio := os.Getenv("COOKIES_TEST_TIMELINE_VIDEO_A"), os.Getenv("COOKIES_TEST_TIMELINE_VIDEO_B"), os.Getenv("COOKIES_TEST_TIMELINE_AUDIO")
	if ffmpegPath == "" || ffprobePath == "" || videoA == "" || videoB == "" || audio == "" {
		if os.Getenv("COOKIES_REQUIRE_TIMELINE_GOLDEN") == "1" {
			t.Fatal("timeline FFmpeg golden is required but its tool or fixture paths are missing")
		}
		t.Skip("set timeline FFmpeg golden fixture environment")
	}
	manifest := loadC0FixtureManifest(t, filepath.Dir(videoA))
	refs := map[contract.AssetID]string{"video-a": videoA, "video-b": videoB, "music": audio}
	source := goldenTimelineSource{files: refs}
	probe := assets.FFprobeVideoProbe{Path: ffprobePath, WorkRoot: t.TempDir()}
	renderer := FFmpegTimelineRenderer{FFmpegPath: ffmpegPath, WorkRoot: t.TempDir(), Videos: source, Audio: source, Probe: probe, Deterministic: true}
	request := TimelineRenderRequest{OrganizationID: "org_golden", ProjectID: "project_golden", DurationMS: manifest.Golden.TimelineDurationMS, Width: 720, Height: 1280, FrameRate: 30, SampleRate: 48000, Video: []TimelineVideoClip{{ID: "a", Asset: contract.AssetVersionRef{AssetID: "video-a", Version: 1}, EndMS: 6000}, {ID: "b", Asset: contract.AssetVersionRef{AssetID: "video-b", Version: 1}, StartMS: 6000, EndMS: 12000}}, Audio: []TimelineAudioClip{{ID: "music", Role: TimelineAudioMusic, Asset: contract.AssetVersionRef{AssetID: "music", Version: 1}, EndMS: 12000, GainDB: -12, Loop: true}}, Captions: []TimelineCaption{{StartMS: 1000, EndMS: 5000, Text: manifest.Golden.Caption}}}
	render := func(name string) string {
		output, err := renderer.Render(context.Background(), request, nil)
		if err != nil {
			t.Fatal(err)
		}
		contents, err := io.ReadAll(output.Content)
		if err != nil {
			t.Fatal(err)
		}
		if err = output.Content.Close(); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), name+".mp4")
		if err = os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	preview, exported := render("preview"), render("export")
	previewMeta, err := probeFile(context.Background(), probe, preview)
	if err != nil {
		t.Fatal(err)
	}
	exportMeta, err := probeFile(context.Background(), probe, exported)
	if err != nil {
		t.Fatal(err)
	}
	if previewMeta.DurationMS != exportMeta.DurationMS || previewMeta.WidthPixels != 720 || previewMeta.HeightPixels != 1280 || previewMeta.VideoCodec != "h264" || previewMeta.AudioCodec == "" {
		t.Fatalf("metadata mismatch preview=%+v export=%+v", previewMeta, exportMeta)
	}
	expected := map[string]string{
		"0:v:0": manifest.Golden.VideoFrameMD5SHA256,
		"0:a:0": manifest.Golden.AudioFrameMD5SHA256,
	}
	for selector, expectedDigest := range expected {
		previewDigest := goldenDigest(t, ffmpegPath, preview, selector)
		if previewDigest != goldenDigest(t, ffmpegPath, exported, selector) {
			t.Fatalf("preview/export %s frame digest mismatch", selector)
		}
		if previewDigest != expectedDigest {
			t.Fatalf("%s golden digest=%s, want %s", selector, previewDigest, expectedDigest)
		}
	}
}

func TestFFmpegC7FrozenMultitrackPreviewAndExportMatchGolden(t *testing.T) {
	ffmpegPath, ffprobePath := os.Getenv("COOKIES_TEST_FFMPEG_PATH"), os.Getenv("COOKIES_TEST_FFPROBE_PATH")
	fixtureRoot := os.Getenv("COOKIES_C0_FIXTURE_ROOT")
	if ffmpegPath == "" || ffprobePath == "" || fixtureRoot == "" {
		if os.Getenv("COOKIES_REQUIRE_TIMELINE_GOLDEN") == "1" {
			t.Fatal("C7 full-timeline golden is required but its tool or fixture paths are missing")
		}
		t.Skip("set C7 full-timeline golden fixture environment")
	}
	manifest := loadC0FixtureManifest(t, fixtureRoot)
	files := map[contract.AssetID]string{}
	for _, fixture := range manifest.Assets {
		files[contract.AssetID(fixture.ID)] = filepath.Join(fixtureRoot, fixture.File)
	}
	source := goldenTimelineSource{files: files, withAudio: map[contract.AssetID]bool{"video-original-audio": true}}
	probe := assets.FFprobeVideoProbe{Path: ffprobePath, WorkRoot: t.TempDir()}
	renderer := FFmpegTimelineRenderer{FFmpegPath: ffmpegPath, WorkRoot: t.TempDir(), Visuals: source, Audio: source, Probe: probe, Deterministic: true}
	fontSHA := ""
	for _, fixture := range manifest.Assets {
		if fixture.ID == "font" {
			fontSHA = fixture.SHA256
		}
	}
	style := TimelineCaptionStyle{ID: "fixture-caption", Version: 1, FontFamily: "Noto Sans CJK SC", FontSHA256: fontSHA, FontSize: 44, PrimaryColor: "#FFFFFF", OutlineColor: "#101010", Outline: 3, Shadow: 1, Alignment: 2, MarginHorizontal: 70, MarginVertical: 80, MaxCharsPerLine: 18, EmphasisColor: "#38BDF8"}
	request := TimelineRenderRequest{
		OrganizationID: "org_c7", ProjectID: "project_c7", DurationMS: 12000, Width: 720, Height: 1280, FrameRate: 30, SampleRate: 48000,
		Visual: []TimelineVisualClip{
			{ID: "original", Kind: "video", Asset: contract.AssetVersionRef{AssetID: "video-original-audio", Version: 1}, EndMS: 6000, Fit: "contain", PositionX: 0.5, PositionY: 0.5, Scale: 1, Opacity: 1, ZIndex: 0},
			{ID: "landscape", Kind: "video", Asset: contract.AssetVersionRef{AssetID: "video-landscape", Version: 1}, StartMS: 6000, EndMS: 12000, SourceIn: 0, Fit: "cover", PositionX: 0.35, PositionY: 0.5, Scale: 1, CropLeft: 0.05, CropRight: 0.05, Opacity: 1, ZIndex: 0},
			{ID: "opaque", Kind: "image", Asset: contract.AssetVersionRef{AssetID: "overlay-opaque", Version: 1}, StartMS: 500, EndMS: 4500, Fit: "contain", PositionX: 0.05, PositionY: 0.78, Scale: 0.35, Opacity: 0.8, ZIndex: 1},
			{ID: "alpha", Kind: "image", Asset: contract.AssetVersionRef{AssetID: "overlay-alpha", Version: 1}, StartMS: 7000, EndMS: 11000, Fit: "contain", PositionX: 0.65, PositionY: 0.08, Scale: 0.3, CropTop: 0.05, CropBottom: 0.05, Opacity: 0.65, ZIndex: 2},
		},
		OriginalAudio: []TimelineOriginalAudioClip{{VisualClipID: "original", StartMS: 0, EndMS: 6000, GainDB: -8, FadeInMS: 250, FadeOutMS: 500}},
		Audio: []TimelineAudioClip{
			{ID: "voice", Role: TimelineAudioVoiceover, Asset: contract.AssetVersionRef{AssetID: "voice", Version: 1}, StartMS: 1000, EndMS: 4000, GainDB: -3, FadeInMS: 200, FadeOutMS: 300},
			{ID: "music", Role: TimelineAudioMusic, Asset: contract.AssetVersionRef{AssetID: "music", Version: 1}, EndMS: 12000, GainDB: -14, FadeInMS: 500, FadeOutMS: 700, Loop: true},
			{ID: "sfx", Role: TimelineAudioSFX, Asset: contract.AssetVersionRef{AssetID: "sfx", Version: 1}, StartMS: 6500, EndMS: 7500, GainDB: -6, FadeInMS: 50, FadeOutMS: 150},
		},
		Captions: []TimelineCaption{
			{StartMS: 1000, EndMS: 5000, Text: "固定黄金字幕", StyleID: style.ID, Emphasis: []TimelineCaptionEmphasis{{StartRune: 2, EndRune: 4}}},
			{StartMS: 7000, EndMS: 10000, Text: "预览与导出一致", StyleID: style.ID},
		},
		CaptionStyles: []TimelineCaptionStyle{style},
	}
	render := func(name string) string {
		output, err := renderer.Render(context.Background(), request, nil)
		if err != nil {
			t.Fatal(err)
		}
		contents, err := io.ReadAll(output.Content)
		if err != nil {
			t.Fatal(err)
		}
		if err := output.Content.Close(); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), name+".mp4")
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	preview, exported := render("c7-preview"), render("c7-export")
	for _, path := range []string{preview, exported} {
		metadata, err := probeFile(context.Background(), probe, path)
		if err != nil {
			t.Fatal(err)
		}
		if metadata.DurationMS != 12000 || metadata.WidthPixels != 720 || metadata.HeightPixels != 1280 || metadata.VideoCodec != "h264" || metadata.AudioCodec != "aac" {
			t.Fatalf("C7 output profile mismatch: %+v", metadata)
		}
	}
	for selector, expected := range map[string]string{"0:v:0": manifest.Golden.C7VideoFrameMD5SHA256, "0:a:0": manifest.Golden.C7AudioFrameMD5SHA256} {
		previewDigest := goldenDigest(t, ffmpegPath, preview, selector)
		exportDigest := goldenDigest(t, ffmpegPath, exported, selector)
		if previewDigest != exportDigest {
			t.Fatalf("C7 preview/export %s digest mismatch: preview=%s export=%s", selector, previewDigest, exportDigest)
		}
		if previewDigest != expected {
			t.Fatalf("C7 %s golden digest=%s, want %s", selector, previewDigest, expected)
		}
	}
}

func loadC0FixtureManifest(t *testing.T, fixtureRoot string) c0FixtureManifest {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("testdata", "video-editor-c0", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest c0FixtureManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != "cookies/video-editor-c0-fixtures/v1" || len(manifest.Assets) != 11 || manifest.Golden.TimelineDurationMS != 12000 || manifest.Golden.Caption == "" {
		t.Fatalf("invalid C0 fixture manifest: %#v", manifest)
	}
	seen := make(map[string]bool, len(manifest.Assets))
	for _, asset := range manifest.Assets {
		if asset.ID == "" || asset.File == "" || len(asset.SHA256) != 64 || seen[asset.ID] {
			t.Fatalf("invalid C0 fixture entry: %#v", asset)
		}
		seen[asset.ID] = true
		data, err := os.ReadFile(filepath.Join(fixtureRoot, asset.File))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		if actual := hex.EncodeToString(digest[:]); actual != asset.SHA256 {
			t.Fatalf("fixture %s sha256=%s, want %s", asset.ID, actual, asset.SHA256)
		}
	}
	for _, required := range []string{"video-a", "video-b", "video-landscape", "video-original-audio", "overlay-opaque", "overlay-alpha", "voice", "music", "sfx", "caption", "font"} {
		if !seen[required] {
			t.Fatal(fmt.Errorf("C0 fixture %s is missing", required))
		}
	}
	return manifest
}

func probeFile(ctx context.Context, probe assets.FFprobeVideoProbe, path string) (assets.VideoMetadata, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return assets.VideoMetadata{}, err
	}
	return probe.Probe(ctx, contents)
}
func goldenDigest(t *testing.T, ffmpegPath, path, selector string) string {
	t.Helper()
	command := exec.Command(ffmpegPath, "-hide_banner", "-loglevel", "error", "-i", path, "-map", selector, "-f", "framemd5", "-")
	contents, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}
