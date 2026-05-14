package usecase

import (
	"fmt"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/port"
)

// TranscriptionService chains the audio converter and the transcriber.
type TranscriptionService struct {
	converter   port.AudioConverter
	transcriber port.Transcriber
}

func NewTranscriptionService(c port.AudioConverter, t port.Transcriber) *TranscriptionService {
	return &TranscriptionService{converter: c, transcriber: t}
}

// FromOGG takes a Telegram .ogg file on disk, converts it to .wav in
// workDir, then transcribes it. The returned TranscriptionResult also
// carries the language Whisper detected so callers can route TTS and
// translation accordingly.
func (s *TranscriptionService) FromOGG(oggPath string, workDir string) (port.TranscriptionResult, error) {
	wavPath, err := s.converter.ToWAV(oggPath, workDir)
	if err != nil {
		return port.TranscriptionResult{}, fmt.Errorf("convert ogg to wav: %w", err)
	}
	return s.transcriber.Transcribe(wavPath)
}
