"""Memory management tool handlers.

Handles: c4_write_memory, c4_read_memory, c4_list_memories, c4_delete_memory,
         c4_search_memory, c4_get_memory_detail
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


def _get_auto_capture_handler(project_id: str | None = None) -> Any:
    """Get auto capture handler instance.

    Args:
        project_id: Optional project ID, defaults to current directory name.

    Returns:
        AutoCaptureHandler instance.
    """
    from c4.memory.auto_capture import get_auto_capture_handler

    # Get project root
    if os.environ.get("C4_PROJECT_ROOT"):
        root = Path(os.environ["C4_PROJECT_ROOT"])
    else:
        root = Path.cwd()

    # Use directory name as project ID if not provided
    if project_id is None:
        project_id = root.name

    db_path = root / ".c4" / "tasks.db"

    return get_auto_capture_handler(
        project_id=project_id,
        db_path=db_path,
        enable_embeddings=False,
    )


def _update_access_stats(db_path: Path, observation_id: str) -> None:
    """Update access_count and accessed_at for an observation.

    Args:
        db_path: Path to the SQLite database.
        observation_id: ID of the observation to update.
    """
    import sqlite3

    conn = sqlite3.connect(db_path)
    try:
        # Check if columns exist, add if needed
        cursor = conn.execute("PRAGMA table_info(c4_observations)")
        columns = [row[1] for row in cursor.fetchall()]

        if "access_count" not in columns:
            conn.execute(
                "ALTER TABLE c4_observations ADD COLUMN access_count INTEGER DEFAULT 0"
            )
        if "accessed_at" not in columns:
            conn.execute(
                "ALTER TABLE c4_observations ADD COLUMN accessed_at TIMESTAMP"
            )

        # Update access stats
        conn.execute(
            """
            UPDATE c4_observations
            SET access_count = COALESCE(access_count, 0) + 1,
                accessed_at = ?
            WHERE id = ?
            """,
            (datetime.now().isoformat(), observation_id),
        )
        conn.commit()
    finally:
        conn.close()


def _estimate_tokens(text: str) -> int:
    """Estimate token count for text.

    Args:
        text: The text to estimate.

    Returns:
        Estimated token count (~4 chars per token).
    """
    if not text:
        return 0
    return max(1, len(text) // 4)


@register_tool("c4_get_memory_detail")
def handle_get_memory_detail(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Get full details of a specific memory.

    Args (via arguments):
        memory_id: ID of the memory/observation to retrieve
        include_related: Whether to include related memories (default: false)

    Returns:
        Full memory details including content, metadata, and optionally related memories
    """
    memory_id = arguments.get("memory_id")
    include_related = arguments.get("include_related", False)

    if not memory_id:
        return {"error": "memory_id is required"}

    try:
        # Get project root and db path
        if os.environ.get("C4_PROJECT_ROOT"):
            root = Path(os.environ["C4_PROJECT_ROOT"])
        else:
            root = Path.cwd()
        db_path = root / ".c4" / "tasks.db"

        # Get the observation
        handler = _get_auto_capture_handler()
        observation = handler.get_observation(memory_id)

        if observation is None:
            return {
                "found": False,
                "memory_id": memory_id,
                "message": f"Memory '{memory_id}' not found",
            }

        # Update access stats
        _update_access_stats(db_path, memory_id)

        # Calculate content tokens
        content_tokens = _estimate_tokens(observation.content)

        # Build response
        result: dict[str, Any] = {
            "found": True,
            "id": observation.id,
            "title": observation.source,
            "content": observation.content,
            "content_tokens": content_tokens,
            "metadata": {
                "source": observation.source,
                "importance": observation.importance,
                "tags": observation.tags,
                "created_at": observation.created_at.isoformat() if observation.created_at else None,
                **observation.metadata,
            },
        }

        # Find related memories if requested
        if include_related:
            related = _find_related_memories(memory_id, observation.content, limit=5)
            result["related"] = related

        return result

    except Exception as e:
        return {"error": f"Failed to get memory detail: {e}"}


def _find_related_memories(
    memory_id: str, content: str, limit: int = 5
) -> list[dict[str, Any]]:
    """Find memories related to the given content.

    Args:
        memory_id: ID of the current memory (to exclude from results).
        content: Content to find related memories for.
        limit: Maximum number of related memories to return.

    Returns:
        List of related memory summaries.
    """
    try:
        searcher = _get_memory_searcher()

        # Extract key terms from content for search
        # Use first 100 words as the query
        words = content.split()[:100]
        query = " ".join(words)

        if not query.strip():
            return []

        # Search for related memories
        results = searcher.search(query, limit=limit + 1)  # +1 to account for self

        # Filter out the current memory and format results
        related = []
        for r in results:
            if r.id != memory_id:
                related.append({
                    "id": r.id,
                    "title": r.title,
                    "preview": r.preview,
                    "score": round(r.score, 4),
                })
                if len(related) >= limit:
                    break

        return related

    except Exception:
        # Return empty list on any error (non-critical feature)
        return []
