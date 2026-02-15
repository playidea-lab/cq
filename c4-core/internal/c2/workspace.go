package c2

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ProjectType defines the type of c2 project.
type ProjectType string

const (
	AcademicPaper ProjectType = "academic_paper"
	Proposal      ProjectType = "proposal"
	Report        ProjectType = "report"
)

// SectionStatus defines the writing status of a section.
type SectionStatus string

const (
	NotStarted SectionStatus = "미시작"
	Drafting   SectionStatus = "초안"
	Revising   SectionStatus = "수정중"
	Complete   SectionStatus = "완료"
)

// WorkspaceState holds the full state of a c2 project workspace.
type WorkspaceState struct {
	ProjectName  string            `json:"project_name"`
	ProjectType  ProjectType       `json:"project_type"`
	Goal         string            `json:"goal"`
	CreatedAt    *string           `json:"created_at"`
	LastSession  string            `json:"last_session"`
	Sources      []Source          `json:"sources"`
	ReadingNotes []ReadingNote     `json:"reading_notes"`
	Sections     []SectionState    `json:"sections"`
	Reviews      []ReviewRecord    `json:"reviews"`
	ClaimEvidence []ClaimEvidenceItem `json:"claim_evidence"`
	OpenQuestions []string          `json:"open_questions"`
	Changelog    []ChangeEntry     `json:"changelog"`
}

// Source represents a discovered source.
type Source struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	Relevance string `json:"relevance"`
	Status    string `json:"status"`
	Notes     string `json:"notes"`
}

// ReadingNote represents a reading note.
type ReadingNote struct {
	SourceID    string        `json:"source_id"`
	SourceTitle string        `json:"source_title"`
	Passes      []ReadingPass `json:"passes"`
}

// ReadingPass represents a single reading pass.
type ReadingPass struct {
	PassNumber          int      `json:"pass_number"`
	Claims              []string `json:"claims"`
	MethodNotes         string   `json:"method_notes"`
	ConnectionToProject string   `json:"connection_to_project"`
}

// SectionState represents the status of a document section.
type SectionState struct {
	Name   string        `json:"name"`
	Status SectionStatus `json:"status"`
	Notes  string        `json:"notes"`
}

// ReviewRecord represents a review entry.
type ReviewRecord struct {
	Date             *string `json:"date"`
	Reviewer         string  `json:"reviewer"`
	Type             string  `json:"type"`
	Summary          string  `json:"summary"`
	ReflectionStatus string  `json:"reflection_status"`
}

// ClaimEvidenceItem maps a claim to its evidence.
type ClaimEvidenceItem struct {
	Claim          string `json:"claim"`
	EvidenceSource string `json:"evidence_source"`
	Result         string `json:"result"`
	Location       string `json:"location"`
}

// ChangeEntry represents a changelog entry.
type ChangeEntry struct {
	Date     *string `json:"date"`
	Domain   string  `json:"domain"`
	Action   string  `json:"action"`
	Decision string  `json:"decision"`
}

// DefaultSections returns default sections for a project type.
func DefaultSections(pt ProjectType) []string {
	switch pt {
	case AcademicPaper:
		return []string{"abstract", "introduction", "related_work", "method", "experiments", "discussion", "conclusion"}
	case Proposal:
		return []string{"executive_summary", "background", "objectives", "approach", "timeline", "budget", "expected_outcomes"}
	default:
		return nil
	}
}

// CreateWorkspace creates a new workspace state with default sections.
func CreateWorkspace(name string, projectType ProjectType, goal string, sections []string) *WorkspaceState {
	if len(sections) == 0 {
		sections = DefaultSections(projectType)
	}

	today := time.Now().Format("2006-01-02")
	secs := make([]SectionState, len(sections))
	for i, s := range sections {
		secs[i] = SectionState{Name: s, Status: NotStarted}
	}

	return &WorkspaceState{
		ProjectName: name,
		ProjectType: projectType,
		Goal:        goal,
		CreatedAt:   &today,
		LastSession: today + " - 프로젝트 생성",
		Sections:    secs,
	}
}

// RenderWorkspace renders a WorkspaceState to markdown (c2_workspace.md format).
func RenderWorkspace(state *WorkspaceState) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# c2 Workspace - %s\n\n", state.ProjectName))

	// 프로젝트 정보
	b.WriteString("## 프로젝트 정보\n")
	b.WriteString(fmt.Sprintf("- **유형**: %s\n", state.ProjectType))
	b.WriteString(fmt.Sprintf("- **목표**: %s\n", state.Goal))
	createdAt := ""
	if state.CreatedAt != nil {
		createdAt = *state.CreatedAt
	}
	b.WriteString(fmt.Sprintf("- **생성일**: %s\n", createdAt))
	b.WriteString(fmt.Sprintf("- **마지막 세션**: %s\n\n", state.LastSession))

	// Discover
	b.WriteString("## Discover (자료 탐색)\n")
	b.WriteString("| # | 자료 | 유형 | 관련도 | 상태 | 비고 |\n")
	b.WriteString("|---|------|------|--------|------|------|\n")
	for i, src := range state.Sources {
		b.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s | %s |\n",
			i+1, src.Title, src.Type, src.Relevance, src.Status, src.Notes))
	}
	b.WriteString("\n")

	// Read
	b.WriteString("## Read (읽기 노트)\n")
	b.WriteString("| 자료 | 핵심 주장 | 방법/접근 | 우리와의 연결 | 노트 파일 |\n")
	b.WriteString("|------|----------|----------|-------------|----------|\n")
	for _, note := range state.ReadingNotes {
		claims := ""
		method := ""
		conn := ""
		if len(note.Passes) > 0 {
			p := note.Passes[0]
			if len(p.Claims) > 0 {
				max := 2
				if len(p.Claims) < max {
					max = len(p.Claims)
				}
				claims = strings.Join(p.Claims[:max], "; ")
			}
			if len(p.MethodNotes) > 40 {
				method = p.MethodNotes[:40]
			} else {
				method = p.MethodNotes
			}
			if len(p.ConnectionToProject) > 40 {
				conn = p.ConnectionToProject[:40]
			} else {
				conn = p.ConnectionToProject
			}
		}
		title := note.SourceTitle
		if title == "" {
			title = note.SourceID
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | read/%s_note.md |\n",
			title, claims, method, conn, note.SourceID))
	}
	b.WriteString("\n")

	// Write
	b.WriteString("## Write (작성 상태)\n")
	b.WriteString("| 섹션 | 상태 | 비고 |\n")
	b.WriteString("|------|------|------|\n")
	for _, sec := range state.Sections {
		b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", sec.Name, sec.Status, sec.Notes))
	}
	b.WriteString("\n")

	// Review
	b.WriteString("## Review (리뷰 이력)\n")
	b.WriteString("| 날짜 | 리뷰어 | 유형 | 주요 피드백 | 반영 상태 |\n")
	b.WriteString("|------|--------|------|-----------|----------|\n")
	for _, rev := range state.Reviews {
		d := ""
		if rev.Date != nil {
			d = *rev.Date
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			d, rev.Reviewer, rev.Type, rev.Summary, rev.ReflectionStatus))
	}
	b.WriteString("\n")

	// Claim-Evidence
	b.WriteString("## Claim-Evidence 매핑\n")
	b.WriteString("| 주장 | 근거 자료 | 결과/수치 | 위치 |\n")
	b.WriteString("|------|----------|----------|------|\n")
	for _, ce := range state.ClaimEvidence {
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			ce.Claim, ce.EvidenceSource, ce.Result, ce.Location))
	}
	b.WriteString("\n")

	// Open questions
	b.WriteString("## 열린 질문\n")
	if len(state.OpenQuestions) > 0 {
		for _, q := range state.OpenQuestions {
			b.WriteString(fmt.Sprintf("- %s\n", q))
		}
	} else {
		b.WriteString("-\n")
	}
	b.WriteString("\n")

	// Changelog
	b.WriteString("## 변경 이력\n")
	b.WriteString("| 날짜 | 도메인 | 작업 | 결정 |\n")
	b.WriteString("|------|--------|------|------|\n")
	for _, entry := range state.Changelog {
		d := ""
		if entry.Date != nil {
			d = *entry.Date
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			d, entry.Domain, entry.Action, entry.Decision))
	}
	b.WriteString("\n")

	return b.String()
}

// SaveWorkspace saves workspace state as c2_workspace.md.
func SaveWorkspace(state *WorkspaceState, projectDir string) (string, error) {
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return "", fmt.Errorf("create project dir: %w", err)
	}
	wsPath := filepath.Join(projectDir, "c2_workspace.md")
	content := RenderWorkspace(state)
	if err := os.WriteFile(wsPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write workspace: %w", err)
	}
	return wsPath, nil
}

// ParseWorkspace parses a c2_workspace.md markdown string into a WorkspaceState.
func ParseWorkspace(mdText string) *WorkspaceState {
	state := &WorkspaceState{}

	// Extract project name
	nameRe := regexp.MustCompile(`(?m)^#\s+c2\s+Workspace\s+-\s+(.+)$`)
	if m := nameRe.FindStringSubmatch(mdText); m != nil {
		state.ProjectName = strings.TrimSpace(m[1])
	}

	sections := splitSections(mdText)

	// Metadata
	if meta, ok := sections["프로젝트 정보"]; ok {
		parseMeta(meta, state)
	}

	// Discover
	if sec, ok := sections["Discover (자료 탐색)"]; ok {
		state.Sources = parseDiscoverSection(sec)
	}

	// Read
	if sec, ok := sections["Read (읽기 노트)"]; ok {
		state.ReadingNotes = parseReadSection(sec)
	}

	// Write
	if sec, ok := sections["Write (작성 상태)"]; ok {
		state.Sections = parseWriteSection(sec)
	}

	// Review
	if sec, ok := sections["Review (리뷰 이력)"]; ok {
		state.Reviews = parseReviewSection(sec)
	}

	// Claim-Evidence
	if sec, ok := sections["Claim-Evidence 매핑"]; ok {
		state.ClaimEvidence = parseClaimEvidence(sec)
	}

	// Open questions
	if sec, ok := sections["열린 질문"]; ok {
		state.OpenQuestions = parseOpenQuestions(sec)
	}

	// Changelog
	if sec, ok := sections["변경 이력"]; ok {
		state.Changelog = parseChangelog(sec)
	}

	return state
}

// =========================================================================
// Internal parsing helpers
// =========================================================================

func splitSections(md string) map[string]string {
	sections := map[string]string{}
	var currentHeader string
	var currentLines []string

	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(line, "## ") {
			if currentHeader != "" {
				sections[currentHeader] = strings.Join(currentLines, "\n")
			}
			currentHeader = strings.TrimSpace(line[3:])
			currentLines = nil
		} else {
			currentLines = append(currentLines, line)
		}
	}
	if currentHeader != "" {
		sections[currentHeader] = strings.Join(currentLines, "\n")
	}
	return sections
}

func parseMeta(text string, state *WorkspaceState) {
	metaRe := regexp.MustCompile(`(?m)^-\s+\*\*(.+?)\*\*:\s*(.+)$`)
	for _, m := range metaRe.FindAllStringSubmatch(text, -1) {
		key := strings.TrimSpace(m[1])
		val := strings.TrimSpace(m[2])
		switch key {
		case "유형":
			state.ProjectType = ProjectType(val)
		case "목표":
			state.Goal = val
		case "생성일":
			state.CreatedAt = &val
		case "마지막 세션":
			state.LastSession = val
		}
	}
}

func parseTableRows(sectionText string) [][]string {
	var rows [][]string
	foundHeader := false
	skipSeparator := false

	for _, line := range strings.Split(sectionText, "\n") {
		stripped := strings.TrimSpace(line)
		if !foundHeader && strings.HasPrefix(stripped, "|") && strings.Contains(stripped[1:], "|") {
			foundHeader = true
			skipSeparator = true
			continue
		}
		if skipSeparator {
			skipSeparator = false
			if strings.HasPrefix(stripped, "|") && isSeparatorRow(stripped) {
				continue
			}
		}
		if foundHeader && strings.HasPrefix(stripped, "|") && strings.Contains(stripped[1:], "|") {
			cells := splitTableCells(stripped)
			if len(cells) > 0 && !allDashes(cells) {
				rows = append(rows, cells)
			}
		} else if foundHeader && !strings.HasPrefix(stripped, "|") && stripped != "" {
			break
		}
	}
	return rows
}

func isSeparatorRow(line string) bool {
	sepRe := regexp.MustCompile(`^\|[\s\-|]+\|$`)
	return sepRe.MatchString(line)
}

func splitTableCells(line string) []string {
	parts := strings.Split(line, "|")
	if len(parts) < 3 {
		return nil
	}
	// Trim first and last empty parts from leading/trailing |
	cells := make([]string, 0, len(parts)-2)
	for _, p := range parts[1 : len(parts)-1] {
		cells = append(cells, strings.TrimSpace(p))
	}
	return cells
}

func allDashes(cells []string) bool {
	dashRe := regexp.MustCompile(`^-+$`)
	for _, c := range cells {
		if !dashRe.MatchString(c) {
			return false
		}
	}
	return true
}

var slugRe = regexp.MustCompile(`[^a-zA-Z0-9가-힣]`)

func parseDiscoverSection(text string) []Source {
	rows := parseTableRows(text)
	var sources []Source
	for _, cells := range rows {
		if len(cells) < 6 {
			continue
		}
		slug := slugRe.ReplaceAllString(cells[1], "_")
		if len(slug) > 40 {
			slug = slug[:40]
		}
		slug = strings.Trim(slug, "_")
		if slug == "" {
			slug = fmt.Sprintf("src_%d", len(sources)+1)
		}
		sources = append(sources, Source{
			ID:        slug,
			Title:     cells[1],
			Type:      cells[2],
			Relevance: cells[3],
			Status:    cells[4],
			Notes:     cells[5],
		})
	}
	return sources
}

func parseReadSection(text string) []ReadingNote {
	rows := parseTableRows(text)
	var notes []ReadingNote
	for _, cells := range rows {
		if len(cells) < 5 {
			continue
		}
		noteRe := regexp.MustCompile(`read/(.+?)_note\.md`)
		sourceID := fmt.Sprintf("src_%d", len(notes)+1)
		if m := noteRe.FindStringSubmatch(cells[4]); m != nil {
			sourceID = m[1]
		}
		claims := splitClaims(cells[1])
		var passes []ReadingPass
		if cells[1] != "" || cells[2] != "" || cells[3] != "" {
			passes = []ReadingPass{{
				PassNumber:          1,
				Claims:              claims,
				MethodNotes:         cells[2],
				ConnectionToProject: cells[3],
			}}
		}
		notes = append(notes, ReadingNote{
			SourceID:    sourceID,
			SourceTitle: cells[0],
			Passes:      passes,
		})
	}
	return notes
}

func splitClaims(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	claims := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			claims = append(claims, p)
		}
	}
	return claims
}

func parseWriteSection(text string) []SectionState {
	rows := parseTableRows(text)
	var sections []SectionState
	for _, cells := range rows {
		if len(cells) < 3 {
			continue
		}
		status := lookupSectionStatus(cells[1])
		sections = append(sections, SectionState{
			Name:   cells[0],
			Status: status,
			Notes:  cells[2],
		})
	}
	return sections
}

func lookupSectionStatus(val string) SectionStatus {
	val = strings.TrimSpace(val)
	switch val {
	case string(NotStarted):
		return NotStarted
	case string(Drafting):
		return Drafting
	case string(Revising):
		return Revising
	case string(Complete):
		return Complete
	default:
		return NotStarted
	}
}

func parseReviewSection(text string) []ReviewRecord {
	rows := parseTableRows(text)
	var records []ReviewRecord
	for _, cells := range rows {
		if len(cells) < 5 {
			continue
		}
		var d *string
		if cells[0] != "" {
			d = &cells[0]
		}
		records = append(records, ReviewRecord{
			Date:             d,
			Reviewer:         cells[1],
			Type:             cells[2],
			Summary:          cells[3],
			ReflectionStatus: cells[4],
		})
	}
	return records
}

func parseClaimEvidence(text string) []ClaimEvidenceItem {
	rows := parseTableRows(text)
	var items []ClaimEvidenceItem
	for _, cells := range rows {
		if len(cells) < 4 {
			continue
		}
		items = append(items, ClaimEvidenceItem{
			Claim:          cells[0],
			EvidenceSource: cells[1],
			Result:         cells[2],
			Location:       cells[3],
		})
	}
	return items
}

func parseOpenQuestions(text string) []string {
	var questions []string
	for _, line := range strings.Split(text, "\n") {
		stripped := strings.TrimSpace(line)
		if strings.HasPrefix(stripped, "- ") {
			q := strings.TrimSpace(stripped[2:])
			if q != "" {
				questions = append(questions, q)
			}
		}
	}
	return questions
}

func parseChangelog(text string) []ChangeEntry {
	rows := parseTableRows(text)
	var entries []ChangeEntry
	for _, cells := range rows {
		if len(cells) < 4 {
			continue
		}
		var d *string
		if cells[0] != "" {
			d = &cells[0]
		}
		entries = append(entries, ChangeEntry{
			Date:     d,
			Domain:   cells[1],
			Action:   cells[2],
			Decision: cells[3],
		})
	}
	return entries
}
