"""C4 Subagents - Lightweight specialized agents.

Subagents are focused, single-purpose agents that handle specific tasks
with minimal token overhead. They run with smaller models (Haiku) and
return compressed context for downstream processing.

Available subagents:
- Scout: Code exploration and symbol extraction

Prompt templates for Worker, Reviewer, and Planner agents are available
via the load_prompt() function.
"""

from .base import SubagentBase
from .prompts import load_prompt
from .scout import Scout

__all__ = ["SubagentBase", "Scout", "load_prompt"]
