"""C4 API Routes.

Provides modular route handlers for different C4 phases:
- c4: Core orchestration (status, tasks, submit)
- discovery: Discovery phase (specs, requirements)
- design: Design phase (architecture, decisions)
- validation: Validation execution
- git: Git operations
- shell: Shell command execution
- workspace: Workspace management
"""

from . import c4, design, discovery, git, shell, validation, workspace

__all__ = ["c4", "discovery", "design", "validation", "git", "shell", "workspace"]
