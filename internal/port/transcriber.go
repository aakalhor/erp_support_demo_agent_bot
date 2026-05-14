package port

import "github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"

// TranscriptionResult bundles the transcript text with the language
// the engine detected. Detection is the engine's responsibility because
// it can do it during decoding for free; downstream code (translator,
// TTS) then knows which language to route in.
type TranscriptionResult struct {
	Transcript          string
	Language            domain.Language
	LanguageProbability float64
}

// Transcriber turns a local audio file into text. The MVP uses a local
// whisper CLI; the port stays minimal so other engines can slot in.
type Transcriber interface {
	// Transcribe reads audioPath (a local .wav file) and returns the
	// detected transcript + language. It never reaches the network.
	Transcribe(audioPath string) (TranscriptionResult, error)
}

// AudioConverter normalises any inbound audio container/codec into a
// .wav file the transcriber can read. The local implementation shells
// out to ffmpeg.
type AudioConverter interface {
	// ToWAV converts inputPath into a new .wav file in outputDir and
	// returns the resulting file path.
	ToWAV(inputPath string, outputDir string) (string, error)
}
