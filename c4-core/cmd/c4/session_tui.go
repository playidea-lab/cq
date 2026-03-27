package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// statusOrder defines the display order for session groups.
var statusOrder = []string{"in-progress", "planned", "idea", "active", "done"}

// tuiRow represents a single row in the TUI list.
type tuiRow struct {
	isHeader bool
	// header fields
	status string
	count  int
	// session row fields
	tag     string
	summary string
	date    string
	rowStatus string
}

// sessionTUIModel is the bubbletea model for the session picker.
type sessionTUIModel struct {
	// raw session data
	sessions    map[string]namedSessionEntry
	searchIndex map[string]string // tag → lowercased search text

	// filtered/grouped rows to display
	rows []tuiRow

	// state
	query        string
	statusFilter string // "" means all
	cursor       int    // points to a non-header row by visual index

	// result
	selectedTag string
}

// filterCycle is the order of Tab-cycling for status filters.
var filterCycle = []string{"all", "in-progress", "planned", "idea", "active", "done"}

func newSessionTUIModel() sessionTUIModel {
	m := sessionTUIModel{
		statusFilter: "all",
	}
	m.sessions, _ = loadNamedSessions()
	m.searchIndex = buildSearchIndex(m.sessions)
	m.rows = buildRows(m.sessions, m.searchIndex, m.query, m.statusFilter)
	return m
}

// buildSearchIndex reads idea/spec file contents and builds a per-tag search corpus.
func buildSearchIndex(sessions map[string]namedSessionEntry) map[string]string {
	idx := make(map[string]string, len(sessions))
	ideasDir := filepath.Join(projectDir, ".c4", "ideas")
	specsDir := filepath.Join(projectDir, "docs", "specs")

	for tag, entry := range sessions {
		parts := []string{
			strings.ToLower(tag),
			strings.ToLower(entry.Summary),
			strings.ToLower(entry.Status),
		}

		// Idea slug: use stored Idea field or fuzzy-match
		ideaSlug := entry.Idea
		if ideaSlug == "" {
			ideaSlug, _ = matchIdeaByTag(tag)
		}
		if ideaSlug != "" {
			parts = append(parts, strings.ToLower(ideaSlug))
			ideaContent, err := os.ReadFile(filepath.Join(ideasDir, ideaSlug+".md"))
			if err == nil {
				parts = append(parts, strings.ToLower(string(ideaContent)))
			}
		}

		// Also scan specs dir for a file matching the tag
		if specEntries, err := os.ReadDir(specsDir); err == nil {
			lowerTag := strings.ToLower(tag)
			for _, e := range specEntries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
					continue
				}
				name := strings.ToLower(strings.TrimSuffix(e.Name(), ".md"))
				if strings.Contains(name, lowerTag) || strings.Contains(lowerTag, name) {
					content, err := os.ReadFile(filepath.Join(specsDir, e.Name()))
					if err == nil {
						parts = append(parts, strings.ToLower(string(content)))
					}
					break
				}
			}
		}

		idx[tag] = strings.Join(parts, " ")
	}
	return idx
}

// buildRows groups sessions and returns the display rows, applying query and status filter.
func buildRows(sessions map[string]namedSessionEntry, idx map[string]string, query, statusFilter string) []tuiRow {
	lowerQuery := strings.ToLower(query)

	// Collect matching tags per status
	byStatus := make(map[string][]string)
	for tag, entry := range sessions {
		status := entry.Status
		if status == "" {
			status = "active"
		}

		// Status filter
		if statusFilter != "" && statusFilter != "all" && status != statusFilter {
			continue
		}

		// Query filter
		if lowerQuery != "" {
			corpus := idx[tag]
			if !strings.Contains(corpus, lowerQuery) {
				continue
			}
		}

		byStatus[status] = append(byStatus[status], tag)
	}

	// Sort tags within each group alphabetically
	for status := range byStatus {
		sort.Strings(byStatus[status])
	}

	var rows []tuiRow
	for _, status := range statusOrder {
		tags, ok := byStatus[status]
		if !ok || len(tags) == 0 {
			continue
		}
		// Group header
		rows = append(rows, tuiRow{
			isHeader: true,
			status:   status,
			count:    len(tags),
		})
		// Session rows
		for _, tag := range tags {
			entry := sessions[tag]
			dateStr := "--"
			if t, err := time.Parse(time.RFC3339, entry.Updated); err == nil {
				dateStr = t.Format("Jan 02 15:04")
			}
			// Summary with fallback: summary → memo → idea slug → (empty)
			summary := entry.Summary
			if summary == "" {
				summary = entry.Memo
			}
			if summary == "" && entry.Idea != "" {
				summary = "💡 " + entry.Idea
			}
			if len(summary) > 50 {
				summary = summary[:50] + "…"
			}
			rows = append(rows, tuiRow{
				isHeader:  false,
				tag:       tag,
				summary:   summary,
				date:      dateStr,
				rowStatus: status,
			})
		}
	}
	return rows
}

// nonHeaderRows returns indices of all non-header rows.
func (m *sessionTUIModel) nonHeaderIndices() []int {
	var out []int
	for i, r := range m.rows {
		if !r.isHeader {
			out = append(out, i)
		}
	}
	return out
}

// Init loads sessions and builds the initial display.
func (m sessionTUIModel) Init() tea.Cmd {
	return nil
}

// Update handles keyboard input.
func (m sessionTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			indices := m.nonHeaderIndices()
			if len(indices) > 0 {
				// Find the cursor-th non-header row
				idx := m.cursorRowIndex()
				if idx >= 0 {
					m.selectedTag = m.rows[idx].tag
				}
			}
			return m, tea.Quit

		case tea.KeyEsc:
			return m, tea.Quit

		case tea.KeyUp:
			m.moveCursor(-1)

		case tea.KeyDown:
			m.moveCursor(1)

		case tea.KeyTab:
			m.cycleFilter()

		case tea.KeyBackspace:
			if len(m.query) > 0 {
				// Remove last rune
				runes := []rune(m.query)
				m.query = string(runes[:len(runes)-1])
				m.rebuildRows()
			}

		case tea.KeyRunes:
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "j":
				m.moveCursor(1)
			case "k":
				m.moveCursor(-1)
			default:
				m.query += msg.String()
				m.rebuildRows()
			}
		}
	}
	return m, nil
}

// cursorRowIndex returns the visual index in m.rows of the row at cursor position.
func (m *sessionTUIModel) cursorRowIndex() int {
	indices := m.nonHeaderIndices()
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
	return indices[c]
}

// moveCursor moves the cursor by delta, skipping headers.
func (m *sessionTUIModel) moveCursor(delta int) {
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

// cycleFilter cycles the status filter.
func (m *sessionTUIModel) cycleFilter() {
	current := m.statusFilter
	if current == "" {
		current = "all"
	}
	for i, v := range filterCycle {
		if v == current {
			m.statusFilter = filterCycle[(i+1)%len(filterCycle)]
			break
		}
	}
	m.cursor = 0
	m.rebuildRows()
}

// rebuildRows rebuilds the display rows after a filter/query change.
func (m *sessionTUIModel) rebuildRows() {
	m.rows = buildRows(m.sessions, m.searchIndex, m.query, m.statusFilter)
	// Clamp cursor
	indices := m.nonHeaderIndices()
	if m.cursor >= len(indices) {
		m.cursor = max(0, len(indices)-1)
	}
}

// --- Lipgloss styles ---

var (
	styleHeader = lipgloss.NewStyle().Bold(true)
	styleFaint  = lipgloss.NewStyle().Faint(true)
	styleReverse = lipgloss.NewStyle().Reverse(true)

	statusColors = map[string]lipgloss.Color{
		"idea":        lipgloss.Color("3"),  // yellow
		"planned":     lipgloss.Color("4"),  // blue
		"in-progress": lipgloss.Color("2"),  // green
		"active":      lipgloss.Color("7"),  // white
		"done":        lipgloss.Color("8"),  // gray
	}
)

func headerStyle(status string) lipgloss.Style {
	col, ok := statusColors[status]
	if !ok {
		col = lipgloss.Color("7")
	}
	return styleHeader.Foreground(col)
}

// View renders the TUI.
func (m sessionTUIModel) View() string {
	var sb strings.Builder

	// Top line: query + filter indicator
	filterLabel := m.statusFilter
	if filterLabel == "" || filterLabel == "all" {
		filterLabel = "all"
	}
	queryDisplay := fmt.Sprintf("🔍 %s", m.query)
	filterDisplay := fmt.Sprintf("[Tab: %s]", filterLabel)

	// Simple two-column: left query, right filter
	sb.WriteString(queryDisplay)
	// Pad to 50 chars for right-alignment (best-effort, fixed width)
	pad := 60 - len(queryDisplay) - len(filterDisplay)
	if pad < 1 {
		pad = 1
	}
	sb.WriteString(strings.Repeat(" ", pad))
	sb.WriteString(filterDisplay)
	sb.WriteString("\n\n")

	// Rows
	cursorRowIdx := m.cursorRowIndex()
	nonHeaderCount := 0

	for i, row := range m.rows {
		if row.isHeader {
			hs := headerStyle(row.status)
			sb.WriteString(hs.Render(fmt.Sprintf("── %s (%d) ──", row.status, row.count)))
			sb.WriteString("\n")
		} else {
			isSelected := i == cursorRowIdx
			// Show: cursor tag status summary date
			statusLabel := row.rowStatus
			if statusLabel == "" {
				statusLabel = "active"
			}
			line := fmt.Sprintf("  %-20s  %-12s  %-50s  %s", row.tag, statusLabel, row.summary, row.date)

			var style lipgloss.Style
			if row.rowStatus == "done" {
				style = styleFaint
			} else {
				style = lipgloss.NewStyle()
			}
			if isSelected {
				style = styleReverse
			}
			sb.WriteString(style.Render(line))
			sb.WriteString("\n")
			nonHeaderCount++
		}
	}

	if nonHeaderCount == 0 {
		sb.WriteString(styleFaint.Render("  (no sessions match)"))
		sb.WriteString("\n")
	}

	// Bottom help bar
	sb.WriteString("\n")
	sb.WriteString(styleFaint.Render("[↑↓/jk] 이동  [Enter] 시작  [/] 검색  [Tab] 필터  [q] 종료"))
	sb.WriteString("\n")

	return sb.String()
}

// runSessionsTUI runs the interactive session picker and returns the selected tag.
// Returns empty string if user quit without selection.
func runSessionsTUI() (string, error) {
	m := newSessionTUIModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return "", err
	}
	if final, ok := result.(sessionTUIModel); ok {
		return final.selectedTag, nil
	}
	return "", nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
