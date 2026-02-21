package serve

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestIsServeRunning_NoPIDFile(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "serve.pid")
	if isServeRunningWith(pidPath, "http://localhost:0/health") {
		t.Error("expected false when PID file does not exist")
	}
}

func TestIsServeRunning_EmptyPIDPath(t *testing.T) {
	if isServeRunningWith("", "http://localhost:0/health") {
		t.Error("expected false for empty PID path")
	}
}

func TestIsServeRunning_InvalidPID(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "serve.pid")
	os.WriteFile(pidPath, []byte("not-a-number"), 0644)
	if isServeRunningWith(pidPath, "http://localhost:0/health") {
		t.Error("expected false for invalid PID content")
	}
}

func TestIsServeRunning_StalePID(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "serve.pid")
	// PID 999999999 is almost certainly not running
	os.WriteFile(pidPath, []byte("999999999"), 0644)
	if isServeRunningWith(pidPath, "http://localhost:0/health") {
		t.Error("expected false for stale PID (dead process)")
	}
}

func TestIsServeRunning_AliveButNoHealth(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "serve.pid")
	// Use our own PID (definitely alive)
	os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644)
	// Point health to a port that's not listening
	if isServeRunningWith(pidPath, "http://localhost:1/health") {
		t.Error("expected false when process alive but health unreachable")
	}
}

func TestIsServeRunning_AliveAndHealthy(t *testing.T) {
	// Start a fake health server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	pidPath := filepath.Join(dir, "serve.pid")
	// Use our own PID (definitely alive)
	os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644)

	if !isServeRunningWith(pidPath, srv.URL+"/health") {
		t.Error("expected true when process alive and health responds 200")
	}
}

func TestIsServeRunning_AliveButHealth500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dir := t.TempDir()
	pidPath := filepath.Join(dir, "serve.pid")
	os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0644)

	if isServeRunningWith(pidPath, srv.URL+"/health") {
		t.Error("expected false when health responds non-200")
	}
}

func TestDefaultPIDPath(t *testing.T) {
	p := DefaultPIDPath()
	if p == "" {
		t.Skip("could not determine home dir")
	}
	if filepath.Base(p) != "serve.pid" {
		t.Errorf("expected serve.pid, got %s", filepath.Base(p))
	}
}

func TestStatusMessage(t *testing.T) {
	msg := StatusMessage("hub poller")
	if msg != "cq: serve running, skipping hub poller (managed by serve)" {
		t.Errorf("unexpected message: %s", msg)
	}
}
