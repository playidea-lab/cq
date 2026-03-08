//go:build research

package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/spf13/cobra"
)

func init() {
	parent := findOrCreateResearchCmd()

	watchCmd := &cobra.Command{
		Use:   "watch [--hypothesis <id>] [--interval <seconds>]",
		Short: "Watch research loop metrics + context in real-time",
		RunE:  runResearchWatch,
	}
	watchCmd.Flags().StringP("hypothesis", "H", "", "Hypothesis ID to watch (optional, shows latest if omitted)")
	watchCmd.Flags().IntP("interval", "i", 10, "Refresh interval in seconds (min 1)")

	parent.AddCommand(watchCmd)
}

func runResearchWatch(cmd *cobra.Command, args []string) error {
	hypID, _ := cmd.Flags().GetString("hypothesis")
	interval, _ := cmd.Flags().GetInt("interval")
	if interval < 1 {
		interval = 10
	}

	store, err := openKnowledgeStore()
	if err != nil {
		return err
	}
	defer store.Close()

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	// initial render
	if err := renderWatchView(store, hypID, os.Stdout); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := renderWatchView(store, hypID, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "watch: %v\n", err)
			}
		}
	}
}

// watchData holds the state used to render one watch frame.
type watchData struct {
	HypothesisID   string
	Round          int
	Status         string
	ValLossHistory []float64
	TestMetricHist []float64
	VerdictHistory []string
}

// renderWatchView fetches data from the store and prints the watch UI.
func renderWatchView(store *knowledge.Store, hypID string, out *os.File) error {
	data, err := buildWatchData(store, hypID)
	if err != nil {
		return err
	}
	printWatchFrame(out, data)
	return nil
}

// buildWatchData loads experiment documents and extracts metrics/context.
func buildWatchData(store *knowledge.Store, hypID string) (*watchData, error) {
	docs, err := store.List(string(knowledge.TypeExperiment), "", 100)
	if err != nil {
		return nil, fmt.Errorf("list experiments: %w", err)
	}

	// Filter by hypothesis if specified.
	var filtered []map[string]any
	for _, d := range docs {
		if hypID == "" {
			filtered = append(filtered, d)
		} else {
			if hid, _ := d["hypothesis_id"].(string); hid == hypID {
				filtered = append(filtered, d)
			} else if id, _ := d["id"].(string); id == hypID {
				filtered = append(filtered, d)
			}
		}
	}

	data := &watchData{
		HypothesisID: hypID,
		Status:       "running",
	}

	for _, d := range filtered {
		body, _ := d["body"].(string)
		parseMetricsFromBody(body, data)
	}

	data.Round = len(data.VerdictHistory)
	return data, nil
}

// parseMetricsFromBody extracts val_loss, test_metric, and verdict lines from a doc body.
func parseMetricsFromBody(body string, data *watchData) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "val_loss:") {
			val := parseFloat(strings.TrimPrefix(line, "val_loss:"))
			if val != 0 {
				data.ValLossHistory = append(data.ValLossHistory, val)
			}
		} else if strings.HasPrefix(line, "test_metric:") {
			val := parseFloat(strings.TrimPrefix(line, "test_metric:"))
			if val != 0 {
				data.TestMetricHist = append(data.TestMetricHist, val)
			}
		} else if strings.HasPrefix(line, "verdict:") {
			v := strings.TrimSpace(strings.TrimPrefix(line, "verdict:"))
			if v != "" {
				data.VerdictHistory = append(data.VerdictHistory, v)
			}
		} else if strings.HasPrefix(line, "status:") {
			s := strings.TrimSpace(strings.TrimPrefix(line, "status:"))
			if s != "" {
				data.Status = s
			}
		}
	}
}

// parseFloat parses a trimmed float string; returns 0 on error.
func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// trendArrow returns ▼ or ▲ based on direction (lower vs higher than prev).
func trendArrow(prev, cur float64, lowerIsBetter bool) string {
	if lowerIsBetter {
		if cur < prev {
			return "▼"
		}
		return "▲"
	}
	if cur > prev {
		return "▲"
	}
	return "▼"
}

// formatMetricHistory formats a history slice as "0.45 ▼ 0.42 ... (improving|degrading)".
func formatMetricHistory(history []float64, lowerIsBetter bool) string {
	if len(history) == 0 {
		return "(no data)"
	}
	var parts []string
	for i, v := range history {
		if i == 0 {
			parts = append(parts, fmt.Sprintf("%.4g", v))
		} else {
			parts = append(parts, trendArrow(history[i-1], v, lowerIsBetter))
			parts = append(parts, fmt.Sprintf("%.4g", v))
		}
	}
	label := trendLabel(history, lowerIsBetter)
	return strings.Join(parts, " ") + " (" + label + ")"
}

func trendLabel(history []float64, lowerIsBetter bool) string {
	if len(history) < 2 {
		return "stable"
	}
	last := history[len(history)-1]
	first := history[0]
	if lowerIsBetter {
		if last < first {
			return "improving"
		}
		return "degrading"
	}
	if last > first {
		return "improving"
	}
	return "degrading"
}

// interventionSignal returns a human-readable signal string or "" if no intervention needed.
func interventionSignal(data *watchData) string {
	// Check val_loss degradation: 3 consecutive increases.
	if countConsecutiveDegrading(data.ValLossHistory, true) >= 3 {
		return "[개입 추천] val_loss 악화 3회 연속"
	}
	// Check null_result streak >= 2.
	streak := nullResultStreak(data.VerdictHistory)
	if streak >= 2 {
		return fmt.Sprintf("[개입 추천] null_result %d회 연속", streak)
	}
	return ""
}

// countConsecutiveDegrading counts trailing degrading steps in a history slice.
func countConsecutiveDegrading(history []float64, lowerIsBetter bool) int {
	count := 0
	for i := len(history) - 1; i >= 1; i-- {
		degrading := false
		if lowerIsBetter {
			degrading = history[i] > history[i-1]
		} else {
			degrading = history[i] < history[i-1]
		}
		if degrading {
			count++
		} else {
			break
		}
	}
	return count
}

// nullResultStreak counts the trailing null_result entries in verdictHistory.
func nullResultStreak(verdicts []string) int {
	count := 0
	for i := len(verdicts) - 1; i >= 0; i-- {
		if verdicts[i] == "null_result" {
			count++
		} else {
			break
		}
	}
	return count
}

// printWatchFrame renders one frame of the watch TUI to out.
func printWatchFrame(out *os.File, data *watchData) {
	const width = 58
	border := strings.Repeat("─", width)

	hypLabel := data.HypothesisID
	if hypLabel == "" {
		hypLabel = "(all)"
	}

	valLossStr := formatMetricHistory(data.ValLossHistory, true)
	testMetStr := formatMetricHistory(data.TestMetricHist, false)

	verdictStr := "(none)"
	if len(data.VerdictHistory) > 0 {
		verdictStr = strings.Join(data.VerdictHistory, ", ")
	}

	nullStreak := nullResultStreak(data.VerdictHistory)
	signal := interventionSignal(data)
	signalStr := "(없음)"
	if signal != "" {
		signalStr = "⚠️  " + signal
	}

	fmt.Fprintf(out, "┌─ Research Loop Watch %s┐\n", strings.Repeat("─", width-22))
	fmt.Fprintf(out, "│ %-*s│\n", width-1, "[메트릭 레이어]")
	fmt.Fprintf(out, "│ Hypothesis: %-12s Round: %-5d Status: %-8s│\n",
		truncate(hypLabel, 12), data.Round, truncate(data.Status, 8))
	fmt.Fprintf(out, "│ val_loss:    %-*s│\n", width-15, truncate(valLossStr, width-15))
	fmt.Fprintf(out, "│ test_metric: %-*s│\n", width-15, truncate(testMetStr, width-15))
	fmt.Fprintf(out, "│ %-*s│\n", width-1, "")
	fmt.Fprintf(out, "│ %-*s│\n", width-1, "[컨텍스트 레이어]")
	fmt.Fprintf(out, "│ Verdict history: %-*s│\n", width-19, truncate(verdictStr, width-19))
	fmt.Fprintf(out, "│ NullResult streak: %-2d회 (threshold: 2)%-*s│\n",
		nullStreak, width-37, "")
	fmt.Fprintf(out, "│ %-*s│\n", width-1, "")
	fmt.Fprintf(out, "│ %-*s│\n", width-1, "[개입 추천 시그널]")
	fmt.Fprintf(out, "│ %-*s│\n", width-1, truncate(signalStr, width-1))
	fmt.Fprintf(out, "└%s┘\n", border)
}

// truncate cuts s to max length, appending "…" if needed.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}
