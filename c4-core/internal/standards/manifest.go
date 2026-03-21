package standards

import (
	"fmt"

	pkgstandards "github.com/changmin/c4-core/cmd/c4/standards_src"
	"gopkg.in/yaml.v3"
)

// Manifest is the top-level structure of manifest.yaml.
type Manifest struct {
	Version   int                    `yaml:"version"`
	Source    string                 `yaml:"source"`
	Common    CommonLayer            `yaml:"common"`
	Languages map[string]LangLayer   `yaml:"languages"`
	Teams     map[string]TeamLayer   `yaml:"teams"`
	Skills    map[string]SkillEntry  `yaml:"skills"`
	Doctor    DoctorConfig           `yaml:"doctor"`
}

// CommonLayer holds rules and files that are always applied.
type CommonLayer struct {
	Rules []string      `yaml:"rules"`
	Files []FileMapping `yaml:"files"`
}

// FileMapping describes a file copy operation from the standards FS to a project.
type FileMapping struct {
	Src       string `yaml:"src"`
	Dst       string `yaml:"dst"`
	Merge     bool   `yaml:"merge"`
	Overwrite *bool  `yaml:"overwrite"` // nil means true (default)
}

// LangLayer holds language-specific rules and validation commands.
type LangLayer struct {
	Aliases    []string            `yaml:"aliases"`
	Rules      []string            `yaml:"rules"`
	Validation map[string]string   `yaml:"validation"`
	Scaffold   ScaffoldConfig      `yaml:"scaffold"`
}

// ScaffoldConfig describes directories and files to create for a language.
type ScaffoldConfig struct {
	Dirs  []string `yaml:"dirs"`
	Files []string `yaml:"files"`
}

// TeamLayer holds team-specific rules and default language selections.
type TeamLayer struct {
	Rules        []string `yaml:"rules"`
	DefaultLangs []string `yaml:"default_langs"`
	Domain       string   `yaml:"domain"`
}

// SkillEntry describes a skill that can be installed into a project.
type SkillEntry struct {
	Src         string `yaml:"src"`
	AutoInstall bool   `yaml:"auto_install"`
}

// DoctorConfig holds cq doctor check configuration.
type DoctorConfig struct {
	CheckVersion   bool     `yaml:"check_version"`
	CheckGitignore []string `yaml:"check_gitignore"`
	CheckFiles     []string `yaml:"check_files"`
}

// Parse reads and parses manifest.yaml from the embedded standards FS.
func Parse() (*Manifest, error) {
	data, err := pkgstandards.FS.ReadFile("manifest.yaml")
	if err != nil {
		return nil, fmt.Errorf("standards: read manifest.yaml: %w", err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("standards: parse manifest.yaml: %w", err)
	}

	return &m, nil
}

// ResolveLanguage finds a LangLayer by name or alias. Returns the canonical name
// and the layer, or an empty string and nil if not found.
func (m *Manifest) ResolveLanguage(nameOrAlias string) (string, *LangLayer) {
	// direct match
	if l, ok := m.Languages[nameOrAlias]; ok {
		return nameOrAlias, &l
	}
	// alias match
	for canonical, l := range m.Languages {
		for _, alias := range l.Aliases {
			if alias == nameOrAlias {
				return canonical, &l
			}
		}
	}
	return "", nil
}
