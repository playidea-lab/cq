package shell_test

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/shell"
)

func TestCommand_ReturnsCmd(t *testing.T) {
	ctx := context.Background()
	cmd := shell.Command(ctx, "echo hello")
	if cmd == nil {
		t.Fatal("Command returned nil")
	}
}

func TestCommand_UsesCorrectShell(t *testing.T) {
	ctx := context.Background()
	cmd := shell.Command(ctx, "echo hello")

	base := cmd.Path
	// Path may be an absolute path; check the base name.
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(base, "bash.exe") {
			t.Errorf("expected bash.exe on Windows, got %q", base)
		}
	} else {
		if !strings.HasSuffix(base, "sh") {
			t.Errorf("expected sh on Unix, got %q", base)
		}
	}
}

func TestCommand_Args(t *testing.T) {
	ctx := context.Background()
	const script = "echo hello"
	cmd := shell.Command(ctx, script)

	args := cmd.Args
	// Args[0] is the binary; Args[1] should be "-c"; Args[2] is the script.
	if len(args) < 3 {
		t.Fatalf("expected at least 3 args, got %d: %v", len(args), args)
	}
	if args[1] != "-c" {
		t.Errorf("expected args[1]='-c', got %q", args[1])
	}
	if args[2] != script {
		t.Errorf("expected args[2]=%q, got %q", script, args[2])
	}
}

func TestCommand_RunsSuccessfully(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("integration run skipped on Windows CI without bash.exe")
	}
	ctx := context.Background()
	out, err := shell.Command(ctx, "echo hello").Output()
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestCommand_ContextCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("integration run skipped on Windows CI without bash.exe")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	cmd := shell.Command(ctx, "sleep 10")
	err := cmd.Run()
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}
