package main

import (
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

func init() {
	relayCmd.AddCommand(relayStatusCmd)
	rootCmd.AddCommand(relayCmd)
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
