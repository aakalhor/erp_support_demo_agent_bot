package transcription

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/port"
)

// LocalWhisperTranscriber implements port.Transcriber by POSTing to the
// persistent voice service (scripts/voice_service.py). The service holds
// the faster-whisper "small" model in memory across calls, so each
// transcription only pays for inference (~1-2s for a short clip) rather
// than the previous ~5-15s cold start.
type LocalWhisperTranscriber struct {
	baseURL    string
	httpClient *http.Client
}

func NewLocalWhisperTranscriber(baseURL string) *LocalWhisperTranscriber {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "http://127.0.0.1:7860"
	}
	return &LocalWhisperTranscriber{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 3 * time.Minute},
	}
}

type transcribeReq struct {
	AudioPath string `json:"audio_path"`
	Language  string `json:"language,omitempty"`
}

type transcribeResp struct {
	Transcript          string  `json:"transcript"`
	Language            string  `json:"language"`
	LanguageProbability float64 `json:"language_probability"`
}

func (t *LocalWhisperTranscriber) Transcribe(audioPath string) (port.TranscriptionResult, error) {
	if _, err := os.Stat(audioPath); err != nil {
		return port.TranscriptionResult{}, fmt.Errorf("%w: audio file not found at %s", domain.ErrTranscription, audioPath)
	}

	body, _ := json.Marshal(transcribeReq{AudioPath: audioPath})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/transcribe", bytes.NewReader(body))
	if err != nil {
		return port.TranscriptionResult{}, fmt.Errorf("%w: %v", domain.ErrTranscription, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return port.TranscriptionResult{}, fmt.Errorf("%w: voice service unreachable at %s: %v. Is scripts/voice_service.py running?",
			domain.ErrTranscription, t.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		var apiErr struct {
			Detail string `json:"detail"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		return port.TranscriptionResult{}, fmt.Errorf("%w: voice service returned %d: %s",
			domain.ErrTranscription, resp.StatusCode, apiErr.Detail)
	}

	var out transcribeResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return port.TranscriptionResult{}, fmt.Errorf("%w: decode response: %v", domain.ErrTranscription, err)
	}
	return port.TranscriptionResult{
		Transcript:          strings.TrimSpace(out.Transcript),
		Language:            domain.NormalizeLanguage(out.Language),
		LanguageProbability: out.LanguageProbability,
	}, nil
}
