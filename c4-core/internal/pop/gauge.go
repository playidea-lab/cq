package pop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// gaugeEntry holds a single named gauge value.
type gaugeEntry struct {
	Name      string  `json:"name"`
	Value     float64 `json:"value"`
	UpdatedAt string  `json:"updated_at"`
}

// gaugeFile is the on-disk schema for gauge.json.
type gaugeFile struct {
	Gauges  []gaugeEntry `json:"gauges"`
	Version int          `json:"version"`
}

// GaugeTracker reads and writes gauge.json, and evaluates thresholds.
type GaugeTracker struct {
	path string
	data gaugeFile
}

// Thresholds for triggering POP actions.
const (
	ThresholdMergeAmbiguity  = 0.20
	ThresholdAvgFanOut       = 3.0
	ThresholdContradictions  = 10.0
	ThresholdTemporalQueries = 5.0
)

// DefaultGaugePath returns the canonical path for gauge.json under root.
func DefaultGaugePath(root string) string {
	return filepath.Join(root, ".c4", "pop", "gauge.json")
}

// NewGaugeTracker creates a GaugeTracker for the given gauge.json path.
func NewGaugeTracker(path string) *GaugeTracker {
	return &GaugeTracker{path: path}
}

// Load reads gauge.json from disk. If the file does not exist, gauge data is
// zero-initialized. The directory is created if needed.
func (g *GaugeTracker) Load() error {
	dir := filepath.Dir(g.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	raw, err := os.ReadFile(g.path)
	if os.IsNotExist(err) {
		g.data = gaugeFile{Version: 1}
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, &g.data)
}

// Save persists the current gauge data to disk atomically (tmpfile → Rename).
func (g *GaugeTracker) Save() error {
	raw, err := json.Marshal(&g.data)
	if err != nil {
		return err
	}
	dir := filepath.Dir(g.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "gauge-*.json.tmp")
	if err != nil {
		return fmt.Errorf("pop: gauge tmp create: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("pop: gauge tmp write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("pop: gauge tmp close: %w", err)
	}
	return os.Rename(tmpName, g.path)
}

// Set updates (or appends) the named gauge to value.
func (g *GaugeTracker) Set(name string, value float64) {
	now := time.Now().UTC().Format(time.RFC3339)
	for i, e := range g.data.Gauges {
		if e.Name == name {
			g.data.Gauges[i].Value = value
			g.data.Gauges[i].UpdatedAt = now
			return
		}
	}
	g.data.Gauges = append(g.data.Gauges, gaugeEntry{
		Name:      name,
		Value:     value,
		UpdatedAt: now,
	})
}

// Get returns the current value of the named gauge (0 if not set).
func (g *GaugeTracker) Get(name string) float64 {
	for _, e := range g.data.Gauges {
		if e.Name == name {
			return e.Value
		}
	}
	return 0
}

// ExceedsThreshold returns true when a gauge's value exceeds its defined threshold.
// Unknown gauge names always return false.
func (g *GaugeTracker) ExceedsThreshold(name string) bool {
	v := g.Get(name)
	switch name {
	case "merge_ambiguity":
		return v > ThresholdMergeAmbiguity
	case "avg_fan_out":
		return v > ThresholdAvgFanOut
	case "contradictions":
		return v > ThresholdContradictions
	case "temporal_queries":
		return v > ThresholdTemporalQueries
	}
	return false
}
