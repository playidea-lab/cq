package store

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/piqsol/c4/c5/internal/model"
)

// edgeMetricsRing is an in-memory ring buffer for edge metrics (max 100 entries per edge).
// Server restart clears all data — acceptable for v1.
type edgeMetricsRing struct {
	mu      sync.Mutex
	entries map[string][]model.EdgeMetricsEntry // edgeID → ring buffer
}

var globalEdgeMetrics = &edgeMetricsRing{
	entries: make(map[string][]model.EdgeMetricsEntry),
}

const edgeMetricsMaxPerEdge = 100

// AddEdgeMetrics stores a metrics snapshot for an edge device.
func (s *Store) AddEdgeMetrics(edgeID string, values map[string]float64, ts int64) error {
	if ts == 0 {
		ts = time.Now().Unix()
	}
	entry := model.EdgeMetricsEntry{Timestamp: ts, Values: values}

	globalEdgeMetrics.mu.Lock()
	defer globalEdgeMetrics.mu.Unlock()

	buf := globalEdgeMetrics.entries[edgeID]
	buf = append(buf, entry)
	if len(buf) > edgeMetricsMaxPerEdge {
		buf = buf[len(buf)-edgeMetricsMaxPerEdge:]
	}
	globalEdgeMetrics.entries[edgeID] = buf
	return nil
}

// GetEdgeMetrics returns the most recent `limit` metrics entries for an edge device.
func (s *Store) GetEdgeMetrics(edgeID string, limit int) ([]model.EdgeMetricsEntry, error) {
	globalEdgeMetrics.mu.Lock()
	defer globalEdgeMetrics.mu.Unlock()

	buf := globalEdgeMetrics.entries[edgeID]
	if limit <= 0 || limit >= len(buf) {
		result := make([]model.EdgeMetricsEntry, len(buf))
		copy(result, buf)
		return result, nil
	}
	// Return the most recent `limit` entries.
	result := make([]model.EdgeMetricsEntry, limit)
	copy(result, buf[len(buf)-limit:])
	return result, nil
}

// EnqueueControl adds a control message to the edge's queue.
func (s *Store) EnqueueControl(edgeID string, req model.ControlMessageRequest) (string, error) {
	id := generateID("cm")
	paramsJSON := "null"
	if len(req.Params) > 0 {
		b, err := json.Marshal(req.Params)
		if err != nil {
			return "", fmt.Errorf("marshal params: %w", err)
		}
		paramsJSON = string(b)
	}
	_, err := s.db.Exec(
		`INSERT INTO edge_control_queue (id, edge_id, action, params, created_at)
		 VALUES (?, ?, ?, ?, datetime('now'))`,
		id, edgeID, req.Action, paramsJSON,
	)
	if err != nil {
		return "", fmt.Errorf("enqueue control: %w", err)
	}
	return id, nil
}

// DequeueControl returns all pending control messages for an edge and deletes them (auto-ack).
func (s *Store) DequeueControl(edgeID string) ([]model.EdgeControlMessage, error) {
	rows, err := s.db.Query(
		`SELECT id, action, params FROM edge_control_queue WHERE edge_id = ? ORDER BY rowid ASC`,
		edgeID,
	)
	if err != nil {
		return nil, fmt.Errorf("query control queue: %w", err)
	}
	defer rows.Close()

	var msgs []model.EdgeControlMessage
	var ids []string
	for rows.Next() {
		var msg model.EdgeControlMessage
		var paramsJSON string
		if err := rows.Scan(&msg.MessageID, &msg.Action, &paramsJSON); err != nil {
			return nil, fmt.Errorf("scan control message: %w", err)
		}
		if paramsJSON != "" && paramsJSON != "null" {
			_ = json.Unmarshal([]byte(paramsJSON), &msg.Params)
		}
		msgs = append(msgs, msg)
		ids = append(ids, msg.MessageID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate control queue: %w", err)
	}

	// Auto-ack: delete all returned messages.
	for _, id := range ids {
		_, _ = s.db.Exec(`DELETE FROM edge_control_queue WHERE id = ?`, id)
	}

	if msgs == nil {
		msgs = []model.EdgeControlMessage{}
	}
	return msgs, nil
}
