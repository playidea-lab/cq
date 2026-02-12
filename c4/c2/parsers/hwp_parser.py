"""HWP Parser - HWP 5.0 스펙 기반 바이트 레벨 파서.

새로운 hwp5 모듈을 사용하여 HWP 문서를 파싱합니다.
- OleReader: OLE Compound Document 읽기
- DocInfoParser: CharShape, ParaShape 등 스타일 정보
- BodyTextParser: 문단 텍스트 추출
- TableParser: 테이블 구조 추출
- ImageExtractor: 이미지 추출
- BlockBuilder: IR 블록 변환
"""

import struct
from dataclasses import dataclass
from pathlib import Path

from c4.c2.parsers.base import BaseParser, ImageData, ParseResult
from c4.c2.parsers.hwp5 import (
    BlockBuilder,
    BlockType,
    BodyTextParser,
    DocInfoParser,
    ImageExtractor,
    OleReader,
    TableParser,
)
from c4.c2.parsers.hwp5 import Document as Hwp5Document
from c4.c2.parsers.hwp5 import HeadingBlock as Hwp5HeadingBlock
from c4.c2.parsers.hwp5 import ImageBlock as Hwp5ImageBlock
from c4.c2.parsers.hwp5 import ParagraphBlock as Hwp5ParagraphBlock
from c4.c2.parsers.hwp5 import TableBlock as Hwp5TableBlock
from c4.c2.parsers.ir_models import (
    CellStyle,
    Document,
    MergeInfo,
    create_heading,
    create_image,
    create_paragraph,
    create_table,
)
from c4.c2.parsers.utils.image import generate_image_id


@dataclass
class CharShapeInfo:
    """글자 모양 정보 (레거시 호환용)."""

    char_shape_id: int
    font_size: float  # 폰트 크기 (pt)
    is_bold: bool
    is_italic: bool


# 확장자 → MIME 타입 매핑
EXTENSION_TO_MIME = {
    ".png": "image/png",
    ".jpg": "image/jpeg",
    ".jpeg": "image/jpeg",
    ".gif": "image/gif",
    ".bmp": "image/bmp",
    ".tiff": "image/tiff",
    ".wmf": "image/x-wmf",
    ".emf": "image/x-emf",
    ".bin": "application/octet-stream",
}


class HwpParser(BaseParser):
    """HWP 문서 파서.

    HWP 5.0 스펙 기반 바이트 레벨 파서를 사용하여
    텍스트, 테이블, 이미지를 추출합니다.

    사용법:
        parser = HwpParser()
        document = parser.parse(Path("document.hwp"))
        # 또는
        result = parser.parse_with_images(Path("document.hwp"))
    """

    @property
    def supported_extensions(self) -> list[str]:
        return [".hwp"]

    def parse(self, file_path: Path) -> Document:
        """HWP 파일을 IR로 변환 (이미지 제외)."""
        result = self.parse_with_images(file_path)
        return result.document

    def parse_with_images(self, file_path: Path) -> ParseResult:
        """HWP 파일을 IR과 이미지로 변환.

        Args:
            file_path: HWP 파일 경로

        Returns:
            ParseResult: Document + 이미지 목록
        """
        try:
            return self._parse_hwp(file_path)
        except Exception as e:
            # 파싱 실패 시 예외 발생 (내부 에러 메시지 노출 방지)
            import logging
            logging.getLogger(__name__).error(f"HWP 파싱 실패: {e}", exc_info=True)
            raise ValueError("HWP 파일을 처리할 수 없습니다.")

    def _parse_hwp(self, file_path: Path) -> ParseResult:
        """HWP 파일 파싱 핵심 로직."""
        # 1. OLE 파일 열기
        reader = OleReader(file_path)

        # 2. DocInfo 파싱 (CharShape 등)
        doc_info = self._parse_doc_info(reader)

        # 3. BodyText 파싱 (문단) + GSO 위치 정보
        paragraphs, gso_positions = self._parse_body_text(reader)

        # 4. 테이블 파싱
        tables = self._parse_tables(reader)

        # 5. 이미지 추출 (GSO 위치 정보 전달)
        images, image_id_map = self._extract_images(reader, gso_positions)

        # 5.1 테이블 셀에 포함된 이미지 bin_id 수집
        cell_image_bin_ids = self._collect_cell_image_bin_ids(tables)

        # 6. BlockBuilder로 hwp5.Document 변환
        # 셀에 포함된 이미지는 별도 블록으로 생성하지 않음
        images_for_blocks = [
            img for img in images.values()
            if img.bin_id not in cell_image_bin_ids
        ]
        builder = BlockBuilder(doc_info=doc_info)
        hwp5_doc = builder.build_document(
            paragraphs=paragraphs,
            tables=tables,
            images=images_for_blocks,
        )

        # 7. hwp5.Document → ir_builder.Document 변환
        ir_document = self._convert_to_ir_document(hwp5_doc, image_id_map, doc_info)

        # 8. 이미지 데이터 변환
        extracted_images = self._convert_images(images, image_id_map)

        # 빈 문서 처리
        if not ir_document.blocks:
            ir_document = Document(
                blocks=[create_paragraph("HWP 파일에서 내용을 추출할 수 없습니다.")]
            )

        return ParseResult(document=ir_document, images=extracted_images)

    def _collect_cell_image_bin_ids(self, tables: list) -> set[int]:
        """테이블 셀에 포함된 이미지 bin_id들을 수집합니다."""
        bin_ids: set[int] = set()
        for table in tables:
            for cell in table.cells:
                if hasattr(cell, 'image_bin_ids') and cell.image_bin_ids:
                    bin_ids.update(cell.image_bin_ids)
        return bin_ids

    def _parse_doc_info(self, reader: OleReader) -> DocInfoParser | None:
        """DocInfo 스트림 파싱."""
        try:
            data = reader.read_stream("DocInfo")
            parser = DocInfoParser()
            parser.parse(data)
            return parser
        except Exception:
            return None

    def _parse_body_text(self, reader: OleReader) -> tuple[list, dict[int, int]]:
        """BodyText 스트림 파싱.

        Returns:
            (paragraphs, gso_positions) 튜플
            - paragraphs: 문단 리스트
            - gso_positions: {bin_id: record_index} 이미지 위치 매핑
        """
        paragraphs = []
        gso_positions: dict[int, int] = {}
        parser = BodyTextParser()

        # BodyText/Section0, Section1, ... 정렬된 순서로 읽기
        sections = sorted(
            [s for s in reader.list_streams() if s.startswith("BodyText/Section")]
        )

        for section_idx, stream_name in enumerate(sections):
            try:
                data = reader.read_stream(stream_name)
                section_paragraphs = parser.parse(data)

                # 섹션별 record_index 오프셋 적용 (섹션 간 순서 유지)
                section_offset = section_idx * 1000000
                for para in section_paragraphs:
                    para.record_index += section_offset

                paragraphs.extend(section_paragraphs)

                # GSO 위치 정보 수집 (오프셋 적용)
                for bin_id, ri in parser.gso_positions.items():
                    gso_positions[bin_id] = ri + section_offset
            except Exception:
                pass

        return paragraphs, gso_positions

    def _parse_tables(self, reader: OleReader) -> list:
        """테이블 파싱."""
        tables = []

        # BodyText/Section0, Section1, ... 정렬된 순서로 읽기
        sections = sorted(
            [s for s in reader.list_streams() if s.startswith("BodyText/Section")]
        )

        for section_idx, stream_name in enumerate(sections):
            try:
                parser = TableParser()  # 각 섹션마다 새 파서 인스턴스
                data = reader.read_stream(stream_name)
                section_tables = parser.parse(data)

                # 섹션별 record_index 오프셋 적용 (섹션 간 순서 유지)
                section_offset = section_idx * 1000000
                for table in section_tables:
                    table.record_index += section_offset

                tables.extend(section_tables)
            except Exception:
                pass

        return tables

    def _extract_images(
        self, reader: OleReader, gso_positions: dict[int, int] | None = None
    ) -> tuple[dict, dict]:
        """이미지 추출.

        Args:
            reader: OleReader 인스턴스
            gso_positions: {bin_id: record_index} GSO 위치 매핑

        Returns:
            (images, image_id_map) 튜플
            - images: {bin_id: ImageData} (record_index 설정됨)
            - image_id_map: {bin_id: image_id_with_ext}
        """
        images = {}
        image_id_map = {}
        gso_positions = gso_positions or {}

        try:
            extractor = ImageExtractor(reader)
            extracted = extractor.extract_images()

            for idx, img in enumerate(extracted):
                # GSO 위치 정보가 있으면 record_index 설정
                if img.bin_id in gso_positions:
                    img.record_index = gso_positions[img.bin_id]

                images[img.bin_id] = img

                # image_id 생성 (hash + extension)
                ext = img.extension.lower()
                image_id = generate_image_id(img.data, idx)
                image_id_with_ext = f"{image_id}{ext}"

                image_id_map[img.bin_id] = image_id_with_ext
        except Exception:
            pass

        return images, image_id_map

    def _convert_to_ir_document(
        self,
        hwp5_doc: Hwp5Document,
        image_id_map: dict,
        doc_info: DocInfoParser | None = None,
    ) -> Document:
        """hwp5.Document를 ir_builder.Document로 변환."""
        blocks = []

        for block in hwp5_doc.blocks:
            ir_block = self._convert_block(block, image_id_map, doc_info)
            if ir_block:
                blocks.append(ir_block)

        return Document(blocks=blocks)

    def _convert_block(
        self, block, image_id_map: dict, doc_info: DocInfoParser | None = None
    ):
        """hwp5 블록을 IR 블록으로 변환."""
        if block.block_type == BlockType.PARAGRAPH:
            if isinstance(block, Hwp5ParagraphBlock):
                return self._convert_paragraph(block)

        elif block.block_type == BlockType.HEADING:
            if isinstance(block, Hwp5HeadingBlock):
                return self._convert_heading(block)

        elif block.block_type == BlockType.TABLE:
            if isinstance(block, Hwp5TableBlock):
                return self._convert_table(block, doc_info, image_id_map)

        elif block.block_type == BlockType.IMAGE:
            if isinstance(block, Hwp5ImageBlock):
                return self._convert_image(block, image_id_map)

        return None

    def _convert_paragraph(self, block: Hwp5ParagraphBlock):
        """ParagraphBlock 변환."""
        text = block.text.strip()
        if not text:
            return None

        # 텍스트 런에서 스타일 정보 추출
        font_size = None
        is_bold = False

        if block.runs:
            first_run = block.runs[0]
            font_size = first_run.font_size
            is_bold = first_run.bold

        return create_paragraph(text, font_size=font_size, is_bold=is_bold)

    def _convert_heading(self, block: Hwp5HeadingBlock):
        """HeadingBlock 변환."""
        text = block.text.strip()
        if not text:
            return None

        return create_heading(block.level, text)

    def _convert_table(
        self, block: Hwp5TableBlock, doc_info: DocInfoParser | None = None,
        image_id_map: dict | None = None,
    ):
        """TableBlock 변환."""
        if not block.rows:
            return None

        col_count = block.col_count
        image_id_map = image_id_map or {}

        # 헤더와 본문 분리
        header_rows = block.thead
        body_rows = block.tbody

        # 행을 col_count 크기로 정규화하는 헬퍼 함수
        def normalize_row(row_cells) -> list[str]:
            """셀들을 원본 컬럼 위치에 맞춰 배열로 변환."""
            result = [""] * col_count
            for cell in row_cells:
                if 0 <= cell.col < col_count:
                    cell_content = cell.text
                    # 셀 내 이미지 추가
                    if hasattr(cell, 'image_bin_ids') and cell.image_bin_ids:
                        for bin_id in cell.image_bin_ids:
                            img_id = image_id_map.get(bin_id)
                            if img_id:
                                # 이미지 태그 추가 (images/ 경로 포함, 스타일 적용)
                                img_tag = f'<img src="images/{img_id}" alt="" style="max-width: 100%;">'
                                if cell_content:
                                    cell_content = f"{cell_content}<br>{img_tag}"
                                else:
                                    cell_content = img_tag
                    result[cell.col] = cell_content
            return result

        # 스타일 추출 헬퍼 함수
        def extract_row_styles(row_cells) -> list[CellStyle]:
            """셀들에서 스타일 정보 추출."""
            styles = [CellStyle()] * col_count
            for cell in row_cells:
                if 0 <= cell.col < col_count:
                    cell_style = self._extract_cell_style(cell, doc_info)
                    styles[cell.col] = cell_style
            return styles

        # 첫 행을 헤더로 (정규화)
        if header_rows:
            header = normalize_row(header_rows[0].cells)
            header_styles = extract_row_styles(header_rows[0].cells)
        elif block.rows:
            header = normalize_row(block.rows[0].cells)
            header_styles = extract_row_styles(block.rows[0].cells)
            body_rows = block.rows[1:] if len(block.rows) > 1 else []
        else:
            return None

        # 본문 행들 (정규화)
        rows = []
        rows_styles: list[list[CellStyle]] = [header_styles]  # 헤더 스타일 포함
        for row in body_rows:
            rows.append(normalize_row(row.cells))
            rows_styles.append(extract_row_styles(row.cells))

        # 병합 정보 - cell.col을 사용하여 원본 컬럼 위치 유지
        merge_info = []
        for row_idx, row in enumerate(block.rows):
            for cell in row.cells:
                if cell.colspan > 1 or cell.rowspan > 1:
                    merge_info.append(
                        MergeInfo(
                            row=row_idx,
                            col=cell.col,  # 원본 컬럼 위치 사용
                            rowspan=cell.rowspan,
                            colspan=cell.colspan,
                        )
                    )

        return create_table(
            header=header,
            rows=rows,
            merge_info=merge_info if merge_info else None,
            cell_styles=rows_styles if rows_styles else None,
        )

    def _convert_image(self, block: Hwp5ImageBlock, image_id_map: dict):
        """ImageBlock 변환."""
        image_id = image_id_map.get(block.bin_id)
        if not image_id:
            return None

        # block.format은 확장자 없는 포맷 문자열 (예: "png")
        ext = f".{block.format.lower()}" if block.format else ".bin"
        mime_type = EXTENSION_TO_MIME.get(ext, "application/octet-stream")
        return create_image(image_id=image_id, mime_type=mime_type)

    def _extract_cell_style(
        self, cell, doc_info: DocInfoParser | None
    ) -> CellStyle:
        """셀에서 스타일 정보 추출.

        Args:
            cell: TableCell (hwp5 block_builder)
            doc_info: DocInfoParser (border_fill, char_shape 정보)

        Returns:
            CellStyle 객체
        """
        background_color = None
        is_bold = False
        font_size = None
        text_align = None

        if doc_info is None:
            return CellStyle()

        # border_fill_id로 배경색 조회
        if hasattr(cell, "border_fill_id") and cell.border_fill_id > 0:
            border_fill = doc_info.get_border_fill(cell.border_fill_id)
            if border_fill and border_fill.background_color_hex:
                background_color = border_fill.background_color_hex

        # first_char_shape_id로 볼드/폰트 크기 조회 (0도 유효한 ID)
        if hasattr(cell, "first_char_shape_id") and cell.first_char_shape_id >= 0:
            char_shape = doc_info.get_char_shape(cell.first_char_shape_id)
            if char_shape:
                is_bold = char_shape.bold
                font_size = char_shape.font_size

        return CellStyle(
            background_color=background_color,
            is_bold=is_bold,
            font_size=font_size,
            text_align=text_align,
        )

    def _convert_images(self, images: dict, image_id_map: dict) -> list[ImageData]:
        """이미지 데이터를 base_parser.ImageData로 변환."""
        result = []

        for bin_id, img in sorted(images.items()):
            image_id = image_id_map.get(bin_id)
            if not image_id:
                continue

            ext = img.extension.lower()
            mime_type = EXTENSION_TO_MIME.get(ext, "application/octet-stream")
            result.append(
                ImageData(
                    image_id=image_id,
                    data=img.data,
                    mime_type=mime_type,
                )
            )

        return result

    # ========== 레거시 호환 메서드들 ==========

    def _looks_like_heading(
        self, text: str, font_size: float = 10.0, is_bold: bool = False
    ) -> bool:
        """heading처럼 보이는지 판별 (레거시 호환).

        Args:
            text: 텍스트 내용
            font_size: 폰트 크기 (pt)
            is_bold: bold 여부
        """
        if not text:
            return False

        # 서로게이트 문자 필터링
        if any(ord(c) > 0xFFFF or (0xD800 <= ord(c) <= 0xDFFF) for c in text):
            return False

        words = text.split()
        if len(words) > 10:
            return False

        # 폰트 크기가 크면 heading (14pt 이상)
        if font_size >= 14:
            return True

        # Bold이고 짧은 텍스트면 heading
        if is_bold and len(text) < 100:
            if len(words) <= 10:
                return True

        # 숫자. 로 시작 (예: "1. 서론")
        if text[0].isdigit() and "." in text[:5]:
            return True

        # 전부 대문자 (영문 제목)
        if text.isupper() and len(text) > 3 and text.isascii():
            return True

        # Bold + 마침표 없음 = heading
        if is_bold and "." not in text:
            return True

        return False

    def _determine_heading_level(
        self, text: str, font_size: float = 10.0, is_bold: bool = False
    ) -> int:
        """heading 레벨 결정 (레거시 호환).

        Args:
            text: 텍스트 내용
            font_size: 폰트 크기 (pt)
            is_bold: bold 여부
        """
        # 폰트 크기 기반 레벨 결정
        if font_size >= 18:
            return 1
        if font_size >= 14:
            return 2

        # 번호 패턴으로 레벨 결정
        if text and text[0].isdigit():
            num_part = ""
            for ch in text:
                if ch.isdigit() or ch == ".":
                    num_part += ch
                else:
                    break

            segments = [s for s in num_part.split(".") if s]

            if len(segments) >= 3:
                return 3
            elif len(segments) == 2:
                return 2
            elif len(segments) == 1:
                return 1

        # Bold이면 h2
        if is_bold:
            return 2

        return 2

    def _decode_para_text(self, data: bytes) -> str:
        """문단 텍스트 디코딩 (레거시 호환).

        HWP 제어 코드:
        - 0: null
        - 1-8: 확장 제어 (12바이트 추가 데이터)
        - 9: 탭
        - 10: 줄바꿈
        - 11-23: 단순 제어 (건너뛰기)
        - 24-31: 확장 제어 (12바이트 추가 데이터)
        """
        try:
            text = []
            i = 0

            while i < len(data) - 1:
                char_code = struct.unpack("<H", data[i : i + 2])[0]
                i += 2

                # 제어 코드 처리
                if char_code < 32:
                    if char_code == 0:
                        continue
                    elif char_code == 10:
                        text.append("\n")
                    elif char_code == 13:
                        continue
                    elif char_code in [1, 2, 3, 4, 5, 6, 7, 8]:
                        i += 12
                    elif char_code in [9, 11, 12, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23]:
                        continue
                    elif 24 <= char_code <= 30:
                        i += 12
                    elif char_code == 31:
                        text.append(" ")
                    continue

                # 유효한 문자
                if 0x20 <= char_code <= 0x7E:
                    text.append(chr(char_code))
                elif 0xAC00 <= char_code <= 0xD7A3:  # 한글 음절
                    text.append(chr(char_code))
                elif 0x4E00 <= char_code <= 0x9FFF:  # CJK 한자
                    text.append(chr(char_code))
                elif 0x3000 <= char_code <= 0x303F:  # CJK 기호
                    text.append(chr(char_code))
                elif 0x1100 <= char_code <= 0x11FF:  # 한글 자모
                    text.append(chr(char_code))
                elif 0x3130 <= char_code <= 0x318F:  # 한글 호환 자모
                    text.append(chr(char_code))

            return "".join(text).strip()

        except (struct.error, IndexError):
            return ""

    def _create_block_from_text(
        self, text: str, char_shape: CharShapeInfo | None = None
    ):
        """텍스트로부터 적절한 블록 생성 (레거시 호환).

        Args:
            text: 텍스트 내용
            char_shape: 글자 모양 정보
        """
        text = text.strip()
        if not text:
            return None

        if len(text) <= 2:
            has_meaningful = any(c.isalnum() or "\uac00" <= c <= "\ud7a3" for c in text)
            if not has_meaningful:
                return None

        # CharShape 정보 추출
        font_size = char_shape.font_size if char_shape else 10.0
        is_bold = char_shape.is_bold if char_shape else False

        if len(text) < 100 and self._looks_like_heading(text, font_size, is_bold):
            level = self._determine_heading_level(text, font_size, is_bold)
            return create_heading(level, text)

        return create_paragraph(text)
