package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/botstore"
	"github.com/spf13/cobra"
)

// lsCmd lists bots (project-local + global) in a tabular format.
var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List registered bots",
	Long: `List Telegram bots registered for this project and globally.

Project bots are stored in .c4/bots/ and global bots in ~/.claude/bots/.
Use 'cq sessions' to list named Claude Code sessions.`,
	Args: cobra.NoArgs,
	RunE: runBotList,
}

// removeCmd deletes a bot configuration directory after confirmation.
var removeCmd = &cobra.Command{
	Use:   "remove <username>",
	Short: "Remove a registered bot",
	Long: `Remove a Telegram bot from the local or global registry.

This deletes the bot's directory (.c4/bots/<username>/ or ~/.claude/bots/<username>/).
The bot token itself is NOT revoked — use BotFather to revoke: https://t.me/BotFather`,
	Args: cobra.ExactArgs(1),
	RunE: runBotRemove,
}

func init() {
	rootCmd.AddCommand(lsCmd, removeCmd)
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
		fmt.Println("No bots registered. Use 'cq bot add <token>' to add one.")
		return nil
	}

	// Compute column widths.
	maxUser := 8
	for _, b := range bots {
		if w := len(b.Username); w > maxUser {
			maxUser = w
		}
	}

	fmt.Printf("%-*s  %-7s  %-13s  %s\n", maxUser, "USERNAME", "SCOPE", "LAST_ACTIVE", "DISPLAY_NAME")
	fmt.Println(strings.Repeat("-", maxUser+2+7+2+13+2+20))
	for _, b := range bots {
		lastActive := "--"
		if !b.LastActive.IsZero() {
			lastActive = b.LastActive.In(time.Local).Format("Jan 02 15:04")
		}
		displayName := b.DisplayName
		if displayName == "" {
			displayName = "--"
		}
		fmt.Printf("%-*s  %-7s  %-13s  %s\n", maxUser, b.Username, b.Scope, lastActive, displayName)
	}

	return nil
}

func runBotRemove(cmd *cobra.Command, args []string) error {
	username := args[0]

	store, err := botstore.New(projectDir)
	if err != nil {
		return fmt.Errorf("opening bot store: %w", err)
	}

	// Verify bot exists before prompting.
	bot, err := store.Get(username)
	if err != nil {
		return fmt.Errorf("bot %q not found", username)
	}

	// Confirmation prompt (skipped with --yes).
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

	fmt.Printf("Removed bot @%s\n", username)
	fmt.Println()
	fmt.Println("Note: The bot token is still active. To revoke it, use BotFather:")
	fmt.Println("  https://t.me/BotFather  → /mybots → select bot → Revoke Token")

	return nil
}
