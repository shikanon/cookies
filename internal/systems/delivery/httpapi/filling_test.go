package httpapi

import (
	"context"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/systems/delivery"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fillingApplicationStub struct {
	applicationStub
	err   error
	calls int
}

func (s *fillingApplicationStub) SuggestFilling(context.Context, contract.ActorContext, contract.ProjectID, delivery.FillingRequest) (delivery.FillingResult, error) {
	s.calls++
	return delivery.FillingResult{Suggestions: []delivery.FillingSuggestion{}}, s.err
}

func TestFillingHTTPRejectsUnknownFieldsAndReportsProviderFailure(t *testing.T) {
	app := &fillingApplicationStub{err: delivery.ErrFillingUnavailable}
	server := New(app)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/filling-suggestions", `{"arbitrary_path":"x"}`))
	if response.Code != 400 || app.calls != 0 {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/api/delivery/v1/projects/project_1/filling-suggestions", `{"page":"plan","fields":["name"],"current":{},"ocean":{}}`))
	if response.Code != 503 || !strings.Contains(response.Body.String(), "FILLING_UNAVAILABLE") {
		t.Fatalf("%d %s", response.Code, response.Body.String())
	}
}
