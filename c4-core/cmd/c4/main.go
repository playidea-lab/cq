// Package main is the entry point for the c4 CLI tool.
//
// Usage:
//
//	c4 status           # Show project state
//	c4 run              # Start execution with workers
//	c4 stop             # Stop execution
//	c4 add-task         # Add a task interactively
//	c4 mcp              # Start MCP server (stdio)
//
// Global flags:
//
//	--config FILE   Path to config file (default: .c4/config.yaml)
//	--verbose       Enable verbose output
//	--dir PATH      Project root directory (default: current directory)
package main

import (
	"os"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
