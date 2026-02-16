---
description: |
  Quick reference for C4 skills, agents, and tools. Provides summaries,
  decision trees, and keyword search across 19 skills, 37 agents (9 categories),
  and 133 MCP tools (107 base + 26 hub). Use when the user needs help, reference,
  skill list, or wants to search C4 capabilities. Triggers: "도움말",
  "명령어 목록", "도구 검색", "help", "list commands", "show agents",
  "what tools", "how to".
---

# C4 Help

Quick reference for commands, agents, and tools. Parse `$ARGUMENTS` and branch accordingly.

## Usage

```
/c4-help              → Full summary
/c4-help commands     → All 19 skills
/c4-help agents       → 37 agents by category
/c4-help tools        → 133 MCP tools (107 base + 26 hub)
/c4-help <keyword>    → Keyword search
```

## Instructions

Parse `$ARGUMENTS` for branch. No MCP calls needed (static output).

---

### Branch 1: No args (`$ARGUMENTS` empty) → Full summary

```
## C4 Quick Reference

### Decision Tree

What's the task?
├─ 1-line fix → Just fix it (no C4)
├─ Small (1-5 files) → /c4-quick "desc" → /c4-submit
├─ Medium (5-15 files) → /c4-add-task → /c4-run  OR  c4_claim → c4_report
├─ Large (15+ files) → /c4-plan → /c4-run N  OR  /c4-swarm N
├─ Research/experiment → /c4-research
└─ Document work → /c4-review (paper) OR c4_parse_document

### Core Commands (Daily)

| Command | Purpose | Args |
|---------|---------|------|
| /c4-status | Check status | (none) |
| /c4-quick | Start task now | "desc" |
| /c4-run | Parallel workers | [N] [--continuous] |
| /c4-submit | Submit completion | [task-id] |
| /c4-validate | Run validation | (none) |

### More Info

- /c4-help commands  → All 19 skills
- /c4-help agents    → 37 agents by category
- /c4-help tools     → 133 MCP tools
- /c4-help <keyword> → Keyword search (e.g., /c4-help review)
```

---

### Branch 2: `$ARGUMENTS` = "commands" → All commands

```
## C4 Commands (16)

### Daily (6)

| Command | Purpose | Args Format | Example |
|---------|---------|-------------|---------|
| /c4-status | Check status | (none) | /c4-status |
| /c4-quick | Start task now | "desc" [scope=path] | /c4-quick "fix: timeout bug" |
| /c4-run | Parallel workers | [N] [--continuous] [--max N] | /c4-run 3 |
| /c4-submit | Submit completion | [task-id] | /c4-submit T-001 |
| /c4-validate | Run validation | (none) | /c4-validate |
| c4_claim/report | Direct mode | task_id (MCP tool) | c4_claim("T-001-0") |

### Weekly (5)

| Command | Purpose | Args Format | Example |
|---------|---------|-------------|---------|
| /c4-plan | Large planning | (none) | /c4-plan |
| /c4-add-task | Add task manually | "desc" [--domain D] | /c4-add-task "JWT auth" |
| /c4-checkpoint | Step-by-step review | (none) | /c4-checkpoint |
| /c4-swarm | Team collaboration | [N] [--review] [--investigate] | /c4-swarm --review |
| /c4-stop | Stop execution | (none) | /c4-stop |

### Occasional (5)

| Command | Purpose | Args Format | Example |
|---------|---------|-------------|---------|
| /c4-interview | Deep interview | "topic" | /c4-interview "realtime sync" |
| /c4-release | Generate changelog | (none) | /c4-release |
| /c4-research | Research iteration | [start\|status\|next\|record\|approve] | /c4-research next |
| /c4-review | Paper review | <pdf_path> | /c4-review paper.pdf |
| /c4-init | Project init | (none) | /c4-init |

### Operations (1)

| Command | Purpose | Args Format | Warning |
|---------|---------|-------------|---------|
| /c4-clear | Full state reset | (none) | Irreversible |
```

---

### Branch 3: `$ARGUMENTS` = "agents" → Agents by category

```
## C4 Agents (37, 9 categories)

### Backend (3)
| Agent | Expertise |
|-------|-----------|
| backend-architect | REST API, microservices, DB schema |
| database-optimizer | Query tuning, indexes, caching |
| graphql-architect | GraphQL schema, N+1, subscriptions |

### Frontend (3)
| Agent | Expertise |
|-------|-----------|
| frontend-developer | React components, state, a11y (spec→code) |
| frontend-designer | Design→spec conversion, design systems |
| react-pro | React 19+, Server Components, optimization |

### DevOps/Infra (3)
| Agent | Expertise |
|-------|-----------|
| deployment-engineer | CI/CD pipelines, Docker, K8s |
| devops-troubleshooter | Production debugging, logs |
| cloud-architect | AWS/GCP/Azure infra, Terraform IaC, cost |

### Quality (4)
| Agent | Expertise |
|-------|-----------|
| code-reviewer | Read-only review, test coverage |
| code-refactorer | Code structure, refactoring |
| security-auditor | Vuln detection, auth/authz |
| test-automator | unit/integration/e2e, CI setup |

### Data/ML (4)
| Agent | Expertise |
|-------|-----------|
| data-engineer | ETL, data warehouse, Spark/Airflow/Kafka |
| data-scientist | SQL, BigQuery, data insights |
| ml-engineer | Model serving, features, A/B test |
| ai-engineer | LLM apps, RAG, vector search, agents |

### Languages (4)
| Agent | Expertise |
|-------|-----------|
| golang-pro | goroutine, channel, error, performance |
| python-pro | decorator, async, testing, performance |
| rust-pro | ownership, lifetime, trait, async, unsafe |
| tauri-developer | Tauri 2.x, Rust-JS bridge, desktop apps |

### Documentation (4)
| Agent | Expertise |
|-------|-----------|
| api-documenter | OpenAPI/Swagger, SDK gen, dev docs |
| content-writer | Tech blog, documentation |
| prd-writer | PRD writing, story breakdown, checkpoints |
| prompt-engineer | LLM prompt optimization, system prompts |

### Project (4)
| Agent | Expertise |
|-------|-----------|
| workflow-orchestrator | Dev lifecycle coordination, agent collab |
| project-task-planner | PRD→task list generation |
| context-manager | Multi-agent context management |
| memory-bank | Project knowledge store/search |

### Specialty (8)
| Agent | Expertise |
|-------|-----------|
| debugger | Dev bugs, test failures |
| incident-responder | Production incidents, postmortems |
| performance-engineer | Profiling, bottlenecks, caching |
| legacy-modernizer | Legacy refactoring, framework migration |
| mobile-developer | React Native/Flutter, native integration |
| dx-optimizer | Developer experience, tools/workflow |
| payment-integration | Stripe/PayPal, subscriptions/webhooks, PCI |
| math-teacher | Math proofs, rigorous solutions |

### Internal (3)
| Agent | Expertise |
|-------|-----------|
| architect-review | PRD/architecture review (/c4-plan internal) |
| c4-scout | Fast codebase exploration (500-token compression) |
| vibe-coding-coach | Vision→app, conversational app building |

### Common Confusions

| Pair | Distinction |
|------|-------------|
| debugger vs incident-responder | Dev bugs vs Production incidents |
| frontend-designer vs frontend-developer | Design→spec vs Spec→code |
| backend-architect vs cloud-architect | API/DB design vs Infra/IaC |
| code-reviewer vs code-refactorer | Read-only review vs Code changes |
| deployment-engineer vs devops-troubleshooter | CI/CD build vs Incident debug |
```

---

### Branch 4: `$ARGUMENTS` = "tools" → MCP tools (3 layers)

```
## C4 MCP Tools (133, 107 base + 26 hub)

### Layer 1: Daily tools (6)
Direct use or via commands.

| Tool | Purpose |
|------|---------|
| /c4-status | Check project status |
| /c4-quick "desc" | Start task now |
| /c4-run [N] | Parallel workers |
| /c4-submit | Submit completion |
| /c4-validate | lint + test |
| c4_claim / c4_report | Direct mode task |

### Layer 2: Weekly/situational tools (16)
Direct calls for specific scenarios.

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

### Layer 3: Internal tools (80+)
Auto-used by agents/workers. Rarely need direct calls.

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
```

---

### Branch 5: Other `$ARGUMENTS` → Keyword search

Search the keyword in the data below and output matching items.

**Search data**:

Commands:
- c4-status: check status, current state
- c4-quick: start now, quick start, small tasks
- c4-run: parallel workers, independent tasks
- c4-submit: submit completion, task done
- c4-validate: validation, lint, test
- c4-plan: large planning, design, Discovery
- c4-add-task: add task, manual creation
- c4-checkpoint: review, inspection, step-by-step
- c4-swarm: team collab, parallel, review, investigate
- c4-stop: stop, pause, HALTED
- c4-interview: interview, requirements exploration
- c4-release: release, changelog
- c4-research: research, paper, experiment, iteration
- c4-review: paper review, academic, 6-axis, PDF
- c4-init: initialize, project start
- c4-clear: reset, state delete, wipe

Agents:
- backend-architect: API, DB, microservices, REST, schema
- frontend-developer: React, components, state, UI, accessibility
- frontend-designer: design, mockup, wireframe, design system
- golang-pro: Go, goroutine, channel, concurrency
- python-pro: Python, decorator, async, performance
- rust-pro: Rust, ownership, lifetime, trait
- debugger: bug, debugging, test failure, error
- incident-responder: production incident, emergency, downtime
- code-reviewer: code review, quality, security
- code-refactorer: refactoring, structure, deduplication
- security-auditor: security, vulnerability, auth, OWASP
- test-automator: testing, CI, automation, coverage
- deployment-engineer: CI/CD, Docker, K8s, deploy
- devops-troubleshooter: incident, logs, monitoring
- cloud-architect: AWS, GCP, Azure, Terraform, infrastructure
- database-optimizer: query, index, caching, DB performance
- ml-engineer: ML, model, feature, serving
- ai-engineer: LLM, RAG, vector, prompt
- data-engineer: ETL, pipeline, Spark, Kafka
- data-scientist: data analysis, SQL, BigQuery
- performance-engineer: performance, profiling, bottleneck, caching
- mobile-developer: React Native, Flutter, app
- tauri-developer: Tauri, desktop, Rust-JS bridge

Tools:
- c4_knowledge_record: knowledge recording, insight, pattern
- c4_knowledge_search: knowledge search, past cases
- c4_reflect: reflection, pattern analysis, retrospective
- c4_hub_submit: GPU task, training, remote execution
- c4_hub_dag_from_yaml: pipeline, DAG, multi-step
- c4_lighthouse: API contract, stub, TDD, spec-first
- c4_llm_costs: cost, model, routing
- c4_parse_document: document, HWP, DOCX, PDF, PPTX
- c4_extract_text: text extraction
- c4_cdp_run: browser, automation, CDP

**Output format**:

```
## /c4-help "{keyword}" search results

### Related Commands
- /c4-xxx: description

### Related Agents
- agent-name: description

### Related Tools
- c4_xxx: description
```

If no matches:
```
No results for "{keyword}".
Use /c4-help to see the full list.
```
