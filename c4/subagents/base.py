"""Base class for C4 subagents."""

from abc import ABC, abstractmethod
from typing import Any


class SubagentBase(ABC):
    """Base class for specialized lightweight agents.

    Subagents are designed to:
    - Run with smaller models (Haiku) for cost efficiency
    - Handle focused, single-purpose tasks
    - Return compressed context (max 2000 tokens)
    - Minimize token overhead

    Subclasses must implement:
    - execute(): Main execution logic
    - validate_input(): Input validation
    """

    def __init__(self, model: str = "haiku"):
        """Initialize subagent.

        Args:
            model: Model to use (default: haiku)
        """
        self.model = model
        self.max_tokens = 2000

    @abstractmethod
    def execute(self, **kwargs: Any) -> dict[str, Any]:
        """Execute the subagent task.

        Args:
            **kwargs: Task-specific parameters

        Returns:
            Result dictionary with compressed context
        """
        pass

    @abstractmethod
    def validate_input(self, **kwargs: Any) -> tuple[bool, str | None]:
        """Validate input parameters.

        Args:
            **kwargs: Parameters to validate

        Returns:
            Tuple of (is_valid, error_message)
        """
        pass

    def estimate_tokens(self, text: str) -> int:
        """Estimate token count for text.

        Rough approximation: 1 token ≈ 4 characters

        Args:
            text: Text to estimate

        Returns:
            Estimated token count
        """
        return len(text) // 4

    def truncate_if_needed(
        self, context: dict[str, Any], current_tokens: int
    ) -> dict[str, Any]:
        """Add truncation metadata if context exceeds max tokens.

        Args:
            context: Context dictionary
            current_tokens: Current token count

        Returns:
            Updated context with truncation metadata
        """
        if current_tokens > self.max_tokens:
            context["truncated"] = True
            context["message"] = f"Context truncated at {self.max_tokens} tokens"
            context["token_count"] = current_tokens

        return context
