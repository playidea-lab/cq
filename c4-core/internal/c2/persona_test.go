package c2

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeEdits_ToneSoftening(t *testing.T) {
	original := "이 코드에 오류가 있습니다.\n코드를 수정하세요."
	edited := "이 코드에 확인이 필요합니다.\n코드를 검토해 주시기 바랍니다."

	patterns := AnalyzeEdits(original, edited)
	found := false
	for _, p := range patterns {
		if p.Category == "tone" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected tone softening pattern")
	}
}

func TestAnalyzeEdits_Conciseness(t *testing.T) {
	original := "This is a very long text that needs to be shortened significantly to make it more concise and readable."
	edited := "Short text."

	patterns := AnalyzeEdits(original, edited)
	found := false
	for _, p := range patterns {
		if p.Category == "structure" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected structure pattern for conciseness")
	}
}

func TestAnalyzeEdits_SectionReorder(t *testing.T) {
	original := "# Introduction\ncontent\n# Method\ncontent\n# Results\ncontent"
	edited := "# Method\ncontent\n# Introduction\ncontent\n# Results\ncontent"

	patterns := AnalyzeEdits(original, edited)
	found := false
	for _, p := range patterns {
		if p.Category == "structure" && p.Description == "User reordered sections" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected section reorder pattern")
	}
}

func TestAnalyzeEdits_PureDeletion(t *testing.T) {
	original := "line1\nline2\nline3"
	edited := "line1"

	patterns := AnalyzeEdits(original, edited)
	found := false
	for _, p := range patterns {
		if p.Category == "deletion" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected deletion pattern")
	}
}

func TestAnalyzeEdits_PureAddition(t *testing.T) {
	original := "line1"
	edited := "line1\nline2\nline3"

	patterns := AnalyzeEdits(original, edited)
	found := false
	for _, p := range patterns {
		if p.Category == "addition" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected addition pattern")
	}
}

func TestAnalyzeEdits_NoChanges(t *testing.T) {
	text := "same text"
	patterns := AnalyzeEdits(text, text)
	if len(patterns) != 0 {
		t.Errorf("expected 0 patterns, got %d", len(patterns))
	}
}

func TestSuggestProfileUpdates(t *testing.T) {
	patterns := []EditPattern{
		{Category: "tone", Description: "softened"},
		{Category: "structure", Description: "shortened"},
		{Category: "wording", Description: "changed word"},
	}

	diff := SuggestProfileUpdates(patterns)
	if len(diff.ToneUpdates) != 1 {
		t.Errorf("ToneUpdates = %d, want 1", len(diff.ToneUpdates))
	}
	if len(diff.StructureUpdates) != 1 {
		t.Errorf("StructureUpdates = %d, want 1", len(diff.StructureUpdates))
	}
	if diff.Summary == "변경 없음" {
		t.Error("expected non-empty summary")
	}
}

func TestSuggestProfileUpdates_Empty(t *testing.T) {
	diff := SuggestProfileUpdates(nil)
	if diff.Summary != "변경 없음" {
		t.Errorf("Summary = %q, want 변경 없음", diff.Summary)
	}
}

func TestRunPersonaLearn(t *testing.T) {
	dir := t.TempDir()
	draftPath := filepath.Join(dir, "draft.md")
	finalPath := filepath.Join(dir, "final.md")
	profilePath := filepath.Join(dir, "profile.yaml")

	os.WriteFile(draftPath, []byte("이 코드에 오류가 있습니다."), 0644)
	os.WriteFile(finalPath, []byte("이 코드에 확인이 필요합니다."), 0644)

	diff, err := RunPersonaLearn(draftPath, finalPath, profilePath, true)
	if err != nil {
		t.Fatalf("RunPersonaLearn: %v", err)
	}
	if diff == nil {
		t.Fatal("expected non-nil diff")
	}

	// Profile should be created if auto_apply
	if len(diff.ToneUpdates) > 0 {
		_, err := os.Stat(profilePath)
		if os.IsNotExist(err) {
			t.Error("expected profile to be created with auto_apply")
		}
	}
}

func TestRunPersonaLearn_FileNotFound(t *testing.T) {
	_, err := RunPersonaLearn("/nonexistent/draft.md", "/nonexistent/final.md", "", false)
	if err == nil {
		t.Error("expected error for nonexistent files")
	}
}

func TestSimilarityRatio(t *testing.T) {
	if r := similarityRatio("hello", "hello"); r != 1.0 {
		t.Errorf("identical = %f, want 1.0", r)
	}
	if r := similarityRatio("abc", "xyz"); r > 0.1 {
		t.Errorf("completely different = %f, want ~0", r)
	}
	if r := similarityRatio("hello world", "hello earth"); r < 0.4 {
		t.Errorf("similar = %f, expected > 0.4", r)
	}
}

func TestDetectSectionReorder_NoReorder(t *testing.T) {
	text := "# A\n# B\n# C"
	if detectSectionReorder(text, text) {
		t.Error("expected false for same order")
	}
}
