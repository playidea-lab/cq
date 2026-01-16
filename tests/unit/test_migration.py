"""Unit tests for SQLite-Supabase Migration Utilities."""

from __future__ import annotations

import json
import sqlite3
from datetime import datetime, timedelta
from pathlib import Path

import pytest

from c4.store.exceptions import MigrationError
from c4.store.migration import (
    ExportData,
    MigrationManager,
    MigrationSnapshot,
    migrate_local_to_team,
    migrate_team_to_local,
)


@pytest.fixture
def temp_db(tmp_path: Path) -> Path:
    """Create a temporary SQLite database with schema."""
    db_path = tmp_path / "c4.db"

    conn = sqlite3.connect(db_path)
    conn.execute("PRAGMA journal_mode=WAL")

    # Create tables
    conn.execute("""
        CREATE TABLE c4_state (
            project_id TEXT PRIMARY KEY,
            state_json TEXT NOT NULL,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )
    """)

    conn.execute("""
        CREATE TABLE c4_tasks (
            project_id TEXT NOT NULL,
            task_id TEXT NOT NULL,
            task_json TEXT NOT NULL,
            status TEXT DEFAULT 'pending',
            assigned_to TEXT,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            PRIMARY KEY (project_id, task_id)
        )
    """)

    conn.execute("""
        CREATE TABLE c4_locks (
            project_id TEXT NOT NULL,
            scope TEXT NOT NULL,
            owner TEXT NOT NULL,
            expires_at TIMESTAMP NOT NULL,
            PRIMARY KEY (project_id, scope)
        )
    """)

    conn.commit()
    conn.close()

    return db_path


@pytest.fixture
def populated_db(temp_db: Path) -> Path:
    """Create a database with sample data."""
    conn = sqlite3.connect(temp_db)

    # Insert state
    state_data = {
        "status": "execute",
        "current_phase": "planning",
        "config": {"domain": "web-backend"},
    }
    conn.execute(
        "INSERT INTO c4_state (project_id, state_json) VALUES (?, ?)",
        ("test-project", json.dumps(state_data)),
    )

    # Insert tasks
    tasks = [
        {"id": "T-001", "title": "Task 1", "status": "completed"},
        {"id": "T-002", "title": "Task 2", "status": "in_progress", "assigned_to": "worker-1"},
        {"id": "T-003", "title": "Task 3", "status": "pending"},
    ]
    for task in tasks:
        conn.execute(
            """INSERT INTO c4_tasks
               (project_id, task_id, task_json, status, assigned_to)
               VALUES (?, ?, ?, ?, ?)""",
            (
                "test-project",
                task["id"],
                json.dumps(task),
                task["status"],
                task.get("assigned_to"),
            ),
        )

    # Insert locks
    expires_at = datetime.now() + timedelta(hours=1)
    conn.execute(
        "INSERT INTO c4_locks (project_id, scope, owner, expires_at) VALUES (?, ?, ?, ?)",
        ("test-project", "src/main.py", "worker-1", expires_at),
    )

    conn.commit()
    conn.close()

    return temp_db


class TestMigrationSnapshot:
    """Tests for MigrationSnapshot dataclass."""

    def test_to_dict(self):
        """Test serialization to dictionary."""
        snapshot = MigrationSnapshot(
            snapshot_id="import-20250116-120000",
            source="team",
            target="local",
            timestamp="2025-01-16T12:00:00",
            backup_path="/backups/pre-import.db",
            state_data={"key": "value"},
            tasks_count=5,
            locks_count=2,
            status="completed",
        )

        result = snapshot.to_dict()

        assert result["snapshot_id"] == "import-20250116-120000"
        assert result["source"] == "team"
        assert result["target"] == "local"
        assert result["backup_path"] == "/backups/pre-import.db"
        assert result["state_data"] == {"key": "value"}
        assert result["tasks_count"] == 5
        assert result["locks_count"] == 2
        assert result["status"] == "completed"

    def test_from_dict(self):
        """Test deserialization from dictionary."""
        data = {
            "snapshot_id": "export-20250116-130000",
            "source": "local",
            "target": "team",
            "timestamp": "2025-01-16T13:00:00",
            "backup_path": None,
            "state_data": None,
            "tasks_count": 3,
            "locks_count": 1,
            "status": "pending",
        }

        snapshot = MigrationSnapshot.from_dict(data)

        assert snapshot.snapshot_id == "export-20250116-130000"
        assert snapshot.source == "local"
        assert snapshot.target == "team"
        assert snapshot.backup_path is None
        assert snapshot.tasks_count == 3
        assert snapshot.status == "pending"

    def test_from_dict_with_defaults(self):
        """Test deserialization with missing optional fields."""
        data = {
            "snapshot_id": "test-001",
            "source": "local",
            "target": "team",
            "timestamp": "2025-01-16T14:00:00",
        }

        snapshot = MigrationSnapshot.from_dict(data)

        assert snapshot.backup_path is None
        assert snapshot.state_data is None
        assert snapshot.tasks_count == 0
        assert snapshot.locks_count == 0
        assert snapshot.status == "pending"


class TestExportData:
    """Tests for ExportData dataclass."""

    def test_to_dict(self):
        """Test serialization to dictionary."""
        export = ExportData(
            project_id="my-project",
            exported_at="2025-01-16T12:00:00",
            version="1.0",
            state={"status": "execute"},
            tasks=[{"id": "T-001", "title": "Task 1"}],
            locks=[{"scope": "src/main.py", "owner": "worker-1"}],
        )

        result = export.to_dict()

        assert result["project_id"] == "my-project"
        assert result["version"] == "1.0"
        assert len(result["tasks"]) == 1
        assert len(result["locks"]) == 1

    def test_from_dict(self):
        """Test deserialization from dictionary."""
        data = {
            "project_id": "test-project",
            "exported_at": "2025-01-16T13:00:00",
            "version": "1.0",
            "state": {"status": "planning"},
            "tasks": [{"id": "T-001"}],
            "locks": [],
        }

        export = ExportData.from_dict(data)

        assert export.project_id == "test-project"
        assert export.version == "1.0"
        assert len(export.tasks) == 1
        assert len(export.locks) == 0

    def test_from_dict_with_defaults(self):
        """Test deserialization with missing optional fields."""
        data = {
            "project_id": "minimal",
            "exported_at": "2025-01-16T14:00:00",
            "state": {"status": "init"},
        }

        export = ExportData.from_dict(data)

        assert export.version == "1.0"  # Default version
        assert export.tasks == []
        assert export.locks == []


class TestMigrationManager:
    """Tests for MigrationManager class."""

    def test_init_creates_backup_dir(self, temp_db: Path, tmp_path: Path):
        """Test that manager initializes with backup directory."""
        backup_dir = tmp_path / "custom_backups"

        manager = MigrationManager(temp_db, backup_dir=backup_dir)

        assert manager.db_path == temp_db
        assert manager.backup_dir == backup_dir

    def test_export_to_team(self, populated_db: Path):
        """Test exporting data for team migration."""
        manager = MigrationManager(populated_db)

        export = manager.export_to_team("test-project")

        assert export.project_id == "test-project"
        assert export.version == "1.0"
        assert export.state["status"] == "execute"
        assert len(export.tasks) == 3
        assert len(export.locks) == 1

    def test_export_to_team_project_not_found(self, temp_db: Path):
        """Test export fails for non-existent project."""
        manager = MigrationManager(temp_db)

        with pytest.raises(MigrationError) as exc:
            manager.export_to_team("non-existent")

        assert "No state found" in str(exc.value)

    def test_export_to_file(self, populated_db: Path, tmp_path: Path):
        """Test exporting data to a JSON file."""
        manager = MigrationManager(populated_db)
        output_path = tmp_path / "export.json"

        manager.export_to_file("test-project", output_path)

        assert output_path.exists()
        data = json.loads(output_path.read_text())
        assert data["project_id"] == "test-project"
        assert len(data["tasks"]) == 3

    def test_import_from_team(self, temp_db: Path):
        """Test importing team data to local database."""
        manager = MigrationManager(temp_db)

        export_data = ExportData(
            project_id="imported-project",
            exported_at="2025-01-16T12:00:00",
            version="1.0",
            state={"status": "planning", "phase": "design"},
            tasks=[
                {"id": "T-001", "title": "Task 1", "status": "pending"},
                {"id": "T-002", "title": "Task 2", "status": "completed"},
            ],
            locks=[
                {"scope": "src/api.py", "owner": "worker-2", "expires_at": "2025-01-17T12:00:00"},
            ],
        )

        snapshot = manager.import_from_team(export_data, create_backup=False)

        assert snapshot.status == "completed"
        assert snapshot.tasks_count == 2
        assert snapshot.locks_count == 1
        assert snapshot.source == "team"
        assert snapshot.target == "local"

        # Verify data was imported
        conn = sqlite3.connect(temp_db)
        cursor = conn.execute(
            "SELECT state_json FROM c4_state WHERE project_id = ?",
            ("imported-project",),
        )
        row = cursor.fetchone()
        assert row is not None
        state = json.loads(row[0])
        assert state["status"] == "planning"

        cursor = conn.execute(
            "SELECT COUNT(*) FROM c4_tasks WHERE project_id = ?",
            ("imported-project",),
        )
        assert cursor.fetchone()[0] == 2
        conn.close()

    def test_import_from_file(self, temp_db: Path, tmp_path: Path):
        """Test importing from JSON file."""
        manager = MigrationManager(temp_db)

        # Create export file
        export_data = {
            "project_id": "file-import",
            "exported_at": "2025-01-16T12:00:00",
            "version": "1.0",
            "state": {"status": "execute"},
            "tasks": [{"id": "T-001", "title": "Task 1", "status": "pending"}],
            "locks": [],
        }
        input_path = tmp_path / "import.json"
        input_path.write_text(json.dumps(export_data))

        snapshot = manager.import_from_file(input_path, create_backup=False)

        assert snapshot.status == "completed"
        assert snapshot.tasks_count == 1

    def test_import_from_file_not_found(self, temp_db: Path, tmp_path: Path):
        """Test import fails for non-existent file."""
        manager = MigrationManager(temp_db)
        input_path = tmp_path / "nonexistent.json"

        with pytest.raises(MigrationError) as exc:
            manager.import_from_file(input_path)

        assert "not found" in str(exc.value)

    def test_create_backup(self, populated_db: Path):
        """Test creating database backup."""
        manager = MigrationManager(populated_db)

        backup_path = manager.create_backup("test-backup")

        assert backup_path.exists()
        assert backup_path.name == "test-backup.db"

        # Verify backup is valid SQLite database
        conn = sqlite3.connect(backup_path)
        cursor = conn.execute("SELECT COUNT(*) FROM c4_state")
        assert cursor.fetchone()[0] == 1
        conn.close()

    def test_create_backup_with_timestamp(self, populated_db: Path):
        """Test backup creates timestamped name by default."""
        manager = MigrationManager(populated_db)

        backup_path = manager.create_backup()

        assert backup_path.exists()
        # Name should be timestamp format: YYYYMMDD-HHMMSS.db
        assert len(backup_path.stem) == 15  # e.g., "20250116-120000"

    def test_create_backup_db_not_found(self, tmp_path: Path):
        """Test backup fails for non-existent database."""
        db_path = tmp_path / "nonexistent.db"
        manager = MigrationManager(db_path)

        with pytest.raises(MigrationError) as exc:
            manager.create_backup()

        assert "not found" in str(exc.value)

    def test_rollback(self, populated_db: Path):
        """Test rollback to previous state."""
        manager = MigrationManager(populated_db)

        # Create a backup
        manager.create_backup("pre-change")

        # Make a change
        conn = sqlite3.connect(populated_db)
        conn.execute(
            "UPDATE c4_state SET state_json = ? WHERE project_id = ?",
            (json.dumps({"status": "changed"}), "test-project"),
        )
        conn.commit()
        conn.close()

        # Import with backup
        export_data = ExportData(
            project_id="test-project",
            exported_at="2025-01-16T12:00:00",
            version="1.0",
            state={"status": "after-import"},
            tasks=[],
            locks=[],
        )
        snapshot = manager.import_from_team(export_data, create_backup=True)

        # Rollback
        result = manager.rollback(snapshot.snapshot_id)

        assert result is True

        # Verify state was restored
        conn = sqlite3.connect(populated_db)
        cursor = conn.execute(
            "SELECT state_json FROM c4_state WHERE project_id = ?",
            ("test-project",),
        )
        row = cursor.fetchone()
        state = json.loads(row[0])
        # Should be the changed state (pre-import backup)
        assert state["status"] == "changed"
        conn.close()

    def test_rollback_no_snapshot(self, populated_db: Path):
        """Test rollback fails when no valid snapshot exists."""
        manager = MigrationManager(populated_db)

        with pytest.raises(MigrationError) as exc:
            manager.rollback()

        assert "No valid snapshots" in str(exc.value)

    def test_list_backups(self, populated_db: Path):
        """Test listing available backups."""
        manager = MigrationManager(populated_db)

        # Create some backups
        manager.create_backup("backup-1")
        manager.create_backup("backup-2")

        backups = manager.list_backups()

        assert len(backups) == 2
        # Should be sorted by created_at descending
        assert backups[0]["name"] == "backup-2"
        assert backups[1]["name"] == "backup-1"

    def test_list_snapshots(self, temp_db: Path):
        """Test listing migration snapshots."""
        manager = MigrationManager(temp_db)

        # Import some data to create snapshots
        export_data = ExportData(
            project_id="test",
            exported_at="2025-01-16T12:00:00",
            version="1.0",
            state={"status": "test"},
            tasks=[],
            locks=[],
        )
        manager.import_from_team(export_data, create_backup=False)

        snapshots = manager.list_snapshots()

        assert len(snapshots) == 1
        assert snapshots[0].status == "completed"

    def test_cleanup_old_backups(self, populated_db: Path):
        """Test cleanup of old backups."""
        manager = MigrationManager(populated_db)

        # Create several backups
        for i in range(7):
            manager.create_backup(f"backup-{i:02d}")

        # Should have 7 backups
        assert len(manager.list_backups()) == 7

        # Cleanup keeping only 3
        removed = manager.cleanup_old_backups(keep_count=3)

        assert removed == 4
        assert len(manager.list_backups()) == 3

    def test_cleanup_old_backups_nothing_to_remove(self, populated_db: Path):
        """Test cleanup when fewer backups than keep_count."""
        manager = MigrationManager(populated_db)

        manager.create_backup("backup-1")
        manager.create_backup("backup-2")

        removed = manager.cleanup_old_backups(keep_count=5)

        assert removed == 0
        assert len(manager.list_backups()) == 2


class TestConvenienceFunctions:
    """Tests for module-level convenience functions."""

    def test_migrate_local_to_team(self, populated_db: Path):
        """Test convenience function for local to team migration."""
        export = migrate_local_to_team(populated_db, "test-project")

        assert export.project_id == "test-project"
        assert len(export.tasks) == 3

    def test_migrate_local_to_team_with_file(self, populated_db: Path, tmp_path: Path):
        """Test convenience function with output file."""
        output_path = tmp_path / "export.json"

        export = migrate_local_to_team(populated_db, "test-project", output_path)

        assert output_path.exists()
        assert export.project_id == "test-project"

    def test_migrate_team_to_local(self, temp_db: Path, tmp_path: Path):
        """Test convenience function for team to local migration."""
        # Create import file
        export_data = {
            "project_id": "team-project",
            "exported_at": "2025-01-16T12:00:00",
            "version": "1.0",
            "state": {"status": "execute"},
            "tasks": [],
            "locks": [],
        }
        input_path = tmp_path / "team_export.json"
        input_path.write_text(json.dumps(export_data))

        snapshot = migrate_team_to_local(temp_db, input_path, create_backup=False)

        assert snapshot.status == "completed"
        assert snapshot.source == "team"
        assert snapshot.target == "local"
