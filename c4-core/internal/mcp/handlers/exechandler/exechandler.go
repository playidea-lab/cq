// Package exechandler provides the c4_execute MCP tool.
// It runs shell commands in a subprocess and compresses large outputs
// to reduce context window consumption — analogous to Context Mode's execute tool.
package exechandler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
)

const (
	defaultMaxBytes         = 8192
	defaultCompressThreshold = 4096
	defaultTimeoutSec       = 60
)

// Register registers the c4_execute tool on the registry.
func Register(reg *mcp.Registry, rootDir string) {
	reg.Register(mcp.ToolSchema{
		Name: "c4_execute",
		Description: "Run a shell command and return compressed output. " +
			"Use instead of Bash for commands with large output (tests, logs, git history, builds). " +
			"Automatically extracts relevant lines (errors, failures, warnings) and omits noise. " +
			"Reports original vs returned size so you know how much was compressed.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Shell command to run (executed via sh -c)",
				},
				"work_dir": map[string]any{
					"type":        "string",
					"description": "Working directory (default: project root)",
				},
				"mode": map[string]any{
					"type":        "string",
					"enum":        []string{"auto", "test", "build", "git", "generic"},
					"description": "Compression mode. auto=detect from command pattern (default)",
				},
				"max_bytes": map[string]any{
					"type":        "integer",
					"description": fmt.Sprintf("Max bytes to return after compression (default: %d)", defaultMaxBytes),
				},
				"timeout_s": map[string]any{
					"type":        "integer",
					"description": fmt.Sprintf("Timeout in seconds (default: %d)", defaultTimeoutSec),
				},
			},
			"required": []string{"command"},
		},
	}, func(args json.RawMessage) (any, error) {
		return handle(rootDir, args)
	})
}

type input struct {
	Command   string `json:"command"`
	WorkDir   string `json:"work_dir"`
	Mode      string `json:"mode"`
	MaxBytes  int    `json:"max_bytes"`
	TimeoutS  int    `json:"timeout_s"`
}

type result struct {
	ExitCode     int    `json:"exit_code"`
	Stdout       string `json:"stdout"`
	Stderr       string `json:"stderr,omitempty"`
	Compressed   bool   `json:"compressed"`
	OriginalSize int    `json:"original_size"`
	ReturnedSize int    `json:"returned_size"`
	DurationMs   int64  `json:"duration_ms"`
	Mode         string `json:"mode"`
}

func handle(rootDir string, raw json.RawMessage) (any, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if in.Command == "" {
		return nil, errors.New("command is required")
	}
	if in.MaxBytes <= 0 {
		in.MaxBytes = defaultMaxBytes
	}
	if in.TimeoutS <= 0 {
		in.TimeoutS = defaultTimeoutSec
	}
	workDir := in.WorkDir
	if workDir == "" {
		workDir = rootDir
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(in.TimeoutS)*time.Second)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, "sh", "-c", in.Command)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	elapsed := time.Since(start).Milliseconds()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() != nil {
			exitCode = -1
		}
	}

	stdoutStr := stdout.String()
	stderrStr := stderr.String()
	origSize := len(stdoutStr) + len(stderrStr)

	mode := in.Mode
	if mode == "" || mode == "auto" {
		mode = detectMode(in.Command)
	}

	compressedStdout, compressed := compress(stdoutStr, mode, in.MaxBytes)
	compressedStderr := truncate(stderrStr, in.MaxBytes/4)

	retSize := len(compressedStdout) + len(compressedStderr)

	return result{
		ExitCode:     exitCode,
		Stdout:       compressedStdout,
		Stderr:       compressedStderr,
		Compressed:   compressed,
		OriginalSize: origSize,
		ReturnedSize: retSize,
		DurationMs:   elapsed,
		Mode:         mode,
	}, nil
}

// detectMode infers compression mode from the command string.
func detectMode(cmd string) string {
	cmd = strings.ToLower(cmd)
	switch {
	case strings.Contains(cmd, "go test") || strings.Contains(cmd, "pytest") ||
		strings.Contains(cmd, "uv run pytest") || strings.Contains(cmd, "cargo test") ||
		strings.Contains(cmd, "npm test") || strings.Contains(cmd, "bun test"):
		return "test"
	case strings.Contains(cmd, "go build") || strings.Contains(cmd, "make ") ||
		strings.Contains(cmd, "cargo build") || strings.Contains(cmd, "npm run build") ||
		strings.Contains(cmd, "go vet") || strings.Contains(cmd, "golangci"):
		return "build"
	case strings.Contains(cmd, "git log") || strings.Contains(cmd, "git diff") ||
		strings.Contains(cmd, "git show"):
		return "git"
	default:
		return "generic"
	}
}

// compress reduces output size based on mode. Returns (compressed, wasCompressed).
func compress(output, mode string, maxBytes int) (string, bool) {
	if len(output) <= defaultCompressThreshold {
		return output, false
	}
	var result string
	switch mode {
	case "test":
		result = compressTest(output)
	case "build":
		result = compressBuild(output)
	case "git":
		result = compressGit(output)
	default:
		result = compressGeneric(output)
	}
	return truncate(result, maxBytes), true
}

// compressTest extracts failures, panics, errors, and summary from test output.
func compressTest(output string) string {
	lines := strings.Split(output, "\n")
	var kept, summary []string
	passCount, failCount := 0, 0

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "--- FAIL") || strings.HasPrefix(line, "FAIL"):
			kept = append(kept, line)
			failCount++
		case strings.HasPrefix(line, "--- PASS") || strings.HasPrefix(line, "ok "):
			passCount++
			summary = append(summary, line)
		case strings.Contains(line, "panic:") || strings.Contains(line, "PANIC"):
			kept = append(kept, line)
		case strings.HasPrefix(line, "    ") && len(kept) > 0:
			// indented lines following a FAIL — keep as error context
			kept = append(kept, line)
		case strings.Contains(line, "Error:") || strings.Contains(line, "error:"):
			kept = append(kept, line)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[test: %d pass, %d fail]\n", passCount, failCount)
	if len(kept) > 0 {
		b.WriteString(strings.Join(kept, "\n"))
		b.WriteByte('\n')
	}
	if len(summary) > 0 && len(summary) <= 5 {
		b.WriteString(strings.Join(summary, "\n"))
	}
	return b.String()
}

// compressBuild extracts error and warning lines from build output.
func compressBuild(output string) string {
	lines := strings.Split(output, "\n")
	var kept []string
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "warning") ||
			strings.Contains(lower, "undefined") || strings.Contains(lower, "cannot") ||
			strings.HasPrefix(lower, "ld:") || strings.HasPrefix(lower, "linker") {
			kept = append(kept, line)
		}
	}
	if len(kept) == 0 {
		// No errors found — return last 20 lines
		start := len(lines) - 20
		if start < 0 {
			start = 0
		}
		return "[no errors detected]\n" + strings.Join(lines[start:], "\n")
	}
	return strings.Join(kept, "\n")
}

// compressGit keeps the first line (hash + subject) of each commit.
func compressGit(output string) string {
	lines := strings.Split(output, "\n")
	var kept []string
	inCommit := false
	for _, line := range lines {
		if strings.HasPrefix(line, "commit ") {
			inCommit = true
			kept = append(kept, line)
		} else if inCommit && strings.TrimSpace(line) != "" &&
			!strings.HasPrefix(line, "Author:") && !strings.HasPrefix(line, "Date:") {
			// first non-empty body line = subject
			kept = append(kept, "  "+strings.TrimSpace(line))
			inCommit = false
		}
	}
	if len(kept) == 0 {
		return compressGeneric(output)
	}
	return strings.Join(kept, "\n")
}

// compressGeneric returns head + tail with an omission marker.
func compressGeneric(output string) string {
	const headLines = 50
	const tailLines = 20
	lines := strings.Split(output, "\n")
	total := len(lines)
	if total <= headLines+tailLines {
		return output
	}
	head := lines[:headLines]
	tail := lines[total-tailLines:]
	omitted := total - headLines - tailLines
	return strings.Join(head, "\n") +
		fmt.Sprintf("\n... [%d lines omitted] ...\n", omitted) +
		strings.Join(tail, "\n")
}

// truncate hard-caps the string at maxBytes.
func truncate(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	return s[:maxBytes] + fmt.Sprintf("\n[truncated: %d bytes total]", len(s))
}
