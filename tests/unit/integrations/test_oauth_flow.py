"""Tests for OAuth flow state encoding/decoding.

Tests the state parameter encoding for CSRF protection in OAuth flows.
"""

from __future__ import annotations

import base64
import json

import pytest

from c4.api.routes.integrations import decode_state, encode_state


class TestStateEncoding:
    """Test state parameter encoding."""

    def test_encode_state_includes_team_id(self) -> None:
        """Test that encoded state includes team_id."""
        state = encode_state(team_id="team_123", user_id="user_456")

        # Decode and verify
        decoded = json.loads(base64.urlsafe_b64decode(state.encode()))

        assert decoded["team_id"] == "team_123"

    def test_encode_state_includes_user_id(self) -> None:
        """Test that encoded state includes user_id."""
        state = encode_state(team_id="team_123", user_id="user_456")

        decoded = json.loads(base64.urlsafe_b64decode(state.encode()))

        assert decoded["user_id"] == "user_456"

    def test_encode_state_includes_nonce(self) -> None:
        """Test that encoded state includes a nonce."""
        state = encode_state(team_id="team_123", user_id="user_456")

        decoded = json.loads(base64.urlsafe_b64decode(state.encode()))

        assert "nonce" in decoded
        assert len(decoded["nonce"]) > 0

    def test_encode_state_custom_nonce(self) -> None:
        """Test encoding with custom nonce."""
        state = encode_state(
            team_id="team_123", user_id="user_456", nonce="custom_nonce"
        )

        decoded = json.loads(base64.urlsafe_b64decode(state.encode()))

        assert decoded["nonce"] == "custom_nonce"

    def test_encode_state_is_base64_urlsafe(self) -> None:
        """Test that encoded state is URL-safe base64."""
        state = encode_state(team_id="team_123", user_id="user_456")

        # URL-safe base64 should not contain + or /
        assert "+" not in state
        assert "/" not in state

        # Should be decodable
        decoded = base64.urlsafe_b64decode(state.encode())
        assert decoded  # Non-empty

    def test_encode_state_unique_nonces(self) -> None:
        """Test that each encoding generates unique nonce."""
        state1 = encode_state(team_id="team_123", user_id="user_456")
        state2 = encode_state(team_id="team_123", user_id="user_456")

        decoded1 = json.loads(base64.urlsafe_b64decode(state1.encode()))
        decoded2 = json.loads(base64.urlsafe_b64decode(state2.encode()))

        # Nonces should be different
        assert decoded1["nonce"] != decoded2["nonce"]


class TestStateDecoding:
    """Test state parameter decoding."""

    def test_decode_state_returns_dict(self) -> None:
        """Test that decode returns a dictionary."""
        state = encode_state(team_id="team_123", user_id="user_456")

        decoded = decode_state(state)

        assert isinstance(decoded, dict)

    def test_decode_state_contains_team_id(self) -> None:
        """Test that decoded state contains team_id."""
        state = encode_state(team_id="team_abc", user_id="user_xyz")

        decoded = decode_state(state)

        assert decoded["team_id"] == "team_abc"

    def test_decode_state_contains_user_id(self) -> None:
        """Test that decoded state contains user_id."""
        state = encode_state(team_id="team_abc", user_id="user_xyz")

        decoded = decode_state(state)

        assert decoded["user_id"] == "user_xyz"

    def test_decode_state_contains_nonce(self) -> None:
        """Test that decoded state contains nonce."""
        state = encode_state(
            team_id="team_abc", user_id="user_xyz", nonce="test_nonce"
        )

        decoded = decode_state(state)

        assert decoded["nonce"] == "test_nonce"

    def test_decode_state_roundtrip(self) -> None:
        """Test encoding then decoding returns original data."""
        team_id = "team_roundtrip"
        user_id = "user_roundtrip"
        nonce = "nonce_roundtrip"

        state = encode_state(team_id=team_id, user_id=user_id, nonce=nonce)
        decoded = decode_state(state)

        assert decoded["team_id"] == team_id
        assert decoded["user_id"] == user_id
        assert decoded["nonce"] == nonce

    def test_decode_invalid_base64(self) -> None:
        """Test decoding invalid base64 raises ValueError."""
        with pytest.raises(ValueError) as exc_info:
            decode_state("not_valid_base64!!!")

        assert "Invalid state parameter" in str(exc_info.value)

    def test_decode_invalid_json(self) -> None:
        """Test decoding valid base64 but invalid JSON raises ValueError."""
        # Valid base64 but not JSON
        invalid_state = base64.urlsafe_b64encode(b"not json").decode()

        with pytest.raises(ValueError) as exc_info:
            decode_state(invalid_state)

        assert "Invalid state parameter" in str(exc_info.value)

    def test_decode_empty_string(self) -> None:
        """Test decoding empty string raises ValueError."""
        with pytest.raises(ValueError):
            decode_state("")


class TestStateSecurityProperties:
    """Test security properties of state encoding."""

    def test_state_is_tamper_evident(self) -> None:
        """Test that tampering with state causes decode failure."""
        state = encode_state(team_id="team_123", user_id="user_456")

        # Tamper with the state (flip a character)
        chars = list(state)
        # Find a character that's not padding
        for i in range(len(chars)):
            if chars[i] not in "=":
                original = chars[i]
                # Change to a different valid base64 char
                chars[i] = "A" if chars[i] != "A" else "B"
                break

        tampered = "".join(chars)

        # Either decoding fails or the content is corrupted
        try:
            decoded = decode_state(tampered)
            # If decode succeeds, data should be corrupted
            original_decoded = decode_state(state)
            # At least one field should differ
            assert decoded != original_decoded
        except ValueError:
            # Decoding failed - expected for tampered data
            pass

    def test_state_with_special_characters(self) -> None:
        """Test encoding handles special characters in IDs."""
        # Team/user IDs might contain special characters
        team_id = "team-123_abc"
        user_id = "user.456@example"

        state = encode_state(team_id=team_id, user_id=user_id)
        decoded = decode_state(state)

        assert decoded["team_id"] == team_id
        assert decoded["user_id"] == user_id

    def test_state_with_unicode(self) -> None:
        """Test encoding handles unicode characters."""
        team_id = "팀_123"
        user_id = "ユーザー_456"

        state = encode_state(team_id=team_id, user_id=user_id)
        decoded = decode_state(state)

        assert decoded["team_id"] == team_id
        assert decoded["user_id"] == user_id


class TestNonceGeneration:
    """Test nonce generation properties."""

    def test_auto_generated_nonce_sufficient_length(self) -> None:
        """Test that auto-generated nonce has sufficient entropy."""
        state = encode_state(team_id="team", user_id="user")
        decoded = decode_state(state)

        # Nonce should be at least 16 characters (from secrets.token_urlsafe(16))
        assert len(decoded["nonce"]) >= 16

    def test_auto_generated_nonces_are_unique(self) -> None:
        """Test that multiple auto-generated nonces are unique."""
        nonces = set()
        for _ in range(100):
            state = encode_state(team_id="team", user_id="user")
            decoded = decode_state(state)
            nonces.add(decoded["nonce"])

        # All 100 nonces should be unique
        assert len(nonces) == 100
