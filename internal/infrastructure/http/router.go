package http

import (
	"net/http"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/shared/logger"
)

// NewRouter wires the application handlers to URL paths. Uses the
// Go 1.22 method-aware mux so we can mount the admin form routes
// alongside the JSON API on the same port.
//
// adminHandler may be nil if the admin UI is disabled at startup.
func NewRouter(ask *AskHandler, health *HealthHandler, adminHandler *AdminHandler, log logger.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/health", health)
	mux.Handle("/ask", ask)
	if adminHandler != nil {
		adminHandler.Register(mux)
	}
	return logRequests(mux, log)
}

// logRequests is a minimal access-log middleware.
func logRequests(next http.Handler, log logger.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Infof("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
