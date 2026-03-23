package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/changmin/c4-core/internal/cloud"
	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/store"
	"github.com/spf13/cobra"
)

// validCloudModes lists accepted values for cloud.mode.
var validCloudModes = []string{config.CloudModeLocalFirst, config.CloudModePrimary}

var cloudCmd = &cobra.Command{
	Use:   "cloud",
	Short: "Manage cloud settings",
	Long: `Manage C4 Cloud configuration.

Subcommands:
  mode get          - Show the current cloud.mode value
  mode set <value>  - Update cloud.mode in .c4/config.yaml

Valid modes:
  local-first    Writes go to SQLite first, then async to cloud (default)
  cloud-primary  Writes go to cloud first, then async to local`,
}

var cloudModeCmd = &cobra.Command{
	Use:   "mode",
	Short: "Get or set cloud.mode",
}

var cloudModeGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Show the current cloud.mode value",
	Args:  cobra.NoArgs,
	RunE:  runCloudModeGet,
}

var cloudModeSetCmd = &cobra.Command{
	Use:   "set <value>",
	Short: "Update cloud.mode in .c4/config.yaml (local-first | cloud-primary)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCloudModeSet,
}

func init() {
	cloudModeCmd.AddCommand(cloudModeGetCmd)
	cloudModeCmd.AddCommand(cloudModeSetCmd)
	cloudCmd.AddCommand(cloudModeCmd)
	rootCmd.AddCommand(cloudCmd)
}

func runCloudModeGet(cmd *cobra.Command, args []string) error {
	cfgMgr, err := config.New(projectDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	mode := cfgMgr.GetConfig().Cloud.Mode
	if mode == "" {
		mode = config.CloudModeLocalFirst
	}
	fmt.Println(mode)
	return nil
}

func runCloudModeSet(cmd *cobra.Command, args []string) error {
	value := args[0]
	if !isValidCloudMode(value) {
		return fmt.Errorf("invalid cloud mode %q: must be one of %v", value, validCloudModes)
	}

	configPath := filepath.Join(projectDir, ".c4", "config.yaml")
	if err := writeCloudModeToYAML(configPath, value); err != nil {
		return fmt.Errorf("updating config: %w", err)
	}

	fmt.Fprintf(os.Stderr, "cq: cloud.mode=%s written to .c4/config.yaml (restart required)\n", value)
	return nil
}

func isValidCloudMode(mode string) bool {
	for _, v := range validCloudModes {
		if mode == v {
			return true
		}
	}
	return false
}

// writeCloudModeToYAML writes cloud.mode: <value> into .c4/config.yaml.
// Searches only within the cloud: section to avoid matching mode: in other sections.
func writeCloudModeToYAML(configPath, value string) error {
	var existing string
	if data, err := os.ReadFile(configPath); err == nil {
		existing = string(data)
	}

	lines := strings.Split(existing, "\n")

	const modeKey = "  mode: "
	const cloudKey = "cloud:"

	// Find cloud: section start and end (end = next top-level key or EOF).
	cloudStart := -1
	cloudEnd := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == cloudKey {
			cloudStart = i
			continue
		}
		if cloudStart >= 0 && cloudEnd < 0 {
			// A top-level key is non-empty, not indented, and not a comment.
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				cloudEnd = i
				break
			}
		}
	}
	if cloudStart >= 0 && cloudEnd < 0 {
		cloudEnd = len(lines)
	}

	// Look for mode: only inside the cloud section.
	modeLineIdx := -1
	if cloudStart >= 0 {
		for i := cloudStart + 1; i < cloudEnd; i++ {
			if strings.HasPrefix(lines[i], modeKey) {
				modeLineIdx = i
				break
			}
		}
	}

	if modeLineIdx >= 0 {
		// Update existing mode line.
		lines[modeLineIdx] = modeKey + value
		return os.WriteFile(configPath, []byte(strings.Join(lines, "\n")), 0644)
	}

	if cloudStart >= 0 {
		// Insert mode as first line inside cloud: section.
		newLines := make([]string, 0, len(lines)+1)
		newLines = append(newLines, lines[:cloudStart+1]...)
		newLines = append(newLines, modeKey+value)
		newLines = append(newLines, lines[cloudStart+1:]...)
		return os.WriteFile(configPath, []byte(strings.Join(newLines, "\n")), 0644)
	}

	// No cloud: section — append one.
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	existing += cloudKey + "\n" + modeKey + value + "\n"
	return os.WriteFile(configPath, []byte(existing), 0644)
}

// selectCloudStore picks the store implementation based on cloud.mode.
// "cloud-primary" → CloudPrimaryStore, anything else → HybridStore (default).
// This is extracted to allow unit testing of the factory logic.
func selectCloudStore(mode string, local, remote store.Store) store.Store {
	if mode == config.CloudModePrimary {
		return cloud.NewCloudPrimaryStore(local, remote)
	}
	return cloud.NewHybridStore(local, remote)
}

// readCloudURL reads the Supabase URL from env vars or .c4/config.yaml.
// Priority: C4_CLOUD_URL > SUPABASE_URL > config.yaml cloud.url > ldflags default.
func readCloudURL(projDir string) string {
	if v := os.Getenv("C4_CLOUD_URL"); v != "" {
		return v
	}
	if v := os.Getenv("SUPABASE_URL"); v != "" {
		return v
	}
	configPath := filepath.Join(projDir, ".c4", "config.yaml")
	if data, err := os.ReadFile(configPath); err == nil {
		if v := sectionYAMLValue(string(data), "cloud", "url:"); v != "" {
			return v
		}
	}
	return builtinSupabaseURL
}

// readCloudAnonKey reads the Supabase anon key from env vars or .c4/config.yaml.
// Priority: C4_CLOUD_ANON_KEY > SUPABASE_KEY > config.yaml cloud.anon_key > ldflags default.
func readCloudAnonKey(projDir string) string {
	if v := os.Getenv("C4_CLOUD_ANON_KEY"); v != "" {
		return v
	}
	if v := os.Getenv("SUPABASE_KEY"); v != "" {
		return v
	}
	configPath := filepath.Join(projDir, ".c4", "config.yaml")
	if data, err := os.ReadFile(configPath); err == nil {
		if v := sectionYAMLValue(string(data), "cloud", "anon_key:"); v != "" {
			return v
		}
	}
	return builtinSupabaseKey
}

// getActiveProjectID reads the active project_id from env var or .c4/config.yaml.
// Priority: C4_PROJECT_ID > config.yaml cloud.active_project_id.
func getActiveProjectID(projDir string) string {
	if v := os.Getenv("C4_PROJECT_ID"); v != "" {
		return v
	}
	configPath := filepath.Join(projDir, ".c4", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}
	return sectionYAMLValue(string(data), "cloud", "active_project_id:")
}

// getActiveProjectIDWithProjects extends getActiveProjectID with two additional
// fallbacks using the provided project list:
//  3. Directory-name matching: current working directory name vs project names (case-insensitive).
//  4. Single-project auto-select: if exactly one project exists, use it.
//
// Priority: C4_PROJECT_ID env > config active_project_id > dir-name match > single auto-select.
func getActiveProjectIDWithProjects(projDir string, projects []cloud.Project) string {
	if id := getActiveProjectID(projDir); id != "" {
		return id
	}
	if len(projects) == 0 {
		return ""
	}
	// Fallback 3: match current directory name against project names.
	if cwd, err := os.Getwd(); err == nil {
		dirName := strings.ToLower(filepath.Base(cwd))
		for _, p := range projects {
			if strings.ToLower(p.Name) == dirName {
				return p.ID
			}
		}
	}
	// Fallback 4: single project auto-select.
	if len(projects) == 1 {
		return projects[0].ID
	}
	return ""
}
