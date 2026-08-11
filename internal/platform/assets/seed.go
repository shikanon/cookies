package assets

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type SeedAsset struct {
	OrganizationID        contract.OrganizationID
	ProjectID             contract.ProjectID
	AssetID               contract.AssetID
	BlobID                string
	Kind                  contract.AssetKind
	SourceType            contract.AssetSourceType
	MIMEType              string
	SizeBytes             int64
	SHA256                string
	WidthPixels           int
	HeightPixels          int
	Media                 MediaMetadata
	ProviderJobID         string
	ProviderOutputID      string
	ProjectContextVersion int64
	Location              ObjectLocation
}

func (a SeedAsset) Validate() error {
	if strings.TrimSpace(string(a.OrganizationID)) == "" || strings.TrimSpace(string(a.ProjectID)) == "" {
		return fmt.Errorf("seed asset scope is required")
	}
	if strings.TrimSpace(string(a.AssetID)) == "" || strings.TrimSpace(a.BlobID) == "" {
		return fmt.Errorf("seed asset ids are required")
	}
	if !validSeedAssetKindForMIME(a.Kind, a.MIMEType) || a.SizeBytes < 1 || !validSHA256(a.SHA256) {
		return fmt.Errorf("seed asset metadata is invalid")
	}
	if a.SourceType != contract.AssetSourceImported && a.SourceType != contract.AssetSourceProviderGenerated {
		return fmt.Errorf("seed asset source_type must be imported or provider_generated")
	}
	if a.SourceType == contract.AssetSourceProviderGenerated && (strings.TrimSpace(a.ProviderJobID) == "" || strings.TrimSpace(a.ProviderOutputID) == "") {
		return fmt.Errorf("provider-generated seed asset requires provider_job_id and provider_output_id")
	}
	if a.ProjectContextVersion < 1 {
		return fmt.Errorf("seed asset project_context_version must be positive")
	}
	if a.Location.Provider == "" || validateObjectTarget(a.Location.Bucket, a.Location.Key) != nil {
		return fmt.Errorf("seed asset object location is invalid")
	}
	return nil
}

// EnsureSeedAsset records a deterministic, deployment-visible project asset
// reference for canonical demo data. The object bytes are owned by the stable
// object location, while DB rows remain idempotent across repeated startup seed.
func (r MySQLRepository) EnsureSeedAsset(ctx context.Context, asset SeedAsset, now time.Time) (contract.ProjectAssetRef, error) {
	db, err := r.db()
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if err := asset.Validate(); err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO asset_blobs
		(id, organization_id, storage_provider, bucket_name, object_key, storage_version_id, etag, sha256, size_bytes, mime_type, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'ready', ?)
		ON DUPLICATE KEY UPDATE status='ready', sha256=VALUES(sha256), size_bytes=VALUES(size_bytes), mime_type=VALUES(mime_type)`,
		asset.BlobID, asset.OrganizationID, asset.Location.Provider, asset.Location.Bucket, asset.Location.Key,
		nullable(asset.Location.VersionID), nullable(asset.Location.ETag), asset.SHA256, asset.SizeBytes, asset.MIMEType, now); err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO assets
		(id, organization_id, asset_kind, status, owner_system, latest_version, created_at, updated_at)
		VALUES (?, ?, ?, 'ready', 'assets', 1, ?, ?)
		ON DUPLICATE KEY UPDATE asset_kind=VALUES(asset_kind), status='ready', owner_system='assets', latest_version=1, updated_at=VALUES(updated_at)`,
		asset.AssetID, asset.OrganizationID, asset.Kind, now, now); err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO asset_versions
		(organization_id, asset_id, version, blob_id, status, source_type, mime_type, size_bytes, sha256,
		 width_pixels, height_pixels, duration_ms, frame_rate, video_codec, duration_seconds, fps, codec, bitrate_bps, audio_codec,
		 audio_channels, audio_sample_rate, poster_frame_ref, probe_status, probe_error,
		 provider_job_id, provider_output_id, project_context_version, created_at)
		VALUES (?, ?, 1, ?, 'ready', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE status='ready', source_type=VALUES(source_type), mime_type=VALUES(mime_type),
		 size_bytes=VALUES(size_bytes), sha256=VALUES(sha256), width_pixels=VALUES(width_pixels), height_pixels=VALUES(height_pixels),
		 duration_ms=VALUES(duration_ms), frame_rate=VALUES(frame_rate), video_codec=VALUES(video_codec),
		 duration_seconds=VALUES(duration_seconds), fps=VALUES(fps), codec=VALUES(codec), bitrate_bps=VALUES(bitrate_bps),
		 audio_codec=VALUES(audio_codec), audio_channels=VALUES(audio_channels), audio_sample_rate=VALUES(audio_sample_rate),
		 probe_status=VALUES(probe_status), probe_error=VALUES(probe_error), project_context_version=VALUES(project_context_version)`,
		asset.OrganizationID, asset.AssetID, asset.BlobID, asset.SourceType, asset.MIMEType, asset.SizeBytes, asset.SHA256,
		nullableInt(asset.WidthPixels), nullableInt(asset.HeightPixels), nullableInt64(seedDurationMS(asset.Media)), nullable(seedFrameRate(asset.Media)), nullable(asset.Media.Codec), nullableFloat(asset.Media.DurationSeconds),
		nullableFloat(asset.Media.FPS), nullable(asset.Media.Codec), nullableInt64(asset.Media.BitrateBPS),
		nullable(asset.Media.AudioCodec), nullableInt(asset.Media.AudioChannels), nullableInt(asset.Media.AudioSampleRate),
		nullable(asset.Media.PosterFrameRef), probeStatusValue(asset.Media.ProbeStatus), nullable(asset.Media.ProbeError),
		nullable(asset.ProviderJobID), nullable(asset.ProviderOutputID), nullableInt64(asset.ProjectContextVersion), now); err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO project_assets
		(organization_id, project_id, asset_id, asset_version, status, created_at)
		VALUES (?, ?, ?, 1, 'active', ?)
		ON DUPLICATE KEY UPDATE status='active'`,
		asset.OrganizationID, asset.ProjectID, asset.AssetID, now); err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if err := tx.Commit(); err != nil {
		return contract.ProjectAssetRef{}, err
	}
	return contract.ProjectAssetRef{
		ProjectID:    asset.ProjectID,
		AssetVersion: contract.AssetVersionRef{AssetID: asset.AssetID, Version: 1},
	}, nil
}

func seedDurationMS(media MediaMetadata) int64 {
	if media.DurationSeconds <= 0 {
		return 0
	}
	return int64(math.Round(media.DurationSeconds * 1000))
}

func seedFrameRate(media MediaMetadata) string {
	if media.FPS <= 0 {
		return ""
	}
	return strconv.FormatFloat(media.FPS, 'f', -1, 64) + "/1"
}

func validSeedAssetKindForMIME(kind contract.AssetKind, mimeType string) bool {
	if validAssetKindForMIME(kind, mimeType) {
		return true
	}
	switch kind {
	case contract.AssetDocument:
		return mimeType == "text/plain" || mimeType == "application/pdf" || mimeType == "application/json"
	case contract.AssetText:
		return mimeType == "text/plain"
	default:
		return false
	}
}
