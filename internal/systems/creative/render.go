package creative

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/jobruntime"
	"github.com/shikanon/cookies/internal/platform/media"
)

const renderExecutionKind = "creative.video.render"

type RenderStatus string

const (
	RenderQueued    RenderStatus = "queued"
	RenderRunning   RenderStatus = "running"
	RenderSucceeded RenderStatus = "succeeded"
	RenderFailed    RenderStatus = "failed"
)

type RenderJob struct {
	ID               string                    `json:"id"`
	OrganizationID   contract.OrganizationID   `json:"organization_id"`
	ProjectID        contract.ProjectID        `json:"project_id"`
	TaskID           string                    `json:"creative_task_id"`
	Status           RenderStatus              `json:"status"`
	PreRollVideo     contract.AssetVersionRef  `json:"pre_roll_video"`
	MainVideo        contract.AssetVersionRef  `json:"main_video"`
	OutputAsset      *contract.ProjectAssetRef `json:"output_asset,omitempty"`
	ErrorCode        string                    `json:"error_code,omitempty"`
	ErrorMessage     string                    `json:"error_message,omitempty"`
	CreatedBy        contract.Principal        `json:"created_by"`
	IdempotencyKey   contract.IdempotencyKey   `json:"-"`
	RequestHash      string                    `json:"-"`
	CreatedAt        time.Time                 `json:"created_at"`
	UpdatedAt        time.Time                 `json:"updated_at"`
	ProductionUsage  *RenderUsage              `json:"-"`
	ProductionEvents []RenderEvent             `json:"-"`
}

type CreateRenderJobRequest struct {
	PreRollVideo contract.AssetVersionRef `json:"pre_roll_video"`
}

type RenderedAssetWriter interface {
	IngestRenderedVideo(context.Context, contract.RequestContext, contract.ProjectID, string, io.Reader, int64) (contract.ProjectAssetRef, error)
}

type RenderScheduler interface {
	ScheduleRender(context.Context, RenderJob) error
}

func (s Service) CreateRenderJob(ctx context.Context, requestContext contract.RequestContext, projectID contract.ProjectID, taskID string, request CreateRenderJobRequest, key contract.IdempotencyKey) (RenderJob, bool, error) {
	if s.Repository == nil || s.Projects == nil || s.Assets == nil || s.RenderScheduler == nil || s.Composer == nil || s.RenderedAssets == nil {
		return RenderJob{}, false, fmt.Errorf("creative render dependencies are incomplete")
	}
	if err := requestContext.Validate(); err != nil {
		return RenderJob{}, false, err
	}
	if !requestContext.Actor.HasScope(ScopeWrite) {
		return RenderJob{}, false, fmt.Errorf("%s scope is required", ScopeWrite)
	}
	if err := key.Validate(); err != nil {
		return RenderJob{}, false, err
	}
	if err := request.PreRollVideo.Validate(); err != nil {
		return RenderJob{}, false, err
	}
	if _, err := s.Projects.RequireActiveContext(ctx, requestContext.Actor, projectID); err != nil {
		return RenderJob{}, false, err
	}
	detail, err := s.Repository.GetTaskDetail(ctx, requestContext.Actor.OrganizationID, projectID, taskID)
	if err != nil {
		return RenderJob{}, false, err
	}
	if detail.Task.Format != FormatVideo || detail.VideoDraft == nil || detail.Task.PerformanceMode != "pre_roll" {
		return RenderJob{}, false, ErrInvalidState
	}
	preRoll, err := s.Assets.ReadForCreative(ctx, requestContext.Actor, projectID, request.PreRollVideo)
	if err != nil {
		return RenderJob{}, false, err
	}
	if !preRoll.Ready || preRoll.Kind != contract.AssetVideo || preRoll.MIMEType != "video/mp4" || preRoll.Ref != request.PreRollVideo {
		return RenderJob{}, false, fmt.Errorf("pre_roll_video must be a ready MP4 in the same project")
	}
	requestHash, err := contract.CanonicalJSONHash(struct {
		TaskID       string                   `json:"task_id"`
		PreRollVideo contract.AssetVersionRef `json:"pre_roll_video"`
		MainVideo    contract.AssetVersionRef `json:"main_video"`
	}{TaskID: taskID, PreRollVideo: request.PreRollVideo, MainVideo: detail.VideoDraft.SourceVideo})
	if err != nil {
		return RenderJob{}, false, err
	}
	id, err := s.idGenerator()("renderjob")
	if err != nil {
		return RenderJob{}, false, err
	}
	now := s.now()
	value := RenderJob{
		ID: id, OrganizationID: requestContext.Actor.OrganizationID, ProjectID: projectID, TaskID: taskID,
		Status: RenderQueued, PreRollVideo: request.PreRollVideo, MainVideo: detail.VideoDraft.SourceVideo,
		CreatedBy: requestContext.Actor.Principal, IdempotencyKey: key, RequestHash: requestHash,
		CreatedAt: now, UpdatedAt: now,
	}
	stored, duplicate, err := s.Repository.CreateRenderJob(ctx, value)
	if err != nil {
		return RenderJob{}, false, err
	}
	if stored.Status == RenderQueued {
		if err := s.RenderScheduler.ScheduleRender(ctx, stored); err != nil {
			return RenderJob{}, duplicate, err
		}
	}
	return stored, duplicate, nil
}

func (s Service) GetRenderJob(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, renderJobID string) (RenderJob, error) {
	if s.Repository == nil || s.Projects == nil {
		return RenderJob{}, fmt.Errorf("creative dependencies are incomplete")
	}
	if !actor.HasScope(ScopeRead) {
		return RenderJob{}, fmt.Errorf("%s scope is required", ScopeRead)
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return RenderJob{}, err
	}
	return s.Repository.GetRenderJob(ctx, actor.OrganizationID, projectID, renderJobID)
}

func (s Service) ExecuteRenderJob(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, renderJobID string) error {
	if s.Repository == nil || s.Composer == nil || s.RenderedAssets == nil {
		return fmt.Errorf("creative render execution dependencies are incomplete")
	}
	value, err := s.Repository.GetRenderJob(ctx, organizationID, projectID, renderJobID)
	if err != nil {
		return err
	}
	if value.Status == RenderSucceeded {
		return nil
	}
	if value.Status == RenderFailed {
		return ErrInvalidState
	}
	value, err = s.Repository.MarkRenderRunning(ctx, organizationID, projectID, renderJobID, s.now())
	if err != nil {
		return err
	}
	output, err := s.Composer.ComposePreRoll(ctx, media.PreRollCompositionRequest{
		OrganizationID: organizationID, ProjectID: projectID,
		PreRollVideo: value.PreRollVideo, MainVideo: value.MainVideo,
	})
	if err != nil {
		_ = s.Repository.FailRenderJob(ctx, organizationID, projectID, renderJobID, "VIDEO_COMPOSITION_FAILED", boundedError(err), s.now())
		return err
	}
	defer output.Content.Close()
	requestContext := contract.RequestContext{
		RequestID: "render-" + renderJobID, TraceID: "render-" + renderJobID,
		Actor: contract.ActorContext{
			OrganizationID: organizationID, Principal: value.CreatedBy,
			Scopes: []contract.Scope{"project.read", "assets.read", "assets.write", ScopeRead, ScopeWrite},
		},
	}
	ref, err := s.RenderedAssets.IngestRenderedVideo(ctx, requestContext, projectID, renderJobID, output.Content, output.SizeBytes)
	if err != nil {
		_ = s.Repository.FailRenderJob(ctx, organizationID, projectID, renderJobID, "RENDERED_ASSET_INTAKE_FAILED", boundedError(err), s.now())
		return err
	}
	return s.Repository.CompleteRenderJob(ctx, organizationID, projectID, renderJobID, ref, s.now())
}

func boundedError(err error) string {
	value := strings.TrimSpace(err.Error())
	if len(value) > 1000 {
		return value[:350] + " … " + value[len(value)-645:]
	}
	return value
}

type JobRuntimeRenderScheduler struct {
	Store jobruntime.Store
	NewID func() (string, error)
	Now   func() time.Time
}

func (s JobRuntimeRenderScheduler) ScheduleRender(ctx context.Context, render RenderJob) error {
	if s.Store == nil || s.NewID == nil {
		return fmt.Errorf("job runtime store and ID generator are required")
	}
	payload, err := json.Marshal(struct {
		RenderJobID string `json:"render_job_id"`
	}{RenderJobID: render.ID})
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
	digest := sha256.Sum256([]byte(render.ID))
	_, _, err = s.Store.Enqueue(ctx, jobruntime.CreateRequest{
		Job: contract.Job{
			ID: id, Kind: renderExecutionKind, OrganizationID: render.OrganizationID, ProjectID: render.ProjectID,
			Status: contract.JobQueued, MaxAttempts: 3, Version: 1, CreatedAt: now, UpdatedAt: now,
		},
		Payload: payload, IdempotencyKey: contract.IdempotencyKey("creative-render-" + render.ID),
		RequestHash: hex.EncodeToString(digest[:]),
	})
	return err
}

func NewRenderRuntimeWorker(store jobruntime.Store, service Service) jobruntime.Worker {
	return jobruntime.Worker{
		Store: store,
		Handlers: map[string]jobruntime.Handler{
			renderExecutionKind: RenderRuntimeHandler(service),
		},
	}
}

func RenderRuntimeHandler(service Service) jobruntime.Handler {
	return func(ctx context.Context, claim jobruntime.Claim) (jobruntime.Result, error) {
		var payload struct {
			RenderJobID string `json:"render_job_id"`
		}
		if claim.Job.Kind != renderExecutionKind || json.Unmarshal(claim.Payload, &payload) != nil || strings.TrimSpace(payload.RenderJobID) == "" {
			return jobruntime.Result{}, jobruntime.ExecutionError{JobError: contract.JobError{Code: "CREATIVE_RENDER_PAYLOAD_INVALID", Message: "Creative render payload is invalid", Retryable: false}}
		}
		if err := service.ExecuteRenderJob(ctx, claim.Job.OrganizationID, claim.Job.ProjectID, payload.RenderJobID); err != nil {
			return jobruntime.Result{}, jobruntime.ExecutionError{JobError: contract.JobError{Code: "CREATIVE_RENDER_FAILED", Message: "Creative video render failed", Retryable: false}}
		}
		return jobruntime.Result{}, nil
	}
}
