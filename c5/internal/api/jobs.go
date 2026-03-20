package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/piqsol/c4/c5/internal/model"
)

func (s *Server) handleJobSubmit(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeRequest[model.JobSubmitRequest](w, r, "POST")
	if !ok {
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

	// Override project_id from auth context and set submitted_by for audit trail.
	if pid := projectIDFromContext(r); pid != "" {
		req.ProjectID = pid
		req.SubmittedBy = pid
	}

	job, err := s.store.CreateJob(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Wake up any long-poll waiters in /v1/leases/acquire
	s.notifyJobAvailable()

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
	projectID := r.URL.Query().Get("project_id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	// Auth context overrides query parameter
	if pid := projectIDFromContext(r); pid != "" {
		projectID = pid
	}

	if limit == 0 {
		limit = 50
	}

	jobs, err := s.store.ListJobs(status, projectID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if jobs == nil {
		jobs = []*model.Job{}
	}

	writeJSON(w, jobs)
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

	// Check project ownership
	if pid := projectIDFromContext(r); pid != "" && job.ProjectID != pid {
		writeError(w, http.StatusForbidden, "access denied: job belongs to different project")
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

	// SSE broadcast for cancellation: consistent with completed/failed paths.
	if j, err := s.store.GetJob(jobID); err == nil {
		s.broadcastSSEEvent(j.ProjectID, "hub.job.cancelled", map[string]any{"job_id": jobID, "status": "CANCELLED"})
		s.maybeCompleteExperimentRun(r.Context(), j, model.StatusCancelled)
	} else {
		log.Printf("c5: handleJobCancel: GetJob(%s) failed after UpdateJobStatus: %v (SSE broadcast skipped)", jobID, err)
	}

	if s.eventPub.IsEnabled() {
		if err := s.eventPub.Publish("hub.job.cancelled", "c5", map[string]any{"job_id": jobID}); err != nil {
			log.Printf("c5: eventpub hub.job.cancelled: %v", err)
		}
	}

	writeJSON(w, map[string]any{
		"job_id": jobID,
		"status": "CANCELLED",
	})
}

func (s *Server) handleJobComplete(w http.ResponseWriter, r *http.Request, jobID string) {
	req, ok := decodeRequest[model.JobCompleteRequest](w, r, "POST")
	if !ok {
		return
	}

	status := model.StatusSucceeded
	exitCode := 0
	if req.ExitCode != nil {
		exitCode = *req.ExitCode
	}
	if req.Status == "FAILED" || exitCode != 0 {
		status = model.StatusFailed
	}

	if err := s.store.CompleteJob(jobID, status, exitCode); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Store structured result if provided (capability jobs).
	if req.Result != nil {
		if err := s.store.SetJobResult(jobID, req.Result); err != nil {
			log.Printf("c5: set job result for %s: %v", jobID, err)
		}
	}

	// Clean up lease
	s.store.DeleteLease(jobID)

	// Signal any MCP tools/call waiter that the job is done.
	s.handleWorkerComplete(jobID)

	// DAG orchestrator hook: advance DAG if this job was a DAG node
	s.onJobComplete(jobID, status, exitCode)

	// Fetch job once for both SSE broadcast, deploy rules and Dooray notification.
	// Fetched after state mutations are finalized so subscribers observe consistent state.
	var completedJob *model.Job
	if j, err := s.store.GetJob(jobID); err == nil {
		completedJob = j
		s.maybeCompleteExperimentRun(r.Context(), j, status)
	} else {
		log.Printf("c5: handleJobComplete: GetJob(%s) failed after CompleteJob: %v (SSE broadcast skipped)", jobID, err)
	}

	// Compute event type once for both SSE and EventBus paths.
	evType := "hub.job.completed"
	if status == model.StatusFailed {
		evType = "hub.job.failed"
	}

	// SSE broadcast: notify project-scoped subscribers immediately.
	if completedJob != nil {
		s.broadcastSSEEvent(completedJob.ProjectID, evType, map[string]any{
			"job_id":    jobID,
			"status":    string(status),
			"exit_code": exitCode,
		})
	}

	// Deploy rules: evaluate on job success and create deployments for matching rules
	if status == model.StatusSucceeded && completedJob != nil {
		tags := completedJob.Tags
		if tags == nil {
			tags = []string{}
		}
		if n, _ := s.store.EvaluateDeployRulesForJob(jobID, tags, completedJob.ProjectID); n > 0 {
			log.Printf("c5: deploy rules matched for job %s, created %d deployment(s)", jobID, n)
		}
	}

	// Affinity: record worker performance history (best-effort).
	if s.affinity != nil && completedJob != nil && completedJob.ProjectID != "" && completedJob.WorkerID != "" {
		if status == model.StatusSucceeded {
			tags := completedJob.Tags
			if tags == nil {
				tags = []string{}
			}
			if err := s.affinity.RecordSuccess(completedJob.WorkerID, completedJob.ProjectID, tags); err != nil {
				log.Printf("c5: affinity record success job=%s worker=%s: %v", jobID, completedJob.WorkerID, err)
			}
		} else if status == model.StatusFailed {
			if err := s.affinity.RecordFailure(completedJob.WorkerID, completedJob.ProjectID); err != nil {
				log.Printf("c5: affinity record failure job=%s worker=%s: %v", jobID, completedJob.WorkerID, err)
			}
		}
	}

	// Dooray completion notification (no-op if job was not from Dooray).
	s.notifyDoorayJobComplete(completedJob, status, exitCode)

	// Generic completion notification: alert via default webhook for any job
	// that wasn't already notified through Dooray channel integration.
	if completedJob != nil {
		if _, hasDoorayChannel := completedJob.Env["DOORAY_CHANNEL"]; !hasDoorayChannel {
			s.notifyJobCompletion(completedJob, status, exitCode)
		}
	}

	if s.eventPub.IsEnabled() {
		if err := s.eventPub.Publish(evType, "c5", map[string]any{
			"job_id":    jobID,
			"status":    string(status),
			"exit_code": exitCode,
		}); err != nil {
			log.Printf("c5: eventpub %s: %v", evType, err)
		}
	}

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
	metrics, _ := s.store.GetMetrics(jobID, 0, 0)
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
		ProjectID:   job.ProjectID,
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

// maybeCompleteExperimentRun completes the linked experiment run if the job has
// an exp_run_id set. It is a no-op when job is nil or exp_run_id is empty.
func (s *Server) maybeCompleteExperimentRun(ctx context.Context, job *model.Job, status model.JobStatus) {
	if job == nil || job.ExpRunID == "" {
		return
	}
	var finalMetric float64
	if job.BestMetric != nil {
		finalMetric = *job.BestMetric
	}
	// Map job statuses to experiment terminal statuses.
	expStatus := map[model.JobStatus]string{
		model.StatusSucceeded: "success",
		model.StatusFailed:    "failed",
		model.StatusCancelled: "cancelled",
	}[status]
	if expStatus == "" {
		expStatus = strings.ToLower(string(status))
	}
	if err := s.store.CompleteRun(ctx, job.ExpRunID, expStatus, finalMetric, ""); err != nil {
		log.Printf("c5: maybeCompleteExperimentRun: run_id=%s status=%s err=%v", job.ExpRunID, expStatus, err)
		return
	}
	log.Printf("c5: experiment run completed: run_id=%s status=%s metric=%f", job.ExpRunID, expStatus, finalMetric)
}

// notifyDoorayJobComplete sends a job completion notification to Dooray if the
// job was submitted via the Dooray integration (DOORAY_CHANNEL env set).
// It is a no-op when job is nil or lacks the DOORAY_CHANNEL env var.
func (s *Server) notifyDoorayJobComplete(job *model.Job, status model.JobStatus, exitCode int) {
	if job == nil {
		return
	}
	channelID, ok := job.Env["DOORAY_CHANNEL"]
	if !ok || channelID == "" {
		return // Not a Dooray-originated job.
	}
	webhookURL := s.resolveWebhookURL(channelID)
	if webhookURL == "" {
		log.Printf("c5: dooray complete notify: no webhook URL for channel %q", channelID)
		return
	}
	shortID := job.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}

	var text string
	if status == model.StatusSucceeded {
		var sb strings.Builder
		fmt.Fprintf(&sb, "✅ 실험 완료: %s (%s)\n", job.Name, shortID)
		metrics, _ := s.store.GetMetrics(job.ID, 0, 0)
		if len(metrics) > 0 {
			latest := metrics[len(metrics)-1].Metrics
			sb.WriteString("\n최종 메트릭:\n")
			for k, v := range latest {
				fmt.Fprintf(&sb, "• %s: %v\n", k, v)
			}
		}
		text = strings.TrimSpace(sb.String())
	} else {
		var sb strings.Builder
		fmt.Fprintf(&sb, "❌ 실험 실패: %s (%s) — exit %d\n", job.Name, shortID, exitCode)
		// Fetch actual last 3 lines (not the first 200 then tail).
		_, total, _, _ := s.store.GetLogs(job.ID, 0, 1)
		offset := 0
		if total > 3 {
			offset = total - 3
		}
		tail, _, _, _ := s.store.GetLogs(job.ID, offset, 3)
		if len(tail) > 0 {
			fmt.Fprintf(&sb, "\n마지막 로그:\n%s", strings.Join(tail, "\n"))
		}
		text = strings.TrimSpace(sb.String())
	}

	go postToDooray(context.Background(), webhookURL, text)
}

// notifyJobCompletion sends a completion notification via the default webhook URL
// for jobs that were NOT submitted through Dooray (those are handled separately).
// No-op if no default webhook URL is configured.
func (s *Server) notifyJobCompletion(job *model.Job, status model.JobStatus, exitCode int) {
	if s.doorayWebhookURL == "" {
		return
	}
	shortID := job.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	worker := job.WorkerID
	if worker == "" {
		worker = "(unknown)"
	}

	var text string
	if status == model.StatusSucceeded {
		text = fmt.Sprintf("✅ Job 완료: %s (%s)\n워커: %s\n커맨드: %s",
			job.Name, shortID, worker, job.Command)
		// Append metrics if available
		if metrics, err := s.store.GetMetrics(job.ID, 0, 0); err == nil && len(metrics) > 0 {
			latest := metrics[len(metrics)-1].Metrics
			var sb strings.Builder
			sb.WriteString(text)
			sb.WriteString("\n메트릭:")
			for k, v := range latest {
				fmt.Fprintf(&sb, "\n• %s: %v", k, v)
			}
			text = sb.String()
		}
	} else {
		text = fmt.Sprintf("❌ Job 실패: %s (%s)\n워커: %s\n종료코드: %d\n커맨드: %s",
			job.Name, shortID, worker, exitCode, job.Command)
	}

	go postToDooray(context.Background(), s.doorayWebhookURL, text)
}
