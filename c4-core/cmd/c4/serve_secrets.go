package main

import (
	"context"
	"sync"

	"github.com/changmin/c4-core/internal/secrets"
	"github.com/changmin/c4-core/internal/serve"
)

// secretsSyncComponent wires a CloudSyncer into the secret store.
// It must be registered (and started) before any other component that
// relies on cloud-backed secrets.  When cloud is disabled (syncer == nil)
// the component starts successfully as a no-op.
type secretsSyncComponent struct {
	store  *secrets.Store
	syncer secrets.CloudSyncer
	mu     sync.Mutex
	health serve.ComponentHealth
}

// compile-time interface assertion
var _ serve.Component = (*secretsSyncComponent)(nil)

func newSecretsSyncComponent(store *secrets.Store, syncer secrets.CloudSyncer) *secretsSyncComponent {
	return &secretsSyncComponent{
		store:  store,
		syncer: syncer,
		health: serve.ComponentHealth{Status: "pending"},
	}
}

func (s *secretsSyncComponent) Name() string { return "secrets-sync" }

// Start wires the syncer into the store (if both are non-nil) and marks the
// component healthy.  When store is nil the component is skipped gracefully.
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
func (s *secretsSyncComponent) Health() serve.ComponentHealth {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.health
}

// GetForEnv resolves envInject keys against the secret store and returns a
// map of environment-variable-name → secret-value.  Missing secrets are
// silently omitted.
//
// Example usage (C5 Hub credential passthrough):
//
//	env := comp.GetForEnv(map[string]string{
//	    "openai.api_key": "OPENAI_API_KEY",
//	})
//	// → map["OPENAI_API_KEY"]"sk-..."
func (s *secretsSyncComponent) GetForEnv(envInject map[string]string) map[string]string {
	result := make(map[string]string, len(envInject))
	if s.store == nil {
		return result
	}
	for secretKey, envName := range envInject {
		val, err := s.store.Get(secretKey)
		if err != nil {
			continue // ErrNotFound or decrypt error — skip silently
		}
		result[envName] = val
	}
	return result
}
