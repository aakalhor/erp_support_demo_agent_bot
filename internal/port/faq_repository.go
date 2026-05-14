package port

import "github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"

// FAQRepository abstracts the source of truth for FAQ records. The
// concrete implementation (JSONL on disk) lives in infrastructure.
type FAQRepository interface {
	// LoadAll returns every FAQ record. The seed dataset is small enough
	// that streaming is unnecessary for the MVP.
	LoadAll() ([]domain.FAQRecord, error)
}
