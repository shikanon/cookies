package media

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/contract"
)

type audioMixTestSource struct{}

func (audioMixTestSource) OpenVideo(context.Context, contract.OrganizationID, contract.ProjectID, contract.AssetVersionRef) (assets.AssetVersion, io.ReadCloser, error) {
	return assets.AssetVersion{}, io.NopCloser(bytes.NewReader([]byte("video"))), nil
}
func (audioMixTestSource) OpenVisual(context.Context, contract.OrganizationID, contract.ProjectID, contract.AssetVersionRef) (assets.AssetVersion, io.ReadCloser, error) {
	return assets.AssetVersion{}, io.NopCloser(bytes.NewReader([]byte("visual"))), nil
}
func (audioMixTestSource) OpenAudio(context.Context, contract.OrganizationID, contract.ProjectID, contract.AssetVersionRef) (assets.AssetVersion, io.ReadCloser, error) {
	return assets.AssetVersion{}, io.NopCloser(bytes.NewReader([]byte("audio"))), nil
}

type audioMixTestRunner struct{ args []string }

func (r *audioMixTestRunner) Run(_ context.Context, _ string, args ...string) error {
	r.args = append([]string(nil), args...)
	return os.WriteFile(args[len(args)-1], []byte("mixed-video"), 0o600)
}

type audioMixTestProbe struct{ duration int64 }

func (p audioMixTestProbe) Probe(context.Context, []byte) (assets.VideoMetadata, error) {
	return assets.VideoMetadata{DurationMS: p.duration, WidthPixels: 720, HeightPixels: 1280, FrameRate: "25/1", VideoCodec: "h264", AudioCodec: "aac"}, nil
}

func TestBuildAudioMixFilterAppliesTimelineProcessingDuckingAndMastering(t *testing.T) {
	t.Parallel()
	request := AudioMixRequest{OrganizationID: "org_1", ProjectID: "project_1", Visual: contract.AssetVersionRef{AssetID: "visual", Version: 1}, MasterDurationMS: 30000, SampleRate: 48000, ChannelLayout: "stereo", Clips: []AudioMixClip{
		{ID: "voice", TrackType: "voiceover", Asset: contract.AssetVersionRef{AssetID: "voice", Version: 1}, TimelineStartMS: 1000, TimelineEndMS: 5500, SourceOutMS: 4500, GainDB: 1, FadeInMS: 80, FadeOutMS: 120, PlaybackRate: 1},
		{ID: "music", TrackType: "music", Asset: contract.AssetVersionRef{AssetID: "music", Version: 1}, TimelineEndMS: 30000, SourceOutMS: 30000, GainDB: -18, FadeInMS: 800, FadeOutMS: 1200, PlaybackRate: 1},
	}}
	graph, output := BuildAudioMixFilter(request)
	for _, want := range []string{"adelay=1000|1000", "volume=-18.000dB", "afade=t=in", "asplit=2[voicesidechain][voiceout]", "sidechaincompress", "alimiter", "loudnorm=I=-16", "atrim=duration=30.000"} {
		if !strings.Contains(graph, want) {
			t.Fatalf("filter graph %q does not contain %q", graph, want)
		}
	}
	if output != "[mixout]" {
		t.Fatalf("output = %q", output)
	}
	if strings.Contains(graph, "amix=inputs=1") || !strings.Contains(graph, "anull[musicbus]") {
		t.Fatalf("single-input bus must bypass amix: %q", graph)
	}
}

func TestAudioMixRequestSupportsStandardAndCustomDurations(t *testing.T) {
	t.Parallel()
	for _, duration := range []int{15000, 30000, 47000} {
		request := AudioMixRequest{OrganizationID: "org_1", ProjectID: "project_1", Visual: contract.AssetVersionRef{AssetID: "visual", Version: 1}, MasterDurationMS: duration, SampleRate: 48000, ChannelLayout: "stereo", Clips: []AudioMixClip{{ID: "music", TrackType: "music", Asset: contract.AssetVersionRef{AssetID: "music", Version: 1}, TimelineEndMS: duration, SourceOutMS: duration, PlaybackRate: 1}}}
		if err := request.Validate(); err != nil {
			t.Fatalf("duration %d rejected: %v", duration, err)
		}
		graph, _ := BuildAudioMixFilter(request)
		if !strings.Contains(graph, "atrim=duration="+seconds(duration)) {
			t.Fatalf("duration %d not compiled", duration)
		}
	}
}

func TestFFmpegAudioMixRendererProducesValidatedOwnedOutput(t *testing.T) {
	t.Parallel()
	runner := &audioMixTestRunner{}
	request := AudioMixRequest{OrganizationID: "org_1", ProjectID: "project_1", Visual: contract.AssetVersionRef{AssetID: "visual", Version: 1}, MasterDurationMS: 15000, SampleRate: 48000, ChannelLayout: "stereo", Clips: []AudioMixClip{{ID: "music", TrackType: "music", Asset: contract.AssetVersionRef{AssetID: "music", Version: 1}, TimelineEndMS: 15000, SourceOutMS: 15000, PlaybackRate: 1}}}
	renderer := FFmpegAudioMixRenderer{FFmpegPath: "ffmpeg", WorkRoot: t.TempDir(), Videos: audioMixTestSource{}, Audio: audioMixTestSource{}, Probe: audioMixTestProbe{duration: 15000}, Runner: runner}
	output, err := renderer.RenderAudioMix(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Content.Close()
	contents, err := io.ReadAll(output.Content)
	if err != nil || string(contents) != "mixed-video" {
		t.Fatalf("unexpected output %q: %v", contents, err)
	}
	joined := strings.Join(runner.args, " ")
	for _, want := range []string{"-filter_complex", "-map 0:v:0", "-c:a aac", "-t 15.000"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("ffmpeg args %q do not contain %q", joined, want)
		}
	}
}
