package main

import "github.com/changmin/c4-core/internal/mcp"

// readOnlyTools is the list of MCP tools to unregister when schema_diet is enabled.
// These tools are read-only and can be invoked via "cq tool <name> --json" instead,
// saving ~2,760 tokens (~34% of total MCP schema) per session.
//
// Measurement (2026-03-05): 92 tools total = 32,308 chars (~8,077 tokens)
// Read-only: 37 tools = 11,041 chars (~2,760 tokens, 34%)
var readOnlyTools = []string{
	"c4_lighthouse",
	"c4_task_list",
	"c4_whoami",
	"c4_find_symbol",
	"c4_search_for_pattern",
	"c4_knowledge_discover",
	"c4_read_file",
	"c4_knowledge_search",
	"c4_find_referencing_symbols",
	"c4_analyze_history",
	"c4_mail_ls",
	"c4_soul_get",
	"c4_config_get",
	"c4_find_file",
	"c4_search_commits",
	"c4_stale_tasks",
	"c4_persona_stats",
	"c4_knowledge_get",
	"c4_list_dir",
	"c4_get_symbols_overview",
	"c4_experiment_search",
	"c4_get_task",
	"c4_worktree_cleanup",
	"c4_mail_read",
	"c4_artifact_get",
	"c4_get_design",
	"c4_get_spec",
	"c4_pop_status",
	"c4_start",
	"c4_status",
	"c4_knowledge_stats",
	"c4_secret_list",
	"c4_pop_reflect",
	"c4_health",
	"c4_worktree_status",
	"c4_artifact_list",
	"c4_list_designs",
	"c4_list_specs",
}

// applySDchemaDiet unregisters read-only tools from the MCP registry.
// Agents should use "cq tool <name> --json" to call these tools instead.
func applySDchemaDiet(reg *mcp.Registry) {
	for _, name := range readOnlyTools {
		reg.Unregister(name)
	}
}
