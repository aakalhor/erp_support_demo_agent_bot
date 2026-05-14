// Package voiceservice owns the lifecycle of the persistent FastAPI
// process that hosts faster-whisper + MMS-TTS. The bot spawns it on
// startup (if not already running), polls /health until it's ready,
// and tears it down on shutdown.
package voiceservice

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/shared/logger"
)

type Supervisor struct {
	baseURL    string
	python     string
	script     string
	log        logger.Logger
	cmd        *exec.Cmd
	logFile    *os.File
	owned      bool // true if we spawned the process (so we own teardown)
	httpClient *http.Client
}

func New(baseURL, python, script string, log logger.Logger) *Supervisor {
	return &Supervisor{
		baseURL:    strings.TrimRight(baseURL, "/"),
		python:     python,
		script:     script,
		log:        log,
		httpClient: &http.Client{Timeout: 3 * time.Second},
	}
}

// EnsureRunning checks /health. If the service responds, returns nil
// (we attach to the running instance, won't tear it down on exit). If
// not, spawns python <script> as a child, polls /health for up to
// readyTimeout, and returns once it's ready.
func (s *Supervisor) EnsureRunning(ctx context.Context, readyTimeout time.Duration) error {
	if s.ping() {
		s.log.Infof("voice service already running at %s; attaching", s.baseURL)
		s.owned = false
		return nil
	}

	if strings.TrimSpace(s.python) == "" {
		return fmt.Errorf("voice service not running at %s and VOICE_SERVICE_PYTHON not set", s.baseURL)
	}
	if _, err := os.Stat(s.script); err != nil {
		return fmt.Errorf("voice service script not found at %s: %w", s.script, err)
	}

	logPath := filepath.Join(filepath.Dir(s.script), "..", "tmp", "voice_service.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("create voice service log dir: %w", err)
	}
	f, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("create voice service log %s: %w", logPath, err)
	}
	s.logFile = f

	s.log.Infof("spawning voice service: %s %s (log: %s)", s.python, s.script, logPath)
	cmd := exec.Command(s.python, s.script)
	cmd.Stdout = f
	cmd.Stderr = f
	// Ensure Python flushes stdout/stderr without buffering so the log
	// reflects real-time progress.
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")
	if err := cmd.Start(); err != nil {
		_ = f.Close()
		return fmt.Errorf("start voice service: %w", err)
	}
	s.cmd = cmd
	s.owned = true

	deadline := time.Now().Add(readyTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			s.Shutdown()
			return ctx.Err()
		default:
		}
		// If the process exits early, give up immediately so we don't
		// poll a dead service for the full timeout.
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			return fmt.Errorf("voice service exited during startup; see %s", logPath)
		}
		if s.ping() {
			s.log.Infof("voice service ready at %s after %s", s.baseURL, time.Since(deadline.Add(-readyTimeout)).Round(time.Second))
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	s.Shutdown()
	return fmt.Errorf("voice service did not become ready in %s; see %s", readyTimeout, logPath)
}

func (s *Supervisor) ping() bool {
	resp, err := s.httpClient.Get(s.baseURL + "/health")
	if err != nil {
		return false
	}
	defer io.Copy(io.Discard, resp.Body)
	defer resp.Body.Close()
	return resp.StatusCode/100 == 2
}

// Shutdown kills the spawned voice service (if we own it). Safe to
// call multiple times.
func (s *Supervisor) Shutdown() {
	if !s.owned || s.cmd == nil || s.cmd.Process == nil {
		return
	}
	if err := s.cmd.Process.Kill(); err != nil {
		s.log.Errorf("voice service kill: %v", err)
	} else {
		s.log.Infof("voice service stopped")
	}
	_ = s.cmd.Wait()
	if s.logFile != nil {
		_ = s.logFile.Close()
	}
	s.cmd = nil
}
