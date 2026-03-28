package persona

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// MetricEntry records correction/suggestion counts for a single session.
type MetricEntry struct {
	SessionID   string    `yaml:"session_id"`
	Corrections int       `yaml:"corrections"`
	Suggestions int       `yaml:"suggestions"`
	Date        time.Time `yaml:"date"`
}

// GrowthSummary aggregates growth metrics across sessions.
type GrowthSummary struct {
	TotalSessions     int     `yaml:"total_sessions"`
	CorrectionRate30d float64 `yaml:"correction_rate_30d"`
	TrendDirection    string  `yaml:"trend_direction"` // "improving", "stable", "declining"
}

// defaultGrowthMetricsPath returns ~/.c4/growth_metrics.yaml.
func defaultGrowthMetricsPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(homeDir, ".c4", "growth_metrics.yaml"), nil
}

// loadMetricEntries reads the YAML list from path. Returns empty slice if file
// does not exist.
func loadMetricEntries(path string) ([]MetricEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read growth metrics %s: %w", path, err)
	}

	var entries []MetricEntry
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse growth metrics: %w", err)
	}
	return entries, nil
}

// saveMetricEntries writes entries to path as YAML, creating parent dirs as needed.
func saveMetricEntries(entries []MetricEntry, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create growth metrics dir: %w", err)
	}
	data, err := yaml.Marshal(entries)
	if err != nil {
		return fmt.Errorf("marshal growth metrics: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write growth metrics %s: %w", path, err)
	}
	return nil
}

// RecordSessionMetrics appends a new MetricEntry for the given session to the
// YAML file at path, creating parent directories if needed.
func RecordSessionMetrics(path, sessionID string, corrections, suggestions int) error {
	entries, err := loadMetricEntries(path)
	if err != nil {
		return err
	}
	entries = append(entries, MetricEntry{
		SessionID:   sessionID,
		Corrections: corrections,
		Suggestions: suggestions,
		Date:        time.Now().UTC(),
	})
	return saveMetricEntries(entries, path)
}

// CalcCorrectionRate returns corrections/suggestions for entries within the
// given days window from now. Returns 0.0 when there are no entries in the
// window or total suggestions is zero.
func CalcCorrectionRate(entries []MetricEntry, days int) float64 {
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)

	var totalCorrections, totalSuggestions int
	for _, e := range entries {
		if e.Date.Before(cutoff) {
			continue
		}
		totalCorrections += e.Corrections
		totalSuggestions += e.Suggestions
	}
	if totalSuggestions == 0 {
		return 0.0
	}
	return float64(totalCorrections) / float64(totalSuggestions)
}

// LoadGrowthSummary loads all metric entries from path and computes a
// GrowthSummary. Trend direction is determined by comparing the 30-day
// correction rate with the previous 30-day window.
//
//   - "improving"  — current rate < previous rate (fewer corrections per suggestion)
//   - "declining"  — current rate > previous rate
//   - "stable"     — rates are equal, or insufficient data for comparison
func LoadGrowthSummary(path string) (GrowthSummary, error) {
	entries, err := loadMetricEntries(path)
	if err != nil {
		return GrowthSummary{}, err
	}

	summary := GrowthSummary{
		TotalSessions:  len(entries),
		TrendDirection: "stable",
	}

	if len(entries) == 0 {
		return summary, nil
	}

	now := time.Now().UTC()
	cutoff30 := now.Add(-30 * 24 * time.Hour)
	cutoff60 := now.Add(-60 * 24 * time.Hour)

	// current 30d
	var curr30Corrections, curr30Suggestions int
	// previous 30d (30–60 days ago)
	var prev30Corrections, prev30Suggestions int

	for _, e := range entries {
		switch {
		case !e.Date.Before(cutoff30): // within last 30 days
			curr30Corrections += e.Corrections
			curr30Suggestions += e.Suggestions
		case !e.Date.Before(cutoff60): // within 30–60 days ago
			prev30Corrections += e.Corrections
			prev30Suggestions += e.Suggestions
		}
	}

	if curr30Suggestions > 0 {
		summary.CorrectionRate30d = float64(curr30Corrections) / float64(curr30Suggestions)
	}

	// Determine trend only when both windows have data.
	if curr30Suggestions > 0 && prev30Suggestions > 0 {
		currRate := float64(curr30Corrections) / float64(curr30Suggestions)
		prevRate := float64(prev30Corrections) / float64(prev30Suggestions)
		switch {
		case currRate < prevRate:
			summary.TrendDirection = "improving"
		case currRate > prevRate:
			summary.TrendDirection = "declining"
		default:
			summary.TrendDirection = "stable"
		}
	}

	return summary, nil
}
