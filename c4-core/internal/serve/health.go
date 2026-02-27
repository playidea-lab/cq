package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// HealthResponse is the JSON structure returned by the health endpoint.
type HealthResponse struct {
	Status     string                     `json:"status"`
	Components map[string]ComponentHealth `json:"components"`
}

// HealthHandler returns an http.HandlerFunc that reports aggregate health.
// The overall status is "ok" if all components are "ok", otherwise "degraded".
func HealthHandler(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		components := m.HealthMap()

		overall := "ok"
		for _, h := range components {
			if h.Status != "ok" && h.Status != "skipped" {
				overall = "degraded"
				break
			}
		}

		resp := HealthResponse{
			Status:     overall,
			Components: components,
		}

		w.Header().Set("Content-Type", "application/json")
		if overall != "ok" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			fmt.Fprintf(os.Stderr, "cq serve: health encode error: %v\n", err)
		}
	}
}
