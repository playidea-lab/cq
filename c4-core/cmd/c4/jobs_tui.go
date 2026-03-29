package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/changmin/c4-core/internal/hub"
)

// =========================================================================
// Cobra command registration
// =========================================================================

var (
	jobsNoTUI      bool
	jobsJSON       bool
	jobsStatusFlag string
	jobsLimit      int
)

var jobsRootCmd = &cobra.Command{
	Use:   "jobs",
	Short: "List and monitor Hub jobs",
	Long:  "Interactive TUI dashboard for monitoring Hub jobs. Same as 'cq hub jobs'.",
	RunE:  runJobs,
}

var hubJobsCmd = &cobra.Command{
	Use:   "jobs",
	Short: "List and monitor Hub jobs",
	RunE:  runJobs,
}

func init() {
	for _, cmd := range []*cobra.Command{jobsRootCmd, hubJobsCmd} {
		cmd.Flags().BoolVar(&jobsNoTUI, "no-tui", false, "disable interactive TUI, print table instead")
		cmd.Flags().BoolVar(&jobsJSON, "json", false, "output jobs as JSON")
		cmd.Flags().StringVar(&jobsStatusFlag, "status", "", "filter by status (QUEUED, RUNNING, SUCCEEDED, FAILED, CANCELLED)")
		cmd.Flags().IntVar(&jobsLimit, "limit", 50, "max number of jobs to display")
	}

	hubCmd.AddCommand(hubJobsCmd)
	rootCmd.AddCommand(jobsRootCmd)
}

func runJobs(cmd *cobra.Command, args []string) error {
	client, err := newHubClient()
	if err != nil {
		return err
	}

	// JSON output mode.
	if jobsJSON {
		jobs, err := client.ListJobs(jobsStatusFlag, jobsLimit)
		if err != nil {
			return fmt.Errorf("list jobs: %w", err)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(jobs)
	}

	// TUI mode (default unless --no-tui).
	if !jobsNoTUI {
		return runJobsTUI(client)
	}

	// Table output mode (--no-tui).
	jobs, err := client.ListJobs(jobsStatusFlag, jobsLimit)
	if err != nil {
		return fmt.Errorf("list jobs: %w", err)
	}

	if len(jobs) == 0 {
		fmt.Println("No jobs found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "ID\tSTATUS\tNAME\tWORKER\tBEST METRIC\tCREATED\n")
	for _, j := range jobs {
		id := j.GetID()
		name := j.Name
		if name == "" {
			name = "-"
		}
		worker := j.WorkerID
		if worker == "" {
			worker = "-"
		}
		best := "-"
		if j.BestMetric != nil {
			best = fmt.Sprintf("%.4f", *j.BestMetric)
			if j.PrimaryMetric != "" {
				best = j.PrimaryMetric + "=" + best
			}
		}
		created := formatLastJob(j.CreatedAt)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", id, j.Status, name, worker, best, created)
	}
	w.Flush()
	return nil
}

// =========================================================================
// Bubbletea TUI
// =========================================================================

// Job status badge styles.
var jobStatusBadgeStyles = map[string]lipgloss.Style{
	"QUEUED":    lipgloss.NewStyle().Background(lipgloss.Color("4")).Foreground(lipgloss.Color("15")).Bold(true).Padding(0, 1),
	"RUNNING":   lipgloss.NewStyle().Background(lipgloss.Color("3")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1),
	"SUCCEEDED": lipgloss.NewStyle().Background(lipgloss.Color("2")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1),
	"FAILED":    lipgloss.NewStyle().Background(lipgloss.Color("1")).Foreground(lipgloss.Color("15")).Bold(true).Padding(0, 1),
	"CANCELLED": lipgloss.NewStyle().Background(lipgloss.Color("237")).Foreground(lipgloss.Color("245")).Padding(0, 1),
}

// Status filter options for cycling through with Tab.
var jobStatusFilters = []string{"", "QUEUED", "RUNNING", "SUCCEEDED", "FAILED", "CANCELLED"}

// Messages for the jobs TUI event loop.
type jobsUpdatedMsg struct {
	jobs []hub.Job
	err  error
}

type jobsTickMsg struct{}

type jobCancelledMsg struct {
	jobID string
	err   error
}

// jobsTUIModel is the bubbletea model for the jobs TUI.
type jobsTUIModel struct {
	jobs         []hub.Job
	cursor       int
	query        string
	searchMode   bool
	statusFilter int // index into jobStatusFilters
	width        int
	height       int
	loading      bool
	err          error
	tickCount    int
	hubClient    *hub.Client
	confirmKill  bool   // true when showing kill confirmation prompt
	killTargetID string // job ID pending kill confirmation
}

func newJobsTUIModel(client *hub.Client) jobsTUIModel {
	// Apply CLI --status filter if set.
	filterIdx := 0
	if jobsStatusFlag != "" {
		upper := strings.ToUpper(jobsStatusFlag)
		for i, s := range jobStatusFilters {
			if s == upper {
				filterIdx = i
				break
			}
		}
	}
	return jobsTUIModel{
		loading:      true,
		hubClient:    client,
		statusFilter: filterIdx,
	}
}

func fetchJobsCmd(client *hub.Client, status string, limit int) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return jobsUpdatedMsg{err: fmt.Errorf("hub client not configured")}
		}
		jobs, err := client.ListJobs(status, limit)
		return jobsUpdatedMsg{jobs: jobs, err: err}
	}
}

func jobsTickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		return jobsTickMsg{}
	})
}

func cancelJobCmd(client *hub.Client, jobID string) tea.Cmd {
	return func() tea.Msg {
		err := client.CancelJob(jobID)
		return jobCancelledMsg{jobID: jobID, err: err}
	}
}

func (m jobsTUIModel) currentStatusFilter() string {
	return jobStatusFilters[m.statusFilter]
}

func (m jobsTUIModel) Init() tea.Cmd {
	return tea.Batch(
		fetchJobsCmd(m.hubClient, m.currentStatusFilter(), jobsLimit),
		jobsTickCmd(),
	)
}

func (m jobsTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case jobsUpdatedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.jobs = msg.jobs
		}
		return m, nil

	case jobCancelledMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			// Refresh after cancel.
			return m, fetchJobsCmd(m.hubClient, m.currentStatusFilter(), jobsLimit)
		}
		return m, nil

	case jobsTickMsg:
		m.tickCount++
		return m, tea.Batch(
			fetchJobsCmd(m.hubClient, m.currentStatusFilter(), jobsLimit),
			jobsTickCmd(),
		)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Kill confirmation mode input handling.
		if m.confirmKill {
			switch msg.Type {
			case tea.KeyEsc:
				m.confirmKill = false
				m.killTargetID = ""
				return m, nil
			case tea.KeyRunes:
				ch := msg.String()
				switch ch {
				case "y", "Y":
					jobID := m.killTargetID
					m.confirmKill = false
					m.killTargetID = ""
					return m, cancelJobCmd(m.hubClient, jobID)
				case "n", "N":
					m.confirmKill = false
					m.killTargetID = ""
					return m, nil
				}
			}
			return m, nil
		}

		// Search mode input handling.
		if m.searchMode {
			switch msg.Type {
			case tea.KeyEsc:
				m.searchMode = false
				m.query = ""
				m.cursor = 0
				return m, nil
			case tea.KeyEnter:
				m.searchMode = false
				return m, nil
			case tea.KeyBackspace:
				if len(m.query) > 0 {
					runes := []rune(m.query)
					m.query = string(runes[:len(runes)-1])
					m.cursor = 0
				}
				return m, nil
			case tea.KeyRunes:
				m.query += msg.String()
				m.cursor = 0
				return m, nil
			}
			return m, nil
		}

		switch msg.Type {
		case tea.KeyTab:
			// Cycle status filter.
			m.statusFilter = (m.statusFilter + 1) % len(jobStatusFilters)
			m.cursor = 0
			m.loading = true
			return m, fetchJobsCmd(m.hubClient, m.currentStatusFilter(), jobsLimit)

		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			visible := m.visibleJobs()
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
			case "/":
				m.searchMode = true
				return m, nil
			case "k":
				// Kill selected job (only if QUEUED or RUNNING).
				visible := m.visibleJobs()
				if m.cursor >= 0 && m.cursor < len(visible) {
					j := visible[m.cursor]
					if j.Status == "QUEUED" || j.Status == "RUNNING" {
						m.confirmKill = true
						m.killTargetID = j.GetID()
					}
				}
				return m, nil
			case "j":
				visible := m.visibleJobs()
				if m.cursor < len(visible)-1 {
					m.cursor++
				}
				return m, nil
			case "q":
				return m, tea.Quit
			case "r":
				m.loading = true
				return m, tea.Batch(fetchJobsCmd(m.hubClient, m.currentStatusFilter(), jobsLimit), jobsTickCmd())
			case "x":
				// Alias for kill — same as 'k'.
				visible := m.visibleJobs()
				if m.cursor >= 0 && m.cursor < len(visible) {
					j := visible[m.cursor]
					if j.Status == "QUEUED" || j.Status == "RUNNING" {
						m.confirmKill = true
						m.killTargetID = j.GetID()
					}
				}
				return m, nil
			}
		}
	}
	return m, nil
}

// visibleJobs returns jobs matching the current search query.
func (m *jobsTUIModel) visibleJobs() []hub.Job {
	if m.query == "" {
		return m.jobs
	}
	lower := strings.ToLower(m.query)
	tokens := strings.Fields(lower)
	var out []hub.Job
	for _, j := range m.jobs {
		corpus := strings.ToLower(j.GetID() + " " + j.Name + " " + j.Status + " " + j.WorkerID + " " + j.ExpID + " " + j.Command)
		match := true
		for _, tok := range tokens {
			if !strings.Contains(corpus, tok) {
				match = false
				break
			}
		}
		if match {
			out = append(out, j)
		}
	}
	return out
}

// jobStatusSymbol returns a colored symbol for job status.
func jobStatusSymbol(status string) string {
	switch status {
	case "QUEUED":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Render("◆")
	case "RUNNING":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true).Render("▸")
	case "SUCCEEDED":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("✓")
	case "FAILED":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true).Render("✗")
	case "CANCELLED":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("○")
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("?")
	}
}

// formatJobDuration returns a human-readable duration between start and finish.
func formatJobDuration(startedAt, finishedAt string) string {
	if startedAt == "" {
		return "-"
	}
	start, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return "-"
	}
	var end time.Time
	if finishedAt != "" {
		end, err = time.Parse(time.RFC3339, finishedAt)
		if err != nil {
			end = time.Now()
		}
	} else {
		end = time.Now()
	}
	d := end.Sub(start)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

// formatBestMetric returns a formatted best metric string.
func formatBestMetric(j hub.Job) string {
	if j.BestMetric == nil {
		return "-"
	}
	dir := "↑"
	if j.LowerIsBetter != nil && *j.LowerIsBetter {
		dir = "↓"
	}
	val := fmt.Sprintf("%.4f", *j.BestMetric)
	if j.PrimaryMetric != "" {
		return fmt.Sprintf("%s=%s%s", j.PrimaryMetric, val, dir)
	}
	return val + dir
}

func (m jobsTUIModel) View() string {
	var sb strings.Builder

	// Title bar.
	sb.WriteString(styleTitle.Render(" cq jobs "))
	sb.WriteString(" ")
	if m.loading {
		sb.WriteString(styleCount.Render("loading..."))
	} else if m.err != nil {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(m.err.Error()))
	} else {
		visible := m.visibleJobs()
		sb.WriteString(styleCount.Render(fmt.Sprintf("%d jobs", len(visible))))
	}

	// Status filter indicator.
	sb.WriteString("  ")
	for i, f := range jobStatusFilters {
		label := "ALL"
		if f != "" {
			label = f
		}
		if i == m.statusFilter {
			sb.WriteString(lipgloss.NewStyle().Background(lipgloss.Color("14")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1).Render(label))
		} else {
			sb.WriteString(lipgloss.NewStyle().Faint(true).Render(" "+label+" "))
		}
	}
	sb.WriteString("\n")

	// Search bar.
	if m.searchMode {
		sb.WriteString("  ")
		sb.WriteString(styleSearchBar.Render(fmt.Sprintf(" / %s▏ ", m.query)))
	} else if m.query != "" {
		sb.WriteString("  ")
		sb.WriteString(styleSearchBar.Render(fmt.Sprintf(" / %s ", m.query)))
	} else {
		sb.WriteString("  ")
		sb.WriteString(styleSearchPlaceholder.Render(" / to search... "))
	}
	sb.WriteString("\n\n")

	if m.loading {
		spinner := spinnerFrames[m.tickCount%len(spinnerFrames)]
		sb.WriteString(fmt.Sprintf("  %s fetching jobs...\n", spinner))
	} else if m.err != nil {
		sb.WriteString(fmt.Sprintf("  error: %s\n", m.err.Error()))
	} else {
		visible := m.visibleJobs()
		if len(visible) == 0 {
			sb.WriteString(styleFaint.Render("  No jobs found."))
			sb.WriteString("\n")
		} else {
			m.renderJobRows(&sb, visible)
		}
	}

	// Fill remaining space to pin help bar at bottom.
	content := sb.String()
	contentLines := strings.Count(content, "\n")
	if m.height > 0 {
		gap := m.height - contentLines - 2
		for i := 0; i < gap; i++ {
			sb.WriteString("\n")
		}
	}

	// Separator + help bar.
	if m.width > 0 {
		sb.WriteString(styleFaint.Render(strings.Repeat("─", m.width)))
	} else {
		sb.WriteString(styleFaint.Render(strings.Repeat("─", 74)))
	}
	sb.WriteString("\n")

	var helpBar strings.Builder
	helpBar.WriteString(" ")
	if m.confirmKill {
		killIDDisplay := m.killTargetID
		if len(killIDDisplay) > 12 {
			killIDDisplay = killIDDisplay[len(killIDDisplay)-12:]
		}
		helpBar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true).Render(
			fmt.Sprintf("Kill %s? [y/N]", killIDDisplay)))
	} else if m.searchMode {
		helpBar.WriteString(helpEntry("Enter", "confirm"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("Esc", "cancel"))
	} else {
		helpBar.WriteString(helpEntry("↑↓/j", "navigate"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("Tab", "filter"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("/", "search"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("k", "kill job"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("r", "refresh"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("q", "quit"))
	}
	sb.WriteString(helpBar.String())

	return sb.String()
}

// renderJobRows writes job list rows into sb.
func (m *jobsTUIModel) renderJobRows(sb *strings.Builder, visible []hub.Job) {
	c := m.cursor
	if c < 0 {
		c = 0
	}
	if c >= len(visible) {
		c = len(visible) - 1
	}

	// Determine how many rows we can show (reserve 8 lines for header/footer).
	maxRows := len(visible)
	if m.height > 0 {
		available := m.height - 8
		if available < 5 {
			available = 5
		}
		if maxRows > available {
			maxRows = available
		}
	}

	// Scroll window around cursor.
	start := 0
	if c >= maxRows {
		start = c - maxRows + 1
	}
	end := start + maxRows
	if end > len(visible) {
		end = len(visible)
	}

	for i := start; i < end; i++ {
		j := visible[i]
		isSelected := i == c
		m.renderJobRow(sb, j, isSelected)
	}

	// Scroll indicator.
	if len(visible) > maxRows {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("  ... %d more jobs (scroll to see)\n", len(visible)-maxRows)))
	}
}

// renderJobRow renders a single job row.
func (m *jobsTUIModel) renderJobRow(sb *strings.Builder, j hub.Job, isSelected bool) {
	cursor := "   "
	if isSelected {
		cursor = " ▸ "
	}

	sym := jobStatusSymbol(j.Status)
	id := j.GetID()

	// Truncate ID to last 8 chars for display.
	idDisplay := id
	if len(idDisplay) > 12 {
		idDisplay = "…" + idDisplay[len(idDisplay)-11:]
	}

	name := j.Name
	if name == "" {
		name = j.Command
	}
	if name == "" {
		name = "-"
	}

	// Adapt column widths to terminal width.
	wide := m.width >= 100

	if wide {
		// Full layout: cursor(3) + sym(2) + id(13) + status badge(12) + name(20) + worker(14) + metric(20) + duration(10) + created(10)
		const idColW = 13
		const badgeFieldW = 12
		const nameColW = 20
		const workerColW = 14
		const metricColW = 20
		const durationColW = 10
		const createdColW = 10

		idPadded := lsPadToWidth(idDisplay, idColW)

		// Status badge.
		badge := j.Status
		if bs, ok := jobStatusBadgeStyles[j.Status]; ok {
			badge = bs.Render(j.Status)
		}
		badgeW := lipgloss.Width(badge)
		if badgeW < badgeFieldW {
			badge += strings.Repeat(" ", badgeFieldW-badgeW)
		}

		nameTrunc := name
		if lsDispWidth(nameTrunc) > nameColW {
			nameTrunc = lsTruncateToWidth(nameTrunc, nameColW-1) + "…"
		}
		namePadded := lsPadToWidth(nameTrunc, nameColW)

		worker := j.WorkerID
		if worker == "" {
			worker = "-"
		}
		if lsDispWidth(worker) > workerColW {
			worker = lsTruncateToWidth(worker, workerColW-1) + "…"
		}
		workerPadded := lsPadToWidth(worker, workerColW)

		metric := formatBestMetric(j)
		if lsDispWidth(metric) > metricColW {
			metric = lsTruncateToWidth(metric, metricColW-1) + "…"
		}
		metricPadded := lsPadToWidth(metric, metricColW)

		duration := formatJobDuration(j.StartedAt, j.FinishedAt)
		durationPadded := lsPadToWidth(duration, durationColW)

		created := formatLastJob(j.CreatedAt)
		createdPadded := lsPadToWidth(created, createdColW)

		if isSelected {
			sb.WriteString(styleSelected.Render(cursor))
			sb.WriteString(sym + " ")
			sb.WriteString(styleSelected.Render(idPadded + " "))
			sb.WriteString(badge + " ")
			sb.WriteString(styleSelected.Render(namePadded + " "))
			sb.WriteString(styleDate.Render(workerPadded) + " ")
			sb.WriteString(styleTagName.Render(metricPadded) + " ")
			sb.WriteString(styleDate.Render(durationPadded) + " ")
			sb.WriteString(styleDate.Render(createdPadded))
		} else {
			sb.WriteString(cursor)
			sb.WriteString(sym + " ")
			sb.WriteString(styleFaint.Render(idPadded) + " ")
			sb.WriteString(badge + " ")
			sb.WriteString(styleTagName.Render(namePadded) + " ")
			sb.WriteString(styleDate.Render(workerPadded) + " ")
			sb.WriteString(styleTagName.Render(metricPadded) + " ")
			sb.WriteString(styleDate.Render(durationPadded) + " ")
			sb.WriteString(styleDate.Render(createdPadded))
		}
	} else {
		// Compact layout: cursor(3) + sym(2) + id(13) + name(20) + status(12)
		const idColW = 13
		const nameColW = 20
		const statusColW = 12

		idPadded := lsPadToWidth(idDisplay, idColW)

		nameTrunc := name
		if lsDispWidth(nameTrunc) > nameColW {
			nameTrunc = lsTruncateToWidth(nameTrunc, nameColW-1) + "…"
		}
		namePadded := lsPadToWidth(nameTrunc, nameColW)

		badge := j.Status
		if bs, ok := jobStatusBadgeStyles[j.Status]; ok {
			badge = bs.Render(j.Status)
		}

		if isSelected {
			sb.WriteString(styleSelected.Render(cursor))
			sb.WriteString(sym + " ")
			sb.WriteString(styleSelected.Render(idPadded + " " + namePadded + " "))
			sb.WriteString(badge)
		} else {
			sb.WriteString(cursor)
			sb.WriteString(sym + " ")
			sb.WriteString(styleFaint.Render(idPadded) + " ")
			sb.WriteString(styleTagName.Render(namePadded) + " ")
			sb.WriteString(badge)
		}
	}

	sb.WriteString("\n")
}

// =========================================================================
// TUI entry point
// =========================================================================

func runJobsTUI(client *hub.Client) error {
	m := newJobsTUIModel(client)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
