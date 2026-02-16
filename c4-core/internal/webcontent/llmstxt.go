package webcontent

import (
	"regexp"
	"strings"
)

// LLMSTxt represents a parsed llms.txt file.
type LLMSTxt struct {
	Title       string
	Description string
	Sections    []LLMSSection
}

// LLMSSection is a section within llms.txt.
type LLMSSection struct {
	Title string
	Links []LLMSLink
}

// LLMSLink is a link entry within a section.
type LLMSLink struct {
	Title       string
	URL         string
	Description string
}

var linkRe = regexp.MustCompile(`^\s*-\s+\[([^\]]+)\]\(([^)]+)\)(?:\s*:\s*(.+))?$`)

// ParseLLMSTxt parses llms.txt format content.
// Format: # title, > description, ## sections, - [link](url): description
func ParseLLMSTxt(content string) (*LLMSTxt, error) {
	result := &LLMSTxt{}
	lines := strings.Split(content, "\n")

	var currentSection *LLMSSection

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Title: # heading
		if strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "## ") {
			if result.Title == "" {
				result.Title = strings.TrimPrefix(trimmed, "# ")
			}
			continue
		}

		// Description: > blockquote
		if strings.HasPrefix(trimmed, "> ") {
			desc := strings.TrimPrefix(trimmed, "> ")
			if result.Description == "" {
				result.Description = desc
			} else {
				result.Description += " " + desc
			}
			continue
		}

		// Section: ## heading
		if strings.HasPrefix(trimmed, "## ") {
			if currentSection != nil {
				result.Sections = append(result.Sections, *currentSection)
			}
			currentSection = &LLMSSection{
				Title: strings.TrimPrefix(trimmed, "## "),
			}
			continue
		}

		// Link: - [title](url): description
		if m := linkRe.FindStringSubmatch(line); len(m) > 0 {
			link := LLMSLink{
				Title: m[1],
				URL:   m[2],
			}
			if len(m) > 3 {
				link.Description = strings.TrimSpace(m[3])
			}
			if currentSection != nil {
				currentSection.Links = append(currentSection.Links, link)
			}
			continue
		}
	}

	if currentSection != nil {
		result.Sections = append(result.Sections, *currentSection)
	}

	return result, nil
}
