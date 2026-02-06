"""C4 Local Artifact Store - content-addressable artifact management.

Provides SHA256-based deduplication and 3-tier classification (source/data/output).
"""

from .models import ArtifactRecord, ArtifactVersion
from .store import LocalArtifactStore

__all__ = ["ArtifactRecord", "ArtifactVersion", "LocalArtifactStore"]
