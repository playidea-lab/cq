"""Supervisor - Orchestrates checkpoint review with pluggable backends"""

from __future__ import annotations

import json
from pathlib import Path
from typing import TYPE_CHECKING, Any

from .backend import SupervisorBackend, SupervisorResponse
from .prompt import PromptRenderer
from .verifier import VerificationRunner

if TYPE_CHECKING:
    from ..models.config import LLMConfig
    from ..mcp_server import C4Daemon


class Supervisor:
    """
    Supervisor for checkpoint review.
    ...
    """

    def __init__(
        self,
        project_root: Path,
        backend: SupervisorBackend | None = None,
        llm_config: LLMConfig | None = None,
        prompts_dir: Path | None = None,
        template_name: str = "supervisor.md.j2",
        daemon: "C4Daemon | None" = None,
    ):
        """
        Initialize Supervisor.

        Args:
            project_root: Project root directory
            backend: Explicit backend
            llm_config: LLM configuration
            prompts_dir: Directory containing templates
            template_name: Template file name
            daemon: Optional C4Daemon instance for metrics
        """
        self.root = project_root
        self.daemon = daemon
        self.backend = self._resolve_backend(backend, llm_config, daemon)
        self.renderer = PromptRenderer(
            prompts_dir=prompts_dir or (project_root / "prompts"),
            template_name=template_name,
        )

    def _resolve_backend(
        self,
        backend: SupervisorBackend | None,
        llm_config: LLMConfig | None,
        daemon: "C4Daemon | None" = None,
    ) -> SupervisorBackend:
        """
        Resolve backend using priority order.
        """
        if backend is not None:
            return backend

        if llm_config is not None:
            from .backend_factory import create_backend

            return create_backend(llm_config, working_dir=self.root, daemon=daemon)

        c4_dir = self.root / ".c4"
        if c4_dir.exists():
            from .backend_factory import create_backend_from_config_file

            return create_backend_from_config_file(c4_dir, self.root, daemon=daemon)

        from .claude_backend import ClaudeCliBackend

        return ClaudeCliBackend(working_dir=self.root)

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
        if max_retries is not None and hasattr(self.backend, "max_retries"):
            self.backend.max_retries = max_retries

        # Render prompt from bundle
        prompt = self.renderer.render_from_bundle(bundle_dir)

        # Run backend
        return self.backend.run_review(
            prompt=prompt,
            bundle_dir=bundle_dir,
            timeout=timeout,
        )

    def run_supervisor_strict(
        self,
        bundle_dir: Path,
        verifications: list[dict[str, Any]] | None = None,
        timeout: int = 300,
        max_retries: int | None = None,
    ) -> SupervisorResponse:
        """
        Run strict supervisor review with execution verification.

        This is the enhanced review mode that:
        1. Runs execution verifications (HTTP, CLI, etc.)
        2. Saves verification results to bundle
        3. Uses strict reviewer template with detailed checklist

        Args:
            bundle_dir: Path to bundle directory
            verifications: List of verification configs from config.yaml
            timeout: Timeout in seconds
            max_retries: Maximum retry attempts

        Returns:
            SupervisorResponse with decision
        """
        # Update backend max_retries if provided
        if max_retries is not None and hasattr(self.backend, "max_retries"):
            self.backend.max_retries = max_retries

        # Run verifications if configured
        execution_results = None
        if verifications:
            runner = VerificationRunner(verifications)
            results = runner.run_all()
            execution_results = [r.to_dict() for r in results]

            # Save to bundle
            exec_file = bundle_dir / "execution_results.json"
            exec_file.write_text(json.dumps(execution_results, indent=2))

        # Render strict prompt with execution results
        prompt = self.renderer.render_strict_from_bundle(
            bundle_dir,
            execution_results=execution_results,
        )

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
        from .response_parser import ResponseParser

        return ResponseParser.parse(output)

    # Backward compatibility aliases
    def run_supervisor_mock(self, *args, **kwargs) -> SupervisorResponse:
        """Deprecated: Use MockBackend instead"""
        from ..models import SupervisorDecision
        from .mock_backend import MockBackend

        mock_backend = MockBackend(
            decision=kwargs.get("mock_decision", SupervisorDecision.APPROVE),
            notes=kwargs.get("mock_notes", "Mock approval"),
            required_changes=kwargs.get("mock_changes"),
        )
        bundle_dir = args[0] if args else kwargs.get("bundle_dir")
        prompt = self.renderer.render_from_bundle(bundle_dir)
        return mock_backend.run_review(prompt, bundle_dir)
