"""
c2 persona learning module.

Analyzes diffs between AI drafts and user final edits to extract
patterns, then proposes profile updates.
"""

from __future__ import annotations

import difflib
from pathlib import Path

from c4.c2.models import EditPattern, ProfileDiff


class PersonaLearner:
    """Learns user preferences from edit patterns across all four domains."""

    @staticmethod
    def analyze_edits(original: str, edited: str) -> list[EditPattern]:
        """Extract edit patterns from AI draft vs user final version.

        Args:
            original: AI-generated draft text.
            edited: User's final edited text.

        Returns:
            List of EditPattern objects describing the changes.
        """
        patterns: list[EditPattern] = []

        orig_lines = original.splitlines(keepends=True)
        edit_lines = edited.splitlines(keepends=True)

        diff = list(difflib.unified_diff(orig_lines, edit_lines, lineterm=""))

        # Categorize changes
        deletions: list[str] = []
        additions: list[str] = []

        for line in diff:
            if line.startswith("---") or line.startswith("+++") or line.startswith("@@"):
                continue
            if line.startswith("-"):
                deletions.append(line[1:].strip())
            elif line.startswith("+"):
                additions.append(line[1:].strip())

        # Detect tone changes (softening language)
        tone_softening = _detect_tone_softening(deletions, additions)
        if tone_softening:
            patterns.append(
                EditPattern(
                    category="tone",
                    description="User softened tone: assertive → questioning/hedging",
                    examples=tone_softening[:3],
                )
            )

        # Detect conciseness (user shortened text)
        if len(edited) < len(original) * 0.8:
            ratio = len(edited) / max(len(original), 1)
            patterns.append(
                EditPattern(
                    category="structure",
                    description=f"User significantly shortened text (ratio: {ratio:.2f})",
                    examples=[f"Original: {len(original)} chars → Edited: {len(edited)} chars"],
                )
            )

        # Detect section restructuring
        if _detect_section_reorder(original, edited):
            patterns.append(
                EditPattern(
                    category="structure",
                    description="User reordered sections",
                )
            )

        # Detect deletion patterns (what user removes)
        if deletions and not additions:
            patterns.append(
                EditPattern(
                    category="deletion",
                    description="User removed content without replacement",
                    examples=deletions[:3],
                )
            )

        # Detect addition patterns (what user adds)
        if additions and not deletions:
            patterns.append(
                EditPattern(
                    category="addition",
                    description="User added new content",
                    examples=additions[:3],
                )
            )

        # Detect wording substitutions
        substitutions = _detect_substitutions(deletions, additions)
        if substitutions:
            patterns.append(
                EditPattern(
                    category="wording",
                    description="User substituted specific wordings",
                    examples=substitutions[:3],
                )
            )

        return patterns

    @staticmethod
    def suggest_profile_updates(
        patterns: list[EditPattern],
    ) -> ProfileDiff:
        """Propose profile updates based on accumulated patterns.

        Args:
            patterns: List of edit patterns from multiple sessions.

        Returns:
            ProfileDiff with suggested updates.
        """
        tone_updates: list[str] = []
        structure_updates: list[str] = []

        for p in patterns:
            if p.category == "tone":
                tone_updates.append(p.description)
            elif p.category == "structure":
                structure_updates.append(p.description)

        summary_parts: list[str] = []
        if tone_updates:
            summary_parts.append(f"톤 패턴 {len(tone_updates)}건 발견")
        if structure_updates:
            summary_parts.append(f"구조 패턴 {len(structure_updates)}건 발견")

        return ProfileDiff(
            tone_updates=tone_updates,
            structure_updates=structure_updates,
            new_patterns=patterns,
            summary=". ".join(summary_parts) if summary_parts else "변경 없음",
        )

    @staticmethod
    def apply_profile_diff(
        profile_path: Path,
        diff: ProfileDiff,
    ) -> None:
        """Apply approved profile updates to profile.yaml.

        Args:
            profile_path: Path to .c2/profile.yaml.
            diff: Approved ProfileDiff.
        """
        from c4.c2.profile import load_profile, save_profile, update_learned_patterns

        profile = load_profile(profile_path)
        update_learned_patterns(
            profile,
            tone_preferences=diff.tone_updates,
            structure_preferences=diff.structure_updates,
        )
        save_profile(profile, profile_path)


# --- Helper functions ---


def _detect_tone_softening(
    deletions: list[str], additions: list[str]
) -> list[str]:
    """Detect cases where assertive language was softened."""
    assertive_markers = ["있습니다", "오류가", "잘못", "틀린", "부적절"]
    soft_markers = ["필요합니다", "생각됩니다", "확인", "바랍니다", "감사"]

    examples: list[str] = []

    for d in deletions:
        if any(m in d for m in assertive_markers):
            for a in additions:
                if any(m in a for m in soft_markers):
                    examples.append(f"'{d[:50]}' → '{a[:50]}'")
                    break

    return examples


def _detect_section_reorder(original: str, edited: str) -> bool:
    """Detect if sections were reordered."""
    import re

    orig_headers = re.findall(r"^#{1,3}\s+(.+)$", original, re.MULTILINE)
    edit_headers = re.findall(r"^#{1,3}\s+(.+)$", edited, re.MULTILINE)

    if len(orig_headers) < 2 or len(edit_headers) < 2:
        return False

    return orig_headers != edit_headers and set(orig_headers) == set(edit_headers)


def run_review_learning(
    draft_path: Path,
    final_path: Path,
    profile_path: Path | None = None,
    auto_apply: bool = False,
) -> ProfileDiff:
    """Compare review draft vs final to run persona learning.

    Reads the AI-generated draft and the user's final version, extracts
    edit patterns, and optionally applies them to profile.yaml.

    Args:
        draft_path: Path to review/.draft.md (AI original).
        final_path: Path to review/리뷰의견.md (user-edited version).
        profile_path: Path to .c2/profile.yaml. Defaults to .c2/profile.yaml.
        auto_apply: If True, apply discovered patterns to profile.yaml.

    Returns:
        ProfileDiff with discovered patterns.
    """
    if profile_path is None:
        profile_path = Path(".c2/profile.yaml")

    draft_text = draft_path.read_text(encoding="utf-8")
    final_text = final_path.read_text(encoding="utf-8")

    patterns = PersonaLearner.analyze_edits(draft_text, final_text)
    diff = PersonaLearner.suggest_profile_updates(patterns)

    if auto_apply and (diff.tone_updates or diff.structure_updates):
        PersonaLearner.apply_profile_diff(profile_path, diff)

    return diff


def run_write_learning(
    draft_path: Path,
    final_path: Path,
    profile_path: Path | None = None,
    auto_apply: bool = False,
) -> ProfileDiff:
    """Compare write draft vs final to run persona learning.

    Reads the AI-generated draft and the user's final version, extracts
    edit patterns specific to writing (voice, conciseness, terminology),
    and optionally applies them to profile.yaml.

    Args:
        draft_path: Path to write/drafts/.draft_v1.md (AI original).
        final_path: Path to user's final version.
        profile_path: Path to .c2/profile.yaml. Defaults to .c2/profile.yaml.
        auto_apply: If True, apply discovered patterns to profile.yaml.

    Returns:
        ProfileDiff with discovered patterns.
    """
    if profile_path is None:
        profile_path = Path(".c2/profile.yaml")

    draft_text = draft_path.read_text(encoding="utf-8")
    final_text = final_path.read_text(encoding="utf-8")

    patterns = PersonaLearner.analyze_edits(draft_text, final_text)

    # Tag write-domain patterns for clarity
    for p in patterns:
        if not p.description.startswith("[write]"):
            p.description = f"[write] {p.description}"

    diff = PersonaLearner.suggest_profile_updates(patterns)

    if auto_apply and (diff.tone_updates or diff.structure_updates):
        PersonaLearner.apply_profile_diff(profile_path, diff)

    return diff


def _detect_substitutions(
    deletions: list[str], additions: list[str]
) -> list[str]:
    """Detect word-level substitutions between deletions and additions."""
    examples: list[str] = []

    for d, a in zip(deletions, additions):
        if d and a and d != a:
            ratio = difflib.SequenceMatcher(None, d, a).ratio()
            if 0.4 < ratio < 0.95:
                examples.append(f"'{d[:40]}' → '{a[:40]}'")

    return examples
