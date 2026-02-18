package api

import (
	"encoding/json"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/piqsol/c4/c5/internal/model"
)

// =========================================================================
// Edges
// =========================================================================

func (s *Server) handleEdgeRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}

	var req model.EdgeRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	edge, err := s.store.RegisterEdge(&req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, model.EdgeRegisterResponse{
		EdgeID: edge.ID,
	})
}

func (s *Server) handleEdgeHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}

	var req model.EdgeHeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.EdgeID == "" {
		writeError(w, http.StatusBadRequest, "edge_id is required")
		return
	}

	if err := s.store.UpdateEdgeHeartbeat(&req); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, model.HeartbeatResponse{Acknowledged: true})
}

func (s *Server) handleEdgesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}

	edges, err := s.store.ListEdges()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if edges == nil {
		edges = []model.Edge{}
	}
	writeJSON(w, edges)
}

func (s *Server) handleEdgeByID(w http.ResponseWriter, r *http.Request) {
	edgeID := strings.TrimPrefix(r.URL.Path, "/v1/edges/")
	if edgeID == "" {
		writeError(w, http.StatusBadRequest, "edge ID required")
		return
	}

	switch r.Method {
	case "GET":
		edge, err := s.store.GetEdge(edgeID)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, edge)

	case "DELETE":
		if err := s.store.RemoveEdge(edgeID); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, map[string]string{"status": "removed"})

	default:
		methodNotAllowed(w)
	}
}

// =========================================================================
// Deploy Rules
// =========================================================================

func (s *Server) handleDeployRuleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}

	var req model.DeployRuleCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.Trigger == "" || req.EdgeFilter == "" || req.ArtifactPattern == "" {
		writeError(w, http.StatusBadRequest, "trigger, edge_filter, and artifact_pattern are required")
		return
	}

	rule, err := s.store.CreateDeployRule(&req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, model.DeployRuleCreateResponse{
		RuleID: rule.ID,
	})
}

func (s *Server) handleDeployRulesList(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		s.handleDeployRuleCreate(w, r)
		return
	}
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}

	rules, err := s.store.ListDeployRules()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if rules == nil {
		rules = []model.DeployRule{}
	}
	writeJSON(w, rules)
}

func (s *Server) handleDeployRuleByID(w http.ResponseWriter, r *http.Request) {
	ruleID := strings.TrimPrefix(r.URL.Path, "/v1/deploy/rules/")
	if ruleID == "" {
		writeError(w, http.StatusBadRequest, "rule ID required")
		return
	}

	if r.Method != "DELETE" {
		methodNotAllowed(w)
		return
	}

	if err := s.store.DeleteDeployRule(ruleID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, map[string]string{"status": "deleted"})
}

// =========================================================================
// Deployments
// =========================================================================

func (s *Server) handleDeployTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}

	var req model.DeployTriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.JobID == "" {
		writeError(w, http.StatusBadRequest, "job_id is required")
		return
	}

	// Resolve target edges
	var edges []model.Edge
	var err error

	if len(req.EdgeIDs) > 0 {
		// Explicit edge IDs
		for _, eid := range req.EdgeIDs {
			e, err := s.store.GetEdge(eid)
			if err != nil {
				writeError(w, http.StatusBadRequest, "edge not found: "+eid)
				return
			}
			edges = append(edges, *e)
		}
	} else if req.EdgeFilter != "" {
		edges, err = s.store.MatchEdges(req.EdgeFilter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		writeError(w, http.StatusBadRequest, "edge_ids or edge_filter required")
		return
	}

	if len(edges) == 0 {
		writeError(w, http.StatusBadRequest, "no matching edges found")
		return
	}

	deployment, err := s.store.CreateDeployment(&req, edges)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, model.DeployTriggerResponse{
		DeployID:    deployment.ID,
		Status:      deployment.Status,
		TargetCount: len(edges),
	})
}

func (s *Server) handleDeployStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}

	deployID := strings.TrimPrefix(r.URL.Path, "/v1/deploy/")
	// hub.Client sends GET /deploy/{id}/status — strip /status suffix for compat
	deployID = strings.TrimSuffix(deployID, "/status")
	if deployID == "" {
		writeError(w, http.StatusBadRequest, "deploy ID required")
		return
	}

	deployment, err := s.store.GetDeployment(deployID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, deployment)
}

func (s *Server) handleDeployList(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 50
	}

	deployments, err := s.store.ListDeployments(limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if deployments == nil {
		deployments = []model.Deployment{}
	}
	writeJSON(w, deployments)
}

func (s *Server) handleDeployAssignments(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}
	edgeID := strings.TrimPrefix(r.URL.Path, "/v1/deploy/assignments/")
	edgeID = strings.TrimSuffix(edgeID, "/")
	if edgeID == "" {
		writeError(w, http.StatusBadRequest, "edge_id is required")
		return
	}

	list, err := s.store.ListPendingAssignmentsForEdge(edgeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []model.PendingAssignment{}
	}

	resp := make([]model.DeployAssignmentResponse, 0, len(list))
	for _, a := range list {
		item := model.DeployAssignmentResponse{
			DeployID:        a.DeployID,
			JobID:           a.JobID,
			ArtifactPattern: a.ArtifactPattern,
			PostCommand:     a.PostCommand,
		}
		arts, err := s.store.ListArtifacts(a.JobID)
		if err == nil && len(arts) > 0 && a.ArtifactPattern != "" {
			pattern := a.ArtifactPattern
			for _, art := range arts {
				if !art.Confirmed {
					continue
				}
				ok, _ := path.Match(pattern, art.Path)
				if !ok {
					continue
				}
				url, _, err := s.storage.PresignedURL(art.Path, "GET", 3600)
				if err != nil {
					continue
				}
				item.Artifacts = append(item.Artifacts, model.DeployAssignmentArtifact{Path: art.Path, URL: url})
			}
		}
		resp = append(resp, item)
	}
	writeJSON(w, resp)
}

func (s *Server) handleDeployTargetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}
	var req model.DeployTargetStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.DeployID == "" || req.EdgeID == "" || req.Status == "" {
		writeError(w, http.StatusBadRequest, "deploy_id, edge_id, and status are required")
		return
	}
	errMsg := req.Error
	if err := s.store.UpdateDeployTarget(req.DeployID, req.EdgeID, req.Status, errMsg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"ok": "true"})
}
