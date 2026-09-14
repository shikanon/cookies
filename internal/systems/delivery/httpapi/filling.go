package httpapi

import (
	"context"
	"errors"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/systems/delivery"
	"net/http"
)

func (s *Server) suggestFilling(w http.ResponseWriter, r *http.Request) {
	app, ok := s.app.(interface {
		SuggestFilling(context.Context, contract.ActorContext, contract.ProjectID, delivery.FillingRequest) (delivery.FillingResult, error)
	})
	if !ok {
		writeError(w, r, delivery.ErrInvalidState)
		return
	}
	var request delivery.FillingRequest
	if !decode(w, r, &request) {
		return
	}
	result, err := app.SuggestFilling(r.Context(), mustActor(r), projectID(r), request)
	if errors.Is(err, delivery.ErrFillingCatalogTooLarge) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]string{"code": "FILLING_SEARCH_REQUIRED", "message": "候选项或资料过多，请填写更具体的目录搜索词后重试。"}})
		return
	}
	if errors.Is(err, delivery.ErrFillingUnavailable) || errors.Is(err, delivery.ErrFillingInvalid) {
		message := "智能填写暂不可用，请检查真实文本模型配置或重试。"
		if errors.Is(err, delivery.ErrFillingInvalid) {
			message = "模型返回了无效建议，未修改草稿。请重试。"
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": map[string]string{"code": "FILLING_UNAVAILABLE", "message": message}})
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
