// Package knowledge holds the admin-curated "general knowledge" blob
// that augments retrieved FAQs in every LLM call.
//
// Concurrency model:
//   - Get is called on every /ask request: RLock.
//   - Set is called once per admin Submit: Lock, write atomically to
//     disk, then update the in-memory copy.
//
// File format is plain text/markdown — no schema, no parsing. Empty
// file or missing file both mean "no general knowledge".
package knowledge

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type FileHolder struct {
	mu   sync.RWMutex
	text string
	path string
}

// NewFileHolder builds a holder backed by the given file path. The
// caller is expected to call Load() once at startup; Set() will create
// or replace the file as needed.
func NewFileHolder(path string) *FileHolder {
	return &FileHolder{path: path}
}

// Load reads the file into memory. Missing file is treated as empty
// (not an error) so a fresh checkout boots without manual scaffolding.
func (h *FileHolder) Load() error {
	data, err := os.ReadFile(h.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			h.mu.Lock()
			h.text = ""
			h.mu.Unlock()
			return nil
		}
		return err
	}
	h.mu.Lock()
	h.text = strings.TrimSpace(string(data))
	h.mu.Unlock()
	return nil
}

// Get returns the current in-memory text.
func (h *FileHolder) Get() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.text
}

// Save writes text to disk atomically (temp file + rename) and, on
// success, updates the in-memory copy. Whitespace-only input is
// stored as the empty string so prompts can do a simple non-empty check.
func (h *FileHolder) Save(text string) error {
	trimmed := strings.TrimSpace(text)

	if err := os.MkdirAll(filepath.Dir(h.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(h.path), filepath.Base(h.path)+".*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.WriteString(trimmed); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if trimmed != "" && !strings.HasSuffix(trimmed, "\n") {
		_, _ = tmp.WriteString("\n")
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	// On Windows os.Rename fails if the destination exists; remove
	// first so the rename can proceed.
	if _, statErr := os.Stat(h.path); statErr == nil {
		if rmErr := os.Remove(h.path); rmErr != nil {
			return rmErr
		}
	}
	if err := os.Rename(tmpPath, h.path); err != nil {
		return err
	}

	h.mu.Lock()
	h.text = trimmed
	h.mu.Unlock()
	return nil
}
