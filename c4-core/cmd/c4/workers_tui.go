package main

import (
	"fmt"
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

// Messages for the workers TUI event loop.
type workersUpdatedMsg struct {
	workers []hub.Worker
	err     error
}

type workerTickMsg struct{}

// workersTUIModel is the bubbletea model for the workers TUI.
type workersTUIModel struct {
	workers   []hub.Worker
	cursor    int
	query     string
	width     int
	height    int
	loading   bool
	err       error
	tickCount int

	hubClient *hub.Client
	relayURL  string
}

func newWorkersTUIModel(client *hub.Client, relayURL string) workersTUIModel {
	return workersTUIModel{
		loading:   true,
		hubClient: client,
		relayURL:  relayURL,
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

func (m workersTUIModel) Init() tea.Cmd {
	return tea.Batch(
		fetchWorkersCmd(m.hubClient),
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

	case workerTickMsg:
		m.tickCount++
		// Refresh every tick (3s interval)
		return m, tea.Batch(
			fetchWorkersCmd(m.hubClient),
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
				return m, tea.Batch(fetchWorkersCmd(m.hubClient), workerTickCmd())
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

// renderWorkerRows writes worker list rows into sb.
func (m *workersTUIModel) renderWorkerRows(sb *strings.Builder, visible []hub.Worker) {
	// Layout: cursor(3) + hostname(20) + sp(1) + badge(8) + sp(1) + gpu(dynamic) + sp + vram(10)
	const hostColW = 20
	const badgeFieldW = 8 // "online" = 6 + 2 padding
	const vramColW = 12

	fixedW := 3 + hostColW + 1 + badgeFieldW + 1 + 1 + vramColW + 1
	gpuColW := m.width - fixedW
	if gpuColW < 12 {
		gpuColW = 12
	}
	if gpuColW > 32 {
		gpuColW = 32
	}

	c := m.cursor
	if c < 0 {
		c = 0
	}
	if c >= len(visible) {
		c = len(visible) - 1
	}

	for i, w := range visible {
		isSelected := i == c

		cursor := "   "
		if isSelected {
			cursor = " ▸ "
		}

		// Hostname column
		hostname := w.Hostname
		if hostname == "" {
			hostname = w.ID
		}
		hostPadded := lsPadToWidth(hostname, hostColW)

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
		// Pad badge field to fixed width
		badgeActualW := lipgloss.Width(badge)
		if badgeActualW < badgeFieldW {
			badge += strings.Repeat(" ", badgeFieldW-badgeActualW)
		}

		// GPU model column
		gpuDisplay := w.GPUModel
		if gpuDisplay == "" {
			if w.GPUCount > 0 {
				gpuDisplay = fmt.Sprintf("%d GPU(s)", w.GPUCount)
			} else {
				gpuDisplay = "no GPU"
			}
		}
		if lsDispWidth(gpuDisplay) > gpuColW {
			gpuDisplay = lsTruncateToWidth(gpuDisplay, gpuColW-1) + "…"
		}
		gpuPadded := lsPadToWidth(gpuDisplay, gpuColW)

		// VRAM column: free/total
		vramStr := ""
		if w.TotalVRAM > 0 {
			vramStr = fmt.Sprintf("%.0f/%.0fGB", w.FreeVRAM, w.TotalVRAM)
		} else if w.GPUCount > 0 {
			vramStr = "n/a"
		} else {
			vramStr = ""
		}
		vramPadded := lsPadToWidth(vramStr, vramColW)

		if isSelected {
			sb.WriteString(styleSelected.Render(cursor))
			sb.WriteString(styleSelected.Render(hostPadded))
			sb.WriteString(styleSelected.Render(" "))
			sb.WriteString(badge)
			sb.WriteString(styleSelected.Render(" "))
			sb.WriteString(styleSelected.Render(gpuPadded))
			sb.WriteString(styleSelected.Render(" "))
			sb.WriteString(styleSelected.Render(vramPadded))
			// Trailing fill
			leftUsed := 3 + hostColW + 1 + badgeFieldW + 1 + gpuColW + 1 + vramColW
			trailing := m.width - leftUsed
			if trailing > 0 {
				sb.WriteString(styleSelected.Render(strings.Repeat(" ", trailing)))
			}
		} else {
			sb.WriteString(cursor)
			sb.WriteString(styleTagName.Render(hostPadded))
			sb.WriteString(" ")
			sb.WriteString(badge)
			sb.WriteString(" ")
			sb.WriteString(styleSummary.Render(gpuPadded))
			sb.WriteString(" ")
			sb.WriteString(styleDate.Render(vramPadded))
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
