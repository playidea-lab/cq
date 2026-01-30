"""File operation tools for C4 MCP server.

Provides file reading, writing, searching, and pattern matching capabilities
similar to Serena's tools, but scoped to the project directory.
"""

import fnmatch
import logging
import re
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)


class FileTools:
    """File operation tools for IDE integration.

    All operations are restricted to the project directory for security.
    """

    def __init__(self, project_root: Path):
        """Initialize with project root directory.

        Args:
            project_root: The root directory of the project.
                All file operations are restricted to this directory.
        """
        self.project_root = project_root.resolve()

    def _resolve_path(self, relative_path: str) -> Path:
        """Resolve a relative path to an absolute path within project.

        Args:
            relative_path: Path relative to project root, or "." for root.

        Returns:
            Resolved absolute path.

        Raises:
            ValueError: If the resolved path is outside project root.
        """
        if relative_path == ".":
            return self.project_root

        resolved = (self.project_root / relative_path).resolve()

        # Security check: ensure path is within project root
        try:
            resolved.relative_to(self.project_root)
        except ValueError:
            raise ValueError(
                f"Path '{relative_path}' resolves outside project root"
            )

        return resolved

    def read_file(
        self,
        relative_path: str,
        start_line: int = 0,
        end_line: int | None = None,
        max_chars: int = 100000,
    ) -> dict[str, Any]:
        """Read a file or a portion of it.

        Args:
            relative_path: Path relative to project root.
            start_line: 0-based index of first line to read.
            end_line: 0-based index of last line (inclusive), None for end of file.
            max_chars: Maximum characters to return.

        Returns:
            Dict with 'content', 'total_lines', 'start_line', 'end_line'.
        """
        try:
            path = self._resolve_path(relative_path)

            if not path.exists():
                return {"error": f"File not found: {relative_path}"}

            if not path.is_file():
                return {"error": f"Not a file: {relative_path}"}

            content = path.read_text(encoding="utf-8")
            lines = content.splitlines(keepends=True)
            total_lines = len(lines)

            # Apply line range
            if end_line is None:
                end_line = total_lines - 1
            end_line = min(end_line, total_lines - 1)
            start_line = max(0, start_line)

            selected_lines = lines[start_line : end_line + 1]
            result_content = "".join(selected_lines)

            # Apply max_chars limit
            if len(result_content) > max_chars:
                return {
                    "error": f"Content exceeds {max_chars} characters. "
                    f"Use start_line/end_line to read a smaller portion."
                }

            return {
                "content": result_content,
                "total_lines": total_lines,
                "start_line": start_line,
                "end_line": end_line,
            }

        except Exception as e:
            logger.error(f"Error reading file {relative_path}: {e}")
            return {"error": str(e)}

    def create_text_file(
        self,
        relative_path: str,
        content: str,
    ) -> dict[str, Any]:
        """Create or overwrite a text file.

        Args:
            relative_path: Path relative to project root.
            content: Content to write to the file.

        Returns:
            Dict with 'success' and 'message'.
        """
        try:
            path = self._resolve_path(relative_path)

            # Create parent directories if needed
            path.parent.mkdir(parents=True, exist_ok=True)

            path.write_text(content, encoding="utf-8")

            return {
                "success": True,
                "message": f"Created/updated: {relative_path}",
            }

        except Exception as e:
            logger.error(f"Error creating file {relative_path}: {e}")
            return {"success": False, "error": str(e)}

    def list_dir(
        self,
        relative_path: str = ".",
        recursive: bool = False,
        skip_ignored: bool = True,
    ) -> dict[str, Any]:
        """List files and directories.

        Args:
            relative_path: Path relative to project root.
            recursive: Whether to scan subdirectories.
            skip_ignored: Whether to skip .gitignore patterns.

        Returns:
            Dict with 'directories' and 'files' lists.
        """
        try:
            path = self._resolve_path(relative_path)

            if not path.exists():
                return {"error": f"Directory not found: {relative_path}"}

            if not path.is_dir():
                return {"error": f"Not a directory: {relative_path}"}

            # Patterns to skip
            skip_patterns = {
                ".git",
                ".c4",
                "__pycache__",
                "node_modules",
                ".venv",
                "venv",
                ".mypy_cache",
                ".pytest_cache",
                ".ruff_cache",
                "*.pyc",
                "*.pyo",
            }

            directories: list[str] = []
            files: list[str] = []

            def should_skip(name: str) -> bool:
                if not skip_ignored:
                    return False
                for pattern in skip_patterns:
                    if fnmatch.fnmatch(name, pattern):
                        return True
                return False

            def scan_dir(dir_path: Path, prefix: str = "") -> None:
                try:
                    for item in sorted(dir_path.iterdir()):
                        name = item.name
                        if should_skip(name):
                            continue

                        rel_name = f"{prefix}{name}" if prefix else name

                        if item.is_dir():
                            directories.append(rel_name + "/")
                            if recursive:
                                scan_dir(item, rel_name + "/")
                        else:
                            files.append(rel_name)
                except PermissionError:
                    pass

            scan_dir(path)

            return {
                "directories": directories,
                "files": files,
                "total": len(directories) + len(files),
            }

        except Exception as e:
            logger.error(f"Error listing directory {relative_path}: {e}")
            return {"error": str(e)}

    def find_file(
        self,
        file_mask: str,
        relative_path: str = ".",
    ) -> dict[str, Any]:
        """Find files matching a pattern.

        Args:
            file_mask: Filename or glob pattern (e.g., "*.py", "test_*.py").
            relative_path: Directory to search in.

        Returns:
            Dict with 'matches' list of relative paths.
        """
        try:
            path = self._resolve_path(relative_path)

            if not path.exists():
                return {"error": f"Directory not found: {relative_path}"}

            if not path.is_dir():
                return {"error": f"Not a directory: {relative_path}"}

            matches: list[str] = []

            # Skip patterns
            skip_dirs = {".git", ".c4", "__pycache__", "node_modules", ".venv"}

            for item in path.rglob(file_mask):
                # Skip if in ignored directory
                parts = item.relative_to(self.project_root).parts
                if any(skip in parts for skip in skip_dirs):
                    continue

                if item.is_file():
                    rel_path = str(item.relative_to(self.project_root))
                    matches.append(rel_path)

            return {
                "matches": sorted(matches),
                "count": len(matches),
            }

        except Exception as e:
            logger.error(f"Error finding files {file_mask}: {e}")
            return {"error": str(e)}

    def search_for_pattern(
        self,
        pattern: str,
        relative_path: str = ".",
        glob_pattern: str | None = None,
        context_lines: int = 0,
        max_matches: int = 100,
    ) -> dict[str, Any]:
        """Search for a regex pattern in files.

        Args:
            pattern: Regular expression pattern to search for.
            relative_path: Directory or file to search in.
            glob_pattern: Optional glob to filter files (e.g., "*.py").
            context_lines: Number of context lines before/after match.
            max_matches: Maximum number of matches to return.

        Returns:
            Dict with 'matches' list of {file, line, content, context}.
        """
        try:
            path = self._resolve_path(relative_path)

            if not path.exists():
                return {"error": f"Path not found: {relative_path}"}

            regex = re.compile(pattern, re.MULTILINE | re.DOTALL)
            matches: list[dict[str, Any]] = []

            # Skip patterns
            skip_dirs = {".git", ".c4", "__pycache__", "node_modules", ".venv"}
            binary_extensions = {".pyc", ".pyo", ".so", ".dll", ".exe", ".png", ".jpg"}

            def search_file(file_path: Path) -> None:
                if len(matches) >= max_matches:
                    return

                # Skip binary files
                if file_path.suffix.lower() in binary_extensions:
                    return

                try:
                    content = file_path.read_text(encoding="utf-8")
                    lines = content.splitlines()

                    for i, line in enumerate(lines):
                        if regex.search(line):
                            # Get context lines
                            start = max(0, i - context_lines)
                            end = min(len(lines), i + context_lines + 1)
                            context = lines[start:end]

                            rel_path = str(file_path.relative_to(self.project_root))
                            matches.append({
                                "file": rel_path,
                                "line": i + 1,  # 1-based
                                "content": line,
                                "context": context if context_lines > 0 else None,
                            })

                            if len(matches) >= max_matches:
                                return

                except (UnicodeDecodeError, PermissionError):
                    pass

            if path.is_file():
                search_file(path)
            else:
                # Iterate over files
                if glob_pattern:
                    files = path.rglob(glob_pattern)
                else:
                    files = path.rglob("*")

                for file_path in files:
                    if not file_path.is_file():
                        continue

                    # Skip ignored directories
                    parts = file_path.relative_to(self.project_root).parts
                    if any(skip in parts for skip in skip_dirs):
                        continue

                    search_file(file_path)

                    if len(matches) >= max_matches:
                        break

            return {
                "matches": matches,
                "count": len(matches),
                "truncated": len(matches) >= max_matches,
            }

        except re.error as e:
            return {"error": f"Invalid regex pattern: {e}"}
        except Exception as e:
            logger.error(f"Error searching for pattern: {e}")
            return {"error": str(e)}

    def replace_content(
        self,
        relative_path: str,
        needle: str,
        replacement: str,
        mode: str = "literal",
        allow_multiple: bool = False,
    ) -> dict[str, Any]:
        """Replace content in a file.

        Args:
            relative_path: Path relative to project root.
            needle: String or regex pattern to search for.
            replacement: Replacement string.
            mode: "literal" for exact match, "regex" for regex.
            allow_multiple: Whether to allow multiple replacements.

        Returns:
            Dict with 'success', 'replacements_made', 'message'.
        """
        try:
            path = self._resolve_path(relative_path)

            if not path.exists():
                return {"success": False, "error": f"File not found: {relative_path}"}

            if not path.is_file():
                return {"success": False, "error": f"Not a file: {relative_path}"}

            content = path.read_text(encoding="utf-8")

            if mode == "literal":
                # Count occurrences
                count = content.count(needle)

                if count == 0:
                    return {
                        "success": False,
                        "error": "Pattern not found in file",
                    }

                if count > 1 and not allow_multiple:
                    return {
                        "success": False,
                        "error": f"Pattern matches {count} times. "
                        f"Use allow_multiple=True to replace all.",
                    }

                new_content = content.replace(needle, replacement)
                replacements = count

            elif mode == "regex":
                regex = re.compile(needle, re.MULTILINE | re.DOTALL)
                matches = regex.findall(content)
                count = len(matches)

                if count == 0:
                    return {
                        "success": False,
                        "error": "Regex pattern not found in file",
                    }

                if count > 1 and not allow_multiple:
                    return {
                        "success": False,
                        "error": f"Pattern matches {count} times. "
                        f"Use allow_multiple=True to replace all.",
                    }

                new_content = regex.sub(replacement, content)
                replacements = count

            else:
                return {"success": False, "error": f"Invalid mode: {mode}"}

            # Write the file
            path.write_text(new_content, encoding="utf-8")

            return {
                "success": True,
                "replacements_made": replacements,
                "message": f"Replaced {replacements} occurrence(s) in {relative_path}",
            }

        except re.error as e:
            return {"success": False, "error": f"Invalid regex: {e}"}
        except Exception as e:
            logger.error(f"Error replacing content in {relative_path}: {e}")
            return {"success": False, "error": str(e)}
