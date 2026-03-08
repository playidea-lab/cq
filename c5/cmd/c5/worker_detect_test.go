package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestDetectGPUNvidiaSmiNotFound verifies that detectGPU returns zero values
// when nvidia-smi is not on PATH.
func TestDetectGPUNvidiaSmiNotFound(t *testing.T) {
	// Override PATH to empty so nvidia-smi cannot be found.
	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	os.Setenv("PATH", "")

	count, model, vram := detectGPU()
	if count != 0 || model != "" || vram != 0 {
		t.Errorf("expected zero values when nvidia-smi not found, got count=%d model=%q vram=%f", count, model, vram)
	}
}

// TestDetectGPUParsing verifies parsing of multi-GPU nvidia-smi output.
// It stubs nvidia-smi by creating a fake executable that prints sample output.
func TestDetectGPUParsing(t *testing.T) {
	// Create a temp dir with a fake nvidia-smi script.
	dir := t.TempDir()
	script := dir + "/nvidia-smi"
	// Sample output: 2 GPUs, RTX 5080, 16376 MiB each
	content := "#!/bin/sh\nprintf 'NVIDIA GeForce RTX 5080, 16376\\nNVIDIA GeForce RTX 5080, 16376\\n'"
	if err := os.WriteFile(script, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}

	orig := os.Getenv("PATH")
	t.Cleanup(func() { os.Setenv("PATH", orig) })
	os.Setenv("PATH", dir+":"+orig)

	// Verify the fake nvidia-smi is accessible.
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		t.Skipf("fake nvidia-smi not accessible in PATH: %v", err)
	}

	count, model, vram := detectGPU()
	if count != 2 {
		t.Errorf("expected count=2, got %d", count)
	}
	if model != "NVIDIA GeForce RTX 5080" {
		t.Errorf("expected model=%q, got %q", "NVIDIA GeForce RTX 5080", model)
	}
	// 16376 MiB / 1024 ≈ 15.99 GB — check within 0.1 tolerance
	expectedVRAM := 16376.0 / 1024.0
	if vram < expectedVRAM-0.1 || vram > expectedVRAM+0.1 {
		t.Errorf("expected vram≈%.2f, got %.2f", expectedVRAM, vram)
	}
}

// TestDefaultCapabilitiesWithGPU verifies GPU caps include train_model and run_command.
func TestDefaultCapabilitiesWithGPU(t *testing.T) {
	caps := defaultCapabilities(2)
	names := make(map[string]bool)
	for _, c := range caps {
		names[c.Name] = true
	}
	if !names["run_command"] {
		t.Error("expected run_command capability")
	}
	if !names["train_model"] {
		t.Error("expected train_model capability")
	}
	// Verify GPU tags
	for _, c := range caps {
		hasGPUTag := false
		for _, tag := range c.Tags {
			if tag == "gpu" {
				hasGPUTag = true
			}
		}
		if !hasGPUTag {
			t.Errorf("capability %q missing gpu tag, tags=%v", c.Name, c.Tags)
		}
	}
}

// TestDefaultCapabilitiesNoGPU verifies CPU-only caps: run_command only with cpu tag.
func TestDefaultCapabilitiesNoGPU(t *testing.T) {
	caps := defaultCapabilities(0)
	if len(caps) != 1 {
		t.Fatalf("expected 1 capability, got %d", len(caps))
	}
	if caps[0].Name != "run_command" {
		t.Errorf("expected run_command, got %q", caps[0].Name)
	}
	hasCPU := false
	for _, tag := range caps[0].Tags {
		if tag == "cpu" {
			hasCPU = true
		}
	}
	if !hasCPU {
		t.Errorf("expected cpu tag, got %v", caps[0].Tags)
	}
	hasGPU := false
	for _, tag := range caps[0].Tags {
		if tag == "gpu" {
			hasGPU = true
		}
	}
	if hasGPU {
		t.Errorf("cpu-only caps should not have gpu tag")
	}
}

// TestWorkerCmdAutoDetectFlag verifies --no-auto-detect flag is registered.
func TestWorkerCmdAutoDetectFlag(t *testing.T) {
	cmd := workerCmd()
	f := cmd.Flags().Lookup("no-auto-detect")
	if f == nil {
		t.Fatal("--no-auto-detect flag not found in workerCmd")
	}
	if !strings.Contains(f.Usage, "GPU") && !strings.Contains(f.Usage, "auto") {
		t.Errorf("unexpected flag usage: %q", f.Usage)
	}
}
