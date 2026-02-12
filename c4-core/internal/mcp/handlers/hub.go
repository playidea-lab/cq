package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterHubHandlers registers c4_hub_* MCP tools.
func RegisterHubHandlers(reg *mcp.Registry, hubClient *hub.Client) {
	// c4_hub_submit — Submit a job to the Hub queue
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_submit",
		Description: "Submit a job to the PiQ Hub queue for remote GPU execution",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":         map[string]any{"type": "string", "description": "Job name"},
				"workdir":      map[string]any{"type": "string", "description": "Working directory on the worker"},
				"command":      map[string]any{"type": "string", "description": "Command to execute"},
				"env":          map[string]any{"type": "object", "description": "Environment variables"},
				"tags":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Job tags"},
				"requires_gpu": map[string]any{"type": "boolean", "description": "Whether GPU is required (default: true)"},
				"priority":     map[string]any{"type": "integer", "description": "Priority (-100 to 100, default: 0)"},
				"exp_id":       map[string]any{"type": "string", "description": "Experiment ID to link"},
				"memo":         map[string]any{"type": "string", "description": "Experiment memo/hypothesis"},
				"timeout_sec":  map[string]any{"type": "integer", "description": "Timeout in seconds"},
			},
			"required": []string{"name", "workdir", "command"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubSubmit(hubClient, raw)
	})

	// c4_hub_status — Get job status
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_status",
		Description: "Get status of a Hub job",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id": map[string]any{"type": "string", "description": "Job ID"},
			},
			"required": []string{"job_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubStatus(hubClient, raw)
	})

	// c4_hub_list — List jobs
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_list",
		Description: "List Hub jobs with optional status filter",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{
					"type":        "string",
					"enum":        []string{"QUEUED", "RUNNING", "SUCCEEDED", "FAILED", "CANCELLED"},
					"description": "Filter by status",
				},
				"limit": map[string]any{"type": "integer", "description": "Max results (default: 50)"},
			},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubList(hubClient, raw)
	})

	// c4_hub_cancel — Cancel a job
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_cancel",
		Description: "Cancel a queued or running Hub job",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id": map[string]any{"type": "string", "description": "Job ID to cancel"},
			},
			"required": []string{"job_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubCancel(hubClient, raw)
	})

	// c4_hub_workers — List workers
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_workers",
		Description: "List workers connected to the PiQ Hub with GPU status",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(_ json.RawMessage) (any, error) {
		return handleHubWorkers(hubClient)
	})

	// c4_hub_stats — Queue statistics
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_stats",
		Description: "Get Hub queue statistics (queued/running/succeeded/failed counts)",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(_ json.RawMessage) (any, error) {
		return handleHubStats(hubClient)
	})

	// c4_hub_metrics — Get metrics for a job
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_metrics",
		Description: "Get training metrics for a Hub job",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id": map[string]any{"type": "string", "description": "Job ID"},
				"limit":  map[string]any{"type": "integer", "description": "Max metric points (default: 100)"},
			},
			"required": []string{"job_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubMetrics(hubClient, raw)
	})

	// c4_hub_log_metrics — Log metrics for a job
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_log_metrics",
		Description: "Log training metrics for a running Hub job",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id":  map[string]any{"type": "string", "description": "Job ID"},
				"step":    map[string]any{"type": "integer", "description": "Training step (0-indexed)"},
				"metrics": map[string]any{"type": "object", "description": "Metric name-value pairs (e.g. {\"loss\": 0.5})"},
			},
			"required": []string{"job_id", "step", "metrics"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubLogMetrics(hubClient, raw)
	})

	// c4_hub_upload — Upload artifact for a job
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_upload",
		Description: "Upload a local file as a Hub job artifact (presigned URL + SHA256 verification)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id":       map[string]any{"type": "string", "description": "Job ID"},
				"local_path":   map[string]any{"type": "string", "description": "Local file path to upload"},
				"storage_path": map[string]any{"type": "string", "description": "Storage path (e.g. outputs/model.pt)"},
			},
			"required": []string{"job_id", "local_path", "storage_path"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubUpload(hubClient, raw)
	})

	// c4_hub_download — Download artifact from a job
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_download",
		Description: "Download a Hub job artifact to a local file",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id":     map[string]any{"type": "string", "description": "Job ID"},
				"name":       map[string]any{"type": "string", "description": "Artifact name"},
				"local_path": map[string]any{"type": "string", "description": "Local destination path"},
			},
			"required": []string{"job_id", "name", "local_path"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubDownload(hubClient, raw)
	})

	// =====================================================================
	// DAG Orchestration Tools
	// =====================================================================

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
// Handler implementations
// =========================================================================

func handleHubSubmit(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		Name        string            `json:"name"`
		Workdir     string            `json:"workdir"`
		Command     string            `json:"command"`
		Env         map[string]string `json:"env"`
		Tags        []string          `json:"tags"`
		RequiresGPU *bool             `json:"requires_gpu"`
		Priority    int               `json:"priority"`
		ExpID       string            `json:"exp_id"`
		Memo        string            `json:"memo"`
		TimeoutSec  int               `json:"timeout_sec"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.Name == "" || params.Workdir == "" || params.Command == "" {
		return nil, fmt.Errorf("name, workdir, and command are required")
	}

	requiresGPU := true
	if params.RequiresGPU != nil {
		requiresGPU = *params.RequiresGPU
	}

	resp, err := client.SubmitJob(&hub.JobSubmitRequest{
		Name:        params.Name,
		Workdir:     params.Workdir,
		Command:     params.Command,
		Env:         params.Env,
		Tags:        params.Tags,
		RequiresGPU: requiresGPU,
		Priority:    params.Priority,
		ExpID:       params.ExpID,
		Memo:        params.Memo,
		TimeoutSec:  params.TimeoutSec,
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"job_id":         resp.JobID,
		"status":         resp.Status,
		"queue_position": resp.QueuePosition,
	}, nil
}

func handleHubStatus(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.JobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}

	job, err := client.GetJob(params.JobID)
	if err != nil {
		return nil, err
	}

	return job, nil
}

func handleHubList(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		Status string `json:"status"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.Limit == 0 {
		params.Limit = 50
	}

	jobs, err := client.ListJobs(params.Status, params.Limit)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"jobs":  jobs,
		"count": len(jobs),
	}, nil
}

func handleHubCancel(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.JobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}

	if err := client.CancelJob(params.JobID); err != nil {
		return nil, err
	}

	return map[string]any{
		"cancelled": true,
		"job_id":    params.JobID,
	}, nil
}

func handleHubWorkers(client *hub.Client) (any, error) {
	workers, err := client.ListWorkers()
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"workers": workers,
		"count":   len(workers),
	}, nil
}

func handleHubStats(client *hub.Client) (any, error) {
	stats, err := client.GetQueueStats()
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"queued":    stats.Queued,
		"running":   stats.Running,
		"succeeded": stats.Succeeded,
		"failed":    stats.Failed,
		"cancelled": stats.Cancelled,
		"total":     stats.Queued + stats.Running + stats.Succeeded + stats.Failed + stats.Cancelled,
	}, nil
}

func handleHubMetrics(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		JobID string `json:"job_id"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.JobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	if params.Limit == 0 {
		params.Limit = 100
	}

	resp, err := client.GetMetrics(params.JobID, params.Limit)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"job_id":      resp.JobID,
		"metrics":     resp.Metrics,
		"total_steps": resp.TotalSteps,
	}, nil
}

func handleHubLogMetrics(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		JobID   string         `json:"job_id"`
		Step    int            `json:"step"`
		Metrics map[string]any `json:"metrics"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.JobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}
	if params.Metrics == nil {
		return nil, fmt.Errorf("metrics is required")
	}

	if err := client.LogMetrics(params.JobID, params.Step, params.Metrics); err != nil {
		return nil, err
	}

	return map[string]any{
		"logged": true,
		"job_id": params.JobID,
		"step":   params.Step,
	}, nil
}

func handleHubUpload(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		JobID       string `json:"job_id"`
		LocalPath   string `json:"local_path"`
		StoragePath string `json:"storage_path"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.JobID == "" || params.LocalPath == "" || params.StoragePath == "" {
		return nil, fmt.Errorf("job_id, local_path, and storage_path are required")
	}

	resp, err := client.UploadArtifact(params.JobID, params.StoragePath, params.LocalPath)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"uploaded":    true,
		"artifact_id": resp.ArtifactID,
		"job_id":      params.JobID,
		"path":        params.StoragePath,
	}, nil
}

func handleHubDownload(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		JobID     string `json:"job_id"`
		Name      string `json:"name"`
		LocalPath string `json:"local_path"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.JobID == "" || params.Name == "" || params.LocalPath == "" {
		return nil, fmt.Errorf("job_id, name, and local_path are required")
	}

	if err := client.DownloadArtifact(params.JobID, params.Name, params.LocalPath); err != nil {
		return nil, err
	}

	return map[string]any{
		"downloaded": true,
		"job_id":     params.JobID,
		"name":       params.Name,
		"local_path": params.LocalPath,
	}, nil
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
