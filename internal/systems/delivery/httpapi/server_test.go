package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/identity"
	"github.com/shikanon/cookies/internal/systems/delivery"
)

func TestDeliveryHTTPExposesPlanAndControlledActions(t *testing.T) {
	t.Parallel()
	app := &applicationStub{
		plan:      delivery.DeliveryPlan{ID: "deliveryplan_1", Version: 1},
		changeSet: delivery.ChangeSet{ID: "deliverychangeset_1", PlanID: "deliveryplan_1", Version: 1},
	}
	server := New(app)

	response := httptest.NewRecorder()
	request := authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/plans", `{
		"intent":{},"platform_configuration":{}
	}`)
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), "deliveryplan_1") {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	request = authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/plans/deliveryplan_1:create-change-set", `{"expected_version":1}`)
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || app.createdPlanID != "deliveryplan_1" {
		t.Fatalf("change-set status=%d body=%s plan=%q", response.Code, response.Body.String(), app.createdPlanID)
	}
}

func TestDeliveryHTTPMapsImmutableContractIdentityConflict(t *testing.T) {
	server := New(&immutableIdentityConflictApplicationStub{applicationStub: applicationStub{}})
	response := httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPatch, "/api/delivery/v1/projects/project_1/plans/plan_1", `{
		"expected_version":4,"intent":{},"platform_configuration":{}
	}`))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"IMMUTABLE_IDENTITY_CONFLICT"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestMechanisticSimulationHTTPUsesPlanVersionWithoutExecution(t *testing.T) {
	app := &mechanisticApplicationStub{applicationStub: applicationStub{}}
	server := New(app)
	response := httptest.NewRecorder()
	request := authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/plans/plan_1/versions/2/mechanistic-simulation-runs", `{
		"stable_seed":"seed","sample_count":100,"prediction_horizon_days":1,"review_state":"unknown",
		"prior_set":{"version":"prior/v0"}
	}`)
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || app.planID != "plan_1" || app.version != 2 || app.request.StableSeed != "seed" || !strings.Contains(response.Body.String(), `"calibration_status":"assumption_driven"`) {
		t.Fatalf("create status=%d plan=%q version=%d request=%+v body=%s", response.Code, app.planID, app.version, app.request, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/delivery/v1/projects/project_1/mechanistic-simulation-runs/run_1", ""))
	if response.Code != http.StatusOK || app.runID != "run_1" || !strings.Contains(response.Body.String(), `"is_simulated":true`) {
		t.Fatalf("get status=%d run=%q body=%s", response.Code, app.runID, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/delivery/v1/projects/project_1/plans/plan_1/versions/2/mechanistic-simulation-run", ""))
	if response.Code != http.StatusOK || app.planID != "plan_1" || app.version != 2 || !strings.Contains(response.Body.String(), `"id":"latest_run"`) {
		t.Fatalf("latest status=%d plan=%q version=%d body=%s", response.Code, app.planID, app.version, response.Body.String())
	}
}

func TestConnectorInspectionHTTPDoesNotUseFixtureOrExecution(t *testing.T) {
	app := &connectorInspectionApplicationStub{applicationStub: applicationStub{}}
	server := New(app)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/alerts:inspect", `{"plan_id":"plan_1","window_days":14}`))
	if response.Code != http.StatusOK || app.request.PlanID != "plan_1" || app.request.WindowDays != 14 || !strings.Contains(response.Body.String(), `"is_simulated":false`) {
		t.Fatalf("status=%d request=%+v body=%s", response.Code, app.request, response.Body.String())
	}
}

type connectorInspectionApplicationStub struct {
	applicationStub
	request delivery.ConnectorInspectionRequest
}

func (s *connectorInspectionApplicationStub) InspectConnectorAlerts(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, request delivery.ConnectorInspectionRequest) (delivery.ConnectorInspectionResponse, error) {
	s.request = request
	return delivery.ConnectorInspectionResponse{Items: []delivery.DeliveryAlert{}, Source: "connector", IsSimulated: false, Status: "quarantined", DatasetVersion: "connector/v1"}, nil
}

type mechanisticApplicationStub struct {
	applicationStub
	planID  string
	version int
	runID   string
	request delivery.MechanisticSimulationRequest
}

func (s *mechanisticApplicationStub) CreatePrelaunchMechanisticSimulation(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, planID string, version int, request delivery.MechanisticSimulationRequest) (delivery.MechanisticSimulationEnvelope, error) {
	s.planID, s.version, s.request = planID, version, request
	return delivery.MechanisticSimulationEnvelope{Result: delivery.MechanisticSimulationResult{ID: "run_1", SchemaVersion: delivery.MechanisticSimulationSchemaVersion, ModelVersion: delivery.MechanisticSimulationModelVersion, CalibrationStatus: delivery.CalibrationStatusAssumptionDriven, IsSimulated: true, Status: "completed"}}, nil
}

func (s *mechanisticApplicationStub) GetPrelaunchMechanisticSimulation(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, runID string) (delivery.MechanisticSimulationResult, error) {
	s.runID = runID
	return delivery.MechanisticSimulationResult{ID: runID, SchemaVersion: delivery.MechanisticSimulationSchemaVersion, ModelVersion: delivery.MechanisticSimulationModelVersion, CalibrationStatus: delivery.CalibrationStatusAssumptionDriven, IsSimulated: true, Status: "completed"}, nil
}

func (s *mechanisticApplicationStub) GetLatestPrelaunchMechanisticSimulation(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, planID string, version int) (delivery.MechanisticSimulationResult, error) {
	s.planID, s.version = planID, version
	return delivery.MechanisticSimulationResult{ID: "latest_run", PlanID: planID, PlanVersion: version, IsSimulated: true}, nil
}

func TestPlatformEntityMappingHTTPKeepsPlatformValuesServerOwnedAndRequiresTwoReadbacks(t *testing.T) {
	createdAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	app := &mappingApplicationStub{applicationStub: applicationStub{}, mapping: delivery.PlatformEntityMapping{SchemaVersion: delivery.PlatformEntityMappingV1, ID: "mapping_1", OrganizationID: "org_1", ProjectID: "project_1", AccountReferenceID: "account_1", PlanID: "plan_1", ConfigurationID: "configuration_1", BusinessExecutionID: "execution_1", BrowserRpaRunID: "run_1", InternalObjectKind: "promotion", InternalObjectID: "draft_1", PlatformObjectKind: "promotion", Status: delivery.PlatformEntityMappingPending, Version: 1, CreatedAt: createdAt, UpdatedAt: createdAt}, controlledChange: delivery.ControlledChangeSet{SchemaVersion: delivery.ControlledChangeSetSchemaV1, ID: "change_mutation"}}
	server := New(app)
	body := `{"id":"mapping_1","account_reference_id":"account_1","plan_id":"plan_1","configuration_id":"configuration_1","business_execution_id":"execution_1","browser_rpa_run_id":"run_1","internal_object_kind":"project","internal_object_id":"draft_1","platform_object_kind":"project"}`
	response := httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/platform-entity-mappings", body))
	if response.Code != http.StatusCreated || app.created.ProjectID != "project_1" || app.created.PlatformObjectID != "" || !strings.Contains(response.Body.String(), `"status":"pending_verification"`) {
		t.Fatalf("create status=%d created=%#v body=%s", response.Code, app.created, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/delivery/v1/projects/project_1/platform-entity-mappings?account_reference_id=account_1", ""))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items"`) || !strings.Contains(response.Body.String(), `"id":"mapping_1"`) {
		t.Fatalf("list status=%d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/delivery/v1/projects/project_1/platform-entity-mappings", ""))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("list without account status=%d body=%s", response.Code, response.Body.String())
	}
	injected := strings.TrimSuffix(body, "}") + `,"platform_object_id":"forged"}`
	response = httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/platform-entity-mappings", injected))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("injected status=%d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/platform-entity-mappings/mapping_1:confirm", `{"expected_version":1,"result_evidence_id":"evidence_result","list_evidence_id":"evidence_list"}`))
	if response.Code != http.StatusOK || app.confirm.ExpectedVersion != 1 || app.confirm.ResultEvidenceID == app.confirm.ListEvidenceID || !strings.Contains(response.Body.String(), `"status":"confirmed"`) {
		t.Fatalf("confirm status=%d request=%#v body=%s", response.Code, app.confirm, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/platform-entity-mappings/mapping_1:confirm", `{"expected_version":1,"result_evidence_id":"evidence_result","list_evidence_id":"evidence_list","platform_object_id":"forged"}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("client-owned platform value status=%d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/platform-entity-mappings/mapping_1/controlled-change-sets", `{"expected_mapping_version":2,"action":"update_promotion_budget","current_daily_budget_minor":30000,"target_daily_budget_minor":36000,"supersedes_controlled_change_set_id":"change_cancelled"}`))
	if response.Code != http.StatusCreated || app.mappedCompile.Action != delivery.ControlledActionUpdatePromotionBudget || app.mappedCompile.ExpectedMappingVersion != 2 || app.mappedCompile.SupersedesControlledChangeSetID != "change_cancelled" || !strings.Contains(response.Body.String(), `"id":"change_mutation"`) {
		t.Fatalf("compile mutation status=%d request=%#v body=%s", response.Code, app.mappedCompile, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/platform-entity-mappings/mapping_1:confirm-mutation", `{"expected_version":2,"business_execution_id":"mutation_execution","result_evidence_id":"mutation_result","list_evidence_id":"mutation_list"}`))
	if response.Code != http.StatusOK || app.confirmMutation.BusinessExecutionID != "mutation_execution" || !strings.Contains(response.Body.String(), `"revision"`) {
		t.Fatalf("confirm mutation status=%d request=%#v body=%s", response.Code, app.confirmMutation, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/platform-entity-mappings/mapping_1/emergency-pause-change-sets", `{"expected_mapping_version":3,"current_daily_budget_minor":30000,"current_platform_status":"delivering"}`))
	if response.Code != http.StatusCreated || app.pauseCompile.ExpectedMappingVersion != 3 || app.pauseCompile.CurrentPlatformStatus != "delivering" || !strings.Contains(response.Body.String(), `"id":"change_mutation"`) {
		t.Fatalf("compile pause status=%d request=%#v body=%s", response.Code, app.pauseCompile, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/platform-entity-mappings/mapping_1/controlled-restart-change-sets", `{"expected_mapping_version":4,"current_daily_budget_minor":30000,"approved_daily_budget_minor":30000,"current_platform_status":"paused","schedule":{"start_at":"2026-08-14T00:00:00+08:00","end_at":"2026-08-15T00:00:00+08:00","timezone":"Asia/Shanghai"},"materials":[{"reference_id":"asset_test","authorization_evidence_id":"material_evidence_test"}],"landing_page":{"reference_id":"landing_test","authorization_evidence_id":"landing_evidence_test"}}`))
	if response.Code != http.StatusCreated || app.restartCompile.ExpectedMappingVersion != 4 || app.restartCompile.CurrentPlatformStatus != "paused" || app.restartCompile.LandingPage.ReferenceID != "landing_test" || !strings.Contains(response.Body.String(), `"id":"change_mutation"`) {
		t.Fatalf("compile restart status=%d request=%#v body=%s", response.Code, app.restartCompile, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/platform-entity-mappings/mapping_1:confirm-change", `{"expected_version":3,"business_execution_id":"pause_execution","result_evidence_id":"pause_result","list_evidence_id":"pause_list"}`))
	if response.Code != http.StatusOK || app.confirmChange.BusinessExecutionID != "pause_execution" || !strings.Contains(response.Body.String(), `"revision"`) {
		t.Fatalf("confirm change status=%d request=%#v body=%s", response.Code, app.confirmChange, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/controlled-change-sets/change_mutation:invalidate-calibration", `{"expected_version":3}`))
	if response.Code != http.StatusOK || app.invalidatedChangeSetID != "change_mutation" || app.invalidateCalibration.ExpectedVersion != 3 || !strings.Contains(response.Body.String(), `"status":"cancelled"`) {
		t.Fatalf("invalidate calibration status=%d id=%q request=%#v body=%s", response.Code, app.invalidatedChangeSetID, app.invalidateCalibration, response.Body.String())
	}
}

func TestDeliveryHTTPMapsProjectIsolationDenial(t *testing.T) {
	response := httptest.NewRecorder()
	request := authenticatedRequest(http.MethodGet, "/api/delivery/v1/projects/project_other/plans/plan_1", "")
	writeError(response, request, identity.ErrProjectAccessDenied)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "PROJECT_ACCESS_DENIED") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestLegacyConfigurationWriteEndpointsAreStableReadOnlyFailures(t *testing.T) {
	server := New(&applicationStub{})
	for _, path := range []string{
		"/api/delivery/v1/projects/project_1/plans/plan_1/configuration:compile",
		"/api/delivery/v1/projects/project_1/plans/plan_1/configuration:override",
	} {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, authenticatedRequest(http.MethodPost, path, `{}`))
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "LEGACY_CONFIGURATION_UNSUPPORTED") {
			t.Fatalf("path=%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}

func TestDeliveryHTTPMapsContractErrorsWithoutHidingTheStableCode(t *testing.T) {
	response := httptest.NewRecorder()
	request := authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/plans", `{}`)
	writeError(response, request, &delivery.DeliveryContractError{Code: delivery.ContractErrorCanonicalHashMismatch, Field: "canonical_hash", Message: "mismatch"})
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), delivery.ContractErrorCanonicalHashMismatch) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestApprovalRequestRejectsInjectedIdentityAndScopeFields(t *testing.T) {
	t.Parallel()
	server := New(&applicationStub{
		changeSet: delivery.ChangeSet{ID: "deliverychangeset_1", Version: 2},
	})
	fields := []string{"actor", "role", "approver", "scope"}
	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := authenticatedRequest(
				http.MethodPost,
				"/api/delivery/v1/projects/project_1/change-sets/deliverychangeset_1:approve",
				`{"expected_version":2,"`+field+`":"forged"}`,
			)
			server.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("injected %s status=%d body=%s", field, response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), `"source":"mock"`) ||
				!strings.Contains(response.Body.String(), `"scenario":"invalid_request"`) {
				t.Fatalf("injected %s response lacks mock provenance: %s", field, response.Body.String())
			}
		})
	}
}

func TestRejectChangeSetHTTPRequiresReasonAndReturnsDurableDecision(t *testing.T) {
	t.Parallel()
	server := New(&applicationStub{changeSet: delivery.ChangeSet{ID: "deliverychangeset_1", Version: 2}})

	response := httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/change-sets/deliverychangeset_1:reject", `{"expected_version":2,"reason":""}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("empty reason status=%d body=%s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/change-sets/deliverychangeset_1:reject", `{"expected_version":2,"reason":"needs revision"}`))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"rejected"`) || !strings.Contains(response.Body.String(), `"rejection_reason":"needs revision"`) {
		t.Fatalf("reject status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDeliveryHTTPMapsStableApprovalErrorsWithMockProvenance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "required", err: delivery.ErrApprovalRequired, status: http.StatusConflict, code: "APPROVAL_REQUIRED"},
		{name: "expired", err: delivery.ErrApprovalExpired, status: http.StatusConflict, code: "APPROVAL_EXPIRED"},
		{name: "content mismatch", err: delivery.ErrApprovalContentMismatch, status: http.StatusConflict, code: "APPROVAL_CONTENT_MISMATCH"},
		{name: "scope exceeded", err: delivery.ErrApprovalScopeExceeded, status: http.StatusForbidden, code: "APPROVAL_SCOPE_EXCEEDED"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/change-sets/change_1:execute", `{"expected_version":3}`)
			writeError(response, request, testCase.err)
			if response.Code != testCase.status ||
				!strings.Contains(response.Body.String(), `"`+testCase.code+`"`) ||
				!strings.Contains(response.Body.String(), `"source":"mock"`) ||
				!strings.Contains(response.Body.String(), `"scenario":"`+strings.ToLower(testCase.code)+`"`) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestExecutionHTTPRequiresIdempotencyKeyAndCreates(t *testing.T) {
	server := New(&applicationStub{changeSet: delivery.ChangeSet{ID: "change_1", Version: 2}})
	request := authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/change-sets/change_1:execute", `{"expected_version":2,"scenario":"success"}`)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing key status=%d body=%s", response.Code, response.Body.String())
	}
	request = authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/change-sets/change_1:execute", `{"expected_version":2,"scenario":"success"}`)
	request.Header.Set("Idempotency-Key", "key_1")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDeliveryTourHTTPUsesStableActionRoutes(t *testing.T) {
	t.Parallel()
	app := &applicationStub{tourRun: delivery.DeliveryTourRun{ID: "investor-tour-01", Status: delivery.TourRunPrepared}}
	server := New(app)

	response := httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/tour-runs/investor-tour-01:prepare", ""))
	if response.Code != http.StatusCreated || app.tourRunID != "investor-tour-01" || !strings.Contains(response.Body.String(), `"status":"prepared"`) {
		t.Fatalf("prepare status=%d run=%q body=%s", response.Code, app.tourRunID, response.Body.String())
	}

	app.tourReplay = true
	response = httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/tour-runs/investor-tour-01:prepare", ""))
	if response.Code != http.StatusOK {
		t.Fatalf("replay status=%d body=%s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/delivery/v1/projects/project_1/tour-runs/investor-tour-01", ""))
	if response.Code != http.StatusOK || app.tourRunID != "investor-tour-01" {
		t.Fatalf("get status=%d run=%q body=%s", response.Code, app.tourRunID, response.Body.String())
	}

	response = httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/tour-runs/investor-tour-01:reset", ""))
	if response.Code != http.StatusOK || app.tourRunID != "investor-tour-01" {
		t.Fatalf("reset status=%d run=%q body=%s", response.Code, app.tourRunID, response.Body.String())
	}
}

func TestDeliveryTourHTTPMapsOwnerMismatch(t *testing.T) {
	t.Parallel()
	response := httptest.NewRecorder()
	request := authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/tour-runs/investor-tour-01:reset", "")
	writeError(response, request, delivery.ErrTourOwnerMismatch)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "TOUR_OWNER_MISMATCH") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDecisionWorkflowHTTPStopsAtReadyForFinalApproval(t *testing.T) {
	app := &applicationStub{
		decision:  delivery.DeliveryDecision{ID: "decision_1", SchemaVersion: delivery.DeliveryDecisionSchemaV1},
		selection: delivery.DecisionSelection{ID: "selection_1", Workflow: delivery.CompiledDeliveryWorkflow{Status: "ready_for_final_approval", RemoteWriteEnabled: false}},
	}
	server := New(app)

	response := httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/plans/plan_1/decisions:generate", `{"expected_version":1}`))
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"schema_version":"delivery-decision/v1"`) {
		t.Fatalf("generate status=%d body=%s", response.Code, response.Body.String())
	}

	request := authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/decisions/decision_1:select", `{"candidate_id":"decision_1-balanced","expected_plan_version":1}`)
	request.Header.Set("Idempotency-Key", "decision-selection-1")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"status":"ready_for_final_approval"`) || !strings.Contains(response.Body.String(), `"remote_write_enabled":false`) {
		t.Fatalf("select status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestObservatoryHTTPExposesReplayAndAuditableFeedback(t *testing.T) {
	app := &applicationStub{
		observatoryRun:      delivery.DeliveryObservatoryRun{ID: "observatory_1", SchemaVersion: delivery.ObservatoryRunSchemaV1, Source: delivery.ObservatorySourceReplay, Status: "completed", Outcome: "drift_detected", RemoteWriteEnabled: false},
		observatoryFeedback: delivery.DeliveryObservatoryFeedback{ID: "feedback_1", SchemaVersion: delivery.ObservatoryFeedbackSchemaV1, Disposition: delivery.ObservatoryFeedbackAccepted},
	}
	server := New(app)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/decision-selections/selection_1/observatory-runs", `{"source":"replay","mode":"observe_existing","fixture":{"fixture_id":"fixture_1","data_state":"ready","observed_at":"2026-08-12T08:00:00Z","data_through":"2026-08-12T07:55:00Z","observed_values":{},"selector_matches":{},"evidence_refs":["replay://fixture/1"],"page_refs":[]}}`))
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"outcome":"drift_detected"`) || !strings.Contains(response.Body.String(), `"remote_write_enabled":false`) {
		t.Fatalf("run status=%d body=%s", response.Code, response.Body.String())
	}

	request := authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/observatory-runs/observatory_1/feedback", `{"disposition":"accepted","reason":"reviewed evidence","diff_keys":[]}`)
	request.Header.Set("Idempotency-Key", "feedback-1")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"disposition":"accepted"`) {
		t.Fatalf("feedback status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestStartBrowserRpaExecutionHTTPReturnsRealRunID(t *testing.T) {
	app := &applicationStub{browserRpaExecution: delivery.StartBrowserRpaExecutionResult{BrowserRpaRun: delivery.BrowserRpaLaunchResult{RunID: "curun_real_1"}}}
	server := New(app)
	request := authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/plans/plan_1/browser-rpa-runs", `{"expected_version":3,"execution_driver":"playwright-rpa/edge/v3"}`)
	request.Header.Set("Idempotency-Key", "start-real-run-1")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"run_id":"curun_real_1"`) || app.startedPlanID != "plan_1" || app.startedBrowserRpa.ExpectedVersion != 3 || app.startedBrowserRpa.ExecutionDriver != browserautomation.ExecutionDriverPlaywrightEdgeV3 || app.startedBrowserRpa.IdempotencyKey != "start-real-run-1" {
		t.Fatalf("start Browser RPA status=%d body=%s request=%#v", response.Code, response.Body.String(), app.startedBrowserRpa)
	}
}

func authenticatedRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	ctx := contract.WithRequestContext(request.Context(), contract.RequestContext{
		RequestID: "req_1", TraceID: "trace_1",
		Actor: contract.ActorContext{
			OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user_1"},
			Scopes: []contract.Scope{delivery.ScopeRead, delivery.ScopeWrite, delivery.ScopeApprove, delivery.ScopeExecute},
		},
	})
	return request.WithContext(ctx)
}

type applicationStub struct {
	plan                delivery.DeliveryPlan
	changeSet           delivery.ChangeSet
	createdPlanID       string
	tourRun             delivery.DeliveryTourRun
	tourRunID           string
	tourReplay          bool
	decision            delivery.DeliveryDecision
	selection           delivery.DecisionSelection
	observatoryRun      delivery.DeliveryObservatoryRun
	observatoryFeedback delivery.DeliveryObservatoryFeedback
	browserRpaExecution delivery.StartBrowserRpaExecutionResult
	startedPlanID       string
	startedBrowserRpa   delivery.StartBrowserRpaExecutionRequest
}

func (s *applicationStub) StartBrowserRpaExecution(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, planID string, request delivery.StartBrowserRpaExecutionRequest) (delivery.StartBrowserRpaExecutionResult, error) {
	s.startedPlanID = planID
	s.startedBrowserRpa = request
	return s.browserRpaExecution, nil
}

type mappingApplicationStub struct {
	applicationStub
	mapping                delivery.PlatformEntityMapping
	created                delivery.PlatformEntityMapping
	confirm                delivery.ConfirmPlatformEntityMappingRequest
	confirmMutation        delivery.ConfirmPlatformEntityMappingMutationRequest
	confirmChange          delivery.ConfirmPlatformEntityMappingChangeRequest
	controlledChange       delivery.ControlledChangeSet
	mappedCompile          delivery.CompileMappedControlledChangeSetRequest
	pauseCompile           delivery.CompileEmergencyPauseChangeSetRequest
	restartCompile         delivery.CompileControlledRestartChangeSetRequest
	invalidatedChangeSetID string
	invalidateCalibration  delivery.InvalidateCalibratedControlledChangeSetRequest
}

func (s *mappingApplicationStub) CreatePendingPlatformEntityMapping(_ context.Context, _ contract.ActorContext, value delivery.PlatformEntityMapping) (delivery.PlatformEntityMapping, error) {
	s.created = value
	return s.mapping, nil
}
func (s *mappingApplicationStub) GetPlatformEntityMapping(context.Context, contract.ActorContext, contract.ProjectID, string) (delivery.PlatformEntityMapping, error) {
	return s.mapping, nil
}
func (s *mappingApplicationStub) ListPlatformEntityMappings(context.Context, contract.ActorContext, contract.ProjectID, string) ([]delivery.PlatformEntityMapping, error) {
	return []delivery.PlatformEntityMapping{s.mapping}, nil
}
func (s *mappingApplicationStub) ConfirmPlatformEntityMapping(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, _ string, request delivery.ConfirmPlatformEntityMappingRequest) (delivery.PlatformEntityMapping, error) {
	s.confirm = request
	value := s.mapping
	value.Status, value.PlatformObjectID, value.PlatformStatus, value.ResultEvidenceID, value.ListEvidenceID = delivery.PlatformEntityMappingConfirmed, "platform_1", "pending_review", request.ResultEvidenceID, request.ListEvidenceID
	return value, nil
}
func (s *mappingApplicationStub) ConfirmPlatformEntityMappingMutation(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, _ string, request delivery.ConfirmPlatformEntityMappingMutationRequest) (delivery.PlatformEntityMapping, delivery.PlatformEntityMappingRevision, error) {
	s.confirmMutation = request
	value := s.mapping
	value.Version++
	return value, delivery.PlatformEntityMappingRevision{MappingID: value.ID, Version: value.Version, BusinessExecutionID: request.BusinessExecutionID, ResultEvidenceID: request.ResultEvidenceID, ListEvidenceID: request.ListEvidenceID}, nil
}
func (s *mappingApplicationStub) ConfirmPlatformEntityMappingChange(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, _ string, request delivery.ConfirmPlatformEntityMappingChangeRequest) (delivery.PlatformEntityMapping, delivery.PlatformEntityMappingRevision, error) {
	s.confirmChange = request
	value := s.mapping
	value.Version++
	return value, delivery.PlatformEntityMappingRevision{MappingID: value.ID, Version: value.Version, BusinessExecutionID: request.BusinessExecutionID, ResultEvidenceID: request.ResultEvidenceID, ListEvidenceID: request.ListEvidenceID}, nil
}
func (s *mappingApplicationStub) CompileControlledChangeSet(context.Context, contract.ActorContext, contract.ProjectID, delivery.CompileControlledChangeSetRequest) (delivery.ControlledChangeSet, bool, error) {
	return s.controlledChange, false, nil
}
func (s *mappingApplicationStub) CompileMappedControlledChangeSet(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, _ string, request delivery.CompileMappedControlledChangeSetRequest) (delivery.ControlledChangeSet, bool, error) {
	s.mappedCompile = request
	return s.controlledChange, false, nil
}
func (s *mappingApplicationStub) CompileEmergencyPauseChangeSet(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, _ string, request delivery.CompileEmergencyPauseChangeSetRequest) (delivery.ControlledChangeSet, bool, error) {
	s.pauseCompile = request
	return s.controlledChange, false, nil
}
func (s *mappingApplicationStub) CompileControlledRestartChangeSet(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, _ string, request delivery.CompileControlledRestartChangeSetRequest) (delivery.ControlledChangeSet, bool, error) {
	s.restartCompile = request
	return s.controlledChange, false, nil
}
func (s *mappingApplicationStub) GetControlledChangeSet(context.Context, contract.ActorContext, contract.ProjectID, string) (delivery.ControlledChangeSet, error) {
	return s.controlledChange, nil
}
func (s *mappingApplicationStub) InvalidateCalibratedControlledChangeSet(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, id string, request delivery.InvalidateCalibratedControlledChangeSetRequest) (delivery.ControlledChangeSet, delivery.ControlledExecution, error) {
	s.invalidatedChangeSetID = id
	s.invalidateCalibration = request
	return s.controlledChange, delivery.ControlledExecution{Status: "cancelled"}, nil
}
func (s *mappingApplicationStub) ApproveControlledChangeSet(context.Context, contract.ActorContext, contract.ProjectID, string, delivery.ApproveControlledChangeSetRequest) (delivery.ControlledChangeSet, delivery.RemoteWriteApproval, error) {
	return s.controlledChange, delivery.RemoteWriteApproval{}, nil
}
func (s *mappingApplicationStub) CreateControlledExecution(context.Context, contract.ActorContext, contract.ProjectID, string) (delivery.ControlledExecution, error) {
	return delivery.ControlledExecution{}, nil
}
func (s *mappingApplicationStub) GetControlledExecution(context.Context, contract.ActorContext, contract.ProjectID, string) (delivery.ControlledExecution, error) {
	return delivery.ControlledExecution{}, nil
}

func (s *applicationStub) GenerateDecision(context.Context, contract.ActorContext, contract.ProjectID, string, int) (delivery.DeliveryDecision, error) {
	return s.decision, nil
}
func (s *applicationStub) ListDecisions(context.Context, contract.ActorContext, contract.ProjectID, int) ([]delivery.DeliveryDecision, error) {
	return []delivery.DeliveryDecision{s.decision}, nil
}
func (s *applicationStub) GetDecision(context.Context, contract.ActorContext, contract.ProjectID, string) (delivery.DeliveryDecision, error) {
	return s.decision, nil
}
func (s *applicationStub) SelectDecision(context.Context, contract.ActorContext, contract.ProjectID, string, string, delivery.SelectDecisionRequest) (delivery.DecisionSelection, bool, error) {
	return s.selection, false, nil
}
func (s *applicationStub) GetDecisionSelection(context.Context, contract.ActorContext, contract.ProjectID, string) (delivery.DecisionSelection, error) {
	return s.selection, nil
}
func (s *applicationStub) RunObservatory(context.Context, contract.ActorContext, contract.ProjectID, string, delivery.RunObservatoryRequest) (delivery.DeliveryObservatoryRun, bool, error) {
	return s.observatoryRun, false, nil
}
func (s *applicationStub) ListObservatoryRuns(context.Context, contract.ActorContext, contract.ProjectID, int) ([]delivery.DeliveryObservatoryRun, error) {
	return []delivery.DeliveryObservatoryRun{s.observatoryRun}, nil
}
func (s *applicationStub) GetObservatoryRun(context.Context, contract.ActorContext, contract.ProjectID, string) (delivery.DeliveryObservatoryRun, error) {
	return s.observatoryRun, nil
}
func (s *applicationStub) SubmitObservatoryFeedback(context.Context, contract.ActorContext, contract.ProjectID, string, string, delivery.SubmitObservatoryFeedbackRequest) (delivery.DeliveryObservatoryFeedback, bool, error) {
	return s.observatoryFeedback, false, nil
}
func (s *applicationStub) ListObservatoryFeedback(context.Context, contract.ActorContext, contract.ProjectID, string, int) ([]delivery.DeliveryObservatoryFeedback, error) {
	return []delivery.DeliveryObservatoryFeedback{s.observatoryFeedback}, nil
}

func (s *applicationStub) CreatePlan(context.Context, contract.ActorContext, contract.ProjectID, delivery.CreatePlanRequest) (delivery.DeliveryPlan, error) {
	return s.plan, nil
}
func (s *applicationStub) UpdatePlan(context.Context, contract.ActorContext, contract.ProjectID, string, delivery.UpdatePlanRequest) (delivery.DeliveryPlan, error) {
	return s.plan, nil
}

type immutableIdentityConflictApplicationStub struct{ applicationStub }

func (s *immutableIdentityConflictApplicationStub) UpdatePlan(context.Context, contract.ActorContext, contract.ProjectID, string, delivery.UpdatePlanRequest) (delivery.DeliveryPlan, error) {
	return delivery.DeliveryPlan{}, delivery.ErrImmutableContractIdentityConflict
}
func (s *applicationStub) ListPlans(context.Context, contract.ActorContext, contract.ProjectID, int) ([]delivery.DeliveryPlan, error) {
	return []delivery.DeliveryPlan{s.plan}, nil
}
func (s *applicationStub) GetPlan(context.Context, contract.ActorContext, contract.ProjectID, string) (delivery.DeliveryPlan, error) {
	return s.plan, nil
}
func (s *applicationStub) ListPlanVersions(context.Context, contract.ActorContext, contract.ProjectID, string) ([]delivery.DeliveryPlanVersion, error) {
	return s.plan.Versions, nil
}
func (s *applicationStub) GetPlanVersion(context.Context, contract.ActorContext, contract.ProjectID, string, int) (delivery.DeliveryPlanVersion, error) {
	return s.plan.CurrentVersion, nil
}
func (s *applicationStub) RunPlanPreflight(context.Context, contract.ActorContext, contract.ProjectID, string) (delivery.PreflightResult, error) {
	return delivery.PreflightResult{PlanID: s.plan.ID, Source: delivery.SourceMock}, nil
}
func (s *applicationStub) GetPlanDetail(context.Context, contract.ActorContext, contract.ProjectID, string) (delivery.PlanDetail, error) {
	return delivery.PlanDetail{Plan: s.plan}, nil
}
func (s *applicationStub) ListChangeSets(context.Context, contract.ActorContext, contract.ProjectID, int) ([]delivery.ChangeSet, error) {
	return []delivery.ChangeSet{s.changeSet}, nil
}
func (s *applicationStub) GetChangeSet(context.Context, contract.ActorContext, contract.ProjectID, string) (delivery.ChangeSet, error) {
	return s.changeSet, nil
}
func (s *applicationStub) CreateChangeSet(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, planID string, _ int64) (delivery.ChangeSet, error) {
	s.createdPlanID = planID
	return s.changeSet, nil
}
func (s *applicationStub) Preflight(context.Context, contract.ActorContext, contract.ProjectID, string, int64) (delivery.ChangeSet, error) {
	return s.changeSet, nil
}
func (s *applicationStub) Approve(context.Context, contract.ActorContext, contract.ProjectID, string, int64) (delivery.ChangeSet, error) {
	return s.changeSet, nil
}

func (s *applicationStub) RejectChangeSet(context.Context, contract.ActorContext, contract.ProjectID, string, delivery.RejectChangeSetRequest) (delivery.ChangeSet, error) {
	return delivery.ChangeSet{ID: "changeset-rejected", Status: delivery.ChangeSetRejected, Version: 2, RejectionReason: "needs revision"}, nil
}
func (s *applicationStub) Execute(context.Context, contract.ActorContext, contract.ProjectID, string, string, delivery.ExecuteRequest) (delivery.ExecutionResult, bool, error) {
	now := time.Now()
	return delivery.ExecutionResult{ChangeSet: s.changeSet, Execution: delivery.Execution{CompletedAt: &now}}, false, nil
}
func (s *applicationStub) ExecutePlan(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, _, _ string, _ delivery.ExecutePlanRequest) (delivery.ExecutionResult, bool, error) {
	now := time.Now()
	return delivery.ExecutionResult{ChangeSet: s.changeSet, Execution: delivery.Execution{CompletedAt: &now}}, false, nil
}
func (s *applicationStub) Rollback(context.Context, contract.ActorContext, contract.ProjectID, string, int64) (delivery.ChangeSet, error) {
	return s.changeSet, nil
}

func (s *applicationStub) GenerateRecommendation(context.Context, contract.ActorContext, contract.ProjectID, string, int) (delivery.DeliveryRecommendation, error) {
	return delivery.DeliveryRecommendation{}, nil
}
func (s *applicationStub) ListRecommendations(context.Context, contract.ActorContext, contract.ProjectID, int) ([]delivery.DeliveryRecommendation, error) {
	return nil, nil
}
func (s *applicationStub) GetRecommendation(context.Context, contract.ActorContext, contract.ProjectID, string) (delivery.DeliveryRecommendation, error) {
	return delivery.DeliveryRecommendation{}, nil
}
func (s *applicationStub) AcceptRecommendation(context.Context, contract.ActorContext, contract.ProjectID, string, string, int64) (delivery.RecommendationAcceptance, bool, error) {
	return delivery.RecommendationAcceptance{}, false, nil
}
func (s *applicationStub) RejectRecommendation(context.Context, contract.ActorContext, contract.ProjectID, string, int64) (delivery.DeliveryRecommendation, error) {
	return delivery.DeliveryRecommendation{}, nil
}
func (s *applicationStub) GetManualActionPackage(context.Context, contract.ActorContext, contract.ProjectID, string) (delivery.ManualActionPackage, error) {
	return delivery.ManualActionPackage{}, nil
}
func (s *applicationStub) ListExecutions(context.Context, contract.ActorContext, contract.ProjectID, int) ([]delivery.ExecutionResult, error) {
	return nil, nil
}
func (s *applicationStub) GetExecution(context.Context, contract.ActorContext, contract.ProjectID, string) (delivery.ExecutionResult, error) {
	return delivery.ExecutionResult{}, nil
}
func (s *applicationStub) CreateOutcomeSimulation(context.Context, contract.ActorContext, contract.ProjectID, string, delivery.CreateOutcomeSimulationRequest) (delivery.OutcomeSimulationResult, error) {
	return delivery.OutcomeSimulationResult{Run: delivery.OutcomeSimulationRun{ID: "deliverysimulationrun_1"}}, nil
}
func (s *applicationStub) GetLatestOutcomeSimulation(context.Context, contract.ActorContext, contract.ProjectID, string) (delivery.OutcomeSimulationResult, error) {
	return delivery.OutcomeSimulationResult{Run: delivery.OutcomeSimulationRun{ID: "deliverysimulationrun_1"}, Replay: true}, nil
}
func (s *applicationStub) CreateDemoMetricSnapshot(context.Context, contract.ActorContext, contract.ProjectID, string, delivery.CreateMetricSnapshotRequest) (delivery.DeliveryMetricSnapshot, error) {
	return delivery.DeliveryMetricSnapshot{ID: "deliverymetric_1", IsSimulated: true}, nil
}
func (s *applicationStub) ListMetricSnapshots(context.Context, contract.ActorContext, contract.ProjectID, string, int) ([]delivery.DeliveryMetricSnapshot, error) {
	return []delivery.DeliveryMetricSnapshot{{ID: "deliverymetric_1", IsSimulated: true}}, nil
}
func (s *applicationStub) EvaluateAlerts(context.Context, contract.ActorContext, contract.ProjectID, delivery.EvaluateAlertsRequest) (delivery.EvaluateAlertsResponse, error) {
	return delivery.EvaluateAlertsResponse{Items: []delivery.DeliveryAlert{}}, nil
}
func (s *applicationStub) InspectConnectorAlerts(context.Context, contract.ActorContext, contract.ProjectID, delivery.ConnectorInspectionRequest) (delivery.ConnectorInspectionResponse, error) {
	return delivery.ConnectorInspectionResponse{Items: []delivery.DeliveryAlert{}, Source: "connector"}, nil
}
func (s *applicationStub) ListAlerts(context.Context, contract.ActorContext, contract.ProjectID, delivery.AlertFilter) ([]delivery.DeliveryAlert, error) {
	return []delivery.DeliveryAlert{}, nil
}
func (s *applicationStub) UpdateAlert(context.Context, contract.ActorContext, contract.ProjectID, string, delivery.UpdateAlertRequest) (delivery.DeliveryAlert, error) {
	return delivery.DeliveryAlert{}, nil
}
func (s *applicationStub) PrepareTourRun(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, runID string) (delivery.DeliveryTourRun, bool, error) {
	s.tourRunID = runID
	return s.tourRun, s.tourReplay, nil
}
func (s *applicationStub) GetTourRun(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, runID string) (delivery.DeliveryTourRun, error) {
	s.tourRunID = runID
	return s.tourRun, nil
}
func (s *applicationStub) ResetTourRun(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, runID string) (delivery.DeliveryTourResetResult, error) {
	s.tourRunID = runID
	return delivery.DeliveryTourResetResult{Run: s.tourRun}, nil
}
