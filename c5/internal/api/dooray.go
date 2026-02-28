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

	"github.com/piqsol/c4/c5/internal/knowledge"
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
		go s.processDoorayServerSide(payload)
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

// processDoorayServerSide handles server-side LLM processing in a goroutine.
// It queries the knowledge base (if configured), calls the LLM, and posts the
// response to the Dooray Incoming Webhook.
func (s *Server) processDoorayServerSide(payload doorayInbound) {
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

	// 3. Build system prompt and call LLM.
	systemPrompt := buildDooraySystemPrompt(projectID, knowledgeCtx)
	answer, err := s.llmClient.Chat(ctx, systemPrompt, payload.Text)
	if err != nil {
		log.Printf("c5: dooray: LLM error: %v", err)
		postToDooray(ctx, webhookURL, "⚠️ LLM 오류가 발생했습니다.")
		return
	}

	// 4. Sanitize and post.
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

// buildDooraySystemPrompt assembles the system prompt with optional project
// and knowledge context.
func buildDooraySystemPrompt(projectID, knowledgeCtx string) string {
	prompt := "당신은 Dooray 메신저 봇입니다. 사용자의 질문에 간결하고 정확하게 답변하세요."
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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("c5: dooray: webhook returned status %d", resp.StatusCode)
	}
}
