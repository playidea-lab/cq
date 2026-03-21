package webcontent

import (
	"testing"
)

func TestParseLLMSTxt(t *testing.T) {
	input := `# My Project

> A great project for doing things

## Documentation

- [Getting Started](https://example.com/start): How to get started
- [API Reference](https://example.com/api): Full API docs

## Examples

- [Tutorial](https://example.com/tutorial): Step by step guide
`

	result, err := ParseLLMSTxt(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Title != "My Project" {
		t.Errorf("expected title 'My Project', got '%s'", result.Title)
	}
	if result.Description != "A great project for doing things" {
		t.Errorf("expected description, got '%s'", result.Description)
	}
	if len(result.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(result.Sections))
	}

	sec0 := result.Sections[0]
	if sec0.Title != "Documentation" {
		t.Errorf("expected section 'Documentation', got '%s'", sec0.Title)
	}
	if len(sec0.Links) != 2 {
		t.Fatalf("expected 2 links in section 0, got %d", len(sec0.Links))
	}
	if sec0.Links[0].Title != "Getting Started" {
		t.Errorf("expected link title 'Getting Started', got '%s'", sec0.Links[0].Title)
	}
	if sec0.Links[0].URL != "https://example.com/start" {
		t.Errorf("expected link URL, got '%s'", sec0.Links[0].URL)
	}
	if sec0.Links[0].Description != "How to get started" {
		t.Errorf("expected link description, got '%s'", sec0.Links[0].Description)
	}

	sec1 := result.Sections[1]
	if sec1.Title != "Examples" {
		t.Errorf("expected section 'Examples', got '%s'", sec1.Title)
	}
	if len(sec1.Links) != 1 {
		t.Fatalf("expected 1 link in section 1, got %d", len(sec1.Links))
	}
}

func TestParseLLMSTxtEmpty(t *testing.T) {
	result, err := ParseLLMSTxt("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Title != "" {
		t.Errorf("expected empty title, got '%s'", result.Title)
	}
	if len(result.Sections) != 0 {
		t.Errorf("expected 0 sections, got %d", len(result.Sections))
	}
}

func TestParseLLMSTxtNoDescription(t *testing.T) {
	input := `# Title Only

## Links

- [Docs](https://example.com/docs)
`

	result, err := ParseLLMSTxt(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Title != "Title Only" {
		t.Errorf("expected 'Title Only', got '%s'", result.Title)
	}
	if result.Description != "" {
		t.Errorf("expected empty description, got '%s'", result.Description)
	}
	if len(result.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(result.Sections))
	}
	if result.Sections[0].Links[0].Description != "" {
		t.Errorf("expected empty link description, got '%s'", result.Sections[0].Links[0].Description)
	}
}
