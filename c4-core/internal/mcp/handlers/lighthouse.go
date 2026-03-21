package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/changmin/c4-core/internal/mcp"
)

// validLighthouseName restricts lighthouse names to safe characters for task ID parsing.
var validLighthouseName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]{0,63}$`)

// RegisterLighthouseHandlers registers the c4_lighthouse MCP tool.
// Concurrency: designed for single-session use via MCP stdio. The SQLite store
// (MaxOpenConns=1 + WAL) serializes writes, but lighthouse stub handlers capture
// value copies and are not updated concurrently.
func RegisterLighthouseHandlers(reg *mcp.Registry, store *SQLiteStore) {
	reg.Register(mcp.ToolSchema{
		Name:        "c4_lighthouse",
		Description: "Spec-as-MCP: register/promote/manage lighthouse stub tools",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "Action: register, register_all, list, get, promote, update, remove, export_llms_txt",
					"enum":        []string{"register", "register_all", "list", "get", "promote", "update", "remove", "export_llms_txt"},
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
				"auto_task": map[string]any{
					"type":        "boolean",
					"description": "Auto-create implementation task on register (default: true)",
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
			AutoTask    *bool  `json:"auto_task"`
		}
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}

		agentID := args.AgentID
		if agentID == "" {
			agentID = "direct"
		}

		autoTask := args.AutoTask == nil || *args.AutoTask // default true

		switch args.Action {
		case "register":
			return lighthouseRegister(reg, store, args.Name, args.Description, args.InputSchema, args.Spec, agentID, autoTask)
		case "register_all":
			return lighthouseRegisterAll(reg, store, agentID)
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
		case "export_llms_txt":
			return lighthouseExportLLMSTxt(store)
		default:
			return nil, fmt.Errorf("unknown action: %s", args.Action)
		}
	})
}

// lighthouseRegisterExisting saves a lighthouse record for an already-implemented core tool.
// No MCP stub is created — the real tool stays in the registry unchanged.
// No auto-task is created — the tool is already implemented.
func lighthouseRegisterExisting(store *SQLiteStore, name, description, inputSchema, spec, agentID string) (any, error) {
	if inputSchema == "" {
		inputSchema = `{"type":"object"}`
	}
	if inputSchema != `{"type":"object"}` {
		var tmp map[string]any
		if err := json.Unmarshal([]byte(inputSchema), &tmp); err != nil {
			return nil, fmt.Errorf("input_schema is not valid JSON: %w", err)
		}
	}

	lh := &Lighthouse{
		Name:        name,
		Description: description,
		InputSchema: inputSchema,
		Spec:        spec,
		Status:      "implemented",
		Version:     1,
		CreatedBy:   agentID,
		PromotedBy:  "pre-existing",
	}

	if err := store.saveLighthouse(lh); err != nil {
		return nil, fmt.Errorf("saving lighthouse: %w", err)
	}

	store.logTrace("lighthouse_register_existing", agentID, name, "registered as pre-existing implemented tool")

	return map[string]any{
		"success": true,
		"message": fmt.Sprintf("Lighthouse '%s' registered as pre-existing implemented tool (documentation-only)", name),
		"name":    name,
		"status":  "implemented",
		"version": 1,
	}, nil
}

// CLICommand describes a CLI subcommand for lighthouse registration.
type CLICommand struct {
	FullCommand string // e.g. "cq hub worker start"
	Short       string // one-line description
	Long        string // multi-line help text
}

// RegisterCLICommands registers CLI commands as lighthouse entries with "cli:" prefix.
// This allows agents to discover CLI commands via c4_lighthouse get/list.
func RegisterCLICommands(store *SQLiteStore, commands []CLICommand) int {
	registered := 0
	for _, cmd := range commands {
		name := "cli:" + cmd.FullCommand
		existing, _ := store.getLighthouse(name)
		if existing != nil {
			continue
		}
		spec := fmt.Sprintf("CLI Command: %s\n\nUsage:\n  %s\n\nDescription:\n  %s", cmd.Short, cmd.FullCommand, cmd.Long)
		lh := &Lighthouse{
			Name:        name,
			Description: cmd.Short,
			InputSchema: `{"type":"object"}`,
			Spec:        spec,
			Status:      "implemented",
			Version:     1,
			CreatedBy:   "auto-seed",
			PromotedBy:  "cli-auto",
		}
		if err := store.saveLighthouse(lh); err == nil {
			registered++
		}
	}
	return registered
}

// lighthouseRegisterAll bulk-registers all existing MCP tools as lighthouse entries.
// Skips tools that already have a lighthouse record or are the lighthouse tool itself.
func lighthouseRegisterAll(reg *mcp.Registry, store *SQLiteStore, agentID string) (any, error) {
	tools := reg.ListTools()
	registered, skipped, updated := 0, 0, 0
	var errors []string

	for _, tool := range tools {
		name := tool.Name
		// Skip the lighthouse tool itself
		if name == "c4_lighthouse" {
			skipped++
			continue
		}
		// Skip if already has a lighthouse entry (but sync description/spec from registry)
		if existing, _ := store.getLighthouse(name); existing != nil {
			changed := false
			// Sync description from registry (authoritative source)
			if tool.Description != "" && tool.Description != existing.Description {
				existing.Description = tool.Description
				changed = true
			}
			// Backfill empty spec
			if existing.Spec == "" {
				// Prefer registry schema, fall back to DB-stored schema
				schema := tool.InputSchema
				if schema == nil && existing.InputSchema != "" {
					var parsed map[string]any
					if json.Unmarshal([]byte(existing.InputSchema), &parsed) == nil {
						schema = parsed
					}
				}
				existing.Spec = generateSpecFromSchema(name, existing.Description, schema)
				changed = true
			}
			if changed {
				store.updateLighthouseDescAndSpec(name, existing.Description, existing.Spec)
				updated++
			}
			skipped++
			continue
		}

		// Serialize input_schema to JSON string
		schemaJSON := `{"type":"object"}`
		if tool.InputSchema != nil {
			if data, err := json.Marshal(tool.InputSchema); err == nil {
				schemaJSON = string(data)
			}
		}

		lh := &Lighthouse{
			Name:        name,
			Description: tool.Description,
			InputSchema: schemaJSON,
			Spec:        generateSpecFromSchema(name, tool.Description, tool.InputSchema),
			Status:      "implemented",
			Version:     1,
			CreatedBy:   agentID,
			PromotedBy:  "register_all",
		}

		if err := store.saveLighthouse(lh); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		registered++
	}

	store.logTrace("lighthouse_register_all", agentID, fmt.Sprintf("%d tools", registered), "bulk registration")

	result := map[string]any{
		"success":    true,
		"registered": registered,
		"skipped":    skipped,
		"updated":    updated,
		"total":      len(tools),
	}
	if len(errors) > 0 {
		result["errors"] = errors
	}
	return result, nil
}

// generateSpecFromSchema creates a structured markdown spec from a tool's description and input schema.
func generateSpecFromSchema(name, description string, schema map[string]any) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s\n\n%s\n", name, description))

	props := extractMap(schema, "properties")
	if len(props) == 0 {
		b.WriteString("\n## Parameters\nNone\n")
		return b.String()
	}

	required := make(map[string]bool)
	for _, r := range extractStringSlice(schema, "required") {
		required[r] = true
	}

	b.WriteString("\n## Parameters\n")
	for propName, propVal := range props {
		pm, ok := propVal.(map[string]any)
		if !ok {
			continue
		}
		pType := ""
		if t, ok := pm["type"].(string); ok {
			pType = t
		}
		pDesc := ""
		if d, ok := pm["description"].(string); ok {
			pDesc = d
		}
		req := ""
		if required[propName] {
			req = " **(required)**"
		}

		b.WriteString(fmt.Sprintf("- `%s` (%s)%s: %s", propName, pType, req, pDesc))

		// Add enum values if present
		if enumVal, ok := pm["enum"].([]any); ok && len(enumVal) > 0 {
			vals := make([]string, 0, len(enumVal))
			for _, v := range enumVal {
				if s, ok := v.(string); ok {
					vals = append(vals, s)
				}
			}
			if len(vals) > 0 {
				b.WriteString(fmt.Sprintf(" [%s]", strings.Join(vals, ", ")))
			}
		}
		// Add default value if present
		if def, ok := pm["default"]; ok {
			b.WriteString(fmt.Sprintf(" (default: %v)", def))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// lighthouseRegister creates a new lighthouse stub and registers it in the MCP registry.
func lighthouseRegister(reg *mcp.Registry, store *SQLiteStore, name, description, inputSchema, spec, agentID string, autoTask bool) (any, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required for register")
	}
	if !validLighthouseName.MatchString(name) {
		return nil, fmt.Errorf("invalid lighthouse name %q: must match [a-zA-Z_][a-zA-Z0-9_-]{0,63}", name)
	}
	if description == "" {
		return nil, fmt.Errorf("description is required for register")
	}

	// Check for existing lighthouse record
	existing, _ := store.getLighthouse(name)
	if existing != nil {
		return nil, fmt.Errorf("lighthouse '%s' already exists (status: %s)", name, existing.Status)
	}

	// If a core tool already exists, register as "implemented" (documentation-only mode)
	if reg.HasTool(name) {
		return lighthouseRegisterExisting(store, name, description, inputSchema, spec, agentID)
	}

	if inputSchema == "" {
		inputSchema = `{"type":"object"}`
	}

	// Validate JSON before persisting (F-03: reject malformed schema early)
	if inputSchema != `{"type":"object"}` {
		var tmp map[string]any
		if err := json.Unmarshal([]byte(inputSchema), &tmp); err != nil {
			return nil, fmt.Errorf("input_schema is not valid JSON: %w", err)
		}
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

	result := map[string]any{
		"success": true,
		"message": fmt.Sprintf("Lighthouse '%s' registered as stub (v%d)", name, lh.Version),
		"name":    name,
		"status":  "stub",
		"version": lh.Version,
	}

	// Auto-create implementation task
	if autoTask {
		taskID := fmt.Sprintf("T-LH-%s-0", name)

		// Build DoD from spec and input_schema
		dod := fmt.Sprintf("Implement '%s' matching lighthouse spec.", name)
		if spec != "" {
			specSummary := spec
			if len(specSummary) > 200 {
				specSummary = specSummary[:200] + "..."
			}
			dod += fmt.Sprintf(" Spec contract: %s", specSummary)
		}
		if inputSchema != "" && inputSchema != `{"type":"object"}` {
			dod += fmt.Sprintf(" Required input_schema: %s", inputSchema)
		}

		t := &Task{
			ID:     taskID,
			Title:  fmt.Sprintf("Implement lighthouse: %s", name),
			DoD:    dod,
			Domain: "lighthouse",
		}
		if err := store.AddTask(t); err != nil {
			// Non-blocking: log warning but don't fail the register
			fmt.Fprintf(os.Stderr, "c4: warning: failed to auto-create task %s: %v\n", taskID, err)
		} else {
			lh.TaskID = taskID
			if err := store.setLighthouseTaskID(name, taskID); err != nil {
				fmt.Fprintf(os.Stderr, "c4: warning: failed to link task %s to lighthouse %s: %v\n", taskID, name, err)
			}
			result["task_id"] = taskID
		}
	}

	return result, nil
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

// lighthouseExportLLMSTxt generates llms.txt + per-tool markdown from lighthouse data.
// Output: { llms_txt: string, tools: { name: markdown }[], tool_count: int }
func lighthouseExportLLMSTxt(store *SQLiteStore) (any, error) {
	lighthouses, err := store.listLighthouses()
	if err != nil {
		return nil, fmt.Errorf("listing lighthouses: %w", err)
	}

	// Group tools by category (prefix-based)
	categories := groupByCategory(lighthouses)

	// Generate llms.txt
	var llms strings.Builder
	llms.WriteString("# C4 Engine\n\n")
	llms.WriteString("> AI orchestration engine — plan, execute, review, and learn.\n\n")

	// Summary
	implemented, stubs := 0, 0
	for _, lh := range lighthouses {
		switch lh.Status {
		case "implemented":
			implemented++
		case "stub":
			stubs++
		}
	}
	llms.WriteString(fmt.Sprintf("> %d tools (%d implemented, %d stubs)\n\n", implemented+stubs, implemented, stubs))

	// Sections by category
	categoryOrder := []struct {
		key   string
		title string
	}{
		{"status", "Status & Config"},
		{"task", "Tasks"},
		{"review", "Review & Validation"},
		{"file", "Files & Search"},
		{"git", "Git"},
		{"discovery", "Discovery & Design"},
		{"artifact", "Artifacts"},
		{"lsp", "Code Intelligence (LSP)"},
		{"knowledge", "Knowledge (C9)"},
		{"research", "Research"},
		{"soul", "Soul & Persona"},
		{"llm", "LLM Gateway"},
		{"cdp", "CDP & WebMCP"},
		{"web", "Web Content"},
		{"c1", "Messenger (C1)"},
		{"c2", "Documents (C2)"},
		{"drive", "Drive (C0)"},
		{"event", "EventBus (C3)"},
		{"hub", "Hub (C5)"},
		{"gpu", "GPU & Jobs"},
		{"lighthouse", "Lighthouse"},
		{"other", "Other"},
	}

	for _, cat := range categoryOrder {
		tools, ok := categories[cat.key]
		if !ok || len(tools) == 0 {
			continue
		}
		llms.WriteString(fmt.Sprintf("## %s\n\n", cat.title))
		for _, lh := range tools {
			llms.WriteString(fmt.Sprintf("- [%s](c4://tools/%s.md): %s\n", lh.Name, lh.Name, lh.Description))
		}
		llms.WriteString("\n")
	}

	// Generate per-tool markdown
	toolDocs := make(map[string]string, len(lighthouses))
	for _, lh := range lighthouses {
		if lh.Status == "deprecated" {
			continue
		}
		toolDocs[lh.Name] = generateToolDoc(lh)
	}

	return map[string]any{
		"llms_txt":   llms.String(),
		"tools":      toolDocs,
		"tool_count": len(toolDocs),
	}, nil
}

// generateToolDoc creates a full markdown doc for a single tool.
func generateToolDoc(lh *Lighthouse) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s\n\n", lh.Name))
	b.WriteString(fmt.Sprintf("%s\n\n", lh.Description))
	b.WriteString(fmt.Sprintf("**Status:** %s | **Version:** %d\n\n", lh.Status, lh.Version))

	// Input schema as parameters table
	if lh.InputSchema != "" && lh.InputSchema != `{"type":"object"}` {
		var schema map[string]any
		if json.Unmarshal([]byte(lh.InputSchema), &schema) == nil {
			props := extractMap(schema, "properties")
			required := make(map[string]bool)
			for _, r := range extractStringSlice(schema, "required") {
				required[r] = true
			}
			if len(props) > 0 {
				b.WriteString("## Parameters\n\n")
				b.WriteString("| Name | Type | Required | Description |\n")
				b.WriteString("|------|------|----------|-------------|\n")
				for name, val := range props {
					pm, ok := val.(map[string]any)
					if !ok {
						continue
					}
					pType, _ := pm["type"].(string)
					pDesc, _ := pm["description"].(string)
					req := "no"
					if required[name] {
						req = "**yes**"
					}
					b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", name, pType, req, pDesc))
				}
				b.WriteString("\n")
			}
		}
	}

	// Spec (usage/workflow/examples)
	if lh.Spec != "" {
		b.WriteString("## Spec\n\n")
		b.WriteString(lh.Spec)
		if !strings.HasSuffix(lh.Spec, "\n") {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// groupByCategory groups lighthouses by tool name prefix.
func groupByCategory(lighthouses []*Lighthouse) map[string][]*Lighthouse {
	categories := make(map[string][]*Lighthouse)
	for _, lh := range lighthouses {
		if lh.Status == "deprecated" {
			continue
		}
		cat := categorize(lh.Name)
		categories[cat] = append(categories[cat], lh)
	}
	return categories
}

// categorize returns a category key based on tool name prefix.
func categorize(name string) string {
	prefixes := []struct {
		prefix string
		cat    string
	}{
		{"c4_status", "status"}, {"c4_start", "status"}, {"c4_clear", "status"},
		{"c4_config", "status"}, {"c4_health", "status"},
		{"c4_add_todo", "task"}, {"c4_get_task", "task"}, {"c4_submit", "task"},
		{"c4_mark_blocked", "task"}, {"c4_claim", "task"}, {"c4_report", "task"},
		{"c4_task_list", "task"},
		{"c4_checkpoint", "review"}, {"c4_request_changes", "review"},
		{"c4_ensure_supervisor", "review"}, {"c4_run_validation", "review"},
		{"c4_find_file", "file"}, {"c4_search_for_pattern", "file"},
		{"c4_read_file", "file"}, {"c4_replace_content", "file"},
		{"c4_create_text_file", "file"}, {"c4_list_dir", "file"},
		{"c4_worktree", "git"}, {"c4_analyze_history", "git"}, {"c4_search_commits", "git"},
		{"c4_save_spec", "discovery"}, {"c4_get_spec", "discovery"}, {"c4_list_specs", "discovery"},
		{"c4_save_design", "discovery"}, {"c4_get_design", "discovery"}, {"c4_list_designs", "discovery"},
		{"c4_discovery_complete", "discovery"}, {"c4_design_complete", "discovery"},
		{"c4_artifact", "artifact"},
		{"c4_find_symbol", "lsp"}, {"c4_get_symbols", "lsp"}, {"c4_replace_symbol", "lsp"},
		{"c4_insert_before", "lsp"}, {"c4_insert_after", "lsp"}, {"c4_rename_symbol", "lsp"},
		{"c4_find_referencing", "lsp"},
		{"c4_knowledge", "knowledge"}, {"c4_experiment", "knowledge"}, {"c4_pattern_suggest", "knowledge"},
		{"c4_research", "research"},
		{"c4_soul", "soul"}, {"c4_whoami", "soul"}, {"c4_persona", "soul"}, {"c4_reflect", "soul"},
		{"c4_llm", "llm"},
		{"c4_cdp", "cdp"}, {"c4_webmcp", "cdp"},
		{"c4_web_fetch", "web"},
		{"c1_", "c1"},
		{"c4_parse_document", "c2"}, {"c4_extract_text", "c2"},
		{"c4_workspace", "c2"}, {"c4_profile", "c2"}, {"c4_persona_learn", "c2"},
		{"c4_drive", "drive"},
		{"c4_event", "event"}, {"c4_rule", "event"},
		{"c4_hub", "hub"},
		{"c4_gpu", "gpu"}, {"c4_job_submit", "gpu"},
		{"c4_lighthouse", "lighthouse"},
		{"c4_onboard", "other"},
	}

	for _, p := range prefixes {
		if strings.HasPrefix(name, p.prefix) {
			return p.cat
		}
	}
	return "other"
}

// lighthouseGet returns details for a specific lighthouse.
func lighthouseGet(store *SQLiteStore, name string) (any, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required for get")
	}
	lh, err := store.getLighthouse(name)
	if err != nil {
		return nil, fmt.Errorf("lighthouse '%s': %w", name, err)
	}
	return lh, nil
}

// lighthousePromote changes a lighthouse's status to "implemented".
func lighthousePromote(reg *mcp.Registry, store *SQLiteStore, name, agentID string) (any, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required for promote")
	}

	// Get lighthouse before promotion for schema validation
	lh, err := store.getLighthouse(name)
	if err != nil {
		return nil, fmt.Errorf("lighthouse '%s' not found", name)
	}

	// F-01: Schema validation BEFORE unregister — check if a real (non-stub) tool is registered
	var schemaWarnings []string
	if realSchema, ok := reg.GetToolSchema(name); ok {
		if !strings.HasPrefix(realSchema.Description, "[LIGHTHOUSE]") {
			schemaWarnings = validateSchemaCompat(lh.InputSchema, realSchema.InputSchema)
		}
	}

	if err := store.promoteLighthouse(name, agentID); err != nil {
		return nil, err
	}

	// Remove the stub from registry — but only if no real implementation is already registered.
	// If a real tool exists (description without [LIGHTHOUSE] prefix), keep it.
	if schema, ok := reg.GetToolSchema(name); ok {
		if strings.HasPrefix(schema.Description, "[LIGHTHOUSE]") {
			reg.Unregister(name)
		}
		// Real tool already registered — leave it in place
	}

	store.logTrace("lighthouse_promote", agentID, name, "promoted to implemented")

	result := map[string]any{
		"success": true,
		"message": fmt.Sprintf("Lighthouse '%s' promoted to implemented. Stub removed from registry.", name),
		"name":    name,
		"status":  "implemented",
	}

	if len(schemaWarnings) > 0 {
		result["schema_warnings"] = schemaWarnings
	}

	// F-13: Complete linked task via store method (preserves logTrace)
	if lh.TaskID != "" {
		if err := store.completeLighthouseTask(lh.TaskID, name); err != nil {
			fmt.Fprintf(os.Stderr, "c4: warning: failed to complete lighthouse task %s: %v\n", lh.TaskID, err)
		} else {
			result["task_completed"] = lh.TaskID
		}
	}

	return result, nil
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

	// Re-read updated lighthouse from DB
	lh, err := store.getLighthouse(name)
	if err != nil {
		return nil, fmt.Errorf("lighthouse '%s' updated but failed to re-read: %w", name, err)
	}

	// Refresh the stub in registry if it's still a stub
	if lh.Status == "stub" {
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
// F-04: Takes value copy to avoid capturing mutable pointer.
func makeLighthouseStub(lh *Lighthouse) mcp.HandlerFunc {
	snapshot := *lh // value copy — closure is immutable
	return func(args json.RawMessage) (any, error) {
		var inputSchema map[string]any
		if err := json.Unmarshal([]byte(snapshot.InputSchema), &inputSchema); err != nil {
			// Continue with empty schema if parsing fails
			inputSchema = map[string]any{"type": "object"}
		}
		return map[string]any{
			"lighthouse":   true,
			"status":       "stub",
			"name":         snapshot.Name,
			"description":  snapshot.Description,
			"spec":         snapshot.Spec,
			"input_schema": inputSchema,
			"version":      snapshot.Version,
			"message":      "This is a lighthouse stub. The spec above defines the contract.",
			"called_with":  json.RawMessage(args),
		}, nil
	}
}

// LoadLighthousesOnStartup loads all stub lighthouses from DB into the MCP registry.
// Must be called after all core handlers are registered.
// If a real tool is already registered with the same name as a stub, the lighthouse
// is auto-promoted to "implemented" status — eliminating manual promote steps.
func LoadLighthousesOnStartup(reg *mcp.Registry, store *SQLiteStore) int {
	lighthouses, err := store.listLighthouses()
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4: warning: failed to load lighthouses on startup: %v\n", err)
		return 0
	}

	// Auto-seed: if no lighthouses exist, register all current tools as documentation
	if len(lighthouses) == 0 {
		if result, err := lighthouseRegisterAll(reg, store, "auto-seed"); err == nil {
			if n, ok := result.(map[string]any)["registered"].(int); ok && n > 0 {
				fmt.Fprintf(os.Stderr, "c4: lighthouse auto-seed: %d tools registered\n", n)
			}
		}
		// Re-read after seeding
		lighthouses, _ = store.listLighthouses()
	} else {
		// Backfill empty specs directly from DB entries (covers Hub/Drive/C1 tools not in registry)
		backfilled := 0
		for _, lh := range lighthouses {
			if lh.Spec != "" {
				continue
			}
			var schema map[string]any
			if lh.InputSchema != "" {
				json.Unmarshal([]byte(lh.InputSchema), &schema)
			}
			spec := generateSpecFromSchema(lh.Name, lh.Description, schema)
			if spec != "" {
				store.updateLighthouseSpec(lh.Name, spec)
				backfilled++
			}
		}
		if backfilled > 0 {
			fmt.Fprintf(os.Stderr, "c4: lighthouse spec backfill: %d tools updated\n", backfilled)
		}
	}

	count := 0
	for _, lh := range lighthouses {
		if lh.Status != "stub" {
			continue
		}
		// If real tool already registered, auto-promote the lighthouse
		if reg.HasTool(lh.Name) {
			if err := store.promoteLighthouse(lh.Name, "auto-startup"); err == nil {
				fmt.Fprintf(os.Stderr, "c4: lighthouse auto-promoted: %s (real tool detected)\n", lh.Name)
			}
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
		INSERT INTO c4_lighthouses (name, description, input_schema, spec, status, version, created_by, promoted_by, task_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		lh.Name, lh.Description, lh.InputSchema, lh.Spec, lh.Status, lh.Version, lh.CreatedBy, lh.PromotedBy, lh.TaskID, now, now,
	)
	return err
}

func (s *SQLiteStore) getLighthouse(name string) (*Lighthouse, error) {
	var lh Lighthouse
	err := s.db.QueryRow(`
		SELECT name, description, input_schema, spec, status, version, created_by, promoted_by, task_id, created_at, updated_at
		FROM c4_lighthouses WHERE name = ?`, name,
	).Scan(&lh.Name, &lh.Description, &lh.InputSchema, &lh.Spec, &lh.Status, &lh.Version,
		&lh.CreatedBy, &lh.PromotedBy, &lh.TaskID, &lh.CreatedAt, &lh.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &lh, nil
}

func (s *SQLiteStore) listLighthouses() ([]*Lighthouse, error) {
	rows, err := s.db.Query(`
		SELECT name, description, input_schema, spec, status, version, created_by, promoted_by, task_id, created_at, updated_at
		FROM c4_lighthouses ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lighthouses []*Lighthouse
	for rows.Next() {
		var lh Lighthouse
		if err := rows.Scan(&lh.Name, &lh.Description, &lh.InputSchema, &lh.Spec, &lh.Status, &lh.Version,
			&lh.CreatedBy, &lh.PromotedBy, &lh.TaskID, &lh.CreatedAt, &lh.UpdatedAt); err != nil {
			fmt.Fprintf(os.Stderr, "c4: warning: skipping malformed lighthouse row: %v\n", err)
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

func (s *SQLiteStore) updateLighthouseSpec(name, spec string) error {
	_, err := s.db.Exec(`UPDATE c4_lighthouses SET spec=?, updated_at=CURRENT_TIMESTAMP WHERE name=?`, spec, name)
	return err
}

func (s *SQLiteStore) updateLighthouseDescAndSpec(name, description, spec string) error {
	_, err := s.db.Exec(`UPDATE c4_lighthouses SET description=?, spec=?, updated_at=CURRENT_TIMESTAMP WHERE name=?`, description, spec, name)
	return err
}

func (s *SQLiteStore) updateLighthouse(name string, updates map[string]any) error {
	lh, err := s.getLighthouse(name)
	if err != nil {
		return fmt.Errorf("lighthouse '%s' not found", name)
	}
	// input_schema changes only allowed for stubs (contract change)
	if v, ok := updates["input_schema"].(string); ok && v != "" {
		if lh.Status != "stub" {
			return fmt.Errorf("cannot change input_schema of %s lighthouse '%s' — only stubs allow schema changes", lh.Status, name)
		}
		lh.InputSchema = v
	}

	// description and spec can be updated for any status (documentation evolves)
	if v, ok := updates["description"].(string); ok && v != "" {
		lh.Description = v
	}
	if v, ok := updates["spec"].(string); ok && v != "" {
		lh.Spec = v
	}

	_, err = s.db.Exec(`
		UPDATE c4_lighthouses SET description=?, input_schema=?, spec=?, version=version+1, updated_at=CURRENT_TIMESTAMP
		WHERE name=?`, lh.Description, lh.InputSchema, lh.Spec, name)
	return err
}

func (s *SQLiteStore) setLighthouseTaskID(name, taskID string) error {
	_, err := s.db.Exec(`UPDATE c4_lighthouses SET task_id=? WHERE name=?`, taskID, name)
	return err
}

func (s *SQLiteStore) completeLighthouseTask(taskID, lighthouseName string) error {
	_, err := s.db.Exec(`UPDATE c4_tasks SET status='done', updated_at=CURRENT_TIMESTAMP WHERE task_id=?`, taskID)
	if err != nil {
		return err
	}
	s.logTrace("lighthouse_task_complete", "", taskID, fmt.Sprintf("completed via promote of %s", lighthouseName))
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

// validateSchemaCompat compares a lighthouse's input_schema (JSON string) against
// a real tool's inputSchema (map). Returns a list of warnings for missing required fields.
// Uses a lenient superset check: the real tool may have extra properties.
func validateSchemaCompat(lhSchemaJSON string, realSchema map[string]any) []string {
	var lhSchema map[string]any
	if err := json.Unmarshal([]byte(lhSchemaJSON), &lhSchema); err != nil {
		return []string{"lighthouse input_schema is not valid JSON"}
	}

	var warnings []string

	// Compare required fields: lighthouse required should be a subset of real required
	lhRequired := extractStringSlice(lhSchema, "required")
	realRequired := extractStringSlice(realSchema, "required")
	realRequiredSet := make(map[string]bool, len(realRequired))
	for _, r := range realRequired {
		realRequiredSet[r] = true
	}
	for _, r := range lhRequired {
		if !realRequiredSet[r] {
			warnings = append(warnings, fmt.Sprintf("lighthouse requires '%s' but real tool does not", r))
		}
	}

	// Reverse required check: real tool requires fields that lighthouse doesn't
	lhRequiredSet := make(map[string]bool, len(lhRequired))
	for _, r := range lhRequired {
		lhRequiredSet[r] = true
	}
	for _, r := range realRequired {
		if !lhRequiredSet[r] {
			warnings = append(warnings, fmt.Sprintf("real tool requires '%s' but lighthouse does not", r))
		}
	}

	// Compare properties: lighthouse properties should exist in real tool
	lhProps := extractMap(lhSchema, "properties")
	realProps := extractMap(realSchema, "properties")
	for propName, lhPropVal := range lhProps {
		realPropVal, exists := realProps[propName]
		if !exists {
			warnings = append(warnings, fmt.Sprintf("lighthouse property '%s' not found in real tool", propName))
			continue
		}
		// Type comparison
		lhType := extractType(lhPropVal)
		realType := extractType(realPropVal)
		if lhType != "" && realType != "" && lhType != realType {
			warnings = append(warnings, fmt.Sprintf("property '%s' type mismatch: lighthouse=%s, real=%s", propName, lhType, realType))
		}
	}

	return warnings
}

func extractStringSlice(m map[string]any, key string) []string {
	v, ok := m[key]
	if !ok {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		// Try []string directly
		if ss, ok := v.([]string); ok {
			return ss
		}
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func extractType(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	t, ok := m["type"].(string)
	if !ok {
		return ""
	}
	return t
}

func extractMap(m map[string]any, key string) map[string]any {
	v, ok := m[key]
	if !ok {
		return nil
	}
	mm, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return mm
}
