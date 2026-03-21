package standards

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	pkgstandards "github.com/changmin/c4-core/standards"
	"gopkg.in/yaml.v3"
)

// ApplyResult reports what Apply() did.
type ApplyResult struct {
	FilesCreated []string
	FilesSkipped []string // already existed with overwrite=false
	Team         string
	Langs        []string
}

// lockEntry records a single applied file in .piki-lock.yaml.
type lockEntry struct {
	Src  string `yaml:"src"`
	Dst  string `yaml:"dst"`
	Hash string `yaml:"hash"`
}

// lockFile is the structure written to .piki-lock.yaml.
type lockFile struct {
	Team      string      `yaml:"team"`
	Langs     []string    `yaml:"langs"`
	AppliedAt string      `yaml:"applied_at"`
	Files     []lockEntry `yaml:"files"`
}

// Apply copies standard files to projectDir according to the manifest layers:
// common → lang (in order) → team. Writes .piki-lock.yaml on success.
func Apply(projectDir string, team string, langs []string) (*ApplyResult, error) {
	m, err := Parse()
	if err != nil {
		return nil, err
	}

	result := &ApplyResult{
		Team:  team,
		Langs: langs,
	}

	var lockEntries []lockEntry

	// copyFile copies src from the embedded FS to dst (relative to projectDir).
	// merge=true appends to an existing file; overwrite=nil/true replaces it.
	copyFile := func(src, dst string, merge bool, overwrite *bool) error {
		data, readErr := pkgstandards.FS.ReadFile(src)
		if readErr != nil {
			return fmt.Errorf("standards: read %s: %w", src, readErr)
		}

		dstAbs := filepath.Join(projectDir, dst)
		if mkErr := os.MkdirAll(filepath.Dir(dstAbs), 0o755); mkErr != nil {
			return fmt.Errorf("standards: mkdir for %s: %w", dst, mkErr)
		}

		// Determine whether to overwrite existing files (default = true).
		doOverwrite := true
		if overwrite != nil {
			doOverwrite = *overwrite
		}

		_, statErr := os.Stat(dstAbs)
		exists := statErr == nil

		if exists {
			if merge {
				existing, err2 := os.ReadFile(dstAbs)
				if err2 != nil {
					return fmt.Errorf("standards: read existing %s: %w", dstAbs, err2)
				}
				sep := []byte{}
				if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
					sep = []byte("\n")
				}
				merged := append(existing, sep...)
				merged = append(merged, data...)
				if wErr := os.WriteFile(dstAbs, merged, 0o644); wErr != nil {
					return fmt.Errorf("standards: write merged %s: %w", dstAbs, wErr)
				}
				result.FilesCreated = append(result.FilesCreated, dst)
				hash := fmt.Sprintf("%x", sha256.Sum256(merged))[:8]
				lockEntries = append(lockEntries, lockEntry{Src: src, Dst: dst, Hash: hash})
				return nil
			}
			if !doOverwrite {
				result.FilesSkipped = append(result.FilesSkipped, dst)
				return nil
			}
		}

		if wErr := os.WriteFile(dstAbs, data, 0o644); wErr != nil {
			return fmt.Errorf("standards: write %s: %w", dstAbs, wErr)
		}
		result.FilesCreated = append(result.FilesCreated, dst)
		hash := fmt.Sprintf("%x", sha256.Sum256(data))[:8]
		lockEntries = append(lockEntries, lockEntry{Src: src, Dst: dst, Hash: hash})
		return nil
	}

	// copyRule copies a rules file to .claude/rules/<basename>.
	copyRule := func(src string) error {
		base := filepath.Base(src)
		dst := filepath.Join(".claude", "rules", base)
		return copyFile(src, dst, false, nil)
	}

	// 1. Common rules.
	for _, rule := range m.Common.Rules {
		if err := copyRule(rule); err != nil {
			return nil, err
		}
	}

	// 2. Common file mappings (agents.md → CLAUDE.md, soul.md, settings, config).
	for _, fm := range m.Common.Files {
		if err := copyFile(fm.Src, fm.Dst, fm.Merge, fm.Overwrite); err != nil {
			return nil, err
		}
	}

	// 3. Resolve langs — if empty and team is given, use team defaults.
	resolvedLangs := langs
	if len(resolvedLangs) == 0 && team != "" {
		if tl, ok := m.Teams[team]; ok {
			resolvedLangs = tl.DefaultLangs
		}
	}
	result.Langs = resolvedLangs

	// 4. Language layers.
	for _, lang := range resolvedLangs {
		_, ll := m.ResolveLanguage(lang)
		if ll == nil {
			continue
		}
		for _, rule := range ll.Rules {
			if err := copyRule(rule); err != nil {
				return nil, err
			}
		}
	}

	// 5. Team layer.
	if team != "" {
		if tl, ok := m.Teams[team]; ok {
			for _, rule := range tl.Rules {
				if err := copyRule(rule); err != nil {
					return nil, err
				}
			}
		}
	}

	// 6. Auto-install skills.
	_ = fs.WalkDir(pkgstandards.FS, "skills", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		for skillName, se := range m.Skills {
			if se.Src == path && se.AutoInstall {
				base := filepath.Base(path)
				dst := filepath.Join(".claude", "skills", skillName, base)
				_ = copyFile(path, dst, false, nil)
			}
		}
		return nil
	})

	// 7. Write .piki-lock.yaml.
	lock := lockFile{
		Team:      team,
		Langs:     resolvedLangs,
		AppliedAt: time.Now().UTC().Format(time.RFC3339),
		Files:     lockEntries,
	}
	lockData, err := yaml.Marshal(lock)
	if err != nil {
		return nil, fmt.Errorf("standards: marshal lock: %w", err)
	}
	lockPath := filepath.Join(projectDir, ".piki-lock.yaml")
	if err := os.WriteFile(lockPath, lockData, 0o644); err != nil {
		return nil, fmt.Errorf("standards: write .piki-lock.yaml: %w", err)
	}

	return result, nil
}
