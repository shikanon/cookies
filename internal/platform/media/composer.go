// Package media contains infrastructure-only media processing. It never owns
// Creative task state or Asset records; callers provide immutable asset
// references and persist the returned bytes through the Assets boundary.
package media

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/contract"
)

type VideoSource interface {
	OpenVideo(context.Context, contract.OrganizationID, contract.ProjectID, contract.AssetVersionRef) (assets.AssetVersion, io.ReadCloser, error)
}

// VisualSource opens an immutable video or still-image AssetVersion for the
// multi-layer timeline renderer. The request's clip kind remains authoritative.
type VisualSource interface {
	OpenVisual(context.Context, contract.OrganizationID, contract.ProjectID, contract.AssetVersionRef) (assets.AssetVersion, io.ReadCloser, error)
}

type PreRollCompositionRequest struct {
	OrganizationID contract.OrganizationID
	ProjectID      contract.ProjectID
	PreRollVideo   contract.AssetVersionRef
	MainVideo      contract.AssetVersionRef
}

func (r PreRollCompositionRequest) Validate() error {
	if r.OrganizationID == "" || r.ProjectID == "" {
		return fmt.Errorf("organization_id and project_id are required")
	}
	if err := r.PreRollVideo.Validate(); err != nil {
		return fmt.Errorf("pre-roll asset: %w", err)
	}
	if err := r.MainVideo.Validate(); err != nil {
		return fmt.Errorf("main video asset: %w", err)
	}
	return nil
}

// CompositionOutput owns a temporary output file. The caller must Close it
// after streaming the bytes into Assets; Close removes all working files.
type CompositionOutput struct {
	Content   io.ReadCloser
	SizeBytes int64
	Metadata  assets.VideoMetadata
}

type VideoComposer interface {
	ComposePreRoll(context.Context, PreRollCompositionRequest) (CompositionOutput, error)
}

type SegmentComposition struct {
	Asset           contract.AssetVersionRef
	DurationSeconds int
}

type SegmentCompositionRequest struct {
	OrganizationID contract.OrganizationID
	ProjectID      contract.ProjectID
	Segments       []SegmentComposition
}

func (r SegmentCompositionRequest) Validate() error {
	if r.OrganizationID == "" || r.ProjectID == "" || len(r.Segments) == 0 {
		return fmt.Errorf("organization_id, project_id and segments are required")
	}
	total := 0
	for index, segment := range r.Segments {
		if err := segment.Asset.Validate(); err != nil || segment.DurationSeconds < 1 || segment.DurationSeconds > 15 {
			return fmt.Errorf("segment %d is invalid", index+1)
		}
		total += segment.DurationSeconds
	}
	if total < 4 || total > 120 {
		return fmt.Errorf("segment composition must total between 4 and 120 seconds")
	}
	return nil
}

type SegmentComposer interface {
	ComposeSegments(context.Context, SegmentCompositionRequest) (CompositionOutput, error)
}

type CommandRunner interface {
	Run(context.Context, string, ...string) error
}

type ExecCommandRunner struct{}

func (ExecCommandRunner) Run(ctx context.Context, executable string, args ...string) error {
	command := exec.CommandContext(ctx, executable, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		summary := strings.TrimSpace(string(output))
		if len(summary) > 1200 {
			summary = summary[len(summary)-1200:]
		}
		return fmt.Errorf("media command failed: %w: %s", err, summary)
	}
	return nil
}

type FFmpegComposer struct {
	FFmpegPath string
	WorkRoot   string
	Sources    VideoSource
	Probe      assets.VideoMetadataProbe
	Runner     CommandRunner
}

func (c FFmpegComposer) ComposePreRoll(ctx context.Context, request PreRollCompositionRequest) (CompositionOutput, error) {
	if err := request.Validate(); err != nil {
		return CompositionOutput{}, err
	}
	if strings.TrimSpace(c.FFmpegPath) == "" || c.Sources == nil || c.Probe == nil {
		return CompositionOutput{}, fmt.Errorf("video composition capability is unavailable")
	}
	runner := c.Runner
	if runner == nil {
		runner = ExecCommandRunner{}
	}
	workRoot := strings.TrimSpace(c.WorkRoot)
	if workRoot == "" {
		workRoot = filepath.Join(".data", "video-work")
	}
	if err := os.MkdirAll(workRoot, 0o700); err != nil {
		return CompositionOutput{}, fmt.Errorf("create media work root: %w", err)
	}
	workDir, err := os.MkdirTemp(workRoot, "preroll-*")
	if err != nil {
		return CompositionOutput{}, fmt.Errorf("create composition directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(workDir) }

	preRollPath := filepath.Join(workDir, "preroll.mp4")
	mainPath := filepath.Join(workDir, "main.mp4")
	preRoll, err := c.copySource(ctx, request.OrganizationID, request.ProjectID, request.PreRollVideo, preRollPath)
	if err != nil {
		cleanup()
		return CompositionOutput{}, err
	}
	main, err := c.copySource(ctx, request.OrganizationID, request.ProjectID, request.MainVideo, mainPath)
	if err != nil {
		cleanup()
		return CompositionOutput{}, err
	}
	normalizedPreRoll := filepath.Join(workDir, "preroll-normalized.mp4")
	normalizedMain := filepath.Join(workDir, "main-normalized.mp4")
	if err := c.normalize(ctx, runner, preRollPath, normalizedPreRoll, preRoll); err != nil {
		cleanup()
		return CompositionOutput{}, fmt.Errorf("normalize pre-roll: %w", err)
	}
	if err := c.normalize(ctx, runner, mainPath, normalizedMain, main); err != nil {
		cleanup()
		return CompositionOutput{}, fmt.Errorf("normalize main video: %w", err)
	}
	concatList := filepath.Join(workDir, "concat.txt")
	listContents := "file '" + ffconcatPath(normalizedPreRoll) + "'\nfile '" + ffconcatPath(normalizedMain) + "'\n"
	if err := os.WriteFile(concatList, []byte(listContents), 0o600); err != nil {
		cleanup()
		return CompositionOutput{}, err
	}
	outputPath := filepath.Join(workDir, "final.mp4")
	if err := runner.Run(ctx, c.FFmpegPath,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "concat", "-safe", "0", "-i", concatList,
		"-c", "copy", "-movflags", "+faststart", outputPath,
	); err != nil {
		cleanup()
		return CompositionOutput{}, fmt.Errorf("concat normalized videos: %w", err)
	}
	contents, err := os.ReadFile(outputPath)
	if err != nil {
		cleanup()
		return CompositionOutput{}, err
	}
	metadata, err := c.Probe.Probe(ctx, contents)
	if err != nil {
		cleanup()
		return CompositionOutput{}, fmt.Errorf("validate composition output: %w", err)
	}
	file, err := os.Open(outputPath)
	if err != nil {
		cleanup()
		return CompositionOutput{}, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		cleanup()
		return CompositionOutput{}, err
	}
	return CompositionOutput{
		Content:   &cleanupReadCloser{ReadCloser: file, cleanup: cleanup},
		SizeBytes: info.Size(),
		Metadata:  metadata,
	}, nil
}

func (c FFmpegComposer) ComposeSegments(ctx context.Context, request SegmentCompositionRequest) (CompositionOutput, error) {
	if err := request.Validate(); err != nil {
		return CompositionOutput{}, err
	}
	if strings.TrimSpace(c.FFmpegPath) == "" || c.Sources == nil || c.Probe == nil {
		return CompositionOutput{}, fmt.Errorf("video composition capability is unavailable")
	}
	runner := c.Runner
	if runner == nil {
		runner = ExecCommandRunner{}
	}
	workRoot := strings.TrimSpace(c.WorkRoot)
	if workRoot == "" {
		workRoot = filepath.Join(".data", "video-work")
	}
	if err := os.MkdirAll(workRoot, 0o700); err != nil {
		return CompositionOutput{}, fmt.Errorf("create media work root: %w", err)
	}
	workDir, err := os.MkdirTemp(workRoot, "brand-film-*")
	if err != nil {
		return CompositionOutput{}, fmt.Errorf("create composition directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(workDir) }
	listContents := strings.Builder{}
	for index, segment := range request.Segments {
		sourcePath := filepath.Join(workDir, fmt.Sprintf("segment-%02d.mp4", index+1))
		version, copyErr := c.copySource(ctx, request.OrganizationID, request.ProjectID, segment.Asset, sourcePath)
		if copyErr != nil {
			cleanup()
			return CompositionOutput{}, copyErr
		}
		normalizedPath := filepath.Join(workDir, fmt.Sprintf("segment-%02d-normalized.mp4", index+1))
		if normalizeErr := c.normalizeDuration(ctx, runner, sourcePath, normalizedPath, version, segment.DurationSeconds); normalizeErr != nil {
			cleanup()
			return CompositionOutput{}, fmt.Errorf("normalize segment %d: %w", index+1, normalizeErr)
		}
		listContents.WriteString("file '" + ffconcatPath(normalizedPath) + "'\n")
	}
	concatList := filepath.Join(workDir, "concat.txt")
	if err := os.WriteFile(concatList, []byte(listContents.String()), 0o600); err != nil {
		cleanup()
		return CompositionOutput{}, err
	}
	outputPath := filepath.Join(workDir, "brand-film-preview.mp4")
	if err := runner.Run(ctx, c.FFmpegPath, "-hide_banner", "-loglevel", "error", "-y", "-f", "concat", "-safe", "0", "-i", concatList, "-c", "copy", "-movflags", "+faststart", outputPath); err != nil {
		cleanup()
		return CompositionOutput{}, fmt.Errorf("concat brand film segments: %w", err)
	}
	contents, err := os.ReadFile(outputPath)
	if err != nil {
		cleanup()
		return CompositionOutput{}, err
	}
	metadata, err := c.Probe.Probe(ctx, contents)
	if err != nil {
		cleanup()
		return CompositionOutput{}, fmt.Errorf("validate composition output: %w", err)
	}
	file, err := os.Open(outputPath)
	if err != nil {
		cleanup()
		return CompositionOutput{}, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		cleanup()
		return CompositionOutput{}, err
	}
	return CompositionOutput{Content: &cleanupReadCloser{ReadCloser: file, cleanup: cleanup}, SizeBytes: info.Size(), Metadata: metadata}, nil
}

func (c FFmpegComposer) copySource(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, ref contract.AssetVersionRef, destination string) (assets.AssetVersion, error) {
	version, reader, err := c.Sources.OpenVideo(ctx, organizationID, projectID, ref)
	if err != nil {
		return assets.AssetVersion{}, err
	}
	defer reader.Close()
	if version.MIMEType != "video/mp4" || version.SizeBytes < 1 || version.SizeBytes > assets.MaxVideoBytes {
		return assets.AssetVersion{}, fmt.Errorf("asset %s is not a supported MP4", ref.AssetID)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return assets.AssetVersion{}, err
	}
	written, copyErr := io.Copy(file, io.LimitReader(reader, assets.MaxVideoBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		return assets.AssetVersion{}, copyErr
	}
	if closeErr != nil {
		return assets.AssetVersion{}, closeErr
	}
	if written != version.SizeBytes {
		return assets.AssetVersion{}, fmt.Errorf("asset size changed while reading")
	}
	return version, nil
}

func (c FFmpegComposer) normalize(ctx context.Context, runner CommandRunner, input, output string, version assets.AssetVersion) error {
	return c.normalizeDuration(ctx, runner, input, output, version, 0)
}

func (c FFmpegComposer) normalizeDuration(ctx context.Context, runner CommandRunner, input, output string, version assets.AssetVersion, durationSeconds int) error {
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-i", input}
	if version.AudioCodec == "" {
		duration := strconv.FormatFloat(float64(version.DurationMS)/1000, 'f', 3, 64)
		args = append(args, "-f", "lavfi", "-t", duration, "-i", "anullsrc=channel_layout=stereo:sample_rate=48000")
	}
	args = append(args,
		"-map", "0:v:0",
	)
	if version.AudioCodec == "" {
		args = append(args, "-map", "1:a:0", "-shortest")
	} else {
		args = append(args, "-map", "0:a:0")
	}
	args = append(args,
		"-vf", "scale=720:1280:force_original_aspect_ratio=decrease,pad=720:1280:(ow-iw)/2:(oh-ih)/2:black,fps=25,setsar=1",
		"-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-ar", "48000", "-ac", "2",
		"-movflags", "+faststart",
	)
	if durationSeconds > 0 {
		args = append(args, "-t", strconv.Itoa(durationSeconds))
	}
	args = append(args, output)
	return runner.Run(ctx, c.FFmpegPath, args...)
}

func ffconcatPath(value string) string {
	if !filepath.IsAbs(value) && !isWindowsAbsolutePath(value) {
		absolute, err := filepath.Abs(value)
		if err == nil {
			value = absolute
		}
	}
	normalized := strings.ReplaceAll(filepath.ToSlash(value), `\`, "/")
	return strings.ReplaceAll(normalized, "'", "'\\''")
}

func isWindowsAbsolutePath(value string) bool {
	if len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' {
		return value[2] == '\\' || value[2] == '/'
	}
	return strings.HasPrefix(value, `\\`)
}

type cleanupReadCloser struct {
	io.ReadCloser
	cleanup func()
}

func (r *cleanupReadCloser) Close() error {
	err := r.ReadCloser.Close()
	r.cleanup()
	return err
}
