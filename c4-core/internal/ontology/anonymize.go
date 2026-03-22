package ontology

import (
	"path/filepath"
	"regexp"
	"strings"
)

// AnonymizeThreshold is the minimum frequency required for a node to be
// included in the anonymized pattern set sent to the Hub.
const AnonymizeThreshold = 5

// AnonPattern is a single anonymized ontology node ready for Hub upload.
// All project-specific identifiers (project_id, username, file paths) have
// been removed; only the structural concept and its frequency remain.
type AnonPattern struct {
	// Domain is the project domain (e.g. "go-backend"), read from config.yaml.
	Domain string
	// Path is the concept identifier with personal/path segments stripped.
	Path string
	// Value is the concept label/description, sanitized of PII.
	Value string
	// Frequency is the raw observation count from the project ontology.
	Frequency int
	// Confidence is the node confidence level ("low", "medium", "high").
	Confidence string
	// Tags are the node tags, preserved as-is.
	Tags []string
}

// absPathRe matches absolute Unix/Windows paths (e.g. /home/user/..., C:\...).
var absPathRe = regexp.MustCompile(`(?:^|[\s:=,])(?:/[^\s,;:]+|[A-Za-z]:\\[^\s,;:]+)`)

// Anonymize filters nodes from proj with Frequency >= AnonymizeThreshold,
// strips project_id, username, and file-path segments from paths/values,
// and returns the result as []AnonPattern tagged with domain.
//
// Nodes below the threshold or with empty labels are silently skipped.
func Anonymize(proj *ProjectOntology, domain string) []AnonPattern {
	if proj == nil {
		return nil
	}

	var out []AnonPattern
	for rawPath, node := range proj.Schema.Nodes {
		if node.Frequency < AnonymizeThreshold {
			continue
		}
		if node.Label == "" {
			continue
		}

		cleanPath := sanitizePath(rawPath)
		cleanValue := sanitizeValue(node.Label)

		out = append(out, AnonPattern{
			Domain:     domain,
			Path:       cleanPath,
			Value:      cleanValue,
			Frequency:  node.Frequency,
			Confidence: string(node.NodeConfidence),
			Tags:       node.Tags,
		})
	}
	return out
}

// sanitizePath strips file-path segments and known-personal prefixes from a
// concept path key (e.g. "src/user/project/api" → "api").
func sanitizePath(p string) string {
	// If the path looks like an absolute or relative file path, reduce to the
	// base name without extension.
	if strings.ContainsRune(p, '/') || strings.ContainsRune(p, '\\') {
		base := filepath.Base(p)
		// Strip extension if present.
		if ext := filepath.Ext(base); ext != "" {
			base = strings.TrimSuffix(base, ext)
		}
		return base
	}
	return p
}

// sanitizeValue removes absolute paths from a value string, replacing them
// with a placeholder, then trims surrounding whitespace.
func sanitizeValue(v string) string {
	clean := absPathRe.ReplaceAllStringFunc(v, func(match string) string {
		// Preserve any leading non-path character (space, colon, etc.).
		for i, ch := range match {
			if ch == '/' || (ch >= 'A' && ch <= 'Z' && strings.HasPrefix(match[i+1:], `:\`)) {
				prefix := match[:i]
				return prefix + "<path>"
			}
		}
		return "<path>"
	})
	return strings.TrimSpace(clean)
}
