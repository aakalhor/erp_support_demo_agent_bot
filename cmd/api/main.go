// Command api starts the local RAG HTTP API on PORT (default 8080).
// It refuses to start if the index file is missing — run the indexer
// first.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/answers"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/config"
	apphttp "github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/http"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/llm"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/search"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/translation"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/port"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/shared/logger"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/usecase"
)

func main() {
	log := logger.New("api")

	cfg, err := config.Load()
	if err != nil {
		log.Errorf("load config: %v", err)
		os.Exit(1)
	}

	idx, err := search.NewLoader().Load(cfg.IndexPath)
	if err != nil {
		if errors.Is(err, domain.ErrIndexNotFound) {
			log.Errorf("Index not found at %s. Run: go run ./cmd/indexer", cfg.IndexPath)
			os.Exit(1)
		}
		log.Errorf("load index: %v", err)
		os.Exit(1)
	}

	safetyGen := answers.NewTemplateAnswerGenerator()

	// LLM + translator are optional. If the local Ollama daemon is
	// unreachable we degrade gracefully to extractive-only answers.
	var primary port.AnswerGenerator = safetyGen
	var translator port.Translator
	if cfg.LLMEnabled {
		client := llm.NewClient(cfg.OllamaBaseURL, cfg.OllamaModel)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := client.Health(ctx); err != nil {
			log.Errorf("ollama health check failed at %s: %v — continuing with extractive-only answers", cfg.OllamaBaseURL, err)
		} else {
			log.Infof("ollama OK at %s (model=%s)", cfg.OllamaBaseURL, cfg.OllamaModel)
			primary = answers.NewOllamaAnswerGenerator(client)
			translator = translation.NewOllamaTranslator(client)
		}
		cancel()
	} else {
		log.Infof("LLM disabled by config; using extractive answers only")
	}

	ask := usecase.NewAskService(
		idx,
		usecase.NewIntentClassifier(),
		usecase.NewRiskDetector(),
		safetyGen,
		primary,
		translator,
	)

	router := apphttp.NewRouter(apphttp.NewAskHandler(ask), apphttp.NewHealthHandler(), log)

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Infof("listening on %s (index: %s)", addr, cfg.IndexPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("http server: %v", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Infof("shutdown signal received, stopping HTTP server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Errorf("shutdown: %v", err)
	}
}
