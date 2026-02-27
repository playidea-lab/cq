//go:build c5_hub

package hubhandler

import (
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp"
)

func registerHubInfraHandlers(reg *mcp.Registry, hubClient *hub.Client) {
	// c4_hub_workers — List workers
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_workers",
		Description: "List workers connected to the PiQ Hub with GPU status",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(_ json.RawMessage) (any, error) {
		return handleHubWorkers(hubClient)
	})

	// c4_hub_stats — Queue statistics
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_stats",
		Description: "Get Hub queue statistics (queued/running/succeeded/failed counts)",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(_ json.RawMessage) (any, error) {
		return handleHubStats(hubClient)
	})

	// c4_hub_upload — Upload artifact for a job
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_upload",
		Description: "Upload a local file as a Hub job artifact (presigned URL + SHA256 verification)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id":       map[string]any{"type": "string", "description": "Job ID"},
				"local_path":   map[string]any{"type": "string", "description": "Local file path to upload"},
				"storage_path": map[string]any{"type": "string", "description": "Storage path (e.g. outputs/model.pt)"},
			},
			"required": []string{"job_id", "local_path", "storage_path"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubUpload(hubClient, raw)
	})

	// c4_hub_download — Download artifact from a job
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_download",
		Description: "Download a Hub job artifact to a local file",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id":     map[string]any{"type": "string", "description": "Job ID"},
				"name":       map[string]any{"type": "string", "description": "Artifact name"},
				"local_path": map[string]any{"type": "string", "description": "Local destination path"},
			},
			"required": []string{"job_id", "name", "local_path"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubDownload(hubClient, raw)
	})
}

// =========================================================================
// Infra handler implementations
// =========================================================================

func handleHubWorkers(client *hub.Client) (any, error) {
	workers, err := client.ListWorkers()
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"workers": workers,
		"count":   len(workers),
	}, nil
}

func handleHubStats(client *hub.Client) (any, error) {
	stats, err := client.GetQueueStats()
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"queued":    stats.Queued,
		"running":   stats.Running,
		"succeeded": stats.Succeeded,
		"failed":    stats.Failed,
		"cancelled": stats.Cancelled,
		"total":     stats.Queued + stats.Running + stats.Succeeded + stats.Failed + stats.Cancelled,
	}, nil
}

func handleHubUpload(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		JobID       string `json:"job_id"`
		LocalPath   string `json:"local_path"`
		StoragePath string `json:"storage_path"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.JobID == "" || params.LocalPath == "" || params.StoragePath == "" {
		return nil, fmt.Errorf("job_id, local_path, and storage_path are required")
	}

	resp, err := client.UploadArtifact(params.JobID, params.StoragePath, params.LocalPath)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"uploaded":    true,
		"artifact_id": resp.ArtifactID,
		"job_id":      params.JobID,
		"path":        params.StoragePath,
	}, nil
}

func handleHubDownload(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		JobID     string `json:"job_id"`
		Name      string `json:"name"`
		LocalPath string `json:"local_path"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.JobID == "" || params.Name == "" || params.LocalPath == "" {
		return nil, fmt.Errorf("job_id, name, and local_path are required")
	}

	if err := client.DownloadArtifact(params.JobID, params.Name, params.LocalPath); err != nil {
		return nil, err
	}

	return map[string]any{
		"downloaded": true,
		"job_id":     params.JobID,
		"name":       params.Name,
		"local_path": params.LocalPath,
	}, nil
}
