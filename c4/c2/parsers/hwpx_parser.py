"""HWPX Parser - ZIP + XML 기반 한글 문서 파서."""

import logging
import zipfile
from io import BytesIO
from pathlib import Path
from xml.etree import ElementTree as ET

from c4.c2.parsers.base import BaseParser, ImageData, ParseResult
from c4.c2.parsers.ir_models import (
    CellStyle,
    Document,
    MergeInfo,
    create_heading,
    create_image,
    create_paragraph,
    create_table,
)
from c4.c2.parsers.utils.chart_parser import parse_chart_xml
from c4.c2.parsers.utils.image import generate_image_id

logger = logging.getLogger(__name__)

# PNG/JPEG 시그니처
PNG_SIGNATURE = bytes([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A])
PNG_IEND = b"\x49\x45\x4e\x44\xae\x42\x60\x82"
JPEG_SIGNATURE = bytes([0xFF, 0xD8, 0xFF])

# HWPX XML 네임스페이스
NAMESPACES = {
    "hp": "http://www.hancom.co.kr/hwpml/2011/paragraph",
    "hs": "http://www.hancom.co.kr/hwpml/2011/section",
    "hh": "http://www.hancom.co.kr/hwpml/2011/head",
    "hc": "http://www.hancom.co.kr/hwpml/2011/core",
}


class HwpxParser(BaseParser):
    """HWPX 문서 파서.

    HWPX는 OOXML 형식(ZIP + XML)의 한글 문서입니다.

    파싱 규칙:
    - hp:p → paragraph (문단)
    - hp:t → text (텍스트)
    - hp:tbl → table (테이블)
    - hp:tc → table cell (셀)
    - 첫 번째 문단 → h1 (제목)
    - 짧고 독립적인 문단 → h2 후보
    """

    @property
    def supported_extensions(self) -> list[str]:
        return [".hwpx"]

    def parse(self, file_path: Path) -> Document:
        """HWPX 파일을 IR로 변환 (이미지 제외)."""
        result = self.parse_with_images(file_path)
        return result.document

    def parse_with_images(self, file_path: Path) -> ParseResult:
        """HWPX 파일을 IR과 이미지로 변환."""
        blocks = []
        extracted_images: list[ImageData] = []
        image_index = 0
        first_text_found = False

        with zipfile.ZipFile(file_path, "r") as zf:
            # header.xml에서 스타일 정보 파싱
            border_fill_map, char_pr_map = self._parse_header_styles(zf)

            # 섹션 파일들 찾기 (section0.xml, section1.xml, ...)
            section_files = sorted(
                [f for f in zf.namelist() if f.startswith("Contents/section") and f.endswith(".xml")]
            )

            # 이미지 추출
            extracted_images, image_index = self._extract_images(zf)

            for section_file in section_files:
                section_content = zf.read(section_file).decode("utf-8")
                section_blocks, first_text_found = self._parse_section(
                    section_content, first_text_found, border_fill_map, char_pr_map
                )
                blocks.extend(section_blocks)

            # 차트 XML 파싱 → 테이블로 변환
            chart_blocks = self._parse_charts(zf)
            blocks.extend(chart_blocks)

        # 추출된 이미지들을 문서 블록에 추가 (차트 이미지 포함)
        for img in extracted_images:
            blocks.append(create_image(img.image_id, img.mime_type))

        return ParseResult(document=Document(blocks=blocks), images=extracted_images)

    def _extract_images(self, zf: zipfile.ZipFile) -> tuple[list[ImageData], int]:
        """ZIP 파일에서 이미지 추출 (일반 이미지 + OLE 차트)."""
        images = []
        image_index = 0

        for file_name in zf.namelist():
            # BinData 폴더 내 이미지 파일
            if file_name.startswith("BinData/") or file_name.startswith("Contents/BinData/"):
                ext = Path(file_name).suffix.lower()
                mime_types = {
                    ".jpg": "image/jpeg",
                    ".jpeg": "image/jpeg",
                    ".png": "image/png",
                    ".gif": "image/gif",
                    ".bmp": "image/bmp",
                }
                if ext in mime_types:
                    try:
                        image_data = zf.read(file_name)
                        mime_type = mime_types[ext]
                        image_id = generate_image_id(image_data, image_index)
                        image_id_with_ext = f"{image_id}{ext}"

                        images.append(
                            ImageData(
                                image_id=image_id_with_ext,
                                data=image_data,
                                mime_type=mime_type,
                            )
                        )
                        image_index += 1
                    except Exception as e:
                        logger.debug("Failed to extract image %s: %s", file_name, e)

        # OLE 파일에서 차트 이미지 추출
        ole_images, image_index = self._extract_ole_chart_images(zf, image_index)
        images.extend(ole_images)

        return images, image_index

    def _parse_header_styles(
        self, zf: zipfile.ZipFile
    ) -> tuple[dict[int, str | None], dict[int, tuple[bool, float | None]]]:
        """header.xml에서 borderFill 및 charPr 스타일 정보 파싱.

        Returns:
            border_fill_map: borderFill ID → 배경색 ("#RRGGBB" 또는 None)
            char_pr_map: charPr ID → (is_bold, font_size)
        """
        border_fill_map: dict[int, str | None] = {}
        char_pr_map: dict[int, tuple[bool, float | None]] = {}

        # header.xml 찾기 (Contents/header.xml 또는 header.xml)
        header_path = None
        for path in ["Contents/header.xml", "header.xml"]:
            if path in zf.namelist():
                header_path = path
                break

        if not header_path:
            return border_fill_map, char_pr_map

        try:
            header_content = zf.read(header_path).decode("utf-8")
            root = ET.fromstring(header_content)

            # borderFills 파싱
            for border_fill in root.findall(".//hh:borderFill", NAMESPACES):
                bf_id_str = border_fill.get("id")
                if not bf_id_str:
                    continue

                bf_id = int(bf_id_str)
                background_color = None

                # fillBrush/winBrush에서 faceColor 추출
                win_brush = border_fill.find(".//hc:winBrush", NAMESPACES)
                if win_brush is not None:
                    face_color = win_brush.get("faceColor")
                    # "none"이 아니고 색상값이 있으면 저장
                    if face_color and face_color.lower() != "none":
                        # 색상이 이미 #으로 시작하면 그대로, 아니면 # 추가
                        if face_color.startswith("#"):
                            background_color = face_color.upper()
                        else:
                            background_color = f"#{face_color}".upper()

                border_fill_map[bf_id] = background_color

            # charProperties 파싱
            for char_pr in root.findall(".//hh:charPr", NAMESPACES):
                cp_id_str = char_pr.get("id")
                if not cp_id_str:
                    continue

                cp_id = int(cp_id_str)
                is_bold = char_pr.find("hh:bold", NAMESPACES) is not None

                # height 속성에서 폰트 크기 추출 (단위: 1/100 pt)
                font_size = None
                height_str = char_pr.get("height")
                if height_str:
                    try:
                        # HWP는 1/100pt 단위 사용
                        font_size = float(height_str) / 100.0
                    except ValueError:
                        pass

                char_pr_map[cp_id] = (is_bold, font_size)

        except Exception as e:
            logger.debug("Failed to parse header styles: %s", e)

        return border_fill_map, char_pr_map

    def _parse_section(
        self,
        xml_content: str,
        first_text_found: bool,
        border_fill_map: dict[int, str | None] | None = None,
        char_pr_map: dict[int, tuple[bool, float | None]] | None = None,
    ) -> tuple[list, bool]:
        """섹션 XML 파싱.

        중요: 테이블 내부 문단은 별도로 처리하지 않음 (테이블 파싱 시 처리됨)
        순서: 문단 순서대로 처리하되, 테이블 포함 문단에서는 테이블 추출
        중첩 테이블은 별도 블록으로 추출하지 않음 (부모 테이블 셀에 포함)
        """
        blocks = []

        try:
            root = ET.fromstring(xml_content)
        except ET.ParseError:
            return blocks, first_text_found

        # 테이블 찾기
        all_tables = root.findall(".//hp:tbl", NAMESPACES)

        # 중첩 테이블 ID 수집 (다른 테이블 안에 있는 테이블)
        nested_table_ids = set()
        for table in all_tables:
            for inner_table in table.findall(".//hp:tbl", NAMESPACES):
                nested_table_ids.add(id(inner_table))

        # 테이블 내부 문단들의 ID 수집 (중복 추출 방지)
        table_para_ids = set()
        for table in all_tables:
            for para in table.findall(".//hp:p", NAMESPACES):
                table_para_ids.add(id(para))

        # 처리된 테이블 추적
        processed_table_ids = set()

        # 모든 문단(hp:p) 찾기
        paragraphs = root.findall(".//hp:p", NAMESPACES)

        for para in paragraphs:
            # 테이블 내부 문단은 건너뜀 (테이블 파싱 시 처리됨)
            if id(para) in table_para_ids:
                continue

            # 테이블 포함 문단 처리 - 최상위 테이블만 추출 (중첩 테이블 제외)
            tables_in_para = para.findall(".//hp:tbl", NAMESPACES)
            if tables_in_para:
                for table in tables_in_para:
                    # 중첩 테이블은 건너뜀 (부모 테이블 파싱 시 처리)
                    if id(table) in nested_table_ids:
                        continue
                    if id(table) not in processed_table_ids:
                        table_block = self._parse_table(
                            table, border_fill_map, char_pr_map
                        )
                        if table_block:
                            blocks.append(table_block)
                        processed_table_ids.add(id(table))
                continue

            # 문단 텍스트 추출
            text = self._extract_paragraph_text(para)
            if text:
                if not first_text_found:
                    # 첫 번째 텍스트는 제목(h1)으로
                    blocks.append(create_heading(1, text))
                    first_text_found = True
                elif self._is_heading_candidate(text, para):
                    blocks.append(create_heading(2, text))
                else:
                    blocks.append(create_paragraph(text))

        return blocks, first_text_found

    def _extract_paragraph_text(self, para) -> str:
        """문단에서 텍스트 추출."""
        texts = []

        # hp:run/hp:t 에서 텍스트 추출
        for run in para.findall(".//hp:run", NAMESPACES):
            for t in run.findall("hp:t", NAMESPACES):
                if t.text:
                    texts.append(t.text)

        return "".join(texts).strip()

    def _is_heading_candidate(self, text: str, para) -> bool:
        """heading 후보인지 판단."""
        # 짧고 줄바꿈 없으면 heading 후보
        if len(text) < 50 and "\n" not in text:
            # 숫자로 시작하거나 특수 패턴이면 heading
            if text and (text[0].isdigit() or text.startswith("제") or text.startswith("Ⅰ")):
                return True
        return False

    def _parse_table(
        self,
        table_elem,
        border_fill_map: dict[int, str | None] | None = None,
        char_pr_map: dict[int, tuple[bool, float | None]] | None = None,
    ) -> dict | None:
        """테이블 요소 파싱.

        HWPX 테이블에서 rowspan이 있으면 해당 셀은 아래 행들의 컬럼을 차지합니다.
        따라서 각 행에서 실제 컬럼 위치를 계산해야 합니다.

        중첩 테이블이 있는 경우, 중첩 테이블의 행은 제외합니다.

        스타일 추출:
        - 셀의 borderFillIDRef → 배경색
        - run의 charPrIDRef → bold, font_size
        """
        # 중첩 테이블 찾기 (현재 테이블 내부의 테이블)
        nested_tables = table_elem.findall(".//hp:tbl", NAMESPACES)
        nested_tr_ids = set()
        for nested_tbl in nested_tables:
            for nested_tr in nested_tbl.findall(".//hp:tr", NAMESPACES):
                nested_tr_ids.add(id(nested_tr))

        # hp:tr (테이블 행) 찾기 - 중첩 테이블의 행 제외
        all_trs = table_elem.findall(".//hp:tr", NAMESPACES)
        trs = [tr for tr in all_trs if id(tr) not in nested_tr_ids]
        if not trs:
            return None

        # 1단계: 모든 셀 정보 수집 (스타일 포함)
        raw_rows = []
        for tr in trs:
            tcs = tr.findall("hp:tc", NAMESPACES)
            row_cells = []
            for tc in tcs:
                cell_text = self._extract_cell_text(tc)
                cell_span = tc.find("hp:cellSpan", NAMESPACES)
                colspan = int(cell_span.get("colSpan", 1)) if cell_span is not None else 1
                rowspan = int(cell_span.get("rowSpan", 1)) if cell_span is not None else 1

                # 스타일 정보 추출
                cell_style = self._extract_cell_style(
                    tc, border_fill_map, char_pr_map
                )

                row_cells.append({
                    "text": cell_text,
                    "colspan": colspan,
                    "rowspan": rowspan,
                    "style": cell_style,
                })
            raw_rows.append(row_cells)

        if not raw_rows:
            return None

        # 2단계: 최대 컬럼 수 계산 (첫 행 또는 가장 많은 셀이 있는 행 기준)
        max_cols = 0
        for row in raw_rows:
            col_count = sum(cell["colspan"] for cell in row)
            if col_count > max_cols:
                max_cols = col_count

        if max_cols == 0:
            return None

        # 3단계: 그리드 생성 (rowspan으로 점유된 셀 추적)
        num_rows = len(raw_rows)
        # occupied[row][col] = True if cell is occupied by rowspan from above
        occupied = [[False] * max_cols for _ in range(num_rows)]

        rows_data = []
        rows_styles: list[list[CellStyle]] = []
        merge_info = []

        # 기본 스타일 (빈 셀용)
        default_style = CellStyle()

        for row_idx, row_cells in enumerate(raw_rows):
            row_data = [""] * max_cols
            row_style = [default_style] * max_cols
            cell_idx = 0  # 현재 처리 중인 셀 인덱스

            for col_idx in range(max_cols):
                # 이미 위 행의 rowspan으로 점유된 컬럼이면 건너뜀
                if occupied[row_idx][col_idx]:
                    continue

                # 현재 행에서 다음 셀 가져오기
                if cell_idx >= len(row_cells):
                    break

                cell = row_cells[cell_idx]
                cell_idx += 1

                # 셀 텍스트 배치
                row_data[col_idx] = cell["text"]
                row_style[col_idx] = cell["style"]

                # 병합 정보 기록
                colspan = cell["colspan"]
                rowspan = cell["rowspan"]

                if colspan > 1 or rowspan > 1:
                    merge_info.append(
                        MergeInfo(
                            row=row_idx,
                            col=col_idx,
                            rowspan=rowspan,
                            colspan=colspan,
                        )
                    )

                # rowspan으로 아래 행들 점유 표시
                if rowspan > 1:
                    for r in range(row_idx + 1, min(row_idx + rowspan, num_rows)):
                        for c in range(col_idx, min(col_idx + colspan, max_cols)):
                            occupied[r][c] = True

                # colspan으로 현재 행의 다음 컬럼들 점유 표시
                if colspan > 1:
                    for c in range(col_idx + 1, min(col_idx + colspan, max_cols)):
                        occupied[row_idx][c] = True

            rows_data.append(row_data)
            rows_styles.append(row_style)

        # header_rows 자동 계산: 첫 행의 rowspan 최대값
        header_rows = 1
        for m in merge_info:
            if m.row == 0 and m.rowspan > header_rows:
                header_rows = m.rowspan

        # 첫 행은 header
        header = rows_data[0] if rows_data else []
        body_rows = rows_data[1:] if len(rows_data) > 1 else []

        return create_table(
            header=header,
            rows=body_rows,
            merge_info=merge_info if merge_info else None,
            header_rows=header_rows,
            cell_styles=rows_styles if rows_styles else None,
        )

    def _extract_cell_text(self, tc) -> str:
        """테이블 셀에서 텍스트 및 중첩 테이블 HTML 추출.

        중첩 테이블이 있는 경우 HTML로 렌더링하여 포함합니다.
        직접 자식 subList/p 만 처리하고, 각 문단은 줄바꿈으로 구분됩니다.
        """
        content_parts = []  # 텍스트 또는 HTML 조각들

        # 직접 자식 hp:subList만 처리 (중첩 제외)
        for sublist in tc.findall("hp:subList", NAMESPACES):
            # 직접 자식 hp:p만 처리
            for para in sublist.findall("hp:p", NAMESPACES):
                # 이 문단에 포함된 테이블 찾기 (run 내부 포함)
                tables_in_para = para.findall(".//hp:tbl", NAMESPACES)

                if tables_in_para:
                    # 테이블 앞의 텍스트 추출
                    pre_table_texts = []
                    table_run_ids = set()
                    for tbl in tables_in_para:
                        for run in tbl.findall(".//hp:run", NAMESPACES):
                            table_run_ids.add(id(run))

                    for run in para.findall(".//hp:run", NAMESPACES):
                        if id(run) in table_run_ids:
                            continue
                        for t in run.findall("hp:t", NAMESPACES):
                            if t.text:
                                pre_table_texts.append(t.text)

                    pre_text = "".join(pre_table_texts).strip()
                    if pre_text:
                        content_parts.append(pre_text)

                    # 중첩 테이블 HTML로 렌더링
                    for nested_tbl in tables_in_para:
                        nested_html = self._render_nested_table_html(nested_tbl)
                        if nested_html:
                            content_parts.append(nested_html)
                else:
                    # 테이블 없으면 모든 run 처리
                    run_texts = []
                    for run in para.findall(".//hp:run", NAMESPACES):
                        for t in run.findall("hp:t", NAMESPACES):
                            if t.text:
                                run_texts.append(t.text)

                    para_text = "".join(run_texts).strip()
                    if para_text:
                        content_parts.append(para_text)

        # 내용들을 줄바꿈으로 연결
        return "\n".join(content_parts)

    def _extract_cell_style(
        self,
        tc,
        border_fill_map: dict[int, str | None] | None,
        char_pr_map: dict[int, tuple[bool, float | None]] | None,
    ) -> CellStyle:
        """셀에서 스타일 정보 추출.

        Args:
            tc: 테이블 셀 요소
            border_fill_map: borderFill ID → 배경색 매핑
            char_pr_map: charPr ID → (is_bold, font_size) 매핑

        Returns:
            CellStyle 객체
        """
        background_color = None
        is_bold = False
        font_size = None
        text_align = None

        # 1. 셀의 borderFillIDRef에서 배경색 추출
        border_fill_id_str = tc.get("borderFillIDRef")
        if border_fill_id_str and border_fill_map:
            try:
                border_fill_id = int(border_fill_id_str)
                if border_fill_id in border_fill_map:
                    background_color = border_fill_map[border_fill_id]
            except ValueError:
                pass

        # 2. 셀 내 run들에서 charPrIDRef로 볼드/폰트크기 추출
        # 첫 번째 run의 스타일을 셀 대표 스타일로 사용
        for sublist in tc.findall("hp:subList", NAMESPACES):
            for para in sublist.findall("hp:p", NAMESPACES):
                for run in para.findall(".//hp:run", NAMESPACES):
                    char_pr_id_str = run.get("charPrIDRef")
                    if char_pr_id_str and char_pr_map:
                        try:
                            char_pr_id = int(char_pr_id_str)
                            if char_pr_id in char_pr_map:
                                run_bold, run_font_size = char_pr_map[char_pr_id]
                                is_bold = run_bold
                                font_size = run_font_size
                                # 첫 run만 사용
                                break
                        except ValueError:
                            pass
                if is_bold or font_size:
                    break
            if is_bold or font_size:
                break

        # 3. subList의 vertAlign에서 정렬 추출
        for sublist in tc.findall("hp:subList", NAMESPACES):
            vert_align = sublist.get("vertAlign")
            if vert_align:
                # HWPX의 vertAlign을 CSS text-align에 매핑
                align_map = {
                    "CENTER": "center",
                    "LEFT": "left",
                    "RIGHT": "right",
                    "JUSTIFY": "justify",
                }
                text_align = align_map.get(vert_align.upper())
            break

        return CellStyle(
            background_color=background_color,
            is_bold=is_bold,
            font_size=font_size,
            text_align=text_align,
        )

    def _render_nested_table_html(self, table_elem) -> str:
        """중첩 테이블을 HTML로 렌더링.

        이 테이블 안에 또 다른 중첩 테이블이 있을 수 있으므로,
        해당 테이블의 행은 제외합니다.
        """
        from html import escape

        # 이 테이블 안의 중첩 테이블 찾기
        nested_tables = table_elem.findall(".//hp:tbl", NAMESPACES)
        nested_tr_ids = set()
        for nested_tbl in nested_tables:
            for nested_tr in nested_tbl.findall(".//hp:tr", NAMESPACES):
                nested_tr_ids.add(id(nested_tr))

        # 중첩 테이블의 행 제외
        all_trs = table_elem.findall(".//hp:tr", NAMESPACES)
        trs = [tr for tr in all_trs if id(tr) not in nested_tr_ids]
        if not trs:
            return ""

        html_lines = ['<table class="nested-table" style="border-collapse: collapse; width: 100%; margin: 8px 0;">']

        for row_idx, tr in enumerate(trs):
            html_lines.append("  <tr>")
            tcs = tr.findall("hp:tc", NAMESPACES)

            for tc in tcs:
                # 셀 span 정보
                cell_span = tc.find("hp:cellSpan", NAMESPACES)
                colspan = int(cell_span.get("colSpan", 1)) if cell_span is not None else 1
                rowspan = int(cell_span.get("rowSpan", 1)) if cell_span is not None else 1

                # 셀 텍스트 (재귀적으로 중첩 테이블 처리하지 않고 텍스트만)
                cell_text = self._extract_simple_cell_text(tc)
                escaped_text = escape(cell_text).replace("\n", "<br>")

                # span 속성
                attrs = 'style="border: 1px solid #ccc; padding: 4px;"'
                if colspan > 1:
                    attrs += f' colspan="{colspan}"'
                if rowspan > 1:
                    attrs += f' rowspan="{rowspan}"'

                # 첫 행은 헤더로 처리
                tag = "th" if row_idx == 0 else "td"
                if tag == "th":
                    attrs += ' style="border: 1px solid #ccc; padding: 4px; background: #f5f5f5; font-weight: bold;"'

                html_lines.append(f"    <{tag} {attrs}>{escaped_text}</{tag}>")

            html_lines.append("  </tr>")

        html_lines.append("</table>")
        return "\n".join(html_lines)

    def _extract_simple_cell_text(self, tc) -> str:
        """셀에서 텍스트만 추출 (중첩 테이블 무시)."""
        para_texts = []

        for sublist in tc.findall("hp:subList", NAMESPACES):
            for para in sublist.findall("hp:p", NAMESPACES):
                run_texts = []
                # 중첩 테이블 내 run 제외
                tables_in_para = para.findall(".//hp:tbl", NAMESPACES)
                table_run_ids = set()
                for tbl in tables_in_para:
                    for run in tbl.findall(".//hp:run", NAMESPACES):
                        table_run_ids.add(id(run))

                for run in para.findall(".//hp:run", NAMESPACES):
                    if id(run) in table_run_ids:
                        continue
                    for t in run.findall("hp:t", NAMESPACES):
                        if t.text:
                            run_texts.append(t.text)

                para_text = "".join(run_texts).strip()
                if para_text:
                    para_texts.append(para_text)

        return "\n".join(para_texts)

    def _extract_ole_chart_images(
        self, zf: zipfile.ZipFile, start_index: int
    ) -> tuple[list[ImageData], int]:
        """OLE 파일에서 차트 이미지 추출.

        HWPX 차트는 BinData/oleX.ole 형태로 저장되며,
        OLE compound document 내 OlePres000 스트림에 PNG가 포함됨.
        """
        images = []
        image_index = start_index

        for file_name in zf.namelist():
            # BinData 폴더 내 OLE 파일 찾기
            if not (file_name.startswith("BinData/") or file_name.startswith("Contents/BinData/")):
                continue
            if not file_name.lower().endswith(".ole"):
                continue

            try:
                ole_data = zf.read(file_name)
                image_data = self._extract_image_from_ole(ole_data)
                if image_data:
                    # 이미지 타입 감지
                    if image_data.startswith(PNG_SIGNATURE):
                        ext = ".png"
                        mime_type = "image/png"
                    elif image_data.startswith(JPEG_SIGNATURE):
                        ext = ".jpg"
                        mime_type = "image/jpeg"
                    else:
                        continue

                    image_id = generate_image_id(image_data, image_index)
                    image_id_with_ext = f"{image_id}{ext}"

                    images.append(
                        ImageData(
                            image_id=image_id_with_ext,
                            data=image_data,
                            mime_type=mime_type,
                        )
                    )
                    image_index += 1
            except Exception as e:
                logger.debug("Failed to extract OLE chart image %s: %s", file_name, e)

        return images, image_index

    def _extract_image_from_ole(self, ole_data: bytes) -> bytes | None:
        """OLE compound document에서 이미지 추출.

        HWP OLE 형식: 4바이트 헤더 + 표준 OLE compound document
        OlePres000 스트림에 프레젠테이션 데이터(PNG 등)가 저장됨.
        """
        try:
            import olefile
        except ImportError:
            # olefile 없으면 직접 PNG 시그니처 검색
            return self._extract_image_by_signature(ole_data)

        # HWP OLE 헤더 확인 (4바이트 건너뛰기)
        ole_signature = bytes([0xD0, 0xCF, 0x11, 0xE0])
        ole_start = ole_data.find(ole_signature)
        if ole_start == -1:
            return self._extract_image_by_signature(ole_data)

        try:
            ole = olefile.OleFileIO(BytesIO(ole_data[ole_start:]))
            try:
                # OlePres000 스트림에서 이미지 찾기
                for entry in ole.listdir():
                    stream_name = "/".join(entry)
                    if "OlePres" in stream_name:
                        stream_data = ole.openstream(entry).read()
                        # 스트림 내에서 PNG/JPEG 찾기
                        image = self._extract_image_by_signature(stream_data)
                        if image:
                            return image
            finally:
                ole.close()
        except Exception as e:
            logger.debug("Failed to parse OLE compound document: %s", e)

        # fallback: 직접 시그니처 검색
        return self._extract_image_by_signature(ole_data)

    def _extract_image_by_signature(self, data: bytes) -> bytes | None:
        """바이너리 데이터에서 PNG/JPEG 시그니처로 이미지 추출."""
        # PNG 찾기
        png_start = data.find(PNG_SIGNATURE)
        if png_start != -1:
            png_end = data.find(PNG_IEND, png_start)
            if png_end != -1:
                return data[png_start : png_end + len(PNG_IEND)]

        # JPEG 찾기
        jpeg_start = data.find(JPEG_SIGNATURE)
        if jpeg_start != -1:
            # JPEG 끝 마커 (FFD9) 찾기
            jpeg_end = data.find(b"\xFF\xD9", jpeg_start)
            if jpeg_end != -1:
                return data[jpeg_start : jpeg_end + 2]

        return None

    def _parse_charts(self, zf: zipfile.ZipFile) -> list:
        """Chart XML 파일들을 파싱하여 테이블 블록으로 변환."""
        blocks = []

        # Chart 폴더 내 XML 파일 찾기
        chart_files = sorted(
            [f for f in zf.namelist() if f.startswith("Chart/") and f.endswith(".xml")]
        )

        for chart_file in chart_files:
            try:
                chart_content = zf.read(chart_file).decode("utf-8")
                chart_block = parse_chart_xml(chart_content)
                if chart_block:
                    blocks.append(chart_block)
            except Exception as e:
                logger.debug("Failed to parse chart %s: %s", chart_file, e)

        return blocks
