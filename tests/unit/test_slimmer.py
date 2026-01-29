"""Tests for c4.utils.slimmer module"""

import json

import pytest

from c4.utils.slimmer import ContextSlimmer, SlimResult


class TestSlimLog:
    """Tests for slim_log method"""

    def test_short_log_unchanged(self):
        """Test that short logs are returned unchanged"""
        log = "Line 1\nLine 2\nLine 3"
        result = ContextSlimmer.slim_log(log, max_lines=10)
        assert log in result

    def test_long_log_truncated(self):
        """Test that long logs are truncated"""
        lines = [f"Line {i}" for i in range(100)]
        log = "\n".join(lines)
        result = ContextSlimmer.slim_log(log, max_lines=20)
        assert len(result.split("\n")) < 100

    def test_error_patterns_preserved(self):
        """Test that error patterns are preserved"""
        log = """
Starting test...
Running setup...
ERROR: Something went wrong
More lines here
Another line
Final line
"""
        result = ContextSlimmer.slim_log(log, max_lines=5)
        assert "ERROR" in result

    def test_traceback_preserved(self):
        """Test that traceback lines are preserved"""
        log = """
Starting...
Traceback (most recent call last):
  File "test.py", line 10, in test
    raise ValueError("Test error")
ValueError: Test error
More output...
"""
        result = ContextSlimmer.slim_log(log, max_lines=10)
        assert "Traceback" in result
        assert "ValueError" in result

    def test_pytest_errors_preserved(self):
        """Test that pytest error patterns are preserved"""
        log = """
===== test session starts =====
collected 5 items
FAILED tests/test_foo.py::test_bar
E   AssertionError: expected True
"""
        result = ContextSlimmer.slim_log(log, max_lines=10)
        assert "FAILED" in result
        assert "AssertionError" in result

    def test_summary_header(self):
        """Test that summary header is included"""
        lines = [f"Line {i}" for i in range(100)]
        log = "\n".join(lines)
        result = ContextSlimmer.slim_log(log, max_lines=20, include_summary=True)
        assert "[Slimmed:" in result

    def test_no_summary_header(self):
        """Test that summary header can be disabled"""
        lines = [f"Line {i}" for i in range(100)]
        log = "\n".join(lines)
        result = ContextSlimmer.slim_log(log, max_lines=20, include_summary=False)
        assert "[Slimmed:" not in result


class TestSlimJson:
    """Tests for slim_json method"""

    def test_small_json_unchanged(self):
        """Test that small JSON is unchanged"""
        data = {"a": 1, "b": 2}
        result = ContextSlimmer.slim_json(data)
        assert result == data

    def test_array_truncated(self):
        """Test that large arrays are truncated"""
        data = {"items": list(range(100))}
        result = ContextSlimmer.slim_json(data, max_array_items=5)
        assert len(result["items"]) < 100
        assert "omitted" in str(result["items"])

    def test_nested_arrays(self):
        """Test nested array truncation"""
        data = {"level1": {"level2": list(range(50))}}
        result = ContextSlimmer.slim_json(data, max_array_items=5, max_depth=5)
        assert "omitted" in str(result["level1"]["level2"])

    def test_long_strings_truncated(self):
        """Test that long strings are truncated"""
        data = {"text": "a" * 500}
        result = ContextSlimmer.slim_json(data, max_string_length=100)
        assert len(result["text"]) < 500
        assert "chars" in result["text"]

    def test_max_depth(self):
        """Test max depth handling"""
        data = {"l1": {"l2": {"l3": {"l4": {"l5": {"l6": "deep"}}}}}}
        result = ContextSlimmer.slim_json(data, max_depth=3)
        # At depth 3, nested structures should be summarized
        assert "..." in str(result["l1"]["l2"]["l3"])

    def test_primitive_values_preserved(self):
        """Test that primitive values are preserved"""
        data = {"int": 42, "float": 3.14, "bool": True, "null": None}
        result = ContextSlimmer.slim_json(data)
        assert result["int"] == 42
        assert result["float"] == 3.14
        assert result["bool"] is True
        assert result["null"] is None


class TestSlimCode:
    """Tests for slim_code method"""

    def test_short_code_unchanged(self):
        """Test that short code is returned unchanged"""
        code = """def foo():
    pass

def bar():
    pass
"""
        result = ContextSlimmer.slim_code(code, max_lines=50)
        assert code in result

    def test_signatures_extracted(self):
        """Test that function signatures are extracted"""
        code = """
import os
from pathlib import Path

class MyClass:
    def __init__(self):
        self.x = 1
        self.y = 2
        # many lines...

    def method(self, arg):
        # implementation
        return arg * 2

def standalone_function():
    # more code
    pass
"""
        result = ContextSlimmer.slim_code(code, max_lines=10, extract_signatures=True)
        assert "import os" in result
        assert "class MyClass" in result
        assert "def __init__" in result
        assert "def method" in result
        assert "def standalone_function" in result
        # Implementation details should not be in result
        assert "self.x = 1" not in result

    def test_async_functions(self):
        """Test that async functions are extracted"""
        code = """
async def async_function():
    await something()
    # lots of code
    return result
"""
        result = ContextSlimmer.slim_code(code, max_lines=5, extract_signatures=True)
        assert "async def async_function" in result

    def test_decorators_extracted(self):
        """Test that decorators are extracted"""
        code = """
@decorator
def decorated():
    pass

@property
def my_prop(self):
    return self._value
"""
        result = ContextSlimmer.slim_code(code, max_lines=10, extract_signatures=True)
        assert "@decorator" in result
        assert "@property" in result


class TestFormatValidationError:
    """Tests for format_validation_error method"""

    def test_short_error_unchanged(self):
        """Test that short errors are returned unchanged"""
        error = "Error: something failed"
        result = ContextSlimmer.format_validation_error(error, max_chars=1000)
        assert error in result

    def test_long_error_compressed(self):
        """Test that long errors are compressed"""
        # Create a really long error with many lines
        error_lines = ["Line " + str(i) for i in range(200)]
        error = "ERROR: test failed\n" + "\n".join(error_lines) + "\nFinal error"
        result = ContextSlimmer.format_validation_error(error, max_chars=500)
        # Result should be shorter or contain slimmed indicator
        assert len(result) < len(error) or "[Slimmed:" in result
        assert "ERROR" in result


class TestSlimAuto:
    """Tests for the auto-detect slim method"""

    def test_dict_handling(self):
        """Test dict auto-detection"""
        data = {"items": list(range(100))}
        result = ContextSlimmer.slim(data, max_size=500)
        assert isinstance(result, str)  # Returns JSON string
        assert "omitted" in result

    def test_list_handling(self):
        """Test list auto-detection"""
        data = list(range(100))
        result = ContextSlimmer.slim(data, max_size=200)
        assert isinstance(result, str)

    def test_json_string_handling(self):
        """Test JSON string auto-detection"""
        # Make it larger so it exceeds max_size
        data = json.dumps({"items": list(range(500))})
        result = ContextSlimmer.slim(data, max_size=200)
        # Either slimmed or contains truncation indicator
        assert len(result) < len(data) or "omitted" in result

    def test_code_string_handling(self):
        """Test code string auto-detection"""
        code = "\ndef foo():\n    pass\n" * 50
        result = ContextSlimmer.slim(code, max_size=500)
        assert "def foo" in result

    def test_log_string_handling(self):
        """Test log string auto-detection (fallback)"""
        log = "INFO: Starting\nERROR: Failed\n" + "log line\n" * 100
        result = ContextSlimmer.slim(log, max_size=500)
        assert "ERROR" in result
