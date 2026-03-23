package standards

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	pkgstandards "github.com/changmin/c4-core/cmd/c4/standards_src"
	"gopkg.in/yaml.v3"
)

// ApplyOptions controls Apply() behaviour.
type ApplyOptions struct {
	// Force overwrites files that have been locally modified since last Apply.
	// When false (default), modified files are skipped and reported in FilesSkipped.
	Force bool
}

// ApplyResult reports what Apply() did.
type ApplyResult struct {
	FilesCreated []string
	FilesSkipped []string // already existed with overwrite=false, or user-modified without Force
	FilesRemoved []string // orphaned files removed after team/lang change
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
func Apply(projectDir string, team string, langs []string, opts ApplyOptions) (*ApplyResult, error) {
	m, err := Parse()
	if err != nil {
		return nil, err
	}

	// Read old lock (if any) to detect orphans and modified files.
	oldLock, _ := ReadLock(projectDir) // ignore error — may not exist yet

	// Build a map from dst → hash from the old lock for fast lookup.
	oldHashByDst := make(map[string]string)
	if oldLock != nil {
		for _, e := range oldLock.Files {
			oldHashByDst[e.Dst] = e.Hash
		}
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
				// Skip if content already present (prevents duplicate appends on re-apply)
				if bytes.Contains(existing, bytes.TrimSpace(data)) {
					result.FilesSkipped = append(result.FilesSkipped, dst)
					return nil
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

			// Check if the file was modified by the user since last Apply.
			if !opts.Force {
				if lockHash, tracked := oldHashByDst[dst]; tracked {
					existing, err2 := os.ReadFile(dstAbs)
					if err2 == nil {
						currentHash := fmt.Sprintf("%x", sha256.Sum256(existing))[:8]
						if currentHash != lockHash {
							// User has modified this file — skip it.
							result.FilesSkipped = append(result.FilesSkipped, dst)
							// Still record in lock so it is not treated as orphan.
							lockEntries = append(lockEntries, lockEntry{Src: src, Dst: dst, Hash: currentHash})
							return nil
						}
					}
				}
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

	// copyRule copies a rules file to .claude/rules/<name>.
	// For language/team rules under a subdirectory (e.g. rules/go/style.md),
	// the parent directory is prefixed to avoid collisions: go-style.md.
	copyRule := func(src string) error {
		base := filepath.Base(src)
		dir := filepath.Base(filepath.Dir(src))
		// common rules keep their name; lang/team rules get prefixed
		if dir != "rules" && dir != "common" {
			base = dir + "-" + base
		}
		dst := filepath.Join(".claude", "rules", base)
		return copyFile(src, dst, false, nil)
	}

	// 1. Common rules.
	for _, rule := range m.Common.Rules {
		if err := copyRule(rule); err != nil {
			return nil, err
		}
	}

	// 2. Common hooks (bash-security.sh, stop-guard.sh, session-start.sh).
	for _, hm := range m.Common.Hooks {
		if err := copyFile(hm.Src, hm.Dst, false, nil); err != nil {
			return nil, err
		}
		if hm.Executable {
			dstAbs := filepath.Join(projectDir, hm.Dst)
			_ = os.Chmod(dstAbs, 0o755)
		}
	}

	// 3. Common file mappings (agents.md → CLAUDE.md, soul.md, settings, config).
	for _, fm := range m.Common.Files {
		if err := copyFile(fm.Src, fm.Dst, fm.Merge, fm.Overwrite); err != nil {
			return nil, err
		}
	}

	// 4. Resolve langs — if empty and team is given, use team defaults.
	resolvedLangs := langs
	if len(resolvedLangs) == 0 && team != "" {
		if tl, ok := m.Teams[team]; ok {
			resolvedLangs = tl.DefaultLangs
		}
	}
	result.Langs = resolvedLangs

	// 5. Language layers.
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

	// 6. Team layer.
	if team != "" {
		if tl, ok := m.Teams[team]; ok {
			for _, rule := range tl.Rules {
				if err := copyRule(rule); err != nil {
					return nil, err
				}
			}
		}
	}

	// 7. Auto-install skills.
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

	// 7.5. Team-specific skills.
	if team != "" {
		if tl, ok := m.Teams[team]; ok {
			for _, skillName := range tl.Skills {
				se, exists := m.Skills[skillName]
				if !exists {
					continue
				}
				_ = fs.WalkDir(pkgstandards.FS, "skills", func(path string, d fs.DirEntry, walkErr error) error {
					if walkErr != nil || d.IsDir() {
						return nil
					}
					if se.Src == path {
						base := filepath.Base(path)
						dst := filepath.Join(".claude", "skills", skillName, base)
						_ = copyFile(path, dst, false, nil)
					}
					return nil
				})
			}
		}
	}

	// 8. Write .piki-lock.yaml.
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

	// 9. Remove orphaned files — present in old lock but not in new lock.
	if oldLock != nil {
		newDsts := make(map[string]bool, len(lockEntries))
		for _, e := range lockEntries {
			newDsts[e.Dst] = true
		}
		for _, e := range oldLock.Files {
			if newDsts[e.Dst] {
				continue
			}
			dstAbs := filepath.Join(projectDir, e.Dst)
			if rmErr := os.Remove(dstAbs); rmErr == nil {
				result.FilesRemoved = append(result.FilesRemoved, e.Dst)
			}
		}
	}

	return result, nil
}

// installEmbeddedFile copies a file from the embedded FS to a destination path.
func installEmbeddedFile(src, dst string) error {
	data, err := fs.ReadFile(pkgstandards.FS, src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// InstallSkill installs a specific skill by name into .claude/skills/<name>/.
func InstallSkill(skillName string) error {
	m, err := Parse()
	if err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	se, ok := m.Skills[skillName]
	if !ok {
		return fmt.Errorf("skill %q not found in manifest", skillName)
	}

	dst := filepath.Join(".claude", "skills", skillName, filepath.Base(se.Src))
	if err := installEmbeddedFile(se.Src, dst); err != nil {
		return fmt.Errorf("install %s: %w", skillName, err)
	}
	return nil
}

// ListSkills returns all skill entries and sorted names from the manifest.
func ListSkills() (map[string]SkillEntry, []string, error) {
	m, err := Parse()
	if err != nil {
		return nil, nil, err
	}
	return m.Skills, sortedKeys(m.Skills), nil
}

// SkillsForTeam returns the skill names mapped to a team.
func SkillsForTeam(teamName string) ([]string, error) {
	m, err := Parse()
	if err != nil {
		return nil, err
	}
	tl, ok := m.Teams[teamName]
	if !ok {
		return nil, fmt.Errorf("team %q not found", teamName)
	}
	return tl.Skills, nil
}

func sortedKeys(m map[string]SkillEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple sort
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}
