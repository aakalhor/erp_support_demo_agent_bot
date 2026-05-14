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
	"strings"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
)

// JSONLRepository reads FAQ records from a JSON-lines file. One record
// per line. Empty lines and lines starting with '#' are ignored to make
// the seed file easy to maintain by hand.
type JSONLRepository struct {
	path string
}

func NewJSONLRepository(path string) *JSONLRepository {
	return &JSONLRepository{path: path}
}

// LoadAll opens the file and decodes every record. Malformed lines are
// reported with their 1-based line number so the seed author can fix
// them quickly.
func (r *JSONLRepository) LoadAll() ([]domain.FAQRecord, error) {
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
	// FAQ answers may be long; bump the line buffer to 1 MiB.
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
		// Default to global so missing client_id is never a security issue.
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
