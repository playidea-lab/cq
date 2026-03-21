package botstore

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestStore creates a Store with a temp directory as both projectDir and
// overrides globalRoot to another temp dir so tests are fully isolated.
func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	projectDir := t.TempDir()
	s, err := New(projectDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Override globalRoot to a temp dir so tests don't touch ~/.claude/
	s.globalRoot = t.TempDir()
	return s, projectDir
}

func TestSaveAndGet(t *testing.T) {
	s, _ := newTestStore(t)

	bot := Bot{
		Username:    "mybot",
		Token:       "123:abc",
		DisplayName: "My Bot",
		LastActive:  time.Now().Round(time.Second),
		Scope:       "project",
	}
	if err := s.Save(bot); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Get("mybot")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Username != bot.Username {
		t.Errorf("Username: got %q, want %q", got.Username, bot.Username)
	}
	if got.Token != bot.Token {
		t.Errorf("Token: got %q, want %q", got.Token, bot.Token)
	}
	if got.DisplayName != bot.DisplayName {
		t.Errorf("DisplayName: got %q, want %q", got.DisplayName, bot.DisplayName)
	}
	if !got.LastActive.Equal(bot.LastActive) {
		t.Errorf("LastActive: got %v, want %v", got.LastActive, bot.LastActive)
	}
}

func TestGetNotFound(t *testing.T) {
	s, _ := newTestStore(t)
	_, err := s.Get("nonexistent")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestList_SortedByLastActive(t *testing.T) {
	s, _ := newTestStore(t)

	now := time.Now().Round(time.Second)
	bots := []Bot{
		{Username: "alpha", Token: "t1", LastActive: now.Add(-2 * time.Hour), Scope: "project"},
		{Username: "beta", Token: "t2", LastActive: now.Add(-1 * time.Hour), Scope: "project"},
		{Username: "gamma", Token: "t3", LastActive: now, Scope: "project"},
	}
	for _, b := range bots {
		if err := s.Save(b); err != nil {
			t.Fatalf("Save %s: %v", b.Username, err)
		}
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 bots, got %d", len(list))
	}
	// gamma (most recent) should be first
	if list[0].Username != "gamma" {
		t.Errorf("expected gamma first, got %s", list[0].Username)
	}
	if list[2].Username != "alpha" {
		t.Errorf("expected alpha last, got %s", list[2].Username)
	}
}

func TestList_ProjectTakePrecedenceOverGlobal(t *testing.T) {
	s, _ := newTestStore(t)

	// Save "sharedbot" to project root
	projBot := Bot{Username: "sharedbot", Token: "proj-token", Scope: "project"}
	if err := s.Save(projBot); err != nil {
		t.Fatalf("Save project bot: %v", err)
	}

	// Manually write a "sharedbot" entry to globalRoot with different token
	globalDir := filepath.Join(s.globalRoot, "sharedbot")
	if err := os.MkdirAll(globalDir, 0o700); err != nil {
		t.Fatalf("mkdir global: %v", err)
	}
	if err := writeJSON(filepath.Join(globalDir, "config.json"), configFile{
		Username: "sharedbot",
		Token:    "global-token",
	}); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 bot (deduplicated), got %d", len(list))
	}
	if list[0].Token != "proj-token" {
		t.Errorf("expected project token to win, got %q", list[0].Token)
	}
}

func TestRemove(t *testing.T) {
	s, _ := newTestStore(t)

	if err := s.Save(Bot{Username: "delme", Token: "t", Scope: "project"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Remove("delme"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := s.Get("delme"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after Remove, got %v", err)
	}
}

func TestRemoveNotFound(t *testing.T) {
	s, _ := newTestStore(t)
	if err := s.Remove("ghost"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestList_GlobalOnly(t *testing.T) {
	// Store with no projectDir
	s, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.globalRoot = t.TempDir()

	bot := Bot{Username: "globalbot", Token: "gt", Scope: "global"}
	if err := s.Save(bot); err != nil {
		t.Fatalf("Save: %v", err)
	}
	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Username != "globalbot" {
		t.Fatalf("unexpected list: %v", list)
	}
}
