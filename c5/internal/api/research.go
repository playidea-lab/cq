package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// handleResearchState routes GET and PUT for /v1/research/state.
func (s *Server) handleResearchState(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleResearchStateGet(w, r)
	case http.MethodPut:
		s.handleResearchStatePut(w, r)
	default:
		methodNotAllowed(w)
	}
}

// handleResearchStateLock routes POST and DELETE for /v1/research/state/lock.
func (s *Server) handleResearchStateLock(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleResearchStateLockAcquire(w, r)
	case http.MethodDelete:
		s.handleResearchStateLockRelease(w, r)
	default:
		methodNotAllowed(w)
	}
}

// handleResearchStateGet handles GET /v1/research/state.
// On first call it upserts a default row and returns it.
func (s *Server) handleResearchStateGet(w http.ResponseWriter, r *http.Request) {
	projectID := projectIDFromContext(r)
	state, err := s.store.GetOrCreateResearchState(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, state)
}

// researchStatePutRequest is the body for PUT /v1/research/state.
type researchStatePutRequest struct {
	Round   int    `json:"round"`
	Phase   string `json:"phase"`
	Version int    `json:"version"`
}

// handleResearchStatePut handles PUT /v1/research/state.
func (s *Server) handleResearchStatePut(w http.ResponseWriter, r *http.Request) {
	var req researchStatePutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	projectID := projectIDFromContext(r)
	state, ok, err := s.store.UpdateResearchState(projectID, req.Round, req.Phase, req.Version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusConflict, "version conflict")
		return
	}
	writeJSON(w, state)
}

// researchStateLockRequest is the body for POST /v1/research/state/lock.
type researchStateLockRequest struct {
	WorkerID string `json:"worker_id"`
	TTLSec   int    `json:"ttl_sec"`
}

// handleResearchStateLockAcquire handles POST /v1/research/state/lock.
func (s *Server) handleResearchStateLockAcquire(w http.ResponseWriter, r *http.Request) {
	var req researchStateLockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.WorkerID == "" {
		writeError(w, http.StatusBadRequest, "worker_id is required")
		return
	}
	ttl := req.TTLSec
	if ttl <= 0 {
		ttl = 60
	}

	projectID := projectIDFromContext(r)
	acquired, holder, err := s.store.AcquireResearchLock(projectID, req.WorkerID, ttl)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{
		"acquired":    acquired,
		"lock_holder": holder,
	})
}

// researchStateLockReleaseRequest is the body for DELETE /v1/research/state/lock.
type researchStateLockReleaseRequest struct {
	WorkerID string `json:"worker_id"`
}

// handleResearchStateLockRelease handles DELETE /v1/research/state/lock.
func (s *Server) handleResearchStateLockRelease(w http.ResponseWriter, r *http.Request) {
	var req researchStateLockReleaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.WorkerID == "" {
		writeError(w, http.StatusBadRequest, "worker_id is required")
		return
	}

	projectID := projectIDFromContext(r)
	if err := s.store.ReleaseResearchLock(projectID, req.WorkerID); err != nil {
		if strings.Contains(err.Error(), "not held") {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"released": true})
}
