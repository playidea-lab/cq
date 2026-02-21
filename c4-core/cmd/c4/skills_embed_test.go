package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// makeTestEmbedFS constructs a minimal fstest.MapFS that looks like skills_src/
// with the given skill directories and a .version file.
func makeTestEmbedFS(version string, skills ...string) fs.FS {
	m := fstest.MapFS{}
	m["skills_src/.version"] = &fstest.MapFile{Data: []byte(version + "\n")}
	for _, name := range skills {
		m["skills_src/"+name+"/SKILL.md"] = &fstest.MapFile{Data: []byte("# " + name + "\n")}
	}
	return m
}

// TestSetupSkillsNoSourceRoot verifies that when findC4Root fails but
// EmbeddedSkillsFS is set, skills are extracted to ~/.c4/skills/ and
// symlinked into the project.
func TestSetupSkillsNoSourceRoot(t *testing.T) {
	// Disable findC4Root
	t.Setenv("C4_SOURCE_ROOT", "")
	oldBuiltin := builtinC4Root
	builtinC4Root = ""
	defer func() { builtinC4Root = oldBuiltin }()

	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	// Inject embedded FS
	oldEmbed := EmbeddedSkillsFS
	EmbeddedSkillsFS = makeTestEmbedFS("abc123", "c4-run", "c4-plan", "c4-finish")
	defer func() { EmbeddedSkillsFS = oldEmbed }()

	projectDir := t.TempDir()
	if err := setupSkills(projectDir); err != nil {
		t.Fatalf("setupSkills failed: %v", err)
	}

	// Skills should appear in ~/.c4/skills/
	c4Skills := filepath.Join(fakeHome, ".c4", "skills")
	for _, skill := range []string{"c4-run", "c4-plan", "c4-finish"} {
		skillFile := filepath.Join(c4Skills, skill, "SKILL.md")
		if _, err := os.Stat(skillFile); err != nil {
			t.Errorf("expected extracted skill file %s: %v", skillFile, err)
		}
	}

	// Skills should be symlinked into project .claude/skills/
	for _, skill := range []string{"c4-run", "c4-plan", "c4-finish"} {
		link := filepath.Join(projectDir, ".claude", "skills", skill)
		info, err := os.Lstat(link)
		if err != nil {
			t.Errorf("expected symlink for skill %s: %v", skill, err)
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("skill %s: expected symlink, got %v", skill, info.Mode())
		}
	}
}

// TestSetupSkillsWithSourceRoot verifies that when findC4Root succeeds,
// symlink mode is used and EmbeddedSkillsFS is NOT consulted.
func TestSetupSkillsWithSourceRoot(t *testing.T) {
	// Create a fake c4Root with skills
	c4Root := t.TempDir()
	skillsDir := filepath.Join(c4Root, ".claude", "skills")
	skillDir := filepath.Join(skillsDir, "c4-run")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# c4-run\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("C4_SOURCE_ROOT", c4Root)

	// Also inject embedded FS — it should NOT be used
	oldEmbed := EmbeddedSkillsFS
	EmbeddedSkillsFS = makeTestEmbedFS("abc123", "embed-only-skill")
	defer func() { EmbeddedSkillsFS = oldEmbed }()

	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	projectDir := t.TempDir()
	if err := setupSkills(projectDir); err != nil {
		t.Fatalf("setupSkills failed: %v", err)
	}

	// c4-run should be symlinked (source root mode)
	link := filepath.Join(projectDir, ".claude", "skills", "c4-run")
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("c4-run symlink not created: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("c4-run: expected symlink")
	}

	// embed-only-skill should NOT be extracted (embedded path not taken)
	embedExtracted := filepath.Join(fakeHome, ".c4", "skills", "embed-only-skill")
	if _, err := os.Stat(embedExtracted); err == nil {
		t.Error("embed-only-skill should not be extracted when source root is found")
	}
}

// TestExtractSkillsVersionUpdate verifies that when the installed version
// differs from the embedded version, skills are re-extracted.
func TestExtractSkillsVersionUpdate(t *testing.T) {
	destDir := t.TempDir()

	// Pre-install with old version
	oldVersionFile := filepath.Join(destDir, ".version")
	if err := os.WriteFile(oldVersionFile, []byte("old-sha\n"), 0644); err != nil {
		t.Fatalf("write old version: %v", err)
	}
	// Old content in destDir
	oldSkillDir := filepath.Join(destDir, "old-skill")
	if err := os.MkdirAll(oldSkillDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Inject new embedded FS with different version
	oldEmbed := EmbeddedSkillsFS
	EmbeddedSkillsFS = makeTestEmbedFS("new-sha", "new-skill")
	defer func() { EmbeddedSkillsFS = oldEmbed }()

	if err := extractEmbeddedSkills(destDir); err != nil {
		t.Fatalf("extractEmbeddedSkills: %v", err)
	}

	// new-skill should now be present
	newSkillFile := filepath.Join(destDir, "new-skill", "SKILL.md")
	if _, err := os.Stat(newSkillFile); err != nil {
		t.Errorf("new-skill not extracted after version update: %v", err)
	}

	// Version file should be updated
	versionData, err := os.ReadFile(oldVersionFile)
	if err != nil {
		t.Fatalf("read version: %v", err)
	}
	if strings.TrimSpace(string(versionData)) != "new-sha" {
		t.Errorf("version not updated: %q", string(versionData))
	}
}

// TestExtractSkillsIdempotent verifies that when the installed version
// matches the embedded version, extraction is skipped (fast path).
func TestExtractSkillsIdempotent(t *testing.T) {
	destDir := t.TempDir()

	// Pre-install with matching version
	versionFile := filepath.Join(destDir, ".version")
	if err := os.WriteFile(versionFile, []byte("same-sha\n"), 0644); err != nil {
		t.Fatalf("write version: %v", err)
	}

	// Create a sentinel file that would be overwritten on re-extraction
	sentinelDir := filepath.Join(destDir, "test-skill")
	if err := os.MkdirAll(sentinelDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sentinelFile := filepath.Join(sentinelDir, "sentinel.txt")
	if err := os.WriteFile(sentinelFile, []byte("original"), 0644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	// Inject embedded FS with same version but different skill content
	oldEmbed := EmbeddedSkillsFS
	EmbeddedSkillsFS = makeTestEmbedFS("same-sha", "different-skill")
	defer func() { EmbeddedSkillsFS = oldEmbed }()

	if err := extractEmbeddedSkills(destDir); err != nil {
		t.Fatalf("extractEmbeddedSkills: %v", err)
	}

	// Sentinel should not be overwritten (extraction was skipped)
	data, err := os.ReadFile(sentinelFile)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}
	if string(data) != "original" {
		t.Error("sentinel file was modified (extraction should have been skipped)")
	}
}

// TestExtractSkillsReadOnlyDir verifies that when the destination directory
// is not writable, extractEmbeddedSkills logs a warning and returns nil (no panic).
func TestExtractSkillsReadOnlyDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; read-only permission test not meaningful")
	}

	destDir := t.TempDir()

	// Inject embedded FS
	oldEmbed := EmbeddedSkillsFS
	EmbeddedSkillsFS = makeTestEmbedFS("abc123", "c4-run")
	defer func() { EmbeddedSkillsFS = oldEmbed }()

	// Make destDir read-only after creation (mkdir will fail for subdirs)
	if err := os.Chmod(destDir, 0555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(destDir, 0755) // restore for cleanup

	// Should return nil, not panic
	err := extractEmbeddedSkills(destDir)
	if err != nil {
		t.Errorf("expected nil on read-only dir, got: %v", err)
	}
}

// TestExtractSkillsCorruptedVersion verifies that when .version is corrupted
// (e.g. binary garbage), extraction proceeds normally.
func TestExtractSkillsCorruptedVersion(t *testing.T) {
	destDir := t.TempDir()

	// Write corrupted .version
	versionFile := filepath.Join(destDir, ".version")
	if err := os.WriteFile(versionFile, []byte("\x00\xff\x00corrupted"), 0644); err != nil {
		t.Fatalf("write corrupted version: %v", err)
	}

	// Inject embedded FS
	oldEmbed := EmbeddedSkillsFS
	EmbeddedSkillsFS = makeTestEmbedFS("clean-sha", "c4-run")
	defer func() { EmbeddedSkillsFS = oldEmbed }()

	if err := extractEmbeddedSkills(destDir); err != nil {
		t.Fatalf("extractEmbeddedSkills with corrupted version: %v", err)
	}

	// Skill should be extracted despite corrupted version
	skillFile := filepath.Join(destDir, "c4-run", "SKILL.md")
	if _, err := os.Stat(skillFile); err != nil {
		t.Errorf("skill not extracted after corrupted version: %v", err)
	}

	// Version should be updated to the clean SHA
	data, err := os.ReadFile(versionFile)
	if err != nil {
		t.Fatalf("read version: %v", err)
	}
	if strings.TrimSpace(string(data)) != "clean-sha" {
		t.Errorf("version not updated: %q", string(data))
	}
}
