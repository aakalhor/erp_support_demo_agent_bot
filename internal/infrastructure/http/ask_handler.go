package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/usecase"
)

// askDeadline is the hard cap on a single /ask request. Bigger than the
// expected p95 but bounded so a runaway LLM never holds a connection
// forever.
const askDeadline = 4 * time.Minute

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

	ctx, cancel := context.WithTimeout(r.Context(), askDeadline)
	defer cancel()
	r = r.WithContext(ctx)

	var req domain.AskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body: " + err.Error()})
		return
	}

	// Run the use case in a goroutine so we can return a clean timeout
	// response if the deadline fires before AskService finishes.
	type result struct {
		resp domain.AskResponse
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := h.service.Ask(req)
		done <- result{resp: resp, err: err}
	}()

	select {
	case <-ctx.Done():
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{
			"error": "request timed out; the LLM or translator did not respond in time",
		})
		return
	case res := <-done:
		if res.err != nil {
			switch {
			case errors.Is(res.err, domain.ErrInvalidRequest):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "question must be non-empty"})
			case errors.Is(res.err, domain.ErrIndexNotFound):
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{
					"error": "search index not loaded; run: go run ./cmd/indexer",
				})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": res.err.Error()})
			}
			return
		}
		writeJSON(w, http.StatusOK, res.resp)
	}
}
