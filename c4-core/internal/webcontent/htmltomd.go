package webcontent

import (
	"regexp"
	"strings"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

// ConvertHTMLToMarkdown converts raw HTML to clean Markdown.
// It strips <script>, <style>, <nav>, <footer>, and <header> tags before conversion.
func ConvertHTMLToMarkdown(htmlContent string) (string, error) {
	if strings.TrimSpace(htmlContent) == "" {
		return "", nil
	}

	cleaned := stripTags(htmlContent, "script", "style", "nav", "footer", "header")

	md, err := htmltomarkdown.ConvertString(cleaned)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(md), nil
}

// ExtractTitle extracts the page title from HTML.
// It first looks for <title>, then falls back to the first <h1>.
func ExtractTitle(htmlContent string) string {
	// Try <title> first
	re := regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	if m := re.FindStringSubmatch(htmlContent); len(m) > 1 {
		t := strings.TrimSpace(m[1])
		if t != "" {
			return t
		}
	}

	// Fallback to first <h1>
	re = regexp.MustCompile(`(?is)<h1[^>]*>(.*?)</h1>`)
	if m := re.FindStringSubmatch(htmlContent); len(m) > 1 {
		// Strip any nested HTML tags
		stripped := regexp.MustCompile(`<[^>]*>`).ReplaceAllString(m[1], "")
		t := strings.TrimSpace(stripped)
		if t != "" {
			return t
		}
	}

	return ""
}

// stripTags removes entire tag blocks (opening through closing) for the given tag names.
func stripTags(html string, tags ...string) string {
	for _, tag := range tags {
		re := regexp.MustCompile(`(?is)<` + tag + `[^>]*>.*?</` + tag + `>`)
		html = re.ReplaceAllString(html, "")
	}
	return html
}
