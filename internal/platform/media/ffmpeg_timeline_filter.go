package media

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
)

func BuildTimelineFilter(request TimelineRenderRequest, subtitlePath string) (string, string, string) {
	parts := make([]string, 0, len(request.Video)+len(request.Visual)+len(request.Audio)+16)
	visualInputCount := len(request.Video)
	if len(request.Visual) > 0 {
		visual := SortedTimelineVisuals(request.Visual)
		visualInputCount = len(visual)
		parts = append(parts, fmt.Sprintf("color=c=#000000:s=%dx%d:r=%d:d=%s[base0]", request.Width, request.Height, request.FrameRate, seconds(request.DurationMS)))
		base := "base0"
		for index, clip := range visual {
			duration := clip.EndMS - clip.StartMS
			filters := []string{}
			if clip.Kind == "video" {
				filters = append(filters, "trim=start="+seconds(clip.SourceIn)+":duration="+seconds(duration))
			} else {
				filters = append(filters, "trim=duration="+seconds(duration))
			}
			filters = append(filters, "setpts=PTS-STARTPTS+"+seconds(clip.StartMS)+"/TB")
			if clip.CropLeft+clip.CropRight+clip.CropTop+clip.CropBottom > 0 {
				filters = append(filters, fmt.Sprintf("crop=iw*%.6f:ih*%.6f:iw*%.6f:ih*%.6f", 1-clip.CropLeft-clip.CropRight, 1-clip.CropTop-clip.CropBottom, clip.CropLeft, clip.CropTop))
			}
			fit := "decrease"
			if clip.Fit == "cover" {
				fit = "increase"
			}
			filters = append(filters, fmt.Sprintf("scale=%d*%.6f:%d*%.6f:force_original_aspect_ratio=%s", request.Width, clip.Scale, request.Height, clip.Scale, fit), fmt.Sprintf("fps=%d", request.FrameRate), "format=rgba")
			if clip.Opacity < 0.999999 {
				filters = append(filters, fmt.Sprintf("colorchannelmixer=aa=%.3f", clip.Opacity))
			}
			layer := fmt.Sprintf("layer%d", index)
			parts = append(parts, fmt.Sprintf("[%d:v]%s[%s]", index, strings.Join(filters, ","), layer))
			next := fmt.Sprintf("base%d", index+1)
			parts = append(parts, fmt.Sprintf("[%s][%s]overlay=x='(W-w)*%.6f':y='(H-h)*%.6f':eof_action=pass:shortest=0:enable='between(t,%s,%s)'[%s]", base, layer, clip.PositionX, clip.PositionY, seconds(clip.StartMS), seconds(clip.EndMS), next))
			base = next
		}
		parts = append(parts, fmt.Sprintf("[%s]format=yuv420p[visual]", base))
	} else {
		video := append([]TimelineVideoClip(nil), request.Video...)
		sort.Slice(video, func(i, j int) bool { return video[i].StartMS < video[j].StartMS })
		videoInputs := make([]string, 0, len(video))
		for index, clip := range video {
			duration := clip.EndMS - clip.StartMS
			label := fmt.Sprintf("v%d", index)
			parts = append(parts, fmt.Sprintf("[%d:v]trim=start=%s:duration=%s,setpts=PTS-STARTPTS,scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,fps=%d,format=yuv420p[%s]", index, seconds(clip.SourceIn), seconds(duration), request.Width, request.Height, request.Width, request.Height, request.FrameRate, label))
			videoInputs = append(videoInputs, "["+label+"]")
		}
		parts = append(parts, strings.Join(videoInputs, "")+fmt.Sprintf("concat=n=%d:v=1:a=0[visual]", len(videoInputs)))
	}
	escapedSubtitle := strings.ReplaceAll(filepath.ToSlash(subtitlePath), ":", `\:`)
	escapedSubtitle = strings.ReplaceAll(escapedSubtitle, "'", `\'`)
	parts = append(parts, fmt.Sprintf("[visual]subtitles='%s'[videoout]", escapedSubtitle))

	groups := map[TimelineAudioRole][]string{}
	originalInputs := []string{}
	if len(request.Visual) > 0 {
		visual := SortedTimelineVisuals(request.Visual)
		visualIndexes := make(map[string]int, len(visual))
		for index, clip := range visual {
			visualIndexes[clip.ID] = index
		}
		for index, clip := range request.OriginalAudio {
			inputIndex, ok := visualIndexes[clip.VisualClipID]
			if !ok {
				continue
			}
			label := fmt.Sprintf("original%d", index)
			duration := clip.EndMS - clip.StartMS
			filters := audioClipFilters(clip.SourceIn, duration, clip.StartMS, clip.GainDB, clip.FadeInMS, clip.FadeOutMS, false)
			parts = append(parts, fmt.Sprintf("[%d:a]%s[%s]", inputIndex, strings.Join(filters, ","), label))
			originalInputs = append(originalInputs, "["+label+"]")
		}
	}
	audioOffset := visualInputCount
	for index, clip := range request.Audio {
		label := fmt.Sprintf("a%d", index)
		duration := clip.EndMS - clip.StartMS
		filters := audioClipFilters(clip.SourceIn, duration, clip.StartMS, clip.GainDB, clip.FadeInMS, clip.FadeOutMS, clip.Loop)
		parts = append(parts, fmt.Sprintf("[%d:a]%s[%s]", audioOffset+index, strings.Join(filters, ","), label))
		groups[clip.Role] = append(groups[clip.Role], "["+label+"]")
	}
	bus := func(role TimelineAudioRole, label string) string {
		inputs := groups[role]
		if len(inputs) == 0 {
			return ""
		}
		parts = append(parts, strings.Join(inputs, "")+fmt.Sprintf("amix=inputs=%d:normalize=0[%s]", len(inputs), label))
		return "[" + label + "]"
	}
	voice, music, sfx := bus(TimelineAudioVoiceover, "voicebus"), bus(TimelineAudioMusic, "musicbus"), bus(TimelineAudioSFX, "sfxbus")
	if voice != "" && music != "" {
		parts = append(parts, music+voice+"sidechaincompress=threshold=0.03:ratio=8:attack=20:release=300[duckedmusic]")
		music = "[duckedmusic]"
	}
	parts = append(parts, fmt.Sprintf("[%d:a]atrim=duration=%s[silence]", visualInputCount+len(request.Audio), seconds(request.DurationMS)))
	finalInputs := []string{"[silence]"}
	finalInputs = append(finalInputs, originalInputs...)
	for _, input := range []string{voice, music, sfx} {
		if input != "" {
			finalInputs = append(finalInputs, input)
		}
	}
	parts = append(parts, strings.Join(finalInputs, "")+fmt.Sprintf("amix=inputs=%d:normalize=0,loudnorm=I=-16:TP=-1.5:LRA=11,alimiter=limit=0.95,atrim=duration=%s[audioout]", len(finalInputs), seconds(request.DurationMS)))
	return strings.Join(parts, ";"), "[videoout]", "[audioout]"
}

func audioClipFilters(sourceIn, duration, start int, gainDB float64, fadeIn, fadeOut int, loop bool) []string {
	filters := []string{fmt.Sprintf("atrim=start=%s:duration=%s", seconds(sourceIn), seconds(duration)), "asetpts=PTS-STARTPTS"}
	if loop {
		filters = []string{fmt.Sprintf("atrim=start=%s,asetpts=PTS-STARTPTS,aloop=loop=-1:size=2147483647,atrim=duration=%s", seconds(sourceIn), seconds(duration)), "asetpts=PTS-STARTPTS"}
	}
	if math.Abs(gainDB) > 0.001 {
		filters = append(filters, fmt.Sprintf("volume=%.2fdB", gainDB))
	}
	if fadeIn > 0 {
		filters = append(filters, fmt.Sprintf("afade=t=in:st=0:d=%s", seconds(fadeIn)))
	}
	if fadeOut > 0 {
		filters = append(filters, fmt.Sprintf("afade=t=out:st=%s:d=%s", seconds(duration-fadeOut), seconds(fadeOut)))
	}
	return append(filters, fmt.Sprintf("adelay=%d|%d", start, start))
}

func SortedTimelineVisuals(input []TimelineVisualClip) []TimelineVisualClip {
	visual := append([]TimelineVisualClip(nil), input...)
	sort.SliceStable(visual, func(i, j int) bool {
		if visual[i].ZIndex != visual[j].ZIndex {
			return visual[i].ZIndex < visual[j].ZIndex
		}
		if visual[i].StartMS != visual[j].StartMS {
			return visual[i].StartMS < visual[j].StartMS
		}
		return visual[i].ID < visual[j].ID
	})
	return visual
}
