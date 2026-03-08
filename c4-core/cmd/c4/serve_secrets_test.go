package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/secrets"
)

// mockSecretsSyncer implements secrets.CloudSyncer for testing.
type mockSecretsSyncer struct {
	synced []string
}

func (m *mockSecretsSyncer) SyncSecret(key, _ string) error {
	m.synced = append(m.synced, key)
	return nil
}

// newTestStore creates an in-memory (temp-file) secrets store for tests.
func newTestStore(t *testing.T) *secrets.Store {
	t.Helper()
	dir := t.TempDir()
	store, err := secrets.NewWithPaths(
		filepath.Join(dir, "secrets.db"),
		filepath.Join(dir, "master.key"),
	)
	if err != nil {
		t.Fatalf("newTestStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSecretsSyncComponent_Name(t *testing.T) {
	c := newSecretsSyncComponent(nil, nil)
	if got := c.Name(); got != "secrets-sync" {
		t.Errorf("Name() = %q, want %q", got, "secrets-sync")
	}
}

func TestSecretsSyncComponent_Health_Pending(t *testing.T) {
	c := newSecretsSyncComponent(nil, nil)
	if got := c.Health().Status; got != "pending" {
		t.Errorf("before Start: Health().Status = %q, want %q", got, "pending")
	}
}

func TestSecretsSyncComponent_Start_NilStore_Skipped(t *testing.T) {
	c := newSecretsSyncComponent(nil, nil)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if got := c.Health().Status; got != "skipped" {
		t.Errorf("Health().Status = %q, want %q", got, "skipped")
	}
}

func TestSecretsSyncComponent_Start_NilSyncer_OK(t *testing.T) {
	// nil syncer with valid store — should not panic, health = "ok"
	store := newTestStore(t)
	c := newSecretsSyncComponent(store, nil)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if got := c.Health().Status; got != "ok" {
		t.Errorf("Health().Status = %q, want %q", got, "ok")
	}
}

func TestSecretsSyncComponent_Start_CloudDisabled(t *testing.T) {
	// syncer nil (cloud disabled) — store's cloud field remains nil, no panic
	store := newTestStore(t)
	c := newSecretsSyncComponent(store, nil)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	h := c.Health()
	if h.Status != "ok" {
		t.Errorf("expected ok, got %q", h.Status)
	}
}

func TestSecretsSyncComponent_Start_WiresSyncer(t *testing.T) {
	store := newTestStore(t)
	syncer := &mockSecretsSyncer{}
	c := newSecretsSyncComponent(store, syncer)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if got := c.Health().Status; got != "ok" {
		t.Errorf("Health().Status = %q, want %q", got, "ok")
	}
}

func TestSecretsSyncComponent_GetForEnv(t *testing.T) {
	store := newTestStore(t)

	// Pre-populate secrets.
	if err := store.Set("openai.api_key", "sk-test-123"); err != nil {
		t.Fatalf("store.Set: %v", err)
	}

	c := newSecretsSyncComponent(store, nil)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	envInject := map[string]string{
		"openai.api_key":  "OPENAI_API_KEY",
		"missing.api_key": "MISSING_ENV", // should be silently omitted
	}
	got := c.GetForEnv(envInject)

	if v, ok := got["OPENAI_API_KEY"]; !ok || v != "sk-test-123" {
		t.Errorf("OPENAI_API_KEY = %q, want %q", v, "sk-test-123")
	}
	if _, ok := got["MISSING_ENV"]; ok {
		t.Error("MISSING_ENV should not appear in result")
	}
}

func TestSecretsSyncComponent_GetForEnv_NilStore(t *testing.T) {
	c := newSecretsSyncComponent(nil, nil)
	got := c.GetForEnv(map[string]string{"k": "V"})
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestSecretsSyncComponent_Stop(t *testing.T) {
	c := newSecretsSyncComponent(nil, nil)
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop() error: %v", err)
	}
}
