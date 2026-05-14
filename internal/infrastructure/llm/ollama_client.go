// Package llm holds the Ollama HTTP client. It is intentionally a thin
// wrapper around the local /api/chat endpoint — nothing leaves the
// machine, no API keys, no streaming, no SDK dependency.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to a local Ollama server (default http://localhost:11434).
type Client struct {
	BaseURL    string
	Model      string
	HTTPClient *http.Client
}

func NewClient(baseURL, model string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "http://localhost:11434"
	}
	if strings.TrimSpace(model) == "" {
		model = "qwen3:8b"
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		// Generous timeout: an 8B model on CPU at Q4 can take a while
		// for the first token. 5 minutes is enough headroom while
		// still bounding the wait.
		HTTPClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

// Message is one turn in the chat history.
type Message struct {
	Role    string `json:"role"`    // "system" | "user" | "assistant"
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string         `json:"model"`
	Messages []Message      `json:"messages"`
	Stream   bool           `json:"stream"`
	Options  map[string]any `json:"options,omitempty"`
	// Tell Qwen3 to skip its "thinking" mode for short factual answers.
	Think *bool `json:"think,omitempty"`
}

type chatResponse struct {
	Model      string  `json:"model"`
	Message    Message `json:"message"`
	Done       bool    `json:"done"`
	DoneReason string  `json:"done_reason"`
}

// Chat sends a non-streaming chat completion and returns the assistant
// message content.
func (c *Client) Chat(ctx context.Context, messages []Message, opts map[string]any) (string, error) {
	think := false
	body, err := json.Marshal(chatRequest{
		Model:    c.Model,
		Messages: messages,
		Stream:   false,
		Options:  opts,
		Think:    &think,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call ollama %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}
	return strings.TrimSpace(out.Message.Content), nil
}

// Health pings /api/tags. Used at startup to fail fast if the daemon
// is not running.
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/tags", nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama at %s unreachable: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("ollama at %s returned %d", c.BaseURL, resp.StatusCode)
	}
	return nil
}
