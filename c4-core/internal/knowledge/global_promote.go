package knowledge

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

// pii patterns — conservative: any match blocks promotion.
// Names are prefixed with "pii" to avoid collision with sync.go's reAbsPath.
var (
	piiAbsPath    = regexp.MustCompile(`(?i)(/Users/[^\s"']+|/home/[^\s"']+|C:\\Users\\[^\s"']+)`)
	piiEmail      = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	piiGitHubURL  = regexp.MustCompile(`https?://github\.com/[a-zA-Z0-9_\-]+(?:/[^\s"']*)?`)
	piiProjectDir = regexp.MustCompile(`\.c4/projects/[a-zA-Z0-9_\-/]+`)
)

// ContainsPII returns true if text contains patterns that look like personal
// information (absolute paths, emails, GitHub user URLs, project UUIDs).
// Conservative: when in doubt, returns true to block promotion.
func ContainsPII(text string) bool {
	return piiAbsPath.MatchString(text) ||
		piiEmail.MatchString(text) ||
		piiGitHubURL.MatchString(text) ||
		piiProjectDir.MatchString(text)
}

// Depersonalize applies rule-based substitutions to remove personal information.
// It does not call an LLM; v1 is intentionally rule-based for speed and
// determinism. Lines that still look like personal directories are stripped.
func Depersonalize(text string) string {
	// Replace absolute paths first (most specific).
	result := piiAbsPath.ReplaceAllString(text, "<PATH>")

	// Replace email-like patterns.
	result = piiEmail.ReplaceAllString(result, "<EMAIL>")

	// Replace GitHub URLs (profile + repo).
	result = piiGitHubURL.ReplaceAllString(result, "<GITHUB_URL>")

	// Replace .c4/projects/ paths (project UUIDs).
	result = piiProjectDir.ReplaceAllString(result, "<PROJECT_ID>")

	// Strip lines that still reference personal-looking directories
	// (e.g. leftover "~username" or bare "/home/…" fragments).
	lines := strings.Split(result, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if rePersonalDir.MatchString(line) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// rePersonalDir catches residual personal directory fragments that survived
// the substitution pass above.
var rePersonalDir = regexp.MustCompile(`(?i)(~[a-zA-Z][a-zA-Z0-9_\-]+|/home/[^\s]+|/Users/[^\s]+)`)

// ErrPIIDetected is returned by PromoteToGlobal when PII survives depersonalization.
var ErrPIIDetected = errors.New("content contains PII and cannot be promoted to global")

// PromoteToGlobal depersonalizes content and creates a public knowledge document.
// It returns the new document ID or an error if PII survives depersonalization.
func PromoteToGlobal(ctx context.Context, store *Store, title, content string, tags []string) (string, error) {
	cleaned := Depersonalize(content)

	// Safety gate: block if PII survived depersonalization.
	if ContainsPII(cleaned) {
		return "", ErrPIIDetected
	}

	// Merge "community" tag to signal global origin.
	allTags := appendIfMissing(tags, "community")

	docID, err := store.Create(TypeInsight, map[string]any{
		"title":      title,
		"tags":       allTags,
		"visibility": "public",
		"status":     "pending",
	}, cleaned)
	if err != nil {
		return "", err
	}
	return docID, nil
}

// appendIfMissing adds tag to slice if not already present.
func appendIfMissing(tags []string, tag string) []string {
	for _, t := range tags {
		if t == tag {
			return tags
		}
	}
	out := make([]string, len(tags)+1)
	copy(out, tags)
	out[len(tags)] = tag
	return out
}
