package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// limitedWriter wraps a bytes.Buffer with a maximum size to prevent OOM from
// unbounded process output.
type limitedWriter struct {
	buf   *bytes.Buffer
	limit int
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	if lw.buf.Len()+len(p) > lw.limit {
		remaining := lw.limit - lw.buf.Len()
		if remaining > 0 {
			lw.buf.Write(p[:remaining])
		}
		return len(p), nil // silently discard excess (don't break the process)
	}
	return lw.buf.Write(p)
}

// cqMentionRe matches @cq mentions in message content.
// Matches "@cq" preceded by whitespace/start-of-string and followed by
// a non-alphanumeric character or end-of-string. Case-insensitive.
var cqMentionRe = regexp.MustCompile(`(?i)(?:^|[\s,;:!?.(])@cq(?:$|[^a-zA-Z0-9_])`)

// msgRequest holds parameters for a single message processing job.
type msgRequest struct {
	id        string
	channelID string
	content   string
	actionID  string // non-empty if triggered by A2UI button click
}

// channelMsg is a lightweight message record fetched for context.
type channelMsg struct {
	ID         string          `json:"id"`
	Content    string          `json:"content"`
	SenderType string          `json:"sender_type"`
	Metadata   json.RawMessage `json:"metadata"`
}

// AgentConfig holds configuration for the Agent component.
type AgentConfig struct {
	SupabaseURL string // Supabase project URL (e.g., https://xxx.supabase.co)
	APIKey      string // Supabase anon key
	AuthToken   string // Supabase auth token (JWT)
	ProjectID   string // C4 cloud project ID
	WorkerID    string // Worker identifier (default: "cq-agent")
	ProjectDir  string // Working directory passed to claude -p via --dir
}

// Agent is a Component that listens for @cq mentions in c1_messages
// via Supabase Realtime and dispatches them to `claude -p`.
type Agent struct {
	cfg    AgentConfig
	client *RealtimeClient

	mu         sync.Mutex
	status     string // "stopped", "starting", "running", "degraded", "failed"
	cancel     context.CancelFunc
	childCtx   context.Context
	claudePath string // path to claude CLI binary
	memberID   string // cached member ID from ensureMember
	httpClient *http.Client
	wg         sync.WaitGroup
	inFlight   atomic.Int32 // count of active processMessage goroutines
	sem        chan struct{} // concurrency limiter for processMessage goroutines
}

// NewAgent creates a new Agent component.
func NewAgent(cfg AgentConfig) *Agent {
	if cfg.WorkerID == "" {
		cfg.WorkerID = "cq-agent"
	}
	return &Agent{
		cfg:    cfg,
		status: "stopped",
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		sem: make(chan struct{}, 5), // max 5 concurrent claude -p processes
	}
}

// Name implements Component.
func (a *Agent) Name() string { return "agent" }

// Health implements Component.
func (a *Agent) Health() ComponentHealth {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch a.status {
	case "running":
		return ComponentHealth{Status: "ok"}
	case "degraded":
		return ComponentHealth{Status: "degraded", Detail: "claude CLI not found"}
	case "failed":
		return ComponentHealth{Status: "error", Detail: "failed"}
	default:
		return ComponentHealth{Status: "ok", Detail: a.status}
	}
}

func (a *Agent) setStatus(s string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status = s
}

// Start implements Component.
func (a *Agent) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.status == "running" || a.status == "degraded" {
		a.mu.Unlock()
		return fmt.Errorf("agent already running")
	}
	a.status = "starting"
	a.mu.Unlock()

	// Check for claude CLI
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: [agent] claude CLI not found, running in degraded mode\n")
		a.setStatus("degraded")
		a.claudePath = ""
	} else {
		a.claudePath = claudePath
		fmt.Fprintf(os.Stderr, "cq: [agent] claude CLI found at %s\n", claudePath)
	}

	// Validate config
	if a.cfg.SupabaseURL == "" || a.cfg.APIKey == "" {
		return fmt.Errorf("supabase URL and API key are required")
	}

	// Create Realtime client
	a.client = NewRealtimeClient(a.cfg.SupabaseURL, a.cfg.APIKey, a.cfg.AuthToken)
	a.client.Subscribe("c1_messages")
	a.client.OnMessage(func(event RealtimeEvent) {
		a.handleEvent(event)
	})

	childCtx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.cancel = cancel
	a.childCtx = childCtx
	a.mu.Unlock()

	if err := a.client.Connect(childCtx); err != nil {
		cancel()
		a.setStatus("failed")
		return fmt.Errorf("realtime connect: %w", err)
	}

	if a.claudePath == "" {
		a.setStatus("degraded")
	} else {
		a.setStatus("running")
	}
	fmt.Fprintf(os.Stderr, "cq: [agent] started (health=%s)\n", a.Health().Status)
	return nil
}

// Stop implements Component.
func (a *Agent) Stop(ctx context.Context) error {
	a.mu.Lock()
	cancel := a.cancel
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if a.client != nil {
		a.client.Close()
	}

	done := make(chan struct{})
	go func() { a.wg.Wait(); close(done) }()
	select {
	case <-done:
		// all goroutines finished cleanly
	case <-ctx.Done():
		fmt.Fprintf(os.Stderr, "cq: [agent] stop timed out, %d goroutines may still run\n", a.inFlight.Load())
	}

	a.setStatus("stopped")
	fmt.Fprintf(os.Stderr, "cq: [agent] stopped\n")
	return nil
}

// handleEvent processes a Realtime event from c1_messages.
func (a *Agent) handleEvent(event RealtimeEvent) {
	if event.Table != "c1_messages" || event.ChangeType != "INSERT" {
		return
	}

	// Parse the message record
	var msg struct {
		ID         string          `json:"id"`
		ChannelID  string          `json:"channel_id"`
		Content    string          `json:"content"`
		SenderName string          `json:"sender_name"`
		SenderType string          `json:"sender_type"`
		ProjectID  string          `json:"project_id"`
		ClaimedBy  *string         `json:"claimed_by"`
		Metadata   json.RawMessage `json:"metadata"`
	}
	if err := json.Unmarshal(event.Record, &msg); err != nil {
		return
	}

	// Filter: only our project
	if msg.ProjectID != "" && msg.ProjectID != a.cfg.ProjectID {
		return
	}

	// Filter: skip messages from agents (prevent loops) — applied before both @cq and A2UI checks
	if msg.SenderType == "agent" || msg.SenderType == "system" {
		return
	}

	// Filter: already claimed
	if msg.ClaimedBy != nil {
		return
	}

	// Detect trigger: @cq mention OR a2ui_response action
	var actionID string
	if len(msg.Metadata) > 0 {
		var meta struct {
			A2UIResponse *struct {
				ActionID string `json:"action_id"`
			} `json:"a2ui_response"`
		}
		if err := json.Unmarshal(msg.Metadata, &meta); err == nil && meta.A2UIResponse != nil {
			actionID = meta.A2UIResponse.ActionID
		}
	}

	isCQMention := cqMentionRe.MatchString(msg.Content)
	isA2UI := actionID != ""

	if !isCQMention && !isA2UI {
		return
	}

	if isCQMention {
		fmt.Fprintf(os.Stderr, "cq: [agent] @cq mention detected in msg %s from %s\n", msg.ID, msg.SenderName)
	} else {
		fmt.Fprintf(os.Stderr, "cq: [agent] a2ui_response detected in msg %s from %s (action=%s)\n", msg.ID, msg.SenderName, actionID)
	}

	// Check concurrency limit before claiming (avoid claiming then dropping)
	select {
	case a.sem <- struct{}{}:
		// acquired semaphore slot
	default:
		fmt.Fprintf(os.Stderr, "cq: [agent] msg %s skipped (concurrency limit)\n", msg.ID)
		return
	}

	// Try to claim the message
	claimed, err := a.claimMessage(msg.ID)
	if err != nil {
		<-a.sem // release slot
		fmt.Fprintf(os.Stderr, "cq: [agent] claim msg %s failed: %v\n", msg.ID, err)
		return
	}
	if !claimed {
		<-a.sem // release slot
		fmt.Fprintf(os.Stderr, "cq: [agent] msg %s already claimed by another worker\n", msg.ID)
		return
	}

	// Process in background
	a.wg.Add(1)
	a.inFlight.Add(1)
	go func() {
		defer a.wg.Done()
		defer a.inFlight.Add(-1)
		defer func() { <-a.sem }()
		a.processMessage(msgRequest{id: msg.ID, channelID: msg.ChannelID, content: msg.Content, actionID: actionID})
	}()
}

// claimMessage atomically claims a message via Supabase RPC.
func (a *Agent) claimMessage(messageID string) (bool, error) {
	payload := map[string]interface{}{
		"p_message_id": messageID,
		"p_worker_id":  a.cfg.WorkerID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}

	rpcURL := a.cfg.SupabaseURL + "/rest/v1/rpc/claim_message"
	req, err := http.NewRequest("POST", rpcURL, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	a.setHeaders(req)
	req.Header.Set("Prefer", "return=representation")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("rpc claim_message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return false, fmt.Errorf("rpc claim_message: %d %s", resp.StatusCode, string(respBody))
	}

	var rows []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return false, fmt.Errorf("decode claim response: %w", err)
	}
	return len(rows) > 0, nil
}

// fetchChannelContext fetches recent messages from a channel for A2UI context building.
func (a *Agent) fetchChannelContext(ctx context.Context, channelID string, limit int) ([]channelMsg, error) {
	fetchURL := fmt.Sprintf(
		"%s/rest/v1/c1_messages?channel_id=eq.%s&order=created_at.desc&limit=%d&select=id,content,sender_type,metadata",
		a.cfg.SupabaseURL, url.QueryEscape(channelID), limit,
	)
	req, err := http.NewRequestWithContext(ctx, "GET", fetchURL, nil)
	if err != nil {
		return nil, err
	}
	a.setHeaders(req)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetchChannelContext: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return nil, fmt.Errorf("fetchChannelContext: %d %s", resp.StatusCode, string(body))
	}

	var msgs []channelMsg
	if err := json.NewDecoder(resp.Body).Decode(&msgs); err != nil {
		return nil, fmt.Errorf("fetchChannelContext decode: %w", err)
	}
	return msgs, nil
}

// buildA2UIPrompt builds a prompt string from channel context and the selected action.
// It scans msgs for the most recent agent message that contains an "a2ui" key in metadata.
// If found, it returns a formatted prompt with original context; otherwise returns just the label.
func buildA2UIPrompt(msgs []channelMsg, actionID, label string) string {
	for _, m := range msgs {
		if m.SenderType != "agent" {
			continue
		}
		if len(m.Metadata) == 0 {
			continue
		}
		var meta map[string]json.RawMessage
		if err := json.Unmarshal(m.Metadata, &meta); err != nil {
			continue
		}
		if _, ok := meta["a2ui"]; !ok {
			continue
		}
		// Found the original A2UI message
		return fmt.Sprintf("[A2UI action selected]\nAction: %q (id: %s)\n\nOriginal context:\n%s", label, actionID, m.Content)
	}
	return label
}

// processMessage invokes `claude -p` with the message content and posts the response.
func (a *Agent) processMessage(req msgRequest) {
	a.mu.Lock()
	claudePath := a.claudePath
	a.mu.Unlock()

	// Ensure member record exists before presence updates.
	a.mu.Lock()
	memberID := a.memberID
	a.mu.Unlock()
	if memberID == "" {
		if id := a.ensureMember(); id != "" {
			a.mu.Lock()
			a.memberID = id
			a.mu.Unlock()
		}
	}

	// Async notification: signal "typing" presence immediately so the user sees
	// feedback before the potentially long claude -p invocation completes.
	a.updateMemberPresence("typing")
	defer a.updateMemberPresence("online")

	if claudePath == "" {
		errMsg := "claude CLI is not available. The agent is running in degraded mode."
		a.postMessage(req.channelID, errMsg)
		return
	}

	// Derive parent context for graceful shutdown propagation.
	a.mu.Lock()
	parentCtx := a.childCtx
	a.mu.Unlock()
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	var prompt string
	if req.actionID != "" {
		// A2UI path: fetch channel context and build contextual prompt.
		// Use a short timeout so a slow Supabase call does not delay the 120s claude budget.
		fetchCtx, fetchCancel := context.WithTimeout(parentCtx, 10*time.Second)
		msgs, err := a.fetchChannelContext(fetchCtx, req.channelID, 20)
		fetchCancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "cq: [agent] fetchChannelContext for msg %s: %v (falling back to label)\n", req.id, err)
			prompt = req.content
		} else {
			prompt = buildA2UIPrompt(msgs, req.actionID, req.content)
		}
	} else {
		// @cq mention path: strip @cq from the prompt
		prompt = cqMentionRe.ReplaceAllString(req.content, "")
		prompt = strings.TrimSpace(prompt)
		if prompt == "" {
			prompt = "Hello! How can I help?"
		}
	}

	fmt.Fprintf(os.Stderr, "cq: [agent] spawning claude -p for msg %s\n", req.id)
	ctx, cancel := context.WithTimeout(parentCtx, 120*time.Second)
	defer cancel()

	args := []string{"-p", prompt, "--output-format", "json"}
	a.mu.Lock()
	projectDir := a.cfg.ProjectDir
	a.mu.Unlock()
	if projectDir != "" {
		args = append(args, "--dir", projectDir)
	}

	cmd := exec.CommandContext(ctx, claudePath, args...)
	var stdout, stderr bytes.Buffer
	const maxOutputSize = 1 << 20 // 1MB limit
	cmd.Stdout = &limitedWriter{buf: &stdout, limit: maxOutputSize}
	cmd.Stderr = &limitedWriter{buf: &stderr, limit: maxOutputSize}

	err := cmd.Run()

	var response string
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			response = fmt.Sprintf("Request timed out after 120s. Partial stderr:\n```\n%s\n```",
				truncate(stderr.String(), 500))
		} else {
			response = fmt.Sprintf("claude -p failed: %v\nStderr:\n```\n%s\n```",
				err, truncate(stderr.String(), 500))
		}
	} else {
		// Parse JSON output
		response = parseClaudeOutput(stdout.Bytes())
	}

	fmt.Fprintf(os.Stderr, "cq: [agent] posting response for msg %s (%d chars)\n", req.id, len(response))
	a.postMessage(req.channelID, response)
}

// updateMemberPresence updates the agent member's status field in c1_members.
// Used to emit an async "typing" notification before claude -p runs so the user
// sees immediate feedback, and "online" after the response is posted.
// No-op if memberID is not yet resolved or the request fails.
func (a *Agent) updateMemberPresence(status string) {
	a.mu.Lock()
	memberID := a.memberID
	a.mu.Unlock()
	if memberID == "" {
		return
	}

	payload := map[string]interface{}{
		"status":       status,
		"last_seen_at": time.Now().UTC().Format(time.RFC3339),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	patchURL := fmt.Sprintf("%s/rest/v1/c1_members?id=eq.%s", a.cfg.SupabaseURL, url.QueryEscape(memberID))
	req, err := http.NewRequest("PATCH", patchURL, bytes.NewReader(body))
	if err != nil {
		return
	}
	a.setHeaders(req)
	req.Header.Set("Prefer", "return=minimal")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: [agent] updateMemberPresence %s: %v\n", status, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		fmt.Fprintf(os.Stderr, "cq: [agent] updateMemberPresence %s: %d %s\n", status, resp.StatusCode, string(b))
	}
}

// parseClaudeOutput extracts the text result from claude --output-format json.
func parseClaudeOutput(data []byte) string {
	// Claude JSON output format: {"type":"result","subtype":"success","result":"..."}
	var output struct {
		Type    string `json:"type"`
		Subtype string `json:"subtype"`
		Result  string `json:"result"`
	}
	if err := json.Unmarshal(data, &output); err != nil {
		// Fallback: return raw output
		return truncate(string(data), 4000)
	}
	if output.Result != "" {
		return truncate(output.Result, 4000)
	}
	return truncate(string(data), 4000)
}

// postMessage sends a response message to a channel via Supabase REST.
func (a *Agent) postMessage(channelID, content string) {
	a.mu.Lock()
	memberID := a.memberID
	a.mu.Unlock()

	if memberID == "" {
		memberID = a.ensureMember()
		if memberID != "" {
			a.mu.Lock()
			a.memberID = memberID
			a.mu.Unlock()
		}
	}

	payload := map[string]interface{}{
		"channel_id":  channelID,
		"project_id":  a.cfg.ProjectID,
		"sender_type": "agent",
		"sender_id":   a.cfg.WorkerID,
		"sender_name": a.cfg.WorkerID,
		"content":     content,
	}
	if memberID != "" {
		payload["member_id"] = memberID
	}

	body, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: [agent] marshal response: %v\n", err)
		return
	}

	postURL := a.cfg.SupabaseURL + "/rest/v1/c1_messages"
	req, err := http.NewRequest("POST", postURL, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: [agent] create request: %v\n", err)
		return
	}
	a.setHeaders(req)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cq: [agent] post message: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		fmt.Fprintf(os.Stderr, "cq: [agent] post message %d: %s\n", resp.StatusCode, string(respBody))
	}
}

// ensureMember creates or finds the agent member record. Returns member ID or "".
func (a *Agent) ensureMember() string {
	// Check if member exists
	filter := fmt.Sprintf("project_id=eq.%s&member_type=eq.agent&external_id=eq.%s&select=id",
		url.QueryEscape(a.cfg.ProjectID), url.QueryEscape(a.cfg.WorkerID))
	getURL := a.cfg.SupabaseURL + "/rest/v1/c1_members?" + filter

	req, err := http.NewRequest("GET", getURL, nil)
	if err != nil {
		return ""
	}
	a.setHeaders(req)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var members []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&members); err == nil && len(members) > 0 {
		return members[0].ID
	}

	// Create member
	payload := map[string]interface{}{
		"project_id":   a.cfg.ProjectID,
		"member_type":  "agent",
		"external_id":  a.cfg.WorkerID,
		"display_name": a.cfg.WorkerID,
		"status":       "online",
		"last_seen_at": time.Now().UTC().Format(time.RFC3339),
	}
	body, _ := json.Marshal(payload)
	postURL := a.cfg.SupabaseURL + "/rest/v1/c1_members"
	req, err = http.NewRequest("POST", postURL, bytes.NewReader(body))
	if err != nil {
		return ""
	}
	a.setHeaders(req)
	req.Header.Set("Prefer", "return=representation")

	resp2, err := a.httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp2.Body.Close()

	var created []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&created); err == nil && len(created) > 0 {
		return created[0].ID
	}
	return ""
}

// setHeaders sets Supabase REST headers.
func (a *Agent) setHeaders(req *http.Request) {
	req.Header.Set("apikey", a.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	if a.cfg.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+a.cfg.AuthToken)
	}
}

// truncate limits a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}
