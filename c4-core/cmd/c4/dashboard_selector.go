package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// toolSelectedMsg is returned when the user picks a tool.
type toolSelectedMsg struct{ tool string }

// toolEntry holds display info for a single tool option.
type toolEntry struct {
	name      string
	installed bool
	version   string // e.g. "v1.0.40" or ""
}

// selectorModel is the bubbletea model for the tool selector.
type selectorModel struct {
	tools    []toolEntry
	cursor   int
	selected string // set when user picks a tool, empty if quit without selecting

	width  int
	height int
}

// supportedTools is the list of tools to probe.
var supportedTools = []string{"claude", "cursor", "codex", "gemini"}

func newSelectorModel() selectorModel {
	tools := make([]toolEntry, len(supportedTools))
	for i, name := range supportedTools {
		tools[i] = probeToolEntry(name)
	}
	return selectorModel{tools: tools}
}

// probeToolEntry checks if a tool is installed and gets its version.
func probeToolEntry(name string) toolEntry {
	entry := toolEntry{name: name}
	path, err := exec.LookPath(name)
	if err != nil || path == "" {
		return entry
	}
	entry.installed = true

	// Run --version with a 2s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, "--version")
	out, err := cmd.Output()
	if err == nil {
		ver := strings.TrimSpace(string(out))
		// Take first line only
		if idx := strings.IndexByte(ver, '\n'); idx > 0 {
			ver = ver[:idx]
		}
		// Trim to something short if too long
		if len(ver) > 40 {
			ver = ver[:40] + "..."
		}
		entry.version = ver
	}
	return entry
}

func (m selectorModel) Init() tea.Cmd {
	return nil
}

func (m selectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case toolSelectedMsg:
		m.selected = msg.tool
		return m, tea.Quit
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(m.tools)-1 {
				m.cursor++
			}
		case tea.KeyEnter:
			selected := m.tools[m.cursor]
			if selected.installed {
				return m, func() tea.Msg { return toolSelectedMsg{tool: selected.name} }
			}
		case tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyRunes:
			switch string(msg.Runes) {
			case "q":
				return m, tea.Quit
			case "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "j":
				if m.cursor < len(m.tools)-1 {
					m.cursor++
				}
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m selectorModel) View() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("62")).
		Padding(0, 1)

	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("15")).
		Bold(true)

	installedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("2"))

	notInstalledStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("1")).
		Faint(true)

	versionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("244"))

	hintKeyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("14")).
		Bold(true)

	hintDescStyle := lipgloss.NewStyle().
		Faint(true)

	b.WriteString(titleStyle.Render(" 기본 도구 선택 "))
	b.WriteString("\n\n")

	for i, t := range m.tools {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		// Tool name (padded to 16 chars)
		nameStr := fmt.Sprintf("%-16s", t.name)
		if i == m.cursor {
			nameStr = selectedStyle.Render(nameStr)
		}

		var statusStr string
		if t.installed {
			ver := t.version
			if ver == "" {
				ver = "installed"
			}
			statusStr = versionStyle.Render(ver) + " " + installedStyle.Render("✓")
		} else {
			statusStr = notInstalledStyle.Render("✗ 미설치")
		}

		b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, nameStr, statusStr))
	}

	b.WriteString("\n")
	b.WriteString("  ")
	b.WriteString(hintKeyStyle.Render("[↑↓]"))
	b.WriteString(hintDescStyle.Render(" 이동  "))
	b.WriteString(hintKeyStyle.Render("[Enter]"))
	b.WriteString(hintDescStyle.Render(" 선택  "))
	b.WriteString(hintKeyStyle.Render("[Esc]"))
	b.WriteString(hintDescStyle.Render(" 취소"))
	b.WriteString("\n")

	return b.String()
}
