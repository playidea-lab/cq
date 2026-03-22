package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/changmin/c4-core/internal/standards"
	"github.com/spf13/cobra"
)

var standardsForce bool

var standardsCmd = &cobra.Command{
	Use:   "standards",
	Short: "Manage project standards",
	Long: `standards shows the current state of applied project standards.

Reads .piki-lock.yaml and displays team, languages, applied_at, and managed files.
Use subcommands to apply, check, or list available standards.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
	RunE:              runStandards,
}

var standardsApplyCmd = &cobra.Command{
	Use:   "apply [team.lang,lang...]",
	Short: "Apply standards to the current project",
	Long: `apply copies standard files into the project directory.

Argument formats:
  (none)             apply common layer only
  backend            apply common + team backend with default langs
  backend.go         apply common + team backend + go
  backend.go,python  apply common + team backend + go,python`,
	Args: cobra.MaximumNArgs(1),
	RunE: runStandardsApply,
}

var standardsCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check whether project files match embedded standards",
	RunE:  runStandardsCheck,
}

var standardsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available teams and languages in the standards manifest",
	RunE:  runStandardsList,
}

var standardsSkillsTeam string
var standardsSkillsInstalled bool

func init() {
	standardsApplyCmd.Flags().BoolVar(&standardsForce, "force", false, "overwrite modified files")
	standardsSkillsCmd.Flags().StringVar(&standardsSkillsTeam, "team", "", "filter by team")
	standardsSkillsCmd.Flags().BoolVar(&standardsSkillsInstalled, "installed", false, "show only installed skills")
	standardsCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error { return nil }
	standardsCmd.AddCommand(standardsApplyCmd)
	standardsCmd.AddCommand(standardsCheckCmd)
	standardsCmd.AddCommand(standardsListCmd)
	standardsCmd.AddCommand(standardsSkillsCmd)
	standardsCmd.AddCommand(standardsInstallCmd)
	rootCmd.AddCommand(standardsCmd)
}

func runStandards(cmd *cobra.Command, args []string) error {
	dir := projectDir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	lock, err := standards.ReadLock(dir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("표준 미적용. cq standards apply <team.lang> 실행")
			return nil
		}
		fmt.Println("표준 미적용. cq standards apply <team.lang> 실행")
		return nil
	}

	fmt.Printf("Team:       %s\n", lock.Team)
	fmt.Printf("Langs:      %s\n", strings.Join(lock.Langs, ", "))
	fmt.Printf("Applied at: %s\n", lock.AppliedAt)
	fmt.Printf("Files (%d):\n", len(lock.Files))
	for _, f := range lock.Files {
		fmt.Printf("  %s\n", f.Dst)
	}
	return nil
}

func runStandardsApply(cmd *cobra.Command, args []string) error {
	dir := projectDir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	team, langs := parseTeamLangs(args)

	result, err := standards.Apply(dir, team, langs, standards.ApplyOptions{Force: standardsForce})
	if err != nil {
		return err
	}

	fmt.Printf("Team:    %s\n", result.Team)
	fmt.Printf("Langs:   %s\n", strings.Join(result.Langs, ", "))
	fmt.Printf("Created: %d file(s)\n", len(result.FilesCreated))
	for _, f := range result.FilesCreated {
		fmt.Printf("  + %s\n", f)
	}
	if len(result.FilesSkipped) > 0 {
		fmt.Printf("Skipped: %d file(s) (already exist, use --force to overwrite)\n", len(result.FilesSkipped))
		for _, f := range result.FilesSkipped {
			fmt.Printf("  ~ %s\n", f)
		}
	}
	if len(result.FilesRemoved) > 0 {
		fmt.Printf("Removed: %d file(s)\n", len(result.FilesRemoved))
		for _, f := range result.FilesRemoved {
			fmt.Printf("  - %s\n", f)
		}
	}
	return nil
}

func runStandardsCheck(cmd *cobra.Command, args []string) error {
	dir := projectDir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	results, err := standards.Check(dir)
	if err != nil {
		return err
	}

	if len(results) == 0 {
		fmt.Println("모든 파일이 최신 상태입니다.")
		return nil
	}

	for _, r := range results {
		switch r.Status {
		case standards.DiffMatch:
			fmt.Printf("  OK       %s\n", r.FileName)
		case standards.DiffModified:
			fmt.Printf("  MODIFIED %s\n", r.FileName)
		case standards.DiffMissing:
			fmt.Printf("  MISSING  %s\n", r.FileName)
		case standards.DiffExtra:
			fmt.Printf("  EXTRA    %s\n", r.FileName)
		}
	}
	return nil
}

func runStandardsList(cmd *cobra.Command, args []string) error {
	m, err := standards.Parse()
	if err != nil {
		return err
	}

	fmt.Println("Teams:")
	for name, team := range m.Teams {
		fmt.Printf("  %-20s default_langs=%s\n", name, strings.Join(team.DefaultLangs, ","))
	}

	fmt.Println("\nLanguages:")
	for name, lang := range m.Languages {
		var validationNames []string
		for k := range lang.Validation {
			validationNames = append(validationNames, k)
		}
		fmt.Printf("  %-20s validation=%s\n", name, strings.Join(validationNames, ","))
	}

	return nil
}

// parseTeamLangs parses the optional argument into team and langs.
// "" → ("", nil)
// "backend" → ("backend", nil)
// "backend.go" → ("backend", ["go"])
// "backend.go,python" → ("backend", ["go","python"])
func parseTeamLangs(args []string) (team string, langs []string) {
	if len(args) == 0 {
		return "", nil
	}
	arg := args[0]
	dot := strings.IndexByte(arg, '.')
	if dot < 0 {
		return arg, nil
	}
	team = arg[:dot]
	rest := arg[dot+1:]
	for _, l := range strings.Split(rest, ",") {
		l = strings.TrimSpace(l)
		if l != "" {
			langs = append(langs, l)
		}
	}
	return team, langs
}

// ── skills subcommand ──

var standardsSkillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "List available piki skills",
	Long:  "List all skills in the piki standards manifest.\nUse --team to filter by team, --installed to show only installed.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
	RunE: func(cmd *cobra.Command, args []string) error {
		skills, names, err := standards.ListSkills()
		if err != nil {
			return err
		}

		// Build team→skills reverse map
		m, _ := standards.Parse()
		teamOf := map[string][]string{} // skill → teams
		if m != nil {
			for tName, tl := range m.Teams {
				for _, sn := range tl.Skills {
					teamOf[sn] = append(teamOf[sn], tName)
				}
			}
		}

		// Filter by team
		if standardsSkillsTeam != "" {
			teamSkills, err := standards.SkillsForTeam(standardsSkillsTeam)
			if err != nil {
				return err
			}
			allowed := map[string]bool{}
			for _, s := range teamSkills {
				allowed[s] = true
			}
			// Also include auto_install
			for _, n := range names {
				if skills[n].AutoInstall {
					allowed[n] = true
				}
			}
			var filtered []string
			for _, n := range names {
				if allowed[n] {
					filtered = append(filtered, n)
				}
			}
			names = filtered
		}

		// Filter by installed
		if standardsSkillsInstalled {
			var filtered []string
			for _, n := range names {
				path := fmt.Sprintf(".claude/skills/%s/skill.md", n)
				if _, err := os.Stat(path); err == nil {
					filtered = append(filtered, n)
				}
			}
			names = filtered
		}

		if len(names) == 0 {
			fmt.Println("No skills found.")
			return nil
		}

		fmt.Printf("Skills (%d):\n", len(names))
		for _, n := range names {
			se := skills[n]
			tag := "     "
			if se.AutoInstall {
				tag = "[auto]"
			}
			teams := ""
			if ts, ok := teamOf[n]; ok {
				teams = " (" + strings.Join(ts, ",") + ")"
			}
			installed := " "
			path := fmt.Sprintf(".claude/skills/%s/skill.md", n)
			if _, err := os.Stat(path); err == nil {
				installed = "✓"
			}
			fmt.Printf("  %s %s %-25s%s\n", installed, tag, n, teams)
		}
		return nil
	},
}

// ── install subcommand ──

var standardsInstallCmd = &cobra.Command{
	Use:   "install <skill> [skill...]",
	Short: "Install piki skills into the project",
	Long:  "Install one or more piki skills from the embedded standards into .claude/skills/.",
	Args:  cobra.MinimumNArgs(1),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, name := range args {
			if err := standards.InstallSkill(name); err != nil {
				fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", name, err)
			} else {
				fmt.Fprintf(os.Stderr, "  ✓ %s installed\n", name)
			}
		}
		return nil
	},
}
