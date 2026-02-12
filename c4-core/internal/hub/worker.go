package hub

import "fmt"

// RegisterWorker registers this machine as a Hub worker.
// The returned worker ID is stored on the client for subsequent calls.
func (c *Client) RegisterWorker(capabilities map[string]any) (string, error) {
	body := map[string]any{
		"capabilities": capabilities,
	}
	var resp WorkerRegisterResponse
	if err := c.post("/v1/workers/register", body, &resp); err != nil {
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
	if err := c.post("/v1/workers/heartbeat", body, &resp); err != nil {
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
	if err := c.post("/v1/leases/acquire", body, &resp); err != nil {
		return nil, "", fmt.Errorf("claim job: %w", err)
	}
	if resp.JobID == "" {
		return nil, "", nil
	}
	return &resp.Job, resp.LeaseID, nil
}

// RenewLease renews a job lease.
func (c *Client) RenewLease(leaseID string) (string, error) {
	body := map[string]any{
		"lease_id": leaseID,
	}
	var resp RenewLeaseResponse
	if err := c.post("/v1/leases/renew", body, &resp); err != nil {
		return "", fmt.Errorf("renew lease: %w", err)
	}
	if !resp.Renewed {
		return "", fmt.Errorf("lease not renewed")
	}
	return resp.NewExpiresAt, nil
}
