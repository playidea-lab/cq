"""Artifact tool handlers for MCP.

Handles: c4_artifact_list, c4_artifact_save, c4_artifact_get
"""

import os
from pathlib import Path
from typing import Any

from ..registry import register_tool


def _get_artifact_store():
    """Get artifact store instance."""
    from c4.artifacts.store import LocalArtifactStore

    root = Path(os.environ.get("C4_PROJECT_ROOT", "."))
    return LocalArtifactStore(base_path=root / ".c4" / "artifacts")


@register_tool("c4_artifact_list")
def handle_artifact_list(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """List artifacts for a task.

    Args (via arguments):
        task_id: Task ID to list artifacts for

    Returns:
        List of artifact records.
    """
    task_id = arguments.get("task_id")
    if not task_id:
        return {"error": "task_id is required"}

    try:
        import asyncio

        store = _get_artifact_store()
        artifacts = asyncio.get_event_loop().run_until_complete(
            store.list(task_id)
        )

        return {
            "task_id": task_id,
            "count": len(artifacts),
            "artifacts": [
                {
                    "name": a.name,
                    "type": a.type,
                    "content_hash": a.content_hash[:12] + "..." if a.content_hash else "",
                    "size_bytes": a.size_bytes,
                    "version": a.version,
                    "local_path": a.local_path,
                }
                for a in artifacts
            ],
        }
    except Exception as e:
        return {"error": f"List artifacts failed: {e}"}


@register_tool("c4_artifact_save")
def handle_artifact_save(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Save an artifact.

    Args (via arguments):
        task_id: Related task ID
        path: Local file path to save as artifact
        artifact_type: Type: source, data, or output (default: output)

    Returns:
        Saved artifact reference.
    """
    task_id = arguments.get("task_id")
    path = arguments.get("path")

    if not task_id:
        return {"error": "task_id is required"}
    if not path:
        return {"error": "path is required"}

    local_path = Path(path)
    if not local_path.exists():
        return {"error": f"File not found: {path}"}

    artifact_type = arguments.get("artifact_type", "output")

    try:
        import asyncio

        store = _get_artifact_store()
        ref = asyncio.get_event_loop().run_until_complete(
            store.save(task_id, local_path, artifact_type)
        )

        return {
            "success": True,
            "artifact": {
                "name": ref.name,
                "type": ref.type,
                "content_hash": ref.content_hash,
                "size_bytes": ref.size_bytes,
                "version": ref.version,
                "local_path": ref.local_path,
            },
            "message": f"Artifact saved: {ref.name}",
        }
    except Exception as e:
        return {"error": f"Save artifact failed: {e}"}


@register_tool("c4_artifact_get")
def handle_artifact_get(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Get artifact path and metadata.

    Args (via arguments):
        task_id: Task ID
        name: Artifact name
        version: Specific version (optional, latest if omitted)

    Returns:
        Artifact path and metadata.
    """
    task_id = arguments.get("task_id")
    name = arguments.get("name")

    if not task_id:
        return {"error": "task_id is required"}
    if not name:
        return {"error": "name is required"}

    version = arguments.get("version")

    try:
        import asyncio

        store = _get_artifact_store()
        path = asyncio.get_event_loop().run_until_complete(
            store.get(task_id, name, version)
        )

        return {
            "task_id": task_id,
            "name": name,
            "path": str(path),
            "exists": path.exists(),
        }
    except FileNotFoundError:
        return {"error": f"Artifact not found: {name} for task {task_id}"}
    except Exception as e:
        return {"error": f"Get artifact failed: {e}"}
