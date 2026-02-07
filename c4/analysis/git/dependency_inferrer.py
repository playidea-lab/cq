"""File-based dependency inference from commit history.

This module analyzes git commit patterns to infer file dependencies
and relationships based on co-change frequency and import analysis.

Usage:
    from c4.analysis.git.dependency_inferrer import DependencyInferrer, get_dependency_inferrer

    inferrer = get_dependency_inferrer()

    # From commit data
    graph = inferrer.infer_from_commits(commits)

    # Analyze file relations
    relations = inferrer.analyze_file_relations(changed_files)
"""

import logging
import os
import re
from collections import defaultdict
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)


# =============================================================================
# Data Models
# =============================================================================


@dataclass
class FileChange:
    """Represents a file change in a commit.

    Attributes:
        path: File path relative to repository root.
        commit_sha: The commit SHA where change occurred.
        change_type: Type of change (add, modify, delete).
        diff: Optional diff content for the file.
    """

    path: str
    commit_sha: str
    change_type: str = "modify"
    diff: str = ""

    @property
    def extension(self) -> str:
        """Get file extension."""
        return Path(self.path).suffix.lower()

    @property
    def directory(self) -> str:
        """Get parent directory."""
        return str(Path(self.path).parent)


@dataclass
class CommitData:
    """Represents commit data for dependency analysis.

    Attributes:
        sha: The commit SHA.
        files: List of changed files in this commit.
        message: Commit message.
        author: Optional author name.
        timestamp: Optional timestamp string.
    """

    sha: str
    files: list[FileChange] = field(default_factory=list)
    message: str = ""
    author: str = ""
    timestamp: str = ""

    @property
    def file_paths(self) -> list[str]:
        """Get list of file paths in this commit."""
        return [f.path for f in self.files]


@dataclass
class Relation:
    """Represents a dependency relation between two files.

    Attributes:
        source: Source file path.
        target: Target file path.
        relation_type: Type of relation (co_change, import, export).
        weight: Strength of the relation (0.0 to 1.0).
        direction: Direction of relation (forward, backward, bidirectional).
        evidence: List of evidence supporting this relation.
    """

    source: str
    target: str
    relation_type: str = "co_change"
    weight: float = 0.5
    direction: str = "bidirectional"
    evidence: list[str] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary representation."""
        return {
            "source": self.source,
            "target": self.target,
            "relation_type": self.relation_type,
            "weight": self.weight,
            "direction": self.direction,
            "evidence": self.evidence,
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "Relation":
        """Create Relation from dictionary."""
        return cls(
            source=data.get("source", ""),
            target=data.get("target", ""),
            relation_type=data.get("relation_type", "co_change"),
            weight=data.get("weight", 0.5),
            direction=data.get("direction", "bidirectional"),
            evidence=data.get("evidence", []),
        )


@dataclass
class DependencyNode:
    """Represents a node in the dependency graph.

    Attributes:
        id: Unique identifier (usually file path).
        node_type: Type of node (file, module, package).
        metadata: Additional node metadata.
    """

    id: str
    node_type: str = "file"
    metadata: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary representation."""
        return {
            "id": self.id,
            "node_type": self.node_type,
            "metadata": self.metadata,
        }


@dataclass
class DependencyGraph:
    """Represents a graph of file dependencies.

    Attributes:
        nodes: Dictionary of node ID to DependencyNode.
        edges: List of Relation objects representing edges.
        metadata: Graph-level metadata.
    """

    nodes: dict[str, DependencyNode] = field(default_factory=dict)
    edges: list[Relation] = field(default_factory=list)
    metadata: dict[str, Any] = field(default_factory=dict)

    def add_node(self, node_id: str, node_type: str = "file", **metadata: Any) -> None:
        """Add a node to the graph."""
        if node_id not in self.nodes:
            self.nodes[node_id] = DependencyNode(
                id=node_id,
                node_type=node_type,
                metadata=metadata,
            )

    def add_edge(self, relation: Relation) -> None:
        """Add an edge (relation) to the graph."""
        # Ensure nodes exist
        self.add_node(relation.source)
        self.add_node(relation.target)
        self.edges.append(relation)

    def get_neighbors(self, node_id: str) -> list[str]:
        """Get all nodes connected to the given node."""
        neighbors: set[str] = set()
        for edge in self.edges:
            if edge.source == node_id:
                neighbors.add(edge.target)
            if edge.target == node_id:
                neighbors.add(edge.source)
        return list(neighbors)

    def get_outgoing(self, node_id: str) -> list[Relation]:
        """Get all outgoing relations from a node."""
        return [e for e in self.edges if e.source == node_id]

    def get_incoming(self, node_id: str) -> list[Relation]:
        """Get all incoming relations to a node."""
        return [e for e in self.edges if e.target == node_id]

    @property
    def node_count(self) -> int:
        """Get number of nodes in the graph."""
        return len(self.nodes)

    @property
    def edge_count(self) -> int:
        """Get number of edges in the graph."""
        return len(self.edges)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary representation."""
        return {
            "nodes": {k: v.to_dict() for k, v in self.nodes.items()},
            "edges": [e.to_dict() for e in self.edges],
            "metadata": self.metadata,
        }

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "DependencyGraph":
        """Create DependencyGraph from dictionary."""
        graph = cls(
            metadata=data.get("metadata", {}),
        )
        for node_id, node_data in data.get("nodes", {}).items():
            graph.nodes[node_id] = DependencyNode(
                id=node_data.get("id", node_id),
                node_type=node_data.get("node_type", "file"),
                metadata=node_data.get("metadata", {}),
            )
        for edge_data in data.get("edges", []):
            graph.edges.append(Relation.from_dict(edge_data))
        return graph


# =============================================================================
# Import Analysis
# =============================================================================


# Python import patterns
PYTHON_IMPORT_PATTERNS = [
    # import module
    r"^\s*import\s+([a-zA-Z_][a-zA-Z0-9_]*(?:\.[a-zA-Z_][a-zA-Z0-9_]*)*)",
    # from module import ...
    r"^\s*from\s+([a-zA-Z_][a-zA-Z0-9_]*(?:\.[a-zA-Z_][a-zA-Z0-9_]*)*)\s+import",
]

# JavaScript/TypeScript import patterns
JS_IMPORT_PATTERNS = [
    # import X from 'module'
    r"^\s*import\s+.+\s+from\s+['\"]([^'\"]+)['\"]",
    # import 'module'
    r"^\s*import\s+['\"]([^'\"]+)['\"]",
    # require('module')
    r"require\s*\(\s*['\"]([^'\"]+)['\"]\s*\)",
]


def extract_python_imports(content: str) -> list[str]:
    """Extract Python import statements from file content.

    Args:
        content: File content as string.

    Returns:
        List of imported module names.
    """
    imports: list[str] = []
    for line in content.split("\n"):
        for pattern in PYTHON_IMPORT_PATTERNS:
            match = re.match(pattern, line)
            if match:
                imports.append(match.group(1))
                break
    return imports


def extract_js_imports(content: str) -> list[str]:
    """Extract JavaScript/TypeScript imports from file content.

    Args:
        content: File content as string.

    Returns:
        List of imported module paths.
    """
    imports: list[str] = []
    for pattern in JS_IMPORT_PATTERNS:
        matches = re.findall(pattern, content, re.MULTILINE)
        imports.extend(matches)
    return imports


def resolve_python_module_to_file(
    module: str,
    project_root: str | Path,
) -> str | None:
    """Resolve Python module name to file path.

    Args:
        module: Python module name (e.g., "c4.memory.store").
        project_root: Project root directory.

    Returns:
        File path if found, None otherwise.
    """
    project_root = Path(project_root)

    # Convert module.submodule to module/submodule
    path_parts = module.split(".")

    # Try as direct file
    file_path = project_root / "/".join(path_parts)
    if (file_path.with_suffix(".py")).exists():
        return str(file_path.with_suffix(".py").relative_to(project_root))

    # Try as package
    package_init = file_path / "__init__.py"
    if package_init.exists():
        return str(package_init.relative_to(project_root))

    return None


def resolve_js_import_to_file(
    import_path: str,
    source_file: str,
    project_root: str | Path,
) -> str | None:
    """Resolve JavaScript import path to file path.

    Args:
        import_path: Import path from source file.
        source_file: Source file making the import.
        project_root: Project root directory.

    Returns:
        File path if found, None otherwise.
    """
    project_root = Path(project_root).resolve()
    source_dir = (project_root / source_file).parent

    # Skip node_modules imports
    if not import_path.startswith("."):
        return None

    # Resolve relative path
    resolved = (source_dir / import_path).resolve()

    # Try common extensions
    extensions = ["", ".js", ".ts", ".jsx", ".tsx", "/index.js", "/index.ts"]
    for ext in extensions:
        candidate = Path(str(resolved) + ext)
        if candidate.exists():
            try:
                return str(candidate.relative_to(project_root))
            except ValueError:
                return None

    return None


# =============================================================================
# Dependency Inferrer
# =============================================================================


class DependencyInferrer:
    """Infers file dependencies from commit history and code analysis.

    This class analyzes git commit patterns to identify which files
    are related based on:
    1. Co-change frequency (files changed together)
    2. Import/export relationships

    Attributes:
        project_root: Root directory of the project.
        min_cochange_count: Minimum co-changes to create relation.
        max_cochange_weight: Maximum weight for co-change relations.
    """

    def __init__(
        self,
        project_root: str | Path | None = None,
        min_cochange_count: int = 2,
        max_cochange_weight: float = 1.0,
    ) -> None:
        """Initialize the dependency inferrer.

        Args:
            project_root: Root directory of the project.
            min_cochange_count: Minimum times files must change together.
            max_cochange_weight: Maximum weight for co-change relations.
        """
        self.project_root = Path(project_root) if project_root else Path.cwd()
        self.min_cochange_count = min_cochange_count
        self.max_cochange_weight = max_cochange_weight

    def infer_from_commits(
        self,
        commits: list[CommitData],
        include_imports: bool = True,
    ) -> DependencyGraph:
        """Infer dependency graph from commit data.

        Analyzes commit history to find:
        1. Co-change patterns (files frequently changed together)
        2. Import relationships (if include_imports=True)

        Args:
            commits: List of CommitData objects.
            include_imports: Whether to analyze imports.

        Returns:
            DependencyGraph with inferred dependencies.
        """
        graph = DependencyGraph(
            metadata={
                "commit_count": len(commits),
                "include_imports": include_imports,
            }
        )

        # Build co-change matrix
        cochange_counts: dict[tuple[str, str], int] = defaultdict(int)
        file_commit_counts: dict[str, int] = defaultdict(int)

        for commit in commits:
            files = commit.file_paths
            # Count individual file occurrences
            for file in files:
                file_commit_counts[file] += 1
                graph.add_node(file, node_type="file")

            # Count co-changes (pairs of files in same commit)
            for i, file1 in enumerate(files):
                for file2 in files[i + 1 :]:
                    # Use sorted pair to avoid duplicates
                    pair = tuple(sorted([file1, file2]))
                    cochange_counts[pair] += 1

        # Create relations from co-change patterns
        for (file1, file2), count in cochange_counts.items():
            if count >= self.min_cochange_count:
                # Calculate weight based on co-change frequency
                max_individual = max(
                    file_commit_counts[file1],
                    file_commit_counts[file2],
                )
                weight = min(
                    count / max_individual if max_individual > 0 else 0,
                    self.max_cochange_weight,
                )

                relation = Relation(
                    source=file1,
                    target=file2,
                    relation_type="co_change",
                    weight=weight,
                    direction="bidirectional",
                    evidence=[f"Changed together {count} times"],
                )
                graph.add_edge(relation)

        # Analyze imports if requested
        if include_imports:
            import_relations = self._analyze_all_imports(commits)
            for relation in import_relations:
                graph.add_edge(relation)

        return graph

    def analyze_file_relations(
        self,
        changed_files: list[str],
        analyze_imports: bool = True,
    ) -> list[Relation]:
        """Analyze relations between a set of files.

        Useful for understanding the impact of changes in a PR or commit.

        Args:
            changed_files: List of file paths to analyze.
            analyze_imports: Whether to analyze import relationships.

        Returns:
            List of Relation objects.
        """
        relations: list[Relation] = []

        # Same directory relation
        dir_files: dict[str, list[str]] = defaultdict(list)
        for file in changed_files:
            dir_path = str(Path(file).parent)
            dir_files[dir_path].append(file)

        for dir_path, files in dir_files.items():
            if len(files) > 1:
                # Files in same directory are likely related
                for i, file1 in enumerate(files):
                    for file2 in files[i + 1 :]:
                        relations.append(
                            Relation(
                                source=file1,
                                target=file2,
                                relation_type="same_directory",
                                weight=0.3,
                                direction="bidirectional",
                                evidence=[f"Both in {dir_path}"],
                            )
                        )

        # Same extension relation (weaker)
        ext_files: dict[str, list[str]] = defaultdict(list)
        for file in changed_files:
            ext = Path(file).suffix
            if ext:
                ext_files[ext].append(file)

        # Analyze imports if requested
        if analyze_imports:
            for file in changed_files:
                import_relations = self._analyze_file_imports(file)
                # Only keep relations where target is in changed_files
                for rel in import_relations:
                    if rel.target in changed_files:
                        relations.append(rel)

        return relations

    def _analyze_all_imports(self, commits: list[CommitData]) -> list[Relation]:
        """Analyze import relationships from commit file changes.

        Args:
            commits: List of commits to analyze.

        Returns:
            List of import-based Relation objects.
        """
        relations: list[Relation] = []
        seen_relations: set[tuple[str, str]] = set()

        for commit in commits:
            for file_change in commit.files:
                if not file_change.diff:
                    continue

                file_relations = self._extract_import_relations(
                    file_change.path,
                    file_change.diff,
                )
                for rel in file_relations:
                    key = (rel.source, rel.target)
                    if key not in seen_relations:
                        seen_relations.add(key)
                        relations.append(rel)

        return relations

    def _analyze_file_imports(self, file_path: str) -> list[Relation]:
        """Analyze imports in a specific file.

        Args:
            file_path: Path to the file to analyze.

        Returns:
            List of import-based Relation objects.
        """
        full_path = self.project_root / file_path
        if not full_path.exists():
            return []

        try:
            content = full_path.read_text(encoding="utf-8")
        except (OSError, UnicodeDecodeError):
            return []

        return self._extract_import_relations(file_path, content)

    def _extract_import_relations(
        self,
        source_file: str,
        content: str,
    ) -> list[Relation]:
        """Extract import relations from file content.

        Args:
            source_file: Source file path.
            content: File content (source or diff).

        Returns:
            List of import-based Relation objects.
        """
        relations: list[Relation] = []
        extension = Path(source_file).suffix.lower()

        if extension == ".py":
            imports = extract_python_imports(content)
            for module in imports:
                target = resolve_python_module_to_file(module, self.project_root)
                if target and target != source_file:
                    relations.append(
                        Relation(
                            source=source_file,
                            target=target,
                            relation_type="import",
                            weight=0.7,
                            direction="forward",
                            evidence=[f"Imports {module}"],
                        )
                    )

        elif extension in (".js", ".ts", ".jsx", ".tsx"):
            imports = extract_js_imports(content)
            for import_path in imports:
                target = resolve_js_import_to_file(
                    import_path,
                    source_file,
                    self.project_root,
                )
                if target and target != source_file:
                    relations.append(
                        Relation(
                            source=source_file,
                            target=target,
                            relation_type="import",
                            weight=0.7,
                            direction="forward",
                            evidence=[f"Imports {import_path}"],
                        )
                    )

        return relations


# =============================================================================
# Factory Function
# =============================================================================


def get_dependency_inferrer(
    project_root: str | Path | None = None,
    min_cochange_count: int = 2,
    **kwargs: Any,
) -> DependencyInferrer:
    """Factory function to create a DependencyInferrer.

    Args:
        project_root: Root directory of the project.
        min_cochange_count: Minimum times files must change together.
        **kwargs: Additional arguments passed to DependencyInferrer.

    Returns:
        Configured DependencyInferrer instance.

    Example:
        >>> inferrer = get_dependency_inferrer()
        >>> graph = inferrer.infer_from_commits(commits)
    """
    if project_root is None:
        # Try to find project root from environment or cwd
        project_root = os.environ.get("C4_PROJECT_ROOT", Path.cwd())

    return DependencyInferrer(
        project_root=project_root,
        min_cochange_count=min_cochange_count,
        **kwargs,
    )
