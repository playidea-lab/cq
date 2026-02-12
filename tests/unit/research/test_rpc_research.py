"""Tests for Research RPC handlers in BridgeServer."""

import pytest

from c4.bridge.rpc_server import BridgeServer


@pytest.fixture
def server(tmp_path):
    return BridgeServer(project_root=tmp_path)


class TestResearchMethodRegistry:
    def test_has_research_methods(self, server):
        expected = [
            "ResearchStart",
            "ResearchStatus",
            "ResearchRecord",
            "ResearchApprove",
            "ResearchNext",
        ]
        for method in expected:
            assert method in server.methods, f"Missing method: {method}"


class TestResearchStart:
    @pytest.mark.asyncio
    async def test_creates_project(self, server):
        result = await server.dispatch("ResearchStart", {
            "name": "PPAD Paper 1",
            "paper_path": "/tmp/paper.pdf",
            "repo_path": "/tmp/repo",
        })
        assert result["success"] is True
        assert "project_id" in result
        assert "iteration_id" in result

    @pytest.mark.asyncio
    async def test_requires_name(self, server):
        result = await server.dispatch("ResearchStart", {})
        assert "error" in result

    @pytest.mark.asyncio
    async def test_custom_target_score(self, server):
        result = await server.dispatch("ResearchStart", {
            "name": "Test",
            "target_score": 8.5,
        })
        assert result["success"] is True
        # Verify via status
        status = await server.dispatch("ResearchStatus", {
            "project_id": result["project_id"],
        })
        assert status["project"]["target_score"] == 8.5


class TestResearchStatus:
    @pytest.mark.asyncio
    async def test_returns_history(self, server):
        start = await server.dispatch("ResearchStart", {"name": "P1"})
        pid = start["project_id"]

        status = await server.dispatch("ResearchStatus", {"project_id": pid})
        assert "project" in status
        assert "iterations" in status
        assert "current_iteration" in status
        assert status["project"]["name"] == "P1"
        assert len(status["iterations"]) == 1
        assert status["current_iteration"]["iteration_num"] == 1

    @pytest.mark.asyncio
    async def test_requires_project_id(self, server):
        result = await server.dispatch("ResearchStatus", {})
        assert "error" in result

    @pytest.mark.asyncio
    async def test_nonexistent_project(self, server):
        result = await server.dispatch("ResearchStatus", {"project_id": "nope"})
        assert "error" in result


class TestResearchRecord:
    @pytest.mark.asyncio
    async def test_updates_iteration(self, server):
        start = await server.dispatch("ResearchStart", {"name": "P1"})
        pid = start["project_id"]

        result = await server.dispatch("ResearchRecord", {
            "project_id": pid,
            "review_score": 6.5,
            "axis_scores": {"quality": 7.0, "novelty": 5.0},
            "gaps": [{"type": "experiment", "desc": "Add LOSO"}],
            "status": "planning",
        })
        assert result["success"] is True

        # Verify
        status = await server.dispatch("ResearchStatus", {"project_id": pid})
        current = status["current_iteration"]
        assert current["review_score"] == 6.5
        assert current["axis_scores"]["quality"] == 7.0
        assert len(current["gaps"]) == 1
        assert current["status"] == "planning"

    @pytest.mark.asyncio
    async def test_requires_project_id(self, server):
        result = await server.dispatch("ResearchRecord", {"review_score": 5.0})
        assert "error" in result


class TestResearchApprove:
    @pytest.mark.asyncio
    async def test_continue_creates_new_iteration(self, server):
        start = await server.dispatch("ResearchStart", {"name": "P1"})
        pid = start["project_id"]

        result = await server.dispatch("ResearchApprove", {
            "project_id": pid,
            "action": "continue",
        })
        assert result["success"] is True
        assert "iteration_id" in result

        status = await server.dispatch("ResearchStatus", {"project_id": pid})
        assert len(status["iterations"]) == 2
        assert status["current_iteration"]["iteration_num"] == 2

    @pytest.mark.asyncio
    async def test_pause(self, server):
        start = await server.dispatch("ResearchStart", {"name": "P1"})
        pid = start["project_id"]

        result = await server.dispatch("ResearchApprove", {
            "project_id": pid,
            "action": "pause",
        })
        assert result["success"] is True

        status = await server.dispatch("ResearchStatus", {"project_id": pid})
        assert status["project"]["status"] == "paused"

    @pytest.mark.asyncio
    async def test_complete(self, server):
        start = await server.dispatch("ResearchStart", {"name": "P1"})
        pid = start["project_id"]

        result = await server.dispatch("ResearchApprove", {
            "project_id": pid,
            "action": "complete",
        })
        assert result["success"] is True

        status = await server.dispatch("ResearchStatus", {"project_id": pid})
        assert status["project"]["status"] == "completed"

    @pytest.mark.asyncio
    async def test_invalid_action(self, server):
        start = await server.dispatch("ResearchStart", {"name": "P1"})
        result = await server.dispatch("ResearchApprove", {
            "project_id": start["project_id"],
            "action": "invalid",
        })
        assert "error" in result


class TestResearchNext:
    @pytest.mark.asyncio
    async def test_suggests_action(self, server):
        start = await server.dispatch("ResearchStart", {"name": "P1"})
        pid = start["project_id"]

        result = await server.dispatch("ResearchNext", {"project_id": pid})
        assert "action" in result
        assert "reason" in result
        assert result["action"] == "review"

    @pytest.mark.asyncio
    async def test_requires_project_id(self, server):
        result = await server.dispatch("ResearchNext", {})
        assert "error" in result
