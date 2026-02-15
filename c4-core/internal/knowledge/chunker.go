package knowledge

import (
	"strings"
	"unicode/utf8"
)

// Chunk represents a text segment from a larger document.
type Chunk struct {
	Index       int    `json:"index"`
	Body        string `json:"body"`
	StartOffset int    `json:"start_offset"`
}

// ChunkText splits text into chunks of approximately maxTokens tokens.
// Uses paragraph boundaries when possible, with overlap for context continuity.
// Token estimation: 1 token ≈ 4 characters (conservative for English/mixed).
func ChunkText(text string, maxTokens int) []Chunk {
	if maxTokens <= 0 {
		maxTokens = 512
	}
	maxChars := maxTokens * 4
	overlapChars := 50 * 4 // ~50 token overlap

	if utf8.RuneCountInString(text) <= maxChars {
		return []Chunk{{Index: 0, Body: text, StartOffset: 0}}
	}

	paragraphs := splitParagraphs(text)

	var chunks []Chunk
	var current strings.Builder
	currentStart := 0
	offset := 0

	for _, para := range paragraphs {
		paraLen := utf8.RuneCountInString(para)

		// If single paragraph exceeds max, split it by sentences
		if paraLen > maxChars {
			// Flush current buffer first
			if current.Len() > 0 {
				chunks = append(chunks, Chunk{
					Index:       len(chunks),
					Body:        strings.TrimSpace(current.String()),
					StartOffset: currentStart,
				})
				current.Reset()
			}

			// Split large paragraph
			subChunks := splitLargeParagraph(para, maxChars, overlapChars, offset, len(chunks))
			chunks = append(chunks, subChunks...)
			offset += paraLen + 1 // +1 for separator
			currentStart = offset
			continue
		}

		// Would adding this paragraph exceed the limit?
		if utf8.RuneCountInString(current.String())+paraLen+1 > maxChars && current.Len() > 0 {
			// Flush current chunk
			chunks = append(chunks, Chunk{
				Index:       len(chunks),
				Body:        strings.TrimSpace(current.String()),
				StartOffset: currentStart,
			})

			// Start new chunk with overlap from end of previous
			current.Reset()
			prevBody := chunks[len(chunks)-1].Body
			if utf8.RuneCountInString(prevBody) > overlapChars {
				overlap := prevBody[len(prevBody)-overlapChars*1:] // byte approx
				if idx := strings.LastIndex(overlap, " "); idx > 0 {
					overlap = overlap[idx+1:]
				}
				current.WriteString(overlap)
				current.WriteString("\n\n")
			}
			currentStart = offset
		}

		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(para)
		offset += paraLen + 1
	}

	// Final chunk
	if current.Len() > 0 {
		body := strings.TrimSpace(current.String())
		if body != "" {
			chunks = append(chunks, Chunk{
				Index:       len(chunks),
				Body:        body,
				StartOffset: currentStart,
			})
		}
	}

	return chunks
}

// splitParagraphs splits text on double newlines.
func splitParagraphs(text string) []string {
	raw := strings.Split(text, "\n\n")
	var result []string
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// splitLargeParagraph breaks an oversized paragraph by sentence boundaries.
func splitLargeParagraph(text string, maxChars, overlapChars, baseOffset, startIndex int) []Chunk {
	sentences := splitSentences(text)
	var chunks []Chunk
	var current strings.Builder
	currentStart := baseOffset

	for _, sent := range sentences {
		sentLen := utf8.RuneCountInString(sent)

		if utf8.RuneCountInString(current.String())+sentLen+1 > maxChars && current.Len() > 0 {
			chunks = append(chunks, Chunk{
				Index:       startIndex + len(chunks),
				Body:        strings.TrimSpace(current.String()),
				StartOffset: currentStart,
			})
			current.Reset()
			currentStart = baseOffset + len(chunks)*maxChars // approximate
		}

		if current.Len() > 0 {
			current.WriteString(" ")
		}
		current.WriteString(sent)
	}

	if current.Len() > 0 {
		chunks = append(chunks, Chunk{
			Index:       startIndex + len(chunks),
			Body:        strings.TrimSpace(current.String()),
			StartOffset: currentStart,
		})
	}

	return chunks
}

// splitSentences does basic sentence splitting on ". ", "! ", "? ".
func splitSentences(text string) []string {
	var sentences []string
	var current strings.Builder

	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		current.WriteRune(runes[i])
		if (runes[i] == '.' || runes[i] == '!' || runes[i] == '?') &&
			i+1 < len(runes) && runes[i+1] == ' ' {
			sentences = append(sentences, strings.TrimSpace(current.String()))
			current.Reset()
		}
	}

	if current.Len() > 0 {
		sentences = append(sentences, strings.TrimSpace(current.String()))
	}

	return sentences
}
