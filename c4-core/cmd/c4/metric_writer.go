package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// metricPattern matches @key=value where value is a numeric literal.
// Regex from DoD: @(\w+)=([+-]?\d*\.?\d+(?:[eE][+-]?\d+)?)
var metricPattern = regexp.MustCompile(`@(\w+)=([+-]?\d*\.?\d+(?:[eE][+-]?\d+)?)`)

// metricEvent carries a single parsed metric for delivery.
type metricEvent struct {
	key   string
	value float64
}

// checkpointBody is the JSON payload for the experiment_checkpoint RPC.
type checkpointBody struct {
	RunID  string  `json:"run_id"`
	Metric float64 `json:"metric"`
	Path   string  `json:"path"`
}

// MetricWriter implements io.Writer with line buffering.
// It scans each line for @key=value patterns and forwards parsed metrics
// to a background MetricCollector goroutine that calls the Supabase
// experiment_checkpoint RPC.
//
// The underlying writer (e.g. os.Stdout) still receives every byte written
// to MetricWriter — metric interception is transparent.
type MetricWriter struct {
	underlying io.Writer
	buf        []byte

	ch     chan metricEvent
	done   chan struct{}
	once   sync.Once
}

// NewMetricWriter creates a MetricWriter that wraps underlying writer w.
// experimentID is used as the RPC run_id field.
// supabaseURL must be the Supabase project URL (e.g. https://xxx.supabase.co).
// anonKey is the Supabase anon key; jwt is the user JWT for RLS.
func NewMetricWriter(w io.Writer, experimentID, supabaseURL, anonKey, jwt string) *MetricWriter {
	mw := &MetricWriter{
		underlying: w,
		ch:         make(chan metricEvent, 64),
		done:       make(chan struct{}),
	}
	go runMetricCollector(mw.ch, mw.done, experimentID, supabaseURL, anonKey, jwt)
	return mw
}

// Write implements io.Writer. Bytes are forwarded to the underlying writer,
// and complete lines are scanned for @key=value patterns.
func (mw *MetricWriter) Write(p []byte) (int, error) {
	n, err := mw.underlying.Write(p)
	if err != nil {
		return n, err
	}

	mw.buf = append(mw.buf, p...)
	for {
		idx := bytes.IndexByte(mw.buf, '\n')
		if idx < 0 {
			break
		}
		line := string(mw.buf[:idx])
		mw.buf = mw.buf[idx+1:]
		mw.parseLine(line)
	}
	return n, nil
}

// parseLine scans a single text line for @key=value patterns and enqueues
// each match to the collector channel (non-blocking; drops if full).
func (mw *MetricWriter) parseLine(line string) {
	matches := metricPattern.FindAllStringSubmatch(line, -1)
	for _, m := range matches {
		val, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			continue
		}
		select {
		case mw.ch <- metricEvent{key: m[1], value: val}:
		default:
			// channel full — drop metric to avoid blocking stdout
		}
	}
}

// Close drains the metric channel and waits for the collector goroutine to exit.
func (mw *MetricWriter) Close() {
	mw.once.Do(func() {
		close(mw.ch)
		<-mw.done
	})
}

// runMetricCollector is the background goroutine that consumes metric events
// and sends them to the Supabase experiment_checkpoint RPC.
// Circuit breaker: 3 consecutive failures disable further HTTP calls.
func runMetricCollector(ch <-chan metricEvent, done chan<- struct{}, experimentID, supabaseURL, anonKey, jwt string) {
	defer close(done)

	client := &http.Client{Timeout: 5 * time.Second}
	rpcURL := supabaseURL + "/rest/v1/rpc/experiment_checkpoint"

	consecutiveFails := 0
	const maxFails = 3
	disabled := false

	for ev := range ch {
		if disabled {
			continue
		}

		body := checkpointBody{
			RunID:  experimentID,
			Metric: ev.value,
			Path:   ev.key,
		}
		if err := postCheckpoint(client, rpcURL, anonKey, jwt, body); err != nil {
			consecutiveFails++
			if consecutiveFails >= maxFails {
				disabled = true
				fmt.Fprintf(os.Stderr,
					"[MetricWriter] circuit breaker: %d consecutive failures, disabling metric upload. last error: %v\n",
					maxFails, err)
			}
		} else {
			consecutiveFails = 0
		}
	}
}

// postCheckpoint sends a single checkpoint record to the Supabase RPC endpoint.
func postCheckpoint(client *http.Client, rpcURL, anonKey, jwt string, body checkpointBody) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, rpcURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if anonKey != "" {
		req.Header.Set("apikey", anonKey)
	}
	bearerToken := jwt
	if bearerToken == "" {
		bearerToken = anonKey
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("POST experiment_checkpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST experiment_checkpoint: %d %s", resp.StatusCode, string(b))
	}
	return nil
}
