"""AI-powered content summarization for memory management.

This module provides summarization capabilities to compress long content
into shorter summaries, helping manage context window limits.

Uses Claude (Anthropic) or OpenAI APIs for high-quality summarization.

Usage:
    from c4.memory.summarizer import MemorySummarizer, get_summarizer

    summarizer = get_summarizer()

    if summarizer.should_summarize(long_content):
        summary = summarizer.summarize(long_content, context="project memory")
"""

import logging
import os
from abc import ABC, abstractmethod
from typing import Protocol, runtime_checkable

from .tokenizer import count_tokens

logger = logging.getLogger(__name__)


# Thresholds for summarization
SUMMARIZE_THRESHOLD = 500  # Tokens above which to summarize
TARGET_SUMMARY_TOKENS = 100  # Target summary length in tokens


@runtime_checkable
class SummarizerProvider(Protocol):
    """Protocol for summarization providers."""

    @abstractmethod
    def summarize(
        self, content: str, context: str | None = None, model: str | None = None
    ) -> str:
        """Generate a summary of the content.

        Args:
            content: The content to summarize.
            context: Optional context about what the content is (e.g., "meeting notes").
            model: Optional model override.

        Returns:
            A concise summary of the content.
        """
        ...

    @abstractmethod
    def should_summarize(self, content: str) -> bool:
        """Check if content should be summarized.

        Args:
            content: The content to check.

        Returns:
            True if content exceeds the summarization threshold.
        """
        ...


class BaseSummarizer(ABC):
    """Base class for summarizers with common functionality."""

    def __init__(
        self,
        threshold: int = SUMMARIZE_THRESHOLD,
        target_tokens: int = TARGET_SUMMARY_TOKENS,
    ) -> None:
        """Initialize the summarizer.

        Args:
            threshold: Token count above which to summarize.
            target_tokens: Target length for summaries.
        """
        self.threshold = threshold
        self.target_tokens = target_tokens

    def should_summarize(self, content: str) -> bool:
        """Check if content should be summarized based on token count.

        Args:
            content: The content to check.

        Returns:
            True if content exceeds the threshold.
        """
        if not content:
            return False
        return count_tokens(content) > self.threshold

    @abstractmethod
    def summarize(
        self, content: str, context: str | None = None, model: str | None = None
    ) -> str:
        """Generate a summary. Must be implemented by subclasses."""
        ...


class AnthropicSummarizer(BaseSummarizer):
    """Summarizer using Claude (Anthropic API).

    Attributes:
        api_key: Anthropic API key. Defaults to ANTHROPIC_API_KEY env var.
        model: Default model to use. Defaults to claude-3-haiku-20240307.
    """

    DEFAULT_MODEL = "claude-3-haiku-20240307"

    def __init__(
        self,
        api_key: str | None = None,
        model: str | None = None,
        threshold: int = SUMMARIZE_THRESHOLD,
        target_tokens: int = TARGET_SUMMARY_TOKENS,
    ) -> None:
        """Initialize the Anthropic summarizer.

        Args:
            api_key: Anthropic API key. Uses env var if not provided.
            model: Model to use for summarization.
            threshold: Token count above which to summarize.
            target_tokens: Target length for summaries.
        """
        super().__init__(threshold, target_tokens)
        self.api_key = api_key or os.environ.get("ANTHROPIC_API_KEY")
        self.model = model or self.DEFAULT_MODEL
        self._client = None

    def _get_client(self):
        """Get or create Anthropic client."""
        if self._client is None:
            try:
                import anthropic

                self._client = anthropic.Anthropic(api_key=self.api_key)
            except ImportError as e:
                raise ImportError(
                    "anthropic package required for AnthropicSummarizer. "
                    "Install with: pip install anthropic"
                ) from e
        return self._client

    def summarize(
        self, content: str, context: str | None = None, model: str | None = None
    ) -> str:
        """Summarize content using Claude.

        Args:
            content: The content to summarize.
            context: Optional context about the content type.
            model: Optional model override.

        Returns:
            A concise summary of the content.
        """
        if not content:
            return ""

        # Don't summarize if under threshold
        if not self.should_summarize(content):
            return content

        use_model = model or self.model
        client = self._get_client()

        # Build the prompt
        context_str = f" ({context})" if context else ""
        prompt = f"""Summarize the following content{context_str} in approximately {self.target_tokens} tokens.
Focus on the key information and main points. Be concise but comprehensive.

Content to summarize:
{content}

Summary:"""

        try:
            response = client.messages.create(
                model=use_model,
                max_tokens=self.target_tokens * 2,  # Allow some buffer
                messages=[{"role": "user", "content": prompt}],
            )
            return response.content[0].text.strip()
        except Exception as e:
            logger.error(f"Anthropic summarization failed: {e}")
            # Fallback to truncation
            return _truncate_summary(content, self.target_tokens)


class OpenAISummarizer(BaseSummarizer):
    """Summarizer using OpenAI API.

    Attributes:
        api_key: OpenAI API key. Defaults to OPENAI_API_KEY env var.
        model: Default model to use. Defaults to gpt-3.5-turbo.
    """

    DEFAULT_MODEL = "gpt-3.5-turbo"

    def __init__(
        self,
        api_key: str | None = None,
        model: str | None = None,
        threshold: int = SUMMARIZE_THRESHOLD,
        target_tokens: int = TARGET_SUMMARY_TOKENS,
    ) -> None:
        """Initialize the OpenAI summarizer.

        Args:
            api_key: OpenAI API key. Uses env var if not provided.
            model: Model to use for summarization.
            threshold: Token count above which to summarize.
            target_tokens: Target length for summaries.
        """
        super().__init__(threshold, target_tokens)
        self.api_key = api_key or os.environ.get("OPENAI_API_KEY")
        self.model = model or self.DEFAULT_MODEL
        self._client = None

    def _get_client(self):
        """Get or create OpenAI client."""
        if self._client is None:
            try:
                import openai

                self._client = openai.OpenAI(api_key=self.api_key)
            except ImportError as e:
                raise ImportError(
                    "openai package required for OpenAISummarizer. "
                    "Install with: pip install openai"
                ) from e
        return self._client

    def summarize(
        self, content: str, context: str | None = None, model: str | None = None
    ) -> str:
        """Summarize content using OpenAI.

        Args:
            content: The content to summarize.
            context: Optional context about the content type.
            model: Optional model override.

        Returns:
            A concise summary of the content.
        """
        if not content:
            return ""

        # Don't summarize if under threshold
        if not self.should_summarize(content):
            return content

        use_model = model or self.model
        client = self._get_client()

        # Build the prompt
        context_str = f" ({context})" if context else ""
        system_prompt = (
            f"You are a precise summarizer. Summarize content{context_str} "
            f"in approximately {self.target_tokens} tokens. "
            "Focus on key information and main points. Be concise but comprehensive."
        )

        try:
            response = client.chat.completions.create(
                model=use_model,
                max_tokens=self.target_tokens * 2,
                messages=[
                    {"role": "system", "content": system_prompt},
                    {"role": "user", "content": content},
                ],
            )
            return response.choices[0].message.content.strip()
        except Exception as e:
            logger.error(f"OpenAI summarization failed: {e}")
            # Fallback to truncation
            return _truncate_summary(content, self.target_tokens)


class MockSummarizer(BaseSummarizer):
    """Mock summarizer for testing.

    Generates deterministic summaries without API calls.
    """

    def summarize(
        self, content: str, context: str | None = None, model: str | None = None
    ) -> str:
        """Generate a mock summary by truncating content.

        Args:
            content: The content to summarize.
            context: Optional context (used in prefix).
            model: Ignored in mock.

        Returns:
            Truncated content with optional context prefix.
        """
        if not content:
            return ""

        # Don't summarize if under threshold
        if not self.should_summarize(content):
            return content

        # Create a simple mock summary
        prefix = f"[Summary of {context}]: " if context else "[Summary]: "
        return prefix + _truncate_summary(content, self.target_tokens - 10)


class MemorySummarizer:
    """Main summarizer class that delegates to a provider.

    This is the recommended way to use summarization in the memory system.

    Example:
        >>> summarizer = MemorySummarizer()
        >>> if summarizer.should_summarize(long_text):
        ...     summary = summarizer.summarize(long_text, "meeting notes")
    """

    def __init__(
        self,
        provider: BaseSummarizer | None = None,
        threshold: int = SUMMARIZE_THRESHOLD,
        target_tokens: int = TARGET_SUMMARY_TOKENS,
    ) -> None:
        """Initialize the memory summarizer.

        Args:
            provider: Summarization provider. Auto-detected if not provided.
            threshold: Token count above which to summarize.
            target_tokens: Target length for summaries.
        """
        self.threshold = threshold
        self.target_tokens = target_tokens

        if provider is not None:
            self._provider = provider
        else:
            self._provider = self._auto_detect_provider()

    def _auto_detect_provider(self) -> BaseSummarizer:
        """Auto-detect the best available summarization provider.

        Priority:
        1. Anthropic (if ANTHROPIC_API_KEY set)
        2. OpenAI (if OPENAI_API_KEY set)
        3. Mock (fallback)
        """
        if os.environ.get("ANTHROPIC_API_KEY"):
            return AnthropicSummarizer(
                threshold=self.threshold, target_tokens=self.target_tokens
            )
        elif os.environ.get("OPENAI_API_KEY"):
            return OpenAISummarizer(
                threshold=self.threshold, target_tokens=self.target_tokens
            )
        else:
            logger.warning(
                "No API key found for summarization. Using mock summarizer. "
                "Set ANTHROPIC_API_KEY or OPENAI_API_KEY for AI-powered summaries."
            )
            return MockSummarizer(
                threshold=self.threshold, target_tokens=self.target_tokens
            )

    @property
    def provider(self) -> BaseSummarizer:
        """Get the current summarization provider."""
        return self._provider

    def should_summarize(self, content: str) -> bool:
        """Check if content should be summarized.

        Args:
            content: The content to check.

        Returns:
            True if content exceeds the summarization threshold.
        """
        return self._provider.should_summarize(content)

    def summarize(
        self, content: str, context: str | None = None, model: str | None = None
    ) -> str:
        """Generate a summary of the content.

        Args:
            content: The content to summarize.
            context: Optional context about what the content is.
            model: Optional model override.

        Returns:
            A concise summary of the content.
        """
        return self._provider.summarize(content, context, model)


def _truncate_summary(content: str, max_tokens: int) -> str:
    """Truncate content to approximately max_tokens.

    This is used as a fallback when API summarization fails.

    Args:
        content: Content to truncate.
        max_tokens: Target token count.

    Returns:
        Truncated content with ellipsis if needed.
    """
    from .tokenizer import truncate_to_tokens

    truncated = truncate_to_tokens(content, max_tokens)
    if len(truncated) < len(content):
        # Remove partial word at end and add ellipsis
        if truncated and not truncated[-1].isspace():
            last_space = truncated.rfind(" ")
            if last_space > 0:
                truncated = truncated[:last_space]
        truncated = truncated.rstrip() + "..."
    return truncated


def get_summarizer(
    provider: str | None = None, **kwargs
) -> MemorySummarizer:
    """Factory function to create a MemorySummarizer.

    Args:
        provider: Provider name ("anthropic", "openai", "mock") or None for auto.
        **kwargs: Additional arguments passed to MemorySummarizer.

    Returns:
        MemorySummarizer instance with the specified provider.

    Example:
        >>> summarizer = get_summarizer("mock")  # For testing
        >>> summarizer = get_summarizer()  # Auto-detect
    """
    threshold = kwargs.pop("threshold", SUMMARIZE_THRESHOLD)
    target_tokens = kwargs.pop("target_tokens", TARGET_SUMMARY_TOKENS)

    if provider == "anthropic":
        return MemorySummarizer(
            provider=AnthropicSummarizer(
                threshold=threshold, target_tokens=target_tokens, **kwargs
            ),
            threshold=threshold,
            target_tokens=target_tokens,
        )
    elif provider == "openai":
        return MemorySummarizer(
            provider=OpenAISummarizer(
                threshold=threshold, target_tokens=target_tokens, **kwargs
            ),
            threshold=threshold,
            target_tokens=target_tokens,
        )
    elif provider == "mock":
        return MemorySummarizer(
            provider=MockSummarizer(threshold=threshold, target_tokens=target_tokens),
            threshold=threshold,
            target_tokens=target_tokens,
        )
    else:
        # Auto-detect
        return MemorySummarizer(threshold=threshold, target_tokens=target_tokens)
