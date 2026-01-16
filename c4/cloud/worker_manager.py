"""Cloud Worker Manager - Fly.io Machines dynamic scaling."""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import Any


class MachineState(str, Enum):
    """Fly.io Machine states."""

    CREATED = "created"
    STARTING = "starting"
    STARTED = "started"
    STOPPING = "stopping"
    STOPPED = "stopped"
    DESTROYING = "destroying"
    DESTROYED = "destroyed"


@dataclass
class WorkerInstance:
    """Represents a cloud worker instance."""

    id: str
    name: str
    state: MachineState
    region: str
    created_at: datetime
    task_id: str | None = None
    project_id: str | None = None
    metadata: dict[str, Any] = field(default_factory=dict)


class WorkerScaler:
    """
    Manages Fly.io Machines for dynamic worker scaling.

    Features:
    - Create workers on demand
    - Auto-scale based on queue depth
    - Scale to zero when idle
    - Regional worker placement

    Environment Variables:
        FLY_API_TOKEN: Fly.io API token
        FLY_APP_NAME: Application name (default: c4-worker)

    Example:
        scaler = WorkerScaler()
        worker = scaler.create_worker("my-project", "T-001")
        scaler.destroy_worker(worker.id)
    """

    API_BASE = "https://api.machines.dev/v1"
    DEFAULT_APP = "c4-worker"
    DEFAULT_REGION = "nrt"  # Tokyo

    def __init__(
        self,
        api_token: str | None = None,
        app_name: str | None = None,
    ):
        """Initialize worker scaler.

        Args:
            api_token: Fly.io API token (or FLY_API_TOKEN env)
            app_name: Fly.io app name (or FLY_APP_NAME env)
        """
        self._token = api_token or os.environ.get("FLY_API_TOKEN", "")
        self._app = app_name or os.environ.get("FLY_APP_NAME", self.DEFAULT_APP)
        self._client: Any = None

    @property
    def client(self) -> Any:
        """Get HTTP client (lazy init)."""
        if self._client is None:
            import httpx

            self._client = httpx.Client(
                base_url=self.API_BASE,
                headers={
                    "Authorization": f"Bearer {self._token}",
                    "Content-Type": "application/json",
                },
                timeout=60.0,
            )
        return self._client

    def close(self) -> None:
        """Close HTTP client."""
        if self._client:
            self._client.close()
            self._client = None

    def __enter__(self) -> "WorkerScaler":
        return self

    def __exit__(self, *args: Any) -> None:
        self.close()

    # =========================================================================
    # Machine Operations
    # =========================================================================

    def create_worker(
        self,
        project_id: str,
        task_id: str,
        region: str | None = None,
        env_vars: dict[str, str] | None = None,
    ) -> WorkerInstance | None:
        """Create a new worker machine for a task.

        Args:
            project_id: C4 project ID
            task_id: Task to process
            region: Fly.io region (default: nrt)
            env_vars: Additional environment variables

        Returns:
            WorkerInstance if created, None on failure
        """
        region = region or self.DEFAULT_REGION

        config = {
            "name": f"c4-worker-{task_id.lower()}",
            "region": region,
            "config": {
                "image": f"registry.fly.io/{self._app}:latest",
                "env": {
                    "C4_PROJECT_ID": project_id,
                    "C4_TASK_ID": task_id,
                    "C4_WORKER_MODE": "remote",
                    **(env_vars or {}),
                },
                "guest": {
                    "cpu_kind": "shared",
                    "cpus": 1,
                    "memory_mb": 512,
                },
                "auto_destroy": True,  # Destroy after task
            },
        }

        try:
            response = self.client.post(
                f"/apps/{self._app}/machines",
                json=config,
            )

            if response.status_code in (200, 201):
                data = response.json()
                return WorkerInstance(
                    id=data["id"],
                    name=data["name"],
                    state=MachineState(data["state"]),
                    region=data["region"],
                    created_at=datetime.fromisoformat(
                        data["created_at"].replace("Z", "+00:00")
                    ),
                    task_id=task_id,
                    project_id=project_id,
                )
            return None
        except Exception:
            return None

    def destroy_worker(self, machine_id: str, force: bool = False) -> bool:
        """Destroy a worker machine.

        Args:
            machine_id: Machine ID to destroy
            force: Force destroy even if running

        Returns:
            True if destroyed successfully
        """
        try:
            params = {"force": "true"} if force else {}
            response = self.client.delete(
                f"/apps/{self._app}/machines/{machine_id}",
                params=params,
            )
            return response.status_code in (200, 204)
        except Exception:
            return False

    def stop_worker(self, machine_id: str) -> bool:
        """Stop a worker machine (keeps state).

        Args:
            machine_id: Machine ID to stop

        Returns:
            True if stopped successfully
        """
        try:
            response = self.client.post(
                f"/apps/{self._app}/machines/{machine_id}/stop"
            )
            return response.status_code == 200
        except Exception:
            return False

    def start_worker(self, machine_id: str) -> bool:
        """Start a stopped worker machine.

        Args:
            machine_id: Machine ID to start

        Returns:
            True if started successfully
        """
        try:
            response = self.client.post(
                f"/apps/{self._app}/machines/{machine_id}/start"
            )
            return response.status_code == 200
        except Exception:
            return False

    # =========================================================================
    # Listing and Status
    # =========================================================================

    def list_workers(self) -> list[WorkerInstance]:
        """List all worker machines.

        Returns:
            List of WorkerInstance objects
        """
        try:
            response = self.client.get(f"/apps/{self._app}/machines")

            if response.status_code != 200:
                return []

            workers = []
            for machine in response.json():
                env = machine.get("config", {}).get("env", {})
                workers.append(
                    WorkerInstance(
                        id=machine["id"],
                        name=machine["name"],
                        state=MachineState(machine["state"]),
                        region=machine["region"],
                        created_at=datetime.fromisoformat(
                            machine["created_at"].replace("Z", "+00:00")
                        ),
                        task_id=env.get("C4_TASK_ID"),
                        project_id=env.get("C4_PROJECT_ID"),
                    )
                )
            return workers
        except Exception:
            return []

    def get_worker(self, machine_id: str) -> WorkerInstance | None:
        """Get a specific worker machine.

        Args:
            machine_id: Machine ID

        Returns:
            WorkerInstance if found, None otherwise
        """
        try:
            response = self.client.get(
                f"/apps/{self._app}/machines/{machine_id}"
            )

            if response.status_code != 200:
                return None

            machine = response.json()
            env = machine.get("config", {}).get("env", {})
            return WorkerInstance(
                id=machine["id"],
                name=machine["name"],
                state=MachineState(machine["state"]),
                region=machine["region"],
                created_at=datetime.fromisoformat(
                    machine["created_at"].replace("Z", "+00:00")
                ),
                task_id=env.get("C4_TASK_ID"),
                project_id=env.get("C4_PROJECT_ID"),
            )
        except Exception:
            return None

    # =========================================================================
    # Auto-Scaling
    # =========================================================================

    def get_active_count(self) -> int:
        """Get count of active (running) workers."""
        workers = self.list_workers()
        return len([w for w in workers if w.state == MachineState.STARTED])

    def scale_to(self, target_count: int, project_id: str) -> dict[str, Any]:
        """Scale workers to target count.

        Args:
            target_count: Desired number of workers
            project_id: Project for new workers

        Returns:
            Dict with scaling results
        """
        current = self.get_active_count()
        results = {
            "current": current,
            "target": target_count,
            "created": 0,
            "destroyed": 0,
        }

        if target_count > current:
            # Scale up
            for i in range(target_count - current):
                worker = self.create_worker(
                    project_id=project_id,
                    task_id=f"POOL-{i}",
                )
                if worker:
                    results["created"] += 1

        elif target_count < current:
            # Scale down - destroy idle workers
            workers = self.list_workers()
            idle_workers = [
                w for w in workers
                if w.state == MachineState.STARTED and not w.task_id
            ]

            for worker in idle_workers[: current - target_count]:
                if self.destroy_worker(worker.id):
                    results["destroyed"] += 1

        return results

    def calculate_desired_workers(
        self,
        pending_tasks: int,
        max_workers: int = 10,
        tasks_per_worker: int = 1,
    ) -> int:
        """Calculate desired worker count based on queue.

        Args:
            pending_tasks: Number of pending tasks
            max_workers: Maximum workers to scale to
            tasks_per_worker: Tasks each worker handles concurrently

        Returns:
            Desired worker count
        """
        if pending_tasks == 0:
            return 0

        desired = (pending_tasks + tasks_per_worker - 1) // tasks_per_worker
        return min(desired, max_workers)
