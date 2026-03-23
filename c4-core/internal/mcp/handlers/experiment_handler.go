package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/store"
)

// ExperimentStore defines the data access interface for experiment runs.
type ExperimentStore interface {
	// StartRun creates a new experiment run and returns its ID.
	// commitSHA and configHash are optional (pass empty strings if not available).
	StartRun(ctx context.Context, name, config, commitSHA, configHash string) (string, error)
	// RecordCheckpoint records a checkpoint metric and returns true if it's the best so far.
	RecordCheckpoint(ctx context.Context, runID string, metric float64, path string) (bool, error)
	// ShouldContinue returns true if the run has not been cancelled or completed.
	ShouldContinue(ctx context.Context, runID string) (bool, error)
	// CompleteRun marks the run as complete with a final metric and optional summary.
	CompleteRun(ctx context.Context, runID, status string, finalMetric float64, summary string) error
}

// validStatuses is the set of accepted completion statuses for CompleteRun.
var validStatuses = map[string]bool{"success": true, "failed": true, "cancelled": true}

// hubHTTPClient is used for all Hub API calls with a 30s timeout to prevent blocking MCP goroutines.
var hubHTTPClient = &http.Client{Timeout: 30 * time.Second}

// ExperimentHandlers holds dependencies for experiment MCP handlers.
type ExperimentHandlers struct {
	Store           ExperimentStore
	KnowledgeRecord func(ctx context.Context, title, content, domain string) error // nil if knowledge disabled
	HubBaseURL      string                                                          // "" → local store; non-empty → proxy to Hub API
	HubAPIKey       string                                                          // Hub authentication key
}

// hubPost sends a POST request to the Hub experiment API and decodes the JSON response into dest.
func (h *ExperimentHandlers) hubPost(ctx context.Context, path string, body, dest any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.HubBaseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.HubAPIKey != "" {
		req.Header.Set("X-API-Key", h.HubAPIKey)
	}
	resp, err := hubHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return errors.New(fmt.Sprintf("hub API %s: status %d: %s", path, resp.StatusCode, b))
	}
	if dest != nil {
		return json.Unmarshal(b, dest)
	}
	return nil
}

// hubGet sends a GET request to the Hub experiment API and decodes the JSON response into dest.
func (h *ExperimentHandlers) hubGet(ctx context.Context, path string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.HubBaseURL+path, nil)
	if err != nil {
		return err
	}
	if h.HubAPIKey != "" {
		req.Header.Set("X-API-Key", h.HubAPIKey)
	}
	resp, err := hubHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return errors.New(fmt.Sprintf("hub API %s: status %d: %s", path, resp.StatusCode, b))
	}
	return json.Unmarshal(b, dest)
}

// RegisterExperimentHandlers registers c4_experiment_register, c4_run_checkpoint,
// c4_run_complete, and c4_run_should_continue MCP tools.
// When h.HubBaseURL is non-empty the handlers proxy to the Hub API;
// otherwise h.Store (local SQLite) is used as fallback.
func RegisterExperimentHandlers(reg *mcp.Registry, h ExperimentHandlers) {
	if h.Store == nil && h.HubBaseURL == "" {
		return
	}

	reg.RegisterBlocking(mcp.ToolSchema{
		Name:        "c4_experiment_register",
		Description: "Register a new experiment run and return a run_id for checkpoint tracking.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":        map[string]any{"type": "string", "description": "Experiment name"},
				"config":      map[string]any{"type": "string", "description": "JSON config snapshot (optional)"},
				"commit_sha":  map[string]any{"type": "string", "description": "Git commit SHA at experiment start (optional)"},
				"config_hash": map[string]any{"type": "string", "description": "SHA256 hash of config file (optional)"},
			},
			"required": []string{"name"},
		},
	}, registerRunHandler(h))

	reg.RegisterBlocking(mcp.ToolSchema{
		Name:        "c4_run_checkpoint",
		Description: "Record an experiment checkpoint metric. Called automatically by C5 wrapper on epoch pattern match.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"run_id": map[string]any{"type": "string", "description": "Run ID from c4_experiment_register"},
				"metric": map[string]any{"type": "number", "description": "Checkpoint metric value (e.g. loss, MPJPE)"},
				"path":   map[string]any{"type": "string", "description": "Optional path to saved checkpoint file"},
			},
			"required": []string{"run_id", "metric"},
		},
	}, checkpointHandler(h))

	reg.RegisterBlocking(mcp.ToolSchema{
		Name:        "c4_run_complete",
		Description: "Mark an experiment run complete and auto-bridge results to knowledge store.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"run_id":       map[string]any{"type": "string", "description": "Run ID from c4_experiment_register"},
				"status":       map[string]any{"type": "string", "description": "Completion status: success, failed, cancelled"},
				"final_metric": map[string]any{"type": "number", "description": "Final metric value (e.g. loss, accuracy)"},
				"summary":      map[string]any{"type": "string", "description": "Human-readable summary for knowledge record"},
			},
			"required": []string{"run_id", "status"},
		},
	}, completeRunHandler(h))

	reg.RegisterBlocking(mcp.ToolSchema{
		Name:        "c4_run_should_continue",
		Description: "Check whether an experiment run should continue (not cancelled/completed).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"run_id": map[string]any{"type": "string", "description": "Run ID from c4_experiment_register"},
			},
			"required": []string{"run_id"},
		},
	}, shouldContinueHandler(h))
}

func registerRunHandler(h ExperimentHandlers) mcp.BlockingHandlerFunc {
	return func(ctx context.Context, rawArgs json.RawMessage) (any, error) {
		var args struct {
			Name       string `json:"name"`
			Config     string `json:"config"`
			CommitSHA  string `json:"commit_sha"`
			ConfigHash string `json:"config_hash"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return map[string]any{"error": "invalid arguments"}, nil
		}
		if args.Name == "" {
			return map[string]any{"error": "name is required"}, nil
		}

		// Auto-fill commit_sha from git if not provided
		var warnings []string
		if args.CommitSHA == "" {
			if out, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
				args.CommitSHA = strings.TrimSpace(string(out))
			}
		}

		// Auto-compute config_hash from config content if not provided
		if args.ConfigHash == "" && args.Config != "" {
			h := sha256.Sum256([]byte(args.Config))
			args.ConfigHash = hex.EncodeToString(h[:])[:16]
		}

		// Git dirty check — warn but don't block
		if out, err := exec.Command("git", "status", "--porcelain").Output(); err == nil {
			if len(strings.TrimSpace(string(out))) > 0 {
				warnings = append(warnings, "git working tree is dirty — experiment may not be reproducible. Consider committing first.")
			}
		}

		var runID string
		if h.HubBaseURL != "" {
			var resp struct {
				RunID string `json:"run_id"`
			}
			if err := h.hubPost(ctx, "/v1/experiment/run", map[string]any{
				"name":        args.Name,
				"capability":  args.Config,
				"commit_sha":  args.CommitSHA,
				"config_hash": args.ConfigHash,
			}, &resp); err != nil {
				return map[string]any{"error": fmt.Sprintf("RegisterRun failed: %v", err)}, nil
			}
			runID = resp.RunID
		} else {
			var err error
			runID, err = h.Store.StartRun(ctx, args.Name, args.Config, args.CommitSHA, args.ConfigHash)
			if err != nil {
				return map[string]any{"error": fmt.Sprintf("RegisterRun failed: %v", err)}, nil
			}
		}
		result := map[string]any{
			"success":    true,
			"run_id":     runID,
			"registered": time.Now().UTC().Format(time.RFC3339),
		}
		if args.CommitSHA != "" {
			result["commit_sha"] = args.CommitSHA
		}
		if args.ConfigHash != "" {
			result["config_hash"] = args.ConfigHash
		}
		if len(warnings) > 0 {
			result["warnings"] = warnings
		}
		return result, nil
	}
}

func checkpointHandler(h ExperimentHandlers) mcp.BlockingHandlerFunc {
	return func(ctx context.Context, rawArgs json.RawMessage) (any, error) {
		var args struct {
			RunID  string  `json:"run_id"`
			Metric float64 `json:"metric"`
			Path   string  `json:"path"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return map[string]any{"error": "invalid arguments"}, nil
		}
		if args.RunID == "" {
			return map[string]any{"error": "run_id is required"}, nil
		}

		var isBest bool
		if h.HubBaseURL != "" {
			var resp struct {
				IsBest bool `json:"is_best"`
			}
			if err := h.hubPost(ctx, "/v1/experiment/checkpoint", map[string]any{"run_id": args.RunID, "metric": args.Metric, "path": args.Path}, &resp); err != nil {
				return map[string]any{"error": fmt.Sprintf("RecordCheckpoint failed: %v", err)}, nil
			}
			isBest = resp.IsBest
		} else {
			var err error
			isBest, err = h.Store.RecordCheckpoint(ctx, args.RunID, args.Metric, args.Path)
			if err != nil {
				if errors.Is(err, store.ErrRunNotFound) {
					return map[string]any{"error": "run not found", "not_found": true}, nil
				}
				return map[string]any{"error": fmt.Sprintf("RecordCheckpoint failed: %v", err)}, nil
			}
		}
		return map[string]any{
			"success": true,
			"run_id":  args.RunID,
			"is_best": isBest,
		}, nil
	}
}

func completeRunHandler(h ExperimentHandlers) mcp.BlockingHandlerFunc {
	return func(ctx context.Context, rawArgs json.RawMessage) (any, error) {
		var args struct {
			RunID       string  `json:"run_id"`
			Status      string  `json:"status"`
			FinalMetric float64 `json:"final_metric"`
			Summary     string  `json:"summary"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return map[string]any{"error": "invalid arguments"}, nil
		}
		if args.RunID == "" {
			return map[string]any{"error": "run_id is required"}, nil
		}
		if args.Status == "" {
			return map[string]any{"error": "status is required"}, nil
		}
		if !validStatuses[args.Status] {
			return map[string]any{"error": fmt.Sprintf("invalid status %q: must be success, failed, or cancelled", args.Status)}, nil
		}

		if h.HubBaseURL != "" {
			if err := h.hubPost(ctx, "/v1/experiment/complete", map[string]any{
				"run_id":       args.RunID,
				"status":       args.Status,
				"final_metric": args.FinalMetric,
				"summary":      args.Summary,
			}, nil); err != nil {
				return map[string]any{"error": fmt.Sprintf("CompleteRun failed: %v", err)}, nil
			}
		} else if err := h.Store.CompleteRun(ctx, args.RunID, args.Status, args.FinalMetric, args.Summary); err != nil {
			return map[string]any{"error": fmt.Sprintf("CompleteRun failed: %v", err)}, nil
		}

		// Auto-bridge: record results to knowledge store asynchronously.
		// Use context.WithoutCancel so the goroutine outlives the MCP request,
		// with a 30s timeout to prevent goroutine leaks if KnowledgeRecord hangs.
		if h.KnowledgeRecord != nil {
			title := fmt.Sprintf("Experiment %s: %s", args.RunID, args.Status)
			content := args.Summary
			if content == "" {
				content = fmt.Sprintf("run_id=%s status=%s final_metric=%g",
					args.RunID, args.Status, args.FinalMetric)
			}
			go func() {
				ctx2, cancel2 := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
				defer cancel2()
				if err := h.KnowledgeRecord(ctx2, title, content, "experiment"); err != nil {
					fmt.Fprintf(os.Stderr, "c4: experiment auto-bridge failed: %v\n", err)
				}
			}()
		}

		return map[string]any{
			"success": true,
			"run_id":  args.RunID,
			"status":  args.Status,
		}, nil
	}
}

func shouldContinueHandler(h ExperimentHandlers) mcp.BlockingHandlerFunc {
	return func(ctx context.Context, rawArgs json.RawMessage) (any, error) {
		var args struct {
			RunID string `json:"run_id"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return map[string]any{"error": "invalid arguments"}, nil
		}
		if args.RunID == "" {
			return map[string]any{"error": "run_id is required"}, nil
		}

		var ok bool
		if h.HubBaseURL != "" {
			var resp struct {
				ShouldContinue bool `json:"should_continue"`
			}
			q := url.Values{"run_id": {args.RunID}}
			if err := h.hubGet(ctx, "/v1/experiment/continue?"+q.Encode(), &resp); err != nil {
				return map[string]any{"error": fmt.Sprintf("ShouldContinue failed: %v", err)}, nil
			}
			ok = resp.ShouldContinue
		} else {
			var err error
			ok, err = h.Store.ShouldContinue(ctx, args.RunID)
			if err != nil {
				if errors.Is(err, store.ErrRunNotFound) {
					return map[string]any{"error": "run not found", "not_found": true}, nil
				}
				return map[string]any{"error": fmt.Sprintf("ShouldContinue failed: %v", err)}, nil
			}
		}
		return map[string]any{
			"run_id":          args.RunID,
			"should_continue": ok,
		}, nil
	}
}
