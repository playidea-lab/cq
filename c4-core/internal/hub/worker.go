package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// RegisterWorker registers this machine as a Hub worker via Supabase RPC.
// The returned worker ID is stored on the client for subsequent calls.
func (c *Client) RegisterWorker(capabilities map[string]any) (string, error) {
	// Build capabilities as []string from the map keys.
	caps := make([]string, 0, len(capabilities))
	for k := range capabilities {
		caps = append(caps, k)
	}

	hostname, _ := os.Hostname()
	workerID := c.workerID
	if workerID == "" {
		workerID = hostname
	}

	body := map[string]any{
		"p_worker_id":    workerID,
		"p_hostname":     hostname,
		"p_capabilities": caps,
		"p_mcp_url":      "",
		"p_project_id":   c.teamID,
	}
	var result map[string]any
	if err := c.supabaseRPC("register_worker", body, &result); err != nil {
		return "", fmt.Errorf("register worker: %w", err)
	}
	// Store capabilities for use in subsequent ClaimJob calls.
	c.capabilities = caps

	// RPC returns the hub_workers row; use our workerID as authoritative ID.
	if id, ok := result["id"].(string); ok && id != "" {
		c.workerID = id
		return id, nil
	}
	c.workerID = workerID
	return workerID, nil
}

// Heartbeat sends a heartbeat to Supabase by patching the hub_workers row.
func (c *Client) Heartbeat(status string) error {
	if c.workerID == "" {
		return fmt.Errorf("heartbeat: worker not registered (no worker ID)")
	}
	body := map[string]any{
		"status":         status,
		"last_heartbeat": time.Now().UTC().Format(time.RFC3339),
	}
	path := "/rest/v1/hub_workers?id=eq." + c.workerID
	if err := c.supabasePatch(path, body, nil); err != nil {
		return fmt.Errorf("heartbeat: %w", err)
	}
	return nil
}

// ClaimJob claims the next available job from Supabase via RPC.
// Returns the job and lease ID, or nil if no job is available.
func (c *Client) ClaimJob(freeVRAM float64) (*Job, string, error) {
	caps := c.capabilities
	if caps == nil {
		caps = []string{}
	}
	body := map[string]any{
		"p_worker_id":    c.workerID,
		"p_capabilities": caps,
		"p_project_id":   c.teamID,
	}
	// RPC returns jsonb {job: {...}, lease_id: TEXT} or NULL.
	var result map[string]any
	if err := c.supabaseRPC("claim_job", body, &result); err != nil {
		return nil, "", fmt.Errorf("claim job: %w", err)
	}
	if result == nil {
		return nil, "", nil // no job available
	}
	leaseID, _ := result["lease_id"].(string)
	if leaseID == "" {
		return nil, "", nil
	}
	// Unmarshal job from nested map.
	jobData, err := json.Marshal(result["job"])
	if err != nil {
		return nil, "", fmt.Errorf("claim job: marshal job: %w", err)
	}
	var job Job
	if err := json.Unmarshal(jobData, &job); err != nil {
		return nil, "", fmt.Errorf("claim job: decode job: %w", err)
	}
	return &job, leaseID, nil
}

// ClaimJobWithWait claims the next available job.
// waitSecs is ignored for Supabase (no server-side long-polling); caller should
// implement polling at the application level. ctx cancellation aborts the request.
func (c *Client) ClaimJobWithWait(ctx context.Context, freeVRAM float64, waitSecs int) (*Job, string, error) {
	if waitSecs <= 0 {
		return c.ClaimJob(freeVRAM)
	}

	caps := c.capabilities
	if caps == nil {
		caps = []string{}
	}
	body := map[string]any{
		"p_worker_id":    c.workerID,
		"p_capabilities": caps,
		"p_project_id":   c.teamID,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, "", fmt.Errorf("marshal: %w", err)
	}

	rpcURL := c.supabaseRestURL("/rest/v1/rpc/claim_job")
	req, err := http.NewRequestWithContext(ctx, "POST", rpcURL, strings.NewReader(string(data)))
	if err != nil {
		return nil, "", err
	}
	c.setSupabaseHeaders(req)
	req.Header.Set("Prefer", "return=representation")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", nil // context cancelled — no job
		}
		return nil, "", fmt.Errorf("claim job (long-poll): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("claim job: %d %s", resp.StatusCode, string(respBody))
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, "", fmt.Errorf("decode claim response: %w", err)
	}
	if result == nil {
		return nil, "", nil
	}
	leaseID, _ := result["lease_id"].(string)
	if leaseID == "" {
		return nil, "", nil
	}
	jobData, err := json.Marshal(result["job"])
	if err != nil {
		return nil, "", fmt.Errorf("claim job: marshal job: %w", err)
	}
	var job Job
	if err := json.Unmarshal(jobData, &job); err != nil {
		return nil, "", fmt.Errorf("claim job: decode job: %w", err)
	}
	return &job, leaseID, nil
}

// RenewLease renews a job lease via Supabase RPC.
func (c *Client) RenewLease(leaseID string) (string, error) {
	body := map[string]any{
		"p_lease_id":     leaseID,
		"p_duration_sec": 300, // 5 minutes
	}
	if err := c.supabaseRPC("renew_lease", body, nil); err != nil {
		return "", fmt.Errorf("renew lease: %w", err)
	}
	// renew_lease RPC returns void; compute new expiry locally.
	newExpiresAt := time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339)
	return newExpiresAt, nil
}
