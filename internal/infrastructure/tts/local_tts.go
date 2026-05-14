// Package tts contains the local Synthesizer implementation. It POSTs
// to the persistent voice service (scripts/voice_service.py) for the
// neural synthesis step, then re-encodes the WAV into an OGG/Opus voice
// note via ffmpeg so Telegram renders it as a real voice message.
package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
)

type LocalSynthesizer struct {
	baseURL    string
	ffmpegPath string
	httpClient *http.Client
}

func NewLocalSynthesizer(baseURL, ffmpegPath string) *LocalSynthesizer {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "http://127.0.0.1:7860"
	}
	if strings.TrimSpace(ffmpegPath) == "" {
		ffmpegPath = "ffmpeg"
	}
	return &LocalSynthesizer{
		baseURL:    strings.TrimRight(baseURL, "/"),
		ffmpegPath: ffmpegPath,
		httpClient: &http.Client{Timeout: 2 * time.Minute},
	}
}

type speakReq struct {
	Lang    string `json:"lang"`
	Text    string `json:"text"`
	OutPath string `json:"out_path"`
}

type speakResp struct {
	OutPath    string `json:"out_path"`
	SampleRate int    `json:"sample_rate"`
}

// Speak synthesises text and returns the path to a .ogg voice note.
// Empty text → ("", nil) so the caller can quietly skip voice.
func (s *LocalSynthesizer) Speak(text string, lang domain.Language, outputDir string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}
	if !lang.IsSupported() {
		lang = domain.LanguageEnglish
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}

	stamp := time.Now().UnixNano()
	wavPath := filepath.Join(outputDir, fmt.Sprintf("tts-%d.wav", stamp))
	oggPath := filepath.Join(outputDir, fmt.Sprintf("tts-%d.ogg", stamp))

	// 1. WAV via the persistent voice service.
	body, _ := json.Marshal(speakReq{Lang: string(lang), Text: text, OutPath: wavPath})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/speak", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("voice service unreachable at %s: %w. Is scripts/voice_service.py running?", s.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		var apiErr struct {
			Detail string `json:"detail"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		return "", fmt.Errorf("voice service /speak returned %d: %s", resp.StatusCode, apiErr.Detail)
	}
	var sp speakResp
	if err := json.NewDecoder(resp.Body).Decode(&sp); err != nil {
		return "", fmt.Errorf("decode /speak response: %w", err)
	}
	if _, err := os.Stat(wavPath); err != nil {
		return "", fmt.Errorf("voice service did not produce %s: %w", wavPath, err)
	}

	// 2. WAV -> OGG/Opus via ffmpeg (Telegram voice-note format).
	enc := exec.Command(
		s.ffmpegPath, "-y",
		"-i", wavPath,
		"-c:a", "libopus",
		"-b:a", "32k",
		"-ar", "48000",
		"-ac", "1",
		oggPath,
	)
	var encErr bytes.Buffer
	enc.Stderr = &encErr
	if err := enc.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg wav->ogg failed: %w; stderr: %s", err, strings.TrimSpace(encErr.String()))
	}
	_ = os.Remove(wavPath)
	return oggPath, nil
}
