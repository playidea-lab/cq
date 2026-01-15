"""Tests for C4 Git Operations module."""

from pathlib import Path

import pytest

from c4.daemon.git_ops import GitOperations, GitResult


class TestGitResult:
    """Test GitResult namedtuple."""

    def test_success_result(self) -> None:
        """Test successful result."""
        result = GitResult(success=True, message="Success", sha="abc123")
        assert result.success is True
        assert result.sha == "abc123"

    def test_failure_result(self) -> None:
        """Test failure result."""
        result = GitResult(success=False, message="Failed")
        assert result.success is False
        assert result.sha is None


class TestGitOperations:
    """Test GitOperations class."""

    @pytest.fixture
    def temp_git_repo(self, tmp_path: Path) -> Path:
        """Create a temporary git repository."""
        import subprocess

        subprocess.run(
            ["git", "init"],
            cwd=tmp_path,
            capture_output=True,
            check=True,
        )
        subprocess.run(
            ["git", "config", "user.email", "test@example.com"],
            cwd=tmp_path,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Test User"],
            cwd=tmp_path,
            capture_output=True,
        )
        # Create initial commit
        (tmp_path / "README.md").write_text("# Test")
        subprocess.run(["git", "add", "."], cwd=tmp_path, capture_output=True)
        subprocess.run(
            ["git", "commit", "-m", "Initial commit"],
            cwd=tmp_path,
            capture_output=True,
        )
        return tmp_path

    @pytest.fixture
    def git_ops(self, temp_git_repo: Path) -> GitOperations:
        """Create GitOperations for temp repo."""
        return GitOperations(temp_git_repo)

    def test_is_git_repo(self, git_ops: GitOperations, temp_git_repo: Path) -> None:
        """Test git repo detection."""
        assert git_ops.is_git_repo() is True

    def test_not_git_repo(self, tmp_path: Path) -> None:
        """Test non-git directory detection."""
        ops = GitOperations(tmp_path)
        assert ops.is_git_repo() is False

    def test_is_git_available(self, git_ops: GitOperations) -> None:
        """Test git availability check."""
        assert git_ops.is_git_available() is True

    def test_get_current_sha(self, git_ops: GitOperations) -> None:
        """Test getting current SHA."""
        sha = git_ops.get_current_sha()
        assert sha is not None
        assert len(sha) >= 7

    def test_has_uncommitted_changes_clean(self, git_ops: GitOperations) -> None:
        """Test clean working directory."""
        assert git_ops.has_uncommitted_changes() is False

    def test_has_uncommitted_changes_dirty(
        self, git_ops: GitOperations, temp_git_repo: Path
    ) -> None:
        """Test dirty working directory."""
        (temp_git_repo / "new_file.txt").write_text("New content")
        assert git_ops.has_uncommitted_changes() is True

    def test_stage_all(
        self, git_ops: GitOperations, temp_git_repo: Path
    ) -> None:
        """Test staging changes."""
        (temp_git_repo / "staged.txt").write_text("Content")
        assert git_ops.stage_all() is True

    def test_commit_task_completion(
        self, git_ops: GitOperations, temp_git_repo: Path
    ) -> None:
        """Test task completion commit."""
        (temp_git_repo / "task_file.txt").write_text("Task work")

        result = git_ops.commit_task_completion(
            task_id="T-001",
            task_title="Test Task",
            worker_id="worker-1",
        )

        assert result.success is True
        assert result.sha is not None
        assert "T-001" in result.message

    def test_commit_task_no_changes(self, git_ops: GitOperations) -> None:
        """Test commit with no changes."""
        result = git_ops.commit_task_completion(
            task_id="T-002",
            task_title="Empty Task",
        )

        assert result.success is True
        assert "No changes" in result.message

    def test_commit_repair_completion(
        self, git_ops: GitOperations, temp_git_repo: Path
    ) -> None:
        """Test repair completion commit."""
        (temp_git_repo / "repair_file.txt").write_text("Repair work")

        result = git_ops.commit_repair_completion(
            task_id="REPAIR-T-001-1",
            original_task_id="T-001",
            repair_reason="Fix validation error",
        )

        assert result.success is True
        assert "REPAIR" in result.message

    def test_create_checkpoint_tag(self, git_ops: GitOperations) -> None:
        """Test checkpoint tag creation."""
        result = git_ops.create_checkpoint_tag(
            checkpoint_id="CP-001",
            checkpoint_name="First Checkpoint",
        )

        assert result.success is True
        assert result.tag == "c4/CP-001"

    def test_create_checkpoint_tag_duplicate(self, git_ops: GitOperations) -> None:
        """Test creating duplicate tag."""
        git_ops.create_checkpoint_tag("CP-002")
        result = git_ops.create_checkpoint_tag("CP-002")

        assert result.success is True
        assert "already exists" in result.message

    def test_get_checkpoint_tags(self, git_ops: GitOperations) -> None:
        """Test getting checkpoint tags."""
        git_ops.create_checkpoint_tag("CP-001")
        git_ops.create_checkpoint_tag("CP-002")

        tags = git_ops.get_checkpoint_tags()

        assert len(tags) == 2
        assert "c4/CP-001" in tags
        assert "c4/CP-002" in tags

    def test_get_branch_name(self, git_ops: GitOperations) -> None:
        """Test getting branch name."""
        branch = git_ops.get_branch_name()
        # Could be main, master, or something else
        assert branch is not None

    def test_create_task_branch_new(
        self, git_ops: GitOperations, temp_git_repo: Path
    ) -> None:
        """Test creating new task branch."""
        result = git_ops.create_task_branch("T-100")

        assert result.success is True
        assert "c4/w-T-100" in result.message

    def test_create_task_branch_existing(self, git_ops: GitOperations) -> None:
        """Test switching to existing branch."""
        git_ops.create_task_branch("T-101")
        # Switch to main first
        git_ops._run_git("checkout", "-")
        # Now create same branch again
        result = git_ops.create_task_branch("T-101")

        assert result.success is True
        assert "existing" in result.message.lower()

    def test_get_tag_info(self, git_ops: GitOperations) -> None:
        """Test getting tag info."""
        git_ops.create_checkpoint_tag("CP-003", "Test checkpoint")

        info = git_ops.get_tag_info("c4/CP-003")

        assert info is not None
        assert "sha" in info
        assert "date" in info

    def test_get_tag_info_nonexistent(self, git_ops: GitOperations) -> None:
        """Test getting info for nonexistent tag."""
        info = git_ops.get_tag_info("nonexistent")
        assert info is None

    def test_list_checkpoint_tags(self, git_ops: GitOperations) -> None:
        """Test listing checkpoint tags with details."""
        git_ops.create_checkpoint_tag("CP-004", "Checkpoint 4")

        tags = git_ops.list_checkpoint_tags()

        assert len(tags) >= 1
        assert "tag" in tags[0]
        assert "sha" in tags[0]

    def test_rollback_to_checkpoint(
        self, git_ops: GitOperations, temp_git_repo: Path
    ) -> None:
        """Test rollback to checkpoint."""
        # Create checkpoint
        git_ops.create_checkpoint_tag("CP-005")
        initial_sha = git_ops.get_current_sha()

        # Make new commit
        (temp_git_repo / "new_work.txt").write_text("New work")
        git_ops.commit_task_completion("T-200", "New work")

        # Rollback
        result = git_ops.rollback_to_checkpoint("c4/CP-005")

        assert result.success is True
        assert git_ops.get_current_sha() == initial_sha

    def test_rollback_nonexistent_tag(self, git_ops: GitOperations) -> None:
        """Test rollback to nonexistent tag."""
        result = git_ops.rollback_to_checkpoint("c4/CP-NONEXISTENT")

        assert result.success is False
        assert "not found" in result.message

    def test_get_commits_since_tag(
        self, git_ops: GitOperations, temp_git_repo: Path
    ) -> None:
        """Test getting commits since a tag."""
        git_ops.create_checkpoint_tag("CP-006")

        # Make some commits
        (temp_git_repo / "file1.txt").write_text("1")
        git_ops.commit_task_completion("T-301", "Task 301")
        (temp_git_repo / "file2.txt").write_text("2")
        git_ops.commit_task_completion("T-302", "Task 302")

        commits = git_ops.get_commits_since_tag("c4/CP-006")

        assert len(commits) == 2
        assert "sha" in commits[0]
        assert "message" in commits[0]


class TestGitOperationsNonGitDir:
    """Test GitOperations behavior in non-git directory."""

    @pytest.fixture
    def non_git_ops(self, tmp_path: Path) -> GitOperations:
        """Create GitOperations for non-git directory."""
        return GitOperations(tmp_path)

    def test_is_git_repo_false(self, non_git_ops: GitOperations) -> None:
        """Test detection of non-git directory."""
        assert non_git_ops.is_git_repo() is False

    def test_get_current_sha_none(self, non_git_ops: GitOperations) -> None:
        """Test SHA retrieval in non-git directory."""
        assert non_git_ops.get_current_sha() is None

    def test_commit_fails(self, non_git_ops: GitOperations) -> None:
        """Test commit failure in non-git directory."""
        result = non_git_ops.commit_task_completion("T-001", "Test")
        assert result.success is False
        assert "Not a Git" in result.message

    def test_create_tag_fails(self, non_git_ops: GitOperations) -> None:
        """Test tag creation failure in non-git directory."""
        result = non_git_ops.create_checkpoint_tag("CP-001")
        assert result.success is False
