package media

import (
	"strings"
	"testing"

	"github.com/shikanon/cookies/internal/platform/contract"
)

func TestTimelineRenderRequestAcceptsClosedVerticalTimeline(t *testing.T) {
	request := TimelineRenderRequest{
		OrganizationID: "org_1", ProjectID: "project_1", DurationMS: 20000,
		Width: 720, Height: 1280, FrameRate: 30, SampleRate: 48000,
		Video: []TimelineVideoClip{
			{ID: "v1", Asset: contract.AssetVersionRef{AssetID: "video_1", Version: 1}, StartMS: 0, EndMS: 4000},
			{ID: "v2", Asset: contract.AssetVersionRef{AssetID: "video_2", Version: 1}, StartMS: 4000, EndMS: 20000},
		},
		Audio:    []TimelineAudioClip{{ID: "voice_1", Role: TimelineAudioVoiceover, Asset: contract.AssetVersionRef{AssetID: "audio_1", Version: 1}, StartMS: 0, EndMS: 3500}},
		Captions: []TimelineCaption{{StartMS: 0, EndMS: 3500, Text: "通勤背包，容量真的够吗？"}},
	}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	request.Video[1].StartMS = 4500
	if err := request.Validate(); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("timeline gap should be rejected: %v", err)
	}
}

func TestASSSubtitlesEscapeContentAndUseVerticalSafeArea(t *testing.T) {
	contents, err := BuildASSSubtitles([]TimelineCaption{{StartMS: 1200, EndMS: 3600, Text: "轻便 {耐磨}\\防泼水\n适合通勤"}}, 720, 1280)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, expected := range []string{"PlayResX: 720", "PlayResY: 1280", ",70,70,80,1", "0:00:01.20,0:00:03.60", `\{耐磨\}`, `\\防泼水`, `\N适合通勤`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("ASS output missing %q:\n%s", expected, text)
		}
	}
}

func TestC4ASSSubtitlesUseVersionedBrandStyleAndKeywordEmphasis(t *testing.T) {
	style := TimelineCaptionStyle{
		ID: "brand-white", Version: 3, FontFamily: "Cookies Fixture Sans", FontSHA256: strings.Repeat("a", 64),
		FontSize: 44, PrimaryColor: "#F8FAFC", OutlineColor: "#0F172A", Outline: 3, Shadow: 1,
		Alignment: 2, MarginHorizontal: 64, MarginVertical: 96, MaxCharsPerLine: 12,
		EmphasisColor: "#38BDF8",
	}
	contents, diagnostics, err := BuildASSSubtitlesWithStyles([]TimelineCaption{{
		StartMS: 1000, EndMS: 2500, Text: "中文 English 123，标点。", StyleID: style.ID,
		Emphasis: []TimelineCaptionEmphasis{{StartRune: 3, EndRune: 10}},
	}}, 720, 1280, []TimelineCaptionStyle{style})
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, expected := range []string{"Style: brand-white,Cookies Fixture Sans,44", "FontSHA256: " + strings.Repeat("a", 64), `{\c&H00F8BD38&}English{\c&H00FCFAF8&}`, `\N`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("styled ASS output missing %q:\n%s", expected, text)
		}
	}
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected subtitle diagnostics: %#v", diagnostics)
	}
}

func TestBuildTimelineFilterIncludesDuckingMixAndSubtitleBurnIn(t *testing.T) {
	request := TimelineRenderRequest{DurationMS: 15000, Width: 720, Height: 1280, FrameRate: 30, SampleRate: 48000,
		Video: []TimelineVideoClip{{ID: "v1", StartMS: 0, EndMS: 15000}},
		Audio: []TimelineAudioClip{
			{ID: "voice", Role: TimelineAudioVoiceover, StartMS: 0, EndMS: 5000, GainDB: 0},
			{ID: "music", Role: TimelineAudioMusic, StartMS: 0, EndMS: 15000, GainDB: -12, Loop: true},
			{ID: "sfx", Role: TimelineAudioSFX, StartMS: 4500, EndMS: 5200, GainDB: -3},
		}}
	graph, videoLabel, audioLabel := BuildTimelineFilter(request, "subtitles.ass")
	for _, expected := range []string{"concat=n=1:v=1:a=0", "subtitles=", "sidechaincompress", "amix=inputs=4", "loudnorm", "alimiter"} {
		if !strings.Contains(graph, expected) {
			t.Fatalf("filter graph missing %q: %s", expected, graph)
		}
	}
	if videoLabel != "[videoout]" || audioLabel != "[audioout]" {
		t.Fatalf("unexpected output labels: %s %s", videoLabel, audioLabel)
	}
}

func TestBuildTimelineFilterUsesAudioSourceInBeforeLooping(t *testing.T) {
	request := TimelineRenderRequest{DurationMS: 15000, Width: 720, Height: 1280, FrameRate: 30, SampleRate: 48000,
		Video: []TimelineVideoClip{{ID: "v1", Asset: contract.AssetVersionRef{AssetID: "video", Version: 1}, StartMS: 0, EndMS: 15000}},
		Audio: []TimelineAudioClip{{ID: "music", Role: TimelineAudioMusic, Asset: contract.AssetVersionRef{AssetID: "music", Version: 1}, StartMS: 0, EndMS: 15000, SourceIn: 1200, Loop: true}},
	}
	graph, _, _ := BuildTimelineFilter(request, "subtitles.ass")
	if !strings.Contains(graph, "atrim=start=1.2") {
		t.Fatalf("looping music must honor source_in before loop: %s", graph)
	}
}

func TestC5TimelineFilterAppliesOriginalAudioFadeAndMutedTracksAreAbsent(t *testing.T) {
	request := TimelineRenderRequest{DurationMS: 5000, Width: 720, Height: 1280, FrameRate: 30, SampleRate: 48000,
		Visual:        []TimelineVisualClip{{ID: "video", Kind: "video", StartMS: 0, EndMS: 5000, Fit: "contain", PositionX: .5, PositionY: .5, Scale: 1, Opacity: 1}},
		OriginalAudio: []TimelineOriginalAudioClip{{VisualClipID: "video", StartMS: 0, EndMS: 5000, SourceIn: 1000, GainDB: -3, FadeInMS: 500, FadeOutMS: 750}},
		Audio:         []TimelineAudioClip{{ID: "voice", Role: TimelineAudioVoiceover, StartMS: 1000, EndMS: 4000, SourceIn: 200, GainDB: -6, FadeInMS: 300, FadeOutMS: 400}},
	}
	graph, _, _ := BuildTimelineFilter(request, "subtitles.ass")
	for _, expected := range []string{"[0:a]atrim=start=1.000:duration=5.000", "volume=-3.00dB", "afade=t=in:st=0:d=0.500", "afade=t=out:st=4.250:d=0.750", "atrim=start=0.200:duration=3.000", "afade=t=out:st=2.600:d=0.400", "amix=inputs=3"} {
		if !strings.Contains(graph, expected) {
			t.Fatalf("C5 filter missing %q: %s", expected, graph)
		}
	}
}

func TestBuildTimelineFilterCompositesC3VisualLayersByZOrder(t *testing.T) {
	request := TimelineRenderRequest{DurationMS: 3000, Width: 1080, Height: 1080, FrameRate: 30, SampleRate: 48000,
		Visual: []TimelineVisualClip{
			{ID: "overlay-video", Kind: "video", StartMS: 1000, EndMS: 2500, SourceIn: 500, Fit: "cover", PositionX: 0.5, PositionY: 0.5, Scale: 1.2, Opacity: 1, ZIndex: 2},
			{ID: "primary", Kind: "video", EndMS: 3000, Fit: "contain", PositionX: 0.5, PositionY: 0.5, Scale: 1, Opacity: 1, ZIndex: 0},
			{ID: "logo", Kind: "image", StartMS: 500, EndMS: 2000, Fit: "contain", PositionX: 0.1, PositionY: 0.9, Scale: 0.3, CropLeft: 0.1, Opacity: 0.6, ZIndex: 1},
		},
	}
	graph, video, audio := BuildTimelineFilter(request, "subtitles.ass")
	for _, fragment := range []string{
		"color=c=#000000:s=1080x1080:r=30:d=3.000[base0]",
		"[0:v]trim=start=0.000:duration=3.000",
		"[1:v]trim=duration=1.500",
		"colorchannelmixer=aa=0.600",
		"[2:v]trim=start=0.500:duration=1.500",
		"overlay=x='(W-w)*0.100000':y='(H-h)*0.900000'",
		"enable='between(t,0.500,2.000)'",
	} {
		if !strings.Contains(graph, fragment) {
			t.Fatalf("visual graph missing %q: %s", fragment, graph)
		}
	}
	if video != "[videoout]" || audio != "[audioout]" {
		t.Fatalf("unexpected output labels %s/%s", video, audio)
	}
}

func TestTimelineRenderProfileAcceptsFrozen15_20And30SecondDurations(t *testing.T) {
	for _, duration := range []int{15000, 20000, 30000} {
		request := TimelineRenderRequest{OrganizationID: "org_1", ProjectID: "project_1", DurationMS: duration, Width: 720, Height: 1280, FrameRate: 30, SampleRate: 48000,
			Video: []TimelineVideoClip{{ID: "video", Asset: contract.AssetVersionRef{AssetID: "video", Version: 1}, StartMS: 0, EndMS: duration}}}
		if err := request.Validate(); err != nil {
			t.Fatalf("%dms fixture rejected: %v", duration, err)
		}
	}
}
