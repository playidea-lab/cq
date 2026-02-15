package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/piqsol/c4/c5/internal/model"
)

func (s *Server) handleJobSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}

	var req model.JobSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

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

	queuePos, _ := s.store.CountByStatus(model.StatusQueued)

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, model.JobSubmitResponse{
		JobID:         job.ID,
		Status:        string(job.Status),
		QueuePosition: queuePos,
	})
}

func (s *Server) handleJobsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
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
	path := strings.TrimPrefix(r.URL.Path, "/v1/jobs/")
	parts := strings.SplitN(path, "/", 2)
	jobID := parts[0]

	if jobID == "" {
		writeError(w, http.StatusBadRequest, "job ID required")
		return
	}

	// Route sub-paths
	if len(parts) > 1 {
		switch parts[1] {
		case "cancel":
			s.handleJobCancel(w, r, jobID)
			return
		case "complete":
			s.handleJobComplete(w, r, jobID)
			return
		case "logs":
			if r.Method == "POST" {
				s.handleJobLogAppend(w, r, jobID)
				return
			}
			s.handleJobLogs(w, r, jobID)
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
		default:
			writeError(w, http.StatusNotFound, "unknown sub-path: "+parts[1])
			return
		}
	}

	// GET /v1/jobs/{id}
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}

	job, err := s.store.GetJob(jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, job)
}

func (s *Server) handleJobCancel(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}

	if err := s.store.UpdateJobStatus(jobID, model.StatusCancelled, ""); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Clean up lease if any
	s.store.DeleteLease(jobID)

	writeJSON(w, map[string]any{
		"job_id": jobID,
		"status": "CANCELLED",
	})
}

func (s *Server) handleJobComplete(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}

	var req model.JobCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	status := model.StatusSucceeded
	if req.Status == "FAILED" || req.ExitCode != 0 {
		status = model.StatusFailed
	}

	if err := s.store.CompleteJob(jobID, status, req.ExitCode); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Clean up lease
	s.store.DeleteLease(jobID)

	// DAG orchestrator hook: advance DAG if this job was a DAG node
	s.onJobComplete(jobID, status, req.ExitCode)

	writeJSON(w, map[string]any{
		"job_id": jobID,
		"status": string(status),
	})
}

func (s *Server) handleJobLogAppend(w http.ResponseWriter, r *http.Request, jobID string) {
	var req struct {
		Line   string `json:"line"`
		Stream string `json:"stream"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Stream == "" {
		req.Stream = "stdout"
	}
	if err := s.store.AppendLog(jobID, req.Line, req.Stream); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "appended"})
}

func (s *Server) handleJobLogs(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 200
	}

	lines, total, hasMore, err := s.store.GetLogs(jobID, offset, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, model.JobLogsResponse{
		JobID:      jobID,
		Lines:      lines,
		TotalLines: total,
		Offset:     offset,
		HasMore:    hasMore,
	})
}

func (s *Server) handleJobSummary(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}

	job, err := s.store.GetJob(jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Get last few log lines
	lines, _, _, _ := s.store.GetLogs(jobID, 0, 200)
	var logTail []string
	if len(lines) > 10 {
		logTail = lines[len(lines)-10:]
	} else {
		logTail = lines
	}

	// Get latest metrics
	metrics, _ := s.store.GetMetrics(jobID, 0)
	var latestMetrics map[string]any
	if len(metrics) > 0 {
		latestMetrics = metrics[len(metrics)-1].Metrics
	}

	resp := model.JobSummaryResponse{
		JobID:       job.ID,
		Name:        job.Name,
		Status:      string(job.Status),
		DurationSec: job.DurationSec(),
		ExitCode:    job.ExitCode,
		Metrics:     latestMetrics,
		LogTail:     logTail,
	}

	writeJSON(w, resp)
}

func (s *Server) handleJobEstimate(w http.ResponseWriter, r *http.Request, jobID string) {
	if r.Method != "GET" {
		methodNotAllowed(w)
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
		methodNotAllowed(w)
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

	newJob, err := s.store.CreateJob(&model.JobSubmitRequest{
		Name:        job.Name,
		Workdir:     job.Workdir,
		Command:     job.Command,
		Env:         job.Env,
		Tags:        job.Tags,
		RequiresGPU: job.RequiresGPU,
		Priority:    job.Priority,
		ExpID:       job.ExpID,
		Memo:        fmt.Sprintf("retry of %s", jobID),
		TimeoutSec:  job.TimeoutSec,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, model.JobRetryResponse{
		NewJobID:      newJob.ID,
		Status:        string(newJob.Status),
		OriginalJobID: jobID,
	})
}
