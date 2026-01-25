"""Tests for PIQ (Paper-Intelligence-Quotient) Protocol."""

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from c4.templates.piq_protocol import (
    KnowledgeSource,
    KnowledgeType,
    PIQClient,
    PIQConfig,
    PIQKnowledge,
    PIQKnowledgeQuery,
    PIQKnowledgeResult,
    get_piq_client,
)


class TestKnowledgeType:
    """Tests for KnowledgeType enum."""

    def test_knowledge_type_values(self):
        """Test that all expected knowledge types exist."""
        # Model architectures
        assert KnowledgeType.ARCHITECTURE == "architecture"
        assert KnowledgeType.BACKBONE == "backbone"

        # Training configurations
        assert KnowledgeType.OPTIMIZER == "optimizer"
        assert KnowledgeType.SCHEDULER == "scheduler"
        assert KnowledgeType.AUGMENTATION == "augmentation"
        assert KnowledgeType.REGULARIZATION == "regularization"

        # Hyperparameters
        assert KnowledgeType.LEARNING_RATE == "learning_rate"
        assert KnowledgeType.BATCH_SIZE == "batch_size"
        assert KnowledgeType.HYPERPARAMETER == "hyperparameter"

        # Loss functions
        assert KnowledgeType.LOSS_FUNCTION == "loss_function"

        # Evaluation
        assert KnowledgeType.METRIC == "metric"
        assert KnowledgeType.BENCHMARK == "benchmark"

        # Papers and research
        assert KnowledgeType.PAPER == "paper"
        assert KnowledgeType.TECHNIQUE == "technique"
        assert KnowledgeType.BEST_PRACTICE == "best_practice"


class TestKnowledgeSource:
    """Tests for KnowledgeSource enum."""

    def test_knowledge_source_values(self):
        """Test that all expected sources exist."""
        assert KnowledgeSource.PAPER == "paper"
        assert KnowledgeSource.SEED == "seed"
        assert KnowledgeSource.EXPERIMENT == "experiment"
        assert KnowledgeSource.COMMUNITY == "community"
        assert KnowledgeSource.CURATED == "curated"


class TestPIQConfig:
    """Tests for PIQConfig dataclass."""

    def test_default_config(self):
        """Test default configuration."""
        config = PIQConfig()
        assert config.base_url == "http://localhost:8000"
        assert config.timeout == 30
        assert config.enabled is True
        assert config.cache_ttl == 3600
        assert config.api_key is None

    def test_custom_config(self):
        """Test custom configuration."""
        config = PIQConfig(
            base_url="https://piq.example.com",
            api_key="test-key",
            timeout=60,
            enabled=True,
            cache_ttl=7200,
        )
        assert config.base_url == "https://piq.example.com"
        assert config.api_key == "test-key"
        assert config.timeout == 60
        assert config.cache_ttl == 7200

    def test_disabled_config(self):
        """Test disabled configuration."""
        config = PIQConfig(enabled=False)
        assert config.enabled is False


class TestPIQKnowledge:
    """Tests for PIQKnowledge dataclass."""

    def test_create_knowledge(self):
        """Test creating a knowledge entry."""
        knowledge = PIQKnowledge(
            id="k-001",
            knowledge_type=KnowledgeType.ARCHITECTURE,
            title="ResNet",
            content="Deep Residual Learning for Image Recognition",
            source=KnowledgeSource.PAPER,
            confidence=0.95,
            paper_id="arxiv-1512.03385",
            paper_title="Deep Residual Learning",
            paper_url="https://arxiv.org/abs/1512.03385",
            tags=["cnn", "vision", "classification"],
        )
        assert knowledge.id == "k-001"
        assert knowledge.knowledge_type == KnowledgeType.ARCHITECTURE
        assert knowledge.title == "ResNet"
        assert knowledge.content == "Deep Residual Learning for Image Recognition"
        assert knowledge.confidence == 0.95
        assert "cnn" in knowledge.tags

    def test_knowledge_defaults(self):
        """Test knowledge entry with defaults."""
        knowledge = PIQKnowledge(
            id="k-002",
            knowledge_type=KnowledgeType.HYPERPARAMETER,
            title="Learning Rate",
            content="Optimal learning rate for Adam",
            source=KnowledgeSource.EXPERIMENT,
        )
        assert knowledge.paper_id is None
        assert knowledge.paper_title is None
        assert knowledge.paper_url is None
        assert knowledge.experiment_id is None
        assert knowledge.confidence == 1.0
        assert knowledge.tags == []
        assert knowledge.related_ids == []
        assert knowledge.applicable_to == []
        assert knowledge.constraints == {}


class TestPIQKnowledgeQuery:
    """Tests for PIQKnowledgeQuery dataclass."""

    def test_create_query(self):
        """Test creating a knowledge query."""
        query = PIQKnowledgeQuery(
            knowledge_type=KnowledgeType.ARCHITECTURE,
            context={"task": "image-classification", "dataset": "imagenet"},
            template_id="image-classification",
            task_type="classification",
            limit=10,
        )
        assert query.knowledge_type == KnowledgeType.ARCHITECTURE
        assert query.context["task"] == "image-classification"
        assert query.template_id == "image-classification"
        assert query.limit == 10

    def test_query_defaults(self):
        """Test query with defaults."""
        query = PIQKnowledgeQuery(knowledge_type=KnowledgeType.OPTIMIZER)
        assert query.context == {}
        assert query.template_id is None
        assert query.task_type is None
        assert query.dataset is None
        assert query.limit == 10
        assert query.min_confidence == 0.0

    def test_query_none_knowledge_type(self):
        """Test query without knowledge type."""
        query = PIQKnowledgeQuery()
        assert query.knowledge_type is None


class TestPIQKnowledgeResult:
    """Tests for PIQKnowledgeResult dataclass."""

    def test_create_result(self):
        """Test creating a knowledge result."""
        knowledge_items = [
            PIQKnowledge(
                id="k-001",
                knowledge_type=KnowledgeType.OPTIMIZER,
                title="AdamW",
                content="Adam with weight decay fix",
                source=KnowledgeSource.PAPER,
            )
        ]
        result = PIQKnowledgeResult(
            success=True,
            knowledge=knowledge_items,
            suggestions=["AdamW", "Adam", "SGD"],
            total_count=1,
            query_time_ms=50,
        )
        assert result.success is True
        assert len(result.knowledge) == 1
        assert result.total_count == 1
        assert "AdamW" in result.suggestions
        assert result.query_time_ms == 50
        assert result.error is None

    def test_empty_result(self):
        """Test empty result."""
        result = PIQKnowledgeResult(success=True)
        assert result.knowledge == []
        assert result.suggestions == []
        assert result.total_count == 0

    def test_failed_result(self):
        """Test failed result."""
        result = PIQKnowledgeResult(
            success=False,
            error="Connection timeout",
        )
        assert result.success is False
        assert result.error == "Connection timeout"
        assert result.knowledge == []


class TestPIQClient:
    """Tests for PIQClient."""

    @pytest.fixture
    def config(self):
        """Create test config."""
        return PIQConfig(
            base_url="http://test-piq:8000",
            api_key="test-key",
        )

    @pytest.fixture
    def client(self, config):
        """Create test client."""
        return PIQClient(config)

    def test_client_initialization(self, client, config):
        """Test client initialization."""
        assert client.config == config
        assert client.config.base_url == "http://test-piq:8000"

    def test_client_disabled(self):
        """Test client with disabled config."""
        config = PIQConfig(enabled=False)
        client = PIQClient(config)
        assert client.config.enabled is False

    def test_client_default_config(self):
        """Test client with default config."""
        client = PIQClient()
        assert client.config.base_url == "http://localhost:8000"
        assert client.config.timeout == 30

    @pytest.mark.asyncio
    async def test_query_when_disabled(self):
        """Test query returns failure when disabled."""
        config = PIQConfig(enabled=False)
        client = PIQClient(config)

        result = await client.query(
            knowledge_type=KnowledgeType.ARCHITECTURE,
            context={},
        )
        assert result.success is False
        assert "disabled" in result.error.lower()

    @pytest.mark.asyncio
    async def test_query_success(self, client):
        """Test successful query."""
        mock_response = MagicMock()
        mock_response.status_code = 200
        mock_response.json.return_value = {
            "knowledge": [
                {
                    "id": "k-001",
                    "knowledge_type": "architecture",
                    "title": "ResNet",
                    "content": "Deep residual networks",
                    "source": "paper",
                    "confidence": 0.95,
                }
            ],
            "suggestions": ["resnet50", "resnet101"],
            "total_count": 1,
            "query_time_ms": 25,
        }

        with patch.object(client, "_client") as mock_client:
            mock_client.post = AsyncMock(return_value=mock_response)
            client._client = mock_client

            result = await client.query(
                knowledge_type=KnowledgeType.ARCHITECTURE,
                context={"task": "classification"},
            )

            assert result.success is True
            assert len(result.knowledge) == 1
            assert result.knowledge[0].title == "ResNet"
            assert "resnet50" in result.suggestions

    @pytest.mark.asyncio
    async def test_query_error_handling(self, client):
        """Test query handles errors gracefully."""
        with patch.object(client, "_client") as mock_client:
            mock_client.post = AsyncMock(side_effect=Exception("Connection error"))
            client._client = mock_client

            result = await client.query(
                knowledge_type=KnowledgeType.ARCHITECTURE,
                context={},
            )

            assert result.success is False
            assert result.error is not None

    @pytest.mark.asyncio
    async def test_get_suggestions(self, client):
        """Test getting suggestions for a parameter."""
        mock_result = PIQKnowledgeResult(
            success=True,
            knowledge=[],
            suggestions=["resnet50", "efficientnet", "vit"],
            total_count=0,
        )

        with patch.object(client, "query", new_callable=AsyncMock) as mock_query:
            mock_query.return_value = mock_result

            suggestions = await client.get_suggestions(
                template_id="image-classification",
                param_name="backbone",
                current_params={},
            )

            assert isinstance(suggestions, list)
            assert "resnet50" in suggestions

    @pytest.mark.asyncio
    async def test_get_best_practices(self, client):
        """Test getting best practices."""
        mock_result = PIQKnowledgeResult(
            success=True,
            knowledge=[
                PIQKnowledge(
                    id="bp-001",
                    knowledge_type=KnowledgeType.BEST_PRACTICE,
                    title="Use pretrained weights",
                    content="Always use pretrained weights for transfer learning",
                    source=KnowledgeSource.CURATED,
                )
            ],
            suggestions=[],
            total_count=1,
        )

        with patch.object(client, "query", new_callable=AsyncMock) as mock_query:
            mock_query.return_value = mock_result

            practices = await client.get_best_practices(
                task_type="classification",
                dataset="imagenet",
            )

            assert len(practices) == 1
            assert practices[0].title == "Use pretrained weights"

    @pytest.mark.asyncio
    async def test_get_related_papers(self, client):
        """Test getting related papers."""
        mock_result = PIQKnowledgeResult(
            success=True,
            knowledge=[
                PIQKnowledge(
                    id="p-001",
                    knowledge_type=KnowledgeType.PAPER,
                    title="ImageNet Classification with Deep CNNs",
                    content="AlexNet paper description",
                    source=KnowledgeSource.PAPER,
                    paper_url="https://arxiv.org/abs/1234",
                )
            ],
            suggestions=[],
            total_count=1,
        )

        with patch.object(client, "query", new_callable=AsyncMock) as mock_query:
            mock_query.return_value = mock_result

            papers = await client.get_related_papers(
                template_id="image-classification",
                technique="cnn",
            )

            assert len(papers) == 1
            assert papers[0].paper_url is not None

    @pytest.mark.asyncio
    async def test_health_check_disabled(self):
        """Test health check when disabled."""
        config = PIQConfig(enabled=False)
        client = PIQClient(config)

        result = await client.health_check()
        assert result is False

    @pytest.mark.asyncio
    async def test_health_check_success(self, client):
        """Test successful health check."""
        mock_response = MagicMock()
        mock_response.status_code = 200

        with patch.object(client, "_client") as mock_client:
            mock_client.get = AsyncMock(return_value=mock_response)
            client._client = mock_client

            result = await client.health_check()
            assert result is True


class TestGetPIQClient:
    """Tests for get_piq_client factory function."""

    def test_get_client_creates_instance(self):
        """Test getting client creates an instance."""
        # Reset global singleton for testing
        import c4.templates.piq_protocol as piq_module
        piq_module._piq_client = None

        client = get_piq_client()
        assert client is not None
        assert isinstance(client, PIQClient)

    def test_get_client_with_config(self):
        """Test getting client with explicit config."""
        import c4.templates.piq_protocol as piq_module
        piq_module._piq_client = None

        config = PIQConfig(base_url="http://custom:9000")
        client = get_piq_client(config)
        assert client.config.base_url == "http://custom:9000"

    def test_get_client_singleton_behavior(self):
        """Test that clients are cached/reused."""
        import c4.templates.piq_protocol as piq_module
        piq_module._piq_client = None

        client1 = get_piq_client()
        client2 = get_piq_client()
        # Should be the same instance
        assert client1 is client2


class TestPIQClientCache:
    """Tests for PIQ client caching behavior."""

    @pytest.fixture
    def client(self):
        """Create test client."""
        config = PIQConfig(cache_ttl=3600)
        return PIQClient(config)

    def test_cache_key_generation(self, client):
        """Test that cache keys are generated consistently."""
        query1 = PIQKnowledgeQuery(
            knowledge_type=KnowledgeType.ARCHITECTURE,
            context={"task": "classification"},
        )
        query2 = PIQKnowledgeQuery(
            knowledge_type=KnowledgeType.ARCHITECTURE,
            context={"task": "classification"},
        )
        query3 = PIQKnowledgeQuery(
            knowledge_type=KnowledgeType.OPTIMIZER,
            context={"task": "classification"},
        )

        key1 = client._make_cache_key(query1)
        key2 = client._make_cache_key(query2)
        key3 = client._make_cache_key(query3)

        # Same query should produce same key
        assert key1 == key2
        # Different query should produce different key
        assert key1 != key3


class TestPIQParsingMethods:
    """Tests for PIQ client parsing methods."""

    @pytest.fixture
    def client(self):
        """Create test client."""
        return PIQClient()

    def test_parse_knowledge(self, client):
        """Test parsing a knowledge item from API response."""
        data = {
            "id": "k-001",
            "knowledge_type": "architecture",
            "title": "ResNet",
            "content": "Deep residual networks",
            "source": "paper",
            "confidence": 0.95,
            "paper_id": "1512.03385",
            "paper_title": "Deep Residual Learning",
            "paper_url": "https://arxiv.org/abs/1512.03385",
            "tags": ["cnn", "vision"],
            "related_ids": ["k-002", "k-003"],
        }

        knowledge = client._parse_knowledge(data)

        assert knowledge.id == "k-001"
        assert knowledge.knowledge_type == KnowledgeType.ARCHITECTURE
        assert knowledge.title == "ResNet"
        assert knowledge.content == "Deep residual networks"
        assert knowledge.source == KnowledgeSource.PAPER
        assert knowledge.confidence == 0.95
        assert knowledge.paper_id == "1512.03385"
        assert "cnn" in knowledge.tags

    def test_parse_query_result(self, client):
        """Test parsing a query result from API response."""
        data = {
            "knowledge": [
                {
                    "id": "k-001",
                    "knowledge_type": "backbone",
                    "title": "EfficientNet",
                    "content": "Efficient backbone",
                    "source": "curated",
                }
            ],
            "suggestions": ["efficientnet_b0", "efficientnet_b1"],
            "total_count": 1,
            "query_time_ms": 30,
        }

        result = client._parse_query_result(data)

        assert result.success is True
        assert len(result.knowledge) == 1
        assert result.knowledge[0].title == "EfficientNet"
        assert result.total_count == 1
        assert result.query_time_ms == 30
        assert "efficientnet_b0" in result.suggestions
