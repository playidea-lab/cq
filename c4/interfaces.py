"""C4 Abstract Interfaces - Extension points for Cloud/Remote implementations.

현재는 로컬 구현만 제공. ABC 인터페이스를 분리해두면
C4 Cloud 확장 시 구현체만 교체 가능.

Local implementations:
- LocalArtifactStore (c4/artifacts/store.py)
- LocalKnowledgeStore (c4/knowledge/store.py)
- LocalGpuScheduler (c4/gpu/monitor.py)
- LocalTracker (c4/tracker/decorator.py)
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from pathlib import Path
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from c4.models.task import ArtifactRef, ExecutionStats


class ArtifactStore(ABC):
    """아티팩트 저장소 인터페이스.

    현재: LocalArtifactStore (파일 복사 + SHA256)
    Cloud: RemoteArtifactStore (presigned URL + S3/MinIO)
    """

    @abstractmethod
    async def save(
        self, task_id: str, local_path: Path, artifact_type: str = "output"
    ) -> ArtifactRef:
        """아티팩트를 저장소에 저장.

        Args:
            task_id: 관련 태스크 ID
            local_path: 저장할 로컬 파일 경로
            artifact_type: source, data, output 중 하나

        Returns:
            저장된 아티팩트 참조
        """
        ...

    @abstractmethod
    async def get(
        self, task_id: str, name: str, version: int | None = None
    ) -> Path:
        """아티팩트 경로 반환.

        Args:
            task_id: 태스크 ID
            name: 아티팩트 이름
            version: 특정 버전 (None이면 최신)

        Returns:
            로컬 파일 경로
        """
        ...

    @abstractmethod
    async def list(self, task_id: str) -> list[ArtifactRef]:
        """태스크별 아티팩트 목록.

        Args:
            task_id: 태스크 ID

        Returns:
            아티팩트 참조 목록
        """
        ...

    @abstractmethod
    async def delete(self, task_id: str, name: str) -> bool:
        """아티팩트 삭제.

        Args:
            task_id: 태스크 ID
            name: 아티팩트 이름

        Returns:
            삭제 성공 여부
        """
        ...


class KnowledgeStore(ABC):
    """지식 저장소 인터페이스.

    현재: DocumentStore (Obsidian-style Markdown + FTS5)
    Cloud: SharedKnowledgeStore (팀간 공유, 중앙 서버)

    Note: 검색은 KnowledgeSearcher (별도 클래스)가 담당.
    """

    @abstractmethod
    def create(
        self, doc_type: str, metadata: dict[str, Any], body: str = ""
    ) -> str:
        """문서 생성.

        Args:
            doc_type: experiment, pattern, insight, hypothesis
            metadata: frontmatter 메타데이터
            body: Markdown 본문

        Returns:
            생성된 문서 ID (exp-xxxx, pat-xxxx 등)
        """
        ...

    @abstractmethod
    def get(self, doc_id: str) -> Any:
        """문서 조회.

        Args:
            doc_id: 문서 ID

        Returns:
            KnowledgeDocument 또는 None
        """
        ...

    @abstractmethod
    def update(
        self, doc_id: str, metadata: dict[str, Any] | None = None, body: str | None = None
    ) -> bool:
        """문서 업데이트.

        Args:
            doc_id: 문서 ID
            metadata: 업데이트할 메타데이터
            body: 업데이트할 본문

        Returns:
            성공 여부
        """
        ...

    @abstractmethod
    def delete(self, doc_id: str) -> bool:
        """문서 삭제.

        Args:
            doc_id: 문서 ID

        Returns:
            삭제 성공 여부
        """
        ...


class GpuScheduler(ABC):
    """GPU 스케줄링 인터페이스.

    현재: LocalGpuScheduler (단일 머신 GPU)
    Cloud: RemoteGpuScheduler (multi-node GPU pool)
    """

    @abstractmethod
    def detect_gpus(self) -> list[dict[str, Any]]:
        """사용 가능한 GPU 목록 반환.

        Returns:
            GpuInfo dict 목록
        """
        ...

    @abstractmethod
    def allocate(
        self, gpu_count: int = 1, min_vram_gb: float = 8.0
    ) -> list[int]:
        """GPU 할당.

        Args:
            gpu_count: 필요한 GPU 수
            min_vram_gb: 최소 VRAM (GB)

        Returns:
            할당된 GPU 인덱스 목록
        """
        ...

    @abstractmethod
    def release(self, gpu_ids: list[int]) -> None:
        """GPU 할당 해제.

        Args:
            gpu_ids: 해제할 GPU 인덱스 목록
        """
        ...


class ExperimentTracker(ABC):
    """실험 추적 인터페이스.

    현재: LocalTracker (@c4_track → Task.execution_stats)
    Cloud: RemoteTracker (중앙 서버 + 실시간 대시보드)
    """

    @abstractmethod
    def start_run(self, task_id: str, code_features: dict[str, Any]) -> str:
        """실험 실행 시작.

        Args:
            task_id: 관련 태스크 ID
            code_features: AST 분석 결과 (imports, algorithm 등)

        Returns:
            run ID
        """
        ...

    @abstractmethod
    def log_metrics(self, run_id: str, metrics: dict[str, Any]) -> None:
        """메트릭 기록.

        Args:
            run_id: 실행 ID
            metrics: key-value 메트릭
        """
        ...

    @abstractmethod
    def end_run(
        self, run_id: str, final_stats: ExecutionStats | None = None
    ) -> None:
        """실험 실행 종료.

        Args:
            run_id: 실행 ID
            final_stats: 최종 실행 통계
        """
        ...
