package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/config"
	"github.com/spf13/cobra"
)

var relayCmd = &cobra.Command{
	Use:   "relay",
	Short: "Relay server commands",
}

var relayStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check relay connection status",
	Long: `Check if the relay server is reachable and if this worker is connected.

Example:
  cq relay status`,
	RunE: runRelayStatus,
}

var relayCallCmd = &cobra.Command{
	Use:   "call <worker> <method>",
	Short: "Send a JSON-RPC command to a remote worker via relay",
	Long: `Send a JSON-RPC method call to a remote worker through the relay server.

Currently supported methods:
  restart   Trigger a graceful restart of the worker's child process (requires --watchdog)

Example:
  cq relay call my-worker restart`,
	Args: cobra.ExactArgs(2),
	RunE: runRelayCall,
}

func init() {
	relayCmd.AddCommand(relayStatusCmd)
	relayCmd.AddCommand(relayCallCmd)
	rootCmd.AddCommand(relayCmd)
}

func runRelayCall(cmd *cobra.Command, args []string) error {
	workerID := args[0]
	method := args[1]

	if method != "restart" {
		return fmt.Errorf("unsupported method %q; currently supported: restart", method)
	}

	cfgMgr, err := config.New(projectDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg := cfgMgr.GetConfig()

	if !cfg.Relay.Enabled || cfg.Relay.URL == "" {
		return fmt.Errorf("relay not configured; run 'cq auth login' to configure relay")
	}

	relayHTTPS := strings.Replace(cfg.Relay.URL, "wss://", "https://", 1)
	relayHTTPS = strings.Replace(relayHTTPS, "ws://", "http://", 1)
	relayHTTPS = strings.TrimRight(relayHTTPS, "/")

	// The watchdog registers with worker_id "<hostname>-watchdog".
	// Accept the user passing either the bare hostname or the full watchdog ID.
	targetWorkerID := workerID
	if !strings.HasSuffix(workerID, "-watchdog") {
		targetWorkerID = workerID + "-watchdog"
	}

	// Build JSON-RPC request.
	rpcReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "worker/restart",
	}
	body, err := json.Marshal(rpcReq)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/w/%s/mcp", relayHTTPS, targetWorkerID)
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Cloud.AnonKey != "" {
		req.Header.Set("apikey", cfg.Cloud.AnonKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("relay call: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusServiceUnavailable {
		return fmt.Errorf("worker %q not connected to relay (is 'cq serve --watchdog' running?)", targetWorkerID)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("relay returned %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse JSON-RPC response.
	var rpcResp struct {
		Result string          `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		fmt.Println(string(respBody))
		return nil
	}
	if rpcResp.Error != nil && string(rpcResp.Error) != "null" {
		return fmt.Errorf("worker error: %s", string(rpcResp.Error))
	}

	fmt.Printf("worker %q restart triggered: %s\n", workerID, rpcResp.Result)
	return nil
}

func runRelayStatus(cmd *cobra.Command, args []string) error {
	// Load relay config
	cfgMgr, err := config.New(projectDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg := cfgMgr.GetConfig()

	if !cfg.Relay.Enabled || cfg.Relay.URL == "" {
		fmt.Println("Relay: not configured")
		fmt.Println("  Run 'cq auth login' to auto-configure relay.")
		return nil
	}

	relayHTTPS := strings.Replace(cfg.Relay.URL, "wss://", "https://", 1)
	relayHTTPS = strings.Replace(relayHTTPS, "ws://", "http://", 1)

	hostname, _ := os.Hostname()
	client := &http.Client{Timeout: 10 * time.Second}

	// 1. Relay health
	fmt.Printf("Relay:  %s\n", relayHTTPS)
	healthResp, err := client.Get(relayHTTPS + "/health")
	if err != nil {
		fmt.Printf("  Status:  offline (%v)\n", err)
		return nil
	}
	defer healthResp.Body.Close()
	body, _ := io.ReadAll(healthResp.Body)
	var health struct {
		Status  string `json:"status"`
		Workers int    `json:"workers"`
	}
	json.Unmarshal(body, &health)
	fmt.Printf("  Status:  %s (%d workers connected)\n", health.Status, health.Workers)

	// 2. This worker
	fmt.Printf("Worker: %s\n", hostname)
	workerResp, err := client.Get(relayHTTPS + "/w/" + hostname + "/health")
	if err != nil {
		fmt.Printf("  Status:  unknown (%v)\n", err)
		return nil
	}
	defer workerResp.Body.Close()

	if workerResp.StatusCode == 200 {
		fmt.Printf("  Status:  connected ✓\n")
		fmt.Printf("  MCP URL: %s/w/%s/mcp\n", relayHTTPS, hostname)
	} else {
		fmt.Printf("  Status:  disconnected ✗\n")
		fmt.Println("  Check 'cq serve' is running.")
	}

	return nil
}
