package media

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/shikanon/cookies/internal/platform/contract"
)

const TimelineRendererVersion = "ffmpeg-ai-ad-timeline/v2"

type TimelineAudioRole string

const (
	TimelineAudioVoiceover TimelineAudioRole = "voiceover"
	TimelineAudioMusic     TimelineAudioRole = "music"
	TimelineAudioSFX       TimelineAudioRole = "sfx"
)

type TimelineVideoClip struct {
	ID       string
	Asset    contract.AssetVersionRef
	StartMS  int
	EndMS    int
	SourceIn int
}

type TimelineVisualClip struct {
	ID         string
	Kind       string
	Asset      contract.AssetVersionRef
	StartMS    int
	EndMS      int
	SourceIn   int
	Fit        string
	PositionX  float64
	PositionY  float64
	Scale      float64
	CropLeft   float64
	CropTop    float64
	CropRight  float64
	CropBottom float64
	Opacity    float64
	ZIndex     int
}

type TimelineAudioClip struct {
	ID        string
	Role      TimelineAudioRole
	Asset     contract.AssetVersionRef
	StartMS   int
	EndMS     int
	SourceIn  int
	GainDB    float64
	FadeInMS  int
	FadeOutMS int
	Loop      bool
}

type TimelineOriginalAudioClip struct {
	VisualClipID string
	StartMS      int
	EndMS        int
	SourceIn     int
	GainDB       float64
	FadeInMS     int
	FadeOutMS    int
}

type TimelineCaption struct {
	StartMS  int
	EndMS    int
	Text     string
	StyleID  string
	Emphasis []TimelineCaptionEmphasis
}

type TimelineCaptionEmphasis struct {
	StartRune int
	EndRune   int
}

type TimelineCaptionStyle struct {
	ID               string
	Version          int64
	FontFamily       string
	FontSHA256       string
	FontSize         int
	PrimaryColor     string
	OutlineColor     string
	Outline          int
	Shadow           int
	Alignment        int
	MarginHorizontal int
	MarginVertical   int
	MaxCharsPerLine  int
	EmphasisColor    string
}

type TimelineRenderRequest struct {
	OrganizationID contract.OrganizationID
	ProjectID      contract.ProjectID
	DurationMS     int
	Width          int
	Height         int
	FrameRate      int
	SampleRate     int
	Video          []TimelineVideoClip
	Visual         []TimelineVisualClip
	Audio          []TimelineAudioClip
	OriginalAudio  []TimelineOriginalAudioClip
	Captions       []TimelineCaption
	CaptionStyles  []TimelineCaptionStyle
}

func (r TimelineRenderRequest) Validate() error {
	if r.OrganizationID == "" || r.ProjectID == "" || r.DurationMS < 1000 || !validTimelineCanvas(r.Width, r.Height) || r.FrameRate != 30 || r.SampleRate != 48000 || len(r.Video) == 0 && len(r.Visual) == 0 {
		return fmt.Errorf("timeline scope and Douyin output specification are required")
	}
	if len(r.Visual) > 0 {
		for index, clip := range r.Visual {
			crop := clip.CropLeft + clip.CropRight
			cropY := clip.CropTop + clip.CropBottom
			if strings.TrimSpace(clip.ID) == "" || (clip.Kind != "video" && clip.Kind != "image") || clip.Asset.Validate() != nil || clip.StartMS < 0 || clip.EndMS <= clip.StartMS || clip.EndMS > r.DurationMS || clip.SourceIn < 0 || (clip.Fit != "contain" && clip.Fit != "cover") || clip.PositionX < 0 || clip.PositionX > 1 || clip.PositionY < 0 || clip.PositionY > 1 || clip.Scale <= 0 || crop < 0 || crop >= 1 || cropY < 0 || cropY >= 1 || clip.Opacity < 0 || clip.Opacity > 1 || clip.ZIndex < 0 || clip.ZIndex > 2 {
				return fmt.Errorf("visual clip %d is invalid", index+1)
			}
		}
		return r.validateAudioAndCaptions()
	}
	video := append([]TimelineVideoClip(nil), r.Video...)
	sort.Slice(video, func(i, j int) bool { return video[i].StartMS < video[j].StartMS })
	cursor := 0
	for index, clip := range video {
		if strings.TrimSpace(clip.ID) == "" || clip.Asset.Validate() != nil || clip.StartMS != cursor || clip.EndMS <= clip.StartMS || clip.EndMS > r.DurationMS || clip.SourceIn < 0 {
			return fmt.Errorf("video timeline must be closed at clip %d", index+1)
		}
		cursor = clip.EndMS
	}
	if cursor != r.DurationMS {
		return fmt.Errorf("video timeline must be closed through the master duration")
	}
	return r.validateAudioAndCaptions()
}

func validTimelineCanvas(width, height int) bool {
	return width == 720 && height == 1280 || width == 1280 && height == 720 || width == 1080 && height == 1080
}

func (r TimelineRenderRequest) validateAudioAndCaptions() error {
	for index, clip := range r.Audio {
		if strings.TrimSpace(clip.ID) == "" || clip.Asset.Validate() != nil || clip.StartMS < 0 || clip.EndMS <= clip.StartMS || clip.EndMS > r.DurationMS || clip.SourceIn < 0 || clip.FadeInMS < 0 || clip.FadeOutMS < 0 || clip.FadeInMS > clip.EndMS-clip.StartMS || clip.FadeOutMS > clip.EndMS-clip.StartMS || (clip.Role != TimelineAudioVoiceover && clip.Role != TimelineAudioMusic && clip.Role != TimelineAudioSFX) {
			return fmt.Errorf("audio clip %d is invalid", index+1)
		}
	}
	for index, clip := range r.OriginalAudio {
		if strings.TrimSpace(clip.VisualClipID) == "" || clip.StartMS < 0 || clip.EndMS <= clip.StartMS || clip.EndMS > r.DurationMS || clip.SourceIn < 0 || clip.FadeInMS < 0 || clip.FadeOutMS < 0 || clip.FadeInMS > clip.EndMS-clip.StartMS || clip.FadeOutMS > clip.EndMS-clip.StartMS {
			return fmt.Errorf("original audio clip %d is invalid", index+1)
		}
	}
	for index, caption := range r.Captions {
		if caption.StartMS < 0 || caption.EndMS <= caption.StartMS || caption.EndMS > r.DurationMS || strings.TrimSpace(caption.Text) == "" || len([]rune(caption.Text)) > 80 {
			return fmt.Errorf("caption %d is invalid", index+1)
		}
	}
	styleIDs := make(map[string]struct{}, len(r.CaptionStyles))
	for _, style := range r.CaptionStyles {
		if err := validateCaptionStyle(style); err != nil {
			return err
		}
		if _, exists := styleIDs[style.ID]; exists {
			return fmt.Errorf("caption style id must be unique")
		}
		styleIDs[style.ID] = struct{}{}
	}
	if len(r.CaptionStyles) > 0 {
		for index, caption := range r.Captions {
			if _, exists := styleIDs[caption.StyleID]; !exists {
				return fmt.Errorf("caption %d style is unavailable", index+1)
			}
		}
	}
	return nil
}

type TimelineProgress struct {
	Percent   int
	OutTimeMS int
}

type TimelineProgressFunc func(TimelineProgress) error

type TimelineRenderer interface {
	Render(context.Context, TimelineRenderRequest, TimelineProgressFunc) (CompositionOutput, error)
}
