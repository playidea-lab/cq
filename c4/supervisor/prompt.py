"""Supervisor Prompt - Template rendering for supervisor review"""

import json
from pathlib import Path
from typing import Any

from jinja2 import Environment, FileSystemLoader, Template

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


STRICT_REVIEWER_TEMPLATE = """# C4 Supervisor Strict Review - {{ checkpoint_id }}

## Role

You are a **VERY STRICT code reviewer**. Your job is to catch issues that others might miss.
If ANY of the following criteria are not met, you MUST choose **REQUEST_CHANGES**.

---

## Mandatory Checklist

### 1. Code Quality
- [ ] All lint errors resolved
- [ ] No unnecessary comments or debug code (console.log, print, etc.)
- [ ] Clear and consistent naming conventions
- [ ] Functions/classes are appropriately sized (< 100 lines recommended)
- [ ] No code duplication

### 2. Testing
- [ ] All unit tests pass
- [ ] New functionality has corresponding tests
- [ ] Edge cases are tested
- [ ] Test coverage is maintained or improved

### 3. Security
- [ ] No hardcoded secrets (API keys, passwords, tokens)
- [ ] Input validation is present where needed
- [ ] SQL injection prevention (parameterized queries)
- [ ] XSS prevention (for web applications)

### 4. Requirements Fulfillment
- [ ] Definition of Done (DoD) is satisfied
- [ ] Implementation matches EARS specification
- [ ] Edge cases are handled

### 5. Execution Verification
{% if execution_results %}
{% for result in execution_results %}
- **{{ result.type }}** ({{ result.name }}): {{ result.status }} - {{ result.summary }}
{% endfor %}
{% else %}
- (No execution verification configured)
{% endif %}

---

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
- {{ r.name }}: {{ r.status }}{% if r.duration_ms %} ({{ r.duration_ms }}ms){% endif %}
{%- if r.message %} - {{ r.message }}{% endif %}
{% endfor %}

## Diff
```diff
{{ diff_preview }}
```

---

## Decision Criteria

### APPROVE (ALL of the following must be true):
- Checklist 100% satisfied
- All tests pass
- Execution verification successful (if configured)
- No security issues

### REQUEST_CHANGES (ANY of the following):
- Checklist partially unsatisfied
- Test failures
- Code quality issues found
- Execution verification failed
- Security concerns identified

### REPLAN (Only for):
- Fundamental design flaws
- Requirements need redefinition
- Architecture changes required

---

## Response Format

You MUST respond with a valid JSON object ONLY:

```json
{
  "decision": "REQUEST_CHANGES",
  "checkpoint": "{{ checkpoint_id }}",
  "checklist_score": "17/20",
  "notes": "Description of issues found",
  "required_changes": [
    "Specific change item 1 with file:line reference",
    "Specific change item 2 with file:line reference"
  ]
}
```

**IMPORTANT**: Be specific in required_changes. Include file names and line numbers where possible.
"""


class PromptRenderer:
    """Renders supervisor review prompts from templates"""

    def __init__(
        self,
        prompts_dir: Path | None = None,
        template_name: str = "supervisor.md.j2",
    ):
        """
        Initialize prompt renderer.

        Args:
            prompts_dir: Directory containing template files
            template_name: Name of the template file
        """
        self.prompts_dir = prompts_dir
        self.template_name = template_name
        self._env: Environment | None = None

    @property
    def env(self) -> Environment:
        """Jinja2 environment with template directory"""
        if self._env is None:
            if self.prompts_dir and self.prompts_dir.exists():
                self._env = Environment(
                    loader=FileSystemLoader(str(self.prompts_dir)),
                    autoescape=False,
                )
            else:
                self._env = Environment(autoescape=False)
        return self._env

    def render(
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
        Render the supervisor prompt from template.

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
            template = Template(DEFAULT_TEMPLATE)
            return template.render(**context)

    def render_from_bundle(self, bundle_dir: Path) -> str:
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

        return self.render(
            checkpoint_id=summary["checkpoint_id"],
            tasks_completed=summary["tasks_completed"],
            test_results=test_results,
            files_changed=summary["files_changed"],
            lines_added=summary["lines_added"],
            lines_removed=summary["lines_removed"],
            diff_content=diff_content,
        )

    def render_strict(
        self,
        checkpoint_id: str,
        tasks_completed: list[str],
        test_results: list[dict],
        files_changed: int = 0,
        lines_added: int = 0,
        lines_removed: int = 0,
        diff_content: str = "",
        execution_results: list[dict[str, Any]] | None = None,
    ) -> str:
        """
        Render the strict supervisor prompt with execution verification.

        This template is more rigorous and includes:
        - Detailed checklist (code quality, testing, security, requirements)
        - Execution verification results
        - Stricter decision criteria

        Args:
            checkpoint_id: ID of the checkpoint
            tasks_completed: List of completed task IDs
            test_results: List of validation results
            files_changed: Number of files changed
            lines_added: Lines added
            lines_removed: Lines removed
            diff_content: Full diff content
            execution_results: List of execution verification results

        Returns:
            Rendered strict prompt string
        """
        # Prepare diff preview (truncate if too long)
        diff_preview = diff_content[:3000] if diff_content else "(no changes)"
        if len(diff_content) > 3000:
            diff_preview += f"\n\n... ({len(diff_content) - 3000} more characters)"

        context = {
            "checkpoint_id": checkpoint_id,
            "tasks_completed": tasks_completed,
            "test_results": test_results,
            "files_changed": files_changed,
            "lines_added": lines_added,
            "lines_removed": lines_removed,
            "diff_preview": diff_preview,
            "execution_results": execution_results or [],
        }

        template = Template(STRICT_REVIEWER_TEMPLATE)
        return template.render(**context)

    def render_strict_from_bundle(
        self,
        bundle_dir: Path,
        execution_results: list[dict[str, Any]] | None = None,
    ) -> str:
        """
        Render strict prompt from a bundle directory.

        Args:
            bundle_dir: Path to bundle directory
            execution_results: List of execution verification results

        Returns:
            Rendered strict prompt string
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

        # Load execution results from bundle if not provided
        if execution_results is None:
            exec_file = bundle_dir / "execution_results.json"
            if exec_file.exists():
                execution_results = json.loads(exec_file.read_text())

        return self.render_strict(
            checkpoint_id=summary["checkpoint_id"],
            tasks_completed=summary["tasks_completed"],
            test_results=test_results,
            files_changed=summary["files_changed"],
            lines_added=summary["lines_added"],
            lines_removed=summary["lines_removed"],
            diff_content=diff_content,
            execution_results=execution_results,
        )
