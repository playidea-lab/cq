package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/changmin/c4-core/internal/craft"
	"github.com/spf13/cobra"
)

var (
	craftListFlag  bool   // --list for cq add
	craftForceFlag bool   // --force for cq remove
	craftMineFlag  bool   // --mine for cq list
)

// craftAddCmd implements `cq add [preset]`.
var craftAddCmd = &cobra.Command{
	Use:   "add [preset | github-url]",
	Short: "스킬/에이전트/룰 설치 (내장 프리셋 또는 GitHub)",
	Long: `스킬, 에이전트, 룰을 설치합니다.

인자 없이 실행하면 내장 프리셋 53개를 TUI로 브라우징합니다.

사용 예시:
  cq add                                    TUI 카탈로그 열기
  cq add code-review                        내장 프리셋 설치
  cq add anthropics/skills:pdf              GitHub shorthand로 설치
  cq add obra/superpowers:brainstorming     커뮤니티 스킬 설치
  cq add https://github.com/.../skills/pdf  풀 URL로 설치
  cq add --list                             프리셋 목록 텍스트 출력

설치된 도구는 ~/.claude/{skills|agents|rules}/에 저장되며 즉시 사용 가능합니다.
대화형으로 새 도구를 만들려면 /craft 스킬을 사용하세요.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCraftAdd,
}

// craftListCmd implements `cq list --mine`.
// Registered as a sub-alias; --mine flag activates installed-preset listing.
var craftListCmd = &cobra.Command{
	Use:   "list",
	Short: "설치된 커스텀 도구 목록",
	Long: `설치된 커스텀 도구(스킬/에이전트/룰) 목록을 출력합니다.

사용 예시:
  cq list --mine    설치된 도구 목록 (내장/원격 구분 표시)`,
	RunE: runCraftList,
}

// craftRemoveCmd implements `cq remove <name>`.
var craftRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "설치된 커스텀 도구 제거",
	Long: `설치된 스킬/에이전트/룰을 삭제합니다.

사용 예시:
  cq remove pdf             PDF 스킬 삭제
  cq remove --force pdf     확인 없이 삭제`,
	Args: cobra.ExactArgs(1),
	RunE: runCraftRemove,
}

// craftUpdateCmd implements `cq update <name>`.
var craftUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "원격 설치 스킬 업데이트",
	Long: `cq add <url>로 설치한 스킬을 원본 GitHub에서 최신 버전으로 업데이트합니다.
파일에 기록된 # source: URL을 사용하여 원본을 다시 가져옵니다.

사용 예시:
  cq update pdf             PDF 스킬을 최신으로 업데이트`,
	Args: cobra.ExactArgs(1),
	RunE: runCraftUpdate,
}

func init() {
	craftAddCmd.Flags().BoolVar(&craftListFlag, "list", false, "프리셋 목록만 출력")
	craftListCmd.Flags().BoolVar(&craftMineFlag, "mine", false, "설치된 커스텀 도구만 출력")
	craftRemoveCmd.Flags().BoolVar(&craftForceFlag, "force", false, "확인 없이 삭제")

	rootCmd.AddCommand(craftAddCmd)
	rootCmd.AddCommand(craftListCmd)
	rootCmd.AddCommand(craftRemoveCmd)
	rootCmd.AddCommand(craftUpdateCmd)
}

// runCraftAdd handles `cq add` and `cq add <name>` and `cq add --list`.
func runCraftAdd(cmd *cobra.Command, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("홈 디렉토리 확인 실패: %w", err)
	}

	// `cq add --list` — simple text list
	if craftListFlag {
		return printPresetCatalog()
	}

	// `cq add` — interactive TUI catalog
	if len(args) == 0 {
		return runCraftTUI(homeDir)
	}

	// `cq add <url-or-name>` — install
	arg := args[0]

	// Route to remote installer if arg looks like a GitHub URL or shorthand.
	if craft.IsRemoteSource(arg) {
		src, err := craft.ParseGitHubURL(arg)
		if err != nil {
			return fmt.Errorf("URL 파싱 실패: %w", err)
		}
		dest, err := craft.FetchAndInstall(src, homeDir)
		if err != nil {
			return fmt.Errorf("원격 설치 실패: %w", err)
		}
		name := dest // use dest path as display; skillName extracted inside
		fmt.Printf("✓ 설치 (remote) → %s\n  source: %s\n", name, src.URL)
		return nil
	}

	// Local preset from embedded catalog.
	preset, err := craft.Find(arg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "프리셋 '%s'을 찾을 수 없습니다. cq add로 목록을 확인하세요.\n", arg)
		return nil
	}

	dest, err := craft.Install(preset, homeDir)
	if err != nil {
		return fmt.Errorf("설치 실패: %w", err)
	}

	fmt.Printf("✓ %s 설치 → %s\n", arg, dest)
	fmt.Printf("  %s\n", craftUsageHint(preset.Type, arg))
	return nil
}

// runCraftUpdate handles `cq update <name>`.
func runCraftUpdate(cmd *cobra.Command, args []string) error {
	name := args[0]

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("홈 디렉토리 확인 실패: %w", err)
	}

	return craft.Update(name, homeDir)
}

// runCraftList handles `cq list --mine`.
// When --mine is absent the command exits with a usage hint.
func runCraftList(cmd *cobra.Command, args []string) error {
	if !craftMineFlag {
		fmt.Fprintln(os.Stderr, "사용법: cq list --mine  (설치된 커스텀 도구 목록)")
		return nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("홈 디렉토리 확인 실패: %w", err)
	}

	items, err := craft.ListInstalled(homeDir)
	if err != nil {
		return fmt.Errorf("설치 목록 조회 실패: %w", err)
	}

	if len(items) == 0 {
		fmt.Println("커스텀 도구가 없습니다. cq add로 시작하세요.")
		return nil
	}

	printInstalledByCategory(items)
	return nil
}

// runCraftRemove handles `cq remove <name>`.
func runCraftRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("홈 디렉토리 확인 실패: %w", err)
	}

	if !craftForceFlag {
		fmt.Printf("'%s'을 삭제하시겠습니까? [y/N] ", name)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("취소됨.")
			return nil
		}
	}

	if err := craft.Remove(name, homeDir); err != nil {
		return fmt.Errorf("제거 실패: %w", err)
	}

	fmt.Printf("✓ %s 제거됨\n", name)
	return nil
}

// printPresetCatalog lists all available presets grouped by category.
func printPresetCatalog() error {
	presets, err := craft.List()
	if err != nil {
		return fmt.Errorf("카탈로그 로드 실패: %w", err)
	}

	var skills, agents, rules, claudeMds []craft.Preset
	for _, p := range presets {
		switch p.Type {
		case craft.TypeSkill:
			skills = append(skills, p)
		case craft.TypeAgent:
			agents = append(agents, p)
		case craft.TypeRule:
			rules = append(rules, p)
		case craft.TypeClaudeMd:
			claudeMds = append(claudeMds, p)
		}
	}

	if len(skills) > 0 {
		fmt.Println("Skills:")
		for _, p := range skills {
			printPresetLine(p)
		}
	}
	if len(agents) > 0 {
		fmt.Println("Agents:")
		for _, p := range agents {
			printPresetLine(p)
		}
	}
	if len(rules) > 0 {
		fmt.Println("Rules:")
		for _, p := range rules {
			printPresetLine(p)
		}
	}
	if len(claudeMds) > 0 {
		fmt.Println("CLAUDE.md:")
		for _, p := range claudeMds {
			printPresetLine(p)
		}
	}

	return nil
}

// printPresetLine prints a single preset entry with tab alignment.
func printPresetLine(p craft.Preset) {
	if p.Description != "" {
		fmt.Printf("  %-24s — %s\n", p.Name, p.Description)
	} else {
		fmt.Printf("  %s\n", p.Name)
	}
}

// printInstalledByCategory prints installed items grouped by type.
func printInstalledByCategory(items []craft.InstalledItem) {
	var skills, agents, rules []craft.InstalledItem
	for _, it := range items {
		switch it.Type {
		case craft.TypeSkill:
			skills = append(skills, it)
		case craft.TypeAgent:
			agents = append(agents, it)
		case craft.TypeRule:
			rules = append(rules, it)
		}
	}

	if len(skills) > 0 {
		fmt.Println("Skills:")
		for _, it := range skills {
			fmt.Printf("  %s\n", installedItemLabel(it))
		}
	}
	if len(agents) > 0 {
		fmt.Println("Agents:")
		for _, it := range agents {
			fmt.Printf("  %s\n", installedItemLabel(it))
		}
	}
	if len(rules) > 0 {
		fmt.Println("Rules:")
		for _, it := range rules {
			fmt.Printf("  %s\n", installedItemLabel(it))
		}
	}

	// Check for CLAUDE.md in current directory
	if _, err := os.Stat("CLAUDE.md"); err == nil {
		fmt.Println("CLAUDE.md:")
		fmt.Println("  ./CLAUDE.md")
	}
}

// installedItemLabel formats a name with a "(remote)" badge when the item
// was installed from a remote URL.
func installedItemLabel(it craft.InstalledItem) string {
	if it.Source != "" {
		return it.Name + " (remote)"
	}
	return it.Name
}

// runCraftTUI launches the interactive preset picker TUI and installs the
// selected preset.  homeDir is used as the installation root.
func runCraftTUI(homeDir string) error {
	presets, err := craft.List()
	if err != nil {
		return fmt.Errorf("카탈로그 로드 실패: %w", err)
	}

	m := newCraftTUIModel(presets)
	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI 실패: %w", err)
	}

	final, ok := result.(craftTUIModel)
	if !ok || final.selected == nil {
		// User quit without selecting.
		return nil
	}

	preset := final.selected
	dest, err := craft.Install(preset, homeDir)
	if err != nil {
		return fmt.Errorf("설치 실패: %w", err)
	}

	fmt.Printf("✓ %s 설치 → %s\n", preset.Name, dest)
	fmt.Printf("  %s\n", craftUsageHint(preset.Type, preset.Name))
	return nil
}

// craftUsageHint returns a short usage hint based on preset type.
func craftUsageHint(t craft.PresetType, name string) string {
	switch t {
	case craft.TypeSkill:
		return fmt.Sprintf("/%s으로 호출", name)
	case craft.TypeAgent:
		return "자동 적용"
	case craft.TypeRule:
		return "항상 적용"
	case craft.TypeClaudeMd:
		return "프로젝트 루트에 CLAUDE.md 생성됨"
	default:
		return ""
	}
}
