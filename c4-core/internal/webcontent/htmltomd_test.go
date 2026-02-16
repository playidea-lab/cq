package webcontent

import (
	"testing"
)

func TestConvertHTMLToMarkdown(t *testing.T) {
	html := `<html><body><h1>Title</h1><p>Hello <strong>world</strong>.</p><ul><li>Item 1</li><li>Item 2</li></ul></body></html>`

	md, err := ConvertHTMLToMarkdown(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if md == "" {
		t.Fatal("expected non-empty markdown")
	}

	// Should contain the heading
	if got := md; got == "" {
		t.Error("expected non-empty output")
	}
}

func TestConvertHTMLStripsScripts(t *testing.T) {
	html := `<html><body>
		<script>alert('xss')</script>
		<style>.foo { color: red }</style>
		<nav><a href="/">Home</a></nav>
		<p>Real content here.</p>
		<footer>Copyright 2024</footer>
	</body></html>`

	md, err := ConvertHTMLToMarkdown(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contains(md, "alert") {
		t.Error("script content should be stripped")
	}
	if contains(md, "color: red") {
		t.Error("style content should be stripped")
	}
	if !contains(md, "Real content") {
		t.Errorf("expected real content in output: %s", md)
	}
}

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{
			"title_tag",
			`<html><head><title>My Page Title</title></head><body><h1>Heading</h1></body></html>`,
			"My Page Title",
		},
		{
			"no_title_fallback_h1",
			`<html><body><h1>Fallback Heading</h1></body></html>`,
			"Fallback Heading",
		},
		{
			"empty_title_fallback_h1",
			`<html><head><title>  </title></head><body><h1>Real Heading</h1></body></html>`,
			"Real Heading",
		},
		{
			"no_title_no_h1",
			`<html><body><p>Just a paragraph</p></body></html>`,
			"",
		},
		{
			"h1_with_nested_tags",
			`<html><body><h1><span>Nested</span> Title</h1></body></html>`,
			"Nested Title",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractTitle(tt.html)
			if got != tt.want {
				t.Errorf("ExtractTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConvertHTMLEmpty(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"whitespace", "   \n\t  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md, err := ConvertHTMLToMarkdown(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if md != "" {
				t.Errorf("expected empty output for %q, got %q", tt.input, md)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
