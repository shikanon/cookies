package media

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/contract"
)

type squareTimelineProbe struct{}

func (squareTimelineProbe) Probe(context.Context, []byte) (assets.VideoMetadata, error) {
	return assets.VideoMetadata{DurationMS: 3000, WidthPixels: 1080, HeightPixels: 1080, FrameRate: "30/1", VideoCodec: "h264", AudioCodec: "aac"}, nil
}

func TestFFmpegTimelineRendererDeterministicModePinsCPUAndThreads(t *testing.T) {
	runner := &timelineProgressRunnerStub{}
	request := TimelineRenderRequest{OrganizationID: "org_1", ProjectID: "project_1", DurationMS: 3000, Width: 720, Height: 1280, FrameRate: 30, SampleRate: 48000,
		Video: []TimelineVideoClip{{ID: "v1", Asset: contract.AssetVersionRef{AssetID: "video", Version: 1}, EndMS: 3000}}}
	renderer := FFmpegTimelineRenderer{FFmpegPath: "ffmpeg", WorkRoot: t.TempDir(), Videos: audioMixTestSource{}, Audio: audioMixTestSource{}, Probe: audioMixTestProbe{duration: 3000}, Runner: runner, Deterministic: true}
	output, err := renderer.Render(context.Background(), request, func(TimelineProgress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := output.Content.Close(); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(runner.args, " ")
	for _, expected := range []string{"-cpuflags 0", "-filter_threads 1", "-filter_complex_threads 1", "-threads 1", "-x264-params asm=0"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("deterministic FFmpeg args missing %q: %s", expected, joined)
		}
	}
}

type timelineProgressRunnerStub struct {
	args []string
}

func TestCleanupTimelineWorkRootOnlyRemovesExpiredRendererDirectories(t *testing.T) {
	root := t.TempDir()
	expired := filepath.Join(root, "timeline-expired")
	active := filepath.Join(root, "timeline-active")
	unrelated := filepath.Join(root, "other-work")
	for _, path := range []string{expired, active, unrelated} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(expired, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(unrelated, old, old); err != nil {
		t.Fatal(err)
	}
	if err := CleanupTimelineWorkRoot(root, time.Now().Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(expired); !os.IsNotExist(err) {
		t.Fatalf("expired renderer directory was not removed: %v", err)
	}
	for _, path := range []string{active, unrelated} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("safe directory %s was removed: %v", path, err)
		}
	}
}

func (r *timelineProgressRunnerStub) Run(_ context.Context, _ string, args []string, report TimelineProgressFunc, totalMS int) error {
	r.args = append([]string(nil), args...)
	if err := report(TimelineProgress{Percent: 50, OutTimeMS: totalMS / 2}); err != nil {
		return err
	}
	return os.WriteFile(args[len(args)-1], []byte("final-video"), 0o600)
}

func TestFFmpegTimelineRendererProducesValidatedOutputAndCleansWorkDirectory(t *testing.T) {
	workRoot := t.TempDir()
	runner := &timelineProgressRunnerStub{}
	request := TimelineRenderRequest{OrganizationID: "org_1", ProjectID: "project_1", DurationMS: 15000, Width: 720, Height: 1280, FrameRate: 30, SampleRate: 48000,
		Video:    []TimelineVideoClip{{ID: "v1", Asset: contract.AssetVersionRef{AssetID: "video", Version: 1}, StartMS: 0, EndMS: 15000}},
		Audio:    []TimelineAudioClip{{ID: "voice", Role: TimelineAudioVoiceover, Asset: contract.AssetVersionRef{AssetID: "audio", Version: 1}, StartMS: 0, EndMS: 5000}},
		Captions: []TimelineCaption{{StartMS: 0, EndMS: 5000, Text: "核心卖点"}}}
	renderer := FFmpegTimelineRenderer{FFmpegPath: "ffmpeg", WorkRoot: workRoot, Videos: audioMixTestSource{}, Audio: audioMixTestSource{}, Probe: audioMixTestProbe{duration: 15000}, Runner: runner}
	progress := []int{}
	output, err := renderer.Render(context.Background(), request, func(value TimelineProgress) error { progress = append(progress, value.Percent); return nil })
	if err != nil {
		t.Fatal(err)
	}
	contents, readErr := io.ReadAll(output.Content)
	if readErr != nil || string(contents) != "final-video" {
		t.Fatalf("unexpected output %q: %v", contents, readErr)
	}
	if err := output.Content.Close(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(workRoot)
	if len(entries) != 0 {
		t.Fatalf("work directory leaked after output close: %v", entries)
	}
	joined := strings.Join(runner.args, " ")
	for _, expected := range []string{"-progress pipe:1", "-filter_complex", "libx264", "-pix_fmt yuv420p", "-c:a aac", "+faststart"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("ffmpeg args missing %q: %s", expected, joined)
		}
	}
	if len(progress) != 1 || progress[0] != 50 {
		t.Fatalf("progress not forwarded: %v", progress)
	}
}

func TestFFmpegTimelineRendererOpensC3ImagesAndVideosAsVisualInputs(t *testing.T) {
	runner := &timelineProgressRunnerStub{}
	request := TimelineRenderRequest{OrganizationID: "org_1", ProjectID: "project_1", DurationMS: 3000, Width: 1080, Height: 1080, FrameRate: 30, SampleRate: 48000,
		Visual: []TimelineVisualClip{
			{ID: "image", Kind: "image", Asset: contract.AssetVersionRef{AssetID: "image", Version: 1}, EndMS: 3000, Fit: "contain", PositionX: 0.5, PositionY: 0.5, Scale: 1, Opacity: 1, ZIndex: 1},
			{ID: "video", Kind: "video", Asset: contract.AssetVersionRef{AssetID: "video", Version: 1}, EndMS: 3000, Fit: "cover", PositionX: 0.5, PositionY: 0.5, Scale: 1, Opacity: 1, ZIndex: 0},
		},
	}
	renderer := FFmpegTimelineRenderer{FFmpegPath: "ffmpeg", WorkRoot: t.TempDir(), Visuals: audioMixTestSource{}, Audio: audioMixTestSource{}, Probe: squareTimelineProbe{}, Runner: runner}
	output, err := renderer.Render(context.Background(), request, func(TimelineProgress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer output.Content.Close()
	joined := strings.Join(runner.args, " ")
	if !strings.Contains(joined, "-i "+filepath.Join(filepath.Dir(runner.args[len(runner.args)-1]), "visual-01.bin")) || !strings.Contains(joined, "-loop 1 -i "+filepath.Join(filepath.Dir(runner.args[len(runner.args)-1]), "visual-02.bin")) {
		t.Fatalf("visual inputs were not opened in z order with still-image looping: %s", joined)
	}
}
