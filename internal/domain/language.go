package domain

import "strings"

// Language is a small closed-enum value object for the languages the
// MVP supports end-to-end. Adding a new language means adding (a) a
// constant here, (b) a TTS model mapping, and (c) a script detection
// rule in the bot. Everything else flows from these.
type Language string

const (
	LanguageEnglish Language = "en"
	LanguagePunjabi Language = "pa"
)

// LangAuto is the sentinel used when the caller has not yet detected a
// language (e.g. inbound HTTP request without a hint). The use case
// layer falls back to detect-by-script.
const LangAuto Language = ""

// Normalize maps common variants ("eng", "english", "EN", etc.) onto
// the closed enum, and falls back to English when the input is empty
// or unknown.
func NormalizeLanguage(s string) Language {
	v := strings.ToLower(strings.TrimSpace(s))
	switch v {
	case "", "auto", "und":
		return LangAuto
	case "en", "eng", "english":
		return LanguageEnglish
	case "pa", "pan", "punjabi", "panjabi":
		return LanguagePunjabi
	}
	// Best effort: take the first two letters as ISO code.
	if len(v) >= 2 {
		switch Language(v[:2]) {
		case LanguageEnglish, LanguagePunjabi:
			return Language(v[:2])
		}
	}
	return LangAuto
}

// IsSupported reports whether l is one of the closed-enum supported
// languages (not LangAuto and not an unknown ISO code).
func (l Language) IsSupported() bool {
	switch l {
	case LanguageEnglish, LanguagePunjabi:
		return true
	}
	return false
}

// HumanName returns a short English label, used in log/debug output
// (and in prompts to the LLM).
func (l Language) HumanName() string {
	switch l {
	case LanguageEnglish:
		return "English"
	case LanguagePunjabi:
		return "Punjabi"
	}
	return "Unknown"
}
