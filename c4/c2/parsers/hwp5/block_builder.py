"""BlockBuilder - HWP 구조를 IR 블록으로 변환.

파싱된 HWP 구조체(Paragraph, Table, ImageData)를
문서 중간 표현(IR) 블록으로 변환합니다.

IR 블록 구조:
- Document: 전체 문서
- Paragraph: 일반 문단
- Heading: 제목 (레벨 1-6)
- Table: 표 (thead/tbody 분리)
- Image: 이미지
"""

import re
from dataclasses import dataclass, field
from enum import Enum, auto

from .body_text_parser import Paragraph
from .doc_info_parser import CharShape, DocInfoParser
from .image_extractor import ImageData
from .table_parser import Cell, Table


class BlockType(Enum):
    """IR 블록 타입."""

    PARAGRAPH = auto()
    HEADING = auto()
    TABLE = auto()
    IMAGE = auto()
    TEXT_BOX = auto()
    LIST = auto()  # 미래 확장용


@dataclass
class TextRun:
    """텍스트 조각 (스타일 포함)."""

    text: str
    bold: bool = False
    italic: bool = False
    underline: bool = False
    font_size: float = 10.0  # pt
    font_name: str = ""


@dataclass
class ParagraphBlock:
    """문단 블록."""

    block_type: BlockType = BlockType.PARAGRAPH
    text: str = ""
    runs: list[TextRun] = field(default_factory=list)
    para_shape_id: int = 0
    style_id: int = 0


@dataclass
class HeadingBlock:
    """제목 블록."""

    block_type: BlockType = BlockType.HEADING
    level: int = 1  # 1-6
    text: str = ""
    runs: list[TextRun] = field(default_factory=list)
    numbering: str = ""  # "1.", "1.1", "가." 등


@dataclass
class TableCell:
    """테이블 셀 IR."""

    text: str = ""
    colspan: int = 1
    rowspan: int = 1
    is_header: bool = False
    col: int = 0  # 원본 테이블에서의 컬럼 위치
    border_fill_id: int = 0  # 테두리/배경 ID (스타일 조회용)
    first_char_shape_id: int = -1  # 첫 글자 모양 ID (-1=미설정)
    image_bin_ids: list[int] = field(default_factory=list)  # 셀 내 이미지 bin_id들


@dataclass
class TableRow:
    """테이블 행 IR."""

    cells: list[TableCell] = field(default_factory=list)
    is_header: bool = False


@dataclass
class TableBlock:
    """테이블 블록."""

    block_type: BlockType = BlockType.TABLE
    rows: list[TableRow] = field(default_factory=list)
    col_count: int = 0
    has_header: bool = False

    @property
    def thead(self) -> list[TableRow]:
        """헤더 행들을 반환합니다."""
        return [row for row in self.rows if row.is_header]

    @property
    def tbody(self) -> list[TableRow]:
        """본문 행들을 반환합니다."""
        return [row for row in self.rows if not row.is_header]


@dataclass
class ImageBlock:
    """이미지 블록."""

    block_type: BlockType = BlockType.IMAGE
    bin_id: int = 0
    width: int = 0
    height: int = 0
    format: str = ""
    alt_text: str = ""
    data: bytes = b""


# Union type for blocks
Block = ParagraphBlock | HeadingBlock | TableBlock | ImageBlock


@dataclass
class Document:
    """문서 IR."""

    blocks: list[Block] = field(default_factory=list)
    title: str = ""

    @property
    def block_count(self) -> int:
        """블록 수를 반환합니다."""
        return len(self.blocks)

    def get_blocks_by_type(self, block_type: BlockType) -> list[Block]:
        """특정 타입의 블록들을 반환합니다."""
        return [b for b in self.blocks if b.block_type == block_type]


# 제목 판별을 위한 번호 패턴
HEADING_PATTERNS = [
    # 숫자 기반
    r"^(\d+)\.\s",  # "1. ", "2. "
    r"^(\d+)\.(\d+)\s",  # "1.1 ", "2.3 "
    r"^(\d+)\.(\d+)\.(\d+)\s",  # "1.1.1 "
    r"^제\s*(\d+)\s*[장절조항]",  # "제1장", "제 2 절"
    r"^(\d+)\)",  # "1)", "2)"
    # 한글 기반
    r"^[가나다라마바사아자차카타파하]\.\s",  # "가. ", "나. "
    r"^[ㄱㄴㄷㄹㅁㅂㅅㅇㅈㅊㅋㅌㅍㅎ]\.\s",  # "ㄱ. "
    # 괄호 기반
    r"^\([가나다라마바사]\)",  # "(가)", "(나)"
    r"^\(\d+\)",  # "(1)", "(2)"
    r"^[①②③④⑤⑥⑦⑧⑨⑩]",  # 원숫자
]

COMPILED_HEADING_PATTERNS = [re.compile(p) for p in HEADING_PATTERNS]


class BlockBuilder:
    """HWP 구조를 IR 블록으로 변환합니다.

    사용법:
        builder = BlockBuilder(doc_info_parser)
        document = builder.build_document(
            paragraphs=paragraphs,
            tables=tables,
            images=images,
        )
    """

    # 제목 판별 기준
    HEADING_MIN_FONT_SIZE = 14.0  # pt
    HEADING_LEVEL_SIZES = {
        1: 24.0,  # >= 24pt
        2: 18.0,  # >= 18pt
        3: 16.0,  # >= 16pt
        4: 14.0,  # >= 14pt
        5: 12.0,  # >= 12pt
        6: 10.0,  # >= 10pt
    }

    def __init__(self, doc_info: DocInfoParser | None = None) -> None:
        """BlockBuilder를 초기화합니다.

        Args:
            doc_info: DocInfoParser 인스턴스 (스타일 정보용)
        """
        self._doc_info = doc_info

    def build_document(
        self,
        paragraphs: list[Paragraph],
        tables: list[Table] | None = None,
        images: list[ImageData] | None = None,
    ) -> Document:
        """파싱된 구조를 Document IR로 변환합니다.

        Args:
            paragraphs: 문단 리스트
            tables: 테이블 리스트 (선택)
            images: 이미지 리스트 (선택)

        Returns:
            Document IR
        """
        tables = tables or []
        images = images or []

        document = Document()

        # 문단, 테이블, 이미지를 정렬
        # - 유효한 항목들이 모두 y_offset > 0이면 y_offset으로 정렬 (페이지 레이아웃 문서)
        # - 그렇지 않으면 record_index로 정렬 (일반 문서)
        items: list[tuple[int, int, str, Paragraph | Table | ImageData]] = []

        for para in paragraphs:
            items.append((para.record_index, para.y_offset, "para", para))

        for table in tables:
            items.append((table.record_index, table.y_offset, "table", table))

        # 이미지도 정렬 대상에 포함 (record_index가 있는 경우)
        # record_index가 없는 이미지는 별도로 문서 끝에 추가
        images_with_position: list[ImageData] = []
        images_without_position: list[ImageData] = []

        for image in images:
            if image.record_index is not None:
                items.append((image.record_index, 0, "image", image))
                images_with_position.append(image)
            else:
                images_without_position.append(image)

        # 유효한 항목들(텍스트 있는 문단 또는 테이블)의 y_offset 확인
        # 빈 문단은 y_offset 체크에서 제외
        valid_items = [
            (rec_idx, y_offset, item_type, item)
            for rec_idx, y_offset, item_type, item in items
            if item_type == "table" or (hasattr(item, "text") and item.text and item.text.strip())
        ]

        # 유효한 항목들이 모두 y_offset > 0인지 확인 (GSO 문단 등 페이지 레이아웃 문서)
        all_have_y_offset = (
            all(y_offset > 0 for _, y_offset, _, _ in valid_items)
            if valid_items else False
        )

        if all_have_y_offset:
            # 페이지 레이아웃 문서: y_offset으로 정렬, 같으면 record_index로
            items.sort(key=lambda x: (x[1], x[0]))
        else:
            # 일반 문서: record_index로 정렬, 같은 record_index 내에서 y_offset으로
            items.sort(key=lambda x: (x[0], x[1] if x[1] > 0 else 0))

        blocks: list[Block] = []

        # 정렬된 순서대로 블록 생성
        for _, y_offset, item_type, item in items:
            if item_type == "para":
                block = self._convert_paragraph(item)  # type: ignore
                if block:
                    blocks.append(block)
            elif item_type == "table":
                block = self._convert_table(item)  # type: ignore
                if block:
                    blocks.append(block)
            elif item_type == "image":
                block = self._convert_image(item)  # type: ignore
                if block:
                    blocks.append(block)

        # record_index가 없는 이미지는 문서 끝에 추가
        for image in images_without_position:
            block = self._convert_image(image)
            if block:
                blocks.append(block)

        document.blocks = blocks

        # 첫 번째 제목을 문서 제목으로 설정
        headings = document.get_blocks_by_type(BlockType.HEADING)
        if headings:
            first_heading = headings[0]
            if isinstance(first_heading, HeadingBlock):
                document.title = first_heading.text

        return document

    def build_blocks(
        self,
        paragraphs: list[Paragraph],
        tables: list[Table] | None = None,
        images: list[ImageData] | None = None,
    ) -> list[Block]:
        """파싱된 구조를 블록 리스트로 변환합니다.

        Args:
            paragraphs: 문단 리스트
            tables: 테이블 리스트 (선택)
            images: 이미지 리스트 (선택)

        Returns:
            Block 리스트
        """
        doc = self.build_document(paragraphs, tables, images)
        return doc.blocks

    def _convert_paragraph(self, para: Paragraph) -> ParagraphBlock | HeadingBlock | None:
        """Paragraph를 ParagraphBlock 또는 HeadingBlock으로 변환합니다."""
        text = para.text.strip()
        if not text:
            return None

        # 글자 모양 정보 가져오기
        char_shape = self._get_char_shape(para)

        # 제목 여부 판별
        is_heading, heading_level, numbering = self._detect_heading(
            text, char_shape
        )

        if is_heading:
            return HeadingBlock(
                level=heading_level,
                text=text,
                numbering=numbering,
                runs=self._build_text_runs(para, char_shape),
            )
        else:
            return ParagraphBlock(
                text=text,
                para_shape_id=para.para_shape_id,
                style_id=para.style_id,
                runs=self._build_text_runs(para, char_shape),
            )

    def _convert_table(self, table: Table) -> TableBlock | None:
        """Table을 TableBlock으로 변환합니다."""
        if table.row_count == 0 or table.col_count == 0:
            return None

        table_block = TableBlock(col_count=table.col_count)
        rows: list[TableRow] = []

        for row_idx in range(table.row_count):
            row_cells: list[TableCell] = []
            is_header_row = row_idx == 0 and table.repeat_header

            for col_idx in range(table.col_count):
                cell = table.get_cell(row_idx, col_idx)

                if cell and cell.row == row_idx and cell.col == col_idx:
                    # 셀의 텍스트 추출
                    cell_text = self._extract_cell_text(cell)
                    # 셀 내 이미지 bin_id 추출
                    image_ids = cell.image_bin_ids if hasattr(cell, 'image_bin_ids') else []
                    row_cells.append(
                        TableCell(
                            text=cell_text,
                            colspan=cell.colspan,
                            rowspan=cell.rowspan,
                            is_header=is_header_row,
                            col=col_idx,  # 원본 컬럼 위치 저장
                            border_fill_id=cell.border_fill_id,
                            first_char_shape_id=cell.first_char_shape_id,
                            image_bin_ids=list(image_ids),
                        )
                    )
                elif cell:
                    # 병합된 셀의 일부 - 스킵 (colspan/rowspan이 처리)
                    continue
                else:
                    row_cells.append(TableCell(text=""))

            if row_cells:
                rows.append(TableRow(cells=row_cells, is_header=is_header_row))

        table_block.rows = rows
        table_block.has_header = any(row.is_header for row in rows)

        return table_block

    def _convert_image(self, image: ImageData) -> ImageBlock:
        """ImageData를 ImageBlock으로 변환합니다."""
        return ImageBlock(
            bin_id=image.bin_id,
            width=image.width,
            height=image.height,
            format=image.extension.lstrip("."),
            data=image.data,
        )

    def _get_char_shape(self, para: Paragraph) -> CharShape | None:
        """문단의 첫 글자 모양을 가져옵니다."""
        if not self._doc_info or not para.char_shapes:
            return None

        # 첫 번째 글자 모양 ID
        first_shape_ref = para.char_shapes[0]
        return self._doc_info.get_char_shape(first_shape_ref.shape_id)

    def _detect_heading(
        self, text: str, char_shape: CharShape | None
    ) -> tuple[bool, int, str]:
        """텍스트가 제목인지 판별합니다.

        Args:
            text: 텍스트
            char_shape: 글자 모양

        Returns:
            (is_heading, level, numbering) 튜플
        """
        # 1. 글자 모양 기반 판별 (font_size >= 14pt and bold)
        is_style_heading = False
        heading_level = 4  # 기본 레벨

        if char_shape:
            is_style_heading = (
                char_shape.font_size >= self.HEADING_MIN_FONT_SIZE
                and char_shape.bold
            )

            # 폰트 크기로 레벨 결정
            for level, min_size in sorted(
                self.HEADING_LEVEL_SIZES.items()
            ):
                if char_shape.font_size >= min_size:
                    heading_level = level
                    break

        # 2. 번호 패턴 기반 판별
        numbering = ""
        has_numbering = False

        for pattern in COMPILED_HEADING_PATTERNS:
            match = pattern.match(text)
            if match:
                numbering = match.group(0).strip()
                has_numbering = True

                # 패턴 깊이로 레벨 조정
                dot_count = numbering.count(".")
                if dot_count >= 2:
                    heading_level = min(heading_level, 3)
                elif dot_count == 1:
                    heading_level = min(heading_level, 2)
                break

        # 최종 판단: 스타일 또는 번호 패턴
        is_heading = is_style_heading or (has_numbering and len(text) < 100)

        return is_heading, heading_level, numbering

    def _build_text_runs(
        self, para: Paragraph, default_shape: CharShape | None
    ) -> list[TextRun]:
        """문단의 텍스트 런을 생성합니다."""
        # 간단한 구현: 전체 텍스트를 하나의 런으로
        run = TextRun(text=para.text)

        if default_shape:
            run.bold = default_shape.bold
            run.italic = default_shape.italic
            run.underline = default_shape.underline_type != 0
            run.font_size = default_shape.font_size

        return [run]

    def _extract_cell_text(self, cell: Cell) -> str:
        """셀에서 텍스트를 추출합니다."""
        texts: list[str] = []
        for para in cell.paragraphs:
            if hasattr(para, "text"):
                texts.append(para.text)
            elif isinstance(para, str):
                texts.append(para)
            elif isinstance(para, Table):
                # 중첩 테이블의 텍스트도 추출
                nested_text = self._extract_nested_table_text(para)
                if nested_text:
                    texts.append(nested_text)
        return "\n".join(texts)

    def _extract_nested_table_text(self, table: Table) -> str:
        """중첩 테이블을 HTML 테이블로 변환합니다."""
        from html import escape

        lines: list[str] = ['<table class="nested-table">']
        lines.append("  <tbody>")

        for row_idx in range(table.row_count):
            lines.append("    <tr>")
            for col_idx in range(table.col_count):
                cell = table.get_cell(row_idx, col_idx)
                if cell and cell.row == row_idx and cell.col == col_idx:
                    # 셀 텍스트 추출 (재귀적으로 중첩 테이블 처리)
                    cell_text = self._extract_cell_text(cell)
                    # 중첩 테이블 HTML이 아닌 경우에만 escape
                    if "<table" not in cell_text:
                        cell_text = escape(cell_text) if cell_text else ""
                        cell_text = cell_text.replace("\n", "<br>")

                    # 병합 속성
                    attrs = ""
                    if cell.rowspan > 1:
                        attrs += f' rowspan="{cell.rowspan}"'
                    if cell.colspan > 1:
                        attrs += f' colspan="{cell.colspan}"'

                    lines.append(f"      <td{attrs}>{cell_text}</td>")
            lines.append("    </tr>")

        lines.append("  </tbody>")
        lines.append("</table>")
        return "\n".join(lines)
