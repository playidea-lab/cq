"""Supervisor - Orchestrates checkpoint review with pluggable backends"""

from pathlib import Path

from .backend import SupervisorBackend, SupervisorResponse
from .claude_backend import ClaudeCliBackend
from .prompt import PromptRenderer


class Supervisor:
    """
    Supervisor for checkpoint review.

    Uses pluggable backends to support different LLM providers:
    - ClaudeCliBackend (default)
    - MockBackend (testing)
    - Future: OpenAIBackend, CodexBackend, MCPBackend
    """

    def __init__(
        self,
        project_root: Path,
        backend: SupervisorBackend | None = None,
        prompts_dir: Path | None = None,
        template_name: str = "supervisor.md.j2",
    ):
        """
        Initialize Supervisor.

        Args:
            project_root: Project root directory
            backend: Supervisor backend (default: ClaudeCliBackend)
            prompts_dir: Directory containing prompt templates
            template_name: Template file name
        """
        self.root = project_root
        self.backend = backend or ClaudeCliBackend(working_dir=project_root)
        self.renderer = PromptRenderer(
            prompts_dir=prompts_dir or (project_root / "prompts"),
            template_name=template_name,
        )

    def run_supervisor(
        self,
        bundle_dir: Path,
        timeout: int = 300,
        max_retries: int | None = None,
    ) -> SupervisorResponse:
        """
        Run supervisor review on a checkpoint bundle.

        Args:
            bundle_dir: Path to bundle directory
            timeout: Timeout in seconds
            max_retries: Maximum retry attempts (updates backend if provided)

        Returns:
            SupervisorResponse with decision
        """
        # Update backend max_retries if provided (backward compatibility)
        if max_retries is not None and hasattr(self.backend, 'max_retries'):
            self.backend.max_retries = max_retries

        # Render prompt from bundle
        prompt = self.renderer.render_from_bundle(bundle_dir)

        # Run backend
        return self.backend.run_review(
            prompt=prompt,
            bundle_dir=bundle_dir,
            timeout=timeout,
        )

    def render_prompt(
        self,
        checkpoint_id: str,
        tasks_completed: list[str],
        test_results: list[dict],
        files_changed: int = 0,
        lines_added: int = 0,
        lines_removed: int = 0,
        diff_content: str = "",
    ) -> str:
        """Render prompt (delegate to renderer)"""
        return self.renderer.render(
            checkpoint_id=checkpoint_id,
            tasks_completed=tasks_completed,
            test_results=test_results,
            files_changed=files_changed,
            lines_added=lines_added,
            lines_removed=lines_removed,
            diff_content=diff_content,
        )

    def render_prompt_from_bundle(self, bundle_dir: Path) -> str:
        """Render prompt from bundle (delegate to renderer)"""
        return self.renderer.render_from_bundle(bundle_dir)

    def parse_decision(self, output: str) -> SupervisorResponse:
        """
        Parse supervisor decision from LLM output.

        Args:
            output: Raw output from LLM

        Returns:
            Parsed SupervisorResponse
        """
        # Delegate to backend if it has the method
        if hasattr(self.backend, '_parse_decision'):
            return self.backend._parse_decision(output)

        # Otherwise use ClaudeCliBackend's parsing logic
        from .claude_backend import ClaudeCliBackend
        parser = ClaudeCliBackend()
        return parser._parse_decision(output)

    # Backward compatibility aliases
    def run_supervisor_mock(self, *args, **kwargs) -> SupervisorResponse:
        """Deprecated: Use MockBackend instead"""
        from .mock_backend import MockBackend

        mock_backend = MockBackend(
            decision=kwargs.get("mock_decision"),
            notes=kwargs.get("mock_notes", "Mock approval"),
            required_changes=kwargs.get("mock_changes"),
        )
        bundle_dir = args[0] if args else kwargs.get("bundle_dir")
        prompt = self.renderer.render_from_bundle(bundle_dir)
        return mock_backend.run_review(prompt, bundle_dir)
