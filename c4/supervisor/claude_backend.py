"""Claude CLI Backend - Uses `claude -p` for supervisor review"""

import json
import re
import subprocess
from pathlib import Path

from .backend import SupervisorBackend, SupervisorError, SupervisorResponse


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
                    last_error = SupervisorError(
                        f"Claude exited with code {result.returncode}: {result.stderr}"
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
        """Parse supervisor decision from Claude output"""
        # Method 1: Try to find JSON in code block
        match = re.search(r"```json\s*(\{.*?\})\s*```", output, re.DOTALL)
        if match:
            try:
                data = json.loads(match.group(1))
                return SupervisorResponse.from_dict(data)
            except (json.JSONDecodeError, ValueError):
                pass

        # Method 2: Try to find raw JSON with decision key
        match = re.search(r'\{\s*"decision"\s*:.*?\}', output, re.DOTALL)
        if match:
            try:
                json_str = self._extract_json_object(output, match.start())
                data = json.loads(json_str)
                return SupervisorResponse.from_dict(data)
            except (json.JSONDecodeError, ValueError):
                pass

        # Method 3: Try to parse the entire output as JSON
        try:
            data = json.loads(output.strip())
            return SupervisorResponse.from_dict(data)
        except (json.JSONDecodeError, ValueError):
            pass

        raise ValueError(f"No valid JSON found in supervisor output: {output[:200]}...")

    def _extract_json_object(self, text: str, start: int, max_length: int = 10000) -> str:
        """Extract a complete JSON object starting at given position"""
        depth = 0
        in_string = False
        escape = False
        result = []

        for i, char in enumerate(text[start:start + max_length], start):
            result.append(char)

            if escape:
                escape = False
                continue

            if char == "\\":
                escape = True
                continue

            if char == '"' and not escape:
                in_string = not in_string
                continue

            if in_string:
                continue

            if char == "{":
                depth += 1
            elif char == "}":
                depth -= 1
                if depth == 0:
                    return "".join(result)

        return "".join(result)
