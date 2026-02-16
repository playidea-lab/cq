package cdp

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// DefaultDebugURL is the fallback CDP debug URL.
const DefaultDebugURL = "http://localhost:9222"

// commonPorts are the ports to probe when auto-discovering Chrome CDP.
var commonPorts = []int{9222, 9229, 9223, 9224}

// DiscoverCDPURL attempts to find a running Chrome CDP endpoint.
// Priority: 1) explicit override, 2) CDP_DEBUG_URL env, 3) DevToolsActivePort file, 4) port scan.
func DiscoverCDPURL(explicit string) string {
	if explicit != "" {
		return explicit
	}

	if env := os.Getenv("CDP_DEBUG_URL"); env != "" {
		return env
	}

	if port := readDevToolsActivePort(); port > 0 {
		url := fmt.Sprintf("http://localhost:%d", port)
		if probeCDP(url) {
			return url
		}
	}

	for _, port := range commonPorts {
		url := fmt.Sprintf("http://localhost:%d", port)
		if probeCDP(url) {
			return url
		}
	}

	return DefaultDebugURL
}

// readDevToolsActivePort reads Chrome's DevToolsActivePort file to find the debug port.
// Chrome writes this file when started with --remote-debugging-port=0 (auto-assign).
func readDevToolsActivePort() int {
	paths := devToolsActivePortPaths()
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
		if len(lines) == 0 {
			continue
		}
		port, err := strconv.Atoi(strings.TrimSpace(lines[0]))
		if err != nil || port <= 0 || port > 65535 {
			continue
		}
		return port
	}
	return 0
}

// devToolsActivePortPaths returns platform-specific paths for Chrome's DevToolsActivePort file.
func devToolsActivePortPaths() []string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return nil
	}

	switch runtime.GOOS {
	case "darwin":
		return []string{
			filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "DevToolsActivePort"),
			filepath.Join(home, "Library", "Application Support", "Google", "Chrome Canary", "DevToolsActivePort"),
			filepath.Join(home, "Library", "Application Support", "Chromium", "DevToolsActivePort"),
		}
	case "linux":
		return []string{
			filepath.Join(home, ".config", "google-chrome", "DevToolsActivePort"),
			filepath.Join(home, ".config", "google-chrome-unstable", "DevToolsActivePort"),
			filepath.Join(home, ".config", "chromium", "DevToolsActivePort"),
		}
	default:
		return nil
	}
}

// probeCDP checks if a CDP endpoint is responsive at the given URL.
func probeCDP(baseURL string) bool {
	client := &http.Client{
		Timeout: 500 * time.Millisecond,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: 300 * time.Millisecond}).DialContext,
		},
	}
	resp, err := client.Get(baseURL + "/json/version")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
