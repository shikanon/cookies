package httpapi

import (
	"context"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/systems/delivery"
	"net/http"
)

func (s *Server) previewPlanObjects(w http.ResponseWriter, r *http.Request) {
	app, ok := s.app.(interface {
		PreviewPlanObjects(context.Context, contract.ActorContext, contract.ProjectID, string) (delivery.PlanObjectPreview, error)
	})
	if !ok {
		writeError(w, r, delivery.ErrUnsupportedConfigurationWorkflow)
		return
	}
	value, err := app.PreviewPlanObjects(r.Context(), mustActor(r), projectID(r), r.PathValue("plan_id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) startObjectExecution(w http.ResponseWriter, r *http.Request) {
	app, ok := s.app.(interface {
		StartObjectExecution(context.Context, contract.ActorContext, contract.ProjectID, string, delivery.StartBrowserRpaExecutionRequest) (delivery.StartBrowserRpaExecutionResult, error)
	})
	if !ok {
		writeError(w, r, delivery.ErrUnsupportedConfigurationWorkflow)
		return
	}
	var body delivery.StartBrowserRpaExecutionRequest
	if !decode(w, r, &body) {
		return
	}
	body.IdempotencyKey = r.Header.Get("Idempotency-Key")
	value, err := app.StartObjectExecution(r.Context(), mustActor(r), projectID(r), r.PathValue("change_set_id"), body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) readObjectFieldCapabilities(w http.ResponseWriter, r *http.Request) {
	app, ok := s.app.(interface {
		ReadObjectFieldCapabilities(context.Context, contract.ActorContext, contract.ProjectID, string) (delivery.FieldCapabilitySnapshot, error)
	})
	if !ok {
		writeError(w, r, delivery.ErrUnsupportedConfigurationWorkflow)
		return
	}
	value, err := app.ReadObjectFieldCapabilities(r.Context(), mustActor(r), projectID(r), r.PathValue("mapping_id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, value)
}
