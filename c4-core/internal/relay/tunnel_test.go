package relay

import (
	"archive/tar"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// buildTarBytes creates an in-memory tar archive containing one file.
func buildTarBytes(name, content string) []byte {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{
		Name:    name,
		Mode:    0644,
		Size:    int64(len(content)),
		ModTime: time.Now(),
	}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write([]byte(content))
	_ = tw.Close()
	return buf.Bytes()
}

// TestHandleTunnel starts a mock WebSocket server that sends tar bytes,
// then verifies the file is extracted to a temp directory.
func TestHandleTunnel(t *testing.T) {
	// Build a tar archive containing one test file.
	tarData := buildTarBytes("hello.txt", "hello from tunnel")

	tunnelID := "test-tunnel-123"
	destDir := t.TempDir()

	// clientDone signals that the client has finished reading.
	clientDone := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify path and query params.
		if !strings.HasPrefix(r.URL.Path, "/tunnel/") {
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("role") != "receiver" {
			http.Error(w, "missing role=receiver", http.StatusBadRequest)
			return
		}

		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}

		// Send tar data as a binary frame.
		if err := wsutil.WriteServerMessage(conn, ws.OpBinary, tarData); err != nil {
			t.Errorf("server write: %v", err)
			conn.Close()
			return
		}

		// Wait until client has processed the data, then close.
		select {
		case <-clientDone:
		case <-time.After(5 * time.Second):
		}
		conn.Close()
	}))
	defer srv.Close()

	wsURL := "ws://" + srv.Listener.Addr().String()

	// Run HandleTunnel in a goroutine — it will block until the WS closes.
	errCh := make(chan error, 1)
	go func() {
		errCh <- HandleTunnel(wsURL, tunnelID, destDir, "test-token")
	}()

	// Wait a moment for tar to extract the data, then signal server to close.
	time.Sleep(200 * time.Millisecond)

	// Verify the file was extracted.
	extracted := filepath.Join(destDir, "hello.txt")
	data, err := os.ReadFile(extracted)
	close(clientDone)

	if err != nil {
		t.Fatalf("ReadFile %q: %v", extracted, err)
	}
	if string(data) != "hello from tunnel" {
		t.Errorf("file content: got %q, want %q", string(data), "hello from tunnel")
	}

	// Wait for HandleTunnel to complete.
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("HandleTunnel error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("HandleTunnel did not return after server closed connection")
	}
}

// TestHandleTunnel_CreatesDest verifies that HandleTunnel creates the
// destination directory if it does not exist.
func TestHandleTunnel_CreatesDest(t *testing.T) {
	tarData := buildTarBytes("sub.txt", "sub content")
	base := t.TempDir()
	destDir := filepath.Join(base, "new", "nested", "dir")

	clientDone := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			return
		}
		_ = wsutil.WriteServerMessage(conn, ws.OpBinary, tarData)
		select {
		case <-clientDone:
		case <-time.After(5 * time.Second):
		}
		conn.Close()
	}))
	defer srv.Close()

	wsURL := "ws://" + srv.Listener.Addr().String()

	errCh := make(chan error, 1)
	go func() {
		errCh <- HandleTunnel(wsURL, "tid", destDir, "")
	}()

	time.Sleep(200 * time.Millisecond)

	extracted := filepath.Join(destDir, "sub.txt")
	data, err := os.ReadFile(extracted)
	close(clientDone)

	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "sub content" {
		t.Errorf("content: got %q", string(data))
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("HandleTunnel error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("HandleTunnel did not return")
	}
}

// TestHandleTunnel_InvalidURL verifies that an invalid URL returns an error.
func TestHandleTunnel_InvalidURL(t *testing.T) {
	err := HandleTunnel("http://invalid-scheme.example.com", "tid", t.TempDir(), "")
	if err == nil {
		t.Fatal("expected error for http:// scheme")
	}
	if !strings.Contains(err.Error(), "unsupported URL scheme") {
		t.Errorf("unexpected error: %v", err)
	}
}
