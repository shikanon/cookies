package assets

import "testing"

func TestSeedVideoMetadataPopulatesNormalizedEditingFields(t *testing.T) {
	media := MediaMetadata{DurationSeconds: 15, FPS: 30, Codec: "h264", AudioCodec: "aac"}
	if got := seedDurationMS(media); got != 15000 {
		t.Fatalf("duration_ms = %d, want 15000", got)
	}
	if got := seedFrameRate(media); got != "30/1" {
		t.Fatalf("frame_rate = %q, want 30/1", got)
	}
}
