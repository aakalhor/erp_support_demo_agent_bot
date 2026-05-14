// Package search contains the local lexical retrieval engine. The MVP
// uses TF-IDF cosine similarity over a stopword-filtered token bag,
// which gives a natural 0..1 score and avoids any external service.
package search

import (
	"strings"
	"unicode"
)

// stopwords is a small set of high-frequency English function words.
// ERP-relevant words like "down", "out", "stuck", "blocked", "failed"
// are deliberately kept because they carry signal in this domain.
var stopwords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "is": {}, "are": {}, "was": {}, "were": {},
	"be": {}, "been": {}, "being": {}, "of": {}, "to": {}, "in": {}, "on": {},
	"at": {}, "for": {}, "with": {}, "and": {}, "or": {}, "but": {},
	"this": {}, "that": {}, "these": {}, "those": {}, "it": {}, "its": {},
	"you": {}, "your": {}, "we": {}, "our": {}, "us": {}, "i": {},
	"has": {}, "have": {}, "had": {}, "do": {}, "does": {}, "did": {},
	"how": {}, "what": {}, "when": {}, "where": {}, "who": {}, "why": {},
	"can": {}, "could": {}, "should": {}, "would": {}, "will": {},
	"from": {}, "by": {}, "as": {}, "if": {}, "so": {}, "than": {}, "then": {},
}

// Tokenize lowercases, strips punctuation, and filters stopwords.
// It also drops tokens of length 1 (almost always noise).
func Tokenize(text string) []string {
	if text == "" {
		return nil
	}
	lower := strings.ToLower(text)

	// Replace non-letter/digit runes with spaces. Hyphens and slashes
	// inside terms (e.g. "go-live", "sx.e") become word boundaries —
	// this is fine because the indexer also tokenizes the corpus the
	// same way, so "go-live" becomes tokens ["go", "live"] in both
	// query and document.
	b := strings.Builder{}
	b.Grow(len(lower))
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	fields := strings.Fields(b.String())
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if len(f) < 2 {
			continue
		}
		if _, isStop := stopwords[f]; isStop {
			continue
		}
		out = append(out, f)
	}
	return out
}

// TokenizeRecordText joins the searchable fields of an FAQRecord-shaped
// payload into a single token stream. Selected fields are repeated so
// they get a static boost without needing a multi-field scoring engine:
//
//   - question:      ×3  (the most direct signal of intent)
//   - source_title:  ×2
//   - product:       ×2
//   - module:        ×2
//   - tags (each):   ×2
//   - answer:        ×1  (long, dilutes if over-weighted)
func TokenizeRecordText(question, answer, sourceTitle, product, module string, tags []string) []string {
	parts := []string{
		question, " ", question, " ", question, " ",
		sourceTitle, " ", sourceTitle, " ",
		product, " ", product, " ",
		module, " ", module, " ",
		answer,
	}
	for _, t := range tags {
		parts = append(parts, " ", t, " ", t)
	}
	return Tokenize(strings.Join(parts, ""))
}
