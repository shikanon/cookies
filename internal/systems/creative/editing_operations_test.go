package creative

import (
	"testing"

	"github.com/shikanon/cookies/internal/platform/contract"
)

func TestOperationBatchIsAtomicAndReplayProducesTheSameHash(t *testing.T) {
	document := operationTestDocument()
	ref := contract.AssetVersionRef{AssetID: "asset_2", Version: 1}
	batch := EditOperationBatch{BatchID: "batch_1", BaseTimelineVersion: 4, Actor: "user_1", Operations: []EditOperation{{OperationID: "op_1", Type: OperationInsertAsset, BaseTimelineVersion: 4, Actor: "user_1", TrackID: "visual-primary", ClipID: "clip-2", AssetRef: &ref, AtFrame: 90, DurationFrames: 30, Source: &EditingSourceRangeV2{InUS: 0, OutUS: 1_000_000}}}}
	first, err := ApplyEditOperations(document, batch)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ApplyEditOperations(document, batch)
	if err != nil {
		t.Fatal(err)
	}
	firstHash, _ := editingDocumentHash(first.Document)
	secondHash, _ := editingDocumentHash(second.Document)
	if firstHash != secondHash || len(first.InverseOperations) != 1 || first.ChangeSummary != "加入 1 个素材" {
		t.Fatalf("replay mismatch: %s/%s %#v", firstHash, secondHash, first)
	}

	bad := batch
	bad.Operations = append(bad.Operations, EditOperation{OperationID: "op_2", Type: OperationDeleteClip, BaseTimelineVersion: 4, Actor: "user_1", ClipID: "missing"})
	if _, err := ApplyEditOperations(document, bad); err == nil {
		t.Fatal("invalid operation must reject the entire batch")
	}
	originalHash, _ := editingDocumentHash(document)
	if after, _ := editingDocumentHash(document); after != originalHash {
		t.Fatal("failed batch mutated its input")
	}
}

func TestVisualOperationBatchSupportsImagesCrossTrackMovesAndCanvasProfiles(t *testing.T) {
	document := operationTestDocument()
	document.V2.Tracks = append(document.V2.Tracks,
		EditingTrackV2{ID: "visual-overlay-1", Kind: "visual", Role: "overlay", ZIndex: 1, Clips: []EditingClipV2{}},
		EditingTrackV2{ID: "visual-overlay-2", Kind: "visual", Role: "overlay", ZIndex: 2, Clips: []EditingClipV2{}},
	)
	imageRef := contract.AssetVersionRef{AssetID: "image_1", Version: 3}
	transform := EditingVisualTransformV2{
		Fit: "cover", PositionX: 0.25, PositionY: 0.75, Scale: 1.5,
		Crop: EditingCropV2{Left: 0.1, Top: 0.2, Right: 0.05, Bottom: 0.1}, Opacity: 0.65,
	}
	batch := EditOperationBatch{
		BatchID: "batch-c3-visual", BaseTimelineVersion: 7, Actor: "user_1",
		Operations: []EditOperation{
			{OperationID: "insert-image", Type: OperationInsertAsset, BaseTimelineVersion: 7, Actor: "user_1", TrackID: "visual-overlay-1", ClipID: "image-clip", AssetKind: "image", AssetRef: &imageRef, AtFrame: 15, DurationFrames: 60},
			{OperationID: "move-image", Type: OperationMoveClip, BaseTimelineVersion: 7, Actor: "user_1", ClipID: "image-clip", TargetTrackID: "visual-overlay-2", AtFrame: 30},
			{OperationID: "transform-image", Type: OperationUpdateVisualTransform, BaseTimelineVersion: 7, Actor: "user_1", ClipID: "image-clip", Transform: &transform},
			{OperationID: "square-canvas", Type: OperationSetCanvasProfile, BaseTimelineVersion: 7, Actor: "user_1", CanvasProfileID: "square-1080-v1"},
		},
	}

	result, err := ApplyEditOperations(document, batch)
	if err != nil {
		t.Fatal(err)
	}
	if result.Document.V2.Canvas.Width != 1080 || result.Document.V2.Canvas.Height != 1080 {
		t.Fatalf("canvas profile was not applied: %#v", result.Document.V2.Canvas)
	}
	if len(result.Document.V2.Tracks[1].Clips) != 0 || len(result.Document.V2.Tracks[2].Clips) != 1 {
		t.Fatalf("image was not moved across visual tracks: %#v", result.Document.V2.Tracks)
	}
	clip := result.Document.V2.Tracks[2].Clips[0]
	if clip.Kind != "image" || clip.Source != nil || clip.OriginalAudio != nil || clip.Timeline.StartFrame != 30 || clip.Transform == nil || *clip.Transform != transform {
		t.Fatalf("image clip did not retain C3 semantics: %#v", clip)
	}
	if len(result.InverseOperations) != len(batch.Operations) {
		t.Fatalf("expected an inverse for every operation, got %d", len(result.InverseOperations))
	}
}

func TestC4CaptionOperationsSupportUpsertSplitMergeAndDelete(t *testing.T) {
	document := operationTestDocument()
	document.V2.Tracks = append(document.V2.Tracks, EditingTrackV2{ID: "captions-zh", Kind: "caption", Language: "zh-CN", Clips: []EditingClipV2{}})
	style := EditingCaptionStyleRefV2{StyleID: "brand-default", Version: 1}
	base := int64(12)
	common := func(id string, operationType EditOperationType) EditOperation {
		return EditOperation{OperationID: id, Type: operationType, BaseTimelineVersion: base, Actor: "user_1"}
	}
	upsert := common("caption-upsert", OperationUpsertCaption)
	upsert.TrackID, upsert.ClipID, upsert.Timeline, upsert.Text, upsert.StyleRef = "captions-zh", "caption-1", &EditingTimelineRangeV2{StartFrame: 15, DurationFrames: 60}, "中文 English 123，。", &style
	result, err := ApplyEditOperations(document, EditOperationBatch{BatchID: "captions-1", BaseTimelineVersion: base, Actor: "user_1", Operations: []EditOperation{upsert}})
	if err != nil {
		t.Fatal(err)
	}
	caption := result.Document.V2.Tracks[1].Clips[0]
	if caption.Text != upsert.Text || result.InverseOperations[0].Type != OperationDeleteCaption {
		t.Fatalf("caption upsert or inverse is invalid: %#v / %#v", caption, result.InverseOperations)
	}

	split := common("caption-split", OperationSplitCaption)
	split.ClipID, split.LeftID, split.RightID, split.AtFrame = caption.ID, "caption-1-a", "caption-1-b", 45
	split.Text, split.RightText = "中文 English", "123，。"
	result, err = ApplyEditOperations(result.Document, EditOperationBatch{BatchID: "captions-2", BaseTimelineVersion: base, Actor: "user_1", Operations: []EditOperation{split}})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Document.V2.Tracks[1].Clips; len(got) != 2 || got[0].Timeline.DurationFrames != 30 || got[1].Timeline.StartFrame != 45 || got[1].Text != "123，。" {
		t.Fatalf("caption split is invalid: %#v", got)
	}

	merge := common("caption-merge", OperationMergeCaptions)
	merge.LeftID, merge.RightID, merge.ClipID, merge.Text = "caption-1-a", "caption-1-b", "caption-merged", "中文 English 123，。"
	result, err = ApplyEditOperations(result.Document, EditOperationBatch{BatchID: "captions-3", BaseTimelineVersion: base, Actor: "user_1", Operations: []EditOperation{merge}})
	if err != nil {
		t.Fatal(err)
	}
	got := result.Document.V2.Tracks[1].Clips
	if len(got) != 1 || got[0].ID != "caption-merged" || got[0].Timeline.StartFrame != 15 || got[0].Timeline.DurationFrames != 60 {
		t.Fatalf("caption merge is invalid: %#v", got)
	}

	deleteCaption := common("caption-delete", OperationDeleteCaption)
	deleteCaption.ClipID = "caption-merged"
	result, err = ApplyEditOperations(result.Document, EditOperationBatch{BatchID: "captions-4", BaseTimelineVersion: base, Actor: "user_1", Operations: []EditOperation{deleteCaption}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Document.V2.Tracks[1].Clips) != 0 || result.InverseOperations[0].Type != OperationUpsertCaption || result.InverseOperations[0].Text != merge.Text {
		t.Fatalf("caption delete inverse must restore the caption: %#v", result)
	}
}

func TestC5AudioOperationsSupportInsertUpdateMoveSplitMuteAndDelete(t *testing.T) {
	document := operationTestDocument()
	document.V2.Tracks = append(document.V2.Tracks,
		EditingTrackV2{ID: "audio-voiceover", Kind: "audio", Role: "voiceover", Clips: []EditingClipV2{}},
		EditingTrackV2{ID: "audio-music", Kind: "audio", Role: "music", Clips: []EditingClipV2{}},
		EditingTrackV2{ID: "audio-sfx", Kind: "audio", Role: "sfx", Clips: []EditingClipV2{}},
	)
	ref := contract.AssetVersionRef{AssetID: "audio_1", Version: 2}
	gain, fadeIn, fadeOut, loop, muted := -12.0, 15, 20, true, true
	base := int64(20)
	operations := []EditOperation{
		{OperationID: "audio-insert", Type: OperationInsertAsset, BaseTimelineVersion: base, Actor: "user_1", TrackID: "audio-music", ClipID: "music-1", AssetKind: "audio", AssetRef: &ref, AtFrame: 0, DurationFrames: 90, Source: &EditingSourceRangeV2{InUS: 0, OutUS: 3_000_000}},
		{OperationID: "audio-update", Type: OperationUpdateAudioClip, BaseTimelineVersion: base, Actor: "user_1", ClipID: "music-1", GainDB: &gain, FadeInFrames: &fadeIn, FadeOutFrames: &fadeOut, Loop: &loop},
		{OperationID: "audio-move", Type: OperationMoveClip, BaseTimelineVersion: base, Actor: "user_1", ClipID: "music-1", TargetTrackID: "audio-sfx", AtFrame: 15},
		{OperationID: "audio-split", Type: OperationSplitClip, BaseTimelineVersion: base, Actor: "user_1", ClipID: "music-1", LeftID: "music-1-a", RightID: "music-1-b", AtFrame: 45},
		{OperationID: "audio-mute", Type: OperationSetTrackMuted, BaseTimelineVersion: base, Actor: "user_1", TrackID: "audio-sfx", Muted: &muted},
	}
	result, err := ApplyEditOperations(document, EditOperationBatch{BatchID: "audio-c5", BaseTimelineVersion: base, Actor: "user_1", Operations: operations})
	if err != nil {
		t.Fatal(err)
	}
	clips := result.Document.V2.Tracks[3].Clips
	if len(clips) != 2 || clips[0].GainDB != -12 || clips[0].FadeInFrames != 15 || clips[0].FadeOutFrames != 0 || !clips[0].Loop || clips[0].Source.OutUS != 1_000_000 || clips[1].Source.InUS != 1_000_000 || !result.Document.V2.Tracks[3].Muted {
		t.Fatalf("audio operation result is invalid: %#v / %#v", clips, result.Document.V2.Tracks[3])
	}
}

func operationTestDocument() EditingDocument {
	ref := contract.AssetVersionRef{AssetID: "asset_1", Version: 1}
	transform := defaultTransformV2()
	original := EditingOriginalAudioV2{Enabled: true}
	v2 := EditingTimelineV2{SchemaVersion: EditingTimelineSchemaV2, Timebase: EditingTimebaseV2{30, 1}, Canvas: EditingCanvasV2{ProfileID: "vertical-720p-v1", Width: 720, Height: 1280, SampleRate: 48000, Background: EditingBackgroundV2{Type: "color", Value: "#000000"}}, DurationFrames: 120, Tracks: []EditingTrackV2{{ID: "visual-primary", Kind: "visual", Role: "primary", Clips: []EditingClipV2{{ID: "clip-1", Kind: "video", AssetRef: &ref, Timeline: EditingTimelineRangeV2{DurationFrames: 90}, Source: &EditingSourceRangeV2{OutUS: 3_000_000}, Transform: &transform, OriginalAudio: &original}}}}}
	return EditingDocument{V2: &v2}
}
