package httpserver

import (
	"context"
	"net/http"
	"strings"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/provider"
	"github.com/shikanon/cookies/internal/systems/creative"
)

type commercePrerollV2Commands interface {
	CreateCommercePrerollV2Workspace(context.Context, contract.RequestContext, contract.ProjectID, contract.IdempotencyKey, creative.CreateCommercePrerollV2WorkspaceRequest) (creative.TaskDetail, error)
	GetCommercePrerollV2Workspace(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.TaskDetail, error)
	GetLatestCommercePrerollV2Workspace(context.Context, contract.ActorContext, contract.ProjectID) (creative.TaskDetail, error)
	AnalyzeCommercePrerollV2Source(context.Context, contract.ActorContext, contract.ProjectID, string, creative.AnalyzeCommercePrerollV2SourceRequest) (creative.TaskDetail, error)
	ConfirmCommercePrerollV2Understanding(context.Context, contract.ActorContext, contract.ProjectID, string, creative.ConfirmCommercePrerollV2UnderstandingRequest) (creative.TaskDetail, error)
	GenerateCommercePrerollV2Hooks(context.Context, contract.ActorContext, contract.ProjectID, string, creative.GenerateCommercePrerollV2HooksRequest) (creative.TaskDetail, error)
	SelectCommercePrerollV2Hook(context.Context, contract.ActorContext, contract.ProjectID, string, creative.SelectCommercePrerollV2HookRequest) (creative.TaskDetail, error)
	PrepareCommercePrerollV2References(context.Context, contract.RequestContext, contract.ProjectID, string, creative.PrepareCommercePrerollV2ReferencesRequest) (creative.TaskDetail, error)
	BindCommercePrerollV2ProductReference(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BindCommercePrerollV2ProductReferenceRequest) (creative.TaskDetail, error)
	BindCommercePrerollV2CustomFirstFrame(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BindCommercePrerollV2CustomFirstFrameRequest) (creative.TaskDetail, error)
	GenerateCommercePrerollV2FirstFrames(context.Context, contract.ActorContext, contract.ProjectID, string, creative.GenerateCommercePrerollV2FirstFramesRequest) (creative.TaskDetail, error)
	ReconcileCommercePrerollV2FirstFrame(context.Context, contract.ActorContext, contract.ProjectID, string, creative.ReconcileCommercePrerollV2FirstFrameRequest) (creative.TaskDetail, error)
	SelectCommercePrerollV2FirstFrame(context.Context, contract.ActorContext, contract.ProjectID, string, creative.SelectCommercePrerollV2FirstFrameRequest) (creative.TaskDetail, error)
	CommercePrerollV2ProviderInput(context.Context, contract.ActorContext, contract.ProjectID, string) (provider.VideoGenerationInput, string, string, error)
	ReserveCommercePrerollV2VideoOperation(context.Context, contract.ActorContext, contract.ProjectID, string, int64, string) (creative.TaskDetail, error)
	RegisterCommercePrerollV2VideoJob(context.Context, contract.ActorContext, contract.ProjectID, string, int64, string, string) (creative.TaskDetail, error)
	FailCommercePrerollV2VideoOperation(context.Context, contract.ActorContext, contract.ProjectID, string, int64, string, contract.JobError) (creative.TaskDetail, error)
	ReconcileCommercePrerollV2Video(context.Context, contract.ActorContext, contract.ProjectID, string, creative.ReconcileCommercePrerollV2VideoRequest) (creative.TaskDetail, error)
	AdoptCommercePrerollV2Output(context.Context, contract.ActorContext, contract.ProjectID, string, creative.AdoptCommercePrerollV2OutputRequest) (creative.TaskDetail, error)
	ListCommercePrerollV2Versions(context.Context, contract.ActorContext, contract.ProjectID, string) ([]creative.CreativeVersion, error)
	SaveCommercePrerollV2Version(context.Context, contract.RequestContext, contract.ProjectID, string, creative.SaveCommercePrerollV2VersionRequest, contract.IdempotencyKey) (creative.CreativeVersion, bool, error)
	RestoreCommercePrerollV2Version(context.Context, contract.ActorContext, contract.ProjectID, string, creative.RestoreCommercePrerollV2VersionRequest) (creative.TaskDetail, error)
	SelectCommercePrerollV2ProductReference(context.Context, contract.ActorContext, contract.ProjectID, string, creative.SelectCommercePrerollV2ProductReferenceRequest) (creative.TaskDetail, error)
	UpdateCommercePrerollV2Storyboard(context.Context, contract.ActorContext, contract.ProjectID, string, creative.UpdateCommercePrerollV2StoryboardRequest) (creative.TaskDetail, error)
	UpdateCommercePrerollV2Prompt(context.Context, contract.ActorContext, contract.ProjectID, string, creative.UpdateCommercePrerollV2PromptRequest) (creative.TaskDetail, error)
}

func (s *Server) commercePrerollV2(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.creative.(commercePrerollV2Commands)
	if !ok {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	projectID := contract.ProjectID(r.PathValue("project_id"))
	taskID := r.PathValue("task_id")
	if r.Method == http.MethodGet {
		if strings.HasSuffix(r.URL.Path, "/versions") {
			items, err := manager.ListCommercePrerollV2Versions(r.Context(), rc.Actor, projectID, taskID)
			if err != nil {
				s.writeServiceError(w, r, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"items": items})
			return
		}
		var value creative.TaskDetail
		var err error
		if strings.HasSuffix(r.URL.Path, ":latest") {
			value, err = manager.GetLatestCommercePrerollV2Workspace(r.Context(), rc.Actor, projectID)
		} else {
			value, err = manager.GetCommercePrerollV2Workspace(r.Context(), rc.Actor, projectID, taskID)
		}
		if err != nil {
			s.writeServiceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
		return
	}
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	if taskID == "" {
		var body creative.CreateCommercePrerollV2WorkspaceRequest
		if err := decodeJSON(w, r, &body); err != nil {
			s.badRequest(w, r, err)
			return
		}
		value, err := manager.CreateCommercePrerollV2Workspace(r.Context(), rc, projectID, key, body)
		if err != nil {
			s.writeServiceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, value)
		return
	}
	var value creative.TaskDetail
	var err error
	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, ":analyze-source"):
		var body creative.AnalyzeCommercePrerollV2SourceRequest
		if decodeErr := decodeJSON(w, r, &body); decodeErr != nil {
			s.badRequest(w, r, decodeErr)
			return
		}
		value, err = manager.AnalyzeCommercePrerollV2Source(r.Context(), rc.Actor, projectID, taskID, body)
	case strings.HasSuffix(path, ":confirm-understanding"):
		var body creative.ConfirmCommercePrerollV2UnderstandingRequest
		if decodeErr := decodeJSON(w, r, &body); decodeErr != nil {
			s.badRequest(w, r, decodeErr)
			return
		}
		value, err = manager.ConfirmCommercePrerollV2Understanding(r.Context(), rc.Actor, projectID, taskID, body)
	case strings.HasSuffix(path, ":generate-hooks"):
		var body creative.GenerateCommercePrerollV2HooksRequest
		if decodeErr := decodeJSON(w, r, &body); decodeErr != nil {
			s.badRequest(w, r, decodeErr)
			return
		}
		value, err = manager.GenerateCommercePrerollV2Hooks(r.Context(), rc.Actor, projectID, taskID, body)
	case strings.HasSuffix(path, ":select-hook"):
		var body creative.SelectCommercePrerollV2HookRequest
		if decodeErr := decodeJSON(w, r, &body); decodeErr != nil {
			s.badRequest(w, r, decodeErr)
			return
		}
		value, err = manager.SelectCommercePrerollV2Hook(r.Context(), rc.Actor, projectID, taskID, body)
	case strings.HasSuffix(path, ":prepare-references"):
		var body creative.PrepareCommercePrerollV2ReferencesRequest
		if decodeErr := decodeJSON(w, r, &body); decodeErr != nil {
			s.badRequest(w, r, decodeErr)
			return
		}
		value, err = manager.PrepareCommercePrerollV2References(r.Context(), rc, projectID, taskID, body)
	case strings.HasSuffix(path, ":bind-product-reference"):
		var body creative.BindCommercePrerollV2ProductReferenceRequest
		if decodeErr := decodeJSON(w, r, &body); decodeErr != nil {
			s.badRequest(w, r, decodeErr)
			return
		}
		value, err = manager.BindCommercePrerollV2ProductReference(r.Context(), rc.Actor, projectID, taskID, body)
	case strings.HasSuffix(path, ":select-product-reference"):
		var body creative.SelectCommercePrerollV2ProductReferenceRequest
		if decodeErr := decodeJSON(w, r, &body); decodeErr != nil {
			s.badRequest(w, r, decodeErr)
			return
		}
		value, err = manager.SelectCommercePrerollV2ProductReference(r.Context(), rc.Actor, projectID, taskID, body)
	case strings.HasSuffix(path, ":update-storyboard"):
		var body creative.UpdateCommercePrerollV2StoryboardRequest
		if decodeErr := decodeJSON(w, r, &body); decodeErr != nil {
			s.badRequest(w, r, decodeErr)
			return
		}
		value, err = manager.UpdateCommercePrerollV2Storyboard(r.Context(), rc.Actor, projectID, taskID, body)
	case strings.HasSuffix(path, ":update-prompt"):
		var body creative.UpdateCommercePrerollV2PromptRequest
		if decodeErr := decodeJSON(w, r, &body); decodeErr != nil {
			s.badRequest(w, r, decodeErr)
			return
		}
		value, err = manager.UpdateCommercePrerollV2Prompt(r.Context(), rc.Actor, projectID, taskID, body)
	case strings.HasSuffix(path, ":save-version"):
		var body creative.SaveCommercePrerollV2VersionRequest
		if decodeErr := decodeJSON(w, r, &body); decodeErr != nil {
			s.badRequest(w, r, decodeErr)
			return
		}
		version, _, saveErr := manager.SaveCommercePrerollV2Version(r.Context(), rc, projectID, taskID, body, key)
		if saveErr != nil {
			err = saveErr
		} else {
			writeJSON(w, http.StatusCreated, version)
			return
		}
	case strings.HasSuffix(path, ":restore-version"):
		var body creative.RestoreCommercePrerollV2VersionRequest
		if decodeErr := decodeJSON(w, r, &body); decodeErr != nil {
			s.badRequest(w, r, decodeErr)
			return
		}
		value, err = manager.RestoreCommercePrerollV2Version(r.Context(), rc.Actor, projectID, taskID, body)
	case strings.HasSuffix(path, ":bind-custom-first-frame"):
		var body creative.BindCommercePrerollV2CustomFirstFrameRequest
		if decodeErr := decodeJSON(w, r, &body); decodeErr != nil {
			s.badRequest(w, r, decodeErr)
			return
		}
		value, err = manager.BindCommercePrerollV2CustomFirstFrame(r.Context(), rc.Actor, projectID, taskID, body)
	case strings.HasSuffix(path, ":generate-first-frames"):
		var body creative.GenerateCommercePrerollV2FirstFramesRequest
		if decodeErr := decodeJSON(w, r, &body); decodeErr != nil {
			s.badRequest(w, r, decodeErr)
			return
		}
		value, err = manager.GenerateCommercePrerollV2FirstFrames(r.Context(), rc.Actor, projectID, taskID, body)
	case strings.HasSuffix(path, ":reconcile-first-frame"):
		if s.providerJobs == nil {
			s.notImplemented(w, r)
			return
		}
		var body struct {
			ExpectedRevision int64  `json:"expected_revision"`
			CandidateID      string `json:"candidate_id"`
			ProviderJobID    string `json:"provider_job_id"`
		}
		if decodeErr := decodeJSON(w, r, &body); decodeErr != nil {
			s.badRequest(w, r, decodeErr)
			return
		}
		job, getErr := s.providerJobs.GetJob(r.Context(), rc.Actor.OrganizationID, projectID, body.ProviderJobID)
		if getErr != nil {
			s.writeServiceError(w, r, getErr)
			return
		}
		value, err = manager.ReconcileCommercePrerollV2FirstFrame(r.Context(), rc.Actor, projectID, taskID, creative.ReconcileCommercePrerollV2FirstFrameRequest{ExpectedRevision: body.ExpectedRevision, CandidateID: body.CandidateID, Job: job})
	case strings.HasSuffix(path, ":select-first-frame"):
		var body creative.SelectCommercePrerollV2FirstFrameRequest
		if decodeErr := decodeJSON(w, r, &body); decodeErr != nil {
			s.badRequest(w, r, decodeErr)
			return
		}
		value, err = manager.SelectCommercePrerollV2FirstFrame(r.Context(), rc.Actor, projectID, taskID, body)
	case strings.HasSuffix(path, ":generate-video"):
		value, err = s.generateCommercePrerollV2Video(w, r, rc, projectID, taskID, key, manager)
	case strings.HasSuffix(path, ":reconcile-video"):
		if s.providerJobs == nil {
			s.notImplemented(w, r)
			return
		}
		var body struct {
			ExpectedRevision int64  `json:"expected_revision"`
			ProviderJobID    string `json:"provider_job_id"`
		}
		if decodeErr := decodeJSON(w, r, &body); decodeErr != nil {
			s.badRequest(w, r, decodeErr)
			return
		}
		job, getErr := s.providerJobs.GetJob(r.Context(), rc.Actor.OrganizationID, projectID, body.ProviderJobID)
		if getErr != nil {
			s.writeServiceError(w, r, getErr)
			return
		}
		value, err = manager.ReconcileCommercePrerollV2Video(r.Context(), rc.Actor, projectID, taskID, creative.ReconcileCommercePrerollV2VideoRequest{ExpectedRevision: body.ExpectedRevision, Job: job})
	case strings.HasSuffix(path, ":adopt-output"):
		var body creative.AdoptCommercePrerollV2OutputRequest
		if decodeErr := decodeJSON(w, r, &body); decodeErr != nil {
			s.badRequest(w, r, decodeErr)
			return
		}
		value, err = manager.AdoptCommercePrerollV2Output(r.Context(), rc.Actor, projectID, taskID, body)
	default:
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) generateCommercePrerollV2Video(w http.ResponseWriter, r *http.Request, rc contract.RequestContext, projectID contract.ProjectID, taskID string, key contract.IdempotencyKey, manager commercePrerollV2Commands) (creative.TaskDetail, error) {
	if s.providerJobs == nil || s.projects == nil {
		return creative.TaskDetail{}, creative.ErrInvalidState
	}
	var body struct {
		ExpectedRevision int64  `json:"expected_revision"`
		ModelAlias       string `json:"model_alias,omitempty"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return creative.TaskDetail{}, err
	}
	detail, err := manager.GetCommercePrerollV2Workspace(r.Context(), rc.Actor, projectID, taskID)
	if err != nil {
		return creative.TaskDetail{}, err
	}
	if detail.VideoDraft.Revision != body.ExpectedRevision {
		return creative.TaskDetail{}, creative.ErrVersionConflict
	}
	input, promptHash, specHash, err := manager.CommercePrerollV2ProviderInput(r.Context(), rc.Actor, projectID, taskID)
	if err != nil {
		return creative.TaskDetail{}, err
	}
	project, err := s.projects.GetContext(r.Context(), rc.Actor, projectID)
	if err != nil {
		return creative.TaskDetail{}, err
	}
	modelAlias := strings.TrimSpace(body.ModelAlias)
	if modelAlias == "" {
		modelAlias = "cookies.video.standard"
	}
	requestHash, err := contract.CanonicalJSONHash(struct {
		TaskID     string                        `json:"task_id"`
		Revision   int64                         `json:"revision"`
		PromptHash string                        `json:"prompt_hash"`
		SpecHash   string                        `json:"spec_hash"`
		Input      provider.VideoGenerationInput `json:"input"`
	}{taskID, body.ExpectedRevision, promptHash, specHash, input})
	if err != nil {
		return creative.TaskDetail{}, err
	}
	operationID := "sha256:" + requestHash
	reserved, err := manager.ReserveCommercePrerollV2VideoOperation(r.Context(), rc.Actor, projectID, taskID, body.ExpectedRevision, operationID)
	if err != nil {
		return creative.TaskDetail{}, err
	}
	job, _, err := s.providerJobs.CreateVideoJob(r.Context(), provider.CreateVideoJobRequest{Actor: rc.Actor, Project: project, IdempotencyKey: key, RequestHash: requestHash, ModelAlias: modelAlias, SourceSystem: "creative.commerce_preroll_v2", SourceTaskID: taskID, Input: input})
	if err != nil {
		_, _ = manager.FailCommercePrerollV2VideoOperation(r.Context(), rc.Actor, projectID, taskID, reserved.VideoDraft.Revision, operationID, contract.JobError{Code: "VIDEO_JOB_CREATE_FAILED", Message: err.Error(), Retryable: true})
		return creative.TaskDetail{}, err
	}
	return manager.RegisterCommercePrerollV2VideoJob(r.Context(), rc.Actor, projectID, taskID, reserved.VideoDraft.Revision, operationID, job.ID)
}
