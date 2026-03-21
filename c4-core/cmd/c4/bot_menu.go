package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/changmin/c4-core/internal/botstore"
)

// botSelectMenu shows an interactive numbered menu for bot selection.
// When cq is run without subcommands, this is the default entry point.
//
// Behaviour:
//   - No bots found: print setup guidance and return.
//   - Bots found: list project bots, then global bots, then "새 봇 만들기", then "종료".
//   - On bot selection: set C4_TELEGRAM_BOT_TOKEN and launch claude.
//   - "새 봇 만들기": print guidance to run `cq setup`.
//   - "종료": exit cleanly.
func botSelectMenu() error {
	store, err := botstore.New(projectDir)
	if err != nil {
		return fmt.Errorf("botstore: %w", err)
	}

	bots, err := store.List()
	if err != nil {
		return fmt.Errorf("botstore list: %w", err)
	}

	if len(bots) == 0 {
		fmt.Fprintln(os.Stderr, "등록된 봇이 없습니다.")
		fmt.Fprintln(os.Stderr, "  cq setup 으로 시작하세요.")
		return nil
	}

	printBotMenu(bots)

	newBotIdx := len(bots) + 1
	quitIdx := len(bots) + 2

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Fprintf(os.Stderr, "선택 [1-%d]: ", quitIdx)
		if !scanner.Scan() {
			// EOF / non-interactive — fall back to default launch
			return initAndLaunch("claude")
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
			selected := bots[choice-1]
			if err := os.Setenv("C4_TELEGRAM_BOT_TOKEN", selected.Token); err != nil {
				return fmt.Errorf("set env: %w", err)
			}
			fmt.Fprintf(os.Stderr, "봇 선택: @%s\n", selected.Username)
			return initAndLaunch("claude")

		case choice == newBotIdx:
			fmt.Fprintln(os.Stderr, "새 봇 만들기: cq setup 을 실행하세요.")
			return nil

		case choice == quitIdx:
			return nil

		default:
			fmt.Fprintf(os.Stderr, "잘못된 입력입니다. 1-%d 사이의 숫자를 입력하세요.\n", quitIdx)
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
	fmt.Fprintf(os.Stderr, "  %d) 종료\n", len(bots)+2)
	fmt.Fprintln(os.Stderr, "")
}
