package port

import "github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"

// Synthesizer turns text into a local audio file (OGG/Opus). The MVP
// uses Meta's MMS-TTS via a Python wrapper followed by ffmpeg to
// produce a Telegram-friendly voice-note container.
type Synthesizer interface {
	// Speak writes a voice file for text in lang into outputDir and
	// returns the resulting path. An empty path with a nil error means
	// the synthesizer chose to skip (e.g. text was empty).
	Speak(text string, lang domain.Language, outputDir string) (string, error)
}
