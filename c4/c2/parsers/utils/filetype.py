"""파일 타입 판별 유틸리티."""

from pathlib import Path

# 지원하는 파일 확장자 매핑
SUPPORTED_EXTENSIONS = {
    ".hwp": "hwp",
    ".hwpx": "hwpx",
    ".doc": "doc",
    ".docx": "docx",
    ".ppt": "ppt",
    ".pptx": "pptx",
    ".xls": "xls",
    ".xlsx": "xlsx",
    ".pdf": "pdf",
}


def get_file_type(filename: str) -> str | None:
    """파일 확장자로 문서 타입 판별.

    Args:
        filename: 파일명

    Returns:
        문서 타입 문자열 또는 None (미지원 포맷)
    """
    ext = Path(filename).suffix.lower()
    return SUPPORTED_EXTENSIONS.get(ext)


def is_supported(filename: str) -> bool:
    """지원하는 파일 포맷인지 확인."""
    return get_file_type(filename) is not None
