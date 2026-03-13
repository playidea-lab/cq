package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/spf13/cobra"
)

// tunnelStarter abstracts cloudflared tunnel startup for testability.
type tunnelStarter interface {
	Start(ctx context.Context, localPort int) (tunnelURL string, cmd *exec.Cmd, err error)
}

// cloudflaredTunnel is the real implementation using cloudflared quick tunnel.
type cloudflaredTunnel struct{}

func (cloudflaredTunnel) Start(ctx context.Context, localPort int) (string, *exec.Cmd, error) {
	if _, err := exec.LookPath("cloudflared"); err != nil {
		return "", nil, fmt.Errorf("cloudflared not found in PATH\n  Install: brew install cloudflared\n  Or: https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/install-and-setup/installation/")
	}

	cmd := exec.CommandContext(ctx, "cloudflared",
		"--config", os.DevNull,
		"--log-format", "json",
		"tunnel", "--url", fmt.Sprintf("http://127.0.0.1:%d", localPort),
	)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", nil, fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", nil, fmt.Errorf("start cloudflared: %w", err)
	}

	// Parse tunnel URL from cloudflared log output (30s timeout).
	urlCh := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		tunnelURL, err := parseTunnelURL(stderr)
		if err != nil {
			errCh <- err
		} else {
			urlCh <- tunnelURL
		}
	}()

	select {
	case u := <-urlCh:
		return u, cmd, nil
	case err := <-errCh:
		_ = cmd.Process.Kill()
		return "", nil, fmt.Errorf("parse tunnel URL: %w", err)
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		return "", nil, fmt.Errorf("timeout waiting for cloudflared tunnel URL (30s)")
	}
}

// jsonLogLine matches cloudflared JSON log entries.
var reTunnelURL = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

// parseTunnelURL reads cloudflared stderr and extracts the tunnel URL.
// It prefers the JSON "url" field, falling back to regex on raw output.
func parseTunnelURL(r interface{ Read(p []byte) (n int, err error) }) (string, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()

		// Try JSON parse first.
		var entry struct {
			URL string `json:"url"`
		}
		if json.Unmarshal([]byte(line), &entry) == nil && entry.URL != "" {
			if strings.HasPrefix(entry.URL, "https://") {
				return entry.URL, nil
			}
		}

		// Regex fallback for non-JSON output.
		if m := reTunnelURL.FindString(line); m != "" {
			return m, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no tunnel URL found in cloudflared output")
}

// Transfer flags.
var (
	hubTransferTo   string
	hubTransferPort int
)

var hubTransferCmd = &cobra.Command{
	Use:   "transfer <path>",
	Short: "Transfer a file to a remote worker via cloudflared tunnel",
	Long: `Transfer a local file to a remote Hub worker using a cloudflared quick tunnel.

The file is served over HTTPS via a temporary cloudflared tunnel URL.
The remote worker downloads it using wget (with resume support).

Requires cloudflared in PATH (brew install cloudflared).

Example:
  cq hub transfer dataset.tar.gz --to worker-abc123`,
	Args: cobra.ExactArgs(1),
	RunE: runHubTransfer,
}

func init() {
	hubTransferCmd.Flags().StringVar(&hubTransferTo, "to", "", "target worker ID (required)")
	hubTransferCmd.Flags().IntVar(&hubTransferPort, "port", 0, "local HTTP port (0 = auto)")
	_ = hubTransferCmd.MarkFlagRequired("to")
}

func runHubTransfer(cmd *cobra.Command, args []string) error {
	return runHubTransferWithTunnel(cmd, args, cloudflaredTunnel{})
}

func runHubTransferWithTunnel(cmd *cobra.Command, args []string, ts tunnelStarter) error {
	filePath := args[0]

	// Validate file exists.
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("file not found: %s", filePath)
	}

	// Generate a random 32-byte hex token for URL security.
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return fmt.Errorf("generate token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)
	filename := url.PathEscape(filepath.Base(filePath))

	// Start HTTP server on a dynamic port.
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", hubTransferPort))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	localPort := ln.Addr().(*net.TCPAddr).Port

	servePath := "/t/" + token + "/" + filename
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != servePath {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, filePath)
		}),
	}
	go func() { _ = srv.Serve(ln) }()

	// Setup signal context for cleanup.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start cloudflared tunnel.
	fmt.Printf("Starting cloudflared tunnel (local port %d)...\n", localPort)
	tunnelURL, cfCmd, err := ts.Start(ctx, localPort)
	if err != nil {
		_ = srv.Shutdown(context.Background())
		return err
	}

	downloadURL := tunnelURL + servePath
	fmt.Printf("File:    %s (%d bytes)\n", filepath.Base(filePath), info.Size())
	fmt.Printf("Tunnel:  %s\n", tunnelURL)

	// Cleanup function: kill cloudflared, shutdown HTTP server.
	cleanup := func() {
		if cfCmd != nil && cfCmd.Process != nil {
			_ = cfCmd.Process.Signal(syscall.SIGINT)
			done := make(chan struct{})
			go func() {
				_ = cfCmd.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				_ = cfCmd.Process.Kill()
			}
		}
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}
	defer cleanup()

	// Submit wget job to the target worker via Hub capability invoke.
	client, err := newHubClient()
	if err != nil {
		return err
	}

	wgetCmd := fmt.Sprintf("wget -c %q", downloadURL)
	resp, err := client.InvokeCapability(&hub.InvokeCapabilityRequest{
		Capability: "run_command",
		Params: map[string]any{
			"command":   wgetCmd,
			"worker_id": hubTransferTo,
		},
		Name:       "hub-transfer",
		TimeoutSec: 3600,
	})
	if err != nil {
		return fmt.Errorf("invoke capability: %w", err)
	}

	jobID := resp.JobID
	fmt.Printf("Job:     %s (status=%s)\n", jobID, resp.Status)
	fmt.Println("Waiting for download to complete (Ctrl+C to abort)...")

	// Poll job status every 3 seconds.
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "\nAborted. Remote wget may still be running (job %s).\n", jobID)
			return nil
		case <-ticker.C:
			job, jerr := client.GetJob(jobID)
			if jerr != nil {
				fmt.Fprintf(os.Stderr, "poll error: %v\n", jerr)
				continue
			}
			if hub.IsTerminal(job.Status) {
				if job.Status == "SUCCEEDED" {
					fmt.Printf("Transfer complete (job %s)\n", jobID)
					return nil
				}
				return fmt.Errorf("transfer failed: job %s status=%s\n  Re-run: cq hub transfer %s --to %s",
					jobID, job.Status, filePath, hubTransferTo)
			}
		}
	}
}
