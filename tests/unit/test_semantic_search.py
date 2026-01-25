"""Tests for semantic search functionality."""

import tempfile
from pathlib import Path

import pytest

from c4.docs.semantic_search import (
    SemanticSearcher,
    SearchScope,
    SearchResult,
    SearchHit,
)


@pytest.fixture
def sample_project(tmp_path: Path) -> Path:
    """Create a sample project for testing."""
    # Create Python files
    (tmp_path / "main.py").write_text('''
"""Main application module."""

from auth import authenticate_user
from database import get_connection


def process_request(request):
    """Process an incoming HTTP request."""
    user = authenticate_user(request.token)
    conn = get_connection()
    return {"status": "ok", "user": user}


def handle_error(error):
    """Handle application errors."""
    return {"error": str(error)}
''')

    (tmp_path / "auth.py").write_text('''
"""Authentication module."""

from database import get_user_by_token


def authenticate_user(token):
    """Authenticate a user by their token."""
    user = get_user_by_token(token)
    if not user:
        raise ValueError("Invalid token")
    return user


def validate_password(password):
    """Validate password strength."""
    return len(password) >= 8
''')

    (tmp_path / "database.py").write_text('''
"""Database connection module."""

import sqlite3


def get_connection():
    """Get a database connection."""
    return sqlite3.connect(":memory:")


def get_user_by_token(token):
    """Fetch user by authentication token."""
    conn = get_connection()
    cursor = conn.execute("SELECT * FROM users WHERE token = ?", (token,))
    return cursor.fetchone()


def save_user(user):
    """Save user to database."""
    conn = get_connection()
    conn.execute("INSERT INTO users VALUES (?)", (user,))
    conn.commit()
''')

    return tmp_path


class TestSemanticSearcher:
    """Tests for SemanticSearcher."""

    def test_index_project(self, sample_project: Path):
        """Test indexing a project."""
        searcher = SemanticSearcher(project_root=sample_project)
        count = searcher.index()

        assert count > 0
        assert searcher._indexed is True

    def test_search_natural_language(self, sample_project: Path):
        """Test natural language search."""
        searcher = SemanticSearcher(project_root=sample_project)
        searcher.index()

        # Search for authentication
        result = searcher.search("how to authenticate users")

        assert isinstance(result, SearchResult)
        assert result.total_hits > 0
        assert "authenticate" in result.query.lower() or any(
            "auth" in h.content.lower() for h in result.hits
        )

    def test_search_with_synonyms(self, sample_project: Path):
        """Test synonym expansion in search."""
        searcher = SemanticSearcher(project_root=sample_project)
        searcher.index()

        # Search with abbreviation
        result = searcher.search("db connection", expand_synonyms=True)

        assert result.total_hits > 0
        # Should find database-related content
        contents = " ".join(h.content for h in result.hits)
        assert "database" in contents.lower() or "connection" in contents.lower()

    def test_search_by_scope(self, sample_project: Path):
        """Test scoped search."""
        searcher = SemanticSearcher(project_root=sample_project)
        searcher.index()

        # Search only symbols
        result = searcher.search("authenticate", scope=SearchScope.SYMBOLS)

        assert result.total_hits > 0
        # All results should have symbol names
        for hit in result.hits:
            assert hit.symbol_name is not None or hit.symbol_kind is not None

    def test_search_result_format(self, sample_project: Path):
        """Test search result formatting."""
        searcher = SemanticSearcher(project_root=sample_project)
        searcher.index()

        result = searcher.search("process request")

        # Test to_dict
        result_dict = result.to_dict()
        assert "query" in result_dict
        assert "total_hits" in result_dict
        assert "hits" in result_dict

        # Test to_markdown
        markdown = result.to_markdown()
        assert "# Search Results" in markdown
        assert result.query in markdown

    def test_find_related(self, sample_project: Path):
        """Test finding related symbols."""
        searcher = SemanticSearcher(project_root=sample_project)
        searcher.index()

        related = searcher.find_related("authenticate_user")

        # Should find related authentication or database symbols
        assert isinstance(related, list)

    def test_search_by_type(self, sample_project: Path):
        """Test searching by symbol type."""
        from c4.docs.analyzer import SymbolKind

        searcher = SemanticSearcher(project_root=sample_project)
        searcher.index()

        hits = searcher.search_by_type(kind=SymbolKind.FUNCTION)

        assert len(hits) > 0
        for hit in hits:
            assert hit.symbol_kind == SymbolKind.FUNCTION

    def test_tokenization(self, sample_project: Path):
        """Test text tokenization."""
        searcher = SemanticSearcher(project_root=sample_project)

        # Test snake_case splitting
        tokens = searcher._tokenize("get_user_by_id")
        assert "get" in tokens
        assert "user" in tokens
        assert "id" in tokens

        # Test regular words
        tokens = searcher._tokenize("authenticate user request")
        assert "authenticate" in tokens
        assert "user" in tokens
        assert "request" in tokens

    def test_query_expansion(self, sample_project: Path):
        """Test synonym expansion."""
        searcher = SemanticSearcher(project_root=sample_project)

        expanded = searcher._expand_query(["auth"])
        assert "authentication" in expanded or "login" in expanded

        expanded = searcher._expand_query(["db"])
        assert "database" in expanded


class TestSearchHit:
    """Tests for SearchHit."""

    def test_to_dict(self):
        """Test SearchHit serialization."""
        from c4.docs.analyzer import SymbolKind

        hit = SearchHit(
            file_path="test.py",
            line_number=10,
            score=0.85,
            content="def test_function():",
            symbol_name="test_function",
            symbol_kind=SymbolKind.FUNCTION,
        )

        result = hit.to_dict()

        assert result["file_path"] == "test.py"
        assert result["line_number"] == 10
        assert result["score"] == 0.85
        assert result["symbol_name"] == "test_function"
        assert result["symbol_kind"] == "function"
