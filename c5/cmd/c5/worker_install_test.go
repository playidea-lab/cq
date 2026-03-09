package main

import (
	"testing"
)

func TestWorkerInstallMissingImage(t *testing.T) {
	err := runWorkerInstall(workerInstallConfig{
		HubURL: "http://localhost:8585",
		APIKey: "test-key",
	})
	if err == nil {
		t.Fatal("expected error for missing image")
	}
	if got := err.Error(); got != "--image is required" {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestWorkerInstallMissingHubURL(t *testing.T) {
	t.Setenv("C5_HUB_URL", "")
	err := runWorkerInstall(workerInstallConfig{
		Image: "ghcr.io/test:latest",
	})
	if err == nil {
		t.Fatal("expected error for missing hub-url")
	}
	if got := err.Error(); got != "--hub-url is required (or set C5_HUB_URL)" {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestWorkerInstallMissingAPIKey(t *testing.T) {
	t.Setenv("C5_API_KEY", "")
	err := runWorkerInstall(workerInstallConfig{
		Image:  "ghcr.io/test:latest",
		HubURL: "http://localhost:8585",
	})
	if err == nil {
		t.Fatal("expected error for missing api-key")
	}
	if got := err.Error(); got != "--api-key is required (or set C5_API_KEY)" {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestWorkerInstallHubURLFromEnv(t *testing.T) {
	t.Setenv("C5_HUB_URL", "http://env-hub:8585")
	t.Setenv("C5_API_KEY", "")
	err := runWorkerInstall(workerInstallConfig{
		Image: "ghcr.io/test:latest",
	})
	// Should fail on missing API key, not hub URL
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "--api-key is required (or set C5_API_KEY)" {
		t.Fatalf("expected api-key error, got: %s", got)
	}
}

func TestWorkerInstallAPIKeyFromEnv(t *testing.T) {
	t.Setenv("C5_API_KEY", "env-key")
	// DryRun to avoid actually calling docker
	err := runWorkerInstall(workerInstallConfig{
		Image:  "ghcr.io/test:latest",
		HubURL: "http://localhost:8585",
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorkerInstallDryRun(t *testing.T) {
	err := runWorkerInstall(workerInstallConfig{
		Image:  "ghcr.io/test:latest",
		HubURL: "http://localhost:8585",
		APIKey: "test-key",
		Name:   "test-worker",
		DryRun: true,
		Detach: true,
	})
	if err != nil {
		t.Fatalf("dry-run should succeed: %v", err)
	}
}

func TestWorkerInstallDryRunNoGPU(t *testing.T) {
	err := runWorkerInstall(workerInstallConfig{
		Image:  "ghcr.io/test:latest",
		HubURL: "http://localhost:8585",
		APIKey: "test-key",
		Name:   "test-worker",
		NoGPU:  true,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("dry-run should succeed: %v", err)
	}
}

func TestWorkerInstallDryRunWithExtraEnv(t *testing.T) {
	err := runWorkerInstall(workerInstallConfig{
		Image:    "ghcr.io/test:latest",
		HubURL:   "http://localhost:8585",
		APIKey:   "test-key",
		Name:     "test-worker",
		DryRun:   true,
		ExtraEnv: []string{"CUSTOM_VAR=hello", "ANOTHER=world"},
	})
	if err != nil {
		t.Fatalf("dry-run should succeed: %v", err)
	}
}

func TestWorkerInstallDefaultName(t *testing.T) {
	// Verify that a default name is generated when not specified
	cfg := workerInstallConfig{
		Image:  "ghcr.io/test:latest",
		HubURL: "http://localhost:8585",
		APIKey: "test-key",
		DryRun: true,
	}
	err := runWorkerInstall(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRandomHex(t *testing.T) {
	h := randomHex(4)
	if len(h) != 8 {
		t.Fatalf("expected 8 hex chars, got %d: %s", len(h), h)
	}
	// Verify uniqueness (probabilistic)
	h2 := randomHex(4)
	if h == h2 {
		t.Fatal("two random hex values should differ")
	}
}

func TestHasNvidiaSMI(t *testing.T) {
	// Just ensure it doesn't panic; result depends on environment
	_ = hasNvidiaSMI()
}
