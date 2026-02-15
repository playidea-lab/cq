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

	// Send history if requested
	if includeHistory {
		metrics, _ := s.store.GetMetrics(jobID, 0)
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

	// Poll for new metrics and status changes
	lastStep := -1
	if includeHistory {
		metrics, _ := s.store.GetMetrics(jobID, 0)
		if len(metrics) > 0 {
			lastStep = metrics[len(metrics)-1].Step
		}
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
			// Check for new metrics
			allMetrics, _ := s.store.GetMetrics(jobID, 0)
			for _, m := range allMetrics {
				if m.Step > lastStep {
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

func writeWSJSON(conn interface{ Write([]byte) (int, error) }, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return wsutil.WriteServerMessage(conn, ws.OpText, data)
}
