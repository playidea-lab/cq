// Package edgeagent implements the C5 edge agent: register, heartbeat, poll assignments, download artifacts, run post_command, report status.
package edgeagent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/piqsol/c4/c5/internal/model"
)

// Config holds edge agent configuration.
type Config struct {
	HubURL      string
	APIKey      string
	EdgeName    string
	Workdir     string
	PollInterval time.Duration

	// Metrics reporting (optional — disabled if MetricsCommand is empty).
	MetricsCommand  string
	MetricsInterval time.Duration // default 60s

	// Health check timeout (default 30s).
	HealthCheckTimeout time.Duration

	// Drive upload for collect control action (optional).
	DriveURL    string
	DriveAPIKey string

	// AllowExec enables the "exec" control action (default: disabled for security).
	// Set to true only in trusted environments where Hub-originated shell commands are acceptable.
	AllowExec bool

	// AllowedArtifactURLPrefixes restricts artifact download URLs to specific prefixes.
	// If empty, only the Hub's own origin is allowed (same scheme+host as HubURL).
	AllowedArtifactURLPrefixes []string
}

// runHealthCheck runs the health check command within the given timeout.
func runHealthCheck(ctx context.Context, command string, timeout time.Duration) error {
	hctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(hctx, "sh", "-c", command)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("health check failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Run registers the edge, starts heartbeat, and runs the assignment poll loop until ctx is done.
func Run(ctx context.Context, cfg Config) error {
	baseURL := strings.TrimRight(cfg.HubURL, "/")
	client := &http.Client{Timeout: 30 * time.Second}

	if cfg.Workdir == "" {
		log.Println("edge-agent: --workdir not set; collect path guard disabled (any local path may be uploaded)")
	} else {
		// Normalize workdir to absolute path at startup so filepath.Abs guards are stable
		// even if the process later changes its working directory.
		absWorkdir, err := filepath.Abs(cfg.Workdir)
		if err != nil {
			return fmt.Errorf("workdir: %w", err)
		}
		cfg.Workdir = absWorkdir
	}

	if cfg.MetricsInterval <= 0 {
		cfg.MetricsInterval = 60 * time.Second
	}
	if cfg.HealthCheckTimeout <= 0 {
		cfg.HealthCheckTimeout = 30 * time.Second
	}

	// Register edge (retry with exponential backoff on failure)
	var edgeID string
	retryDelay := 5 * time.Second
	const maxRetryDelay = 60 * time.Second
	for {
		reqBody, _ := json.Marshal(model.EdgeRegisterRequest{Name: cfg.EdgeName})
		req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/edges/register", strings.NewReader(string(reqBody)))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if cfg.APIKey != "" {
			req.Header.Set("X-API-Key", cfg.APIKey)
		}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("edge-agent: register failed: %v; retrying in %s", err, retryDelay)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryDelay):
			}
			if retryDelay < maxRetryDelay {
				retryDelay *= 2
				if retryDelay > maxRetryDelay {
					retryDelay = maxRetryDelay
				}
			}
			continue
		}
		if resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
			resp.Body.Close()
			log.Printf("edge-agent: register returned %d: %s; retrying in %s", resp.StatusCode, string(body), retryDelay)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryDelay):
			}
			if retryDelay < maxRetryDelay {
				retryDelay *= 2
				if retryDelay > maxRetryDelay {
					retryDelay = maxRetryDelay
				}
			}
			continue
		}
		var regResp model.EdgeRegisterResponse
		if err := json.NewDecoder(resp.Body).Decode(&regResp); err != nil {
			resp.Body.Close()
			return fmt.Errorf("decode register response: %w", err)
		}
		resp.Body.Close()
		edgeID = regResp.EdgeID
		break
	}
	log.Printf("edge-agent: registered as %s", edgeID)

	// MetricsReporter goroutine (no-op if MetricsCommand is empty)
	if cfg.MetricsCommand != "" {
		mr := newMetricsReporter(edgeID, baseURL, cfg.APIKey, cfg.MetricsCommand, cfg.MetricsInterval, client)
		go mr.Start(ctx)
	}

	// ControlPoller goroutine
	cp := newControlPoller(edgeID, baseURL, cfg.APIKey, cfg.DriveURL, cfg.DriveAPIKey, cfg.Workdir, cfg.AllowExec, client)
	go cp.Start(ctx)

	// Heartbeat goroutine
	go func() {
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				reqBody, _ := json.Marshal(model.EdgeHeartbeatRequest{EdgeID: edgeID})
				req, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/edges/heartbeat", strings.NewReader(string(reqBody)))
				req.Header.Set("Content-Type", "application/json")
				if cfg.APIKey != "" {
					req.Header.Set("X-API-Key", cfg.APIKey)
				}
				resp, err := client.Do(req)
				if err != nil {
					log.Printf("edge-agent: heartbeat failed: %v", err)
				} else {
					resp.Body.Close()
				}
			}
		}
	}()

	// Poll loop
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/v1/deploy/assignments/"+edgeID, nil)
		if err != nil {
			log.Printf("edge-agent: assignments request: %v", err)
			sleepOrDone(ctx, cfg.PollInterval)
			continue
		}
		if cfg.APIKey != "" {
			req.Header.Set("X-API-Key", cfg.APIKey)
		}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("edge-agent: GET assignments: %v", err)
			sleepOrDone(ctx, cfg.PollInterval)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			sleepOrDone(ctx, cfg.PollInterval)
			continue
		}
		var assignments []model.DeployAssignmentResponse
		if err := json.NewDecoder(resp.Body).Decode(&assignments); err != nil {
			resp.Body.Close()
			log.Printf("edge-agent: decode assignments: %v", err)
			sleepOrDone(ctx, cfg.PollInterval)
			continue
		}
		resp.Body.Close()

		for _, a := range assignments {
			processAssignment(ctx, client, baseURL, cfg.APIKey, edgeID, cfg.Workdir, cfg.HealthCheckTimeout, cfg.AllowedArtifactURLPrefixes, &a)
		}

		sleepOrDone(ctx, cfg.PollInterval)
	}
}

func sleepOrDone(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func processAssignment(ctx context.Context, client *http.Client, baseURL, apiKey, edgeID, workdir string, healthCheckTimeout time.Duration, allowedURLPrefixes []string, a *model.DeployAssignmentResponse) {
	deployDir := filepath.Join(workdir, a.DeployID)
	if err := os.MkdirAll(deployDir, 0o755); err != nil {
		reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed", err.Error())
		return
	}

	rb := newRollbackManager(deployDir)
	// Backup existing deploy dir before downloading new artifacts.
	if err := rb.BeforeDeploy(deployDir); err != nil {
		log.Printf("edge-agent: BeforeDeploy: %v", err)
		// Non-fatal: proceed without rollback capability
	}

	// Report downloading
	reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "downloading", "")

	// Download artifacts
	for _, art := range a.Artifacts {
		dest := filepath.Join(deployDir, art.Path)
		// Guard against path traversal (e.g. art.Path = "../../etc/passwd")
		deployDirClean := filepath.Clean(deployDir) + string(filepath.Separator)
		if !strings.HasPrefix(filepath.Clean(dest)+string(filepath.Separator), deployDirClean) {
			reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed",
				fmt.Sprintf("artifact path traversal rejected: %s", art.Path))
			return
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed", "mkdir: "+err.Error())
			return
		}
		// Guard against SSRF: artifact URL must match Hub origin or an explicit allowlist prefix.
		if !isAllowedArtifactURL(art.URL, baseURL, allowedURLPrefixes) {
			reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed",
				fmt.Sprintf("artifact URL not allowed (SSRF guard): %s", art.URL))
			return
		}
		req, err := http.NewRequestWithContext(ctx, "GET", art.URL, nil)
		if err != nil {
			reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed", "request: "+err.Error())
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed", "download: "+err.Error())
			return
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed", fmt.Sprintf("download %s: status %d", art.Path, resp.StatusCode))
			return
		}
		f, err := os.Create(dest)
		if err != nil {
			resp.Body.Close()
			reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed", "create file: "+err.Error())
			return
		}
		_, err = io.Copy(f, resp.Body)
		f.Close()
		resp.Body.Close()
		if err != nil {
			reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed", "write: "+err.Error())
			return
		}
	}

	// Report deploying
	reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "deploying", "")

	// Run post_command if set — stream output line by line.
	if strings.TrimSpace(a.PostCommand) != "" {
		pr, pw := io.Pipe()
		cmd := exec.CommandContext(ctx, "sh", "-c", a.PostCommand)
		cmd.Dir = deployDir
		cmd.Env = os.Environ()
		cmd.Stdout = pw
		cmd.Stderr = pw
		if err := cmd.Start(); err != nil {
			pw.Close()
			reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed", "start: "+err.Error())
			return
		}
		waitErr := make(chan error, 1)
		go func() {
			waitErr <- cmd.Wait()
			pw.Close()
		}()
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			log.Printf("edge-agent: post_command> %s", scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			log.Printf("edge-agent: post_command scanner error: %v", err)
		}
		if postErr := <-waitErr; postErr != nil {
			reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed", postErr.Error())
			return
		}
	}

	// Health check gate
	if a.HealthCheck.Command != "" {
		timeout := healthCheckTimeout
		if a.HealthCheck.TimeoutSec > 0 {
			timeout = time.Duration(a.HealthCheck.TimeoutSec) * time.Second
		}
		if err := runHealthCheck(ctx, a.HealthCheck.Command, timeout); err != nil {
			log.Printf("edge-agent: health check failed for deploy %s: %v; rolling back", a.DeployID, err)
			if rbErr := rb.Rollback(deployDir); rbErr != nil {
				log.Printf("edge-agent: rollback failed: %v", rbErr)
			}
			reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "failed", err.Error())
			return
		}
	}

	rb.Cleanup(deployDir)
	reportTargetStatus(ctx, client, baseURL, apiKey, a.DeployID, edgeID, "succeeded", "")
}

// isAllowedArtifactURL validates that rawURL is safe to fetch.
// It first checks explicit allowlist prefixes by comparing parsed origins
// (scheme + host) to prevent prefix-spoofing attacks such as
// https://cdn.example.com.attacker.com/ matching the prefix "https://cdn.example.com".
// Falls back to same-origin as hubURL.
// This prevents SSRF attacks where a compromised Hub directs the agent to fetch
// internal metadata endpoints (e.g. http://169.254.169.254/).
func isAllowedArtifactURL(rawURL, hubURL string, allowedPrefixes []string) bool {
	u1, err1 := url.Parse(rawURL)
	if err1 != nil {
		return false
	}
	for _, p := range allowedPrefixes {
		up, err := url.Parse(p)
		if err != nil {
			continue
		}
		// Compare by scheme+host to prevent prefix-spoofing.
		if u1.Scheme == up.Scheme && strings.EqualFold(u1.Host, up.Host) {
			return true
		}
	}
	u2, err2 := url.Parse(hubURL)
	if err2 != nil {
		return false
	}
	return u1.Scheme == u2.Scheme && strings.EqualFold(u1.Host, u2.Host)
}

func reportTargetStatus(ctx context.Context, client *http.Client, baseURL, apiKey, deployID, edgeID, status, errMsg string) {
	body := model.DeployTargetStatusRequest{
		DeployID: deployID,
		EdgeID:   edgeID,
		Status:   status,
		Error:    errMsg,
	}
	reqBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/deploy/target-status", strings.NewReader(string(reqBody)))
	if err != nil {
		log.Printf("edge-agent: report status: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("edge-agent: POST target-status: %v", err)
		return
	}
	resp.Body.Close()
}
