// Package answers contains the extractive/template AnswerGenerator and
// the LLM-backed AnswerGenerator. The template generator is the safety
// fallback: it only echoes curated FAQ text and never reaches the
// network. The use case decides when to call which one.
package answers

import (
	"strings"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/port"
)

// FallbackAnswer is shown when no hit was retrieved or when the
// AskService flags the result as low-confidence and non-critical.
const FallbackAnswer = "I'm not fully sure based on the current demo knowledge base. I can route this to a support professional."

// TemplateAnswerGenerator implements port.AnswerGenerator using purely
// curated text from the matched FAQ. Never invents content, never
// reaches the network, never errors.
type TemplateAnswerGenerator struct{}

func NewTemplateAnswerGenerator() *TemplateAnswerGenerator { return &TemplateAnswerGenerator{} }

func (g *TemplateAnswerGenerator) Generate(in port.GenerateInput) (string, error) {
	if len(in.Hits) == 0 {
		return FallbackAnswer, nil
	}

	top := in.Hits[0].Record
	base := strings.TrimSpace(top.Answer)

	if in.Intent == domain.IntentCriticalIssue || in.Risk == domain.RiskHigh {
		return base + " This is a high-impact topic; please use the critical support option on the support line so a support professional can engage.", nil
	}

	if in.Risk == domain.RiskMedium {
		return base + " If the situation persists or impacts many users, please open a support ticket so the team can review before any change is made.", nil
	}

	return base, nil
}
