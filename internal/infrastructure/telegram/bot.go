package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/port"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/shared/logger"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/shared/util"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/usecase"
)

type Bot struct {
	api             *tgbotapi.BotAPI
	transcription   *usecase.TranscriptionService
	synth           port.Synthesizer // may be nil → voice reply disabled
	apiBaseURL      string
	defaultClientID string
	tmpDir          string
	httpClient      *http.Client
	log             logger.Logger
}

type Config struct {
	Token           string
	APIBaseURL      string
	DefaultClientID string
	TmpDir          string
}

func New(cfg Config, ts *usecase.TranscriptionService, synth port.Synthesizer, log logger.Logger) (*Bot, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, fmt.Errorf("telegram token is empty")
	}
	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("connect to Telegram: %w", err)
	}
	if err := util.EnsureDir(cfg.TmpDir); err != nil {
		return nil, fmt.Errorf("create tmp dir: %w", err)
	}
	return &Bot{
		api:             api,
		transcription:   ts,
		synth:           synth,
		apiBaseURL:      strings.TrimRight(cfg.APIBaseURL, "/"),
		defaultClientID: cfg.DefaultClientID,
		tmpDir:          cfg.TmpDir,
		httpClient:      &http.Client{Timeout: 6 * time.Minute},
		log:             log,
	}, nil
}

func (b *Bot) Run(ctx context.Context) error {
	b.log.Infof("telegram bot @%s online", b.api.Self.UserName)
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	updates := b.api.GetUpdatesChan(u)
	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			return ctx.Err()
		case update, ok := <-updates:
			if !ok {
				return nil
			}
			b.handleUpdate(update)
		}
	}
}

func (b *Bot) handleUpdate(update tgbotapi.Update) {
	if update.Message == nil {
		return
	}
	msg := update.Message
	switch {
	case msg.Voice != nil:
		b.handleVoice(msg)
	case strings.TrimSpace(msg.Text) != "":
		b.handleText(msg)
	}
}

func (b *Bot) handleText(msg *tgbotapi.Message) {
	text := strings.TrimSpace(msg.Text)
	if text == "/start" || text == "/help" {
		b.reply(msg.Chat.ID, helpText())
		return
	}
	lang := detectTextLanguage(text)
	resp, err := b.ask(text, lang)
	if err != nil {
		b.replyError(msg.Chat.ID, "Sorry, the /ask call failed: "+err.Error())
		return
	}
	b.sendAnswer(msg.Chat.ID, "", resp)
}

func (b *Bot) handleVoice(msg *tgbotapi.Message) {
	b.reply(msg.Chat.ID, "Got your voice message. Transcribing locally...")

	fileURL, err := b.api.GetFileDirectURL(msg.Voice.FileID)
	if err != nil {
		b.replyError(msg.Chat.ID, "Could not get file URL from Telegram: "+err.Error())
		return
	}
	oggPath := filepath.Join(b.tmpDir, fmt.Sprintf("voice-%d-%d.ogg", msg.Chat.ID, msg.MessageID))
	if err := util.DownloadToFile(fileURL, oggPath); err != nil {
		b.replyError(msg.Chat.ID, "Could not download voice file: "+err.Error())
		return
	}
	defer os.Remove(oggPath)

	tr, err := b.transcription.FromOGG(oggPath, b.tmpDir)
	if err != nil {
		b.replyError(msg.Chat.ID, "Local transcription failed: "+err.Error())
		return
	}
	transcript := strings.TrimSpace(tr.Transcript)
	if transcript == "" {
		b.replyError(msg.Chat.ID, "Transcript was empty. Try again, speaking more clearly or for a few seconds longer.")
		return
	}

	// Whisper-detected language overrides script detection.
	lang := tr.Language
	if !lang.IsSupported() {
		lang = detectTextLanguage(transcript)
	}

	resp, err := b.ask(transcript, lang)
	if err != nil {
		b.replyError(msg.Chat.ID, "The /ask call failed: "+err.Error())
		return
	}
	b.sendAnswer(msg.Chat.ID, transcript, resp)
}

// sendAnswer always sends a text message and, if the synthesizer is
// available, additionally sends a voice note with the answer.
func (b *Bot) sendAnswer(chatID int64, transcript string, resp domain.AskResponse) {
	b.reply(chatID, FormatReply(transcript, resp))
	if b.synth == nil {
		return
	}
	oggPath, err := b.synth.Speak(resp.Answer, resp.Language, b.tmpDir)
	if err != nil {
		b.log.Errorf("tts failed for chat %d (lang=%s): %v", chatID, resp.Language, err)
		return
	}
	if oggPath == "" {
		return
	}
	defer os.Remove(oggPath)

	voice := tgbotapi.NewVoice(chatID, tgbotapi.FilePath(oggPath))
	if _, err := b.api.Send(voice); err != nil {
		b.log.Errorf("send voice to %d: %v", chatID, err)
	}
}

func (b *Bot) ask(question string, lang domain.Language) (domain.AskResponse, error) {
	body, _ := json.Marshal(domain.AskRequest{
		ClientID: b.defaultClientID,
		Question: question,
		Language: lang,
	})
	req, err := http.NewRequest(http.MethodPost, b.apiBaseURL+"/ask", bytes.NewReader(body))
	if err != nil {
		return domain.AskResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return domain.AskResponse{}, fmt.Errorf("call API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		var apiErr map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if msg, ok := apiErr["error"]; ok {
			return domain.AskResponse{}, fmt.Errorf("API %d: %s", resp.StatusCode, msg)
		}
		return domain.AskResponse{}, fmt.Errorf("API returned status %d", resp.StatusCode)
	}
	var out domain.AskResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return domain.AskResponse{}, fmt.Errorf("decode API response: %w", err)
	}
	return out, nil
}

func (b *Bot) reply(chatID int64, text string) {
	if _, err := b.api.Send(tgbotapi.NewMessage(chatID, text)); err != nil {
		b.log.Errorf("send reply to %d: %v", chatID, err)
	}
}

func (b *Bot) replyError(chatID int64, text string) {
	b.log.Errorf("reply error to %d: %s", chatID, text)
	b.reply(chatID, "Error: "+text)
}

// detectTextLanguage uses script-only detection: any Gurmukhi codepoint
// → Punjabi; otherwise English.
func detectTextLanguage(s string) domain.Language {
	for _, r := range s {
		if r >= 0x0A00 && r <= 0x0A7F {
			return domain.LanguagePunjabi
		}
	}
	return domain.LanguageEnglish
}

func helpText() string {
	return "ERP support demo assistant.\n\n" +
		"Send a voice message or a text question about distribution ERP support, in English or Punjabi.\n" +
		"You will receive both a text reply and a voice reply.\n\n" +
		"Examples:\n" +
		"  - What does your company do?\n" +
		"  - Do you support Infor CloudSuite Distribution?\n" +
		"  - Our ERP system is down.\n" +
		"  - Can you train our warehouse users?\n\n" +
		"This is a demo assistant; it never invents contact details."
}
