"""Base Parser - 모든 파서의 추상 기본 클래스."""

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from pathlib import Path

from c4.c2.parsers.ir_models import Document


@dataclass
class ImageData:
    """추출된 이미지 데이터."""

    image_id: str
    data: bytes
    mime_type: str


@dataclass
class ParseResult:
    """파싱 결과.

    Attributes:
        document: IR 문서 구조
        images: 추출된 이미지 목록
    """

    document: Document
    images: list[ImageData] = field(default_factory=list)


class BaseParser(ABC):
    """문서 파서 추상 기본 클래스.

    모든 포맷별 파서는 이 클래스를 상속받아 구현.
    """

    @abstractmethod
    def parse(self, file_path: Path) -> Document:
        """문서를 파싱하여 IR(Document)로 변환.

        Args:
            file_path: 문서 파일 경로

        Returns:
            Document: IR 구조
        """
        pass

    def parse_with_images(self, file_path: Path) -> ParseResult:
        """문서를 파싱하여 IR과 이미지를 함께 반환.

        기본 구현은 이미지 없이 Document만 반환.
        이미지 추출이 필요한 파서는 이 메서드를 오버라이드.

        Args:
            file_path: 문서 파일 경로

        Returns:
            ParseResult: Document + 이미지 목록
        """
        return ParseResult(document=self.parse(file_path))

    @property
    @abstractmethod
    def supported_extensions(self) -> list[str]:
        """지원하는 파일 확장자 목록."""
        pass
