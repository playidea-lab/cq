package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// newAuthClient creates an AuthClient using the following priority:
//  1. Environment: C4_CLOUD_URL / C4_CLOUD_ANON_KEY
//  2. Environment: SUPABASE_URL / SUPABASE_KEY
//  3. Built-in defaults (set via ldflags at build time)
func newAuthClient() (*cloud.AuthClient, error) {
	supabaseURL := os.Getenv("C4_CLOUD_URL")
	if supabaseURL == "" {
		supabaseURL = os.Getenv("SUPABASE_URL")
	}
	if supabaseURL == "" {
		supabaseURL = builtinSupabaseURL
	}
	anonKey := os.Getenv("C4_CLOUD_ANON_KEY")
	if anonKey == "" {
		anonKey = os.Getenv("SUPABASE_KEY")
	}
	if anonKey == "" {
		anonKey = builtinSupabaseKey
	}

	if supabaseURL == "" {
		return nil, fmt.Errorf("no Supabase URL configured (set C4_CLOUD_URL or build with -ldflags)")
	}
	if anonKey == "" {
		return nil, fmt.Errorf("no Supabase key configured (set C4_CLOUD_ANON_KEY or build with -ldflags)")
	}

	return cloud.NewAuthClient(supabaseURL, anonKey), nil
}

func runAuthLogin(cmd *cobra.Command, args []string) error {
	client, err := newAuthClient()
	if err != nil {
		return err
	}
	if err := client.LoginWithGitHub(); err != nil {
		return err
	}

	// Auto-patch .c4/config.yaml cloud section after successful login.
	url := patchCloudConfigAfterLogin(projectDir)
	if url != "" {
		fmt.Fprintf(os.Stderr, "Cloud configured: %s\n", url)
	}
	return nil
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
		fmt.Println("Run 'cq auth login' to authenticate with GitHub.")
		return nil
	}

	expiresAt := time.Unix(session.ExpiresAt, 0)
	expired := time.Now().After(expiresAt)

	fmt.Printf("User:    %s (%s)\n", session.User.Name, session.User.Email)
	fmt.Printf("ID:      %s\n", session.User.ID)

	if expired {
		fmt.Printf("Status:  expired (at %s)\n", expiresAt.Format(time.RFC3339))
		fmt.Println("Run 'cq auth login' to re-authenticate.")
	} else {
		remaining := time.Until(expiresAt).Round(time.Minute)
		fmt.Printf("Status:  authenticated (expires in %s)\n", remaining)
	}

	return nil
}

// patchCloudConfigAfterLogin patches the cloud section of .c4/config.yaml
// after a successful OAuth login. It sets cloud.enabled=true, cloud.url,
// cloud.anon_key, and cloud.mode=local-first. If the user has already set
// cloud.url, it is preserved (not overwritten). Returns the effective URL
// on success, or "" if patching was skipped (e.g., .c4/ doesn't exist).
func patchCloudConfigAfterLogin(projDir string) string {
	c4DirPath := filepath.Join(projDir, ".c4")
	if _, err := os.Stat(c4DirPath); os.IsNotExist(err) {
		// .c4/ directory doesn't exist — cq init hasn't been run yet.
		// Gracefully skip config patching.
		return ""
	}

	configPath := filepath.Join(c4DirPath, "config.yaml")

	var existing string
	if data, err := os.ReadFile(configPath); err == nil {
		existing = string(data)
	}

	// Determine the effective URL: preserve user's custom value if set.
	effectiveURL := builtinSupabaseURL
	if userURL := cloudYAMLValue(existing, "url:"); userURL != "" {
		effectiveURL = userURL
	}

	effectiveAnonKey := builtinSupabaseKey
	if userKey := cloudYAMLValue(existing, "anon_key:"); userKey != "" {
		effectiveAnonKey = userKey
	}

	// Build the desired cloud section values.
	desired := map[string]string{
		"enabled:":  "true",
		"url:":      effectiveURL,
		"anon_key:": effectiveAnonKey,
		"mode:":     "local-first",
	}

	result := writeCloudSectionToYAML(existing, desired)
	if err := os.WriteFile(configPath, []byte(result), 0644); err != nil {
		// Non-fatal: login succeeded, config patch failed.
		fmt.Fprintf(os.Stderr, "Warning: failed to write config: %v\n", err)
		return ""
	}

	return effectiveURL
}

// cloudYAMLValue extracts a value for a key within the cloud: section of a
// YAML string. Returns "" if not found. The key must include the trailing colon
// (e.g., "url:").
func cloudYAMLValue(content, key string) string {
	lines := strings.Split(content, "\n")
	inCloud := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "cloud:" {
			inCloud = true
			continue
		}
		if inCloud {
			// End of cloud section: non-empty, non-comment, non-indented line.
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") &&
				!strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				break
			}
			if trimmed == key || strings.HasPrefix(trimmed, key+" ") || strings.HasPrefix(trimmed, key+"	") {
				val := strings.TrimSpace(strings.TrimPrefix(trimmed, key))
				return val
			}
		}
	}
	return ""
}

// writeCloudSectionToYAML updates or creates the cloud: section with the given
// key-value pairs. Keys must include trailing colon (e.g., "enabled:").
func writeCloudSectionToYAML(existing string, desired map[string]string) string {
	lines := strings.Split(existing, "\n")

	// Find cloud: section boundaries.
	cloudStart := -1
	cloudEnd := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "cloud:" {
			cloudStart = i
			continue
		}
		if cloudStart >= 0 && cloudEnd < 0 {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				cloudEnd = i
				break
			}
		}
	}
	if cloudStart >= 0 && cloudEnd < 0 {
		cloudEnd = len(lines)
	}

	if cloudStart >= 0 {
		// Update existing cloud section: replace or insert keys.
		remaining := make(map[string]string)
		for k, v := range desired {
			remaining[k] = v
		}

		for i := cloudStart + 1; i < cloudEnd; i++ {
			trimmed := strings.TrimSpace(lines[i])
			for key, val := range remaining {
				if trimmed == key || strings.HasPrefix(trimmed, key+" ") || strings.HasPrefix(trimmed, key+"	") {
					lines[i] = "  " + key + " " + val
					delete(remaining, key)
					break
				}
			}
		}

		// Append any keys not found in existing section.
		if len(remaining) > 0 {
			var insertLines []string
			// Insert in a deterministic order.
			for _, key := range []string{"enabled:", "url:", "anon_key:", "mode:"} {
				if val, ok := remaining[key]; ok {
					insertLines = append(insertLines, "  "+key+" "+val)
				}
			}
			newLines := make([]string, 0, len(lines)+len(insertLines))
			newLines = append(newLines, lines[:cloudStart+1]...)
			newLines = append(newLines, insertLines...)
			newLines = append(newLines, lines[cloudStart+1:]...)
			lines = newLines
		}

		return strings.Join(lines, "\n")
	}

	// No cloud: section — append one.
	var sb strings.Builder
	sb.WriteString(existing)
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		sb.WriteString("\n")
	}
	sb.WriteString("cloud:\n")
	for _, key := range []string{"enabled:", "url:", "anon_key:", "mode:"} {
		if val, ok := desired[key]; ok {
			sb.WriteString("  " + key + " " + val + "\n")
		}
	}
	return sb.String()
}
