"""Code operations for C4 Daemon.

This module contains code analysis and modification operations extracted from C4Daemon:
- c4_find_symbol: Find symbols matching a name path pattern
- c4_replace_symbol_body: Replace the body of a symbol
- c4_insert_before_symbol: Insert content before a symbol
- c4_insert_after_symbol: Insert content after a symbol
- c4_rename_symbol: Rename a symbol across the codebase
- c4_read_file: Read a file or portion of it
- c4_create_text_file: Create or overwrite a text file
- c4_search_for_pattern: Search for a regex pattern in files
- c4_replace_content: Replace content in a file
- c4_get_symbols_overview: Get an overview of symbols in a file

These operations are delegated from C4Daemon for modularity.
"""

import re
from pathlib import Path
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from .c4_daemon import C4Daemon


class CodeOps:
    """Code operations handler for C4 Daemon.

    Provides code analysis and modification operations for working
    with Python source code, including symbol manipulation and file operations.
    """

    def __init__(self, daemon: "C4Daemon"):
        """Initialize CodeOps with parent daemon reference.

        Args:
            daemon: Parent C4Daemon instance for root path access
        """
        self._daemon = daemon

    # =========================================================================
    # Symbol Operations
    # =========================================================================

    def _get_symbol_by_name_path(
        self,
        name_path: str,
        file_path: str | None = None,
    ) -> tuple[Any | None, str | None, str | None]:
        """Find a symbol by name path.

        Args:
            name_path: Symbol name or qualified name (e.g., "MyClass" or "MyClass.method")
            file_path: Optional file path to restrict search

        Returns:
            Tuple of (symbol, file_path, error_message)
        """
        from c4.docs.analyzer import CodeAnalyzer

        try:
            analyzer = CodeAnalyzer()

            if file_path:
                abs_file_path = Path(file_path)
                if not abs_file_path.is_absolute():
                    abs_file_path = self._daemon.root / file_path
                if not abs_file_path.exists():
                    return None, None, f"File not found: {file_path}"
                analyzer.add_file(abs_file_path)
                search_path = str(abs_file_path)
            else:
                analyzer.add_directory(
                    self._daemon.root,
                    recursive=True,
                    exclude_patterns=[
                        "**/node_modules/**",
                        "**/__pycache__/**",
                        "**/.git/**",
                        "**/venv/**",
                        "**/.venv/**",
                        "**/.c4/**",
                        "**/.claude/**",
                    ],
                )
                search_path = None

            # Parse name_path to get symbol name and parent
            parts = name_path.split(".")
            symbol_name = parts[-1]

            # Find the symbol
            symbols = analyzer.find_symbol(
                symbol_name, file_path=search_path, exact_match=True
            )

            if not symbols:
                return None, None, f"Symbol not found: {name_path}"

            # If qualified name, filter by parent
            if len(parts) > 1:
                parent_name = ".".join(parts[:-1])
                matching = [
                    s for s in symbols
                    if s.parent == parent_name or s.qualified_name == name_path
                ]
                if not matching:
                    return None, None, f"Symbol with parent '{parent_name}' not found"
                symbols = matching

            # Return the first match
            symbol = symbols[0]

            # Find which file contains this symbol
            symbol_file = symbol.location.file_path

            return symbol, symbol_file, None

        except Exception as e:
            return None, None, str(e)

    def find_symbol(
        self,
        name_path_pattern: str,
        relative_path: str = "",
        include_body: bool = False,
        depth: int = 0,
    ) -> dict[str, Any]:
        """Find symbols matching the name path pattern.

        Name path patterns:
        - Simple name: "method_name" - matches any symbol with that name
        - Relative path: "ClassName/method_name" - matches method in class
        - Absolute path: "/ClassName/method_name" - exact match from root

        Args:
            name_path_pattern: Pattern to match (e.g., "MyClass/my_method")
            relative_path: Restrict search to this file or directory
            include_body: Whether to include symbol body in results
            depth: Depth of children to include (0 = symbol only)

        Returns:
            Dict with list of matching symbols
        """
        # Warn if relative_path is not provided
        if not relative_path:
            return {
                "success": False,
                "error": (
                    "relative_path is required for reliable symbol search. "
                    "Workspace-wide search is disabled due to timeout issues. "
                    "Please provide a file or directory path to limit the search scope."
                ),
                "hint": "Use relative_path parameter, e.g., relative_path='c4/lsp/provider.py'",
            }

        try:
            from c4.lsp.unified_provider import find_symbol_unified

            symbols = find_symbol_unified(
                name_path_pattern=name_path_pattern,
                relative_path=relative_path,
                include_body=include_body,
                project_path=str(self._daemon.root),
                timeout=30,
            )

            return {
                "success": True,
                "pattern": name_path_pattern,
                "relative_path": relative_path,
                "symbols": symbols,
                "count": len(symbols),
            }

        except ImportError:
            return {
                "success": False,
                "error": "LSP providers not available. Install with: uv add multilspy jedi",
            }
        except Exception as e:
            return {"success": False, "error": str(e)}

    def get_symbols_overview(
        self,
        relative_path: str,
        depth: int = 0,
    ) -> dict[str, Any]:
        """Get an overview of symbols in a file.

        This should be the first tool to call when you want to understand a new file,
        unless you already know what you are looking for.

        Args:
            relative_path: Path to the file (relative to project root)
            depth: Depth of children to include (0 = top-level only)

        Returns:
            Dictionary with symbols grouped by kind
        """
        try:
            from c4.lsp.unified_provider import get_symbols_overview_unified

            result = get_symbols_overview_unified(
                relative_path=relative_path,
                depth=depth,
                project_path=str(self._daemon.root),
                timeout=30,
            )

            if "error" in result:
                return {"success": False, "error": result["error"]}

            return {
                "success": True,
                **result,
            }

        except ImportError:
            return {
                "success": False,
                "error": "LSP providers not available. Install with: uv add multilspy jedi",
            }
        except Exception as e:
            return {"success": False, "error": str(e)}

    def replace_symbol_body(
        self,
        name_path: str,
        file_path: str | None,
        new_body: str,
    ) -> dict[str, Any]:
        """Replace the body of a symbol (function, class, method).

        Args:
            name_path: Symbol name or qualified name (e.g., "MyClass.method")
            file_path: File containing the symbol (optional for single-file search)
            new_body: New source code for the symbol body

        Returns:
            Dict with success status and details about the edit
        """
        symbol, symbol_file, error = self._get_symbol_by_name_path(name_path, file_path)
        if error:
            return {"success": False, "error": error}

        try:
            # Read the file
            file_path_obj = Path(symbol_file)
            content = file_path_obj.read_text(encoding="utf-8")
            lines = content.splitlines(keepends=True)

            # Get symbol location (1-indexed in Location)
            start_line = symbol.location.start_line - 1  # Convert to 0-indexed
            end_line = symbol.location.end_line - 1

            # Preserve leading indentation from original
            original_first_line = lines[start_line] if start_line < len(lines) else ""
            indent = len(original_first_line) - len(original_first_line.lstrip())
            indent_str = original_first_line[:indent]

            # Ensure new_body lines have proper indentation
            new_lines = new_body.splitlines(keepends=True)
            if new_lines and not new_lines[-1].endswith("\n"):
                new_lines[-1] += "\n"

            # Apply indentation to new body (except first line if it already has it)
            indented_lines = []
            for i, line in enumerate(new_lines):
                if i == 0 or not line.strip():
                    indented_lines.append(line)
                else:
                    indented_lines.append(indent_str + line.lstrip())

            # Replace the lines
            new_content_lines = (
                lines[:start_line] + indented_lines + lines[end_line + 1:]
            )
            new_content = "".join(new_content_lines)

            # Write back
            file_path_obj.write_text(new_content, encoding="utf-8")

            return {
                "success": True,
                "file_path": symbol_file,
                "symbol": name_path,
                "start_line": start_line + 1,
                "end_line": end_line + 1,
                "lines_replaced": end_line - start_line + 1,
                "new_lines": len(indented_lines),
            }

        except Exception as e:
            return {"success": False, "error": str(e)}

    def insert_before_symbol(
        self,
        name_path: str,
        file_path: str | None,
        content: str,
    ) -> dict[str, Any]:
        """Insert content before a symbol.

        Args:
            name_path: Symbol name or qualified name
            file_path: File containing the symbol
            content: Content to insert before the symbol

        Returns:
            Dict with success status and details
        """
        symbol, symbol_file, error = self._get_symbol_by_name_path(name_path, file_path)
        if error:
            return {"success": False, "error": error}

        try:
            file_path_obj = Path(symbol_file)
            file_content = file_path_obj.read_text(encoding="utf-8")
            lines = file_content.splitlines(keepends=True)

            # Get insertion point (line before the symbol)
            insert_line = symbol.location.start_line - 1  # 0-indexed

            # Ensure content ends with newline
            if content and not content.endswith("\n"):
                content += "\n"

            # Insert the content
            content_lines = content.splitlines(keepends=True)
            new_lines = lines[:insert_line] + content_lines + lines[insert_line:]
            new_content = "".join(new_lines)

            # Write back
            file_path_obj.write_text(new_content, encoding="utf-8")

            return {
                "success": True,
                "file_path": symbol_file,
                "symbol": name_path,
                "inserted_at_line": insert_line + 1,
                "lines_inserted": len(content_lines),
            }

        except Exception as e:
            return {"success": False, "error": str(e)}

    def insert_after_symbol(
        self,
        name_path: str,
        file_path: str | None,
        content: str,
    ) -> dict[str, Any]:
        """Insert content after a symbol.

        Args:
            name_path: Symbol name or qualified name
            file_path: File containing the symbol
            content: Content to insert after the symbol

        Returns:
            Dict with success status and details
        """
        symbol, symbol_file, error = self._get_symbol_by_name_path(name_path, file_path)
        if error:
            return {"success": False, "error": error}

        try:
            file_path_obj = Path(symbol_file)
            file_content = file_path_obj.read_text(encoding="utf-8")
            lines = file_content.splitlines(keepends=True)

            # Get insertion point (line after the symbol ends)
            insert_line = symbol.location.end_line

            # Ensure content starts with newline for separation and ends with newline
            if content and not content.startswith("\n"):
                content = "\n" + content
            if content and not content.endswith("\n"):
                content += "\n"

            # Insert the content
            content_lines = content.splitlines(keepends=True)
            new_lines = lines[:insert_line] + content_lines + lines[insert_line:]
            new_content = "".join(new_lines)

            # Write back
            file_path_obj.write_text(new_content, encoding="utf-8")

            return {
                "success": True,
                "file_path": symbol_file,
                "symbol": name_path,
                "inserted_at_line": insert_line + 1,
                "lines_inserted": len(content_lines),
            }

        except Exception as e:
            return {"success": False, "error": str(e)}

    def rename_symbol(
        self,
        name_path: str,
        file_path: str | None,
        new_name: str,
    ) -> dict[str, Any]:
        """Rename a symbol across the entire codebase.

        This finds all references to the symbol and renames them.

        Args:
            name_path: Current symbol name or qualified name
            file_path: File containing the symbol definition (optional)
            new_name: New name for the symbol

        Returns:
            Dict with success status and list of files modified
        """
        from c4.docs.analyzer import CodeAnalyzer

        try:
            # First, find the symbol definition
            symbol, symbol_file, error = self._get_symbol_by_name_path(
                name_path, file_path
            )
            if error:
                return {"success": False, "error": error}

            # Get the simple name (last part of qualified name)
            old_name = name_path.split(".")[-1]

            # Validate new name
            if not new_name.isidentifier():
                return {"success": False, "error": f"Invalid identifier: {new_name}"}

            # Create analyzer for finding references
            analyzer = CodeAnalyzer()
            analyzer.add_directory(
                self._daemon.root,
                recursive=True,
                exclude_patterns=[
                    "**/node_modules/**",
                    "**/__pycache__/**",
                    "**/.git/**",
                    "**/venv/**",
                    "**/.venv/**",
                    "**/.c4/**",
                    "**/.claude/**",
                ],
            )

            # Find all references
            references = analyzer.find_references(old_name)

            # Group references by file
            refs_by_file: dict[str, list] = {}
            for ref in references:
                fp = ref.location.file_path
                if fp not in refs_by_file:
                    refs_by_file[fp] = []
                refs_by_file[fp].append(ref)

            # Also include the definition file
            if symbol_file not in refs_by_file:
                refs_by_file[symbol_file] = []

            # Perform replacements file by file
            files_modified = []
            total_replacements = 0

            for fp in refs_by_file:
                try:
                    file_path_obj = Path(fp)
                    file_content = file_path_obj.read_text(encoding="utf-8")

                    # Use word boundary replacement to avoid partial matches
                    pattern = r"\b" + re.escape(old_name) + r"\b"
                    new_content, count = re.subn(pattern, new_name, file_content)

                    if count > 0:
                        file_path_obj.write_text(new_content, encoding="utf-8")
                        files_modified.append({
                            "file_path": fp,
                            "replacements": count,
                        })
                        total_replacements += count

                except Exception:
                    # Log but continue with other files
                    pass

            return {
                "success": True,
                "old_name": old_name,
                "new_name": new_name,
                "files_modified": files_modified,
                "total_files": len(files_modified),
                "total_replacements": total_replacements,
            }

        except Exception as e:
            return {"success": False, "error": str(e)}

    # =========================================================================
    # File Operations
    # =========================================================================

    def _get_file_tools(self):
        """Get or create FileTools instance."""
        if not hasattr(self, "_file_tools"):
            from c4.lsp.file_tools import FileTools
            self._file_tools = FileTools(self._daemon.root)
        return self._file_tools

    def read_file(
        self,
        relative_path: str,
        start_line: int = 0,
        end_line: int | None = None,
    ) -> dict[str, Any]:
        """Read a file or portion of it.

        Args:
            relative_path: Path relative to project root
            start_line: 0-based index of first line to read
            end_line: 0-based index of last line (inclusive), None for end

        Returns:
            Dictionary with content, total_lines, start_line, end_line
        """
        return self._get_file_tools().read_file(
            relative_path=relative_path,
            start_line=start_line,
            end_line=end_line,
        )

    def create_text_file(
        self,
        relative_path: str,
        content: str,
    ) -> dict[str, Any]:
        """Create or overwrite a text file.

        Args:
            relative_path: Path relative to project root
            content: Content to write

        Returns:
            Dictionary with success status and message
        """
        return self._get_file_tools().create_text_file(
            relative_path=relative_path,
            content=content,
        )

    def list_dir(
        self,
        relative_path: str = ".",
        recursive: bool = False,
    ) -> dict[str, Any]:
        """List files and directories.

        Args:
            relative_path: Path relative to project root
            recursive: Whether to scan subdirectories

        Returns:
            Dictionary with directories and files lists
        """
        return self._get_file_tools().list_dir(
            relative_path=relative_path,
            recursive=recursive,
        )

    def find_file(
        self,
        file_mask: str,
        relative_path: str = ".",
    ) -> dict[str, Any]:
        """Find files matching a pattern.

        Args:
            file_mask: Filename or glob pattern
            relative_path: Directory to search in

        Returns:
            Dictionary with matches list
        """
        return self._get_file_tools().find_file(
            file_mask=file_mask,
            relative_path=relative_path,
        )

    def search_for_pattern(
        self,
        pattern: str,
        relative_path: str = ".",
        glob_pattern: str | None = None,
        context_lines: int = 0,
    ) -> dict[str, Any]:
        """Search for a regex pattern in files.

        Args:
            pattern: Regular expression pattern
            relative_path: Directory or file to search in
            glob_pattern: Optional glob to filter files
            context_lines: Number of context lines before/after match

        Returns:
            Dictionary with matches list
        """
        return self._get_file_tools().search_for_pattern(
            pattern=pattern,
            relative_path=relative_path,
            glob_pattern=glob_pattern,
            context_lines=context_lines,
        )

    def replace_content(
        self,
        relative_path: str,
        needle: str,
        replacement: str,
        mode: str = "literal",
        allow_multiple: bool = False,
    ) -> dict[str, Any]:
        """Replace content in a file.

        Args:
            relative_path: Path relative to project root
            needle: String or regex pattern to search for
            replacement: Replacement string
            mode: 'literal' for exact match, 'regex' for regex
            allow_multiple: Whether to allow multiple replacements

        Returns:
            Dictionary with success status and replacements_made count
        """
        return self._get_file_tools().replace_content(
            relative_path=relative_path,
            needle=needle,
            replacement=replacement,
            mode=mode,
            allow_multiple=allow_multiple,
        )
