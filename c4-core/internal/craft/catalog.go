package craft

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

//go:embed presets/*
var presetsFS embed.FS

// PresetType identifies the category of a preset.
type PresetType string

const (
	TypeSkill    PresetType = "skill"
	TypeAgent    PresetType = "agent"
	TypeRule     PresetType = "rule"
	TypeClaudeMd PresetType = "claude-md"
)

// Preset represents a single installable preset (skill, agent, or rule).
type Preset struct {
	Name        string
	Type        PresetType
	Description string
	Content     []byte
	Path        string // relative path inside embed.FS
}

// List returns all available presets by walking the embedded filesystem.
// Skills are loaded from presets/skills/{name}/SKILL.md,
// agents from presets/agents/{name}.md,
// rules from presets/rules/{name}.md.
func List() ([]Preset, error) {
	var presets []Preset

	skills, err := listSkills()
	if err != nil {
		return nil, fmt.Errorf("craft: list skills: %w", err)
	}
	presets = append(presets, skills...)

	agents, err := listAgents()
	if err != nil {
		return nil, fmt.Errorf("craft: list agents: %w", err)
	}
	presets = append(presets, agents...)

	rules, err := listRules()
	if err != nil {
		return nil, fmt.Errorf("craft: list rules: %w", err)
	}
	presets = append(presets, rules...)

	claudeMds, err := listClaudeMds()
	if err != nil {
		return nil, fmt.Errorf("craft: list claude-md: %w", err)
	}
	presets = append(presets, claudeMds...)

	return presets, nil
}

// Find returns a preset by name, searching skills → agents → rules in order.
func Find(name string) (*Preset, error) {
	presets, err := List()
	if err != nil {
		return nil, err
	}
	for i := range presets {
		if presets[i].Name == name {
			return &presets[i], nil
		}
	}
	return nil, fmt.Errorf("craft: preset %q not found", name)
}

func listSkills() ([]Preset, error) {
	const base = "presets/skills"
	entries, err := presetsFS.ReadDir(base)
	if err != nil {
		return nil, err
	}

	var out []Preset
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		filePath := path.Join(base, e.Name(), "SKILL.md")
		data, err := presetsFS.ReadFile(filePath)
		if err != nil {
			continue
		}
		out = append(out, Preset{
			Name:        e.Name(),
			Type:        TypeSkill,
			Description: parseFrontmatterDescription(data),
			Content:     data,
			Path:        filePath,
		})
	}
	return out, nil
}

func listAgents() ([]Preset, error) {
	return listMarkdownFiles("presets/agents", TypeAgent)
}

func listRules() ([]Preset, error) {
	return listMarkdownFiles("presets/rules", TypeRule)
}

func listClaudeMds() ([]Preset, error) {
	return listMarkdownFiles("presets/claude-md", TypeClaudeMd)
}

func listMarkdownFiles(base string, t PresetType) ([]Preset, error) {
	var out []Preset
	err := fs.WalkDir(presetsFS, base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".md") {
			return nil
		}
		data, err := presetsFS.ReadFile(p)
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(path.Base(p), ".md")
		out = append(out, Preset{
			Name:        name,
			Type:        t,
			Description: parseFrontmatterDescription(data),
			Content:     data,
			Path:        p,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// parseFrontmatterDescription extracts the description field from YAML frontmatter.
// It supports both single-line and block scalar (|) values.
// Returns an empty string when no frontmatter or description is found.
func parseFrontmatterDescription(data []byte) string {
	content := string(data)

	// Must start with ---
	if !strings.HasPrefix(content, "---") {
		return ""
	}
	rest := content[3:]

	// Find closing ---
	end := strings.Index(rest, "\n---")
	if end == -1 {
		return ""
	}
	frontmatter := rest[:end]

	lines := strings.Split(frontmatter, "\n")
	var descLines []string
	inDesc := false

	for _, line := range lines {
		if strings.HasPrefix(line, "description:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			if value == "|" || value == "" {
				// Block scalar — collect following indented lines
				inDesc = true
				continue
			}
			// Inline value
			return strings.Trim(value, `"'`)
		}
		if inDesc {
			if line == "" || (len(line) > 0 && line[0] == ' ') {
				// Still inside block scalar
				descLines = append(descLines, strings.TrimSpace(line))
			} else {
				// New key — description block ended
				break
			}
		}
	}

	if len(descLines) > 0 {
		// Return first non-empty line as the summary
		for _, l := range descLines {
			if l != "" {
				return l
			}
		}
	}
	return ""
}
