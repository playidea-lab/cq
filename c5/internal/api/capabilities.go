package api

import (
	"log"
	"net/http"
	"strings"

	"github.com/piqsol/c4/c5/internal/model"
)

// handleCapabilitiesList serves GET /v1/capabilities.
// Returns capabilities grouped by name with worker summaries.
// Project-scoped: non-master callers see only their project's capabilities.
func (s *Server) handleCapabilitiesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}

	projectID := r.URL.Query().Get("project_id")
	if pid := projectIDFromContext(r); pid != "" {
		projectID = pid
	}

	caps, err := s.store.ListCapabilities(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Optionally filter by name
	if name := r.URL.Query().Get("name"); name != "" {
		filtered := caps[:0]
		for _, c := range caps {
			if strings.EqualFold(c.Name, name) {
				filtered = append(filtered, c)
			}
		}
		caps = filtered
	}

	if caps == nil {
		caps = []model.CapabilityGroup{}
	}

	writeJSON(w, model.CapabilityListResponse{Capabilities: caps})
}

// handleCapabilitiesUpdate serves POST /v1/capabilities/update.
// Workers call this to refresh their capability set after registration.
func (s *Server) handleCapabilitiesUpdate(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeRequest[model.CapabilityUpdateRequest](w, r, "POST")
	if !ok {
		return
	}
	if req.WorkerID == "" {
		writeError(w, http.StatusBadRequest, "worker_id is required")
		return
	}

	projectID := ""
	if pid := projectIDFromContext(r); pid != "" {
		projectID = pid
	}

	// Verify the worker belongs to the authenticated project (non-master callers only).
	if projectID != "" {
		worker, err := s.store.GetWorker(req.WorkerID)
		if err != nil || worker.ProjectID != projectID {
			writeError(w, http.StatusForbidden, "worker not found in project")
			return
		}
	}

	if err := s.store.UpsertCapabilities(req.WorkerID, projectID, req.CapabilitySet); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]any{"updated": len(req.CapabilitySet), "worker_id": req.WorkerID})
}

// handleCapabilitiesInvoke serves POST /v1/capabilities/invoke.
// Creates a capability-typed job and queues it for a capable worker.
func (s *Server) handleCapabilitiesInvoke(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeRequest[model.CapabilityInvokeRequest](w, r, "POST")
	if !ok {
		return
	}

	if req.Capability == "" {
		writeError(w, http.StatusBadRequest, "capability is required")
		return
	}

	// Verify at least one online worker has this capability.
	projectID := req.ProjectID
	if pid := projectIDFromContext(r); pid != "" {
		projectID = pid
	}
	regs, err := s.store.FindCapability(req.Capability, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(regs) == 0 {
		writeError(w, http.StatusNotFound, "capability not found or no online workers available: "+req.Capability)
		return
	}

	// Determine command from capability registration (first match).
	command := regs[0].Command
	if command == "" {
		// No explicit command; worker will dispatch by capability name.
		command = "__capability__:" + req.Capability
	}

	name := req.Name
	if name == "" {
		name = req.Capability
	}

	jobReq := &model.JobSubmitRequest{
		Name:       name,
		Workdir:    ".",
		Command:    command,
		Memo:       req.Memo,
		Priority:   req.Priority,
		TimeoutSec: req.TimeoutSec,
		ProjectID:  projectID,
		Capability: req.Capability,
		Params:     req.Params,
	}

	job, err := s.store.CreateJob(jobReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.notifyJobAvailable()
	queuePos, _ := s.store.CountByStatus(model.StatusQueued)

	log.Printf("c5: capability invoke %q → job %s", req.Capability, job.ID)

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, model.CapabilityInvokeResponse{
		JobID:         job.ID,
		Status:        string(job.Status),
		QueuePosition: queuePos,
		Capability:    req.Capability,
	})
}
