"""Result type for standardized error handling.

This module provides a Result[T] type inspired by Rust's Result<T, E> pattern.
It enables explicit error handling without exceptions for operations that may fail.

Usage:
    from c4.core import Result, Ok, Err

    def divide(a: int, b: int) -> Result[float]:
        if b == 0:
            return Err("Division by zero", code="DIVISION_BY_ZERO")
        return Ok(a / b)

    result = divide(10, 2)
    if result.is_ok():
        print(f"Result: {result.unwrap()}")
    else:
        print(f"Error: {result.error}")

    # Or use pattern matching (Python 3.10+)
    match result:
        case Ok(value):
            print(f"Success: {value}")
        case Err(error):
            print(f"Failed: {error}")
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Callable, Generic, TypeVar, Union

T = TypeVar("T")
U = TypeVar("U")


@dataclass(frozen=True, slots=True)
class Ok(Generic[T]):
    """Represents a successful result containing a value."""

    value: T

    def is_ok(self) -> bool:
        return True

    def is_err(self) -> bool:
        return False

    def unwrap(self) -> T:
        """Returns the contained value."""
        return self.value

    def unwrap_or(self, default: T) -> T:
        """Returns the contained value (ignores default)."""
        return self.value

    def unwrap_or_else(self, f: Callable[[], T]) -> T:
        """Returns the contained value (ignores function)."""
        return self.value

    def map(self, f: Callable[[T], U]) -> Result[U]:
        """Applies function to the contained value."""
        return Ok(f(self.value))

    def map_err(self, f: Callable[[str], str]) -> Result[T]:
        """Returns self (no error to map)."""
        return self

    def and_then(self, f: Callable[[T], Result[U]]) -> Result[U]:
        """Chains another Result-returning function."""
        return f(self.value)

    def to_dict(self) -> dict[str, Any]:
        """Converts to dictionary for JSON serialization."""
        return {
            "ok": True,
            "value": self.value,
        }

    def __repr__(self) -> str:
        return f"Ok({self.value!r})"


@dataclass(frozen=True, slots=True)
class Err(Generic[T]):
    """Represents a failed result containing an error message."""

    error: str
    code: str | None = None
    details: dict[str, Any] = field(default_factory=dict)
    cause: Exception | None = field(default=None, compare=False)

    def is_ok(self) -> bool:
        return False

    def is_err(self) -> bool:
        return True

    def unwrap(self) -> T:
        """Raises an exception since this is an error."""
        if self.cause:
            raise ValueError(f"{self.error}: {self.cause}") from self.cause
        raise ValueError(self.error)

    def unwrap_or(self, default: T) -> T:
        """Returns the default value."""
        return default

    def unwrap_or_else(self, f: Callable[[], T]) -> T:
        """Returns the result of calling f."""
        return f()

    def map(self, f: Callable[[T], U]) -> Result[U]:
        """Returns self (no value to map)."""
        return self  # type: ignore

    def map_err(self, f: Callable[[str], str]) -> Result[T]:
        """Applies function to the error message."""
        return Err(f(self.error), code=self.code, details=self.details, cause=self.cause)

    def and_then(self, f: Callable[[T], Result[U]]) -> Result[U]:
        """Returns self (no value to chain)."""
        return self  # type: ignore

    def to_dict(self) -> dict[str, Any]:
        """Converts to dictionary for JSON serialization."""
        result: dict[str, Any] = {
            "ok": False,
            "error": self.error,
        }
        if self.code:
            result["code"] = self.code
        if self.details:
            result["details"] = self.details
        return result

    def __repr__(self) -> str:
        parts = [f"error={self.error!r}"]
        if self.code:
            parts.append(f"code={self.code!r}")
        if self.details:
            parts.append(f"details={self.details!r}")
        return f"Err({', '.join(parts)})"


# Type alias for Result
Result = Union[Ok[T], Err[T]]


def from_exception(e: Exception, code: str | None = None) -> Err[Any]:
    """Creates an Err from an exception.

    Args:
        e: The exception to convert
        code: Optional error code

    Returns:
        An Err containing the exception message and reference
    """
    return Err(
        error=str(e),
        code=code or type(e).__name__,
        cause=e,
    )


def try_call(f: Callable[[], T], code: str | None = None) -> Result[T]:
    """Wraps a function call in a Result.

    Args:
        f: Function to call
        code: Optional error code if exception occurs

    Returns:
        Ok(result) if successful, Err if exception raised

    Example:
        result = try_call(lambda: int("not a number"))
        # Returns Err(error="invalid literal...", code="ValueError")
    """
    try:
        return Ok(f())
    except Exception as e:
        return from_exception(e, code)


def collect(results: list[Result[T]]) -> Result[list[T]]:
    """Collects a list of Results into a Result of list.

    If any Result is an Err, returns the first Err.
    Otherwise, returns Ok with all values.

    Args:
        results: List of Result objects

    Returns:
        Ok(list of values) or first Err encountered
    """
    values: list[T] = []
    for r in results:
        if r.is_err():
            return r  # type: ignore
        values.append(r.unwrap())
    return Ok(values)
