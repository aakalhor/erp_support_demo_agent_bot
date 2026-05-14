package domain

// RiskLevel categorises how cautious the assistant must be when answering
// a question. Values are intentionally a small closed enum so the rest of
// the codebase can switch on them exhaustively.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// IsValid reports whether r is one of the known risk levels.
func (r RiskLevel) IsValid() bool {
	switch r {
	case RiskLow, RiskMedium, RiskHigh:
		return true
	}
	return false
}

// Normalize returns r if valid, otherwise RiskLow as a safe default.
func (r RiskLevel) Normalize() RiskLevel {
	if r.IsValid() {
		return r
	}
	return RiskLow
}
