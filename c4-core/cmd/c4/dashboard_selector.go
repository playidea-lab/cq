package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

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
	var sb strings.Builder

	// Title bar — same as cq sessions
	sb.WriteString(styleTitle.Render(" 기본 도구 선택 "))
	sb.WriteString("  ")
	sb.WriteString(styleCount.Render(fmt.Sprintf("%d tools", len(m.tools))))
	sb.WriteString("\n\n")

	for i, t := range m.tools {
		isSelected := i == m.cursor

		cursor := "   "
		if isSelected {
			cursor = " ▸ "
		}

		// Tool name (padded to 16 chars)
		nameStr := fmt.Sprintf("%-16s", t.name)

		// Status badge: installed → running style, not installed → done(dim) style
		var badge string
		if t.installed {
			bStyle := statusBadgeStyles["running"]
			badge = bStyle.Render(" installed ")
		} else {
			bStyle := statusBadgeStyles["done"]
			badge = bStyle.Render(" 미설치    ")
		}

		// Version or empty
		var verStr string
		if t.installed && t.version != "" {
			verStr = t.version
		}

		if isSelected {
			sb.WriteString(styleSelected.Render(cursor))
			sb.WriteString(styleSelected.Render(nameStr))
			sb.WriteString(badge)
			sb.WriteString(" ")
			sb.WriteString(styleSelected.Render(verStr))
		} else if !t.installed {
			sb.WriteString(styleTagNameDim.Render(cursor))
			sb.WriteString(styleTagNameDim.Render(nameStr))
			sb.WriteString(badge)
		} else {
			sb.WriteString(cursor)
			sb.WriteString(styleTagName.Render(nameStr))
			sb.WriteString(badge)
			sb.WriteString(" ")
			sb.WriteString(styleDate.Render(verStr))
		}
		sb.WriteString("\n")
	}

	// Help bar — same pattern as cq sessions
	sb.WriteString("\n")
	sb.WriteString(" ")
	sb.WriteString(helpEntry("↑↓", "이동"))
	sb.WriteString("  ")
	sb.WriteString(helpEntry("Enter", "선택"))
	sb.WriteString("  ")
	sb.WriteString(helpEntry("Esc", "취소"))
	sb.WriteString("\n")

	return sb.String()
}
