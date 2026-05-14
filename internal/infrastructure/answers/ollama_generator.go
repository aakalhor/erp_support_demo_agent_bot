package answers

import (
	"context"
	"fmt"
	"strings"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/llm"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/port"
)

// confidenceThresholdForGrounding is the cosine score above which we
// trust the top retrieved FAQ enough to ground the LLM in it strictly.
// Below this threshold we switch the LLM into a free-form, conversational
// prompt that handles small talk and politely-bound questions without
// inventing facts.
const confidenceThresholdForGrounding = 0.55

// OllamaAnswerGenerator implements port.AnswerGenerator by asking a
// local Ollama model (Qwen3 by default) to compose the answer.
//
// Two prompt modes:
//   - Grounded     — used when retrieval is confident. The LLM may
//                    only use the supplied FAQs (and the admin-curated
//                    general knowledge, if any).
//   - Conversational — used when retrieval is weak or empty. The LLM
//                    is free to greet, deflect off-topic asks, or
//                    politely defer, but must still never invent facts
//                    beyond the FAQs / general knowledge.
//
// The use case still gates entry to this generator so the LLM never
// sees critical-issue or high-risk inputs — those always go through
// the curated extractive template.
type OllamaAnswerGenerator struct {
	client    *llm.Client
	knowledge port.GeneralKnowledgeProvider // optional; may be nil
}

// emptyKnowledge is a tiny do-nothing provider used when none is wired.
type emptyKnowledge struct{}

func (emptyKnowledge) Get() string { return "" }

func NewOllamaAnswerGenerator(client *llm.Client, knowledge port.GeneralKnowledgeProvider) *OllamaAnswerGenerator {
	if knowledge == nil {
		knowledge = emptyKnowledge{}
	}
	return &OllamaAnswerGenerator{client: client, knowledge: knowledge}
}

const groundedSystemPromptTemplate = `You are a friendly ERP support demo assistant. Answer the user's question using ONLY the information in the FAQs and any "Admin-curated context" provided below. Follow these rules strictly:

1. Stay grounded. Do not introduce facts, numbers, prices, policies, phone numbers, emails, URLs, or contact details that are not present in the FAQs or in the admin-curated context.
2. If neither the FAQs nor the admin-curated context cover the question, reply naturally that you don't have a specific answer in the current demo knowledge base and that you can route the question to a support professional. Do not guess.
3. Be concise (2-5 sentences). Plain text only. No bullet points unless the user asked for a list. No markdown.
4. Reply in %s. If the question is in a different language, still answer in %s.
5. Never claim to be an official company channel. This is a demo assistant.
6. Do not say "according to the FAQs" or cite IDs in the reply.`

const conversationalSystemPromptTemplate = `You are a warm, friendly ERP support demo assistant for a consulting and support firm focused on Infor distribution ERPs (CloudSuite Distribution and SX.e), helping importers, wholesalers, and distributors. The user's most recent message did not strongly match any specific FAQ in the demo knowledge base.

Decide which of these the user is doing and reply accordingly:

A) Greeting / small talk / thanks / goodbye (e.g. "hi", "hello", "how are you", "thanks", "bye")
   - Greet warmly in one short sentence, then in one more sentence invite them to ask about ERP support topics (implementation, training, support, custom development, hosting, etc.). Match their energy.

B) Specific ERP-related question for which the FAQs below provide no direct answer, BUT the admin-curated context (if present) answers it
   - Use the admin-curated context as authoritative. Answer in 1-3 sentences. Do not introduce facts beyond it.

C) Specific ERP-related question for which neither the FAQs nor the admin-curated context have a direct answer
   - Acknowledge the question. Say you don't have a specific answer in the current demo knowledge base, and offer to route the question to a support professional if it's important.

D) Question clearly unrelated to ERP support (weather, jokes, math, news, etc.)
   - Politely note that you are focused on ERP support questions and offer to help with that.

Rules:
1. Reply in %s.
2. Be concise: 1-3 short sentences. Plain prose only. No bullet points, no markdown, no emoji.
3. NEVER invent specific facts, numbers, prices, policies, phone numbers, emails, URLs, names, or product details that are not in the FAQs or admin-curated context below.
4. Do not pretend to be an official company channel; you are a demo assistant.
5. Do not say "FAQs" or "knowledge base" word-for-word; just speak naturally.
6. Do not start with "Sure" or "Of course". Just respond.`

func (g *OllamaAnswerGenerator) Generate(in port.GenerateInput) (string, error) {
	target := in.Language
	if !target.IsSupported() {
		target = domain.LanguageEnglish
	}

	knowledge := strings.TrimSpace(g.knowledge.Get())
	mode := pickMode(in.Hits)
	var sys, user string
	if mode == modeGrounded {
		sys = fmt.Sprintf(groundedSystemPromptTemplate, target.HumanName(), target.HumanName())
		user = buildGroundedUserPrompt(in, knowledge)
	} else {
		sys = fmt.Sprintf(conversationalSystemPromptTemplate, target.HumanName())
		user = buildConversationalUserPrompt(in, knowledge)
	}

	out, err := g.client.Chat(
		context.Background(),
		[]llm.Message{
			{Role: "system", Content: sys},
			{Role: "user", Content: user},
		},
		map[string]any{
			"temperature":    0.4,
			"num_predict":    700,
			"repeat_penalty": 1.15,
		},
	)
	if err != nil {
		return "", err
	}
	out = stripThinkBlocks(out)
	out = strings.TrimSpace(out)
	if out == "" {
		return FallbackAnswer, nil
	}
	return out, nil
}

type promptMode int

const (
	modeGrounded promptMode = iota
	modeConversational
)

func pickMode(hits []domain.SearchHit) promptMode {
	if len(hits) == 0 {
		return modeConversational
	}
	if hits[0].Score < confidenceThresholdForGrounding {
		return modeConversational
	}
	return modeGrounded
}

func buildGroundedUserPrompt(in port.GenerateInput, knowledge string) string {
	var b strings.Builder
	b.WriteString("User question: ")
	b.WriteString(strings.TrimSpace(in.Question))
	if knowledge != "" {
		b.WriteString("\n\nAdmin-curated context (treat as authoritative alongside the FAQs):\n")
		b.WriteString(knowledge)
	}
	b.WriteString("\n\nRelevant FAQs (most relevant first):\n")
	for i, h := range in.Hits {
		if i >= 5 {
			break
		}
		r := h.Record
		b.WriteString("---\n")
		b.WriteString("Q: ")
		b.WriteString(strings.TrimSpace(r.Question))
		b.WriteString("\nA: ")
		b.WriteString(strings.TrimSpace(r.Answer))
		b.WriteString("\n")
	}
	b.WriteString("---\n\nWrite the answer now.")
	return b.String()
}

func buildConversationalUserPrompt(in port.GenerateInput, knowledge string) string {
	var b strings.Builder
	b.WriteString("User said: ")
	b.WriteString(strings.TrimSpace(in.Question))
	if knowledge != "" {
		b.WriteString("\n\nAdmin-curated context (use this when it answers the question):\n")
		b.WriteString(knowledge)
	}
	if len(in.Hits) > 0 {
		b.WriteString("\n\nLoosely related material from the FAQs (may or may not be useful):\n")
		for i, h := range in.Hits {
			if i >= 3 {
				break
			}
			r := h.Record
			b.WriteString("- ")
			b.WriteString(strings.TrimSpace(r.Question))
			b.WriteString(" — ")
			b.WriteString(strings.TrimSpace(r.Answer))
			b.WriteString("\n")
		}
	}
	b.WriteString("\nReply now.")
	return b.String()
}

// stripThinkBlocks removes any <think>...</think> segments Qwen3
// sometimes emits even with think=false.
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
