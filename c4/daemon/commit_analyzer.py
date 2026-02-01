"""Commit Analyzer - Analyze git commits to update C4 tasks.

This module bridges Claude Code's independent work with C4 task tracking by:
1. Analyzing git commits to identify completed work
2. Matching changes to C4 tasks via scope or patterns
3. Updating C4 task status → triggers PlanFileSync for bidirectional sync
"""

from __future__ import annotations

import logging
import re
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..models import Task

logger = logging.getLogger(__name__)


@dataclass
class CommitInfo:
    """Information extracted from a git commit."""

    sha: str
    message: str
    files_added: list[str]
    files_modified: list[str]
    files_deleted: list[str]

    @property
    def all_files(self) -> list[str]:
        """All files changed in the commit."""
        return self.files_added + self.files_modified + self.files_deleted


@dataclass
class TaskMatch:
    """A matched task based on commit analysis."""

    task_id: str
    confidence: float  # 0.0 - 1.0
    reason: str  # Why this task was matched


class CommitAnalyzer:
    """Analyze git commits and match them to C4 tasks.

    This enables Claude Code work done outside C4 to be reflected
    in C4's task tracking via commit analysis.
    """

    # Patterns to extract task IDs from commit messages
    TASK_ID_PATTERNS = [
        re.compile(r"\b(T-\d{3,}-\d+)\b"),  # T-001-0
        re.compile(r"\b(R-\d{3,}-\d+)\b"),  # R-001-0 (review tasks)
        re.compile(r"\[(T-\d{3,}-\d+)\]"),  # [T-001-0]
        re.compile(r"#(T-\d{3,}-\d+)"),  # #T-001-0
    ]

    def __init__(self, project_root: Path):
        """Initialize CommitAnalyzer.

        Args:
            project_root: Root directory of the project (contains .git/)
        """
        self.root = project_root

    def _run_git(self, *args: str) -> subprocess.CompletedProcess[str]:
        """Run a git command in the project root."""
        return subprocess.run(
            ["git", *args],
            cwd=self.root,
            capture_output=True,
            text=True,
        )

    def get_commit_info(self, commit_sha: str) -> CommitInfo | None:
        """Get detailed information about a commit.

        Args:
            commit_sha: The commit SHA to analyze

        Returns:
            CommitInfo or None if commit not found
        """
        # Get commit message
        result = self._run_git("log", "-1", "--format=%B", commit_sha)
        if result.returncode != 0:
            logger.warning(f"Failed to get commit info for {commit_sha}")
            return None
        message = result.stdout.strip()

        # Get changed files with status
        result = self._run_git(
            "diff-tree", "--no-commit-id", "--name-status", "-r", commit_sha
        )
        if result.returncode != 0:
            logger.warning(f"Failed to get diff for {commit_sha}")
            return None

        files_added: list[str] = []
        files_modified: list[str] = []
        files_deleted: list[str] = []

        for line in result.stdout.strip().split("\n"):
            if not line:
                continue
            parts = line.split("\t")
            if len(parts) < 2:
                continue
            status, filepath = parts[0], parts[1]
            if status == "A":
                files_added.append(filepath)
            elif status == "M":
                files_modified.append(filepath)
            elif status == "D":
                files_deleted.append(filepath)

        return CommitInfo(
            sha=commit_sha,
            message=message,
            files_added=files_added,
            files_modified=files_modified,
            files_deleted=files_deleted,
        )

    def get_commits_since(self, since_sha: str | None = None) -> list[str]:
        """Get list of commit SHAs since a given commit.

        Args:
            since_sha: Starting commit (exclusive). If None, returns last commit.

        Returns:
            List of commit SHAs (newest first)
        """
        if since_sha:
            result = self._run_git(
                "log", "--format=%H", f"{since_sha}..HEAD"
            )
        else:
            result = self._run_git("log", "-1", "--format=%H")

        if result.returncode != 0:
            return []

        return [sha for sha in result.stdout.strip().split("\n") if sha]

    def extract_task_ids_from_message(self, message: str) -> list[str]:
        """Extract task IDs mentioned in a commit message.

        Args:
            message: The commit message

        Returns:
            List of task IDs found (e.g., ["T-001-0", "T-002-0"])
        """
        task_ids = set()
        for pattern in self.TASK_ID_PATTERNS:
            for match in pattern.finditer(message):
                task_ids.add(match.group(1))
        return list(task_ids)

    def match_commit_to_tasks(
        self, commit: CommitInfo, c4_tasks: dict[str, Task]
    ) -> list[TaskMatch]:
        """Match a commit to relevant C4 tasks.

        Uses multiple strategies:
        1. Task ID in commit message (highest confidence)
        2. File scope matching (medium confidence)
        3. Title/pattern matching (lower confidence)

        Args:
            commit: CommitInfo to analyze
            c4_tasks: Dictionary of C4 tasks keyed by task_id

        Returns:
            List of matched tasks with confidence scores
        """
        matches: list[TaskMatch] = []

        # Strategy 1: Task ID in commit message
        mentioned_ids = self.extract_task_ids_from_message(commit.message)
        for task_id in mentioned_ids:
            if task_id in c4_tasks:
                matches.append(TaskMatch(
                    task_id=task_id,
                    confidence=1.0,
                    reason="Task ID mentioned in commit message",
                ))

        # Strategy 2: File scope matching
        for task_id, task in c4_tasks.items():
            if task_id in mentioned_ids:
                continue  # Already matched

            if task.status.value == "done":
                continue  # Skip already done tasks

            if task.scope:
                # Check if any changed files match the task scope
                scope_parts = task.scope.split("/")
                for filepath in commit.all_files:
                    if self._path_matches_scope(filepath, scope_parts):
                        matches.append(TaskMatch(
                            task_id=task_id,
                            confidence=0.7,
                            reason=f"File '{filepath}' matches scope '{task.scope}'",
                        ))
                        break

        # Strategy 3: Keyword matching in commit message vs task title
        # (Lower confidence, requires more context)
        commit_words = set(commit.message.lower().split())
        for task_id, task in c4_tasks.items():
            if any(m.task_id == task_id for m in matches):
                continue  # Already matched

            if task.status.value == "done":
                continue

            title_words = set(task.title.lower().split())
            overlap = commit_words & title_words
            # Require at least 3 meaningful word overlap
            meaningful_overlap = {w for w in overlap if len(w) > 3}
            if len(meaningful_overlap) >= 3:
                matches.append(TaskMatch(
                    task_id=task_id,
                    confidence=0.4,
                    reason=f"Keywords overlap: {', '.join(meaningful_overlap)}",
                ))

        return matches

    def _path_matches_scope(self, filepath: str, scope_parts: list[str]) -> bool:
        """Check if a file path matches a scope.

        Args:
            filepath: The file path (e.g., "src/models/user.py")
            scope_parts: The scope split by "/" (e.g., ["src", "models"])

        Returns:
            True if the file is within the scope
        """
        file_parts = filepath.split("/")
        if len(file_parts) < len(scope_parts):
            return False
        return file_parts[:len(scope_parts)] == scope_parts

    def analyze_and_suggest(
        self,
        commit_sha: str,
        c4_tasks: dict[str, Task],
        min_confidence: float = 0.5,
    ) -> list[TaskMatch]:
        """Analyze a commit and suggest task status updates.

        Args:
            commit_sha: The commit SHA to analyze
            c4_tasks: Dictionary of C4 tasks
            min_confidence: Minimum confidence threshold for suggestions

        Returns:
            List of TaskMatch objects above the confidence threshold
        """
        commit = self.get_commit_info(commit_sha)
        if not commit:
            return []

        matches = self.match_commit_to_tasks(commit, c4_tasks)

        # Filter by confidence threshold
        return [m for m in matches if m.confidence >= min_confidence]

    def analyze_commits_batch(
        self,
        commit_shas: list[str],
        c4_tasks: dict[str, Task],
        min_confidence: float = 0.5,
    ) -> dict[str, list[TaskMatch]]:
        """Analyze multiple commits.

        Args:
            commit_shas: List of commit SHAs to analyze
            c4_tasks: Dictionary of C4 tasks
            min_confidence: Minimum confidence threshold

        Returns:
            Dictionary mapping commit SHA to list of matches
        """
        results = {}
        for sha in commit_shas:
            matches = self.analyze_and_suggest(sha, c4_tasks, min_confidence)
            if matches:
                results[sha] = matches
        return results
