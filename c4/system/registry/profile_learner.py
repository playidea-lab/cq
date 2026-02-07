"""Profile Learner - Infers profile updates from observable behavior.

Only infers traits that can be directly observed:
- domain frequency -> expertise.domains
- REQUEST_CHANGES ratio -> review.strictness
- checkpoint notes keywords -> review.focus, paper_criteria
- dod_length average -> communication.dod_detail_level
- summary_length average -> writing.verbosity

Does NOT infer: tone, thoroughness (not directly observable)
"""

from __future__ import annotations

import logging
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import yaml
from pydantic import BaseModel

from .profile import UserProfile
from .profile_observer import ProfileObservation

logger = logging.getLogger(__name__)


class ProfileDelta(BaseModel):
    """A single profile change."""

    field_path: str
    old_value: Any
    new_value: Any
    reason: str


class ProfileLearner:
    """Analyzes observations and produces profile updates.

    Conservative: only updates when statistical evidence is sufficient.
    """

    def __init__(self, profile_path: Path):
        self.profile_path = profile_path

    def load_or_default(self) -> UserProfile:
        """Load profile from YAML or return default."""
        if self.profile_path.exists():
            try:
                data = yaml.safe_load(self.profile_path.read_text())
                if data:
                    return UserProfile(**data)
            except Exception as e:
                logger.warning(f"Failed to load profile: {e}")
        return UserProfile()

    def save(self, profile: UserProfile) -> None:
        """Save profile to YAML."""
        self.profile_path.parent.mkdir(parents=True, exist_ok=True)
        profile.last_updated = datetime.now(timezone.utc).isoformat()
        profile.version += 1
        data = profile.model_dump(exclude_none=True)
        self.profile_path.write_text(
            yaml.dump(data, default_flow_style=False, allow_unicode=True)
        )

    def analyze(
        self, observations: list[ProfileObservation], current: UserProfile
    ) -> list[ProfileDelta]:
        """Analyze observations and produce deltas.

        Rules (only observable traits):
        1. domain frequency -> expertise.domains
        2. REQUEST_CHANGES ratio -> review.strictness
        3. checkpoint notes keywords -> review.focus
        4. paper-related keywords -> review.paper_criteria
        5. dod_length average -> communication.dod_detail_level
        6. summary_length average -> writing.verbosity
        """
        deltas: list[ProfileDelta] = []

        deltas.extend(self._analyze_domains(observations, current))
        deltas.extend(self._analyze_strictness(observations, current))
        deltas.extend(self._analyze_review_focus(observations, current))
        deltas.extend(self._analyze_paper_criteria(observations, current))
        deltas.extend(self._analyze_dod_detail(observations, current))
        deltas.extend(self._analyze_verbosity(observations, current))

        return deltas

    def apply(
        self, current: UserProfile, deltas: list[ProfileDelta]
    ) -> UserProfile:
        """Apply deltas to produce updated profile."""
        data = current.model_dump()
        for delta in deltas:
            parts = delta.field_path.split(".")
            obj = data
            for part in parts[:-1]:
                obj = obj[part]
            obj[parts[-1]] = delta.new_value
        return UserProfile(**data)

    def _analyze_domains(
        self, observations: list[ProfileObservation], current: UserProfile
    ) -> list[ProfileDelta]:
        """Infer domain expertise from task domain frequency."""
        domain_counts: Counter[str] = Counter()
        for obs in observations:
            if obs.task_domain:
                domain_counts[obs.task_domain] += 1

        if not domain_counts:
            return []

        new_domains = dict(current.expertise.domains)
        changed = False
        for domain, count in domain_counts.items():
            if count >= 5:
                level = "expert"
            elif count >= 2:
                level = "intermediate"
            else:
                level = "beginner"

            if new_domains.get(domain) != level:
                new_domains[domain] = level
                changed = True

        if not changed:
            return []

        return [
            ProfileDelta(
                field_path="expertise.domains",
                old_value=dict(current.expertise.domains),
                new_value=new_domains,
                reason=f"Domain frequency: {dict(domain_counts)}",
            )
        ]

    def _analyze_strictness(
        self, observations: list[ProfileObservation], current: UserProfile
    ) -> list[ProfileDelta]:
        """Infer review strictness from REQUEST_CHANGES ratio."""
        checkpoint_obs = [
            o for o in observations if o.event_type == "checkpoint" and o.checkpoint_decision
        ]
        if len(checkpoint_obs) < 3:
            return []

        rc_count = sum(
            1 for o in checkpoint_obs if o.checkpoint_decision == "REQUEST_CHANGES"
        )
        ratio = rc_count / len(checkpoint_obs)
        new_strictness = round(ratio, 2)

        if abs(new_strictness - current.review.strictness) < 0.1:
            return []

        return [
            ProfileDelta(
                field_path="review.strictness",
                old_value=current.review.strictness,
                new_value=new_strictness,
                reason=f"REQUEST_CHANGES {rc_count}/{len(checkpoint_obs)} = {ratio:.2f}",
            )
        ]

    def _analyze_review_focus(
        self, observations: list[ProfileObservation], current: UserProfile
    ) -> list[ProfileDelta]:
        """Infer review focus from checkpoint notes keywords."""
        keyword_counts: Counter[str] = Counter()
        for obs in observations:
            if obs.dod_keywords:
                keyword_counts.update(obs.dod_keywords)

        if not keyword_counts:
            return []

        # Keep keywords that appear at least twice
        frequent = [kw for kw, count in keyword_counts.items() if count >= 2]
        if not frequent:
            return []

        # Merge with existing, keeping order stable
        merged = list(current.review.focus)
        for kw in frequent:
            if kw not in merged:
                merged.append(kw)

        if merged == list(current.review.focus):
            return []

        return [
            ProfileDelta(
                field_path="review.focus",
                old_value=list(current.review.focus),
                new_value=merged,
                reason=f"Keyword frequency: {dict(keyword_counts)}",
            )
        ]

    def _analyze_paper_criteria(
        self, observations: list[ProfileObservation], current: UserProfile
    ) -> list[ProfileDelta]:
        """Infer paper-specific criteria from notes."""
        paper_keywords = {
            "reproducibility", "citations", "methodology",
            "statistical-rigor", "baselines", "ablation",
            "experimental-design",
        }
        found: Counter[str] = Counter()
        for obs in observations:
            if obs.dod_keywords:
                for kw in obs.dod_keywords:
                    if kw in paper_keywords:
                        found[kw] += 1

        frequent = [kw for kw, count in found.items() if count >= 2]
        if not frequent:
            return []

        merged = list(current.review.paper_criteria)
        for kw in frequent:
            if kw not in merged:
                merged.append(kw)

        if merged == list(current.review.paper_criteria):
            return []

        return [
            ProfileDelta(
                field_path="review.paper_criteria",
                old_value=list(current.review.paper_criteria),
                new_value=merged,
                reason=f"Paper keyword frequency: {dict(found)}",
            )
        ]

    def _analyze_dod_detail(
        self, observations: list[ProfileObservation], current: UserProfile
    ) -> list[ProfileDelta]:
        """Infer DoD detail level from average dod_length."""
        lengths = [
            o.dod_length for o in observations
            if o.event_type == "add_todo" and o.dod_length is not None
        ]
        if len(lengths) < 3:
            return []

        avg = sum(lengths) / len(lengths)
        if avg < 50:
            level = "brief"
        elif avg < 200:
            level = "standard"
        else:
            level = "exhaustive"

        if level == current.communication.dod_detail_level:
            return []

        return [
            ProfileDelta(
                field_path="communication.dod_detail_level",
                old_value=current.communication.dod_detail_level,
                new_value=level,
                reason=f"Average DoD length: {avg:.0f} chars over {len(lengths)} tasks",
            )
        ]

    def _analyze_verbosity(
        self, observations: list[ProfileObservation], current: UserProfile
    ) -> list[ProfileDelta]:
        """Infer writing verbosity from report summary lengths."""
        lengths = [
            o.summary_length for o in observations
            if o.event_type == "report" and o.summary_length is not None
        ]
        if len(lengths) < 3:
            return []

        avg = sum(lengths) / len(lengths)
        if avg < 100:
            level = "concise"
        elif avg < 300:
            level = "moderate"
        else:
            level = "detailed"

        if level == current.writing.verbosity:
            return []

        return [
            ProfileDelta(
                field_path="writing.verbosity",
                old_value=current.writing.verbosity,
                new_value=level,
                reason=f"Average summary length: {avg:.0f} chars over {len(lengths)} reports",
            )
        ]
