package search

import (
	"sync"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/port"
)

// SwappableIndex is a thread-safe wrapper around a port.SearchIndex
// that allows the admin to hot-swap the underlying index after a
// reindex without restarting the API.
//
// All readers (AskService.Ask) take an RLock while searching; the
// admin's reindex flow takes a full Lock for the brief moment when
// the new index replaces the old one.
type SwappableIndex struct {
	mu  sync.RWMutex
	idx port.SearchIndex
}

func NewSwappableIndex(initial port.SearchIndex) *SwappableIndex {
	return &SwappableIndex{idx: initial}
}

// Search delegates to the currently-loaded index. Safe to call from
// any goroutine.
func (s *SwappableIndex) Search(query string, clientID string, topK int) ([]domain.SearchHit, error) {
	s.mu.RLock()
	cur := s.idx
	s.mu.RUnlock()
	if cur == nil {
		return nil, domain.ErrIndexNotFound
	}
	return cur.Search(query, clientID, topK)
}

// Swap replaces the underlying index. The previous index is discarded;
// in-flight searches that already captured it complete normally.
func (s *SwappableIndex) Swap(next port.SearchIndex) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idx = next
}
