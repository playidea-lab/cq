"""Tests for Supabase StateStore."""

from datetime import datetime, timedelta
from unittest.mock import MagicMock

import pytest

from c4.models import C4State
from c4.store.exceptions import StateNotFoundError
from c4.store.supabase import SupabaseStateStore, create_supabase_store


class TestSupabaseStateStoreInit:
    """Test SupabaseStateStore initialization."""

    def test_init_with_params(self) -> None:
        """Test initialization with explicit parameters."""
        store = SupabaseStateStore(
            url="https://test.supabase.co",
            key="test-key",
            realtime=False,
        )

        assert store._url == "https://test.supabase.co"
        assert store._key == "test-key"
        assert store._realtime_enabled is False
        assert store._client is None

    def test_init_from_env(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test initialization from environment variables."""
        monkeypatch.setenv("SUPABASE_URL", "https://env.supabase.co")
        monkeypatch.setenv("SUPABASE_KEY", "env-key")

        store = SupabaseStateStore()

        assert store._url == "https://env.supabase.co"
        assert store._key == "env-key"

    def test_client_requires_credentials(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test that accessing client without credentials raises error."""
        # Clear environment variables to ensure no fallback
        monkeypatch.delenv("SUPABASE_URL", raising=False)
        monkeypatch.delenv("SUPABASE_KEY", raising=False)

        store = SupabaseStateStore(url="", key="")

        with pytest.raises(ValueError, match="URL and key required"):
            _ = store.client


class TestSupabaseStateStoreOperations:
    """Test StateStore protocol implementation."""

    @pytest.fixture
    def mock_client(self) -> MagicMock:
        """Create mock Supabase client."""
        return MagicMock()

    @pytest.fixture
    def store(self, mock_client: MagicMock) -> SupabaseStateStore:
        """Create store with mock client."""
        store = SupabaseStateStore(
            url="https://test.supabase.co",
            key="test-key",
        )
        store._client = mock_client
        return store

    def test_load_success(self, store: SupabaseStateStore) -> None:
        """Test loading existing state."""
        state_data = {
            "project_id": "test-project",
            "status": "EXECUTE",
            "execution_mode": "running",
            "state_data": {
                "project_id": "test-project",
                "status": "EXECUTE",
                "execution_mode": "running",
                "checkpoint": {"current": None, "state": "pending"},
                "queue": {"pending": [], "in_progress": {}, "done": []},
                "workers": {},
                "locks": {"leader": None, "scopes": {}},
                "metrics": {
                    "events_emitted": 0,
                    "validations_run": 0,
                    "tasks_completed": 0,
                    "checkpoints_passed": 0,
                },
                "passed_checkpoints": [],
                "checkpoint_queue": [],
                "repair_queue": [],
                "created_at": "2024-01-01T00:00:00",
                "updated_at": "2024-01-01T00:00:00",
            },
            "created_at": "2024-01-01T00:00:00",
            "updated_at": "2024-01-01T00:00:00",
        }

        mock_response = MagicMock()
        mock_response.data = state_data
        store._client.table.return_value.select.return_value.eq.return_value.maybe_single.return_value.execute.return_value = mock_response

        state = store.load("test-project")

        assert state.project_id == "test-project"
        assert state.status.value == "EXECUTE"

    def test_load_not_found(self, store: SupabaseStateStore) -> None:
        """Test loading non-existent state."""
        mock_response = MagicMock()
        mock_response.data = None
        store._client.table.return_value.select.return_value.eq.return_value.maybe_single.return_value.execute.return_value = mock_response

        with pytest.raises(StateNotFoundError):
            store.load("nonexistent")

    def test_save(self, store: SupabaseStateStore) -> None:
        """Test saving state."""
        state = C4State(project_id="test-project")

        store.save(state)

        store._client.table.assert_called_with("c4_state")
        store._client.table.return_value.upsert.assert_called_once()

    def test_exists_true(self, store: SupabaseStateStore) -> None:
        """Test exists returns True for existing project."""
        mock_response = MagicMock()
        mock_response.data = {"project_id": "test"}
        store._client.table.return_value.select.return_value.eq.return_value.maybe_single.return_value.execute.return_value = mock_response

        assert store.exists("test") is True

    def test_exists_false(self, store: SupabaseStateStore) -> None:
        """Test exists returns False for non-existent project."""
        mock_response = MagicMock()
        mock_response.data = None
        store._client.table.return_value.select.return_value.eq.return_value.maybe_single.return_value.execute.return_value = mock_response

        assert store.exists("nonexistent") is False

    def test_delete(self, store: SupabaseStateStore) -> None:
        """Test deleting state."""
        store.delete("test-project")

        store._client.table.return_value.delete.return_value.eq.assert_called_with(
            "project_id", "test-project"
        )


class TestSupabaseLockStore:
    """Test LockStore protocol implementation."""

    @pytest.fixture
    def store(self) -> SupabaseStateStore:
        """Create store with mock client."""
        store = SupabaseStateStore(
            url="https://test.supabase.co",
            key="test-key",
        )
        store._client = MagicMock()
        return store

    def test_acquire_scope_lock(self, store: SupabaseStateStore) -> None:
        """Test acquiring a scope lock."""
        result = store.acquire_scope_lock(
            project_id="test",
            scope="src/api",
            owner="worker-1",
            ttl_seconds=300,
        )

        assert result is True
        store._client.table.assert_called_with("c4_locks")

    def test_release_scope_lock(self, store: SupabaseStateStore) -> None:
        """Test releasing a scope lock."""
        result = store.release_scope_lock("test", "src/api")

        assert result is True
        store._client.table.return_value.delete.assert_called()

    def test_refresh_scope_lock(self, store: SupabaseStateStore) -> None:
        """Test refreshing lock TTL."""
        mock_response = MagicMock()
        mock_response.data = [{"scope": "src/api"}]
        store._client.table.return_value.update.return_value.eq.return_value.eq.return_value.eq.return_value.execute.return_value = mock_response

        result = store.refresh_scope_lock("test", "src/api", "worker-1", 300)

        assert result is True

    def test_get_scope_lock(self, store: SupabaseStateStore) -> None:
        """Test getting lock info."""
        expires = datetime.now() + timedelta(hours=1)
        mock_response = MagicMock()
        mock_response.data = {
            "owner": "worker-1",
            "expires_at": expires.isoformat(),
        }
        store._client.table.return_value.select.return_value.eq.return_value.eq.return_value.maybe_single.return_value.execute.return_value = mock_response

        result = store.get_scope_lock("test", "src/api")

        assert result is not None
        assert result[0] == "worker-1"

    def test_get_scope_lock_not_found(self, store: SupabaseStateStore) -> None:
        """Test getting non-existent lock."""
        mock_response = MagicMock()
        mock_response.data = None
        store._client.table.return_value.select.return_value.eq.return_value.eq.return_value.maybe_single.return_value.execute.return_value = mock_response

        result = store.get_scope_lock("test", "src/api")

        assert result is None

    def test_cleanup_expired(self, store: SupabaseStateStore) -> None:
        """Test cleaning up expired locks."""
        mock_response = MagicMock()
        mock_response.data = [{"scope": "old-scope"}]
        store._client.table.return_value.select.return_value.eq.return_value.lt.return_value.execute.return_value = mock_response

        result = store.cleanup_expired("test")

        assert result == ["old-scope"]


class TestSupabaseRealtime:
    """Test real-time subscription features."""

    @pytest.fixture
    def store(self) -> SupabaseStateStore:
        """Create store with mock client."""
        store = SupabaseStateStore(
            url="https://test.supabase.co",
            key="test-key",
            realtime=True,
        )
        store._client = MagicMock()
        return store

    def test_subscribe(self, store: SupabaseStateStore) -> None:
        """Test subscribing to state changes."""
        callback = MagicMock()
        mock_channel = MagicMock()
        mock_channel.on_postgres_changes.return_value = mock_channel
        mock_channel.subscribe.return_value = mock_channel
        store._client.channel.return_value = mock_channel

        sub_id = store.subscribe("test-project", callback)

        assert sub_id.startswith("test-project:")
        assert "test-project" in store._callbacks
        assert callback in store._callbacks["test-project"]

    def test_unsubscribe(self, store: SupabaseStateStore) -> None:
        """Test unsubscribing from state changes."""
        callback = MagicMock()
        mock_channel = MagicMock()
        mock_channel.on_postgres_changes.return_value = mock_channel
        mock_channel.subscribe.return_value = mock_channel
        store._client.channel.return_value = mock_channel

        sub_id = store.subscribe("test-project", callback)
        store.unsubscribe(sub_id)

        assert "test-project" not in store._callbacks

    def test_realtime_disabled(self) -> None:
        """Test that subscribe fails when realtime disabled."""
        store = SupabaseStateStore(
            url="https://test.supabase.co",
            key="test-key",
            realtime=False,
        )

        with pytest.raises(RuntimeError, match="Real-time disabled"):
            store.subscribe("test", MagicMock())


class TestFactoryFunction:
    """Test create_supabase_store factory."""

    def test_create_with_defaults(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Test factory with environment variables."""
        monkeypatch.setenv("SUPABASE_URL", "https://factory.supabase.co")
        monkeypatch.setenv("SUPABASE_KEY", "factory-key")

        store = create_supabase_store()

        assert isinstance(store, SupabaseStateStore)
        assert store._url == "https://factory.supabase.co"

    def test_create_with_params(self) -> None:
        """Test factory with explicit parameters."""
        store = create_supabase_store(
            url="https://custom.supabase.co",
            key="custom-key",
            realtime=False,
        )

        assert store._url == "https://custom.supabase.co"
        assert store._realtime_enabled is False
