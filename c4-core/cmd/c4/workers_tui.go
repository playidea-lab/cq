package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/changmin/c4-core/internal/hub"
)

// workerStatusBadgeStyles maps worker status strings to badge styles.
var workerStatusBadgeStyles = map[string]lipgloss.Style{
	"online":  lipgloss.NewStyle().Background(lipgloss.Color("2")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1),
	"busy":    lipgloss.NewStyle().Background(lipgloss.Color("3")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1),
	"offline": lipgloss.NewStyle().Background(lipgloss.Color("237")).Foreground(lipgloss.Color("245")).Padding(0, 1),
	"idle":    lipgloss.NewStyle().Background(lipgloss.Color("4")).Foreground(lipgloss.Color("15")).Padding(0, 1),
}

// Status symbol styles
var (
	styleStatusOnline  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	styleStatusOffline = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleStatusRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	styleRelayConn     = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleRelayDisconn  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleLastSeen      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	// GPU bar colors
	styleGPUBarHigh   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))  // green >= 60%
	styleGPUBarMedium = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))  // yellow 30-59%
	styleGPUBarLow    = lipgloss.NewStyle().Foreground(lipgloss.Color("240")) // dim < 30%
)

const (
	gpuBarFull  = "█"
	gpuBarEmpty = "░"
)

// Messages for the workers TUI event loop.
type workersUpdatedMsg struct {
	workers []hub.Worker
	err     error
}

type workerTickMsg struct{}

type relayHealthMsg struct {
	connectedIDs map[string]bool
	err          error
}

// workersTUIModel is the bubbletea model for the workers TUI.
type workersTUIModel struct {
	workers      []hub.Worker
	cursor       int
	query        string
	width        int
	height       int
	loading      bool
	err          error
	tickCount    int
	relayConnected map[string]bool // worker ID -> connected to relay

	hubClient *hub.Client
	relayURL  string
}

func newWorkersTUIModel(client *hub.Client, relayURL string) workersTUIModel {
	return workersTUIModel{
		loading:      true,
		hubClient:    client,
		relayURL:     relayURL,
		relayConnected: make(map[string]bool),
	}
}

// fetchWorkersCmd returns a tea.Cmd that fetches workers from the hub asynchronously.
func fetchWorkersCmd(client *hub.Client) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return workersUpdatedMsg{err: fmt.Errorf("hub client not configured")}
		}
		workers, err := client.ListWorkers()
		return workersUpdatedMsg{workers: workers, err: err}
	}
}

// workerTickCmd returns a tea.Cmd that fires after 3 seconds.
func workerTickCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return workerTickMsg{}
	})
}

// fetchRelayHealthCmd fetches the relay /health endpoint to get connected worker IDs.
// It returns a relayHealthMsg. If relayURL is empty, it returns an empty map gracefully.
func fetchRelayHealthCmd(relayURL string) tea.Cmd {
	return func() tea.Msg {
		if relayURL == "" {
			return relayHealthMsg{connectedIDs: make(map[string]bool)}
		}
		healthURL := strings.TrimRight(relayURL, "/") + "/health"
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(healthURL)
		if err != nil {
			return relayHealthMsg{connectedIDs: make(map[string]bool), err: err}
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return relayHealthMsg{connectedIDs: make(map[string]bool), err: err}
		}
		// Try to parse connected worker IDs from relay health response.
		// Expected shape: {"connected_workers": ["id1","id2",...]} or {"workers": [...]}
		var payload struct {
			ConnectedWorkers []string `json:"connected_workers"`
			Workers          []string `json:"workers"`
		}
		connected := make(map[string]bool)
		if err := json.Unmarshal(body, &payload); err == nil {
			for _, id := range payload.ConnectedWorkers {
				connected[id] = true
			}
			for _, id := range payload.Workers {
				connected[id] = true
			}
		}
		return relayHealthMsg{connectedIDs: connected}
	}
}

func (m workersTUIModel) Init() tea.Cmd {
	return tea.Batch(
		fetchWorkersCmd(m.hubClient),
		fetchRelayHealthCmd(m.relayURL),
		workerTickCmd(),
	)
}

func (m workersTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case workersUpdatedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.workers = msg.workers
		}
		return m, nil

	case relayHealthMsg:
		if msg.err == nil {
			m.relayConnected = msg.connectedIDs
		}
		return m, nil

	case workerTickMsg:
		m.tickCount++
		// Refresh every tick (3s interval)
		return m, tea.Batch(
			fetchWorkersCmd(m.hubClient),
			fetchRelayHealthCmd(m.relayURL),
			workerTickCmd(),
		)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp:
			visible := m.visibleWorkers()
			if m.cursor > 0 {
				m.cursor--
			}
			_ = visible
		case tea.KeyDown:
			visible := m.visibleWorkers()
			if m.cursor < len(visible)-1 {
				m.cursor++
			}
		case tea.KeyEsc:
			if m.query != "" {
				m.query = ""
				m.cursor = 0
				return m, nil
			}
			return m, tea.Quit
		case tea.KeyBackspace:
			if len(m.query) > 0 {
				runes := []rune(m.query)
				m.query = string(runes[:len(runes)-1])
				m.cursor = 0
			}
		case tea.KeyRunes:
			ch := msg.String()
			switch ch {
			case "k":
				if m.query == "" {
					if m.cursor > 0 {
						m.cursor--
					}
					return m, nil
				}
				m.query += ch
				m.cursor = 0
			case "j":
				if m.query == "" {
					visible := m.visibleWorkers()
					if m.cursor < len(visible)-1 {
						m.cursor++
					}
					return m, nil
				}
				m.query += ch
				m.cursor = 0
			case "q":
				if m.query == "" {
					return m, tea.Quit
				}
				m.query += ch
				m.cursor = 0
			case "r":
				// Manual refresh
				m.loading = true
				return m, tea.Batch(fetchWorkersCmd(m.hubClient), fetchRelayHealthCmd(m.relayURL), workerTickCmd())
			default:
				m.query += ch
				m.cursor = 0
			}
		}
	}
	return m, nil
}

// visibleWorkers returns the subset of workers matching the current search query.
func (m *workersTUIModel) visibleWorkers() []hub.Worker {
	if m.query == "" {
		return m.workers
	}
	lower := strings.ToLower(m.query)
	tokens := strings.Fields(lower)
	var out []hub.Worker
	for _, w := range m.workers {
		corpus := strings.ToLower(w.ID + " " + w.Hostname + " " + w.Status + " " + w.GPUModel + " " + strings.Join(w.Capabilities, " "))
		match := true
		for _, tok := range tokens {
			if !strings.Contains(corpus, tok) {
				match = false
				break
			}
		}
		if match {
			out = append(out, w)
		}
	}
	return out
}

// renderGPUBar renders a GPU utilization bar like "████████░░ 82%".
// barWidth is the total character width of the bar (excluding the percentage text).
func renderGPUBar(utilPct int, barWidth int) string {
	if barWidth < 4 {
		barWidth = 4
	}
	if utilPct < 0 {
		utilPct = 0
	}
	if utilPct > 100 {
		utilPct = 100
	}
	filled := barWidth * utilPct / 100
	empty := barWidth - filled

	bar := strings.Repeat(gpuBarFull, filled) + strings.Repeat(gpuBarEmpty, empty)
	label := fmt.Sprintf(" %3d%%", utilPct)

	var style lipgloss.Style
	switch {
	case utilPct >= 60:
		style = styleGPUBarHigh
	case utilPct >= 30:
		style = styleGPUBarMedium
	default:
		style = styleGPUBarLow
	}

	return style.Render(bar) + label
}

// workerGPUUtil calculates GPU utilization percentage for a worker.
// Prefers GPUs[].Utilization average; falls back to VRAM usage ratio.
func workerGPUUtil(w hub.Worker) int {
	if len(w.GPUs) > 0 {
		total := 0
		for _, g := range w.GPUs {
			total += g.Utilization
		}
		return total / len(w.GPUs)
	}
	if w.TotalVRAM > 0 {
		used := w.TotalVRAM - w.FreeVRAM
		return int(used / w.TotalVRAM * 100)
	}
	return 0
}

// statusSymbol returns a symbol + style string for a worker status.
// "online" → green ●, "offline" → dim ○, "busy"/"running" → yellow ▸
func statusSymbol(status string) string {
	switch status {
	case "online", "idle":
		return styleStatusOnline.Render("●")
	case "busy", "running":
		return styleStatusRunning.Render("▸")
	case "offline":
		return styleStatusOffline.Render("○")
	default:
		return styleStatusOffline.Render("○")
	}
}

// relaySymbol returns a relay connection symbol.
func relaySymbol(connected bool) string {
	if connected {
		return styleRelayConn.Render("⇄")
	}
	return styleRelayDisconn.Render("⇄")
}

// lastSeenStr returns a human-readable "last seen X ago" string for a worker.
func lastSeenStr(w hub.Worker) string {
	if w.LastJobAt == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, w.LastJobAt)
	if err != nil {
		return ""
	}
	diff := time.Since(t)
	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
	}
}

func (m workersTUIModel) View() string {
	var sb strings.Builder

	// Title bar
	sb.WriteString(styleTitle.Render(" cq workers "))
	sb.WriteString(" ")
	if m.loading {
		sb.WriteString(styleCount.Render("loading..."))
	} else if m.err != nil {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(m.err.Error()))
	} else {
		visible := m.visibleWorkers()
		sb.WriteString(styleCount.Render(fmt.Sprintf("%d workers", len(visible))))
	}
	sb.WriteString("\n")

	// Search bar
	if m.query != "" {
		sb.WriteString("  ")
		sb.WriteString(styleSearchBar.Render(fmt.Sprintf(" 🔍 %s▏ ", m.query)))
	} else {
		sb.WriteString("  ")
		sb.WriteString(styleSearchPlaceholder.Render(" 🔍 type to search... "))
	}
	sb.WriteString("\n\n")

	if m.loading {
		spinner := spinnerFrames[m.tickCount%len(spinnerFrames)]
		sb.WriteString(fmt.Sprintf("  %s fetching workers...\n", spinner))
	} else if m.err != nil {
		sb.WriteString(fmt.Sprintf("  error: %s\n", m.err.Error()))
	} else {
		visible := m.visibleWorkers()
		if len(visible) == 0 {
			sb.WriteString(styleFaint.Render("  No workers found."))
			sb.WriteString("\n")
		} else {
			m.renderWorkerRows(&sb, visible)
		}
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
	m.renderSeparator(&sb)
	sb.WriteString("\n")

	var helpBar strings.Builder
	helpBar.WriteString(" ")
	helpBar.WriteString(helpEntry("↑↓", "navigate"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("r", "refresh"))
	helpBar.WriteString("  ")
	helpBar.WriteString(helpEntry("q", "quit"))
	sb.WriteString(helpBar.String())

	return sb.String()
}

// layoutMode defines how much information to show per worker row.
type workerLayoutMode int

const (
	layoutFull    workerLayoutMode = iota // width >= 100
	layoutCompact                         // width >= 80
	layoutMinimal                         // width < 80
)

func (m *workersTUIModel) layoutMode() workerLayoutMode {
	switch {
	case m.width >= 100:
		return layoutFull
	case m.width >= 80:
		return layoutCompact
	default:
		return layoutMinimal
	}
}

// renderWorkerRows writes worker list rows into sb.
func (m *workersTUIModel) renderWorkerRows(sb *strings.Builder, visible []hub.Worker) {
	c := m.cursor
	if c < 0 {
		c = 0
	}
	if c >= len(visible) {
		c = len(visible) - 1
	}

	mode := m.layoutMode()

	for i, w := range visible {
		isSelected := i == c
		m.renderWorkerRow(sb, w, isSelected, mode)
	}
}

// renderWorkerRow renders a single worker row based on layout mode.
func (m *workersTUIModel) renderWorkerRow(sb *strings.Builder, w hub.Worker, isSelected bool, mode workerLayoutMode) {
	cursor := "   "
	if isSelected {
		cursor = " ▸ "
	}

	// Hostname
	hostname := w.Hostname
	if hostname == "" {
		hostname = w.ID
	}

	// Status symbol (●/○/▸)
	sym := statusSymbol(w.Status)

	switch mode {
	case layoutMinimal:
		// Minimal: cursor + symbol + hostname
		hostW := m.width - 3 - 3 - 1
		if hostW < 10 {
			hostW = 10
		}
		hostStr := lsPadToWidth(hostname, hostW)
		if isSelected {
			sb.WriteString(styleSelected.Render(cursor + hostStr))
		} else {
			sb.WriteString(cursor)
			sb.WriteString(sym)
			sb.WriteString(" ")
			sb.WriteString(styleTagName.Render(hostStr))
		}
		sb.WriteString("\n")

	case layoutCompact:
		// Compact: cursor(3) + symbol(1) + sp(1) + hostname(20) + sp(1) + GPU bar(20) + sp(1) + vram(10)
		const hostColW = 20
		gpuBarW := 10
		const vramColW = 10
		hostStr := lsPadToWidth(hostname, hostColW)

		gpuUtil := 0
		if w.Status != "offline" {
			gpuUtil = workerGPUUtil(w)
		}
		gpuBar := renderGPUBar(gpuUtil, gpuBarW)

		vramStr := ""
		if w.TotalVRAM > 0 {
			vramStr = fmt.Sprintf("%.0f/%.0fG", w.FreeVRAM, w.TotalVRAM)
		}
		vramPadded := lsPadToWidth(vramStr, vramColW)

		if isSelected {
			sb.WriteString(styleSelected.Render(cursor))
			sb.WriteString(sym + " ")
			sb.WriteString(styleSelected.Render(hostStr + " "))
			sb.WriteString(gpuBar)
			sb.WriteString(styleSelected.Render(" " + vramPadded))
		} else {
			sb.WriteString(cursor)
			sb.WriteString(sym + " ")
			sb.WriteString(styleTagName.Render(hostStr))
			sb.WriteString(" ")
			sb.WriteString(gpuBar)
			sb.WriteString(" ")
			sb.WriteString(styleDate.Render(vramPadded))
		}
		sb.WriteString("\n")

	default: // layoutFull
		// Full: cursor(3) + symbol(2) + relay(2) + hostname(22) + status badge(9) + GPU model(16) + GPU bar(14) + VRAM(12) + last seen(12)
		const hostColW = 22
		const badgeFieldW = 9
		const gpuModelColW = 16
		gpuBarW := 10
		const vramColW = 12
		const lastSeenW = 12

		hostStr := lsPadToWidth(hostname, hostColW)

		// Status badge
		statusText := w.Status
		if statusText == "" {
			statusText = "unknown"
		}
		badge := ""
		if bs, ok := workerStatusBadgeStyles[statusText]; ok {
			badge = bs.Render(statusText)
		} else {
			badge = lipgloss.NewStyle().Background(lipgloss.Color("237")).Foreground(lipgloss.Color("245")).Padding(0, 1).Render(statusText)
		}
		badgeActualW := lipgloss.Width(badge)
		if badgeActualW < badgeFieldW {
			badge += strings.Repeat(" ", badgeFieldW-badgeActualW)
		}

		// GPU model
		gpuDisplay := w.GPUModel
		if gpuDisplay == "" {
			if w.GPUCount > 0 {
				gpuDisplay = fmt.Sprintf("%dx GPU", w.GPUCount)
			} else {
				gpuDisplay = "no GPU"
			}
		}
		if lsDispWidth(gpuDisplay) > gpuModelColW {
			gpuDisplay = lsTruncateToWidth(gpuDisplay, gpuModelColW-1) + "…"
		}
		gpuModelPadded := lsPadToWidth(gpuDisplay, gpuModelColW)

		// GPU utilization bar
		gpuUtil := 0
		if w.Status != "offline" {
			gpuUtil = workerGPUUtil(w)
		}
		gpuBar := renderGPUBar(gpuUtil, gpuBarW)

		// VRAM
		vramStr := ""
		if w.TotalVRAM > 0 {
			vramStr = fmt.Sprintf("%.0f/%.0fGB", w.FreeVRAM, w.TotalVRAM)
		} else if w.GPUCount > 0 {
			vramStr = "n/a"
		}
		vramPadded := lsPadToWidth(vramStr, vramColW)

		// Relay indicator
		relaySym := ""
		if m.relayURL != "" {
			relaySym = relaySymbol(m.relayConnected[w.ID]) + " "
		}

		// Last seen (for offline workers)
		lastSeen := ""
		if w.Status == "offline" {
			ls := lastSeenStr(w)
			if ls != "" {
				lastSeen = styleLastSeen.Render(lsPadToWidth(ls, lastSeenW))
			}
		} else {
			lastSeen = strings.Repeat(" ", lastSeenW)
		}

		if isSelected {
			sb.WriteString(styleSelected.Render(cursor))
			sb.WriteString(sym + " ")
			sb.WriteString(relaySym)
			sb.WriteString(styleSelected.Render(hostStr + " "))
			sb.WriteString(badge)
			sb.WriteString(styleSelected.Render(" " + gpuModelPadded + " "))
			sb.WriteString(gpuBar)
			sb.WriteString(styleSelected.Render(" " + vramPadded + " "))
			sb.WriteString(lastSeen)
		} else {
			sb.WriteString(cursor)
			sb.WriteString(sym + " ")
			sb.WriteString(relaySym)
			sb.WriteString(styleTagName.Render(hostStr))
			sb.WriteString(" ")
			sb.WriteString(badge)
			sb.WriteString(" ")
			sb.WriteString(styleSummary.Render(gpuModelPadded))
			sb.WriteString(" ")
			sb.WriteString(gpuBar)
			sb.WriteString(" ")
			sb.WriteString(styleDate.Render(vramPadded))
			sb.WriteString(" ")
			sb.WriteString(lastSeen)
		}
		sb.WriteString("\n")
	}
}

func (m workersTUIModel) renderSeparator(sb *strings.Builder) {
	if m.width > 0 {
		sb.WriteString(styleFaint.Render(strings.Repeat("─", m.width)))
	} else {
		sb.WriteString(styleFaint.Render(strings.Repeat("─", 74)))
	}
}

// runWorkersTUI launches the interactive Bubble Tea TUI for workers.
func runWorkersTUI(client *hub.Client, relayURL string) error {
	m := newWorkersTUIModel(client, relayURL)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
