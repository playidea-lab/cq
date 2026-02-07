"""Tests for c4/memory/dependency_inferrer.py"""

import tempfile
from pathlib import Path

from c4.analysis.git.dependency_inferrer import (
    CommitData,
    DependencyGraph,
    DependencyInferrer,
    DependencyNode,
    FileChange,
    Relation,
    extract_js_imports,
    extract_python_imports,
    get_dependency_inferrer,
    resolve_js_import_to_file,
    resolve_python_module_to_file,
)

# =============================================================================
# FileChange Tests
# =============================================================================


class TestFileChange:
    """Tests for FileChange dataclass."""

    def test_create_basic(self) -> None:
        """Should create with required fields."""
        fc = FileChange(path="src/module.py", commit_sha="abc123")

        assert fc.path == "src/module.py"
        assert fc.commit_sha == "abc123"
        assert fc.change_type == "modify"
        assert fc.diff == ""

    def test_extension_property(self) -> None:
        """Should return file extension."""
        assert FileChange(path="file.py", commit_sha="x").extension == ".py"
        assert FileChange(path="file.ts", commit_sha="x").extension == ".ts"
        assert FileChange(path="Makefile", commit_sha="x").extension == ""

    def test_directory_property(self) -> None:
        """Should return parent directory."""
        fc = FileChange(path="src/module/file.py", commit_sha="x")
        assert fc.directory == "src/module"


# =============================================================================
# CommitData Tests
# =============================================================================


class TestCommitData:
    """Tests for CommitData dataclass."""

    def test_create_basic(self) -> None:
        """Should create with required fields."""
        commit = CommitData(sha="abc123")

        assert commit.sha == "abc123"
        assert commit.files == []
        assert commit.message == ""

    def test_file_paths_property(self) -> None:
        """Should return list of file paths."""
        commit = CommitData(
            sha="abc123",
            files=[
                FileChange(path="a.py", commit_sha="abc123"),
                FileChange(path="b.py", commit_sha="abc123"),
            ],
        )
        assert commit.file_paths == ["a.py", "b.py"]


# =============================================================================
# Relation Tests
# =============================================================================


class TestRelation:
    """Tests for Relation dataclass."""

    def test_create_basic(self) -> None:
        """Should create with required fields."""
        rel = Relation(source="a.py", target="b.py")

        assert rel.source == "a.py"
        assert rel.target == "b.py"
        assert rel.relation_type == "co_change"
        assert rel.weight == 0.5

    def test_to_dict(self) -> None:
        """Should convert to dictionary."""
        rel = Relation(
            source="a.py",
            target="b.py",
            relation_type="import",
            weight=0.8,
            direction="forward",
            evidence=["Imports module"],
        )
        result = rel.to_dict()

        assert result["source"] == "a.py"
        assert result["target"] == "b.py"
        assert result["relation_type"] == "import"
        assert result["weight"] == 0.8
        assert result["evidence"] == ["Imports module"]

    def test_from_dict(self) -> None:
        """Should create from dictionary."""
        data = {
            "source": "x.py",
            "target": "y.py",
            "relation_type": "co_change",
            "weight": 0.6,
        }
        rel = Relation.from_dict(data)

        assert rel.source == "x.py"
        assert rel.target == "y.py"
        assert rel.weight == 0.6


# =============================================================================
# DependencyNode Tests
# =============================================================================


class TestDependencyNode:
    """Tests for DependencyNode dataclass."""

    def test_create_basic(self) -> None:
        """Should create with required fields."""
        node = DependencyNode(id="file.py")

        assert node.id == "file.py"
        assert node.node_type == "file"
        assert node.metadata == {}

    def test_to_dict(self) -> None:
        """Should convert to dictionary."""
        node = DependencyNode(
            id="module.py",
            node_type="module",
            metadata={"lines": 100},
        )
        result = node.to_dict()

        assert result["id"] == "module.py"
        assert result["node_type"] == "module"
        assert result["metadata"]["lines"] == 100


# =============================================================================
# DependencyGraph Tests
# =============================================================================


class TestDependencyGraph:
    """Tests for DependencyGraph dataclass."""

    def test_create_empty(self) -> None:
        """Should create empty graph."""
        graph = DependencyGraph()

        assert graph.node_count == 0
        assert graph.edge_count == 0

    def test_add_node(self) -> None:
        """Should add nodes."""
        graph = DependencyGraph()
        graph.add_node("a.py")
        graph.add_node("b.py", node_type="module")

        assert graph.node_count == 2
        assert graph.nodes["a.py"].node_type == "file"
        assert graph.nodes["b.py"].node_type == "module"

    def test_add_node_idempotent(self) -> None:
        """Should not duplicate nodes."""
        graph = DependencyGraph()
        graph.add_node("a.py")
        graph.add_node("a.py")

        assert graph.node_count == 1

    def test_add_edge(self) -> None:
        """Should add edges and create nodes."""
        graph = DependencyGraph()
        rel = Relation(source="a.py", target="b.py")
        graph.add_edge(rel)

        assert graph.node_count == 2
        assert graph.edge_count == 1
        assert graph.nodes["a.py"] is not None
        assert graph.nodes["b.py"] is not None

    def test_get_neighbors(self) -> None:
        """Should return connected nodes."""
        graph = DependencyGraph()
        graph.add_edge(Relation(source="a.py", target="b.py"))
        graph.add_edge(Relation(source="a.py", target="c.py"))

        neighbors = graph.get_neighbors("a.py")
        assert set(neighbors) == {"b.py", "c.py"}

        neighbors_b = graph.get_neighbors("b.py")
        assert set(neighbors_b) == {"a.py"}

    def test_get_outgoing(self) -> None:
        """Should return outgoing relations."""
        graph = DependencyGraph()
        graph.add_edge(Relation(source="a.py", target="b.py"))
        graph.add_edge(Relation(source="a.py", target="c.py"))
        graph.add_edge(Relation(source="b.py", target="a.py"))

        outgoing = graph.get_outgoing("a.py")
        assert len(outgoing) == 2

    def test_get_incoming(self) -> None:
        """Should return incoming relations."""
        graph = DependencyGraph()
        graph.add_edge(Relation(source="a.py", target="b.py"))
        graph.add_edge(Relation(source="c.py", target="b.py"))

        incoming = graph.get_incoming("b.py")
        assert len(incoming) == 2

    def test_to_dict_roundtrip(self) -> None:
        """Should roundtrip through dict."""
        graph = DependencyGraph(metadata={"version": 1})
        graph.add_edge(Relation(source="a.py", target="b.py", weight=0.8))

        data = graph.to_dict()
        restored = DependencyGraph.from_dict(data)

        assert restored.node_count == 2
        assert restored.edge_count == 1
        assert restored.metadata["version"] == 1


# =============================================================================
# Import Extraction Tests
# =============================================================================


class TestExtractPythonImports:
    """Tests for extract_python_imports function."""

    def test_basic_import(self) -> None:
        """Should extract basic imports."""
        content = "import os\nimport sys"
        imports = extract_python_imports(content)

        assert "os" in imports
        assert "sys" in imports

    def test_from_import(self) -> None:
        """Should extract from imports."""
        content = "from pathlib import Path\nfrom typing import Any"
        imports = extract_python_imports(content)

        assert "pathlib" in imports
        assert "typing" in imports

    def test_nested_module_import(self) -> None:
        """Should extract nested module imports."""
        content = "from c4.memory.store import MemoryStore"
        imports = extract_python_imports(content)

        assert "c4.memory.store" in imports

    def test_ignores_comments(self) -> None:
        """Should not extract from comments."""
        content = "# import fake\nimport real"
        imports = extract_python_imports(content)

        assert "real" in imports
        assert "fake" not in imports


class TestExtractJsImports:
    """Tests for extract_js_imports function."""

    def test_import_from(self) -> None:
        """Should extract import from statements."""
        content = "import React from 'react'"
        imports = extract_js_imports(content)

        assert "react" in imports

    def test_import_side_effect(self) -> None:
        """Should extract side-effect imports."""
        content = "import './styles.css'"
        imports = extract_js_imports(content)

        assert "./styles.css" in imports

    def test_require(self) -> None:
        """Should extract require statements."""
        content = "const fs = require('fs')"
        imports = extract_js_imports(content)

        assert "fs" in imports

    def test_relative_import(self) -> None:
        """Should extract relative imports."""
        content = "import { utils } from './utils'"
        imports = extract_js_imports(content)

        assert "./utils" in imports


# =============================================================================
# Module Resolution Tests
# =============================================================================


class TestResolvePythonModuleToFile:
    """Tests for resolve_python_module_to_file function."""

    def test_resolves_direct_file(self) -> None:
        """Should resolve module to .py file."""
        with tempfile.TemporaryDirectory() as tmpdir:
            # Create test file
            (Path(tmpdir) / "module.py").touch()

            result = resolve_python_module_to_file("module", tmpdir)
            assert result == "module.py"

    def test_resolves_nested_module(self) -> None:
        """Should resolve nested module."""
        with tempfile.TemporaryDirectory() as tmpdir:
            # Create nested structure
            (Path(tmpdir) / "pkg").mkdir()
            (Path(tmpdir) / "pkg" / "submodule.py").touch()

            result = resolve_python_module_to_file("pkg.submodule", tmpdir)
            assert result == "pkg/submodule.py"

    def test_resolves_package_init(self) -> None:
        """Should resolve package to __init__.py."""
        with tempfile.TemporaryDirectory() as tmpdir:
            # Create package
            (Path(tmpdir) / "pkg").mkdir()
            (Path(tmpdir) / "pkg" / "__init__.py").touch()

            result = resolve_python_module_to_file("pkg", tmpdir)
            assert result == "pkg/__init__.py"

    def test_returns_none_for_missing(self) -> None:
        """Should return None for missing module."""
        with tempfile.TemporaryDirectory() as tmpdir:
            result = resolve_python_module_to_file("nonexistent", tmpdir)
            assert result is None


class TestResolveJsImportToFile:
    """Tests for resolve_js_import_to_file function."""

    def test_resolves_relative_import(self) -> None:
        """Should resolve relative import."""
        with tempfile.TemporaryDirectory() as tmpdir:
            # Create test file
            (Path(tmpdir) / "src").mkdir()
            (Path(tmpdir) / "src" / "utils.js").touch()
            (Path(tmpdir) / "src" / "main.js").touch()

            result = resolve_js_import_to_file(
                "./utils",
                "src/main.js",
                tmpdir,
            )
            assert result == "src/utils.js"

    def test_skips_node_modules(self) -> None:
        """Should skip non-relative imports."""
        with tempfile.TemporaryDirectory() as tmpdir:
            result = resolve_js_import_to_file(
                "react",
                "src/main.js",
                tmpdir,
            )
            assert result is None

    def test_returns_none_for_missing(self) -> None:
        """Should return None for missing file."""
        with tempfile.TemporaryDirectory() as tmpdir:
            (Path(tmpdir) / "src").mkdir()
            (Path(tmpdir) / "src" / "main.js").touch()

            result = resolve_js_import_to_file(
                "./nonexistent",
                "src/main.js",
                tmpdir,
            )
            assert result is None


# =============================================================================
# DependencyInferrer Tests
# =============================================================================


class TestDependencyInferrer:
    """Tests for DependencyInferrer class."""

    def test_init_default(self) -> None:
        """Should create with defaults."""
        inferrer = DependencyInferrer()

        assert inferrer.min_cochange_count == 2
        assert inferrer.max_cochange_weight == 1.0

    def test_init_custom(self) -> None:
        """Should create with custom settings."""
        inferrer = DependencyInferrer(
            min_cochange_count=3,
            max_cochange_weight=0.8,
        )

        assert inferrer.min_cochange_count == 3
        assert inferrer.max_cochange_weight == 0.8


class TestInferFromCommits:
    """Tests for infer_from_commits method."""

    def test_empty_commits(self) -> None:
        """Should handle empty commit list."""
        inferrer = DependencyInferrer()
        graph = inferrer.infer_from_commits([])

        assert graph.node_count == 0
        assert graph.edge_count == 0

    def test_single_commit_no_relations(self) -> None:
        """Should not create relations from single file commit."""
        inferrer = DependencyInferrer()
        commits = [
            CommitData(
                sha="abc",
                files=[FileChange(path="a.py", commit_sha="abc")],
            )
        ]
        graph = inferrer.infer_from_commits(commits, include_imports=False)

        assert graph.node_count == 1
        assert graph.edge_count == 0

    def test_cochange_creates_relation(self) -> None:
        """Should create relation for co-changed files."""
        inferrer = DependencyInferrer(min_cochange_count=2)
        commits = [
            CommitData(
                sha="1",
                files=[
                    FileChange(path="a.py", commit_sha="1"),
                    FileChange(path="b.py", commit_sha="1"),
                ],
            ),
            CommitData(
                sha="2",
                files=[
                    FileChange(path="a.py", commit_sha="2"),
                    FileChange(path="b.py", commit_sha="2"),
                ],
            ),
        ]
        graph = inferrer.infer_from_commits(commits, include_imports=False)

        assert graph.node_count == 2
        assert graph.edge_count == 1

        edge = graph.edges[0]
        assert edge.relation_type == "co_change"
        assert "2 times" in edge.evidence[0]

    def test_below_min_cochange_no_relation(self) -> None:
        """Should not create relation below min_cochange_count."""
        inferrer = DependencyInferrer(min_cochange_count=3)
        commits = [
            CommitData(
                sha="1",
                files=[
                    FileChange(path="a.py", commit_sha="1"),
                    FileChange(path="b.py", commit_sha="1"),
                ],
            ),
            CommitData(
                sha="2",
                files=[
                    FileChange(path="a.py", commit_sha="2"),
                    FileChange(path="b.py", commit_sha="2"),
                ],
            ),
        ]
        graph = inferrer.infer_from_commits(commits, include_imports=False)

        assert graph.node_count == 2
        assert graph.edge_count == 0  # Only 2 co-changes, need 3

    def test_weight_calculation(self) -> None:
        """Should calculate weight based on co-change frequency."""
        inferrer = DependencyInferrer(min_cochange_count=2)
        commits = [
            CommitData(
                sha="1",
                files=[
                    FileChange(path="a.py", commit_sha="1"),
                    FileChange(path="b.py", commit_sha="1"),
                ],
            ),
            CommitData(
                sha="2",
                files=[
                    FileChange(path="a.py", commit_sha="2"),
                    FileChange(path="b.py", commit_sha="2"),
                ],
            ),
            CommitData(
                sha="3",
                files=[FileChange(path="a.py", commit_sha="3")],  # Only a
            ),
        ]
        graph = inferrer.infer_from_commits(commits, include_imports=False)

        # a changed 3 times, b changed 2 times, co-changed 2 times
        # weight = 2 / max(3, 2) = 2/3 ≈ 0.667
        edge = graph.edges[0]
        assert 0.6 < edge.weight < 0.7

    def test_multiple_files_creates_all_pairs(self) -> None:
        """Should create relations for all file pairs."""
        inferrer = DependencyInferrer(min_cochange_count=2)
        commits = [
            CommitData(
                sha="1",
                files=[
                    FileChange(path="a.py", commit_sha="1"),
                    FileChange(path="b.py", commit_sha="1"),
                    FileChange(path="c.py", commit_sha="1"),
                ],
            ),
            CommitData(
                sha="2",
                files=[
                    FileChange(path="a.py", commit_sha="2"),
                    FileChange(path="b.py", commit_sha="2"),
                    FileChange(path="c.py", commit_sha="2"),
                ],
            ),
        ]
        graph = inferrer.infer_from_commits(commits, include_imports=False)

        # 3 files = 3 pairs: (a,b), (a,c), (b,c)
        assert graph.node_count == 3
        assert graph.edge_count == 3

    def test_metadata_includes_commit_count(self) -> None:
        """Should include commit count in metadata."""
        inferrer = DependencyInferrer()
        commits = [
            CommitData(sha="1", files=[]),
            CommitData(sha="2", files=[]),
        ]
        graph = inferrer.infer_from_commits(commits)

        assert graph.metadata["commit_count"] == 2


class TestAnalyzeFileRelations:
    """Tests for analyze_file_relations method."""

    def test_empty_files(self) -> None:
        """Should handle empty file list."""
        inferrer = DependencyInferrer()
        relations = inferrer.analyze_file_relations([])

        assert relations == []

    def test_same_directory_relation(self) -> None:
        """Should create relation for files in same directory."""
        inferrer = DependencyInferrer()
        relations = inferrer.analyze_file_relations(
            ["src/a.py", "src/b.py"],
            analyze_imports=False,
        )

        assert len(relations) == 1
        assert relations[0].relation_type == "same_directory"
        assert relations[0].weight == 0.3

    def test_different_directories_no_relation(self) -> None:
        """Should not create relation for files in different dirs."""
        inferrer = DependencyInferrer()
        relations = inferrer.analyze_file_relations(
            ["src/a.py", "lib/b.py"],
            analyze_imports=False,
        )

        assert len(relations) == 0

    def test_multiple_same_directory_files(self) -> None:
        """Should create all pairs for same directory files."""
        inferrer = DependencyInferrer()
        relations = inferrer.analyze_file_relations(
            ["src/a.py", "src/b.py", "src/c.py"],
            analyze_imports=False,
        )

        # 3 files = 3 pairs
        assert len(relations) == 3


class TestImportAnalysis:
    """Tests for import analysis integration."""

    def test_python_import_creates_relation(self) -> None:
        """Should create relation for Python imports."""
        with tempfile.TemporaryDirectory() as tmpdir:
            # Create test files
            src_dir = Path(tmpdir) / "src"
            src_dir.mkdir()
            (src_dir / "main.py").write_text(
                "from src.utils import helper\n"
            )
            (src_dir / "utils.py").touch()

            inferrer = DependencyInferrer(project_root=tmpdir)
            relations = inferrer._analyze_file_imports("src/main.py")

            # Should find import relation
            import_relations = [r for r in relations if r.relation_type == "import"]
            assert len(import_relations) == 1
            assert import_relations[0].direction == "forward"


# =============================================================================
# Factory Function Tests
# =============================================================================


class TestGetDependencyInferrer:
    """Tests for get_dependency_inferrer factory function."""

    def test_create_default(self) -> None:
        """Should create with defaults."""
        inferrer = get_dependency_inferrer()

        assert inferrer is not None
        assert inferrer.min_cochange_count == 2

    def test_create_with_custom_settings(self) -> None:
        """Should pass custom settings."""
        inferrer = get_dependency_inferrer(min_cochange_count=5)

        assert inferrer.min_cochange_count == 5

    def test_create_with_project_root(self) -> None:
        """Should set project root."""
        with tempfile.TemporaryDirectory() as tmpdir:
            inferrer = get_dependency_inferrer(project_root=tmpdir)

            assert inferrer.project_root == Path(tmpdir)


# =============================================================================
# Integration Tests
# =============================================================================


class TestIntegration:
    """Integration tests for full workflow."""

    def test_full_workflow(self) -> None:
        """Should handle full inference workflow."""
        inferrer = DependencyInferrer(min_cochange_count=2)

        # Simulate commit history
        commits = [
            CommitData(
                sha="commit1",
                message="feat: add user auth",
                files=[
                    FileChange(path="src/auth/login.py", commit_sha="commit1"),
                    FileChange(path="src/auth/utils.py", commit_sha="commit1"),
                    FileChange(path="tests/test_auth.py", commit_sha="commit1"),
                ],
            ),
            CommitData(
                sha="commit2",
                message="fix: auth bug",
                files=[
                    FileChange(path="src/auth/login.py", commit_sha="commit2"),
                    FileChange(path="src/auth/utils.py", commit_sha="commit2"),
                ],
            ),
            CommitData(
                sha="commit3",
                message="feat: add api",
                files=[
                    FileChange(path="src/api/routes.py", commit_sha="commit3"),
                    FileChange(path="src/api/handlers.py", commit_sha="commit3"),
                ],
            ),
            CommitData(
                sha="commit4",
                message="test: add more tests",
                files=[
                    FileChange(path="src/auth/login.py", commit_sha="commit4"),
                    FileChange(path="tests/test_auth.py", commit_sha="commit4"),
                ],
            ),
        ]

        graph = inferrer.infer_from_commits(commits, include_imports=False)

        # Should have nodes for all files
        assert graph.node_count == 5

        # Should have relations:
        # - login.py <-> utils.py (2 co-changes)
        # - login.py <-> test_auth.py (2 co-changes)
        # Note: routes.py <-> handlers.py only 1 co-change, not enough
        assert graph.edge_count == 2

        # Check specific relations
        login_neighbors = graph.get_neighbors("src/auth/login.py")
        assert "src/auth/utils.py" in login_neighbors
        assert "tests/test_auth.py" in login_neighbors

    def test_analyze_pr_files(self) -> None:
        """Should analyze files in a PR-like scenario."""
        inferrer = DependencyInferrer()

        # Files changed in a PR
        changed_files = [
            "src/api/routes.py",
            "src/api/handlers.py",
            "src/api/models.py",
            "tests/test_api.py",
        ]

        relations = inferrer.analyze_file_relations(
            changed_files,
            analyze_imports=False,
        )

        # Same directory relations: 3 files in src/api/ = 3 pairs
        api_relations = [
            r for r in relations
            if r.relation_type == "same_directory" and "api" in r.evidence[0]
        ]
        assert len(api_relations) == 3
