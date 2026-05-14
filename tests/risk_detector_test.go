package tests

import (
	"testing"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/usecase"
)

func TestRiskDetector_CriticalPhrasesAreHigh(t *testing.T) {
	d := usecase.NewRiskDetector()
	cases := []string{
		"Our ERP system is down",
		"erp is down",
		"All users are locked out",
		"Warehouse blocked since this morning",
		"Cannot process orders in our main warehouse",
		"A financial posting failed during the close",
		"The database shows an error when we try to post",
		"Billing cannot run on tonight's batch",
	}
	for _, q := range cases {
		got := d.Detect(q, nil)
		if got != domain.RiskHigh {
			t.Errorf("%q: expected high, got %s", q, got)
		}
	}
}

func TestRiskDetector_HighRiskActionsAreHigh(t *testing.T) {
	d := usecase.NewRiskDetector()
	cases := []string{
		"Can I delete a transaction?",
		"How do I change inventory quantity directly?",
		"Can we modify production configuration?",
		"Should we post financials manually?",
		"How can I overwrite pricing for this customer?",
	}
	for _, q := range cases {
		got := d.Detect(q, nil)
		if got != domain.RiskHigh {
			t.Errorf("%q: expected high, got %s", q, got)
		}
	}
}

func TestRiskDetector_MediumPhrases(t *testing.T) {
	d := usecase.NewRiskDetector()
	cases := []string{
		"We have an invoice issue on a single order",
		"There is an inventory mismatch on a few SKUs",
		"This order stuck before billing",
		"User permission not working as expected",
		"Integration issue with our shipping system",
		"Can you help with a custom report?",
	}
	for _, q := range cases {
		got := d.Detect(q, nil)
		if got != domain.RiskMedium {
			t.Errorf("%q: expected medium, got %s", q, got)
		}
	}
}

func TestRiskDetector_LowByDefault(t *testing.T) {
	d := usecase.NewRiskDetector()
	cases := []string{
		"What does your company do?",
		"Who do you help?",
		"What happens during the Plan phase?",
		"Can you train our finance team?",
	}
	for _, q := range cases {
		got := d.Detect(q, nil)
		if got != domain.RiskLow {
			t.Errorf("%q: expected low, got %s", q, got)
		}
	}
}

func TestRiskDetector_TopHitRiskRaisesFloor(t *testing.T) {
	d := usecase.NewRiskDetector()
	// A benign-looking question but the top hit is flagged high.
	hits := []domain.SearchHit{{
		Record: domain.FAQRecord{ID: "x", RiskLevel: domain.RiskHigh},
		Score:  0.9,
	}}
	got := d.Detect("just curious about something", hits)
	if got != domain.RiskHigh {
		t.Errorf("expected high because top hit is high, got %s", got)
	}
}

func TestIntentClassifier_CriticalBeatsTechnical(t *testing.T) {
	c := usecase.NewIntentClassifier()
	got := c.Classify("Our warehouse users cannot process orders", nil)
	if got != domain.IntentCriticalIssue {
		t.Errorf("expected critical_issue, got %s", got)
	}
}

func TestIntentClassifier_TrainingMatch(t *testing.T) {
	c := usecase.NewIntentClassifier()
	got := c.Classify("Can you train our warehouse users?", nil)
	if got != domain.IntentTrainingQuestion {
		t.Errorf("expected training_question, got %s", got)
	}
}

func TestIntentClassifier_ImplementationMatch(t *testing.T) {
	c := usecase.NewIntentClassifier()
	got := c.Classify("What happens during the Plan phase?", nil)
	if got != domain.IntentImplementationQuestion {
		t.Errorf("expected implementation_question, got %s", got)
	}
}
