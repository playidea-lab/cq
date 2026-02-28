package knowledge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearch_Disabled(t *testing.T) {
	c := New("", "")
	results, err := c.Search(context.Background(), "proj", "query", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results when disabled, got %v", results)
	}
}

func TestSearch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apikey") == "" {
			t.Error("expected apikey header")
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("expected Authorization header")
		}
		// Verify path and basic query params.
		if !strings.Contains(r.URL.Path, "c4_documents") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]SearchResult{
			{DocID: "doc-1", Title: "Test Doc", Domain: "general", Body: "Test content"},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	results, err := c.Search(context.Background(), "proj-1", "test", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].DocID != "doc-1" {
		t.Errorf("got DocID %q, want doc-1", results[0].DocID)
	}
	if results[0].Title != "Test Doc" {
		t.Errorf("got Title %q, want Test Doc", results[0].Title)
	}
}

func TestSearch_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]SearchResult{})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	results, err := c.Search(context.Background(), "proj-1", "noresults", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestSearch_BodyTruncation(t *testing.T) {
	longBody := strings.Repeat("가", 3000) // 3000 Korean chars, exceeds maxBodyRunes=2000
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]SearchResult{
			{DocID: "doc-long", Title: "Long", Body: longBody},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key")
	results, err := c.Search(context.Background(), "proj-1", "query", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	runeCount := len([]rune(results[0].Body))
	if runeCount != maxBodyRunes {
		t.Errorf("expected body truncated to %d runes, got %d", maxBodyRunes, runeCount)
	}
}
