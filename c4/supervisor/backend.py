"""Supervisor Backend - Abstract base class for supervisor implementations"""

from abc import ABC, abstractmethod
from dataclasses import dataclass
from pathlib import Path

from ..models import SupervisorDecision


@dataclass
class SupervisorResponse:
    """Response from Supervisor review"""

    decision: SupervisorDecision
    checkpoint_id: str
    notes: str
    required_changes: list[str]

    @classmethod
    def from_dict(cls, data: dict) -> "SupervisorResponse":
        decision_str = data.get("decision", "").upper()
        try:
            decision = SupervisorDecision(decision_str)
        except ValueError:
            raise ValueError(f"Invalid decision: {decision_str}")

        return cls(
            decision=decision,
            checkpoint_id=data.get("checkpoint", data.get("checkpoint_id", "")),
            notes=data.get("notes", ""),
            required_changes=data.get("required_changes", []),
        )

    def to_dict(self) -> dict:
        """Convert to dictionary"""
        return {
            "decision": self.decision.value,
            "checkpoint": self.checkpoint_id,
            "notes": self.notes,
            "required_changes": self.required_changes,
        }


class SupervisorError(Exception):
    """Error during Supervisor execution"""

    pass


class SupervisorBackend(ABC):
    """
    Abstract base class for Supervisor backends.

    Implement this interface to use different LLMs/agents as supervisors:
    - ClaudeCliBackend: Uses `claude -p` CLI
    - MockBackend: For testing
    - OpenAIBackend: Uses OpenAI API (future)
    - CodexBackend: Uses Codex CLI (future)
    - MCPBackend: Uses any MCP-compatible agent (future)
    """

    @abstractmethod
    def run_review(
        self,
        prompt: str,
        bundle_dir: Path,
        timeout: int = 300,
    ) -> SupervisorResponse:
        """
        Run supervisor review with the given prompt.

        Args:
            prompt: Rendered review prompt
            bundle_dir: Path to bundle directory (for saving artifacts)
            timeout: Timeout in seconds

        Returns:
            SupervisorResponse with decision

        Raises:
            SupervisorError: If review fails
        """
        pass

    @property
    @abstractmethod
    def name(self) -> str:
        """Backend name for logging/debugging"""
        pass

    def save_response(self, bundle_dir: Path, response: SupervisorResponse) -> None:
        """Save response to bundle directory"""
        import json
        (bundle_dir / "response.json").write_text(
            json.dumps(response.to_dict(), indent=2)
        )
