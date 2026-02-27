# Gemini Operational Playbook (SOP)

This playbook defines the **Standard Operating Procedures** for Gemini agents in the CQ project.
Follow these phases sequentially to ensure high-quality, safe, and verifiable work.

## Phase 1: Context & Assessment
**Goal**: Understand the current situation without making changes.

1.  **Check Status**: Run `cq status` (or `./.gemini/skills/c4-status.sh`).
2.  **Scan Environment**:
    - **Discovery**: Run `activate_skill c4-scout` to explore the codebase with high efficiency.
    - `ls -F`: Check directory structure.
    - `git status`: Check for uncommitted changes.
    - `read_file .gemini/memory/global_context.md`: Read shared project knowledge.
3.  **Identify Objective**: Clarify what needs to be done (Bugfix? Feature? Refactor?).

## Phase 2: Planning & Design
**Goal**: Define *what* to do before doing it.

1.  **Requirement Analysis**:
    - If ambiguous, ask questions (EARS pattern).
    - **Expert Consultation**: Use `activate_skill <agent>` (e.g., `backend-architect`, `ai-engineer`) to get domain-specific guidance.
2.  **Task Breakdown**:
    - Split large goals into small, atomic tasks.
    - **Critical**: Ensure each task has a clear **Definition of Done (DoD)**.
3.  **Task Registration**:
    - Use `cq add-task --title "..." --scope "..."` for each task.
    - **Verify**: Run `cq status` to confirm tasks are created.

## Phase 3: Execution (The Loop)
**Goal**: Implement changes safely.

**For each active task:**
1.  **Claim**: (Implicitly claimed if you are the operator).
2.  **Read**: `read_file` relevant source code.
3.  **Test-First**: Create or update a test case that reproduces the issue or verifies the feature.
4.  **Implement**: Modify code using `replace` or `write_file`.
5.  **Verify**:
    - Run the specific test: `go test -run TestName` or `pytest -k test_name`.
    - Run static analysis: `golangci-lint` or `ruff`.

## Phase 4: Validation & Delivery
**Goal**: Ensure no regressions and deliver quality code.

1.  **Global Validation**: Run `./.gemini/skills/c4-validate.sh all`.
    - **Must Fix**: Security issues (CRITICAL) and Build errors.
2.  **Documentation**: Update `README.md` or `docs/` if architecture changed.
3.  **Commit**:
    - `git add .`
    - `git commit -m "type(scope): message"` (Follow Conventional Commits).
4.  **Report**: Summarize work done and next steps.

## Multi-Platform Operation (Parity)
Gemini agents must maintain parity with other CQ interfaces:
- **Doc Interpretation**: If a guide in `docs/` mentions a Claude-specific skill, map it to the corresponding agent in `.gemini/agents/`.
- **Consistency**: Ensure all tasks created via Gemini are visible and actionable in Claude/Codex (use standard `cq` CLI).
- **Tooling**: Prefer `cq` CLI commands over service-specific aliases to ensure cross-platform compatibility.

## Gemini 3.0 Research Operations (Exclusive)
Leverage Gemini 3.0's unique capabilities for c5 research projects:
1.  **Literature-Code Mapping**: Use `c4-research-scientist` to compare paper PDF math with Python/Go implementations.
2.  **Live Benchmark Grounding**: Always run a search-enabled agent to check latest SOTA results before starting a new experiment.
3.  **Holistic Log Analysis**: Feed long-duration experiment logs (up to 10M tokens) to `c4-global-brain` to find hidden correlations.
4.  **HITL (Human-In-the-Loop)**: Present summaries of large-context findings to the researcher for direction approval before executing c5 jobs.

## Emergency Protocols
- **Build Break**: Revert changes (`git restore`) and re-read the error log.
- **Lost Context**: Read `.gemini/memory/scratchpad.md` or ask the user.
- **Tool Failure**: Check `.gemini/tools.md` for syntax.
