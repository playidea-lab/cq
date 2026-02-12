package hub

import "fmt"

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

// DAGCreateRequest is the payload for POST /v1/dags.
type DAGCreateRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// DAGCreateResponse is the response from POST /v1/dags.
type DAGCreateResponse struct {
	DAGID  string `json:"dag_id"`
	Status string `json:"status"`
}

// DAGAddNodeRequest is the payload for POST /v1/dags/{id}/nodes.
type DAGAddNodeRequest struct {
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	Description string            `json:"description,omitempty"`
	WorkingDir  string            `json:"working_dir,omitempty"`
	Env         map[string]string `json:"environment,omitempty"`
	GPUCount    int               `json:"gpu_count,omitempty"`
	MaxRetries  int               `json:"max_retries,omitempty"`
}

// DAGAddNodeResponse is the response from POST /v1/dags/{id}/nodes.
type DAGAddNodeResponse struct {
	NodeID string `json:"node_id"`
	Name   string `json:"name"`
}

// DAGAddDependencyRequest is the payload for POST /v1/dags/{id}/dependencies.
type DAGAddDependencyRequest struct {
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
	Type     string `json:"dependency_type,omitempty"`
}

// DAGExecuteRequest is the payload for POST /v1/dags/{id}/execute.
type DAGExecuteRequest struct {
	DryRun bool `json:"dry_run,omitempty"`
}

// DAGExecuteResponse is the response from POST /v1/dags/{id}/execute.
type DAGExecuteResponse struct {
	DAGID      string   `json:"dag_id"`
	Status     string   `json:"status"`
	NodeOrder  []string `json:"node_order,omitempty"`  // topological execution order
	Validation string   `json:"validation,omitempty"`  // for dry_run: "valid" or error
	Errors     []string `json:"errors,omitempty"`      // validation errors
}

// DAGFromYAMLRequest is the payload for POST /v1/dags/from-yaml.
type DAGFromYAMLRequest struct {
	YAMLContent string `json:"yaml_content"`
}

// =========================================================================
// DAG Client Methods
// =========================================================================

// CreateDAG creates a new DAG.
func (c *Client) CreateDAG(req *DAGCreateRequest) (*DAGCreateResponse, error) {
	var resp DAGCreateResponse
	if err := c.post("/v1/dags", req, &resp); err != nil {
		return nil, fmt.Errorf("create dag: %w", err)
	}
	return &resp, nil
}

// AddDAGNode adds a job node to a DAG.
func (c *Client) AddDAGNode(dagID string, req *DAGAddNodeRequest) (*DAGAddNodeResponse, error) {
	var resp DAGAddNodeResponse
	if err := c.post("/v1/dags/"+dagID+"/nodes", req, &resp); err != nil {
		return nil, fmt.Errorf("add dag node: %w", err)
	}
	return &resp, nil
}

// AddDAGDependency adds a dependency between two nodes.
func (c *Client) AddDAGDependency(dagID string, req *DAGAddDependencyRequest) error {
	if err := c.post("/v1/dags/"+dagID+"/dependencies", req, nil); err != nil {
		return fmt.Errorf("add dag dependency: %w", err)
	}
	return nil
}

// ExecuteDAG starts execution of a DAG.
func (c *Client) ExecuteDAG(dagID string, dryRun bool) (*DAGExecuteResponse, error) {
	var resp DAGExecuteResponse
	body := &DAGExecuteRequest{DryRun: dryRun}
	if err := c.post("/v1/dags/"+dagID+"/execute", body, &resp); err != nil {
		return nil, fmt.Errorf("execute dag: %w", err)
	}
	return &resp, nil
}

// GetDAGStatus returns the execution status of a DAG with node details.
func (c *Client) GetDAGStatus(dagID string) (*DAG, error) {
	var dag DAG
	if err := c.get("/v1/dags/"+dagID+"/status", &dag); err != nil {
		return nil, fmt.Errorf("get dag status: %w", err)
	}
	return &dag, nil
}

// ListDAGs returns DAGs with optional status filter.
func (c *Client) ListDAGs(status string, limit int) ([]DAG, error) {
	path := "/v1/dags"
	params := []string{}
	if status != "" {
		params = append(params, "status="+status)
	}
	if limit > 0 {
		params = append(params, fmt.Sprintf("limit=%d", limit))
	}
	if len(params) > 0 {
		path += "?" + joinParams(params)
	}

	var dags []DAG
	if err := c.get(path, &dags); err != nil {
		return nil, fmt.Errorf("list dags: %w", err)
	}
	return dags, nil
}

// CreateDAGFromYAML creates a complete DAG from a YAML definition.
func (c *Client) CreateDAGFromYAML(yamlContent string) (*DAG, error) {
	var dag DAG
	body := &DAGFromYAMLRequest{YAMLContent: yamlContent}
	if err := c.post("/v1/dags/from-yaml", body, &dag); err != nil {
		return nil, fmt.Errorf("create dag from yaml: %w", err)
	}
	return &dag, nil
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
