"""
c2 profile management.

Loads/saves the unified .c2/profile.yaml and project type configs.
Extends auto_review.profile with cross-domain support.
"""

from __future__ import annotations

from pathlib import Path
from typing import Any

import yaml


def load_profile(profile_path: Path | None = None) -> dict[str, Any]:
    """Load the c2 profile from YAML.

    Args:
        profile_path: Path to profile.yaml. Defaults to .c2/profile.yaml
                      relative to cwd.

    Returns:
        Profile dict.
    """
    if profile_path is None:
        profile_path = Path(".c2/profile.yaml")

    if not profile_path.exists():
        return {}

    with open(profile_path, encoding="utf-8") as f:
        data = yaml.safe_load(f) or {}
    return data


def save_profile(data: dict[str, Any], profile_path: Path | None = None) -> None:
    """Save profile data to YAML.

    Args:
        data: Profile dict to save.
        profile_path: Target path. Defaults to .c2/profile.yaml.
    """
    if profile_path is None:
        profile_path = Path(".c2/profile.yaml")

    profile_path.parent.mkdir(parents=True, exist_ok=True)
    with open(profile_path, "w", encoding="utf-8") as f:
        yaml.dump(data, f, allow_unicode=True, sort_keys=False, default_flow_style=False)


def load_project_type(type_name: str, base_path: Path | None = None) -> dict[str, Any]:
    """Load a project type configuration.

    Args:
        type_name: Project type name (e.g. 'academic_paper', 'proposal').
        base_path: Base path for project_types dir. Defaults to .c2/project_types/.

    Returns:
        Project type config dict.
    """
    if base_path is None:
        base_path = Path(".c2/project_types")

    type_path = base_path / f"{type_name}.yaml"
    if not type_path.exists():
        raise FileNotFoundError(f"Project type not found: {type_path}")

    with open(type_path, encoding="utf-8") as f:
        return yaml.safe_load(f) or {}


def get_preference(profile: dict[str, Any], domain: str, key: str, default: Any = None) -> Any:
    """Get a preference value from the profile.

    Args:
        profile: Loaded profile dict.
        domain: Domain name (discover/read/write/review).
        key: Preference key within the domain.
        default: Default value if not found.

    Returns:
        Preference value.
    """
    return profile.get("preferences", {}).get(domain, {}).get(key, default)


def get_learned_patterns(profile: dict[str, Any]) -> dict[str, Any]:
    """Get the learned_patterns section from profile.

    Args:
        profile: Loaded profile dict.

    Returns:
        Learned patterns dict.
    """
    return profile.get("learned_patterns", {})


def update_learned_patterns(
    profile: dict[str, Any],
    tone_preferences: list[str] | None = None,
    structure_preferences: list[str] | None = None,
    frequent_edits: list[str] | None = None,
) -> dict[str, Any]:
    """Update the learned_patterns section (non-destructive merge).

    Args:
        profile: Profile dict to update (mutated in place).
        tone_preferences: New tone patterns to add.
        structure_preferences: New structure patterns to add.
        frequent_edits: New edit patterns to add.

    Returns:
        Updated profile dict.
    """
    from datetime import date

    patterns = profile.setdefault("learned_patterns", {})

    if tone_preferences:
        existing = patterns.get("tone_preferences", [])
        patterns["tone_preferences"] = list(set(existing + tone_preferences))

    if structure_preferences:
        existing = patterns.get("structure_preferences", [])
        patterns["structure_preferences"] = list(set(existing + structure_preferences))

    if frequent_edits:
        existing = patterns.get("frequent_edits", [])
        patterns["frequent_edits"] = list(set(existing + frequent_edits))

    patterns["last_updated"] = str(date.today())
    return profile
