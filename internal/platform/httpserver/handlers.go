package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/agent"
	"github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/identity"
	"github.com/shikanon/cookies/internal/platform/knowledge"
	"github.com/shikanon/cookies/internal/platform/project"
	"github.com/shikanon/cookies/internal/platform/remix"
	"github.com/shikanon/cookies/internal/systems/creative"
)

const maxJSONBody = 1 << 20

func (s *Server) currentIdentity(w http.ResponseWriter, r *http.Request) {
	if s.identities == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.identities.GetCurrent(r.Context(), rc.Actor)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) updateCurrentIdentity(w http.ResponseWriter, r *http.Request) {
	if s.accounts == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	var body struct {
		DisplayName string `json:"display_name"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	value, err := s.accounts.UpdateCurrentUser(r.Context(), rc.Actor, body.DisplayName)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) listOrganizations(w http.ResponseWriter, r *http.Request) {
	if s.accounts == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	values, err := s.accounts.ListOrganizations(r.Context(), rc.Actor)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) listOrganizationMembers(w http.ResponseWriter, r *http.Request) {
	if s.accounts == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	if string(rc.Actor.OrganizationID) != r.PathValue("organization_id") {
		s.writeServiceError(w, r, identity.ErrMembershipForbidden)
		return
	}
	values, err := s.accounts.ListOrganizationMembers(r.Context(), rc.Actor)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) addOrganizationMember(w http.ResponseWriter, r *http.Request) {
	if s.accounts == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	if string(rc.Actor.OrganizationID) != r.PathValue("organization_id") {
		s.writeServiceError(w, r, identity.ErrMembershipForbidden)
		return
	}
	var body struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	value, err := s.accounts.AddOrganizationMember(r.Context(), rc.Actor, body.UserID, body.Role)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) updateOrganizationMember(w http.ResponseWriter, r *http.Request) {
	if s.accounts == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	if string(rc.Actor.OrganizationID) != r.PathValue("organization_id") {
		s.writeServiceError(w, r, identity.ErrMembershipForbidden)
		return
	}
	var body identity.UpdateOrganizationMembershipRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	value, err := s.accounts.UpdateOrganizationMember(r.Context(), rc.Actor, r.PathValue("user_id"), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) listProjectMembers(w http.ResponseWriter, r *http.Request) {
	if s.projectMembers == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	values, err := s.projectMembers.ListProjectMembers(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": values})
}

func (s *Server) addProjectMember(w http.ResponseWriter, r *http.Request) {
	if s.projectMembers == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	var body struct {
		PrincipalKind contract.PrincipalKind `json:"principal_kind"`
		PrincipalID   string                 `json:"principal_id"`
		Role          string                 `json:"role"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	value, err := s.projectMembers.AddProjectMember(r.Context(), rc.Actor,
		contract.ProjectID(r.PathValue("project_id")),
		contract.Principal{Kind: body.PrincipalKind, ID: body.PrincipalID}, body.Role)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) updateProjectMember(w http.ResponseWriter, r *http.Request) {
	if s.projectMembers == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	var body project.UpdateProjectMembershipRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	value, err := s.projectMembers.UpdateProjectMember(r.Context(), rc.Actor,
		contract.ProjectID(r.PathValue("project_id")),
		contract.Principal{Kind: contract.PrincipalKind(r.PathValue("principal_kind")), ID: r.PathValue("principal_id")}, body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) createBrand(w http.ResponseWriter, r *http.Request) {
	if s.projects == nil {
		s.notImplemented(w, r)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if strings.TrimSpace(body.Name) == "" || len(body.Name) > 255 {
		s.badRequest(w, r, fmt.Errorf("name must be between 1 and 255 characters"))
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.CreateBrand(r.Context(), rc.Actor, body.Name)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	if s.projects == nil {
		s.notImplemented(w, r)
		return
	}
	var body project.CreateProjectRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := body.Validate(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.CreateProject(r.Context(), rc.Actor, body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	if s.projects == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.ListProjects(r.Context(), rc.Actor)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": value})
}

func (s *Server) projectDetail(w http.ResponseWriter, r *http.Request) {
	if s.projects == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	detail, err := s.projects.GetDetail(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	assetsValue := []assets.ProjectAsset{}
	if s.uploads != nil {
		assetsValue, err = s.uploads.List(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), 100)
		if err != nil {
			s.writeServiceError(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, struct {
		Project    project.Project                  `json:"project"`
		Runtime    project.ProjectRuntime           `json:"runtime"`
		Artifacts  []project.ProjectArtifactSummary `json:"artifacts"`
		Assets     []assets.ProjectAsset            `json:"assets"`
		Tasks      []project.BusinessTask           `json:"tasks"`
		Operations []project.OperationalRecord      `json:"operations"`
		ChangeSets []project.ChangeSet              `json:"change_sets"`
	}{
		Project:    detail.Project,
		Runtime:    detail.Runtime,
		Artifacts:  detail.Artifacts,
		Assets:     assetsValue,
		Tasks:      detail.Tasks,
		Operations: detail.Operations,
		ChangeSets: detail.ChangeSets,
	})
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	if s.projects == nil {
		s.notImplemented(w, r)
		return
	}
	var body project.UpdateProjectRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := body.Validate(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.UpdateProject(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) listProjectArtifacts(w http.ResponseWriter, r *http.Request) {
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.ListProjectArtifacts(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": value})
}

func (s *Server) createProjectArtifact(w http.ResponseWriter, r *http.Request) {
	if _, ok := idempotencyKey(w, r); !ok {
		return
	}
	var body project.CreateProjectArtifactRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := body.Validate(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.CreateProjectArtifact(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/platform/v1/projects/%s/artifacts/%s", url.PathEscape(r.PathValue("project_id")), url.PathEscape(value.ID)))
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) getProjectArtifact(w http.ResponseWriter, r *http.Request) {
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.GetProjectArtifact(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("artifact_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) updateProjectArtifact(w http.ResponseWriter, r *http.Request) {
	var body project.UpdateProjectArtifactRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := body.Validate(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.UpdateProjectArtifact(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("artifact_id"), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) projectWorkbench(w http.ResponseWriter, r *http.Request) {
	if s.projects == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.GetWorkbench(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) providerCapabilities(w http.ResponseWriter, r *http.Request) {
	if s.providerConfig == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	items, err := s.providerConfig.ListCapabilities(r.Context(), rc.Actor.OrganizationID)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	configured := false
	var checkedAt time.Time
	for _, item := range items {
		if item.Available {
			configured = true
		}
		if item.UpdatedAt.After(checkedAt) {
			checkedAt = item.UpdatedAt
		}
	}
	if checkedAt.IsZero() {
		checkedAt = time.Now().UTC()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"provider":     "cookies-provider-gateway",
		"status":       map[bool]string{true: "configured", false: "not_configured"}[configured],
		"capabilities": items,
		"credential": map[string]any{
			"source":         "workspace",
			"masked_api_key": map[bool]string{true: "encrypted", false: ""}[configured],
		},
		"checked_at": checkedAt,
	})
}

func (s *Server) runWorkbenchQualityCheck(w http.ResponseWriter, r *http.Request) {
	if s.projects == nil {
		s.notImplemented(w, r)
		return
	}
	if _, ok := idempotencyKey(w, r); !ok {
		return
	}
	version, err := positivePathVersion(r.PathValue("version"))
	if err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.RunWorkbenchQualityCheck(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), project.RunWorkbenchQualityCheckRequest{
		AssetID: r.PathValue("asset_id"), AssetVersion: version,
	})
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) recordWorkbenchMaterialConfirmation(w http.ResponseWriter, r *http.Request) {
	if s.projects == nil {
		s.notImplemented(w, r)
		return
	}
	if _, ok := idempotencyKey(w, r); !ok {
		return
	}
	version, err := positivePathVersion(r.PathValue("version"))
	if err != nil {
		s.badRequest(w, r, err)
		return
	}
	var body project.RecordWorkbenchMaterialConfirmationRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	body.AssetID, body.AssetVersion = r.PathValue("asset_id"), version
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.RecordWorkbenchMaterialConfirmation(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) updateWorkbenchAssetPointer(w http.ResponseWriter, r *http.Request) {
	if s.projects == nil {
		s.notImplemented(w, r)
		return
	}
	if _, ok := idempotencyKey(w, r); !ok {
		return
	}
	var body project.UpdateWorkbenchAssetPointerRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	body.AssetID = r.PathValue("asset_id")
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.UpdateWorkbenchAssetPointer(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func positivePathVersion(value string) (int, error) {
	version, err := strconv.Atoi(value)
	if err != nil || version < 1 {
		return 0, fmt.Errorf("version must be a positive integer")
	}
	return version, nil
}

func (s *Server) listProjectTasks(w http.ResponseWriter, r *http.Request) {
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.ListBusinessTasks(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": value})
}

func (s *Server) createProjectTask(w http.ResponseWriter, r *http.Request) {
	if _, ok := idempotencyKey(w, r); !ok {
		return
	}
	var body project.CreateBusinessTaskRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.CreateBusinessTask(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/platform/v1/projects/%s/tasks/%s", url.PathEscape(r.PathValue("project_id")), url.PathEscape(value.ID)))
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) getProjectTask(w http.ResponseWriter, r *http.Request) {
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.GetBusinessTask(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("task_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) updateProjectTask(w http.ResponseWriter, r *http.Request) {
	var body project.UpdateBusinessTaskRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.UpdateBusinessTask(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("task_id"), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) listProjectOperations(w http.ResponseWriter, r *http.Request) {
	limit, ok := boundedLimit(w, r, 100, 200)
	if !ok {
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.ListOperationalRecords(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	if len(value) > limit {
		value = value[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": value})
}

func (s *Server) createProjectOperation(w http.ResponseWriter, r *http.Request) {
	if _, ok := idempotencyKey(w, r); !ok {
		return
	}
	var body project.UpsertOperationalRecordRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.CreateOperationalRecord(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/platform/v1/projects/%s/operations/%s", url.PathEscape(r.PathValue("project_id")), url.PathEscape(value.ID)))
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) getProjectOperation(w http.ResponseWriter, r *http.Request) {
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.GetOperationalRecord(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("operation_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) upsertProjectOperation(w http.ResponseWriter, r *http.Request) {
	var body project.UpsertOperationalRecordRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.UpsertOperationalRecord(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("operation_id"), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) listProjectChangeSets(w http.ResponseWriter, r *http.Request) {
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.ListChangeSets(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": value})
}

func (s *Server) createProjectChangeSet(w http.ResponseWriter, r *http.Request) {
	if _, ok := idempotencyKey(w, r); !ok {
		return
	}
	var body project.CreateChangeSetRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.CreateChangeSet(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/platform/v1/projects/%s/change-sets/%s", url.PathEscape(r.PathValue("project_id")), url.PathEscape(value.ID)))
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) getProjectChangeSet(w http.ResponseWriter, r *http.Request) {
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.GetChangeSet(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("change_set_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) preflightProjectChangeSet(w http.ResponseWriter, r *http.Request) {
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.PreflightChangeSet(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("change_set_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) approveProjectChangeSet(w http.ResponseWriter, r *http.Request) {
	var body project.ChangeSetApprovalRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.ApproveChangeSet(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("change_set_id"), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) executeProjectChangeSet(w http.ResponseWriter, r *http.Request) {
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.ExecuteChangeSet(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("change_set_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) rollbackProjectChangeSet(w http.ResponseWriter, r *http.Request) {
	var body project.RollbackChangeSetRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.RollbackChangeSet(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("change_set_id"), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) listProjectAuditEvents(w http.ResponseWriter, r *http.Request) {
	limit, ok := boundedLimit(w, r, 100, 200)
	if !ok {
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.projects.ListAuditEvents(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	entityType := r.URL.Query().Get("entity_type")
	entityID := r.URL.Query().Get("entity_id")
	filtered := make([]project.AuditEvent, 0, len(value))
	for _, event := range value {
		if entityType != "" && string(event.EntityType) != entityType {
			continue
		}
		if entityID != "" && event.EntityID != entityID {
			continue
		}
		filtered = append(filtered, event)
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": filtered})
}

func (s *Server) createUpload(w http.ResponseWriter, r *http.Request) {
	if s.uploads == nil {
		s.notImplemented(w, r)
		return
	}
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var body assets.CreateUploadRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := body.Validate(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.uploads.Create(r.Context(), rc, contract.ProjectID(r.PathValue("project_id")), key, body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	if value.Upload != nil && value.Upload.URL == "" {
		value.Upload.URL = fmt.Sprintf("/platform/v1/projects/%s/assets/uploads/%s", r.PathValue("project_id"), value.Session.ID)
	}
	writerHeaderNoStore(w)
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) putUpload(w http.ResponseWriter, r *http.Request) {
	if s.uploads == nil {
		s.notImplemented(w, r)
		return
	}
	if r.ContentLength < 1 || r.ContentLength > assets.MaxVideoBytes {
		s.badRequest(w, r, fmt.Errorf("Content-Length is required and outside the supported range"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, assets.MaxVideoBytes)
	rc, _ := contract.RequestContextFrom(r.Context())
	err := s.uploads.PutContent(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("upload_id"), r.Body, r.ContentLength)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) finalizeUpload(w http.ResponseWriter, r *http.Request) {
	if s.uploads == nil {
		s.notImplemented(w, r)
		return
	}
	action := r.PathValue("upload_action")
	if !strings.HasSuffix(action, ":finalize") {
		s.notFound(w, r)
		return
	}
	id := strings.TrimSuffix(action, ":finalize")
	if id == "" {
		s.notFound(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.uploads.Finalize(r.Context(), rc, contract.ProjectID(r.PathValue("project_id")), id)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) listAssets(w http.ResponseWriter, r *http.Request) {
	if s.uploads == nil {
		s.notImplemented(w, r)
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			s.badRequest(w, r, fmt.Errorf("limit must be between 1 and 100"))
			return
		}
		limit = value
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.uploads.List(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), limit)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": value})
}

func (s *Server) previewAsset(w http.ResponseWriter, r *http.Request) {
	if s.uploads == nil {
		s.notImplemented(w, r)
		return
	}
	version, err := strconv.ParseInt(r.PathValue("version"), 10, 64)
	if err != nil || version < 1 {
		s.badRequest(w, r, fmt.Errorf("version must be positive"))
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.uploads.Preview(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), contract.AssetVersionRef{AssetID: contract.AssetID(r.PathValue("asset_id")), Version: version})
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	if value.URL == "" {
		value.URL = fmt.Sprintf("/platform/v1/projects/%s/assets/%s/versions/%d/content",
			url.PathEscape(r.PathValue("project_id")), url.PathEscape(r.PathValue("asset_id")), version)
	}
	writerHeaderNoStore(w)
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) assetContent(w http.ResponseWriter, r *http.Request) {
	if s.uploads == nil {
		s.notImplemented(w, r)
		return
	}
	version, err := strconv.ParseInt(r.PathValue("version"), 10, 64)
	if err != nil || version < 1 {
		s.badRequest(w, r, fmt.Errorf("version must be positive"))
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	reader, info, err := s.uploads.OpenPreview(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), contract.AssetVersionRef{
		AssetID: contract.AssetID(r.PathValue("asset_id")),
		Version: version,
	})
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	defer reader.Close()

	writerHeaderNoStore(w)
	w.Header().Set("Content-Type", info.MIMEType)
	w.Header().Set("Content-Length", strconv.FormatInt(info.SizeBytes, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, reader)
}

func (s *Server) removeAsset(w http.ResponseWriter, r *http.Request) {
	if s.uploads == nil {
		s.notImplemented(w, r)
		return
	}
	version, err := strconv.ParseInt(r.PathValue("version"), 10, 64)
	if err != nil || version < 1 {
		s.badRequest(w, r, fmt.Errorf("version must be positive"))
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	err = s.uploads.Remove(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), contract.AssetVersionRef{
		AssetID: contract.AssetID(r.PathValue("asset_id")),
		Version: version,
	})
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listAssetFeatures(w http.ResponseWriter, r *http.Request) {
	if s.uploads == nil {
		s.notImplemented(w, r)
		return
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 200 {
			s.badRequest(w, r, fmt.Errorf("limit must be between 1 and 200"))
			return
		}
		limit = value
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.uploads.ListFeatures(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), limit)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": value})
}

func (s *Server) getAssetFeature(w http.ResponseWriter, r *http.Request) {
	if s.uploads == nil {
		s.notImplemented(w, r)
		return
	}
	ref, ok := assetFeatureRef(w, r)
	if !ok {
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.uploads.GetFeature(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), ref, r.PathValue("feature_version"))
	if errors.Is(err, assets.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"feature": nil})
		return
	}
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"feature": value})
}

func (s *Server) putAssetFeature(w http.ResponseWriter, r *http.Request) {
	if s.uploads == nil {
		s.notImplemented(w, r)
		return
	}
	ref, ok := assetFeatureRef(w, r)
	if !ok {
		return
	}
	var body assets.AssetFeature
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	body.AssetID = ref.AssetID
	body.AssetVersion = ref.Version
	body.FeatureVersion = r.PathValue("feature_version")
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.uploads.UpsertFeature(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func assetFeatureRef(w http.ResponseWriter, r *http.Request) (contract.AssetVersionRef, bool) {
	version, err := strconv.ParseInt(r.PathValue("version"), 10, 64)
	if err != nil || version < 1 || strings.TrimSpace(r.PathValue("feature_version")) == "" {
		writeProblem(w, http.StatusBadRequest, contract.Error{Code: "INVALID_REQUEST", Message: "The request does not satisfy the API contract.", RequestID: requestIDFrom(r.Context()), Retryable: false})
		return contract.AssetVersionRef{}, false
	}
	return contract.AssetVersionRef{AssetID: contract.AssetID(r.PathValue("asset_id")), Version: version}, true
}

func (s *Server) createGeneratedIntake(w http.ResponseWriter, r *http.Request) {
	if s.intakes == nil {
		s.notImplemented(w, r)
		return
	}
	var body assets.GeneratedAssetIntakeRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := body.Validate(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	keyValue := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if keyValue == "" {
		keyValue = "provider-job-" + body.ProviderJobID + "-output-" + body.Output.OutputID
	}
	key := contract.IdempotencyKey(keyValue)
	if err := key.Validate(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.intakes.Create(r.Context(), rc, contract.ProjectID(r.PathValue("project_id")), key, body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/platform/v1/projects/%s/assets/generated-intakes/%s", r.PathValue("project_id"), value.ID))
	writeJSON(w, http.StatusAccepted, value.Response())
}

func (s *Server) getGeneratedIntake(w http.ResponseWriter, r *http.Request) {
	if s.intakes == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.intakes.Get(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("intake_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value.Response())
}

func (s *Server) createRemixPlan(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
		s.notImplemented(w, r)
		return
	}
	var body remix.CreatePlanRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := body.Validate(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.remixPlans.Create(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/platform/v1/projects/%s/remix-plans/%s", r.PathValue("project_id"), value.ID))
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) listRemixPlans(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
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
	value, err := s.remixPlans.List(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), limit)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": value})
}

func (s *Server) getRemixPlan(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.remixPlans.Get(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("plan_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) createRemixRenderJob(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
		s.notImplemented(w, r)
		return
	}
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var body remix.CreateRenderJobRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := body.Validate(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.remixPlans.CreateRenderJob(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), key, body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/platform/v1/projects/%s/remix-render-jobs/%s", r.PathValue("project_id"), value.ID))
	writeJSON(w, http.StatusAccepted, value)
}

func (s *Server) getRemixRenderJob(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.remixPlans.GetRenderJob(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("job_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) createRemixQualityReport(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
		s.notImplemented(w, r)
		return
	}
	var body remix.CreateQualityReportRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := body.Validate(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.remixPlans.CreateQualityReport(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/platform/v1/projects/%s/remix-quality-reports/%s", r.PathValue("project_id"), value.ID))
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) getRemixQualityReport(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.remixPlans.GetQualityReport(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("report_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) getRemixRenderJobQualityReport(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.remixPlans.GetQualityReportForRenderJob(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("job_id"))
	if errors.Is(err, remix.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"quality_report": nil})
		return
	}
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"quality_report": value})
}

func (s *Server) createRemixHitAnalysis(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
		s.notImplemented(w, r)
		return
	}
	var body remix.CreateHitAnalysisRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := body.Validate(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.remixPlans.CreateHitAnalysis(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/platform/v1/projects/%s/remix-hit-analyses/%s", r.PathValue("project_id"), value.ID))
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) getRemixHitAnalysis(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.remixPlans.GetHitAnalysis(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("analysis_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) createRemixProductMapping(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
		s.notImplemented(w, r)
		return
	}
	var body remix.CreateProductMappingRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := body.Validate(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.remixPlans.CreateProductMapping(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/platform/v1/projects/%s/remix-product-mappings/%s", r.PathValue("project_id"), value.ID))
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) getRemixProductMapping(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.remixPlans.GetProductMapping(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("mapping_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) generateRemixPlanFromProductMapping(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.remixPlans.GeneratePlanFromProductMapping(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("mapping_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/platform/v1/projects/%s/remix-plans/%s", r.PathValue("project_id"), value.ID))
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) createRemixPreroll(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
		s.notImplemented(w, r)
		return
	}
	var body remix.CreatePrerollRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := body.Validate(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.remixPlans.CreatePreroll(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/platform/v1/projects/%s/remix-prerolls/%s", r.PathValue("project_id"), value.ID))
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) getRemixPreroll(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.remixPlans.GetPreroll(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("preroll_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) applyRemixPreroll(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.remixPlans.ApplyPreroll(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("preroll_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) createRemixFeedbackEvent(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
		s.notImplemented(w, r)
		return
	}
	var body remix.CreateFeedbackEventRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := body.Validate(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.remixPlans.CreateFeedbackEvent(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/platform/v1/projects/%s/remix-feedback-events/%s", r.PathValue("project_id"), value.ID))
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) listRemixFeedbackEvents(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
		s.notImplemented(w, r)
		return
	}
	filter := remix.FeedbackEventFilter{
		TargetType: remix.FeedbackTargetType(r.URL.Query().Get("target_type")),
		TargetID:   r.URL.Query().Get("target_id"),
		Limit:      50,
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			s.badRequest(w, r, fmt.Errorf("limit must be between 1 and 100"))
			return
		}
		filter.Limit = value
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.remixPlans.ListFeedbackEvents(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), filter)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": value})
}

func (s *Server) getRemixAssetPerformance(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.remixPlans.GetAssetPerformanceSnapshot(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": value})
}

func (s *Server) createRemixPlannerWeightSnapshot(w http.ResponseWriter, r *http.Request) {
	if s.remixPlans == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.remixPlans.CreatePlannerWeightSnapshot(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/platform/v1/projects/%s/remix-planner-weight-snapshots/%s", r.PathValue("project_id"), value.ID))
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) listRemixEvalCases(w http.ResponseWriter, r *http.Request) {
	if s.evals == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.evals.ListEvalCases(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": value})
}

func (s *Server) createRemixEvalCase(w http.ResponseWriter, r *http.Request) {
	if s.evals == nil {
		s.notImplemented(w, r)
		return
	}
	var body remix.CreateEvalCaseRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := body.Validate(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.evals.CreateEvalCase(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) createRemixEvalRun(w http.ResponseWriter, r *http.Request) {
	if s.evals == nil {
		s.notImplemented(w, r)
		return
	}
	var body remix.CreateEvalRunRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := body.Validate(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.evals.CreateEvalRun(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/platform/v1/projects/%s/remix-eval-runs/%s", r.PathValue("project_id"), value.ID))
	writeJSON(w, http.StatusAccepted, value)
}

func (s *Server) getRemixEvalRun(w http.ResponseWriter, r *http.Request) {
	if s.evals == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.evals.GetEvalRun(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("run_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) listAgentRuns(w http.ResponseWriter, r *http.Request) {
	if s.agentRuns == nil {
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
	value, err := s.agentRuns.ListRuns(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), limit)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": value})
}

func (s *Server) createAgentRun(w http.ResponseWriter, r *http.Request) {
	if s.agentRuns == nil {
		s.notImplemented(w, r)
		return
	}
	var body agent.CreateRunRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := body.Validate(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.agentRuns.CreateRun(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/platform/v1/projects/%s/agent-runs/%s", r.PathValue("project_id"), value.ID))
	writeJSON(w, http.StatusAccepted, value)
}

func (s *Server) getAgentRun(w http.ResponseWriter, r *http.Request) {
	if s.agentRuns == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.agentRuns.GetRun(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("agent_run_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) cancelAgentRun(w http.ResponseWriter, r *http.Request) {
	if s.agentRuns == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.agentRuns.CancelRun(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("agent_run_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) importKnowledgeDocument(w http.ResponseWriter, r *http.Request) {
	if s.knowledge == nil {
		s.notImplemented(w, r)
		return
	}
	var body knowledge.ImportDocumentRequest
	if err := decodeJSON(w, r, &body); err != nil {
		s.badRequest(w, r, err)
		return
	}
	if err := body.Validate(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.knowledge.ImportDocument(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), body)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("/platform/v1/projects/%s/knowledge/documents/%s", r.PathValue("project_id"), value.ID))
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) listKnowledgeDocuments(w http.ResponseWriter, r *http.Request) {
	if s.knowledge == nil {
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
	value, err := s.knowledge.ListDocuments(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), limit)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": value})
}

func (s *Server) getKnowledgeDocument(w http.ResponseWriter, r *http.Request) {
	if s.knowledge == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.knowledge.GetDocument(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("document_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) extractKnowledgeDocumentMedia(w http.ResponseWriter, r *http.Request) {
	if s.knowledge == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	items, err := s.knowledge.ExtractDocumentMedia(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), r.PathValue("document_id"))
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) searchKnowledge(w http.ResponseWriter, r *http.Request) {
	if s.knowledge == nil {
		s.notImplemented(w, r)
		return
	}
	limit := 10
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 50 {
			s.badRequest(w, r, fmt.Errorf("limit must be between 1 and 50"))
			return
		}
		limit = value
	}
	request := knowledge.SearchRequest{Query: r.URL.Query().Get("q"), Limit: limit}
	if err := request.Validate(); err != nil {
		s.badRequest(w, r, err)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.knowledge.Search(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), request)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": request.Query, "items": value})
}

func (s *Server) listKnowledgeResearchRuns(w http.ResponseWriter, r *http.Request) {
	if s.knowledge == nil {
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
	value, err := s.knowledge.ListResearchRuns(
		r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), limit,
	)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": value})
}

func (s *Server) getKnowledgeResearchRun(w http.ResponseWriter, r *http.Request) {
	if s.knowledge == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.knowledge.GetResearchRun(
		r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")),
		r.PathValue("research_run_id"),
	)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) listKnowledgeResearchArtifacts(w http.ResponseWriter, r *http.Request) {
	if s.knowledge == nil {
		s.notImplemented(w, r)
		return
	}
	limit, ok := boundedLimit(w, r, 50, 100)
	if !ok {
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.knowledge.ListResearchArtifacts(
		r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")),
		r.URL.Query().Get("category"), limit,
	)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": value})
}

func (s *Server) getKnowledgeResearchArtifact(w http.ResponseWriter, r *http.Request) {
	if s.knowledge == nil {
		s.notImplemented(w, r)
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	value, err := s.knowledge.GetResearchArtifact(
		r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")),
		r.PathValue("artifact_id"),
	)
	if err != nil {
		s.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return fmt.Errorf("request body must contain one JSON value")
	} else if !errors.Is(err, io.EOF) {
		return fmt.Errorf("invalid trailing JSON: %w", err)
	}
	return nil
}
func idempotencyKey(w http.ResponseWriter, r *http.Request) (contract.IdempotencyKey, bool) {
	key := contract.IdempotencyKey(strings.TrimSpace(r.Header.Get("Idempotency-Key")))
	if err := key.Validate(); err != nil {
		writeProblem(w, http.StatusBadRequest, contract.Error{Code: "INVALID_REQUEST", Message: "A valid Idempotency-Key header is required.", RequestID: requestIDFrom(r.Context()), Retryable: false})
		return "", false
	}
	return key, true
}
func boundedLimit(w http.ResponseWriter, r *http.Request, defaultValue, maxValue int) (int, bool) {
	limit := defaultValue
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > maxValue {
			writeProblem(w, http.StatusBadRequest, contract.Error{Code: "INVALID_REQUEST", Message: "The limit query parameter is outside the supported range.", RequestID: requestIDFrom(r.Context()), Retryable: false})
			return 0, false
		}
		limit = value
	}
	return limit, true
}
func (s *Server) badRequest(w http.ResponseWriter, r *http.Request, _ error) {
	writeProblem(w, http.StatusBadRequest, contract.Error{Code: "INVALID_REQUEST", Message: "The request does not satisfy the API contract.", RequestID: requestIDFrom(r.Context()), Retryable: false})
}
func (s *Server) notImplemented(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, http.StatusServiceUnavailable, contract.Error{Code: "SERVICE_UNAVAILABLE", Message: "This platform service is not configured.", RequestID: requestIDFrom(r.Context()), Retryable: true})
}
func writerHeaderNoStore(w http.ResponseWriter) { w.Header().Set("Cache-Control", "private, no-store") }
func (s *Server) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message, retryable := http.StatusInternalServerError, "INTERNAL", "The service could not complete the request.", true
	var assetUseDenied assets.AssetUseDeniedError
	switch {
	case errors.As(err, &assetUseDenied):
		status, code, message, retryable = http.StatusForbidden, string(assetUseDenied.Code), "The asset rights do not allow this operation.", false
	case errors.Is(err, identity.ErrMembershipForbidden):
		status, code, message, retryable = http.StatusForbidden, "MEMBERSHIP_OPERATION_FORBIDDEN", "当前身份无权执行该成员操作。", false
	case errors.Is(err, identity.ErrMembershipNotFound), errors.Is(err, identity.ErrUserNotFound):
		status, code, message, retryable = http.StatusNotFound, "IDENTITY_RESOURCE_NOT_FOUND", "指定的用户或成员关系不存在。", false
	case errors.Is(err, identity.ErrLastOwner):
		status, code, message, retryable = http.StatusConflict, "LAST_OWNER_REQUIRED", "组织必须保留至少一名有效 owner。", false
	case errors.Is(err, identity.ErrMembershipConflict):
		status, code, message, retryable = http.StatusConflict, "MEMBERSHIP_CHANGED", "成员信息已发生变化，请刷新后重试。", false
	case errors.Is(err, project.ErrMembershipForbidden):
		status, code, message, retryable = http.StatusForbidden, "PROJECT_MEMBERSHIP_OPERATION_FORBIDDEN", "当前身份无权管理该项目成员。", false
	case errors.Is(err, project.ErrMembershipNotFound):
		status, code, message, retryable = http.StatusNotFound, "PROJECT_MEMBERSHIP_NOT_FOUND", "指定的项目成员关系不存在。", false
	case errors.Is(err, project.ErrLastOwner):
		status, code, message, retryable = http.StatusConflict, "PROJECT_LAST_OWNER_REQUIRED", "项目必须保留至少一名有效 owner。", false
	case errors.Is(err, project.ErrMembershipConflict):
		status, code, message, retryable = http.StatusConflict, "PROJECT_MEMBERSHIP_CHANGED", "项目成员信息已发生变化，请刷新后重试。", false
	case errors.Is(err, assets.ErrNotFound), errors.Is(err, project.ErrNotFound), errors.Is(err, remix.ErrNotFound), errors.Is(err, knowledge.ErrNotFound), errors.Is(err, agent.ErrNotFound), errors.Is(err, agent.ErrRunNotFound):
		status, code, message, retryable = http.StatusNotFound, "RESOURCE_NOT_FOUND", "The scoped resource does not exist.", false
	case errors.Is(err, assets.ErrIdempotencyConflict), errors.Is(err, remix.ErrIdempotencyConflict):
		status, code, message, retryable = http.StatusConflict, contract.ErrorIdempotencyConflict, "The idempotency key conflicts with an earlier request.", false
	case errors.Is(err, assets.ErrInvalidState):
		status, code, message, retryable = http.StatusConflict, "INVALID_STATE", "The resource is not in a valid state for this operation.", false
	case errors.Is(err, agent.ErrInvalidState):
		status, code, message, retryable = http.StatusConflict, "INVALID_STATE", "The agent run is not in a valid state for this operation.", false
	case errors.Is(err, assets.ErrOutputMetadataMismatch):
		status, code, message, retryable = http.StatusConflict, contract.ErrorOutputMetadataMismatch, "The uploaded content does not match its declared metadata.", false
	case errors.Is(err, assets.ErrAssetChecksumMismatch):
		status, code, message, retryable = http.StatusConflict, contract.ErrorAssetChecksumMismatch, "The uploaded content does not match its declared checksum.", false
	case errors.Is(err, assets.ErrInvalidAssetContent):
		status, code, message, retryable = http.StatusUnprocessableEntity, contract.ErrorAssetIntakeFailed, "The uploaded content is not a supported valid image.", false
	case errors.Is(err, assets.ErrMalwareDetected):
		status, code, message, retryable = http.StatusUnprocessableEntity, contract.ErrorAssetQuarantined, "The uploaded content was rejected by security scanning.", false
	case errors.Is(err, assets.ErrAssetNotReady):
		status, code, message, retryable = http.StatusConflict, contract.ErrorAssetNotReady, "The asset version is not ready for preview.", false
	case errors.Is(err, assets.ErrProviderOutputExpired):
		status, code, message, retryable = http.StatusGone, contract.ErrorProviderOutputExpired, "The provider output retrieval handle has expired.", false
	case errors.Is(err, assets.ErrUnsupportedAsset):
		status, code, message, retryable = http.StatusUnprocessableEntity, contract.ErrorAssetIntakeFailed, "Only JPEG and PNG assets within the size limit are supported.", false
	case errors.Is(err, assets.ErrProjectContextStale):
		status, code, message, retryable = http.StatusConflict, "PROJECT_CONTEXT_STALE", "The requested project context version is stale.", false
	case errors.Is(err, creative.ErrNotFound):
		status, code, message, retryable = http.StatusNotFound, "RESOURCE_NOT_FOUND", "The scoped Creative resource does not exist.", false
	case errors.Is(err, creative.ErrIdempotencyConflict):
		status, code, message, retryable = http.StatusConflict, contract.ErrorIdempotencyConflict, "The idempotency key conflicts with an earlier Creative request.", false
	case errors.Is(err, creative.ErrOperationVersionConflict):
		status, code, message, retryable = http.StatusConflict, "EDIT_OPERATION_VERSION_CONFLICT", "The operation base version no longer matches the current timeline.", false
	case errors.Is(err, creative.ErrEditTimelineVersionConflict):
		status, code, message, retryable = http.StatusConflict, "EDIT_TIMELINE_VERSION_CONFLICT", "The timeline version no longer matches the current EditTask.", false
	case errors.Is(err, creative.ErrIntakeNotReady):
		status, code, message, retryable = http.StatusConflict, "INTAKE_NEEDS_CLARIFICATION", "The Creative intake needs the missing fields before a task can be created.", false
	case errors.Is(err, creative.ErrProviderJobConflict):
		status, code, message, retryable = http.StatusConflict, "PRODUCTION_JOB_CONFLICT", "A different cover production job already exists for this task.", false
	case errors.Is(err, creative.ErrInvalidState):
		status, code, message, retryable = http.StatusConflict, "INVALID_STATE", "The Creative resource is not in a valid state for this operation.", false
	case errors.Is(err, creative.ErrProviderInputInvalid):
		status, code, message, retryable = http.StatusUnprocessableEntity, "CREATIVE_PROVIDER_INPUT_INVALID", "视频生成参数与当前模型不兼容，请返回剧本分镜检查时长、画幅和清晰度。", false
	case errors.Is(err, creative.ErrVersionConflict):
		status, code, message, retryable = http.StatusPreconditionFailed, "CREATIVE_VERSION_CONFLICT", "The Creative draft changed. Refresh the task and try again.", false
	case errors.Is(err, creative.ErrViralAnalysisSourceUnavailable):
		status, code, message, retryable = http.StatusUnprocessableEntity, "VIRAL_ANALYSIS_SOURCE_UNAVAILABLE", "爆款源视频不可读取，请重新上传后再拆解。", false
	case errors.Is(err, creative.ErrViralAnalysisPreparationFailed):
		status, code, message, retryable = http.StatusUnprocessableEntity, "VIRAL_ANALYSIS_VIDEO_UNREADABLE", "源视频无法抽帧分析，请上传可正常播放的视频后重试。", false
	case errors.Is(err, creative.ErrViralAnalysisProviderRejected):
		status, code, message, retryable = http.StatusBadGateway, "VIRAL_ANALYSIS_REQUEST_REJECTED", "视觉分析模型拒绝了本次请求，请检查模型配置或更换源视频。", false
	case errors.Is(err, creative.ErrViralAnalysisProviderUnavailable):
		status, code, message, retryable = http.StatusServiceUnavailable, "VIRAL_ANALYSIS_PROVIDER_UNAVAILABLE", "视觉分析模型网关暂时不可用，请稍后重试。", true
	case errors.Is(err, creative.ErrViralAnalysisResponseInvalid):
		status, code, message, retryable = http.StatusBadGateway, "VIRAL_ANALYSIS_RESPONSE_INVALID", "视觉分析模型未返回可用的五维拆解结果，请稍后重试。", true
	case errors.Is(err, creative.ErrShortDramaAnalysisSourceUnavailable):
		status, code, message, retryable = http.StatusUnprocessableEntity, "SHORT_DRAMA_SOURCE_UNAVAILABLE", "短剧源视频不可读取，请重新上传后再进行素材理解。", false
	case errors.Is(err, creative.ErrShortDramaAnalysisPreparationFailed):
		status, code, message, retryable = http.StatusUnprocessableEntity, "SHORT_DRAMA_VIDEO_UNREADABLE", "短剧源视频无法抽帧分析，请上传可正常播放的视频后重试。", false
	case errors.Is(err, creative.ErrShortDramaAnalysisProviderRejected):
		status, code, message, retryable = http.StatusBadGateway, "SHORT_DRAMA_ANALYSIS_REQUEST_REJECTED", "视频理解模型拒绝了本次多模态请求，请检查模型能力配置。", false
	case errors.Is(err, creative.ErrShortDramaAnalysisProviderUnavailable):
		status, code, message, retryable = http.StatusServiceUnavailable, "SHORT_DRAMA_ANALYSIS_PROVIDER_UNAVAILABLE", "视频理解模型网关暂时不可用，系统已重试，请稍后再次尝试。", true
	case errors.Is(err, creative.ErrShortDramaAnalysisResponseInvalid):
		status, code, message, retryable = http.StatusBadGateway, "SHORT_DRAMA_ANALYSIS_RESPONSE_INVALID", "视频理解模型没有返回符合剧情分析合约的结果，请重新生成。", true
	case errors.Is(err, creative.ErrInvalidAINativeRequirement):
		status, code, message, retryable = http.StatusBadRequest, "INVALID_AI_NATIVE_REQUIREMENT", "AI 原生广告需求参数不符合当前抖音 P0 规则。", false
	case errors.Is(err, creative.ErrAINativeProductLinkIncomplete):
		status, code, message, retryable = http.StatusUnprocessableEntity, "AI_NATIVE_PRODUCT_LINK_INCOMPLETE", "复制内容不完整，商品参数在中途被截断。请从抖音商品页重新复制完整链接。", false
	case errors.Is(err, creative.ErrAINativeProductLinkUnsupported):
		status, code, message, retryable = http.StatusBadRequest, "AI_NATIVE_PRODUCT_LINK_UNSUPPORTED", "没有识别到受支持的抖音商城商品链接。", false
	case errors.Is(err, creative.ErrAINativeProductDetailMissing):
		status, code, message, retryable = http.StatusUnprocessableEntity, "AI_NATIVE_PRODUCT_DETAIL_MISSING", "链接中没有完整商品信息，可能复制的是视频链接而不是商品详情链接。", false
	case errors.Is(err, creative.ErrAINativeProductUnavailable):
		status, code, message, retryable = http.StatusUnprocessableEntity, "AI_NATIVE_PRODUCT_UNAVAILABLE", "商品链接无法解析或未提供可用商品信息。", false
	case errors.Is(err, project.ErrNotActive):
		status, code, message, retryable = http.StatusConflict, contract.ErrorProjectNotActive, "The project must be active and brand-bound.", false
	case errors.Is(err, project.ErrVersionConflict):
		status, code, message, retryable = http.StatusConflict, "VERSION_CONFLICT", "The submitted expected version does not match the current resource version.", false
	case errors.Is(err, project.ErrInvalidState):
		status, code, message, retryable = http.StatusConflict, "INVALID_STATE", "The resource is not in a valid state for this operation.", false
	case errors.Is(err, project.ErrBrandNotFound):
		status, code, message, retryable = http.StatusBadRequest, "BRAND_NOT_FOUND", "The selected brand does not exist in this organization.", false
	case errors.Is(err, project.ErrProductNotFound):
		status, code, message, retryable = http.StatusBadRequest, "PRODUCT_NOT_FOUND", "A selected product does not exist in this organization.", false
	}
	if status == http.StatusInternalServerError {
		log.Printf("request %s failed: %v", requestIDFrom(r.Context()), err)
	}
	writeProblem(w, status, contract.Error{Code: code, Message: message, RequestID: requestIDFrom(r.Context()), Retryable: retryable})
}
