package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/chat"
	"github.com/changmin/c4-core/internal/c1push"
	"github.com/spf13/cobra"
)

var chatCmd = &cobra.Command{
	Use:   "chat [channel-name]",
	Short: "Open an interactive chat session",
	Long: `Connect to a CQ chat channel and exchange messages in real-time.

If no channel name is given, defaults to "general".

Special commands:
  /sessions   List available chat channels
  /quit       Exit the chat`,
	Args: cobra.MaximumNArgs(1),
	RunE: runChat,
}

func init() {
	rootCmd.AddCommand(chatCmd)
}

func runChat(cmd *cobra.Command, args []string) error {
	// 1. Auth check
	supabaseURL := readCloudURL(projectDir)
	anonKey := readCloudAnonKey(projectDir)
	if supabaseURL == "" || anonKey == "" {
		return fmt.Errorf("not configured: run 'cq auth login' first")
	}

	authClient, err := newAuthClient()
	if err != nil {
		return fmt.Errorf("not authenticated: run 'cq auth login'")
	}
	session, err := authClient.GetSession()
	if err != nil || session == nil {
		fmt.Fprintln(os.Stderr, "Not logged in. Run: cq auth login")
		return fmt.Errorf("authentication required")
	}
	if time.Now().Unix() >= session.ExpiresAt {
		fmt.Fprintln(os.Stderr, "Session expired. Run: cq auth login")
		return fmt.Errorf("session expired")
	}

	accessToken := session.AccessToken
	userName := session.User.Email
	if userName == "" {
		userName = session.User.Name
	}
	if userName == "" {
		userName = "user"
	}

	// 2. Resolve channel name
	channelName := "general"
	if len(args) > 0 {
		channelName = args[0]
	}

	// 3. Ensure channel exists
	pusher := c1push.New(supabaseURL, anonKey)
	if pusher == nil {
		return fmt.Errorf("failed to create pusher: check cloud config")
	}
	ctx := context.Background()
	channelID, err := pusher.EnsureChannel(ctx, session.User.ID, "", channelName, c1push.PlatformClaudeCode)
	if err != nil {
		return fmt.Errorf("entering channel %q: %w", channelName, err)
	}
	if channelID == "" {
		return fmt.Errorf("channel %q not found and could not be created", channelName)
	}

	fmt.Printf("Joined #%s (id: %s)\n", channelName, channelID)
	fmt.Println("Type /sessions to list channels, /quit to exit.")
	fmt.Println(strings.Repeat("─", 60))

	// 4. Subscribe to real-time messages
	chatClient := chat.New(supabaseURL, anonKey, accessToken)
	chatClient.Subscribe(
		fmt.Sprintf("channel_id=eq.%s", channelID),
		func(msg chat.Message) {
			ts := formatChatTime(msg.CreatedAt)
			fmt.Printf("\r[%s] %s: %s\n> ", ts, msg.SenderName, msg.Content)
		},
	)
	if err := chatClient.Connect(ctx); err != nil {
		return fmt.Errorf("connecting to realtime: %w", err)
	}
	defer chatClient.Close()

	// 5. Readline loop
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		switch {
		case line == "/quit" || line == "/exit":
			fmt.Println("Bye!")
			return nil

		case line == "/sessions":
			sessions, listErr := listChatChannels(supabaseURL, anonKey, accessToken)
			if listErr != nil {
				fmt.Fprintf(os.Stderr, "error listing sessions: %v\n", listErr)
			} else if len(sessions) == 0 {
				fmt.Println("(no channels found)")
			} else {
				fmt.Println("Available channels:")
				for _, s := range sessions {
					marker := " "
					if s.ID == channelID {
						marker = "*"
					}
					fmt.Printf("  %s #%s (%s)\n", marker, s.Name, s.ID)
				}
			}

		case line == "":
			// skip empty input

		default:
			sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			sendErr := chatClient.SendMessage(sendCtx, channelID, line, "user")
			cancel()
			if sendErr != nil {
				fmt.Fprintf(os.Stderr, "send error: %v\n", sendErr)
			}
		}

		fmt.Print("> ")
	}
	return scanner.Err()
}

// chatChannelRow is a minimal row from c1_channels.
type chatChannelRow struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// listChatChannels fetches channels visible to the authenticated user.
func listChatChannels(supabaseURL, anonKey, accessToken string) ([]chatChannelRow, error) {
	params := url.Values{
		"select": {"id,name"},
		"order":  {"name.asc"},
		"limit":  {"50"},
	}
	endpoint := strings.TrimRight(supabaseURL, "/") + "/rest/v1/c1_channels?" + params.Encode()
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", anonKey)
	token := anonKey
	if accessToken != "" {
		token = accessToken
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	var rows []chatChannelRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

// formatChatTime formats an ISO 8601 timestamp for display.
func formatChatTime(ts string) string {
	if ts == "" {
		return time.Now().Format("15:04")
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		// try without nanoseconds
		t, err = time.Parse("2006-01-02T15:04:05", ts)
		if err != nil {
			return ts
		}
	}
	return t.Local().Format("15:04")
}
