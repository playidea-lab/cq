package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the CQ service",
	Long:  `Stop the CQ background service (OS service or manual process).`,
	RunE:  runStopService,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func runStopService(cmd *cobra.Command, args []string) error {
	// Try PID file first (manual serve process).
	if err := runServeStop(cmd, args); err == nil {
		return nil
	}

	// Try OS service stop.
	if err := tryStopOSService(func() error { return stopOSService() }); err != nil {
		return err
	}

	fmt.Println("CQ service stopped.")
	return nil
}
