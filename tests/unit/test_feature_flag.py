"""Unit tests for C4_USE_GRAPH_ROUTER feature flag.

Tests cover:
1. Feature flag detection
2. GraphRouter used when flag is True (default)
3. Legacy AgentRouter used when flag is False
"""

from __future__ import annotations

import os
import warnings
from unittest import mock

import pytest


class TestUseGraphRouterFlag:
    """Tests for _use_graph_router function."""

    def test_default_is_true(self) -> None:
        """Should default to True when env var not set."""
        from c4.mcp_server import _use_graph_router

        with mock.patch.dict(os.environ, {}, clear=True):
            # Clear the env var to test default
            os.environ.pop("C4_USE_GRAPH_ROUTER", None)
            assert _use_graph_router() is True

    def test_explicit_true_values(self) -> None:
        """Should return True for various true values."""
        from c4.mcp_server import _use_graph_router

        for value in ["true", "True", "TRUE", "1", "yes", "YES", "on", "ON"]:
            with mock.patch.dict(os.environ, {"C4_USE_GRAPH_ROUTER": value}):
                assert _use_graph_router() is True, f"Failed for value: {value}"

    def test_explicit_false_values(self) -> None:
        """Should return False for various false values."""
        from c4.mcp_server import _use_graph_router

        for value in ["false", "False", "FALSE", "0", "no", "NO", "off", "OFF"]:
            with mock.patch.dict(os.environ, {"C4_USE_GRAPH_ROUTER": value}):
                assert _use_graph_router() is False, f"Failed for value: {value}"


class TestFeatureFlagIntegration:
    """Tests for feature flag integration with routing."""

    def test_graph_router_used_by_default(self) -> None:
        """GraphRouter should be used when flag is True."""
        from c4.supervisor.agent_graph import GraphRouter

        # Verify GraphRouter is importable and works
        router = GraphRouter()
        config = router.get_recommended_agent("web-backend")
        assert config.primary is not None

    def test_legacy_router_deprecation_warning(self) -> None:
        """Legacy AgentRouter should emit deprecation warning."""
        from c4.supervisor.agent_router import AgentRouter

        with warnings.catch_warnings(record=True) as w:
            warnings.simplefilter("always")
            _router = AgentRouter()

            # Check that a deprecation warning was issued
            assert len(w) >= 1
            assert issubclass(w[0].category, DeprecationWarning)
            assert "deprecated" in str(w[0].message).lower()

    def test_legacy_function_deprecation_warning(self) -> None:
        """Legacy get_recommended_agent function should emit deprecation warning."""
        from c4.supervisor.agent_router import get_recommended_agent

        with warnings.catch_warnings(record=True) as w:
            warnings.simplefilter("always")
            _config = get_recommended_agent("web-backend")

            # Check that a deprecation warning was issued
            assert len(w) >= 1
            assert issubclass(w[0].category, DeprecationWarning)
            assert "deprecated" in str(w[0].message).lower()


class TestGraphRouterProperty:
    """Tests for C4Daemon.graph_router property."""

    @pytest.fixture
    def daemon(self, tmp_path):
        """Create a C4Daemon instance with temporary directory."""
        from c4.mcp_server import C4Daemon

        # Create .c4 directory
        c4_dir = tmp_path / ".c4"
        c4_dir.mkdir()

        return C4Daemon(project_root=tmp_path)

    def test_graph_router_property_creates_router(self, daemon) -> None:
        """graph_router property should create GraphRouter."""
        from c4.supervisor.agent_graph import GraphRouter

        router = daemon.graph_router
        assert isinstance(router, GraphRouter)

    def test_graph_router_property_cached(self, daemon) -> None:
        """graph_router property should return same instance."""
        router1 = daemon.graph_router
        router2 = daemon.graph_router
        assert router1 is router2

    def test_graph_router_has_skill_matcher(self, daemon) -> None:
        """graph_router should have skill_matcher configured."""
        router = daemon.graph_router
        assert router._skill_matcher is not None

    def test_graph_router_has_rule_engine(self, daemon) -> None:
        """graph_router should have rule_engine configured."""
        router = daemon.graph_router
        assert router._rule_engine is not None
