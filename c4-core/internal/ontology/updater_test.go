package ontology

import "testing"

func newTestOntology() *Ontology {
	return &Ontology{Version: defaultVersion}
}

func TestUpdater_AddNew_SetsDefaults(t *testing.T) {
	o := newTestOntology()
	u := NewUpdater(o)

	n := u.AddOrUpdate("tools/git", Node{Label: "Git"})

	if n.Frequency != 1 {
		t.Errorf("expected frequency 1, got %d", n.Frequency)
	}
	if n.NodeConfidence != ConfidenceLow {
		t.Errorf("expected confidence low, got %s", n.NodeConfidence)
	}
	if n.Label != "Git" {
		t.Errorf("expected label Git, got %s", n.Label)
	}
}

func TestUpdater_AddNew_PreservesExplicitFields(t *testing.T) {
	o := newTestOntology()
	u := NewUpdater(o)

	n := u.AddOrUpdate("lang/go", Node{
		Label:          "Go",
		Description:    "Programming language",
		Tags:           []string{"lang"},
		Properties:     map[string]string{"version": "1.22"},
		Frequency:      5,
		NodeConfidence: ConfidenceHigh,
	})

	if n.Frequency != 5 {
		t.Errorf("expected preserved frequency 5, got %d", n.Frequency)
	}
	if n.NodeConfidence != ConfidenceHigh {
		t.Errorf("expected preserved confidence high, got %s", n.NodeConfidence)
	}
}

func TestUpdater_DuplicatePath_IncrementsFrequency(t *testing.T) {
	o := newTestOntology()
	u := NewUpdater(o)

	u.AddOrUpdate("tools/git", Node{Label: "Git"})
	n := u.AddOrUpdate("tools/git", Node{Label: "Git VCS"})

	if n.Frequency != 2 {
		t.Errorf("expected frequency 2, got %d", n.Frequency)
	}
	if n.Label != "Git VCS" {
		t.Errorf("expected updated label 'Git VCS', got %s", n.Label)
	}
}

func TestUpdater_AutoPromote_AtThreshold(t *testing.T) {
	o := newTestOntology()
	u := NewUpdater(o)

	u.AddOrUpdate("tools/git", Node{Label: "Git"})               // freq=1
	u.AddOrUpdate("tools/git", Node{})                            // freq=2
	n := u.AddOrUpdate("tools/git", Node{Description: "VCS tool"}) // freq=3

	if n.Frequency != 3 {
		t.Errorf("expected frequency 3, got %d", n.Frequency)
	}
	if n.NodeConfidence != ConfidenceHigh {
		t.Errorf("expected auto-promoted to high, got %s", n.NodeConfidence)
	}
	if n.Description != "VCS tool" {
		t.Errorf("expected description 'VCS tool', got %s", n.Description)
	}
}

func TestUpdater_MergesTags(t *testing.T) {
	o := newTestOntology()
	u := NewUpdater(o)

	u.AddOrUpdate("tools/git", Node{Label: "Git", Tags: []string{"vcs", "tool"}})
	n := u.AddOrUpdate("tools/git", Node{Tags: []string{"tool", "scm"}})

	expected := map[string]bool{"vcs": true, "tool": true, "scm": true}
	if len(n.Tags) != len(expected) {
		t.Fatalf("expected %d tags, got %d: %v", len(expected), len(n.Tags), n.Tags)
	}
	for _, tag := range n.Tags {
		if !expected[tag] {
			t.Errorf("unexpected tag: %s", tag)
		}
	}
}

func TestUpdater_MergesProperties(t *testing.T) {
	o := newTestOntology()
	u := NewUpdater(o)

	u.AddOrUpdate("lang/go", Node{Label: "Go", Properties: map[string]string{"version": "1.21"}})
	n := u.AddOrUpdate("lang/go", Node{Properties: map[string]string{"version": "1.22", "gc": "yes"}})

	if n.Properties["version"] != "1.22" {
		t.Errorf("expected version 1.22, got %s", n.Properties["version"])
	}
	if n.Properties["gc"] != "yes" {
		t.Errorf("expected gc=yes, got %s", n.Properties["gc"])
	}
}

func TestUpdater_EmptyIncoming_DoesNotOverwrite(t *testing.T) {
	o := newTestOntology()
	u := NewUpdater(o)

	u.AddOrUpdate("tools/git", Node{Label: "Git", Description: "VCS"})
	n := u.AddOrUpdate("tools/git", Node{}) // empty incoming

	if n.Label != "Git" {
		t.Errorf("expected label preserved as 'Git', got %s", n.Label)
	}
	if n.Description != "VCS" {
		t.Errorf("expected description preserved as 'VCS', got %s", n.Description)
	}
}

func TestUpdater_NilNodes_Initialised(t *testing.T) {
	o := &Ontology{Version: "1.0.0"}
	if o.Schema.Nodes != nil {
		t.Fatal("precondition failed: Nodes should be nil")
	}

	u := NewUpdater(o)
	u.AddOrUpdate("x", Node{Label: "X"})

	if len(u.Ontology().Schema.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(u.Ontology().Schema.Nodes))
	}
}

func TestUpdater_MultipleDistinctPaths(t *testing.T) {
	o := newTestOntology()
	u := NewUpdater(o)

	u.AddOrUpdate("a", Node{Label: "A"})
	u.AddOrUpdate("b", Node{Label: "B"})
	u.AddOrUpdate("c", Node{Label: "C"})

	if len(o.Schema.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(o.Schema.Nodes))
	}
}

func TestUpdater_PropertiesMerge_NilExisting(t *testing.T) {
	o := newTestOntology()
	u := NewUpdater(o)

	// Add node without properties first
	u.AddOrUpdate("x", Node{Label: "X"})
	// Then update with properties
	n := u.AddOrUpdate("x", Node{Properties: map[string]string{"k": "v"}})

	if n.Properties["k"] != "v" {
		t.Errorf("expected property k=v, got %s", n.Properties["k"])
	}
}
