package domain

// AskRequest is the inbound request to the Ask use case.
//
// Language is optional; if empty the use case will fall back to
// script-based detection (Gurmukhi → Punjabi, otherwise English).
type AskRequest struct {
	ClientID string   `json:"client_id"`
	Question string   `json:"question"`
	Language Language `json:"language,omitempty"`
}

// MatchedSource is a single retrieved FAQ row, surfaced to the caller for
// transparency and debugging.
type MatchedSource struct {
	ID          string  `json:"id"`
	SourceTitle string  `json:"source_title"`
	Module      string  `json:"module"`
	Score       float64 `json:"score"`
}

// AskResponse is the outbound response of the Ask use case.
//
// Language echoes the detected (or supplied) language so callers can
// route the answer to the right TTS voice.
type AskResponse struct {
	Question           string          `json:"question"`
	Intent             Intent          `json:"intent"`
	Answer             string          `json:"answer"`
	Confidence         float64         `json:"confidence"`
	EscalationRequired bool            `json:"escalation_required"`
	Language           Language        `json:"language"`
	MatchedSources     []MatchedSource `json:"matched_sources"`
}

// SearchHit is the result of a vector/lexical search over the index. It is
// a domain type rather than a port-level type so the use case layer can
// pass it around without depending on infrastructure.
type SearchHit struct {
	Record FAQRecord
	Score  float64
}
