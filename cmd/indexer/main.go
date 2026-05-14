// Command indexer rebuilds the local lexical index from the seed JSONL
// file. Run once after editing data/seed_faq.jsonl, then start the API.
package main

import (
	"os"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/config"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/repository"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/search"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/shared/logger"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/usecase"
)

func main() {
	log := logger.New("indexer")

	cfg, err := config.Load()
	if err != nil {
		log.Errorf("load config: %v", err)
		os.Exit(1)
	}

	repo := repository.NewJSONLRepository(cfg.SeedPath)
	builder := search.NewBuilder()
	svc := usecase.NewIndexService(repo, builder)

	log.Infof("reading seed from %s", cfg.SeedPath)
	stats, err := svc.Rebuild(cfg.IndexPath)
	if err != nil {
		log.Errorf("index build failed: %v", err)
		os.Exit(1)
	}

	log.Infof("records loaded   : %d", stats.RecordsLoaded)
	log.Infof("records indexed  : %d", stats.RecordsIndexed)
	log.Infof("unique client ids: %d", stats.UniqueClients)
	log.Infof("index written to : %s", stats.IndexPath)
}
