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
                    "model": {
                        "type": "string",
                        "enum": ["opus", "sonnet", "haiku"],
                        "description": "Claude model tier for this task (default: opus). Use sonnet for simpler tasks to reduce cost.",
                    },
                    "execution_mode": {
                        "type": "string",
                        "enum": ["worker", "direct", "auto"],
                        "description": "Execution mode: worker (full protocol), direct (lightweight claim/report), auto. Default: worker.",
                        "default": "worker",
                    },
                    "review_required": {
                        "type": "boolean",
                        "description": "Whether to auto-generate review task on completion. Default: true.",
                        "default": True,
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
            name="c4_cleanup_workers",
            description=(
                "Purge all stale/zombie workers (one-time cleanup). "
                "Removes idle workers, zombie busy workers, and TTL-expired workers."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "max_idle_minutes": {
                        "type": "number",
                        "description": "Override for idle worker threshold (uses config default if not provided)",
                    },
                },
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
        # Direct Mode Tools (c4_claim / c4_report)
        Tool(
            name="c4_claim",
            description=(
                "Claim a task for direct execution by the main session (no worker protocol). "
                "Lightweight alternative to c4_get_task: no branch creation, no worker registration. "
                "Creates .c4/active_claim.json for hook enforcement."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "task_id": {
                        "type": "string",
                        "description": "ID of the task to claim (e.g., T-DASH-001-0)",
                    },
                },
                "required": ["task_id"],
            },
        ),
        Tool(
            name="c4_report",
            description=(
                "Report task completion for direct mode. "
                "Lightweight alternative to c4_submit: records summary, marks done, "
                "optionally creates review task. Deletes .c4/active_claim.json."
            ),
            inputSchema={
                "type": "object",
                "properties": {
                    "task_id": {
                        "type": "string",
                        "description": "ID of the completed task",
                    },
                    "summary": {
                        "type": "string",
                        "description": "Summary of work done",
                    },
                    "files_changed": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "List of files changed during the work",
                    },
                },
                "required": ["task_id", "summary"],
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
        Tool(
            name="c4_search_memory",
            description="Search memories using hybrid semantic and keyword search. Returns relevant memories with previews and scores.",
            inputSchema={
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "Search query to find relevant memories",
                    },
                    "limit": {
                        "type": "integer",
                        "description": "Maximum number of results to return (default: 10)",
                        "default": 10,
                    },
                    "filters": {
                        "type": "object",
                        "description": "Optional filters to narrow search",
                        "properties": {
                            "memory_type": {
                                "type": "string",
                                "description": "Filter by memory source/type (e.g., 'read_file', 'user_message')",
                            },
                            "tags": {
                                "type": "array",
                                "items": {"type": "string"},
                                "description": "Filter by tags (OR logic)",
                            },
                            "since": {
                                "type": "string",
                                "description": "Filter by creation date (ISO format, e.g., '2024-01-01T00:00:00')",
                            },
                        },
                    },
                },
                "required": ["query"],
            },
        ),
        Tool(
            name="c4_get_memory_detail",
            description="Get full details of a specific memory including content, metadata, and optionally related memories.",
            inputSchema={
                "type": "object",
                "properties": {
                    "memory_id": {
                        "type": "string",
                        "description": "ID of the memory to retrieve (e.g., 'obs-abc123')",
                    },
                    "include_related": {
                        "type": "boolean",
                        "description": "Include related memories based on content similarity (default: false)",
                        "default": False,
                    },
                },
                "required": ["memory_id"],
            },
        ),
        # Git history analysis tool
        Tool(
            name="c4_analyze_history",
            description="Analyze git commit history, cluster related commits, and generate narrative stories. "
            "Returns stories with titles, commits, and dependencies, plus a dependency graph.",
            inputSchema={
                "type": "object",
                "properties": {
                    "since": {
                        "type": "string",
                        "description": "Start date in ISO format (e.g., '2025-01-01'). Required.",
                    },
                    "until": {
                        "type": "string",
                        "description": "End date in ISO format (optional). If not provided, includes all commits since 'since'.",
                    },
                    "branch": {
                        "type": "string",
                        "description": "Branch to analyze (default: HEAD).",
                        "default": "HEAD",
                    },
                    "save_to_knowledge": {
                        "type": "boolean",
                        "description": "Save generated stories as knowledge insight documents (default: false).",
                        "default": False,
                    },
                },
                "required": ["since"],
            },
        ),
        # Git commit search tool
        Tool(
            name="c4_search_commits",
            description="Search git commits using semantic matching. "
            "Returns relevant commits with scores based on query similarity.",
            inputSchema={
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "Semantic search query to find relevant commits. Required.",
                    },
                    "filters": {
                        "type": "object",
                        "description": "Optional filters to narrow search results.",
                        "properties": {
                            "author": {
                                "type": "string",
                                "description": "Filter by commit author name.",
                            },
                            "since": {
                                "type": "string",
                                "description": "Start date in ISO format (e.g., '2025-01-01').",
                            },
                            "path": {
                                "type": "string",
                                "description": "Filter by file/directory path (e.g., 'src/auth/').",
                            },
                            "story_id": {
                                "type": "string",
                                "description": "Filter by story ID to find related commits.",
                            },
                        },
                    },
                },
                "required": ["query"],
            },
        ),
        # GPU tools
        Tool(
            name="c4_gpu_status",
            description="Get GPU status - available GPUs, VRAM, utilization. Returns GPU count, backend, and per-GPU details.",
            inputSchema={
                "type": "object",
                "properties": {},
                "required": [],
            },
        ),
        Tool(
            name="c4_job_submit",
            description="Submit a GPU job for execution. Allocates GPU and runs the command.",
            inputSchema={
                "type": "object",
                "properties": {
                    "command": {
                        "type": "string",
                        "description": "Command to execute (e.g., 'python train.py')",
                    },
                    "task_id": {
                        "type": "string",
                        "description": "Optional C4 task ID to link the job to",
                    },
                    "gpu_count": {
                        "type": "integer",
                        "description": "Number of GPUs to allocate (default: 1)",
                        "default": 1,
                    },
                    "working_dir": {
                        "type": "string",
                        "description": "Working directory for the job",
                    },
                },
                "required": ["command"],
            },
        ),
        Tool(
            name="c4_job_status",
            description="Get GPU job status. Returns details for a specific job or lists all jobs.",
            inputSchema={
                "type": "object",
                "properties": {
                    "job_id": {
                        "type": "string",
                        "description": "Job ID to check. If omitted, returns all jobs.",
                    },
                },
                "required": [],
            },
        ),
        # Knowledge tools
        Tool(
            name="c4_experiment_search",
            description="Search experiment knowledge base by keyword. Returns matching experiments with titles, hypotheses, and results.",
            inputSchema={
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "Search query (keywords matched against title, hypothesis, lessons, tags)",
                    },
                    "top_k": {
                        "type": "integer",
                        "description": "Maximum results to return (default: 5)",
                        "default": 5,
                    },
                    "domain": {
                        "type": "string",
                        "description": "Optional domain filter (e.g., 'ml-dl')",
                    },
                },
                "required": ["query"],
            },
        ),
        Tool(
            name="c4_experiment_record",
            description="Record an experiment result to the knowledge store. Stores hypothesis, config, metrics, and lessons.",
            inputSchema={
                "type": "object",
                "properties": {
                    "title": {
                        "type": "string",
                        "description": "Experiment title",
                    },
                    "task_id": {
                        "type": "string",
                        "description": "Related C4 task ID",
                    },
                    "hypothesis": {
                        "type": "string",
                        "description": "Hypothesis being tested",
                    },
                    "config": {
                        "type": "object",
                        "description": "Experiment configuration (algorithm, hyperparams, etc.)",
                    },
                    "result": {
                        "type": "object",
                        "description": "Result dict with 'metrics', 'success', 'error_message'",
                    },
                    "lessons_learned": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "Lessons learned from this experiment",
                    },
                    "tags": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "Tags for categorization",
                    },
                    "domain": {
                        "type": "string",
                        "description": "Domain (e.g., 'ml-dl', 'web-backend')",
                    },
                },
                "required": ["title"],
            },
        ),
        Tool(
            name="c4_pattern_suggest",
            description="Get pattern-based suggestions from experiment knowledge. "
            "Returns discovered patterns, success rates, and best practices.",
            inputSchema={
                "type": "object",
                "properties": {
                    "domain": {
                        "type": "string",
                        "description": "Domain to get patterns for (optional)",
                    },
                    "include_best_practices": {
                        "type": "boolean",
                        "description": "Include best practice recommendations (default: true)",
                        "default": True,
                    },
                },
                "required": [],
            },
        ),
        # Artifact tools
        Tool(
            name="c4_artifact_list",
            description="List artifacts for a task. Returns artifact names, types, sizes, and content hashes.",
            inputSchema={
                "type": "object",
                "properties": {
                    "task_id": {
                        "type": "string",
                        "description": "Task ID to list artifacts for",
                    },
                },
                "required": ["task_id"],
            },
        ),
        Tool(
            name="c4_artifact_save",
            description="Save a file as a content-addressable artifact. Deduplicates by SHA256 hash.",
            inputSchema={
                "type": "object",
                "properties": {
                    "task_id": {
                        "type": "string",
                        "description": "Related task ID",
                    },
                    "path": {
                        "type": "string",
                        "description": "Local file path to save as artifact",
                    },
                    "artifact_type": {
                        "type": "string",
                        "enum": ["source", "data", "output"],
                        "description": "Artifact type (default: output)",
                        "default": "output",
                    },
                },
                "required": ["task_id", "path"],
            },
        ),
        Tool(
            name="c4_artifact_get",
            description="Get artifact path and metadata by task ID and name.",
            inputSchema={
                "type": "object",
                "properties": {
                    "task_id": {
                        "type": "string",
                        "description": "Task ID",
                    },
                    "name": {
                        "type": "string",
                        "description": "Artifact name",
                    },
                    "version": {
                        "type": "integer",
                        "description": "Specific version (optional, latest if omitted)",
                    },
                },
                "required": ["task_id", "name"],
            },
        ),
    ]
