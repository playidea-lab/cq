# C4 MCP Tools (133, 107 base + 26 hub)

## Layer 1: Daily tools (6)

| Tool | Purpose |
|------|---------|
| /c4-status | Check project status |
| /c4-quick "desc" | Start task now |
| /c4-run [N] | Parallel workers |
| /c4-submit | Submit completion |
| /c4-validate | lint + test |
| c4_claim / c4_report | Direct mode task |

## Layer 2: Weekly/situational (16)

| Category | Tools |
|----------|-------|
| Planning | /c4-plan, /c4-add-task, /c4-interview |
| Review | /c4-checkpoint, /c4-swarm --review |
| Research | /c4-research, /c4-review |
| Knowledge | c4_knowledge_record, c4_knowledge_search, c4_pattern_suggest |
| Reflection | c4_reflect |
| Hub | c4_hub_submit, c4_hub_watch, c4_hub_summary |
| Lighthouse | c4_lighthouse |
| Cost | c4_llm_costs |

## Layer 3: Internal tools (80+)

| Category | Count | Examples |
|----------|:-----:|----------|
| Task mgmt | 6 | c4_add_todo, c4_get_task, c4_submit |
| Files/search | 6 | c4_find_file, c4_read_file, c4_search_for_pattern |
| Git | 4 | c4_worktree_status, c4_analyze_history |
| LSP/symbols | 7 | c4_find_symbol, c4_replace_symbol_body |
| Discovery | 8 | c4_save_spec, c4_save_design |
| Artifact | 3 | c4_artifact_save, c4_artifact_get |
| Soul/Persona | 7 | c4_soul_get, c4_persona_evolve |
| LLM Gateway | 3 | c4_llm_call, c4_llm_providers |
| CDP | 2 | c4_cdp_run, c4_cdp_list |
| C2 docs | 8 | c4_parse_document, c4_extract_text |
| Hub (full) | 26 | Job, DAG, Edge, Deploy, Artifact |
| Other | 5 | c4_onboard, c4_run_validation, c4_clear |
