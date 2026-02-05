"""Shared test fixtures for C4 tests"""

import subprocess
import tempfile
import uuid
from pathlib import Path

import pytest

from c4.mcp_server import C4Daemon
from c4.models import CheckpointConfig, Task

# =============================================================================
# Worker ID Constants (valid UUID-based format)
# =============================================================================

# Standard worker IDs for tests (matches pattern: worker-[a-f0-9]{8})
WORKER_1 = "worker-a1b2c3d4"
WORKER_2 = "worker-b2c3d4e5"
WORKER_3 = "worker-c3d4e5f6"
WORKER_4 = "worker-d4e5f6a7"
SUPERVISOR_WORKER = "worker-00000000"


def make_worker_id(suffix: str = "") -> str:
    """Generate a unique valid worker ID.

    Args:
        suffix: Optional suffix to append for debugging (ignored in actual ID)

    Returns:
        Valid worker ID in format "worker-[a-f0-9]{8}"
    """
    return f"worker-{uuid.uuid4().hex[:8]}"

# =============================================================================
# Git Repository Fixtures
# =============================================================================


@pytest.fixture
def git_repo(tmp_path: Path) -> Path:
    """Create a temporary git repository with initial commit.

    This fixture creates a real git repository with:
    - Git initialized with 'main' as default branch
    - User config (email and name)
    - Initial commit with README.md
    - .c4 directory created

    Use this for tests that need a real git repository.

    Returns:
        Path to the git repository root
    """
    # Initialize git repo with main as default branch
    subprocess.run(
        ["git", "init", "-b", "main"],
        cwd=tmp_path,
        capture_output=True,
        check=True,
    )
    subprocess.run(
        ["git", "config", "user.email", "test@example.com"],
        cwd=tmp_path,
        capture_output=True,
        check=True,
    )
    subprocess.run(
        ["git", "config", "user.name", "Test User"],
        cwd=tmp_path,
        capture_output=True,
        check=True,
    )

    # Create initial commit
    (tmp_path / "README.md").write_text("# Test Project")
    subprocess.run(
        ["git", "add", "README.md"],
        cwd=tmp_path,
        capture_output=True,
        check=True,
    )
    subprocess.run(
        ["git", "commit", "-m", "Initial commit"],
        cwd=tmp_path,
        capture_output=True,
        check=True,
    )

    # Create .c4 directory
    (tmp_path / ".c4").mkdir()

    return tmp_path


@pytest.fixture
def mock_git_repo(tmp_path: Path) -> Path:
    """Create a mock git repository structure without real git.

    This fixture creates a minimal .git directory structure:
    - .git/hooks directory

    Use this for tests that only need git directory structure
    but don't need actual git functionality.

    Returns:
        Path to the mock git repository root
    """
    git_dir = tmp_path / ".git"
    git_dir.mkdir()
    hooks_dir = git_dir / "hooks"
    hooks_dir.mkdir()
    return tmp_path


@pytest.fixture
def git_repo_with_feature_branch(git_repo: Path) -> tuple[Path, str]:
    """Create a git repository with a feature branch.

    This fixture extends git_repo with:
    - A feature branch named 'feature/test'
    - A commit on the feature branch

    Returns:
        Tuple of (repo_path, branch_name)
    """
    branch_name = "feature/test"

    # Create and checkout feature branch
    subprocess.run(
        ["git", "checkout", "-b", branch_name],
        cwd=git_repo,
        capture_output=True,
        check=True,
    )

    # Add a commit on the feature branch
    (git_repo / "feature.txt").write_text("Feature content")
    subprocess.run(
        ["git", "add", "feature.txt"],
        cwd=git_repo,
        capture_output=True,
        check=True,
    )
    subprocess.run(
        ["git", "commit", "-m", "Add feature"],
        cwd=git_repo,
        capture_output=True,
        check=True,
    )

    # Return to main branch
    subprocess.run(
        ["git", "checkout", "main"],
        cwd=git_repo,
        capture_output=True,
        check=True,
    )

    return git_repo, branch_name


# =============================================================================
# Short Path Fixture (for systems with path length limits)
# =============================================================================


@pytest.fixture
def short_tmp_path(tmp_path: Path) -> Path:
    """Create a temporary directory with a shorter path.

    Some git operations (especially on macOS) can fail with very long paths.
    This fixture creates a directory directly under /tmp to avoid path length issues.

    Returns:
        Path to a temporary directory with shorter path
    """
    import tempfile

    short_dir = tempfile.mkdtemp(prefix="c4_")
    short_path = Path(short_dir)
    yield short_path
    # Cleanup
    import shutil

    shutil.rmtree(short_path, ignore_errors=True)


@pytest.fixture
def temp_project():
    """Create a temporary project directory"""
    with tempfile.TemporaryDirectory() as tmpdir:
        yield Path(tmpdir)


@pytest.fixture
def daemon(temp_project):
    """Create an initialized daemon (without default checkpoints for test isolation)"""
    d = C4Daemon(temp_project)
    d.initialize("test-project", with_default_checkpoints=False)
    return d


@pytest.fixture
def multi_worker_daemon(temp_project):
    """Create a daemon configured for multi-worker testing"""
    daemon = C4Daemon(temp_project)
    daemon.initialize("test-project", with_default_checkpoints=False)
    # Set short lock TTL for testing
    daemon._config.scope_lock_ttl_sec = 60
    return daemon


@pytest.fixture
def daemon_with_checkpoints(temp_project):
    """Create a daemon with checkpoint configuration"""
    daemon = C4Daemon(temp_project)
    daemon.initialize("test-project", with_default_checkpoints=False)

    # Add checkpoint configuration
    daemon._config.checkpoints = [
        CheckpointConfig(
            id="CP0",
            required_tasks=["T-001", "T-002"],
            required_validations=["lint", "unit"],
        )
    ]

    # Add validation commands
    daemon._config.validation.commands = {
        "lint": "echo 'lint passed'",
        "unit": "echo 'unit passed'",
    }
    daemon._config.validation.required = ["lint", "unit"]

    return daemon


@pytest.fixture
def daemon_in_execute(daemon):
    """Create a daemon in EXECUTE state with a task"""
    task = Task(id="T-001", title="Test Task", dod="Test definition of done")
    daemon.add_task(task)
    daemon.state_machine.transition("c4_run")
    return daemon
