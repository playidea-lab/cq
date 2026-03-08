package hub

import (
	"fmt"
	"net/http"
)

// =========================================================================
// Edge Models
// =========================================================================

// Edge represents a registered edge device for artifact deployment.
type Edge struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Status   string            `json:"status"` // online, offline
	Tags     []string          `json:"tags,omitempty"`
	Arch     string            `json:"arch,omitempty"`     // arm64, amd64, etc.
	Runtime  string            `json:"runtime,omitempty"`  // onnx, tflite, tensorrt, etc.
	Storage  float64           `json:"storage_gb,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	LastSeen string            `json:"last_seen,omitempty"`
}

// EdgeRegisterResponse is the response from POST /v1/edges/register.
type EdgeRegisterResponse struct {
	EdgeID string `json:"edge_id"`
}

// DeployRule defines an automatic deployment trigger.
type DeployRule struct {
	ID              string `json:"id"`
	Name            string `json:"name,omitempty"`
	Trigger         string `json:"trigger"`           // e.g. "job_tag:production", "dag_complete:*"
	EdgeFilter      string `json:"edge_filter"`        // e.g. "tag:onnx,arm64", "name:jetson-*"
	ArtifactPattern string `json:"artifact_pattern"`   // e.g. "outputs/*.onnx"
	PostCommand     string `json:"post_command,omitempty"` // command to run on edge after deploy
	Enabled         bool   `json:"enabled"`
	CreatedAt       string `json:"created_at,omitempty"`
}

// DeployRuleCreateRequest is the payload for POST /v1/deploy/rules.
type DeployRuleCreateRequest struct {
	Name            string `json:"name,omitempty"`
	Trigger         string `json:"trigger"`
	EdgeFilter      string `json:"edge_filter"`
	ArtifactPattern string `json:"artifact_pattern"`
	PostCommand     string `json:"post_command,omitempty"`
}

// DeployRuleCreateResponse is the response from POST /v1/deploy/rules.
type DeployRuleCreateResponse struct {
	RuleID string `json:"rule_id"`
}

// Deployment represents a deployment instance (triggered by rule or manual).
type Deployment struct {
	ID         string       `json:"id"`
	RuleID     string       `json:"rule_id,omitempty"`
	JobID      string       `json:"job_id,omitempty"`
	Status     string       `json:"status"` // pending, deploying, completed, failed, partial
	Targets    []DeployTarget `json:"targets,omitempty"`
	CreatedAt  string       `json:"created_at,omitempty"`
	FinishedAt string       `json:"finished_at,omitempty"`
}

// DeployTarget represents the deployment status for a single edge device.
type DeployTarget struct {
	EdgeID    string `json:"edge_id"`
	EdgeName  string `json:"edge_name,omitempty"`
	Status    string `json:"status"` // pending, downloading, deploying, succeeded, failed
	Error     string `json:"error,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	DoneAt    string `json:"done_at,omitempty"`
}

// DeployTriggerRequest is the payload for POST /v1/deploy/trigger.
type DeployTriggerRequest struct {
	JobID           string   `json:"job_id"`
	ArtifactPattern string   `json:"artifact_pattern,omitempty"` // default: all artifacts
	EdgeFilter      string   `json:"edge_filter,omitempty"`      // default: all online edges
	EdgeIDs         []string `json:"edge_ids,omitempty"`         // explicit edge list (overrides filter)
	PostCommand     string   `json:"post_command,omitempty"`
}

// DeployTriggerResponse is the response from POST /v1/deploy/trigger.
type DeployTriggerResponse struct {
	DeployID    string `json:"deploy_id"`
	Status      string `json:"status"`
	TargetCount int    `json:"target_count"`
}

// =========================================================================
// Edge Client Methods
// =========================================================================

// RegisterEdge registers an edge device with the Hub.
func (c *Client) RegisterEdge(name string, tags []string, capabilities map[string]any) (string, error) {
	body := map[string]any{
		"name":         name,
		"tags":         tags,
		"capabilities": capabilities,
	}
	var resp EdgeRegisterResponse
	if err := c.post("/edges/register", body, &resp); err != nil {
		return "", fmt.Errorf("register edge: %w", err)
	}
	return resp.EdgeID, nil
}

// ListEdges returns all registered edge devices.
func (c *Client) ListEdges() ([]Edge, error) {
	var edges []Edge
	if err := c.get("/edges", &edges); err != nil {
		return nil, fmt.Errorf("list edges: %w", err)
	}
	return edges, nil
}

// EdgeHeartbeat sends a heartbeat from an edge device.
func (c *Client) EdgeHeartbeat(edgeID, status string) error {
	body := map[string]any{
		"edge_id": edgeID,
		"status":  status,
	}
	var resp HeartbeatResponse
	if err := c.post("/edges/heartbeat", body, &resp); err != nil {
		return fmt.Errorf("edge heartbeat: %w", err)
	}
	if !resp.Acknowledged {
		return fmt.Errorf("edge heartbeat not acknowledged")
	}
	return nil
}

// RemoveEdge unregisters an edge device.
func (c *Client) RemoveEdge(edgeID string) error {
	req, err := newDeleteRequest(c.url("/edges/" + edgeID))
	if err != nil {
		return err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("remove edge: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("remove edge: %d", resp.StatusCode)
	}
	return nil
}

// EdgeControlRequest is the payload for POST /v1/edges/{id}/control.
type EdgeControlRequest struct {
	Action string         `json:"action"`
	Params map[string]any `json:"params,omitempty"`
}

// EdgeControlResponse is the response from POST /v1/edges/{id}/control.
type EdgeControlResponse struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

// EdgeControl sends a control message to an edge device.
func (c *Client) EdgeControl(edgeID string, req *EdgeControlRequest) (*EdgeControlResponse, error) {
	var resp EdgeControlResponse
	if err := c.post("/edges/"+edgeID+"/control", req, &resp); err != nil {
		return nil, fmt.Errorf("edge control: %w", err)
	}
	return &resp, nil
}

// GetEdgeMetrics retrieves recent metrics for an edge device.
func (c *Client) GetEdgeMetrics(edgeID string, limit int) (*EdgeMetricsResponse, error) {
	path := fmt.Sprintf("/edges/%s/metrics?limit=%d", edgeID, limit)
	var resp EdgeMetricsResponse
	if err := c.get(path, &resp); err != nil {
		return nil, fmt.Errorf("get edge metrics: %w", err)
	}
	return &resp, nil
}

// =========================================================================
// Deploy Rule Client Methods
// =========================================================================

// CreateDeployRule creates an automatic deployment rule.
func (c *Client) CreateDeployRule(req *DeployRuleCreateRequest) (*DeployRuleCreateResponse, error) {
	var resp DeployRuleCreateResponse
	if err := c.post("/deploy/rules", req, &resp); err != nil {
		return nil, fmt.Errorf("create deploy rule: %w", err)
	}
	return &resp, nil
}

// ListDeployRules returns all deployment rules.
func (c *Client) ListDeployRules() ([]DeployRule, error) {
	var rules []DeployRule
	if err := c.get("/deploy/rules", &rules); err != nil {
		return nil, fmt.Errorf("list deploy rules: %w", err)
	}
	return rules, nil
}

// DeleteDeployRule deletes a deployment rule.
func (c *Client) DeleteDeployRule(ruleID string) error {
	req, err := newDeleteRequest(c.url("/deploy/rules/" + ruleID))
	if err != nil {
		return err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete deploy rule: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("delete deploy rule: %d", resp.StatusCode)
	}
	return nil
}

// =========================================================================
// Deploy Trigger Client Methods
// =========================================================================

// TriggerDeploy manually triggers deployment of artifacts to edges.
func (c *Client) TriggerDeploy(req *DeployTriggerRequest) (*DeployTriggerResponse, error) {
	var resp DeployTriggerResponse
	if err := c.post("/deploy/trigger", req, &resp); err != nil {
		return nil, fmt.Errorf("trigger deploy: %w", err)
	}
	return &resp, nil
}

// GetDeployStatus returns the status of a deployment.
func (c *Client) GetDeployStatus(deployID string) (*Deployment, error) {
	var deploy Deployment
	if err := c.get("/deploy/"+deployID+"/status", &deploy); err != nil {
		return nil, fmt.Errorf("get deploy status: %w", err)
	}
	return &deploy, nil
}

// =========================================================================
// Helpers
// =========================================================================

func newDeleteRequest(url string) (*http.Request, error) {
	return http.NewRequest("DELETE", url, nil)
}
