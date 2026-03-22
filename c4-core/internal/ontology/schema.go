// Package ontology defines Go types for the C4 ontology schema and provides
// YAML-based persistence at ~/.c4/personas/{username}/ontology.yaml.
package ontology

import "time"

// Confidence represents the trust level of an ontology node.
type Confidence string

const (
	ConfidenceLow      Confidence = "low"
	ConfidenceMedium   Confidence = "medium"
	ConfidenceHigh     Confidence = "high"
	ConfidenceVerified Confidence = "verified"
)

// PromotionThreshold is the frequency count at which a node is auto-promoted to HIGH confidence.
const PromotionThreshold = 3

// Node represents a named concept in the ontology with associated metadata.
type Node struct {
	// Label is a human-readable name for the concept.
	Label string `yaml:"label"`
	// Description explains what the concept means in context.
	Description string `yaml:"description,omitempty"`
	// Tags provide free-form categorization.
	Tags []string `yaml:"tags,omitempty"`
	// Properties holds arbitrary key-value pairs for extensibility.
	Properties map[string]string `yaml:"properties,omitempty"`
	// Frequency tracks how many times this node has been observed.
	Frequency int `yaml:"frequency,omitempty"`
	// NodeConfidence indicates the trust level (low, medium, high).
	NodeConfidence Confidence `yaml:"confidence,omitempty"`
	// Scope narrows the context in which this node is relevant (e.g. "project", "global").
	Scope string `yaml:"scope,omitempty"`
	// SourceRole identifies the role or origin that introduced this node (e.g. "user", "agent").
	SourceRole string `yaml:"source_role,omitempty"`
}

// ProjectOntology is the top-level structure persisted to .c4/project-ontology.yaml.
// It reuses the same schema as Ontology to remain backward-compatible with L1.
type ProjectOntology struct {
	// Version is a semantic version string for schema evolution.
	Version string `yaml:"version"`
	// UpdatedAt records the last modification time.
	UpdatedAt time.Time `yaml:"updated_at"`
	// Schema contains the core concept definitions.
	Schema CoreSchema `yaml:"schema"`
}

// CoreSchema holds the named concept nodes that make up the ontology.
type CoreSchema struct {
	// Nodes maps concept identifiers to their Node definitions.
	Nodes map[string]Node `yaml:"nodes,omitempty"`
}

// Ontology is the top-level structure persisted to YAML.
type Ontology struct {
	// Version is a semantic version string for schema evolution.
	Version string `yaml:"version"`
	// UpdatedAt records the last modification time.
	UpdatedAt time.Time `yaml:"updated_at"`
	// Schema contains the core concept definitions.
	Schema CoreSchema `yaml:"schema"`
}
