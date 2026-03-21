package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/piqsol/c4/c5/internal/model"
	"github.com/piqsol/c4/c5/internal/store"
)

// pushDispatcher tries to push jobs directly to MCP-capable workers.
type pushDispatcher struct {
	store      *store.Store
	httpClient *http.Client
}

// newPushDispatcher creates a dispatcher with a 5-second HTTP timeout.
func newPushDispatcher(st *store.Store) *pushDispatcher {
	return &pushDispatcher{
		store: st,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// pushMCPCallParams is the params payload for a JSON-RPC tools/call request sent to a worker.
type pushMCPCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// TryPushDispatch finds an eligible online worker with an mcp_url and pushes the job to it.
// Returns nil on success (push delivered), or an error. If no eligible worker is found, returns nil.
func (d *pushDispatcher) TryPushDispatch(job *model.Job) error {
	worker, err := d.store.GetWorkerForPushDispatch(job)
	if err != nil {
		return fmt.Errorf("find push worker: %w", err)
	}
	if worker == nil {
		return nil // no push-capable worker; fall back to pull
	}

	lease, claimedJob, err := d.store.AcquireLeaseForWorker(job.ID, worker.ID)
	if err != nil {
		return fmt.Errorf("acquire lease for push: %w", err)
	}
	if lease == nil {
		return nil // already claimed by another path
	}

	if err := d.pushToWorker(worker.MCPURL, claimedJob, lease); err != nil {
		log.Printf("c5: push dispatch to %s failed (%v), requeueing job %s", worker.MCPURL, err, job.ID)
		if rbErr := d.store.ReleaseLeaseAndRequeue(lease.ID); rbErr != nil {
			log.Printf("c5: requeue after push failure: %v", rbErr)
		}
		return err
	}

	log.Printf("c5: push dispatch: job %s → worker %s (%s)", job.ID, worker.ID, worker.MCPURL)
	return nil
}

// pushToWorker sends a JSON-RPC tools/call hub_dispatch_job to the worker's MCP URL.
func (d *pushDispatcher) pushToWorker(mcpURL string, job *model.Job, lease *model.Lease) error {
	callParams := pushMCPCallParams{
		Name: "hub_dispatch_job",
		Arguments: map[string]any{
			"job_id":   job.ID,
			"lease_id": lease.ID,
			"job":      job,
		},
	}
	paramsBytes, err := json.Marshal(callParams)
	if err != nil {
		return fmt.Errorf("marshal params: %w", err)
	}

	payload := mcpRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  paramsBytes,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	resp, err := d.httpClient.Post(mcpURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("worker returned HTTP %d", resp.StatusCode)
	}
	return nil
}
