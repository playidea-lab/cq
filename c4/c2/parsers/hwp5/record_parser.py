"""RecordParser - HWP 5.0 데이터 레코드 파서.

HWP 5.0 스펙에 따른 레코드 구조:
- 레코드 헤더: 32bits (TagID 10bits, Level 10bits, Size 12bits)
- 확장 레코드: Size == 0xFFF일 때 추가 DWORD로 실제 크기 표현
- DocInfo, BodyText 등의 스트림은 zlib 압축될 수 있음 (-15 윈도우)

참조:
- HWP 5.0 스펙 4.1절: 데이터 레코드 구조
"""

import struct
import zlib
from dataclasses import dataclass
from enum import IntEnum

# HWPTAG 상수 정의 (HWP 5.0 스펙 기준)
HWPTAG_BEGIN = 0x010


class HwpTagId(IntEnum):
    """HWP 레코드 태그 ID."""

    # DocInfo 관련 태그
    DOCUMENT_PROPERTIES = HWPTAG_BEGIN  # 0x10
    ID_MAPPINGS = HWPTAG_BEGIN + 1  # 0x11
    BIN_DATA = HWPTAG_BEGIN + 2  # 0x12
    FACE_NAME = HWPTAG_BEGIN + 3  # 0x13
    BORDER_FILL = HWPTAG_BEGIN + 4  # 0x14
    CHAR_SHAPE = HWPTAG_BEGIN + 5  # 0x15
    TAB_DEF = HWPTAG_BEGIN + 6  # 0x16
    NUMBERING = HWPTAG_BEGIN + 7  # 0x17
    BULLET = HWPTAG_BEGIN + 8  # 0x18
    PARA_SHAPE = HWPTAG_BEGIN + 9  # 0x19
    STYLE = HWPTAG_BEGIN + 10  # 0x1A
    DOC_DATA = HWPTAG_BEGIN + 11  # 0x1B
    DISTRIBUTE_DOC_DATA = HWPTAG_BEGIN + 12  # 0x1C
    # RESERVED = HWPTAG_BEGIN + 13  # 0x1D
    COMPATIBLE_DOCUMENT = HWPTAG_BEGIN + 14  # 0x1E
    LAYOUT_COMPATIBILITY = HWPTAG_BEGIN + 15  # 0x1F
    TRACKCHANGE = HWPTAG_BEGIN + 16  # 0x20

    # BodyText 관련 태그
    PARA_HEADER = HWPTAG_BEGIN + 50  # 0x42
    PARA_TEXT = HWPTAG_BEGIN + 51  # 0x43
    PARA_CHAR_SHAPE = HWPTAG_BEGIN + 52  # 0x44
    PARA_LINE_SEG = HWPTAG_BEGIN + 53  # 0x45
    PARA_RANGE_TAG = HWPTAG_BEGIN + 54  # 0x46
    CTRL_HEADER = HWPTAG_BEGIN + 55  # 0x47
    LIST_HEADER = HWPTAG_BEGIN + 56  # 0x48
    PAGE_DEF = HWPTAG_BEGIN + 57  # 0x49
    FOOTNOTE_SHAPE = HWPTAG_BEGIN + 58  # 0x4A
    PAGE_BORDER_FILL = HWPTAG_BEGIN + 59  # 0x4B
    SHAPE_COMPONENT = HWPTAG_BEGIN + 60  # 0x4C
    TABLE = HWPTAG_BEGIN + 61  # 0x4D
    SHAPE_COMPONENT_LINE = HWPTAG_BEGIN + 62  # 0x4E
    SHAPE_COMPONENT_RECTANGLE = HWPTAG_BEGIN + 63  # 0x4F
    SHAPE_COMPONENT_ELLIPSE = HWPTAG_BEGIN + 64  # 0x50
    SHAPE_COMPONENT_ARC = HWPTAG_BEGIN + 65  # 0x51
    SHAPE_COMPONENT_POLYGON = HWPTAG_BEGIN + 66  # 0x52
    SHAPE_COMPONENT_CURVE = HWPTAG_BEGIN + 67  # 0x53
    SHAPE_COMPONENT_OLE = HWPTAG_BEGIN + 68  # 0x54
    SHAPE_COMPONENT_PICTURE = HWPTAG_BEGIN + 69  # 0x55
    SHAPE_COMPONENT_CONTAINER = HWPTAG_BEGIN + 70  # 0x56
    CTRL_DATA = HWPTAG_BEGIN + 71  # 0x57
    EQEDIT = HWPTAG_BEGIN + 72  # 0x58
    # ... 추가 태그는 필요시 정의

    # 특수 태그
    MEMO_SHAPE = HWPTAG_BEGIN + 76  # 0x5C
    FORBIDDEN_CHAR = HWPTAG_BEGIN + 78  # 0x5E
    TRACK_CHANGE = HWPTAG_BEGIN + 80  # 0x60
    TRACK_CHANGE_AUTHOR = HWPTAG_BEGIN + 81  # 0x61


# 확장 레코드 크기 마커
EXTENDED_SIZE_MARKER = 0xFFF


@dataclass
class Record:
    """HWP 데이터 레코드."""

    tag_id: int
    level: int
    size: int
    data: bytes

    @property
    def tag_name(self) -> str:
        """태그 ID의 이름을 반환합니다."""
        try:
            return HwpTagId(self.tag_id).name
        except ValueError:
            return f"UNKNOWN_0x{self.tag_id:03X}"

    def __repr__(self) -> str:
        return (
            f"Record(tag={self.tag_name}, level={self.level}, "
            f"size={self.size}, data={len(self.data)} bytes)"
        )


class RecordParseError(Exception):
    """레코드 파싱 오류."""

    pass


class RecordParser:
    """HWP 5.0 데이터 레코드 파서.

    사용법:
        parser = RecordParser()
        records = parser.parse_records(data)

        # 압축된 데이터 처리 (자동 감지)
        records = parser.parse_records(compressed_data, decompress=True)
    """

    def __init__(self) -> None:
        """RecordParser를 초기화합니다."""
        pass

    def decompress(self, data: bytes) -> bytes:
        """zlib 압축을 해제합니다.

        HWP 5.0은 raw deflate 압축을 사용합니다 (윈도우 비트: -15).

        Args:
            data: 압축된 데이터

        Returns:
            압축 해제된 데이터

        Raises:
            RecordParseError: 압축 해제 실패 시
        """
        try:
            # HWP는 raw deflate 사용 (zlib 헤더 없음)
            return zlib.decompress(data, -15)
        except zlib.error as e:
            raise RecordParseError(f"zlib 압축 해제 실패: {e}") from e

    def is_compressed(self, data: bytes) -> bool:
        """데이터가 압축되어 있는지 확인합니다.

        레코드 헤더를 파싱해보고 실패하면 압축된 것으로 판단합니다.

        Args:
            data: 확인할 데이터

        Returns:
            압축 여부
        """
        if len(data) < 4:
            return False

        # 첫 레코드 헤더 파싱 시도
        header = struct.unpack("<I", data[:4])[0]
        tag_id = header & 0x3FF
        level = (header >> 10) & 0x3FF
        # size는 압축 판단에 사용하지 않으므로 추출하지 않음

        # 유효한 HWP 태그 범위 확인 (0x010 ~ 0x0FF 정도가 일반적)
        # 태그 ID가 비정상적이거나 레벨이 너무 높으면 압축된 것으로 판단
        if tag_id < 0x010 or tag_id > 0x200:
            return True
        if level > 100:  # 레벨이 100을 넘는 경우는 거의 없음
            return True

        return False

    def parse_header(self, data: bytes, offset: int = 0) -> tuple[int, int, int, int]:
        """레코드 헤더를 파싱합니다.

        Args:
            data: 레코드 데이터
            offset: 시작 오프셋

        Returns:
            (tag_id, level, size, header_size) 튜플
            - header_size: 헤더가 차지하는 바이트 수 (4 또는 8)

        Raises:
            RecordParseError: 파싱 실패 시
        """
        if offset + 4 > len(data):
            raise RecordParseError(f"헤더를 읽기에 데이터가 부족합니다: offset={offset}")

        # 32비트 헤더 읽기
        header = struct.unpack_from("<I", data, offset)[0]

        # 비트 추출: TagID(10bits) | Level(10bits) | Size(12bits)
        tag_id = header & 0x3FF  # 하위 10비트
        level = (header >> 10) & 0x3FF  # 중간 10비트
        size = (header >> 20) & 0xFFF  # 상위 12비트

        header_size = 4

        # 확장 레코드 처리: Size == 0xFFF일 때 추가 DWORD
        if size == EXTENDED_SIZE_MARKER:
            if offset + 8 > len(data):
                raise RecordParseError(
                    f"확장 크기를 읽기에 데이터가 부족합니다: offset={offset}"
                )
            size = struct.unpack_from("<I", data, offset + 4)[0]
            header_size = 8

        return tag_id, level, size, header_size

    def parse_records(
        self, data: bytes, *, decompress: bool = True
    ) -> list[Record]:
        """데이터에서 레코드를 파싱합니다.

        Args:
            data: 레코드 데이터 (압축되었을 수 있음)
            decompress: True이면 압축 자동 감지 및 해제

        Returns:
            Record 객체 리스트

        Raises:
            RecordParseError: 파싱 실패 시
        """
        if not data:
            return []

        # 압축 자동 감지 및 해제
        if decompress and self.is_compressed(data):
            data = self.decompress(data)

        records: list[Record] = []
        offset = 0

        while offset < len(data):
            # 헤더 파싱
            tag_id, level, size, header_size = self.parse_header(data, offset)
            offset += header_size

            # 데이터 추출
            if offset + size > len(data):
                raise RecordParseError(
                    f"레코드 데이터가 부족합니다: "
                    f"offset={offset}, size={size}, data_len={len(data)}"
                )

            record_data = data[offset : offset + size]
            offset += size

            records.append(
                Record(
                    tag_id=tag_id,
                    level=level,
                    size=size,
                    data=record_data,
                )
            )

        return records

    def parse_record(self, data: bytes, offset: int = 0) -> tuple[Record, int]:
        """단일 레코드를 파싱합니다.

        Args:
            data: 레코드 데이터
            offset: 시작 오프셋

        Returns:
            (Record, 다음 오프셋) 튜플
        """
        tag_id, level, size, header_size = self.parse_header(data, offset)
        data_start = offset + header_size
        data_end = data_start + size

        if data_end > len(data):
            raise RecordParseError(
                f"레코드 데이터가 부족합니다: "
                f"offset={offset}, size={size}, data_len={len(data)}"
            )

        record = Record(
            tag_id=tag_id,
            level=level,
            size=size,
            data=data[data_start:data_end],
        )

        return record, data_end

    def build_tree(self, records: list[Record]) -> list[dict]:
        """레코드 리스트를 계층 구조 트리로 변환합니다.

        Args:
            records: Record 리스트

        Returns:
            계층 구조 딕셔너리 리스트
        """
        if not records:
            return []

        root: list[dict] = []
        stack: list[tuple[int, list[dict]]] = [(-1, root)]

        for record in records:
            node = {
                "record": record,
                "children": [],
            }

            # 현재 레벨에 맞는 부모 찾기
            while stack and stack[-1][0] >= record.level:
                stack.pop()

            if not stack:
                stack = [(-1, root)]

            # 부모의 children에 추가
            parent_children = stack[-1][1]
            parent_children.append(node)

            # 스택에 현재 노드 추가
            stack.append((record.level, node["children"]))

        return root
