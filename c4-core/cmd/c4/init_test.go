package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookNeedsUpdate(t *testing.T) {
	dir := t.TempDir()
	content := "#!/bin/bash\necho hello"
	hookPath := filepath.Join(dir, "hook.sh")

	// Missing file → needs update
	if !hookNeedsUpdate(hookPath, content) {
		t.Error("expected true for missing file")
	}

	// Write same content → no update needed
	os.WriteFile(hookPath, []byte(content), 0755)
	if hookNeedsUpdate(hookPath, content) {
		t.Error("expected false for same content")
	}

	// Different content → needs update
	if !hookNeedsUpdate(hookPath, content+"different") {
		t.Error("expected true for different content")
	}
}

func TestSetupGlobalHooks(t *testing.T) {
	homeDir := t.TempDir()

	// First run: installs hook
	if err := setupGlobalHooks(homeDir); err != nil {
		t.Fatalf("setupGlobalHooks: %v", err)
	}
	hookPath := filepath.Join(homeDir, ".claude", "hooks", "c4-bash-security-hook.sh")
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("hook file not created: %v", err)
	}
	info, _ := os.Stat(hookPath)
	if info.Mode()&0111 == 0 {
		t.Error("hook file not executable")
	}

	// Second run: idempotent
	if err := setupGlobalHooks(homeDir); err != nil {
		t.Fatalf("second setupGlobalHooks: %v", err)
	}
}

func TestSetupClaudeMD_NewFile(t *testing.T) {
	dir := t.TempDir()

	if err := setupClaudeMD(dir); err != nil {
		t.Fatalf("setupClaudeMD failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md not created: %v", err)
	}

	content := string(data)
	if !containsSubstring(content, "## CRITICAL: C4 Overrides") {
		t.Error("CLAUDE.md missing C4 Overrides marker")
	}
	if !containsSubstring(content, "EnterPlanMode") {
		t.Error("CLAUDE.md missing EnterPlanMode override")
	}
	if !containsSubstring(content, "c4_add_todo") {
		t.Error("CLAUDE.md missing c4_add_todo reference")
	}
}

func TestSetupClaudeMD_ExistingWithMarker(t *testing.T) {
	dir := t.TempDir()
	claudePath := filepath.Join(dir, "CLAUDE.md")

	original := "# My Project\n\n## CRITICAL: C4 Overrides\nAlready configured.\n"
	if err := os.WriteFile(claudePath, []byte(original), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := setupClaudeMD(dir); err != nil {
		t.Fatalf("setupClaudeMD failed: %v", err)
	}

	data, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Should be unchanged (marker already present)
	if string(data) != original {
		t.Error("CLAUDE.md was modified despite having marker")
	}
}

func TestSetupClaudeMD_ExistingWithoutMarker(t *testing.T) {
	dir := t.TempDir()
	claudePath := filepath.Join(dir, "CLAUDE.md")

	original := "# My Project\n\nExisting instructions.\n"
	if err := os.WriteFile(claudePath, []byte(original), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := setupClaudeMD(dir); err != nil {
		t.Fatalf("setupClaudeMD failed: %v", err)
	}

	data, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	content := string(data)
	// Should have C4 overrides prepended
	if !containsSubstring(content, "## CRITICAL: C4 Overrides") {
		t.Error("C4 Overrides not prepended")
	}
	// Original content should be preserved
	if !containsSubstring(content, "Existing instructions.") {
		t.Error("original content lost")
	}
	// C4 section should come before original
	if !containsSubstring(content, "Original CLAUDE.md content below") {
		t.Error("missing separator comment")
	}
}

func TestSetupClaudeMD_SkipsAGENTSSymlink(t *testing.T) {
	dir := t.TempDir()
	agentsPath := filepath.Join(dir, "AGENTS.md")
	claudePath := filepath.Join(dir, "CLAUDE.md")

	// Create AGENTS.md and symlink CLAUDE.md → AGENTS.md
	if err := os.WriteFile(agentsPath, []byte("# AGENTS\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Symlink("AGENTS.md", claudePath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := setupClaudeMD(dir); err != nil {
		t.Fatalf("setupClaudeMD failed: %v", err)
	}

	// Should not modify the symlink target
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "# AGENTS\n" {
		t.Error("AGENTS.md was modified (should be skipped for symlink)")
	}
}

func TestSetupSkills_DeploysSymlinks(t *testing.T) {
	// Create a fake C4 root with skills
	c4Root := t.TempDir()
	skillDir := filepath.Join(c4Root, ".claude", "skills", "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte("# Test Skill\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Set env for findC4Root
	t.Setenv("C4_SOURCE_ROOT", c4Root)

	// Target project
	targetDir := t.TempDir()

	if err := setupSkills(targetDir); err != nil {
		t.Fatalf("setupSkills failed: %v", err)
	}

	// Verify symlink was created
	targetSkill := filepath.Join(targetDir, ".claude", "skills", "test-skill")
	info, err := os.Lstat(targetSkill)
	if err != nil {
		t.Fatalf("skill symlink not created: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink, got regular file/dir")
	}

	// Verify symlink target
	target, err := os.Readlink(targetSkill)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != skillDir {
		t.Errorf("symlink target = %q, want %q", target, skillDir)
	}

	// Verify SKILL.md is accessible through symlink
	data, err := os.ReadFile(filepath.Join(targetSkill, "SKILL.md"))
	if err != nil {
		t.Fatalf("read through symlink: %v", err)
	}
	if string(data) != "# Test Skill\n" {
		t.Error("skill content mismatch through symlink")
	}
}

func TestSetupSkills_SkipsExisting(t *testing.T) {
	c4Root := t.TempDir()
	skillDir := filepath.Join(c4Root, ".claude", "skills", "existing-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	t.Setenv("C4_SOURCE_ROOT", c4Root)

	targetDir := t.TempDir()
	targetSkill := filepath.Join(targetDir, ".claude", "skills", "existing-skill")
	if err := os.MkdirAll(targetSkill, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write a local file to verify it's not replaced
	localFile := filepath.Join(targetSkill, "local.txt")
	if err := os.WriteFile(localFile, []byte("local"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := setupSkills(targetDir); err != nil {
		t.Fatalf("setupSkills failed: %v", err)
	}

	// Verify local file still exists (not replaced by symlink)
	data, err := os.ReadFile(localFile)
	if err != nil {
		t.Fatalf("local file lost: %v", err)
	}
	if string(data) != "local" {
		t.Error("local file content changed")
	}
}

func TestSetupSkills_NoC4Root(t *testing.T) {
	t.Setenv("C4_SOURCE_ROOT", "")
	t.Setenv("HOME", t.TempDir()) // Prevent reading real ~/.c4-install-path

	// Save and clear builtinC4Root
	oldRoot := builtinC4Root
	builtinC4Root = ""
	defer func() { builtinC4Root = oldRoot }()

	targetDir := t.TempDir()

	// Should not fail — just skip gracefully
	if err := setupSkills(targetDir); err != nil {
		t.Fatalf("setupSkills should not fail: %v", err)
	}

	// .claude/skills/ should not be created
	_, err := os.Stat(filepath.Join(targetDir, ".claude", "skills"))
	if err == nil {
		t.Error(".claude/skills/ should not be created when C4 root not found")
	}
}

func TestFindC4Root_EnvVar(t *testing.T) {
	c4Root := t.TempDir()
	skillDir := filepath.Join(c4Root, ".claude", "skills")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	t.Setenv("C4_SOURCE_ROOT", c4Root)

	root, err := findC4Root()
	if err != nil {
		t.Fatalf("findC4Root failed: %v", err)
	}
	if root != c4Root {
		t.Errorf("root = %q, want %q", root, c4Root)
	}
}

func TestFindC4Root_BuiltinVar(t *testing.T) {
	c4Root := t.TempDir()
	skillDir := filepath.Join(c4Root, ".claude", "skills")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	t.Setenv("C4_SOURCE_ROOT", "")

	oldRoot := builtinC4Root
	builtinC4Root = c4Root
	defer func() { builtinC4Root = oldRoot }()

	root, err := findC4Root()
	if err != nil {
		t.Fatalf("findC4Root failed: %v", err)
	}
	if root != c4Root {
		t.Errorf("root = %q, want %q", root, c4Root)
	}
}

func TestFindC4Root_InstallPathFile(t *testing.T) {
	c4Root := t.TempDir()
	skillDir := filepath.Join(c4Root, ".claude", "skills")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	t.Setenv("C4_SOURCE_ROOT", "")
	oldRoot := builtinC4Root
	builtinC4Root = ""
	defer func() { builtinC4Root = oldRoot }()

	// Create temporary home with .c4-install-path
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	installPathFile := filepath.Join(tmpHome, ".c4-install-path")
	if err := os.WriteFile(installPathFile, []byte(c4Root+"\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	root, err := findC4Root()
	if err != nil {
		t.Fatalf("findC4Root failed: %v", err)
	}
	if root != c4Root {
		t.Errorf("root = %q, want %q", root, c4Root)
	}
}

func TestHasSkills(t *testing.T) {
	dir := t.TempDir()

	// No .claude/skills/ → false
	if hasSkills(dir) {
		t.Error("hasSkills should return false for empty dir")
	}

	// Create .claude/skills/ → true
	if err := os.MkdirAll(filepath.Join(dir, ".claude", "skills"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if !hasSkills(dir) {
		t.Error("hasSkills should return true when .claude/skills/ exists")
	}
}

func TestSetupCodexConfig_NewFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	t.Setenv("CODEX_CONFIG", configPath)

	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := setupCodexConfig(projectDir); err != nil {
		t.Fatalf("setupCodexConfig failed: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config not created: %v", err)
	}
	content := string(data)

	if !containsSubstring(content, "[mcp_servers.cq]") {
		t.Error("missing [mcp_servers.cq] block")
	}
	if !containsSubstring(content, projectDir) {
		t.Error("project dir not reflected in codex config")
	}
}

func TestSetupCodexConfig_ReplaceExistingBlock(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	t.Setenv("CODEX_CONFIG", configPath)

	original := strings.Join([]string{
		"[general]",
		`theme = "light"`,
		"",
		"[mcp_servers.cq]",
		`command = "/old/cq"`,
		`args = ["mcp", "--dir", "/old/project"]`,
		`env = { C4_PROJECT_ROOT = "/old/project" }`,
		"",
		"[other]",
		`value = "ok"`,
		"",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(original), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	projectDir := filepath.Join(dir, "new-project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := setupCodexConfig(projectDir); err != nil {
		t.Fatalf("setupCodexConfig failed: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	content := string(data)

	if strings.Count(content, "[mcp_servers.cq]") != 1 {
		t.Fatalf("expected exactly one cq mcp block, got %d", strings.Count(content, "[mcp_servers.cq]"))
	}
	if containsSubstring(content, "/old/project") {
		t.Error("old cq block content still present")
	}
	if !containsSubstring(content, "[general]") || !containsSubstring(content, "[other]") {
		t.Error("non-cq blocks should be preserved")
	}
}

func TestSetupCodexAgents_DeploysSymlinks(t *testing.T) {
	c4Root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(c4Root, ".claude", "skills"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sourceAgentsDir := filepath.Join(c4Root, ".codex", "agents")
	if err := os.MkdirAll(sourceAgentsDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	sourceAgent := filepath.Join(sourceAgentsDir, "c4-run.md")
	if err := os.WriteFile(sourceAgent, []byte("# c4-run\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Non-c4 files are intentionally ignored.
	if err := os.WriteFile(filepath.Join(sourceAgentsDir, "README.md"), []byte("# docs\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	t.Setenv("C4_SOURCE_ROOT", c4Root)

	targetDir := t.TempDir()
	if err := setupCodexAgents(targetDir); err != nil {
		t.Fatalf("setupCodexAgents failed: %v", err)
	}

	targetAgent := filepath.Join(targetDir, ".codex", "agents", "c4-run.md")
	info, err := os.Lstat(targetAgent)
	if err != nil {
		t.Fatalf("agent symlink not created: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected codex agent symlink")
	}
	linkTarget, err := os.Readlink(targetAgent)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if linkTarget != sourceAgent {
		t.Errorf("symlink target = %q, want %q", linkTarget, sourceAgent)
	}

	_, err = os.Stat(filepath.Join(targetDir, ".codex", "agents", "README.md"))
	if err == nil {
		t.Error("README.md should not be deployed by setupCodexAgents")
	}
}

func TestSetupMCPConfig_ServerKeyCQ(t *testing.T) {
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, ".mcp.json")

	// setupMCPConfig uses os.Executable() to find the binary; override with PATH fallback.
	// We just verify the server key written to .mcp.json is "cq", not "c4".
	if err := setupMCPConfig(dir); err != nil {
		// Binary lookup may fail in test environment — skip rather than fail.
		t.Skipf("setupMCPConfig: %v", err)
	}

	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf(".mcp.json not created: %v", err)
	}
	content := string(data)

	if !containsSubstring(content, `"cq"`) {
		t.Errorf(".mcp.json missing \"cq\" server key; got:\n%s", content)
	}
	if containsSubstring(content, `"c4"`) {
		t.Errorf(".mcp.json has stale \"c4\" server key (should be \"cq\"); got:\n%s", content)
	}
}

func TestSetupCursorMCPConfig_ServerKeyCQ(t *testing.T) {
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, ".cursor", "mcp.json")

	if err := setupCursorMCPConfig(dir); err != nil {
		t.Skipf("setupCursorMCPConfig: %v", err)
	}

	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf(".cursor/mcp.json not created: %v", err)
	}
	content := string(data)

	if !containsSubstring(content, `"cq"`) {
		t.Errorf(".cursor/mcp.json missing \"cq\" server key; got:\n%s", content)
	}
	if containsSubstring(content, `"c4"`) {
		t.Errorf(".cursor/mcp.json has stale \"c4\" server key (should be \"cq\"); got:\n%s", content)
	}
}
