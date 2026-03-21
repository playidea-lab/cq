package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/botstore"
)

// verifyTokenFunc is injectable for tests.
var verifyTokenFunc = func(token string) (botstore.BotInfo, error) {
	return botstore.VerifyToken(token)
}

// runSetupWizardInline runs the bot setup wizard inline (no separate command).
// Returns the created bot on success.
func runSetupWizardInline(store *botstore.Store) (botstore.Bot, error) {
	scanner := bufio.NewScanner(os.Stdin)
	w := os.Stderr

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w, "  새 Telegram 봇 만들기")
	fmt.Fprintln(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  1. Telegram에서 @BotFather 를 검색해 대화를 시작하세요.")
	fmt.Fprintln(w, "  2. /newbot → 봇 이름 → 사용자명 (bot으로 끝남)")
	fmt.Fprintln(w, "  3. BotFather가 토큰을 발급합니다.")
	fmt.Fprintln(w, "")
	fmt.Fprint(w, "준비되면 Enter... ")
	if !scanner.Scan() {
		return botstore.Bot{}, fmt.Errorf("입력이 종료되었습니다")
	}

	fmt.Fprintln(w, "")
	fmt.Fprint(w, "봇 토큰: ")
	if !scanner.Scan() {
		return botstore.Bot{}, fmt.Errorf("입력이 종료되었습니다")
	}
	token := strings.TrimSpace(scanner.Text())
	if token == "" {
		return botstore.Bot{}, fmt.Errorf("토큰이 비어있습니다")
	}

	fmt.Fprintln(w, "  검증 중...")
	info, err := verifyTokenFunc(token)
	if err != nil {
		return botstore.Bot{}, fmt.Errorf("토큰 검증 실패: %w", err)
	}
	fmt.Fprintf(w, "  ✓ @%s (%s)\n", info.Username, info.FirstName)

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  내 Telegram ID를 입력하세요 (숫자).")
	fmt.Fprintln(w, "  모르면 @userinfobot 에게 /start 전송.")
	fmt.Fprintln(w, "")
	fmt.Fprint(w, "Telegram ID: ")
	if !scanner.Scan() {
		return botstore.Bot{}, fmt.Errorf("입력이 종료되었습니다")
	}
	chatIDStr := strings.TrimSpace(scanner.Text())
	if chatIDStr == "" {
		return botstore.Bot{}, fmt.Errorf("ID가 비어있습니다")
	}

	var chatID int64
	if _, err := fmt.Sscanf(chatIDStr, "%d", &chatID); err != nil {
		return botstore.Bot{}, fmt.Errorf("ID 파싱 실패: %q", chatIDStr)
	}

	bot := botstore.Bot{
		Username:    info.Username,
		Token:       token,
		DisplayName: info.FirstName,
		LastActive:  time.Now(),
		Scope:       "global",
		AllowFrom:   []int64{chatID},
	}
	if err := store.Save(bot); err != nil {
		return botstore.Bot{}, fmt.Errorf("저장 실패: %w", err)
	}

	fmt.Fprintf(w, "  ✓ @%s 등록 완료!\n", info.Username)
	fmt.Fprintln(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w, "")

	return bot, nil
}
