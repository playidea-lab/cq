"""Tests for backend factory credential resolution."""

import os
from unittest.mock import Mock, patch

import pytest

from c4.config.credentials import CredentialsManager
from c4.models.config import LLMConfig
from c4.supervisor.backend_factory import create_backend
from c4.supervisor.litellm_backend import LiteLLMBackend
from c4.supervisor.backend import SupervisorError


class TestBackendFactoryCredentials:
    """Test credential resolution in create_backend."""

    def test_create_backend_from_env(self, monkeypatch):
        """Test API key resolution from environment variable."""
        monkeypatch.setenv("TEST_API_KEY", "env-key")
        
        config = LLMConfig(
            model="gpt-4o",
            api_key_env="TEST_API_KEY"
        )
        
        # Should rely on environment variable directly
        # Note: generic env vars won't trigger CredentialsManager lookup 
        # because they aren't in ENV_VAR_MAPPING
        backend = create_backend(config)
        
        assert isinstance(backend, LiteLLMBackend)
        # LiteLLMBackend might store api_key privately, but we can check if it initialized without error
        
    def test_create_backend_from_credentials_manager(self, monkeypatch):
        """Test API key resolution from CredentialsManager when env var is missing."""
        # Ensure env var is NOT set
        monkeypatch.delenv("GOOGLE_API_KEY", raising=False)
        
        config = LLMConfig(
            model="gemini/gemini-1.5-pro",
            api_key_env="GOOGLE_API_KEY"
        )
        
        # Mock CredentialsManager
        # Mock CredentialsManager
        with patch("c4.config.credentials.CredentialsManager") as MockCreds:
            mock_instance = MockCreds.return_value
            mock_instance.get_api_key.return_value = "stored-gemini-key"
            
            backend = create_backend(config)
            
            assert isinstance(backend, LiteLLMBackend)
            # Verify CredentialsManager was queried for 'gemini'
            mock_instance.get_api_key.assert_called_with("gemini")

    def test_create_backend_missing_key(self, monkeypatch):
        """Test error when key is missing from both env and credentials."""
        monkeypatch.delenv("GOOGLE_API_KEY", raising=False)
        
        config = LLMConfig(
            model="gemini/gemini-1.5-pro",
            api_key_env="GOOGLE_API_KEY"
        )
        
        # Mock CredentialsManager
        with patch("c4.config.credentials.CredentialsManager") as MockCreds:
            mock_instance = MockCreds.return_value
            mock_instance.get_api_key.return_value = None
            
            with pytest.raises(SupervisorError, match="API key not found"):
                create_backend(config)

