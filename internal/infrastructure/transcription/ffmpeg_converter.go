// Package transcription contains the local ffmpeg + whisper adapters.
// Every external command is invoked via os/exec; nothing leaves the
// machine.
package transcription

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// FFmpegConverter implements port.AudioConverter by shelling out to a
// local ffmpeg binary. The output is 16 kHz mono PCM WAV, which is what
// whisper.cpp expects.
type FFmpegConverter struct {
	binary string
}

func NewFFmpegConverter(binary string) *FFmpegConverter {
	if strings.TrimSpace(binary) == "" {
		binary = "ffmpeg"
	}
	return &FFmpegConverter{binary: binary}
}

// ToWAV converts inputPath into a freshly-named .wav inside outputDir
// and returns the resulting path.
func (c *FFmpegConverter) ToWAV(inputPath string, outputDir string) (string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create work dir: %w", err)
	}
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	stamp := time.Now().UnixNano()
	outPath := filepath.Join(outputDir, fmt.Sprintf("%s-%d.wav", base, stamp))

	// -y      : overwrite output
	// -i      : input
	// -ar 16000: 16 kHz (whisper's expected sample rate)
	// -ac 1   : mono
	cmd := exec.Command(c.binary, "-y", "-i", inputPath, "-ar", "16000", "-ac", "1", outPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg failed (binary %q): %w; stderr: %s", c.binary, err, strings.TrimSpace(stderr.String()))
	}
	return outPath, nil
}
