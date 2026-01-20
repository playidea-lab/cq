"""Tests for Claude CLI backend, especially sensitive data sanitization."""

from c4.supervisor.claude_backend import (
    MAX_ERROR_MESSAGE_LENGTH,
    _sanitize_stderr,
)


class TestSanitizeStderr:
    """Test _sanitize_stderr function."""

    def test_empty_input(self):
        """Empty input returns empty string."""
        assert _sanitize_stderr("") == ""
        assert _sanitize_stderr(None) == ""

    def test_no_sensitive_data(self):
        """Normal error messages pass through unchanged."""
        error = "Error: Connection refused"
        assert _sanitize_stderr(error) == error

    def test_mask_anthropic_api_key(self):
        """Anthropic API keys are masked."""
        error = "Error: Invalid API key sk-ant-api03-abc123def456ghi789jkl012mno345pqr678stu901vwx234yz5678"
        result = _sanitize_stderr(error)
        assert "sk-ant" not in result
        assert "[MASKED" in result

    def test_mask_openai_style_key(self):
        """OpenAI-style API keys are masked."""
        error = "Error: sk-abcdef1234567890abcdef1234567890"
        result = _sanitize_stderr(error)
        assert "sk-abcdef" not in result
        assert "[MASKED" in result

    def test_mask_env_var_key(self):
        """Environment variable style API keys are masked."""
        error = "ANTHROPIC_API_KEY=sk-ant-secret123 not valid"
        result = _sanitize_stderr(error)
        assert "sk-ant-secret123" not in result
        assert "[MASKED]" in result

    def test_mask_bearer_token(self):
        """Bearer tokens are masked."""
        error = "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.abcdef"
        result = _sanitize_stderr(error)
        assert "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9" not in result
        assert "[MASKED_TOKEN]" in result

    def test_mask_generic_api_key(self):
        """Generic api_key patterns are masked."""
        error = "Config error: api_key='my_secret_api_key_12345'"
        result = _sanitize_stderr(error)
        assert "my_secret_api_key_12345" not in result
        assert "[MASKED]" in result

    def test_truncate_long_message(self):
        """Long messages are truncated."""
        long_error = "x" * (MAX_ERROR_MESSAGE_LENGTH + 100)
        result = _sanitize_stderr(long_error)
        assert len(result) <= MAX_ERROR_MESSAGE_LENGTH + 20  # Allow for truncation marker
        assert result.endswith("... [truncated]")

    def test_case_insensitive_masking(self):
        """Masking is case insensitive."""
        error = "API_KEY=secret123456789012"
        result = _sanitize_stderr(error)
        assert "secret" not in result

    def test_multiple_patterns_masked(self):
        """Multiple sensitive patterns are all masked."""
        error = (
            "Error: sk-abc12345678901234567890 failed, "
            "bearer token123.abc456.xyz789 expired, api_key=secret123456789"
        )
        result = _sanitize_stderr(error)
        assert "sk-abc" not in result
        # The bearer pattern should match
        assert "token123" not in result
        # api_key pattern requires 10+ chars
        assert "secret123456789" not in result.lower()

    def test_preserves_useful_error_info(self):
        """Useful error information is preserved."""
        error = "Error: Network timeout after 30s - please check connectivity"
        result = _sanitize_stderr(error)
        assert "Network timeout" in result
        assert "30s" in result
        assert "connectivity" in result
