package creative

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type SubmitEditingVersionRequest struct {
	RenderJobID             string `json:"render_job_id"`
	ExpectedTimelineVersion int64  `json:"expected_timeline_version"`
}

func (r SubmitEditingVersionRequest) Validate() error {
	if strings.TrimSpace(r.RenderJobID) == "" || r.ExpectedTimelineVersion < 1 {
		return fmt.Errorf("render_job_id and a positive expected_timeline_version are required")
	}
	return nil
}

func (s Service) SubmitEditingVersion(ctx context.Context, rc contract.RequestContext, projectID contract.ProjectID, editTaskID string, request SubmitEditingVersionRequest, key contract.IdempotencyKey) (CreativeVersion, bool, error) {
	if s.Repository == nil || s.EditTasks == nil || s.EditingRenders == nil || s.Assets == nil || s.Projects == nil {
		return CreativeVersion{}, false, fmt.Errorf("editing submission dependencies are incomplete")
	}
	if err := rc.Validate(); err != nil {
		return CreativeVersion{}, false, err
	}
	if !rc.Actor.HasScope(ScopeWrite) {
		return CreativeVersion{}, false, fmt.Errorf("%s scope is required", ScopeWrite)
	}
	if err := request.Validate(); err != nil {
		return CreativeVersion{}, false, err
	}
	if err := key.Validate(); err != nil {
		return CreativeVersion{}, false, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, rc.Actor, projectID); err != nil {
		return CreativeVersion{}, false, err
	}
	task, err := s.EditTasks.GetEditTask(ctx, rc.Actor.OrganizationID, projectID, editTaskID)
	if err != nil {
		return CreativeVersion{}, false, err
	}
	if task.CurrentTimeline == nil || task.CurrentTimeline.Version != request.ExpectedTimelineVersion {
		return CreativeVersion{}, false, ErrVersionConflict
	}
	render, err := s.EditingRenders.GetEditingRender(ctx, rc.Actor.OrganizationID, projectID, request.RenderJobID)
	if err != nil {
		return CreativeVersion{}, false, err
	}
	if err = editingRenderMaySubmit(render, editTaskID, request.ExpectedTimelineVersion); err != nil {
		return CreativeVersion{}, false, err
	}
	if render.Timeline.ContentHash != task.CurrentTimeline.ContentHash {
		return CreativeVersion{}, false, ErrVersionConflict
	}

	width, height, duration := 720, 1280, render.Timeline.Timeline.DurationMS
	if render.Timeline.Schema() == EditingTimelineSchemaV2 {
		width, height = render.Timeline.TimelineV2.Canvas.Width, render.Timeline.TimelineV2.Canvas.Height
		duration = frameToMSV2(render.Timeline.TimelineV2.DurationFrames)
	}
	outputSnapshot, err := s.Assets.ReadForCreative(ctx, rc.Actor, projectID, render.OutputAsset.AssetVersion)
	if err != nil {
		return CreativeVersion{}, false, err
	}
	if outputSnapshot.Ref != render.OutputAsset.AssetVersion || !outputSnapshot.Ready || outputSnapshot.Kind != contract.AssetVideo || outputSnapshot.MIMEType != "video/mp4" || outputSnapshot.VideoCodec != "h264" || outputSnapshot.AudioCodec != "aac" || outputSnapshot.WidthPixels != width || outputSnapshot.HeightPixels != height || (outputSnapshot.FrameRate != "30" && outputSnapshot.FrameRate != "30/1") || absInt64(outputSnapshot.DurationMS-int64(duration)) > 100 {
		return CreativeVersion{}, false, fmt.Errorf("export output does not satisfy the frozen channel specification")
	}
	inputs := editingVersionInputAssets(render)
	editing := &EditingVersionSnapshot{ContractVersion: "creative-editing-version/v1", EditTaskID: editTaskID, TimelineVersion: render.Timeline.Version, TimelineSchema: render.Timeline.Schema(), TimelineHash: render.Timeline.ContentHash, CompilerVersion: render.Timeline.CompilerCompatibility, RendererFingerprint: render.RendererFingerprint, RenderJobID: render.ID, OutputAsset: render.OutputAsset.AssetVersion, InputAssets: inputs, Width: width, Height: height, FrameRate: 30, SampleRate: 48000, DurationMS: duration, VideoCodec: "h264", AudioCodec: "aac", TargetLUFS: -16}
	video := &VideoVersionSnapshot{ContractVersion: "creative-video-version/v1", Format: FormatVideo, VideoPurpose: "editing", PerformanceMode: "material_edit", DraftRevision: render.Timeline.Version, FinalVideo: render.OutputAsset.AssetVersion, RenderJobID: render.ID, Editing: editing}
	contentHash, err := contract.NewContentHash(video)
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
	value := CreativeVersion{ID: id, OrganizationID: rc.Actor.OrganizationID, ProjectID: projectID, EditTaskID: editTaskID, Format: FormatVideo, Version: render.Timeline.Version, DraftVersion: render.Timeline.Version, Status: CreativeVersionCreated, VideoSnapshot: video, ContentHash: contentHash, CreatedBy: rc.Actor.Principal.ID, CreatedAt: s.now(), IdempotencyKey: key, RequestHash: requestHash}
	if err = value.Validate(); err != nil {
		return CreativeVersion{}, false, err
	}
	created, duplicate, err := s.Repository.CreateVersion(ctx, value)
	if err != nil {
		return CreativeVersion{}, false, err
	}
	if err = s.EditTasks.UpdateEditTaskStatus(ctx, rc.Actor.OrganizationID, projectID, editTaskID, EditTaskReviewReady, s.now()); err != nil {
		return CreativeVersion{}, false, err
	}
	return created, duplicate, nil
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func editingRenderMaySubmit(render EditingRenderJob, editTaskID string, timelineVersion int64) error {
	if render.EditTaskID != editTaskID || render.Kind != EditingRenderExport || render.Status != EditingRenderSucceeded || render.OutputAsset == nil || render.Timeline.Version != timelineVersion {
		return ErrInvalidState
	}
	return nil
}

func editingVersionInputAssets(render EditingRenderJob) []contract.AssetVersionRef {
	refs := make([]contract.AssetVersionRef, 0)
	for _, source := range editingRenderSources(render) {
		if source.Type != "asset_version" || source.Version == nil {
			continue
		}
		refs = append(refs, contract.AssetVersionRef{AssetID: contract.AssetID(source.ID), Version: *source.Version})
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].AssetID != refs[j].AssetID {
			return refs[i].AssetID < refs[j].AssetID
		}
		return refs[i].Version < refs[j].Version
	})
	return refs
}

func evaluateEditingVersion(version CreativeVersion, actorID string, now time.Time) CreativeCheck {
	blockers := make([]string, 0)
	if version.VideoSnapshot == nil || version.VideoSnapshot.Editing == nil || version.VideoSnapshot.Editing.Validate() != nil {
		blockers = append(blockers, "editing output lineage or channel specification is incomplete")
	}
	return CreativeCheck{Passed: len(blockers) == 0, Blockers: blockers, Warnings: []string{}, CheckedBy: actorID, CheckedAt: now}
}
