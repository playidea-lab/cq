package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// spinnerFrames is the braille-dot rotation sequence for the loading spinner.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// severityBadgeStyles maps check statuses to colored badge styles.
var severityBadgeStyles = map[checkStatus]lipgloss.Style{
	checkFail: lipgloss.NewStyle().Background(lipgloss.Color("1")).Foreground(lipgloss.Color("15")).Bold(true).Padding(0, 1),
	checkWarn: lipgloss.NewStyle().Background(lipgloss.Color("3")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1),
	checkInfo: lipgloss.NewStyle().Background(lipgloss.Color("4")).Foreground(lipgloss.Color("15")).Padding(0, 1),
	checkOK:   lipgloss.NewStyle().Background(lipgloss.Color("2")).Foreground(lipgloss.Color("0")).Padding(0, 1),
}

// severityOrder defines the display order for check result groups.
var severityOrder = []checkStatus{checkFail, checkWarn, checkInfo, checkOK}

// checkItem wraps a check entry with runtime state.
type checkItem struct {
	entry   doctorCheckEntry // from doctor.go (Name, Fn, FixSafe, IsNetwork)
	result  checkResult      // filled when check completes
	loading bool             // true while check is running
	detail  string           // extended error info
}

// Messages for the TUI event loop.
type checkCompleteMsg struct {
	index  int
	result checkResult
}

type tickMsg struct{}

// doctorTUIModel is the bubbletea model for the doctor TUI.
type doctorTUIModel struct {
	checks       []checkItem
	cursor       int
	query        string
	statusFilter string // "all", "FAIL", "WARN", "INFO", "OK"
	width        int
	height       int

	// Spinner state
	spinnerFrame int
}

func newDoctorTUIModel() doctorTUIModel {
	checks := make([]checkItem, len(doctorChecks))
	for i, entry := range doctorChecks {
		checks[i] = checkItem{
			entry:   entry,
			loading: true,
		}
	}
	return doctorTUIModel{
		checks:       checks,
		statusFilter: "all",
	}
}

// runCheckCmd returns a tea.Cmd that executes a single check asynchronously.
func runCheckCmd(index int, entry doctorCheckEntry) tea.Cmd {
	return func() tea.Msg {
		result := entry.Fn()
		return checkCompleteMsg{
			index:  index,
			result: result,
		}
	}
}

// tickCmd returns a tea.Cmd that sends a tickMsg after 100ms.
func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m doctorTUIModel) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.checks)+1)
	for i, item := range m.checks {
		cmds = append(cmds, runCheckCmd(i, item.entry))
	}
	cmds = append(cmds, tickCmd())
	return tea.Batch(cmds...)
}

func (m doctorTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case checkCompleteMsg:
		if msg.index >= 0 && msg.index < len(m.checks) {
			m.checks[msg.index].loading = false
			m.checks[msg.index].result = msg.result
			if msg.result.Fix != "" {
				m.checks[msg.index].detail = msg.result.Fix
			}
		}
		return m, nil

	case tickMsg:
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		// Only keep ticking if there are still loading checks.
		for _, c := range m.checks {
			if c.loading {
				return m, tickCmd()
			}
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp:
			m.moveCursor(-1)
		case tea.KeyDown:
			m.moveCursor(1)
		case tea.KeyEsc:
			if m.query != "" {
				m.query = ""
				return m, nil
			}
			return m, tea.Quit
		case tea.KeyBackspace:
			if len(m.query) > 0 {
				runes := []rune(m.query)
				m.query = string(runes[:len(runes)-1])
			}
		case tea.KeyRunes:
			ch := msg.String()
			switch ch {
			case "k":
				if m.query == "" {
					m.moveCursor(-1)
					return m, nil
				}
				m.query += ch
			case "j":
				if m.query == "" {
					m.moveCursor(1)
					return m, nil
				}
				m.query += ch
			case "q":
				if m.query == "" {
					return m, tea.Quit
				}
				m.query += ch
			default:
				// Enter, r, Tab, search keys — no-ops for now (T-903)
				m.query += ch
			}
		}
	}
	return m, nil
}

// doctorVisibleRows returns the indices of checks visible with current filter/search.
type doctorRow struct {
	isHeader bool
	status   checkStatus
	count    int
	index    int // index into m.checks (for non-header rows)
}

func (m *doctorTUIModel) buildVisibleRows() []doctorRow {
	lowerQuery := strings.ToLower(m.query)

	// Group checks by severity
	type group struct {
		status  checkStatus
		indices []int
	}

	// Collect completed checks by severity, plus loading
	bySeverity := make(map[checkStatus][]int)
	var loadingIndices []int

	for i, c := range m.checks {
		if c.loading {
			loadingIndices = append(loadingIndices, i)
			continue
		}
		// Apply filter
		if m.statusFilter != "all" && string(c.result.Status) != m.statusFilter {
			continue
		}
		// Apply search
		if lowerQuery != "" {
			corpus := strings.ToLower(c.entry.Name + " " + c.result.Message)
			if !strings.Contains(corpus, lowerQuery) {
				continue
			}
		}
		bySeverity[c.result.Status] = append(bySeverity[c.result.Status], i)
	}

	var rows []doctorRow

	// Add groups in severity order
	for _, status := range severityOrder {
		indices, ok := bySeverity[status]
		if !ok || len(indices) == 0 {
			continue
		}
		rows = append(rows, doctorRow{isHeader: true, status: status, count: len(indices)})
		for _, idx := range indices {
			rows = append(rows, doctorRow{index: idx})
		}
	}

	// Loading group at the end
	if len(loadingIndices) > 0 && m.statusFilter == "all" {
		rows = append(rows, doctorRow{isHeader: true, status: "Loading", count: len(loadingIndices)})
		for _, idx := range loadingIndices {
			rows = append(rows, doctorRow{index: idx})
		}
	}

	return rows
}

func (m *doctorTUIModel) nonHeaderRows(rows []doctorRow) []int {
	var out []int
	for i, r := range rows {
		if !r.isHeader {
			out = append(out, i)
		}
	}
	return out
}

func (m *doctorTUIModel) moveCursor(delta int) {
	rows := m.buildVisibleRows()
	indices := m.nonHeaderRows(rows)
	if len(indices) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(indices) {
		m.cursor = len(indices) - 1
	}
}

func (m *doctorTUIModel) cursorCheckIndex() int {
	rows := m.buildVisibleRows()
	indices := m.nonHeaderRows(rows)
	if len(indices) == 0 {
		return -1
	}
	c := m.cursor
	if c < 0 {
		c = 0
	}
	if c >= len(indices) {
		c = len(indices) - 1
	}
	return rows[indices[c]].index
}

func doctorSeverityHeaderStyle(status checkStatus) lipgloss.Style {
	col := lipgloss.Color("7")
	switch status {
	case checkFail:
		col = lipgloss.Color("1")
	case checkWarn:
		col = lipgloss.Color("3")
	case checkInfo:
		col = lipgloss.Color("4")
	case checkOK:
		col = lipgloss.Color("2")
	}
	return lipgloss.NewStyle().Bold(true).Foreground(col)
}

func (m doctorTUIModel) View() string {
	var sb strings.Builder

	rows := m.buildVisibleRows()
	nonHeaderIndices := m.nonHeaderRows(rows)

	// Count completed checks
	doneCount := 0
	for _, c := range m.checks {
		if !c.loading {
			doneCount++
		}
	}

	// Title bar
	sb.WriteString(styleTitle.Render(" cq doctor "))
	sb.WriteString(" ")
	sb.WriteString(styleCount.Render(fmt.Sprintf("%d checks  %d/%d done",
		len(m.checks), doneCount, len(m.checks))))
	sb.WriteString("\n")

	// Search bar
	if m.query != "" {
		sb.WriteString("  ")
		sb.WriteString(styleSearchBar.Render(fmt.Sprintf(" 🔍 %s▏ ", m.query)))
	} else {
		sb.WriteString("  ")
		sb.WriteString(styleSearchPlaceholder.Render(" 🔍 type to search... "))
	}

	// Filter badge
	filterLabel := m.statusFilter
	if filterLabel == "" {
		filterLabel = "all"
	}
	var filterBadge lipgloss.Style
	switch filterLabel {
	case "FAIL":
		filterBadge = severityBadgeStyles[checkFail]
	case "WARN":
		filterBadge = severityBadgeStyles[checkWarn]
	case "INFO":
		filterBadge = severityBadgeStyles[checkInfo]
	case "OK":
		filterBadge = severityBadgeStyles[checkOK]
	default:
		filterBadge = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("15")).Padding(0, 1)
	}
	sb.WriteString("  ")
	sb.WriteString(filterBadge.Render(filterLabel))
	sb.WriteString("\n\n")

	// Rows
	const nameColW = 20
	cursorIdx := -1
	if len(nonHeaderIndices) > 0 {
		c := m.cursor
		if c < 0 {
			c = 0
		}
		if c >= len(nonHeaderIndices) {
			c = len(nonHeaderIndices) - 1
		}
		cursorIdx = nonHeaderIndices[c]
	}

	nonHeaderCount := 0

	for i, row := range rows {
		if row.isHeader {
			hs := doctorSeverityHeaderStyle(row.status)
			label := fmt.Sprintf(" ── %s (%d) ", string(row.status), row.count)
			sb.WriteString(hs.Render(label))
			headerW := m.width
			if headerW < 74 {
				headerW = 74
			}
			remaining := headerW - lipgloss.Width(label)
			if remaining > 0 {
				sb.WriteString(styleFaint.Render(strings.Repeat("─", remaining)))
			}
			sb.WriteString("\n")
			continue
		}

		nonHeaderCount++
		isSelected := i == cursorIdx
		item := m.checks[row.index]

		cursor := "   "
		if isSelected {
			cursor = " ▸ "
		}

		namePadded := lsPadToWidth(item.entry.Name, nameColW)

		if item.loading {
			// Loading row: spinner + "checking..."
			spinner := spinnerFrames[m.spinnerFrame]
			if isSelected {
				sb.WriteString(styleSelected.Render(fmt.Sprintf("%s%s  %s checking...", cursor, namePadded, spinner)))
				pad := m.width - 3 - nameColW - 2 - lsDispWidth(spinner) - 12
				if pad > 0 {
					sb.WriteString(styleSelected.Render(strings.Repeat(" ", pad)))
				}
			} else {
				sb.WriteString(cursor)
				sb.WriteString(styleFaint.Render(namePadded))
				sb.WriteString("  ")
				sb.WriteString(styleFaint.Render(spinner + " checking..."))
			}
			sb.WriteString("\n")
			continue
		}

		// Completed check
		msgColW := m.width - 3 - nameColW - 2 - 8 // cursor + name + gaps + badge
		if msgColW < 20 {
			msgColW = 20
		}
		if msgColW > 60 {
			msgColW = 60
		}

		msgDisplay := item.result.Message
		if lsDispWidth(msgDisplay) > msgColW {
			msgDisplay = lsTruncateToWidth(msgDisplay, msgColW-1) + "…"
		}
		msgPadded := lsPadToWidth(msgDisplay, msgColW)

		badge := ""
		if bStyle, ok := severityBadgeStyles[item.result.Status]; ok {
			badge = bStyle.Render(string(item.result.Status))
		}

		if isSelected {
			sb.WriteString(styleSelected.Render(cursor))
			sb.WriteString(styleSelected.Render(namePadded))
			sb.WriteString(styleSelected.Render("  "))
			sb.WriteString(styleSelected.Render(msgPadded))
			sb.WriteString(styleSelected.Render(" "))
			sb.WriteString(badge)
			used := 3 + nameColW + 2 + msgColW + 1 + lipgloss.Width(badge)
			if pad := m.width - used; pad > 0 {
				sb.WriteString(styleSelected.Render(strings.Repeat(" ", pad)))
			}
		} else {
			sb.WriteString(cursor)
			sb.WriteString(styleTagName.Render(namePadded))
			sb.WriteString("  ")
			sb.WriteString(styleSummary.Render(msgPadded))
			sb.WriteString(" ")
			sb.WriteString(badge)
		}
		sb.WriteString("\n")
	}

	if nonHeaderCount == 0 {
		sb.WriteString("\n")
		sb.WriteString(styleFaint.Render("  No checks match your filter."))
		sb.WriteString("\n")
	}

	// Fill remaining space to pin help bar at bottom
	content := sb.String()
	contentLines := strings.Count(content, "\n")
	if m.height > 0 {
		gap := m.height - contentLines - 2
		for i := 0; i < gap; i++ {
			sb.WriteString("\n")
		}
	}

	// Separator + help bar
	if m.width > 0 {
		sb.WriteString(styleFaint.Render(strings.Repeat("─", m.width)))
	} else {
		sb.WriteString(styleFaint.Render(strings.Repeat("─", 74)))
	}
	sb.WriteString("\n")

	var helpBar strings.Builder
	helpBar.WriteString(" ")
	helpBar.WriteString(helpEntry("↑↓", "navigate"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("Enter", "fix"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("→", "detail"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("r", "recheck"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("Tab", "filter"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("q", "quit"))
	sb.WriteString(helpBar.String())

	return sb.String()
}

// runDoctorTUI launches the interactive Bubble Tea TUI for doctor.
func runDoctorTUI() error {
	m := newDoctorTUIModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
