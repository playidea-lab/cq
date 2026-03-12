package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/piqsol/c4/c5/internal/store"
)

// handleExperimentCreateRun handles POST /v1/experiment/run
func (s *Server) handleExperimentCreateRun(w http.ResponseWriter, r *http.Request) {
	type request struct {
		Name       string `json:"name"`
		Capability string `json:"capability"`
	}
	req, ok := decodeRequest[request](w, r, "POST")
	if !ok {
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	runID, err := s.store.StartRun(r.Context(), req.Name, req.Capability)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{
		"run_id": runID,
		"status": "running",
	})
}

// handleExperimentCheckpoint handles POST /v1/experiment/checkpoint
func (s *Server) handleExperimentCheckpoint(w http.ResponseWriter, r *http.Request) {
	type request struct {
		RunID  string  `json:"run_id"`
		Metric float64 `json:"metric"`
		Path   string  `json:"path"`
	}
	req, ok := decodeRequest[request](w, r, "POST")
	if !ok {
		return
	}

	isBest, err := s.store.RecordCheckpoint(r.Context(), req.RunID, req.Metric, req.Path)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]any{"is_best": isBest})
}

// handleExperimentContinue handles GET /v1/experiment/continue?run_id=…
func (s *Server) handleExperimentContinue(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}
	runID := r.URL.Query().Get("run_id")
	if runID == "" {
		writeError(w, http.StatusBadRequest, "run_id is required")
		return
	}

	should, err := s.store.ShouldContinue(r.Context(), runID)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]any{"should_continue": should})
}

// handleExperimentComplete handles POST /v1/experiment/complete
func (s *Server) handleExperimentComplete(w http.ResponseWriter, r *http.Request) {
	type request struct {
		RunID       string  `json:"run_id"`
		Status      string  `json:"status"`
		FinalMetric float64 `json:"final_metric"`
		Summary     string  `json:"summary"`
	}
	req, ok := decodeRequest[request](w, r, "POST")
	if !ok {
		return
	}

	valid := map[string]bool{"success": true, "failed": true, "cancelled": true}
	if !valid[req.Status] {
		writeError(w, http.StatusBadRequest, "status must be one of: success, failed, cancelled")
		return
	}

	err := s.store.CompleteRun(r.Context(), req.RunID, req.Status, req.FinalMetric, req.Summary)
	if err != nil {
		if errors.Is(err, store.ErrRunNotFound) {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]any{"success": true})
}

// handleExperimentSearch handles GET /v1/experiment/search?query=…&limit=…
func (s *Server) handleExperimentSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}
	q := r.URL.Query().Get("query")
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}

	runs, err := s.store.SearchRuns(r.Context(), q, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if runs == nil {
		runs = []store.ExperimentRun{}
	}
	writeJSON(w, runs)
}
