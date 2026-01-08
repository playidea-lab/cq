"""C4 Supervisor - Pluggable checkpoint review system

Usage:
    from c4.supervisor import Supervisor, ClaudeCliBackend, MockBackend

    # Default (Claude CLI)
    supervisor = Supervisor(project_root)
    response = supervisor.run_supervisor(bundle_dir)

    # With specific backend
    backend = ClaudeCliBackend(model="claude-3-opus")
    supervisor = Supervisor(project_root, backend=backend)

    # For testing
    backend = MockBackend(decision=SupervisorDecision.APPROVE)
    supervisor = Supervisor(project_root, backend=backend)
"""

from .backend import SupervisorBackend, SupervisorError, SupervisorResponse
from .claude_backend import ClaudeCliBackend
from .mock_backend import MockBackend
from .prompt import PromptRenderer
from .supervisor import Supervisor

__all__ = [
    # Main class
    "Supervisor",
    # Backends
    "SupervisorBackend",
    "ClaudeCliBackend",
    "MockBackend",
    # Supporting classes
    "SupervisorResponse",
    "SupervisorError",
    "PromptRenderer",
]
