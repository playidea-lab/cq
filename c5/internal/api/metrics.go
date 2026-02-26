package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/piqsol/c4/c5/internal/model"
)

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// Parse job ID from path: /v1/metrics/{job_id}
	jobID := strings.TrimPrefix(r.URL.Path, "/v1/metrics/")
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "job_id required")
		return
	}

	switch r.Method {
	case "POST":
		s.handleMetricsLog(w, r, jobID)
	case "GET":
		s.handleMetricsGet(w, r, jobID)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleMetricsLog(w http.ResponseWriter, r *http.Request, jobID string) {
	var req model.MetricsLogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := s.store.InsertMetric(jobID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]any{
		"status": "recorded",
		"step":   req.Step,
	})
}

func (s *Server) handleMetricsGet(w http.ResponseWriter, r *http.Request, jobID string) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 100
	}

	entries, err := s.store.GetMetrics(jobID, 0, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if entries == nil {
		entries = []model.MetricEntry{}
	}

	writeJSON(w, model.MetricsResponse{
		JobID:      jobID,
		Metrics:    entries,
		TotalSteps: len(entries),
	})
}
