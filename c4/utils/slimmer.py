"""Context Slimmer - Utility for compressing large text contexts."""

import re
import json
from typing import Any

class ContextSlimmer:
    @staticmethod
    def slim_log(text: str, max_lines: int = 100) -> str:
        """Slim down a log file by keeping only error contexts and headers."""
        lines = text.splitlines()
        if len(lines) <= max_lines:
            return text

        # Patterns that indicate important info
        important_patterns = [
            r"ERROR", r"FAIL", r"FAILED", r"Exception", r"Traceback", 
            r"[\d+/\d+]", r"step \d+", r"Building", r"Summary"
        ] # Note: The original regex patterns were already correctly escaped for Python raw strings.
        
        keep_indices = set()
        
        # Always keep first 10 and last 10 lines
        for i in range(min(10, len(lines))):
            keep_indices.add(i)
        for i in range(max(0, len(lines) - 10), len(lines)):
            keep_indices.add(i)
            
        # Keep lines matching important patterns + context
        for i, line in enumerate(lines):
            if any(re.search(p, line, re.IGNORECASE) for p in important_patterns):
                # Keep 2 lines before and 5 lines after for context
                for j in range(max(0, i-2), min(len(lines), i+6)):
                    keep_indices.add(j)
        
        result_lines = []
        last_idx = -1
        for i in sorted(list(keep_indices)):
            if last_idx != -1 and i > last_idx + 1:
                result_lines.append(f"\n... (truncated {i - last_idx - 1} lines) ...\n")
            result_lines.append(lines[i])
            last_idx = i
            
        return "\n".join(result_lines)

    @staticmethod
    def slim_json(data: Any, max_list_len: int = 5) -> str:
        """Truncate large lists in JSON to save space."""
        def _slim(obj):
            if isinstance(obj, list):
                if len(obj) > max_list_len:
                    return [_slim(item) for item in obj[:max_list_len]] + [f"... ({len(obj) - max_list_len} more items) ..."]
                return [_slim(item) for item in obj]
            elif isinstance(obj, dict):
                return {k: _slim(v) for k, v in obj.items()}
            return obj
            
        slimmed = _slim(data)
        return json.dumps(slimmed, indent=2, default=str)

    @staticmethod
    def slim_code(code: str) -> str:
        """Extract only signatures (class/def) from code."""
        lines = code.splitlines()
        signatures = []
        for line in lines:
            # Match class or function definitions
            if re.match(r"^\s*(class|def|async def)\s+\w+", line):
                signatures.append(line.rstrip())
            elif re.match(r"^import\s+|from\s+\w+\s+import", line):
                # Optional: keep imports? Let's skip for now to save max tokens
                pass
                
        if not signatures:
            return code[:1000] + "\n... (content truncated) ..."
            
        return "\n".join(signatures) + "\n\n... (bodies omitted for slimming) ..."
