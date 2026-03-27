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

// filterCycle is the order of Tab-cycling for status filters.
var filterCycle = []string{"all", "in-progress", "planned", "idea", "active", "done"}

// tuiRow represents a single row in the TUI list.
type tuiRow struct {
	isHeader bool
	// header fields
	status string
	count  int
	// session row fields
	tag       string
	summary   string
	date      string
	rowStatus string
	// file paths for detail display
	ideaPath   string
	specPath   string
	designPath string
}

// sessionTUIModel is the bubbletea model for the session picker.
type sessionTUIModel struct {
	sessions    map[string]namedSessionEntry
	searchIndex map[string]string // tag → lowercased search text

	rows []tuiRow

	query        string
	statusFilter string // "all" means no filter
	cursor       int

	selectedTag  string
	confirmDelete bool   // waiting for delete confirmation
	deleteTarget  string // tag to delete
}

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
			strings.ToLower(entry.Memo),
		}

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

// resolveFilePaths finds idea/spec/design file paths for a session entry.
func resolveFilePaths(tag string, entry namedSessionEntry) (ideaPath, specPath, designPath string) {
	ideaSlug := entry.Idea
	if ideaSlug == "" {
		ideaSlug, _ = matchIdeaByTag(tag)
	}
	if ideaSlug != "" {
		p := filepath.Join(".c4", "ideas", ideaSlug+".md")
		if _, err := os.Stat(filepath.Join(projectDir, p)); err == nil {
			ideaPath = p
		}
		p = filepath.Join("docs", "specs", ideaSlug+".md")
		if _, err := os.Stat(filepath.Join(projectDir, p)); err == nil {
			specPath = p
		}
		p = filepath.Join(".c4", "designs", ideaSlug+".md")
		if _, err := os.Stat(filepath.Join(projectDir, p)); err == nil {
			designPath = p
		}
	}
	return
}

func buildRows(sessions map[string]namedSessionEntry, idx map[string]string, query, statusFilter string) []tuiRow {
	lowerQuery := strings.ToLower(query)

	byStatus := make(map[string][]string)
	for tag, entry := range sessions {
		status := entry.Status
		if status == "" {
			status = "active"
		}
		if statusFilter != "" && statusFilter != "all" && status != statusFilter {
			continue
		}
		if lowerQuery != "" {
			corpus := idx[tag]
			if !strings.Contains(corpus, lowerQuery) {
				continue
			}
		}
		byStatus[status] = append(byStatus[status], tag)
	}

	for status := range byStatus {
		sort.Strings(byStatus[status])
	}

	var rows []tuiRow
	for _, status := range statusOrder {
		tags, ok := byStatus[status]
		if !ok || len(tags) == 0 {
			continue
		}
		rows = append(rows, tuiRow{
			isHeader: true,
			status:   status,
			count:    len(tags),
		})
		for _, tag := range tags {
			entry := sessions[tag]
			dateStr := "--"
			if t, err := time.Parse(time.RFC3339, entry.Updated); err == nil {
				dateStr = t.Format("Jan 02 15:04")
			}
			summary := entry.Summary
			if summary == "" {
				summary = entry.Memo
			}
			if summary == "" && entry.Idea != "" {
				summary = entry.Idea
			}
			if len(summary) > 45 {
				summary = summary[:45] + "…"
			}
			ideaPath, specPath, designPath := resolveFilePaths(tag, entry)
			rows = append(rows, tuiRow{
				tag:        tag,
				summary:    summary,
				date:       dateStr,
				rowStatus:  status,
				ideaPath:   ideaPath,
				specPath:   specPath,
				designPath: designPath,
			})
		}
	}
	return rows
}

func (m *sessionTUIModel) nonHeaderIndices() []int {
	var out []int
	for i, r := range m.rows {
		if !r.isHeader {
			out = append(out, i)
		}
	}
	return out
}

func (m sessionTUIModel) Init() tea.Cmd {
	return nil
}

// isSearching returns true when search query is active — keys go to query instead of shortcuts.
func (m *sessionTUIModel) isSearching() bool {
	return m.query != ""
}

func (m sessionTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Delete confirmation mode
		if m.confirmDelete {
			switch msg.String() {
			case "y", "Y":
				delete(m.sessions, m.deleteTarget)
				_ = saveNamedSessions(m.sessions)
				delete(m.searchIndex, m.deleteTarget)
				m.confirmDelete = false
				m.deleteTarget = ""
				m.rebuildRows()
			default:
				m.confirmDelete = false
				m.deleteTarget = ""
			}
			return m, nil
		}

		switch msg.Type {
		case tea.KeyEnter:
			idx := m.cursorRowIndex()
			if idx >= 0 {
				m.selectedTag = m.rows[idx].tag
			}
			return m, tea.Quit

		case tea.KeyEsc:
			if m.isSearching() {
				m.query = ""
				m.rebuildRows()
				return m, nil
			}
			return m, tea.Quit

		case tea.KeyUp:
			m.moveCursor(-1)

		case tea.KeyDown:
			m.moveCursor(1)

		case tea.KeyTab:
			m.cycleFilter()

		case tea.KeyBackspace:
			if len(m.query) > 0 {
				runes := []rune(m.query)
				m.query = string(runes[:len(runes)-1])
				m.rebuildRows()
			}

		case tea.KeyRunes:
			ch := msg.String()
			if m.isSearching() {
				// In search mode: all chars go to query
				m.query += ch
				m.rebuildRows()
			} else {
				// Navigation mode: shortcuts active
				switch ch {
				case "q":
					return m, tea.Quit
				case "j":
					m.moveCursor(1)
				case "k":
					m.moveCursor(-1)
				case "d":
					idx := m.cursorRowIndex()
					if idx >= 0 {
						m.confirmDelete = true
						m.deleteTarget = m.rows[idx].tag
					}
				case "/":
					// Enter search mode (query starts empty, next char will make isSearching true)
					// Do nothing — user will type next char which enters search
				default:
					// Start searching
					m.query += ch
					m.rebuildRows()
				}
			}
		}
	}
	return m, nil
}

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

func (m *sessionTUIModel) rebuildRows() {
	m.rows = buildRows(m.sessions, m.searchIndex, m.query, m.statusFilter)
	indices := m.nonHeaderIndices()
	if m.cursor >= len(indices) {
		if len(indices) > 0 {
			m.cursor = len(indices) - 1
		} else {
			m.cursor = 0
		}
	}
}

// --- Lipgloss styles ---

var (
	styleHeader    = lipgloss.NewStyle().Bold(true)
	styleFaint     = lipgloss.NewStyle().Faint(true)
	styleSelected  = lipgloss.NewStyle().Bold(true).Reverse(true)
	styleFilePath  = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("6")) // cyan, cmd+click friendly
	styleStatusTag = map[string]lipgloss.Style{
		"idea":        lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true),  // yellow
		"planned":     lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true),  // blue
		"in-progress": lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true),  // green
		"active":      lipgloss.NewStyle().Foreground(lipgloss.Color("7")),             // white
		"done":        lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true), // gray
	}
	styleSearchBar = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	styleHelpBar   = lipgloss.NewStyle().Faint(true)
	styleConfirm   = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true) // red
)

func headerStyle(status string) lipgloss.Style {
	col := lipgloss.Color("7")
	switch status {
	case "idea":
		col = lipgloss.Color("3")
	case "planned":
		col = lipgloss.Color("4")
	case "in-progress":
		col = lipgloss.Color("2")
	case "done":
		col = lipgloss.Color("8")
	}
	return styleHeader.Foreground(col)
}

func (m sessionTUIModel) View() string {
	var sb strings.Builder

	// Delete confirmation
	if m.confirmDelete {
		sb.WriteString(styleConfirm.Render(fmt.Sprintf("  Delete session '%s'? [y/N] ", m.deleteTarget)))
		sb.WriteString("\n")
		return sb.String()
	}

	// Search bar
	filterLabel := m.statusFilter
	if filterLabel == "" || filterLabel == "all" {
		filterLabel = "all"
	}
	if m.isSearching() {
		sb.WriteString(styleSearchBar.Render(fmt.Sprintf("🔍 %s▏", m.query)))
	} else {
		sb.WriteString(styleFaint.Render("🔍 type to search..."))
	}
	filterText := styleFaint.Render(fmt.Sprintf("[Tab: %s]", filterLabel))
	// Right-align filter
	pad := 60 - lipgloss.Width(sb.String()) - lipgloss.Width(filterText)
	if pad < 1 {
		pad = 1
	}
	sb.WriteString(strings.Repeat(" ", pad))
	sb.WriteString(filterText)
	sb.WriteString("\n\n")

	// Rows
	cursorRowIdx := m.cursorRowIndex()
	nonHeaderCount := 0

	for i, row := range m.rows {
		if row.isHeader {
			hs := headerStyle(row.status)
			sb.WriteString(hs.Render(fmt.Sprintf(" ── %s (%d) ──", row.status, row.count)))
			sb.WriteString("\n")
			continue
		}

		isSelected := i == cursorRowIdx
		nonHeaderCount++

		// Status badge with color
		stStyle, ok := styleStatusTag[row.rowStatus]
		if !ok {
			stStyle = lipgloss.NewStyle()
		}
		statusBadge := stStyle.Render(fmt.Sprintf("%-12s", row.rowStatus))

		// Build the line
		cursor := "  "
		if isSelected {
			cursor = "▸ "
		}

		tagStr := fmt.Sprintf("%-18s", row.tag)
		summaryStr := fmt.Sprintf("%-45s", row.summary)
		dateStr := row.date

		if isSelected {
			line := fmt.Sprintf("%s%s  %s  %s  %s", cursor, tagStr, statusBadge, summaryStr, dateStr)
			sb.WriteString(styleSelected.Render(line))
		} else if row.rowStatus == "done" {
			line := fmt.Sprintf("%s%s  %s  %s  %s", cursor, tagStr, statusBadge, summaryStr, dateStr)
			sb.WriteString(styleFaint.Render(line))
		} else {
			sb.WriteString(cursor)
			sb.WriteString(lipgloss.NewStyle().Bold(true).Render(tagStr))
			sb.WriteString("  ")
			sb.WriteString(statusBadge)
			sb.WriteString("  ")
			sb.WriteString(summaryStr)
			sb.WriteString("  ")
			sb.WriteString(styleFaint.Render(dateStr))
		}
		sb.WriteString("\n")

		// Show file paths for selected row
		if isSelected {
			if row.ideaPath != "" {
				sb.WriteString(styleFilePath.Render(fmt.Sprintf("    ├─ 💡 %s", row.ideaPath)))
				sb.WriteString("\n")
			}
			if row.specPath != "" {
				sb.WriteString(styleFilePath.Render(fmt.Sprintf("    ├─ 📄 %s", row.specPath)))
				sb.WriteString("\n")
			}
			if row.designPath != "" {
				last := "└─"
				sb.WriteString(styleFilePath.Render(fmt.Sprintf("    %s 🏗  %s", last, row.designPath)))
				sb.WriteString("\n")
			}
		}
	}

	if nonHeaderCount == 0 {
		sb.WriteString(styleFaint.Render("  (no sessions match)"))
		sb.WriteString("\n")
	}

	// Help bar
	sb.WriteString("\n")
	help := "[↑↓/jk] 이동  [Enter] 시작  [d] 삭제  [Tab] 필터  [Esc] 검색취소  [q] 종료"
	sb.WriteString(styleHelpBar.Render(help))
	sb.WriteString("\n")

	return sb.String()
}

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
