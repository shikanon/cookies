package creative

import (
	"fmt"
	"sort"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/media"
)

const EditingRenderIRSchemaV2 = "editing-render-ir/v2"

type EditingRenderIRInput struct {
	AssetRef contract.AssetVersionRef `json:"asset_ref"`
	Kind     string                   `json:"kind"`
}
type EditingRenderIRVisual struct {
	ClipID         string                   `json:"clip_id"`
	Kind           string                   `json:"kind"`
	AssetRef       contract.AssetVersionRef `json:"asset_ref"`
	StartFrame     int                      `json:"start_frame"`
	DurationFrames int                      `json:"duration_frames"`
	Source         *EditingSourceRangeV2    `json:"source,omitempty"`
	Transform      EditingVisualTransformV2 `json:"transform"`
	ZIndex         int                      `json:"z_index"`
}
type EditingRenderIRCaption struct {
	ClipID   string                     `json:"clip_id"`
	Range    EditingTimelineRangeV2     `json:"range"`
	Text     string                     `json:"text"`
	StyleRef EditingCaptionStyleRefV2   `json:"style_ref"`
	Emphasis []EditingCaptionEmphasisV2 `json:"emphasis,omitempty"`
}
type EditingRenderIRAudio struct {
	ClipID        string                   `json:"clip_id"`
	AssetRef      contract.AssetVersionRef `json:"asset_ref"`
	Role          string                   `json:"role"`
	Range         EditingTimelineRangeV2   `json:"range"`
	Source        EditingSourceRangeV2     `json:"source"`
	GainDB        float64                  `json:"gain_db"`
	FadeInFrames  int                      `json:"fade_in_frames"`
	FadeOutFrames int                      `json:"fade_out_frames"`
	Loop          bool                     `json:"loop"`
}
type EditingRenderIRV2 struct {
	SchemaVersion   string                   `json:"schema_version"`
	Inputs          []EditingRenderIRInput   `json:"inputs"`
	VisualLayers    []EditingRenderIRVisual  `json:"visual_layers"`
	Captions        []EditingRenderIRCaption `json:"captions"`
	AudioInputs     []EditingRenderIRAudio   `json:"audio_inputs"`
	Canvas          EditingCanvasV2          `json:"canvas"`
	DurationFrames  int                      `json:"duration_frames"`
	CompilerVersion string                   `json:"compiler_version"`
}
type CompiledEditingDocument struct {
	CompilerVersion string
	IR              EditingRenderIRV2
	MediaRequest    media.TimelineRenderRequest
}
type EditingCompilerRegistry struct{}

func DefaultEditingCompilerRegistry() EditingCompilerRegistry { return EditingCompilerRegistry{} }
func (EditingCompilerRegistry) Compile(document EditingDocument, org contract.OrganizationID, project contract.ProjectID) (CompiledEditingDocument, error) {
	if err := document.Validate(); err != nil {
		return CompiledEditingDocument{}, err
	}
	if document.V1 != nil {
		request, err := CompileEditingTimelineV1(*document.V1, org, project)
		return CompiledEditingDocument{CompilerVersion: "editing-v1", MediaRequest: request}, err
	}
	return compileEditingTimelineV2Skeleton(*document.V2, org, project)
}
func compileEditingTimelineV2Skeleton(timeline EditingTimelineV2, org contract.OrganizationID, project contract.ProjectID) (CompiledEditingDocument, error) {
	if org == "" || project == "" {
		return CompiledEditingDocument{}, fmt.Errorf("editing compiler scope is required")
	}
	if err := timeline.Validate(); err != nil {
		return CompiledEditingDocument{}, err
	}
	ir := EditingRenderIRV2{SchemaVersion: EditingRenderIRSchemaV2, Canvas: timeline.Canvas, DurationFrames: timeline.DurationFrames, CompilerVersion: "editing-v2-audio-c5"}
	request := media.TimelineRenderRequest{OrganizationID: org, ProjectID: project, DurationMS: frameToMSV2(timeline.DurationFrames), Width: timeline.Canvas.Width, Height: timeline.Canvas.Height, FrameRate: 30, SampleRate: timeline.Canvas.SampleRate}
	seen := map[contract.AssetVersionRef]bool{}
	seenCaptionStyles := map[EditingCaptionStyleRefV2]bool{}
	for _, track := range timeline.Tracks {
		if track.Kind == "visual" && track.Hidden {
			continue
		}
		if track.Kind == "audio" && track.Muted {
			continue
		}
		for _, clip := range track.Clips {
			if clip.AssetRef != nil && !seen[*clip.AssetRef] {
				ir.Inputs = append(ir.Inputs, EditingRenderIRInput{AssetRef: *clip.AssetRef, Kind: clip.Kind})
				seen[*clip.AssetRef] = true
			}
			switch track.Kind {
			case "visual":
				ir.VisualLayers = append(ir.VisualLayers, EditingRenderIRVisual{ClipID: clip.ID, Kind: clip.Kind, AssetRef: *clip.AssetRef, StartFrame: clip.Timeline.StartFrame, DurationFrames: clip.Timeline.DurationFrames, Source: clip.Source, Transform: *clip.Transform, ZIndex: track.ZIndex})
				sourceIn := 0
				if clip.Source != nil {
					sourceIn = int(clip.Source.InUS / 1000)
				}
				request.Visual = append(request.Visual, media.TimelineVisualClip{ID: clip.ID, Kind: clip.Kind, Asset: *clip.AssetRef, StartMS: frameToMSV2(clip.Timeline.StartFrame), EndMS: frameToMSV2(clip.Timeline.StartFrame + clip.Timeline.DurationFrames), SourceIn: sourceIn, Fit: clip.Transform.Fit, PositionX: clip.Transform.PositionX, PositionY: clip.Transform.PositionY, Scale: clip.Transform.Scale, CropLeft: clip.Transform.Crop.Left, CropTop: clip.Transform.Crop.Top, CropRight: clip.Transform.Crop.Right, CropBottom: clip.Transform.Crop.Bottom, Opacity: clip.Transform.Opacity, ZIndex: track.ZIndex})
				if clip.Kind == "video" && clip.OriginalAudio != nil && clip.OriginalAudio.Enabled && !track.Muted {
					request.OriginalAudio = append(request.OriginalAudio, media.TimelineOriginalAudioClip{VisualClipID: clip.ID, StartMS: frameToMSV2(clip.Timeline.StartFrame), EndMS: frameToMSV2(clip.Timeline.StartFrame + clip.Timeline.DurationFrames), SourceIn: sourceIn, GainDB: clip.OriginalAudio.GainDB, FadeInMS: frameToMSV2(clip.OriginalAudio.FadeInFrames), FadeOutMS: frameToMSV2(clip.OriginalAudio.FadeOutFrames)})
				}
			case "caption":
				ir.Captions = append(ir.Captions, EditingRenderIRCaption{ClipID: clip.ID, Range: clip.Timeline, Text: clip.Text, StyleRef: *clip.StyleRef, Emphasis: clip.Emphasis})
				emphasis := make([]media.TimelineCaptionEmphasis, 0, len(clip.Emphasis))
				for _, span := range clip.Emphasis {
					emphasis = append(emphasis, media.TimelineCaptionEmphasis{StartRune: span.StartRune, EndRune: span.EndRune})
				}
				request.Captions = append(request.Captions, media.TimelineCaption{StartMS: frameToMSV2(clip.Timeline.StartFrame), EndMS: frameToMSV2(clip.Timeline.StartFrame + clip.Timeline.DurationFrames), Text: clip.Text, StyleID: clip.StyleRef.StyleID, Emphasis: emphasis})
				if !seenCaptionStyles[*clip.StyleRef] {
					style, err := resolveEditingCaptionStyle(*clip.StyleRef)
					if err != nil {
						return CompiledEditingDocument{}, err
					}
					request.CaptionStyles = append(request.CaptionStyles, style)
					seenCaptionStyles[*clip.StyleRef] = true
				}
			case "audio":
				ir.AudioInputs = append(ir.AudioInputs, EditingRenderIRAudio{ClipID: clip.ID, AssetRef: *clip.AssetRef, Role: track.Role, Range: clip.Timeline, Source: *clip.Source, GainDB: clip.GainDB, FadeInFrames: clip.FadeInFrames, FadeOutFrames: clip.FadeOutFrames, Loop: clip.Loop})
				request.Audio = append(request.Audio, media.TimelineAudioClip{ID: clip.ID, Role: mediaRoleForV2(track.Role), Asset: *clip.AssetRef, StartMS: frameToMSV2(clip.Timeline.StartFrame), EndMS: frameToMSV2(clip.Timeline.StartFrame + clip.Timeline.DurationFrames), SourceIn: int(clip.Source.InUS / 1000), GainDB: clip.GainDB, FadeInMS: frameToMSV2(clip.FadeInFrames), FadeOutMS: frameToMSV2(clip.FadeOutFrames), Loop: clip.Loop})
			}
		}
	}
	sort.SliceStable(ir.VisualLayers, func(i, j int) bool {
		if ir.VisualLayers[i].ZIndex != ir.VisualLayers[j].ZIndex {
			return ir.VisualLayers[i].ZIndex < ir.VisualLayers[j].ZIndex
		}
		if ir.VisualLayers[i].StartFrame != ir.VisualLayers[j].StartFrame {
			return ir.VisualLayers[i].StartFrame < ir.VisualLayers[j].StartFrame
		}
		return ir.VisualLayers[i].ClipID < ir.VisualLayers[j].ClipID
	})
	request.Visual = media.SortedTimelineVisuals(request.Visual)
	if err := request.Validate(); err != nil {
		return CompiledEditingDocument{}, err
	}
	return CompiledEditingDocument{CompilerVersion: "editing-v2-audio-c5", IR: ir, MediaRequest: request}, nil
}
func frameToMSV2(frame int) int { return (frame*1000 + 15) / 30 }
func mediaRoleForV2(role string) media.TimelineAudioRole {
	switch role {
	case "voiceover":
		return media.TimelineAudioVoiceover
	case "music":
		return media.TimelineAudioMusic
	default:
		return media.TimelineAudioSFX
	}
}

func resolveEditingCaptionStyle(ref EditingCaptionStyleRefV2) (media.TimelineCaptionStyle, error) {
	if ref.Version != 1 || ref.StyleID != "brand-default" && ref.StyleID != "legacy-default-v1" {
		return media.TimelineCaptionStyle{}, fmt.Errorf("caption style %s@%d is unavailable", ref.StyleID, ref.Version)
	}
	return media.TimelineCaptionStyle{
		ID: ref.StyleID, Version: ref.Version, FontFamily: "Noto Sans CJK SC",
		FontSHA256: "b76b0433203017ca80401b2ee0dd69350349871c4b19d504c34dbdd80541690a",
		FontSize:   44, PrimaryColor: "#FFFFFF", OutlineColor: "#101010", Outline: 3, Shadow: 1,
		Alignment: 2, MarginHorizontal: 70, MarginVertical: 80, MaxCharsPerLine: 18, EmphasisColor: "#38BDF8",
	}, nil
}
