//go:build hub

package hub

import "fmt"

// UploadCrashLog uploads a worker crash log to Supabase hub_worker_logs table.
// workerID identifies the worker; content is the log text (e.g. from RingBuffer.String()).
// Returns an error if the upload fails or the client is not configured.
func (c *Client) UploadCrashLog(workerID, content string) error {
	if !c.IsAvailable() {
		return fmt.Errorf("hub client not configured")
	}
	if workerID == "" {
		return fmt.Errorf("workerID must not be empty")
	}
	row := map[string]any{
		"worker_id": workerID,
		"content":   content,
	}
	return c.supabasePost("/rest/v1/hub_worker_logs", row, nil)
}
