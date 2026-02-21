# CQ for Gemini CLI Playbook

This directory defines the operational protocols for using Gemini CLI within the CQ project.

## Core Principles

1.  **Respect Project Conventions**: Strictly adhere to the project's structure, naming conventions, and architectural patterns (e.g., Python `c4` core, Go `c5` hybrid).
2.  **Safety First**: Always verify `c4_status` before taking action. Do not mix `Direct` and `Worker` protocols on the same task.
3.  **Context Awareness**: Use `read_file` and `search_file_content` extensively to understand the codebase before modifying it. Do not assume; verify.
4.  **Validation Mandatory**: Run relevant tests or `c4_validate` (if available) before reporting task completion.

## Operational Scenarios

### 1. Direct Task Execution (Interactive)
*   **Best for**: Complex refactoring, architectural changes, cross-module updates.
*   **Flow**:
    1.  Check status: `c4_status`
    2.  Claim task: `c4_claim <task_id>`
    3.  Implement changes (Read -> Plan -> Edit -> Verify)
    4.  Validate: Run tests / `c4_validate`
    5.  Commit changes (if requested)
    6.  Report completion: `c4_report <task_id>`
    7.  Verify status: `c4_status`

### 2. Worker Mode (Batch/Parallel)
*   **Best for**: Independent, well-scoped tasks, large-scale migrations.
*   **Flow**:
    1.  Get task: `c4_get_task`
    2.  Implement changes
    3.  Submit: `c4_submit <task_id>`

## Agent Configuration
Gemini CLI agents are defined in `.gemini/agents/`. These provide specialized instructions for different roles.

## Tools & Permissions
Gemini has access to standard shell commands and specific MCP tools defined in the project.
Ensure all `c4_*` commands are executed via `run_shell_command`.
