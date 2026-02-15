package api

import (
	"encoding/json"
	"net/http"

	"github.com/piqsol/c4/c5/internal/model"
)

func (s *Server) handleWorkerRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}

	var req model.WorkerRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.Hostname == "" {
		writeError(w, http.StatusBadRequest, "hostname is required")
		return
	}

	worker, err := s.store.RegisterWorker(&req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, model.WorkerRegisterResponse{
		WorkerID: worker.ID,
	})
}

func (s *Server) handleWorkerHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}

	var req model.HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Fallback: hub.Client sends worker_id in X-Worker-ID header
	if req.WorkerID == "" {
		req.WorkerID = r.Header.Get("X-Worker-ID")
	}
	if req.WorkerID == "" {
		writeError(w, http.StatusBadRequest, "worker_id is required")
		return
	}

	if err := s.store.UpdateHeartbeat(&req); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, model.HeartbeatResponse{
		Acknowledged: true,
	})
}

func (s *Server) handleWorkersList(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}

	workers, err := s.store.ListWorkers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if workers == nil {
		workers = []*model.Worker{}
	}

	writeJSON(w, workers)
}

func (s *Server) handleLeaseAcquire(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}

	var req model.LeaseAcquireRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Fallback: hub.Client sends worker_id in X-Worker-ID header
	if req.WorkerID == "" {
		req.WorkerID = r.Header.Get("X-Worker-ID")
	}
	if req.WorkerID == "" {
		writeError(w, http.StatusBadRequest, "worker_id is required")
		return
	}

	// Check if worker has GPU capability
	worker, err := s.store.GetWorker(req.WorkerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "worker not found: "+req.WorkerID)
		return
	}

	hasGPU := worker.GPUCount > 0

	lease, job, err := s.store.AcquireLease(req.WorkerID, hasGPU)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if lease == nil {
		// No GPU jobs? Try non-GPU jobs
		if hasGPU {
			lease, job, err = s.store.AcquireLease(req.WorkerID, false)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	}

	if lease == nil {
		// No jobs available
		writeJSON(w, map[string]any{
			"job_id":   nil,
			"lease_id": nil,
			"message":  "no jobs available",
		})
		return
	}

	writeJSON(w, model.LeaseAcquireResponse{
		JobID:   job.ID,
		LeaseID: lease.ID,
		Job:     *job,
	})
}

func (s *Server) handleLeaseRenew(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}

	var req model.LeaseRenewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.LeaseID == "" || req.WorkerID == "" {
		writeError(w, http.StatusBadRequest, "lease_id and worker_id are required")
		return
	}

	newExpiry, err := s.store.RenewLease(req.LeaseID, req.WorkerID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, model.LeaseRenewResponse{
		Renewed:      true,
		NewExpiresAt: newExpiry.Format("2006-01-02T15:04:05Z"),
	})
}
