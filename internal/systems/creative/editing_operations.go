package creative

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type EditOperationType string

const (
	OperationInsertAsset           EditOperationType = "insert_asset"
	OperationMoveClip              EditOperationType = "move_clip"
	OperationTrimClip              EditOperationType = "trim_clip"
	OperationSplitClip             EditOperationType = "split_clip"
	OperationDeleteClip            EditOperationType = "delete_clip"
	OperationUpdateVisualTransform EditOperationType = "update_visual_transform"
	OperationUpdateOriginalAudio   EditOperationType = "update_original_audio"
	OperationUpsertCaption         EditOperationType = "upsert_caption"
	OperationDeleteCaption         EditOperationType = "delete_caption"
	OperationSplitCaption          EditOperationType = "split_caption"
	OperationMergeCaptions         EditOperationType = "merge_captions"
	OperationUpdateAudioClip       EditOperationType = "update_audio_clip"
	OperationSetTrackMuted         EditOperationType = "set_track_muted"
	OperationSetTrackHidden        EditOperationType = "set_track_hidden"
	OperationSetTrackLocked        EditOperationType = "set_track_locked"
	OperationSetCanvasProfile      EditOperationType = "set_canvas_profile"
)

type EditOperation struct {
	OperationID         string                     `json:"operation_id"`
	Type                EditOperationType          `json:"type"`
	BaseTimelineVersion int64                      `json:"base_timeline_version"`
	Actor               string                     `json:"actor"`
	TrackID             string                     `json:"track_id,omitempty"`
	TargetTrackID       string                     `json:"target_track_id,omitempty"`
	ClipID              string                     `json:"clip_id,omitempty"`
	LeftID              string                     `json:"left_id,omitempty"`
	RightID             string                     `json:"right_id,omitempty"`
	AssetRef            *contract.AssetVersionRef  `json:"asset_ref,omitempty"`
	AssetKind           string                     `json:"asset_kind,omitempty"`
	AtFrame             int                        `json:"at_frame,omitempty"`
	DurationFrames      int                        `json:"duration_frames,omitempty"`
	Source              *EditingSourceRangeV2      `json:"source,omitempty"`
	Timeline            *EditingTimelineRangeV2    `json:"timeline,omitempty"`
	Transform           *EditingVisualTransformV2  `json:"transform,omitempty"`
	OriginalAudio       *EditingOriginalAudioV2    `json:"original_audio,omitempty"`
	Text                string                     `json:"text,omitempty"`
	RightText           string                     `json:"right_text,omitempty"`
	StyleRef            *EditingCaptionStyleRefV2  `json:"style_ref,omitempty"`
	Emphasis            []EditingCaptionEmphasisV2 `json:"emphasis,omitempty"`
	GainDB              *float64                   `json:"gain_db,omitempty"`
	FadeInFrames        *int                       `json:"fade_in_frames,omitempty"`
	FadeOutFrames       *int                       `json:"fade_out_frames,omitempty"`
	Loop                *bool                      `json:"loop,omitempty"`
	Muted               *bool                      `json:"muted,omitempty"`
	Hidden              *bool                      `json:"hidden,omitempty"`
	Locked              *bool                      `json:"locked,omitempty"`
	CanvasProfileID     string                     `json:"canvas_profile_id,omitempty"`
}

type EditOperationBatch struct {
	BatchID             string          `json:"batch_id"`
	BaseTimelineVersion int64           `json:"base_timeline_version"`
	Actor               string          `json:"actor"`
	Operations          []EditOperation `json:"operations"`
}
type EditOperationResult struct {
	Document          EditingDocument
	AppliedOperations []EditOperation
	InverseOperations []EditOperation
	Diagnostics       []string
	ChangeSummary     string
}

func ApplyEditOperations(document EditingDocument, batch EditOperationBatch) (EditOperationResult, error) {
	if document.V2 == nil || document.V1 != nil || document.Validate() != nil {
		return EditOperationResult{}, fmt.Errorf("operation batches require a valid v2 document")
	}
	if strings.TrimSpace(batch.BatchID) == "" || batch.BaseTimelineVersion < 1 || strings.TrimSpace(batch.Actor) == "" || len(batch.Operations) == 0 {
		return EditOperationResult{}, fmt.Errorf("operation batch identity, base version, actor and operations are required")
	}
	working, err := cloneTimelineV2(*document.V2)
	if err != nil {
		return EditOperationResult{}, err
	}
	seen := map[string]bool{}
	inverses := make([]EditOperation, 0, len(batch.Operations))
	counts := map[EditOperationType]int{}
	for _, op := range batch.Operations {
		if strings.TrimSpace(op.OperationID) == "" || seen[op.OperationID] || op.BaseTimelineVersion != batch.BaseTimelineVersion || op.Actor != batch.Actor {
			return EditOperationResult{}, fmt.Errorf("operation identity, actor or base version is invalid")
		}
		seen[op.OperationID] = true
		inverse, applyErr := applyEditOperation(&working, op)
		if applyErr != nil {
			return EditOperationResult{}, fmt.Errorf("operation %s: %w", op.OperationID, applyErr)
		}
		inverses = append([]EditOperation{inverse}, inverses...)
		counts[op.Type]++
	}
	normalizeOperationDocument(&working)
	if err := working.Validate(); err != nil {
		return EditOperationResult{}, err
	}
	return EditOperationResult{Document: EditingDocument{V2: &working}, AppliedOperations: append([]EditOperation(nil), batch.Operations...), InverseOperations: inverses, Diagnostics: []string{}, ChangeSummary: summarizeEditOperations(counts)}, nil
}

func applyEditOperation(document *EditingTimelineV2, op EditOperation) (EditOperation, error) {
	switch op.Type {
	case OperationInsertAsset:
		track := findTrackV2(document, op.TrackID)
		kind := op.AssetKind
		if kind == "" {
			kind = "video"
		}
		if track == nil || track.Locked || (track.Kind == "visual" && kind != "video" && kind != "image") || (track.Kind == "audio" && kind != "audio") || (track.Kind != "visual" && track.Kind != "audio") || op.AssetRef == nil || op.AssetRef.Validate() != nil || strings.TrimSpace(op.ClipID) == "" || findClipV2(document, op.ClipID) != nil || op.AtFrame < 0 || op.DurationFrames < 1 || ((kind == "video" || kind == "audio") && (op.Source == nil || op.Source.OutUS <= op.Source.InUS)) {
			return EditOperation{}, fmt.Errorf("insert asset fields are invalid")
		}
		if kind == "audio" {
			clip := EditingClipV2{ID: op.ClipID, Kind: kind, AssetRef: op.AssetRef, Timeline: EditingTimelineRangeV2{StartFrame: op.AtFrame, DurationFrames: op.DurationFrames}, Source: op.Source}
			if op.GainDB != nil {
				clip.GainDB = *op.GainDB
			}
			if op.FadeInFrames != nil {
				clip.FadeInFrames = *op.FadeInFrames
			}
			if op.FadeOutFrames != nil {
				clip.FadeOutFrames = *op.FadeOutFrames
			}
			if op.Loop != nil {
				clip.Loop = *op.Loop
			}
			track.Clips = append(track.Clips, clip)
			return inverseFor(op, OperationDeleteClip), nil
		}
		transform := defaultTransformV2()
		if op.Transform != nil {
			transform = *op.Transform
		}
		clip := EditingClipV2{ID: op.ClipID, Kind: kind, AssetRef: op.AssetRef, Timeline: EditingTimelineRangeV2{StartFrame: op.AtFrame, DurationFrames: op.DurationFrames}, Source: op.Source, Transform: &transform}
		if kind == "video" {
			clip.OriginalAudio = &EditingOriginalAudioV2{Enabled: true}
		}
		track.Clips = append(track.Clips, clip)
		if end := op.AtFrame + op.DurationFrames; end > document.DurationFrames {
			document.DurationFrames = end
		}
		return inverseFor(op, OperationDeleteClip), nil
	case OperationDeleteClip, OperationDeleteCaption:
		track, index, clip := locateClipV2(document, op.ClipID)
		if track == nil || track.Locked || op.Type == OperationDeleteCaption && track.Kind != "caption" {
			return EditOperation{}, fmt.Errorf("clip does not exist")
		}
		track.Clips = append(track.Clips[:index], track.Clips[index+1:]...)
		if track.Kind == "caption" {
			inverse := EditOperation{Type: OperationUpsertCaption, TrackID: track.ID, ClipID: clip.ID, Timeline: &clip.Timeline, Text: clip.Text, StyleRef: clip.StyleRef, Emphasis: clip.Emphasis}
			return inheritInverse(op, inverse), nil
		}
		inverse := EditOperation{Type: OperationInsertAsset, TrackID: track.ID, ClipID: clip.ID, AssetKind: clip.Kind, AssetRef: clip.AssetRef, AtFrame: clip.Timeline.StartFrame, DurationFrames: clip.Timeline.DurationFrames, Source: clip.Source, Transform: clip.Transform}
		return inheritInverse(op, inverse), nil
	case OperationUpsertCaption:
		track := findTrackV2(document, op.TrackID)
		if track == nil || track.Kind != "caption" || track.Locked || strings.TrimSpace(op.ClipID) == "" || strings.TrimSpace(op.Text) == "" || op.Timeline == nil || op.StyleRef == nil || op.StyleRef.Version < 1 {
			return EditOperation{}, fmt.Errorf("caption fields are invalid")
		}
		if existingTrack, _, existing := locateClipV2(document, op.ClipID); existing != nil {
			if existingTrack.ID != track.ID || existing.Kind != "caption" {
				return EditOperation{}, fmt.Errorf("caption id belongs to another track")
			}
			oldTimeline, oldText, oldStyle, oldEmphasis := existing.Timeline, existing.Text, existing.StyleRef, existing.Emphasis
			existing.Timeline, existing.Text, existing.StyleRef, existing.Emphasis = *op.Timeline, strings.TrimSpace(op.Text), op.StyleRef, append([]EditingCaptionEmphasisV2(nil), op.Emphasis...)
			return inheritInverse(op, EditOperation{Type: OperationUpsertCaption, TrackID: track.ID, ClipID: existing.ID, Timeline: &oldTimeline, Text: oldText, StyleRef: oldStyle, Emphasis: oldEmphasis}), nil
		}
		track.Clips = append(track.Clips, EditingClipV2{ID: op.ClipID, Kind: "caption", Timeline: *op.Timeline, Text: strings.TrimSpace(op.Text), StyleRef: op.StyleRef, Emphasis: append([]EditingCaptionEmphasisV2(nil), op.Emphasis...)})
		return inverseFor(op, OperationDeleteCaption), nil
	case OperationSplitCaption:
		track, index, clip := locateClipV2(document, op.ClipID)
		if track == nil || track.Kind != "caption" || track.Locked || clip == nil || op.AtFrame <= clip.Timeline.StartFrame || op.AtFrame >= clip.Timeline.StartFrame+clip.Timeline.DurationFrames || strings.TrimSpace(op.LeftID) == "" || strings.TrimSpace(op.RightID) == "" || op.LeftID == op.RightID || findClipV2(document, op.LeftID) != nil || findClipV2(document, op.RightID) != nil || strings.TrimSpace(op.Text) == "" || strings.TrimSpace(op.RightText) == "" {
			return EditOperation{}, fmt.Errorf("caption split fields are invalid")
		}
		original := *clip
		left := EditingClipV2{ID: op.LeftID, Kind: "caption", Timeline: EditingTimelineRangeV2{StartFrame: clip.Timeline.StartFrame, DurationFrames: op.AtFrame - clip.Timeline.StartFrame}, Text: strings.TrimSpace(op.Text), StyleRef: clip.StyleRef}
		right := EditingClipV2{ID: op.RightID, Kind: "caption", Timeline: EditingTimelineRangeV2{StartFrame: op.AtFrame, DurationFrames: clip.Timeline.StartFrame + clip.Timeline.DurationFrames - op.AtFrame}, Text: strings.TrimSpace(op.RightText), StyleRef: clip.StyleRef}
		track.Clips = append(append(track.Clips[:index:index], left, right), track.Clips[index+1:]...)
		return inheritInverse(op, EditOperation{Type: OperationMergeCaptions, LeftID: left.ID, RightID: right.ID, ClipID: original.ID, Text: original.Text}), nil
	case OperationMergeCaptions:
		leftTrack, leftIndex, left := locateClipV2(document, op.LeftID)
		rightTrack, rightIndex, right := locateClipV2(document, op.RightID)
		if leftTrack == nil || rightTrack == nil || leftTrack.ID != rightTrack.ID || leftTrack.Kind != "caption" || leftTrack.Locked || left == nil || right == nil || left.Timeline.StartFrame+left.Timeline.DurationFrames != right.Timeline.StartFrame || strings.TrimSpace(op.ClipID) == "" || strings.TrimSpace(op.Text) == "" || op.ClipID != left.ID && op.ClipID != right.ID && findClipV2(document, op.ClipID) != nil || left.StyleRef == nil || right.StyleRef == nil || *left.StyleRef != *right.StyleRef {
			return EditOperation{}, fmt.Errorf("caption merge fields are invalid")
		}
		leftCopy, rightCopy := *left, *right
		if leftIndex > rightIndex {
			leftIndex, rightIndex = rightIndex, leftIndex
		}
		leftTrack.Clips = append(leftTrack.Clips[:rightIndex], leftTrack.Clips[rightIndex+1:]...)
		leftTrack.Clips = append(leftTrack.Clips[:leftIndex], leftTrack.Clips[leftIndex+1:]...)
		leftTrack.Clips = append(leftTrack.Clips, EditingClipV2{ID: op.ClipID, Kind: "caption", Timeline: EditingTimelineRangeV2{StartFrame: leftCopy.Timeline.StartFrame, DurationFrames: rightCopy.Timeline.StartFrame + rightCopy.Timeline.DurationFrames - leftCopy.Timeline.StartFrame}, Text: strings.TrimSpace(op.Text), StyleRef: leftCopy.StyleRef})
		return inheritInverse(op, EditOperation{Type: OperationSplitCaption, ClipID: op.ClipID, LeftID: leftCopy.ID, RightID: rightCopy.ID, AtFrame: rightCopy.Timeline.StartFrame, Text: leftCopy.Text, RightText: rightCopy.Text}), nil
	case OperationMoveClip:
		source, index, clip := locateClipV2(document, op.ClipID)
		target := findTrackV2(document, op.TargetTrackID)
		if source == nil || target == nil || source.Kind != target.Kind || source.Locked || target.Locked || op.AtFrame < 0 {
			return EditOperation{}, fmt.Errorf("move source or target is invalid")
		}
		oldTrack, oldFrame := source.ID, clip.Timeline.StartFrame
		source.Clips = append(source.Clips[:index], source.Clips[index+1:]...)
		clip.Timeline.StartFrame = op.AtFrame
		target.Clips = append(target.Clips, *clip)
		return inheritInverse(op, EditOperation{Type: OperationMoveClip, ClipID: clip.ID, TargetTrackID: oldTrack, AtFrame: oldFrame}), nil
	case OperationSplitClip:
		track, index, clip := locateClipV2(document, op.ClipID)
		if track == nil || track.Kind != "audio" || track.Locked || clip == nil || clip.Source == nil || op.AtFrame <= clip.Timeline.StartFrame || op.AtFrame >= clip.Timeline.StartFrame+clip.Timeline.DurationFrames || strings.TrimSpace(op.LeftID) == "" || strings.TrimSpace(op.RightID) == "" || findClipV2(document, op.LeftID) != nil || findClipV2(document, op.RightID) != nil {
			return EditOperation{}, fmt.Errorf("audio split fields are invalid")
		}
		splitFrames := op.AtFrame - clip.Timeline.StartFrame
		sourceSplit := clip.Source.InUS + int64(splitFrames)*1_000_000/30
		left, right := *clip, *clip
		left.ID, left.Timeline.DurationFrames, left.Source = op.LeftID, splitFrames, &EditingSourceRangeV2{InUS: clip.Source.InUS, OutUS: sourceSplit}
		left.FadeOutFrames = 0
		right.ID, right.Timeline.StartFrame, right.Timeline.DurationFrames, right.Source = op.RightID, op.AtFrame, clip.Timeline.DurationFrames-splitFrames, &EditingSourceRangeV2{InUS: sourceSplit, OutUS: clip.Source.OutUS}
		right.FadeInFrames = 0
		track.Clips = append(append(track.Clips[:index:index], left, right), track.Clips[index+1:]...)
		return inheritInverse(op, EditOperation{Type: OperationDeleteClip, ClipID: right.ID}), nil
	case OperationTrimClip:
		track, _, clip := locateClipV2(document, op.ClipID)
		if track == nil || track.Locked || clip == nil || op.Timeline == nil || op.Timeline.StartFrame < 0 || op.Timeline.DurationFrames < 1 || (clip.Kind == "video" || clip.Kind == "audio") && (clip.Source == nil || op.Source == nil || op.Source.OutUS <= op.Source.InUS) || clip.Kind == "image" && op.Source != nil {
			return EditOperation{}, fmt.Errorf("trim target or range is invalid")
		}
		oldSource, oldTimeline := clip.Source, clip.Timeline
		clip.Source, clip.Timeline = op.Source, *op.Timeline
		return inheritInverse(op, EditOperation{Type: OperationTrimClip, ClipID: op.ClipID, Source: oldSource, Timeline: &oldTimeline}), nil
	case OperationUpdateVisualTransform:
		track, _, clip := locateClipV2(document, op.ClipID)
		if track == nil || track.Locked || clip == nil || clip.Transform == nil || op.Transform == nil {
			return EditOperation{}, fmt.Errorf("visual transform target is invalid")
		}
		old := *clip.Transform
		*clip.Transform = *op.Transform
		return inheritInverse(op, EditOperation{Type: OperationUpdateVisualTransform, ClipID: op.ClipID, Transform: &old}), nil
	case OperationUpdateOriginalAudio:
		clip := findClipV2(document, op.ClipID)
		if clip == nil || clip.OriginalAudio == nil || op.OriginalAudio == nil {
			return EditOperation{}, fmt.Errorf("original audio target is invalid")
		}
		old := *clip.OriginalAudio
		*clip.OriginalAudio = *op.OriginalAudio
		return inheritInverse(op, EditOperation{Type: OperationUpdateOriginalAudio, ClipID: op.ClipID, OriginalAudio: &old}), nil
	case OperationUpdateAudioClip:
		track, _, clip := locateClipV2(document, op.ClipID)
		if track == nil || track.Kind != "audio" || clip == nil || (op.GainDB == nil && op.FadeInFrames == nil && op.FadeOutFrames == nil && op.Loop == nil) {
			return EditOperation{}, fmt.Errorf("audio update target is invalid")
		}
		oldGain, oldFadeIn, oldFadeOut, oldLoop := clip.GainDB, clip.FadeInFrames, clip.FadeOutFrames, clip.Loop
		if op.GainDB != nil {
			clip.GainDB = *op.GainDB
		}
		if op.FadeInFrames != nil {
			clip.FadeInFrames = *op.FadeInFrames
		}
		if op.FadeOutFrames != nil {
			clip.FadeOutFrames = *op.FadeOutFrames
		}
		if op.Loop != nil {
			clip.Loop = *op.Loop
		}
		return inheritInverse(op, EditOperation{Type: OperationUpdateAudioClip, ClipID: op.ClipID, GainDB: &oldGain, FadeInFrames: &oldFadeIn, FadeOutFrames: &oldFadeOut, Loop: &oldLoop}), nil
	case OperationSetTrackMuted:
		track := findTrackV2(document, op.TrackID)
		if track == nil || op.Muted == nil {
			return EditOperation{}, fmt.Errorf("track mute target is invalid")
		}
		old := track.Muted
		track.Muted = *op.Muted
		return inheritInverse(op, EditOperation{Type: OperationSetTrackMuted, TrackID: op.TrackID, Muted: &old}), nil
	case OperationSetTrackHidden:
		track := findTrackV2(document, op.TrackID)
		if track == nil || track.Kind != "visual" || op.Hidden == nil {
			return EditOperation{}, fmt.Errorf("track hidden target is invalid")
		}
		old := track.Hidden
		track.Hidden = *op.Hidden
		return inheritInverse(op, EditOperation{Type: OperationSetTrackHidden, TrackID: op.TrackID, Hidden: &old}), nil
	case OperationSetTrackLocked:
		track := findTrackV2(document, op.TrackID)
		if track == nil || track.Kind != "visual" || op.Locked == nil {
			return EditOperation{}, fmt.Errorf("track lock target is invalid")
		}
		old := track.Locked
		track.Locked = *op.Locked
		return inheritInverse(op, EditOperation{Type: OperationSetTrackLocked, TrackID: op.TrackID, Locked: &old}), nil
	case OperationSetCanvasProfile:
		old := document.Canvas.ProfileID
		if err := setCanvasProfileV2(&document.Canvas, op.CanvasProfileID); err != nil {
			return EditOperation{}, err
		}
		return inheritInverse(op, EditOperation{Type: OperationSetCanvasProfile, CanvasProfileID: old}), nil
	default:
		return EditOperation{}, fmt.Errorf("operation type %s is not implemented", op.Type)
	}
}

func cloneTimelineV2(value EditingTimelineV2) (EditingTimelineV2, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return EditingTimelineV2{}, err
	}
	var cloned EditingTimelineV2
	err = json.Unmarshal(payload, &cloned)
	return cloned, err
}
func findTrackV2(document *EditingTimelineV2, id string) *EditingTrackV2 {
	for i := range document.Tracks {
		if document.Tracks[i].ID == id {
			return &document.Tracks[i]
		}
	}
	return nil
}
func findClipV2(document *EditingTimelineV2, id string) *EditingClipV2 {
	_, _, clip := locateClipV2(document, id)
	return clip
}
func locateClipV2(document *EditingTimelineV2, id string) (*EditingTrackV2, int, *EditingClipV2) {
	for ti := range document.Tracks {
		for ci := range document.Tracks[ti].Clips {
			if document.Tracks[ti].Clips[ci].ID == id {
				return &document.Tracks[ti], ci, &document.Tracks[ti].Clips[ci]
			}
		}
	}
	return nil, -1, nil
}
func inverseFor(op EditOperation, t EditOperationType) EditOperation {
	return inheritInverse(op, EditOperation{Type: t, ClipID: op.ClipID})
}
func inheritInverse(op EditOperation, inverse EditOperation) EditOperation {
	inverse.OperationID = "undo-" + op.OperationID
	inverse.Actor = op.Actor
	inverse.BaseTimelineVersion = op.BaseTimelineVersion + 1
	return inverse
}
func setCanvasProfileV2(canvas *EditingCanvasV2, id string) error {
	canvas.ProfileID = id
	switch id {
	case "vertical-720p-v1":
		canvas.Width, canvas.Height = 720, 1280
	case "landscape-720p-v1":
		canvas.Width, canvas.Height = 1280, 720
	case "square-1080-v1":
		canvas.Width, canvas.Height = 1080, 1080
	default:
		return fmt.Errorf("canvas profile is invalid")
	}
	return nil
}
func summarizeEditOperations(counts map[EditOperationType]int) string {
	if n := counts[OperationInsertAsset]; n > 0 && len(counts) == 1 {
		return fmt.Sprintf("加入 %d 个素材", n)
	}
	return fmt.Sprintf("应用 %d 项剪辑修改", sumOperationCounts(counts))
}
func sumOperationCounts(counts map[EditOperationType]int) int {
	n := 0
	for _, v := range counts {
		n += v
	}
	return n
}

func normalizeOperationDocument(document *EditingTimelineV2) {
	maxEnd := 0
	for index := range document.Tracks {
		track := &document.Tracks[index]
		sort.SliceStable(track.Clips, func(i, j int) bool {
			if track.Clips[i].Timeline.StartFrame == track.Clips[j].Timeline.StartFrame {
				return track.Clips[i].ID < track.Clips[j].ID
			}
			return track.Clips[i].Timeline.StartFrame < track.Clips[j].Timeline.StartFrame
		})
		for _, clip := range track.Clips {
			if end := clip.Timeline.StartFrame + clip.Timeline.DurationFrames; end > maxEnd {
				maxEnd = end
			}
		}
	}
	if maxEnd > 0 {
		document.DurationFrames = maxEnd
	}
}
