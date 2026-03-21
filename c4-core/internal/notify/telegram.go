package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SendTelegram sends a Markdown message to a Telegram chat via the Bot API.
// token is the bot token (without the "bot" prefix), chatID is the target chat identifier.
// A 15-second HTTP timeout is applied automatically.
func SendTelegram(ctx context.Context, token, chatID, message string) error {
	return sendTelegram(ctx, "https://api.telegram.org", token, chatID, message)
}

// sendTelegram is the testable variant that accepts an explicit base URL.
func sendTelegram(ctx context.Context, baseURL, token, chatID, message string) error {
	payload := map[string]string{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "Markdown",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram: marshal: %w", err)
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", baseURL, token)
	client := &http.Client{Timeout: 15 * time.Second}

	if err := postJSON(ctx, client, url, body); err != nil {
		return fmt.Errorf("telegram: %w", err)
	}
	return nil
}
