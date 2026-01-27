"""Provider strategies for LiteLLM backend.

Defines model-specific parameters and handling logic to abstract
differences between providers (Claude, Gemini, OpenAI).
"""

from abc import ABC, abstractmethod
from typing import Any, Dict, Optional


class ProviderStrategy(ABC):
    """Abstract base class for LLM provider strategies."""

    @abstractmethod
    def get_request_params(
        self,
        model: str,
        system_message: str,
        user_message: str,
        temperature: float,
        max_tokens: int,
        timeout: int,
        drop_params: bool,
        api_key: Optional[str] = None,
        api_base: Optional[str] = None,
    ) -> Dict[str, Any]:
        """Get parameters for litellm.completion."""
        pass

    @abstractmethod
    def parse_response(self, response: Any) -> str:
        """Parse text content from response."""
        pass

class ClaudeStrategy(ProviderStrategy):
    """Strategy for Anthropic Claude models."""

    def get_request_params(
        self,
        model: str,
        system_message: str,
        user_message: str,
        temperature: float,
        max_tokens: int,
        timeout: int,
        drop_params: bool,
        api_key: Optional[str] = None,
        api_base: Optional[str] = None,
    ) -> Dict[str, Any]:
        kwargs = {
            "model": model,
            "messages": [
                {"role": "system", "content": system_message},
                {"role": "user", "content": user_message},
            ],
            "temperature": temperature,
            "max_tokens": max_tokens,
            "timeout": timeout,
            "drop_params": drop_params,
        }
        if api_key:
            kwargs["api_key"] = api_key
        if api_base:
            kwargs["api_base"] = api_base
        return kwargs

    def parse_response(self, response: Any) -> str:
        return response.choices[0].message.content or ""

class GeminiStrategy(ProviderStrategy):
    """Strategy for Google Gemini models."""

    def get_request_params(
        self,
        model: str,
        system_message: str,
        user_message: str,
        temperature: float,
        max_tokens: int,
        timeout: int,
        drop_params: bool,
        api_key: Optional[str] = None,
        api_base: Optional[str] = None,
    ) -> Dict[str, Any]:
        kwargs = {
            "model": model,
            "messages": [
                # LiteLLM maps 'system' role to appropriate field for Gemini
                {"role": "system", "content": system_message},
                {"role": "user", "content": user_message},
            ],
            "temperature": temperature,
            "max_tokens": max_tokens,
            "timeout": timeout,
            "drop_params": drop_params,
            # Gemini-specific: JSON mode and Safety Settings
            "response_format": {"type": "json_object"},
            "safety_settings": [
                {
                    "category": "HARM_CATEGORY_HARASSMENT",
                    "threshold": "BLOCK_NONE",
                },
                {
                    "category": "HARM_CATEGORY_HATE_SPEECH",
                    "threshold": "BLOCK_NONE",
                },
                {
                    "category": "HARM_CATEGORY_SEXUALLY_EXPLICIT",
                    "threshold": "BLOCK_NONE",
                },
                {
                    "category": "HARM_CATEGORY_DANGEROUS_CONTENT",
                    "threshold": "BLOCK_NONE",
                },
            ]
        }
        if api_key:
            kwargs["api_key"] = api_key
        if api_base:
            kwargs["api_base"] = api_base
        return kwargs

    def parse_response(self, response: Any) -> str:
        return response.choices[0].message.content or ""

class OpenAIStrategy(ProviderStrategy):
    """Strategy for OpenAI models."""

    def get_request_params(
        self,
        model: str,
        system_message: str,
        user_message: str,
        temperature: float,
        max_tokens: int,
        timeout: int,
        drop_params: bool,
        api_key: Optional[str] = None,
        api_base: Optional[str] = None,
    ) -> Dict[str, Any]:
        kwargs = {
            "model": model,
            "messages": [
                {"role": "system", "content": system_message},
                {"role": "user", "content": user_message},
            ],
            "temperature": temperature,
            "max_tokens": max_tokens,
            "timeout": timeout,
            "drop_params": drop_params,
            "response_format": {"type": "json_object"},
        }
        if api_key:
            kwargs["api_key"] = api_key
        if api_base:
            kwargs["api_base"] = api_base
        return kwargs

    def parse_response(self, response: Any) -> str:
        return response.choices[0].message.content or ""

def get_strategy_for_model(model: str) -> ProviderStrategy:
    """Factory to get strategy based on model name."""
    model_lower = model.lower()
    if "gemini" in model_lower:
        return GeminiStrategy()
    elif "gpt" in model_lower or "openai" in model_lower or "o1-" in model_lower:
        return OpenAIStrategy()
    else:
        # Default to Claude/Generic
        return ClaudeStrategy()
