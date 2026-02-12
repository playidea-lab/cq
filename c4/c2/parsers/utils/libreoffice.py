"""LibreOffice 변환 유틸리티.

레거시 포맷(DOC, XLS, PPT)을 OOXML(DOCX, XLSX, PPTX)로 변환.
"""

import shutil
import subprocess
import tempfile
from pathlib import Path

# LibreOffice 실행 파일 경로 후보
SOFFICE_PATHS = [
    # Linux
    "/usr/bin/soffice",
    "/usr/bin/libreoffice",
    "/usr/lib/libreoffice/program/soffice",
    "/opt/libreoffice/program/soffice",
    # macOS
    "/Applications/LibreOffice.app/Contents/MacOS/soffice",
    # Windows (다양한 설치 경로)
    "C:\\Program Files\\LibreOffice\\program\\soffice.exe",
    "C:\\Program Files (x86)\\LibreOffice\\program\\soffice.exe",
    "C:\\Program Files\\LibreOffice 7\\program\\soffice.exe",
    "C:\\Program Files\\LibreOffice 24\\program\\soffice.exe",
    # PATH에 있는 경우
    "soffice",
    "libreoffice",
]


def find_soffice() -> str | None:
    """LibreOffice 실행 파일 찾기."""
    for path in SOFFICE_PATHS:
        if shutil.which(path):
            return path
        if Path(path).exists():
            return path
    return None


def convert_to_ooxml(
    input_path: Path,
    output_format: str,
    timeout: int = 60,
) -> Path:
    """LibreOffice로 레거시 포맷을 OOXML로 변환.

    Args:
        input_path: 입력 파일 경로 (.doc, .xls, .ppt)
        output_format: 출력 포맷 ("docx", "xlsx", "pptx")
        timeout: 변환 타임아웃 (초)

    Returns:
        변환된 파일 경로 (임시 디렉토리)

    Raises:
        RuntimeError: LibreOffice 미설치 또는 변환 실패
    """
    soffice = find_soffice()
    if not soffice:
        raise RuntimeError(
            "LibreOffice가 설치되어 있지 않습니다. "
            "레거시 포맷(DOC, XLS, PPT) 변환을 위해 LibreOffice를 설치해주세요."
        )

    # 임시 디렉토리 생성
    temp_dir = Path(tempfile.mkdtemp(prefix="doc2html_"))

    # 입력 파일을 임시 디렉토리로 복사 (파일명 문제 방지)
    temp_input = temp_dir / input_path.name
    shutil.copy2(input_path, temp_input)

    # 포맷 매핑
    format_map = {
        "docx": "docx",
        "xlsx": "xlsx",
        "pptx": "pptx",
    }

    if output_format not in format_map:
        raise ValueError(f"지원하지 않는 출력 포맷: {output_format}")

    # LibreOffice 변환 명령
    cmd = [
        soffice,
        "--headless",
        "--convert-to",
        format_map[output_format],
        "--outdir",
        str(temp_dir),
        str(temp_input),
    ]

    try:
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=timeout,
        )

        if result.returncode != 0:
            raise RuntimeError(
                f"LibreOffice 변환 실패: {result.stderr or result.stdout}"
            )

    except subprocess.TimeoutExpired:
        raise RuntimeError(f"LibreOffice 변환 타임아웃 ({timeout}초)")

    # 변환된 파일 찾기
    output_ext = f".{output_format}"
    output_name = temp_input.stem + output_ext
    output_path = temp_dir / output_name

    if not output_path.exists():
        # 다른 이름으로 생성되었을 수 있음
        candidates = list(temp_dir.glob(f"*{output_ext}"))
        if candidates:
            output_path = candidates[0]
        else:
            raise RuntimeError(
                f"변환된 파일을 찾을 수 없습니다: {output_path}"
            )

    return output_path


def cleanup_temp_file(file_path: Path) -> None:
    """임시 파일 및 디렉토리 정리."""
    try:
        if file_path.exists():
            temp_dir = file_path.parent
            shutil.rmtree(temp_dir, ignore_errors=True)
    except Exception:
        pass
