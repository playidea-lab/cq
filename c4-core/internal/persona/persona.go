package persona

import (
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
)

// EditPattern represents a pattern extracted from user edits.
type EditPattern struct {
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Frequency   int      `json:"frequency"`
	Examples    []string `json:"examples"`
}

// ProfileDiff represents proposed changes to the user profile.
type ProfileDiff struct {
	ToneUpdates      []string      `json:"tone_updates"`
	StructureUpdates []string      `json:"structure_updates"`
	NewPatterns      []EditPattern `json:"new_patterns"`
	Summary          string        `json:"summary"`
}

var defaultAssertiveWords = []string{"있습니다", "오류가", "잘못", "틀린", "부적절"}
var defaultSoftWords = []string{"필요합니다", "생각됩니다", "확인", "바랍니다", "감사"}

// AnalyzeEdits extracts edit patterns from AI draft vs user final version.
func AnalyzeEdits(original, edited string) []EditPattern {
	return analyzeEdits(original, edited, defaultAssertiveWords, defaultSoftWords)
}

// analyzeEdits is the unexported helper that accepts custom tone dictionaries.
func analyzeEdits(original, edited string, assertive, soft []string) []EditPattern {
	var patterns []EditPattern

	origLines := strings.Split(original, "\n")
	editLines := strings.Split(edited, "\n")

	deletions, additions := computeLineDiff(origLines, editLines)

	// Detect tone softening
	if examples := detectToneSoftening(deletions, additions, assertive, soft); len(examples) > 0 {
		patterns = append(patterns, EditPattern{
			Category:    "tone",
			Description: "User softened tone: assertive → questioning/hedging",
			Frequency:   1,
			Examples:    truncateSlice(examples, 3),
		})
	}

	// Detect conciseness
	if len(edited) < int(float64(len(original))*0.8) && len(original) > 0 {
		ratio := float64(len(edited)) / float64(len(original))
		patterns = append(patterns, EditPattern{
			Category:    "structure",
			Description: fmt.Sprintf("User significantly shortened text (ratio: %.2f)", ratio),
			Frequency:   1,
			Examples:    []string{fmt.Sprintf("Original: %d chars → Edited: %d chars", len(original), len(edited))},
		})
	}

	// Detect section reordering
	if detectSectionReorder(original, edited) {
		patterns = append(patterns, EditPattern{
			Category:    "structure",
			Description: "User reordered sections",
			Frequency:   1,
		})
	}

	// Detect pure deletions
	if len(deletions) > 0 && len(additions) == 0 {
		patterns = append(patterns, EditPattern{
			Category:    "deletion",
			Description: "User removed content without replacement",
			Frequency:   1,
			Examples:    truncateSlice(deletions, 3),
		})
	}

	// Detect pure additions
	if len(additions) > 0 && len(deletions) == 0 {
		patterns = append(patterns, EditPattern{
			Category:    "addition",
			Description: "User added new content",
			Frequency:   1,
			Examples:    truncateSlice(additions, 3),
		})
	}

	// Detect wording substitutions
	if subs := detectSubstitutions(deletions, additions); len(subs) > 0 {
		patterns = append(patterns, EditPattern{
			Category:    "wording",
			Description: "User substituted specific wordings",
			Frequency:   1,
			Examples:    truncateSlice(subs, 3),
		})
	}

	return patterns
}

// SuggestProfileUpdates proposes profile updates based on accumulated patterns.
func SuggestProfileUpdates(patterns []EditPattern) ProfileDiff {
	var toneUpdates, structureUpdates []string

	for _, p := range patterns {
		switch p.Category {
		case "tone":
			toneUpdates = append(toneUpdates, p.Description)
		case "structure":
			structureUpdates = append(structureUpdates, p.Description)
		}
	}

	var parts []string
	if len(toneUpdates) > 0 {
		parts = append(parts, fmt.Sprintf("톤 패턴 %d건 발견", len(toneUpdates)))
	}
	if len(structureUpdates) > 0 {
		parts = append(parts, fmt.Sprintf("구조 패턴 %d건 발견", len(structureUpdates)))
	}

	summary := "변경 없음"
	if len(parts) > 0 {
		summary = strings.Join(parts, ". ")
	}

	return ProfileDiff{
		ToneUpdates:      toneUpdates,
		StructureUpdates: structureUpdates,
		NewPatterns:      patterns,
		Summary:          summary,
	}
}

// RunPersonaLearn compares draft vs final and optionally applies to profile.
func RunPersonaLearn(draftPath, finalPath, profilePath string, autoApply bool) (*ProfileDiff, error) {
	if profilePath == "" {
		profilePath = ".c2/profile.yaml"
	}

	draftBytes, err := os.ReadFile(draftPath)
	if err != nil {
		return nil, fmt.Errorf("read draft: %w", err)
	}
	finalBytes, err := os.ReadFile(finalPath)
	if err != nil {
		return nil, fmt.Errorf("read final: %w", err)
	}

	// Load profile for custom tone dictionaries; missing-file errors are silently ignored
	// (fallback to defaults), but other errors (e.g. malformed YAML) are logged.
	profile, loadErr := LoadProfile(profilePath)
	if loadErr != nil {
		slog.Warn("persona_learn: profile load failed, using default tone dicts", "error", loadErr, "path", profilePath)
	}
	if profile == nil {
		profile = map[string]any{}
	}
	assertive := profileStringSlice(profile, "learned_patterns", "tone_assertive", defaultAssertiveWords)
	soft := profileStringSlice(profile, "learned_patterns", "tone_soft", defaultSoftWords)

	patterns := analyzeEdits(string(draftBytes), string(finalBytes), assertive, soft)
	diff := SuggestProfileUpdates(patterns)

	if autoApply && (len(diff.ToneUpdates) > 0 || len(diff.StructureUpdates) > 0) {
		UpdateLearnedPatterns(profile, diff.ToneUpdates, diff.StructureUpdates, nil)
		if err := SaveProfile(profile, profilePath); err != nil {
			return nil, fmt.Errorf("save profile: %w", err)
		}
	}

	return &diff, nil
}

// profileStringSlice retrieves a string slice from profile[section][key],
// returning fallback if absent.
func profileStringSlice(profile map[string]any, section, key string, fallback []string) []string {
	if profile == nil {
		return fallback
	}
	sec, ok := profile[section].(map[string]any)
	if !ok {
		return fallback
	}
	if v := toStringSlice(sec[key]); len(v) > 0 {
		return v
	}
	return fallback
}

// =========================================================================
// Diff helpers
// =========================================================================

// computeLineDiff returns deleted and added lines using a frequency-based (bag-of-lines) diff.
func computeLineDiff(orig, edit []string) (deletions, additions []string) {
	// Build a set of lines in each
	origSet := make(map[string]int)
	editSet := make(map[string]int)
	for _, l := range orig {
		origSet[l]++
	}
	for _, l := range edit {
		editSet[l]++
	}

	// Lines in orig but not in edit = deletions
	used := make(map[string]int)
	for _, l := range orig {
		used[l]++
		if used[l] > editSet[l] {
			trimmed := strings.TrimSpace(l)
			if trimmed != "" {
				deletions = append(deletions, trimmed)
			}
		}
	}

	// Lines in edit but not in orig = additions
	used = make(map[string]int)
	for _, l := range edit {
		used[l]++
		if used[l] > origSet[l] {
			trimmed := strings.TrimSpace(l)
			if trimmed != "" {
				additions = append(additions, trimmed)
			}
		}
	}

	return
}

func detectToneSoftening(deletions, additions []string, assertive, soft []string) []string {
	var examples []string
	for _, d := range deletions {
		if containsAny(d, assertive) {
			for _, a := range additions {
				if containsAny(a, soft) {
					examples = append(examples, fmt.Sprintf("'%s' → '%s'", truncStr(d, 50), truncStr(a, 50)))
					break
				}
			}
		}
	}
	return examples
}

func detectSectionReorder(original, edited string) bool {
	headerRe := regexp.MustCompile(`(?m)^#{1,3}\s+(.+)$`)
	origHeaders := headerRe.FindAllStringSubmatch(original, -1)
	editHeaders := headerRe.FindAllStringSubmatch(edited, -1)

	if len(origHeaders) < 2 || len(editHeaders) < 2 {
		return false
	}

	origNames := make([]string, len(origHeaders))
	editNames := make([]string, len(editHeaders))
	origSet := make(map[string]bool)
	editSet := make(map[string]bool)

	for i, m := range origHeaders {
		origNames[i] = m[1]
		origSet[m[1]] = true
	}
	for i, m := range editHeaders {
		editNames[i] = m[1]
		editSet[m[1]] = true
	}

	// Same headers, different order?
	if len(origSet) != len(editSet) {
		return false
	}
	for k := range origSet {
		if !editSet[k] {
			return false
		}
	}

	// Check if order differs
	for i := range origNames {
		if i >= len(editNames) {
			return false
		}
		if origNames[i] != editNames[i] {
			return true
		}
	}
	return false
}

func detectSubstitutions(deletions, additions []string) []string {
	var examples []string
	minLen := len(deletions)
	if len(additions) < minLen {
		minLen = len(additions)
	}
	for i := 0; i < minLen; i++ {
		d, a := deletions[i], additions[i]
		if d != "" && a != "" && d != a {
			ratio := similarityRatio(d, a)
			if ratio > 0.4 && ratio < 0.95 {
				examples = append(examples, fmt.Sprintf("'%s' → '%s'", truncStr(d, 40), truncStr(a, 40)))
			}
		}
	}
	return examples
}

// similarityRatio computes a simple character overlap ratio.
func similarityRatio(a, b string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	// Count matching characters using a simple approach
	shorter, longer := a, b
	if len(a) > len(b) {
		shorter, longer = b, a
	}
	matches := 0
	used := make([]bool, len(longer))
	for _, c := range shorter {
		for j, lc := range longer {
			if !used[j] && c == lc {
				matches++
				used[j] = true
				break
			}
		}
	}
	return 2.0 * float64(matches) / float64(len(a)+len(b))
}

func containsAny(s string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

func truncStr(s string, maxLen int) string {
	r := []rune(s)
	if len(r) > maxLen {
		return string(r[:maxLen])
	}
	return s
}

func truncateSlice(s []string, max int) []string {
	if len(s) > max {
		return s[:max]
	}
	return s
}
