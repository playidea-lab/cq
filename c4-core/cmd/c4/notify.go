package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var notifyCmd = &cobra.Command{
	Use:   "notify",
	Short: "Manage notification channels",
	Long: `Manage C4 notification channels.

Subcommands:
  add telegram  - Pair a Telegram bot for notifications
  list          - List configured notification channels
  remove <id>   - Remove a notification channel`,
}

var notifyAddChatID string

var notifyAddCmd = &cobra.Command{
	Use:   "add <channel>",
	Short: "Add a notification channel (e.g. telegram)",
	Long: `Add a notification channel.

Examples:
  cq notify add telegram --chat-id 123456789   # Direct registration
  cq notify add telegram                        # Interactive pairing via bot`,
	Args: cobra.ExactArgs(1),
	RunE: runNotifyAdd,
}

var notifyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured notification channels",
	Args:  cobra.NoArgs,
	RunE:  runNotifyList,
}

var notifyRemoveCmd = &cobra.Command{
	Use:   "remove <id>",
	Short: "Remove a notification channel",
	Args:  cobra.ExactArgs(1),
	RunE:  runNotifyRemove,
}

func init() {
	notifyAddCmd.Flags().StringVar(&notifyAddChatID, "chat-id", "", "Telegram chat ID (skip bot pairing)")
	notifyCmd.AddCommand(notifyAddCmd)
	notifyCmd.AddCommand(notifyListCmd)
	notifyCmd.AddCommand(notifyRemoveCmd)
	rootCmd.AddCommand(notifyCmd)
}

// notifySession holds the minimal session fields we need.
type notifySession struct {
	AccessToken string `json:"access_token"`
	UserID      string `json:"-"` // populated from user sub-object
	User        struct {
		ID string `json:"id"`
	} `json:"user"`
}

// readNotifySession loads the session from ~/.c4/session.json.
func readNotifySession() (*notifySession, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}
	sessionPath := filepath.Join(home, ".c4", "session.json")
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		return nil, fmt.Errorf("run cq auth login first")
	}
	var s notifySession
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("invalid session file: %w", err)
	}
	if s.AccessToken == "" {
		return nil, fmt.Errorf("run cq auth login first")
	}
	s.UserID = s.User.ID
	return &s, nil
}

// generatePairingCode returns "CQ-" + 4 random hex bytes (8 hex chars).
func generatePairingCode() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating pairing code: %w", err)
	}
	return "CQ-" + hex.EncodeToString(b), nil
}

// supabasePost performs a POST to the Supabase PostgREST endpoint.
func supabasePost(supabaseURL, jwt, table string, body interface{}) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := supabaseURL + "/rest/v1/" + table
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("apikey", readCloudAnonKey(projectDir))
	req.Header.Set("Prefer", "return=representation")
	return http.DefaultClient.Do(req)
}

// supabaseGet performs a GET with query params to the Supabase PostgREST endpoint.
func supabaseGet(supabaseURL, jwt, table, query string) (*http.Response, error) {
	url := supabaseURL + "/rest/v1/" + table + "?" + query
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("apikey", readCloudAnonKey(projectDir))
	return http.DefaultClient.Do(req)
}

// supabaseDelete performs a DELETE with query params to the Supabase PostgREST endpoint.
func supabaseDelete(supabaseURL, jwt, table, query string) (*http.Response, error) {
	url := supabaseURL + "/rest/v1/" + table + "?" + query
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("apikey", readCloudAnonKey(projectDir))
	return http.DefaultClient.Do(req)
}

func runNotifyAdd(cmd *cobra.Command, args []string) error {
	channel := args[0]
	if channel != "telegram" {
		return fmt.Errorf("unsupported channel %q: only 'telegram' is supported", channel)
	}

	supabaseURL := readCloudURL(projectDir)
	if supabaseURL == "" {
		return fmt.Errorf("no Supabase URL configured (set C4_CLOUD_URL or run cq init)")
	}

	projectID := getActiveProjectID(projectDir)
	if projectID == "" {
		return fmt.Errorf("no project_id configured: run cq init or set cloud.active_project_id")
	}

	session, err := readNotifySession()
	if err != nil {
		return err
	}

	// Direct registration with --chat-id
	if notifyAddChatID != "" {
		return registerNotifyChannel(supabaseURL, session.AccessToken, projectID, channel, notifyAddChatID)
	}

	// Interactive pairing via bot
	code, err := generatePairingCode()
	if err != nil {
		return err
	}

	expiresAt := time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339)
	pairingBody := map[string]interface{}{
		"code":         code,
		"user_id":      session.UserID,
		"project_id":   projectID,
		"channel_type": "telegram",
		"expires_at":   expiresAt,
	}

	resp, err := supabasePost(supabaseURL, session.AccessToken, "notification_pairings", pairingBody)
	if err != nil {
		return fmt.Errorf("creating pairing: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("creating pairing failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	fmt.Printf("텔레그램 봇에 다음 메시지를 보내세요:\n/start %s\n\n대기 중...\n", code)

	// Poll every 3 seconds for up to 5 minutes.
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)

		pollResp, err := supabaseGet(supabaseURL, session.AccessToken, "notification_pairings",
			"code=eq."+code+"&select=used")
		if err != nil {
			continue
		}
		pollBody, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		var rows []struct {
			Used bool `json:"used"`
		}
		if err := json.Unmarshal(pollBody, &rows); err != nil || len(rows) == 0 {
			continue
		}
		if rows[0].Used {
			fmt.Println("✅ 텔레그램 알림 설정 완료")
			return nil
		}
	}

	fmt.Println("⏰ 시간 초과. 다시 시도하세요.")
	return nil
}

// registerNotifyChannel inserts a notification channel directly.
func registerNotifyChannel(supabaseURL, jwt, projectID, channelType, chatID string) error {
	channelBody := map[string]interface{}{
		"project_id":   projectID,
		"channel_type": channelType,
		"config":       map[string]string{"chat_id": chatID},
	}

	resp, err := supabasePost(supabaseURL, jwt, "project_notification_channels", channelBody)
	if err != nil {
		return fmt.Errorf("registering channel: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("registering channel failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	fmt.Printf("✅ 텔레그램 알림 설정 완료 (chat_id: %s)\n", chatID)
	return nil
}

func runNotifyList(cmd *cobra.Command, args []string) error {
	supabaseURL := readCloudURL(projectDir)
	if supabaseURL == "" {
		return fmt.Errorf("no Supabase URL configured (set C4_CLOUD_URL or run cq init)")
	}

	projectID := getActiveProjectID(projectDir)
	if projectID == "" {
		return fmt.Errorf("no project_id configured: run cq init or set cloud.active_project_id")
	}

	session, err := readNotifySession()
	if err != nil {
		return err
	}

	resp, err := supabaseGet(supabaseURL, session.AccessToken, "project_notification_channels",
		"project_id=eq."+projectID+"&select=id,channel_type,events,created_at")
	if err != nil {
		return fmt.Errorf("fetching channels: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("fetching channels failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var channels []struct {
		ID          string   `json:"id"`
		ChannelType string   `json:"channel_type"`
		Events      []string `json:"events"`
		CreatedAt   string   `json:"created_at"`
	}
	if err := json.Unmarshal(body, &channels); err != nil {
		return fmt.Errorf("parsing channels: %w", err)
	}

	if len(channels) == 0 {
		fmt.Println("알림 채널이 없습니다.")
		return nil
	}

	fmt.Printf("%-36s  %-10s  %-20s  %s\n", "ID", "Type", "Events", "Created")
	fmt.Printf("%-36s  %-10s  %-20s  %s\n",
		"------------------------------------",
		"----------",
		"--------------------",
		"-------------------")
	for _, ch := range channels {
		created := ch.CreatedAt
		if len(created) > 19 {
			created = created[:19]
		}
		events := strings.Join(ch.Events, ",")
		fmt.Printf("%-36s  %-10s  %-20s  %s\n", ch.ID, ch.ChannelType, events, created)
	}
	return nil
}

func runNotifyRemove(cmd *cobra.Command, args []string) error {
	id := args[0]

	supabaseURL := readCloudURL(projectDir)
	if supabaseURL == "" {
		return fmt.Errorf("no Supabase URL configured (set C4_CLOUD_URL or run cq init)")
	}

	session, err := readNotifySession()
	if err != nil {
		return err
	}

	projectID := getActiveProjectID(projectDir)
	if projectID == "" {
		return fmt.Errorf("no project_id configured: run cq init or set cloud.active_project_id")
	}

	resp, err := supabaseDelete(supabaseURL, session.AccessToken, "project_notification_channels",
		"id=eq."+id+"&project_id=eq."+projectID)
	if err != nil {
		return fmt.Errorf("deleting channel: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("deleting channel failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	fmt.Println("✅ 알림 채널 삭제 완료")
	return nil
}
