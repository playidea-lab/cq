package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RegisterWorker registers this machine as a Hub worker.
// The returned worker ID is stored on the client for subsequent calls.
func (c *Client) RegisterWorker(capabilities map[string]any) (string, error) {
	body := map[string]any{
		"capabilities": capabilities,
	}
	var resp WorkerRegisterResponse
	if err := c.post("/workers/register", body, &resp); err != nil {
		return "", fmt.Errorf("register worker: %w", err)
	}
	c.workerID = resp.WorkerID
	return resp.WorkerID, nil
}

// Heartbeat sends a heartbeat to the Hub.
func (c *Client) Heartbeat(status string) error {
	body := map[string]any{
		"status": status,
	}
	var resp HeartbeatResponse
	if err := c.post("/workers/heartbeat", body, &resp); err != nil {
		return fmt.Errorf("heartbeat: %w", err)
	}
	if !resp.Acknowledged {
		return fmt.Errorf("heartbeat not acknowledged")
	}
	return nil
}

// ClaimJob claims the next available job from the queue.
// Returns the job and lease ID, or nil if no job is available.
func (c *Client) ClaimJob(freeVRAM float64) (*Job, string, error) {
	body := map[string]any{
		"free_vram_gb": freeVRAM,
	}
	var resp ClaimResponse
	if err := c.post("/leases/acquire", body, &resp); err != nil {
		return nil, "", fmt.Errorf("claim job: %w", err)
	}
	if resp.JobID == "" {
		return nil, "", nil
	}
	return &resp.Job, resp.LeaseID, nil
}

// ClaimJobWithWait claims the next available job using long-polling.
// If waitSecs > 0, the server blocks up to waitSecs seconds waiting for a job.
// ctx cancellation aborts the request immediately.
func (c *Client) ClaimJobWithWait(ctx context.Context, freeVRAM float64, waitSecs int) (*Job, string, error) {
	if waitSecs <= 0 {
		return c.ClaimJob(freeVRAM)
	}

	body := map[string]any{
		"free_vram_gb":  freeVRAM,
		"wait_seconds": waitSecs,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, "", fmt.Errorf("marshal: %w", err)
	}

	// Use a context with timeout = waitSecs + 10s buffer, respecting caller's ctx.
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(waitSecs+10)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "POST", c.url("/leases/acquire"), strings.NewReader(string(data)))
	if err != nil {
		return nil, "", err
	}
	c.setHeaders(req)

	// Bypass httpClient.Timeout (which is 30s) by using the transport directly.
	// The reqCtx deadline provides the effective timeout.
	resp, err := c.httpClient.Transport.RoundTrip(req)
	if err != nil {
		if reqCtx.Err() != nil {
			return nil, "", nil // context cancelled — no job
		}
		return nil, "", fmt.Errorf("claim job (long-poll): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("claim job: %d %s", resp.StatusCode, string(respBody))
	}

	var claimResp ClaimResponse
	if err := json.NewDecoder(resp.Body).Decode(&claimResp); err != nil {
		return nil, "", fmt.Errorf("decode claim response: %w", err)
	}
	if claimResp.JobID == "" {
		return nil, "", nil
	}
	return &claimResp.Job, claimResp.LeaseID, nil
}

// RenewLease renews a job lease.
func (c *Client) RenewLease(leaseID string) (string, error) {
	body := map[string]any{
		"lease_id": leaseID,
	}
	var resp RenewLeaseResponse
	if err := c.post("/leases/renew", body, &resp); err != nil {
		return "", fmt.Errorf("renew lease: %w", err)
	}
	if !resp.Renewed {
		return "", fmt.Errorf("lease not renewed")
	}
	return resp.NewExpiresAt, nil
}
