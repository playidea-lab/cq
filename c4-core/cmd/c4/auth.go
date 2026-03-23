package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/cloud"
	"github.com/changmin/c4-core/internal/config"
	"github.com/spf13/cobra"
)

// authLoginFunc is a package-level variable so tests can stub the login call.
// mode: "" = browser OAuth, "link" = Direct Link, "device" = Device Flow.
var authLoginFunc = func(mode string) error {
	switch mode {
	case "device":
		return runAuthLoginHeadless(nil, true)
	case "link":
		return runAuthLoginHeadless(nil, false)
	case "otp":
		return runAuthLoginOTP()
	default:
		return runAuthLogin(nil, nil) // browser OAuth fallback
	}
}

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage cloud authentication",
	Long: `Manage C4 Cloud authentication using Supabase Auth with GitHub OAuth.

Subcommands:
  login   - Authenticate with GitHub
  logout  - Clear stored credentials
  status  - Show current authentication status
  refresh - Login using a refresh token (headless)
  token   - Import a session token (headless login)`,
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

var authRefreshCmd = &cobra.Command{
	Use:   "refresh <token>",
	Short: "Login using a refresh token (headless)",
	Long: `Exchange a refresh token for a new session without browser OAuth.

Get your refresh token from another machine with: cq auth status
Then run on the remote server:
  cq auth refresh <refresh-token>`,
	Args: cobra.ExactArgs(1),
	RunE: runAuthRefresh,
}

var authTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Import a session token (headless login)",
	Long: `Import a pre-existing session token from JSON without browser OAuth.

Reads session JSON from stdin or --json flag and saves it to ~/.c4/session.json.
Useful for headless/remote servers where browser-based OAuth is not available.

Examples:
  # Pipe from stdin
  echo '{"access_token":"...","refresh_token":"...","expires_at":...}' | cq auth token

  # From a file
  cat ~/.c4/session.json | ssh user@remote-server cq auth token

  # Via --json flag
  cq auth token --json '{"access_token":"..."}'`,
	RunE: runAuthToken,
}

func init() {
	authLoginCmd.Flags().Bool("no-browser", false, "Do not open the browser; print the URL and SSH hint to stderr instead")
	authLoginCmd.Flags().Bool("device", false, "Device Flow: refresh token or guided headless login")
	authLoginCmd.Flags().Bool("link", false, "Direct Link: refresh token or guided headless login")
	authLoginCmd.Flags().Bool("otp", false, "Email OTP: receive a 6-digit code via email (no browser needed)")
	authLoginCmd.MarkFlagsMutuallyExclusive("device", "link", "otp")
	authTokenCmd.Flags().String("json", "", "Session JSON string (reads from stdin if not set)")
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authRefreshCmd)
	authCmd.AddCommand(authTokenCmd)
	rootCmd.AddCommand(authCmd)
}

// newAuthClient creates an AuthClient using the following priority:
//  1. Environment: C4_CLOUD_URL / C4_CLOUD_ANON_KEY
//  2. Environment: SUPABASE_URL / SUPABASE_KEY
//  3. .c4/config.yaml cloud.url / cloud.anon_key
//  4. Built-in defaults (set via ldflags at build time)
func newAuthClient() (*cloud.AuthClient, error) {
	supabaseURL := readCloudURL(projectDir)
	anonKey := readCloudAnonKey(projectDir)

	if supabaseURL == "" {
		return nil, fmt.Errorf("no Supabase URL configured (set C4_CLOUD_URL or build with -ldflags)")
	}
	if anonKey == "" {
		return nil, fmt.Errorf("no Supabase key configured (set C4_CLOUD_ANON_KEY or build with -ldflags)")
	}

	return cloud.NewAuthClient(supabaseURL, anonKey), nil
}

func runAuthLogin(cmd *cobra.Command, args []string) error {
	// Check --device / --link flags.
	var useDevice, useLink, useOTP bool
	if cmd != nil {
		if v, err := cmd.Flags().GetBool("device"); err == nil {
			useDevice = v
		}
		if v, err := cmd.Flags().GetBool("link"); err == nil {
			useLink = v
		}
		if v, err := cmd.Flags().GetBool("otp"); err == nil {
			useOTP = v
		}
	}

	if useOTP {
		return runAuthLoginOTP()
	}
	if useDevice || useLink {
		return runAuthLoginHeadless(cmd, useDevice)
	}

	client, err := newAuthClient()
	if err != nil {
		return err
	}
	if cmd != nil {
		if noBrowser, err := cmd.Flags().GetBool("no-browser"); err == nil {
			client.NoBrowser = noBrowser
		}
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

// runAuthLoginHeadless handles --device and --link flows via C5 Hub.
func runAuthLoginHeadless(cmd *cobra.Command, isDevice bool) error {
	// Try refresh token first — works without any server.
	authClient, err := newAuthClient()
	if err != nil {
		return err
	}
	session, getErr := authClient.GetSession()
	if getErr == nil && session != nil && session.RefreshToken != "" {
		// Have a refresh token — try refreshing.
		newSession, refreshErr := authClient.RefreshTokenWithToken(session.RefreshToken)
		if refreshErr == nil {
			fmt.Printf("✓ 토큰 갱신 성공 (%s)\n", newSession.User.Email)
			url := patchCloudConfigAfterLogin(projectDir)
			if url != "" {
				fmt.Fprintf(os.Stderr, "Cloud configured: %s\n", url)
			}
			return nil
		}
		fmt.Fprintf(os.Stderr, "cq: 토큰 갱신 실패: %v\n", refreshErr)
	}

	// No refresh token or refresh failed — guide the user.
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Headless login requires a refresh token from another machine.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "On a machine with a browser:")
	fmt.Fprintln(os.Stderr, "  1. cq auth login              (브라우저 OAuth)")
	fmt.Fprintln(os.Stderr, "  2. cq auth status             (refresh token 확인)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Then on this machine:")
	fmt.Fprintln(os.Stderr, "  cq auth refresh <refresh_token>")
	fmt.Fprintln(os.Stderr, "")
	return fmt.Errorf("headless login: no refresh token available")
}

// resolveHubURL reads hub.url from env → .c4/config.yaml → builtinHubURL (ldflags).
func resolveHubURL() string {
	// 1. Environment variable (worker/CI environments)
	if v := os.Getenv("C5_HUB_URL"); v != "" {
		return v
	}
	// 2. Config file
	configPath := filepath.Join(projectDir, ".c4", "config.yaml")
	if data, err := os.ReadFile(configPath); err == nil {
		if v := sectionYAMLValue(string(data), "hub", "url:"); v != "" {
			return v
		}
	}
	// 3. Baked-in at build time via ldflags
	return builtinHubURL
}

// ensureCloudAuth checks cloud authentication and prompts the user to log in if
// needed. Returns true if the caller should continue (solo mode, valid session,
// or login succeeded). Returns false if the user declined or login failed.
//
//   - r == nil → uses os.Stdin
//   - builtinSupabaseURL == "" → solo mode → returns true immediately
//   - valid session exists → returns true
//   - not logged in → prompts; yesAll=true skips the prompt
//   - Hub configured: shows [y/d/N] prompt (y=link, d=device)
//   - Hub not configured: shows [y/N] prompt (y=browser OAuth)
//   - user inputs "y" → link (hub) or browser OAuth (no hub)
//   - user inputs "d" → device flow (hub required)
//   - user inputs "n" or EOF → returns false
func ensureCloudAuth(r io.Reader, yesAll bool) bool {
	// Resolve cloud URL (same priority as newAuthClient).
	supabaseURL := os.Getenv("C4_CLOUD_URL")
	if supabaseURL == "" {
		supabaseURL = os.Getenv("SUPABASE_URL")
	}
	if supabaseURL == "" {
		supabaseURL = builtinSupabaseURL
	}

	// Solo mode: no cloud URL configured.
	if supabaseURL == "" {
		return true
	}

	// Check for a valid existing session.
	client := cloud.NewAuthClient(supabaseURL, "")
	session, err := client.GetSession()
	if err == nil && session != nil && !time.Now().After(time.Unix(session.ExpiresAt, 0)) {
		return true
	}

	// Not logged in — prompt the user (or skip prompt if yesAll).
	fmt.Fprintln(os.Stderr, "cq: [cloud] 로그인 필요. 'cq auth login'으로 GitHub OAuth 인증 후 클라우드 동기화 및 Hub 이용 가능.")

	hasHub := resolveHubURL() != ""
	mode := ""

	if !yesAll {
		if r == nil {
			r = os.Stdin
		}
		if hasHub {
			fmt.Fprint(os.Stderr, "지금 로그인하시겠습니까? (y=링크, d=디바이스 코드) [y/d/N] ")
		} else {
			fmt.Fprint(os.Stderr, "지금 로그인하시겠습니까? [y/N] ")
		}
		scanner := bufio.NewScanner(r)
		if !scanner.Scan() {
			return false
		}
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		switch answer {
		case "y", "yes":
			if hasHub {
				mode = "link"
			}
		case "d", "device":
			if !hasHub {
				fmt.Fprintln(os.Stderr, "cq: 디바이스 코드 방식은 Hub URL 설정이 필요합니다. 'cq config set hub.url <URL>'")
				return false
			}
			mode = "device"
		default:
			return false
		}
	} else {
		if hasHub {
			mode = "link"
		}
	}

	if err := authLoginFunc(mode); err != nil {
		fmt.Fprintf(os.Stderr, "cq: login failed: %v\n", err)
		return false
	}
	return true
}

func runAuthLoginOTP() error {
	client, err := newAuthClient()
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(os.Stdin)

	fmt.Fprint(os.Stderr, "이메일 주소: ")
	if !scanner.Scan() {
		return fmt.Errorf("입력이 종료되었습니다")
	}
	email := strings.TrimSpace(scanner.Text())
	if email == "" {
		return fmt.Errorf("이메일이 비어있습니다")
	}

	fmt.Fprintf(os.Stderr, "%s 으로 인증 코드를 전송합니다...\n", email)
	if err := client.SendOTP(email); err != nil {
		return fmt.Errorf("OTP 전송 실패: %w", err)
	}
	fmt.Fprintln(os.Stderr, "✓ 인증 코드가 이메일로 전송되었습니다. (스팸 폴더도 확인하세요)")

	fmt.Fprint(os.Stderr, "인증 코드:")
	if !scanner.Scan() {
		return fmt.Errorf("입력이 종료되었습니다")
	}
	code := strings.TrimSpace(scanner.Text())
	if code == "" {
		return fmt.Errorf("인증 코드가 비어있습니다")
	}

	session, err := client.VerifyOTP(email, code)
	if err != nil {
		return fmt.Errorf("인증 실패: %w", err)
	}

	fmt.Printf("✓ 로그인 성공: %s (%s)\n", session.User.Name, session.User.Email)

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
	fmt.Printf("Refresh: %s\n", session.RefreshToken)

	if expired {
		fmt.Printf("Status:  expired (at %s)\n", expiresAt.Format(time.RFC3339))
		fmt.Println("Run 'cq auth login' to re-authenticate.")
	} else {
		remaining := time.Until(expiresAt).Round(time.Minute)
		fmt.Printf("Status:  authenticated (expires in %s)\n", remaining)
	}

	return nil
}

func runAuthRefresh(cmd *cobra.Command, args []string) error {
	client, err := newAuthClient()
	if err != nil {
		return err
	}
	session, err := client.RefreshTokenWithToken(args[0])
	if err != nil {
		return err
	}
	expiresAt := time.Unix(session.ExpiresAt, 0)
	fmt.Printf("Logged in as %s (%s)\n", session.User.Name, session.User.Email)
	fmt.Printf("Expires: %s\n", expiresAt.Format(time.RFC3339))
	return nil
}

func runAuthToken(cmd *cobra.Command, args []string) error {
	// Read JSON from --json flag or stdin.
	var raw string
	if jsonFlag, err := cmd.Flags().GetString("json"); err == nil && jsonFlag != "" {
		raw = jsonFlag
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		raw = strings.TrimSpace(string(data))
	}

	if raw == "" {
		return fmt.Errorf("no session JSON provided (use --json flag or pipe via stdin)")
	}

	var session cloud.Session
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return fmt.Errorf("invalid session JSON: %w", err)
	}
	if session.AccessToken == "" {
		return fmt.Errorf("session JSON missing access_token")
	}

	client, err := newAuthClient()
	if err != nil {
		return err
	}
	if err := client.SaveSessionPublic(&session); err != nil {
		return fmt.Errorf("saving session: %w", err)
	}

	expiresAt := time.Unix(session.ExpiresAt, 0)
	fmt.Printf("Session imported: %s (%s)\n", session.User.Name, session.User.Email)
	fmt.Printf("Expires: %s\n", expiresAt.Format(time.RFC3339))
	return nil
}

// patchCloudConfigAfterLogin patches the cloud section of .c4/config.yaml
// after a successful OAuth login. It sets cloud.enabled=true, cloud.url,
// cloud.anon_key, and cloud.mode=cloud-primary. If the user has already set
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
		"mode:":     config.CloudModePrimary,
	}

	result := writeCloudSectionToYAML(existing, desired)

	// Auto-set LLM proxy base_url for connected tier (no user API key needed).
	if effectiveURL != "" {
		llmProxyURL := effectiveURL + "/functions/v1/llm-proxy"
		result = ensureLLMGatewayBaseURL(result, llmProxyURL)
	}
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

// ensureLLMGatewayBaseURL ensures llm_gateway.providers.anthropic.base_url
// is set in the config YAML. Preserves existing value if already present.
func ensureLLMGatewayBaseURL(content, proxyURL string) string {
	hasGateway := false
	hasAnthropicBaseURL := false
	inGateway := false
	inAnthropic := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		// Track llm_gateway section
		if trimmed == "llm_gateway:" {
			hasGateway = true
			inGateway = true
			continue
		}
		if inGateway && trimmed != "" && !strings.HasPrefix(trimmed, "#") &&
			!strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inGateway = false
			inAnthropic = false
		}
		// Track anthropic provider within llm_gateway
		if inGateway && trimmed == "anthropic:" {
			inAnthropic = true
			continue
		}
		// Another provider resets inAnthropic
		if inGateway && inAnthropic && !strings.HasPrefix(line, "      ") &&
			trimmed != "" && !strings.HasPrefix(trimmed, "#") && strings.HasSuffix(trimmed, ":") {
			inAnthropic = false
		}
		if inAnthropic && strings.HasPrefix(trimmed, "base_url:") {
			hasAnthropicBaseURL = true
			break
		}
	}

	if hasAnthropicBaseURL {
		return content
	}
	if !hasGateway {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return content + "\nllm_gateway:\n  providers:\n    anthropic:\n      base_url: " + proxyURL + "\n"
	}

	// llm_gateway exists but anthropic has no base_url — insert after anthropic:
	lines := strings.Split(content, "\n")
	var result []string
	inserted := false
	for _, line := range lines {
		result = append(result, line)
		if !inserted && strings.TrimSpace(line) == "anthropic:" {
			// Only insert if we're in the llm_gateway section (anthropic: is indented)
			if strings.HasPrefix(line, "    ") {
				result = append(result, "      base_url: "+proxyURL)
				inserted = true
			}
		}
	}
	return strings.Join(result, "\n")
}
