package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

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

	// hub.Client sends {"capabilities": {"hostname":"h1","gpu_count":2,...}}
	// Extract fields from capabilities map if top-level fields are empty
	if req.Hostname == "" && len(req.Capabilities) > 0 {
		if v, ok := req.Capabilities["hostname"]; ok {
			if s, ok := v.(string); ok {
				req.Hostname = s
			}
		}
		if req.GPUCount == 0 {
			if v, ok := req.Capabilities["gpu_count"]; ok {
				switch n := v.(type) {
				case float64:
					req.GPUCount = int(n)
				case int:
					req.GPUCount = n
				}
			}
		}
		if req.GPUModel == "" {
			if v, ok := req.Capabilities["gpu_model"]; ok {
				if s, ok := v.(string); ok {
					req.GPUModel = s
				}
			}
		}
	}

	if req.Hostname == "" {
		writeError(w, http.StatusBadRequest, "hostname is required")
		return
	}

	// Override project_id from auth context
	if pid := projectIDFromContext(r); pid != "" {
		req.ProjectID = pid
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

	projectID := r.URL.Query().Get("project_id")

	// Auth context overrides query parameter
	if pid := projectIDFromContext(r); pid != "" {
		projectID = pid
	}

	workers, err := s.store.ListWorkers(projectID)
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

	lease, job, err := s.store.AcquireLease(req.WorkerID, hasGPU, worker.ProjectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if lease == nil {
		// No GPU jobs? Try non-GPU jobs
		if hasGPU {
			lease, job, err = s.store.AcquireLease(req.WorkerID, false, worker.ProjectID)
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

	if s.eventPub.IsEnabled() {
		if err := s.eventPub.Publish("hub.job.started", "c5", map[string]any{
			"job_id":    job.ID,
			"worker_id": req.WorkerID,
		}); err != nil {
			log.Printf("c5: eventpub hub.job.started: %v", err)
		}
	}

	// Generate presigned GET URLs for input artifacts (5-second timeout per call).
	var inputPresignedURLs []model.InputPresignedArtifact
	for _, art := range job.InputArtifacts {
		type result struct {
			url       string
			expiresAt time.Time
			err       error
		}
		ch := make(chan result, 1)
		artPath := art.Path
		go func() {
			u, exp, e := s.storage.PresignedURL(artPath, "GET", 3600)
			ch <- result{u, exp, e}
		}()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		var res result
		select {
		case res = <-ch:
		case <-ctx.Done():
			res.err = ctx.Err()
		}
		cancel()
		if res.err != nil {
			log.Printf("c5: presigned URL failed for %s: %v", art.Path, res.err)
			continue
		}
		inputPresignedURLs = append(inputPresignedURLs, model.InputPresignedArtifact{
			Path:      art.Path,
			LocalPath: art.LocalPath,
			URL:       res.url,
			ExpiresAt: res.expiresAt.UTC().Format(time.RFC3339),
		})
	}

	writeJSON(w, model.LeaseAcquireResponse{
		JobID:              job.ID,
		LeaseID:            lease.ID,
		Job:                *job,
		InputPresignedURLs: inputPresignedURLs,
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
