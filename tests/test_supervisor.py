"""Tests for C4D Supervisor - Real subprocess execution"""

import json
import subprocess
import tempfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from c4d.bundle import BundleCreator
from c4d.models import SupervisorDecision
from c4d.supervisor import Supervisor, SupervisorError


@pytest.fixture
def temp_project():
    """Create a temporary project directory"""
    with tempfile.TemporaryDirectory() as tmpdir:
        project = Path(tmpdir)
        c4_dir = project / ".c4"
        c4_dir.mkdir()
        (c4_dir / "bundles").mkdir()
        yield project


@pytest.fixture
def bundle_with_summary(temp_project):
    """Create a bundle directory with summary for testing"""
    bundle_dir = temp_project / ".c4" / "bundles" / "cp-CP1-test"
    bundle_dir.mkdir(parents=True)

    # Create summary.json
    summary = {
        "checkpoint_id": "CP1",
        "timestamp": "2025-01-07T12:00:00",
        "tasks_completed": ["T-001"],
        "files_changed": 5,
        "lines_added": 120,
        "lines_removed": 30,
    }
    (bundle_dir / "summary.json").write_text(json.dumps(summary))

    # Create test_results.json
    test_results = [
        {"name": "lint", "status": "pass", "duration_ms": 100},
        {"name": "unit", "status": "pass", "duration_ms": 500},
    ]
    (bundle_dir / "test_results.json").write_text(json.dumps(test_results))

    # Create changes.diff
    (bundle_dir / "changes.diff").write_text("+ new line\n- old line")

    return bundle_dir


class TestRunSupervisor:
    """Test run_supervisor() with subprocess mocking"""

    @patch("subprocess.run")
    def test_successful_approve(self, mock_run, temp_project, bundle_with_summary):
        """Test successful APPROVE decision from Claude"""
        supervisor = Supervisor(temp_project)

        # Mock Claude CLI output
        mock_run.return_value = MagicMock(
            returncode=0,
            stdout='''I've reviewed the checkpoint. Here is my decision:

```json
{
  "decision": "APPROVE",
  "checkpoint": "CP1",
  "notes": "All tests pass, code looks good",
  "required_changes": []
}
```
''',
            stderr="",
        )

        response = supervisor.run_supervisor(bundle_with_summary)

        assert response.decision == SupervisorDecision.APPROVE
        assert response.checkpoint_id == "CP1"
        assert response.notes == "All tests pass, code looks good"
        assert len(response.required_changes) == 0

        # Verify prompt was saved
        assert (bundle_with_summary / "prompt.md").exists()

        # Verify response was saved
        assert (bundle_with_summary / "response.json").exists()

        # Verify Claude CLI was called correctly
        mock_run.assert_called_once()
        call_args = mock_run.call_args
        assert call_args[0][0][0] == "claude"
        assert call_args[0][0][1] == "-p"

    @patch("subprocess.run")
    def test_successful_request_changes(self, mock_run, temp_project, bundle_with_summary):
        """Test successful REQUEST_CHANGES decision"""
        supervisor = Supervisor(temp_project)

        mock_run.return_value = MagicMock(
            returncode=0,
            stdout='''```json
{
  "decision": "REQUEST_CHANGES",
  "checkpoint": "CP1",
  "notes": "Found some issues",
  "required_changes": ["Fix lint error", "Add tests"]
}
```''',
            stderr="",
        )

        response = supervisor.run_supervisor(bundle_with_summary)

        assert response.decision == SupervisorDecision.REQUEST_CHANGES
        assert len(response.required_changes) == 2
        assert "Fix lint error" in response.required_changes

    @patch("subprocess.run")
    def test_successful_replan(self, mock_run, temp_project, bundle_with_summary):
        """Test successful REPLAN decision"""
        supervisor = Supervisor(temp_project)

        mock_run.return_value = MagicMock(
            returncode=0,
            stdout=(
                '{"decision": "REPLAN", "checkpoint": "CP1", '
                '"notes": "Architecture issue", "required_changes": []}'
            ),
            stderr="",
        )

        response = supervisor.run_supervisor(bundle_with_summary)

        assert response.decision == SupervisorDecision.REPLAN

    @patch("subprocess.run")
    def test_retry_on_parse_failure(self, mock_run, temp_project, bundle_with_summary):
        """Test retry when JSON parsing fails"""
        supervisor = Supervisor(temp_project)

        # First call returns invalid output, second succeeds
        mock_run.side_effect = [
            MagicMock(returncode=0, stdout="I'm not sure what to decide...", stderr=""),
            MagicMock(returncode=0, stdout="Still thinking...", stderr=""),
            MagicMock(
                returncode=0,
                stdout=(
                    '{"decision": "APPROVE", "checkpoint": "CP1", '
                    '"notes": "OK", "required_changes": []}'
                ),
                stderr="",
            ),
        ]

        response = supervisor.run_supervisor(bundle_with_summary, max_retries=3)

        assert response.decision == SupervisorDecision.APPROVE
        assert mock_run.call_count == 3

    @patch("subprocess.run")
    def test_fail_after_max_retries(self, mock_run, temp_project, bundle_with_summary):
        """Test failure after max retries"""
        supervisor = Supervisor(temp_project)

        # All calls return invalid output
        mock_run.return_value = MagicMock(
            returncode=0,
            stdout="I cannot make a decision",
            stderr="",
        )

        with pytest.raises(SupervisorError) as exc_info:
            supervisor.run_supervisor(bundle_with_summary, max_retries=3)

        assert "Failed to parse response" in str(exc_info.value)
        assert mock_run.call_count == 3

    @patch("subprocess.run")
    def test_timeout_handling(self, mock_run, temp_project, bundle_with_summary):
        """Test timeout handling"""
        supervisor = Supervisor(temp_project)

        mock_run.side_effect = subprocess.TimeoutExpired("claude", 300)

        with pytest.raises(SupervisorError) as exc_info:
            supervisor.run_supervisor(bundle_with_summary, timeout=300, max_retries=1)

        assert "timed out" in str(exc_info.value)

    @patch("subprocess.run")
    def test_timeout_retry(self, mock_run, temp_project, bundle_with_summary):
        """Test retry after timeout"""
        supervisor = Supervisor(temp_project)

        # First call times out, second succeeds
        mock_run.side_effect = [
            subprocess.TimeoutExpired("claude", 300),
            MagicMock(
                returncode=0,
                stdout=(
                    '{"decision": "APPROVE", "checkpoint": "CP1", '
                    '"notes": "OK", "required_changes": []}'
                ),
                stderr="",
            ),
        ]

        response = supervisor.run_supervisor(bundle_with_summary, max_retries=2)

        assert response.decision == SupervisorDecision.APPROVE
        assert mock_run.call_count == 2

    @patch("subprocess.run")
    def test_nonzero_exit_code_retry(self, mock_run, temp_project, bundle_with_summary):
        """Test retry on non-zero exit code"""
        supervisor = Supervisor(temp_project)

        # First call fails with non-zero exit, second succeeds
        mock_run.side_effect = [
            MagicMock(returncode=1, stdout="", stderr="Error: Something went wrong"),
            MagicMock(
                returncode=0,
                stdout=(
                    '{"decision": "APPROVE", "checkpoint": "CP1", '
                    '"notes": "OK", "required_changes": []}'
                ),
                stderr="",
            ),
        ]

        response = supervisor.run_supervisor(bundle_with_summary, max_retries=2)

        assert response.decision == SupervisorDecision.APPROVE

    @patch("subprocess.run")
    def test_claude_not_found(self, mock_run, temp_project, bundle_with_summary):
        """Test when Claude CLI is not installed"""
        supervisor = Supervisor(temp_project)

        mock_run.side_effect = FileNotFoundError("claude not found")

        with pytest.raises(SupervisorError) as exc_info:
            supervisor.run_supervisor(bundle_with_summary)

        assert "Claude CLI not found" in str(exc_info.value)


class TestSupervisorPromptRendering:
    """Test prompt rendering with different scenarios"""

    def test_render_prompt_with_custom_template(self, temp_project):
        """Test rendering with custom template from prompts directory"""
        prompts_dir = temp_project / "prompts"
        prompts_dir.mkdir()

        # Create custom template
        custom_template = """# Custom Supervisor Template
Checkpoint: {{ checkpoint_id }}
Tasks: {{ tasks_completed | join(', ') }}
Decision required: APPROVE/REQUEST_CHANGES/REPLAN
"""
        (prompts_dir / "supervisor.md.j2").write_text(custom_template)

        supervisor = Supervisor(temp_project, prompts_dir=prompts_dir)

        prompt = supervisor.render_prompt(
            checkpoint_id="CP1",
            tasks_completed=["T-001", "T-002"],
            test_results=[],
        )

        assert "Custom Supervisor Template" in prompt
        assert "T-001, T-002" in prompt

    def test_render_prompt_fallback_to_default(self, temp_project):
        """Test fallback to default template when file not found"""
        prompts_dir = temp_project / "prompts"
        prompts_dir.mkdir()
        # Don't create the template file

        supervisor = Supervisor(temp_project, prompts_dir=prompts_dir)

        prompt = supervisor.render_prompt(
            checkpoint_id="CP1",
            tasks_completed=["T-001"],
            test_results=[],
        )

        # Should use DEFAULT_TEMPLATE
        assert "CP1" in prompt
        assert "APPROVE" in prompt

    def test_render_prompt_with_validation_results(self, temp_project):
        """Test rendering with validation results"""
        supervisor = Supervisor(temp_project)

        prompt = supervisor.render_prompt(
            checkpoint_id="CP1",
            tasks_completed=["T-001"],
            test_results=[
                {"name": "lint", "status": "pass", "duration_ms": 100},
                {"name": "unit", "status": "pass", "duration_ms": 500},
                {"name": "e2e", "status": "fail", "duration_ms": 3000},
            ],
        )

        assert "lint" in prompt
        assert "unit" in prompt
        assert "e2e" in prompt
        assert "100ms" in prompt
        assert "500ms" in prompt


class TestSupervisorResponseParsing:
    """Test various JSON parsing scenarios"""

    def test_parse_nested_json(self, temp_project):
        """Test parsing JSON with nested structures"""
        supervisor = Supervisor(temp_project)

        output = '''```json
{
  "decision": "REQUEST_CHANGES",
  "checkpoint": "CP1",
  "notes": "Multiple issues found",
  "required_changes": [
    "Fix the authentication flow in auth.py",
    "Add error handling for network failures",
    "Update documentation in README.md"
  ]
}
```'''

        response = supervisor.parse_decision(output)
        assert response.decision == SupervisorDecision.REQUEST_CHANGES
        assert len(response.required_changes) == 3

    def test_parse_json_with_special_characters(self, temp_project):
        """Test parsing JSON with special characters in strings"""
        supervisor = Supervisor(temp_project)

        # Use raw JSON without problematic escapes in the test
        output = '''```json
{
  "decision": "REQUEST_CHANGES",
  "checkpoint": "CP1",
  "notes": "Found issues with special characters: <>&",
  "required_changes": ["Fix the issue with 'single quotes'"]
}
```'''

        response = supervisor.parse_decision(output)
        assert response.decision == SupervisorDecision.REQUEST_CHANGES
        assert "special characters" in response.notes

    def test_parse_json_with_unicode(self, temp_project):
        """Test parsing JSON with unicode characters"""
        supervisor = Supervisor(temp_project)

        output = '''```json
{
  "decision": "APPROVE",
  "checkpoint": "CP1",
  "notes": "코드가 잘 작성되었습니다 ✅",
  "required_changes": []
}
```'''

        response = supervisor.parse_decision(output)
        assert response.decision == SupervisorDecision.APPROVE
        assert "코드가" in response.notes

    def test_parse_json_with_extra_whitespace(self, temp_project):
        """Test parsing JSON with extra whitespace"""
        supervisor = Supervisor(temp_project)

        output = '''

        ```json

        {
            "decision":    "APPROVE"   ,
            "checkpoint":  "CP1",
            "notes": "OK",
            "required_changes": []
        }

        ```

        '''

        response = supervisor.parse_decision(output)
        assert response.decision == SupervisorDecision.APPROVE

    def test_parse_multiple_json_blocks(self, temp_project):
        """Test that first valid JSON block is used"""
        supervisor = Supervisor(temp_project)

        output = '''Here's an example of wrong format:
```json
{"wrong": "format"}
```

And here's the correct decision:
```json
{
  "decision": "APPROVE",
  "checkpoint": "CP1",
  "notes": "Correct one",
  "required_changes": []
}
```'''

        # First JSON block doesn't have "decision" key, so it should fail
        # and fall through to find the valid one
        response = supervisor.parse_decision(output)
        assert response.decision == SupervisorDecision.APPROVE


class TestBundleIntegration:
    """Test bundle creation and supervisor integration"""

    def test_full_bundle_to_supervisor_flow(self, temp_project):
        """Test creating bundle and running supervisor"""
        c4_dir = temp_project / ".c4"
        c4_dir.mkdir(exist_ok=True)
        (c4_dir / "bundles").mkdir(exist_ok=True)

        bundle_creator = BundleCreator(temp_project, c4_dir)
        supervisor = Supervisor(temp_project)

        # Create bundle
        bundle_dir = bundle_creator.create_bundle(
            checkpoint_id="CP1",
            tasks_completed=["T-001", "T-002"],
            validation_results=[
                {"name": "lint", "status": "pass", "duration_ms": 100},
                {"name": "unit", "status": "pass", "duration_ms": 500},
            ],
        )

        # Render prompt from bundle
        prompt = supervisor.render_prompt_from_bundle(bundle_dir)

        assert "CP1" in prompt
        assert "T-001" in prompt
        assert "T-002" in prompt
        assert "lint" in prompt

        # Run mock supervisor
        response = supervisor.run_supervisor_mock(
            bundle_dir,
            mock_decision=SupervisorDecision.APPROVE,
            mock_notes="All good",
        )

        assert response.decision == SupervisorDecision.APPROVE

        # Verify all files exist
        assert (bundle_dir / "summary.json").exists()
        assert (bundle_dir / "changes.diff").exists()
        assert (bundle_dir / "test_results.json").exists()
        assert (bundle_dir / "prompt.md").exists()
        assert (bundle_dir / "response.json").exists()
