"""Agent Graph System - Graph-based agent routing with 4-layer architecture.

Layers:
1. Skills - Atomic capabilities that agents can possess
2. Agents - Personas with skills and relationships
3. Domains - Problem areas with workflows
4. Rules - Routing overrides and chain extensions

Schema files are in the schema/ subdirectory.
"""

from pathlib import Path

SCHEMA_DIR = Path(__file__).parent / "schema"

__all__ = ["SCHEMA_DIR"]
