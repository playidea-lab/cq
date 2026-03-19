package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/changmin/c4-core/internal/c1push"
	"github.com/changmin/c4-core/internal/chat"
	"github.com/spf13/cobra"
)

var chatCmd = &cobra.Command{
	Use:   "chat [channel-name]",
	Short: "Open an interactive chat session",
	Long: `Connect to a CQ chat channel and exchange messages in real-time.

If no channel name is given, defaults to "general".

Keyboard shortcuts:
  /sessions   List and switch channels
  /quit       Exit (or Ctrl+C / Esc)
  PgUp/PgDn   Scroll history`,
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

	// 4. Run bubbletea TUI (handles realtime subscribe + send internally)
	return runTUI(ctx, supabaseURL, anonKey, accessToken, channelID, channelName)
}

// runTUI wires up the bubbletea TUI with the realtime chat client.
// Side effects (send, channel switch) are handled in a pump goroutine.
func runTUI(
	ctx context.Context,
	supabaseURL, anonKey, accessToken string,
	channelID, channelName string,
) error {
	sendCh := make(chan string, 16)
	switchCh := make(chan chat.ChannelSwitchedMsg, 4)

	model := &sideEffectModel{
		inner:    chat.NewTUIModel(channelID, channelName),
		sendCh:   sendCh,
		switchCh: switchCh,
	}
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Subscribe to realtime
	chatClient := chat.New(supabaseURL, anonKey, accessToken)
	chatClient.Subscribe(
		fmt.Sprintf("channel_id=eq.%s", channelID),
		func(msg chat.Message) { p.Send(chat.IncomingMsg(msg)) },
	)
	if err := chatClient.Connect(ctx); err != nil {
		return fmt.Errorf("connecting to realtime: %w", err)
	}
	defer chatClient.Close()

	// Preload channel list
	go func() {
		channels, listErr := listChatChannels(supabaseURL, anonKey, accessToken)
		if listErr == nil {
			msgs := make(chat.ChannelListMsg, len(channels))
			for i, ch := range channels {
				msgs[i] = chat.ChatChannel(ch.ID, ch.Name)
			}
			p.Send(msgs)
		}
	}()

	// Pump: handle outbound messages and channel switches
	go func() {
		currentChannelID := channelID
		currentClient := chatClient
		for {
			select {
			case <-ctx.Done():
				return
			case content := <-sendCh:
				sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				if err := currentClient.SendMessage(sendCtx, currentChannelID, content, "user"); err != nil {
					p.Send(chat.ErrorMsg{Err: err})
				}
				cancel()
			case sw := <-switchCh:
				currentChannelID = sw.ChannelID
				currentClient.Close()
				nc := chat.New(supabaseURL, anonKey, accessToken)
				nc.Subscribe(
					fmt.Sprintf("channel_id=eq.%s", sw.ChannelID),
					func(msg chat.Message) { p.Send(chat.IncomingMsg(msg)) },
				)
				if err := nc.Connect(ctx); err != nil {
					p.Send(chat.ErrorMsg{Err: fmt.Errorf("realtime: %w", err)})
					return
				}
				currentClient = nc
			}
		}
	}()

	_, err := p.Run()
	return err
}

// sideEffectModel wraps TUIModel to intercept SendRequestMsg / ChannelSwitchedMsg.
type sideEffectModel struct {
	inner    chat.TUIModel
	sendCh   chan<- string
	switchCh chan<- chat.ChannelSwitchedMsg
}

func (s *sideEffectModel) Init() tea.Cmd { return s.inner.Init() }

func (s *sideEffectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case chat.SendRequestMsg:
		select {
		case s.sendCh <- m.Content:
		default:
		}
	case chat.ChannelSwitchedMsg:
		select {
		case s.switchCh <- m:
		default:
		}
	}
	newInner, cmd := s.inner.Update(msg)
	s.inner = newInner.(chat.TUIModel)
	return s, cmd
}

func (s *sideEffectModel) View() string { return s.inner.View() }

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
