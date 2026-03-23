package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/botstore"
	"github.com/spf13/cobra"
)

var botCmd = &cobra.Command{
	Use:   "bot",
	Short: "Manage Telegram bots",
	Long: `Manage Telegram bots for notifications and messaging.

  cq bot add       Register a new bot
  cq bot ls        List registered bots
  cq bot remove    Remove a bot`,
}

var botAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Register a Telegram bot and enable notifications",
	Long: `Register a Telegram bot for event notifications.

Steps:
  1. Create a bot via @BotFather on Telegram (/newbot)
  2. Run 'cq bot add' and paste the token
  3. The bot is verified, your Telegram ID is saved, and notifications are enabled

After setup, EventBus events (task completion, checkpoint, hub jobs)
are automatically sent to your Telegram.`,
	Args: cobra.NoArgs,
	RunE: runBotAdd,
}

var botLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List registered bots",
	Args:  cobra.NoArgs,
	RunE:  runBotList,
}

var botRemoveCmd = &cobra.Command{
	Use:   "remove <username>",
	Short: "Remove a registered bot",
	Long: `Remove a Telegram bot from the registry.

The bot token itself is NOT revoked — use BotFather to revoke.`,
	Args: cobra.ExactArgs(1),
	RunE: runBotRemove,
}

// Keep old top-level aliases for backward compatibility (hidden).
var lsCmd = &cobra.Command{
	Use:    "ls",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE:   runBotList,
}
var removeCmd = &cobra.Command{
	Use:    "remove <username>",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE:   runBotRemove,
}

func init() {
	botCmd.AddCommand(botAddCmd, botLsCmd, botRemoveCmd)
	rootCmd.AddCommand(botCmd)
	// Backward compat: keep top-level ls/remove
	rootCmd.AddCommand(lsCmd, removeCmd)
}

func runBotAdd(cmd *cobra.Command, args []string) error {
	store, err := botstore.New(projectDir)
	if err != nil {
		return fmt.Errorf("opening bot store: %w", err)
	}

	w := os.Stderr

	fmt.Fprintln(w)
	fmt.Fprintln(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w, "  Telegram 봇 등록")
	fmt.Fprintln(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  준비물:")
	fmt.Fprintln(w, "  1. @BotFather 에서 봇 생성 → 토큰 복사")
	fmt.Fprintln(w, "  2. @userinfobot 에서 /start → 내 Telegram ID 확인")
	fmt.Fprintln(w)

	// Step 1: Token
	fmt.Fprint(w, "봇 토큰: ")
	var token string
	if _, err := fmt.Fscanln(cmd.InOrStdin(), &token); err != nil {
		return fmt.Errorf("입력 종료")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("토큰이 비어있습니다")
	}

	// Step 2: Verify token
	fmt.Fprintln(w, "  검증 중...")
	info, err := verifyTokenFunc(token)
	if err != nil {
		return fmt.Errorf("토큰 검증 실패: %w", err)
	}
	fmt.Fprintf(w, "  ✓ @%s (%s)\n\n", info.Username, info.FirstName)

	// Check if bot already registered
	if existing, err := store.Get(info.Username); err == nil && existing != nil {
		fmt.Fprintf(w, "  @%s 은 이미 등록되어 있습니다.\n", info.Username)
		if len(existing.AllowFrom) > 0 {
			fmt.Fprintf(w, "  기존 chat_id: %d\n", existing.AllowFrom[0])
		}
		if !yesAll {
			fmt.Fprint(w, "  덮어쓸까요? [y/N] ")
			var answer string
			fmt.Fscanln(cmd.InOrStdin(), &answer) //nolint:errcheck
			if answer != "y" && answer != "Y" {
				fmt.Fprintln(w, "  취소됨")
				return nil
			}
		}
		fmt.Fprintln(w)
	}

	// Step 3: Telegram ID
	fmt.Fprintln(w, "  내 Telegram ID (숫자)를 입력하세요.")
	fmt.Fprintln(w, "  모르면 @userinfobot 에게 /start 전송.")
	fmt.Fprintln(w)
	fmt.Fprint(w, "Telegram ID: ")
	var chatIDStr string
	if _, err := fmt.Fscanln(cmd.InOrStdin(), &chatIDStr); err != nil {
		return fmt.Errorf("입력 종료")
	}
	chatIDStr = strings.TrimSpace(chatIDStr)

	var chatID int64
	if _, err := fmt.Sscanf(chatIDStr, "%d", &chatID); err != nil {
		return fmt.Errorf("잘못된 ID: %q", chatIDStr)
	}

	// Step 4: Save bot
	bot := botstore.Bot{
		Username:    info.Username,
		Token:       token,
		DisplayName: info.FirstName,
		LastActive:  time.Now(),
		Scope:       "global",
		AllowFrom:   []int64{chatID},
	}
	if err := store.Save(bot); err != nil {
		return fmt.Errorf("봇 저장 실패: %w", err)
	}
	fmt.Fprintf(w, "  ✓ @%s 등록 완료\n\n", info.Username)

	// Step 5: Auto-configure notifications.json
	notifCfg := map[string]any{
		"bot_username": info.Username,
	}
	notifData, _ := json.MarshalIndent(notifCfg, "", "  ")
	c4Dir := filepath.Join(projectDir, ".c4")
	os.MkdirAll(c4Dir, 0o750) //nolint:errcheck
	notifPath := filepath.Join(c4Dir, "notifications.json")
	if err := os.WriteFile(notifPath, notifData, 0o640); err != nil {
		fmt.Fprintf(w, "  ⚠ notifications.json 저장 실패: %v\n", err)
	} else {
		fmt.Fprintln(w, "  ✓ 이벤트 알림 활성화 (.c4/notifications.json)")
	}

	// Step 6: Auto-set Supabase secrets for server-side notifications
	chatIDStr = strconv.FormatInt(chatID, 10)
	if sbPath, err := exec.LookPath("supabase"); err == nil {
		fmt.Fprintln(w, "  서버 알림 설정 중...")
		setSecret := func(key, val string) bool {
			out, err := exec.Command(sbPath, "secrets", "set", key+"="+val).CombinedOutput()
			if err != nil {
				fmt.Fprintf(w, "  ⚠ %s 설정 실패: %s\n", key, strings.TrimSpace(string(out)))
				return false
			}
			return true
		}
		okToken := setSecret("TELEGRAM_BOT_TOKEN", token)
		okChat := setSecret("TELEGRAM_CHAT_ID", chatIDStr)
		if okToken && okChat {
			fmt.Fprintln(w, "  ✓ Supabase secrets 설정 완료")
		}
	} else {
		fmt.Fprintln(w, "  ⚠ supabase CLI 없음 — 서버 알림은 수동 설정 필요:")
		fmt.Fprintf(w, "    supabase secrets set TELEGRAM_BOT_TOKEN=<토큰>\n")
		fmt.Fprintf(w, "    supabase secrets set TELEGRAM_CHAT_ID=%s\n", chatIDStr)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w, "  완료!")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  DB Webhook은 Supabase Dashboard에서 설정:")
	fmt.Fprintln(w, "    c4_tasks UPDATE → telegram-notify Edge Function")
	fmt.Fprintln(w, "    hub_jobs UPDATE → telegram-notify Edge Function")
	fmt.Fprintln(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w)

	return nil
}

func runBotList(cmd *cobra.Command, args []string) error {
	store, err := botstore.New(projectDir)
	if err != nil {
		return fmt.Errorf("opening bot store: %w", err)
	}

	bots, err := store.List()
	if err != nil {
		return fmt.Errorf("listing bots: %w", err)
	}

	if len(bots) == 0 {
		fmt.Println("No bots registered. Run 'cq bot add' to register one.")
		return nil
	}

	maxUser := 8
	for _, b := range bots {
		if w := len(b.Username); w > maxUser {
			maxUser = w
		}
	}

	fmt.Printf("%-*s  %-7s  %-13s  %-5s  %s\n", maxUser, "USERNAME", "SCOPE", "LAST_ACTIVE", "CHAT", "DISPLAY_NAME")
	fmt.Println(strings.Repeat("-", maxUser+2+7+2+13+2+5+2+20))
	for _, b := range bots {
		lastActive := "--"
		if !b.LastActive.IsZero() {
			lastActive = b.LastActive.In(time.Local).Format("Jan 02 15:04")
		}
		displayName := b.DisplayName
		if displayName == "" {
			displayName = "--"
		}
		chatStatus := "✗"
		if len(b.AllowFrom) > 0 {
			chatStatus = "✓"
		}
		fmt.Printf("%-*s  %-7s  %-13s  %-5s  %s\n", maxUser, b.Username, b.Scope, lastActive, chatStatus, displayName)
	}

	// Show notification config status
	notifPath := filepath.Join(projectDir, ".c4", "notifications.json")
	if data, err := os.ReadFile(notifPath); err == nil {
		var cfg struct {
			BotUsername string `json:"bot_username"`
		}
		if json.Unmarshal(data, &cfg) == nil && cfg.BotUsername != "" {
			fmt.Printf("\nNotify bot: @%s\n", cfg.BotUsername)
		}
	}

	return nil
}

func runBotRemove(cmd *cobra.Command, args []string) error {
	username := args[0]

	store, err := botstore.New(projectDir)
	if err != nil {
		return fmt.Errorf("opening bot store: %w", err)
	}

	bot, err := store.Get(username)
	if err != nil {
		return fmt.Errorf("bot %q not found", username)
	}

	if !yesAll {
		fmt.Printf("Remove bot @%s (%s scope)? [y/N] ", bot.Username, bot.Scope)
		var answer string
		fmt.Fscan(cmd.InOrStdin(), &answer) //nolint:errcheck
		if answer != "y" && answer != "Y" {
			fmt.Println("aborted")
			return nil
		}
	}

	if err := store.Remove(username); err != nil {
		return fmt.Errorf("removing bot: %w", err)
	}

	// If this was the notification bot, clear notifications.json
	notifPath := filepath.Join(projectDir, ".c4", "notifications.json")
	if data, err := os.ReadFile(notifPath); err == nil {
		var cfg struct {
			BotUsername string `json:"bot_username"`
		}
		if json.Unmarshal(data, &cfg) == nil && cfg.BotUsername == username {
			os.Remove(notifPath)
			fmt.Printf("Cleared notification config (was @%s)\n", username)
		}
	}

	fmt.Printf("Removed bot @%s\n", username)
	fmt.Println()
	fmt.Println("Note: The bot token is still active. To revoke it, use BotFather:")
	fmt.Println("  https://t.me/BotFather  → /mybots → select bot → Revoke Token")

	return nil
}
