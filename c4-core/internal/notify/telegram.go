package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// telegramClient is reused across calls to benefit from HTTP keep-alive.
var telegramClient = &http.Client{Timeout: 15 * time.Second}

// SendTelegram sends a Markdown message to a Telegram chat via the Bot API.
// token is the bot token (without the "bot" prefix), chatID is the target chat identifier.
// A 15-second HTTP timeout is applied automatically.
func SendTelegram(ctx context.Context, token, chatID, message string) error {
	return sendTelegram(ctx, "https://api.telegram.org", token, chatID, message)
}

// BotSender implements eventbus.TelegramSender using a fixed bot token.
type BotSender struct {
	Token string
}

// Send sends a Markdown message to the given chatID via the Telegram Bot API.
func (b *BotSender) Send(chatID, message string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return SendTelegram(ctx, b.Token, chatID, message)
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

	if err := postJSON(ctx, telegramClient, url, body); err != nil {
		return fmt.Errorf("telegram: %w", err)
	}
	return nil
}
