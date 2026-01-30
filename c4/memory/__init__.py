"""C4 Memory System.

Provides session-persistent knowledge storage following the Serena pattern.
Memories are stored as markdown files in .c4/memories/ directory.

Usage:
    from c4.memory import MemoryManager, get_memory_manager

    # Get global instance
    manager = get_memory_manager(project_path)

    # Write memory
    manager.write("architecture-decisions", "# ADR-001\\n...")

    # Read memory
    content = manager.read("architecture-decisions")

    # List all memories
    names = manager.list()

    # Delete memory
    manager.delete("old-memory")
"""

from c4.memory.manager import (
    MemoryManager,
    get_memory_manager,
    reset_global_manager,
)

__all__ = [
    "MemoryManager",
    "get_memory_manager",
    "reset_global_manager",
]
