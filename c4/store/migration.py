"""SQLite-Supabase Migration Utilities.

Provides utilities for:
- Exporting local SQLite data for team migration
- Importing team data to local SQLite
- Backup and rollback support
"""

from __future__ import annotations

import json
import shutil
import sqlite3
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Any

from .exceptions import MigrationError


@dataclass
class MigrationSnapshot:
    """Snapshot of a migration operation for rollback."""

    snapshot_id: str
    source: str  # "local" or "team"
    target: str  # "team" or "local"
    timestamp: str
    backup_path: str | None
    state_data: dict[str, Any] | None
    tasks_count: int
    locks_count: int
    status: str  # "pending", "completed", "rolled_back", "failed"

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary for JSON serialization."""
        return {
            "snapshot_id": self.snapshot_id,
            "source": self.source,
            "target": self.target,
            "timestamp": self.timestamp,
            "backup_path": self.backup_path,
            "state_data": self.state_data,
            "tasks_count": self.tasks_count,
            "locks_count": self.locks_count,
            "status": self.status,
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> MigrationSnapshot:
        """Create from dictionary."""
        return cls(
            snapshot_id=data["snapshot_id"],
            source=data["source"],
            target=data["target"],
            timestamp=data["timestamp"],
            backup_path=data.get("backup_path"),
            state_data=data.get("state_data"),
            tasks_count=data.get("tasks_count", 0),
            locks_count=data.get("locks_count", 0),
            status=data.get("status", "pending"),
        )


@dataclass
class ExportData:
    """Data structure for migration export."""

    project_id: str
    exported_at: str
    version: str
    state: dict[str, Any]
    tasks: list[dict[str, Any]]
    locks: list[dict[str, Any]]

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary for JSON serialization."""
        return {
            "project_id": self.project_id,
            "exported_at": self.exported_at,
            "version": self.version,
            "state": self.state,
            "tasks": self.tasks,
            "locks": self.locks,
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> ExportData:
        """Create from dictionary."""
        return cls(
            project_id=data["project_id"],
            exported_at=data["exported_at"],
            version=data.get("version", "1.0"),
            state=data["state"],
            tasks=data.get("tasks", []),
            locks=data.get("locks", []),
        )


class MigrationManager:
    """Manager for SQLite-Supabase data migration.

    Provides:
    - export_to_team(): Export local data for team migration
    - import_from_team(): Import team data to local SQLite
    - create_backup(): Create backup before migration
    - rollback(): Rollback to previous state
    """

    EXPORT_VERSION = "1.0"

    def __init__(self, db_path: Path, backup_dir: Path | None = None):
        """Initialize migration manager.

        Args:
            db_path: Path to SQLite database
            backup_dir: Directory for backups (default: db_path.parent / "backups")
        """
        self.db_path = db_path
        self.backup_dir = backup_dir or db_path.parent / "backups"
        self._snapshots: list[MigrationSnapshot] = []
        self._load_snapshots()

    def _load_snapshots(self) -> None:
        """Load migration snapshots from disk."""
        snapshots_file = self.backup_dir / "snapshots.json"
        if snapshots_file.exists():
            data = json.loads(snapshots_file.read_text())
            self._snapshots = [MigrationSnapshot.from_dict(s) for s in data]

    def _save_snapshots(self) -> None:
        """Save migration snapshots to disk."""
        self.backup_dir.mkdir(parents=True, exist_ok=True)
        snapshots_file = self.backup_dir / "snapshots.json"
        data = [s.to_dict() for s in self._snapshots]
        snapshots_file.write_text(json.dumps(data, indent=2))

    def _get_connection(self) -> sqlite3.Connection:
        """Get a database connection."""
        if not self.db_path.exists():
            raise MigrationError(f"Database not found: {self.db_path}")

        conn = sqlite3.connect(
            self.db_path,
            detect_types=sqlite3.PARSE_DECLTYPES | sqlite3.PARSE_COLNAMES,
            timeout=30.0,
        )
        conn.execute("PRAGMA journal_mode=WAL")
        return conn

    def export_to_team(self, project_id: str) -> ExportData:
        """Export local SQLite data for team migration.

        Args:
            project_id: Project identifier

        Returns:
            ExportData containing all project data

        Raises:
            MigrationError: If export fails
        """
        try:
            conn = self._get_connection()

            # Export state
            cursor = conn.execute(
                "SELECT state_json FROM c4_state WHERE project_id = ?",
                (project_id,),
            )
            row = cursor.fetchone()
            if row is None:
                raise MigrationError(f"No state found for project: {project_id}")

            state_data = json.loads(row[0])

            # Export tasks
            cursor = conn.execute(
                "SELECT task_json FROM c4_tasks WHERE project_id = ?",
                (project_id,),
            )
            tasks = [json.loads(row[0]) for row in cursor.fetchall()]

            # Export locks
            cursor = conn.execute(
                "SELECT scope, owner, expires_at FROM c4_locks WHERE project_id = ?",
                (project_id,),
            )
            locks = [
                {
                    "scope": row[0],
                    "owner": row[1],
                    "expires_at": row[2].isoformat() if hasattr(row[2], "isoformat") else row[2],
                }
                for row in cursor.fetchall()
            ]

            conn.close()

            return ExportData(
                project_id=project_id,
                exported_at=datetime.now().isoformat(),
                version=self.EXPORT_VERSION,
                state=state_data,
                tasks=tasks,
                locks=locks,
            )
        except sqlite3.Error as e:
            raise MigrationError(f"Export failed: {e}") from e

    def export_to_file(self, project_id: str, output_path: Path) -> ExportData:
        """Export data to a JSON file.

        Args:
            project_id: Project identifier
            output_path: Path for output JSON file

        Returns:
            ExportData that was exported
        """
        export_data = self.export_to_team(project_id)
        output_path.parent.mkdir(parents=True, exist_ok=True)
        output_path.write_text(json.dumps(export_data.to_dict(), indent=2))
        return export_data

    def import_from_team(
        self,
        data: ExportData,
        create_backup: bool = True,
    ) -> MigrationSnapshot:
        """Import team data to local SQLite.

        Args:
            data: ExportData to import
            create_backup: Whether to create backup before import

        Returns:
            MigrationSnapshot for rollback

        Raises:
            MigrationError: If import fails
        """
        snapshot_id = f"import-{datetime.now().strftime('%Y%m%d-%H%M%S')}"
        backup_path = None

        # Create backup if requested
        if create_backup and self.db_path.exists():
            backup_path = str(self.create_backup(f"pre-import-{snapshot_id}"))

        snapshot = MigrationSnapshot(
            snapshot_id=snapshot_id,
            source="team",
            target="local",
            timestamp=datetime.now().isoformat(),
            backup_path=backup_path,
            state_data=None,  # Not needed for team->local
            tasks_count=len(data.tasks),
            locks_count=len(data.locks),
            status="pending",
        )

        try:
            conn = self._get_connection()

            # Import state
            conn.execute(
                """
                INSERT OR REPLACE INTO c4_state (project_id, state_json, updated_at)
                VALUES (?, ?, ?)
                """,
                (
                    data.project_id,
                    json.dumps(data.state),
                    datetime.now(),
                ),
            )

            # Import tasks
            for task in data.tasks:
                task_id = task.get("id")
                conn.execute(
                    """
                    INSERT OR REPLACE INTO c4_tasks
                        (project_id, task_id, task_json, status, assigned_to, updated_at)
                    VALUES (?, ?, ?, ?, ?, ?)
                    """,
                    (
                        data.project_id,
                        task_id,
                        json.dumps(task),
                        task.get("status", "pending"),
                        task.get("assigned_to"),
                        datetime.now(),
                    ),
                )

            # Import locks
            for lock in data.locks:
                expires_at = lock.get("expires_at")
                if isinstance(expires_at, str):
                    expires_at = datetime.fromisoformat(expires_at)

                conn.execute(
                    """
                    INSERT OR REPLACE INTO c4_locks
                        (project_id, scope, owner, expires_at)
                    VALUES (?, ?, ?, ?)
                    """,
                    (
                        data.project_id,
                        lock["scope"],
                        lock["owner"],
                        expires_at,
                    ),
                )

            conn.commit()
            conn.close()

            snapshot.status = "completed"
        except (sqlite3.Error, Exception) as e:
            snapshot.status = "failed"
            self._snapshots.append(snapshot)
            self._save_snapshots()
            raise MigrationError(f"Import failed: {e}") from e

        self._snapshots.append(snapshot)
        self._save_snapshots()
        return snapshot

    def import_from_file(
        self,
        input_path: Path,
        create_backup: bool = True,
    ) -> MigrationSnapshot:
        """Import data from a JSON file.

        Args:
            input_path: Path to input JSON file
            create_backup: Whether to create backup before import

        Returns:
            MigrationSnapshot for rollback
        """
        if not input_path.exists():
            raise MigrationError(f"Import file not found: {input_path}")

        data = ExportData.from_dict(json.loads(input_path.read_text()))
        return self.import_from_team(data, create_backup)

    def create_backup(self, name: str | None = None) -> Path:
        """Create a backup of the current database.

        Args:
            name: Optional backup name (default: timestamp-based)

        Returns:
            Path to backup file
        """
        if not self.db_path.exists():
            raise MigrationError(f"Database not found: {self.db_path}")

        self.backup_dir.mkdir(parents=True, exist_ok=True)

        if name is None:
            name = datetime.now().strftime("%Y%m%d-%H%M%S")

        backup_path = self.backup_dir / f"{name}.db"
        shutil.copy2(self.db_path, backup_path)

        # Also backup WAL files if present
        wal_path = Path(str(self.db_path) + "-wal")
        shm_path = Path(str(self.db_path) + "-shm")

        if wal_path.exists():
            shutil.copy2(wal_path, self.backup_dir / f"{name}.db-wal")
        if shm_path.exists():
            shutil.copy2(shm_path, self.backup_dir / f"{name}.db-shm")

        return backup_path

    def rollback(self, snapshot_id: str | None = None) -> bool:
        """Rollback to a previous state.

        Args:
            snapshot_id: Snapshot to rollback to (default: most recent)

        Returns:
            True if rollback succeeded

        Raises:
            MigrationError: If rollback fails
        """
        # Find snapshot
        if snapshot_id is None:
            # Get most recent completed snapshot with backup
            valid_snapshots = [
                s for s in self._snapshots if s.status == "completed" and s.backup_path
            ]
            if not valid_snapshots:
                raise MigrationError("No valid snapshots available for rollback")
            snapshot = valid_snapshots[-1]
        else:
            matching = [s for s in self._snapshots if s.snapshot_id == snapshot_id]
            if not matching:
                raise MigrationError(f"Snapshot not found: {snapshot_id}")
            snapshot = matching[0]

        if not snapshot.backup_path:
            raise MigrationError(f"Snapshot has no backup: {snapshot.snapshot_id}")

        backup_path = Path(snapshot.backup_path)
        if not backup_path.exists():
            raise MigrationError(f"Backup file not found: {backup_path}")

        try:
            # Restore backup
            shutil.copy2(backup_path, self.db_path)

            # Restore WAL files if present
            backup_wal = Path(str(backup_path) + "-wal")
            backup_shm = Path(str(backup_path) + "-shm")

            if backup_wal.exists():
                shutil.copy2(backup_wal, Path(str(self.db_path) + "-wal"))
            if backup_shm.exists():
                shutil.copy2(backup_shm, Path(str(self.db_path) + "-shm"))

            # Update snapshot status
            snapshot.status = "rolled_back"
            self._save_snapshots()

            return True
        except OSError as e:
            raise MigrationError(f"Rollback failed: {e}") from e

    def list_backups(self) -> list[dict[str, Any]]:
        """List available backups.

        Returns:
            List of backup info dictionaries
        """
        if not self.backup_dir.exists():
            return []

        backups = []
        for path in self.backup_dir.glob("*.db"):
            # Skip WAL and SHM files
            if str(path).endswith("-wal") or str(path).endswith("-shm"):
                continue

            stat = path.stat()
            backups.append(
                {
                    "name": path.stem,
                    "path": str(path),
                    "size_bytes": stat.st_size,
                    "created_at": datetime.fromtimestamp(stat.st_mtime).isoformat(),
                }
            )

        return sorted(backups, key=lambda x: x["created_at"], reverse=True)

    def list_snapshots(self) -> list[MigrationSnapshot]:
        """List all migration snapshots.

        Returns:
            List of MigrationSnapshot objects
        """
        return list(self._snapshots)

    def cleanup_old_backups(self, keep_count: int = 5) -> int:
        """Remove old backups, keeping the most recent ones.

        Args:
            keep_count: Number of backups to keep

        Returns:
            Number of backups removed
        """
        backups = self.list_backups()

        if len(backups) <= keep_count:
            return 0

        to_remove = backups[keep_count:]
        removed = 0

        for backup in to_remove:
            path = Path(backup["path"])
            try:
                path.unlink()
                # Also remove WAL and SHM files
                wal_path = Path(str(path) + "-wal")
                shm_path = Path(str(path) + "-shm")
                if wal_path.exists():
                    wal_path.unlink()
                if shm_path.exists():
                    shm_path.unlink()
                removed += 1
            except OSError:
                pass

        return removed


def migrate_local_to_team(
    db_path: Path,
    project_id: str,
    output_path: Path | None = None,
) -> ExportData:
    """Convenience function to migrate local data for team use.

    Args:
        db_path: Path to local SQLite database
        project_id: Project identifier
        output_path: Optional path to save export JSON

    Returns:
        ExportData containing migration data
    """
    manager = MigrationManager(db_path)

    if output_path:
        return manager.export_to_file(project_id, output_path)
    else:
        return manager.export_to_team(project_id)


def migrate_team_to_local(
    db_path: Path,
    input_path: Path,
    create_backup: bool = True,
) -> MigrationSnapshot:
    """Convenience function to migrate team data to local.

    Args:
        db_path: Path to local SQLite database
        input_path: Path to team export JSON file
        create_backup: Whether to create backup before import

    Returns:
        MigrationSnapshot for rollback
    """
    manager = MigrationManager(db_path)
    return manager.import_from_file(input_path, create_backup)


# =============================================================================
# Direct Supabase Migration
# =============================================================================


async def export_to_supabase(
    db_path: Path,
    project_id: str,
    team_id: str,
    supabase_url: str | None = None,
    supabase_key: str | None = None,
) -> MigrationSnapshot:
    """Export local SQLite data directly to Supabase.

    Args:
        db_path: Path to local SQLite database
        project_id: Project identifier
        team_id: Target team ID in Supabase
        supabase_url: Supabase URL (default: from env)
        supabase_key: Supabase API key (default: from env)

    Returns:
        MigrationSnapshot for tracking

    Raises:
        MigrationError: If export fails
    """
    import os

    from .supabase import SupabaseStateStore

    # Get credentials
    url = supabase_url or os.environ.get("SUPABASE_URL")
    key = supabase_key or os.environ.get("SUPABASE_KEY")

    if not url or not key:
        raise MigrationError(
            "Supabase credentials not provided. "
            "Set SUPABASE_URL and SUPABASE_KEY environment variables."
        )

    # Export from SQLite
    manager = MigrationManager(db_path)
    export_data = manager.export_to_team(project_id)

    # Create snapshot
    snapshot_id = f"to-supabase-{datetime.now().strftime('%Y%m%d-%H%M%S')}"
    backup_path = str(manager.create_backup(f"pre-export-{snapshot_id}"))

    snapshot = MigrationSnapshot(
        snapshot_id=snapshot_id,
        source="local",
        target="supabase",
        timestamp=datetime.now().isoformat(),
        backup_path=backup_path,
        state_data=export_data.state,
        tasks_count=len(export_data.tasks),
        locks_count=len(export_data.locks),
        status="pending",
    )

    try:
        # Initialize Supabase store
        store = SupabaseStateStore(
            project_id=project_id,
            team_id=team_id,
            supabase_url=url,
            supabase_key=key,
        )

        # Upload state
        await store.save(export_data.state)

        # Upload tasks to c4_tasks table
        for task in export_data.tasks:
            task_id = task.get("id")
            await store._supabase.table("c4_tasks").upsert(
                {
                    "project_id": project_id,
                    "team_id": team_id,
                    "task_id": task_id,
                    "task_json": json.dumps(task),
                    "status": task.get("status", "pending"),
                    "assigned_to": task.get("assigned_to"),
                }
            ).execute()

        # Upload locks to c4_locks table
        for lock in export_data.locks:
            await store._supabase.table("c4_locks").upsert(
                {
                    "project_id": project_id,
                    "team_id": team_id,
                    "scope": lock["scope"],
                    "owner": lock["owner"],
                    "expires_at": lock.get("expires_at"),
                }
            ).execute()

        snapshot.status = "completed"
    except Exception as e:
        snapshot.status = "failed"
        manager._snapshots.append(snapshot)
        manager._save_snapshots()
        raise MigrationError(f"Supabase export failed: {e}") from e

    manager._snapshots.append(snapshot)
    manager._save_snapshots()
    return snapshot


async def import_from_supabase(
    db_path: Path,
    project_id: str,
    team_id: str,
    supabase_url: str | None = None,
    supabase_key: str | None = None,
    create_backup: bool = True,
) -> MigrationSnapshot:
    """Import data from Supabase to local SQLite.

    Args:
        db_path: Path to local SQLite database
        project_id: Project identifier
        team_id: Source team ID in Supabase
        supabase_url: Supabase URL (default: from env)
        supabase_key: Supabase API key (default: from env)
        create_backup: Whether to create backup before import

    Returns:
        MigrationSnapshot for rollback

    Raises:
        MigrationError: If import fails
    """
    import os

    from .supabase import SupabaseStateStore

    # Get credentials
    url = supabase_url or os.environ.get("SUPABASE_URL")
    key = supabase_key or os.environ.get("SUPABASE_KEY")

    if not url or not key:
        raise MigrationError(
            "Supabase credentials not provided. "
            "Set SUPABASE_URL and SUPABASE_KEY environment variables."
        )

    manager = MigrationManager(db_path)

    # Create backup if requested
    backup_path = None
    if create_backup and db_path.exists():
        snapshot_id = f"from-supabase-{datetime.now().strftime('%Y%m%d-%H%M%S')}"
        backup_path = str(manager.create_backup(f"pre-import-{snapshot_id}"))
    else:
        snapshot_id = f"from-supabase-{datetime.now().strftime('%Y%m%d-%H%M%S')}"

    try:
        # Initialize Supabase store
        store = SupabaseStateStore(
            project_id=project_id,
            team_id=team_id,
            supabase_url=url,
            supabase_key=key,
        )

        # Load state from Supabase
        state_data = await store.load()
        if state_data is None:
            raise MigrationError(f"No state found for project: {project_id}")

        # Load tasks from Supabase
        tasks_result = await store._supabase.table("c4_tasks").select("*").eq(
            "project_id", project_id
        ).eq("team_id", team_id).execute()

        tasks = [json.loads(row["task_json"]) for row in tasks_result.data]

        # Load locks from Supabase
        locks_result = await store._supabase.table("c4_locks").select("*").eq(
            "project_id", project_id
        ).eq("team_id", team_id).execute()

        locks = [
            {
                "scope": row["scope"],
                "owner": row["owner"],
                "expires_at": row.get("expires_at"),
            }
            for row in locks_result.data
        ]

        # Create export data
        export_data = ExportData(
            project_id=project_id,
            exported_at=datetime.now().isoformat(),
            version=MigrationManager.EXPORT_VERSION,
            state=state_data,
            tasks=tasks,
            locks=locks,
        )

        # Import to local SQLite
        snapshot = manager.import_from_team(export_data, create_backup=False)
        snapshot.snapshot_id = snapshot_id
        snapshot.source = "supabase"
        snapshot.backup_path = backup_path

        return snapshot

    except Exception as e:
        snapshot = MigrationSnapshot(
            snapshot_id=snapshot_id,
            source="supabase",
            target="local",
            timestamp=datetime.now().isoformat(),
            backup_path=backup_path,
            state_data=None,
            tasks_count=0,
            locks_count=0,
            status="failed",
        )
        manager._snapshots.append(snapshot)
        manager._save_snapshots()
        raise MigrationError(f"Supabase import failed: {e}") from e


def sync_with_supabase(
    db_path: Path,
    project_id: str,
    team_id: str,
    direction: str = "upload",
    supabase_url: str | None = None,
    supabase_key: str | None = None,
) -> MigrationSnapshot:
    """Synchronous wrapper for Supabase migration.

    Args:
        db_path: Path to local SQLite database
        project_id: Project identifier
        team_id: Team ID in Supabase
        direction: "upload" (local→cloud) or "download" (cloud→local)
        supabase_url: Supabase URL (default: from env)
        supabase_key: Supabase API key (default: from env)

    Returns:
        MigrationSnapshot for tracking
    """
    import asyncio

    if direction == "upload":
        return asyncio.run(
            export_to_supabase(
                db_path, project_id, team_id, supabase_url, supabase_key
            )
        )
    elif direction == "download":
        return asyncio.run(
            import_from_supabase(
                db_path, project_id, team_id, supabase_url, supabase_key
            )
        )
    else:
        raise MigrationError(f"Invalid direction: {direction}. Use 'upload' or 'download'.")
