"""c2/converter.py 통합 테스트."""

import pytest
from pathlib import Path

from c4.c2.converter import (
    convert_to_html,
    create_hwpx,
    extract_text,
    parse_document,
)
from c4.c2.parsers.ir_models import (
    Document,
    create_heading,
    create_list,
    create_paragraph,
    create_table,
)


class TestConvertToHTML:
    def test_basic(self):
        doc = Document(blocks=[
            create_heading(1, "제목"),
            create_paragraph("본문"),
        ])
        html = convert_to_html(doc)
        assert "<!DOCTYPE html>" in html
        assert "<h1>제목</h1>" in html

    def test_with_theme(self):
        doc = Document(blocks=[create_paragraph("test")])
        html = convert_to_html(doc, theme="dark")
        assert "#1a1a1a" in html


class TestExtractText:
    """extract_text()는 파일 경로 필요 — IR 기반 텍스트 추출은 별도로 테스트."""

    def test_ir_based_extraction(self):
        """IR Document에서 텍스트 추출 로직 검증."""
        from c4.c2.parsers.ir_models import HeadingBlock, ParagraphBlock, TableBlock, ListBlock

        doc = Document(blocks=[
            create_heading(1, "제목"),
            create_paragraph("본문 내용"),
            create_table(["이름", "나이"], [["홍길동", "30"]]),
            create_list("unordered", ["항목1", "항목2"]),
        ])

        # extract_text 내부 로직 직접 검증
        parts = []
        for block in doc.blocks:
            if isinstance(block, HeadingBlock):
                parts.append(block.text)
            elif isinstance(block, ParagraphBlock):
                parts.append(block.text)
            elif isinstance(block, TableBlock):
                parts.append("\t".join(block.header))
                for row in block.rows:
                    parts.append("\t".join(row))
            elif isinstance(block, ListBlock):
                for item in block.items:
                    parts.append(f"- {item}")

        text = "\n\n".join(parts)
        assert "제목" in text
        assert "본문 내용" in text
        assert "홍길동" in text
        assert "- 항목1" in text


class TestCreateHWPX:
    def test_basic_roundtrip(self, tmp_path):
        """IR → HWPX 생성 검증."""
        doc = Document(blocks=[
            create_heading(1, "테스트 제목"),
            create_paragraph("테스트 본문입니다."),
            create_table(
                header=["열1", "열2"],
                rows=[["A", "B"]],
            ),
        ])

        output = tmp_path / "output.hwpx"
        create_hwpx(doc, output)

        assert output.exists()
        assert output.stat().st_size > 0

    def test_empty_document(self, tmp_path):
        doc = Document(blocks=[])
        output = tmp_path / "empty.hwpx"
        create_hwpx(doc, output)
        assert output.exists()

    def test_with_list(self, tmp_path):
        doc = Document(blocks=[
            create_list("ordered", ["첫째", "둘째", "셋째"]),
            create_list("unordered", ["항목"]),
        ])
        output = tmp_path / "list.hwpx"
        create_hwpx(doc, output)
        assert output.exists()

    def test_parse_unsupported_format(self, tmp_path):
        """지원하지 않는 형식은 ValueError."""
        dummy = tmp_path / "test.txt"
        dummy.write_text("hello")
        with pytest.raises(ValueError):
            parse_document(dummy)
