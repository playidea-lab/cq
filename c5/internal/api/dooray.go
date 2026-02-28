package api

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/piqsol/c4/c5/internal/model"
)

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
// POST: validates the optional cmd token, creates a Hub Job with domain tag
// "dooray", and responds with an ephemeral acknowledgement.
func (s *Server) handleDooray(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	var payload doorayInbound
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Token verification — skipped when C5_DOORAY_CMD_TOKEN is empty.
	// Security model: subtle.ConstantTimeCompare returns 0 immediately when
	// lengths differ (length oracle). This is acceptable for static webhook
	// tokens; HMAC-based dynamic tokens would need padding.
	cmdToken := os.Getenv("C5_DOORAY_CMD_TOKEN")
	if cmdToken != "" {
		expected := []byte(cmdToken)
		appMatch := subtle.ConstantTimeCompare(expected, []byte(payload.AppToken))
		cmdMatch := subtle.ConstantTimeCompare(expected, []byte(payload.CmdToken))
		if appMatch != 1 && cmdMatch != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Build a Hub Job. The "dooray" tag is used for domain routing.
	// Command is intentionally empty — workers are keyed by env vars.
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.notifyJobAvailable()

	_ = job // job ID available for future logging

	ackText := "⏳ 수신: " + payload.Text
	if payload.Text == "" {
		ackText = "⏳ 수신 완료"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(doorayResponse{
		Text:         ackText,
		ResponseType: "ephemeral",
	})
}
