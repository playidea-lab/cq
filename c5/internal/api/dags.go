package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/piqsol/c4/c5/internal/model"

	"gopkg.in/yaml.v3"
)

func (s *Server) handleDAGCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}

	var req model.DAGCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	dag, err := s.store.CreateDAG(&req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, model.DAGCreateResponse{
		DAGID:  dag.ID,
		Status: dag.Status,
	})
}

func (s *Server) handleDAGsList(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		s.handleDAGCreate(w, r)
		return
	}
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}

	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 20
	}

	dags, err := s.store.ListDAGs(status, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if dags == nil {
		dags = []model.DAG{}
	}
	writeJSON(w, dags)
}

func (s *Server) handleDAGByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/dags/")
	parts := strings.SplitN(path, "/", 2)
	dagID := parts[0]

	if dagID == "" {
		writeError(w, http.StatusBadRequest, "DAG ID required")
		return
	}

	// Route sub-paths
	if len(parts) > 1 {
		switch parts[1] {
		case "nodes":
			s.handleDAGAddNode(w, r, dagID)
			return
		case "dependencies":
			s.handleDAGAddDependency(w, r, dagID)
			return
		case "execute":
			s.handleDAGExecute(w, r, dagID)
			return
		case "status":
			s.handleDAGStatus(w, r, dagID)
			return
		default:
			writeError(w, http.StatusNotFound, "unknown sub-path: "+parts[1])
			return
		}
	}

	// GET /v1/dags/{id}
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}

	dag, err := s.store.GetDAG(dagID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, dag)
}

func (s *Server) handleDAGAddNode(w http.ResponseWriter, r *http.Request, dagID string) {
	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}

	var req model.DAGAddNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.Name == "" || req.Command == "" {
		writeError(w, http.StatusBadRequest, "name and command are required")
		return
	}
	if req.MaxRetries == 0 {
		req.MaxRetries = 3
	}

	node, err := s.store.AddDAGNode(dagID, &req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, model.DAGAddNodeResponse{
		NodeID: node.ID,
		Name:   node.Name,
	})
}

func (s *Server) handleDAGAddDependency(w http.ResponseWriter, r *http.Request, dagID string) {
	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}

	var req model.DAGAddDependencyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.SourceID == "" || req.TargetID == "" {
		writeError(w, http.StatusBadRequest, "source_id and target_id are required")
		return
	}

	if err := s.store.AddDAGDependency(dagID, &req); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]string{
		"status": "added",
	})
}

func (s *Server) handleDAGExecute(w http.ResponseWriter, r *http.Request, dagID string) {
	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}

	var req model.DAGExecuteRequest
	json.NewDecoder(r.Body).Decode(&req) // optional body

	// Validate DAG with topological sort
	nodeOrder, err := s.store.TopologicalSort(dagID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.DryRun {
		writeJSON(w, model.DAGExecuteResponse{
			DAGID:      dagID,
			Status:     "validated",
			NodeOrder:  nodeOrder,
			Validation: "OK",
		})
		return
	}

	// Start DAG execution
	if err := s.store.UpdateDAGStatus(dagID, "running"); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Queue ready nodes (root nodes with no dependencies)
	created, err := s.store.AdvanceDAG(dagID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	log.Printf("c5: DAG %s started, %d root jobs queued", dagID, created)

	writeJSON(w, model.DAGExecuteResponse{
		DAGID:     dagID,
		Status:    "running",
		NodeOrder: nodeOrder,
	})
}

func (s *Server) handleDAGStatus(w http.ResponseWriter, r *http.Request, dagID string) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}

	dag, err := s.store.GetDAG(dagID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, dag)
}

// handleDAGFromYAML creates a complete DAG from a YAML definition.
func (s *Server) handleDAGFromYAML(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}

	var req model.DAGFromYAMLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.YAMLContent == "" {
		writeError(w, http.StatusBadRequest, "yaml_content is required")
		return
	}

	// Parse YAML
	var def dagYAMLDef
	if err := yaml.Unmarshal([]byte(req.YAMLContent), &def); err != nil {
		writeError(w, http.StatusBadRequest, "invalid YAML: "+err.Error())
		return
	}

	if def.Name == "" {
		writeError(w, http.StatusBadRequest, "YAML must include 'name'")
		return
	}

	// Create DAG
	dag, err := s.store.CreateDAG(&model.DAGCreateRequest{
		Name:        def.Name,
		Description: def.Description,
		Tags:        def.Tags,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Add nodes
	nodeNameToID := make(map[string]string)
	for _, n := range def.Nodes {
		node, err := s.store.AddDAGNode(dag.ID, &model.DAGAddNodeRequest{
			Name:        n.Name,
			Command:     n.Command,
			Description: n.Description,
			WorkingDir:  n.WorkingDir,
			Env:         n.Env,
			GPUCount:    n.GPUCount,
			MaxRetries:  n.MaxRetries,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "add node "+n.Name+": "+err.Error())
			return
		}
		nodeNameToID[n.Name] = node.ID
	}

	// Add dependencies
	for _, d := range def.Dependencies {
		sourceID, ok1 := nodeNameToID[d.Source]
		targetID, ok2 := nodeNameToID[d.Target]
		if !ok1 || !ok2 {
			writeError(w, http.StatusBadRequest, "dependency references unknown node")
			return
		}
		if err := s.store.AddDAGDependency(dag.ID, &model.DAGAddDependencyRequest{
			SourceID: sourceID,
			TargetID: targetID,
			Type:     d.Type,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Return full DAG struct (hub.Client expects DAG, not DAGCreateResponse)
	fullDAG, err := s.store.GetDAG(dag.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, fullDAG)
}

// dagYAMLDef is the internal representation of a YAML DAG definition.
type dagYAMLDef struct {
	Name         string          `yaml:"name"`
	Description  string          `yaml:"description"`
	Tags         []string        `yaml:"tags"`
	Nodes        []dagYAMLNode   `yaml:"nodes"`
	Dependencies []dagYAMLDepDef `yaml:"dependencies"`
}

type dagYAMLNode struct {
	Name        string            `yaml:"name"`
	Command     string            `yaml:"command"`
	Description string            `yaml:"description"`
	WorkingDir  string            `yaml:"working_dir"`
	Env         map[string]string `yaml:"environment"`
	GPUCount    int               `yaml:"gpu_count"`
	MaxRetries  int               `yaml:"max_retries"`
}

type dagYAMLDepDef struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
	Type   string `yaml:"type"`
}

// onJobComplete is called after a job is completed to advance any linked DAG.
func (s *Server) onJobComplete(jobID string, status model.JobStatus, exitCode int) {
	dagID, err := s.store.UpdateDAGNodeFromJob(jobID, status, exitCode)
	if err != nil {
		log.Printf("c5: DAG node update error for job %s: %v", jobID, err)
		return
	}
	if dagID == "" {
		return // not a DAG-linked job
	}

	created, err := s.store.AdvanceDAG(dagID)
	if err != nil {
		log.Printf("c5: DAG advance error for %s: %v", dagID, err)
		return
	}
	if created > 0 {
		log.Printf("c5: DAG %s advanced, %d new jobs queued", dagID, created)
	}
}
