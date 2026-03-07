package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/piqsol/c4/c5/internal/edgeagent"
	"github.com/spf13/cobra"
)

func edgeAgentCmd() *cobra.Command {
	var (
		hubURL   string
		apiKey   string
		edgeName string
		workdir  string
		pollSec  int
	)

	cmd := &cobra.Command{
		Use:   "edge-agent",
		Short: "Run C5 edge agent (register, heartbeat, poll assignments, download artifacts, run post_command, report status)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if hubURL == "" {
				hubURL = "http://localhost:8585"
			}
			if edgeName == "" {
				h, _ := os.Hostname()
				edgeName = h
			}
			if workdir == "" {
				workdir = "."
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return edgeagent.Run(ctx, edgeagent.Config{
				HubURL:       hubURL,
				APIKey:       apiKey,
				EdgeName:     edgeName,
				Workdir:      workdir,
				PollInterval: time.Duration(pollSec) * time.Second,
			})
		},
	}

	defaultHubURL := builtinServerURL
	if defaultHubURL == "" {
		defaultHubURL = os.Getenv("C5_HUB_URL")
	}
	cmd.Flags().StringVar(&hubURL, "hub-url", defaultHubURL, "C5 Hub URL (default: builtin > C5_HUB_URL env > http://localhost:8585)")
	cmd.Flags().StringVar(&apiKey, "api-key", os.Getenv("C5_API_KEY"), "API key for authentication")
	cmd.Flags().StringVar(&edgeName, "edge-name", "", "Edge name (default: hostname)")
	cmd.Flags().StringVar(&workdir, "workdir", ".", "Directory to download artifacts into")
	cmd.Flags().IntVar(&pollSec, "poll-interval", 10, "Poll interval in seconds for assignments")

	return cmd
}
