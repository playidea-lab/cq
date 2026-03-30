# MCP Tools Reference

CQ exposes **217 MCP tools** to AI agents (Claude Code, Cursor, Codex CLI, Gemini, ChatGPT via External Brain). Tools are organized into categories based on their function.

**Tool tiers:**
- **Core** — always loaded, available immediately
- **Extended** — loaded on demand, available after MCP initialization
- **Conditional** — requires specific build tag or Hub connection

---

## Project & State Management (Core)

Tools for managing the C4 project lifecycle, state machine, and task queue.

| Tool | Description |
|------|-------------|
| `cq_status` | Show current project state, task counts, and active workers |
| `cq_start` | Initialize C4 project (creates `.c4/` database, config) |
| `cq_get_task` | Request next task from queue (Worker mode) |
| `cq_submit` | Submit completed task with commit SHA and validation results |
| `cq_claim` | Claim a task for direct implementation (Direct mode) |
| `cq_report` | Report task completion (Direct mode) |
| `cq_mark_blocked` | Mark a task as blocked with reason |
| `cq_request_changes` | Request changes on a submitted task |
| `cq_add_todo` | Add a new task to the queue |
| `cq_task_list` | List tasks with optional filter (status, domain, id) |
| `cq_stale_tasks` | Show tasks stuck in `in_progress` state |
| `cq_worker_heartbeat` | Send worker heartbeat to keep lease alive |
| `cq_workers` | List active workers and their current tasks |
| `cq_dashboard` | Project status summary widget (`format=widget` for UI) |
| `cq_task_graph` | Visual dependency graph of all tasks (`format=widget` for UI) |
| `cq_task_events` | Stream task state change events |
| `cq_reset_task` | Reset a task back to pending state |
| `cq_phase_lock_acquire` | Acquire a phase lock to prevent concurrent phase transitions |
| `cq_phase_lock_release` | Release a previously acquired phase lock |
| `cq_clear` | Reset C4 state (development/testing use) |

---

## File Operations (Core)

Tools for reading, searching, and navigating the codebase.

| Tool | Description |
|------|-------------|
| `cq_read_file` | Read a file with line numbers and optional range |
| `cq_find_file` | Fuzzy-find files by name pattern |
| `cq_file_find` | Alternative file finder with extended glob support |
| `cq_search_for_pattern` | Regex/literal search across files (ripgrep-powered) |
| `cq_list_dir` | List directory contents with metadata |
| `cq_create_text_file` | Create or overwrite a text file |
| `cq_replace_content` | Surgical find-and-replace within a file |
| `cq_diff_summary` | Show staged + unstaged git diff summary (`format=widget` for UI) |
| `cq_execute` | Execute shell commands (gated by permission hook) |
| `cq_search_commits` | Search git commit history by message or content |

---

## Code Intelligence / LSP (Extended)

> Python/JavaScript/TypeScript only. Go/Rust → use `cq_search_for_pattern` instead.

| Tool | Description |
|------|-------------|
| `cq_find_symbol` | Find symbol definition by name (Jedi/multilspy) |
| `cq_get_symbols_overview` | Get module/class/function overview |
| `cq_replace_symbol_body` | Replace function/class body |
| `cq_insert_before_symbol` | Insert code before a symbol |
| `cq_insert_after_symbol` | Insert code after a symbol |
| `cq_rename_symbol` | Rename a symbol across files |
| `cq_find_referencing_symbols` | Find all references to a symbol |
| `cq_parse_document` | Parse a structured document (PDF, DOCX, HTML) |
| `cq_extract_text` | Extract plain text from a document |
| `cq_onboard` | Interactive onboarding for new CQ users |

---

## Validation & Testing (Core)

| Tool | Description |
|------|-------------|
| `cq_run_validation` | Run lint + tests (auto-detects: go test, pytest, cargo test, pnpm test). Severity: CRITICAL/HIGH/MEDIUM/LOW (`format=widget` for UI) |
| `cq_run_checkpoint` | Trigger manual checkpoint review |
| `cq_run_complete` | Mark a run as complete with results |
| `cq_run_should_continue` | Check if the current run should continue (loop guard) |
| `cq_error_trace` | Format and display error trace for debugging (`format=widget` for UI) |

---

## Config & Health (Core)

| Tool | Description |
|------|-------------|
| `cq_config_get` | Read config values from `.c4/config.yaml` |
| `cq_config_set` | Write config values to `.c4/config.yaml` |
| `cq_health` | Check health of all CQ components (Doctor equivalent) |
| `cq_whoami` | Show current user identity and connected project |
| `cq_gpu_status` | Show GPU availability and utilization on connected workers |

---

## Specs & Designs (Extended)

Tools for structured specification and architecture decision records.

| Tool | Description |
|------|-------------|
| `cq_get_spec` | Read a spec document from `docs/specs/` |
| `cq_save_spec` | Write or update a spec document |
| `cq_list_specs` | List all specs with metadata |
| `cq_get_design` | Read an architecture design document |
| `cq_save_design` | Write or update a design document |
| `cq_list_designs` | List all design documents |
| `cq_discovery_complete` | Mark discovery phase complete, transition to design |
| `cq_design_complete` | Mark design phase complete, transition to planning |

---

## Lighthouse & Checkpoints (Extended)

| Tool | Description |
|------|-------------|
| `cq_lighthouse` | Check if implementation matches Lighthouse contracts |
| `cq_checkpoint` | Trigger manual checkpoint review |
| `cq_artifact_save` | Save build artifact with content hash |
| `cq_artifact_get` | Load a previously saved artifact |
| `cq_artifact_list` | List all saved artifacts with metadata |
| `cq_snapshot` | Take a manual context snapshot |

---

## Knowledge (Extended)

> Requires `connected` or `full` tier for cloud sync. FTS5 + pgvector (1536-dim OpenAI) + 3-way RRF.

| Tool | Description |
|------|-------------|
| `cq_knowledge_record` | Store knowledge (decisions, patterns, insights, errors, discoveries). AI self-capture: tool description triggers autonomous saves. |
| `cq_knowledge_search` | Search knowledge: vector + FTS + ilike 3-stage fallback. Returns ranked results. |
| `cq_knowledge_distill` | Auto-distill knowledge when doc count ≥ 5 (cluster + summarize) |
| `cq_knowledge_ingest` | Batch ingest files or URLs into knowledge store |
| `cq_knowledge_ingest_paper` | Ingest a research paper with structured metadata |
| `cq_knowledge_publish` | Publish knowledge to shared project namespace |
| `cq_knowledge_pull` | Pull published knowledge from another project |
| `cq_knowledge_get` | Get a specific knowledge document by ID |
| `cq_knowledge_delete` | Delete a knowledge document |
| `cq_knowledge_stats` | Show knowledge store stats (count, last sync, coverage) |
| `cq_knowledge_reindex` | Rebuild the knowledge search index |
| `cq_knowledge_discover` | Discover related knowledge from an input concept |
| `cq_pattern_suggest` | Suggest implementation patterns from knowledge base |
| `cq_recall` | Recall contextually relevant memories for the current task |

---

## Session Intelligence (Extended)

| Tool | Description |
|------|-------------|
| `cq_analyze_history` | Analyze conversation history for patterns and insights |
| `cq_reflect` | Generate structured reflection on current session or task |
| `cq_pop_status` | Show current POP (Point of Progress) status |
| `cq_pop_extract` | Extract key decisions and context from current session |
| `cq_pop_reflect` | Reflect on progress and update session state |
| `cq_profile_load` | Load a saved session profile |
| `cq_profile_save` | Save current session context as a named profile |

---

## Drive (Extended)

> Cloud file storage with TUS resumable upload. Requires `connected` or `full` tier.

| Tool | Description |
|------|-------------|
| `cq_drive_upload` | Upload a file to Drive (TUS resumable, content-addressable) |
| `cq_drive_download` | Download a file from Drive (Range-based resume) |
| `cq_drive_list` | List files in Drive with metadata |
| `cq_drive_delete` | Delete a file from Drive |
| `cq_drive_info` | Get file metadata (size, hash, URLs) |
| `cq_drive_mkdir` | Create a directory in Drive |
| `cq_drive_dataset_list` | List datasets stored in Drive |
| `cq_drive_dataset_pull` | Pull a dataset from Drive to local storage |
| `cq_drive_dataset_upload` | Upload a dataset to Drive with versioning |

---

## Relay (Extended)

| Tool | Description |
|------|-------------|
| `cq_relay_call` | Call an MCP tool on a remote worker via relay |
| `cq_nodes_map` | Visual map of all connected nodes (`format=widget` for UI) |

---

## LLM Gateway (Extended)

> Requires `llm_gateway` build tag. Multi-provider with prompt caching.

| Tool | Description |
|------|-------------|
| `cq_llm_call` | Call LLM with provider routing (Anthropic/OpenAI/Gemini/Ollama) |
| `cq_llm_providers` | List configured LLM providers and their availability |
| `cq_llm_costs` | Show LLM cost breakdown and cache savings (`format=widget` for UI) |
| `cq_llm_usage_stats` | Detailed token usage stats per session/project |

---

## Hub — Distributed Jobs (Conditional, Hub tier)

> Requires Hub connection (`serve.hub.enabled: true` + cloud credentials).

### Job Operations

| Tool | Description |
|------|-------------|
| `cq_job_submit` | Submit a job to the Hub queue with spec and routing |
| `cq_job_status` | Get job status and progress (`format=widget` for UI) |
| `cq_job_summary` | Get job results and output artifacts (`format=widget` for UI) |
| `cq_job_cancel` | Cancel a running or pending job |
| `cq_job_list` | List recent jobs with status and metrics |
| `cq_hub_submit` | Low-level Hub job submission with full spec control |
| `cq_hub_status` | Get Hub connection and queue status |
| `cq_hub_list` | List all jobs in Hub queue |
| `cq_hub_cancel` | Cancel a Hub job by ID |
| `cq_hub_retry` | Retry a failed Hub job |
| `cq_hub_wait` | Wait for a Hub job to complete (blocking) |
| `cq_hub_watch` | Watch Hub job progress in real-time |
| `cq_hub_estimate` | Estimate resource requirements for a job spec |
| `cq_hub_summary` | Get summary statistics for Hub queue |
| `cq_hub_download` | Download output artifacts from a completed job |
| `cq_hub_upload` | Upload input artifacts to Hub storage |

### Worker Operations

| Tool | Description |
|------|-------------|
| `cq_hub_workers` | List workers with affinity scores and current load |
| `cq_hub_workers_unified` | Unified view of all workers across Hub regions |
| `cq_hub_stats` | Detailed Hub infrastructure statistics |
| `cq_hub_lease_renew` | Renew a Hub job lease to prevent timeout |

### Hub Metrics

| Tool | Description |
|------|-------------|
| `cq_hub_log_metrics` | Log metrics from a running Hub job (stdout parser) |
| `cq_hub_metrics` | Query collected metrics for Hub jobs |

### DAG Pipelines

| Tool | Description |
|------|-------------|
| `cq_hub_dag_create` | Create a DAG pipeline with nodes and edges |
| `cq_hub_dag_add_node` | Add a node to an existing DAG |
| `cq_hub_dag_add_dep` | Add a dependency edge between DAG nodes |
| `cq_hub_dag_execute` | Execute a DAG pipeline |
| `cq_hub_dag_from_yaml` | Create a DAG from a YAML definition |
| `cq_hub_dag_status` | Get DAG execution status with per-node progress |
| `cq_hub_dag_list` | List all DAGs |

### Cron Scheduling

| Tool | Description |
|------|-------------|
| `cq_cron_create` | Register a cron schedule with job spec |
| `cq_cron_list` | List cron schedules with last-run times |
| `cq_cron_delete` | Delete a cron schedule |

### Worker Lifecycle

| Tool | Description |
|------|-------------|
| `cq_worker_standby` | Enter standby mode — wait for and execute Hub jobs |
| `cq_worker_complete` | Signal job completion from standby worker |
| `cq_worker_shutdown` | Graceful worker shutdown with task handoff |
| `cq_ensure_supervisor` | Ensure the worker supervisor process is running |

---

## Experiments (Extended)

| Tool | Description |
|------|-------------|
| `cq_experiment_record` | Record experiment result (exp_id, metrics, config) |
| `cq_experiment_search` | Search experiments by metric, date, or tags (`format=widget` for UI) |
| `cq_experiment_register` | Register a new experiment definition with metadata |

---

## Persona & Growth (Extended)

| Tool | Description |
|------|-------------|
| `cq_soul_get` | Get the current soul/judgment criteria |
| `cq_soul_set` | Update soul/judgment criteria |
| `cq_soul_resolve` | Resolve a conflict between soul criteria |
| `cq_persona_learn` | Record a learned behavior pattern |
| `cq_persona_learn_from_diff` | Auto-extract patterns from a git diff |
| `cq_persona_evolve` | Evolve persona based on accumulated patterns |
| `cq_persona_stats` | Show current persona profile and learned patterns |
| `cq_rule_add` | Add a routing or behavior rule |
| `cq_rule_list` | List all active rules |
| `cq_rule_remove` | Remove a rule by ID |
| `cq_rule_toggle` | Enable or disable a rule without deleting it |
| `cq_intelligence_stats` | Show collective intelligence statistics across users |

---

## CDP (Chrome DevTools Protocol, Extended)

| Tool | Description |
|------|-------------|
| `cq_cdp_run` | Execute JavaScript in a browser tab via CDP |
| `cq_cdp_action` | Perform a browser action (click, type, navigate) via CDP |
| `cq_cdp_list` | List available CDP browser targets |
| `cq_webmcp_discover` | Auto-discover Web MCP endpoints on a page |
| `cq_webmcp_call` | Call a discovered Web MCP tool |
| `cq_webmcp_context` | Get context from a Web MCP-enabled page |
| `cq_web_fetch` | Fetch URL with content negotiation + HTML→Markdown conversion |

---

## Notifications & EventBus (Extended)

| Tool | Description |
|------|-------------|
| `cq_notify` | Send notification via configured channels (Telegram/Dooray) |
| `cq_notification_channels` | List configured notification channels |
| `cq_notification_get` | Get notification settings for a channel |
| `cq_notification_set` | Configure notification settings for a channel |
| `cq_event_publish` | Publish an event to EventBus |
| `cq_event_list` | List recent EventBus events with filter |
| `cq_record_gate` | Record a gate decision event |

---

## Mail (Extended)

| Tool | Description |
|------|-------------|
| `cq_mail_send` | Send inter-session mail to a named session |
| `cq_mail_ls` | List messages in the mail inbox |
| `cq_mail_read` | Read a specific mail message |
| `cq_mail_rm` | Delete a mail message from the inbox |

---

## Workspace (Extended)

| Tool | Description |
|------|-------------|
| `cq_workspace_create` | Create a new named workspace with isolated state |
| `cq_workspace_load` | Load a saved workspace context |
| `cq_workspace_save` | Save current workspace state |
| `cq_worktree_status` | Show git worktree status for all active branches |
| `cq_worktree_cleanup` | Clean up merged or stale worktrees |

---

## Secrets (Extended)

| Tool | Description |
|------|-------------|
| `cq_secret_set` | Store a secret in the encrypted secret store |
| `cq_secret_get` | Retrieve a secret by key |
| `cq_secret_list` | List all secret keys (values not shown) |
| `cq_secret_delete` | Delete a secret by key |

---

## Collective & Sync (Extended)

| Tool | Description |
|------|-------------|
| `cq_collective_stats` | Show collective intelligence statistics |
| `cq_collective_sync` | Sync local knowledge to the collective |

---

## Skill Evaluation (Extended)

| Tool | Description |
|------|-------------|
| `cq_skill_eval_run` | Run k-trial haiku classification on a skill's test cases |
| `cq_skill_eval_generate` | Generate positive + negative test cases for a skill |
| `cq_skill_eval_status` | Show trigger accuracy for all evaluated skills (threshold: 0.90) |
| `cq_skill_optimize` | Optimize a skill based on eval results |

---

## Research Loop (Extended)

| Tool | Description |
|------|-------------|
| `cq_research_loop_start` | Start autonomous research loop (LoopOrchestrator) |
| `cq_research_status` | Show current loop state, iteration count, best metric |
| `cq_research_loop_stop` | Stop the research loop gracefully |
| `cq_research_next` | Advance the research loop to the next iteration |
| `cq_research_record` | Record a research finding or intermediate result |
| `cq_research_start` | Start a single research task |
| `cq_research_approve` | Approve a research result to advance the loop |
| `cq_research_suggest` | Get suggestions for the next research direction |

---

## Observability — C7 Observe (Conditional, `c7_observe` build tag)

| Tool | Description |
|------|-------------|
| `cq_observe_metrics` | Query collected metrics (tool calls, latency, error rates) |
| `cq_observe_logs` | Query structured logs with filter |
| `cq_observe_traces` | Show distributed traces for requests |
| `cq_observe_trace_stats` | Show trace statistics and latency percentiles |
| `cq_observe_health` | Show observability pipeline health |
| `cq_observe_config` | Configure observability collection settings |
| `cq_observe_policy` | Set data retention and sampling policies |

---

## Access Control — C6 Guard (Conditional, `c6_guard` build tag)

| Tool | Description |
|------|-------------|
| `cq_guard_check` | Check if an action is permitted by current policies |
| `cq_guard_audit` | Show audit log of guard decisions |
| `cq_guard_policy_list` | List all active policies |
| `cq_guard_policy_set` | Add or update an RBAC policy rule |
| `cq_guard_role_assign` | Assign a role to a user or agent |

---

## External Connectors — C8 Gate (Conditional, `c8_gate` build tag)

| Tool | Description |
|------|-------------|
| `cq_gate_webhook_register` | Register a webhook for outbound notifications |
| `cq_gate_webhook_list` | List registered webhooks |
| `cq_gate_webhook_test` | Test a webhook endpoint |
| `cq_gate_schedule_add` | Schedule a remote action with cron expression |
| `cq_gate_schedule_list` | List all scheduled gate actions |
| `cq_gate_connector_status` | Get status of all configured external connectors |

---

## Legacy / Compatibility (c4_ prefix)

> These tools retain the `c4_` prefix for backward compatibility with existing agent workflows.

| Tool | Description |
|------|-------------|
| `c4_get_task` | Alias for `cq_get_task` (Worker mode task pickup) |
| `c4_health` | Alias for `cq_health` |
| `c4_knowledge_ingest_paper` | Alias for `cq_knowledge_ingest_paper` |
| `c4_research_suggest` | Alias for `cq_research_suggest` |
| `c4_status` | Alias for `cq_status` |
| `c4_test` | Run C4 self-tests |
| `c4_test_tool` | Test a specific MCP tool invocation |

---

## MCP Apps (Widget System)

When calling tools with `format=widget`, the response includes `_meta.ui.resourceUri`. MCP clients (Claude Code, Cursor, VS Code) fetch HTML via `resources/read` and render it in a sandboxed iframe.

| Widget URI | Tool | Description |
|-----------|------|-------------|
| `ui://cq/dashboard` | `cq_dashboard` | Project status summary |
| `ui://cq/job-progress` | `cq_job_status` | Job progress bar |
| `ui://cq/job-result` | `cq_job_summary` | Job results |
| `ui://cq/experiment-compare` | `cq_experiment_search` | Experiment comparison |
| `ui://cq/task-graph` | `cq_task_graph` | Task dependency graph |
| `ui://cq/nodes-map` | `cq_nodes_map` | Connected nodes map |
| `ui://cq/cost-tracker` | `cq_llm_costs` | LLM cost tracker |
| `ui://cq/test-results` | `cq_run_validation` | Test results |
| `ui://cq/git-diff` | `cq_diff_summary` | Git diff viewer |
| `ui://cq/error-trace` | `cq_error_trace` | Error trace viewer |

---

## Tool Count Summary

| Category | Count | Notes |
|----------|-------|-------|
| Project & State | 20 | Core |
| File Operations | 10 | Core |
| Code Intelligence / LSP | 10 | Extended, Python/JS/TS |
| Validation & Testing | 5 | Core |
| Config & Health | 5 | Core |
| Specs & Designs | 8 | Extended |
| Lighthouse & Checkpoints | 6 | Extended |
| Knowledge | 14 | Extended, cloud for sync |
| Session Intelligence | 7 | Extended |
| Drive | 9 | Extended, cloud |
| Relay | 2 | Extended |
| LLM Gateway | 4 | Extended |
| Hub Jobs | 16 | Conditional (Hub tier) |
| Hub Worker Operations | 4 | Conditional (Hub tier) |
| Hub Metrics | 2 | Conditional (Hub tier) |
| Hub DAG Pipelines | 7 | Conditional (Hub tier) |
| Cron Scheduling | 3 | Conditional (Hub tier) |
| Worker Lifecycle | 4 | Conditional (Hub tier) |
| Experiments | 3 | Extended |
| Persona & Growth | 11 | Extended |
| CDP & WebMCP | 7 | Extended |
| Notifications & EventBus | 7 | Extended |
| Mail | 4 | Extended |
| Workspace | 5 | Extended |
| Secrets | 4 | Extended |
| Collective & Sync | 2 | Extended |
| Skill Evaluation | 4 | Extended |
| Research Loop | 8 | Extended |
| C7 Observe | 7 | Conditional (build tag) |
| C6 Guard | 5 | Conditional (build tag) |
| C8 Gate | 6 | Conditional (build tag) |
| Legacy (c4_ prefix) | 7 | Compatibility aliases |
| **Total** | **217** | — |
