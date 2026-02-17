package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// MetricMessage represents a message received from the metrics WebSocket.
type MetricMessage struct {
	Type    string         `json:"type"` // "metric", "status", "history", "error"
	JobID   string         `json:"job_id,omitempty"`
	Step    int            `json:"step,omitempty"`
	Metrics map[string]any `json:"metrics,omitempty"`
	Status  string         `json:"status,omitempty"` // for type=status: SUCCEEDED, FAILED, etc.
	Error   string         `json:"error,omitempty"`
}

const (
	wsMaxReconnect = 10
	wsBaseBackoff  = 1 * time.Second
	wsMaxBackoff   = 30 * time.Second
)

// StreamMetrics connects to the Hub WebSocket and streams metrics for a job.
// It calls onMessage for each received message and stops when:
// - ctx is cancelled
// - job reaches terminal status (SUCCEEDED, FAILED, CANCELLED)
// Auto-reconnects on connection loss with exponential backoff.
func (c *Client) StreamMetrics(ctx context.Context, jobID string, includeHistory bool, onMessage func(MetricMessage)) error {
	wsURL := c.wsURL(jobID, includeHistory)

	var reconnects int
	for {
		err := c.streamOnce(ctx, wsURL, onMessage)
		if err == nil {
			return nil // clean exit (terminal status or OpClose)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		reconnects++
		if reconnects > wsMaxReconnect {
			return fmt.Errorf("websocket: max reconnect attempts reached (%d): %w", wsMaxReconnect, err)
		}

		backoff := wsBaseBackoff << (reconnects - 1)
		if backoff > wsMaxBackoff {
			backoff = wsMaxBackoff
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		// After reconnect, always request history to avoid gaps
		wsURL = c.wsURL(jobID, true)
	}
}

// streamOnce runs a single WebSocket connection lifecycle.
func (c *Client) streamOnce(ctx context.Context, wsURL string, onMessage func(MetricMessage)) error {
	dialer := ws.Dialer{
		Header: ws.HandshakeHeaderHTTP{
			"X-API-Key":   []string{c.apiKey},
			"X-Team-ID":   []string{c.teamID},
			"X-Worker-ID": []string{c.workerID},
		},
		Timeout: 10 * time.Second,
	}

	conn, _, _, err := dialer.Dial(ctx, wsURL)
	if err != nil {
		return fmt.Errorf("websocket connect: %w", err)
	}
	defer conn.Close()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		readDeadline := time.Now().Add(5 * time.Second)
		if dl, ok := ctx.Deadline(); ok && dl.Before(readDeadline) {
			readDeadline = dl
		}
		if nc, ok := conn.(net.Conn); ok {
			nc.SetReadDeadline(readDeadline)
		}

		data, op, err := wsutil.ReadServerData(conn)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if isTimeout(err) {
				continue
			}
			return fmt.Errorf("websocket read: %w", err)
		}

		if op == ws.OpClose {
			return nil
		}

		if op == ws.OpText || op == ws.OpBinary {
			var msg MetricMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			onMessage(msg)
			if msg.Type == "status" && isTerminalStatus(msg.Status) {
				return nil
			}
		}
	}
}

// wsURL builds the WebSocket URL for metrics streaming.
func (c *Client) wsURL(jobID string, includeHistory bool) string {
	base := c.baseURL
	base = strings.Replace(base, "http://", "ws://", 1)
	base = strings.Replace(base, "https://", "wss://", 1)
	url := fmt.Sprintf("%s%s/ws/metrics/%s", base, c.apiPrefix, jobID)
	if includeHistory {
		url += "?include_history=true"
	}
	return url
}

// IsTerminal returns true if a job status is terminal.
func IsTerminal(status string) bool {
	switch status {
	case "SUCCEEDED", "FAILED", "CANCELLED":
		return true
	}
	return false
}

func isTerminalStatus(status string) bool {
	return IsTerminal(status)
}

func isTimeout(err error) bool {
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return true
	}
	if err == io.EOF {
		return false
	}
	return false
}
