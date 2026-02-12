"""공용 이미지 유틸리티 - 이미지 ID 생성 및 MIME 타입 매핑."""

import hashlib

# MIME 타입 → 확장자 매핑
MIME_TO_EXTENSION = {
    "image/jpeg": ".jpg",
    "image/png": ".png",
    "image/gif": ".gif",
    "image/bmp": ".bmp",
    "image/webp": ".webp",
    "image/tiff": ".tiff",
}

# 확장자 → MIME 타입 매핑 (PDF 파서용)
EXTENSION_TO_MIME = {
    "png": ("image/png", ".png"),
    "jpeg": ("image/jpeg", ".jpg"),
    "jpg": ("image/jpeg", ".jpg"),
    "jpe": ("image/jpeg", ".jpg"),
    "jpx": ("image/jpeg", ".jpg"),
    "gif": ("image/gif", ".gif"),
    "bmp": ("image/bmp", ".bmp"),
    "tiff": ("image/tiff", ".tiff"),
    "tif": ("image/tiff", ".tiff"),
    "webp": ("image/webp", ".webp"),
}


def generate_image_id(data: bytes, index: int) -> str:
    """이미지 데이터로부터 고유 ID 생성.

    SHA256 12자 사용으로 충돌 확률 최소화.
    - MD5 8자: 16^8 = 4.3억 (충돌 확률 높음)
    - SHA256 12자: 16^12 = 281조 (충돌 확률 극히 낮음)

    Args:
        data: 이미지 바이너리 데이터
        index: 이미지 인덱스 번호

    Returns:
        고유 ID (예: "img_001_abc123456789")
    """
    content_hash = hashlib.sha256(data).hexdigest()[:12]
    return f"img_{index:03d}_{content_hash}"


def get_extension_from_mime(mime_type: str) -> str:
    """MIME 타입에서 파일 확장자 추출.

    Args:
        mime_type: MIME 타입 (예: "image/jpeg")

    Returns:
        확장자 (예: ".jpg") 또는 기본값 ".bin"
    """
    return MIME_TO_EXTENSION.get(mime_type, ".bin")


def get_mime_from_extension(ext: str) -> tuple[str, str]:
    """파일 확장자에서 MIME 타입과 정규화된 확장자 추출.

    Args:
        ext: 파일 확장자 (예: "jpeg", "jpg")

    Returns:
        (MIME 타입, 정규화된 확장자) 튜플
        기본값: ("image/png", ".png")
    """
    ext_lower = ext.lower().lstrip(".")
    return EXTENSION_TO_MIME.get(ext_lower, ("image/png", ".png"))
