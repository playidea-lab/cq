"""C4 Memory Module - Project memory management for context persistence

This module provides memory management similar to Serena's memory system,
allowing persistent storage and retrieval of project context across sessions.

Memories are stored as markdown files in .c4/memories/ directory.

Usage:
    from c4.memory import MemoryStore

    store = MemoryStore(c4_dir)
    store.write("architecture", "# Architecture Decisions\\n...")
    content = store.read("architecture")
    memories = store.list_all()
    store.edit("architecture", "old text", "new text")
    store.delete("architecture")
"""

from .store import MemoryStore

__all__ = [
    "MemoryStore",
]
