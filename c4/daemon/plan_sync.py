"""Plan File Sync - Bidirectional sync between Claude Plan files and C4 tasks.

This module provides synchronization between Claude's plan files (~/.claude/plans/)
and C4's task queue, enabling:
- C4 → Plan: Task creation/completion updates plan file automatically
- Plan → C4: Plan file changes detected and synced to C4 tasks
"""

from __future__ import annotations

import re
from datetime import datetime
from pathlib import Path
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from ..models import Task


class PlanFileSync:
    """Claude Plan file ↔ C4 task bidirectional synchronization.

    Plan files are stored at ~/.claude/plans/c4-{project_id}.md
    with a standardized markdown format containing task checkboxes.

    Example plan file format:
        # C4 Project: my-project

        > Generated: 2025-01-15T10:30:00
        > Progress: 5/10 (50%)

        ## Tasks

        - [x] T-001-0: Implement feature A
        - [ ] T-002-0: Implement feature B
        - [ ] New task without ID
    """

    PLAN_DIR = Path.home() / ".claude" / "plans"

    # Regex patterns for parsing plan files
    # Matches: - [x] T-001-0: Title  or  - [ ] Title (without task ID)
    TASK_LINE_PATTERN = re.compile(
        r"^- \[(x| )\] (?:(T-[\w-]+): )?(.+)$", re.MULTILINE
    )

    def __init__(self, plan_dir: Path | None = None):
        """Initialize PlanFileSync.

        Args:
            plan_dir: Custom plan directory (default: ~/.claude/plans)
        """
        if plan_dir is not None:
            self.plan_dir = plan_dir
        else:
            self.plan_dir = self.PLAN_DIR

    def get_plan_file(self, project_id: str) -> Path:
        """Get the plan file path for a project.

        Args:
            project_id: The C4 project identifier

        Returns:
            Path to the plan file (~/.claude/plans/c4-{project_id}.md)
        """
        return self.plan_dir / f"c4-{project_id}.md"

    # =========================================================================
    # C4 → Plan (Event-driven updates)
    # =========================================================================

    def generate_plan_file(self, project_id: str, tasks: list[Task]) -> Path:
        """Generate a plan file from C4 tasks.

        Creates or overwrites the plan file with current task state.

        Args:
            project_id: The C4 project identifier
            tasks: List of Task objects from C4

        Returns:
            Path to the generated plan file
        """
        content = self._render_markdown(project_id, tasks)
        plan_file = self.get_plan_file(project_id)
        plan_file.parent.mkdir(parents=True, exist_ok=True)
        plan_file.write_text(content, encoding="utf-8")
        return plan_file

    def update_task_status(
        self, project_id: str, task_id: str, status: str
    ) -> bool:
        """Update a single task's checkbox in the plan file.

        Args:
            project_id: The C4 project identifier
            task_id: The task ID to update (e.g., "T-001-0")
            status: New status ("done" or "pending")

        Returns:
            True if the task was found and updated, False otherwise
        """
        plan_file = self.get_plan_file(project_id)
        if not plan_file.exists():
            return False

        content = plan_file.read_text(encoding="utf-8")
        original_content = content

        if status == "done":
            # Check the checkbox: - [ ] T-001-0: → - [x] T-001-0:
            pattern = rf"- \[ \] ({re.escape(task_id)}:)"
            replacement = r"- [x] \1"
        else:
            # Uncheck the checkbox: - [x] T-001-0: → - [ ] T-001-0:
            pattern = rf"- \[x\] ({re.escape(task_id)}:)"
            replacement = r"- [ ] \1"

        content = re.sub(pattern, replacement, content)

        if content != original_content:
            plan_file.write_text(content, encoding="utf-8")
            return True

        return False

    # =========================================================================
    # Plan → C4 (Query-time sync)
    # =========================================================================

    def sync_from_plan_file(
        self, project_id: str, c4_tasks: dict[str, Task]
    ) -> dict[str, list[dict[str, Any]]]:
        """Sync changes from plan file back to C4.

        Compares plan file state with C4 tasks and returns required updates.

        Args:
            project_id: The C4 project identifier
            c4_tasks: Dictionary of C4 tasks keyed by task_id

        Returns:
            Dictionary with:
                - "new_tasks": List of new tasks found only in plan file
                - "status_updates": List of status changes needed in C4
                - "plan_updates": List of updates needed in plan file
        """
        result: dict[str, list[dict[str, Any]]] = {
            "new_tasks": [],
            "status_updates": [],
            "plan_updates": [],
        }

        plan_file = self.get_plan_file(project_id)
        if not plan_file.exists():
            return result

        content = plan_file.read_text(encoding="utf-8")

        for match in self.TASK_LINE_PATTERN.finditer(content):
            checked = match.group(1) == "x"
            task_id = match.group(2)  # May be None for new tasks
            title = match.group(3).strip()

            if task_id:
                # Existing task - compare status
                c4_task = c4_tasks.get(task_id)
                if c4_task:
                    c4_done = c4_task.status.value == "done"

                    if checked and not c4_done:
                        # Plan: done, C4: pending → Update C4
                        result["status_updates"].append({
                            "task_id": task_id,
                            "new_status": "done",
                            "source": "plan",
                        })
                    elif not checked and c4_done:
                        # Plan: pending, C4: done → Update plan file
                        result["plan_updates"].append({
                            "task_id": task_id,
                            "new_status": "done",
                        })
            else:
                # New task (no task_id) - needs to be added to C4
                result["new_tasks"].append({
                    "title": title,
                    "status": "done" if checked else "pending",
                })

        return result

    def has_plan_file(self, project_id: str) -> bool:
        """Check if a plan file exists for the project.

        Args:
            project_id: The C4 project identifier

        Returns:
            True if the plan file exists
        """
        return self.get_plan_file(project_id).exists()

    # =========================================================================
    # Helper Methods
    # =========================================================================

    def _render_markdown(self, project_id: str, tasks: list[Task]) -> str:
        """Render task list to markdown format.

        Args:
            project_id: The C4 project identifier
            tasks: List of Task objects

        Returns:
            Formatted markdown string
        """
        done = sum(1 for t in tasks if t.status.value == "done")
        total = len(tasks)
        progress = int((done / total) * 100) if total > 0 else 0

        lines = [
            f"# C4 Project: {project_id}",
            "",
            f"> Generated: {datetime.now().isoformat()}",
            f"> Progress: {done}/{total} ({progress}%)",
            "",
            "## Tasks",
            "",
        ]

        # Group tasks by type/phase if possible
        for task in tasks:
            checkbox = "[x]" if task.status.value == "done" else "[ ]"
            lines.append(f"- {checkbox} {task.id}: {task.title}")

        # Add footer
        lines.extend([
            "",
            "---",
            "*Synced with C4 Task Queue*",
        ])

        return "\n".join(lines)

    def delete_plan_file(self, project_id: str) -> bool:
        """Delete the plan file for a project.

        Args:
            project_id: The C4 project identifier

        Returns:
            True if the file was deleted, False if it didn't exist
        """
        plan_file = self.get_plan_file(project_id)
        if plan_file.exists():
            plan_file.unlink()
            return True
        return False
