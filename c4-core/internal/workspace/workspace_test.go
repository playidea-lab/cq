package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCreateWorkspace_AcademicPaper(t *testing.T) {
	ws := CreateWorkspace("Test Paper", AcademicPaper, "Write a paper", nil)
	if ws.ProjectName != "Test Paper" {
		t.Errorf("ProjectName = %q, want Test Paper", ws.ProjectName)
	}
	if ws.ProjectType != AcademicPaper {
		t.Errorf("ProjectType = %q, want academic_paper", ws.ProjectType)
	}
	if len(ws.Sections) != 7 {
		t.Errorf("Sections = %d, want 7", len(ws.Sections))
	}
	if ws.Sections[0].Name != "abstract" {
		t.Errorf("first section = %q, want abstract", ws.Sections[0].Name)
	}
	if ws.Sections[0].Status != NotStarted {
		t.Errorf("section status = %q, want 미시작", ws.Sections[0].Status)
	}
}

func TestCreateWorkspace_Proposal(t *testing.T) {
	ws := CreateWorkspace("Proposal", Proposal, "Get funding", nil)
	if len(ws.Sections) != 7 {
		t.Errorf("Sections = %d, want 7", len(ws.Sections))
	}
	if ws.Sections[0].Name != "executive_summary" {
		t.Errorf("first section = %q, want executive_summary", ws.Sections[0].Name)
	}
}

func TestCreateWorkspace_CustomSections(t *testing.T) {
	ws := CreateWorkspace("Custom", Report, "Report", []string{"intro", "body", "conclusion"})
	if len(ws.Sections) != 3 {
		t.Errorf("Sections = %d, want 3", len(ws.Sections))
	}
}

func TestRenderAndParseWorkspace_RoundTrip(t *testing.T) {
	today := "2026-02-15"
	original := &WorkspaceState{
		ProjectName: "My Paper",
		ProjectType: AcademicPaper,
		Goal:        "Publish at ICML",
		CreatedAt:   &today,
		LastSession: "2026-02-15 - 프로젝트 생성",
		Sources: []Source{
			{ID: "smith2024", Title: "Smith et al", Type: "paper", Relevance: "H", Status: "발견", Notes: "important"},
		},
		ReadingNotes: []ReadingNote{
			{
				SourceID:    "smith2024",
				SourceTitle: "Smith et al",
				Passes: []ReadingPass{
					{PassNumber: 1, Claims: []string{"claim1", "claim2"}, MethodNotes: "transformer", ConnectionToProject: "baseline"},
				},
			},
		},
		Sections: []SectionState{
			{Name: "abstract", Status: NotStarted, Notes: ""},
			{Name: "introduction", Status: Drafting, Notes: "started"},
		},
		Reviews: []ReviewRecord{
			{Date: &today, Reviewer: "self", Type: "self_review", Summary: "needs work", ReflectionStatus: "미반영"},
		},
		ClaimEvidence: []ClaimEvidenceItem{
			{Claim: "Our method is faster", EvidenceSource: "exp001", Result: "2x speedup", Location: "Table 1"},
		},
		OpenQuestions: []string{"Is this novel enough?"},
		Changelog: []ChangeEntry{
			{Date: &today, Domain: "write", Action: "started draft", Decision: "use transformer"},
		},
	}

	md := RenderWorkspace(original)

	// Verify key content in rendered markdown
	if !strings.Contains(md, "# c2 Workspace - My Paper") {
		t.Error("missing project name in render")
	}
	if !strings.Contains(md, "academic_paper") {
		t.Error("missing project type in render")
	}
	if !strings.Contains(md, "Smith et al") {
		t.Error("missing source in render")
	}

	// Parse back
	parsed := ParseWorkspace(md)

	if parsed.ProjectName != "My Paper" {
		t.Errorf("ProjectName = %q, want My Paper", parsed.ProjectName)
	}
	if parsed.ProjectType != AcademicPaper {
		t.Errorf("ProjectType = %q, want academic_paper", parsed.ProjectType)
	}
	if parsed.Goal != "Publish at ICML" {
		t.Errorf("Goal = %q, want Publish at ICML", parsed.Goal)
	}
	if len(parsed.Sources) != 1 {
		t.Fatalf("Sources = %d, want 1", len(parsed.Sources))
	}
	if parsed.Sources[0].Title != "Smith et al" {
		t.Errorf("Source.Title = %q, want Smith et al", parsed.Sources[0].Title)
	}
	if len(parsed.Sections) != 2 {
		t.Fatalf("Sections = %d, want 2", len(parsed.Sections))
	}
	if parsed.Sections[1].Status != Drafting {
		t.Errorf("Section[1].Status = %q, want 초안", parsed.Sections[1].Status)
	}
	if len(parsed.OpenQuestions) != 1 {
		t.Errorf("OpenQuestions = %d, want 1", len(parsed.OpenQuestions))
	}
	if len(parsed.ClaimEvidence) != 1 {
		t.Errorf("ClaimEvidence = %d, want 1", len(parsed.ClaimEvidence))
	}
	if len(parsed.Changelog) != 1 {
		t.Errorf("Changelog = %d, want 1", len(parsed.Changelog))
	}
}

func TestSaveAndLoadWorkspace(t *testing.T) {
	dir := t.TempDir()
	ws := CreateWorkspace("Test", AcademicPaper, "test goal", nil)

	savedPath, err := SaveWorkspace(ws, dir)
	if err != nil {
		t.Fatalf("SaveWorkspace: %v", err)
	}

	expected := filepath.Join(dir, "c2_workspace.md")
	if savedPath != expected {
		t.Errorf("path = %q, want %q", savedPath, expected)
	}

	data, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	parsed := ParseWorkspace(string(data))
	if parsed.ProjectName != "Test" {
		t.Errorf("ProjectName = %q, want Test", parsed.ProjectName)
	}
	if len(parsed.Sections) != 7 {
		t.Errorf("Sections = %d, want 7", len(parsed.Sections))
	}
}

func TestParseWorkspace_Empty(t *testing.T) {
	parsed := ParseWorkspace("")
	if parsed.ProjectName != "" {
		t.Errorf("expected empty project name, got %q", parsed.ProjectName)
	}
}

func TestParseTableRows(t *testing.T) {
	text := `| Header1 | Header2 |
|---------|---------|
| val1 | val2 |
| val3 | val4 |`

	rows := parseTableRows(text)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0][0] != "val1" {
		t.Errorf("rows[0][0] = %q, want val1", rows[0][0])
	}
}

func TestSplitSections(t *testing.T) {
	md := `## Section A
content a
## Section B
content b`

	sections := splitSections(md)
	if len(sections) != 2 {
		t.Errorf("sections = %d, want 2", len(sections))
	}
	if !strings.Contains(sections["Section A"], "content a") {
		t.Error("missing content a")
	}
}

func TestParseOpenQuestions_Empty(t *testing.T) {
	q := parseOpenQuestions("-\n")
	if len(q) != 0 {
		t.Errorf("expected 0, got %d", len(q))
	}
}

func TestLookupSectionStatus(t *testing.T) {
	tests := []struct {
		input string
		want  SectionStatus
	}{
		{"미시작", NotStarted},
		{"초안", Drafting},
		{"수정중", Revising},
		{"완료", Complete},
		{"unknown", NotStarted},
	}
	for _, tt := range tests {
		got := lookupSectionStatus(tt.input)
		if got != tt.want {
			t.Errorf("lookupSectionStatus(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseDiscoverSection_KoreanSlug(t *testing.T) {
	// 40 rune Korean title — slug should be exactly 40 runes, valid UTF-8
	title40 := strings.Repeat("가", 40)
	section40 := "| # | 자료 | 유형 | 관련도 | 상태 | 비고 |\n|---|------|------|--------|------|------|\n| 1 | " + title40 + " | paper | H | 발견 | - |\n"
	sources40 := parseDiscoverSection(section40)
	if len(sources40) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources40))
	}
	slug40 := sources40[0].ID
	if !utf8.ValidString(slug40) {
		t.Errorf("slug is not valid UTF-8: %q", slug40)
	}
	if utf8.RuneCountInString(slug40) > 40 {
		t.Errorf("slug rune count = %d, want <= 40", utf8.RuneCountInString(slug40))
	}

	// 41 rune Korean title — should be truncated to 40 runes, non-empty after Trim
	title41 := strings.Repeat("나", 41)
	section41 := "| # | 자료 | 유형 | 관련도 | 상태 | 비고 |\n|---|------|------|--------|------|------|\n| 1 | " + title41 + " | paper | H | 발견 | - |\n"
	sources41 := parseDiscoverSection(section41)
	if len(sources41) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources41))
	}
	slug41 := sources41[0].ID
	if !utf8.ValidString(slug41) {
		t.Errorf("slug is not valid UTF-8: %q", slug41)
	}
	if utf8.RuneCountInString(slug41) != 40 {
		t.Errorf("slug rune count = %d, want 40", utf8.RuneCountInString(slug41))
	}
	if slug41 == "" {
		t.Error("slug must not be empty after Trim")
	}
}
