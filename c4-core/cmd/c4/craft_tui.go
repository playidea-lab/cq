package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/changmin/c4-core/internal/craft"
)

// craftRow is a single display row: either a category header or a preset entry.
type craftRow struct {
	isHeader bool
	category string         // header label
	count    int            // number of items in this category
	preset   craft.Preset
}

// craftTUIModel is the bubbletea model for the `cq add` TUI.
type craftTUIModel struct {
	presets  []craft.Preset
	rows     []craftRow
	cursor   int
	query    string
	width    int
	height   int
	selected *craft.Preset
}

func newCraftTUIModel(presets []craft.Preset) craftTUIModel {
	m := craftTUIModel{presets: presets}
	m.rows = buildCraftRows(presets, "")
	return m
}

// categoryOrder defines display order and styling for preset types.
var categoryOrder = []struct {
	t     craft.PresetType
	label string
	color lipgloss.Color
}{
	{craft.TypeSkill, "Skills", lipgloss.Color("4")},    // blue
	{craft.TypeAgent, "Agents", lipgloss.Color("2")},    // green
	{craft.TypeRule, "Rules", lipgloss.Color("3")},      // yellow
	{craft.TypeClaudeMd, "CLAUDE.md", lipgloss.Color("5")}, // magenta
}

// Type badge styles (colored pills like session status badges)
var craftBadgeStyles = map[craft.PresetType]lipgloss.Style{
	craft.TypeSkill:    lipgloss.NewStyle().Background(lipgloss.Color("4")).Foreground(lipgloss.Color("15")).Bold(true).Padding(0, 1),
	craft.TypeAgent:    lipgloss.NewStyle().Background(lipgloss.Color("2")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1),
	craft.TypeRule:     lipgloss.NewStyle().Background(lipgloss.Color("3")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1),
	craft.TypeClaudeMd: lipgloss.NewStyle().Background(lipgloss.Color("5")).Foreground(lipgloss.Color("15")).Bold(true).Padding(0, 1),
}

var craftBadgeLabels = map[craft.PresetType]string{
	craft.TypeSkill:    "  skill  ",
	craft.TypeAgent:    "  agent  ",
	craft.TypeRule:     "  rule   ",
	craft.TypeClaudeMd: "claude-md",
}

func buildCraftRows(presets []craft.Preset, query string) []craftRow {
	lq := strings.ToLower(query)

	byType := map[craft.PresetType][]craft.Preset{}
	for _, p := range presets {
		if lq != "" {
			corpus := strings.ToLower(p.Name) + " " + strings.ToLower(p.Description)
			match := true
			for _, word := range strings.Fields(lq) {
				if !strings.Contains(corpus, word) {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}
		byType[p.Type] = append(byType[p.Type], p)
	}

	var rows []craftRow
	for _, cat := range categoryOrder {
		items := byType[cat.t]
		if len(items) == 0 {
			continue
		}
		rows = append(rows, craftRow{
			isHeader: true,
			category: cat.label,
			count:    len(items),
		})
		for _, p := range items {
			rows = append(rows, craftRow{preset: p})
		}
	}
	return rows
}

func (m *craftTUIModel) nonHeaderIndices() []int {
	var out []int
	for i, r := range m.rows {
		if !r.isHeader {
			out = append(out, i)
		}
	}
	return out
}

func (m *craftTUIModel) cursorRowIndex() int {
	indices := m.nonHeaderIndices()
	if len(indices) == 0 || m.cursor < 0 || m.cursor >= len(indices) {
		return -1
	}
	return indices[m.cursor]
}

func (m *craftTUIModel) moveCursor(delta int) {
	indices := m.nonHeaderIndices()
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

func (m *craftTUIModel) jumpToNextCategory() {
	indices := m.nonHeaderIndices()
	if len(indices) == 0 {
		return
	}
	curRowIdx := m.cursorRowIndex()
	if curRowIdx < 0 {
		return
	}
	for i := curRowIdx + 1; i < len(m.rows); i++ {
		if m.rows[i].isHeader && i+1 < len(m.rows) && !m.rows[i+1].isHeader {
			for ci, ri := range indices {
				if ri == i+1 {
					m.cursor = ci
					return
				}
			}
		}
	}
	m.cursor = 0
}

func (m *craftTUIModel) rebuildRows() {
	m.rows = buildCraftRows(m.presets, m.query)
	indices := m.nonHeaderIndices()
	if m.cursor >= len(indices) {
		if len(indices) > 0 {
			m.cursor = len(indices) - 1
		} else {
			m.cursor = 0
		}
	}
}

func (m craftTUIModel) Init() tea.Cmd { return nil }

func (m craftTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
		case tea.KeyTab:
			m.jumpToNextCategory()
		case tea.KeyEnter:
			idx := m.cursorRowIndex()
			if idx >= 0 {
				p := m.rows[idx].preset
				m.selected = &p
			}
			return m, tea.Quit
		case tea.KeyEsc:
			if m.query != "" {
				m.query = ""
				m.rebuildRows()
				return m, nil
			}
			return m, tea.Quit
		case tea.KeyBackspace:
			if len(m.query) > 0 {
				runes := []rune(m.query)
				m.query = string(runes[:len(runes)-1])
				m.rebuildRows()
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
				m.rebuildRows()
			case "j":
				if m.query == "" {
					m.moveCursor(1)
					return m, nil
				}
				m.query += ch
				m.rebuildRows()
			case "q":
				if m.query == "" {
					return m, tea.Quit
				}
				m.query += ch
				m.rebuildRows()
			default:
				m.query += ch
				m.rebuildRows()
			}
		}
	}
	return m, nil
}

// --- Lipgloss styles (craft-prefixed to avoid collision) ---

func craftGroupHeaderStyle(cat string) lipgloss.Style {
	col := lipgloss.Color("7")
	for _, c := range categoryOrder {
		if c.label == cat {
			col = c.color
			break
		}
	}
	return lipgloss.NewStyle().Bold(true).Foreground(col)
}

func (m craftTUIModel) View() string {
	var sb strings.Builder
	w := m.width
	if w < 74 {
		w = 74
	}

	// Title bar
	total := 0
	for _, r := range m.rows {
		if !r.isHeader {
			total++
		}
	}
	sb.WriteString(styleTitle.Render(" cq add "))
	sb.WriteString(" ")
	sb.WriteString(styleCount.Render(fmt.Sprintf("%d presets", total)))
	sb.WriteString("\n\n")

	// Search bar (inline, like sessions — type to search)
	if m.query != "" {
		sb.WriteString("  ")
		sb.WriteString(styleSearchBar.Render(fmt.Sprintf(" 🔍 %s▏ ", m.query)))
	} else {
		sb.WriteString("  ")
		sb.WriteString(styleSearchPlaceholder.Render(" 🔍 type to search... "))
	}
	sb.WriteString("\n\n")

	// Columns: cursor(3) + name(20) + sp(1) + badge(11) + sp(1) + desc(dynamic)
	const nameColW = 20
	const badgeColW = 11
	fixedW := 3 + nameColW + 1 + badgeColW + 1
	descColW := w - fixedW
	if descColW < 10 {
		descColW = 10
	}

	cursorRowIdx := m.cursorRowIndex()

	// Viewport
	maxVisible := m.height - 8
	if maxVisible < 10 {
		maxVisible = 10
	}
	viewStart := 0
	viewEnd := len(m.rows)
	if len(m.rows) > maxVisible {
		viewEnd = viewStart + maxVisible
		if cursorRowIdx >= viewEnd {
			viewStart = cursorRowIdx - maxVisible + 3
			if viewStart < 0 {
				viewStart = 0
			}
			viewEnd = viewStart + maxVisible
			if viewEnd > len(m.rows) {
				viewEnd = len(m.rows)
			}
		}
	}

	if viewStart > 0 {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("  ▲ %d more", viewStart)))
		sb.WriteString("\n")
	}

	nonHeaderCount := 0
	for i := viewStart; i < viewEnd; i++ {
		row := m.rows[i]

		if row.isHeader {
			hs := craftGroupHeaderStyle(row.category)
			label := fmt.Sprintf(" ── %s (%d) ", row.category, row.count)
			sb.WriteString(hs.Render(label))
			remaining := w - lipgloss.Width(label)
			if remaining > 0 {
				sb.WriteString(styleFaint.Render(strings.Repeat("─", remaining)))
			}
			sb.WriteString("\n")
			continue
		}

		isSelected := i == cursorRowIdx
		nonHeaderCount++

		cursor := "   "
		if isSelected {
			cursor = " ▸ "
		}

		// Name (CJK-aware)
		nameDisplay := row.preset.Name
		if lsDispWidth(nameDisplay) > nameColW {
			nameDisplay = lsTruncateToWidth(nameDisplay, nameColW-1) + "…"
		}
		namePadded := lsPadToWidth(nameDisplay, nameColW)

		// Type badge
		bStyle, ok := craftBadgeStyles[row.preset.Type]
		if !ok {
			bStyle = craftBadgeStyles[craft.TypeSkill]
		}
		badgeLabel, ok := craftBadgeLabels[row.preset.Type]
		if !ok {
			badgeLabel = "  other  "
		}
		badge := bStyle.Render(badgeLabel)

		// Description (truncate)
		descDisplay := row.preset.Description
		if lsDispWidth(descDisplay) > descColW {
			descDisplay = lsTruncateToWidth(descDisplay, descColW-1) + "…"
		}

		if isSelected {
			sb.WriteString(styleSelected.Render(cursor))
			sb.WriteString(styleSelected.Render(namePadded))
			sb.WriteString(styleSelected.Render(" "))
			sb.WriteString(badge)
			sb.WriteString(styleSelected.Render(" "))
			sb.WriteString(styleSelected.Render(descDisplay))
			// Pad to full width
			lineW := 3 + nameColW + 1 + lipgloss.Width(badge) + 1 + lsDispWidth(descDisplay)
			padW := w - lineW
			if padW > 0 {
				sb.WriteString(styleSelected.Render(strings.Repeat(" ", padW)))
			}
		} else {
			sb.WriteString(cursor)
			sb.WriteString(styleTagName.Render(namePadded))
			sb.WriteString(" ")
			sb.WriteString(badge)
			sb.WriteString(" ")
			sb.WriteString(styleSummary.Render(descDisplay))
		}
		sb.WriteString("\n")
	}

	if viewEnd < len(m.rows) {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("  ▼ %d more", len(m.rows)-viewEnd)))
		sb.WriteString("\n")
	}

	if nonHeaderCount == 0 {
		sb.WriteString("\n")
		sb.WriteString(styleFaint.Render("  No presets match your search."))
		sb.WriteString("\n")
	}

	// Pin help bar to bottom
	content := sb.String()
	contentLines := strings.Count(content, "\n")
	if m.height > 0 {
		gap := m.height - contentLines - 2
		for i := 0; i < gap; i++ {
			sb.WriteString("\n")
		}
	}

	if w > 0 {
		sb.WriteString(styleFaint.Render(strings.Repeat("─", w)))
	}
	sb.WriteString("\n")

	sb.WriteString(" ")
	sb.WriteString(helpEntry("↑↓", "move"))
	sb.WriteString("  ")
	sb.WriteString(helpEntry("Enter", "install"))
	sb.WriteString("  ")
	sb.WriteString(helpEntry("Tab", "category"))
	sb.WriteString("  ")
	sb.WriteString(helpEntry("Esc", "quit/clear"))

	return sb.String()
}
