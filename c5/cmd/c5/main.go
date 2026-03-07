// Package main is the entry point for the C5 distributed job queue.
//
// Usage:
//
//	c5 serve --port 8585 --db ./c5.db
//	c5 worker --server http://hub:8585 --gpu-count 2
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

// builtinServerURL is the default C5 Hub URL baked in at build time via ldflags:
//
//	-X main.builtinServerURL=https://your-hub.fly.dev
//
// If empty, falls back to C5_HUB_URL env var or http://localhost:8585.
var builtinServerURL = ""

// builtinAPIKey is the default API key baked in at build time via ldflags:
//
//	-X main.builtinAPIKey=your-api-key
//
// If empty, falls back to C5_API_KEY env var.
var builtinAPIKey = ""

func main() {
	root := &cobra.Command{
		Use:     "c5",
		Short:   "C5 — Distributed Job Queue Server",
		Version: version,
	}

	root.AddCommand(serveCmd())
	root.AddCommand(workerCmd())
	root.AddCommand(edgeAgentCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
