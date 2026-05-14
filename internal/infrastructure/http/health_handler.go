// Package http hosts the HTTP delivery layer (handlers + router). It
// uses the standard library's net/http so we don't pull in a heavier
// framework for two endpoints.
package http

import (
	"encoding/json"
	"net/http"
)

// HealthHandler responds with a small JSON payload so a caller can
// confirm the API is up.
type HealthHandler struct{}

func NewHealthHandler() *HealthHandler { return &HealthHandler{} }

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// writeJSON is the shared response encoder used by all handlers.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
