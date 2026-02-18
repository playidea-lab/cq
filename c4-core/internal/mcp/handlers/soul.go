package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/changmin/c4-core/internal/eventbus"
	"github.com/changmin/c4-core/internal/mcp"
)

var soulEventPub eventbus.Publisher
var soulProjectID string

// SetSoulEventBus sets the EventBus publisher and project ID for soul event publishing.
func SetSoulEventBus(pub eventbus.Publisher, projectID string) {
	soulEventPub = pub
	soulProjectID = projectID
}

// StageToRoles maps workflow stages to the soul roles that should be active.
// Multiple roles can be active simultaneously for cross-validation.
var StageToRoles = map[string][]string{
	"INIT":       {"ceo"},
	"DISCOVERY":  {"ceo"},
	"DESIGN":     {"ceo", "designer"},
	"PLAN":       {"ceo"},
	"EXECUTE":    {"developer"},
	"CHECKPOINT": {"developer", "ceo"},
	"COMPLETE":   {"ceo"},
}

// GetActiveRolesForStage returns the soul roles that should be active for a workflow stage.
// If projectRoot is provided via SetProjectRootForRoles, includes the project role.
func GetActiveRolesForStage(stage string) []string {
	roles := StageToRoles[stage]
	if len(roles) == 0 {
		roles = []string{"developer"}
	}

	// Append project role if configured
	if projectRoleForStage != "" {
		// Check if already in list
		found := false
		for _, r := range roles {
			if r == projectRoleForStage {
				found = true
				break
			}
		}
		if !found {
			roles = append(roles, projectRoleForStage)
		}
	}

	return roles
}

// projectRoleForStage holds the project-specific role name (e.g. "project-c4").
// Set by SetProjectRoleForStage during initialization.
var projectRoleForStage string

// SetProjectRoleForStage sets the project role that will be included in all stages.
func SetProjectRoleForStage(role string) {
	projectRoleForStage = role
}

// soulTemplate is the default content for a new soul file.
const soulTemplate = `# Soul: %s — %s

## Principles
> 이 역할에서의 판단 기준과 가치관

- (여기에 원칙을 추가하세요)

## Preferences
> 선호하는 도구, 스타일, 접근 방식

- (여기에 선호도를 추가하세요)

## Overrides
> 팀 persona 대비 개인 override 항목

- (persona 기본값과 다른 점을 기록하세요)

## Learned
> 자동 축적되는 학습 내용 (persona_evolve → soul 반영)

`

// soulSections defines valid soul file sections.
var soulSections = map[string]bool{
	"Principles": true,
	"Preferences": true,
	"Overrides":  true,
	"Learned":    true,
}

// validSoulID matches safe username/role identifiers (alphanumeric, dash, underscore).
var validSoulID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// RegisterSoulHandlers registers c4_soul_get, c4_soul_set, and c4_soul_resolve MCP tools.
func RegisterSoulHandlers(reg *mcp.Registry, projectRoot string) {
	// c4_soul_get
	reg.Register(mcp.ToolSchema{
		Name:        "c4_soul_get",
		Description: "Get user's soul file for a role, or list all souls",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"username": map[string]any{
					"type":        "string",
					"description": "Username to look up",
				},
				"role": map[string]any{
					"type":        "string",
					"description": "Role (e.g., 'developer', 'ceo', 'designer'). Omit to list all souls for user.",
				},
			},
			"required": []string{"username"},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		var args struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("parsing arguments: %w", err)
		}

		if !validSoulID.MatchString(args.Username) {
			return nil, fmt.Errorf("invalid username: must be alphanumeric, dash, or underscore")
		}

		soulsDir := filepath.Join(projectRoot, ".c4", "souls", args.Username)

		// If role specified, return that soul
		if args.Role != "" {
			if !validSoulID.MatchString(args.Role) {
				return nil, fmt.Errorf("invalid role: must be alphanumeric, dash, or underscore")
			}
			return getSoul(projectRoot, soulsDir, args.Username, args.Role)
		}

		// List all souls for user
		return listSouls(projectRoot, soulsDir, args.Username)
	})

	// c4_soul_set
	reg.Register(mcp.ToolSchema{
		Name:        "c4_soul_set",
		Description: "Set a section of user's soul file (Principles/Preferences/Overrides/Learned)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"username": map[string]any{
					"type":        "string",
					"description": "Username",
				},
				"role": map[string]any{
					"type":        "string",
					"description": "Role (e.g., 'developer', 'ceo', 'designer')",
				},
				"section": map[string]any{
					"type":        "string",
					"description": "Section to update: Principles, Preferences, Overrides, or Learned",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "New content for the section (markdown)",
				},
			},
			"required": []string{"username", "role", "section", "content"},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		var args struct {
			Username string `json:"username"`
			Role     string `json:"role"`
			Section  string `json:"section"`
			Content  string `json:"content"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("parsing arguments: %w", err)
		}

		if !validSoulID.MatchString(args.Username) {
			return nil, fmt.Errorf("invalid username: must be alphanumeric, dash, or underscore")
		}
		if !validSoulID.MatchString(args.Role) {
			return nil, fmt.Errorf("invalid role: must be alphanumeric, dash, or underscore")
		}
		if !soulSections[args.Section] {
			valid := make([]string, 0, len(soulSections))
			for k := range soulSections {
				valid = append(valid, k)
			}
			return nil, fmt.Errorf("invalid section %q: must be one of %v", args.Section, valid)
		}

		soulPath := filepath.Join(projectRoot, ".c4", "souls", args.Username, fmt.Sprintf("soul-%s.md", args.Role))

		return setSoulSection(projectRoot, soulPath, args.Username, args.Role, args.Section, args.Content)
	})

	// c4_soul_resolve
	reg.Register(mcp.ToolSchema{
		Name:        "c4_soul_resolve",
		Description: "Resolve merged persona+soul for a user's role",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"username": map[string]any{
					"type":        "string",
					"description": "Username",
				},
				"role": map[string]any{
					"type":        "string",
					"description": "Role (e.g., 'developer', 'ceo', 'designer')",
				},
			},
			"required": []string{"username", "role"},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		var args struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("parsing arguments: %w", err)
		}

		if !validSoulID.MatchString(args.Username) {
			return nil, fmt.Errorf("invalid username: must be alphanumeric, dash, or underscore")
		}
		if !validSoulID.MatchString(args.Role) {
			return nil, fmt.Errorf("invalid role: must be alphanumeric, dash, or underscore")
		}

		return ResolveSoul(projectRoot, args.Username, args.Role)
	})
}

// getSoul returns soul content for a specific user+role, falling back to persona.
func getSoul(projectRoot, soulsDir, username, role string) (map[string]any, error) {
	soulPath := filepath.Join(soulsDir, fmt.Sprintf("soul-%s.md", role))

	data, err := os.ReadFile(soulPath)
	if err == nil {
		// Soul file exists
		sections := parseSoulSections(string(data))
		return map[string]any{
			"username": username,
			"role":     role,
			"path":     soulPath,
			"content":  string(data),
			"sections": sections,
			"source":   "soul",
		}, nil
	}

	// Fallback: try matching persona file
	personaContent, personaPath := findPersonaForRole(projectRoot, role)
	if personaContent != "" {
		return map[string]any{
			"username":      username,
			"role":          role,
			"content":       personaContent,
			"persona_path":  personaPath,
			"source":        "persona_fallback",
			"message":       fmt.Sprintf("No soul file for %s/%s. Showing persona fallback. Use c4_soul_set to create a personal soul.", username, role),
		}, nil
	}

	return map[string]any{
		"username": username,
		"role":     role,
		"source":   "none",
		"message":  fmt.Sprintf("No soul or persona found for %s/%s. Use c4_soul_set to create one.", username, role),
	}, nil
}

// listSouls returns all souls for a user.
func listSouls(projectRoot, soulsDir, username string) (map[string]any, error) {
	entries, err := os.ReadDir(soulsDir)
	if err != nil {
		// No souls directory yet
		return map[string]any{
			"username": username,
			"souls":   []map[string]any{},
			"message":  "No souls found. Use c4_soul_set to create one.",
		}, nil
	}

	souls := []map[string]any{}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "soul-") || !strings.HasSuffix(name, ".md") {
			continue
		}
		role := strings.TrimPrefix(name, "soul-")
		role = strings.TrimSuffix(role, ".md")

		info, _ := entry.Info()
		soul := map[string]any{
			"role": role,
			"path": filepath.Join(soulsDir, name),
		}
		if info != nil {
			soul["size"] = info.Size()
			soul["modified"] = info.ModTime().Format("2006-01-02 15:04:05")
		}
		souls = append(souls, soul)
	}

	return map[string]any{
		"username": username,
		"souls":   souls,
		"count":    len(souls),
	}, nil
}

// setSoulSection creates or updates a section in a soul file.
func setSoulSection(projectRoot, soulPath, username, role, section, content string) (map[string]any, error) {
	// Ensure directory exists
	dir := filepath.Dir(soulPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating soul directory: %w", err)
	}

	// Read existing or create from template
	existing, err := os.ReadFile(soulPath)
	var fileContent string
	created := false
	if err != nil {
		// New soul file — use template
		fileContent = fmt.Sprintf(soulTemplate, username, role)
		created = true
	} else {
		fileContent = string(existing)
	}

	// Update the section
	updated := updateSection(fileContent, section, content)

	// Atomic write: tmp + rename
	tmpPath := soulPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(updated), 0644); err != nil {
		return nil, fmt.Errorf("writing soul file: %w", err)
	}
	if err := os.Rename(tmpPath, soulPath); err != nil {
		os.Remove(tmpPath) // cleanup
		return nil, fmt.Errorf("renaming soul file: %w", err)
	}

	action := "updated"
	if created {
		action = "created"
	}
	if soulEventPub != nil {
		payload, _ := json.Marshal(map[string]any{"username": username, "role": role, "section": section, "action": action})
		soulEventPub.PublishAsync("soul.updated", "c4.soul", payload, soulProjectID)
	}
	return map[string]any{
		"success":  true,
		"username": username,
		"role":     role,
		"section":  section,
		"path":     soulPath,
		"action":   action,
		"message":  fmt.Sprintf("Soul %s/%s section '%s' %s", username, role, section, action),
	}, nil
}

// parseSoulSections extracts section contents from a soul markdown file.
func parseSoulSections(content string) map[string]string {
	sections := make(map[string]string)
	lines := strings.Split(content, "\n")

	var currentSection string
	var sectionLines []string

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			// Save previous section
			if currentSection != "" {
				sections[currentSection] = strings.TrimSpace(strings.Join(sectionLines, "\n"))
			}
			currentSection = strings.TrimPrefix(line, "## ")
			sectionLines = nil
		} else if currentSection != "" {
			sectionLines = append(sectionLines, line)
		}
	}
	// Save last section
	if currentSection != "" {
		sections[currentSection] = strings.TrimSpace(strings.Join(sectionLines, "\n"))
	}

	return sections
}

// updateSection replaces a ## section's content in markdown.
func updateSection(fileContent, section, newContent string) string {
	lines := strings.Split(fileContent, "\n")
	var result []string

	header := "## " + section
	inSection := false
	replaced := false

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			if inSection {
				// End of target section — content already written
				inSection = false
			}
			if line == header {
				// Found target section
				result = append(result, line)
				result = append(result, newContent)
				result = append(result, "")
				inSection = true
				replaced = true
				continue
			}
		}
		if !inSection {
			result = append(result, line)
		}
	}

	// If section not found, append it
	if !replaced {
		result = append(result, "")
		result = append(result, header)
		result = append(result, newContent)
		result = append(result, "")
	}

	return strings.Join(result, "\n")
}

// ResolveSoul merges persona (team default) with soul (personal override) for a user+role.
// Returns the merged instruction text plus debug info about sources.
func ResolveSoul(projectRoot, username, role string) (map[string]any, error) {
	// 1. Read persona (team default)
	personaContent, personaPath := findPersonaForRole(projectRoot, role)

	// 2. Read soul (personal override)
	soulPath := filepath.Join(projectRoot, ".c4", "souls", username, fmt.Sprintf("soul-%s.md", role))
	soulData, soulErr := os.ReadFile(soulPath)

	hasSoul := soulErr == nil
	hasPersona := personaContent != ""

	// 3. Build merged output
	var merged strings.Builder
	soulSource := "none"
	personaSource := "none"

	if hasPersona {
		personaSource = personaPath
	}
	if hasSoul {
		soulSource = soulPath
	}

	switch {
	case hasPersona && hasSoul:
		// Full merge: persona base + soul overrides
		soulSections := parseSoulSections(string(soulData))

		merged.WriteString(fmt.Sprintf("# %s — %s (persona + soul merged)\n\n", username, role))
		merged.WriteString("## Base Persona\n")
		merged.WriteString(personaContent)
		merged.WriteString("\n\n---\n\n")
		merged.WriteString("## Personal Soul Overrides\n\n")

		for _, section := range []string{"Principles", "Preferences", "Overrides", "Learned"} {
			content, exists := soulSections[section]
			if exists && content != "" {
				merged.WriteString(fmt.Sprintf("### %s\n%s\n\n", section, content))
			}
		}

	case hasSoul && !hasPersona:
		// Soul only (no matching persona)
		merged.WriteString(string(soulData))

	case hasPersona && !hasSoul:
		// Persona only (no personal soul yet)
		merged.WriteString(personaContent)

	default:
		return map[string]any{
			"username":       username,
			"role":           role,
			"merged":         "",
			"persona_source": personaSource,
			"soul_source":    soulSource,
			"message":        fmt.Sprintf("No persona or soul found for %s/%s", username, role),
		}, nil
	}

	return map[string]any{
		"username":       username,
		"role":           role,
		"merged":         merged.String(),
		"persona_source": personaSource,
		"soul_source":    soulSource,
		"has_persona":    hasPersona,
		"has_soul":       hasSoul,
	}, nil
}

// findPersonaForRole looks for a persona file that matches the role name.
// Matching priority: exact match > role contains personaName (longest wins).
// Does NOT match when personaName contains role (e.g., "developer" must not
// match "frontend-developer").
func findPersonaForRole(projectRoot, role string) (string, string) {
	personasDir := filepath.Join(projectRoot, ".c4", "personas")

	// Try exact match: persona-{role}.md
	exactPath := filepath.Join(personasDir, fmt.Sprintf("persona-%s.md", role))
	data, err := os.ReadFile(exactPath)
	if err == nil {
		return string(data), exactPath
	}

	// Try partial match: only role contains personaName (not the reverse).
	// Pick the longest (most specific) personaName that matches.
	entries, err := os.ReadDir(personasDir)
	if err != nil {
		return "", ""
	}

	var bestName string
	var bestPath string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "persona-") || !strings.HasSuffix(name, ".md") {
			continue
		}
		personaName := strings.TrimPrefix(name, "persona-")
		personaName = strings.TrimSuffix(personaName, ".md")

		// Only match when role contains personaName (e.g., "senior-developer" contains "developer").
		// Skip when personaName contains role (e.g., "frontend-developer" contains "developer").
		if strings.Contains(role, personaName) && len(personaName) > len(bestName) {
			bestName = personaName
			bestPath = filepath.Join(personasDir, name)
		}
	}

	if bestPath != "" {
		data, err := os.ReadFile(bestPath)
		if err == nil {
			return string(data), bestPath
		}
	}

	return "", ""
}
