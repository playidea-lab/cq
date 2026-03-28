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
	Use:   "add [preset]",
	Short: "프리셋 스킬/에이전트/룰 설치",
	Long:  "프리셋 카탈로그에서 커스텀 도구를 설치합니다.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runCraftAdd,
}

// craftListCmd implements `cq list --mine`.
// Registered as a sub-alias; --mine flag activates installed-preset listing.
var craftListCmd = &cobra.Command{
	Use:   "list",
	Short: "설치된 커스텀 도구 목록",
	Long:  "설치된 커스텀 도구(스킬/에이전트/룰) 목록을 출력합니다.",
	RunE:  runCraftList,
}

// craftRemoveCmd implements `cq remove <name>`.
var craftRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "설치된 커스텀 도구 제거",
	Long:  "설치된 프리셋(스킬/에이전트/룰)을 삭제합니다.",
	Args:  cobra.ExactArgs(1),
	RunE:  runCraftRemove,
}

func init() {
	craftAddCmd.Flags().BoolVar(&craftListFlag, "list", false, "프리셋 목록만 출력")
	craftListCmd.Flags().BoolVar(&craftMineFlag, "mine", false, "설치된 커스텀 도구만 출력")
	craftRemoveCmd.Flags().BoolVar(&craftForceFlag, "force", false, "확인 없이 삭제")

	rootCmd.AddCommand(craftAddCmd)
	rootCmd.AddCommand(craftListCmd)
	rootCmd.AddCommand(craftRemoveCmd)
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

	// `cq add <name>` — install
	name := args[0]
	preset, err := craft.Find(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "프리셋 '%s'을 찾을 수 없습니다. cq add로 목록을 확인하세요.\n", name)
		return nil
	}

	dest, err := craft.Install(preset, homeDir)
	if err != nil {
		return fmt.Errorf("설치 실패: %w", err)
	}

	fmt.Printf("✓ %s 설치 → %s\n", name, dest)
	fmt.Printf("  %s\n", craftUsageHint(preset.Type, name))
	return nil
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
			fmt.Printf("  %s\n", it.Name)
		}
	}
	if len(agents) > 0 {
		fmt.Println("Agents:")
		for _, it := range agents {
			fmt.Printf("  %s\n", it.Name)
		}
	}
	if len(rules) > 0 {
		fmt.Println("Rules:")
		for _, it := range rules {
			fmt.Printf("  %s\n", it.Name)
		}
	}

	// Check for CLAUDE.md in current directory
	if _, err := os.Stat("CLAUDE.md"); err == nil {
		fmt.Println("CLAUDE.md:")
		fmt.Println("  ./CLAUDE.md")
	}
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
