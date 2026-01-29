"""Context Slimmer - Compress large outputs to save LLM context"""

import json
import re
from dataclasses import dataclass
from typing import Any


@dataclass
class SlimResult:
    """Result of a slim operation"""

    content: str
    original_size: int
    slimmed_size: int
    compression_ratio: float


class ContextSlimmer:
    """
    Utility class for compressing large outputs to save LLM context.

    Provides methods to:
    - Compress logs by extracting error patterns
    - Truncate large JSON arrays
    - Extract function signatures from code
    """

    # Error patterns to extract from logs
    ERROR_PATTERNS = [
        r"(?i)(error|exception|fail|fatal|critical).*",
        r"(?i)traceback.*",
        r"(?i)^\s*File \".*\", line \d+.*",
        r"(?i)^\s*\^+\s*$",  # Python syntax error markers
        r"(?i)assert(ion)?.*failed.*",
        r"(?i)FAILED.*",
        r"(?i)E\s+.*",  # pytest error lines
    ]

    @classmethod
    def slim_log(
        cls,
        log: str,
        max_lines: int = 50,
        context_lines: int = 2,
        include_summary: bool = True,
    ) -> str:
        """
        Slim a log output by extracting error patterns and relevant context.

        Args:
            log: The log content to slim
            max_lines: Maximum number of lines in the result
            context_lines: Number of context lines before/after errors
            include_summary: Whether to include a summary header

        Returns:
            Slimmed log content
        """
        lines = log.split("\n")
        original_count = len(lines)

        if original_count <= max_lines:
            return log

        # Find lines matching error patterns
        error_indices: set[int] = set()
        for i, line in enumerate(lines):
            for pattern in cls.ERROR_PATTERNS:
                if re.search(pattern, line):
                    # Add the line and context
                    for j in range(
                        max(0, i - context_lines), min(original_count, i + context_lines + 1)
                    ):
                        error_indices.add(j)
                    break

        # If no errors found, just take head and tail
        if not error_indices:
            head_count = max_lines // 2
            tail_count = max_lines - head_count - 1
            result_lines = (
                lines[:head_count]
                + [f"... ({original_count - max_lines} lines omitted) ..."]
                + lines[-tail_count:]
            )
        else:
            # Sort and limit error lines
            sorted_indices = sorted(error_indices)
            if len(sorted_indices) > max_lines:
                sorted_indices = sorted_indices[:max_lines]

            result_lines = []
            last_idx = -2
            for idx in sorted_indices:
                if idx > last_idx + 1:
                    result_lines.append("...")
                result_lines.append(lines[idx])
                last_idx = idx

        result = "\n".join(result_lines)

        if include_summary:
            summary = f"[Slimmed: {original_count} -> {len(result_lines)} lines]"
            result = f"{summary}\n{result}"

        return result

    @classmethod
    def slim_json(
        cls,
        data: Any,
        max_array_items: int = 5,
        max_string_length: int = 200,
        max_depth: int = 5,
    ) -> Any:
        """
        Slim a JSON-serializable object by truncating arrays and long strings.

        Args:
            data: The data to slim
            max_array_items: Maximum items in arrays (shows first and last)
            max_string_length: Maximum string length
            max_depth: Maximum nesting depth

        Returns:
            Slimmed data structure
        """
        return cls._slim_value(data, max_array_items, max_string_length, max_depth, 0)

    @classmethod
    def _slim_value(
        cls,
        value: Any,
        max_array_items: int,
        max_string_length: int,
        max_depth: int,
        current_depth: int,
    ) -> Any:
        """Recursively slim a value"""
        if current_depth >= max_depth:
            if isinstance(value, dict):
                return f"{{...{len(value)} keys...}}"
            elif isinstance(value, list):
                return f"[...{len(value)} items...]"
            else:
                return value

        if isinstance(value, dict):
            return {
                k: cls._slim_value(
                    v, max_array_items, max_string_length, max_depth, current_depth + 1
                )
                for k, v in value.items()
            }

        elif isinstance(value, list):
            if len(value) <= max_array_items:
                return [
                    cls._slim_value(
                        item, max_array_items, max_string_length, max_depth, current_depth + 1
                    )
                    for item in value
                ]
            else:
                # Show first few and last few with omission marker
                half = max_array_items // 2
                head = [
                    cls._slim_value(
                        item, max_array_items, max_string_length, max_depth, current_depth + 1
                    )
                    for item in value[:half]
                ]
                tail = [
                    cls._slim_value(
                        item, max_array_items, max_string_length, max_depth, current_depth + 1
                    )
                    for item in value[-half:] if half > 0
                ]
                omitted = len(value) - max_array_items
                return head + [f"...({omitted} items omitted)..."] + tail

        elif isinstance(value, str):
            if len(value) > max_string_length:
                half = max_string_length // 2
                omitted = len(value) - max_string_length
                return f"{value[:half]}...({omitted} chars)...{value[-half:]}"
            return value

        else:
            return value

    @classmethod
    def slim_code(
        cls,
        code: str,
        max_lines: int = 30,
        extract_signatures: bool = True,
    ) -> str:
        """
        Slim code by extracting only signatures and key structures.

        Args:
            code: The code content
            max_lines: Maximum number of lines
            extract_signatures: Whether to extract function/class signatures

        Returns:
            Slimmed code with signatures only
        """
        lines = code.split("\n")

        if len(lines) <= max_lines:
            return code

        if not extract_signatures:
            # Simple truncation
            return "\n".join(lines[: max_lines - 1]) + "\n... (truncated)"

        # Extract signatures (Python-focused)
        signature_patterns = [
            r"^(class\s+\w+.*?):$",  # Class definitions
            r"^(\s*def\s+\w+.*?):$",  # Function definitions
            r"^(\s*async\s+def\s+\w+.*?):$",  # Async function definitions
            r"^(import\s+.*)$",  # Imports
            r"^(from\s+.*)$",  # From imports
            r"^(@\w+.*)$",  # Decorators
        ]

        signatures = []
        in_multiline_signature = False
        current_signature = []

        for line in lines:
            # Handle multiline signatures (e.g., long function definitions)
            if in_multiline_signature:
                current_signature.append(line)
                if line.rstrip().endswith(":"):
                    signatures.append("\n".join(current_signature))
                    in_multiline_signature = False
                    current_signature = []
                continue

            # Check for signature patterns
            for pattern in signature_patterns:
                if re.match(pattern, line):
                    if line.rstrip().endswith("(") or line.rstrip().endswith(","):
                        in_multiline_signature = True
                        current_signature = [line]
                    else:
                        signatures.append(line)
                    break

        if len(signatures) == 0:
            # Fallback to simple truncation
            return "\n".join(lines[: max_lines - 1]) + "\n... (truncated)"

        result = f"[Code signatures ({len(lines)} lines total)]\n"
        result += "\n".join(signatures[:max_lines])

        if len(signatures) > max_lines:
            result += f"\n... ({len(signatures) - max_lines} more signatures)"

        return result

    @classmethod
    def format_validation_error(cls, error_output: str, max_chars: int = 2000) -> str:
        """
        Format validation error output for display.

        Extracts key error information and truncates if needed.

        Args:
            error_output: The raw error output
            max_chars: Maximum characters to return

        Returns:
            Formatted error output
        """
        if len(error_output) <= max_chars:
            return error_output

        # Calculate max lines based on average line length
        avg_line_length = 80
        max_lines = max_chars // avg_line_length

        return cls.slim_log(error_output, max_lines=max_lines, include_summary=True)

    @classmethod
    def slim(cls, content: str | dict | list, max_size: int = 2000) -> str | Any:
        """
        Auto-detect content type and apply appropriate slimming.

        Args:
            content: Content to slim (string, dict, or list)
            max_size: Target maximum size

        Returns:
            Slimmed content
        """
        if isinstance(content, dict) or isinstance(content, list):
            slimmed = cls.slim_json(content, max_array_items=5)
            result = json.dumps(slimmed, indent=2, ensure_ascii=False)
            if len(result) > max_size:
                # Further reduce array items
                slimmed = cls.slim_json(content, max_array_items=3, max_string_length=100)
                result = json.dumps(slimmed, indent=2, ensure_ascii=False)
            return result

        elif isinstance(content, str):
            if len(content) <= max_size:
                return content

            # Try to detect if it's JSON
            try:
                data = json.loads(content)
                return cls.slim(data, max_size)
            except json.JSONDecodeError:
                pass

            # Try code extraction
            if "\ndef " in content or "\nclass " in content:
                return cls.slim_code(content, max_lines=max_size // 80)

            # Default to log slimming
            return cls.slim_log(content, max_lines=max_size // 80)

        return content
