package tests

import (
	"strings"
	"testing"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/answers"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/search"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/usecase"
)

func newTestService() *usecase.AskService {
	idx := search.BuildInMemoryIndex(fixtureCorpus())
	safety := answers.NewTemplateAnswerGenerator()
	return usecase.NewAskService(
		idx,
		usecase.NewIntentClassifier(),
		usecase.NewRiskDetector(),
		safety,
		safety, // no LLM in unit tests; reuse the safety generator
		nil,    // no translator in unit tests
	)
}

func TestAskService_CSDQuestion(t *testing.T) {
	svc := newTestService()
	resp, err := svc.Ask(domain.AskRequest{
		ClientID: domain.ClientGlobal,
		Question: "Do you support Infor CloudSuite Distribution?",
	})
	if err != nil {
		t.Fatalf("ask error: %v", err)
	}
	if len(resp.MatchedSources) == 0 || resp.MatchedSources[0].ID != "g_csd" {
		t.Fatalf("expected top hit g_csd; got %#v", resp.MatchedSources)
	}
	if resp.EscalationRequired {
		t.Errorf("low-risk CSD question must not escalate, got escalation=true (confidence=%v)", resp.Confidence)
	}
}

func TestAskService_CriticalIssue_EscalatesAndAvoidsOperationalFix(t *testing.T) {
	svc := newTestService()
	resp, err := svc.Ask(domain.AskRequest{
		ClientID: domain.ClientGlobal,
		Question: "Our ERP system is down and nobody can process orders",
	})
	if err != nil {
		t.Fatalf("ask error: %v", err)
	}
	if resp.Intent != domain.IntentCriticalIssue {
		t.Errorf("expected intent=critical_issue, got %s", resp.Intent)
	}
	if !resp.EscalationRequired {
		t.Errorf("critical issue must escalate")
	}
	// Sanity check that the answer text reminds the user to escalate
	// rather than offering a step-by-step fix.
	low := strings.ToLower(resp.Answer)
	if !strings.Contains(low, "critical support") && !strings.Contains(low, "support professional") {
		t.Errorf("critical-issue answer should mention support; got %q", resp.Answer)
	}
}

func TestAskService_DeleteTransaction_IsHighRiskAndEscalates(t *testing.T) {
	svc := newTestService()
	resp, err := svc.Ask(domain.AskRequest{
		ClientID: domain.ClientGlobal,
		Question: "Can I delete a transaction?",
	})
	if err != nil {
		t.Fatalf("ask error: %v", err)
	}
	if !resp.EscalationRequired {
		t.Errorf("delete-transaction must escalate")
	}
	if len(resp.MatchedSources) == 0 || resp.MatchedSources[0].ID != "g_delete_tx" {
		t.Fatalf("expected delete_tx top hit, got %#v", resp.MatchedSources)
	}
}

func TestAskService_ClientIsolation_AlphaDoesNotSeeBeta(t *testing.T) {
	svc := newTestService()
	resp, err := svc.Ask(domain.AskRequest{
		ClientID: "client_alpha",
		Question: "What is our private SKU pattern?",
	})
	if err != nil {
		t.Fatalf("ask error: %v", err)
	}
	for _, m := range resp.MatchedSources {
		if m.ID == "beta_sku" {
			t.Fatalf("client_alpha must not see beta-only record")
		}
	}
}

func TestAskService_LowConfidence_DoesNotEscalateButFallsBackToSafeText(t *testing.T) {
	// New policy: low retrieval confidence on a non-critical, non-high-risk
	// topic no longer auto-escalates. The LLM's conversational mode is
	// expected to handle greetings / off-topic chatter gracefully. In
	// these unit tests we inject the safety generator as the "primary"
	// generator (no real LLM), so the curated fallback text is still
	// what we get — but escalation should be off.
	svc := newTestService()
	resp, err := svc.Ask(domain.AskRequest{
		ClientID: domain.ClientGlobal,
		Question: "Tell me a poem about the sea",
	})
	if err != nil {
		t.Fatalf("ask error: %v", err)
	}
	if resp.EscalationRequired {
		t.Errorf("low-confidence non-critical query should not auto-escalate; got escalate=true")
	}
	if !strings.Contains(strings.ToLower(resp.Answer), "not fully sure") {
		t.Errorf("with the safety generator stubbed as primary, low-confidence answer should still be the fallback; got %q", resp.Answer)
	}
}

func TestAskService_EmptyQuestion_RejectsRequest(t *testing.T) {
	svc := newTestService()
	_, err := svc.Ask(domain.AskRequest{ClientID: domain.ClientGlobal, Question: "   "})
	if err == nil {
		t.Fatal("expected error for empty question")
	}
}
