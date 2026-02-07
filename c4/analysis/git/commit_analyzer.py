"""AI-powered commit intent extraction and analysis.

This module analyzes git commits to extract intent, categorize changes,
and identify affected domains using AI or heuristic fallback.

Usage:
    from c4.analysis.git.commit_analyzer import CommitAnalyzer, get_commit_analyzer

    analyzer = get_commit_analyzer()

    intent = analyzer.analyze_commit(
        sha="abc123",
        message="fix: resolve null pointer in auth module",
        diff="+def validate_user(): ...",
    )
    # intent.category = "bug_fix"
    # intent.affected_domains = ["auth"]
"""

import json
import logging
import os
import re
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Any, Protocol, runtime_checkable

logger = logging.getLogger(__name__)


# =============================================================================
# Data Models
# =============================================================================


@dataclass
class CommitIntent:
    """Represents the analyzed intent of a git commit.

    Attributes:
        sha: The commit SHA (short or full).
        intent: A natural language description of the commit intent.
        category: The category of change (e.g., "bug_fix", "feature", "refactor").
        affected_domains: List of domains affected (e.g., ["auth", "api", "db"]).
        key_changes: List of key changes identified in the commit.
        confidence: Confidence score from 0.0 to 1.0 (1.0 = high confidence).
        metadata: Additional analysis metadata.
    """

    sha: str
    intent: str
    category: str
    affected_domains: list[str] = field(default_factory=list)
    key_changes: list[str] = field(default_factory=list)
    confidence: float = 1.0
    metadata: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary representation."""
        return {
            "sha": self.sha,
            "intent": self.intent,
            "category": self.category,
            "affected_domains": self.affected_domains,
            "key_changes": self.key_changes,
            "confidence": self.confidence,
            "metadata": self.metadata,
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "CommitIntent":
        """Create CommitIntent from dictionary."""
        return cls(
            sha=data.get("sha", ""),
            intent=data.get("intent", ""),
            category=data.get("category", "unknown"),
            affected_domains=data.get("affected_domains", []),
            key_changes=data.get("key_changes", []),
            confidence=data.get("confidence", 1.0),
            metadata=data.get("metadata", {}),
        )


# =============================================================================
# Category and Domain Detection
# =============================================================================


# Commit categories with associated patterns
COMMIT_CATEGORIES: dict[str, list[str]] = {
    "bug_fix": [
        r"\bfix(?:ed|es|ing)?\b",
        r"\bbug(?:fix)?\b",
        r"\bpatch\b",
        r"\bresolve[sd]?\b",
        r"\bcorrect(?:ed|s|ion)?\b",
        r"\bhotfix\b",
        r"\brepair\b",
    ],
    "feature": [
        r"\bfeat(?:ure)?\b",
        r"\badd(?:ed|s|ing)?\b",
        r"\bimplement(?:ed|s|ation)?\b",
        r"\bintroduc(?:e|ed|ing)\b",
        r"\bnew\b",
        r"\bsupport\b",
        r"\benable[sd]?\b",
    ],
    "refactor": [
        r"\brefactor(?:ed|s|ing)?\b",
        r"\brestructur(?:e|ed|ing)\b",
        r"\breorganiz(?:e|ed|ing)\b",
        r"\bclean(?:ed|s|ing)?\s*up\b",
        r"\bsimplif(?:y|ied|ies)\b",
        r"\bmoderniz(?:e|ed|ing)\b",
        r"\bextract(?:ed|s|ing)?\b",
    ],
    "performance": [
        r"\bperf(?:ormance)?\b",
        r"\boptimiz(?:e|ed|ation|ing)\b",
        r"\bspeed\s*up\b",
        r"\bfaster\b",
        r"\bcach(?:e|ed|ing)\b",
        r"\bmemoiz(?:e|ation)\b",
        r"\bparallel\b",
    ],
    "documentation": [
        r"\bdoc(?:s|umentation)?\b",
        r"\breadme\b",
        r"\bcomment(?:ed|s|ing)?\b",
        r"\bjsdoc\b",
        r"\btypedoc\b",
        r"\bchangelog\b",
    ],
    "test": [
        r"\btest(?:s|ed|ing)?\b",
        r"\bspec(?:s)?\b",
        r"\bcoverage\b",
        r"\bunit\b",
        r"\be2e\b",
        r"\bintegration\b",
        r"\bmock(?:ed|s|ing)?\b",
    ],
    "style": [
        r"\bstyle\b",
        r"\bformat(?:ted|s|ting)?\b",
        r"\blint(?:ed|s|ing)?\b",
        r"\bprettier\b",
        r"\bwhitespace\b",
        r"\bindent(?:ation)?\b",
    ],
    "build": [
        r"\bbuild\b",
        r"\bci(?:/cd)?\b",
        r"\bpipeline\b",
        r"\bworkflow\b",
        r"\bpackage\b",
        r"\bdeps?\b",
        r"\bdependenc(?:y|ies)\b",
        r"\bupgrade[sd]?\b",
        r"\bbump(?:ed|s|ing)?\b",
    ],
    "security": [
        r"\bsecur(?:e|ity)\b",
        r"\bvuln(?:erability|erabilities)?\b",
        r"\bcve\b",
        r"\bxss\b",
        r"\bsql\s*injection\b",
        r"\bauth(?:entication|orization)?\b",
        r"\bpermission\b",
    ],
    "chore": [
        r"\bchore\b",
        r"\bmisc\b",
        r"\bclean(?:ed|s|ing)?\b",
        r"\bremov(?:e|ed|ing)?\b",
        r"\bdelete[sd]?\b",
        r"\bunused\b",
        r"\bdeprecated?\b",
    ],
    "revert": [
        r"\brevert(?:ed|s|ing)?\b",
        r"\brollback\b",
        r"\bundo\b",
    ],
}

# Domain patterns for detection
DOMAIN_PATTERNS: dict[str, list[str]] = {
    "auth": [
        r"\bauth(?:entication|orization)?\b",
        r"\blogin\b",
        r"\blogout\b",
        r"\bsession\b",
        r"\btoken\b",
        r"\bjwt\b",
        r"\boauth\b",
        r"\bpassword\b",
        r"\buser(?:s)?\b",
        r"\bpermission\b",
        r"\brole(?:s)?\b",
        r"auth[_/]",
        r"[_/]auth",
    ],
    "api": [
        r"\bapi\b",
        r"\bendpoint(?:s)?\b",
        r"\broute(?:s|r)?\b",
        r"\bcontroller(?:s)?\b",
        r"\bhandler(?:s)?\b",
        r"\brequest(?:s)?\b",
        r"\bresponse(?:s)?\b",
        r"\brest\b",
        r"\bgraphql\b",
        r"\bgrpc\b",
        r"api[_/]",
        r"[_/]api",
    ],
    "database": [
        r"\bdb\b",
        r"\bdatabase\b",
        r"\bsql\b",
        r"\bquery\b",
        r"\bmigration(?:s)?\b",
        r"\bmodel(?:s)?\b",
        r"\bschema\b",
        r"\borm\b",
        r"\bsqlalchemy\b",
        r"\bprisma\b",
        r"\bmongo(?:db)?\b",
        r"\bredis\b",
        r"models?[_/]",
        r"[_/]models?",
    ],
    "frontend": [
        r"\bfrontend\b",
        r"\bui\b",
        r"\bcomponent(?:s)?\b",
        r"\breact\b",
        r"\bvue\b",
        r"\bangular\b",
        r"\bsvelte\b",
        r"\bcss\b",
        r"\bstyle(?:s)?\b",
        r"\bhtml\b",
        r"\btemplate(?:s)?\b",
        r"components?[_/]",
        r"[_/]components?",
        r"\.tsx?$",
        r"\.jsx$",
        r"\.vue$",
    ],
    "backend": [
        r"\bbackend\b",
        r"\bserver\b",
        r"\bservice(?:s)?\b",
        r"\bworker(?:s)?\b",
        r"\bjob(?:s)?\b",
        r"\bqueue\b",
        r"\bcelery\b",
        r"\bsidekiq\b",
        r"services?[_/]",
        r"[_/]services?",
    ],
    "config": [
        r"\bconfig(?:uration)?\b",
        r"\bsetting(?:s)?\b",
        r"\benv(?:ironment)?\b",
        r"\b\.env\b",
        r"\byaml\b",
        r"\btoml\b",
        r"\bjson\b",
        r"config[_/]",
        r"[_/]config",
    ],
    "testing": [
        r"\btest(?:s|ing)?\b",
        r"\bspec(?:s)?\b",
        r"\bfixture(?:s)?\b",
        r"\bmock(?:s)?\b",
        r"\bfactory\b",
        r"tests?[_/]",
        r"[_/]tests?",
        r"__tests__",
        r"\.test\.",
        r"\.spec\.",
        r"_test\.",
    ],
    "infrastructure": [
        r"\binfra(?:structure)?\b",
        r"\bdevops\b",
        r"\bdocker(?:file)?\b",
        r"\bkubernetes\b",
        r"\bk8s\b",
        r"\bterraform\b",
        r"\bansible\b",
        r"\bci(?:/cd)?\b",
        r"\bpipeline\b",
        r"\bworkflow\b",
        r"\bgithub\s*action\b",
        r"\.github[_/]",
        r"infra[_/]",
        r"Dockerfile",
    ],
    "ml": [
        r"\bml\b",
        r"\bmachine\s*learning\b",
        r"\bmodel(?:s)?\b",
        r"\btraining\b",
        r"\binference\b",
        r"\bpytorch\b",
        r"\btensorflow\b",
        r"\bkeras\b",
        r"\bscikit\b",
        r"\bhuggingface\b",
        r"models?[_/]",
    ],
}


def detect_category(message: str, diff: str | None = None) -> tuple[str, float]:
    """Detect the commit category from message and diff.

    Args:
        message: The commit message.
        diff: Optional diff content.

    Returns:
        Tuple of (category, confidence).
    """
    text = message.lower()
    if diff:
        text += " " + diff.lower()

    # Check conventional commit prefix first
    conventional_match = re.match(r"^(\w+)(?:\([^)]+\))?[!:]", message)
    if conventional_match:
        prefix = conventional_match.group(1).lower()
        prefix_map = {
            "fix": "bug_fix",
            "feat": "feature",
            "refactor": "refactor",
            "perf": "performance",
            "docs": "documentation",
            "test": "test",
            "style": "style",
            "build": "build",
            "ci": "build",
            "chore": "chore",
            "revert": "revert",
            "security": "security",
        }
        if prefix in prefix_map:
            return prefix_map[prefix], 0.95

    # Pattern matching with scoring
    scores: dict[str, int] = {}
    for category, patterns in COMMIT_CATEGORIES.items():
        for pattern in patterns:
            if re.search(pattern, text, re.IGNORECASE):
                scores[category] = scores.get(category, 0) + 1

    if scores:
        best_category = max(scores, key=lambda k: scores[k])
        max_score = scores[best_category]
        confidence = min(0.5 + (max_score * 0.15), 0.9)
        return best_category, confidence

    return "unknown", 0.3


def detect_domains(message: str, diff: str | None = None) -> list[str]:
    """Detect affected domains from message and diff.

    Args:
        message: The commit message.
        diff: Optional diff content.

    Returns:
        List of detected domain names.
    """
    text = message.lower()
    if diff:
        text += " " + diff.lower()

    detected: set[str] = set()
    for domain, patterns in DOMAIN_PATTERNS.items():
        for pattern in patterns:
            if re.search(pattern, text, re.IGNORECASE):
                detected.add(domain)
                break

    return sorted(detected)


def extract_key_changes(message: str, diff: str | None = None) -> list[str]:
    """Extract key changes from commit message and diff.

    Args:
        message: The commit message.
        diff: Optional diff content.

    Returns:
        List of key change descriptions.
    """
    changes: list[str] = []

    # Extract from commit message body
    lines = message.strip().split("\n")
    for line in lines[1:]:  # Skip first line (subject)
        line = line.strip()
        # Skip empty lines and common markers
        if not line or line.startswith("#"):
            continue
        # Look for bullet points or numbered items
        if re.match(r"^[-*+]|\d+\.", line):
            # Clean up the line
            cleaned = re.sub(r"^[-*+]\s*|\d+\.\s*", "", line)
            if cleaned:
                changes.append(cleaned)

    # If no body items, try to extract from first line
    if not changes and lines:
        subject = lines[0]
        # Remove conventional commit prefix
        subject = re.sub(r"^\w+(?:\([^)]+\))?[!:]?\s*", "", subject)
        if subject:
            changes.append(subject)

    # Limit to 5 key changes
    return changes[:5]


# =============================================================================
# Analyzer Providers
# =============================================================================


@runtime_checkable
class CommitAnalyzerProvider(Protocol):
    """Protocol for commit analyzer providers."""

    @abstractmethod
    def analyze_commit(
        self,
        sha: str,
        message: str,
        diff: str | None = None,
        model: str | None = None,
    ) -> CommitIntent:
        """Analyze a commit and extract its intent.

        Args:
            sha: The commit SHA.
            message: The commit message.
            diff: Optional diff content.
            model: Optional model override.

        Returns:
            CommitIntent with analysis results.
        """
        ...


class BaseCommitAnalyzer(ABC):
    """Base class for commit analyzers."""

    @abstractmethod
    def analyze_commit(
        self,
        sha: str,
        message: str,
        diff: str | None = None,
        model: str | None = None,
    ) -> CommitIntent:
        """Analyze a commit and extract its intent."""
        ...


class HeuristicAnalyzer(BaseCommitAnalyzer):
    """Heuristic-based commit analyzer using pattern matching.

    Uses regex patterns and conventional commit detection without AI.
    Suitable for fast, offline analysis.
    """

    def analyze_commit(
        self,
        sha: str,
        message: str,
        diff: str | None = None,
        model: str | None = None,
    ) -> CommitIntent:
        """Analyze a commit using heuristics.

        Args:
            sha: The commit SHA.
            message: The commit message.
            diff: Optional diff content.
            model: Ignored in heuristic analyzer.

        Returns:
            CommitIntent with heuristic analysis results.
        """
        category, confidence = detect_category(message, diff)
        domains = detect_domains(message, diff)
        key_changes = extract_key_changes(message, diff)

        # Generate intent description
        intent = self._generate_intent(message, category)

        return CommitIntent(
            sha=sha,
            intent=intent,
            category=category,
            affected_domains=domains,
            key_changes=key_changes,
            confidence=confidence,
            metadata={"analyzer": "heuristic"},
        )

    def _generate_intent(self, message: str, category: str) -> str:
        """Generate a simple intent description.

        Args:
            message: The commit message.
            category: The detected category.

        Returns:
            Intent description string.
        """
        # Use first line of commit message
        subject = message.strip().split("\n")[0]

        # Remove conventional commit prefix for cleaner intent
        cleaned = re.sub(r"^\w+(?:\([^)]+\))?[!:]?\s*", "", subject)

        if not cleaned:
            cleaned = subject

        # Add category context if not already clear
        category_verbs = {
            "bug_fix": "Fix",
            "feature": "Add",
            "refactor": "Refactor",
            "performance": "Optimize",
            "documentation": "Document",
            "test": "Test",
            "style": "Format",
            "build": "Update build",
            "security": "Secure",
            "chore": "Maintain",
            "revert": "Revert",
        }

        verb = category_verbs.get(category, "")
        if verb and not cleaned.lower().startswith(verb.lower()):
            return f"{verb}: {cleaned}"

        return cleaned


class AnthropicCommitAnalyzer(BaseCommitAnalyzer):
    """Commit analyzer using Claude (Anthropic API).

    Uses AI for high-quality intent extraction and categorization.
    """

    DEFAULT_MODEL = "claude-3-haiku-20240307"

    def __init__(
        self,
        api_key: str | None = None,
        model: str | None = None,
    ) -> None:
        """Initialize the Anthropic analyzer.

        Args:
            api_key: Anthropic API key. Uses env var if not provided.
            model: Model to use for analysis.
        """
        self.api_key = api_key or os.environ.get("ANTHROPIC_API_KEY")
        self.model = model or self.DEFAULT_MODEL
        self._client = None
        self._fallback = HeuristicAnalyzer()

    def _get_client(self):
        """Get or create Anthropic client."""
        if self._client is None:
            try:
                import anthropic

                self._client = anthropic.Anthropic(api_key=self.api_key)
            except ImportError as e:
                raise ImportError(
                    "anthropic package required for AnthropicCommitAnalyzer. "
                    "Install with: pip install anthropic"
                ) from e
        return self._client

    def analyze_commit(
        self,
        sha: str,
        message: str,
        diff: str | None = None,
        model: str | None = None,
    ) -> CommitIntent:
        """Analyze a commit using Claude.

        Args:
            sha: The commit SHA.
            message: The commit message.
            diff: Optional diff content.
            model: Optional model override.

        Returns:
            CommitIntent with AI analysis results.
        """
        use_model = model or self.model

        # Truncate diff if too long
        truncated_diff = diff[:4000] if diff and len(diff) > 4000 else diff

        prompt = self._build_prompt(sha, message, truncated_diff)

        try:
            client = self._get_client()
            response = client.messages.create(
                model=use_model,
                max_tokens=500,
                messages=[{"role": "user", "content": prompt}],
            )

            result_text = response.content[0].text.strip()
            return self._parse_response(sha, message, result_text)
        except Exception as e:
            logger.warning(f"AI analysis failed, falling back to heuristics: {e}")
            return self._fallback.analyze_commit(sha, message, diff)

    def _build_prompt(self, sha: str, message: str, diff: str | None) -> str:
        """Build the analysis prompt."""
        diff_section = f"\n\nDiff:\n```\n{diff}\n```" if diff else ""

        return f"""Analyze this git commit and extract its intent.

Commit SHA: {sha}
Commit Message:
{message}{diff_section}

Respond with a JSON object containing:
- intent: A clear, natural language description of what this commit does (1-2 sentences)
- category: One of: bug_fix, feature, refactor, performance, documentation, test, style, build, security, chore, revert, unknown
- affected_domains: Array of affected areas like auth, api, database, frontend, backend, config, testing, infrastructure, ml
- key_changes: Array of 1-5 key changes (brief descriptions)
- confidence: Your confidence in this analysis (0.0 to 1.0)

Respond ONLY with the JSON object, no explanation."""

    def _parse_response(self, sha: str, message: str, response: str) -> CommitIntent:
        """Parse the AI response into CommitIntent."""
        try:
            # Try to extract JSON from response
            json_match = re.search(r"\{[\s\S]*\}", response)
            if json_match:
                data = json.loads(json_match.group())
                return CommitIntent(
                    sha=sha,
                    intent=data.get("intent", message.split("\n")[0]),
                    category=data.get("category", "unknown"),
                    affected_domains=data.get("affected_domains", []),
                    key_changes=data.get("key_changes", []),
                    confidence=data.get("confidence", 0.8),
                    metadata={"analyzer": "anthropic", "model": self.model},
                )
        except json.JSONDecodeError:
            pass

        # Fallback to heuristics if parsing fails
        logger.warning("Failed to parse AI response, falling back to heuristics")
        return self._fallback.analyze_commit(sha, message)


class OpenAICommitAnalyzer(BaseCommitAnalyzer):
    """Commit analyzer using OpenAI API."""

    DEFAULT_MODEL = "gpt-3.5-turbo"

    def __init__(
        self,
        api_key: str | None = None,
        model: str | None = None,
    ) -> None:
        """Initialize the OpenAI analyzer.

        Args:
            api_key: OpenAI API key. Uses env var if not provided.
            model: Model to use for analysis.
        """
        self.api_key = api_key or os.environ.get("OPENAI_API_KEY")
        self.model = model or self.DEFAULT_MODEL
        self._client = None
        self._fallback = HeuristicAnalyzer()

    def _get_client(self):
        """Get or create OpenAI client."""
        if self._client is None:
            try:
                import openai

                self._client = openai.OpenAI(api_key=self.api_key)
            except ImportError as e:
                raise ImportError(
                    "openai package required for OpenAICommitAnalyzer. "
                    "Install with: pip install openai"
                ) from e
        return self._client

    def analyze_commit(
        self,
        sha: str,
        message: str,
        diff: str | None = None,
        model: str | None = None,
    ) -> CommitIntent:
        """Analyze a commit using OpenAI.

        Args:
            sha: The commit SHA.
            message: The commit message.
            diff: Optional diff content.
            model: Optional model override.

        Returns:
            CommitIntent with AI analysis results.
        """
        use_model = model or self.model

        # Truncate diff if too long
        truncated_diff = diff[:4000] if diff and len(diff) > 4000 else diff

        system_prompt = """You are a git commit analyzer. Analyze commits and extract their intent.
Respond ONLY with a JSON object containing:
- intent: Clear description of what the commit does (1-2 sentences)
- category: One of: bug_fix, feature, refactor, performance, documentation, test, style, build, security, chore, revert, unknown
- affected_domains: Array of affected areas (auth, api, database, frontend, backend, config, testing, infrastructure, ml)
- key_changes: Array of 1-5 key changes
- confidence: Your confidence (0.0 to 1.0)"""

        user_content = f"Commit SHA: {sha}\nMessage:\n{message}"
        if truncated_diff:
            user_content += f"\n\nDiff:\n{truncated_diff}"

        try:
            client = self._get_client()
            response = client.chat.completions.create(
                model=use_model,
                max_tokens=500,
                messages=[
                    {"role": "system", "content": system_prompt},
                    {"role": "user", "content": user_content},
                ],
            )

            result_text = response.choices[0].message.content.strip()
            return self._parse_response(sha, message, result_text)
        except Exception as e:
            logger.warning(f"AI analysis failed, falling back to heuristics: {e}")
            return self._fallback.analyze_commit(sha, message, diff)

    def _parse_response(self, sha: str, message: str, response: str) -> CommitIntent:
        """Parse the AI response into CommitIntent."""
        try:
            json_match = re.search(r"\{[\s\S]*\}", response)
            if json_match:
                data = json.loads(json_match.group())
                return CommitIntent(
                    sha=sha,
                    intent=data.get("intent", message.split("\n")[0]),
                    category=data.get("category", "unknown"),
                    affected_domains=data.get("affected_domains", []),
                    key_changes=data.get("key_changes", []),
                    confidence=data.get("confidence", 0.8),
                    metadata={"analyzer": "openai", "model": self.model},
                )
        except json.JSONDecodeError:
            pass

        logger.warning("Failed to parse AI response, falling back to heuristics")
        return self._fallback.analyze_commit(sha, message)


# =============================================================================
# Main CommitAnalyzer Class
# =============================================================================


class CommitAnalyzer:
    """Main commit analyzer that delegates to a provider.

    This is the recommended way to use commit analysis.

    Example:
        >>> analyzer = CommitAnalyzer()
        >>> intent = analyzer.analyze_commit(
        ...     sha="abc123",
        ...     message="fix: resolve auth token expiry issue",
        ...     diff="+def refresh_token(): ..."
        ... )
        >>> intent.category
        'bug_fix'
        >>> intent.affected_domains
        ['auth']
    """

    def __init__(self, provider: BaseCommitAnalyzer | None = None) -> None:
        """Initialize the commit analyzer.

        Args:
            provider: Analysis provider. Auto-detected if not provided.
        """
        if provider is not None:
            self._provider = provider
        else:
            self._provider = self._auto_detect_provider()

    def _auto_detect_provider(self) -> BaseCommitAnalyzer:
        """Auto-detect the best available provider.

        Priority:
        1. Anthropic (if ANTHROPIC_API_KEY set)
        2. OpenAI (if OPENAI_API_KEY set)
        3. Heuristic (fallback)
        """
        if os.environ.get("ANTHROPIC_API_KEY"):
            return AnthropicCommitAnalyzer()
        elif os.environ.get("OPENAI_API_KEY"):
            return OpenAICommitAnalyzer()
        else:
            logger.info(
                "No API key found for AI analysis. Using heuristic analyzer. "
                "Set ANTHROPIC_API_KEY or OPENAI_API_KEY for AI-powered analysis."
            )
            return HeuristicAnalyzer()

    @property
    def provider(self) -> BaseCommitAnalyzer:
        """Get the current analysis provider."""
        return self._provider

    def analyze_commit(
        self,
        sha: str,
        message: str,
        diff: str | None = None,
        model: str | None = None,
    ) -> CommitIntent:
        """Analyze a commit and extract its intent.

        Args:
            sha: The commit SHA.
            message: The commit message.
            diff: Optional diff content.
            model: Optional model override.

        Returns:
            CommitIntent with analysis results.
        """
        return self._provider.analyze_commit(sha, message, diff, model)


# =============================================================================
# Factory Function
# =============================================================================


def get_commit_analyzer(
    provider: str | None = None, **kwargs
) -> CommitAnalyzer:
    """Factory function to create a CommitAnalyzer.

    Args:
        provider: Provider name ("anthropic", "openai", "heuristic") or None for auto.
        **kwargs: Additional arguments passed to the provider.

    Returns:
        CommitAnalyzer instance with the specified provider.

    Example:
        >>> analyzer = get_commit_analyzer("heuristic")  # For testing
        >>> analyzer = get_commit_analyzer()  # Auto-detect
    """
    if provider == "anthropic":
        return CommitAnalyzer(provider=AnthropicCommitAnalyzer(**kwargs))
    elif provider == "openai":
        return CommitAnalyzer(provider=OpenAICommitAnalyzer(**kwargs))
    elif provider == "heuristic":
        return CommitAnalyzer(provider=HeuristicAnalyzer())
    else:
        # Auto-detect
        return CommitAnalyzer()
