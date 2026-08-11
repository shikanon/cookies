package creative

import (
	"reflect"
	"testing"

	"github.com/shikanon/cookies/internal/platform/contract"
)

func TestCompilerRegistryKeepsTheV1CompilerAndSelectsV2Explicitly(t *testing.T) {
	v1 := *DefaultEditingCodecRegistryMustMigrateFixture(t).V1
	legacy, err := CompileEditingTimelineV1(v1, "org_1", "project_1")
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := DefaultEditingCompilerRegistry().Compile(EditingDocument{V1: &v1}, "org_1", "project_1")
	if err != nil {
		t.Fatal(err)
	}
	if compiled.CompilerVersion != "editing-v1" || !reflect.DeepEqual(legacy, compiled.MediaRequest) {
		t.Fatalf("v1 compiler drifted: %#v", compiled)
	}
	v2, err := DefaultEditingCodecRegistry().MigrateToV2(EditingDocument{V1: &v1})
	if err != nil {
		t.Fatal(err)
	}
	compiled, err = DefaultEditingCompilerRegistry().Compile(v2, "org_1", "project_1")
	if err != nil {
		t.Fatal(err)
	}
	if compiled.CompilerVersion != "editing-v2-audio-c5" || compiled.IR.SchemaVersion != "editing-render-ir/v2" || len(compiled.IR.Inputs) != 1 {
		t.Fatalf("v2 compiler not selected: %#v", compiled)
	}
}

func TestC3CompilerProducesDeterministicThreeTrackVisualRenderIR(t *testing.T) {
	document := operationTestDocument()
	document.V2.DurationFrames = 90
	imageRef := contract.AssetVersionRef{AssetID: "image-overlay", Version: 2}
	videoRef := contract.AssetVersionRef{AssetID: "video-overlay", Version: 4}
	imageTransform := EditingVisualTransformV2{Fit: "contain", PositionX: 0.2, PositionY: 0.8, Scale: 0.5, Opacity: 0.75}
	videoTransform := EditingVisualTransformV2{Fit: "cover", PositionX: 0.5, PositionY: 0.5, Scale: 1.2, Opacity: 1}
	document.V2.Tracks = append(document.V2.Tracks,
		EditingTrackV2{ID: "visual-overlay-1", Kind: "visual", Role: "overlay", ZIndex: 1, Clips: []EditingClipV2{{ID: "image-overlay-clip", Kind: "image", AssetRef: &imageRef, Timeline: EditingTimelineRangeV2{StartFrame: 15, DurationFrames: 30}, Transform: &imageTransform}}},
		EditingTrackV2{ID: "visual-overlay-2", Kind: "visual", Role: "overlay", ZIndex: 2, Clips: []EditingClipV2{{ID: "video-overlay-clip", Kind: "video", AssetRef: &videoRef, Timeline: EditingTimelineRangeV2{StartFrame: 30, DurationFrames: 30}, Source: &EditingSourceRangeV2{InUS: 500_000, OutUS: 1_500_000}, Transform: &videoTransform, OriginalAudio: &EditingOriginalAudioV2{Enabled: false}}}},
	)

	compiled, err := DefaultEditingCompilerRegistry().Compile(document, "org_1", "project_1")
	if err != nil {
		t.Fatal(err)
	}
	if compiled.CompilerVersion != "editing-v2-audio-c5" || len(compiled.IR.VisualLayers) != 3 || len(compiled.MediaRequest.Visual) != 3 {
		t.Fatalf("C3 visual compiler output is incomplete: %#v", compiled)
	}
	if compiled.MediaRequest.Visual[1].Kind != "image" || compiled.MediaRequest.Visual[1].ZIndex != 1 || compiled.MediaRequest.Visual[1].SourceIn != 0 {
		t.Fatalf("image render instruction is invalid: %#v", compiled.MediaRequest.Visual[1])
	}
	if compiled.MediaRequest.Visual[2].Kind != "video" || compiled.MediaRequest.Visual[2].SourceIn != 500 {
		t.Fatalf("overlay video render instruction is invalid: %#v", compiled.MediaRequest.Visual[2])
	}
}

func TestC4CompilerPreservesCaptionFramesStyleAndEmphasis(t *testing.T) {
	document := operationTestDocument()
	document.V2.Tracks = append(document.V2.Tracks, EditingTrackV2{ID: "captions-main", Kind: "caption", Language: "zh-CN", Clips: []EditingClipV2{{
		ID: "caption-1", Kind: "caption", Timeline: EditingTimelineRangeV2{StartFrame: 30, DurationFrames: 45}, Text: "中文 English 123。",
		StyleRef: &EditingCaptionStyleRefV2{StyleID: "brand-default", Version: 1}, Emphasis: []EditingCaptionEmphasisV2{{StartRune: 3, EndRune: 10}},
	}}})
	compiled, err := DefaultEditingCompilerRegistry().Compile(document, "org_1", "project_1")
	if err != nil {
		t.Fatal(err)
	}
	if compiled.CompilerVersion != "editing-v2-audio-c5" || len(compiled.IR.Captions) != 1 || compiled.IR.Captions[0].StyleRef.StyleID != "brand-default" {
		t.Fatalf("caption render IR is incomplete: %#v", compiled.IR.Captions)
	}
	caption := compiled.MediaRequest.Captions[0]
	if caption.StartMS != 1000 || caption.EndMS != 2500 || caption.StyleID != "brand-default" || len(caption.Emphasis) != 1 || caption.Emphasis[0].StartRune != 3 || len(compiled.MediaRequest.CaptionStyles) != 1 || compiled.MediaRequest.CaptionStyles[0].FontSHA256 == "" {
		t.Fatalf("caption media request drifted: %#v", caption)
	}
}

func TestC5CompilerPreservesOriginalAudioAndThreeBusClipProperties(t *testing.T) {
	document := operationTestDocument()
	ref := contract.AssetVersionRef{AssetID: "audio-music", Version: 2}
	document.V2.Tracks = append(document.V2.Tracks, EditingTrackV2{ID: "audio-music", Kind: "audio", Role: "music", Clips: []EditingClipV2{{
		ID: "music", Kind: "audio", AssetRef: &ref, Timeline: EditingTimelineRangeV2{StartFrame: 15, DurationFrames: 60}, Source: &EditingSourceRangeV2{InUS: 500_000, OutUS: 2_500_000}, GainDB: -12, FadeInFrames: 15, FadeOutFrames: 30, Loop: true,
	}}})
	compiled, err := DefaultEditingCompilerRegistry().Compile(document, "org_1", "project_1")
	if err != nil {
		t.Fatal(err)
	}
	if compiled.CompilerVersion != "editing-v2-audio-c5" || len(compiled.MediaRequest.OriginalAudio) != 1 || len(compiled.MediaRequest.Audio) != 1 {
		t.Fatalf("C5 audio compiler output is incomplete: %#v", compiled)
	}
	audio := compiled.MediaRequest.Audio[0]
	if audio.Role != "music" || audio.StartMS != 500 || audio.SourceIn != 500 || audio.GainDB != -12 || audio.FadeInMS != 500 || audio.FadeOutMS != 1000 || !audio.Loop {
		t.Fatalf("C5 audio properties drifted: %#v", audio)
	}
}

func DefaultEditingCodecRegistryMustMigrateFixture(t *testing.T) EditingDocument {
	t.Helper()
	doc := operationTestDocument()
	v2 := doc.V2
	ref := v2.Tracks[0].Clips[0].AssetRef
	v1 := EditingTimelineV1{SchemaVersion: EditingTimelineSchemaV1, OutputProfile: EditingMVPVerticalOutputProfile, DurationMS: 3000, Tracks: []EditingTimelineTrack{{ID: "video-primary", Role: EditingTrackPrimaryVideo, Clips: []EditingTimelineClip{{ID: "clip-1", AssetRef: ref, TimelineEndMS: 3000, SourceOutMS: 3000}}}}}
	return EditingDocument{V1: &v1}
}
