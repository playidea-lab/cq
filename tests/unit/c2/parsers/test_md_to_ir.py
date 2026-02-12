"""Tests for c2.parsers.writer.md_to_ir — Markdown → IR 변환."""

import pytest

from c4.c2.parsers.ir_models import (
    Document,
    HeadingBlock,
    ListBlock,
    ParagraphBlock,
    TableBlock,
)
from c4.c2.parsers.writer.md_to_ir import markdown_to_ir


class TestHeadingParsing:
    def test_h1(self):
        doc = markdown_to_ir("# Title")
        assert len(doc.blocks) == 1
        assert isinstance(doc.blocks[0], HeadingBlock)
        assert doc.blocks[0].level == 1
        assert doc.blocks[0].text == "Title"

    def test_h2(self):
        doc = markdown_to_ir("## Subtitle")
        assert doc.blocks[0].level == 2

    def test_h3(self):
        doc = markdown_to_ir("### Sub-subtitle")
        assert doc.blocks[0].level == 3

    def test_multiple_headings(self):
        md = "# H1\n\n## H2\n\n### H3"
        doc = markdown_to_ir(md)
        assert len(doc.blocks) == 3
        assert [b.level for b in doc.blocks] == [1, 2, 3]


class TestParagraphParsing:
    def test_single_line(self):
        doc = markdown_to_ir("Hello world")
        assert len(doc.blocks) == 1
        assert isinstance(doc.blocks[0], ParagraphBlock)
        assert doc.blocks[0].text == "Hello world"

    def test_multiline_paragraph(self):
        md = "Line one\nLine two\nLine three"
        doc = markdown_to_ir(md)
        assert len(doc.blocks) == 1
        assert doc.blocks[0].text == "Line one Line two Line three"

    def test_paragraphs_separated_by_blank_line(self):
        md = "Para one\n\nPara two"
        doc = markdown_to_ir(md)
        assert len(doc.blocks) == 2
        assert doc.blocks[0].text == "Para one"
        assert doc.blocks[1].text == "Para two"

    def test_horizontal_rule_skipped(self):
        md = "Before\n\n---\n\nAfter"
        doc = markdown_to_ir(md)
        texts = [b.text for b in doc.blocks if isinstance(b, ParagraphBlock)]
        assert "Before" in texts
        assert "After" in texts


class TestTableParsing:
    def test_simple_table(self):
        md = "| A | B |\n|---|---|\n| 1 | 2 |\n| 3 | 4 |"
        doc = markdown_to_ir(md)
        assert len(doc.blocks) == 1
        tbl = doc.blocks[0]
        assert isinstance(tbl, TableBlock)
        assert tbl.header == ["A", "B"]
        assert tbl.rows == [["1", "2"], ["3", "4"]]

    def test_table_header_only(self):
        md = "| Col1 | Col2 |\n|---|---|"
        doc = markdown_to_ir(md)
        assert len(doc.blocks) == 1
        tbl = doc.blocks[0]
        assert tbl.header == ["Col1", "Col2"]
        assert tbl.rows == []

    def test_table_with_alignment(self):
        md = "| Left | Center | Right |\n|:---|:---:|---:|\n| a | b | c |"
        doc = markdown_to_ir(md)
        tbl = doc.blocks[0]
        assert tbl.header == ["Left", "Center", "Right"]
        assert tbl.rows == [["a", "b", "c"]]


class TestListParsing:
    def test_unordered_list(self):
        md = "- Item 1\n- Item 2\n- Item 3"
        doc = markdown_to_ir(md)
        assert len(doc.blocks) == 1
        lst = doc.blocks[0]
        assert isinstance(lst, ListBlock)
        assert lst.list_type == "unordered"
        assert lst.items == ["Item 1", "Item 2", "Item 3"]

    def test_ordered_list(self):
        md = "1. First\n2. Second\n3. Third"
        doc = markdown_to_ir(md)
        lst = doc.blocks[0]
        assert lst.list_type == "ordered"
        assert lst.items == ["First", "Second", "Third"]

    def test_asterisk_list(self):
        md = "* A\n* B"
        doc = markdown_to_ir(md)
        lst = doc.blocks[0]
        assert lst.list_type == "unordered"
        assert lst.items == ["A", "B"]


class TestMixedContent:
    def test_heading_paragraph_table_list(self):
        md = """# Title

Some intro paragraph.

| Col | Val |
|-----|-----|
| a   | 1   |

- Item A
- Item B
"""
        doc = markdown_to_ir(md)
        assert len(doc.blocks) == 4
        assert isinstance(doc.blocks[0], HeadingBlock)
        assert isinstance(doc.blocks[1], ParagraphBlock)
        assert isinstance(doc.blocks[2], TableBlock)
        assert isinstance(doc.blocks[3], ListBlock)

    def test_proposal_structure(self):
        md = """# 제안서 제목

## 1. 사업 배경

사업 배경 설명입니다.

## 2. 목표

1. 첫 번째 목표
2. 두 번째 목표

## 3. 예산

| 항목 | 금액 |
|------|------|
| 인건비 | 1000만원 |
| 장비비 | 500만원 |
"""
        doc = markdown_to_ir(md)
        headings = [b for b in doc.blocks if isinstance(b, HeadingBlock)]
        assert len(headings) == 4  # title + 3 sections

        tables = [b for b in doc.blocks if isinstance(b, TableBlock)]
        assert len(tables) == 1
        assert tables[0].header == ["항목", "금액"]

        lists = [b for b in doc.blocks if isinstance(b, ListBlock)]
        assert len(lists) == 1
        assert lists[0].list_type == "ordered"

    def test_empty_input(self):
        doc = markdown_to_ir("")
        assert doc.blocks == []

    def test_whitespace_only(self):
        doc = markdown_to_ir("   \n\n   \n")
        assert doc.blocks == []


class TestEdgeCases:
    def test_heading_without_space(self):
        # "#Title" without space — treated as paragraph, not heading
        doc = markdown_to_ir("#NoSpace")
        assert len(doc.blocks) == 1
        assert isinstance(doc.blocks[0], ParagraphBlock)

    def test_table_followed_by_paragraph(self):
        md = "| A |\n|---|\n| 1 |\n\nSome text"
        doc = markdown_to_ir(md)
        assert len(doc.blocks) == 2
        assert isinstance(doc.blocks[0], TableBlock)
        assert isinstance(doc.blocks[1], ParagraphBlock)

    def test_list_followed_by_paragraph(self):
        md = "- item\n\nParagraph"
        doc = markdown_to_ir(md)
        assert len(doc.blocks) == 2
        assert isinstance(doc.blocks[0], ListBlock)
        assert isinstance(doc.blocks[1], ParagraphBlock)
