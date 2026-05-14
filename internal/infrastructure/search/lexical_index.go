package search

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/port"
)

const indexFormatVersion = 1

// diskIndex is the persisted shape on disk. The runtime LexicalIndex
// mirrors the same fields so search can operate without further
// processing after load.
type diskIndex struct {
	Version    int                `json:"version"`
	TotalDocs  int                `json:"total_docs"`
	Vocabulary map[string]float64 `json:"vocabulary"`
	Docs       []diskDoc          `json:"docs"`
}

type diskDoc struct {
	Record  domain.FAQRecord   `json:"record"`
	Weights map[string]float64 `json:"weights"`
	Norm    float64            `json:"norm"`
}

// LexicalIndex implements port.SearchIndex. It is created either by
// loading a previously persisted file or built in-memory by Builder.
type LexicalIndex struct {
	vocab map[string]float64
	docs  []diskDoc
}

// Search returns the top-k records visible to clientID, ranked by
// cosine similarity. Visibility rule: client-specific queries see their
// own client_id plus "global"; "global" callers see only global rows.
func (idx *LexicalIndex) Search(query string, clientID string, topK int) ([]domain.SearchHit, error) {
	if topK <= 0 {
		topK = 5
	}
	tokens := Tokenize(query)
	qWeights := weightVector(tokens, idx.vocab)
	qNorm := l2Norm(qWeights)

	hits := make([]domain.SearchHit, 0, len(idx.docs))
	for _, d := range idx.docs {
		if !d.Record.VisibleTo(clientID) {
			continue
		}
		score := cosine(qWeights, d.Weights, qNorm, d.Norm)
		if score <= 0 {
			continue
		}
		hits = append(hits, domain.SearchHit{Record: d.Record, Score: score})
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > topK {
		hits = hits[:topK]
	}
	return hits, nil
}

// Loader reads a persisted index from disk.
type Loader struct{}

func NewLoader() *Loader { return &Loader{} }

func (l *Loader) Load(path string) (port.SearchIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", domain.ErrIndexNotFound, path)
		}
		return nil, fmt.Errorf("read index: %w", err)
	}
	var raw diskIndex
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse index %s: %w", path, err)
	}
	if raw.Version != indexFormatVersion {
		return nil, fmt.Errorf("index %s: unsupported version %d (expected %d); rebuild with the indexer", path, raw.Version, indexFormatVersion)
	}
	return &LexicalIndex{vocab: raw.Vocabulary, docs: raw.Docs}, nil
}

// Builder implements port.SearchIndexBuilder. It computes IDF over the
// corpus, builds per-doc TF-IDF weight vectors, and persists the result
// as JSON.
type Builder struct{}

func NewBuilder() *Builder { return &Builder{} }

func (b *Builder) Build(records []domain.FAQRecord, outputPath string) (port.BuildStats, error) {
	if len(records) == 0 {
		return port.BuildStats{}, errors.New("no records to index")
	}

	// 1. Tokenize every record.
	docTokens := make([][]string, len(records))
	for i, r := range records {
		docTokens[i] = TokenizeRecordText(r.Question, r.Answer, r.SourceTitle, r.Product, r.Module, r.Tags)
	}

	// 2. Document frequency per term.
	df := make(map[string]int)
	for _, tokens := range docTokens {
		seen := make(map[string]struct{}, len(tokens))
		for _, t := range tokens {
			if _, ok := seen[t]; ok {
				continue
			}
			seen[t] = struct{}{}
			df[t]++
		}
	}

	// 3. Vocabulary: term -> idf.
	n := len(records)
	vocab := make(map[string]float64, len(df))
	for term, dfCount := range df {
		vocab[term] = idf(n, dfCount)
	}

	// 4. Per-doc weights and norms.
	docs := make([]diskDoc, 0, len(records))
	clientSet := make(map[string]struct{})
	indexed := 0
	for i, r := range records {
		w := weightVector(docTokens[i], vocab)
		if len(w) == 0 {
			// Skip empty docs but keep counting them as loaded.
			continue
		}
		docs = append(docs, diskDoc{
			Record:  r,
			Weights: w,
			Norm:    l2Norm(w),
		})
		clientSet[r.ClientID] = struct{}{}
		indexed++
	}

	// 5. Persist.
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return port.BuildStats{}, fmt.Errorf("create index dir: %w", err)
	}
	out := diskIndex{
		Version:    indexFormatVersion,
		TotalDocs:  indexed,
		Vocabulary: vocab,
		Docs:       docs,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return port.BuildStats{}, fmt.Errorf("marshal index: %w", err)
	}
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return port.BuildStats{}, fmt.Errorf("write index: %w", err)
	}

	return port.BuildStats{
		RecordsLoaded:  len(records),
		RecordsIndexed: indexed,
		UniqueClients:  len(clientSet),
		IndexPath:      outputPath,
	}, nil
}

// BuildInMemoryIndex is a convenience for tests: it returns a runtime
// LexicalIndex without touching disk.
func BuildInMemoryIndex(records []domain.FAQRecord) port.SearchIndex {
	b := NewBuilder()
	// Use a temp path then immediately read back; simpler than a parallel code path.
	tmpDir, err := os.MkdirTemp("", "erp-index-mem-*")
	if err != nil {
		return &LexicalIndex{vocab: map[string]float64{}, docs: nil}
	}
	defer os.RemoveAll(tmpDir)
	p := filepath.Join(tmpDir, "index.json")
	if _, err := b.Build(records, p); err != nil {
		return &LexicalIndex{vocab: map[string]float64{}, docs: nil}
	}
	idx, err := NewLoader().Load(p)
	if err != nil {
		return &LexicalIndex{vocab: map[string]float64{}, docs: nil}
	}
	return idx
}
