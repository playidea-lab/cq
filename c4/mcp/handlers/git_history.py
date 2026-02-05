"""Git history analysis and search handlers for MCP.

This module provides MCP tools for git commit history analysis:
- c4_analyze_history: Analyzes commits, clusters them, generates stories
- c4_search_commits: Semantic search for finding related commits

Usage:
    c4_analyze_history(since="2025-01-01", until="2025-01-31", branch="main")
    c4_search_commits(query="authentication", filters={"author": "John"})

Returns:
    c4_analyze_history:
        {"stories": [{"id", "title", "commits", "dependencies"}], "graph": {...}}

    c4_search_commits:
        {"commits": [{"sha", "message", "intent", "story_id", "score"}]}
"""

from __future__ import annotations

import logging
import subprocess
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from c4.memory.story_builder import Cluster

from ..registry import register_tool

logger = logging.getLogger(__name__)


# =============================================================================
# Data Models
# =============================================================================


@dataclass
class GitCommit:
    """Represents a git commit for analysis.

    Attributes:
        sha: The commit SHA (short or full).
        message: The commit message.
        author: The commit author.
        date: The commit timestamp.
        files_changed: List of files changed in the commit.
    """

    sha: str
    message: str
    author: str
    date: datetime
    files_changed: list[str] = field(default_factory=list)


# =============================================================================
# Git Log Parsing
# =============================================================================


def parse_git_log(log_output: str) -> list[GitCommit]:
    """Parse git log output into GitCommit objects.

    Expected format (from git log with custom format):
        sha
        message
        author
        date

        (blank line separator)

    Args:
        log_output: Raw git log output string.

    Returns:
        List of GitCommit objects.
    """
    if not log_output.strip():
        return []

    commits: list[GitCommit] = []
    # Split by double newline (commit separator)
    entries = log_output.strip().split("\n\n")

    for entry in entries:
        if not entry.strip():
            continue

        lines = entry.strip().split("\n")
        if len(lines) < 4:
            continue

        sha = lines[0].strip()
        message = lines[1].strip()
        author = lines[-2].strip()  # Author is second to last
        date_str = lines[-1].strip()  # Date is last

        # Handle multiline messages
        if len(lines) > 4:
            # Message spans multiple lines
            message_lines = lines[1:-2]
            message = "\n".join(line.strip() for line in message_lines)

        # Parse date
        try:
            # Handle various date formats
            date_str_clean = date_str.split(" +")[0].split(" -")[0]
            for fmt in [
                "%Y-%m-%d %H:%M:%S",
                "%a %b %d %H:%M:%S %Y",
                "%Y-%m-%dT%H:%M:%S",
            ]:
                try:
                    date = datetime.strptime(date_str_clean, fmt)
                    break
                except ValueError:
                    continue
            else:
                date = datetime.now()
        except (ValueError, IndexError):
            date = datetime.now()

        commits.append(
            GitCommit(
                sha=sha,
                message=message,
                author=author,
                date=date,
            )
        )

    return commits


# =============================================================================
# Git History Analyzer
# =============================================================================


class GitHistoryAnalyzer:
    """Analyzes git commit history and generates stories.

    Uses commit analysis and clustering to group related commits
    and generate narrative stories about the work done.

    Attributes:
        project_root: Path to the git repository root.

    Example:
        >>> analyzer = GitHistoryAnalyzer(Path("/project"))
        >>> commits = analyzer.get_commits(since="2025-01-01")
        >>> result = analyzer.analyze_commits(commits)
        >>> print(result["stories"])
    """

    def __init__(self, project_root: Path) -> None:
        """Initialize the analyzer.

        Args:
            project_root: Path to the git repository root.
        """
        self.project_root = project_root

    def get_commits(
        self,
        since: str,
        until: str | None = None,
        branch: str = "HEAD",
    ) -> list[GitCommit]:
        """Get commits from git log.

        Args:
            since: Start date (ISO format, e.g., "2025-01-01").
            until: End date (optional, ISO format).
            branch: Branch to analyze (default: HEAD).

        Returns:
            List of GitCommit objects.
        """
        # Build git log command
        cmd = [
            "git",
            "log",
            f"--since={since}",
            "--format=%H%n%s%n%an%n%ai%n",  # Hash, subject, author, date
            branch,
        ]

        if until:
            cmd.insert(3, f"--until={until}")

        try:
            result = subprocess.run(
                cmd,
                cwd=self.project_root,
                capture_output=True,
                text=True,
                check=True,
            )
            return parse_git_log(result.stdout)
        except subprocess.CalledProcessError as e:
            logger.warning(f"Git log failed: {e}")
            return []
        except Exception as e:
            logger.error(f"Unexpected error getting commits: {e}")
            return []

    def analyze_commits(self, commits: list[GitCommit]) -> dict[str, Any]:
        """Analyze commits and generate stories.

        Uses commit clustering and story generation to create
        a structured analysis of the commit history.

        Args:
            commits: List of commits to analyze.

        Returns:
            Dictionary with:
                - stories: List of story objects
                - graph: Dependency graph with nodes and edges
        """
        if not commits:
            return {
                "stories": [],
                "graph": {"nodes": [], "edges": []},
            }

        # Import here to avoid circular imports
        from c4.memory.commit_analyzer import get_commit_analyzer
        from c4.memory.story_builder import (
            CommitEmbedding,
            HeuristicStoryGenerator,
            cluster_agglomerative,
        )

        # Analyze each commit to get intent and category
        analyzer = get_commit_analyzer(provider="heuristic")
        commit_embeddings: list[CommitEmbedding] = []

        for commit in commits:
            intent = analyzer.analyze_commit(
                sha=commit.sha[:7],
                message=commit.message,
            )

            # Create a simple embedding based on category and domains
            # This is a heuristic embedding for clustering similar commits
            embedding = self._create_heuristic_embedding(intent.category, intent.affected_domains)

            commit_embeddings.append(
                CommitEmbedding(
                    sha=commit.sha,
                    embedding=embedding,
                    message=commit.message,
                    category=intent.category,
                    domains=intent.affected_domains,
                    timestamp=commit.date,
                    metadata={
                        "author": commit.author,
                        "intent": intent.intent,
                        "key_changes": intent.key_changes,
                    },
                )
            )

        # Cluster commits
        clusters = cluster_agglomerative(commit_embeddings, threshold=0.6)

        # Generate stories
        generator = HeuristicStoryGenerator()
        stories: list[dict[str, Any]] = []
        nodes: list[dict[str, Any]] = []
        edges: list[dict[str, Any]] = []

        for cluster in clusters:
            story = generator.generate_story(cluster)

            # Infer dependencies from commit order and domains
            dependencies = self._infer_dependencies(cluster, clusters)

            story_dict = {
                "id": story.id,
                "title": story.title,
                "description": story.description,
                "commits": story.commits,
                "category": story.category,
                "domains": story.domains,
                "time_span": story.time_span,
                "dependencies": dependencies,
            }
            stories.append(story_dict)

            # Add to graph
            nodes.append({
                "id": story.id,
                "label": story.title,
                "category": story.category,
                "size": len(story.commits),
            })

            # Add edges for dependencies
            for dep_id in dependencies:
                edges.append({
                    "source": dep_id,
                    "target": story.id,
                    "type": "depends_on",
                })

        return {
            "stories": stories,
            "graph": {
                "nodes": nodes,
                "edges": edges,
            },
        }

    def _create_heuristic_embedding(
        self,
        category: str,
        domains: list[str],
    ) -> list[float]:
        """Create a simple embedding vector for clustering.

        Uses one-hot encoding for categories and domains.

        Args:
            category: Commit category.
            domains: Affected domains.

        Returns:
            Embedding vector.
        """
        # Category embedding (11 categories)
        categories = [
            "bug_fix", "feature", "refactor", "performance",
            "documentation", "test", "style", "build",
            "security", "chore", "revert",
        ]
        category_vec = [1.0 if c == category else 0.0 for c in categories]

        # Domain embedding (10 domains)
        domain_list = [
            "auth", "api", "database", "frontend", "backend",
            "config", "testing", "infrastructure", "ml", "unknown",
        ]
        domain_vec = [1.0 if d in domains else 0.0 for d in domain_list]

        return category_vec + domain_vec

    def _infer_dependencies(
        self,
        cluster: "Cluster",
        all_clusters: list["Cluster"],
    ) -> list[str]:
        """Infer dependencies between clusters.

        Uses chronological ordering and domain overlap to infer
        dependencies between clusters.

        Args:
            cluster: Current cluster.
            all_clusters: All clusters for comparison.

        Returns:
            List of dependent cluster IDs.
        """
        dependencies: list[str] = []

        # Get earliest timestamp in current cluster
        current_earliest = min(
            (c.timestamp for c in cluster.commits if c.timestamp),
            default=datetime.now(),
        )

        for other_cluster in all_clusters:
            if other_cluster.id == cluster.id:
                continue

            # Check if other cluster is earlier
            other_latest = max(
                (c.timestamp for c in other_cluster.commits if c.timestamp),
                default=datetime.now(),
            )

            if other_latest < current_earliest:
                # Check for domain overlap
                current_domains = set(cluster.dominant_domains)
                other_domains = set(other_cluster.dominant_domains)

                if current_domains & other_domains:
                    dependencies.append(other_cluster.id)

        return dependencies


# =============================================================================
# MCP Handler
# =============================================================================


@register_tool("c4_analyze_history")
def handle_analyze_history(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Handle c4_analyze_history MCP tool call.

    Analyzes git commit history, clusters related commits,
    and generates narrative stories.

    Args:
        daemon: C4Daemon instance.
        arguments: Tool arguments:
            - since (required): Start date (ISO format)
            - until (optional): End date (ISO format)
            - branch (optional): Branch to analyze (default: HEAD)

    Returns:
        Dictionary with:
            - stories: List of story objects with id, title, commits, dependencies
            - graph: Dependency graph with nodes and edges
    """
    since = arguments.get("since")
    if not since:
        return {"error": "Missing required parameter: since"}

    until = arguments.get("until")
    branch = arguments.get("branch", "HEAD")

    # Get project root from daemon
    project_root = Path(daemon.project_root)

    analyzer = GitHistoryAnalyzer(project_root=project_root)

    # Get commits
    commits = analyzer.get_commits(
        since=since,
        until=until,
        branch=branch,
    )

    # Analyze and generate stories
    result = analyzer.analyze_commits(commits)

    return result


# =============================================================================
# Commit Search
# =============================================================================


class CommitSearcher:
    """Searches commits using semantic matching.

    Uses text similarity and commit analysis to find commits
    relevant to a given query.

    Attributes:
        project_root: Path to the git repository root.

    Example:
        >>> searcher = CommitSearcher(Path("/project"))
        >>> results = searcher.search("authentication bug")
        >>> for r in results:
        ...     print(f"{r['sha']}: {r['score']:.2f}")
    """

    def __init__(self, project_root: Path) -> None:
        """Initialize the searcher.

        Args:
            project_root: Path to the git repository root.
        """
        self.project_root = project_root

    def search(
        self,
        query: str,
        filters: dict[str, Any] | None = None,
        limit: int = 20,
    ) -> list[dict[str, Any]]:
        """Search commits matching the query.

        Args:
            query: Semantic search query.
            filters: Optional filters:
                - author: Filter by author name
                - since: Start date (ISO format)
                - path: File/directory path
                - story_id: Story ID to find related commits
            limit: Maximum number of results (default: 20).

        Returns:
            List of commit results with:
                - sha: Commit SHA
                - message: Commit message
                - intent: Analyzed intent
                - story_id: Associated story (if any)
                - score: Relevance score (0-1)
        """
        if not query or not query.strip():
            return []

        filters = filters or {}

        # Get commits from git
        commits = self._get_commits_for_search(filters)

        if not commits:
            return []

        # Score and rank commits
        results = self._score_commits(commits, query, filters)

        # Sort by score descending
        results.sort(key=lambda x: x["score"], reverse=True)

        # Apply limit
        return results[:limit]

    def _get_commits_for_search(
        self,
        filters: dict[str, Any],
    ) -> list[GitCommit]:
        """Get commits from git with filters applied.

        Args:
            filters: Search filters.

        Returns:
            List of GitCommit objects.
        """
        # Build git log command
        cmd = [
            "git",
            "log",
            "--format=%H%n%s%n%an%n%ai%n",  # Hash, subject, author, date
            "-n",
            "100",  # Limit for performance
        ]

        # Apply filters
        if filters.get("since"):
            cmd.insert(2, f"--since={filters['since']}")

        if filters.get("author"):
            cmd.insert(2, f"--author={filters['author']}")

        if filters.get("path"):
            cmd.append("--")
            cmd.append(filters["path"])

        try:
            result = subprocess.run(
                cmd,
                cwd=self.project_root,
                capture_output=True,
                text=True,
                check=True,
            )
            return parse_git_log(result.stdout)
        except subprocess.CalledProcessError as e:
            logger.warning(f"Git log failed: {e}")
            return []
        except Exception as e:
            logger.error(f"Unexpected error getting commits: {e}")
            return []

    def _score_commits(
        self,
        commits: list[GitCommit],
        query: str,
        filters: dict[str, Any],
    ) -> list[dict[str, Any]]:
        """Score commits based on query relevance.

        Uses text similarity and commit analysis for scoring.

        Args:
            commits: Commits to score.
            query: Search query.
            filters: Search filters.

        Returns:
            List of scored commit results.
        """
        from c4.memory.commit_analyzer import get_commit_analyzer

        analyzer = get_commit_analyzer(provider="heuristic")
        query_lower = query.lower()
        query_words = set(query_lower.split())

        results: list[dict[str, Any]] = []

        for commit in commits:
            # Analyze commit intent
            intent = analyzer.analyze_commit(
                sha=commit.sha[:7],
                message=commit.message,
            )

            # Calculate relevance score
            score = self._calculate_score(
                query_lower,
                query_words,
                commit,
                intent,
            )

            # Skip very low scores
            if score < 0.1:
                continue

            results.append({
                "sha": commit.sha,
                "message": commit.message,
                "intent": intent.intent,
                "category": intent.category,
                "domains": intent.affected_domains,
                "author": commit.author,
                "date": commit.date.isoformat(),
                "story_id": filters.get("story_id"),  # Will be None if not filtered
                "score": score,
            })

        return results

    def _calculate_score(
        self,
        query_lower: str,
        query_words: set[str],
        commit: GitCommit,
        intent: Any,
    ) -> float:
        """Calculate relevance score for a commit.

        Uses multiple signals for scoring:
        - Direct text match in message
        - Word overlap
        - Domain/category match
        - Intent similarity

        Args:
            query_lower: Lowercase query string.
            query_words: Set of query words.
            commit: The commit to score.
            intent: Analyzed commit intent.

        Returns:
            Score between 0 and 1.
        """
        score = 0.0
        message_lower = commit.message.lower()

        # Direct substring match (high signal)
        if query_lower in message_lower:
            score += 0.5

        # Word overlap
        message_words = set(message_lower.split())
        overlap = len(query_words & message_words)
        if overlap > 0:
            word_score = overlap / len(query_words)
            score += 0.3 * word_score

        # Domain match
        for domain in intent.affected_domains:
            if domain.lower() in query_lower:
                score += 0.1
                break

        # Category match
        if intent.category.lower() in query_lower:
            score += 0.05

        # Intent text match
        if intent.intent:
            intent_lower = intent.intent.lower()
            if any(word in intent_lower for word in query_words):
                score += 0.05

        # Normalize to 0-1
        return min(score, 1.0)


@register_tool("c4_search_commits")
def handle_search_commits(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Handle c4_search_commits MCP tool call.

    Searches git commits using semantic matching.

    Args:
        daemon: C4Daemon instance.
        arguments: Tool arguments:
            - query (required): Semantic search query
            - filters (optional): {author, since, path, story_id}

    Returns:
        Dictionary with:
            - commits: List of matching commits with sha, message, intent, score
    """
    query = arguments.get("query")
    if not query:
        return {"error": "Missing required parameter: query"}

    filters = arguments.get("filters", {})

    # Get project root from daemon
    project_root = Path(daemon.project_root)

    searcher = CommitSearcher(project_root=project_root)

    # Search commits
    commits = searcher.search(query=query, filters=filters)

    return {"commits": commits}
