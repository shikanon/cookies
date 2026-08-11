package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/systems/creative"
)

func (s *Server) listProductionRuns(w http.ResponseWriter, r *http.Request) {
	if s.productionCenter == nil {
		writeProblem(w, http.StatusServiceUnavailable, contract.Error{Code: "PRODUCTION_SOURCE_UNAVAILABLE", Message: "Production center query is not configured.", RequestID: requestIDFrom(r.Context()), Retryable: true})
		return
	}
	request, err := parseProductionRunQuery(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, contract.Error{Code: "INVALID_REQUEST", Message: err.Error(), RequestID: requestIDFrom(r.Context()), Retryable: false})
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	page, err := s.productionCenter.ListRuns(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), request)
	if err != nil {
		writeProductionProblem(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) getProductionRun(w http.ResponseWriter, r *http.Request) {
	if s.productionCenter == nil {
		writeProblem(w, http.StatusServiceUnavailable, contract.Error{Code: "PRODUCTION_SOURCE_UNAVAILABLE", Message: "Production center query is not configured.", RequestID: requestIDFrom(r.Context()), Retryable: true})
		return
	}
	source := creative.ProductionRunSourceKind(r.PathValue("production_source"))
	if !validProductionSource(source) || strings.TrimSpace(r.PathValue("production_run_id")) == "" {
		writeProblem(w, http.StatusBadRequest, contract.Error{Code: "INVALID_REQUEST", Message: "Production source and run ID are invalid.", RequestID: requestIDFrom(r.Context()), Retryable: false})
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	detail, err := s.productionCenter.GetRun(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), creative.ProductionRunRef{Source: source, ID: r.PathValue("production_run_id")})
	if err != nil {
		writeProductionProblem(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) listProductionAssets(w http.ResponseWriter, r *http.Request) {
	if s.productionAssets == nil {
		writeProblem(w, http.StatusServiceUnavailable, contract.Error{Code: "PRODUCTION_SOURCE_UNAVAILABLE", Message: "Production asset query is not configured.", RequestID: requestIDFrom(r.Context()), Retryable: true})
		return
	}
	request, err := parseProductionAssetQuery(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, contract.Error{Code: "INVALID_REQUEST", Message: err.Error(), RequestID: requestIDFrom(r.Context()), Retryable: false})
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	page, err := s.productionAssets.ListAssets(r.Context(), rc.Actor, contract.ProjectID(r.PathValue("project_id")), request)
	if err != nil {
		writeProductionProblem(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) retryProductionRun(w http.ResponseWriter, r *http.Request) {
	if s.productionRetry == nil {
		writeProblem(w, http.StatusServiceUnavailable, contract.Error{Code: "PRODUCTION_SOURCE_UNAVAILABLE", Message: "Production retry is not configured.", RequestID: requestIDFrom(r.Context()), Retryable: true})
		return
	}
	source := creative.ProductionRunSourceKind(r.PathValue("production_source"))
	action := strings.TrimSpace(r.PathValue("production_run_action"))
	if !validProductionSource(source) || !strings.HasSuffix(action, ":retry") {
		writeProblem(w, http.StatusBadRequest, contract.Error{Code: "INVALID_REQUEST", Message: "Production retry target is invalid.", RequestID: requestIDFrom(r.Context()), Retryable: false})
		return
	}
	runID := strings.TrimSuffix(action, ":retry")
	if strings.TrimSpace(runID) == "" {
		writeProblem(w, http.StatusBadRequest, contract.Error{Code: "INVALID_REQUEST", Message: "Production run ID is required.", RequestID: requestIDFrom(r.Context()), Retryable: false})
		return
	}
	key := contract.IdempotencyKey(strings.TrimSpace(r.Header.Get("Idempotency-Key")))
	if err := key.Validate(); err != nil {
		writeProblem(w, http.StatusBadRequest, contract.Error{Code: "IDEMPOTENCY_KEY_INVALID", Message: "A valid Idempotency-Key header is required.", RequestID: requestIDFrom(r.Context()), Retryable: false})
		return
	}
	rc, _ := contract.RequestContextFrom(r.Context())
	result, err := s.productionRetry.Retry(r.Context(), rc, contract.ProjectID(r.PathValue("project_id")), creative.ProductionRunRef{Source: source, ID: runID}, key)
	if err != nil {
		writeProductionRetryProblem(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func parseProductionRunQuery(r *http.Request) (creative.ListProductionRunsRequest, error) {
	values := r.URL.Query()
	request := creative.ListProductionRunsRequest{Cursor: values.Get("cursor"), SourceTaskID: values.Get("source_task_id"), Query: values.Get("q")}
	if value := values.Get("media_kind"); value != "" {
		request.MediaKind = creative.ProductionMediaKind(value)
		if request.MediaKind != creative.ProductionMediaImage && request.MediaKind != creative.ProductionMediaVideo && request.MediaKind != creative.ProductionMediaAudio && request.MediaKind != creative.ProductionMediaRender {
			return request, errors.New("media_kind is invalid")
		}
	}
	if value := values.Get("status"); value != "" {
		for _, raw := range strings.Split(value, ",") {
			status := creative.ProductionStatus(strings.TrimSpace(raw))
			if !validProductionStatus(status) {
				return request, errors.New("status is invalid")
			}
			request.Statuses = append(request.Statuses, status)
		}
	}
	if value := values.Get("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 100 {
			return request, errors.New("limit must be between 1 and 100")
		}
		request.Limit = limit
	}
	if value := values.Get("created_after"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return request, errors.New("created_after must be RFC3339")
		}
		request.CreatedAfter = &parsed
	}
	if value := values.Get("created_before"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return request, errors.New("created_before must be RFC3339")
		}
		request.CreatedBefore = &parsed
	}
	if len(request.SourceTaskID) > 96 || len(request.Query) > 200 || len(request.Cursor) > 8192 {
		return request, errors.New("production query parameter exceeds its maximum length")
	}
	return request, nil
}

func parseProductionAssetQuery(r *http.Request) (creative.ListProductionAssetsRequest, error) {
	values := r.URL.Query()
	request := creative.ListProductionAssetsRequest{Role: values.Get("role"), Cursor: values.Get("cursor")}
	if request.Role != "" && request.Role != "input" && request.Role != "output" {
		return request, errors.New("role is invalid")
	}
	if value := values.Get("media_kind"); value != "" {
		request.MediaKind = creative.ProductionMediaKind(value)
		if request.MediaKind != creative.ProductionMediaImage && request.MediaKind != creative.ProductionMediaVideo && request.MediaKind != creative.ProductionMediaAudio {
			return request, errors.New("media_kind is invalid")
		}
	}
	if value := values.Get("run_source"); value != "" {
		request.RunSource = creative.ProductionRunSourceKind(value)
		if !validProductionSource(request.RunSource) {
			return request, errors.New("run_source is invalid")
		}
	}
	if value := values.Get("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 100 {
			return request, errors.New("limit must be between 1 and 100")
		}
		request.Limit = limit
	}
	if len(request.Cursor) > 8192 {
		return request, errors.New("production query parameter exceeds its maximum length")
	}
	return request, nil
}

func validProductionStatus(status creative.ProductionStatus) bool {
	switch status {
	case creative.ProductionQueued, creative.ProductionRunning, creative.ProductionIngesting,
		creative.ProductionSucceeded, creative.ProductionPartiallySucceeded, creative.ProductionFailed,
		creative.ProductionExpired, creative.ProductionCancelled:
		return true
	default:
		return false
	}
}

func validProductionSource(source creative.ProductionRunSourceKind) bool {
	return source == creative.ProductionSourceProvider || source == creative.ProductionSourceCreativeRender || source == creative.ProductionSourceEditingRender || source == creative.ProductionSourceAudioRender
}

func writeProductionProblem(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, creative.ErrProductionCursorInvalid):
		writeProblem(w, http.StatusBadRequest, contract.Error{Code: "PRODUCTION_CURSOR_INVALID", Message: "The production cursor is invalid.", RequestID: requestIDFrom(r.Context()), Retryable: false})
	case errors.Is(err, creative.ErrProductionRunNotFound):
		writeProblem(w, http.StatusNotFound, contract.Error{Code: "PRODUCTION_RUN_NOT_FOUND", Message: "The production run was not found in this Project.", RequestID: requestIDFrom(r.Context()), Retryable: false})
	case errors.Is(err, creative.ErrProductionSourcesUnavailable):
		writeProblem(w, http.StatusServiceUnavailable, contract.Error{Code: "PRODUCTION_SOURCE_UNAVAILABLE", Message: "Production sources are temporarily unavailable.", RequestID: requestIDFrom(r.Context()), Retryable: true})
	default:
		writeProblem(w, http.StatusServiceUnavailable, contract.Error{Code: "PRODUCTION_SOURCE_UNAVAILABLE", Message: "The production query could not be completed.", RequestID: requestIDFrom(r.Context()), Retryable: true})
	}
}

func writeProductionRetryProblem(w http.ResponseWriter, r *http.Request, err error) {
	problem := creative.ProductionProblem{ContractVersion: "creative-production-problem/v1", Retryable: false}
	var sourceWorkflowError creative.ProductionRetryRequiresSourceWorkflowError
	if errors.As(err, &sourceWorkflowError) {
		problem.SourceTask = sourceWorkflowError.SourceTask
	}
	switch {
	case errors.Is(err, creative.ErrProductionRunNotFound):
		problem.Code, problem.Message = "PRODUCTION_RUN_NOT_FOUND", "The production run was not found in this Project."
		writeJSON(w, http.StatusNotFound, problem)
	case errors.Is(err, creative.ErrProductionRetryRequiresSourceWorkflow):
		problem.Code, problem.Message = "PRODUCTION_RETRY_REQUIRES_SOURCE_WORKFLOW", "Retry this run from its source workflow."
		writeJSON(w, http.StatusConflict, problem)
	case errors.Is(err, creative.ErrProductionInputAssetUnavailable):
		problem.Code, problem.Message = "PRODUCTION_INPUT_ASSET_UNAVAILABLE", "A frozen input asset is no longer available for this retry."
		writeJSON(w, http.StatusConflict, problem)
	case errors.Is(err, creative.ErrProductionIdempotencyConflict), errors.Is(err, creative.ErrIdempotencyConflict):
		problem.Code, problem.Message = "PRODUCTION_IDEMPOTENCY_CONFLICT", "The Idempotency-Key was already used for a different production retry."
		writeJSON(w, http.StatusConflict, problem)
	case errors.Is(err, creative.ErrProductionRetryNotAllowed), errors.Is(err, creative.ErrInvalidState):
		problem.Code, problem.Message = "PRODUCTION_RETRY_NOT_ALLOWED", "This production run cannot be retried from Production Center."
		writeJSON(w, http.StatusConflict, problem)
	default:
		problem.Code, problem.Message, problem.Retryable = "PRODUCTION_SOURCE_UNAVAILABLE", "The production retry could not be completed.", true
		writeJSON(w, http.StatusServiceUnavailable, problem)
	}
}
