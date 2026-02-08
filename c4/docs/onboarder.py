"""ProjectOnboarder - Automatic project structure analysis for onboarding.

Scans a project using CodeAnalyzer (tree-sitter + regex) and generates
a `pat-project-map.md` knowledge document with:
- Language breakdown
- Framework detection
- Entry points
- Key symbols (top N by importance)
- Type hierarchy
- Module dependency graph

Usage:
    from c4.docs.onboarder import ProjectOnboarder

    onboarder = ProjectOnboarder(Path("/my/project"))
    analysis = onboarder.scan()
    markdown = onboarder.render_markdown(analysis)
"""

from __future__ import annotations

import time
from collections import Counter
from pathlib import Path
from typing import Any

from c4.docs.analyzer import CodeAnalyzer, SymbolKind

# Directories to always exclude
DEFAULT_EXCLUDE = {
    "node_modules",
    "__pycache__",
    ".git",
    ".venv",
    "venv",
    ".c4",
    ".mypy_cache",
    ".pytest_cache",
    ".ruff_cache",
    "dist",
    "build",
    ".next",
    ".nuxt",
    "target",  # Rust/Java
    "vendor",  # Go
    "bin",
}

# Extensions to language names
EXT_TO_LANG: dict[str, str] = {
    ".py": "Python",
    ".pyi": "Python",
    ".go": "Go",
    ".ts": "TypeScript",
    ".tsx": "TypeScript",
    ".js": "JavaScript",
    ".jsx": "JavaScript",
    ".rs": "Rust",
    ".java": "Java",
    ".rb": "Ruby",
    ".c": "C",
    ".cpp": "C++",
    ".h": "C/C++ Header",
    ".cs": "C#",
    ".swift": "Swift",
    ".kt": "Kotlin",
    ".md": "Markdown",
    ".toml": "TOML",
    ".yaml": "YAML",
    ".yml": "YAML",
    ".json": "JSON",
}

# Analyzable extensions (those CodeAnalyzer can parse)
ANALYZABLE_EXTENSIONS = {".py", ".pyi", ".ts", ".tsx", ".js", ".jsx", ".go"}


class ProjectOnboarder:
    """Analyze a project and generate a structure map."""

    def __init__(
        self,
        project_root: Path,
        max_files: int = 500,
        exclude: set[str] | None = None,
    ) -> None:
        self.project_root = project_root
        self.max_files = max_files
        self.exclude = exclude or DEFAULT_EXCLUDE
        self._analyzer = CodeAnalyzer()

    def scan(self) -> dict[str, Any]:
        """Run full project analysis.

        Returns:
            Dict with keys: languages, frameworks, entry_points,
            symbols, type_hierarchy, dependencies, stats.
        """
        t0 = time.monotonic()

        # Detect languages (all files, not just analyzable)
        languages = self._detect_languages()

        # Add analyzable files to CodeAnalyzer
        file_count = self._add_files()

        # Extract analysis
        frameworks = self._detect_frameworks()
        entry_points = self._analyzer.get_entry_points()
        all_symbols = self._analyzer.get_all_symbols_flat()
        top_symbols = self._prioritize_symbols(all_symbols)
        type_hierarchy = self._analyzer.get_type_hierarchy()
        dep_graph = self._analyzer.get_dependency_graph()
        aggregated_deps = self._aggregate_deps(dep_graph)

        elapsed = time.monotonic() - t0

        return {
            "languages": languages,
            "frameworks": frameworks,
            "entry_points": entry_points,
            "symbols": top_symbols,
            "type_hierarchy": type_hierarchy,
            "dependencies": aggregated_deps,
            "stats": {
                "total_files_scanned": file_count,
                "total_symbols": len(all_symbols),
                "elapsed_seconds": round(elapsed, 2),
            },
        }

    def render_markdown(self, analysis: dict[str, Any]) -> str:
        """Render analysis results as Markdown body (no frontmatter)."""
        lines: list[str] = []
        stats = analysis["stats"]

        lines.append("# Project Structure Map")
        lines.append("")
        lines.append(
            f"**Scanned**: {stats['total_files_scanned']} files, "
            f"{stats['total_symbols']} symbols in {stats['elapsed_seconds']}s"
        )
        lines.append("")

        # Languages
        lines.append("## Languages")
        lines.append("")
        languages: dict[str, int] = analysis["languages"]
        total = sum(languages.values()) or 1
        lines.append("| Language | Files | % |")
        lines.append("|----------|------:|--:|")
        for lang, count in sorted(languages.items(), key=lambda x: -x[1]):
            pct = round(100 * count / total)
            lines.append(f"| {lang} | {count} | {pct}% |")
        lines.append("")

        # Frameworks
        if analysis["frameworks"]:
            lines.append("## Frameworks")
            lines.append("")
            for fw in analysis["frameworks"]:
                lines.append(f"- {fw}")
            lines.append("")

        # Entry points
        if analysis["entry_points"]:
            lines.append("## Entry Points")
            lines.append("")
            for ep in analysis["entry_points"]:
                rel = self._rel_path(ep["file_path"])
                lines.append(f"- `{rel}` ({ep['kind']})")
            lines.append("")

        # Key symbols
        symbols = analysis["symbols"]
        if symbols:
            lines.append(f"## Key Symbols (Top {len(symbols)})")
            lines.append("")

            # Group by kind
            classes = [s for s in symbols if s["kind"] in ("class", "interface")]
            functions = [s for s in symbols if s["kind"] in ("function", "method")]

            if classes:
                lines.append("### Classes & Interfaces")
                lines.append("")
                for s in classes:
                    rel = self._rel_path(s["file_path"])
                    bases_str = ""
                    if s.get("bases"):
                        bases_str = f" ({', '.join(s['bases'])})"
                    lines.append(f"- **{s['name']}**{bases_str} — `{rel}:{s['line']}`")
                lines.append("")

            if functions:
                lines.append("### Key Functions")
                lines.append("")
                for s in functions:
                    rel = self._rel_path(s["file_path"])
                    parent_str = f"{s['parent']}." if s.get("parent") else ""
                    lines.append(
                        f"- `{parent_str}{s['name']}` — `{rel}:{s['line']}`"
                    )
                lines.append("")

        # Type hierarchy
        hierarchy = analysis["type_hierarchy"]
        if hierarchy:
            lines.append("## Type Hierarchy")
            lines.append("")
            lines.append("```")
            for child, bases in sorted(hierarchy.items()):
                lines.append(f"{' + '.join(bases)} <- {child}")
            lines.append("```")
            lines.append("")

        # Module dependencies
        deps = analysis["dependencies"]
        if deps:
            lines.append("## Module Dependencies")
            lines.append("")
            lines.append("```")
            for dep in deps[:30]:  # Limit to 30 for readability
                lines.append(f"{dep['source']} -> {dep['target']}")
            if len(deps) > 30:
                lines.append(f"... and {len(deps) - 30} more")
            lines.append("```")
            lines.append("")

        return "\n".join(lines)

    # ------------------------------------------------------------------
    # Internal
    # ------------------------------------------------------------------

    def _rel_path(self, file_path: str) -> str:
        """Convert absolute path to project-relative."""
        try:
            return str(Path(file_path).relative_to(self.project_root))
        except ValueError:
            return file_path

    def _detect_languages(self) -> dict[str, int]:
        """Count files per language by extension."""
        counter: Counter[str] = Counter()
        for path in self._iter_files(all_extensions=True):
            lang = EXT_TO_LANG.get(path.suffix.lower())
            if lang:
                counter[lang] += 1
        return dict(counter.most_common())

    def _add_files(self) -> int:
        """Add analyzable files to CodeAnalyzer. Returns count."""
        count = 0
        for path in self._iter_files(all_extensions=False):
            if count >= self.max_files:
                break
            try:
                self._analyzer.add_file(path)
                count += 1
            except Exception:
                pass
        return count

    def _iter_files(self, all_extensions: bool = False) -> list[Path]:
        """Iterate project files, excluding configured directories."""
        result = []
        for path in self.project_root.rglob("*"):
            if not path.is_file():
                continue

            # Check exclusions
            parts = path.relative_to(self.project_root).parts
            if any(part in self.exclude for part in parts):
                continue

            if not all_extensions and path.suffix.lower() not in ANALYZABLE_EXTENSIONS:
                continue

            result.append(path)

        return sorted(result)

    def _detect_frameworks(self) -> list[str]:
        """Detect frameworks from config files (root + immediate subdirectories)."""
        frameworks: list[str] = []
        seen: set[str] = set()  # Deduplicate framework names
        root = self.project_root

        # Scan root and one level of subdirectories
        dirs_to_scan = [root]
        for child in sorted(root.iterdir()):
            if child.is_dir() and child.name not in self.exclude:
                dirs_to_scan.append(child)

        for scan_dir in dirs_to_scan:
            prefix = ""
            if scan_dir != root:
                prefix = f" ({scan_dir.name}/)"

            self._detect_frameworks_in(scan_dir, prefix, frameworks, seen)

        return frameworks

    def _detect_frameworks_in(
        self,
        directory: Path,
        prefix: str,
        frameworks: list[str],
        seen: set[str],
    ) -> None:
        """Detect frameworks in a single directory."""
        if (directory / "go.mod").exists():
            label = f"Go module{prefix}"
            if label not in seen:
                seen.add(label)
                frameworks.append(label)

        if (directory / "pyproject.toml").exists():
            label = f"Python (pyproject.toml){prefix}"
            try:
                content = (directory / "pyproject.toml").read_text(encoding="utf-8")
                if "hatchling" in content:
                    label = f"Python (Hatch){prefix}"
                elif "poetry" in content:
                    label = f"Python (Poetry){prefix}"
                elif "setuptools" in content:
                    label = f"Python (setuptools){prefix}"
            except Exception:
                pass
            if label not in seen:
                seen.add(label)
                frameworks.append(label)

        if (directory / "package.json").exists():
            label = f"Node.js{prefix}"
            if label not in seen:
                seen.add(label)
                frameworks.append(label)
            try:
                import json

                pkg = json.loads(
                    (directory / "package.json").read_text(encoding="utf-8")
                )
                deps = {
                    **pkg.get("dependencies", {}),
                    **pkg.get("devDependencies", {}),
                }
                for lib, name in [
                    ("react", "React"),
                    ("next", "Next.js"),
                    ("express", "Express"),
                    ("vue", "Vue.js"),
                ]:
                    if lib in deps:
                        lib_label = f"{name}{prefix}"
                        if lib_label not in seen:
                            seen.add(lib_label)
                            frameworks.append(lib_label)
            except Exception:
                pass

        if (directory / "Cargo.toml").exists():
            label = f"Rust (Cargo){prefix}"
            if label not in seen:
                seen.add(label)
                frameworks.append(label)

        if (directory / "manage.py").exists():
            label = f"Django{prefix}"
            if label not in seen:
                seen.add(label)
                frameworks.append(label)

        for name in ("next.config.js", "next.config.ts", "next.config.mjs"):
            if (directory / name).exists():
                label = f"Next.js{prefix}"
                if label not in seen:
                    seen.add(label)
                    frameworks.append(label)
                break

        if (directory / "src-tauri").is_dir():
            label = f"Tauri{prefix}"
            if label not in seen:
                seen.add(label)
                frameworks.append(label)

    def _prioritize_symbols(
        self, symbols: list[Any], max_count: int = 100
    ) -> list[dict[str, Any]]:
        """Prioritize and serialize symbols for output.

        Priority: classes/interfaces > functions > methods > variables.
        Within each group, sort by name.
        """
        priority = {
            SymbolKind.CLASS: 0,
            SymbolKind.INTERFACE: 1,
            SymbolKind.FUNCTION: 2,
            SymbolKind.METHOD: 3,
            SymbolKind.ENUM: 4,
            SymbolKind.TYPE_ALIAS: 5,
            SymbolKind.CONSTANT: 6,
            SymbolKind.VARIABLE: 7,
        }

        # Filter out imports, parameters, and private/dunder names
        filtered = []
        for s in symbols:
            if s.kind in (SymbolKind.IMPORT, SymbolKind.PARAMETER, SymbolKind.MODULE):
                continue
            if s.name.startswith("_") and s.kind == SymbolKind.VARIABLE:
                continue
            filtered.append(s)

        sorted_symbols = sorted(
            filtered, key=lambda s: (priority.get(s.kind, 99), s.name)
        )

        result = []
        for s in sorted_symbols[:max_count]:
            entry: dict[str, Any] = {
                "name": s.name,
                "kind": s.kind.value,
                "file_path": s.location.file_path,
                "line": s.location.start_line,
            }
            if s.parent:
                entry["parent"] = s.parent
            bases = s.metadata.get("bases", [])
            if bases:
                entry["bases"] = bases
            impl = s.metadata.get("implements", [])
            if impl:
                entry["implements"] = impl
            result.append(entry)

        return result

    def _aggregate_deps(
        self, dep_graph: dict[str, list[str]]
    ) -> list[dict[str, str]]:
        """Aggregate file-level deps to directory-level for readability."""
        dir_deps: dict[tuple[str, str], int] = {}

        for source_file, targets in dep_graph.items():
            try:
                source_dir = str(
                    Path(source_file).relative_to(self.project_root).parent
                )
            except ValueError:
                source_dir = str(Path(source_file).parent)

            if not source_dir or source_dir == ".":
                source_dir = "(root)"

            for target in targets:
                # For internal deps, try to resolve to directory
                target_dir = target.replace(".", "/").split("/")[0] if "." in target else target
                key = (source_dir, target_dir)
                dir_deps[key] = dir_deps.get(key, 0) + 1

        result = []
        for (src, tgt), count in sorted(dir_deps.items(), key=lambda x: -x[1]):
            result.append({"source": src, "target": tgt, "count": count})

        return result
