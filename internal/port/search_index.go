package port

import "github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"

// SearchIndex is the read side of the local lexical index. It performs a
// client-scoped retrieval and returns the top-k records together with a
// normalised 0..1 score.
type SearchIndex interface {
	// Search runs a query against the index. clientID controls visibility:
	// "global" sees only global records; any other value sees that client's
	// records plus global records. topK limits the number of hits.
	Search(query string, clientID string, topK int) ([]domain.SearchHit, error)
}

// SearchIndexBuilder is the write side: given a corpus, persist an index
// that SearchIndex implementations can load later.
type SearchIndexBuilder interface {
	Build(records []domain.FAQRecord, outputPath string) (BuildStats, error)
}

// BuildStats is a tiny value object the indexer CLI uses to print a
// human-readable summary at the end of an index build.
type BuildStats struct {
	RecordsLoaded  int
	RecordsIndexed int
	UniqueClients  int
	IndexPath      string
}

// SearchIndexLoader reads a previously persisted index from disk.
type SearchIndexLoader interface {
	Load(path string) (SearchIndex, error)
}

// SearchIndexSwapper is implemented by indexes that support being
// hot-replaced at runtime (e.g. after the admin edits the corpus and
// triggers a reindex). The use case layer depends only on this port,
// not on the concrete swappable implementation in infrastructure.
type SearchIndexSwapper interface {
	SearchIndex
	Swap(next SearchIndex)
}
