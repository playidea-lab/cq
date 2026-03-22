package ontology

import "testing"

func TestSeedFromGlobal_EmptyGlobal_IsNoOp(t *testing.T) {
	setupTempHome(t)

	n, err := SeedFromGlobal("alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 nodes seeded, got %d", n)
	}
}

func TestSeedFromGlobal_CopiesNodes(t *testing.T) {
	setupTempHome(t)

	// Set up a global ontology with some nodes.
	global := &Ontology{Version: defaultVersion, Schema: CoreSchema{
		Nodes: map[string]Node{
			"tools/git":  {Label: "Git", Description: "VCS", Frequency: 5, NodeConfidence: ConfidenceHigh},
			"lang/go":    {Label: "Go", Frequency: 2, NodeConfidence: ConfidenceMedium},
		},
	}}
	if err := Save(GlobalUsername, global); err != nil {
		t.Fatalf("save global: %v", err)
	}

	n, err := SeedFromGlobal("bob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 nodes seeded, got %d", n)
	}

	local, err := Load("bob")
	if err != nil {
		t.Fatalf("load local: %v", err)
	}
	if len(local.Schema.Nodes) != 2 {
		t.Fatalf("expected 2 nodes in local, got %d", len(local.Schema.Nodes))
	}
	if local.Schema.Nodes["tools/git"].Label != "Git" {
		t.Errorf("expected label Git, got %s", local.Schema.Nodes["tools/git"].Label)
	}
}

func TestSeedFromGlobal_MergesWithExisting(t *testing.T) {
	setupTempHome(t)

	// Global has one node.
	global := &Ontology{Version: defaultVersion, Schema: CoreSchema{
		Nodes: map[string]Node{
			"tools/git": {Label: "Git", Frequency: 3, NodeConfidence: ConfidenceHigh},
		},
	}}
	if err := Save(GlobalUsername, global); err != nil {
		t.Fatalf("save global: %v", err)
	}

	// Local already has a different node + same path with different data.
	local := &Ontology{Version: defaultVersion, Schema: CoreSchema{
		Nodes: map[string]Node{
			"lang/go":   {Label: "Go", Frequency: 1, NodeConfidence: ConfidenceLow},
			"tools/git": {Label: "Git Local", Frequency: 1, NodeConfidence: ConfidenceLow},
		},
	}}
	if err := Save("carol", local); err != nil {
		t.Fatalf("save local: %v", err)
	}

	n, err := SeedFromGlobal("carol")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 node processed, got %d", n)
	}

	merged, err := Load("carol")
	if err != nil {
		t.Fatalf("load merged: %v", err)
	}
	// Should have both nodes.
	if len(merged.Schema.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(merged.Schema.Nodes))
	}
	// tools/git should have been merged (frequency incremented).
	git := merged.Schema.Nodes["tools/git"]
	if git.Frequency != 2 {
		t.Errorf("expected frequency 2 after merge, got %d", git.Frequency)
	}
}

func TestSeedFromGlobal_RejectsEmptyUsername(t *testing.T) {
	setupTempHome(t)

	_, err := SeedFromGlobal("")
	if err == nil {
		t.Error("expected error for empty username")
	}
}

func TestSeedFromGlobal_RejectsGlobalUsername(t *testing.T) {
	setupTempHome(t)

	_, err := SeedFromGlobal(GlobalUsername)
	if err == nil {
		t.Error("expected error for global username as target")
	}
}

func TestMergeToGlobal_EmptyLocal_IsNoOp(t *testing.T) {
	setupTempHome(t)

	n, err := MergeToGlobal("empty_user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 nodes merged, got %d", n)
	}
}

func TestMergeToGlobal_AddsNodesToGlobal(t *testing.T) {
	setupTempHome(t)

	local := &Ontology{Version: defaultVersion, Schema: CoreSchema{
		Nodes: map[string]Node{
			"tools/git": {Label: "Git", Frequency: 2, NodeConfidence: ConfidenceMedium},
			"lang/go":   {Label: "Go", Frequency: 1, NodeConfidence: ConfidenceLow},
		},
	}}
	if err := Save("dave", local); err != nil {
		t.Fatalf("save local: %v", err)
	}

	n, err := MergeToGlobal("dave")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 nodes merged, got %d", n)
	}

	global, err := Load(GlobalUsername)
	if err != nil {
		t.Fatalf("load global: %v", err)
	}
	if len(global.Schema.Nodes) != 2 {
		t.Fatalf("expected 2 nodes in global, got %d", len(global.Schema.Nodes))
	}
}

func TestMergeToGlobal_DeduplicatesExisting(t *testing.T) {
	setupTempHome(t)

	// Global already has a node.
	global := &Ontology{Version: defaultVersion, Schema: CoreSchema{
		Nodes: map[string]Node{
			"tools/git": {Label: "Git", Description: "VCS", Frequency: 2, NodeConfidence: ConfidenceMedium, Tags: []string{"vcs"}},
		},
	}}
	if err := Save(GlobalUsername, global); err != nil {
		t.Fatalf("save global: %v", err)
	}

	// Local has the same path with additional data.
	local := &Ontology{Version: defaultVersion, Schema: CoreSchema{
		Nodes: map[string]Node{
			"tools/git": {Label: "Git", Description: "Version control", Frequency: 1, NodeConfidence: ConfidenceLow, Tags: []string{"scm"}},
		},
	}}
	if err := Save("eve", local); err != nil {
		t.Fatalf("save local: %v", err)
	}

	n, err := MergeToGlobal("eve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 node merged, got %d", n)
	}

	merged, err := Load(GlobalUsername)
	if err != nil {
		t.Fatalf("load global: %v", err)
	}

	git := merged.Schema.Nodes["tools/git"]
	// Frequency should be incremented (2+1=3).
	if git.Frequency != 3 {
		t.Errorf("expected frequency 3, got %d", git.Frequency)
	}
	// At threshold 3 → auto-promoted to high.
	if git.NodeConfidence != ConfidenceHigh {
		t.Errorf("expected confidence high after merge, got %s", git.NodeConfidence)
	}
	// Description should be updated from incoming.
	if git.Description != "Version control" {
		t.Errorf("expected description 'Version control', got %s", git.Description)
	}
	// Tags should be merged (vcs + scm).
	tagSet := make(map[string]bool)
	for _, tag := range git.Tags {
		tagSet[tag] = true
	}
	if !tagSet["vcs"] || !tagSet["scm"] {
		t.Errorf("expected tags [vcs, scm], got %v", git.Tags)
	}
}

func TestMergeToGlobal_RejectsEmptyUsername(t *testing.T) {
	setupTempHome(t)

	_, err := MergeToGlobal("")
	if err == nil {
		t.Error("expected error for empty username")
	}
}

func TestMergeToGlobal_RejectsGlobalUsername(t *testing.T) {
	setupTempHome(t)

	_, err := MergeToGlobal(GlobalUsername)
	if err == nil {
		t.Error("expected error for global username as source")
	}
}

func TestSeedThenMerge_RoundTrip(t *testing.T) {
	setupTempHome(t)

	// 1. Create global with initial nodes.
	global := &Ontology{Version: defaultVersion, Schema: CoreSchema{
		Nodes: map[string]Node{
			"tools/git": {Label: "Git", Frequency: 1, NodeConfidence: ConfidenceLow},
		},
	}}
	if err := Save(GlobalUsername, global); err != nil {
		t.Fatalf("save global: %v", err)
	}

	// 2. Seed a new project user from global.
	n, err := SeedFromGlobal("frank")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 seeded, got %d", n)
	}

	// 3. Add a new node locally.
	local, err := Load("frank")
	if err != nil {
		t.Fatalf("load local: %v", err)
	}
	u := NewUpdater(local)
	u.AddOrUpdate("lang/rust", Node{Label: "Rust", Frequency: 1, NodeConfidence: ConfidenceLow})
	if err := Save("frank", local); err != nil {
		t.Fatalf("save local: %v", err)
	}

	// 4. Merge back to global.
	n, err = MergeToGlobal("frank")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 merged, got %d", n)
	}

	// 5. Verify global has both nodes.
	result, err := Load(GlobalUsername)
	if err != nil {
		t.Fatalf("load global: %v", err)
	}
	if len(result.Schema.Nodes) != 2 {
		t.Fatalf("expected 2 nodes in global, got %d", len(result.Schema.Nodes))
	}
	if _, ok := result.Schema.Nodes["lang/rust"]; !ok {
		t.Error("expected lang/rust in global after merge")
	}
	// tools/git frequency should be incremented.
	git := result.Schema.Nodes["tools/git"]
	if git.Frequency != 2 {
		t.Errorf("expected git frequency 2, got %d", git.Frequency)
	}
}

func TestSeedFromProject_EmptyProject_IsNoOp(t *testing.T) {
	setupTempHome(t)
	root := t.TempDir()

	n, err := SeedFromProject("alice", root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 nodes seeded, got %d", n)
	}
}

func TestSeedFromProject_CopiesProjectScopedNodes(t *testing.T) {
	setupTempHome(t)
	root := t.TempDir()

	proj := &ProjectOntology{
		Version: defaultVersion,
		Schema: CoreSchema{
			Nodes: map[string]Node{
				"arch/event-bus": {Label: "EventBus", Scope: "project", Frequency: 4, NodeConfidence: ConfidenceHigh},
				"arch/hub":       {Label: "Hub", Scope: "project", Frequency: 2, NodeConfidence: ConfidenceMedium},
			},
		},
	}
	if err := SaveProject(root, proj); err != nil {
		t.Fatalf("save project: %v", err)
	}

	n, err := SeedFromProject("bob", root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 nodes seeded, got %d", n)
	}

	local, err := Load("bob")
	if err != nil {
		t.Fatalf("load local: %v", err)
	}
	if len(local.Schema.Nodes) != 2 {
		t.Fatalf("expected 2 nodes in local, got %d", len(local.Schema.Nodes))
	}
	if local.Schema.Nodes["arch/event-bus"].Label != "EventBus" {
		t.Errorf("expected label EventBus, got %s", local.Schema.Nodes["arch/event-bus"].Label)
	}
}

func TestSeedFromProject_SkipsNonProjectScopedNodes(t *testing.T) {
	setupTempHome(t)
	root := t.TempDir()

	// Project ontology with mixed scopes.
	proj := &ProjectOntology{
		Version: defaultVersion,
		Schema: CoreSchema{
			Nodes: map[string]Node{
				"arch/core":  {Label: "Core", Scope: "project", Frequency: 3, NodeConfidence: ConfidenceHigh},
				"tools/lint": {Label: "Lint", Scope: "global", Frequency: 1, NodeConfidence: ConfidenceLow},
				"tools/ci":   {Label: "CI", Scope: "", Frequency: 1, NodeConfidence: ConfidenceLow},
			},
		},
	}
	if err := SaveProject(root, proj); err != nil {
		t.Fatalf("save project: %v", err)
	}

	n, err := SeedFromProject("carol", root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 node seeded (project-scoped only), got %d", n)
	}

	local, err := Load("carol")
	if err != nil {
		t.Fatalf("load local: %v", err)
	}
	if _, ok := local.Schema.Nodes["arch/core"]; !ok {
		t.Error("expected arch/core in local ontology")
	}
	if _, ok := local.Schema.Nodes["tools/lint"]; ok {
		t.Error("did not expect global-scoped tools/lint in local ontology")
	}
}

func TestSeedFromProject_MergesWithExistingL1(t *testing.T) {
	setupTempHome(t)
	root := t.TempDir()

	proj := &ProjectOntology{
		Version: defaultVersion,
		Schema: CoreSchema{
			Nodes: map[string]Node{
				"arch/hub": {Label: "Hub", Scope: "project", Frequency: 3, NodeConfidence: ConfidenceHigh},
			},
		},
	}
	if err := SaveProject(root, proj); err != nil {
		t.Fatalf("save project: %v", err)
	}

	// L1 already has a different node + the same node with different data.
	local := &Ontology{
		Version: defaultVersion,
		Schema: CoreSchema{
			Nodes: map[string]Node{
				"lang/go":  {Label: "Go", Frequency: 2, NodeConfidence: ConfidenceMedium},
				"arch/hub": {Label: "Hub Local", Frequency: 1, NodeConfidence: ConfidenceLow},
			},
		},
	}
	if err := Save("dave", local); err != nil {
		t.Fatalf("save local: %v", err)
	}

	n, err := SeedFromProject("dave", root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 node processed, got %d", n)
	}

	merged, err := Load("dave")
	if err != nil {
		t.Fatalf("load merged: %v", err)
	}
	if len(merged.Schema.Nodes) != 2 {
		t.Fatalf("expected 2 nodes total, got %d", len(merged.Schema.Nodes))
	}
	// arch/hub frequency should be incremented (1 existing + 3 incoming → 4, or via AddOrUpdate).
	hub := merged.Schema.Nodes["arch/hub"]
	if hub.Frequency < 1 {
		t.Errorf("expected positive frequency after merge, got %d", hub.Frequency)
	}
}

func TestSeedFromProject_RejectsEmptyUsername(t *testing.T) {
	setupTempHome(t)
	root := t.TempDir()

	_, err := SeedFromProject("", root)
	if err == nil {
		t.Error("expected error for empty username")
	}
}

func TestSeedFromProject_RejectsGlobalUsername(t *testing.T) {
	setupTempHome(t)
	root := t.TempDir()

	_, err := SeedFromProject(GlobalUsername, root)
	if err == nil {
		t.Error("expected error for global username as target")
	}
}
