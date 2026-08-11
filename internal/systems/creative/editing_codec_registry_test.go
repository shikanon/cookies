package creative

import (
	"encoding/json"
	"testing"

	"github.com/shikanon/cookies/internal/platform/contract"
)

func TestCodecRegistryKeepsHistoricalV1ReadableWithoutChangingItsPayload(t *testing.T) {
	ref := contract.AssetVersionRef{AssetID: "asset_1", Version: 2}
	v1 := EditingTimelineV1{SchemaVersion: EditingTimelineSchemaV1, OutputProfile: EditingMVPVerticalOutputProfile, DurationMS: 6000, Tracks: []EditingTimelineTrack{{ID: "video-primary", Role: EditingTrackPrimaryVideo, Clips: []EditingTimelineClip{{ID: "clip-1", AssetRef: &ref, TimelineEndMS: 6000, SourceOutMS: 6000}}}}}
	payload, _ := json.Marshal(v1)
	document, err := DefaultEditingCodecRegistry().Decode(payload)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := DefaultEditingCodecRegistry().Encode(document)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := contract.CanonicalJSONHash(v1)
	var restored EditingTimelineV1
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	after, _ := contract.CanonicalJSONHash(restored)
	if before != after || document.SchemaVersion() != EditingTimelineSchemaV1 {
		t.Fatalf("v1 changed: %s != %s", before, after)
	}
}

func TestV1ToV2MigrationIsDeterministicAndUsesFramesAndMicroseconds(t *testing.T) {
	ref := contract.AssetVersionRef{AssetID: "asset_1", Version: 2}
	v1 := EditingTimelineV1{SchemaVersion: EditingTimelineSchemaV1, OutputProfile: EditingMVPVerticalOutputProfile, DurationMS: 6000, Tracks: []EditingTimelineTrack{{ID: "video-primary", Role: EditingTrackPrimaryVideo, Clips: []EditingTimelineClip{{ID: "clip-1", AssetRef: &ref, TimelineEndMS: 6000, SourceOutMS: 6000}}}}}
	document := EditingDocument{V1: &v1}
	first, err := DefaultEditingCodecRegistry().MigrateToV2(document)
	if err != nil {
		t.Fatal(err)
	}
	second, err := DefaultEditingCodecRegistry().MigrateToV2(document)
	if err != nil {
		t.Fatal(err)
	}
	firstHash, _ := contract.CanonicalJSONHash(first.V2)
	secondHash, _ := contract.CanonicalJSONHash(second.V2)
	clip := first.V2.Tracks[0].Clips[0]
	if firstHash != secondHash || first.V2.DurationFrames != 180 || clip.Timeline.DurationFrames != 180 || clip.Source.OutUS != 6_000_000 {
		t.Fatalf("migration is not deterministic or normalized: %#v hashes=%s/%s", first.V2, firstHash, secondHash)
	}
}
