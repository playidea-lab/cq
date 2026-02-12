"""Markdown → IR Document 변환.

Markdown 텍스트를 파싱하여 IR Document(HeadingBlock, ParagraphBlock,
TableBlock, ListBlock)로 변환. 제안서 HWPX 출력 파이프라인에서 사용.

사용법::

    from c4.c2.parsers.writer.md_to_ir import markdown_to_ir
    doc = markdown_to_ir(md_text)
"""

from __future__ import annotations

import re

from c4.c2.parsers.ir_models import (
    Block,
    Document,
    HeadingBlock,
    ListBlock,
    ParagraphBlock,
    TableBlock,
)


def markdown_to_ir(text: str) -> Document:
    """Markdown 텍스트를 IR Document로 변환.

    Args:
        text: Markdown 형식 텍스트.

    Returns:
        IR Document 구조.
    """
    blocks: list[Block] = []
    lines = text.split("\n")
    i = 0

    while i < len(lines):
        line = lines[i]
        stripped = line.strip()

        # 빈 줄 스킵
        if not stripped:
            i += 1
            continue

        # 헤딩: # ~ ###
        heading_match = re.match(r"^(#{1,3})\s+(.+)$", stripped)
        if heading_match:
            level = len(heading_match.group(1))
            blocks.append(HeadingBlock(level=level, text=heading_match.group(2).strip()))
            i += 1
            continue

        # 테이블: | col1 | col2 | ...
        if stripped.startswith("|") and "|" in stripped[1:]:
            table_block, i = _parse_table(lines, i)
            if table_block:
                blocks.append(table_block)
            continue

        # 리스트: - item 또는 1. item
        if re.match(r"^[-*]\s+", stripped) or re.match(r"^\d+\.\s+", stripped):
            list_block, i = _parse_list(lines, i)
            if list_block:
                blocks.append(list_block)
            continue

        # 일반 문단: 연속된 비-빈 줄을 하나의 문단으로
        para_lines: list[str] = []
        while i < len(lines):
            cur = lines[i].strip()
            if not cur:
                break
            # 다음 블록 타입이 시작되면 중단
            if re.match(r"^#{1,3}\s+", cur):
                break
            if cur.startswith("|") and "|" in cur[1:]:
                break
            if re.match(r"^[-*]\s+", cur) or re.match(r"^\d+\.\s+", cur):
                break
            if cur.startswith("---") and len(cur.replace("-", "")) == 0:
                i += 1  # 수평선 스킵
                break
            para_lines.append(cur)
            i += 1

        if para_lines:
            blocks.append(ParagraphBlock(text=" ".join(para_lines)))

    return Document(blocks=blocks)


def _parse_table(lines: list[str], start: int) -> tuple[TableBlock | None, int]:
    """테이블 블록 파싱.

    Returns:
        (TableBlock or None, 다음 줄 인덱스)
    """
    i = start
    raw_rows: list[list[str]] = []

    while i < len(lines):
        stripped = lines[i].strip()
        if not stripped.startswith("|") or "|" not in stripped[1:]:
            break
        cells = [c.strip() for c in stripped.split("|")[1:-1]]
        # 구분선 스킵 (|---|---|)
        if cells and all(re.match(r"^:?-+:?$", c) for c in cells):
            i += 1
            continue
        raw_rows.append(cells)
        i += 1

    if not raw_rows:
        return None, i

    header = raw_rows[0]
    data_rows = raw_rows[1:] if len(raw_rows) > 1 else []

    return TableBlock(header=header, rows=data_rows), i


def _parse_list(lines: list[str], start: int) -> tuple[ListBlock | None, int]:
    """리스트 블록 파싱.

    Returns:
        (ListBlock or None, 다음 줄 인덱스)
    """
    i = start
    items: list[str] = []
    first_line = lines[i].strip()

    # 순서 있는 리스트인지 판별
    is_ordered = bool(re.match(r"^\d+\.\s+", first_line))

    while i < len(lines):
        stripped = lines[i].strip()
        if not stripped:
            break

        if is_ordered:
            m = re.match(r"^\d+\.\s+(.+)$", stripped)
        else:
            m = re.match(r"^[-*]\s+(.+)$", stripped)

        if m:
            items.append(m.group(1).strip())
            i += 1
        else:
            break

    if not items:
        return None, i

    list_type = "ordered" if is_ordered else "unordered"
    return ListBlock(list_type=list_type, items=items), i
