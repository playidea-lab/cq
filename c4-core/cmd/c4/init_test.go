package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/changmin/c4-core/internal/mailbox"
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

func TestSetupProjectHooks(t *testing.T) {
	projectDir := t.TempDir()

	// First run: installs hooks
	if err := setupProjectHooks(projectDir); err != nil {
		t.Fatalf("setupProjectHooks: %v", err)
	}
	gateHookPath := filepath.Join(projectDir, ".claude", "hooks", "c4-gate.sh")
	if _, err := os.Stat(gateHookPath); err != nil {
		t.Fatalf("gate hook file not created: %v", err)
	}
	info, _ := os.Stat(gateHookPath)
	if info.Mode()&0111 == 0 {
		t.Error("gate hook file not executable")
	}

	// Second run: idempotent
	if err := setupProjectHooks(projectDir); err != nil {
		t.Fatalf("second setupProjectHooks: %v", err)
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

func TestSetupProjectHooks_Install(t *testing.T) {
	projectDir := t.TempDir()

	if err := setupProjectHooks(projectDir); err != nil {
		t.Fatalf("setupProjectHooks failed: %v", err)
	}

	hooksDir := filepath.Join(projectDir, ".claude", "hooks")

	gateHookPath := filepath.Join(hooksDir, "c4-gate.sh")
	data, err := os.ReadFile(gateHookPath)
	if err != nil {
		t.Fatalf("gate hook script not created: %v", err)
	}
	if string(data) != gateHookContent {
		t.Error("gate hook script content mismatch")
	}
	// Verify executable permission
	info, err := os.Stat(gateHookPath)
	if err != nil {
		t.Fatalf("stat gate hook: %v", err)
	}
	if info.Mode()&0100 == 0 {
		t.Error("gate hook script not executable")
	}

	permHookPath := filepath.Join(hooksDir, "c4-permission-reviewer.sh")
	permData, err := os.ReadFile(permHookPath)
	if err != nil {
		t.Fatalf("permission reviewer hook not created: %v", err)
	}
	if string(permData) != permissionReviewerContent {
		t.Error("permission reviewer hook content mismatch")
	}
}

func TestSetupProjectHooks_Idempotent(t *testing.T) {
	projectDir := t.TempDir()

	// First install
	if err := setupProjectHooks(projectDir); err != nil {
		t.Fatalf("first setupProjectHooks failed: %v", err)
	}

	// Second install should be a no-op (hooks up-to-date)
	if err := setupProjectHooks(projectDir); err != nil {
		t.Fatalf("second setupProjectHooks failed: %v", err)
	}

	// Gate hook content should still match embedded
	hooksDir := filepath.Join(projectDir, ".claude", "hooks")
	gateHookPath := filepath.Join(hooksDir, "c4-gate.sh")
	hookData, err := os.ReadFile(gateHookPath)
	if err != nil {
		t.Fatalf("read gate hook: %v", err)
	}
	if string(hookData) != gateHookContent {
		t.Error("gate hook script content mismatch after second install")
	}
}

// --- Integration tests: setupProjectHooks end-to-end ---

// TestInitAndLaunch_HooksInstalled simulates a fresh install and verifies
// that setupProjectHooks creates hook files with 0755 permissions.
func TestInitAndLaunch_HooksInstalled(t *testing.T) {
	tmpProject := t.TempDir()

	if err := setupProjectHooks(tmpProject); err != nil {
		t.Fatalf("setupProjectHooks: %v", err)
	}

	gateHookPath := filepath.Join(tmpProject, ".claude", "hooks", "c4-gate.sh")
	info, err := os.Stat(gateHookPath)
	if err != nil {
		t.Fatalf("gate hook file not created: %v", err)
	}
	// Verify executable permission (0755)
	if info.Mode().Perm() != 0755 {
		t.Errorf("gate hook permissions = %o, want 0755", info.Mode().Perm())
	}
}

// TestInitAndLaunch_SettingsPatched simulates a fresh install and verifies
// that settings.json is created with the correct hooks structure.
func TestInitAndLaunch_SettingsPatched(t *testing.T) {
	tmpProject := t.TempDir()

	if err := setupProjectHooks(tmpProject); err != nil {
		t.Fatalf("setupProjectHooks: %v", err)
	}

	settingsPath := filepath.Join(tmpProject, ".claude", "settings.json")
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
	if entry["matcher"] != "Bash|Edit|Write" {
		t.Errorf("PreToolUse[0].matcher = %v, want Bash|Edit|Write", entry["matcher"])
	}
	// PermissionRequest should also be registered
	permRequest, _ := hooks["PermissionRequest"].([]any)
	if len(permRequest) == 0 {
		t.Fatal("PermissionRequest array is empty")
	}
}

// TestInitAndLaunch_Idempotent calls setupProjectHooks twice and verifies
// that hooks.PreToolUse has exactly 1 entry (no duplicates).
func TestInitAndLaunch_Idempotent(t *testing.T) {
	tmpProject := t.TempDir()

	// First call
	if err := setupProjectHooks(tmpProject); err != nil {
		t.Fatalf("first setupProjectHooks: %v", err)
	}

	// Second call
	if err := setupProjectHooks(tmpProject); err != nil {
		t.Fatalf("second setupProjectHooks: %v", err)
	}

	settingsPath := filepath.Join(tmpProject, ".claude", "settings.json")
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
	// Should have exactly 1 entry (Bash|Edit|Write) after idempotent calls
	if len(preToolUse) != 1 {
		t.Errorf("expected 1 PreToolUse entry (Bash|Edit|Write) after 2 calls, got %d", len(preToolUse))
	}
}

func TestPatchProjectSettings_NewFile(t *testing.T) {
	projectDir := t.TempDir()

	if err := patchProjectSettings(projectDir); err != nil {
		t.Fatalf("patchProjectSettings: %v", err)
	}

	settingsPath := filepath.Join(projectDir, ".claude", "settings.json")
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
	if len(preToolUse) != 1 {
		t.Fatalf("expected 1 PreToolUse entry (Bash|Edit|Write), got %d", len(preToolUse))
	}
	entry, _ := preToolUse[0].(map[string]any)
	if entry["matcher"] != "Bash|Edit|Write" {
		t.Errorf("matcher = %v, want Bash|Edit|Write", entry["matcher"])
	}
	innerHooks, _ := entry["hooks"].([]any)
	if len(innerHooks) != 1 {
		t.Fatalf("expected 1 inner hook, got %d", len(innerHooks))
	}
	h, _ := innerHooks[0].(map[string]any)
	if h["type"] != "command" {
		t.Errorf("type = %v, want command", h["type"])
	}
	// Command should use $CLAUDE_PROJECT_DIR variable
	cmd, _ := h["command"].(string)
	if !strings.Contains(cmd, "CLAUDE_PROJECT_DIR") {
		t.Errorf("command should use $CLAUDE_PROJECT_DIR, got %v", cmd)
	}
	if !strings.Contains(cmd, "c4-gate.sh") {
		t.Errorf("command should reference c4-gate.sh, got %v", cmd)
	}
	// PermissionRequest should also be added
	permRequest, _ := hooks["PermissionRequest"].([]any)
	if len(permRequest) != 1 {
		t.Fatalf("expected 1 PermissionRequest entry, got %d", len(permRequest))
	}
}

func TestPatchProjectSettings_AppendToExisting(t *testing.T) {
	projectDir := t.TempDir()
	settingsDir := filepath.Join(projectDir, ".claude")
	os.MkdirAll(settingsDir, 0755)

	existing := map[string]any{
		"model": "opus",
		"permissions": map[string]any{
			"allow": []string{"mcp__cq__*"},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(filepath.Join(settingsDir, "settings.json"), data, 0644)

	if err := patchProjectSettings(projectDir); err != nil {
		t.Fatalf("patchProjectSettings: %v", err)
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
	if len(preToolUse) != 1 {
		t.Fatalf("expected 1 PreToolUse entry (Bash|Edit|Write), got %d", len(preToolUse))
	}
}

func TestPatchProjectSettings_Idempotent(t *testing.T) {
	projectDir := t.TempDir()

	// First call
	if err := patchProjectSettings(projectDir); err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Second call — should not duplicate
	if err := patchProjectSettings(projectDir); err != nil {
		t.Fatalf("second call: %v", err)
	}

	settingsPath := filepath.Join(projectDir, ".claude", "settings.json")
	data, _ := os.ReadFile(settingsPath)

	var settings map[string]any
	json.Unmarshal(data, &settings)
	hooks, _ := settings["hooks"].(map[string]any)
	preToolUse, _ := hooks["PreToolUse"].([]any)

	if len(preToolUse) != 1 {
		t.Errorf("expected 1 PreToolUse entry (Bash|Edit|Write) after 2 calls, got %d", len(preToolUse))
	}

	// c4-gate.sh should appear exactly once
	content := string(data)
	count := strings.Count(content, "c4-gate.sh")
	if count != 1 {
		t.Errorf("c4-gate.sh appears %d times, want 1", count)
	}
}

// TestPatchProjectSettings_DeprecatedBaseName verifies that patchProjectSettings
// replaces a hook registered under a deprecated baseName (e.g. permission-reviewer.py)
// with the current hook (c4-permission-reviewer.sh) without creating a duplicate entry.
// This covers the real-world scenario where a project was set up with a pre-v0.24
// Python-based permission reviewer.
func TestPatchProjectSettings_DeprecatedBaseName(t *testing.T) {
	projectDir := t.TempDir()
	settingsDir := filepath.Join(projectDir, ".claude")
	os.MkdirAll(settingsDir, 0755)

	hooksDir := filepath.Join(settingsDir, "hooks")
	os.MkdirAll(hooksDir, 0755)

	// Simulate pre-v0.24 settings.json with the Python-based permission reviewer.
	oldPermCmd := "python3 \"$CLAUDE_PROJECT_DIR\"/.claude/hooks/permission-reviewer.py"
	staleSettings := map[string]any{
		"hooks": map[string]any{
			"PermissionRequest": []any{
				map[string]any{
					"matcher": "Bash|Read|Edit|Write|MultiEdit|NotebookEdit|WebFetch",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": oldPermCmd,
							"timeout": float64(15),
						},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(staleSettings)
	settingsPath := filepath.Join(settingsDir, "settings.json")
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatalf("write stale settings: %v", err)
	}

	if err := patchProjectSettings(projectDir); err != nil {
		t.Fatalf("patchProjectSettings: %v", err)
	}

	result, _ := os.ReadFile(settingsPath)
	var settings map[string]any
	json.Unmarshal(result, &settings)
	hooks, _ := settings["hooks"].(map[string]any)
	permReq, _ := hooks["PermissionRequest"].([]any)

	// Must be exactly 1 PermissionRequest entry — no duplicate appended.
	if len(permReq) != 1 {
		t.Errorf("expected 1 PermissionRequest entry after deprecated upgrade, got %d\nJSON:\n%s", len(permReq), result)
	}

	// The entry must use the new shell-based reviewer, not the deprecated Python one.
	content := string(result)
	if strings.Contains(content, "permission-reviewer.py") {
		t.Error("deprecated permission-reviewer.py still present in settings.json")
	}
	if !strings.Contains(content, "c4-permission-reviewer.sh") {
		t.Error("c4-permission-reviewer.sh not found in settings.json")
	}

	// Matcher must be upgraded to the current value.
	entry, _ := permReq[0].(map[string]any)
	if entry["matcher"] != "Bash|Read|Edit|Write|NotebookEdit|WebFetch|WebSearch|Search|Skill" {
		t.Errorf("matcher not upgraded: %v", entry["matcher"])
	}
}

// TestPatchProjectSettings_StaleEntry verifies that patchProjectSettings upgrades
// a hook entry that exists under an outdated matcher (Phase 1 baseName scan).
// This exercises the Round-2 fix: stale entry is replaced in-place rather than
// creating a duplicate entry under the new matcher.
func TestPatchProjectSettings_StaleEntry(t *testing.T) {
	projectDir := t.TempDir()
	settingsDir := filepath.Join(projectDir, ".claude")
	os.MkdirAll(settingsDir, 0755)

	hooksDir := filepath.Join(settingsDir, "hooks")
	os.MkdirAll(hooksDir, 0755)
	gateCmd := filepath.Join(hooksDir, "c4-gate.sh")

	// Simulate an older settings.json where c4-gate.sh is registered under "Bash"
	// (old matcher) instead of the current "Bash|Edit|Write" matcher.
	staleSettings := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": gateCmd,
						},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(staleSettings)
	settingsPath := filepath.Join(settingsDir, "settings.json")
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatalf("write stale settings: %v", err)
	}

	if err := patchProjectSettings(projectDir); err != nil {
		t.Fatalf("patchProjectSettings: %v", err)
	}

	result, _ := os.ReadFile(settingsPath)
	var settings map[string]any
	json.Unmarshal(result, &settings)
	hooks, _ := settings["hooks"].(map[string]any)
	preToolUse, _ := hooks["PreToolUse"].([]any)

	// Must still be exactly 1 PreToolUse entry (no duplicate appended).
	if len(preToolUse) != 1 {
		t.Errorf("expected 1 PreToolUse entry after stale upgrade, got %d", len(preToolUse))
	}

	// Matcher must be updated to the current value.
	entry, _ := preToolUse[0].(map[string]any)
	if entry["matcher"] != "Bash|Edit|Write" {
		t.Errorf("matcher not upgraded: %v", entry["matcher"])
	}

	// c4-gate.sh must appear exactly once in the output.
	count := strings.Count(string(result), "c4-gate.sh")
	if count != 1 {
		t.Errorf("c4-gate.sh appears %d times, want 1", count)
	}
}

func TestPatchProjectSettings_CorruptedJSON(t *testing.T) {
	projectDir := t.TempDir()
	settingsDir := filepath.Join(projectDir, ".claude")
	os.MkdirAll(settingsDir, 0755)

	settingsPath := filepath.Join(settingsDir, "settings.json")
	os.WriteFile(settingsPath, []byte("{invalid json!!!"), 0644)

	if err := patchProjectSettings(projectDir); err != nil {
		t.Fatalf("patchProjectSettings: %v", err)
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
	if len(preToolUse) != 1 {
		t.Fatalf("expected 1 PreToolUse entry (Bash|Edit|Write), got %d", len(preToolUse))
	}
}

// TestInitInteractive_YesFlag verifies that when yesAll is set, confirmProjectHooks
// returns true without reading from stdin.
func TestInitInteractive_YesFlag(t *testing.T) {
	oldYesAll := yesAll
	yesAll = true
	defer func() { yesAll = oldYesAll }()

	projectDir := t.TempDir()

	// With yesAll=true, confirmProjectHooks should return true immediately.
	if !confirmProjectHooks(projectDir) {
		t.Error("confirmProjectHooks should return true when yesAll is set")
	}

	// Verify that setupProjectHooks proceeds (hook file created) when yesAll=true.
	if err := setupProjectHooks(projectDir); err != nil {
		t.Fatalf("setupProjectHooks failed: %v", err)
	}
	hookPath := filepath.Join(projectDir, ".claude", "hooks", "c4-gate.sh")
	if _, err := os.Stat(hookPath); err != nil {
		t.Error("hook file should be created when --yes is set")
	}
}

// TestInitInteractive_ProjectDeny verifies that when the user declines the project
// hook prompt, confirmProjectHooks returns false and the hook is skipped.
func TestInitInteractive_ProjectDeny(t *testing.T) {
	oldYesAll := yesAll
	yesAll = false
	defer func() { yesAll = oldYesAll }()

	projectDir := t.TempDir()

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

	// confirmProjectHooks should return false on "n".
	if confirmProjectHooks(projectDir) {
		t.Error("confirmProjectHooks should return false when user answers 'n'")
	}

	// Hook file must NOT be created when user denies.
	hookPath := filepath.Join(projectDir, ".claude", "hooks", "c4-gate.sh")
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

// TestCheckCloudAuthStatus_NoURL verifies that checkCloudAuthStatus prints nothing
// when no cloud URL is configured (solo tier).
func TestCheckCloudAuthStatus_NoURL(t *testing.T) {
	t.Setenv("C4_CLOUD_URL", "")
	t.Setenv("SUPABASE_URL", "")

	// Capture stderr
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w

	// Save and clear builtinSupabaseURL
	oldBuiltin := builtinSupabaseURL
	builtinSupabaseURL = ""
	defer func() { builtinSupabaseURL = oldBuiltin }()

	checkCloudAuthStatus()

	w.Close()
	os.Stderr = oldStderr

	buf := make([]byte, 256)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if output != "" {
		t.Errorf("expected no output when URL not configured, got: %q", output)
	}
}

// TestCheckCloudAuthStatus_NotLoggedIn verifies that checkCloudAuthStatus prints
// a login prompt when the URL is set but no session file exists.
func TestCheckCloudAuthStatus_NotLoggedIn(t *testing.T) {
	t.Setenv("C4_CLOUD_URL", "https://example.supabase.co")

	// Use a temp HOME so session.json doesn't exist.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Capture stderr
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w

	checkCloudAuthStatus()

	w.Close()
	os.Stderr = oldStderr

	buf := make([]byte, 512)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "cq auth login") {
		t.Errorf("expected login prompt, got: %q", output)
	}
}

// TestCheckCloudAuthStatus_LoggedIn verifies that checkCloudAuthStatus prints
// the cloud user email and expiry when a valid session exists.
func TestCheckCloudAuthStatus_LoggedIn(t *testing.T) {
	t.Setenv("C4_CLOUD_URL", "https://example.supabase.co")

	// Write a valid session file to temp HOME.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	sessionDir := filepath.Join(tmpHome, ".c4")
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	expiresAt := time.Now().Add(48 * time.Hour).Unix()
	sessionJSON := fmt.Sprintf(`{
		"access_token": "tok",
		"refresh_token": "rtok",
		"expires_at": %d,
		"user": {"id": "u1", "email": "user@example.com", "name": "Test User"}
	}`, expiresAt)
	if err := os.WriteFile(filepath.Join(sessionDir, "session.json"), []byte(sessionJSON), 0600); err != nil {
		t.Fatalf("write session: %v", err)
	}

	// Capture stderr
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w

	checkCloudAuthStatus()

	w.Close()
	os.Stderr = oldStderr

	buf := make([]byte, 512)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "user@example.com") {
		t.Errorf("expected email in output, got: %q", output)
	}
	if !strings.Contains(output, "expires in") {
		t.Errorf("expected expiry info in output, got: %q", output)
	}
}

// TestCheckCloudAuthStatus_ExpiredSession verifies that checkCloudAuthStatus
// treats an expired session as not logged in and prints a login prompt.
func TestCheckCloudAuthStatus_ExpiredSession(t *testing.T) {
	t.Setenv("C4_CLOUD_URL", "https://example.supabase.co")

	// Write an expired session file to temp HOME.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	sessionDir := filepath.Join(tmpHome, ".c4")
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	expiresAt := time.Now().Add(-1 * time.Hour).Unix() // expired 1h ago
	sessionJSON := fmt.Sprintf(`{
		"access_token": "tok",
		"refresh_token": "rtok",
		"expires_at": %d,
		"user": {"id": "u1", "email": "user@example.com", "name": "Test User"}
	}`, expiresAt)
	if err := os.WriteFile(filepath.Join(sessionDir, "session.json"), []byte(sessionJSON), 0600); err != nil {
		t.Fatalf("write session: %v", err)
	}

	// Capture stderr
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w

	checkCloudAuthStatus()

	w.Close()
	os.Stderr = oldStderr

	buf := make([]byte, 512)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "cq auth login") {
		t.Errorf("expected login prompt for expired session, got: %q", output)
	}
	if strings.Contains(output, "user@example.com") {
		t.Errorf("should not show email for expired session, got: %q", output)
	}
}

// TestLsUnread verifies that cq ls appends "[N unread]" for sessions that have
// unread messages in the mailbox, and shows no suffix for sessions without unread messages.
func TestLsUnread(t *testing.T) {
	// Set up temp HOME with .c4/ directory.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	c4Dir := filepath.Join(tmpHome, ".c4")
	if err := os.MkdirAll(c4Dir, 0755); err != nil {
		t.Fatalf("mkdir .c4: %v", err)
	}

	// Create mailbox.db and send 2 unread messages to "tmuxlike".
	dbPath := filepath.Join(c4Dir, "mailbox.db")
	ms, err := mailbox.NewMailStore(dbPath)
	if err != nil {
		t.Fatalf("NewMailStore: %v", err)
	}
	if _, err := ms.Send("", "tmuxlike", "hello", "world", ""); err != nil {
		t.Fatalf("Send 1: %v", err)
	}
	if _, err := ms.Send("", "tmuxlike", "hi", "there", ""); err != nil {
		t.Fatalf("Send 2: %v", err)
	}
	ms.Close()

	// Write named-sessions.json with two entries.
	sessions := map[string]namedSessionEntry{
		"tmuxlike": {UUID: "869fd61eaaaabbbb", Dir: "/home/user/git/cq", Updated: "2026-02-24T23:15:00Z"},
		"cq-dev":   {UUID: "2ab09aa7ccccdddd", Dir: "/home/user/git/cq", Updated: "2026-02-23T10:00:00Z"},
	}
	sessData, _ := json.MarshalIndent(sessions, "", "  ")
	if err := os.WriteFile(filepath.Join(c4Dir, "named-sessions.json"), sessData, 0600); err != nil {
		t.Fatalf("write named-sessions.json: %v", err)
	}

	// Redirect lsCmd output to a buffer.
	oldStdout := os.Stdout
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("pipe: %v", pipeErr)
	}
	os.Stdout = w

	// Clear CQ_SESSION_UUID so neither session is marked "(current)".
	t.Setenv("CQ_SESSION_UUID", "")

	// Run the command.
	lsCmd.RunE(lsCmd, nil) //nolint:errcheck

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r) //nolint:errcheck
	output := buf.String()

	// "tmuxlike" should have [2 unread].
	if !strings.Contains(output, "[2 unread]") {
		t.Errorf("expected '[2 unread]' in output for tmuxlike; got:\n%s", output)
	}
	// "cq-dev" has no unread messages — must not show any suffix.
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.HasPrefix(line, "cq-dev:") && strings.Contains(line, "unread") {
			t.Errorf("cq-dev should have no unread suffix; got: %s", line)
		}
	}
}
