"""Tests for C4 task lifecycle hook registry."""

from unittest.mock import patch

from c4.hooks.base import BaseHook, HookContext, HookPhase
from c4.hooks.registry import HookRegistry


class _DummyHook(BaseHook):
    """Test hook that tracks execution."""

    def __init__(self, name: str = "dummy", phase: HookPhase = HookPhase.AFTER_COMPLETE):
        self._name = name
        self._phase = phase
        self.called = False
        self.last_context = None
        self._enabled = True

    @property
    def name(self) -> str:
        return self._name

    @property
    def phase(self) -> HookPhase:
        return self._phase

    @property
    def enabled(self) -> bool:
        return self._enabled

    def execute(self, context: HookContext) -> bool:
        self.called = True
        self.last_context = context
        return True


class _FailingHook(BaseHook):
    """Hook that always raises."""

    @property
    def name(self) -> str:
        return "failing"

    @property
    def phase(self) -> HookPhase:
        return HookPhase.AFTER_COMPLETE

    def execute(self, context: HookContext) -> bool:
        raise RuntimeError("hook error")


class TestHookPhase:
    def test_phases_exist(self):
        assert HookPhase.BEFORE_SUBMIT == "before_submit"
        assert HookPhase.AFTER_COMPLETE == "after_complete"
        assert HookPhase.ON_FAILURE == "on_failure"

    def test_all_phases(self):
        assert len(list(HookPhase)) == 3


class TestHookContext:
    def test_basic_context(self):
        ctx = HookContext(task_id="T-001-0", phase=HookPhase.AFTER_COMPLETE)
        assert ctx.task_id == "T-001-0"
        assert ctx.phase == HookPhase.AFTER_COMPLETE
        assert ctx.task_data == {}

    def test_context_with_extras(self):
        ctx = HookContext(
            task_id="T-002-0",
            phase=HookPhase.BEFORE_SUBMIT,
            task_data={"title": "Test task"},
            commit_sha="abc123",
        )
        assert ctx.get("title") == "Test task"
        assert ctx.get("commit_sha") == "abc123"
        assert ctx.get("missing", "default") == "default"

    def test_extras_override_task_data(self):
        ctx = HookContext(
            task_id="T-003-0",
            phase=HookPhase.AFTER_COMPLETE,
            task_data={"key": "from_task"},
            key="from_extras",
        )
        assert ctx.get("key") == "from_extras"


class TestHookRegistry:
    def test_register_hook(self):
        registry = HookRegistry()
        hook = _DummyHook()
        registry.register(hook)
        assert registry.count == 1

    def test_register_duplicate_raises(self):
        registry = HookRegistry()
        registry.register(_DummyHook("dup"))
        try:
            registry.register(_DummyHook("dup"))
            assert False, "Should have raised ValueError"
        except ValueError as e:
            assert "already registered" in str(e)

    def test_register_same_name_different_phase(self):
        registry = HookRegistry()
        registry.register(_DummyHook("hook", HookPhase.AFTER_COMPLETE))
        registry.register(_DummyHook("hook", HookPhase.ON_FAILURE))
        assert registry.count == 2

    def test_execute_hooks(self):
        registry = HookRegistry()
        hook = _DummyHook()
        registry.register(hook)

        ctx = HookContext(task_id="T-001-0", phase=HookPhase.AFTER_COMPLETE)
        results = registry.execute(HookPhase.AFTER_COMPLETE, ctx)

        assert len(results) == 1
        assert results[0]["hook"] == "dummy"
        assert results[0]["success"] is True
        assert hook.called is True
        assert hook.last_context is ctx

    def test_execute_no_hooks_for_phase(self):
        registry = HookRegistry()
        ctx = HookContext(task_id="T-001-0", phase=HookPhase.BEFORE_SUBMIT)
        results = registry.execute(HookPhase.BEFORE_SUBMIT, ctx)
        assert results == []

    def test_execute_disabled_hook(self):
        registry = HookRegistry()
        hook = _DummyHook()
        hook._enabled = False
        registry.register(hook)

        ctx = HookContext(task_id="T-001-0", phase=HookPhase.AFTER_COMPLETE)
        results = registry.execute(HookPhase.AFTER_COMPLETE, ctx)

        assert len(results) == 1
        assert results[0]["error"] == "disabled"
        assert hook.called is False

    def test_execute_failing_hook(self):
        registry = HookRegistry()
        registry.register(_FailingHook())

        ctx = HookContext(task_id="T-001-0", phase=HookPhase.AFTER_COMPLETE)
        results = registry.execute(HookPhase.AFTER_COMPLETE, ctx)

        assert len(results) == 1
        assert results[0]["success"] is False
        assert "hook error" in results[0]["error"]

    def test_execute_multiple_hooks_order(self):
        registry = HookRegistry()
        order = []

        class OrderHook(BaseHook):
            def __init__(self, n):
                self._name = f"hook_{n}"
                self._n = n

            @property
            def name(self):
                return self._name

            @property
            def phase(self):
                return HookPhase.AFTER_COMPLETE

            def execute(self, context):
                order.append(self._n)
                return True

        registry.register(OrderHook(1))
        registry.register(OrderHook(2))
        registry.register(OrderHook(3))

        ctx = HookContext(task_id="T-001-0", phase=HookPhase.AFTER_COMPLETE)
        registry.execute(HookPhase.AFTER_COMPLETE, ctx)

        assert order == [1, 2, 3]

    def test_unregister_hook(self):
        registry = HookRegistry()
        registry.register(_DummyHook("to_remove"))
        assert registry.count == 1

        removed = registry.unregister("to_remove")
        assert removed is True
        assert registry.count == 0

    def test_unregister_nonexistent(self):
        registry = HookRegistry()
        removed = registry.unregister("nonexistent")
        assert removed is False

    def test_unregister_specific_phase(self):
        registry = HookRegistry()
        registry.register(_DummyHook("multi", HookPhase.AFTER_COMPLETE))
        registry.register(_DummyHook("multi", HookPhase.ON_FAILURE))
        assert registry.count == 2

        registry.unregister("multi", phase=HookPhase.AFTER_COMPLETE)
        assert registry.count == 1

    def test_list_hooks(self):
        registry = HookRegistry()
        registry.register(_DummyHook("a", HookPhase.AFTER_COMPLETE))
        registry.register(_DummyHook("b", HookPhase.ON_FAILURE))

        all_hooks = registry.list_hooks()
        assert len(all_hooks) == 2

        after_hooks = registry.list_hooks(phase=HookPhase.AFTER_COMPLETE)
        assert len(after_hooks) == 1
        assert after_hooks[0]["name"] == "a"


class TestKnowledgeHook:
    def test_skip_when_no_execution_stats(self):
        from c4.hooks.builtin.knowledge_hook import KnowledgeHook

        hook = KnowledgeHook()
        assert hook.name == "knowledge_auto_save"
        assert hook.phase == HookPhase.AFTER_COMPLETE

        ctx = HookContext(
            task_id="T-001-0",
            phase=HookPhase.AFTER_COMPLETE,
            task_data={"title": "No stats"},
        )
        assert hook.execute(ctx) is True  # No-op, not a failure

    @patch("c4.hooks.builtin.knowledge_hook._save_to_knowledge_store")
    def test_saves_with_execution_stats(self, mock_save):
        from c4.hooks.builtin.knowledge_hook import KnowledgeHook

        hook = KnowledgeHook()
        ctx = HookContext(
            task_id="T-ML-001-0",
            phase=HookPhase.AFTER_COMPLETE,
            task_data={
                "title": "Train model",
                "execution_stats": {
                    "metrics": {"loss": 0.5},
                    "code_features": {"imports": ["torch"]},
                    "run_time_sec": 120.5,
                },
            },
        )
        result = hook.execute(ctx)
        assert result is True
        mock_save.assert_called_once()


class TestArtifactHook:
    def test_skip_when_no_workspace(self):
        from c4.hooks.builtin.artifact_hook import ArtifactHook

        hook = ArtifactHook()
        assert hook.name == "artifact_auto_detect"
        assert hook.phase == HookPhase.AFTER_COMPLETE

        ctx = HookContext(
            task_id="T-001-0",
            phase=HookPhase.AFTER_COMPLETE,
        )
        assert hook.execute(ctx) is True

    def test_detects_artifacts(self):
        from c4.hooks.builtin.artifact_hook import ArtifactHook

        mock_results = [
            {"name": "model.pt", "path": "/tmp/work/model.pt", "size_bytes": 1024, "type": "output"}
        ]

        with patch("c4.artifacts.detector.scan_outputs", return_value=mock_results) as mock_scan:
            hook = ArtifactHook()
            ctx = HookContext(
                task_id="T-ML-001-0",
                phase=HookPhase.AFTER_COMPLETE,
                workspace="/tmp/work",
            )
            result = hook.execute(ctx)
            assert result is True
            mock_scan.assert_called_once_with("/tmp/work")
