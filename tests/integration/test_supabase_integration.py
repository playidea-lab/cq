"""Supabase Integration Tests

These tests require Supabase credentials:
- SUPABASE_URL: Supabase project URL
- SUPABASE_KEY: Supabase anon key

Run with:
    SUPABASE_URL=https://xxx.supabase.co SUPABASE_KEY=your-key pytest tests/integration/test_supabase_integration.py -v
"""

import os
import uuid
from datetime import datetime

import pytest

# Skip all tests if Supabase credentials not available
pytestmark = pytest.mark.skipif(
    not os.environ.get("SUPABASE_URL") or not os.environ.get("SUPABASE_KEY"),
    reason="Supabase credentials not available (SUPABASE_URL, SUPABASE_KEY)",
)


@pytest.fixture
def project_id():
    """Generate unique project ID for test isolation"""
    return f"test-{uuid.uuid4().hex[:8]}"


@pytest.fixture
def supabase_store():
    """Create Supabase store instance"""
    from c4.store import SupabaseStateStore

    return SupabaseStateStore()


class TestSupabaseStateStore:
    """Test SupabaseStateStore CRUD operations"""

    def test_save_and_load(self, supabase_store, project_id):
        """Test basic save and load"""
        from c4.models import C4State, ProjectStatus

        # Create state
        state = C4State(
            project_id=project_id,
            status=ProjectStatus.INIT,
        )

        # Save
        supabase_store.save(state)

        # Load
        loaded = supabase_store.load(project_id)
        assert loaded.project_id == project_id
        assert loaded.status == ProjectStatus.INIT

        # Cleanup
        supabase_store.delete(project_id)

    def test_exists(self, supabase_store, project_id):
        """Test exists check"""
        from c4.models import C4State, ProjectStatus

        # Initially doesn't exist
        assert not supabase_store.exists(project_id)

        # Create
        state = C4State(project_id=project_id, status=ProjectStatus.INIT)
        supabase_store.save(state)

        # Now exists
        assert supabase_store.exists(project_id)

        # Cleanup
        supabase_store.delete(project_id)
        assert not supabase_store.exists(project_id)

    def test_load_not_found(self, supabase_store):
        """Test load raises StateNotFoundError for missing project"""
        from c4.store.exceptions import StateNotFoundError

        with pytest.raises(StateNotFoundError):
            supabase_store.load("nonexistent-project-12345")

    def test_update_state(self, supabase_store, project_id):
        """Test state update (upsert)"""
        from c4.models import C4State, ProjectStatus

        # Create initial state
        state = C4State(project_id=project_id, status=ProjectStatus.INIT)
        supabase_store.save(state)

        # Update state
        state.status = ProjectStatus.DISCOVERY
        supabase_store.save(state)

        # Verify update
        loaded = supabase_store.load(project_id)
        assert loaded.status == ProjectStatus.DISCOVERY

        # Cleanup
        supabase_store.delete(project_id)


class TestSupabaseAtomicModify:
    """Test atomic modification with optimistic locking"""

    def test_atomic_modify_success(self, supabase_store, project_id):
        """Test successful atomic modification"""
        from c4.models import C4State, ProjectStatus

        # Create initial state
        state = C4State(project_id=project_id, status=ProjectStatus.INIT)
        supabase_store.save(state)

        # Atomic modify
        with supabase_store.atomic_modify(project_id) as state:
            state.status = ProjectStatus.DISCOVERY
            state.metrics.events_emitted = 5

        # Verify
        loaded = supabase_store.load(project_id)
        assert loaded.status == ProjectStatus.DISCOVERY
        assert loaded.metrics.events_emitted == 5

        # Cleanup
        supabase_store.delete(project_id)

    def test_atomic_modify_not_found(self, supabase_store):
        """Test atomic_modify raises StateNotFoundError"""
        from c4.store.exceptions import StateNotFoundError

        with pytest.raises(StateNotFoundError):
            with supabase_store.atomic_modify("nonexistent-12345"):
                pass


class TestSupabaseLockStore:
    """Test SupabaseStateStore lock operations (it implements both protocols)"""

    def test_acquire_and_release_scope_lock(self, supabase_store, project_id):
        """Test scope lock acquire and release"""
        from c4.models import C4State, ProjectStatus

        # Create project first
        state = C4State(project_id=project_id, status=ProjectStatus.EXECUTE)
        supabase_store.save(state)

        # Acquire lock
        result = supabase_store.acquire_scope_lock(
            project_id=project_id,
            scope="src/backend",
            owner="worker-1",
            ttl_seconds=300,
        )
        assert result is True

        # Check lock
        lock_info = supabase_store.get_scope_lock(project_id, "src/backend")
        assert lock_info is not None
        owner, expires_at = lock_info
        assert owner == "worker-1"
        assert expires_at > datetime.now()

        # Release lock
        result = supabase_store.release_scope_lock(project_id, "src/backend")
        assert result is True

        # Verify released
        lock_info = supabase_store.get_scope_lock(project_id, "src/backend")
        assert lock_info is None

        # Cleanup
        supabase_store.delete(project_id)

    def test_lock_refresh(self, supabase_store, project_id):
        """Test lock TTL refresh"""
        from c4.models import C4State, ProjectStatus

        # Create project
        state = C4State(project_id=project_id, status=ProjectStatus.EXECUTE)
        supabase_store.save(state)

        # Acquire lock
        supabase_store.acquire_scope_lock(
            project_id=project_id,
            scope="src/api",
            owner="worker-1",
            ttl_seconds=60,
        )

        # Refresh with same owner
        result = supabase_store.refresh_scope_lock(
            project_id=project_id,
            scope="src/api",
            owner="worker-1",
            ttl_seconds=300,
        )
        assert result is True

        # Refresh with different owner fails
        result = supabase_store.refresh_scope_lock(
            project_id=project_id,
            scope="src/api",
            owner="worker-2",
            ttl_seconds=300,
        )
        assert result is False

        # Cleanup
        supabase_store.release_scope_lock(project_id, "src/api")
        supabase_store.delete(project_id)


class TestStoreFactory:
    """Test store factory with Supabase backend"""

    def test_create_supabase_store_from_env(self):
        """Test factory creates Supabase store from environment"""
        import os
        from pathlib import Path

        from c4.store import create_state_store

        # Set backend env
        old_backend = os.environ.get("C4_STORE_BACKEND")
        try:
            os.environ["C4_STORE_BACKEND"] = "supabase"
            store = create_state_store(Path("/tmp/test-c4"))

            # Should be Supabase store
            assert "Supabase" in type(store).__name__
        finally:
            if old_backend:
                os.environ["C4_STORE_BACKEND"] = old_backend
            else:
                os.environ.pop("C4_STORE_BACKEND", None)

    def test_create_supabase_store_from_config(self, tmp_path):
        """Test factory creates Supabase store from config"""
        from c4.models.config import StoreConfig
        from c4.store import create_state_store

        config = StoreConfig(
            backend="supabase",
            supabase_url=os.environ.get("SUPABASE_URL"),
            supabase_key=os.environ.get("SUPABASE_KEY"),
        )

        store = create_state_store(tmp_path, config)
        assert "Supabase" in type(store).__name__
