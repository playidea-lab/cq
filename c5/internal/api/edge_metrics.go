package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/piqsol/c4/c5/internal/model"
)

// handleEdgeMetricsPost handles POST /v1/edges/{id}/metrics
func (s *Server) handleEdgeMetricsPost(w http.ResponseWriter, r *http.Request) {
	edgeID := edgeIDFromMetricsPath(r.URL.Path)
	if edgeID == "" {
		writeError(w, http.StatusBadRequest, "edge_id is required")
		return
	}

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

// edgeIDFromMetricsPath extracts edge ID from /v1/edges/{id}/metrics
func edgeIDFromMetricsPath(p string) string {
	p = strings.TrimPrefix(p, "/v1/edges/")
	p = strings.TrimSuffix(p, "/metrics")
	return strings.TrimSuffix(p, "/")
}
