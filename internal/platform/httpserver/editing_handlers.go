package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/systems/creative"
)

type creativeEditTaskManager interface {
	CreateEditTask(context.Context, contract.ActorContext, contract.ProjectID, creative.CreateEditTaskRequest) (creative.EditTask, error)
	CreateShortDramaV2EditTask(context.Context, contract.ActorContext, contract.ProjectID, creative.CreateShortDramaV2EditTaskRequest) (creative.EditTask, error)
	CreateCreativeVersionEditTask(context.Context, contract.ActorContext, contract.ProjectID, creative.CreateCreativeVersionEditTaskRequest) (creative.EditTask, error)
	ListEditTasks(context.Context, contract.ActorContext, contract.ProjectID, creative.ListEditTasksRequest) ([]creative.EditTask, error)
	ListVersions(context.Context, contract.ActorContext, contract.ProjectID, string, int) ([]creative.CreativeVersion, error)
	GetEditTask(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.EditTask, error)
	SaveEditTimeline(context.Context, contract.ActorContext, contract.ProjectID, string, creative.SaveEditTimelineRequest) (creative.EditTask, error)
	CreateEditingRender(context.Context, contract.RequestContext, contract.ProjectID, string, creative.CreateEditingRenderRequest) (creative.EditingRenderJob, error)
	GetEditingRender(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.EditingRenderJob, error)
	CancelEditingRender(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.EditingRenderJob, error)
	RetryEditingRender(context.Context, contract.RequestContext, contract.ProjectID, string) (creative.EditingRenderJob, error)
	ApplyEditOperations(context.Context, contract.ActorContext, contract.ProjectID, string, creative.ApplyEditOperationsRequest) (creative.EditTask, error)
	ListEditTimelineVersions(context.Context, contract.ActorContext, contract.ProjectID, string, int) ([]creative.TimelineVersion, error)
	SubmitEditingVersion(context.Context, contract.RequestContext, contract.ProjectID, string, creative.SubmitEditingVersionRequest, contract.IdempotencyKey) (creative.CreativeVersion, bool, error)
}

func (s *Server) submitEditingVersion(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.editTaskManager(w, r)
	if !ok {
		return
	}
	var body creative.SubmitEditingVersionRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, duplicate, err := manager.SubmitEditingVersion(r.Context(), rc, contract.ProjectID(r.PathValue("project_id")), r.PathValue("edit_task_id"), body, key)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	if duplicate {
		w.Header().Set("Idempotent-Replay", "true")
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) listEditTimelineVersions(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.creative.(creativeEditTaskManager)
	if !ok {
		s.notImplemented(w, r)
		return
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			s.badRequest(w, r, fmt.Errorf("limit must be between 1 and 100"))
			return
		}
		limit = value
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	items, err := manager.ListEditTimelineVersions(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("edit_task_id"), limit)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) applyEditOperations(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.creative.(creativeEditTaskManager)
	if !ok {
		s.notImplemented(w, r)
		return
	}
	var batch creative.EditOperationBatch
	if err := decodeJSON(w, r, &batch); err != nil {
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := manager.ApplyEditOperations(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("edit_task_id"), creative.ApplyEditOperationsRequest{Batch: batch})
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) listEditTasks(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.editTaskManager(w, r)
	if !ok {
		return
	}
	request := creative.ListEditTasksRequest{Status: creative.EditTaskStatus(r.URL.Query().Get("status"))}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			s.badRequest(w, r, fmt.Errorf("limit must be between 1 and 100"))
			return
		}
		request.Limit = limit
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	items, err := manager.ListEditTasks(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), request)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) createEditingRender(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.editTaskManager(w, r)
	if !ok {
		return
	}
	var body creative.CreateEditingRenderRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := manager.CreateEditingRender(r.Context(), rc, contract.ProjectID(r.PathValue("project_id")), r.PathValue("edit_task_id"), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}
func (s *Server) getEditingRender(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.editTaskManager(w, r)
	if !ok {
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := manager.GetEditingRender(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("render_job_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (s *Server) cancelEditingRender(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.editTaskManager(w, r)
	if !ok {
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := manager.CancelEditingRender(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("render_job_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (s *Server) retryEditingRender(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.editTaskManager(w, r)
	if !ok {
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := manager.RetryEditingRender(r.Context(), rc, contract.ProjectID(r.PathValue("project_id")), r.PathValue("render_job_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) editTaskManager(w http.ResponseWriter, r *http.Request) (creativeEditTaskManager, bool) {
	manager, ok := s.creative.(creativeEditTaskManager)
	if !ok || manager == nil {
		s.notImplemented(w, r)
		return nil, false
	}
	return manager, true
}

func (s *Server) createEditTask(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.editTaskManager(w, r)
	if !ok {
		return
	}
	var body creative.CreateEditTaskRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := manager.CreateEditTask(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) getEditTask(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.editTaskManager(w, r)
	if !ok {
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := manager.GetEditTask(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("edit_task_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) saveEditTimeline(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.editTaskManager(w, r)
	if !ok {
		return
	}
	var body creative.SaveEditTimelineRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := manager.SaveEditTimeline(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("edit_task_id"), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) openShortDramaV2Editor(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.editTaskManager(w, r)
	if !ok {
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	projectID, creativeTaskID := contract.ProjectID(r.PathValue("project_id")), r.PathValue("task_id")
	detail, err := s.creative.GetTaskDetail(r.Context(), rc.Actor, projectID, creativeTaskID)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	if detail.VideoDraft == nil || detail.VideoDraft.ShortDramaPrerollV2 == nil || detail.VideoDraft.ShortDramaPrerollV2.OutputAsset == nil {
		s.writeServiceError(w, r, fmt.Errorf("short drama V2 video is not ready for editing"))
		return
	}
	value, err := manager.CreateShortDramaV2EditTask(r.Context(), rc.Actor, projectID, creative.CreateShortDramaV2EditTaskRequest{
		SourceCreativeTaskID: creativeTaskID,
		PrerollAsset:         detail.VideoDraft.ShortDramaPrerollV2.OutputAsset.AssetVersion,
		SourceAsset:          detail.VideoDraft.ShortDramaPrerollV2.SourceVideo.AssetVersion,
	})
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) openCreativeVersionEditor(w http.ResponseWriter, r *http.Request) {
	manager, ok := s.editTaskManager(w, r)
	if !ok {
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	projectID, taskID := contract.ProjectID(r.PathValue("project_id")), r.PathValue("task_id")
	versions, err := manager.ListVersions(r.Context(), rc.Actor, projectID, taskID, 30)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	var selected *creative.CreativeVersion
	for index := range versions {
		if versions[index].VideoSnapshot != nil && versions[index].VideoSnapshot.FinalVideo.Validate() == nil {
			selected = &versions[index]
			break
		}
	}
	if selected == nil {
		s.writeServiceError(w, r, fmt.Errorf("creative task has no immutable final video available for editing"))
		return
	}
	value, err := manager.CreateCreativeVersionEditTask(r.Context(), rc.Actor, projectID, creative.CreateCreativeVersionEditTaskRequest{SourceCreativeTaskID: taskID, FinalVideo: selected.VideoSnapshot.FinalVideo, DisplayName: "广告成片剪辑"})
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}
