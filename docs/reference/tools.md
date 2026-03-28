# MCP Tools Reference

CQ exposes **169 MCP tools** to AI agents (Claude Code, Cursor, Codex CLI, Gemini, ChatGPT via External Brain). Tools are organized into categories based on their function.

**Tool tiers:**
- **Core** (40 tools) — always loaded, available immediately
- **Extended** (129 tools) — loaded on demand, available after MCP initialization

---

## Project & State Management (Core)

Tools for managing the C4 project lifecycle, state machine, and task queue.

| Tool | Description |
|------|-------------|
| `c4_status` | Show current project state, task counts, and active workers |
| `c4_start` | Initialize C4 project (creates `.c4/` database, config) |
| `c4_stop` | Transition to HALTED state, preserve current progress |
| `c4_get_task` | Request next task from queue (Worker mode) |
| `c4_submit` | Submit completed task with commit SHA and validation results |
| `c4_claim` | Claim a task for direct implementation (Direct mode) |
| `c4_report` | Report task completion (Direct mode) |
| `c4_mark_blocked` | Mark a task as blocked with reason |
| `c4_request_changes` | Request changes on a submitted task |
| `c4_add_todo` | Add a new task to the queue |
| `c4_task_list` | List tasks with optional filter (status, domain, id) |
| `c4_stale_tasks` | Show tasks stuck in `in_progress` state |
| `c4_worker_heartbeat` | Send worker heartbeat to keep lease alive |
| `c4_workers` | List active workers and their current tasks |
| `c4_dashboard` | Project status summary widget (`format=widget` for UI) |
| `c4_task_graph` | Visual dependency graph of all tasks (`format=widget` for UI) |

---

## File Operations (Core)

Tools for reading, searching, and navigating the codebase.

| Tool | Description |
|------|-------------|
| `c4_read_file` | Read a file with line numbers and optional range |
| `c4_find_file` | Fuzzy-find files by name pattern |
| `c4_search_for_pattern` | Regex/literal search across files (ripgrep-powered) |
| `c4_list_dir` | List directory contents with metadata |
| `c4_create_text_file` | Create or overwrite a text file |
| `c4_replace_content` | Surgical find-and-replace within a file |
| `c4_diff_summary` | Show staged + unstaged git diff summary (`format=widget` for UI) |
| `c4_execute` | Execute shell commands (gated by permission hook) |

---

## Git Operations (Core)

| Tool | Description |
|------|-------------|
| `c4_git_log` | Show recent commits with conventional commit parsing |
| `c4_git_status` | Show working tree status |
| `c4_git_diff` | Show diff between revisions or files |

---

## Validation & Testing (Core)

| Tool | Description |
|------|-------------|
| `c4_run_validation` | Run lint + tests (auto-detects: go test, pytest, cargo test, pnpm test). Severity: CRITICAL/HIGH/MEDIUM/LOW (`format=widget` for UI) |
| `c4_error_trace` | Format and display error trace for debugging (`format=widget` for UI) |

---

## Config & Health (Core)

| Tool | Description |
|------|-------------|
| `c4_config_get` | Read config values from `.c4/config.yaml` |
| `c4_health` | Check health of all CQ components (Doctor equivalent) |
| `c4_version` | Show CQ binary version and build tags |

---

## Specs & Designs (Extended)

Tools for structured specification and architecture decision records.

| Tool | Description |
|------|-------------|
| `c4_get_spec` | Read a spec document from `docs/specs/` |
| `c4_save_spec` | Write or update a spec document |
| `c4_list_specs` | List all specs with metadata |
| `c4_get_design` | Read an architecture design document |
| `c4_save_design` | Write or update a design document |
| `c4_list_designs` | List all design documents |
| `c4_discovery_complete` | Mark discovery phase complete, transition to design |
| `c4_design_complete` | Mark design phase complete, transition to planning |

---

## Lighthouse & Checkpoints (Extended)

| Tool | Description |
|------|-------------|
| `c4_lighthouse` | Check if implementation matches Lighthouse contracts |
| `c4_checkpoint` | Trigger manual checkpoint review |
| `c4_artifact_save` | Save build artifact with content hash |
| `c4_artifact_load` | Load a previously saved artifact |

---

## Knowledge (Extended)

> Requires `connected` or `full` tier for cloud sync. FTS5 + pgvector (1536-dim OpenAI) + 3-way RRF.

| Tool | Description |
|------|-------------|
| `c4_knowledge_record` | Store knowledge (decisions, patterns, insights, errors, discoveries). AI self-capture: tool description triggers autonomous saves. |
| `c4_knowledge_search` | Search knowledge: vector + FTS + ilike 3-stage fallback. Returns ranked results. |
| `c4_knowledge_distill` | Auto-distill knowledge when doc count ≥ 5 (cluster + summarize) |
| `c4_knowledge_ingest` | Batch ingest files or URLs into knowledge store |
| `c4_knowledge_sync` | Sync local knowledge to/from Supabase cloud |
| `c4_knowledge_publish` | Publish knowledge to shared project namespace |
| `c4_knowledge_pull` | Pull published knowledge from another project |
| `c4_knowledge_usage` | Show knowledge access statistics |
| `c4_knowledge_embed` | Generate embedding for a text (without storing) |
| `c4_knowledge_chunk` | Chunk a document for embedding with overlap |
| `c4_pattern_suggest` | Suggest implementation patterns from knowledge base |
| `c4_knowledge_feed` | Real-time knowledge feed widget (`format=widget` for UI) |
| `c4_knowledge_status` | Show knowledge store stats (count, last sync, coverage) |

---

## Session Intelligence (Extended)

| Tool | Description |
|------|-------------|
| `c4_session_index` | List all sessions with summaries and timestamps |
| `c4_session_summarize` | Summarize current session using LLM (safety net capture) |
| `c4_session_snapshot` | Take a manual context snapshot |
| `c4_session_recall` | Recall a previous session's context |
| `c4_session_summary` | Capture complete session summary on conversation end |

---

## Memory (Extended)

| Tool | Description |
|------|-------------|
| `c4_memory_import` | Import ChatGPT or Claude conversation exports into knowledge store |

---

## Drive (Extended)

> Cloud file storage with TUS resumable upload. Requires `connected` or `full` tier.

| Tool | Description |
|------|-------------|
| `c4_drive_upload` | Upload a file to Drive (TUS resumable, content-addressable) |
| `c4_drive_download` | Download a file from Drive (Range-based resume) |
| `c4_drive_list` | List files in Drive with metadata |
| `c4_drive_delete` | Delete a file from Drive |
| `c4_drive_info` | Get file metadata (size, hash, URLs) |
| `c4_drive_mkdir` | Create a directory in Drive |

---

## File Index (Extended)

> Cross-device file search — find files across all connected machines.

| Tool | Description |
|------|-------------|
| `c4_fileindex_search` | Search for files by name/path across all indexed devices |
| `c4_fileindex_status` | Show file index coverage and last update time |

---

## Relay (Extended)

| Tool | Description |
|------|-------------|
| `cq_workers` | List workers connected via relay with latency and tunnel info |
| `cq_relay_call` | Call an MCP tool on a remote worker via relay |

---

## LLM Gateway (Extended)

> Requires `llm_gateway` build tag. Multi-provider with prompt caching.

| Tool | Description |
|------|-------------|
| `c4_llm_call` | Call LLM with provider routing (Anthropic/OpenAI/Gemini/Ollama) |
| `c4_llm_providers` | List configured LLM providers and their availability |
| `c4_llm_costs` | Show LLM cost breakdown and cache savings (`format=widget` for UI) |
| `c4_llm_usage_stats` | Detailed token usage stats per session/project |

---

## Hub — Distributed Jobs (Conditional, Hub tier)

> Requires Hub connection (`serve.hub.enabled: true` + cloud credentials).

### Job Operations

| Tool | Description |
|------|-------------|
| `c4_job_submit` | Submit a job to the Hub queue with spec and routing |
| `c4_job_status` | Get job status and progress (`format=widget` for UI) |
| `c4_job_summary` | Get job results and output artifacts (`format=widget` for UI) |
| `c4_job_cancel` | Cancel a running or pending job |
| `c4_job_list` | List recent jobs with status and metrics |
| `c4_job_logs` | Stream job execution logs |

### Worker Operations

| Tool | Description |
|------|-------------|
| `c4_hub_workers` | List workers with affinity scores and current load |
| `c4_hub_worker_status` | Get detailed worker status and capabilities |
| `c4_hub_worker_tags` | Update worker tags for routing |
| `c4_nodes_map` | Visual map of all connected nodes (`format=widget` for UI) |

### DAG Pipelines

| Tool | Description |
|------|-------------|
| `c4_hub_dag_create` | Create a DAG pipeline with nodes and edges |
| `c4_hub_dag_status` | Get DAG execution status with per-node progress |
| `c4_hub_dag_list` | List all DAGs |
| `c4_hub_dag_cancel` | Cancel a running DAG |

### Artifacts

| Tool | Description |
|------|-------------|
| `c4_hub_artifact_upload` | Upload artifact to Hub storage |
| `c4_hub_artifact_download` | Download artifact from Hub |
| `c4_hub_artifact_list` | List artifacts for a job |

### Cron Scheduling

| Tool | Description |
|------|-------------|
| `c4_cron_create` | Register a cron schedule with job spec |
| `c4_cron_list` | List cron schedules with last-run times |
| `c4_cron_delete` | Delete a cron schedule |

### Worker Standby (Hub conditional)

| Tool | Description |
|------|-------------|
| `c4_worker_standby` | Enter standby mode — wait for and execute Hub jobs |
| `c4_worker_complete` | Signal job completion from standby worker |
| `c4_worker_shutdown` | Graceful worker shutdown with task handoff |

---

## Experiments (Extended)

| Tool | Description |
|------|-------------|
| `c4_experiment_record` | Record experiment result (exp_id, metrics, config) |
| `c4_experiment_search` | Search experiments by metric, date, or tags (`format=widget` for UI) |
| `c4_experiment_compare` | Compare two or more experiments side-by-side |

---

## Soul & Persona (Extended)

| Tool | Description |
|------|-------------|
| `c4_soul_evolve` | Update soul/judgment criteria based on outcomes |
| `c4_soul_check` | Evaluate a decision against current soul |
| `c4_persona_learn` | Record a learned behavior pattern |
| `c4_persona_learn_from_diff` | Auto-extract patterns from a git diff |
| `c4_persona_apply` | Apply persona context to current task |
| `c4_persona_status` | Show current persona profile and learned patterns |
| `c4_twin_record` | Record an interaction to the digital twin |
| `c4_twin_query` | Query the digital twin for behavior prediction |
| `c4_growth_loop` | Trigger growth loop evaluation cycle |
| `c4_global_knowledge` | Push local persona patterns to global knowledge |

---

## CDP (Chrome DevTools Protocol, Extended)

| Tool | Description |
|------|-------------|
| `c4_cdp_run` | Execute JavaScript in a browser tab via CDP |
| `c4_webmcp_discover` | Auto-discover Web MCP endpoints on a page |
| `c4_webmcp_call` | Call a discovered Web MCP tool |
| `c4_web_fetch` | Fetch URL with content negotiation + HTML→Markdown conversion |
| `c4_cdp_screenshot` | Take a screenshot of current browser state |

---

## Notifications & EventBus (Extended)

| Tool | Description |
|------|-------------|
| `c4_notify` | Send notification via configured channels (Telegram/Dooray) |
| `c4_notification_channels` | List configured notification channels |
| `c4_rule_add` | Add EventBus rule with optional notification channel |
| `c4_rule_list` | List all EventBus routing rules |
| `c4_event_publish` | Publish an event to EventBus |
| `c4_event_subscribe` | Subscribe to EventBus events (streaming) |
| `c4_mail_send` | Send inter-session mail to a named session |
| `c4_mail_list` | List messages in the mail inbox |
| `c4_mail_read` | Read a specific mail message |

---

## Skill Evaluation (Extended)

| Tool | Description |
|------|-------------|
| `c4_skill_eval_run` | Run k-trial haiku classification on a skill's test cases |
| `c4_skill_eval_generate` | Generate positive + negative test cases for a skill |
| `c4_skill_eval_status` | Show trigger accuracy for all evaluated skills (threshold: 0.90) |

---

## Research Loop (Extended)

| Tool | Description |
|------|-------------|
| `c4_research_loop_start` | Start autonomous research loop (LoopOrchestrator) |
| `c4_research_loop_status` | Show current loop state, iteration count, best metric |
| `c4_research_loop_stop` | Stop the research loop gracefully |
| `c4_research_intervene` | Manually intervene to steer loop direction |

---

## Observability — C7 Observe (Conditional, `c7_observe` build tag)

| Tool | Description |
|------|-------------|
| `c4_observe_metrics` | Query collected metrics (tool calls, latency, error rates) |
| `c4_observe_logs` | Query structured logs with filter |
| `c4_observe_trace` | Show distributed trace for a request |
| `c4_observe_status` | Show observability pipeline health |

---

## Access Control — C6 Guard (Conditional, `c6_guard` build tag)

| Tool | Description |
|------|-------------|
| `c4_guard_check` | Check if an action is permitted by current policies |
| `c4_guard_audit` | Show audit log of guard decisions |
| `c4_guard_policy_add` | Add an RBAC policy rule |
| `c4_guard_policy_list` | List all active policies |
| `c4_guard_deny` | Manually deny an action with reason |

---

## External Connectors — C8 Gate (Conditional, `c8_gate` build tag)

| Tool | Description |
|------|-------------|
| `c4_gate_webhook` | Register a webhook for outbound notifications |
| `c4_gate_schedule` | Schedule a remote action |
| `c4_gate_slack_send` | Send a message to Slack |
| `c4_gate_github_pr` | Create or update a GitHub pull request |
| `c4_gate_connector_list` | List configured external connectors |
| `c4_gate_connector_test` | Test a connector endpoint |

---

## Python Sidecar (LSP, Extended)

> Python/JavaScript/TypeScript only. Go/Rust → use `c4_search_for_pattern` instead.

| Tool | Description |
|------|-------------|
| `c4_find_symbol` | Find symbol definition by name (Jedi/multilspy) |
| `c4_get_overview` | Get module/class/function overview |
| `c4_replace_body` | Replace function/class body |
| `c4_insert_before` | Insert code before a symbol |
| `c4_insert_after` | Insert code after a symbol |
| `c4_rename` | Rename a symbol across files |
| `c4_find_refs` | Find all references to a symbol |
| `c4_parse_document` | Parse a structured document (PDF, DOCX, HTML) |
| `c4_extract_text` | Extract plain text from a document |
| `c4_onboard` | Interactive onboarding for new CQ users |

---

## MCP Apps (Widget System)

When calling tools with `format=widget`, the response includes `_meta.ui.resourceUri`. MCP clients (Claude Code, Cursor, VS Code) fetch HTML via `resources/read` and render it in a sandboxed iframe.

| Widget URI | Tool | Description |
|-----------|------|-------------|
| `ui://cq/dashboard` | `c4_dashboard` | Project status summary |
| `ui://cq/job-progress` | `c4_job_status` | Job progress bar |
| `ui://cq/job-result` | `c4_job_summary` | Job results |
| `ui://cq/experiment-compare` | `c4_experiment_search` | Experiment comparison |
| `ui://cq/task-graph` | `c4_task_graph` | Task dependency graph |
| `ui://cq/nodes-map` | `c4_nodes_map` | Connected nodes map |
| `ui://cq/knowledge-feed` | `c4_knowledge_search` | Knowledge feed |
| `ui://cq/cost-tracker` | `c4_llm_costs` | LLM cost tracker |
| `ui://cq/test-results` | `c4_run_validation` | Test results |
| `ui://cq/git-diff` | `c4_diff_summary` | Git diff viewer |
| `ui://cq/error-trace` | `c4_error_trace` | Error trace viewer |

---

## Tool Count Summary

| Category | Count | Notes |
|----------|-------|-------|
| Project & State | 16 | Core |
| File Operations | 8 | Core |
| Git | 3 | Core |
| Validation | 2 | Core |
| Config & Health | 3 | Core |
| Specs & Designs | 8 | Extended |
| Lighthouse & Checkpoints | 4 | Extended |
| Knowledge | 13 | Extended, cloud for sync |
| Session Intelligence | 5 | Extended |
| Memory | 1 | Extended |
| Drive | 6 | Extended, cloud |
| File Index | 2 | Extended |
| Relay | 2 | Extended |
| LLM Gateway | 4 | Extended |
| Hub Jobs | 19 | Conditional (Hub tier) |
| Worker Standby | 3 | Conditional (Hub tier) |
| Experiments | 3 | Extended |
| Soul & Persona | 10 | Extended |
| CDP & WebMCP | 5 | Extended |
| Notifications & EventBus | 9 | Extended |
| Skill Evaluation | 3 | Extended |
| Research Loop | 4 | Extended |
| C7 Observe | 4 | Conditional (build tag) |
| C6 Guard | 5 | Conditional (build tag) |
| C8 Gate | 6 | Conditional (build tag) |
| Python Sidecar (LSP) | 10 | Extended, Python/JS/TS |
| **Total** | **169** | 118 base + 26 Hub + 25 conditional |
