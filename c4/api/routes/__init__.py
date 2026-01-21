"""C4 API Routes.

Provides modular route handlers for different C4 phases:
- c4: Core orchestration (status, tasks, submit)
- discovery: Discovery phase (specs, requirements)
- design: Design phase (architecture, decisions)
- validation: Validation execution
- git: Git operations
- files: File operations (read, write, list, search, delete)
- shell: Shell command execution
- workspace: Workspace management
"""

from . import c4, design, discovery, files, git, shell, validation, workspace

__all__ = [
    "c4",
    "discovery",
    "design",
    "validation",
    "git",
    "files",
    "shell",
    "workspace",
]
