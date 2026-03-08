package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/secrets"
	"github.com/changmin/c4-core/internal/serve"
)

// newTestSecretStore creates an in-memory (temp-file) secrets store for tests.
func newTestSecretStore(t *testing.T) *secrets.Store {
	t.Helper()
	dir := t.TempDir()
	store, err := secrets.NewWithPaths(
		filepath.Join(dir, "secrets.db"),
		filepath.Join(dir, "master.key"),
	)
	if err != nil {
		t.Fatalf("newTestSecretStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSecretsSyncComponent_Name(t *testing.T) {
	mgr := serve.NewManager()
	comp := registerSecretsSyncComponent(mgr, config.C4Config{}, nil)
	if got := comp.Name(); got != "secrets-sync" {
		t.Errorf("Name() = %q, want %q", got, "secrets-sync")
	}
}

func TestSecretsSyncComponent_Health_NilStore_Skipped(t *testing.T) {
	comp := &secretsSyncComponent{store: nil}
	if got := comp.Health().Status; got != "skipped" {
		t.Errorf("Health().Status = %q, want %q", got, "skipped")
	}
}

func TestSecretsSyncComponent_Start_NilStore_Skipped(t *testing.T) {
	mgr := serve.NewManager()
	comp := registerSecretsSyncComponent(mgr, config.C4Config{}, nil)
	if err := comp.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if got := comp.Health().Status; got != "skipped" {
		t.Errorf("Health().Status = %q, want %q", got, "skipped")
	}
}

func TestSecretsSyncComponent_Start_NilSyncer_OK(t *testing.T) {
	store := newTestSecretStore(t)
	comp := &secretsSyncComponent{
		store:  store,
		health: serve.ComponentHealth{Status: "pending"},
	}
	if err := comp.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if got := comp.Health().Status; got != "ok" {
		t.Errorf("Health().Status = %q, want %q", got, "ok")
	}
}

func TestSecretsSyncComponent_Stop(t *testing.T) {
	comp := &secretsSyncComponent{}
	if err := comp.Stop(context.Background()); err != nil {
		t.Errorf("Stop() error: %v", err)
	}
}

func TestSecretsSyncComponent_GetForEnv_KeyMapping(t *testing.T) {
	store := newTestSecretStore(t)
	if err := store.Set("anthropic.api_key", "sk-ant-test"); err != nil {
		t.Fatalf("store.Set: %v", err)
	}
	if err := store.Set("openai.api_key", "sk-open-test"); err != nil {
		t.Fatalf("store.Set: %v", err)
	}

	comp := &secretsSyncComponent{store: store}
	envs := comp.GetForEnv([]string{"anthropic.api_key", "openai.api_key", "missing.key"})

	want := map[string]string{
		"ANTHROPIC_API_KEY": "sk-ant-test",
		"OPENAI_API_KEY":    "sk-open-test",
	}
	got := make(map[string]string, len(envs))
	for _, e := range envs {
		for i := 0; i < len(e); i++ {
			if e[i] == '=' {
				got[e[:i]] = e[i+1:]
				break
			}
		}
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("env %s = %q, want %q", k, got[k], v)
		}
	}
	if len(got) != 2 {
		t.Errorf("expected 2 env vars, got %d: %v", len(got), got)
	}
}
