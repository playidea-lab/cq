package exechandler

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDetectMode(t *testing.T) {
	cases := []struct {
		cmd  string
		want string
	}{
		{"go test ./...", "test"},
		{"uv run pytest tests/", "test"},
		{"cargo test", "test"},
		{"go build ./...", "build"},
		{"make build", "build"},
		{"go vet ./...", "build"},
		{"git log --oneline -20", "git"},
		{"git diff HEAD~1", "git"},
		{"ls -la", "generic"},
		{"cat file.txt", "generic"},
	}
	for _, c := range cases {
		got := detectMode(c.cmd)
		if got != c.want {
			t.Errorf("detectMode(%q) = %q, want %q", c.cmd, got, c.want)
		}
	}
}

func TestCompressTest(t *testing.T) {
	output := `=== RUN   TestFoo
--- FAIL: TestFoo (0.01s)
    foo_test.go:12: expected 1, got 2
--- PASS: TestBar (0.00s)
ok  	github.com/foo/bar	0.02s
FAIL	github.com/foo/baz	0.01s`

	result := compressTest(output)
	if !strings.Contains(result, "FAIL") {
		t.Error("expected FAIL in compressed test output")
	}
	if !strings.Contains(result, "fail") {
		t.Errorf("expected fail count in summary, got: %s", result)
	}
	// Should not contain verbose RUN lines
	if strings.Contains(result, "=== RUN") {
		t.Error("should not contain === RUN lines")
	}
}

func TestCompressBuild(t *testing.T) {
	output := `# compiling foo
# compiling bar
./main.go:12:5: undefined: Foo
./main.go:15:1: cannot use x (type int) as type string
# done`

	result := compressBuild(output)
	if !strings.Contains(result, "undefined") {
		t.Error("expected error line in build output")
	}
	if strings.Contains(result, "# compiling") {
		t.Error("should not contain compiling lines")
	}
}

func TestCompressGit(t *testing.T) {
	output := `commit abc123
Author: foo <foo@bar.com>
Date:   Mon Jan 1 00:00:00 2026

    fix: some bug

commit def456
Author: bar <bar@baz.com>
Date:   Sun Dec 31 00:00:00 2025

    feat: new feature`

	result := compressGit(output)
	if !strings.Contains(result, "commit abc123") {
		t.Error("expected commit hash")
	}
	if !strings.Contains(result, "fix: some bug") {
		t.Error("expected commit subject")
	}
	if strings.Contains(result, "Author:") {
		t.Error("should not contain Author lines")
	}
}

func TestCompressGeneric(t *testing.T) {
	var lines []string
	for i := 0; i < 200; i++ {
		lines = append(lines, "line")
	}
	output := strings.Join(lines, "\n")
	result := compressGeneric(output)
	if !strings.Contains(result, "lines omitted") {
		t.Error("expected omission marker")
	}
}

func TestHandleBasicCommand(t *testing.T) {
	args, _ := json.Marshal(map[string]any{
		"command": "echo hello",
	})
	res, err := handle("/tmp", args)
	if err != nil {
		t.Fatal(err)
	}
	r := res.(result)
	if r.ExitCode != 0 {
		t.Errorf("expected exit 0, got %d", r.ExitCode)
	}
	if !strings.Contains(r.Stdout, "hello") {
		t.Errorf("expected 'hello' in stdout, got %q", r.Stdout)
	}
	if r.Compressed {
		t.Error("small output should not be compressed")
	}
}

func TestHandleExitCode(t *testing.T) {
	args, _ := json.Marshal(map[string]any{
		"command": "exit 42",
	})
	res, err := handle("/tmp", args)
	if err != nil {
		t.Fatal(err)
	}
	r := res.(result)
	if r.ExitCode != 42 {
		t.Errorf("expected exit 42, got %d", r.ExitCode)
	}
}

func TestHandleModeDetection(t *testing.T) {
	args, _ := json.Marshal(map[string]any{
		"command": "go test ./...",
	})
	res, err := handle("/tmp", args)
	if err != nil {
		t.Fatal(err)
	}
	r := res.(result)
	if r.Mode != "test" {
		t.Errorf("expected mode=test, got %q", r.Mode)
	}
}

func TestHandleTimeout(t *testing.T) {
	args, _ := json.Marshal(map[string]any{
		"command":   "sleep 10",
		"timeout_s": 1,
	})
	res, err := handle("/tmp", args)
	if err != nil {
		t.Fatal(err)
	}
	r := res.(result)
	if r.ExitCode == 0 {
		t.Error("expected non-zero exit code on timeout")
	}
}

func TestTruncate(t *testing.T) {
	s := strings.Repeat("a", 100)
	got := truncate(s, 50)
	if len(got) > 100 { // truncated + marker
		t.Errorf("truncated output too long: %d", len(got))
	}
	if !strings.Contains(got, "truncated") {
		t.Error("expected truncation marker")
	}
}
