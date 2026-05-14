package domain

// ClientGlobal is the reserved client_id for records that are visible to
// every client. Client-specific queries also see global records, but never
// records belonging to a different client.
const ClientGlobal = "global"

// FAQRecord is the canonical knowledge-base entity. It mirrors the JSONL
// seed file but the field names use Go conventions; JSON tags map to the
// storage format.
type FAQRecord struct {
	ID                 string    `json:"id"`
	ClientID           string    `json:"client_id"`
	CompanyType        string    `json:"company_type"`
	Product            string    `json:"product"`
	Module             string    `json:"module"`
	Question           string    `json:"question"`
	Answer             string    `json:"answer"`
	SourceTitle        string    `json:"source_title"`
	SourceType         string    `json:"source_type"`
	RiskLevel          RiskLevel `json:"risk_level"`
	EscalationRequired bool      `json:"escalation_required"`
	Tags               []string  `json:"tags"`
}

// IsGlobal reports whether this record is shared across all clients.
func (r FAQRecord) IsGlobal() bool { return r.ClientID == ClientGlobal }

// VisibleTo reports whether this record is allowed to be returned for a
// query that was issued under the given clientID. The rule is:
//
//   - global records are visible to everyone (including "global" callers);
//   - client-specific records are visible only to that same client;
//   - cross-client leakage is never allowed.
func (r FAQRecord) VisibleTo(clientID string) bool {
	if r.IsGlobal() {
		return true
	}
	return r.ClientID == clientID
}
