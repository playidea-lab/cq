package cloud

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListMembers_Success(t *testing.T) {
	// Track request count to serve different responses.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "c4_project_members") {
			// Request 1: return member rows.
			rows := []teamMemberRow{
				{UserID: "uid-001", Role: "admin"},
				{UserID: "uid-002", Role: "member"},
			}
			json.NewEncoder(w).Encode(rows) //nolint:errcheck
			return
		}

		if strings.Contains(r.URL.Path, "c4_profiles") {
			// Verify user_id=in.(...) query param.
			q := r.URL.RawQuery
			if !strings.Contains(q, "user_id=in.") {
				http.Error(w, "missing user_id=in. filter", http.StatusBadRequest)
				return
			}
			// Request 2: return profile rows.
			profiles := []profileRow{
				{UserID: "uid-001", Email: "alice@example.com", DisplayName: "Alice"},
				{UserID: "uid-002", Email: "bob@example.com", DisplayName: ""},
			}
			json.NewEncoder(w).Encode(profiles) //nolint:errcheck
			return
		}

		http.Error(w, "unexpected path", http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewTeamClient(srv.URL, "anon-key", "access-token")
	members, err := client.ListMembers("proj-123")
	if err != nil {
		t.Fatalf("ListMembers returned error: %v", err)
	}

	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls (2-request pattern), got %d", callCount)
	}

	// Verify alice.
	alice := members[0]
	if alice.UserID != "uid-001" {
		t.Errorf("expected UserID uid-001, got %q", alice.UserID)
	}
	if alice.Email != "alice@example.com" {
		t.Errorf("expected alice@example.com, got %q", alice.Email)
	}
	if alice.DisplayName != "Alice" {
		t.Errorf("expected DisplayName Alice, got %q", alice.DisplayName)
	}
	if alice.Role != "admin" {
		t.Errorf("expected role admin, got %q", alice.Role)
	}

	// Verify bob: DisplayName falls back to email when empty.
	bob := members[1]
	if bob.Email != "bob@example.com" {
		t.Errorf("expected bob@example.com, got %q", bob.Email)
	}
	if bob.DisplayName != "bob@example.com" {
		t.Errorf("expected DisplayName fallback to email, got %q", bob.DisplayName)
	}
}

func TestListMembers_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return empty members array.
		w.Write([]byte("[]")) //nolint:errcheck
	}))
	defer srv.Close()

	client := NewTeamClient(srv.URL, "anon-key", "access-token")
	members, err := client.ListMembers("proj-empty")
	if err != nil {
		t.Fatalf("ListMembers returned error: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("expected 0 members, got %d", len(members))
	}
}

func TestInviteOrAdd_Added(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "expected POST", http.StatusMethodNotAllowed)
			return
		}
		if !strings.Contains(r.URL.Path, "c4_invite_or_pend") {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		// Verify body contains expected fields.
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
		if body["p_project_id"] == "" || body["p_email"] == "" {
			http.Error(w, "missing required fields", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode("added") //nolint:errcheck
	}))
	defer srv.Close()

	client := NewTeamClient(srv.URL, "anon-key", "access-token")
	result, err := client.InviteOrAdd("proj-123", "newuser@example.com")
	if err != nil {
		t.Fatalf("InviteOrAdd returned error: %v", err)
	}
	if result.Status != "added" {
		t.Errorf("expected status 'added', got %q", result.Status)
	}
}

func TestInviteOrAdd_Invited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode("invited") //nolint:errcheck
	}))
	defer srv.Close()

	client := NewTeamClient(srv.URL, "anon-key", "access-token")
	result, err := client.InviteOrAdd("proj-123", "unknown@example.com")
	if err != nil {
		t.Fatalf("InviteOrAdd returned error: %v", err)
	}
	if result.Status != "invited" {
		t.Errorf("expected status 'invited', got %q", result.Status)
	}
}

func TestRemoveMember_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "expected DELETE", http.StatusMethodNotAllowed)
			return
		}
		// Verify query params contain project_id and user_id filters.
		q := r.URL.RawQuery
		if !strings.Contains(q, "project_id=eq.") {
			http.Error(w, "missing project_id filter", http.StatusBadRequest)
			return
		}
		if !strings.Contains(q, "user_id=eq.") {
			http.Error(w, "missing user_id filter", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewTeamClient(srv.URL, "anon-key", "access-token")
	err := client.RemoveMember("proj-123", "uid-001")
	if err != nil {
		t.Fatalf("RemoveMember returned error: %v", err)
	}
}
