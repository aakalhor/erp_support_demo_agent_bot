package port

import "github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"

// FAQRepository abstracts the source of truth for FAQ records.
//
// The MVP implementation rewrites the JSONL file on every write. The
// corpus is small enough (~60 records) that this is fine; if it grew
// beyond ~5k records we would switch to an append-only log or a small
// embedded KV store.
type FAQRepository interface {
	// LoadAll returns every FAQ record in stable on-disk order.
	LoadAll() ([]domain.FAQRecord, error)

	// Save upserts a record by ID. If a record with the same ID exists
	// it is replaced (keeping its on-disk position); otherwise the
	// record is appended to the end of the file.
	Save(rec domain.FAQRecord) error

	// Delete removes the record with the given ID. Deleting a missing
	// ID is not an error (idempotent).
	Delete(id string) error
}
