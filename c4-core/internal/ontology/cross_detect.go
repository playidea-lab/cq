package ontology

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// CrossLLMClient is a minimal interface for the LLM used by CrossPositionDetector.
// It is satisfied by any client that can complete a prompt (e.g. the llm.Gateway
// or a test stub).
type CrossLLMClient interface {
	// Complete sends a prompt and returns the raw completion text.
	Complete(ctx context.Context, prompt string) (string, error)
}

// CrossPositionDetector detects cross-position feedback in a behavior summary
// and tags nodes with scope "cross:{src}→{tgt}".
type CrossPositionDetector struct {
	llm CrossLLMClient
}

// NewCrossPositionDetector creates a detector backed by the given LLM client.
func NewCrossPositionDetector(llm CrossLLMClient) *CrossPositionDetector {
	return &CrossPositionDetector{llm: llm}
}

// crossPrompt builds the Haiku-style prompt for extracting source/target roles.
func crossPrompt(summary string) string {
	var sb strings.Builder
	sb.WriteString("Analyze the following behavior summary and detect cross-position feedback.\n")
	sb.WriteString("Cross-position feedback occurs when knowledge or feedback flows from one role to another\n")
	sb.WriteString("(e.g. frontend developer providing feedback to backend, researcher giving input to engineer).\n\n")
	sb.WriteString("Return a JSON object with fields:\n")
	sb.WriteString("  source_role: the role providing the feedback (e.g. \"frontend\", \"researcher\", \"designer\")\n")
	sb.WriteString("  target_role: the role receiving the feedback (e.g. \"backend\", \"engineer\", \"product\")\n\n")
	sb.WriteString("If no cross-position feedback is detected, return {\"source_role\": \"\", \"target_role\": \"\"}.\n")
	sb.WriteString("Return only valid JSON. Example: {\"source_role\": \"frontend\", \"target_role\": \"backend\"}\n\n")
	sb.WriteString("Behavior summary:\n")
	sb.WriteString(summary)
	return sb.String()
}

// crossRoles holds the extracted role pair from the LLM response.
type crossRoles struct {
	SourceRole string `json:"source_role"`
	TargetRole string `json:"target_role"`
}

// parseCrossRoles attempts to unmarshal the LLM JSON response into a crossRoles pair.
// Returns empty crossRoles on failure.
func parseCrossRoles(raw string) crossRoles {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return crossRoles{}
	}
	jsonPart := raw[start : end+1]

	var roles crossRoles
	if err := json.Unmarshal([]byte(jsonPart), &roles); err != nil {
		log.Printf("ontology: parseCrossRoles: unmarshal failed: %v (snippet: %.100s)", err, jsonPart)
		return crossRoles{}
	}
	return roles
}

// crossScope builds the scope tag from a role pair.
// Returns empty string if either role is missing.
func crossScope(src, tgt string) string {
	src = strings.TrimSpace(src)
	tgt = strings.TrimSpace(tgt)
	if src == "" || tgt == "" {
		return ""
	}
	return fmt.Sprintf("cross:%s→%s", src, tgt)
}

// DetectResult holds the output of a cross-position detection.
type DetectResult struct {
	// SourceRole is the role that provides the cross-position knowledge.
	SourceRole string
	// TargetRole is the role that receives or is the subject of the knowledge.
	TargetRole string
	// Scope is the formatted scope tag "cross:{src}→{tgt}", or empty if no
	// cross-position feedback was detected.
	Scope string
	// Detected is true when a valid cross-position pair was found.
	Detected bool
}

// Detect calls the LLM to extract source/target roles from the given behavior
// summary. On LLM failure or when no cross-position pair is found, Detected is
// false and Scope is empty.
func (d *CrossPositionDetector) Detect(ctx context.Context, summary string) (DetectResult, error) {
	if strings.TrimSpace(summary) == "" {
		return DetectResult{}, nil
	}

	prompt := crossPrompt(summary)
	raw, err := d.llm.Complete(ctx, prompt)
	if err != nil {
		log.Printf("ontology: CrossPositionDetector: llm failed: %v", err)
		return DetectResult{}, nil
	}

	roles := parseCrossRoles(raw)
	scope := crossScope(roles.SourceRole, roles.TargetRole)
	if scope == "" {
		return DetectResult{}, nil
	}

	return DetectResult{
		SourceRole: roles.SourceRole,
		TargetRole: roles.TargetRole,
		Scope:      scope,
		Detected:   true,
	}, nil
}

// TagNode applies the cross-position scope and source/target roles to a Node.
// If the result has no detected cross-position, the node is returned unchanged.
func TagNode(node Node, result DetectResult) Node {
	if !result.Detected {
		return node
	}
	node.Scope = result.Scope
	node.SourceRole = result.SourceRole
	if node.Properties == nil {
		node.Properties = make(map[string]string)
	}
	node.Properties["target_role"] = result.TargetRole
	return node
}
