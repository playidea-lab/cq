package craft

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// InstalledItem represents a preset that has been installed into ~/.claude/.
type InstalledItem struct {
	Name   string
	Type   PresetType
	Path   string // absolute path on disk
	Source string // remote source URL if installed via cq add <url>, else empty
}

// Install copies a preset into the user's ~/.claude/ directory.
//
// Destination layout:
//   - skill → {homeDir}/.claude/skills/{name}/SKILL.md
//   - agent → {homeDir}/.claude/agents/{name}.md
//   - rule  → {homeDir}/.claude/rules/{name}.md
//
// Returns the destination path on success.
// Returns an error (without overwriting) if the destination already exists.
func Install(preset *Preset, homeDir string) (string, error) {
	dest, err := destPath(preset, homeDir)
	if err != nil {
		return "", err
	}

	if _, statErr := os.Stat(dest); statErr == nil {
		return "", fmt.Errorf("craft: %s already exists at %s", preset.Name, dest)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", fmt.Errorf("craft: create directory: %w", err)
	}

	if err := os.WriteFile(dest, preset.Content, 0o644); err != nil {
		return "", fmt.Errorf("craft: write file: %w", err)
	}

	if err := OpenInEditor(dest); err != nil {
		// Non-fatal: file is already written; just surface the info.
		fmt.Fprintf(os.Stderr, "craft: could not open editor: %v\n", err)
	}

	return dest, nil
}

// Remove deletes a user-installed preset from ~/.claude/.
//
//   - skill → removes the directory {homeDir}/.claude/skills/{name}/
//   - agent → removes {homeDir}/.claude/agents/{name}.md
//   - rule  → removes {homeDir}/.claude/rules/{name}.md
//
// Remove infers the type by scanning the three possible locations.
// If the name is ambiguous (exists in more than one location), it removes the
// first match found (skill → agent → rule).
func Remove(name string, homeDir string) error {
	// Try skill directory first.
	skillDir := filepath.Join(homeDir, ".claude", "skills", name)
	if _, err := os.Stat(skillDir); err == nil {
		if err := os.RemoveAll(skillDir); err != nil {
			return fmt.Errorf("craft: remove skill directory: %w", err)
		}
		return nil
	}

	// Try agent file.
	agentFile := filepath.Join(homeDir, ".claude", "agents", name+".md")
	if _, err := os.Stat(agentFile); err == nil {
		if err := os.Remove(agentFile); err != nil {
			return fmt.Errorf("craft: remove agent file: %w", err)
		}
		return nil
	}

	// Try rule file.
	ruleFile := filepath.Join(homeDir, ".claude", "rules", name+".md")
	if _, err := os.Stat(ruleFile); err == nil {
		if err := os.Remove(ruleFile); err != nil {
			return fmt.Errorf("craft: remove rule file: %w", err)
		}
		return nil
	}

	// Try CLAUDE.md in current directory.
	claudeMdFile := filepath.Join(".", "CLAUDE.md")
	if _, err := os.Stat(claudeMdFile); err == nil && name == "CLAUDE" {
		if err := os.Remove(claudeMdFile); err != nil {
			return fmt.Errorf("craft: remove CLAUDE.md: %w", err)
		}
		return nil
	}

	return fmt.Errorf("craft: %q not found in ~/.claude/", name)
}

// ListInstalled scans ~/.claude/skills/, ~/.claude/agents/, and ~/.claude/rules/
// and returns all installed items. Only homeDir-based paths are included;
// project-local .claude/ directories are not scanned.
func ListInstalled(homeDir string) ([]InstalledItem, error) {
	var items []InstalledItem

	// Skills: ~/.claude/skills/*/SKILL.md
	skillsDir := filepath.Join(homeDir, ".claude", "skills")
	if entries, err := os.ReadDir(skillsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			skillFile := filepath.Join(skillsDir, e.Name(), "SKILL.md")
			if _, err := os.Stat(skillFile); err == nil {
				items = append(items, InstalledItem{
					Name:   e.Name(),
					Type:   TypeSkill,
					Path:   skillFile,
					Source: readSourceComment(skillFile),
				})
			}
		}
	}

	// Agents: ~/.claude/agents/*.md
	items = append(items, scanMarkdownDir(
		filepath.Join(homeDir, ".claude", "agents"),
		TypeAgent,
	)...)

	// Rules: ~/.claude/rules/*.md
	items = append(items, scanMarkdownDir(
		filepath.Join(homeDir, ".claude", "rules"),
		TypeRule,
	)...)

	return items, nil
}

// OpenInEditor tries to open path in a GUI/terminal editor.
//
// Priority:
//  1. code (VS Code)
//  2. open (macOS Finder / default app)
//  3. prints the path to stdout as a fallback
func OpenInEditor(path string) error {
	for _, bin := range []string{"code", "open"} {
		if _, err := exec.LookPath(bin); err == nil {
			cmd := exec.Command(bin, path)
			if err := cmd.Start(); err != nil {
				continue
			}
			return nil
		}
	}
	// Fallback: print path so the user can open it manually.
	fmt.Println(path)
	return nil
}

// destPath computes the installation destination for a preset.
func destPath(preset *Preset, homeDir string) (string, error) {
	base := filepath.Join(homeDir, ".claude")
	switch preset.Type {
	case TypeSkill:
		return filepath.Join(base, "skills", preset.Name, "SKILL.md"), nil
	case TypeAgent:
		return filepath.Join(base, "agents", preset.Name+".md"), nil
	case TypeRule:
		return filepath.Join(base, "rules", preset.Name+".md"), nil
	case TypeClaudeMd:
		return filepath.Join(".", "CLAUDE.md"), nil
	default:
		return "", fmt.Errorf("craft: unknown preset type %q", preset.Type)
	}
}

// scanMarkdownDir returns InstalledItems for all *.md files in dir.
func scanMarkdownDir(dir string, t PresetType) []InstalledItem {
	var items []InstalledItem
	_ = fs.WalkDir(os.DirFS(dir), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		name := strings.TrimSuffix(filepath.Base(p), ".md")
		absPath := filepath.Join(dir, p)
		items = append(items, InstalledItem{
			Name:   name,
			Type:   t,
			Path:   absPath,
			Source: readSourceComment(absPath),
		})
		return nil
	})
	return items
}

// RestoreExtraFiles writes extra_files from a registry version into the skill
// directory, preserving subdirectory structure (references/, examples/, scripts/).
// Keys are relative paths like "references/patterns.md".
func RestoreExtraFiles(skillName string, extraFilesJSON json.RawMessage, homeDir string) {
	var files map[string]string
	if err := json.Unmarshal(extraFilesJSON, &files); err != nil {
		return
	}

	skillDir := filepath.Join(homeDir, ".claude", "skills", skillName)
	for relPath, content := range files {
		dest := filepath.Join(skillDir, relPath)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			continue
		}
		_ = os.WriteFile(dest, []byte(content), 0o644)
	}
}

// readSourceComment reads the first "# source: <url>" line from a file.
// Returns an empty string if the file cannot be read or no such line exists.
func readSourceComment(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.SplitN(string(data), "\n", 10) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# source:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# source:"))
		}
	}
	return ""
}
