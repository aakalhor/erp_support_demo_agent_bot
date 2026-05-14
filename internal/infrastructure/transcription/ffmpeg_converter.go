// Package transcription contains the local ffmpeg + whisper adapters.
// Every external command is invoked via os/exec; nothing leaves the
// machine.
package transcription

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// FFmpegConverter implements port.AudioConverter by shelling out to a
// local ffmpeg binary. The output is 16 kHz mono PCM WAV, which is what
// whisper.cpp / faster-whisper expects.
type FFmpegConverter struct {
	binary string
}

// ffmpegTimeout bounds every ffmpeg run so a malformed audio file
// cannot stall the bot forever.
const ffmpegTimeout = 60 * time.Second

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

	ctx, cancel := context.WithTimeout(context.Background(), ffmpegTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.binary, "-y", "-i", inputPath, "-ar", "16000", "-ac", "1", outPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("ffmpeg timed out after %s converting %s; stderr: %s", ffmpegTimeout, inputPath, strings.TrimSpace(stderr.String()))
		}
		return "", fmt.Errorf("ffmpeg failed (binary %q): %w; stderr: %s", c.binary, err, strings.TrimSpace(stderr.String()))
	}
	return outPath, nil
}
