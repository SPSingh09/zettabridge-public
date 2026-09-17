package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

var httpClient = &http.Client{Timeout: 5 * time.Second}

// Sender posts messages to a Telegram chat via the Bot API.
// A nil Sender is safe to use — all methods are no-ops.
type Sender struct {
	token string
}

// New returns a Sender for the given bot token.
// Returns nil if the token is empty, disabling Telegram globally.
func New(token string) *Sender {
	if token == "" {
		return nil
	}
	return &Sender{token: token}
}

// Send posts text to chatID using HTML parse mode.
// A nil Sender silently succeeds.
func (s *Sender) Send(ctx context.Context, chatID, text string) error {
	if s == nil {
		return nil
	}
	body, err := json.Marshal(map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	})
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram: unexpected status %d", resp.StatusCode)
	}
	return nil
}
