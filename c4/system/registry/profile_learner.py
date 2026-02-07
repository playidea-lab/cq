"""Profile Learner - Infers profile updates from observable behavior.

Level 1 (statistical): directly observable traits
- domain frequency -> expertise.domains
- REQUEST_CHANGES ratio -> review.strictness
- checkpoint notes keywords -> review.focus, paper_criteria
- dod_length average -> communication.dod_detail_level
- summary_length average -> writing.verbosity

Level 2+3 (LLM-based): workflow weight learning
- checkpoint notes -> workflow_weights per persona

Does NOT infer: tone, thoroughness (not directly observable)
"""

from __future__ import annotations

import json as _json
import logging
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import yaml
from pydantic import BaseModel

from .profile import UserProfile, WorkflowWeight
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
        self,
        observations: list[ProfileObservation],
        current: UserProfile,
        llm_analyze: bool = True,
    ) -> list[ProfileDelta]:
        """Analyze observations and produce deltas.

        Level 1 (statistical):
        1. domain frequency -> expertise.domains
        2. REQUEST_CHANGES ratio -> review.strictness
        3. checkpoint notes keywords -> review.focus
        4. paper-related keywords -> review.paper_criteria
        5. dod_length average -> communication.dod_detail_level
        6. summary_length average -> writing.verbosity

        Level 2+3 (LLM, when llm_analyze=True):
        7. checkpoint notes -> workflow_weights per persona
        """
        deltas: list[ProfileDelta] = []

        # Level 1: statistical learning
        deltas.extend(self._analyze_domains(observations, current))
        deltas.extend(self._analyze_strictness(observations, current))
        deltas.extend(self._analyze_review_focus(observations, current))
        deltas.extend(self._analyze_paper_criteria(observations, current))
        deltas.extend(self._analyze_dod_detail(observations, current))
        deltas.extend(self._analyze_verbosity(observations, current))

        # Level 2+3: LLM-based workflow learning
        if llm_analyze:
            deltas.extend(self._analyze_workflow_with_llm(observations, current))

        return deltas

    def apply(
        self, current: UserProfile, deltas: list[ProfileDelta]
    ) -> UserProfile:
        """Apply deltas to produce updated profile.

        Supports nested dict paths (e.g. 'workflow_weights.paper-reviewer').
        Creates intermediate dicts as needed.
        Skips deltas with invalid field paths instead of crashing.
        """
        data = current.model_dump()
        for delta in deltas:
            try:
                parts = delta.field_path.split(".")
                obj = data
                for part in parts[:-1]:
                    if part not in obj:
                        obj[part] = {}
                    obj = obj[part]
                obj[parts[-1]] = delta.new_value
            except (KeyError, TypeError) as e:
                logger.warning(
                    f"Skipping invalid delta {delta.field_path}: {e}"
                )
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

    # =========================================================================
    # Level 2+3: LLM-based workflow learning
    # =========================================================================

    def _analyze_workflow_with_llm(
        self,
        observations: list[ProfileObservation],
        current: UserProfile,
    ) -> list[ProfileDelta]:
        """LLM analyzes checkpoint notes to learn workflow weights.

        Instead of static keyword mapping, LLM understands natural language:
        - Which workflow steps the user emphasizes (weight)
        - Preferred checking order (order)
        - Additional substeps not in defaults (custom_substeps)
        """
        checkpoint_obs = [
            o for o in observations
            if o.event_type == "checkpoint" and o.checkpoint_notes
        ]
        if len(checkpoint_obs) < 2:
            return []

        workflow_personas = self._load_workflow_personas()
        if not workflow_personas:
            return []

        notes_text = "\n".join(
            f"- [{o.checkpoint_decision}] {o.checkpoint_notes}"
            for o in checkpoint_obs
        )

        deltas: list[ProfileDelta] = []
        for agent_id, steps in workflow_personas.items():
            steps_text = "\n".join(
                f"  - {s['id']}: {s['description']}"
                for s in steps
            )

            prompt = (
                "사용자의 체크포인트 피드백을 분석하여 워크플로우 가중치를 결정하세요.\n\n"
                f"## 사용자 피드백 기록\n{notes_text}\n\n"
                f"## {agent_id} 워크플로우 단계\n{steps_text}\n\n"
                "## 출력 (JSON)\n"
                "각 워크플로우 단계에 대해:\n"
                "- weight: 사용자가 중시하는 정도 (0.0~1.0). 피드백에서 관련 언급이 많으면 높게.\n"
                "- order: 사용자가 원하는 확인 순서 (1부터 시작). 자주 먼저 언급되면 낮은 번호.\n"
                "- custom_substeps: 기존 단계에 없지만 사용자가 반복 요구하는 추가 확인 항목 (빈 배열 가능).\n\n"
                '언급되지 않은 단계는 weight=0.3, order는 기본 순서.\n\n'
                'JSON만 출력하세요:\n'
                '{"step_id": {"weight": 0.8, "order": 1, "custom_substeps": []}}'
            )

            try:
                result = self._call_llm(prompt)
                if result:
                    parsed = self._parse_workflow_response(
                        result, agent_id, current, len(checkpoint_obs)
                    )
                    deltas.extend(parsed)
            except Exception as e:
                logger.debug(f"LLM workflow analysis failed for {agent_id}: {e}")

        return deltas

    def _call_llm(self, prompt: str) -> str | None:
        """Call haiku for lightweight LLM analysis.

        Only called at checkpoint APPROVE (rare), so cost is minimal.
        """
        try:
            import litellm

            from c4.supervisor.claude_models import get_api_key, resolve_model_id

            model = resolve_model_id("haiku")
            api_key = get_api_key()

            response = litellm.completion(
                model=model,
                api_key=api_key,
                messages=[
                    {
                        "role": "system",
                        "content": "당신은 사용자 행동 분석가입니다. JSON만 출력하세요.",
                    },
                    {"role": "user", "content": prompt},
                ],
                temperature=0.0,
                max_tokens=1024,
                timeout=30,
            )
            return response.choices[0].message.content
        except Exception as e:
            logger.debug(f"LLM call failed: {e}")
            return None

    def _load_workflow_personas(self) -> dict[str, list[dict]]:
        """Load persona workflow_steps from agent graph."""
        try:
            from c4.supervisor.agent_graph.loader import AgentGraphLoader

            result: dict[str, list[dict]] = {}
            loader = AgentGraphLoader()
            agents = loader.load_agents()
            for agent_def in agents:
                steps = agent_def.agent.instructions and agent_def.agent.instructions.workflow_steps
                if steps:
                    result[agent_def.agent.id] = [
                        {"id": s.id, "description": s.description, "default_order": s.default_order}
                        for s in steps
                    ]
            return result
        except Exception as e:
            logger.debug(f"Failed to load workflow personas: {e}")
            return {}

    def _parse_workflow_response(
        self,
        llm_response: str,
        agent_id: str,
        current: UserProfile,
        obs_count: int,
    ) -> list[ProfileDelta]:
        """Parse LLM JSON response into ProfileDelta."""
        try:
            text = llm_response.strip()
            if "```" in text:
                parts = text.split("```")
                text = parts[1] if len(parts) > 1 else text
                if text.startswith("json"):
                    text = text[4:]
            data = _json.loads(text.strip())
        except (_json.JSONDecodeError, IndexError):
            return []

        if not isinstance(data, dict):
            return []

        new_weights: dict[str, WorkflowWeight] = {}
        for step_id, vals in data.items():
            if not isinstance(vals, dict):
                continue
            existing = (
                current.workflow_weights
                .get(agent_id, {})
                .get(step_id)
            )
            prev_count = existing.mention_count if isinstance(existing, WorkflowWeight) else 0
            new_weights[step_id] = WorkflowWeight(
                weight=min(1.0, max(0.0, float(vals.get("weight", 0.5)))),
                order=int(vals.get("order", 0)),
                mention_count=prev_count + 1,
                custom_substeps=vals.get("custom_substeps", []),
            )

        if not new_weights:
            return []

        # Serialize for comparison and storage
        new_value = {k: v.model_dump() for k, v in new_weights.items()}
        old_raw = current.workflow_weights.get(agent_id, {})
        old_value = {
            k: v.model_dump() if isinstance(v, WorkflowWeight) else v
            for k, v in old_raw.items()
        } if old_raw else {}

        if new_value == old_value:
            return []

        return [
            ProfileDelta(
                field_path=f"workflow_weights.{agent_id}",
                old_value=old_value,
                new_value=new_value,
                reason=f"LLM analysis of {obs_count} checkpoint notes",
            )
        ]
