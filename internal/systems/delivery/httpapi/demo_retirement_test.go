package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/systems/delivery"
)

func TestRetiredDemoWritesReturnGoneWithoutCallingApplication(t *testing.T) {
	server := New(nil)
	for _, path := range []string{
		"tour-runs/old-run:prepare", "tour-runs/old-run:reset",
		"plans/old-plan/recommendations:generate",
		"recommendations/old-recommendation:accept", "recommendations/old-recommendation:reject",
		"executions/old-execution/simulation-runs", "executions/old-execution/metric-snapshots",
		"alerts:evaluate",
	} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/"+path, `{}`))
			var body struct {
				Error contract.Error `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusGone || body.Error.Code != "DELIVERY_DEMO_RETIRED" || body.Error.Retryable {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestHistoricalTourReadRemainsAvailable(t *testing.T) {
	app := &applicationStub{tourRun: delivery.DeliveryTourRun{ID: "old-run", Source: delivery.SourceMock, Status: delivery.TourRunPrepared}}
	response := httptest.NewRecorder()
	New(app).ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/delivery/v1/projects/project_1/tour-runs/old-run", ""))
	var body delivery.DeliveryTourRun
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || body.ID != "old-run" || body.Source != delivery.SourceMock {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestMissingExecutionAdapterHasExplicitHTTPError(t *testing.T) {
	response := httptest.NewRecorder()
	writeError(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/plans/plan_1/execute", ""), delivery.ErrExecutionUnavailable)
	var body struct {
		Error contract.Error `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusServiceUnavailable || body.Error.Code != "EXECUTION_UNAVAILABLE" || body.Error.Retryable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
