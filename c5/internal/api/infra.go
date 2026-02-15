package api

import (
	"net/http"
	"time"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, map[string]any{
		"status":         "ok",
		"version":        s.version,
		"uptime_seconds": int(time.Since(s.startTime).Seconds()),
	})
}

func (s *Server) handleQueueStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}

	stats, err := s.store.GetQueueStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, stats)
}
