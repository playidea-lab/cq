package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/config"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/spf13/cobra"
)

var transferDest string

var transferCmd = &cobra.Command{
	Use:   "transfer <path> --to <worker-id>",
	Short: "Transfer files to a remote worker via relay tunnel",
	Long: `Send a local file or directory to a remote worker via the relay server.

The relay URL is read from .c4/config.yaml (relay.url).
The auth token is read from the saved session.

Example:
  cq transfer data/ --to gpu-server --dest /data/received
  cq transfer model.pt --to worker-abc123`,
	Args: cobra.ExactArgs(1),
	RunE: runTransfer,
}

var transferTo string

func init() {
	transferCmd.Flags().StringVar(&transferTo, "to", "", "target worker ID (required)")
	transferCmd.Flags().StringVar(&transferDest, "dest", "", "destination path on the remote worker (default: same as source base name)")
	_ = transferCmd.MarkFlagRequired("to")
	rootCmd.AddCommand(transferCmd)
}

func runTransfer(cmd *cobra.Command, args []string) error {
	srcPath := args[0]

	// Resolve source path.
	absSrc, err := filepath.Abs(srcPath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	if _, err := os.Stat(absSrc); err != nil {
		return fmt.Errorf("source path %q: %w", absSrc, err)
	}

	// Default dest to source base name.
	destPath := transferDest
	if destPath == "" {
		destPath = "/" + filepath.Base(absSrc)
	}

	// Load relay URL from config.
	cfgMgr, err := config.New(projectDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg := cfgMgr.GetConfig()
	relayWSS := cfg.Relay.URL
	if relayWSS == "" {
		return fmt.Errorf("relay.url not configured in .c4/config.yaml")
	}

	// Load auth token from saved session.
	token := ""
	authClient, err := newAuthClient()
	if err == nil {
		if session, err := authClient.GetSession(); err == nil && session != nil {
			token = session.AccessToken
		}
	}

	// Convert WSS URL to HTTPS for REST calls.
	relayHTTPS := wssToHTTPS(relayWSS)

	// Calculate total size.
	totalBytes, err := calcSize(absSrc)
	if err != nil {
		return fmt.Errorf("calc size: %w", err)
	}

	fmt.Printf("Transferring %s → %s:%s (%s)\n", srcPath, transferTo, destPath, formatBytes(totalBytes))

	// Step 1: POST /tunnel to get tunnel_id.
	tunnelID, err := createTunnel(relayHTTPS, token)
	if err != nil {
		return fmt.Errorf("create tunnel: %w", err)
	}

	// Step 2: Tell the worker to receive via MCP.
	if err := notifyWorker(relayHTTPS, token, transferTo, tunnelID, destPath); err != nil {
		// Non-fatal: worker may already be listening or manually started.
		fmt.Fprintf(os.Stderr, "warning: failed to notify worker (will attempt send anyway): %v\n", err)
	}

	// Step 3: Wait for worker to connect as receiver.
	fmt.Println("Waiting for worker to connect...")
	time.Sleep(2 * time.Second)

	// Step 4: Dial WSS as sender.
	start := time.Now()
	if err := sendTarStream(relayWSS, tunnelID, token, absSrc, totalBytes); err != nil {
		return fmt.Errorf("send: %w", err)
	}

	elapsed := time.Since(start)
	fmt.Printf("Transferred %s in %s to %s:%s\n",
		formatBytes(totalBytes), formatDuration(elapsed), transferTo, destPath)
	return nil
}

// createTunnel POSTs to {relayHTTPS}/tunnel and returns the tunnel_id.
func createTunnel(relayHTTPS, token string) (string, error) {
	u := strings.TrimRight(relayHTTPS, "/") + "/tunnel"
	req, err := http.NewRequest(http.MethodPost, u, nil)
	if err != nil {
		return "", err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("POST %s: %w", u, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("POST %s: status %d: %s", u, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		TunnelID string `json:"tunnel_id"`
		ID       string `json:"id"` // fallback field name
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode tunnel response: %w", err)
	}
	id := result.TunnelID
	if id == "" {
		id = result.ID
	}
	if id == "" {
		return "", fmt.Errorf("tunnel response missing tunnel_id")
	}
	return id, nil
}

// notifyWorker sends a JSON-RPC tools/call to the worker via the relay MCP endpoint.
func notifyWorker(relayHTTPS, token, workerID, tunnelID, destPath string) error {
	u := strings.TrimRight(relayHTTPS, "/") + "/w/" + workerID + "/mcp"

	receiveCmd := fmt.Sprintf("cq tunnel receive %s --dest %s", tunnelID, destPath)
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "c4_execute",
			"arguments": map[string]interface{}{
				"command": receiveCmd,
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// Fire-and-forget with a short timeout — the worker will block until
	// the transfer completes (or times out), so we don't wait for the response.
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Timeout or connection refused — expected for async call.
		return nil
	}
	resp.Body.Close()
	return nil
}

// sendTarStream dials the relay as sender, runs tar cf - <path>, and writes
// the output as binary WebSocket frames with progress reporting.
func sendTarStream(relayWSS, tunnelID, token, srcPath string, totalBytes int64) error {
	// Build sender WSS URL.
	u, err := url.Parse(relayWSS)
	if err != nil {
		return fmt.Errorf("invalid relay URL %q: %w", relayWSS, err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "ws" && scheme != "wss" {
		return fmt.Errorf("unsupported URL scheme %q (want ws or wss)", u.Scheme)
	}

	u.Path = "/tunnel/" + tunnelID
	q := u.Query()
	q.Set("role", "sender")
	if token != "" {
		q.Set("token", token)
	}
	u.RawQuery = q.Encode()

	dialer := ws.Dialer{Timeout: 15 * time.Second}
	ctx := context.Background()
	conn, _, _, err := dialer.Dial(ctx, u.String())
	if err != nil {
		return fmt.Errorf("dial %s: %w", u.String(), err)
	}
	defer conn.Close()

	// Start tar.
	tarArgs := []string{"cf", "-", srcPath}
	tarCmd := exec.Command("tar", tarArgs...)
	tarCmd.Stderr = os.Stderr
	tarOut, err := tarCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("tar stdout pipe: %w", err)
	}
	if err := tarCmd.Start(); err != nil {
		return fmt.Errorf("start tar: %w", err)
	}

	// Track progress.
	var written int64
	lastReport := time.Now()
	startTime := time.Now()
	buf := make([]byte, 64*1024) // 64 KB chunks

	printProgress := func() {
		pct := 0
		if totalBytes > 0 {
			pct = int(written * 100 / totalBytes)
		}
		elapsed := time.Since(startTime).Seconds()
		speed := float64(0)
		if elapsed > 0 {
			speed = float64(written) / elapsed
		}
		eta := ""
		if speed > 0 && totalBytes > written {
			etaSec := float64(totalBytes-written) / speed
			eta = fmt.Sprintf(" ETA %s", formatDuration(time.Duration(etaSec)*time.Second))
		}
		bar := progressBar(pct, 20)
		fmt.Printf("\r  [%s] %d%% %s %s/s%s", bar, pct, formatBytes(written), formatBytes(int64(speed)), eta)
	}

	pipeErr := func() error {
		for {
			n, readErr := tarOut.Read(buf)
			if n > 0 {
				if err := wsutil.WriteClientMessage(conn, ws.OpBinary, buf[:n]); err != nil {
					return fmt.Errorf("ws write: %w", err)
				}
				written += int64(n)

				if time.Since(lastReport) >= 5*time.Second {
					printProgress()
					lastReport = time.Now()
				}
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				return fmt.Errorf("tar read: %w", readErr)
			}
		}
		return nil
	}()

	// Print final progress.
	printProgress()
	fmt.Println()

	// Close WebSocket gracefully.
	_ = wsutil.WriteClientMessage(conn, ws.OpClose, ws.NewCloseFrameBody(ws.StatusNormalClosure, ""))

	// Wait for tar to finish.
	if waitErr := tarCmd.Wait(); waitErr != nil {
		if pipeErr == nil {
			return fmt.Errorf("tar exited: %w", waitErr)
		}
	}

	return pipeErr
}

// wssToHTTPS converts a WebSocket URL to its HTTP(S) equivalent.
// wss://host/path → https://host/path
// ws://host/path  → http://host/path
func wssToHTTPS(wssURL string) string {
	switch {
	case strings.HasPrefix(wssURL, "wss://"):
		return "https://" + wssURL[len("wss://"):]
	case strings.HasPrefix(wssURL, "ws://"):
		return "http://" + wssURL[len("ws://"):]
	}
	return wssURL
}

// calcSize returns the total byte count of path (file or directory tree).
func calcSize(path string) (int64, error) {
	var total int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// formatDuration formats a duration as "Xm Ys" or "Xs".
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := d / time.Minute
	s := (d % time.Minute) / time.Second
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// progressBar returns a simple ASCII progress bar of width w.
func progressBar(pct, w int) string {
	filled := pct * w / 100
	if filled > w {
		filled = w
	}
	bar := strings.Repeat("=", filled)
	if filled < w {
		bar += ">"
		bar += strings.Repeat(" ", w-filled-1)
	}
	return bar
}
