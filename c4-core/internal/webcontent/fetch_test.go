package webcontent

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestFetchMarkdownNative(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Write([]byte("# Hello World\n\nThis is markdown content."))
	}))
	defer srv.Close()

	result, err := Fetch(srv.URL, &FetchOpts{SkipSSRFCheck: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Method != "markdown_native" {
		t.Errorf("expected method markdown_native, got %s", result.Method)
	}
	if !strings.Contains(result.Content, "Hello World") {
		t.Errorf("expected content to contain 'Hello World', got: %s", result.Content)
	}
	if result.Title != "Hello World" {
		t.Errorf("expected title 'Hello World', got '%s'", result.Title)
	}
}

func TestFetchHTMLConverted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>Test Page</title></head><body><h1>Hello</h1><p>Some <strong>bold</strong> text.</p></body></html>`))
	}))
	defer srv.Close()

	result, err := Fetch(srv.URL, &FetchOpts{SkipSSRFCheck: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Method != "html_converted" {
		t.Errorf("expected method html_converted, got %s", result.Method)
	}
	if result.Title != "Test Page" {
		t.Errorf("expected title 'Test Page', got '%s'", result.Title)
	}
	if !strings.Contains(result.Content, "Hello") {
		t.Errorf("expected converted content to contain 'Hello', got: %s", result.Content)
	}
}

func TestFetchWithAlternateLink(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>HTML Page</title><link rel="alternate" type="text/markdown" href="/page.md"></head><body><h1>HTML</h1></body></html>`))
	})
	mux.HandleFunc("/page.md", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		w.Write([]byte("# Markdown Version\n\nThis is the markdown version."))
	})

	result, err := Fetch(srv.URL+"/page", &FetchOpts{SkipSSRFCheck: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Method != "markdown_native" {
		t.Errorf("expected method markdown_native, got %s", result.Method)
	}
	if !strings.Contains(result.Content, "Markdown Version") {
		t.Errorf("expected markdown alt content, got: %s", result.Content)
	}
}

func TestFetchSSRFBlocked(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"loopback", "http://127.0.0.1/test"},
		{"private_10", "http://10.0.0.1/test"},
		{"private_172", "http://172.16.0.1/test"},
		{"private_192", "http://192.168.1.1/test"},
		{"localhost", "http://localhost/test"},
		{"ipv6_loopback", "http://[::1]/test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Fetch(tt.url, nil)
			if err == nil {
				t.Errorf("expected SSRF error for %s, got nil", tt.url)
			}
			if !strings.Contains(err.Error(), "SSRF blocked") {
				t.Errorf("expected SSRF blocked error, got: %v", err)
			}
		})
	}
}

func TestFetchBodyLimit(t *testing.T) {
	// Create a response larger than 1KB limit
	bigContent := strings.Repeat("x", 2000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(bigContent))
	}))
	defer srv.Close()

	result, err := Fetch(srv.URL, &FetchOpts{MaxBodyBytes: 1024, SkipSSRFCheck: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Content) > 1024 {
		t.Errorf("expected body truncated to 1024 bytes, got %d", len(result.Content))
	}
}

func TestFetchTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Write([]byte("late"))
	}))
	defer srv.Close()

	_, err := Fetch(srv.URL, &FetchOpts{Timeout: 100 * time.Millisecond, SkipSSRFCheck: true})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestFetchPlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Just plain text content"))
	}))
	defer srv.Close()

	result, err := Fetch(srv.URL, &FetchOpts{SkipSSRFCheck: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Method != "plain" {
		t.Errorf("expected method plain, got %s", result.Method)
	}
	if result.Content != "Just plain text content" {
		t.Errorf("unexpected content: %s", result.Content)
	}
}

func TestTokenEstimate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(strings.Repeat("a", 400)))
	}))
	defer srv.Close()

	result, err := Fetch(srv.URL, &FetchOpts{SkipSSRFCheck: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := 400 / 4
	if result.TokenEstimate != expected {
		t.Errorf("expected token estimate %d, got %d", expected, result.TokenEstimate)
	}
}

func TestFetchInvalidScheme(t *testing.T) {
	_, err := Fetch("ftp://example.com/file", nil)
	if err == nil {
		t.Fatal("expected error for ftp scheme")
	}
	if !strings.Contains(err.Error(), "unsupported scheme") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFetchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := Fetch(srv.URL, &FetchOpts{SkipSSRFCheck: true})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 error, got: %v", err)
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{"valid_https", "https://example.com", false},
		{"valid_http", "http://example.com", false},
		{"ftp_blocked", "ftp://example.com", true},
		{"file_blocked", "file:///etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, _ := parseTestURL(tt.rawURL)
			err := validateURL(u)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateURL(%s) error = %v, wantErr %v", tt.rawURL, err, tt.wantErr)
			}
		})
	}
}

func parseTestURL(rawURL string) (*url.URL, error) {
	return url.Parse(rawURL)
}

func TestExtractMarkdownTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"# Hello\nContent", "Hello"},
		{"## Only H2\nContent", ""},
		{"No heading", ""},
		{"Content\n# Late Title\nMore", "Late Title"},
	}

	for _, tt := range tests {
		got := extractMarkdownTitle(tt.input)
		if got != tt.want {
			t.Errorf("extractMarkdownTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFindAlternateMarkdown(t *testing.T) {
	base, _ := url.Parse("https://example.com/page")

	tests := []struct {
		name string
		html string
		want string
	}{
		{
			"found",
			`<link rel="alternate" type="text/markdown" href="/page.md">`,
			"https://example.com/page.md",
		},
		{
			"reversed_attrs",
			`<link type="text/markdown" rel="alternate" href="/doc.md">`,
			"https://example.com/doc.md",
		},
		{
			"not_found",
			`<link rel="stylesheet" href="/style.css">`,
			"",
		},
		{
			"absolute_url",
			`<link rel="alternate" type="text/markdown" href="https://cdn.example.com/page.md">`,
			"https://cdn.example.com/page.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findAlternateMarkdown(tt.html, base)
			if got != tt.want {
				t.Errorf("findAlternateMarkdown() = %q, want %q", got, tt.want)
			}
		})
	}
}
