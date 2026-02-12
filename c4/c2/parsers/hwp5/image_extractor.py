"""ImageExtractor - HWP 5.0 이미지 추출 모듈.

HWP 문서의 BinData 스토리지에서 이미지를 추출합니다.

참조:
- HWP 5.0 스펙 3.2.5절: 바이너리 데이터
- HWP 5.0 스펙 4.2.3절: 바이너리 데이터 레코드
"""

import struct
import zlib
from dataclasses import dataclass
from enum import IntEnum
from pathlib import Path

from .ole_reader import OleReader
from .record_parser import HwpTagId, RecordParser


class BinDataType(IntEnum):
    """바이너리 데이터 타입."""

    LINK = 0  # 외부 파일 참조
    EMBEDDING = 1  # 파일 포함
    STORAGE = 2  # OLE 포함


class BinDataCompression(IntEnum):
    """바이너리 데이터 압축 모드."""

    DEFAULT = 0x00  # 스토리지 기본 모드
    COMPRESS = 0x10  # 무조건 압축
    NO_COMPRESS = 0x20  # 무조건 비압축


class ImageFormat(IntEnum):
    """이미지 포맷."""

    UNKNOWN = 0
    BMP = 1
    JPEG = 2
    PNG = 3
    GIF = 4
    TIFF = 5
    WMF = 6
    EMF = 7


# 이미지 시그니처 (매직 바이트)
IMAGE_SIGNATURES = {
    # PNG: 89 50 4E 47 0D 0A 1A 0A
    b"\x89PNG\r\n\x1a\n": ImageFormat.PNG,
    # JPEG: FF D8 FF
    b"\xff\xd8\xff": ImageFormat.JPEG,
    # GIF87a / GIF89a
    b"GIF87a": ImageFormat.GIF,
    b"GIF89a": ImageFormat.GIF,
    # BMP: 42 4D
    b"BM": ImageFormat.BMP,
    # TIFF (little-endian): 49 49 2A 00
    b"II*\x00": ImageFormat.TIFF,
    # TIFF (big-endian): 4D 4D 00 2A
    b"MM\x00*": ImageFormat.TIFF,
    # WMF: D7 CD C6 9A (placeable) or 01 00 09 00
    b"\xd7\xcd\xc6\x9a": ImageFormat.WMF,
    b"\x01\x00\x09\x00": ImageFormat.WMF,
    # EMF: 01 00 00 00
    b"\x01\x00\x00\x00": ImageFormat.EMF,
}

# 포맷별 파일 확장자
FORMAT_EXTENSIONS = {
    ImageFormat.BMP: ".bmp",
    ImageFormat.JPEG: ".jpg",
    ImageFormat.PNG: ".png",
    ImageFormat.GIF: ".gif",
    ImageFormat.TIFF: ".tiff",
    ImageFormat.WMF: ".wmf",
    ImageFormat.EMF: ".emf",
    ImageFormat.UNKNOWN: ".bin",
}


@dataclass
class BinDataInfo:
    """바이너리 데이터 메타데이터."""

    bin_id: int  # BinData ID (0부터 시작)
    data_type: BinDataType  # 데이터 타입
    compression: BinDataCompression  # 압축 모드
    extension: str = ""  # 파일 확장자 (확장자 없이)
    abs_path: str = ""  # 절대 경로 (LINK 타입)
    rel_path: str = ""  # 상대 경로 (LINK 타입)


@dataclass
class ImageData:
    """추출된 이미지 데이터."""

    bin_id: int  # BinData ID
    data: bytes  # 원본 이미지 데이터
    format: ImageFormat  # 감지된 이미지 포맷
    extension: str  # 파일 확장자 (점 포함)
    width: int = 0  # 이미지 너비 (가능한 경우)
    height: int = 0  # 이미지 높이 (가능한 경우)
    record_index: int | None = None  # GSO 앵커의 레코드 인덱스 (위치 정렬용)

    @property
    def size(self) -> int:
        """이미지 데이터 크기 (바이트)."""
        return len(self.data)

    @property
    def suggested_filename(self) -> str:
        """제안 파일명."""
        return f"image_{self.bin_id:04d}{self.extension}"


class ImageExtractor:
    """HWP 이미지 추출기.

    사용법:
        # OleReader와 함께 사용
        reader = OleReader("document.hwp")
        extractor = ImageExtractor(reader)
        images = extractor.extract_images()

        # 특정 ID의 이미지 가져오기
        image = extractor.get_image_by_id(0)

        # 이미지 저장
        extractor.save_images("output_dir/")
    """

    def __init__(self, reader: OleReader) -> None:
        """ImageExtractor를 초기화합니다.

        Args:
            reader: OleReader 인스턴스
        """
        self._reader = reader
        self._record_parser = RecordParser()
        self._bin_data_info: dict[int, BinDataInfo] = {}
        self._images: dict[int, ImageData] = {}
        self._parsed = False

    def extract_images(self) -> list[ImageData]:
        """문서에서 모든 이미지를 추출합니다.

        Returns:
            ImageData 리스트 (순서 유지)
        """
        if not self._parsed:
            self._parse_bin_data_info()
            self._extract_all_images()
            self._parsed = True

        # bin_id 순서로 정렬하여 반환
        return [
            self._images[bin_id]
            for bin_id in sorted(self._images.keys())
        ]

    def get_image_by_id(self, bin_id: int) -> ImageData | None:
        """특정 ID의 이미지를 가져옵니다.

        Args:
            bin_id: BinData ID

        Returns:
            ImageData 또는 None
        """
        if not self._parsed:
            self._parse_bin_data_info()
            self._extract_all_images()
            self._parsed = True

        return self._images.get(bin_id)

    def list_bin_data_streams(self) -> list[str]:
        """BinData 스트림 목록을 반환합니다.

        Returns:
            BinData 스트림 이름 리스트
        """
        streams = self._reader.list_streams()
        return [s for s in streams if s.startswith("BinData/")]

    def save_images(self, output_dir: str | Path) -> list[Path]:
        """추출된 이미지를 파일로 저장합니다.

        Args:
            output_dir: 출력 디렉토리

        Returns:
            저장된 파일 경로 리스트
        """
        output_path = Path(output_dir)
        output_path.mkdir(parents=True, exist_ok=True)

        images = self.extract_images()
        saved_paths: list[Path] = []

        for image in images:
            file_path = output_path / image.suggested_filename
            file_path.write_bytes(image.data)
            saved_paths.append(file_path)

        return saved_paths

    def _parse_bin_data_info(self) -> None:
        """DocInfo에서 BinData 메타데이터를 파싱합니다."""
        try:
            docinfo_data = self._reader.read_stream("DocInfo")
        except (KeyError, FileNotFoundError):
            return

        records = self._record_parser.parse_records(docinfo_data, decompress=True)

        bin_id = 0
        for record in records:
            if record.tag_id == HwpTagId.BIN_DATA:
                info = self._parse_bin_data_record(record.data, bin_id)
                if info:
                    self._bin_data_info[bin_id] = info
                bin_id += 1

    def _parse_bin_data_record(self, data: bytes, bin_id: int) -> BinDataInfo | None:
        """HWPTAG_BIN_DATA 레코드를 파싱합니다.

        표 17 바이너리 데이터:
        - UINT16: 속성
        - Type에 따라 가변 길이 필드
        """
        if len(data) < 2:
            return None

        offset = 0

        # 속성 (UINT16)
        props = struct.unpack_from("<H", data, offset)[0]
        offset += 2

        # 속성 비트 파싱
        data_type = BinDataType(props & 0x0F)
        compression = BinDataCompression((props >> 4) & 0x30)

        info = BinDataInfo(
            bin_id=bin_id,
            data_type=data_type,
            compression=compression,
        )

        # LINK 타입: 절대/상대 경로
        if data_type == BinDataType.LINK:
            # 절대 경로
            if len(data) >= offset + 2:
                len1 = struct.unpack_from("<H", data, offset)[0]
                offset += 2
                if len(data) >= offset + len1 * 2:
                    info.abs_path = data[offset : offset + len1 * 2].decode(
                        "utf-16-le", errors="ignore"
                    ).rstrip("\x00")
                    offset += len1 * 2

            # 상대 경로
            if len(data) >= offset + 2:
                len2 = struct.unpack_from("<H", data, offset)[0]
                offset += 2
                if len(data) >= offset + len2 * 2:
                    info.rel_path = data[offset : offset + len2 * 2].decode(
                        "utf-16-le", errors="ignore"
                    ).rstrip("\x00")
                    offset += len2 * 2

        # EMBEDDING/STORAGE 타입: BinData ID 및 확장자
        elif data_type in (BinDataType.EMBEDDING, BinDataType.STORAGE):
            # BinData 스토리지 ID (이미 bin_id로 설정됨)
            if len(data) >= offset + 2:
                # storage_id = struct.unpack_from("<H", data, offset)[0]
                offset += 2

            # 확장자 (EMBEDDING 타입)
            if data_type == BinDataType.EMBEDDING and len(data) >= offset + 2:
                len3 = struct.unpack_from("<H", data, offset)[0]
                offset += 2
                if len(data) >= offset + len3 * 2:
                    info.extension = data[offset : offset + len3 * 2].decode(
                        "utf-16-le", errors="ignore"
                    ).rstrip("\x00")

        return info

    def _extract_all_images(self) -> None:
        """모든 BinData 스트림에서 이미지를 추출합니다."""
        streams = self.list_bin_data_streams()

        for stream_name in streams:
            # 스트림 이름에서 ID 추출 (예: "BinData/BIN0001.jpg" → 0)
            bin_id = self._parse_stream_id(stream_name)
            if bin_id is None:
                continue

            try:
                raw_data = self._reader.read_stream(stream_name)
                image_data = self._decompress_if_needed(raw_data, bin_id)
                image_format = self.detect_format(image_data)
                extension = FORMAT_EXTENSIONS.get(image_format, ".bin")

                # BinData 메타데이터에서 확장자 가져오기
                if bin_id in self._bin_data_info:
                    info = self._bin_data_info[bin_id]
                    if info.extension:
                        extension = f".{info.extension.lower()}"

                self._images[bin_id] = ImageData(
                    bin_id=bin_id,
                    data=image_data,
                    format=image_format,
                    extension=extension,
                )

            except (KeyError, zlib.error):
                continue

    def _parse_stream_id(self, stream_name: str) -> int | None:
        """스트림 이름에서 BinData ID를 추출합니다.

        예: "BinData/BIN0001.jpg" → 0 (0-indexed)
        """
        # BinData/BINxxxx 형식
        if not stream_name.startswith("BinData/"):
            return None

        name = stream_name.split("/")[-1]  # "BIN0001.jpg"

        # 확장자 제거
        if "." in name:
            name = name.rsplit(".", 1)[0]  # "BIN0001"

        # BIN 접두사 제거 및 숫자 추출
        if name.upper().startswith("BIN"):
            try:
                # BIN0001 → 1 → 0 (0-indexed로 변환)
                return int(name[3:]) - 1
            except ValueError:
                pass

        return None

    def _decompress_if_needed(self, data: bytes, bin_id: int) -> bytes:
        """필요시 데이터 압축을 해제합니다."""
        # BinData 메타데이터에서 압축 모드 확인
        info = self._bin_data_info.get(bin_id)

        if info and info.compression == BinDataCompression.NO_COMPRESS:
            return data

        # 압축 여부 자동 감지: zlib 헤더 확인 또는 이미지 시그니처 확인
        if self.detect_format(data) != ImageFormat.UNKNOWN:
            # 이미 유효한 이미지 데이터면 압축 해제 불필요
            return data

        # zlib 압축 해제 시도 (raw deflate)
        try:
            return zlib.decompress(data, -15)
        except zlib.error:
            # 압축되지 않은 데이터
            return data

    @staticmethod
    def detect_format(data: bytes) -> ImageFormat:
        """이미지 시그니처로 포맷을 감지합니다.

        Args:
            data: 이미지 데이터

        Returns:
            ImageFormat enum
        """
        if not data:
            return ImageFormat.UNKNOWN

        # 시그니처 매칭 (가장 긴 것부터 확인)
        for signature, fmt in sorted(
            IMAGE_SIGNATURES.items(), key=lambda x: -len(x[0])
        ):
            if data.startswith(signature):
                return fmt

        return ImageFormat.UNKNOWN

    @staticmethod
    def get_image_dimensions(data: bytes, fmt: ImageFormat) -> tuple[int, int]:
        """이미지 크기를 추출합니다.

        Args:
            data: 이미지 데이터
            fmt: 이미지 포맷

        Returns:
            (width, height) 튜플, 추출 실패시 (0, 0)
        """
        try:
            if fmt == ImageFormat.PNG:
                # PNG: IHDR 청크에서 크기 추출
                if len(data) >= 24 and data[12:16] == b"IHDR":
                    width = struct.unpack(">I", data[16:20])[0]
                    height = struct.unpack(">I", data[20:24])[0]
                    return width, height

            elif fmt == ImageFormat.JPEG:
                # JPEG: SOF0/SOF2 마커에서 크기 추출
                offset = 2
                while offset < len(data) - 8:
                    if data[offset] != 0xFF:
                        break
                    marker = data[offset + 1]
                    if marker in (0xC0, 0xC2):  # SOF0, SOF2
                        height = struct.unpack(">H", data[offset + 5 : offset + 7])[0]
                        width = struct.unpack(">H", data[offset + 7 : offset + 9])[0]
                        return width, height
                    # 다음 마커로 이동
                    length = struct.unpack(">H", data[offset + 2 : offset + 4])[0]
                    offset += 2 + length

            elif fmt == ImageFormat.GIF:
                # GIF: 논리 화면 크기
                if len(data) >= 10:
                    width = struct.unpack("<H", data[6:8])[0]
                    height = struct.unpack("<H", data[8:10])[0]
                    return width, height

            elif fmt == ImageFormat.BMP:
                # BMP: DIB 헤더에서 크기 추출
                if len(data) >= 26:
                    width = struct.unpack("<I", data[18:22])[0]
                    height = abs(struct.unpack("<i", data[22:26])[0])
                    return width, height

        except (struct.error, IndexError):
            pass

        return 0, 0
