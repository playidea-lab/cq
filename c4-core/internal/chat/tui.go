// Package chat — bubbletea TUI for cq chat.
// CJK (한글/中文/日本語) widths handled via charmbracelet/x/ansi + mattn/go-runewidth.
package chat

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// ─── styles ──────────────────────────────────────────────────────────────────

var (
	styleSender = map[string]lipgloss.Style{
		"user":   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")),
		"agent":  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("82")),
		"system": lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("241")),
	}
	styleTime       = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("241"))
	styleStatusBar  = lipgloss.NewStyle().Background(lipgloss.Color("235")).Foreground(lipgloss.Color("250")).Padding(0, 1)
	styleOnline     = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	styleOffline    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	styleBorder     = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("238"))
)

// ─── messages ─────────────────────────────────────────────────────────────────

// IncomingMsg is a tea.Msg carrying a newly received chat message.
type IncomingMsg Message

// AgentStatusMsg updates the agent online/offline status.
type AgentStatusMsg struct {
	Name   string
	Online bool
}

// ChannelListMsg delivers available channels.
type ChannelListMsg []chatChannel

// ChannelSwitchedMsg signals that the active channel changed.
type ChannelSwitchedMsg struct {
	ChannelID   string
	ChannelName string
}

// SendRequestMsg is emitted by the TUI when the user submits a message.
// The caller (cmd layer) handles actual posting and injects IncomingMsg.
type SendRequestMsg struct {
	Content string
}

// ErrorMsg carries a non-fatal error string.
type ErrorMsg struct{ Err error }

func (e ErrorMsg) Error() string { return e.Err.Error() }

// ─── channel helper ────────────────────────────────────────────────────────────

type chatChannel struct {
	ID   string
	Name string
}

// ChatChannel constructs a chatChannel for use in ChannelListMsg.
// This is the exported constructor used by the cmd layer.
func ChatChannel(id, name string) chatChannel {
	return chatChannel{ID: id, Name: name}
}

// ─── model ────────────────────────────────────────────────────────────────────

// TUIModel is the bubbletea model for the chat interface.
type TUIModel struct {
	// dimensions
	width  int
	height int

	// channel state
	channelID   string
	channelName string
	channels    []chatChannel

	// messages
	history  []renderedLine
	vp       viewport.Model
	vpReady  bool

	// input
	input textinput.Model

	// agent status
	agents  map[string]bool // name → online
	agentStatusLine string

	// channel switcher
	switchMode    bool
	switchCursor  int

	// lifecycle
	quitting bool
}

type renderedLine struct {
	raw string // pre-rendered string for viewport
}

// NewTUIModel creates a TUIModel ready to be passed to tea.NewProgram.
func NewTUIModel(channelID, channelName string) TUIModel {
	ti := textinput.New()
	ti.Placeholder = "메시지를 입력하세요…"
	ti.Focus()
	ti.CharLimit = 2000
	// Ensure CJK characters are measured correctly by go-runewidth
	ti.Width = 60

	return TUIModel{
		channelID:   channelID,
		channelName: channelName,
		input:       ti,
		agents:      make(map[string]bool),
	}
}

// ─── tea.Model interface ───────────────────────────────────────────────────────

func (m TUIModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerH := 1 // status bar
		footerH := 3 // input box
		vpH := m.height - headerH - footerH
		if vpH < 3 {
			vpH = 3
		}
		if !m.vpReady {
			m.vp = viewport.New(m.width, vpH)
			m.vp.SetContent(m.renderHistory())
			m.vp.GotoBottom()
			m.vpReady = true
		} else {
			m.vp.Width = m.width
			m.vp.Height = vpH
		}
		m.input.Width = m.width - 4
		return m, nil

	case IncomingMsg:
		line := renderMessage(Message(msg))
		m.history = append(m.history, renderedLine{raw: line})
		if m.vpReady {
			m.vp.SetContent(m.renderHistory())
			m.vp.GotoBottom()
		}
		return m, nil

	case AgentStatusMsg:
		m.agents[msg.Name] = msg.Online
		m.agentStatusLine = m.buildAgentStatus()
		return m, nil

	case ChannelListMsg:
		m.channels = make([]chatChannel, len(msg))
		copy(m.channels, msg)
		return m, nil

	case ChannelSwitchedMsg:
		m.channelID = msg.ChannelID
		m.channelName = msg.ChannelName
		m.history = nil
		m.switchMode = false
		if m.vpReady {
			m.vp.SetContent("")
		}
		return m, nil

	case ErrorMsg:
		line := styleTime.Render(fmt.Sprintf("[err] %v", msg.Err))
		m.history = append(m.history, renderedLine{raw: line})
		if m.vpReady {
			m.vp.SetContent(m.renderHistory())
			m.vp.GotoBottom()
		}
		return m, nil

	case tea.KeyMsg:
		// Channel switch mode
		if m.switchMode {
			return m.updateSwitchMode(msg)
		}

		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quitting = true
			return m, tea.Quit

		case tea.KeyEnter:
			line := strings.TrimSpace(m.input.Value())
			m.input.Reset()
			if line == "" {
				return m, nil
			}
			switch line {
			case "/quit", "/exit":
				m.quitting = true
				return m, tea.Quit
			case "/sessions", "/channels":
				m.switchMode = true
				m.switchCursor = 0
				return m, nil
			default:
				return m, func() tea.Msg { return SendRequestMsg{Content: line} }
			}

		case tea.KeyPgUp:
			if m.vpReady {
				m.vp.HalfViewUp()
			}
			return m, nil
		case tea.KeyPgDown:
			if m.vpReady {
				m.vp.HalfViewDown()
			}
			return m, nil
		}
	}

	// Delegate to textinput
	if !m.switchMode {
		var inputCmd tea.Cmd
		m.input, inputCmd = m.input.Update(msg)
		cmds = append(cmds, inputCmd)
	}

	// Delegate viewport scrolling
	if m.vpReady {
		var vpCmd tea.Cmd
		m.vp, vpCmd = m.vp.Update(msg)
		cmds = append(cmds, vpCmd)
	}

	return m, tea.Batch(cmds...)
}

func (m TUIModel) updateSwitchMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.switchMode = false
		return m, nil
	case tea.KeyUp:
		if m.switchCursor > 0 {
			m.switchCursor--
		}
	case tea.KeyDown:
		if m.switchCursor < len(m.channels)-1 {
			m.switchCursor++
		}
	case tea.KeyEnter:
		if len(m.channels) > 0 {
			ch := m.channels[m.switchCursor]
			return m, func() tea.Msg {
				return ChannelSwitchedMsg{ChannelID: ch.ID, ChannelName: ch.Name}
			}
		}
		m.switchMode = false
	}
	return m, nil
}

func (m TUIModel) View() string {
	if !m.vpReady {
		return "Connecting…"
	}
	if m.quitting {
		return "Bye!\n"
	}

	header := m.renderHeader()
	body := styleBorder.Width(m.width - 2).Render(m.vp.View())

	var footer string
	if m.switchMode {
		footer = m.renderChannelPicker()
	} else {
		footer = m.renderInputBox()
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

// ─── rendering helpers ─────────────────────────────────────────────────────────

func (m TUIModel) renderHeader() string {
	channel := fmt.Sprintf("#%s", m.channelName)
	left := styleStatusBar.Render(channel)
	right := styleStatusBar.Render(m.agentStatusLine)

	gap := m.width - visibleWidth(left) - visibleWidth(right)
	if gap < 0 {
		gap = 0
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m TUIModel) renderInputBox() string {
	return fmt.Sprintf(" > %s", m.input.View())
}

func (m TUIModel) renderChannelPicker() string {
	if len(m.channels) == 0 {
		return " (채널 없음) [ESC]"
	}
	var sb strings.Builder
	sb.WriteString(" 채널 선택 [↑↓ Enter ESC]\n")
	for i, ch := range m.channels {
		cursor := "  "
		if i == m.switchCursor {
			cursor = "▶ "
		}
		active := ""
		if ch.ID == m.channelID {
			active = " *"
		}
		sb.WriteString(fmt.Sprintf(" %s#%s%s\n", cursor, ch.Name, active))
	}
	return sb.String()
}

func (m TUIModel) renderHistory() string {
	lines := make([]string, len(m.history))
	for i, l := range m.history {
		lines[i] = l.raw
	}
	return strings.Join(lines, "\n")
}

func (m TUIModel) buildAgentStatus() string {
	if len(m.agents) == 0 {
		return ""
	}
	parts := make([]string, 0, len(m.agents))
	for name, online := range m.agents {
		if online {
			parts = append(parts, styleOnline.Render("● "+name))
		} else {
			parts = append(parts, styleOffline.Render("○ "+name))
		}
	}
	return strings.Join(parts, " ")
}

// ─── message rendering ─────────────────────────────────────────────────────────

func renderMessage(msg Message) string {
	ts := formatTUITime(msg.CreatedAt)
	sender := msg.SenderName
	if sender == "" {
		sender = msg.SenderType
	}

	sStyle, ok := styleSender[msg.SenderType]
	if !ok {
		sStyle = styleSender["user"]
	}

	return fmt.Sprintf("%s %s  %s",
		styleTime.Render(ts),
		sStyle.Render(sender+":"),
		msg.Content,
	)
}

func formatTUITime(ts string) string {
	if ts == "" {
		return time.Now().Format("15:04")
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05", ts)
		if err != nil {
			return ts
		}
	}
	return t.Local().Format("15:04")
}

// visibleWidth returns the visible terminal width of s, accounting for ANSI
// escape sequences and CJK double-width characters.
func visibleWidth(s string) int {
	// Strip ANSI sequences then measure CJK width.
	plain := stripANSI(s)
	w := 0
	for _, r := range plain {
		w += runewidth.RuneWidth(r)
	}
	return w
}

// stripANSI removes ANSI escape sequences from s.
func stripANSI(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	inEsc := false
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}
