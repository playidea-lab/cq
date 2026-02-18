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
