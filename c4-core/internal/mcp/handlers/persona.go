package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
	"gopkg.in/yaml.v3"
)

// PersonaStore extends Store with persona-specific methods.
type PersonaStore interface {
	GetPersonaStats(personaID string) (map[string]any, error)
	ListPersonas() ([]map[string]any, error)
}

// RegisterPersonaHandlers registers persona-related MCP tools.
// projectRoot is needed for soul file auto-update in persona_evolve.
func RegisterPersonaHandlers(reg *mcp.Registry, store *SQLiteStore) {
	projectRoot := store.projectRoot
	// c4_persona_stats
	reg.Register(mcp.ToolSchema{
		Name:        "c4_persona_stats",
		Description: "Get performance statistics for a persona (approved/rejected ratio, avg review score)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"persona_id": map[string]any{
					"type":        "string",
					"description": "Persona ID (e.g., 'code-reviewer', 'direct'). Omit to list all.",
				},
			},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		var args struct {
			PersonaID string `json:"persona_id"`
		}
		if len(rawArgs) > 0 {
			_ = json.Unmarshal(rawArgs, &args)
		}

		if args.PersonaID != "" {
			// Validate PersonaID: reject directory traversal characters
			if strings.Contains(args.PersonaID, "..") || strings.Contains(args.PersonaID, "/") || strings.Contains(args.PersonaID, "\\") {
				return nil, fmt.Errorf("invalid persona_id: must not contain path separators or '..'")
			}
			return store.GetPersonaStats(args.PersonaID)
		}

		personas, err := store.ListPersonas()
		if err != nil {
			return nil, fmt.Errorf("listing personas: %w", err)
		}
		if personas == nil {
			personas = []map[string]any{}
		}
		return map[string]any{"personas": personas}, nil
	})

	// c4_persona_evolve
	reg.Register(mcp.ToolSchema{
		Name:        "c4_persona_evolve",
		Description: "Suggest persona evolution based on task outcomes (auto-applies to Soul)",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"persona_id": map[string]any{
					"type":        "string",
					"description": "Persona ID to analyze",
				},
				"username": map[string]any{
					"type":        "string",
					"description": "Username for Soul auto-update. If omitted, reads from team.yaml.",
				},
				"apply": map[string]any{
					"type":        "boolean",
					"description": "Auto-apply suggestions to Soul's Learned section (default: true)",
				},
			},
			"required": []string{"persona_id"},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		var args struct {
			PersonaID string `json:"persona_id"`
			Username  string `json:"username"`
			Apply     *bool  `json:"apply"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("parsing arguments: %w", err)
		}

		// Validate PersonaID
		if strings.Contains(args.PersonaID, "..") || strings.Contains(args.PersonaID, "/") || strings.Contains(args.PersonaID, "\\") {
			return nil, fmt.Errorf("invalid persona_id: must not contain path separators or '..'")
		}

		stats, err := store.GetPersonaStats(args.PersonaID)
		if err != nil {
			return nil, err
		}

		total, _ := stats["total_tasks"].(int)
		if total == 0 {
			return map[string]any{
				"persona_id":  args.PersonaID,
				"suggestions": []string{},
				"message":     "No task history found. Complete some tasks first.",
			}, nil
		}

		// Analyze patterns
		suggestions := analyzePatternsForSuggestions(stats, total)

		// Auto-apply to soul's Learned section (default: true)
		apply := args.Apply == nil || *args.Apply
		applied := false

		if apply && len(suggestions) > 0 && projectRoot != "" {
			username := args.Username
			if username == "" {
				username = getActiveUsername(projectRoot)
			}
			if username != "" {
				if err := applySuggestionsToSoul(projectRoot, username, args.PersonaID, suggestions); err != nil {
					fmt.Fprintf(os.Stderr, "c4: persona_evolve apply failed: %v\n", err)
				} else {
					applied = true
				}
			}
		}

		return map[string]any{
			"persona_id":  args.PersonaID,
			"stats":       stats,
			"suggestions": suggestions,
			"applied":     applied,
			"message":     fmt.Sprintf("Analysis based on %d tasks", total),
		}, nil
	})
}

// TeamConfig represents .c4/team.yaml structure.
type TeamConfig struct {
	Members map[string]TeamMember `yaml:"members"`
}

// TeamMember represents a team member's configuration.
type TeamMember struct {
	Role          string   `yaml:"role"`
	Roles         []string `yaml:"roles"`
	Personas      []string `yaml:"personas"`
	ActivePersona string   `yaml:"active_persona"`
}

// RegisterTeamHandlers registers the c4_whoami tool.
func RegisterTeamHandlers(reg *mcp.Registry, projectRoot string) {
	// c4_whoami
	reg.Register(mcp.ToolSchema{
		Name:        "c4_whoami",
		Description: "Get or set current user identity, roles, and active persona",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"username": map[string]any{
					"type":        "string",
					"description": "Username to look up or register",
				},
				"role": map[string]any{
					"type":        "string",
					"description": "Primary role (e.g., 'frontend', 'backend', 'ceo')",
				},
				"roles": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "All roles this user can assume (e.g., ['developer', 'ceo']). Used for workflow-based Soul activation.",
				},
				"active_persona": map[string]any{
					"type":        "string",
					"description": "Set active persona for this session",
				},
			},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		var args struct {
			Username      string   `json:"username"`
			Role          string   `json:"role"`
			Roles         []string `json:"roles"`
			ActivePersona string   `json:"active_persona"`
		}
		if len(rawArgs) > 0 {
			_ = json.Unmarshal(rawArgs, &args)
		}

		teamPath := filepath.Join(projectRoot, ".c4", "team.yaml")

		// Load existing team config
		var team TeamConfig
		data, err := os.ReadFile(teamPath)
		if err == nil {
			_ = yaml.Unmarshal(data, &team)
		}
		if team.Members == nil {
			team.Members = make(map[string]TeamMember)
		}

		// If no username specified, return current team info
		if args.Username == "" {
			members := make([]map[string]any, 0, len(team.Members))
			for name, member := range team.Members {
				members = append(members, map[string]any{
					"username":       name,
					"role":           member.Role,
					"roles":          member.Roles,
					"personas":       member.Personas,
					"active_persona": member.ActivePersona,
				})
			}
			return map[string]any{
				"members": members,
				"message": fmt.Sprintf("%d team members registered", len(team.Members)),
			}, nil
		}

		// Update or create member
		member, exists := team.Members[args.Username]
		if !exists {
			member = TeamMember{}
		}
		if args.Role != "" {
			member.Role = args.Role
		}
		if len(args.Roles) > 0 {
			member.Roles = args.Roles
		}
		if args.ActivePersona != "" {
			member.ActivePersona = args.ActivePersona
			// Auto-add to personas list if not present
			found := false
			for _, p := range member.Personas {
				if p == args.ActivePersona {
					found = true
					break
				}
			}
			if !found {
				member.Personas = append(member.Personas, args.ActivePersona)
			}
		}

		team.Members[args.Username] = member

		// Save team config
		out, err := yaml.Marshal(team)
		if err != nil {
			return nil, fmt.Errorf("marshaling team config: %w", err)
		}
		_ = os.MkdirAll(filepath.Dir(teamPath), 0755)
		if err := os.WriteFile(teamPath, out, 0644); err != nil {
			return nil, fmt.Errorf("writing team config: %w", err)
		}

		// Find matching persona file
		personaFile := ""
		if member.ActivePersona != "" {
			personaGlob := filepath.Join(projectRoot, ".c4", "personas", "persona-*.md")
			matches, _ := filepath.Glob(personaGlob)
			for _, m := range matches {
				base := filepath.Base(m)
				name := strings.TrimPrefix(base, "persona-")
				name = strings.TrimSuffix(name, ".md")
				if name == member.ActivePersona {
					personaFile = m
					break
				}
			}
		}

		// List soul files for this user
		soulFiles := listUserSoulFiles(projectRoot, args.Username)

		result := map[string]any{
			"username":       args.Username,
			"role":           member.Role,
			"roles":          member.Roles,
			"personas":       member.Personas,
			"active_persona": member.ActivePersona,
			"soul_files":     soulFiles,
		}
		if personaFile != "" {
			result["persona_file"] = personaFile
		}
		return result, nil
	})
}

// listUserSoulFiles returns a list of soul role names for a user.
func listUserSoulFiles(projectRoot, username string) []string {
	soulsDir := filepath.Join(projectRoot, ".c4", "souls", username)
	entries, err := os.ReadDir(soulsDir)
	if err != nil {
		return []string{}
	}

	var roles []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "soul-") && strings.HasSuffix(name, ".md") {
			role := strings.TrimPrefix(name, "soul-")
			role = strings.TrimSuffix(role, ".md")
			roles = append(roles, role)
		}
	}
	return roles
}

// analyzePatternsForSuggestions generates suggestions from persona stats.
func analyzePatternsForSuggestions(stats map[string]any, total int) []string {
	suggestions := []string{}
	outcomes, _ := stats["outcomes"].(map[string]int)

	if rejected, ok := outcomes["rejected"]; ok && rejected > 0 {
		rate := float64(rejected) / float64(total) * 100
		if rate > 30 {
			suggestions = append(suggestions, fmt.Sprintf(
				"High rejection rate (%.0f%%). Review failure patterns and add pre-submit checklist.",
				rate))
		}
	}

	if avgScore, ok := stats["avg_review_score"].(float64); ok && avgScore < 0.7 {
		suggestions = append(suggestions, fmt.Sprintf(
			"Low average review score (%.2f). Focus on code quality and DoD compliance.",
			avgScore))
	}

	if total > 10 {
		suggestions = append(suggestions, "Consider specializing: check which domains have best outcomes.")
	}

	return suggestions
}

// getActivePersonaForUser reads team.yaml and returns the active_persona for a specific user.
func getActivePersonaForUser(projectRoot, username string) string {
	teamPath := filepath.Join(projectRoot, ".c4", "team.yaml")
	data, err := os.ReadFile(teamPath)
	if err != nil {
		return ""
	}

	var team TeamConfig
	if err := yaml.Unmarshal(data, &team); err != nil {
		return ""
	}

	if member, ok := team.Members[username]; ok {
		return member.ActivePersona
	}
	return ""
}

// getActiveUsername reads team.yaml and returns the first member's username.
func getActiveUsername(projectRoot string) string {
	teamPath := filepath.Join(projectRoot, ".c4", "team.yaml")
	data, err := os.ReadFile(teamPath)
	if err != nil {
		return ""
	}

	var team TeamConfig
	if err := yaml.Unmarshal(data, &team); err != nil {
		return ""
	}

	// Return first member (solo mode)
	for name := range team.Members {
		return name
	}
	return ""
}

// applySuggestionsToSoul appends suggestions to a user's soul Learned section.
func applySuggestionsToSoul(projectRoot, username, role string, suggestions []string) error {
	soulPath := filepath.Join(projectRoot, ".c4", "souls", username, fmt.Sprintf("soul-%s.md", role))

	// Read existing soul or create new
	existing, err := os.ReadFile(soulPath)
	var fileContent string
	if err != nil {
		// Create soul directory and file
		dir := filepath.Dir(soulPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating soul directory: %w", err)
		}
		fileContent = fmt.Sprintf(soulTemplate, username, role)
	} else {
		fileContent = string(existing)
	}

	// Build new Learned content: append suggestions with timestamp
	sections := parseSoulSections(fileContent)
	existingLearned := sections["Learned"]

	date := time.Now().Format("2006-01-02")
	var newLines []string

	// Build set of existing suggestion texts for exact dedup
	existingSuggestions := make(map[string]bool)
	for _, line := range strings.Split(existingLearned, "\n") {
		if idx := strings.Index(line, "] "); idx >= 0 {
			existingSuggestions[strings.TrimSpace(line[idx+2:])] = true
		}
	}

	for _, s := range suggestions {
		// Deduplicate by exact suggestion text (not substring)
		if !existingSuggestions[s] {
			newLines = append(newLines, fmt.Sprintf("- [%s] %s", date, s))
		}
	}

	if len(newLines) == 0 {
		return nil // nothing new to add
	}

	// Append to existing Learned content
	newLearned := existingLearned
	if newLearned != "" {
		newLearned += "\n"
	}
	newLearned += strings.Join(newLines, "\n")

	updated := updateSection(fileContent, "Learned", newLearned)

	// Atomic write (tmp + rename)
	tmpPath := soulPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(updated), 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, soulPath); err != nil {
		os.Remove(tmpPath) // cleanup orphaned tmp
		return fmt.Errorf("atomic rename failed: %w", err)
	}
	return nil
}
