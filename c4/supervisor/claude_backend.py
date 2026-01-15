"""Claude CLI Backend - Uses `claude -p` for supervisor review"""

import re
import subprocess
from pathlib import Path

from .backend import SupervisorBackend, SupervisorError, SupervisorResponse
from .response_parser import ResponseParser

# Patterns for sensitive information to mask
SENSITIVE_PATTERNS = [
    # API keys (various formats)
    (r"sk-[a-zA-Z0-9]{20,}", "[MASKED_API_KEY]"),
    (r"sk-ant-[a-zA-Z0-9-]{20,}", "[MASKED_ANTHROPIC_KEY]"),
    (r"ANTHROPIC_API_KEY=[^\s]+", "ANTHROPIC_API_KEY=[MASKED]"),
    (r"api[_-]?key[=:]\s*['\"]?[a-zA-Z0-9-_]{10,}['\"]?", "api_key=[MASKED]"),
    # Tokens
    (r"bearer\s+[a-zA-Z0-9-_.]+", "bearer [MASKED_TOKEN]"),
    (r"token[=:]\s*['\"]?[a-zA-Z0-9-_]{10,}['\"]?", "token=[MASKED]"),
]

MAX_ERROR_MESSAGE_LENGTH = 500


def _sanitize_stderr(stderr: str) -> str:
    """
    Sanitize stderr to remove sensitive information.

    Args:
        stderr: Raw stderr output

    Returns:
        Sanitized stderr with masked sensitive data and limited length
    """
    if not stderr:
        return ""

    sanitized = stderr

    # Apply all masking patterns
    for pattern, replacement in SENSITIVE_PATTERNS:
        sanitized = re.sub(pattern, replacement, sanitized, flags=re.IGNORECASE)

    # Limit length
    if len(sanitized) > MAX_ERROR_MESSAGE_LENGTH:
        sanitized = sanitized[:MAX_ERROR_MESSAGE_LENGTH] + "... [truncated]"

    return sanitized


class ClaudeCliBackend(SupervisorBackend):
    """
    Supervisor backend using Claude CLI (`claude -p`).

    This is the default backend that runs headless Claude for checkpoint review.
    """

    def __init__(
        self,
        working_dir: Path | None = None,
        max_retries: int = 3,
        model: str | None = None,
    ):
        """
        Initialize Claude CLI backend.

        Args:
            working_dir: Working directory for Claude CLI
            max_retries: Maximum retry attempts
            model: Optional model override (e.g., "claude-3-opus")
        """
        self.working_dir = working_dir or Path.cwd()
        self.max_retries = max_retries
        self.model = model

    @property
    def name(self) -> str:
        return "claude-cli"

    def run_review(
        self,
        prompt: str,
        bundle_dir: Path,
        timeout: int = 300,
    ) -> SupervisorResponse:
        """Run review using Claude CLI"""
        # Save prompt to bundle
        (bundle_dir / "prompt.md").write_text(prompt)

        last_error: SupervisorError | None = None

        for attempt in range(self.max_retries):
            try:
                # Build command
                cmd = ["claude", "-p", prompt]
                if self.model:
                    cmd.extend(["--model", self.model])

                # Run claude -p
                result = subprocess.run(
                    cmd,
                    capture_output=True,
                    text=True,
                    timeout=timeout,
                    cwd=self.working_dir,
                )

                if result.returncode != 0:
                    sanitized_err = _sanitize_stderr(result.stderr)
                    last_error = SupervisorError(
                        f"Claude exited with code {result.returncode}: {sanitized_err}"
                    )
                    continue

                # Parse response
                response = self._parse_decision(result.stdout)

                # Save response
                self.save_response(bundle_dir, response)

                return response

            except subprocess.TimeoutExpired:
                last_error = SupervisorError(f"Supervisor timed out after {timeout}s")
            except ValueError as e:
                last_error = SupervisorError(f"Failed to parse response: {e}")
            except FileNotFoundError:
                raise SupervisorError(
                    "Claude CLI not found. Please install claude and ensure it's in PATH."
                )

        raise last_error or SupervisorError("Supervisor failed after retries")

    def _parse_decision(self, output: str) -> SupervisorResponse:
        """Parse supervisor decision from Claude output.

        Delegates to ResponseParser for consistent parsing across backends.
        """
        return ResponseParser.parse(output)
