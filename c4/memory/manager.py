"""Memory Manager for C4.

Provides persistent knowledge storage across sessions.
Follows the Serena MCP server pattern for memory management.

Features:
- Local storage in .c4/memories/*.md
- Content-based change detection
- Optional Hub synchronization (future)
- Thread-safe operations
"""

from __future__ import annotations

import hashlib
import logging
import re
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from threading import RLock
from typing import TYPE_CHECKING, Any, Protocol

if TYPE_CHECKING:
    pass

logger = logging.getLogger(__name__)


class HubClient(Protocol):
    """Protocol for Hub synchronization (future implementation)."""

    def upload_memory(self, name: str, content: str) -> bool:
        """Upload memory to Hub."""
        ...

    def download_memory(self, name: str) -> str | None:
        """Download memory from Hub."""
        ...

    def list_memories(self) -> list[str]:
        """List memories on Hub."""
        ...

    def delete_memory(self, name: str) -> bool:
        """Delete memory from Hub."""
        ...


@dataclass
class MemoryMetadata:
    """Metadata for a memory entry."""

    name: str
    path: Path
    size_bytes: int
    content_hash: str
    created_at: datetime
    modified_at: datetime

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "name": self.name,
            "path": str(self.path),
            "size_bytes": self.size_bytes,
            "content_hash": self.content_hash,
            "created_at": self.created_at.isoformat(),
            "modified_at": self.modified_at.isoformat(),
        }


@dataclass
class MemoryStats:
    """Statistics for memory operations."""

    total_memories: int = 0
    total_size_bytes: int = 0
    reads: int = 0
    writes: int = 0
    deletes: int = 0
    cache_hits: int = 0
    cache_misses: int = 0

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "total_memories": self.total_memories,
            "total_size_bytes": self.total_size_bytes,
            "reads": self.reads,
            "writes": self.writes,
            "deletes": self.deletes,
            "cache_hits": self.cache_hits,
            "cache_misses": self.cache_misses,
        }


class MemoryManager:
    """Manager for persistent memory storage.

    Stores memories as markdown files in .c4/memories/ directory.
    Supports optional synchronization with C4Hub (future).

    Example:
        manager = MemoryManager(Path("/project"))
        manager.write("decisions", "# Architecture Decisions\\n...")
        content = manager.read("decisions")
    """

    # File extension for memories
    EXTENSION = ".md"

    # Maximum memory name length
    MAX_NAME_LENGTH = 100

    # Characters allowed in memory names (after sanitization)
    SAFE_NAME_PATTERN = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9_-]*$")

    def __init__(
        self,
        project_path: Path,
        hub_client: HubClient | None = None,
        *,
        auto_sync: bool = True,
    ) -> None:
        """Initialize memory manager.

        Args:
            project_path: Path to the project root
            hub_client: Optional Hub client for synchronization
            auto_sync: Whether to auto-sync with Hub on write/delete
        """
        self._project_path = Path(project_path)
        self._memory_dir = self._project_path / ".c4" / "memories"
        self._hub = hub_client
        self._auto_sync = auto_sync
        self._lock = RLock()
        self._stats = MemoryStats()
        self._content_cache: dict[str, str] = {}  # name -> content
        self._hash_cache: dict[str, str] = {}  # name -> content_hash

        # Ensure directory exists
        self._memory_dir.mkdir(parents=True, exist_ok=True)
        logger.debug(f"MemoryManager initialized at {self._memory_dir}")

    @property
    def memory_dir(self) -> Path:
        """Get the memory storage directory."""
        return self._memory_dir

    @property
    def stats(self) -> MemoryStats:
        """Get memory statistics."""
        with self._lock:
            self._stats.total_memories = len(list(self._memory_dir.glob(f"*{self.EXTENSION}")))
            self._stats.total_size_bytes = sum(
                f.stat().st_size for f in self._memory_dir.glob(f"*{self.EXTENSION}")
            )
            return self._stats

    def write(
        self,
        name: str,
        content: str,
        *,
        sync: bool | None = None,
    ) -> Path:
        """Write content to a memory.

        Creates or updates a memory file. If Hub client is configured
        and sync is enabled, also uploads to Hub.

        Args:
            name: Memory name (will be sanitized)
            content: Content to write (markdown)
            sync: Override auto_sync setting for this write

        Returns:
            Path to the written file

        Raises:
            ValueError: If name is invalid or too long
        """
        safe_name = self._sanitize_name(name)
        path = self._resolve_path(safe_name)

        with self._lock:
            # Compute hash for change detection
            content_hash = self._compute_hash(content)

            # Skip write if content unchanged
            if safe_name in self._hash_cache and self._hash_cache[safe_name] == content_hash:
                logger.debug(f"Memory '{safe_name}' unchanged, skipping write")
                self._stats.cache_hits += 1
                return path

            # Write to file
            path.write_text(content, encoding="utf-8")
            self._stats.writes += 1

            # Update caches
            self._content_cache[safe_name] = content
            self._hash_cache[safe_name] = content_hash

            logger.info(f"Memory written: {safe_name} ({len(content)} bytes)")

        # Sync to Hub if configured
        should_sync = sync if sync is not None else self._auto_sync
        if should_sync and self._hub:
            try:
                self._hub.upload_memory(safe_name, content)
                logger.debug(f"Memory synced to Hub: {safe_name}")
            except Exception as e:
                logger.warning(f"Failed to sync memory to Hub: {e}")

        return path

    def read(self, name: str) -> str | None:
        """Read content from a memory.

        Checks local cache first, then local file, then Hub (if configured).

        Args:
            name: Memory name

        Returns:
            Content if found, None otherwise
        """
        safe_name = self._sanitize_name(name)

        with self._lock:
            self._stats.reads += 1

            # Check cache first
            if safe_name in self._content_cache:
                self._stats.cache_hits += 1
                return self._content_cache[safe_name]

            self._stats.cache_misses += 1

            # Check local file
            path = self._resolve_path(safe_name)
            if path.exists():
                content = path.read_text(encoding="utf-8")
                self._content_cache[safe_name] = content
                self._hash_cache[safe_name] = self._compute_hash(content)
                return content

        # Try Hub as fallback
        if self._hub:
            try:
                content = self._hub.download_memory(safe_name)
                if content:
                    # Cache locally
                    self.write(safe_name, content, sync=False)
                    return content
            except Exception as e:
                logger.warning(f"Failed to download memory from Hub: {e}")

        return None

    def list(self) -> list[str]:
        """List all memory names.

        Returns:
            List of memory names (without extension)
        """
        with self._lock:
            names = [
                p.stem for p in sorted(self._memory_dir.glob(f"*{self.EXTENSION}"))
            ]
            return names

    def delete(self, name: str, *, sync: bool | None = None) -> bool:
        """Delete a memory.

        Args:
            name: Memory name
            sync: Override auto_sync setting for this delete

        Returns:
            True if deleted, False if not found
        """
        safe_name = self._sanitize_name(name)
        path = self._resolve_path(safe_name)

        with self._lock:
            if not path.exists():
                return False

            path.unlink()
            self._stats.deletes += 1

            # Clear caches
            self._content_cache.pop(safe_name, None)
            self._hash_cache.pop(safe_name, None)

            logger.info(f"Memory deleted: {safe_name}")

        # Sync to Hub if configured
        should_sync = sync if sync is not None else self._auto_sync
        if should_sync and self._hub:
            try:
                self._hub.delete_memory(safe_name)
                logger.debug(f"Memory deleted from Hub: {safe_name}")
            except Exception as e:
                logger.warning(f"Failed to delete memory from Hub: {e}")

        return True

    def exists(self, name: str) -> bool:
        """Check if a memory exists.

        Args:
            name: Memory name

        Returns:
            True if exists, False otherwise
        """
        safe_name = self._sanitize_name(name)
        path = self._resolve_path(safe_name)
        return path.exists()

    def get_metadata(self, name: str) -> MemoryMetadata | None:
        """Get metadata for a memory.

        Args:
            name: Memory name

        Returns:
            Metadata if found, None otherwise
        """
        safe_name = self._sanitize_name(name)
        path = self._resolve_path(safe_name)

        if not path.exists():
            return None

        stat = path.stat()
        content = path.read_text(encoding="utf-8")

        return MemoryMetadata(
            name=safe_name,
            path=path,
            size_bytes=stat.st_size,
            content_hash=self._compute_hash(content),
            created_at=datetime.fromtimestamp(stat.st_ctime, tz=timezone.utc),
            modified_at=datetime.fromtimestamp(stat.st_mtime, tz=timezone.utc),
        )

    def search(self, pattern: str) -> list[str]:
        """Search memories by name pattern.

        Args:
            pattern: Glob pattern to match

        Returns:
            List of matching memory names
        """
        with self._lock:
            matches = []
            for path in self._memory_dir.glob(f"*{pattern}*{self.EXTENSION}"):
                matches.append(path.stem)
            return sorted(matches)

    def clear_cache(self) -> int:
        """Clear the in-memory cache.

        Returns:
            Number of entries cleared
        """
        with self._lock:
            count = len(self._content_cache)
            self._content_cache.clear()
            self._hash_cache.clear()
            return count

    def get_status(self) -> dict[str, Any]:
        """Get status information for monitoring.

        Returns:
            Status dictionary
        """
        stats = self.stats
        return {
            "memory_dir": str(self._memory_dir),
            "hub_connected": self._hub is not None,
            "auto_sync": self._auto_sync,
            "cache_size": len(self._content_cache),
            **stats.to_dict(),
        }

    def _sanitize_name(self, name: str) -> str:
        """Sanitize memory name for safe file storage.

        Args:
            name: Raw memory name

        Returns:
            Sanitized name

        Raises:
            ValueError: If name is invalid
        """
        if not name:
            raise ValueError("Memory name cannot be empty")

        # Replace path separators and spaces
        safe = name.replace("/", "-").replace("\\", "-").replace(" ", "-")

        # Remove consecutive dashes
        safe = re.sub(r"-+", "-", safe)

        # Strip leading/trailing dashes
        safe = safe.strip("-")

        # Validate
        if not safe:
            raise ValueError(f"Memory name '{name}' results in empty string after sanitization")

        if len(safe) > self.MAX_NAME_LENGTH:
            raise ValueError(
                f"Memory name too long: {len(safe)} > {self.MAX_NAME_LENGTH}"
            )

        if not self.SAFE_NAME_PATTERN.match(safe):
            raise ValueError(
                f"Memory name '{safe}' contains invalid characters. "
                "Use alphanumeric, dash, or underscore only."
            )

        return safe

    def _resolve_path(self, safe_name: str) -> Path:
        """Resolve sanitized name to file path.

        Args:
            safe_name: Already sanitized name

        Returns:
            Full path to memory file
        """
        return self._memory_dir / f"{safe_name}{self.EXTENSION}"

    @staticmethod
    def _compute_hash(content: str) -> str:
        """Compute content hash for change detection.

        Args:
            content: Content to hash

        Returns:
            SHA-256 hash prefix (16 chars)
        """
        return hashlib.sha256(content.encode("utf-8")).hexdigest()[:16]


# Global instance management
_global_manager: MemoryManager | None = None
_manager_lock = RLock()


def get_memory_manager(
    project_path: Path | str | None = None,
    hub_client: HubClient | None = None,
) -> MemoryManager:
    """Get or create the global memory manager.

    Args:
        project_path: Project root path (required on first call)
        hub_client: Optional Hub client for synchronization

    Returns:
        Global MemoryManager instance

    Raises:
        ValueError: If project_path not provided on first call
    """
    global _global_manager

    with _manager_lock:
        if _global_manager is None:
            if project_path is None:
                raise ValueError("project_path required on first call")
            _global_manager = MemoryManager(
                Path(project_path),
                hub_client=hub_client,
            )

    return _global_manager


def reset_global_manager() -> None:
    """Reset the global memory manager (for testing)."""
    global _global_manager

    with _manager_lock:
        if _global_manager is not None:
            _global_manager.clear_cache()
        _global_manager = None
