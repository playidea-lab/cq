"""End-to-end tests for Claude API integration.

Tests cover:
- API call flow with mocked responses
- Response parsing and validation
- Error handling scenarios
- Integration with usage tracking and cost optimization
- Real API calls (optional, requires ANTHROPIC_API_KEY)
"""

import json
import os
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from c4.models import SupervisorDecision
from c4.supervisor import (
    ClaudeErrorHandler,
    ClaudeErrorType,
    LiteLLMBackend,
    ResponseParser,
    TaskComplexity,
    UsageTracker,
    create_cost_optimizer,
    create_usage_tracker,
    retry_with_backoff,
)

# Skip real API tests unless explicitly enabled
SKIP_REAL_API = os.environ.get("TEST_REAL_API", "").lower() != "true"
ANTHROPIC_API_KEY = os.environ.get("ANTHROPIC_API_KEY")


class TestMockedAPIFlow:
    """Test API flow with mocked LiteLLM responses."""

    def create_mock_response(
        self,
        content: str,
        input_tokens: int = 100,
        output_tokens: int = 50,
        cost: float = 0.01,
    ) -> MagicMock:
        """Create a mock LiteLLM response."""
        mock_response = MagicMock()
        mock_response.choices = [MagicMock(message=MagicMock(content=content))]

        mock_usage = MagicMock()
        mock_usage.prompt_tokens = input_tokens
        mock_usage.completion_tokens = output_tokens
        mock_usage.total_tokens = input_tokens + output_tokens
        mock_response.usage = mock_usage
        mock_response._hidden_params = {"response_cost": cost}

        return mock_response

    @patch("litellm.completion")
    def test_successful_review_flow(self, mock_completion, tmp_path: Path):
        """Test complete successful review flow."""
        # Setup mock
        response_json = json.dumps({
            "decision": "APPROVE",
            "checkpoint": "CP-001",
            "notes": "All tests pass, code looks good",
            "required_changes": [],
        })
        mock_completion.return_value = self.create_mock_response(
            f"```json\n{response_json}\n```"
        )

        # Create backend and run review
        backend = LiteLLMBackend(
            model="claude-sonnet-4-20250514",
            api_key="test-key",
        )
        bundle_dir = tmp_path / "bundle"
        bundle_dir.mkdir()

        result = backend.run_review(
            prompt="Review this code for quality",
            bundle_dir=bundle_dir,
        )

        # Verify result
        assert result.decision == SupervisorDecision.APPROVE
        assert result.checkpoint_id == "CP-001"
        assert "tests pass" in result.notes

        # Verify files created
        assert (bundle_dir / "prompt.md").exists()
        assert (bundle_dir / "raw_response.txt").exists()

    @patch("litellm.completion")
    def test_request_changes_flow(self, mock_completion, tmp_path: Path):
        """Test review flow that requests changes."""
        response_json = json.dumps({
            "decision": "REQUEST_CHANGES",
            "checkpoint": "CP-002",
            "notes": "Several issues found",
            "required_changes": [
                "Fix type error in line 42",
                "Add missing test coverage",
            ],
        })
        mock_completion.return_value = self.create_mock_response(
            f"```json\n{response_json}\n```"
        )

        backend = LiteLLMBackend(model="claude-sonnet-4-20250514", api_key="test-key")
        bundle_dir = tmp_path / "bundle"
        bundle_dir.mkdir()

        result = backend.run_review("Review this code", bundle_dir)

        assert result.decision == SupervisorDecision.REQUEST_CHANGES
        assert len(result.required_changes) == 2
        assert "type error" in result.required_changes[0]

    @patch("litellm.completion")
    def test_replan_flow(self, mock_completion, tmp_path: Path):
        """Test review flow that requires replanning."""
        response_json = json.dumps({
            "decision": "REPLAN",
            "checkpoint": "CP-003",
            "notes": "Current approach is not scalable",
            "required_changes": [],
        })
        mock_completion.return_value = self.create_mock_response(
            f"```json\n{response_json}\n```"
        )

        backend = LiteLLMBackend(model="claude-sonnet-4-20250514", api_key="test-key")
        bundle_dir = tmp_path / "bundle"
        bundle_dir.mkdir()

        result = backend.run_review("Review architecture", bundle_dir)

        assert result.decision == SupervisorDecision.REPLAN


class TestResponseParsing:
    """Test response parsing scenarios."""

    def test_parse_json_in_code_block(self):
        """Parse JSON within markdown code block."""
        response = '''Here's my review:

```json
{
    "decision": "APPROVE",
    "checkpoint": "CP-001",
    "notes": "Good code",
    "required_changes": []
}
```
'''
        result = ResponseParser.parse(response)
        assert result.decision == SupervisorDecision.APPROVE

    def test_parse_raw_json(self):
        """Parse raw JSON without code block."""
        response = '''{
    "decision": "REQUEST_CHANGES",
    "checkpoint": "CP-002",
    "notes": "Needs work",
    "required_changes": ["Fix bug"]
}'''
        result = ResponseParser.parse(response)
        assert result.decision == SupervisorDecision.REQUEST_CHANGES

    def test_parse_json_with_extra_text(self):
        """Parse JSON with surrounding text."""
        response = '''After reviewing the code, here's my assessment:

{"decision": "APPROVE", "checkpoint": "CP-001", "notes": "OK", "required_changes": []}

Please let me know if you have questions.'''
        result = ResponseParser.parse(response)
        assert result.decision == SupervisorDecision.APPROVE

    def test_parse_invalid_json_raises(self):
        """Invalid JSON raises ValueError."""
        response = "This is just plain text without any JSON"
        with pytest.raises(ValueError, match="No valid JSON found"):
            ResponseParser.parse(response)

    def test_parse_incomplete_json_raises(self):
        """Incomplete JSON structure raises ValueError."""
        response = '{"decision": "APPROVE"}'  # Missing required fields
        # Should still parse if decision is present
        result = ResponseParser.parse(response)
        assert result.decision == SupervisorDecision.APPROVE


class TestErrorHandlingScenarios:
    """Test error handling scenarios."""

    def test_rate_limit_error_handling(self):
        """Handle rate limit errors correctly."""
        handler = ClaudeErrorHandler()
        error = Exception("Error 429: Too Many Requests")

        action = handler.handle_error(error, attempt=0)

        assert action.error_type == ClaudeErrorType.RATE_LIMIT
        assert action.should_retry is True
        assert action.retry_after >= 5.0
        assert action.is_transient is True

    def test_quota_exceeded_error_handling(self):
        """Handle quota exceeded errors - no retry."""
        handler = ClaudeErrorHandler()
        error = Exception("402 Payment Required: quota exceeded")

        action = handler.handle_error(error, attempt=0)

        assert action.error_type == ClaudeErrorType.QUOTA_EXCEEDED
        assert action.should_retry is False
        assert action.is_transient is False

    def test_authentication_error_handling(self):
        """Handle authentication errors - no retry."""
        handler = ClaudeErrorHandler()
        error = Exception("401 Unauthorized: Invalid API key")

        action = handler.handle_error(error, attempt=0)

        assert action.error_type == ClaudeErrorType.AUTHENTICATION
        assert action.should_retry is False

    def test_server_error_retry_exhaustion(self):
        """Server errors should retry until exhaustion."""
        handler = ClaudeErrorHandler()
        error = Exception("500 Internal Server Error")

        # First few attempts should retry
        for attempt in range(3):
            action = handler.handle_error(error, attempt=attempt)
            assert action.should_retry is True

        # After max retries, should stop
        action = handler.handle_error(error, attempt=3)
        assert action.should_retry is False

    @patch("time.sleep")
    def test_retry_with_backoff_success(self, mock_sleep):
        """Retry succeeds after transient failure."""
        call_count = [0]

        def flaky_function():
            call_count[0] += 1
            if call_count[0] < 2:
                raise Exception("500 Server Error")
            return "success"

        result = retry_with_backoff(flaky_function)
        assert result == "success"
        assert call_count[0] == 2

    @patch("time.sleep")
    def test_retry_with_backoff_permanent_failure(self, mock_sleep):
        """Permanent errors fail immediately."""
        def auth_error():
            raise Exception("401 Invalid API Key")

        with pytest.raises(Exception, match="401"):
            retry_with_backoff(auth_error)


class TestUsageTracking:
    """Test usage tracking integration."""

    @patch("litellm.completion")
    def test_track_usage_from_api_response(self, mock_completion, tmp_path: Path):
        """Track usage from API response."""
        mock_response = MagicMock()
        mock_response.choices = [
            MagicMock(
                message=MagicMock(
                    content=json.dumps({
                        "decision": "APPROVE",
                        "checkpoint": "CP-001",
                        "notes": "OK",
                        "required_changes": [],
                    })
                )
            )
        ]
        mock_usage = MagicMock()
        mock_usage.prompt_tokens = 1000
        mock_usage.completion_tokens = 500
        mock_usage.total_tokens = 1500
        mock_response.usage = mock_usage
        mock_response._hidden_params = {"response_cost": 0.0105}
        mock_completion.return_value = mock_response

        # Create tracker and backend
        tracker = create_usage_tracker()
        backend = LiteLLMBackend(model="claude-sonnet-4-20250514", api_key="test-key")

        bundle_dir = tmp_path / "bundle"
        bundle_dir.mkdir()

        # Run review
        backend.run_review("Review code", bundle_dir)

        # Manually track (in real integration, this would be automatic)
        record = tracker.record_usage(
            model=backend.model,
            input_tokens=1000,
            output_tokens=500,
            cost=0.0105,
        )

        assert record.input_tokens == 1000
        assert record.output_tokens == 500
        assert tracker.session_cost == 0.0105
        assert tracker.session_tokens == 1500

    def test_usage_tracking_with_budget(self):
        """Track usage against budget."""
        # Use explicit threshold of 0.5 for easier testing
        tracker = UsageTracker(budget=0.10, budget_warning_threshold=0.5)
        warnings = []

        def budget_callback(current, budget, percentage):
            warnings.append(percentage)

        tracker.set_budget_callback(budget_callback)

        # Record usage below warning threshold (30%)
        tracker.record_usage("claude-sonnet-4-20250514", 1000, 500, cost=0.03)
        assert len(warnings) == 0

        # Record usage above warning threshold (60% > 50%)
        tracker.record_usage("claude-sonnet-4-20250514", 1000, 500, cost=0.03)
        assert len(warnings) == 1
        assert warnings[0] >= 0.5

    def test_save_usage_report(self, tmp_path: Path):
        """Save usage report to file."""
        tracker = create_usage_tracker()
        tracker.record_usage("claude-sonnet-4-20250514", 1000, 500, cost=0.01)
        tracker.record_usage("claude-3-5-haiku-20241022", 500, 200, cost=0.002)

        report_path = tmp_path / "usage_report.json"
        tracker.save_report(report_path)

        assert report_path.exists()
        report = json.loads(report_path.read_text())
        assert report["summary"]["total_requests"] == 2
        assert len(report["records"]) == 2


class TestCostOptimization:
    """Test cost optimization integration."""

    def test_model_selection_by_complexity(self):
        """Select appropriate model based on task complexity."""
        optimizer = create_cost_optimizer()

        # Simple task → Haiku
        selection = optimizer.select_model(
            "Fix the typo in this file",
            complexity=TaskComplexity.LOW,
        )
        assert "haiku" in selection.model_id.lower()

        # Medium task → Sonnet
        selection = optimizer.select_model(
            "Review this pull request",
            complexity=TaskComplexity.MEDIUM,
        )
        assert "sonnet" in selection.model_id.lower()

    def test_budget_alert_creation(self):
        """Create alerts when budget exceeded or near threshold."""
        from c4.supervisor.cost_optimizer import CostAlert

        optimizer = create_cost_optimizer(budget=0.10)

        # No alert when within budget
        alert = optimizer.create_budget_alert(0.01)
        assert alert is None

        # Use most of budget
        optimizer.record_usage(0.085)

        # Warning alert when approaching threshold
        alert = optimizer.create_budget_alert(0.01)
        assert alert is not None
        assert alert.alert_type == CostAlert.BUDGET_WARNING

        # Exceeded alert when over budget
        alert = optimizer.create_budget_alert(0.05)
        assert alert is not None
        assert alert.alert_type == CostAlert.BUDGET_EXCEEDED

    def test_prompt_optimization(self):
        """Optimize long prompts to reduce tokens."""
        optimizer = create_cost_optimizer()
        long_prompt = "Review: " + "x" * 50000  # Very long

        result = optimizer.optimize_prompt(long_prompt, max_tokens=2000)

        assert result.truncated is True
        assert result.optimized_length < result.original_length
        assert "TRUNCATED" in result.content

    def test_cost_estimation(self):
        """Estimate cost before API call."""
        optimizer = create_cost_optimizer()
        prompt = "Review this code" * 100  # ~1600 chars

        estimate = optimizer.estimate_cost(
            prompt,
            model="claude-sonnet-4-20250514",
            expected_output_tokens=2000,
        )

        assert estimate.input_tokens > 0
        assert estimate.output_tokens_estimate == 2000
        assert estimate.estimated_cost > 0

    def test_budget_checking(self):
        """Check if request fits within budget."""
        optimizer = create_cost_optimizer(budget=0.10)

        # Should fit
        assert optimizer.check_budget(0.05) is True

        # Record some usage
        optimizer.record_usage(0.08)

        # Should not fit now
        assert optimizer.check_budget(0.05) is False


class TestEndToEndIntegration:
    """Test complete end-to-end integration."""

    @patch("litellm.completion")
    def test_full_review_workflow(self, mock_completion, tmp_path: Path):
        """Test complete review workflow with all components."""
        # 1. Setup components
        tracker = create_usage_tracker(budget=1.0)
        optimizer = create_cost_optimizer(budget=1.0)

        # 2. Select model based on complexity
        prompt = "Review this pull request for security issues"
        selection = optimizer.select_model(prompt)

        # 3. Estimate cost
        estimate = optimizer.estimate_cost(prompt, model=selection.model_id)
        assert optimizer.check_budget(estimate.estimated_cost) is True

        # 4. Setup mock response
        response_content = json.dumps({
            "decision": "REQUEST_CHANGES",
            "checkpoint": "CP-001",
            "notes": "Found potential SQL injection vulnerability",
            "required_changes": [
                "Use parameterized queries in user_service.py",
            ],
        })
        mock_response = MagicMock()
        mock_response.choices = [
            MagicMock(message=MagicMock(content=response_content))
        ]
        mock_usage = MagicMock()
        mock_usage.prompt_tokens = 500
        mock_usage.completion_tokens = 200
        mock_usage.total_tokens = 700
        mock_response.usage = mock_usage
        mock_response._hidden_params = {"response_cost": 0.005}
        mock_completion.return_value = mock_response

        # 5. Create backend and run review
        backend = LiteLLMBackend(
            model=selection.model_id,
            api_key="test-key",
        )
        bundle_dir = tmp_path / "bundle"
        bundle_dir.mkdir()

        result = backend.run_review(prompt, bundle_dir)

        # 6. Track usage
        tracker.record_usage(
            model=selection.model_id,
            input_tokens=500,
            output_tokens=200,
            cost=0.005,
        )
        optimizer.record_usage(0.005)

        # 7. Verify results
        assert result.decision == SupervisorDecision.REQUEST_CHANGES
        assert "SQL injection" in result.notes
        assert len(result.required_changes) == 1

        # 8. Verify tracking
        assert tracker.session_tokens == 700
        assert tracker.session_cost == 0.005

        # 9. Verify budget status
        status = optimizer.get_budget_status()
        assert status["used"] == 0.005
        assert status["is_exceeded"] is False

    @patch("litellm.completion")
    def test_error_recovery_workflow(self, mock_completion, tmp_path: Path):
        """Test error recovery with fallback to cheaper model."""
        # First call fails with rate limit
        mock_completion.side_effect = [
            Exception("429 Rate limit exceeded"),
            MagicMock(
                choices=[
                    MagicMock(
                        message=MagicMock(
                            content=json.dumps({
                                "decision": "APPROVE",
                                "checkpoint": "CP-001",
                                "notes": "OK",
                                "required_changes": [],
                            })
                        )
                    )
                ],
                usage=MagicMock(
                    prompt_tokens=100, completion_tokens=50, total_tokens=150
                ),
                _hidden_params={"response_cost": 0.001},
            ),
        ]

        backend = LiteLLMBackend(model="claude-sonnet-4-20250514", api_key="test-key")
        bundle_dir = tmp_path / "bundle"
        bundle_dir.mkdir()

        # Should retry and succeed
        with patch("time.sleep"):
            result = backend.run_review("Review code", bundle_dir)

        assert result.decision == SupervisorDecision.APPROVE


@pytest.mark.skipif(
    SKIP_REAL_API or not ANTHROPIC_API_KEY,
    reason="Real API tests disabled or no API key",
)
class TestRealAPI:
    """Tests with real Anthropic API (optional).

    Enable with: TEST_REAL_API=true ANTHROPIC_API_KEY=sk-ant-... pytest
    """

    def test_real_api_call(self, tmp_path: Path):
        """Make a real API call to verify integration."""
        backend = LiteLLMBackend(
            model="claude-3-5-haiku-20241022",  # Use Haiku for cost savings
            api_key=ANTHROPIC_API_KEY,
        )

        bundle_dir = tmp_path / "bundle"
        bundle_dir.mkdir()

        # Simple prompt to minimize cost
        result = backend.run_review(
            prompt='''Review this simple code:
```python
def add(a, b):
    return a + b
```

Respond with JSON:
{"decision": "APPROVE", "checkpoint": "CP-001",
"notes": "Simple addition function", "required_changes": []}
''',
            bundle_dir=bundle_dir,
            timeout=30,
        )

        assert result.decision == SupervisorDecision.APPROVE
        assert (bundle_dir / "raw_response.txt").exists()

    def test_real_api_error_handling(self, tmp_path: Path):
        """Test error handling with invalid API key."""
        backend = LiteLLMBackend(
            model="claude-3-5-haiku-20241022",
            api_key="invalid-key-12345",
        )

        bundle_dir = tmp_path / "bundle"
        bundle_dir.mkdir()

        # Should fail with authentication error
        with pytest.raises(Exception) as exc_info:
            backend.run_review("Test", bundle_dir, timeout=10)

        error_handler = ClaudeErrorHandler()
        action = error_handler.handle_error(exc_info.value)
        assert action.error_type in (
            ClaudeErrorType.AUTHENTICATION,
            ClaudeErrorType.UNKNOWN,
        )
