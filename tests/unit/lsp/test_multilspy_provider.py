"""Tests for multilspy-based symbol provider."""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest

# Check if multilspy is available
try:
    from c4.lsp.multilspy_provider import (
        LANGUAGE_SERVER_INSTALL_GUIDE,
        MULTILSPY_AVAILABLE,
        MultilspyProvider,
    )
except ImportError:
    MULTILSPY_AVAILABLE = False
    MultilspyProvider = None  # type: ignore
    LANGUAGE_SERVER_INSTALL_GUIDE = {}


class TestLanguageServerInstallGuide:
    """Tests for language server installation guides."""

    def test_guide_exists_for_common_languages(self):
        """Test that guides exist for common languages."""
        expected_languages = ["python", "typescript", "go", "rust"]
        for lang in expected_languages:
            assert lang in LANGUAGE_SERVER_INSTALL_GUIDE, f"Missing guide for {lang}"

    def test_guide_has_required_fields(self):
        """Test that each guide has required fields."""
        for lang, guide in LANGUAGE_SERVER_INSTALL_GUIDE.items():
            assert "package" in guide, f"Missing 'package' for {lang}"
            assert "commands" in guide, f"Missing 'commands' for {lang}"
            assert isinstance(guide["commands"], list), f"'commands' should be list for {lang}"

    def test_python_guide_mentions_jedi(self):
        """Test that Python guide mentions Jedi fallback."""
        python_guide = LANGUAGE_SERVER_INSTALL_GUIDE.get("python", {})
        note = python_guide.get("note", "")
        assert "jedi" in note.lower() or "Jedi" in note

    def test_typescript_requires_nodejs(self):
        """Test that TypeScript guide mentions Node.js requirement."""
        ts_guide = LANGUAGE_SERVER_INSTALL_GUIDE.get("typescript", {})
        requires = ts_guide.get("requires", "")
        assert "node" in requires.lower()


@pytest.mark.skipif(not MULTILSPY_AVAILABLE, reason="multilspy not installed")
class TestMultilspyProvider:
    """Tests for MultilspyProvider class."""

    @pytest.fixture
    def provider(self, tmp_path):
        """Create a provider with a temp directory."""
        return MultilspyProvider(project_path=str(tmp_path))

    def test_language_detection(self, provider):
        """Test language detection from file extensions."""
        assert provider._detect_language("test.py") == "python"
        assert provider._detect_language("test.ts") == "typescript"
        assert provider._detect_language("test.tsx") == "typescript"
        assert provider._detect_language("test.go") == "go"
        assert provider._detect_language("test.rs") == "rust"
        assert provider._detect_language("test.unknown") is None

    def test_get_install_guide(self, provider):
        """Test getting installation guide for a language."""
        guide = provider.get_install_guide("python")
        assert "package" in guide
        assert "commands" in guide

        # Unknown language returns empty dict
        unknown_guide = provider.get_install_guide("unknown_lang")
        assert unknown_guide == {}

    def test_get_stats_empty(self, provider):
        """Test getting stats with no servers."""
        stats = provider.get_stats()
        assert stats["active_servers"] == 0
        assert stats["servers"] == {}


@pytest.mark.skipif(not MULTILSPY_AVAILABLE, reason="multilspy not installed")
class TestMultilspyDiagnosis:
    """Tests for language server diagnosis functionality."""

    @pytest.fixture
    def provider(self, tmp_path):
        """Create a provider with a temp directory."""
        return MultilspyProvider(project_path=str(tmp_path))

    def test_diagnose_returns_dict(self, provider):
        """Test that diagnose returns a dictionary."""
        # This may take time as it tries to start servers
        # We just verify the structure
        with patch.object(provider, "_get_server") as mock_get:
            mock_get.side_effect = RuntimeError("Server not available")

            results = provider.diagnose_language_servers()

            assert isinstance(results, dict)
            # Should have at least Python entry
            assert "python" in results

    def test_diagnose_result_structure(self, provider):
        """Test the structure of diagnosis results."""
        with patch.object(provider, "_get_server") as mock_get:
            mock_get.side_effect = RuntimeError("Test error")

            results = provider.diagnose_language_servers()

            for lang, diagnosis in results.items():
                assert "available" in diagnosis
                assert "error" in diagnosis
                assert "install_guide" in diagnosis
                assert isinstance(diagnosis["available"], bool)

    def test_diagnose_unavailable_server(self, provider):
        """Test diagnosis when server is not available."""
        with patch.object(provider, "_get_server") as mock_get:
            mock_get.side_effect = RuntimeError("LSP server not available for test")

            results = provider.diagnose_language_servers()

            # All should be unavailable due to mocked error
            for lang, diagnosis in results.items():
                assert diagnosis["available"] is False
                assert diagnosis["error"] is not None

    def test_diagnose_available_server(self, provider):
        """Test diagnosis when server is available."""
        mock_server = MagicMock()

        with patch.object(provider, "_get_server") as mock_get:
            mock_get.return_value = mock_server

            results = provider.diagnose_language_servers()

            # At least one should be marked available
            available_count = sum(1 for d in results.values() if d["available"])
            assert available_count > 0


class TestServerErrorMessages:
    """Tests for friendly error messages when servers fail."""

    @pytest.mark.skipif(not MULTILSPY_AVAILABLE, reason="multilspy not installed")
    def test_error_includes_install_hint(self, tmp_path):
        """Test that server errors include installation hints."""
        provider = MultilspyProvider(project_path=str(tmp_path))

        # Mock the internal server creation to fail
        with patch("c4.lsp.multilspy_provider.SyncLanguageServer") as mock_cls:
            mock_cls.create.side_effect = Exception("Server binary not found")

            with pytest.raises(RuntimeError) as exc_info:
                provider._get_server("typescript")

            error_msg = str(exc_info.value)
            # Should mention install command
            assert "npm" in error_msg or "Install" in error_msg

    @pytest.mark.skipif(not MULTILSPY_AVAILABLE, reason="multilspy not installed")
    def test_error_mentions_requirements(self, tmp_path):
        """Test that errors mention requirements for languages that need them."""
        provider = MultilspyProvider(project_path=str(tmp_path))

        with patch("c4.lsp.multilspy_provider.SyncLanguageServer") as mock_cls:
            mock_cls.create.side_effect = Exception("gopls not found")

            with pytest.raises(RuntimeError) as exc_info:
                provider._get_server("go")

            error_msg = str(exc_info.value)
            # Should mention Go requirement
            assert "Go" in error_msg or "golang" in error_msg.lower() or "Install" in error_msg
