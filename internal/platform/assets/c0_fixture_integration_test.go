package assets

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestC0FixtureIsPreviewableThroughAssetService(t *testing.T) {
	fixtureRoot := os.Getenv("COOKIES_C0_FIXTURE_ROOT")
	ffprobePath := os.Getenv("COOKIES_TEST_FFPROBE_PATH")
	if fixtureRoot == "" || ffprobePath == "" {
		if os.Getenv("COOKIES_REQUIRE_TIMELINE_GOLDEN") == "1" {
			t.Fatal("C0 Asset preview verification requires fixture root and ffprobe")
		}
		t.Skip("set C0 fixture environment")
	}
	contents, err := os.ReadFile(filepath.Join(fixtureRoot, "video-a.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contents)
	digestText := hex.EncodeToString(digest[:])
	blobs, err := NewFilesystemBlobStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	repository := newFakeRepository()
	service := UploadService{
		Repository:       repository,
		Projects:         fakeProjects{organization: "org_c0", project: "project_c0", version: 1},
		Blobs:            blobs,
		Scanner:          NoopScanner{},
		VideoProbe:       FFprobeVideoProbe{Path: ffprobePath, WorkRoot: t.TempDir()},
		QuarantineBucket: "quarantine",
		AssetsBucket:     "assets",
		Now:              func() time.Time { return now },
		NewID:            sequenceIDs(),
	}
	requestContext := testRequestContext("org_c0", "project_c0")
	created, err := service.Create(context.Background(), requestContext, "project_c0", "c0-video-a", CreateUploadRequest{
		Filename:          "video-a.mp4",
		DeclaredMIMEType:  "video/mp4",
		DeclaredSizeBytes: int64(len(contents)),
		DeclaredSHA256:    &digestText,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.PutContent(context.Background(), requestContext.Actor, "project_c0", created.Session.ID, bytes.NewReader(contents), int64(len(contents))); err != nil {
		t.Fatal(err)
	}
	finalized, err := service.Finalize(context.Background(), requestContext, "project_c0", created.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finalized.ProjectAssetRef == nil {
		t.Fatal("fixture upload did not create an AssetVersion reference")
	}
	stored, err := repository.GetProjectAsset(context.Background(), "org_c0", "project_c0", finalized.ProjectAssetRef.AssetVersion)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Version.SHA256 != digestText || stored.Version.DurationMS != 6000 || stored.Version.WidthPixels != 720 || stored.Version.HeightPixels != 1280 || stored.Version.VideoCodec != "h264" {
		t.Fatalf("unexpected persisted C0 fixture metadata: %#v", stored.Version)
	}
	preview, info, err := service.OpenPreview(context.Background(), requestContext.Actor, "project_c0", finalized.ProjectAssetRef.AssetVersion)
	if err != nil {
		t.Fatal(err)
	}
	previewContents, readErr := io.ReadAll(preview)
	closeErr := preview.Close()
	if readErr != nil || closeErr != nil || info.MIMEType != "video/mp4" || !bytes.Equal(previewContents, contents) {
		t.Fatalf("C0 fixture preview mismatch: mime=%q size=%d read=%v close=%v", info.MIMEType, len(previewContents), readErr, closeErr)
	}
}
