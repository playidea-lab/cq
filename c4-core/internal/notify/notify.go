// Package notify provides Telegram-based notification delivery.
//
// Import-cycle constraint: this package MUST NOT import internal/eventbus.
// All HTTP calls are made directly via net/http.
package notify

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
)

// ---------------------------------------------------------------------------
// postJSON posts body to url and drains the response.
// ---------------------------------------------------------------------------

func postJSON(ctx context.Context, client *http.Client, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("POST: %w", err)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}
	return nil
}
