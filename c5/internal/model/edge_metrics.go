package model

// EdgeMetricsRequest is the payload for POST /v1/edges/{id}/metrics.
type EdgeMetricsRequest struct {
	Values    map[string]float64 `json:"values"`
	Timestamp int64              `json:"timestamp,omitempty"`
}

// EdgeMetricsEntry is a single timestamped metrics snapshot.
type EdgeMetricsEntry struct {
	Timestamp int64              `json:"timestamp"`
	Values    map[string]float64 `json:"values"`
}

// EdgeMetricsListResponse is returned from GET /v1/edges/{id}/metrics.
type EdgeMetricsListResponse struct {
	EdgeID  string             `json:"edge_id"`
	Metrics []EdgeMetricsEntry `json:"metrics"`
}
