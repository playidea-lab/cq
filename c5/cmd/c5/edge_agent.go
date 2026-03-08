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
		hubURL          string
		apiKey          string
		edgeName        string
		workdir         string
		pollSec         int
		metricsCommand  string
		metricsInterval int
		healthCheckTimeout int
		driveURL        string
		driveAPIKey     string
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
				HubURL:             hubURL,
				APIKey:             apiKey,
				EdgeName:           edgeName,
				Workdir:            workdir,
				PollInterval:       time.Duration(pollSec) * time.Second,
				MetricsCommand:     metricsCommand,
				MetricsInterval:    time.Duration(metricsInterval) * time.Second,
				HealthCheckTimeout: time.Duration(healthCheckTimeout) * time.Second,
				DriveURL:           driveURL,
				DriveAPIKey:        driveAPIKey,
			})
		},
	}

	defaultHubURL := builtinServerURL
	if defaultHubURL == "" {
		defaultHubURL = os.Getenv("C5_HUB_URL")
	}
	cmd.Flags().StringVar(&hubURL, "hub-url", defaultHubURL, "C5 Hub URL (default: builtin > C5_HUB_URL env > http://localhost:8585)")
	defaultEdgeAPIKey := builtinAPIKey
	if defaultEdgeAPIKey == "" {
		defaultEdgeAPIKey = os.Getenv("C5_API_KEY")
	}
	cmd.Flags().StringVar(&apiKey, "api-key", defaultEdgeAPIKey, "API key for authentication")
	cmd.Flags().StringVar(&edgeName, "edge-name", "", "Edge name (default: hostname)")
	cmd.Flags().StringVar(&workdir, "workdir", ".", "Directory to download artifacts into")
	cmd.Flags().IntVar(&pollSec, "poll-interval", 10, "Poll interval in seconds for assignments")
	cmd.Flags().StringVar(&metricsCommand, "metrics-command", "", "Shell command to collect metrics (stdout KEY=VALUE)")
	cmd.Flags().IntVar(&metricsInterval, "metrics-interval", 60, "Metrics reporting interval in seconds")
	cmd.Flags().IntVar(&healthCheckTimeout, "health-check-timeout", 30, "Health check timeout in seconds")
	cmd.Flags().StringVar(&driveURL, "drive-url", os.Getenv("C5_DRIVE_URL"), "Drive server URL for collect action uploads")
	cmd.Flags().StringVar(&driveAPIKey, "drive-api-key", os.Getenv("C5_DRIVE_API_KEY"), "Drive API key for authentication")

	return cmd
}
