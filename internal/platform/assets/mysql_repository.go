package assets

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/shikanon/cookies/internal/platform/contract"
)

type MySQLRepository struct{ DB *sql.DB }

func (r MySQLRepository) GetCurrentAssetRights(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, ref contract.AssetVersionRef) (AssetRightsVersion, error) {
	if _, err := r.db(); err != nil {
		return AssetRightsVersion{}, err
	}
	var value AssetRightsVersion
	var channels, territories, purposes, evidence []byte
	var verifiedBy sql.NullString
	err := r.DB.QueryRowContext(ctx, `SELECT id, version, organization_id, project_id, asset_id, asset_version,
		source, rights_holder, status, derivative_work_allowed, generative_ai_allowed, allowed_channels,
		territories, purposes, valid_from, valid_until, revoked_at, asserted_by, verified_by, evidence, created_at
		FROM asset_rights_versions WHERE organization_id=? AND project_id=? AND asset_id=? AND asset_version=?
		ORDER BY version DESC LIMIT 1`, org, project, ref.AssetID, ref.Version).Scan(
		&value.ID, &value.Version, &value.OrganizationID, &value.ProjectID, &value.AssetRef.AssetID, &value.AssetRef.Version,
		&value.Source, &value.RightsHolder, &value.Status, &value.DerivativeWorkAllowed, &value.GenerativeAIAllowed,
		&channels, &territories, &purposes, &value.ValidFrom, &value.ValidUntil, &value.RevokedAt, &value.AssertedBy,
		&verifiedBy, &evidence, &value.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AssetRightsVersion{}, ErrNotFound
	}
	if err != nil {
		return AssetRightsVersion{}, err
	}
	if verifiedBy.Valid {
		value.VerifiedBy = verifiedBy.String
	}
	if err := json.Unmarshal(channels, &value.AllowedChannels); err != nil {
		return AssetRightsVersion{}, err
	}
	if err := json.Unmarshal(territories, &value.Territories); err != nil {
		return AssetRightsVersion{}, err
	}
	if err := json.Unmarshal(purposes, &value.Purposes); err != nil {
		return AssetRightsVersion{}, err
	}
	if err := json.Unmarshal(evidence, &value.Evidence); err != nil {
		return AssetRightsVersion{}, err
	}
	return value, nil
}

func (r MySQLRepository) db() (*sql.DB, error) {
	if r.DB == nil {
		return nil, fmt.Errorf("assets database is required")
	}
	return r.DB, nil
}

func (r MySQLRepository) CreateUpload(ctx context.Context, value UploadSession) (UploadSession, bool, error) {
	db, err := r.db()
	if err != nil {
		return UploadSession{}, false, err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO upload_sessions
		(id, organization_id, project_id, principal_kind, principal_id, status, original_filename,
		 declared_mime_type, declared_size_bytes, declared_sha256, quarantine_provider, quarantine_bucket,
		 quarantine_object_key, idempotency_key, request_hash, project_context_version, target_asset_id,
		 target_blob_id, request_id, trace_id, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID, value.OrganizationID, value.ProjectID, value.Principal.Kind, value.Principal.ID, value.Status,
		value.Filename, value.DeclaredMIMEType, value.DeclaredSizeBytes, value.DeclaredSHA256,
		value.Quarantine.Provider, value.Quarantine.Bucket, value.Quarantine.Key, value.IdempotencyKey,
		value.RequestHash, value.ProjectContextVersion, value.TargetAssetID, value.TargetBlobID,
		value.RequestID, value.TraceID, value.ExpiresAt, value.CreatedAt, value.UpdatedAt)
	if err == nil {
		return value, true, nil
	}
	if !isDuplicate(err) {
		return UploadSession{}, false, err
	}
	existing, getErr := r.getUploadByIdempotency(ctx, value)
	if getErr != nil {
		return UploadSession{}, false, getErr
	}
	if existing.RequestHash != value.RequestHash {
		return UploadSession{}, false, ErrIdempotencyConflict
	}
	return existing, false, nil
}

func (r MySQLRepository) getUploadByIdempotency(ctx context.Context, value UploadSession) (UploadSession, error) {
	row := r.DB.QueryRowContext(ctx, uploadSelect+` WHERE organization_id=? AND project_id=? AND principal_kind=? AND principal_id=? AND idempotency_key=?`,
		value.OrganizationID, value.ProjectID, value.Principal.Kind, value.Principal.ID, value.IdempotencyKey)
	return scanUpload(row)
}

func (r MySQLRepository) GetUpload(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string) (UploadSession, error) {
	if _, err := r.db(); err != nil {
		return UploadSession{}, err
	}
	return scanUpload(r.DB.QueryRowContext(ctx, uploadSelect+` WHERE organization_id=? AND project_id=? AND id=?`, org, project, id))
}

func (r MySQLRepository) MarkUploadUploaded(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string, now time.Time) error {
	return r.transitionUpload(ctx, org, project, id, now, []UploadStatus{UploadCreated, UploadUploaded}, UploadUploaded)
}

func (r MySQLRepository) MarkUploadProcessing(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string, now time.Time) error {
	return r.transitionUpload(ctx, org, project, id, now, []UploadStatus{UploadCreated, UploadUploaded}, UploadProcessing)
}

func (r MySQLRepository) transitionUpload(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string, now time.Time, from []UploadStatus, to UploadStatus) error {
	if _, err := r.db(); err != nil {
		return err
	}
	result, err := r.DB.ExecContext(ctx, `UPDATE upload_sessions SET status=?, updated_at=?
		WHERE organization_id=? AND project_id=? AND id=? AND status IN (?, ?) AND expires_at>?`,
		to, now, org, project, id, from[0], from[len(from)-1], now)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrInvalidState
	}
	return nil
}

func (r MySQLRepository) CompleteUpload(ctx context.Context, uploadID string, commit AssetCommit, now time.Time) (contract.ProjectAssetRef, error) {
	return r.complete(ctx, "upload", uploadID, GeneratedIntake{}, commit, now)
}

func (r MySQLRepository) FailUpload(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id, code string, now time.Time) error {
	if _, err := r.db(); err != nil {
		return err
	}
	_, err := r.DB.ExecContext(ctx, `UPDATE upload_sessions SET status='failed', error_code=?, updated_at=?
		WHERE organization_id=? AND project_id=? AND id=? AND status IN ('created','uploaded','processing')`, code, now, org, project, id)
	return err
}

func (r MySQLRepository) CreateIntake(ctx context.Context, value GeneratedIntake) (GeneratedIntake, bool, error) {
	db, err := r.db()
	if err != nil {
		return GeneratedIntake{}, false, err
	}
	payload, err := json.Marshal(value.Request)
	if err != nil {
		return GeneratedIntake{}, false, err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO generated_intakes
		(id, organization_id, project_id, provider_job_id, output_id, provider_code, status, request_payload,
		 idempotency_key, request_hash, target_asset_id, target_blob_id, attempt_count, max_attempts,
		 request_id, trace_id, available_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, value.OrganizationID,
		value.ProjectID, value.ProviderJobID, value.OutputID, value.ProviderCode, value.Status, payload,
		value.IdempotencyKey, value.RequestHash, value.TargetAssetID, value.TargetBlobID, value.AttemptCount,
		value.MaxAttempts, value.RequestID, value.TraceID, value.AvailableAt, value.CreatedAt, value.UpdatedAt)
	if err == nil {
		return value, true, nil
	}
	if !isDuplicate(err) {
		return GeneratedIntake{}, false, err
	}
	existing, getErr := r.getIntakeByOutput(ctx, value)
	if getErr != nil {
		return GeneratedIntake{}, false, getErr
	}
	if existing.IdempotencyKey != value.IdempotencyKey || existing.RequestHash != value.RequestHash {
		return GeneratedIntake{}, false, ErrIdempotencyConflict
	}
	return existing, false, nil
}

func (r MySQLRepository) getIntakeByOutput(ctx context.Context, value GeneratedIntake) (GeneratedIntake, error) {
	return scanIntake(r.DB.QueryRowContext(ctx, intakeSelect+` WHERE organization_id=? AND project_id=? AND provider_job_id=? AND output_id=?`,
		value.OrganizationID, value.ProjectID, value.ProviderJobID, value.OutputID))
}

func (r MySQLRepository) GetIntake(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string) (GeneratedIntake, error) {
	if _, err := r.db(); err != nil {
		return GeneratedIntake{}, err
	}
	return scanIntake(r.DB.QueryRowContext(ctx, intakeSelect+` WHERE organization_id=? AND project_id=? AND id=?`, org, project, id))
}

func (r MySQLRepository) ClaimIntake(ctx context.Context, actor contract.ActorContext, worker string, now time.Time) (GeneratedIntake, bool, error) {
	db, err := r.db()
	if err != nil {
		return GeneratedIntake{}, false, err
	}
	if err := actor.Validate(); err != nil {
		return GeneratedIntake{}, false, err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return GeneratedIntake{}, false, err
	}
	defer tx.Rollback()
	staleBefore := now.Add(-5 * time.Minute)
	value, err := scanIntake(tx.QueryRowContext(ctx, intakeSelect+` gi WHERE gi.organization_id=? AND EXISTS (
		SELECT 1 FROM project_memberships pm WHERE pm.organization_id=gi.organization_id AND pm.project_id=gi.project_id
		AND pm.principal_kind=? AND pm.principal_id=? AND pm.status='active')
		AND ((gi.status='queued' AND gi.available_at<=?) OR (gi.status='running' AND gi.locked_at<?))
		ORDER BY gi.created_at LIMIT 1 FOR UPDATE SKIP LOCKED`, actor.OrganizationID, actor.Principal.Kind, actor.Principal.ID, now, staleBefore))
	if errors.Is(err, ErrNotFound) {
		return GeneratedIntake{}, false, nil
	}
	if err != nil {
		return GeneratedIntake{}, false, err
	}
	if value.AttemptCount >= value.MaxAttempts {
		_, err = tx.ExecContext(ctx, `UPDATE generated_intakes SET status='failed', error_code='INTAKE_ATTEMPTS_EXHAUSTED', error_message='intake attempts exhausted', retryable=FALSE, lock_owner=NULL, locked_at=NULL, updated_at=? WHERE id=?`, now, value.ID)
		if err != nil {
			return GeneratedIntake{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return GeneratedIntake{}, false, err
		}
		return GeneratedIntake{}, false, nil
	}
	result, err := tx.ExecContext(ctx, `UPDATE generated_intakes SET status='running', lock_owner=?, locked_at=?, attempt_count=attempt_count+1, updated_at=? WHERE id=? AND status IN ('queued','running')`, worker, now, now, value.ID)
	if err != nil {
		return GeneratedIntake{}, false, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return GeneratedIntake{}, false, nil
	}
	if err := tx.Commit(); err != nil {
		return GeneratedIntake{}, false, err
	}
	value.Status, value.LockOwner, value.AttemptCount, value.UpdatedAt = GeneratedIntakeRunning, worker, value.AttemptCount+1, now
	return value, true, nil
}

func (r MySQLRepository) CompleteIntake(ctx context.Context, intake GeneratedIntake, commit AssetCommit, now time.Time) (contract.ProjectAssetRef, error) {
	return r.complete(ctx, "intake", intake.ID, intake, commit, now)
}

func (r MySQLRepository) RetryIntake(ctx context.Context, intake GeneratedIntake, failure contract.JobError, available time.Time) error {
	if intake.AttemptCount >= intake.MaxAttempts {
		return r.FailIntake(ctx, intake, failure, time.Now().UTC())
	}
	result, err := r.DB.ExecContext(ctx, `UPDATE generated_intakes SET status='queued', error_code=?, error_message=?, retryable=?, available_at=?, lock_owner=NULL, locked_at=NULL, updated_at=CURRENT_TIMESTAMP(6) WHERE id=? AND status='running' AND lock_owner=?`,
		failure.Code, failure.Message, failure.Retryable, available, intake.ID, intake.LockOwner)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return ErrInvalidState
	}
	return nil
}

func (r MySQLRepository) FailIntake(ctx context.Context, intake GeneratedIntake, failure contract.JobError, now time.Time) error {
	result, err := r.DB.ExecContext(ctx, `UPDATE generated_intakes SET status='failed', error_code=?, error_message=?, retryable=?, lock_owner=NULL, locked_at=NULL, updated_at=? WHERE id=? AND (status='queued' OR (status='running' AND lock_owner=?))`,
		failure.Code, failure.Message, failure.Retryable, now, intake.ID, intake.LockOwner)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return ErrInvalidState
	}
	return nil
}

func (r MySQLRepository) CompleteRender(ctx context.Context, renderJobID string, commit AssetCommit, now time.Time) (contract.ProjectAssetRef, error) {
	db, err := r.db()
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if renderJobID == "" || commit.SourceType != contract.AssetSourceRendered || commit.RenderJobID != renderJobID {
		return contract.ProjectAssetRef{}, ErrInvalidState
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	defer tx.Rollback()
	if err := insertAssetCommit(ctx, tx, commit, now); err != nil {
		if !isDuplicate(err) {
			return contract.ProjectAssetRef{}, err
		}
		var assetID contract.AssetID
		var version int64
		lookupErr := tx.QueryRowContext(ctx, `SELECT asset_id, version FROM asset_versions WHERE organization_id=? AND render_job_id=?`, commit.OrganizationID, renderJobID).Scan(&assetID, &version)
		if lookupErr != nil {
			return contract.ProjectAssetRef{}, err
		}
		return contract.ProjectAssetRef{ProjectID: commit.ProjectID, AssetVersion: contract.AssetVersionRef{AssetID: assetID, Version: version}}, nil
	}
	if err := tx.Commit(); err != nil {
		return contract.ProjectAssetRef{}, err
	}
	return contract.ProjectAssetRef{ProjectID: commit.ProjectID, AssetVersion: contract.AssetVersionRef{AssetID: commit.AssetID, Version: commit.Version}}, nil
}

func (r MySQLRepository) CompleteDerived(ctx context.Context, derivationID string, commit AssetCommit, now time.Time) (contract.ProjectAssetRef, error) {
	db, err := r.db()
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if derivationID == "" || commit.SourceType != contract.AssetSourceDerived || commit.DerivationID != derivationID {
		return contract.ProjectAssetRef{}, ErrInvalidState
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	defer tx.Rollback()
	if err := insertAssetCommit(ctx, tx, commit, now); err != nil {
		if !isDuplicate(err) {
			return contract.ProjectAssetRef{}, err
		}
		var assetID contract.AssetID
		var version int64
		lookupErr := tx.QueryRowContext(ctx, `SELECT asset_id, version FROM asset_versions WHERE organization_id=? AND derivation_id=?`, commit.OrganizationID, derivationID).Scan(&assetID, &version)
		if lookupErr != nil {
			return contract.ProjectAssetRef{}, err
		}
		return contract.ProjectAssetRef{ProjectID: commit.ProjectID, AssetVersion: contract.AssetVersionRef{AssetID: assetID, Version: version}}, nil
	}
	if err := tx.Commit(); err != nil {
		return contract.ProjectAssetRef{}, err
	}
	return contract.ProjectAssetRef{ProjectID: commit.ProjectID, AssetVersion: contract.AssetVersionRef{AssetID: commit.AssetID, Version: commit.Version}}, nil
}

func (r MySQLRepository) complete(ctx context.Context, source, sourceID string, intake GeneratedIntake, commit AssetCommit, now time.Time) (contract.ProjectAssetRef, error) {
	db, err := r.db()
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	defer tx.Rollback()
	var status string
	var org contract.OrganizationID
	var project contract.ProjectID
	var existingAsset sql.NullString
	var existingVersion sql.NullInt64
	var targetAssetID contract.AssetID
	var targetBlobID string
	var actualLockOwner sql.NullString
	query := `SELECT status, organization_id, project_id, asset_id, asset_version, target_asset_id, target_blob_id FROM upload_sessions WHERE id=? FOR UPDATE`
	if source == "intake" {
		query = `SELECT status, organization_id, project_id, asset_id, asset_version, target_asset_id, target_blob_id, lock_owner FROM generated_intakes WHERE id=? FOR UPDATE`
	}
	row := tx.QueryRowContext(ctx, query, sourceID)
	if source == "intake" {
		err = row.Scan(&status, &org, &project, &existingAsset, &existingVersion, &targetAssetID, &targetBlobID, &actualLockOwner)
	} else {
		err = row.Scan(&status, &org, &project, &existingAsset, &existingVersion, &targetAssetID, &targetBlobID)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return contract.ProjectAssetRef{}, ErrNotFound
		}
		return contract.ProjectAssetRef{}, err
	}
	if status == "succeeded" && existingAsset.Valid && existingVersion.Valid {
		return contract.ProjectAssetRef{ProjectID: project, AssetVersion: contract.AssetVersionRef{AssetID: contract.AssetID(existingAsset.String), Version: existingVersion.Int64}}, nil
	}
	expected := "processing"
	if source == "intake" {
		expected = "running"
	}
	if status != expected || org != commit.OrganizationID || project != commit.ProjectID || targetAssetID != commit.AssetID || targetBlobID != commit.BlobID {
		return contract.ProjectAssetRef{}, ErrInvalidState
	}
	if source == "intake" && (!actualLockOwner.Valid || actualLockOwner.String != intake.LockOwner) {
		return contract.ProjectAssetRef{}, ErrInvalidState
	}
	if source == "intake" && (commit.SourceType != contract.AssetSourceProviderGenerated || commit.ProviderJobID != intake.ProviderJobID || commit.ProviderOutputID != intake.OutputID) {
		return contract.ProjectAssetRef{}, ErrInvalidState
	}
	if source == "upload" && commit.SourceType != contract.AssetSourceUpload {
		return contract.ProjectAssetRef{}, ErrInvalidState
	}
	if err := insertAssetCommit(ctx, tx, commit, now); err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if source == "upload" {
		_, err = tx.ExecContext(ctx, `UPDATE upload_sessions SET status='succeeded', asset_id=?, asset_version=?, error_code=NULL, updated_at=? WHERE id=?`, commit.AssetID, commit.Version, now, sourceID)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE generated_intakes SET status='succeeded', asset_id=?, asset_version=?, error_code=NULL, error_message=NULL, retryable=NULL, lock_owner=NULL, locked_at=NULL, updated_at=? WHERE id=?`, commit.AssetID, commit.Version, now, sourceID)
	}
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if err := tx.Commit(); err != nil {
		// Commit errors are outcome-ambiguous. Re-read the source before the
		// caller considers deleting the durable object.
		var committedStatus string
		var committedAsset sql.NullString
		var committedVersion sql.NullInt64
		check := `SELECT status, asset_id, asset_version FROM upload_sessions WHERE id=?`
		if source == "intake" {
			check = `SELECT status, asset_id, asset_version FROM generated_intakes WHERE id=?`
		}
		if checkErr := db.QueryRowContext(ctx, check, sourceID).Scan(&committedStatus, &committedAsset, &committedVersion); checkErr == nil && committedStatus == "succeeded" && committedAsset.Valid && committedVersion.Valid {
			return contract.ProjectAssetRef{ProjectID: commit.ProjectID, AssetVersion: contract.AssetVersionRef{AssetID: contract.AssetID(committedAsset.String), Version: committedVersion.Int64}}, nil
		}
		return contract.ProjectAssetRef{}, err
	}
	return contract.ProjectAssetRef{ProjectID: commit.ProjectID, AssetVersion: contract.AssetVersionRef{AssetID: commit.AssetID, Version: commit.Version}}, nil
}

func insertAssetCommit(ctx context.Context, tx *sql.Tx, c AssetCommit, now time.Time) error {
	expectedKind, maxBytes, supported := generatedAssetPolicy(c.MIMEType)
	if c.Version != 1 || c.OrganizationID == "" || c.ProjectID == "" || c.AssetID == "" || c.BlobID == "" || c.SizeBytes < 1 || c.SizeBytes > maxBytes || !validSHA256(c.SHA256) || !supported || c.Kind != expectedKind || c.OwnerSystem != "assets" {
		return fmt.Errorf("invalid asset commit")
	}
	if c.Location.Provider == "" || validateObjectTarget(c.Location.Bucket, c.Location.Key) != nil {
		return fmt.Errorf("invalid asset object location")
	}
	if err := c.Event.Validate(); err != nil || c.Event.OrganizationID != c.OrganizationID || c.Event.ProjectID != c.ProjectID || c.Event.Subject.ID != string(c.AssetID) || c.Event.Subject.Version == nil || *c.Event.Subject.Version != c.Version {
		return fmt.Errorf("invalid asset event")
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO asset_blobs (id, organization_id, storage_provider, bucket_name, object_key, storage_version_id, etag, sha256, size_bytes, mime_type, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'ready', ?)`,
		c.BlobID, c.OrganizationID, c.Location.Provider, c.Location.Bucket, c.Location.Key, nullable(c.Location.VersionID), nullable(c.Location.ETag), c.SHA256, c.SizeBytes, c.MIMEType, now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO assets (id, organization_id, asset_kind, status, owner_system, latest_version, created_at, updated_at) VALUES (?, ?, ?, 'ready', ?, ?, ?, ?)`, c.AssetID, c.OrganizationID, c.Kind, c.OwnerSystem, c.Version, now, now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO asset_versions
		(organization_id, asset_id, version, blob_id, status, source_type, mime_type, size_bytes, sha256,
		 width_pixels, height_pixels, duration_ms, frame_rate, video_codec, audio_codec, render_job_id, derivation_id,
		 duration_seconds, fps, codec, bitrate_bps, audio_channels, audio_sample_rate, poster_frame_ref,
		 probe_status, probe_error, provider_job_id, provider_output_id, project_context_version, created_at)
		VALUES (?, ?, ?, ?, 'ready', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.OrganizationID, c.AssetID, c.Version, c.BlobID, c.SourceType, c.MIMEType, c.SizeBytes, c.SHA256,
		nullableInt(c.WidthPixels), nullableInt(c.HeightPixels), nullableInt64(c.DurationMS), nullable(c.FrameRate),
		nullable(c.VideoCodec), nullable(c.AudioCodec), nullable(c.RenderJobID), nullable(c.DerivationID), nullableFloat(c.Media.DurationSeconds),
		nullableFloat(c.Media.FPS), nullable(c.Media.Codec), nullableInt64(c.Media.BitrateBPS),
		nullableInt(c.Media.AudioChannels), nullableInt(c.Media.AudioSampleRate), nullable(c.Media.PosterFrameRef),
		probeStatusValue(c.Media.ProbeStatus), nullable(c.Media.ProbeError), nullable(c.ProviderJobID),
		nullable(c.ProviderOutputID), nullableInt64(c.ProjectContextVersion), now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO project_assets (organization_id, project_id, asset_id, asset_version, status, created_at) VALUES (?, ?, ?, ?, 'active', ?)`, c.OrganizationID, c.ProjectID, c.AssetID, c.Version, now)
	if err != nil {
		return err
	}
	for _, relation := range c.Relations {
		relation.CreatedAt = now
		if err := relation.Validate(); err != nil {
			return err
		}
		if relation.OrganizationID != c.OrganizationID || relation.ProjectID != c.ProjectID || relation.OutputAsset.AssetID != c.AssetID || relation.OutputAsset.Version != c.Version {
			return fmt.Errorf("asset relation does not match committed output")
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO asset_relations
			(organization_id, project_id, output_asset_id, output_asset_version, relation_type, source_type, source_id, source_version, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			relation.OrganizationID, relation.ProjectID, relation.OutputAsset.AssetID, relation.OutputAsset.Version,
			relation.RelationType, relation.Source.Type, relation.Source.ID, sourceVersionValue(relation.Source.Version), now)
		if err != nil {
			return err
		}
	}
	payload, err := json.Marshal(c.Event)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO assets_outbox (event_id, organization_id, project_id, event_type, aggregate_type, aggregate_id, aggregate_version, payload, created_at) VALUES (?, ?, ?, ?, 'asset', ?, ?, ?, ?)`,
		c.Event.EventID, c.OrganizationID, c.ProjectID, c.Event.EventType, c.AssetID, c.Version, payload, now)
	return err
}

func (r MySQLRepository) GetProjectAsset(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, ref contract.AssetVersionRef) (ProjectAsset, error) {
	if _, err := r.db(); err != nil {
		return ProjectAsset{}, err
	}
	return scanProjectAsset(r.DB.QueryRowContext(ctx, projectAssetSelect+` WHERE pa.organization_id=? AND pa.project_id=? AND pa.asset_id=? AND pa.asset_version=? AND pa.status='active'`, org, project, ref.AssetID, ref.Version))
}

func (r MySQLRepository) ListProjectAssets(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, limit int) ([]ProjectAsset, error) {
	if _, err := r.db(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := r.DB.QueryContext(ctx, projectAssetSelect+` WHERE pa.organization_id=? AND pa.project_id=? AND pa.status='active' ORDER BY pa.created_at DESC LIMIT ?`, org, project, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ProjectAsset, 0)
	for rows.Next() {
		value, err := scanProjectAsset(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (r MySQLRepository) RemoveProjectAsset(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, ref contract.AssetVersionRef) error {
	if _, err := r.db(); err != nil {
		return err
	}
	_, err := r.DB.ExecContext(ctx, `UPDATE project_assets SET status='removed'
		WHERE organization_id=? AND project_id=? AND asset_id=? AND asset_version=? AND status<>'removed'`,
		org, project, ref.AssetID, ref.Version)
	return err
}

func (r MySQLRepository) ListAssetRelations(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, ref contract.AssetVersionRef) ([]AssetRelation, error) {
	if _, err := r.db(); err != nil {
		return nil, err
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT organization_id, project_id, output_asset_id, output_asset_version,
		relation_type, source_type, source_id, source_version, created_at
		FROM asset_relations
		WHERE organization_id=? AND project_id=? AND output_asset_id=? AND output_asset_version=?
		ORDER BY created_at, source_type, source_id`, org, project, ref.AssetID, ref.Version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	relations := make([]AssetRelation, 0)
	for rows.Next() {
		var relation AssetRelation
		var sourceVersion int64
		if err := rows.Scan(&relation.OrganizationID, &relation.ProjectID, &relation.OutputAsset.AssetID, &relation.OutputAsset.Version,
			&relation.RelationType, &relation.Source.Type, &relation.Source.ID, &sourceVersion, &relation.CreatedAt); err != nil {
			return nil, err
		}
		if sourceVersion > 0 {
			version := sourceVersion
			relation.Source.Version = &version
		}
		relations = append(relations, relation)
	}
	return relations, rows.Err()
}

func (r MySQLRepository) UpsertAssetFeature(ctx context.Context, value AssetFeature, now time.Time) (AssetFeature, error) {
	if _, err := r.db(); err != nil {
		return AssetFeature{}, err
	}
	if err := value.Validate(); err != nil {
		return AssetFeature{}, err
	}
	sceneTags, err := json.Marshal(value.SceneTags)
	if err != nil {
		return AssetFeature{}, err
	}
	productTags, err := json.Marshal(value.ProductTags)
	if err != nil {
		return AssetFeature{}, err
	}
	personTags, err := json.Marshal(value.PersonTags)
	if err != nil {
		return AssetFeature{}, err
	}
	actionTags, err := json.Marshal(value.ActionTags)
	if err != nil {
		return AssetFeature{}, err
	}
	emotionTags, err := json.Marshal(value.EmotionTags)
	if err != nil {
		return AssetFeature{}, err
	}
	sellingPoints, err := json.Marshal(value.SellingPoints)
	if err != nil {
		return AssetFeature{}, err
	}
	evidence, err := json.Marshal(value.Evidence)
	if err != nil {
		return AssetFeature{}, err
	}
	_, err = r.DB.ExecContext(ctx, `INSERT INTO asset_features
		(organization_id, project_id, asset_id, asset_version, schema_version, feature_version,
		 hook_strength, product_visibility, scene_tags, product_tags, person_tags, action_tags,
		 emotion_tags, selling_points, cta_presence, similarity_group, similarity_risk, evidence,
		 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE schema_version=VALUES(schema_version), hook_strength=VALUES(hook_strength),
		 product_visibility=VALUES(product_visibility), scene_tags=VALUES(scene_tags), product_tags=VALUES(product_tags),
		 person_tags=VALUES(person_tags), action_tags=VALUES(action_tags), emotion_tags=VALUES(emotion_tags),
		 selling_points=VALUES(selling_points), cta_presence=VALUES(cta_presence), similarity_group=VALUES(similarity_group),
		 similarity_risk=VALUES(similarity_risk), evidence=VALUES(evidence), updated_at=VALUES(updated_at)`,
		value.OrganizationID, value.ProjectID, value.AssetID, value.AssetVersion, value.SchemaVersion, value.FeatureVersion,
		value.HookStrength, value.ProductVisibility, sceneTags, productTags, personTags, actionTags, emotionTags,
		sellingPoints, value.CTAPresence, nullable(value.SimilarityGroup), value.SimilarityRisk, evidence, now, now)
	if err != nil {
		return AssetFeature{}, err
	}
	return r.GetAssetFeature(ctx, value.OrganizationID, value.ProjectID, value.Ref(), value.FeatureVersion)
}

func (r MySQLRepository) GetAssetFeature(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, ref contract.AssetVersionRef, featureVersion string) (AssetFeature, error) {
	if _, err := r.db(); err != nil {
		return AssetFeature{}, err
	}
	return scanAssetFeature(r.DB.QueryRowContext(ctx, assetFeatureSelect+` WHERE organization_id=? AND project_id=? AND asset_id=? AND asset_version=? AND feature_version=?`,
		org, project, ref.AssetID, ref.Version, featureVersion))
}

func (r MySQLRepository) ListAssetFeatures(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, limit int) ([]AssetFeature, error) {
	if _, err := r.db(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 200 {
		limit = 100
	}
	rows, err := r.DB.QueryContext(ctx, assetFeatureSelect+` WHERE organization_id=? AND project_id=? ORDER BY updated_at DESC LIMIT ?`, org, project, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AssetFeature, 0)
	for rows.Next() {
		value, err := scanAssetFeature(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

const uploadSelect = `SELECT id, organization_id, project_id, principal_kind, principal_id, status, original_filename, declared_mime_type, declared_size_bytes, declared_sha256, quarantine_provider, quarantine_bucket, quarantine_object_key, idempotency_key, request_hash, project_context_version, target_asset_id, target_blob_id, request_id, trace_id, asset_id, asset_version, error_code, expires_at, created_at, updated_at FROM upload_sessions`
const intakeSelect = `SELECT id, organization_id, project_id, provider_job_id, output_id, provider_code, status, request_payload, idempotency_key, request_hash, target_asset_id, target_blob_id, asset_id, asset_version, error_code, error_message, retryable, attempt_count, max_attempts, available_at, lock_owner, request_id, trace_id, created_at, updated_at FROM generated_intakes`
const projectAssetSelect = `SELECT pa.project_id, pa.created_at, a.id, a.organization_id, a.asset_kind, a.status, a.owner_system, a.latest_version, a.created_at, a.updated_at, av.version, av.status, av.source_type, av.mime_type, av.size_bytes, av.sha256, av.width_pixels, av.height_pixels, av.duration_ms, av.frame_rate, av.video_codec, av.audio_codec, av.render_job_id, av.derivation_id, av.duration_seconds, av.fps, av.codec, av.bitrate_bps, av.audio_codec, av.audio_channels, av.audio_sample_rate, av.poster_frame_ref, av.probe_status, av.probe_error, av.provider_job_id, av.provider_output_id, av.project_context_version, av.created_at, b.storage_provider, b.bucket_name, b.object_key, b.storage_version_id, b.etag FROM project_assets pa JOIN assets a ON a.organization_id=pa.organization_id AND a.id=pa.asset_id JOIN asset_versions av ON av.organization_id=pa.organization_id AND av.asset_id=pa.asset_id AND av.version=pa.asset_version JOIN asset_blobs b ON b.id=av.blob_id`
const assetFeatureSelect = `SELECT organization_id, project_id, asset_id, asset_version, schema_version, feature_version, hook_strength, product_visibility, scene_tags, product_tags, person_tags, action_tags, emotion_tags, selling_points, cta_presence, similarity_group, similarity_risk, evidence, created_at, updated_at FROM asset_features`

type scanner interface{ Scan(...any) error }

func scanUpload(row scanner) (UploadSession, error) {
	var v UploadSession
	var sha, assetID, errorCode sql.NullString
	var version sql.NullInt64
	err := row.Scan(&v.ID, &v.OrganizationID, &v.ProjectID, &v.Principal.Kind, &v.Principal.ID, &v.Status, &v.Filename, &v.DeclaredMIMEType, &v.DeclaredSizeBytes, &sha, &v.Quarantine.Provider, &v.Quarantine.Bucket, &v.Quarantine.Key, &v.IdempotencyKey, &v.RequestHash, &v.ProjectContextVersion, &v.TargetAssetID, &v.TargetBlobID, &v.RequestID, &v.TraceID, &assetID, &version, &errorCode, &v.ExpiresAt, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return UploadSession{}, ErrNotFound
	}
	if err != nil {
		return UploadSession{}, err
	}
	if sha.Valid {
		v.DeclaredSHA256 = &sha.String
	}
	if errorCode.Valid {
		v.ErrorCode = errorCode.String
	}
	if assetID.Valid && version.Valid {
		ref := contract.ProjectAssetRef{ProjectID: v.ProjectID, AssetVersion: contract.AssetVersionRef{AssetID: contract.AssetID(assetID.String), Version: version.Int64}}
		v.ProjectAssetRef = &ref
	}
	return v, nil
}
func scanIntake(row scanner) (GeneratedIntake, error) {
	var v GeneratedIntake
	var payload []byte
	var assetID, errorCode, errorMessage, lockOwner sql.NullString
	var version sql.NullInt64
	var retryable sql.NullBool
	err := row.Scan(&v.ID, &v.OrganizationID, &v.ProjectID, &v.ProviderJobID, &v.OutputID, &v.ProviderCode, &v.Status, &payload, &v.IdempotencyKey, &v.RequestHash, &v.TargetAssetID, &v.TargetBlobID, &assetID, &version, &errorCode, &errorMessage, &retryable, &v.AttemptCount, &v.MaxAttempts, &v.AvailableAt, &lockOwner, &v.RequestID, &v.TraceID, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GeneratedIntake{}, ErrNotFound
	}
	if err != nil {
		return GeneratedIntake{}, err
	}
	if err := json.Unmarshal(payload, &v.Request); err != nil {
		return GeneratedIntake{}, err
	}
	v.LockOwner = lockOwner.String
	if assetID.Valid && version.Valid {
		ref := contract.ProjectAssetRef{ProjectID: v.ProjectID, AssetVersion: contract.AssetVersionRef{AssetID: contract.AssetID(assetID.String), Version: version.Int64}}
		v.ProjectAssetRef = &ref
	}
	if errorCode.Valid {
		v.Error = &contract.JobError{Code: errorCode.String, Message: errorMessage.String, Retryable: retryable.Bool}
	}
	return v, nil
}
func scanProjectAsset(row scanner) (ProjectAsset, error) {
	var v ProjectAsset
	var width, height, duration, bitrate, channels, sampleRate sql.NullInt64
	var mediaDuration, fps sql.NullFloat64
	var frameRate, videoCodec, audioCodec, renderJob, derivationID, codec, mediaAudioCodec, posterFrame, probeStatus, probeError, job, output, versionID, etag sql.NullString
	var contextVersion sql.NullInt64
	err := row.Scan(&v.Ref.ProjectID, &v.CreatedAt, &v.Asset.ID, &v.Asset.OrganizationID, &v.Asset.Kind, &v.Asset.Status, &v.Asset.OwnerSystem, &v.Asset.LatestVersion, &v.Asset.CreatedAt, &v.Asset.UpdatedAt, &v.Version.Version, &v.Version.Status, &v.Version.SourceType, &v.Version.MIMEType, &v.Version.SizeBytes, &v.Version.SHA256, &width, &height, &duration, &frameRate, &videoCodec, &audioCodec, &renderJob, &derivationID, &mediaDuration, &fps, &codec, &bitrate, &mediaAudioCodec, &channels, &sampleRate, &posterFrame, &probeStatus, &probeError, &job, &output, &contextVersion, &v.Version.CreatedAt, &v.Version.Blob.Provider, &v.Version.Blob.Bucket, &v.Version.Blob.Key, &versionID, &etag)
	if errors.Is(err, sql.ErrNoRows) {
		return ProjectAsset{}, ErrNotFound
	}
	if err != nil {
		return ProjectAsset{}, err
	}
	v.Ref.AssetVersion = contract.AssetVersionRef{AssetID: v.Asset.ID, Version: v.Version.Version}
	v.Version.OrganizationID = v.Asset.OrganizationID
	v.Version.AssetID = v.Asset.ID
	v.Version.WidthPixels = int(width.Int64)
	v.Version.HeightPixels = int(height.Int64)
	v.Version.DurationMS = duration.Int64
	v.Version.FrameRate = frameRate.String
	v.Version.VideoCodec = videoCodec.String
	v.Version.AudioCodec = audioCodec.String
	v.Version.RenderJobID = renderJob.String
	v.Version.DerivationID = derivationID.String
	v.Version.Media = MediaMetadata{
		DurationSeconds: mediaDuration.Float64,
		FPS:             fps.Float64,
		Codec:           codec.String,
		BitrateBPS:      bitrate.Int64,
		AudioCodec:      mediaAudioCodec.String,
		AudioChannels:   int(channels.Int64),
		AudioSampleRate: int(sampleRate.Int64),
		PosterFrameRef:  posterFrame.String,
		ProbeStatus:     MediaProbeStatus(probeStatus.String),
		ProbeError:      probeError.String,
	}
	if v.Version.Media.ProbeStatus == "" {
		v.Version.Media.ProbeStatus = MediaProbeNotRequired
	}
	v.Version.ProviderJobID = job.String
	v.Version.ProviderOutputID = output.String
	v.Version.ProjectContextVersion = contextVersion.Int64
	v.Version.Blob.VersionID = versionID.String
	v.Version.Blob.ETag = etag.String
	return v, nil
}
func scanAssetFeature(row scanner) (AssetFeature, error) {
	var v AssetFeature
	var sceneTags, productTags, personTags, actionTags, emotionTags, sellingPoints, evidence []byte
	var similarityGroup sql.NullString
	err := row.Scan(&v.OrganizationID, &v.ProjectID, &v.AssetID, &v.AssetVersion, &v.SchemaVersion, &v.FeatureVersion,
		&v.HookStrength, &v.ProductVisibility, &sceneTags, &productTags, &personTags, &actionTags, &emotionTags,
		&sellingPoints, &v.CTAPresence, &similarityGroup, &v.SimilarityRisk, &evidence, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AssetFeature{}, ErrNotFound
	}
	if err != nil {
		return AssetFeature{}, err
	}
	if err := json.Unmarshal(sceneTags, &v.SceneTags); err != nil {
		return AssetFeature{}, err
	}
	if err := json.Unmarshal(productTags, &v.ProductTags); err != nil {
		return AssetFeature{}, err
	}
	if err := json.Unmarshal(personTags, &v.PersonTags); err != nil {
		return AssetFeature{}, err
	}
	if err := json.Unmarshal(actionTags, &v.ActionTags); err != nil {
		return AssetFeature{}, err
	}
	if err := json.Unmarshal(emotionTags, &v.EmotionTags); err != nil {
		return AssetFeature{}, err
	}
	if err := json.Unmarshal(sellingPoints, &v.SellingPoints); err != nil {
		return AssetFeature{}, err
	}
	if err := json.Unmarshal(evidence, &v.Evidence); err != nil {
		return AssetFeature{}, err
	}
	v.SimilarityGroup = similarityGroup.String
	return v, nil
}
func isDuplicate(err error) bool {
	var value *mysql.MySQLError
	return errors.As(err, &value) && value.Number == 1062
}
func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}
func nullableInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}
func sourceVersionValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
func nullableFloat(value float64) any {
	if value == 0 {
		return nil
	}
	return value
}
func probeStatusValue(value MediaProbeStatus) any {
	if value == "" {
		return MediaProbeNotRequired
	}
	return value
}
func validAssetKindForMIME(kind contract.AssetKind, mimeType string) bool {
	return (kind == contract.AssetImage && allowedDeclaredImageMIME(mimeType)) ||
		(kind == contract.AssetVideo && allowedDeclaredVideoMIME(mimeType))
}
