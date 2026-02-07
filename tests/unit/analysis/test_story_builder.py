"""Tests for c4/memory/story_builder.py"""

import tempfile
from datetime import datetime, timedelta
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from c4.analysis.git.story_builder import (
    AIStoryGenerator,
    Cluster,
    CommitEmbedding,
    HeuristicStoryGenerator,
    Story,
    StoryBuilder,
    calculate_cluster_coherence,
    cluster_agglomerative,
    compute_centroid,
    cosine_similarity,
    euclidean_distance,
    get_story_builder,
)

# =============================================================================
# CommitEmbedding Tests
# =============================================================================


class TestCommitEmbedding:
    """Tests for CommitEmbedding dataclass."""

    def test_create_basic(self) -> None:
        """Should create with required fields."""
        commit = CommitEmbedding(
            sha="abc123",
            embedding=[1.0, 0.0, 0.0],
        )
        assert commit.sha == "abc123"
        assert commit.embedding == [1.0, 0.0, 0.0]
        assert commit.message == ""
        assert commit.category == ""
        assert commit.domains == []

    def test_create_with_all_fields(self) -> None:
        """Should create with all fields."""
        ts = datetime.now()
        commit = CommitEmbedding(
            sha="def456",
            embedding=[0.5, 0.5, 0.0],
            message="fix: resolve bug",
            category="bug_fix",
            domains=["auth", "api"],
            timestamp=ts,
            metadata={"key": "value"},
        )
        assert commit.message == "fix: resolve bug"
        assert commit.category == "bug_fix"
        assert commit.domains == ["auth", "api"]
        assert commit.timestamp == ts
        assert commit.metadata == {"key": "value"}


# =============================================================================
# Cluster Tests
# =============================================================================


class TestCluster:
    """Tests for Cluster dataclass."""

    def test_create_basic(self) -> None:
        """Should create with required fields."""
        commits = [CommitEmbedding(sha="abc", embedding=[1.0, 0.0])]
        cluster = Cluster(id="cluster-001", commits=commits)

        assert cluster.id == "cluster-001"
        assert len(cluster.commits) == 1
        assert cluster.centroid is None
        assert cluster.coherence == 0.0

    def test_size_property(self) -> None:
        """Should return correct size."""
        commits = [
            CommitEmbedding(sha="a", embedding=[1.0, 0.0]),
            CommitEmbedding(sha="b", embedding=[0.0, 1.0]),
        ]
        cluster = Cluster(id="cluster-001", commits=commits)

        assert cluster.size == 2
        assert len(cluster) == 2


# =============================================================================
# Story Tests
# =============================================================================


class TestStory:
    """Tests for Story dataclass."""

    def test_create_basic(self) -> None:
        """Should create with required fields."""
        story = Story(
            id="story-001",
            title="Bug fixes",
            description="Fixed several bugs",
            commits=["abc", "def"],
        )
        assert story.id == "story-001"
        assert story.title == "Bug fixes"
        assert story.commits == ["abc", "def"]

    def test_to_dict(self) -> None:
        """Should convert to dictionary."""
        story = Story(
            id="story-001",
            title="New features",
            description="Added features",
            commits=["abc"],
            cluster_id="cluster-001",
            category="feature",
            domains=["api"],
            time_span="Jan 15-20, 2025",
        )
        result = story.to_dict()

        assert result["id"] == "story-001"
        assert result["title"] == "New features"
        assert result["commits"] == ["abc"]
        assert result["category"] == "feature"
        assert result["domains"] == ["api"]

    def test_from_dict(self) -> None:
        """Should create from dictionary."""
        data = {
            "id": "story-002",
            "title": "Refactoring",
            "description": "Code cleanup",
            "commits": ["ghi"],
            "category": "refactor",
            "domains": ["backend"],
        }
        story = Story.from_dict(data)

        assert story.id == "story-002"
        assert story.title == "Refactoring"
        assert story.commits == ["ghi"]
        assert story.category == "refactor"

    def test_roundtrip(self) -> None:
        """Should survive to_dict/from_dict roundtrip."""
        original = Story(
            id="story-003",
            title="Test",
            description="Description",
            commits=["a", "b"],
            category="test",
            domains=["testing"],
        )
        restored = Story.from_dict(original.to_dict())

        assert restored.id == original.id
        assert restored.title == original.title
        assert restored.commits == original.commits


# =============================================================================
# Similarity Functions Tests
# =============================================================================


class TestCosineSimilarity:
    """Tests for cosine_similarity function."""

    def test_identical_vectors(self) -> None:
        """Should return 1.0 for identical vectors."""
        v = [1.0, 2.0, 3.0]
        assert cosine_similarity(v, v) == pytest.approx(1.0)

    def test_orthogonal_vectors(self) -> None:
        """Should return 0.0 for orthogonal vectors."""
        v1 = [1.0, 0.0, 0.0]
        v2 = [0.0, 1.0, 0.0]
        assert cosine_similarity(v1, v2) == pytest.approx(0.0)

    def test_opposite_vectors(self) -> None:
        """Should return -1.0 for opposite vectors."""
        v1 = [1.0, 0.0]
        v2 = [-1.0, 0.0]
        assert cosine_similarity(v1, v2) == pytest.approx(-1.0)

    def test_similar_vectors(self) -> None:
        """Should return high similarity for similar vectors."""
        v1 = [1.0, 0.0, 0.0]
        v2 = [0.9, 0.1, 0.0]
        sim = cosine_similarity(v1, v2)
        assert sim > 0.9

    def test_dimension_mismatch(self) -> None:
        """Should raise error for dimension mismatch."""
        with pytest.raises(ValueError):
            cosine_similarity([1.0, 0.0], [1.0, 0.0, 0.0])

    def test_zero_vector(self) -> None:
        """Should return 0.0 for zero vectors."""
        v1 = [0.0, 0.0]
        v2 = [1.0, 0.0]
        assert cosine_similarity(v1, v2) == 0.0


class TestEuclideanDistance:
    """Tests for euclidean_distance function."""

    def test_identical_vectors(self) -> None:
        """Should return 0.0 for identical vectors."""
        v = [1.0, 2.0, 3.0]
        assert euclidean_distance(v, v) == pytest.approx(0.0)

    def test_unit_distance(self) -> None:
        """Should return correct distance."""
        v1 = [0.0, 0.0]
        v2 = [1.0, 0.0]
        assert euclidean_distance(v1, v2) == pytest.approx(1.0)

    def test_pythagorean(self) -> None:
        """Should follow Pythagorean theorem."""
        v1 = [0.0, 0.0]
        v2 = [3.0, 4.0]
        assert euclidean_distance(v1, v2) == pytest.approx(5.0)

    def test_dimension_mismatch(self) -> None:
        """Should raise error for dimension mismatch."""
        with pytest.raises(ValueError):
            euclidean_distance([1.0], [1.0, 0.0])


class TestComputeCentroid:
    """Tests for compute_centroid function."""

    def test_single_embedding(self) -> None:
        """Should return embedding itself for single item."""
        emb = [1.0, 2.0, 3.0]
        centroid = compute_centroid([emb])
        assert centroid == emb

    def test_two_embeddings(self) -> None:
        """Should return midpoint for two embeddings."""
        e1 = [0.0, 0.0]
        e2 = [2.0, 2.0]
        centroid = compute_centroid([e1, e2])
        assert centroid == [1.0, 1.0]

    def test_empty_list(self) -> None:
        """Should return empty for empty list."""
        assert compute_centroid([]) == []

    def test_multiple_embeddings(self) -> None:
        """Should return average for multiple embeddings."""
        embeddings = [
            [3.0, 0.0],
            [0.0, 3.0],
            [0.0, 0.0],
        ]
        centroid = compute_centroid(embeddings)
        assert centroid == pytest.approx([1.0, 1.0])


# =============================================================================
# Clustering Tests
# =============================================================================


class TestClusterAgglomerative:
    """Tests for cluster_agglomerative function."""

    def test_empty_commits(self) -> None:
        """Should return empty for empty input."""
        assert cluster_agglomerative([]) == []

    def test_single_commit(self) -> None:
        """Should create single cluster for single commit."""
        commits = [CommitEmbedding(sha="abc", embedding=[1.0, 0.0])]
        clusters = cluster_agglomerative(commits)

        assert len(clusters) == 1
        assert len(clusters[0].commits) == 1

    def test_similar_commits_merged(self) -> None:
        """Should merge similar commits."""
        commits = [
            CommitEmbedding(sha="a", embedding=[1.0, 0.0, 0.0]),
            CommitEmbedding(sha="b", embedding=[0.95, 0.05, 0.0]),
            CommitEmbedding(sha="c", embedding=[0.0, 0.0, 1.0]),  # Different
        ]
        clusters = cluster_agglomerative(commits, threshold=0.9)

        # a and b should be merged, c separate
        assert len(clusters) == 2

    def test_dissimilar_commits_separate(self) -> None:
        """Should keep dissimilar commits separate."""
        commits = [
            CommitEmbedding(sha="a", embedding=[1.0, 0.0, 0.0]),
            CommitEmbedding(sha="b", embedding=[0.0, 1.0, 0.0]),
            CommitEmbedding(sha="c", embedding=[0.0, 0.0, 1.0]),
        ]
        clusters = cluster_agglomerative(commits, threshold=0.9)

        # All orthogonal, should be separate
        assert len(clusters) == 3

    def test_cluster_has_centroid(self) -> None:
        """Should compute centroid for clusters."""
        commits = [
            CommitEmbedding(sha="a", embedding=[1.0, 0.0]),
            CommitEmbedding(sha="b", embedding=[0.0, 1.0]),
        ]
        # Use negative threshold to force merge (similarity 0.0 > -0.1 is True)
        clusters = cluster_agglomerative(commits, threshold=-0.1)

        assert len(clusters) == 1
        assert clusters[0].centroid is not None
        assert len(clusters[0].centroid) == 2

    def test_cluster_has_coherence(self) -> None:
        """Should calculate coherence for clusters."""
        commits = [
            CommitEmbedding(sha="a", embedding=[1.0, 0.0]),
            CommitEmbedding(sha="b", embedding=[0.99, 0.01]),
        ]
        clusters = cluster_agglomerative(commits, threshold=0.5)

        assert len(clusters) == 1
        assert clusters[0].coherence > 0.9

    def test_dominant_category(self) -> None:
        """Should find dominant category."""
        commits = [
            CommitEmbedding(sha="a", embedding=[1.0, 0.0], category="bug_fix"),
            CommitEmbedding(sha="b", embedding=[0.99, 0.01], category="bug_fix"),
            CommitEmbedding(sha="c", embedding=[0.98, 0.02], category="feature"),
        ]
        clusters = cluster_agglomerative(commits, threshold=0.5)

        assert len(clusters) == 1
        assert clusters[0].dominant_category == "bug_fix"

    def test_dominant_domains(self) -> None:
        """Should collect dominant domains."""
        commits = [
            CommitEmbedding(sha="a", embedding=[1.0, 0.0], domains=["auth"]),
            CommitEmbedding(sha="b", embedding=[0.99, 0.01], domains=["auth", "api"]),
        ]
        clusters = cluster_agglomerative(commits, threshold=0.5)

        assert len(clusters) == 1
        assert "auth" in clusters[0].dominant_domains
        assert "api" in clusters[0].dominant_domains


class TestCalculateClusterCoherence:
    """Tests for calculate_cluster_coherence function."""

    def test_single_commit(self) -> None:
        """Should return 1.0 for single commit."""
        commits = [CommitEmbedding(sha="a", embedding=[1.0, 0.0])]
        assert calculate_cluster_coherence(commits) == 1.0

    def test_identical_embeddings(self) -> None:
        """Should return 1.0 for identical embeddings."""
        commits = [
            CommitEmbedding(sha="a", embedding=[1.0, 0.0]),
            CommitEmbedding(sha="b", embedding=[1.0, 0.0]),
        ]
        assert calculate_cluster_coherence(commits) == pytest.approx(1.0)

    def test_orthogonal_embeddings(self) -> None:
        """Should return 0.0 for orthogonal embeddings."""
        commits = [
            CommitEmbedding(sha="a", embedding=[1.0, 0.0]),
            CommitEmbedding(sha="b", embedding=[0.0, 1.0]),
        ]
        assert calculate_cluster_coherence(commits) == pytest.approx(0.0)


# =============================================================================
# HeuristicStoryGenerator Tests
# =============================================================================


class TestHeuristicStoryGenerator:
    """Tests for HeuristicStoryGenerator."""

    def test_generate_basic_story(self) -> None:
        """Should generate story with title and description."""
        generator = HeuristicStoryGenerator()
        cluster = Cluster(
            id="cluster-001",
            commits=[
                CommitEmbedding(sha="abc", embedding=[1.0, 0.0], message="fix: resolve bug"),
            ],
            dominant_category="bug_fix",
            dominant_domains=["auth"],
        )

        story = generator.generate_story(cluster)

        assert story.id.startswith("story-")
        assert story.title
        assert story.description
        assert story.commits == ["abc"]
        assert story.cluster_id == "cluster-001"

    def test_title_uses_category(self) -> None:
        """Should generate title based on category."""
        generator = HeuristicStoryGenerator()

        # Bug fix cluster
        cluster = Cluster(
            id="c1",
            commits=[CommitEmbedding(sha="a", embedding=[1.0, 0.0])],
            dominant_category="bug_fix",
            dominant_domains=["api"],
        )
        story = generator.generate_story(cluster)
        assert any(word in story.title.lower() for word in ["bug", "fix", "resolved"])

        # Feature cluster
        cluster = Cluster(
            id="c2",
            commits=[CommitEmbedding(sha="b", embedding=[1.0, 0.0])],
            dominant_category="feature",
            dominant_domains=["auth"],
        )
        story = generator.generate_story(cluster)
        assert any(word in story.title.lower() for word in ["feature", "new", "added", "enhancements"])

    def test_description_includes_commits(self) -> None:
        """Should include commit messages in description."""
        generator = HeuristicStoryGenerator()
        cluster = Cluster(
            id="c1",
            commits=[
                CommitEmbedding(sha="abc", embedding=[1.0, 0.0], message="fix: bug one"),
                CommitEmbedding(sha="def", embedding=[0.9, 0.1], message="fix: bug two"),
            ],
            dominant_category="bug_fix",
            coherence=0.95,
        )

        story = generator.generate_story(cluster)

        assert "bug one" in story.description
        assert "bug two" in story.description

    def test_time_span_same_day(self) -> None:
        """Should format time span for same day."""
        generator = HeuristicStoryGenerator()
        ts = datetime(2025, 1, 15, 10, 0)
        cluster = Cluster(
            id="c1",
            commits=[
                CommitEmbedding(sha="a", embedding=[1.0], timestamp=ts),
                CommitEmbedding(sha="b", embedding=[1.0], timestamp=ts),
            ],
        )

        story = generator.generate_story(cluster)
        assert "Jan 15, 2025" in story.time_span

    def test_time_span_different_days(self) -> None:
        """Should format time span for different days."""
        generator = HeuristicStoryGenerator()
        cluster = Cluster(
            id="c1",
            commits=[
                CommitEmbedding(sha="a", embedding=[1.0], timestamp=datetime(2025, 1, 15)),
                CommitEmbedding(sha="b", embedding=[1.0], timestamp=datetime(2025, 1, 20)),
            ],
        )

        story = generator.generate_story(cluster)
        assert "Jan 15" in story.time_span
        assert "Jan 20" in story.time_span

    def test_metadata_includes_generator(self) -> None:
        """Should include generator info in metadata."""
        generator = HeuristicStoryGenerator()
        cluster = Cluster(
            id="c1",
            commits=[CommitEmbedding(sha="a", embedding=[1.0])],
        )

        story = generator.generate_story(cluster)
        assert story.metadata.get("generator") == "heuristic"


# =============================================================================
# AIStoryGenerator Tests
# =============================================================================


class TestAIStoryGenerator:
    """Tests for AIStoryGenerator."""

    def test_init_anthropic(self) -> None:
        """Should initialize with Anthropic provider."""
        generator = AIStoryGenerator(provider="anthropic", api_key="test-key")
        assert generator.provider == "anthropic"
        assert generator.api_key == "test-key"

    def test_init_openai(self) -> None:
        """Should initialize with OpenAI provider."""
        generator = AIStoryGenerator(provider="openai", api_key="test-key")
        assert generator.provider == "openai"

    def test_generate_with_anthropic(self) -> None:
        """Should generate story using Anthropic API."""
        mock_response = MagicMock()
        mock_response.content = [
            MagicMock(
                text='{"title": "Bug fixes in auth", "description": "Fixed authentication issues"}'
            )
        ]

        mock_client = MagicMock()
        mock_client.messages.create.return_value = mock_response

        generator = AIStoryGenerator(provider="anthropic", api_key="test-key")
        generator._client = mock_client

        cluster = Cluster(
            id="c1",
            commits=[
                CommitEmbedding(sha="abc", embedding=[1.0], message="fix: auth bug"),
            ],
            dominant_category="bug_fix",
            dominant_domains=["auth"],
            coherence=0.9,
        )

        story = generator.generate_story(cluster)

        assert story.title == "Bug fixes in auth"
        assert "authentication" in story.description.lower()

    def test_fallback_on_error(self) -> None:
        """Should fall back to heuristic on API error."""
        mock_client = MagicMock()
        mock_client.messages.create.side_effect = Exception("API error")

        generator = AIStoryGenerator(provider="anthropic", api_key="test-key")
        generator._client = mock_client

        cluster = Cluster(
            id="c1",
            commits=[CommitEmbedding(sha="abc", embedding=[1.0], message="fix: bug")],
            dominant_category="bug_fix",
        )

        story = generator.generate_story(cluster)

        # Should still get a story from heuristic fallback
        assert story is not None
        assert story.metadata.get("generator") == "heuristic"

    def test_fallback_on_parse_error(self) -> None:
        """Should fall back on JSON parse error."""
        mock_response = MagicMock()
        mock_response.content = [MagicMock(text="Invalid JSON")]

        mock_client = MagicMock()
        mock_client.messages.create.return_value = mock_response

        generator = AIStoryGenerator(provider="anthropic", api_key="test-key")
        generator._client = mock_client

        cluster = Cluster(
            id="c1",
            commits=[CommitEmbedding(sha="abc", embedding=[1.0])],
        )

        story = generator.generate_story(cluster)
        assert story is not None


# =============================================================================
# StoryBuilder Tests
# =============================================================================


class TestStoryBuilder:
    """Tests for StoryBuilder class."""

    def test_init_creates_tables(self) -> None:
        """Should create database tables on init."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            builder = StoryBuilder("test-project", db_path)

            # Check tables exist
            conn = builder._get_connection()
            cursor = conn.execute(
                "SELECT name FROM sqlite_master WHERE type='table'"
            )
            tables = [row[0] for row in cursor.fetchall()]

            assert "c4_commit_embeddings" in tables
            assert "c4_stories" in tables

            builder.close()

    def test_add_and_get_commit(self) -> None:
        """Should store and retrieve commits."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            builder = StoryBuilder("test-project", db_path)

            ts = datetime.now()
            builder.add_commit(
                sha="abc123",
                embedding=[1.0, 0.0, 0.0],
                message="fix: bug",
                category="bug_fix",
                domains=["auth"],
                timestamp=ts,
                metadata={"key": "value"},
            )

            commits = builder.get_commits()

            assert len(commits) == 1
            assert commits[0].sha == "abc123"
            assert commits[0].embedding == [1.0, 0.0, 0.0]
            assert commits[0].message == "fix: bug"
            assert commits[0].category == "bug_fix"
            assert commits[0].domains == ["auth"]

            builder.close()

    def test_get_commits_with_limit(self) -> None:
        """Should respect limit parameter."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            builder = StoryBuilder("test-project", db_path)

            for i in range(10):
                builder.add_commit(
                    sha=f"sha{i}",
                    embedding=[float(i), 0.0],
                )

            commits = builder.get_commits(limit=5)
            assert len(commits) == 5

            builder.close()

    def test_get_commits_since(self) -> None:
        """Should filter by timestamp."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            builder = StoryBuilder("test-project", db_path)

            now = datetime.now()
            yesterday = now - timedelta(days=1)
            last_week = now - timedelta(days=7)

            builder.add_commit(sha="old", embedding=[1.0], timestamp=last_week)
            builder.add_commit(sha="recent", embedding=[1.0], timestamp=yesterday)

            commits = builder.get_commits(since=now - timedelta(days=2))

            assert len(commits) == 1
            assert commits[0].sha == "recent"

            builder.close()

    def test_cluster_commits(self) -> None:
        """Should cluster commits."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            builder = StoryBuilder("test-project", db_path)

            # Add similar commits
            builder.add_commit(sha="a", embedding=[1.0, 0.0, 0.0])
            builder.add_commit(sha="b", embedding=[0.95, 0.05, 0.0])
            # Add different commit
            builder.add_commit(sha="c", embedding=[0.0, 0.0, 1.0])

            clusters = builder.cluster_commits(threshold=0.9)

            # Should have at least 2 clusters
            assert len(clusters) >= 1

            builder.close()

    def test_cluster_with_min_size(self) -> None:
        """Should filter clusters by minimum size."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            builder = StoryBuilder("test-project", db_path)

            # Add 3 commits that will form one cluster and 1 outlier
            builder.add_commit(sha="a", embedding=[1.0, 0.0])
            builder.add_commit(sha="b", embedding=[0.99, 0.01])
            builder.add_commit(sha="c", embedding=[0.98, 0.02])
            builder.add_commit(sha="d", embedding=[0.0, 1.0])  # Outlier

            clusters = builder.cluster_commits(threshold=0.9, min_cluster_size=2)

            # Only cluster with 3 commits should remain
            for cluster in clusters:
                assert cluster.size >= 2

            builder.close()

    def test_generate_story_saves_to_db(self) -> None:
        """Should save generated story to database."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            builder = StoryBuilder(
                "test-project",
                db_path,
                story_generator=HeuristicStoryGenerator(),
            )

            cluster = Cluster(
                id="cluster-001",
                commits=[
                    CommitEmbedding(sha="abc", embedding=[1.0], message="fix: bug"),
                ],
                dominant_category="bug_fix",
            )

            story = builder.generate_story(cluster)

            # Story should be saved
            stories = builder.get_stories()
            assert len(stories) == 1
            assert stories[0].id == story.id
            assert stories[0].title == story.title

            builder.close()

    def test_auto_detect_generator_heuristic(self) -> None:
        """Should use heuristic when no API keys."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"

            with patch.dict("os.environ", {}, clear=True):
                builder = StoryBuilder("test-project", db_path)
                assert isinstance(builder._story_generator, HeuristicStoryGenerator)

                builder.close()

    def test_close_connection(self) -> None:
        """Should close database connection."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            builder = StoryBuilder("test-project", db_path)

            assert builder._conn is not None
            builder.close()
            assert builder._conn is None


# =============================================================================
# Factory Function Tests
# =============================================================================


class TestGetStoryBuilder:
    """Tests for get_story_builder factory function."""

    def test_create_with_heuristic(self) -> None:
        """Should create builder with heuristic generator."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            builder = get_story_builder("test", db_path, generator="heuristic")

            assert isinstance(builder._story_generator, HeuristicStoryGenerator)
            builder.close()

    def test_create_with_anthropic(self) -> None:
        """Should create builder with Anthropic generator."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            builder = get_story_builder(
                "test",
                db_path,
                generator="anthropic",
                api_key="test-key",
            )

            assert isinstance(builder._story_generator, AIStoryGenerator)
            assert builder._story_generator.provider == "anthropic"
            builder.close()

    def test_create_with_openai(self) -> None:
        """Should create builder with OpenAI generator."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            builder = get_story_builder(
                "test",
                db_path,
                generator="openai",
                api_key="test-key",
            )

            assert isinstance(builder._story_generator, AIStoryGenerator)
            assert builder._story_generator.provider == "openai"
            builder.close()

    def test_auto_detect(self) -> None:
        """Should auto-detect generator when not specified."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"

            with patch.dict("os.environ", {}, clear=True):
                builder = get_story_builder("test", db_path)
                # Should fall back to heuristic
                assert isinstance(builder._story_generator, HeuristicStoryGenerator)
                builder.close()


# =============================================================================
# Integration Tests
# =============================================================================


class TestStoryBuilderIntegration:
    """Integration tests for complete workflow."""

    def test_full_workflow(self) -> None:
        """Should complete full clustering and story generation workflow."""
        with tempfile.TemporaryDirectory() as tmpdir:
            db_path = Path(tmpdir) / "test.db"
            builder = StoryBuilder(
                "test-project",
                db_path,
                story_generator=HeuristicStoryGenerator(),
            )

            # Add commits for a "feature" cluster
            for i in range(3):
                builder.add_commit(
                    sha=f"feat{i}",
                    embedding=[1.0, 0.0, 0.0 + i * 0.01],
                    message=f"feat: add feature {i}",
                    category="feature",
                    domains=["frontend"],
                    timestamp=datetime.now() - timedelta(hours=i),
                )

            # Add commits for a "bug fix" cluster
            for i in range(2):
                builder.add_commit(
                    sha=f"fix{i}",
                    embedding=[0.0, 1.0, 0.0 + i * 0.01],
                    message=f"fix: resolve bug {i}",
                    category="bug_fix",
                    domains=["backend"],
                    timestamp=datetime.now() - timedelta(hours=i),
                )

            # Cluster commits
            clusters = builder.cluster_commits(threshold=0.8)

            # Should have at least 2 clusters
            assert len(clusters) >= 2

            # Generate stories for each cluster
            stories = []
            for cluster in clusters:
                story = builder.generate_story(cluster)
                stories.append(story)

            # Verify stories
            assert len(stories) == len(clusters)
            for story in stories:
                assert story.title
                assert story.description
                assert len(story.commits) > 0

            # Verify stories are persisted
            saved_stories = builder.get_stories()
            assert len(saved_stories) == len(clusters)

            builder.close()
