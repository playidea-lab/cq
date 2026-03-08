package edgeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/piqsol/c4/c5/internal/model"
)

// MetricsReporter runs MetricsCommand periodically, parses stdout KEY=VALUE lines,
// and POSTs aggregated metrics to Hub POST /v1/edges/{id}/metrics.
type MetricsReporter struct {
	edgeID   string
	hubURL   string
	apiKey   string
	command  string
	interval time.Duration
	client   *http.Client

	mu      sync.Mutex
	metrics map[string]float64
}

func newMetricsReporter(edgeID, hubURL, apiKey, command string, interval time.Duration, client *http.Client) *MetricsReporter {
	return &MetricsReporter{
		edgeID:   edgeID,
		hubURL:   hubURL,
		apiKey:   apiKey,
		command:  command,
		interval: interval,
		client:   client,
		metrics:  make(map[string]float64),
	}
}

// ParseMetricsLine parses a single "key=value" line. Returns ok=false for non-numeric values or missing "=".
func ParseMetricsLine(line string) (key string, value float64, ok bool) {
	idx := strings.IndexByte(line, '=')
	if idx < 0 {
		return "", 0, false
	}
	k := strings.TrimSpace(line[:idx])
	v := strings.TrimSpace(line[idx+1:])
	if k == "" {
		return "", 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return "", 0, false
	}
	return k, f, true
}

// Ingest parses a line and stores the metric if valid.
func (m *MetricsReporter) Ingest(line string) {
	k, v, ok := ParseMetricsLine(line)
	if !ok {
		return
	}
	m.mu.Lock()
	m.metrics[k] = v
	m.mu.Unlock()
}

// Start runs the metrics loop until ctx is done.
func (m *MetricsReporter) Start(ctx context.Context) {
	if m.command == "" {
		return
	}
	tick := time.NewTicker(m.interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			m.collect(ctx)
			m.report(ctx)
		}
	}
}

func (m *MetricsReporter) collect(ctx context.Context) {
	cmd := exec.CommandContext(ctx, "sh", "-c", m.command)
	out, err := cmd.Output()
	if err != nil {
		log.Printf("edge-agent: metrics command error: %v", err)
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		m.Ingest(line)
	}
}

func (m *MetricsReporter) report(ctx context.Context) {
	m.mu.Lock()
	if len(m.metrics) == 0 {
		m.mu.Unlock()
		return
	}
	snapshot := make(map[string]float64, len(m.metrics))
	for k, v := range m.metrics {
		snapshot[k] = v
	}
	m.mu.Unlock()

	payload := model.EdgeMetricsRequest{Values: snapshot}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("edge-agent: marshal metrics: %v", err)
		return
	}
	url := fmt.Sprintf("%s/v1/edges/%s/metrics", strings.TrimRight(m.hubURL, "/"), m.edgeID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		log.Printf("edge-agent: metrics request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if m.apiKey != "" {
		req.Header.Set("X-API-Key", m.apiKey)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		log.Printf("edge-agent: POST metrics: %v", err)
		return
	}
	resp.Body.Close()
}
