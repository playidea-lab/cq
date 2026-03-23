package hub

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// newID generates a new UUID string for use as a primary key.
func newID() string {
	return uuid.NewString()
}

// =========================================================================
// DAG Models
// =========================================================================

// DAG represents a directed acyclic graph of job nodes.
type DAG struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	Tags         []string        `json:"tags,omitempty"`
	Status       string          `json:"status,omitempty"` // pending, running, completed, failed
	Nodes        []DAGNode       `json:"nodes,omitempty"`
	Dependencies []DAGDependency `json:"dependencies,omitempty"`
	CreatedAt    string          `json:"created_at,omitempty"`
	StartedAt    string          `json:"started_at,omitempty"`
	FinishedAt   string          `json:"finished_at,omitempty"`
}

// DAGNode represents a single executable node in a DAG.
type DAGNode struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Command     string            `json:"command"`
	WorkingDir  string            `json:"working_dir,omitempty"`
	Env         map[string]string `json:"environment,omitempty"`
	GPUCount    int               `json:"gpu_count,omitempty"`
	MaxRetries  int               `json:"max_retries,omitempty"`
	Status      string            `json:"status,omitempty"` // pending, running, succeeded, failed, skipped
	JobID       string            `json:"job_id,omitempty"` // linked Hub job ID when running
	StartedAt   string            `json:"started_at,omitempty"`
	FinishedAt  string            `json:"finished_at,omitempty"`
	ExitCode    *int              `json:"exit_code,omitempty"`
}

// DAGDependency represents a directed edge between two nodes.
type DAGDependency struct {
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
	Type     string `json:"dependency_type,omitempty"` // sequential, data_dependency, conditional
}

// DAGCreateRequest is the payload for creating a DAG.
type DAGCreateRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// DAGCreateResponse is the response from creating a DAG.
type DAGCreateResponse struct {
	DAGID  string `json:"dag_id"`
	Status string `json:"status"`
}

// DAGAddNodeRequest is the payload for adding a node to a DAG.
type DAGAddNodeRequest struct {
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	Description string            `json:"description,omitempty"`
	WorkingDir  string            `json:"working_dir,omitempty"`
	Env         map[string]string `json:"environment,omitempty"`
	GPUCount    int               `json:"gpu_count,omitempty"`
	MaxRetries  int               `json:"max_retries,omitempty"`
}

// DAGAddNodeResponse is the response from adding a node.
type DAGAddNodeResponse struct {
	NodeID string `json:"node_id"`
	Name   string `json:"name"`
}

// DAGAddDependencyRequest is the payload for adding a dependency.
type DAGAddDependencyRequest struct {
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
	Type     string `json:"dependency_type,omitempty"`
}

// DAGExecuteRequest is the payload for executing a DAG.
type DAGExecuteRequest struct {
	DryRun bool `json:"dry_run,omitempty"`
}

// DAGExecuteResponse is the response from executing a DAG.
type DAGExecuteResponse struct {
	DAGID      string   `json:"dag_id"`
	Status     string   `json:"status"`
	NodeOrder  []string `json:"node_order,omitempty"`
	Validation string   `json:"validation,omitempty"` // for dry_run: "valid" or error
	Errors     []string `json:"errors,omitempty"`     // validation errors
}

// DAGFromYAMLRequest is the payload for creating a DAG from YAML.
type DAGFromYAMLRequest struct {
	YAMLContent string `json:"yaml_content"`
}

// =========================================================================
// DAG Client Methods (Supabase PostgREST)
// =========================================================================

// CreateDAG inserts a new DAG into hub_dags.
func (c *Client) CreateDAG(req *DAGCreateRequest) (*DAGCreateResponse, error) {
	id := newID()
	row := map[string]any{
		"id":          id,
		"name":        req.Name,
		"description": req.Description,
		"status":      "pending",
		"project_id":  c.teamID,
	}
	var rows []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := c.supabasePost("/rest/v1/hub_dags", row, &rows); err != nil {
		return nil, fmt.Errorf("create dag: %w", err)
	}
	dagID := id
	status := "pending"
	if len(rows) > 0 {
		dagID = rows[0].ID
		status = rows[0].Status
	}
	return &DAGCreateResponse{DAGID: dagID, Status: status}, nil
}

// AddDAGNode inserts a node into hub_dag_nodes.
func (c *Client) AddDAGNode(dagID string, req *DAGAddNodeRequest) (*DAGAddNodeResponse, error) {
	nodeID := newID()
	row := map[string]any{
		"id":          nodeID,
		"dag_id":      dagID,
		"name":        req.Name,
		"command":     req.Command,
		"description": req.Description,
		"working_dir": req.WorkingDir,
		"gpu_count":   req.GPUCount,
		"max_retries": req.MaxRetries,
	}
	var rows []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.supabasePost("/rest/v1/hub_dag_nodes", row, &rows); err != nil {
		return nil, fmt.Errorf("add dag node: %w", err)
	}
	id := nodeID
	name := req.Name
	if len(rows) > 0 {
		id = rows[0].ID
		name = rows[0].Name
	}
	return &DAGAddNodeResponse{NodeID: id, Name: name}, nil
}

// AddDAGDependency inserts a dependency into hub_dag_dependencies.
func (c *Client) AddDAGDependency(dagID string, req *DAGAddDependencyRequest) error {
	depType := req.Type
	if depType == "" {
		depType = "sequential"
	}
	row := map[string]any{
		"dag_id":    dagID,
		"source_id": req.SourceID,
		"target_id": req.TargetID,
		"dep_type":  depType,
	}
	if err := c.supabasePost("/rest/v1/hub_dag_dependencies", row, nil); err != nil {
		return fmt.Errorf("add dag dependency: %w", err)
	}
	return nil
}

// topologicalSort performs a Kahn's algorithm topological sort on a set of
// nodes with their dependencies. nodes is the full list of node IDs; deps maps
// each node ID to the set of IDs it depends on (predecessors).
// Returns the sorted order, or an error if a cycle is detected.
func topologicalSort(nodes []string, deps map[string][]string) ([]string, error) {
	// Build in-degree map and adjacency list (successor direction).
	inDegree := make(map[string]int, len(nodes))
	successors := make(map[string][]string, len(nodes))
	for _, n := range nodes {
		if _, ok := inDegree[n]; !ok {
			inDegree[n] = 0
		}
		for _, pre := range deps[n] {
			successors[pre] = append(successors[pre], n)
			inDegree[n]++
		}
	}

	// Enqueue nodes with no predecessors (roots).
	queue := []string{}
	for _, n := range nodes {
		if inDegree[n] == 0 {
			queue = append(queue, n)
		}
	}

	order := make([]string, 0, len(nodes))
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		order = append(order, cur)
		for _, suc := range successors[cur] {
			inDegree[suc]--
			if inDegree[suc] == 0 {
				queue = append(queue, suc)
			}
		}
	}

	if len(order) != len(nodes) {
		return nil, fmt.Errorf("cycle detected in DAG")
	}
	return order, nil
}

// ExecuteDAG performs topological sort on DAG nodes, submits root nodes as Hub
// jobs, and transitions the DAG to "running" status.
func (c *Client) ExecuteDAG(dagID string, dryRun bool) (*DAGExecuteResponse, error) {
	// Fetch nodes.
	var nodes []DAGNode
	if err := c.supabaseGet("/rest/v1/hub_dag_nodes?dag_id=eq."+dagID, &nodes); err != nil {
		return nil, fmt.Errorf("execute dag: fetch nodes: %w", err)
	}

	// Fetch dependencies.
	var dbDeps []struct {
		SourceID string `json:"source_id"`
		TargetID string `json:"target_id"`
	}
	if err := c.supabaseGet("/rest/v1/hub_dag_dependencies?dag_id=eq."+dagID, &dbDeps); err != nil {
		return nil, fmt.Errorf("execute dag: fetch deps: %w", err)
	}

	// Build node ID list and predecessor map.
	nodeIDs := make([]string, 0, len(nodes))
	nodeByID := make(map[string]*DAGNode, len(nodes))
	for i := range nodes {
		nodeIDs = append(nodeIDs, nodes[i].ID)
		nodeByID[nodes[i].ID] = &nodes[i]
	}
	deps := make(map[string][]string, len(nodes))
	for _, d := range dbDeps {
		// d.TargetID depends on d.SourceID
		deps[d.TargetID] = append(deps[d.TargetID], d.SourceID)
	}

	// Topological sort — cycle detection happens here.
	order, err := topologicalSort(nodeIDs, deps)
	if err != nil {
		return nil, fmt.Errorf("execute dag: %w", err)
	}

	if dryRun {
		var rows []DAG
		if err := c.supabaseGet("/rest/v1/hub_dags?id=eq."+dagID, &rows); err != nil {
			return nil, fmt.Errorf("execute dag (dry run): %w", err)
		}
		if len(rows) == 0 {
			return nil, fmt.Errorf("execute dag: dag not found: %s", dagID)
		}
		return &DAGExecuteResponse{
			DAGID:      dagID,
			Status:     rows[0].Status,
			NodeOrder:  order,
			Validation: "valid",
		}, nil
	}

	// Submit root nodes (nodes with no predecessors) as Hub jobs.
	for _, nodeID := range order {
		if len(deps[nodeID]) > 0 {
			// Not a root — skip initial submission.
			continue
		}
		node := nodeByID[nodeID]
		submitResp, err := c.SubmitJob(&JobSubmitRequest{
			Name:        node.Name,
			Command:     node.Command,
			Workdir:     node.WorkingDir,
			RequiresGPU: node.GPUCount > 0,
		})
		if err != nil {
			return nil, fmt.Errorf("execute dag: submit root node %s: %w", nodeID, err)
		}
		// Patch hub_dag_nodes.job_id with the submitted job ID.
		patch := map[string]any{"job_id": submitResp.JobID}
		if err := c.supabasePatch("/rest/v1/hub_dag_nodes?id=eq."+nodeID, patch, nil); err != nil {
			return nil, fmt.Errorf("execute dag: patch node job_id %s: %w", nodeID, err)
		}
	}

	// Transition DAG to "running".
	body := map[string]any{"status": "running", "started_at": time.Now().UTC().Format(time.RFC3339)}
	if err := c.supabasePatch("/rest/v1/hub_dags?id=eq."+dagID, body, nil); err != nil {
		return nil, fmt.Errorf("execute dag: %w", err)
	}
	return &DAGExecuteResponse{
		DAGID:     dagID,
		Status:    "running",
		NodeOrder: order,
	}, nil
}

// GetDAGStatus returns a DAG with its nodes and dependencies.
func (c *Client) GetDAGStatus(dagID string) (*DAG, error) {
	var dags []DAG
	if err := c.supabaseGet("/rest/v1/hub_dags?id=eq."+dagID, &dags); err != nil {
		return nil, fmt.Errorf("get dag status: %w", err)
	}
	if len(dags) == 0 {
		return nil, fmt.Errorf("get dag status: not found: %s", dagID)
	}
	dag := &dags[0]

	// Fetch nodes
	var nodes []DAGNode
	if err := c.supabaseGet("/rest/v1/hub_dag_nodes?dag_id=eq."+dagID, &nodes); err != nil {
		return nil, fmt.Errorf("get dag nodes: %w", err)
	}
	dag.Nodes = nodes

	// Fetch dependencies
	var dbDeps []struct {
		SourceID string `json:"source_id"`
		TargetID string `json:"target_id"`
		DepType  string `json:"dep_type"`
	}
	if err := c.supabaseGet("/rest/v1/hub_dag_dependencies?dag_id=eq."+dagID, &dbDeps); err != nil {
		return nil, fmt.Errorf("get dag deps: %w", err)
	}
	for _, d := range dbDeps {
		dag.Dependencies = append(dag.Dependencies, DAGDependency{
			SourceID: d.SourceID,
			TargetID: d.TargetID,
			Type:     d.DepType,
		})
	}

	return dag, nil
}

// ListDAGs returns DAGs filtered by optional status and limit.
func (c *Client) ListDAGs(status string, limit int) ([]DAG, error) {
	path := "/rest/v1/hub_dags?order=created_at.desc"
	if status != "" {
		path += "&status=eq." + status
	}
	if limit > 0 {
		path += fmt.Sprintf("&limit=%d", limit)
	}

	var dags []DAG
	if err := c.supabaseGet(path, &dags); err != nil {
		return nil, fmt.Errorf("list dags: %w", err)
	}
	return dags, nil
}

// CreateDAGFromYAML creates a DAG from a YAML definition (stores raw YAML as description).
func (c *Client) CreateDAGFromYAML(yamlContent string) (*DAG, error) {
	id := newID()
	// Extract name from yaml content (simple heuristic: look for "name: " prefix)
	name := "dag-" + id[:8]
	for _, line := range strings.Split(yamlContent, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			n := strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			if n != "" {
				name = n
			}
			break
		}
	}

	row := map[string]any{
		"id":          id,
		"name":        name,
		"description": yamlContent,
		"status":      "pending",
		"project_id":  c.teamID,
	}
	var rows []DAG
	if err := c.supabasePost("/rest/v1/hub_dags", row, &rows); err != nil {
		return nil, fmt.Errorf("create dag from yaml: %w", err)
	}
	if len(rows) > 0 {
		return &rows[0], nil
	}
	return &DAG{ID: id, Name: name, Status: "pending"}, nil
}

// AdvanceDAG reports a job's completion to the DAG engine via Supabase RPC.
// The RPC automatically submits downstream nodes, handles retries, and marks
// the DAG completed or failed as appropriate.
func (c *Client) AdvanceDAG(jobID, status string, exitCode int) error {
	body := map[string]any{
		"p_job_id":    jobID,
		"p_status":    status,
		"p_exit_code": exitCode,
	}
	if err := c.supabaseRPC("advance_dag", body, nil); err != nil {
		return fmt.Errorf("advance dag: %w", err)
	}
	return nil
}

// joinParams joins query parameters with "&".
func joinParams(params []string) string {
	result := ""
	for i, p := range params {
		if i > 0 {
			result += "&"
		}
		result += p
	}
	return result
}
