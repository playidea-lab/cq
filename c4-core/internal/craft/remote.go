package craft

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RemoteSource represents a parsed GitHub URL or shorthand.
type RemoteSource struct {
	Owner string
	Repo  string
	Path  string // e.g., "skills/pdf"
	Ref   string // branch, default "main"
	URL   string // original input for source tracking
}

// ParseGitHubURL parses various GitHub URL formats:
//
//   - https://github.com/owner/repo/tree/main/path/to/skill
//   - github.com/owner/repo/tree/main/path/to/skill
//   - owner/repo:path/to/skill (shorthand)
//   - owner/repo:skill-name (shorthand, assumes skills/ prefix when no slash in name)
func ParseGitHubURL(input string) (*RemoteSource, error) {
	original := input

	// Strip scheme
	input = strings.TrimPrefix(input, "https://")
	input = strings.TrimPrefix(input, "http://")

	// Full GitHub tree URL: github.com/owner/repo/tree/ref/path...
	if strings.HasPrefix(input, "github.com/") {
		parts := strings.SplitN(strings.TrimPrefix(input, "github.com/"), "/", -1)
		// Minimum: owner/repo/tree/ref
		if len(parts) < 4 {
			return nil, fmt.Errorf("craft: remote: invalid GitHub URL %q", original)
		}
		if parts[2] != "tree" {
			return nil, fmt.Errorf("craft: remote: expected /tree/ in GitHub URL, got %q", original)
		}
		owner := parts[0]
		repo := parts[1]
		ref := parts[3]
		path := strings.Join(parts[4:], "/")
		if path == "" {
			return nil, fmt.Errorf("craft: remote: no path in GitHub URL %q", original)
		}
		return &RemoteSource{
			Owner: owner,
			Repo:  repo,
			Path:  path,
			Ref:   ref,
			URL:   original,
		}, nil
	}

	// Shorthand: owner/repo:name  or  owner/repo:path/to/skill
	if strings.Contains(input, ":") {
		colonIdx := strings.Index(input, ":")
		repoSpec := input[:colonIdx]
		name := input[colonIdx+1:]

		slashParts := strings.SplitN(repoSpec, "/", 2)
		if len(slashParts) != 2 {
			return nil, fmt.Errorf("craft: remote: shorthand must be owner/repo:name, got %q", original)
		}
		owner := slashParts[0]
		repo := slashParts[1]

		// If name has no slash, assume skills/ prefix
		if !strings.Contains(name, "/") {
			name = "skills/" + name
		}

		return &RemoteSource{
			Owner: owner,
			Repo:  repo,
			Path:  name,
			Ref:   "main",
			URL:   original,
		}, nil
	}

	return nil, fmt.Errorf("craft: remote: unrecognised source format %q (expected github.com/... or owner/repo:name)", original)
}

// ghContent mirrors the GitHub Contents API response.
type ghContent struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`     // "file" or "dir"
	Content     string `json:"content"`  // base64-encoded, only for files
	Encoding    string `json:"encoding"` // "base64"
	DownloadURL string `json:"download_url"`
}

// FetchAndInstall fetches a directory (or single file) from GitHub via the
// Contents API and installs it under ~/.claude/.
//
// Type detection:
//   - directory with SKILL.md → skill → ~/.claude/skills/{name}/
//   - single .md with frontmatter description → agent → ~/.claude/agents/{name}.md
//   - single .md without frontmatter description → rule → ~/.claude/rules/{name}.md
//
// Prepends "# source: {URL}" to the primary file so Update can re-fetch it.
func FetchAndInstall(src *RemoteSource, homeDir string) (string, error) {
	token := getGHToken()

	apiURL := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/contents/%s?ref=%s",
		src.Owner, src.Repo, src.Path, src.Ref,
	)

	body, err := ghGet(apiURL, token)
	if err != nil {
		return "", fmt.Errorf("craft: remote: fetch %s: %w", apiURL, err)
	}

	// Try to unmarshal as array first (directory listing).
	var entries []ghContent
	if jsonErr := json.Unmarshal(body, &entries); jsonErr == nil && len(entries) > 0 {
		return installDirectory(src, entries, token, homeDir)
	}

	// Fall back to single-file object.
	var single ghContent
	if err := json.Unmarshal(body, &single); err != nil {
		return "", fmt.Errorf("craft: remote: parse response: %w", err)
	}
	return installSingleFile(src, &single, homeDir)
}

// Update re-fetches from the source URL recorded in the installed file and
// overwrites the existing installation.
func Update(name string, homeDir string) error {
	sourceURL, err := findSourceURL(name, homeDir)
	if err != nil {
		return fmt.Errorf("craft: remote: update %q: %w", name, err)
	}

	src, err := ParseGitHubURL(sourceURL)
	if err != nil {
		return fmt.Errorf("craft: remote: update %q: parse source URL: %w", name, err)
	}

	// Remove existing installation before re-fetching.
	if removeErr := Remove(name, homeDir); removeErr != nil {
		// Best-effort; proceed anyway (files will be overwritten).
		fmt.Fprintf(os.Stderr, "craft: remote: update: remove warning: %v\n", removeErr)
	}

	dest, err := FetchAndInstall(src, homeDir)
	if err != nil {
		return err
	}
	fmt.Printf("✓ %s 업데이트 → %s\n  source: %s\n", name, dest, sourceURL)
	return nil
}

// getGHToken attempts to retrieve an authenticated token via gh CLI.
// Returns an empty string when gh is not installed or not authenticated.
func getGHToken() string {
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ghGet performs a GET request against the GitHub API with an optional bearer token.
func ghGet(url, token string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return data, nil
	case http.StatusNotFound:
		return nil, fmt.Errorf("not found (404) — check owner/repo/path and visibility")
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, fmt.Errorf("rate limited or forbidden (%d) — run `gh auth login` for higher limits", resp.StatusCode)
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("unauthorised (401) — private repo requires `gh auth login`")
	default:
		return nil, fmt.Errorf("unexpected HTTP %d", resp.StatusCode)
	}
}

// decodeContent base64-decodes a GitHub API file content field (which may
// contain newlines inserted by the API).
func decodeContent(encoded string) ([]byte, error) {
	clean := strings.ReplaceAll(encoded, "\n", "")
	return base64.StdEncoding.DecodeString(clean)
}

// skillName returns the last path segment (the leaf directory/file name).
func skillName(path string) string {
	parts := strings.Split(strings.TrimRight(path, "/"), "/")
	return parts[len(parts)-1]
}

// installDirectory installs a GitHub directory as a skill (if SKILL.md found)
// or falls back to installing the first .md file as agent/rule.
func installDirectory(src *RemoteSource, entries []ghContent, token, homeDir string) (string, error) {
	name := skillName(src.Path)

	// Check for SKILL.md among entries.
	hasSkillMd := false
	for _, e := range entries {
		if e.Name == "SKILL.md" {
			hasSkillMd = true
			break
		}
	}

	if hasSkillMd {
		return installSkillDir(src, name, entries, token, homeDir)
	}

	// No SKILL.md — try to find a single .md file to install as agent/rule.
	var mdEntries []ghContent
	for _, e := range entries {
		if strings.HasSuffix(e.Name, ".md") {
			mdEntries = append(mdEntries, e)
		}
	}
	if len(mdEntries) == 0 {
		return "", fmt.Errorf("craft: remote: no SKILL.md or .md files found in %s/%s@%s/%s",
			src.Owner, src.Repo, src.Ref, src.Path)
	}
	return installSingleFile(src, &mdEntries[0], homeDir)
}

// installSkillDir downloads all files in the directory and places them under
// ~/.claude/skills/{name}/.
func installSkillDir(src *RemoteSource, name string, entries []ghContent, token, homeDir string) (string, error) {
	destDir := filepath.Join(homeDir, ".claude", "skills", name)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("craft: remote: mkdir %s: %w", destDir, err)
	}

	primaryDest := filepath.Join(destDir, "SKILL.md")

	for _, entry := range entries {
		if entry.Type != "file" {
			continue
		}

		data, err := fetchFileContent(&entry, token)
		if err != nil {
			return "", fmt.Errorf("craft: remote: fetch %s: %w", entry.Name, err)
		}

		// Prepend source comment to SKILL.md.
		if entry.Name == "SKILL.md" {
			data = prependSourceComment(data, src.URL)
		}

		dest := filepath.Join(destDir, entry.Name)
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return "", fmt.Errorf("craft: remote: write %s: %w", dest, err)
		}
	}

	return primaryDest, nil
}

// installSingleFile installs one .md file as an agent or rule.
func installSingleFile(src *RemoteSource, entry *ghContent, homeDir string) (string, error) {
	data, err := fetchFileContent(entry, "")
	if err != nil {
		return "", fmt.Errorf("craft: remote: fetch file: %w", err)
	}

	data = prependSourceComment(data, src.URL)

	// Determine type by frontmatter description presence.
	var pType PresetType
	if parseFrontmatterDescription(data) != "" {
		pType = TypeAgent
	} else {
		pType = TypeRule
	}

	name := skillName(src.Path)
	// Strip .md suffix from path-derived name if present.
	name = strings.TrimSuffix(name, ".md")

	var dest string
	switch pType {
	case TypeAgent:
		dest = filepath.Join(homeDir, ".claude", "agents", name+".md")
	default:
		dest = filepath.Join(homeDir, ".claude", "rules", name+".md")
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", fmt.Errorf("craft: remote: mkdir: %w", err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", fmt.Errorf("craft: remote: write: %w", err)
	}
	return dest, nil
}

// fetchFileContent retrieves the decoded byte content of a ghContent entry.
// If Content is populated (from a direct file endpoint), decode it.
// Otherwise fall back to DownloadURL.
func fetchFileContent(entry *ghContent, token string) ([]byte, error) {
	if entry.Content != "" && entry.Encoding == "base64" {
		return decodeContent(entry.Content)
	}
	if entry.DownloadURL == "" {
		return nil, fmt.Errorf("no content or download_url for %s", entry.Name)
	}
	return ghGet(entry.DownloadURL, token)
}

// prependSourceComment inserts "# source: <url>" as the first line of data,
// preserving any existing content.
func prependSourceComment(data []byte, url string) []byte {
	comment := []byte("# source: " + url + "\n")
	return append(comment, data...)
}

// findSourceURL scans the primary file for a "# source:" line and returns the URL.
func findSourceURL(name, homeDir string) (string, error) {
	candidates := []string{
		filepath.Join(homeDir, ".claude", "skills", name, "SKILL.md"),
		filepath.Join(homeDir, ".claude", "agents", name+".md"),
		filepath.Join(homeDir, ".claude", "rules", name+".md"),
	}

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "# source:") {
				return strings.TrimSpace(strings.TrimPrefix(line, "# source:")), nil
			}
		}
	}

	return "", fmt.Errorf("no '# source:' line found — was this installed via cq add <url>?")
}

// IsRemoteSource reports whether arg looks like a GitHub URL or shorthand.
// Used by cmd/c4/craft.go to route `cq add <arg>`.
func IsRemoteSource(arg string) bool {
	stripped := strings.TrimPrefix(arg, "https://")
	stripped = strings.TrimPrefix(stripped, "http://")
	if strings.HasPrefix(stripped, "github.com/") {
		return true
	}
	// owner/repo:name shorthand
	if strings.Contains(arg, ":") && strings.Contains(arg, "/") {
		colonIdx := strings.Index(arg, ":")
		before := arg[:colonIdx]
		return strings.Count(before, "/") == 1
	}
	return false
}
