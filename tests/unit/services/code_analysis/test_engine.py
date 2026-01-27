"""Tests for Code Analysis Engine."""

from pathlib import Path

import pytest

from c4.services.code_analysis import (
    CodeAnalysisEngine,
    CodebaseIndex,
    SymbolInfo,
    SymbolKind,
    SymbolLocation,
    SymbolTable,
    analyze_codebase,
)


class TestCodeAnalysisEngine:
    """Test CodeAnalysisEngine class."""

    @pytest.fixture
    def engine(self) -> CodeAnalysisEngine:
        """Create an engine instance."""
        return CodeAnalysisEngine()

    @pytest.fixture
    def sample_project(self, tmp_path: Path) -> Path:
        """Create a sample project structure."""
        # Python files
        py_dir = tmp_path / "src"
        py_dir.mkdir()

        (py_dir / "__init__.py").write_text("")

        (py_dir / "models.py").write_text('''
"""Data models."""

class User:
    """A user model."""

    def __init__(self, name: str, email: str):
        self.name = name
        self.email = email

    def greet(self) -> str:
        """Return a greeting."""
        return f"Hello, {self.name}!"

class Admin(User):
    """An admin user."""

    def has_permission(self, action: str) -> bool:
        return True
''')

        (py_dir / "service.py").write_text('''
"""User service."""

from .models import User

class UserService:
    """Service for user operations."""

    def get_user(self, user_id: int) -> User:
        """Get user by ID."""
        pass

    def create_user(self, name: str, email: str) -> User:
        """Create a new user."""
        return User(name, email)
''')

        # TypeScript files
        ts_dir = tmp_path / "frontend"
        ts_dir.mkdir()

        (ts_dir / "types.ts").write_text('''
export interface UserDTO {
    id: number;
    name: string;
    email: string;
}

export type UserRole = "admin" | "user" | "guest";
''')

        (ts_dir / "api.ts").write_text('''
import { UserDTO } from "./types";

export async function fetchUser(id: number): Promise<UserDTO> {
    const response = await fetch(`/api/users/${id}`);
    return response.json();
}

export class ApiClient {
    private baseUrl: string;

    constructor(baseUrl: string) {
        this.baseUrl = baseUrl;
    }

    async get<T>(path: string): Promise<T> {
        const response = await fetch(`${this.baseUrl}${path}`);
        return response.json();
    }
}
''')

        # Ignore directory
        cache_dir = tmp_path / "__pycache__"
        cache_dir.mkdir()
        (cache_dir / "module.pyc").write_bytes(b"")

        return tmp_path

    def test_analyze_codebase(self, engine: CodeAnalysisEngine, sample_project: Path):
        """Test analyzing a complete codebase."""
        result = engine.analyze_codebase(sample_project)

        assert result.success is True
        assert result.index is not None
        assert result.duration_ms is not None

        # Check stats
        stats = result.index.stats
        assert stats["files"] >= 4  # At least 4 source files
        assert stats["class"] >= 4  # User, Admin, UserService, ApiClient

    def test_analyze_file_python(self, engine: CodeAnalysisEngine, sample_project: Path):
        """Test analyzing a single Python file."""
        table = engine.analyze_file(sample_project / "src" / "models.py")

        assert table.language == "python"
        assert "User" in table.symbols
        assert "Admin" in table.symbols
        assert "User/greet" in table.symbols

    def test_analyze_file_typescript(
        self, engine: CodeAnalysisEngine, sample_project: Path
    ):
        """Test analyzing a single TypeScript file."""
        table = engine.analyze_file(sample_project / "frontend" / "types.ts")

        assert table.language == "typescript"
        assert "UserDTO" in table.symbols
        assert table.symbols["UserDTO"].kind == SymbolKind.INTERFACE
        assert "UserRole" in table.symbols
        assert table.symbols["UserRole"].kind == SymbolKind.TYPE_ALIAS

    def test_analyze_source(self, engine: CodeAnalysisEngine):
        """Test analyzing source code strings."""
        py_source = "def hello(): pass"
        ts_source = "function hello() {}"

        py_table = engine.analyze_source(py_source, "python", "test.py")
        assert "hello" in py_table.symbols
        assert py_table.symbols["hello"].kind == SymbolKind.FUNCTION

        ts_table = engine.analyze_source(ts_source, "typescript", "test.ts")
        assert "hello" in ts_table.symbols

    def test_find_symbol(self, engine: CodeAnalysisEngine, sample_project: Path):
        """Test finding symbols across codebase."""
        result = engine.analyze_codebase(sample_project)
        index = result.index

        # Find by exact name
        users = engine.find_symbol(index, "User")
        assert len(users) >= 1
        assert any(s.name == "User" for _, s in users)

        # Find by path pattern
        methods = engine.find_symbol(index, "User/greet")
        assert len(methods) >= 1

    def test_find_by_kind(self, engine: CodeAnalysisEngine, sample_project: Path):
        """Test finding symbols by kind."""
        result = engine.analyze_codebase(sample_project)
        index = result.index

        classes = engine.find_by_kind(index, SymbolKind.CLASS)
        assert len(classes) >= 4

        interfaces = engine.find_by_kind(index, SymbolKind.INTERFACE)
        assert len(interfaces) >= 1

    def test_find_references(self, engine: CodeAnalysisEngine, sample_project: Path):
        """Test finding references."""
        result = engine.analyze_codebase(sample_project)
        index = result.index

        refs = engine.find_references(index, "User", include_definitions=True)
        # Should find definition + imports
        assert len(refs) >= 1
        assert any(r.reference_kind == "definition" for r in refs)

    def test_get_symbol_tree(self, engine: CodeAnalysisEngine, sample_project: Path):
        """Test getting symbol tree."""
        result = engine.analyze_codebase(sample_project)
        index = result.index

        # Find a Python file
        py_file = str(sample_project / "src" / "models.py")
        tree = engine.get_symbol_tree(index, py_file)

        assert tree["file"] == py_file
        assert tree["language"] == "python"
        assert len(tree["children"]) >= 2  # User, Admin

    def test_ignore_patterns(self, sample_project: Path):
        """Test that ignore patterns work."""
        engine = CodeAnalysisEngine(ignore_patterns={"frontend"})
        result = engine.analyze_codebase(sample_project)

        # Should not include TypeScript files
        for file_path in result.index.tables.keys():
            assert "frontend" not in file_path

    def test_file_patterns(self, engine: CodeAnalysisEngine, sample_project: Path):
        """Test filtering by file patterns."""
        result = engine.analyze_codebase(sample_project, file_patterns=["*.py"])

        # Should only include Python files
        for table in result.index.tables.values():
            assert table.language == "python"

    def test_progress_callback(self, sample_project: Path):
        """Test progress callback."""
        progress_calls = []

        def on_progress(file_path: str, current: int, total: int):
            progress_calls.append((file_path, current, total))

        engine = CodeAnalysisEngine(progress_callback=on_progress)
        engine.analyze_codebase(sample_project)

        assert len(progress_calls) >= 4
        # Verify progress increases
        for i, (_, current, total) in enumerate(progress_calls):
            assert current == i + 1
            assert total > 0

    def test_nonexistent_path(self, engine: CodeAnalysisEngine):
        """Test handling of non-existent path."""
        result = engine.analyze_codebase("/nonexistent/path")

        assert result.success is False
        assert len(result.errors) == 1
        assert "does not exist" in result.errors[0]

    def test_unsupported_file(self, engine: CodeAnalysisEngine, tmp_path: Path):
        """Test handling of unsupported file types."""
        unsupported = tmp_path / "file.xyz"
        unsupported.write_text("content")

        table = engine.analyze_file(unsupported)

        assert table.language == "unknown"
        assert len(table.errors) > 0

    def test_convenience_function(self, sample_project: Path):
        """Test the analyze_codebase convenience function."""
        result = analyze_codebase(
            sample_project,
            file_patterns=["*.py"],
            ignore_patterns={"__pycache__"},
        )

        assert result.success is True
        assert result.index is not None


class TestCodebaseIndex:
    """Test CodebaseIndex class."""

    @pytest.fixture
    def sample_index(self) -> CodebaseIndex:
        """Create a sample index."""
        index = CodebaseIndex(root_path="/project")

        # Add Python table
        py_table = SymbolTable(file_path="/project/src/main.py", language="python")
        py_table.add_symbol(
            SymbolInfo(
                name="MyClass",
                kind=SymbolKind.CLASS,
                location=SymbolLocation(file_path="/project/src/main.py", start_line=1),
            )
        )
        py_table.add_symbol(
            SymbolInfo(
                name="my_method",
                kind=SymbolKind.METHOD,
                location=SymbolLocation(
                    file_path="/project/src/main.py", start_line=5
                ),
                parent="MyClass",
            )
        )
        index.add_table(py_table)

        # Add TypeScript table
        ts_table = SymbolTable(file_path="/project/frontend/app.ts", language="typescript")
        ts_table.add_symbol(
            SymbolInfo(
                name="AppComponent",
                kind=SymbolKind.CLASS,
                location=SymbolLocation(
                    file_path="/project/frontend/app.ts", start_line=1
                ),
            )
        )
        index.add_table(ts_table)

        return index

    def test_find_symbol_exact(self, sample_index: CodebaseIndex):
        """Test finding symbol by exact path."""
        results = sample_index.find_symbol("MyClass")
        assert len(results) == 1
        assert results[0][1].name == "MyClass"

    def test_find_symbol_suffix(self, sample_index: CodebaseIndex):
        """Test finding symbol by suffix match."""
        results = sample_index.find_symbol("MyClass/my_method")
        assert len(results) == 1
        assert results[0][1].name == "my_method"

    def test_find_symbol_in_file(self, sample_index: CodebaseIndex):
        """Test finding symbol in specific file."""
        results = sample_index.find_symbol("MyClass", "/project/src/main.py")
        assert len(results) == 1

        results = sample_index.find_symbol("MyClass", "/project/frontend/app.ts")
        assert len(results) == 0

    def test_find_by_kind(self, sample_index: CodebaseIndex):
        """Test finding all symbols of a kind."""
        classes = sample_index.find_by_kind(SymbolKind.CLASS)
        assert len(classes) == 2

        methods = sample_index.find_by_kind(SymbolKind.METHOD)
        assert len(methods) == 1

    def test_stats(self, sample_index: CodebaseIndex):
        """Test statistics."""
        stats = sample_index.stats

        assert stats["files"] == 2
        assert stats["symbols"] == 3
        assert stats["class"] == 2
        assert stats["method"] == 1
