"""Tests for the symbol cache module."""

import pytest

from c4.lsp.cache import (
    CacheEntry,
    CacheStats,
    SymbolCache,
    get_symbol_cache,
    reset_global_cache,
)


class TestCacheEntry:
    """Tests for CacheEntry dataclass."""

    def test_entry_creation(self):
        """Test creating a cache entry."""
        entry = CacheEntry(
            content_hash="abc123",
            raw_symbols=[{"name": "test"}],
        )
        assert entry.content_hash == "abc123"
        assert entry.raw_symbols == [{"name": "test"}]
        assert entry.processed_symbols is None
        assert entry.timestamp > 0

    def test_entry_is_valid(self):
        """Test content hash validation."""
        entry = CacheEntry(
            content_hash="abc123",
            raw_symbols=[{"name": "test"}],
        )
        assert entry.is_valid("abc123") is True
        assert entry.is_valid("different") is False


class TestCacheStats:
    """Tests for CacheStats."""

    def test_initial_stats(self):
        """Test initial stats are zero."""
        stats = CacheStats()
        assert stats.hits == 0
        assert stats.misses == 0
        assert stats.evictions == 0
        assert stats.hit_rate == 0.0

    def test_hit_rate_calculation(self):
        """Test hit rate calculation."""
        stats = CacheStats()
        stats.record_hit()
        stats.record_hit()
        stats.record_miss()
        # 2 hits, 1 miss = 66.7%
        assert abs(stats.hit_rate - 66.67) < 1.0

    def test_to_dict(self):
        """Test stats serialization."""
        stats = CacheStats()
        stats.record_hit()
        stats.record_eviction()

        d = stats.to_dict()
        assert d["hits"] == 1
        assert d["misses"] == 0
        assert d["evictions"] == 1
        assert "hit_rate" in d


class TestSymbolCache:
    """Tests for SymbolCache."""

    @pytest.fixture
    def cache(self):
        """Create a fresh cache for each test."""
        return SymbolCache(max_entries=10)

    def test_put_and_get(self, cache):
        """Test basic put/get operations."""
        cache.put(
            "/test/file.py",
            "hash123",
            [{"name": "symbol1"}],
        )

        result = cache.get("/test/file.py", "hash123")
        assert result == [{"name": "symbol1"}]

    def test_get_with_wrong_hash(self, cache):
        """Test cache miss with different hash."""
        cache.put(
            "/test/file.py",
            "hash123",
            [{"name": "symbol1"}],
        )

        result = cache.get("/test/file.py", "different_hash")
        assert result is None

    def test_get_nonexistent(self, cache):
        """Test get for non-existent file."""
        result = cache.get("/nonexistent.py", "hash123")
        assert result is None

    def test_lru_eviction(self):
        """Test LRU eviction when at capacity."""
        cache = SymbolCache(max_entries=3)

        # Fill cache
        cache.put("/file1.py", "h1", [{"name": "s1"}])
        cache.put("/file2.py", "h2", [{"name": "s2"}])
        cache.put("/file3.py", "h3", [{"name": "s3"}])

        assert cache.size == 3

        # Add one more - should evict oldest
        cache.put("/file4.py", "h4", [{"name": "s4"}])

        assert cache.size == 3
        assert cache.get("/file1.py", "h1") is None  # Evicted
        assert cache.get("/file4.py", "h4") is not None

    def test_lru_access_order(self):
        """Test that access updates LRU order."""
        cache = SymbolCache(max_entries=3)

        cache.put("/file1.py", "h1", [{"name": "s1"}])
        cache.put("/file2.py", "h2", [{"name": "s2"}])
        cache.put("/file3.py", "h3", [{"name": "s3"}])

        # Access file1 - moves to end
        cache.get("/file1.py", "h1")

        # Add new file - should evict file2 (now oldest)
        cache.put("/file4.py", "h4", [{"name": "s4"}])

        assert cache.get("/file1.py", "h1") is not None  # Still here
        assert cache.get("/file2.py", "h2") is None  # Evicted
        assert cache.get("/file3.py", "h3") is not None

    def test_compute_hash(self, cache):
        """Test content hash computation."""
        content = "def hello(): pass"
        hash1 = cache.compute_hash(content)
        hash2 = cache.compute_hash(content)

        assert hash1 == hash2
        assert len(hash1) == 16

        # Different content should have different hash
        hash3 = cache.compute_hash("def goodbye(): pass")
        assert hash1 != hash3

    def test_invalidate(self, cache):
        """Test cache invalidation."""
        cache.put("/test/file.py", "hash123", [{"name": "symbol"}])
        assert cache.size == 1

        result = cache.invalidate("/test/file.py")
        assert result is True
        assert cache.size == 0

        # Invalidating again returns False
        result = cache.invalidate("/test/file.py")
        assert result is False

    def test_clear(self, cache):
        """Test clearing the cache."""
        cache.put("/file1.py", "h1", [{"name": "s1"}])
        cache.put("/file2.py", "h2", [{"name": "s2"}])

        count = cache.clear()
        assert count == 2
        assert cache.size == 0

    def test_processed_symbols(self, cache):
        """Test storing and retrieving processed symbols."""
        cache.put(
            "/test/file.py",
            "hash123",
            [{"name": "raw"}],
            [{"name": "processed"}],
        )

        raw = cache.get("/test/file.py", "hash123", stage="raw")
        processed = cache.get("/test/file.py", "hash123", stage="processed")

        assert raw == [{"name": "raw"}]
        assert processed == [{"name": "processed"}]

    def test_update_processed(self, cache):
        """Test updating processed symbols separately."""
        cache.put("/test/file.py", "hash123", [{"name": "raw"}])

        result = cache.update_processed(
            "/test/file.py", [{"name": "processed"}]
        )
        assert result is True

        processed = cache.get("/test/file.py", "hash123", stage="processed")
        assert processed == [{"name": "processed"}]

    def test_update_processed_nonexistent(self, cache):
        """Test update_processed for non-existent entry."""
        result = cache.update_processed("/nonexistent.py", [{"name": "data"}])
        assert result is False

    def test_stats_tracking(self, cache):
        """Test that stats are tracked correctly."""
        cache.put("/test/file.py", "hash123", [{"name": "s"}])

        # Hit
        cache.get("/test/file.py", "hash123")
        # Miss (wrong hash)
        cache.get("/test/file.py", "wrong")
        # Miss (not found)
        cache.get("/other.py", "hash")

        stats = cache.stats
        assert stats.hits == 1
        assert stats.misses == 2

    def test_get_status(self, cache):
        """Test status reporting."""
        cache.put("/test/file.py", "hash123", [{"name": "s"}])
        cache.get("/test/file.py", "hash123")

        status = cache.get_status()
        assert status["size"] == 1
        assert status["max_entries"] == 10
        assert "stats" in status


class TestGlobalCache:
    """Tests for global cache management."""

    def setup_method(self):
        """Reset global cache before each test."""
        reset_global_cache()

    def test_get_symbol_cache_singleton(self):
        """Test that get_symbol_cache returns same instance."""
        cache1 = get_symbol_cache()
        cache2 = get_symbol_cache()
        assert cache1 is cache2

    def test_reset_global_cache(self):
        """Test resetting the global cache."""
        cache1 = get_symbol_cache()
        cache1.put("/test.py", "hash", [{"name": "s"}])

        reset_global_cache()

        cache2 = get_symbol_cache()
        assert cache1 is not cache2
        assert cache2.size == 0

    def test_custom_config_on_first_call(self):
        """Test that config is applied on first call only."""
        cache1 = get_symbol_cache(max_entries=500)
        cache2 = get_symbol_cache(max_entries=1000)  # Ignored

        assert cache1._max_entries == 500
        assert cache1 is cache2  # Same instance
