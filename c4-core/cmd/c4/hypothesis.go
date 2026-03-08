package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/spf13/cobra"
)

// suggestCmd is the root command for hypothesis suggestion management.
var suggestCmd = &cobra.Command{
	Use:   "suggest",
	Short: "Manage experiment hypotheses (human approval gate)",
}

// suggestListCmd lists pending hypothesis suggestions.
var suggestListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pending hypothesis suggestions",
	RunE:  runSuggestList,
}

// suggestApproveCmd approves a hypothesis and submits it to Hub.
var suggestApproveCmd = &cobra.Command{
	Use:   "approve <hyp-id>",
	Short: "Approve a hypothesis and submit to Hub",
	Args:  cobra.ExactArgs(1),
	RunE:  runSuggestApprove,
}

func init() {
	suggestCmd.AddCommand(suggestListCmd)
	suggestCmd.AddCommand(suggestApproveCmd)
	rootCmd.AddCommand(suggestCmd)
}

// openKnowledgeStore opens the knowledge store for the current project.
func openKnowledgeStore() (*knowledge.Store, error) {
	knowledgeDir := filepath.Join(projectDir, ".c4", "knowledge")
	if err := os.MkdirAll(knowledgeDir, 0755); err != nil {
		return nil, fmt.Errorf("create knowledge dir: %w", err)
	}
	return knowledge.NewStore(knowledgeDir)
}

// readHypMeta reads expires_at and yaml_draft from the raw markdown frontmatter.
// Reads directly from the file to avoid a full store.Get() round-trip.
func readHypMeta(store *knowledge.Store, docID string) (expiresAt string, yamlDraft string) {
	filePath := filepath.Join(store.DocsDir(), docID+".md")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", ""
	}
	content := string(data)

	// Parse frontmatter: ---\n...\n---
	const sep = "---"
	if !strings.HasPrefix(content, sep) {
		return "", ""
	}
	rest := content[len(sep):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", ""
	}
	fm := rest[:end]

	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "expires_at:") {
			expiresAt = strings.TrimSpace(strings.TrimPrefix(line, "expires_at:"))
		} else if strings.HasPrefix(line, "yaml_draft:") {
			yamlDraft = strings.TrimSpace(strings.TrimPrefix(line, "yaml_draft:"))
		}
	}
	return expiresAt, yamlDraft
}

func runSuggestList(cmd *cobra.Command, args []string) error {
	store, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer store.Close()

	docs, err := store.List(string(knowledge.TypeHypothesis), "", 20)
	if err != nil {
		return fmt.Errorf("list hypotheses: %w", err)
	}

	// Filter pending by checking doc.Status via Get
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tINSIGHT\tEXPIRES_AT")
	count := 0
	for _, d := range docs {
		id, _ := d["id"].(string)
		doc, err := store.Get(id)
		if err != nil || doc == nil {
			continue
		}
		if doc.Status != "pending" {
			continue
		}
		expiresAt, _ := readHypMeta(store, id)
		insight := doc.Body
		if len(insight) > 100 {
			insight = insight[:100]
		}
		insight = strings.ReplaceAll(insight, "\n", " ")
		fmt.Fprintf(tw, "%s\t%s\t%s\n", id, insight, expiresAt)
		count++
	}
	tw.Flush()
	if count == 0 {
		fmt.Println("(no pending suggestions)")
	}
	return nil
}

func runSuggestApprove(cmd *cobra.Command, args []string) error {
	hypID := args[0]

	store, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer store.Close()

	doc, err := store.Get(hypID)
	if err != nil {
		return fmt.Errorf("get hypothesis: %w", err)
	}
	if doc == nil {
		return fmt.Errorf("hypothesis not found: %s", hypID)
	}

	// Validate status
	if doc.Status != "pending" {
		return fmt.Errorf("이미 처리된 제안입니다 (status=%s)", doc.Status)
	}

	// Check expiry
	expiresAt, yamlDraft := readHypMeta(store, hypID)
	if expiresAt != "" {
		exp, err := time.Parse(time.RFC3339, expiresAt)
		if err == nil && time.Now().After(exp) {
			return fmt.Errorf("만료된 제안입니다 (expires_at=%s)", expiresAt)
		}
	}

	// Submit to Hub
	if yamlDraft == "" {
		yamlDraft = doc.Body
	}

	hubClient, err := newHubClient()
	if err != nil {
		return fmt.Errorf("hub not configured: %w", err)
	}

	req := &hub.JobSubmitRequest{
		Name:      "suggest-" + hypID,
		Command:   yamlDraft,
		ProjectID: getActiveProjectID(projectDir),
	}
	resp, err := hubClient.SubmitJob(req)
	if err != nil {
		// Do NOT update status on hub failure (keep pending)
		return fmt.Errorf("Hub 제출 실패: %w", err)
	}

	// Update status to approved + store job_id
	_, updateErr := store.Update(hypID, map[string]any{
		"hypothesis_status": "approved",
		"status":            "approved",
	}, nil)
	if updateErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: status update failed: %v\n", updateErr)
	}

	fmt.Printf("submitted: %s\n", resp.JobID)
	return nil
}
