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
	category string // header label: "Skills", "Agents", "Rules", "CLAUDE.md"
	count    int    // number of items in this category
	preset   craft.Preset
}

// craftTUIModel is the bubbletea model for the `cq add` TUI.
type craftTUIModel struct {
	presets    []craft.Preset
	rows       []craftRow
	cursor     int // index into nonHeaderIndices()
	query      string
	searchMode bool
	width      int
	height     int
	selected   *craft.Preset // set on Enter; nil means cancelled
	err        error
}

func newCraftTUIModel(presets []craft.Preset) craftTUIModel {
	m := craftTUIModel{presets: presets}
	m.rows = buildCraftRows(presets, "")
	return m
}

// buildCraftRows groups presets by category, filtering by query, and returns
// a flat list of header + preset rows.
func buildCraftRows(presets []craft.Preset, query string) []craftRow {
	lq := strings.ToLower(query)

	byType := map[craft.PresetType][]craft.Preset{
		craft.TypeSkill:    {},
		craft.TypeAgent:    {},
		craft.TypeRule:     {},
		craft.TypeClaudeMd: {},
	}
	for _, p := range presets {
		if lq != "" {
			corpus := strings.ToLower(p.Name) + " " + strings.ToLower(p.Description)
			if !strings.Contains(corpus, lq) {
				continue
			}
		}
		byType[p.Type] = append(byType[p.Type], p)
	}

	order := []struct {
		t     craft.PresetType
		label string
	}{
		{craft.TypeSkill, "Skills"},
		{craft.TypeAgent, "Agents"},
		{craft.TypeRule, "Rules"},
		{craft.TypeClaudeMd, "CLAUDE.md"},
	}

	var rows []craftRow
	for _, cat := range order {
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

// nonHeaderIndices returns the row indices that are preset entries (not headers).
func (m *craftTUIModel) nonHeaderIndices() []int {
	var out []int
	for i, r := range m.rows {
		if !r.isHeader {
			out = append(out, i)
		}
	}
	return out
}

// cursorRowIndex converts the cursor (index into non-header items) to an index
// into m.rows. Returns -1 when no preset rows exist.
func (m *craftTUIModel) cursorRowIndex() int {
	indices := m.nonHeaderIndices()
	if len(indices) == 0 || m.cursor < 0 || m.cursor >= len(indices) {
		return -1
	}
	return indices[m.cursor]
}

// moveCursor advances the cursor by delta, skipping header rows.
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

// jumpToNextCategory advances the cursor to the first item of the next category.
func (m *craftTUIModel) jumpToNextCategory() {
	indices := m.nonHeaderIndices()
	if len(indices) == 0 {
		return
	}
	curRowIdx := m.cursorRowIndex()
	if curRowIdx < 0 {
		return
	}
	// Find the next header after curRowIdx.
	for i := curRowIdx + 1; i < len(m.rows); i++ {
		if m.rows[i].isHeader {
			// The item right after this header.
			if i+1 < len(m.rows) && !m.rows[i+1].isHeader {
				for ci, ri := range indices {
					if ri == i+1 {
						m.cursor = ci
						return
					}
				}
			}
		}
	}
	// Wrap to first item.
	m.cursor = 0
}

func (m craftTUIModel) Init() tea.Cmd {
	return nil
}

func (m craftTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Search mode: typing
		if m.searchMode {
			switch msg.Type {
			case tea.KeyEsc:
				m.searchMode = false
				m.query = ""
				m.rows = buildCraftRows(m.presets, "")
				m.cursor = 0
			case tea.KeyEnter:
				m.searchMode = false
			case tea.KeyBackspace:
				if len(m.query) > 0 {
					runes := []rune(m.query)
					m.query = string(runes[:len(runes)-1])
				}
				m.rows = buildCraftRows(m.presets, m.query)
				m.cursor = 0
			case tea.KeyRunes:
				m.query += msg.String()
				m.rows = buildCraftRows(m.presets, m.query)
				m.cursor = 0
			}
			return m, nil
		}

		// Normal mode
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
			return m, tea.Quit
		case tea.KeyRunes:
			switch msg.String() {
			case "k":
				m.moveCursor(-1)
			case "j":
				m.moveCursor(1)
			case "/":
				m.searchMode = true
			case "q":
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

// --- Craft TUI lipgloss styles (prefixed with craft to avoid collision) ---

var (
	craftStyleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	craftStyleSelected = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("15")).
				Bold(true)

	craftStyleHeader = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("14"))

	craftStyleName = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15"))

	craftStyleDesc = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	craftStyleDescDim = lipgloss.NewStyle().
				Faint(true)

	craftStyleFaint = lipgloss.NewStyle().Faint(true)

	craftStyleSearchBar = lipgloss.NewStyle().
				Foreground(lipgloss.Color("14")).
				Bold(true)

	craftStyleSearchPlaceholder = lipgloss.NewStyle().
					Faint(true).
					Italic(true)

	craftStyleHelpKey = lipgloss.NewStyle().
				Foreground(lipgloss.Color("14")).
				Bold(true)

	craftStyleHelpDesc = lipgloss.NewStyle().
				Faint(true)
)

func craftHelpEntry(key, desc string) string {
	return craftStyleHelpKey.Render(key) + " " + craftStyleHelpDesc.Render(desc)
}

func (m craftTUIModel) View() string {
	var sb strings.Builder

	// Title bar
	total := 0
	for _, r := range m.rows {
		if !r.isHeader {
			total++
		}
	}
	countLabel := craftStyleFaint.Render(fmt.Sprintf("%d presets available", total))
	var searchLabel string
	if m.searchMode {
		searchLabel = craftStyleSearchBar.Render(fmt.Sprintf(" / %s▏", m.query))
	} else if m.query != "" {
		searchLabel = craftStyleSearchBar.Render(fmt.Sprintf(" 🔍 %s", m.query))
	} else {
		searchLabel = craftStyleSearchPlaceholder.Render(" / search")
	}

	titleWidth := m.width
	if titleWidth < 60 {
		titleWidth = 60
	}
	titleLine := craftStyleTitle.Render(" CQ Craft ") + "  " + countLabel + "  " + searchLabel
	sb.WriteString(titleLine)
	sb.WriteString("\n")

	// Separator
	if m.width > 0 {
		sb.WriteString(craftStyleFaint.Render(strings.Repeat("─", titleWidth)))
	}
	sb.WriteString("\n")

	// Viewport: scroll to keep cursor visible
	const nameColW = 24
	// Layout: cursor(3) + name(nameColW) + gap(2) + desc(dynamic)
	fixedW := 3 + nameColW + 2
	descColW := m.width - fixedW
	if descColW < 10 {
		descColW = 10
	}

	cursorRowIdx := m.cursorRowIndex()
	maxVisible := m.height - 6 // title(1) + sep(1) + sep(1) + help(1) + margin(2)
	if maxVisible < 5 {
		maxVisible = 5
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
		sb.WriteString(craftStyleFaint.Render(fmt.Sprintf("  ▲ %d more", viewStart)))
		sb.WriteString("\n")
	}

	for i := viewStart; i < viewEnd; i++ {
		row := m.rows[i]

		if row.isHeader {
			label := fmt.Sprintf("  %s (%d)", row.category, row.count)
			headerLine := craftStyleHeader.Render(label)
			hdrW := titleWidth - lipgloss.Width(label)
			if hdrW > 0 {
				headerLine += craftStyleFaint.Render(strings.Repeat(" ", hdrW))
			}
			sb.WriteString(headerLine)
			sb.WriteString("\n")
			continue
		}

		isSelected := i == cursorRowIdx

		cursor := "   "
		if isSelected {
			cursor = " ▸ "
		}

		// Name column (CJK-aware truncation + pad)
		nameDisplay := row.preset.Name
		if lsDispWidth(nameDisplay) > nameColW {
			nameDisplay = lsTruncateToWidth(nameDisplay, nameColW-1) + "…"
		}
		namePadded := lsPadToWidth(nameDisplay, nameColW)

		// Description (truncate)
		descDisplay := row.preset.Description
		if lsDispWidth(descDisplay) > descColW {
			descDisplay = lsTruncateToWidth(descDisplay, descColW-1) + "…"
		}

		if isSelected {
			line := craftStyleSelected.Render(cursor + namePadded + "  " + descDisplay)
			// Pad to full width so background extends across the row
			lineW := lsDispWidth(cursor+namePadded+"  "+descDisplay)
			padW := m.width - lineW
			if padW > 0 {
				line += craftStyleSelected.Render(strings.Repeat(" ", padW))
			}
			sb.WriteString(line)
		} else {
			sb.WriteString(cursor)
			sb.WriteString(craftStyleName.Render(namePadded))
			sb.WriteString("  ")
			if descDisplay != "" {
				sb.WriteString(craftStyleDesc.Render(descDisplay))
			}
		}
		sb.WriteString("\n")
	}

	if viewEnd < len(m.rows) {
		sb.WriteString(craftStyleFaint.Render(fmt.Sprintf("  ▼ %d more", len(m.rows)-viewEnd)))
		sb.WriteString("\n")
	}

	if total == 0 {
		sb.WriteString("\n")
		sb.WriteString(craftStyleFaint.Render("  No presets match your search."))
		sb.WriteString("\n")
	}

	// Help bar — pin to bottom
	content := sb.String()
	contentLines := strings.Count(content, "\n")
	if m.height > 0 {
		gap := m.height - contentLines - 2
		for i := 0; i < gap; i++ {
			sb.WriteString("\n")
		}
	}

	if m.width > 0 {
		sb.WriteString(craftStyleFaint.Render(strings.Repeat("─", titleWidth)))
	}
	sb.WriteString("\n")

	var helpBar strings.Builder
	helpBar.WriteString(" ")
	helpBar.WriteString(craftHelpEntry("↑↓/jk", "이동"))
	helpBar.WriteString("  ")
	helpBar.WriteString(craftHelpEntry("Enter", "설치"))
	helpBar.WriteString("  ")
	helpBar.WriteString(craftHelpEntry("/", "검색"))
	helpBar.WriteString("  ")
	helpBar.WriteString(craftHelpEntry("Tab", "카테고리"))
	helpBar.WriteString("  ")
	helpBar.WriteString(craftHelpEntry("q/Esc", "종료"))
	sb.WriteString(helpBar.String())

	return sb.String()
}
