package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/changmin/c4-core/internal/cloud"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var signupCmd = &cobra.Command{
	Use:   "signup",
	Short: "Create a new account with email and password",
	Long: `Create a new C4 Cloud account using email and password.

In interactive mode (default), prompts for email and password if not provided.
In headless mode (--headless, CI=1, or no display), --email and --password are required.

Examples:
  # Interactive
  cq auth signup

  # Headless / CI
  cq auth signup --email foo@example.com --password secret --headless`,
	RunE: runAuthSignup,
}

func init() {
	signupCmd.Flags().String("email", "", "Email address for the new account")
	signupCmd.Flags().String("password", "", "Password for the new account")
	signupCmd.Flags().Bool("headless", false, "Non-interactive mode: --email and --password required")
	authCmd.AddCommand(signupCmd)
}

// isHeadless returns true when the environment indicates a non-interactive session.
// headlessFlag overrides all other checks.
func isHeadless(headlessFlag bool, envMap map[string]string) bool {
	if headlessFlag {
		return true
	}
	if envMap["CI"] != "" {
		return true
	}
	if envMap["DISPLAY"] == "" && envMap["TERM_PROGRAM"] == "" {
		return true
	}
	return false
}

// runAuthSignup implements the cq auth signup command.
func runAuthSignup(cmd *cobra.Command, args []string) error {
	headlessFlag, _ := cmd.Flags().GetBool("headless")
	hl := isHeadless(headlessFlag, map[string]string{
		"CI":           os.Getenv("CI"),
		"DISPLAY":      os.Getenv("DISPLAY"),
		"TERM_PROGRAM": os.Getenv("TERM_PROGRAM"),
	})

	email, _ := cmd.Flags().GetString("email")
	password, _ := cmd.Flags().GetString("password")

	if hl {
		if email == "" || password == "" {
			return errors.New("--email and --password required in headless mode")
		}
	} else {
		if email == "" {
			fmt.Fprint(os.Stderr, "Email: ")
			fmt.Scan(&email)
		}
		if password == "" {
			fmt.Fprint(os.Stderr, "Password: ")
			pw, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr) // newline after hidden input
			if err != nil {
				return fmt.Errorf("reading password: %w", err)
			}
			password = string(pw)
		}
	}

	email = strings.TrimSpace(email)
	password = strings.TrimSpace(password)

	if email == "" {
		return errors.New("email is required")
	}
	if password == "" {
		return errors.New("password is required")
	}

	client, err := newAuthClient()
	if err != nil {
		return err
	}

	session, err := client.SignUpWithEmail(email, password)
	if err != nil {
		if strings.Contains(err.Error(), "already") {
			fmt.Fprintln(os.Stderr, "Email already in use. Try: cq auth login")
			return err
		}
		return err
	}

	if err := client.UpsertProfile(session); err != nil {
		// Non-fatal: profile upsert failure should not block account creation.
		fmt.Fprintf(os.Stderr, "warn: profile setup failed: %v\n", err)
	} else if patchErr := cloud.PatchTeamYAMLCloudUID(projectDir, session.UserID); patchErr != nil {
		fmt.Fprintf(os.Stderr, "warn: team.yaml cloud_uid update failed: %v\n", patchErr)
	}

	fmt.Printf("Account created! Welcome, %s\n", session.Email)
	return nil
}
