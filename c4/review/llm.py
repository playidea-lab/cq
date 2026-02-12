"""
LLM client abstraction layer.

Supports Anthropic (Claude) and OpenAI (GPT) backends with a unified interface.
"""

from __future__ import annotations

import os
from dataclasses import dataclass
from typing import Protocol, runtime_checkable


@dataclass
class LLMResponse:
    """Unified response from any LLM provider."""

    text: str


@runtime_checkable
class LLMClient(Protocol):
    """Protocol for LLM clients."""

    def chat(self, messages: list[dict], max_tokens: int = 4096) -> LLMResponse: ...

    def format_image(self, image_base64: str, media_type: str = "image/png") -> dict: ...

    def format_text(self, text: str) -> dict: ...


class AnthropicLLM:
    """Anthropic Claude backend."""

    def __init__(self, api_key: str | None = None, model: str = "claude-opus-4"):
        from anthropic import Anthropic

        self.client = Anthropic(api_key=api_key)
        self.model = model

    def chat(self, messages: list[dict], max_tokens: int = 4096) -> LLMResponse:
        response = self.client.messages.create(
            model=self.model, max_tokens=max_tokens, messages=messages
        )
        return LLMResponse(text=response.content[0].text)

    def format_image(self, image_base64: str, media_type: str = "image/png") -> dict:
        return {
            "type": "image",
            "source": {"type": "base64", "media_type": media_type, "data": image_base64},
        }

    def format_text(self, text: str) -> dict:
        return {"type": "text", "text": text}


class OpenAILLM:
    """OpenAI GPT backend."""

    def __init__(self, api_key: str | None = None, model: str = "gpt-4o"):
        from openai import OpenAI

        self.client = OpenAI(api_key=api_key)
        self.model = model

    def chat(self, messages: list[dict], max_tokens: int = 4096) -> LLMResponse:
        response = self.client.chat.completions.create(
            model=self.model, max_tokens=max_tokens, messages=messages
        )
        return LLMResponse(text=response.choices[0].message.content)

    def format_image(self, image_base64: str, media_type: str = "image/png") -> dict:
        return {
            "type": "image_url",
            "image_url": {"url": f"data:{media_type};base64,{image_base64}"},
        }

    def format_text(self, text: str) -> dict:
        return {"type": "text", "text": text}


def create_llm(
    provider: str | None = None,
    api_key: str | None = None,
    model: str | None = None,
) -> LLMClient:
    """Factory: create an LLM client from env or explicit args.

    Auto-detects provider from available env vars if *provider* is None:
      - ANTHROPIC_API_KEY → AnthropicLLM
      - OPENAI_API_KEY   → OpenAILLM

    Args:
        provider: "anthropic" or "openai". Auto-detect if None.
        api_key: Explicit API key. Falls back to env var.
        model: Model name override.

    Returns:
        LLMClient instance.
    """
    if provider is None:
        if api_key or os.getenv("ANTHROPIC_API_KEY"):
            provider = "anthropic"
        elif os.getenv("OPENAI_API_KEY"):
            provider = "openai"
        else:
            provider = "anthropic"  # default

    if provider == "openai":
        return OpenAILLM(api_key=api_key, model=model or "gpt-4o")
    return AnthropicLLM(api_key=api_key, model=model or "claude-opus-4")
