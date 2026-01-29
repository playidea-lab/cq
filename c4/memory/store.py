"""Memory Store - Persistent memory storage for C4 projects

Provides a simple key-value store for project memories,
similar to Serena's memory system for maintaining context
across conversation sessions.

Memories are stored as markdown files in .c4/memories/ directory.
"""

from __future__ import annotations

import logging
import re
from pathlib import Path

logger = logging.getLogger(__name__)


class MemoryError(Exception):
    """Base exception for memory operations."""

    pass


class MemoryNotFoundError(MemoryError):
    """Raised when a memory file is not found."""

    pass


class MemoryStore:
    """
    Persistent memory storage for C4 projects.

    Stores memories as markdown files in the .c4/memories/ directory.
    Memory names are sanitized to be valid filenames.

    Attributes:
        memories_dir: Path to the memories directory

    Example:
        store = MemoryStore(Path(".c4"))

        # Write memory
        store.write("project-architecture", "# Architecture\\n...")

        # Read memory
        content = store.read("project-architecture")

        # List all memories
        for name, info in store.list_all().items():
            print(f"{name}: {info['size']} bytes")

        # Edit memory with regex or literal replacement
        store.edit("project-architecture", "old pattern", "replacement", mode="literal")

        # Delete memory
        store.delete("project-architecture")
    """

    def __init__(self, c4_dir: Path) -> None:
        """
        Initialize memory store.

        Args:
            c4_dir: Path to .c4 directory
        """
        self.c4_dir = c4_dir
        self.memories_dir = c4_dir / "memories"

    def _ensure_dir(self) -> None:
        """Ensure memories directory exists."""
        self.memories_dir.mkdir(parents=True, exist_ok=True)

    def _sanitize_name(self, name: str) -> str:
        """
        Sanitize memory name for use as filename.

        Args:
            name: Raw memory name

        Returns:
            Sanitized name safe for filesystem
        """
        # Remove .md extension if provided
        if name.endswith(".md"):
            name = name[:-3]

        # Replace unsafe characters with dashes
        sanitized = re.sub(r"[^\w\-]", "-", name)

        # Remove multiple consecutive dashes
        sanitized = re.sub(r"-+", "-", sanitized)

        # Remove leading/trailing dashes
        sanitized = sanitized.strip("-")

        # Ensure non-empty
        if not sanitized:
            sanitized = "unnamed"

        return sanitized

    def _get_memory_path(self, name: str) -> Path:
        """
        Get file path for a memory.

        Args:
            name: Memory name

        Returns:
            Path to memory file
        """
        sanitized = self._sanitize_name(name)
        return self.memories_dir / f"{sanitized}.md"

    def write(self, name: str, content: str) -> dict[str, str]:
        """
        Write or overwrite a memory.

        Args:
            name: Memory name (will be sanitized for filesystem)
            content: Memory content (markdown)

        Returns:
            Dict with operation result
        """
        self._ensure_dir()

        path = self._get_memory_path(name)
        sanitized_name = self._sanitize_name(name)

        path.write_text(content, encoding="utf-8")

        logger.info(f"Memory written: {sanitized_name}")

        return {
            "status": "success",
            "name": sanitized_name,
            "path": str(path),
            "size": len(content),
        }

    def read(self, name: str, max_chars: int = -1) -> dict[str, str]:
        """
        Read a memory.

        Args:
            name: Memory name
            max_chars: Maximum characters to return (-1 for no limit)

        Returns:
            Dict with memory content and metadata

        Raises:
            MemoryNotFoundError: If memory does not exist
        """
        path = self._get_memory_path(name)
        sanitized_name = self._sanitize_name(name)

        if not path.exists():
            raise MemoryNotFoundError(f"Memory not found: {sanitized_name}")

        content = path.read_text(encoding="utf-8")

        # Apply character limit if specified
        truncated = False
        if max_chars > 0 and len(content) > max_chars:
            content = content[:max_chars]
            truncated = True

        logger.debug(f"Memory read: {sanitized_name}")

        return {
            "name": sanitized_name,
            "content": content,
            "size": path.stat().st_size,
            "truncated": truncated,
        }

    def list_all(self) -> dict[str, dict[str, str | int]]:
        """
        List all available memories.

        Returns:
            Dict mapping memory names to their metadata
        """
        if not self.memories_dir.exists():
            return {}

        memories = {}
        for path in sorted(self.memories_dir.glob("*.md")):
            name = path.stem  # filename without .md
            stat = path.stat()
            memories[name] = {
                "path": str(path),
                "size": stat.st_size,
                "modified": stat.st_mtime,
            }

        return memories

    def edit(
        self,
        name: str,
        needle: str,
        replacement: str,
        mode: str = "literal",
    ) -> dict[str, str | int]:
        """
        Edit a memory by replacing content.

        Args:
            name: Memory name
            needle: String or pattern to find
            replacement: Replacement text
            mode: "literal" for exact match, "regex" for regex pattern

        Returns:
            Dict with operation result

        Raises:
            MemoryNotFoundError: If memory does not exist
            ValueError: If invalid mode specified
        """
        path = self._get_memory_path(name)
        sanitized_name = self._sanitize_name(name)

        if not path.exists():
            raise MemoryNotFoundError(f"Memory not found: {sanitized_name}")

        content = path.read_text(encoding="utf-8")

        if mode == "literal":
            # Exact string replacement
            if needle not in content:
                return {
                    "status": "no_match",
                    "name": sanitized_name,
                    "message": "Pattern not found in memory",
                }
            new_content = content.replace(needle, replacement)
            replacements = content.count(needle)

        elif mode == "regex":
            # Regex replacement with DOTALL and MULTILINE
            try:
                pattern = re.compile(needle, re.DOTALL | re.MULTILINE)
            except re.error as e:
                return {
                    "status": "error",
                    "name": sanitized_name,
                    "message": f"Invalid regex pattern: {e}",
                }

            matches = pattern.findall(content)
            if not matches:
                return {
                    "status": "no_match",
                    "name": sanitized_name,
                    "message": "Pattern not found in memory",
                }

            new_content = pattern.sub(replacement, content)
            replacements = len(matches)

        else:
            raise ValueError(f"Invalid mode: {mode}. Use 'literal' or 'regex'")

        # Write updated content
        path.write_text(new_content, encoding="utf-8")

        logger.info(f"Memory edited: {sanitized_name} ({replacements} replacements)")

        return {
            "status": "success",
            "name": sanitized_name,
            "replacements": replacements,
            "old_size": len(content),
            "new_size": len(new_content),
        }

    def delete(self, name: str) -> dict[str, str]:
        """
        Delete a memory.

        Args:
            name: Memory name

        Returns:
            Dict with operation result

        Raises:
            MemoryNotFoundError: If memory does not exist
        """
        path = self._get_memory_path(name)
        sanitized_name = self._sanitize_name(name)

        if not path.exists():
            raise MemoryNotFoundError(f"Memory not found: {sanitized_name}")

        path.unlink()

        logger.info(f"Memory deleted: {sanitized_name}")

        return {
            "status": "success",
            "name": sanitized_name,
            "message": f"Memory '{sanitized_name}' deleted",
        }

    def exists(self, name: str) -> bool:
        """
        Check if a memory exists.

        Args:
            name: Memory name

        Returns:
            True if memory exists
        """
        path = self._get_memory_path(name)
        return path.exists()


def migrate_from_serena(
    serena_memories_dir: Path,
    c4_dir: Path,
) -> dict[str, list[str]]:
    """
    Migrate memories from Serena to C4 format.

    This utility helps migrate existing Serena memory files
    to the C4 memories directory.

    Args:
        serena_memories_dir: Path to Serena's memories directory
        c4_dir: Path to C4's .c4 directory

    Returns:
        Dict with lists of migrated and skipped files
    """
    store = MemoryStore(c4_dir)
    migrated = []
    skipped = []

    if not serena_memories_dir.exists():
        return {"migrated": [], "skipped": [], "error": "Source directory not found"}

    for path in serena_memories_dir.glob("*.md"):
        name = path.stem
        content = path.read_text(encoding="utf-8")

        # Check if already exists in C4
        if store.exists(name):
            skipped.append(name)
            continue

        store.write(name, content)
        migrated.append(name)

    return {"migrated": migrated, "skipped": skipped}
