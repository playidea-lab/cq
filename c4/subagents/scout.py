"""Scout subagent - Lightweight code exploration."""

import os
from pathlib import Path
from typing import Any

from c4.docs.analyzer import CodeAnalyzer

from .base import SubagentBase


class Scout(SubagentBase):
    """Scout subagent for code exploration.

    Explores a task's scope and extracts:
    - File paths
    - Symbol signatures (functions, classes)
    - Compressed context (max 2000 tokens)

    Uses Haiku model for cost efficiency.
    """

    def __init__(self):
        """Initialize Scout subagent."""
        super().__init__(model="haiku")

    def validate_input(self, **kwargs: Any) -> tuple[bool, str | None]:
        """Validate input parameters.

        Args:
            **kwargs: Must contain 'task_id'

        Returns:
            Tuple of (is_valid, error_message)
        """
        task_id = kwargs.get("task_id")
        if not task_id:
            return False, "task_id is required"

        return True, None

    def execute(self, **kwargs: Any) -> dict[str, Any]:
        """Execute scout exploration.

        Args:
            **kwargs: Must contain:
                - task_id: Task ID to explore
                - daemon: Daemon instance (optional, for getting task)
                - scope: Scope path (optional, if not provided will fetch from task)

        Returns:
            Dictionary with:
                - task_id: Task ID
                - scope: Scope path
                - files: List of file info with symbols
                - token_count: Estimated tokens
                - truncated: Whether context was truncated
                - errors: List of errors encountered (if any)
        """
        # Validate input
        is_valid, error = self.validate_input(**kwargs)
        if not is_valid:
            return {"error": error}

        task_id = kwargs["task_id"]
        scope = kwargs.get("scope")

        # If scope not provided, get from daemon
        if not scope:
            daemon = kwargs.get("daemon")
            if not daemon:
                return {"error": "Either 'scope' or 'daemon' must be provided"}

            try:
                task = daemon.get_task(task_id)
                if not task:
                    return {"error": f"Task {task_id} not found"}

                scope = task.scope
                if not scope:
                    return {"error": f"Task {task_id} has no scope defined"}

            except Exception as e:
                return {"error": f"Failed to get task: {str(e)}"}

        # Explore scope
        return self._explore_scope(task_id, scope)

    def _explore_scope(self, task_id: str, scope: str) -> dict[str, Any]:
        """Explore scope and extract symbols.

        Args:
            task_id: Task ID
            scope: Scope path

        Returns:
            Context dictionary
        """
        try:
            project_root = Path(os.environ.get("C4_PROJECT_ROOT", Path.cwd()))
            scope_path = project_root / scope

            if not scope_path.exists():
                return {"error": f"Scope path does not exist: {scope}"}

            # Collect files to explore
            files_to_scan = []
            if scope_path.is_file():
                files_to_scan.append(scope_path)
            elif scope_path.is_dir():
                # Recursively find Python files
                files_to_scan.extend(scope_path.rglob("*.py"))

            # Extract symbols from each file
            context = {
                "task_id": task_id,
                "scope": scope,
                "files": [],
            }

            total_tokens = 0
            analyzer = CodeAnalyzer()

            for file_path in files_to_scan:
                file_info, tokens = self._extract_file_symbols(
                    file_path, project_root, total_tokens, analyzer
                )

                if file_info is None:
                    # Truncation reached
                    context["truncated"] = True
                    context["message"] = f"Context truncated at {self.max_tokens} tokens"
                    context["token_count"] = total_tokens
                    return context

                # Check for error
                if "error" in file_info:
                    context.setdefault("errors", []).append(file_info)
                elif file_info.get("symbols"):
                    context["files"].append(file_info)

                total_tokens += tokens

            context["token_count"] = total_tokens
            return context

        except Exception as e:
            return {"error": f"Failed to explore scope: {str(e)}"}

    def _extract_file_symbols(
        self,
        file_path: Path,
        project_root: Path,
        current_tokens: int,
        analyzer: CodeAnalyzer,
    ) -> tuple[dict[str, Any] | None, int]:
        """Extract symbols from a file.

        Args:
            file_path: Path to file
            project_root: Project root path
            current_tokens: Current token count
            analyzer: CodeAnalyzer instance

        Returns:
            Tuple of (file_info, tokens_added).
            file_info is None if truncation limit reached.
        """
        try:
            # Extract symbols (signatures only, no bodies)
            analyzer.add_file(str(file_path))
            symbols = analyzer.get_file_symbols(str(file_path))

            # Build compact representation
            file_info = {
                "path": str(file_path.relative_to(project_root)),
                "symbols": [],
            }

            tokens_added = 0

            for symbol in symbols:
                # Build signature
                sig = f"{symbol.kind.value} {symbol.name}"
                if symbol.signature:
                    sig += f" {symbol.signature}"

                estimated_tokens = self.estimate_tokens(sig)

                # Check if we exceed max tokens
                if current_tokens + tokens_added + estimated_tokens > self.max_tokens:
                    return None, tokens_added

                file_info["symbols"].append(
                    {
                        "kind": symbol.kind.value,
                        "name": symbol.name,
                        "signature": symbol.signature,
                    }
                )
                tokens_added += estimated_tokens

            return file_info, tokens_added

        except Exception as e:
            # Return error info
            error_info = {
                "path": str(file_path.relative_to(project_root)),
                "error": str(e),
            }
            return error_info, 0
