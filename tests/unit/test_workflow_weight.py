"""Tests for WorkflowWeight model, workflow instructions, and LLM learning."""

from unittest.mock import MagicMock, patch

import pytest
import yaml

from c4.system.registry.profile import UserProfile, WorkflowWeight
from c4.system.registry.profile_learner import ProfileDelta, ProfileLearner
from c4.system.registry.profile_observer import ProfileObservation

# =============================================================================
# WorkflowWeight Model Tests
# =============================================================================


class TestWorkflowWeight:
    """Test WorkflowWeight Pydantic model."""

    def test_default_values(self):
        w = WorkflowWeight()
        assert w.weight == 0.5
        assert w.order == 0
        assert w.mention_count == 0
        assert w.custom_substeps == []

    def test_custom_values(self):
        w = WorkflowWeight(
            weight=0.9, order=1, mention_count=3,
            custom_substeps=["ablation study"],
        )
        assert w.weight == 0.9
        assert w.order == 1
        assert w.mention_count == 3
        assert w.custom_substeps == ["ablation study"]

    def test_weight_bounds(self):
        with pytest.raises(Exception):
            WorkflowWeight(weight=1.5)
        with pytest.raises(Exception):
            WorkflowWeight(weight=-0.1)

    def test_serialization_roundtrip(self):
        w = WorkflowWeight(weight=0.8, order=2, custom_substeps=["check seeds"])
        data = w.model_dump()
        restored = WorkflowWeight(**data)
        assert restored.weight == 0.8
        assert restored.order == 2
        assert restored.custom_substeps == ["check seeds"]


# =============================================================================
# UserProfile with workflow_weights Tests
# =============================================================================


class TestProfileWithWorkflowWeights:
    """Test UserProfile integration with workflow_weights."""

    def test_default_empty_weights(self):
        p = UserProfile()
        assert p.workflow_weights == {}

    def test_profile_with_weights(self):
        p = UserProfile(
            workflow_weights={
                "paper-reviewer": {
                    "methodology_check": WorkflowWeight(weight=0.9, order=1),
                    "reproducibility": WorkflowWeight(weight=0.8, order=2),
                },
            }
        )
        assert "paper-reviewer" in p.workflow_weights
        assert p.workflow_weights["paper-reviewer"]["methodology_check"].weight == 0.9

    def test_serialization_roundtrip(self):
        p = UserProfile(
            name="test",
            workflow_weights={
                "paper-reviewer": {
                    "methodology_check": WorkflowWeight(
                        weight=0.9, order=1,
                        custom_substeps=["ablation study"],
                    ),
                },
            },
        )
        data = p.model_dump()
        restored = UserProfile(**data)
        mc = restored.workflow_weights["paper-reviewer"]["methodology_check"]
        # Pydantic auto-coerces dict -> WorkflowWeight
        assert mc.weight == 0.9
        assert "ablation study" in mc.custom_substeps

    def test_yaml_roundtrip(self, tmp_path):
        p = UserProfile(
            name="test",
            workflow_weights={
                "paper-reviewer": {
                    "methodology_check": WorkflowWeight(weight=0.9, order=1),
                },
            },
        )
        path = tmp_path / "profile.yaml"
        data = p.model_dump()
        path.write_text(yaml.dump(data, allow_unicode=True))

        loaded_data = yaml.safe_load(path.read_text())
        restored = UserProfile(**loaded_data)
        mc = restored.workflow_weights["paper-reviewer"]["methodology_check"]
        # Pydantic coerces dict from YAML back to WorkflowWeight
        assert mc.weight == 0.9

    def test_apply_delta_with_workflow_weights(self, tmp_path):
        profile_path = tmp_path / "profile.yaml"
        learner = ProfileLearner(profile_path)
        current = learner.load_or_default()

        deltas = [
            ProfileDelta(
                field_path="workflow_weights.paper-reviewer",
                old_value={},
                new_value={
                    "methodology_check": {
                        "weight": 0.9, "order": 1,
                        "mention_count": 1, "custom_substeps": [],
                    },
                },
                reason="test",
            ),
        ]

        updated = learner.apply(current, deltas)
        assert "paper-reviewer" in updated.workflow_weights
        mc = updated.workflow_weights["paper-reviewer"]["methodology_check"]
        # Pydantic coerces dict -> WorkflowWeight
        assert mc.weight == 0.9


# =============================================================================
# Build Workflow Instructions Tests
# =============================================================================


class TestBuildWorkflowInstructions:
    """Test TaskOps._build_workflow_instructions."""

    def _make_task_ops(self):
        from c4.daemon.task_ops import TaskOps

        daemon = MagicMock()
        ops = TaskOps.__new__(TaskOps)
        ops._daemon = daemon
        return ops

    def _mock_steps(self):
        """Return sample workflow steps."""
        return [
            {"id": "step_a", "description": "Step A desc", "default_order": 1},
            {"id": "step_b", "description": "Step B desc", "default_order": 2},
            {"id": "step_c", "description": "Step C desc", "default_order": 3},
        ]

    def test_default_order_no_weights(self):
        ops = self._make_task_ops()
        profile = UserProfile()  # empty workflow_weights

        with patch.object(ops, "_get_base_workflow", return_value=self._mock_steps()):
            result = ops._build_workflow_instructions("paper-reviewer", profile)

        assert result is not None
        lines = result.strip().split("\n")
        assert len(lines) == 3
        # Default weight=0.5, so all MEDIUM
        assert "[MEDIUM]" in lines[0]
        assert "Step A" in lines[0]

    def test_custom_order_with_weights(self):
        ops = self._make_task_ops()
        profile = UserProfile(
            workflow_weights={
                "paper-reviewer": {
                    "step_c": WorkflowWeight(weight=0.9, order=1),
                    "step_a": WorkflowWeight(weight=0.3, order=2),
                    "step_b": WorkflowWeight(weight=0.5, order=3),
                },
            }
        )

        with patch.object(ops, "_get_base_workflow", return_value=self._mock_steps()):
            result = ops._build_workflow_instructions("paper-reviewer", profile)

        lines = result.strip().split("\n")
        # step_c should be first (order=1)
        assert "Step C" in lines[0]
        assert "[HIGH]" in lines[0]
        # step_a second (order=2)
        assert "Step A" in lines[1]
        assert "[LOW]" in lines[1]

    def test_emphasis_levels(self):
        ops = self._make_task_ops()
        profile = UserProfile(
            workflow_weights={
                "test-agent": {
                    "step_a": WorkflowWeight(weight=0.9),  # HIGH
                    "step_b": WorkflowWeight(weight=0.5),  # MEDIUM
                    "step_c": WorkflowWeight(weight=0.2),  # LOW
                },
            }
        )

        with patch.object(ops, "_get_base_workflow", return_value=self._mock_steps()):
            result = ops._build_workflow_instructions("test-agent", profile)

        assert "[HIGH]" in result
        assert "[MEDIUM]" in result
        assert "[LOW]" in result

    def test_custom_substeps(self):
        ops = self._make_task_ops()
        profile = UserProfile(
            workflow_weights={
                "paper-reviewer": {
                    "step_a": WorkflowWeight(
                        weight=0.8,
                        custom_substeps=["ablation study", "seed check"],
                    ),
                },
            }
        )

        with patch.object(ops, "_get_base_workflow", return_value=self._mock_steps()):
            result = ops._build_workflow_instructions("paper-reviewer", profile)

        assert "also check: ablation study, seed check" in result

    def test_no_workflow_steps_returns_none(self):
        ops = self._make_task_ops()
        profile = UserProfile()

        with patch.object(ops, "_get_base_workflow", return_value=None):
            result = ops._build_workflow_instructions("debugger", profile)

        assert result is None

    def test_none_agent_id_returns_none(self):
        ops = self._make_task_ops()
        profile = UserProfile()
        result = ops._build_workflow_instructions(None, profile)
        assert result is None


# =============================================================================
# LLM Workflow Analysis Tests
# =============================================================================


class TestLLMWorkflowAnalysis:
    """Test ProfileLearner LLM-based workflow analysis."""

    def test_parse_workflow_response_valid(self, tmp_path):
        learner = ProfileLearner(tmp_path / "profile.yaml")
        current = UserProfile()

        response = '{"methodology_check": {"weight": 0.9, "order": 1, "custom_substeps": ["ablation"]}}'
        deltas = learner._parse_workflow_response(response, "paper-reviewer", current, 3)
        assert len(deltas) == 1
        assert deltas[0].field_path == "workflow_weights.paper-reviewer"
        new_val = deltas[0].new_value
        assert new_val["methodology_check"]["weight"] == 0.9
        assert new_val["methodology_check"]["custom_substeps"] == ["ablation"]

    def test_parse_workflow_response_with_code_block(self, tmp_path):
        learner = ProfileLearner(tmp_path / "profile.yaml")
        current = UserProfile()

        response = '```json\n{"step_a": {"weight": 0.8, "order": 1, "custom_substeps": []}}\n```'
        deltas = learner._parse_workflow_response(response, "test-agent", current, 2)
        assert len(deltas) == 1
        assert deltas[0].new_value["step_a"]["weight"] == 0.8

    def test_parse_workflow_response_invalid_json(self, tmp_path):
        learner = ProfileLearner(tmp_path / "profile.yaml")
        current = UserProfile()

        deltas = learner._parse_workflow_response("not json", "test", current, 1)
        assert deltas == []

    def test_parse_workflow_response_no_change(self, tmp_path):
        """If new weights equal current, no delta emitted."""
        learner = ProfileLearner(tmp_path / "profile.yaml")
        current = UserProfile(
            workflow_weights={
                "test-agent": {
                    "step_a": WorkflowWeight(weight=0.8, order=1, mention_count=0),
                },
            }
        )

        response = '{"step_a": {"weight": 0.8, "order": 1, "custom_substeps": []}}'
        deltas = learner._parse_workflow_response(response, "test-agent", current, 2)
        # mention_count increments, so it's different
        assert len(deltas) == 1

    def test_analyze_skips_insufficient_observations(self, tmp_path):
        learner = ProfileLearner(tmp_path / "profile.yaml")
        current = UserProfile()

        obs = [
            ProfileObservation(
                event_type="checkpoint",
                checkpoint_decision="APPROVE",
                checkpoint_notes="good",
            ),
        ]

        deltas = learner._analyze_workflow_with_llm(obs, current)
        assert deltas == []

    def test_analyze_calls_llm_mock(self, tmp_path):
        """Test full flow with mocked LLM and loader."""
        learner = ProfileLearner(tmp_path / "profile.yaml")
        current = UserProfile()

        obs = [
            ProfileObservation(
                event_type="checkpoint",
                checkpoint_decision="REQUEST_CHANGES",
                checkpoint_notes="methodology is weak",
            ),
            ProfileObservation(
                event_type="checkpoint",
                checkpoint_decision="APPROVE",
                checkpoint_notes="methodology improved",
            ),
        ]

        mock_personas = {
            "paper-reviewer": [
                {"id": "methodology_check", "description": "Check method", "default_order": 1},
                {"id": "reproducibility", "description": "Check repro", "default_order": 2},
            ],
        }
        llm_response = (
            '{"methodology_check": {"weight": 0.9, "order": 1, "custom_substeps": []},'
            ' "reproducibility": {"weight": 0.5, "order": 2, "custom_substeps": []}}'
        )

        with patch.object(learner, "_load_workflow_personas", return_value=mock_personas), \
             patch.object(learner, "_call_llm", return_value=llm_response):
            deltas = learner._analyze_workflow_with_llm(obs, current)

        assert len(deltas) == 1
        assert deltas[0].field_path == "workflow_weights.paper-reviewer"
        assert deltas[0].new_value["methodology_check"]["weight"] == 0.9

    def test_analyze_skips_on_no_personas(self, tmp_path):
        learner = ProfileLearner(tmp_path / "profile.yaml")
        current = UserProfile()

        obs = [
            ProfileObservation(event_type="checkpoint", checkpoint_notes="test1"),
            ProfileObservation(event_type="checkpoint", checkpoint_notes="test2"),
        ]

        with patch.object(learner, "_load_workflow_personas", return_value={}):
            deltas = learner._analyze_workflow_with_llm(obs, current)

        assert deltas == []


# =============================================================================
# Builder Adapted Workflow Tests
# =============================================================================


class TestBuilderAdaptedWorkflow:
    """Test RegistryBuilder._build_user_context_section with workflow."""

    def test_adapted_workflow_in_persona_md(self, tmp_path):
        from c4.system.registry.builder import RegistryBuilder

        builder = RegistryBuilder(tmp_path)

        profile = UserProfile(
            name="test",
            workflow_weights={
                "paper-reviewer": {
                    "methodology_check": WorkflowWeight(weight=0.9, order=1),
                    "reproducibility": WorkflowWeight(weight=0.8, order=2),
                },
            },
        )

        mock_steps = [
            {"id": "methodology_check", "description": "Check method", "default_order": 2},
            {"id": "reproducibility", "description": "Check repro", "default_order": 3},
            {"id": "claims_identification", "description": "Check claims", "default_order": 1},
        ]

        with patch.object(builder, "_get_workflow_steps", return_value=mock_steps):
            section = builder._build_user_context_section("paper-reviewer", profile)

        assert section is not None
        assert "### Adapted Workflow" in section
        assert "[HIGH]" in section
        # methodology_check (order=1) should be first
        lines = section.split("\n")
        workflow_lines = [line for line in lines if line.startswith("- Step")]
        assert "Check method" in workflow_lines[0]

    def test_no_workflow_without_weights(self, tmp_path):
        from c4.system.registry.builder import RegistryBuilder

        builder = RegistryBuilder(tmp_path)
        profile = UserProfile(name="test")

        section = builder._build_user_context_section("debugger", profile)
        # No review/writer context, no weights -> None
        assert section is None
