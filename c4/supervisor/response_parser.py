"""Response Parser - Shared parsing logic for supervisor backends."""

import json
import re

from .backend import SupervisorResponse


class ResponseParser:
    """Parses LLM output into SupervisorResponse.

    Tries multiple strategies to extract JSON:
    1. JSON in ```json code block
    2. Raw JSON with "decision" key
    3. Entire output as JSON
    """

    @staticmethod
    def parse(output: str) -> SupervisorResponse:
        """
        Parse supervisor decision from LLM output.

        Args:
            output: Raw LLM response text

        Returns:
            SupervisorResponse with parsed decision

        Raises:
            ValueError: If no valid JSON found
        """
        # Method 1: Try to find JSON in code block
        match = re.search(r"```json\s*(\{.*?\})\s*```", output, re.DOTALL)
        if match:
            try:
                data = json.loads(match.group(1))
                return SupervisorResponse.from_dict(data)
            except (json.JSONDecodeError, ValueError):
                pass

        # Method 2: Try to find raw JSON with decision key
        match = re.search(r'\{\s*"decision"\s*:.*?\}', output, re.DOTALL)
        if match:
            try:
                json_str = ResponseParser._extract_json_object(output, match.start())
                data = json.loads(json_str)
                return SupervisorResponse.from_dict(data)
            except (json.JSONDecodeError, ValueError):
                pass

        # Method 3: Try to parse the entire output as JSON
        try:
            data = json.loads(output.strip())
            return SupervisorResponse.from_dict(data)
        except (json.JSONDecodeError, ValueError):
            pass

        raise ValueError(f"No valid JSON found in output: {output[:200]}...")

    @staticmethod
    def _extract_json_object(text: str, start: int, max_length: int = 10000) -> str:
        """
        Extract a complete JSON object starting at given position.

        Handles nested braces and string escaping.

        Args:
            text: Full text containing JSON
            start: Starting position of JSON object
            max_length: Maximum characters to scan

        Returns:
            Complete JSON object string
        """
        depth = 0
        in_string = False
        escape = False
        result = []

        for char in text[start : start + max_length]:
            result.append(char)

            if escape:
                escape = False
                continue

            if char == "\\":
                escape = True
                continue

            if char == '"' and not escape:
                in_string = not in_string
                continue

            if in_string:
                continue

            if char == "{":
                depth += 1
            elif char == "}":
                depth -= 1
                if depth == 0:
                    return "".join(result)

        return "".join(result)
