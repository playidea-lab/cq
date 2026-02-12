"""c2 document parsers - 멀티포맷 문서 파싱 패키지.

지원 포맷: HWP, HWPX, DOCX, DOC, XLSX, XLS, PPTX, PPT, PDF

사용법::

    from c4.c2.parsers import ParserDispatcher, Document

    dispatcher = ParserDispatcher()
    doc = dispatcher.parse(Path("example.hwp"))
"""

from c4.c2.parsers.base import BaseParser, ImageData, ParseResult
from c4.c2.parsers.dispatcher import ParserDispatcher
from c4.c2.parsers.ir_models import (
    Block,
    Document,
    HeadingBlock,
    ImageBlock,
    ListBlock,
    MergeInfo,
    ParagraphBlock,
    TableBlock,
    create_heading,
    create_image,
    create_list,
    create_paragraph,
    create_table,
)

__all__ = [
    "BaseParser",
    "Block",
    "Document",
    "HeadingBlock",
    "ImageBlock",
    "ImageData",
    "ListBlock",
    "MergeInfo",
    "ParagraphBlock",
    "ParseResult",
    "ParserDispatcher",
    "TableBlock",
    "create_heading",
    "create_image",
    "create_list",
    "create_paragraph",
    "create_table",
]
