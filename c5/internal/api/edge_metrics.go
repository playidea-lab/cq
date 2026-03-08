package api

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/piqsol/c4/c5/internal/model"
)

// edgeIDRe validates edge IDs extracted from URL paths.
// Edge IDs are UUIDs (hex digits and hyphens); reject any traversal attempts.
var edgeIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// thresholdMu protects thresholdCooldowns against concurrent access.
var (
	thresholdMu        sync.Mutex
	thresholdCooldowns = make(map[string]time.Time)
	thresholdCooldown  = 60 * time.Second
)

const maxMetricsBodyBytes = 64 * 1024 // 64 KB

// handleEdgeMetricsPost handles POST /v1/edges/{id}/metrics
func (s *Server) handleEdgeMetricsPost(w http.ResponseWriter, r *http.Request) {
	edgeID := edgeIDFromMetricsPath(r.URL.Path)
	if edgeID == "" {
		writeError(w, http.StatusBadRequest, "edge_id is required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxMetricsBodyBytes)
	req, ok := decodeRequest[model.EdgeMetricsRequest](w, r, "POST")
	if !ok {
		return
	}
	if len(req.Values) == 0 {
		writeError(w, http.StatusBadRequest, "values must not be empty")
		return
	}

	ts := req.Timestamp
	if ts == 0 {
		ts = time.Now().Unix()
	}

	if err := s.store.AddEdgeMetrics(edgeID, req.Values, ts); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Check thresholds asynchronously — non-blocking metric ingestion.
	edge, err := s.store.GetEdge(edgeID)
	if err == nil && edge != nil {
		go checkThresholds(s, edgeID, req.Values, edge.Metadata)
	}

	writeJSON(w, map[string]bool{"ok": true})
}

// handleEdgeMetricsGet handles GET /v1/edges/{id}/metrics?limit=N
func (s *Server) handleEdgeMetricsGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}

	edgeID := edgeIDFromMetricsPath(r.URL.Path)
	if edgeID == "" {
		writeError(w, http.StatusBadRequest, "edge_id is required")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	entries, err := s.store.GetEdgeMetrics(edgeID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, model.EdgeMetricsListResponse{
		EdgeID:  edgeID,
		Metrics: entries,
	})
}

// edgeIDFromMetricsPath extracts and validates edge ID from /v1/edges/{id}/metrics.
// Returns "" if the path is malformed or the ID contains unexpected characters.
func edgeIDFromMetricsPath(p string) string {
	p = strings.TrimPrefix(p, "/v1/edges/")
	p = strings.TrimSuffix(p, "/metrics")
	id := strings.TrimSuffix(p, "/")
	if !edgeIDRe.MatchString(id) {
		return ""
	}
	return id
}

// checkThresholds evaluates metric values against thresholds stored in edge metadata.
// Metadata key format: "threshold_<metric_key>" = "<float64 min value>"
// If a value is below the threshold and cooldown has elapsed, an event is published.
func checkThresholds(s *Server, edgeID string, values map[string]float64, metadata map[string]string) {
	for metricKey, value := range values {
		threshKey := "threshold_" + metricKey
		threshStr, ok := metadata[threshKey]
		if !ok {
			continue
		}

		threshold, err := strconv.ParseFloat(strings.TrimSpace(threshStr), 64)
		if err != nil {
			log.Printf("WARN threshold_monitor: edge=%s metric=%s invalid threshold %q: %v",
				edgeID, metricKey, threshStr, err)
			continue
		}

		if value < threshold {
			if !tryAcquireCooldown(edgeID, metricKey) {
				continue
			}

			log.Printf("WARN THRESHOLD_BREACH edge_id=%s metric=%s value=%g threshold=%g",
				edgeID, metricKey, value, threshold)

			go publishThresholdEvent(s, edgeID, metricKey, value, threshold)
		}
	}
}

// tryAcquireCooldown returns true if the cooldown for this (edgeID, metricKey)
// has elapsed (or was never set), and records the current time.
// Uses a mutex-protected map to prevent concurrent goroutines from both passing
// the cooldown check for the same key simultaneously.
func tryAcquireCooldown(edgeID, metricKey string) bool {
	key := edgeID + "\x00" + metricKey
	now := time.Now()

	thresholdMu.Lock()
	defer thresholdMu.Unlock()

	if last, ok := thresholdCooldowns[key]; ok && now.Sub(last) < thresholdCooldown {
		return false
	}
	thresholdCooldowns[key] = now
	return true
}

// resetThresholdCooldown removes the cooldown entry for the given key.
// Exported for testing only.
func resetThresholdCooldown(key string) {
	thresholdMu.Lock()
	delete(thresholdCooldowns, key)
	thresholdMu.Unlock()
}

// publishThresholdEvent sends an edge.metrics.threshold_exceeded event to the EventBus.
func publishThresholdEvent(s *Server, edgeID, metricKey string, value, threshold float64) {
	if s.eventPub == nil || !s.eventPub.IsEnabled() {
		return
	}

	err := s.eventPub.Publish("edge.metrics.threshold_exceeded", "c5", map[string]any{
		"edge_id":    edgeID,
		"metric_key": metricKey,
		"value":      value,
		"threshold":  threshold,
		"message":    fmt.Sprintf("edge %s: %s=%.4g below threshold %.4g", edgeID, metricKey, value, threshold),
	})
	if err != nil {
		log.Printf("WARN threshold_monitor: failed to publish event for edge=%s metric=%s: %v",
			edgeID, metricKey, err)
	}
}
