package ontology

import (
	"testing"
	"time"
)

func TestOntology_ZeroValue(t *testing.T) {
	var o Ontology
	if o.Version != "" {
		t.Errorf("expected empty version, got %q", o.Version)
	}
	if o.Schema.Nodes != nil {
		t.Errorf("expected nil nodes, got %v", o.Schema.Nodes)
	}
}

func TestNode_Fields(t *testing.T) {
	n := Node{
		Label:       "Agent",
		Description: "An autonomous AI worker",
		Tags:        []string{"ai", "worker"},
		Properties:  map[string]string{"color": "blue"},
	}
	if n.Label != "Agent" {
		t.Errorf("unexpected label: %s", n.Label)
	}
	if len(n.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(n.Tags))
	}
	if n.Properties["color"] != "blue" {
		t.Errorf("expected property color=blue, got %s", n.Properties["color"])
	}
}

func TestCoreSchema_Nodes(t *testing.T) {
	cs := CoreSchema{
		Nodes: map[string]Node{
			"task": {Label: "Task"},
			"plan": {Label: "Plan"},
		},
	}
	if len(cs.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(cs.Nodes))
	}
	if cs.Nodes["task"].Label != "Task" {
		t.Errorf("unexpected label for task: %s", cs.Nodes["task"].Label)
	}
}

func TestOntology_WithData(t *testing.T) {
	o := Ontology{
		Version:   "1.0.0",
		UpdatedAt: time.Now().UTC(),
		Schema: CoreSchema{
			Nodes: map[string]Node{
				"concept": {Label: "Concept", Description: "A core idea"},
			},
		},
	}
	if o.Version != "1.0.0" {
		t.Errorf("unexpected version: %s", o.Version)
	}
	if o.UpdatedAt.IsZero() {
		t.Error("expected non-zero UpdatedAt")
	}
	if _, ok := o.Schema.Nodes["concept"]; !ok {
		t.Error("expected concept node to exist")
	}
}
