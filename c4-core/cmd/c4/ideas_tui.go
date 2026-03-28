package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ideaRow represents a single row in the ideas TUI.
type ideaRow struct {
	slug     string   // filename without .md
	title    string   // first heading from file, or slug
	sessions []string // matched session tags
	hasSpec  bool     // docs/specs/{slug}.md exists
	hasDesign bool   // .c4/designs/{slug}.md exists
	modTime  string   // "Jan 02 15:04"
	modUnix  int64    // for sorting
}

// ideasTUIModel is the bubbletea model for browsing ideas.
type ideasTUIModel struct {
	ideas       []ideaRow
	filtered    []ideaRow
	searchIndex map[string]string // slug → lowercased search corpus
	query       string
	cursor      int
	detailMode  bool
	detailCursor int
	selectedSlug string   // set on Enter — slug of selected idea
	selectedSessions []string // sessions linked to selected idea
	width       int
	height      int
}

func newIdeasTUIModel() ideasTUIModel {
	m := ideasTUIModel{
		searchIndex: make(map[string]string),
	}

	ideasDir := filepath.Join(projectDir, ".c4", "ideas")
	entries, err := os.ReadDir(ideasDir)
	if err != nil {
		return m
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".md")

		// Read title from first # heading
		title := slug
		content, err := os.ReadFile(filepath.Join(ideasDir, e.Name()))
		if err == nil {
			for _, line := range strings.Split(string(content), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "# ") {
					title = strings.TrimPrefix(line, "# ")
					break
				}
			}
		}

		// Get mod time
		info, _ := e.Info()
		var modTime string
		var modUnix int64
		if info != nil {
			modTime = info.ModTime().Format("Jan 02 15:04")
			modUnix = info.ModTime().Unix()
		} else {
			modTime = "--"
		}

		// Match sessions
		sessions := matchSessionsByIdea(slug)

		// Check spec/design existence
		hasSpec := false
		hasDesign := false
		specPath := filepath.Join(projectDir, "docs", "specs", slug+".md")
		if _, err := os.Stat(specPath); err == nil {
			hasSpec = true
		}
		designPath := filepath.Join(projectDir, ".c4", "designs", slug+".md")
		if _, err := os.Stat(designPath); err == nil {
			hasDesign = true
		}

		row := ideaRow{
			slug:     slug,
			title:    title,
			sessions: sessions,
			hasSpec:  hasSpec,
			hasDesign: hasDesign,
			modTime:  modTime,
			modUnix:  modUnix,
		}
		m.ideas = append(m.ideas, row)

		// Build search index
		var parts []string
		parts = append(parts, strings.ToLower(slug))
		parts = append(parts, strings.ToLower(title))
		if content != nil {
			parts = append(parts, strings.ToLower(string(content)))
		}
		m.searchIndex[slug] = strings.Join(parts, " ")
	}

	// Sort by mod time descending (newest first)
	sort.Slice(m.ideas, func(i, j int) bool {
		return m.ideas[i].modUnix > m.ideas[j].modUnix
	})

	m.filtered = m.ideas
	return m
}

func (m ideasTUIModel) Init() tea.Cmd {
	return nil
}

// rebuildFiltered returns a new filtered list based on query.
// Space-separated tokens are AND-matched (all must appear in corpus).
func rebuildIdeasFiltered(ideas []ideaRow, index map[string]string, query string) []ideaRow {
	if query == "" {
		return ideas
	}
	tokens := strings.Fields(strings.ToLower(query))
	if len(tokens) == 0 {
		return ideas
	}
	var out []ideaRow
	for _, row := range ideas {
		corpus := index[row.slug]
		match := true
		for _, tok := range tokens {
			if !strings.Contains(corpus, tok) {
				match = false
				break
			}
		}
		if match {
			out = append(out, row)
		}
	}
	return out
}

// detailPaths returns file paths for the currently selected idea.
func (m ideasTUIModel) detailPaths() []string {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	row := m.filtered[m.cursor]
	var paths []string
	if row.hasSpec {
		paths = append(paths, filepath.Join("docs", "specs", row.slug+".md"))
	}
	if row.hasDesign {
		paths = append(paths, filepath.Join(".c4", "designs", row.slug+".md"))
	}
	for _, tag := range row.sessions {
		paths = append(paths, "session:"+tag)
	}
	return paths
}

func (m ideasTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		// Detail mode
		if m.detailMode {
			paths := m.detailPaths()
			switch msg.Type {
			case tea.KeyLeft, tea.KeyEsc:
				m.detailMode = false
				m.detailCursor = 0
			case tea.KeyUp:
				if m.detailCursor > 0 {
					m.detailCursor--
				}
			case tea.KeyDown:
				if m.detailCursor < len(paths)-1 {
					m.detailCursor++
				}
			case tea.KeyEnter:
				if m.detailCursor < len(paths) {
					p := paths[m.detailCursor]
					if strings.HasPrefix(p, "session:") {
						tag := strings.TrimPrefix(p, "session:")
						m.selectedSlug = m.filtered[m.cursor].slug
						m.selectedSessions = []string{tag}
						return m, tea.Quit
					}
					return m, openFileCmd(p)
				}
			case tea.KeyRunes:
				switch msg.String() {
				case "k":
					if m.detailCursor > 0 {
						m.detailCursor--
					}
				case "j":
					if m.detailCursor < len(paths)-1 {
						m.detailCursor++
					}
				case "q", "h":
					m.detailMode = false
					m.detailCursor = 0
				}
			}
			return m, nil
		}

		// Normal mode
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit

		case tea.KeyEnter:
			if m.cursor >= 0 && m.cursor < len(m.filtered) {
				row := m.filtered[m.cursor]
				m.selectedSlug = row.slug
				m.selectedSessions = row.sessions
				if len(row.sessions) == 1 {
					// Single session → launch directly
					return m, tea.Quit
				} else if len(row.sessions) > 1 {
					// Multiple sessions → enter detail mode to pick
					m.detailMode = true
					m.detailCursor = 0
					return m, nil
				}
				// No sessions → will create new
				return m, tea.Quit
			}

		case tea.KeyEsc:
			if m.query != "" {
				m.query = ""
				m.filtered = m.ideas
				m.cursor = 0
			} else {
				return m, tea.Quit
			}

		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}

		case tea.KeyDown:
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}

		case tea.KeyRight:
			paths := m.detailPaths()
			if len(paths) > 0 {
				m.detailMode = true
				m.detailCursor = 0
			}

		case tea.KeySpace:
			m.query += " "
			m.filtered = rebuildIdeasFiltered(m.ideas, m.searchIndex, m.query)
			if m.cursor >= len(m.filtered) {
				if len(m.filtered) > 0 {
					m.cursor = len(m.filtered) - 1
				} else {
					m.cursor = 0
				}
			}

		case tea.KeyBackspace:
			if len(m.query) > 0 {
				runes := []rune(m.query)
				m.query = string(runes[:len(runes)-1])
				m.filtered = rebuildIdeasFiltered(m.ideas, m.searchIndex, m.query)
			if m.cursor >= len(m.filtered) {
				if len(m.filtered) > 0 {
					m.cursor = len(m.filtered) - 1
				} else {
					m.cursor = 0
				}
			}
			}

		case tea.KeyRunes:
			ch := msg.String()
			if ch == "q" && m.query == "" {
				return m, tea.Quit
			}
			if ch == "j" && m.query == "" {
				if m.cursor < len(m.filtered)-1 {
					m.cursor++
				}
				return m, nil
			}
			if ch == "k" && m.query == "" {
				if m.cursor > 0 {
					m.cursor--
				}
				return m, nil
			}
			m.query += ch
			m.filtered = rebuildIdeasFiltered(m.ideas, m.searchIndex, m.query)
			if m.cursor >= len(m.filtered) {
				if len(m.filtered) > 0 {
					m.cursor = len(m.filtered) - 1
				} else {
					m.cursor = 0
				}
			}
		}
	}
	return m, nil
}

func (m ideasTUIModel) View() string {
	var sb strings.Builder

	// Title bar
	sb.WriteString(styleTitle.Render(" cq ideas "))
	sb.WriteString(" ")
	if m.query != "" {
		sb.WriteString(styleCount.Render(fmt.Sprintf("%d / %d ideas", len(m.filtered), len(m.ideas))))
	} else {
		sb.WriteString(styleCount.Render(fmt.Sprintf("%d ideas", len(m.ideas))))
	}
	sb.WriteString("\n\n")

	// Search bar
	if m.query != "" {
		sb.WriteString("  ")
		sb.WriteString(styleSearchBar.Render(fmt.Sprintf(" 🔍 %s▏ ", m.query)))
	} else {
		sb.WriteString("  ")
		sb.WriteString(styleSearchPlaceholder.Render(" 🔍 type to search... "))
	}
	sb.WriteString("\n\n")

	// Rows
	// Fixed: cursor(3) + slug(28) + sessions_count(8) + markers(4) + summary(dynamic) + date(12)
	const slugColW = 28
	fixedW := 3 + slugColW + 8 + 4 + 1 + 12 + 1
	titleColW := m.width - fixedW
	if titleColW < 10 {
		titleColW = 10
	}

	// Visible window: keep cursor in view
	// Reserve lines: title(2) + search(2) + helpbar(2) + margins(2) = 8
	maxVisible := m.height - 8
	if maxVisible < 5 {
		maxVisible = 5
	}
	viewStart := 0
	viewEnd := len(m.filtered)
	if len(m.filtered) > maxVisible {
		// Center cursor in viewport
		viewStart = m.cursor - maxVisible/2
		if viewStart < 0 {
			viewStart = 0
		}
		viewEnd = viewStart + maxVisible
		if viewEnd > len(m.filtered) {
			viewEnd = len(m.filtered)
			viewStart = viewEnd - maxVisible
		}
	}

	if viewStart > 0 {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("  ▲ %d more", viewStart)))
		sb.WriteString("\n")
	}

	for i := viewStart; i < viewEnd; i++ {
		row := m.filtered[i]
		isSelected := i == m.cursor

		cursor := "   "
		if isSelected {
			cursor = " ▸ "
		}

		// Slug: truncate + pad
		slugDisplay := row.slug
		if lsDispWidth(slugDisplay) > slugColW {
			slugDisplay = lsTruncateToWidth(slugDisplay, slugColW-1) + "…"
		}
		slugPadded := lsPadToWidth(slugDisplay, slugColW)

		// Session count
		sessCount := fmt.Sprintf(" %2d ⚡ ", len(row.sessions))

		// Doc markers: ●·· for spec/design
		var markers string
		if row.hasSpec || row.hasDesign {
			m1, m2 := "·", "·"
			if row.hasSpec {
				m1 = "●"
			}
			if row.hasDesign {
				m2 = "●"
			}
			markers = m1 + m2 + "  "
		} else {
			markers = "    "
		}

		// Title: truncate
		titleDisplay := row.title
		if lsDispWidth(titleDisplay) > titleColW {
			titleDisplay = lsTruncateToWidth(titleDisplay, titleColW-1) + "…"
		}

		dateStr := row.modTime

		// Calculate padding between title and date
		leftUsed := 3 + slugColW + 8 + 4 + lsDispWidth(titleDisplay)
		dateW := len(dateStr) + 1
		midPad := m.width - leftUsed - dateW
		if midPad < 1 {
			midPad = 1
		}

		markerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Faint(true)
		markerStyleSel := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Background(lipgloss.Color("236"))
		sessStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Faint(true)
		sessStyleSel := lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Background(lipgloss.Color("236"))

		if isSelected {
			sb.WriteString(styleSelected.Render(cursor))
			sb.WriteString(styleSelected.Render(slugPadded))
			sb.WriteString(sessStyleSel.Render(sessCount))
			sb.WriteString(markerStyleSel.Render(markers))
			sb.WriteString(styleSelected.Render(titleDisplay))
			sb.WriteString(styleSelected.Render(strings.Repeat(" ", midPad)))
			sb.WriteString(styleSelected.Render(dateStr))
		} else {
			sb.WriteString(cursor)
			sb.WriteString(styleTagName.Render(slugPadded))
			sb.WriteString(sessStyle.Render(sessCount))
			sb.WriteString(markerStyle.Render(markers))
			sb.WriteString(styleSummary.Render(titleDisplay))
			sb.WriteString(strings.Repeat(" ", midPad))
			sb.WriteString(styleDate.Render(dateStr))
		}
		sb.WriteString("\n")

		// Detail paths for selected row
		if isSelected {
			paths := m.detailPaths()
			for pi, p := range paths {
				isLast := pi == len(paths)-1
				branch := "├─"
				if isLast {
					branch = "└─"
				}
				icon := "📄"
				label := p
				if strings.HasPrefix(p, "session:") {
					icon = "🔗"
					label = strings.TrimPrefix(p, "session:")
				} else if strings.Contains(p, "designs") {
					icon = "🏗 "
				}
				line := fmt.Sprintf("      %s %s %s", branch, icon, label)
				if m.detailMode && pi == m.detailCursor {
					sb.WriteString(styleFileSelected.Render(line))
				} else {
					sb.WriteString(styleFilePath.Render(line))
				}
				sb.WriteString("\n")
			}
		}
	}

	if viewEnd < len(m.filtered) {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("  ▼ %d more", len(m.filtered)-viewEnd)))
		sb.WriteString("\n")
	}

	if len(m.filtered) == 0 {
		sb.WriteString("\n")
		sb.WriteString(styleFaint.Render("  No ideas match your search."))
		sb.WriteString("\n")
	}

	// Build help bar
	var helpBar strings.Builder
	if m.detailMode {
		helpBar.WriteString(" ")
		helpBar.WriteString(helpEntry("↑↓", "move"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("Enter", "open"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("←", "back"))
	} else {
		helpBar.WriteString(" ")
		helpBar.WriteString(helpEntry("↑↓", "move"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("Enter", "start"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("→", "details"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("Esc", "quit/clear"))
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
	sb.WriteString(helpBar.String())

	return sb.String()
}

func runIdeasTUI() error {
	m := newIdeasTUIModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return err
	}

	im := result.(ideasTUIModel)
	if im.selectedSlug == "" {
		return nil // quit without selection
	}

	// Determine which tool to use
	tool := readGlobalConfig("default_tool")
	if tool == "" {
		tool = "claude"
	}

	if len(im.selectedSessions) == 1 {
		// Launch existing session
		fmt.Fprintf(os.Stderr, "cq: opening session '%s' for idea '%s'...\n", im.selectedSessions[0], im.selectedSlug)
		return launchToolNamed(tool, projectDir, im.selectedSessions[0])
	}

	// No session → create new session named after idea slug
	fmt.Fprintf(os.Stderr, "cq: creating new session '%s'...\n", im.selectedSlug)
	return launchToolNamed(tool, projectDir, im.selectedSlug)
}
