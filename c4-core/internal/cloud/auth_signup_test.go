package cloud

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- SignUpWithEmail tests ---

func TestSignUpWithEmail_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v1/signup" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("apikey") == "" {
			t.Error("missing apikey header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"access_token":  "tok-abc",
			"refresh_token": "ref-abc",
			"user": map[string]string{
				"id":    "uid-123",
				"email": "test@example.com",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewAuthClient(srv.URL, "anon-key")
	session, err := client.SignUpWithEmail("test@example.com", "password123")
	if err != nil {
		t.Fatalf("SignUpWithEmail error: %v", err)
	}
	if session.UserID != "uid-123" {
		t.Errorf("UserID = %q, want %q", session.UserID, "uid-123")
	}
	if session.Email != "test@example.com" {
		t.Errorf("Email = %q, want %q", session.Email, "test@example.com")
	}
	if session.AccessToken != "tok-abc" {
		t.Errorf("AccessToken = %q, want %q", session.AccessToken, "tok-abc")
	}
	if session.RefreshToken != "ref-abc" {
		t.Errorf("RefreshToken = %q, want %q", session.RefreshToken, "ref-abc")
	}
}

func TestSignUpWithEmail_EmailExists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		resp := map[string]string{
			"error_code": "email_exists",
			"msg":        "User already registered",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewAuthClient(srv.URL, "anon-key")
	_, err := client.SignUpWithEmail("existing@example.com", "password123")
	if err == nil {
		t.Fatal("expected error for duplicate email, got nil")
	}
	if !strings.Contains(err.Error(), "email already in use") {
		t.Errorf("expected 'email already in use' in error, got: %v", err)
	}
}

func TestSignUpWithEmail_WeakPassword(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		resp := map[string]string{
			"error_code": "weak_password",
			"msg":        "Password should be at least 6 characters",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewAuthClient(srv.URL, "anon-key")
	_, err := client.SignUpWithEmail("test@example.com", "abc")
	if err == nil {
		t.Fatal("expected error for weak password, got nil")
	}
	if !strings.Contains(err.Error(), "weak password") {
		t.Errorf("expected 'weak password' in error, got: %v", err)
	}
}

// --- UpsertProfile tests ---

func TestUpsertProfile_Success(t *testing.T) {
	var profileCalled, rpcCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/v1/c4_profiles":
			profileCalled = true
			if got := r.Header.Get("Prefer"); got != "resolution=merge-duplicates" {
				t.Errorf("Prefer header = %q, want %q", got, "resolution=merge-duplicates")
			}
			if r.Header.Get("Authorization") == "" {
				t.Error("missing Authorization header")
			}
			w.WriteHeader(http.StatusCreated)
		case "/rest/v1/rpc/c4_resolve_pending_invitations":
			rpcCalled = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewAuthClient(srv.URL, "anon-key")
	session := &AuthSession{
		UserID:      "uid-123",
		Email:       "test@example.com",
		AccessToken: "tok-abc",
	}
	if err := client.UpsertProfile(session); err != nil {
		t.Fatalf("UpsertProfile error: %v", err)
	}
	if !profileCalled {
		t.Error("c4_profiles endpoint was not called")
	}
	if !rpcCalled {
		t.Error("c4_resolve_pending_invitations RPC was not called")
	}
}

func TestUpsertProfile_ResolvePendingCalled(t *testing.T) {
	var rpcBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/v1/c4_profiles":
			w.WriteHeader(http.StatusCreated)
		case "/rest/v1/rpc/c4_resolve_pending_invitations":
			json.NewDecoder(r.Body).Decode(&rpcBody)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewAuthClient(srv.URL, "anon-key")
	session := &AuthSession{
		UserID:      "uid-456",
		Email:       "invite@example.com",
		AccessToken: "tok-def",
	}
	if err := client.UpsertProfile(session); err != nil {
		t.Fatalf("UpsertProfile error: %v", err)
	}
	if rpcBody["p_user_id"] != "uid-456" {
		t.Errorf("p_user_id = %q, want %q", rpcBody["p_user_id"], "uid-456")
	}
	if rpcBody["p_email"] != "invite@example.com" {
		t.Errorf("p_email = %q, want %q", rpcBody["p_email"], "invite@example.com")
	}
}

// --- patchTeamYAMLCloudUID tests ---

func TestPatchTeamYAMLCloudUID_SingleMember(t *testing.T) {
	dir := t.TempDir()
	c4dir := filepath.Join(dir, ".c4")
	if err := os.MkdirAll(c4dir, 0o700); err != nil {
		t.Fatal(err)
	}

	teamYAML := `members:
  alice:
    roles:
      - developer
    personas:
      - coder
    active_persona: coder
`
	teamPath := filepath.Join(c4dir, "team.yaml")
	if err := os.WriteFile(teamPath, []byte(teamYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := PatchTeamYAMLCloudUID(dir, "cloud-uid-abc"); err != nil {
		t.Fatalf("patchTeamYAMLCloudUID error: %v", err)
	}

	data, err := os.ReadFile(teamPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "cloud_uid") {
		t.Error("cloud_uid not found in patched team.yaml")
	}
	if !strings.Contains(content, "cloud-uid-abc") {
		t.Error("cloud_uid value not found in patched team.yaml")
	}
	// Verify existing fields are preserved.
	if !strings.Contains(content, "roles") {
		t.Error("roles field missing after patch")
	}
	if !strings.Contains(content, "personas") {
		t.Error("personas field missing after patch")
	}
	if !strings.Contains(content, "active_persona") {
		t.Error("active_persona field missing after patch")
	}
}

func TestPatchTeamYAMLCloudUID_MultiMember(t *testing.T) {
	dir := t.TempDir()
	c4dir := filepath.Join(dir, ".c4")
	if err := os.MkdirAll(c4dir, 0o700); err != nil {
		t.Fatal(err)
	}

	teamYAML := `members:
  alice:
    roles:
      - developer
  bob:
    roles:
      - designer
`
	teamPath := filepath.Join(c4dir, "team.yaml")
	if err := os.WriteFile(teamPath, []byte(teamYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	// Should be no-op (return nil) for multiple members.
	if err := PatchTeamYAMLCloudUID(dir, "cloud-uid-xyz"); err != nil {
		t.Fatalf("patchTeamYAMLCloudUID error: %v", err)
	}

	data, err := os.ReadFile(teamPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "cloud_uid") {
		t.Error("cloud_uid should NOT be written for multi-member team")
	}
}

func TestPatchTeamYAMLCloudUID_MissingFile(t *testing.T) {
	dir := t.TempDir()
	// No .c4/team.yaml exists.
	if err := PatchTeamYAMLCloudUID(dir, "cloud-uid-xyz"); err != nil {
		t.Fatalf("expected no-op for missing file, got: %v", err)
	}
}

func TestPatchTeamYAMLCloudUID_UpdateExisting(t *testing.T) {
	dir := t.TempDir()
	c4dir := filepath.Join(dir, ".c4")
	if err := os.MkdirAll(c4dir, 0o700); err != nil {
		t.Fatal(err)
	}

	teamYAML := `members:
  alice:
    cloud_uid: old-uid
    roles:
      - developer
`
	teamPath := filepath.Join(c4dir, "team.yaml")
	if err := os.WriteFile(teamPath, []byte(teamYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := PatchTeamYAMLCloudUID(dir, "new-uid-abc"); err != nil {
		t.Fatalf("patchTeamYAMLCloudUID error: %v", err)
	}

	data, err := os.ReadFile(teamPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "old-uid") {
		t.Error("old cloud_uid should be replaced")
	}
	if !strings.Contains(content, "new-uid-abc") {
		t.Error("new cloud_uid value not found")
	}
}
