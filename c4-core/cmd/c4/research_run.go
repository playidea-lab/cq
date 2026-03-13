//go:build research

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	parent := findOrCreateResearchCmd()

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Hub worker entrypoint: execute experiment from spec",
		RunE:  runResearchRun,
	}
	parent.AddCommand(runCmd)
}

func runResearchRun(cmd *cobra.Command, args []string) error {
	hypothesisID := os.Getenv("C4_HYPOTHESIS_ID")
	specID := os.Getenv("C4_EXPERIMENT_SPEC_ID")

	if hypothesisID == "" {
		fmt.Fprintln(os.Stderr, "error: C4_HYPOTHESIS_ID env var is required")
		os.Exit(1)
	}
	if specID == "" {
		fmt.Fprintln(os.Stderr, "error: C4_EXPERIMENT_SPEC_ID env var is required")
		os.Exit(1)
	}

	store, err := openKnowledgeStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: open knowledge store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	doc, err := store.Get(specID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: fetch spec %s: %v\n", specID, err)
		os.Exit(1)
	}
	if doc == nil {
		fmt.Fprintf(os.Stderr, "error: spec %s not found\n", specID)
		os.Exit(1)
	}

	var spec ExperimentSpec
	if err := json.Unmarshal([]byte(doc.Body), &spec); err != nil {
		fmt.Fprintf(os.Stderr, "error: parse spec JSON: %v\n", err)
		os.Exit(1)
	}

	switch spec.Type {
	case "ml_training":
		fmt.Fprintln(os.Stderr, "unsupported experiment type: ml_training (v1)")
		os.Exit(1)
	case "code_validation":
		return runCodeValidation()
	default:
		fmt.Fprintf(os.Stderr, "error: unknown experiment type: %s\n", spec.Type)
		os.Exit(1)
	}
	return nil
}

// goTestJSONLine is the subset of go test -json output we care about.
type goTestJSONLine struct {
	Action string `json:"Action"`
	Test   string `json:"Test"`
}

func runCodeValidation() error {
	workDir := os.Getenv("C4_PROJECT_DIR")
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: getwd: %v\n", err)
			os.Exit(1)
		}
	}

	goCmd := exec.Command("go", "test", "-timeout", "10m", "-json", "./...")
	goCmd.Dir = workDir

	out, _ := goCmd.Output() // ignore exit error; parse output for pass/fail counts

	passCount, failCount := parseGoTestJSON(out)

	total := passCount + failCount
	var passRate float64
	if total > 0 {
		passRate = float64(passCount) / float64(total)
	}

	status := "failed"
	if passRate >= 1.0 && total > 0 {
		status = "completed"
	}

	fmt.Printf("@VAL_LOSS=\n")
	fmt.Printf("@TEST_METRIC=%.3f\n", passRate)
	fmt.Printf("@STATUS=%s\n", status)
	return nil
}

func parseGoTestJSON(data []byte) (passCount, failCount int) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry goTestJSONLine
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Test == "" {
			continue
		}
		switch entry.Action {
		case "pass":
			passCount++
		case "fail":
			failCount++
		}
	}
	return
}
