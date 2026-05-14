// Package translation contains the Translator implementations. The MVP
// uses the same Ollama model as for answer generation, with a tight
// system prompt that returns only the translated text.
package translation

import (
	"context"
	"strings"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/llm"
)

type OllamaTranslator struct {
	client *llm.Client
}

func NewOllamaTranslator(client *llm.Client) *OllamaTranslator {
	return &OllamaTranslator{client: client}
}

// Translate returns text rendered in target. Pass-through if source ==
// target or if either side is unsupported (we never want translation
// to introduce hallucinations on edge cases).
func (t *OllamaTranslator) Translate(text string, source, target domain.Language) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}
	if source == target {
		return text, nil
	}
	if !target.IsSupported() {
		return text, nil
	}

	srcLabel := source.HumanName()
	if source == domain.LangAuto || !source.IsSupported() {
		srcLabel = "the source language (auto-detect)"
	}

	sys := "You are a precise translator. Translate the user's text from " + srcLabel +
		" to " + target.HumanName() + ". Reply with ONLY the translation, no quotes, " +
		"no preamble, no commentary, no explanation. Translate each sentence exactly " +
		"once — do not repeat phrases. Preserve numbers, product names, and ERP " +
		"terminology unchanged (for example: Infor CloudSuite Distribution, SX.e)."

	out, err := t.client.Chat(
		context.Background(),
		[]llm.Message{
			{Role: "system", Content: sys},
			{Role: "user", Content: text},
		},
		map[string]any{
			"temperature":    0.0,
			"num_predict":    800,
			// Strong repetition penalty: at temperature 0 some models
			// (Qwen3 included) can loop when translating into a lower-
			// resource target language. 1.3 keeps output decisive
			// without distorting normal phrasing.
			"repeat_penalty": 1.3,
			"repeat_last_n":  64,
		},
	)
	if err != nil {
		return "", err
	}
	out = stripThinkBlocks(out)
	out = strings.TrimSpace(out)
	if out == "" {
		// Fall back to the original text rather than returning empty.
		return text, nil
	}
	return out, nil
}

func stripThinkBlocks(s string) string {
	for {
		start := strings.Index(s, "<think>")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], "</think>")
		if end < 0 {
			s = s[:start]
			break
		}
		s = s[:start] + s[start+end+len("</think>"):]
	}
	return s
}
