"""C4 API Routes.

Provides modular route handlers for different C4 phases:
- c4: Core orchestration (status, tasks, submit)
- discovery: Discovery phase (specs, requirements)
- design: Design phase (architecture, decisions)
- validation: Validation execution
- git: Git operations
"""

from . import c4, design, discovery, git, validation

__all__ = ["c4", "discovery", "design", "validation", "git"]
