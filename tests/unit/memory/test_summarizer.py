"""Tests for the AI summarizer module."""

import os
from unittest.mock import MagicMock, patch

from c4.memory.summarizer import (
    SUMMARIZE_THRESHOLD,
    TARGET_SUMMARY_TOKENS,
    AnthropicSummarizer,
    MemorySummarizer,
    MockSummarizer,
    OpenAISummarizer,
    SummarizerProvider,
    get_summarizer,
)


class TestMockSummarizer:
    """Tests for MockSummarizer."""

    def test_should_summarize_short_content(self) -> None:
        """Short content should not need summarization."""
        summarizer = MockSummarizer()
        short_content = "Hello, world!"
        assert summarizer.should_summarize(short_content) is False

    def test_should_summarize_long_content(self) -> None:
        """Long content should need summarization."""
        summarizer = MockSummarizer()
        # Create content that exceeds threshold
        long_content = "This is a test. " * 200  # ~800 tokens
        assert summarizer.should_summarize(long_content) is True

    def test_should_summarize_empty_content(self) -> None:
        """Empty content should not need summarization."""
        summarizer = MockSummarizer()
        assert summarizer.should_summarize("") is False
        assert summarizer.should_summarize(None) is False  # type: ignore

    def test_summarize_short_content_returns_unchanged(self) -> None:
        """Short content should be returned unchanged."""
        summarizer = MockSummarizer()
        short_content = "Hello, world!"
        result = summarizer.summarize(short_content)
        assert result == short_content

    def test_summarize_long_content_returns_truncated(self) -> None:
        """Long content should be truncated."""
        summarizer = MockSummarizer()
        long_content = "This is a test. " * 200
        result = summarizer.summarize(long_content)
        assert len(result) < len(long_content)
        assert "[Summary]:" in result

    def test_summarize_with_context(self) -> None:
        """Context should be included in summary prefix."""
        summarizer = MockSummarizer()
        long_content = "This is a test. " * 200
        result = summarizer.summarize(long_content, context="meeting notes")
        assert "[Summary of meeting notes]:" in result

    def test_summarize_empty_content(self) -> None:
        """Empty content should return empty string."""
        summarizer = MockSummarizer()
        assert summarizer.summarize("") == ""

    def test_custom_threshold(self) -> None:
        """Custom threshold should be respected."""
        summarizer = MockSummarizer(threshold=10)
        content = "Hello, this is a test."  # ~6 tokens
        assert summarizer.should_summarize(content) is False

        content_longer = "Hello, this is a test with more content here."  # ~11 tokens
        assert summarizer.should_summarize(content_longer) is True

    def test_custom_target_tokens(self) -> None:
        """Custom target tokens should affect summary length."""
        summarizer = MockSummarizer(target_tokens=20)
        long_content = "This is a test. " * 200
        result = summarizer.summarize(long_content)
        # Summary should be relatively short
        assert len(result) < 200


class TestAnthropicSummarizer:
    """Tests for AnthropicSummarizer."""

    def test_default_model(self) -> None:
        """Default model should be claude-3-haiku."""
        summarizer = AnthropicSummarizer()
        assert "haiku" in summarizer.model.lower()

    def test_custom_api_key(self) -> None:
        """Custom API key should be stored."""
        summarizer = AnthropicSummarizer(api_key="test-key")
        assert summarizer.api_key == "test-key"

    def test_api_key_from_env(self) -> None:
        """API key should be read from environment."""
        with patch.dict(os.environ, {"ANTHROPIC_API_KEY": "env-key"}):
            summarizer = AnthropicSummarizer()
            assert summarizer.api_key == "env-key"

    def test_summarize_calls_api(self) -> None:
        """summarize() should call Anthropic API."""
        summarizer = AnthropicSummarizer(api_key="test-key")
        long_content = "This is a test. " * 200

        # Mock the client
        mock_response = MagicMock()
        mock_response.content = [MagicMock(text="This is a summary.")]

        with patch.object(summarizer, "_get_client") as mock_get_client:
            mock_client = MagicMock()
            mock_client.messages.create.return_value = mock_response
            mock_get_client.return_value = mock_client

            result = summarizer.summarize(long_content, context="test")

            mock_client.messages.create.assert_called_once()
            assert result == "This is a summary."

    def test_summarize_with_model_override(self) -> None:
        """Model override should be used."""
        summarizer = AnthropicSummarizer(api_key="test-key")
        long_content = "This is a test. " * 200

        mock_response = MagicMock()
        mock_response.content = [MagicMock(text="Summary")]

        with patch.object(summarizer, "_get_client") as mock_get_client:
            mock_client = MagicMock()
            mock_client.messages.create.return_value = mock_response
            mock_get_client.return_value = mock_client

            summarizer.summarize(long_content, model="claude-3-opus-20240229")

            call_kwargs = mock_client.messages.create.call_args.kwargs
            assert call_kwargs["model"] == "claude-3-opus-20240229"

    def test_summarize_api_error_fallback(self) -> None:
        """API errors should fallback to truncation."""
        summarizer = AnthropicSummarizer(api_key="test-key")
        long_content = "This is a test. " * 200

        with patch.object(summarizer, "_get_client") as mock_get_client:
            mock_client = MagicMock()
            mock_client.messages.create.side_effect = Exception("API Error")
            mock_get_client.return_value = mock_client

            result = summarizer.summarize(long_content)

            # Should fallback to truncation
            assert len(result) < len(long_content)
            assert result.endswith("...")


class TestOpenAISummarizer:
    """Tests for OpenAISummarizer."""

    def test_default_model(self) -> None:
        """Default model should be gpt-3.5-turbo."""
        summarizer = OpenAISummarizer()
        assert summarizer.model == "gpt-3.5-turbo"

    def test_custom_api_key(self) -> None:
        """Custom API key should be stored."""
        summarizer = OpenAISummarizer(api_key="test-key")
        assert summarizer.api_key == "test-key"

    def test_api_key_from_env(self) -> None:
        """API key should be read from environment."""
        with patch.dict(os.environ, {"OPENAI_API_KEY": "env-key"}):
            summarizer = OpenAISummarizer()
            assert summarizer.api_key == "env-key"

    def test_summarize_calls_api(self) -> None:
        """summarize() should call OpenAI API."""
        summarizer = OpenAISummarizer(api_key="test-key")
        long_content = "This is a test. " * 200

        # Mock the client
        mock_response = MagicMock()
        mock_response.choices = [
            MagicMock(message=MagicMock(content="OpenAI summary"))
        ]

        with patch.object(summarizer, "_get_client") as mock_get_client:
            mock_client = MagicMock()
            mock_client.chat.completions.create.return_value = mock_response
            mock_get_client.return_value = mock_client

            result = summarizer.summarize(long_content)

            mock_client.chat.completions.create.assert_called_once()
            assert result == "OpenAI summary"

    def test_summarize_api_error_fallback(self) -> None:
        """API errors should fallback to truncation."""
        summarizer = OpenAISummarizer(api_key="test-key")
        long_content = "This is a test. " * 200

        with patch.object(summarizer, "_get_client") as mock_get_client:
            mock_client = MagicMock()
            mock_client.chat.completions.create.side_effect = Exception("API Error")
            mock_get_client.return_value = mock_client

            result = summarizer.summarize(long_content)

            # Should fallback to truncation
            assert len(result) < len(long_content)
            assert result.endswith("...")


class TestMemorySummarizer:
    """Tests for MemorySummarizer main class."""

    def test_auto_detect_anthropic(self) -> None:
        """Should use Anthropic when ANTHROPIC_API_KEY is set."""
        with patch.dict(
            os.environ,
            {"ANTHROPIC_API_KEY": "test", "OPENAI_API_KEY": ""},
            clear=False,
        ):
            # Need to clear OPENAI_API_KEY if it exists
            os.environ.pop("OPENAI_API_KEY", None)
            summarizer = MemorySummarizer()
            assert isinstance(summarizer.provider, AnthropicSummarizer)

    def test_auto_detect_openai(self) -> None:
        """Should use OpenAI when only OPENAI_API_KEY is set."""
        with patch.dict(os.environ, {"OPENAI_API_KEY": "test"}, clear=False):
            os.environ.pop("ANTHROPIC_API_KEY", None)
            summarizer = MemorySummarizer()
            assert isinstance(summarizer.provider, OpenAISummarizer)

    def test_auto_detect_mock_fallback(self) -> None:
        """Should fallback to mock when no API keys are set."""
        with patch.dict(os.environ, {}, clear=True):
            summarizer = MemorySummarizer()
            assert isinstance(summarizer.provider, MockSummarizer)

    def test_custom_provider(self) -> None:
        """Custom provider should be used."""
        mock_provider = MockSummarizer()
        summarizer = MemorySummarizer(provider=mock_provider)
        assert summarizer.provider is mock_provider

    def test_should_summarize_delegates(self) -> None:
        """should_summarize should delegate to provider."""
        mock_provider = MockSummarizer()
        summarizer = MemorySummarizer(provider=mock_provider)

        assert summarizer.should_summarize("short") is False
        # Need content that exceeds 500 tokens (~2000+ characters)
        long_content = "This is a test sentence with multiple words. " * 100
        assert summarizer.should_summarize(long_content) is True

    def test_summarize_delegates(self) -> None:
        """summarize should delegate to provider."""
        mock_provider = MockSummarizer()
        summarizer = MemorySummarizer(provider=mock_provider)

        # Need content that exceeds 500 tokens (~2000+ characters)
        long_content = "This is a test sentence with multiple words. " * 100
        result = summarizer.summarize(long_content, context="test")

        assert "[Summary of test]:" in result

    def test_custom_threshold_and_target(self) -> None:
        """Custom threshold and target should be passed to provider."""
        summarizer = MemorySummarizer(threshold=100, target_tokens=50)
        assert summarizer.threshold == 100
        assert summarizer.target_tokens == 50


class TestGetSummarizer:
    """Tests for get_summarizer factory function."""

    def test_get_mock_provider(self) -> None:
        """Should return MockSummarizer for 'mock'."""
        summarizer = get_summarizer("mock")
        assert isinstance(summarizer.provider, MockSummarizer)

    def test_get_anthropic_provider(self) -> None:
        """Should return AnthropicSummarizer for 'anthropic'."""
        summarizer = get_summarizer("anthropic")
        assert isinstance(summarizer.provider, AnthropicSummarizer)

    def test_get_openai_provider(self) -> None:
        """Should return OpenAISummarizer for 'openai'."""
        summarizer = get_summarizer("openai")
        assert isinstance(summarizer.provider, OpenAISummarizer)

    def test_auto_detect(self) -> None:
        """None should trigger auto-detection."""
        with patch.dict(os.environ, {}, clear=True):
            summarizer = get_summarizer()
            # Without API keys, should fallback to mock
            assert isinstance(summarizer.provider, MockSummarizer)

    def test_custom_threshold_kwarg(self) -> None:
        """Threshold kwarg should be passed through."""
        summarizer = get_summarizer("mock", threshold=100)
        assert summarizer.threshold == 100

    def test_custom_target_tokens_kwarg(self) -> None:
        """Target tokens kwarg should be passed through."""
        summarizer = get_summarizer("mock", target_tokens=50)
        assert summarizer.target_tokens == 50


class TestSummarizerProtocol:
    """Tests for SummarizerProvider protocol."""

    def test_mock_implements_protocol(self) -> None:
        """MockSummarizer should implement SummarizerProvider."""
        summarizer = MockSummarizer()
        assert isinstance(summarizer, SummarizerProvider)

    def test_anthropic_implements_protocol(self) -> None:
        """AnthropicSummarizer should implement SummarizerProvider."""
        summarizer = AnthropicSummarizer()
        assert isinstance(summarizer, SummarizerProvider)

    def test_openai_implements_protocol(self) -> None:
        """OpenAISummarizer should implement SummarizerProvider."""
        summarizer = OpenAISummarizer()
        assert isinstance(summarizer, SummarizerProvider)


class TestConstants:
    """Tests for module constants."""

    def test_summarize_threshold_value(self) -> None:
        """SUMMARIZE_THRESHOLD should be 500."""
        assert SUMMARIZE_THRESHOLD == 500

    def test_target_summary_tokens_value(self) -> None:
        """TARGET_SUMMARY_TOKENS should be 100."""
        assert TARGET_SUMMARY_TOKENS == 100
