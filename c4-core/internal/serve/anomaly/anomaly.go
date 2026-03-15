package anomaly

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/changmin/c4-core/internal/knowledge"
	"github.com/changmin/c4-core/internal/serve"
)

// Config holds configuration for the Monitor component.
type Config struct {
	Store        *knowledge.Store
	PollInterval time.Duration
}

// Monitor implements serve.Component.
// It polls TypeExperiment documents, checks metric ranges, and creates
// TypeDebate escalation records when anomalies are detected.
type Monitor struct {
	cfg    Config
	cancel context.CancelFunc
	done   chan struct{}

	mu             sync.Mutex
	status         string
	lastEscalation map[string]time.Time // hypothesis_id → last escalation time
}

// compile-time interface assertion
var _ serve.Component = (*Monitor)(nil)

// New creates a new Monitor with the given config.
func New(cfg Config) *Monitor {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 30 * time.Second
	}
	return &Monitor{
		cfg:            cfg,
		status:         "ok",
		lastEscalation: make(map[string]time.Time),
	}
}

func (a *Monitor) Name() string { return "anomaly_monitor" }

func (a *Monitor) Start(ctx context.Context) error {
	ctx2, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	a.done = make(chan struct{})
	go a.loop(ctx2)
	return nil
}

// Stop cancels the context and waits for the loop goroutine to exit.
// cancel() is idempotent in Go, so no nil-out is needed.
func (a *Monitor) Stop(_ context.Context) error {
	if a.cancel != nil {
		a.cancel()
	}
	if a.done != nil {
		<-a.done
	}
	return nil
}

func (a *Monitor) Health() serve.ComponentHealth {
	a.mu.Lock()
	defer a.mu.Unlock()
	return serve.ComponentHealth{Status: a.status}
}

func (a *Monitor) loop(ctx context.Context) {
	defer close(a.done)
	ticker := time.NewTicker(a.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.check(ctx)
		}
	}
}

// MetricRange represents an expected metric range from experiment frontmatter.
type MetricRange struct {
	Name string  `json:"name"`
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
}

// check scans TypeExperiment documents for metric anomalies and creates
// TypeDebate escalation records when a metric is out of the expected range.
// Duplicate escalations for the same hypothesis_id within 24 hours are skipped.
func (a *Monitor) check(ctx context.Context) {
	docs, err := a.cfg.Store.List(string(knowledge.TypeExperiment), "", 100)
	if err != nil {
		fmt.Fprintf(os.Stderr, "anomaly-monitor: list experiments: %v\n", err)
		a.mu.Lock()
		a.status = "degraded"
		a.mu.Unlock()
		return
	}

	docsDir := a.cfg.Store.DocsDir()

	for _, doc := range docs {
		if ctx.Err() != nil {
			return
		}
		hypID, _ := doc["id"].(string)
		if hypID == "" || strings.Contains(hypID, "/") || strings.Contains(hypID, "..") {
			continue
		}

		// Read full frontmatter from Markdown file (SSOT).
		fm := ReadFrontmatter(filepath.Join(docsDir, hypID+".md"))
		ranges := ParseMetricRanges(fm)
		if len(ranges) == 0 {
			continue
		}

		anomaly, detail := DetectAnomaly(fm, ranges)
		if !anomaly {
			continue
		}

		// 24h dedup watermark: skip if escalated recently.
		a.mu.Lock()
		if last, ok := a.lastEscalation[hypID]; ok && time.Since(last) < 24*time.Hour {
			a.mu.Unlock()
			continue
		}
		a.mu.Unlock()

		meta := map[string]any{
			"hypothesis_id":  hypID,
			"domain":         "escalation",
			"trigger_reason": "escalation",
		}
		if _, createErr := a.cfg.Store.Create(knowledge.TypeDebate, meta, detail); createErr != nil {
			fmt.Fprintf(os.Stderr, "anomaly-monitor: create debate: %v\n", createErr)
		} else {
			// Update watermark only after successful store write.
			a.mu.Lock()
			a.lastEscalation[hypID] = time.Now()
			a.mu.Unlock()
		}
	}

	a.mu.Lock()
	a.status = "ok"
	a.mu.Unlock()
}

// ReadFrontmatter reads and parses YAML frontmatter from a Markdown file.
// Returns an empty map on any error.
func ReadFrontmatter(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{}
	}
	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return map[string]any{}
	}
	end := strings.Index(content[3:], "\n---")
	if end < 0 {
		return map[string]any{}
	}
	yamlBlock := content[3 : end+3]
	return ParseAnomalyYAML(yamlBlock)
}

// ParseAnomalyYAML parses simple key: value YAML frontmatter into a map.
func ParseAnomalyYAML(text string) map[string]any {
	result := make(map[string]any)
	for _, line := range strings.Split(text, "\n") {
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if key == "" {
			continue
		}
		// Attempt numeric parse; fall back to string.
		if f, err := parseAnomalyFloat(val); err == nil {
			result[key] = f
		} else {
			result[key] = val
		}
	}
	return result
}

// parseAnomalyFloat parses a float64 from a string, returning an error if not numeric.
func parseAnomalyFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%g", &f)
	return f, err
}

// ParseMetricRanges extracts expected_metrics_range from frontmatter.
// The field value is a JSON array: [{name, min, max}, ...].
func ParseMetricRanges(fm map[string]any) []MetricRange {
	raw, ok := fm["expected_metrics_range"]
	if !ok {
		return nil
	}
	rawStr, _ := raw.(string)
	if rawStr == "" {
		return nil
	}
	var ranges []MetricRange
	if err := json.Unmarshal([]byte(rawStr), &ranges); err != nil {
		return nil
	}
	return ranges
}

// DetectAnomaly returns true if any metric in the frontmatter falls outside its range.
func DetectAnomaly(fm map[string]any, ranges []MetricRange) (bool, string) {
	for _, r := range ranges {
		val, ok := fm[r.Name]
		if !ok {
			continue
		}
		f, ok := val.(float64)
		if !ok {
			continue
		}
		if f < r.Min || f > r.Max {
			return true, fmt.Sprintf("metric %q value %g out of range [%g, %g]", r.Name, f, r.Min, r.Max)
		}
	}
	return false, ""
}
