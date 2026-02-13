package daemon

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// GpuMonitor detects and queries GPU status via nvidia-smi.
type GpuMonitor struct {
	// parseFunc can be overridden for testing (default: runNvidiaSmi)
	parseFunc func() (string, error)
}

// NewGpuMonitor creates a GPU monitor.
func NewGpuMonitor() *GpuMonitor {
	return &GpuMonitor{
		parseFunc: runNvidiaSmi,
	}
}

// IsAvailable returns true if nvidia-smi is found and functional.
func (m *GpuMonitor) IsAvailable() bool {
	_, err := m.parseFunc()
	return err == nil
}

// GetAllGPUs returns info for all detected GPUs.
func (m *GpuMonitor) GetAllGPUs() ([]GpuInfo, error) {
	output, err := m.parseFunc()
	if err != nil {
		return nil, err
	}
	return parseNvidiaSmiOutput(output)
}

// GetGPU returns info for a specific GPU index.
func (m *GpuMonitor) GetGPU(index int) (*GpuInfo, error) {
	gpus, err := m.GetAllGPUs()
	if err != nil {
		return nil, err
	}
	for _, g := range gpus {
		if g.Index == index {
			return &g, nil
		}
	}
	return nil, fmt.Errorf("GPU index %d not found", index)
}

// FindBestGPU returns the GPU index with the most free VRAM >= minVRAM.
func (m *GpuMonitor) FindBestGPU(minVRAM float64) (int, error) {
	gpus, err := m.GetAllGPUs()
	if err != nil {
		return -1, err
	}

	bestIdx := -1
	bestFree := 0.0
	for _, g := range gpus {
		if g.FreeVRAM >= minVRAM && g.FreeVRAM > bestFree {
			bestIdx = g.Index
			bestFree = g.FreeVRAM
		}
	}

	if bestIdx < 0 {
		return -1, fmt.Errorf("no GPU with %.1f GB free VRAM available", minVRAM)
	}
	return bestIdx, nil
}

// GPUCount returns the number of available GPUs (0 if nvidia-smi unavailable).
func (m *GpuMonitor) GPUCount() int {
	gpus, err := m.GetAllGPUs()
	if err != nil {
		return 0
	}
	return len(gpus)
}

// runNvidiaSmi executes nvidia-smi and returns CSV output.
func runNvidiaSmi() (string, error) {
	cmd := exec.Command("nvidia-smi",
		"--query-gpu=index,name,memory.total,memory.free,utilization.gpu,temperature.gpu",
		"--format=csv,noheader,nounits")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("nvidia-smi: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// parseNvidiaSmiOutput parses CSV output from nvidia-smi.
// Expected format per line: "0, NVIDIA GeForce RTX 4090, 24564, 20000, 15, 42"
func parseNvidiaSmiOutput(output string) ([]GpuInfo, error) {
	if output == "" {
		return nil, nil
	}

	lines := strings.Split(output, "\n")
	gpus := make([]GpuInfo, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) < 6 {
			continue // skip malformed lines
		}

		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}

		index, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		totalMB, _ := strconv.ParseFloat(parts[2], 64)
		freeMB, _ := strconv.ParseFloat(parts[3], 64)
		util, _ := strconv.Atoi(parts[4])
		temp, _ := strconv.ParseFloat(parts[5], 64)

		gpus = append(gpus, GpuInfo{
			Index:       index,
			Name:        parts[1],
			TotalVRAM:   totalMB / 1024.0, // MB → GB
			FreeVRAM:    freeMB / 1024.0,
			Utilization: util,
			Temperature: temp,
		})
	}

	return gpus, nil
}
