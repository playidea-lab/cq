package daemon

import (
	"fmt"
	"testing"
)

// Mock nvidia-smi output (2 GPUs)
const mockNvidiaSmiOutput = `0, NVIDIA GeForce RTX 4090, 24564, 20000, 15, 42
1, NVIDIA GeForce RTX 4090, 24564, 12000, 85, 78`

func mockGpuMonitor(output string) *GpuMonitor {
	return &GpuMonitor{
		parseFunc: func() (string, error) {
			return output, nil
		},
	}
}

func mockUnavailableMonitor() *GpuMonitor {
	return &GpuMonitor{
		parseFunc: func() (string, error) {
			return "", fmt.Errorf("nvidia-smi: not found")
		},
	}
}

func TestGpuMonitor_GetAllGPUs(t *testing.T) {
	m := mockGpuMonitor(mockNvidiaSmiOutput)

	gpus, err := m.GetAllGPUs()
	if err != nil {
		t.Fatalf("GetAllGPUs: %v", err)
	}
	if len(gpus) != 2 {
		t.Fatalf("len = %d, want 2", len(gpus))
	}

	// GPU 0
	g0 := gpus[0]
	if g0.Index != 0 {
		t.Errorf("g0.Index = %d", g0.Index)
	}
	if g0.Name != "NVIDIA GeForce RTX 4090" {
		t.Errorf("g0.Name = %s", g0.Name)
	}
	// 24564 MB ≈ 23.98 GB
	if g0.TotalVRAM < 23.0 || g0.TotalVRAM > 24.1 {
		t.Errorf("g0.TotalVRAM = %.2f, want ~24", g0.TotalVRAM)
	}
	// 20000 MB ≈ 19.53 GB
	if g0.FreeVRAM < 19.0 || g0.FreeVRAM > 20.0 {
		t.Errorf("g0.FreeVRAM = %.2f, want ~19.5", g0.FreeVRAM)
	}
	if g0.Utilization != 15 {
		t.Errorf("g0.Utilization = %d", g0.Utilization)
	}
	if g0.Temperature != 42 {
		t.Errorf("g0.Temperature = %.1f", g0.Temperature)
	}

	// GPU 1
	g1 := gpus[1]
	if g1.Index != 1 {
		t.Errorf("g1.Index = %d", g1.Index)
	}
	if g1.Utilization != 85 {
		t.Errorf("g1.Utilization = %d", g1.Utilization)
	}
}

func TestGpuMonitor_GetGPU(t *testing.T) {
	m := mockGpuMonitor(mockNvidiaSmiOutput)

	gpu, err := m.GetGPU(1)
	if err != nil {
		t.Fatalf("GetGPU: %v", err)
	}
	if gpu.Index != 1 {
		t.Errorf("Index = %d, want 1", gpu.Index)
	}
}

func TestGpuMonitor_GetGPU_NotFound(t *testing.T) {
	m := mockGpuMonitor(mockNvidiaSmiOutput)

	_, err := m.GetGPU(5)
	if err == nil {
		t.Fatal("expected error for nonexistent GPU")
	}
}

func TestGpuMonitor_FindBestGPU(t *testing.T) {
	m := mockGpuMonitor(mockNvidiaSmiOutput)

	// GPU 0 has ~19.5 GB free, GPU 1 has ~11.7 GB free
	idx, err := m.FindBestGPU(10.0)
	if err != nil {
		t.Fatalf("FindBestGPU: %v", err)
	}
	if idx != 0 {
		t.Errorf("idx = %d, want 0 (most free VRAM)", idx)
	}
}

func TestGpuMonitor_FindBestGPU_NotEnough(t *testing.T) {
	m := mockGpuMonitor(mockNvidiaSmiOutput)

	_, err := m.FindBestGPU(30.0) // more than any GPU has
	if err == nil {
		t.Fatal("expected error when no GPU has enough VRAM")
	}
}

func TestGpuMonitor_IsAvailable(t *testing.T) {
	m := mockGpuMonitor(mockNvidiaSmiOutput)
	if !m.IsAvailable() {
		t.Error("expected available")
	}
}

func TestGpuMonitor_Unavailable(t *testing.T) {
	m := mockUnavailableMonitor()
	if m.IsAvailable() {
		t.Error("expected unavailable")
	}

	gpus, err := m.GetAllGPUs()
	if err == nil {
		t.Error("expected error")
	}
	if gpus != nil {
		t.Error("expected nil gpus")
	}
}

func TestGpuMonitor_GPUCount(t *testing.T) {
	m := mockGpuMonitor(mockNvidiaSmiOutput)
	if m.GPUCount() != 2 {
		t.Errorf("GPUCount = %d, want 2", m.GPUCount())
	}

	m2 := mockUnavailableMonitor()
	if m2.GPUCount() != 0 {
		t.Errorf("GPUCount = %d, want 0", m2.GPUCount())
	}
}

func TestParseNvidiaSmi_EmptyOutput(t *testing.T) {
	gpus, err := parseNvidiaSmiOutput("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gpus) != 0 {
		t.Errorf("len = %d, want 0", len(gpus))
	}
}

func TestParseNvidiaSmi_MalformedLine(t *testing.T) {
	output := "0, GPU Name, 24564, 20000, 15, 42\nbad line\n2, Another GPU, 8192, 4096, 50, 65"
	gpus, err := parseNvidiaSmiOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gpus) != 2 {
		t.Errorf("len = %d, want 2 (skip malformed)", len(gpus))
	}
}

func TestParseNvidiaSmi_SingleGPU(t *testing.T) {
	output := "0, Tesla V100-SXM2-16GB, 16384, 15000, 0, 35"
	gpus, err := parseNvidiaSmiOutput(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gpus) != 1 {
		t.Fatalf("len = %d, want 1", len(gpus))
	}
	if gpus[0].Name != "Tesla V100-SXM2-16GB" {
		t.Errorf("name = %s", gpus[0].Name)
	}
	// 16384 MB ≈ 16 GB
	if gpus[0].TotalVRAM < 15.9 || gpus[0].TotalVRAM > 16.1 {
		t.Errorf("TotalVRAM = %.2f, want ~16", gpus[0].TotalVRAM)
	}
}
