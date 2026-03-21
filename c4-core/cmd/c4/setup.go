package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/botstore"
	"github.com/spf13/cobra"
)

// verifyTokenFunc is injectable for tests.
var verifyTokenFunc = func(token string) (botstore.BotInfo, error) {
	return botstore.VerifyToken(token)
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Set up a Telegram bot for CQ (step-by-step wizard)",
	Long: `Interactive wizard to configure a Telegram bot for CQ.

Steps:
  1. Create a bot via BotFather
  2. Enter your bot token
  3. Pair your Telegram account (allowFrom)
  4. Start the CQ session

Run: cq setup`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSetupWizard(os.Stdin, os.Stdout)
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
}

// runSetupWizard runs the interactive bot setup. r/w are injectable for tests.
func runSetupWizard(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)

	// ── Step 1: BotFather 안내 ────────────────────────────────────────────
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w, "  CQ Telegram Bot Setup Wizard")
	fmt.Fprintln(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Step 1/3  봇 생성 (BotFather)")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  1. Telegram에서 @BotFather 를 검색해 대화를 시작하세요.")
	fmt.Fprintln(w, "  2. /newbot 명령어를 전송하세요.")
	fmt.Fprintln(w, "  3. 봇 이름을 입력하세요 (예: My CQ Bot).")
	fmt.Fprintln(w, "  4. 봇 사용자명을 입력하세요 (예: mycqbot) — 반드시 'bot'으로 끝나야 합니다.")
	fmt.Fprintln(w, "  5. BotFather가 토큰을 발급합니다. 예: 1234567890:ABCdef...")
	fmt.Fprintln(w, "")
	fmt.Fprint(w, "준비되면 Enter를 누르세요... ")
	if !scanner.Scan() {
		return fmt.Errorf("입력이 종료되었습니다")
	}

	// ── Step 2: 토큰 입력 + 검증 ────────────────────────────────────────
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Step 2/3  토큰 입력")
	fmt.Fprintln(w, "")
	fmt.Fprint(w, "봇 토큰을 입력하세요: ")
	if !scanner.Scan() {
		return fmt.Errorf("입력이 종료되었습니다")
	}
	token := strings.TrimSpace(scanner.Text())
	if token == "" {
		return fmt.Errorf("토큰이 비어있습니다")
	}

	fmt.Fprintln(w, "  토큰 검증 중...")
	info, err := verifyTokenFunc(token)
	if err != nil {
		return fmt.Errorf("토큰 검증 실패: %w\n  BotFather에서 발급한 토큰을 다시 확인해 주세요.", err)
	}

	fmt.Fprintf(w, "  ✓ 봇 확인: @%s (%s)\n", info.Username, info.FirstName)

	// config.json 저장
	store, err := botstore.New(projectDir)
	if err != nil {
		return fmt.Errorf("botstore 초기화 실패: %w", err)
	}

	bot := botstore.Bot{
		Username:    info.Username,
		Token:       token,
		DisplayName: info.FirstName,
		LastActive:  time.Now(),
		Scope:       "global",
	}
	if err := store.Save(bot); err != nil {
		return fmt.Errorf("config.json 저장 실패: %w", err)
	}
	fmt.Fprintf(w, "  ✓ 저장 완료: ~/.claude/bots/%s/config.json\n", info.Username)

	// ── Step 3: 페어링 ───────────────────────────────────────────────────
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Step 3/3  내 계정 페어링 (allowFrom)")
	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "  1. Telegram에서 @%s 봇을 열고 /start 를 전송하세요.\n", info.Username)
	fmt.Fprintln(w, "  2. 봇이 채팅 ID를 포함한 메시지를 전송합니다.")
	fmt.Fprintln(w, "     (봇 서버가 아직 실행 중이 아니라면 'cq serve'로 먼저 실행하세요.)")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  또는 아래 방법으로 내 채팅 ID를 확인할 수 있습니다:")
	fmt.Fprintln(w, "    https://t.me/userinfobot — 봇에 /start 전송 → ID 확인")
	fmt.Fprintln(w, "")
	fmt.Fprint(w, "내 Telegram 채팅 ID를 입력하세요 (숫자, 예: 123456789): ")
	if !scanner.Scan() {
		return fmt.Errorf("입력이 종료되었습니다")
	}
	chatIDStr := strings.TrimSpace(scanner.Text())
	if chatIDStr == "" {
		return fmt.Errorf("채팅 ID가 비어있습니다")
	}

	var chatID int64
	if _, err := fmt.Sscanf(chatIDStr, "%d", &chatID); err != nil {
		return fmt.Errorf("채팅 ID 파싱 실패: %q는 유효한 정수가 아닙니다", chatIDStr)
	}

	// allowFrom 업데이트 후 재저장
	bot.AllowFrom = []int64{chatID}
	if err := store.Save(bot); err != nil {
		return fmt.Errorf("access.json 저장 실패: %w", err)
	}
	fmt.Fprintf(w, "  ✓ 페어링 완료: 채팅 ID %d 허용됨\n", chatID)

	// ── 완료 ─────────────────────────────────────────────────────────────
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintf(w, "  설정 완료! @%s 봇이 준비되었습니다.\n", info.Username)
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  다음 단계:")
	fmt.Fprintln(w, "    cq serve   — CQ 서버 시작 (봇 수신 대기)")
	fmt.Fprintf(w, "    Telegram에서 @%s 에게 메시지를 보내 테스트하세요.\n", info.Username)
	fmt.Fprintln(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(w, "")

	return nil
}
