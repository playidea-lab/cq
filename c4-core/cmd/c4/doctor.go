package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/changmin/c4-core/internal/ontology"
	"github.com/changmin/c4-core/internal/standards"
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// checkStatus represents the result of a single diagnostic check.
type checkStatus string

const (
	checkOK      checkStatus = "OK"
	checkInfo    checkStatus = "INFO"
	checkWarn    checkStatus = "WARN"
	checkFail    checkStatus = "FAIL"
	checkLoading checkStatus = "Loading"
)

// checkResult holds the outcome of one diagnostic check.
type checkResult struct {
	Name    string      `json:"name"`
	Status  checkStatus `json:"status"`
	Message string      `json:"message"`
	Fix     string      `json:"fix,omitempty"`
}

// doctorCheckEntry describes a single diagnostic check with metadata.
type doctorCheckEntry struct {
	Name      string
	Fn        func() checkResult
	FixSafe   bool // true if --fix can auto-repair this check
	IsNetwork bool // true if this check requires network access
}

var (
	doctorFix   bool
	doctorJSON  bool
	doctorPlain bool
)

// doctorChecks is the ordered list of diagnostic checks run by the doctor command.
var doctorChecks = []doctorCheckEntry{
	{Name: "cq binary", Fn: checkBinary},
	{Name: ".c4 directory", Fn: checkC4Dir},
	{Name: ".mcp.json", Fn: checkMCPJson},
	{Name: "CLAUDE.md symlink", Fn: checkClaudeMDSymlink, FixSafe: true},
	{Name: "hooks", Fn: checkHooks, FixSafe: true},
	{Name: "python sidecar", Fn: checkPythonSidecar},
	{Name: "hub", Fn: checkHub, IsNetwork: true},
	{Name: "cloud", Fn: checkCloud, IsNetwork: true},
	{Name: "supabase", Fn: checkSupabase, IsNetwork: true},
	{Name: "relay", Fn: checkRelay, IsNetwork: true},
	{Name: "OS service", Fn: func() checkResult { return checkOSService(doctorFix) }, FixSafe: true},
	{Name: "stale socket", Fn: checkStaleSocket, FixSafe: true},
	{Name: "zombie serve", Fn: checkZombieServe},
	{Name: "sidecar hang", Fn: checkSidecarHang},
	{Name: "skill health", Fn: checkSkillHealth},
	{Name: "standards", Fn: checkStandards},
	{Name: "ontology L1", Fn: checkOntologyL1},
	{Name: "ontology L2", Fn: checkOntologyL2},
	{Name: "knowledge health", Fn: checkKnowledgeHealth},
	{Name: "LLM gateway", Fn: checkLLMGateway},
	{Name: "DB schema", Fn: checkDBSchema},
	{Name: "permission reviewer", Fn: checkPermissionReviewer},
	{Name: "config schema", Fn: checkConfigSchema},
	{Name: "session store", Fn: checkSessionStore},
	{Name: "telegram", Fn: checkTelegram},
	{Name: "migrations", Fn: checkMigrations},
	{Name: "serve health", Fn: checkServeHealth, IsNetwork: false},
}

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
	doctorCmd.Flags().BoolVar(&doctorPlain, "plain", false, "force plain text output (no TUI)")
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
	// TUI mode: launch async TUI when stdout is a terminal and --plain/--json are not set.
	if !doctorJSON && !doctorPlain && term.IsTerminal(int(os.Stdout.Fd())) {
		return runDoctorTUI()
	}

	// Synchronous mode: run all checks and print.
	results := make([]checkResult, 0, len(doctorChecks))
	for _, entry := range doctorChecks {
		r := entry.Fn()
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

// runDoctorTUI is implemented in doctor_tui.go.

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
		case checkInfo:
			icon = "INFO"
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

// checkHooks verifies project-level .claude/hooks/ setup and settings.json registration.
func checkHooks() checkResult {
	gateHookFile := filepath.Join(projectDir, ".claude", "hooks", "c4-gate.sh")
	permHookFile := filepath.Join(projectDir, ".claude", "hooks", "c4-permission-reviewer.sh")

	if _, err := os.Stat(gateHookFile); os.IsNotExist(err) {
		return checkResult{
			Name:    "hooks",
			Status:  checkWarn,
			Message: "c4-gate.sh not installed in .claude/hooks/",
			Fix:     "cq claude to install hooks",
		}
	}

	// Check if installed gate hook content matches embedded template
	if hookNeedsUpdate(gateHookFile, gateHookContent) {
		return checkResult{
			Name:    "hooks",
			Status:  checkWarn,
			Message: fmt.Sprintf("gate hook outdated at %s (binary has newer version)", gateHookFile),
			Fix:     "cq doctor --fix  (or: cq claude)",
		}
	}

	if _, err := os.Stat(permHookFile); os.IsNotExist(err) {
		return checkResult{
			Name:    "hooks",
			Status:  checkWarn,
			Message: "c4-permission-reviewer.sh not installed in .claude/hooks/",
			Fix:     "cq doctor --fix  (or: cq claude)",
		}
	}
	if hookNeedsUpdate(permHookFile, permissionReviewerContent) {
		return checkResult{
			Name:    "hooks",
			Status:  checkWarn,
			Message: fmt.Sprintf("permission reviewer hook outdated at %s (binary has newer version)", permHookFile),
			Fix:     "cq doctor --fix  (or: cq claude)",
		}
	}

	settingsPath := filepath.Join(projectDir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return checkResult{
			Name:    "hooks",
			Status:  checkWarn,
			Message: fmt.Sprintf("hook files found but .claude/settings.json missing: %v", err),
			Fix:     "cq claude to register hooks in settings.json",
		}
	}
	settingsStr := string(data)
	if !strings.Contains(settingsStr, "c4-gate.sh") {
		return checkResult{
			Name:    "hooks",
			Status:  checkWarn,
			Message: "gate hook exists but not registered in settings.json",
			Fix:     "cq claude to patch settings.json",
		}
	}
	if !strings.Contains(settingsStr, "c4-permission-reviewer.sh") {
		return checkResult{
			Name:    "hooks",
			Status:  checkWarn,
			Message: "permission reviewer hook exists but not registered in settings.json",
			Fix:     "cq doctor --fix  (or: cq claude)",
		}
	}
	return checkResult{
		Name:    "hooks",
		Status:  checkOK,
		Message: "gate+permission-reviewer hooks installed and registered in settings.json",
	}
}

// runWithTimeout runs cmd with a timeout and returns combined output and error.
func runWithTimeout(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// checkPythonSidecar verifies that c4-bridge is runnable via uv.
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

	_, err = runWithTimeout(5*time.Second, uvPath, "run", "c4-bridge", "--version")
	if err != nil {
		return checkResult{
			Name:    "Python sidecar",
			Status:  checkWarn,
			Message: "c4-bridge not runnable — LSP/doc tools will be unavailable",
			Fix:     "cq doctor --fix",
		}
	}

	return checkResult{
		Name:    "Python sidecar",
		Status:  checkOK,
		Message: fmt.Sprintf("c4-bridge runnable via %s", uvPath),
	}
}

// checkHub checks Hub worker queue connectivity via Supabase.
func checkHub() checkResult {
	cfgPath := filepath.Join(projectDir, ".c4", "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return checkResult{
			Name:    "Hub",
			Status:  checkOK,
			Message: "skipped (no config.yaml)",
		}
	}

	content := string(data)
	if !isHubEnabled(content) {
		return checkResult{
			Name:    "Hub",
			Status:  checkOK,
			Message: "hub not enabled",
		}
	}

	// Hub now uses Supabase directly — check Supabase reachability instead of fly.io.
	// The Supabase check is handled by checkSupabase(), so just verify hub is enabled.
	return checkResult{
		Name:    "Hub",
		Status:  checkOK,
		Message: "enabled (Supabase worker queue)",
	}
}

// checkCloud displays cloud configuration status (no HTTP calls — config display only).
func checkCloud() checkResult {
	cfgPath := filepath.Join(projectDir, ".c4", "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return checkResult{
			Name:    "Cloud",
			Status:  checkOK,
			Message: "disabled (solo tier)",
		}
	}

	content := string(data)
	enabled := sectionYAMLValue(content, "cloud", "enabled:")
	if enabled != "true" {
		return checkResult{
			Name:    "Cloud",
			Status:  checkOK,
			Message: "disabled (solo tier)",
		}
	}

	url := sectionYAMLValue(content, "cloud", "url:")
	mode := sectionYAMLValue(content, "cloud", "mode:")
	if mode == "" {
		mode = "unknown"
	}

	// Use c4.db mtime as a proxy for last sync time.
	dbPath := filepath.Join(projectDir, ".c4", "c4.db")
	lastSync := "unknown"
	if info, statErr := os.Stat(dbPath); statErr == nil {
		elapsed := time.Since(info.ModTime())
		switch {
		case elapsed < time.Minute:
			lastSync = "방금 전"
		case elapsed < time.Hour:
			lastSync = fmt.Sprintf("%d분 전", int(elapsed.Minutes()))
		case elapsed < 24*time.Hour:
			lastSync = fmt.Sprintf("%d시간 전", int(elapsed.Hours()))
		default:
			lastSync = fmt.Sprintf("%d일 전", int(elapsed.Hours()/24))
		}
	}

	msg := fmt.Sprintf("connected (%s), mode: %s, last sync: %s", url, mode, lastSync)
	return checkResult{
		Name:    "Cloud",
		Status:  checkOK,
		Message: msg,
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

// checkRelay checks relay configuration and connectivity.
func checkRelay() checkResult {
	cfgPath := filepath.Join(projectDir, ".c4", "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return checkResult{
			Name:    "relay",
			Status:  checkOK,
			Message: "skipped (no config)",
		}
	}

	enabled := sectionYAMLValue(string(data), "relay", "enabled:")
	relayURL := sectionYAMLValue(string(data), "relay", "url:")
	if enabled != "true" || relayURL == "" {
		return checkResult{
			Name:    "relay",
			Status:  checkOK,
			Message: "not configured",
			Fix:     "cq auth login to auto-configure relay",
		}
	}

	// Check relay server health
	httpURL := strings.Replace(relayURL, "wss://", "https://", 1)
	httpURL = strings.Replace(httpURL, "ws://", "http://", 1)
	healthURL := strings.TrimRight(httpURL, "/") + "/health"
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(healthURL)
	if err != nil {
		return checkResult{
			Name:    "relay",
			Status:  checkWarn,
			Message: fmt.Sprintf("server unreachable: %s", relayURL),
			Fix:     "Check network or relay server status",
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return checkResult{
			Name:    "relay",
			Status:  checkWarn,
			Message: fmt.Sprintf("server returned HTTP %d", resp.StatusCode),
		}
	}

	// Parse health response for worker count
	var health struct {
		Workers int `json:"workers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err == nil {
		return checkResult{
			Name:    "relay",
			Status:  checkOK,
			Message: fmt.Sprintf("connected (%s, %d workers)", httpURL, health.Workers),
		}
	}

	return checkResult{
		Name:    "relay",
		Status:  checkOK,
		Message: fmt.Sprintf("server reachable (%s)", httpURL),
	}
}

// checkStaleSocket checks whether .c4/tool.sock exists but is unresponsive.
func checkStaleSocket() checkResult {
	sockPath := filepath.Join(projectDir, ".c4", "tool.sock")
	if _, err := os.Stat(sockPath); os.IsNotExist(err) {
		return checkResult{Name: "tool-socket", Status: checkOK, Message: "no socket file (serve not running)"}
	}
	// Try connecting — if it fails the socket is stale
	conn, err := net.DialTimeout("unix", sockPath, time.Second)
	if err != nil {
		return checkResult{
			Name:    "tool-socket",
			Status:  checkWarn,
			Message: fmt.Sprintf("stale socket at %s (unresponsive)", sockPath),
			Fix:     fmt.Sprintf("rm %s", sockPath),
		}
	}
	conn.Close()
	return checkResult{Name: "tool-socket", Status: checkOK, Message: "responsive"}
}

// checkZombieServe detects multiple cq serve processes for the same project.
func checkZombieServe() checkResult {
	out, err := exec.Command("pgrep", "-af", "cq serve").Output()
	if err != nil {
		// pgrep returns exit 1 when no match — not an error
		return checkResult{Name: "zombie-serve", Status: checkOK, Message: "no serve processes"}
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var serving []string
	for _, l := range lines {
		if l != "" {
			serving = append(serving, l)
		}
	}
	if len(serving) <= 1 {
		return checkResult{Name: "zombie-serve", Status: checkOK, Message: fmt.Sprintf("%d serve process", len(serving))}
	}
	return checkResult{
		Name:    "zombie-serve",
		Status:  checkWarn,
		Message: fmt.Sprintf("%d cq serve processes detected (possible zombies)", len(serving)),
		Fix:     "pkill -f 'cq serve' && cq serve",
	}
}

// checkSidecarHang detects a Python sidecar process that is running but unresponsive.
func checkSidecarHang() checkResult {
	// Find sidecar PID file
	pidPath := filepath.Join(projectDir, ".c4", "sidecar.pid")
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return checkResult{Name: "sidecar", Status: checkOK, Message: "not running (lazy start)"}
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return checkResult{Name: "sidecar", Status: checkWarn, Message: "invalid PID file", Fix: fmt.Sprintf("rm %s", pidPath)}
	}
	// Check process is alive
	proc, err := os.FindProcess(pid)
	if err != nil {
		return checkResult{Name: "sidecar", Status: checkWarn, Message: fmt.Sprintf("PID %d not found, stale PID file", pid), Fix: fmt.Sprintf("rm %s", pidPath)}
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return checkResult{Name: "sidecar", Status: checkWarn, Message: fmt.Sprintf("PID %d no longer running, stale PID file", pid), Fix: fmt.Sprintf("rm %s", pidPath)}
	}
	// Process alive — check responsiveness via HTTP health endpoint
	sockPath := filepath.Join(projectDir, ".c4", "sidecar.sock")
	if _, err := os.Stat(sockPath); os.IsNotExist(err) {
		return checkResult{Name: "sidecar", Status: checkOK, Message: fmt.Sprintf("running (PID %d)", pid)}
	}
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{}).DialContext,
		},
	}
	resp, err := client.Get("http://unix/health")
	if err != nil {
		return checkResult{
			Name:    "sidecar",
			Status:  checkWarn,
			Message: fmt.Sprintf("PID %d unresponsive (possible hang)", pid),
			Fix:     fmt.Sprintf("kill %d && rm %s", pid, pidPath),
		}
	}
	resp.Body.Close()
	return checkResult{Name: "sidecar", Status: checkOK, Message: fmt.Sprintf("running and responsive (PID %d)", pid)}
}

// tryFix attempts automatic remediation for known FAIL cases.
// Returns a short description of what was fixed, or empty string if no fix was applied.
// The caller's checkResult pointer is updated (e.g. status downgraded to WARN).
func tryFix(r *checkResult) string {
	switch r.Name {
	case "CLAUDE.md":
		claudePath := filepath.Join(projectDir, "CLAUDE.md")
		// Remove broken symlink before recreating
		if fi, err := os.Lstat(claudePath); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			if _, err := os.Stat(claudePath); os.IsNotExist(err) {
				_ = os.Remove(claudePath)
			}
		}
		if err := setupClaudeMD(projectDir); err != nil {
			return ""
		}
		r.Status = checkOK
		r.Fix = ""
		return "CLAUDE.md created"
	case "hooks":
		if err := setupProjectHooks(projectDir); err != nil {
			return ""
		}
		r.Status = checkOK
		r.Fix = ""
		return "hook updated"
	case ".mcp.json":
		if err := setupMCPConfig(projectDir); err != nil {
			return ""
		}
		r.Status = checkOK
		r.Fix = ""
		return ".mcp.json generated"
	case "Hub":
		cqBin, err := os.Executable()
		if err != nil {
			return ""
		}
		cmd := exec.Command(cqBin, "auth", "login", "--device")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return ""
		}
		r.Status = checkOK
		r.Fix = ""
		return "authenticated"
	case "tool-socket":
		sockPath := filepath.Join(projectDir, ".c4", "tool.sock")
		if os.Remove(sockPath) == nil {
			r.Status = checkOK
			return "stale socket removed"
		}
	case "zombie-serve":
		if err := exec.Command("pkill", "-f", "cq serve").Run(); err == nil {
			r.Status = checkOK
			return "zombie processes killed (restart cq serve manually)"
		}
	case "sidecar":
		pidPath := filepath.Join(projectDir, ".c4", "sidecar.pid")
		if data, err := os.ReadFile(pidPath); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				_ = exec.Command("kill", strconv.Itoa(pid)).Run()
			}
		}
		_ = os.Remove(pidPath)
		r.Status = checkOK
		return "sidecar killed and PID file removed"
	case "Python sidecar":
		uvPath, err := exec.LookPath("uv")
		if err != nil {
			// uv not found — cannot auto-install, show link
			return ""
		}
		_, err = runWithTimeout(60*time.Second, uvPath, "tool", "install",
			"git+https://github.com/PlayIdea-Lab/cq")
		if err != nil {
			return ""
		}
		r.Status = checkOK
		r.Fix = ""
		return "c4-bridge installed via uv"
	case "standards":
		lock, err := standards.ReadLock(projectDir)
		if err != nil {
			return ""
		}
		result, err := standards.Apply(projectDir, lock.Team, lock.Langs, standards.ApplyOptions{})
		if err != nil {
			return ""
		}
		if len(result.FilesCreated) > 0 {
			r.Status = checkOK
			r.Fix = ""
			return fmt.Sprintf("restored %d file(s): %s", len(result.FilesCreated), strings.Join(result.FilesCreated, ", "))
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
		// Require exact key match: key followed by space, tab, or end-of-string.
		// This prevents "url:" from matching "url_extra:" (prefix collision).
		if inSection && (trimmed == key || strings.HasPrefix(trimmed, key+" ") || strings.HasPrefix(trimmed, key+"\t")) {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, key))
			// Strip inline YAML comments (# ...)
			if idx := strings.Index(val, " #"); idx >= 0 {
				val = strings.TrimSpace(val[:idx])
			}
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
func isHubEnabled(content string) bool {
	return sectionYAMLValue(content, "hub", "enabled:") == "true"
}

// checkOSService checks whether the cq-serve OS service (LaunchAgent/systemd/Windows) is installed.
// When fix=true and the service is stopped, it attempts to start the service automatically.
func checkOSService(fix bool) checkResult {
	svcConfig := newServiceConfig("", "")
	svc, err := service.New(&serviceWrapper{}, &svcConfig)
	if err != nil {
		return checkResult{
			Name:    "os-service",
			Status:  checkWarn,
			Message: fmt.Sprintf("cannot query OS service: %v", err),
			Fix:     "cq serve install",
		}
	}

	status, _ := svc.Status()
	switch status {
	case service.StatusRunning:
		return checkResult{
			Name:    "os-service",
			Status:  checkOK,
			Message: "OS service installed (running)",
		}
	case service.StatusStopped:
		if fix {
			if err := svc.Start(); err == nil {
				return checkResult{
					Name:    "os-service",
					Status:  checkOK,
					Message: "service started",
				}
			}
		}
		return checkResult{
			Name:    "os-service",
			Status:  checkWarn,
			Message: "service installed but stopped",
			Fix:     "launchctl start cq-serve  (macOS)  or  systemctl --user start cq-serve  (Linux)",
		}
	default:
		// Not installed — check for manual serve via PID file with liveness verification.
		pidDir, _ := resolveServePIDDir()
		pidPath := filepath.Join(pidDir, "serve.pid")
		if data, readErr := os.ReadFile(pidPath); readErr == nil {
			pid := strings.TrimSpace(string(data))
			if pidInt, parseErr := strconv.Atoi(pid); parseErr == nil {
				if proc, findErr := os.FindProcess(pidInt); findErr == nil && proc.Signal(syscall.Signal(0)) == nil {
					return checkResult{
						Name:    "os-service",
						Status:  checkOK,
						Message: fmt.Sprintf("serve running (pid=%s)", pid),
					}
				}
			}
			// Stale PID file — clean up silently
			os.Remove(pidPath)
		}
		return checkResult{
			Name:    "os-service",
			Status:  checkOK,
			Message: "serve not running (auto-starts on next cq claude)",
		}
	}
}

const skillHealthThreshold = 0.90

// checkSkillHealth checks C9 experiment records for skill evaluations.
// Records with name starting with "skill-eval-" are scanned from the knowledge docs dir.
// trigger_accuracy < 0.90 → WARN, no records at all → INFO (not WARN, prevents noise).
func checkSkillHealth() checkResult {
	docsDir := filepath.Join(projectDir, ".c4", "knowledge", "docs")
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		// Knowledge dir not yet initialised — not an error
		return checkResult{Name: "skill-health", Status: checkOK, Message: "knowledge store not found (skipped)"}
	}

	var warn []string
	evaluated := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "skill-eval-") || !strings.HasSuffix(name, ".md") {
			continue
		}
		skillName := strings.TrimSuffix(strings.TrimPrefix(name, "skill-eval-"), ".md")
		data, readErr := os.ReadFile(filepath.Join(docsDir, name))
		if readErr != nil {
			continue
		}
		acc, ok := parseSkillEvalAccuracy(string(data))
		if !ok {
			continue
		}
		evaluated++
		if acc < skillHealthThreshold {
			warn = append(warn, fmt.Sprintf("%s: trigger_accuracy=%.2f (< %.2f)", skillName, acc, skillHealthThreshold))
		}
	}

	if evaluated == 0 {
		return checkResult{
			Name:    "skill-health",
			Status:  checkInfo,
			Message: "no skills evaluated yet (run c4_skill_eval_run to populate)",
		}
	}
	if len(warn) > 0 {
		return checkResult{
			Name:    "skill-health",
			Status:  checkWarn,
			Message: strings.Join(warn, "; "),
			Fix:     "cq tool c4_skill_eval_run --skill=<name>",
		}
	}
	return checkResult{
		Name:    "skill-health",
		Status:  checkOK,
		Message: fmt.Sprintf("all %d evaluated skills pass trigger threshold (>= %.2f)", evaluated, skillHealthThreshold),
	}
}

// parseSkillEvalAccuracy extracts the trigger_accuracy value from a skill-eval Markdown body.
func parseSkillEvalAccuracy(content string) (float64, bool) {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "trigger_accuracy:") {
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(trimmed, "trigger_accuracy:"))
		var acc float64
		if _, scanErr := fmt.Sscanf(val, "%f", &acc); scanErr == nil {
			return acc, true
		}
	}
	return 0, false
}


// checkOntologyL1 checks the personal ontology file at ~/.c4/personas/$USER/ontology.yaml.
func checkOntologyL1() checkResult {
	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("LOGNAME")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return checkResult{Name: "ontology-l1", Status: checkWarn, Message: "cannot determine home directory"}
	}
	path := filepath.Join(home, ".c4", "personas", user, "ontology.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return checkResult{
			Name:    "ontology-l1",
			Status:  checkWarn,
			Message: fmt.Sprintf("not found: %s", path),
			Fix:     "cq tool c4_persona_learn to build personal ontology",
		}
	}
	o, err := ontology.Load(user)
	if err != nil {
		return checkResult{Name: "ontology-l1", Status: checkWarn, Message: fmt.Sprintf("cannot load: %v", err)}
	}
	nodeCount := len(o.Schema.Nodes)
	if nodeCount == 0 {
		return checkResult{
			Name:    "ontology-l1",
			Status:  checkWarn,
			Message: fmt.Sprintf("file exists (%s) but 0 nodes", path),
			Fix:     "cq tool c4_persona_learn to populate ontology",
		}
	}
	var low, high, verified int
	for _, n := range o.Schema.Nodes {
		switch n.NodeConfidence {
		case ontology.ConfidenceLow:
			low++
		case ontology.ConfidenceHigh:
			high++
		case ontology.ConfidenceVerified:
			verified++
		}
	}
	return checkResult{
		Name:    "ontology-l1",
		Status:  checkOK,
		Message: fmt.Sprintf("%d nodes (LOW=%d HIGH=%d VERIFIED=%d) — %s", nodeCount, low, high, verified, path),
	}
}

// checkOntologyL2 checks the project ontology file at .c4/project-ontology.yaml.
func checkOntologyL2() checkResult {
	path := filepath.Join(projectDir, ".c4", "project-ontology.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return checkResult{
			Name:    "ontology-l2",
			Status:  checkWarn,
			Message: fmt.Sprintf("not found: %s", path),
			Fix:     "cq tool c4_collective_sync to generate project ontology",
		}
	}
	o, err := ontology.LoadProject(projectDir)
	if err != nil {
		return checkResult{Name: "ontology-l2", Status: checkWarn, Message: fmt.Sprintf("cannot load: %v", err)}
	}
	nodeCount := len(o.Schema.Nodes)
	if nodeCount == 0 {
		return checkResult{
			Name:    "ontology-l2",
			Status:  checkWarn,
			Message: fmt.Sprintf("file exists (%s) but 0 nodes", path),
			Fix:     "cq tool c4_collective_sync to populate project ontology",
		}
	}
	return checkResult{
		Name:    "ontology-l2",
		Status:  checkOK,
		Message: fmt.Sprintf("%d nodes — %s", nodeCount, path),
	}
}

// checkKnowledgeHealth checks total knowledge docs and search hit_rate.
// hit_rate = unique docs with at least one search_hit / total docs.
func checkKnowledgeHealth() checkResult {
	docsDir := filepath.Join(projectDir, ".c4", "knowledge", "docs")
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		return checkResult{Name: "knowledge-health", Status: checkOK, Message: "knowledge store not found (skipped)"}
	}
	totalDocs := 0
	for _, e := range entries {
		if !e.IsDir() {
			totalDocs++
		}
	}
	if totalDocs == 0 {
		return checkResult{
			Name:    "knowledge-health",
			Status:  checkWarn,
			Message: "knowledge/docs/ exists but 0 documents",
			Fix:     "cq tool c4_knowledge_ingest to add documents",
		}
	}

	// Compute hit_rate via doc_usage table (unique docs with search_hit / total docs).
	dbPath := filepath.Join(projectDir, ".c4", "knowledge", "index.db")
	hitRate := -1.0
	if out, qErr := runWithTimeout(3*time.Second, "sqlite3", dbPath,
		"SELECT COUNT(DISTINCT doc_id) FROM doc_usage WHERE action='search_hit';"); qErr == nil {
		var hitDocs int
		if _, scanErr := fmt.Sscanf(strings.TrimSpace(out), "%d", &hitDocs); scanErr == nil && totalDocs > 0 {
			hitRate = float64(hitDocs) / float64(totalDocs)
		}
	}

	if hitRate < 0 {
		return checkResult{
			Name:    "knowledge-health",
			Status:  checkOK,
			Message: fmt.Sprintf("%d docs (hit_rate: unknown — index.db inaccessible)", totalDocs),
		}
	}
	return checkResult{
		Name:    "knowledge-health",
		Status:  checkOK,
		Message: fmt.Sprintf("%d docs, hit_rate=%.2f (%.0f%% of docs have been search-hit)", totalDocs, hitRate, hitRate*100),
	}
}

func checkStandards() checkResult {
	diffs, err := standards.Check(projectDir)
	if err != nil {
		return checkResult{
			Name:    "standards",
			Status:  checkInfo,
			Message: fmt.Sprintf("Standards check skipped: %v", err),
		}
	}
	if len(diffs) == 0 {
		return checkResult{
			Name:    "standards",
			Status:  checkOK,
			Message: "Standards up to date",
		}
	}
	// No lock file = standards not applied yet
	if len(diffs) == 1 && diffs[0].Status == standards.DiffMissing && diffs[0].FileName == ".piki-lock.yaml" {
		return checkResult{
			Name:    "standards",
			Status:  checkInfo,
			Message: "Standards not applied (run cq init --team <team> --lang <lang>)",
		}
	}

	var modified, missing int
	var names []string
	for _, d := range diffs {
		switch d.Status {
		case standards.DiffModified:
			modified++
			names = append(names, d.FileName+" (modified)")
		case standards.DiffMissing:
			missing++
			names = append(names, d.FileName+" (missing)")
		}
	}
	if modified == 0 && missing == 0 {
		return checkResult{
			Name:    "standards",
			Status:  checkOK,
			Message: "Standards up to date",
		}
	}
	msg := fmt.Sprintf("%d modified, %d missing: %s", modified, missing, strings.Join(names, ", "))
	return checkResult{
		Name:    "standards",
		Status:  checkWarn,
		Message: msg,
		Fix:     "run: cq init --team <team> --lang <lang>",
	}
}

// checkLLMGateway verifies LLM gateway configuration and API key availability.
func checkLLMGateway() checkResult {
	cfgPath := filepath.Join(projectDir, ".c4", "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return checkResult{
			Name:    "LLM gateway",
			Status:  checkOK,
			Message: "skipped (no config.yaml)",
		}
	}
	content := string(data)

	enabled := sectionYAMLValue(content, "llm_gateway", "enabled:")
	if enabled != "true" {
		return checkResult{
			Name:    "LLM gateway",
			Status:  checkOK,
			Message: "disabled",
		}
	}

	defaultProvider := sectionYAMLValue(content, "llm_gateway", "default:")
	if defaultProvider == "" {
		defaultProvider = "anthropic"
	}

	// Check common API key env vars based on provider
	envKeys := map[string]string{
		"anthropic": "ANTHROPIC_API_KEY",
		"openai":    "OPENAI_API_KEY",
		"google":    "GOOGLE_API_KEY",
		"gemini":    "GOOGLE_API_KEY",
	}

	var missing []string
	var present []string
	for provider, envKey := range envKeys {
		if os.Getenv(envKey) != "" {
			present = append(present, provider)
		} else if provider == defaultProvider {
			missing = append(missing, envKey)
		}
	}

	if len(missing) > 0 {
		return checkResult{
			Name:    "LLM gateway",
			Status:  checkFail,
			Message: fmt.Sprintf("default provider %q missing env: %s", defaultProvider, strings.Join(missing, ", ")),
			Fix:     fmt.Sprintf("export %s=<your-key>", missing[0]),
		}
	}

	return checkResult{
		Name:    "LLM gateway",
		Status:  checkOK,
		Message: fmt.Sprintf("enabled, default: %s (%d providers configured)", defaultProvider, len(present)),
	}
}

// checkDBSchema verifies that essential tables exist in the task database.
func checkDBSchema() checkResult {
	dbPath := ""
	for _, f := range []string{"c4.db", "tasks.db"} {
		p := filepath.Join(projectDir, ".c4", f)
		if _, err := os.Stat(p); err == nil {
			dbPath = p
			break
		}
	}
	if dbPath == "" {
		return checkResult{
			Name:    "DB schema",
			Status:  checkWarn,
			Message: "no database file found",
		}
	}

	// Use sqlite3 to check for essential tables
	requiredTables := []string{"c4_tasks", "c4_state"}
	out, err := runWithTimeout(5*time.Second, "sqlite3", dbPath,
		"SELECT name FROM sqlite_master WHERE type='table' ORDER BY name;")
	if err != nil {
		return checkResult{
			Name:    "DB schema",
			Status:  checkFail,
			Message: fmt.Sprintf("cannot query database: %v", err),
		}
	}

	tables := strings.Split(strings.TrimSpace(out), "\n")
	tableSet := make(map[string]bool, len(tables))
	for _, t := range tables {
		tableSet[strings.TrimSpace(t)] = true
	}

	var missingTables []string
	for _, req := range requiredTables {
		if !tableSet[req] {
			missingTables = append(missingTables, req)
		}
	}

	if len(missingTables) > 0 {
		return checkResult{
			Name:    "DB schema",
			Status:  checkFail,
			Message: fmt.Sprintf("missing tables: %s", strings.Join(missingTables, ", ")),
			Fix:     "cq init to re-create schema",
		}
	}

	return checkResult{
		Name:    "DB schema",
		Status:  checkOK,
		Message: fmt.Sprintf("%d tables in %s", len(tables), filepath.Base(dbPath)),
	}
}

// checkPermissionReviewer verifies permission reviewer configuration and API key.
func checkPermissionReviewer() checkResult {
	cfgPath := filepath.Join(projectDir, ".c4", "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return checkResult{
			Name:    "permission reviewer",
			Status:  checkOK,
			Message: "skipped (no config.yaml)",
		}
	}
	content := string(data)

	enabled := sectionYAMLValue(content, "permission_reviewer", "enabled:")
	if enabled != "true" {
		return checkResult{
			Name:    "permission reviewer",
			Status:  checkOK,
			Message: "disabled",
		}
	}

	model := sectionYAMLValue(content, "permission_reviewer", "model:")
	if model == "" {
		model = "haiku"
	}

	validModels := map[string]bool{
		"haiku": true, "sonnet": true, "opus": true,
		"claude-haiku-4-5-20251001": true,
		"claude-sonnet-4-5-20250514": true,
	}
	if !validModels[model] {
		return checkResult{
			Name:    "permission reviewer",
			Status:  checkWarn,
			Message: fmt.Sprintf("unknown model %q — may not work", model),
		}
	}

	apiKeyEnv := sectionYAMLValue(content, "permission_reviewer", "api_key_env:")
	if apiKeyEnv == "" {
		apiKeyEnv = "ANTHROPIC_API_KEY"
	}
	if os.Getenv(apiKeyEnv) == "" {
		return checkResult{
			Name:    "permission reviewer",
			Status:  checkFail,
			Message: fmt.Sprintf("enabled but %s not set", apiKeyEnv),
			Fix:     fmt.Sprintf("export %s=<your-key>", apiKeyEnv),
		}
	}

	autoApprove := sectionYAMLValue(content, "permission_reviewer", "auto_approve:")
	patternCount := 0
	lines := strings.Split(content, "\n")
	inSection := false
	inAllow := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			inSection = strings.HasPrefix(trimmed, "permission_reviewer:")
			inAllow = false
			continue
		}
		if inSection && strings.HasPrefix(trimmed, "allow_patterns:") {
			inAllow = true
			continue
		}
		if inSection && inAllow && strings.HasPrefix(trimmed, "- ") {
			patternCount++
			continue
		}
		if inSection && !strings.HasPrefix(trimmed, "- ") && inAllow {
			inAllow = false
		}
	}

	return checkResult{
		Name:    "permission reviewer",
		Status:  checkOK,
		Message: fmt.Sprintf("model=%s, auto_approve=%s, %d allow patterns", model, autoApprove, patternCount),
	}
}

// checkConfigSchema verifies that .c4/config.yaml has a project_id and counts known top-level sections.
func checkConfigSchema() checkResult {
	cfgPath := filepath.Join(projectDir, ".c4", "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return checkResult{
			Name:    "config schema",
			Status:  checkOK,
			Message: "skipped (no config.yaml)",
		}
	}
	content := string(data)

	// Find project_id at top level (not indented)
	projectID := ""
	for _, line := range strings.Split(content, "\n") {
		if len(line) == 0 || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "project_id:") {
			projectID = strings.TrimSpace(strings.TrimPrefix(trimmed, "project_id:"))
			projectID = strings.Trim(projectID, `"'`)
			break
		}
	}

	knownSections := []string{"cloud", "llm_gateway", "permission_reviewer", "hub", "relay", "eventbus", "worker"}
	sectionCount := 0
	for _, sec := range knownSections {
		// Check if top-level section key exists
		for _, line := range strings.Split(content, "\n") {
			if len(line) == 0 || line[0] == ' ' || line[0] == '\t' {
				continue
			}
			trimmed := strings.TrimSpace(line)
			if trimmed == sec+":" || strings.HasPrefix(trimmed, sec+":") {
				sectionCount++
				break
			}
		}
	}

	if projectID == "" {
		return checkResult{
			Name:    "config schema",
			Status:  checkWarn,
			Message: "project_id not set",
			Fix:     "add project_id: <your-project-id> to .c4/config.yaml",
		}
	}
	return checkResult{
		Name:    "config schema",
		Status:  checkOK,
		Message: fmt.Sprintf("project_id=%s, %d sections configured", projectID, sectionCount),
	}
}

// checkSessionStore verifies the named-sessions.json file integrity.
func checkSessionStore() checkResult {
	sessPath := expandTilde("~/.c4/named-sessions.json")
	data, err := os.ReadFile(sessPath)
	if os.IsNotExist(err) {
		return checkResult{
			Name:    "session store",
			Status:  checkOK,
			Message: "no sessions",
		}
	}
	if err != nil {
		return checkResult{
			Name:    "session store",
			Status:  checkWarn,
			Message: fmt.Sprintf("cannot read %s: %v", sessPath, err),
		}
	}
	var sessions map[string]any
	if err := json.Unmarshal(data, &sessions); err != nil {
		return checkResult{
			Name:    "session store",
			Status:  checkWarn,
			Message: "corrupt sessions file",
			Fix:     fmt.Sprintf("rm %s", sessPath),
		}
	}
	return checkResult{
		Name:    "session store",
		Status:  checkOK,
		Message: fmt.Sprintf("%d named sessions", len(sessions)),
	}
}

// checkTelegram verifies Telegram bot configuration and token availability.
func checkTelegram() checkResult {
	cfgPath := filepath.Join(projectDir, ".c4", "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return checkResult{
			Name:    "telegram",
			Status:  checkOK,
			Message: "not configured",
		}
	}

	// Check for top-level telegram: section (key at column 0)
	found := false
	for _, line := range strings.Split(string(data), "\n") {
		if len(line) == 0 || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "telegram:" || strings.HasPrefix(trimmed, "telegram:") {
			found = true
			break
		}
	}

	if !found {
		return checkResult{
			Name:    "telegram",
			Status:  checkOK,
			Message: "not configured",
		}
	}

	if os.Getenv("TELEGRAM_BOT_TOKEN") != "" {
		return checkResult{
			Name:    "telegram",
			Status:  checkOK,
			Message: "configured, token set",
		}
	}
	return checkResult{
		Name:    "telegram",
		Status:  checkWarn,
		Message: "configured but TELEGRAM_BOT_TOKEN not set",
		Fix:     "export TELEGRAM_BOT_TOKEN=<your-bot-token>",
	}
}

// checkMigrations counts SQL migration files in supabase/migrations/.
func checkMigrations() checkResult {
	candidates := []string{
		filepath.Join(projectDir, "supabase", "migrations", "*.sql"),
		filepath.Join(projectDir, "..", "supabase", "migrations", "*.sql"),
	}

	var files []string
	for _, pattern := range candidates {
		matches, err := filepath.Glob(pattern)
		if err == nil {
			files = append(files, matches...)
		}
	}

	if len(files) == 0 {
		return checkResult{
			Name:    "migrations",
			Status:  checkOK,
			Message: "no local migrations",
		}
	}

	sort.Strings(files)
	latest := filepath.Base(files[len(files)-1])
	return checkResult{
		Name:    "migrations",
		Status:  checkOK,
		Message: fmt.Sprintf("%d migration files, latest: %s", len(files), latest),
	}
}

func checkServeHealth() checkResult {
	components, err := fetchServeHealth(4140)
	if err != nil {
		return checkResult{
			Name:    "serve health",
			Status:  checkInfo,
			Message: "serve not running",
		}
	}

	ok, degraded, errCount, skipped := 0, 0, 0, 0
	var details []string
	for name, h := range components {
		switch h.Status {
		case "ok":
			ok++
		case "degraded":
			degraded++
		case "error":
			errCount++
		case "skipped":
			skipped++
		}
		line := fmt.Sprintf("%s: %s", name, h.Status)
		if h.Detail != "" {
			line += " (" + h.Detail + ")"
		}
		details = append(details, line)
	}
	sort.Strings(details)

	total := ok + degraded + errCount + skipped
	if degraded == 0 && errCount == 0 {
		return checkResult{
			Name:    "serve health",
			Status:  checkOK,
			Message: fmt.Sprintf("%d components healthy (ok=%d, skipped=%d)", total, ok, skipped),
		}
	}

	status := checkWarn
	if errCount > 0 {
		status = checkFail
	}
	return checkResult{
		Name:    "serve health",
		Status:  status,
		Message: fmt.Sprintf("%d components: ok=%d, degraded=%d, error=%d", total, ok, degraded, errCount),
		Fix:     strings.Join(details, "\n"),
	}
}

