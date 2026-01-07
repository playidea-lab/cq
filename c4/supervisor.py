"""C4D Supervisor - Headless Claude execution for checkpoint review"""

import json
import re
import subprocess
from dataclasses import dataclass
from pathlib import Path

from jinja2 import Environment, FileSystemLoader, Template

from .models import SupervisorDecision


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


class SupervisorError(Exception):
    """Error during Supervisor execution"""

    pass


class Supervisor:
    """Execute headless Claude as Supervisor for checkpoint review"""

    DEFAULT_TEMPLATE = """# C4 Supervisor Review - {{ checkpoint_id }}

## Checkpoint Overview
You are reviewing checkpoint **{{ checkpoint_id }}** for the C4 project.

## Completed Tasks
{% for task in tasks_completed %}
- [x] {{ task }}
{% endfor %}

## Changes Summary
- Files changed: {{ files_changed }}
- Lines added: +{{ lines_added }}
- Lines removed: -{{ lines_removed }}

## Validation Results
{% for r in test_results %}
- {{ r.name }}: {{ r.status }}{% if r.duration_ms %} ({{ r.duration_ms }}ms){%- endif %}
{% endfor %}

## Diff Summary
```diff
{{ diff_preview }}
```

## Your Task

Review the changes and make a decision:

1. **APPROVE** - All requirements met, tests pass, code quality acceptable
2. **REQUEST_CHANGES** - Minor issues found, specify what needs to be fixed
3. **REPLAN** - Major issues require returning to planning phase

**IMPORTANT**: You MUST respond with a valid JSON object and nothing else:

```json
{
  "decision": "APPROVE",
  "checkpoint": "{{ checkpoint_id }}",
  "notes": "All tests pass, code looks good",
  "required_changes": []
}
```

Or for REQUEST_CHANGES:

```json
{
  "decision": "REQUEST_CHANGES",
  "checkpoint": "{{ checkpoint_id }}",
  "notes": "Found some issues",
  "required_changes": ["Fix lint errors in module.py", "Add tests for edge case"]
}
```
"""

    def __init__(
        self,
        project_root: Path,
        prompts_dir: Path | None = None,
        template_name: str = "supervisor.md.j2",
    ):
        self.root = project_root
        self.prompts_dir = prompts_dir or (project_root / "prompts")
        self.template_name = template_name
        self._env: Environment | None = None

    @property
    def env(self) -> Environment:
        """Jinja2 environment with template directory"""
        if self._env is None:
            if self.prompts_dir.exists():
                self._env = Environment(
                    loader=FileSystemLoader(str(self.prompts_dir)),
                    autoescape=False,
                )
            else:
                # Use default template
                self._env = Environment(autoescape=False)
        return self._env

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
        """
        Render the Supervisor prompt from template.

        Args:
            checkpoint_id: ID of the checkpoint
            tasks_completed: List of completed task IDs
            test_results: List of validation results
            files_changed: Number of files changed
            lines_added: Lines added
            lines_removed: Lines removed
            diff_content: Full diff content

        Returns:
            Rendered prompt string
        """
        # Prepare diff preview (truncate if too long)
        diff_preview = diff_content[:2000] if diff_content else "(no changes)"
        if len(diff_content) > 2000:
            diff_preview += f"\n\n... ({len(diff_content) - 2000} more characters)"

        context = {
            "checkpoint_id": checkpoint_id,
            "tasks_completed": tasks_completed,
            "test_results": test_results,
            "files_changed": files_changed,
            "lines_added": lines_added,
            "lines_removed": lines_removed,
            "diff_preview": diff_preview,
        }

        # Try to load template from file
        try:
            template = self.env.get_template(self.template_name)
            return template.render(**context)
        except Exception:
            # Fall back to default template
            template = Template(self.DEFAULT_TEMPLATE)
            return template.render(**context)

    def render_prompt_from_bundle(self, bundle_dir: Path) -> str:
        """
        Render prompt from a bundle directory.

        Args:
            bundle_dir: Path to bundle directory

        Returns:
            Rendered prompt string
        """
        # Load summary
        summary_file = bundle_dir / "summary.json"
        if not summary_file.exists():
            raise FileNotFoundError(f"summary.json not found in {bundle_dir}")

        summary = json.loads(summary_file.read_text())

        # Load test results
        test_results_file = bundle_dir / "test_results.json"
        test_results = []
        if test_results_file.exists():
            test_results = json.loads(test_results_file.read_text())

        # Load diff
        diff_file = bundle_dir / "changes.diff"
        diff_content = ""
        if diff_file.exists():
            diff_content = diff_file.read_text()

        return self.render_prompt(
            checkpoint_id=summary["checkpoint_id"],
            tasks_completed=summary["tasks_completed"],
            test_results=test_results,
            files_changed=summary["files_changed"],
            lines_added=summary["lines_added"],
            lines_removed=summary["lines_removed"],
            diff_content=diff_content,
        )

    def parse_decision(self, output: str) -> SupervisorResponse:
        """
        Parse Supervisor decision from Claude output.

        Args:
            output: Raw output from Claude

        Returns:
            Parsed SupervisorResponse

        Raises:
            ValueError: If no valid JSON found
        """
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
                # Need to be careful with nested braces
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

    def _extract_json_object(self, text: str, start: int) -> str:
        """Extract a complete JSON object starting at given position"""
        depth = 0
        in_string = False
        escape = False
        result = []

        for i, char in enumerate(text[start:], start):
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

    def run_supervisor(
        self,
        bundle_dir: Path,
        timeout: int = 300,
        max_retries: int = 3,
    ) -> SupervisorResponse:
        """
        Run headless Claude as Supervisor.

        Args:
            bundle_dir: Path to bundle directory
            timeout: Timeout in seconds
            max_retries: Maximum retry attempts

        Returns:
            SupervisorResponse with decision

        Raises:
            SupervisorError: If Supervisor execution fails
        """
        # Render prompt
        prompt = self.render_prompt_from_bundle(bundle_dir)

        # Save prompt to bundle
        (bundle_dir / "prompt.md").write_text(prompt)

        last_error = None

        for attempt in range(max_retries):
            try:
                # Run claude -p
                result = subprocess.run(
                    ["claude", "-p", prompt],
                    capture_output=True,
                    text=True,
                    timeout=timeout,
                    cwd=self.root,
                )

                if result.returncode != 0:
                    last_error = SupervisorError(
                        f"Claude exited with code {result.returncode}: {result.stderr}"
                    )
                    continue

                # Parse response
                response = self.parse_decision(result.stdout)

                # Save response to bundle
                (bundle_dir / "response.json").write_text(
                    json.dumps(
                        {
                            "decision": response.decision.value,
                            "checkpoint": response.checkpoint_id,
                            "notes": response.notes,
                            "required_changes": response.required_changes,
                        },
                        indent=2,
                    )
                )

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

    def run_supervisor_mock(
        self,
        bundle_dir: Path,
        mock_decision: SupervisorDecision = SupervisorDecision.APPROVE,
        mock_notes: str = "Mock approval",
        mock_changes: list[str] | None = None,
    ) -> SupervisorResponse:
        """
        Run mock Supervisor (for testing without Claude CLI).

        Args:
            bundle_dir: Path to bundle directory
            mock_decision: Decision to return
            mock_notes: Notes to include
            mock_changes: Required changes (for REQUEST_CHANGES)

        Returns:
            SupervisorResponse with mock decision
        """
        # Render prompt (for completeness)
        prompt = self.render_prompt_from_bundle(bundle_dir)
        (bundle_dir / "prompt.md").write_text(prompt)

        # Load summary for checkpoint_id
        summary = json.loads((bundle_dir / "summary.json").read_text())

        response = SupervisorResponse(
            decision=mock_decision,
            checkpoint_id=summary["checkpoint_id"],
            notes=mock_notes,
            required_changes=mock_changes or [],
        )

        # Save response
        (bundle_dir / "response.json").write_text(
            json.dumps(
                {
                    "decision": response.decision.value,
                    "checkpoint": response.checkpoint_id,
                    "notes": response.notes,
                    "required_changes": response.required_changes,
                },
                indent=2,
            )
        )

        return response
