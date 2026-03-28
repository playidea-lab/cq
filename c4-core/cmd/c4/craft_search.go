package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var craftSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "레지스트리에서 스킬 검색",
	Long: `CQ 스킬 레지스트리에서 이름/설명으로 검색합니다.

사용 예시:
  cq search review          "review" 키워드로 검색
  cq search "python pro"    여러 단어로 검색`,
	Args: cobra.ExactArgs(1),
	RunE: runCraftSearch,
}

func init() {
	rootCmd.AddCommand(craftSearchCmd)
}

func runCraftSearch(cmd *cobra.Command, args []string) error {
	query := args[0]

	client, err := newRegistryClient()
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "레지스트리 연결 실패: %v\n빌트인 목록: cq add --list\n", err)
		return nil
	}

	skills, err := client.SearchFTS(query)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "검색 실패: %v\n", err)
		return nil
	}

	if len(skills) == 0 {
		fmt.Println("검색 결과 없음. cq add --list로 빌트인 프리셋을 확인하세요.")
		return nil
	}

	// Header
	fmt.Printf("%-24s %-8s %-40s %-10s %s\n", "NAME", "TYPE", "DESCRIPTION", "VERSION", "DOWNLOADS")
	fmt.Println(strings.Repeat("─", 100))

	for _, s := range skills {
		desc := s.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		fmt.Printf("%-24s %-8s %-40s %-10s %d\n",
			s.Name, s.Type, desc, s.LatestVersion, s.DownloadCount)
	}

	fmt.Printf("\n%d개 결과. cq add <name>으로 설치.\n", len(skills))
	return nil
}
