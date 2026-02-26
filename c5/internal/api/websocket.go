package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/piqsol/c4/c5/internal/model"
)

const (
	wsPollInterval = 2 * time.Second
	wsReadTimeout  = 30 * time.Second
)

// handleWSMetrics handles WebSocket connections for real-time metrics streaming.
// Path: /ws/metrics/{job_id}?include_history=true
func (s *Server) handleWSMetrics(w http.ResponseWriter, r *http.Request) {
	// Support both /ws/metrics/{id} and /v1/ws/metrics/{id}
	jobID := strings.TrimPrefix(r.URL.Path, "/v1/ws/metrics/")
	if jobID == r.URL.Path {
		jobID = strings.TrimPrefix(r.URL.Path, "/ws/metrics/")
	}
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "job_id required")
		return
	}

	// Check job exists
	job, err := s.store.GetJob(jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Upgrade to WebSocket
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		log.Printf("c5: ws upgrade error for job %s: %v", jobID, err)
		return
	}
	defer conn.Close()

	includeHistory := r.URL.Query().Get("include_history") == "true"

	// Determine initial lastStep before sending history.
	// For include_history=false, initialise to the current DB max step so the
	// poll loop only returns rows inserted after the connection was established.
	lastStep := -1
	if !includeHistory {
		existing, _ := s.store.GetMetrics(jobID, 0, 0)
		if len(existing) > 0 {
			lastStep = existing[len(existing)-1].Step
		}
	}

	// Send history if requested
	if includeHistory {
		metrics, _ := s.store.GetMetrics(jobID, 0, 0)
		for _, m := range metrics {
			msg := model.MetricMessage{
				Type:    "history",
				JobID:   jobID,
				Step:    m.Step,
				Metrics: m.Metrics,
			}
			if err := writeWSJSON(conn, msg); err != nil {
				return
			}
			if m.Step > lastStep {
				lastStep = m.Step
			}
		}
	}

	// Send current status
	if err := writeWSJSON(conn, model.MetricMessage{
		Type:   "status",
		JobID:  jobID,
		Status: string(job.Status),
	}); err != nil {
		return
	}

	// If already terminal, close after sending status
	if job.Status.IsTerminal() {
		return
	}

	ticker := time.NewTicker(wsPollInterval)
	defer ticker.Stop()

	// Start a goroutine to detect client disconnect
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
			_, _, err := wsutil.ReadClientData(conn)
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			// Incremental fetch: only rows with step > lastStep
			newMetrics, _ := s.store.GetMetrics(jobID, lastStep, 0)
			for _, m := range newMetrics {
				msg := model.MetricMessage{
					Type:    "metric",
					JobID:   jobID,
					Step:    m.Step,
					Metrics: m.Metrics,
				}
				if err := writeWSJSON(conn, msg); err != nil {
					return
				}
				lastStep = m.Step
			}

			// Check job status
			currentJob, err := s.store.GetJob(jobID)
			if err != nil {
				writeWSJSON(conn, model.MetricMessage{
					Type:  "error",
					JobID: jobID,
					Error: err.Error(),
				})
				return
			}

			if currentJob.Status != job.Status {
				job = currentJob
				writeWSJSON(conn, model.MetricMessage{
					Type:   "status",
					JobID:  jobID,
					Status: string(job.Status),
				})

				// Terminal status → close
				if job.Status.IsTerminal() {
					return
				}
			}
		}
	}
}

const wsWriteTimeout = 10 * time.Second

func writeWSJSON(conn interface {
	Write([]byte) (int, error)
	SetWriteDeadline(time.Time) error
}, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
	return wsutil.WriteServerMessage(conn, ws.OpText, data)
}
