"""Token counting utilities for memory management.

This module provides functions to count tokens in text content,
useful for estimating context window usage and managing memory budgets.

Uses tiktoken (OpenAI's tokenizer) for accurate counting.
"""

from functools import lru_cache

import tiktoken


@lru_cache(maxsize=4)
def _get_encoding(model: str = "gpt-4") -> tiktoken.Encoding:
    """Get cached tiktoken encoding for a model.

    Args:
        model: Model name for tokenization. Defaults to "gpt-4".

    Returns:
        tiktoken.Encoding instance for the specified model.
    """
    try:
        return tiktoken.encoding_for_model(model)
    except KeyError:
        # Fallback to cl100k_base for unknown models
        return tiktoken.get_encoding("cl100k_base")


def count_tokens(text: str, model: str = "gpt-4") -> int:
    """Count the exact number of tokens in text using tiktoken.

    Args:
        text: The text to tokenize.
        model: Model name for tokenization. Defaults to "gpt-4".
            Supports OpenAI models (gpt-4, gpt-3.5-turbo, etc.)

    Returns:
        Exact token count for the text.

    Example:
        >>> count_tokens("Hello, world!")
        4
    """
    if not text:
        return 0

    encoding = _get_encoding(model)
    return len(encoding.encode(text))


def estimate_tokens(text: str) -> int:
    """Fast token estimation without using tiktoken.

    Uses a simple heuristic: ~4 characters per token on average.
    This is useful for quick estimates when exact counts aren't needed.

    Args:
        text: The text to estimate tokens for.

    Returns:
        Estimated token count (tends to slightly overestimate).

    Example:
        >>> estimate_tokens("Hello, world!")  # 13 chars -> ~3-4 tokens
        3
    """
    if not text:
        return 0

    # Average English text is roughly 4 characters per token
    return max(1, len(text) // 4)


def truncate_to_tokens(text: str, max_tokens: int, model: str = "gpt-4") -> str:
    """Truncate text to fit within a token limit.

    Args:
        text: The text to truncate.
        max_tokens: Maximum number of tokens allowed.
        model: Model name for tokenization.

    Returns:
        Truncated text that fits within the token limit.

    Example:
        >>> truncate_to_tokens("Hello world! How are you?", 3)
        'Hello world!'
    """
    if not text or max_tokens <= 0:
        return ""

    encoding = _get_encoding(model)
    tokens = encoding.encode(text)

    if len(tokens) <= max_tokens:
        return text

    return encoding.decode(tokens[:max_tokens])
