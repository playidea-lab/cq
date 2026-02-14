package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/changmin/c4-core/internal/hub"
	"github.com/changmin/c4-core/internal/mcp"
)

func registerHubEdgeHandlers(reg *mcp.Registry, hubClient *hub.Client) {
	// c4_hub_edge_register — Register an edge device
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_edge_register",
		Description: "Register an edge device for artifact deployment (Jetson, RPi, server, etc.)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":       map[string]any{"type": "string", "description": "Edge device name (e.g. 'jetson-factory-1')"},
				"tags":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags for filtering (e.g. ['onnx','arm64'])"},
				"arch":       map[string]any{"type": "string", "description": "Architecture (arm64, amd64)"},
				"runtime":    map[string]any{"type": "string", "description": "Inference runtime (onnx, tflite, tensorrt)"},
				"storage_gb": map[string]any{"type": "number", "description": "Available storage in GB"},
			},
			"required": []string{"name"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubEdgeRegister(hubClient, raw)
	})

	// c4_hub_edge_list — List registered edge devices
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_edge_list",
		Description: "List registered edge devices with status and capabilities",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(_ json.RawMessage) (any, error) {
		return handleHubEdgeList(hubClient)
	})

	// c4_hub_deploy_rule — Create a deployment rule (auto-deploy on trigger)
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_deploy_rule",
		Description: "Create an auto-deployment rule. When trigger matches, artifacts deploy to matching edges",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":             map[string]any{"type": "string", "description": "Rule name"},
				"trigger":          map[string]any{"type": "string", "description": "Trigger condition (e.g. 'job_tag:production', 'dag_complete:pipeline-*')"},
				"edge_filter":      map[string]any{"type": "string", "description": "Edge filter (e.g. 'tag:onnx', 'name:jetson-*')"},
				"artifact_pattern": map[string]any{"type": "string", "description": "Artifact glob pattern (e.g. 'outputs/*.onnx')"},
				"post_command":     map[string]any{"type": "string", "description": "Command to run on edge after deployment (e.g. 'systemctl restart inference')"},
			},
			"required": []string{"trigger", "edge_filter", "artifact_pattern"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubDeployRule(hubClient, raw)
	})

	// c4_hub_deploy — Manually trigger artifact deployment to edges
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_deploy",
		Description: "Manually trigger deployment of job artifacts to edge devices",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id":           map[string]any{"type": "string", "description": "Source job ID with artifacts"},
				"artifact_pattern": map[string]any{"type": "string", "description": "Artifact glob pattern (default: all)"},
				"edge_filter":      map[string]any{"type": "string", "description": "Edge filter expression"},
				"edge_ids":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Explicit edge IDs (overrides filter)"},
				"post_command":     map[string]any{"type": "string", "description": "Post-deploy command on edges"},
			},
			"required": []string{"job_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubDeploy(hubClient, raw)
	})

	// c4_hub_deploy_status — Check deployment status
	reg.Register(mcp.ToolSchema{
		Name:        "c4_hub_deploy_status",
		Description: "Get deployment status with per-edge progress",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"deploy_id": map[string]any{"type": "string", "description": "Deployment ID"},
			},
			"required": []string{"deploy_id"},
		},
	}, func(raw json.RawMessage) (any, error) {
		return handleHubDeployStatus(hubClient, raw)
	})
}

// =========================================================================
// Edge + Deploy handler implementations
// =========================================================================

func handleHubEdgeRegister(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		Name      string   `json:"name"`
		Tags      []string `json:"tags"`
		Arch      string   `json:"arch"`
		Runtime   string   `json:"runtime"`
		StorageGB float64  `json:"storage_gb"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	caps := map[string]any{}
	if params.Arch != "" {
		caps["arch"] = params.Arch
	}
	if params.Runtime != "" {
		caps["runtime"] = params.Runtime
	}
	if params.StorageGB > 0 {
		caps["storage_gb"] = params.StorageGB
	}

	edgeID, err := client.RegisterEdge(params.Name, params.Tags, caps)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"edge_id": edgeID,
		"name":    params.Name,
	}, nil
}

func handleHubEdgeList(client *hub.Client) (any, error) {
	edges, err := client.ListEdges()
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"edges": edges,
		"count": len(edges),
	}, nil
}

func handleHubDeployRule(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		Name            string `json:"name"`
		Trigger         string `json:"trigger"`
		EdgeFilter      string `json:"edge_filter"`
		ArtifactPattern string `json:"artifact_pattern"`
		PostCommand     string `json:"post_command"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.Trigger == "" || params.EdgeFilter == "" || params.ArtifactPattern == "" {
		return nil, fmt.Errorf("trigger, edge_filter, and artifact_pattern are required")
	}

	resp, err := client.CreateDeployRule(&hub.DeployRuleCreateRequest{
		Name:            params.Name,
		Trigger:         params.Trigger,
		EdgeFilter:      params.EdgeFilter,
		ArtifactPattern: params.ArtifactPattern,
		PostCommand:     params.PostCommand,
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"rule_id": resp.RuleID,
		"trigger": params.Trigger,
	}, nil
}

func handleHubDeploy(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		JobID           string   `json:"job_id"`
		ArtifactPattern string   `json:"artifact_pattern"`
		EdgeFilter      string   `json:"edge_filter"`
		EdgeIDs         []string `json:"edge_ids"`
		PostCommand     string   `json:"post_command"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.JobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}

	resp, err := client.TriggerDeploy(&hub.DeployTriggerRequest{
		JobID:           params.JobID,
		ArtifactPattern: params.ArtifactPattern,
		EdgeFilter:      params.EdgeFilter,
		EdgeIDs:         params.EdgeIDs,
		PostCommand:     params.PostCommand,
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"deploy_id":    resp.DeployID,
		"status":       resp.Status,
		"target_count": resp.TargetCount,
	}, nil
}

func handleHubDeployStatus(client *hub.Client, raw json.RawMessage) (any, error) {
	var params struct {
		DeployID string `json:"deploy_id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}
	if params.DeployID == "" {
		return nil, fmt.Errorf("deploy_id is required")
	}

	deploy, err := client.GetDeployStatus(params.DeployID)
	if err != nil {
		return nil, err
	}

	targets := make([]map[string]any, len(deploy.Targets))
	for i, t := range deploy.Targets {
		target := map[string]any{
			"edge_id": t.EdgeID,
			"status":  t.Status,
		}
		if t.EdgeName != "" {
			target["edge_name"] = t.EdgeName
		}
		if t.Error != "" {
			target["error"] = t.Error
		}
		targets[i] = target
	}

	return map[string]any{
		"deploy_id": deploy.ID,
		"status":    deploy.Status,
		"targets":   targets,
	}, nil
}
