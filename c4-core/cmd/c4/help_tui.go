package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// HelpCmdRow is a single row in the commands reference (header or command).
type HelpCmdRow struct {
	IsHeader bool
	Category string // header label
	Name     string
	Desc     string
}

type helpModel struct {
	cmdRows    []HelpCmdRow
	scroll     int
	width      int
	height     int
	nextScreen string
}

func newHelpModel() helpModel {
	return helpModel{
		cmdRows: BuildCommandRows(),
	}
}

// visibleLines returns how many command rows fit in the viewport.
func (m helpModel) visibleLines() int {
	// header(2) + help bar(2) + nav bar(1) + margins(3)
	used := 8
	visible := m.height - used
	if visible < 5 {
		visible = 5
	}
	if visible > len(m.cmdRows) {
		visible = len(m.cmdRows)
	}
	return visible
}

func (m helpModel) Init() tea.Cmd {
	return nil
}

func (m helpModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
		case tea.KeyUp:
			if m.scroll > 0 {
				m.scroll--
			}
		case tea.KeyDown:
			maxScroll := len(m.cmdRows) - m.visibleLines()
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.scroll < maxScroll {
				m.scroll++
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m helpModel) View() string {
	var sb strings.Builder

	// Header
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)
	sb.WriteString(headerStyle.Render("  Commands Reference"))
	sb.WriteString("\n\n")

	// Scrollable command list
	visible := m.visibleLines()
	end := m.scroll + visible
	if end > len(m.cmdRows) {
		end = len(m.cmdRows)
	}

	if m.scroll > 0 {
		sb.WriteString(styleFaint.Render("  ▲ more"))
		sb.WriteString("\n")
	}

	for _, row := range m.cmdRows[m.scroll:end] {
		if row.IsHeader {
			header := fmt.Sprintf(" ── %s ", row.Category)
			hs := groupHeaderStyle("active")
			sb.WriteString(hs.Render(header))
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
			sb.WriteString(styleHelpKey.Render(fmt.Sprintf("  %-24s", row.Name)))
			sb.WriteString(styleFaint.Render(row.Desc))
			sb.WriteString("\n")
		}
	}

	if end < len(m.cmdRows) {
		sb.WriteString(styleFaint.Render("  ▼ more"))
		sb.WriteString("\n")
	}

	// Help bar
	var helpBar strings.Builder
	helpBar.WriteString(" ")
	helpBar.WriteString(helpEntry("↑↓", "스크롤"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("Esc", "돌아가기"))
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
	sb.WriteString(renderNavBar(screenHelp, m.width))

	return sb.String()
}

// runHelpNav runs Help TUI and returns the next screen for the main loop.
func runHelpNav() string {
	p := tea.NewProgram(newHelpModel(), tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return screenQuit
	}
	hm, ok := result.(helpModel)
	if !ok {
		return screenQuit
	}
	if hm.nextScreen != "" {
		return hm.nextScreen
	}
	return screenDashboard
}
