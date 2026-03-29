//go:build hub

package hubhandler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp"
)

const (
	relayHealthTimeout = 10 * time.Second
)

func registerHubWorkersUnified(reg *mcp.Registry, hubClient *hub.Client) {
	reg.Register(mcp.ToolSchema{
		Name:        "cq_hub_workers_unified",
		Description: "List all workers by merging hub_workers table with relay /health. Status is relay-based: online (WSS alive) or offline.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"include_offline": map[string]any{
					"type":        "boolean",
					"description": "Include offline workers in results (default: true)",
				},
				"relay_url": map[string]any{
					"type":        "string",
					"description": "Relay server base URL (e.g. https://cq-relay.fly.dev). If omitted, relay merge is skipped.",
				},
			},
		},
	}, func(raw json.RawMessage) (any, error) {
		var params struct {
			IncludeOffline *bool  `json:"include_offline"`
			RelayURL       string `json:"relay_url"`
		}
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, err
		}
		includeOffline := true
		if params.IncludeOffline != nil {
			includeOffline = *params.IncludeOffline
		}
		return handleHubWorkersUnified(hubClient, params.RelayURL, includeOffline)
	})
}

// handleHubWorkersUnified merges hub_workers table with relay /health.
func handleHubWorkersUnified(client *hub.Client, relayURL string, includeOffline bool) (any, error) {
	// Step 1: Fetch hub workers (all, including offline for heartbeat-based status).
	hubWorkers, hubErr := client.ListWorkers(false) // false = include offline

	if hubErr != nil {
		return map[string]any{
			"workers": []any{},
			"summary": map[string]any{"total": 0, "online": 0, "idle": 0, "offline": 0},
			"error":   hubErr.Error(),
		}, nil
	}

	// Step 2: Fetch relay worker names (best-effort; failure = empty set).
	relayNames := fetchRelayWorkerNames(relayURL)

	// Step 3: Build unified worker list.
	// Status is relay-based: online (in relay /health) or offline.
	result := make([]any, 0, len(hubWorkers))

	var countOnline, countOffline int

	for _, w := range hubWorkers {
		// Relay presence = online, absence = offline.
		key := w.Hostname
		if key == "" {
			key = w.Name
		}
		if key == "" {
			key = w.ID
		}
		_, inRelay := relayNames[key]
		status := "offline"
		if inRelay {
			status = "online"
		}

		// Filter offline if requested.
		if !includeOffline && status == "offline" {
			continue
		}

		entry := map[string]any{
			"id":           w.ID,
			"name":         nameOrID(w),
			"hostname":     w.Hostname,
			"status":       status,
			"gpu_model":    w.GPUModel,
			"current_task": nil,
			"last_seen":    w.LastHeartbeat,
			"connected_at": w.RegisteredAt,
		}

		// Try to fetch current running job (best-effort).
		if jobID, err := client.FetchRunningJobForWorker(w.ID); err == nil && jobID != "" {
			entry["current_task"] = jobID
		}

		result = append(result, entry)

		if status == "online" {
			countOnline++
		} else {
			countOffline++
		}
	}

	return map[string]any{
		"workers": result,
		"summary": map[string]any{
			"total":   len(result),
			"online":  countOnline,
			"offline": countOffline,
		},
	}, nil
}

// fetchRelayWorkerNames calls GET <relayURL>/health and returns the set of connected worker names.
// On any error (relay offline, bad response) it returns an empty set.
func fetchRelayWorkerNames(relayURL string) map[string]struct{} {
	if relayURL == "" {
		return map[string]struct{}{}
	}

	base := strings.TrimRight(relayURL, "/")
	base = strings.Replace(base, "wss://", "https://", 1)
	base = strings.Replace(base, "ws://", "http://", 1)

	ctx, cancel := context.WithTimeout(context.Background(), relayHealthTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", base+"/health", nil)
	if err != nil {
		return map[string]struct{}{}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return map[string]struct{}{}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	var health struct {
		WorkerNames []string `json:"worker_names"`
	}
	if err := json.Unmarshal(body, &health); err != nil {
		return map[string]struct{}{}
	}

	names := make(map[string]struct{}, len(health.WorkerNames))
	for _, n := range health.WorkerNames {
		names[n] = struct{}{}
	}
	return names
}

// nameOrID returns the worker's display name, falling back to ID.
func nameOrID(w hub.Worker) string {
	if w.Name != "" {
		return w.Name
	}
	return w.ID
}
