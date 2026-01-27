"""External Skill Loader - Multi-source skill loading with conflict resolution.

Supports loading skills from:
1. Built-in skills (c4/supervisor/agent_graph/skills/)
2. Project skills (.c4/skills/)
3. External paths (user-configured)

Skills are loaded in priority order:
  External Registry > Project Skills > Built-in Skills
"""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path

import yaml

from c4.supervisor.agent_graph.models import Skill
from c4.supervisor.agent_graph.skill_md_parser import SkillMdParser
from c4.supervisor.agent_graph.skill_validator import SkillValidator

logger = logging.getLogger(__name__)


class SkillSource(str, Enum):
    """Source type for skills."""

    BUILTIN = "builtin"
    PROJECT = "project"
    EXTERNAL = "external"


class ConflictResolution(str, Enum):
    """Strategy for resolving skill ID conflicts."""

    PROJECT_WINS = "project_wins"
    BUILTIN_WINS = "builtin_wins"
    ERROR = "error"


@dataclass
class LoadedSkill:
    """A skill with metadata about its source."""

    skill: Skill
    source: SkillSource
    path: Path
    overridden_by: Path | None = None


@dataclass
class SkillLoadResult:
    """Result of loading skills from all sources."""

    skills: dict[str, LoadedSkill] = field(default_factory=dict)
    conflicts: list[tuple[str, Path, Path]] = field(default_factory=list)
    errors: list[tuple[Path, str]] = field(default_factory=list)
    warnings: list[str] = field(default_factory=list)


@dataclass
class ExternalLoaderConfig:
    """Configuration for the external skill loader."""

    # Paths to search for skills
    builtin_path: Path | None = None
    project_paths: list[Path] = field(default_factory=list)
    external_paths: list[Path] = field(default_factory=list)

    # Conflict resolution
    conflict_resolution: ConflictResolution = ConflictResolution.PROJECT_WINS
    override_allowed: list[str] = field(default_factory=list)

    # Skill filtering
    disabled_skills: list[str] = field(default_factory=list)
    enabled_groups: list[str] = field(default_factory=list)

    # Validation
    validate_on_load: bool = True
    strict_mode: bool = False


class ExternalSkillLoader:
    """Loader for skills from multiple sources.

    Implements the 3-layer skill loading model:
    - Layer 1: Built-in Skills (C4 Core)
    - Layer 2: Project Skills (.c4/skills/)
    - Layer 3: External Registry (optional)
    """

    # Default search paths
    DEFAULT_PROJECT_PATHS = [
        Path(".c4/skills"),
        Path(".cursor/skills"),
    ]

    DEFAULT_EXTERNAL_PATHS = [
        Path.home() / ".c4/skills",
        Path.home() / ".cursor/skills",
    ]

    def __init__(self, config: ExternalLoaderConfig | None = None):
        """Initialize the loader.

        Args:
            config: Loader configuration. If None, uses defaults.
        """
        self.config = config or ExternalLoaderConfig()
        self.validator = SkillValidator()
        self.md_parser = SkillMdParser()
        self._groups: dict[str, list[str]] = {}

    def load_all(self, project_root: Path | None = None) -> SkillLoadResult:
        """Load skills from all configured sources.

        Args:
            project_root: Root directory of the project. Used to resolve
                relative paths for project skills.

        Returns:
            SkillLoadResult containing all loaded skills and any issues.
        """
        result = SkillLoadResult()
        project_root = project_root or Path.cwd()

        # Load skill groups if available
        self._load_groups(project_root)

        # Layer 1: Built-in skills (lowest priority)
        if self.config.builtin_path and self.config.builtin_path.exists():
            self._load_from_directory(self.config.builtin_path, SkillSource.BUILTIN, result)

        # Layer 2: Project skills (medium priority)
        for rel_path in self.config.project_paths or self.DEFAULT_PROJECT_PATHS:
            project_skill_path = project_root / rel_path
            if project_skill_path.exists():
                self._load_from_directory(project_skill_path, SkillSource.PROJECT, result)

        # Layer 3: External skills (highest priority)
        for ext_path in self.config.external_paths or self.DEFAULT_EXTERNAL_PATHS:
            if ext_path.exists():
                self._load_from_directory(ext_path, SkillSource.EXTERNAL, result)

        # Apply filters
        self._apply_filters(result)

        return result

    def _load_from_directory(
        self,
        directory: Path,
        source: SkillSource,
        result: SkillLoadResult,
    ) -> None:
        """Load all skills from a directory.

        Args:
            directory: Directory to scan for skills
            source: Source type for the skills
            result: Result object to populate
        """
        if not directory.exists():
            return

        import os

        # Find all skill files with a single directory traversal
        # instead of 4 separate glob calls (performance optimization)
        skill_files: list[Path] = []
        for root, _dirs, files in os.walk(directory):
            root_path = Path(root)
            for filename in files:
                # Check if file matches skill patterns
                is_yaml = filename.endswith((".yaml", ".yml"))
                is_skill_md = filename == "SKILL.md" or filename.endswith(".skill.md")

                if is_yaml or is_skill_md:
                    # Filter out meta files
                    if not filename.startswith("_") and filename not in (
                        "_domain.yaml",
                        "_groups.yaml",
                    ):
                        skill_files.append(root_path / filename)

        for skill_path in skill_files:
            self._load_skill_file(skill_path, source, result)

    def _load_skill_file(
        self,
        skill_path: Path,
        source: SkillSource,
        result: SkillLoadResult,
    ) -> None:
        """Load a single skill file.

        Args:
            skill_path: Path to the skill file
            source: Source type
            result: Result object to populate
        """
        try:
            # Parse based on file type
            if skill_path.suffix == ".md":
                skill = self.md_parser.parse(skill_path)
            else:
                skill = self._load_yaml_skill(skill_path)

            if skill is None:
                return

            # Validate if configured
            if self.config.validate_on_load:
                validation = self.validator.validate_file(skill_path)
                if not validation.valid:
                    if self.config.strict_mode:
                        result.errors.append(
                            (skill_path, f"Validation failed: {validation.issues}")
                        )
                        return
                    else:
                        for issue in validation.issues:
                            if issue.level == "error":
                                result.warnings.append(f"{skill_path}: {issue.message}")

            # Handle conflicts
            skill_id = skill.id
            if skill_id in result.skills:
                existing = result.skills[skill_id]
                conflict_resolved = self._resolve_conflict(
                    skill_id, existing, LoadedSkill(skill, source, skill_path), result
                )
                if not conflict_resolved:
                    return

            # Add to results
            result.skills[skill_id] = LoadedSkill(
                skill=skill,
                source=source,
                path=skill_path,
            )

        except Exception as e:
            result.errors.append((skill_path, str(e)))

    def _load_yaml_skill(self, skill_path: Path) -> Skill | None:
        """Load a skill from a YAML file.

        Args:
            skill_path: Path to the YAML file

        Returns:
            Skill model or None if loading failed
        """
        content = skill_path.read_text(encoding="utf-8")
        data = yaml.safe_load(content)

        if not data:
            return None

        # Handle nested skill key
        if "skill" in data:
            data = data["skill"]

        return Skill.model_validate(data)

    def _resolve_conflict(
        self,
        skill_id: str,
        existing: LoadedSkill,
        new: LoadedSkill,
        result: SkillLoadResult,
    ) -> bool:
        """Resolve a skill ID conflict.

        Args:
            skill_id: The conflicting skill ID
            existing: The existing loaded skill
            new: The new skill attempting to load
            result: Result object for recording conflicts

        Returns:
            True if the new skill should replace existing, False otherwise
        """
        # Record the conflict
        result.conflicts.append((skill_id, existing.path, new.path))

        # Check override allowlist
        if skill_id in self.config.override_allowed:
            if new.source.value > existing.source.value:  # Higher priority wins
                existing.overridden_by = new.path
                return True
            return False

        # Apply resolution strategy
        strategy = self.config.conflict_resolution

        if strategy == ConflictResolution.ERROR:
            result.errors.append(
                (
                    new.path,
                    f"Skill ID '{skill_id}' conflicts with {existing.path}",
                )
            )
            return False

        elif strategy == ConflictResolution.PROJECT_WINS:
            # Project > Builtin, External > Project
            if new.source == SkillSource.PROJECT and existing.source == SkillSource.BUILTIN:
                existing.overridden_by = new.path
                return True
            elif new.source == SkillSource.EXTERNAL:
                existing.overridden_by = new.path
                return True
            return False

        elif strategy == ConflictResolution.BUILTIN_WINS:
            # Builtin always wins
            if existing.source == SkillSource.BUILTIN:
                return False
            return True

        return False

    def _load_groups(self, project_root: Path) -> None:
        """Load skill group definitions.

        Args:
            project_root: Project root directory
        """
        # Check built-in groups
        if self.config.builtin_path:
            groups_file = self.config.builtin_path / "_groups.yaml"
            if groups_file.exists():
                self._parse_groups_file(groups_file)

        # Check project groups (override built-in)
        for rel_path in self.config.project_paths or self.DEFAULT_PROJECT_PATHS:
            groups_file = project_root / rel_path / "_groups.yaml"
            if groups_file.exists():
                self._parse_groups_file(groups_file)

    def _parse_groups_file(self, groups_file: Path) -> None:
        """Parse a skill groups file.

        Args:
            groups_file: Path to the groups YAML file
        """
        try:
            content = groups_file.read_text(encoding="utf-8")
            data = yaml.safe_load(content)

            if data and "groups" in data:
                for group_name, group_data in data["groups"].items():
                    if "skills" in group_data:
                        self._groups[group_name] = group_data["skills"]
        except Exception as e:
            logger.warning(f"Failed to load groups from {groups_file}: {e}")

    def _apply_filters(self, result: SkillLoadResult) -> None:
        """Apply skill filters (disabled skills, enabled groups).

        Args:
            result: Result object to filter
        """
        # Remove disabled skills
        for skill_id in self.config.disabled_skills:
            if skill_id in result.skills:
                del result.skills[skill_id]

        # If enabled_groups is set, only keep skills in those groups
        if self.config.enabled_groups:
            enabled_skills = set()
            for group_name in self.config.enabled_groups:
                if group_name in self._groups:
                    enabled_skills.update(self._groups[group_name])

            # Keep only enabled skills
            result.skills = {
                skill_id: loaded
                for skill_id, loaded in result.skills.items()
                if skill_id in enabled_skills
            }

    def get_groups(self) -> dict[str, list[str]]:
        """Get loaded skill groups.

        Returns:
            Dictionary of group name to skill IDs
        """
        return self._groups.copy()


def load_all_skills(
    project_root: Path | None = None,
    config: ExternalLoaderConfig | None = None,
) -> dict[str, Skill]:
    """Convenience function to load all skills.

    Args:
        project_root: Project root directory
        config: Loader configuration

    Returns:
        Dictionary of skill ID to Skill model
    """
    loader = ExternalSkillLoader(config)
    result = loader.load_all(project_root)
    return {skill_id: loaded.skill for skill_id, loaded in result.skills.items()}
