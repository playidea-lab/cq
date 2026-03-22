// Package ontology defines Go types for the C4 ontology schema and provides
// YAML-based persistence at ~/.c4/personas/{username}/ontology.yaml.
package ontology

import "time"

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
