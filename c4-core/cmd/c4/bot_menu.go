package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/changmin/c4-core/internal/botstore"
)

// botLaunch selects a bot and launches claude with telegram.
// Used when -b is the only flag (no -t).
func botLaunch(name string) error {
	store, err := botstore.New(projectDir)
	if err != nil {
		return fmt.Errorf("botstore: %w", err)
	}

	if name != "" && name != " " {
		return botLaunchByName(store, name)
	}
	return botMenuAndLaunch(store)
}

// botLaunchByName finds a bot by username and launches.
func botLaunchByName(store *botstore.Store, name string) error {
	bots, err := store.List()
	if err != nil {
		return fmt.Errorf("botstore list: %w", err)
	}
	name = strings.TrimPrefix(name, "@")
	for _, bot := range bots {
		if strings.EqualFold(bot.Username, name) {
			return launchWithBot(bot)
		}
	}
	return fmt.Errorf("봇 '%s'을(를) 찾을 수 없습니다. cq --bot 으로 목록을 확인하세요.", name)
}

// botMenuAndLaunch shows menu, selects bot, and launches (used when -b only, no -t).
func botMenuAndLaunch(store *botstore.Store) error {
	bot, err := botMenuSelect(store)
	if err != nil {
		return err
	}
	if bot == nil {
		return nil // cancelled
	}
	return launchWithBot(*bot)
}

// botMenuSelect shows interactive bot selection menu and returns the selected bot.
// Returns nil if user cancelled. Does NOT launch — caller decides what to do.
func botMenuSelect(store *botstore.Store) (*botstore.Bot, error) {
	bots, err := store.List()
	if err != nil {
		return nil, fmt.Errorf("botstore list: %w", err)
	}

	if len(bots) == 0 {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "등록된 봇이 없습니다.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  1) 새 봇 만들기")
		fmt.Fprintln(os.Stderr, "  2) 취소")
		fmt.Fprintln(os.Stderr, "")

		scanner := bufio.NewScanner(os.Stdin)
		for {
			fmt.Fprint(os.Stderr, "선택 [1-2]: ")
			if !scanner.Scan() {
				return nil, nil
			}
			switch strings.TrimSpace(scanner.Text()) {
			case "1":
				bot, err := runSetupWizardInline(store)
				if err != nil {
					return nil, err
				}
				return &bot, nil
			case "2":
				return nil, nil
			default:
				fmt.Fprintln(os.Stderr, "1 또는 2를 입력하세요.")
			}
		}
	}

	printBotMenu(bots)

	newBotIdx := len(bots) + 1
	cancelIdx := len(bots) + 2

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Fprintf(os.Stderr, "선택 [1-%d]: ", cancelIdx)
		if !scanner.Scan() {
			return nil, nil
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		choice, err := strconv.Atoi(line)
		if err != nil {
			choice = -1
		}

		switch {
		case choice >= 1 && choice <= len(bots):
			return &bots[choice-1], nil
		case choice == newBotIdx:
			bot, err := runSetupWizardInline(store)
			if err != nil {
				return nil, err
			}
			return &bot, nil
		case choice == cancelIdx:
			return nil, nil
		default:
			fmt.Fprintf(os.Stderr, "잘못된 입력입니다. 1-%d 사이의 숫자를 입력하세요.\n", cancelIdx)
		}
	}
}

// printBotMenu prints the numbered bot list to stderr.
func printBotMenu(bots []botstore.Bot) {
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "봇을 선택하세요:")
	fmt.Fprintln(os.Stderr, "")

	prevScope := ""
	for i, bot := range bots {
		if bot.Scope != prevScope {
			switch bot.Scope {
			case "project":
				fmt.Fprintln(os.Stderr, "  [프로젝트 봇]")
			case "global":
				fmt.Fprintln(os.Stderr, "  [글로벌 봇]")
			}
			prevScope = bot.Scope
		}
		name := bot.DisplayName
		if name == "" {
			name = "@" + bot.Username
		} else {
			name = fmt.Sprintf("%s (@%s)", name, bot.Username)
		}
		fmt.Fprintf(os.Stderr, "  %d) %s\n", i+1, name)
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "  %d) 새 봇 만들기\n", len(bots)+1)
	fmt.Fprintf(os.Stderr, "  %d) 취소\n", len(bots)+2)
	fmt.Fprintln(os.Stderr, "")
}

// launchWithBot sets the bot token and launches claude with telegram.
func launchWithBot(bot botstore.Bot) error {
	if err := os.Setenv("C4_TELEGRAM_BOT_TOKEN", bot.Token); err != nil {
		return fmt.Errorf("set env: %w", err)
	}
	fmt.Fprintf(os.Stderr, "봇 선택: @%s\n", bot.Username)
	return initAndLaunch("claude")
}
