"""Embeddings provider module for vector-based knowledge storage.

This module provides abstractions for generating text embeddings
from different providers (OpenAI, local models, etc.).

Usage:
    from c4.knowledge.embeddings_provider import get_embeddings_provider

    # Get default provider
    provider = get_embeddings_provider()
    embedding = provider.embed("Hello, world!")

    # Get specific provider
    provider = get_embeddings_provider("openai")
"""

import os
from abc import abstractmethod
from typing import Protocol, runtime_checkable


@runtime_checkable
class EmbeddingProvider(Protocol):
    """Protocol for embedding providers.

    All embedding providers must implement:
    - embed(): Convert text to embedding vector
    - dimension: Property returning the embedding dimension
    """

    @property
    @abstractmethod
    def dimension(self) -> int:
        """Return the dimension of embeddings produced by this provider."""
        ...

    @abstractmethod
    def embed(self, text: str) -> list[float]:
        """Convert text to an embedding vector.

        Args:
            text: The text to embed.

        Returns:
            A list of floats representing the embedding vector.
        """
        ...

    @abstractmethod
    def embed_batch(self, texts: list[str]) -> list[list[float]]:
        """Convert multiple texts to embedding vectors.

        Args:
            texts: List of texts to embed.

        Returns:
            List of embedding vectors.
        """
        ...


class OpenAIEmbeddings:
    """OpenAI embeddings provider using text-embedding-3-small.

    Uses OpenAI's API to generate high-quality embeddings.
    Requires OPENAI_API_KEY environment variable.

    Attributes:
        model: The OpenAI model to use (default: text-embedding-3-small)
        dimension: The embedding dimension (1536 for text-embedding-3-small)
    """

    def __init__(self, model: str = "text-embedding-3-small", api_key: str | None = None) -> None:
        """Initialize OpenAI embeddings provider.

        Args:
            model: OpenAI model name. Defaults to "text-embedding-3-small".
            api_key: OpenAI API key. If None, reads from OPENAI_API_KEY env var.
        """
        self.model = model
        self._api_key = api_key or os.environ.get("OPENAI_API_KEY")
        self._client = None

        # Dimension map for known models
        self._dimensions = {
            "text-embedding-3-small": 1536,
            "text-embedding-3-large": 3072,
            "text-embedding-ada-002": 1536,
        }

    @property
    def dimension(self) -> int:
        """Return the embedding dimension for this model."""
        return self._dimensions.get(self.model, 1536)

    def _get_client(self):
        """Get or create OpenAI client lazily."""
        if self._client is None:
            try:
                from openai import OpenAI

                self._client = OpenAI(api_key=self._api_key)
            except ImportError:
                raise ImportError("openai package is required. Install with: uv add openai")
        return self._client

    def embed(self, text: str) -> list[float]:
        """Convert text to an embedding vector using OpenAI API.

        Args:
            text: The text to embed.

        Returns:
            A list of floats representing the embedding vector.

        Raises:
            ImportError: If openai package is not installed.
            openai.AuthenticationError: If API key is invalid.
        """
        client = self._get_client()
        response = client.embeddings.create(input=[text], model=self.model)
        return response.data[0].embedding

    def embed_batch(self, texts: list[str]) -> list[list[float]]:
        """Convert multiple texts to embedding vectors.

        Args:
            texts: List of texts to embed.

        Returns:
            List of embedding vectors.
        """
        if not texts:
            return []

        client = self._get_client()
        response = client.embeddings.create(input=texts, model=self.model)
        return [item.embedding for item in response.data]


class LocalEmbeddings:
    """Local embeddings provider using sentence-transformers.

    Uses a local model for embedding generation without external API calls.
    Requires sentence-transformers package (optional dependency).

    Attributes:
        model_name: The sentence-transformers model to use
        dimension: The embedding dimension (depends on model)
    """

    def __init__(self, model_name: str = "all-MiniLM-L6-v2") -> None:
        """Initialize local embeddings provider.

        Args:
            model_name: sentence-transformers model name.
                Defaults to "all-MiniLM-L6-v2" (384 dimensions).
        """
        self.model_name = model_name
        self._model = None
        self._dimension = None

        # Known model dimensions
        self._known_dimensions = {
            "all-MiniLM-L6-v2": 384,
            "all-MiniLM-L12-v2": 384,
            "all-mpnet-base-v2": 768,
            "all-distilroberta-v1": 768,
        }

    @property
    def dimension(self) -> int:
        """Return the embedding dimension for this model."""
        if self._dimension is not None:
            return self._dimension

        if self.model_name in self._known_dimensions:
            return self._known_dimensions[self.model_name]

        # Load model to get dimension if unknown
        model = self._get_model()
        self._dimension = model.get_sentence_embedding_dimension()
        return self._dimension

    def _get_model(self):
        """Get or create model lazily."""
        if self._model is None:
            try:
                from sentence_transformers import SentenceTransformer

                self._model = SentenceTransformer(self.model_name)
            except ImportError:
                raise ImportError(
                    "sentence-transformers package is required. " "Install with: uv add sentence-transformers"
                )
        return self._model

    def embed(self, text: str) -> list[float]:
        """Convert text to an embedding vector using local model.

        Args:
            text: The text to embed.

        Returns:
            A list of floats representing the embedding vector.

        Raises:
            ImportError: If sentence-transformers package is not installed.
        """
        model = self._get_model()
        embedding = model.encode(text, convert_to_numpy=True)
        return embedding.tolist()

    def embed_batch(self, texts: list[str]) -> list[list[float]]:
        """Convert multiple texts to embedding vectors.

        Args:
            texts: List of texts to embed.

        Returns:
            List of embedding vectors.
        """
        if not texts:
            return []

        model = self._get_model()
        embeddings = model.encode(texts, convert_to_numpy=True)
        return [emb.tolist() for emb in embeddings]


class OllamaEmbeddings:
    """Ollama embeddings provider using nomic-embed-text (768-dim).

    Uses a local Ollama server for embedding generation without external API calls.
    Defaults to nomic-embed-text model which produces 768-dimensional embeddings.

    Attributes:
        model: The Ollama model to use (default: nomic-embed-text)
        base_url: The Ollama server URL (default: http://localhost:11434)
        dimension: The embedding dimension (768 for nomic-embed-text)
    """

    def __init__(
        self,
        model: str = "nomic-embed-text",
        base_url: str = "http://localhost:11434",
    ) -> None:
        self.model = model
        self.base_url = base_url.rstrip("/")

        # Known model dimensions
        self._known_dimensions = {
            "nomic-embed-text": 768,
            "mxbai-embed-large": 1024,
            "all-minilm": 384,
        }

    @property
    def dimension(self) -> int:
        """Return the embedding dimension for this model."""
        return self._known_dimensions.get(self.model, 768)

    def embed(self, text: str) -> list[float]:
        """Convert text to an embedding vector using Ollama API."""
        import json
        import urllib.error
        import urllib.request

        payload = json.dumps({"model": self.model, "prompt": text}).encode()
        url = f"{self.base_url}/api/embeddings"
        try:
            req = urllib.request.Request(
                url,
                data=payload,
                headers={"Content-Type": "application/json"},
                method="POST",
            )
            with urllib.request.urlopen(req, timeout=30) as resp:
                data = json.loads(resp.read())
                return data["embedding"]
        except urllib.error.URLError as e:
            raise RuntimeError(
                f"Cannot connect to Ollama at {self.base_url}. "
                "Ensure Ollama is running: `ollama serve`"
            ) from e
        except KeyError as e:
            raise RuntimeError("Unexpected response from Ollama (missing 'embedding' key)") from e

    def embed_batch(self, texts: list[str]) -> list[list[float]]:
        """Convert multiple texts to embedding vectors (sequential calls)."""
        return [self.embed(text) for text in texts]


class MockEmbeddings:
    """Mock embeddings provider for testing.

    Generates deterministic pseudo-embeddings based on text hash.
    Useful for testing without requiring external dependencies or API calls.
    """

    def __init__(self, dimension: int = 384) -> None:
        """Initialize mock embeddings provider.

        Args:
            dimension: The embedding dimension to produce.
        """
        self._dimension = dimension

    @property
    def dimension(self) -> int:
        """Return the mock embedding dimension."""
        return self._dimension

    def embed(self, text: str) -> list[float]:
        """Generate a deterministic pseudo-embedding from text.

        Args:
            text: The text to embed.

        Returns:
            A deterministic list of floats based on text hash.
        """
        # Generate deterministic values from text hash
        import hashlib

        hash_bytes = hashlib.sha256(text.encode()).digest()

        # Generate enough floats to fill dimension
        result = []
        for i in range(self._dimension):
            # Use hash bytes to generate values between -1 and 1
            byte_idx = i % len(hash_bytes)
            value = (hash_bytes[byte_idx] / 255.0) * 2 - 1
            result.append(value)

        return result

    def embed_batch(self, texts: list[str]) -> list[list[float]]:
        """Generate embeddings for multiple texts.

        Args:
            texts: List of texts to embed.

        Returns:
            List of embedding vectors.
        """
        return [self.embed(text) for text in texts]


# Provider registry
_PROVIDERS = {
    "openai": OpenAIEmbeddings,
    "local": LocalEmbeddings,
    "mock": MockEmbeddings,
    "ollama": OllamaEmbeddings,
}


def get_embeddings_provider(
    provider_name: str | None = None, **kwargs
) -> EmbeddingProvider:
    """Factory function to get an embeddings provider.

    Args:
        provider_name: Name of the provider ("openai", "local", "mock").
            If None, defaults to "mock" (for testing) or "openai" if API key is available.
        **kwargs: Additional arguments to pass to the provider constructor.

    Returns:
        An EmbeddingProvider instance.

    Raises:
        ValueError: If provider_name is not recognized.

    Example:
        >>> provider = get_embeddings_provider("mock")
        >>> embedding = provider.embed("Hello, world!")
        >>> len(embedding) == provider.dimension
        True
    """
    if provider_name is None:
        # Auto-detect: use openai if API key available, else mock
        if os.environ.get("OPENAI_API_KEY"):
            provider_name = "openai"
        else:
            provider_name = "mock"

    if provider_name not in _PROVIDERS:
        valid = ", ".join(_PROVIDERS.keys())
        raise ValueError(f"Unknown provider: {provider_name}. Valid providers: {valid}")

    return _PROVIDERS[provider_name](**kwargs)
