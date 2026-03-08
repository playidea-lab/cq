package main

import (
	"os/exec"
	"strconv"
	"strings"

	"github.com/piqsol/c4/c5/internal/model"
)

// detectGPU runs nvidia-smi to detect GPU count, model, and VRAM.
// Returns zero values if nvidia-smi is not available.
func detectGPU() (count int, model string, totalVRAM float64) {
	out, err := exec.Command("nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return 0, "", 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var validLines []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			validLines = append(validLines, l)
		}
	}
	if len(validLines) == 0 {
		return 0, "", 0
	}
	count = len(validLines)
	// Parse first line: "NVIDIA GeForce RTX 5080, 16376"
	parts := strings.SplitN(validLines[0], ",", 2)
	if len(parts) >= 1 {
		model = strings.TrimSpace(parts[0])
	}
	if len(parts) >= 2 {
		vramMiB, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err == nil {
			totalVRAM = vramMiB / 1024
		}
	}
	return count, model, totalVRAM
}

// defaultCapabilities returns auto-generated capabilities based on GPU availability.
func defaultCapabilities(gpuCount int) []model.Capability {
	if gpuCount > 0 {
		return []model.Capability{
			{
				Name:        "run_command",
				Description: "Run arbitrary shell commands",
				Tags:        []string{"gpu", "shell"},
			},
			{
				Name:        "train_model",
				Description: "PyTorch/TF model training",
				Tags:        []string{"gpu", "pytorch"},
			},
		}
	}
	return []model.Capability{
		{
			Name:        "run_command",
			Description: "Run arbitrary shell commands",
			Tags:        []string{"cpu", "shell"},
		},
	}
}
