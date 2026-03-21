package botstore

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// BotInfo contains the information returned by Telegram's getMe endpoint.
type BotInfo struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

// telegramResponse is the envelope returned by Telegram Bot API.
type telegramResponse struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	// Error fields
	ErrorCode   int    `json:"error_code,omitempty"`
	Description string `json:"description,omitempty"`
}

// httpClient is used for Telegram API calls with a short timeout.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// VerifyToken calls the Telegram Bot API getMe endpoint with the provided token
// and returns a BotInfo on success.
// Returns an error if the token is invalid or the network call fails.
func VerifyToken(token string) (BotInfo, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", token)
	resp, err := httpClient.Get(url)
	if err != nil {
		return BotInfo{}, fmt.Errorf("telegram getMe: %w", err)
	}
	defer resp.Body.Close()

	var tgResp telegramResponse
	if err := json.NewDecoder(resp.Body).Decode(&tgResp); err != nil {
		return BotInfo{}, fmt.Errorf("decode telegram response: %w", err)
	}

	if !tgResp.OK {
		return BotInfo{}, fmt.Errorf("telegram API error %d: %s", tgResp.ErrorCode, tgResp.Description)
	}

	var info BotInfo
	if err := json.Unmarshal(tgResp.Result, &info); err != nil {
		return BotInfo{}, fmt.Errorf("parse bot info: %w", err)
	}
	return info, nil
}
