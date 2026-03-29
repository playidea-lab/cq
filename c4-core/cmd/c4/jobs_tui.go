package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
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

type jobMetricsFetchedMsg struct {
	jobID   string
	metrics []hub.MetricEntry
	total   int
	err     error
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

	// Detail mode: show all metrics for the selected job.
	detailMode    bool
	detailJobID   string
	detailMetrics map[string][]float64 // metric name -> values by step
	detailBest    map[string]float64   // metric name -> best value
	detailLoading bool
	detailErr     error
	detailScroll  int // scroll offset for detail panel

	// Compare mode: overlay two jobs' primary_metric trajectories.
	compareMode      bool
	compareSelecting bool   // true when user is picking second job
	compareJobA      string // first job ID (the one selected when 'c' was pressed)
	compareJobB      string // second job ID
	compareCursor    int    // cursor for selecting second job
	compareMetricName string // primary_metric name (must match)
	compareDataA     []float64
	compareDataB     []float64
	compareLoading   bool
	compareErr       error
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

func fetchJobMetricsCmd(client *hub.Client, jobID string) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.GetMetrics(jobID, 500)
		if err != nil {
			return jobMetricsFetchedMsg{jobID: jobID, err: err}
		}
		return jobMetricsFetchedMsg{
			jobID:   jobID,
			metrics: resp.Metrics,
			total:   resp.TotalSteps,
		}
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

	case jobMetricsFetchedMsg:
		if msg.err != nil {
			if m.detailLoading && msg.jobID == m.detailJobID {
				m.detailLoading = false
				m.detailErr = msg.err
			}
			if m.compareLoading {
				m.compareLoading = false
				m.compareErr = msg.err
			}
			return m, nil
		}
		// Parse metric entries into per-metric float64 slices.
		data, best := parseMetricEntries(msg.metrics)

		if m.detailLoading && msg.jobID == m.detailJobID {
			m.detailLoading = false
			m.detailMetrics = data
			m.detailBest = best
			return m, nil
		}
		if m.compareLoading {
			if msg.jobID == m.compareJobA && m.compareDataA == nil {
				if vals, ok := data[m.compareMetricName]; ok {
					m.compareDataA = vals
				}
			}
			if msg.jobID == m.compareJobB && m.compareDataB == nil {
				if vals, ok := data[m.compareMetricName]; ok {
					m.compareDataB = vals
				}
			}
			// Check if both loaded.
			if m.compareDataA != nil && m.compareDataB != nil {
				m.compareLoading = false
			}
			return m, nil
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

		// Compare selection mode: user picking second job.
		if m.compareSelecting {
			return m.updateCompareSelecting(msg)
		}

		// Detail or compare view: Esc exits, j/k scrolls detail.
		if m.detailMode || m.compareMode {
			if msg.Type == tea.KeyEsc {
				m.detailMode = false
				m.compareMode = false
				m.compareSelecting = false
				return m, nil
			}
			if msg.Type == tea.KeyEnter && m.detailMode {
				m.detailMode = false
				return m, nil
			}
			if m.detailMode {
				switch msg.String() {
				case "j", "down":
					m.detailScroll++
				case "k", "up":
					if m.detailScroll > 0 {
						m.detailScroll--
					}
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
		case tea.KeyEnter:
			// Toggle detail panel for selected job.
			visible := m.visibleJobs()
			if m.cursor >= 0 && m.cursor < len(visible) {
				j := visible[m.cursor]
				jobID := j.GetID()
				if m.detailMode && m.detailJobID == jobID {
					m.detailMode = false
				} else {
					m.detailMode = true
					m.detailJobID = jobID
					m.detailLoading = true
					m.detailErr = nil
					m.detailMetrics = nil
					m.detailBest = nil
					m.detailScroll = 0
					return m, fetchJobMetricsCmd(m.hubClient, jobID)
				}
			}
			return m, nil
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
				// Cancel selected job (only if QUEUED or RUNNING).
				visible := m.visibleJobs()
				if m.cursor >= 0 && m.cursor < len(visible) {
					j := visible[m.cursor]
					if j.Status == "QUEUED" || j.Status == "RUNNING" {
						return m, cancelJobCmd(m.hubClient, j.GetID())
					}
				}
				return m, nil
			case "c":
				// Enter compare mode: select current job as A, then pick B.
				visible := m.visibleJobs()
				if len(visible) < 2 {
					return m, nil
				}
				if m.cursor >= 0 && m.cursor < len(visible) {
					j := visible[m.cursor]
					m.compareJobA = j.GetID()
					m.compareMetricName = j.PrimaryMetric
					m.compareSelecting = true
					m.compareCursor = 0
					if m.compareCursor == m.cursor && len(visible) > 1 {
						m.compareCursor = 1
					}
				}
				return m, nil
			}
		}
	}
	return m, nil
}

// updateCompareSelecting handles key input while selecting the second job for compare.
func (m jobsTUIModel) updateCompareSelecting(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	visible := m.visibleJobs()
	switch msg.Type {
	case tea.KeyEsc:
		m.compareSelecting = false
		m.compareJobA = ""
		return m, nil
	case tea.KeyUp:
		if m.compareCursor > 0 {
			m.compareCursor--
		}
	case tea.KeyDown:
		if m.compareCursor < len(visible)-1 {
			m.compareCursor++
		}
	case tea.KeyRunes:
		ch := msg.String()
		if ch == "j" && m.compareCursor < len(visible)-1 {
			m.compareCursor++
		} else if ch == "k" && m.compareCursor > 0 {
			m.compareCursor--
		}
	case tea.KeyEnter:
		if m.compareCursor >= 0 && m.compareCursor < len(visible) {
			jB := visible[m.compareCursor]
			if jB.GetID() == m.compareJobA {
				// Same job selected — ignore.
				return m, nil
			}
			// Check primary_metric compatibility.
			if jB.PrimaryMetric != m.compareMetricName {
				m.compareSelecting = false
				m.compareMode = true
				m.compareJobB = jB.GetID()
				m.compareErr = fmt.Errorf("비교 불가: primary_metric이 다릅니다 (%s vs %s)", m.compareMetricName, jB.PrimaryMetric)
				return m, nil
			}
			if m.compareMetricName == "" {
				m.compareSelecting = false
				m.compareMode = true
				m.compareJobB = jB.GetID()
				m.compareErr = fmt.Errorf("비교 불가: primary_metric이 설정되지 않았습니다")
				return m, nil
			}
			m.compareSelecting = false
			m.compareMode = true
			m.compareJobB = jB.GetID()
			m.compareLoading = true
			m.compareErr = nil
			m.compareDataA = nil
			m.compareDataB = nil
			return m, tea.Batch(
				fetchJobMetricsCmd(m.hubClient, m.compareJobA),
				fetchJobMetricsCmd(m.hubClient, m.compareJobB),
			)
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

// parseMetricEntries converts MetricEntry slices into per-metric float64 slices and best values.
func parseMetricEntries(entries []hub.MetricEntry) (data map[string][]float64, best map[string]float64) {
	data = make(map[string][]float64)
	best = make(map[string]float64)

	// Sort by step to guarantee order.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Step < entries[j].Step
	})

	for _, e := range entries {
		for k, v := range e.Metrics {
			var fv float64
			switch val := v.(type) {
			case float64:
				fv = val
			case json.Number:
				fv, _ = val.Float64()
			default:
				continue
			}
			data[k] = append(data[k], fv)

			if _, ok := best[k]; !ok {
				best[k] = fv
			} else {
				if metricLowerIsBetter(k) {
					if fv < best[k] {
						best[k] = fv
					}
				} else {
					if fv > best[k] {
						best[k] = fv
					}
				}
			}
		}
	}
	return data, best
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

	// Mode indicator.
	if m.compareSelecting {
		sb.WriteString("  ")
		sb.WriteString(lipgloss.NewStyle().Background(lipgloss.Color("5")).Foreground(lipgloss.Color("15")).Bold(true).Padding(0, 1).Render("COMPARE: select 2nd job"))
	} else if m.compareMode {
		sb.WriteString("  ")
		sb.WriteString(lipgloss.NewStyle().Background(lipgloss.Color("5")).Foreground(lipgloss.Color("15")).Bold(true).Padding(0, 1).Render("COMPARE"))
	} else if m.detailMode {
		sb.WriteString("  ")
		sb.WriteString(lipgloss.NewStyle().Background(lipgloss.Color("12")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1).Render("DETAIL"))
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
			if m.compareSelecting {
				m.renderJobRowsCompareSelect(&sb, visible)
			} else {
				m.renderJobRows(&sb, visible)
			}
		}
	}

	// Detail panel below the list.
	if m.detailMode && !m.compareSelecting {
		m.renderDetailPanel(&sb)
	}

	// Compare panel below the list.
	if m.compareMode && !m.compareSelecting {
		m.renderComparePanel(&sb)
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
	if m.searchMode {
		helpBar.WriteString(helpEntry("Enter", "confirm"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("Esc", "cancel"))
	} else if m.compareSelecting {
		helpBar.WriteString(helpEntry("↑↓/jk", "navigate"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("Enter", "select"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("Esc", "cancel"))
	} else if m.detailMode || m.compareMode {
		if m.detailMode {
			helpBar.WriteString(helpEntry("↑↓/jk", "scroll"))
			helpBar.WriteString("  ")
		}
		helpBar.WriteString(helpEntry("Esc", "close"))
		if m.detailMode {
			helpBar.WriteString("  ")
			helpBar.WriteString(helpEntry("Enter", "close"))
		}
	} else {
		helpBar.WriteString(helpEntry("↑↓/jk", "navigate"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("Enter", "detail"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("c", "compare"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("Tab", "filter"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("/", "search"))
		helpBar.WriteString("  ")
		helpBar.WriteString(helpEntry("x", "cancel job"))
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

	// Reserve extra lines for detail/compare panels.
	reserveLines := 8
	if m.detailMode {
		// Detail takes 2/3 of screen — reserve heavily so job list shrinks.
		reserveLines = m.height * 2 / 3
		if reserveLines < 16 {
			reserveLines = 16
		}
	}
	if m.compareMode {
		reserveLines += 14
	}
	maxRows := len(visible)
	if m.height > 0 {
		available := m.height - reserveLines
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

// renderJobRowsCompareSelect renders job list with compare selection cursor.
func (m *jobsTUIModel) renderJobRowsCompareSelect(sb *strings.Builder, visible []hub.Job) {
	c := m.compareCursor
	if c < 0 {
		c = 0
	}
	if c >= len(visible) {
		c = len(visible) - 1
	}

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

	start := 0
	if c >= maxRows {
		start = c - maxRows + 1
	}
	end := start + maxRows
	if end > len(visible) {
		end = len(visible)
	}

	compareAStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)

	for i := start; i < end; i++ {
		j := visible[i]
		isJobA := j.GetID() == m.compareJobA
		isCursorHere := i == c

		if isJobA {
			sb.WriteString(compareAStyle.Render(" A "))
		} else if isCursorHere {
			sb.WriteString(" ▸ ")
		} else {
			sb.WriteString("   ")
		}

		sym := jobStatusSymbol(j.Status)
		id := j.GetID()
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
		nameTrunc := name
		if lsDispWidth(nameTrunc) > 20 {
			nameTrunc = lsTruncateToWidth(nameTrunc, 19) + "…"
		}
		metric := formatBestMetric(j)

		if isCursorHere {
			sb.WriteString(sym + " ")
			sb.WriteString(styleSelected.Render(lsPadToWidth(idDisplay, 13) + " " + lsPadToWidth(nameTrunc, 20) + " "))
			sb.WriteString(styleTagName.Render(metric))
		} else if isJobA {
			sb.WriteString(sym + " ")
			sb.WriteString(compareAStyle.Render(lsPadToWidth(idDisplay, 13) + " " + lsPadToWidth(nameTrunc, 20) + " "))
			sb.WriteString(styleTagName.Render(metric))
		} else {
			sb.WriteString(sym + " ")
			sb.WriteString(styleFaint.Render(lsPadToWidth(idDisplay, 13)) + " ")
			sb.WriteString(styleTagName.Render(lsPadToWidth(nameTrunc, 20)) + " ")
			sb.WriteString(styleFaint.Render(metric))
		}
		sb.WriteString("\n")
	}

	if len(visible) > maxRows {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("  ... %d more jobs\n", len(visible)-maxRows)))
	}
}

// renderDetailPanel renders the detail panel showing all metrics for the selected job.
func (m *jobsTUIModel) renderDetailPanel(sb *strings.Builder) {
	sb.WriteString("\n")
	borderW := m.width
	if borderW <= 0 {
		borderW = 74
	}
	sb.WriteString(styleFaint.Render(strings.Repeat("─", borderW)))
	sb.WriteString("\n")

	title := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true).Render("detail")
	idDisplay := m.detailJobID
	if len(idDisplay) > 16 {
		idDisplay = "…" + idDisplay[len(idDisplay)-15:]
	}
	sb.WriteString("  " + title + " " + styleFaint.Render(idDisplay) + "\n")

	if m.detailLoading {
		spinner := spinnerFrames[m.tickCount%len(spinnerFrames)]
		sb.WriteString(fmt.Sprintf("  %s loading metrics...\n", spinner))
		return
	}
	if m.detailErr != nil {
		sb.WriteString(fmt.Sprintf("  error: %s\n", m.detailErr.Error()))
		return
	}
	if len(m.detailMetrics) == 0 {
		sb.WriteString(styleFaint.Render("  No metrics recorded for this job."))
		sb.WriteString("\n")
		return
	}

	names := sortedMetricNames(m.detailMetrics)

	// Available height for detail panel.
	detailHeight := m.height*2/3 - 4
	if detailHeight < 8 {
		detailHeight = 8
	}

	// Chart width: nearly full terminal, minus small left margin and Y-axis labels.
	chartIndent := 4 // left margin
	yAxisW := 12     // right-side Y-axis labels
	sparkW := m.width - chartIndent - yAxisW
	if sparkW < 20 {
		sparkW = 20
	}

	// Apply scroll.
	if m.detailScroll >= len(names) {
		m.detailScroll = len(names) - 1
	}
	if m.detailScroll < 0 {
		m.detailScroll = 0
	}
	end := m.detailScroll + detailHeight
	if end > len(names) {
		end = len(names)
	}

	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)

	// Adaptive height per metric based on data points.
	// 1 pt → 1 row, 2-5 pts → 2 rows, 6-20 → 3 rows, 20+ → 4 rows
	chartHeight := func(pts int) int {
		switch {
		case pts <= 1:
			return 1
		case pts <= 5:
			return 2
		case pts <= 20:
			return 3
		default:
			return 4
		}
	}

	// Calculate how many metrics fit in available height.
	usedLines := 0
	end = m.detailScroll
	for end < len(names) && usedLines < detailHeight {
		pts := len(m.detailMetrics[names[end]])
		usedLines += chartHeight(pts) + 1 // chart + header
		end++
	}
	if end > len(names) {
		end = len(names)
	}

	for i := m.detailScroll; i < end; i++ {
		name := names[i]
		vals := m.detailMetrics[name]
		if len(vals) == 0 {
			continue
		}
		latest := vals[len(vals)-1]
		bestVal := m.detailBest[name]
		dir := "↑"
		if metricLowerIsBetter(name) {
			dir = "↓"
		}

		stats := fmt.Sprintf("latest: %.4f  %s best: %.4f  (%d pts)", latest, dir, bestVal, len(vals))
		sb.WriteString("  ")
		sb.WriteString(nameStyle.Render(name))
		sb.WriteString("  ")
		sb.WriteString(styleFaint.Render(stats))
		sb.WriteString("\n")

		h := chartHeight(len(vals))
		renderTallSparkline(sb, vals, sparkW, h, chartIndent-2)
	}
	if end < len(names) {
		sb.WriteString(styleFaint.Render(fmt.Sprintf("  ... %d more (↓ scroll)", len(names)-end)))
		sb.WriteString("\n")
	}
}

// renderTallSparkline renders a multi-row sparkline chart.
func renderTallSparkline(sb *strings.Builder, values []float64, width, height, indent int) {
	if len(values) == 0 || width <= 0 || height <= 0 {
		return
	}

	// Resample to always fill the full width.
	resampled := values
	if len(values) != width && len(values) > 1 {
		resampled = make([]float64, width)
		for i := 0; i < width; i++ {
			idx := i * (len(values) - 1) / (width - 1)
			resampled[i] = values[idx]
		}
	} else if len(values) == 1 {
		// Single point: fill entire width with that value.
		resampled = make([]float64, width)
		for i := range resampled {
			resampled[i] = values[0]
		}
	}

	mn, mx := resampled[0], resampled[0]
	for _, v := range resampled {
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
	}
	rang := mx - mn
	if rang == 0 {
		rang = 1
	}

	chartColor := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	topColor := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
	axisColor := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	pad := strings.Repeat(" ", indent+2)

	// For each row (top=max, bottom=min), render blocks.
	for r := height - 1; r >= 0; r-- {
		threshold := mn + rang*float64(r)/float64(height)
		topThreshold := mn + rang*float64(r+1)/float64(height)

		sb.WriteString(pad)
		for _, v := range resampled {
			if v >= topThreshold {
				// Value is above this row — full block.
				sb.WriteString(chartColor.Render("█"))
			} else if v >= threshold {
				// Value ends in this row — use partial block for precision.
				frac := (v - threshold) / (topThreshold - threshold)
				idx := int(frac * 7)
				if idx > 7 {
					idx = 7
				}
				chars := []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
				sb.WriteString(topColor.Render(chars[idx]))
			} else {
				sb.WriteString(" ")
			}
		}

		// Y-axis label on right.
		if r == height-1 {
			sb.WriteString(axisColor.Render(fmt.Sprintf("  %.4f", mx)))
		} else if r == 0 {
			sb.WriteString(axisColor.Render(fmt.Sprintf("  %.4f", mn)))
		}
		sb.WriteString("\n")
	}
}

// renderASCIILineChart draws a clean line chart using braille-like dots.
func renderASCIILineChart(sb *strings.Builder, values []float64, width, height int) {
	if len(values) == 0 || width <= 0 || height <= 0 {
		return
	}

	labelW := 10

	// Chart area width (minus labels and axis).
	chartW := width - labelW - 2
	if chartW < 10 {
		chartW = 10
	}

	// Resample values to fit chartW, spreading points evenly.
	resampled := values
	if len(values) > chartW {
		resampled = make([]float64, chartW)
		for i := 0; i < chartW; i++ {
			idx := i * (len(values) - 1) / (chartW - 1)
			resampled[i] = values[idx]
		}
	}

	// Find min/max with 5% padding.
	mn, mx := resampled[0], resampled[0]
	for _, v := range resampled {
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
	}
	rang := mx - mn
	if rang == 0 {
		rang = 1
	}
	mn -= rang * 0.05
	mx += rang * 0.05
	rang = mx - mn

	// Build grid: dots for data, spaces elsewhere.
	grid := make([][]rune, height)
	for r := 0; r < height; r++ {
		grid[r] = make([]rune, chartW)
		for c := range grid[r] {
			grid[r][c] = ' '
		}
	}

	// Map each value to a row and plot.
	rows := make([]int, len(resampled))
	for c, v := range resampled {
		row := height - 1 - int((v-mn)/rang*float64(height-1))
		if row < 0 {
			row = 0
		}
		if row >= height {
			row = height - 1
		}
		rows[c] = row
		grid[row][c] = '●'
	}

	// Connect adjacent points with line characters.
	for c := 0; c < len(resampled)-1; c++ {
		r1, r2 := rows[c], rows[c+1]
		if r1 == r2 {
			// Same row — horizontal segment (already have dots).
			continue
		}
		// Fill vertical gap between consecutive points.
		step := 1
		if r1 > r2 {
			step = -1
		}
		for r := r1 + step; r != r2; r += step {
			if grid[r][c] == ' ' {
				grid[r][c] = '│'
			}
		}
	}

	// Render.
	lineColor := lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	dotColor := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
	axisColor := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	gridColor := lipgloss.NewStyle().Foreground(lipgloss.Color("236"))

	for r := 0; r < height; r++ {
		// Y-axis label.
		label := strings.Repeat(" ", labelW)
		if r == 0 {
			label = fmt.Sprintf("%*.4f ", labelW-1, mx)
		} else if r == height/2 {
			label = fmt.Sprintf("%*.4f ", labelW-1, (mn+mx)/2)
		} else if r == height-1 {
			label = fmt.Sprintf("%*.4f ", labelW-1, mn)
		}
		sb.WriteString(axisColor.Render(label))
		sb.WriteString(axisColor.Render("│"))

		// Render row character by character.
		var row strings.Builder
		for c := 0; c < chartW && c < len(resampled); c++ {
			ch := grid[r][c]
			switch ch {
			case '●':
				row.WriteString(dotColor.Render("●"))
			case '│':
				row.WriteString(lineColor.Render("│"))
			default:
				// Grid dots at label rows for reference.
				if r == 0 || r == height/2 || r == height-1 {
					row.WriteString(gridColor.Render("·"))
				} else {
					row.WriteRune(' ')
				}
			}
		}
		sb.WriteString(row.String())
		sb.WriteString("\n")
	}

	// X-axis.
	sb.WriteString(strings.Repeat(" ", labelW))
	sb.WriteString(axisColor.Render("└"))
	axisLen := len(resampled)
	if axisLen > chartW {
		axisLen = chartW
	}
	sb.WriteString(axisColor.Render(strings.Repeat("─", axisLen)))

	// Step labels.
	sb.WriteString("  ")
	sb.WriteString(axisColor.Render(fmt.Sprintf("step 1 → %d", len(values))))
	sb.WriteString("\n")
}

// renderComparePanel renders the compare overlay panel.
func (m *jobsTUIModel) renderComparePanel(sb *strings.Builder) {
	sb.WriteString("\n")
	borderW := m.width
	if borderW <= 0 {
		borderW = 74
	}
	sb.WriteString(styleFaint.Render(strings.Repeat("─", borderW)))
	sb.WriteString("\n")

	title := lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true).Render("compare")
	sb.WriteString("  " + title + "  ")

	idA := m.compareJobA
	if len(idA) > 12 {
		idA = "…" + idA[len(idA)-11:]
	}
	idB := m.compareJobB
	if len(idB) > 12 {
		idB = "…" + idB[len(idB)-11:]
	}
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(idA))
	sb.WriteString(styleFaint.Render(" vs "))
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(idB))
	sb.WriteString("  " + styleFaint.Render(m.compareMetricName))
	sb.WriteString("\n")

	if m.compareErr != nil {
		sb.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(m.compareErr.Error())))
		return
	}
	if m.compareLoading {
		spinner := spinnerFrames[m.tickCount%len(spinnerFrames)]
		sb.WriteString(fmt.Sprintf("  %s loading metrics...\n", spinner))
		return
	}
	if m.compareDataA == nil || m.compareDataB == nil {
		sb.WriteString(styleFaint.Render("  No metric data available."))
		sb.WriteString("\n")
		return
	}

	// Render step-aligned overlay chart.
	chartW := m.width - 14
	if chartW < 20 {
		chartW = 20
	}
	chartH := 8

	sb.WriteString(renderCompareChart(m.compareDataA, m.compareDataB, chartW, chartH))

	// Legend.
	sb.WriteString("  ")
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("●"))
	sb.WriteString(" " + idA + "  ")
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("○"))
	sb.WriteString(" " + idB)
	sb.WriteString("\n")
}

// renderCompareChart renders a step-aligned ASCII chart overlaying two metric series.
func renderCompareChart(dataA, dataB []float64, chartW, chartH int) string {
	if len(dataA) == 0 && len(dataB) == 0 {
		return ""
	}
	if chartW < 4 || chartH < 2 {
		return ""
	}

	// Find global min/max across both series.
	globalMin := math.MaxFloat64
	globalMax := -math.MaxFloat64
	for _, v := range dataA {
		if v < globalMin {
			globalMin = v
		}
		if v > globalMax {
			globalMax = v
		}
	}
	for _, v := range dataB {
		if v < globalMin {
			globalMin = v
		}
		if v > globalMax {
			globalMax = v
		}
	}
	if globalMin == globalMax {
		globalMax = globalMin + 1
	}

	// Max steps for x-axis alignment.
	maxSteps := len(dataA)
	if len(dataB) > maxSteps {
		maxSteps = len(dataB)
	}

	mapY := func(v float64) int {
		y := int((v - globalMin) / (globalMax - globalMin) * float64(chartH-1))
		if y < 0 {
			y = 0
		}
		if y >= chartH {
			y = chartH - 1
		}
		return y
	}

	mapX := func(step, total int) int {
		if total <= 1 {
			return 0
		}
		x := step * (chartW - 1) / (total - 1)
		if x >= chartW {
			x = chartW - 1
		}
		return x
	}

	// Build grid: [row][col] -> 0=empty, 1=A(green), 2=B(gray), 3=both.
	grid := make([][]int, chartH)
	for r := range grid {
		grid[r] = make([]int, chartW)
	}

	for i, v := range dataA {
		x := mapX(i, maxSteps)
		y := mapY(v)
		grid[y][x] |= 1
	}
	for i, v := range dataB {
		x := mapX(i, maxSteps)
		y := mapY(v)
		grid[y][x] |= 2
	}

	greenStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	grayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	bothStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

	var sb strings.Builder
	labelW := 10

	for row := chartH - 1; row >= 0; row-- {
		val := globalMin + (globalMax-globalMin)*float64(row)/float64(chartH-1)
		label := fmt.Sprintf("%*.4f", labelW, val)
		sb.WriteString(styleFaint.Render(label))
		sb.WriteString(" ")

		for col := 0; col < chartW; col++ {
			switch grid[row][col] {
			case 0:
				sb.WriteString(" ")
			case 1:
				sb.WriteString(greenStyle.Render("●"))
			case 2:
				sb.WriteString(grayStyle.Render("○"))
			case 3:
				sb.WriteString(bothStyle.Render("◉"))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
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
		// Full layout: cursor(3) + sym(2) + id(13) + name(24) + worker(14) + metric(16) + duration(10) + created(10) + badge(12)
		const idColW = 13
		const nameColW = 24
		const workerColW = 14
		const metricColW = 16
		const durationColW = 10
		const createdColW = 10
		const badgeFieldW = 12

		idPadded := lsPadToWidth(idDisplay, idColW)

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

		// Status badge (right-aligned, last column).
		badge := j.Status
		if bs, ok := jobStatusBadgeStyles[j.Status]; ok {
			badge = bs.Render(j.Status)
		}
		badgeW := lipgloss.Width(badge)
		if badgeW < badgeFieldW {
			badge = strings.Repeat(" ", badgeFieldW-badgeW) + badge
		}

		if isSelected {
			sb.WriteString(styleSelected.Render(cursor))
			sb.WriteString(sym + " ")
			sb.WriteString(styleSelected.Render(idPadded + " "))
			sb.WriteString(styleSelected.Render(namePadded + " "))
			sb.WriteString(styleDate.Render(workerPadded) + " ")
			sb.WriteString(styleTagName.Render(metricPadded) + " ")
			sb.WriteString(styleDate.Render(durationPadded) + " ")
			sb.WriteString(styleDate.Render(createdPadded) + " ")
			sb.WriteString(badge)
		} else {
			sb.WriteString(cursor)
			sb.WriteString(sym + " ")
			sb.WriteString(styleFaint.Render(idPadded) + " ")
			sb.WriteString(styleTagName.Render(namePadded) + " ")
			sb.WriteString(styleDate.Render(workerPadded) + " ")
			sb.WriteString(styleTagName.Render(metricPadded) + " ")
			sb.WriteString(styleDate.Render(durationPadded) + " ")
			sb.WriteString(styleDate.Render(createdPadded) + " ")
			sb.WriteString(badge)
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
