"""MCP Tool Schemas - Tool definitions for C4 MCP server."""

from mcp.types import Tool


def get_tool_definitions() -> list[Tool]:
    """Return list of all C4 MCP tool definitions."""
    return [
        Tool(
            name="c4_status",
            description="Get current C4 project status including state, queue, and workers",
            inputSchema={
                "type": "object",
                "properties": {},
                "required": [],
            },
        ),
        Tool(
            name="c4_clear",
            description="Clear C4 state completely. Deletes .c4 directory and clears daemon cache. Use for development/debugging.",
            inputSchema={
                "type": "object",
                "properties": {
                    "confirm": {
                        "type": "boolean",
                        "description": "Must be true to confirm deletion",
                    },
                    "keep_config": {
                        "type": "boolean",
                        "description": "Keep config.yaml (default: false)",
                        "default": False,
                    },
                },
                "required": ["confirm"],
            },
        ),
        Tool(
            name="c4_get_task",
            description="Request next task assignment for a worker.",
            inputSchema={
                "type": "object",
                "properties": {
                    "worker_id": {
                        "type": "string",
                        "description": "Unique identifier for the worker",
                    },
                },
                "required": ["worker_id"],
            },
        ),
        Tool(
            name="c4_submit",
            description="Report task completion with validation results",
            inputSchema={
                "type": "object",
                "properties": {
                    "task_id": {
                        "type": "string",
                        "description": "ID of the completed task",
                    },
                    "commit_sha": {
                        "type": "string",
                        "description": "Git commit SHA of the work",
                    },
                    "validation_results": {
                        "type": "array",
                        "items": {
                            "type": "object",
                            "properties": {
                                "name": {"type": "string"},
                                "status": {"type": "string", "enum": ["pass", "fail"]},
                                "message": {"type": "string"},
                            },
                            "required": ["name", "status"],
                        },
                        "description": "Results of validation runs (lint, test, etc.)",
                    },
                    "worker_id": {
                        "type": "string",
                        "description": "Worker ID submitting the task (for ownership verification)",
                    },
                },
                "required": ["task_id", "commit_sha", "validation_results"],
            },
        ),
        Tool(
            name="c4_add_todo",
            description="Add a new task to the queue with optional dependencies",
            inputSchema={
                "type": "object",
                "properties": {
                    "task_id": {"type": "string", "description": "Unique task ID (e.g., T-001)"},
                    "title": {"type": "string", "description": "Task title"},
                    "scope": {"type": "string", "description": "File/directory scope for lock"},
                    "dod": {"type": "string", "description": "Definition of Done"},
                    "dependencies": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "Task IDs that must complete first (e.g., ['T-001', 'T-002'])",
                    },
                    "domain": {
                        "type": "string",
                        "description": "Domain for agent routing (web-frontend, web-backend, etc.)",
                    },
                    "priority": {
                        "type": "integer",
                        "description": "Higher priority tasks assigned first (default: 0)",
                    },
                },
                "required": ["task_id", "title", "dod"],
            },
        ),
        Tool(
            name="c4_checkpoint",
            description="Record supervisor checkpoint decision",
            inputSchema={
                "type": "object",
                "properties": {
                    "checkpoint_id": {"type": "string"},
                    "decision": {
                        "type": "string",
                        "enum": ["APPROVE", "REQUEST_CHANGES", "REPLAN"],
                    },
                    "notes": {"type": "string"},
                    "required_changes": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "List of required changes (for REQUEST_CHANGES)",
                    },
                },
                "required": ["checkpoint_id", "decision", "notes"],
            },
        ),
        Tool(
            name="c4_start",
            description="Start execution by transitioning from PLAN/HALTED to EXECUTE state",
            inputSchema={
                "type": "object",
                "properties": {},
                "required": [],
            },
        ),
        Tool(
            name="c4_ensure_supervisor",
            description="Ensure supervisor loop is running for AI review. Auto-starts if in EXECUTE/CHECKPOINT state.",
            inputSchema={
                "type": "object",
                "properties": {
                    "force_restart": {
                        "type": "boolean",
                        "description": "Force restart even if already running (default: false)",
                        "default": False,
                    },
                },
                "required": [],
            },
        ),
        Tool(
            name="c4_run_validation",
            description="Run validation commands (lint, test) and return results.",
            inputSchema={
                "type": "object",
                "properties": {
                    "names": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "Validations to run (e.g., ['lint', 'unit'])",
                    },
                    "fail_fast": {
                        "type": "boolean",
                        "description": "Stop on first failure (default: true)",
                        "default": True,
                    },
                    "timeout": {
                        "type": "integer",
                        "description": "Timeout per validation in seconds (default: 300)",
                        "default": 300,
                    },
                },
                "required": [],
            },
        ),
        Tool(
            name="c4_mark_blocked",
            description="Mark a task as blocked after max retry attempts. Adds to repair queue for supervisor guidance.",
            inputSchema={
                "type": "object",
                "properties": {
                    "task_id": {
                        "type": "string",
                        "description": "ID of the blocked task",
                    },
                    "worker_id": {
                        "type": "string",
                        "description": "ID of the worker that was working on the task",
                    },
                    "failure_signature": {
                        "type": "string",
                        "description": "Error signature from validation failures",
                    },
                    "attempts": {
                        "type": "integer",
                        "description": "Number of fix attempts made",
                    },
                    "last_error": {
                        "type": "string",
                        "description": "Last error message",
                    },
                },
                "required": ["task_id", "worker_id", "failure_signature", "attempts"],
            },
        ),
        # Discovery & Specification Tools
        Tool(
            name="c4_save_spec",
            description="Save feature specification to .c4/specs/. Used during discovery phase to persist EARS requirements.",
            inputSchema={
                "type": "object",
                "properties": {
                    "feature": {
                        "type": "string",
                        "description": "Feature name (e.g., 'user-auth', 'dashboard')",
                    },
                    "requirements": {
                        "type": "array",
                        "items": {
                            "type": "object",
                            "properties": {
                                "id": {"type": "string", "description": "Requirement ID (e.g., 'REQ-001')"},
                                "pattern": {
                                    "type": "string",
                                    "enum": ["ubiquitous", "state-driven", "event-driven", "optional", "unwanted"],
                                    "description": "EARS pattern type",
                                },
                                "text": {"type": "string", "description": "Full EARS requirement text"},
                            },
                            "required": ["id", "text"],
                        },
                        "description": "List of EARS requirements",
                    },
                    "domain": {
                        "type": "string",
                        "enum": ["web-frontend", "web-backend", "fullstack", "ml-dl", "mobile-app", "infra", "library", "unknown"],
                        "description": "Project domain",
                    },
                    "description": {
                        "type": "string",
                        "description": "Optional feature description",
                    },
                },
                "required": ["feature", "requirements", "domain"],
            },
        ),
        Tool(
            name="c4_list_specs",
            description="List all feature specifications in .c4/specs/",
            inputSchema={
                "type": "object",
                "properties": {},
                "required": [],
            },
        ),
        Tool(
            name="c4_get_spec",
            description="Get a specific feature specification by name",
            inputSchema={
                "type": "object",
                "properties": {
                    "feature": {
                        "type": "string",
                        "description": "Feature name to retrieve",
                    },
                },
                "required": ["feature"],
            },
        ),
        Tool(
            name="c4_discovery_complete",
            description="Mark discovery phase as complete and transition to DESIGN state. "
            "Requires at least one specification to be saved.",
            inputSchema={
                "type": "object",
                "properties": {},
                "required": [],
            },
        ),
        # Design Phase Tools
        Tool(
            name="c4_save_design",
            description="Save design specification for a feature including architecture options, components, and decisions.",
            inputSchema={
                "type": "object",
                "properties": {
                    "feature": {
                        "type": "string",
                        "description": "Feature name (e.g., 'user-auth')",
                    },
                    "domain": {
                        "type": "string",
                        "enum": ["web-frontend", "web-backend", "fullstack", "ml-dl", "mobile-app", "infra", "library", "unknown"],
                        "description": "Project domain",
                    },
                    "description": {
                        "type": "string",
                        "description": "Optional feature description",
                    },
                    "selected_option": {
                        "type": "string",
                        "description": "ID of selected architecture option",
                    },
                    "options": {
                        "type": "array",
                        "items": {
                            "type": "object",
                            "properties": {
                                "id": {"type": "string"},
                                "name": {"type": "string"},
                                "description": {"type": "string"},
                                "complexity": {"type": "string", "enum": ["low", "medium", "high"]},
                                "pros": {"type": "array", "items": {"type": "string"}},
                                "cons": {"type": "array", "items": {"type": "string"}},
                                "recommended": {"type": "boolean"},
                            },
                            "required": ["id", "name", "description"],
                        },
                        "description": "Architecture options",
                    },
                    "components": {
                        "type": "array",
                        "items": {
                            "type": "object",
                            "properties": {
                                "name": {"type": "string"},
                                "type": {"type": "string"},
                                "description": {"type": "string"},
                                "responsibilities": {"type": "array", "items": {"type": "string"}},
                                "dependencies": {"type": "array", "items": {"type": "string"}},
                                "interfaces": {"type": "array", "items": {"type": "string"}},
                            },
                            "required": ["name", "type", "description"],
                        },
                        "description": "Component designs",
                    },
                    "decisions": {
                        "type": "array",
                        "items": {
                            "type": "object",
                            "properties": {
                                "id": {"type": "string"},
                                "question": {"type": "string"},
                                "decision": {"type": "string"},
                                "rationale": {"type": "string"},
                                "alternatives_considered": {"type": "array", "items": {"type": "string"}},
                            },
                            "required": ["id", "question", "decision", "rationale"],
                        },
                        "description": "Design decisions",
                    },
                    "mermaid_diagram": {
                        "type": "string",
                        "description": "Mermaid diagram source",
                    },
                    "constraints": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "Technical constraints",
                    },
                    "nfr": {
                        "type": "object",
                        "additionalProperties": {"type": "string"},
                        "description": "Non-functional requirements (e.g., {'latency': '<500ms'})",
                    },
                },
                "required": ["feature", "domain"],
            },
        ),
        Tool(
            name="c4_get_design",
            description="Get design specification for a feature",
            inputSchema={
                "type": "object",
                "properties": {
                    "feature": {
                        "type": "string",
                        "description": "Feature name to retrieve",
                    },
                },
                "required": ["feature"],
            },
        ),
        Tool(
            name="c4_list_designs",
            description="List all features with design specifications",
            inputSchema={
                "type": "object",
                "properties": {},
                "required": [],
            },
        ),
        Tool(
            name="c4_design_complete",
            description="Mark design phase as complete and transition to PLAN state. "
            "Requires at least one design with selected option.",
            inputSchema={
                "type": "object",
                "properties": {},
                "required": [],
            },
        ),
        Tool(
            name="c4_test_agent_routing",
            description="Test agent routing configuration. Debug which agent is assigned for domain/task_type combinations.",
            inputSchema={
                "type": "object",
                "properties": {
                    "domain": {
                        "type": "string",
                        "description": "Domain to test (e.g., 'web-frontend', 'ml-dl'). If not provided, shows all domains.",
                    },
                    "task_type": {
                        "type": "string",
                        "description": "Task type to test (e.g., 'debug', 'security'). Shows override if exists.",
                    },
                },
                "required": [],
            },
        ),
        Tool(
            name="c4_query_agent_graph",
            description="Query the agent graph for agents, skills, domains, paths, and chains. Supports filtering and Mermaid output.",
            inputSchema={
                "type": "object",
                "properties": {
                    "query_type": {
                        "type": "string",
                        "description": "Type of query: 'overview', 'agents', 'skills', 'domains', 'path', 'chain'",
                        "enum": ["overview", "agents", "skills", "domains", "path", "chain"],
                    },
                    "filter_by": {
                        "type": "string",
                        "description": "Filter type: 'skill', 'domain', 'agent'",
                        "enum": ["skill", "domain", "agent"],
                    },
                    "filter_value": {
                        "type": "string",
                        "description": "Value to filter by",
                    },
                    "output_format": {
                        "type": "string",
                        "description": "Output format: 'json' or 'mermaid'",
                        "enum": ["json", "mermaid"],
                    },
                    "from_agent": {
                        "type": "string",
                        "description": "Source agent for path/chain queries",
                    },
                    "to_agent": {
                        "type": "string",
                        "description": "Target agent for path query",
                    },
                },
                "required": [],
            },
        ),
        Tool(
            name="c4_replace_symbol_body",
            description=(
                "Replace the body of a symbol (function, class, method). "
                "Returns details about the edit performed."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "name_path": {
                        "type": "string",
                        "description": "Symbol name or qualified name (e.g., 'MyClass.method')",
                    },
                    "file_path": {
                        "type": "string",
                        "description": "File containing the symbol (optional)",
                    },
                    "new_body": {
                        "type": "string",
                        "description": "New source code for the symbol body",
                    },
                },
                "required": ["name_path", "new_body"],
            },
        ),
        Tool(
            name="c4_insert_before_symbol",
            description="Insert content before a symbol definition.",
            inputSchema={
                "type": "object",
                "properties": {
                    "name_path": {
                        "type": "string",
                        "description": "Symbol name or qualified name",
                    },
                    "file_path": {
                        "type": "string",
                        "description": "File containing the symbol (optional)",
                    },
                    "content": {
                        "type": "string",
                        "description": "Content to insert before the symbol",
                    },
                },
                "required": ["name_path", "content"],
            },
        ),
        Tool(
            name="c4_insert_after_symbol",
            description="Insert content after a symbol definition.",
            inputSchema={
                "type": "object",
                "properties": {
                    "name_path": {
                        "type": "string",
                        "description": "Symbol name or qualified name",
                    },
                    "file_path": {
                        "type": "string",
                        "description": "File containing the symbol (optional)",
                    },
                    "content": {
                        "type": "string",
                        "description": "Content to insert after the symbol",
                    },
                },
                "required": ["name_path", "content"],
            },
        ),
        Tool(
            name="c4_rename_symbol",
            description=(
                "Rename a symbol across the entire codebase. "
                "Finds all references and renames them."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "name_path": {
                        "type": "string",
                        "description": "Current symbol name or qualified name",
                    },
                    "file_path": {
                        "type": "string",
                        "description": "File containing the symbol definition (optional)",
                    },
                    "new_name": {
                        "type": "string",
                        "description": "New name for the symbol",
                    },
                },
                "required": ["name_path", "new_name"],
            },
        ),
        Tool(
            name="c4_find_symbol",
            description=(
                "Find symbols matching a name path pattern. "
                "Use 'ClassName/method' for methods, simple name for any symbol. "
                "Supports both single file (e.g., 'src/main.py') and directory search (e.g., 'src/'). "
                "Directory search finds all .py files recursively."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "name_path_pattern": {
                        "type": "string",
                        "description": "Pattern to match (e.g., 'MyClass/my_method', 'function_name')",
                    },
                    "relative_path": {
                        "type": "string",
                        "description": (
                            "File or directory path to search in (REQUIRED). "
                            "Examples: 'c4/lsp/provider.py' (single file), 'c4/lsp/' (all .py in directory)"
                        ),
                    },
                    "include_body": {
                        "type": "boolean",
                        "description": "Include symbol body in results (default: false)",
                        "default": False,
                    },
                    "depth": {
                        "type": "integer",
                        "description": "Depth of children to include (default: 0)",
                        "default": 0,
                    },
                },
                "required": ["name_path_pattern", "relative_path"],
            },
        ),
        Tool(
            name="c4_get_symbols_overview",
            description=(
                "Get an overview of symbols in a file. "
                "Use this first to understand a file's structure."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "relative_path": {
                        "type": "string",
                        "description": "Path to the file (relative to project root)",
                    },
                    "depth": {
                        "type": "integer",
                        "description": "Depth of children to include (0 = top-level only)",
                        "default": 0,
                    },
                },
                "required": ["relative_path"],
            },
        ),
        # File operation tools
        Tool(
            name="c4_read_file",
            description=(
                "Read a file or portion of it. Returns file content with line numbers."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "relative_path": {
                        "type": "string",
                        "description": "Path to the file (relative to project root)",
                    },
                    "start_line": {
                        "type": "integer",
                        "description": "0-based index of first line to read",
                        "default": 0,
                    },
                    "end_line": {
                        "type": "integer",
                        "description": "0-based index of last line (inclusive), null for end",
                    },
                },
                "required": ["relative_path"],
            },
        ),
        Tool(
            name="c4_create_text_file",
            description="Create or overwrite a text file",
            inputSchema={
                "type": "object",
                "properties": {
                    "relative_path": {
                        "type": "string",
                        "description": "Path to the file (relative to project root)",
                    },
                    "content": {
                        "type": "string",
                        "description": "Content to write to the file",
                    },
                },
                "required": ["relative_path", "content"],
            },
        ),
        Tool(
            name="c4_list_dir",
            description="List files and directories",
            inputSchema={
                "type": "object",
                "properties": {
                    "relative_path": {
                        "type": "string",
                        "description": "Path relative to project root (use '.' for root)",
                        "default": ".",
                    },
                    "recursive": {
                        "type": "boolean",
                        "description": "Whether to scan subdirectories",
                        "default": False,
                    },
                },
                "required": [],
            },
        ),
        Tool(
            name="c4_find_file",
            description="Find files matching a glob pattern",
            inputSchema={
                "type": "object",
                "properties": {
                    "file_mask": {
                        "type": "string",
                        "description": "Filename or glob pattern (e.g., '*.py', 'test_*.py')",
                    },
                    "relative_path": {
                        "type": "string",
                        "description": "Directory to search in",
                        "default": ".",
                    },
                },
                "required": ["file_mask"],
            },
        ),
        Tool(
            name="c4_search_for_pattern",
            description="Search for a regex pattern in files",
            inputSchema={
                "type": "object",
                "properties": {
                    "pattern": {
                        "type": "string",
                        "description": "Regular expression pattern to search for",
                    },
                    "relative_path": {
                        "type": "string",
                        "description": "Directory or file to search in",
                        "default": ".",
                    },
                    "glob_pattern": {
                        "type": "string",
                        "description": "Optional glob to filter files (e.g., '*.py')",
                    },
                    "context_lines": {
                        "type": "integer",
                        "description": "Number of context lines before/after match",
                        "default": 0,
                    },
                },
                "required": ["pattern"],
            },
        ),
        Tool(
            name="c4_replace_content",
            description="Replace content in a file using literal or regex matching",
            inputSchema={
                "type": "object",
                "properties": {
                    "relative_path": {
                        "type": "string",
                        "description": "Path to the file (relative to project root)",
                    },
                    "needle": {
                        "type": "string",
                        "description": "String or regex pattern to search for",
                    },
                    "replacement": {
                        "type": "string",
                        "description": "Replacement string",
                    },
                    "mode": {
                        "type": "string",
                        "enum": ["literal", "regex"],
                        "description": "'literal' for exact match, 'regex' for regex",
                        "default": "literal",
                    },
                    "allow_multiple": {
                        "type": "boolean",
                        "description": "Whether to allow multiple replacements",
                        "default": False,
                    },
                },
                "required": ["relative_path", "needle", "replacement"],
            },
        ),
        Tool(
            name="c4_worktree_status",
            description="Get worktree status for all workers or a specific worker. Returns worktree paths, branches, and status.",
            inputSchema={
                "type": "object",
                "properties": {
                    "worker_id": {
                        "type": "string",
                        "description": "Worker ID to get status for. If not provided, returns all worktrees.",
                    },
                },
                "required": [],
            },
        ),
        Tool(
            name="c4_worktree_cleanup",
            description="Clean up worktrees, optionally keeping active workers. Returns the count of deleted worktrees.",
            inputSchema={
                "type": "object",
                "properties": {
                    "keep_active": {
                        "type": "boolean",
                        "description": "If True, keep worktrees for workers with in_progress tasks. If False, remove all worktrees.",
                        "default": True,
                    },
                },
                "required": [],
            },
        ),
        # Memory tools
        Tool(
            name="c4_write_memory",
            description="Write content to a memory. Persists across sessions for architecture decisions, patterns.",
            inputSchema={
                "type": "object",
                "properties": {
                    "name": {
                        "type": "string",
                        "description": "Memory name (e.g., 'architecture-decisions'). Sanitized for file storage.",
                    },
                    "content": {
                        "type": "string",
                        "description": "Content to write (markdown format recommended)",
                    },
                },
                "required": ["name", "content"],
            },
        ),
        Tool(
            name="c4_read_memory",
            description="Read content from a memory. Returns the content if found, or indicates not found.",
            inputSchema={
                "type": "object",
                "properties": {
                    "name": {
                        "type": "string",
                        "description": "Memory name to read",
                    },
                },
                "required": ["name"],
            },
        ),
        Tool(
            name="c4_list_memories",
            description="List all available memories. Can optionally filter by pattern.",
            inputSchema={
                "type": "object",
                "properties": {
                    "pattern": {
                        "type": "string",
                        "description": "Optional search pattern to filter memories (e.g., 'adr' to find all ADR memories)",
                    },
                },
                "required": [],
            },
        ),
        Tool(
            name="c4_delete_memory",
            description="Delete a memory by name.",
            inputSchema={
                "type": "object",
                "properties": {
                    "name": {
                        "type": "string",
                        "description": "Memory name to delete",
                    },
                },
                "required": ["name"],
            },
        ),
    ]
