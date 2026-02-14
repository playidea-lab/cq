package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Server is the HTTP API server for the daemon.
type Server struct {
	store      *Store
	scheduler  *Scheduler
	gpuMonitor *GpuMonitor
	estimator  *Estimator
	startTime  time.Time
	version    string
	cancelFunc context.CancelFunc // for /daemon/stop
	mux        *http.ServeMux
}

// ServerConfig holds server settings.
type ServerConfig struct {
	Store      *Store
	Scheduler  *Scheduler
	GpuMonitor *GpuMonitor
	Version    string
	CancelFunc context.CancelFunc
}

// NewServer creates an HTTP API server.
func NewServer(cfg ServerConfig) *Server {
	s := &Server{
		store:      cfg.Store,
		scheduler:  cfg.Scheduler,
		gpuMonitor: cfg.GpuMonitor,
		estimator:  NewEstimator(cfg.Store),
		startTime:  time.Now(),
		version:    cfg.Version,
		cancelFunc: cfg.CancelFunc,
		mux:        http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// Handler returns the HTTP handler for use with httptest or http.Server.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/jobs/submit", s.handleJobSubmit)
	s.mux.HandleFunc("/jobs", s.handleJobs)
	s.mux.HandleFunc("/jobs/", s.handleJobByID)
	s.mux.HandleFunc("/stats/queue", s.handleQueueStats)
	s.mux.HandleFunc("/gpu/status", s.handleGPUStatus)
	s.mux.HandleFunc("/daemon/stop", s.handleDaemonStop)
}

// =========================================================================
// Handlers
// =========================================================================

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{
		"status":            "ok",
		"version":           s.version,
		"uptime_seconds":    int(time.Since(s.startTime).Seconds()),
		"scheduler_running": s.scheduler != nil,
	})
}

func (s *Server) handleJobSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req JobSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Validate required fields
	if req.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}
	if req.Workdir == "" {
		req.Workdir = "."
	}
	if req.Name == "" {
		req.Name = "untitled"
	}

	job, err := s.store.CreateJob(&req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Calculate queue position
	queuePos, _ := s.store.CountByStatus(StatusQueued)

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, JobSubmitResponse{
		JobID:         job.ID,
		Status:        string(job.Status),
		QueuePosition: queuePos,
	})
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	if limit == 0 {
		limit = 50
	}

	jobs, err := s.store.ListJobs(status, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	stats, _ := s.store.GetQueueStats()
	runningCount := 0
	queuedCount := 0
	if stats != nil {
		runningCount = stats.Running
		queuedCount = stats.Queued
	}

	writeJSON(w, map[string]any{
		"jobs":          jobs,
		"total_count":   len(jobs),
		"running_count": runningCount,
		"queued_count":  queuedCount,
	})
}

func (s *Server) handleJobByID(w http.ResponseWriter, r *http.Request) {
	// Parse job ID from path: /jobs/{id} or /jobs/{id}/logs or /jobs/{id}/cancel
	path := strings.TrimPrefix(r.URL.Path, "/jobs/")
	parts := strings.SplitN(path, "/", 2)
	jobID := parts[0]

	if jobID == "" {
		writeError(w, http.StatusBadRequest, "job ID required")
		return
	}

	// Route sub-paths
	if len(parts) > 1 {
		switch parts[1] {
		case "logs":
			s.handleJobLogs(w, r, jobID)
			return
		case "cancel":
			s.handleJobCancel(w, r, jobID)
			return
		case "complete":
			s.handleJobComplete(w, r, jobID)
			return
		case "summary":
			s.handleJobSummary(w, r, jobID)
			return
		case "estimate":
			s.handleJobEstimate(w, r, jobID)
			return
		case "retry":
			s.handleJobRetry(w, r, jobID)
			return
		}
	}

	// GET /jobs/{id}
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	job, err := s.store.GetJob(jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, job)
}

func (s *Server) handleJobLogs(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 100
	}

	lines, total, hasMore, err := s.scheduler.GetJobLog(jobID, offset, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]any{
		"job_id":      jobID,
		"lines":       lines,
		"total_lines": total,
		"offset":      offset,
		"has_more":    hasMore,
	})
}

func (s *Server) handleJobCancel(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.scheduler.Cancel(jobID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, map[string]any{
		"job_id": jobID,
		"status": "CANCELLED",
	})
}

func (s *Server) handleJobComplete(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req JobCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	status := StatusSucceeded
	if req.Status == "FAILED" || req.ExitCode != 0 {
		status = StatusFailed
	}

	if err := s.store.CompleteJob(jobID, status, req.ExitCode); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, map[string]any{
		"job_id": jobID,
		"status": string(status),
	})
}

func (s *Server) handleJobSummary(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	job, err := s.store.GetJob(jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Get last few log lines
	lines, _, _, _ := s.scheduler.GetJobLog(jobID, 0, 0)
	var logTail []string
	if len(lines) > 10 {
		logTail = lines[len(lines)-10:]
	} else {
		logTail = lines
	}

	resp := map[string]any{
		"job_id":  job.ID,
		"name":    job.Name,
		"status":  string(job.Status),
		"log_tail": logTail,
	}
	if dur := job.DurationSec(); dur != nil {
		resp["duration_seconds"] = *dur
	}
	if job.ExitCode != nil {
		resp["exit_code"] = *job.ExitCode
	}

	writeJSON(w, resp)
}

func (s *Server) handleJobEstimate(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	job, err := s.store.GetJob(jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	result := s.estimator.EstimateWithQueue(job)
	writeJSON(w, result)
}

func (s *Server) handleJobRetry(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	job, err := s.store.GetJob(jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if !job.Status.IsTerminal() {
		writeError(w, http.StatusBadRequest, "can only retry terminal jobs")
		return
	}

	// Create a new job with the same spec
	newJob, err := s.store.CreateJob(&JobSubmitRequest{
		Name:        job.Name,
		Workdir:     job.Workdir,
		Command:     job.Command,
		Env:         job.Env,
		Tags:        job.Tags,
		RequiresGPU: job.RequiresGPU,
		GPUCount:    job.GPUCount,
		Priority:    job.Priority,
		ExpID:       job.ExpID,
		Memo:        fmt.Sprintf("retry of %s", jobID),
		TimeoutSec:  job.TimeoutSec,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]any{
		"new_job_id":       newJob.ID,
		"status":           string(newJob.Status),
		"original_job_id":  jobID,
	})
}

func (s *Server) handleQueueStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := s.store.GetQueueStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleGPUStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.gpuMonitor == nil || !s.gpuMonitor.IsAvailable() {
		writeJSON(w, map[string]any{
			"available": false,
			"gpus":      []any{},
		})
		return
	}

	gpus, err := s.gpuMonitor.GetAllGPUs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]any{
		"available": true,
		"gpus":      gpus,
		"count":     len(gpus),
	})
}

func (s *Server) handleDaemonStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, map[string]string{
		"status":  "stopping",
		"message": "daemon shutting down",
	})

	cancel := s.cancelFunc
	if cancel != nil {
		log.Println("daemon: stop requested via API")
		go func() {
			time.Sleep(100 * time.Millisecond) // let response flush
			cancel()
		}()
	}
}

// =========================================================================
// Helpers
// =========================================================================

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
