package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// sourceBadgeStyles maps config source to colored badge styles.
var sourceBadgeStyles = map[string]lipgloss.Style{
	"default": lipgloss.NewStyle().Faint(true).Padding(0, 1),
	"project": lipgloss.NewStyle().Background(lipgloss.Color("2")).Foreground(lipgloss.Color("0")).Padding(0, 1),
	"global":  lipgloss.NewStyle().Background(lipgloss.Color("4")).Foreground(lipgloss.Color("15")).Padding(0, 1),
	"env":     lipgloss.NewStyle().Background(lipgloss.Color("3")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1),
}

// configTUIModel is the bubbletea model for the config TUI.
type configTUIModel struct {
	entries       []configEntry
	rows          []configRow
	cursor        int
	query         string
	sectionFilter string // "all" or section name
	width, height int
}

// configRow represents a row in the config TUI (either a section header or an entry).
type configRow struct {
	isHeader bool
	section  string
	count    int
	index    int // into entries (for non-header)
}

func newConfigTUIModel(entries []configEntry) configTUIModel {
	m := configTUIModel{
		entries:       entries,
		sectionFilter: "all",
	}
	m.rows = m.buildVisibleRows()
	return m
}

func (m configTUIModel) Init() tea.Cmd {
	return nil
}

func (m configTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.rows = m.buildVisibleRows()
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp:
			m.moveCursorConfig(-1)
		case tea.KeyDown:
			m.moveCursorConfig(1)
		case tea.KeyTab:
			m.cycleSectionFilter()
			m.cursor = 0
			m.rows = m.buildVisibleRows()
		case tea.KeyEsc:
			if m.query != "" {
				m.query = ""
				m.cursor = 0
				m.rows = m.buildVisibleRows()
				return m, nil
			}
			return m, tea.Quit
		case tea.KeyBackspace:
			if len(m.query) > 0 {
				runes := []rune(m.query)
				m.query = string(runes[:len(runes)-1])
				m.cursor = 0
				m.rows = m.buildVisibleRows()
			}
		case tea.KeyRunes:
			ch := msg.String()
			switch ch {
			case "k":
				if m.query == "" {
					m.moveCursorConfig(-1)
					return m, nil
				}
				m.query += ch
				m.cursor = 0
				m.rows = m.buildVisibleRows()
			case "j":
				if m.query == "" {
					m.moveCursorConfig(1)
					return m, nil
				}
				m.query += ch
				m.cursor = 0
				m.rows = m.buildVisibleRows()
			case "q":
				if m.query == "" {
					return m, tea.Quit
				}
				m.query += ch
				m.cursor = 0
				m.rows = m.buildVisibleRows()
			case " ", "a", "d", "e":
				// no-op for now (T-908)
				if m.query != "" {
					m.query += ch
					m.cursor = 0
					m.rows = m.buildVisibleRows()
				}
			default:
				m.query += ch
				m.cursor = 0
				m.rows = m.buildVisibleRows()
			}
		case tea.KeyEnter:
			// no-op for now (T-908)
		}
	}
	return m, nil
}

// cycleSectionFilter cycles through "all" + unique sections from entries.
func (m *configTUIModel) cycleSectionFilter() {
	cycle := m.sectionCycle()
	cur := 0
	for i, v := range cycle {
		if v == m.sectionFilter {
			cur = i
			break
		}
	}
	m.sectionFilter = cycle[(cur+1)%len(cycle)]
}

// sectionCycle returns ["all", section1, section2, ...] in order.
func (m *configTUIModel) sectionCycle() []string {
	seen := make(map[string]bool)
	var sections []string
	for _, e := range m.entries {
		if !seen[e.Section] {
			seen[e.Section] = true
			sections = append(sections, e.Section)
		}
	}
	cycle := make([]string, 0, len(sections)+1)
	cycle = append(cycle, "all")
	cycle = append(cycle, sections...)
	return cycle
}

// buildVisibleRows groups entries by section, applies filters and search.
func (m *configTUIModel) buildVisibleRows() []configRow {
	lowerQuery := strings.ToLower(m.query)

	// Group entries by section, preserving order
	type group struct {
		section string
		indices []int
	}
	var groups []group
	groupMap := make(map[string]int) // section -> index in groups

	for i, e := range m.entries {
		// Apply section filter
		if m.sectionFilter != "all" && e.Section != m.sectionFilter {
			continue
		}
		// Apply search query
		if lowerQuery != "" {
			corpus := strings.ToLower(e.Key + " " + configValueString(e))
			if !strings.Contains(corpus, lowerQuery) {
				continue
			}
		}

		if idx, ok := groupMap[e.Section]; ok {
			groups[idx].indices = append(groups[idx].indices, i)
		} else {
			groupMap[e.Section] = len(groups)
			groups = append(groups, group{section: e.Section, indices: []int{i}})
		}
	}

	var rows []configRow
	for _, g := range groups {
		rows = append(rows, configRow{isHeader: true, section: g.section, count: len(g.indices)})
		for _, idx := range g.indices {
			rows = append(rows, configRow{index: idx})
		}
	}
	return rows
}

// configNonHeaderRows returns indices of non-header rows.
func configNonHeaderRows(rows []configRow) []int {
	var out []int
	for i, r := range rows {
		if !r.isHeader {
			out = append(out, i)
		}
	}
	return out
}

func (m *configTUIModel) moveCursorConfig(delta int) {
	m.rows = m.buildVisibleRows()
	indices := configNonHeaderRows(m.rows)
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

// configValueString returns the display string for a config entry value.
func configValueString(e configEntry) string {
	switch e.Kind {
	case "bool":
		if v, ok := e.Value.(bool); ok {
			if v {
				return "true"
			}
			return "false"
		}
		return fmt.Sprint(e.Value)
	case "int":
		return fmt.Sprint(e.Value)
	case "array":
		return fmt.Sprintf("[%d items]", arrayLen(e.Value))
	default:
		return fmt.Sprint(e.Value)
	}
}

// arrayLen returns the length of a slice value via reflection-free type assertions.
func arrayLen(v any) int {
	switch arr := v.(type) {
	case []string:
		return len(arr)
	case []int:
		return len(arr)
	case []any:
		return len(arr)
	default:
		// fallback: just say 0
		return 0
	}
}

func configSectionHeaderStyle(section string) lipgloss.Style {
	// Rotate through a few colors based on section name hash
	colors := []lipgloss.Color{"6", "5", "3", "4", "2", "1"}
	h := 0
	for _, c := range section {
		h += int(c)
	}
	col := colors[h%len(colors)]
	return lipgloss.NewStyle().Bold(true).Foreground(col)
}

func (m configTUIModel) View() string {
	var sb strings.Builder

	rows := m.rows
	nonHeaderIndices := configNonHeaderRows(rows)

	// Count unique sections
	sectionSet := make(map[string]bool)
	for _, e := range m.entries {
		sectionSet[e.Section] = true
	}

	// Title bar
	sb.WriteString(styleTitle.Render(" cq config "))
	sb.WriteString(" ")
	sb.WriteString(styleCount.Render(fmt.Sprintf("%d sections  %d keys",
		len(sectionSet), len(m.entries))))
	sb.WriteString("\n")

	// Search bar
	if m.query != "" {
		sb.WriteString("  ")
		sb.WriteString(styleSearchBar.Render(fmt.Sprintf(" \U0001F50D %s\u25CF ", m.query)))
	} else {
		sb.WriteString("  ")
		sb.WriteString(styleSearchPlaceholder.Render(" \U0001F50D type to search... "))
	}

	// Section filter badge
	filterLabel := m.sectionFilter
	if filterLabel == "" {
		filterLabel = "all"
	}
	var filterBadge lipgloss.Style
	if filterLabel == "all" {
		filterBadge = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("15")).Padding(0, 1)
	} else {
		filterBadge = lipgloss.NewStyle().Background(lipgloss.Color("6")).Foreground(lipgloss.Color("0")).Padding(0, 1)
	}
	sb.WriteString("  ")
	sb.WriteString(filterBadge.Render(filterLabel+" \u25BE"))
	sb.WriteString("\n\n")

	// Column layout: cursor(3) + key(28) + sp(1) + value(dynamic) + pad + source(8)
	const keyColW = 28
	const sourceColW = 8
	fixedW := 3 + keyColW + 1 + 1 + sourceColW + 1
	valColW := m.width - fixedW
	if valColW < 12 {
		valColW = 12
	}
	if valColW > 40 {
		valColW = 40
	}

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
			hs := configSectionHeaderStyle(row.section)
			label := fmt.Sprintf(" \u2500\u2500 %s (%d) ", row.section, row.count)
			sb.WriteString(hs.Render(label))
			headerW := m.width
			if headerW < 74 {
				headerW = 74
			}
			remaining := headerW - lipgloss.Width(label)
			if remaining > 0 {
				sb.WriteString(styleFaint.Render(strings.Repeat("\u2500", remaining)))
			}
			sb.WriteString("\n")
			continue
		}

		nonHeaderCount++
		isSelected := i == cursorIdx
		entry := m.entries[row.index]

		cursor := "   "
		if isSelected {
			cursor = " \u25B8 "
		}

		// Key column: truncate if needed
		keyDisplay := entry.Key
		if lsDispWidth(keyDisplay) > keyColW {
			keyDisplay = lsTruncateToWidth(keyDisplay, keyColW-2) + ".."
		}
		keyPadded := lsPadToWidth(keyDisplay, keyColW)

		// Value column
		valDisplay := configValueString(entry)
		if lsDispWidth(valDisplay) > valColW {
			valDisplay = lsTruncateToWidth(valDisplay, valColW-1) + "\u2026"
		}

		// Source badge
		sourceBadge := entry.Source
		if bStyle, ok := sourceBadgeStyles[entry.Source]; ok {
			sourceBadge = bStyle.Render(entry.Source)
		}

		// Calculate padding between value and source to right-align source
		leftUsed := 3 + keyColW + 1 + lsDispWidth(valDisplay)
		sourceVisW := lipgloss.Width(sourceBadge)
		midPad := m.width - leftUsed - sourceVisW - 1
		if midPad < 1 {
			midPad = 1
		}

		if isSelected {
			sb.WriteString(styleSelected.Render(cursor))
			sb.WriteString(styleSelected.Render(keyPadded))
			sb.WriteString(styleSelected.Render(" "))
			sb.WriteString(styleSelected.Render(valDisplay))
			sb.WriteString(styleSelected.Render(strings.Repeat(" ", midPad)))
			sb.WriteString(sourceBadge)
			trailing := m.width - leftUsed - midPad - sourceVisW
			if trailing > 0 {
				sb.WriteString(styleSelected.Render(strings.Repeat(" ", trailing)))
			}
		} else {
			sb.WriteString(cursor)
			sb.WriteString(styleTagName.Render(keyPadded))
			sb.WriteString(" ")
			sb.WriteString(styleSummary.Render(valDisplay))
			sb.WriteString(strings.Repeat(" ", midPad))
			sb.WriteString(sourceBadge)
		}
		sb.WriteString("\n")
	}

	if nonHeaderCount == 0 {
		sb.WriteString("\n")
		sb.WriteString(styleFaint.Render("  No config entries match your filter."))
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
		sb.WriteString(styleFaint.Render(strings.Repeat("\u2500", m.width)))
	} else {
		sb.WriteString(styleFaint.Render(strings.Repeat("\u2500", 74)))
	}
	sb.WriteString("\n")

	var helpBar strings.Builder
	helpBar.WriteString(" ")
	helpBar.WriteString(helpEntry("\u2191\u2193", "navigate"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("Space", "toggle"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("Enter", "edit"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("Tab", "filter"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("q", "quit"))
	sb.WriteString(helpBar.String())

	return sb.String()
}
