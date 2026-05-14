package port

// GeneralKnowledgeProvider supplies admin-curated free-form text that
// the LLM should treat as authoritative alongside the matched FAQs.
//
// The provider is consulted on every /ask request; implementations
// should be cheap (typically an in-memory string under an RWMutex).
type GeneralKnowledgeProvider interface {
	// Get returns the current general knowledge text, or "" if none
	// has been set. Implementations must be safe for concurrent reads.
	Get() string
}

// GeneralKnowledgeWriter is the admin-side counterpart. Implementations
// persist the text to disk AND update the in-memory copy returned by
// Get(), so changes are visible to the bot immediately after Save.
type GeneralKnowledgeWriter interface {
	// Save persists the new text and updates the in-memory copy
	// atomically. Empty string clears the knowledge.
	Save(text string) error
}
