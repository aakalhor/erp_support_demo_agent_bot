// Package repository contains FAQRepository implementations. The MVP
// stores the corpus in a flat JSONL file on disk.
package repository

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
)

// JSONLRepository reads/writes FAQ records as JSON-lines on disk.
//
// One record per line. Empty lines and lines starting with '#' are
// skipped on read, so the seed file remains easy to edit by hand.
// Writes go through Save/Delete which rewrite the file atomically via
// a tempfile + rename, protected by a per-repository mutex.
type JSONLRepository struct {
	path string
	mu   sync.Mutex
}

func NewJSONLRepository(path string) *JSONLRepository {
	return &JSONLRepository{path: path}
}

// LoadAll opens the file and decodes every record.
func (r *JSONLRepository) LoadAll() ([]domain.FAQRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadLocked()
}

func (r *JSONLRepository) loadLocked() ([]domain.FAQRecord, error) {
	f, err := os.Open(r.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", domain.ErrSeedNotFound, r.path)
		}
		return nil, err
	}
	defer f.Close()

	var out []domain.FAQRecord
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var rec domain.FAQRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("seed file %s line %d: %w", r.path, lineNum, err)
		}
		if err := validate(rec, lineNum); err != nil {
			return nil, err
		}
		if strings.TrimSpace(rec.ClientID) == "" {
			rec.ClientID = domain.ClientGlobal
		}
		rec.RiskLevel = rec.RiskLevel.Normalize()
		out = append(out, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read seed file: %w", err)
	}
	return out, nil
}

// Save upserts a record by ID. If the file doesn't exist yet it is
// created.
func (r *JSONLRepository) Save(rec domain.FAQRecord) error {
	if strings.TrimSpace(rec.ID) == "" {
		return fmt.Errorf("Save: missing id")
	}
	if strings.TrimSpace(rec.ClientID) == "" {
		rec.ClientID = domain.ClientGlobal
	}
	rec.RiskLevel = rec.RiskLevel.Normalize()

	r.mu.Lock()
	defer r.mu.Unlock()

	records, err := r.loadOrEmptyLocked()
	if err != nil {
		return err
	}
	replaced := false
	for i := range records {
		if records[i].ID == rec.ID {
			records[i] = rec
			replaced = true
			break
		}
	}
	if !replaced {
		records = append(records, rec)
	}
	return r.writeAllLocked(records)
}

// Delete removes the record with the given ID. Idempotent.
func (r *JSONLRepository) Delete(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("Delete: missing id")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	records, err := r.loadOrEmptyLocked()
	if err != nil {
		return err
	}
	out := make([]domain.FAQRecord, 0, len(records))
	for _, rec := range records {
		if rec.ID == id {
			continue
		}
		out = append(out, rec)
	}
	return r.writeAllLocked(out)
}

// loadOrEmptyLocked is like loadLocked but treats a missing file as an
// empty corpus. Used by Save/Delete so an admin can bootstrap from an
// empty knowledge base.
func (r *JSONLRepository) loadOrEmptyLocked() ([]domain.FAQRecord, error) {
	recs, err := r.loadLocked()
	if err != nil && errors.Is(err, domain.ErrSeedNotFound) {
		return nil, nil
	}
	return recs, err
}

// writeAllLocked rewrites the entire file atomically via tempfile +
// rename. The mutex MUST be held by the caller.
func (r *JSONLRepository) writeAllLocked(records []domain.FAQRecord) error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(r.path), filepath.Base(r.path)+".*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	w := bufio.NewWriter(tmp)
	for _, rec := range records {
		data, mErr := json.Marshal(rec)
		if mErr != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
			return fmt.Errorf("marshal %s: %w", rec.ID, mErr)
		}
		if _, wErr := w.Write(data); wErr != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
			return wErr
		}
		if _, wErr := w.WriteString("\n"); wErr != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
			return wErr
		}
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	// On Windows os.Rename will fail if the destination exists. Remove
	// it first; the temp file still has the new content so a crash
	// between Remove and Rename leaves the temp on disk (recoverable).
	if _, statErr := os.Stat(r.path); statErr == nil {
		if rmErr := os.Remove(r.path); rmErr != nil {
			return fmt.Errorf("remove old file: %w", rmErr)
		}
	}
	if err := os.Rename(tmpPath, r.path); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, r.path, err)
	}
	return nil
}

func validate(rec domain.FAQRecord, lineNum int) error {
	if strings.TrimSpace(rec.ID) == "" {
		return fmt.Errorf("seed line %d: missing id", lineNum)
	}
	if strings.TrimSpace(rec.Question) == "" {
		return fmt.Errorf("seed line %d (%s): missing question", lineNum, rec.ID)
	}
	if strings.TrimSpace(rec.Answer) == "" {
		return fmt.Errorf("seed line %d (%s): missing answer", lineNum, rec.ID)
	}
	return nil
}
