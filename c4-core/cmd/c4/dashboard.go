package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"

	tea "github.com/charmbracelet/bubbletea"
)

// --- Custom messages for dashboard actions ---

type launchToolMsg struct{ tool string }
type changeConfigMsg struct{}
type openSessionsMsg struct{}
type openDoctorMsg struct{}

// --- Dashboard TUI Model ---

// dashboardRow represents a single info row in the dashboard.
type dashboardRow struct {
	label string
	value string
	badge string // optional: status badge key (for statusBadgeStyles)
}

// componentRow holds a single service component's health for detail view.
type componentRow struct {
	name   string
	status string // "ok", "degraded", "error", "skipped"
	detail string
}

// toolChangelog holds cached changelog for the user's default tool.
type toolChangelog struct {
	ToolName string   `json:"tool_name" yaml:"tool_name"`
	Version  string   `json:"version" yaml:"version"`
	Items    []string `json:"items" yaml:"items"` // bullet point features
}

type dashboardModel struct {
	version     string
	rows        []dashboardRow
	components  []componentRow  // service health details
	cmdRows     []cmdRow        // command reference rows
	cmdScroll   int             // scroll offset for commands section
	changelog   *toolChangelog  // tool changelog (nil if unavailable)
	whatsNew    string          // "New in vX.Y.Z: ..." or ""
	defaultTool string
	action      string // "launch", "status", "config", or "" (quit)
	showDetail  bool   // toggle: show component details

	width  int
	height int
}

// cmdRow is a single row in the commands reference (header or command).
type cmdRow struct {
	isHeader bool
	category string // header label
	name     string
	desc     string
}

// buildCommandRows returns the full command reference with category headers.
func buildCommandRows() []cmdRow {
	return []cmdRow{
		// CLI Commands
		{isHeader: true, category: "CLI"},
		{name: "cq claude", desc: "Claude Code 시작"},
		{name: "cq cursor", desc: "Cursor 시작"},
		{name: "cq codex", desc: "Codex CLI 시작"},
		{name: "cq gemini", desc: "Gemini CLI 시작"},
		{name: "cq -t <name>", desc: "이름 붙인 세션 시작/이어가기"},
		{name: "cq sessions", desc: "세션 목록 관리"},
		{name: "cq status", desc: "서비스 + 프로젝트 상태"},
		{name: "cq doctor", desc: "설치 환경 진단"},
		{name: "cq update", desc: "CQ 최신 버전 업데이트"},
		{name: "cq stop", desc: "CQ 서비스 중지"},

		// Slash Commands (Skills)
		{isHeader: true, category: "Slash Commands"},
		{name: "/pi", desc: "아이디어 발산·수렴 (plan 이전 단계)"},
		{name: "/plan", desc: "구조화된 구현 계획 생성"},
		{name: "/run", desc: "워커 스폰, 태스크 병렬 실행"},
		{name: "/finish", desc: "품질 수렴 + 빌드 + 커밋"},
		{name: "/quick", desc: "태스크 1개 빠른 실행"},
		{name: "/status", desc: "프로젝트 상태 + 태스크 그래프"},
		{name: "/review", desc: "6축 코드 리뷰"},
		{name: "/help", desc: "스킬/에이전트/도구 레퍼런스"},
		{name: "/attach", desc: "현재 세션에 이름 붙이기"},
		{name: "/reboot", desc: "세션 재시작"},
		{name: "/release", desc: "릴리스 노트 + 태그 생성"},
		{name: "/simplify", desc: "변경 코드 품질·효율 점검"},

		// MCP Tools (Core)
		{isHeader: true, category: "MCP Tools — 태스크"},
		{name: "cq_status", desc: "프로젝트 상태 조회"},
		{name: "cq_add_todo", desc: "태스크 추가"},
		{name: "cq_get_task", desc: "워커에 태스크 할당"},
		{name: "cq_submit", desc: "태스크 완료 제출"},
		{name: "cq_claim / cq_report", desc: "Direct 모드 태스크 수행"},
		{name: "cq_task_list", desc: "태스크 목록 필터링"},
		{name: "cq_start", desc: "EXECUTE 상태로 전환"},

		// MCP Tools (Knowledge)
		{isHeader: true, category: "MCP Tools — 지식"},
		{name: "cq_knowledge_search", desc: "지식 베이스 검색"},
		{name: "cq_knowledge_record", desc: "지식 문서 저장"},
		{name: "cq_save_spec", desc: "EARS 스펙 저장"},
		{name: "cq_save_design", desc: "설계 문서 저장"},
		{name: "cq_lighthouse", desc: "API 계약 관리 (TDD)"},

		// MCP Tools (Infra)
		{isHeader: true, category: "MCP Tools — 인프라"},
		{name: "cq_read_file", desc: "파일 읽기"},
		{name: "cq_find_file", desc: "파일 검색"},
		{name: "cq_search_for_pattern", desc: "코드 패턴 검색"},
		{name: "cq_run_validation", desc: "lint/test 실행"},
		{name: "cq_notify", desc: "알림 전송 (텔레그램 등)"},
		{name: "cq_relay_call", desc: "원격 서버 MCP 호출"},
		{name: "cq_llm_call", desc: "LLM Gateway 호출"},
	}
}

// cqLogo is the dot-art CQ mark.
var cqLogo = []string{
	" ▄▀▀▀▀▄  ▄▀▀▀▀▄ ",
	" █      ██    ██ ",
	" █      ██  ▄ ██ ",
	" ▀▄▄▄▄▀  ▀▄▄▀▄▀ ",
}

func newDashboardModel() dashboardModel {
	m := dashboardModel{
		version:     version,
		defaultTool: readGlobalConfig("default_tool"),
	}
	if m.defaultTool == "" {
		m.defaultTool = "claude"
	}

	// User info
	if ac, err := newAuthClient(); err == nil {
		if sess, err := ac.GetSession(); err == nil && sess != nil && sess.User.Name != "" {
			m.rows = append(m.rows, dashboardRow{
				label: "User",
				value: sess.User.Name,
			})
		}
	}

	// Service health row + component details
	components, err := fetchServeHealth(servePort)
	if err != nil {
		m.rows = append(m.rows, dashboardRow{
			label: "Service",
			value: "starting...",
		})
	} else {
		okCount := 0
		for name, h := range components {
			if h.Status == "ok" {
				okCount++
			}
			m.components = append(m.components, componentRow{
				name:   name,
				status: h.Status,
				detail: h.Detail,
			})
		}
		badge := "active"
		if okCount == len(components) {
			badge = "running"
		}
		m.rows = append(m.rows, dashboardRow{
			label: "Service",
			value: fmt.Sprintf("%d/%d components", okCount, len(components)),
			badge: badge,
		})
		sort.Slice(m.components, func(i, j int) bool {
			return m.components[i].name < m.components[j].name
		})
	}

	// Project status row (if inside a .c4/ project)
	c4Dir := filepath.Join(projectDir, ".c4")
	if info, err := os.Stat(c4Dir); err == nil && info.IsDir() {
		name, phase := readProjectState()
		badge := "active"
		switch strings.ToLower(phase) {
		case "execute":
			badge = "active"
		case "plan", "design", "discovery":
			badge = "planned"
		case "complete":
			badge = "done"
		}
		m.rows = append(m.rows, dashboardRow{
			label: "Project",
			value: name,
			badge: badge,
		})
	}

	// Sessions summary
	if sessions, err := loadNamedSessions(); err == nil && len(sessions) > 0 {
		activeCount := 0
		var recentTag, recentDate string
		for tag, entry := range sessions {
			if entry.Status == "" || entry.Status == "active" || entry.Status == "running" {
				activeCount++
			}
			if entry.Updated > recentDate {
				recentDate = entry.Updated
				recentTag = tag
			}
		}
		m.rows = append(m.rows, dashboardRow{
			label: "Sessions",
			value: fmt.Sprintf("%d active / %d total", activeCount, len(sessions)),
		})
		if recentTag != "" {
			m.rows = append(m.rows, dashboardRow{
				label: "Recent",
				value: recentTag,
			})
		}
	}

	// Default tool row
	m.rows = append(m.rows, dashboardRow{
		label: "Tool",
		value: m.defaultTool,
	})

	// What's New check
	lastSeen := readGlobalConfig("last_seen_version")
	if lastSeen != "" && lastSeen != version && version != "dev" {
		m.whatsNew = fmt.Sprintf("✨ New in %s", version)
	}

	// Tool changelog (cached, fetched on version change)
	m.changelog = loadToolChangelog(m.defaultTool)

	// Command reference
	m.cmdRows = buildCommandRows()

	return m
}

// readProjectState reads project name and phase from .c4/state.yaml.
func readProjectState() (name, phase string) {
	statePath := filepath.Join(projectDir, ".c4", "state.yaml")
	data, err := os.ReadFile(statePath)
	if err != nil {
		return filepath.Base(projectDir), "idle"
	}
	var state map[string]interface{}
	if err := yaml.Unmarshal(data, &state); err != nil {
		return filepath.Base(projectDir), "idle"
	}
	name, _ = state["project_id"].(string)
	if name == "" {
		name = filepath.Base(projectDir)
	}
	phase, _ = state["phase"].(string)
	if phase == "" {
		phase = "idle"
	}
	return
}

// cmdVisibleLines returns how many command rows fit in the viewport.
func (m dashboardModel) cmdVisibleLines() int {
	// Estimate: total height minus logo(5) + gap(1) + version(1) + gap(1)
	// + info rows + changelog(~7) + cmd header(1) + help bar(2) + margins
	used := 10 + len(m.rows)
	if m.showDetail {
		used += len(m.components)
	}
	if m.changelog != nil && len(m.changelog.Items) > 0 {
		used += 7
	}
	if m.whatsNew != "" {
		used += 2
	}
	visible := m.height - used
	if visible < 5 {
		visible = 5
	}
	if visible > len(m.cmdRows) {
		visible = len(m.cmdRows)
	}
	return visible
}

func (m dashboardModel) Init() tea.Cmd {
	return nil
}

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case launchToolMsg:
		m.action = "launch"
		return m, tea.Quit
	case changeConfigMsg:
		m.action = "config"
		return m, tea.Quit
	case openSessionsMsg:
		m.action = "sessions"
		return m, tea.Quit
	case openDoctorMsg:
		m.action = "doctor"
		return m, tea.Quit
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEnter:
			return m, func() tea.Msg { return launchToolMsg{tool: m.defaultTool} }
		case tea.KeyUp:
			if m.cmdScroll > 0 {
				m.cmdScroll--
			}
		case tea.KeyDown:
			maxScroll := len(m.cmdRows) - m.cmdVisibleLines()
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.cmdScroll < maxScroll {
				m.cmdScroll++
			}
		case tea.KeyRunes:
			switch string(msg.Runes) {
			case "q":
				return m, tea.Quit
			case "s":
				m.showDetail = !m.showDetail
			case "c":
				return m, func() tea.Msg { return changeConfigMsg{} }
			case "t":
				return m, func() tea.Msg { return openSessionsMsg{} }
			case "d":
				return m, func() tea.Msg { return openDoctorMsg{} }
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m dashboardModel) View() string {
	var sb strings.Builder

	// CQ Logo
	logoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)
	for _, line := range cqLogo {
		sb.WriteString(logoStyle.Render("  " + line))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	sb.WriteString("  ")
	sb.WriteString(styleTitle.Render(fmt.Sprintf(" %s ", m.version)))
	sb.WriteString("\n\n")

	// Info rows with aligned labels + optional status badges
	const labelW = 10
	for _, row := range m.rows {
		label := fmt.Sprintf("  %-*s", labelW, row.label)
		sb.WriteString(styleDate.Render(label))

		if row.badge != "" {
			bStyle, ok := statusBadgeStyles[row.badge]
			if !ok {
				bStyle = statusBadgeStyles["active"]
			}
			// Pad badge text to fixed 11-char width (same as sessions)
			badgeText := row.badge
			padTotal := 11 - len(badgeText)
			if padTotal > 0 {
				padLeft := padTotal / 2
				padRight := padTotal - padLeft
				badgeText = strings.Repeat(" ", padLeft) + badgeText + strings.Repeat(" ", padRight)
			}
			sb.WriteString(bStyle.Render(badgeText))
			sb.WriteString(" ")
		}

		sb.WriteString(styleTagName.Render(row.value))
		sb.WriteString("\n")

		// Component detail expansion (below Service row)
		if row.label == "Service" && m.showDetail && len(m.components) > 0 {
			for i, c := range m.components {
				branch := "├─"
				if i == len(m.components)-1 {
					branch = "└─"
				}
				icon := "✓"
				cStyle := styleFilePath
				if c.status != "ok" {
					icon = "✗"
					cStyle = styleConfirm
				}
				line := fmt.Sprintf("      %s %s %-20s %s", branch, icon, c.name, c.status)
				if c.detail != "" {
					line += fmt.Sprintf(" (%s)", c.detail)
				}
				sb.WriteString(cStyle.Render(line))
				sb.WriteString("\n")
			}
		}
	}

	// What's New (CQ version)
	if m.whatsNew != "" {
		sb.WriteString("\n")
		hs := groupHeaderStyle("idea")
		sb.WriteString(hs.Render("  " + m.whatsNew))
		sb.WriteString("\n")
	}

	// Tool changelog
	if m.changelog != nil && len(m.changelog.Items) > 0 {
		sb.WriteString("\n")
		header := fmt.Sprintf(" ── %s %s ", m.changelog.ToolName, m.changelog.Version)
		hs := groupHeaderStyle("active")
		sb.WriteString(hs.Render(header))
		headerW := m.width
		if headerW < 74 {
			headerW = 74
		}
		remaining := headerW - lipgloss.Width(header)
		if remaining > 0 {
			sb.WriteString(styleFaint.Render(strings.Repeat("─", remaining)))
		}
		sb.WriteString("\n")
		maxItems := 5
		if len(m.changelog.Items) < maxItems {
			maxItems = len(m.changelog.Items)
		}
		for _, item := range m.changelog.Items[:maxItems] {
			sb.WriteString(styleSummary.Render("  • " + item))
			sb.WriteString("\n")
		}
	}

	// Commands reference (scrollable)
	sb.WriteString("\n")
	visible := m.cmdVisibleLines()
	end := m.cmdScroll + visible
	if end > len(m.cmdRows) {
		end = len(m.cmdRows)
	}

	if m.cmdScroll > 0 {
		sb.WriteString(styleFaint.Render("  ▲ more"))
		sb.WriteString("\n")
	}

	for _, row := range m.cmdRows[m.cmdScroll:end] {
		if row.isHeader {
			header := fmt.Sprintf(" ── %s ", row.category)
			hs2 := groupHeaderStyle("active")
			sb.WriteString(hs2.Render(header))
			hdrW := m.width
			if hdrW < 74 {
				hdrW = 74
			}
			rem := hdrW - lipgloss.Width(header)
			if rem > 0 {
				sb.WriteString(styleFaint.Render(strings.Repeat("─", rem)))
			}
			sb.WriteString("\n")
		} else {
			sb.WriteString(styleHelpKey.Render(fmt.Sprintf("  %-24s", row.name)))
			sb.WriteString(styleFaint.Render(row.desc))
			sb.WriteString("\n")
		}
	}

	if end < len(m.cmdRows) {
		sb.WriteString(styleFaint.Render("  ▼ more"))
		sb.WriteString("\n")
	}

	// Build help bar
	var helpBar strings.Builder
	helpBar.WriteString(" ")
	helpBar.WriteString(helpEntry("Enter", m.defaultTool+" 시작"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("↑↓", "스크롤"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("s", "상태"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("t", "sessions"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("d", "doctor"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("c", "설정"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("q", "종료"))

	// Pin help bar at bottom — same as cq sessions
	content := sb.String()
	contentLines := strings.Count(content, "\n")
	if m.height > 0 {
		gap := m.height - contentLines - 2
		for i := 0; i < gap; i++ {
			sb.WriteString("\n")
		}
	}
	if m.width > 0 {
		sb.WriteString(styleFaint.Render(strings.Repeat("─", m.width)))
	} else {
		sb.WriteString(styleFaint.Render(strings.Repeat("─", 74)))
	}
	sb.WriteString("\n")
	sb.WriteString(helpBar.String())

	return sb.String()
}

// --- Tool changelog: fetch + cache ---

// toolGitHubRepo maps tool names to their GitHub repos for changelog fetching.
var toolGitHubRepo = map[string]string{
	"claude": "anthropics/claude-code",
	"codex":  "openai/codex",
	"gemini": "google-gemini/gemini-cli",
}

func changelogCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".c4", "cache", "tool-changelog.json")
}

// loadToolChangelog returns cached changelog, refreshing if tool version changed.
func loadToolChangelog(tool string) *toolChangelog {
	// Get current tool version
	currentVer := getToolVersion(tool)
	if currentVer == "" {
		return nil
	}

	// Read cache
	cachePath := changelogCachePath()
	if cachePath == "" {
		return nil
	}
	if data, err := os.ReadFile(cachePath); err == nil {
		var cached toolChangelog
		if json.Unmarshal(data, &cached) == nil {
			if cached.ToolName == tool && cached.Version == currentVer {
				return &cached // cache hit
			}
		}
	}

	// Cache miss — fetch in background-safe way (with short timeout)
	cl := fetchToolChangelog(tool, currentVer)
	if cl == nil {
		return nil
	}

	// Save cache
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err == nil {
		if data, err := json.Marshal(cl); err == nil {
			_ = os.WriteFile(cachePath, data, 0644)
		}
	}
	return cl
}

// getToolVersion runs `tool --version` and returns the version string.
func getToolVersion(tool string) string {
	toolPath, err := exec.LookPath(tool)
	if err != nil || toolPath == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, tool, "--version").Output()
	if err != nil {
		return ""
	}
	ver := strings.TrimSpace(string(out))
	if idx := strings.IndexByte(ver, '\n'); idx > 0 {
		ver = ver[:idx]
	}
	return ver
}

// fetchToolChangelog fetches the latest release from GitHub and parses bullet points.
func fetchToolChangelog(tool, currentVer string) *toolChangelog {
	repo, ok := toolGitHubRepo[tool]
	if !ok {
		return nil // no known repo (e.g. cursor)
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil
	}

	var release struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
	}
	if json.Unmarshal(body, &release) != nil {
		return nil
	}

	items := parseChangelogBullets(release.Body)
	if len(items) == 0 {
		return nil
	}

	return &toolChangelog{
		ToolName: tool,
		Version:  currentVer,
		Items:    items,
	}
}

// parseChangelogBullets extracts bullet points from markdown release body.
func parseChangelogBullets(body string) []string {
	var items []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Match markdown bullets: "- ", "* ", "• "
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "• ") {
			item := strings.TrimSpace(line[2:])
			// Skip sub-bullets (indented)
			if item == "" {
				continue
			}
			// Strip markdown links: [text](url) → text
			for {
				start := strings.Index(item, "[")
				mid := strings.Index(item, "](")
				end := strings.Index(item, ")")
				if start >= 0 && mid > start && end > mid {
					text := item[start+1 : mid]
					item = item[:start] + text + item[end+1:]
				} else {
					break
				}
			}
			// Strip bold markers
			item = strings.ReplaceAll(item, "**", "")
			if len(item) > 80 {
				item = item[:77] + "..."
			}
			items = append(items, item)
		}
	}
	return items
}

// --- Global config helpers (~/.c4/config.yaml) ---

func globalConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".c4", "config.yaml")
}

func readGlobalConfig(key string) string {
	path := globalConfigPath()
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ""
	}
	val, _ := cfg[key].(string)
	return val
}

func writeGlobalConfig(key, value string) error {
	path := globalConfigPath()
	if path == "" {
		return fmt.Errorf("cannot determine home directory")
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	// Read existing config
	var cfg map[string]interface{}
	data, err := os.ReadFile(path)
	if err == nil {
		_ = yaml.Unmarshal(data, &cfg)
	}
	if cfg == nil {
		cfg = make(map[string]interface{})
	}

	cfg[key] = value

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}
	return os.WriteFile(path, out, 0644)
}
