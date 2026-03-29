package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHandleGlobalKey_Mapping(t *testing.T) {
	tests := []struct {
		name       string
		msg        tea.KeyMsg
		inputMode  bool
		wantScreen string
		wantOK     bool
	}{
		{"t → sessions", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}, false, screenSessions, true},
		{"a → add", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}, false, screenAdd, true},
		{"g → config", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}, false, screenConfig, true},
		{"d → doctor", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}, false, screenDoctor, true},
		{"i → ideas", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}}, false, screenIdeas, true},
		{"? → help", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}, false, screenHelp, true},
		{"q → quit", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}, false, screenQuit, true},
		{"Esc → dashboard", tea.KeyMsg{Type: tea.KeyEsc}, false, screenDashboard, true},
		{"Ctrl+C → quit", tea.KeyMsg{Type: tea.KeyCtrlC}, false, screenQuit, true},
		// inputMode ignores all
		{"t ignored in inputMode", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}, true, "", false},
		{"Esc ignored in inputMode", tea.KeyMsg{Type: tea.KeyEsc}, true, "", false},
		// Unmapped key
		{"x not handled", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}, false, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			screen, ok := handleGlobalKey(tt.msg, tt.inputMode)
			if screen != tt.wantScreen || ok != tt.wantOK {
				t.Errorf("handleGlobalKey() = (%q, %v), want (%q, %v)",
					screen, ok, tt.wantScreen, tt.wantOK)
			}
		})
	}
}

func TestRenderNavBar_ContainsAllItems(t *testing.T) {
	bar := renderNavBar("sessions", 120)
	for _, item := range navBarItems {
		if !strings.Contains(bar, item.label) {
			t.Errorf("navBar missing label %q", item.label)
		}
	}
}

func TestRenderNavBar_HighlightsCurrent(t *testing.T) {
	// Just verify it doesn't panic and returns non-empty for various screens.
	screens := []string{screenSessions, screenConfig, screenAdd, screenDoctor, screenIdeas, screenDashboard, screenHelp}
	for _, s := range screens {
		bar := renderNavBar(s, 80)
		if bar == "" {
			t.Errorf("renderNavBar(%q) returned empty", s)
		}
	}
}
