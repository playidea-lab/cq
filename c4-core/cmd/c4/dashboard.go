package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"

	tea "github.com/charmbracelet/bubbletea"
)

// --- Custom messages for dashboard actions ---

type launchToolMsg struct{ tool string }
type showStatusMsg struct{}
type changeConfigMsg struct{}

// --- Dashboard TUI Model ---

type dashboardModel struct {
	version     string
	serviceMsg  string // "running (N/M components)" or "starting..."
	projectMsg  string // project status line or ""
	whatsNew    string // "New in vX.Y.Z: ..." or ""
	defaultTool string
	action      string // "launch", "status", "config", or "" (quit)

	width  int
	height int
}

func newDashboardModel() dashboardModel {
	m := dashboardModel{
		version:     version,
		defaultTool: readGlobalConfig("default_tool"),
	}
	if m.defaultTool == "" {
		m.defaultTool = "claude"
	}

	// Service health
	components, err := fetchServeHealth(servePort)
	if err != nil {
		m.serviceMsg = "starting..."
	} else {
		okCount := 0
		for _, h := range components {
			if h.Status == "ok" {
				okCount++
			}
		}
		m.serviceMsg = fmt.Sprintf("running (%d/%d components)", okCount, len(components))
	}

	// Project status (if inside a .c4/ project)
	c4Dir := filepath.Join(projectDir, ".c4")
	if info, err := os.Stat(c4Dir); err == nil && info.IsDir() {
		m.projectMsg = buildProjectStatusLine()
	}

	// What's New check
	lastSeen := readGlobalConfig("last_seen_version")
	if lastSeen != "" && lastSeen != version && version != "dev" {
		m.whatsNew = fmt.Sprintf("✨ New in %s", version)
	}

	return m
}

// buildProjectStatusLine reads .c4/state.yaml or tasks.db to produce a summary.
func buildProjectStatusLine() string {
	// Try reading project name from .c4/state.yaml
	statePath := filepath.Join(projectDir, ".c4", "state.yaml")
	data, err := os.ReadFile(statePath)
	if err != nil {
		return filepath.Base(projectDir)
	}
	var state map[string]interface{}
	if err := yaml.Unmarshal(data, &state); err != nil {
		return filepath.Base(projectDir)
	}

	name, _ := state["project_id"].(string)
	if name == "" {
		name = filepath.Base(projectDir)
	}
	phase, _ := state["phase"].(string)
	if phase == "" {
		phase = "idle"
	}
	return fmt.Sprintf("%s [%s]", name, phase)
}

func (m dashboardModel) Init() tea.Cmd {
	return nil
}

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case launchToolMsg:
		m.action = "launch"
		return m, tea.Quit
	case showStatusMsg:
		m.action = "status"
		return m, tea.Quit
	case changeConfigMsg:
		m.action = "config"
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
				return m, func() tea.Msg { return showStatusMsg{} }
			case "c":
				return m, func() tea.Msg { return changeConfigMsg{} }
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m dashboardModel) View() string {
	var b strings.Builder

	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("62")).
		Padding(0, 1)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("244"))

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("15"))

	hintKeyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("14")).
		Bold(true)

	hintDescStyle := lipgloss.NewStyle().
		Faint(true)

	newStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("3")).
		Bold(true)

	// Title
	b.WriteString(titleStyle.Render(fmt.Sprintf(" CQ %s ", m.version)))
	b.WriteString("\n\n")

	// Service
	b.WriteString(labelStyle.Render("  Service: "))
	b.WriteString(valueStyle.Render(m.serviceMsg))
	b.WriteString("\n")

	// Project (if available)
	if m.projectMsg != "" {
		b.WriteString(labelStyle.Render("  Project: "))
		b.WriteString(valueStyle.Render(m.projectMsg))
		b.WriteString("\n")
	}

	// Default tool
	b.WriteString(labelStyle.Render("  Tool:    "))
	b.WriteString(valueStyle.Render(m.defaultTool))
	b.WriteString("\n")

	// What's New
	if m.whatsNew != "" {
		b.WriteString("\n")
		b.WriteString(newStyle.Render("  "+m.whatsNew))
		b.WriteString("\n")
	}

	// Key hints
	b.WriteString("\n")
	b.WriteString("  ")
	b.WriteString(hintKeyStyle.Render("[Enter]"))
	b.WriteString(hintDescStyle.Render(fmt.Sprintf(" %s 시작  ", m.defaultTool)))
	b.WriteString(hintKeyStyle.Render("[s]"))
	b.WriteString(hintDescStyle.Render(" 상태 상세  "))
	b.WriteString(hintKeyStyle.Render("[c]"))
	b.WriteString(hintDescStyle.Render(" 설정 변경  "))
	b.WriteString(hintKeyStyle.Render("[q]"))
	b.WriteString(hintDescStyle.Render(" 종료"))
	b.WriteString("\n")

	return b.String()
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
