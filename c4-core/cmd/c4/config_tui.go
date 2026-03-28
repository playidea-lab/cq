package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/changmin/c4-core/internal/mcp/handlers/cfghandler"
)

// sourceBadgeStyles maps config source to colored badge styles.
var sourceBadgeStyles = map[string]lipgloss.Style{
	"default":  lipgloss.NewStyle().Faint(true).Padding(0, 1),
	"project":  lipgloss.NewStyle().Background(lipgloss.Color("2")).Foreground(lipgloss.Color("0")).Padding(0, 1),
	"global":   lipgloss.NewStyle().Background(lipgloss.Color("4")).Foreground(lipgloss.Color("15")).Padding(0, 1),
	"env":      lipgloss.NewStyle().Background(lipgloss.Color("3")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1),
	"readonly": lipgloss.NewStyle().Background(lipgloss.Color("240")).Foreground(lipgloss.Color("252")).Padding(0, 1),
}

// configTUIModel is the bubbletea model for the config TUI.
type configTUIModel struct {
	entries       []configEntry
	rows          []configRow
	cursor        int
	query         string
	sectionFilter string // "all" or section name
	width, height int

	// Navigation
	nextScreen string

	// Inline editing
	editMode  bool   // inline editing active
	editInput string // current edit text
	editIndex int    // which entry index is being edited

	// Array expansion
	arrayExpanded int  // index of expanded array entry (-1 = none)
	arrayCursor   int  // cursor within array items
	arrayAddMode  bool // adding new array item
	arrayAddInput string

	// Array delete confirmation
	confirmDelete  bool // waiting for delete confirmation
	deleteArrayIdx int  // which array item to delete
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
		arrayExpanded: -1,
		editIndex:     -1,
	}
	m.rows = m.buildVisibleRows()
	return m
}

func (m configTUIModel) Init() tea.Cmd {
	return nil
}

// editorFinishedMsg is sent after an external editor exits.
type editorFinishedMsg struct{ err error }

func (m configTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.rows = m.buildVisibleRows()
		return m, nil

	case editorFinishedMsg:
		// Re-scan entries after editor exit, preserving external tool entries
		if entries, err := scanConfigEntries(projectDir); err == nil {
			entries = append(entries, scanExternalToolEntries()...)
			m.entries = entries
			m.arrayExpanded = -1
			m.rows = m.buildVisibleRows()
		}
		return m, nil

	case tea.KeyMsg:
		// Delete confirmation mode
		if m.confirmDelete {
			switch msg.String() {
			case "y", "Y":
				m.handleArrayDelete()
			default:
				m.confirmDelete = false
			}
			return m, nil
		}

		// Array add mode
		if m.arrayAddMode {
			switch msg.Type {
			case tea.KeyEnter:
				if m.arrayAddInput != "" {
					m.handleArrayAdd(m.arrayAddInput)
				}
				m.arrayAddMode = false
				m.arrayAddInput = ""
			case tea.KeyEsc:
				m.arrayAddMode = false
				m.arrayAddInput = ""
			case tea.KeyBackspace:
				if len(m.arrayAddInput) > 0 {
					runes := []rune(m.arrayAddInput)
					m.arrayAddInput = string(runes[:len(runes)-1])
				}
			default:
				if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
					m.arrayAddInput += msg.String()
				}
			}
			return m, nil
		}

		// Edit mode
		if m.editMode {
			switch msg.Type {
			case tea.KeyEnter:
				m.handleEditSave()
				m.editMode = false
			case tea.KeyEsc:
				m.editMode = false
			case tea.KeyBackspace:
				if len(m.editInput) > 0 {
					runes := []rune(m.editInput)
					m.editInput = string(runes[:len(runes)-1])
				}
			default:
				if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
					m.editInput += msg.String()
				}
			}
			return m, nil
		}

		// Array expanded navigation
		if m.arrayExpanded >= 0 {
			switch msg.Type {
			case tea.KeyUp:
				if m.arrayCursor > 0 {
					m.arrayCursor--
				}
				return m, nil
			case tea.KeyDown:
				items := configArrayItems(m.entries[m.arrayExpanded].Value)
				if m.arrayCursor < len(items)-1 {
					m.arrayCursor++
				}
				return m, nil
			case tea.KeyEsc:
				m.arrayExpanded = -1
				m.arrayCursor = 0
				return m, nil
			case tea.KeyRunes:
				ch := msg.String()
				switch ch {
				case "k":
					if m.arrayCursor > 0 {
						m.arrayCursor--
					}
					return m, nil
				case "j":
					items := configArrayItems(m.entries[m.arrayExpanded].Value)
					if m.arrayCursor < len(items)-1 {
						m.arrayCursor++
					}
					return m, nil
				case "a":
					m.arrayAddMode = true
					m.arrayAddInput = ""
					return m, nil
				case "d":
					items := configArrayItems(m.entries[m.arrayExpanded].Value)
					if len(items) > 0 && m.arrayCursor < len(items) {
						m.confirmDelete = true
						m.deleteArrayIdx = m.arrayCursor
					}
					return m, nil
				case "e":
					return m, m.openEditorCmd()
				case "q":
					m.arrayExpanded = -1
					m.arrayCursor = 0
					return m, nil
				}
			}
		}

		// Normal mode — check global nav keys first.
		inputActive := m.editMode || m.arrayAddMode || m.arrayExpanded >= 0 || m.query != ""
		if next, ok := handleGlobalKey(msg, inputActive); ok {
			m.nextScreen = next
			return m, tea.Quit
		}

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
			case " ":
				if m.query != "" {
					m.query += ch
					m.cursor = 0
					m.rows = m.buildVisibleRows()
				} else {
					m.handleBoolToggle()
				}
			case "a", "d", "e":
				if m.query != "" {
					m.query += ch
					m.cursor = 0
					m.rows = m.buildVisibleRows()
				}
				// no-op in normal mode without array expanded
			default:
				m.query += ch
				m.cursor = 0
				m.rows = m.buildVisibleRows()
			}
		case tea.KeyEnter:
			m.handleEnter()
		}
	}
	return m, nil
}

// cursorEntry returns the entry at the current cursor position, or nil.
func (m *configTUIModel) cursorEntry() *configEntry {
	indices := configNonHeaderRows(m.rows)
	if len(indices) == 0 {
		return nil
	}
	c := m.cursor
	if c < 0 {
		c = 0
	}
	if c >= len(indices) {
		c = len(indices) - 1
	}
	return &m.entries[m.rows[indices[c]].index]
}

// cursorEntryIndex returns the entries index at the current cursor position, or -1.
func (m *configTUIModel) cursorEntryIndex() int {
	indices := configNonHeaderRows(m.rows)
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
	return m.rows[indices[c]].index
}

// handleBoolToggle toggles a bool entry on Space.
func (m *configTUIModel) handleBoolToggle() {
	idx := m.cursorEntryIndex()
	if idx < 0 {
		return
	}
	entry := &m.entries[idx]
	if entry.Source == "readonly" {
		return // read-only external tool config — not editable
	}
	if entry.Kind != "bool" {
		return
	}
	newVal := "true"
	if fmt.Sprint(entry.Value) == "true" {
		newVal = "false"
	}
	if err := saveConfigValue(entry.Key, newVal); err != nil {
		return
	}
	entry.Value = newVal == "true"
	entry.Source = "project"
	m.rows = m.buildVisibleRows()
}

// handleEnter handles Enter on entries: toggle array expand, bool toggle, or start inline edit.
// For sensitive keys, opens inline edit with empty input (new value entry).
func (m *configTUIModel) handleEnter() {
	idx := m.cursorEntryIndex()
	if idx < 0 {
		return
	}
	entry := &m.entries[idx]
	if entry.Source == "readonly" {
		return // read-only external tool config — not editable
	}

	// Sensitive key: enter edit mode with empty input (user types new value)
	if isSensitiveKey(entry.Key) {
		m.editMode = true
		m.editInput = "" // start empty — user enters new key
		m.editIndex = idx
		return
	}

	switch entry.Kind {
	case "array":
		if m.arrayExpanded == idx {
			m.arrayExpanded = -1
			m.arrayCursor = 0
		} else {
			m.arrayExpanded = idx
			m.arrayCursor = 0
		}
	case "bool":
		m.handleBoolToggle()
	default:
		m.editMode = true
		m.editInput = fmt.Sprint(entry.Value)
		m.editIndex = idx
	}
}

// handleEditSave saves the inline edit value.
// Sensitive keys are saved via `cq secret set` (encrypted store).
func (m *configTUIModel) handleEditSave() {
	if m.editIndex < 0 || m.editIndex >= len(m.entries) {
		return
	}
	entry := &m.entries[m.editIndex]

	if isSensitiveKey(entry.Key) {
		// Save to secret store via CLI (subprocess — avoids importing secrets package)
		if m.editInput != "" {
			cmd := exec.Command("cq", "secret", "set", entry.Key, m.editInput)
			_ = cmd.Run()
			entry.Value = m.editInput
			entry.Source = "project"
		}
		m.rows = m.buildVisibleRows()
		return
	}

	if err := saveConfigValue(entry.Key, m.editInput); err != nil {
		return
	}
	entry.Value = m.editInput
	entry.Source = "project"
	m.rows = m.buildVisibleRows()
}

// handleArrayAdd appends an item to the expanded array.
func (m *configTUIModel) handleArrayAdd(item string) {
	if m.arrayExpanded < 0 || m.arrayExpanded >= len(m.entries) {
		return
	}
	entry := &m.entries[m.arrayExpanded]
	items := configArrayItems(entry.Value)
	items = append(items, item)
	if err := saveConfigArray(entry.Key, items); err != nil {
		return
	}
	entry.Value = configToAnySlice(items)
	entry.Source = "project"
	m.arrayCursor = len(items) - 1
	m.rows = m.buildVisibleRows()
}

// handleArrayDelete removes the selected array item.
func (m *configTUIModel) handleArrayDelete() {
	m.confirmDelete = false
	if m.arrayExpanded < 0 || m.arrayExpanded >= len(m.entries) {
		return
	}
	entry := &m.entries[m.arrayExpanded]
	items := configArrayItems(entry.Value)
	if m.deleteArrayIdx < 0 || m.deleteArrayIdx >= len(items) {
		return
	}
	items = append(items[:m.deleteArrayIdx], items[m.deleteArrayIdx+1:]...)
	if err := saveConfigArray(entry.Key, items); err != nil {
		return
	}
	entry.Value = configToAnySlice(items)
	entry.Source = "project"
	if m.arrayCursor >= len(items) && len(items) > 0 {
		m.arrayCursor = len(items) - 1
	}
	m.rows = m.buildVisibleRows()
}

// openEditorCmd returns a tea.Cmd that opens an external editor on the config file.
func (m *configTUIModel) openEditorCmd() tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		if _, err := exec.LookPath("code"); err == nil {
			editor = "code"
		} else {
			editor = "vi"
		}
	}
	path := cfghandler.ConfigFilePath(projectDir)
	c := exec.Command(editor, path) //nolint:gosec
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{err: err}
	})
}

// saveConfigValue saves a single key=value to the project config.
func saveConfigValue(key, value string) error {
	path := cfghandler.ConfigFilePath(projectDir)
	return cfghandler.UpdateYAMLValue(path, key, value)
}

// saveConfigArray writes an array to the project config YAML by rewriting the array block.
func saveConfigArray(dotKey string, items []string) error {
	path := cfghandler.ConfigFilePath(projectDir)
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if os.IsNotExist(err) {
		dir := path[:strings.LastIndex(path, "/")]
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			return mkErr
		}
		data = []byte{}
	}

	lines := strings.Split(string(data), "\n")
	parts := strings.Split(dotKey, ".")

	newLines := configReplaceArrayBlock(lines, parts, items)

	output := strings.Join(newLines, "\n")
	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	return os.WriteFile(path, []byte(output), 0o644)
}

// configReplaceArrayBlock finds the array key in YAML and replaces its items.
func configReplaceArrayBlock(lines []string, parts []string, items []string) []string {
	// Find the leaf key line
	leafKey := parts[len(parts)-1]
	targetIndent := (len(parts) - 1) * 2

	var result []string
	i := 0
	found := false

	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Check if this line is our target key
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent == targetIndent && strings.HasPrefix(trimmed, leafKey+":") {
			// Check that parent sections match (simplified: just match the leaf)
			if configArrayParentsMatch(lines, i, parts) {
				found = true
				result = append(result, line) // keep the key line
				i++

				// Skip old array items (lines starting with "- " at indent+2)
				itemIndent := targetIndent + 2
				for i < len(lines) {
					l := lines[i]
					li := len(l) - len(strings.TrimLeft(l, " "))
					lt := strings.TrimSpace(l)
					if li == itemIndent && strings.HasPrefix(lt, "- ") {
						i++ // skip old item
						continue
					}
					if lt == "" {
						i++ // skip blank lines within array
						continue
					}
					break
				}

				// Write new items
				prefix := strings.Repeat(" ", itemIndent)
				for _, item := range items {
					result = append(result, prefix+"- "+item)
				}
				continue
			}
		}
		result = append(result, line)
		i++
	}

	// If key not found, append the entire structure
	if !found {
		result = configAppendArrayKey(result, parts, items)
	}

	return result
}

// configArrayParentsMatch checks that the parent sections above line idx match parts[:len(parts)-1].
func configArrayParentsMatch(lines []string, idx int, parts []string) bool {
	if len(parts) <= 1 {
		return true
	}
	// Walk backwards to find parent sections
	for p := len(parts) - 2; p >= 0; p-- {
		expectedIndent := p * 2
		expectedKey := parts[p] + ":"
		found := false
		for j := idx - 1; j >= 0; j-- {
			l := lines[j]
			indent := len(l) - len(strings.TrimLeft(l, " "))
			trimmed := strings.TrimSpace(l)
			if indent == expectedIndent && strings.HasPrefix(trimmed, expectedKey) {
				found = true
				break
			}
			if indent <= expectedIndent && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// configAppendArrayKey appends a new array key with items to the YAML.
func configAppendArrayKey(lines []string, parts []string, items []string) []string {
	result := make([]string, len(lines))
	copy(result, lines)

	// Ensure parent sections exist, then append array
	for depth, part := range parts {
		indent := strings.Repeat(" ", depth*2)
		if depth < len(parts)-1 {
			// Check if section exists
			found := false
			for _, l := range result {
				li := len(l) - len(strings.TrimLeft(l, " "))
				if li == depth*2 && strings.TrimSpace(l) == part+":" {
					found = true
					break
				}
			}
			if !found {
				result = append(result, indent+part+":")
			}
		} else {
			// Leaf array key
			result = append(result, indent+part+":")
			itemIndent := strings.Repeat(" ", (depth+1)*2)
			for _, item := range items {
				result = append(result, itemIndent+"- "+item)
			}
		}
	}
	return result
}

// configArrayItems converts an entry value to a string slice.
func configArrayItems(v any) []string {
	switch arr := v.(type) {
	case []string:
		out := make([]string, len(arr))
		copy(out, arr)
		return out
	case []any:
		out := make([]string, 0, len(arr))
		for _, item := range arr {
			out = append(out, fmt.Sprint(item))
		}
		return out
	case []int:
		out := make([]string, 0, len(arr))
		for _, item := range arr {
			out = append(out, fmt.Sprint(item))
		}
		return out
	default:
		return nil
	}
}

// configToAnySlice converts a string slice to []any for storage.
func configToAnySlice(items []string) []any {
	out := make([]any, len(items))
	for i, s := range items {
		out[i] = s
	}
	return out
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

// sensitiveKeywords identifies config keys whose values should be masked.
var sensitiveKeywords = []string{
	"key", "token", "secret", "password", "credential", "auth",
	"api_key", "apikey", "access_token", "refresh_token",
}

// isSensitiveKey checks if a config key contains sensitive keywords.
func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, kw := range sensitiveKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// maskValue fully masks a string value.
func maskValue(val string) string {
	if val == "" {
		return "(미설정)"
	}
	return "••••••••"
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
		val := fmt.Sprint(e.Value)
		if isSensitiveKey(e.Key) && val != "" {
			return maskValue(val)
		}
		return val
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

	// Delete confirmation — full screen takeover
	if m.confirmDelete && m.arrayExpanded >= 0 {
		items := configArrayItems(m.entries[m.arrayExpanded].Value)
		itemText := ""
		if m.deleteArrayIdx >= 0 && m.deleteArrayIdx < len(items) {
			itemText = items[m.deleteArrayIdx]
		}
		sb.WriteString("\n")
		sb.WriteString(styleConfirm.Render(fmt.Sprintf("  \u26A0  Delete item '%s'? ", itemText)))
		sb.WriteString(styleFaint.Render(" (y/N)"))
		sb.WriteString("\n")
		return sb.String()
	}

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

	// Viewport: keep cursor visible (sessions pattern)
	maxVisible := m.height - 8
	if maxVisible < 10 {
		maxVisible = 10
	}
	viewStart := 0
	viewEnd := len(rows)
	if len(rows) > maxVisible {
		viewEnd = viewStart + maxVisible
		if cursorIdx >= viewEnd {
			viewStart = cursorIdx - maxVisible + 3
			if viewStart < 0 {
				viewStart = 0
			}
			viewEnd = viewStart + maxVisible
			if viewEnd > len(rows) {
				viewEnd = len(rows)
			}
		}
	}

	if viewStart > 0 {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("  ▲ %d more", viewStart)))
		sb.WriteString("\n")
	}

	for i := viewStart; i < viewEnd; i++ {
		row := rows[i]
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

		// Value column — show edit input if editing this entry
		var valDisplay string
		if m.editMode && m.editIndex == row.index {
			valDisplay = m.editInput + "\u2581" // block cursor
		} else {
			valDisplay = configValueString(entry)
			if entry.Source == "readonly" {
				valDisplay = "\U0001F512 " + valDisplay // lock icon prefix for read-only entries
			}
		}
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

		// Show expanded array items after the array row
		if entry.Kind == "array" && m.arrayExpanded == row.index {
			items := configArrayItems(entry.Value)
			for ai, item := range items {
				prefix := "     \u251C "
				if ai == len(items)-1 && !m.arrayAddMode {
					prefix = "     \u2514 "
				}
				arrSelected := ai == m.arrayCursor
				itemDisplay := item
				if lsDispWidth(itemDisplay) > valColW+keyColW-6 {
					itemDisplay = lsTruncateToWidth(itemDisplay, valColW+keyColW-7) + "\u2026"
				}
				if arrSelected {
					sb.WriteString(styleSelected.Render(prefix + itemDisplay))
					// Pad to width
					lineW := lsDispWidth(prefix + itemDisplay)
					if m.width > lineW {
						sb.WriteString(styleSelected.Render(strings.Repeat(" ", m.width-lineW)))
					}
				} else {
					sb.WriteString(styleFaint.Render(prefix))
					sb.WriteString(itemDisplay)
				}
				sb.WriteString("\n")
			}
			// Show add input at end of array
			if m.arrayAddMode {
				addPrefix := "     \u2514 "
				addDisplay := m.arrayAddInput + "\u2581"
				sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(addPrefix))
				sb.WriteString(addDisplay)
				sb.WriteString("\n")
			}
		}
	}

	if viewEnd < len(rows) {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("  ▼ %d more", len(rows)-viewEnd)))
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

	sb.WriteString(m.configHelpBar())
	sb.WriteString("\n")
	sb.WriteString(renderNavBar(screenConfig, m.width))

	return sb.String()
}

// configHelpBar returns context-sensitive help text.
func (m configTUIModel) configHelpBar() string {
	var hb strings.Builder
	hb.WriteString(" ")

	if m.editMode {
		hb.WriteString(helpEntry("Enter", "confirm"))
		hb.WriteString("  ")
		hb.WriteString(helpEntry("Esc", "cancel"))
		return hb.String()
	}

	if m.arrayAddMode {
		hb.WriteString(helpEntry("Enter", "add"))
		hb.WriteString("  ")
		hb.WriteString(helpEntry("Esc", "cancel"))
		return hb.String()
	}

	if m.arrayExpanded >= 0 {
		hb.WriteString(helpEntry("\u2191\u2193", "navigate"))
		hb.WriteString("  ")
		hb.WriteString(helpEntry("a", "add"))
		hb.WriteString("  ")
		hb.WriteString(helpEntry("d", "delete"))
		hb.WriteString("  ")
		hb.WriteString(helpEntry("e", "editor"))
		hb.WriteString("  ")
		hb.WriteString(helpEntry("Esc", "close"))
		return hb.String()
	}

	hb.WriteString(helpEntry("\u2191\u2193", "navigate"))
	hb.WriteString("  ")
	hb.WriteString(helpEntry("Space", "toggle"))
	hb.WriteString("  ")
	hb.WriteString(helpEntry("Enter", "edit"))
	hb.WriteString("  ")
	hb.WriteString(helpEntry("Tab", "filter"))
	hb.WriteString("  ")
	hb.WriteString(helpEntry("q", "quit"))
	return hb.String()
}
