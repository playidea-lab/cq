"""c2 document converter.

멀티포맷 문서 변환 통합 API:
- parse_document(): 모든 포맷 → IR Document
- convert_to_html(): IR → HTML
- extract_text(): 모든 포맷 → 플레인 텍스트
- create_hwpx(): IR → HWPX 파일 생성

기존 호환:
- PDFConverter: PDF → 페이지 이미지 (auto_review 레거시)
- extract_text_from_docx(): DOCX → 텍스트 (레거시)
"""

from __future__ import annotations

from pathlib import Path

from c4.c2.parsers.dispatcher import ParserDispatcher
from c4.c2.parsers.ir_models import Document, HeadingBlock, ListBlock, ParagraphBlock, TableBlock
from c4.c2.parsers.normalizer import normalize_document
from c4.c2.parsers.writer.md_to_ir import markdown_to_ir

# Re-export auto_review converter for backward compatibility
from c4.review.converter import PDFConverter

_dispatcher = ParserDispatcher()


def parse_document(file_path: Path) -> Document:
    """모든 포맷 파싱 → IR Document.

    Args:
        file_path: 문서 파일 경로 (HWP, HWPX, DOCX, PDF, XLSX, PPTX 등)

    Returns:
        IR Document 구조

    Raises:
        ValueError: 지원하지 않는 파일 포맷
    """
    return _dispatcher.parse(file_path)


def convert_to_html(
    document: Document,
    theme: str | None = None,
    image_paths: dict[str, str] | None = None,
) -> str:
    """IR Document → HTML 변환.

    Args:
        document: IR Document
        theme: CSS 테마명 (default, minimal, dark, print)
        image_paths: 이미지 ID → 파일 경로 매핑

    Returns:
        완성된 HTML 문자열
    """
    return normalize_document(document, image_paths=image_paths, theme=theme)


def extract_text(file_path: Path) -> str:
    """모든 포맷 → 플레인 텍스트 추출.

    Args:
        file_path: 문서 파일 경로

    Returns:
        추출된 텍스트
    """
    doc = parse_document(file_path)
    parts: list[str] = []

    for block in doc.blocks:
        if isinstance(block, HeadingBlock):
            parts.append(block.text)
        elif isinstance(block, ParagraphBlock):
            parts.append(block.text)
        elif isinstance(block, TableBlock):
            # 헤더 + 행들을 탭 구분 텍스트로
            parts.append("\t".join(block.header))
            for row in block.rows:
                parts.append("\t".join(row))
        elif isinstance(block, ListBlock):
            for item in block.items:
                parts.append(f"- {item}")

    return "\n\n".join(parts)


def create_hwpx(
    document: Document,
    output_path: Path,
    template: Path | None = None,
) -> None:
    """IR Document → HWPX 파일 생성.

    Args:
        document: IR Document
        output_path: 출력 HWPX 파일 경로
        template: HWPX 템플릿 파일 경로 (None이면 빈 문서 사용)
    """
    from c4.c2.parsers.writer.hwpx_writer import HwpxWriter

    writer = HwpxWriter(template_path=template)
    writer.write(document, output_path)


def extract_text_from_docx(docx_path: Path) -> str:
    """DOCX에서 텍스트 추출 (레거시 호환).

    새 코드에서는 extract_text() 사용을 권장합니다.
    """
    return extract_text(docx_path)


__all__ = [
    "PDFConverter",
    "convert_to_html",
    "create_hwpx",
    "extract_text",
    "extract_text_from_docx",
    "markdown_to_ir",
    "parse_document",
]
