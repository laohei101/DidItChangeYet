package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/laohei101/diditchangeyet/config"
)

// Telegram sends alerts via the Telegram Bot API.
type Telegram struct {
	token  string
	chatID string
	client *http.Client
}

func NewTelegram(cfg config.AlertConfig) (*Telegram, error) {
	if cfg.Token == "" {
		return nil, fmt.Errorf("telegram: token is required")
	}
	if cfg.ChatID == "" {
		return nil, fmt.Errorf("telegram: chat_id is required")
	}
	return &Telegram{
		token:  cfg.Token,
		chatID: cfg.ChatID,
		client: &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (t *Telegram) Name() string { return "telegram" }

func (t *Telegram) Send(message, watchID, current, previous string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)

	payload, err := json.Marshal(map[string]string{
		"chat_id":    t.chatID,
		"text":       message,
		"parse_mode": "HTML",
	})
	if err != nil {
		return fmt.Errorf("telegram: marshalling payload: %w", err)
	}

	resp, err := t.client.Post(apiURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("telegram: HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram: API returned %d: %s", resp.StatusCode, body)
	}

	return nil
}
