"""Tests for C2 RPC handlers in the Bridge Server.

Tests the 8 C2 methods registered via _register_c2_methods():
C2ParseDocument, C2ExtractText, C2WorkspaceCreate, C2WorkspaceLoad,
C2WorkspaceSave, C2PersonaLearn, C2ProfileLoad, C2ProfileSave.
"""

import asyncio
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from c4.bridge.rpc_server import BridgeServer


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture
def server(tmp_path):
    """Create a BridgeServer for dispatch testing."""
    return BridgeServer(project_root=tmp_path)


# ---------------------------------------------------------------------------
# Registration
# ---------------------------------------------------------------------------


class TestC2MethodRegistry:
    """Verify all C2 RPC methods are registered."""

    def test_has_c2_methods(self, server):
        expected = [
            "C2ParseDocument",
            "C2ExtractText",
            "C2WorkspaceCreate",
            "C2WorkspaceLoad",
            "C2WorkspaceSave",
            "C2PersonaLearn",
            "C2ProfileLoad",
            "C2ProfileSave",
        ]
        for method in expected:
            assert method in server.methods, f"Missing method: {method}"


# ---------------------------------------------------------------------------
# C2ParseDocument
# ---------------------------------------------------------------------------


class TestC2ParseDocument:

    @pytest.mark.asyncio
    async def test_requires_file_path(self, server):
        result = await server.dispatch("C2ParseDocument", {})
        assert "error" in result

    @pytest.mark.asyncio
    async def test_success(self, server):
        mock_block = MagicMock()
        mock_block.model_dump.return_value = {"type": "paragraph", "text": "hello"}
        mock_doc = MagicMock()
        mock_doc.blocks = [mock_block]
        mock_doc.metadata = None

        with patch("c4.c2.converter.parse_document", return_value=mock_doc):
            result = await server.dispatch("C2ParseDocument", {"file_path": "/tmp/test.docx"})
            assert result["block_count"] == 1
            assert result["blocks"][0]["type"] == "paragraph"

    @pytest.mark.asyncio
    async def test_handles_exception(self, server):
        with patch("c4.c2.converter.parse_document", side_effect=ValueError("Unsupported format")):
            result = await server.dispatch("C2ParseDocument", {"file_path": "/tmp/bad.xyz"})
            assert "error" in result
            assert "Unsupported format" in result["error"]


# ---------------------------------------------------------------------------
# C2ExtractText
# ---------------------------------------------------------------------------


class TestC2ExtractText:

    @pytest.mark.asyncio
    async def test_requires_file_path(self, server):
        result = await server.dispatch("C2ExtractText", {})
        assert "error" in result

    @pytest.mark.asyncio
    async def test_success(self, server):
        with patch("c4.c2.converter.extract_text", return_value="Hello World"):
            result = await server.dispatch("C2ExtractText", {"file_path": "/tmp/test.docx"})
            assert result["text"] == "Hello World"
            assert result["char_count"] == 11

    @pytest.mark.asyncio
    async def test_handles_exception(self, server):
        with patch("c4.c2.converter.extract_text", side_effect=FileNotFoundError("not found")):
            result = await server.dispatch("C2ExtractText", {"file_path": "/tmp/nope.docx"})
            assert "error" in result


# ---------------------------------------------------------------------------
# C2WorkspaceCreate
# ---------------------------------------------------------------------------


class TestC2WorkspaceCreate:

    @pytest.mark.asyncio
    async def test_requires_name(self, server):
        result = await server.dispatch("C2WorkspaceCreate", {})
        assert "error" in result

    @pytest.mark.asyncio
    async def test_success_default_type(self, server):
        result = await server.dispatch("C2WorkspaceCreate", {
            "name": "test-project",
            "goal": "Test review",
        })
        assert "state" in result
        state = result["state"]
        assert state["project_name"] == "test-project"
        assert state["project_type"] == "academic_paper"
        assert len(state["sections"]) > 0  # default sections

    @pytest.mark.asyncio
    async def test_proposal_type(self, server):
        result = await server.dispatch("C2WorkspaceCreate", {
            "name": "my-proposal",
            "project_type": "proposal",
            "goal": "Win grant",
        })
        state = result["state"]
        assert state["project_type"] == "proposal"

    @pytest.mark.asyncio
    async def test_invalid_type_defaults(self, server):
        result = await server.dispatch("C2WorkspaceCreate", {
            "name": "test",
            "project_type": "invalid_type",
        })
        state = result["state"]
        assert state["project_type"] == "academic_paper"


# ---------------------------------------------------------------------------
# C2WorkspaceLoad
# ---------------------------------------------------------------------------


class TestC2WorkspaceLoad:

    @pytest.mark.asyncio
    async def test_requires_project_dir(self, server):
        result = await server.dispatch("C2WorkspaceLoad", {})
        assert "error" in result

    @pytest.mark.asyncio
    async def test_missing_file(self, server, tmp_path):
        result = await server.dispatch("C2WorkspaceLoad", {
            "project_dir": str(tmp_path / "nonexistent"),
        })
        assert "error" in result
        assert "not found" in result["error"].lower()

    @pytest.mark.asyncio
    async def test_success(self, server, tmp_path):
        # Create a minimal workspace file
        ws_content = """# c2 Workspace - TestProject

## 프로젝트 정보
- **유형**: academic_paper
- **목표**: Test goal
- **생성일**: 2026-01-01
- **마지막 세션**: 2026-01-01 - 생성

## Discover (자료 탐색)
| # | 자료 | 유형 | 관련도 | 상태 | 비고 |
|---|------|------|--------|------|------|

## Read (읽기 노트)
| 자료 | 핵심 주장 | 방법/접근 | 우리와의 연결 | 노트 파일 |
|------|----------|----------|-------------|----------|

## Write (작성 상태)
| 섹션 | 상태 | 비고 |
|------|------|------|

## Review (리뷰 이력)
| 날짜 | 리뷰어 | 유형 | 주요 피드백 | 반영 상태 |
|------|--------|------|-----------|----------|

## Claim-Evidence 매핑
| 주장 | 근거 자료 | 결과/수치 | 위치 |
|------|----------|----------|------|

## 열린 질문
-

## 변경 이력
| 날짜 | 도메인 | 작업 | 결정 |
|------|--------|------|------|
"""
        project_dir = tmp_path / "test-project"
        project_dir.mkdir()
        (project_dir / "c2_workspace.md").write_text(ws_content, encoding="utf-8")

        result = await server.dispatch("C2WorkspaceLoad", {
            "project_dir": str(project_dir),
        })
        assert "state" in result
        assert result["state"]["project_name"] == "TestProject"


# ---------------------------------------------------------------------------
# C2WorkspaceSave
# ---------------------------------------------------------------------------


class TestC2WorkspaceSave:

    @pytest.mark.asyncio
    async def test_requires_project_dir(self, server):
        result = await server.dispatch("C2WorkspaceSave", {"state": {}})
        assert "error" in result

    @pytest.mark.asyncio
    async def test_requires_state(self, server, tmp_path):
        result = await server.dispatch("C2WorkspaceSave", {
            "project_dir": str(tmp_path),
        })
        assert "error" in result

    @pytest.mark.asyncio
    async def test_success(self, server, tmp_path):
        state_data = {
            "project_name": "test",
            "project_type": "academic_paper",
            "goal": "test goal",
        }
        project_dir = tmp_path / "save-test"
        result = await server.dispatch("C2WorkspaceSave", {
            "project_dir": str(project_dir),
            "state": state_data,
        })
        assert result["success"] is True
        assert (project_dir / "c2_workspace.md").exists()


# ---------------------------------------------------------------------------
# C2PersonaLearn
# ---------------------------------------------------------------------------


class TestC2PersonaLearn:

    @pytest.mark.asyncio
    async def test_requires_paths(self, server):
        result = await server.dispatch("C2PersonaLearn", {})
        assert "error" in result

        result = await server.dispatch("C2PersonaLearn", {"draft_path": "/a"})
        assert "error" in result

    @pytest.mark.asyncio
    async def test_success(self, server, tmp_path):
        draft = tmp_path / "draft.md"
        final = tmp_path / "final.md"
        draft.write_text("Original text with assertive claims. 오류가 있습니다.", encoding="utf-8")
        final.write_text("Modified text with questions. 확인이 필요합니다.", encoding="utf-8")

        result = await server.dispatch("C2PersonaLearn", {
            "draft_path": str(draft),
            "final_path": str(final),
        })
        assert "summary" in result
        assert "new_patterns" in result

    @pytest.mark.asyncio
    async def test_missing_files(self, server):
        result = await server.dispatch("C2PersonaLearn", {
            "draft_path": "/nonexistent/draft.md",
            "final_path": "/nonexistent/final.md",
        })
        assert "error" in result


# ---------------------------------------------------------------------------
# C2ProfileLoad
# ---------------------------------------------------------------------------


class TestC2ProfileLoad:

    @pytest.mark.asyncio
    async def test_missing_file_returns_empty(self, server):
        result = await server.dispatch("C2ProfileLoad", {
            "profile_path": "/nonexistent/profile.yaml",
        })
        assert result["profile"] == {}

    @pytest.mark.asyncio
    async def test_success(self, server, tmp_path):
        profile_path = tmp_path / "profile.yaml"
        profile_path.write_text(
            "reviewer_name: Test User\npreferences:\n  review:\n    tone: formal\n",
            encoding="utf-8",
        )
        result = await server.dispatch("C2ProfileLoad", {
            "profile_path": str(profile_path),
        })
        assert result["profile"]["reviewer_name"] == "Test User"


# ---------------------------------------------------------------------------
# C2ProfileSave
# ---------------------------------------------------------------------------


class TestC2ProfileSave:

    @pytest.mark.asyncio
    async def test_requires_data(self, server):
        result = await server.dispatch("C2ProfileSave", {})
        assert "error" in result

    @pytest.mark.asyncio
    async def test_rejects_non_dict_data(self, server):
        result = await server.dispatch("C2ProfileSave", {"data": "not a dict"})
        assert "error" in result

    @pytest.mark.asyncio
    async def test_success(self, server, tmp_path):
        profile_path = tmp_path / "profile.yaml"
        result = await server.dispatch("C2ProfileSave", {
            "profile_path": str(profile_path),
            "data": {"reviewer_name": "Saved User", "tone": "formal"},
        })
        assert result["success"] is True
        assert profile_path.exists()
        content = profile_path.read_text(encoding="utf-8")
        assert "Saved User" in content
