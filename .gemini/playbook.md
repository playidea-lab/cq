# Gemini Operational Playbook (SOP)

This playbook defines the **Standard Operating Procedures** for Gemini agents in the C4 project.
Follow these phases sequentially to ensure high-quality, safe, and verifiable work.

## Phase 1: Context & Assessment
**Goal**: Understand the current situation without making changes.

1.  **Check Status**: Run `cq status` (or `./.gemini/skills/c4-status.sh`).
2.  **Scan Environment**:
    - `ls -F`: Check directory structure.
    - `git status`: Check for uncommitted changes.
    - `read_file .gemini/memory/global_context.md`: Read shared project knowledge.
3.  **Identify Objective**: Clarify what needs to be done (Bugfix? Feature? Refactor?).

## Phase 2: Planning & Design
**Goal**: Define *what* to do before doing it.

1.  **Requirement Analysis**:
    - If ambiguous, ask questions (EARS pattern).
    - If complex, run `gemini activate_skill c4-plan` (simulated).
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

## Emergency Protocols
- **Build Break**: Revert changes (`git restore`) and re-read the error log.
- **Lost Context**: Read `.gemini/memory/scratchpad.md` or ask the user.
- **Tool Failure**: Check `.gemini/tools.md` for syntax.
