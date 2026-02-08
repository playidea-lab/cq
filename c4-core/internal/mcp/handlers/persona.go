package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/changmin/c4-core/internal/mcp"
	"gopkg.in/yaml.v3"
)

// PersonaStore extends Store with persona-specific methods.
type PersonaStore interface {
	GetPersonaStats(personaID string) (map[string]any, error)
	ListPersonas() ([]map[string]any, error)
}

// RegisterPersonaHandlers registers persona-related MCP tools.
func RegisterPersonaHandlers(reg *mcp.Registry, store *SQLiteStore) {
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
		Description: "Suggest evolution rules for a persona based on task outcomes and feedback patterns",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"persona_id": map[string]any{
					"type":        "string",
					"description": "Persona ID to analyze",
				},
			},
			"required": []string{"persona_id"},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		var args struct {
			PersonaID string `json:"persona_id"`
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

		return map[string]any{
			"persona_id":  args.PersonaID,
			"stats":       stats,
			"suggestions": suggestions,
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
	Personas      []string `yaml:"personas"`
	ActivePersona string   `yaml:"active_persona"`
}

// RegisterTeamHandlers registers the c4_whoami tool.
func RegisterTeamHandlers(reg *mcp.Registry, projectRoot string) {
	// c4_whoami
	reg.Register(mcp.ToolSchema{
		Name:        "c4_whoami",
		Description: "Get or set current user identity and active persona for the session",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"username": map[string]any{
					"type":        "string",
					"description": "Username to look up or register",
				},
				"role": map[string]any{
					"type":        "string",
					"description": "Role (e.g., 'frontend', 'backend', 'ceo')",
				},
				"active_persona": map[string]any{
					"type":        "string",
					"description": "Set active persona for this session",
				},
			},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		var args struct {
			Username      string `json:"username"`
			Role          string `json:"role"`
			ActivePersona string `json:"active_persona"`
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

		result := map[string]any{
			"username":       args.Username,
			"role":           member.Role,
			"personas":       member.Personas,
			"active_persona": member.ActivePersona,
		}
		if personaFile != "" {
			result["persona_file"] = personaFile
		}
		return result, nil
	})
}
