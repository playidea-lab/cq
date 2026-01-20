"""C4 Artifact Delivery - Generate and deliver task artifacts."""

from __future__ import annotations

import hashlib
import io
import logging
import secrets
import subprocess
import tempfile
import zipfile
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from enum import Enum
from pathlib import Path
from typing import Any

from fastapi import APIRouter, Depends, Header, HTTPException
from fastapi.responses import StreamingResponse
from pydantic import BaseModel, Field

logger = logging.getLogger(__name__)

router = APIRouter()


# =============================================================================
# Models
# =============================================================================


class ArtifactType(str, Enum):
    """Types of deliverable artifacts."""

    ZIP = "zip"
    PR = "pr"
    FILE = "file"


class ArtifactStatus(str, Enum):
    """Status of artifact generation."""

    PENDING = "pending"
    GENERATING = "generating"
    READY = "ready"
    EXPIRED = "expired"
    FAILED = "failed"


@dataclass
class DownloadToken:
    """Secure token for artifact download."""

    token: str
    artifact_id: str
    created_at: datetime
    expires_at: datetime
    download_count: int = 0
    max_downloads: int = 5

    def is_valid(self) -> bool:
        """Check if token is still valid."""
        return datetime.now() < self.expires_at and self.download_count < self.max_downloads


@dataclass
class Artifact:
    """Artifact metadata and content."""

    id: str
    type: ArtifactType
    status: ArtifactStatus
    task_id: str | None = None
    project_id: str | None = None
    name: str = ""
    description: str = ""
    created_at: datetime = field(default_factory=datetime.now)
    expires_at: datetime | None = None
    size_bytes: int = 0
    content_hash: str = ""
    download_url: str | None = None
    pr_url: str | None = None
    metadata: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "id": self.id,
            "type": self.type.value,
            "status": self.status.value,
            "task_id": self.task_id,
            "project_id": self.project_id,
            "name": self.name,
            "description": self.description,
            "created_at": self.created_at.isoformat(),
            "expires_at": self.expires_at.isoformat() if self.expires_at else None,
            "size_bytes": self.size_bytes,
            "content_hash": self.content_hash,
            "download_url": self.download_url,
            "pr_url": self.pr_url,
            "metadata": self.metadata,
        }


class ZipRequest(BaseModel):
    """Request to create ZIP artifact."""

    files: list[str] = Field(..., description="List of file paths to include")
    base_path: str = Field(default=".", description="Base path for relative paths")
    name: str | None = Field(default=None, description="ZIP file name")
    task_id: str | None = Field(default=None, description="Associated task ID")
    project_id: str | None = Field(default=None, description="Project ID")
    include_patterns: list[str] = Field(
        default_factory=list, description="Glob patterns to include"
    )
    exclude_patterns: list[str] = Field(
        default_factory=lambda: [".git", "__pycache__", "*.pyc", ".env"],
        description="Glob patterns to exclude",
    )


class PRRequest(BaseModel):
    """Request to create Pull Request."""

    repo: str = Field(..., description="Repository in owner/repo format")
    branch: str = Field(..., description="Source branch name")
    base: str = Field(default="main", description="Target branch name")
    title: str = Field(..., description="PR title")
    body: str = Field(default="", description="PR description")
    task_id: str | None = Field(default=None, description="Associated task ID")
    project_id: str | None = Field(default=None, description="Project ID")
    draft: bool = Field(default=False, description="Create as draft PR")


class ArtifactResponse(BaseModel):
    """Response with artifact info."""

    id: str
    type: str
    status: str
    name: str
    download_url: str | None = None
    pr_url: str | None = None
    expires_at: str | None = None
    size_bytes: int = 0


# =============================================================================
# Service
# =============================================================================


class ArtifactService:
    """Service for creating and managing artifacts."""

    def __init__(
        self,
        storage_path: Path | None = None,
        download_base_url: str = "/api/artifacts/download",
        token_expiry_hours: int = 24,
    ):
        """Initialize artifact service.

        Args:
            storage_path: Path for artifact storage (default: temp directory)
            download_base_url: Base URL for download links
            token_expiry_hours: Hours until download token expires
        """
        self.storage_path = storage_path or Path(tempfile.gettempdir()) / "c4_artifacts"
        self.storage_path.mkdir(parents=True, exist_ok=True)
        self.download_base_url = download_base_url
        self.token_expiry_hours = token_expiry_hours

        self._artifacts: dict[str, Artifact] = {}
        self._tokens: dict[str, DownloadToken] = {}
        self._content: dict[str, bytes] = {}

    def _generate_id(self) -> str:
        """Generate unique artifact ID."""
        return f"art-{secrets.token_hex(8)}"

    def _generate_token(self) -> str:
        """Generate secure download token."""
        return secrets.token_urlsafe(32)

    def _compute_hash(self, content: bytes) -> str:
        """Compute SHA256 hash of content."""
        return hashlib.sha256(content).hexdigest()[:16]

    async def create_zip(self, request: ZipRequest) -> Artifact:
        """Create ZIP artifact from files.

        Args:
            request: ZIP creation request

        Returns:
            Created artifact
        """
        artifact_id = self._generate_id()
        artifact = Artifact(
            id=artifact_id,
            type=ArtifactType.ZIP,
            status=ArtifactStatus.GENERATING,
            task_id=request.task_id,
            project_id=request.project_id,
            name=request.name or f"artifact-{artifact_id}.zip",
        )
        self._artifacts[artifact_id] = artifact

        try:
            # Create ZIP in memory
            zip_buffer = io.BytesIO()
            base_path = Path(request.base_path).resolve()

            with zipfile.ZipFile(zip_buffer, "w", zipfile.ZIP_DEFLATED) as zf:
                for file_path in request.files:
                    full_path = base_path / file_path
                    if not full_path.exists():
                        logger.warning(f"File not found: {full_path}")
                        continue

                    # Check exclusions
                    if self._should_exclude(file_path, request.exclude_patterns):
                        continue

                    if full_path.is_file():
                        arcname = str(Path(file_path))
                        zf.write(full_path, arcname)
                    elif full_path.is_dir():
                        self._add_directory_to_zip(
                            zf, full_path, base_path, request.exclude_patterns
                        )

            # Get ZIP content
            zip_content = zip_buffer.getvalue()
            content_hash = self._compute_hash(zip_content)

            # Store content
            self._content[artifact_id] = zip_content

            # Generate download token
            token = self._generate_token()
            expires_at = datetime.now() + timedelta(hours=self.token_expiry_hours)

            self._tokens[token] = DownloadToken(
                token=token,
                artifact_id=artifact_id,
                created_at=datetime.now(),
                expires_at=expires_at,
            )

            # Update artifact
            artifact.status = ArtifactStatus.READY
            artifact.size_bytes = len(zip_content)
            artifact.content_hash = content_hash
            artifact.expires_at = expires_at
            artifact.download_url = f"{self.download_base_url}/{token}"

            logger.info(f"Created ZIP artifact: {artifact_id}, {len(zip_content)} bytes")
            return artifact

        except Exception as e:
            artifact.status = ArtifactStatus.FAILED
            artifact.metadata["error"] = str(e)
            logger.error(f"Failed to create ZIP artifact: {e}")
            raise HTTPException(status_code=500, detail=f"Failed to create ZIP: {e}")

    def _add_directory_to_zip(
        self,
        zf: zipfile.ZipFile,
        dir_path: Path,
        base_path: Path,
        exclude_patterns: list[str],
    ) -> None:
        """Recursively add directory to ZIP."""
        for item in dir_path.rglob("*"):
            if item.is_file():
                rel_path = str(item.relative_to(base_path))
                if not self._should_exclude(rel_path, exclude_patterns):
                    zf.write(item, rel_path)

    def _should_exclude(self, path: str, patterns: list[str]) -> bool:
        """Check if path matches exclusion patterns."""
        from fnmatch import fnmatch

        for pattern in patterns:
            if fnmatch(path, pattern) or fnmatch(Path(path).name, pattern):
                return True
            if pattern in path:
                return True
        return False

    async def create_pr(self, request: PRRequest) -> Artifact:
        """Create Pull Request artifact.

        Args:
            request: PR creation request

        Returns:
            Created artifact with PR URL
        """
        artifact_id = self._generate_id()
        artifact = Artifact(
            id=artifact_id,
            type=ArtifactType.PR,
            status=ArtifactStatus.GENERATING,
            task_id=request.task_id,
            project_id=request.project_id,
            name=request.title,
            description=request.body,
        )
        self._artifacts[artifact_id] = artifact

        try:
            # Try gh CLI first
            pr_url = await self._create_pr_with_gh(request)

            if pr_url:
                artifact.status = ArtifactStatus.READY
                artifact.pr_url = pr_url
                artifact.metadata["repo"] = request.repo
                artifact.metadata["branch"] = request.branch
                artifact.metadata["base"] = request.base
                logger.info(f"Created PR artifact: {artifact_id}, URL: {pr_url}")
            else:
                artifact.status = ArtifactStatus.FAILED
                artifact.metadata["error"] = "Failed to create PR"

            return artifact

        except Exception as e:
            artifact.status = ArtifactStatus.FAILED
            artifact.metadata["error"] = str(e)
            logger.error(f"Failed to create PR: {e}")
            raise HTTPException(status_code=500, detail=f"Failed to create PR: {e}")

    async def _create_pr_with_gh(self, request: PRRequest) -> str | None:
        """Create PR using gh CLI.

        Args:
            request: PR creation request

        Returns:
            PR URL if successful, None otherwise
        """
        try:
            cmd = [
                "gh",
                "pr",
                "create",
                "--repo",
                request.repo,
                "--head",
                request.branch,
                "--base",
                request.base,
                "--title",
                request.title,
                "--body",
                request.body or "Created by C4 Artifact Service",
            ]

            if request.draft:
                cmd.append("--draft")

            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=60,
            )

            if result.returncode == 0:
                # gh pr create outputs the PR URL
                return result.stdout.strip()
            else:
                logger.error(f"gh pr create failed: {result.stderr}")
                return None

        except subprocess.TimeoutExpired:
            logger.error("PR creation timed out")
            return None
        except FileNotFoundError:
            logger.error("gh CLI not found")
            return None

    def get_artifact(self, artifact_id: str) -> Artifact | None:
        """Get artifact by ID."""
        return self._artifacts.get(artifact_id)

    def get_content_by_token(self, token: str) -> tuple[bytes, str] | None:
        """Get artifact content by download token.

        Args:
            token: Download token

        Returns:
            Tuple of (content, filename) if valid, None otherwise
        """
        download_token = self._tokens.get(token)
        if not download_token or not download_token.is_valid():
            return None

        artifact = self._artifacts.get(download_token.artifact_id)
        if not artifact:
            return None

        content = self._content.get(download_token.artifact_id)
        if not content:
            return None

        # Increment download count
        download_token.download_count += 1

        return content, artifact.name

    def list_artifacts(
        self,
        task_id: str | None = None,
        project_id: str | None = None,
        artifact_type: ArtifactType | None = None,
    ) -> list[Artifact]:
        """List artifacts with optional filters.

        Args:
            task_id: Filter by task ID
            project_id: Filter by project ID
            artifact_type: Filter by type

        Returns:
            List of matching artifacts
        """
        results = []
        for artifact in self._artifacts.values():
            if task_id and artifact.task_id != task_id:
                continue
            if project_id and artifact.project_id != project_id:
                continue
            if artifact_type and artifact.type != artifact_type:
                continue
            results.append(artifact)
        return sorted(results, key=lambda a: a.created_at, reverse=True)

    def cleanup_expired(self) -> int:
        """Remove expired artifacts.

        Returns:
            Number of artifacts cleaned up
        """
        now = datetime.now()
        expired_ids = [
            aid
            for aid, artifact in self._artifacts.items()
            if artifact.expires_at and artifact.expires_at < now
        ]

        for artifact_id in expired_ids:
            self._artifacts.pop(artifact_id, None)
            self._content.pop(artifact_id, None)

        # Clean up expired tokens
        expired_tokens = [token for token, dt in self._tokens.items() if not dt.is_valid()]
        for token in expired_tokens:
            self._tokens.pop(token, None)

        return len(expired_ids)


# =============================================================================
# Global Service
# =============================================================================


_artifact_service: ArtifactService | None = None


def get_artifact_service() -> ArtifactService:
    """Get or create artifact service instance."""
    global _artifact_service
    if _artifact_service is None:
        _artifact_service = ArtifactService()
    return _artifact_service


# =============================================================================
# Routes
# =============================================================================


@router.post("/zip", response_model=ArtifactResponse)
async def create_zip_artifact(
    request: ZipRequest,
    x_task_id: str | None = Header(None, alias="X-Task-ID"),
    x_project_id: str | None = Header(None, alias="X-Project-ID"),
    service: ArtifactService = Depends(get_artifact_service),
) -> ArtifactResponse:
    """Create ZIP artifact from files.

    Args:
        request: ZIP creation request
        x_task_id: Task ID header
        x_project_id: Project ID header
        service: Injected artifact service

    Returns:
        Created artifact info with download URL
    """

    # Override request IDs with headers if provided
    if x_task_id:
        request.task_id = x_task_id
    if x_project_id:
        request.project_id = x_project_id

    artifact = await service.create_zip(request)

    return ArtifactResponse(
        id=artifact.id,
        type=artifact.type.value,
        status=artifact.status.value,
        name=artifact.name,
        download_url=artifact.download_url,
        expires_at=artifact.expires_at.isoformat() if artifact.expires_at else None,
        size_bytes=artifact.size_bytes,
    )


@router.post("/pr", response_model=ArtifactResponse)
async def create_pr_artifact(
    request: PRRequest,
    x_task_id: str | None = Header(None, alias="X-Task-ID"),
    x_project_id: str | None = Header(None, alias="X-Project-ID"),
    service: ArtifactService = Depends(get_artifact_service),
) -> ArtifactResponse:
    """Create Pull Request artifact.

    Args:
        request: PR creation request
        x_task_id: Task ID header
        x_project_id: Project ID header
        service: Injected artifact service

    Returns:
        Created artifact info with PR URL
    """
    if x_task_id:
        request.task_id = x_task_id
    if x_project_id:
        request.project_id = x_project_id

    artifact = await service.create_pr(request)

    return ArtifactResponse(
        id=artifact.id,
        type=artifact.type.value,
        status=artifact.status.value,
        name=artifact.name,
        pr_url=artifact.pr_url,
    )


@router.get("/download/{token}")
async def download_artifact(
    token: str,
    service: ArtifactService = Depends(get_artifact_service),
) -> StreamingResponse:
    """Download artifact by token.

    Args:
        token: Secure download token
        service: Injected artifact service

    Returns:
        Streaming file response
    """
    result = service.get_content_by_token(token)
    if not result:
        raise HTTPException(
            status_code=404,
            detail="Artifact not found or download link expired",
        )

    content, filename = result

    return StreamingResponse(
        io.BytesIO(content),
        media_type="application/zip",
        headers={
            "Content-Disposition": f'attachment; filename="{filename}"',
            "Content-Length": str(len(content)),
        },
    )


@router.get("/")
async def list_all_artifacts(
    task_id: str | None = None,
    project_id: str | None = None,
    type: str | None = None,
    service: ArtifactService = Depends(get_artifact_service),
) -> list[dict[str, Any]]:
    """List artifacts with optional filters.

    Args:
        task_id: Filter by task ID
        project_id: Filter by project ID
        type: Filter by artifact type
        service: Injected artifact service

    Returns:
        List of artifacts
    """
    artifact_type = None
    if type:
        try:
            artifact_type = ArtifactType(type)
        except ValueError:
            raise HTTPException(status_code=400, detail=f"Invalid artifact type: {type}")

    artifacts = service.list_artifacts(
        task_id=task_id,
        project_id=project_id,
        artifact_type=artifact_type,
    )

    return [a.to_dict() for a in artifacts]


@router.get("/{artifact_id}")
async def get_artifact_by_id(
    artifact_id: str,
    service: ArtifactService = Depends(get_artifact_service),
) -> dict[str, Any]:
    """Get artifact info by ID.

    Args:
        artifact_id: Artifact ID
        service: Injected artifact service

    Returns:
        Artifact details
    """
    artifact = service.get_artifact(artifact_id)
    if not artifact:
        raise HTTPException(status_code=404, detail="Artifact not found")

    return artifact.to_dict()
