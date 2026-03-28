package persona

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// MetricEntry records correction and suggestion counts for a single session.
type MetricEntry struct {
	SessionID   string    `yaml:"session_id"`
	Corrections int       `yaml:"corrections"`
	Suggestions int       `yaml:"suggestions"`
	Date        time.Time `yaml:"date"`
}

// GrowthSummary summarises learning trends derived from MetricEntry history.
type GrowthSummary struct {
	TotalSessions    int     `yaml:"total_sessions"`
	CorrectionRate30d float64 `yaml:"correction_rate_30d"`
	// TrendDirection is "improving", "stable", or "declining".
	TrendDirection string `yaml:"trend_direction"`
}

// loadMetrics reads the YAML list from path. Returns an empty slice if the
// file does not exist.
func loadMetrics(path string) ([]MetricEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read metrics %s: %w", path, err)
	}

	var entries []MetricEntry
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse metrics: %w", err)
	}
	return entries, nil
}

// saveMetrics writes entries to path as YAML, creating parent directories as
// needed.
func saveMetrics(entries []MetricEntry, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create metrics dir: %w", err)
	}
	data, err := yaml.Marshal(entries)
	if err != nil {
		return fmt.Errorf("marshal metrics: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write metrics %s: %w", path, err)
	}
	return nil
}

// RecordSessionMetrics appends a new MetricEntry for the given session to the
// YAML list at path, creating the file (and any parent directories) if needed.
func RecordSessionMetrics(path, sessionID string, corrections, suggestions int) error {
	entries, err := loadMetrics(path)
	if err != nil {
		return err
	}
	entries = append(entries, MetricEntry{
		SessionID:   sessionID,
		Corrections: corrections,
		Suggestions: suggestions,
		Date:        time.Now().UTC(),
	})
	return saveMetrics(entries, path)
}

// CalcCorrectionRate returns the ratio of total corrections to total
// suggestions for entries whose Date falls within the last [days] days.
// Returns 0.0 when there are no matching entries or when total suggestions is
// zero.
func CalcCorrectionRate(entries []MetricEntry, days int) float64 {
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	var totalCorrections, totalSuggestions int
	for _, e := range entries {
		if e.Date.After(cutoff) || e.Date.Equal(cutoff) {
			totalCorrections += e.Corrections
			totalSuggestions += e.Suggestions
		}
	}
	if totalSuggestions == 0 {
		return 0.0
	}
	return float64(totalCorrections) / float64(totalSuggestions)
}

// LoadGrowthSummary loads the metric entries at path and computes a
// GrowthSummary. The trend is determined by comparing the correction rate of
// the most recent 30 days against the previous 30 days (days 31–60):
//   - "improving"  — current rate < prev rate (fewer corrections needed)
//   - "declining"  — current rate > prev rate
//   - "stable"     — rates are equal or there is insufficient history
func LoadGrowthSummary(path string) (GrowthSummary, error) {
	entries, err := loadMetrics(path)
	if err != nil {
		return GrowthSummary{}, err
	}

	current30 := CalcCorrectionRate(entries, 30)

	// Build prev-30 window manually so we don't pull in a second helper.
	now := time.Now().UTC()
	cutoff30 := now.AddDate(0, 0, -30)
	cutoff60 := now.AddDate(0, 0, -60)
	var prevCorr, prevSugg int
	for _, e := range entries {
		if (e.Date.After(cutoff60) || e.Date.Equal(cutoff60)) && e.Date.Before(cutoff30) {
			prevCorr += e.Corrections
			prevSugg += e.Suggestions
		}
	}
	var prev30 float64
	if prevSugg > 0 {
		prev30 = float64(prevCorr) / float64(prevSugg)
	}

	trend := "stable"
	if prevSugg > 0 {
		switch {
		case current30 < prev30:
			trend = "improving"
		case current30 > prev30:
			trend = "declining"
		}
	}

	return GrowthSummary{
		TotalSessions:     len(entries),
		CorrectionRate30d: current30,
		TrendDirection:    trend,
	}, nil
}
