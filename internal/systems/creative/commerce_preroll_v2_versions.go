package creative

import (
	"context"
	"fmt"
	"strings"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type SaveCommercePrerollV2VersionRequest struct {
	ExpectedRevision    int64  `json:"expected_revision"`
	ExpectedTaskVersion int64  `json:"expected_task_version"`
	DisplayName         string `json:"display_name"`
}

type RestoreCommercePrerollV2VersionRequest struct {
	ExpectedRevision int64  `json:"expected_revision"`
	VersionID        string `json:"version_id"`
}

func (s Service) ListCommercePrerollV2Versions(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string) ([]CreativeVersion, error) {
	if _, err := s.GetCommercePrerollV2Workspace(ctx, actor, projectID, taskID); err != nil {
		return nil, err
	}
	values, err := s.Repository.ListVersions(ctx, actor.OrganizationID, projectID, taskID, 100)
	if err != nil {
		return nil, err
	}
	result := make([]CreativeVersion, 0, len(values))
	for _, value := range values {
		if value.VideoSnapshot != nil && value.VideoSnapshot.CommercePrerollV2 != nil {
			result = append(result, value)
		}
	}
	return result, nil
}

func (s Service) SaveCommercePrerollV2Version(ctx context.Context, rc contract.RequestContext, projectID contract.ProjectID, taskID string, request SaveCommercePrerollV2VersionRequest, key contract.IdempotencyKey) (CreativeVersion, bool, error) {
	name := strings.TrimSpace(request.DisplayName)
	if name == "" || len([]rune(name)) > 160 {
		return CreativeVersion{}, false, fmt.Errorf("display_name is required and must not exceed 160 characters")
	}
	if err := key.Validate(); err != nil {
		return CreativeVersion{}, false, err
	}
	detail, err := s.GetCommercePrerollV2Workspace(ctx, rc.Actor, projectID, taskID)
	if err != nil {
		return CreativeVersion{}, false, err
	}
	if detail.VideoDraft.Revision != request.ExpectedRevision {
		return CreativeVersion{}, false, ErrVersionConflict
	}
	if detail.Task.DisplayName != name {
		metadata, ok := s.Repository.(TaskMetadataRepository)
		if !ok {
			return CreativeVersion{}, false, fmt.Errorf("creative task metadata repository is unavailable")
		}
		if _, err = metadata.RenameTask(ctx, rc.Actor.OrganizationID, projectID, taskID, request.ExpectedTaskVersion, name, s.now()); err != nil {
			return CreativeVersion{}, false, err
		}
	}
	existing, err := s.ListCommercePrerollV2Versions(ctx, rc.Actor, projectID, taskID)
	if err != nil {
		return CreativeVersion{}, false, err
	}
	snapshot := &VideoVersionSnapshot{ContractVersion: "creative-video-version/v1", Format: FormatVideo, Channel: detail.Task.Channel, VideoPurpose: "performance", PerformanceMode: PerformanceModeCommercePreroll, DraftRevision: detail.VideoDraft.Revision, SourceVideo: detail.VideoDraft.SourceVideo, CommercePrerollV2: &CommercePrerollV2VersionSnapshot{ContractVersion: "creative-commerce-preroll-version/v1", Workspace: *detail.VideoDraft.CommercePrerollV2}}
	hash, err := contract.NewContentHash(snapshot)
	if err != nil {
		return CreativeVersion{}, false, err
	}
	requestHash, err := contract.CanonicalJSONHash(request)
	if err != nil {
		return CreativeVersion{}, false, err
	}
	id, err := s.idGenerator()("creativeversion")
	if err != nil {
		return CreativeVersion{}, false, err
	}
	value := CreativeVersion{ID: id, OrganizationID: rc.Actor.OrganizationID, ProjectID: projectID, TaskID: taskID, Format: FormatVideo, Version: int64(len(existing) + 1), DraftVersion: detail.VideoDraft.Revision, Status: CreativeVersionCreated, VideoSnapshot: snapshot, ContentHash: hash, CreatedBy: rc.Actor.Principal.ID, CreatedAt: s.now(), IdempotencyKey: key, RequestHash: requestHash}
	if err := value.Validate(); err != nil {
		return CreativeVersion{}, false, err
	}
	return s.Repository.CreateVersion(ctx, value)
}

func (s Service) RestoreCommercePrerollV2Version(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request RestoreCommercePrerollV2VersionRequest) (TaskDetail, error) {
	value, err := s.Repository.GetVersion(ctx, actor.OrganizationID, projectID, request.VersionID)
	if err != nil {
		return TaskDetail{}, err
	}
	if value.TaskID != taskID || value.VideoSnapshot == nil || value.VideoSnapshot.CommercePrerollV2 == nil {
		return TaskDetail{}, ErrNotFound
	}
	snapshot := value.VideoSnapshot.CommercePrerollV2.Workspace
	return s.reviseCommercePrerollV2(ctx, actor, projectID, taskID, request.ExpectedRevision, func(draft *VideoDraft, workspace *CommercePrerollV2Workspace) error {
		createdAt := workspace.CreatedAt
		*workspace = snapshot
		workspace.TaskID = taskID
		workspace.Revision = draft.Revision
		workspace.CreatedAt = createdAt
		workspace.UpdatedAt = s.now()
		return nil
	}, TaskInProgress)
}
