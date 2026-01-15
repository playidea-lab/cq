"""Tests for C4 Artifact Delivery - ZIP/PR generation and download links."""

import tempfile
from datetime import datetime, timedelta
from pathlib import Path
from unittest.mock import patch

import pytest

from c4.api.artifact import (
    Artifact,
    ArtifactService,
    ArtifactStatus,
    ArtifactType,
    DownloadToken,
    PRRequest,
    ZipRequest,
)

# =============================================================================
# DownloadToken Tests
# =============================================================================


class TestDownloadToken:
    """Test DownloadToken validation."""

    def test_valid_token(self):
        """Test valid token is recognized."""
        token = DownloadToken(
            token="abc123",
            artifact_id="art-1",
            created_at=datetime.now(),
            expires_at=datetime.now() + timedelta(hours=1),
            download_count=0,
            max_downloads=5,
        )
        assert token.is_valid() is True

    def test_expired_token(self):
        """Test expired token is invalid."""
        token = DownloadToken(
            token="abc123",
            artifact_id="art-1",
            created_at=datetime.now() - timedelta(hours=2),
            expires_at=datetime.now() - timedelta(hours=1),
            download_count=0,
            max_downloads=5,
        )
        assert token.is_valid() is False

    def test_max_downloads_exceeded(self):
        """Test token invalid when max downloads exceeded."""
        token = DownloadToken(
            token="abc123",
            artifact_id="art-1",
            created_at=datetime.now(),
            expires_at=datetime.now() + timedelta(hours=1),
            download_count=5,
            max_downloads=5,
        )
        assert token.is_valid() is False


# =============================================================================
# Artifact Tests
# =============================================================================


class TestArtifact:
    """Test Artifact dataclass."""

    def test_create_artifact(self):
        """Test creating artifact."""
        artifact = Artifact(
            id="art-123",
            type=ArtifactType.ZIP,
            status=ArtifactStatus.READY,
            name="test.zip",
        )
        assert artifact.id == "art-123"
        assert artifact.type == ArtifactType.ZIP
        assert artifact.status == ArtifactStatus.READY

    def test_to_dict(self):
        """Test converting artifact to dict."""
        artifact = Artifact(
            id="art-123",
            type=ArtifactType.ZIP,
            status=ArtifactStatus.READY,
            name="test.zip",
            task_id="T-001",
            size_bytes=1024,
        )
        data = artifact.to_dict()

        assert data["id"] == "art-123"
        assert data["type"] == "zip"
        assert data["status"] == "ready"
        assert data["name"] == "test.zip"
        assert data["task_id"] == "T-001"
        assert data["size_bytes"] == 1024

    def test_artifact_types(self):
        """Test artifact type values."""
        assert ArtifactType.ZIP.value == "zip"
        assert ArtifactType.PR.value == "pr"
        assert ArtifactType.FILE.value == "file"

    def test_artifact_status(self):
        """Test artifact status values."""
        assert ArtifactStatus.PENDING.value == "pending"
        assert ArtifactStatus.GENERATING.value == "generating"
        assert ArtifactStatus.READY.value == "ready"
        assert ArtifactStatus.EXPIRED.value == "expired"
        assert ArtifactStatus.FAILED.value == "failed"


# =============================================================================
# ArtifactService Tests
# =============================================================================


class TestArtifactService:
    """Test ArtifactService."""

    @pytest.fixture
    def temp_dir(self):
        """Create temporary directory."""
        with tempfile.TemporaryDirectory() as tmpdir:
            yield Path(tmpdir)

    @pytest.fixture
    def service(self, temp_dir):
        """Create artifact service."""
        return ArtifactService(storage_path=temp_dir)

    @pytest.fixture
    def test_files(self, temp_dir):
        """Create test files for ZIP creation."""
        files_dir = temp_dir / "files"
        files_dir.mkdir()

        (files_dir / "file1.txt").write_text("Hello World")
        (files_dir / "file2.txt").write_text("Test content")
        (files_dir / "subdir").mkdir()
        (files_dir / "subdir" / "nested.txt").write_text("Nested file")

        return files_dir

    def test_generate_id(self, service):
        """Test ID generation."""
        id1 = service._generate_id()
        id2 = service._generate_id()

        assert id1.startswith("art-")
        assert id2.startswith("art-")
        assert id1 != id2

    def test_generate_token(self, service):
        """Test token generation."""
        token1 = service._generate_token()
        token2 = service._generate_token()

        assert len(token1) > 20
        assert len(token2) > 20
        assert token1 != token2

    def test_compute_hash(self, service):
        """Test hash computation."""
        content = b"Hello World"
        hash1 = service._compute_hash(content)
        hash2 = service._compute_hash(content)

        assert len(hash1) == 16
        assert hash1 == hash2

        different_hash = service._compute_hash(b"Different content")
        assert different_hash != hash1

    @pytest.mark.asyncio
    async def test_create_zip_single_file(self, service, test_files):
        """Test creating ZIP with single file."""
        request = ZipRequest(
            files=["file1.txt"],
            base_path=str(test_files),
            name="test.zip",
            task_id="T-001",
        )

        artifact = await service.create_zip(request)

        assert artifact.type == ArtifactType.ZIP
        assert artifact.status == ArtifactStatus.READY
        assert artifact.name == "test.zip"
        assert artifact.task_id == "T-001"
        assert artifact.size_bytes > 0
        assert artifact.download_url is not None
        assert artifact.content_hash is not None

    @pytest.mark.asyncio
    async def test_create_zip_multiple_files(self, service, test_files):
        """Test creating ZIP with multiple files."""
        request = ZipRequest(
            files=["file1.txt", "file2.txt"],
            base_path=str(test_files),
        )

        artifact = await service.create_zip(request)

        assert artifact.status == ArtifactStatus.READY
        assert artifact.size_bytes > 0

    @pytest.mark.asyncio
    async def test_create_zip_with_directory(self, service, test_files):
        """Test creating ZIP with directory."""
        request = ZipRequest(
            files=["subdir"],
            base_path=str(test_files),
        )

        artifact = await service.create_zip(request)

        assert artifact.status == ArtifactStatus.READY

    @pytest.mark.asyncio
    async def test_create_zip_excludes_patterns(self, service, test_files):
        """Test ZIP excludes specified patterns."""
        # Create .pyc file
        (test_files / "test.pyc").write_bytes(b"bytecode")

        request = ZipRequest(
            files=["file1.txt", "test.pyc"],
            base_path=str(test_files),
            exclude_patterns=["*.pyc"],
        )

        artifact = await service.create_zip(request)

        # Get content and verify .pyc not included
        content = service._content.get(artifact.id)
        assert content is not None
        assert b"bytecode" not in content

    @pytest.mark.asyncio
    async def test_create_zip_missing_file(self, service, test_files):
        """Test ZIP handles missing files gracefully."""
        request = ZipRequest(
            files=["nonexistent.txt", "file1.txt"],
            base_path=str(test_files),
        )

        artifact = await service.create_zip(request)

        # Should still succeed with available files
        assert artifact.status == ArtifactStatus.READY

    @pytest.mark.asyncio
    async def test_download_by_token(self, service, test_files):
        """Test downloading artifact by token."""
        request = ZipRequest(
            files=["file1.txt"],
            base_path=str(test_files),
            name="download.zip",
        )

        artifact = await service.create_zip(request)

        # Extract token from download URL
        token = artifact.download_url.split("/")[-1]

        result = service.get_content_by_token(token)
        assert result is not None

        content, filename = result
        assert filename == "download.zip"
        assert len(content) > 0

    @pytest.mark.asyncio
    async def test_download_increments_count(self, service, test_files):
        """Test download increments count."""
        request = ZipRequest(
            files=["file1.txt"],
            base_path=str(test_files),
        )

        artifact = await service.create_zip(request)
        token = artifact.download_url.split("/")[-1]

        # Download multiple times
        for i in range(3):
            result = service.get_content_by_token(token)
            assert result is not None

        # Check download count
        download_token = service._tokens[token]
        assert download_token.download_count == 3

    @pytest.mark.asyncio
    async def test_download_invalid_token(self, service):
        """Test download with invalid token returns None."""
        result = service.get_content_by_token("invalid-token")
        assert result is None

    def test_get_artifact(self, service):
        """Test getting artifact by ID."""
        # Add artifact manually
        artifact = Artifact(
            id="art-test",
            type=ArtifactType.ZIP,
            status=ArtifactStatus.READY,
        )
        service._artifacts["art-test"] = artifact

        result = service.get_artifact("art-test")
        assert result == artifact

    def test_get_artifact_not_found(self, service):
        """Test getting nonexistent artifact."""
        result = service.get_artifact("nonexistent")
        assert result is None

    def test_list_artifacts(self, service):
        """Test listing artifacts."""
        # Add some artifacts
        service._artifacts["art-1"] = Artifact(
            id="art-1",
            type=ArtifactType.ZIP,
            status=ArtifactStatus.READY,
            task_id="T-001",
            project_id="P-001",
        )
        service._artifacts["art-2"] = Artifact(
            id="art-2",
            type=ArtifactType.PR,
            status=ArtifactStatus.READY,
            task_id="T-001",
            project_id="P-002",
        )
        service._artifacts["art-3"] = Artifact(
            id="art-3",
            type=ArtifactType.ZIP,
            status=ArtifactStatus.READY,
            task_id="T-002",
            project_id="P-001",
        )

        # List all
        all_artifacts = service.list_artifacts()
        assert len(all_artifacts) == 3

        # Filter by task
        task_artifacts = service.list_artifacts(task_id="T-001")
        assert len(task_artifacts) == 2

        # Filter by project
        project_artifacts = service.list_artifacts(project_id="P-001")
        assert len(project_artifacts) == 2

        # Filter by type
        zip_artifacts = service.list_artifacts(artifact_type=ArtifactType.ZIP)
        assert len(zip_artifacts) == 2

    def test_cleanup_expired(self, service):
        """Test cleanup removes expired artifacts."""
        # Add expired artifact
        service._artifacts["art-expired"] = Artifact(
            id="art-expired",
            type=ArtifactType.ZIP,
            status=ArtifactStatus.READY,
            expires_at=datetime.now() - timedelta(hours=1),
        )
        service._content["art-expired"] = b"old content"

        # Add valid artifact
        service._artifacts["art-valid"] = Artifact(
            id="art-valid",
            type=ArtifactType.ZIP,
            status=ArtifactStatus.READY,
            expires_at=datetime.now() + timedelta(hours=1),
        )
        service._content["art-valid"] = b"new content"

        removed = service.cleanup_expired()

        assert removed == 1
        assert "art-expired" not in service._artifacts
        assert "art-valid" in service._artifacts

    def test_should_exclude(self, service):
        """Test exclusion pattern matching."""
        patterns = [".git", "__pycache__", "*.pyc"]

        assert service._should_exclude(".git/config", patterns) is True
        assert service._should_exclude("src/__pycache__/mod.py", patterns) is True
        assert service._should_exclude("test.pyc", patterns) is True
        assert service._should_exclude("src/main.py", patterns) is False


# =============================================================================
# PR Creation Tests
# =============================================================================


class TestPRCreation:
    """Test PR artifact creation."""

    @pytest.fixture
    def service(self):
        """Create artifact service."""
        with tempfile.TemporaryDirectory() as tmpdir:
            yield ArtifactService(storage_path=Path(tmpdir))

    @pytest.mark.asyncio
    async def test_create_pr_success(self, service):
        """Test successful PR creation."""
        request = PRRequest(
            repo="owner/repo",
            branch="feature-branch",
            base="main",
            title="Test PR",
            body="Test body",
            task_id="T-001",
        )

        with patch.object(
            service,
            "_create_pr_with_gh",
            return_value="https://github.com/owner/repo/pull/123",
        ):
            artifact = await service.create_pr(request)

        assert artifact.type == ArtifactType.PR
        assert artifact.status == ArtifactStatus.READY
        assert artifact.pr_url == "https://github.com/owner/repo/pull/123"
        assert artifact.task_id == "T-001"
        assert artifact.metadata["repo"] == "owner/repo"

    @pytest.mark.asyncio
    async def test_create_pr_failure(self, service):
        """Test PR creation failure."""
        request = PRRequest(
            repo="owner/repo",
            branch="feature-branch",
            base="main",
            title="Test PR",
        )

        with patch.object(service, "_create_pr_with_gh", return_value=None):
            artifact = await service.create_pr(request)

        assert artifact.status == ArtifactStatus.FAILED

    @pytest.mark.asyncio
    async def test_create_pr_with_gh_success(self, service):
        """Test _create_pr_with_gh with successful subprocess."""
        request = PRRequest(
            repo="owner/repo",
            branch="feature",
            base="main",
            title="Title",
        )

        with patch("subprocess.run") as mock_run:
            mock_run.return_value.returncode = 0
            mock_run.return_value.stdout = "https://github.com/owner/repo/pull/1\n"

            result = await service._create_pr_with_gh(request)

        assert result == "https://github.com/owner/repo/pull/1"

    @pytest.mark.asyncio
    async def test_create_pr_with_gh_failure(self, service):
        """Test _create_pr_with_gh with failed subprocess."""
        request = PRRequest(
            repo="owner/repo",
            branch="feature",
            base="main",
            title="Title",
        )

        with patch("subprocess.run") as mock_run:
            mock_run.return_value.returncode = 1
            mock_run.return_value.stderr = "Error message"

            result = await service._create_pr_with_gh(request)

        assert result is None

    @pytest.mark.asyncio
    async def test_create_pr_gh_not_found(self, service):
        """Test _create_pr_with_gh when gh not installed."""
        request = PRRequest(
            repo="owner/repo",
            branch="feature",
            base="main",
            title="Title",
        )

        with patch("subprocess.run", side_effect=FileNotFoundError()):
            result = await service._create_pr_with_gh(request)

        assert result is None


# =============================================================================
# Request Model Tests
# =============================================================================


class TestZipRequest:
    """Test ZipRequest model."""

    def test_default_values(self):
        """Test default values."""
        request = ZipRequest(files=["file.txt"])

        assert request.base_path == "."
        assert request.name is None
        assert request.task_id is None
        assert ".git" in request.exclude_patterns
        assert "__pycache__" in request.exclude_patterns

    def test_custom_values(self):
        """Test custom values."""
        request = ZipRequest(
            files=["a.txt", "b.txt"],
            base_path="/custom/path",
            name="custom.zip",
            task_id="T-123",
            exclude_patterns=["*.log"],
        )

        assert request.files == ["a.txt", "b.txt"]
        assert request.base_path == "/custom/path"
        assert request.name == "custom.zip"
        assert request.task_id == "T-123"
        assert request.exclude_patterns == ["*.log"]


class TestPRRequest:
    """Test PRRequest model."""

    def test_required_fields(self):
        """Test required fields."""
        request = PRRequest(
            repo="owner/repo",
            branch="feature",
            title="My PR",
        )

        assert request.repo == "owner/repo"
        assert request.branch == "feature"
        assert request.title == "My PR"
        assert request.base == "main"  # default
        assert request.draft is False  # default

    def test_optional_fields(self):
        """Test optional fields."""
        request = PRRequest(
            repo="owner/repo",
            branch="feature",
            base="develop",
            title="Draft PR",
            body="Description",
            task_id="T-001",
            draft=True,
        )

        assert request.base == "develop"
        assert request.body == "Description"
        assert request.task_id == "T-001"
        assert request.draft is True
