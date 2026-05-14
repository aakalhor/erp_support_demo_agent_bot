package usecase

import (
	"strings"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
)

// IntentClassifier turns a free-text question into a coarse intent label.
// The MVP uses keyword/phrase rules; the order of checks matters because
// critical-issue phrases must take precedence over more generic ones (for
// example "warehouse users cannot process orders" mentions "warehouse"
// but is a critical issue, not a routine training/technical question).
type IntentClassifier struct{}

func NewIntentClassifier() *IntentClassifier { return &IntentClassifier{} }

// Phrase tables. Lowercase substrings; the matcher lowercases the input
// before comparing.
var (
	criticalPhrases = []string{
		"system down",
		"system is down",
		"erp down",
		"erp is down",
		"erp system is down",
		"all users",
		"cannot process orders",
		"can't process orders",
		"can not process orders",
		"unable to process orders",
		"warehouse blocked",
		"billing cannot run",
		"billing can't run",
		"financial posting failed",
		"posting failed",
		"database error",
		"database shows an error",
		"database showed an error",
		"locked out",
	}
	trainingPhrases = []string{
		"training",
		"train ",
		"learn",
		"teach",
		"warehouse users", // training context only — critical check runs first
	}
	implementationPhrases = []string{
		"implementation",
		"go-live",
		"go live",
		"plan phase",
		"design phase",
		"build phase",
		"test phase",
		"deliver phase",
	}
	supportPhrases = []string{
		"support",
		"ticket",
		"login",
		"issue",
	}
	technicalPhrases = []string{
		"order",
		"inventory",
		"billing",
		"invoice",
		"pricing",
		"warehouse",
	}
)

// Classify applies the rule cascade in priority order and returns the
// matched Intent. Hits from the retriever are not strictly needed for
// the rules but are accepted so the signature stays useful for future
// metadata-aware refinements.
func (c *IntentClassifier) Classify(question string, _ []domain.SearchHit) domain.Intent {
	q := strings.ToLower(question)

	if containsAny(q, criticalPhrases) {
		return domain.IntentCriticalIssue
	}
	if containsAny(q, trainingPhrases) {
		return domain.IntentTrainingQuestion
	}
	if containsAny(q, implementationPhrases) {
		return domain.IntentImplementationQuestion
	}
	if containsAny(q, supportPhrases) {
		return domain.IntentSupportRequest
	}
	if containsAny(q, technicalPhrases) {
		return domain.IntentTechnicalIssue
	}
	if strings.TrimSpace(q) == "" {
		return domain.IntentUnknown
	}
	return domain.IntentGeneralInfo
}

func containsAny(haystack string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}
