"""PDF Parser - PyMuPDF(fitz) 기반 + pdfplumber 테이블 추출."""

from pathlib import Path

import fitz  # PyMuPDF
import pdfplumber

from c4.c2.parsers.base import BaseParser, ImageData, ParseResult
from c4.c2.parsers.ir_models import (
    Document,
    create_heading,
    create_image,
    create_list,
    create_paragraph,
    create_table,
)
from c4.c2.parsers.utils.image import generate_image_id, get_mime_from_extension


class PdfParser(BaseParser):
    """PDF 문서 파서.

    파싱 전략:
    - PyMuPDF: 텍스트 라인 추출 → 문단 군집화 → 요소 분류
    - pdfplumber: 테이블 추출 전용 (multi-strategy)
    - 페이지 중앙 X축 기준 좌/우 컬럼 분리
    - 리스트 아이템 자동 감지 및 그룹화
    """

    # 문단 군집화 설정
    LINE_GAP_THRESHOLD = 1.5  # 줄 간격 배수 (이 이상이면 새 문단)
    FONT_SIZE_TOLERANCE = 1.0  # 폰트 크기 허용 오차 (pt)

    # 테이블 추출 설정 - 여러 전략 순차 시도
    TABLE_SETTINGS_STRATEGIES = [
        # Strategy 1: 기본 설정 (선이 있는 테이블)
        {},
        # Strategy 2: 인접 선 병합 (두꺼운 테두리 또는 그라데이션 선)
        # - 정부 PDF 등에서 작은 선 수천 개로 테두리를 그리는 경우 대응
        {
            "snap_tolerance": 8,
            "join_tolerance": 8,
            "intersection_tolerance": 8,
        },
        # Strategy 3: 텍스트 기반 (borderless 테이블)
        {
            "vertical_strategy": "text",
            "horizontal_strategy": "text",
            "snap_tolerance": 5,
            "join_tolerance": 5,
            "min_words_vertical": 2,
            "min_words_horizontal": 1,
        },
        # Strategy 4: 명시적 라인 + 텍스트 혼합
        {
            "vertical_strategy": "lines_strict",
            "horizontal_strategy": "text",
            "snap_tolerance": 3,
        },
    ]

    @property
    def supported_extensions(self) -> list[str]:
        return [".pdf"]

    def parse(self, file_path: Path) -> Document:
        """PDF 파일을 IR로 변환."""
        result = self.parse_with_images(file_path)
        return result.document

    def parse_with_images(self, file_path: Path) -> ParseResult:
        """PDF 파일을 IR과 이미지로 변환."""
        all_elements = []  # [(page_idx, y_pos, element_type, block/image_data), ...]
        extracted_images: list[ImageData] = []
        image_index = 0

        # 1. pdfplumber로 테이블 추출 (multi-strategy)
        table_regions = []  # [(page_idx, bbox, block), ...]
        with pdfplumber.open(str(file_path)) as pdf:
            for page_idx, page in enumerate(pdf.pages):
                page_tables = self._extract_tables_multi_strategy(page)
                for table_bbox, table_data in page_tables:
                    block = self._parse_table(table_data)
                    if block:
                        y_pos = table_bbox[1] if table_bbox else 0
                        table_regions.append((page_idx, table_bbox, block))
                        all_elements.append((page_idx, y_pos, "table", block))

        # 2. PyMuPDF로 텍스트 블록 및 이미지 추출
        doc = fitz.open(str(file_path))
        for page_idx, page in enumerate(doc):
            page_width = page.rect.width

            # 텍스트 블록 추출 (y좌표 포함)
            page_blocks = self._parse_page_with_fitz_positioned(
                page, page_idx, page_width, table_regions
            )
            for y_pos, block in page_blocks:
                all_elements.append((page_idx, y_pos, "text", block))

            # 이미지 추출 (y좌표 포함)
            page_images, image_index = self._extract_page_images_positioned(
                page, image_index
            )
            for y_pos, img_block, img_data in page_images:
                all_elements.append((page_idx, y_pos, "image", img_block))
                extracted_images.append(img_data)

        doc.close()

        # 3. 페이지 순서 → y좌표 순서로 정렬
        all_elements.sort(key=lambda x: (x[0], x[1]))

        # 4. 블록만 추출
        blocks = [elem[3] for elem in all_elements]

        return ParseResult(document=Document(blocks=blocks), images=extracted_images)

    def _extract_page_images_positioned(self, page, image_index: int) -> tuple[list, int]:
        """페이지에서 이미지 추출 (y좌표 포함).

        Returns:
            ([(y_pos, image_block, image_data), ...], 다음 이미지 인덱스)
        """
        results = []

        # 페이지의 이미지 목록 가져오기
        image_list = page.get_images(full=True)

        for img_info in image_list:
            try:
                xref = img_info[0]  # 이미지 참조 번호

                # 이미지 데이터 추출
                base_image = page.parent.extract_image(xref)
                if not base_image:
                    continue

                image_data = base_image["image"]
                image_ext = base_image.get("ext", "png").lower()

                # 확장자로 MIME 타입 결정
                mime_type, ext = get_mime_from_extension(image_ext)

                # 너무 작은 이미지 스킵 (아이콘 등)
                width = base_image.get("width", 0)
                height = base_image.get("height", 0)
                if width < 50 or height < 50:
                    continue

                # 이미지 위치 찾기
                y_pos = self._get_image_position(page, xref)

                # 이미지 ID 생성
                image_id = generate_image_id(image_data, image_index)
                image_id_with_ext = f"{image_id}{ext}"

                # IR 블록 및 데이터 생성
                img_block = create_image(image_id=image_id_with_ext, mime_type=mime_type)
                img_data = ImageData(
                    image_id=image_id_with_ext,
                    data=image_data,
                    mime_type=mime_type,
                )

                results.append((y_pos, img_block, img_data))
                image_index += 1

            except Exception:
                # 개별 이미지 추출 실패 시 스킵
                continue

        return results, image_index

    def _get_image_position(self, page, xref: int) -> float:
        """이미지의 y좌표 찾기."""
        try:
            # 페이지에서 이미지 위치 정보 검색
            for img in page.get_image_info():
                if img.get("xref") == xref:
                    bbox = img.get("bbox", (0, 0, 0, 0))
                    return bbox[1]  # y0
        except Exception:
            pass
        return 0.0

    def _parse_page_with_fitz_positioned(
        self, page, page_idx: int, page_width: float, table_regions: list
    ) -> list:
        """PyMuPDF로 페이지 파싱. 라인 기반 군집화 적용.

        Phase 1: 문단 재군집화
        - PyMuPDF block 대신 라인 단위로 추출
        - 수직 간격 + 폰트 크기 기준으로 문단 분리
        - 같은 문단의 줄들을 하나로 합침

        Phase 2: 리스트 아이템 분류
        - 불릿/번호로 시작하는 연속 항목 감지
        - ListBlock으로 그룹화

        Returns:
            [(y_pos, block), ...]
        """
        # 테이블 영역 bbox (이 페이지에 해당하는 것만)
        table_bboxes = [bbox for (idx, bbox, _) in table_regions if idx == page_idx]

        # 1. 모든 텍스트 라인 수집
        lines = self._extract_text_lines(page, table_bboxes)

        if not lines:
            return []

        # 2. 라인을 문단으로 군집화
        clusters = self._cluster_lines_to_paragraphs(lines)

        # 3. 2-column 처리
        is_two_col = self._is_two_column_from_lines(lines, page_width)

        if is_two_col:
            col_boundary = page_width / 2
            margin = page_width * 0.05

            # 컬럼별로 분리
            left_clusters = [c for c in clusters if c["x0"] < col_boundary - margin]
            right_clusters = [c for c in clusters if c["x0"] >= col_boundary - margin]

            # 왼쪽 먼저, 오른쪽 나중에
            ordered_clusters = sorted(left_clusters, key=lambda c: c["y0"])
            right_sorted = sorted(right_clusters, key=lambda c: c["y0"])

            # 오른쪽 클러스터에 오프셋 추가
            for c in right_sorted:
                c["y0"] += 10000
            ordered_clusters.extend(right_sorted)
        else:
            ordered_clusters = sorted(clusters, key=lambda c: c["y0"])

        # 4. 리스트 아이템 그룹화 + IR 블록 생성
        results = self._convert_clusters_to_blocks(ordered_clusters)

        return results

    def _extract_text_lines(self, page, table_bboxes: list) -> list[dict]:
        """페이지에서 모든 텍스트 라인 추출.

        Returns:
            [{text, bbox, font_size, is_bold, x0, y0, y1, line_height}, ...]
        """
        text_dict = page.get_text("dict", flags=fitz.TEXT_PRESERVE_WHITESPACE)
        lines = []

        for block in text_dict.get("blocks", []):
            if block.get("type") != 0:  # 0 = text block
                continue

            block_bbox = block.get("bbox", (0, 0, 0, 0))

            # 테이블 영역과 겹치면 스킵
            if self._overlaps_any_bbox(block_bbox, table_bboxes):
                continue

            for line in block.get("lines", []):
                line_bbox = line.get("bbox", (0, 0, 0, 0))
                line_text = ""
                max_font_size = 0
                bold_char_count = 0
                total_char_count = 0

                for span in line.get("spans", []):
                    span_text = span.get("text", "")
                    line_text += span_text
                    max_font_size = max(max_font_size, span.get("size", 12))

                    # Bold 감지
                    flags = span.get("flags", 0)
                    is_bold = bool(flags & 16)
                    char_count = len(span_text.strip())
                    total_char_count += char_count
                    if is_bold:
                        bold_char_count += char_count

                line_text = line_text.strip()
                if not line_text:
                    continue

                is_mostly_bold = (
                    bold_char_count > total_char_count * 0.5
                    if total_char_count > 0
                    else False
                )

                lines.append({
                    "text": line_text,
                    "bbox": line_bbox,
                    "font_size": max_font_size,
                    "is_bold": is_mostly_bold,
                    "x0": line_bbox[0],
                    "y0": line_bbox[1],
                    "y1": line_bbox[3],
                    "line_height": line_bbox[3] - line_bbox[1],
                })

        # y좌표로 정렬
        lines.sort(key=lambda ln: (ln["y0"], ln["x0"]))
        return lines

    def _cluster_lines_to_paragraphs(self, lines: list[dict]) -> list[dict]:
        """텍스트 라인들을 문단으로 군집화.

        규칙:
        - 수직 간격이 줄 높이의 LINE_GAP_THRESHOLD배 이상이면 새 문단
        - 폰트 크기가 FONT_SIZE_TOLERANCE 이상 다르면 새 문단
        - 불릿/번호로 시작하면 새 문단 (리스트 아이템)

        Returns:
            [{text, x0, y0, font_size, is_bold, is_list_item, list_marker}, ...]
        """
        if not lines:
            return []

        clusters = []
        current_cluster = None

        for i, line in enumerate(lines):
            # 리스트 아이템 감지
            list_marker = self._detect_list_marker(line["text"])
            is_list_item = list_marker is not None

            # 새 문단 시작 조건
            start_new = False

            if current_cluster is None:
                start_new = True
            else:
                # 수직 간격 체크
                gap = line["y0"] - current_cluster["y1"]
                avg_line_height = (current_cluster["line_height"] + line["line_height"]) / 2
                threshold = avg_line_height * self.LINE_GAP_THRESHOLD

                if gap > threshold:
                    start_new = True

                # 폰트 크기 변화 체크
                if abs(line["font_size"] - current_cluster["font_size"]) > self.FONT_SIZE_TOLERANCE:
                    start_new = True

                # 리스트 아이템은 항상 새 문단
                if is_list_item:
                    start_new = True

                # x좌표가 크게 다르면 새 문단 (들여쓰기 변화)
                if abs(line["x0"] - current_cluster["x0"]) > 20:
                    start_new = True

            if start_new:
                # 이전 클러스터 저장
                if current_cluster:
                    clusters.append(current_cluster)

                # 새 클러스터 시작
                current_cluster = {
                    "text": line["text"],
                    "x0": line["x0"],
                    "y0": line["y0"],
                    "y1": line["y1"],
                    "font_size": line["font_size"],
                    "is_bold": line["is_bold"],
                    "line_height": line["line_height"],
                    "is_list_item": is_list_item,
                    "list_marker": list_marker,
                    "line_count": 1,
                }
            else:
                # 현재 클러스터에 라인 추가
                current_cluster["text"] += " " + line["text"]
                current_cluster["y1"] = line["y1"]
                current_cluster["line_count"] += 1
                # Bold는 과반수 기준 유지
                if line["is_bold"]:
                    current_cluster["is_bold"] = True

        # 마지막 클러스터 저장
        if current_cluster:
            clusters.append(current_cluster)

        return clusters

    def _detect_list_marker(self, text: str) -> str | None:
        """텍스트가 리스트 마커로 시작하는지 감지.

        Returns:
            마커 타입 ("bullet", "number", "letter") 또는 None
        """
        import re

        text = text.strip()
        if not text:
            return None

        # 불릿 패턴: •, -, ‣, ▪, ▸, ○, ●, ◆, ★ 등
        bullet_pattern = r"^[•\-‣▪▸○●◆★►◦‑–—]\s"
        if re.match(bullet_pattern, text):
            return "bullet"

        # 번호 패턴: 1., 1), (1), ① 등
        number_pattern = r"^(\d+[.)\]:]|\(\d+\)|[①②③④⑤⑥⑦⑧⑨⑩])\s"
        if re.match(number_pattern, text):
            return "number"

        # 알파벳 패턴: a., a), (a) 등
        letter_pattern = r"^([a-zA-Z][.)\]:]|\([a-zA-Z]\))\s"
        if re.match(letter_pattern, text):
            return "letter"

        # 한글 가나다 패턴: 가., 나., 다. 등
        korean_pattern = r"^[가나다라마바사아자차카타파하][.)\]]\s"
        if re.match(korean_pattern, text):
            return "letter"

        return None

    def _is_two_column_from_lines(self, lines: list[dict], page_width: float) -> bool:
        """라인 기반으로 2-column 레이아웃 감지."""
        if len(lines) < 4:
            return False

        col_boundary = page_width / 2
        margin = page_width * 0.1

        left_lines = [ln for ln in lines if ln["x0"] < col_boundary - margin]
        right_lines = [ln for ln in lines if ln["x0"] > col_boundary - margin]

        if len(left_lines) < 2 or len(right_lines) < 2:
            return False

        # 우측 라인들의 x0이 중앙 근처에서 시작하는지
        right_x0s = [ln["x0"] for ln in right_lines]
        if right_x0s:
            min_right_x0 = min(right_x0s)
            if col_boundary * 0.8 < min_right_x0 < col_boundary * 1.2:
                return True

        return len(left_lines) >= 2 and len(right_lines) >= 2

    def _convert_clusters_to_blocks(self, clusters: list[dict]) -> list[tuple]:
        """클러스터들을 IR 블록으로 변환. 연속 리스트 아이템 그룹화.

        Returns:
            [(y_pos, block), ...]
        """
        results = []
        i = 0

        while i < len(clusters):
            cluster = clusters[i]

            # 리스트 아이템 그룹화 시도
            if cluster.get("is_list_item"):
                list_items = []
                list_type = "ordered" if cluster.get("list_marker") == "number" else "unordered"
                start_y = cluster["y0"]

                # 연속된 리스트 아이템 수집
                while i < len(clusters) and clusters[i].get("is_list_item"):
                    item_text = self._clean_text(clusters[i]["text"])
                    # 마커 제거
                    item_text = self._remove_list_marker(item_text)
                    if item_text:
                        list_items.append(item_text)
                    i += 1

                # 리스트 블록 생성 (2개 이상일 때만)
                if len(list_items) >= 2:
                    list_block = create_list(list_type=list_type, items=list_items)
                    results.append((start_y, list_block))
                elif list_items:
                    # 1개면 그냥 문단으로
                    text = list_items[0]
                    para_block = create_paragraph(text)
                    results.append((start_y, para_block))
            else:
                # 일반 텍스트 블록
                text = self._clean_text(cluster["text"])
                if text:
                    ir_block = self._create_block_from_text(
                        text, cluster["font_size"], cluster.get("is_bold", False)
                    )
                    if ir_block:
                        results.append((cluster["y0"], ir_block))
                i += 1

        return results

    def _remove_list_marker(self, text: str) -> str:
        """텍스트에서 리스트 마커 제거."""
        import re

        text = text.strip()

        # 불릿 제거
        text = re.sub(r"^[•\-‣▪▸○●◆★►◦‑–—]\s*", "", text)

        # 번호 제거
        text = re.sub(r"^(\d+[.)\]:]|\(\d+\)|[①②③④⑤⑥⑦⑧⑨⑩])\s*", "", text)

        # 알파벳 제거
        text = re.sub(r"^([a-zA-Z][.)\]:]|\([a-zA-Z]\))\s*", "", text)

        # 한글 가나다 제거
        text = re.sub(r"^[가나다라마바사아자차카타파하][.)\]]\s*", "", text)

        return text.strip()

    def _is_two_column(self, text_blocks: list, page_width: float) -> bool:
        """2-column 레이아웃인지 감지.

        개선된 감지 로직:
        - 블록들의 x0 분포를 분석
        - 중앙 근처에 빈 공간(gutter)이 있는지 확인
        - 좌/우에 블록이 분포되어 있는지 확인
        """
        if len(text_blocks) < 4:
            return False

        col_boundary = page_width / 2
        margin = page_width * 0.1  # 10% 마진

        # 좌측 컬럼: x0이 페이지 왼쪽 절반에 있는 블록
        left_blocks = [b for b in text_blocks if b["x0"] < col_boundary - margin]
        # 우측 컬럼: x0이 페이지 중앙 이후에 있는 블록
        right_blocks = [b for b in text_blocks if b["x0"] > col_boundary - margin]

        # 양쪽에 블록이 있어야 2-column
        if len(left_blocks) < 2 or len(right_blocks) < 2:
            return False

        # 추가 검증: 우측 블록들의 x0이 중앙 근처에서 시작하는지
        # (실제 2단이면 우측 블록 x0이 페이지 중앙 근처에 몰려있음)
        right_x0s = [b["x0"] for b in right_blocks]
        if right_x0s:
            min_right_x0 = min(right_x0s)
            # 우측 블록이 페이지 중앙 근처(40~60%)에서 시작하면 2-column
            if col_boundary * 0.8 < min_right_x0 < col_boundary * 1.2:
                return True

        # 기존 로직: 양쪽에 블록이 충분히 있으면 2-column
        return len(left_blocks) >= 2 and len(right_blocks) >= 2

    def _overlaps_any_bbox(self, bbox: tuple, table_bboxes: list) -> bool:
        """bbox가 테이블 영역과 겹치는지 확인."""
        x0, y0, x1, y1 = bbox
        for tb in table_bboxes:
            tx0, ty0, tx1, ty1 = tb
            # 겹침 확인
            if not (x1 < tx0 or x0 > tx1 or y1 < ty0 or y0 > ty1):
                return True
        return False

    def _create_block_from_text(self, text: str, font_size: float, is_bold: bool = False):
        """텍스트로부터 적절한 블록 생성."""
        text = text.strip()
        if not text:
            return None

        # 후처리: PUA 문자 정리 + 하이픈 복원
        text = self._clean_text(text)
        if not text:
            return None

        # Heading 판별 (bold 정보 추가)
        if self._looks_like_heading(text, font_size, is_bold):
            level = self._determine_heading_level(text, font_size, is_bold)
            return create_heading(level, text)

        # 일반 문단: 스타일 정보 포함
        return create_paragraph(text, font_size=font_size, is_bold=is_bold)

    def _clean_text(self, text: str) -> str:
        """텍스트 정리: PUA 문자, 대체 문자, 특수문자 처리."""
        import re

        # 1. Private Use Area (PUA) 문자를 불릿/대시로 변환
        # U+F000-U+F8FF: PDF 폰트 내장 특수 기호
        def replace_pua(match):
            char = match.group(0)
            code = ord(char)
            # 흔한 PUA 패턴: 불릿 포인트
            if 0xF000 <= code <= 0xF0FF:
                return "• "  # 불릿으로 변환
            return ""  # 나머지는 제거

        text = re.sub(r"[\uF000-\uF8FF]", replace_pua, text)

        # 2. U+FFFD (REPLACEMENT CHARACTER) 제거
        # PDF에서 문자를 추출하지 못할 때 나타나는 대체 문자
        text = text.replace("\uFFFD", "")

        # 3. 하이픈 복원 (줄바꿈)
        text = re.sub(r"(\w)-\s+(\w)", r"\1\2", text)

        # 4. 연속 공백 정리
        text = re.sub(r"\s+", " ", text)

        return text.strip()

    def _looks_like_heading(self, text: str, font_size: float, is_bold: bool = False) -> bool:
        """heading처럼 보이는지 판별.

        Args:
            text: 텍스트 내용
            font_size: 폰트 크기
            is_bold: bold 여부 (50% 이상 bold일 때 True)

        Note:
            너무 많은 텍스트가 heading으로 판별되지 않도록 매우 보수적으로 판단.
            확실한 경우만 heading으로 인정함.
        """
        if not text:
            return False

        import re

        # 불릿/대시로 시작하면 heading 아님
        if text.startswith("•") or text.startswith("-") or text.startswith("‣"):
            return False

        # 날짜 패턴 (예: "2025. 8.", "2025년") → heading 아님
        if re.match(r"^\d{4}[.\s년]", text):
            return False

        # 괄호로 시작 (예: "[참고]", "(붙임)") → heading 아님
        if text.startswith("[") or text.startswith("("):
            return False

        words = text.split()
        word_count = len(words)

        # 1. 폰트 크기 기반 (18pt 이상만 = 매우 보수적)
        if font_size >= 18:
            return True

        # 2. 로마숫자 패턴 (Ⅰ., Ⅱ. 등) - 확실한 목차/장 제목
        if re.match(r"^[ⅠⅡⅢⅣⅤⅥⅦⅧⅨⅩ]+\.", text):
            return True

        # 3. 숫자 + 한글 제목 패턴 (예: "1 발간 배경 및 목적")
        # 단, 숫자만 있거나 긴 문장은 제외
        if re.match(r"^\d+\s+[가-힣]", text) and word_count <= 6:
            # 제목처럼 보이는지 추가 검증 (마침표로 끝나면 문장)
            if not text.endswith(".") and not text.endswith(","):
                return True

        # 4. 전부 대문자인 영문 (짧을 때만)
        if text.isupper() and text.isascii() and 3 < len(text) < 30 and word_count <= 4:
            return True

        # 나머지는 모두 heading 아님
        return False

    def _determine_heading_level(self, text: str, font_size: float, is_bold: bool = False) -> int:
        """heading 레벨 결정.

        Args:
            text: 텍스트 내용
            font_size: 폰트 크기
            is_bold: bold 여부
        """
        import re

        # 폰트 크기 기반
        if font_size >= 20:
            return 1
        if font_size >= 16:
            return 2

        # 로마숫자 패턴 (Ⅰ, Ⅱ 등) = h1
        if re.match(r"^[ⅠⅡⅢⅣⅤⅥⅦⅧⅨⅩ]+\.?\s", text):
            return 1

        # 번호 패턴으로 레벨 결정
        if re.match(r"^\d+\.\s", text):  # "1. "
            return 1
        if re.match(r"^\d+\.\d+\s", text):  # "1.1 "
            return 2
        if re.match(r"^\d+\.\d+\.\d+\s", text):  # "1.1.1 "
            return 3

        # 기본값
        return 2

    def _extract_tables_multi_strategy(self, page) -> list[tuple]:
        """여러 전략으로 테이블 추출 시도.

        Returns:
            [(bbox, table_data), ...] - 추출된 테이블들
        """
        best_tables = []
        best_score = 0

        # 2단 레이아웃 감지
        is_two_col = self._detect_two_column_layout(page)

        for idx, strategy in enumerate(self.TABLE_SETTINGS_STRATEGIES):
            # 2단 레이아웃에서는 텍스트 기반 전략(인덱스 2, 3) 스킵
            # 텍스트 기반 전략은 2단 텍스트를 테이블로 오인함
            # idx 0, 1은 선 기반이므로 허용
            if is_two_col and idx > 1:
                continue

            try:
                tables = page.find_tables(table_settings=strategy) if strategy else page.find_tables()

                if not tables:
                    continue

                # 이 전략으로 추출된 테이블들의 품질 평가
                strategy_tables = []
                strategy_score = 0

                for table in tables:
                    table_data = table.extract()
                    if table_data and len(table_data) > 0:
                        quality = self._evaluate_table_quality(table_data)
                        if quality > 0:  # 최소 품질 기준 통과
                            strategy_tables.append((table.bbox, table_data))
                            strategy_score += quality

                # 더 좋은 전략 발견 시 교체
                if strategy_score > best_score:
                    best_tables = strategy_tables
                    best_score = strategy_score

            except Exception:
                # 전략 실패 시 다음 전략 시도
                continue

        return best_tables

    def _detect_two_column_layout(self, page) -> bool:
        """페이지가 2단 레이아웃인지 빠르게 감지.

        pdfplumber의 텍스트 추출로 간단히 판단.
        """
        try:
            text = page.extract_text() or ""
            if len(text) < 500:  # 텍스트가 적으면 판단 불가
                return False

            # 텍스트 블록들의 x좌표 분포 확인
            chars = page.chars
            if len(chars) < 100:
                return False

            page_width = page.width
            mid = page_width / 2
            margin = page_width * 0.1

            left_chars = sum(1 for c in chars if c["x0"] < mid - margin)
            right_chars = sum(1 for c in chars if c["x0"] > mid + margin)

            # 양쪽에 문자가 골고루 분포하면 2단
            total = left_chars + right_chars
            if total > 0:
                balance = min(left_chars, right_chars) / max(left_chars, right_chars)
                return balance > 0.3  # 30% 이상 균형이면 2단

            return False
        except Exception:
            return False

    def _evaluate_table_quality(self, table_data: list) -> float:
        """테이블 품질 평가.

        Returns:
            품질 점수 (0~100). 0이면 테이블로 인식 안 함.
        """
        if not table_data or len(table_data) < 2:
            return 0

        # 최소 열 수 체크
        max_cols = max(len(row) for row in table_data if row)
        if max_cols < 2:
            return 0

        # 너무 많은 열은 잘못된 감지일 가능성
        if max_cols > 15:
            return 0

        # 행 수 기준 점수
        row_count = len(table_data)
        row_score = min(row_count * 5, 30)  # 최대 30점

        # 열 일관성 점수 (모든 행이 같은 열 수인지)
        col_counts = [len(row) for row in table_data if row]
        consistency = col_counts.count(max_cols) / len(col_counts) if col_counts else 0
        consistency_score = consistency * 30  # 최대 30점

        # 셀 내용 점수 (비어있지 않은 셀 비율)
        total_cells = sum(len(row) for row in table_data if row)
        filled_cells = sum(1 for row in table_data if row for cell in row if cell and str(cell).strip())
        fill_rate = filled_cells / total_cells if total_cells > 0 else 0
        fill_score = fill_rate * 40  # 최대 40점

        total_score = row_score + consistency_score + fill_score

        # 품질 기준 강화: 최소 50점 필요
        if total_score < 50:
            return 0

        # 헤더 검증: 첫 행의 셀들이 의미있는 텍스트인지
        header = table_data[0]
        header_filled = sum(1 for cell in header if cell and len(str(cell).strip()) > 1)
        if header_filled < max_cols * 0.5:  # 헤더 50% 이상 채워져야 함
            return total_score * 0.5  # 점수 반감

        return total_score

    def _clean_table_cell(self, cell) -> str:
        """테이블 셀 텍스트 정리."""
        import re

        if not cell:
            return ""

        text = str(cell)

        # 1. NULL 문자 제거
        text = text.replace("\x00", " ")

        # 2. 줄바꿈을 공백으로 (세로 텍스트 복원)
        text = text.replace("\n", " ")

        # 3. U+FFFD 제거
        text = text.replace("\uFFFD", "")

        # 4. PUA 문자 처리 (U+F000~U+F8FF → 불릿)
        def replace_pua(match):
            char = match.group(0)
            code = ord(char)
            if 0xF000 <= code <= 0xF0FF:
                return "• "
            return ""

        text = re.sub(r"[\uF000-\uF8FF]", replace_pua, text)

        # 5. 연속 공백 정리
        text = re.sub(r"\s+", " ", text)

        return text.strip()

    def _parse_table(self, table_data: list):
        """테이블 데이터를 IR 블록으로 변환."""
        if not table_data or len(table_data) == 0:
            return None

        # 빈 행 제거 및 정규화
        rows_data = []
        max_cols = 0

        for row in table_data:
            if row:
                cleaned_row = [self._clean_table_cell(cell) for cell in row]
                if any(cell for cell in cleaned_row):
                    rows_data.append(cleaned_row)
                    max_cols = max(max_cols, len(cleaned_row))

        if not rows_data:
            return None

        # 열 수 정규화
        for row in rows_data:
            while len(row) < max_cols:
                row.append("")

        # 첫 행은 header
        header = rows_data[0]
        body_rows = rows_data[1:] if len(rows_data) > 1 else []

        return create_table(header=header, rows=body_rows)
