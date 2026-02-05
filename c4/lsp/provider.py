"""LSP provider for symbol extraction.

Provides a simple interface for extracting symbols from files
using the CodeAnalyzer.
"""

from pathlib import Path

from c4.docs.analyzer import CodeAnalyzer, Symbol


def extract_symbols_from_file(file_path: str) -> list[Symbol]:
    """Extract symbols from a Python file.

    Args:
        file_path: Path to the file

    Returns:
        List of Symbol objects

    Raises:
        FileNotFoundError: If file doesn't exist
        Exception: If parsing fails
    """
    path = Path(file_path)
    if not path.exists():
        raise FileNotFoundError(f"File not found: {file_path}")

    analyzer = CodeAnalyzer()
    analyzer.add_file(str(path))
    return analyzer.get_file_symbols(str(path))
