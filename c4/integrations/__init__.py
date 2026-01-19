"""C4 Integrations - External service integrations for C4."""

from .github import (
    CommitResult,
    GitHubAutomation,
    GitHubClient,
    GitHubResult,
    PRResult,
)

__all__ = [
    "CommitResult",
    "GitHubAutomation",
    "GitHubClient",
    "GitHubResult",
    "PRResult",
]
