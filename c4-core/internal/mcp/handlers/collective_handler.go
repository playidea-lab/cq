package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/changmin/c4-core/internal/mcp"
	"github.com/changmin/c4-core/internal/ontology"
)

// CollectiveOpts holds optional config for collective ontology handlers.
type CollectiveOpts struct {
	CloudURL    string
	CloudKey    string
	TokenFn     func() string
	Domain      string
	ProjectRoot string
}

// RegisterCollectiveHandlers registers c4_collective_sync and c4_collective_stats.
func RegisterCollectiveHandlers(reg *mcp.Registry, opts CollectiveOpts) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_collective_sync",
		Description: "Upload local project ontology to Hub and/or download collective patterns",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"direction": map[string]any{
					"type":        "string",
					"enum":        []string{"upload", "download", "both"},
					"description": "Sync direction (default: both)",
				},
			},
		},
	}, collectiveSyncHandler(opts))

	reg.Register(mcp.ToolSchema{
		Name:        "c4_collective_stats",
		Description: "Show collective ontology statistics (local + Hub)",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, collectiveStatsHandler(opts))
}

func collectiveSyncHandler(opts CollectiveOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		var args struct {
			Direction string `json:"direction"`
		}
		if len(rawArgs) > 0 {
			json.Unmarshal(rawArgs, &args)
		}
		if args.Direction == "" {
			args.Direction = "both"
		}

		result := map[string]any{"direction": args.Direction}

		if opts.CloudURL == "" || opts.CloudKey == "" {
			return map[string]any{"error": "Hub not configured (cloud.url/cloud.anon_key missing)"}, nil
		}

		// Upload
		if args.Direction == "upload" || args.Direction == "both" {
			proj, err := ontology.LoadProject(opts.ProjectRoot)
			if err != nil {
				slog.Warn("collective_sync: LoadProject failed", "error", err)
			} else {
				patterns := ontology.Anonymize(proj, opts.Domain)
				if len(patterns) > 0 {
					uploader := ontology.NewHubUploader(opts.CloudURL, opts.CloudKey, opts.TokenFn, opts.ProjectRoot)
					n, err := uploader.Upload(patterns)
					if err != nil {
						result["upload_error"] = err.Error()
					}
					result["uploaded"] = n
				} else {
					result["uploaded"] = 0
					result["upload_note"] = fmt.Sprintf("no patterns above threshold (%d)", ontology.AnonymizeThreshold)
				}
			}
		}

		// Download
		if args.Direction == "download" || args.Direction == "both" {
			downloader := ontology.NewHubDownloader(opts.CloudURL, opts.CloudKey, opts.TokenFn)
			username := os.Getenv("USER")
			n, err := downloader.SeedFromHub(username, opts.Domain)
			if err != nil {
				result["download_error"] = err.Error()
			}
			result["downloaded"] = n
		}

		return result, nil
	}
}

func collectiveStatsHandler(opts CollectiveOpts) mcp.HandlerFunc {
	return func(rawArgs json.RawMessage) (any, error) {
		stats := map[string]any{
			"domain":      opts.Domain,
			"hub_url":     opts.CloudURL,
			"hub_enabled": opts.CloudURL != "",
		}

		// Local project ontology stats
		proj, err := ontology.LoadProject(opts.ProjectRoot)
		if err == nil && proj != nil {
			stats["project_nodes"] = len(proj.Schema.Nodes)
		} else {
			stats["project_nodes"] = 0
		}

		// Anonymizable count
		if proj != nil {
			patterns := ontology.Anonymize(proj, opts.Domain)
			stats["uploadable_patterns"] = len(patterns)
		}

		return stats, nil
	}
}
