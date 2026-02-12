"""Normalizer (IR → HTML) 테스트."""

import pytest

from c4.c2.parsers.ir_models import (
    CellStyle,
    Document,
    MergeInfo,
    create_heading,
    create_image,
    create_list,
    create_paragraph,
    create_table,
)
from c4.c2.parsers.normalizer import (
    get_css,
    normalize_block,
    normalize_document,
    normalize_heading,
    normalize_image,
    normalize_list,
    normalize_paragraph,
    normalize_table,
)


class TestNormalizeHeading:
    def test_h1(self):
        h = create_heading(1, "제목")
        assert normalize_heading(h) == "<h1>제목</h1>"

    def test_h3(self):
        h = create_heading(3, "소제목")
        assert normalize_heading(h) == "<h3>소제목</h3>"

    def test_html_escaped(self):
        h = create_heading(1, "<script>alert('xss')</script>")
        html = normalize_heading(h)
        assert "<script>" not in html
        assert "&lt;script&gt;" in html


class TestNormalizeParagraph:
    def test_basic(self):
        p = create_paragraph("본문 텍스트")
        assert normalize_paragraph(p) == "<p>본문 텍스트</p>"

    def test_bold(self):
        p = create_paragraph("굵은", is_bold=True)
        html = normalize_paragraph(p)
        assert "font-weight: bold" in html

    def test_font_size(self):
        p = create_paragraph("큰 글자", font_size=16.0)
        html = normalize_paragraph(p)
        assert "font-size: 16.0pt" in html


class TestNormalizeTable:
    def test_basic(self):
        t = create_table(
            header=["이름", "나이"],
            rows=[["홍길동", "30"]],
        )
        html = normalize_table(t)
        assert "<table>" in html
        assert "</table>" in html
        assert "이름" in html
        assert "홍길동" in html

    def test_merge_colspan(self):
        t = create_table(
            header=["A", "B", "C"],
            rows=[],
            merge_info=[MergeInfo(row=0, col=0, colspan=2)],
        )
        html = normalize_table(t)
        assert 'colspan="2"' in html

    def test_merge_rowspan(self):
        t = create_table(
            header=["A", "B"],
            rows=[["1", "2"]],
            merge_info=[MergeInfo(row=0, col=0, rowspan=2)],
        )
        html = normalize_table(t)
        assert 'rowspan="2"' in html


class TestNormalizeImage:
    def test_with_caption(self):
        img = create_image("img001", "image/png", caption="Figure 1")
        html = normalize_image(img)
        assert "<figure>" in html
        assert "<figcaption>Figure 1</figcaption>" in html
        assert 'alt="Figure 1"' in html

    def test_without_caption(self):
        img = create_image("img002", "image/jpeg")
        html = normalize_image(img)
        assert 'alt="image"' in html

    def test_custom_path(self):
        img = create_image("img003", "image/png")
        html = normalize_image(img, image_path="custom/path.png")
        assert 'src="custom/path.png"' in html


class TestNormalizeList:
    def test_unordered(self):
        lst = create_list("unordered", ["하나", "둘"])
        html = normalize_list(lst)
        assert "<ul>" in html
        assert "<li>하나</li>" in html

    def test_ordered(self):
        lst = create_list("ordered", ["첫째", "둘째"])
        html = normalize_list(lst)
        assert "<ol>" in html


class TestNormalizeDocument:
    def test_full_document(self):
        doc = Document(blocks=[
            create_heading(1, "테스트"),
            create_paragraph("본문"),
        ])
        html = normalize_document(doc)
        assert "<!DOCTYPE html>" in html
        assert "<h1>테스트</h1>" in html
        assert "<p>본문</p>" in html

    def test_theme(self):
        doc = Document(blocks=[create_paragraph("test")])
        html = normalize_document(doc, theme="dark")
        assert "background: #1a1a1a" in html

    def test_empty_document(self):
        doc = Document(blocks=[])
        html = normalize_document(doc)
        assert "<!DOCTYPE html>" in html


class TestGetCSS:
    def test_default(self):
        css = get_css()
        assert "body" in css

    def test_named_theme(self):
        css = get_css("minimal")
        assert "system-ui" in css

    def test_dark_theme(self):
        css = get_css("dark")
        assert "#1a1a1a" in css

    def test_print_theme(self):
        css = get_css("print")
        assert "Georgia" in css

    def test_unknown_theme_falls_back(self):
        css = get_css("nonexistent")
        # Falls back to DEFAULT_CSS
        assert "body" in css
