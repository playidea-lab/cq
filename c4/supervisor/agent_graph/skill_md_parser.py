"""SKILL.md Parser - Vercel-style skill format support.

Parses SKILL.md files into SkillV2 format for compatibility with
external skill registries like Vercel's agent-skills.

SKILL.md Format:
```markdown
---
name: react-best-practices
description: 40+ React/Next.js optimization rules
---

## When to Use
- React 컴포넌트 작성 시
- 번들 사이즈 최적화 시

## Rules

### PERF-001: Eliminate async waterfalls (impact: critical)
...
```
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import yaml

from c4.supervisor.agent_graph.models import (
    DomainSpecificConfig,
    ImpactLevel,
    Skill,
    SkillCategory,
    SkillDependencies,
    SkillMetadata,
    SkillRule,
    SkillTriggers,
)


@dataclass
class ParsedSkillMd:
    """Intermediate representation of parsed SKILL.md content."""

    name: str
    description: str
    frontmatter: dict[str, Any] = field(default_factory=dict)
    when_to_use: list[str] = field(default_factory=list)
    rules: list[dict[str, Any]] = field(default_factory=list)
    examples: list[dict[str, str]] = field(default_factory=list)
    raw_sections: dict[str, str] = field(default_factory=dict)


class SkillMdParser:
    """Parser for SKILL.md format files.

    Converts Vercel-style SKILL.md files into C4 SkillV2 format.
    """

    # Regex patterns
    FRONTMATTER_PATTERN = re.compile(r"^---\s*\n(.*?)\n---\s*\n", re.DOTALL)
    SECTION_PATTERN = re.compile(r"^##\s+(.+?)$", re.MULTILINE)
    RULE_PATTERN = re.compile(
        r"^###\s+([A-Z]+-\d{3}):\s*(.+?)(?:\s*\(impact:\s*(\w+)\))?\s*$",
        re.MULTILINE,
    )
    CODE_BLOCK_PATTERN = re.compile(r"```(\w*)\n(.*?)```", re.DOTALL)

    # Default mappings
    DEFAULT_IMPACT = ImpactLevel.MEDIUM
    IMPACT_KEYWORDS = {
        "critical": ImpactLevel.CRITICAL,
        "high": ImpactLevel.HIGH,
        "medium": ImpactLevel.MEDIUM,
        "low": ImpactLevel.LOW,
    }

    def parse(self, skill_path: Path) -> Skill:
        """Parse a SKILL.md file into a Skill model.

        Args:
            skill_path: Path to the SKILL.md file

        Returns:
            Skill model populated from the markdown content

        Raises:
            ValueError: If the file cannot be parsed
        """
        content = skill_path.read_text(encoding="utf-8")
        parsed = self._parse_markdown(content)
        return self._to_skill(parsed, skill_path)

    def _parse_markdown(self, content: str) -> ParsedSkillMd:
        """Parse markdown content into intermediate representation."""
        # Extract frontmatter
        frontmatter = {}
        content_body = content

        fm_match = self.FRONTMATTER_PATTERN.match(content)
        if fm_match:
            try:
                frontmatter = yaml.safe_load(fm_match.group(1)) or {}
            except yaml.YAMLError:
                frontmatter = {}
            content_body = content[fm_match.end() :]

        # Extract name and description from frontmatter
        name = frontmatter.get("name", "")
        description = frontmatter.get("description", "")

        # Parse sections
        sections = self._split_sections(content_body)

        # Parse "When to Use" section
        when_to_use = []
        if "When to Use" in sections:
            when_to_use = self._parse_list_items(sections["When to Use"])

        # Parse Rules section
        rules = []
        if "Rules" in sections:
            rules = self._parse_rules(sections["Rules"])

        # Parse Examples section
        examples = []
        if "Examples" in sections:
            examples = self._parse_examples(sections["Examples"])

        return ParsedSkillMd(
            name=name,
            description=description,
            frontmatter=frontmatter,
            when_to_use=when_to_use,
            rules=rules,
            examples=examples,
            raw_sections=sections,
        )

    def _split_sections(self, content: str) -> dict[str, str]:
        """Split content into sections by ## headers."""
        sections: dict[str, str] = {}
        matches = list(self.SECTION_PATTERN.finditer(content))

        for i, match in enumerate(matches):
            section_name = match.group(1).strip()
            start = match.end()
            end = matches[i + 1].start() if i + 1 < len(matches) else len(content)
            sections[section_name] = content[start:end].strip()

        return sections

    def _parse_list_items(self, content: str) -> list[str]:
        """Parse markdown list items."""
        items = []
        for line in content.split("\n"):
            line = line.strip()
            if line.startswith("- ") or line.startswith("* "):
                items.append(line[2:].strip())
            elif line.startswith("1. ") or re.match(r"^\d+\.\s", line):
                items.append(re.sub(r"^\d+\.\s*", "", line).strip())
        return items

    def _parse_rules(self, content: str) -> list[dict[str, Any]]:
        """Parse rules from the Rules section."""
        rules = []
        matches = list(self.RULE_PATTERN.finditer(content))

        for i, match in enumerate(matches):
            rule_id = match.group(1)
            description = match.group(2).strip()
            impact_str = match.group(3)
            impact = (
                self.IMPACT_KEYWORDS.get(impact_str.lower(), self.DEFAULT_IMPACT)
                if impact_str
                else self.DEFAULT_IMPACT
            )

            # Get rule body (content until next rule or section)
            start = match.end()
            end = matches[i + 1].start() if i + 1 < len(matches) else len(content)
            body = content[start:end].strip()

            # Extract code examples
            example_bad = None
            example_good = None
            code_blocks = self.CODE_BLOCK_PATTERN.findall(body)

            for j, (lang, code) in enumerate(code_blocks):
                code = code.strip()
                # Heuristic: first block is bad, second is good
                # Or look for "bad" / "good" markers
                body_lower = body.lower()
                if j == 0 and ("bad" in body_lower or "avoid" in body_lower):
                    example_bad = code
                elif j == 1 or "good" in body_lower or "prefer" in body_lower:
                    example_good = code
                elif j == 0:
                    example_bad = code
                elif j == 1:
                    example_good = code

            rules.append(
                {
                    "id": rule_id,
                    "description": description,
                    "impact": impact,
                    "example_bad": example_bad,
                    "example_good": example_good,
                }
            )

        return rules

    def _parse_examples(self, content: str) -> list[dict[str, str]]:
        """Parse examples section."""
        examples = []
        code_blocks = self.CODE_BLOCK_PATTERN.findall(content)
        for lang, code in code_blocks:
            examples.append({"language": lang or "text", "code": code.strip()})
        return examples

    def _to_skill(self, parsed: ParsedSkillMd, skill_path: Path) -> Skill:
        """Convert parsed SKILL.md to Skill model."""
        frontmatter = parsed.frontmatter

        # Generate skill ID from name or filename
        skill_id = frontmatter.get("id") or self._name_to_id(parsed.name or skill_path.stem)

        # Extract triggers from "When to Use"
        keywords = self._extract_keywords(parsed.when_to_use + [parsed.description])
        triggers = SkillTriggers(
            keywords=keywords,
            task_types=frontmatter.get("task_types", []),
            file_patterns=frontmatter.get("file_patterns", []),
        )

        # Build rules
        rules = [
            SkillRule(
                id=r["id"],
                description=r["description"],
                impact=r["impact"],
                example_bad=r.get("example_bad"),
                example_good=r.get("example_good"),
            )
            for r in parsed.rules
        ]

        # Extract domains
        domains = frontmatter.get("domains", ["universal"])
        if isinstance(domains, str):
            domains = [domains]

        # Build domain_specific config
        domain_specific = None
        if "domain_config" in frontmatter:
            domain_specific = {
                k: DomainSpecificConfig(**v) for k, v in frontmatter["domain_config"].items()
            }

        # Metadata
        metadata = SkillMetadata(
            version=frontmatter.get("version", "1.0.0"),
            author=frontmatter.get("author"),
            license=frontmatter.get("license", "unlicensed"),
            tags=frontmatter.get("tags", []),
        )

        # Impact level
        impact_str = frontmatter.get("impact", "medium")
        impact = self.IMPACT_KEYWORDS.get(impact_str.lower(), self.DEFAULT_IMPACT)

        # Category
        category_str = frontmatter.get("category")
        category = None
        if category_str:
            try:
                category = SkillCategory(category_str)
            except ValueError:
                category = None

        # Dependencies
        dependencies = None
        if "dependencies" in frontmatter:
            deps = frontmatter["dependencies"]
            dependencies = SkillDependencies(
                required=deps.get("required", []),
                optional=deps.get("optional", []),
            )

        # Capabilities (from frontmatter or derived from rules)
        capabilities = frontmatter.get("capabilities", [])
        if not capabilities and rules:
            # Generate capabilities from rule IDs
            capabilities = [r.id.split("-")[0].lower() for r in rules[:5]]
            capabilities = list(set(capabilities))

        return Skill(
            id=skill_id,
            name=parsed.name or skill_id,
            description=parsed.description or f"Skill: {skill_id}",
            capabilities=capabilities,
            triggers=triggers,
            impact=impact,
            category=category,
            domains=domains,
            domain_specific=domain_specific,
            metadata=metadata,
            rules=rules if rules else None,
            dependencies=dependencies,
            tools=frontmatter.get("tools", []),
            complementary_skills=frontmatter.get("complementary_skills", []),
            prerequisites=frontmatter.get("prerequisites", []),
            leads_to=frontmatter.get("leads_to", []),
        )

    def _name_to_id(self, name: str) -> str:
        """Convert a skill name to kebab-case ID."""
        # Remove special characters, convert to lowercase, replace spaces with hyphens
        name = re.sub(r"[^\w\s-]", "", name.lower())
        name = re.sub(r"[\s_]+", "-", name)
        name = re.sub(r"-+", "-", name)
        return name.strip("-")

    def _extract_keywords(self, texts: list[str]) -> list[str]:
        """Extract potential keywords from text."""
        keywords = set()
        # Common technical terms to look for
        tech_terms = [
            "react",
            "vue",
            "angular",
            "node",
            "python",
            "javascript",
            "typescript",
            "api",
            "database",
            "test",
            "deploy",
            "docker",
            "kubernetes",
            "aws",
            "gcp",
            "azure",
            "performance",
            "security",
            "accessibility",
            "cache",
            "auth",
            "graphql",
            "rest",
            "ml",
            "ai",
            "data",
        ]

        for text in texts:
            text_lower = text.lower()
            for term in tech_terms:
                if term in text_lower:
                    keywords.add(term)

        return list(keywords)


def parse_skill_md(skill_path: Path) -> Skill:
    """Convenience function to parse a SKILL.md file.

    Args:
        skill_path: Path to the SKILL.md file

    Returns:
        Skill model
    """
    parser = SkillMdParser()
    return parser.parse(skill_path)
