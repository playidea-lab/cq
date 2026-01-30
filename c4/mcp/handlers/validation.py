"""Validation tool handlers.

Handles: c4_run_validation
"""

from typing import Any

from ..registry import register_tool


@register_tool("c4_run_validation")
def handle_run_validation(daemon: Any, arguments: dict[str, Any]) -> dict[str, Any]:
    """Run validation commands (lint, test, etc.)."""
    return daemon.c4_run_validation(
        names=arguments.get("names"),
        fail_fast=arguments.get("fail_fast", True),
        timeout=arguments.get("timeout", 300),
    )
