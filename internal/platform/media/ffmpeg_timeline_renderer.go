package media

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/assets"
)

type ProgressCommandRunner interface {
	Run(context.Context, string, []string, TimelineProgressFunc, int) error
}

type ExecProgressCommandRunner struct{}

func (ExecProgressCommandRunner) Run(ctx context.Context, executable string, args []string, report TimelineProgressFunc, totalMS int) error {
	command := exec.CommandContext(ctx, executable, args...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok || (key != "out_time_ms" && key != "out_time_us") {
			continue
		}
		micros, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil {
			continue
		}
		outMS := int(micros / 1000)
		percent := min(99, outMS*100/max(1, totalMS))
		if report != nil {
			if err := report(TimelineProgress{Percent: percent, OutTimeMS: outMS}); err != nil {
				_ = command.Process.Kill()
				_ = command.Wait()
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return err
	}
	if err := command.Wait(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if len(message) > 1200 {
			message = message[len(message)-1200:]
		}
		return fmt.Errorf("timeline command failed: %w: %s", err, message)
	}
	return nil
}

type FFmpegTimelineRenderer struct {
	FFmpegPath string
	WorkRoot   string
	Videos     VideoSource
	Visuals    VisualSource
	Audio      AudioSource
	Probe      assets.VideoMetadataProbe
	Runner     ProgressCommandRunner
	TempTTL    time.Duration
	// Deterministic disables CPU- and core-count-dependent FFmpeg paths for
	// portable golden tests. Production rendering keeps the optimized default.
	Deterministic bool
}

func (r FFmpegTimelineRenderer) Render(ctx context.Context, request TimelineRenderRequest, report TimelineProgressFunc) (CompositionOutput, error) {
	if err := request.Validate(); err != nil {
		return CompositionOutput{}, err
	}
	if strings.TrimSpace(r.FFmpegPath) == "" || (len(request.Visual) == 0 && r.Videos == nil) || (len(request.Visual) > 0 && r.Visuals == nil) || r.Audio == nil || r.Probe == nil {
		return CompositionOutput{}, fmt.Errorf("timeline rendering capability is unavailable")
	}
	root := strings.TrimSpace(r.WorkRoot)
	if root == "" {
		root = filepath.Join(".data", "ai-native-timeline-work")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return CompositionOutput{}, fmt.Errorf("create timeline work root: %w", err)
	}
	ttl := r.TempTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	_ = CleanupTimelineWorkRoot(root, time.Now().Add(-ttl))
	dir, err := os.MkdirTemp(root, "timeline-*")
	if err != nil {
		return CompositionOutput{}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	fail := func(err error) (CompositionOutput, error) { cleanup(); return CompositionOutput{}, err }
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-progress", "pipe:1", "-nostats"}
	if r.Deterministic {
		args = append(args, "-cpuflags", "0", "-filter_threads", "1", "-filter_complex_threads", "1")
	}
	video := append([]TimelineVideoClip(nil), request.Video...)
	sort.Slice(video, func(i, j int) bool { return video[i].StartMS < video[j].StartMS })
	request.Video = video
	for index, clip := range request.Video {
		path := filepath.Join(dir, fmt.Sprintf("video-%02d.mp4", index+1))
		_, reader, openErr := r.Videos.OpenVideo(ctx, request.OrganizationID, request.ProjectID, clip.Asset)
		if openErr != nil {
			return fail(openErr)
		}
		copyErr := copyToFile(path, io.LimitReader(reader, assets.MaxVideoBytes+1))
		closeErr := reader.Close()
		if copyErr != nil {
			return fail(copyErr)
		}
		if closeErr != nil {
			return fail(closeErr)
		}
		args = append(args, "-i", path)
	}
	if len(request.Visual) > 0 {
		request.Visual = SortedTimelineVisuals(request.Visual)
		audioAvailable := map[string]bool{}
		for index, clip := range request.Visual {
			path := filepath.Join(dir, fmt.Sprintf("visual-%02d.bin", index+1))
			version, reader, openErr := r.Visuals.OpenVisual(ctx, request.OrganizationID, request.ProjectID, clip.Asset)
			if openErr != nil {
				return fail(openErr)
			}
			copyErr := copyToFile(path, io.LimitReader(reader, assets.MaxVideoBytes+1))
			closeErr := reader.Close()
			if copyErr != nil {
				return fail(copyErr)
			}
			if closeErr != nil {
				return fail(closeErr)
			}
			if clip.Kind == "image" {
				args = append(args, "-loop", "1")
			}
			audioAvailable[clip.ID] = clip.Kind == "video" && strings.TrimSpace(version.AudioCodec) != ""
			args = append(args, "-i", path)
		}
		original := request.OriginalAudio[:0]
		for _, clip := range request.OriginalAudio {
			if audioAvailable[clip.VisualClipID] {
				original = append(original, clip)
			}
		}
		request.OriginalAudio = original
	}
	for index, clip := range request.Audio {
		path := filepath.Join(dir, fmt.Sprintf("audio-%02d.bin", index+1))
		_, reader, openErr := r.Audio.OpenAudio(ctx, request.OrganizationID, request.ProjectID, clip.Asset)
		if openErr != nil {
			return fail(openErr)
		}
		copyErr := copyToFile(path, io.LimitReader(reader, assets.MaxAudioBytes+1))
		closeErr := reader.Close()
		if copyErr != nil {
			return fail(copyErr)
		}
		if closeErr != nil {
			return fail(closeErr)
		}
		if clip.Loop {
			args = append(args, "-stream_loop", "-1")
		}
		args = append(args, "-i", path)
	}
	args = append(args, "-f", "lavfi", "-t", seconds(request.DurationMS), "-i", "anullsrc=channel_layout=stereo:sample_rate=48000")
	var subtitles []byte
	if len(request.CaptionStyles) > 0 {
		subtitles, _, err = BuildASSSubtitlesWithStyles(request.Captions, request.Width, request.Height, request.CaptionStyles)
	} else {
		subtitles, err = BuildASSSubtitles(request.Captions, request.Width, request.Height)
	}
	if err != nil {
		return fail(err)
	}
	subtitlePath := filepath.Join(dir, "subtitles.ass")
	if err := os.WriteFile(subtitlePath, subtitles, 0o600); err != nil {
		return fail(err)
	}
	graph, videoLabel, audioLabel := BuildTimelineFilter(request, subtitlePath)
	outputPath := filepath.Join(dir, "final.mp4")
	args = append(args, "-filter_complex", graph, "-map", videoLabel, "-map", audioLabel, "-c:v", "libx264", "-preset", "medium", "-crf", "20")
	if r.Deterministic {
		args = append(args, "-threads", "1", "-x264-params", "asm=0")
	}
	args = append(args, "-pix_fmt", "yuv420p", "-r", strconv.Itoa(request.FrameRate), "-c:a", "aac", "-b:a", "192k", "-ar", strconv.Itoa(request.SampleRate), "-ac", "2", "-t", seconds(request.DurationMS), "-movflags", "+faststart", outputPath)
	runner := r.Runner
	if runner == nil {
		runner = ExecProgressCommandRunner{}
	}
	if err := runner.Run(ctx, r.FFmpegPath, args, report, request.DurationMS); err != nil {
		return fail(err)
	}
	contents, err := os.ReadFile(outputPath)
	if err != nil {
		return fail(err)
	}
	metadata, err := r.Probe.Probe(ctx, contents)
	if err != nil {
		return fail(fmt.Errorf("validate timeline output: %w", err))
	}
	if math.Abs(float64(metadata.DurationMS-int64(request.DurationMS))) > 250 || metadata.WidthPixels != request.Width || metadata.HeightPixels != request.Height || strings.TrimSpace(metadata.AudioCodec) == "" || strings.TrimSpace(metadata.VideoCodec) == "" {
		return fail(fmt.Errorf("timeline output metadata does not match the frozen output profile"))
	}
	file, err := os.Open(outputPath)
	if err != nil {
		return fail(err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return fail(err)
	}
	return CompositionOutput{Content: &cleanupReadCloser{ReadCloser: file, cleanup: cleanup}, SizeBytes: info.Size(), Metadata: metadata}, nil
}

// CleanupTimelineWorkRoot removes only renderer-owned directories older than
// the cutoff. It never follows caller-provided names outside the work root.
func CleanupTimelineWorkRoot(root string, cutoff time.Time) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "timeline-") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
