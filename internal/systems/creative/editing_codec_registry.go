package creative

import (
	"encoding/json"
	"fmt"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type EditingDocument struct {
	V1 *EditingTimelineV1
	V2 *EditingTimelineV2
}

func (d EditingDocument) SchemaVersion() string {
	if d.V1 != nil {
		return EditingTimelineSchemaV1
	}
	if d.V2 != nil {
		return EditingTimelineSchemaV2
	}
	return ""
}
func (d EditingDocument) Validate() error {
	if (d.V1 == nil) == (d.V2 == nil) {
		return fmt.Errorf("editing document must contain exactly one schema")
	}
	if d.V1 != nil {
		return d.V1.Validate()
	}
	return d.V2.Validate()
}

type EditingCodecRegistry struct{}

func DefaultEditingCodecRegistry() EditingCodecRegistry { return EditingCodecRegistry{} }

func (EditingCodecRegistry) Decode(payload []byte) (EditingDocument, error) {
	var envelope struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return EditingDocument{}, err
	}
	switch envelope.SchemaVersion {
	case EditingTimelineSchemaV1:
		var v EditingTimelineV1
		if err := json.Unmarshal(payload, &v); err != nil {
			return EditingDocument{}, err
		}
		d := EditingDocument{V1: &v}
		return d, d.Validate()
	case EditingTimelineSchemaV2:
		var v EditingTimelineV2
		if err := json.Unmarshal(payload, &v); err != nil {
			return EditingDocument{}, err
		}
		d := EditingDocument{V2: &v}
		return d, d.Validate()
	default:
		return EditingDocument{}, fmt.Errorf("unsupported editing timeline schema %q", envelope.SchemaVersion)
	}
}
func (EditingCodecRegistry) Encode(document EditingDocument) ([]byte, error) {
	if err := document.Validate(); err != nil {
		return nil, err
	}
	if document.V1 != nil {
		return json.Marshal(document.V1)
	}
	return json.Marshal(document.V2)
}
func (EditingCodecRegistry) MigrateToV2(document EditingDocument) (EditingDocument, error) {
	if err := document.Validate(); err != nil {
		return EditingDocument{}, err
	}
	if document.V2 != nil {
		copy := *document.V2
		return EditingDocument{V2: &copy}, nil
	}
	v1 := document.V1
	v2 := EditingTimelineV2{SchemaVersion: EditingTimelineSchemaV2, Timebase: EditingTimebaseV2{30, 1}, Canvas: EditingCanvasV2{ProfileID: "vertical-720p-v1", Width: 720, Height: 1280, SampleRate: 48000, Background: EditingBackgroundV2{Type: "color", Value: "#000000"}}, DurationFrames: msToFrameV2(v1.DurationMS)}
	for _, sourceTrack := range v1.Tracks {
		track := EditingTrackV2{ID: sourceTrack.ID, Clips: make([]EditingClipV2, 0, len(sourceTrack.Clips))}
		switch sourceTrack.Role {
		case EditingTrackPrimaryVideo:
			track.Kind, track.Role, track.ZIndex = "visual", "primary", 0
		case EditingTrackCaption:
			track.Kind, track.Language = "caption", "zh-CN"
		default:
			track.Kind, track.Role = "audio", string(sourceTrack.Role)
		}
		for _, sourceClip := range sourceTrack.Clips {
			clip := EditingClipV2{ID: sourceClip.ID, Timeline: EditingTimelineRangeV2{StartFrame: msToFrameV2(sourceClip.TimelineStartMS), DurationFrames: msToFrameV2(sourceClip.TimelineEndMS - sourceClip.TimelineStartMS)}}
			if sourceTrack.Role == EditingTrackCaption {
				clip.Kind, clip.Text = "caption", sourceClip.Text
				clip.StyleRef = &EditingCaptionStyleRefV2{StyleID: "legacy-default-v1", Version: 1}
			} else {
				ref := *sourceClip.AssetRef
				clip.AssetRef = &ref
				clip.Source = &EditingSourceRangeV2{InUS: int64(sourceClip.SourceInMS) * 1000, OutUS: int64(sourceClip.SourceOutMS) * 1000}
				if sourceTrack.Role == EditingTrackPrimaryVideo {
					transform := defaultTransformV2()
					original := EditingOriginalAudioV2{Enabled: true}
					clip.Kind, clip.Transform, clip.OriginalAudio = "video", &transform, &original
				} else {
					clip.Kind, clip.GainDB, clip.Loop = "audio", sourceClip.GainDB, sourceClip.Loop
				}
			}
			track.Clips = append(track.Clips, clip)
		}
		v2.Tracks = append(v2.Tracks, track)
	}
	visualTracks := 0
	for _, track := range v2.Tracks {
		if track.Kind == "visual" {
			visualTracks++
		}
	}
	for visualTracks < 3 {
		visualTracks++
		v2.Tracks = append(v2.Tracks, EditingTrackV2{ID: fmt.Sprintf("visual-overlay-%d", visualTracks-1), Kind: "visual", Role: "overlay", ZIndex: visualTracks - 1, Clips: []EditingClipV2{}})
	}
	if err := v2.Validate(); err != nil {
		return EditingDocument{}, err
	}
	return EditingDocument{V2: &v2}, nil
}

func editingDocumentHash(document EditingDocument) (string, error) {
	payload, err := DefaultEditingCodecRegistry().Encode(document)
	if err != nil {
		return "", err
	}
	var value any
	if err = json.Unmarshal(payload, &value); err != nil {
		return "", err
	}
	hash, err := contract.CanonicalJSONHash(value)
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}
