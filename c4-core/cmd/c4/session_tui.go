package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
)

// statusOrder defines the display order for session groups.
var statusOrder = []string{"running", "active", "planned", "idea", "done"}

// filterCycle is the order of Tab-cycling for status filters.
var filterCycle = []string{"all", "running", "active", "planned", "idea", "done"}

// tuiRow represents a single row in the TUI list.
type tuiRow struct {
	isHeader bool
	// header fields
	status string
	count  int
	// session row fields
	tag       string
	tool      string // "claude", "chatgpt", "gemini", "codex"
	summary   string
	date      string
	rowStatus string
	// file paths for detail display
	ideaPaths  []string
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

	selectedTag   string
	confirmDelete bool   // waiting for delete confirmation
	deleteTarget  string // tag to delete

	// Detail mode: navigate file paths of selected session
	detailMode   bool
	detailCursor int

	// New session mode
	newMode  bool
	newInput string

	// Terminal size
	width  int
	height int

	// Navigation: set by handleGlobalKey to switch TUI screens.
	nextScreen string

	// History mode: user questions from JSONL
	historyMode   bool
	historyItems  []string // user questions (first line each)
	historyCursor int
	historyScroll int

	// External AI tool processes (not managed by CQ)
	externalTools []aiToolProcess

	// Remote sessions from Supabase
	remoteFetched   bool
	remoteTickCount int // counts ticks since last remote fetch
	remoteSessions  []remoteSession
}

// detailPaths returns the file paths for the currently selected session.
func (m *sessionTUIModel) detailPaths() []string {
	idx := m.cursorRowIndex()
	if idx < 0 {
		return nil
	}
	row := m.rows[idx]
	var paths []string
	paths = append(paths, row.ideaPaths...)
	if row.specPath != "" {
		paths = append(paths, row.specPath)
	}
	if row.designPath != "" {
		paths = append(paths, row.designPath)
	}
	return paths
}

func newSessionTUIModel() sessionTUIModel {
	m := sessionTUIModel{
		statusFilter: "all",
	}
	m.sessions, _ = loadNamedSessions()
	recalcStatuses(m.sessions)
	// Merge local sessions from all providers (Gemini, Codex)
	for _, p := range providers {
		if p.ScanSessions == nil {
			continue
		}
		for tag, entry := range p.ScanSessions() {
			if _, exists := m.sessions[tag]; !exists {
				m.sessions[tag] = entry
			}
		}
	}
	m.searchIndex = buildSearchIndex(m.sessions)
	m.rows = buildRows(m.sessions, m.searchIndex, m.query, m.statusFilter)
	m.externalTools = detectRunningAITools()
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

		var ideaSlugs []string
		if entry.Idea != "" {
			ideaSlugs = []string{entry.Idea}
		} else {
			ideaSlugs = matchIdeasByTag(tag)
		}
		for _, slug := range ideaSlugs {
			parts = append(parts, strings.ToLower(slug))
			ideaContent, err := os.ReadFile(filepath.Join(ideasDir, slug+".md"))
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
func resolveFilePaths(tag string, entry namedSessionEntry) (ideaPaths []string, specPath, designPath string) {
	var slugs []string
	if entry.Idea != "" {
		slugs = []string{entry.Idea}
	} else {
		slugs = matchIdeasByTag(tag)
	}
	for _, slug := range slugs {
		p := filepath.Join(".c4", "ideas", slug+".md")
		if _, err := os.Stat(filepath.Join(projectDir, p)); err == nil {
			ideaPaths = append(ideaPaths, p)
		}
	}
	// Use the first matched slug for spec/design lookup
	primarySlug := entry.Idea
	if primarySlug == "" && len(slugs) > 0 {
		primarySlug = slugs[0]
	}
	if primarySlug != "" {
		p := filepath.Join("docs", "specs", primarySlug+".md")
		if _, err := os.Stat(filepath.Join(projectDir, p)); err == nil {
			specPath = p
		}
		p = filepath.Join(".c4", "designs", primarySlug+".md")
		if _, err := os.Stat(filepath.Join(projectDir, p)); err == nil {
			designPath = p
		}
	}
	return
}

// sanitizeSessionSummary strips newlines and markdown noise from summary text.
func sanitizeSessionSummary(s string) string {
	if s == "" {
		return ""
	}
	// Take first line only
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	// Strip leading markdown headers
	for strings.HasPrefix(s, "#") {
		s = strings.TrimLeft(s, "# ")
		s = strings.TrimSpace(s)
	}
	return s
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

	// Sort each group by Updated descending (newest first)
	for status := range byStatus {
		sort.Slice(byStatus[status], func(i, j int) bool {
			return sessions[byStatus[status][i]].Updated > sessions[byStatus[status][j]].Updated
		})
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
				dateStr = t.Format("2006 Jan 02")
			}
			summary := sanitizeSessionSummary(entry.Summary)
			if summary == "" {
				summary = sanitizeSessionSummary(entry.Memo)
			}
			if summary == "" && entry.Idea != "" {
				summary = entry.Idea
			}
			// Don't truncate here — View truncates dynamically based on terminal width
			ideaPaths, specPath, designPath := resolveFilePaths(tag, entry)
			tool := inferSessionTool(tag, entry)
			rows = append(rows, tuiRow{
				tag:        tag,
				tool:       tool,
				summary:    summary,
				date:       dateStr,
				rowStatus:  status,
				ideaPaths:  ideaPaths,
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

// sessionTickMsg triggers periodic refresh of session data.
type sessionTickMsg struct{}

func sessionTickCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return sessionTickMsg{}
	})
}

// remoteSessionsMsg carries fetched remote sessions from Supabase.
type remoteSessionsMsg struct {
	sessions []remoteSession
}

type remoteSession struct {
	ID        string  `json:"id"`
	Tool      string  `json:"tool"`
	Title     *string `json:"title"`
	Summary   *string `json:"summary"`
	Status    string  `json:"status"`
	CreatedAt string  `json:"created_at"`
}

// fetchRemoteSessions queries Supabase ai_sessions table.
func fetchRemoteSessions() tea.Msg {
	session, err := readNotifySession()
	if err != nil {
		return remoteSessionsMsg{}
	}
	sbURL := readCloudURL(projectDir)
	if sbURL == "" {
		return remoteSessionsMsg{}
	}
	anonKey := readCloudAnonKey(projectDir)
	reqURL := sbURL + "/rest/v1/ai_sessions?select=id,tool,title,summary,status,created_at&order=created_at.desc&limit=50"
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return remoteSessionsMsg{}
	}
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	req.Header.Set("apikey", anonKey)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return remoteSessionsMsg{}
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != 200 {
		return remoteSessionsMsg{}
	}
	var remoteSessions []remoteSession
	if err := json.Unmarshal(respBody, &remoteSessions); err != nil {
		return remoteSessionsMsg{}
	}
	return remoteSessionsMsg{sessions: remoteSessions}
}

// mergeRemoteSessions adds remote sessions to the local map with a tool prefix tag.
func mergeRemoteSessions(local map[string]namedSessionEntry, remote []remoteSession) {
	for _, rs := range remote {
		if rs.Tool == "claude" {
			continue
		}
		tag := fmt.Sprintf("%s/%s", rs.Tool, rs.ID[:8])
		if _, exists := local[tag]; exists {
			continue
		}
		summary := ""
		if rs.Summary != nil {
			summary = *rs.Summary
		}
		if summary == "" && rs.Title != nil {
			summary = *rs.Title
		}
		local[tag] = namedSessionEntry{
			UUID:    rs.ID,
			Tool:    rs.Tool,
			Status:  rs.Status,
			Summary: summary,
			Updated: rs.CreatedAt,
		}
	}
}

func (m sessionTUIModel) Init() tea.Cmd {
	return tea.Batch(sessionTickCmd(), fetchRemoteSessions)
}

func (m *sessionTUIModel) isSearching() bool {
	return m.query != ""
}

// loadHistory reads user questions from the session transcript using the provider system.
// Tries each provider's FindTranscript with the session's dir, then projectDir.
func loadHistory(uuid string, entry namedSessionEntry) []string {
	tool := entry.Tool
	if tool == "" {
		tool = "claude"
	}

	// Try the matching provider first
	if p := findProviderByTool(tool); p != nil {
		dirs := []string{entry.Dir, projectDir}
		for _, d := range dirs {
			if d == "" {
				continue
			}
			path := p.FindTranscript(d, uuid)
			if path != "" {
				return p.LoadHistory(path)
			}
		}
	}

	// Fallback: try all providers
	for _, p := range providers {
		if p.Tool == tool {
			continue
		}
		dirs := []string{entry.Dir, projectDir}
		for _, d := range dirs {
			if d == "" {
				continue
			}
			path := p.FindTranscript(d, uuid)
			if path != "" {
				return p.LoadHistory(path)
			}
		}
	}
	return nil
}

// extractUserText extracts plain text from a JSONL message content field.
func extractUserText(content any) string {
	switch v := content.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		var parts []string
		for _, block := range v {
			if m, ok := block.(map[string]any); ok {
				if t, ok := m["text"].(string); ok && t != "" {
					parts = append(parts, strings.TrimSpace(t))
				}
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

// wrapText splits text into lines respecting display width and newlines.
func wrapText(text string, maxW int) []string {
	var result []string
	for _, paragraph := range strings.Split(text, "\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		// Simple word-wrap
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if lsDispWidth(line)+1+lsDispWidth(w) > maxW {
				result = append(result, line)
				line = w
			} else {
				line += " " + w
			}
		}
		if line != "" {
			result = append(result, line)
		}
	}
	if len(result) == 0 {
		result = []string{text}
	}
	return result
}

// openFileCmd opens a file path using the OS default handler.
func openFileCmd(path string) tea.Cmd {
	return func() tea.Msg {
		absPath := filepath.Join(projectDir, path)
		// Try 'code' first (VS Code), then 'open' (macOS default)
		for _, cmd := range []string{"code", "open"} {
			if p, err := exec.LookPath(cmd); err == nil {
				_ = exec.Command(p, absPath).Start()
				return nil
			}
		}
		return nil
	}
}

func (m sessionTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case remoteSessionsMsg:
		m.remoteSessions = msg.sessions
		if len(msg.sessions) > 0 {
			mergeRemoteSessions(m.sessions, msg.sessions)
			m.searchIndex = buildSearchIndex(m.sessions)
			m.rebuildRows()
			m.remoteFetched = true
		}
		return m, nil
	case sessionTickMsg:
		sessions, err := loadNamedSessions()
		if err == nil {
			recalcStatuses(sessions)
			// Re-merge cached remote sessions
			if m.remoteFetched && len(m.remoteSessions) > 0 {
				mergeRemoteSessions(sessions, m.remoteSessions)
			}
			// Re-use cached provider sessions on every tick, but only re-scan every ~30s
			for tag, entry := range m.sessions {
				if entry.Tool != "" && entry.Tool != "claude" {
					if _, exists := sessions[tag]; !exists {
						sessions[tag] = entry
					}
				}
			}
			m.sessions = sessions
			m.searchIndex = buildSearchIndex(sessions)
			m.rebuildRows()
		}
		m.externalTools = detectRunningAITools()
		// Re-fetch remote sessions every ~30s (10 ticks)
		m.remoteTickCount++
		var cmds []tea.Cmd
		cmds = append(cmds, sessionTickCmd())
		if m.remoteTickCount >= 10 {
			m.remoteTickCount = 0
			cmds = append(cmds, fetchRemoteSessions)
		}
		return m, tea.Batch(cmds...)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		// History mode: scrollable list of user questions
		if m.historyMode {
			switch msg.Type {
			case tea.KeyEsc, tea.KeyLeft, tea.KeySpace:
				m.historyMode = false
				return m, nil
			case tea.KeyUp:
				if m.historyCursor > 0 {
					m.historyCursor--
				}
			case tea.KeyDown:
				if m.historyCursor < len(m.historyItems)-1 {
					m.historyCursor++
				}
			}
			return m, nil
		}

		// New session input mode
		if m.newMode {
			switch msg.Type {
			case tea.KeyEnter:
				name := strings.TrimSpace(m.newInput)
				if name != "" {
					m.selectedTag = name
					m.newMode = false
					return m, tea.Quit
				}
			case tea.KeyEsc:
				m.newMode = false
				m.newInput = ""
			case tea.KeyBackspace:
				if len(m.newInput) > 0 {
					runes := []rune(m.newInput)
					m.newInput = string(runes[:len(runes)-1])
				}
			case tea.KeyRunes:
				m.newInput += msg.String()
			}
			return m, nil
		}

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

		// Detail mode: navigating file paths
		if m.detailMode {
			paths := m.detailPaths()
			switch msg.Type {
			case tea.KeyLeft, tea.KeyEsc:
				m.detailMode = false
				m.detailCursor = 0
				return m, nil
			case tea.KeyUp:
				if m.detailCursor > 0 {
					m.detailCursor--
				}
				return m, nil
			case tea.KeyDown:
				if m.detailCursor < len(paths)-1 {
					m.detailCursor++
				}
				return m, nil
			case tea.KeyEnter:
				if m.detailCursor < len(paths) {
					return m, openFileCmd(paths[m.detailCursor])
				}
				return m, nil
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
				return m, nil
			}
			return m, nil
		}

		// Normal mode
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
			m.nextScreen = screenDashboard
			return m, tea.Quit

		case tea.KeyUp:
			m.moveCursor(-1)

		case tea.KeyDown:
			m.moveCursor(1)

		case tea.KeyRight:
			paths := m.detailPaths()
			if len(paths) > 0 {
				m.detailMode = true
				m.detailCursor = 0
			}

		case tea.KeySpace:
			if m.isSearching() {
				m.query += " "
				m.rebuildRows()
			} else {
				idx := m.cursorRowIndex()
				if idx >= 0 {
					tag := m.rows[idx].tag
					entry := m.sessions[tag]
					items := loadHistory(entry.UUID, entry)
					if len(items) > 0 {
						m.historyMode = true
						m.historyItems = items
						m.historyCursor = len(items) - 1
						m.historyScroll = 0
					}
				}
			}

		case tea.KeyTab:
			m.cycleFilter()

		case tea.KeyCtrlN:
			m.newMode = true
			m.newInput = ""

		case tea.KeyCtrlD:
			idx := m.cursorRowIndex()
			if idx >= 0 {
				m.confirmDelete = true
				m.deleteTarget = m.rows[idx].tag
			}

		case tea.KeyCtrlS:
			idx := m.cursorRowIndex()
			if idx >= 0 {
				tag := m.rows[idx].tag
				entry := m.sessions[tag]
				status := entry.Status
				if status == "" {
					status = "active"
				}
				switch status {
				case "active", "":
					entry.Status = "done"
				case "done":
					entry.Status = "active"
				}
				m.sessions[tag] = entry
				_ = saveNamedSessions(m.sessions)
				m.rebuildRows()
			}

		case tea.KeyBackspace:
			if len(m.query) > 0 {
				runes := []rune(m.query)
				m.query = string(runes[:len(runes)-1])
				m.rebuildRows()
			}

		case tea.KeyRunes:
			ch := msg.String()
			// All printable characters go to search query.
			m.query += ch
			m.rebuildRows()
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
	styleFaint    = lipgloss.NewStyle().Faint(true)
	styleSelected = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("15")).
			Bold(true)
	styleFilePath = lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")).
			Faint(true)
	styleFileSelected = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("14")).
				Bold(true)
	styleSearchBar = lipgloss.NewStyle().
			Foreground(lipgloss.Color("14")).
			Bold(true)
	styleSearchPlaceholder = lipgloss.NewStyle().
				Faint(true).
				Italic(true)
	styleHelpKey = lipgloss.NewStyle().
			Foreground(lipgloss.Color("14")).
			Bold(true)
	styleHelpDesc = lipgloss.NewStyle().
			Faint(true)
	styleConfirm = lipgloss.NewStyle().
			Foreground(lipgloss.Color("1")).
			Bold(true)
	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)
	styleCount = lipgloss.NewStyle().
			Faint(true)
	styleTagName = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15"))
	styleTagNameDim = lipgloss.NewStyle().
			Faint(true)
	styleSummary = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
	styleSummaryDim = lipgloss.NewStyle().
			Faint(true)
	styleDate = lipgloss.NewStyle().
			Faint(true).
			Foreground(lipgloss.Color("244"))

	// Status badge styles: colored background pill
	statusBadgeStyles = map[string]lipgloss.Style{
		"running": lipgloss.NewStyle().Background(lipgloss.Color("2")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1),
		"idea":    lipgloss.NewStyle().Background(lipgloss.Color("3")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1),
		"planned": lipgloss.NewStyle().Background(lipgloss.Color("4")).Foreground(lipgloss.Color("15")).Bold(true).Padding(0, 1),
		"active":  lipgloss.NewStyle().Background(lipgloss.Color("6")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1),
		"done":    lipgloss.NewStyle().Background(lipgloss.Color("237")).Foreground(lipgloss.Color("245")).Padding(0, 1),
	}
)

func groupHeaderStyle(status string) lipgloss.Style {
	col := lipgloss.Color("7")
	switch status {
	case "running":
		col = lipgloss.Color("2")
	case "idea":
		col = lipgloss.Color("3")
	case "planned":
		col = lipgloss.Color("4")
	case "active":
		col = lipgloss.Color("6")
	case "done":
		col = lipgloss.Color("8")
	}
	return lipgloss.NewStyle().Bold(true).Foreground(col)
}

func helpEntry(key, desc string) string {
	return styleHelpKey.Render(key) + " " + styleHelpDesc.Render(desc)
}

func (m sessionTUIModel) View() string {
	var sb strings.Builder

	// New session input
	if m.newMode {
		sb.WriteString("\n")
		sb.WriteString(styleTitle.Render(" New Session "))
		sb.WriteString("\n\n")
		sb.WriteString("  Session name: ")
		sb.WriteString(styleSearchBar.Render(m.newInput + "▏"))
		sb.WriteString("\n\n")
		sb.WriteString(" ")
		sb.WriteString(helpEntry("Enter", "create & start"))
		sb.WriteString("  ")
		sb.WriteString(helpEntry("Esc", "cancel"))
		sb.WriteString("\n")
		return sb.String()
	}

	// History mode
	if m.historyMode {
		sb.WriteString(styleTitle.Render(" Session History "))
		sb.WriteString("  ")
		sb.WriteString(styleCount.Render(fmt.Sprintf("%d questions", len(m.historyItems))))
		sb.WriteString("\n\n")

		// Pre-compute wrapped lines per item
		type renderedItem struct {
			lines []string
		}
		allItems := make([]renderedItem, len(m.historyItems))
		for i, text := range m.historyItems {
			allItems[i] = renderedItem{lines: wrapText(text, 68)}
		}

		// Line-budget based scrolling: find window around cursor that fits ~30 lines
		// Use terminal height minus header(3) + footer(3) lines
		maxLines := m.height - 6
		if maxLines < 10 {
			maxLines = 10
		}
		// Start from cursor, expand outward
		start, end := m.historyCursor, m.historyCursor+1
		usedLines := len(allItems[m.historyCursor].lines) + 1 // +1 for separator
		// Expand upward and downward alternately
		for {
			expanded := false
			if start > 0 {
				candidate := len(allItems[start-1].lines) + 1
				if usedLines+candidate <= maxLines {
					start--
					usedLines += candidate
					expanded = true
				}
			}
			if end < len(allItems) {
				candidate := len(allItems[end].lines) + 1
				if usedLines+candidate <= maxLines {
					end++
					usedLines += candidate
					expanded = true
				}
			}
			if !expanded {
				break
			}
		}

		for i := start; i < end; i++ {
			num := fmt.Sprintf("%3d", i+1)
			isSel := i == m.historyCursor
			lines := allItems[i].lines

			if isSel {
				first := lines[0]
				sb.WriteString(styleSelected.Render(fmt.Sprintf(" ▸ %s  %s", num, first)))
				padW := 74 - 7 - lsDispWidth(first)
				if padW > 0 {
					sb.WriteString(styleSelected.Render(strings.Repeat(" ", padW)))
				}
				sb.WriteString("\n")
				for _, l := range lines[1:] {
					sb.WriteString(styleSelected.Render(fmt.Sprintf("         %s", l)))
					padW = 74 - 9 - lsDispWidth(l)
					if padW > 0 {
						sb.WriteString(styleSelected.Render(strings.Repeat(" ", padW)))
					}
					sb.WriteString("\n")
				}
			} else {
				sb.WriteString(styleDate.Render(fmt.Sprintf("   %s", num)))
				sb.WriteString(fmt.Sprintf("  %s\n", lines[0]))
				for _, l := range lines[1:] {
					sb.WriteString(fmt.Sprintf("         %s\n", l))
				}
			}

			if i < end-1 {
				sb.WriteString(styleFaint.Render("   ───"))
				sb.WriteString("\n")
			}
		}

		sb.WriteString("\n")
		sb.WriteString(styleFaint.Render(fmt.Sprintf("   %d/%d", m.historyCursor+1, len(m.historyItems))))
		sb.WriteString("\n")

		sb.WriteString(" ")
		sb.WriteString(helpEntry("↑↓", "move"))
		sb.WriteString("  ")
		sb.WriteString(helpEntry("Space", "back"))
		sb.WriteString("  ")
		sb.WriteString(helpEntry("Esc", "back"))
		sb.WriteString("\n")
		return sb.String()
	}

	// Delete confirmation — full screen takeover
	if m.confirmDelete {
		sb.WriteString("\n")
		sb.WriteString(styleConfirm.Render(fmt.Sprintf("  ⚠  Delete session '%s'? ", m.deleteTarget)))
		sb.WriteString(styleHelpKey.Render("[y]"))
		sb.WriteString(styleHelpDesc.Render(" yes  "))
		sb.WriteString(styleHelpKey.Render("[N]"))
		sb.WriteString(styleHelpDesc.Render(" cancel"))
		sb.WriteString("\n")
		return sb.String()
	}

	// Title bar
	total := len(m.sessions)
	sb.WriteString(styleTitle.Render(" cq sessions "))
	sb.WriteString(" ")
	sb.WriteString(styleCount.Render(fmt.Sprintf("%d sessions", total)))

	// External AI tool indicator
	if len(m.externalTools) > 0 {
		toolCounts := make(map[string]int)
		for _, t := range m.externalTools {
			toolCounts[t.tool]++
		}
		var parts []string
		for _, tool := range []string{"claude", "gemini", "cursor", "codex"} {
			if c, ok := toolCounts[tool]; ok {
				if c == 1 {
					parts = append(parts, tool)
				} else {
					parts = append(parts, fmt.Sprintf("%s(%d)", tool, c))
				}
			}
		}
		runningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
		sb.WriteString("  ")
		sb.WriteString(runningStyle.Render("● " + strings.Join(parts, " ")))
	}
	sb.WriteString("\n\n")

	// Search bar
	filterLabel := m.statusFilter
	if filterLabel == "" || filterLabel == "all" {
		filterLabel = "all"
	}
	if m.isSearching() {
		sb.WriteString("  ")
		sb.WriteString(styleSearchBar.Render(fmt.Sprintf(" 🔍 %s▏ ", m.query)))
	} else {
		sb.WriteString("  ")
		sb.WriteString(styleSearchPlaceholder.Render(" 🔍 type to search... "))
	}

	// Filter badge
	badgeStyle, ok := statusBadgeStyles[filterLabel]
	if !ok {
		badgeStyle = lipgloss.NewStyle().Background(lipgloss.Color("240")).Foreground(lipgloss.Color("15")).Padding(0, 1)
	}
	if filterLabel == "all" {
		badgeStyle = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("15")).Padding(0, 1)
	}
	sb.WriteString("  ")
	sb.WriteString(badgeStyle.Render(filterLabel))
	sb.WriteString("\n\n")

	// Rows — display-width-aware column alignment
	// Fixed: cursor(3) + markers(4) + tag(18) + gap(1) + badge(~14) + gap(1) + summary(dynamic) + gap(1) + tool(8) + gap(1) + date(12)
	const tagColW = 18
	// Fixed columns: cursor(3) + markers(4) + tag(18) + sp(1) + badge(13) + sp(1) + sp(1) + tool(8) + sp(1) + date(12) + margin(1)
	fixedW := 3 + 4 + tagColW + 1 + 13 + 1 + 1 + 8 + 1 + 12 + 1
	sumColW := m.width - fixedW
	if sumColW < 10 {
		sumColW = 10
	}
	cursorRowIdx := m.cursorRowIndex()
	nonHeaderCount := 0

	// Viewport: keep cursor in view, start from top
	// Account for detail expansion lines of the selected row
	detailLines := 0
	if cursorRowIdx >= 0 && cursorRowIdx < len(m.rows) {
		row := m.rows[cursorRowIdx]
		if len(row.ideaPaths) > 0 || row.specPath != "" || row.designPath != "" {
			detailLines = len(row.ideaPaths)
			if row.specPath != "" {
				detailLines++
			}
			if row.designPath != "" {
				detailLines++
			}
		}
	}
	maxVisible := m.height - 8 - detailLines
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
		if viewStart < 0 {
			viewStart = 0
		}
	}

	if viewStart > 0 {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("  ▲ %d more", viewStart)))
		sb.WriteString("\n")
	}

	for i := viewStart; i < viewEnd; i++ {
		row := m.rows[i]
		if row.isHeader {
			hs := groupHeaderStyle(row.status)
			label := fmt.Sprintf(" ── %s (%d) ", row.status, row.count)
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

		isSelected := i == cursorRowIdx
		nonHeaderCount++

		// Status badge pill — fixed visual width (pad shorter badges)
		bStyle, ok := statusBadgeStyles[row.rowStatus]
		if !ok {
			bStyle = statusBadgeStyles["active"]
		}
		// Center status text in a fixed-width badge field.
		statusText := row.rowStatus
		padTotal := 11 - len(statusText)
		if padTotal > 0 {
			padLeft := padTotal / 2
			padRight := padTotal - padLeft
			statusText = strings.Repeat(" ", padLeft) + statusText + strings.Repeat(" ", padRight)
		}
		badge := bStyle.Render(statusText)
		badgeW := lipgloss.Width(badge)

		cursor := "   "
		if isSelected {
			cursor = " ▸ "
		}

		// Doc markers: small dots indicating which documents exist
		var markers string
		if len(row.ideaPaths) > 0 || row.specPath != "" || row.designPath != "" {
			m1, m2, m3 := "·", "·", "·"
			if len(row.ideaPaths) > 0 {
				m1 = "●"
			}
			if row.specPath != "" {
				m2 = "●"
			}
			if row.designPath != "" {
				m3 = "●"
			}
			markers = m1 + m2 + m3 + " "
		} else {
			markers = "    "
		}

		// Tag: strip provider prefix (shown in tool column instead)
		tagDisplay := row.tag
		if idx := strings.IndexByte(tagDisplay, '/'); idx >= 0 {
			tagDisplay = tagDisplay[idx+1:]
		}
		if lsDispWidth(tagDisplay) > tagColW {
			tagDisplay = lsTruncateToWidth(tagDisplay, tagColW-1) + "…"
		}
		tagPadded := lsPadToWidth(tagDisplay, tagColW)

		// Tool badge (short label, right before date)
		const toolColW = 8
		toolLabel := row.tool
		if len(toolLabel) > toolColW {
			toolLabel = toolLabel[:toolColW]
		}
		toolPadded := fmt.Sprintf("%-*s", toolColW, toolLabel)

		// Summary: truncate + pad (CJK-aware)
		sumDisplay := row.summary
		if lsDispWidth(sumDisplay) > sumColW {
			sumDisplay = lsTruncateToWidth(sumDisplay, sumColW-1) + "…"
		}
		dateStr := row.date

		markerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Faint(true)
		markerStyleSel := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Background(lipgloss.Color("236"))

		// Calculate padding between summary and date to right-align date
		// Layout: cursor(3) + markers(4) + tag + " " + badge + " " + summary + pad + tool(8) + " " + date
		leftUsed := 3 + 4 + tagColW + 1 + badgeW + 1 + lsDispWidth(sumDisplay)
		dateW := toolColW + 1 + len(dateStr) + 1
		midPad := m.width - leftUsed - dateW
		if midPad < 1 {
			midPad = 1
		}

		toolStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Faint(true)
		toolStyleSel := lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Background(lipgloss.Color("236"))

		if isSelected {
			sb.WriteString(styleSelected.Render(cursor))
			sb.WriteString(markerStyleSel.Render(markers))
			sb.WriteString(styleSelected.Render(tagPadded))
			sb.WriteString(styleSelected.Render(" "))
			sb.WriteString(badge)
			sb.WriteString(styleSelected.Render(" "))
			sb.WriteString(styleSelected.Render(sumDisplay))
			sb.WriteString(styleSelected.Render(strings.Repeat(" ", midPad)))
			sb.WriteString(toolStyleSel.Render(toolPadded))
			sb.WriteString(styleSelected.Render(" "))
			sb.WriteString(styleSelected.Render(dateStr))
		} else if row.rowStatus == "done" {
			sb.WriteString(styleTagNameDim.Render(cursor))
			sb.WriteString(styleFaint.Render(markers))
			sb.WriteString(styleTagNameDim.Render(tagPadded))
			sb.WriteString(" ")
			sb.WriteString(badge)
			sb.WriteString(" ")
			sb.WriteString(styleSummaryDim.Render(sumDisplay))
			sb.WriteString(strings.Repeat(" ", midPad))
			sb.WriteString(styleFaint.Render(toolPadded))
			sb.WriteString(" ")
			sb.WriteString(styleDate.Render(dateStr))
		} else {
			sb.WriteString(cursor)
			sb.WriteString(markerStyle.Render(markers))
			sb.WriteString(styleTagName.Render(tagPadded))
			sb.WriteString(" ")
			sb.WriteString(badge)
			sb.WriteString(" ")
			sb.WriteString(styleSummary.Render(sumDisplay))
			sb.WriteString(strings.Repeat(" ", midPad))
			sb.WriteString(toolStyle.Render(toolPadded))
			sb.WriteString(" ")
			sb.WriteString(styleDate.Render(dateStr))
		}
		sb.WriteString("\n")

		// File paths for selected row
		if isSelected {
			paths := m.detailPaths()
			pathIdx := 0
			renderPath := func(icon, path string, isLast bool) {
				branch := "├─"
				if isLast {
					branch = "└─"
				}
				line := fmt.Sprintf("      %s %s %s", branch, icon, path)
				if m.detailMode && pathIdx == m.detailCursor {
					sb.WriteString(styleFileSelected.Render(line))
				} else {
					sb.WriteString(styleFilePath.Render(line))
				}
				sb.WriteString("\n")
				pathIdx++
			}
			_ = paths
			hasFiles := len(row.ideaPaths) > 0 || row.specPath != "" || row.designPath != ""
			if hasFiles {
				for ii, ip := range row.ideaPaths {
					isLast := ii == len(row.ideaPaths)-1 && row.specPath == "" && row.designPath == ""
					renderPath("💡", ip, isLast)
				}
				if row.specPath != "" {
					renderPath("📄", row.specPath, row.designPath == "")
				}
				if row.designPath != "" {
					renderPath("🏗 ", row.designPath, true)
				}
			}
		}
	}

	if viewEnd < len(m.rows) {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("  ▼ %d more", len(m.rows)-viewEnd)))
		sb.WriteString("\n")
	}

	if nonHeaderCount == 0 {
		sb.WriteString("\n")
		sb.WriteString(styleFaint.Render("  No sessions match your search."))
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
		helpBar.WriteString(helpEntry("→", "files"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("Space", "history"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("Enter", "start"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("^S", "done/active"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("^D", "delete"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("^N", "new"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("Tab", "filter"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("Esc", "quit/clear"))
	}

	// Fill remaining space to pin help bar at bottom
	content := sb.String()
	contentLines := strings.Count(content, "\n")
	if m.height > 0 {
		// -3 for separator + help bar + nav bar
		gap := m.height - contentLines - 2
		for i := 0; i < gap; i++ {
			sb.WriteString("\n")
		}
	}
	// Separator + help bar + nav bar
	if m.width > 0 {
		sb.WriteString(styleFaint.Render(strings.Repeat("─", m.width)))
	} else {
		sb.WriteString(styleFaint.Render(strings.Repeat("─", 74)))
	}
	sb.WriteString("\n")
	sb.WriteString(helpBar.String())

	return sb.String()
}

// runSessionsNav runs sessions TUI and returns the next screen name for the main loop.
func runSessionsNav() string {
	m := newSessionTUIModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: sessions TUI error: %v\n", err)
		return screenQuit
	}
	final, ok := result.(sessionTUIModel)
	if !ok {
		return screenQuit
	}
	if final.nextScreen != "" {
		return final.nextScreen
	}
	// Session selected → launch it.
	if final.selectedTag != "" {
		tool := ""
		if entry, ok := final.sessions[final.selectedTag]; ok {
			tool = inferSessionTool(final.selectedTag, entry)
		}
		_ = resumeSelectedSession(final.selectedTag, tool)
		return screenSessions
	}
	// No explicit action — user quit sessions (Esc/q without nav key).
	return screenQuit
}

// runSessionsTUI returns (selectedTag, tool, error).
// tool is the AI tool name for the selected session (claude/gemini/codex/chatgpt).
func runSessionsTUI() (string, string, error) {
	m := newSessionTUIModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return "", "", err
	}
	if final, ok := result.(sessionTUIModel); ok {
		tool := ""
		if final.selectedTag != "" {
			if entry, ok := final.sessions[final.selectedTag]; ok {
				tool = inferSessionTool(final.selectedTag, entry)
			}
		}
		return final.selectedTag, tool, nil
	}
	return "", "", nil
}
