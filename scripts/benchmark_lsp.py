#!/usr/bin/env python3
"""Benchmark script for LSP provider performance.

Compares Jedi-only vs Unified (multilspy + Jedi fallback) provider performance.

Usage:
    uv run python scripts/benchmark_lsp.py
    uv run python scripts/benchmark_lsp.py --iterations 10
"""

from __future__ import annotations

import argparse
import statistics
import time
from pathlib import Path
from typing import Any

# Test configuration
TEST_FILES = [
    "c4/daemon/c4_daemon.py",  # Large file (~4800 lines)
    "c4/lsp/unified_provider.py",  # Medium file
    "c4/mcp/registry.py",  # Small file
]

TEST_SYMBOLS = [
    "C4Daemon",
    "UnifiedSymbolProvider",
    "find_symbol",
]


def benchmark_jedi(
    project_path: Path,
    iterations: int = 5,
) -> dict[str, Any]:
    """Benchmark Jedi-only provider."""
    from c4.lsp.jedi_provider import find_symbol_mcp, get_symbols_overview_mcp

    results = {
        "find_symbol_times": [],
        "get_overview_times": [],
        "errors": [],
    }

    for i in range(iterations):
        # find_symbol benchmark
        for symbol in TEST_SYMBOLS:
            start = time.perf_counter()
            try:
                find_symbol_mcp(
                    name_path_pattern=symbol,
                    project_path=str(project_path),
                    timeout=30,
                )
                elapsed = time.perf_counter() - start
                results["find_symbol_times"].append(elapsed)
            except Exception as e:
                results["errors"].append(f"find_symbol({symbol}): {e}")

        # get_symbols_overview benchmark
        for file_path in TEST_FILES:
            start = time.perf_counter()
            try:
                get_symbols_overview_mcp(
                    relative_path=file_path,
                    project_path=str(project_path),
                )
                elapsed = time.perf_counter() - start
                results["get_overview_times"].append(elapsed)
            except Exception as e:
                results["errors"].append(f"get_overview({file_path}): {e}")

    return results


def benchmark_unified(
    project_path: Path,
    iterations: int = 5,
) -> dict[str, Any]:
    """Benchmark Unified provider."""
    from c4.lsp.unified_provider import (
        find_symbol_unified,
        get_symbols_overview_unified,
    )

    results = {
        "find_symbol_times": [],
        "get_overview_times": [],
        "errors": [],
    }

    for i in range(iterations):
        # find_symbol benchmark
        for symbol in TEST_SYMBOLS:
            start = time.perf_counter()
            try:
                find_symbol_unified(
                    name_path_pattern=symbol,
                    project_path=str(project_path),
                    timeout=30,
                )
                elapsed = time.perf_counter() - start
                results["find_symbol_times"].append(elapsed)
            except Exception as e:
                results["errors"].append(f"find_symbol({symbol}): {e}")

        # get_symbols_overview benchmark
        for file_path in TEST_FILES:
            start = time.perf_counter()
            try:
                get_symbols_overview_unified(
                    relative_path=file_path,
                    project_path=str(project_path),
                    timeout=30,
                )
                elapsed = time.perf_counter() - start
                results["get_overview_times"].append(elapsed)
            except Exception as e:
                results["errors"].append(f"get_overview({file_path}): {e}")

    return results


def format_stats(times: list[float], name: str) -> str:
    """Format timing statistics."""
    if not times:
        return f"{name}: No data"

    mean = statistics.mean(times)
    if len(times) > 1:
        stdev = statistics.stdev(times)
        return f"{name}: mean={mean:.3f}s, stdev={stdev:.3f}s, min={min(times):.3f}s, max={max(times):.3f}s"
    else:
        return f"{name}: {mean:.3f}s"


def main():
    parser = argparse.ArgumentParser(description="Benchmark LSP providers")
    parser.add_argument(
        "--iterations",
        type=int,
        default=3,
        help="Number of iterations (default: 3)",
    )
    parser.add_argument(
        "--project",
        type=str,
        default=".",
        help="Project path (default: current directory)",
    )
    args = parser.parse_args()

    project_path = Path(args.project).resolve()
    print(f"Benchmarking LSP providers in: {project_path}")
    print(f"Iterations: {args.iterations}")
    print()

    # Check availability
    try:
        from c4.lsp.jedi_provider import JEDI_AVAILABLE
        from c4.lsp.multilspy_provider import MULTILSPY_AVAILABLE

        print(f"JEDI_AVAILABLE: {JEDI_AVAILABLE}")
        print(f"MULTILSPY_AVAILABLE: {MULTILSPY_AVAILABLE}")
        print()
    except ImportError as e:
        print(f"Import error: {e}")
        return

    # Benchmark Jedi
    print("=" * 60)
    print("Benchmarking Jedi-only provider...")
    print("=" * 60)
    jedi_results = benchmark_jedi(project_path, args.iterations)
    print(format_stats(jedi_results["find_symbol_times"], "find_symbol"))
    print(format_stats(jedi_results["get_overview_times"], "get_overview"))
    if jedi_results["errors"]:
        print(f"Errors: {len(jedi_results['errors'])}")
        for err in jedi_results["errors"][:3]:
            print(f"  - {err}")
    print()

    # Benchmark Unified
    print("=" * 60)
    print("Benchmarking Unified provider (multilspy + Jedi fallback)...")
    print("=" * 60)
    unified_results = benchmark_unified(project_path, args.iterations)
    print(format_stats(unified_results["find_symbol_times"], "find_symbol"))
    print(format_stats(unified_results["get_overview_times"], "get_overview"))
    if unified_results["errors"]:
        print(f"Errors: {len(unified_results['errors'])}")
        for err in unified_results["errors"][:3]:
            print(f"  - {err}")
    print()

    # Comparison
    print("=" * 60)
    print("Comparison")
    print("=" * 60)

    if jedi_results["find_symbol_times"] and unified_results["find_symbol_times"]:
        jedi_mean = statistics.mean(jedi_results["find_symbol_times"])
        unified_mean = statistics.mean(unified_results["find_symbol_times"])
        speedup = jedi_mean / unified_mean if unified_mean > 0 else 0
        print(f"find_symbol: Jedi={jedi_mean:.3f}s, Unified={unified_mean:.3f}s, speedup={speedup:.2f}x")

    if jedi_results["get_overview_times"] and unified_results["get_overview_times"]:
        jedi_mean = statistics.mean(jedi_results["get_overview_times"])
        unified_mean = statistics.mean(unified_results["get_overview_times"])
        speedup = jedi_mean / unified_mean if unified_mean > 0 else 0
        print(f"get_overview: Jedi={jedi_mean:.3f}s, Unified={unified_mean:.3f}s, speedup={speedup:.2f}x")


if __name__ == "__main__":
    main()
