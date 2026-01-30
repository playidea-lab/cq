"""Tests for Memory Manager."""

from pathlib import Path
from unittest.mock import MagicMock

import pytest

from c4.memory.manager import (
    MemoryManager,
    MemoryMetadata,
    MemoryStats,
    get_memory_manager,
    reset_global_manager,
)


class TestMemoryStats:
    """Tests for MemoryStats dataclass."""

    def test_default_values(self) -> None:
        """Should have zero defaults."""
        stats = MemoryStats()

        assert stats.total_memories == 0
        assert stats.total_size_bytes == 0
        assert stats.reads == 0
        assert stats.writes == 0
        assert stats.deletes == 0

    def test_to_dict(self) -> None:
        """Should convert to dictionary."""
        stats = MemoryStats(total_memories=5, reads=10, writes=3)
        result = stats.to_dict()

        assert result["total_memories"] == 5
        assert result["reads"] == 10
        assert result["writes"] == 3


class TestMemoryMetadata:
    """Tests for MemoryMetadata dataclass."""

    def test_to_dict(self, tmp_path: Path) -> None:
        """Should convert to dictionary."""
        from datetime import datetime, timezone

        now = datetime.now(tz=timezone.utc)
        metadata = MemoryMetadata(
            name="test",
            path=tmp_path / "test.md",
            size_bytes=100,
            content_hash="abc123",
            created_at=now,
            modified_at=now,
        )

        result = metadata.to_dict()

        assert result["name"] == "test"
        assert result["size_bytes"] == 100
        assert result["content_hash"] == "abc123"
        assert "created_at" in result
        assert "modified_at" in result


class TestMemoryManagerInit:
    """Tests for MemoryManager initialization."""

    def test_creates_memory_dir(self, tmp_path: Path) -> None:
        """Should create .c4/memories directory."""
        manager = MemoryManager(tmp_path)

        assert manager.memory_dir.exists()
        assert manager.memory_dir == tmp_path / ".c4" / "memories"

    def test_existing_dir_ok(self, tmp_path: Path) -> None:
        """Should handle existing directory."""
        memory_dir = tmp_path / ".c4" / "memories"
        memory_dir.mkdir(parents=True)

        manager = MemoryManager(tmp_path)

        assert manager.memory_dir.exists()


class TestMemoryManagerWrite:
    """Tests for MemoryManager.write()."""

    @pytest.fixture
    def manager(self, tmp_path: Path) -> MemoryManager:
        """Create a test manager."""
        return MemoryManager(tmp_path)

    def test_write_creates_file(self, manager: MemoryManager) -> None:
        """Should create markdown file."""
        path = manager.write("test-memory", "# Test Content")

        assert path.exists()
        assert path.suffix == ".md"
        assert path.read_text() == "# Test Content"

    def test_write_updates_stats(self, manager: MemoryManager) -> None:
        """Should update write stats."""
        manager.write("test", "content")

        assert manager.stats.writes == 1

    def test_write_skips_unchanged(self, manager: MemoryManager) -> None:
        """Should skip write if content unchanged."""
        manager.write("test", "content")
        manager.write("test", "content")  # Same content

        # Only one actual write (second is cache hit)
        assert manager.stats.writes == 1
        assert manager.stats.cache_hits == 1

    def test_write_updates_changed(self, manager: MemoryManager) -> None:
        """Should write if content changed."""
        manager.write("test", "content v1")
        manager.write("test", "content v2")  # Different content

        assert manager.stats.writes == 2

    def test_write_sanitizes_name(self, manager: MemoryManager) -> None:
        """Should sanitize memory name."""
        path = manager.write("my/memory name", "content")

        assert path.name == "my-memory-name.md"

    def test_write_rejects_empty_name(self, manager: MemoryManager) -> None:
        """Should reject empty name."""
        with pytest.raises(ValueError, match="cannot be empty"):
            manager.write("", "content")

    def test_write_rejects_too_long_name(self, manager: MemoryManager) -> None:
        """Should reject too long name."""
        long_name = "a" * 150

        with pytest.raises(ValueError, match="too long"):
            manager.write(long_name, "content")

    def test_write_rejects_invalid_chars(self, manager: MemoryManager) -> None:
        """Should reject invalid characters."""
        with pytest.raises(ValueError, match="invalid characters"):
            manager.write("test@memory!", "content")


class TestMemoryManagerRead:
    """Tests for MemoryManager.read()."""

    @pytest.fixture
    def manager(self, tmp_path: Path) -> MemoryManager:
        """Create a test manager."""
        return MemoryManager(tmp_path)

    def test_read_existing(self, manager: MemoryManager) -> None:
        """Should read existing memory."""
        manager.write("test", "# Content")

        content = manager.read("test")

        assert content == "# Content"

    def test_read_nonexistent(self, manager: MemoryManager) -> None:
        """Should return None for nonexistent."""
        content = manager.read("nonexistent")

        assert content is None

    def test_read_uses_cache(self, manager: MemoryManager) -> None:
        """Should use cache on second read."""
        manager.write("test", "content")
        manager.read("test")  # Populate cache
        manager.read("test")  # Cache hit

        assert manager.stats.cache_hits >= 1

    def test_read_updates_stats(self, manager: MemoryManager) -> None:
        """Should update read stats."""
        manager.write("test", "content")
        manager.read("test")
        manager.read("test")

        assert manager.stats.reads == 2


class TestMemoryManagerList:
    """Tests for MemoryManager.list()."""

    @pytest.fixture
    def manager(self, tmp_path: Path) -> MemoryManager:
        """Create a test manager."""
        return MemoryManager(tmp_path)

    def test_list_empty(self, manager: MemoryManager) -> None:
        """Should return empty list."""
        result = manager.list()

        assert result == []

    def test_list_memories(self, manager: MemoryManager) -> None:
        """Should list all memories."""
        manager.write("alpha", "content")
        manager.write("beta", "content")
        manager.write("gamma", "content")

        result = manager.list()

        assert result == ["alpha", "beta", "gamma"]

    def test_list_sorted(self, manager: MemoryManager) -> None:
        """Should return sorted list."""
        manager.write("zebra", "content")
        manager.write("apple", "content")

        result = manager.list()

        assert result == ["apple", "zebra"]


class TestMemoryManagerDelete:
    """Tests for MemoryManager.delete()."""

    @pytest.fixture
    def manager(self, tmp_path: Path) -> MemoryManager:
        """Create a test manager."""
        return MemoryManager(tmp_path)

    def test_delete_existing(self, manager: MemoryManager) -> None:
        """Should delete existing memory."""
        manager.write("test", "content")

        result = manager.delete("test")

        assert result is True
        assert not manager.exists("test")

    def test_delete_nonexistent(self, manager: MemoryManager) -> None:
        """Should return False for nonexistent."""
        result = manager.delete("nonexistent")

        assert result is False

    def test_delete_clears_cache(self, manager: MemoryManager) -> None:
        """Should clear cache on delete."""
        manager.write("test", "content")
        manager.read("test")  # Populate cache
        manager.delete("test")

        # Should not return cached value
        assert manager.read("test") is None

    def test_delete_updates_stats(self, manager: MemoryManager) -> None:
        """Should update delete stats."""
        manager.write("test", "content")
        manager.delete("test")

        assert manager.stats.deletes == 1


class TestMemoryManagerExists:
    """Tests for MemoryManager.exists()."""

    @pytest.fixture
    def manager(self, tmp_path: Path) -> MemoryManager:
        """Create a test manager."""
        return MemoryManager(tmp_path)

    def test_exists_true(self, manager: MemoryManager) -> None:
        """Should return True for existing."""
        manager.write("test", "content")

        assert manager.exists("test") is True

    def test_exists_false(self, manager: MemoryManager) -> None:
        """Should return False for nonexistent."""
        assert manager.exists("nonexistent") is False


class TestMemoryManagerMetadata:
    """Tests for MemoryManager.get_metadata()."""

    @pytest.fixture
    def manager(self, tmp_path: Path) -> MemoryManager:
        """Create a test manager."""
        return MemoryManager(tmp_path)

    def test_get_metadata_existing(self, manager: MemoryManager) -> None:
        """Should return metadata for existing memory."""
        manager.write("test", "# Test Content")

        metadata = manager.get_metadata("test")

        assert metadata is not None
        assert metadata.name == "test"
        assert metadata.size_bytes > 0
        assert metadata.content_hash is not None

    def test_get_metadata_nonexistent(self, manager: MemoryManager) -> None:
        """Should return None for nonexistent."""
        metadata = manager.get_metadata("nonexistent")

        assert metadata is None


class TestMemoryManagerSearch:
    """Tests for MemoryManager.search()."""

    @pytest.fixture
    def manager(self, tmp_path: Path) -> MemoryManager:
        """Create a test manager."""
        return MemoryManager(tmp_path)

    def test_search_pattern(self, manager: MemoryManager) -> None:
        """Should find matching memories."""
        manager.write("adr-001-database", "content")
        manager.write("adr-002-auth", "content")
        manager.write("pattern-observer", "content")

        result = manager.search("adr")

        assert "adr-001-database" in result
        assert "adr-002-auth" in result
        assert "pattern-observer" not in result

    def test_search_no_match(self, manager: MemoryManager) -> None:
        """Should return empty for no match."""
        manager.write("test", "content")

        result = manager.search("xyz")

        assert result == []


class TestMemoryManagerCache:
    """Tests for cache operations."""

    @pytest.fixture
    def manager(self, tmp_path: Path) -> MemoryManager:
        """Create a test manager."""
        return MemoryManager(tmp_path)

    def test_clear_cache(self, manager: MemoryManager) -> None:
        """Should clear cache."""
        manager.write("test1", "content1")
        manager.write("test2", "content2")
        manager.read("test1")
        manager.read("test2")

        count = manager.clear_cache()

        assert count == 2

    def test_get_status(self, manager: MemoryManager) -> None:
        """Should return status dict."""
        manager.write("test", "content")

        status = manager.get_status()

        assert "memory_dir" in status
        assert "hub_connected" in status
        assert "total_memories" in status
        assert status["total_memories"] == 1


class TestMemoryManagerHub:
    """Tests for Hub synchronization."""

    @pytest.fixture
    def mock_hub(self) -> MagicMock:
        """Create mock Hub client."""
        hub = MagicMock()
        hub.upload_memory.return_value = True
        hub.download_memory.return_value = None
        hub.delete_memory.return_value = True
        return hub

    def test_write_syncs_to_hub(self, tmp_path: Path, mock_hub: MagicMock) -> None:
        """Should sync write to Hub."""
        manager = MemoryManager(tmp_path, hub_client=mock_hub)

        manager.write("test", "content")

        mock_hub.upload_memory.assert_called_once_with("test", "content")

    def test_write_no_sync_option(self, tmp_path: Path, mock_hub: MagicMock) -> None:
        """Should skip sync when sync=False."""
        manager = MemoryManager(tmp_path, hub_client=mock_hub)

        manager.write("test", "content", sync=False)

        mock_hub.upload_memory.assert_not_called()

    def test_read_fallback_to_hub(self, tmp_path: Path, mock_hub: MagicMock) -> None:
        """Should fallback to Hub on local miss."""
        mock_hub.download_memory.return_value = "hub content"
        manager = MemoryManager(tmp_path, hub_client=mock_hub)

        content = manager.read("remote-only")

        assert content == "hub content"
        mock_hub.download_memory.assert_called_once()

    def test_delete_syncs_to_hub(self, tmp_path: Path, mock_hub: MagicMock) -> None:
        """Should sync delete to Hub."""
        manager = MemoryManager(tmp_path, hub_client=mock_hub)
        manager.write("test", "content", sync=False)

        manager.delete("test")

        mock_hub.delete_memory.assert_called_once_with("test")


class TestMemoryManagerSanitization:
    """Tests for name sanitization."""

    @pytest.fixture
    def manager(self, tmp_path: Path) -> MemoryManager:
        """Create a test manager."""
        return MemoryManager(tmp_path)

    def test_sanitize_slashes(self, manager: MemoryManager) -> None:
        """Should replace slashes with dashes."""
        path = manager.write("path/to/memory", "content")

        assert path.name == "path-to-memory.md"

    def test_sanitize_spaces(self, manager: MemoryManager) -> None:
        """Should replace spaces with dashes."""
        path = manager.write("my memory name", "content")

        assert path.name == "my-memory-name.md"

    def test_sanitize_consecutive_dashes(self, manager: MemoryManager) -> None:
        """Should collapse consecutive dashes."""
        path = manager.write("a--b---c", "content")

        assert path.name == "a-b-c.md"

    def test_sanitize_leading_trailing_dashes(self, manager: MemoryManager) -> None:
        """Should strip leading/trailing dashes."""
        path = manager.write("-test-", "content")

        assert path.name == "test.md"


class TestGlobalManager:
    """Tests for global manager functions."""

    def teardown_method(self) -> None:
        """Reset global manager after each test."""
        reset_global_manager()

    def test_get_manager_creates(self, tmp_path: Path) -> None:
        """Should create manager on first call."""
        manager = get_memory_manager(tmp_path)

        assert manager is not None
        assert manager.memory_dir.exists()

    def test_get_manager_singleton(self, tmp_path: Path) -> None:
        """Should return same instance."""
        manager1 = get_memory_manager(tmp_path)
        manager2 = get_memory_manager()

        assert manager1 is manager2

    def test_get_manager_requires_path_first(self) -> None:
        """Should require path on first call."""
        with pytest.raises(ValueError, match="project_path required"):
            get_memory_manager()

    def test_reset_global_manager(self, tmp_path: Path) -> None:
        """Should reset global manager."""
        manager1 = get_memory_manager(tmp_path)
        reset_global_manager()
        manager2 = get_memory_manager(tmp_path)

        assert manager1 is not manager2


class TestMemoryManagerConcurrency:
    """Tests for thread-safety."""

    def test_concurrent_writes(self, tmp_path: Path) -> None:
        """Should handle concurrent writes."""
        import threading

        manager = MemoryManager(tmp_path)
        results: list[Path] = []
        errors: list[Exception] = []

        def write_memory(name: str) -> None:
            try:
                path = manager.write(name, f"content for {name}")
                results.append(path)
            except Exception as e:
                errors.append(e)

        threads = [
            threading.Thread(target=write_memory, args=(f"memory-{i}",))
            for i in range(10)
        ]

        for t in threads:
            t.start()
        for t in threads:
            t.join()

        assert len(errors) == 0
        assert len(results) == 10
        assert len(manager.list()) == 10

    def test_concurrent_reads(self, tmp_path: Path) -> None:
        """Should handle concurrent reads."""
        import threading

        manager = MemoryManager(tmp_path)
        manager.write("shared", "shared content")
        results: list[str | None] = []

        def read_memory() -> None:
            content = manager.read("shared")
            results.append(content)

        threads = [threading.Thread(target=read_memory) for _ in range(10)]

        for t in threads:
            t.start()
        for t in threads:
            t.join()

        assert all(r == "shared content" for r in results)
