package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// Screen names used for TUI navigation.
const (
	screenSessions  = "sessions"
	screenConfig    = "config"
	screenAdd       = "add"
	screenDoctor    = "doctor"
	screenIdeas     = "ideas"
	screenDashboard = "dashboard"
	screenLaunch    = "launch"
	screenQuit      = "quit"
)

// navMapping maps key runes to screen names.
var navMapping = map[string]string{
	"t": screenSessions,
	"a": screenAdd,
	"g": screenConfig,
	"d": screenDoctor,
	"i": screenIdeas,
	"?": screenDashboard,
}

// handleGlobalKey checks if a key press is a global navigation key.
// When inputMode is true (e.g. search/edit active), all global keys are ignored.
// Returns the target screen name and whether the key was handled.
func handleGlobalKey(msg tea.KeyMsg, inputMode bool) (string, bool) {
	if inputMode {
		return "", false
	}

	switch msg.Type {
	case tea.KeyEsc:
		return screenSessions, true
	case tea.KeyCtrlC:
		return screenQuit, true
	case tea.KeyRunes:
		ch := string(msg.Runes)
		if target, ok := navMapping[ch]; ok {
			return target, true
		}
		if ch == "q" {
			return screenQuit, true
		}
	}
	return "", false
}

// navBar items in display order.
var navBarItems = []struct {
	key    string
	label  string
	screen string
}{
	{"t", "Sessions", screenSessions},
	{"a", "Add", screenAdd},
	{"g", "Config", screenConfig},
	{"d", "Doctor", screenDoctor},
	{"i", "Ideas", screenIdeas},
	{"?", "Help", screenDashboard},
}

var (
	navBarKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("14")).
			Bold(true)
	navBarLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))
	navBarActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("14")).
				Bold(true)
	navBarSep = lipgloss.NewStyle().
			Faint(true).
			Render(" | ")
)

// renderNavBar renders a bottom navigation bar highlighting the current screen.
func renderNavBar(current string, width int) string {
	var parts []string
	for _, item := range navBarItems {
		key := "[" + item.key + "]"
		if item.screen == current {
			parts = append(parts, navBarActiveStyle.Render(key+" "+item.label))
		} else {
			parts = append(parts, navBarKeyStyle.Render(key)+" "+navBarLabelStyle.Render(item.label))
		}
	}
	bar := strings.Join(parts, navBarSep)
	// Center the bar if width allows.
	barLen := lipgloss.Width(bar)
	if width > barLen {
		pad := (width - barLen) / 2
		bar = strings.Repeat(" ", pad) + bar
	}
	return bar
}
