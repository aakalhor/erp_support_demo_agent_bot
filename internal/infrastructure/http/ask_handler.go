package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/usecase"
)

// AskHandler is the HTTP adapter over the AskService use case.
type AskHandler struct {
	service *usecase.AskService
}

func NewAskHandler(s *usecase.AskService) *AskHandler { return &AskHandler{service: s} }

func (h *AskHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
		return
	}
	defer r.Body.Close()

	var req domain.AskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body: " + err.Error()})
		return
	}

	resp, err := h.service.Ask(req)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidRequest):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "question must be non-empty"})
		case errors.Is(err, domain.ErrIndexNotFound):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "search index not loaded; run: go run ./cmd/indexer",
			})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
