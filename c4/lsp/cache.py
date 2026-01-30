"""Two-stage symbol cache for C4 LSP.

Implements a content-hash based caching strategy inspired by Serena's approach:
1. Stage 1: Raw symbols from Jedi/LSP (fast lookup by content hash)
2. Stage 2: Processed symbols (transformed for MCP tool output)

This eliminates redundant parsing of unchanged files and provides
significant performance improvements for large codebases.

Features:
- Content-hash based invalidation (file changes -> cache miss)
- LRU eviction (configurable max entries)
- Separate cache for raw and processed symbols
- Thread-safe operations
- Hit/miss statistics for monitoring
"""

from __future__ import annotations

import hashlib
import logging
import threading
from collections import OrderedDict
from dataclasses import dataclass, field
from time import time
from typing import Any

logger = logging.getLogger(__name__)


@dataclass
class CacheEntry:
    """Single cache entry with content hash and symbols."""

    content_hash: str
    raw_symbols: list[dict]
    processed_symbols: list[dict] | None = None
    timestamp: float = field(default_factory=time)

    def is_valid(self, content_hash: str) -> bool:
        """Check if entry is still valid for the given content hash."""
        return self.content_hash == content_hash


@dataclass
class CacheStats:
    """Cache hit/miss statistics."""

    hits: int = 0
    misses: int = 0
    evictions: int = 0

    @property
    def hit_rate(self) -> float:
        """Calculate hit rate as a percentage."""
        total = self.hits + self.misses
        return (self.hits / total * 100) if total > 0 else 0.0

    def record_hit(self) -> None:
        """Record a cache hit."""
        self.hits += 1

    def record_miss(self) -> None:
        """Record a cache miss."""
        self.misses += 1

    def record_eviction(self) -> None:
        """Record a cache eviction."""
        self.evictions += 1

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary for monitoring."""
        return {
            "hits": self.hits,
            "misses": self.misses,
            "evictions": self.evictions,
            "hit_rate": f"{self.hit_rate:.1f}%",
        }


class SymbolCache:
    """Thread-safe LRU cache for symbol data.

    Uses content hash for invalidation - when file content changes,
    the cache entry becomes invalid regardless of timestamp.

    Args:
        max_entries: Maximum number of files to cache (default: 1000)
        ttl_seconds: Optional TTL for entries (default: None = no TTL)
    """

    def __init__(
        self,
        max_entries: int = 1000,
        ttl_seconds: float | None = None,
    ) -> None:
        self._max_entries = max_entries
        self._ttl_seconds = ttl_seconds
        self._cache: OrderedDict[str, CacheEntry] = OrderedDict()
        self._lock = threading.RLock()
        self._stats = CacheStats()

    @staticmethod
    def compute_hash(content: str) -> str:
        """Compute content hash for cache key validation.

        Uses SHA-256 for content integrity verification.
        """
        return hashlib.sha256(content.encode("utf-8")).hexdigest()[:16]

    def get(
        self,
        file_path: str,
        content_hash: str,
        stage: str = "raw",
    ) -> list[dict] | None:
        """Get cached symbols if valid.

        Args:
            file_path: Path to the file
            content_hash: Current content hash for validation
            stage: "raw" for raw symbols, "processed" for processed

        Returns:
            Cached symbols if valid, None if miss
        """
        with self._lock:
            entry = self._cache.get(file_path)

            if entry is None:
                self._stats.record_miss()
                return None

            # Check content hash validity
            if not entry.is_valid(content_hash):
                # Content changed, invalidate entry
                del self._cache[file_path]
                self._stats.record_miss()
                return None

            # Check TTL if configured
            if self._ttl_seconds is not None:
                age = time() - entry.timestamp
                if age > self._ttl_seconds:
                    del self._cache[file_path]
                    self._stats.record_miss()
                    return None

            # Move to end (LRU)
            self._cache.move_to_end(file_path)
            self._stats.record_hit()

            if stage == "processed" and entry.processed_symbols is not None:
                return entry.processed_symbols
            return entry.raw_symbols

    def put(
        self,
        file_path: str,
        content_hash: str,
        raw_symbols: list[dict],
        processed_symbols: list[dict] | None = None,
    ) -> None:
        """Store symbols in cache.

        Args:
            file_path: Path to the file
            content_hash: Content hash for validation
            raw_symbols: Raw symbols from Jedi/LSP
            processed_symbols: Optional processed symbols
        """
        with self._lock:
            # Evict oldest if at capacity
            while len(self._cache) >= self._max_entries:
                oldest_key = next(iter(self._cache))
                del self._cache[oldest_key]
                self._stats.record_eviction()

            # Store entry
            self._cache[file_path] = CacheEntry(
                content_hash=content_hash,
                raw_symbols=raw_symbols,
                processed_symbols=processed_symbols,
            )

    def update_processed(
        self,
        file_path: str,
        processed_symbols: list[dict],
    ) -> bool:
        """Update processed symbols for an existing entry.

        Args:
            file_path: Path to the file
            processed_symbols: Processed symbols to store

        Returns:
            True if entry existed and was updated
        """
        with self._lock:
            entry = self._cache.get(file_path)
            if entry is None:
                return False

            entry.processed_symbols = processed_symbols
            return True

    def invalidate(self, file_path: str) -> bool:
        """Invalidate cache entry for a file.

        Args:
            file_path: Path to the file

        Returns:
            True if entry was removed
        """
        with self._lock:
            if file_path in self._cache:
                del self._cache[file_path]
                return True
            return False

    def clear(self) -> int:
        """Clear all cache entries.

        Returns:
            Number of entries cleared
        """
        with self._lock:
            count = len(self._cache)
            self._cache.clear()
            return count

    @property
    def stats(self) -> CacheStats:
        """Get cache statistics."""
        return self._stats

    @property
    def size(self) -> int:
        """Get current cache size."""
        with self._lock:
            return len(self._cache)

    def get_status(self) -> dict[str, Any]:
        """Get cache status for monitoring.

        Returns:
            Dictionary with cache statistics and status
        """
        with self._lock:
            return {
                "size": len(self._cache),
                "max_entries": self._max_entries,
                "ttl_seconds": self._ttl_seconds,
                "stats": self._stats.to_dict(),
            }


# Global cache instance for shared use
_global_cache: SymbolCache | None = None
_global_cache_lock = threading.Lock()


def get_symbol_cache(
    max_entries: int = 1000,
    ttl_seconds: float | None = None,
) -> SymbolCache:
    """Get or create the global symbol cache.

    Args:
        max_entries: Maximum entries (only used on first call)
        ttl_seconds: TTL in seconds (only used on first call)

    Returns:
        Global SymbolCache instance
    """
    global _global_cache

    with _global_cache_lock:
        if _global_cache is None:
            _global_cache = SymbolCache(
                max_entries=max_entries,
                ttl_seconds=ttl_seconds,
            )
            logger.info(
                f"Symbol cache initialized (max_entries={max_entries}, "
                f"ttl={ttl_seconds})"
            )

    return _global_cache


def reset_global_cache() -> None:
    """Reset the global cache instance (for testing)."""
    global _global_cache

    with _global_cache_lock:
        if _global_cache is not None:
            _global_cache.clear()
        _global_cache = None
