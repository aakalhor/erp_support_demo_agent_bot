package http

import (
	"net/http"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/shared/logger"
)

// NewRouter wires the application handlers to URL paths. The MVP uses
// the stdlib mux; Go 1.22's method-aware patterns ("POST /ask") would
// also work but plain registration keeps compatibility broad.
func NewRouter(ask *AskHandler, health *HealthHandler, log logger.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/health", health)
	mux.Handle("/ask", ask)
	return logRequests(mux, log)
}

// logRequests is a minimal access-log middleware. Useful when watching
// the API while exercising the bot.
func logRequests(next http.Handler, log logger.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Infof("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
