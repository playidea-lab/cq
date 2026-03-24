package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/cloud"
)

// relayProxyHandler returns an http.Handler that proxies requests from
// /w/{worker}/mcp to the relay server with auto-injected JWT tokens.
//
// This eliminates the need for Bearer tokens in .mcp.json — Claude Code
// talks to localhost (no auth), and this proxy adds the fresh JWT from
// TokenProvider before forwarding to the relay.
func relayProxyHandler(cloudTP *cloud.TokenProvider, relayURL, anonKey string) http.Handler {
	// Normalize relay URL: wss:// → https://, ws:// → http://
	httpURL := strings.Replace(relayURL, "wss://", "https://", 1)
	httpURL = strings.Replace(httpURL, "ws://", "http://", 1)

	client := &http.Client{Timeout: 60 * time.Second}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract /w/{worker}/mcp from path
		// Path format: /w/pi-System-Product-Name/mcp
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		// Forward the entire path to relay
		targetURL := httpURL + r.URL.Path

		// Read request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// Create forwarding request
		proxyReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, targetURL, strings.NewReader(string(body)))
		if err != nil {
			http.Error(w, fmt.Sprintf("proxy request: %v", err), http.StatusInternalServerError)
			return
		}
		proxyReq.Header.Set("Content-Type", "application/json")

		// Inject fresh JWT from TokenProvider
		if cloudTP != nil {
			token := cloudTP.Token()
			if token != "" {
				proxyReq.Header.Set("Authorization", "Bearer "+token)
			}
		}

		// Inject Supabase anon key (required by relay for apikey validation)
		if anonKey != "" {
			proxyReq.Header.Set("apikey", anonKey)
		}

		// Forward to relay
		resp, err := client.Do(proxyReq)
		if err != nil {
			http.Error(w, fmt.Sprintf("relay error: %v", err), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Copy response headers
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})
}
