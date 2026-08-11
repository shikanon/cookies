package assets

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/ids"
)

type ActiveProjectResolver interface {
	RequireActiveContext(context.Context, contract.ActorContext, contract.ProjectID) (contract.ProjectContext, error)
}

type UploadService struct {
	Repository       Repository
	Projects         ActiveProjectResolver
	Blobs            BlobStore
	Scanner          ContentScanner
	MediaProbe       MediaProbe
	QuarantineBucket string
	AssetsBucket     string
	UploadTTL        time.Duration
	PreviewTTL       time.Duration
	Now              func() time.Time
	NewID            ids.Generator
	VideoProbe       VideoMetadataProbe
	AudioProbe       AudioMetadataProbe
	UsePolicy        AssetUseAuthorizer
}

func (s UploadService) Create(ctx context.Context, requestContext contract.RequestContext, projectID contract.ProjectID, key contract.IdempotencyKey, request CreateUploadRequest) (CreateUploadResponse, error) {
	if err := s.validateDependencies(); err != nil {
		return CreateUploadResponse{}, err
	}
	if err := key.Validate(); err != nil {
		return CreateUploadResponse{}, err
	}
	if err := request.Validate(); err != nil {
		return CreateUploadResponse{}, err
	}
	projectContext, err := s.Projects.RequireActiveContext(ctx, requestContext.Actor, projectID)
	if err != nil {
		return CreateUploadResponse{}, err
	}
	request.Filename = safeFilename(request.Filename)
	requestHash, err := contract.CanonicalJSONHash(request)
	if err != nil {
		return CreateUploadResponse{}, err
	}
	newID := s.idGenerator()
	sessionID, err := newID("upload")
	if err != nil {
		return CreateUploadResponse{}, err
	}
	assetID, err := newID("asset")
	if err != nil {
		return CreateUploadResponse{}, err
	}
	blobID, err := newID("blob")
	if err != nil {
		return CreateUploadResponse{}, err
	}
	now := s.now()
	session := UploadSession{
		ID: sessionID, OrganizationID: requestContext.Actor.OrganizationID, ProjectID: projectID,
		Principal: requestContext.Actor.Principal, Status: UploadCreated, Filename: request.Filename,
		DeclaredMIMEType: request.DeclaredMIMEType, DeclaredSizeBytes: request.DeclaredSizeBytes,
		DeclaredSHA256: request.DeclaredSHA256, Quarantine: ObjectLocation{Provider: s.BlobProvider(), Bucket: s.QuarantineBucket, Key: quarantineKey(requestContext.Actor.OrganizationID, sessionID)},
		IdempotencyKey: key, RequestHash: requestHash, ProjectContextVersion: projectContext.ProjectContextVersion,
		TargetAssetID: contract.AssetID(assetID), TargetBlobID: blobID, RequestID: requestContext.RequestID,
		TraceID: requestContext.TraceID, ExpiresAt: now.Add(s.uploadTTL()), CreatedAt: now, UpdatedAt: now,
	}
	stored, duplicate, err := s.Repository.CreateUpload(ctx, session)
	if err != nil {
		return CreateUploadResponse{}, err
	}
	if duplicate && stored.RequestHash != requestHash {
		return CreateUploadResponse{}, ErrIdempotencyConflict
	}
	if stored.Status != UploadCreated && stored.Status != UploadUploaded {
		return CreateUploadResponse{Session: stored}, nil
	}
	ttl := stored.ExpiresAt.Sub(s.now())
	if ttl <= 0 {
		return CreateUploadResponse{}, ErrInvalidState
	}
	signed, err := s.Blobs.SignPut(ctx, stored.Quarantine.Bucket, stored.Quarantine.Key, stored.DeclaredMIMEType, stored.DeclaredSizeBytes, ttl)
	if err != nil {
		return CreateUploadResponse{}, err
	}
	return CreateUploadResponse{Session: stored, Upload: &signed}, nil
}

// PutContent is a development and fallback proxy. Production clients should
// use the signed TOS PUT returned by Create.
func (s UploadService) PutContent(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, sessionID string, content io.Reader, size int64) error {
	if err := s.validateDependencies(); err != nil {
		return err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return err
	}
	session, err := s.Repository.GetUpload(ctx, actor.OrganizationID, projectID, sessionID)
	if err != nil {
		return err
	}
	if session.Principal != actor.Principal {
		return ErrNotFound
	}
	if session.Status != UploadCreated || s.now().After(session.ExpiresAt) {
		return ErrInvalidState
	}
	if size != session.DeclaredSizeBytes {
		return ErrOutputMetadataMismatch
	}
	if _, err := s.Blobs.Put(ctx, session.Quarantine.Bucket, session.Quarantine.Key, content, size, session.DeclaredMIMEType); err != nil {
		return err
	}
	return s.Repository.MarkUploadUploaded(ctx, actor.OrganizationID, projectID, sessionID, s.now())
}

func (s UploadService) Finalize(ctx context.Context, requestContext contract.RequestContext, projectID contract.ProjectID, sessionID string) (UploadSession, error) {
	if err := s.validateDependencies(); err != nil {
		return UploadSession{}, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, requestContext.Actor, projectID); err != nil {
		return UploadSession{}, err
	}
	session, err := s.Repository.GetUpload(ctx, requestContext.Actor.OrganizationID, projectID, sessionID)
	if err != nil {
		return UploadSession{}, err
	}
	if session.Principal != requestContext.Actor.Principal {
		return UploadSession{}, ErrNotFound
	}
	if session.Status == UploadSucceeded {
		return session, nil
	}
	if s.now().After(session.ExpiresAt) {
		_ = s.Repository.FailUpload(ctx, session.OrganizationID, session.ProjectID, session.ID, "UPLOAD_EXPIRED", s.now())
		return UploadSession{}, ErrInvalidState
	}
	if session.Status != UploadCreated && session.Status != UploadUploaded && session.Status != UploadProcessing {
		return UploadSession{}, ErrInvalidState
	}
	if session.Status != UploadProcessing {
		if err := s.Repository.MarkUploadProcessing(ctx, session.OrganizationID, session.ProjectID, session.ID, s.now()); err != nil {
			return UploadSession{}, err
		}
	}
	commit, err := s.ingestStoredObject(ctx, session.OrganizationID, session.ProjectID, session.TargetAssetID,
		session.TargetBlobID, session.ProjectContextVersion, contract.AssetSourceUpload, session.Quarantine,
		"", "", "", session.TraceID)
	if err != nil {
		code := "ASSET_INTAKE_FAILED"
		if errors.Is(err, ErrMalwareDetected) {
			code = contract.ErrorAssetQuarantined
		}
		if errors.Is(err, ErrMalwareDetected) || errors.Is(err, ErrInvalidAssetContent) {
			_ = s.Repository.FailUpload(ctx, session.OrganizationID, session.ProjectID, session.ID, code, s.now())
		}
		return UploadSession{}, err
	}
	if commit.MIMEType != session.DeclaredMIMEType || commit.SizeBytes != session.DeclaredSizeBytes {
		_ = s.Blobs.Delete(ctx, commit.Location)
		_ = s.Repository.FailUpload(ctx, session.OrganizationID, session.ProjectID, session.ID, "OUTPUT_METADATA_MISMATCH", s.now())
		return UploadSession{}, ErrOutputMetadataMismatch
	}
	if session.DeclaredSHA256 != nil && commit.SHA256 != *session.DeclaredSHA256 {
		_ = s.Blobs.Delete(ctx, commit.Location)
		_ = s.Repository.FailUpload(ctx, session.OrganizationID, session.ProjectID, session.ID, "ASSET_CHECKSUM_MISMATCH", s.now())
		return UploadSession{}, ErrAssetChecksumMismatch
	}
	ref, err := s.Repository.CompleteUpload(ctx, session.ID, commit, s.now())
	if err != nil {
		_ = s.Blobs.Delete(ctx, commit.Location)
		return UploadSession{}, err
	}
	_ = s.Blobs.Delete(ctx, session.Quarantine)
	session.Status = UploadSucceeded
	session.ProjectAssetRef = &ref
	session.UpdatedAt = s.now()
	return session, nil
}

func (s UploadService) List(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, limit int) ([]ProjectAsset, error) {
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	items, err := s.Repository.ListProjectAssets(ctx, actor.OrganizationID, projectID, limit)
	if err != nil || s.UsePolicy == nil {
		return items, err
	}
	for index := range items {
		decision, _ := s.UsePolicy.Authorize(ctx, AssetUseRequest{OrganizationID: actor.OrganizationID, ProjectID: projectID, AssetRef: items[index].Version.Ref(), Purpose: AssetUsePreview})
		items[index].UseDecision = &decision
	}
	return items, nil
}

func (s UploadService) Get(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, ref contract.AssetVersionRef) (ProjectAsset, error) {
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ProjectAsset{}, err
	}
	if err := ref.Validate(); err != nil {
		return ProjectAsset{}, err
	}
	return s.Repository.GetProjectAsset(ctx, actor.OrganizationID, projectID, ref)
}

func (s UploadService) Preview(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, ref contract.AssetVersionRef) (SignedRequest, error) {
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return SignedRequest{}, err
	}
	if err := ref.Validate(); err != nil {
		return SignedRequest{}, err
	}
	asset, err := s.Repository.GetProjectAsset(ctx, actor.OrganizationID, projectID, ref)
	if err != nil {
		return SignedRequest{}, err
	}
	if asset.Version.Status != AssetReady {
		return SignedRequest{}, ErrAssetNotReady
	}
	if err := s.authorizeAssetUse(ctx, actor, projectID, ref, AssetUsePreview); err != nil {
		return SignedRequest{}, err
	}
	return s.Blobs.SignGet(ctx, asset.Version.Blob, s.previewTTL())
}

// OpenPreview re-authorizes the project relationship before streaming local
// content. It is used only when the configured blob store cannot issue an
// externally reachable signed URL.
func (s UploadService) OpenPreview(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, ref contract.AssetVersionRef) (io.ReadCloser, ObjectInfo, error) {
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return nil, ObjectInfo{}, err
	}
	if err := ref.Validate(); err != nil {
		return nil, ObjectInfo{}, err
	}
	asset, err := s.Repository.GetProjectAsset(ctx, actor.OrganizationID, projectID, ref)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	if asset.Version.Status != AssetReady {
		return nil, ObjectInfo{}, ErrAssetNotReady
	}
	if err := s.authorizeAssetUse(ctx, actor, projectID, ref, AssetUsePreview); err != nil {
		return nil, ObjectInfo{}, err
	}
	reader, info, err := s.Blobs.Open(ctx, asset.Version.Blob)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	if info.SizeBytes != asset.Version.SizeBytes {
		reader.Close()
		return nil, ObjectInfo{}, ErrOutputMetadataMismatch
	}
	info.MIMEType = asset.Version.MIMEType
	return reader, info, nil
}

func (s UploadService) authorizeAssetUse(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, ref contract.AssetVersionRef, purpose AssetUsePurpose) error {
	if s.UsePolicy == nil {
		return nil
	}
	_, err := s.UsePolicy.Authorize(ctx, AssetUseRequest{OrganizationID: actor.OrganizationID, ProjectID: projectID, AssetRef: ref, Purpose: purpose})
	return err
}

func (s UploadService) Remove(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, ref contract.AssetVersionRef) error {
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return err
	}
	if err := ref.Validate(); err != nil {
		return err
	}
	return s.Repository.RemoveProjectAsset(ctx, actor.OrganizationID, projectID, ref)
}

// IngestRenderedVideo is the Assets-owned boundary used by a media renderer.
// The render job remains Creative-owned; Assets validates and persists a new
// immutable video version and makes retries idempotent by render_job_id.
func (s UploadService) IngestRenderedVideo(ctx context.Context, requestContext contract.RequestContext, projectID contract.ProjectID, renderJobID string, content io.Reader, sizeBytes int64) (contract.ProjectAssetRef, error) {
	return s.ingestRenderedVideo(ctx, requestContext, projectID, renderJobID, nil, content, sizeBytes)
}

// IngestRenderedVideoWithSources records immutable input resources alongside
// the rendered AssetVersion. It is used by timeline renderers whose outputs
// must remain traceable to exact source assets and a frozen timeline version.
func (s UploadService) IngestRenderedVideoWithSources(ctx context.Context, requestContext contract.RequestContext, projectID contract.ProjectID, renderJobID string, sources []contract.ResourceRef, content io.Reader, sizeBytes int64) (contract.ProjectAssetRef, error) {
	return s.ingestRenderedVideo(ctx, requestContext, projectID, renderJobID, sources, content, sizeBytes)
}

func (s UploadService) ingestRenderedVideo(ctx context.Context, requestContext contract.RequestContext, projectID contract.ProjectID, renderJobID string, sources []contract.ResourceRef, content io.Reader, sizeBytes int64) (contract.ProjectAssetRef, error) {
	if err := s.validateDependencies(); err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if err := requestContext.Validate(); err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if !requestContext.Actor.HasScope("assets.write") {
		return contract.ProjectAssetRef{}, fmt.Errorf("assets.write scope is required")
	}
	if strings.TrimSpace(renderJobID) == "" || len(renderJobID) > 96 || content == nil || sizeBytes < 1 || sizeBytes > MaxVideoBytes {
		return contract.ProjectAssetRef{}, fmt.Errorf("render_job_id and supported video content are required")
	}
	project, err := s.Projects.RequireActiveContext(ctx, requestContext.Actor, projectID)
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	newID := s.idGenerator()
	assetIDValue, err := newID("asset")
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	blobID, err := newID("blob")
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	stagingID, err := newID("renderstage")
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	staged, err := s.Blobs.Put(ctx, s.QuarantineBucket, quarantineKey(requestContext.Actor.OrganizationID, stagingID), io.LimitReader(content, MaxVideoBytes+1), sizeBytes, "video/mp4")
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	defer s.Blobs.Delete(ctx, staged.ObjectLocation)
	commit, err := s.ingestStoredObject(ctx, requestContext.Actor.OrganizationID, projectID, contract.AssetID(assetIDValue), blobID,
		project.ProjectContextVersion, contract.AssetSourceRendered, staged.ObjectLocation, "", "", renderJobID, requestContext.TraceID)
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	for _, source := range sources {
		if err := source.Validate(); err != nil {
			return contract.ProjectAssetRef{}, fmt.Errorf("rendered video source: %w", err)
		}
		commit.Relations = append(commit.Relations, AssetRelation{
			OrganizationID: requestContext.Actor.OrganizationID,
			ProjectID:      projectID,
			OutputAsset:    contract.AssetVersionRef{AssetID: commit.AssetID, Version: commit.Version},
			RelationType:   AssetRelationDerivedFrom,
			Source:         source,
		})
	}
	ref, err := s.Repository.CompleteRender(ctx, renderJobID, commit, s.now())
	if err != nil {
		_ = s.Blobs.Delete(ctx, commit.Location)
		return contract.ProjectAssetRef{}, err
	}
	if ref.AssetVersion.AssetID != commit.AssetID || ref.AssetVersion.Version != commit.Version {
		_ = s.Blobs.Delete(ctx, commit.Location)
	}
	return ref, nil
}

// IngestDerivedImage persists a processor-produced image as an immutable Asset
// and records its exact source AssetVersion. derivationID is the stable retry
// identity (source version + processor version + parameters), not a filename.
func (s UploadService) IngestDerivedImage(ctx context.Context, requestContext contract.RequestContext, projectID contract.ProjectID, derivationID string, source contract.AssetVersionRef, content io.Reader, sizeBytes int64, mimeType string) (contract.ProjectAssetRef, error) {
	if err := s.validateDependencies(); err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if err := requestContext.Validate(); err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if !requestContext.Actor.HasScope("assets.write") {
		return contract.ProjectAssetRef{}, fmt.Errorf("assets.write scope is required")
	}
	if strings.TrimSpace(derivationID) == "" || len(derivationID) > 128 || content == nil || sizeBytes < 1 || sizeBytes > MaxImageBytes || !allowedDeclaredImageMIME(mimeType) {
		return contract.ProjectAssetRef{}, fmt.Errorf("derivation_id and supported image content are required")
	}
	if err := source.Validate(); err != nil {
		return contract.ProjectAssetRef{}, fmt.Errorf("source asset: %w", err)
	}
	project, err := s.Projects.RequireActiveContext(ctx, requestContext.Actor, projectID)
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	sourceAsset, err := s.Repository.GetProjectAsset(ctx, requestContext.Actor.OrganizationID, projectID, source)
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if sourceAsset.Asset.Status != AssetReady || sourceAsset.Asset.Kind != contract.AssetVideo {
		return contract.ProjectAssetRef{}, fmt.Errorf("%w: derived frame source must be a ready video", ErrUnsupportedAsset)
	}
	newID := s.idGenerator()
	assetIDValue, err := newID("asset")
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	blobID, err := newID("blob")
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	stagingID, err := newID("derivestage")
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	staged, err := s.Blobs.Put(ctx, s.QuarantineBucket, quarantineKey(requestContext.Actor.OrganizationID, stagingID), io.LimitReader(content, MaxImageBytes+1), sizeBytes, mimeType)
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	defer s.Blobs.Delete(ctx, staged.ObjectLocation)
	commit, err := s.ingestStoredObject(ctx, requestContext.Actor.OrganizationID, projectID, contract.AssetID(assetIDValue), blobID,
		project.ProjectContextVersion, contract.AssetSourceDerived, staged.ObjectLocation, "", "", "", requestContext.TraceID)
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	commit.DerivationID = derivationID
	commit.Event.Data, err = json.Marshal(contract.AssetReadyData{
		AssetKind: commit.Kind, MIMEType: commit.MIMEType, SizeBytes: commit.SizeBytes,
		SourceType: contract.AssetSourceDerived, DerivationID: derivationID,
	})
	if err != nil {
		_ = s.Blobs.Delete(ctx, commit.Location)
		return contract.ProjectAssetRef{}, err
	}
	commit.Relations = []AssetRelation{{
		OrganizationID: requestContext.Actor.OrganizationID,
		ProjectID:      projectID,
		OutputAsset:    contract.AssetVersionRef{AssetID: commit.AssetID, Version: commit.Version},
		RelationType:   AssetRelationDerivedFrom,
		Source:         contract.ResourceRef{Type: "asset_version", ID: string(source.AssetID), Version: &source.Version},
	}}
	ref, err := s.Repository.CompleteDerived(ctx, derivationID, commit, s.now())
	if err != nil {
		_ = s.Blobs.Delete(ctx, commit.Location)
		return contract.ProjectAssetRef{}, err
	}
	if ref.AssetVersion.AssetID != commit.AssetID || ref.AssetVersion.Version != commit.Version {
		_ = s.Blobs.Delete(ctx, commit.Location)
	}
	return ref, nil
}

// IngestDerivedAudio persists processor- or fixture-produced audio with stable
// derivation identity and Creative resource lineage. The bytes pass through
// the same scanner, MIME sniffing, FFprobe, and project authorization gates as
// uploaded and provider-generated assets.
func (s UploadService) IngestDerivedAudio(ctx context.Context, requestContext contract.RequestContext, projectID contract.ProjectID, derivationID string, content io.Reader, sizeBytes int64, mimeType string, sourceResources []contract.ResourceRef) (contract.ProjectAssetRef, error) {
	if err := s.validateDependencies(); err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if err := requestContext.Validate(); err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if !requestContext.Actor.HasScope("assets.write") {
		return contract.ProjectAssetRef{}, fmt.Errorf("assets.write scope is required")
	}
	if strings.TrimSpace(derivationID) == "" || len(derivationID) > 128 || content == nil || sizeBytes < 1 || sizeBytes > MaxAudioBytes || !allowedDeclaredAudioMIME(mimeType) || len(sourceResources) == 0 {
		return contract.ProjectAssetRef{}, fmt.Errorf("derivation_id, lineage, and supported audio content are required")
	}
	for index, source := range sourceResources {
		if err := source.Validate(); err != nil {
			return contract.ProjectAssetRef{}, fmt.Errorf("source resource %d: %w", index, err)
		}
	}
	project, err := s.Projects.RequireActiveContext(ctx, requestContext.Actor, projectID)
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	newID := s.idGenerator()
	assetIDValue, err := newID("asset")
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	blobID, err := newID("blob")
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	stagingID, err := newID("audiostage")
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	staged, err := s.Blobs.Put(ctx, s.QuarantineBucket, quarantineKey(requestContext.Actor.OrganizationID, stagingID), io.LimitReader(content, MaxAudioBytes+1), sizeBytes, mimeType)
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	defer s.Blobs.Delete(ctx, staged.ObjectLocation)
	commit, err := s.ingestStoredObject(ctx, requestContext.Actor.OrganizationID, projectID, contract.AssetID(assetIDValue), blobID,
		project.ProjectContextVersion, contract.AssetSourceDerived, staged.ObjectLocation, "", "", "", requestContext.TraceID)
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	commit.DerivationID = derivationID
	commit.Event.Data, err = json.Marshal(contract.AssetReadyData{
		AssetKind: commit.Kind, MIMEType: commit.MIMEType, SizeBytes: commit.SizeBytes,
		SourceType: contract.AssetSourceDerived, DerivationID: derivationID,
	})
	if err != nil {
		_ = s.Blobs.Delete(ctx, commit.Location)
		return contract.ProjectAssetRef{}, err
	}
	for _, source := range sourceResources {
		commit.Relations = append(commit.Relations, AssetRelation{
			OrganizationID: requestContext.Actor.OrganizationID,
			ProjectID:      projectID,
			OutputAsset:    contract.AssetVersionRef{AssetID: commit.AssetID, Version: commit.Version},
			RelationType:   AssetRelationDerivedFrom,
			Source:         source,
		})
	}
	ref, err := s.Repository.CompleteDerived(ctx, derivationID, commit, s.now())
	if err != nil {
		_ = s.Blobs.Delete(ctx, commit.Location)
		return contract.ProjectAssetRef{}, err
	}
	if ref.AssetVersion.AssetID != commit.AssetID || ref.AssetVersion.Version != commit.Version {
		_ = s.Blobs.Delete(ctx, commit.Location)
	}
	return ref, nil
}

// IngestRenderedImage is the Assets-owned boundary for deterministic Creative
// image composition. The caller supplies only stable lineage references;
// Assets validates the bytes and persists the immutable rendered version.
func (s UploadService) IngestRenderedImage(
	ctx context.Context,
	requestContext contract.RequestContext,
	projectID contract.ProjectID,
	renderJobID string,
	content io.Reader,
	sizeBytes int64,
	expectedWidth int,
	expectedHeight int,
	sourceAssets []contract.AssetVersionRef,
	sourceResources []contract.ResourceRef,
) (contract.ProjectAssetRef, error) {
	if err := s.validateDependencies(); err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if err := requestContext.Validate(); err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if !requestContext.Actor.HasScope("assets.write") {
		return contract.ProjectAssetRef{}, fmt.Errorf("assets.write scope is required")
	}
	if strings.TrimSpace(renderJobID) == "" || len(renderJobID) > 96 || expectedWidth < 2 || expectedHeight < 2 ||
		content == nil || sizeBytes < 1 || sizeBytes > MaxImageBytes {
		return contract.ProjectAssetRef{}, fmt.Errorf("render_job_id and supported image content are required")
	}
	for _, ref := range sourceAssets {
		if err := ref.Validate(); err != nil {
			return contract.ProjectAssetRef{}, fmt.Errorf("rendered image source asset: %w", err)
		}
	}
	for _, ref := range sourceResources {
		if err := ref.Validate(); err != nil {
			return contract.ProjectAssetRef{}, fmt.Errorf("rendered image source resource: %w", err)
		}
	}
	project, err := s.Projects.RequireActiveContext(ctx, requestContext.Actor, projectID)
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	newID := s.idGenerator()
	assetIDValue, err := newID("asset")
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	blobID, err := newID("blob")
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	stagingID, err := newID("renderstage")
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	staged, err := s.Blobs.Put(
		ctx, s.QuarantineBucket,
		quarantineKey(requestContext.Actor.OrganizationID, stagingID),
		io.LimitReader(content, MaxImageBytes+1), sizeBytes, "image/png",
	)
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	defer s.Blobs.Delete(ctx, staged.ObjectLocation)
	commit, err := s.ingestStoredObject(
		ctx, requestContext.Actor.OrganizationID, projectID, contract.AssetID(assetIDValue), blobID,
		project.ProjectContextVersion, contract.AssetSourceRendered, staged.ObjectLocation,
		"", "", renderJobID, requestContext.TraceID,
	)
	if err != nil {
		return contract.ProjectAssetRef{}, err
	}
	if commit.Kind != contract.AssetImage || commit.MIMEType != "image/png" ||
		commit.WidthPixels != expectedWidth || commit.HeightPixels != expectedHeight {
		_ = s.Blobs.Delete(ctx, commit.Location)
		return contract.ProjectAssetRef{}, fmt.Errorf("%w: rendered image must be a %dx%d PNG", ErrOutputMetadataMismatch, expectedWidth, expectedHeight)
	}
	outputRef := contract.AssetVersionRef{AssetID: commit.AssetID, Version: commit.Version}
	for _, ref := range sourceAssets {
		version := ref.Version
		commit.Relations = append(commit.Relations, AssetRelation{
			OrganizationID: commit.OrganizationID, ProjectID: projectID, OutputAsset: outputRef,
			RelationType: AssetRelationGeneratedFrom,
			Source:       contract.ResourceRef{Type: "asset_version", ID: string(ref.AssetID), Version: &version},
		})
	}
	for _, ref := range sourceResources {
		commit.Relations = append(commit.Relations, AssetRelation{
			OrganizationID: commit.OrganizationID, ProjectID: projectID, OutputAsset: outputRef,
			RelationType: AssetRelationGeneratedFrom, Source: ref,
		})
	}
	ref, err := s.Repository.CompleteRender(ctx, renderJobID, commit, s.now())
	if err != nil {
		_ = s.Blobs.Delete(ctx, commit.Location)
		return contract.ProjectAssetRef{}, err
	}
	if ref.AssetVersion != outputRef {
		_ = s.Blobs.Delete(ctx, commit.Location)
	}
	return ref, nil
}

func (s UploadService) UpsertFeature(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, feature AssetFeature) (AssetFeature, error) {
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return AssetFeature{}, err
	}
	feature.OrganizationID = actor.OrganizationID
	feature.ProjectID = projectID
	if err := feature.Validate(); err != nil {
		return AssetFeature{}, err
	}
	if _, err := s.Repository.GetProjectAsset(ctx, actor.OrganizationID, projectID, feature.Ref()); err != nil {
		return AssetFeature{}, err
	}
	return s.Repository.UpsertAssetFeature(ctx, feature, s.now())
}

func (s UploadService) GetFeature(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, ref contract.AssetVersionRef, featureVersion string) (AssetFeature, error) {
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return AssetFeature{}, err
	}
	if err := ref.Validate(); err != nil {
		return AssetFeature{}, err
	}
	if strings.TrimSpace(featureVersion) == "" {
		return AssetFeature{}, fmt.Errorf("feature_version is required")
	}
	return s.Repository.GetAssetFeature(ctx, actor.OrganizationID, projectID, ref, featureVersion)
}

func (s UploadService) ListFeatures(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, limit int) ([]AssetFeature, error) {
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	return s.Repository.ListAssetFeatures(ctx, actor.OrganizationID, projectID, limit)
}

func (s UploadService) ingestStoredObject(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, assetID contract.AssetID, blobID string, projectContextVersion int64, source contract.AssetSourceType, sourceLocation ObjectLocation, providerJobID, providerOutputID, renderJobID, traceID string) (AssetCommit, error) {
	reader, info, err := s.Blobs.Open(ctx, sourceLocation)
	if err != nil {
		return AssetCommit{}, err
	}
	defer reader.Close()
	info.MIMEType = canonicalDetectedMediaMIME(info.MIMEType)
	assetKind, maxBytes, supported := generatedAssetPolicy(info.MIMEType)
	if !supported || info.SizeBytes < 1 || info.SizeBytes > maxBytes {
		return AssetCommit{}, fmt.Errorf("%w: size or media type is outside the supported range", ErrInvalidAssetContent)
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return AssetCommit{}, err
	}
	if int64(len(data)) != info.SizeBytes || int64(len(data)) > maxBytes {
		return AssetCommit{}, fmt.Errorf("%w: size changed while reading", ErrInvalidAssetContent)
	}
	mimeType := http.DetectContentType(data[:min(len(data), 512)])
	width, height := 0, 0
	var videoMetadata VideoMetadata
	var audioMetadata AudioMetadata
	switch assetKind {
	case contract.AssetImage:
		if !allowedDeclaredImageMIME(mimeType) {
			return AssetCommit{}, fmt.Errorf("%w: detected content is not a supported image", ErrInvalidAssetContent)
		}
		imageConfig, _, decodeErr := image.DecodeConfig(bytes.NewReader(data))
		if decodeErr != nil {
			return AssetCommit{}, fmt.Errorf("%w: decode image metadata: %v", ErrInvalidAssetContent, decodeErr)
		}
		width, height = imageConfig.Width, imageConfig.Height
		if width < 1 || height < 1 {
			return AssetCommit{}, fmt.Errorf("%w: decoded image dimensions are invalid", ErrInvalidAssetContent)
		}
		if width > MaxImageDimension || height > MaxImageDimension || int64(width)*int64(height) > MaxImagePixels {
			return AssetCommit{}, fmt.Errorf("%w: decoded image dimensions exceed safety limits", ErrInvalidAssetContent)
		}
	case contract.AssetVideo:
		if info.MIMEType != "video/mp4" || len(data) < 12 || string(data[4:8]) != "ftyp" {
			return AssetCommit{}, fmt.Errorf("%w: detected content is not a supported MP4", ErrInvalidAssetContent)
		}
		if s.VideoProbe != nil {
			videoMetadata, err = s.VideoProbe.Probe(ctx, data)
			if err != nil {
				return AssetCommit{}, fmt.Errorf("%w: %v", ErrInvalidAssetContent, err)
			}
			width, height = videoMetadata.WidthPixels, videoMetadata.HeightPixels
		} else if s.MediaProbe == nil {
			return AssetCommit{}, fmt.Errorf("%w: video metadata probe is unavailable", ErrInvalidAssetContent)
		}
		mimeType = "video/mp4"
	case contract.AssetAudio:
		detectedAudioMIME, ok := detectAudioMIME(data)
		if !ok || detectedAudioMIME != info.MIMEType {
			return AssetCommit{}, fmt.Errorf("%w: detected content is not the declared audio type", ErrInvalidAssetContent)
		}
		if s.AudioProbe == nil {
			return AssetCommit{}, fmt.Errorf("%w: audio metadata probe is unavailable", ErrInvalidAssetContent)
		}
		audioMetadata, err = s.AudioProbe.Probe(ctx, data, detectedAudioMIME)
		if err != nil {
			return AssetCommit{}, fmt.Errorf("%w: %v", ErrInvalidAssetContent, err)
		}
		if err := audioMetadata.Validate(); err != nil {
			return AssetCommit{}, fmt.Errorf("%w: %v", ErrInvalidAssetContent, err)
		}
		mimeType = detectedAudioMIME
	}
	if err := s.Scanner.Scan(ctx, bytes.NewReader(data)); err != nil {
		if errors.Is(err, ErrMalwareDetected) {
			return AssetCommit{}, err
		}
		return AssetCommit{}, fmt.Errorf("scan asset content: %w", err)
	}
	digest := sha256.Sum256(data)
	sha256Value := hex.EncodeToString(digest[:])
	media := MediaMetadata{ProbeStatus: MediaProbeNotRequired}
	durableKey := fmt.Sprintf("assets/%s/%s/versions/1/original", organizationID, assetID)
	durable, err := s.Blobs.Put(ctx, s.AssetsBucket, durableKey, bytes.NewReader(data), int64(len(data)), mimeType)
	if err != nil {
		return AssetCommit{}, err
	}
	if assetKind == contract.AssetVideo {
		if s.MediaProbe != nil {
			media, err = s.MediaProbe.Probe(ctx, MediaProbeSource{Location: durable.ObjectLocation, MIMEType: mimeType, SizeBytes: int64(len(data)), SHA256: sha256Value})
			if err != nil {
				media = normalizeProbeFailure(err)
			} else if media.ProbeStatus == "" {
				media.ProbeStatus = MediaProbeSucceeded
			}
		} else {
			media = mediaMetadataFromVideo(videoMetadata)
		}
	} else if assetKind == contract.AssetAudio {
		media = mediaMetadataFromAudio(audioMetadata)
	}
	eventID, err := s.idGenerator()("event")
	if err != nil {
		_ = s.Blobs.Delete(ctx, durable.ObjectLocation)
		return AssetCommit{}, err
	}
	version := int64(1)
	eventData, err := json.Marshal(contract.AssetReadyData{
		AssetKind: assetKind, MIMEType: mimeType, SizeBytes: int64(len(data)), SourceType: source,
		ProviderJobID: providerJobID, OutputID: providerOutputID, RenderJobID: renderJobID,
	})
	if err != nil {
		_ = s.Blobs.Delete(ctx, durable.ObjectLocation)
		return AssetCommit{}, err
	}
	event := contract.EventEnvelope{
		EventID: eventID, EventType: "asset.ready.v1", OccurredAt: s.now(), Producer: "assets",
		OrganizationID: organizationID, ProjectID: projectID,
		Subject: contract.Subject{Type: "asset_version", ID: string(assetID), Version: &version},
		Data:    eventData, TraceID: traceID,
	}
	if err := event.Validate(); err != nil {
		_ = s.Blobs.Delete(ctx, durable.ObjectLocation)
		return AssetCommit{}, err
	}
	return AssetCommit{
		BlobID: blobID, OrganizationID: organizationID, ProjectID: projectID, AssetID: assetID, Version: 1,
		Kind: assetKind, SourceType: source, OwnerSystem: "assets", MIMEType: mimeType,
		SizeBytes: int64(len(data)), SHA256: sha256Value, WidthPixels: width, HeightPixels: height, Media: media,
		DurationMS: max(videoMetadata.DurationMS, audioMetadata.DurationMS), FrameRate: videoMetadata.FrameRate, VideoCodec: videoMetadata.VideoCodec, AudioCodec: firstNonEmpty(videoMetadata.AudioCodec, audioMetadata.Codec),
		RenderJobID: renderJobID, ProviderJobID: providerJobID, ProviderOutputID: providerOutputID, ProjectContextVersion: projectContextVersion,
		Location: durable.ObjectLocation, Event: event,
	}, nil
}

func canonicalDetectedMediaMIME(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "audio/wave", "audio/x-wav":
		return "audio/wav"
	case "audio/mp3":
		return "audio/mpeg"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func mediaMetadataFromAudio(value AudioMetadata) MediaMetadata {
	return MediaMetadata{
		DurationSeconds: float64(value.DurationMS) / 1000,
		Codec:           value.Codec,
		BitrateBPS:      value.BitrateBPS,
		AudioCodec:      value.Codec,
		AudioChannels:   value.Channels,
		AudioSampleRate: value.SampleRate,
		ProbeStatus:     MediaProbeSucceeded,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func mediaMetadataFromVideo(value VideoMetadata) MediaMetadata {
	return MediaMetadata{
		DurationSeconds: float64(value.DurationMS) / 1000,
		Codec:           value.VideoCodec,
		AudioCodec:      value.AudioCodec,
		ProbeStatus:     MediaProbeSucceeded,
	}
}

func (s UploadService) validateDependencies() error {
	if s.Repository == nil || s.Projects == nil || s.Blobs == nil || s.Scanner == nil || s.QuarantineBucket == "" || s.AssetsBucket == "" {
		return fmt.Errorf("upload service dependencies are incomplete")
	}
	return nil
}

func (s UploadService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s UploadService) idGenerator() ids.Generator {
	if s.NewID != nil {
		return s.NewID
	}
	return ids.New
}

func (s UploadService) mediaProbe() MediaProbe {
	if s.MediaProbe != nil {
		return s.MediaProbe
	}
	return UnconfiguredMediaProbe{}
}

func (s UploadService) uploadTTL() time.Duration {
	if s.UploadTTL <= 0 {
		return 15 * time.Minute
	}
	return s.UploadTTL
}

func (s UploadService) previewTTL() time.Duration {
	if s.PreviewTTL <= 0 {
		return 5 * time.Minute
	}
	return s.PreviewTTL
}

func (s UploadService) BlobProvider() string {
	switch s.Blobs.(type) {
	case *TOSBlobStore:
		return "tos"
	case *FilesystemBlobStore:
		return "filesystem"
	default:
		return "memory"
	}
}

func quarantineKey(organizationID contract.OrganizationID, sessionID string) string {
	return fmt.Sprintf("quarantine/%s/%s", organizationID, sessionID)
}

func safeFilename(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = path.Base(value)
	value = strings.Map(func(character rune) rune {
		if character < 32 || character == 127 {
			return -1
		}
		return character
	}, value)
	if value == "." || value == "" {
		return "upload"
	}
	return value
}
