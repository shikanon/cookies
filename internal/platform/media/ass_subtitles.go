package media

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var assHexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func BuildASSSubtitles(captions []TimelineCaption, width, height int) ([]byte, error) {
	if !validTimelineCanvas(width, height) {
		return nil, fmt.Errorf("ASS subtitles require a supported timeline canvas")
	}
	fontSize := max(32, int(float64(min(width, height))*42.0/720.0))
	marginHorizontal := max(48, int(float64(width)*70.0/720.0))
	marginVertical := max(48, int(float64(height)*80.0/1280.0))
	var body strings.Builder
	body.WriteString(fmt.Sprintf("[Script Info]\nScriptType: v4.00+\nPlayResX: %d\nPlayResY: %d\nWrapStyle: 2\nScaledBorderAndShadow: yes\n\n", width, height))
	body.WriteString("[V4+ Styles]\nFormat: Name,Fontname,Fontsize,PrimaryColour,SecondaryColour,OutlineColour,BackColour,Bold,Italic,Underline,StrikeOut,ScaleX,ScaleY,Spacing,Angle,BorderStyle,Outline,Shadow,Alignment,MarginL,MarginR,MarginV,Encoding\n")
	body.WriteString(fmt.Sprintf("Style: Default,Noto Sans CJK SC,%d,&H00FFFFFF,&H000000FF,&H00101010,&H60000000,-1,0,0,0,100,100,0,0,1,3,1,2,%d,%d,%d,1\n\n", fontSize, marginHorizontal, marginHorizontal, marginVertical))
	body.WriteString("[Events]\nFormat: Layer,Start,End,Style,Name,MarginL,MarginR,MarginV,Effect,Text\n")
	style := TimelineCaptionStyle{PrimaryColor: "#FFFFFF", EmphasisColor: "#38BDF8", MaxCharsPerLine: 18}
	for _, caption := range captions {
		if caption.EndMS <= caption.StartMS || strings.TrimSpace(caption.Text) == "" {
			return nil, fmt.Errorf("caption interval and text are required")
		}
		text, err := styledASSText(caption.Text, caption.Emphasis, style)
		if err != nil {
			return nil, err
		}
		body.WriteString(fmt.Sprintf("Dialogue: 0,%s,%s,Default,,0,0,0,,%s\n", assTime(caption.StartMS), assTime(caption.EndMS), text))
	}
	return []byte(body.String()), nil
}

// BuildASSSubtitlesWithStyles is the deterministic C4 subtitle compiler. Styles
// are versioned inputs; a caller must resolve and authorize their font assets
// before invoking the renderer.
func BuildASSSubtitlesWithStyles(captions []TimelineCaption, width, height int, styles []TimelineCaptionStyle) ([]byte, []string, error) {
	if !validTimelineCanvas(width, height) {
		return nil, nil, fmt.Errorf("ASS subtitles require a supported timeline canvas")
	}
	styleByID := make(map[string]TimelineCaptionStyle, len(styles))
	for _, style := range styles {
		if err := validateCaptionStyle(style); err != nil {
			return nil, nil, err
		}
		if _, exists := styleByID[style.ID]; exists {
			return nil, nil, fmt.Errorf("caption style id must be unique")
		}
		styleByID[style.ID] = style
	}
	var body strings.Builder
	body.WriteString(fmt.Sprintf("[Script Info]\nScriptType: v4.00+\nPlayResX: %d\nPlayResY: %d\nWrapStyle: 2\nScaledBorderAndShadow: yes\n", width, height))
	for _, style := range styles {
		body.WriteString(fmt.Sprintf("; StyleVersion: %s@%d FontSHA256: %s\n", style.ID, style.Version, style.FontSHA256))
	}
	body.WriteString("\n[V4+ Styles]\nFormat: Name,Fontname,Fontsize,PrimaryColour,SecondaryColour,OutlineColour,BackColour,Bold,Italic,Underline,StrikeOut,ScaleX,ScaleY,Spacing,Angle,BorderStyle,Outline,Shadow,Alignment,MarginL,MarginR,MarginV,Encoding\n")
	for _, style := range styles {
		body.WriteString(fmt.Sprintf("Style: %s,%s,%d,%s,&H000000FF,%s,&H60000000,-1,0,0,0,100,100,0,0,1,%d,%d,%d,%d,%d,%d,1\n", style.ID, style.FontFamily, style.FontSize, assColor(style.PrimaryColor), assColor(style.OutlineColor), style.Outline, style.Shadow, style.Alignment, style.MarginHorizontal, style.MarginHorizontal, style.MarginVertical))
	}
	body.WriteString("\n[Events]\nFormat: Layer,Start,End,Style,Name,MarginL,MarginR,MarginV,Effect,Text\n")
	diagnostics := []string{}
	for _, caption := range captions {
		style, ok := styleByID[caption.StyleID]
		if caption.EndMS <= caption.StartMS || strings.TrimSpace(caption.Text) == "" || !ok {
			return nil, nil, fmt.Errorf("caption interval, text and style are required")
		}
		text, err := styledASSText(caption.Text, caption.Emphasis, style)
		if err != nil {
			return nil, nil, err
		}
		body.WriteString(fmt.Sprintf("Dialogue: 0,%s,%s,%s,,0,0,0,,%s\n", assTime(caption.StartMS), assTime(caption.EndMS), style.ID, text))
	}
	return []byte(body.String()), diagnostics, nil
}

func validateCaptionStyle(style TimelineCaptionStyle) error {
	if strings.TrimSpace(style.ID) == "" || style.Version < 1 || strings.TrimSpace(style.FontFamily) == "" || len(style.FontSHA256) != 64 || !assHexColor.MatchString(style.PrimaryColor) || !assHexColor.MatchString(style.OutlineColor) || !assHexColor.MatchString(style.EmphasisColor) || style.FontSize < 8 || style.Outline < 0 || style.Shadow < 0 || style.Alignment < 1 || style.Alignment > 9 || style.MarginHorizontal < 0 || style.MarginVertical < 0 || style.MaxCharsPerLine < 1 {
		return fmt.Errorf("caption style version is invalid")
	}
	return nil
}

func styledASSText(value string, emphasis []TimelineCaptionEmphasis, style TimelineCaptionStyle) (string, error) {
	runes := []rune(strings.TrimSpace(value))
	sort.Slice(emphasis, func(i, j int) bool { return emphasis[i].StartRune < emphasis[j].StartRune })
	previousEnd := 0
	for _, span := range emphasis {
		if span.StartRune < previousEnd || span.StartRune < 0 || span.EndRune <= span.StartRune || span.EndRune > len(runes) {
			return "", fmt.Errorf("caption emphasis range is invalid")
		}
		previousEnd = span.EndRune
	}
	var body strings.Builder
	spanIndex := 0
	lineRunes := 0
	for index, current := range runes {
		if current == '\r' {
			continue
		}
		if current == '\n' || lineRunes >= style.MaxCharsPerLine && current != ' ' {
			body.WriteString(`\N`)
			lineRunes = 0
			if current == '\n' {
				continue
			}
		}
		if spanIndex < len(emphasis) && index == emphasis[spanIndex].StartRune {
			body.WriteString(`{\c` + assColor(style.EmphasisColor) + `}`)
		}
		body.WriteString(escapeASSText(string(current)))
		lineRunes++
		if spanIndex < len(emphasis) && index+1 == emphasis[spanIndex].EndRune {
			body.WriteString(`{\c` + assColor(style.PrimaryColor) + `}`)
			spanIndex++
		}
	}
	return body.String(), nil
}

func assColor(value string) string {
	return "&H00" + strings.ToUpper(value[5:7]+value[3:5]+value[1:3]) + "&"
}

func assTime(ms int) string {
	hours := ms / 3600000
	minutes := ms / 60000 % 60
	seconds := ms / 1000 % 60
	centiseconds := ms / 10 % 100
	return fmt.Sprintf("%d:%02d:%02d.%02d", hours, minutes, seconds, centiseconds)
}

func escapeASSText(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "{", `\{`)
	value = strings.ReplaceAll(value, "}", `\}`)
	value = strings.ReplaceAll(value, "\r\n", `\N`)
	value = strings.ReplaceAll(value, "\n", `\N`)
	return value
}
