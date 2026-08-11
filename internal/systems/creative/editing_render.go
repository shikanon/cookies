package creative

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	platformassets "github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/jobruntime"
	"github.com/shikanon/cookies/internal/platform/media"
)

// EditingRenderJob freezes a TimelineVersion at submission time. A later edit
// can therefore never change the input of an already queued render.
type EditingRenderJob struct {
	ID                  string                    `json:"id"`
	OrganizationID      contract.OrganizationID   `json:"organization_id"`
	ProjectID           contract.ProjectID        `json:"project_id"`
	EditTaskID          string                    `json:"edit_task_id"`
	Timeline            TimelineVersion           `json:"timeline"`
	Kind                EditingRenderKind         `json:"kind"`
	RendererFingerprint string                    `json:"renderer_fingerprint"`
	Status              EditingRenderStatus       `json:"status"`
	ProgressPercent     int                       `json:"progress_percent"`
	OutputAsset         *contract.ProjectAssetRef `json:"output_asset,omitempty"`
	ErrorCode           string                    `json:"error_code,omitempty"`
	ErrorMessage        string                    `json:"error_message,omitempty"`
	RetryOf             string                    `json:"retry_of,omitempty"`
	RetryIdempotencyKey contract.IdempotencyKey   `json:"-"`
	RetryRequestHash    string                    `json:"-"`
	CreatedBy           contract.Principal        `json:"created_by"`
	CreatedAt           time.Time                 `json:"created_at"`
	UpdatedAt           time.Time                 `json:"updated_at"`
	ProductionUsage     *RenderUsage              `json:"-"`
	ProductionEvents    []RenderEvent             `json:"-"`
}

type EditingRenderKind string

const (
	EditingRenderPreview EditingRenderKind = "preview"
	EditingRenderExport  EditingRenderKind = "export"
)

var ErrEditingRenderInputUnavailable = errors.New("editing render input is unavailable")

type EditingRenderStatus string

const (
	EditingRenderQueued    EditingRenderStatus = "queued"
	EditingRenderRunning   EditingRenderStatus = "running"
	EditingRenderSucceeded EditingRenderStatus = "succeeded"
	EditingRenderFailed    EditingRenderStatus = "failed"
	EditingRenderCancelled EditingRenderStatus = "cancelled"
)

type EditingRenderRepository interface {
	CreateEditingRender(context.Context, EditingRenderJob) (EditingRenderJob, error)
	GetEditingRender(context.Context, contract.OrganizationID, contract.ProjectID, string) (EditingRenderJob, error)
	FindReusableEditingRender(context.Context, contract.OrganizationID, contract.ProjectID, string, EditingRenderKind) (EditingRenderJob, error)
	MarkEditingRenderRunning(context.Context, contract.OrganizationID, contract.ProjectID, string, time.Time) (EditingRenderJob, error)
	UpdateEditingRenderProgress(context.Context, contract.OrganizationID, contract.ProjectID, string, int, time.Time) error
	CompleteEditingRender(context.Context, contract.OrganizationID, contract.ProjectID, string, contract.ProjectAssetRef, time.Time) error
	FailEditingRender(context.Context, contract.OrganizationID, contract.ProjectID, string, string, string, time.Time) error
	CancelEditingRender(context.Context, contract.OrganizationID, contract.ProjectID, string, time.Time) (EditingRenderJob, error)
}

type EditingRenderRetryRepository interface {
	GetEditingRenderByRetryKey(context.Context, contract.OrganizationID, contract.ProjectID, contract.IdempotencyKey) (EditingRenderJob, error)
}

type CreateEditingRenderRequest struct {
	Kind EditingRenderKind `json:"kind"`
}

const editingRenderExecutionKind = "creative.editing.render"

type EditingRenderScheduler interface {
	ScheduleEditingRender(context.Context, EditingRenderJob) error
}

func (s Service) CreateEditingRender(ctx context.Context, rc contract.RequestContext, projectID contract.ProjectID, editTaskID string, request CreateEditingRenderRequest) (EditingRenderJob, error) {
	if s.EditingRenders == nil || s.EditingRenderScheduler == nil || s.AINativeTimelineRenderer == nil || s.RenderedAssets == nil {
		return EditingRenderJob{}, fmt.Errorf("editing render dependencies are incomplete")
	}
	if err := rc.Validate(); err != nil {
		return EditingRenderJob{}, err
	}
	if !rc.Actor.HasScope(ScopeWrite) {
		return EditingRenderJob{}, fmt.Errorf("%s scope is required", ScopeWrite)
	}
	if request.Kind != EditingRenderPreview && request.Kind != EditingRenderExport {
		return EditingRenderJob{}, fmt.Errorf("editing render kind is unsupported")
	}
	task, err := s.GetEditTask(ctx, rc.Actor, projectID, editTaskID)
	if err != nil {
		return EditingRenderJob{}, err
	}
	if task.CurrentTimeline == nil {
		return EditingRenderJob{}, fmt.Errorf("edit task has no renderable timeline")
	}
	fingerprint, err := editingRenderFingerprint(*task.CurrentTimeline, request.Kind)
	if err != nil {
		return EditingRenderJob{}, err
	}
	purpose := platformassets.AssetUseRenderPreview
	if request.Kind == EditingRenderExport {
		purpose = platformassets.AssetUseRenderExport
	}
	if task.CurrentTimeline.Schema() == EditingTimelineSchemaV2 {
		if err := s.validateTimelineV2Assets(ctx, rc.Actor, projectID, *task.CurrentTimeline.TimelineV2, purpose); err != nil {
			return EditingRenderJob{}, err
		}
	} else if err := s.validateTimelineAssetsForPurpose(ctx, rc.Actor, projectID, task.CurrentTimeline.Timeline, purpose); err != nil {
		return EditingRenderJob{}, err
	}
	if cached, cacheErr := s.EditingRenders.FindReusableEditingRender(ctx, rc.Actor.OrganizationID, projectID, fingerprint, request.Kind); cacheErr == nil {
		status := EditTaskDraft
		if request.Kind == EditingRenderExport {
			status = EditTaskReviewReady
		}
		_ = s.EditTasks.UpdateEditTaskStatus(ctx, rc.Actor.OrganizationID, projectID, task.ID, status, s.now())
		return cached, nil
	} else if cacheErr != ErrNotFound {
		return EditingRenderJob{}, cacheErr
	}
	id, err := s.idGenerator()("editingrender")
	if err != nil {
		return EditingRenderJob{}, err
	}
	now := s.now()
	job := EditingRenderJob{ID: id, OrganizationID: rc.Actor.OrganizationID, ProjectID: projectID, EditTaskID: task.ID, Timeline: *task.CurrentTimeline, Kind: request.Kind, RendererFingerprint: fingerprint, Status: EditingRenderQueued, CreatedBy: rc.Actor.Principal, CreatedAt: now, UpdatedAt: now}
	created, err := s.EditingRenders.CreateEditingRender(ctx, job)
	if err != nil {
		return EditingRenderJob{}, err
	}
	if err := s.EditTasks.UpdateEditTaskStatus(ctx, created.OrganizationID, created.ProjectID, created.EditTaskID, EditTaskRendering, s.now()); err != nil {
		_ = s.EditingRenders.FailEditingRender(ctx, created.OrganizationID, created.ProjectID, created.ID, "EDIT_TASK_STATUS_FAILED", boundedError(err), s.now())
		return EditingRenderJob{}, err
	}
	if err := s.EditingRenderScheduler.ScheduleEditingRender(ctx, created); err != nil {
		_ = s.EditingRenders.FailEditingRender(ctx, created.OrganizationID, created.ProjectID, created.ID, "SCHEDULER_ENQUEUE_FAILED", boundedError(err), s.now())
		_ = s.EditTasks.UpdateEditTaskStatus(ctx, created.OrganizationID, created.ProjectID, created.EditTaskID, EditTaskFailed, s.now())
		return EditingRenderJob{}, err
	}
	return created, nil
}

type JobRuntimeEditingRenderScheduler struct {
	Store jobruntime.Store
	NewID func() (string, error)
	Now   func() time.Time
}

func (s JobRuntimeEditingRenderScheduler) ScheduleEditingRender(ctx context.Context, render EditingRenderJob) error {
	if s.Store == nil || s.NewID == nil {
		return fmt.Errorf("job runtime store and ID generator are required")
	}
	payload, err := json.Marshal(struct {
		RenderJobID string `json:"render_job_id"`
	}{render.ID})
	if err != nil {
		return err
	}
	id, err := s.NewID()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	sum := sha256.Sum256([]byte(render.ID))
	_, _, err = s.Store.Enqueue(ctx, jobruntime.CreateRequest{Job: contract.Job{ID: id, Kind: editingRenderExecutionKind, OrganizationID: render.OrganizationID, ProjectID: render.ProjectID, Status: contract.JobQueued, MaxAttempts: 3, Version: 1, CreatedAt: now, UpdatedAt: now}, Payload: payload, IdempotencyKey: contract.IdempotencyKey("editing-render-" + render.ID), RequestHash: hex.EncodeToString(sum[:])})
	return err
}
func EditingRenderRuntimeHandler(service Service) jobruntime.Handler {
	return func(ctx context.Context, claim jobruntime.Claim) (jobruntime.Result, error) {
		var payload struct {
			RenderJobID string `json:"render_job_id"`
		}
		if claim.Job.Kind != editingRenderExecutionKind || json.Unmarshal(claim.Payload, &payload) != nil || strings.TrimSpace(payload.RenderJobID) == "" {
			return jobruntime.Result{}, jobruntime.ExecutionError{JobError: contract.JobError{Code: "EDITING_RENDER_PAYLOAD_INVALID", Message: "editing render payload is invalid", Retryable: false}}
		}
		if err := service.ExecuteEditingRender(ctx, claim.Job.OrganizationID, claim.Job.ProjectID, payload.RenderJobID); err != nil {
			// Keep the public RenderJob terminal even when restoration fails before
			// ExecuteEditingRender can mark it running. Otherwise the UI polls a
			// permanently queued job after the runtime job has already failed.
			if service.EditingRenders != nil {
				_ = service.EditingRenders.FailEditingRender(ctx, claim.Job.OrganizationID, claim.Job.ProjectID, payload.RenderJobID, "EDITING_RENDER_FAILED", boundedError(err), service.now())
			}
			return jobruntime.Result{}, jobruntime.ExecutionError{JobError: contract.JobError{Code: "EDITING_RENDER_FAILED", Message: "editing render failed", Retryable: false}}
		}
		return jobruntime.Result{}, nil
	}
}

func (s Service) ExecuteEditingRender(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, jobID string) error {
	if s.EditingRenders == nil || s.EditTasks == nil || s.Assets == nil || s.AINativeTimelineRenderer == nil || s.RenderedAssets == nil {
		return fmt.Errorf("editing render dependencies are incomplete")
	}
	job, err := s.EditingRenders.GetEditingRender(ctx, organizationID, projectID, jobID)
	if err != nil {
		return err
	}
	if job.Status == EditingRenderSucceeded {
		return nil
	}
	if job.Status == EditingRenderCancelled {
		return nil
	}
	if job.Status != EditingRenderQueued {
		return ErrInvalidState
	}
	actor := contract.ActorContext{OrganizationID: organizationID, Principal: job.CreatedBy, Scopes: []contract.Scope{"project.read", "assets.read", "assets.write", ScopeRead, ScopeWrite}}
	purpose := platformassets.AssetUseRenderPreview
	if job.Kind == EditingRenderExport {
		purpose = platformassets.AssetUseRenderExport
	}
	if job.Timeline.Schema() == EditingTimelineSchemaV2 {
		if err := s.validateTimelineV2Assets(ctx, actor, projectID, *job.Timeline.TimelineV2, purpose); err != nil {
			return s.failEditingRender(ctx, job, "ASSET_USE_REVOKED", err)
		}
	} else if err := s.validateTimelineAssetsForPurpose(ctx, actor, projectID, job.Timeline.Timeline, purpose); err != nil {
		return s.failEditingRender(ctx, job, "ASSET_USE_REVOKED", err)
	}
	job, err = s.EditingRenders.MarkEditingRenderRunning(ctx, organizationID, projectID, jobID, s.now())
	if err != nil {
		return err
	}
	document := EditingDocument{V1: &job.Timeline.Timeline}
	if job.Timeline.Schema() == EditingTimelineSchemaV2 {
		document = EditingDocument{V2: job.Timeline.TimelineV2}
	}
	compiled, err := DefaultEditingCompilerRegistry().Compile(document, organizationID, projectID)
	if err != nil {
		return s.failEditingRender(ctx, job, "TIMELINE_INVALID", err)
	}
	request := compiled.MediaRequest
	last := -1
	output, err := s.AINativeTimelineRenderer.Render(ctx, request, func(progress media.TimelineProgress) error {
		latest, getErr := s.EditingRenders.GetEditingRender(ctx, organizationID, projectID, jobID)
		if getErr != nil {
			return getErr
		}
		if latest.Status == EditingRenderCancelled {
			return context.Canceled
		}
		if progress.Percent <= last {
			return nil
		}
		last = progress.Percent
		return s.EditingRenders.UpdateEditingRenderProgress(ctx, organizationID, projectID, jobID, progress.Percent, s.now())
	})
	if err != nil {
		return s.failEditingRender(ctx, job, "TIMELINE_RENDER_FAILED", err)
	}
	defer output.Content.Close()
	requestContext := contract.RequestContext{RequestID: "editing-render-" + jobID, TraceID: "editing-render-" + jobID, Actor: actor}
	var ref contract.ProjectAssetRef
	if writer, ok := s.RenderedAssets.(editingRenderedAssetWriterWithSources); ok {
		ref, err = writer.IngestRenderedVideoWithSources(ctx, requestContext, projectID, jobID, editingRenderSources(job), output.Content, output.SizeBytes)
	} else {
		ref, err = s.RenderedAssets.IngestRenderedVideo(ctx, requestContext, projectID, jobID, output.Content, output.SizeBytes)
	}
	if err != nil {
		return s.failEditingRender(ctx, job, "RENDERED_ASSET_INTAKE_FAILED", err)
	}
	if err = s.EditingRenders.CompleteEditingRender(ctx, organizationID, projectID, jobID, ref, s.now()); err != nil {
		return err
	}
	nextStatus := EditTaskDraft
	if job.Kind == EditingRenderExport {
		nextStatus = EditTaskReviewReady
	}
	return s.EditTasks.UpdateEditTaskStatus(ctx, organizationID, projectID, job.EditTaskID, nextStatus, s.now())
}

func (s Service) CancelEditingRender(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, jobID string) (EditingRenderJob, error) {
	if s.EditingRenders == nil || s.EditTasks == nil || s.Projects == nil {
		return EditingRenderJob{}, fmt.Errorf("editing render dependencies are incomplete")
	}
	if !actor.HasScope(ScopeWrite) {
		return EditingRenderJob{}, fmt.Errorf("%s scope is required", ScopeWrite)
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return EditingRenderJob{}, err
	}
	job, err := s.EditingRenders.CancelEditingRender(ctx, actor.OrganizationID, projectID, jobID, s.now())
	if err == nil {
		_ = s.EditTasks.UpdateEditTaskStatus(ctx, actor.OrganizationID, projectID, job.EditTaskID, EditTaskDraft, s.now())
	}
	return job, err
}

func (s Service) RetryEditingRender(ctx context.Context, rc contract.RequestContext, projectID contract.ProjectID, jobID string) (EditingRenderJob, error) {
	return s.retryEditingRender(ctx, rc, projectID, jobID, "", "")
}

func (s Service) RetryEditingRenderForProduction(ctx context.Context, rc contract.RequestContext, projectID contract.ProjectID, jobID string, key contract.IdempotencyKey) (EditingRenderJob, error) {
	if err := key.Validate(); err != nil {
		return EditingRenderJob{}, err
	}
	hash, err := contract.CanonicalJSONHash(struct {
		ProjectID contract.ProjectID `json:"project_id"`
		RunID     string             `json:"run_id"`
	}{ProjectID: projectID, RunID: jobID})
	if err != nil {
		return EditingRenderJob{}, err
	}
	repository, ok := s.EditingRenders.(EditingRenderRetryRepository)
	if !ok {
		return EditingRenderJob{}, fmt.Errorf("editing render retry idempotency is unavailable")
	}
	if existing, getErr := repository.GetEditingRenderByRetryKey(ctx, rc.Actor.OrganizationID, projectID, key); getErr == nil {
		if existing.RetryRequestHash != hash {
			return EditingRenderJob{}, ErrIdempotencyConflict
		}
		return existing, nil
	} else if !errors.Is(getErr, ErrNotFound) {
		return EditingRenderJob{}, getErr
	}
	return s.retryEditingRender(ctx, rc, projectID, jobID, key, hash)
}

func (s Service) retryEditingRender(ctx context.Context, rc contract.RequestContext, projectID contract.ProjectID, jobID string, key contract.IdempotencyKey, requestHash string) (EditingRenderJob, error) {
	if s.EditingRenders == nil || s.EditTasks == nil || s.EditingRenderScheduler == nil || s.Assets == nil {
		return EditingRenderJob{}, fmt.Errorf("editing render dependencies are incomplete")
	}
	if err := rc.Validate(); err != nil {
		return EditingRenderJob{}, err
	}
	if !rc.Actor.HasScope(ScopeWrite) {
		return EditingRenderJob{}, fmt.Errorf("%s scope is required", ScopeWrite)
	}
	previous, err := s.GetEditingRender(ctx, rc.Actor, projectID, jobID)
	if err != nil {
		return EditingRenderJob{}, err
	}
	if previous.Status != EditingRenderFailed {
		return EditingRenderJob{}, ErrInvalidState
	}
	purpose := platformassets.AssetUseRenderPreview
	if previous.Kind == EditingRenderExport {
		purpose = platformassets.AssetUseRenderExport
	}
	if previous.Timeline.Schema() == EditingTimelineSchemaV2 {
		if err := s.validateTimelineV2Assets(ctx, rc.Actor, projectID, *previous.Timeline.TimelineV2, purpose); err != nil {
			return EditingRenderJob{}, fmt.Errorf("%w: %v", ErrEditingRenderInputUnavailable, err)
		}
	} else if err := s.validateTimelineAssetsForPurpose(ctx, rc.Actor, projectID, previous.Timeline.Timeline, purpose); err != nil {
		return EditingRenderJob{}, fmt.Errorf("%w: %v", ErrEditingRenderInputUnavailable, err)
	}
	id, err := s.idGenerator()("editingrender")
	if err != nil {
		return EditingRenderJob{}, err
	}
	now := s.now()
	next := EditingRenderJob{ID: id, OrganizationID: rc.Actor.OrganizationID, ProjectID: projectID, EditTaskID: previous.EditTaskID, Timeline: previous.Timeline, Kind: previous.Kind, RendererFingerprint: previous.RendererFingerprint, Status: EditingRenderQueued, RetryOf: previous.ID, RetryIdempotencyKey: key, RetryRequestHash: requestHash, CreatedBy: rc.Actor.Principal, CreatedAt: now, UpdatedAt: now}
	created, err := s.EditingRenders.CreateEditingRender(ctx, next)
	if err != nil {
		return EditingRenderJob{}, err
	}
	if err = s.EditingRenderScheduler.ScheduleEditingRender(ctx, created); err != nil {
		_ = s.EditingRenders.FailEditingRender(ctx, created.OrganizationID, created.ProjectID, created.ID, "SCHEDULER_ENQUEUE_FAILED", boundedError(err), s.now())
		return EditingRenderJob{}, err
	}
	_ = s.EditTasks.UpdateEditTaskStatus(ctx, created.OrganizationID, created.ProjectID, created.EditTaskID, EditTaskRendering, s.now())
	return created, nil
}

func (s Service) GetEditingRender(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, jobID string) (EditingRenderJob, error) {
	if s.EditingRenders == nil || s.Projects == nil {
		return EditingRenderJob{}, fmt.Errorf("editing render dependencies are incomplete")
	}
	if !actor.HasScope(ScopeRead) {
		return EditingRenderJob{}, fmt.Errorf("%s scope is required", ScopeRead)
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return EditingRenderJob{}, err
	}
	return s.EditingRenders.GetEditingRender(ctx, actor.OrganizationID, projectID, jobID)
}

func (s Service) failEditingRender(ctx context.Context, job EditingRenderJob, code string, cause error) error {
	_ = s.EditingRenders.FailEditingRender(ctx, job.OrganizationID, job.ProjectID, job.ID, code, boundedError(cause), s.now())
	if s.EditTasks != nil {
		_ = s.EditTasks.UpdateEditTaskStatus(ctx, job.OrganizationID, job.ProjectID, job.EditTaskID, EditTaskFailed, s.now())
	}
	return cause
}

type editingRenderedAssetWriter interface {
	IngestRenderedVideo(context.Context, contract.RequestContext, contract.ProjectID, string, io.Reader, int64) (contract.ProjectAssetRef, error)
}

type editingRenderedAssetWriterWithSources interface {
	IngestRenderedVideoWithSources(context.Context, contract.RequestContext, contract.ProjectID, string, []contract.ResourceRef, io.Reader, int64) (contract.ProjectAssetRef, error)
}

func editingRenderSources(job EditingRenderJob) []contract.ResourceRef {
	seen := make(map[contract.AssetVersionRef]bool)
	result := make([]contract.ResourceRef, 0)
	if job.Timeline.Schema() == EditingTimelineSchemaV2 {
		for _, track := range job.Timeline.TimelineV2.Tracks {
			for _, clip := range track.Clips {
				if clip.AssetRef == nil || seen[*clip.AssetRef] {
					continue
				}
				seen[*clip.AssetRef] = true
				version := clip.AssetRef.Version
				result = append(result, contract.ResourceRef{Type: "asset_version", ID: string(clip.AssetRef.AssetID), Version: &version})
			}
		}
		version := job.Timeline.Version
		return append(result, contract.ResourceRef{Type: "edit_timeline_version", ID: job.EditTaskID, Version: &version})
	}
	for _, track := range job.Timeline.Timeline.Tracks {
		for _, clip := range track.Clips {
			if clip.AssetRef == nil || seen[*clip.AssetRef] {
				continue
			}
			seen[*clip.AssetRef] = true
			version := clip.AssetRef.Version
			result = append(result, contract.ResourceRef{Type: "asset_version", ID: string(clip.AssetRef.AssetID), Version: &version})
		}
	}
	version := job.Timeline.Version
	return append(result, contract.ResourceRef{Type: "edit_timeline_version", ID: job.EditTaskID, Version: &version})
}

func editingRenderFingerprint(timeline TimelineVersion, kind EditingRenderKind) (string, error) {
	hash, err := contract.CanonicalJSONHash(struct {
		TimelineHash    string            `json:"timeline_hash"`
		TimelineSchema  string            `json:"timeline_schema"`
		CompilerVersion string            `json:"compiler_version"`
		RendererVersion string            `json:"renderer_version"`
		Kind            EditingRenderKind `json:"kind"`
	}{timeline.ContentHash, timeline.Schema(), timeline.CompilerCompatibility, media.TimelineRendererVersion, kind})
	if err != nil {
		return "", err
	}
	return "sha256:" + hash, nil
}

func (j EditingRenderJob) Validate() error {
	if strings.TrimSpace(j.ID) == "" || j.OrganizationID == "" || j.ProjectID == "" || strings.TrimSpace(j.EditTaskID) == "" || j.Timeline.Validate() != nil || !strings.HasPrefix(j.RendererFingerprint, "sha256:") || (j.Kind != EditingRenderPreview && j.Kind != EditingRenderExport) || (j.Status != EditingRenderQueued && j.Status != EditingRenderRunning && j.Status != EditingRenderSucceeded && j.Status != EditingRenderFailed && j.Status != EditingRenderCancelled) || j.ProgressPercent < 0 || j.ProgressPercent > 100 || j.CreatedAt.IsZero() || j.UpdatedAt.IsZero() {
		return fmt.Errorf("editing render job is incomplete")
	}
	return nil
}
