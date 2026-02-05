"""Tests for embeddings provider module."""

from unittest.mock import MagicMock, patch

import pytest

from c4.memory.embeddings import (
    EmbeddingProvider,
    LocalEmbeddings,
    MockEmbeddings,
    OpenAIEmbeddings,
    get_embeddings_provider,
)


class TestMockEmbeddings:
    """Tests for MockEmbeddings provider."""

    def test_dimension_default(self) -> None:
        """Default dimension should be 384."""
        provider = MockEmbeddings()
        assert provider.dimension == 384

    def test_dimension_custom(self) -> None:
        """Custom dimension should be respected."""
        provider = MockEmbeddings(dimension=1536)
        assert provider.dimension == 1536

    def test_embed_returns_correct_dimension(self) -> None:
        """Embedding should have correct dimension."""
        provider = MockEmbeddings(dimension=128)
        embedding = provider.embed("test text")
        assert len(embedding) == 128

    def test_embed_returns_floats(self) -> None:
        """Embedding should contain floats."""
        provider = MockEmbeddings()
        embedding = provider.embed("test text")
        assert all(isinstance(x, float) for x in embedding)

    def test_embed_values_in_range(self) -> None:
        """Embedding values should be in [-1, 1] range."""
        provider = MockEmbeddings()
        embedding = provider.embed("test text")
        assert all(-1 <= x <= 1 for x in embedding)

    def test_embed_deterministic(self) -> None:
        """Same text should produce same embedding."""
        provider = MockEmbeddings()
        embedding1 = provider.embed("hello")
        embedding2 = provider.embed("hello")
        assert embedding1 == embedding2

    def test_embed_different_texts_different_embeddings(self) -> None:
        """Different texts should produce different embeddings."""
        provider = MockEmbeddings()
        embedding1 = provider.embed("hello")
        embedding2 = provider.embed("world")
        assert embedding1 != embedding2

    def test_embed_batch_empty(self) -> None:
        """Empty batch should return empty list."""
        provider = MockEmbeddings()
        result = provider.embed_batch([])
        assert result == []

    def test_embed_batch_multiple(self) -> None:
        """Batch embedding should return list of embeddings."""
        provider = MockEmbeddings(dimension=64)
        texts = ["hello", "world", "test"]
        result = provider.embed_batch(texts)
        assert len(result) == 3
        assert all(len(emb) == 64 for emb in result)

    def test_implements_protocol(self) -> None:
        """MockEmbeddings should implement EmbeddingProvider protocol."""
        provider = MockEmbeddings()
        assert isinstance(provider, EmbeddingProvider)


class TestOpenAIEmbeddings:
    """Tests for OpenAIEmbeddings provider."""

    def test_dimension_text_embedding_3_small(self) -> None:
        """text-embedding-3-small should have dimension 1536."""
        provider = OpenAIEmbeddings(model="text-embedding-3-small")
        assert provider.dimension == 1536

    def test_dimension_text_embedding_3_large(self) -> None:
        """text-embedding-3-large should have dimension 3072."""
        provider = OpenAIEmbeddings(model="text-embedding-3-large")
        assert provider.dimension == 3072

    def test_dimension_text_embedding_ada_002(self) -> None:
        """text-embedding-ada-002 should have dimension 1536."""
        provider = OpenAIEmbeddings(model="text-embedding-ada-002")
        assert provider.dimension == 1536

    def test_dimension_unknown_model(self) -> None:
        """Unknown model should default to 1536."""
        provider = OpenAIEmbeddings(model="unknown-model")
        assert provider.dimension == 1536

    def test_embed_calls_api(self) -> None:
        """embed() should call OpenAI API."""
        provider = OpenAIEmbeddings(api_key="test-key")

        # Mock the OpenAI client
        mock_response = MagicMock()
        mock_response.data = [MagicMock(embedding=[0.1, 0.2, 0.3])]

        with patch.object(provider, "_get_client") as mock_get_client:
            mock_client = MagicMock()
            mock_client.embeddings.create.return_value = mock_response
            mock_get_client.return_value = mock_client

            result = provider.embed("test text")

            mock_client.embeddings.create.assert_called_once_with(
                input=["test text"], model="text-embedding-3-small"
            )
            assert result == [0.1, 0.2, 0.3]

    def test_embed_batch_calls_api(self) -> None:
        """embed_batch() should call OpenAI API with all texts."""
        provider = OpenAIEmbeddings(api_key="test-key")

        mock_response = MagicMock()
        mock_response.data = [
            MagicMock(embedding=[0.1, 0.2]),
            MagicMock(embedding=[0.3, 0.4]),
        ]

        with patch.object(provider, "_get_client") as mock_get_client:
            mock_client = MagicMock()
            mock_client.embeddings.create.return_value = mock_response
            mock_get_client.return_value = mock_client

            result = provider.embed_batch(["text1", "text2"])

            assert len(result) == 2
            assert result[0] == [0.1, 0.2]
            assert result[1] == [0.3, 0.4]

    def test_embed_batch_empty(self) -> None:
        """Empty batch should return empty list without API call."""
        provider = OpenAIEmbeddings(api_key="test-key")
        result = provider.embed_batch([])
        assert result == []


class TestLocalEmbeddings:
    """Tests for LocalEmbeddings provider."""

    def test_dimension_known_model(self) -> None:
        """Known model should return correct dimension without loading."""
        provider = LocalEmbeddings(model_name="all-MiniLM-L6-v2")
        assert provider.dimension == 384

    def test_dimension_mpnet(self) -> None:
        """mpnet model should have dimension 768."""
        provider = LocalEmbeddings(model_name="all-mpnet-base-v2")
        assert provider.dimension == 768

    def test_embed_raises_import_error(self) -> None:
        """embed() should raise ImportError if sentence-transformers not installed."""
        provider = LocalEmbeddings()

        with patch.dict("sys.modules", {"sentence_transformers": None}):
            with patch("builtins.__import__", side_effect=ImportError):
                with pytest.raises(ImportError, match="sentence-transformers"):
                    provider.embed("test")

    def test_embed_batch_empty(self) -> None:
        """Empty batch should return empty list without loading model."""
        provider = LocalEmbeddings()
        result = provider.embed_batch([])
        assert result == []


class TestGetEmbeddingsProvider:
    """Tests for get_embeddings_provider factory function."""

    def test_get_mock_provider(self) -> None:
        """Should return MockEmbeddings for 'mock'."""
        provider = get_embeddings_provider("mock")
        assert isinstance(provider, MockEmbeddings)

    def test_get_openai_provider(self) -> None:
        """Should return OpenAIEmbeddings for 'openai'."""
        provider = get_embeddings_provider("openai")
        assert isinstance(provider, OpenAIEmbeddings)

    def test_get_local_provider(self) -> None:
        """Should return LocalEmbeddings for 'local'."""
        provider = get_embeddings_provider("local")
        assert isinstance(provider, LocalEmbeddings)

    def test_unknown_provider_raises_error(self) -> None:
        """Unknown provider should raise ValueError."""
        with pytest.raises(ValueError, match="Unknown provider"):
            get_embeddings_provider("invalid")

    def test_default_without_api_key(self) -> None:
        """Default should be mock if no API key."""
        with patch.dict("os.environ", {}, clear=True):
            # Remove OPENAI_API_KEY if present
            import os
            os.environ.pop("OPENAI_API_KEY", None)
            provider = get_embeddings_provider()
            assert isinstance(provider, MockEmbeddings)

    def test_default_with_api_key(self) -> None:
        """Default should be openai if API key is present."""
        with patch.dict("os.environ", {"OPENAI_API_KEY": "test-key"}):
            provider = get_embeddings_provider()
            assert isinstance(provider, OpenAIEmbeddings)

    def test_kwargs_passed_to_provider(self) -> None:
        """Kwargs should be passed to provider constructor."""
        provider = get_embeddings_provider("mock", dimension=256)
        assert provider.dimension == 256

    def test_openai_kwargs(self) -> None:
        """OpenAI provider should accept model kwarg."""
        provider = get_embeddings_provider("openai", model="text-embedding-3-large")
        assert provider.model == "text-embedding-3-large"
        assert provider.dimension == 3072


class TestEmbeddingProviderProtocol:
    """Tests for EmbeddingProvider protocol compliance."""

    def test_mock_implements_protocol(self) -> None:
        """MockEmbeddings should implement protocol."""
        provider = MockEmbeddings()
        assert isinstance(provider, EmbeddingProvider)

    def test_openai_implements_protocol(self) -> None:
        """OpenAIEmbeddings should implement protocol."""
        provider = OpenAIEmbeddings()
        assert isinstance(provider, EmbeddingProvider)

    def test_local_implements_protocol(self) -> None:
        """LocalEmbeddings should implement protocol."""
        provider = LocalEmbeddings()
        assert isinstance(provider, EmbeddingProvider)
