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

// --- Custom messages for dashboard ---
// tickMsg is declared in doctor_tui.go (shared across TUIs).

// boardItem represents a menu entry in the dashboard board list.
type boardItem struct {
	key        string // shortcut key (e.g. "t")
	label      string // display name
	desc       string // one-line description
	screen     string // target screen name
	comingSoon bool   // true = not yet implemented
}

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
	components  []componentRow // service health details
	boards      []boardItem    // board menu items
	selectedIdx int            // cursor position in boards
	changelog   *toolChangelog // tool changelog (nil if unavailable)
	whatsNew    string         // "New in vX.Y.Z: ..." or ""
	defaultTool string
	action      string // "launch", "status", "config", or "" (quit)
	showDetail  bool   // toggle: show component details

	width  int
	height int

	// Navigation
	nextScreen string
}

// defaultBoards defines the board menu items displayed on the dashboard.
func defaultBoards() []boardItem {
	return []boardItem{
		{key: "t", label: "Sessions", desc: "세션 목록 관리", screen: screenSessions},
		{key: "i", label: "Ideas", desc: "아이디어 탐색·연결", screen: screenIdeas},
		{key: "a", label: "Add", desc: "스킬·에이전트 설치", screen: screenAdd},
		{key: "d", label: "Doctor", desc: "설치 환경 진단", screen: screenDoctor},
		{key: "g", label: "Config", desc: "설정 관리", screen: screenConfig},
		{key: "w", label: "Workers", desc: "워커 모니터링", screen: screenWorkers},
		{key: "m", label: "Metrics", desc: "실험 메트릭 추적", comingSoon: true},
	}
}

// BuildCommandRows returns the full command reference with category headers.
// Used by the Help TUI screen.
func BuildCommandRows() []HelpCmdRow {
	return []HelpCmdRow{
		// CLI Commands
		{IsHeader: true, Category: "CLI"},
		{Name: "cq claude", Desc: "Claude Code 시작"},
		{Name: "cq cursor", Desc: "Cursor 시작"},
		{Name: "cq codex", Desc: "Codex CLI 시작"},
		{Name: "cq gemini", Desc: "Gemini CLI 시작"},
		{Name: "cq -t <name>", Desc: "이름 붙인 세션 시작/이어가기"},
		{Name: "cq sessions", Desc: "세션 목록 관리"},
		{Name: "cq status", Desc: "서비스 + 프로젝트 상태"},
		{Name: "cq doctor", Desc: "설치 환경 진단"},
		{Name: "cq update", Desc: "CQ 최신 버전 업데이트"},
		{Name: "cq stop", Desc: "CQ 서비스 중지"},
		{Name: "cq add", Desc: "스킬/에이전트/룰 설치 (프리셋 + GitHub)"},
		{Name: "cq add <url>", Desc: "GitHub에서 원격 설치"},
		{Name: "cq list --mine", Desc: "설치된 커스텀 도구 목록"},
		{Name: "cq remove <name>", Desc: "설치된 도구 삭제"},
		{Name: "cq update <name>", Desc: "원격 설치 도구 업데이트"},

		// Slash Commands (Skills)
		{IsHeader: true, Category: "Slash Commands"},
		{Name: "/pi", Desc: "아이디어 발산·수렴 (plan 이전 단계)"},
		{Name: "/plan", Desc: "구조화된 구현 계획 생성"},
		{Name: "/run", Desc: "워커 스폰, 태스크 병렬 실행"},
		{Name: "/finish", Desc: "품질 수렴 + 빌드 + 커밋"},
		{Name: "/quick", Desc: "태스크 1개 빠른 실행"},
		{Name: "/status", Desc: "프로젝트 상태 + 태스크 그래프"},
		{Name: "/review", Desc: "6축 코드 리뷰"},
		{Name: "/craft", Desc: "대화형 스킬/에이전트/룰 생성"},
		{Name: "/help", Desc: "스킬/에이전트/도구 레퍼런스"},
		{Name: "/attach", Desc: "현재 세션에 이름 붙이기"},
		{Name: "/reboot", Desc: "세션 재시작"},
		{Name: "/release", Desc: "릴리스 노트 + 태그 생성"},
		{Name: "/simplify", Desc: "변경 코드 품질·효율 점검"},

		// MCP Tools (Core)
		{IsHeader: true, Category: "MCP Tools — 태스크"},
		{Name: "cq_status", Desc: "프로젝트 상태 조회"},
		{Name: "cq_add_todo", Desc: "태스크 추가"},
		{Name: "cq_get_task", Desc: "워커에 태스크 할당"},
		{Name: "cq_submit", Desc: "태스크 완료 제출"},
		{Name: "cq_claim / cq_report", Desc: "Direct 모드 태스크 수행"},
		{Name: "cq_task_list", Desc: "태스크 목록 필터링"},
		{Name: "cq_start", Desc: "EXECUTE 상태로 전환"},

		// MCP Tools (Knowledge)
		{IsHeader: true, Category: "MCP Tools — 지식"},
		{Name: "cq_knowledge_search", Desc: "지식 베이스 검색"},
		{Name: "cq_knowledge_record", Desc: "지식 문서 저장"},
		{Name: "cq_save_spec", Desc: "EARS 스펙 저장"},
		{Name: "cq_save_design", Desc: "설계 문서 저장"},
		{Name: "cq_lighthouse", Desc: "API 계약 관리 (TDD)"},

		// MCP Tools (Infra)
		{IsHeader: true, Category: "MCP Tools — 인프라"},
		{Name: "cq_read_file", Desc: "파일 읽기"},
		{Name: "cq_find_file", Desc: "파일 검색"},
		{Name: "cq_search_for_pattern", Desc: "코드 패턴 검색"},
		{Name: "cq_run_validation", Desc: "lint/test 실행"},
		{Name: "cq_notify", Desc: "알림 전송 (텔레그램 등)"},
		{Name: "cq_relay_call", Desc: "원격 서버 MCP 호출"},
		{Name: "cq_llm_call", Desc: "LLM Gateway 호출"},
	}
}

// cqLogo is a compact 2-line dot-art CQ mark.
var cqLogo = []string{
	"▄▀▀▀ ▄▀▀▀▄",
	"▀▄▄▄ ▀▄▄▀▄",
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

	// Board menu items
	m.boards = defaultBoards()

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

// tickInterval is the dashboard refresh interval.
const tickInterval = 3 * time.Second

func (m dashboardModel) Init() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		// Refresh service health
		components, err := fetchServeHealth(servePort)
		if err == nil {
			okCount := 0
			m.components = nil
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
			sort.Slice(m.components, func(i, j int) bool {
				return m.components[i].name < m.components[j].name
			})
			// Update Service row
			for i, row := range m.rows {
				if row.label == "Service" {
					badge := "active"
					if okCount == len(components) {
						badge = "running"
					}
					m.rows[i].value = fmt.Sprintf("%d/%d components", okCount, len(components))
					m.rows[i].badge = badge
					break
				}
			}
		}
		return m, tea.Tick(tickInterval, func(t time.Time) tea.Msg {
			return tickMsg{}
		})

	case tea.KeyMsg:
		// Global navigation keys
		if next, ok := handleGlobalKey(msg, false); ok {
			if next == screenQuit {
				return m, tea.Quit
			}
			m.nextScreen = next
			return m, tea.Quit
		}

		switch msg.Type {
		case tea.KeyEnter:
			if len(m.boards) > 0 {
				b := m.boards[m.selectedIdx]
				if b.comingSoon {
					// Do nothing for coming soon items
				} else {
					m.nextScreen = b.screen
					return m, tea.Quit
				}
			}
		case tea.KeyUp:
			for idx := m.selectedIdx - 1; idx >= 0; idx-- {
				if !m.boards[idx].comingSoon {
					m.selectedIdx = idx
					break
				}
			}
		case tea.KeyDown:
			for idx := m.selectedIdx + 1; idx < len(m.boards); idx++ {
				if !m.boards[idx].comingSoon {
					m.selectedIdx = idx
					break
				}
			}
		case tea.KeyRunes:
			switch string(msg.Runes) {
			case "s":
				m.showDetail = !m.showDetail
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

	// CQ Logo + version inline
	logoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)
	sb.WriteString(logoStyle.Render("  " + cqLogo[0]))
	sb.WriteString("\n")
	sb.WriteString(logoStyle.Render("  " + cqLogo[1]))
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

	// Board menu list
	sb.WriteString("\n")
	for i, b := range m.boards {
		cursor := "  "
		if i == m.selectedIdx {
			cursor = "> "
		}

		keyStyle := styleHelpKey
		descStyle := styleFaint
		suffix := ""
		if b.comingSoon {
			keyStyle = styleFaint
			descStyle = styleFaint
			suffix = " (coming soon)"
		}

		line := fmt.Sprintf("%s[%s] %-12s%s%s", cursor, b.key, b.label, b.desc, suffix)
		if i == m.selectedIdx && !b.comingSoon {
			sb.WriteString(styleTagName.Render(line))
		} else if b.comingSoon {
			sb.WriteString(descStyle.Render(line))
		} else {
			sb.WriteString(keyStyle.Render(fmt.Sprintf("%s[%s] %-12s", cursor, b.key, b.label)))
			sb.WriteString(descStyle.Render(b.desc))
		}
		sb.WriteString("\n")
	}

	// Build help bar
	var helpBar strings.Builder
	helpBar.WriteString(" ")
	helpBar.WriteString(helpEntry("↑↓", "이동"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("Enter", "진입"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("s", "상태"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("?", "Help"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("q", "종료"))

	// Pin help bar at bottom
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
	sb.WriteString("\n")
	sb.WriteString(renderNavBar(screenDashboard, m.width))

	return sb.String()
}

// runDashboardNav runs dashboard TUI and returns the next screen for the main loop.
func runDashboardNav() string {
	p := tea.NewProgram(newDashboardModel(), tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return screenQuit
	}
	dm, ok := result.(dashboardModel)
	if !ok {
		return screenQuit
	}
	if dm.nextScreen != "" {
		return dm.nextScreen
	}
	return screenQuit
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
