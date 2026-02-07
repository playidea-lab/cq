"""Semantic commit clustering and story generation.

This module clusters related commits based on vector similarity
and generates narrative stories from commit groups.

Usage:
    from c4.analysis.git.story_builder import StoryBuilder, get_story_builder

    builder = get_story_builder(project_id, db_path)

    # Add commits with embeddings
    builder.add_commit("abc123", embedding, metadata)

    # Cluster commits
    clusters = builder.cluster_commits(threshold=0.7)

    # Generate story from cluster
    story = builder.generate_story(cluster)
"""

import json
import logging
import math
import os
import sqlite3
import uuid
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Any, Protocol, runtime_checkable

logger = logging.getLogger(__name__)


# =============================================================================
# Data Models
# =============================================================================


@dataclass
class CommitEmbedding:
    """A commit with its embedding vector.

    Attributes:
        sha: The commit SHA.
        embedding: The embedding vector.
        message: The commit message.
        category: The commit category (bug_fix, feature, etc.).
        domains: Affected domains (auth, api, etc.).
        timestamp: When the commit was made.
        metadata: Additional metadata.
    """

    sha: str
    embedding: list[float]
    message: str = ""
    category: str = ""
    domains: list[str] = field(default_factory=list)
    timestamp: datetime | None = None
    metadata: dict[str, Any] = field(default_factory=dict)


@dataclass
class Cluster:
    """A cluster of related commits.

    Attributes:
        id: Unique cluster identifier.
        commits: List of commits in this cluster.
        centroid: Centroid embedding of the cluster.
        coherence: Cluster coherence score (0-1, higher = more coherent).
        dominant_category: Most common category in the cluster.
        dominant_domains: Most common domains in the cluster.
        created_at: When the cluster was created.
    """

    id: str
    commits: list[CommitEmbedding]
    centroid: list[float] | None = None
    coherence: float = 0.0
    dominant_category: str = ""
    dominant_domains: list[str] = field(default_factory=list)
    created_at: datetime = field(default_factory=datetime.now)

    def __len__(self) -> int:
        return len(self.commits)

    @property
    def size(self) -> int:
        """Number of commits in the cluster."""
        return len(self.commits)


@dataclass
class Story:
    """A narrative story generated from a commit cluster.

    Attributes:
        id: Unique story identifier.
        title: Short descriptive title.
        description: Detailed narrative description.
        commits: List of commit SHAs in chronological order.
        cluster_id: ID of the source cluster.
        category: Primary category (feature, bug_fix, etc.).
        domains: Affected domains.
        time_span: Time span description (e.g., "Jan 15-20, 2025").
        created_at: When the story was generated.
        metadata: Additional metadata.
    """

    id: str
    title: str
    description: str
    commits: list[str]
    cluster_id: str = ""
    category: str = ""
    domains: list[str] = field(default_factory=list)
    time_span: str = ""
    created_at: datetime = field(default_factory=datetime.now)
    metadata: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "id": self.id,
            "title": self.title,
            "description": self.description,
            "commits": self.commits,
            "cluster_id": self.cluster_id,
            "category": self.category,
            "domains": self.domains,
            "time_span": self.time_span,
            "created_at": self.created_at.isoformat(),
            "metadata": self.metadata,
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "Story":
        """Create from dictionary."""
        created_at = data.get("created_at")
        if isinstance(created_at, str):
            created_at = datetime.fromisoformat(created_at)
        elif created_at is None:
            created_at = datetime.now()

        return cls(
            id=data["id"],
            title=data["title"],
            description=data["description"],
            commits=data.get("commits", []),
            cluster_id=data.get("cluster_id", ""),
            category=data.get("category", ""),
            domains=data.get("domains", []),
            time_span=data.get("time_span", ""),
            created_at=created_at,
            metadata=data.get("metadata", {}),
        )


# =============================================================================
# Clustering Algorithms
# =============================================================================


def cosine_similarity(a: list[float], b: list[float]) -> float:
    """Calculate cosine similarity between two vectors.

    Args:
        a: First vector.
        b: Second vector.

    Returns:
        Cosine similarity between -1 and 1 (1 = identical).
    """
    if len(a) != len(b):
        raise ValueError("Vectors must have same dimension")

    dot_product = sum(x * y for x, y in zip(a, b))
    norm_a = math.sqrt(sum(x * x for x in a))
    norm_b = math.sqrt(sum(x * x for x in b))

    if norm_a == 0 or norm_b == 0:
        return 0.0

    return dot_product / (norm_a * norm_b)


def euclidean_distance(a: list[float], b: list[float]) -> float:
    """Calculate Euclidean distance between two vectors.

    Args:
        a: First vector.
        b: Second vector.

    Returns:
        Euclidean distance (0 = identical).
    """
    if len(a) != len(b):
        raise ValueError("Vectors must have same dimension")

    return math.sqrt(sum((x - y) ** 2 for x, y in zip(a, b)))


def compute_centroid(embeddings: list[list[float]]) -> list[float]:
    """Compute the centroid of a list of embeddings.

    Args:
        embeddings: List of embedding vectors.

    Returns:
        Centroid vector (average of all embeddings).
    """
    if not embeddings:
        return []

    dimension = len(embeddings[0])
    centroid = [0.0] * dimension

    for emb in embeddings:
        for i, val in enumerate(emb):
            centroid[i] += val

    n = len(embeddings)
    return [c / n for c in centroid]


def cluster_agglomerative(
    commits: list[CommitEmbedding],
    threshold: float = 0.7,
) -> list[Cluster]:
    """Cluster commits using agglomerative clustering with single linkage.

    Uses cosine similarity with a threshold to group related commits.

    Args:
        commits: List of commits with embeddings.
        threshold: Similarity threshold (0-1). Commits with similarity
            above this threshold are grouped together.

    Returns:
        List of Cluster objects.
    """
    if not commits:
        return []

    # Initialize: each commit is its own cluster
    clusters: list[list[CommitEmbedding]] = [[c] for c in commits]

    def cluster_similarity(c1: list[CommitEmbedding], c2: list[CommitEmbedding]) -> float:
        """Single linkage: max similarity between any pair."""
        max_sim = 0.0
        for commit1 in c1:
            for commit2 in c2:
                sim = cosine_similarity(commit1.embedding, commit2.embedding)
                max_sim = max(max_sim, sim)
        return max_sim

    # Agglomerative merging
    changed = True
    while changed:
        changed = False
        best_i, best_j = -1, -1
        best_sim = threshold

        # Find most similar pair
        for i in range(len(clusters)):
            for j in range(i + 1, len(clusters)):
                sim = cluster_similarity(clusters[i], clusters[j])
                if sim > best_sim:
                    best_sim = sim
                    best_i, best_j = i, j

        # Merge if above threshold
        if best_i >= 0 and best_j >= 0:
            clusters[best_i].extend(clusters[best_j])
            clusters.pop(best_j)
            changed = True

    # Convert to Cluster objects
    result: list[Cluster] = []
    for commit_list in clusters:
        cluster_id = f"cluster-{uuid.uuid4().hex[:8]}"
        embeddings = [c.embedding for c in commit_list]
        centroid = compute_centroid(embeddings)

        # Calculate coherence (average pairwise similarity)
        coherence = calculate_cluster_coherence(commit_list)

        # Find dominant category and domains
        categories = [c.category for c in commit_list if c.category]
        domains_flat = [d for c in commit_list for d in c.domains]

        dominant_category = max(set(categories), key=categories.count) if categories else ""
        dominant_domains = list(set(domains_flat))[:5]  # Top 5 unique domains

        result.append(
            Cluster(
                id=cluster_id,
                commits=commit_list,
                centroid=centroid,
                coherence=coherence,
                dominant_category=dominant_category,
                dominant_domains=dominant_domains,
            )
        )

    return result


def calculate_cluster_coherence(commits: list[CommitEmbedding]) -> float:
    """Calculate cluster coherence as average pairwise similarity.

    Args:
        commits: Commits in the cluster.

    Returns:
        Coherence score (0-1, higher = more coherent).
    """
    if len(commits) < 2:
        return 1.0  # Single-element cluster is perfectly coherent

    total_sim = 0.0
    count = 0

    for i in range(len(commits)):
        for j in range(i + 1, len(commits)):
            total_sim += cosine_similarity(commits[i].embedding, commits[j].embedding)
            count += 1

    return total_sim / count if count > 0 else 0.0


# =============================================================================
# Story Generation
# =============================================================================


@runtime_checkable
class StoryGeneratorProvider(Protocol):
    """Protocol for story generators."""

    @abstractmethod
    def generate_story(
        self,
        cluster: Cluster,
        model: str | None = None,
    ) -> Story:
        """Generate a story from a cluster of commits."""
        ...


class BaseStoryGenerator(ABC):
    """Base class for story generators."""

    @abstractmethod
    def generate_story(
        self,
        cluster: Cluster,
        model: str | None = None,
    ) -> Story:
        """Generate a story from a cluster of commits."""
        ...


class HeuristicStoryGenerator(BaseStoryGenerator):
    """Heuristic-based story generator using templates.

    Generates story titles and descriptions based on commit metadata
    without requiring AI API calls.
    """

    # Title templates by category
    TITLE_TEMPLATES = {
        "bug_fix": [
            "Bug fixes in {domains}",
            "Resolved issues in {domains}",
            "Fixed {count} bugs",
        ],
        "feature": [
            "New features for {domains}",
            "Added {count} new capabilities",
            "{domains} enhancements",
        ],
        "refactor": [
            "Refactored {domains}",
            "Code improvements in {domains}",
            "Cleaned up {count} modules",
        ],
        "documentation": [
            "Documentation updates",
            "Updated docs for {domains}",
            "Improved documentation",
        ],
        "test": [
            "Added tests for {domains}",
            "Improved test coverage",
            "Testing improvements",
        ],
        "performance": [
            "Performance improvements in {domains}",
            "Optimized {domains}",
            "Speed improvements",
        ],
        "default": [
            "Updates to {domains}",
            "{count} commits in {domains}",
            "Various improvements",
        ],
    }

    def generate_story(
        self,
        cluster: Cluster,
        model: str | None = None,
    ) -> Story:
        """Generate a story from a cluster using templates.

        Args:
            cluster: The cluster to generate a story from.
            model: Ignored in heuristic generator.

        Returns:
            Generated Story object.
        """
        # Generate title
        title = self._generate_title(cluster)

        # Generate description
        description = self._generate_description(cluster)

        # Get commit SHAs
        commit_shas = [c.sha for c in cluster.commits]

        # Calculate time span
        time_span = self._get_time_span(cluster)

        story_id = f"story-{uuid.uuid4().hex[:8]}"

        return Story(
            id=story_id,
            title=title,
            description=description,
            commits=commit_shas,
            cluster_id=cluster.id,
            category=cluster.dominant_category,
            domains=cluster.dominant_domains,
            time_span=time_span,
            metadata={"generator": "heuristic"},
        )

    def _generate_title(self, cluster: Cluster) -> str:
        """Generate a title for the cluster."""
        category = cluster.dominant_category or "default"
        templates = self.TITLE_TEMPLATES.get(category, self.TITLE_TEMPLATES["default"])

        # Format variables
        domains_str = ", ".join(cluster.dominant_domains[:3]) if cluster.dominant_domains else "codebase"
        count = len(cluster.commits)

        # Use first template that works
        for template in templates:
            try:
                return template.format(domains=domains_str, count=count)
            except KeyError:
                continue

        return f"{count} commits"

    def _generate_description(self, cluster: Cluster) -> str:
        """Generate a description summarizing the commits."""
        lines = []

        # Summary line
        count = len(cluster.commits)
        category = cluster.dominant_category or "changes"
        domains = cluster.dominant_domains

        if domains:
            lines.append(f"A series of {count} {category} commits affecting {', '.join(domains)}.")
        else:
            lines.append(f"A series of {count} {category} commits.")

        # Add commit summaries (first 5)
        lines.append("\nKey changes:")
        for commit in cluster.commits[:5]:
            # Use first line of commit message
            msg_line = commit.message.split("\n")[0] if commit.message else commit.sha[:7]
            lines.append(f"- {msg_line}")

        if len(cluster.commits) > 5:
            lines.append(f"- ... and {len(cluster.commits) - 5} more commits")

        # Add coherence info
        lines.append(f"\nCluster coherence: {cluster.coherence:.2f}")

        return "\n".join(lines)

    def _get_time_span(self, cluster: Cluster) -> str:
        """Get a human-readable time span for the cluster."""
        timestamps = [c.timestamp for c in cluster.commits if c.timestamp]

        if not timestamps:
            return ""

        min_time = min(timestamps)
        max_time = max(timestamps)

        if min_time.date() == max_time.date():
            return min_time.strftime("%b %d, %Y")
        elif min_time.year == max_time.year:
            return f"{min_time.strftime('%b %d')} - {max_time.strftime('%b %d, %Y')}"
        else:
            return f"{min_time.strftime('%b %d, %Y')} - {max_time.strftime('%b %d, %Y')}"


class AIStoryGenerator(BaseStoryGenerator):
    """AI-powered story generator using LLM APIs.

    Uses Claude or OpenAI to generate narrative descriptions.
    """

    def __init__(
        self,
        provider: str = "anthropic",
        api_key: str | None = None,
        model: str | None = None,
    ) -> None:
        """Initialize AI story generator.

        Args:
            provider: "anthropic" or "openai".
            api_key: API key for the provider.
            model: Model to use.
        """
        self.provider = provider
        self.api_key = api_key
        self.model = model
        self._client = None
        self._fallback = HeuristicStoryGenerator()

    def _get_anthropic_client(self):
        """Get or create Anthropic client."""
        if self._client is None:
            try:
                import anthropic
                key = self.api_key or os.environ.get("ANTHROPIC_API_KEY")
                self._client = anthropic.Anthropic(api_key=key)
            except ImportError as e:
                raise ImportError("anthropic package required") from e
        return self._client

    def _get_openai_client(self):
        """Get or create OpenAI client."""
        if self._client is None:
            try:
                import openai
                key = self.api_key or os.environ.get("OPENAI_API_KEY")
                self._client = openai.OpenAI(api_key=key)
            except ImportError as e:
                raise ImportError("openai package required") from e
        return self._client

    def generate_story(
        self,
        cluster: Cluster,
        model: str | None = None,
    ) -> Story:
        """Generate a story using AI.

        Args:
            cluster: The cluster to generate a story from.
            model: Optional model override.

        Returns:
            Generated Story object.
        """
        try:
            if self.provider == "anthropic":
                return self._generate_with_anthropic(cluster, model)
            elif self.provider == "openai":
                return self._generate_with_openai(cluster, model)
            else:
                return self._fallback.generate_story(cluster)
        except Exception as e:
            logger.warning(f"AI story generation failed: {e}")
            return self._fallback.generate_story(cluster)

    def _build_prompt(self, cluster: Cluster) -> str:
        """Build the prompt for story generation."""
        commits_info = []
        for c in cluster.commits[:10]:  # Limit to 10 commits
            info = f"- [{c.sha[:7]}] {c.message.split(chr(10))[0]}"
            if c.category:
                info += f" (category: {c.category})"
            commits_info.append(info)

        domains_str = ", ".join(cluster.dominant_domains) if cluster.dominant_domains else "general"

        return f"""Generate a brief narrative story for this group of related commits.

Commits ({len(cluster.commits)} total):
{chr(10).join(commits_info)}

Dominant category: {cluster.dominant_category or "mixed"}
Affected domains: {domains_str}
Cluster coherence: {cluster.coherence:.2f}

Generate:
1. A short title (under 60 characters) summarizing the work
2. A 2-3 sentence description explaining what was accomplished

Respond in JSON format:
{{"title": "...", "description": "..."}}"""

    def _generate_with_anthropic(
        self,
        cluster: Cluster,
        model: str | None = None,
    ) -> Story:
        """Generate story using Anthropic API."""
        client = self._get_anthropic_client()
        use_model = model or self.model or "claude-3-haiku-20240307"

        response = client.messages.create(
            model=use_model,
            max_tokens=300,
            messages=[{"role": "user", "content": self._build_prompt(cluster)}],
        )

        return self._parse_response(cluster, response.content[0].text)

    def _generate_with_openai(
        self,
        cluster: Cluster,
        model: str | None = None,
    ) -> Story:
        """Generate story using OpenAI API."""
        client = self._get_openai_client()
        use_model = model or self.model or "gpt-3.5-turbo"

        response = client.chat.completions.create(
            model=use_model,
            max_tokens=300,
            messages=[
                {
                    "role": "system",
                    "content": "You are a technical writer. Generate brief, accurate commit stories.",
                },
                {"role": "user", "content": self._build_prompt(cluster)},
            ],
        )

        return self._parse_response(cluster, response.choices[0].message.content)

    def _parse_response(self, cluster: Cluster, response: str) -> Story:
        """Parse AI response into Story object."""
        import re

        story_id = f"story-{uuid.uuid4().hex[:8]}"
        commit_shas = [c.sha for c in cluster.commits]

        try:
            # Extract JSON from response
            json_match = re.search(r"\{[\s\S]*\}", response)
            if json_match:
                data = json.loads(json_match.group())
                return Story(
                    id=story_id,
                    title=data.get("title", "Untitled"),
                    description=data.get("description", ""),
                    commits=commit_shas,
                    cluster_id=cluster.id,
                    category=cluster.dominant_category,
                    domains=cluster.dominant_domains,
                    metadata={"generator": f"ai-{self.provider}"},
                )
        except json.JSONDecodeError:
            pass

        # Fallback to heuristic if parsing fails
        logger.warning("Failed to parse AI response, using fallback")
        return self._fallback.generate_story(cluster)


# =============================================================================
# StoryBuilder Main Class
# =============================================================================


class StoryBuilder:
    """Main class for commit clustering and story generation.

    Manages commit embeddings, performs clustering, and generates
    narrative stories from commit groups.

    Attributes:
        project_id: The project ID.
        db_path: Path to the SQLite database.
        story_generator: Generator for creating stories.

    Example:
        >>> builder = StoryBuilder("my-project", "memory.db")
        >>> builder.add_commit("abc123", embedding, "fix: resolve bug")
        >>> clusters = builder.cluster_commits(threshold=0.7)
        >>> story = builder.generate_story(clusters[0])
    """

    def __init__(
        self,
        project_id: str,
        db_path: str | Path,
        story_generator: BaseStoryGenerator | None = None,
    ) -> None:
        """Initialize the story builder.

        Args:
            project_id: The project ID.
            db_path: Path to the SQLite database.
            story_generator: Story generator to use. Auto-detected if None.
        """
        self.project_id = project_id
        self.db_path = Path(db_path)
        self._conn = None
        self._ensure_tables()

        if story_generator is not None:
            self._story_generator = story_generator
        else:
            self._story_generator = self._auto_detect_generator()

    def _auto_detect_generator(self) -> BaseStoryGenerator:
        """Auto-detect the best story generator."""
        if os.environ.get("ANTHROPIC_API_KEY"):
            return AIStoryGenerator(provider="anthropic")
        elif os.environ.get("OPENAI_API_KEY"):
            return AIStoryGenerator(provider="openai")
        else:
            return HeuristicStoryGenerator()

    def _get_connection(self) -> sqlite3.Connection:
        """Get or create database connection."""
        if self._conn is None:
            self.db_path.parent.mkdir(parents=True, exist_ok=True)
            self._conn = sqlite3.connect(str(self.db_path))
            self._conn.row_factory = sqlite3.Row
        return self._conn

    def _ensure_tables(self) -> None:
        """Create required tables."""
        conn = self._get_connection()
        conn.execute("""
            CREATE TABLE IF NOT EXISTS c4_commit_embeddings (
                sha TEXT PRIMARY KEY,
                project_id TEXT NOT NULL,
                embedding TEXT NOT NULL,
                message TEXT,
                category TEXT,
                domains TEXT,
                timestamp TEXT,
                metadata TEXT,
                created_at TEXT DEFAULT CURRENT_TIMESTAMP
            )
        """)
        conn.execute("""
            CREATE TABLE IF NOT EXISTS c4_stories (
                id TEXT PRIMARY KEY,
                project_id TEXT NOT NULL,
                title TEXT NOT NULL,
                description TEXT,
                commits TEXT,
                cluster_id TEXT,
                category TEXT,
                domains TEXT,
                time_span TEXT,
                metadata TEXT,
                created_at TEXT DEFAULT CURRENT_TIMESTAMP
            )
        """)
        conn.commit()

    def add_commit(
        self,
        sha: str,
        embedding: list[float],
        message: str = "",
        category: str = "",
        domains: list[str] | None = None,
        timestamp: datetime | None = None,
        metadata: dict[str, Any] | None = None,
    ) -> None:
        """Add a commit with its embedding.

        Args:
            sha: The commit SHA.
            embedding: The embedding vector.
            message: The commit message.
            category: The commit category.
            domains: Affected domains.
            timestamp: When the commit was made.
            metadata: Additional metadata.
        """
        conn = self._get_connection()
        conn.execute(
            """
            INSERT OR REPLACE INTO c4_commit_embeddings
            (sha, project_id, embedding, message, category, domains, timestamp, metadata)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (
                sha,
                self.project_id,
                json.dumps(embedding),
                message,
                category,
                json.dumps(domains or []),
                timestamp.isoformat() if timestamp else None,
                json.dumps(metadata or {}),
            ),
        )
        conn.commit()

    def get_commits(
        self,
        limit: int | None = None,
        since: datetime | None = None,
    ) -> list[CommitEmbedding]:
        """Retrieve commits with embeddings.

        Args:
            limit: Maximum number of commits to retrieve.
            since: Only commits after this timestamp.

        Returns:
            List of CommitEmbedding objects.
        """
        conn = self._get_connection()
        query = "SELECT * FROM c4_commit_embeddings WHERE project_id = ?"
        params: list[Any] = [self.project_id]

        if since:
            query += " AND timestamp >= ?"
            params.append(since.isoformat())

        query += " ORDER BY timestamp DESC"

        if limit:
            query += " LIMIT ?"
            params.append(limit)

        cursor = conn.execute(query, params)

        commits = []
        for row in cursor.fetchall():
            ts = row["timestamp"]
            if ts:
                ts = datetime.fromisoformat(ts)

            commits.append(
                CommitEmbedding(
                    sha=row["sha"],
                    embedding=json.loads(row["embedding"]),
                    message=row["message"] or "",
                    category=row["category"] or "",
                    domains=json.loads(row["domains"] or "[]"),
                    timestamp=ts,
                    metadata=json.loads(row["metadata"] or "{}"),
                )
            )

        return commits

    def cluster_commits(
        self,
        threshold: float = 0.7,
        min_cluster_size: int = 1,
        commits: list[CommitEmbedding] | None = None,
    ) -> list[Cluster]:
        """Cluster commits based on embedding similarity.

        Args:
            threshold: Similarity threshold (0-1).
            min_cluster_size: Minimum commits per cluster.
            commits: Commits to cluster. If None, retrieves from DB.

        Returns:
            List of Cluster objects.
        """
        if commits is None:
            commits = self.get_commits()

        if not commits:
            return []

        clusters = cluster_agglomerative(commits, threshold=threshold)

        # Filter by minimum size
        if min_cluster_size > 1:
            clusters = [c for c in clusters if c.size >= min_cluster_size]

        # Sort by size (largest first)
        clusters.sort(key=lambda c: c.size, reverse=True)

        return clusters

    def generate_story(self, cluster: Cluster, model: str | None = None) -> Story:
        """Generate a story from a cluster.

        Args:
            cluster: The cluster to generate a story from.
            model: Optional model override.

        Returns:
            Generated Story object.
        """
        story = self._story_generator.generate_story(cluster, model)

        # Save story to database
        self._save_story(story)

        return story

    def _save_story(self, story: Story) -> None:
        """Save a story to the database."""
        conn = self._get_connection()
        conn.execute(
            """
            INSERT OR REPLACE INTO c4_stories
            (id, project_id, title, description, commits, cluster_id, category, domains, time_span, metadata, created_at)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (
                story.id,
                self.project_id,
                story.title,
                story.description,
                json.dumps(story.commits),
                story.cluster_id,
                story.category,
                json.dumps(story.domains),
                story.time_span,
                json.dumps(story.metadata),
                story.created_at.isoformat(),
            ),
        )
        conn.commit()

    def get_stories(self, limit: int = 100) -> list[Story]:
        """Retrieve saved stories.

        Args:
            limit: Maximum number of stories.

        Returns:
            List of Story objects.
        """
        conn = self._get_connection()
        cursor = conn.execute(
            """
            SELECT * FROM c4_stories
            WHERE project_id = ?
            ORDER BY created_at DESC
            LIMIT ?
            """,
            (self.project_id, limit),
        )

        stories = []
        for row in cursor.fetchall():
            stories.append(
                Story(
                    id=row["id"],
                    title=row["title"],
                    description=row["description"] or "",
                    commits=json.loads(row["commits"] or "[]"),
                    cluster_id=row["cluster_id"] or "",
                    category=row["category"] or "",
                    domains=json.loads(row["domains"] or "[]"),
                    time_span=row["time_span"] or "",
                    created_at=datetime.fromisoformat(row["created_at"]),
                    metadata=json.loads(row["metadata"] or "{}"),
                )
            )

        return stories

    def close(self) -> None:
        """Close database connection."""
        if self._conn is not None:
            self._conn.close()
            self._conn = None


# =============================================================================
# Factory Function
# =============================================================================


def get_story_builder(
    project_id: str,
    db_path: str | Path,
    generator: str | None = None,
    **kwargs,
) -> StoryBuilder:
    """Factory function to create a StoryBuilder.

    Args:
        project_id: The project ID.
        db_path: Path to the SQLite database.
        generator: Generator type ("heuristic", "anthropic", "openai") or auto.
        **kwargs: Additional arguments for the generator.

    Returns:
        Configured StoryBuilder instance.
    """
    if generator == "heuristic":
        story_generator = HeuristicStoryGenerator()
    elif generator == "anthropic":
        story_generator = AIStoryGenerator(provider="anthropic", **kwargs)
    elif generator == "openai":
        story_generator = AIStoryGenerator(provider="openai", **kwargs)
    else:
        story_generator = None  # Auto-detect

    return StoryBuilder(
        project_id=project_id,
        db_path=db_path,
        story_generator=story_generator,
    )
