//go:build hub

package hubhandler

import (
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp"
)

func registerHubDAGHandlers(reg *mcp.Registry, hubClient *hub.Client) {
	// c4_hub_dag_create — Create a new DAG
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_dag_create",
		Description: "Create a new experiment DAG (Directed Acyclic Graph) for multi-step ML pipelines",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":        map[string]any{"type": "string", "description": "DAG name (e.g. 'resnet-cifar10-pipeline')"},
				"description": map[string]any{"type": "string", "description": "DAG description"},
				"tags":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags for filtering"},
			},
			"required": []string{"name"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubDAGCreate(hubClient, raw)
	})

	// c4_hub_dag_add_node — Add a job node to a DAG
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_dag_add_node",
		Description: "Add a job node to a DAG. Each node is a single executable task (preprocess, train, eval)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"dag_id":      map[string]any{"type": "string", "description": "DAG ID to add node to"},
				"name":        map[string]any{"type": "string", "description": "Node name (e.g. 'preprocess', 'train')"},
				"command":     map[string]any{"type": "string", "description": "Command to execute (e.g. 'python train.py')"},
				"description": map[string]any{"type": "string", "description": "Node description"},
				"working_dir": map[string]any{"type": "string", "description": "Working directory"},
				"environment": map[string]any{"type": "object", "description": "Environment variables"},
				"gpu_count":   map[string]any{"type": "integer", "description": "Number of GPUs required (default: 0)"},
				"max_retries": map[string]any{"type": "integer", "description": "Maximum retry attempts on failure (default: 3)"},
			},
			"required": []string{"dag_id", "name", "command"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubDAGAddNode(hubClient, raw)
	})

	// c4_hub_dag_add_dep — Add a dependency between two DAG nodes
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_dag_add_dep",
		Description: "Add a dependency between two nodes. Target runs only after source completes",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"dag_id":          map[string]any{"type": "string", "description": "DAG ID"},
				"source_id":       map[string]any{"type": "string", "description": "Source node ID (runs first)"},
				"target_id":       map[string]any{"type": "string", "description": "Target node ID (runs after source)"},
				"dependency_type": map[string]any{"type": "string", "enum": []string{"sequential", "data_dependency", "conditional"}, "description": "Type of dependency (default: sequential)"},
			},
			"required": []string{"dag_id", "source_id", "target_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubDAGAddDep(hubClient, raw)
	})

	// c4_hub_dag_execute — Start DAG execution
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_dag_execute",
		Description: "Start execution of a DAG. Nodes run in dependency order with parallelism where possible",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"dag_id":  map[string]any{"type": "string", "description": "DAG ID to execute"},
				"dry_run": map[string]any{"type": "boolean", "description": "Validate without executing (default: false)"},
			},
			"required": []string{"dag_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubDAGExecute(hubClient, raw)
	})

	// c4_hub_dag_status — Get DAG execution status
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_dag_status",
		Description: "Get execution status of a DAG with per-node progress and timing",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"dag_id": map[string]any{"type": "string", "description": "DAG ID to query"},
			},
			"required": []string{"dag_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubDAGStatus(hubClient, raw)
	})

	// c4_hub_dag_list — List DAGs
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_dag_list",
		Description: "List DAGs with optional status filter",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{
					"type":        "string",
					"enum":        []string{"pending", "running", "completed", "failed"},
					"description": "Filter by status",
				},
				"limit": map[string]any{"type": "integer", "description": "Max results (default: 20)"},
			},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubDAGList(hubClient, raw)
	})

	// c4_hub_dag_from_yaml — Create DAG from YAML definition
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_dag_from_yaml",
		Description: "Create a complete DAG from a YAML definition. Define nodes, dependencies, and settings in one document",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"yaml_content": map[string]any{"type": "string", "description": "YAML content defining the DAG (nodes, dependencies, settings)"},
			},
			"required": []string{"yaml_content"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubDAGFromYAML(hubClient, raw)
	})
}

// =========================================================================
// DAG handler implementations
// =========================================================================

func handleHubDAGCreate(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	resp, err := client.CreateDAG(&hub.DAGCreateRequest{
		Name:        params.Name,
		Description: params.Description,
		Tags:        params.Tags,
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"dag_id": resp.DAGID,
		"status": resp.Status,
	}, nil
}

func handleHubDAGAddNode(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		DAGID       string            `json:"dag_id"`
		Name        string            `json:"name"`
		Command     string            `json:"command"`
		Description string            `json:"description"`
		WorkingDir  string            `json:"working_dir"`
		Env         map[string]string `json:"environment"`
		GPUCount    int               `json:"gpu_count"`
		MaxRetries  int               `json:"max_retries"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.DAGID == "" || params.Name == "" || params.Command == "" {
		return nil, fmt.Errorf("dag_id, name, and command are required")
	}

	resp, err := client.AddDAGNode(params.DAGID, &hub.DAGAddNodeRequest{
		Name:        params.Name,
		Command:     params.Command,
		Description: params.Description,
		WorkingDir:  params.WorkingDir,
		Env:         params.Env,
		GPUCount:    params.GPUCount,
		MaxRetries:  params.MaxRetries,
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"node_id": resp.NodeID,
		"name":    resp.Name,
		"dag_id":  params.DAGID,
	}, nil
}

func handleHubDAGAddDep(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		DAGID    string `json:"dag_id"`
		SourceID string `json:"source_id"`
		TargetID string `json:"target_id"`
		Type     string `json:"dependency_type"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.DAGID == "" || params.SourceID == "" || params.TargetID == "" {
		return nil, fmt.Errorf("dag_id, source_id, and target_id are required")
	}

	depType := params.Type
	if depType == "" {
		depType = "sequential"
	}

	if err := client.AddDAGDependency(params.DAGID, &hub.DAGAddDependencyRequest{
		SourceID: params.SourceID,
		TargetID: params.TargetID,
		Type:     depType,
	}); err != nil {
		return nil, err
	}

	return map[string]any{
		"added":     true,
		"dag_id":    params.DAGID,
		"source_id": params.SourceID,
		"target_id": params.TargetID,
		"type":      depType,
	}, nil
}

func handleHubDAGExecute(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		DAGID  string `json:"dag_id"`
		DryRun bool   `json:"dry_run"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.DAGID == "" {
		return nil, fmt.Errorf("dag_id is required")
	}

	resp, err := client.ExecuteDAG(params.DAGID, params.DryRun)
	if err != nil {
		return nil, err
	}

	if hubEventPub != nil && !params.DryRun {
		payload, _ := json.Marshal(map[string]any{
			"dag_id": resp.DAGID, "node_count": len(resp.NodeOrder),
		})
		hubEventPub.PublishAsync("hub.dag.executed", "c4.hub", payload, hubProjectID)
	}

	result := map[string]any{
		"dag_id": resp.DAGID,
		"status": resp.Status,
	}
	if len(resp.NodeOrder) > 0 {
		result["node_order"] = resp.NodeOrder
	}
	if resp.Validation != "" {
		result["validation"] = resp.Validation
	}
	if len(resp.Errors) > 0 {
		result["errors"] = resp.Errors
	}
	return result, nil
}

func handleHubDAGStatus(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		DAGID string `json:"dag_id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.DAGID == "" {
		return nil, fmt.Errorf("dag_id is required")
	}

	dag, err := client.GetDAGStatus(params.DAGID)
	if err != nil {
		return nil, err
	}

	// Build node summary
	nodes := make([]map[string]any, len(dag.Nodes))
	for i, n := range dag.Nodes {
		node := map[string]any{
			"id":     n.ID,
			"name":   n.Name,
			"status": n.Status,
		}
		if n.JobID != "" {
			node["job_id"] = n.JobID
		}
		if n.ExitCode != nil {
			node["exit_code"] = *n.ExitCode
		}
		nodes[i] = node
	}

	return map[string]any{
		"dag_id":       dag.ID,
		"name":         dag.Name,
		"status":       dag.Status,
		"nodes":        nodes,
		"dependencies": dag.Dependencies,
	}, nil
}

func handleHubDAGList(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		Status string `json:"status"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.Limit == 0 {
		params.Limit = 20
	}

	dags, err := client.ListDAGs(params.Status, params.Limit)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"dags":  dags,
		"count": len(dags),
	}, nil
}

func handleHubDAGFromYAML(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		YAMLContent string `json:"yaml_content"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.YAMLContent == "" {
		return nil, fmt.Errorf("yaml_content is required")
	}

	dag, err := client.CreateDAGFromYAML(params.YAMLContent)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"dag_id":       dag.ID,
		"name":         dag.Name,
		"status":       dag.Status,
		"nodes":        len(dag.Nodes),
		"dependencies": len(dag.Dependencies),
	}, nil
}
