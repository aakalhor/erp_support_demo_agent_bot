package usecase

import (
	"fmt"
	"strings"
	"time"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/port"
)

// AdminService is the use case behind the admin UI. It coordinates
// repository writes with a rebuild + hot-swap of the search index so
// the bot picks up changes without a restart, and exposes the general
// knowledge holder so the admin can edit free-form context for the LLM.
type AdminService struct {
	repo      port.FAQRepository
	builder   port.SearchIndexBuilder
	loader    port.SearchIndexLoader
	index     port.SearchIndexSwapper
	indexPath string

	knowledgeReader port.GeneralKnowledgeProvider
	knowledgeWriter port.GeneralKnowledgeWriter
}

func NewAdminService(
	repo port.FAQRepository,
	builder port.SearchIndexBuilder,
	loader port.SearchIndexLoader,
	index port.SearchIndexSwapper,
	indexPath string,
	knowledgeReader port.GeneralKnowledgeProvider,
	knowledgeWriter port.GeneralKnowledgeWriter,
) *AdminService {
	return &AdminService{
		repo:            repo,
		builder:         builder,
		loader:          loader,
		index:           index,
		indexPath:       indexPath,
		knowledgeReader: knowledgeReader,
		knowledgeWriter: knowledgeWriter,
	}
}

// GeneralKnowledge returns the current admin-curated context. Empty
// string means none has been set yet.
func (s *AdminService) GeneralKnowledge() string {
	if s.knowledgeReader == nil {
		return ""
	}
	return s.knowledgeReader.Get()
}

// SaveGeneralKnowledge persists the new text and hot-updates the
// in-memory copy used by the LLM. The change is visible to the next
// /ask request; no reindex is needed because the FAQ corpus did not
// change.
func (s *AdminService) SaveGeneralKnowledge(text string) error {
	if s.knowledgeWriter == nil {
		return fmt.Errorf("general knowledge writer is not configured")
	}
	return s.knowledgeWriter.Save(text)
}

// List returns every FAQ record, in their on-disk order.
func (s *AdminService) List() ([]domain.FAQRecord, error) {
	return s.repo.LoadAll()
}

// Get returns the record with the given ID, or an error if missing.
func (s *AdminService) Get(id string) (domain.FAQRecord, error) {
	records, err := s.repo.LoadAll()
	if err != nil {
		return domain.FAQRecord{}, err
	}
	for _, r := range records {
		if r.ID == id {
			return r, nil
		}
	}
	return domain.FAQRecord{}, fmt.Errorf("faq %q not found", id)
}

// Create inserts a new record. If the supplied ID is empty a fresh one
// of the form "faq_<unix-nano>" is assigned.
func (s *AdminService) Create(rec domain.FAQRecord) (domain.FAQRecord, error) {
	rec = normaliseRecord(rec)
	if strings.TrimSpace(rec.ID) == "" {
		rec.ID = fmt.Sprintf("faq_%d", time.Now().UnixNano())
	}
	if err := validateAdminRecord(rec); err != nil {
		return domain.FAQRecord{}, err
	}
	if err := s.repo.Save(rec); err != nil {
		return domain.FAQRecord{}, err
	}
	if err := s.Reindex(); err != nil {
		return rec, fmt.Errorf("saved record %q but reindex failed: %w", rec.ID, err)
	}
	return rec, nil
}

// Update replaces an existing record by ID.
func (s *AdminService) Update(rec domain.FAQRecord) (domain.FAQRecord, error) {
	rec = normaliseRecord(rec)
	if strings.TrimSpace(rec.ID) == "" {
		return domain.FAQRecord{}, fmt.Errorf("Update: missing id")
	}
	if err := validateAdminRecord(rec); err != nil {
		return domain.FAQRecord{}, err
	}
	if err := s.repo.Save(rec); err != nil {
		return domain.FAQRecord{}, err
	}
	if err := s.Reindex(); err != nil {
		return rec, fmt.Errorf("saved record %q but reindex failed: %w", rec.ID, err)
	}
	return rec, nil
}

// Delete removes a record by ID. Idempotent.
func (s *AdminService) Delete(id string) error {
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	return s.Reindex()
}

// Reindex rebuilds the search index from the current corpus and
// hot-swaps it into the running API. Safe to call any time.
func (s *AdminService) Reindex() error {
	records, err := s.repo.LoadAll()
	if err != nil {
		return err
	}
	if _, err := s.builder.Build(records, s.indexPath); err != nil {
		return err
	}
	next, err := s.loader.Load(s.indexPath)
	if err != nil {
		return err
	}
	s.index.Swap(next)
	return nil
}

// normaliseRecord cleans up text input from the admin form: trim
// whitespace, default empty client_id to "global", normalise risk
// level, and dedupe tags.
func normaliseRecord(rec domain.FAQRecord) domain.FAQRecord {
	rec.ID = strings.TrimSpace(rec.ID)
	rec.ClientID = strings.TrimSpace(rec.ClientID)
	if rec.ClientID == "" {
		rec.ClientID = domain.ClientGlobal
	}
	rec.CompanyType = strings.TrimSpace(rec.CompanyType)
	rec.Product = strings.TrimSpace(rec.Product)
	rec.Module = strings.TrimSpace(rec.Module)
	rec.Question = strings.TrimSpace(rec.Question)
	rec.Answer = strings.TrimSpace(rec.Answer)
	rec.SourceTitle = strings.TrimSpace(rec.SourceTitle)
	rec.SourceType = strings.TrimSpace(rec.SourceType)
	if rec.SourceType == "" {
		rec.SourceType = "admin"
	}
	rec.RiskLevel = rec.RiskLevel.Normalize()

	// Trim + dedupe tags, dropping empties.
	seen := make(map[string]struct{}, len(rec.Tags))
	cleaned := make([]string, 0, len(rec.Tags))
	for _, t := range rec.Tags {
		t = strings.TrimSpace(strings.ToLower(t))
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		cleaned = append(cleaned, t)
	}
	rec.Tags = cleaned
	return rec
}

func validateAdminRecord(rec domain.FAQRecord) error {
	if strings.TrimSpace(rec.Question) == "" {
		return fmt.Errorf("question is required")
	}
	if strings.TrimSpace(rec.Answer) == "" {
		return fmt.Errorf("answer is required")
	}
	return nil
}
