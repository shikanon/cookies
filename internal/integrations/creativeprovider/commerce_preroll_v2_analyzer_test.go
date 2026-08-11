package creativeprovider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommerceFrameSamplePlanCoversOpeningMiddleTailAndScenes(t *testing.T) {
	t.Parallel()

	const durationMS = int64(90000)
	values := commerceFrameSamplePlan(durationMS, []int64{17321, 61123})
	if len(values) < 20 || len(values) > 32 {
		t.Fatalf("sample count = %d, want 20..32", len(values))
	}
	wants := []int64{0, 500, 3000, 17321, 61123, durationMS - 250}
	for _, want := range wants {
		if !containsCommerceTimestamp(values, want) {
			t.Fatalf("sample plan does not contain %dms: %#v", want, values)
		}
	}
	if values[0] != 0 || values[len(values)-1] != durationMS-250 {
		t.Fatalf("sample plan does not span the full video: %#v", values)
	}
}

func TestCommerceFrameSamplePlanClampsShortVideos(t *testing.T) {
	t.Parallel()

	values := commerceFrameSamplePlan(1800, []int64{-100, 9000})
	if len(values) == 0 || values[0] != 0 || values[len(values)-1] > 1550 {
		t.Fatalf("short-video sample plan is not clamped: %#v", values)
	}
	for _, value := range values {
		if value < 0 || value >= 1800 {
			t.Fatalf("sample %d is outside the source duration", value)
		}
	}
}

func TestReadCommerceSampledFrameSkipsMissingFFmpegOutput(t *testing.T) {
	t.Parallel()

	frame, ok, err := readCommerceSampledFrame(filepath.Join(t.TempDir(), "missing.jpg"), 1200)
	if err != nil || ok || len(frame.Content) != 0 {
		t.Fatalf("missing frame = (%#v, %v, %v), want a clean skip", frame, ok, err)
	}
}

func TestReadCommerceSampledFrameKeepsValidOutput(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "frame.jpg")
	if err := os.WriteFile(path, []byte("jpeg"), 0o600); err != nil {
		t.Fatal(err)
	}
	frame, ok, err := readCommerceSampledFrame(path, 1200)
	if err != nil || !ok || frame.TimestampMS != 1200 || string(frame.Content) != "jpeg" {
		t.Fatalf("valid frame = (%#v, %v, %v)", frame, ok, err)
	}
}

func containsCommerceTimestamp(values []int64, wanted int64) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
