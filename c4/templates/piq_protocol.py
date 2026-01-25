"""PIQ-C4 Communication Protocol.

Defines the protocol for communication between C4 Template Library and PIQ Knowledge Hub.
PIQ provides ML/DL knowledge (papers, experiments, best practices) that C4 uses to
enhance template suggestions and experiment guidance.
"""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import Any
from urllib.parse import urljoin

import httpx
from pydantic import BaseModel, Field

logger = logging.getLogger(__name__)


# =============================================================================
# Configuration
# =============================================================================


class PIQConfig(BaseModel):
    """PIQ connection configuration.

    Attributes:
        base_url: PIQ Hub API base URL
        api_key: API key for authentication (optional)
        timeout: Request timeout in seconds
        enabled: Whether PIQ integration is enabled
        cache_ttl: Cache TTL in seconds (0 = no cache)
    """

    base_url: str = Field(
        default="http://localhost:8000",
        description="PIQ Hub API base URL",
    )
    api_key: str | None = Field(
        default=None,
        description="API key for authentication",
    )
    timeout: int = Field(
        default=30,
        description="Request timeout in seconds",
    )
    enabled: bool = Field(
        default=True,
        description="Whether PIQ integration is enabled",
    )
    cache_ttl: int = Field(
        default=3600,
        description="Cache TTL in seconds",
    )


# =============================================================================
# Knowledge Types
# =============================================================================


class KnowledgeType(str, Enum):
    """Types of knowledge from PIQ."""

    # Model architectures
    ARCHITECTURE = "architecture"  # CNN, Transformer, etc.
    BACKBONE = "backbone"  # ResNet, EfficientNet, ViT, etc.

    # Training configurations
    OPTIMIZER = "optimizer"  # Adam, SGD, AdamW, etc.
    SCHEDULER = "scheduler"  # CosineAnnealing, OneCycle, etc.
    AUGMENTATION = "augmentation"  # Data augmentation strategies
    REGULARIZATION = "regularization"  # Dropout, weight decay, etc.

    # Hyperparameters
    LEARNING_RATE = "learning_rate"  # LR recommendations
    BATCH_SIZE = "batch_size"  # Batch size recommendations
    HYPERPARAMETER = "hyperparameter"  # General hyperparameters

    # Loss functions
    LOSS_FUNCTION = "loss_function"  # CrossEntropy, Focal, etc.

    # Evaluation
    METRIC = "metric"  # Accuracy, mAP, BLEU, etc.
    BENCHMARK = "benchmark"  # Dataset benchmarks

    # Papers and research
    PAPER = "paper"  # Research papers
    TECHNIQUE = "technique"  # Novel techniques
    BEST_PRACTICE = "best_practice"  # Best practices


class KnowledgeSource(str, Enum):
    """Source of knowledge."""

    PAPER = "paper"  # From research papers
    SEED = "seed"  # From PIQ Seeds
    EXPERIMENT = "experiment"  # From PIQ experiments
    COMMUNITY = "community"  # From community contributions
    CURATED = "curated"  # Curated by PIQ team


# =============================================================================
# Knowledge Models
# =============================================================================


@dataclass
class PIQKnowledge:
    """Knowledge item from PIQ.

    Represents a piece of ML/DL knowledge such as a technique,
    hyperparameter recommendation, or best practice.
    """

    id: str
    knowledge_type: KnowledgeType
    title: str
    content: str
    source: KnowledgeSource
    confidence: float = 1.0  # 0.0 - 1.0

    # Optional metadata
    paper_id: str | None = None
    paper_title: str | None = None
    paper_url: str | None = None
    experiment_id: str | None = None
    tags: list[str] = field(default_factory=list)
    related_ids: list[str] = field(default_factory=list)

    # Context
    applicable_to: list[str] = field(default_factory=list)  # Template IDs
    constraints: dict[str, Any] = field(default_factory=dict)

    # Timestamps
    created_at: datetime | None = None
    updated_at: datetime | None = None


@dataclass
class PIQKnowledgeQuery:
    """Query for PIQ knowledge.

    Attributes:
        knowledge_type: Type of knowledge to query
        context: Additional context for the query
        template_id: Template ID for context
        task_type: Task type (classification, detection, etc.)
        dataset: Dataset name/type
        limit: Maximum results to return
        min_confidence: Minimum confidence threshold
    """

    knowledge_type: KnowledgeType | None = None
    context: dict[str, Any] = field(default_factory=dict)
    template_id: str | None = None
    task_type: str | None = None
    dataset: str | None = None
    limit: int = 10
    min_confidence: float = 0.0


@dataclass
class PIQKnowledgeResult:
    """Result of a PIQ knowledge query.

    Attributes:
        success: Whether query was successful
        knowledge: List of knowledge items
        suggestions: Extracted suggestions (values)
        total_count: Total matching knowledge count
        query_time_ms: Query time in milliseconds
        error: Error message if failed
    """

    success: bool
    knowledge: list[PIQKnowledge] = field(default_factory=list)
    suggestions: list[Any] = field(default_factory=list)
    total_count: int = 0
    query_time_ms: int = 0
    error: str | None = None


# =============================================================================
# PIQ Client
# =============================================================================


class PIQClient:
    """Client for PIQ Knowledge Hub API.

    Handles communication with PIQ to fetch ML/DL knowledge
    for template suggestions and experiment guidance.
    """

    def __init__(self, config: PIQConfig | None = None) -> None:
        """Initialize PIQ client.

        Args:
            config: PIQ configuration. If None, uses defaults.
        """
        self.config = config or PIQConfig()
        self._client: httpx.AsyncClient | None = None
        self._cache: dict[str, tuple[datetime, PIQKnowledgeResult]] = {}

    async def __aenter__(self) -> "PIQClient":
        """Async context manager entry."""
        await self.connect()
        return self

    async def __aexit__(self, *args: Any) -> None:
        """Async context manager exit."""
        await self.close()

    async def connect(self) -> None:
        """Initialize HTTP client."""
        headers = {"Content-Type": "application/json"}
        if self.config.api_key:
            headers["Authorization"] = f"Bearer {self.config.api_key}"

        self._client = httpx.AsyncClient(
            base_url=self.config.base_url,
            headers=headers,
            timeout=self.config.timeout,
        )

    async def close(self) -> None:
        """Close HTTP client."""
        if self._client:
            await self._client.aclose()
            self._client = None

    @property
    def is_connected(self) -> bool:
        """Check if client is connected."""
        return self._client is not None

    # =========================================================================
    # Health Check
    # =========================================================================

    async def health_check(self) -> bool:
        """Check if PIQ Hub is available.

        Returns:
            True if PIQ Hub is healthy
        """
        if not self.config.enabled:
            return False

        if not self._client:
            await self.connect()

        try:
            response = await self._client.get("/health")
            return response.status_code == 200
        except Exception as e:
            logger.warning(f"PIQ health check failed: {e}")
            return False

    # =========================================================================
    # Knowledge Queries
    # =========================================================================

    async def query(
        self,
        knowledge_type: KnowledgeType | str | None = None,
        context: dict[str, Any] | None = None,
        template_id: str | None = None,
        task_type: str | None = None,
        dataset: str | None = None,
        limit: int = 10,
        min_confidence: float = 0.0,
    ) -> PIQKnowledgeResult:
        """Query PIQ for knowledge.

        Args:
            knowledge_type: Type of knowledge to query
            context: Additional context
            template_id: Template ID for filtering
            task_type: Task type for filtering
            dataset: Dataset for filtering
            limit: Maximum results
            min_confidence: Minimum confidence threshold

        Returns:
            PIQKnowledgeResult with matching knowledge
        """
        if not self.config.enabled:
            return PIQKnowledgeResult(
                success=False,
                error="PIQ integration is disabled",
            )

        if not self._client:
            await self.connect()

        # Build query
        query = PIQKnowledgeQuery(
            knowledge_type=(
                KnowledgeType(knowledge_type)
                if isinstance(knowledge_type, str)
                else knowledge_type
            ),
            context=context or {},
            template_id=template_id,
            task_type=task_type,
            dataset=dataset,
            limit=limit,
            min_confidence=min_confidence,
        )

        # Check cache
        cache_key = self._make_cache_key(query)
        if cache_key in self._cache:
            cached_time, cached_result = self._cache[cache_key]
            if (datetime.now() - cached_time).seconds < self.config.cache_ttl:
                return cached_result

        # Make request
        try:
            response = await self._client.post(
                "/api/knowledge/query",
                json={
                    "knowledge_type": query.knowledge_type.value
                    if query.knowledge_type
                    else None,
                    "context": query.context,
                    "template_id": query.template_id,
                    "task_type": query.task_type,
                    "dataset": query.dataset,
                    "limit": query.limit,
                    "min_confidence": query.min_confidence,
                },
            )

            if response.status_code == 200:
                data = response.json()
                result = self._parse_query_result(data)
                self._cache[cache_key] = (datetime.now(), result)
                return result
            else:
                return PIQKnowledgeResult(
                    success=False,
                    error=f"PIQ query failed: {response.status_code}",
                )

        except httpx.ConnectError:
            logger.warning("Cannot connect to PIQ Hub")
            return PIQKnowledgeResult(
                success=False,
                error="Cannot connect to PIQ Hub",
            )
        except Exception as e:
            logger.error(f"PIQ query error: {e}")
            return PIQKnowledgeResult(
                success=False,
                error=str(e),
            )

    async def get_knowledge(self, knowledge_id: str) -> PIQKnowledge | None:
        """Get a specific knowledge item by ID.

        Args:
            knowledge_id: Knowledge item ID

        Returns:
            PIQKnowledge or None if not found
        """
        if not self.config.enabled or not self._client:
            return None

        try:
            response = await self._client.get(f"/api/knowledge/{knowledge_id}")
            if response.status_code == 200:
                return self._parse_knowledge(response.json())
            return None
        except Exception as e:
            logger.error(f"Failed to get knowledge {knowledge_id}: {e}")
            return None

    async def get_suggestions(
        self,
        template_id: str,
        param_name: str,
        current_params: dict[str, Any] | None = None,
    ) -> list[Any]:
        """Get parameter suggestions for a template.

        Args:
            template_id: Template ID
            param_name: Parameter name to get suggestions for
            current_params: Current parameter values for context

        Returns:
            List of suggested values
        """
        result = await self.query(
            context={
                "template_id": template_id,
                "param_name": param_name,
                "current_params": current_params or {},
            },
            limit=5,
        )

        return result.suggestions if result.success else []

    async def get_best_practices(
        self,
        task_type: str,
        dataset: str | None = None,
    ) -> list[PIQKnowledge]:
        """Get best practices for a task type.

        Args:
            task_type: Task type (classification, detection, etc.)
            dataset: Optional dataset name

        Returns:
            List of best practice knowledge items
        """
        result = await self.query(
            knowledge_type=KnowledgeType.BEST_PRACTICE,
            task_type=task_type,
            dataset=dataset,
            limit=20,
        )

        return result.knowledge if result.success else []

    async def get_related_papers(
        self,
        template_id: str | None = None,
        technique: str | None = None,
    ) -> list[PIQKnowledge]:
        """Get related research papers.

        Args:
            template_id: Template ID for context
            technique: Technique name to search for

        Returns:
            List of paper knowledge items
        """
        result = await self.query(
            knowledge_type=KnowledgeType.PAPER,
            template_id=template_id,
            context={"technique": technique} if technique else {},
            limit=10,
        )

        return result.knowledge if result.success else []

    # =========================================================================
    # Seeds Integration
    # =========================================================================

    async def generate_from_seed(
        self,
        seed_type: str,
        topic: str,
        options: dict[str, Any] | None = None,
    ) -> PIQKnowledgeResult:
        """Generate knowledge from PIQ Seeds.

        This triggers PIQ's knowledge generation pipeline:
        1. Paper fetching (if paper-based)
        2. LLM extraction
        3. Relation generation

        Args:
            seed_type: Type of seed (paper, direct, relation)
            topic: Topic to generate knowledge about
            options: Additional generation options

        Returns:
            Generated knowledge result
        """
        if not self.config.enabled or not self._client:
            return PIQKnowledgeResult(
                success=False,
                error="PIQ not available",
            )

        try:
            response = await self._client.post(
                "/api/seeds/generate",
                json={
                    "seed_type": seed_type,
                    "topic": topic,
                    "options": options or {},
                },
                timeout=120,  # Generation can take longer
            )

            if response.status_code == 200:
                return self._parse_query_result(response.json())
            else:
                return PIQKnowledgeResult(
                    success=False,
                    error=f"Seed generation failed: {response.status_code}",
                )
        except Exception as e:
            logger.error(f"Seed generation error: {e}")
            return PIQKnowledgeResult(
                success=False,
                error=str(e),
            )

    # =========================================================================
    # Internal Methods
    # =========================================================================

    def _make_cache_key(self, query: PIQKnowledgeQuery) -> str:
        """Generate cache key for query."""
        import hashlib
        import json

        query_str = json.dumps(
            {
                "type": query.knowledge_type.value if query.knowledge_type else None,
                "context": query.context,
                "template_id": query.template_id,
                "task_type": query.task_type,
                "dataset": query.dataset,
                "limit": query.limit,
                "min_confidence": query.min_confidence,
            },
            sort_keys=True,
        )
        return hashlib.sha256(query_str.encode()).hexdigest()[:16]

    def _parse_query_result(self, data: dict[str, Any]) -> PIQKnowledgeResult:
        """Parse query result from API response."""
        knowledge = [
            self._parse_knowledge(k) for k in data.get("knowledge", [])
        ]

        return PIQKnowledgeResult(
            success=True,
            knowledge=knowledge,
            suggestions=data.get("suggestions", []),
            total_count=data.get("total_count", len(knowledge)),
            query_time_ms=data.get("query_time_ms", 0),
        )

    def _parse_knowledge(self, data: dict[str, Any]) -> PIQKnowledge:
        """Parse knowledge item from API response."""
        return PIQKnowledge(
            id=data.get("id", ""),
            knowledge_type=KnowledgeType(data.get("knowledge_type", "technique")),
            title=data.get("title", ""),
            content=data.get("content", ""),
            source=KnowledgeSource(data.get("source", "curated")),
            confidence=data.get("confidence", 1.0),
            paper_id=data.get("paper_id"),
            paper_title=data.get("paper_title"),
            paper_url=data.get("paper_url"),
            experiment_id=data.get("experiment_id"),
            tags=data.get("tags", []),
            related_ids=data.get("related_ids", []),
            applicable_to=data.get("applicable_to", []),
            constraints=data.get("constraints", {}),
            created_at=datetime.fromisoformat(data["created_at"])
            if data.get("created_at")
            else None,
            updated_at=datetime.fromisoformat(data["updated_at"])
            if data.get("updated_at")
            else None,
        )


# =============================================================================
# Singleton Instance
# =============================================================================


_piq_client: PIQClient | None = None


def get_piq_client(config: PIQConfig | None = None) -> PIQClient:
    """Get or create PIQ client singleton.

    Args:
        config: Optional configuration. Only used on first call.

    Returns:
        PIQClient instance
    """
    global _piq_client
    if _piq_client is None:
        _piq_client = PIQClient(config)
    return _piq_client
