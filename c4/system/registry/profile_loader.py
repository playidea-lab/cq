"""Profile Loader - Loads and merges user profiles from multiple sources.

Search order:
1. Project-level: .c4/profiles/{user}.yaml
2. Global: ~/.c4/profile.yaml
3. None (graceful degradation)
"""

from __future__ import annotations

import logging
import subprocess
from pathlib import Path

import yaml

from .profile import UserProfile

logger = logging.getLogger(__name__)


def _get_git_user_name() -> str:
    """Get git user name for default profile."""
    try:
        result = subprocess.run(
            ["git", "config", "user.name"],
            capture_output=True,
            text=True,
            timeout=5,
        )
        if result.returncode == 0 and result.stdout.strip():
            return result.stdout.strip()
    except Exception:
        pass
    return "default"


class ProfileLoader:
    """Loads user profile with fallback chain."""

    def __init__(self, c4_dir: Path):
        self.c4_dir = c4_dir
        self.global_path = Path.home() / ".c4" / "profile.yaml"
        self.project_profiles_dir = c4_dir / "profiles"

    def load(self, user: str | None = None) -> UserProfile | None:
        """Load profile with project > global > None fallback.

        Args:
            user: Username to load. None = auto-detect from git config.

        Returns:
            UserProfile if found, None otherwise.
        """
        if user is None:
            user = _get_git_user_name()

        # 1. Project-level override
        project_path = self.project_profiles_dir / f"{user}.yaml"
        project_profile = self._load_from(project_path)

        # 2. Global profile
        global_profile = self._load_from(self.global_path)

        if project_profile and global_profile:
            return self._merge(global_profile, project_profile)
        if project_profile:
            return project_profile
        if global_profile:
            return global_profile

        return None

    def _load_from(self, path: Path) -> UserProfile | None:
        """Load profile from a single YAML file."""
        if not path.exists():
            return None
        try:
            data = yaml.safe_load(path.read_text())
            if data:
                return UserProfile(**data)
        except Exception as e:
            logger.debug(f"Failed to load profile from {path}: {e}")
        return None

    def _merge(
        self, base: UserProfile, override: UserProfile
    ) -> UserProfile:
        """Merge project-level overrides into global profile.

        Project profile wins for non-default values.
        """
        base_data = base.model_dump()
        override_data = override.model_dump()
        default_data = UserProfile().model_dump()

        merged = self._deep_merge(base_data, override_data, default_data)
        return UserProfile(**merged)

    def _deep_merge(self, base: dict, override: dict, default: dict) -> dict:
        """Deep merge where override wins for non-default values."""
        result = dict(base)
        for key, val in override.items():
            if key not in base:
                result[key] = val
            elif isinstance(val, dict) and isinstance(base.get(key), dict):
                result[key] = self._deep_merge(
                    base[key], val, default.get(key, {})
                )
            elif val != default.get(key):
                result[key] = val
        return result

    @staticmethod
    def install_template(target_path: Path | None = None) -> Path:
        """Install default profile template.

        Args:
            target_path: Where to install. Default: ~/.c4/profile.yaml

        Returns:
            Path to installed template.
        """
        if target_path is None:
            target_path = Path.home() / ".c4" / "profile.yaml"

        if target_path.exists():
            logger.debug(f"Profile already exists at {target_path}")
            return target_path

        template_path = Path(__file__).parent.parent.parent / "data" / "profile-template.yaml"
        if template_path.exists():
            content = template_path.read_text()
        else:
            content = _DEFAULT_TEMPLATE

        # Substitute git user name
        user_name = _get_git_user_name()
        content = content.replace("__USER_NAME__", user_name)

        target_path.parent.mkdir(parents=True, exist_ok=True)
        target_path.write_text(content)
        logger.info(f"Profile template installed at {target_path}")
        return target_path


_DEFAULT_TEMPLATE = """\
# C4 User Profile
# Auto-updated by C4 from checkpoint observations.
# Manual edits are preserved and merged.

name: __USER_NAME__
version: 0

review:
  thoroughness: balanced
  tone: diplomatic
  focus:
    - correctness
    - clarity
  strictness: 0.5
  paper_criteria: []

writing:
  language: en
  formality: professional
  verbosity: moderate
  citation_style: null

communication:
  explanation_depth: standard
  dod_detail_level: standard

expertise:
  domains: {}
  research_fields: []

persona_overrides: {}
learned_from: {}
"""
