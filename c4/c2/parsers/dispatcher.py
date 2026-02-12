"""Dispatcher - 파일 타입에 따라 적절한 파서 선택."""

from pathlib import Path

from c4.c2.parsers.base import BaseParser, ParseResult
from c4.c2.parsers.doc_parser import DocParser
from c4.c2.parsers.docx_parser import DocxParser
from c4.c2.parsers.hwp_parser import HwpParser
from c4.c2.parsers.hwpx_parser import HwpxParser
from c4.c2.parsers.pdf_parser import PdfParser
from c4.c2.parsers.ppt_parser import PptParser
from c4.c2.parsers.pptx_parser import PptxParser
from c4.c2.parsers.xls_parser import XlsParser
from c4.c2.parsers.xlsx_parser import XlsxParser
from c4.c2.parsers.ir_models import Document
from c4.c2.parsers.utils.filetype import get_file_type


class ParserDispatcher:
    """파일 타입에 따라 적절한 파서를 선택하고 실행."""

    def __init__(self):
        self._parsers: dict[str, BaseParser] = {
            "docx": DocxParser(),
            "doc": DocParser(),
            "xlsx": XlsxParser(),
            "xls": XlsParser(),
            "pptx": PptxParser(),
            "ppt": PptParser(),
            "pdf": PdfParser(),
            "hwp": HwpParser(),
            "hwpx": HwpxParser(),
        }

    def parse(self, file_path: Path) -> Document:
        """파일을 파싱하여 IR로 변환.

        Args:
            file_path: 문서 파일 경로

        Returns:
            Document: IR 구조

        Raises:
            ValueError: 지원하지 않는 파일 포맷
        """
        file_type = get_file_type(file_path.name)
        if file_type is None:
            raise ValueError(f"Unsupported file format: {file_path.suffix}")

        parser = self._parsers.get(file_type)
        if parser is None:
            raise ValueError(f"No parser available for: {file_type}")

        return parser.parse(file_path)

    def parse_with_images(self, file_path: Path) -> ParseResult:
        """파일을 파싱하여 IR과 이미지를 함께 반환.

        Args:
            file_path: 문서 파일 경로

        Returns:
            ParseResult: Document + 이미지 목록

        Raises:
            ValueError: 지원하지 않는 파일 포맷
        """
        file_type = get_file_type(file_path.name)
        if file_type is None:
            raise ValueError(f"Unsupported file format: {file_path.suffix}")

        parser = self._parsers.get(file_type)
        if parser is None:
            raise ValueError(f"No parser available for: {file_type}")

        return parser.parse_with_images(file_path)


# 싱글톤 인스턴스
dispatcher = ParserDispatcher()
