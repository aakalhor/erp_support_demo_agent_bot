package usecase

import (
	"strings"
	"unicode"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/port"
)

// ConfidenceThreshold is the cosine score below which the AskService
// refuses to answer directly (except for critical/high-risk topics) and
// instead routes to a human.
const ConfidenceThreshold = 0.55

// TopK is the retrieval size.
const TopK = 5

// AskService composes search + classifiers + translator + answer
// generators into a single deterministic flow.
//
// Two AnswerGenerators are injected:
//
//   - safety: the curated/extractive generator. Used when intent is
//     critical_issue, when risk is high, or when confidence is low.
//     Never reaches the network. Cannot hallucinate.
//   - primary: the LLM generator. Used for the remaining (safe) path
//     so the assistant can phrase the answer naturally in the user's
//     language. The safety prompt grounds it in the retrieved FAQs.
//
// The translator is optional. If nil, the assistant will only echo
// English text (matching the corpus). When a translator is wired, the
// service uses it to (a) convert non-English questions to English for
// retrieval and (b) translate template-driven answers into the user's
// language.
type AskService struct {
	index      port.SearchIndex
	intent     *IntentClassifier
	risk       *RiskDetector
	safety     port.AnswerGenerator
	primary    port.AnswerGenerator
	translator port.Translator
}

func NewAskService(
	index port.SearchIndex,
	intent *IntentClassifier,
	risk *RiskDetector,
	safety port.AnswerGenerator,
	primary port.AnswerGenerator,
	translator port.Translator,
) *AskService {
	if primary == nil {
		primary = safety
	}
	return &AskService{
		index:      index,
		intent:     intent,
		risk:       risk,
		safety:     safety,
		primary:    primary,
		translator: translator,
	}
}

// Ask runs the full RAG flow.
func (s *AskService) Ask(req domain.AskRequest) (domain.AskResponse, error) {
	question := strings.TrimSpace(req.Question)
	if question == "" {
		return domain.AskResponse{}, domain.ErrInvalidRequest
	}

	clientID := req.ClientID
	if strings.TrimSpace(clientID) == "" {
		clientID = domain.ClientGlobal
	}

	// Resolve language: trust the caller's value, else detect from
	// script (Gurmukhi → Punjabi, otherwise English).
	lang := domain.NormalizeLanguage(string(req.Language))
	if !lang.IsSupported() {
		lang = detectLanguage(question)
	}

	// Retrieval is always done in English because the corpus is English.
	englishQuestion := question
	if lang != domain.LanguageEnglish && s.translator != nil {
		if translated, err := s.translator.Translate(question, lang, domain.LanguageEnglish); err == nil && strings.TrimSpace(translated) != "" {
			englishQuestion = translated
		}
	}

	hits, err := s.index.Search(englishQuestion, clientID, TopK)
	if err != nil {
		return domain.AskResponse{}, err
	}

	// Intent and risk are evaluated on the English form because the
	// rule phrases are English. The original-language question is
	// preserved for the LLM so the answer is phrased naturally.
	intent := s.intent.Classify(englishQuestion, hits)
	risk := s.risk.Detect(englishQuestion, hits)

	confidence := 0.0
	if len(hits) > 0 {
		confidence = hits[0].Score
	}
	escalate := decideEscalation(intent, risk, confidence, hits)

	// Route to safety or primary generator.
	answer, err := s.generateAnswer(question, englishQuestion, intent, risk, confidence, hits, lang)
	if err != nil {
		return domain.AskResponse{}, err
	}

	return domain.AskResponse{
		Question:           question,
		Intent:             intent,
		Answer:             answer,
		Confidence:         roundTo3(confidence),
		EscalationRequired: escalate,
		Language:           lang,
		MatchedSources:     toMatchedSources(hits),
	}, nil
}

// generateAnswer encapsulates the routing policy between the safety
// generator (extractive, deterministic) and the primary generator (LLM).
//
// Policy:
//   - critical_issue → safety template (no LLM, ever).
//   - high risk      → safety template (no LLM, ever).
//   - otherwise      → LLM. The LLM internally picks a "grounded" prompt
//                      when retrieval is confident and a "conversational"
//                      prompt when it isn't (handles greetings, off-topic
//                      questions, and graceful "I don't know" without
//                      ever inventing facts).
//   - LLM unavailable / fails → fall through to the safety template so
//                      we never return an error string to the user.
func (s *AskService) generateAnswer(
	originalQuestion string,
	englishQuestion string,
	intent domain.Intent,
	risk domain.RiskLevel,
	_ float64,
	hits []domain.SearchHit,
	target domain.Language,
) (string, error) {
	useLLM := intent != domain.IntentCriticalIssue && risk != domain.RiskHigh

	in := port.GenerateInput{
		Question: pickPromptQuestion(originalQuestion, englishQuestion, target),
		Intent:   intent,
		Risk:     risk,
		Hits:     hits,
		Language: target,
	}

	if useLLM && s.primary != nil {
		out, err := s.primary.Generate(in)
		if err == nil && strings.TrimSpace(out) != "" {
			return out, nil
		}
		// LLM failed (Ollama down, timeout) — fall through to the safe
		// extractive generator rather than returning an error to the user.
	}

	// Safety / fallback path. Returns curated English text.
	out, err := s.safety.Generate(in)
	if err != nil {
		return "", err
	}
	if target != domain.LanguageEnglish && s.translator != nil {
		if translated, terr := s.translator.Translate(out, domain.LanguageEnglish, target); terr == nil && strings.TrimSpace(translated) != "" {
			return translated, nil
		}
	}
	return out, nil
}

// pickPromptQuestion sends the LLM the question in the target answer
// language so it stays consistent: if the user asked in Punjabi we
// give the LLM the Punjabi question; the FAQs (English) ground the
// content; the system prompt instructs it to answer in Punjabi.
func pickPromptQuestion(original, englishForm string, target domain.Language) string {
	if target == domain.LanguageEnglish {
		return englishForm
	}
	return original
}

// detectLanguage uses the script of the input as a simple, dependency-
// free heuristic: any Gurmukhi codepoint → Punjabi; otherwise English.
// Good enough for the closed-enum we support today.
func detectLanguage(s string) domain.Language {
	for _, r := range s {
		// Gurmukhi block: U+0A00..U+0A7F
		if r >= 0x0A00 && r <= 0x0A7F {
			return domain.LanguagePunjabi
		}
		_ = unicode.IsLetter // (kept to make intent obvious)
	}
	return domain.LanguageEnglish
}

// decideEscalation fires for real-risk situations only. We deliberately
// no longer escalate purely on low retrieval confidence: the LLM's
// conversational mode handles greetings, off-topic chatter, and "I
// don't know" gracefully without flagging every casual message to a
// human.
func decideEscalation(intent domain.Intent, risk domain.RiskLevel, _ float64, hits []domain.SearchHit) bool {
	if intent == domain.IntentCriticalIssue {
		return true
	}
	if risk == domain.RiskHigh {
		return true
	}
	if len(hits) > 0 && hits[0].Record.EscalationRequired {
		return true
	}
	return false
}

func toMatchedSources(hits []domain.SearchHit) []domain.MatchedSource {
	out := make([]domain.MatchedSource, 0, len(hits))
	for _, h := range hits {
		out = append(out, domain.MatchedSource{
			ID:          h.Record.ID,
			SourceTitle: h.Record.SourceTitle,
			Module:      h.Record.Module,
			Score:       roundTo3(h.Score),
		})
	}
	return out
}

func roundTo3(v float64) float64 {
	const f = 1000.0
	return float64(int64(v*f+0.5)) / f
}
