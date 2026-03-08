package api

import (
	"net/http"
	"strings"

	"github.com/piqsol/c4/c5/internal/model"
)

// handleEdgeControlPost handles POST /v1/edges/{id}/control (enqueue a control message)
func (s *Server) handleEdgeControlPost(w http.ResponseWriter, r *http.Request) {
	edgeID := edgeIDFromControlPath(r.URL.Path)
	if edgeID == "" {
		writeError(w, http.StatusBadRequest, "edge_id is required")
		return
	}

	req, ok := decodeRequest[model.ControlMessageRequest](w, r, "POST")
	if !ok {
		return
	}
	if req.Action == "" {
		writeError(w, http.StatusBadRequest, "action is required")
		return
	}

	msgID, err := s.store.EnqueueControl(edgeID, *req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, model.ControlEnqueueResponse{
		MessageID: msgID,
		Status:    "queued",
	})
}

// handleEdgeControlGet handles GET /v1/edges/{id}/control (dequeue control messages, auto-ack)
func (s *Server) handleEdgeControlGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}

	edgeID := edgeIDFromControlPath(r.URL.Path)
	if edgeID == "" {
		writeError(w, http.StatusBadRequest, "edge_id is required")
		return
	}

	msgs, err := s.store.DequeueControl(edgeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, msgs)
}

// edgeIDFromControlPath extracts edge ID from /v1/edges/{id}/control
func edgeIDFromControlPath(p string) string {
	p = strings.TrimPrefix(p, "/v1/edges/")
	p = strings.TrimSuffix(p, "/control")
	return strings.TrimSuffix(p, "/")
}
