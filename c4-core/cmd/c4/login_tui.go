package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
)

// loginPhase represents the current step of the login TUI.
type loginPhase string

const (
	loginPhaseMenu     loginPhase = "menu"
	loginPhaseWaiting  loginPhase = "waiting"
	loginPhaseOTPEmail loginPhase = "otp_email"
	loginPhaseOTPCode  loginPhase = "otp_code"
	loginPhaseSuccess  loginPhase = "success"
	loginPhaseError    loginPhase = "error"
)

// loginOption is a single menu item.
type loginOption struct {
	label  string
	desc   string
	method string // "github", "otp", "skip"
}

// loginResultMsg is the async result of a login attempt.
type loginResultMsg struct {
	err      error
	userInfo string // e.g. "changmin (changmin@example.com)"
}

// loginOTPSentMsg signals that the OTP email was dispatched successfully.
type loginOTPSentMsg struct{}

// loginTUIModel is the bubbletea model for the login screen.
type loginTUIModel struct {
	width, height int

	phase   loginPhase
	cursor  int
	options []loginOption

	// OTP flow
	emailInput string
	codeInput  string
	otpEmail   string // confirmed email after OTP send

	// Success / error
	errorMsg string
	userInfo string

	// Navigation result — non-empty signals the loop to switch screens.
	nextScreen string
}

func newLoginTUIModel() loginTUIModel {
	return loginTUIModel{
		phase: loginPhaseMenu,
		options: []loginOption{
			{"GitHub OAuth", "브라우저에서 인증", "github"},
			{"Email OTP", "이메일 코드 인증", "otp"},
			{"건너뛰기", "오프라인 모드 (레지스트리 비활성)", "skip"},
		},
	}
}

// --- tea.Model ---

func (m loginTUIModel) Init() tea.Cmd {
	return nil
}

func (m loginTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case loginOTPSentMsg:
		// OTP email sent — advance to code-entry phase.
		m.phase = loginPhaseOTPCode
		m.codeInput = ""
		return m, nil

	case loginResultMsg:
		if msg.err != nil {
			m.phase = loginPhaseError
			m.errorMsg = msg.err.Error()
			return m, nil
		}
		// Success — set a brief success display, then immediately quit to sessions.
		m.phase = loginPhaseSuccess
		m.userInfo = msg.userInfo
		// Delay one render tick so the user sees the success message.
		return m, tea.Tick(600*time.Millisecond, func(time.Time) tea.Msg {
			return loginNavigateMsg{}
		})

	case loginNavigateMsg:
		m.nextScreen = screenSessions
		return m, tea.Quit

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// loginNavigateMsg triggers navigation after a brief success display.
type loginNavigateMsg struct{}

func (m loginTUIModel) handleKey(msg tea.KeyMsg) (loginTUIModel, tea.Cmd) {
	switch m.phase {

	case loginPhaseMenu:
		switch msg.String() {
		case "ctrl+c", "q":
			m.nextScreen = screenQuit
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case "enter", " ":
			return m.selectOption()
		}

	case loginPhaseWaiting:
		if msg.String() == "ctrl+c" {
			m.nextScreen = screenQuit
			return m, tea.Quit
		}

	case loginPhaseOTPEmail:
		switch msg.String() {
		case "ctrl+c":
			m.nextScreen = screenQuit
			return m, tea.Quit
		case "esc":
			m.phase = loginPhaseMenu
			m.emailInput = ""
		case "enter":
			email := strings.TrimSpace(m.emailInput)
			if email != "" {
				m.otpEmail = email
				m.phase = loginPhaseWaiting
				return m, doOTPSend(email)
			}
		case "backspace", "ctrl+h":
			if len(m.emailInput) > 0 {
				m.emailInput = m.emailInput[:len(m.emailInput)-1]
			}
		default:
			if msg.Type == tea.KeyRunes {
				m.emailInput += string(msg.Runes)
			}
		}

	case loginPhaseOTPCode:
		switch msg.String() {
		case "ctrl+c":
			m.nextScreen = screenQuit
			return m, tea.Quit
		case "esc":
			m.phase = loginPhaseMenu
			m.codeInput = ""
			m.emailInput = ""
		case "enter":
			code := strings.TrimSpace(m.codeInput)
			if code != "" {
				m.phase = loginPhaseWaiting
				return m, doOTPVerify(m.otpEmail, code)
			}
		case "backspace", "ctrl+h":
			if len(m.codeInput) > 0 {
				m.codeInput = m.codeInput[:len(m.codeInput)-1]
			}
		default:
			if msg.Type == tea.KeyRunes && len(m.codeInput) < 6 {
				m.codeInput += string(msg.Runes)
			}
		}

	case loginPhaseError:
		switch msg.String() {
		case "ctrl+c", "q":
			m.nextScreen = screenQuit
			return m, tea.Quit
		case "enter", "esc", " ":
			m.phase = loginPhaseMenu
			m.errorMsg = ""
			m.emailInput = ""
			m.codeInput = ""
		}

	case loginPhaseSuccess:
		// Handled by timer; allow manual advance.
		if msg.String() == "enter" || msg.String() == " " {
			m.nextScreen = screenSessions
			return m, tea.Quit
		}
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			m.nextScreen = screenQuit
			return m, tea.Quit
		}
	}
	return m, nil
}

// selectOption processes the selected menu item.
func (m loginTUIModel) selectOption() (loginTUIModel, tea.Cmd) {
	switch m.options[m.cursor].method {
	case "github":
		m.phase = loginPhaseWaiting
		return m, doGitHubLogin()
	case "otp":
		m.phase = loginPhaseOTPEmail
	case "skip":
		m.nextScreen = screenSessions
		return m, tea.Quit
	}
	return m, nil
}

// --- async login commands ---

func doGitHubLogin() tea.Cmd {
	return func() tea.Msg {
		client, err := newAuthClient()
		if err != nil {
			return loginResultMsg{err: err}
		}
		if err := client.LoginWithGitHub(); err != nil {
			return loginResultMsg{err: err}
		}
		patchCloudConfigAfterLogin(projectDir)
		autoInstallServe()

		sess, getErr := client.GetSession()
		if getErr == nil && sess != nil {
			return loginResultMsg{
				userInfo: fmt.Sprintf("%s (%s)", sess.User.Name, sess.User.Email),
			}
		}
		return loginResultMsg{}
	}
}

func doOTPSend(email string) tea.Cmd {
	return func() tea.Msg {
		client, err := newAuthClient()
		if err != nil {
			return loginResultMsg{err: err}
		}
		if err := client.SendOTP(email); err != nil {
			return loginResultMsg{err: fmt.Errorf("OTP 전송 실패: %w", err)}
		}
		return loginOTPSentMsg{}
	}
}

func doOTPVerify(email, code string) tea.Cmd {
	return func() tea.Msg {
		client, err := newAuthClient()
		if err != nil {
			return loginResultMsg{err: err}
		}
		sess, err := client.VerifyOTP(email, code)
		if err != nil {
			return loginResultMsg{err: fmt.Errorf("인증 실패: %w", err)}
		}
		patchCloudConfigAfterLogin(projectDir)
		return loginResultMsg{
			userInfo: fmt.Sprintf("%s (%s)", sess.User.Name, sess.User.Email),
		}
	}
}

// --- styles ---

var (
	loginBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 3)

	loginLogoStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("62")).
		Bold(true)

	loginTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15"))

	loginCursorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("14")).
		Bold(true)

	loginLabelStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Bold(true)

	loginDescStyle = lipgloss.NewStyle().
		Faint(true)

	loginInputStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("14"))

	loginErrorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("1")).
		Bold(true)

	loginSuccessStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("2")).
		Bold(true)
)

// --- View ---

func (m loginTUIModel) View() string {
	var inner strings.Builder

	// Logo + title
	inner.WriteString(loginLogoStyle.Render("  "+cqLogo[0]) + "\n")
	inner.WriteString(loginLogoStyle.Render("  "+cqLogo[1]) + "\n")
	inner.WriteString("\n")
	inner.WriteString(loginTitleStyle.Render("CQ — AI Orchestration") + "\n")
	inner.WriteString("\n")

	switch m.phase {
	case loginPhaseMenu:
		inner.WriteString(styleFaint.Render("로그인 방법을 선택하세요:") + "\n")
		inner.WriteString("\n")
		for i, opt := range m.options {
			label := lsPadToWidth(opt.label, 18)
			desc := loginDescStyle.Render(opt.desc)
			if i == m.cursor {
				inner.WriteString(loginCursorStyle.Render("▸ "))
				inner.WriteString(loginLabelStyle.Render(label))
				inner.WriteString(" " + desc + "\n")
			} else {
				inner.WriteString("  " + label + " " + desc + "\n")
			}
		}
		inner.WriteString("\n")
		inner.WriteString(styleFaint.Render("↑↓ 이동  Enter 선택  q 종료"))

	case loginPhaseWaiting:
		inner.WriteString(styleFaint.Render("처리 중...") + "\n")
		inner.WriteString("\n")
		inner.WriteString(loginDescStyle.Render("브라우저 인증 또는 서버 응답을 기다리는 중입니다."))

	case loginPhaseOTPEmail:
		inner.WriteString(styleFaint.Render("이메일 주소를 입력하세요:") + "\n")
		inner.WriteString("\n")
		inner.WriteString(loginInputStyle.Render("> "+m.emailInput+"_") + "\n")
		inner.WriteString("\n")
		inner.WriteString(styleFaint.Render("Enter 전송  Esc 뒤로"))

	case loginPhaseOTPCode:
		inner.WriteString(styleFaint.Render("이메일로 전송된 6자리 코드:") + "\n")
		inner.WriteString(loginDescStyle.Render("  "+m.otpEmail) + "\n")
		inner.WriteString("\n")
		inner.WriteString(loginInputStyle.Render("> "+m.codeInput+"_") + "\n")
		inner.WriteString("\n")
		inner.WriteString(styleFaint.Render("Enter 인증  Esc 뒤로"))

	case loginPhaseSuccess:
		inner.WriteString(loginSuccessStyle.Render("로그인 성공!") + "\n")
		if m.userInfo != "" {
			inner.WriteString("\n")
			inner.WriteString(loginLabelStyle.Render("  "+m.userInfo) + "\n")
		}
		inner.WriteString("\n")
		inner.WriteString(styleFaint.Render("  Sessions으로 이동 중..."))

	case loginPhaseError:
		inner.WriteString(loginErrorStyle.Render("오류 발생") + "\n")
		inner.WriteString("\n")
		inner.WriteString(loginDescStyle.Render("  "+m.errorMsg) + "\n")
		inner.WriteString("\n")
		inner.WriteString(styleFaint.Render("Enter / Esc 로 메뉴로 돌아가기"))
	}

	box := loginBoxStyle.Render(inner.String())

	// Center box in terminal.
	if m.width > 0 && m.height > 0 {
		boxW := lipgloss.Width(box)
		boxH := lipgloss.Height(box)
		padLeft := 0
		if m.width > boxW {
			padLeft = (m.width - boxW) / 2
		}
		padTop := 0
		if m.height > boxH+2 {
			padTop = (m.height - boxH) / 2
		}
		var sb strings.Builder
		for i := 0; i < padTop; i++ {
			sb.WriteString("\n")
		}
		for _, line := range strings.Split(box, "\n") {
			sb.WriteString(strings.Repeat(" ", padLeft) + line + "\n")
		}
		return sb.String()
	}
	return box
}

// runLoginNav launches the login TUI and returns the next screen name.
func runLoginNav() string {
	m := newLoginTUIModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return screenQuit
	}
	if final, ok := result.(loginTUIModel); ok && final.nextScreen != "" {
		return final.nextScreen
	}
	return screenSessions
}
