"""Entry point for Jedi worker subprocess.

Runs in isolated process, receives commands via Queue.

Key responsibilities:
- Set up project context (cwd, sys.path, jedi.Project)
- Execute Jedi operations safely
- Return serializable results only (no Jedi objects)
- Smart cache clearing (error/timeout only)

Protocol:
    Request dict:
        {
            "id": "uuid",          # Required: request/response matching
            "op": "get_names",     # Operation: get_names, completions, goto
            "source": "...",       # Source code
            "path": "/path/...",   # Optional file path
            "line": 10,            # Line number (for some ops)
            "column": 5,           # Column number (for some ops)
            "options": {...},      # Operation-specific options
            "meta": {...},         # Metadata (repo_root, sys_path, etc.)
        }

    Response dict:
        {
            "id": "...",           # Same as request
            "ok": True,            # Success boolean
            "result": [...],       # Serializable result
            "error": None,         # Or {"code": "...", "message": "..."}
            "stats": {"ms": 123},  # Execution stats
        }
"""

from __future__ import annotations

import logging
import os
import queue
import sys
import time
from multiprocessing import Queue
from typing import Any

logger = logging.getLogger(__name__)

# Jedi import with configuration
try:
    import jedi
    import jedi.cache
    import jedi.settings

    # Configure Jedi for subprocess use
    jedi.settings.cache_directory = None  # No disk cache
    jedi.settings.call_signatures_validity = 0.0
    jedi.settings.auto_import_modules = []

    # Monkey-patch to prevent GC recursion errors
    try:
        from jedi.inference.compiled.subprocess import InferenceStateSubprocess

        InferenceStateSubprocess.__del__ = lambda self: None  # type: ignore[method-assign]
    except (ImportError, AttributeError):
        pass

    JEDI_AVAILABLE = True
except ImportError:
    JEDI_AVAILABLE = False
    jedi = None  # type: ignore[assignment]


def worker_main(
    input_queue: "Queue[dict[str, Any] | None]",
    output_queue: "Queue[dict[str, Any]]",
    repo_root: str,
) -> None:
    """Main loop for worker process.

    This function runs in a separate process and:
    1. Sets up the project context
    2. Waits for requests on input_queue
    3. Executes Jedi operations
    4. Sends responses on output_queue

    Args:
        input_queue: Queue for receiving requests (None = shutdown).
        output_queue: Queue for sending responses.
        repo_root: Project root path.
    """
    # Set up project context
    try:
        os.chdir(repo_root)
    except OSError:
        pass  # May fail if directory doesn't exist

    if repo_root not in sys.path:
        sys.path.insert(0, repo_root)

    # Create Jedi Project (reused for all requests)
    project: "jedi.Project | None" = None
    if JEDI_AVAILABLE:
        try:
            project = jedi.Project(
                path=repo_root,
                added_sys_path=[],
                smart_sys_path=False,
            )
        except Exception:
            logger.debug("Failed to create Jedi project for %s", repo_root, exc_info=True)
            project = None

    # Main loop
    while True:
        try:
            # Wait for request with idle timeout
            request = input_queue.get(timeout=300)  # 5 min idle timeout
        except (queue.Empty, OSError):
            # Queue timeout or broken pipe - exit cleanly
            break

        if request is None:
            # Shutdown signal
            break

        # Execute and respond
        start_ms = time.time() * 1000
        result = _execute_jedi_operation(request, project)
        result["stats"] = {
            "ms": round(time.time() * 1000 - start_ms, 2),
            "op": request.get("op", "unknown"),
        }

        try:
            output_queue.put(result)
        except OSError:
            # Broken pipe / closed queue - exit
            break


def _execute_jedi_operation(
    request: dict[str, Any],
    project: "jedi.Project | None",
) -> dict[str, Any]:
    """Execute a Jedi operation and return serializable result.

    Supported operations:
    - get_names: Get all symbol names in source
    - completions: Get code completions at position
    - goto: Go to definition at position
    - references: Find references to symbol at position

    Args:
        request: Request dict with operation parameters.
        project: Jedi Project for context (may be None).

    Returns:
        Response dict with result or error.
    """
    if not JEDI_AVAILABLE:
        return {
            "id": request.get("id", "unknown"),
            "ok": False,
            "result": None,
            "error": {"code": "jedi_unavailable", "message": "Jedi is not installed"},
        }

    request_id = request.get("id", "unknown")
    source = request.get("source", "")
    op = request.get("op", "")
    path = request.get("path")
    options = request.get("options", {})

    had_error = False

    try:
        # Create Jedi Script
        script = jedi.Script(
            code=source,
            path=path,
            project=project,
        )

        if op == "get_names":
            names = script.get_names(
                all_scopes=options.get("all_scopes", True),
                definitions=options.get("definitions", True),
            )
            return {
                "id": request_id,
                "ok": True,
                "result": [_serialize_name(n) for n in names],
                "error": None,
            }

        elif op == "completions":
            line = request.get("line", 1)
            column = request.get("column", 0)
            completions = script.complete(line, column)
            # Limit results to prevent huge responses
            return {
                "id": request_id,
                "ok": True,
                "result": [_serialize_completion(c) for c in completions[:50]],
                "error": None,
            }

        elif op == "goto":
            line = request.get("line", 1)
            column = request.get("column", 0)
            definitions = script.goto(line, column)
            return {
                "id": request_id,
                "ok": True,
                "result": [_serialize_name(d) for d in definitions],
                "error": None,
            }

        elif op == "references":
            line = request.get("line", 1)
            column = request.get("column", 0)
            refs = script.get_references(line, column)
            return {
                "id": request_id,
                "ok": True,
                "result": [_serialize_name(r) for r in refs[:100]],  # Limit refs
                "error": None,
            }

        elif op == "infer":
            line = request.get("line", 1)
            column = request.get("column", 0)
            inferences = script.infer(line, column)
            return {
                "id": request_id,
                "ok": True,
                "result": [_serialize_name(i) for i in inferences],
                "error": None,
            }

        else:
            return {
                "id": request_id,
                "ok": False,
                "result": None,
                "error": {"code": "unknown_op", "message": f"Unknown operation: {op}"},
            }

    except RecursionError:
        had_error = True
        return {
            "id": request_id,
            "ok": False,
            "result": None,
            "error": {
                "code": "recursion_limit",
                "message": "Python recursion limit exceeded",
            },
        }

    except Exception as e:
        had_error = True
        return {
            "id": request_id,
            "ok": False,
            "result": None,
            "error": {
                "code": "exception",
                "message": str(e),
                "type": type(e).__name__,
            },
        }

    finally:
        # Clear cache only on error (performance optimization)
        if had_error and JEDI_AVAILABLE:
            try:
                jedi.cache.clear_time_caches()
            except Exception:
                logger.debug("Failed to clear Jedi cache", exc_info=True)


def _serialize_name(name: Any) -> dict[str, Any]:
    """Convert Jedi Name to serializable dict.

    Extracts only the essential information needed for symbol operations.

    Args:
        name: Jedi Name object.

    Returns:
        Dict with serializable name information.
    """
    try:
        # Get parent info for method detection
        parent = None
        parent_type = None
        parent_name = None
        try:
            parent = name.parent()
            if parent:
                parent_type = parent.type
                parent_name = parent.name
        except (AttributeError, TypeError):
            pass

        # Get signatures for functions/methods
        signature = None
        if name.type in ("function", "class"):
            try:
                sigs = name.get_signatures()
                if sigs:
                    signature = str(sigs[0])
            except (AttributeError, TypeError):
                pass

        # Get docstring
        docstring = None
        try:
            docstring = name.docstring(raw=True)
            if docstring and len(docstring) > 500:
                docstring = docstring[:500] + "..."
        except (AttributeError, TypeError):
            pass

        # Get end position for editing support
        end_line = None
        end_column = None
        try:
            end_pos = name.get_definition_end_position()
            if end_pos:
                end_line = end_pos[0]    # 1-indexed (same as name.line)
                end_column = end_pos[1]  # 0-indexed (same as name.column)
        except (AttributeError, TypeError):
            pass

        return {
            "name": name.name,
            "type": name.type,
            "line": name.line,
            "column": name.column,
            "end_line": end_line,
            "end_column": end_column,
            "module_path": str(name.module_path) if name.module_path else None,
            "full_name": name.full_name,
            "description": name.description,
            "parent_type": parent_type,
            "parent_name": parent_name,
            "signature": signature,
            "docstring": docstring,
        }
    except Exception as e:
        # Fallback for any serialization errors
        return {
            "name": getattr(name, "name", "unknown"),
            "type": getattr(name, "type", "unknown"),
            "error": str(e),
        }


def _serialize_completion(comp: Any) -> dict[str, Any]:
    """Convert Jedi Completion to serializable dict.

    Args:
        comp: Jedi Completion object.

    Returns:
        Dict with serializable completion information.
    """
    try:
        return {
            "name": comp.name,
            "type": comp.type,
            "complete": comp.complete,
            "description": comp.description,
            "module_name": getattr(comp, "module_name", None),
        }
    except Exception as e:
        return {
            "name": getattr(comp, "name", "unknown"),
            "error": str(e),
        }


if __name__ == "__main__":
    # For direct testing
    import multiprocessing

    in_q: Queue[dict[str, Any] | None] = multiprocessing.Queue()
    out_q: Queue[dict[str, Any]] = multiprocessing.Queue()

    # Start worker
    p = multiprocessing.Process(target=worker_main, args=(in_q, out_q, "."))
    p.start()

    # Send test request
    in_q.put({
        "id": "test-1",
        "op": "get_names",
        "source": "def hello(): pass\nx = 1",
        "options": {"all_scopes": True, "definitions": True},
    })

    # Get response
    try:
        response = out_q.get(timeout=5)
        print(f"Response: {response}")
    except (queue.Empty, OSError) as e:
        print(f"Error: {e}")

    # Shutdown
    in_q.put(None)
    p.join(timeout=2)
    if p.is_alive():
        p.kill()
        p.join()
