package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/changmin/c4-core/internal/pop"
	"github.com/spf13/cobra"
)

// popCmd is the root command for POP (Proactive Output Pipeline) operations.
var popCmd = &cobra.Command{
	Use:   "pop",
	Short: "POP (Proactive Output Pipeline) management",
	Long:  "Commands for managing the Proactive Output Pipeline: status, reflect.",
}

// popStatusCmd prints the current POP pipeline status.
var popStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show POP pipeline status",
	Long:  "Display POP pipeline gauge values, last extraction time, and knowledge proposal summary.",
	RunE:  runPopStatus,
}

func init() {
	popCmd.AddCommand(popStatusCmd)
	rootCmd.AddCommand(popCmd)
}

// popState mirrors the on-disk POP state.json schema.
type popStateFile struct {
	LastExtractedAt    time.Time `json:"last_extracted_at"`
	LastCrystallizedAt time.Time `json:"last_crystallized_at"`
}

// gaugeEntry mirrors a single entry in gauge.json.
type gaugeFileData struct {
	Gauges []struct {
		Name      string  `json:"name"`
		Value     float64 `json:"value"`
		UpdatedAt string  `json:"updated_at"`
	} `json:"gauges"`
}

func runPopStatus(cmd *cobra.Command, args []string) error {
	statePath := pop.DefaultStatePath(projectDir)
	gaugePath := pop.DefaultGaugePath(projectDir)

	// Load POP state (best-effort)
	var state popStateFile
	if raw, err := os.ReadFile(statePath); err == nil {
		_ = json.Unmarshal(raw, &state)
	}

	// Load gauge data (best-effort)
	var gaugeData gaugeFileData
	if raw, err := os.ReadFile(gaugePath); err == nil {
		_ = json.Unmarshal(raw, &gaugeData)
	}

	// Display status
	fmt.Println("POP Pipeline Status")
	fmt.Println("===================")

	if state.LastExtractedAt.IsZero() {
		fmt.Println("Last extraction: (never)")
	} else {
		fmt.Printf("Last extraction:    %s\n", state.LastExtractedAt.Local().Format(time.RFC3339))
	}
	if state.LastCrystallizedAt.IsZero() {
		fmt.Println("Last crystallized: (never)")
	} else {
		fmt.Printf("Last crystallized: %s\n", state.LastCrystallizedAt.Local().Format(time.RFC3339))
	}

	fmt.Println()
	fmt.Println("Gauge Values")
	fmt.Println("------------")

	if len(gaugeData.Gauges) == 0 {
		fmt.Println("  (no gauge data)")
	} else {
		for _, g := range gaugeData.Gauges {
			fmt.Printf("  %-20s %.4f  (updated: %s)\n", g.Name, g.Value, g.UpdatedAt)
		}
	}

	fmt.Println()
	fmt.Println("Proposal Status: use c4_pop_status MCP tool for full knowledge stats")

	return nil
}
