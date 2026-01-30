"""Memory management tool handlers.

Handles: c4_write_memory, c4_read_memory, c4_list_memories, c4_delete_memory
"""

import os
from pathlib import Path
from typing import Any

from ..registry import register_tool


def _get_memory_manager() -> Any:
    """Get memory manager instance."""
    from c4.memory import get_memory_manager

    # Get project root
    if os.environ.get("C4_PROJECT_ROOT"):
        root = Path(os.environ["C4_PROJECT_ROOT"])
    else:
        root = Path.cwd()

    return get_memory_manager(root)


@register_tool("c4_write_memory")
def handle_write_memory(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Write content to a memory.

    Args (via arguments):
        name: Memory name (e.g., "architecture-decisions", "coding-patterns")
        content: Content to write (markdown format recommended)

    Returns:
        Success status and path
    """
    name = arguments.get("name")
    content = arguments.get("content")

    if not name:
        return {"error": "name is required"}
    if not content:
        return {"error": "content is required"}

    try:
        manager = _get_memory_manager()
        path = manager.write(name, content)

        return {
            "success": True,
            "name": name,
            "path": str(path),
            "size_bytes": len(content),
            "message": f"Memory '{name}' written successfully",
        }
    except ValueError as e:
        return {"error": str(e)}
    except Exception as e:
        return {"error": f"Failed to write memory: {e}"}


@register_tool("c4_read_memory")
def handle_read_memory(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Read content from a memory.

    Args (via arguments):
        name: Memory name to read

    Returns:
        Memory content or error
    """
    name = arguments.get("name")

    if not name:
        return {"error": "name is required"}

    try:
        manager = _get_memory_manager()
        content = manager.read(name)

        if content is None:
            return {
                "found": False,
                "name": name,
                "message": f"Memory '{name}' not found",
            }

        return {
            "found": True,
            "name": name,
            "content": content,
            "size_bytes": len(content),
        }
    except ValueError as e:
        return {"error": str(e)}
    except Exception as e:
        return {"error": f"Failed to read memory: {e}"}


@register_tool("c4_list_memories")
def handle_list_memories(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """List all available memories.

    Args (via arguments):
        pattern: Optional search pattern to filter memories

    Returns:
        List of memory names
    """
    pattern = arguments.get("pattern")

    try:
        manager = _get_memory_manager()

        if pattern:
            names = manager.search(pattern)
        else:
            names = manager.list()

        return {
            "memories": names,
            "count": len(names),
            "pattern": pattern,
        }
    except Exception as e:
        return {"error": f"Failed to list memories: {e}"}


@register_tool("c4_delete_memory")
def handle_delete_memory(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Delete a memory.

    Args (via arguments):
        name: Memory name to delete

    Returns:
        Success status
    """
    name = arguments.get("name")

    if not name:
        return {"error": "name is required"}

    try:
        manager = _get_memory_manager()
        deleted = manager.delete(name)

        if not deleted:
            return {
                "deleted": False,
                "name": name,
                "message": f"Memory '{name}' not found",
            }

        return {
            "deleted": True,
            "name": name,
            "message": f"Memory '{name}' deleted successfully",
        }
    except ValueError as e:
        return {"error": str(e)}
    except Exception as e:
        return {"error": f"Failed to delete memory: {e}"}
