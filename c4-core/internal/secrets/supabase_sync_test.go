package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestSupabaseSync creates a SupabaseSync pointed at the given test server.
func newTestSupabaseSync(ts *httptest.Server) *SupabaseSync {
	return &SupabaseSync{
		baseURL:    ts.URL,
		anonKey:    "test-anon-key",
		httpClient: ts.Client(),
	}
}

func TestSupabaseSync_Set_OK(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("apikey") == "" {
			t.Error("missing apikey header")
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()

	s := newTestSupabaseSync(ts)
	if err := s.Set(context.Background(), "proj1", "anthropic.api_key", "sk-test"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !called {
		t.Error("handler not called")
	}
}

func TestSupabaseSync_Set_Error(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"internal error"}`))
	}))
	defer ts.Close()

	s := newTestSupabaseSync(ts)
	err := s.Set(context.Background(), "proj1", "key", "val")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSupabaseSync_Get_OK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rows := []map[string]string{{"ciphertext": "sk-ant-value"}}
		json.NewEncoder(w).Encode(rows)
	}))
	defer ts.Close()

	s := newTestSupabaseSync(ts)
	val, err := s.Get(context.Background(), "proj1", "anthropic.api_key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "sk-ant-value" {
		t.Errorf("val = %q, want %q", val, "sk-ant-value")
	}
}

func TestSupabaseSync_Get_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]string{}) // empty array
	}))
	defer ts.Close()

	s := newTestSupabaseSync(ts)
	_, err := s.Get(context.Background(), "proj1", "missing.key")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSupabaseSync_ListKeys_OK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rows := []map[string]string{{"key": "a"}, {"key": "b"}}
		json.NewEncoder(w).Encode(rows)
	}))
	defer ts.Close()

	s := newTestSupabaseSync(ts)
	keys, err := s.ListKeys(context.Background(), "proj1")
	if err != nil {
		t.Fatalf("ListKeys: %v", err)
	}
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "b" {
		t.Errorf("keys = %v, want [a b]", keys)
	}
}

func TestSupabaseSync_Delete_OK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	s := newTestSupabaseSync(ts)
	if err := s.Delete(context.Background(), "proj1", "some.key"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestNewSupabaseSyncFromConfig_NotConfigured(t *testing.T) {
	_, err := NewSupabaseSyncFromConfig("", "")
	if !errors.Is(err, ErrSupabaseNotConfigured) {
		t.Errorf("expected ErrSupabaseNotConfigured, got %v", err)
	}
}

func TestNewSupabaseSyncFromConfig_OK(t *testing.T) {
	sync, err := NewSupabaseSyncFromConfig("https://x.supabase.co", "key123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sync == nil {
		t.Error("expected non-nil SupabaseSync")
	}
}
