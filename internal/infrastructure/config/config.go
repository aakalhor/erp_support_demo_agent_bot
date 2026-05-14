// Package config loads runtime configuration from environment
// variables (with optional .env support via godotenv).
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	TelegramBotToken string
	APIBaseURL       string
	FFmpegPath       string
	DefaultClientID  string
	Port             int
	TmpDir           string
	SeedPath         string
	IndexPath        string
	KnowledgePath    string

	// LLM
	OllamaBaseURL string
	OllamaModel   string
	LLMEnabled    bool

	// Voice service (persistent FastAPI process hosting whisper + TTS)
	VoiceServiceURL    string
	VoiceServicePython string // python executable used to spawn the service
	VoiceServiceScript string // path to scripts/voice_service.py
	VoiceServiceSpawn  bool   // if true, the bot will auto-spawn the service
	TTSEnabled         bool
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	port, err := envInt("PORT", 8080)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		TelegramBotToken:   strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		APIBaseURL:         envOrDefault("API_BASE_URL", "http://localhost:8080"),
		FFmpegPath:         envOrDefault("FFMPEG_PATH", "ffmpeg"),
		DefaultClientID:    envOrDefault("DEFAULT_CLIENT_ID", "global"),
		Port:               port,
		TmpDir:             envOrDefault("TMP_DIR", "./tmp"),
		SeedPath:           envOrDefault("SEED_PATH", "./data/seed_faq.jsonl"),
		IndexPath:          envOrDefault("INDEX_PATH", "./storage/index.json"),
		KnowledgePath:      envOrDefault("KNOWLEDGE_PATH", "./data/general_knowledge.md"),
		OllamaBaseURL:      envOrDefault("OLLAMA_BASE_URL", "http://localhost:11434"),
		OllamaModel:        envOrDefault("OLLAMA_MODEL", "qwen3:8b"),
		LLMEnabled:         envBool("LLM_ENABLED", true),
		VoiceServiceURL:    envOrDefault("VOICE_SERVICE_URL", "http://127.0.0.1:7860"),
		VoiceServicePython: envOrDefault("VOICE_SERVICE_PYTHON", "python"),
		VoiceServiceScript: envOrDefault("VOICE_SERVICE_SCRIPT", "./scripts/voice_service.py"),
		VoiceServiceSpawn:  envBool("VOICE_SERVICE_SPAWN", true),
		TTSEnabled:         envBool("TTS_ENABLED", true),
	}
	return cfg, nil
}

func (c *Config) RequireTelegramToken() error {
	if strings.TrimSpace(c.TelegramBotToken) == "" {
		return errors.New("TELEGRAM_BOT_TOKEN is not set; see .env.example and BotFather instructions in README")
	}
	return nil
}

func envOrDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func envInt(key string, def int) (int, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("env %s: not an int: %w", key, err)
	}
	return n, nil
}
