"""C4 LLM Proxy - Managed proxy for LLM API requests."""

from __future__ import annotations

import asyncio
import logging
import time
import uuid
from typing import Any, AsyncGenerator

from fastapi import APIRouter, Depends, Header, HTTPException
from fastapi.responses import StreamingResponse
from pydantic import BaseModel, Field

from .metering import UsageMeter
from .rate_limit import RateLimiter, RateLimitStore

logger = logging.getLogger(__name__)

router = APIRouter()


# =============================================================================
# Models
# =============================================================================


class LLMMessage(BaseModel):
    """LLM message model."""

    role: str = Field(..., description="Message role (system, user, assistant)")
    content: str = Field(..., description="Message content")


class LLMRequest(BaseModel):
    """LLM completion request."""

    model: str = Field(..., description="Model name (e.g., gpt-4o, claude-3-sonnet)")
    messages: list[LLMMessage] = Field(..., description="Conversation messages")
    temperature: float = Field(default=0.7, ge=0, le=2)
    max_tokens: int = Field(default=4096, ge=1, le=128000)
    stream: bool = Field(default=False, description="Enable streaming response")
    user: str | None = Field(default=None, description="User identifier")


class LLMChoice(BaseModel):
    """LLM response choice."""

    index: int
    message: LLMMessage | None = None
    delta: dict[str, str] | None = None
    finish_reason: str | None = None


class LLMUsage(BaseModel):
    """Token usage info."""

    prompt_tokens: int
    completion_tokens: int
    total_tokens: int


class LLMResponse(BaseModel):
    """LLM completion response."""

    id: str = Field(default_factory=lambda: f"chatcmpl-{uuid.uuid4().hex[:12]}")
    object: str = "chat.completion"
    created: int = Field(default_factory=lambda: int(time.time()))
    model: str
    choices: list[LLMChoice]
    usage: LLMUsage | None = None


class LLMStreamChunk(BaseModel):
    """LLM streaming response chunk."""

    id: str
    object: str = "chat.completion.chunk"
    created: int
    model: str
    choices: list[LLMChoice]


# =============================================================================
# Service
# =============================================================================


class LLMProxyService:
    """Service for proxying LLM requests with metering and rate limiting."""

    def __init__(
        self,
        meter: UsageMeter | None = None,
        rate_store: RateLimitStore | None = None,
    ):
        """Initialize proxy service.

        Args:
            meter: Usage meter instance
            rate_store: Rate limit store
        """
        self.meter = meter or UsageMeter()
        self.rate_store = rate_store or RateLimitStore()
        self._litellm = None

    def _get_litellm(self):
        """Lazy load litellm."""
        if self._litellm is None:
            try:
                import litellm

                self._litellm = litellm
            except ImportError:
                raise HTTPException(
                    status_code=500,
                    detail="litellm package not installed. Run: uv add litellm",
                )
        return self._litellm

    async def check_rate_limit(
        self,
        user_id: str,
        estimated_tokens: int = 0,
    ) -> RateLimiter:
        """Check rate limits for user.

        Args:
            user_id: User identifier
            estimated_tokens: Estimated token usage

        Returns:
            Rate limiter instance

        Raises:
            HTTPException: If rate limit exceeded
        """
        limiter = await self.rate_store.get_limiter(user_id)

        # Check request limit
        allowed, reason = limiter.check_request_limit()
        if not allowed:
            raise HTTPException(
                status_code=429,
                detail={
                    "error": "rate_limit_exceeded",
                    "message": reason,
                    "limits": limiter.get_status(),
                },
            )

        # Check token limit if estimated
        if estimated_tokens > 0:
            allowed, reason = limiter.check_token_limit(estimated_tokens)
            if not allowed:
                raise HTTPException(
                    status_code=429,
                    detail={
                        "error": "rate_limit_exceeded",
                        "message": reason,
                        "limits": limiter.get_status(),
                    },
                )

        return limiter

    async def complete(
        self,
        request: LLMRequest,
        user_id: str | None = None,
        project_id: str | None = None,
    ) -> LLMResponse:
        """Make LLM completion request.

        Args:
            request: LLM request
            user_id: User identifier
            project_id: Project identifier

        Returns:
            LLM response
        """
        litellm = self._get_litellm()
        request_id = f"req-{uuid.uuid4().hex[:12]}"
        start_time = time.monotonic()

        # Check rate limit
        await self.check_rate_limit(user_id or "anonymous")

        try:
            # Build messages
            messages = [{"role": m.role, "content": m.content} for m in request.messages]

            # Call LLM
            response = await asyncio.to_thread(
                litellm.completion,
                model=request.model,
                messages=messages,
                temperature=request.temperature,
                max_tokens=request.max_tokens,
            )

            latency_ms = int((time.monotonic() - start_time) * 1000)

            # Record usage
            if response.usage:
                await self.meter.record_usage(
                    model=request.model,
                    prompt_tokens=response.usage.prompt_tokens,
                    completion_tokens=response.usage.completion_tokens,
                    request_id=request_id,
                    user_id=user_id,
                    project_id=project_id,
                    latency_ms=latency_ms,
                    success=True,
                )

            # Build response
            return LLMResponse(
                id=request_id,
                model=request.model,
                choices=[
                    LLMChoice(
                        index=i,
                        message=LLMMessage(
                            role=c.message.role,
                            content=c.message.content,
                        ),
                        finish_reason=c.finish_reason,
                    )
                    for i, c in enumerate(response.choices)
                ],
                usage=LLMUsage(
                    prompt_tokens=response.usage.prompt_tokens,
                    completion_tokens=response.usage.completion_tokens,
                    total_tokens=response.usage.total_tokens,
                )
                if response.usage
                else None,
            )

        except Exception as e:
            latency_ms = int((time.monotonic() - start_time) * 1000)

            # Record failed request
            await self.meter.record_usage(
                model=request.model,
                prompt_tokens=0,
                completion_tokens=0,
                request_id=request_id,
                user_id=user_id,
                project_id=project_id,
                latency_ms=latency_ms,
                success=False,
                error=str(e),
            )

            raise HTTPException(
                status_code=500,
                detail={"error": "llm_error", "message": str(e)},
            )

    async def stream_complete(
        self,
        request: LLMRequest,
        user_id: str | None = None,
        project_id: str | None = None,
    ) -> AsyncGenerator[str, None]:
        """Stream LLM completion response.

        Args:
            request: LLM request
            user_id: User identifier
            project_id: Project identifier

        Yields:
            SSE formatted response chunks
        """
        litellm = self._get_litellm()
        request_id = f"req-{uuid.uuid4().hex[:12]}"
        start_time = time.monotonic()
        created = int(time.time())

        # Check rate limit
        await self.check_rate_limit(user_id or "anonymous")

        prompt_tokens = 0
        completion_tokens = 0
        full_content = ""

        try:
            # Build messages
            messages = [{"role": m.role, "content": m.content} for m in request.messages]

            # Stream from LLM
            response = litellm.completion(
                model=request.model,
                messages=messages,
                temperature=request.temperature,
                max_tokens=request.max_tokens,
                stream=True,
            )

            for chunk in response:
                if chunk.choices and chunk.choices[0].delta:
                    delta = chunk.choices[0].delta
                    content = getattr(delta, "content", "") or ""

                    if content:
                        full_content += content
                        completion_tokens += 1  # Approximate

                        stream_chunk = LLMStreamChunk(
                            id=request_id,
                            created=created,
                            model=request.model,
                            choices=[
                                LLMChoice(
                                    index=0,
                                    delta={"content": content},
                                    finish_reason=None,
                                )
                            ],
                        )
                        yield f"data: {stream_chunk.model_dump_json()}\n\n"

                # Check for finish
                if chunk.choices and chunk.choices[0].finish_reason:
                    final_chunk = LLMStreamChunk(
                        id=request_id,
                        created=created,
                        model=request.model,
                        choices=[
                            LLMChoice(
                                index=0,
                                delta={},
                                finish_reason=chunk.choices[0].finish_reason,
                            )
                        ],
                    )
                    yield f"data: {final_chunk.model_dump_json()}\n\n"

            yield "data: [DONE]\n\n"

            # Record usage (approximate tokens)
            prompt_tokens = sum(len(m.content) // 4 for m in request.messages)
            latency_ms = int((time.monotonic() - start_time) * 1000)

            await self.meter.record_usage(
                model=request.model,
                prompt_tokens=prompt_tokens,
                completion_tokens=completion_tokens,
                request_id=request_id,
                user_id=user_id,
                project_id=project_id,
                latency_ms=latency_ms,
                success=True,
            )

        except Exception as e:
            latency_ms = int((time.monotonic() - start_time) * 1000)

            await self.meter.record_usage(
                model=request.model,
                prompt_tokens=0,
                completion_tokens=0,
                request_id=request_id,
                user_id=user_id,
                project_id=project_id,
                latency_ms=latency_ms,
                success=False,
                error=str(e),
            )

            error_chunk = {"error": {"message": str(e), "type": "api_error"}}
            yield f"data: {error_chunk}\n\n"


# =============================================================================
# Global Service
# =============================================================================


_proxy_service: LLMProxyService | None = None


def get_proxy_service() -> LLMProxyService:
    """Get or create proxy service instance."""
    global _proxy_service
    if _proxy_service is None:
        _proxy_service = LLMProxyService()
    return _proxy_service


# =============================================================================
# Routes
# =============================================================================


@router.post("/completions", response_model=None)
async def create_completion(
    request: LLMRequest,
    x_user_id: str | None = Header(None, alias="X-User-ID"),
    x_project_id: str | None = Header(None, alias="X-Project-ID"),
    proxy: LLMProxyService = Depends(get_proxy_service),
) -> LLMResponse | StreamingResponse:
    """Create LLM completion.

    Supports both streaming and non-streaming responses.
    Compatible with OpenAI API format.

    Args:
        request: LLM completion request
        x_user_id: User identifier header
        x_project_id: Project identifier header
        proxy: Injected proxy service

    Returns:
        LLM response or streaming response
    """
    user_id = x_user_id or request.user

    if request.stream:
        return StreamingResponse(
            proxy.stream_complete(request, user_id, x_project_id),
            media_type="text/event-stream",
            headers={
                "Cache-Control": "no-cache",
                "Connection": "keep-alive",
            },
        )
    else:
        return await proxy.complete(request, user_id, x_project_id)


@router.get("/usage")
async def get_usage(
    x_user_id: str | None = Header(None, alias="X-User-ID"),
    x_project_id: str | None = Header(None, alias="X-Project-ID"),
    proxy: LLMProxyService = Depends(get_proxy_service),
) -> dict[str, Any]:
    """Get usage summary.

    Args:
        x_user_id: Filter by user ID
        x_project_id: Filter by project ID
        proxy: Injected proxy service

    Returns:
        Usage summary
    """
    summary = proxy.meter.get_summary(
        user_id=x_user_id,
        project_id=x_project_id,
    )
    return summary.to_dict()


@router.get("/usage/recent")
async def get_recent_usage(
    limit: int = 100,
    proxy: LLMProxyService = Depends(get_proxy_service),
) -> list[dict[str, Any]]:
    """Get recent usage records.

    Args:
        limit: Maximum records to return
        proxy: Injected proxy service

    Returns:
        List of recent usage records
    """
    records = proxy.meter.get_recent_records(limit)
    return [r.to_dict() for r in records]


@router.get("/limits")
async def get_rate_limits(
    x_user_id: str | None = Header(None, alias="X-User-ID"),
    proxy: LLMProxyService = Depends(get_proxy_service),
) -> dict[str, Any]:
    """Get current rate limit status.

    Args:
        x_user_id: User identifier
        proxy: Injected proxy service

    Returns:
        Rate limit status
    """
    user_id = x_user_id or "anonymous"
    limiter = await proxy.rate_store.get_limiter(user_id)
    return {
        "user_id": user_id,
        "limits": limiter.get_status(),
        "config": {
            "requests_per_minute": proxy.rate_store.config.requests_per_minute,
            "requests_per_hour": proxy.rate_store.config.requests_per_hour,
            "tokens_per_minute": proxy.rate_store.config.tokens_per_minute,
            "tokens_per_hour": proxy.rate_store.config.tokens_per_hour,
        },
    }


@router.get("/models")
async def list_models() -> dict[str, Any]:
    """List available models.

    Returns:
        List of available models with pricing info
    """
    from .metering import MODEL_COSTS

    models = []
    for name, costs in MODEL_COSTS.items():
        models.append(
            {
                "id": name,
                "input_cost_per_1k": costs["input"],
                "output_cost_per_1k": costs["output"],
            }
        )

    return {
        "models": models,
        "note": "Costs are approximate. Use litellm for full model list.",
    }
