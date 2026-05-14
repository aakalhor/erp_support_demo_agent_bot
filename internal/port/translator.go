package port

import "github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"

// Translator translates between the languages we support. The MVP uses
// Qwen via Ollama for translation, but the port is small enough that
// any service (or a no-op) can be swapped in.
type Translator interface {
	// Translate returns text translated from source to target. If
	// source == target, an implementation is free to return the input
	// unchanged.
	Translate(text string, source, target domain.Language) (string, error)
}
