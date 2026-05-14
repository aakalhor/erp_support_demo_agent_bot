package usecase

import (
	"strings"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
)

// RiskDetector decides how cautiously the assistant should answer a
// question. It combines two signals:
//
//  1. Lexical rules on the question itself (e.g. "delete transaction"
//     is always high risk regardless of what we retrieved).
//  2. The risk_level of the top retrieved FAQ record (the corpus author
//     has already flagged dangerous topics).
//
// The final risk is the maximum of the two, so a low-risk-looking
// question that matches a high-risk FAQ stays high risk.
type RiskDetector struct{}

func NewRiskDetector() *RiskDetector { return &RiskDetector{} }

var (
	highRiskPhrases = []string{
		"delete transaction",
		"delete a transaction",
		"change inventory quantity",
		"override inventory",
		"modify production configuration",
		"production configuration",
		"post financials",
		"financial posting failed",
		"posting failed",
		"database error",
		"database shows an error",
		"database showed an error",
		"erp is down",
		"erp down",
		"erp system is down",
		"system down",
		"system is down",
		"all users locked out",
		"locked out",
		"cannot process orders",
		"can't process orders",
		"can not process orders",
		"unable to process orders",
		"warehouse blocked",
		"billing cannot run",
		"billing can't run",
		"overwrite pricing",
	}
	mediumRiskPhrases = []string{
		"pricing table",
		"invoice issue",
		"inventory mismatch",
		"inventory looks wrong",
		"order stuck",
		"custom report",
		"user permission",
		"integration issue",
	}
)

// Detect returns the effective RiskLevel for the question and retrieved
// hits.
func (d *RiskDetector) Detect(question string, hits []domain.SearchHit) domain.RiskLevel {
	q := strings.ToLower(question)
	rule := d.detectFromText(q)
	corpus := topHitRisk(hits)
	return maxRisk(rule, corpus)
}

func (d *RiskDetector) detectFromText(lowerQ string) domain.RiskLevel {
	if containsAny(lowerQ, highRiskPhrases) {
		return domain.RiskHigh
	}
	if containsAny(lowerQ, mediumRiskPhrases) {
		return domain.RiskMedium
	}
	return domain.RiskLow
}

func topHitRisk(hits []domain.SearchHit) domain.RiskLevel {
	if len(hits) == 0 {
		return domain.RiskLow
	}
	return hits[0].Record.RiskLevel.Normalize()
}

// riskRank gives a total order so we can compute a max.
func riskRank(r domain.RiskLevel) int {
	switch r {
	case domain.RiskHigh:
		return 2
	case domain.RiskMedium:
		return 1
	default:
		return 0
	}
}

func maxRisk(a, b domain.RiskLevel) domain.RiskLevel {
	if riskRank(a) >= riskRank(b) {
		return a.Normalize()
	}
	return b.Normalize()
}
