package creative

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/shikanon/cookies/internal/platform/contract"
)

const EditingTimelineSchemaV2 = "editing-timeline/v2"

type EditingTimebaseV2 struct {
	FrameRateNum int `json:"frame_rate_num"`
	FrameRateDen int `json:"frame_rate_den"`
}
type EditingCanvasV2 struct {
	ProfileID  string              `json:"profile_id"`
	Width      int                 `json:"width"`
	Height     int                 `json:"height"`
	SampleRate int                 `json:"sample_rate"`
	Background EditingBackgroundV2 `json:"background"`
}
type EditingBackgroundV2 struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}
type EditingTimelineRangeV2 struct {
	StartFrame     int `json:"start_frame"`
	DurationFrames int `json:"duration_frames"`
}
type EditingSourceRangeV2 struct {
	InUS  int64 `json:"in_us"`
	OutUS int64 `json:"out_us"`
}
type EditingCropV2 struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
}
type EditingVisualTransformV2 struct {
	Fit       string        `json:"fit"`
	PositionX float64       `json:"position_x"`
	PositionY float64       `json:"position_y"`
	Scale     float64       `json:"scale"`
	Crop      EditingCropV2 `json:"crop"`
	Opacity   float64       `json:"opacity"`
}
type EditingOriginalAudioV2 struct {
	Enabled       bool    `json:"enabled"`
	GainDB        float64 `json:"gain_db"`
	FadeInFrames  int     `json:"fade_in_frames"`
	FadeOutFrames int     `json:"fade_out_frames"`
}
type EditingCaptionStyleRefV2 struct {
	StyleID string `json:"style_id"`
	Version int64  `json:"version"`
}
type EditingCaptionEmphasisV2 struct {
	StartRune int `json:"start_rune"`
	EndRune   int `json:"end_rune"`
}

type EditingClipV2 struct {
	ID            string                     `json:"id"`
	Kind          string                     `json:"kind"`
	AssetRef      *contract.AssetVersionRef  `json:"asset_ref,omitempty"`
	Timeline      EditingTimelineRangeV2     `json:"timeline"`
	Source        *EditingSourceRangeV2      `json:"source,omitempty"`
	Transform     *EditingVisualTransformV2  `json:"transform,omitempty"`
	OriginalAudio *EditingOriginalAudioV2    `json:"original_audio,omitempty"`
	Text          string                     `json:"text,omitempty"`
	StyleRef      *EditingCaptionStyleRefV2  `json:"style_ref,omitempty"`
	Emphasis      []EditingCaptionEmphasisV2 `json:"emphasis,omitempty"`
	GainDB        float64                    `json:"gain_db,omitempty"`
	FadeInFrames  int                        `json:"fade_in_frames,omitempty"`
	FadeOutFrames int                        `json:"fade_out_frames,omitempty"`
	Loop          bool                       `json:"loop,omitempty"`
}

type EditingTrackV2 struct {
	ID       string          `json:"id"`
	Kind     string          `json:"kind"`
	Role     string          `json:"role,omitempty"`
	ZIndex   int             `json:"z_index,omitempty"`
	Muted    bool            `json:"muted,omitempty"`
	Locked   bool            `json:"locked,omitempty"`
	Hidden   bool            `json:"hidden,omitempty"`
	Language string          `json:"language,omitempty"`
	Clips    []EditingClipV2 `json:"clips"`
}

type EditingTimelineV2 struct {
	SchemaVersion  string            `json:"schema_version"`
	Timebase       EditingTimebaseV2 `json:"timebase"`
	Canvas         EditingCanvasV2   `json:"canvas"`
	DurationFrames int               `json:"duration_frames"`
	Tracks         []EditingTrackV2  `json:"tracks"`
}

func (t EditingTrackV2) MarshalJSON() ([]byte, error) {
	switch t.Kind {
	case "visual":
		return json.Marshal(struct {
			ID     string          `json:"id"`
			Kind   string          `json:"kind"`
			Role   string          `json:"role"`
			ZIndex int             `json:"z_index"`
			Muted  bool            `json:"muted"`
			Locked bool            `json:"locked"`
			Hidden bool            `json:"hidden"`
			Clips  []EditingClipV2 `json:"clips"`
		}{t.ID, t.Kind, t.Role, t.ZIndex, t.Muted, t.Locked, t.Hidden, t.Clips})
	case "caption":
		return json.Marshal(struct {
			ID       string          `json:"id"`
			Kind     string          `json:"kind"`
			Language string          `json:"language"`
			Clips    []EditingClipV2 `json:"clips"`
		}{t.ID, t.Kind, t.Language, t.Clips})
	case "audio":
		return json.Marshal(struct {
			ID    string          `json:"id"`
			Kind  string          `json:"kind"`
			Role  string          `json:"role"`
			Muted bool            `json:"muted"`
			Clips []EditingClipV2 `json:"clips"`
		}{t.ID, t.Kind, t.Role, t.Muted, t.Clips})
	default:
		return nil, fmt.Errorf("editing track kind is invalid")
	}
}

func (c EditingClipV2) MarshalJSON() ([]byte, error) {
	switch c.Kind {
	case "video":
		return json.Marshal(struct {
			ID            string                    `json:"id"`
			Kind          string                    `json:"kind"`
			AssetRef      *contract.AssetVersionRef `json:"asset_ref"`
			Timeline      EditingTimelineRangeV2    `json:"timeline"`
			Source        *EditingSourceRangeV2     `json:"source"`
			Transform     *EditingVisualTransformV2 `json:"transform"`
			OriginalAudio *EditingOriginalAudioV2   `json:"original_audio"`
		}{c.ID, c.Kind, c.AssetRef, c.Timeline, c.Source, c.Transform, c.OriginalAudio})
	case "image":
		return json.Marshal(struct {
			ID        string                    `json:"id"`
			Kind      string                    `json:"kind"`
			AssetRef  *contract.AssetVersionRef `json:"asset_ref"`
			Timeline  EditingTimelineRangeV2    `json:"timeline"`
			Transform *EditingVisualTransformV2 `json:"transform"`
		}{c.ID, c.Kind, c.AssetRef, c.Timeline, c.Transform})
	case "caption":
		return json.Marshal(struct {
			ID       string                     `json:"id"`
			Kind     string                     `json:"kind"`
			Timeline EditingTimelineRangeV2     `json:"timeline"`
			Text     string                     `json:"text"`
			StyleRef *EditingCaptionStyleRefV2  `json:"style_ref"`
			Emphasis []EditingCaptionEmphasisV2 `json:"emphasis,omitempty"`
		}{c.ID, c.Kind, c.Timeline, c.Text, c.StyleRef, c.Emphasis})
	case "audio":
		return json.Marshal(struct {
			ID            string                    `json:"id"`
			Kind          string                    `json:"kind"`
			AssetRef      *contract.AssetVersionRef `json:"asset_ref"`
			Timeline      EditingTimelineRangeV2    `json:"timeline"`
			Source        *EditingSourceRangeV2     `json:"source"`
			GainDB        float64                   `json:"gain_db"`
			FadeInFrames  int                       `json:"fade_in_frames"`
			FadeOutFrames int                       `json:"fade_out_frames"`
			Loop          bool                      `json:"loop"`
		}{c.ID, c.Kind, c.AssetRef, c.Timeline, c.Source, c.GainDB, c.FadeInFrames, c.FadeOutFrames, c.Loop})
	default:
		return nil, fmt.Errorf("editing clip kind is invalid")
	}
}

func (d EditingTimelineV2) Validate() error {
	if d.SchemaVersion != EditingTimelineSchemaV2 || d.Timebase != (EditingTimebaseV2{FrameRateNum: 30, FrameRateDen: 1}) {
		return fmt.Errorf("editing timeline v2 schema and 30fps timebase are required")
	}
	if !validCanvasV2(d.Canvas) || d.DurationFrames < 1 || len(d.Tracks) == 0 {
		return fmt.Errorf("editing timeline v2 canvas, duration and tracks are required")
	}
	trackIDs, clipIDs, z := map[string]bool{}, map[string]bool{}, map[int]bool{}
	for _, track := range d.Tracks {
		if strings.TrimSpace(track.ID) == "" || trackIDs[track.ID] {
			return fmt.Errorf("editing timeline v2 track id must be unique and non-empty")
		}
		trackIDs[track.ID] = true
		if track.Kind == "visual" {
			if (track.Role != "primary" && track.Role != "overlay") || track.ZIndex < 0 || track.ZIndex > 2 || z[track.ZIndex] {
				return fmt.Errorf("visual track role or z_index is invalid")
			}
			z[track.ZIndex] = true
		} else if track.Kind == "caption" {
			if strings.TrimSpace(track.Language) == "" {
				return fmt.Errorf("caption language is required")
			}
		} else if track.Kind != "audio" || (track.Role != "voiceover" && track.Role != "music" && track.Role != "sfx") {
			return fmt.Errorf("editing track kind or role is invalid")
		}
		for _, clip := range track.Clips {
			if err := validateClipV2(track, clip, d.DurationFrames, clipIDs); err != nil {
				return err
			}
			clipIDs[clip.ID] = true
		}
	}
	return nil
}

func validCanvasV2(c EditingCanvasV2) bool {
	if c.SampleRate != 48000 || c.Background.Type != "color" || strings.TrimSpace(c.Background.Value) == "" {
		return false
	}
	switch c.ProfileID {
	case "vertical-720p-v1":
		return c.Width == 720 && c.Height == 1280
	case "landscape-720p-v1":
		return c.Width == 1280 && c.Height == 720
	case "square-1080-v1":
		return c.Width == 1080 && c.Height == 1080
	}
	return false
}
func validateClipV2(track EditingTrackV2, clip EditingClipV2, duration int, seen map[string]bool) error {
	if strings.TrimSpace(clip.ID) == "" || seen[clip.ID] || clip.Timeline.StartFrame < 0 || clip.Timeline.DurationFrames < 1 || clip.Timeline.StartFrame+clip.Timeline.DurationFrames > duration {
		return fmt.Errorf("editing clip id or timeline range is invalid")
	}
	if track.Kind == "caption" {
		if clip.Kind != "caption" || strings.TrimSpace(clip.Text) == "" || clip.StyleRef == nil || strings.TrimSpace(clip.StyleRef.StyleID) == "" || clip.StyleRef.Version < 1 {
			return fmt.Errorf("caption clip is invalid")
		}
		previousEnd, runeCount := 0, len([]rune(clip.Text))
		for _, span := range clip.Emphasis {
			if span.StartRune < previousEnd || span.EndRune <= span.StartRune || span.EndRune > runeCount {
				return fmt.Errorf("caption emphasis range is invalid")
			}
			previousEnd = span.EndRune
		}
		return nil
	}
	if clip.AssetRef == nil || clip.AssetRef.Validate() != nil {
		return fmt.Errorf("media clip asset is invalid")
	}
	if track.Kind == "visual" {
		if clip.Kind != "video" && clip.Kind != "image" || clip.Transform == nil {
			return fmt.Errorf("visual clip is invalid")
		}
		if !validVisualTransformV2(*clip.Transform) {
			return fmt.Errorf("visual clip transform is invalid")
		}
		if clip.Kind == "video" && (clip.Source == nil || clip.Source.OutUS <= clip.Source.InUS) {
			return fmt.Errorf("video source range is invalid")
		}
		return nil
	}
	if clip.Kind != "audio" || clip.Source == nil || clip.Source.OutUS <= clip.Source.InUS || clip.GainDB < -96 || clip.GainDB > 24 || clip.FadeInFrames < 0 || clip.FadeOutFrames < 0 || clip.FadeInFrames > clip.Timeline.DurationFrames || clip.FadeOutFrames > clip.Timeline.DurationFrames {
		return fmt.Errorf("audio clip is invalid")
	}
	return nil
}

func validVisualTransformV2(value EditingVisualTransformV2) bool {
	return (value.Fit == "contain" || value.Fit == "cover") &&
		value.PositionX >= 0 && value.PositionX <= 1 && value.PositionY >= 0 && value.PositionY <= 1 &&
		value.Scale > 0 && value.Scale <= 8 && value.Opacity >= 0 && value.Opacity <= 1 &&
		value.Crop.Left >= 0 && value.Crop.Top >= 0 && value.Crop.Right >= 0 && value.Crop.Bottom >= 0 &&
		value.Crop.Left+value.Crop.Right < 1 && value.Crop.Top+value.Crop.Bottom < 1
}

func defaultTransformV2() EditingVisualTransformV2 {
	return EditingVisualTransformV2{Fit: "contain", PositionX: 0.5, PositionY: 0.5, Scale: 1, Opacity: 1}
}
func msToFrameV2(ms int) int { return int(math.Round(float64(ms) * 30 / 1000)) }
