package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// checkStatus represents the result of a single diagnostic check.
type checkStatus string

const (
	checkOK   checkStatus = "OK"
	checkWarn checkStatus = "WARN"
	checkFail checkStatus = "FAIL"
)

// checkResult holds the outcome of one diagnostic check.
type checkResult struct {
	Name    string      `json:"name"`
	Status  checkStatus `json:"status"`
	Message string      `json:"message"`
	Fix     string      `json:"fix,omitempty"`
}

var (
	doctorFix  bool
	doctorJSON bool
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose CQ installation and environment",
	Long: `doctor checks the CQ installation and reports any problems.

Each check is reported as [OK], [WARN], or [FAIL] with a description.
FAIL items include a suggested fix command.

Use --fix to automatically repair simple issues (symlinks, config gaps).
Use --json for machine-readable output.`,
	RunE: runDoctor,
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "auto-fix simple issues")
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "output results as JSON")
	// doctor doesn't require a .c4/ directory — override the root PersistentPreRunE
	doctorCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error { return nil }
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	// Resolve projectDir if not set (PersistentPreRunE is overridden for doctor)
	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
	}
	if abs, err := filepath.Abs(projectDir); err == nil {
		projectDir = abs
	}
	checks := []func() checkResult{
		checkBinary,
		checkC4Dir,
		checkMCPJson,
		checkClaudeMDSymlink,
		checkHooks,
		checkPythonSidecar,
		checkHub,
		checkSupabase,
	}

	results := make([]checkResult, 0, len(checks))
	for _, fn := range checks {
		r := fn()
		if doctorFix && (r.Status == checkFail || r.Status == checkWarn) {
			fixed := tryFix(&r)
			if fixed != "" {
				r.Message += " (fixed: " + fixed + ")"
				r.Fix = ""
			}
		}
		results = append(results, r)
	}

	if doctorJSON {
		return printDoctorJSON(results)
	}
	printDoctorHuman(results)
	return nil
}

// printDoctorJSON outputs results as a JSON array.
func printDoctorJSON(results []checkResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

// printDoctorHuman prints results in human-readable form.
func printDoctorHuman(results []checkResult) {
	failCount := 0
	warnCount := 0
	for _, r := range results {
		icon := "OK  "
		switch r.Status {
		case checkWarn:
			icon = "WARN"
			warnCount++
		case checkFail:
			icon = "FAIL"
			failCount++
		}
		fmt.Printf("[%s] %s: %s\n", icon, r.Name, r.Message)
		if r.Fix != "" {
			fmt.Printf("       Fix: %s\n", r.Fix)
		}
	}
	fmt.Println()
	if failCount == 0 && warnCount == 0 {
		fmt.Println("All checks passed.")
	} else {
		fmt.Printf("%d failed, %d warnings.\n", failCount, warnCount)
	}
}

// checkBinary verifies that cq binary is on PATH and shows its version.
func checkBinary() checkResult {
	path, err := exec.LookPath("cq")
	if err != nil {
		return checkResult{
			Name:    "cq binary",
			Status:  checkFail,
			Message: "cq not found on PATH",
			Fix:     "go build -o ~/.local/bin/cq ./cmd/c4/ && export PATH=$PATH:~/.local/bin",
		}
	}
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return checkResult{
			Name:    "cq binary",
			Status:  checkWarn,
			Message: fmt.Sprintf("found at %s but --version failed: %v", path, err),
		}
	}
	ver := strings.TrimSpace(string(out))
	return checkResult{
		Name:    "cq binary",
		Status:  checkOK,
		Message: fmt.Sprintf("%s (%s)", ver, path),
	}
}

// checkC4Dir verifies the .c4/ directory and required files exist.
func checkC4Dir() checkResult {
	dir := filepath.Join(projectDir, ".c4")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return checkResult{
			Name:    ".c4 directory",
			Status:  checkFail,
			Message: fmt.Sprintf(".c4/ not found in %s", projectDir),
			Fix:     "cq claude  (or: cq init) to initialize the project",
		}
	}

	// Check for database file (tasks.db or c4.db)
	hasDB := false
	for _, f := range []string{"tasks.db", "c4.db"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			hasDB = true
			break
		}
	}
	if !hasDB {
		return checkResult{
			Name:    ".c4 directory",
			Status:  checkWarn,
			Message: ".c4/ found but no database (tasks.db or c4.db)",
			Fix:     "cq claude to re-initialize",
		}
	}
	// config.yaml is optional — note if present
	configInfo := "no config.yaml"
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err == nil {
		configInfo = "config.yaml"
	}
	return checkResult{
		Name:    ".c4 directory",
		Status:  checkOK,
		Message: fmt.Sprintf("%s (db, %s)", dir, configInfo),
	}
}

// checkMCPJson validates the .mcp.json file and that the binary it references exists.
func checkMCPJson() checkResult {
	mcpPath := filepath.Join(projectDir, ".mcp.json")
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		return checkResult{
			Name:    ".mcp.json",
			Status:  checkFail,
			Message: ".mcp.json not found",
			Fix:     "cq claude to (re-)initialize the project",
		}
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return checkResult{
			Name:    ".mcp.json",
			Status:  checkFail,
			Message: fmt.Sprintf("invalid JSON: %v", err),
			Fix:     "cq claude to regenerate .mcp.json",
		}
	}

	// Look for a command entry that references cq binary
	binPath := extractMCPBinaryPath(cfg)
	if binPath == "" {
		return checkResult{
			Name:    ".mcp.json",
			Status:  checkWarn,
			Message: ".mcp.json parsed but could not find cq binary reference",
		}
	}
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		return checkResult{
			Name:    ".mcp.json",
			Status:  checkFail,
			Message: fmt.Sprintf("referenced binary missing: %s", binPath),
			Fix:     "cq claude to regenerate .mcp.json with correct binary path",
		}
	}
	return checkResult{
		Name:    ".mcp.json",
		Status:  checkOK,
		Message: fmt.Sprintf("valid JSON, binary exists (%s)", binPath),
	}
}

// extractMCPBinaryPath looks inside the MCP config for the first command path that looks like cq.
func extractMCPBinaryPath(cfg map[string]interface{}) string {
	servers, ok := cfg["mcpServers"].(map[string]interface{})
	if !ok {
		return ""
	}
	for _, v := range servers {
		srv, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		cmd, _ := srv["command"].(string)
		if strings.Contains(cmd, "cq") {
			return expandTilde(cmd)
		}
		// args may hold the binary
		if args, ok := srv["args"].([]interface{}); ok && len(args) > 0 {
			if s, ok := args[0].(string); ok && strings.Contains(s, "cq") {
				return expandTilde(s)
			}
		}
	}
	return ""
}

// checkClaudeMDSymlink checks CLAUDE.md / AGENTS.md symlink status.
func checkClaudeMDSymlink() checkResult {
	claudePath := filepath.Join(projectDir, "CLAUDE.md")
	fi, err := os.Lstat(claudePath)
	if os.IsNotExist(err) {
		return checkResult{
			Name:    "CLAUDE.md",
			Status:  checkWarn,
			Message: "CLAUDE.md not found",
			Fix:     "cq claude to create CLAUDE.md with C4 overrides",
		}
	}
	if err != nil {
		return checkResult{
			Name:    "CLAUDE.md",
			Status:  checkFail,
			Message: fmt.Sprintf("stat error: %v", err),
		}
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(claudePath)
		if err != nil {
			return checkResult{
				Name:    "CLAUDE.md",
				Status:  checkFail,
				Message: "CLAUDE.md is a broken symlink",
				Fix:     fmt.Sprintf("rm %s && cq claude", claudePath),
			}
		}
		// Verify target exists
		if _, err := os.Stat(claudePath); os.IsNotExist(err) {
			return checkResult{
				Name:    "CLAUDE.md",
				Status:  checkFail,
				Message: fmt.Sprintf("CLAUDE.md symlink target missing: %s", target),
				Fix:     fmt.Sprintf("rm %s && cq claude", claudePath),
			}
		}
		return checkResult{
			Name:    "CLAUDE.md",
			Status:  checkOK,
			Message: fmt.Sprintf("symlink → %s", target),
		}
	}
	return checkResult{
		Name:    "CLAUDE.md",
		Status:  checkOK,
		Message: "regular file with C4 overrides",
	}
}

// checkHooks verifies ~/.claude/hooks/ setup and settings.json patch.
func checkHooks() checkResult {
	home, err := os.UserHomeDir()
	if err != nil {
		return checkResult{
			Name:    "hooks",
			Status:  checkWarn,
			Message: "cannot determine home directory",
		}
	}

	hookFile := filepath.Join(home, ".claude", "hooks", "c4-bash-security-hook.sh")
	if _, err := os.Stat(hookFile); os.IsNotExist(err) {
		return checkResult{
			Name:    "hooks",
			Status:  checkWarn,
			Message: "c4-bash-security-hook.sh not installed",
			Fix:     "cq claude to install hooks (run from any CQ project directory)",
		}
	}

	// Check if installed hook content matches the embedded template
	if hookNeedsUpdate(hookFile, hookShContent) {
		return checkResult{
			Name:    "hooks",
			Status:  checkWarn,
			Message: fmt.Sprintf("hook outdated at %s (binary has newer version)", hookFile),
			Fix:     "cq doctor --fix  (or: cq claude)",
		}
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return checkResult{
			Name:    "hooks",
			Status:  checkWarn,
			Message: fmt.Sprintf("hook file found but ~/.claude/settings.json missing: %v", err),
		}
	}
	if !strings.Contains(string(data), "c4-bash-security-hook") {
		return checkResult{
			Name:    "hooks",
			Status:  checkWarn,
			Message: "hook file exists but not registered in settings.json",
			Fix:     "cq claude to patch settings.json",
		}
	}
	return checkResult{
		Name:    "hooks",
		Status:  checkOK,
		Message: fmt.Sprintf("%s (registered in settings.json)", hookFile),
	}
}

// checkPythonSidecar verifies uv and Python sidecar dependencies.
func checkPythonSidecar() checkResult {
	uvPath, err := exec.LookPath("uv")
	if err != nil {
		return checkResult{
			Name:    "Python sidecar",
			Status:  checkWarn,
			Message: "uv not found — LSP/doc tools will be unavailable",
			Fix:     "curl -LsSf https://astral.sh/uv/install.sh | sh",
		}
	}

	// Check if pyproject.toml or requirements exist in project
	hasProject := false
	for _, f := range []string{
		filepath.Join(projectDir, "pyproject.toml"),
		filepath.Join(projectDir, "c4", "pyproject.toml"),
	} {
		if _, err := os.Stat(f); err == nil {
			hasProject = true
			break
		}
	}
	if !hasProject {
		return checkResult{
			Name:    "Python sidecar",
			Status:  checkOK,
			Message: fmt.Sprintf("uv found at %s (no pyproject.toml in project)", uvPath),
		}
	}

	return checkResult{
		Name:    "Python sidecar",
		Status:  checkOK,
		Message: fmt.Sprintf("uv found at %s", uvPath),
	}
}

// checkHub checks C5 Hub connectivity when hub.enabled=true.
func checkHub() checkResult {
	cfgPath := filepath.Join(projectDir, ".c4", "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return checkResult{
			Name:    "C5 Hub",
			Status:  checkOK,
			Message: "skipped (no config.yaml)",
		}
	}

	content := string(data)
	// Scope "enabled: true" check to the hub: section to avoid false positives
	// from other sections (e.g. observe.enabled: true).
	if !isHubEnabled(content) {
		return checkResult{
			Name:    "C5 Hub",
			Status:  checkOK,
			Message: "hub not enabled",
		}
	}

	url := sectionYAMLValue(content, "hub", "url:")
	if url == "" {
		return checkResult{
			Name:    "C5 Hub",
			Status:  checkWarn,
			Message: "hub.enabled=true but url not configured",
		}
	}

	healthURL := strings.TrimRight(url, "/") + "/health"
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(healthURL)
	if err != nil {
		return checkResult{
			Name:    "C5 Hub",
			Status:  checkFail,
			Message: fmt.Sprintf("hub unreachable at %s: %v", url, err),
			Fix:     "Start C5: c5 serve",
		}
	}
	resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return checkResult{
			Name:    "C5 Hub",
			Status:  checkOK,
			Message: fmt.Sprintf("reachable at %s (HTTP %d)", url, resp.StatusCode),
		}
	}
	return checkResult{
		Name:    "C5 Hub",
		Status:  checkWarn,
		Message: fmt.Sprintf("hub returned HTTP %d at %s", resp.StatusCode, url),
	}
}

// checkSupabase checks Supabase connectivity when cloud is configured.
func checkSupabase() checkResult {
	cfgPath := filepath.Join(projectDir, ".c4", "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		// Try builtin URL
		if builtinSupabaseURL == "" {
			return checkResult{
				Name:    "Supabase",
				Status:  checkOK,
				Message: "skipped (no cloud config)",
			}
		}
		data = []byte{}
	}

	supabaseURL := sectionYAMLValue(string(data), "cloud", "url:")
	if supabaseURL == "" {
		supabaseURL = builtinSupabaseURL
	}
	if supabaseURL == "" || !strings.Contains(supabaseURL, "supabase") {
		return checkResult{
			Name:    "Supabase",
			Status:  checkOK,
			Message: "skipped (not configured)",
		}
	}

	healthURL := strings.TrimRight(supabaseURL, "/") + "/rest/v1/"
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(healthURL)
	if err != nil {
		return checkResult{
			Name:    "Supabase",
			Status:  checkFail,
			Message: fmt.Sprintf("unreachable at %s: %v", supabaseURL, err),
			Fix:     "Check network / Supabase project status",
		}
	}
	resp.Body.Close()
	// Supabase REST returns 200 or 401 (both indicate it's up)
	if resp.StatusCode == 200 || resp.StatusCode == 401 {
		return checkResult{
			Name:    "Supabase",
			Status:  checkOK,
			Message: fmt.Sprintf("reachable at %s", supabaseURL),
		}
	}
	return checkResult{
		Name:    "Supabase",
		Status:  checkWarn,
		Message: fmt.Sprintf("returned HTTP %d at %s", resp.StatusCode, supabaseURL),
	}
}

// tryFix attempts automatic remediation for known FAIL cases.
// Returns a short description of what was fixed, or empty string if no fix was applied.
// The caller's checkResult pointer is updated (e.g. status downgraded to WARN).
func tryFix(r *checkResult) string {
	switch r.Name {
	case "CLAUDE.md":
		claudePath := filepath.Join(projectDir, "CLAUDE.md")
		// Remove broken symlink so next init can recreate it
		if fi, err := os.Lstat(claudePath); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			if _, err := os.Stat(claudePath); os.IsNotExist(err) {
				if os.Remove(claudePath) == nil {
					return "removed broken symlink"
				}
			}
		}
	case "hooks":
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		if err := setupGlobalHooks(home); err != nil {
			return ""
		}
		return "hook updated"
	}
	return ""
}

// extractYAMLValue is a simple line-by-line YAML value extractor (key: value).
// It returns the trimmed value after the first occurrence of key on its own line.
func extractYAMLValue(content, key string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key) {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, key))
			// Strip surrounding quotes
			val = strings.Trim(val, `"'`)
			return val
		}
	}
	return ""
}

// sectionYAMLValue returns the value of key within a specific top-level YAML section.
// It scans lines looking for "section:" (no leading whitespace) and then reads
// indented keys inside that section, stopping when a new top-level key is found.
// This prevents cross-section matches (e.g. hub.url vs cloud.url).
func sectionYAMLValue(content, section, key string) string {
	lines := strings.Split(content, "\n")
	sectionHeader := section + ":"
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Top-level key detection (no leading whitespace)
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			inSection = strings.HasPrefix(trimmed, sectionHeader)
			continue
		}
		if inSection && strings.HasPrefix(trimmed, key) {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, key))
			val = strings.Trim(val, `"'`)
			return val
		}
	}
	return ""
}

// expandTilde replaces a leading ~ with the user home directory.
func expandTilde(p string) string {
	if !strings.HasPrefix(p, "~") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[1:])
}

// isHubEnabled returns true only when the hub: YAML section contains enabled: true.
// A plain strings.Contains(content, "enabled: true") produces false positives when
// other top-level sections (e.g. observe) also have enabled: true.
func isHubEnabled(content string) bool {
	lines := strings.Split(content, "\n")
	inHub := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Top-level key detection (no leading whitespace)
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			inHub = strings.HasPrefix(trimmed, "hub:")
			continue
		}
		if inHub && strings.HasPrefix(trimmed, "enabled:") {
			return strings.Contains(trimmed, "true")
		}
	}
	return false
}
