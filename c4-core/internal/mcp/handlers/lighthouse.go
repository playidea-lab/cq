package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
)

// RegisterLighthouseHandlers registers the c4_lighthouse MCP tool.
func RegisterLighthouseHandlers(reg *mcp.Registry, store *SQLiteStore) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_lighthouse",
		Description: "Spec-as-MCP: register/promote/manage lighthouse stub tools",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "Action: register, list, get, promote, update, remove",
					"enum":        []string{"register", "list", "get", "promote", "update", "remove"},
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Lighthouse tool name",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Tool description",
				},
				"input_schema": map[string]any{
					"type":        "string",
					"description": "JSON string of the tool's input schema",
				},
				"spec": map[string]any{
					"type":        "string",
					"description": "API spec/contract in markdown or text",
				},
				"agent_id": map[string]any{
					"type":        "string",
					"description": "Agent ID for tracing",
				},
			},
			"required": []string{"action"},
		},
	}, func(rawArgs json.RawMessage) (any, error) {
		var args struct {
			Action      string `json:"action"`
			Name        string `json:"name"`
			Description string `json:"description"`
			InputSchema string `json:"input_schema"`
			Spec        string `json:"spec"`
			AgentID     string `json:"agent_id"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}

		agentID := args.AgentID
		if agentID == "" {
			agentID = "direct"
		}

		switch args.Action {
		case "register":
			return lighthouseRegister(reg, store, args.Name, args.Description, args.InputSchema, args.Spec, agentID)
		case "list":
			return lighthouseList(store)
		case "get":
			return lighthouseGet(store, args.Name)
		case "promote":
			return lighthousePromote(reg, store, args.Name, agentID)
		case "update":
			return lighthouseUpdate(reg, store, args.Name, args.Description, args.InputSchema, args.Spec)
		case "remove":
			return lighthouseRemove(reg, store, args.Name, agentID)
		default:
			return nil, fmt.Errorf("unknown action: %s", args.Action)
		}
	})
}

// lighthouseRegister creates a new lighthouse stub and registers it in the MCP registry.
func lighthouseRegister(reg *mcp.Registry, store *SQLiteStore, name, description, inputSchema, spec, agentID string) (any, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required for register")
	}
	if description == "" {
		return nil, fmt.Errorf("description is required for register")
	}

	// Check for name collision with existing non-lighthouse tools
	existing, _ := store.getLighthouse(name)
	if existing != nil {
		return nil, fmt.Errorf("lighthouse '%s' already exists (status: %s)", name, existing.Status)
	}
	if reg.HasTool(name) {
		return nil, fmt.Errorf("tool '%s' already registered as a core tool — choose a different name", name)
	}

	if inputSchema == "" {
		inputSchema = `{"type":"object"}`
	}

	lh := &Lighthouse{
		Name:        name,
		Description: description,
		InputSchema: inputSchema,
		Spec:        spec,
		Status:      "stub",
		Version:     1,
		CreatedBy:   agentID,
	}

	if err := store.saveLighthouse(lh); err != nil {
		return nil, fmt.Errorf("saving lighthouse: %w", err)
	}

	// Parse input schema for MCP registration
	var schemaMap map[string]any
	if err := json.Unmarshal([]byte(inputSchema), &schemaMap); err != nil {
		schemaMap = map[string]any{"type": "object"}
	}

	reg.Register(mcp.ToolSchema{
		Name:        name,
		Description: fmt.Sprintf("[LIGHTHOUSE] %s", description),
		InputSchema: schemaMap,
	}, makeLighthouseStub(lh))

	store.logTrace("lighthouse_register", agentID, name, description)

	return map[string]any{
		"success": true,
		"message": fmt.Sprintf("Lighthouse '%s' registered as stub (v%d)", name, lh.Version),
		"name":    name,
		"status":  "stub",
		"version": lh.Version,
	}, nil
}

// lighthouseList returns all lighthouses with status counts.
func lighthouseList(store *SQLiteStore) (any, error) {
	lighthouses, err := store.listLighthouses()
	if err != nil {
		return nil, fmt.Errorf("listing lighthouses: %w", err)
	}

	stubs, implemented, deprecated := 0, 0, 0
	for _, lh := range lighthouses {
		switch lh.Status {
		case "stub":
			stubs++
		case "implemented":
			implemented++
		case "deprecated":
			deprecated++
		}
	}

	return map[string]any{
		"lighthouses": lighthouses,
		"summary": map[string]any{
			"total":       len(lighthouses),
			"stubs":       stubs,
			"implemented": implemented,
			"deprecated":  deprecated,
		},
	}, nil
}

// lighthouseGet returns details for a specific lighthouse.
func lighthouseGet(store *SQLiteStore, name string) (any, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required for get")
	}
	lh, err := store.getLighthouse(name)
	if err != nil {
		return nil, fmt.Errorf("lighthouse '%s' not found", name)
	}
	return lh, nil
}

// lighthousePromote changes a lighthouse's status to "implemented".
func lighthousePromote(reg *mcp.Registry, store *SQLiteStore, name, agentID string) (any, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required for promote")
	}

	if err := store.promoteLighthouse(name, agentID); err != nil {
		return nil, err
	}

	// Remove the stub from registry — the real implementation should be registered separately
	reg.Unregister(name)

	store.logTrace("lighthouse_promote", agentID, name, "promoted to implemented")

	return map[string]any{
		"success": true,
		"message": fmt.Sprintf("Lighthouse '%s' promoted to implemented. Stub removed from registry.", name),
		"name":    name,
		"status":  "implemented",
	}, nil
}

// lighthouseUpdate updates a lighthouse's spec, description, or input_schema.
func lighthouseUpdate(reg *mcp.Registry, store *SQLiteStore, name, description, inputSchema, spec string) (any, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required for update")
	}

	updates := map[string]any{}
	if description != "" {
		updates["description"] = description
	}
	if inputSchema != "" {
		updates["input_schema"] = inputSchema
	}
	if spec != "" {
		updates["spec"] = spec
	}

	if len(updates) == 0 {
		return nil, fmt.Errorf("at least one of description, input_schema, or spec must be provided")
	}

	if err := store.updateLighthouse(name, updates); err != nil {
		return nil, err
	}

	// Refresh the stub in registry if it's still a stub
	lh, err := store.getLighthouse(name)
	if err == nil && lh.Status == "stub" {
		var schemaMap map[string]any
		if err := json.Unmarshal([]byte(lh.InputSchema), &schemaMap); err != nil {
			schemaMap = map[string]any{"type": "object"}
		}
		reg.Replace(mcp.ToolSchema{
			Name:        name,
			Description: fmt.Sprintf("[LIGHTHOUSE] %s", lh.Description),
			InputSchema: schemaMap,
		}, makeLighthouseStub(lh))
	}

	store.logTrace("lighthouse_update", "", name, fmt.Sprintf("updated fields: %v", updates))

	return map[string]any{
		"success": true,
		"message": fmt.Sprintf("Lighthouse '%s' updated (v%d)", name, lh.Version),
		"name":    name,
		"version": lh.Version,
	}, nil
}

// lighthouseRemove deprecates a lighthouse and removes it from the registry.
func lighthouseRemove(reg *mcp.Registry, store *SQLiteStore, name, agentID string) (any, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required for remove")
	}

	if err := store.deprecateLighthouse(name); err != nil {
		return nil, err
	}

	reg.Unregister(name)

	store.logTrace("lighthouse_remove", agentID, name, "deprecated and removed from registry")

	return map[string]any{
		"success": true,
		"message": fmt.Sprintf("Lighthouse '%s' deprecated and removed from registry", name),
		"name":    name,
		"status":  "deprecated",
	}, nil
}

// makeLighthouseStub creates a handler that returns the lighthouse spec when called.
func makeLighthouseStub(lh *Lighthouse) mcp.HandlerFunc {
	return func(args json.RawMessage) (any, error) {
		var inputSchema map[string]any
		_ = json.Unmarshal([]byte(lh.InputSchema), &inputSchema)
		return map[string]any{
			"lighthouse":   true,
			"status":       "stub",
			"name":         lh.Name,
			"description":  lh.Description,
			"spec":         lh.Spec,
			"input_schema": inputSchema,
			"version":      lh.Version,
			"message":      "This is a lighthouse stub. The spec above defines the contract.",
			"called_with":  json.RawMessage(args),
		}, nil
	}
}

// LoadLighthousesOnStartup loads all stub lighthouses from DB into the MCP registry.
// Must be called after all core handlers are registered.
func LoadLighthousesOnStartup(reg *mcp.Registry, store *SQLiteStore) int {
	lighthouses, err := store.listLighthouses()
	if err != nil {
		return 0
	}

	count := 0
	for _, lh := range lighthouses {
		if lh.Status != "stub" {
			continue
		}
		// Skip if a core tool already has this name
		if reg.HasTool(lh.Name) {
			continue
		}

		var schemaMap map[string]any
		if err := json.Unmarshal([]byte(lh.InputSchema), &schemaMap); err != nil {
			schemaMap = map[string]any{"type": "object"}
		}

		reg.Register(mcp.ToolSchema{
			Name:        lh.Name,
			Description: fmt.Sprintf("[LIGHTHOUSE] %s", lh.Description),
			InputSchema: schemaMap,
		}, makeLighthouseStub(lh))
		count++
	}

	return count
}

// --- SQLiteStore methods for lighthouse persistence ---

func (s *SQLiteStore) saveLighthouse(lh *Lighthouse) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO c4_lighthouses (name, description, input_schema, spec, status, version, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		lh.Name, lh.Description, lh.InputSchema, lh.Spec, lh.Status, lh.Version, lh.CreatedBy, now, now,
	)
	return err
}

func (s *SQLiteStore) getLighthouse(name string) (*Lighthouse, error) {
	var lh Lighthouse
	err := s.db.QueryRow(`
		SELECT name, description, input_schema, spec, status, version, created_by, promoted_by, created_at, updated_at
		FROM c4_lighthouses WHERE name = ?`, name,
	).Scan(&lh.Name, &lh.Description, &lh.InputSchema, &lh.Spec, &lh.Status, &lh.Version,
		&lh.CreatedBy, &lh.PromotedBy, &lh.CreatedAt, &lh.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &lh, nil
}

func (s *SQLiteStore) listLighthouses() ([]*Lighthouse, error) {
	rows, err := s.db.Query(`
		SELECT name, description, input_schema, spec, status, version, created_by, promoted_by, created_at, updated_at
		FROM c4_lighthouses ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lighthouses []*Lighthouse
	for rows.Next() {
		var lh Lighthouse
		if err := rows.Scan(&lh.Name, &lh.Description, &lh.InputSchema, &lh.Spec, &lh.Status, &lh.Version,
			&lh.CreatedBy, &lh.PromotedBy, &lh.CreatedAt, &lh.UpdatedAt); err != nil {
			continue
		}
		lighthouses = append(lighthouses, &lh)
	}
	return lighthouses, nil
}

func (s *SQLiteStore) promoteLighthouse(name, promotedBy string) error {
	lh, err := s.getLighthouse(name)
	if err != nil {
		return fmt.Errorf("lighthouse '%s' not found", name)
	}
	if lh.Status != "stub" {
		return fmt.Errorf("lighthouse '%s' is %s, not stub", name, lh.Status)
	}
	_, err = s.db.Exec(`
		UPDATE c4_lighthouses SET status='implemented', promoted_by=?, updated_at=CURRENT_TIMESTAMP
		WHERE name=?`, promotedBy, name)
	return err
}

func (s *SQLiteStore) updateLighthouse(name string, updates map[string]any) error {
	lh, err := s.getLighthouse(name)
	if err != nil {
		return fmt.Errorf("lighthouse '%s' not found", name)
	}
	if lh.Status == "deprecated" {
		return fmt.Errorf("cannot update deprecated lighthouse '%s'", name)
	}

	if v, ok := updates["description"].(string); ok && v != "" {
		lh.Description = v
	}
	if v, ok := updates["input_schema"].(string); ok && v != "" {
		lh.InputSchema = v
	}
	if v, ok := updates["spec"].(string); ok && v != "" {
		lh.Spec = v
	}

	_, err = s.db.Exec(`
		UPDATE c4_lighthouses SET description=?, input_schema=?, spec=?, version=version+1, updated_at=CURRENT_TIMESTAMP
		WHERE name=?`, lh.Description, lh.InputSchema, lh.Spec, name)
	if err != nil {
		return err
	}

	// Re-read to get updated version
	lh.Version++
	return nil
}

func (s *SQLiteStore) deprecateLighthouse(name string) error {
	lh, err := s.getLighthouse(name)
	if err != nil {
		return fmt.Errorf("lighthouse '%s' not found", name)
	}
	if lh.Status == "deprecated" {
		return fmt.Errorf("lighthouse '%s' is already deprecated", name)
	}
	_, err = s.db.Exec(`
		UPDATE c4_lighthouses SET status='deprecated', updated_at=CURRENT_TIMESTAMP
		WHERE name=?`, name)
	return err
}
