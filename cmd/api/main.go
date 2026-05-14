// Command api starts the local RAG HTTP API on PORT (default 8080).
// It also serves the admin UI at /admin so the knowledge base can be
// edited without restarting anything.
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
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/knowledge"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/llm"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/repository"
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

	loader := search.NewLoader()
	initialIdx, err := loader.Load(cfg.IndexPath)
	if err != nil {
		if errors.Is(err, domain.ErrIndexNotFound) {
			log.Errorf("Index not found at %s. Run: go run ./cmd/indexer", cfg.IndexPath)
			os.Exit(1)
		}
		log.Errorf("load index: %v", err)
		os.Exit(1)
	}
	// SwappableIndex lets the admin hot-reload the index after edits
	// without restarting the API.
	swappable := search.NewSwappableIndex(initialIdx)

	safetyGen := answers.NewTemplateAnswerGenerator()

	// General knowledge holder: loaded once, hot-updated by the admin
	// UI, consulted by the LLM on every /ask call.
	kb := knowledge.NewFileHolder(cfg.KnowledgePath)
	if err := kb.Load(); err != nil {
		log.Errorf("load general knowledge from %s: %v — continuing with empty knowledge", cfg.KnowledgePath, err)
	} else if kb.Get() != "" {
		log.Infof("general knowledge loaded from %s (%d chars)", cfg.KnowledgePath, len(kb.Get()))
	}

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
			primary = answers.NewOllamaAnswerGenerator(client, kb)
			translator = translation.NewOllamaTranslator(client)
		}
		cancel()
	} else {
		log.Infof("LLM disabled by config; using extractive answers only")
	}

	askService := usecase.NewAskService(
		swappable,
		usecase.NewIntentClassifier(),
		usecase.NewRiskDetector(),
		safetyGen,
		primary,
		translator,
	)

	// Admin UI: wires the JSONL repo + index builder + loader + swap +
	// general knowledge holder (one object satisfies both reader and
	// writer ports).
	repo := repository.NewJSONLRepository(cfg.SeedPath)
	builder := search.NewBuilder()
	adminService := usecase.NewAdminService(repo, builder, loader, swappable, cfg.IndexPath, kb, kb)
	adminHandler, err := apphttp.NewAdminHandler(adminService, log)
	if err != nil {
		log.Errorf("admin handler init: %v", err)
		os.Exit(1)
	}

	router := apphttp.NewRouter(
		apphttp.NewAskHandler(askService),
		apphttp.NewHealthHandler(),
		adminHandler,
		log,
	)

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
		log.Infof("admin UI available at http://localhost%s/admin", addr)
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
