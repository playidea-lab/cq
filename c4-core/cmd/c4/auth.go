package main

import (
	"fmt"
	"os"
	"time"

	"github.com/changmin/c4-core/internal/cloud"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage cloud authentication",
	Long: `Manage C4 Cloud authentication using Supabase Auth with GitHub OAuth.

Subcommands:
  login   - Authenticate with GitHub
  logout  - Clear stored credentials
  status  - Show current authentication status`,
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with GitHub OAuth",
	Long: `Start the GitHub OAuth flow to authenticate with C4 Cloud.

This opens your browser to authorize via GitHub. After authorization,
a session token is stored locally at ~/.c4/session.json.`,
	RunE: runAuthLogin,
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Clear stored credentials",
	Long:  "Remove the stored session token from ~/.c4/session.json.",
	RunE:  runAuthLogout,
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current auth status",
	Long:  "Display the current authentication status including user info and token expiry.",
	RunE:  runAuthStatus,
}

func init() {
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)
	rootCmd.AddCommand(authCmd)
}

// newAuthClient creates an AuthClient from environment or config.
// Checks C4_CLOUD_URL first, then falls back to SUPABASE_URL.
// Checks C4_CLOUD_ANON_KEY first, then falls back to SUPABASE_KEY.
func newAuthClient() (*cloud.AuthClient, error) {
	supabaseURL := os.Getenv("C4_CLOUD_URL")
	if supabaseURL == "" {
		supabaseURL = os.Getenv("SUPABASE_URL")
	}
	anonKey := os.Getenv("C4_CLOUD_ANON_KEY")
	if anonKey == "" {
		anonKey = os.Getenv("SUPABASE_KEY")
	}

	if supabaseURL == "" {
		return nil, fmt.Errorf("C4_CLOUD_URL or SUPABASE_URL environment variable is not set")
	}
	if anonKey == "" {
		return nil, fmt.Errorf("C4_CLOUD_ANON_KEY or SUPABASE_KEY environment variable is not set")
	}

	return cloud.NewAuthClient(supabaseURL, anonKey), nil
}

func runAuthLogin(cmd *cobra.Command, args []string) error {
	client, err := newAuthClient()
	if err != nil {
		return err
	}
	return client.LoginWithGitHub()
}

func runAuthLogout(cmd *cobra.Command, args []string) error {
	client, err := newAuthClient()
	if err != nil {
		return err
	}

	if err := client.Logout(); err != nil {
		return err
	}

	fmt.Println("Logged out successfully.")
	return nil
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	client, err := newAuthClient()
	if err != nil {
		return err
	}

	session, err := client.GetSession()
	if err != nil {
		return fmt.Errorf("reading session: %w", err)
	}

	if session == nil {
		fmt.Println("Not authenticated.")
		fmt.Println("Run 'c4 auth login' to authenticate with GitHub.")
		return nil
	}

	expiresAt := time.Unix(session.ExpiresAt, 0)
	expired := time.Now().After(expiresAt)

	fmt.Printf("User:    %s (%s)\n", session.User.Name, session.User.Email)
	fmt.Printf("ID:      %s\n", session.User.ID)

	if expired {
		fmt.Printf("Status:  expired (at %s)\n", expiresAt.Format(time.RFC3339))
		fmt.Println("Run 'c4 auth login' to re-authenticate.")
	} else {
		remaining := time.Until(expiresAt).Round(time.Minute)
		fmt.Printf("Status:  authenticated (expires in %s)\n", remaining)
	}

	return nil
}
