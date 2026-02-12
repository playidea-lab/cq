"""IR (Intermediate Representation) Schema Definition.

모든 문서 포맷을 동일한 구조로 변환하기 위한 중간 표현.
"""

from typing import Literal

from pydantic import BaseModel


class HeadingBlock(BaseModel):
    """제목 블록."""

    type: Literal["heading"] = "heading"
    level: int  # 1, 2, 3
    text: str


class ParagraphBlock(BaseModel):
    """문단 블록."""

    type: Literal["paragraph"] = "paragraph"
    text: str
    font_size: float | None = None  # pt 단위
    is_bold: bool = False


class MergeInfo(BaseModel):
    """셀 병합 정보."""

    row: int
    col: int
    rowspan: int = 1
    colspan: int = 1


class CellStyle(BaseModel):
    """셀 스타일 정보 (헤더 감지용)."""

    background_color: str | None = None  # "#RRGGBB" 형식
    is_bold: bool = False
    font_size: float | None = None  # pt 단위
    text_align: str | None = None  # "left", "center", "right"


class TableBlock(BaseModel):
    """테이블 블록."""

    type: Literal["table"] = "table"
    header: list[str]  # 첫 번째 행 (thead)
    rows: list[list[str]]  # 나머지 행들 (tbody)
    merge_info: list[MergeInfo] | None = None
    header_rows: int = 1  # 헤더 행 개수 (기본 1, 복잡한 테이블은 2 이상)
    # 스타일 정보 (헤더 감지용) - 모든 행 포함 (header + rows)
    cell_styles: list[list[CellStyle]] | None = None


class ImageBlock(BaseModel):
    """이미지 블록."""

    type: Literal["image"] = "image"
    image_id: str  # 이미지 파일 ID (저장 시 파일명으로 사용)
    mime_type: str  # image/jpeg, image/png 등
    caption: str | None = None

    model_config = {"arbitrary_types_allowed": True}


class ListBlock(BaseModel):
    """리스트 블록."""

    type: Literal["list"] = "list"
    list_type: Literal["ordered", "unordered"]
    items: list[str]
    level: int = 0  # 중첩 레벨


# Union type for all block types
Block = HeadingBlock | ParagraphBlock | TableBlock | ImageBlock | ListBlock


class Document(BaseModel):
    """문서 전체 IR 구조."""

    blocks: list[Block]


def create_heading(level: int, text: str) -> HeadingBlock:
    """헤딩 블록 생성 헬퍼."""
    return HeadingBlock(level=min(max(level, 1), 3), text=text)


def create_paragraph(
    text: str,
    font_size: float | None = None,
    is_bold: bool = False,
) -> ParagraphBlock:
    """문단 블록 생성 헬퍼."""
    return ParagraphBlock(text=text, font_size=font_size, is_bold=is_bold)


def create_table(
    header: list[str],
    rows: list[list[str]],
    merge_info: list[MergeInfo] | None = None,
    header_rows: int = 1,
    cell_styles: list[list[CellStyle]] | None = None,
) -> TableBlock:
    """테이블 블록 생성 헬퍼."""
    return TableBlock(
        header=header,
        rows=rows,
        merge_info=merge_info,
        header_rows=header_rows,
        cell_styles=cell_styles,
    )


def create_image(
    image_id: str,
    mime_type: str,
    caption: str | None = None,
) -> ImageBlock:
    """이미지 블록 생성 헬퍼."""
    return ImageBlock(image_id=image_id, mime_type=mime_type, caption=caption)


def create_list(
    list_type: Literal["ordered", "unordered"],
    items: list[str],
    level: int = 0,
) -> ListBlock:
    """리스트 블록 생성 헬퍼."""
    return ListBlock(list_type=list_type, items=items, level=level)
