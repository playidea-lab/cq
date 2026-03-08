package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/changmin/c4-core/internal/config"
	"github.com/changmin/c4-core/internal/secrets"
	"github.com/changmin/c4-core/internal/serve"
)

// secretsSyncComponent wires a CloudSyncer into the secret store.
// It must be registered (and started) before any other component that
// relies on cloud-backed secrets. When cloud is disabled (syncer == nil)
// the component starts successfully as a no-op.
type secretsSyncComponent struct {
	store  *secrets.Store
	syncer secrets.CloudSyncer
	mu     sync.Mutex
	health serve.ComponentHealth
}

// compile-time interface assertion
var _ serve.Component = (*secretsSyncComponent)(nil)

// registerSecretsSyncComponent creates a secretsSyncComponent using the provided
// store, registers it with mgr (always first), and returns it so callers can call GetForEnv.
// If store is nil, a no-op component is registered (non-fatal).
func registerSecretsSyncComponent(mgr *serve.Manager, cfg config.C4Config, store *secrets.Store) *secretsSyncComponent {
	comp := &secretsSyncComponent{
		store:  store,
		health: serve.ComponentHealth{Status: "pending"},
	}
	mgr.Register(comp)
	fmt.Fprintf(os.Stderr, "cq serve: registered secrets-sync\n")
	return comp
}

func (s *secretsSyncComponent) Name() string { return "secrets-sync" }

// Start wires the syncer into the store (if both are non-nil) and marks the
// component healthy. When store is nil the component is skipped gracefully.
func (s *secretsSyncComponent) Start(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.store == nil {
		s.health = serve.ComponentHealth{Status: "skipped", Detail: "no store"}
		return nil
	}

	if s.syncer != nil {
		s.store.SetCloud(s.syncer)
	}

	s.health = serve.ComponentHealth{Status: "ok"}
	return nil
}

// Stop is a no-op; the cloud syncer has no persistent goroutines of its own.
func (s *secretsSyncComponent) Stop(_ context.Context) error { return nil }

// Health returns the current health status.
// Returns "skipped" immediately when store is nil (no Start() required).
func (s *secretsSyncComponent) Health() serve.ComponentHealth {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store == nil {
		return serve.ComponentHealth{Status: "skipped", Detail: "secrets store unavailable"}
	}
	return s.health
}

// GetForEnv returns env-var strings ("KEY=value") for each key listed in envInject.
// Keys not found in the store are silently skipped.
// The returned slice is suitable for appending to HubComponentConfig.Env.
//
// Key format: "some.key" → env var "SOME_KEY" (dots and hyphens → underscores, uppercase).
func (s *secretsSyncComponent) GetForEnv(envInject []string) []string {
	if s.store == nil || len(envInject) == 0 {
		return nil
	}
	var envs []string
	for _, key := range envInject {
		val, err := s.store.Get(key)
		if err != nil {
			// Not found or decrypt error — skip silently.
			continue
		}
		envName := strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
		envName = strings.ReplaceAll(envName, "-", "_")
		envs = append(envs, envName+"="+val)
	}
	return envs
}
