package hub

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
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
	Arch     string            `json:"arch,omitempty"`    // arm64, amd64, etc.
	Runtime  string            `json:"runtime,omitempty"` // onnx, tflite, tensorrt, etc.
	Storage  float64           `json:"storage_gb,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	LastSeen string            `json:"last_seen,omitempty"`
}

// EdgeRegisterResponse is the response from registering an edge.
type EdgeRegisterResponse struct {
	EdgeID string `json:"edge_id"`
}

// DeployRule defines an automatic deployment trigger.
type DeployRule struct {
	ID              string `json:"id"`
	Name            string `json:"name,omitempty"`
	Trigger         string `json:"trigger"`          // e.g. "job_tag:production", "dag_complete:*"
	EdgeFilter      string `json:"edge_filter"`      // e.g. "tag:onnx,arm64", "name:jetson-*"
	ArtifactPattern string `json:"artifact_pattern"` // e.g. "outputs/*.onnx"
	PostCommand     string `json:"post_command,omitempty"`
	Enabled         bool   `json:"enabled"`
	CreatedAt       string `json:"created_at,omitempty"`
}

// DeployRuleCreateRequest is the payload for creating a deploy rule.
type DeployRuleCreateRequest struct {
	Name               string `json:"name,omitempty"`
	Trigger            string `json:"trigger"`
	EdgeFilter         string `json:"edge_filter"`
	ArtifactPattern    string `json:"artifact_pattern"`
	PostCommand        string `json:"post_command,omitempty"`
	HealthCheck        string `json:"health_check,omitempty"`
	HealthCheckTimeout int    `json:"health_check_timeout,omitempty"`
}

// DeployRuleCreateResponse is the response from creating a deploy rule.
type DeployRuleCreateResponse struct {
	RuleID string `json:"rule_id"`
}

// Deployment represents a deployment instance.
type Deployment struct {
	ID         string         `json:"id"`
	RuleID     string         `json:"rule_id,omitempty"`
	JobID      string         `json:"job_id,omitempty"`
	Status     string         `json:"status"` // pending, deploying, completed, failed, partial
	Targets    []DeployTarget `json:"targets,omitempty"`
	CreatedAt  string         `json:"created_at,omitempty"`
	FinishedAt string         `json:"finished_at,omitempty"`
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

// DeployTriggerRequest is the payload for triggering a deployment.
type DeployTriggerRequest struct {
	JobID           string   `json:"job_id"`
	ArtifactPattern string   `json:"artifact_pattern,omitempty"`
	EdgeFilter      string   `json:"edge_filter,omitempty"`
	EdgeIDs         []string `json:"edge_ids,omitempty"`
	PostCommand     string   `json:"post_command,omitempty"`
}

// DeployTriggerResponse is the response from triggering a deployment.
type DeployTriggerResponse struct {
	DeployID    string `json:"deploy_id"`
	Status      string `json:"status"`
	TargetCount int    `json:"target_count"`
}

// =========================================================================
// Edge Client Methods (Supabase PostgREST)
// =========================================================================

// RegisterEdge upserts an edge device in hub_edges.
func (c *Client) RegisterEdge(name string, tags []string, capabilities map[string]any) (string, error) {
	if tags == nil {
		tags = []string{}
	}
	edgeID := newID()
	arch, _ := capabilities["arch"].(string)
	runtime, _ := capabilities["runtime"].(string)
	storageGB, _ := capabilities["storage_gb"].(float64)

	row := map[string]any{
		"id":         edgeID,
		"name":       name,
		"tags":       tags,
		"arch":       arch,
		"runtime":    runtime,
		"storage":    storageGB,
		"status":     "online",
		"project_id": c.teamID,
		"last_seen":  time.Now().UTC().Format(time.RFC3339),
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := c.supabasePost("/rest/v1/hub_edges", row, &rows); err != nil {
		return "", fmt.Errorf("register edge: %w", err)
	}
	if len(rows) > 0 && rows[0].ID != "" {
		return rows[0].ID, nil
	}
	return edgeID, nil
}

// ListEdges returns all registered edge devices.
func (c *Client) ListEdges() ([]Edge, error) {
	var edges []Edge
	if err := c.supabaseGet("/rest/v1/hub_edges?order=created_at.desc", &edges); err != nil {
		return nil, fmt.Errorf("list edges: %w", err)
	}
	return edges, nil
}

// EdgeHeartbeat updates an edge device's last_seen timestamp.
func (c *Client) EdgeHeartbeat(edgeID, status string) error {
	body := map[string]any{
		"status":   status,
		"last_seen": time.Now().UTC().Format(time.RFC3339),
	}
	if err := c.supabasePatch("/rest/v1/hub_edges?id=eq."+edgeID, body, nil); err != nil {
		return fmt.Errorf("edge heartbeat: %w", err)
	}
	return nil
}

// RemoveEdge deletes an edge device from hub_edges.
func (c *Client) RemoveEdge(edgeID string) error {
	req, err := newDeleteRequest(context.Background(), c.supabaseRestURL("/rest/v1/hub_edges?id=eq."+edgeID))
	if err != nil {
		return err
	}
	c.setSupabaseHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("remove edge: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("remove edge: %d %s", resp.StatusCode, string(body))
	}
	return nil
}

// EdgeControlRequest is the payload for sending a control message to an edge.
type EdgeControlRequest struct {
	Action string         `json:"action"`
	Params map[string]any `json:"params,omitempty"`
}

// EdgeControlResponse is the response from an edge control operation.
type EdgeControlResponse struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

// EdgeControl inserts a control message into hub_edge_control_queue.
func (c *Client) EdgeControl(edgeID string, req *EdgeControlRequest) (*EdgeControlResponse, error) {
	msgID := newID()
	row := map[string]any{
		"id":      msgID,
		"edge_id": edgeID,
		"action":  req.Action,
		"params":  req.Params,
	}
	if err := c.supabasePost("/rest/v1/hub_edge_control_queue", row, nil); err != nil {
		return nil, fmt.Errorf("edge control: %w", err)
	}
	return &EdgeControlResponse{MessageID: msgID, Status: "queued"}, nil
}

// GetEdgeMetrics retrieves recent metrics for an edge device.
// NOTE: Edge metrics are not stored in Supabase yet; returns empty response.
func (c *Client) GetEdgeMetrics(edgeID string, limit int) (*EdgeMetricsResponse, error) {
	return &EdgeMetricsResponse{EdgeID: edgeID, Metrics: []EdgeMetricEntry{}}, nil
}

// =========================================================================
// Deploy Rule Client Methods (Supabase PostgREST)
// =========================================================================

// CreateDeployRule inserts a deploy rule into hub_deploy_rules.
func (c *Client) CreateDeployRule(req *DeployRuleCreateRequest) (*DeployRuleCreateResponse, error) {
	ruleID := newID()
	row := map[string]any{
		"id":               ruleID,
		"name":             req.Name,
		"trigger_expr":     req.Trigger,
		"edge_filter":      req.EdgeFilter,
		"artifact_pattern": req.ArtifactPattern,
		"post_command":     req.PostCommand,
		"enabled":          true,
		"project_id":       c.teamID,
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := c.supabasePost("/rest/v1/hub_deploy_rules", row, &rows); err != nil {
		return nil, fmt.Errorf("create deploy rule: %w", err)
	}
	id := ruleID
	if len(rows) > 0 && rows[0].ID != "" {
		id = rows[0].ID
	}
	return &DeployRuleCreateResponse{RuleID: id}, nil
}

// ListDeployRules returns all deploy rules.
func (c *Client) ListDeployRules() ([]DeployRule, error) {
	var dbRules []struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		TriggerExpr     string `json:"trigger_expr"`
		EdgeFilter      string `json:"edge_filter"`
		ArtifactPattern string `json:"artifact_pattern"`
		PostCommand     string `json:"post_command"`
		Enabled         bool   `json:"enabled"`
		CreatedAt       string `json:"created_at"`
	}
	if err := c.supabaseGet("/rest/v1/hub_deploy_rules?order=created_at.desc", &dbRules); err != nil {
		return nil, fmt.Errorf("list deploy rules: %w", err)
	}
	rules := make([]DeployRule, len(dbRules))
	for i, r := range dbRules {
		rules[i] = DeployRule{
			ID:              r.ID,
			Name:            r.Name,
			Trigger:         r.TriggerExpr,
			EdgeFilter:      r.EdgeFilter,
			ArtifactPattern: r.ArtifactPattern,
			PostCommand:     r.PostCommand,
			Enabled:         r.Enabled,
			CreatedAt:       r.CreatedAt,
		}
	}
	return rules, nil
}

// DeleteDeployRule deletes a deploy rule.
func (c *Client) DeleteDeployRule(ruleID string) error {
	req, err := newDeleteRequest(context.Background(), c.supabaseRestURL("/rest/v1/hub_deploy_rules?id=eq."+ruleID))
	if err != nil {
		return err
	}
	c.setSupabaseHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete deploy rule: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete deploy rule: %d %s", resp.StatusCode, string(body))
	}
	return nil
}

// =========================================================================
// Deploy Trigger Client Methods (Supabase PostgREST)
// =========================================================================

// TriggerDeploy creates a deployment record in hub_deployments.
func (c *Client) TriggerDeploy(req *DeployTriggerRequest) (*DeployTriggerResponse, error) {
	deployID := newID()
	row := map[string]any{
		"id":         deployID,
		"job_id":     req.JobID,
		"status":     "deploying",
		"project_id": c.teamID,
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := c.supabasePost("/rest/v1/hub_deployments", row, &rows); err != nil {
		return nil, fmt.Errorf("trigger deploy: %w", err)
	}
	id := deployID
	if len(rows) > 0 && rows[0].ID != "" {
		id = rows[0].ID
	}

	// Count targets: either explicit EdgeIDs or EdgeFilter (just count edges for now).
	targetCount := len(req.EdgeIDs)
	return &DeployTriggerResponse{
		DeployID:    id,
		Status:      "deploying",
		TargetCount: targetCount,
	}, nil
}

// GetDeployStatus returns a deployment with its targets.
func (c *Client) GetDeployStatus(deployID string) (*Deployment, error) {
	var rows []Deployment
	if err := c.supabaseGet("/rest/v1/hub_deployments?id=eq."+deployID, &rows); err != nil {
		return nil, fmt.Errorf("get deploy status: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("get deploy status: not found: %s", deployID)
	}
	deploy := &rows[0]

	// Fetch targets.
	var targets []DeployTarget
	if err := c.supabaseGet("/rest/v1/hub_deploy_targets?deploy_id=eq."+deployID, &targets); err != nil {
		return nil, fmt.Errorf("get deploy targets: %w", err)
	}
	deploy.Targets = targets

	return deploy, nil
}

// =========================================================================
// Helpers
// =========================================================================

func newDeleteRequest(ctx context.Context, url string) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, "DELETE", url, nil)
}
