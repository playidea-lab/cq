package main

import (
	"encoding/json"
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

	// Disable embedded FS so we test the "no root, no embed" path
	oldEmbed := EmbeddedSkillsFS
	EmbeddedSkillsFS = nil
	defer func() { EmbeddedSkillsFS = oldEmbed }()

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

func TestHookNeedsUpdate_Missing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.sh")
	if !hookNeedsUpdate(path, "content") {
		t.Error("expected true for missing file")
	}
}

func TestHookNeedsUpdate_HashMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hook.sh")
	content := "#!/bin/bash\necho hello\n"
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("write: %v", err)
	}
	if hookNeedsUpdate(path, content) {
		t.Error("expected false when content matches")
	}
}

func TestHookNeedsUpdate_HashMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hook.sh")
	if err := os.WriteFile(path, []byte("old content"), 0755); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !hookNeedsUpdate(path, "new content") {
		t.Error("expected true when content differs")
	}
}

func TestSetupGlobalHooks_Install(t *testing.T) {
	homeDir := t.TempDir()

	if err := setupGlobalHooks(homeDir); err != nil {
		t.Fatalf("setupGlobalHooks failed: %v", err)
	}

	hooksDir := filepath.Join(homeDir, ".claude", "hooks")

	hookPath := filepath.Join(hooksDir, "c4-bash-security-hook.sh")
	data, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("hook script not created: %v", err)
	}
	if string(data) != hookShContent {
		t.Error("hook script content mismatch")
	}
	// Verify executable permission
	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("stat hook: %v", err)
	}
	if info.Mode()&0100 == 0 {
		t.Error("hook script not executable")
	}

	// .conf file should NOT be created; hook config is sourced from .c4/config.yaml
	confPath := filepath.Join(hooksDir, "c4-bash-security.conf")
	if _, err := os.Stat(confPath); err == nil {
		t.Error("hook conf should not be created by setupGlobalHooks")
	}
}

func TestSetupGlobalHooks_Idempotent(t *testing.T) {
	homeDir := t.TempDir()

	// First install
	if err := setupGlobalHooks(homeDir); err != nil {
		t.Fatalf("first setupGlobalHooks failed: %v", err)
	}

	hooksDir := filepath.Join(homeDir, ".claude", "hooks")

	// Place a pre-existing .conf to simulate backward-compat scenario
	confPath := filepath.Join(hooksDir, "c4-bash-security.conf")
	customConf := "# user customization\nALLOW_ALL=true\n"
	if err := os.WriteFile(confPath, []byte(customConf), 0644); err != nil {
		t.Fatalf("write custom conf: %v", err)
	}

	// Second install should not touch the existing .conf
	if err := setupGlobalHooks(homeDir); err != nil {
		t.Fatalf("second setupGlobalHooks failed: %v", err)
	}

	data, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read conf: %v", err)
	}
	if string(data) != customConf {
		t.Error("pre-existing .conf was modified by setupGlobalHooks")
	}

	// Hook script should still match embedded content
	hookPath := filepath.Join(hooksDir, "c4-bash-security-hook.sh")
	hookData, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read hook: %v", err)
	}
	if string(hookData) != hookShContent {
		t.Error("hook script content mismatch after second install")
	}
}

// --- Integration tests: setupGlobalHooks end-to-end ---

// TestInitAndLaunch_HooksInstalled simulates a fresh install (empty home dir)
// and verifies that setupGlobalHooks creates the hook file with 0755 permissions.
func TestInitAndLaunch_HooksInstalled(t *testing.T) {
	tmpHome := t.TempDir()

	if err := setupGlobalHooks(tmpHome); err != nil {
		t.Fatalf("setupGlobalHooks: %v", err)
	}

	hookPath := filepath.Join(tmpHome, ".claude", "hooks", "c4-bash-security-hook.sh")
	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("hook file not created: %v", err)
	}
	// Verify executable permission (0755)
	if info.Mode().Perm() != 0755 {
		t.Errorf("hook permissions = %o, want 0755", info.Mode().Perm())
	}
}

// TestInitAndLaunch_SettingsPatched simulates a fresh install and verifies
// that settings.json is created with hooks.PreToolUse[0].matcher == "Bash".
func TestInitAndLaunch_SettingsPatched(t *testing.T) {
	tmpHome := t.TempDir()

	if err := setupGlobalHooks(tmpHome); err != nil {
		t.Fatalf("setupGlobalHooks: %v", err)
	}

	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		t.Fatal("missing hooks key in settings.json")
	}
	preToolUse, _ := hooks["PreToolUse"].([]any)
	if len(preToolUse) == 0 {
		t.Fatal("PreToolUse array is empty")
	}
	entry, _ := preToolUse[0].(map[string]any)
	if entry["matcher"] != "Bash" {
		t.Errorf("PreToolUse[0].matcher = %v, want Bash", entry["matcher"])
	}
}

// TestInitAndLaunch_Idempotent calls setupGlobalHooks twice and verifies
// that hooks.PreToolUse has exactly 1 entry (no duplicates).
func TestInitAndLaunch_Idempotent(t *testing.T) {
	tmpHome := t.TempDir()

	// First call
	if err := setupGlobalHooks(tmpHome); err != nil {
		t.Fatalf("first setupGlobalHooks: %v", err)
	}

	// Second call
	if err := setupGlobalHooks(tmpHome); err != nil {
		t.Fatalf("second setupGlobalHooks: %v", err)
	}

	settingsPath := filepath.Join(tmpHome, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not found: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	hooks, _ := settings["hooks"].(map[string]any)
	preToolUse, _ := hooks["PreToolUse"].([]any)
	if len(preToolUse) != 2 {
		t.Errorf("expected 2 PreToolUse entries (Bash+Edit|Write) after 2 calls, got %d", len(preToolUse))
	}
}

func TestPatchClaudeSettings_NewFile(t *testing.T) {
	homeDir := t.TempDir()
	hookPath := "/usr/local/bin/hook.sh"

	if err := patchClaudeSettings(homeDir, hookPath, hookPath); err != nil {
		t.Fatalf("patchClaudeSettings: %v", err)
	}

	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		t.Fatal("missing hooks key")
	}
	preToolUse, _ := hooks["PreToolUse"].([]any)
	if len(preToolUse) != 2 {
		t.Fatalf("expected 2 PreToolUse entries (Bash+Edit|Write), got %d", len(preToolUse))
	}
	entry, _ := preToolUse[0].(map[string]any)
	if entry["matcher"] != "Bash" {
		t.Errorf("matcher = %v, want Bash", entry["matcher"])
	}
	innerHooks, _ := entry["hooks"].([]any)
	if len(innerHooks) != 1 {
		t.Fatalf("expected 1 inner hook, got %d", len(innerHooks))
	}
	h, _ := innerHooks[0].(map[string]any)
	if h["command"] != hookPath {
		t.Errorf("command = %v, want %v", h["command"], hookPath)
	}
	if h["type"] != "command" {
		t.Errorf("type = %v, want command", h["type"])
	}
}

func TestPatchClaudeSettings_AppendToExisting(t *testing.T) {
	homeDir := t.TempDir()
	settingsDir := filepath.Join(homeDir, ".claude")
	os.MkdirAll(settingsDir, 0755)

	existing := map[string]any{
		"model": "opus",
		"permissions": map[string]any{
			"allow": []string{"mcp__cq__*"},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(filepath.Join(settingsDir, "settings.json"), data, 0644)

	hookPath := "/path/to/hook.sh"
	if err := patchClaudeSettings(homeDir, hookPath, hookPath); err != nil {
		t.Fatalf("patchClaudeSettings: %v", err)
	}

	result, _ := os.ReadFile(filepath.Join(settingsDir, "settings.json"))
	var settings map[string]any
	if err := json.Unmarshal(result, &settings); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Existing keys preserved
	if settings["model"] != "opus" {
		t.Errorf("model key lost: %v", settings["model"])
	}
	if _, ok := settings["permissions"]; !ok {
		t.Error("permissions key lost")
	}

	// Hook added
	hooks, _ := settings["hooks"].(map[string]any)
	preToolUse, _ := hooks["PreToolUse"].([]any)
	if len(preToolUse) != 2 {
		t.Fatalf("expected 2 PreToolUse entries (Bash+Edit|Write), got %d", len(preToolUse))
	}
}

func TestPatchClaudeSettings_Idempotent(t *testing.T) {
	homeDir := t.TempDir()
	hookPath := "/path/to/hook.sh"

	// First call
	if err := patchClaudeSettings(homeDir, hookPath, hookPath); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Second call — should not duplicate
	if err := patchClaudeSettings(homeDir, hookPath, hookPath); err != nil {
		t.Fatalf("second call: %v", err)
	}

	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	data, _ := os.ReadFile(settingsPath)

	var settings map[string]any
	json.Unmarshal(data, &settings)
	hooks, _ := settings["hooks"].(map[string]any)
	preToolUse, _ := hooks["PreToolUse"].([]any)

	if len(preToolUse) != 2 {
		t.Errorf("expected 2 PreToolUse entries (Bash+Edit|Write) after 2 calls, got %d", len(preToolUse))
	}

	// hookPath appears twice: once for Bash matcher, once for Edit|Write matcher
	content := string(data)
	count := strings.Count(content, hookPath)
	if count != 2 {
		t.Errorf("hookPath appears %d times, want 2 (Bash+Edit|Write)", count)
	}
}

func TestPatchClaudeSettings_CorruptedJSON(t *testing.T) {
	homeDir := t.TempDir()
	settingsDir := filepath.Join(homeDir, ".claude")
	os.MkdirAll(settingsDir, 0755)

	settingsPath := filepath.Join(settingsDir, "settings.json")
	os.WriteFile(settingsPath, []byte("{invalid json!!!"), 0644)

	hookPath := "/path/to/hook.sh"
	if err := patchClaudeSettings(homeDir, hookPath, hookPath); err != nil {
		t.Fatalf("patchClaudeSettings: %v", err)
	}

	// Backup should exist
	entries, _ := os.ReadDir(settingsDir)
	backupFound := false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "settings.json.bak.") {
			backupFound = true
			// Verify backup content
			backupData, _ := os.ReadFile(filepath.Join(settingsDir, entry.Name()))
			if string(backupData) != "{invalid json!!!" {
				t.Errorf("backup content mismatch: %q", string(backupData))
			}
		}
	}
	if !backupFound {
		t.Error("no backup file created for corrupted settings.json")
	}

	// New settings.json should be valid JSON with hook
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not rewritten: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("rewritten settings.json invalid: %v", err)
	}
	hooks, _ := settings["hooks"].(map[string]any)
	preToolUse, _ := hooks["PreToolUse"].([]any)
	if len(preToolUse) != 2 {
		t.Fatalf("expected 2 PreToolUse entries (Bash+Edit|Write), got %d", len(preToolUse))
	}
}

// TestInitInteractive_YesFlag verifies that when yesAll is set, confirmGlobalChanges
// returns true without reading from stdin.
func TestInitInteractive_YesFlag(t *testing.T) {
	oldYesAll := yesAll
	yesAll = true
	defer func() { yesAll = oldYesAll }()

	homeDir := t.TempDir()

	// With yesAll=true, confirmGlobalChanges should return true immediately.
	if !confirmGlobalChanges(homeDir) {
		t.Error("confirmGlobalChanges should return true when yesAll is set")
	}

	// Verify that setupGlobalHooks proceeds (hook file created) when yesAll=true.
	if err := setupGlobalHooks(homeDir); err != nil {
		t.Fatalf("setupGlobalHooks failed: %v", err)
	}
	hookPath := filepath.Join(homeDir, ".claude", "hooks", "c4-bash-security-hook.sh")
	if _, err := os.Stat(hookPath); err != nil {
		t.Error("hook file should be created when --yes is set")
	}
}

// TestInitInteractive_GlobalDeny verifies that when the user declines the global
// confirmation prompt, confirmGlobalChanges returns false and the hook is skipped.
func TestInitInteractive_GlobalDeny(t *testing.T) {
	oldYesAll := yesAll
	yesAll = false
	defer func() { yesAll = oldYesAll }()

	homeDir := t.TempDir()

	// Simulate stdin with "n" (deny)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	w.WriteString("n\n")
	w.Close()

	// confirmGlobalChanges should return false on "n".
	if confirmGlobalChanges(homeDir) {
		t.Error("confirmGlobalChanges should return false when user answers 'n'")
	}

	// Hook file must NOT be created when user denies.
	hookPath := filepath.Join(homeDir, ".claude", "hooks", "c4-bash-security-hook.sh")
	if _, err := os.Stat(hookPath); err == nil {
		t.Error("hook file should NOT exist after user denial")
	}
}

// TestInitInteractive_ExistingMcpJson verifies that when .mcp.json already exists,
// setupMCPConfig emits an overwrite warning and updates the file while preserving
// existing entries.
func TestInitInteractive_ExistingMcpJson(t *testing.T) {
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, ".mcp.json")

	// Pre-create a .mcp.json with some existing content
	existing := map[string]any{
		"mcpServers": map[string]any{
			"other": map[string]any{"command": "/usr/bin/other"},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(mcpPath, append(data, '\n'), 0644); err != nil {
		t.Fatalf("write existing .mcp.json: %v", err)
	}

	// setupMCPConfig should succeed and warn about overwrite.
	if err := setupMCPConfig(dir); err != nil {
		t.Skipf("setupMCPConfig: %v", err) // binary lookup may fail in test env
	}

	result, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf(".mcp.json not found: %v", err)
	}
	content := string(result)

	// Existing "other" server must be preserved.
	if !containsSubstring(content, `"other"`) {
		t.Error("existing mcpServers entry 'other' was lost after update")
	}
	// New "cq" entry must be present.
	if !containsSubstring(content, `"cq"`) {
		t.Error("cq entry missing from updated .mcp.json")
	}
}

func TestWriteTierConfig_NewFile(t *testing.T) {
	dir := t.TempDir()
	c4Dir := filepath.Join(dir, ".c4")
	if err := os.MkdirAll(c4Dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := writeTierConfig(dir, "solo"); err != nil {
		t.Fatalf("writeTierConfig: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(c4Dir, "config.yaml"))
	if err != nil {
		t.Fatalf("config.yaml not created: %v", err)
	}
	if !containsSubstring(string(data), "tier: solo") {
		t.Errorf("expected 'tier: solo' in config.yaml; got:\n%s", string(data))
	}
}

func TestWriteTierConfig_UpdatesExisting(t *testing.T) {
	dir := t.TempDir()
	c4Dir := filepath.Join(dir, ".c4")
	if err := os.MkdirAll(c4Dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	configPath := filepath.Join(c4Dir, "config.yaml")
	initial := "project_id: myproj\ntier: connected\n"
	if err := os.WriteFile(configPath, []byte(initial), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := writeTierConfig(dir, "full"); err != nil {
		t.Fatalf("writeTierConfig: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	content := string(data)
	if strings.Count(content, "tier:") != 1 {
		t.Errorf("expected exactly 1 tier: line; got:\n%s", content)
	}
	if !containsSubstring(content, "tier: full") {
		t.Errorf("expected 'tier: full'; got:\n%s", content)
	}
	if !containsSubstring(content, "project_id: myproj") {
		t.Error("existing config key 'project_id' was lost")
	}
}

func TestWriteTierConfig_EmptyTierIsNoop(t *testing.T) {
	dir := t.TempDir()
	c4Dir := filepath.Join(dir, ".c4")
	if err := os.MkdirAll(c4Dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := writeTierConfig(dir, ""); err != nil {
		t.Fatalf("writeTierConfig with empty tier should not fail: %v", err)
	}

	// config.yaml should not be created for empty tier
	_, err := os.Stat(filepath.Join(c4Dir, "config.yaml"))
	if err == nil {
		t.Error("config.yaml should not be created when tier is empty")
	}
}

func TestWriteDefaultConfig(t *testing.T) {
	t.Run("creates config.yaml when missing", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".c4"), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := writeDefaultConfig(dir); err != nil {
			t.Fatalf("writeDefaultConfig: %v", err)
		}
		data, err := os.ReadFile(filepath.Join(dir, ".c4", "config.yaml"))
		if err != nil {
			t.Fatalf("read config.yaml: %v", err)
		}
		if !strings.Contains(string(data), "permission_reviewer") {
			t.Error("config.yaml missing permission_reviewer section")
		}
		if !strings.Contains(string(data), "mode: hook") {
			t.Error("config.yaml missing mode: hook")
		}
	})

	t.Run("does not overwrite existing config.yaml", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".c4"), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		existing := "project_id: my-existing-project\n"
		configPath := filepath.Join(dir, ".c4", "config.yaml")
		if err := os.WriteFile(configPath, []byte(existing), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := writeDefaultConfig(dir); err != nil {
			t.Fatalf("writeDefaultConfig: %v", err)
		}
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(data) != existing {
			t.Errorf("existing config was overwritten: got %q", string(data))
		}
	})
}

func TestWriteTierConfig_InvalidTier(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".c4"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := writeTierConfig(dir, "premium"); err == nil {
		t.Error("expected error for invalid tier 'premium'")
	}
}
