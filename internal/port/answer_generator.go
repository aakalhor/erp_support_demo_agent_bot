package port

import "github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"

// GenerateInput carries everything the AnswerGenerator needs. Bundling
// it into one struct keeps future fields (style, max length, etc.) from
// changing the port signature.
type GenerateInput struct {
	Question string           // original question, in the user's language
	Intent   domain.Intent
	Risk     domain.RiskLevel
	Hits     []domain.SearchHit
	Language domain.Language  // language to answer in
}

// AnswerGenerator turns a question + retrieved hits + risk/intent into
// the final text shown to the user. Implementations include the
// extractive template generator (safe, deterministic) and the Ollama
// LLM generator (richer, used for low/medium-risk paths).
type AnswerGenerator interface {
	Generate(in GenerateInput) (string, error)
}
