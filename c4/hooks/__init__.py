"""C4 Git Hooks Package - Hook installation and daemon communication.

This package provides:
- Pre-commit hook with lint validation
- Unix Domain Socket client for daemon communication
- Fallback to direct validation when daemon is not running

Example usage:
    from c4.hooks import HookClient, install_pre_commit_hook

    # Check daemon status and run validation
    client = HookClient()
    if client.is_daemon_running():
        result = client.request_validation("lint")
    else:
        result = client.run_fallback_validation("lint")

    # Install pre-commit hook to git repository
    install_pre_commit_hook(force=False)
"""

from __future__ import annotations

import json
import os
import socket
import stat
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    pass

# =============================================================================
# Constants
# =============================================================================

# Default socket path relative to project root
DEFAULT_SOCKET_PATH = ".c4/c4.sock"

# Socket communication timeout (seconds)
DEFAULT_SOCKET_TIMEOUT = 5.0

# Buffer size for socket communication
SOCKET_BUFFER_SIZE = 8192

# Fallback validation commands
FALLBACK_COMMANDS = {
    "lint": "uv run ruff check .",
    "format": "uv run ruff format --check .",
    "unit": "uv run pytest tests/unit/ -x -q",
    "test": "uv run pytest tests/unit/ -x -q",
}


# =============================================================================
# Data Classes
# =============================================================================


@dataclass
class ValidationResult:
    """Result of a validation request."""

    status: str  # "pass" or "fail"
    message: str | None = None
    duration_ms: int | None = None
    source: str = "unknown"  # "daemon" or "fallback"

    @property
    def passed(self) -> bool:
        """Check if validation passed."""
        return self.status == "pass"

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "status": self.status,
            "message": self.message,
            "duration_ms": self.duration_ms,
            "source": self.source,
        }


# =============================================================================
# Hook Client
# =============================================================================


class HookClient:
    """Client for communicating with C4 Daemon via Unix Domain Socket.

    The client attempts to communicate with a running daemon for validation.
    If the daemon is not running, it falls back to direct command execution.

    Example:
        client = HookClient()

        # Check if daemon is available
        if client.is_daemon_running():
            result = client.request_validation("lint")
        else:
            result = client.run_fallback_validation("lint")

        if not result.passed:
            print(f"Validation failed: {result.message}")
            sys.exit(1)
    """

    def __init__(
        self,
        socket_path: str | Path | None = None,
        timeout: float = DEFAULT_SOCKET_TIMEOUT,
        project_root: Path | None = None,
    ):
        """Initialize the hook client.

        Args:
            socket_path: Path to the Unix Domain Socket. If None, uses default.
            timeout: Socket communication timeout in seconds.
            project_root: Project root directory. If None, uses current directory.
        """
        self.project_root = project_root or Path.cwd()
        self.timeout = timeout

        if socket_path is None:
            self.socket_path = self.project_root / DEFAULT_SOCKET_PATH
        else:
            self.socket_path = Path(socket_path)

    def is_daemon_running(self) -> bool:
        """Check if the C4 Daemon is running by testing socket availability.

        Returns:
            True if daemon socket exists and is responsive.
        """
        if not self.socket_path.exists():
            return False

        # Check if it's actually a socket
        if not self.socket_path.is_socket():
            return False

        # Try to connect briefly
        try:
            with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as sock:
                sock.settimeout(1.0)
                sock.connect(str(self.socket_path))
                return True
        except (ConnectionRefusedError, TimeoutError, OSError):
            return False

    def request_validation(self, validation_type: str = "lint") -> ValidationResult:
        """Request validation from the daemon via Unix Domain Socket.

        Args:
            validation_type: Type of validation (e.g., "lint", "format").

        Returns:
            ValidationResult with the outcome.

        Raises:
            ConnectionError: If cannot connect to daemon.
            TimeoutError: If daemon doesn't respond in time.
        """
        if not self.socket_path.exists():
            raise ConnectionError(f"Daemon socket not found: {self.socket_path}")

        request = {
            "action": "validate",
            "type": validation_type,
        }

        try:
            with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as sock:
                sock.settimeout(self.timeout)
                sock.connect(str(self.socket_path))

                # Send request with newline terminator
                request_data = json.dumps(request).encode("utf-8") + b"\n"
                sock.sendall(request_data)

                # Receive response
                response_data = b""
                while True:
                    chunk = sock.recv(SOCKET_BUFFER_SIZE)
                    if not chunk:
                        break
                    response_data += chunk
                    # Check if we received complete JSON (simple heuristic)
                    if response_data.endswith(b"}"):
                        break

                if not response_data:
                    raise ConnectionError("Empty response from daemon")

                response = json.loads(response_data.decode("utf-8"))

                return ValidationResult(
                    status=response.get("status", "fail"),
                    message=response.get("message"),
                    duration_ms=response.get("duration_ms"),
                    source="daemon",
                )

        except socket.timeout as e:
            raise TimeoutError(f"Daemon did not respond within {self.timeout}s") from e
        except json.JSONDecodeError as e:
            raise ConnectionError(f"Invalid JSON response from daemon: {e}") from e

    def run_fallback_validation(self, validation_type: str = "lint") -> ValidationResult:
        """Run validation directly without daemon.

        This is used when the daemon is not running.

        Args:
            validation_type: Type of validation (e.g., "lint").

        Returns:
            ValidationResult with the outcome.
        """
        import time

        command = FALLBACK_COMMANDS.get(validation_type)
        if command is None:
            # Default to lint if unknown type
            command = FALLBACK_COMMANDS["lint"]

        start_time = time.time()

        try:
            # Run the validation command
            result = subprocess.run(
                command,
                shell=True,
                cwd=self.project_root,
                capture_output=True,
                text=True,
                timeout=60,  # 60 second timeout for fallback
            )

            duration_ms = int((time.time() - start_time) * 1000)

            if result.returncode == 0:
                return ValidationResult(
                    status="pass",
                    message=None,
                    duration_ms=duration_ms,
                    source="fallback",
                )
            else:
                # Combine stderr and stdout for error message
                error_output = result.stderr or result.stdout
                return ValidationResult(
                    status="fail",
                    message=error_output[:500] if error_output else "Validation failed",
                    duration_ms=duration_ms,
                    source="fallback",
                )

        except subprocess.TimeoutExpired:
            return ValidationResult(
                status="fail",
                message="Validation timed out",
                duration_ms=60000,
                source="fallback",
            )
        except Exception as e:
            return ValidationResult(
                status="fail",
                message=str(e),
                source="fallback",
            )

    def validate(self, validation_type: str = "lint") -> ValidationResult:
        """Run validation, trying daemon first then falling back to direct execution.

        This is the main entry point for validation from hooks.

        Args:
            validation_type: Type of validation (e.g., "lint").

        Returns:
            ValidationResult with the outcome.
        """
        if self.is_daemon_running():
            try:
                return self.request_validation(validation_type)
            except (ConnectionError, TimeoutError):
                # Fall through to fallback
                pass

        return self.run_fallback_validation(validation_type)


# =============================================================================
# Hook Installation
# =============================================================================


def get_c4_install_dir() -> Path:
    """Get C4 installation directory.

    Returns:
        Path to the C4 installation directory.
    """
    # Try install path file first
    install_path_file = Path.home() / ".c4-install-path"
    if install_path_file.exists():
        return Path(install_path_file.read_text().strip())

    # Default locations
    default_paths = [
        Path.home() / ".c4",
        Path.home() / "git" / "c4",
    ]
    for p in default_paths:
        if (p / "c4").is_dir():
            return p

    # Fallback: use this file's location
    return Path(__file__).parent.parent.parent


def get_git_hooks_dir() -> Path | None:
    """Find the .git/hooks directory for the current repository.

    Returns:
        Path to .git/hooks directory, or None if not in a git repo.
    """
    try:
        # Walk up to find .git directory
        current = Path.cwd()
        while current != current.parent:
            git_dir = current / ".git"
            if git_dir.is_dir():
                return git_dir / "hooks"
            current = current.parent
        return None
    except Exception:
        return None


def get_pre_commit_template() -> str:
    """Get the pre-commit hook template content.

    First tries to read from templates/git-hooks/pre-commit,
    then falls back to embedded template.

    Returns:
        Pre-commit hook script content.
    """
    # Try to read from templates directory
    c4_install_dir = get_c4_install_dir()
    template_path = c4_install_dir / "templates" / "git-hooks" / "pre-commit"

    if template_path.exists():
        return template_path.read_text()

    # Fallback to embedded template
    return '''#!/bin/bash
# C4 Git Hook: pre-commit
# Runs lint validation before allowing commit

set -e

# Skip if not in a C4 project
if [[ ! -f ".c4/state.json" ]] && [[ ! -f ".c4/config.yaml" ]]; then
    exit 0
fi

echo "C4: Running pre-commit validation..." >&2

# Try uv first, then ruff directly
if command -v uv &> /dev/null; then
    if uv run ruff check . 2>/dev/null; then
        echo "C4: Pre-commit validation passed." >&2
        exit 0
    else
        echo "C4 ERROR: Lint failed. Please fix the issues and try again." >&2
        exit 1
    fi
elif command -v ruff &> /dev/null; then
    if ruff check . 2>/dev/null; then
        echo "C4: Pre-commit validation passed." >&2
        exit 0
    else
        echo "C4 ERROR: Lint failed. Please fix the issues and try again." >&2
        exit 1
    fi
else
    echo "C4: Warning - ruff not found, skipping lint validation." >&2
    exit 0
fi
'''


def install_pre_commit_hook(force: bool = False) -> tuple[bool, str]:
    """Install the pre-commit hook to the git repository.

    Args:
        force: If True, overwrite existing hooks.

    Returns:
        Tuple of (success, message).
    """
    hooks_dir = get_git_hooks_dir()
    if hooks_dir is None:
        return False, "Not in a git repository"

    hooks_dir.mkdir(parents=True, exist_ok=True)
    hook_path = hooks_dir / "pre-commit"

    # Check for existing hook
    if hook_path.exists() and not force:
        existing_content = hook_path.read_text()
        if "C4 Git Hook" in existing_content or "C4:" in existing_content:
            return True, "pre-commit: Already installed (C4)"
        else:
            return False, "pre-commit: Existing hook found (use --force to overwrite)"

    # Get template content
    content = get_pre_commit_template()

    # Write hook
    hook_path.write_text(content)

    # Make executable
    hook_path.chmod(hook_path.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)

    return True, "pre-commit: Installed"


def uninstall_pre_commit_hook() -> tuple[bool, str]:
    """Uninstall the pre-commit hook from the git repository.

    Only removes hooks installed by C4.

    Returns:
        Tuple of (success, message).
    """
    hooks_dir = get_git_hooks_dir()
    if hooks_dir is None:
        return False, "Not in a git repository"

    hook_path = hooks_dir / "pre-commit"

    if not hook_path.exists():
        return True, "pre-commit: Not installed"

    # Check if it's our hook
    content = hook_path.read_text()
    if "C4 Git Hook" not in content and "C4:" not in content:
        return False, "pre-commit: Not a C4 hook (skipped)"

    hook_path.unlink()
    return True, "pre-commit: Uninstalled"


def get_pre_commit_status() -> dict[str, Any]:
    """Get the status of the pre-commit hook.

    Returns:
        Dict with status information.
    """
    hooks_dir = get_git_hooks_dir()
    if hooks_dir is None:
        return {"installed": False, "is_c4": False, "error": "Not in git repo"}

    hook_path = hooks_dir / "pre-commit"

    if not hook_path.exists():
        return {"installed": False, "is_c4": False}

    content = hook_path.read_text()
    is_c4 = "C4 Git Hook" in content or "C4:" in content
    is_executable = os.access(hook_path, os.X_OK)

    return {
        "installed": True,
        "is_c4": is_c4,
        "executable": is_executable,
        "path": str(hook_path),
    }


# =============================================================================
# Post-Commit Hook Installation
# =============================================================================


def get_post_commit_template() -> str:
    """Get the post-commit hook template content.

    First tries to read from templates/git-hooks/post-commit,
    then falls back to embedded template.

    Returns:
        Post-commit hook script content.
    """
    # Try to read from templates directory
    c4_install_dir = get_c4_install_dir()
    template_path = c4_install_dir / "templates" / "git-hooks" / "post-commit"

    if template_path.exists():
        return template_path.read_text()

    # Fallback to embedded template
    return '''#!/bin/bash
# C4 Git Hook: post-commit
# Runs tests after commit completion

# Skip if not in a C4 project
if [[ ! -f ".c4/state.json" ]] && [[ ! -f ".c4/config.yaml" ]]; then
    exit 0
fi

echo "C4: Running post-commit tests..." >&2

# Background execution option
C4_POST_COMMIT_BACKGROUND="${C4_POST_COMMIT_BACKGROUND:-false}"

# Try uv first, then pytest directly
if command -v uv &> /dev/null; then
    if uv run pytest tests/unit/ -x -q 2>/dev/null; then
        echo "C4 SUCCESS: Tests passed." >&2
    else
        echo "C4 WARNING: Tests failed. Commit completed, but tests need attention." >&2
    fi
elif command -v pytest &> /dev/null; then
    if pytest tests/unit/ -x -q 2>/dev/null; then
        echo "C4 SUCCESS: Tests passed." >&2
    else
        echo "C4 WARNING: Tests failed. Commit completed, but tests need attention." >&2
    fi
else
    echo "C4: pytest not found, skipping post-commit tests." >&2
fi

# Always exit 0 - commit is already done
exit 0
'''


def install_post_commit_hook(force: bool = False) -> tuple[bool, str]:
    """Install the post-commit hook to the git repository.

    Args:
        force: If True, overwrite existing hooks.

    Returns:
        Tuple of (success, message).
    """
    hooks_dir = get_git_hooks_dir()
    if hooks_dir is None:
        return False, "Not in a git repository"

    hooks_dir.mkdir(parents=True, exist_ok=True)
    hook_path = hooks_dir / "post-commit"

    # Check for existing hook
    if hook_path.exists() and not force:
        existing_content = hook_path.read_text()
        if "C4 Git Hook" in existing_content or "C4:" in existing_content:
            return True, "post-commit: Already installed (C4)"
        else:
            return False, "post-commit: Existing hook found (use --force to overwrite)"

    # Get template content
    content = get_post_commit_template()

    # Write hook
    hook_path.write_text(content)

    # Make executable
    hook_path.chmod(hook_path.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)

    return True, "post-commit: Installed"


def uninstall_post_commit_hook() -> tuple[bool, str]:
    """Uninstall the post-commit hook from the git repository.

    Only removes hooks installed by C4.

    Returns:
        Tuple of (success, message).
    """
    hooks_dir = get_git_hooks_dir()
    if hooks_dir is None:
        return False, "Not in a git repository"

    hook_path = hooks_dir / "post-commit"

    if not hook_path.exists():
        return True, "post-commit: Not installed"

    # Check if it's our hook
    content = hook_path.read_text()
    if "C4 Git Hook" not in content and "C4:" not in content:
        return False, "post-commit: Not a C4 hook (skipped)"

    hook_path.unlink()
    return True, "post-commit: Uninstalled"


def get_post_commit_status() -> dict[str, Any]:
    """Get the status of the post-commit hook.

    Returns:
        Dict with status information.
    """
    hooks_dir = get_git_hooks_dir()
    if hooks_dir is None:
        return {"installed": False, "is_c4": False, "error": "Not in git repo"}

    hook_path = hooks_dir / "post-commit"

    if not hook_path.exists():
        return {"installed": False, "is_c4": False}

    content = hook_path.read_text()
    is_c4 = "C4 Git Hook" in content or "C4:" in content
    is_executable = os.access(hook_path, os.X_OK)

    return {
        "installed": True,
        "is_c4": is_c4,
        "executable": is_executable,
        "path": str(hook_path),
    }


# =============================================================================
# Module Exports
# =============================================================================

__all__ = [
    "HookClient",
    "ValidationResult",
    # Pre-commit hook
    "install_pre_commit_hook",
    "uninstall_pre_commit_hook",
    "get_pre_commit_status",
    "get_pre_commit_template",
    # Post-commit hook
    "install_post_commit_hook",
    "uninstall_post_commit_hook",
    "get_post_commit_status",
    "get_post_commit_template",
    # Utilities
    "get_git_hooks_dir",
    "get_c4_install_dir",
    "DEFAULT_SOCKET_PATH",
    "DEFAULT_SOCKET_TIMEOUT",
    "FALLBACK_COMMANDS",
]
