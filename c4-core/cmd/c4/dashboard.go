package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

type dashboardModel struct {
	version     string
	rows        []dashboardRow
	components  []componentRow // service health details
	whatsNew    string         // "New in vX.Y.Z: ..." or ""
	defaultTool string
	action      string // "launch", "status", "config", or "" (quit)
	showDetail  bool   // toggle: show component details

	width  int
	height int
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
			badge = "in-progress"
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
			if entry.Status == "" || entry.Status == "active" || entry.Status == "in-progress" || entry.Status == "running" {
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

	// What's New
	if m.whatsNew != "" {
		sb.WriteString("\n")
		hs := groupHeaderStyle("idea")
		sb.WriteString(hs.Render("  " + m.whatsNew))
		sb.WriteString("\n")
	}

	// Build help bar
	var helpBar strings.Builder
	helpBar.WriteString(" ")
	helpBar.WriteString(helpEntry("Enter", m.defaultTool+" 시작"))
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
