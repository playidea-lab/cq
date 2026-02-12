"""OLE Compound Document Reader - 순수 Python 구현.

OLE (Object Linking and Embedding) Compound Document 형식을 파싱합니다.
HWP 5.x 파일은 OLE 컨테이너 내에 여러 스트림으로 구성됩니다.

참조:
- MS-CFB: Microsoft Compound File Binary Format
- HWP 5.0 스펙 문서
"""

import struct
from dataclasses import dataclass
from io import BytesIO
from pathlib import Path
from typing import BinaryIO

# OLE 시그니처: D0 CF 11 E0 A1 B1 1A E1
OLE_SIGNATURE = b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"

# 특수 섹터 값
ENDOFCHAIN = 0xFFFFFFFE  # -2: 체인 종료
FREESECT = 0xFFFFFFFF  # -1: 사용되지 않는 섹터
FATSECT = 0xFFFFFFFD  # FAT 섹터
DIFSECT = 0xFFFFFFFC  # DIFAT 섹터

# Directory Entry 타입
DIR_TYPE_EMPTY = 0
DIR_TYPE_STORAGE = 1
DIR_TYPE_STREAM = 2
DIR_TYPE_ROOT = 5

# Directory Entry 크기 (고정)
DIR_ENTRY_SIZE = 128


@dataclass
class DirectoryEntry:
    """OLE Directory Entry."""

    name: str
    entry_type: int
    color: int  # 0=red, 1=black (RB 트리)
    left_sibling_id: int
    right_sibling_id: int
    child_id: int
    clsid: bytes
    state_bits: int
    creation_time: int
    modification_time: int
    start_sector: int
    size: int

    @property
    def is_stream(self) -> bool:
        return self.entry_type == DIR_TYPE_STREAM

    @property
    def is_storage(self) -> bool:
        return self.entry_type in (DIR_TYPE_STORAGE, DIR_TYPE_ROOT)

    @property
    def is_root(self) -> bool:
        return self.entry_type == DIR_TYPE_ROOT


class OleReadError(Exception):
    """OLE 파일 읽기 오류."""

    pass


class OleReader:
    """OLE Compound Document 리더.

    순수 Python으로 구현된 OLE 파일 파서입니다.
    olefile 라이브러리 없이 HWP 파일의 스트림을 읽을 수 있습니다.

    사용법:
        reader = OleReader(file_path)
        streams = reader.list_streams()
        data = reader.read_stream("BodyText/Section0")
        reader.close()

    또는 context manager 사용:
        with OleReader(file_path) as reader:
            data = reader.read_stream("DocInfo")
    """

    def __init__(self, source: Path | str | bytes | BinaryIO):
        """OLE 파일을 엽니다.

        Args:
            source: 파일 경로, 바이트 데이터, 또는 파일 객체
        """
        self._fp: BinaryIO | None = None
        self._own_fp = False  # 파일 핸들 소유 여부

        # 헤더 정보
        self._sector_size = 512
        self._mini_sector_size = 64
        self._mini_stream_cutoff = 4096
        self._fat_sector_count = 0
        self._first_dir_sector = 0
        self._first_mini_fat_sector = 0
        self._mini_fat_sector_count = 0
        self._first_difat_sector = 0
        self._difat_sector_count = 0

        # FAT 및 디렉토리
        self._fat: list[int] = []
        self._mini_fat: list[int] = []
        self._directory: list[DirectoryEntry] = []
        self._mini_stream: bytes = b""

        # 스트림 이름 캐시
        self._stream_paths: dict[str, int] = {}

        self._open(source)
        self._parse_header()
        self._build_fat()
        self._parse_directory()
        self._build_mini_stream()

    def _open(self, source: Path | str | bytes | BinaryIO) -> None:
        """소스에서 파일 핸들을 얻습니다."""
        if isinstance(source, (str, Path)):
            self._fp = open(source, "rb")
            self._own_fp = True
        elif isinstance(source, bytes):
            self._fp = BytesIO(source)
            self._own_fp = True
        else:
            self._fp = source
            self._own_fp = False

    def close(self) -> None:
        """파일을 닫습니다."""
        if self._fp and self._own_fp:
            self._fp.close()
        self._fp = None

    def __enter__(self) -> "OleReader":
        return self

    def __exit__(self, *args) -> None:
        self.close()

    def _parse_header(self) -> None:
        """OLE 헤더를 파싱합니다."""
        if not self._fp:
            raise OleReadError("파일이 열리지 않았습니다")

        self._fp.seek(0)
        header = self._fp.read(512)

        if len(header) < 512:
            raise OleReadError("파일이 너무 작습니다")

        # 시그니처 검증
        signature = header[0:8]
        if signature != OLE_SIGNATURE:
            raise OleReadError(
                f"올바른 OLE 시그니처가 아닙니다: {signature.hex()}"
            )

        # Minor/Major 버전 (정보용)
        # minor_version = struct.unpack("<H", header[24:26])[0]
        # major_version = struct.unpack("<H", header[26:28])[0]

        # 바이트 오더 (0xFFFE = 리틀 엔디안)
        byte_order = struct.unpack("<H", header[28:30])[0]
        if byte_order != 0xFFFE:
            raise OleReadError(f"지원하지 않는 바이트 오더: {byte_order}")

        # 섹터 크기 (2^n)
        sector_shift = struct.unpack("<H", header[30:32])[0]
        self._sector_size = 1 << sector_shift

        # 미니 섹터 크기 (2^n)
        mini_sector_shift = struct.unpack("<H", header[32:34])[0]
        self._mini_sector_size = 1 << mini_sector_shift

        # FAT 섹터 수
        self._fat_sector_count = struct.unpack("<I", header[44:48])[0]

        # 첫 번째 Directory 섹터
        self._first_dir_sector = struct.unpack("<I", header[48:52])[0]

        # Mini Stream Cutoff 크기 (기본 4096)
        self._mini_stream_cutoff = struct.unpack("<I", header[56:60])[0]

        # 첫 번째 Mini FAT 섹터
        self._first_mini_fat_sector = struct.unpack("<I", header[60:64])[0]

        # Mini FAT 섹터 수
        self._mini_fat_sector_count = struct.unpack("<I", header[64:68])[0]

        # 첫 번째 DIFAT 섹터
        self._first_difat_sector = struct.unpack("<I", header[68:72])[0]

        # DIFAT 섹터 수
        self._difat_sector_count = struct.unpack("<I", header[72:76])[0]

        # 헤더 내 DIFAT 배열 (최대 109개)
        self._header_difat = []
        for i in range(109):
            offset = 76 + i * 4
            sect = struct.unpack("<I", header[offset : offset + 4])[0]
            if sect != FREESECT:
                self._header_difat.append(sect)

    def _read_sector(self, sector_id: int) -> bytes:
        """섹터 데이터를 읽습니다."""
        if not self._fp:
            raise OleReadError("파일이 열리지 않았습니다")

        # 섹터 0은 헤더 다음에 위치 (오프셋 512부터)
        offset = (sector_id + 1) * self._sector_size
        self._fp.seek(offset)
        return self._fp.read(self._sector_size)

    def _build_fat(self) -> None:
        """FAT (File Allocation Table)을 구축합니다."""
        # DIFAT에서 FAT 섹터 목록 수집
        fat_sectors = list(self._header_difat)

        # 추가 DIFAT 섹터가 있는 경우
        if self._difat_sector_count > 0:
            difat_sect = self._first_difat_sector
            for _ in range(self._difat_sector_count):
                if difat_sect in (FREESECT, ENDOFCHAIN):
                    break
                data = self._read_sector(difat_sect)
                entries_per_sector = (self._sector_size // 4) - 1
                for i in range(entries_per_sector):
                    sect = struct.unpack("<I", data[i * 4 : (i + 1) * 4])[0]
                    if sect != FREESECT:
                        fat_sectors.append(sect)
                # 마지막 4바이트는 다음 DIFAT 섹터
                difat_sect = struct.unpack(
                    "<I", data[-4:]
                )[0]

        # FAT 섹터들을 읽어 FAT 배열 구축
        self._fat = []
        for sect_id in fat_sectors:
            data = self._read_sector(sect_id)
            entries_per_sector = self._sector_size // 4
            for i in range(entries_per_sector):
                entry = struct.unpack("<I", data[i * 4 : (i + 1) * 4])[0]
                self._fat.append(entry)

    def _get_sector_chain(self, start_sector: int) -> list[int]:
        """섹터 체인을 따라갑니다."""
        chain = []
        sect = start_sector
        visited = set()

        while sect not in (ENDOFCHAIN, FREESECT, FATSECT, DIFSECT):
            if sect >= len(self._fat):
                break
            if sect in visited:
                # 순환 감지
                break
            visited.add(sect)
            chain.append(sect)
            sect = self._fat[sect]

        return chain

    def _read_stream_data(self, start_sector: int, size: int) -> bytes:
        """일반 스트림 데이터를 읽습니다."""
        chain = self._get_sector_chain(start_sector)
        data = b""
        for sect_id in chain:
            data += self._read_sector(sect_id)
        return data[:size]

    def _parse_directory(self) -> None:
        """Directory를 파싱합니다."""
        # Directory 섹터 체인 읽기
        dir_data = self._read_stream_data(
            self._first_dir_sector,
            len(self._get_sector_chain(self._first_dir_sector)) * self._sector_size,
        )

        # Directory Entry 파싱
        self._directory = []
        num_entries = len(dir_data) // DIR_ENTRY_SIZE

        for i in range(num_entries):
            offset = i * DIR_ENTRY_SIZE
            entry_data = dir_data[offset : offset + DIR_ENTRY_SIZE]
            entry = self._parse_directory_entry(entry_data)
            self._directory.append(entry)

        # 스트림 경로 맵 구축
        self._build_stream_paths()

    def _parse_directory_entry(self, data: bytes) -> DirectoryEntry:
        """Directory Entry를 파싱합니다."""
        # 이름 (UTF-16LE, 64바이트, null 종료)
        name_len = struct.unpack("<H", data[64:66])[0]
        if name_len > 0:
            name_bytes = data[: name_len - 2]  # null 제외
            name = name_bytes.decode("utf-16-le", errors="replace")
        else:
            name = ""

        entry_type = data[66]
        color = data[67]
        left_sibling_id = struct.unpack("<I", data[68:72])[0]
        right_sibling_id = struct.unpack("<I", data[72:76])[0]
        child_id = struct.unpack("<I", data[76:80])[0]
        clsid = data[80:96]
        state_bits = struct.unpack("<I", data[96:100])[0]
        creation_time = struct.unpack("<Q", data[100:108])[0]
        modification_time = struct.unpack("<Q", data[108:116])[0]
        start_sector = struct.unpack("<I", data[116:120])[0]
        size = struct.unpack("<Q", data[120:128])[0]

        return DirectoryEntry(
            name=name,
            entry_type=entry_type,
            color=color,
            left_sibling_id=left_sibling_id,
            right_sibling_id=right_sibling_id,
            child_id=child_id,
            clsid=clsid,
            state_bits=state_bits,
            creation_time=creation_time,
            modification_time=modification_time,
            start_sector=start_sector,
            size=size,
        )

    def _build_stream_paths(self) -> None:
        """스트림 경로 맵을 구축합니다."""
        self._stream_paths = {}

        if not self._directory:
            return

        def traverse(entry_id: int, path_prefix: str) -> None:
            if entry_id >= len(self._directory) or entry_id == 0xFFFFFFFF:
                return

            entry = self._directory[entry_id]
            if entry.entry_type == DIR_TYPE_EMPTY:
                return

            current_path = f"{path_prefix}/{entry.name}" if path_prefix else entry.name

            if entry.is_stream:
                self._stream_paths[current_path] = entry_id

            # 왼쪽/오른쪽 형제 (RB 트리)
            traverse(entry.left_sibling_id, path_prefix)
            traverse(entry.right_sibling_id, path_prefix)

            # 자식 (스토리지인 경우)
            if entry.is_storage and entry.child_id != 0xFFFFFFFF:
                traverse(entry.child_id, current_path)

        # Root Entry (인덱스 0)부터 시작
        if self._directory and self._directory[0].is_root:
            root = self._directory[0]
            # Root의 자식부터 탐색
            if root.child_id != 0xFFFFFFFF:
                traverse(root.child_id, "")

    def _build_mini_stream(self) -> None:
        """Mini Stream을 구축합니다."""
        if not self._directory:
            return

        # Root Entry의 스트림이 Mini Stream
        root = self._directory[0]
        if root.size > 0 and root.start_sector != ENDOFCHAIN:
            self._mini_stream = self._read_stream_data(root.start_sector, root.size)

        # Mini FAT 구축
        if self._first_mini_fat_sector != ENDOFCHAIN and self._mini_fat_sector_count > 0:
            mini_fat_data = self._read_stream_data(
                self._first_mini_fat_sector,
                self._mini_fat_sector_count * self._sector_size,
            )
            self._mini_fat = []
            for i in range(len(mini_fat_data) // 4):
                entry = struct.unpack("<I", mini_fat_data[i * 4 : (i + 1) * 4])[0]
                self._mini_fat.append(entry)

    def _read_mini_stream_data(self, start_sector: int, size: int) -> bytes:
        """Mini Stream에서 데이터를 읽습니다."""
        if not self._mini_stream:
            return b""

        data = b""
        sect = start_sector
        remaining = size
        visited = set()

        while sect not in (ENDOFCHAIN, FREESECT) and remaining > 0:
            if sect >= len(self._mini_fat):
                break
            if sect in visited:
                break
            visited.add(sect)

            offset = sect * self._mini_sector_size
            chunk_size = min(self._mini_sector_size, remaining)
            data += self._mini_stream[offset : offset + chunk_size]
            remaining -= chunk_size
            sect = self._mini_fat[sect] if sect < len(self._mini_fat) else ENDOFCHAIN

        return data

    def list_streams(self) -> list[str]:
        """모든 스트림 이름을 반환합니다."""
        return list(self._stream_paths.keys())

    def exists(self, name: str) -> bool:
        """스트림이 존재하는지 확인합니다."""
        return name in self._stream_paths

    def get_stream_size(self, name: str) -> int:
        """스트림 크기를 반환합니다."""
        if name not in self._stream_paths:
            raise OleReadError(f"스트림을 찾을 수 없습니다: {name}")

        entry_id = self._stream_paths[name]
        return self._directory[entry_id].size

    def read_stream(self, name: str) -> bytes:
        """스트림 데이터를 읽습니다.

        Args:
            name: 스트림 경로 (예: "DocInfo", "BodyText/Section0")

        Returns:
            스트림 데이터 (bytes)

        Raises:
            OleReadError: 스트림을 찾을 수 없는 경우
        """
        if name not in self._stream_paths:
            raise OleReadError(f"스트림을 찾을 수 없습니다: {name}")

        entry_id = self._stream_paths[name]
        entry = self._directory[entry_id]

        if entry.size == 0:
            return b""

        # Mini Stream Cutoff 이하면 Mini FAT 사용
        if entry.size < self._mini_stream_cutoff:
            return self._read_mini_stream_data(entry.start_sector, entry.size)
        else:
            return self._read_stream_data(entry.start_sector, entry.size)

    def listdir(self) -> list[list[str]]:
        """olefile 호환: 모든 경로를 리스트 형태로 반환합니다."""
        result = []
        for path in self._stream_paths:
            parts = path.split("/")
            result.append(parts)
        return result
