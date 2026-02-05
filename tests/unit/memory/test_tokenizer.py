"""Tests for token counting utilities."""

from c4.memory.tokenizer import count_tokens, estimate_tokens, truncate_to_tokens


class TestCountTokens:
    """Tests for count_tokens function."""

    def test_count_tokens_empty_string(self) -> None:
        """Empty string should return 0 tokens."""
        assert count_tokens("") == 0

    def test_count_tokens_simple_text(self) -> None:
        """Simple text should return reasonable token count."""
        # "Hello, world!" is typically 4 tokens
        result = count_tokens("Hello, world!")
        assert result > 0
        assert result < 10  # Should be small number of tokens

    def test_count_tokens_longer_text(self) -> None:
        """Longer text should have more tokens."""
        short_text = "Hello"
        long_text = "Hello, this is a longer piece of text with many words."

        short_count = count_tokens(short_text)
        long_count = count_tokens(long_text)

        assert long_count > short_count

    def test_count_tokens_unicode(self) -> None:
        """Unicode text should be tokenizable."""
        # Korean text
        result = count_tokens("안녕하세요")
        assert result > 0

    def test_count_tokens_code(self) -> None:
        """Code snippets should be tokenizable."""
        code = "def hello():\n    return 'world'"
        result = count_tokens(code)
        assert result > 0


class TestEstimateTokens:
    """Tests for estimate_tokens function."""

    def test_estimate_tokens_empty_string(self) -> None:
        """Empty string should return 0 tokens."""
        assert estimate_tokens("") == 0

    def test_estimate_tokens_simple_text(self) -> None:
        """Simple text should give rough estimate."""
        # "Hello, world!" is 13 chars -> ~3 tokens
        result = estimate_tokens("Hello, world!")
        assert result > 0
        assert result < 10

    def test_estimate_tokens_long_text(self) -> None:
        """Long text estimation should scale with length."""
        text = "a" * 100  # 100 chars -> ~25 tokens
        result = estimate_tokens(text)
        assert 20 <= result <= 30

    def test_estimate_tokens_minimum_one(self) -> None:
        """Short non-empty text should return at least 1."""
        result = estimate_tokens("Hi")
        assert result >= 1


class TestTruncateToTokens:
    """Tests for truncate_to_tokens function."""

    def test_truncate_empty_string(self) -> None:
        """Empty string should return empty."""
        assert truncate_to_tokens("", 10) == ""

    def test_truncate_zero_tokens(self) -> None:
        """Zero max tokens should return empty."""
        assert truncate_to_tokens("Hello, world!", 0) == ""

    def test_truncate_within_limit(self) -> None:
        """Text within limit should be unchanged."""
        text = "Hello"
        result = truncate_to_tokens(text, 100)
        assert result == text

    def test_truncate_exceeds_limit(self) -> None:
        """Text exceeding limit should be truncated."""
        long_text = "This is a long sentence that should be truncated."
        result = truncate_to_tokens(long_text, 5)

        # Result should be shorter
        assert len(result) < len(long_text)
        # Result should still be valid text
        assert result.strip() != ""

    def test_truncate_negative_tokens(self) -> None:
        """Negative max tokens should return empty."""
        assert truncate_to_tokens("Hello", -1) == ""


class TestTokenizerConsistency:
    """Tests for consistency between count and estimate."""

    def test_estimate_is_reasonable(self) -> None:
        """Estimate should be within reasonable range of actual count."""
        text = "The quick brown fox jumps over the lazy dog."

        exact = count_tokens(text)
        estimate = estimate_tokens(text)

        # Estimate should be within 50% of actual
        assert estimate >= exact * 0.5
        assert estimate <= exact * 2.0

    def test_truncate_respects_limit(self) -> None:
        """Truncated text should respect token limit."""
        text = "This is a sentence that will be truncated to a specific number of tokens."
        max_tokens = 5

        result = truncate_to_tokens(text, max_tokens)
        actual_tokens = count_tokens(result)

        assert actual_tokens <= max_tokens
