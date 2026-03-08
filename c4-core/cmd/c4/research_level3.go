package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/spf13/cobra"
)

func init() {
	parent := findOrCreateResearchCmd()

	specCmd := &cobra.Command{
		Use:   "spec <hyp-id>",
		Short: "Create an ExperimentSpec from a hypothesis",
		Args:  cobra.ExactArgs(1),
		RunE:  runResearchSpec,
	}

	checkpointCmd := &cobra.Command{
		Use:   "checkpoint <spec-id>",
		Short: "Review ExperimentSpec DoD with LLM-Optimizer + LLM-Skeptic",
		Args:  cobra.ExactArgs(1),
		RunE:  runResearchCheckpoint,
	}

	debateCmd := &cobra.Command{
		Use:   "debate <hyp-id>",
		Short: "Trigger multi-agent debate and create TypeDebate knowledge doc",
		Args:  cobra.ExactArgs(1),
		RunE:  runResearchDebate,
	}
	debateCmd.Flags().StringP("trigger", "t", "manual", "Trigger reason: dod_success|dod_null|escalation|manual")

	parent.AddCommand(specCmd)
	parent.AddCommand(checkpointCmd)
	parent.AddCommand(debateCmd)
}

// findOrCreateResearchCmd returns the existing 'research' subcommand of rootCmd,
// or creates and registers a new one.
func findOrCreateResearchCmd() *cobra.Command {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "research" || strings.HasPrefix(cmd.Use, "research ") {
			return cmd
		}
	}
	rc := &cobra.Command{
		Use:   "research",
		Short: "Research loop management (Level 3)",
	}
	rootCmd.AddCommand(rc)
	return rc
}

func runResearchSpec(cmd *cobra.Command, args []string) error {
	hypID := args[0]

	store, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer store.Close()

	// Validate hypothesis exists
	doc, err := store.Get(hypID)
	if err != nil {
		return fmt.Errorf("get hypothesis: %w", err)
	}
	if doc == nil {
		return fmt.Errorf("hypothesis not found: %s", hypID)
	}

	// Prompt for spec fields
	r := bufio.NewReader(os.Stdin)

	fmt.Print("success_condition: ")
	successCond, _ := r.ReadString('\n')
	successCond = strings.TrimSpace(successCond)

	fmt.Print("null_condition: ")
	nullCond, _ := r.ReadString('\n')
	nullCond = strings.TrimSpace(nullCond)

	fmt.Print("escalation_trigger (optional): ")
	escalation, _ := r.ReadString('\n')
	escalation = strings.TrimSpace(escalation)

	fmt.Print("controlled_variables (optional): ")
	controlled, _ := r.ReadString('\n')
	controlled = strings.TrimSpace(controlled)

	body := fmt.Sprintf("## ExperimentSpec\n\nhypothesis_id: %s\nsuccess_condition: %s\nnull_condition: %s\nescalation_trigger: %s\ncontrolled_variables: %s",
		hypID, successCond, nullCond, escalation, controlled)

	specID, err := store.Create(knowledge.TypeExperiment, map[string]any{
		"title":  "ExperimentSpec for " + hypID,
		"status": "pending",
	}, body)
	if err != nil {
		return fmt.Errorf("create spec: %w", err)
	}

	sanitize := func(s string) string { return strings.ReplaceAll(s, "\n", " ") }
	cqYAML := fmt.Sprintf("# cq.yaml — ExperimentSpec %s\nhypothesis_id: %s\nsuccess_condition: %s\nnull_condition: %s\n",
		specID, sanitize(hypID), sanitize(successCond), sanitize(nullCond))

	fmt.Printf("spec_id: %s\n\n%s\n", specID, cqYAML)
	return nil
}

func runResearchCheckpoint(cmd *cobra.Command, args []string) error {
	specID := args[0]

	fmt.Printf("Checkpoint for spec: %s\n", specID)
	fmt.Println("Run via MCP: c4_research_checkpoint({\"spec_id\": \"" + specID + "\"})")
	fmt.Println("Or use: cq tool c4_research_checkpoint --spec_id " + specID)

	r := bufio.NewReader(os.Stdin)
	fmt.Print("Approve? [y/n]: ")
	answer, _ := r.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer == "y" {
		fmt.Println("Approved, ready to submit: cq hub submit")
	} else {
		fmt.Println("Revision requested — edit ExperimentSpec and retry")
	}
	return nil
}

func runResearchDebate(cmd *cobra.Command, args []string) error {
	hypID := args[0]
	trigger, _ := cmd.Flags().GetString("trigger")

	fmt.Printf("Debate for hypothesis: %s (trigger: %s)\n", hypID, trigger)
	fmt.Println("Run via MCP: c4_research_debate({\"hypothesis_id\": \"" + hypID + "\", \"trigger_reason\": \"" + trigger + "\"})")
	fmt.Println("Or use: cq tool c4_research_debate --hypothesis_id " + hypID + " --trigger_reason " + trigger)
	return nil
}
