"""Tests for Result type."""

import pytest

from c4.core.result import Err, Ok, collect, from_exception, try_call


class TestOk:
    """Tests for Ok type."""

    def test_is_ok_returns_true(self) -> None:
        result = Ok(42)
        assert result.is_ok() is True

    def test_is_err_returns_false(self) -> None:
        result = Ok(42)
        assert result.is_err() is False

    def test_unwrap_returns_value(self) -> None:
        result = Ok(42)
        assert result.unwrap() == 42

    def test_unwrap_or_returns_value(self) -> None:
        result = Ok(42)
        assert result.unwrap_or(0) == 42

    def test_unwrap_or_else_returns_value(self) -> None:
        result = Ok(42)
        assert result.unwrap_or_else(lambda: 0) == 42

    def test_map_applies_function(self) -> None:
        result = Ok(21)
        mapped = result.map(lambda x: x * 2)
        assert mapped.unwrap() == 42

    def test_map_err_returns_self(self) -> None:
        result = Ok(42)
        mapped = result.map_err(lambda e: f"Error: {e}")
        assert mapped.unwrap() == 42

    def test_and_then_chains_function(self) -> None:
        result = Ok(21)
        chained = result.and_then(lambda x: Ok(x * 2))
        assert chained.unwrap() == 42

    def test_and_then_propagates_error(self) -> None:
        result = Ok(21)
        chained = result.and_then(lambda x: Err("failed"))
        assert chained.is_err()

    def test_to_dict(self) -> None:
        result = Ok(42)
        d = result.to_dict()
        assert d == {"ok": True, "value": 42}

    def test_repr(self) -> None:
        result = Ok(42)
        assert repr(result) == "Ok(42)"

    def test_ok_with_none_value(self) -> None:
        result = Ok(None)
        assert result.is_ok()
        assert result.unwrap() is None

    def test_ok_with_complex_value(self) -> None:
        data = {"key": [1, 2, 3]}
        result = Ok(data)
        assert result.unwrap() == data


class TestErr:
    """Tests for Err type."""

    def test_is_ok_returns_false(self) -> None:
        result = Err("failed")
        assert result.is_ok() is False

    def test_is_err_returns_true(self) -> None:
        result = Err("failed")
        assert result.is_err() is True

    def test_unwrap_raises_value_error(self) -> None:
        result = Err("something went wrong")
        with pytest.raises(ValueError, match="something went wrong"):
            result.unwrap()

    def test_unwrap_with_cause_chains_exception(self) -> None:
        cause = RuntimeError("original error")
        result = Err("wrapped error", cause=cause)
        with pytest.raises(ValueError, match="wrapped error") as exc_info:
            result.unwrap()
        assert exc_info.value.__cause__ is cause

    def test_unwrap_or_returns_default(self) -> None:
        result: Err[int] = Err("failed")
        assert result.unwrap_or(42) == 42

    def test_unwrap_or_else_calls_function(self) -> None:
        result: Err[int] = Err("failed")
        assert result.unwrap_or_else(lambda: 42) == 42

    def test_map_returns_self(self) -> None:
        result: Err[int] = Err("failed")
        mapped = result.map(lambda x: x * 2)
        assert mapped.is_err()
        assert mapped.error == "failed"  # type: ignore

    def test_map_err_applies_function(self) -> None:
        result: Err[int] = Err("failed")
        mapped = result.map_err(lambda e: f"Error: {e}")
        assert mapped.is_err()
        assert mapped.error == "Error: failed"  # type: ignore

    def test_and_then_returns_self(self) -> None:
        result: Err[int] = Err("failed")
        chained = result.and_then(lambda x: Ok(x * 2))
        assert chained.is_err()

    def test_to_dict_basic(self) -> None:
        result = Err("failed")
        d = result.to_dict()
        assert d == {"ok": False, "error": "failed"}

    def test_to_dict_with_code(self) -> None:
        result = Err("failed", code="E001")
        d = result.to_dict()
        assert d == {"ok": False, "error": "failed", "code": "E001"}

    def test_to_dict_with_details(self) -> None:
        result = Err("failed", details={"field": "name"})
        d = result.to_dict()
        assert d == {"ok": False, "error": "failed", "details": {"field": "name"}}

    def test_to_dict_full(self) -> None:
        result = Err("failed", code="E001", details={"field": "name"})
        d = result.to_dict()
        assert d == {
            "ok": False,
            "error": "failed",
            "code": "E001",
            "details": {"field": "name"},
        }

    def test_repr_basic(self) -> None:
        result = Err("failed")
        assert repr(result) == "Err(error='failed')"

    def test_repr_with_code(self) -> None:
        result = Err("failed", code="E001")
        assert repr(result) == "Err(error='failed', code='E001')"

    def test_repr_full(self) -> None:
        result = Err("failed", code="E001", details={"x": 1})
        assert "error='failed'" in repr(result)
        assert "code='E001'" in repr(result)
        assert "details=" in repr(result)


class TestFromException:
    """Tests for from_exception helper."""

    def test_from_exception_basic(self) -> None:
        exc = ValueError("bad value")
        result = from_exception(exc)
        assert result.is_err()
        assert result.error == "bad value"
        assert result.code == "ValueError"
        assert result.cause is exc

    def test_from_exception_with_custom_code(self) -> None:
        exc = RuntimeError("runtime error")
        result = from_exception(exc, code="CUSTOM_CODE")
        assert result.code == "CUSTOM_CODE"

    def test_from_exception_preserves_cause(self) -> None:
        exc = KeyError("missing key")
        result = from_exception(exc)
        assert result.cause is exc


class TestTryCall:
    """Tests for try_call helper."""

    def test_try_call_success(self) -> None:
        result = try_call(lambda: 42)
        assert result.is_ok()
        assert result.unwrap() == 42

    def test_try_call_failure(self) -> None:
        result = try_call(lambda: int("not a number"))
        assert result.is_err()
        assert "invalid literal" in result.error  # type: ignore

    def test_try_call_with_custom_code(self) -> None:
        result = try_call(lambda: 1 / 0, code="DIVISION_ERROR")
        assert result.is_err()
        assert result.code == "DIVISION_ERROR"  # type: ignore


class TestCollect:
    """Tests for collect helper."""

    def test_collect_all_ok(self) -> None:
        results = [Ok(1), Ok(2), Ok(3)]
        collected = collect(results)
        assert collected.is_ok()
        assert collected.unwrap() == [1, 2, 3]

    def test_collect_with_error(self) -> None:
        results = [Ok(1), Err("failed"), Ok(3)]
        collected = collect(results)
        assert collected.is_err()
        assert collected.error == "failed"  # type: ignore

    def test_collect_empty(self) -> None:
        results: list[Ok[int] | Err[int]] = []
        collected = collect(results)
        assert collected.is_ok()
        assert collected.unwrap() == []

    def test_collect_returns_first_error(self) -> None:
        results = [Ok(1), Err("first"), Err("second")]
        collected = collect(results)
        assert collected.is_err()
        assert collected.error == "first"  # type: ignore


class TestResultUsagePatterns:
    """Tests for common usage patterns."""

    def test_divide_function_pattern(self) -> None:
        def divide(a: int, b: int) -> Ok[float] | Err[float]:
            if b == 0:
                return Err("Division by zero", code="DIVISION_BY_ZERO")
            return Ok(a / b)

        result = divide(10, 2)
        assert result.is_ok()
        assert result.unwrap() == 5.0

        result = divide(10, 0)
        assert result.is_err()
        assert result.code == "DIVISION_BY_ZERO"  # type: ignore

    def test_chaining_pattern(self) -> None:
        def parse_int(s: str) -> Ok[int] | Err[int]:
            try:
                return Ok(int(s))
            except ValueError:
                return Err(f"Invalid integer: {s}")

        def double(x: int) -> Ok[int] | Err[int]:
            return Ok(x * 2)

        result = parse_int("21").and_then(double)
        assert result.unwrap() == 42

        result = parse_int("not a number").and_then(double)
        assert result.is_err()

    def test_dict_serialization_roundtrip(self) -> None:
        ok_result = Ok({"data": [1, 2, 3]})
        ok_dict = ok_result.to_dict()
        assert ok_dict["ok"] is True
        assert ok_dict["value"] == {"data": [1, 2, 3]}

        err_result = Err("validation failed", code="VALIDATION_ERROR", details={"field": "email"})
        err_dict = err_result.to_dict()
        assert err_dict["ok"] is False
        assert err_dict["error"] == "validation failed"
        assert err_dict["code"] == "VALIDATION_ERROR"
        assert err_dict["details"] == {"field": "email"}
