"""Tests for User Profile system: models, observer, learner, loader.

Covers: unit tests, integration tests for MCP handler hooks,
_build_user_context, _load_user_profile_context, _learn_and_rebuild.
"""

from unittest.mock import MagicMock, patch

import pytest
import yaml

from c4.system.registry.profile import (
    DomainExpertise,
    ReviewStyle,
    UserProfile,
    WritingStyle,
)
from c4.system.registry.profile_learner import ProfileDelta, ProfileLearner
from c4.system.registry.profile_loader import ProfileLoader
from c4.system.registry.profile_observer import (
    ProfileObservation,
    ProfileObserver,
    _extract_keywords,
)

# =============================================================================
# Profile Model Tests
# =============================================================================


class TestUserProfile:
    """Test UserProfile Pydantic model."""

    def test_default_profile(self):
        profile = UserProfile()
        assert profile.name == "default"
        assert profile.version == 0
        assert profile.review.strictness == 0.5
        assert profile.review.focus == ["correctness", "clarity"]
        assert profile.writing.language == "en"
        assert profile.communication.dod_detail_level == "standard"
        assert profile.expertise.domains == {}

    def test_custom_profile(self):
        profile = UserProfile(
            name="changmin",
            review=ReviewStyle(strictness=0.8, focus=["testing", "security"]),
            writing=WritingStyle(language="ko", formality="academic"),
            expertise=DomainExpertise(
                domains={"ml-dl": "expert", "web-frontend": "intermediate"},
                research_fields=["computer-vision", "pose-estimation"],
            ),
        )
        assert profile.name == "changmin"
        assert profile.review.strictness == 0.8
        assert profile.writing.language == "ko"
        assert profile.expertise.domains["ml-dl"] == "expert"
        assert "computer-vision" in profile.expertise.research_fields

    def test_strictness_bounds(self):
        with pytest.raises(Exception):
            ReviewStyle(strictness=1.5)
        with pytest.raises(Exception):
            ReviewStyle(strictness=-0.1)

    def test_serialization_roundtrip(self):
        profile = UserProfile(
            name="test",
            review=ReviewStyle(paper_criteria=["reproducibility"]),
        )
        data = profile.model_dump()
        restored = UserProfile(**data)
        assert restored.name == "test"
        assert restored.review.paper_criteria == ["reproducibility"]


# =============================================================================
# Observer Tests
# =============================================================================


class TestProfileObserver:
    """Test ProfileObserver observation recording."""

    def test_record_add_todo(self, tmp_path):
        observer = ProfileObserver(tmp_path)
        observer.record_add_todo(
            title="Fix security bug",
            dod="Add security tests, pass lint",
            domain="web-backend",
        )
        obs = observer.get_all()
        assert len(obs) == 1
        assert obs[0].event_type == "add_todo"
        assert obs[0].task_domain == "web-backend"
        assert obs[0].dod_length == len("Add security tests, pass lint")
        assert "security" in obs[0].dod_keywords

    def test_record_checkpoint(self, tmp_path):
        observer = ProfileObserver(tmp_path)
        observer.record_checkpoint(
            decision="REQUEST_CHANGES",
            notes="Need more test coverage and reproducibility check",
            required_changes=["Add unit tests", "Add seed for reproducibility"],
        )
        obs = observer.get_all()
        assert len(obs) == 1
        assert obs[0].checkpoint_decision == "REQUEST_CHANGES"
        assert "testing" in obs[0].dod_keywords
        assert "reproducibility" in obs[0].dod_keywords

    def test_record_report(self, tmp_path):
        observer = ProfileObserver(tmp_path)
        observer.record_report(
            summary="Implemented feature X with 3 files changed",
            files_changed=["a.py", "b.py", "c.py"],
        )
        obs = observer.get_all()
        assert len(obs) == 1
        assert obs[0].event_type == "report"
        assert obs[0].files_changed_count == 3

    def test_record_submit(self, tmp_path):
        observer = ProfileObserver(tmp_path)
        observer.record_submit(task_domain="ml-dl")
        obs = observer.get_all()
        assert len(obs) == 1
        assert obs[0].task_domain == "ml-dl"

    def test_clear(self, tmp_path):
        observer = ProfileObserver(tmp_path)
        observer.record_submit(task_domain="ml-dl")
        assert len(observer.get_all()) == 1
        observer.clear()
        assert len(observer.get_all()) == 0

    def test_multiple_observations(self, tmp_path):
        observer = ProfileObserver(tmp_path)
        for i in range(5):
            observer.record_add_todo(f"Task {i}", f"DoD {i}", "ml-dl")
        assert len(observer.get_all()) == 5

    def test_corrupted_file_recovery(self, tmp_path):
        path = tmp_path / "profile_observations.json"
        path.write_text("not valid json")
        observer = ProfileObserver(tmp_path)
        assert observer.get_all() == []
        observer.record_submit(task_domain="test")
        assert len(observer.get_all()) == 1


class TestExtractKeywords:
    """Test keyword extraction helper."""

    def test_security_keyword(self):
        assert "security" in _extract_keywords("Check security vulnerabilities")

    def test_testing_keyword(self):
        kw = _extract_keywords("Add test coverage")
        assert "testing" in kw

    def test_paper_keywords(self):
        kw = _extract_keywords("Check reproducibility and methodology")
        assert "reproducibility" in kw
        assert "methodology" in kw

    def test_empty_text(self):
        assert _extract_keywords("") == []
        assert _extract_keywords(None) == []

    def test_no_match(self):
        assert _extract_keywords("hello world foo bar") == []


# =============================================================================
# Learner Tests
# =============================================================================


class TestProfileLearner:
    """Test ProfileLearner inference logic."""

    def _make_observations(self, count=5, **overrides) -> list[ProfileObservation]:
        return [
            ProfileObservation(event_type="add_todo", **overrides)
            for _ in range(count)
        ]

    def test_analyze_domain_expertise(self, tmp_path):
        profile_path = tmp_path / "profile.yaml"
        learner = ProfileLearner(profile_path)
        current = learner.load_or_default()

        obs = [
            ProfileObservation(event_type="add_todo", task_domain="ml-dl")
            for _ in range(5)
        ] + [
            ProfileObservation(event_type="add_todo", task_domain="web-backend")
            for _ in range(3)
        ]

        deltas = learner.analyze(obs, current)
        domain_delta = [d for d in deltas if d.field_path == "expertise.domains"]
        assert len(domain_delta) == 1
        new_domains = domain_delta[0].new_value
        assert new_domains["ml-dl"] == "expert"
        assert new_domains["web-backend"] == "intermediate"

    def test_analyze_strictness(self, tmp_path):
        profile_path = tmp_path / "profile.yaml"
        learner = ProfileLearner(profile_path)
        current = learner.load_or_default()

        # 3 checkpoints: 2 REQUEST_CHANGES, 1 APPROVE -> strictness ~0.67
        obs = [
            ProfileObservation(event_type="checkpoint", checkpoint_decision="REQUEST_CHANGES"),
            ProfileObservation(event_type="checkpoint", checkpoint_decision="REQUEST_CHANGES"),
            ProfileObservation(event_type="checkpoint", checkpoint_decision="APPROVE"),
        ]

        deltas = learner.analyze(obs, current)
        strictness_delta = [d for d in deltas if d.field_path == "review.strictness"]
        assert len(strictness_delta) == 1
        assert strictness_delta[0].new_value == pytest.approx(0.67, abs=0.01)

    def test_analyze_review_focus(self, tmp_path):
        profile_path = tmp_path / "profile.yaml"
        learner = ProfileLearner(profile_path)
        current = learner.load_or_default()

        obs = [
            ProfileObservation(event_type="checkpoint", dod_keywords=["testing", "security"]),
            ProfileObservation(event_type="checkpoint", dod_keywords=["testing"]),
            ProfileObservation(event_type="add_todo", dod_keywords=["testing", "performance"]),
        ]

        deltas = learner.analyze(obs, current)
        focus_delta = [d for d in deltas if d.field_path == "review.focus"]
        assert len(focus_delta) == 1
        assert "testing" in focus_delta[0].new_value

    def test_analyze_paper_criteria(self, tmp_path):
        profile_path = tmp_path / "profile.yaml"
        learner = ProfileLearner(profile_path)
        current = learner.load_or_default()

        obs = [
            ProfileObservation(event_type="checkpoint", dod_keywords=["reproducibility"]),
            ProfileObservation(event_type="checkpoint", dod_keywords=["reproducibility", "methodology"]),
            ProfileObservation(event_type="checkpoint", dod_keywords=["methodology"]),
        ]

        deltas = learner.analyze(obs, current)
        paper_delta = [d for d in deltas if d.field_path == "review.paper_criteria"]
        assert len(paper_delta) == 1
        assert "reproducibility" in paper_delta[0].new_value
        assert "methodology" in paper_delta[0].new_value

    def test_analyze_dod_detail(self, tmp_path):
        profile_path = tmp_path / "profile.yaml"
        learner = ProfileLearner(profile_path)
        current = learner.load_or_default()

        # Average length > 200 -> exhaustive
        obs = [
            ProfileObservation(event_type="add_todo", dod_length=300),
            ProfileObservation(event_type="add_todo", dod_length=250),
            ProfileObservation(event_type="add_todo", dod_length=280),
        ]

        deltas = learner.analyze(obs, current)
        dod_delta = [d for d in deltas if d.field_path == "communication.dod_detail_level"]
        assert len(dod_delta) == 1
        assert dod_delta[0].new_value == "exhaustive"

    def test_analyze_verbosity(self, tmp_path):
        profile_path = tmp_path / "profile.yaml"
        learner = ProfileLearner(profile_path)
        current = learner.load_or_default()

        # Average length < 100 -> concise
        obs = [
            ProfileObservation(event_type="report", summary_length=50),
            ProfileObservation(event_type="report", summary_length=60),
            ProfileObservation(event_type="report", summary_length=70),
        ]

        deltas = learner.analyze(obs, current)
        verb_delta = [d for d in deltas if d.field_path == "writing.verbosity"]
        assert len(verb_delta) == 1
        assert verb_delta[0].new_value == "concise"

    def test_apply_deltas(self, tmp_path):
        profile_path = tmp_path / "profile.yaml"
        learner = ProfileLearner(profile_path)
        current = learner.load_or_default()

        deltas = [
            ProfileDelta(
                field_path="review.strictness",
                old_value=0.5,
                new_value=0.7,
                reason="test",
            ),
            ProfileDelta(
                field_path="expertise.domains",
                old_value={},
                new_value={"ml-dl": "expert"},
                reason="test",
            ),
        ]

        updated = learner.apply(current, deltas)
        assert updated.review.strictness == 0.7
        assert updated.expertise.domains == {"ml-dl": "expert"}

    def test_save_and_load(self, tmp_path):
        profile_path = tmp_path / "profile.yaml"
        learner = ProfileLearner(profile_path)

        profile = UserProfile(name="test-user")
        learner.save(profile)

        loaded = learner.load_or_default()
        assert loaded.name == "test-user"
        assert loaded.version == 1
        assert loaded.last_updated is not None

    def test_insufficient_observations(self, tmp_path):
        """With <3 checkpoints, strictness should not be inferred."""
        profile_path = tmp_path / "profile.yaml"
        learner = ProfileLearner(profile_path)
        current = learner.load_or_default()

        obs = [
            ProfileObservation(event_type="checkpoint", checkpoint_decision="REQUEST_CHANGES"),
        ]

        deltas = learner.analyze(obs, current)
        strictness_delta = [d for d in deltas if d.field_path == "review.strictness"]
        assert len(strictness_delta) == 0

    def test_no_change_no_delta(self, tmp_path):
        """If learned value equals current, no delta emitted."""
        profile_path = tmp_path / "profile.yaml"
        learner = ProfileLearner(profile_path)
        current = UserProfile(
            writing=WritingStyle(verbosity="moderate")
        )

        # Average 150 -> "moderate" same as current
        obs = [
            ProfileObservation(event_type="report", summary_length=150),
            ProfileObservation(event_type="report", summary_length=150),
            ProfileObservation(event_type="report", summary_length=150),
        ]

        deltas = learner.analyze(obs, current)
        verb_delta = [d for d in deltas if d.field_path == "writing.verbosity"]
        assert len(verb_delta) == 0


# =============================================================================
# Loader Tests
# =============================================================================


class TestProfileLoader:
    """Test ProfileLoader with fallback chain."""

    def test_load_global_profile(self, tmp_path, monkeypatch):
        # Create global profile
        global_dir = tmp_path / "home" / ".c4"
        global_dir.mkdir(parents=True)
        global_profile = global_dir / "profile.yaml"
        global_profile.write_text(
            yaml.dump({"name": "global-user", "version": 1})
        )

        c4_dir = tmp_path / "project" / ".c4"
        c4_dir.mkdir(parents=True)

        loader = ProfileLoader(c4_dir)
        loader.global_path = global_profile

        result = loader.load(user="testuser")
        assert result is not None
        assert result.name == "global-user"

    def test_load_project_profile(self, tmp_path):
        c4_dir = tmp_path / ".c4"
        profiles_dir = c4_dir / "profiles"
        profiles_dir.mkdir(parents=True)

        project_profile = profiles_dir / "testuser.yaml"
        project_profile.write_text(
            yaml.dump({"name": "project-user", "version": 2})
        )

        loader = ProfileLoader(c4_dir)
        loader.global_path = tmp_path / "nonexistent" / "profile.yaml"

        result = loader.load(user="testuser")
        assert result is not None
        assert result.name == "project-user"

    def test_project_overrides_global(self, tmp_path):
        c4_dir = tmp_path / ".c4"
        c4_dir.mkdir(parents=True)

        # Global
        global_path = tmp_path / "global_profile.yaml"
        global_path.write_text(
            yaml.dump({
                "name": "global",
                "review": {"strictness": 0.3, "focus": ["correctness"]},
            })
        )

        # Project override
        profiles_dir = c4_dir / "profiles"
        profiles_dir.mkdir()
        project_path = profiles_dir / "user.yaml"
        project_path.write_text(
            yaml.dump({
                "name": "project",
                "review": {"strictness": 0.8},
            })
        )

        loader = ProfileLoader(c4_dir)
        loader.global_path = global_path

        result = loader.load(user="user")
        assert result is not None
        # Project override wins for non-default value
        assert result.review.strictness == 0.8

    def test_no_profile_returns_none(self, tmp_path):
        c4_dir = tmp_path / ".c4"
        c4_dir.mkdir(parents=True)

        loader = ProfileLoader(c4_dir)
        loader.global_path = tmp_path / "nonexistent.yaml"

        result = loader.load(user="nobody")
        assert result is None

    def test_install_template(self, tmp_path):
        target = tmp_path / "profile.yaml"
        result = ProfileLoader.install_template(target)
        assert result == target
        assert target.exists()
        content = yaml.safe_load(target.read_text())
        assert "name" in content
        assert content["review"]["strictness"] == 0.5

    def test_install_template_preserves_existing(self, tmp_path):
        target = tmp_path / "profile.yaml"
        target.write_text("name: existing")

        ProfileLoader.install_template(target)
        # Should NOT overwrite
        assert yaml.safe_load(target.read_text())["name"] == "existing"


# =============================================================================
# Learner: apply() defensive guard tests
# =============================================================================


class TestProfileLearnerApplyDefensive:
    """Test that apply() handles invalid field paths gracefully."""

    def test_apply_invalid_field_path_skipped(self, tmp_path):
        """Invalid delta should be skipped, valid ones applied."""
        profile_path = tmp_path / "profile.yaml"
        learner = ProfileLearner(profile_path)
        current = learner.load_or_default()

        deltas = [
            ProfileDelta(
                field_path="nonexistent.field",
                old_value=None,
                new_value="bad",
                reason="test invalid path",
            ),
            ProfileDelta(
                field_path="review.strictness",
                old_value=0.5,
                new_value=0.9,
                reason="test valid path",
            ),
        ]

        updated = learner.apply(current, deltas)
        # Valid delta applied, invalid skipped
        assert updated.review.strictness == 0.9

    def test_apply_empty_deltas(self, tmp_path):
        profile_path = tmp_path / "profile.yaml"
        learner = ProfileLearner(profile_path)
        current = UserProfile(name="test")

        updated = learner.apply(current, [])
        assert updated.name == "test"


# =============================================================================
# Integration: MCP handler → observer hook tests
# =============================================================================


class TestMCPHandlerObserverIntegration:
    """Test that MCP handlers correctly call profile observer."""

    def test_handle_add_todo_records_observation(self, tmp_path):
        from c4.mcp.handlers.tasks import handle_add_todo

        daemon = MagicMock()
        daemon.c4_add_todo.return_value = {"status": "ok"}
        daemon.profile_observer = ProfileObserver(tmp_path)

        handle_add_todo(daemon, {
            "task_id": "T-001-0",
            "title": "Add security tests",
            "dod": "Write unit tests for auth module, coverage > 80%",
            "domain": "web-backend",
        })

        obs = daemon.profile_observer.get_all()
        assert len(obs) == 1
        assert obs[0].event_type == "add_todo"
        assert obs[0].task_domain == "web-backend"
        assert "testing" in obs[0].dod_keywords

    def test_handle_checkpoint_records_observation(self, tmp_path):
        from c4.mcp.handlers.supervisor import handle_checkpoint

        daemon = MagicMock()
        result_mock = MagicMock()
        result_mock.model_dump.return_value = {"success": True}
        daemon.c4_checkpoint.return_value = result_mock
        daemon.profile_observer = ProfileObserver(tmp_path)

        handle_checkpoint(daemon, {
            "checkpoint_id": "CP-001",
            "decision": "REQUEST_CHANGES",
            "notes": "Need reproducibility verification",
            "required_changes": ["Add seeds"],
        })

        obs = daemon.profile_observer.get_all()
        assert len(obs) == 1
        assert obs[0].checkpoint_decision == "REQUEST_CHANGES"
        assert "reproducibility" in obs[0].dod_keywords

    def test_handle_report_records_observation(self, tmp_path):
        from c4.mcp.handlers.tracking import handle_report

        daemon = MagicMock()
        daemon.c4_report.return_value = {"status": "ok"}
        daemon.profile_observer = ProfileObserver(tmp_path)

        handle_report(daemon, {
            "task_id": "T-001-0",
            "summary": "Implemented auth module with 5 files",
            "files_changed": ["a.py", "b.py", "c.py", "d.py", "e.py"],
        })

        obs = daemon.profile_observer.get_all()
        assert len(obs) == 1
        assert obs[0].event_type == "report"
        assert obs[0].summary_length == len("Implemented auth module with 5 files")
        assert obs[0].files_changed_count == 5

    def test_handle_submit_records_observation(self, tmp_path):
        from c4.mcp.handlers.tasks import handle_submit

        daemon = MagicMock()
        result_mock = MagicMock()
        result_mock.model_dump.return_value = {"success": True}
        daemon.c4_submit.return_value = result_mock

        task_mock = MagicMock()
        task_mock.domain = "ml-dl"
        daemon.get_task.return_value = task_mock
        daemon.profile_observer = ProfileObserver(tmp_path)

        handle_submit(daemon, {
            "task_id": "T-001-0",
            "commit_sha": "abc123",
            "validation_results": {"lint": "pass"},
        })

        obs = daemon.profile_observer.get_all()
        assert len(obs) == 1
        assert obs[0].event_type == "submit"
        assert obs[0].task_domain == "ml-dl"

    def test_handler_continues_on_observer_failure(self, tmp_path):
        """Observer failure should never crash the MCP handler."""
        from c4.mcp.handlers.tasks import handle_add_todo

        daemon = MagicMock()
        daemon.c4_add_todo.return_value = {"status": "ok"}
        # Simulate observer failure
        daemon.profile_observer.record_add_todo.side_effect = RuntimeError("disk full")

        result = handle_add_todo(daemon, {
            "task_id": "T-001-0",
            "title": "Test",
            "dod": "DoD",
        })
        # Handler still returns successfully
        assert result == {"status": "ok"}


# =============================================================================
# Integration: _build_user_context tests
# =============================================================================


class TestBuildUserContext:
    """Test TaskOps._build_user_context with mock profiles."""

    def _make_task_ops(self, tmp_path):
        """Create TaskOps with mock daemon pointing to tmp profile."""
        from c4.daemon.task_ops import TaskOps

        daemon = MagicMock()
        daemon.c4_dir = tmp_path / ".c4"
        daemon.c4_dir.mkdir(parents=True)

        ops = TaskOps.__new__(TaskOps)
        ops._daemon = daemon
        return ops

    def _install_profile(self, c4_dir, **overrides):
        """Install a profile.yaml for testing."""
        profile = UserProfile(
            name="test-user",
            review=ReviewStyle(
                focus=["correctness", "testing"],
                strictness=0.7,
                paper_criteria=["reproducibility"],
            ),
            writing=WritingStyle(language="ko", formality="academic"),
            expertise=DomainExpertise(
                domains={"ml-dl": "expert"},
                research_fields=["pose-estimation"],
            ),
            persona_overrides={"paper-reviewer": "Focus on statistical rigor"},
            **overrides,
        )
        # Use project-level profile instead of touching global
        profiles_dir = c4_dir / "profiles"
        profiles_dir.mkdir(parents=True, exist_ok=True)
        profile_path = profiles_dir / "test-user.yaml"
        profile_path.write_text(
            yaml.dump(profile.model_dump(), allow_unicode=True)
        )
        return profile_path

    def test_reviewer_gets_review_context(self, tmp_path):
        ops = self._make_task_ops(tmp_path)
        self._install_profile(ops._daemon.c4_dir)

        with patch(
            "c4.system.registry.profile_loader._get_git_user_name",
            return_value="test-user",
        ):
            ctx = ops._build_user_context(agent_id="paper-reviewer", task=MagicMock())

        assert ctx is not None
        assert "Review focus:" in ctx
        assert "testing" in ctx
        assert "Strictness: 0.7" in ctx
        assert "Paper criteria:" in ctx
        assert "reproducibility" in ctx
        assert "Special instructions:" in ctx
        assert "statistical rigor" in ctx

    def test_writer_gets_writing_context(self, tmp_path):
        ops = self._make_task_ops(tmp_path)
        self._install_profile(ops._daemon.c4_dir)

        with patch(
            "c4.system.registry.profile_loader._get_git_user_name",
            return_value="test-user",
        ):
            ctx = ops._build_user_context(agent_id="paper-writer", task=MagicMock())

        assert ctx is not None
        assert "Language: ko" in ctx
        assert "Formality: academic" in ctx

    def test_generic_agent_gets_expertise(self, tmp_path):
        ops = self._make_task_ops(tmp_path)
        self._install_profile(ops._daemon.c4_dir)

        with patch(
            "c4.system.registry.profile_loader._get_git_user_name",
            return_value="test-user",
        ):
            ctx = ops._build_user_context(agent_id="ml-engineer", task=MagicMock())

        assert ctx is not None
        assert "Expertise:" in ctx
        assert "ml-dl(expert)" in ctx
        assert "Research fields:" in ctx

    def test_no_profile_returns_none(self, tmp_path):
        ops = self._make_task_ops(tmp_path)
        # No profile installed

        with patch(
            "c4.system.registry.profile_loader._get_git_user_name",
            return_value="nobody",
        ):
            ctx = ops._build_user_context(agent_id="debugger", task=MagicMock())

        assert ctx is None


# =============================================================================
# Integration: _learn_and_rebuild tests
# =============================================================================


class TestLearnAndRebuild:
    """Test CheckpointOps._learn_and_rebuild integration."""

    def test_learn_updates_profile_on_sufficient_observations(self, tmp_path):
        from c4.daemon.checkpoint_ops import CheckpointOps

        # Setup daemon mock
        daemon = MagicMock()
        daemon.c4_dir = tmp_path / ".c4"
        daemon.c4_dir.mkdir(parents=True)
        daemon.root = tmp_path

        # Setup observer with enough data
        observer = ProfileObserver(daemon.c4_dir)
        for _ in range(3):
            observer.record_checkpoint(
                decision="REQUEST_CHANGES",
                notes="Need more test coverage",
            )
        daemon.profile_observer = observer

        ops = CheckpointOps.__new__(CheckpointOps)
        ops._daemon = daemon

        # Mock RegistryBuilder (lazy import inside _learn_and_rebuild)
        with patch("c4.system.registry.builder.RegistryBuilder"):
            ops._learn_and_rebuild("CP-001")

        # Observations should be cleared after learning
        assert len(observer.get_all()) == 0

    def test_learn_skips_with_few_observations(self, tmp_path):
        from c4.daemon.checkpoint_ops import CheckpointOps

        daemon = MagicMock()
        daemon.c4_dir = tmp_path / ".c4"
        daemon.c4_dir.mkdir(parents=True)

        observer = ProfileObserver(daemon.c4_dir)
        observer.record_checkpoint(decision="APPROVE", notes="ok")
        daemon.profile_observer = observer

        ops = CheckpointOps.__new__(CheckpointOps)
        ops._daemon = daemon

        ops._learn_and_rebuild("CP-001")

        # Not cleared because <3 observations
        assert len(observer.get_all()) == 1

    def test_learn_does_not_crash_on_failure(self, tmp_path):
        from c4.daemon.checkpoint_ops import CheckpointOps

        daemon = MagicMock()
        daemon.profile_observer.get_all.side_effect = RuntimeError("boom")

        ops = CheckpointOps.__new__(CheckpointOps)
        ops._daemon = daemon

        # Should not raise
        ops._learn_and_rebuild("CP-001")
