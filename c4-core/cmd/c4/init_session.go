package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/mailbox"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// --- Named session support ---

type namedSessionEntry struct {
	UUID    string `json:"uuid"`
	Dir     string `json:"dir"`
	Tool    string `json:"tool,omitempty"`    // claude, codex, cursor, gemini, chatgpt
	Memo    string `json:"memo,omitempty"`    // user-defined description
	Idea    string `json:"idea,omitempty"`    // session idea / initial goal
	Status  string `json:"status,omitempty"`  // session lifecycle status (e.g., active, paused, done)
	Summary string `json:"summary,omitempty"` // auto-generated or user-set summary
	Updated string `json:"updated"`
}

var launchToolNamedFn = func(tool, projectDir, name string) error {
	return launchToolNamed(tool, projectDir, name)
}

func namedSessionsFile() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".c4", "named-sessions.json")
}

func loadNamedSessions() (map[string]namedSessionEntry, error) {
	data, err := os.ReadFile(namedSessionsFile())
	if os.IsNotExist(err) {
		return map[string]namedSessionEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]namedSessionEntry
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]namedSessionEntry{}, nil
	}
	return m, nil
}

func saveNamedSessions(m map[string]namedSessionEntry) error {
	f := namedSessionsFile()
	if err := os.MkdirAll(filepath.Dir(f), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f, data, 0600)
}

// claudeProjectDir returns the ~/.claude/projects/<encoded-path> directory for the given project.
func claudeProjectDir(projectDir string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	absDir, err := filepath.Abs(projectDir)
	if err != nil {
		return "", err
	}
	// Claude Code encodes the path: replace path separators with '-'
	encoded := strings.ReplaceAll(absDir, string(os.PathSeparator), "-")
	return filepath.Join(homeDir, ".claude", "projects", encoded), nil
}

// listJSONLNames returns a set of JSONL filenames in the given directory.
func listJSONLNames(dir string) map[string]struct{} {
	m := map[string]struct{}{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			m[e.Name()] = struct{}{}
		}
	}
	return m
}

// rebootFlagFile returns the path to the reboot-request flag file for a given session name.
// Each named session watches its own file to avoid cross-session interference.
func rebootFlagFile() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".c4", ".reboot")
}

// rebootFlagFileForSession returns a session-specific reboot flag file.
func rebootFlagFileForSession(sessionName string) string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".c4", ".reboot-"+sessionName)
}

// findGeminiSessionIndex executes 'gemini --list-sessions' and parses the output
// to find the index number corresponding to the given UUID.
func findGeminiSessionIndex(uuid string) string {
	if uuid == "" {
		return "latest"
	}
	out, err := exec.Command("gemini", "--list-sessions").Output()
	if err != nil {
		return "latest"
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, uuid) {
			// Extract index from line like "  10. some text [uuid]"
			trimmed := strings.TrimSpace(line)
			dotIdx := strings.Index(trimmed, ".")
			if dotIdx != -1 {
				return trimmed[:dotIdx]
			}
		}
	}
	return "latest"
}

func inferSessionTool(tag string, entry namedSessionEntry) string {
	if entry.Tool != "" {
		return entry.Tool
	}
	if prefix, _, ok := strings.Cut(tag, "/"); ok {
		switch prefix {
		case "claude", "codex", "cursor", "gemini", "chatgpt":
			return prefix
		}
	}
	return "claude"
}

func resumeSelectedSession(tag, tool string) error {
	if tag == "" {
		return nil
	}

	sessions, _ := loadNamedSessions()
	entry := sessions[tag]
	if entry.Status == "done" {
		entry.Status = "active"
		sessions[tag] = entry
		_ = saveNamedSessions(sessions)
	}
	if tool == "" {
		tool = inferSessionTool(tag, entry)
	}
	return launchToolNamedFn(tool, projectDir, tag)
}

// launchToolNamed starts or resumes a named AI tool session with a reboot loop.
// For claude: uses --session-id (new) or --resume (existing) with fixed UUIDs.
// For gemini: uses --resume with index-based lookup (best effort).
// Env vars CQ_SESSION_NAME and CQ_SESSION_UUID are injected into the subprocess.
func launchToolNamed(tool, projectDir, name string) error {
	// ChatGPT: open in browser (no CLI)
	if tool == "chatgpt" {
		sessions, _ := loadNamedSessions()
		// Also check provider sessions
		uuid := ""
		if entry, ok := sessions[name]; ok {
			uuid = entry.UUID
		} else {
			for _, p := range providers {
				if p.ScanSessions == nil {
					continue
				}
				if entry, ok := p.ScanSessions()[name]; ok {
					uuid = entry.UUID
					break
				}
			}
		}
		if uuid != "" {
			url := fmt.Sprintf("https://chatgpt.com/c/%s", uuid)
			fmt.Fprintf(os.Stderr, "cq: opening ChatGPT conversation in browser...\n")
			return exec.Command("open", url).Run()
		}
		return fmt.Errorf("ChatGPT session '%s' not found", name)
	}

	sessions, err := loadNamedSessions()
	if err != nil {
		return fmt.Errorf("loading named sessions: %w", err)
	}

	toolPath, err := exec.LookPath(tool)
	if err != nil {
		return fmt.Errorf("%s not found in PATH: %w", tool, err)
	}

	// Determine or create UUID for this session.
	// If name is already a UUID, use it directly for --resume (skip named session lookup).
	currentUUID := ""
	isNew := true
	if isUUID(name) {
		currentUUID = name
		isNew = false
		fmt.Fprintf(os.Stderr, "cq: using raw session UUID: %s\n", name)
	} else if entry, ok := sessions[name]; ok {
		if entry.Dir != "" && entry.Dir != projectDir {
			fmt.Fprintf(os.Stderr, "cq: session '%s' belongs to %s (current: %s), starting new session...\n",
				name, entry.Dir, projectDir)
			delete(sessions, name)
		} else {
			currentUUID = entry.UUID
			isNew = false
		}
	} else {
		// Check provider-scanned sessions (gemini/codex/chatgpt)
		for _, p := range providers {
			if p.ScanSessions == nil {
				continue
			}
			if entry, ok := p.ScanSessions()[name]; ok {
				currentUUID = entry.UUID
				isNew = false
				// Save to named-sessions for future lookups
				sessions[name] = entry
				_ = saveNamedSessions(sessions)
				break
			}
		}
	}

	// For new sessions, generate a UUID upfront (no JSONL scanning needed).
	if currentUUID == "" {
		currentUUID = uuid.New().String()
		sessions[name] = namedSessionEntry{
			UUID:    currentUUID,
			Dir:     projectDir,
			Tool:    tool,
			Updated: time.Now().Format(time.RFC3339),
		}
		if err := saveNamedSessions(sessions); err != nil {
			fmt.Fprintf(os.Stderr, "cq: warning: failed to save session: %v\n", err)
		}
	}

	// Reboot loop: re-launches the tool when session-specific reboot file exists after exit.
	sessionRebootFile := rebootFlagFileForSession(name)
	for {
		os.Remove(sessionRebootFile)
		os.Remove(rebootFlagFile()) // also clean legacy global file

		var toolArgs []string
		if isNew {
			fmt.Fprintf(os.Stderr, "cq: launching %s (session: '%s')...\n", tool, name)
			if tool == "claude" {
				toolArgs = []string{"--session-id", currentUUID, "--name", name}
			}
			if isFirstRun() {
				toolArgs = append(toolArgs, "--append-system-prompt", onboardingMsg)
				if err := markFirstRun(); err != nil {
					fmt.Fprintf(os.Stderr, "cq: warning: markFirstRun: %v\n", err)
				}
			}
			// Inject initial prompt from CQ_APPEND_PROMPT (e.g., /pi from ideas TUI)
			if appendPrompt := os.Getenv("CQ_APPEND_PROMPT"); appendPrompt != "" && tool == "claude" {
				toolArgs = append(toolArgs, "--append-system-prompt", appendPrompt)
				os.Unsetenv("CQ_APPEND_PROMPT")
			}
		} else {
			fmt.Fprintf(os.Stderr, "cq: resuming %s session '%s' (%s...)...\n", tool, name, currentUUID[:8])
			if tool == "gemini" {
				resumeID := findGeminiSessionIndex(currentUUID)
				toolArgs = []string{"--resume", resumeID}
			} else if tool == "codex" {
				// codex uses subcommand: codex resume <session_id>
				toolArgs = []string{"resume", currentUUID}
			} else if isUUID(name) {
				// Raw UUID: resume without --name (no named session to track)
				toolArgs = []string{"--resume", currentUUID}
			} else {
				toolArgs = []string{"--resume", currentUUID, "--name", name}
			}
		}

		// Attach telegram channel if configured.
		if tool == "claude" && telegramChannelConfigured() {
			toolArgs = append(toolArgs, "--channels", telegramChannelPlugin)
		}

		// Inject session context into subprocess environment.
		env := append(os.Environ(),
			"CQ_SESSION_NAME="+name,
			"CQ_SESSION_UUID="+currentUUID,
		)

		cmd := exec.Command(toolPath, toolArgs...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = env

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start %s: %w", tool, err)
		}

		// Write PID file so cq sessions can detect running sessions.
		writeSessionPID(name, cmd.Process.Pid)

		// Watch for session-specific .reboot-{name} file — only this session responds.
		rebootDetected := make(chan struct{}, 1)
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if _, err := os.Stat(sessionRebootFile); err == nil {
						select {
						case rebootDetected <- struct{}{}:
						default:
						}
						if cmd.Process != nil {
							_ = cmd.Process.Signal(os.Interrupt)
						}
						return
					}
				case <-rebootDetected:
					return
				}
			}
		}()

		runErr := cmd.Wait()

		// Check session-specific reboot flag FIRST — before resume-failure logic,
		// because SIGINT from reboot also causes non-zero exit.
		if _, err := os.Stat(sessionRebootFile); err == nil {
			os.Remove(sessionRebootFile)
			isNew = false
			fmt.Fprintf(os.Stderr, "cq: rebooting session '%s'...\n", name)
			continue
		}

		// Check legacy global reboot flag (UUID override support).
		if data, err := os.ReadFile(rebootFlagFile()); err == nil {
			os.Remove(rebootFlagFile())
			if overrideUUID := strings.TrimSpace(string(data)); overrideUUID != "" && overrideUUID != currentUUID {
				fmt.Fprintf(os.Stderr, "cq: reboot: overriding UUID → %s\n", overrideUUID[:min(8, len(overrideUUID))])
				currentUUID = overrideUUID
			}
			isNew = false
			fmt.Fprintf(os.Stderr, "cq: rebooting session '%s'...\n", name)
			continue
		}

		// If resume failed, retry as new session with --session-id.
		if runErr != nil && !isNew {
			if exitErr, ok := runErr.(*exec.ExitError); ok && exitErr.ExitCode() != 0 {
				fmt.Fprintf(os.Stderr, "cq: session '%s' resume failed, starting new session...\n", name)
				currentUUID = uuid.New().String()
				isNew = true
				sessions[name] = namedSessionEntry{
					UUID:    currentUUID,
					Dir:     projectDir,
					Tool:    tool,
					Updated: time.Now().Format(time.RFC3339),
				}
				_ = saveNamedSessions(sessions)
				continue
			}
		}

		// After first successful run, future iterations are resumes.
		isNew = false

		// Remove PID file and capture session status before exiting (best-effort).
		removeSessionPID(name)
		if !isUUID(name) {
			// Check for .done marker: /done → full capture, /exit → light capture
			doneMarker := filepath.Join(sessionPIDDir(), name+".done")
			if _, err := os.Stat(doneMarker); err == nil {
				os.Remove(doneMarker)
				captureSessionFull(name)
			} else {
				captureSessionLight(name)
			}
		}
		break
	}

	return nil
}

// currentSessionUUID detects the current Claude Code session UUID.
// Priority: CQ_SESSION_UUID env var → JSONL content timestamp → file ModTime.
// Walks up parent directories to find the correct Claude project JSONL dir.
func currentSessionUUID(dir string) string {
	// 1. Prefer env var (set by cq claude -t).
	if uuid := os.Getenv("CQ_SESSION_UUID"); uuid != "" {
		return uuid
	}

	// 2. Try dir and parent directories (handles subdirectory execution).
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	var sessionDir string
	for d := absDir; d != filepath.Dir(d); d = filepath.Dir(d) {
		candidate, err := claudeProjectDir(d)
		if err != nil {
			continue
		}
		entries, _ := os.ReadDir(candidate)
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".jsonl") {
				sessionDir = candidate
				break
			}
		}
		if sessionDir != "" {
			break
		}
	}
	if sessionDir == "" {
		return ""
	}
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return ""
	}

	type candidate struct {
		uuid      string
		timestamp time.Time // from JSONL content
		modTime   time.Time // file system fallback
	}
	var best candidate

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		uuid := strings.TrimSuffix(e.Name(), ".jsonl")
		ts := jsonlLastTimestamp(filepath.Join(sessionDir, e.Name()))
		c := candidate{uuid: uuid, timestamp: ts, modTime: info.ModTime()}

		// Prefer the candidate with the most recent JSONL content timestamp.
		// Fall back to modTime when timestamps are equal or unavailable.
		var bestTs, cTs time.Time
		if !best.timestamp.IsZero() {
			bestTs = best.timestamp
		} else {
			bestTs = best.modTime
		}
		if !c.timestamp.IsZero() {
			cTs = c.timestamp
		} else {
			cTs = c.modTime
		}
		if cTs.After(bestTs) {
			best = c
		}
	}
	return best.uuid
}

// jsonlLastTimestamp reads the last JSON record from a JSONL file and returns
// its "timestamp" field. Returns zero time if unreadable or not present.
func jsonlLastTimestamp(path string) time.Time {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}
	}
	defer f.Close()

	// Seek to last 4KB to find the last complete line efficiently.
	const tailSize = 4096
	if fi, err := f.Stat(); err == nil && fi.Size() > tailSize {
		_, _ = f.Seek(-tailSize, io.SeekEnd)
	}
	buf, err := io.ReadAll(f)
	if err != nil || len(buf) == 0 {
		return time.Time{}
	}
	// Find the last non-empty line.
	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var rec struct {
			Timestamp string `json:"timestamp"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err == nil && rec.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339Nano, rec.Timestamp); err == nil {
				return t
			}
			if t, err := time.Parse(time.RFC3339, rec.Timestamp); err == nil {
				return t
			}
		}
		break
	}
	return time.Time{}
}

var sessionsPlain bool

// sessionsCmd lists named sessions with enhanced status-aware formatting.
// Active sessions (idea/planned/active) get full 3-4 line format.
// Done sessions appear in compact form below a separator (max 5 recent).
// When stdout is a TTY and --plain is not set, opens an interactive TUI dashboard.
var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "List named Claude Code sessions (tmux-style)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		isTerminal := term.IsTerminal(int(os.Stdout.Fd()))
		if !sessionsPlain && isTerminal {
			tag, tool, err := runSessionsTUI()
			if err != nil {
				return err
			}
			return resumeSelectedSession(tag, tool)
		}

		sessions, err := loadNamedSessions()
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			fmt.Println("No named sessions. Use 'cq claude -t <name>' to create one.")
			return nil
		}
		recalcStatuses(sessions)
		// Detect current session UUID: prefer env var, fall back to filesystem.
		curUUID := os.Getenv("CQ_SESSION_UUID")
		if curUUID == "" {
			curUUID = currentSessionUUID(projectDir)
		}
		// Open mailbox for unread counts (best-effort; errors silently skipped).
		var ms *mailbox.MailStore
		if homeDir, hErr := os.UserHomeDir(); hErr == nil {
			if store, msErr := mailbox.NewMailStore(filepath.Join(homeDir, ".c4", "mailbox.db")); msErr == nil {
				ms = store
				defer ms.Close()
			}
		}
		// Sort names for stable output
		names := make([]string, 0, len(sessions))
		for n := range sessions {
			names = append(names, n)
		}
		for i := 0; i < len(names)-1; i++ {
			for j := i + 1; j < len(names); j++ {
				if names[i] > names[j] {
					names[i], names[j] = names[j], names[i]
				}
			}
		}

		// Separate active vs done sessions.
		var activeNames, doneNames []string
		for _, n := range names {
			if sessions[n].Status == "done" {
				doneNames = append(doneNames, n)
			} else {
				activeNames = append(activeNames, n)
			}
		}

		const reset = "\033[0m"
		activeCurUUID := curUUID

		// Compute max name width for column alignment.
		maxNameW := 8
		for _, n := range names {
			if w := lsDispWidth(n); w > maxNameW {
				maxNameW = w
			}
		}

		// --- Active sessions: full format ---
		counts := map[string]int{"idea": 0, "planned": 0, "active": 0, "done": 0}
		for i, n := range activeNames {
			entry := sessions[n]
			t, tErr := time.Parse(time.RFC3339, entry.Updated)
			dateStr := "--"
			if tErr == nil {
				dateStr = t.Format("Jan 02 15:04")
			}
			isCurrent := activeCurUUID != "" && entry.UUID == activeCurUUID
			if isCurrent {
				activeCurUUID = ""
			}
			indicator := "  "
			if isCurrent {
				indicator = "● "
			}
			statusStr := entry.Status
			if statusStr == "" {
				statusStr = "active"
			}
			color := statusColorCode(statusStr)
			counts[statusStr]++

			extra := ""
			if ms != nil {
				if count, mErr := ms.UnreadCount(n); mErr == nil && count > 0 {
					extra = fmt.Sprintf("  ✉%d", count)
				}
			}

			// Line 1: indicator | name | uuid[:8] | status(colored) | date
			statusFmt := statusStr
			if color != "" {
				statusFmt = color + statusStr + reset
			}
			fmt.Printf("%s%s  %s  %s  %s%s\n",
				indicator,
				lsPadToWidth(n, maxNameW),
				entry.UUID[:8],
				lsPadToWidth(statusFmt, 14),
				dateStr,
				extra)
			// Line 2: summary
			if entry.Summary != "" {
				fmt.Printf("    %s\n", entry.Summary)
			}
			// Lines 3-4: idea path / spec path (if present)
			ideaSlug := entry.Idea
			if ideaSlug == "" {
				ideaSlug, _ = matchIdeaByTag(n)
			}
			if ideaSlug != "" {
				ideaPath := filepath.Join(projectDir, ".c4", "ideas", ideaSlug+".md")
				if _, statErr := os.Stat(ideaPath); statErr == nil {
					fmt.Printf("    ├─ 💡 %s\n", ideaPath)
				}
			}
			// Blank line between sessions (not after last active if done list follows).
			if i < len(activeNames)-1 || len(doneNames) > 0 {
				fmt.Println()
			}
		}

		// --- Done sessions: compact, max 5 recent ---
		if len(doneNames) > 0 {
			counts["done"] = len(doneNames)
			fmt.Printf("\033[90m── done (%d) ──\033[0m\n", len(doneNames))
			// Show at most 5 most recent done sessions (names are sorted alphabetically;
			// for recency we just take the last 5 in sorted order as best-effort).
			start := 0
			if len(doneNames) > 5 {
				start = len(doneNames) - 5
			}
			for _, n := range doneNames[start:] {
				entry := sessions[n]
				t, tErr := time.Parse(time.RFC3339, entry.Updated)
				dateStr := "--"
				if tErr == nil {
					dateStr = t.Format("Jan 02 15:04")
				}
				summary := entry.Summary
				if len(summary) > 40 {
					summary = summary[:40] + "…"
				}
				fmt.Printf("\033[90m  %-20s  %s  %s\033[0m\n", n, dateStr, summary)
			}
		}

		// --- Bottom summary ---
		total := len(sessions)
		fmt.Printf("\n%d sessions", total)
		parts := []string{}
		for _, k := range []string{"idea", "planned", "active", "done"} {
			if counts[k] > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", counts[k], k))
			}
		}
		if len(parts) > 0 {
			fmt.Printf(" — %s", strings.Join(parts, ", "))
		}
		fmt.Println()

		return nil
	},
}

// Note: lsCmd is now defined in bot.go (lists bots).
// sessionsCmd above replaces the old lsCmd for session listing.

// lsIsWide reports whether rune r occupies 2 terminal columns (CJK, Hangul, etc.).
func lsIsWide(r rune) bool {
	return (r >= 0x1100 && r <= 0x115F) || // Hangul Jamo
		(r >= 0x2E80 && r <= 0x303E) || // CJK Radicals, Kangxi
		(r >= 0x3040 && r <= 0xA4CF) || // Hiragana/Katakana/CJK Unified
		(r >= 0xAC00 && r <= 0xD7A3) || // Hangul Syllables
		(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility
		(r >= 0xFE10 && r <= 0xFE1F) ||
		(r >= 0xFE30 && r <= 0xFE4F) ||
		(r >= 0xFF00 && r <= 0xFF60) || // Fullwidth forms
		(r >= 0xFFE0 && r <= 0xFFE6) ||
		(r >= 0x1F300 && r <= 0x1F64F) || // Emoji
		(r >= 0x20000 && r <= 0x2FFFD) || // CJK Extension B+
		(r >= 0x30000 && r <= 0x3FFFD)
}

// lsDispWidth returns the terminal display width of s.
func lsDispWidth(s string) int {
	w := 0
	for _, r := range s {
		if lsIsWide(r) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

// lsPadToWidth pads s with spaces until its display width equals width.
func lsPadToWidth(s string, width int) string {
	w := lsDispWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// lsTruncateToWidth truncates s so that its display width does not exceed maxW.
func lsTruncateToWidth(s string, maxW int) string {
	w := 0
	for i, r := range s {
		rw := 1
		if lsIsWide(r) {
			rw = 2
		}
		if w+rw > maxW {
			return s[:i]
		}
		w += rw
	}
	return s
}

// sessionCmd provides session management subcommands.
var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage named Claude Code sessions",
}

var sessionNameForce bool
var sessionNameMemo string
var sessionNameUUID string

var sessionNameCmd = &cobra.Command{
	Use:   "name <session-name>",
	Short: "Attach a name to the current session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		uuid := sessionNameUUID
		if uuid == "" {
			uuid = currentSessionUUID(projectDir)
		}
		if uuid == "" {
			return fmt.Errorf("could not detect current session UUID (no JSONL files found)")
		}
		sessions, err := loadNamedSessions()
		if err != nil {
			return err
		}
		// Conflict check: name already used by a different session.
		if existing, ok := sessions[name]; ok && existing.UUID != uuid {
			if !sessionNameForce {
				fmt.Printf("session '%s' already exists (uuid=%s...)\n", name, existing.UUID[:8])
				fmt.Printf("overwrite? [y/N] ")
				var answer string
				fmt.Fscan(cmd.InOrStdin(), &answer)
				if answer != "y" && answer != "Y" {
					fmt.Println("aborted")
					return nil
				}
			}
		}
		// Preserve memo/tool from existing entry for the same UUID (rename).
		// Delete ALL entries pointing to this UUID to avoid duplicate aliases.
		var prevMemo, prevTool string
		for k, v := range sessions {
			if v.UUID == uuid {
				if prevMemo == "" {
					prevMemo = v.Memo
				}
				if prevTool == "" {
					prevTool = v.Tool
				}
				delete(sessions, k)
			}
		}
		if sessionNameMemo != "" {
			prevMemo = sessionNameMemo
		}
		// Infer tool from environment if not already known.
		if prevTool == "" {
			if os.Getenv("CQ_SESSION_UUID") != "" || os.Getenv("CQ_SESSION_NAME") != "" {
				prevTool = "claude"
			}
		}
		sessions[name] = namedSessionEntry{
			UUID:    uuid,
			Dir:     projectDir,
			Tool:    prevTool,
			Memo:    prevMemo,
			Updated: time.Now().Format(time.RFC3339),
		}
		if err := saveNamedSessions(sessions); err != nil {
			return err
		}
		fmt.Printf("session '%s' → %s...\n", name, uuid[:8])
		fmt.Printf("Next time: cq claude -t %s\n", name)
		return nil
	},
}

var sessionMemoCmd = &cobra.Command{
	Use:   "memo <session-name> <text>",
	Short: "Set or update the memo for a named session",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, memo := args[0], args[1]
		sessions, err := loadNamedSessions()
		if err != nil {
			return err
		}
		entry, ok := sessions[name]
		if !ok {
			return fmt.Errorf("session '%s' not found", name)
		}
		entry.Memo = memo
		sessions[name] = entry
		if err := saveNamedSessions(sessions); err != nil {
			return err
		}
		fmt.Printf("session '%s' memo updated\n", name)
		return nil
	},
}

func init() {
	sessionsCmd.Flags().BoolVar(&sessionsPlain, "plain", false, "plain text output (no TUI)")
	sessionNameCmd.Flags().BoolVarP(&sessionNameForce, "force", "f", false, "overwrite existing session name without confirmation")
	sessionNameCmd.Flags().StringVarP(&sessionNameMemo, "memo", "m", "", "short description of this session")
	sessionNameCmd.Flags().StringVar(&sessionNameUUID, "uuid", "", "explicitly set session UUID (bypass auto-detection)")
}

// writeSessionDoneMarker creates a .done marker file for the given session name.
// Called by the /done tool so the exit handler knows to run full capture.
func writeSessionDoneMarker(name string) {
	dir := sessionPIDDir()
	_ = os.MkdirAll(dir, 0755)
	_ = os.WriteFile(filepath.Join(dir, name+".done"), []byte("done"), 0600)
}

var sessionRmCmd = &cobra.Command{
	Use:   "rm <session-name>",
	Short: "Remove a named session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		sessions, err := loadNamedSessions()
		if err != nil {
			return err
		}
		if _, ok := sessions[name]; !ok {
			return fmt.Errorf("session '%s' not found", name)
		}
		delete(sessions, name)
		if err := saveNamedSessions(sessions); err != nil {
			return err
		}
		fmt.Printf("session '%s' removed\n", name)
		return nil
	},
}
