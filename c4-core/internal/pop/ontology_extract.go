package pop

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/changmin/c4-core/internal/ontology"
	"github.com/changmin/c4-core/internal/persona"
)

// OntologyExtractor extracts ontology nodes from a behavior summary using
// an LLM (Haiku) with a rule-based fallback on failure.
type OntologyExtractor struct {
	llm LLMClient
}

// NewOntologyExtractor creates an extractor backed by the given LLM client.
func NewOntologyExtractor(llm LLMClient) *OntologyExtractor {
	return &OntologyExtractor{llm: llm}
}

// ontologyPrompt builds the LLM prompt for extracting ontology nodes.
func ontologyPrompt(summary string) string {
	var sb strings.Builder
	sb.WriteString("Extract ontology concept nodes from the following behavior summary.\n")
	sb.WriteString("Return a JSON array of objects with fields: label, description, tags (string array).\n")
	sb.WriteString("Each node should represent a distinct concept, skill, or behavior pattern observed.\n")
	sb.WriteString("Return only valid JSON. Example: [{\"label\":\"Input Validation\",\"description\":\"Always validates user input.\",\"tags\":[\"reliability\",\"security\"]}]\n\n")
	sb.WriteString("Behavior summary:\n")
	sb.WriteString(summary)
	return sb.String()
}

// parseOntologyNodes attempts to unmarshal an LLM JSON response into ontology nodes.
func parseOntologyNodes(raw string) []ontology.Node {
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start < 0 || end <= start {
		return nil
	}
	jsonPart := raw[start : end+1]

	var items []struct {
		Label       string   `json:"label"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}
	if err := json.Unmarshal([]byte(jsonPart), &items); err != nil {
		log.Printf("pop: parseOntologyNodes: unmarshal failed: %v (snippet: %.100s)", err, jsonPart)
		return nil
	}

	nodes := make([]ontology.Node, 0, len(items))
	for _, it := range items {
		if it.Label == "" {
			continue
		}
		nodes = append(nodes, ontology.Node{
			Label:       it.Label,
			Description: it.Description,
			Tags:        it.Tags,
		})
	}
	return nodes
}

// ruleBasedNodes derives ontology nodes from a behavior summary using
// persona.AnalyzeEdits patterns as a rule-based fallback.
// It treats the summary as the "edited" text (with an empty original),
// mapping each EditPattern category to a node label.
func ruleBasedNodes(summary string) []ontology.Node {
	patterns := persona.AnalyzeEdits("", summary)
	if len(patterns) == 0 {
		// Minimal fallback: one generic node from the summary itself.
		label := summary
		if len(label) > 60 {
			label = label[:60]
		}
		label = strings.TrimSpace(label)
		if label == "" {
			return nil
		}
		return []ontology.Node{{Label: label, Tags: []string{"inferred"}}}
	}

	nodes := make([]ontology.Node, 0, len(patterns))
	for _, p := range patterns {
		desc := p.Description
		if len(p.Examples) > 0 {
			desc = fmt.Sprintf("%s (e.g. %s)", p.Description, p.Examples[0])
		}
		nodes = append(nodes, ontology.Node{
			Label:       p.Category,
			Description: desc,
			Tags:        []string{"rule-based"},
		})
	}
	return nodes
}

// Extract calls the LLM to extract ontology nodes from the given behavior summary.
// If the LLM call fails or returns no nodes, it falls back to rule-based extraction.
func (e *OntologyExtractor) Extract(ctx context.Context, summary string) ([]ontology.Node, error) {
	if strings.TrimSpace(summary) == "" {
		return nil, nil
	}

	prompt := ontologyPrompt(summary)
	raw, err := e.llm.Complete(ctx, prompt)
	if err != nil {
		log.Printf("pop: OntologyExtractor: llm failed, falling back to rule-based: %v", err)
		return ruleBasedNodes(summary), nil
	}

	nodes := parseOntologyNodes(raw)
	if len(nodes) == 0 {
		log.Printf("pop: OntologyExtractor: llm returned no nodes, falling back to rule-based")
		return ruleBasedNodes(summary), nil
	}

	return nodes, nil
}
