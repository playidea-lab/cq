package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/cloud"
	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage CQ projects",
	Long: `Manage CQ cloud projects.

Subcommands:
  list          - List projects you are a member of
  new --name    - Create a new project
  use <id>      - Set active project`,
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List projects you are a member of",
	Args:  cobra.NoArgs,
	RunE:  runProjectList,
}

var projectNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new project",
	RunE:  runProjectNew,
}

var projectUseCmd = &cobra.Command{
	Use:   "use <id-or-name>",
	Short: "Set active project",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectUse,
}

func init() {
	projectNewCmd.Flags().String("name", "", "Project name (required)")
	projectNewCmd.MarkFlagRequired("name") //nolint:errcheck

	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectNewCmd)
	projectCmd.AddCommand(projectUseCmd)
	rootCmd.AddCommand(projectCmd)
}

// newProjectClient creates a ProjectClient from config/env/builtin values
// and the current session's access token.
func newProjectClient() (*cloud.ProjectClient, string, error) {
	supabaseURL := readCloudURL(projectDir)
	if supabaseURL == "" {
		return nil, "", errors.New("no Supabase URL configured (set C4_CLOUD_URL or run 'cq auth login')")
	}
	anonKey := readCloudAnonKey(projectDir)
	if anonKey == "" {
		return nil, "", errors.New("no Supabase anon key configured (set C4_CLOUD_ANON_KEY or run 'cq auth login')")
	}

	// Load session for access token.
	authClient := cloud.NewAuthClient(supabaseURL, anonKey)
	session, err := authClient.GetSession()
	if err != nil {
		return nil, "", fmt.Errorf("loading session: %w", err)
	}
	if session == nil {
		return nil, "", errors.New("not authenticated: run 'cq auth login'")
	}
	if time.Now().Unix() >= session.ExpiresAt {
		return nil, "", errors.New("session expired: run 'cq auth login'")
	}

	return cloud.NewProjectClient(supabaseURL, anonKey, session.AccessToken), session.User.ID, nil
}

func runProjectList(cmd *cobra.Command, args []string) error {
	client, userID, err := newProjectClient()
	if err != nil {
		return err
	}

	projects, err := client.ListProjects(userID)
	if err != nil {
		return fmt.Errorf("listing projects: %w", err)
	}

	if len(projects) == 0 {
		fmt.Println("No projects found. Create one: cq project new --name <name>")
		return nil
	}

	activeID := getActiveProjectID(projectDir)

	// Print table: ID (16 chars) | Name | Created
	fmt.Printf("%-18s %-30s %-25s\n", "ID", "NAME", "CREATED")
	fmt.Println(strings.Repeat("-", 75))
	for _, p := range projects {
		shortID := p.ID
		if len(shortID) > 16 {
			shortID = shortID[:16]
		}
		created := formatProjectTime(p.CreatedAt)
		marker := ""
		if p.ID == activeID {
			marker = " *"
		}
		fmt.Printf("%-18s %-30s %-25s%s\n", shortID, p.Name, created, marker)
	}
	if activeID != "" {
		fmt.Println("\n* = active project")
	}

	return nil
}

func runProjectNew(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")

	client, userID, err := newProjectClient()
	if err != nil {
		return err
	}

	project, err := client.CreateProject(name, userID)
	if err != nil {
		return fmt.Errorf("creating project: %w", err)
	}

	// Set as active project.
	if err := cloud.SetActiveProject(projectDir, project.ID); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to set active project: %v\n", err)
	}

	fmt.Printf("Project created: %s (%s)\n", project.Name, project.ID)
	return nil
}

func runProjectUse(cmd *cobra.Command, args []string) error {
	idOrName := args[0]

	client, userID, err := newProjectClient()
	if err != nil {
		return err
	}

	// Fetch projects to resolve name → ID.
	projects, err := client.ListProjects(userID)
	if err != nil {
		return fmt.Errorf("listing projects: %w", err)
	}

	// Find by exact ID or name.
	var matched *cloud.Project
	for i := range projects {
		p := &projects[i]
		if p.ID == idOrName || p.Name == idOrName {
			matched = p
			break
		}
	}
	if matched == nil {
		return fmt.Errorf("project not found: %q (run 'cq project list' to see available projects): %w", idOrName, errors.New("not found"))
	}

	if err := cloud.SetActiveProject(projectDir, matched.ID); err != nil {
		return fmt.Errorf("setting active project: %w", err)
	}

	fmt.Printf("Active project: %s (%s)\n", matched.Name, matched.ID)
	return nil
}

// formatProjectTime parses an ISO 8601 timestamp and returns a short display string.
func formatProjectTime(ts string) string {
	if ts == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		// Try without nanoseconds.
		t, err = time.Parse("2006-01-02T15:04:05Z", ts)
		if err != nil {
			return ts
		}
	}
	return t.Format("2006-01-02 15:04")
}
