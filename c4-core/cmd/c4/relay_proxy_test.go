package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/cloud"
)

func TestRelayProxy_ForwardsRequest(t *testing.T) {
	// Mock relay server
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/w/test-worker/mcp" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"jsonrpc":"2.0","id":1}` {
			t.Errorf("unexpected body: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":"ok"}`))
	}))
	defer relay.Close()

	tp := cloud.NewStaticTokenProvider("")
	handler := relayProxyHandler(tp, relay.URL, "test-anon-key")

	req := httptest.NewRequest(http.MethodPost, "/w/test-worker/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != `{"result":"ok"}` {
		t.Errorf("body = %s", body)
	}
}

func TestRelayProxy_InjectsToken(t *testing.T) {
	var gotAuth, gotApikey string
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotApikey = r.Header.Get("apikey")
		w.WriteHeader(http.StatusOK)
	}))
	defer relay.Close()

	tp := cloud.NewStaticTokenProvider("my-jwt-token")
	handler := relayProxyHandler(tp, relay.URL, "my-anon-key")

	req := httptest.NewRequest(http.MethodPost, "/w/worker/mcp", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if gotAuth != "Bearer my-jwt-token" {
		t.Errorf("Authorization = %q, want 'Bearer my-jwt-token'", gotAuth)
	}
	if gotApikey != "my-anon-key" {
		t.Errorf("apikey = %q, want 'my-anon-key'", gotApikey)
	}
}

func TestRelayProxy_RejectsNonPOST(t *testing.T) {
	handler := relayProxyHandler(nil, "http://unused", "")

	req := httptest.NewRequest(http.MethodGet, "/w/worker/mcp", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestRelayProxy_RelayDown(t *testing.T) {
	// Point to a server that immediately closes
	handler := relayProxyHandler(nil, "http://127.0.0.1:1", "")

	req := httptest.NewRequest(http.MethodPost, "/w/worker/mcp", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}
