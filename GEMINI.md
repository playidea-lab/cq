# C4 Project Rules for Gemini (The Constitution)

> C4: AI Orchestration System - Automated Project Management from Planning to Completion

---

## 1. Core Development Principles

### 1.1 State Machine Authority
The State Machine is the single source of truth for the project's progress.
- **Never bypass state transitions**: Always use tools that trigger valid transitions.
- **Phase-Locked Actions**: Implementation (coding) is only allowed in the `EXECUTE` phase. Planning and Design must be completed first.
- **Pre-check**: Always call `c4_status()` at the beginning of a session to understand the current phase and "workflow.hint".

### 1.2 Task Integrity
- **Review-as-Task**: Every implementation task (`T-*`) must be followed by a review task (`R-*`).
- **Atomic Work**: One task per commit. Each task should have a clear Scope and a verifiable Definition of Done (DoD).
- **Quality Gates**: Linting (`ruff`) and Unit tests (`pytest`) are not optional. They must pass before `c4_submit()`.

---

## 2. Project State Machine Logic

| State | Allowed Commands | Next Step Hint |
|-------|------------------|----------------|
| **INIT** | `c4-init` | Initialize project structure |
| **DISCOVERY** | `c4-plan`, `c4-status` | Scan docs, collect EARS requirements |
| **DESIGN** | `c4-plan`, `c4-status` | Define architecture, ADRs, and components |
| **PLAN** | `c4-run`, `c4-plan` | Generate and confirm tasks |
| **EXECUTE** | `c4-status`, `c4-submit`, `c4-validate` | Implement tasks, run validations |
| **CHECKPOINT** | `c4-status`, `c4-checkpoint` | AI/Human review of the milestone |
| **HALTED** | `c4-run`, `c4-plan` | Investigate logs/repair queue and resume |

---

## 4. Operational Protocol (Mandatory)

### 4.1 Post-Task Rituals
Every task implementation MUST end with the following steps in order:
1.  **Verify**: Run `c4_run_validation()` to ensure no regressions.
2.  **Document**: Update relevant planning documents based on the task's nature:
    - **Engine/Logic Change**: Update `docs/PLAN.md` (Technical milestones).
    - **SaaS/Service Change**: Update root `PLAN.md` (Product roadmap).
    - **New Feature**: Update `docs/ROADMAP.md` (Feature checklist).
3.  **Commit**: Perform a Git commit with a structured message: `[C4] {task_id}: {description}`.

### 4.2 Document Hierarchy & Strategy
- **`GEMINI.md` (The Constitution)**: Primary rules for the agent. Follow this strictly.
- **`README.md` (The Face)**: Project overview and platform guides for users.
- **`PLAN.md` (Product/SaaS)**: Focus on service expansion (Auth, UI, Cloud).
- **`docs/PLAN.md` (Technical/Engine)**: Focus on internal architecture and engine milestones.
- **`docs/ROADMAP.md` (Vision)**: High-level feature status for all stakeholders.

---

## 5. Technical Implementation Rules

### 5.1 Git Workflow
- **Final Act**: A task is NOT complete until it is committed to Git.
- **Peer Review**: Implementation tasks (`T-*`) require a `c4_submit` which triggers a Review task (`R-*`).
- **Branching**: Always check if you are on the correct `c4/w-{task_id}` branch.

---

## 4. MCP Tools usage for Gemini

As a Gemini agent, you must use tools strategically:
1.  **Analyze**: Use `c4_status` to see what's next.
2.  **Plan**: If in `PLAN` phase, use `c4_add_todo` to structure the work.
3.  **Execute**: Use `c4_get_task` to assign work to yourself (as a worker).
4.  **Verify**: Run `c4_run_validation(["lint", "unit"])` before submitting.
5.  **Submit**: Call `c4_submit` with the commit SHA and validation results.

---

## 5. Security & Safety
- Never hardcode secrets. Use `llm.api_key_env` to reference environment variables.
- Respect the Bash security hook; avoid commands that modify system-wide configurations unless explicitly required.
- Maintain a local `.env.example` but never commit `.env`.
