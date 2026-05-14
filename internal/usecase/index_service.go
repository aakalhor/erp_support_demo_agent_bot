package usecase

import (
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/port"
)

// IndexService is the indexer-side use case. It loads the seed corpus
// via FAQRepository and asks the SearchIndexBuilder to persist a fresh
// lexical index to disk.
type IndexService struct {
	repo    port.FAQRepository
	builder port.SearchIndexBuilder
}

func NewIndexService(repo port.FAQRepository, builder port.SearchIndexBuilder) *IndexService {
	return &IndexService{repo: repo, builder: builder}
}

// Rebuild reads every FAQ record and writes a new index file to
// outputPath, returning a small stats struct so the CLI can print a
// human-readable summary.
func (s *IndexService) Rebuild(outputPath string) (port.BuildStats, error) {
	records, err := s.repo.LoadAll()
	if err != nil {
		return port.BuildStats{}, err
	}
	return s.builder.Build(records, outputPath)
}
