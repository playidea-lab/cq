"""HTML Normalizer - IR을 구조적 HTML로 변환.

핵심 규칙:
- table은 tbody 구조, 셀별로 th/td 결정 (점수 기반)
- heading은 h1~h3만 사용
- paragraph는 <p> (br 미사용)
- image는 상대 경로로 링크
- list는 ul/ol 사용
"""

from html import escape

import os
from pathlib import Path

from .ir_models import (
    Block,
    Document,
    HeadingBlock,
    ImageBlock,
    ListBlock,
    ParagraphBlock,
    TableBlock,
)


def _escape_cell_text(text: str) -> str:
    """셀 텍스트를 HTML로 변환 (줄바꿈을 <br>로).

    중첩 테이블/이미지 HTML이 포함된 경우 해당 부분은 이스케이프하지 않음.
    """
    if not text:
        return ""

    # HTML 태그가 포함된 경우 (테이블 또는 이미지)
    if "<table" in text or "<img" in text:
        # 텍스트와 HTML을 분리하여 처리
        import re
        parts = []
        last_end = 0

        # <table...>...</table> 또는 <img.../> 또는 <img...> 패턴 찾기
        # <br> 태그도 보존
        html_pattern = r'<table[^>]*>.*?</table>|<img[^>]*>|<br\s*/?>'
        for match in re.finditer(html_pattern, text, re.DOTALL):
            # HTML 앞의 텍스트 이스케이프
            before_text = text[last_end:match.start()]
            if before_text:
                escaped = escape(before_text)
                parts.append(escaped.replace("\n", "<br>\n"))

            # HTML 태그는 그대로 유지
            parts.append(match.group(0))
            last_end = match.end()

        # HTML 뒤의 텍스트 이스케이프
        after_text = text[last_end:]
        if after_text:
            escaped = escape(after_text)
            parts.append(escaped.replace("\n", "<br>\n"))

        return "".join(parts)

    # 일반 텍스트
    escaped = escape(text)
    return escaped.replace("\n", "<br>\n")


def normalize_heading(block: HeadingBlock) -> str:
    """헤딩 블록을 HTML로 변환."""
    level = min(max(block.level, 1), 3)
    text = escape(block.text)
    return f"<h{level}>{text}</h{level}>"


def normalize_paragraph(block: ParagraphBlock) -> str:
    """문단 블록을 HTML로 변환."""
    text = escape(block.text)

    # 스타일 적용
    styles = []
    if block.font_size and block.font_size > 12:
        styles.append(f"font-size: {block.font_size}pt")
    if block.is_bold:
        styles.append("font-weight: bold")

    if styles:
        style_attr = f' style="{"; ".join(styles)}"'
        return f"<p{style_attr}>{text}</p>"

    return f"<p>{text}</p>"


def normalize_table(block: TableBlock) -> str:
    """테이블 블록을 HTML로 변환.

    규칙:
    1. 병합 셀이 있으면 thead/tbody 구분 없이 출력 (rowspan 경계 문제 방지)
    2. 헤더 감지 알고리즘으로 헤더 여부 결정 (스타일/텍스트 특성 기반)
    3. 값이 비어 있으면 빈 문자열 유지
    4. merge_info가 있으면 rowspan/colspan 적용
    5. 병합된 셀(rowspan/colspan으로 커버되는 셀)은 출력하지 않음
    """
    # 머지 정보를 (row, col) -> (rowspan, colspan) 맵으로 변환
    merge_map: dict[tuple[int, int], tuple[int, int]] = {}
    if block.merge_info:
        for m in block.merge_info:
            merge_map[(m.row, m.col)] = (m.rowspan, m.colspan)

    # 병합으로 인해 생략해야 할 셀 위치 계산
    skip_cells = _build_skip_cells(merge_map)

    lines: list[str] = ["<table>"]

    # 모든 테이블을 tbody로 출력, 볼드 셀은 th, 아닌 셀은 td
    lines.append("  <tbody>")

    # 첫 번째 행 (IR의 header)
    lines.append("    <tr>")
    for col_idx, cell in enumerate(block.header):
        if (0, col_idx) in skip_cells:
            continue
        cell_text = _escape_cell_text(cell)
        attrs = _get_merge_attrs(merge_map, 0, col_idx)
        tag = _get_cell_tag(block, 0, col_idx)
        lines.append(f"      <{tag}{attrs}>{cell_text}</{tag}>")
    lines.append("    </tr>")

    # 나머지 행들
    for row_idx, row in enumerate(block.rows):
        ir_row = row_idx + 1  # IR 기준 row index (header가 row 0)
        lines.append("    <tr>")
        for col_idx, cell in enumerate(row):
            if (ir_row, col_idx) in skip_cells:
                continue
            cell_text = _escape_cell_text(cell)
            attrs = _get_merge_attrs(merge_map, ir_row, col_idx)
            tag = _get_cell_tag(block, ir_row, col_idx)
            lines.append(f"      <{tag}{attrs}>{cell_text}</{tag}>")
        lines.append("    </tr>")

    lines.append("  </tbody>")

    lines.append("</table>")
    return "\n".join(lines)


def normalize_image(block: ImageBlock, image_path: str | None = None) -> str:
    """이미지 블록을 HTML로 변환.

    Args:
        block: 이미지 블록
        image_path: 이미지 파일 상대 경로 (없으면 image_id 사용)
    """
    src = image_path or f"images/{block.image_id}"
    alt = escape(block.caption) if block.caption else "image"

    html = f'<figure>\n  <img src="{src}" alt="{alt}" style="max-width: 100%;">'
    if block.caption:
        html += f"\n  <figcaption>{escape(block.caption)}</figcaption>"
    html += "\n</figure>"
    return html


def normalize_list(block: ListBlock) -> str:
    """리스트 블록을 HTML로 변환."""
    tag = "ol" if block.list_type == "ordered" else "ul"
    indent = "  " * block.level

    lines = [f"{indent}<{tag}>"]
    for item in block.items:
        lines.append(f"{indent}  <li>{escape(item)}</li>")
    lines.append(f"{indent}</{tag}>")

    return "\n".join(lines)


def _get_merge_attrs(merge_map: dict[tuple[int, int], tuple[int, int]], row: int, col: int) -> str:
    """병합 속성 문자열 생성."""
    if (row, col) not in merge_map:
        return ""

    rowspan, colspan = merge_map[(row, col)]
    attrs = []
    if rowspan > 1:
        attrs.append(f'rowspan="{rowspan}"')
    if colspan > 1:
        attrs.append(f'colspan="{colspan}"')

    if attrs:
        return " " + " ".join(attrs)
    return ""


def _is_meaningful_background(color: str | None) -> bool:
    """의미 있는 배경색인지 확인 (흰색 제외)."""
    if not color:
        return False
    # 흰색 계열은 배경색으로 취급하지 않음
    upper = color.upper()
    return upper not in ("#FFFFFF", "#FFF", "#FFFFFFFF")


def _calculate_header_score(
    block: TableBlock, row_idx: int, col_idx: int
) -> int:
    """셀의 헤더 점수를 계산합니다.

    점수 기준:
    - 배경색 있음 (흰색 제외): +3점
    - 볼드: +2점
    - 같은 행의 다른 셀과 스타일 차이: +2점
    - 첫 번째 열(양식 테이블 라벨 위치): +1점
    - 라벨|값 패턴 (볼드 셀 옆에 비볼드 셀): +1점

    Returns:
        헤더 점수 (0 이상)
    """
    if not block.cell_styles or row_idx >= len(block.cell_styles):
        return 0

    row_styles = block.cell_styles[row_idx]
    if col_idx >= len(row_styles):
        return 0

    style = row_styles[col_idx]
    if not style:
        return 0

    score = 0
    has_bg = _is_meaningful_background(style.background_color)

    # 1. 배경색 있음 - 흰색 제외 (+3)
    if has_bg:
        score += 3

    # 2. 볼드 (+2)
    if style.is_bold:
        score += 2

    # 3. 같은 행의 다른 셀과 스타일 차이 분석
    # 이 셀만 특별히 강조되어 있으면 헤더일 가능성 높음
    other_styles = [s for i, s in enumerate(row_styles) if i != col_idx and s]

    if other_styles:
        # 이 셀은 볼드인데 다른 셀들은 대부분 볼드가 아니면 +2
        if style.is_bold:
            non_bold_count = sum(1 for s in other_styles if not s.is_bold)
            if non_bold_count > len(other_styles) / 2:
                score += 2

        # 이 셀은 배경색인데 다른 셀들은 대부분 배경색이 없으면 +2
        if has_bg:
            no_bg_count = sum(1 for s in other_styles if not _is_meaningful_background(s.background_color))
            if no_bg_count > len(other_styles) / 2:
                score += 2

    # 4. 첫 번째 열 (양식 테이블에서 라벨 위치) (+1)
    if col_idx == 0:
        score += 1

    # 5. 라벨|값 패턴 감지 (+1)
    # 볼드 셀 옆(좌/우)에 비볼드 셀이 있으면 양식 테이블의 라벨일 가능성
    if style.is_bold:
        # 우측 셀 확인
        if col_idx + 1 < len(row_styles):
            right_style = row_styles[col_idx + 1]
            if right_style and not right_style.is_bold:
                score += 1
        # 좌측 셀 확인 (우측이 없거나 우측도 볼드인 경우)
        elif col_idx > 0:
            left_style = row_styles[col_idx - 1]
            if left_style and not left_style.is_bold:
                score += 1

    return score


def _get_cell_tag(
    block: TableBlock, row_idx: int, col_idx: int
) -> str:
    """셀의 스타일 점수에 따라 th 또는 td 태그 반환.

    종합 점수 기반 판단:
    - 배경색: +3점, 볼드: +2점, 상대적 차이: +2점, 첫 열: +1점
    - 임계값 5점 이상이면 헤더(th)로 판단

    양식 테이블(라벨|값)과 일반 테이블 모두 처리 가능.
    """
    score = _calculate_header_score(block, row_idx, col_idx)

    # 임계값: 5점 이상이면 th
    # - 배경색+볼드 = 5점 (th)
    # - 배경색+첫열 = 4점 (td)
    # - 볼드+상대적차이 = 4점 (td) - 강조 텍스트
    # - 배경색+볼드+첫열 = 6점 (th) - 확실한 헤더
    # - 볼드+상대적차이+첫열 = 5점 (th) - 양식 라벨
    return "th" if score >= 5 else "td"


def _build_skip_cells(merge_map: dict[tuple[int, int], tuple[int, int]]) -> set[tuple[int, int]]:
    """병합으로 인해 생략해야 할 셀 위치 계산.

    rowspan/colspan이 있는 셀의 경우:
    - colspan > 1: 같은 행의 오른쪽 (colspan-1)개 셀 생략
    - rowspan > 1: 아래 (rowspan-1)개 행의 해당 컬럼들 생략

    Returns:
        생략해야 할 (row, col) 위치들의 집합
    """
    skip: set[tuple[int, int]] = set()

    for (row, col), (rowspan, colspan) in merge_map.items():
        # colspan으로 인해 생략할 셀들 (같은 행, 오른쪽 셀들)
        for c in range(col + 1, col + colspan):
            skip.add((row, c))

        # rowspan으로 인해 생략할 셀들 (아래 행들)
        for r in range(row + 1, row + rowspan):
            for c in range(col, col + colspan):
                skip.add((r, c))

    return skip


def normalize_block(block: Block, image_paths: dict[str, str] | None = None) -> str:
    """단일 블록을 HTML로 변환.

    Args:
        block: IR 블록
        image_paths: 이미지 ID -> 파일 경로 매핑
    """
    if isinstance(block, HeadingBlock):
        return normalize_heading(block)
    elif isinstance(block, ParagraphBlock):
        return normalize_paragraph(block)
    elif isinstance(block, TableBlock):
        return normalize_table(block)
    elif isinstance(block, ImageBlock):
        path = image_paths.get(block.image_id) if image_paths else None
        return normalize_image(block, path)
    elif isinstance(block, ListBlock):
        return normalize_list(block)
    else:
        return ""


# ============================================================
# CSS 테마 (app.config에서 인라인)
# ============================================================

DEFAULT_CSS = """
body { font-family: sans-serif; margin: 2rem; line-height: 1.6; }
table { border-collapse: collapse; width: 100%; margin: 1rem 0; }
th, td { border: 1px solid #ccc; padding: 0.5rem; text-align: left; }
th { background-color: #f5f5f5; }
figure { margin: 1rem 0; text-align: center; }
figcaption { color: #666; font-size: 0.9em; margin-top: 0.5rem; }
ul, ol { margin: 0.5rem 0; padding-left: 2rem; }
img { max-width: 100%; height: auto; }
.nested-table { margin: 0; width: 100%; font-size: 0.95em; }
.nested-table td { border: 1px solid #ddd; padding: 0.3rem 0.5rem; }
td > .nested-table { border: none; }
""".strip()

CSS_THEMES = {
    "default": DEFAULT_CSS,
    "minimal": """
body { font-family: system-ui, sans-serif; max-width: 800px; margin: 0 auto; padding: 1rem; }
table { border-collapse: collapse; width: 100%; }
th, td { border: 1px solid #ddd; padding: 8px; }
th { background: #f9f9f9; }
img { max-width: 100%; }
.nested-table { margin: 0; width: 100%; font-size: 0.95em; }
.nested-table td { border: 1px solid #eee; padding: 4px 8px; }
""".strip(),
    "dark": """
body { font-family: sans-serif; margin: 2rem; line-height: 1.6; background: #1a1a1a; color: #e0e0e0; }
table { border-collapse: collapse; width: 100%; margin: 1rem 0; }
th, td { border: 1px solid #444; padding: 0.5rem; text-align: left; }
th { background-color: #2a2a2a; }
figure { margin: 1rem 0; text-align: center; }
figcaption { color: #888; font-size: 0.9em; margin-top: 0.5rem; }
ul, ol { margin: 0.5rem 0; padding-left: 2rem; }
img { max-width: 100%; height: auto; }
a { color: #6db3f2; }
.nested-table { margin: 0; width: 100%; font-size: 0.95em; }
.nested-table td { border: 1px solid #555; padding: 0.3rem 0.5rem; }
""".strip(),
    "print": """
body { font-family: Georgia, serif; margin: 1cm; line-height: 1.5; font-size: 12pt; }
table { border-collapse: collapse; width: 100%; margin: 1em 0; page-break-inside: avoid; }
th, td { border: 1px solid #000; padding: 4pt; }
th { background-color: #eee; }
figure { margin: 1em 0; text-align: center; page-break-inside: avoid; }
img { max-width: 100%; }
h1, h2, h3 { page-break-after: avoid; }
.nested-table { margin: 0; width: 100%; font-size: 0.9em; }
.nested-table td { border: 1px solid #666; padding: 2pt 4pt; }
""".strip(),
}


def get_css(theme: str | None = None) -> str:
    """CSS 반환.

    Args:
        theme: 테마명. None이면 환경변수 DOC2HTML_THEME 사용 (기본: default).
    """
    if theme is None:
        # 커스텀 CSS 파일이 있으면 사용
        custom_css_path = os.getenv("DOC2HTML_CUSTOM_CSS")
        if custom_css_path:
            p = Path(custom_css_path)
            if p.exists():
                return p.read_text(encoding="utf-8")
        theme = os.getenv("DOC2HTML_THEME", "default")

    return CSS_THEMES.get(theme, DEFAULT_CSS)


def normalize_document(
    doc: Document,
    image_paths: dict[str, str] | None = None,
    theme: str | None = None,
) -> str:
    """전체 문서를 HTML로 변환.

    Args:
        doc: IR 문서
        image_paths: 이미지 ID -> 파일 경로 매핑
        theme: CSS 테마명 (None이면 환경변수 사용)
    """
    body_content = "\n".join(
        normalize_block(block, image_paths) for block in doc.blocks
    )

    css = get_css(theme)

    return f"""<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <style>
{css}
  </style>
</head>
<body>
{body_content}
</body>
</html>"""
