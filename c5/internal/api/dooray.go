package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/piqsol/c4/c5/internal/conversation"
	"github.com/piqsol/c4/c5/internal/knowledge"
	"github.com/piqsol/c4/c5/internal/llmclient"
	"github.com/piqsol/c4/c5/internal/model"
)

// doorayHTTPClient is used for outbound Dooray webhook calls.
// A dedicated client with timeout prevents goroutine leaks on unresponsive endpoints.
var doorayHTTPClient = &http.Client{Timeout: 15 * time.Second}

// doorayInbound is the POST body sent by Dooray Slash Command.
// Field names follow the NHN Cloud Dooray API specification.
type doorayInbound struct {
	TenantID     string `json:"tenantId"`
	TenantDomain string `json:"tenantDomain"`
	ChannelID    string `json:"channelId"`
	ChannelName  string `json:"channelName"`
	UserID       string `json:"userId"`
	UserNickname string `json:"userNickname"`
	Command      string `json:"command"`
	Text         string `json:"text"`
	ResponseURL  string `json:"responseUrl"`
	AppToken     string `json:"appToken"`
	CmdToken     string `json:"cmdToken"`
	TriggerID    string `json:"triggerId"`
}

// doorayResponse is the JSON body returned to Dooray after handling a slash command.
type doorayResponse struct {
	Text         string `json:"text"`
	ResponseType string `json:"responseType"`
}

// handleDooray handles GET and POST /v1/webhooks/dooray.
//
// GET: returns 200 OK (Dooray URL verification).
// POST: validates the optional cmd token, sends an ephemeral ack immediately,
// then either processes the request server-side via LLM (if configured) or
// creates a Hub Job for a standby worker (fallback, backward compatible).
func (s *Server) handleDooray(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 512<<10)) // 512 KiB limit
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	var payload doorayInbound
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Token verification: server field takes precedence over env var (backward compat).
	// Security model: subtle.ConstantTimeCompare returns 0 immediately when lengths
	// differ (length oracle). Acceptable for static webhook tokens.
	cmdToken := s.doorayCmdToken
	if cmdToken == "" {
		cmdToken = os.Getenv("C5_DOORAY_CMD_TOKEN")
	}
	if cmdToken != "" {
		expected := []byte(cmdToken)
		appMatch := subtle.ConstantTimeCompare(expected, []byte(payload.AppToken))
		cmdMatch := subtle.ConstantTimeCompare(expected, []byte(payload.CmdToken))
		if appMatch != 1 && cmdMatch != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Send ephemeral ack immediately so Dooray doesn't time out.
	ackText := "⏳ 수신: " + payload.Text
	if payload.Text == "" {
		ackText = "⏳ 수신 완료"
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doorayResponse{
		Text:         ackText,
		ResponseType: "ephemeral",
	})

	// Server-side LLM path.
	if s.llmClient != nil {
		select {
		case s.llmSem <- struct{}{}: // acquire slot
			go func() {
				defer func() { <-s.llmSem }() // release slot
				s.processDoorayServerSide(payload)
			}()
		default:
			log.Printf("c5: dooray: LLM goroutine pool full — dropping request from %q", payload.UserID)
			notifyURL := s.resolveWebhookURL(payload.ChannelID)
			if notifyURL == "" {
				notifyURL = payload.ResponseURL
			}
			if notifyURL != "" {
				go postToDooray(context.Background(), notifyURL, "⚠️ 현재 요청이 많아 처리할 수 없습니다. 잠시 후 다시 시도해 주세요.")
			}
		}
		return
	}

	// Fallback: create Hub Job for a standby worker.
	title := payload.Text
	if title == "" {
		title = payload.Command
	}
	if title == "" {
		title = "dooray"
	}

	req := model.JobSubmitRequest{
		Name:    title,
		Workdir: ".",
		Command: "",
		Tags:    []string{"dooray"},
		Env: map[string]string{
			"DOORAY_RESPONSE_URL": payload.ResponseURL,
			"DOORAY_TEXT":         payload.Text,
			"DOORAY_CMD":          payload.Command,
			"DOORAY_CHANNEL":      payload.ChannelID,
		},
	}

	job, err := s.store.CreateJob(&req)
	if err != nil {
		log.Printf("c5: dooray: create job error: %v", err)
		return
	}
	s.notifyJobAvailable()
	_ = job
}

// llmAction is the structured response the LLM returns when it decides to
// take an action. The action field determines how the server responds:
// "submit_job" | "query_workers" | "query_jobs" | "invoke_capability".
type llmAction struct {
	Action      string         `json:"action"`
	Name        string         `json:"name,omitempty"`
	Command     string         `json:"command,omitempty"`
	RequiresGPU bool           `json:"requires_gpu,omitempty"`
	ExpID       string         `json:"exp_id,omitempty"`
	Memo        string         `json:"memo,omitempty"`
	Limit       int            `json:"limit,omitempty"`
	Status      string         `json:"status,omitempty"`
	Capability  string         `json:"capability,omitempty"`
	Params      map[string]any `json:"params,omitempty"`
}

// extractAction tries to find an action JSON object in the LLM response.
// It handles plain JSON and markdown code blocks.
func extractAction(answer string) (llmAction, bool) {
	// Strip markdown code fences if present.
	s := strings.TrimSpace(answer)
	if idx := strings.Index(s, "```"); idx != -1 {
		s = s[idx+3:]
		if nl := strings.Index(s, "\n"); nl != -1 {
			s = s[nl+1:]
		}
		if end := strings.Index(s, "```"); end != -1 {
			s = s[:end]
		}
	}
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") {
		return llmAction{}, false
	}
	var action llmAction
	if err := json.Unmarshal([]byte(s), &action); err != nil {
		return llmAction{}, false
	}
	if action.Action == "" {
		return llmAction{}, false
	}
	// submit_job requires a command to execute.
	if action.Action == "submit_job" && action.Command == "" {
		return llmAction{}, false
	}
	return action, true
}

// processDoorayServerSide handles server-side LLM processing in a goroutine.
// It queries the knowledge base (if configured), calls the LLM, and posts the
// response to the Dooray Incoming Webhook. If the LLM returns a submit_job
// intent, a Hub job is created instead of posting plain text.
func (s *Server) processDoorayServerSide(payload doorayInbound) {
	if payload.Text == "" {
		return // nothing to process; avoid polluting conversation history
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. Resolve projectID and webhookURL from channelID.
	projectID := ""
	webhookURL := s.resolveWebhookURL(payload.ChannelID)
	if ch, ok := s.channelMap[payload.ChannelID]; ok {
		projectID = ch.ProjectID
	}
	if webhookURL == "" {
		webhookURL = payload.ResponseURL
	}
	if webhookURL == "" {
		log.Printf("c5: dooray: no webhook URL for channel %q — skipping", payload.ChannelID)
		return
	}

	// 2. Search knowledge (optional, non-fatal).
	var knowledgeCtx string
	if s.knowledgeClient != nil && projectID != "" {
		results, err := s.knowledgeClient.Search(ctx, projectID, payload.Text, 5)
		if err != nil {
			log.Printf("c5: dooray: knowledge search error: %v", err)
		} else if len(results) > 0 {
			knowledgeCtx = formatKnowledgeContext(results)
		}
	}

	// 3. Fetch registered capabilities for dynamic prompt enrichment.
	var capsCtx string
	if caps, err := s.store.ListCapabilities(projectID); err == nil && len(caps) > 0 {
		var sb strings.Builder
		sb.WriteString("\n\n## 사용 가능한 워커 Capability (invoke_capability action으로 실행)\n")
		for _, c := range caps {
			sb.WriteString("- ")
			sb.WriteString(c.Name)
			if c.Description != "" {
				sb.WriteString(": ")
				sb.WriteString(c.Description)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("→ {\"action\":\"invoke_capability\",\"capability\":\"<name>\",\"params\":{...}}")
		capsCtx = sb.String()
	}

	// 4. Build system prompt and call LLM (with conversation history).
	systemPrompt := buildDooraySystemPrompt(projectID, knowledgeCtx, capsCtx)

	var history []llmclient.Message
	if convMsgs, err := s.convStore.Get(ctx, payload.ChannelID, 20); err != nil {
		log.Printf("c5: dooray: conversation get error: %v", err)
	} else {
		history = make([]llmclient.Message, len(convMsgs))
		for i, m := range convMsgs {
			history[i] = llmclient.Message{Role: m.Role, Content: m.Content}
		}
	}

	answer, err := s.llmClient.ChatWithHistory(ctx, systemPrompt, history, payload.Text)
	if err != nil {
		log.Printf("c5: dooray: LLM error: %v", err)
		postToDooray(ctx, webhookURL, "⚠️ LLM 오류가 발생했습니다.")
		return
	}

	// 5. Persist turn — before dispatch so even action answers are part of history.
	appendMsgs := []conversation.Message{
		{Role: "user", Content: payload.Text},
		{Role: "assistant", Content: answer},
	}
	if err := s.convStore.Append(ctx, payload.ChannelID, "dooray", projectID, appendMsgs); err != nil {
		log.Printf("c5: dooray: conversation append error: %v", err)
	}

	// Async knowledge ingestion — fire-and-forget, non-fatal.
	if s.knowledgeClient != nil && projectID != "" {
		go func() {
			title := "대화: " + payload.ChannelID + " " + time.Now().Format("2006-01-02")
			body := "User: " + payload.Text + "\nAssistant: " + answer
			if err := s.knowledgeClient.Record(context.Background(), projectID, title, body); err != nil {
				log.Printf("c5: dooray: knowledge record error: %v", err)
			}
		}()
	}

	// 6. Dispatch on structured action or post plain text.
	if action, ok := extractAction(answer); ok {
		switch action.Action {
		case "submit_job":
			tags := []string{"dooray", "experiment"}
			if action.ExpID != "" {
				tags = append(tags, action.ExpID)
			}
			req := model.JobSubmitRequest{
				Name:        action.Name,
				Command:     action.Command,
				RequiresGPU: action.RequiresGPU,
				ExpID:       action.ExpID,
				Memo:        action.Memo,
				Tags:        tags,
				Workdir:     ".",
				Env:         map[string]string{"DOORAY_CHANNEL": payload.ChannelID},
			}
			job, err := s.store.CreateJob(&req)
			if err != nil {
				log.Printf("c5: dooray: job submit error: %v", err)
				postToDooray(ctx, webhookURL, "⚠️ 잡 제출 실패: "+err.Error())
				return
			}
			s.notifyJobAvailable()
			gpuMark := ""
			if action.RequiresGPU {
				gpuMark = " 🖥️GPU"
			}
			postToDooray(ctx, webhookURL, fmt.Sprintf("🚀 실험 잡 제출됨%s\n이름: %s\nID: %s\n커맨드: %s", gpuMark, action.Name, job.ID, action.Command))
		case "query_status":
			s.handleActionQueryStatus(ctx, webhookURL)
		case "query_workers":
			s.handleActionQueryWorkers(ctx, webhookURL)
		case "query_jobs":
			s.handleActionQueryJobs(ctx, webhookURL, action.Limit, action.Status)
		case "invoke_capability":
			s.handleActionInvokeCapability(ctx, webhookURL, payload.ChannelID, projectID, action.Capability, action.Name, action.Params)
		default:
			log.Printf("c5: dooray: unknown action %q — posting as text", action.Action)
			postToDooray(ctx, webhookURL, sanitizeDoorayText(answer))
		}
		return
	}

	// 7. Plain text answer — sanitize and post.
	postToDooray(ctx, webhookURL, sanitizeDoorayText(answer))
}

// resolveWebhookURL returns the webhook URL for the given channelID.
// Per-channel URL takes precedence; falls back to the server default.
func (s *Server) resolveWebhookURL(channelID string) string {
	if ch, ok := s.channelMap[channelID]; ok && ch.WebhookURL != "" {
		return ch.WebhookURL
	}
	return s.doorayWebhookURL
}

// formatKnowledgeContext converts search results to a numbered text block
// suitable for inclusion in a system prompt.
func formatKnowledgeContext(results []knowledge.SearchResult) string {
	var sb strings.Builder
	for i, r := range results {
		fmt.Fprintf(&sb, "[%d] %s\n%s\n\n", i+1, r.Title, r.Body)
	}
	return strings.TrimSpace(sb.String())
}

// handleActionQueryStatus fetches workers and running/queued jobs, cross-references
// them (worker → current job), and posts a combined status report to Dooray.
func (s *Server) handleActionQueryStatus(ctx context.Context, webhookURL string) {
	workers, err := s.store.ListWorkers("")
	if err != nil {
		log.Printf("c5: dooray: query_status workers: %v", err)
		postToDooray(ctx, webhookURL, "⚠️ 상태 조회 실패: "+err.Error())
		return
	}

	running, _ := s.store.ListJobs("RUNNING", "", 20, 0)
	queued, _ := s.store.ListJobs("QUEUED", "", 10, 0)

	// Build workerID → running job map for cross-reference.
	workerJob := map[string]*model.Job{}
	for _, j := range running {
		if j.WorkerID != "" {
			workerJob[j.WorkerID] = j
		}
	}

	var sb strings.Builder

	// Workers section.
	if len(workers) == 0 {
		sb.WriteString("📋 워커: 없음\n")
	} else {
		fmt.Fprintf(&sb, "📋 워커 (%d개)\n", len(workers))
		for _, w := range workers {
			gpuInfo := ""
			if w.GPUModel != "" {
				gpuInfo = fmt.Sprintf(" | %s %.0f/%.0fGB", w.GPUModel, w.FreeVRAM, w.TotalVRAM)
			}
			age := time.Since(w.LastHeartbeat).Round(time.Second)
			currentJob := ""
			if j, ok := workerJob[w.ID]; ok {
				expInfo := ""
				if j.ExpID != "" {
					expInfo = " [" + j.ExpID + "]"
				}
				currentJob = " → 실행중: " + j.Name + expInfo
			}
			fmt.Fprintf(&sb, "• %s — %s%s%s | %s 전\n", w.Hostname, w.Status, gpuInfo, currentJob, age)
		}
	}

	sb.WriteString("\n")

	// Jobs section.
	if len(running) == 0 && len(queued) == 0 {
		sb.WriteString("🔄 대기/실행 중인 잡 없음")
	} else {
		if len(running) > 0 {
			fmt.Fprintf(&sb, "🔄 실행 중 (%d개)\n", len(running))
			for _, j := range running {
				shortID := j.ID
				if len(shortID) > 8 {
					shortID = shortID[:8]
				}
				expInfo := ""
				if j.ExpID != "" {
					expInfo = " [" + j.ExpID + "]"
				}
				fmt.Fprintf(&sb, "• %s%s %s\n", shortID, expInfo, j.Name)
			}
		}
		if len(queued) > 0 {
			fmt.Fprintf(&sb, "⏳ 대기 중 (%d개)\n", len(queued))
			for _, j := range queued {
				shortID := j.ID
				if len(shortID) > 8 {
					shortID = shortID[:8]
				}
				expInfo := ""
				if j.ExpID != "" {
					expInfo = " [" + j.ExpID + "]"
				}
				fmt.Fprintf(&sb, "• %s%s %s\n", shortID, expInfo, j.Name)
			}
		}
	}

	postToDooray(ctx, webhookURL, strings.TrimSpace(sb.String()))
}

// handleActionQueryWorkers fetches the worker list and posts it to Dooray.
func (s *Server) handleActionQueryWorkers(ctx context.Context, webhookURL string) {
	workers, err := s.store.ListWorkers("")
	if err != nil {
		log.Printf("c5: dooray: list workers: %v", err)
		postToDooray(ctx, webhookURL, "⚠️ 워커 목록 조회 실패: "+err.Error())
		return
	}
	if len(workers) == 0 {
		postToDooray(ctx, webhookURL, "📋 현재 등록된 워커가 없습니다.")
		return
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "📋 워커 목록 (%d개)\n\n", len(workers))
	for _, w := range workers {
		gpuInfo := ""
		if w.GPUModel != "" {
			gpuInfo = fmt.Sprintf(" | GPU: %s (%.0fGB)", w.GPUModel, w.TotalVRAM)
		}
		age := time.Since(w.LastHeartbeat).Round(time.Second)
		fmt.Fprintf(&sb, "• %s — %s%s | 마지막 신호: %s 전\n", w.Hostname, w.Status, gpuInfo, age)
	}
	postToDooray(ctx, webhookURL, strings.TrimSpace(sb.String()))
}

// handleActionQueryJobs fetches the job list and posts it to Dooray.
func (s *Server) handleActionQueryJobs(ctx context.Context, webhookURL string, limit int, status string) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	status = strings.ToUpper(strings.TrimSpace(status))
	jobs, err := s.store.ListJobs(status, "", limit, 0)
	if err != nil {
		log.Printf("c5: dooray: list jobs: %v", err)
		postToDooray(ctx, webhookURL, "⚠️ 잡 목록 조회 실패: "+err.Error())
		return
	}
	if len(jobs) == 0 {
		postToDooray(ctx, webhookURL, "📋 조건에 맞는 잡이 없습니다.")
		return
	}
	label := "최근"
	if status != "" {
		label = status
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "📋 잡 목록 (%s, %d개)\n\n", label, len(jobs))
	for _, j := range jobs {
		expInfo := ""
		if j.ExpID != "" {
			expInfo = " [" + j.ExpID + "]"
		}
		shortID := j.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		fmt.Fprintf(&sb, "• %s%s %s — %s\n", shortID, expInfo, j.Name, string(j.Status))
	}
	postToDooray(ctx, webhookURL, strings.TrimSpace(sb.String()))
}

// handleActionInvokeCapability creates a capability job and notifies Dooray.
func (s *Server) handleActionInvokeCapability(ctx context.Context, webhookURL, channelID, projectID, capability, name string, params map[string]any) {
	if capability == "" {
		postToDooray(ctx, webhookURL, "⚠️ capability 이름이 없습니다.")
		return
	}
	regs, err := s.store.FindCapability(capability, projectID)
	if err != nil || len(regs) == 0 {
		postToDooray(ctx, webhookURL, "⚠️ capability를 찾을 수 없습니다: "+capability)
		return
	}
	command := regs[0].Command
	if command == "" {
		command = "__capability__:" + capability
	}
	if name == "" {
		name = capability
	}
	job, err := s.store.CreateJob(&model.JobSubmitRequest{
		Name:       name,
		Workdir:    ".",
		Command:    command,
		ProjectID:  projectID,
		Capability: capability,
		Params:     params,
		Env:        map[string]string{"DOORAY_CHANNEL": channelID},
	})
	if err != nil {
		log.Printf("c5: dooray: invoke capability %q: %v", capability, err)
		postToDooray(ctx, webhookURL, "⚠️ capability 실행 실패: "+err.Error())
		return
	}
	s.notifyJobAvailable()
	shortID := job.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	postToDooray(ctx, webhookURL, fmt.Sprintf("⚙️ capability 실행 요청됨\n이름: %s\n잡 ID: %s", name, shortID))
}

// buildDooraySystemPrompt assembles the system prompt with optional project,
// knowledge, and capability context.
func buildDooraySystemPrompt(projectID, knowledgeCtx, capsCtx string) string {
	prompt := `당신은 CQ 봇입니다. Google/Gemini/OpenAI를 언급하지 마세요.

⚠️ 핵심 규칙: 상태/현황/목록 조회 요청에 절대 텍스트로 설명하거나 추측하지 마세요.
실제 데이터가 없으므로 반드시 JSON action으로 서버에 위임합니다.

## 사용 가능한 액션 (JSON만 반환)

### 종합 현황 (워커 + 실행/대기 잡 통합) ← 기본 선택
"현황 체크", "상태 봐바", "실험상황 및 워커", "뭐 돌고 있어", "지금 상태",
"현재 상황", "서버 상태", "GPU 현황", "지금 뭐 돌려", "현황 보고", "상황 어때" 등
→ {"action":"query_status"}

### 잡 목록 상세 조회
"최근 실험", "실패한 잡", "잡 목록", "완료된 거 보여줘" 등
→ {"action":"query_jobs","limit":10,"status":""}

### 워커 목록 상세 조회
"워커만 봐", "등록된 워커", "워커 목록만" 등
→ {"action":"query_workers"}

### 실험 실행
"exp401 실행", "TTO 학습 시작" 등
→ {"action":"submit_job","name":"<실험명>","command":"<전체 실행 커맨드>","requires_gpu":true,"exp_id":"<expXXX>","memo":"<한줄 설명>"}

### 일반 질문 (위 4가지 외)
위에 해당하지 않는 질문만 텍스트로 답변하세요.

## hmr_postproc 프로젝트 스크립트 매핑 (pi-server: /home/pi/git/hmr_postproc)
- exp401 (TTO Baseline): uv run python /home/pi/git/hmr_postproc/scripts/train_tto_baseline.py
- exp410 (SAC Env): uv run python /home/pi/git/hmr_postproc/scripts/train_rl_refinement.py --mode sac
- exp411 (SAC+DualLoss): uv run python /home/pi/git/hmr_postproc/scripts/train_rl_refinement.py --mode sac --dual_loss
- exp700 (Multi-backbone LOO): uv run python /home/pi/git/hmr_postproc/scripts/exp700_4backbone_loo.py
- exp750 (DiNOv3 conditioned): uv run python /home/pi/git/hmr_postproc/scripts/exp750_dinov3_conditioned.py
- 평가: uv run python /home/pi/git/hmr_postproc/scripts/eval_rl_refinement.py`

	if capsCtx != "" {
		prompt += capsCtx
	}
	if projectID != "" {
		prompt += "\n\n프로젝트 ID: " + projectID
	}
	if knowledgeCtx != "" {
		prompt += "\n\n관련 프로젝트 지식:\n" + knowledgeCtx
	}
	return prompt
}

// sanitizeDoorayText removes characters that cause Dooray HTTP 400 errors
// (zero-width spaces, BOM, and other problematic Unicode control characters).
func sanitizeDoorayText(text string) string {
	var sb strings.Builder
	sb.Grow(len(text))
	for _, r := range text {
		switch r {
		case '\u200b', '\u200c', '\u200d', '\ufeff', '\u00ad':
			// zero-width space/non-joiner/joiner, BOM, soft hyphen — skip
			continue
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// postToDooray sends a message to a Dooray Incoming Webhook.
// Format: {"botName":"CQ","text":"..."}
func postToDooray(ctx context.Context, webhookURL, text string) {
	body, err := json.Marshal(map[string]string{
		"botName": "CQ",
		"text":    text,
	})
	if err != nil {
		log.Printf("c5: dooray: marshal webhook body: %v", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		log.Printf("c5: dooray: create webhook request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := doorayHTTPClient.Do(req)
	if err != nil {
		log.Printf("c5: dooray: post webhook: %v", err)
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck // drain for connection reuse
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("c5: dooray: webhook returned status %d", resp.StatusCode)
	}
}
