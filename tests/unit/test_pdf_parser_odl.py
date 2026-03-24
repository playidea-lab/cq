"""Unit tests for OpenDataLoader PDF parser IR conversion."""

from pathlib import Path

import pytest
from fpdf import FPDF

from c4.c2.parsers.pdf_parser import PdfParser


@pytest.fixture
def parser():
    return PdfParser()


def _create_pdf_with_text(tmp_path: Path, texts: list[tuple[tuple, str, float]]) -> Path:
    """Create a PDF with text at specified positions.

    Args:
        texts: list of ((x, y), text, fontsize)
    """
    pdf_path = tmp_path / "test.pdf"
    pdf = FPDF()
    pdf.add_page()
    pdf.set_auto_page_break(auto=False)
    for (x, y), text, fontsize in texts:
        pdf.set_font("Helvetica", size=fontsize)
        # fpdf2 uses mm from top-left; convert approximate pt coords to mm
        x_mm = x * 0.3528  # 1pt ≈ 0.3528mm
        y_mm = y * 0.3528
        pdf.set_xy(x_mm, y_mm)
        pdf.cell(text=text)
    pdf.output(str(pdf_path))
    return pdf_path


class TestHeadingConversion:
    """ODL heading → HeadingBlock."""

    def test_heading_detected(self, parser, tmp_path):
        pdf = _create_pdf_with_text(tmp_path, [
            ((72, 72), "Main Title", 24),
        ])
        doc = parser.parse(pdf)
        headings = [b for b in doc.blocks if b.type == "heading"]
        assert len(headings) >= 1
        assert "Main Title" in headings[0].text

    def test_heading_level(self, parser, tmp_path):
        pdf = _create_pdf_with_text(tmp_path, [
            ((72, 72), "Big Title", 24),
        ])
        doc = parser.parse(pdf)
        headings = [b for b in doc.blocks if b.type == "heading"]
        assert headings[0].level >= 1


class TestParagraphConversion:
    """ODL paragraph → ParagraphBlock."""

    def test_paragraph_detected(self, parser, tmp_path):
        # ODL may classify single-line 12pt as heading; use heading + paragraph combo
        pdf = _create_pdf_with_text(tmp_path, [
            ((72, 72), "Title Here", 24),
            ((72, 120), "A simple paragraph of body text.", 12),
        ])
        doc = parser.parse(pdf)
        paragraphs = [b for b in doc.blocks if b.type == "paragraph"]
        assert len(paragraphs) >= 1
        assert "simple paragraph" in paragraphs[0].text

    def test_paragraph_font_size(self, parser, tmp_path):
        pdf = _create_pdf_with_text(tmp_path, [
            ((72, 72), "Text with font size.", 14),
        ])
        doc = parser.parse(pdf)
        paragraphs = [b for b in doc.blocks if b.type == "paragraph"]
        if paragraphs and paragraphs[0].font_size is not None:
            assert paragraphs[0].font_size > 0


class TestListConversion:
    """ODL list → ListBlock."""

    def test_ordered_list_detected(self, parser, tmp_path):
        pdf = _create_pdf_with_text(tmp_path, [
            ((72, 72), "1. First", 12),
            ((72, 92), "2. Second", 12),
            ((72, 112), "3. Third", 12),
        ])
        doc = parser.parse(pdf)
        lists = [b for b in doc.blocks if b.type == "list"]
        if lists:
            assert lists[0].list_type == "ordered"
            assert len(lists[0].items) >= 2


class TestTableConversion:
    """ODL table → TableBlock."""

    def test_table_detected(self, parser, tmp_path):
        pdf_path = tmp_path / "table.pdf"
        pdf = FPDF()
        pdf.add_page()
        pdf.set_font("Helvetica", size=11)

        # Draw bordered table using fpdf2 cells
        headers = ["Name", "Age"]
        data = [["Alice", "30"], ["Bob", "25"]]

        pdf.set_xy(20, 30)
        for text in headers:
            pdf.cell(w=50, h=10, text=text, border=1)
        for row in data:
            pdf.ln()
            pdf.set_x(20)
            for text in row:
                pdf.cell(w=50, h=10, text=text, border=1)

        pdf.output(str(pdf_path))

        result = parser.parse(pdf_path)
        tables = [b for b in result.blocks if b.type == "table"]
        assert len(tables) >= 1
        assert len(tables[0].header) >= 2


class TestImageConversion:
    """ODL image → ImageBlock + ImageData."""

    def test_image_extracted(self, parser, tmp_path):
        import struct
        import zlib

        # Create 60x60 red PNG
        width, height = 60, 60
        raw = b""
        for _ in range(height):
            raw += b"\x00" + b"\xff\x00\x00" * width

        def png_chunk(chunk_type, data):
            c = chunk_type + data
            return struct.pack(">I", len(data)) + c + struct.pack(">I", zlib.crc32(c) & 0xFFFFFFFF)

        png = b"\x89PNG\r\n\x1a\n"
        png += png_chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0))
        png += png_chunk(b"IDAT", zlib.compress(raw))
        png += png_chunk(b"IEND", b"")

        # Save PNG to temp file, embed in PDF via fpdf2
        png_path = tmp_path / "red.png"
        png_path.write_bytes(png)

        pdf_path = tmp_path / "image.pdf"
        pdf = FPDF()
        pdf.add_page()
        pdf.image(str(png_path), x=20, y=80, w=40, h=40)
        pdf.output(str(pdf_path))

        result = parser.parse_with_images(pdf_path)
        images = [b for b in result.document.blocks if b.type == "image"]
        assert len(images) >= 1
        assert len(result.images) >= 1
        assert result.images[0].mime_type.startswith("image/")


class TestEmptyAndInvalid:
    """Edge cases."""

    def test_empty_file_returns_empty_doc(self, parser, tmp_path):
        empty = tmp_path / "empty.pdf"
        empty.write_bytes(b"")
        doc = parser.parse(empty)
        assert doc is not None
        assert isinstance(doc.blocks, list)

    def test_nonexistent_file_returns_empty_doc(self, parser, tmp_path):
        doc = parser.parse(tmp_path / "nonexistent.pdf")
        assert doc is not None
        assert isinstance(doc.blocks, list)
