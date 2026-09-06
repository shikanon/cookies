package httpapi

import (
	"net/http"
	"strings"

	"github.com/shikanon/cookies/internal/systems/delivery"
)

func (s *Server) controlledChangeSetAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(r.PathValue("controlled_change_set_action"), ":", 2)
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	r.SetPathValue("change_set_id", parts[0])
	switch parts[1] {
	case "approve":
		s.approveControlledChangeSet(w, r)
	case "execute":
		s.createControlledExecution(w, r)
	case "invalidate-calibration":
		s.invalidateCalibratedControlledChangeSet(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) invalidateCalibratedControlledChangeSet(w http.ResponseWriter, r *http.Request) {
	app, err := s.controlledApp()
	if err != nil {
		writeError(w, r, err)
		return
	}
	var body delivery.InvalidateCalibratedControlledChangeSetRequest
	if !decode(w, r, &body) {
		return
	}
	change, execution, err := app.InvalidateCalibratedControlledChangeSet(r.Context(), mustActor(r), projectID(r), r.PathValue("change_set_id"), body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"change_set": change, "execution": execution})
}

func (s *Server) controlledApp() (controlledAuthorityApplication, error) {
	app, ok := s.app.(controlledAuthorityApplication)
	if !ok {
		return nil, delivery.ErrUnsupportedConfigurationWorkflow
	}
	return app, nil
}
func (s *Server) compileControlledChangeSet(w http.ResponseWriter, r *http.Request) {
	app, err := s.controlledApp()
	if err != nil {
		writeError(w, r, err)
		return
	}
	var body delivery.CompileControlledChangeSetRequest
	if !decode(w, r, &body) {
		return
	}
	body.ObservatoryRunID = r.PathValue("run_id")
	value, replay, err := app.CompileControlledChangeSet(r.Context(), mustActor(r), projectID(r), body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if replay {
		status = http.StatusOK
	}
	writeJSON(w, status, value)
}

func (s *Server) compileMappedControlledChangeSet(w http.ResponseWriter, r *http.Request) {
	app, err := s.controlledApp()
	if err != nil {
		writeError(w, r, err)
		return
	}
	var body delivery.CompileMappedControlledChangeSetRequest
	if !decode(w, r, &body) {
		return
	}
	value, replay, err := app.CompileMappedControlledChangeSet(r.Context(), mustActor(r), projectID(r), r.PathValue("mapping_id"), body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if replay {
		status = http.StatusOK
	}
	writeJSON(w, status, value)
}

func (s *Server) compileEmergencyPauseChangeSet(w http.ResponseWriter, r *http.Request) {
	app, err := s.controlledApp()
	if err != nil {
		writeError(w, r, err)
		return
	}
	var body delivery.CompileEmergencyPauseChangeSetRequest
	if !decode(w, r, &body) {
		return
	}
	value, replay, err := app.CompileEmergencyPauseChangeSet(r.Context(), mustActor(r), projectID(r), r.PathValue("mapping_id"), body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if replay {
		status = http.StatusOK
	}
	writeJSON(w, status, value)
}

func (s *Server) compileControlledRestartChangeSet(w http.ResponseWriter, r *http.Request) {
	app, err := s.controlledApp()
	if err != nil {
		writeError(w, r, err)
		return
	}
	var body delivery.CompileControlledRestartChangeSetRequest
	if !decode(w, r, &body) {
		return
	}
	value, replay, err := app.CompileControlledRestartChangeSet(r.Context(), mustActor(r), projectID(r), r.PathValue("mapping_id"), body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if replay {
		status = http.StatusOK
	}
	writeJSON(w, status, value)
}

func (s *Server) getControlledChangeSet(w http.ResponseWriter, r *http.Request) {
	app, err := s.controlledApp()
	if err != nil {
		writeError(w, r, err)
		return
	}
	value, err := app.GetControlledChangeSet(r.Context(), mustActor(r), projectID(r), r.PathValue("change_set_id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (s *Server) approveControlledChangeSet(w http.ResponseWriter, r *http.Request) {
	app, err := s.controlledApp()
	if err != nil {
		writeError(w, r, err)
		return
	}
	var body delivery.ApproveControlledChangeSetRequest
	if !decode(w, r, &body) {
		return
	}
	change, approval, err := app.ApproveControlledChangeSet(r.Context(), mustActor(r), projectID(r), r.PathValue("change_set_id"), body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	_ = change
	writeJSON(w, http.StatusOK, approval)
}
func (s *Server) createControlledExecution(w http.ResponseWriter, r *http.Request) {
	app, err := s.controlledApp()
	if err != nil {
		writeError(w, r, err)
		return
	}
	value, err := app.CreateControlledExecution(r.Context(), mustActor(r), projectID(r), r.PathValue("change_set_id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}
func (s *Server) getControlledExecution(w http.ResponseWriter, r *http.Request) {
	app, err := s.controlledApp()
	if err != nil {
		writeError(w, r, err)
		return
	}
	value, err := app.GetControlledExecution(r.Context(), mustActor(r), projectID(r), r.PathValue("execution_id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) mappingApp() (platformEntityMappingApplication, error) {
	app, ok := s.app.(platformEntityMappingApplication)
	if !ok {
		return nil, delivery.ErrUnsupportedConfigurationWorkflow
	}
	return app, nil
}

func (s *Server) createPlatformEntityMapping(w http.ResponseWriter, r *http.Request) {
	app, err := s.mappingApp()
	if err != nil {
		writeError(w, r, err)
		return
	}
	var body delivery.PlatformEntityMapping
	if !decode(w, r, &body) {
		return
	}
	if body.OrganizationID != "" || body.ProjectID != "" || body.SchemaVersion != "" || body.Status != "" || body.PlatformObjectID != "" || body.PlatformStatus != "" || body.CurrentStateAction != "" || body.CurrentStateHash != "" || body.ResultEvidenceID != "" || body.ListEvidenceID != "" || body.Version != 0 || !body.CreatedAt.IsZero() || !body.UpdatedAt.IsZero() {
		writeError(w, r, delivery.ErrInvalidRequest)
		return
	}
	body.ProjectID = projectID(r)
	value, err := app.CreatePendingPlatformEntityMapping(r.Context(), mustActor(r), body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) getPlatformEntityMapping(w http.ResponseWriter, r *http.Request) {
	app, err := s.mappingApp()
	if err != nil {
		writeError(w, r, err)
		return
	}
	value, err := app.GetPlatformEntityMapping(r.Context(), mustActor(r), projectID(r), r.PathValue("mapping_id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) listPlatformEntityMappings(w http.ResponseWriter, r *http.Request) {
	app, err := s.mappingApp()
	if err != nil {
		writeError(w, r, err)
		return
	}
	accountReferenceID := strings.TrimSpace(r.URL.Query().Get("account_reference_id"))
	if accountReferenceID == "" {
		writeError(w, r, delivery.ErrInvalidRequest)
		return
	}
	values, err := app.ListPlatformEntityMappings(r.Context(), mustActor(r), projectID(r), accountReferenceID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) platformEntityMappingAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(r.PathValue("mapping_action"), ":", 2)
	if len(parts) != 2 || (parts[1] != "confirm" && parts[1] != "confirm-mutation" && parts[1] != "confirm-change") {
		http.NotFound(w, r)
		return
	}
	r.SetPathValue("mapping_id", parts[0])
	app, err := s.mappingApp()
	if err != nil {
		writeError(w, r, err)
		return
	}
	if parts[1] == "confirm" {
		var body delivery.ConfirmPlatformEntityMappingRequest
		if !decode(w, r, &body) {
			return
		}
		value, err := app.ConfirmPlatformEntityMapping(r.Context(), mustActor(r), projectID(r), r.PathValue("mapping_id"), body)
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
		return
	}
	var body delivery.ConfirmPlatformEntityMappingChangeRequest
	if !decode(w, r, &body) {
		return
	}
	var mapping delivery.PlatformEntityMapping
	var revision delivery.PlatformEntityMappingRevision
	if parts[1] == "confirm-mutation" {
		mapping, revision, err = app.ConfirmPlatformEntityMappingMutation(r.Context(), mustActor(r), projectID(r), r.PathValue("mapping_id"), body)
	} else {
		mapping, revision, err = app.ConfirmPlatformEntityMappingChange(r.Context(), mustActor(r), projectID(r), r.PathValue("mapping_id"), body)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mapping": mapping, "revision": revision})
}
