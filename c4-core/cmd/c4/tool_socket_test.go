package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// startTestSocket starts a toolSocketComponent on a temp socket path.
// Callers must have already called setupTestDB to set the global projectDir.
func startTestSocket(t *testing.T) (sockPath string, cleanup func()) {
	t.Helper()

	sockPath = filepath.Join(t.TempDir(), "tool.sock")
	comp := newToolSocketComponent(sockPath)

	ctx, cancel := context.WithCancel(context.Background())
	if err := comp.Start(ctx); err != nil {
		cancel()
		t.Fatalf("toolSocketComponent.Start: %v", err)
	}

	// Wait until the socket is accepting connections (deterministic, no fixed sleep).
	for i := 0; i < 20; i++ {
		if conn, err := net.DialTimeout("unix", sockPath, time.Second); err == nil {
			conn.Close()
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	return sockPath, func() {
		cancel()
		comp.Stop(context.Background()) //nolint:errcheck
	}
}

// setupSocket is a one-liner helper: setupTestDB + startTestSocket.
// Returns the db, sockPath, and a combined cleanup.
func setupSocket(t *testing.T) (db *sql.DB, sockPath string, cleanup func()) {
	t.Helper()
	db, dbCleanup := setupTestDB(t)
	sockPath, sockCleanup := startTestSocket(t)
	return db, sockPath, func() {
		sockCleanup()
		dbCleanup()
	}
}

// TestToolSocketHealth verifies Start creates the socket file.
func TestToolSocketHealth(t *testing.T) {
	_, sockPath, cleanup := setupSocket(t)
	defer cleanup()

	if _, err := os.Stat(sockPath); err != nil {
		t.Fatalf("socket file not created: %v", err)
	}
}

// TestToolSocketList verifies the "list" operation returns registered tools.
func TestToolSocketList(t *testing.T) {
	_, sockPath, cleanup := setupSocket(t)
	defer cleanup()

	resp, err := callSocket(sockPath, sockRequest{Op: "list"})
	if err != nil {
		t.Fatalf("callSocket list: %v", err)
	}
	if len(resp.Tools) == 0 {
		t.Fatal("expected at least one tool in list response")
	}

	names := map[string]bool{}
	for _, tool := range resp.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"c4_status", "c4_get_task"} {
		if !names[want] {
			t.Errorf("expected tool %q in socket list response", want)
		}
	}
}

// TestToolSocketSchema verifies the "schema" operation returns the correct schema.
func TestToolSocketSchema(t *testing.T) {
	_, sockPath, cleanup := setupSocket(t)
	defer cleanup()

	resp, err := callSocket(sockPath, sockRequest{Op: "schema", Tool: "c4_status"})
	if err != nil {
		t.Fatalf("callSocket schema: %v", err)
	}
	if resp.Schema == nil {
		t.Fatal("expected non-nil schema for c4_status")
	}
	if resp.Schema.Name != "c4_status" {
		t.Errorf("schema.Name = %q, want c4_status", resp.Schema.Name)
	}
	if resp.Schema.Description == "" {
		t.Error("expected non-empty schema description")
	}
}

// TestToolSocketSchemaUnknown verifies error for an unknown tool.
func TestToolSocketSchemaUnknown(t *testing.T) {
	_, sockPath, cleanup := setupSocket(t)
	defer cleanup()

	_, err := callSocket(sockPath, sockRequest{Op: "schema", Tool: "nonexistent_xyz"})
	if err == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
	if !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestToolSocketCall verifies the "call" operation executes a tool.
func TestToolSocketCall(t *testing.T) {
	db, sockPath, cleanup := setupSocket(t)
	defer cleanup()
	insertState(t, db, "socket-test", "EXECUTE")

	resp, err := callSocket(sockPath, sockRequest{Op: "call", Tool: "c4_status", Args: map[string]any{}})
	if err != nil {
		t.Fatalf("callSocket call: %v", err)
	}

	// Result should be a map with a "state" key.
	b, _ := json.Marshal(resp.Result)
	var result map[string]any
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatalf("result is not a JSON object: %v\nraw: %s", err, b)
	}
	if _, ok := result["state"]; !ok {
		t.Errorf("expected 'state' key in c4_status result, got: %v", result)
	}
}

// TestToolSocketUnknownOp verifies an error is returned for unknown operations.
func TestToolSocketUnknownOp(t *testing.T) {
	_, sockPath, cleanup := setupSocket(t)
	defer cleanup()

	_, err := callSocket(sockPath, sockRequest{Op: "bogus"})
	if err == nil {
		t.Fatal("expected error for unknown op")
	}
	if !strings.Contains(err.Error(), "unknown op") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestToolSocketStop verifies the socket file is removed after Stop.
func TestToolSocketStop(t *testing.T) {
	_, sockPath, cleanup := setupSocket(t)
	// Call cleanup (which calls Stop) immediately.
	cleanup()

	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Errorf("expected socket file to be removed after Stop, err=%v", err)
	}
}

// TestCallSocketUnavailable verifies callSocket returns an error when no server is listening.
func TestCallSocketUnavailable(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "nonexistent.sock")
	_, err := callSocket(sockPath, sockRequest{Op: "list"})
	if err == nil {
		t.Fatal("expected error when socket is unavailable, got nil")
	}
}

// TestToolSocketConcurrent verifies concurrent requests are handled correctly.
func TestToolSocketConcurrent(t *testing.T) {
	_, sockPath, cleanup := setupSocket(t)
	defer cleanup()

	const n = 10
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, err := callSocket(sockPath, sockRequest{Op: "list"})
			errs <- err
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent callSocket: %v", err)
		}
	}
}

// TestToolSocketRawConn verifies the server handles malformed JSON gracefully.
func TestToolSocketRawConn(t *testing.T) {
	_, sockPath, cleanup := setupSocket(t)
	defer cleanup()

	conn, err := net.DialTimeout("unix", sockPath, time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	conn.Write([]byte("not-json\n")) //nolint:errcheck

	var resp sockResponse
	conn.SetDeadline(time.Now().Add(3 * time.Second)) //nolint:errcheck
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if resp.Error == "" {
		t.Error("expected non-empty error for malformed request")
	}
}
