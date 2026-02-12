"""Dispatcher 테스트."""

import pytest

from c4.c2.parsers.dispatcher import ParserDispatcher
from c4.c2.parsers.utils.filetype import get_file_type


class TestGetFileType:
    def test_hwp(self):
        assert get_file_type("document.hwp") == "hwp"

    def test_hwpx(self):
        assert get_file_type("document.hwpx") == "hwpx"

    def test_docx(self):
        assert get_file_type("report.docx") == "docx"

    def test_doc(self):
        assert get_file_type("report.doc") == "doc"

    def test_pdf(self):
        assert get_file_type("paper.pdf") == "pdf"

    def test_xlsx(self):
        assert get_file_type("data.xlsx") == "xlsx"

    def test_pptx(self):
        assert get_file_type("slides.pptx") == "pptx"

    def test_unknown(self):
        assert get_file_type("image.jpg") is None

    def test_case_insensitive(self):
        assert get_file_type("DOCUMENT.HWP") == "hwp"


class TestParserDispatcher:
    def test_init(self):
        d = ParserDispatcher()
        assert "hwp" in d._parsers
        assert "docx" in d._parsers
        assert "pdf" in d._parsers
        assert len(d._parsers) == 9

    def test_unsupported_format(self, tmp_path):
        d = ParserDispatcher()
        with pytest.raises(ValueError, match="Unsupported"):
            d.parse(tmp_path / "test.jpg")
