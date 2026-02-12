"""HWPX Writer - IR Document를 HWPX 파일로 변환.

python-hwpx 라이브러리를 사용하여 HWPX(한글 오피스 XML) 파일 생성.
"""

from __future__ import annotations

from pathlib import Path

from c4.c2.parsers.ir_models import (
    Document,
    HeadingBlock,
    ListBlock,
    ParagraphBlock,
    TableBlock,
)


class HwpxWriter:
    """IR Document → HWPX 파일 생성."""

    def __init__(self, template_path: Path | None = None):
        """초기화.

        Args:
            template_path: HWPX 템플릿 파일 경로. None이면 빈 문서 사용.
        """
        self.template_path = template_path

    def write(self, document: Document, output_path: Path) -> None:
        """IR Document를 HWPX 파일로 저장.

        Args:
            document: IR Document
            output_path: 출력 HWPX 파일 경로
        """
        from hwpx.document import HwpxDocument

        if self.template_path:
            hwpx_doc = HwpxDocument.open(str(self.template_path))
        else:
            hwpx_doc = HwpxDocument.new()

        for block in document.blocks:
            if isinstance(block, HeadingBlock):
                hwpx_doc.add_paragraph(block.text)
            elif isinstance(block, ParagraphBlock):
                hwpx_doc.add_paragraph(block.text)
            elif isinstance(block, TableBlock):
                rows = len(block.rows) + 1  # header + data rows
                cols = len(block.header)
                if rows > 0 and cols > 0:
                    table = hwpx_doc.add_table(rows=rows, cols=cols)
                    # header row
                    for j, h in enumerate(block.header):
                        table.set_cell_text(0, j, h)
                    # data rows
                    for i, row in enumerate(block.rows):
                        for j, cell in enumerate(row):
                            if j < cols:
                                table.set_cell_text(i + 1, j, cell)
            elif isinstance(block, ListBlock):
                for idx, item in enumerate(block.items):
                    prefix = f"{idx + 1}. " if block.list_type == "ordered" else "- "
                    hwpx_doc.add_paragraph(f"{prefix}{item}")

        output_path.parent.mkdir(parents=True, exist_ok=True)
        hwpx_doc.save(str(output_path))
