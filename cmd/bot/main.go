// Command bot runs the Telegram bot. It supervises the persistent
// Python voice service (whisper + TTS), wires HTTP adapters to it, and
// drives the long-polling loop.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/config"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/telegram"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/transcription"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/tts"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/voiceservice"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/port"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/shared/logger"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/usecase"
)

func main() {
	log := logger.New("bot")

	cfg, err := config.Load()
	if err != nil {
		log.Errorf("load config: %v", err)
		os.Exit(1)
	}
	if err := cfg.RequireTelegramToken(); err != nil {
		log.Errorf("%v", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Supervise (or attach to) the persistent voice service.
	var sup *voiceservice.Supervisor
	if cfg.VoiceServiceSpawn {
		sup = voiceservice.New(cfg.VoiceServiceURL, cfg.VoiceServicePython, cfg.VoiceServiceScript, log)
		// 3 minutes is enough for whisper "small" + two MMS-TTS models
		// to load on a cold disk; once cached the warm path is ~10-30s.
		if err := sup.EnsureRunning(ctx, 3*time.Minute); err != nil {
			log.Errorf("voice service: %v", err)
			os.Exit(1)
		}
		defer sup.Shutdown()
	} else {
		log.Infof("VOICE_SERVICE_SPAWN=false; assuming voice service is already running at %s", cfg.VoiceServiceURL)
	}

	converter := transcription.NewFFmpegConverter(cfg.FFmpegPath)
	whisper := transcription.NewLocalWhisperTranscriber(cfg.VoiceServiceURL)
	tsvc := usecase.NewTranscriptionService(converter, whisper)

	var synth port.Synthesizer
	if cfg.TTSEnabled {
		synth = tts.NewLocalSynthesizer(cfg.VoiceServiceURL, cfg.FFmpegPath)
		log.Infof("tts enabled via voice service %s", cfg.VoiceServiceURL)
	} else {
		log.Infof("tts disabled by config; bot will reply with text only")
	}

	bot, err := telegram.New(telegram.Config{
		Token:           cfg.TelegramBotToken,
		APIBaseURL:      cfg.APIBaseURL,
		DefaultClientID: cfg.DefaultClientID,
		TmpDir:          cfg.TmpDir,
	}, tsvc, synth, log)
	if err != nil {
		log.Errorf("init bot: %v", err)
		os.Exit(1)
	}

	if err := bot.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Errorf("bot exited: %v", err)
		os.Exit(1)
	}
	log.Infof("bot stopped")
}
