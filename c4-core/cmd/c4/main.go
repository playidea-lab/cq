// Package main is the entry point for the cq CLI tool.
//
// Usage:
//
//	cq status           # Show project state
//	cq run              # Start execution with workers
//	cq stop             # Stop execution
//	cq add-task         # Add a task interactively
//	cq mcp              # Start MCP server (stdio)
//
// Global flags:
//
//	--config FILE   Path to config file (default: .c4/config.yaml)
//	--verbose       Enable verbose output
//	--dir PATH      Project root directory (default: current directory)
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
