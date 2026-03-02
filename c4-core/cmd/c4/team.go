package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/cloud"
	"github.com/spf13/cobra"
)

var teamCmd = &cobra.Command{
	Use:   "team",
	Short: "Manage project team members",
	Long: `Manage team members for the active CQ project.

Subcommands:
  list                    - List all project members
  invite --email <email>  - Invite a user to the project by email
  remove --user <user-id> - Remove a user from the project`,
}

var teamListCmd = &cobra.Command{
	Use:   "list",
	Short: "List project members",
	Args:  cobra.NoArgs,
	RunE:  runTeamList,
}

var teamInviteCmd = &cobra.Command{
	Use:   "invite",
	Short: "Invite a user to the project by email",
	RunE:  runTeamInvite,
}

var teamRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove a user from the project",
	RunE:  runTeamRemove,
}

func init() {
	teamListCmd.Flags().String("project", "", "Project ID (default: active project)")
	teamInviteCmd.Flags().String("project", "", "Project ID (default: active project)")
	teamInviteCmd.Flags().String("email", "", "Email address of the user to invite (required)")
	teamInviteCmd.MarkFlagRequired("email") //nolint:errcheck
	teamRemoveCmd.Flags().String("project", "", "Project ID (default: active project)")
	teamRemoveCmd.Flags().String("user", "", "User ID to remove (required)")
	teamRemoveCmd.MarkFlagRequired("user") //nolint:errcheck

	teamCmd.AddCommand(teamListCmd, teamInviteCmd, teamRemoveCmd)
	rootCmd.AddCommand(teamCmd)
}

// newTeamClient creates a TeamClient from config/env and session access token.
func newTeamClient() (*cloud.TeamClient, error) {
	supabaseURL := readCloudURL(projectDir)
	if supabaseURL == "" {
		return nil, errors.New("no Supabase URL configured (set C4_CLOUD_URL or run 'cq auth login')")
	}
	anonKey := readCloudAnonKey(projectDir)
	if anonKey == "" {
		return nil, errors.New("no Supabase anon key configured (set C4_CLOUD_ANON_KEY or run 'cq auth login')")
	}

	authClient := cloud.NewAuthClient(supabaseURL, anonKey)
	session, err := authClient.GetSession()
	if err != nil {
		return nil, fmt.Errorf("loading session: %w", err)
	}
	if session == nil {
		return nil, errors.New("not authenticated: run 'cq auth login'")
	}
	if time.Now().Unix() >= session.ExpiresAt {
		return nil, errors.New("session expired: run 'cq auth login'")
	}

	return cloud.NewTeamClient(supabaseURL, anonKey, session.AccessToken), nil
}

// resolveProjectID returns the project ID from the --project flag or the active project.
func resolveProjectID(cmd *cobra.Command) (string, error) {
	if id, _ := cmd.Flags().GetString("project"); id != "" {
		return id, nil
	}
	id := getActiveProjectID(projectDir)
	if id == "" {
		return "", errors.New("no active project: run 'cq project use <id>' or pass --project <id>")
	}
	return id, nil
}

func runTeamList(cmd *cobra.Command, args []string) error {
	projectID, err := resolveProjectID(cmd)
	if err != nil {
		return err
	}

	client, err := newTeamClient()
	if err != nil {
		return err
	}

	members, err := client.ListMembers(projectID)
	if err != nil {
		return fmt.Errorf("listing members: %w", err)
	}

	if len(members) == 0 {
		fmt.Println("No members found.")
		return nil
	}

	fmt.Printf("%-10s %-30s %-30s %-10s\n", "USER_ID", "EMAIL", "DISPLAY_NAME", "ROLE")
	fmt.Println(strings.Repeat("-", 82))
	for _, m := range members {
		shortID := m.UserID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		fmt.Printf("%-10s %-30s %-30s %-10s\n", shortID, m.Email, m.DisplayName, m.Role)
	}

	return nil
}

func runTeamInvite(cmd *cobra.Command, args []string) error {
	email, _ := cmd.Flags().GetString("email")

	projectID, err := resolveProjectID(cmd)
	if err != nil {
		return err
	}

	client, err := newTeamClient()
	if err != nil {
		return err
	}

	result, err := client.InviteOrAdd(projectID, email)
	if err != nil {
		return fmt.Errorf("inviting user: %w", err)
	}

	switch result.Status {
	case "added":
		fmt.Printf("%s added to project\n", email)
	case "invited":
		fmt.Printf("Invitation sent to %s\n", email)
	case "already_member":
		fmt.Printf("%s is already a member\n", email)
	default:
		fmt.Printf("Result: %s\n", result.Status)
	}

	return nil
}

func runTeamRemove(cmd *cobra.Command, args []string) error {
	userID, _ := cmd.Flags().GetString("user")

	projectID, err := resolveProjectID(cmd)
	if err != nil {
		return err
	}

	client, err := newTeamClient()
	if err != nil {
		return err
	}

	if err := client.RemoveMember(projectID, userID); err != nil {
		return fmt.Errorf("removing member: %w", err)
	}

	fmt.Printf("Removed %s from project\n", userID)
	return nil
}
