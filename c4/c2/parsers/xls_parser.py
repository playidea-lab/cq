"""XLS Parser - LibreOffice 변환 기반.

XLS 파일은 OLE Compound 바이너리 형식(BIFF, Excel 97-2003)으로,
XLSX(OOXML)와 완전히 다른 구조입니다.

이 파서는 LibreOffice를 사용하여 XLS → XLSX 변환 후 XlsxParser로 파싱합니다.
"""

from pathlib import Path

from c4.c2.parsers.base import BaseParser, ParseResult
from c4.c2.parsers.xlsx_parser import XlsxParser
from c4.c2.parsers.ir_models import Document, create_paragraph
from c4.c2.parsers.utils.libreoffice import cleanup_temp_file, convert_to_ooxml, find_soffice


class XlsParser(BaseParser):
    """XLS (Excel 97-2003) 문서 파서.

    LibreOffice를 사용하여 XLSX로 변환 후 파싱합니다.
    LibreOffice가 없으면 안내 메시지를 반환합니다.
    """

    def __init__(self):
        self._xlsx_parser = XlsxParser()

    @property
    def supported_extensions(self) -> list[str]:
        return [".xls"]

    def parse(self, file_path: Path) -> Document:
        """XLS 파일을 IR로 변환."""
        result = self.parse_with_images(file_path)
        return result.document

    def parse_with_images(self, file_path: Path) -> ParseResult:
        """XLS 파일을 IR과 이미지로 변환.

        LibreOffice로 XLSX 변환 후 XlsxParser 사용.
        """
        # LibreOffice 설치 확인
        if not find_soffice():
            return ParseResult(
                document=Document(
                    blocks=[
                        create_paragraph(
                            f"XLS 파일 ({file_path.name}) 변환을 위해 LibreOffice가 필요합니다. "
                            "LibreOffice 설치 후 다시 시도해주세요. "
                            "또는 XLSX 형식으로 변환 후 업로드해주세요."
                        )
                    ]
                )
            )

        temp_xlsx = None
        try:
            # XLS → XLSX 변환
            temp_xlsx = convert_to_ooxml(file_path, "xlsx")

            # XLSX 파서로 파싱
            result = self._xlsx_parser.parse_with_images(temp_xlsx)
            return result

        except RuntimeError as e:
            return ParseResult(
                document=Document(
                    blocks=[
                        create_paragraph(
                            f"XLS 파일 ({file_path.name}) 변환 중 오류가 발생했습니다: {e}"
                        )
                    ]
                )
            )

        finally:
            # 임시 파일 정리
            if temp_xlsx:
                cleanup_temp_file(temp_xlsx)
