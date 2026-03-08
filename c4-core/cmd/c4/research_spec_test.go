//go:build research

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/changmin/c4-core/internal/knowledge"
)

func TestResearchSpec_HappyPath(t *testing.T) {
	dir := t.TempDir()
	ks, err := knowledge.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer ks.Close()

	hypID, err := ks.Create(knowledge.TypeHypothesis, map[string]any{
		"title":  "Test hypothesis",
		"domain": "research",
	}, "hypothesis body")
	if err != nil {
		t.Fatalf("Create hypothesis: %v", err)
	}

	handler := researchSpecHandler(ks)
	rawArgs, _ := json.Marshal(map[string]any{
		"hypothesis_id":     hypID,
		"success_condition": "accuracy > 0.9",
		"null_condition":    "accuracy <= 0.5",
	})

	result, err := handler(rawArgs)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["spec_id"] == "" {
		t.Error("expected non-empty spec_id")
	}
	if m["hypothesis_id"] != hypID {
		t.Errorf("hypothesis_id = %q, want %q", m["hypothesis_id"], hypID)
	}
	if m["cq_yaml_draft"] == "" {
		t.Error("expected non-empty cq_yaml_draft")
	}
}

func TestResearchSpec_UnknownHypothesis(t *testing.T) {
	dir := t.TempDir()
	ks, err := knowledge.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer ks.Close()

	handler := researchSpecHandler(ks)
	rawArgs, _ := json.Marshal(map[string]any{
		"hypothesis_id": "hyp-nonexistent",
	})

	_, err = handler(rawArgs)
	if err == nil {
		t.Fatal("expected error for unknown hypothesis_id, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestResearchSpec_WrongType(t *testing.T) {
	dir := t.TempDir()
	ks, err := knowledge.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer ks.Close()

	// Create a TypeExperiment doc (not TypeHypothesis).
	expID, err := ks.Create(knowledge.TypeExperiment, map[string]any{
		"title":  "Some experiment",
		"domain": "research",
	}, "experiment body")
	if err != nil {
		t.Fatalf("Create experiment: %v", err)
	}

	handler := researchSpecHandler(ks)
	rawArgs, _ := json.Marshal(map[string]any{
		"hypothesis_id": expID,
	})

	_, err = handler(rawArgs)
	if err == nil {
		t.Fatal("expected error for wrong doc type, got nil")
	}
	if !strings.Contains(err.Error(), "wrong type") {
		t.Errorf("error = %q, want 'wrong type'", err.Error())
	}
}

func TestTypeDebateConstant(t *testing.T) {
	if knowledge.TypeDebate != "debate" {
		t.Errorf("TypeDebate = %q, want %q", knowledge.TypeDebate, "debate")
	}
}

func TestTypeDebatePrefix(t *testing.T) {
	dir := t.TempDir()
	ks, err := knowledge.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer ks.Close()

	debID, err := ks.Create(knowledge.TypeDebate, map[string]any{
		"title": "Test debate",
	}, "debate body")
	if err != nil {
		t.Fatalf("Create debate: %v", err)
	}
	if !strings.HasPrefix(debID, "deb-") {
		t.Errorf("debate ID = %q, want prefix %q", debID, "deb-")
	}
}
