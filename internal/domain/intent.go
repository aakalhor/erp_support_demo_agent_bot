package domain

// Intent is the high-level user-intent label produced by the intent
// classifier. It is a closed enum so downstream code (answer generation,
// escalation policy) can rely on the set of possible values.
type Intent string

const (
	IntentGeneralInfo            Intent = "general_info"
	IntentSupportRequest         Intent = "support_request"
	IntentImplementationQuestion Intent = "implementation_question"
	IntentTrainingQuestion       Intent = "training_question"
	IntentTechnicalIssue         Intent = "technical_issue"
	IntentCriticalIssue          Intent = "critical_issue"
	IntentUnknown                Intent = "unknown"
)

// String returns the wire/JSON representation of the intent.
func (i Intent) String() string { return string(i) }
