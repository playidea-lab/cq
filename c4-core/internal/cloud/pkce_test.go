package cloud

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

// TestGeneratePKCE_ValidLength verifies verifier is 43–128 chars and challenge is non-empty.
func TestGeneratePKCE_ValidLength(t *testing.T) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() error: %v", err)
	}
	if len(verifier) < 43 || len(verifier) > 128 {
		t.Errorf("verifier length = %d, want 43–128", len(verifier))
	}
	if len(challenge) == 0 {
		t.Error("challenge is empty")
	}
}

// TestGeneratePKCE_ChallengeVerification confirms challenge == base64url(SHA-256(verifier)).
func TestGeneratePKCE_ChallengeVerification(t *testing.T) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() error: %v", err)
	}
	sum := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(sum[:])
	if challenge != expected {
		t.Errorf("challenge = %q, want %q", challenge, expected)
	}
}

// TestGeneratePKCE_Uniqueness ensures two calls produce distinct verifier/challenge pairs.
func TestGeneratePKCE_Uniqueness(t *testing.T) {
	v1, c1, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() first call error: %v", err)
	}
	v2, c2, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() second call error: %v", err)
	}
	if v1 == v2 {
		t.Error("verifiers should be unique across calls")
	}
	if c1 == c2 {
		t.Error("challenges should be unique across calls")
	}
}
