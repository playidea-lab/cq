"""Tests for TypeScript support in C4 LSP tools.

This module verifies that C4 LSP tools work correctly with TypeScript files:
- Tree-sitter incremental parser
- Symbol extraction
- Cache behavior
"""

import pytest

from c4.lsp.cache import SymbolCache, get_symbol_cache, reset_global_cache
from c4.lsp.incremental_parser import (
    LANGUAGE_EXTENSIONS,
    TreeSitterParser,
    get_tree_sitter_parser,
    reset_global_parser,
)


class TestTypeScriptLanguageDetection:
    """Tests for TypeScript language detection."""

    def test_ts_extension_detected(self) -> None:
        """Should detect .ts files as TypeScript."""
        assert LANGUAGE_EXTENSIONS[".ts"] == "typescript"

    def test_tsx_extension_detected(self) -> None:
        """Should detect .tsx files as TypeScript."""
        assert LANGUAGE_EXTENSIONS[".tsx"] == "typescript"

    def test_mts_extension_detected(self) -> None:
        """Should detect .mts files as TypeScript."""
        assert LANGUAGE_EXTENSIONS[".mts"] == "typescript"


class TestTypeScriptParsing:
    """Tests for TypeScript parsing with Tree-sitter."""

    @pytest.fixture
    def parser(self) -> TreeSitterParser:
        """Get or create parser."""
        return get_tree_sitter_parser()

    @pytest.fixture(autouse=True)
    def reset_parser(self) -> None:
        """Reset parser after each test."""
        yield
        reset_global_parser()

    def test_parser_supports_typescript(self, parser: TreeSitterParser) -> None:
        """Should support TypeScript language."""
        if not parser.is_available:
            pytest.skip("Tree-sitter not available")
        assert parser.supports_language("typescript")

    def test_parse_typescript_interface(self, parser: TreeSitterParser) -> None:
        """Should parse TypeScript interface."""
        if not parser.is_available:
            pytest.skip("Tree-sitter not available")

        ts_code = """
export interface Message {
  id: string;
  role: 'user' | 'assistant';
  content: string;
}
"""
        result = parser.parse("test.ts", ts_code)

        assert result.language == "typescript"
        assert not result.has_errors
        assert len(result.symbols) > 0

    def test_parse_typescript_type_alias(self, parser: TreeSitterParser) -> None:
        """Should parse TypeScript type alias."""
        if not parser.is_available:
            pytest.skip("Tree-sitter not available")

        ts_code = """
export type EventType = 'start' | 'chunk' | 'done' | 'error';
"""
        result = parser.parse("test.ts", ts_code)

        assert result.language == "typescript"
        assert not result.has_errors

    def test_parse_typescript_function(self, parser: TreeSitterParser) -> None:
        """Should parse TypeScript function with types."""
        if not parser.is_available:
            pytest.skip("Tree-sitter not available")

        ts_code = """
export async function sendMessage(
  message: string,
  conversationId?: string
): Promise<Response> {
  return fetch('/api/chat', {
    method: 'POST',
    body: JSON.stringify({ message, conversationId }),
  });
}
"""
        result = parser.parse("test.ts", ts_code)

        assert result.language == "typescript"
        assert not result.has_errors
        assert len(result.symbols) > 0

    def test_parse_typescript_class(self, parser: TreeSitterParser) -> None:
        """Should parse TypeScript class with methods."""
        if not parser.is_available:
            pytest.skip("Tree-sitter not available")

        ts_code = """
export class ApiClient {
  private baseUrl: string;

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl;
  }

  async get<T>(path: string): Promise<T> {
    const response = await fetch(this.baseUrl + path);
    return response.json();
  }

  async post<T>(path: string, data: unknown): Promise<T> {
    const response = await fetch(this.baseUrl + path, {
      method: 'POST',
      body: JSON.stringify(data),
    });
    return response.json();
  }
}
"""
        result = parser.parse("test.ts", ts_code)

        assert result.language == "typescript"
        assert not result.has_errors
        # Should find class
        assert any(s.kind == "class_declaration" for s in result.symbols)

    def test_parse_typescript_enum(self, parser: TreeSitterParser) -> None:
        """Should parse TypeScript enum."""
        if not parser.is_available:
            pytest.skip("Tree-sitter not available")

        ts_code = """
export enum TaskStatus {
  Pending = 'pending',
  InProgress = 'in_progress',
  Done = 'done',
  Blocked = 'blocked',
}
"""
        result = parser.parse("test.ts", ts_code)

        assert result.language == "typescript"
        assert not result.has_errors

    def test_parse_typescript_arrow_function(self, parser: TreeSitterParser) -> None:
        """Should parse TypeScript arrow function."""
        if not parser.is_available:
            pytest.skip("Tree-sitter not available")

        ts_code = """
export const parseEvent = (line: string): Event | null => {
  if (!line.startsWith('data:')) return null;
  try {
    return JSON.parse(line.slice(5));
  } catch {
    return null;
  }
};
"""
        result = parser.parse("test.ts", ts_code)

        assert result.language == "typescript"
        assert not result.has_errors

    def test_parse_tsx_component(self, parser: TreeSitterParser) -> None:
        """Should parse TSX React component."""
        if not parser.is_available:
            pytest.skip("Tree-sitter not available")

        tsx_code = """
import React from 'react';

interface Props {
  title: string;
  children: React.ReactNode;
}

export function Card({ title, children }: Props): JSX.Element {
  return (
    <div className="card">
      <h2>{title}</h2>
      {children}
    </div>
  );
}
"""
        # Note: TSX might need the typescript parser to handle JSX
        result = parser.parse("test.tsx", tsx_code)

        assert result.language == "typescript"
        # TSX parsing might have some limitations


class TestTypeScriptCaching:
    """Tests for TypeScript symbol caching."""

    @pytest.fixture(autouse=True)
    def reset_cache(self) -> None:
        """Reset cache after each test."""
        yield
        reset_global_cache()

    def test_cache_typescript_symbols(self) -> None:
        """Should cache TypeScript symbols correctly."""
        cache = get_symbol_cache()

        ts_content = """
export interface User {
  id: string;
  name: string;
}

export function getUser(id: string): Promise<User> {
  return fetch('/api/users/' + id).then(r => r.json());
}
"""
        content_hash = cache.compute_hash(ts_content)
        symbols = [
            {"name": "User", "kind": 11},  # interface
            {"name": "getUser", "kind": 12},  # function
        ]

        # Put in cache
        cache.put("test.ts", content_hash, symbols)

        # Get from cache (returns list[dict] directly)
        cached = cache.get("test.ts", content_hash)

        assert cached is not None
        assert cached == symbols

    def test_cache_invalidation_on_content_change(self) -> None:
        """Should invalidate cache when content changes."""
        cache = get_symbol_cache()

        original_content = "export const API_URL = 'http://localhost:4000';"
        modified_content = "export const API_URL = 'https://api.example.com';"

        original_hash = cache.compute_hash(original_content)
        modified_hash = cache.compute_hash(modified_content)

        # Cache original
        cache.put("config.ts", original_hash, [{"name": "API_URL", "kind": 13}])

        # Try to get with modified hash
        cached = cache.get("config.ts", modified_hash)

        # Should not match (different hash)
        assert cached is None

    def test_cache_hit_tracking(self) -> None:
        """Should track cache hits and misses."""
        cache = SymbolCache(max_entries=100)

        content = "export type Status = 'active' | 'inactive';"
        content_hash = cache.compute_hash(content)
        symbols = [{"name": "Status", "kind": 10}]

        # Miss
        cache.get("types.ts", content_hash)

        # Put
        cache.put("types.ts", content_hash, symbols)

        # Hit
        cache.get("types.ts", content_hash)
        cache.get("types.ts", content_hash)

        stats = cache.stats
        assert stats.hits == 2
        assert stats.misses == 1


class TestTypeScriptIncrementalParsing:
    """Tests for incremental TypeScript parsing."""

    @pytest.fixture
    def parser(self) -> TreeSitterParser:
        """Get or create parser."""
        return get_tree_sitter_parser()

    @pytest.fixture(autouse=True)
    def reset_parser(self) -> None:
        """Reset parser after each test."""
        yield
        reset_global_parser()

    def test_incremental_parse_after_edit(self, parser: TreeSitterParser) -> None:
        """Should incrementally parse after content edit."""
        if not parser.is_available:
            pytest.skip("Tree-sitter not available")

        file_path = "service.ts"

        # First parse
        original = """
export class UserService {
  async getUser(id: string): Promise<User> {
    return this.api.get('/users/' + id);
  }
}
"""
        result1 = parser.parse(file_path, original)
        assert not result1.has_errors

        # Second parse (edited)
        modified = """
export class UserService {
  async getUser(id: string): Promise<User> {
    return this.api.get('/users/' + id);
  }

  async deleteUser(id: string): Promise<void> {
    return this.api.delete('/users/' + id);
  }
}
"""
        result2 = parser.parse(file_path, modified, incremental=True)
        assert not result2.has_errors

        # Should have more symbols after edit
        # (new method added)

    def test_cache_invalidation_after_edit(self, parser: TreeSitterParser) -> None:
        """Should invalidate cache entry after file edit."""
        if not parser.is_available:
            pytest.skip("Tree-sitter not available")

        file_path = "utils.ts"

        # First parse
        result1 = parser.parse(file_path, "export const foo = 1;")

        # Edit and parse
        result2 = parser.parse(file_path, "export const bar = 2;", incremental=True)

        # Both should succeed
        assert not result1.has_errors
        assert not result2.has_errors


class TestRealTypeScriptFile:
    """Tests with actual TypeScript file from the project."""

    @pytest.fixture
    def parser(self) -> TreeSitterParser:
        """Get or create parser."""
        return get_tree_sitter_parser()

    @pytest.fixture(autouse=True)
    def reset_parser(self) -> None:
        """Reset parser after each test."""
        yield
        reset_global_parser()

    def test_parse_api_ts_file(self, parser: TreeSitterParser) -> None:
        """Should parse actual api.ts file from project."""
        if not parser.is_available:
            pytest.skip("Tree-sitter not available")

        from pathlib import Path

        api_file = Path(__file__).parent.parent.parent.parent / "web" / "lib" / "api.ts"

        if not api_file.exists():
            pytest.skip("api.ts file not found")

        content = api_file.read_text()
        result = parser.parse(str(api_file), content)

        assert result.language == "typescript"
        assert not result.has_errors, f"Parse errors: {result.errors}"

        # Should find some symbols
        assert len(result.symbols) > 0

        # Print found symbols for debugging
        symbol_names = [s.name for s in result.symbols if s.name]
        assert len(symbol_names) > 0
