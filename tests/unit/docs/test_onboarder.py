"""Tests for ProjectOnboarder."""

from __future__ import annotations

from pathlib import Path

import pytest

from c4.docs.analyzer import CodeAnalyzer, SymbolKind
from c4.docs.onboarder import ProjectOnboarder


@pytest.fixture
def tmp_project(tmp_path: Path) -> Path:
    """Create a minimal multi-language project."""
    # Python file with class inheritance
    py_dir = tmp_path / "src"
    py_dir.mkdir()
    (py_dir / "__init__.py").write_text("")
    (py_dir / "models.py").write_text(
        'class Base:\n    """Base class."""\n    pass\n\n'
        "class User(Base):\n    name: str = ''\n\n"
        'class Admin(User):\n    """Admin user."""\n    level: int = 0\n'
    )
    (py_dir / "main.py").write_text(
        "def main():\n    print('hello')\n\n"
        "if __name__ == '__main__':\n    main()\n"
    )

    # Go file
    go_dir = tmp_path / "cmd"
    go_dir.mkdir()
    (go_dir / "main.go").write_text(
        'package main\n\nimport "fmt"\n\n'
        "type Server struct {\n\tport int\n}\n\n"
        "type Handler interface {\n\tHandle()\n}\n\n"
        "func (s *Server) Start() {\n\tfmt.Println(s.port)\n}\n\n"
        "func main() {\n\tfmt.Println(\"hello\")\n}\n"
    )

    # TypeScript file
    ts_dir = tmp_path / "web"
    ts_dir.mkdir()
    (ts_dir / "app.ts").write_text(
        "interface Renderable {\n  render(): void;\n}\n\n"
        "class Component {\n  name: string = '';\n}\n\n"
        "class Button extends Component {\n  click(): void {}\n}\n\n"
        "function setup(): void {}\n"
    )

    # Config files
    (tmp_path / "go.mod").write_text("module example.com/test\n\ngo 1.21\n")
    (tmp_path / "pyproject.toml").write_text(
        "[project]\nname = \"test\"\n\n[build-system]\nrequires = [\"hatchling\"]\n"
    )

    return tmp_path


@pytest.fixture
def onboarder(tmp_project: Path) -> ProjectOnboarder:
    return ProjectOnboarder(tmp_project)


class TestDetectLanguages:
    def test_detects_python(self, onboarder: ProjectOnboarder) -> None:
        langs = onboarder._detect_languages()
        assert "Python" in langs
        assert langs["Python"] >= 2  # models.py + main.py + __init__.py

    def test_detects_go(self, onboarder: ProjectOnboarder) -> None:
        langs = onboarder._detect_languages()
        assert "Go" in langs
        assert langs["Go"] >= 1

    def test_detects_typescript(self, onboarder: ProjectOnboarder) -> None:
        langs = onboarder._detect_languages()
        assert "TypeScript" in langs


class TestFrameworkDetection:
    def test_go_module(self, onboarder: ProjectOnboarder) -> None:
        frameworks = onboarder._detect_frameworks()
        assert "Go module" in frameworks

    def test_python_hatch(self, onboarder: ProjectOnboarder) -> None:
        frameworks = onboarder._detect_frameworks()
        assert any("Hatch" in f for f in frameworks)

    def test_subdir_go_module(self, tmp_path: Path) -> None:
        """go.mod in a subdirectory should be detected with location."""
        subdir = tmp_path / "backend"
        subdir.mkdir()
        (subdir / "go.mod").write_text("module example.com/backend\n\ngo 1.21\n")

        onboarder = ProjectOnboarder(tmp_path)
        frameworks = onboarder._detect_frameworks()
        assert any("Go module" in f and "backend" in f for f in frameworks)

    def test_subdir_package_json(self, tmp_path: Path) -> None:
        """package.json in a subdirectory should detect Node.js + React."""
        web = tmp_path / "web"
        web.mkdir()
        (web / "package.json").write_text(
            '{"dependencies": {"react": "^18.0.0", "next": "^14.0.0"}}'
        )

        onboarder = ProjectOnboarder(tmp_path)
        frameworks = onboarder._detect_frameworks()
        assert any("Node.js" in f and "web" in f for f in frameworks)
        assert any("React" in f and "web" in f for f in frameworks)
        assert any("Next.js" in f and "web" in f for f in frameworks)

    def test_subdir_cargo(self, tmp_path: Path) -> None:
        """Cargo.toml in a subdirectory should be detected."""
        rs = tmp_path / "native"
        rs.mkdir()
        (rs / "Cargo.toml").write_text('[package]\nname = "native"\n')

        onboarder = ProjectOnboarder(tmp_path)
        frameworks = onboarder._detect_frameworks()
        assert any("Rust" in f and "native" in f for f in frameworks)

    def test_subdir_tauri(self, tmp_path: Path) -> None:
        """src-tauri in a subdirectory should detect Tauri."""
        app = tmp_path / "app"
        app.mkdir()
        (app / "src-tauri").mkdir()

        onboarder = ProjectOnboarder(tmp_path)
        frameworks = onboarder._detect_frameworks()
        assert any("Tauri" in f and "app" in f for f in frameworks)

    def test_excludes_node_modules_subdir(self, tmp_path: Path) -> None:
        """Excluded directories should not be scanned for frameworks."""
        nm = tmp_path / "node_modules"
        nm.mkdir()
        (nm / "package.json").write_text('{"dependencies": {"react": "^18"}}')

        onboarder = ProjectOnboarder(tmp_path)
        frameworks = onboarder._detect_frameworks()
        assert not any("React" in f for f in frameworks)


class TestScan:
    def test_scan_returns_all_keys(self, onboarder: ProjectOnboarder) -> None:
        result = onboarder.scan()
        assert "languages" in result
        assert "frameworks" in result
        assert "entry_points" in result
        assert "symbols" in result
        assert "type_hierarchy" in result
        assert "dependencies" in result
        assert "stats" in result

    def test_scan_finds_symbols(self, onboarder: ProjectOnboarder) -> None:
        result = onboarder.scan()
        assert result["stats"]["total_symbols"] > 0

    def test_scan_finds_entry_points(self, onboarder: ProjectOnboarder) -> None:
        result = onboarder.scan()
        ep_names = [ep["name"] for ep in result["entry_points"]]
        assert "main" in ep_names


class TestGoRegex:
    def test_go_struct_extraction(self, tmp_path: Path) -> None:
        (tmp_path / "test.go").write_text(
            "package main\n\ntype Server struct {\n\tport int\n}\n"
        )
        analyzer = CodeAnalyzer()
        analyzer.add_file(tmp_path / "test.go")
        symbols = analyzer.get_file_symbols(str(tmp_path / "test.go"))

        structs = [s for s in symbols if s.kind == SymbolKind.CLASS]
        assert len(structs) == 1
        assert structs[0].name == "Server"
        assert structs[0].metadata.get("go_kind") == "struct"

    def test_go_interface_extraction(self, tmp_path: Path) -> None:
        (tmp_path / "test.go").write_text(
            "package main\n\ntype Handler interface {\n\tHandle()\n}\n"
        )
        analyzer = CodeAnalyzer()
        analyzer.add_file(tmp_path / "test.go")
        symbols = analyzer.get_file_symbols(str(tmp_path / "test.go"))

        interfaces = [s for s in symbols if s.kind == SymbolKind.INTERFACE]
        assert len(interfaces) == 1
        assert interfaces[0].name == "Handler"

    def test_go_method_extraction(self, tmp_path: Path) -> None:
        (tmp_path / "test.go").write_text(
            "package main\n\nfunc (s *Server) Start() {\n}\n"
        )
        analyzer = CodeAnalyzer()
        analyzer.add_file(tmp_path / "test.go")
        symbols = analyzer.get_file_symbols(str(tmp_path / "test.go"))

        methods = [s for s in symbols if s.kind == SymbolKind.METHOD]
        assert len(methods) == 1
        assert methods[0].name == "Start"
        assert methods[0].parent == "Server"

    def test_go_function_extraction(self, tmp_path: Path) -> None:
        (tmp_path / "test.go").write_text(
            "package main\n\nfunc main() {\n}\n\nfunc helper() {\n}\n"
        )
        analyzer = CodeAnalyzer()
        analyzer.add_file(tmp_path / "test.go")
        symbols = analyzer.get_file_symbols(str(tmp_path / "test.go"))

        funcs = [s for s in symbols if s.kind == SymbolKind.FUNCTION]
        assert len(funcs) == 2
        names = {f.name for f in funcs}
        assert names == {"main", "helper"}

    def test_go_import_extraction(self, tmp_path: Path) -> None:
        (tmp_path / "test.go").write_text(
            'package main\n\nimport (\n\t"fmt"\n\t"os"\n)\n\nfunc main() {}\n'
        )
        analyzer = CodeAnalyzer()
        analyzer.add_file(tmp_path / "test.go")
        deps = analyzer.get_dependencies(str(tmp_path / "test.go"))
        targets = {d.target for d in deps}
        assert "fmt" in targets
        assert "os" in targets


class TestTypeHierarchy:
    def test_python_hierarchy(self, onboarder: ProjectOnboarder) -> None:
        result = onboarder.scan()
        hierarchy = result["type_hierarchy"]
        # Admin(User) and User(Base) should be detected
        assert "Admin" in hierarchy or "User" in hierarchy

    def test_python_bases_tree_sitter(self, tmp_path: Path) -> None:
        """Test base class extraction via tree-sitter."""
        (tmp_path / "test.py").write_text(
            "class Base:\n    pass\n\nclass Child(Base):\n    pass\n"
        )
        analyzer = CodeAnalyzer()
        analyzer.add_file(tmp_path / "test.py")
        hierarchy = analyzer.get_type_hierarchy()
        assert "Child" in hierarchy
        assert "Base" in hierarchy["Child"]


class TestRenderMarkdown:
    def test_render_contains_sections(self, onboarder: ProjectOnboarder) -> None:
        result = onboarder.scan()
        md = onboarder.render_markdown(result)
        assert "# Project Structure Map" in md
        assert "## Languages" in md
        assert "## Frameworks" in md

    def test_render_contains_entry_points(self, onboarder: ProjectOnboarder) -> None:
        result = onboarder.scan()
        md = onboarder.render_markdown(result)
        assert "## Entry Points" in md
        assert "main" in md


class TestMaxFilesLimit:
    def test_respects_limit(self, tmp_path: Path) -> None:
        # Create many files
        for i in range(20):
            (tmp_path / f"file_{i}.py").write_text(f"x_{i} = {i}\n")

        onboarder = ProjectOnboarder(tmp_path, max_files=5)
        result = onboarder.scan()
        assert result["stats"]["total_files_scanned"] <= 5


class TestExcludePatterns:
    def test_excludes_npm_deps(self, tmp_path: Path) -> None:
        """Test that node_modules directory is excluded from scanning."""
        proj = tmp_path / "myproj"
        proj.mkdir()
        nm = proj / "node_modules" / "pkg"
        nm.mkdir(parents=True)
        (nm / "index.js").write_text("export default {}\n")
        (proj / "app.py").write_text("def main():\n    pass\n")

        onboarder = ProjectOnboarder(proj)
        files = onboarder._iter_files(all_extensions=False)
        # Only app.py should be found, not the node_modules file
        assert len(files) == 1
        assert files[0].name == "app.py"

    def test_excludes_virtualenv(self, tmp_path: Path) -> None:
        """Test that .venv directory is excluded from scanning."""
        proj = tmp_path / "myproj"
        proj.mkdir()
        venv = proj / ".venv" / "lib"
        venv.mkdir(parents=True)
        (venv / "module.py").write_text("class Foo:\n    pass\n")
        (proj / "app.py").write_text("class Bar:\n    pass\n")

        onboarder = ProjectOnboarder(proj)
        files = onboarder._iter_files(all_extensions=False)
        assert len(files) == 1
        assert files[0].name == "app.py"


class TestDuplicatePrevention:
    def test_second_run_updates(self, tmp_project: Path, tmp_path: Path) -> None:
        """Running twice should update, not create duplicate documents."""
        from c4.knowledge.documents import DocumentStore

        kb_path = tmp_path / ".c4" / "knowledge"
        kb_path.mkdir(parents=True, exist_ok=True)

        store = DocumentStore(base_path=kb_path)

        # First run: create
        onboarder = ProjectOnboarder(tmp_project)
        analysis = onboarder.scan()
        body = onboarder.render_markdown(analysis)
        doc_id = store.create("pattern", {
            "id": "pat-project-map",
            "title": "Project Structure Map",
            "tags": ["onboarding"],
        }, body=body)

        assert doc_id == "pat-project-map"
        assert (store.docs_dir / "pat-project-map.md").exists()

        # Second run: update
        analysis2 = onboarder.scan()
        body2 = onboarder.render_markdown(analysis2)
        store.update("pat-project-map", body=body2)

        # Still only one file
        pattern_files = list(store.docs_dir.glob("pat-project-map*.md"))
        assert len(pattern_files) == 1
