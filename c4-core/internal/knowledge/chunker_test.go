package knowledge

import (
	"strings"
	"testing"
)

func TestChunkTextSmall(t *testing.T) {
	text := "Hello world, this is a short document."
	chunks := ChunkText(text, 512)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for short text, got %d", len(chunks))
	}
	if chunks[0].Body != text {
		t.Errorf("body mismatch: got %q", chunks[0].Body)
	}
	if chunks[0].Index != 0 {
		t.Error("first chunk index should be 0")
	}
}

func TestChunkTextMultiple(t *testing.T) {
	// Build a document that's ~3x the chunk size
	var parts []string
	for i := 0; i < 30; i++ {
		parts = append(parts, "This is paragraph number "+strings.Repeat("x", 200)+".")
	}
	text := strings.Join(parts, "\n\n")

	chunks := ChunkText(text, 256) // ~1024 chars per chunk

	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}

	// All chunks should have content
	for i, c := range chunks {
		if strings.TrimSpace(c.Body) == "" {
			t.Errorf("chunk %d is empty", i)
		}
		if c.Index != i {
			t.Errorf("chunk %d: index=%d, want %d", i, c.Index, i)
		}
	}
}

func TestChunkTextEmpty(t *testing.T) {
	chunks := ChunkText("", 512)
	if len(chunks) != 1 {
		t.Fatalf("empty text: got %d chunks", len(chunks))
	}
}

func TestChunkTextDefaultMaxTokens(t *testing.T) {
	chunks := ChunkText("short text", 0)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk with default maxTokens, got %d", len(chunks))
	}
}

func TestSplitParagraphs(t *testing.T) {
	text := "Para 1\n\nPara 2\n\n\n\nPara 3"
	parts := splitParagraphs(text)
	if len(parts) != 3 {
		t.Errorf("expected 3 paragraphs, got %d: %v", len(parts), parts)
	}
}

func TestSplitSentences(t *testing.T) {
	text := "First sentence. Second one! Third? Done."
	sentences := splitSentences(text)
	if len(sentences) != 4 {
		t.Errorf("expected 4 sentences, got %d: %v", len(sentences), sentences)
	}
}
