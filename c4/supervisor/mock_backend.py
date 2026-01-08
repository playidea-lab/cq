"""Mock Backend - For testing without actual LLM calls"""

from pathlib import Path

from ..models import SupervisorDecision
from .backend import SupervisorBackend, SupervisorResponse


class MockBackend(SupervisorBackend):
    """
    Mock supervisor backend for testing.

    Always returns a pre-configured response without making any external calls.
    """

    def __init__(
        self,
        decision: SupervisorDecision = SupervisorDecision.APPROVE,
        notes: str = "Mock approval",
        required_changes: list[str] | None = None,
    ):
        """
        Initialize mock backend.

        Args:
            decision: Decision to return
            notes: Notes to include in response
            required_changes: Required changes (for REQUEST_CHANGES)
        """
        self.decision = decision
        self.notes = notes
        self.required_changes = required_changes or []

    @property
    def name(self) -> str:
        return "mock"

    def run_review(
        self,
        prompt: str,
        bundle_dir: Path,
        timeout: int = 300,
    ) -> SupervisorResponse:
        """Return pre-configured mock response"""
        import json

        # Save prompt (for completeness)
        (bundle_dir / "prompt.md").write_text(prompt)

        # Load checkpoint_id from summary
        summary_file = bundle_dir / "summary.json"
        checkpoint_id = "MOCK"
        if summary_file.exists():
            summary = json.loads(summary_file.read_text())
            checkpoint_id = summary.get("checkpoint_id", "MOCK")

        response = SupervisorResponse(
            decision=self.decision,
            checkpoint_id=checkpoint_id,
            notes=self.notes,
            required_changes=self.required_changes,
        )

        # Save response
        self.save_response(bundle_dir, response)

        return response

    def set_response(
        self,
        decision: SupervisorDecision,
        notes: str = "",
        required_changes: list[str] | None = None,
    ) -> None:
        """Update the mock response for next call"""
        self.decision = decision
        self.notes = notes
        self.required_changes = required_changes or []
