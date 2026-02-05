"""Memory management tool handlers.

Handles: c4_write_memory, c4_read_memory, c4_list_memories, c4_delete_memory, c4_search_memory
"""

import os
from datetime import datetime
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


def _get_memory_searcher(project_id: str | None = None) -> Any:
    """Get memory searcher instance.

    Args:
        project_id: Optional project ID, defaults to current directory name.

    Returns:
        MemorySearcher instance.
    """
    from c4.memory.search import get_memory_searcher

    # Get project root
    if os.environ.get("C4_PROJECT_ROOT"):
        root = Path(os.environ["C4_PROJECT_ROOT"])
    else:
        root = Path.cwd()

    # Use directory name as project ID if not provided
    if project_id is None:
        project_id = root.name

    db_path = root / ".c4" / "tasks.db"

    return get_memory_searcher(
        project_id=project_id,
        db_path=db_path,
        enable_vector_search=False,  # Default to keyword-only for speed
    )


@register_tool("c4_search_memory")
def handle_search_memory(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Search memories using hybrid semantic and keyword search.

    Args (via arguments):
        query: Search query string
        limit: Maximum number of results (default: 10)
        filters: Optional filters
            - memory_type: Filter by source type (e.g., "read_file", "user_message")
            - tags: Filter by tags (observation must have at least one matching tag)
            - since: Only return observations created after this ISO datetime

    Returns:
        Search results with preview and scoring information
    """
    query = arguments.get("query")
    limit = arguments.get("limit", 10)
    filters_arg = arguments.get("filters", {})

    if not query:
        return {"error": "query is required"}

    try:
        # Parse filters
        from c4.memory.search import SearchFilters

        filters = None
        if filters_arg:
            since = None
            if filters_arg.get("since"):
                try:
                    since = datetime.fromisoformat(filters_arg["since"])
                except ValueError:
                    return {"error": f"Invalid since datetime: {filters_arg['since']}"}

            filters = SearchFilters(
                memory_type=filters_arg.get("memory_type"),
                tags=filters_arg.get("tags"),
                since=since,
                min_importance=filters_arg.get("min_importance"),
            )

        # Perform search
        searcher = _get_memory_searcher()
        results = searcher.search(query, limit=limit, filters=filters)

        # Calculate total tokens if all results were expanded
        total_tokens = sum(r.tokens for r in results)

        # Format results
        formatted_results = [
            {
                "id": r.id,
                "title": r.title,
                "preview": r.preview,
                "content_tokens": r.tokens,
                "score": round(r.score, 4),
                "source": r.source,
                "importance": r.importance,
                "created_at": r.created_at.isoformat() if r.created_at else None,
            }
            for r in results
        ]

        # Generate hint based on results
        hint = _generate_search_hint(len(results), total_tokens, limit)

        return {
            "results": formatted_results,
            "count": len(formatted_results),
            "total_tokens_if_expanded": total_tokens,
            "hint": hint,
        }
    except Exception as e:
        return {"error": f"Failed to search memories: {e}"}


def _generate_search_hint(count: int, total_tokens: int, limit: int) -> str:
    """Generate a helpful hint based on search results.

    Args:
        count: Number of results found.
        total_tokens: Total tokens if all results were expanded.
        limit: The limit that was used.

    Returns:
        A hint string for the user.
    """
    if count == 0:
        return "No results found. Try a different query or check if memories exist."

    if total_tokens > 2000:
        return (
            f"Found {count} results totaling ~{total_tokens} tokens. "
            "Consider reading specific memories by ID to avoid context overload."
        )

    if count >= limit:
        return (
            f"Found {count} results (limit reached). "
            "Try a more specific query or increase limit if needed."
        )

    return f"Found {count} results totaling ~{total_tokens} tokens."
