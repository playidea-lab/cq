package main

import (
	"errors"
	"time"

	"github.com/changmin/c4-core/internal/cloud"
	"github.com/changmin/c4-core/internal/craft"
)

// resolveCloudCredentials returns Supabase URL, anon key, and an optional
// token function for authenticated requests.  The token function returns ""
// when no session exists (anonymous/read-only mode).
func resolveCloudCredentials() (supabaseURL, anonKey string, tokenFn craft.TokenFunc, err error) {
	supabaseURL = readCloudURL(projectDir)
	if supabaseURL == "" {
		return "", "", nil, errors.New("Supabase URL 미설정 (C4_CLOUD_URL 또는 cq auth login)")
	}
	anonKey = readCloudAnonKey(projectDir)
	if anonKey == "" {
		return "", "", nil, errors.New("Supabase anon key 미설정 (C4_CLOUD_ANON_KEY 또는 cq auth login)")
	}

	// Try to load session for authenticated access.
	// Falls back to nil tokenFn (anon) when not logged in.
	authClient := cloud.NewAuthClient(supabaseURL, anonKey)
	session, sessionErr := authClient.GetSession()
	if sessionErr != nil || session == nil || time.Now().Unix() >= session.ExpiresAt {
		// No valid session — anonymous mode (read-only).
		return supabaseURL, anonKey, nil, nil
	}

	token := session.AccessToken
	tokenFn = func() string { return token }
	return supabaseURL, anonKey, tokenFn, nil
}

// newRegistryClient creates a RegistryClient (anonymous or authenticated).
func newRegistryClient() (*craft.RegistryClient, error) {
	supabaseURL, anonKey, tokenFn, err := resolveCloudCredentials()
	if err != nil {
		return nil, err
	}
	return craft.NewRegistryClient(supabaseURL, anonKey, tokenFn), nil
}

// newRegistryClientAuthenticated creates a RegistryClient that requires
// a valid authenticated session.  Returns an error if not logged in.
func newRegistryClientAuthenticated() (*craft.RegistryClient, error) {
	supabaseURL := readCloudURL(projectDir)
	if supabaseURL == "" {
		return nil, errors.New("Supabase URL 미설정")
	}
	anonKey := readCloudAnonKey(projectDir)
	if anonKey == "" {
		return nil, errors.New("Supabase anon key 미설정")
	}

	authClient := cloud.NewAuthClient(supabaseURL, anonKey)
	session, err := authClient.GetSession()
	if err != nil || session == nil {
		return nil, errors.New("인증 필요: cq auth login을 먼저 실행하세요")
	}
	if time.Now().Unix() >= session.ExpiresAt {
		return nil, errors.New("세션 만료: cq auth login을 다시 실행하세요")
	}

	token := session.AccessToken
	tokenFn := func() string { return token }
	return craft.NewRegistryClient(supabaseURL, anonKey, tokenFn), nil
}
