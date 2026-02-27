package cloud

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// GeneratePKCE generates a PKCE code_verifier and code_challenge pair.
//
// The verifier is a cryptographically random base64url string (43–128 chars)
// and the challenge is the base64url(SHA-256(verifier)) per RFC 7636.
func GeneratePKCE() (verifier, challenge string, err error) {
	// 32 random bytes → 43 base64url chars (well within 43–128 range)
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generating PKCE verifier: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)

	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}
