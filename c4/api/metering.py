"""C4 Usage Metering - Track API usage, tokens, and costs."""

from __future__ import annotations

import asyncio
import json
import logging
from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)


class ModelProvider(str, Enum):
    """LLM model providers."""

    OPENAI = "openai"
    ANTHROPIC = "anthropic"
    AZURE = "azure"
    OLLAMA = "ollama"
    BEDROCK = "bedrock"
    OTHER = "other"


@dataclass
class UsageRecord:
    """Single API usage record."""

    timestamp: datetime
    model: str
    provider: ModelProvider
    prompt_tokens: int
    completion_tokens: int
    total_tokens: int
    cost: float | None = None
    request_id: str | None = None
    user_id: str | None = None
    project_id: str | None = None
    latency_ms: int | None = None
    success: bool = True
    error: str | None = None
    metadata: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "timestamp": self.timestamp.isoformat(),
            "model": self.model,
            "provider": self.provider.value,
            "prompt_tokens": self.prompt_tokens,
            "completion_tokens": self.completion_tokens,
            "total_tokens": self.total_tokens,
            "cost": self.cost,
            "request_id": self.request_id,
            "user_id": self.user_id,
            "project_id": self.project_id,
            "latency_ms": self.latency_ms,
            "success": self.success,
            "error": self.error,
            "metadata": self.metadata,
        }


@dataclass
class UsageSummary:
    """Aggregated usage summary."""

    period_start: datetime
    period_end: datetime
    total_requests: int = 0
    successful_requests: int = 0
    failed_requests: int = 0
    total_prompt_tokens: int = 0
    total_completion_tokens: int = 0
    total_tokens: int = 0
    total_cost: float = 0.0
    avg_latency_ms: float = 0.0
    by_model: dict[str, dict[str, Any]] = field(default_factory=dict)
    by_user: dict[str, dict[str, Any]] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "period_start": self.period_start.isoformat(),
            "period_end": self.period_end.isoformat(),
            "total_requests": self.total_requests,
            "successful_requests": self.successful_requests,
            "failed_requests": self.failed_requests,
            "total_prompt_tokens": self.total_prompt_tokens,
            "total_completion_tokens": self.total_completion_tokens,
            "total_tokens": self.total_tokens,
            "total_cost": self.total_cost,
            "avg_latency_ms": self.avg_latency_ms,
            "by_model": self.by_model,
            "by_user": self.by_user,
        }


# Cost per 1K tokens (approximate, varies by provider)
MODEL_COSTS: dict[str, dict[str, float]] = {
    "gpt-4o": {"input": 0.005, "output": 0.015},
    "gpt-4o-mini": {"input": 0.00015, "output": 0.0006},
    "gpt-4-turbo": {"input": 0.01, "output": 0.03},
    "gpt-4": {"input": 0.03, "output": 0.06},
    "gpt-3.5-turbo": {"input": 0.0005, "output": 0.0015},
    "claude-3-opus": {"input": 0.015, "output": 0.075},
    "claude-3-sonnet": {"input": 0.003, "output": 0.015},
    "claude-3-haiku": {"input": 0.00025, "output": 0.00125},
    "claude-3-5-sonnet": {"input": 0.003, "output": 0.015},
}


def estimate_cost(
    model: str,
    prompt_tokens: int,
    completion_tokens: int,
) -> float | None:
    """Estimate cost for API call.

    Args:
        model: Model name
        prompt_tokens: Number of input tokens
        completion_tokens: Number of output tokens

    Returns:
        Estimated cost in USD, or None if unknown model
    """
    # Normalize model name
    model_lower = model.lower()
    for name, costs in MODEL_COSTS.items():
        if name in model_lower:
            input_cost = (prompt_tokens / 1000) * costs["input"]
            output_cost = (completion_tokens / 1000) * costs["output"]
            return round(input_cost + output_cost, 6)
    return None


class UsageMeter:
    """Track and aggregate API usage."""

    def __init__(
        self,
        storage_path: Path | None = None,
        max_records: int = 10000,
    ):
        """Initialize usage meter.

        Args:
            storage_path: Path to persist usage data
            max_records: Maximum records to keep in memory
        """
        self.storage_path = storage_path
        self.max_records = max_records
        self._records: list[UsageRecord] = []
        self._lock = asyncio.Lock()

        # Load existing records
        if storage_path and storage_path.exists():
            self._load_records()

    def _load_records(self) -> None:
        """Load records from storage."""
        if not self.storage_path:
            return

        try:
            with open(self.storage_path) as f:
                data = json.load(f)

            for item in data[-self.max_records:]:
                record = UsageRecord(
                    timestamp=datetime.fromisoformat(item["timestamp"]),
                    model=item["model"],
                    provider=ModelProvider(item["provider"]),
                    prompt_tokens=item["prompt_tokens"],
                    completion_tokens=item["completion_tokens"],
                    total_tokens=item["total_tokens"],
                    cost=item.get("cost"),
                    request_id=item.get("request_id"),
                    user_id=item.get("user_id"),
                    project_id=item.get("project_id"),
                    latency_ms=item.get("latency_ms"),
                    success=item.get("success", True),
                    error=item.get("error"),
                    metadata=item.get("metadata", {}),
                )
                self._records.append(record)

            logger.info(f"Loaded {len(self._records)} usage records")
        except Exception as e:
            logger.warning(f"Failed to load usage records: {e}")

    def _save_records(self) -> None:
        """Save records to storage."""
        if not self.storage_path:
            return

        try:
            self.storage_path.parent.mkdir(parents=True, exist_ok=True)
            with open(self.storage_path, "w") as f:
                json.dump([r.to_dict() for r in self._records], f, indent=2)
        except Exception as e:
            logger.warning(f"Failed to save usage records: {e}")

    async def record_usage(
        self,
        model: str,
        prompt_tokens: int,
        completion_tokens: int,
        request_id: str | None = None,
        user_id: str | None = None,
        project_id: str | None = None,
        latency_ms: int | None = None,
        success: bool = True,
        error: str | None = None,
        metadata: dict[str, Any] | None = None,
    ) -> UsageRecord:
        """Record API usage.

        Args:
            model: Model name
            prompt_tokens: Number of input tokens
            completion_tokens: Number of output tokens
            request_id: Optional request ID
            user_id: Optional user ID
            project_id: Optional project ID
            latency_ms: Response latency in milliseconds
            success: Whether request succeeded
            error: Error message if failed
            metadata: Additional metadata

        Returns:
            Created usage record
        """
        # Detect provider from model name
        provider = self._detect_provider(model)

        # Estimate cost
        cost = estimate_cost(model, prompt_tokens, completion_tokens)

        record = UsageRecord(
            timestamp=datetime.now(),
            model=model,
            provider=provider,
            prompt_tokens=prompt_tokens,
            completion_tokens=completion_tokens,
            total_tokens=prompt_tokens + completion_tokens,
            cost=cost,
            request_id=request_id,
            user_id=user_id,
            project_id=project_id,
            latency_ms=latency_ms,
            success=success,
            error=error,
            metadata=metadata or {},
        )

        async with self._lock:
            self._records.append(record)

            # Trim if over limit
            if len(self._records) > self.max_records:
                self._records = self._records[-self.max_records:]

            self._save_records()

        logger.debug(
            f"Recorded usage: {model}, {prompt_tokens + completion_tokens} tokens"
            f"{f', ${cost:.4f}' if cost else ''}"
        )

        return record

    def _detect_provider(self, model: str) -> ModelProvider:
        """Detect provider from model name."""
        model_lower = model.lower()

        # Check prefix-based providers first
        if model_lower.startswith("azure/"):
            return ModelProvider.AZURE
        elif model_lower.startswith("ollama/"):
            return ModelProvider.OLLAMA
        elif model_lower.startswith("bedrock/"):
            return ModelProvider.BEDROCK
        # Then check substring-based providers
        elif "gpt" in model_lower or "o1" in model_lower:
            return ModelProvider.OPENAI
        elif "claude" in model_lower:
            return ModelProvider.ANTHROPIC
        else:
            return ModelProvider.OTHER

    def get_summary(
        self,
        start: datetime | None = None,
        end: datetime | None = None,
        user_id: str | None = None,
        project_id: str | None = None,
    ) -> UsageSummary:
        """Get usage summary for a period.

        Args:
            start: Period start (default: all time)
            end: Period end (default: now)
            user_id: Filter by user ID
            project_id: Filter by project ID

        Returns:
            Usage summary
        """
        end = end or datetime.now()
        start = start or (self._records[0].timestamp if self._records else end)

        # Filter records
        filtered = [
            r
            for r in self._records
            if start <= r.timestamp <= end
            and (user_id is None or r.user_id == user_id)
            and (project_id is None or r.project_id == project_id)
        ]

        summary = UsageSummary(
            period_start=start,
            period_end=end,
        )

        if not filtered:
            return summary

        # Aggregate
        total_latency = 0
        latency_count = 0

        for record in filtered:
            summary.total_requests += 1
            if record.success:
                summary.successful_requests += 1
            else:
                summary.failed_requests += 1

            summary.total_prompt_tokens += record.prompt_tokens
            summary.total_completion_tokens += record.completion_tokens
            summary.total_tokens += record.total_tokens

            if record.cost:
                summary.total_cost += record.cost

            if record.latency_ms:
                total_latency += record.latency_ms
                latency_count += 1

            # By model
            if record.model not in summary.by_model:
                summary.by_model[record.model] = {
                    "requests": 0,
                    "tokens": 0,
                    "cost": 0.0,
                }
            summary.by_model[record.model]["requests"] += 1
            summary.by_model[record.model]["tokens"] += record.total_tokens
            if record.cost:
                summary.by_model[record.model]["cost"] += record.cost

            # By user
            user_key = record.user_id or "anonymous"
            if user_key not in summary.by_user:
                summary.by_user[user_key] = {
                    "requests": 0,
                    "tokens": 0,
                    "cost": 0.0,
                }
            summary.by_user[user_key]["requests"] += 1
            summary.by_user[user_key]["tokens"] += record.total_tokens
            if record.cost:
                summary.by_user[user_key]["cost"] += record.cost

        if latency_count > 0:
            summary.avg_latency_ms = total_latency / latency_count

        return summary

    def get_recent_records(self, limit: int = 100) -> list[UsageRecord]:
        """Get recent usage records.

        Args:
            limit: Maximum records to return

        Returns:
            List of recent records (newest first)
        """
        return list(reversed(self._records[-limit:]))

    def clear(self) -> int:
        """Clear all records.

        Returns:
            Number of records cleared
        """
        count = len(self._records)
        self._records.clear()
        self._save_records()
        return count
